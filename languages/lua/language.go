package lua

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/lua"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/lua"
)

func Language() *ts.Language {
	l := grammar.LuaLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
