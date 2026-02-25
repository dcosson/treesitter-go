package treesitter

import plex "github.com/treesitter-go/treesitter/lexer"

type Input = plex.Input
type StringInput = plex.StringInput

type Lexer = plex.Lexer

func NewStringInput(data []byte) *StringInput { return plex.NewStringInput(data) }
func NewLexer() *Lexer                        { return plex.NewLexer() }
