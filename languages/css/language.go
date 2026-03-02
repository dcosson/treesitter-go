package css

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/css"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/css"
)

func Language() *ts.Language {
	l := grammar.CssLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
