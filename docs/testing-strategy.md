# Testing Strategy: treesitter-go Verification

This document defines how we verify that the Go port of tree-sitter produces
correct results matching the reference C implementation. Correctness is the
primary goal — every test category exists to catch a specific class of divergence.

---

## 1. Tree-sitter's Existing Test Suite

### Test Suite Catalog

The tree-sitter reference implementation (`github.com/tree-sitter/tree-sitter`)
has a substantial test suite organized across several locations:

#### Rust Integration Tests (`crates/cli/src/tests/`)

21 test files totaling ~380KB of test code:

| File | Size | What It Tests |
|------|------|---------------|
| `query_test.rs` | 181KB | Query compiler and cursor: capture, match, predicate evaluation, quantifiers, alternation, anchors, negated fields. The largest test file. |
| `parser_test.rs` | 67KB | Core Parser API: parse from string, parse from callback, edit + reparse, error recovery, cancellation/timeout, included ranges. |
| `node_test.rs` | 42KB | Node traversal: child/sibling/parent navigation, named vs anonymous nodes, field access, byte/point positions, S-expression output. |
| `tree_test.rs` | 26KB | Tree editing, change tracking, included ranges, tree cloning. |
| `highlight_test.rs` | 26KB | Syntax highlighting (out of scope for initial port). |
| `corpus_test.rs` | 16KB | Randomized edit/reparse cycles against grammar corpus tests. The primary correctness oracle. |
| `wasm_language_test.rs` | 13KB | WASM language loading (out of scope). |
| `tags_test.rs` | 12KB | Tag extraction (out of scope for initial port). |
| `language_test.rs` | 6KB | Lookahead iterator, parse state enumeration, language metadata. |
| `detect_language.rs` | 6.6KB | Language detection (out of scope). |
| `text_provider_test.rs` | 5KB | Custom text provider implementations. |
| `async_boundary_test.rs` | 3.7KB | Node/cursor safety across async boundaries. |
| `pathological_test.rs` | 551B | Pathological input (parser robustness). |

#### Test Grammars (`test/fixtures/test_grammars/`)

57 purpose-built grammars, each with a `grammar.js`, `corpus.txt`, and optional
`scanner.c`. These test specific parser features:

- `external_tokens/` — external scanner integration
- `external_and_internal_tokens/` — mixed token types
- `epsilon_rules/` — empty productions
- `dynamic_precedence/` — precedence resolution
- `reserved_words/` — keyword handling
- `unicode_classes/` — Unicode pattern matching
- `aliased_inlined_rules/` — alias behavior
- `depends_on_column/` — column-dependent lexing
- `conflict_in_repeat_rule/` — grammar conflict resolution
- (and 48 more)

#### Real Language Fixtures (`test/fixtures/fixtures.json`)

15 production grammars fetched from upstream repos at pinned versions:

```
bash v0.25.0, c v0.24.1, cpp v0.23.4, embedded-template v0.25.0,
go v0.25.0, html v0.23.2, java v0.23.5, javascript v0.25.0,
jsdoc v0.23.2, json v0.24.8, php v0.24.2, python v0.23.6,
ruby v0.23.1, rust v0.24.0, typescript v0.23.2
```

Each grammar provides `test/corpus/` tests, example source files for
benchmarking, and query files (highlights, injections, locals, tags).

#### Benchmarks (`crates/cli/benches/benchmark.rs`)

Performance benchmarks measuring parse throughput (bytes/ms) across all
grammar example files, with configurable repetition count.

#### CI Configuration (`.github/workflows/`)

- `build.yml` — tests across Linux (ARM64/ARM32/x86-64/x86/PPC64), Windows
  (ARM64/x86-64/x86), macOS (ARM64/x86-64), Wasm
- `sanitize.yml` — address sanitizer and undefined behavior sanitizer runs

### Which Tests Can We Run as Black-Box Tests?

**Yes — grammar corpus tests.** These provide input text and expected parse tree
S-expressions. We can parse the same input with our Go parser and compare the
output. This is the primary verification mechanism (Section 2).

**Yes — query tests (partially).** Tests that construct a tree, run a query, and
check captured nodes can be translated. Tests that depend on Rust-specific APIs
need adaptation.

**No — tests of internal APIs.** Tests that call `ts_parser__advance`,
`ts_stack_push`, `ts_subtree_make_mut`, or other internal functions must be
ported to equivalent Go tests (Section 3).

**No — edit/reparse fuzz tests (directly).** The corpus test runner
(`corpus_test.rs`) uses seeded randomized edits. We should build an equivalent
Go fuzz harness rather than trying to replicate the exact Rust RNG sequence.

---

## 2. Grammar Corpus Tests — The Primary Verification

### Corpus File Format

Every tree-sitter grammar has a `test/corpus/` directory with `.txt` files in
this format:

```
===============================
Test Name
===============================

source code here

---

(expected_parse_tree)
```

**Format details** (from `crates/cli/src/test.rs`, lines 35-55):

- **Header delimiter**: 3+ equals signs, may have suffix text
- **Test name**: One or more lines between delimiters; may include attributes
- **Attributes** (optional, prefixed with `:`):
  - `:skip` — skip this test
  - `:error` — expect ERROR/MISSING nodes in the parse tree
  - `:platform(os)` — platform-specific test
  - `:language(name)` — use specific language for this test
  - `:cst` — compare concrete syntax tree (preserves whitespace in output)
  - `:fail-fast` — stop on first failure
- **Divider**: 3+ hyphens (the _longest_ line of hyphens separates input from
  output, allowing shorter runs of hyphens to appear in source code)
- **Expected output**: S-expression, with:
  - Comments (`;` prefix) stripped
  - Whitespace normalized to single spaces
  - Field annotations like `name:` optionally included
  - `:cst` mode preserves raw formatting

### Go Corpus Test Runner Design

Build a `corpustest` package under `internal/corpustest/` or as a test helper:

```go
// TestCase represents a single corpus test.
type TestCase struct {
    Name       string
    Input      []byte
    Expected   string   // Normalized S-expression
    Attributes TestAttributes
}

type TestAttributes struct {
    Skip      bool
    Error     bool     // Expect ERROR nodes
    Languages []string // Language name(s) for this test
    CST       bool     // Compare full CST
}

// ParseCorpusFile parses a corpus .txt file into test cases.
func ParseCorpusFile(data []byte) ([]TestCase, error)

// ParseCorpusDir parses all .txt files in a directory.
func ParseCorpusDir(dir string) ([]TestCase, error)
```

**Parsing implementation notes:**
- Use the "longest divider" rule: scan all `---+` lines in a test section and
  use the longest as the separator. This matches the C/Rust behavior.
- Normalize expected output: strip `;` comments, collapse whitespace, remove
  space before `)`.
- Handle `:skip` and `:error` attributes.

### Comparison Logic

```go
func TestCorpus(t *testing.T, lang *ts.Language, cases []TestCase) {
    parser := ts.NewParser()
    parser.SetLanguage(lang)

    for _, tc := range cases {
        t.Run(tc.Name, func(t *testing.T) {
            if tc.Attributes.Skip {
                t.Skip("corpus test marked :skip")
            }

            tree := parser.ParseString(nil, tc.Input)
            actual := tree.RootNode().String() // S-expression

            // Normalize actual output to match expected format
            actual = normalizeS expression(actual)

            if tc.Attributes.Error {
                // For :error tests, verify ERROR or MISSING node exists
                if !containsErrorNode(tree.RootNode()) {
                    t.Errorf("expected ERROR node in parse tree")
                }
                return
            }

            if actual != tc.Expected {
                t.Errorf("parse tree mismatch\ninput: %q\nexpected:\n%s\nactual:\n%s",
                    tc.Input, tc.Expected, actual)
            }
        })
    }
}
```

### Running Across All Major Grammars

Create a test driver that clones grammar repos and runs their corpus tests:

```
testdata/
├── grammars.json          # List of grammars to test (name, repo, version)
├── fetch_grammars.sh      # Script to clone/update grammar repos
└── grammars/
    ├── tree-sitter-json/
    │   └── test/corpus/
    ├── tree-sitter-go/
    │   └── test/corpus/
    ├── tree-sitter-javascript/
    │   └── test/corpus/
    └── ...
```

**`grammars.json`** mirrors tree-sitter's `test/fixtures/fixtures.json`:

```json
[
  {"name": "json",       "repo": "tree-sitter/tree-sitter-json",       "version": "v0.24.8"},
  {"name": "go",         "repo": "tree-sitter/tree-sitter-go",         "version": "v0.25.0"},
  {"name": "javascript", "repo": "tree-sitter/tree-sitter-javascript", "version": "v0.25.0"},
  {"name": "python",     "repo": "tree-sitter/tree-sitter-python",     "version": "v0.23.6"},
  {"name": "rust",       "repo": "tree-sitter/tree-sitter-rust",       "version": "v0.24.0"},
  {"name": "c",          "repo": "tree-sitter/tree-sitter-c",          "version": "v0.24.1"},
  {"name": "typescript",  "repo": "tree-sitter/tree-sitter-typescript", "version": "v0.23.2"}
]
```

**Makefile targets:**

```makefile
fetch-test-grammars:
    go run ./cmd/fetch-grammars -config testdata/grammars.json -output testdata/grammars/

test-corpus:
    go test ./... -run TestCorpus -v -count=1

test-corpus-json:
    go test ./... -run TestCorpus/json -v
```

**Priority order for grammar support:**
1. JSON (no external scanner — validates core parser)
2. Go (simple external scanner — validates scanner interface)
3. JavaScript (moderate external scanner — template literals, regex, JSX)
4. Python (indentation scanner — classic external scanner case)
5. Rust (moderate complexity)
6. C (moderate complexity)
7. TypeScript (complex — ~4000 lex states, large scanner)

### Test Infrastructure Code

The corpus test runner is the first piece of test infrastructure to build. It
should be ready before Phase 3 (parser implementation) begins, so that parser
development can be test-driven against real corpus expectations.

---

## 3. Porting Non-Black-Box Tests

### Internal API Test Inventory

The following tests from tree-sitter's Rust test suite test internal APIs that
cannot be exercised through the black-box corpus test format. Each must be
ported to Go `_test.go` files.

#### Parser Internal Tests (`parser_test.rs`, 67KB)

**What it tests (with references to the C equivalent):**
- `ts_parser_parse_string` — basic parsing from byte slice
- `ts_parser_parse` — parsing from callback (`Input` interface)
- Error recovery: missing token insertion, token skipping, repair strategies
- `ts_parser_set_included_ranges` — language injection ranges
- `ts_parser_set_timeout_micros` — parse cancellation (Go: `context.Context`)
- Multiple parse calls on same parser (state reset)
- Parsing with `oldTree` parameter (incremental reuse)
- Parser logging callback
- GLR ambiguity: multiple stack versions, merge, error cost comparison

**Go port**: `parser_test.go` — test `Parser.Parse()`, `Parser.ParseString()`,
error recovery behavior, included ranges. Use hand-compiled JSON/Go grammars
as test languages.

#### Node Tests (`node_test.rs`, 42KB)

**What it tests:**
- `ts_node_child` / `ts_node_named_child` / `ts_node_child_by_field_name`
- `ts_node_next_sibling` / `ts_node_prev_sibling`
- `ts_node_parent`
- `ts_node_start_byte` / `ts_node_end_byte` / `ts_node_start_point` / `ts_node_end_point`
- `ts_node_string` (S-expression output)
- `ts_node_descendant_for_byte_range`
- Alias nodes, extra nodes (comments), missing nodes
- `ts_node_is_named` / `ts_node_is_extra` / `ts_node_is_missing`
- `ts_node_eq` (identity comparison)
- Child count for nodes with visible/invisible children

**Go port**: `tree_test.go` and `node_test.go` — test `Node` value type methods.
Verify that the `visible_descendant_count` bookkeeping produces correct results
for `NamedChild(index)`.

#### Tree Tests (`tree_test.rs`, 26KB)

**What it tests:**
- `ts_tree_edit` — edit application and position adjustment
- `ts_tree_get_changed_ranges` — changed range detection
- Tree cloning
- Included ranges behavior across edits

**Go port**: `tree_test.go` — test `Tree.Edit()` and `Tree.ChangedRanges()`.
Focus on verifying structural sharing: after `Edit()`, unaffected subtrees
should be pointer-identical to the original.

#### Query Tests (`query_test.rs`, 181KB)

**What it tests (comprehensive list):**
- Query parsing: valid patterns, error reporting, syntax errors
- Named node matching, anonymous node matching
- Wildcard matching (`_`)
- Field constraints (`name: (identifier)`)
- Alternation (`[(true) (false)]`)
- Quantifiers (`?`, `+`, `*`)
- Captures (`@name`)
- Predicates: `#eq?`, `#not-eq?`, `#match?`, `#not-match?`, `#any-of?`, `#not-any-of?`
- Anchored patterns (`.` operator)
- Negated fields (`!field`)
- `ts_query_cursor_set_byte_range` / `ts_query_cursor_set_point_range`
- `ts_query_cursor_set_max_start_depth`
- Query step inspection and introspection APIs
- Pattern guarantees (`ts_query_is_pattern_guaranteed_at_step`)

**Go port**: `query_test.go` — this will be the largest test file. Port test
cases directly, adapting Rust API calls to Go equivalents.

#### Language Tests (`language_test.rs`, 6KB)

**What it tests:**
- `ts_lookahead_iterator_new` / `ts_lookahead_iterator_next`
- `ts_language_symbol_count` / `ts_language_field_count`
- `ts_language_state_count`
- Symbol metadata access

**Go port**: `language_test.go` — test `Language` struct accessors and parse
table lookup correctness.

### Test Infrastructure Needed

```go
// testutil/testutil.go — shared test helpers

// MustParse parses input with the given language, failing the test on error.
func MustParse(t *testing.T, lang *ts.Language, input []byte) *ts.Tree

// AssertSexp checks that a node's S-expression matches expected.
func AssertSexp(t *testing.T, node ts.Node, expected string)

// AssertRange checks a node's byte range.
func AssertRange(t *testing.T, node ts.Node, startByte, endByte uint32)

// AssertPoint checks a node's start/end points.
func AssertPoint(t *testing.T, node ts.Node, startRow, startCol, endRow, endCol uint32)
```

### Porting Priority

1. **parser_test.go** — needed for Phase 3 (parser implementation)
2. **node_test.go** + **tree_test.go** — needed for Phase 3
3. **language_test.go** — needed for Phase 1 (types and table loading)
4. **query_test.go** — needed for Phase 7 (query system)

Port tests incrementally as each phase begins. Don't batch-port all tests
upfront — port the tests relevant to the current phase.

---

## 4. Cross-Verification Strategy

### Differential Testing: Go vs C

The strongest guarantee of correctness is parsing identical inputs with both the
C and Go implementations and comparing results. This catches edge cases that
corpus tests miss.

#### Approach

```
                ┌─────────────┐
 source file ──>│  C parser   │──> S-expression A
                └─────────────┘
                ┌─────────────┐
 source file ──>│  Go parser  │──> S-expression B
                └─────────────┘
                      │
                 compare A == B
```

**Implementation**: A Go test binary that:
1. Links to the C tree-sitter library via CGo (test-only dependency)
2. Parses each input file with both C and Go parsers
3. Compares S-expression output, node byte ranges, and node field assignments

```go
// crosscheck/crosscheck_test.go
// +build crosscheck

func TestCrossCheck(t *testing.T) {
    files := collectSourceFiles("testdata/corpora/")
    cParser := cgo.NewParser()     // CGo wrapper around C tree-sitter
    goParser := ts.NewParser()

    for _, f := range files {
        input, _ := os.ReadFile(f.path)
        cTree := cParser.Parse(f.language, input)
        goTree := goParser.Parse(f.language, input)

        cSexp := cTree.RootNode().String()
        goSexp := goTree.RootNode().String()

        if cSexp != goSexp {
            t.Errorf("divergence on %s:\nC:  %s\nGo: %s", f.path, cSexp, goSexp)
        }

        // Also compare byte ranges for every node
        compareTrees(t, cTree.RootNode(), goTree.RootNode())
    }
}
```

#### Real-World Test Corpora

Use real codebases as test inputs — these exercise parsing patterns that
synthetic corpus tests don't cover:

| Language | Source | Files | Purpose |
|----------|--------|-------|---------|
| JSON | `package.json` files from npm | ~1000 | Varied structure, nested objects |
| Go | Go standard library (`src/`) | ~8000 | Production Go code, all features |
| JavaScript | top npm packages | ~500 | Real-world JS including edge cases |
| Python | CPython stdlib | ~1500 | Indentation, decorators, f-strings |
| Rust | Rust compiler source | ~2000 | Complex syntax, macros |

Collect a fixed set of these files (version-pinned) as `testdata/corpora/`.
Run differential testing in CI.

#### Node-Level Comparison

S-expression comparison catches most issues, but some bugs manifest only in
byte ranges or field assignments. The full comparison should check:

```go
func compareTrees(t *testing.T, cNode, goNode Node) {
    // 1. Same symbol / node type
    assert(cNode.Type() == goNode.Type())

    // 2. Same byte range
    assert(cNode.StartByte() == goNode.StartByte())
    assert(cNode.EndByte() == goNode.EndByte())

    // 3. Same point range
    assert(cNode.StartPoint() == goNode.StartPoint())
    assert(cNode.EndPoint() == goNode.EndPoint())

    // 4. Same child count
    assert(cNode.ChildCount() == goNode.ChildCount())
    assert(cNode.NamedChildCount() == goNode.NamedChildCount())

    // 5. Same field assignments
    for _, field := range language.Fields() {
        cChild := cNode.ChildByFieldName(field)
        goChild := goNode.ChildByFieldName(field)
        assert(cChild.IsNull() == goChild.IsNull())
    }

    // 6. Recurse into children
    for i := 0; i < cNode.ChildCount(); i++ {
        compareTrees(t, cNode.Child(i), goNode.Child(i))
    }
}
```

### Regression Testing

Maintain a `testdata/regressions/` directory with input files that previously
caused divergences. Each file has a companion `.expected` file with the correct
S-expression. These are run as standard Go tests and never removed.

```
testdata/regressions/
├── js-template-literal-nested.js
├── js-template-literal-nested.js.expected
├── py-triple-quote-at-eof.py
├── py-triple-quote-at-eof.py.expected
└── ...
```

---

## 5. Incremental Parsing Verification

Incremental parsing is the hardest feature to test because correctness means
"the result after edit + incremental reparse is identical to parsing the edited
text from scratch."

### Test Strategy

#### 5.1. Edit-Reparse Equivalence

For every test input, generate random edits and verify that incremental parsing
matches a full re-parse:

```go
func TestIncrementalEquivalence(t *testing.T, lang *ts.Language, input []byte) {
    parser := ts.NewParser()
    parser.SetLanguage(lang)

    tree := parser.ParseString(nil, input)

    for i := 0; i < 10; i++ {  // 10 iterations per input
        edits := generateRandomEdits(input, rand.Intn(3)+1)
        editedInput := applyEdits(input, edits)

        // Incremental parse
        for _, edit := range edits {
            tree.Edit(&edit.InputEdit)
        }
        incrementalTree := parser.ParseString(tree, editedInput)

        // Full re-parse
        parser.Reset()
        freshTree := parser.ParseString(nil, editedInput)

        // Compare
        assert(incrementalTree.RootNode().String() == freshTree.RootNode().String())

        // Prepare for next iteration
        input = editedInput
        tree = incrementalTree
    }
}
```

This mirrors the approach in tree-sitter's `corpus_test.rs` (lines 231-320).

#### 5.2. Edit + Undo Cycle

The tree-sitter corpus test uses an edit-then-undo approach:

1. Parse original input → tree A
2. Apply random edits → tree B
3. Undo all edits in reverse → tree C
4. Verify tree C's S-expression matches tree A's

This tests that edit tracking and position adjustment are symmetric.

```go
func TestEditUndoCycle(t *testing.T, lang *ts.Language, input []byte) {
    parser := ts.NewParser()
    parser.SetLanguage(lang)
    original := parser.ParseString(nil, input)
    originalSexp := original.RootNode().String()

    edits, undos := generateEditsWithUndos(input)
    edited := applyEdits(input, edits)

    for _, e := range edits {
        original.Edit(&e.InputEdit)
    }
    tree := parser.ParseString(original, edited)

    // Undo
    restored := applyEdits(edited, undos)
    for _, u := range undos {
        tree.Edit(&u.InputEdit)
    }
    restoredTree := parser.ParseString(tree, restored)

    assert(restoredTree.RootNode().String() == originalSexp)
}
```

#### 5.3. Changed Ranges Verification

After incremental parsing, `Tree.ChangedRanges(oldTree)` must report exactly
the regions that differ. Verify by checking that nodes outside changed ranges
have identical byte ranges and types in both trees.

```go
func TestChangedRanges(t *testing.T, oldTree, newTree *ts.Tree, oldInput, newInput []byte) {
    ranges := newTree.ChangedRanges(oldTree)

    // Walk both trees and verify nodes outside changed ranges are identical
    oldCursor := oldTree.RootNode().Walk()
    newCursor := newTree.RootNode().Walk()

    // For each node in the new tree that is NOT within any changed range,
    // find the corresponding node in the old tree and assert equality
    // ...
}
```

#### 5.4. Tree Consistency Checks

After every parse (incremental or fresh), verify structural invariants. This
mirrors `check_consistent_sizes()` in `crates/cli/src/fuzz/corpus_test.rs`:

```go
func assertTreeConsistent(t *testing.T, node ts.Node, input []byte) {
    // 1. Every node's byte range is within the input
    assert(node.EndByte() <= uint32(len(input)))

    // 2. Every child's range is within its parent's range
    // 3. Children are ordered: child[i].EndByte <= child[i+1].StartByte
    // 4. Named child count matches actual named children
    // 5. Point positions match line/column offsets in the input

    for i := 0; i < int(node.ChildCount()); i++ {
        child := node.Child(i)
        assertTreeConsistent(t, child, input)
    }
}
```

#### 5.5. Seeded Randomization

Use deterministic seeding for reproducible failures, matching tree-sitter's
approach:

```go
func TestIncrementalCorpus(t *testing.T) {
    seed := int64(time.Now().UnixNano())
    if s := os.Getenv("TREESITTER_SEED"); s != "" {
        seed, _ = strconv.ParseInt(s, 10, 64)
    }
    t.Logf("seed: %d", seed)
    rng := rand.New(rand.NewSource(seed))
    // ... use rng for edit generation
}
```

### Random Edit Generation

Port the edit generation strategy from `crates/cli/src/fuzz/edits.rs`:

```go
func generateRandomEdit(rng *rand.Rand, input []byte) Edit {
    switch rng.Intn(10) {
    case 0, 1: // 20% — insert at end
        return Edit{Position: len(input), Inserted: randomWords(rng, 3)}
    case 2, 3, 4: // 30% — delete from end
        delLen := min(rng.Intn(30), len(input))
        return Edit{Position: len(input) - delLen, Deleted: delLen}
    case 5, 6, 7: // 30% — insert at random position
        pos := rng.Intn(len(input) + 1)
        return Edit{Position: pos, Inserted: randomWords(rng, rng.Intn(3)+1)}
    default: // 20% — replace at random position
        pos := rng.Intn(len(input) + 1)
        delLen := rng.Intn(len(input) - pos + 1)
        return Edit{Position: pos, Deleted: delLen, Inserted: randomWords(rng, rng.Intn(3)+1)}
    }
}

func randomWords(rng *rand.Rand, maxCount int) []byte {
    // Generate random words: alphanumeric strings and operators (+, -, <, >, etc.)
    // Separated by spaces or newlines (20% chance of newline)
    // Matches the distribution in tree-sitter's random.rs
}
```

---

## 6. Performance Testing

### Benchmark Suite

Use Go's `testing.B` benchmark framework:

```go
// benchmark_test.go

func BenchmarkParseJSON1KB(b *testing.B) {
    input := loadFixture("testdata/bench/small.json")  // ~1KB
    parser := ts.NewParser()
    parser.SetLanguage(jsonLang)
    b.ResetTimer()
    b.SetBytes(int64(len(input)))
    for i := 0; i < b.N; i++ {
        parser.ParseString(nil, input)
    }
}

func BenchmarkParseGo10KB(b *testing.B) { /* ... */ }
func BenchmarkParseJS100KB(b *testing.B) { /* ... */ }
func BenchmarkParseGo1MB(b *testing.B) { /* ... */ }
func BenchmarkIncrementalReparse(b *testing.B) { /* ... */ }
func BenchmarkQueryMatch(b *testing.B) { /* ... */ }
```

### Performance Targets

From the design document (these are targets, not requirements):

| Operation | C (typical) | Go target | Acceptable |
|-----------|-------------|-----------|------------|
| Parse 1KB JSON | ~50 us | < 200 us | 4x slower |
| Parse 10KB Go source | ~500 us | < 2 ms | 4x slower |
| Incremental reparse (1 char, 10KB) | ~5 us | < 50 us | 10x slower |
| Query match (simple, 10KB) | < 100 us | < 500 us | 5x slower |

### Comparison Against C Implementation

Build a benchmark harness that runs the same files through both C (via CGo)
and Go parsers:

```go
// +build crossbench

func BenchmarkComparison(b *testing.B) {
    files := []struct{ name, path string }{
        {"json-1kb", "testdata/bench/small.json"},
        {"go-10kb", "testdata/bench/medium.go"},
        {"js-100kb", "testdata/bench/large.js"},
    }

    for _, f := range files {
        input, _ := os.ReadFile(f.path)

        b.Run("c/"+f.name, func(b *testing.B) {
            p := cgo.NewParser()
            b.SetBytes(int64(len(input)))
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                p.Parse(input)
            }
        })

        b.Run("go/"+f.name, func(b *testing.B) {
            p := ts.NewParser()
            b.SetBytes(int64(len(input)))
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                p.ParseString(nil, input)
            }
        })
    }
}
```

### Memory Profiling

Track allocations per parse:

```go
func TestAllocations(t *testing.T) {
    input := loadFixture("testdata/bench/medium.go")
    parser := ts.NewParser()
    parser.SetLanguage(goLang)

    // Warm up
    parser.ParseString(nil, input)

    // Measure
    var stats runtime.MemStats
    runtime.ReadMemStats(&stats)
    allocsBefore := stats.Mallocs

    for i := 0; i < 100; i++ {
        parser.ParseString(nil, input)
    }

    runtime.ReadMemStats(&stats)
    allocsPerParse := (stats.Mallocs - allocsBefore) / 100

    t.Logf("allocations per parse: %d", allocsPerParse)
    t.Logf("input size: %d bytes", len(input))
}
```

### GC Impact Measurement

```go
func TestGCPause(t *testing.T) {
    // Parse a large file and measure GC pause time
    input := loadFixture("testdata/bench/large.js")  // ~1MB
    parser := ts.NewParser()
    parser.SetLanguage(jsLang)

    debug.SetGCPercent(100)
    var maxPause time.Duration

    // Register GC callback
    // Parse and track GC pauses
    // Report max and p99 pause times
}
```

### Benchdata Collection

Collect benchmark fixtures from real projects (version-pinned):

```
testdata/bench/
├── small.json        # 1KB — package.json
├── medium.go         # 10KB — Go source file
├── medium.py         # 10KB — Python source file
├── medium.js         # 10KB — JavaScript source file
├── large.js          # 100KB — bundled JavaScript
├── large.go          # 100KB — generated Go code
└── xlarge.go         # 1MB — very large generated Go file
```

---

## 7. External Scanner Testing

### The Challenge

External scanners contain language-specific lexing logic that cannot be tested
in isolation — they depend on the full parser state to determine which tokens
are valid. Testing must verify that the Go scanner implementation produces the
same tokens as the C scanner in all contexts.

### Strategy 1: Corpus Test Coverage

Grammar corpus tests already exercise external scanner tokens. For example:

- **JavaScript**: template literals (`` `hello ${name}` ``), regex literals
  (`/pattern/flags`), JSX tags
- **Python**: indentation (INDENT/DEDENT/NEWLINE), f-strings, triple-quoted
  strings
- **Go**: raw string literals
- **TypeScript**: template literals, JSX, automatic semicolon insertion
- **Ruby**: heredocs, string interpolation, regex

If the corpus tests pass for a grammar, its external scanner is correct for
those inputs. This is the primary verification.

### Strategy 2: Edge Case Test Files

For each external scanner, write targeted test files that exercise edge cases:

#### Python Scanner Edge Cases
```python
# Indentation edge cases
if True:
    if True:
        pass
    pass    # Dedent by 1
pass        # Dedent by 2

# Empty body after colon
if True:
    pass

# Mixed indentation (tabs and spaces — should this error?)
# Trailing whitespace after dedent
# Nested f-strings: f"{f'{x}'}"
# Triple-quote at end of file (no trailing newline)
```

#### JavaScript Scanner Edge Cases
```javascript
// Template literal nesting
`a ${`b ${c}`} d`

// Regex vs division ambiguity
x = a / b / c
x = /regex/g

// JSX vs comparison
x = <div></div>
x = a < b && b > c

// Automatic semicolon insertion
return
  value

// Template literal with line breaks
`line1
line2`
```

#### Go Scanner Edge Cases
```go
// Raw string with backticks
x := `raw string with "quotes" and 'apostrophes'`

// Raw string with no content
x := ``

// Nested raw strings (not possible in Go, but test parser behavior)
```

### Strategy 3: Serialization Round-Trip

External scanner state must survive serialization/deserialization for
incremental parsing. Test that:

1. Parse a file → scanner state S1
2. Serialize S1 → bytes
3. Deserialize bytes → scanner state S2
4. Parse the same file starting from S2 → same result

```go
func TestScannerSerialization(t *testing.T) {
    scanner := lang.NewExternalScanner()

    // Parse to establish state
    // ...

    // Serialize
    buf := make([]byte, 1024)
    n := scanner.Serialize(buf)

    // Create fresh scanner, deserialize
    scanner2 := lang.NewExternalScanner()
    scanner2.Deserialize(buf[:n])

    // Continue parsing — results must match
}
```

### Strategy 4: C Scanner Output Comparison

For complex scanners (TypeScript, C++), build a test harness that runs the C
scanner and Go scanner side-by-side on the same input, comparing which token
each produces at each call:

```go
// +build crosscheck

func TestScannerParity(t *testing.T) {
    inputs := loadScannerTestInputs("testdata/scanner/javascript/")

    for _, input := range inputs {
        cScanner := cgo.NewExternalScanner(jsLanguage)
        goScanner := jsgo.NewExternalScanner()

        lexer := newTestLexer(input)

        // At each position, call both scanners with the same valid_symbols
        // and compare: did they accept the same token? Same number of bytes?
        for !lexer.EOF() {
            validSymbols := computeValidSymbols(parseState)
            cResult := cScanner.Scan(lexer.Clone(), validSymbols)
            goResult := goScanner.Scan(lexer.Clone(), validSymbols)

            assert(cResult.accepted == goResult.accepted)
            assert(cResult.token == goResult.token)
            assert(cResult.bytes == goResult.bytes)

            // Advance lexer
            // ...
        }
    }
}
```

### Scanner Test Priority

| Grammar | Scanner Complexity | Priority | Key Tokens |
|---------|-------------------|----------|------------|
| JSON | None | N/A | — |
| Go | Simple (~50 lines) | 1 | Raw strings |
| JavaScript | Moderate (~400 lines) | 2 | Template literals, regex, JSX, ASI |
| Python | Moderate (~300 lines) | 3 | INDENT, DEDENT, NEWLINE, f-strings |
| TypeScript | Complex (~1000 lines) | 4 | All of JS + type-specific tokens |
| Ruby | Complex (~800 lines) | 5 | Heredocs, string interpolation |
| C++ | Very complex (~2000 lines) | 6 | Raw strings, template disambiguation |

---

## 8. CI Pipeline

### Test Stages

```yaml
# .github/workflows/test.yml
name: Test
on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.22' }
      - run: make test

  corpus-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: make fetch-test-grammars
      - run: make test-corpus

  cross-check:
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: make test-crosscheck  # CGo-based differential testing

  benchmarks:
    runs-on: ubuntu-latest
    if: github.event_name == 'pull_request'
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
      - run: make bench
      - uses: benchmark-action/github-action-benchmark@v1
        with:
          output-file-path: bench-results.json
          auto-push: false
          comment-on-alert: true
          alert-threshold: '150%'  # Alert if 50% slower
```

### Makefile Targets

```makefile
.PHONY: test test-corpus test-crosscheck bench

test:
    go test ./... -count=1

test-corpus:
    go test ./... -run TestCorpus -v -count=1 -timeout 10m

test-crosscheck:
    CGO_ENABLED=1 go test -tags crosscheck ./crosscheck/ -v -count=1 -timeout 30m

bench:
    go test ./... -bench=. -benchmem -count=5 -timeout 10m | tee bench-results.txt

fetch-test-grammars:
    go run ./cmd/fetch-grammars
```

---

## 9. Test Implementation Timeline

| Phase | Tests to Build | Depends On |
|-------|---------------|------------|
| **Phase 1** (Types) | `language_test.go`: table lookup, symbol metadata | Hand-compiled JSON grammar |
| **Phase 2** (Lexer) | `lexer_test.go`: token scanning, positions, included ranges | Lexer + JSON lex function |
| **Phase 3** (Parser) | Corpus test runner + JSON corpus, `parser_test.go`, `node_test.go`, `tree_test.go` | Core parser |
| **Phase 4** (Codegen) | Generate JSON grammar → compare output to hand-compiled | Code generator |
| **Phase 5** (Scanners) | Scanner edge case tests for Go and JS grammars | Scanner interface |
| **Phase 6** (Incremental) | Edit/reparse fuzz tests, changed range tests, consistency checks | Incremental parsing |
| **Phase 7** (Query) | `query_test.go` (port from Rust test suite) | Query system |
| **Phase 8** (Polish) | Full cross-check suite, benchmarks, CI pipeline | All components |

The corpus test runner (Section 2) should be built at the start of Phase 3 and
expanded as each grammar is supported. It is the single most important test
artifact — if the corpus tests pass for a grammar, the parser is very likely
correct for that language.
