package golang

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/golang"
)

func Language() *ts.Language {
	return grammar.GoLanguage()
}
