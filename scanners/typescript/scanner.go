// Package typescript provides the external scanner for tree-sitter-typescript.
//
// Ported from tree-sitter-typescript/common/scanner.h.
// The TypeScript external scanner handles 10 context-sensitive token types:
//   - AUTOMATIC_SEMICOLON: Automatic semicolon insertion (ASI)
//   - TEMPLATE_CHARS: Template literal body content
//   - TERNARY_QMARK: Ternary operator ? (disambiguated from optional chaining)
//   - HTML_COMMENT: <!-- and --> HTML-style comments
//   - LOGICAL_OR: Used as context signal for ASI decisions
//   - ESCAPE_SEQUENCE: Used as context signal for HTML comment decisions
//   - REGEX_PATTERN: Used as context signal for HTML comment decisions
//   - JSX_TEXT: Text content inside JSX elements
//   - FUNCTION_SIGNATURE_AUTOMATIC_SEMICOLON: ASI variant for function signatures
//   - ERROR_RECOVERY: Error recovery sentinel
//
// This scanner is stateless — no serialize/deserialize needed.
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
package typescript

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
	FunctionSignatureAutomaticSemicolon
	ErrorRecovery
)

// Scanner implements ts.ExternalScanner for TypeScript.
// This scanner is stateless.
type Scanner struct{}

// New creates a new TypeScript external scanner.
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner state to buf. Always returns 0 (stateless).
func (s *Scanner) Serialize(buf []byte) uint32 {
	return 0
}

// Deserialize restores the scanner state from data. No-op (stateless).
func (s *Scanner) Deserialize(data []byte) {}

// Scan dispatches to the appropriate scanning function based on validSymbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	if validSymbols[TemplateChars] {
		if validSymbols[AutomaticSemicolon] {
			return false
		}
		return scanTemplateChars(lexer)
	}

	if validSymbols[JSXText] && scanJSXText(lexer) {
		return true
	}

	if validSymbols[AutomaticSemicolon] || validSymbols[FunctionSignatureAutomaticSemicolon] {
		scannedComment := false
		ret := scanAutomaticSemicolon(lexer, validSymbols, &scannedComment)
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
		return scanClosingComment(lexer)
	}

	return false
}

func scanTemplateChars(lexer *ts.Lexer) bool {
	lexer.ResultSymbol = TemplateChars
	hasContent := false
	for {
		lexer.MarkEnd()
		switch {
		case lexer.Lookahead == '`':
			return hasContent
		case lexer.EOF():
			return false
		case lexer.Lookahead == '$':
			advance(lexer)
			if lexer.Lookahead == '{' {
				return hasContent
			}
		case lexer.Lookahead == '\\':
			return hasContent
		default:
			advance(lexer)
		}
		hasContent = true
	}
}

func scanWhitespaceAndComments(lexer *ts.Lexer, scannedComment *bool) bool {
	for {
		for isSpace(lexer.Lookahead) {
			skip(lexer)
		}

		if lexer.Lookahead == '/' {
			skip(lexer)

			if lexer.Lookahead == '/' {
				skip(lexer)
				for !lexer.EOF() && lexer.Lookahead != '\n' {
					skip(lexer)
				}
				*scannedComment = true
			} else if lexer.Lookahead == '*' {
				skip(lexer)
				for !lexer.EOF() {
					if lexer.Lookahead == '*' {
						skip(lexer)
						if lexer.Lookahead == '/' {
							skip(lexer)
							break
						}
					} else {
						skip(lexer)
					}
				}
			} else {
				return false
			}
		} else {
			return true
		}
	}
}

func scanAutomaticSemicolon(lexer *ts.Lexer, validSymbols []bool, scannedComment *bool) bool {
	lexer.ResultSymbol = AutomaticSemicolon
	lexer.MarkEnd()

	for {
		if lexer.EOF() {
			return true
		}
		if lexer.Lookahead == '}' {
			for {
				skip(lexer)
				if !isSpace(lexer.Lookahead) {
					break
				}
			}
			if lexer.Lookahead == ':' {
				return validSymbols[LogicalOr]
			}
			return true
		}
		if !isSpace(lexer.Lookahead) {
			return false
		}
		if lexer.Lookahead == '\n' {
			break
		}
		skip(lexer)
	}

	skip(lexer)

	if !scanWhitespaceAndComments(lexer, scannedComment) {
		return false
	}

	switch lexer.Lookahead {
	case '`', ',', '.', ';', '*', '%', '>', '<', '=', '?', '^', '|', '&', '/', ':':
		return false

	case '{':
		if validSymbols[FunctionSignatureAutomaticSemicolon] {
			return false
		}

	case '(', '[':
		if validSymbols[LogicalOr] {
			return false
		}

	case '+':
		skip(lexer)
		return lexer.Lookahead == '+'

	case '-':
		skip(lexer)
		return lexer.Lookahead == '-'

	case '!':
		skip(lexer)
		return lexer.Lookahead != '='

	case 'i':
		skip(lexer)
		if lexer.Lookahead != 'n' {
			return true
		}
		skip(lexer)
		if !isAlpha(lexer.Lookahead) {
			return false
		}
		stanceof := "stanceof"
		for i := 0; i < len(stanceof); i++ {
			if lexer.Lookahead != int32(stanceof[i]) {
				return true
			}
			skip(lexer)
		}
		if !isAlpha(lexer.Lookahead) {
			return false
		}
	}

	return true
}

func scanTernaryQmark(lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) {
		skip(lexer)
	}

	if lexer.Lookahead == '?' {
		advance(lexer)

		// Optional chaining.
		if lexer.Lookahead == '?' || lexer.Lookahead == '.' {
			return false
		}

		lexer.MarkEnd()
		lexer.ResultSymbol = TernaryQmark

		// TypeScript optional arguments contain the ?: sequence, possibly with whitespace.
		for isSpace(lexer.Lookahead) {
			advance(lexer)
		}

		if lexer.Lookahead == ':' || lexer.Lookahead == ')' || lexer.Lookahead == ',' {
			return false
		}

		if lexer.Lookahead == '.' {
			advance(lexer)
			if isDigit(lexer.Lookahead) {
				return true
			}
			return false
		}
		return true
	}
	return false
}

func scanClosingComment(lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) || lexer.Lookahead == 0x2028 || lexer.Lookahead == 0x2029 {
		skip(lexer)
	}

	commentStart := "<!--"
	commentEnd := "-->"

	if lexer.Lookahead == '<' {
		for i := 0; i < len(commentStart); i++ {
			if lexer.Lookahead != int32(commentStart[i]) {
				return false
			}
			advance(lexer)
		}
	} else if lexer.Lookahead == '-' {
		for i := 0; i < len(commentEnd); i++ {
			if lexer.Lookahead != int32(commentEnd[i]) {
				return false
			}
			advance(lexer)
		}
	} else {
		return false
	}

	for !lexer.EOF() && lexer.Lookahead != '\n' &&
		lexer.Lookahead != 0x2028 && lexer.Lookahead != 0x2029 {
		advance(lexer)
	}

	lexer.ResultSymbol = HTMLComment
	lexer.MarkEnd()

	return true
}

func scanJSXText(lexer *ts.Lexer) bool {
	sawText := false
	atNewline := false

	for !lexer.EOF() && lexer.Lookahead != '<' && lexer.Lookahead != '>' &&
		lexer.Lookahead != '{' && lexer.Lookahead != '}' && lexer.Lookahead != '&' {
		isWspace := isSpace(lexer.Lookahead)
		if lexer.Lookahead == '\n' {
			atNewline = true
		} else {
			atNewline = atNewline && isWspace
			if !atNewline {
				sawText = true
			}
		}
		advance(lexer)
	}

	lexer.ResultSymbol = JSXText
	lexer.MarkEnd()
	return sawText
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
