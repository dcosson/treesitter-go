package bash

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/bash"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/bash"
)

func Language() *ts.Language {
	l := grammar.BashLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
