// Command tsgo-generate compiles tree-sitter grammars to Go packages.
//
// It reads a tree-sitter grammar's generated parser.c file (from `tree-sitter generate`)
// and produces a Go source file that creates a Language with compiled parse tables,
// lex functions, symbol metadata, field maps, and alias sequences.
//
// Usage:
//
//	tsgo-generate -parser src/parser.c -package jsongrammar -output json_language.go
//
// The generated output is a self-contained Go file that imports the treesitter
// runtime package and defines a FooLanguage() function returning *ts.Language.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/dcosson/treesitter-go/internal/generate"
)

func main() {
	parserFile := flag.String("parser", "", "path to tree-sitter parser.c file (required)")
	packageName := flag.String("package", "", "Go package name for generated output (default: grammar name)")
	outputFile := flag.String("output", "", "output Go file path (default: stdout)")

	flag.Parse()

	if *parserFile == "" {
		fmt.Fprintf(os.Stderr, "error: -parser flag is required\n")
		flag.Usage()
		os.Exit(1)
	}

	// Read parser.c.
	data, err := os.ReadFile(*parserFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading parser.c: %v\n", err)
		os.Exit(1)
	}

	// Extract grammar from parser.c.
	grammar, err := generate.ExtractGrammar(string(data))
	if err != nil {
		fmt.Fprintf(os.Stderr, "error extracting grammar: %v\n", err)
		os.Exit(1)
	}

	// Determine package name.
	pkg := *packageName
	if pkg == "" {
		pkg = grammar.Name + "grammar"
	}

	// Generate Go source.
	goSrc := generate.GenerateGo(grammar, pkg)

	// Write output.
	if *outputFile == "" {
		fmt.Print(goSrc)
	} else {
		if err := os.WriteFile(*outputFile, []byte(goSrc), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing output: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "generated %s (%d bytes)\n", *outputFile, len(goSrc))
	}
}
