package ruby

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/ruby"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/ruby"
)

func Language() *ts.Language {
	l := grammar.RubyLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
