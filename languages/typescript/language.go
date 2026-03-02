package typescript

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/typescript"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/typescript"
)

func Language() *ts.Language {
	l := grammar.TypescriptLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
