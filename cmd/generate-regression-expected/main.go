// Command generate-regression-expected parses regression test .input files
// with the Go parser and writes the resulting S-expressions as .expected files.
//
// Usage:
//
//	go run ./cmd/generate-regression-expected
//
// Only writes .expected files that don't already exist to avoid overwriting
// manually curated expectations. Use -force to overwrite all.
package main

import (
	"context"
	"flag"
	"fmt"
	iparser "github.com/treesitter-go/treesitter/parser"
	"os"
	"path/filepath"
	"strings"
	"time"

	ts "github.com/treesitter-go/treesitter"
	bashgrammar "github.com/treesitter-go/treesitter/internal/grammars/bash"
	cgrammar "github.com/treesitter-go/treesitter/internal/grammars/c"
	cppgrammar "github.com/treesitter-go/treesitter/internal/grammars/cpp"
	cssgrammar "github.com/treesitter-go/treesitter/internal/grammars/css"
	golanggrammar "github.com/treesitter-go/treesitter/internal/grammars/golang"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/grammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/grammars/java"
	jsgrammar "github.com/treesitter-go/treesitter/internal/grammars/javascript"
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

var force = flag.Bool("force", false, "overwrite existing .expected files")

func main() {
	flag.Parse()

	languages := map[string]*ts.Language{
		"go":         golanggrammar.GoLanguage(),
		"c":          cgrammar.CLanguage(),
		"java":       javagrammar.JavaLanguage(),
		"python":     withScanner(pygrammar.PythonLanguage(), pyscanner.New),
		"javascript": withScanner(jsgrammar.JavascriptLanguage(), jsscanner.New),
		"typescript": withScanner(tsgrammar.TypescriptLanguage(), tsscanner.New),
		"bash":       withScanner(bashgrammar.BashLanguage(), bashscanner.New),
		"ruby":       withScanner(rubygrammar.RubyLanguage(), rubyscanner.New),
		"rust":       withScanner(rustgrammar.RustLanguage(), rustscanner.New),
		"cpp":        withScanner(cppgrammar.CppLanguage(), cppscanner.New),
		"css":        withScanner(cssgrammar.CssLanguage(), cssscanner.New),
		"html":       withScanner(htmlgrammar.HtmlLanguage(), htmlscanner.New),
		"perl":       withScanner(perlgrammar.PerlLanguage(), perlscanner.New),
		"lua":        withScanner(luagrammar.LuaLanguage(), luascanner.New),
	}

	baseDir := "testdata/regressions"
	totalGenerated := 0

	for langName, lang := range languages {
		langDir := filepath.Join(baseDir, langName)
		if _, err := os.Stat(langDir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(langDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", langDir, err)
			continue
		}

		for _, entry := range entries {
			if !strings.HasSuffix(entry.Name(), ".input") {
				continue
			}

			baseName := strings.TrimSuffix(entry.Name(), ".input")
			inputPath := filepath.Join(langDir, entry.Name())
			expectedPath := filepath.Join(langDir, baseName+".expected")

			if !*force {
				if _, err := os.Stat(expectedPath); err == nil {
					continue // Already exists, skip.
				}
			}

			input, err := os.ReadFile(inputPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error reading %s: %v\n", inputPath, err)
				continue
			}

			sexp := parse(lang, input)
			if sexp == "" {
				fmt.Fprintf(os.Stderr, "SKIP %s/%s: nil tree (timeout or parse failure)\n", langName, baseName)
				continue
			}

			if err := os.WriteFile(expectedPath, []byte(sexp+"\n"), 0o644); err != nil {
				fmt.Fprintf(os.Stderr, "error writing %s: %v\n", expectedPath, err)
				continue
			}
			fmt.Printf("WROTE %s/%s.expected\n", langName, baseName)
			totalGenerated++
		}
	}
	fmt.Printf("\ntotal: %d expected files generated\n", totalGenerated)
}

func withScanner(lang *ts.Language, factory ts.ExternalScannerFactory) *ts.Language {
	lang.NewExternalScanner = factory
	return lang
}

func parse(lang *ts.Language, input []byte) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p := iparser.NewParser()
	p.SetLanguage(lang)
	tree := p.ParseString(ctx, input)
	if tree == nil {
		return ""
	}
	return tree.RootNode().String()
}
