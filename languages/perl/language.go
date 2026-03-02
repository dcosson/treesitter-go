package perl

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/perl"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/perl"
)

func Language() *ts.Language {
	l := grammar.PerlLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
