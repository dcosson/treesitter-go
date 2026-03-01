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
treesitter/                    # root — SINGLE FILE, pure type aliases + constructors
  treesitter.go                # ALL public API: type aliases, constructors, constants
treesitter/parser/             # REMOVED (merged into root as alias)
treesitter/language/           # Language, ExternalScanner — unchanged
treesitter/lexer/              # Lexer, Input, StringInput — single copy
treesitter/internal/core/      # primitive types — unchanged
treesitter/internal/subtree/   # Subtree, SubtreeArena, all accessors  ← NEW
treesitter/internal/tree/      # Tree, Node, TreeCursor, S-expression  ← NEW
treesitter/internal/query/     # Query, QueryCursor, compiler          ← NEW
treesitter/internal/stack/     # GSS stack — single implementation (absorb root's stack.go)
treesitter/internal/parser/    # GLR parser engine
treesitter/internal/lexer/     # REMOVED (use lexer/ directly)
treesitter/internal/generate/  # unchanged
treesitter/internal/corpustest/ # unchanged
treesitter/internal/difftest/  # unchanged
treesitter/internal/testgrammars/<lang>/  # unchanged
treesitter/scanners/<lang>/    # unchanged
```

The root package is a **single file** (`treesitter.go`) containing only type
aliases, constructor wrappers, and constants. Zero logic.

### Import Graph (Target)

```
internal/core  (foundation — no internal imports)
    │
    ├──> lexer/              (single copy, imports core)
    │
    ├──> language/           (imports core + lexer/)
    │
    ├──> internal/subtree/   (imports core)
    │      │
    │      ├──> internal/stack/    (imports core + subtree)
    │      │
    │      ├──> internal/tree/     (imports core + subtree + language/)
    │      │
    │      ├──> internal/query/    (imports core + subtree + tree + language/)
    │      │
    │      └──> internal/parser/   (imports core + subtree + stack + tree + lexer/ + language/)
    │
    └──> ROOT (treesitter.go) — imports ALL above, re-exports as aliases
           │
           ├──> internal/testgrammars/*  (import core + subtree)
           └──> scanners/*  (import lexer/ + core)
```

Key improvement: **no internal package imports the root package.** The dependency
flow is strictly bottom-up. The root is a pure leaf in the import graph.

### Root Package: treesitter.go (Exhaustive)

The single file contains ONLY type aliases, constructor wrappers, and constants:

```go
package treesitter

import (
    "github.com/treesitter-go/treesitter/internal/core"
    "github.com/treesitter-go/treesitter/internal/parser"
    "github.com/treesitter-go/treesitter/internal/query"
    "github.com/treesitter-go/treesitter/internal/tree"
    "github.com/treesitter-go/treesitter/language"
    "github.com/treesitter-go/treesitter/lexer"
)

// --- Language & Lexer ---
type Language = language.Language
type ExternalScanner = language.ExternalScanner
type ExternalScannerFactory = language.ExternalScannerFactory
type Lexer = lexer.Lexer
type Input = lexer.Input
type StringInput = lexer.StringInput

// --- Parser ---
type Parser = parser.Parser
func NewParser() *Parser { return parser.NewParser() }

// --- Tree & Node ---
type Tree = tree.Tree
type Node = tree.Node
type TreeCursor = tree.TreeCursor
func NewTreeCursor(n Node) TreeCursor { return tree.NewTreeCursor(n) }

// --- Query ---
type Query = query.Query
type QueryCursor = query.QueryCursor
type QueryMatch = query.QueryMatch
type QueryCapture = query.QueryCapture
type PredicateStep = query.PredicateStep
type PredicateStepType = query.PredicateStepType
type QueryError = query.QueryError
type QueryErrorType = query.QueryErrorType
func NewQuery(lang *Language, src string) (*Query, error) { return query.NewQuery(lang, src) }
func NewQueryCursor(q *Query) *QueryCursor { return query.NewQueryCursor(q) }

// --- Core types ---
type Symbol = core.Symbol
type StateID = core.StateID
type FieldID = core.FieldID
type Point = core.Point
type Range = core.Range
type Length = core.Length
type InputEdit = core.InputEdit
type LexMode = core.LexMode
type SymbolMetadata = core.SymbolMetadata
type ParseActionType = core.ParseActionType
type ParseActionEntry = core.ParseActionEntry
type TableEntry = core.TableEntry
type FieldMapSlice = core.FieldMapSlice
type FieldMapEntry = core.FieldMapEntry

// --- Constants ---
const (
    SymbolEnd         = core.SymbolEnd
    SymbolError       = core.SymbolError
    SymbolErrorRepeat = core.SymbolErrorRepeat
    // ... ParseActionType*, error cost constants, etc.
)

// --- Convenience ---
var LengthZero = core.LengthZero
func LengthAdd(a, b Length) Length { return core.LengthAdd(a, b) }
func LengthSub(a, b Length) Length { return core.LengthSub(a, b) }
```

### internal/tree

Holds the implementations currently in `tree.go`, `tree_cursor.go`:
- `Tree` struct (root subtree, language, arenas)
- `Node` struct (lightweight navigation handle) with all methods:
  `String()`, `Child()`, `NamedChild()`, `ChildByFieldName()`, `Parent()`,
  `StartByte()`, `EndByte()`, `Type()`, `Symbol()`, `IsNamed()`, etc.
- `TreeCursor` struct with `GotoFirstChild()`, `GotoNextSibling()`, `GotoParent()`
- S-expression rendering (`writeSExprSubtree`, MISSING/UNEXPECTED handling)

Imports: `internal/core`, `internal/subtree`, `language/`

### internal/query

Holds the implementations currently in `query.go`, `query_cursor.go`:
- `Query` struct — compiled query with recursive descent parser
- `QueryCursor` struct — query execution, pattern matching
- `QueryMatch`, `QueryCapture`, `PredicateStep`, `QueryError` types

Imports: `internal/core`, `internal/subtree`, `internal/tree`, `language/`

### Grammar Tables and Scanners

The generated grammar files (`internal/testgrammars/<lang>/language.go`) and
scanner files (`scanners/<lang>/scanner.go`) currently import the root package for
types like `Symbol`, `ParseActionEntry`, `ExternalScanner`, `Lexer`, etc.

After restructuring, they import from the appropriate packages directly:
- Grammar tables: `internal/core` for `Symbol`, `ParseActionEntry`, `TableEntry`, etc.
- Scanners: `lexer/` for `Lexer`, `internal/core` for `Symbol`,
  `language/` for `ExternalScanner`

The `tsgo-generate` tool will be updated to emit the new import paths.

## Migration Steps

The migration should be done incrementally, with tests passing at every step.

### Phase 1: Create internal/subtree

1. Create `internal/subtree/` package with all types and functions from `subtree.go`
   and `subtree_edit.go`
2. Update `internal/stack` to import `internal/subtree` instead of root package
3. Update `internal/parser` to import `internal/subtree` instead of root package
4. Root package re-exports subtree types temporarily (type aliases) to avoid
   breaking other packages
5. Verify all tests pass

### Phase 2: Remove duplicate Stack and Lexer

1. Delete `stack.go` from root package (duplicate of `internal/stack`)
2. Move `stack_test.go` tests into `internal/stack/` (or delete if redundant)
3. Delete `internal/lexer/` (duplicate of `lexer/`)
4. Update any imports from `internal/lexer` to `lexer/`
5. Verify all tests pass

### Phase 3: Create internal/tree

1. Move `tree.go` and `tree_cursor.go` logic into `internal/tree/`
2. Root package replaces implementations with type aliases to `internal/tree`
3. Verify all tests pass

### Phase 4: Create internal/query

1. Move `query.go` and `query_cursor.go` logic into `internal/query/`
2. Root package replaces implementations with type aliases to `internal/query`
3. Verify all tests pass

### Phase 5: Update grammar tables and scanners

1. Update `tsgo-generate` to emit imports from `internal/core` instead of root
2. Regenerate all 15 grammar tables
3. Update scanner imports to use `lexer/` and `language/` directly
4. Verify all tests pass

### Phase 6: Consolidate root into treesitter.go

1. Remove all root package files except `treesitter.go`
2. Collapse remaining type aliases and constructors into the single file
3. Move `ReusableNode` into `internal/parser` (only used there)
4. Delete `parser/` public facade (merged into root alias)
5. Verify the root package is a single file with zero logic
6. Verify all tests pass

### Phase 7: Move and clean up test files

1. Move internal-focused tests into their packages:
   - `subtree_test.go` → `internal/subtree/`
   - `stack_test.go` → `internal/stack/`
   - `tree_test.go`, `tree_cursor_test.go` → `internal/tree/`
   - `query_test.go` → `internal/query/`
2. Delete ~14 untracked debug test files
3. Keep in root: `api_test.go` (public API integration tests)
4. Keep in root: corpus, regression, benchmark, fuzz, grammar batch tests
   (these are cross-cutting integration tests that exercise the full stack)

### Phase 8: Rename corpora → realworld

1. Rename `testdata/corpora/` → `testdata/realworld/`
2. Rename `testdata/corpora-manifest.json` → `testdata/realworld-manifest.json`
3. Rename `corpora_diff_test.go` → `realworld_diff_test.go`
4. Update `TestDifferentialCorpora` → `TestDifferentialRealworld`
5. Update Makefile targets: `test-corpora-diff` → `test-realworld-diff`,
   `fetch-corpora` → `fetch-realworld`
6. Update README
7. Verify all tests pass

---

## Testing Overview

### `make test` — Primary Test Suite

`make test` runs `go test -skip 'TestCorpus|TestDifferential' ./...`. This is the
main development command. It runs everything except corpus tests (which need fetched
grammar repos) and differential tests (which need the C CLI). It includes:

| Included in `make test` | Standalone Command | What it Tests |
|:-:|------|-------------|
| Y | `make test-regression` | Curated inputs that previously caused bugs (hangs, panics, wrong output). Fixtures in `testdata/regressions/<lang>/` |
| Y | `make test-scanner-traces` | External scanner parity — replays recorded C scanner calls against Go scanners. Traces committed in `testdata/scanner-traces/` |
| Y | `go test -run TestGrammarBatch` | Hand-written integration tests for specific language constructs across all 15 languages |
| Y | `go test -run TestApi` | Public API tests — Node, Tree, TreeCursor |
| Y | `go test -run TestErrorRecovery` | Malformed input handling — no panics, ERROR nodes produced |
| Y | (internal packages) | Unit tests for parser, lexer, stack, subtree, corpustest framework, scanners |

### Additional Test Suites (not in `make test`)

These require fetched data or external tools:

| Command | What it Tests | Setup Required |
|---------|---------------|----------------|
| `make test-corpus` | Grammar test suites — 1619 cases across 15 languages. Each case has input + expected S-expression from upstream grammar repos. | `make fetch-test-grammars` |
| `make test-diff` | Small sample inputs compared Go vs C tree-sitter CLI output | `make deps` (installs C CLI) |
| `make test-realworld-diff` | Real-world OSS source files (kubernetes, flask, rails, etc.) compared Go vs C CLI | `make deps` + `make fetch-realworld` |
| `make bench` | Parse throughput (bytes/sec) at multiple sizes for all 15 languages. Optional C CLI comparison. | Optional: `make deps` for C comparison |
| `make fuzz` | Run all fuzz targets for 30s each (parse + scanner roundtrip). Finds crashes/panics. | None |
| `make fuzz FUZZ_TIME=5m` | Same, with custom duration per target | None |
| `go test -fuzz=FuzzParseGo -fuzztime=60s .` | Single-language fuzz | None |

### Exhaustive Test Run (CI / Pre-Release)

To run every test:

```bash
# 1. Setup (one-time)
make deps                  # Install C CLI
make fetch-test-grammars   # Fetch grammar repos
make fetch-realworld       # Fetch real-world source files

# 2. Run all tests
make test                  # Unit + regression + scanner traces + grammar batch + ...
make test-corpus           # Grammar corpus tests (1619 cases)
make test-diff             # Go vs C differential
make test-realworld-diff   # Real-world file differential
make fuzz                  # Fuzz all targets (30s each, ~10 min total)
make bench                 # Benchmarks (optional, not correctness)
```

### Rename: "corpora diff" → "realworld diff"

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
grammars.json                  # Grammar version pins (supported languages)
build/
  grammars/                    # Fetched grammar repos (make fetch-test-grammars)
    tree-sitter-<lang>/
      test/corpus/*.txt        # Grammar test fixtures (input + expected S-expression)
      src/parser.c             # C parse tables (used by tsgo-generate)
testdata/
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
make test-diff             # Small differential sample
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

Add to `grammars.json`:
```json
{"name": "newlang", "repo": "tree-sitter/tree-sitter-newlang", "version": "v1.0.0", "ext": ".nl", "scanner": true}
```

**2. Fetch**
```bash
make fetch-test-grammars
```

**3. Generate Go parse tables**
```bash
tsgo-generate \
  -parser build/grammars/tree-sitter-newlang/src/parser.c \
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

- [ ] Entry in `grammars.json` with pinned version
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
