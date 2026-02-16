package treesitter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/corpustest"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
)

// corpusGrammarsDir is the base directory for fetched grammar repos.
// Set by TestMain or defaults to testdata/grammars/ relative to the repo root.
func corpusGrammarsDir() string {
	if dir := os.Getenv("TREESITTER_GRAMMAR_DIR"); dir != "" {
		return dir
	}
	return "testdata/grammars"
}

// TestCorpusJSON runs the tree-sitter-json corpus tests against the Go parser.
func TestCorpusJSON(t *testing.T) {
	corpusDir := filepath.Join(corpusGrammarsDir(), "tree-sitter-json", "test", "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		t.Skipf("JSON corpus not found at %s — run 'make fetch-test-grammars' first", corpusDir)
	}

	cases, err := corpustest.ParseCorpusDir(corpusDir)
	if err != nil {
		t.Fatalf("failed to parse corpus: %v", err)
	}

	if len(cases) == 0 {
		t.Fatal("no corpus test cases found")
	}
	t.Logf("loaded %d corpus test cases", len(cases))

	lang := tg.JSONLanguage()
	lang.LexFn = jsonLexFn

	corpustest.RunCorpus(t, cases, func(input []byte) (string, error) {
		p := ts.NewParser()
		p.SetLanguage(lang)
		tree := p.ParseString(context.Background(), input)
		if tree == nil {
			return "", nil
		}
		return tree.RootNode().String(), nil
	})
}
