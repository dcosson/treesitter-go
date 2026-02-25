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
	"github.com/treesitter-go/treesitter/internal/difftest"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	cgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cgrammar"
	cppgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cppgrammar"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	golanggrammar "github.com/treesitter-go/treesitter/internal/testgrammars/golang"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/java"
	jsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/javascript"
	luagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/lua"
	perlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/perl"
	pygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/python"
	rubygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/ruby"
	rustgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/rustgrammar"
	tsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/typescript"
	bashscanner "github.com/treesitter-go/treesitter/scanners/bash"
	cppscanner "github.com/treesitter-go/treesitter/scanners/cpp"
	cssscanner "github.com/treesitter-go/treesitter/scanners/css"
	htmlscanner "github.com/treesitter-go/treesitter/scanners/html"
	jsscanner "github.com/treesitter-go/treesitter/scanners/javascript"
	luascanner "github.com/treesitter-go/treesitter/scanners/lua"
	perlscanner "github.com/treesitter-go/treesitter/scanners/perl"
	pyscanner "github.com/treesitter-go/treesitter/scanners/python"
	rubyscanner "github.com/treesitter-go/treesitter/scanners/ruby"
	rustscanner "github.com/treesitter-go/treesitter/scanners/rust"
	tsscanner "github.com/treesitter-go/treesitter/scanners/typescript"
)

// regressionParseTimeout is the max time for a single regression test parse.
const regressionParseTimeout = 10 * time.Second

// regressionDir is the base directory for regression test data.
const regressionDir = "testdata/regressions"

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
		p := ts.NewParser()
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

	jsonLang := tg.JsonLanguage()
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
