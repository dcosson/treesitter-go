package c

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/c"
)

func Language() *ts.Language {
	return grammar.CLanguage()
}
