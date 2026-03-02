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
	iparser "github.com/dcosson/treesitter-go/parser"
	"os"
	"path/filepath"
	"strings"
	"time"

	ts "github.com/dcosson/treesitter-go"
	bashgrammar "github.com/dcosson/treesitter-go/internal/grammars/bash"
	cgrammar "github.com/dcosson/treesitter-go/internal/grammars/c"
	cppgrammar "github.com/dcosson/treesitter-go/internal/grammars/cpp"
	cssgrammar "github.com/dcosson/treesitter-go/internal/grammars/css"
	golanggrammar "github.com/dcosson/treesitter-go/internal/grammars/golang"
	htmlgrammar "github.com/dcosson/treesitter-go/internal/grammars/html"
	javagrammar "github.com/dcosson/treesitter-go/internal/grammars/java"
	jsgrammar "github.com/dcosson/treesitter-go/internal/grammars/javascript"
	luagrammar "github.com/dcosson/treesitter-go/internal/grammars/lua"
	perlgrammar "github.com/dcosson/treesitter-go/internal/grammars/perl"
	pygrammar "github.com/dcosson/treesitter-go/internal/grammars/python"
	rubygrammar "github.com/dcosson/treesitter-go/internal/grammars/ruby"
	rustgrammar "github.com/dcosson/treesitter-go/internal/grammars/rust"
	tsgrammar "github.com/dcosson/treesitter-go/internal/grammars/typescript"
	bashscanner "github.com/dcosson/treesitter-go/internal/scanners/bash"
	cppscanner "github.com/dcosson/treesitter-go/internal/scanners/cpp"
	cssscanner "github.com/dcosson/treesitter-go/internal/scanners/css"
	htmlscanner "github.com/dcosson/treesitter-go/internal/scanners/html"
	jsscanner "github.com/dcosson/treesitter-go/internal/scanners/javascript"
	luascanner "github.com/dcosson/treesitter-go/internal/scanners/lua"
	perlscanner "github.com/dcosson/treesitter-go/internal/scanners/perl"
	pyscanner "github.com/dcosson/treesitter-go/internal/scanners/python"
	rubyscanner "github.com/dcosson/treesitter-go/internal/scanners/ruby"
	rustscanner "github.com/dcosson/treesitter-go/internal/scanners/rust"
	tsscanner "github.com/dcosson/treesitter-go/internal/scanners/typescript"
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
