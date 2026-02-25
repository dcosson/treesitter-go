// Package ruby provides the external scanner for tree-sitter-ruby.
//
// Ported from tree-sitter-ruby/src/scanner.c.
// The Ruby external scanner handles context-sensitive token types including:
//   - LINE_BREAK / NO_LINE_BREAK: Significant newline detection
//   - String/symbol/regex/subshell/heredoc delimiters and content
//   - Operator disambiguation (*, **, -, &, [, /, <<)
//   - Hash key symbols, identifier/constant suffixes
//   - Short interpolation (#@var, #$var)
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
// All EOF checks use lexer.EOF() instead of checking for 0.
package ruby

import (
	"strings"
	"unicode"
	"unicode/utf8"

	ts "github.com/treesitter-go/treesitter"
)

// Token types matching the externals array in grammar.js.
const (
	LineBreak = iota
	NoLineBreak

	// Delimited literals
	SimpleSymbol
	StringStart
	SymbolStart
	SubshellStart
	RegexStart
	StringArrayStart
	SymbolArrayStart
	HeredocBodyStart
	StringContent
	HeredocContent
	StringEnd
	HeredocBodyEnd
	HeredocStart

	// Whitespace-sensitive tokens
	ForwardSlash
	BlockAmpersand
	SplatStar
	UnaryMinus
	UnaryMinusNum
	BinaryMinus
	BinaryStar
	SingletonClassLeftAngleLeftAngle
	HashKeySymbol
	IdentifierSuffix
	ConstantSuffix
	HashSplatStarStar
	BinaryStarStar
	ElementReferenceBracket
	ShortInterpolation

	None
)

// nonIdentifierChars contains characters that cannot appear in identifiers.
const nonIdentifierChars = "\x00\n\r\t :;`\"'@$#.,|^&<=>+-*/\\%?!~()[]{}  "

// literal represents a delimited literal (string, regex, etc.) on the stack.
type literal struct {
	tokenType           int
	openDelimiter       int32
	closeDelimiter      int32
	nestingDepth        int32
	allowsInterpolation bool
}

// heredoc represents a pending heredoc with its delimiter and state.
type heredoc struct {
	word                      []byte
	endWordIndentationAllowed bool
	allowsInterpolation       bool
	started                   bool
}

// Scanner implements ts.ExternalScanner for Ruby.
type Scanner struct {
	hasLeadingWhitespace bool
	literalStack         []literal
	openHeredocs         []heredoc
}

// New creates a new Ruby external scanner (ts.ExternalScannerFactory).
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner state to buf and returns the number of bytes written.
// Matches C format: delimiters and nesting depth are single bytes (char cast).
func (s *Scanner) Serialize(buf []byte) uint32 {
	size := uint32(0)

	// Check if we have enough room for at least the literal stack count byte.
	if len(buf) < 1 {
		return 0
	}

	// Check if the literal stack would overflow the buffer.
	// C serializes each literal as 5 bytes: 1 type + 1 open + 1 close + 1 depth + 1 interp.
	// Delimiters and nesting depth are cast to (char) in C, so only low byte is stored.
	if uint32(len(s.literalStack))*5+2 >= uint32(len(buf)) {
		return 0
	}

	buf[size] = byte(len(s.literalStack))
	size++
	for i := range s.literalStack {
		lit := &s.literalStack[i]
		buf[size] = byte(lit.tokenType)
		size++
		buf[size] = byte(lit.openDelimiter)
		size++
		buf[size] = byte(lit.closeDelimiter)
		size++
		buf[size] = byte(lit.nestingDepth)
		size++
		buf[size] = boolByte(lit.allowsInterpolation)
		size++
	}

	if int(size) >= len(buf) {
		return 0
	}

	buf[size] = byte(len(s.openHeredocs))
	size++
	for i := range s.openHeredocs {
		h := &s.openHeredocs[i]
		if int(size)+3+1+len(h.word) >= len(buf) {
			return 0
		}
		buf[size] = boolByte(h.endWordIndentationAllowed)
		size++
		buf[size] = boolByte(h.allowsInterpolation)
		size++
		buf[size] = boolByte(h.started)
		size++
		buf[size] = byte(len(h.word))
		size++
		copy(buf[size:], h.word)
		size += uint32(len(h.word))
	}

	return size
}

// Deserialize restores the scanner state from data.
// C serializes delimiters and nesting depth as single bytes (char cast).
func (s *Scanner) Deserialize(data []byte) {
	s.hasLeadingWhitespace = false
	s.literalStack = nil
	s.openHeredocs = nil

	if len(data) == 0 {
		return
	}

	size := 0
	literalDepth := int(data[size])
	size++
	for j := 0; j < literalDepth && size+5 <= len(data); j++ {
		lit := literal{}
		lit.tokenType = int(data[size])
		size++
		lit.openDelimiter = int32(data[size])
		size++
		lit.closeDelimiter = int32(data[size])
		size++
		lit.nestingDepth = int32(data[size])
		size++
		lit.allowsInterpolation = data[size] != 0
		size++
		s.literalStack = append(s.literalStack, lit)
	}

	if size >= len(data) {
		return
	}

	openHeredocCount := int(data[size])
	size++
	for j := 0; j < openHeredocCount && size < len(data); j++ {
		h := heredoc{}
		if size+4 > len(data) {
			break
		}
		h.endWordIndentationAllowed = data[size] != 0
		size++
		h.allowsInterpolation = data[size] != 0
		size++
		h.started = data[size] != 0
		size++

		wordLength := int(uint8(data[size]))
		size++
		if size+wordLength > len(data) {
			break
		}
		h.word = make([]byte, wordLength)
		copy(h.word, data[size:size+wordLength])
		size += wordLength
		s.openHeredocs = append(s.openHeredocs, h)
	}
}

// Scan dispatches to the appropriate scanning function based on validSymbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	return s.scan(lexer, validSymbols)
}

func (s *Scanner) skip(lexer *ts.Lexer) {
	s.hasLeadingWhitespace = true
	lexer.Advance(true)
}

func advance(lexer *ts.Lexer) {
	lexer.Advance(false)
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func isSpace(ch int32) bool {
	return ch >= 0 && unicode.IsSpace(rune(ch))
}

func isAlpha(ch int32) bool {
	return ch >= 0 && unicode.IsLetter(rune(ch))
}

func isAlnum(ch int32) bool {
	return ch >= 0 && (unicode.IsLetter(rune(ch)) || unicode.IsDigit(rune(ch)))
}

func isDigit(ch int32) bool {
	return ch >= 0 && unicode.IsDigit(rune(ch))
}

func isLower(ch int32) bool {
	return ch >= 0 && unicode.IsLower(rune(ch))
}

func isUpper(ch int32) bool {
	return ch >= 0 && unicode.IsUpper(rune(ch))
}

// isIdenChar returns true if the character can appear in an identifier.
func isIdenChar(ch int32) bool {
	if ch < 0 {
		return false
	}
	return !strings.ContainsRune(nonIdentifierChars, rune(ch))
}

// scanWhitespace handles whitespace, newlines, and line continuations.
func (s *Scanner) scanWhitespace(lexer *ts.Lexer, validSymbols []bool) bool {
	heredocBodyStartIsValid := len(s.openHeredocs) > 0 &&
		!s.openHeredocs[0].started &&
		validSymbols[HeredocBodyStart]
	crossedNewline := false

	for {
		if !validSymbols[NoLineBreak] && validSymbols[LineBreak] && lexer.IsAtIncludedRangeStart() {
			lexer.MarkEnd()
			lexer.ResultSymbol = ts.Symbol(LineBreak)
			return true
		}

		switch lexer.Lookahead {
		case ' ', '\t':
			s.skip(lexer)
		case '\r':
			if heredocBodyStartIsValid {
				lexer.ResultSymbol = ts.Symbol(HeredocBodyStart)
				s.openHeredocs[0].started = true
				return true
			}
			s.skip(lexer)
		case '\n':
			if heredocBodyStartIsValid {
				lexer.ResultSymbol = ts.Symbol(HeredocBodyStart)
				s.openHeredocs[0].started = true
				return true
			} else if !validSymbols[NoLineBreak] && validSymbols[LineBreak] && !crossedNewline {
				lexer.MarkEnd()
				advance(lexer)
				crossedNewline = true
			} else {
				s.skip(lexer)
			}
		case '\\':
			advance(lexer)
			if lexer.Lookahead == '\r' {
				s.skip(lexer)
			}
			if isSpace(lexer.Lookahead) {
				s.skip(lexer)
			} else {
				return false
			}
		default:
			if crossedNewline {
				if lexer.Lookahead != '.' && lexer.Lookahead != '&' && lexer.Lookahead != '#' {
					lexer.ResultSymbol = ts.Symbol(LineBreak)
				} else if lexer.Lookahead == '.' {
					// Don't return LINE_BREAK for the call operator (`.`) but do return
					// one for range operators (`..` and `...`)
					advance(lexer)
					if !lexer.EOF() && lexer.Lookahead == '.' {
						lexer.ResultSymbol = ts.Symbol(LineBreak)
					} else {
						return false
					}
				}
			}
			return true
		}
	}
}

// scanOperator tries to match an operator for symbol identifiers.
func scanOperator(lexer *ts.Lexer) bool {
	switch lexer.Lookahead {
	// <, <=, <<, <=>
	case '<':
		advance(lexer)
		if lexer.Lookahead == '<' {
			advance(lexer)
		} else if lexer.Lookahead == '=' {
			advance(lexer)
			if lexer.Lookahead == '>' {
				advance(lexer)
			}
		}
		return true

	// >, >=, >>
	case '>':
		advance(lexer)
		if lexer.Lookahead == '>' || lexer.Lookahead == '=' {
			advance(lexer)
		}
		return true

	// ==, ===, =~
	case '=':
		advance(lexer)
		if lexer.Lookahead == '~' {
			advance(lexer)
			return true
		}
		if lexer.Lookahead == '=' {
			advance(lexer)
			if lexer.Lookahead == '=' {
				advance(lexer)
			}
			return true
		}
		return false

	// +, -, ~, +@, -@, ~@
	case '+', '-', '~':
		advance(lexer)
		if lexer.Lookahead == '@' {
			advance(lexer)
		}
		return true

	// ..
	case '.':
		advance(lexer)
		if lexer.Lookahead == '.' {
			advance(lexer)
			return true
		}
		return false

	// &, ^, |, /, %, `
	case '&', '^', '|', '/', '%', '`':
		advance(lexer)
		return true

	// !, !=, !~
	case '!':
		advance(lexer)
		if lexer.Lookahead == '=' || lexer.Lookahead == '~' {
			advance(lexer)
		}
		return true

	// *, **
	case '*':
		advance(lexer)
		if lexer.Lookahead == '*' {
			advance(lexer)
		}
		return true

	// [], []=
	case '[':
		advance(lexer)
		if lexer.Lookahead == ']' {
			advance(lexer)
		} else {
			return false
		}
		if lexer.Lookahead == '=' {
			advance(lexer)
		}
		return true

	default:
		return false
	}
}

// scanSymbolIdentifier scans an identifier for use in a symbol literal (:foo).
func scanSymbolIdentifier(lexer *ts.Lexer) bool {
	if lexer.Lookahead == '@' {
		advance(lexer)
		if lexer.Lookahead == '@' {
			advance(lexer)
		}
	} else if lexer.Lookahead == '$' {
		advance(lexer)
	}

	if isIdenChar(lexer.Lookahead) {
		advance(lexer)
	} else if !scanOperator(lexer) {
		return false
	}

	for isIdenChar(lexer.Lookahead) {
		advance(lexer)
	}

	if lexer.Lookahead == '?' || lexer.Lookahead == '!' {
		advance(lexer)
	}

	if lexer.Lookahead == '=' {
		lexer.MarkEnd()
		advance(lexer)
		if lexer.Lookahead != '>' {
			lexer.MarkEnd()
		}
	}

	return true
}

// scanOpenDelimiter attempts to scan an opening delimiter for a literal.
func (s *Scanner) scanOpenDelimiter(lexer *ts.Lexer, lit *literal, validSymbols []bool) bool {
	switch lexer.Lookahead {
	case '"':
		lit.tokenType = StringStart
		lit.openDelimiter = lexer.Lookahead
		lit.closeDelimiter = lexer.Lookahead
		lit.allowsInterpolation = true
		advance(lexer)
		return true

	case '\'':
		lit.tokenType = StringStart
		lit.openDelimiter = lexer.Lookahead
		lit.closeDelimiter = lexer.Lookahead
		lit.allowsInterpolation = false
		advance(lexer)
		return true

	case '`':
		if !validSymbols[SubshellStart] {
			return false
		}
		lit.tokenType = SubshellStart
		lit.openDelimiter = lexer.Lookahead
		lit.closeDelimiter = lexer.Lookahead
		lit.allowsInterpolation = true
		advance(lexer)
		return true

	case '/':
		if !validSymbols[RegexStart] {
			return false
		}
		lit.tokenType = RegexStart
		lit.openDelimiter = lexer.Lookahead
		lit.closeDelimiter = lexer.Lookahead
		lit.allowsInterpolation = true
		advance(lexer)
		if validSymbols[ForwardSlash] {
			if !s.hasLeadingWhitespace {
				return false
			}
			if lexer.Lookahead == ' ' || lexer.Lookahead == '\t' || lexer.Lookahead == '\n' ||
				lexer.Lookahead == '\r' {
				return false
			}
			if lexer.Lookahead == '=' {
				return false
			}
		}
		return true

	case '%':
		advance(lexer)

		switch lexer.Lookahead {
		case 's':
			if !validSymbols[SimpleSymbol] {
				return false
			}
			lit.tokenType = SymbolStart
			lit.allowsInterpolation = false
			advance(lexer)

		case 'r':
			if !validSymbols[RegexStart] {
				return false
			}
			lit.tokenType = RegexStart
			lit.allowsInterpolation = true
			advance(lexer)

		case 'x':
			if !validSymbols[SubshellStart] {
				return false
			}
			lit.tokenType = SubshellStart
			lit.allowsInterpolation = true
			advance(lexer)

		case 'q':
			if !validSymbols[StringStart] {
				return false
			}
			lit.tokenType = StringStart
			lit.allowsInterpolation = false
			advance(lexer)

		case 'Q':
			if !validSymbols[StringStart] {
				return false
			}
			lit.tokenType = StringStart
			lit.allowsInterpolation = true
			advance(lexer)

		case 'w':
			if !validSymbols[StringArrayStart] {
				return false
			}
			lit.tokenType = StringArrayStart
			lit.allowsInterpolation = false
			advance(lexer)

		case 'i':
			if !validSymbols[SymbolArrayStart] {
				return false
			}
			lit.tokenType = SymbolArrayStart
			lit.allowsInterpolation = false
			advance(lexer)

		case 'W':
			if !validSymbols[StringArrayStart] {
				return false
			}
			lit.tokenType = StringArrayStart
			lit.allowsInterpolation = true
			advance(lexer)

		case 'I':
			if !validSymbols[SymbolArrayStart] {
				return false
			}
			lit.tokenType = SymbolArrayStart
			lit.allowsInterpolation = true
			advance(lexer)

		default:
			if !validSymbols[StringStart] {
				return false
			}
			lit.tokenType = StringStart
			lit.allowsInterpolation = true
		}

		switch lexer.Lookahead {
		case '(':
			lit.openDelimiter = '('
			lit.closeDelimiter = ')'
		case '[':
			lit.openDelimiter = '['
			lit.closeDelimiter = ']'
		case '{':
			lit.openDelimiter = '{'
			lit.closeDelimiter = '}'
		case '<':
			lit.openDelimiter = '<'
			lit.closeDelimiter = '>'
		case '\r', '\n', ' ', '\t':
			// If the `/` operator is valid, then so is the `%` operator, which means
			// that a `%` followed by whitespace should be considered an operator,
			// not a percent string.
			if validSymbols[ForwardSlash] {
				return false
			}
			// C leaves open/close delimiter at 0 (default) and just breaks.
			// The delimiter stays unset — this matches the C behavior exactly.
		case '|', '!', '#', '/', '\\', '@', '$', '%', '^', '&', '*',
			')', ']', '}', '>', '+', '-', '~', '`', ',', '.', '?',
			':', ';', '_', '"', '\'':
			lit.openDelimiter = lexer.Lookahead
			lit.closeDelimiter = lexer.Lookahead
		default:
			return false
		}

		advance(lexer)
		return true

	default:
		return false
	}
}

// scanHeredocWord scans the delimiter word of a heredoc.
// P2.3 fix: Uses utf8.EncodeRune instead of byte() to avoid truncating non-ASCII codepoints.
func scanHeredocWord(lexer *ts.Lexer, h *heredoc) {
	var word []byte
	var quote int32

	switch lexer.Lookahead {
	case '\'', '"', '`':
		quote = lexer.Lookahead
		advance(lexer)
		for lexer.Lookahead != quote && !lexer.EOF() {
			var encBuf [4]byte
			n := utf8.EncodeRune(encBuf[:], rune(lexer.Lookahead))
			word = append(word, encBuf[:n]...)
			advance(lexer)
		}
		advance(lexer)

	default:
		if isAlnum(lexer.Lookahead) || lexer.Lookahead == '_' {
			var encBuf [4]byte
			n := utf8.EncodeRune(encBuf[:], rune(lexer.Lookahead))
			word = append(word, encBuf[:n]...)
			advance(lexer)
			for isAlnum(lexer.Lookahead) || lexer.Lookahead == '_' {
				n = utf8.EncodeRune(encBuf[:], rune(lexer.Lookahead))
				word = append(word, encBuf[:n]...)
				advance(lexer)
			}
		}
	}

	h.word = word
	h.allowsInterpolation = quote != '\''
}

// scanShortInterpolation handles #@var and #$var short interpolation inside strings/heredocs.
// P2.4 fix: Added EOF guard before ContainsRune to avoid passing -1 (EOF).
func scanShortInterpolation(lexer *ts.Lexer, hasContent bool, contentSymbol int) bool {
	start := lexer.Lookahead
	if start == '@' || start == '$' {
		if hasContent {
			lexer.ResultSymbol = ts.Symbol(contentSymbol)
			return true
		}
		lexer.MarkEnd()
		advance(lexer)
		isShortInterp := false
		if start == '$' {
			if !lexer.EOF() && strings.ContainsRune("!@&`'+~=/\\,;.<>*$?:\"", rune(lexer.Lookahead)) {
				isShortInterp = true
			} else {
				if lexer.Lookahead == '-' {
					advance(lexer)
					isShortInterp = isAlpha(lexer.Lookahead) || lexer.Lookahead == '_'
				} else {
					isShortInterp = isAlnum(lexer.Lookahead) || lexer.Lookahead == '_'
				}
			}
		}
		if start == '@' {
			if lexer.Lookahead == '@' {
				advance(lexer)
			}
			isShortInterp = isIdenChar(lexer.Lookahead) && !isDigit(lexer.Lookahead)
		}

		if isShortInterp {
			lexer.ResultSymbol = ts.Symbol(ShortInterpolation)
			return true
		}
	}
	return false
}

// scanHeredocContent scans the body content of a heredoc.
func (s *Scanner) scanHeredocContent(lexer *ts.Lexer) bool {
	h := &s.openHeredocs[0]
	positionInWord := 0
	lookForHeredocEnd := true
	hasContent := false

	for {
		if positionInWord == len(h.word) {
			if !hasContent {
				lexer.MarkEnd()
			}
			for lexer.Lookahead == ' ' || lexer.Lookahead == '\t' {
				advance(lexer)
			}
			if lexer.Lookahead == '\n' || lexer.Lookahead == '\r' {
				if hasContent {
					lexer.ResultSymbol = ts.Symbol(HeredocContent)
				} else {
					s.openHeredocs = s.openHeredocs[1:]
					lexer.ResultSymbol = ts.Symbol(HeredocBodyEnd)
				}
				return true
			}
			hasContent = true
			positionInWord = 0
		}

		if lexer.EOF() {
			lexer.MarkEnd()
			if hasContent {
				lexer.ResultSymbol = ts.Symbol(HeredocContent)
			} else {
				s.openHeredocs = s.openHeredocs[1:]
				lexer.ResultSymbol = ts.Symbol(HeredocBodyEnd)
			}
			return true
		}

		if positionInWord < len(h.word) && lexer.Lookahead == int32(h.word[positionInWord]) && lookForHeredocEnd {
			advance(lexer)
			positionInWord++
		} else {
			positionInWord = 0
			lookForHeredocEnd = false

			if h.allowsInterpolation && lexer.Lookahead == '\\' {
				if hasContent {
					lexer.ResultSymbol = ts.Symbol(HeredocContent)
					return true
				}
				return false
			}

			if h.allowsInterpolation && lexer.Lookahead == '#' {
				lexer.MarkEnd()
				advance(lexer)
				if lexer.Lookahead == '{' {
					if hasContent {
						lexer.ResultSymbol = ts.Symbol(HeredocContent)
						return true
					}
					return false
				}
				if scanShortInterpolation(lexer, hasContent, HeredocContent) {
					return true
				}
			} else if lexer.Lookahead == '\r' || lexer.Lookahead == '\n' {
				if lexer.Lookahead == '\r' {
					advance(lexer)
					if lexer.Lookahead == '\n' {
						advance(lexer)
					}
				} else {
					advance(lexer)
				}
				hasContent = true
				lookForHeredocEnd = true
				for lexer.Lookahead == ' ' || lexer.Lookahead == '\t' {
					advance(lexer)
					if !h.endWordIndentationAllowed {
						lookForHeredocEnd = false
					}
				}
				lexer.MarkEnd()
			} else {
				hasContent = true
				advance(lexer)
				lexer.MarkEnd()
			}
		}
	}
}

// scanLiteralContent scans the body content of a string/regex/etc literal.
// P2.2 fix: Removed dead advance() call at EOF.
func (s *Scanner) scanLiteralContent(lexer *ts.Lexer) bool {
	lit := &s.literalStack[len(s.literalStack)-1]
	hasContent := false
	stopOnSpace := lit.tokenType == SymbolArrayStart || lit.tokenType == StringArrayStart

	for {
		if stopOnSpace && isSpace(lexer.Lookahead) {
			if hasContent {
				lexer.MarkEnd()
				lexer.ResultSymbol = ts.Symbol(StringContent)
				return true
			}
			return false
		}
		// In C, closeDelimiter==0 matches at EOF (where lookahead==0).
		// In Go, EOF is Lookahead==-1, so we need an explicit check.
		if lexer.Lookahead == lit.closeDelimiter || (lit.closeDelimiter == 0 && lexer.EOF()) {
			lexer.MarkEnd()
			if lit.nestingDepth == 1 {
				if hasContent {
					lexer.ResultSymbol = ts.Symbol(StringContent)
				} else {
					advance(lexer)
					if lit.tokenType == RegexStart {
						for isLower(lexer.Lookahead) {
							advance(lexer)
						}
					}
					s.literalStack = s.literalStack[:len(s.literalStack)-1]
					lexer.ResultSymbol = ts.Symbol(StringEnd)
					lexer.MarkEnd()
				}
				return true
			}
			lit.nestingDepth--
			advance(lexer)

		} else if lexer.Lookahead == lit.openDelimiter {
			lit.nestingDepth++
			advance(lexer)
		} else if lit.allowsInterpolation && lexer.Lookahead == '#' {
			lexer.MarkEnd()
			advance(lexer)
			if lexer.Lookahead == '{' {
				if hasContent {
					lexer.ResultSymbol = ts.Symbol(StringContent)
					return true
				}
				return false
			}
			if scanShortInterpolation(lexer, hasContent, StringContent) {
				return true
			}
		} else if lexer.Lookahead == '\\' {
			if lit.allowsInterpolation {
				if hasContent {
					lexer.MarkEnd()
					lexer.ResultSymbol = ts.Symbol(StringContent)
					return true
				}
				return false
			}
			advance(lexer)
			advance(lexer)

		} else if lexer.EOF() {
			lexer.MarkEnd()
			return false
		} else {
			advance(lexer)
		}

		hasContent = true
	}
}

// scan is the main scanning entrypoint.
// P3.1 fix: Added MarkEnd() before return true in operator disambiguation paths.
func (s *Scanner) scan(lexer *ts.Lexer, validSymbols []bool) bool {
	s.hasLeadingWhitespace = false

	// Contents of literals, which match any character except for some close delimiter.
	if !validSymbols[StringStart] {
		if (validSymbols[StringContent] || validSymbols[StringEnd]) && len(s.literalStack) > 0 {
			return s.scanLiteralContent(lexer)
		}
		if (validSymbols[HeredocContent] || validSymbols[HeredocBodyEnd]) && len(s.openHeredocs) > 0 {
			return s.scanHeredocContent(lexer)
		}
	}

	// Whitespace
	lexer.ResultSymbol = ts.Symbol(None)
	if !s.scanWhitespace(lexer, validSymbols) {
		return false
	}
	if lexer.ResultSymbol != ts.Symbol(None) {
		return true
	}

	switch lexer.Lookahead {
	case '&':
		if validSymbols[BlockAmpersand] {
			advance(lexer)
			if lexer.Lookahead != '&' && lexer.Lookahead != '.' && lexer.Lookahead != '=' &&
				!isSpace(lexer.Lookahead) {
				lexer.MarkEnd()
				lexer.ResultSymbol = ts.Symbol(BlockAmpersand)
				return true
			}
			return false
		}

	case '<':
		if validSymbols[SingletonClassLeftAngleLeftAngle] {
			advance(lexer)
			if lexer.Lookahead == '<' {
				advance(lexer)
				lexer.MarkEnd()
				lexer.ResultSymbol = ts.Symbol(SingletonClassLeftAngleLeftAngle)
				return true
			}
			return false
		}

	case '*':
		if validSymbols[SplatStar] || validSymbols[BinaryStar] || validSymbols[HashSplatStarStar] ||
			validSymbols[BinaryStarStar] {
			advance(lexer)
			if lexer.Lookahead == '=' {
				return false
			}
			if lexer.Lookahead == '*' {
				if validSymbols[HashSplatStarStar] || validSymbols[BinaryStarStar] {
					advance(lexer)
					if lexer.Lookahead == '=' {
						return false
					}
					lexer.MarkEnd()
					if validSymbols[BinaryStarStar] && !s.hasLeadingWhitespace {
						lexer.ResultSymbol = ts.Symbol(BinaryStarStar)
						return true
					}
					if validSymbols[HashSplatStarStar] && !isSpace(lexer.Lookahead) {
						lexer.ResultSymbol = ts.Symbol(HashSplatStarStar)
						return true
					}
					if validSymbols[BinaryStarStar] {
						lexer.ResultSymbol = ts.Symbol(BinaryStarStar)
						return true
					}
					if validSymbols[HashSplatStarStar] {
						lexer.ResultSymbol = ts.Symbol(HashSplatStarStar)
						return true
					}
					return false
				}
				return false
			}
			lexer.MarkEnd()
			if validSymbols[BinaryStar] && !s.hasLeadingWhitespace {
				lexer.ResultSymbol = ts.Symbol(BinaryStar)
				return true
			}
			if validSymbols[SplatStar] && !isSpace(lexer.Lookahead) {
				lexer.ResultSymbol = ts.Symbol(SplatStar)
				return true
			}
			if validSymbols[BinaryStar] {
				lexer.ResultSymbol = ts.Symbol(BinaryStar)
				return true
			}
			if validSymbols[SplatStar] {
				lexer.ResultSymbol = ts.Symbol(SplatStar)
				return true
			}
			return false
		}

	case '-':
		if validSymbols[UnaryMinus] || validSymbols[UnaryMinusNum] || validSymbols[BinaryMinus] {
			advance(lexer)
			if lexer.Lookahead != '=' && lexer.Lookahead != '>' {
				lexer.MarkEnd()
				if validSymbols[UnaryMinusNum] &&
					(!validSymbols[BinaryStar] || s.hasLeadingWhitespace) &&
					isDigit(lexer.Lookahead) {
					lexer.ResultSymbol = ts.Symbol(UnaryMinusNum)
					return true
				}
				if validSymbols[UnaryMinus] && s.hasLeadingWhitespace && !isSpace(lexer.Lookahead) {
					lexer.ResultSymbol = ts.Symbol(UnaryMinus)
				} else if validSymbols[BinaryMinus] {
					lexer.ResultSymbol = ts.Symbol(BinaryMinus)
				} else {
					lexer.ResultSymbol = ts.Symbol(UnaryMinus)
				}
				return true
			}
			return false
		}

	case ':':
		if validSymbols[SymbolStart] {
			lit := literal{
				tokenType:    SymbolStart,
				nestingDepth: 1,
			}
			advance(lexer)

			switch lexer.Lookahead {
			case '"':
				advance(lexer)
				lit.openDelimiter = '"'
				lit.closeDelimiter = '"'
				lit.allowsInterpolation = true
				s.literalStack = append(s.literalStack, lit)
				lexer.ResultSymbol = ts.Symbol(SymbolStart)
				return true

			case '\'':
				advance(lexer)
				lit.openDelimiter = '\''
				lit.closeDelimiter = '\''
				lit.allowsInterpolation = false
				s.literalStack = append(s.literalStack, lit)
				lexer.ResultSymbol = ts.Symbol(SymbolStart)
				return true

			default:
				if scanSymbolIdentifier(lexer) {
					lexer.ResultSymbol = ts.Symbol(SimpleSymbol)
					return true
				}
			}

			return false
		}

	case '[':
		// Treat a square bracket as an element reference if either:
		// * the bracket is not preceded by any whitespace
		// * an arbitrary expression is not valid at the current position.
		if validSymbols[ElementReferenceBracket] &&
			(!s.hasLeadingWhitespace || !validSymbols[StringStart]) {
			advance(lexer)
			lexer.MarkEnd()
			lexer.ResultSymbol = ts.Symbol(ElementReferenceBracket)
			return true
		}
	}

	// Open delimiters for literals — hash key symbol / identifier suffix / constant suffix
	if ((validSymbols[HashKeySymbol] || validSymbols[IdentifierSuffix]) &&
		(isAlpha(lexer.Lookahead) || lexer.Lookahead == '_')) ||
		(validSymbols[ConstantSuffix] && isUpper(lexer.Lookahead)) {
		validIdentifierSymbol := IdentifierSuffix
		if isUpper(lexer.Lookahead) {
			validIdentifierSymbol = ConstantSuffix
		}
		for isAlnum(lexer.Lookahead) || lexer.Lookahead == '_' {
			advance(lexer)
		}

		if validSymbols[HashKeySymbol] && lexer.Lookahead == ':' {
			lexer.MarkEnd()
			advance(lexer)
			if lexer.Lookahead != ':' {
				lexer.ResultSymbol = ts.Symbol(HashKeySymbol)
				return true
			}
		} else if validSymbols[validIdentifierSymbol] && lexer.Lookahead == '!' {
			advance(lexer)
			if lexer.Lookahead != '=' {
				lexer.ResultSymbol = ts.Symbol(validIdentifierSymbol)
				return true
			}
		}

		return false
	}

	// Open delimiters for literals
	if validSymbols[StringStart] {
		lit := literal{
			nestingDepth: 1,
		}

		if lexer.Lookahead == '<' {
			advance(lexer)
			if lexer.Lookahead != '<' {
				return false
			}
			advance(lexer)

			h := heredoc{}
			if lexer.Lookahead == '-' || lexer.Lookahead == '~' {
				advance(lexer)
				h.endWordIndentationAllowed = true
			}

			scanHeredocWord(lexer, &h)
			if len(h.word) == 0 {
				return false
			}
			s.openHeredocs = append(s.openHeredocs, h)
			lexer.ResultSymbol = ts.Symbol(HeredocStart)
			return true
		}

		if s.scanOpenDelimiter(lexer, &lit, validSymbols) {
			s.literalStack = append(s.literalStack, lit)
			lexer.ResultSymbol = ts.Symbol(lit.tokenType)
			return true
		}
		return false
	}

	return false
}
