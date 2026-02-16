.PHONY: build test fetch-test-grammars test-corpus test-corpus-json

build:
	go build ./...

test:
	go test -v -race ./...

fetch-test-grammars:
	go run ./cmd/fetch-grammars -config testdata/grammars.json -output testdata/grammars/

test-corpus:
	go test ./... -run TestCorpus -v -count=1 -timeout 10m

test-corpus-json:
	go test ./... -run TestCorpus/json -v
