package tsx

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/tsx"
	scanner "github.com/dcosson/treesitter-go/internal/scanners/typescript"
)

func Language() *ts.Language {
	l := grammar.TsxLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
