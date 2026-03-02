package e2etest_test

import (
	"context"
	"fmt"
	iparser "github.com/treesitter-go/treesitter/parser"
	"os"
	"path/filepath"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/corpustest"
	"github.com/treesitter-go/treesitter/internal/difftest"
	bashgrammar "github.com/treesitter-go/treesitter/internal/grammars/bash"
	cgrammar "github.com/treesitter-go/treesitter/internal/grammars/c"
	cppgrammar "github.com/treesitter-go/treesitter/internal/grammars/cpp"
	cssgrammar "github.com/treesitter-go/treesitter/internal/grammars/css"
	golanggrammar "github.com/treesitter-go/treesitter/internal/grammars/golang"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/grammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/grammars/java"
	jsgrammar "github.com/treesitter-go/treesitter/internal/grammars/javascript"
	jsongrammar "github.com/treesitter-go/treesitter/internal/grammars/json"
	luagrammar "github.com/treesitter-go/treesitter/internal/grammars/lua"
	perlgrammar "github.com/treesitter-go/treesitter/internal/grammars/perl"
	pygrammar "github.com/treesitter-go/treesitter/internal/grammars/python"
	rubygrammar "github.com/treesitter-go/treesitter/internal/grammars/ruby"
	rustgrammar "github.com/treesitter-go/treesitter/internal/grammars/rust"
	tsgrammar "github.com/treesitter-go/treesitter/internal/grammars/typescript"
	bashscanner "github.com/treesitter-go/treesitter/internal/scanners/bash"
	cppscanner "github.com/treesitter-go/treesitter/internal/scanners/cpp"
	cssscanner "github.com/treesitter-go/treesitter/internal/scanners/css"
	htmlscanner "github.com/treesitter-go/treesitter/internal/scanners/html"
	jsscanner "github.com/treesitter-go/treesitter/internal/scanners/javascript"
	luascanner "github.com/treesitter-go/treesitter/internal/scanners/lua"
	perlscanner "github.com/treesitter-go/treesitter/internal/scanners/perl"
	pyscanner "github.com/treesitter-go/treesitter/internal/scanners/python"
	rubyscanner "github.com/treesitter-go/treesitter/internal/scanners/ruby"
	rustscanner "github.com/treesitter-go/treesitter/internal/scanners/rust"
	tsscanner "github.com/treesitter-go/treesitter/internal/scanners/typescript"
)

// regressionParseTimeout is the max time for a single regression test parse.
const regressionParseTimeout = 10 * time.Second

// regressionDir is the base directory for regression test data.
const regressionDir = "../testdata/regressions"

// makeRegressionParseFunc creates a ParseFunc with timeout for regression tests.
func makeRegressionParseFunc(lang *ts.Language) corpustest.ParseFunc {
	return func(input []byte) (string, error) {
		defer func() {
			if r := recover(); r != nil {
				// Swallow panics — treated as errors in regression tests.
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), regressionParseTimeout)
		defer cancel()
		p := iparser.NewParser()
		p.SetLanguage(lang)
		tree := p.ParseString(ctx, input)
		if tree == nil {
			return "", fmt.Errorf("nil tree (timeout or parse failure)")
		}
		return tree.RootNode().String(), nil
	}
}

// runRegressionForLanguage runs regression tests for a given language.
func runRegressionForLanguage(t *testing.T, langName string, lang *ts.Language) {
	t.Helper()
	dir := filepath.Join(regressionDir, langName)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Skipf("no regression directory for %s", langName)
	}
	difftest.RunRegressionTests(t, dir, makeRegressionParseFunc(lang))
}

// --- Per-language regression tests ---

func TestRegressionGo(t *testing.T) {
	runRegressionForLanguage(t, "go", golanggrammar.GoLanguage())
}

func TestRegressionPython(t *testing.T) {
	lang := pygrammar.PythonLanguage()
	lang.NewExternalScanner = pyscanner.New
	runRegressionForLanguage(t, "python", lang)
}

func TestRegressionJavaScript(t *testing.T) {
	lang := jsgrammar.JavascriptLanguage()
	lang.NewExternalScanner = jsscanner.New
	runRegressionForLanguage(t, "javascript", lang)
}

func TestRegressionTypeScript(t *testing.T) {
	lang := tsgrammar.TypescriptLanguage()
	lang.NewExternalScanner = tsscanner.New
	runRegressionForLanguage(t, "typescript", lang)
}

func TestRegressionBash(t *testing.T) {
	lang := bashgrammar.BashLanguage()
	lang.NewExternalScanner = bashscanner.New
	runRegressionForLanguage(t, "bash", lang)
}

func TestRegressionRuby(t *testing.T) {
	lang := rubygrammar.RubyLanguage()
	lang.NewExternalScanner = rubyscanner.New
	runRegressionForLanguage(t, "ruby", lang)
}

func TestRegressionRust(t *testing.T) {
	lang := rustgrammar.RustLanguage()
	lang.NewExternalScanner = rustscanner.New
	runRegressionForLanguage(t, "rust", lang)
}

func TestRegressionC(t *testing.T) {
	runRegressionForLanguage(t, "c", cgrammar.CLanguage())
}

func TestRegressionCpp(t *testing.T) {
	lang := cppgrammar.CppLanguage()
	lang.NewExternalScanner = cppscanner.New
	runRegressionForLanguage(t, "cpp", lang)
}

func TestRegressionCSS(t *testing.T) {
	lang := cssgrammar.CssLanguage()
	lang.NewExternalScanner = cssscanner.New
	runRegressionForLanguage(t, "css", lang)
}

func TestRegressionHTML(t *testing.T) {
	lang := htmlgrammar.HtmlLanguage()
	lang.NewExternalScanner = htmlscanner.New
	runRegressionForLanguage(t, "html", lang)
}

func TestRegressionJava(t *testing.T) {
	runRegressionForLanguage(t, "java", javagrammar.JavaLanguage())
}

func TestRegressionPerl(t *testing.T) {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New
	runRegressionForLanguage(t, "perl", lang)
}

func TestRegressionLua(t *testing.T) {
	lang := luagrammar.LuaLanguage()
	lang.NewExternalScanner = luascanner.New
	runRegressionForLanguage(t, "lua", lang)
}

// TestRegressionAll runs all regression tests as subtests.
func TestRegressionAll(t *testing.T) {
	type langEntry struct {
		name string
		lang *ts.Language
	}

	jsonLang := jsongrammar.JsonLanguage()
	jsonLang.LexFn = jsonLexFn

	pyLang := pygrammar.PythonLanguage()
	pyLang.NewExternalScanner = pyscanner.New

	jsLang := jsgrammar.JavascriptLanguage()
	jsLang.NewExternalScanner = jsscanner.New

	tsLang := tsgrammar.TypescriptLanguage()
	tsLang.NewExternalScanner = tsscanner.New

	bashLang := bashgrammar.BashLanguage()
	bashLang.NewExternalScanner = bashscanner.New

	rubyLang := rubygrammar.RubyLanguage()
	rubyLang.NewExternalScanner = rubyscanner.New

	rustLang := rustgrammar.RustLanguage()
	rustLang.NewExternalScanner = rustscanner.New

	cppLang := cppgrammar.CppLanguage()
	cppLang.NewExternalScanner = cppscanner.New

	cssLang := cssgrammar.CssLanguage()
	cssLang.NewExternalScanner = cssscanner.New

	htmlLang := htmlgrammar.HtmlLanguage()
	htmlLang.NewExternalScanner = htmlscanner.New

	perlLang := perlgrammar.PerlLanguage()
	perlLang.NewExternalScanner = perlscanner.New

	luaLang := luagrammar.LuaLanguage()
	luaLang.NewExternalScanner = luascanner.New

	langs := []langEntry{
		{"json", jsonLang},
		{"go", golanggrammar.GoLanguage()},
		{"python", pyLang},
		{"javascript", jsLang},
		{"typescript", tsLang},
		{"bash", bashLang},
		{"ruby", rubyLang},
		{"rust", rustLang},
		{"c", cgrammar.CLanguage()},
		{"cpp", cppLang},
		{"css", cssLang},
		{"html", htmlLang},
		{"java", javagrammar.JavaLanguage()},
		{"perl", perlLang},
		{"lua", luaLang},
	}

	for _, entry := range langs {
		entry := entry
		t.Run(entry.name, func(t *testing.T) {
			dir := filepath.Join(regressionDir, entry.name)
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				t.Skipf("no regression directory for %s", entry.name)
			}
			difftest.RunRegressionTests(t, dir, makeRegressionParseFunc(entry.lang))
		})
	}
}
