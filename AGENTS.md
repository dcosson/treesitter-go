# Agent Guidelines

## Building

Always use `make build` to compile the project. This builds all CLI tools into the `build/bin/` directory.

```bash
make build    # Builds all cmd/ binaries into build/bin/
```

Do **not** run `go build ./...` directly — it only checks compilation and does not produce output binaries.

## Testing

### Use Makefile targets for verification

Always use the Makefile test targets rather than running `go test` directly, unless you are targeting a single specific test case during development. The Makefile targets have the correct package paths, flags, and timeouts.

| What you want | Command |
|---------------|---------|
| Unit + integration tests (no corpus/diff) | `make test` |
| Corpus tests (all 15 languages) | `make test-corpus` |
| Regression tests only | `make test-regression` |
| Realworld differential (needs C CLI) | `make test-realworld-diff` |
| Differential tests (needs C CLI) | `make test-diff` |
| Benchmarks (Go only) | `make bench-self` |
| Benchmarks (Go vs C, needs CLI) | `make bench-compare` |
| Scanner trace tests | `make test-scanner-traces` |
| Fuzz testing | `make fuzz` |
| Lint & format check | `make check` |
| Lint check (CI, no auto-fix) | `make check-nofix` |

### Single-language filtering

All multi-language targets accept `GRAMMAR=<name>` to run for one language only. The value is validated against `grammars.json`:

```bash
make test-corpus GRAMMAR=go
make test-regression GRAMMAR=python
make bench-self GRAMMAR=json
make test-scanner-traces GRAMMAR=bash
```

See the **Testing** section in `README.md` for full details on each test type, what it covers, where the code lives, and what setup is required.

### Running individual tests during development

When working on a specific feature, you can run targeted tests directly:

```bash
# Run a single corpus test case
go test -run TestCorpusGo/Function_declarations -v -count=1 ./e2etest/

# Run a unit test in an internal package
go test -run TestSubtreeLeaf -v ./internal/subtree/

# Run a specific scanner test
go test -run TestPythonIndent -v ./internal/scanners/python/
```

### Important notes

- Tests are slow. Some require compiling all 15 language grammars and parsing large datasets. Scope your test runs carefully.
- Output test results into temporary files so you can parse them without re-running.
- Before closing out any task, run the relevant `make` target to verify — don't rely on individual `go test` runs.

### Test data setup

Some tests require fetched data:

```bash
make fetch-test-grammars   # Needed for corpus tests
make deps                  # Installs tree-sitter CLI (needed for diff tests)
make fetch-realworld       # Needed for realworld diff tests
```

## Code organization

- **Root package** (`treesitter.go`) — pure facade with type aliases and constructor wrappers. Zero logic. Do not add implementation here.
- **`internal/`** — all implementation: parser, lexer, stack, subtree, tree, query, core types, code generation, test frameworks.
- **`languages/`** — public language packages. Each has a `Language()` function that returns a fully configured `*ts.Language` ready for parsing.
- **`internal/grammars/`** — generated grammar files (action tables, lex functions). Produced by `tsgo-generate` from C parser sources.
- **`internal/scanners/`** — hand-ported external scanner implementations per language.
- **`language/`** and **`lexer/`** — public packages for Language/ExternalScanner and Lexer/Input types.
- **`e2etest/`** — end-to-end and integration tests that exercise the full stack.
- **`cmd/`** — CLI tools (built into `build/bin/` via `make build`).

See `README.md` for the full architecture diagram and package descriptions.

## Supported languages

**`grammars.json`** in the project root is the source of truth for all supported languages. The Makefile, shell scripts, and CLI tools read from this manifest — do not maintain separate hardcoded language lists.

## Adding a new grammar

Follow the detailed steps in the README's **"Adding a Grammar"** section. Summary:

1. **Manifest**: Add entry to `grammars.json` (`name`, `repo`, `version`, `ext`, `scanner`)
2. **Fetch**: `make fetch-test-grammars`
3. **Generate**: `build/bin/tsgo-generate -parser build/grammars/tree-sitter-<lang>/src/parser.c -package <lang> -output internal/grammars/<lang>/language.go`
4. **Scanner**: If `scanner.c` exists, port to Go in `internal/scanners/<lang>/scanner.go` with unit tests
5. **Public shim**: Create `languages/<lang>/language.go` that wires grammar + scanner
6. **Test wiring**: Add to all test suites:
   - `e2etest/corpus_languages_test.go` — `TestCorpus<Lang>` function
   - `e2etest/benchmark_test.go` — entry in `benchLanguages()`
   - `e2etest/manifest_coverage_test.go` — entry in `corpusLanguages` map
   - `e2etest/regression_test.go` — `TestRegression<Lang>`
   - `e2etest/fuzz_test.go` — `FuzzParse<Lang>`
   - `e2etest/grammar_batchN_test.go` — integration tests for key constructs
   - `e2etest/scanner_trace_test.go` — entry in `scannerLanguages()` (if scanner)
   - `testdata/realworld-manifest.json` — 2+ real-world projects
7. **Scanner traces**: `make generate-scanner-traces` (if scanner)
8. **README**: Update any language count references
9. **Verify**: `make test && make test-corpus` — manifest coverage tests will catch missing wiring
