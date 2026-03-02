package html

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/html"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/html"
)

func Language() *ts.Language {
	l := grammar.HtmlLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
