TREE_SITTER_CLI := $(shell which tree-sitter 2>/dev/null)

.PHONY: build test bench bench-grammars fetch-test-grammars fetch-realworld test-corpus test-corpus-json test-regression test-realworld-diff deps diff-test generate-scanner-traces test-scanner-traces fuzz

build:
	go build -o bin/ ./cmd/...

test:
	go test -v -race -skip 'TestCorpus|TestDifferential' ./...

fetch-test-grammars:
	go run ./cmd/fetch-grammars -config testdata/grammars.json -output testdata/grammars/

test-corpus:
	go test ./... -run TestCorpus -v -count=1 -timeout 10m

test-corpus-json:
	go test ./... -run TestCorpus/json -v

test-regression:
	go test -v -race -run 'TestRegression' -count=1 -timeout 5m ./e2etest/

fetch-realworld:
	go run ./cmd/fetch-realworld -manifest testdata/realworld-manifest.json -output testdata/realworld/

test-realworld-diff:
ifdef TREE_SITTER_CLI
	go test -v -race -run 'TestDifferentialRealworld' -count=1 -timeout 30m ./e2etest/
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
GRAMMAR_DIR := testdata/grammars

bench-grammars:
ifdef TREE_SITTER_CLI
	@mkdir -p $(BENCH_DYLIB_DIR)
	@find $(GRAMMAR_DIR) -name grammar.json -path '*/src/grammar.json' | while read gj; do \
		grammar_dir=$$(dirname $$(dirname "$$gj")); \
		lang=$$(basename "$$grammar_dir" | sed 's/^tree-sitter-//'); \
		echo "Building $$lang dylib from $$grammar_dir"; \
		$(TREE_SITTER_CLI) build "$$grammar_dir" -o $(BENCH_DYLIB_DIR)/$$lang.dylib; \
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
	go test -v -race -run 'TestScannerTraces' -count=1 -timeout 10m ./e2etest/

FUZZ_TIME ?= 30s

fuzz:
	@echo "Running all fuzz targets ($(FUZZ_TIME) each)..."
	@grep -o 'func Fuzz[A-Za-z]*' e2etest/fuzz_test.go | sed 's/^func //' | while read target; do \
		echo "--- $$target ($(FUZZ_TIME)) ---"; \
		go test -fuzz=$$target -fuzztime=$(FUZZ_TIME) -timeout=0 ./e2etest/ || exit 1; \
	done
	@echo "All fuzz targets passed."

bench:
ifdef TREE_SITTER_CLI
	go test ./e2etest/ -run=NOMATCH -bench=. -benchmem -count=5 -timeout 10m \
		-ts-cli=$(TREE_SITTER_CLI) | tee testdata/bench-results.txt
else
	go test ./e2etest/ -run=NOMATCH -bench=. -benchmem -count=5 -timeout 10m | tee testdata/bench-results.txt
	@echo "Note: tree-sitter CLI not found, Go-vs-C comparison skipped."
endif
