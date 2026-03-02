package cpp

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/cpp"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/cpp"
)

func Language() *ts.Language {
	l := grammar.CppLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
