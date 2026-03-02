package lua

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/lua"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/lua"
)

func Language() *ts.Language {
	l := grammar.LuaLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
