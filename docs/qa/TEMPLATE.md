# Pre-Release QA Checklist

**Release**: vX.Y.Z
**Date**: YYYY-MM-DD
**QA performed by**: [name/agent]
**Branch/Commit**: [sha]

---

## 1. Corpus Pass Rate Gate

Run `make test-corpus` and record pass rates per language.

| Language | Pass | Fail | Rate | Prev Release | Delta |
|----------|------|------|------|-------------|-------|
| JSON | | | | | |
| Go | | | | | |
| Python | | | | | |
| JavaScript | | | | | |
| TypeScript | | | | | |
| C | | | | | |
| C++ | | | | | |
| Rust | | | | | |
| Java | | | | | |
| Ruby | | | | | |
| Bash | | | | | |
| CSS | | | | | |
| HTML | | | | | |
| Perl | | | | | |
| Lua | | | | | |
| **Total** | | | | | |

- [ ] No language regresses by more than 1%
- [ ] Total pass rate meets the release target: ____%

## 2. Error Recovery Spot Check

For each of the top 5 languages (JSON, Go, JavaScript, Python, TypeScript):

### 2a. Deleted line test

- [ ] JSON: Take `testdata/smoke/example.json`, delete a random line, parse. ERROR node localized?
- [ ] Go: Take `testdata/review/example.go`, delete a random line, parse. ERROR node localized?
- [ ] JavaScript: Take `testdata/review/example.js`, delete a random line, parse. ERROR node localized?
- [ ] Python: Take `testdata/review/example.py`, delete a random line, parse. ERROR node localized?
- [ ] TypeScript: Take `testdata/review/example.ts`, delete a random line, parse. ERROR node localized?

### 2b. Garbage insertion test

- [ ] JSON: Insert `@@@GARBAGE@@@` at random position, parse. ERROR node localized?
- [ ] Go: Insert `@@@GARBAGE@@@` at random position, parse. ERROR node localized?
- [ ] JavaScript: Insert `@@@GARBAGE@@@` at random position, parse. ERROR node localized?
- [ ] Python: Insert `@@@GARBAGE@@@` at random position, parse. ERROR node localized?
- [ ] TypeScript: Insert `@@@GARBAGE@@@` at random position, parse. ERROR node localized?

### 2c. Judgment

- [ ] Would a syntax highlighter using these trees produce acceptable results for valid portions?

Notes:

## 3. Visual S-Expression Diff Review

For each of the top 5 languages, compare Go parser vs `tree-sitter parse` on review files.

```bash
diff <(go run ./cmd/parse testdata/review/example.go) \
     <(tree-sitter parse testdata/review/example.go)
```

- [ ] Go: diffs reviewed, all differences are documented known issues
- [ ] JavaScript: diffs reviewed
- [ ] Python: diffs reviewed
- [ ] TypeScript: diffs reviewed
- [ ] JSON: diffs reviewed (should be identical)

Notes:

## 4. Smoke Test Each Language

Parse one small file per language, verify non-empty non-ERROR-only tree:

```bash
for f in testdata/smoke/*; do
    echo "=== $f ==="
    go run ./cmd/parse "$f" 2>&1 | head -5
done
```

- [ ] All 15 files produce output with correct root node type
- [ ] No panics
- [ ] No timeouts (each completes in < 5s)

| Language | Root Type | Status |
|----------|-----------|--------|
| JSON | document | |
| Go | source_file | |
| Python | module | |
| JavaScript | program | |
| TypeScript | program | |
| C | translation_unit | |
| C++ | translation_unit | |
| Rust | source_file | |
| Java | program | |
| Ruby | program | |
| Bash | program | |
| CSS | stylesheet | |
| HTML | document | |
| Perl | source_file | |
| Lua | chunk | |

## 5. Performance Sanity Check

- [ ] Run `make bench` and compare to previous release
- [ ] No benchmark regresses by more than 20% without explanation
- [ ] Parse a ~1MB generated file and verify it completes in under 10 seconds

```bash
go test -run '^$' -bench 'BenchmarkParse/go/json/100KB' -benchmem -count=3
```

| Benchmark | This Release | Prev Release | Delta |
|-----------|-------------|-------------|-------|
| JSON 1KB ns/op | | | |
| JSON 10KB ns/op | | | |
| JSON 100KB ns/op | | | |
| Go 1KB ns/op | | | |

Notes:

## 6. Incremental Parse Spot Check

- [ ] Go: Parse `testdata/review/example.go`, apply single-char edit, reparse with old tree. Result matches fresh parse?
- [ ] JavaScript: Same with `testdata/review/example.js`
- [ ] Python: Same with `testdata/review/example.py`
- [ ] Reparse feels instantaneous (< 50ms for single-char edit on 10KB file)?

Notes:

---

## Sign-off

- [ ] All items above checked
- [ ] Any failures documented with issue references
- [ ] Release approved: YES / NO

**Signed**: _______________
**Date**: _______________
