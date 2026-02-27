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
		return tg.JsonLanguage()
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
