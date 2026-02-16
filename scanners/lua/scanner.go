// Package lua provides the external scanner for tree-sitter-lua.
//
// Ported from tree-sitter-lua/src/scanner.c.
// The Lua external scanner handles 6 context-sensitive token types:
//   - BLOCK_COMMENT_START: Start of --[=*[ block comment
//   - BLOCK_COMMENT_CONTENT: Body of block comment
//   - BLOCK_COMMENT_END: End of ]=*] block comment
//   - BLOCK_STRING_START: Start of [=*[ block string
//   - BLOCK_STRING_CONTENT: Body of block string
//   - BLOCK_STRING_END: End of ]=*] block string
//
// Lua's block comments and block strings use a bracket notation with optional
// equal signs: [=[ ... ]=], [==[ ... ]==], etc. The number of equal signs must
// match between the opening and closing delimiter.
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
// All EOF checks use lexer.EOF() instead of checking for 0.
package lua

import (
	"unicode"

	ts "github.com/treesitter-go/treesitter"
)

// External token type indices (must match grammar.js externals array order).
const (
	BlockCommentStart   = iota // 0
	BlockCommentContent        // 1
	BlockCommentEnd            // 2
	BlockStringStart           // 3
	BlockStringContent         // 4
	BlockStringEnd             // 5
)

// Scanner implements ts.ExternalScanner for Lua.
type Scanner struct {
	endingChar byte
	levelCount uint8
}

// New creates a new Lua external scanner (ts.ExternalScannerFactory).
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner state to buf. Returns 2 (bytes written).
func (s *Scanner) Serialize(buf []byte) uint32 {
	if len(buf) < 2 {
		return 0
	}
	buf[0] = s.endingChar
	buf[1] = s.levelCount
	return 2
}

// Deserialize restores the scanner state from data.
func (s *Scanner) Deserialize(data []byte) {
	s.endingChar = 0
	s.levelCount = 0
	if len(data) == 0 {
		return
	}
	s.endingChar = data[0]
	if len(data) == 1 {
		return
	}
	s.levelCount = data[1]
}

func (s *Scanner) resetState() {
	s.endingChar = 0
	s.levelCount = 0
}

// consumeChar checks if the lookahead matches c, consumes it and returns true.
// Returns false if the lookahead doesn't match.
func consumeChar(c rune, lexer *ts.Lexer) bool {
	if lexer.Lookahead != c {
		return false
	}
	lexer.Advance(false)
	return true
}

// consumeAndCountChar consumes consecutive occurrences of c and returns the count.
func consumeAndCountChar(c rune, lexer *ts.Lexer) uint8 {
	var count uint8
	for lexer.Lookahead == c {
		count++
		lexer.Advance(false)
	}
	return count
}

// skipWhitespaces advances past whitespace characters without recording them.
func skipWhitespaces(lexer *ts.Lexer) {
	for !lexer.EOF() && unicode.IsSpace(lexer.Lookahead) {
		lexer.Advance(true)
	}
}

// scanBlockStart attempts to match [=*[ and records the level count.
func (s *Scanner) scanBlockStart(lexer *ts.Lexer) bool {
	if consumeChar('[', lexer) {
		level := consumeAndCountChar('=', lexer)
		if consumeChar('[', lexer) {
			s.levelCount = level
			return true
		}
	}
	return false
}

// scanBlockEnd attempts to match ]=*] with the same level count.
func (s *Scanner) scanBlockEnd(lexer *ts.Lexer) bool {
	if consumeChar(']', lexer) {
		level := consumeAndCountChar('=', lexer)
		if s.levelCount == level && consumeChar(']', lexer) {
			return true
		}
	}
	return false
}

// scanBlockContent scans content until the matching block end delimiter.
func (s *Scanner) scanBlockContent(lexer *ts.Lexer) bool {
	for !lexer.EOF() {
		if lexer.Lookahead == ']' {
			lexer.MarkEnd()
			if s.scanBlockEnd(lexer) {
				return true
			}
		} else {
			lexer.Advance(false)
		}
	}
	return false
}

// scanCommentStart matches --[=*[ for block comments.
func (s *Scanner) scanCommentStart(lexer *ts.Lexer) bool {
	if consumeChar('-', lexer) && consumeChar('-', lexer) {
		lexer.MarkEnd()
		if s.scanBlockStart(lexer) {
			lexer.MarkEnd()
			lexer.ResultSymbol = ts.Symbol(BlockCommentStart)
			return true
		}
	}
	return false
}

// scanCommentContent scans block comment or line comment content.
func (s *Scanner) scanCommentContent(lexer *ts.Lexer) bool {
	if s.endingChar == 0 { // block comment
		if s.scanBlockContent(lexer) {
			lexer.ResultSymbol = ts.Symbol(BlockCommentContent)
			return true
		}
		return false
	}

	for !lexer.EOF() {
		if lexer.Lookahead == rune(s.endingChar) {
			s.resetState()
			lexer.ResultSymbol = ts.Symbol(BlockCommentContent)
			return true
		}
		lexer.Advance(false)
	}
	return false
}

// Scan dispatches to the appropriate scanning function based on valid_symbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	if len(validSymbols) <= BlockStringEnd {
		return false
	}

	if validSymbols[BlockStringEnd] && s.scanBlockEnd(lexer) {
		s.resetState()
		lexer.ResultSymbol = ts.Symbol(BlockStringEnd)
		return true
	}

	if validSymbols[BlockStringContent] && s.scanBlockContent(lexer) {
		lexer.ResultSymbol = ts.Symbol(BlockStringContent)
		return true
	}

	if validSymbols[BlockCommentEnd] && s.endingChar == 0 && s.scanBlockEnd(lexer) {
		s.resetState()
		lexer.ResultSymbol = ts.Symbol(BlockCommentEnd)
		return true
	}

	if validSymbols[BlockCommentContent] && s.scanCommentContent(lexer) {
		return true
	}

	skipWhitespaces(lexer)

	if validSymbols[BlockStringStart] && s.scanBlockStart(lexer) {
		lexer.ResultSymbol = ts.Symbol(BlockStringStart)
		return true
	}

	if validSymbols[BlockCommentStart] {
		if s.scanCommentStart(lexer) {
			return true
		}
	}

	return false
}
