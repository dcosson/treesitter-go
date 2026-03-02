package ruby

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/ruby"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/ruby"
)

func Language() *ts.Language {
	l := grammar.RubyLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
