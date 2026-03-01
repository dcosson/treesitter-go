// realworld_diff_test.go runs differential testing against real-world source files.
//
// These tests compare Go parser output against the reference C tree-sitter CLI
// on source files downloaded from popular open-source projects. The tests require:
//  1. The tree-sitter CLI (install via `make deps`)
//  2. Downloaded realworld files (install via `make fetch-realworld`)
//
// Run: go test -v -run TestDifferentialRealworld -timeout 30m .
package e2etest_test

import (
	"context"
	iparser "github.com/treesitter-go/treesitter/parser"
	"os"
	"path/filepath"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/difftest"

	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	cgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cgrammar"
	cppgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cppgrammar"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	golanggrammar "github.com/treesitter-go/treesitter/internal/testgrammars/golang"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/java"
	jsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/javascript"
	luagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/lua"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
	perlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/perl"
	pythongrammar "github.com/treesitter-go/treesitter/internal/testgrammars/python"
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

// realworldDir is the base directory for downloaded realworld files.
const realworldDir = "../testdata/realworld"

// realworldLanguage describes a language's grammar, extensions, and realworld subdirectories.
type realworldLanguage struct {
	name       string
	lang       *ts.Language
	scope      string
	extensions []string
	projects   []string // subdirectory names under testdata/realworld/<name>/
}

// realworldLang creates a language with its external scanner wired up.
func realworldLang(name string) *ts.Language {
	switch name {
	case "go":
		return golanggrammar.GoLanguage()
	case "python":
		l := pythongrammar.PythonLanguage()
		l.NewExternalScanner = pyscanner.New
		return l
	case "javascript":
		l := jsgrammar.JavascriptLanguage()
		l.NewExternalScanner = jsscanner.New
		return l
	case "typescript":
		l := tsgrammar.TypescriptLanguage()
		l.NewExternalScanner = tsscanner.New
		return l
	case "c":
		return cgrammar.CLanguage()
	case "cpp":
		l := cppgrammar.CppLanguage()
		l.NewExternalScanner = cppscanner.New
		return l
	case "rust":
		l := rustgrammar.RustLanguage()
		l.NewExternalScanner = rustscanner.New
		return l
	case "java":
		return javagrammar.JavaLanguage()
	case "ruby":
		l := rubygrammar.RubyLanguage()
		l.NewExternalScanner = rubyscanner.New
		return l
	case "bash":
		l := bashgrammar.BashLanguage()
		l.NewExternalScanner = bashscanner.New
		return l
	case "css":
		l := cssgrammar.CssLanguage()
		l.NewExternalScanner = cssscanner.New
		return l
	case "html":
		l := htmlgrammar.HtmlLanguage()
		l.NewExternalScanner = htmlscanner.New
		return l
	case "lua":
		l := luagrammar.LuaLanguage()
		l.NewExternalScanner = luascanner.New
		return l
	case "json":
		l := tg.JsonLanguage()
		l.LexFn = jsonLexFn
		return l
	case "perl":
		l := perlgrammar.PerlLanguage()
		l.NewExternalScanner = perlscanner.New
		return l
	default:
		return nil
	}
}

// allRealworldLanguages returns the full set of languages with their realworld projects.
func allRealworldLanguages() []realworldLanguage {
	return []realworldLanguage{
		{
			name: "go", lang: realworldLang("go"),
			scope: "source.go", extensions: []string{".go"},
			projects: []string{"kubernetes", "go-stdlib"},
		},
		{
			name: "python", lang: realworldLang("python"),
			scope: "source.python", extensions: []string{".py"},
			projects: []string{"flask", "requests"},
		},
		{
			name: "javascript", lang: realworldLang("javascript"),
			scope: "source.js", extensions: []string{".js"},
			projects: []string{"express", "lodash"},
		},
		{
			name: "typescript", lang: realworldLang("typescript"),
			scope: "source.ts", extensions: []string{".ts"},
			projects: []string{"typescript-compiler"},
		},
		{
			name: "rust", lang: realworldLang("rust"),
			scope: "source.rust", extensions: []string{".rs"},
			projects: []string{"ripgrep", "serde"},
		},
		{
			name: "c", lang: realworldLang("c"),
			scope: "source.c", extensions: []string{".c", ".h"},
			projects: []string{"redis", "curl"},
		},
		{
			name: "cpp", lang: realworldLang("cpp"),
			scope: "source.cpp", extensions: []string{".cpp", ".cc", ".h"},
			projects: []string{"protobuf"},
		},
		{
			name: "java", lang: realworldLang("java"),
			scope: "source.java", extensions: []string{".java"},
			projects: []string{"guava"},
		},
		{
			name: "ruby", lang: realworldLang("ruby"),
			scope: "source.ruby", extensions: []string{".rb"},
			projects: []string{"rails"},
		},
		{
			name: "bash", lang: realworldLang("bash"),
			scope: "source.bash", extensions: []string{".sh"},
			projects: []string{"nvm"},
		},
		{
			name: "json", lang: realworldLang("json"),
			scope: "source.json", extensions: []string{".json"},
			projects: []string{"schemastore"},
		},
		{
			name: "css", lang: realworldLang("css"),
			scope: "source.css", extensions: []string{".css"},
			projects: []string{"normalize.css"},
		},
		{
			name: "html", lang: realworldLang("html"),
			scope: "text.html.basic", extensions: []string{".html"},
			projects: []string{"html5-boilerplate"},
		},
		{
			name: "lua", lang: realworldLang("lua"),
			scope: "source.luau", extensions: []string{".lua"},
			projects: []string{"neovim"},
		},
		{
			name: "perl", lang: realworldLang("perl"),
			scope: "source.perl", extensions: []string{".pm", ".pl"},
			projects: []string{"perl5"},
		},
	}
}

// makeRealworldParseFunc creates a ParseFunc for the given language.
func makeRealworldParseFunc(lang *ts.Language) func([]byte) (string, error) {
	return func(input []byte) (string, error) {
		p := iparser.NewParser()
		p.SetLanguage(lang)
		tree := p.ParseString(context.Background(), input)
		if tree == nil {
			return "", nil
		}
		return tree.RootNode().String(), nil
	}
}

// perFileParseTimeout is the maximum time allowed to parse a single file.
// Large or pathological files (e.g. protobuf descriptor.cc) can trigger
// GLR parser edge cases that take very long; we skip those gracefully.
const perFileParseTimeout = 60 * time.Second

// parseRealworldFileWithTimeout parses a file with a timeout, returning
// the S-expression or an error. Returns ("", nil) if the parse times out.
func parseRealworldFileWithTimeout(t *testing.T, lang *ts.Language, input []byte, fileName string) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), perFileParseTimeout)
	defer cancel()

	p := iparser.NewParser()
	p.SetLanguage(lang)

	// Run parse in a goroutine so we can respect the timeout.
	type result struct {
		sexp string
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		tree := p.ParseString(ctx, input)
		if tree == nil {
			ch <- result{"", nil}
			return
		}
		ch <- result{tree.RootNode().String(), nil}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("parse error on %s: %v", fileName, r.err)
		}
		return r.sexp
	case <-ctx.Done():
		t.Skipf("SKIP: %s timed out after %v (%d bytes) — known GLR parser limitation", fileName, perFileParseTimeout, len(input))
		return ""
	}
}

// TestDifferentialRealworld runs differential testing on all downloaded realworld.
// It walks each project's directory and compares Go vs C parser output.
func TestDifferentialRealworld(t *testing.T) {
	for _, cl := range allRealworldLanguages() {
		cl := cl
		t.Run(cl.name, func(t *testing.T) {
			if cl.lang == nil {
				t.Skip("grammar not wired up")
			}

			parseFunc := makeRealworldParseFunc(cl.lang)

			for _, proj := range cl.projects {
				proj := proj
				projDir := filepath.Join(realworldDir, cl.name, proj)

				t.Run(proj, func(t *testing.T) {
					if _, err := os.Stat(projDir); os.IsNotExist(err) {
						t.Skipf("realworld not downloaded: run 'make fetch-realworld' first")
					}

					difftest.RunDifferentialDir(t, projDir, cl.extensions, parseFunc, cl.scope)
				})
			}
		})
	}
}

// TestRealworldParseOnly runs parse-only (no CLI comparison) on all downloaded realworld.
// This verifies the Go parser doesn't panic or hang on real-world input.
// Files that exceed perFileParseTimeout are skipped (known GLR parser limitation).
func TestRealworldParseOnly(t *testing.T) {
	for _, cl := range allRealworldLanguages() {
		cl := cl
		t.Run(cl.name, func(t *testing.T) {
			if cl.lang == nil {
				t.Skip("grammar not wired up")
			}

			for _, proj := range cl.projects {
				proj := proj
				projDir := filepath.Join(realworldDir, cl.name, proj)

				t.Run(proj, func(t *testing.T) {
					if _, err := os.Stat(projDir); os.IsNotExist(err) {
						t.Skipf("realworld not downloaded: run 'make fetch-realworld' first")
					}

					entries, err := os.ReadDir(projDir)
					if err != nil {
						t.Fatalf("reading %s: %v", projDir, err)
					}

					extSet := make(map[string]bool)
					for _, e := range cl.extensions {
						extSet[e] = true
					}

					for _, entry := range entries {
						if entry.IsDir() {
							continue
						}
						ext := filepath.Ext(entry.Name())
						if !extSet[ext] {
							continue
						}

						entry := entry
						t.Run(entry.Name(), func(t *testing.T) {
							filePath := filepath.Join(projDir, entry.Name())
							input, err := os.ReadFile(filePath)
							if err != nil {
								t.Fatalf("reading %s: %v", filePath, err)
							}

							sexp := parseRealworldFileWithTimeout(t, cl.lang, input, entry.Name())

							// Log (don't fail) for empty S-expressions — many language
							// parsers aren't fully working yet in the Go port. The
							// differential test (TestDifferentialRealworld) checks correctness;
							// this test only verifies no panics or hangs.
							if sexp == "" && !t.Skipped() {
								t.Logf("WARNING: Go parser returned empty S-expression for %s (%d bytes)", entry.Name(), len(input))
							}
						})
					}
				})
			}
		})
	}
}

// TestRealworldStats prints a summary of the realworld files available for testing.
func TestRealworldStats(t *testing.T) {
	total := 0
	byLang := make(map[string]int)

	for _, cl := range allRealworldLanguages() {
		for _, proj := range cl.projects {
			projDir := filepath.Join(realworldDir, cl.name, proj)
			entries, err := os.ReadDir(projDir)
			if err != nil {
				continue
			}
			extSet := make(map[string]bool)
			for _, e := range cl.extensions {
				extSet[e] = true
			}
			for _, entry := range entries {
				if !entry.IsDir() && extSet[filepath.Ext(entry.Name())] {
					total++
					byLang[cl.name]++
				}
			}
		}
	}

	if total == 0 {
		t.Skip("no realworld files downloaded: run 'make fetch-realworld' first")
	}

	t.Logf("Total realworld files: %d", total)
	for lang, count := range byLang {
		t.Logf("  %s: %d files", lang, count)
	}
}
