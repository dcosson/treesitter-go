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
- **Before closing out work**, run the relevant `make` target to verify — don't rely on individual `go test` runs alone.

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

## Versioning & Releases

This project uses its own [semver](https://semver.org/) version number, independent of upstream tree-sitter. The current version is stored in the `VERSION` file at the project root. The `CHANGELOG.md` file tracks all changes per release. The README version table tracks which upstream tree-sitter version the runtime logic is based on.

### Version semantics

- **Patch** (0.1.x): Bug fixes, test improvements, doc updates — no API or behavior changes
- **Minor** (0.x.0): New languages, new public API surface, new features, non-breaking behavior improvements
- **Major** (x.0.0): Breaking API changes (not expected until v1.0.0 stabilization)

### Release process

Follow these steps in order. If any step fails, stop and fix before continuing.

```bash
# 1. Ensure you are on the main branch with a clean working tree
git checkout main
git pull origin main
git status  # must be clean

# 2. Decide the new version (read current from VERSION file)
cat VERSION  # e.g. 0.1.0
# Determine bump type: patch, minor, or major

# 3. Update VERSION file with the new version number
echo "0.2.0" > VERSION

# 4. Update the README version table
#    - Set the Version row to the new version
#    - Set the Upstream tree-sitter row if the upstream version has changed

# 5. Update CHANGELOG.md
#    - Move items from [Unreleased] into a new section: ## [0.2.0] - YYYY-MM-DD
#    - Generate the changelog content from git log:
git log $(git describe --tags --abbrev=0)..HEAD --oneline
#    - Categorize changes under: Added, Changed, Fixed, Removed, Upstream
#    - If the upstream tree-sitter version changed, note it under ### Upstream

# 6. Commit the release
git add VERSION README.md CHANGELOG.md
git commit -m "release: v0.2.0"

# 7. Run tests
make test
make test-corpus

# 8. Tag and push
git tag v0.2.0
git push origin main
git push origin v0.2.0
```

### Important notes

- The `v` prefix on tags is required for Go module versioning (`v0.2.0`, not `0.2.0`)
- Go consumers install with: `go get github.com/dcosson/treesitter-go@v0.2.0`
- Never force-push tags. If a release needs fixing, create a new patch version.
- If tests fail after the commit but before tagging, fix the issue, amend or add a new commit, then tag.

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
