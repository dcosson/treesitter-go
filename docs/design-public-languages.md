# Design: Export Public Language Packages

## Problem

The 15 language grammars live under `internal/testgrammars/`, making them
inaccessible to external callers. Users of this library can't parse Go,
Python, JavaScript, etc. without copying generated code or building their own
grammar bindings. Scanners are public (`scanners/<lang>/`) but users never
call them directly — they're an internal implementation detail of parsing.

## Approach

Three-layer architecture:

```
languages/<lang>/language.go      # PUBLIC: Language() → *ts.Language (tiny shim)
internal/grammars/<lang>/...      # INTERNAL: generated parse tables, symbols, lex fns
internal/scanners/<lang>/...      # INTERNAL: scanner implementations
```

### Public API (per language)

```go
package python

import (
    grammar "github.com/treesitter-go/treesitter/internal/grammars/python"
    scanner "github.com/treesitter-go/treesitter/internal/scanners/python"
    ts "github.com/treesitter-go/treesitter"
)

// Language returns a fully configured Python language ready for parsing.
func Language() *ts.Language {
    l := grammar.PythonLanguage()
    l.NewExternalScanner = scanner.New
    return l
}
```

### User-facing usage

```go
import (
    "github.com/treesitter-go/treesitter/languages/python"
    ts "github.com/treesitter-go/treesitter"
)

lang := python.Language()
p := ts.NewParser()
p.SetLanguage(lang)
tree := p.ParseString(ctx, input)
```

## Package naming

Normalize the four inconsistently-named packages:

| Current (`internal/testgrammars/`) | New (`internal/grammars/`) | Public (`languages/`) |
|------------------------------------|----------------------------|-----------------------|
| `cgrammar/` | `c/` | `c/` |
| `cppgrammar/` | `cpp/` | `cpp/` |
| `rustgrammar/` | `rust/` | `rust/` |
| `tsxgrammar/` | `tsx/` | `tsx/` |

The other 11 packages keep their current names (golang, python, javascript,
typescript, java, ruby, bash, css, html, lua, perl).

Note: `c` (lowercase) does NOT conflict with cgo's `import "C"` (uppercase).

## What moves where

### `internal/testgrammars/` → `internal/grammars/`

All 15 language subdirectories move. The generated `language.go` files are
unchanged except package name for the 4 renamed packages. Internal callers
(tests, CLI tools) update import paths.

### `internal/testgrammars/json_language.go` → `internal/grammars/json/language.go`

JSON is currently at the root of testgrammars (not in a subdirectory). It
moves into its own `json/` subdirectory like every other language.

The custom `jsonLexFn` currently in `e2etest/realworld_diff_test.go` moves
into `languages/json/` so the public shim can wire it.

### `internal/testgrammars/extscanner_language.go` → stays internal

This is a test-only grammar, not a real language. It moves to
`internal/grammars/extscanner_language.go` (stays at root of grammars package).

### `scanners/` → `internal/scanners/`

All 11 scanner packages move internal. Users never call scanners directly;
they're wired automatically by the `languages/` shim.

**Breaking change**: `scanners/<lang>/` is currently a public import path.
Any downstream callers importing scanner packages directly will break. The
migration path is to use `languages/<lang>.Language()` instead, which
auto-wires the scanner. This will be noted in the release/changelog.

### New: `languages/<lang>/language.go`

15 thin shim files, one per language. Each exports exactly one function:
`Language() *ts.Language`.

For the 4 languages without scanners (json, go, c, java), the shim just
calls the grammar constructor. For the 11 with scanners, it also wires
`NewExternalScanner`.

## Files to update

### Import path changes (~22 files, ~137 import statements)

All of these change `internal/testgrammars` → `internal/grammars` and
`scanners/` → `internal/scanners/`:

- `e2etest/corpus_languages_test.go`
- `e2etest/corpus_test.go`
- `e2etest/benchmark_test.go`
- `e2etest/fuzz_test.go`
- `e2etest/regression_test.go`
- `e2etest/realworld_diff_test.go`
- `e2etest/scanner_trace_test.go`
- `e2etest/error_recovery_test.go`
- `e2etest/grammar_batch1_test.go` through `grammar_batch4_test.go`
- `e2etest/api_test.go`
- `e2etest/parser_integration_test.go`
- `e2etest/scripting_integration_test.go`
- `e2etest/query_test.go`
- `internal/parser/parser_test.go`
- `internal/difftest/difftest_test.go`
- `cmd/tsgo-parse/main.go`
- `cmd/debug_parse/main.go`
- `cmd/generate-regression-expected/main.go`

### Code generation pipeline

- `cmd/tsgo-generate/main.go` — update default output path conventions
- `internal/generate/codegen.go` — update package naming logic for the 4
  renamed packages
- `Makefile` — update any grammar-related targets
- `scripts/generate-scanner-traces.sh` — update paths if referenced

## Execution plan

No parser-runtime algorithm changes. Behavioral surface changes:
- `scanners/` moves internal (breaking change for direct scanner importers)
- `languages/<lang>.Language()` auto-wires scanners (new convenience behavior)
- JSON custom lex function relocates from test code into `languages/json/`

Order:

1. **Move grammars**: `internal/testgrammars/<lang>/` → `internal/grammars/<lang>/`
   - Rename 4 packages (cgrammar→c, cppgrammar→cpp, rustgrammar→rust, tsxgrammar→tsx)
   - Move json from root to `internal/grammars/json/`
   - Keep extscanner at `internal/grammars/extscanner_language.go`

2. **Move scanners**: `scanners/<lang>/` → `internal/scanners/<lang>/`

3. **Create public shims**: `languages/<lang>/language.go` for all 15 languages

4. **Update all imports**: mechanical find-and-replace across ~22 files

5. **Update generation pipeline**: tsgo-generate output paths and package names

6. **Verify**: `make test && make test-corpus && make test-realworld-diff`

## Testing

- `make test` — unit + integration
- `make test-corpus` — all 15 languages corpus tests
- `make test-realworld-diff` — all 15 languages realworld differential
- Verify `TestManifestCorpusCoverage` and `TestManifestBenchCoverage` still pass
  (these catch missing wiring)
