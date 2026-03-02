package java

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/java"
)

func Language() *ts.Language {
	return grammar.JavaLanguage()
}
