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
| Differential tests (needs C CLI) | `make diff-test` |
| Benchmarks (Go only) | `make bench-self` |
| Benchmarks (Go vs C, needs CLI) | `make bench-compare` |
| Fuzz testing | `make fuzz` |

See the **Testing** section in `README.md` for full details on each test type, what it covers, where the code lives, and what setup is required.

### Running individual tests during development

When working on a specific feature, you can run targeted tests directly:

```bash
# Run a single corpus test case
go test -run TestCorpusGo/Function_declarations -v -count=1 ./e2etest/

# Run a unit test in an internal package
go test -run TestSubtreeLeaf -v ./internal/subtree/

# Run a specific scanner test
go test -run TestPythonIndent -v ./scanners/python/
```

### Important notes

- Tests are slow. Some require compiling all 15 language grammars and parsing large datasets. Scope your test runs carefully.
- Output test results into temporary files so you can parse them without re-running.
- Before closing out any task, run the relevant `make` target to verify — don't rely on individual `go test` runs.
- Coordinate full test suite runs with the scheduler or reviewer agents.

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
- **`language/`** and **`lexer/`** — public packages for Language/ExternalScanner and Lexer/Input types.
- **`scanners/`** — hand-ported external scanner implementations per language.
- **`e2etest/`** — end-to-end and integration tests that exercise the full stack.
- **`cmd/`** — CLI tools (built into `build/bin/` via `make build`).

See `README.md` for the full architecture diagram and package descriptions.
