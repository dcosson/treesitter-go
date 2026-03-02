package tsx

import (
	ts "github.com/treesitter-go/treesitter"
	grammar "github.com/treesitter-go/treesitter/internal/grammars/tsx"
	scanner "github.com/treesitter-go/treesitter/internal/scanners/typescript"
)

func Language() *ts.Language {
	l := grammar.TsxLanguage()
	l.NewExternalScanner = scanner.New
	return l
}
