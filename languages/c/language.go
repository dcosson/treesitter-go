package c

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/c"
)

func Language() *ts.Language {
	return grammar.CLanguage()
}
