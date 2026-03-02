package perl

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/perl"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/perl"
)

func Language() *ts.Language {
	l := grammar.PerlLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
