package python

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/python"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/python"
)

func Language() *ts.Language {
	l := grammar.PythonLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
