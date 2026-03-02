# Project Guidelines

## Building

Always use `make build` to build the project. This compiles all CLI tools into the `build/bin/` directory. Do not run `go build ./...` directly — it only checks compilation and does not produce binaries.

```bash
make build    # Builds all cmd/ binaries into build/bin/
```

## Testing

Tests in this project are slow — some require compiling all 15 language grammars and parsing large datasets. Be deliberate about which tests you run.

### Quick reference

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
| Lint & format | `make check` |
| Lint check (CI, no auto-fix) | `make check-nofix` |

All multi-language targets support a `GRAMMAR=<name>` filter to run for a single language only. The value must match a name in `grammars.json`. Examples:

```bash
make test-corpus GRAMMAR=go
make test-regression GRAMMAR=python
make bench-self GRAMMAR=json
```

See the **Testing** section in `README.md` for full details on each test type, what it covers, where the code lives, and what setup is required.

### When to use make vs direct go test

- **Use `make` targets** for verification before committing, closing out work, or running a full test suite. The Makefile targets have the correct flags, timeouts, and package paths.
- **Use `go test` directly** only when working on a specific feature and you need to run a single targeted test case, e.g.:
  ```bash
  go test -run TestCorpusGo/Function_declarations -v -count=1 ./e2etest/
  go test -run TestSubtreeLeaf -v ./internal/subtree/
  ```
- **Scope test runs** to just the tests you need. Output results into temporary files so you can parse them instead of re-running tests.
- **Coordinate full test runs** with the scheduler or reviewer agents, who should use the Makefile commands to ensure clean runs before closing out work.

### Test data setup

Some tests require fetched data. Run these once before testing:

```bash
make fetch-grammars   # Needed for corpus tests
make deps                  # Installs tree-sitter CLI (needed for diff tests)
make fetch-realworld       # Needed for realworld diff tests
```

## Code organization

The root package (`treesitter.go`) is a pure facade — type aliases and constructor wrappers only, zero logic. All implementation lives in internal packages. See `README.md` Architecture section for the full package layout.

## Supported languages

**`grammars.json`** in the project root is the source of truth for all supported languages. It defines each grammar's name, upstream repo, pinned version, file extension, and whether it has an external scanner. The Makefile, shell scripts, and CLI tools all read from this manifest.

## Adding a new grammar

Follow the steps in the README's "Adding a Grammar" section. Key points:

1. Add entry to `grammars.json` with all fields (`name`, `repo`, `version`, `ext`, and `scanner` if applicable)
2. `make fetch-grammars` to clone the repo into `build/grammars/`
3. Run `tsgo-generate` to produce `internal/grammars/<lang>/language.go`
4. Port the external scanner to Go in `internal/scanners/<lang>/` if one exists
5. Create public shim in `languages/<lang>/language.go` that wires grammar + scanner
6. Wire into all test suites — corpus, benchmarks, regression, fuzz, grammar batch, scanner traces, manifest coverage map
7. Update the README if the supported languages table or count references change
8. Run `make test && make test-corpus` to verify — `TestManifestCorpusCoverage` and `TestManifestBenchCoverage` will catch missing wiring
