package javascript

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/javascript"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/javascript"
)

func Language() *ts.Language {
	l := grammar.JavascriptLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
