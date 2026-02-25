package parser

import (
	"context"

	ts "github.com/treesitter-go/treesitter"
	iparser "github.com/treesitter-go/treesitter/internal/parser"
	plexer "github.com/treesitter-go/treesitter/lexer"
)

// Parser is the public facade over the internal parser implementation.
type Parser struct {
	inner *iparser.Parser
}

// NewParser creates a parser facade backed by internal/parser.
func NewParser() *Parser {
	return &Parser{inner: iparser.NewParser()}
}

// SetDebug enables debug trace output.
func (p *Parser) SetDebug(on bool) {
	p.inner.SetDebug(on)
}

// SetLanguage sets the grammar language.
func (p *Parser) SetLanguage(lang *ts.Language) {
	p.inner.SetLanguage(lang)
}

// Language returns the currently configured language.
func (p *Parser) Language() *ts.Language {
	return p.inner.Language()
}

// Reset clears parser state.
func (p *Parser) Reset() {
	p.inner.Reset()
}

// Parse parses input with optional old tree for incremental parsing.
func (p *Parser) Parse(ctx context.Context, input plexer.Input, oldTree *ts.Tree) *ts.Tree {
	return p.inner.Parse(ctx, input, oldTree)
}

// ParseString parses source bytes with optional old tree.
func (p *Parser) ParseString(ctx context.Context, source []byte, oldTree ...*ts.Tree) *ts.Tree {
	return p.inner.ParseString(ctx, source, oldTree...)
}
