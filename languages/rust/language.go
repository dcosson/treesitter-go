package rust

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/rust"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/rust"
)

func Language() *ts.Language {
	l := grammar.RustLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
