// Package css provides the external scanner for tree-sitter-css.
//
// Ported from tree-sitter-css/src/scanner.c.
// The CSS external scanner handles 3 context-sensitive token types:
//   - DESCENDANT_OP: Whitespace-as-operator in selectors (e.g., "div p")
//   - PSEUDO_CLASS_SELECTOR_COLON: The ":" in pseudo-class selectors vs property declarations
//   - ERROR_RECOVERY: Guard to disable scanning during error recovery
//
// This scanner is stateless — no serialization needed.
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
// All EOF checks use lexer.EOF() instead of checking for 0.
package css

import (
	"unicode"

	ts "github.com/treesitter-go/treesitter"
)

// External token type indices (must match grammar.js externals array order).
const (
	DescendantOp              = iota // 0
	PseudoClassSelectorColon        // 1
	ErrorRecovery                    // 2
)

// Scanner implements ts.ExternalScanner for CSS.
type Scanner struct{}

// New returns a new CSS external scanner.
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize is a no-op (stateless scanner).
func (s *Scanner) Serialize(buf []byte) uint32 {
	return 0
}

// Deserialize is a no-op (stateless scanner).
func (s *Scanner) Deserialize(data []byte) {}

// Scan attempts to recognize one of the 3 CSS external token types.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	if len(validSymbols) <= ErrorRecovery {
		return false
	}

	// During error recovery, disable external scanning.
	if validSymbols[ErrorRecovery] {
		return false
	}

	// Descendant operator: whitespace between selectors.
	if isSpace(lexer.Lookahead) && validSymbols[DescendantOp] {
		lexer.ResultSymbol = ts.Symbol(DescendantOp)

		lexer.Advance(true) // skip whitespace
		for isSpace(lexer.Lookahead) {
			lexer.Advance(true)
		}
		lexer.MarkEnd()

		// Check if the next character could start a selector.
		ch := lexer.Lookahead
		if ch == '#' || ch == '.' || ch == '[' || ch == '-' ||
			ch == '*' || isAlnum(ch) {
			return true
		}

		// If next char is ':', look ahead to determine if this is
		// a selector context (contains '{') or property context (contains ';' or '}').
		if ch == ':' {
			lexer.Advance(false)
			if isSpace(lexer.Lookahead) {
				return false
			}
			for {
				if lexer.Lookahead == ';' || lexer.Lookahead == '}' || lexer.EOF() {
					return false
				}
				if lexer.Lookahead == '{' {
					return true
				}
				lexer.Advance(false)
			}
		}
	}

	// Pseudo-class selector colon: disambiguate ':hover' from 'color: red'.
	if validSymbols[PseudoClassSelectorColon] {
		for isSpace(lexer.Lookahead) {
			lexer.Advance(true)
		}
		if lexer.Lookahead == ':' {
			lexer.Advance(false)
			// '::' is a pseudo-element, not a pseudo-class.
			if lexer.Lookahead == ':' {
				return false
			}
			lexer.MarkEnd()
			lexer.ResultSymbol = ts.Symbol(PseudoClassSelectorColon)

			// Scan ahead: '{' means selector context, ';' or '}' means property.
			for lexer.Lookahead != ';' && lexer.Lookahead != '}' && !lexer.EOF() {
				lexer.Advance(false)
				if lexer.Lookahead == '{' {
					return true
				}
			}

			// At EOF without finding '{': still return pseudo-class for better
			// error recovery on malformed input.
			return lexer.EOF()
		}
	}

	return false
}

// isSpace returns true if ch is a Unicode whitespace character.
func isSpace(ch int32) bool {
	return ch >= 0 && unicode.IsSpace(rune(ch))
}

// isAlnum returns true if ch is a Unicode letter or digit.
func isAlnum(ch int32) bool {
	return ch >= 0 && (unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)))
}
