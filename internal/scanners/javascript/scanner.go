// Package javascript provides the external scanner for tree-sitter-javascript.
//
// Ported from tree-sitter-javascript/src/scanner.c.
// The JavaScript external scanner handles 8 context-sensitive token types:
//   - AUTOMATIC_SEMICOLON: JavaScript's ASI (Automatic Semicolon Insertion)
//   - TEMPLATE_CHARS: Content inside template literals (`...${...}...`)
//   - TERNARY_QMARK: Distinguishes ternary ? from optional chaining ?.
//   - HTML_COMMENT: Legacy <!-- and --> comments
//   - LOGICAL_OR: || operator (used for disambiguation)
//   - ESCAPE_SEQUENCE: Escape sequences in strings
//   - REGEX_PATTERN: Regex literal content
//   - JSX_TEXT: Text content inside JSX elements
//
// The scanner is stateless — serialize/deserialize are no-ops.
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
// All EOF checks use lexer.EOF() instead of checking for 0.
package javascript

import (
	"unicode"

	ts "github.com/treesitter-go/treesitter"
)

// Token types matching the externals array in grammar.js.
const (
	AutomaticSemicolon = iota
	TemplateChars
	TernaryQmark
	HTMLComment
	LogicalOr
	EscapeSequence
	RegexPattern
	JSXText
)

// WhitespaceResult indicates the result of whitespace/comment scanning
// for automatic semicolon insertion.
type WhitespaceResult int

const (
	Reject    WhitespaceResult = iota // Semicolon is illegal (syntax error)
	NoNewline                         // Unclear, continue scanning
	Accept                            // Semicolon is legal
)

// Scanner implements ts.ExternalScanner for JavaScript.
// The JavaScript scanner is stateless — it makes decisions based purely
// on the lexer's current position and lookahead.
type Scanner struct{}

// New creates a new JavaScript external scanner (ts.ExternalScannerFactory).
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize is a no-op — the JavaScript scanner is stateless.
func (s *Scanner) Serialize(buf []byte) uint32 {
	return 0
}

// Deserialize is a no-op — the JavaScript scanner is stateless.
func (s *Scanner) Deserialize(data []byte) {}

// Scan dispatches to the appropriate scanning function based on valid_symbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	if len(validSymbols) <= JSXText {
		return false
	}

	if validSymbols[TemplateChars] {
		if validSymbols[AutomaticSemicolon] {
			return false
		}
		return scanTemplateChars(lexer)
	}

	if validSymbols[JSXText] && scanJSXText(lexer) {
		return true
	}

	if validSymbols[AutomaticSemicolon] {
		scannedComment := false
		ret := scanAutomaticSemicolon(lexer, !validSymbols[LogicalOr], &scannedComment)
		if !ret && !scannedComment && validSymbols[TernaryQmark] && lexer.Lookahead == '?' {
			return scanTernaryQmark(lexer)
		}
		return ret
	}

	if validSymbols[TernaryQmark] {
		return scanTernaryQmark(lexer)
	}

	if validSymbols[HTMLComment] && !validSymbols[LogicalOr] &&
		!validSymbols[EscapeSequence] && !validSymbols[RegexPattern] {
		return scanHTMLComment(lexer)
	}

	return false
}

// scanTemplateChars scans template literal content (between backticks or ${).
func scanTemplateChars(lexer *ts.Lexer) bool {
	lexer.ResultSymbol = ts.Symbol(TemplateChars)
	hasContent := false

	for {
		lexer.MarkEnd()
		if lexer.EOF() {
			return false
		}
		switch lexer.Lookahead {
		case '`':
			return hasContent
		case '$':
			lexer.Advance(false)
			if lexer.Lookahead == '{' {
				return hasContent
			}
		case '\\':
			return hasContent
		default:
			lexer.Advance(false)
		}
		hasContent = true
	}
}

// scanWhitespaceAndComments scans whitespace and JS comments.
// If consume is false, only consumes enough to check if a comment indicates
// semicolon legality.
func scanWhitespaceAndComments(lexer *ts.Lexer, scannedComment *bool, consume bool) WhitespaceResult {
	sawBlockNewline := false

	for {
		for !lexer.EOF() && isWhitespace(lexer.Lookahead) {
			lexer.Skip()
		}

		if lexer.Lookahead == '/' {
			lexer.Skip()

			if lexer.Lookahead == '/' {
				// Line comment.
				lexer.Skip()
				for !lexer.EOF() && lexer.Lookahead != '\n' &&
					lexer.Lookahead != 0x2028 && lexer.Lookahead != 0x2029 {
					lexer.Skip()
				}
				*scannedComment = true
			} else if lexer.Lookahead == '*' {
				// Block comment.
				lexer.Skip()
				for !lexer.EOF() {
					if lexer.Lookahead == '*' {
						lexer.Skip()
						if lexer.Lookahead == '/' {
							lexer.Skip()
							*scannedComment = true

							if lexer.Lookahead != '/' && !consume {
								if sawBlockNewline {
									return Accept
								}
								return NoNewline
							}
							break
						}
					} else if lexer.Lookahead == '\n' || lexer.Lookahead == 0x2028 || lexer.Lookahead == 0x2029 {
						sawBlockNewline = true
						lexer.Skip()
					} else {
						lexer.Skip()
					}
				}
			} else {
				return Reject
			}
		} else {
			return Accept
		}
	}
}

// scanAutomaticSemicolon implements JavaScript's Automatic Semicolon Insertion.
func scanAutomaticSemicolon(lexer *ts.Lexer, commentCondition bool, scannedComment *bool) bool {
	lexer.ResultSymbol = ts.Symbol(AutomaticSemicolon)
	lexer.MarkEnd()

	for {
		if lexer.EOF() {
			return true
		}

		if lexer.Lookahead == '/' {
			result := scanWhitespaceAndComments(lexer, scannedComment, false)
			if result == Reject {
				return false
			}
			if result == Accept && commentCondition &&
				lexer.Lookahead != ',' && lexer.Lookahead != '=' {
				return true
			}
		}

		if lexer.Lookahead == '}' {
			return true
		}

		if lexer.IsAtIncludedRangeStart() {
			return true
		}

		if lexer.Lookahead == '\n' || lexer.Lookahead == 0x2028 || lexer.Lookahead == 0x2029 {
			break
		}

		if lexer.EOF() || !isWhitespace(lexer.Lookahead) {
			return false
		}

		lexer.Skip()
	}

	lexer.Skip()

	if scanWhitespaceAndComments(lexer, scannedComment, true) == Reject {
		return false
	}

	switch lexer.Lookahead {
	case '`', ',', ':', ';', '*', '%', '>', '<', '=', '[', '(', '?', '^', '|', '&', '/':
		return false

	case '.':
		// Insert semicolon before decimal literals but not otherwise.
		lexer.Skip()
		return isDigit(lexer.Lookahead)

	case '+':
		// Insert semicolon before `++` but not before binary `+`.
		lexer.Skip()
		return lexer.Lookahead == '+'

	case '-':
		// Insert semicolon before `--` but not before binary `-`.
		lexer.Skip()
		return lexer.Lookahead == '-'

	case '!':
		// Don't insert semicolon before `!=`, but do before unary `!`.
		lexer.Skip()
		return lexer.Lookahead != '='

	case 'i':
		// Don't insert before `in` or `instanceof`, but do before identifiers.
		lexer.Skip()
		if lexer.Lookahead != 'n' {
			return true
		}
		lexer.Skip()
		if !isAlpha(lexer.Lookahead) {
			return false
		}

		instanceof := "stanceof"
		for i := 0; i < len(instanceof); i++ {
			if lexer.Lookahead != int32(instanceof[i]) {
				return true
			}
			lexer.Skip()
		}

		if !isAlpha(lexer.Lookahead) {
			return false
		}
	}

	return true
}

// scanTernaryQmark distinguishes the ternary operator ? from optional chaining ?.
func scanTernaryQmark(lexer *ts.Lexer) bool {
	for !lexer.EOF() && isWhitespace(lexer.Lookahead) {
		lexer.Skip()
	}

	if lexer.Lookahead == '?' {
		lexer.Advance(false)

		if lexer.Lookahead == '?' {
			return false
		}

		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TernaryQmark)

		if lexer.Lookahead == '.' {
			lexer.Advance(false)
			return isDigit(lexer.Lookahead)
		}
		return true
	}
	return false
}

// scanHTMLComment recognizes legacy HTML-style comments (<!-- and -->).
func scanHTMLComment(lexer *ts.Lexer) bool {
	for !lexer.EOF() && (isWhitespace(lexer.Lookahead) || lexer.Lookahead == 0x2028 || lexer.Lookahead == 0x2029) {
		lexer.Skip()
	}

	commentStart := "<!--"
	commentEnd := "-->"

	if lexer.Lookahead == '<' {
		for i := 0; i < len(commentStart); i++ {
			if lexer.Lookahead != int32(commentStart[i]) {
				return false
			}
			lexer.Advance(false)
		}
	} else if lexer.Lookahead == '-' {
		for i := 0; i < len(commentEnd); i++ {
			if lexer.Lookahead != int32(commentEnd[i]) {
				return false
			}
			lexer.Advance(false)
		}
	} else {
		return false
	}

	// Consume until end of line.
	for !lexer.EOF() && lexer.Lookahead != '\n' &&
		lexer.Lookahead != 0x2028 && lexer.Lookahead != 0x2029 {
		lexer.Advance(false)
	}

	lexer.ResultSymbol = ts.Symbol(HTMLComment)
	lexer.MarkEnd()
	return true
}

// scanJSXText extracts text content within JSX elements.
func scanJSXText(lexer *ts.Lexer) bool {
	sawText := false
	atNewline := false

	for !lexer.EOF() && lexer.Lookahead != '<' && lexer.Lookahead != '>' &&
		lexer.Lookahead != '{' && lexer.Lookahead != '}' && lexer.Lookahead != '&' {

		isWspace := isWhitespace(lexer.Lookahead)

		if lexer.Lookahead == '\n' {
			atNewline = true
		} else {
			// atNewline stays true only if current char is whitespace
			// (whitespace-only lines after newline don't count as text).
			atNewline = atNewline && isWspace
			if !atNewline {
				sawText = true
			}
		}

		lexer.Advance(false)
	}

	lexer.ResultSymbol = ts.Symbol(JSXText)
	return sawText
}

// --- Helper functions ---

func isWhitespace(ch int32) bool {
	if ch <= 0 {
		return false // EOF (-1) or null (0) is not whitespace
	}
	if ch < 128 {
		return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\v'
	}
	return unicode.IsSpace(rune(ch))
}

func isAlpha(ch int32) bool {
	if ch <= 0 {
		return false
	}
	if ch < 128 {
		return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
	}
	return unicode.IsLetter(rune(ch))
}

func isDigit(ch int32) bool {
	return ch >= '0' && ch <= '9'
}
