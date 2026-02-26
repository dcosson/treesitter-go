# Package Restructure Design

## Goal

Move internal implementation types out of the root package into internal packages.
The root package should be a clean public API facade — only types and functions
that external consumers need.

## Current State

### Package Structure

```
treesitter/                    # root — mixed public API + heavy internals
  language.go                  # re-exports Language from language/
  lexer.go                     # re-exports Lexer from internal/lexer
  types.go                     # re-exports core types from internal/core
  tree.go                      # Tree, Node (public API)
  tree_cursor.go               # TreeCursor (public API)
  query.go                     # Query (public API)
  query_cursor.go              # QueryCursor (public API)
  subtree.go                   # Subtree, SubtreeArena, SubtreeHeapData, 50+ accessors (INTERNAL)
  subtree_edit.go              # EditSubtree (INTERNAL)
  stack.go                     # Stack, StackNode — DUPLICATE of internal/stack (INTERNAL)
  reusable_node.go             # ReusableNode (INTERNAL)
treesitter/parser/             # thin public facade wrapping internal/parser
treesitter/language/           # Language, ExternalScanner, ExternalScannerFactory
treesitter/lexer/              # Lexer, Input, StringInput (DUPLICATE of internal/lexer)
treesitter/internal/core/      # primitive types (Symbol, StateID, Length, etc.)
treesitter/internal/parser/    # GLR parser engine
treesitter/internal/stack/     # Graph-structured stack (GSS)
treesitter/internal/lexer/     # Lexer implementation (DUPLICATE of lexer/)
treesitter/internal/generate/  # Code generation (parser.c → language.go)
treesitter/internal/corpustest/ # Corpus test runner framework
treesitter/internal/difftest/  # Differential test utilities
treesitter/internal/testgrammars/<lang>/  # Generated grammar tables
treesitter/scanners/<lang>/    # Hand-ported external scanners
```

### Problems

1. **Root package exposes internals.** `Subtree`, `SubtreeArena`, `SubtreeHeapData`,
   `Stack`, `StackNode`, `ReusableNode`, and 50+ accessor functions (`GetSymbol`,
   `IsVisible`, `SummarizeChildren`, etc.) are all exported from the root package.
   External consumers don't need any of these.

2. **Duplicate Stack.** `stack.go` in the root package is a complete, independent
   GSS implementation. `internal/stack/stack.go` is a second implementation used by
   `internal/parser`. Only the internal one is used in production; the root one is
   only exercised by its own test file.

3. **Duplicate Lexer.** `lexer/lexer.go` and `internal/lexer/lexer.go` are identical
   files. The root package re-exports from `internal/lexer`, but `language/` and
   `internal/parser` import from `lexer/` (the public one).

4. **Inverted imports.** `internal/stack` and `internal/parser` both import the root
   package (as `ts`) to access `Subtree`, `SubtreeArena`, and accessor functions.
   This means internal packages depend on the root rather than the other way around.

### Import Graph (Current)

```
internal/core  (foundation, no internal imports)
    │
    ├──> lexer/           (public, imports core)
    │      │
    │      └──> internal/lexer  (DUPLICATE, imports core)
    │
    ├──> language/        (imports core + lexer/)
    │
    └──> ROOT PACKAGE     (imports core + lexer/ + language/)
           │
           ├──> internal/stack   (imports ROOT + core)  ← inverted
           │      │
           │      └──> internal/parser  (imports ROOT + internal/stack + lexer/)  ← inverted
           │             │
           │             └──> parser/  (public facade, imports ROOT + internal/parser)
           │
           ├──> internal/testgrammars/*  (import ROOT)
           └──> scanners/*  (import ROOT)
```

## Target State

### Package Structure

```
treesitter/                    # root — CLEAN PUBLIC API ONLY
  parser.go                    # Parser type alias from internal/parser
  language.go                  # Language type alias from language/
  lexer.go                     # Lexer, Input type aliases from lexer/
  types.go                     # core type aliases from internal/core
  tree.go                      # Tree, Node
  tree_cursor.go               # TreeCursor
  query.go                     # Query
  query_cursor.go              # QueryCursor
treesitter/parser/             # public facade (may merge into root)
treesitter/language/           # Language, ExternalScanner — unchanged
treesitter/lexer/              # Lexer, Input, StringInput — single copy
treesitter/internal/core/      # primitive types — unchanged
treesitter/internal/subtree/   # Subtree, SubtreeArena, all accessors
treesitter/internal/stack/     # GSS stack — single implementation
treesitter/internal/parser/    # GLR parser engine
treesitter/internal/lexer/     # REMOVED (use lexer/ directly)
treesitter/internal/generate/  # unchanged
treesitter/internal/corpustest/ # unchanged
treesitter/internal/difftest/  # unchanged
treesitter/internal/testgrammars/<lang>/  # unchanged
treesitter/scanners/<lang>/    # unchanged
```

### Import Graph (Target)

```
internal/core  (foundation)
    │
    ├──> lexer/              (single copy, imports core)
    │
    ├──> language/           (imports core + lexer/)
    │
    ├──> internal/subtree/   (imports core)  ← NEW
    │      │
    │      ├──> internal/stack/    (imports core + internal/subtree)
    │      │
    │      └──> internal/parser/   (imports core + subtree + stack + lexer/ + language/)
    │
    └──> ROOT PACKAGE        (imports everything above, re-exports public API)
           │
           ├──> parser/  (optional public facade, or merge into root)
           ├──> internal/testgrammars/*  (import internal/subtree + core)
           └──> scanners/*  (import lexer/ + core)
```

Key improvement: **no internal package imports the root package.** The dependency
flow is strictly bottom-up.

### Root Package Public API (Exhaustive)

After restructuring, the root package exports ONLY:

**Types (re-exported via type aliases):**
- `Language`, `ExternalScanner`, `ExternalScannerFactory` — from `language/`
- `Lexer`, `Input`, `StringInput` — from `lexer/`
- `Parser` — from `internal/parser` (or `parser/` facade)
- `Symbol`, `StateID`, `FieldID` — from `internal/core`
- `Point`, `Range`, `Length`, `InputEdit` — from `internal/core`
- `LexMode`, `SymbolMetadata` — from `internal/core` (needed by grammar tables)
- `ParseActionType`, `ParseActionEntry`, `TableEntry`, `FieldMapSlice`, `FieldMapEntry` — from `internal/core` (needed by grammar tables)

**Types (defined in root):**
- `Tree` — parse tree (holds root subtree, language, arenas)
- `Node` — lightweight tree navigation handle
- `TreeCursor` — efficient DFS traversal
- `Query` — compiled S-expression pattern
- `QueryCursor` — query execution engine
- `QueryMatch`, `QueryCapture`, `PredicateStep` — query result types
- `QueryError`, `QueryErrorType` — query compilation errors

**Constants:**
- `SymbolEnd`, `SymbolError`, `SymbolErrorRepeat`
- `ParseActionType*` constants
- Error cost constants (needed by grammar tables and scanners)

**Functions:**
- `NewParser() *Parser`
- `NewQuery(lang, source) (*Query, error)`
- `NewQueryCursor(query) *QueryCursor`
- `NewTreeCursor(node) TreeCursor`
- `LengthAdd`, `LengthSub` (convenience)

**NOT exported (moves to internal/subtree):**
- `Subtree`, `SubtreeArena`, `SubtreeHeapData`, `SubtreeFlags`, `SubtreeID`, `FirstLeaf`
- All `Get*`, `Is*`, `Set*` accessor functions
- `SummarizeChildren`, `NewLeafSubtree`, `NewNodeSubtree`, `ComputeSizeFromChildren`
- `EditSubtree`, `LengthSaturatingSub`
- `ReusableNode`
- `SubtreeZero`, `NewInlineSubtree`

**NOT exported (consolidated into internal/stack):**
- Root `Stack`, `StackNode`, `StackLink`, `StackHead`, `StackVersion`, `StackIterator`
- Root `NewStack`

### Grammar Tables and Scanners

The generated grammar files (`internal/testgrammars/<lang>/language.go`) and
scanner files (`scanners/<lang>/scanner.go`) currently import the root package for
types like `Symbol`, `ParseActionEntry`, `ExternalScanner`, `Lexer`, etc.

After restructuring, they will need to import from the appropriate packages:
- Grammar tables: `internal/core` for `Symbol`, `ParseActionEntry`, `TableEntry`, etc.
- Scanners: `lexer/` for `Lexer`, `internal/core` for `Symbol`

The `tsgo-generate` tool will need to be updated to emit the new import paths.

### Tree and Node

`Tree` and `Node` remain in the root package. They currently reference `Subtree`
and `SubtreeArena` directly. After the move:

- `Tree` holds a `subtree.Subtree` and `*subtree.SubtreeArena` (unexported fields)
- `Node` holds a `subtree.Subtree` (unexported field)
- All internal access goes through `internal/subtree` package functions
- Public methods on `Tree` and `Node` remain unchanged

### Query

`Query` and `QueryCursor` currently use `Subtree` accessors directly for tree
matching. After the move, they'll import `internal/subtree` for these operations.
Their public API is unchanged.

## Migration Steps

The migration should be done incrementally, with tests passing at every step.

### Phase 1: Create internal/subtree

1. Create `internal/subtree/` package with all types and functions from `subtree.go`
   and `subtree_edit.go`
2. Update `internal/stack` to import `internal/subtree` instead of root package
3. Update `internal/parser` to import `internal/subtree` instead of root package
4. Root package re-exports subtree types temporarily (type aliases) to avoid
   breaking grammar tables and scanners
5. Verify all tests pass

### Phase 2: Remove duplicate Stack

1. Delete `stack.go` from root package
2. Move `stack_test.go` tests into `internal/stack/` (or delete if redundant)
3. Verify all tests pass

### Phase 3: Remove duplicate Lexer

1. Delete `internal/lexer/` (the duplicate)
2. Update any imports from `internal/lexer` to `lexer/`
3. Root package already re-exports from `lexer/` — no change needed
4. Verify all tests pass

### Phase 4: Update grammar tables and scanners

1. Update `tsgo-generate` to emit imports from `internal/core` and `internal/subtree`
   instead of root package
2. Regenerate all grammar tables
3. Update scanner imports to use `lexer/` and `internal/core` directly
4. Verify all tests pass

### Phase 5: Clean root package

1. Remove subtree type aliases from root package (no longer needed)
2. Move `ReusableNode` into `internal/parser` (it's only used there)
3. Verify the root package exports only the target public API
4. Verify all tests pass

### Phase 6: Move test files

1. Move internal-focused tests (stack_test.go, subtree_test.go) into their packages
2. Clean up untracked debug test files (~14 files)
3. Keep public API tests (api_test.go, tree_test.go, query_test.go) in root
4. Keep integration tests (corpus, regression, benchmark, fuzz) in root

---

## Testing Overview

### Test Types

| Test | Command | What it Tests | External Dependencies |
|------|---------|---------------|----------------------|
| **Unit tests** | `make test` | Core runtime: parser, lexer, stack, subtree, API, scanners | None |
| **Corpus tests** | `make test-corpus` | Grammar test suites — 1619 cases across 15 languages, input + expected S-expression | Grammars (`make fetch-test-grammars`) |
| **Regression tests** | `make test-regression` | Curated inputs that previously caused bugs | None (fixtures in repo) |
| **Differential tests** | `make diff-test` | Small sample inputs, Go vs C tree-sitter CLI output | C CLI (`make deps`) |
| **Realworld diff tests** | `make test-realworld-diff` | Real-world OSS source files, Go vs C CLI output | C CLI + fetched files (`make fetch-realworld`) |
| **Scanner trace tests** | `make test-scanner-traces` | External scanner parity — replays C scanner calls against Go scanners | Grammars + traces (committed) |
| **Benchmarks** | `make bench` | Parse throughput (bytes/sec) at multiple sizes for all languages | Optional: C CLI for comparison |
| **Fuzz tests** | `go test -fuzz=FuzzParse<Lang>` | Crash-finding — parser never panics on arbitrary input | None |
| **Grammar batch tests** | `go test -run TestGrammarBatch` | Hand-written integration tests for specific language constructs | Grammars |

**Rename: "corpora diff" → "realworld diff"**

The current "corpora diff tests" (fetching real-world source files from GitHub and
comparing Go vs C output) are confusingly named alongside "corpus tests" (grammar
test suite fixtures). We rename:

| Current | New |
|---------|-----|
| `make test-corpora-diff` | `make test-realworld-diff` |
| `make fetch-corpora` | `make fetch-realworld` |
| `testdata/corpora/` | `testdata/realworld/` |
| `testdata/corpora-manifest.json` | `testdata/realworld-manifest.json` |
| `TestDifferentialCorpora` | `TestDifferentialRealworld` |
| `corpora_diff_test.go` | `realworld_diff_test.go` |

### Test Data Layout

```
testdata/
  grammars/                    # Fetched grammar repos (make fetch-test-grammars)
    tree-sitter-<lang>/
      test/corpus/*.txt        # Grammar test fixtures (input + expected S-expression)
      src/parser.c             # C parse tables (used by tsgo-generate)
  grammars.json                # Grammar version pins
  realworld/                   # Fetched real-world source files (make fetch-realworld)
    <lang>/<project>/<files>
  realworld-manifest.json      # Real-world file version pins
  corpus-overrides.json        # Per-test expected output overrides (currently empty)
  regressions/                 # Curated regression test fixtures
    <lang>/<name>.input + <name>.expected
  scanner-traces/              # Committed C scanner call traces
    <lang>.jsonl
  error-recovery/              # Error recovery test inputs
    <lang>/<files>
  bench/                       # Generated benchmark fixtures (optional)
```

### Running Tests

```bash
# Full test suite (no external deps needed)
make test              # Unit + grammar batch tests
make test-corpus       # Grammar corpus tests (1619 cases)
make test-regression   # Regression tests

# Per-language corpus
make test-corpus-json
go test -run "TestCorpusPerl/Double_dollar" -v -count=1 .

# With C CLI (make deps first)
make diff-test             # Small differential sample
make test-realworld-diff   # Real-world source files

# Benchmarks
make bench                 # Go only
make bench TS_CLI=tree-sitter  # Go + C comparison

# Fuzz
go test -fuzz=FuzzParseGo -fuzztime=60s .

# Scanner traces
make test-scanner-traces
```

---

## Adding a New Language

### Prerequisites

- The grammar repo must have `src/parser.c` (generated by `tree-sitter generate`)
- You need `tsgo-generate` built: `go build ./cmd/tsgo-generate`

### Steps

**1. Register the grammar**

Add to `testdata/grammars.json`:
```json
{"name": "newlang", "repo": "tree-sitter/tree-sitter-newlang", "version": "v1.0.0"}
```

**2. Fetch**
```bash
make fetch-test-grammars
```

**3. Generate Go parse tables**
```bash
tsgo-generate \
  -parser testdata/grammars/tree-sitter-newlang/src/parser.c \
  -package newlanggrammar \
  -output internal/testgrammars/newlang/language.go
```

**4. Port external scanner** (if `src/scanner.c` exists)

Create `scanners/newlang/scanner.go` implementing `ExternalScanner`:
```go
package newlangscanner

import ts "github.com/treesitter-go/treesitter"

type Scanner struct { /* state */ }

func New() ts.ExternalScanner { return &Scanner{} }

func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool { /* ... */ }
func (s *Scanner) Serialize() []byte { /* ... */ }
func (s *Scanner) Deserialize(buf []byte) { /* ... */ }
```

Add unit tests at `scanners/newlang/scanner_test.go`.

**5. Wire into corpus tests**

In `corpus_languages_test.go`:
```go
func TestCorpusNewLang(t *testing.T) {
    lang := newlanggrammar.NewLangLanguage()
    lang.NewExternalScanner = newlangscanner.New // if applicable
    runCorpusForLanguage(t, "tree-sitter-newlang", lang)
}
```

**6. Wire into other test suites**

- `regression_test.go` — add `TestRegressionNewLang`, create `testdata/regressions/newlang/`
- `fuzz_test.go` — add `FuzzParseNewLang` target
- `grammar_batchN_test.go` — add hand-written integration tests for key constructs
- `corpora_diff_test.go` — add to `allCorporaLanguages()` with file extensions and scope
- `testdata/realworld-manifest.json` — add 2+ real-world projects with representative files
- `benchmark_test.go` — add to `benchLanguages()` with a synthetic input generator
- `scanner_trace_test.go` — add to `scannerLanguages()` (if external scanner)

**7. Generate scanner traces** (if external scanner)
```bash
make generate-scanner-traces
```

**8. Run full test suite**
```bash
make test && make test-corpus && make test-regression
```

### Checklist for New Language

- [ ] Entry in `testdata/grammars.json` with pinned version
- [ ] Generated `internal/testgrammars/<lang>/language.go`
- [ ] External scanner ported (if applicable) with unit tests
- [ ] `TestCorpus<Lang>` in `corpus_languages_test.go`
- [ ] `TestRegression<Lang>` in `regression_test.go`
- [ ] `FuzzParse<Lang>` in `fuzz_test.go`
- [ ] Integration tests in `grammar_batch<N>_test.go`
- [ ] Real-world files in `testdata/realworld-manifest.json`
- [ ] Benchmark entry in `benchmark_test.go`
- [ ] Scanner traces generated (if external scanner)
- [ ] All tests pass
