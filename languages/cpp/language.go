package cpp

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/cpp"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/cpp"
)

func Language() *ts.Language {
	l := grammar.CppLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
