package treesitter_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/corpustest"
	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	cgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cgrammar"
	cppgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cppgrammar"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/java"
	perlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/perl"
	rubygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/ruby"
	rustgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/rustgrammar"
	bashscanner "github.com/treesitter-go/treesitter/scanners/bash"
	cppscanner "github.com/treesitter-go/treesitter/scanners/cpp"
	cssscanner "github.com/treesitter-go/treesitter/scanners/css"
	htmlscanner "github.com/treesitter-go/treesitter/scanners/html"
	perlscanner "github.com/treesitter-go/treesitter/scanners/perl"
	rubyscanner "github.com/treesitter-go/treesitter/scanners/ruby"
	rustscanner "github.com/treesitter-go/treesitter/scanners/rust"
)

// perTestTimeout is the maximum time allowed for parsing a single corpus test input.
const perTestTimeout = 2 * time.Second

// makeCorpusParseFunc creates a ParseFunc for the given language with a per-test timeout.
func makeCorpusParseFunc(lang *ts.Language) corpustest.ParseFunc {
	return func(input []byte) (sexp string, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic during parse: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
		defer cancel()
		p := ts.NewParser()
		p.SetLanguage(lang)
		tree := p.ParseString(ctx, input)
		if tree == nil {
			return "", nil
		}
		return tree.RootNode().String(), nil
	}
}

// runCorpusForLanguage is a shared helper that loads corpus test cases
// from the given grammar repo subdirectory and runs them.
func runCorpusForLanguage(t *testing.T, repoName string, lang *ts.Language) {
	t.Helper()
	corpusDir := filepath.Join(corpusGrammarsDir(), repoName, "test", "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		t.Skipf("%s corpus not found at %s — run 'make fetch-test-grammars' first", repoName, corpusDir)
	}

	cases, err := corpustest.ParseCorpusDir(corpusDir)
	if err != nil {
		t.Fatalf("failed to parse corpus: %v", err)
	}

	if len(cases) == 0 {
		t.Fatal("no corpus test cases found")
	}
	t.Logf("loaded %d corpus test cases for %s", len(cases), repoName)

	corpustest.RunCorpus(t, cases, makeCorpusParseFunc(lang))
}

// TestCorpusC runs the tree-sitter-c corpus tests.
func TestCorpusC(t *testing.T) {
	lang := cgrammar.CLanguage()
	runCorpusForLanguage(t, "tree-sitter-c", lang)
}

// TestCorpusCpp runs the tree-sitter-cpp corpus tests.
func TestCorpusCpp(t *testing.T) {
	lang := cppgrammar.CppLanguage()
	lang.NewExternalScanner = cppscanner.New
	runCorpusForLanguage(t, "tree-sitter-cpp", lang)
}

// TestCorpusRust runs the tree-sitter-rust corpus tests.
func TestCorpusRust(t *testing.T) {
	lang := rustgrammar.RustLanguage()
	lang.NewExternalScanner = rustscanner.New
	runCorpusForLanguage(t, "tree-sitter-rust", lang)
}

// TestCorpusBash runs the tree-sitter-bash corpus tests.
func TestCorpusBash(t *testing.T) {
	lang := bashgrammar.BashLanguage()
	lang.NewExternalScanner = bashscanner.New
	runCorpusForLanguage(t, "tree-sitter-bash", lang)
}

// TestCorpusRuby runs the tree-sitter-ruby corpus tests.
func TestCorpusRuby(t *testing.T) {
	lang := rubygrammar.RubyLanguage()
	lang.NewExternalScanner = rubyscanner.New
	runCorpusForLanguage(t, "tree-sitter-ruby", lang)
}

// TestCorpusPerl runs the tree-sitter-perl corpus tests.
func TestCorpusPerl(t *testing.T) {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New
	runCorpusForLanguage(t, "tree-sitter-perl", lang)
}

// TestCorpusCSS runs the tree-sitter-css corpus tests.
func TestCorpusCSS(t *testing.T) {
	lang := cssgrammar.CssLanguage()
	lang.NewExternalScanner = cssscanner.New
	runCorpusForLanguage(t, "tree-sitter-css", lang)
}

// TestCorpusHTML runs the tree-sitter-html corpus tests.
func TestCorpusHTML(t *testing.T) {
	lang := htmlgrammar.HtmlLanguage()
	lang.NewExternalScanner = htmlscanner.New
	runCorpusForLanguage(t, "tree-sitter-html", lang)
}

// TestCorpusJava runs the tree-sitter-java corpus tests.
func TestCorpusJava(t *testing.T) {
	lang := javagrammar.JavaLanguage()
	runCorpusForLanguage(t, "tree-sitter-java", lang)
}

// TestCorpusGo runs the tree-sitter-go corpus tests.
func TestCorpusGo(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-go", goLang())
}

// TestCorpusPython runs the tree-sitter-python corpus tests.
func TestCorpusPython(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-python", pyLang())
}

// TestCorpusJavaScript runs the tree-sitter-javascript corpus tests.
func TestCorpusJavaScript(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-javascript", jsLang())
}

// TestCorpusTypeScript runs the tree-sitter-typescript corpus tests.
func TestCorpusTypeScript(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-typescript", newTSLang())
}

// TestCorpusLua runs the tree-sitter-lua corpus tests.
func TestCorpusLua(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-lua", luaLang())
}
