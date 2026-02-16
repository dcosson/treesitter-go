# Testing Strategy Addendum

Additions and revisions to the main [testing-strategy.md](testing-strategy.md).
Written 2026-02-16.

---

## Section 4 Revision: CLI-Based Cross-Implementation Testing

**Replaces** the CGo-based approach described in the original Section 4. The
`internal/difftest` package already implements this approach.

### Rationale

The original strategy called for a CGo wrapper around the C tree-sitter library
for differential testing. We replaced this with the `tree-sitter` CLI binary
(the Rust-based reference implementation). Benefits:

- **Pure Go project**: No CGo dependency, no C toolchain required for tests
- **Same oracle**: The CLI uses the same C parser core as the library
- **Simpler harness**: Shell out to `tree-sitter parse <file>`, compare output
- **Unified harness**: Same tool for correctness comparison and performance timing

### How It Works

The `internal/difftest` package provides the implementation:

1. **Go parser** produces an S-expression via `tree.RootNode().String()`
2. **CLI parser** runs `tree-sitter parse --scope <scope> <file>` and captures stdout
3. Both outputs are normalized (strip point ranges, field annotations, collapse
   whitespace) via `corpustest.NormalizeSExpression` + `corpustest.StripFields`
4. Normalized strings are compared; divergences report context around the first
   difference

**Note**: Byte position comparisons are explicitly out of scope. S-expression
comparison is sufficient for verifying parse tree correctness. If byte ranges
differ but S-expressions match, the impact is limited to code navigation
(go-to-definition), and such bugs are extremely unlikely when S-expressions
agree. This can be revisited if edge cases arise.

### CLI Installation

A `make deps` target installs the tree-sitter CLI using the best available
package manager:

```makefile
deps:
	@if command -v brew >/dev/null 2>&1; then \
		brew install tree-sitter; \
	elif command -v cargo >/dev/null 2>&1; then \
		cargo install tree-sitter-cli; \
	elif command -v npm >/dev/null 2>&1; then \
		npm install -g tree-sitter-cli; \
	else \
		echo "Install tree-sitter CLI: https://tree-sitter.github.io/tree-sitter/"; \
		exit 1; \
	fi
```

For CI:
```yaml
- run: npm install tree-sitter-cli
- run: export PATH="./node_modules/.bin:$PATH"
```

### Test Functions

- `RunDifferentialCorpus` -- run corpus test cases through both parsers
- `RunDifferentialDir` -- run all files in a directory through both parsers
- `RunDifferentialFile` -- single file comparison
- `Compare` -- low-level: returns `CompareResult` with both S-expressions and diff

### CLI Binary Path as Explicit Argument

The tree-sitter CLI binary path is passed as an explicit argument, not
auto-discovered via `LookPath`. This makes the dependency explicit and
avoids silent skips when the CLI isn't installed.

- **Test code** accepts the path via a `-ts-cli` test flag or `TS_CLI_PATH`
  environment variable. If neither is set, differential/benchmark tests skip.
- **Makefile** passes the path from `$(which tree-sitter)`:

```makefile
TREE_SITTER_CLI := $(shell which tree-sitter 2>/dev/null)

diff-test:
ifdef TREE_SITTER_CLI
	go test ./internal/difftest/... -ts-cli=$(TREE_SITTER_CLI) -v -timeout 15m
else
	@echo "tree-sitter CLI not found. Run 'make deps' to install."
	@exit 1
endif

bench:
ifdef TREE_SITTER_CLI
	go test ./... -bench=. -benchmem -count=5 -timeout 10m \
		-ts-cli=$(TREE_SITTER_CLI) | tee bench-results.txt
else
	go test ./... -bench=. -benchmem -count=5 -timeout 10m | tee bench-results.txt
	@echo "Note: tree-sitter CLI not found, Go-vs-C comparison skipped."
endif
```

- **CI** installs the CLI via `make deps` and passes the path automatically.

This way, benchmarks always run (Go-only timing), and when the tree-sitter
binary is available, the comparison sub-benchmarks run as well.

### Performance Comparison via CLI

Time both implementations on the same input:

```go
var tsCLIPath = flag.String("ts-cli", os.Getenv("TS_CLI_PATH"), "path to tree-sitter CLI binary")

func BenchmarkDifferential(b *testing.B, filePath string, goParseFunc ParseFunc) {
    if *tsCLIPath == "" {
        b.Skip("tree-sitter CLI not available (pass -ts-cli=<path>)")
    }
    input, _ := os.ReadFile(filePath)
    scope := difftest.Scope[filepath.Ext(filePath)]

    b.Run("go", func(b *testing.B) {
        b.SetBytes(int64(len(input)))
        for i := 0; i < b.N; i++ {
            goParseFunc(input)
        }
    })

    b.Run("cli", func(b *testing.B) {
        b.SetBytes(int64(len(input)))
        for i := 0; i < b.N; i++ {
            difftest.ParseWithCLI(*tsCLIPath, filePath, scope)
        }
    })
}
```

Note: CLI timing includes process spawn overhead, so it overstates the C
parser's latency. This is acceptable -- if Go is within 2-4x of the CLI
wall-clock time, it is almost certainly faster than the C parser itself (since
the CLI adds ~5-10ms of startup). For precise C-only timing, use
`tree-sitter parse --time` and parse the reported duration from stderr.

---

## New Section: Fuzz Testing

### Go Native Fuzzing

Use `go test -fuzz` (Go 1.18+). Three fuzz targets, each covering a distinct
attack surface.

#### Fuzz Target 1: Parser Crash Finding

```go
// fuzz_test.go

func FuzzParseBytes(f *testing.F) {
    // Seed with real source files and corpus test inputs.
    seedFromDir(f, "testdata/grammars/tree-sitter-json/test/corpus/")
    seedFromDir(f, "testdata/corpora/json/")

    lang := tg.JSONLanguage()
    lang.LexFn = jsonLexFn

    f.Fuzz(func(t *testing.T, data []byte) {
        p := ts.NewParser()
        p.SetLanguage(lang)
        ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        defer cancel()
        tree := p.ParseString(ctx, data)
        // Property: never panics. Returns a tree or nil.
        if tree != nil {
            _ = tree.RootNode().String()
        }
    })
}
```

One fuzz target per language with an external scanner (JSON, Go, JavaScript,
Python at minimum). The shared `seedFromDir` helper adds corpus `.txt` file
inputs and real source files from `testdata/corpora/`.

#### Fuzz Target 2: Corpus Test Parser

The S-expression parser in `internal/corpustest` is part of our test
infrastructure. If it panics on malformed input, test failures could be masked.

```go
func FuzzParseCorpusFile(f *testing.F) {
    seedFromDir(f, "testdata/grammars/tree-sitter-json/test/corpus/")
    f.Fuzz(func(t *testing.T, data []byte) {
        // Must not panic.
        _, _ = corpustest.ParseCorpusFile(data)
    })
}
```

#### Fuzz Target 3: External Scanner Serialize/Deserialize

Each scanner that implements state serialization must survive round-trip with
arbitrary bytes.

```go
func FuzzScannerSerializeRoundTrip(f *testing.F) {
    f.Add([]byte{})
    f.Add([]byte{0, 0, 0, 0})
    f.Add(bytes.Repeat([]byte{0xFF}, 1024))

    f.Fuzz(func(t *testing.T, data []byte) {
        scanner := pythonscanner.New()
        // Deserialize arbitrary bytes -- must not panic.
        scanner.Deserialize(data)
        // Re-serialize -- must not panic.
        buf := make([]byte, 4096)
        n := scanner.Serialize(buf)
        _ = n
    })
}
```

### Seed Corpus

Seed files come from two sources:
1. Grammar corpus test inputs (extracted from `.txt` files)
2. Real source files from `testdata/corpora/`

A helper extracts individual inputs from corpus files and writes them to
`testdata/fuzz/corpus/<lang>/` as individual seed files. Run once during setup.

### CI Integration

Fuzz tests do not run in the normal `make test` flow. Add a dedicated CI job:

```yaml
fuzz-tests:
  runs-on: ubuntu-latest
  timeout-minutes: 30
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
    - run: make fetch-test-grammars
    - run: go test -fuzz=FuzzParseBytes -fuzztime=5m ./...
    - run: go test -fuzz=FuzzParseCorpusFile -fuzztime=2m ./internal/corpustest/
    - run: go test -fuzz=FuzzScannerSerializeRoundTrip -fuzztime=2m ./...
```

Run on a schedule (nightly) rather than every PR. Crash-reproducing inputs are
committed to `testdata/fuzz/` and become permanent regression seeds.

---

## New Section: Error Recovery Testing

### Purpose

Tree-sitter is designed to parse incomplete and malformed input gracefully. We
must verify that the Go port's error recovery produces reasonable trees -- not
just that it does not crash.

### Input Categories

| Category | Description | Example |
|----------|-------------|---------|
| **Truncated files** | File cut off mid-token or mid-construct | `func main() { fmt.Pr` |
| **Syntax errors** | Single deliberate error in valid code | `if x == { y }` (missing condition) |
| **Mixed valid/invalid** | Valid code with garbage injected | Valid Go with `@#$%` on line 15 |
| **Incomplete constructs** | Unclosed brackets, strings, blocks | `{"key": "value` |
| **Empty input** | Zero bytes, whitespace only | `""`, `"   \n\t  "` |

### Properties to Verify

For every malformed input:

1. **Parser does not panic** -- returns a tree or nil
2. **ERROR or MISSING nodes present** -- the tree acknowledges the problem
3. **Rest of tree is reasonable** -- valid portions before/after the error
   produce correct subtrees (spot-check S-expression structure)
4. **Byte ranges are valid** -- no node extends beyond input length,
   children ordered and within parent ranges (reuse `assertTreeConsistent`
   from Section 5.4)

### Test Structure

```
testdata/error-recovery/
├── json/
│   ├── truncated-object.json
│   ├── missing-comma.json
│   └── unclosed-string.json
├── go/
│   ├── truncated-func.go
│   ├── missing-semicolon.go
│   └── unclosed-brace.go
├── javascript/
│   └── ...
└── ...
```

Each file is a standalone malformed input. No `.expected` file -- we verify
properties, not exact trees (error recovery is implementation-defined and
may differ between Go and C).

```go
func TestErrorRecovery(t *testing.T) {
    for _, lang := range errorRecoveryLanguages {
        dir := filepath.Join("testdata/error-recovery", lang.name)
        files, _ := filepath.Glob(filepath.Join(dir, "*"))

        for _, f := range files {
            t.Run(filepath.Base(f), func(t *testing.T) {
                input, _ := os.ReadFile(f)
                tree := mustParse(t, lang.language, input)

                // Must have at least one ERROR or MISSING node.
                if !hasErrorNode(tree.RootNode()) {
                    t.Error("expected ERROR or MISSING node in malformed input")
                }
                // Structural invariants still hold.
                assertTreeConsistent(t, tree.RootNode(), input)
            })
        }
    }
}
```

### Regression Collection

When users or fuzzing discover inputs where the Go parser's error recovery
diverges significantly from the C parser (e.g., the Go parser produces
`(ERROR)` for the entire file while C recovers most of the tree), add these
to `testdata/regressions/<lang>/` with `.input` and `.expected` pairs. These
become permanent regression tests via `RunRegressionTests`.

### Scope

Start with JSON (simplest grammar, easiest to reason about error recovery),
Go, and JavaScript. Expand to remaining languages as those stabilize. Aim
for 10-20 malformed inputs per language covering each category above.

---

## New Section: Implementation Status

### What Is Implemented

| Component | Status | Notes |
|-----------|--------|-------|
| Corpus test runner (`internal/corpustest`) | Done | Parses corpus files, runs against Go parser |
| Corpus tests for all 15 languages | Done | `corpus_languages_test.go` |
| Integration tests (hand-written) | Done | 79 tests across 15 languages, all passing |
| CI pipeline | Done | GitHub Actions: multi-platform unit tests, corpus tests, benchmarks |
| Benchmark suite | Partial | `benchmark_test.go` exists, Go-only |
| External scanner unit tests | Partial | Lua scanner has 17 unit tests |
| Ported internal API tests | Not started | parser_test, node_test, query_test from upstream |
| Differential testing vs CLI | Not started | Design in this addendum |
| Fuzz testing | Not started | |
| Error recovery tests | Not started | |
| Real-world corpora collection | Not started | Need version-pinned files |
| Performance comparison vs CLI | Not started | |
| Scanner round-trip tests | Not started | |
| Regression test directory | Not started | `testdata/regressions/` |

### Current Corpus Pass Rates

Post trailing-newline fix (d312604) and alias-sequence fix (9e978d1). 343
failures across 14 languages (JSON is 100%). Top failure category: comment
placement (107 failures, 31%).

### Priority Ordering for Remaining Test Work

1. **Fuzz testing** -- highest ROI crash-finding before stabilization
2. **Error recovery test files** -- start with JSON/Go/JavaScript
3. **CLI performance comparison benchmarks** -- quantify Go vs C gap
4. **CI pipeline** -- GitHub Actions for corpus, differential, fuzz, bench
5. **Real-world corpora** -- collect version-pinned files for Go, JS, Python
6. **Differential testing for all 15 languages** -- currently only JSON is wired

---

## Section 6 Revision: Unified Benchmark Design

**Replaces** the separate Go-only and CGo comparison benchmarks in Section 6.

### Principle

All benchmarks should be designed to compare against the C implementation via
CLI. When the CLI is not available, they degrade to Go-only mode. There is no
separate "Go-only benchmark suite" vs "comparison benchmark suite".

### Benchmark Structure

```go
func BenchmarkParse(b *testing.B) {
    files := loadBenchFiles("testdata/bench/")
    hasCLI := exec.LookPath("tree-sitter") == nil

    for _, f := range files {
        input, _ := os.ReadFile(f.path)
        scope := difftest.Scope[filepath.Ext(f.path)]

        b.Run("go/"+f.name, func(b *testing.B) {
            p := ts.NewParser()
            p.SetLanguage(f.language)
            b.SetBytes(int64(len(input)))
            b.ReportAllocs()
            b.ResetTimer()
            for i := 0; i < b.N; i++ {
                p.ParseString(context.Background(), input)
            }
        })

        if hasCLI {
            b.Run("cli/"+f.name, func(b *testing.B) {
                b.SetBytes(int64(len(input)))
                b.ResetTimer()
                for i := 0; i < b.N; i++ {
                    difftest.ParseWithCLI(f.path, scope)
                }
            })
        }
    }
}
```

### Metrics to Report

| Metric | How | Why |
|--------|-----|-----|
| **Throughput** (bytes/ms) | `b.SetBytes` + Go's ns/op | Primary speed metric |
| **Memory** (allocs/parse) | `b.ReportAllocs` | Track allocation pressure |
| **Latency p50/p99** | Custom histogram in extended bench mode | Tail latency for editor use |

For latency percentiles, run a separate benchmark function that collects
individual parse times into a slice and computes percentiles:

```go
func BenchmarkLatencyDistribution(b *testing.B) {
    // Parse 1000 times, record each duration.
    // Report p50, p95, p99 via b.ReportMetric.
    b.ReportMetric(p50.Seconds()*1000, "p50-ms")
    b.ReportMetric(p99.Seconds()*1000, "p99-ms")
}
```

### Benchmark Files

Reuse the existing `testdata/bench/` layout. Add files for each supported
language at ~1KB, ~10KB, and ~100KB sizes where available.

### CI Integration

Benchmarks run on PRs with `benchmark-action/github-action-benchmark` for
regression detection. Alert threshold: 150% (50% slower triggers a comment).
CLI comparison runs when `tree-sitter` is installed in the CI environment.

---

## New Section: Manual QA Testing Plans

### Purpose

Automated tests verify deterministic properties. Manual QA uses human/agent
judgment to catch issues that are correct-by-spec but wrong-by-intent: poor
error recovery, confusing tree shapes, performance regressions that benchmarks
miss because the test files are too small.

### Pre-Release QA Checklist

Run before every tagged release. Each item should be performed by a human or
agent and signed off.

#### 1. Corpus Pass Rate Gate

- [ ] Run `make test-corpus` and record pass rates per language
- [ ] Compare to previous release -- no language regresses by more than 1%
- [ ] Total pass rate meets the release target (document target per release)

#### 2. Error Recovery Spot Check

For each of the top 5 languages (JSON, Go, JavaScript, Python, TypeScript):

- [ ] Take a 50-100 line source file and delete a random line. Parse it.
  Inspect the S-expression. Does the error recovery look reasonable? Is the
  ERROR node localized to the deletion site, or does it swallow the entire file?
- [ ] Take a 50-100 line source file and insert `@@@GARBAGE@@@` at a random
  position. Parse it. Same inspection.
- [ ] Judgment call: would a syntax highlighter using this tree produce
  acceptable results for the valid portions of the file?

#### 3. Visual S-Expression Diff Review

For each of the top 5 languages, pick 3 representative source files (~50 lines
each) and run both the Go parser and `tree-sitter parse`. Visually diff the
S-expression output side by side.

```bash
# Generate side-by-side diff for review.
diff <(go run ./cmd/parse testdata/review/example.go) \
     <(tree-sitter parse testdata/review/example.go) \
     | head -100
```

- [ ] Review each diff. Are remaining differences documented known issues?
- [ ] No new unexpected divergences compared to previous release

#### 4. Smoke Test Each Language

For each of the 15 languages, parse one small representative file and verify
it produces a non-empty, non-ERROR-only tree:

```bash
for f in testdata/smoke/*.{json,go,js,py,rs,c,cpp,java,rb,ts,sh,css,html,lua,pl}; do
    echo "=== $f ==="
    go run ./cmd/parse "$f" | head -5
done
```

- [ ] Every file produces output with the correct root node type
- [ ] No panics, no timeouts

#### 5. Performance Sanity Check

- [ ] Run `make bench` and compare to previous release
- [ ] No benchmark regresses by more than 20% without explanation
- [ ] Parse a ~1MB generated file (e.g., `testdata/bench/xlarge.go`) and verify
  it completes in under 10 seconds

#### 6. Incremental Parse Spot Check

- [ ] Open a ~100 line Go file. Parse it. Apply a single-character edit
  (e.g., rename a variable). Reparse with the old tree. Verify the result
  matches a fresh parse of the edited file.
- [ ] Repeat for JavaScript and Python.
- [ ] Judgment call: does the incremental reparse feel instantaneous (< 50ms
  for a single-char edit on a 10KB file)?

### Recording Results

QA results are recorded in `docs/qa/YYYY-MM-DD-vX.Y.Z.md` with the checklist
above filled in, any notes on judgment calls, and a final sign-off. These files
are committed to the repo as a permanent record.
