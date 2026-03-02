package rust

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/rust"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/rust"
)

func Language() *ts.Language {
	l := grammar.RustLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
