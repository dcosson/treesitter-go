package golang

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/golang"
)

func Language() *ts.Language {
	return grammar.GoLanguage()
}
