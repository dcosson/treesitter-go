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

	ts "github.com/treesitter-go/treesitter"
	iparser "github.com/treesitter-go/treesitter/parser"

	jsongrammar "github.com/treesitter-go/treesitter/internal/grammars/json"
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
