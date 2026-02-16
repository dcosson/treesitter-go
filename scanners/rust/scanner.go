// Package rust provides the external scanner for tree-sitter-rust.
//
// Ported from tree-sitter-rust/src/scanner.c.
// The Rust external scanner handles 10 context-sensitive token types:
//   - STRING_CONTENT: Body content of regular string literals
//   - RAW_STRING_LITERAL_START: r#"..." opening (with hash count)
//   - RAW_STRING_LITERAL_CONTENT: Body of raw string literals
//   - RAW_STRING_LITERAL_END: Closing "# sequence
//   - FLOAT_LITERAL: Float literals (disambiguated from integer.method)
//   - BLOCK_OUTER_DOC_MARKER: /** marker (not /*** or more)
//   - BLOCK_INNER_DOC_MARKER: /*! marker
//   - BLOCK_COMMENT_CONTENT: Content of block comments (with nesting)
//   - LINE_DOC_CONTENT: Content of line doc comments
//   - ERROR_SENTINEL: Error recovery detection
//
// The scanner tracks the opening hash count for raw string literals.
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
package rust

import (
	"unicode"

	ts "github.com/treesitter-go/treesitter"
)

// Token types matching the externals array in grammar.js.
const (
	StringContent = iota
	RawStringLiteralStart
	RawStringLiteralContent
	RawStringLiteralEnd
	FloatLiteral
	BlockOuterDocMarker
	BlockInnerDocMarker
	BlockCommentContent
	LineDocContent
	ErrorSentinel
)

// Scanner implements ts.ExternalScanner for Rust.
type Scanner struct {
	openingHashCount uint8
}

// New creates a new Rust external scanner (ts.ExternalScannerFactory).
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner state to buf.
func (s *Scanner) Serialize(buf []byte) uint32 {
	if len(buf) < 1 {
		return 0
	}
	buf[0] = s.openingHashCount
	return 1
}

// Deserialize restores the scanner state from data.
func (s *Scanner) Deserialize(data []byte) {
	s.openingHashCount = 0
	if len(data) == 1 {
		s.openingHashCount = data[0]
	}
}

// Scan dispatches to the appropriate scanning function based on validSymbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	// Error recovery: bail if all symbols are valid.
	if validSymbols[ErrorSentinel] {
		return false
	}

	if validSymbols[BlockCommentContent] || validSymbols[BlockInnerDocMarker] ||
		validSymbols[BlockOuterDocMarker] {
		return s.processBlockComment(lexer, validSymbols)
	}

	if validSymbols[StringContent] && !validSymbols[FloatLiteral] {
		return processString(lexer)
	}

	if validSymbols[LineDocContent] {
		return processLineDocContent(lexer)
	}

	for isSpace(lexer.Lookahead) {
		skip(lexer)
	}

	if validSymbols[RawStringLiteralStart] &&
		(lexer.Lookahead == 'r' || lexer.Lookahead == 'b' || lexer.Lookahead == 'c') {
		return s.scanRawStringStart(lexer)
	}

	if validSymbols[RawStringLiteralContent] {
		return s.scanRawStringContent(lexer)
	}

	if validSymbols[RawStringLiteralEnd] && lexer.Lookahead == '"' {
		return s.scanRawStringEnd(lexer)
	}

	if validSymbols[FloatLiteral] && isDigit(lexer.Lookahead) {
		return processFloatLiteral(lexer)
	}

	return false
}

func processString(lexer *ts.Lexer) bool {
	hasContent := false
	for {
		if lexer.Lookahead == '"' || lexer.Lookahead == '\\' {
			break
		}
		if lexer.EOF() {
			return false
		}
		hasContent = true
		advance(lexer)
	}
	lexer.ResultSymbol = StringContent
	lexer.MarkEnd()
	return hasContent
}

func (s *Scanner) scanRawStringStart(lexer *ts.Lexer) bool {
	if lexer.Lookahead == 'b' || lexer.Lookahead == 'c' {
		advance(lexer)
	}
	if lexer.Lookahead != 'r' {
		return false
	}
	advance(lexer)

	var openingHashCount uint8
	for lexer.Lookahead == '#' {
		advance(lexer)
		openingHashCount++
	}

	if lexer.Lookahead != '"' {
		return false
	}
	advance(lexer)
	s.openingHashCount = openingHashCount

	lexer.ResultSymbol = RawStringLiteralStart
	return true
}

func (s *Scanner) scanRawStringContent(lexer *ts.Lexer) bool {
	for {
		if lexer.EOF() {
			return false
		}
		if lexer.Lookahead == '"' {
			lexer.MarkEnd()
			advance(lexer)
			var hashCount uint8
			for lexer.Lookahead == '#' && hashCount < s.openingHashCount {
				advance(lexer)
				hashCount++
			}
			if hashCount == s.openingHashCount {
				lexer.ResultSymbol = RawStringLiteralContent
				return true
			}
		} else {
			advance(lexer)
		}
	}
}

func (s *Scanner) scanRawStringEnd(lexer *ts.Lexer) bool {
	advance(lexer) // consume closing "
	for i := uint8(0); i < s.openingHashCount; i++ {
		advance(lexer) // consume each #
	}
	lexer.ResultSymbol = RawStringLiteralEnd
	return true
}

func processFloatLiteral(lexer *ts.Lexer) bool {
	lexer.ResultSymbol = FloatLiteral

	advance(lexer)
	for isNumChar(lexer.Lookahead) {
		advance(lexer)
	}

	hasFraction := false
	hasExponent := false

	if lexer.Lookahead == '.' {
		hasFraction = true
		advance(lexer)
		if isAlpha(lexer.Lookahead) {
			// The dot is followed by a letter: 1.max(2) => not a float but an integer
			return false
		}
		if lexer.Lookahead == '.' {
			return false
		}
		for isNumChar(lexer.Lookahead) {
			advance(lexer)
		}
	}

	lexer.MarkEnd()

	if lexer.Lookahead == 'e' || lexer.Lookahead == 'E' {
		hasExponent = true
		advance(lexer)
		if lexer.Lookahead == '+' || lexer.Lookahead == '-' {
			advance(lexer)
		}
		if !isNumChar(lexer.Lookahead) {
			return true
		}
		advance(lexer)
		for isNumChar(lexer.Lookahead) {
			advance(lexer)
		}
		lexer.MarkEnd()
	}

	if !hasExponent && !hasFraction {
		return false
	}

	if lexer.Lookahead != 'u' && lexer.Lookahead != 'i' && lexer.Lookahead != 'f' {
		return true
	}
	advance(lexer)
	if !isDigit(lexer.Lookahead) {
		return true
	}

	for isDigit(lexer.Lookahead) {
		advance(lexer)
	}

	lexer.MarkEnd()
	return true
}

func processLineDocContent(lexer *ts.Lexer) bool {
	lexer.ResultSymbol = LineDocContent
	for {
		if lexer.EOF() {
			return true
		}
		if lexer.Lookahead == '\n' {
			advance(lexer)
			return true
		}
		advance(lexer)
	}
}

// Block comment state machine states.
const (
	bcLeftForwardSlash = iota
	bcLeftAsterisk
	bcContinuing
)

func (s *Scanner) processBlockComment(lexer *ts.Lexer, validSymbols []bool) bool {
	first := lexer.Lookahead

	if validSymbols[BlockInnerDocMarker] && first == '!' {
		lexer.ResultSymbol = BlockInnerDocMarker
		advance(lexer)
		return true
	}

	if validSymbols[BlockOuterDocMarker] && first == '*' {
		advance(lexer)
		lexer.MarkEnd()
		if lexer.Lookahead == '/' {
			return false
		}
		if lexer.Lookahead != '*' {
			lexer.ResultSymbol = BlockOuterDocMarker
			return true
		}
	} else {
		advance(lexer)
	}

	if validSymbols[BlockCommentContent] {
		state := bcContinuing
		nestingDepth := uint32(1)

		switch first {
		case '*':
			state = bcLeftAsterisk
			if lexer.Lookahead == '/' {
				return false
			}
		case '/':
			state = bcLeftForwardSlash
		default:
			state = bcContinuing
		}

		for !lexer.EOF() && nestingDepth != 0 {
			first = lexer.Lookahead
			switch state {
			case bcLeftForwardSlash:
				if first == '*' {
					nestingDepth++
				}
				state = bcContinuing
			case bcLeftAsterisk:
				if first == '*' {
					lexer.MarkEnd()
					state = bcLeftAsterisk
				} else {
					if first == '/' {
						nestingDepth--
					}
					state = bcContinuing
				}
			case bcContinuing:
				lexer.MarkEnd()
				switch first {
				case '/':
					state = bcLeftForwardSlash
				case '*':
					state = bcLeftAsterisk
				}
			}
			advance(lexer)
			if first == '/' && nestingDepth != 0 {
				lexer.MarkEnd()
			}
		}
		lexer.ResultSymbol = BlockCommentContent
		return true
	}

	return false
}

func advance(lexer *ts.Lexer) {
	lexer.Advance(false)
}

func skip(lexer *ts.Lexer) {
	lexer.Advance(true)
}

func isSpace(ch int32) bool {
	return ch >= 0 && unicode.IsSpace(rune(ch))
}

func isAlpha(ch int32) bool {
	return ch >= 0 && unicode.IsLetter(rune(ch))
}

func isDigit(ch int32) bool {
	return ch >= 0 && unicode.IsDigit(rune(ch))
}

func isNumChar(ch int32) bool {
	return ch == '_' || isDigit(ch)
}
