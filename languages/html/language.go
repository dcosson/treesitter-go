package html

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/html"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/html"
)

func Language() *ts.Language {
	l := grammar.HtmlLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
