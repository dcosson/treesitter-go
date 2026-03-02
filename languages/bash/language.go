package bash

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/bash"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/bash"
)

func Language() *ts.Language {
	l := grammar.BashLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
