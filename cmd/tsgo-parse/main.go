// tsgo-parse is a minimal CLI for parsing files with the Go tree-sitter runtime.
// It is used by benchmark tests for fair subprocess-based comparison against
// the C tree-sitter CLI.
//
// Usage: tsgo-parse -lang <name> <file>
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	ts "github.com/dcosson/treesitter-go"
	iparser "github.com/dcosson/treesitter-go/parser"

	bashgrammar "github.com/dcosson/treesitter-go/internal/grammars/bash"
	cgrammar "github.com/dcosson/treesitter-go/internal/grammars/c"
	cppgrammar "github.com/dcosson/treesitter-go/internal/grammars/cpp"
	cssgrammar "github.com/dcosson/treesitter-go/internal/grammars/css"
	golanggrammar "github.com/dcosson/treesitter-go/internal/grammars/golang"
	htmlgrammar "github.com/dcosson/treesitter-go/internal/grammars/html"
	javagrammar "github.com/dcosson/treesitter-go/internal/grammars/java"
	jsgrammar "github.com/dcosson/treesitter-go/internal/grammars/javascript"
	jsongrammar "github.com/dcosson/treesitter-go/internal/grammars/json"
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

var langFlag = flag.String("lang", "", "language name (must match grammars.json)")

func main() {
	flag.Parse()
	if *langFlag == "" || flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "usage: tsgo-parse -lang <name> <file>\n")
		os.Exit(2)
	}

	lang := getLanguage(*langFlag)
	if lang == nil {
		fmt.Fprintf(os.Stderr, "unknown language: %s\n", *langFlag)
		os.Exit(2)
	}

	input, err := os.ReadFile(flag.Arg(0))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading file: %v\n", err)
		os.Exit(1)
	}

	parser := iparser.NewParser()
	parser.SetLanguage(lang)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	tree := parser.ParseString(ctx, input)
	if tree == nil {
		fmt.Fprintf(os.Stderr, "parse returned nil\n")
		os.Exit(1)
	}

	fmt.Println(tree.RootNode().String())
}

func getLanguage(name string) *ts.Language {
	switch name {
	case "json":
		return jsongrammar.JsonLanguage()
	case "go":
		return golanggrammar.GoLanguage()
	case "python":
		l := pygrammar.PythonLanguage()
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
	case "perl":
		l := perlgrammar.PerlLanguage()
		l.NewExternalScanner = perlscanner.New
		return l
	case "lua":
		l := luagrammar.LuaLanguage()
		l.NewExternalScanner = luascanner.New
		return l
	default:
		return nil
	}
}
