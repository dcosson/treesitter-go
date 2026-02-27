TREE_SITTER_CLI := $(shell which tree-sitter 2>/dev/null)

# Optional language filter. Pass GRAMMAR=<name> to run only one language.
# The value must match a name in grammars.json (e.g. GRAMMAR=go, GRAMMAR=json).
GRAMMAR ?=

# Validate GRAMMAR against the manifest if set.
ifneq ($(GRAMMAR),)
  VALID_GRAMMARS := $(shell jq -r '.[].name' grammars.json)
  ifeq ($(filter $(GRAMMAR),$(VALID_GRAMMARS)),)
    $(error GRAMMAR=$(GRAMMAR) is not in grammars.json. Valid: $(VALID_GRAMMARS))
  endif
  # For top-level test functions (TestCorpusGo, TestRegressionJSON, etc.)
  # use case-insensitive match since function names are capitalized.
  _RUN_CORPUS := TestCorpus(?i)$(GRAMMAR)$$
  _RUN_REGRESSION := TestRegression(?i)$(GRAMMAR)$$
  _RUN_REALWORLD := TestDifferentialRealworld/$(GRAMMAR)
  _RUN_SCANNER_TRACES := TestScannerTraces/$(GRAMMAR)
  _BENCH_FILTER := BenchmarkParse/go/$(GRAMMAR)/
  _BENCH_FILTER_COMPARE := BenchmarkCompare/.*/$(GRAMMAR)/
  _FUZZ_FILTER := FuzzParse(?i)$(GRAMMAR)$$
else
  _RUN_CORPUS := TestCorpus
  _RUN_REGRESSION := TestRegression
  _RUN_REALWORLD := TestDifferentialRealworld
  _RUN_SCANNER_TRACES := TestScannerTraces
  _BENCH_FILTER := .
  _BENCH_FILTER_COMPARE := .
  _FUZZ_FILTER :=
endif

.PHONY: build test test-coverage fetch-test-grammars test-corpus test-regression fetch-realworld test-realworld-diff deps diff-test bench-grammars bench-self bench-compare generate-scanner-traces test-scanner-traces fuzz

build:
	go build -o build/bin/ ./cmd/...

test:
	go test -race -skip 'TestCorpus|Differential|WithCLI' ./...

test-coverage:
	-go test -skip 'TestCorpus|Differential|WithCLI' -coverprofile=testdata/coverage.out $$(go list ./... | grep -v testgrammars)
	go tool cover -html=testdata/coverage.out -o testdata/coverage.html
	@go tool cover -func=testdata/coverage.out | tail -1
	@echo "Coverage report: testdata/coverage.html"

fetch-test-grammars:
	go run ./cmd/fetch-grammars -config grammars.json -output build/grammars/

test-corpus:
	go test ./e2etest/ -run '$(_RUN_CORPUS)' -v -count=1 -timeout 10m

test-regression:
	go test -v -race -run '$(_RUN_REGRESSION)' -count=1 -timeout 5m ./e2etest/

fetch-realworld:
	go run ./cmd/fetch-realworld -manifest testdata/realworld-manifest.json -output testdata/realworld/

test-realworld-diff:
ifdef TREE_SITTER_CLI
	go test -v -race -run '$(_RUN_REALWORLD)' -count=1 -timeout 30m ./e2etest/
else
	@echo "tree-sitter CLI not found. Run 'make deps' to install."
	@exit 1
endif

deps:
	@if command -v brew >/dev/null 2>&1; then \
		brew install tree-sitter-cli; \
	elif command -v cargo >/dev/null 2>&1; then \
		cargo install tree-sitter-cli; \
	elif command -v npm >/dev/null 2>&1; then \
		npm install -g tree-sitter-cli; \
	else \
		echo "Install tree-sitter CLI: https://tree-sitter.github.io/tree-sitter/"; \
		exit 1; \
	fi
	go install golang.org/x/perf/cmd/benchstat@latest
	@echo ""
	@echo "Run 'make bench-grammars' to build grammar dylibs for CLI benchmarks."

BENCH_DYLIB_DIR := build/benchmark-dylibs
GRAMMAR_DIR := build/grammars

bench-grammars:
ifdef TREE_SITTER_CLI
	@mkdir -p $(BENCH_DYLIB_DIR)
	@jq -r '.[] | .name + " " + (.subpath // "")' grammars.json | while read lang subpath; do \
		grammar_dir="$(GRAMMAR_DIR)/tree-sitter-$$lang"; \
		if [ -n "$$subpath" ]; then grammar_dir="$$grammar_dir/$$subpath"; fi; \
		if [ ! -d "$$grammar_dir/src" ]; then \
			echo "ERROR: grammar directory not found: $$grammar_dir/src — run 'make fetch-test-grammars'" >&2; \
			exit 1; \
		fi; \
		echo "Building $$lang dylib from $$grammar_dir"; \
		$(TREE_SITTER_CLI) build "$$grammar_dir" -o $(BENCH_DYLIB_DIR)/$$lang.dylib || exit 1; \
	done
else
	@echo "tree-sitter CLI not found. Run 'make deps' to install."
	@exit 1
endif

diff-test:
ifdef TREE_SITTER_CLI
	go test ./internal/difftest/... -ts-cli=$(TREE_SITTER_CLI) -v -timeout 15m
else
	@echo "tree-sitter CLI not found. Run 'make deps' to install."
	@exit 1
endif

generate-scanner-traces:
	scripts/generate-scanner-traces.sh

test-scanner-traces:
	go test -v -race -run '$(_RUN_SCANNER_TRACES)' -count=1 -timeout 10m ./e2etest/

FUZZ_TIME ?= 30s

fuzz:
ifneq ($(_FUZZ_FILTER),)
	@echo "Running fuzz targets matching $(GRAMMAR) ($(FUZZ_TIME) each)..."
	@grep -o 'func Fuzz[A-Za-z]*' e2etest/fuzz_test.go | sed 's/^func //' | grep -iE 'FuzzParse$(GRAMMAR)$$' | while read target; do \
		echo "--- $$target ($(FUZZ_TIME)) ---"; \
		go test -fuzz=$$target -fuzztime=$(FUZZ_TIME) -timeout=0 ./e2etest/ || exit 1; \
	done
else
	@echo "Running all fuzz targets ($(FUZZ_TIME) each)..."
	@grep -o 'func Fuzz[A-Za-z]*' e2etest/fuzz_test.go | sed 's/^func //' | while read target; do \
		echo "--- $$target ($(FUZZ_TIME)) ---"; \
		go test -fuzz=$$target -fuzztime=$(FUZZ_TIME) -timeout=0 ./e2etest/ || exit 1; \
	done
endif
	@echo "All fuzz targets passed."

bench-self:
	go test ./e2etest/ -run=NOMATCH -bench='$(_BENCH_FILTER)' -benchmem -count=5 -timeout 10m | tee testdata/bench-results.txt

bench-compare: build
ifdef TREE_SITTER_CLI
	go test ./e2etest/ -run=NOMATCH -bench='$(_BENCH_FILTER_COMPARE)' -benchmem -count=5 -timeout 10m \
		-ts-cli=$(TREE_SITTER_CLI) | tee testdata/bench-results.txt
	@scripts/bench-summary.sh testdata/bench-results.txt
else
	@echo "tree-sitter CLI not found. Run 'make deps' to install."
	@exit 1
endif
