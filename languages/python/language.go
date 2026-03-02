package python

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/python"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/python"
)

func Language() *ts.Language {
	l := grammar.PythonLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
