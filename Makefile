TREE_SITTER_CLI := $(shell which tree-sitter 2>/dev/null)

.PHONY: build test bench fetch-test-grammars fetch-corpora test-corpus test-corpus-json test-regression test-corpora-diff deps diff-test

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
		brew install tree-sitter; \
	elif command -v cargo >/dev/null 2>&1; then \
		cargo install tree-sitter-cli; \
	elif command -v npm >/dev/null 2>&1; then \
		npm install -g tree-sitter-cli; \
	else \
		echo "Install tree-sitter CLI: https://tree-sitter.github.io/tree-sitter/"; \
		exit 1; \
	fi

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
