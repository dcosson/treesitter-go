package treesitter_test

import (
	"bytes"
	"context"
	iparser "github.com/treesitter-go/treesitter/parser"
	"os"
	"path/filepath"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/corpustest"
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

// fuzzParseTimeout is the maximum time allowed for a single parse during fuzzing.
const fuzzParseTimeout = 5 * time.Second

// --- Fuzz Target 1: Parser Crash Finding (per language) ---

// fuzzParseWithLang is the shared fuzz body for all language targets.
// Property: the parser must never panic. It returns a tree or nil.
func fuzzParseWithLang(f *testing.F, lang *ts.Language, corpusRepoName string) {
	f.Helper()
	seedFromCorpus(f, corpusRepoName)
	seedFromRealworld(f, corpusRepoName)

	f.Fuzz(func(t *testing.T, data []byte) {
		p := iparser.NewParser()
		p.SetLanguage(lang)
		ctx, cancel := context.WithTimeout(context.Background(), fuzzParseTimeout)
		defer cancel()
		tree := p.ParseString(ctx, data)
		if tree != nil {
			_ = tree.RootNode().String()
		}
	})
}

func FuzzParseJSON(f *testing.F) {
	lang := tg.JsonLanguage()
	lang.LexFn = jsonLexFn
	seedFromCorpus(f, "tree-sitter-json")
	f.Fuzz(func(t *testing.T, data []byte) {
		p := iparser.NewParser()
		p.SetLanguage(lang)
		ctx, cancel := context.WithTimeout(context.Background(), fuzzParseTimeout)
		defer cancel()
		tree := p.ParseString(ctx, data)
		if tree != nil {
			_ = tree.RootNode().String()
		}
	})
}

func FuzzParseGo(f *testing.F) {
	fuzzParseWithLang(f, golanggrammar.GoLanguage(), "tree-sitter-go")
}

func FuzzParsePython(f *testing.F) {
	lang := pygrammar.PythonLanguage()
	lang.NewExternalScanner = pyscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-python")
}

func FuzzParseJavaScript(f *testing.F) {
	lang := jsgrammar.JavascriptLanguage()
	lang.NewExternalScanner = jsscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-javascript")
}

func FuzzParseTypeScript(f *testing.F) {
	lang := tsgrammar.TypescriptLanguage()
	lang.NewExternalScanner = tsscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-typescript")
}

func FuzzParseBash(f *testing.F) {
	lang := bashgrammar.BashLanguage()
	lang.NewExternalScanner = bashscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-bash")
}

func FuzzParseRuby(f *testing.F) {
	lang := rubygrammar.RubyLanguage()
	lang.NewExternalScanner = rubyscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-ruby")
}

func FuzzParseRust(f *testing.F) {
	lang := rustgrammar.RustLanguage()
	lang.NewExternalScanner = rustscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-rust")
}

func FuzzParseC(f *testing.F) {
	fuzzParseWithLang(f, cgrammar.CLanguage(), "tree-sitter-c")
}

func FuzzParseCpp(f *testing.F) {
	lang := cppgrammar.CppLanguage()
	lang.NewExternalScanner = cppscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-cpp")
}

func FuzzParseCSS(f *testing.F) {
	lang := cssgrammar.CssLanguage()
	lang.NewExternalScanner = cssscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-css")
}

func FuzzParseHTML(f *testing.F) {
	lang := htmlgrammar.HtmlLanguage()
	lang.NewExternalScanner = htmlscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-html")
}

func FuzzParseJava(f *testing.F) {
	fuzzParseWithLang(f, javagrammar.JavaLanguage(), "tree-sitter-java")
}

func FuzzParsePerl(f *testing.F) {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-perl")
}

func FuzzParseLua(f *testing.F) {
	lang := luagrammar.LuaLanguage()
	lang.NewExternalScanner = luascanner.New
	fuzzParseWithLang(f, lang, "tree-sitter-lua")
}

// --- Fuzz Target 2: Corpus Test Infrastructure ---

// FuzzParseCorpusFile fuzzes the corpus file parser to ensure it never panics
// on malformed input. If the corpus parser crashes, test failures could be masked.
func FuzzParseCorpusFile(f *testing.F) {
	seedFromCorpus(f, "tree-sitter-json")
	seedFromCorpus(f, "tree-sitter-go")
	seedFromCorpus(f, "tree-sitter-python")

	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = corpustest.ParseCorpusFile(data)
	})
}

// --- Fuzz Target 3: External Scanner Serialize/Deserialize ---

// FuzzScannerSerializeRoundTrip fuzzes all external scanner Deserialize methods
// with arbitrary bytes to ensure they never panic and that re-serialization works.
func FuzzScannerSerializeRoundTrip(f *testing.F) {
	f.Add([]byte{}, "bash")
	f.Add([]byte{0, 0, 0, 0}, "python")
	f.Add(bytes.Repeat([]byte{0xFF}, 1024), "rust")
	f.Add([]byte{1, 2, 3, 4, 5, 6, 7, 8}, "cpp")
	f.Add([]byte{0}, "typescript")
	f.Add(bytes.Repeat([]byte{0x80}, 256), "html")
	f.Add([]byte{0xFF, 0xFE, 0x00, 0x01}, "css")
	f.Add(bytes.Repeat([]byte{0x41}, 2048), "perl")
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0}, "ruby")
	f.Add([]byte{3, 0, 0, 0, 5, 0, 0, 0, 0, 0, 0, 0}, "lua")
	f.Add([]byte{0xDE, 0xAD, 0xBE, 0xEF}, "javascript")

	f.Fuzz(func(t *testing.T, data []byte, scannerName string) {
		scanner := newFuzzScanner(scannerName)
		if scanner == nil {
			return
		}

		// Deserialize arbitrary bytes — must not panic.
		scanner.Deserialize(data)

		// Re-serialize — must not panic.
		buf := make([]byte, 4096)
		n := scanner.Serialize(buf)

		// Deserialize the serialized output — must not panic.
		scanner2 := newFuzzScanner(scannerName)
		if scanner2 == nil {
			return
		}
		scanner2.Deserialize(buf[:n])

		// Second serialize must produce identical output.
		buf2 := make([]byte, 4096)
		n2 := scanner2.Serialize(buf2)
		if n != n2 {
			t.Errorf("%s: serialize size mismatch after roundtrip: %d vs %d", scannerName, n, n2)
		}
		for i := uint32(0); i < n && i < n2; i++ {
			if buf[i] != buf2[i] {
				t.Errorf("%s: byte %d mismatch after roundtrip: %d vs %d", scannerName, i, buf[i], buf2[i])
				break
			}
		}
	})
}

// newFuzzScanner creates a scanner by name, returning nil for unknown names.
func newFuzzScanner(name string) ts.ExternalScanner {
	switch name {
	case "bash":
		return bashscanner.New()
	case "python":
		return pyscanner.New()
	case "rust":
		return rustscanner.New()
	case "cpp":
		return cppscanner.New()
	case "typescript":
		return tsscanner.New()
	case "html":
		return htmlscanner.New()
	case "css":
		return cssscanner.New()
	case "javascript":
		return jsscanner.New()
	case "perl":
		return perlscanner.New()
	case "ruby":
		return rubyscanner.New()
	case "lua":
		return luascanner.New()
	default:
		return nil
	}
}

// --- Seed Corpus Helpers ---

// seedFromCorpus adds seed inputs from a grammar's corpus test directory.
// Each test case's input is added as a separate seed.
func seedFromCorpus(f *testing.F, repoName string) {
	f.Helper()
	corpusDir := filepath.Join(corpusGrammarsDir(), repoName, "test", "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		return
	}
	cases, err := corpustest.ParseCorpusDir(corpusDir)
	if err != nil {
		return
	}
	for _, tc := range cases {
		if len(tc.Input) > 0 {
			f.Add(tc.Input)
		}
	}
}

// seedFromRealworld adds seed inputs from real source files in testdata/realworld/<lang>/.
func seedFromRealworld(f *testing.F, repoName string) {
	f.Helper()
	// Map repo names to realworld directory names.
	langDir := repoNameToRealworldLang(repoName)
	if langDir == "" {
		return
	}
	realworldDir := filepath.Join("testdata", "realworld", langDir)
	if _, err := os.Stat(realworldDir); os.IsNotExist(err) {
		return
	}
	entries, err := os.ReadDir(realworldDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(realworldDir, entry.Name()))
		if err != nil {
			continue
		}
		if len(data) > 0 {
			f.Add(data)
		}
	}
}

// repoNameToRealworldLang maps a grammar repo name to the realworld directory name.
func repoNameToRealworldLang(repoName string) string {
	switch repoName {
	case "tree-sitter-json":
		return "json"
	case "tree-sitter-go":
		return "go"
	case "tree-sitter-python":
		return "python"
	case "tree-sitter-javascript":
		return "javascript"
	case "tree-sitter-typescript":
		return "typescript"
	case "tree-sitter-bash":
		return "bash"
	case "tree-sitter-ruby":
		return "ruby"
	case "tree-sitter-rust":
		return "rust"
	case "tree-sitter-c":
		return "c"
	case "tree-sitter-cpp":
		return "cpp"
	case "tree-sitter-css":
		return "css"
	case "tree-sitter-html":
		return "html"
	case "tree-sitter-java":
		return "java"
	case "tree-sitter-perl":
		return "perl"
	case "tree-sitter-lua":
		return "lua"
	default:
		return ""
	}
}
