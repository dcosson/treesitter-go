package java

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/java"
)

func Language() *ts.Language {
	return grammar.JavaLanguage()
}
