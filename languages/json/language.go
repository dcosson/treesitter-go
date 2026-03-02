// Package json provides the JSON tree-sitter language for parsing.
package json

import (
	ts "github.com/dcosson/treesitter-go"
	grammar "github.com/dcosson/treesitter-go/internal/grammars/json"
)

// Language returns a fully configured JSON language ready for parsing.
func Language() *ts.Language {
	l := grammar.JsonLanguage()
	l.LexFn = lexFn
	return l
}

// lexFn is a hand-written lex function for the JSON grammar.
func lexFn(lexer *ts.Lexer, state ts.StateID) bool {
	// Skip whitespace.
	for !lexer.EOF() && (lexer.Lookahead == ' ' || lexer.Lookahead == '\t' ||
		lexer.Lookahead == '\n' || lexer.Lookahead == '\r') {
		lexer.Skip()
	}

	if lexer.EOF() {
		return false
	}

	ch := lexer.Lookahead

	// String content mode (lex state 1): inside a string.
	if state == 1 {
		return lexStringContent(lexer)
	}

	switch {
	case ch == '{':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymLBrace))
		return true
	case ch == '}':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymRBrace))
		return true
	case ch == '[':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymLBrack))
		return true
	case ch == ']':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymRBrack))
		return true
	case ch == ',':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymComma))
		return true
	case ch == ':':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymColon))
		return true
	case ch == '"':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymDQuote))
		return true
	case ch == 't':
		return lexKW(lexer, "true", ts.Symbol(grammar.SymTrue))
	case ch == 'f':
		return lexKW(lexer, "false", ts.Symbol(grammar.SymFalse))
	case ch == 'n':
		return lexKW(lexer, "null", ts.Symbol(grammar.SymNull))
	case ch == '-' || (ch >= '0' && ch <= '9'):
		return lexNum(lexer)
	case ch == '/':
		return lexCmt(lexer)
	}

	return false
}

func lexStringContent(lexer *ts.Lexer) bool {
	if lexer.EOF() {
		return false
	}
	ch := lexer.Lookahead
	if ch == '\\' {
		lexer.Advance(false)
		if !lexer.EOF() {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymEscapeSequence))
		return true
	}
	if ch == '"' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymDQuote))
		return true
	}
	for !lexer.EOF() && lexer.Lookahead != '"' && lexer.Lookahead != '\\' {
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	lexer.AcceptToken(ts.Symbol(grammar.SymStringContent))
	return true
}

func lexKW(lexer *ts.Lexer, keyword string, symbol ts.Symbol) bool {
	for _, expected := range keyword {
		if lexer.EOF() || lexer.Lookahead != expected {
			return false
		}
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	lexer.AcceptToken(symbol)
	return true
}

func lexNum(lexer *ts.Lexer) bool {
	if lexer.Lookahead == '-' {
		lexer.Advance(false)
	}
	if lexer.EOF() || lexer.Lookahead < '0' || lexer.Lookahead > '9' {
		return false
	}
	for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
		lexer.Advance(false)
	}
	if !lexer.EOF() && lexer.Lookahead == '.' {
		lexer.Advance(false)
		for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
			lexer.Advance(false)
		}
	}
	if !lexer.EOF() && (lexer.Lookahead == 'e' || lexer.Lookahead == 'E') {
		lexer.Advance(false)
		if !lexer.EOF() && (lexer.Lookahead == '+' || lexer.Lookahead == '-') {
			lexer.Advance(false)
		}
		for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
			lexer.Advance(false)
		}
	}
	lexer.MarkEnd()
	lexer.AcceptToken(ts.Symbol(grammar.SymNumber))
	return true
}

func lexCmt(lexer *ts.Lexer) bool {
	if lexer.Lookahead != '/' {
		return false
	}
	lexer.Advance(false)
	if lexer.EOF() {
		return false
	}
	if lexer.Lookahead == '/' {
		for !lexer.EOF() && lexer.Lookahead != '\n' {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymComment))
		return true
	}
	if lexer.Lookahead == '*' {
		lexer.Advance(false)
		for !lexer.EOF() {
			if lexer.Lookahead == '*' {
				lexer.Advance(false)
				if !lexer.EOF() && lexer.Lookahead == '/' {
					lexer.Advance(false)
					lexer.MarkEnd()
					lexer.AcceptToken(ts.Symbol(grammar.SymComment))
					return true
				}
			} else {
				lexer.Advance(false)
			}
		}
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(grammar.SymComment))
		return true
	}
	return false
}
