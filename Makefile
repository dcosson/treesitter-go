TREE_SITTER_CLI := $(shell which tree-sitter 2>/dev/null)

.PHONY: build test bench bench-grammars fetch-test-grammars fetch-corpora test-corpus test-corpus-json test-regression test-corpora-diff deps diff-test

build:
	go build ./...

test:
	go test -v -race -skip 'TestCorpus|TestDifferential' ./...

fetch-test-grammars:
	go run ./cmd/fetch-grammars -config testdata/grammars.json -output testdata/grammars/

test-corpus:
	go test ./... -run TestCorpus -v -count=1 -timeout 10m

test-corpus-json:
	go test ./... -run TestCorpus/json -v

test-regression:
	go test -v -race -run 'TestRegression' -count=1 -timeout 5m .

fetch-corpora:
	go run ./cmd/fetch-corpora -manifest testdata/corpora-manifest.json -output testdata/corpora/

test-corpora-diff:
ifdef TREE_SITTER_CLI
	go test -v -race -run 'TestDifferentialCorpora' -count=1 -timeout 30m .
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
	mkdir -p $(BENCH_DYLIB_DIR)
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-json -o $(BENCH_DYLIB_DIR)/json.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-go -o $(BENCH_DYLIB_DIR)/go.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-python -o $(BENCH_DYLIB_DIR)/python.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-javascript -o $(BENCH_DYLIB_DIR)/javascript.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-typescript/typescript -o $(BENCH_DYLIB_DIR)/typescript.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-c -o $(BENCH_DYLIB_DIR)/c.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-cpp -o $(BENCH_DYLIB_DIR)/cpp.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-rust -o $(BENCH_DYLIB_DIR)/rust.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-java -o $(BENCH_DYLIB_DIR)/java.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-ruby -o $(BENCH_DYLIB_DIR)/ruby.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-bash -o $(BENCH_DYLIB_DIR)/bash.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-css -o $(BENCH_DYLIB_DIR)/css.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-html -o $(BENCH_DYLIB_DIR)/html.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-perl -o $(BENCH_DYLIB_DIR)/perl.dylib
	$(TREE_SITTER_CLI) build $(GRAMMAR_DIR)/tree-sitter-lua -o $(BENCH_DYLIB_DIR)/lua.dylib
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

bench:
ifdef TREE_SITTER_CLI
	go test . -run=NOMATCH -bench=. -benchmem -count=5 -timeout 10m \
		-ts-cli=$(TREE_SITTER_CLI) | tee bench-results.txt
else
	go test . -run=NOMATCH -bench=. -benchmem -count=5 -timeout 10m | tee bench-results.txt
	@echo "Note: tree-sitter CLI not found, Go-vs-C comparison skipped."
endif
