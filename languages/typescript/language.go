package typescript

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/typescript"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/typescript"
)

func Language() *ts.Language {
	l := grammar.TypescriptLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
