package treesitter_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/corpustest"
	"github.com/treesitter-go/treesitter/internal/difftest"
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

	parseFunc := jsonParseFunc(lang)
	corpustest.RunCorpus(t, cases, parseFunc)
}

// TestDifferentialJSON runs the JSON corpus tests comparing Go vs C tree-sitter.
func TestDifferentialJSON(t *testing.T) {
	corpusDir := filepath.Join(corpusGrammarsDir(), "tree-sitter-json", "test", "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		t.Skipf("JSON corpus not found at %s — run 'make fetch-test-grammars' first", corpusDir)
	}

	cases, err := corpustest.ParseCorpusDir(corpusDir)
	if err != nil {
		t.Fatalf("failed to parse corpus: %v", err)
	}

	lang := tg.JSONLanguage()
	lang.LexFn = jsonLexFn

	difftest.RunDifferentialCorpus(t, cases, "source.json", jsonParseFunc(lang))
}

// TestDifferentialJSONSamples runs differential testing on sample JSON files.
func TestDifferentialJSONSamples(t *testing.T) {
	samples := []struct {
		name  string
		input string
	}{
		{"null", "null"},
		{"true", "true"},
		{"false", "false"},
		{"number", "42"},
		{"negative_number", "-3.14"},
		{"string", `"hello world"`},
		{"empty_object", "{}"},
		{"empty_array", "[]"},
		{"simple_object", `{"key": "value"}`},
		{"nested_object", `{"a": {"b": {"c": 1}}}`},
		{"array_of_numbers", `[1, 2, 3, 4, 5]`},
		{"mixed_array", `[null, true, false, 42, "hello"]`},
		{"complex_json", `{"users": [{"name": "Alice", "age": 30}, {"name": "Bob", "age": 25}]}`},
		{"escape_sequences", `"hello\nworld\t\"quoted\""`},
		{"unicode_escape", `"unicode: \u0041"`},
		{"multiline_object", "{\n  \"a\": 1,\n  \"b\": 2,\n  \"c\": 3\n}"},
		{"deeply_nested", `[[[[1]]]]`},
		{"comment_single", "{\n  // comment\n  \"a\": 1\n}"},
		{"comment_block", "{\n  /* block */\n  \"a\": 1\n}"},
		{"multiple_comments", "{\n  \"a\": 1,\n  // c1\n  /* c2 */\n  \"b\": 2\n}"},
	}

	lang := tg.JSONLanguage()
	lang.LexFn = jsonLexFn
	parseFunc := jsonParseFunc(lang)

	for _, s := range samples {
		s := s
		t.Run(s.name, func(t *testing.T) {
			result, err := difftest.Compare([]byte(s.input), "source.json", parseFunc)
			if err != nil {
				t.Fatalf("compare: %v", err)
			}
			if !result.Match {
				t.Errorf("differential divergence\ninput: %s\nGo:  %s\nC:   %s\ndiff: %s",
					s.input, result.GoNorm, result.CNorm, result.Divergence)
			}
		})
	}
}

// TestRegressionJSON runs regression tests from testdata/regressions/json/.
func TestRegressionJSON(t *testing.T) {
	regressionDir := "testdata/regressions/json"
	lang := tg.JSONLanguage()
	lang.LexFn = jsonLexFn
	difftest.RunRegressionTests(t, regressionDir, jsonParseFunc(lang))
}

// TestDifferentialJSONCorpora runs differential testing on real-world JSON files.
func TestDifferentialJSONCorpora(t *testing.T) {
	corporaDir := "testdata/corpora/json"
	if _, err := os.Stat(corporaDir); os.IsNotExist(err) {
		t.Skip("JSON corpora not found")
	}

	lang := tg.JSONLanguage()
	lang.LexFn = jsonLexFn

	difftest.RunDifferentialDir(t, corporaDir, []string{".json"}, jsonParseFunc(lang))
}

// jsonParseFunc returns a ParseFunc for the JSON language.
func jsonParseFunc(lang *ts.Language) corpustest.ParseFunc {
	return func(input []byte) (string, error) {
		p := ts.NewParser()
		p.SetLanguage(lang)
		tree := p.ParseString(context.Background(), input)
		if tree == nil {
			return "", nil
		}
		return tree.RootNode().String(), nil
	}
}
