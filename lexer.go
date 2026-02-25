package treesitter

import ilexer "github.com/treesitter-go/treesitter/internal/lexer"

type Input = ilexer.Input
type StringInput = ilexer.StringInput

type Lexer = ilexer.Lexer

func NewStringInput(data []byte) *StringInput { return ilexer.NewStringInput(data) }
func NewLexer() *Lexer                        { return ilexer.NewLexer() }
