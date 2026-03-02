package javascript

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/javascript"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/javascript"
)

func Language() *ts.Language {
	l := grammar.JavascriptLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
