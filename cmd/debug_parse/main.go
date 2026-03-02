package main

import (
	"context"
	"fmt"
	iparser "github.com/dcosson/treesitter-go/parser"
	"time"

	perlgrammar "github.com/dcosson/treesitter-go/internal/grammars/perl"
	perlscanner "github.com/dcosson/treesitter-go/internal/scanners/perl"
)

func main() {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New

	p := iparser.NewParser()
	p.SetLanguage(lang)

	// Perl non-assoc: chaining cmp triggers error
	src := `12 cmp 34 cmp 56;`
	fmt.Printf("Input: %q\n\n", src)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		fmt.Println("RESULT: tree is nil")
	} else {
		root := tree.RootNode()
		fmt.Printf("RESULT: sexp = %s\n", root.String())
	}
}
