package css

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/css"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/css"
)

func Language() *ts.Language {
	l := grammar.CssLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
