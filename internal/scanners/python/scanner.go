// Package python provides the external scanner for tree-sitter-python.
//
// Ported from tree-sitter-python/src/scanner.c.
// The Python external scanner handles 12 context-sensitive token types:
//   - NEWLINE: Significant newlines (not inside brackets/parens)
//   - INDENT: Indentation increase
//   - DEDENT: Indentation decrease
//   - STRING_START: Beginning of string literals (with prefix flags)
//   - STRING_CONTENT: Body content of string literals
//   - ESCAPE_INTERPOLATION: {{ or }} in f-strings
//   - STRING_END: End of string literals
//   - COMMENT: Line comments
//   - CLOSE_PAREN/CLOSE_BRACKET/CLOSE_BRACE: Closing delimiters (for bracket tracking)
//   - EXCEPT: Exception keyword context
//
// The scanner tracks an indentation stack and a delimiter stack for nested strings.
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
package python

import (
	ts "github.com/dcosson/treesitter-go"
)

// Token types matching the externals array in grammar.js.
const (
	Newline = iota
	Indent
	Dedent
	StringStart
	StringContent
	EscapeInterpolation
	StringEnd
	Comment
	CloseParen
	CloseBracket
	CloseBrace
	Except
)

// Delimiter flag bits.
const (
	flagSingleQuote = 1 << 0
	flagDoubleQuote = 1 << 1
	flagBackQuote   = 1 << 2
	flagRaw         = 1 << 3
	flagFormat      = 1 << 4
	flagTriple      = 1 << 5
	flagBytes       = 1 << 6
)

// delimiter represents a string delimiter with flag bits.
type delimiter byte

func (d delimiter) isFormat() bool { return d&flagFormat != 0 }
func (d delimiter) isRaw() bool    { return d&flagRaw != 0 }
func (d delimiter) isTriple() bool { return d&flagTriple != 0 }
func (d delimiter) isBytes() bool  { return d&flagBytes != 0 }
func (d *delimiter) setFormat()    { *d |= flagFormat }
func (d *delimiter) setRaw()       { *d |= flagRaw }
func (d *delimiter) setTriple()    { *d |= flagTriple }
func (d *delimiter) setBytes()     { *d |= flagBytes }

func (d delimiter) endCharacter() int32 {
	if d&flagSingleQuote != 0 {
		return '\''
	}
	if d&flagDoubleQuote != 0 {
		return '"'
	}
	if d&flagBackQuote != 0 {
		return '`'
	}
	return 0
}

func (d *delimiter) setEndCharacter(ch int32) {
	switch ch {
	case '\'':
		*d |= flagSingleQuote
	case '"':
		*d |= flagDoubleQuote
	case '`':
		*d |= flagBackQuote
	}
}

// Scanner implements ts.ExternalScanner for Python.
type Scanner struct {
	indents                  []uint16
	delimiters               []delimiter
	insideInterpolatedString bool
}

// New creates a new Python external scanner (ts.ExternalScannerFactory).
func New() ts.ExternalScanner {
	return &Scanner{
		indents: []uint16{0}, // Always starts with indent 0
	}
}

// Serialize writes the scanner state to buf.
func (s *Scanner) Serialize(buf []byte) uint32 {
	size := uint32(0)

	if len(buf) < 2 {
		return 0
	}

	buf[size] = boolByte(s.insideInterpolatedString)
	size++

	delimCount := len(s.delimiters)
	if delimCount > 255 {
		delimCount = 255
	}
	buf[size] = byte(delimCount)
	size++

	for i := 0; i < delimCount && int(size) < len(buf); i++ {
		buf[size] = byte(s.delimiters[i])
		size++
	}

	// Serialize indent stack (skip first element which is always 0).
	// C writes 1 byte per indent: buffer[size++] = (char)*array_get(&scanner->indents, iter)
	for i := 1; i < len(s.indents) && int(size) < len(buf); i++ {
		buf[size] = byte(s.indents[i])
		size++
	}

	return size
}

// Deserialize restores the scanner state from data.
func (s *Scanner) Deserialize(data []byte) {
	s.delimiters = nil
	s.indents = []uint16{0} // Always starts with indent 0
	s.insideInterpolatedString = false

	if len(data) < 2 {
		return
	}

	size := 0
	s.insideInterpolatedString = data[size] != 0
	size++

	delimCount := int(data[size])
	size++

	if delimCount > 0 {
		s.delimiters = make([]delimiter, delimCount)
		for i := 0; i < delimCount && size < len(data); i++ {
			s.delimiters[i] = delimiter(data[size])
			size++
		}
	}

	// C reads 1 byte per indent: array_push(&scanner->indents, (unsigned char)buffer[size])
	for size < len(data) {
		s.indents = append(s.indents, uint16(data[size]))
		size++
	}
}

// Scan dispatches to the appropriate scanning function based on validSymbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	errorRecoveryMode := validSymbols[StringContent] && validSymbols[Indent]
	withinBrackets := validSymbols[CloseBrace] || validSymbols[CloseParen] || validSymbols[CloseBracket]

	advancedOnce := false

	// ESCAPE_INTERPOLATION: {{ or }} in f-strings
	if validSymbols[EscapeInterpolation] && len(s.delimiters) > 0 &&
		(lexer.Lookahead == '{' || lexer.Lookahead == '}') && !errorRecoveryMode {
		d := &s.delimiters[len(s.delimiters)-1]
		if d.isFormat() {
			lexer.MarkEnd()
			isLeftBrace := lexer.Lookahead == '{'
			advance(lexer)
			advancedOnce = true
			if (lexer.Lookahead == '{' && isLeftBrace) || (lexer.Lookahead == '}' && !isLeftBrace) {
				advance(lexer)
				lexer.MarkEnd()
				lexer.ResultSymbol = EscapeInterpolation
				return true
			}
			return false
		}
	}

	// STRING_CONTENT
	if validSymbols[StringContent] && len(s.delimiters) > 0 && !errorRecoveryMode {
		d := &s.delimiters[len(s.delimiters)-1]
		endChar := d.endCharacter()
		hasContent := advancedOnce

		for !lexer.EOF() {
			if (advancedOnce || lexer.Lookahead == '{' || lexer.Lookahead == '}') && d.isFormat() {
				lexer.MarkEnd()
				lexer.ResultSymbol = StringContent
				return hasContent
			}
			if lexer.Lookahead == '\\' {
				if d.isRaw() {
					advance(lexer)
					if lexer.Lookahead == d.endCharacter() || lexer.Lookahead == '\\' {
						advance(lexer)
					}
					if lexer.Lookahead == '\r' {
						advance(lexer)
						if lexer.Lookahead == '\n' {
							advance(lexer)
						}
					} else if lexer.Lookahead == '\n' {
						advance(lexer)
					}
					continue
				}
				if d.isBytes() {
					lexer.MarkEnd()
					advance(lexer)
					if lexer.Lookahead == 'N' || lexer.Lookahead == 'u' || lexer.Lookahead == 'U' {
						advance(lexer)
						hasContent = true
						continue
					} else {
						lexer.ResultSymbol = StringContent
						return hasContent
					}
				}
				// Normal escape
				lexer.MarkEnd()
				lexer.ResultSymbol = StringContent
				return hasContent
			} else if lexer.Lookahead == endChar {
				if d.isTriple() {
					lexer.MarkEnd()
					advance(lexer)
					if lexer.Lookahead == endChar {
						advance(lexer)
						if lexer.Lookahead == endChar {
							if hasContent {
								lexer.ResultSymbol = StringContent
							} else {
								advance(lexer)
								lexer.MarkEnd()
								s.delimiters = s.delimiters[:len(s.delimiters)-1]
								lexer.ResultSymbol = StringEnd
								s.insideInterpolatedString = false
							}
							return true
						}
						lexer.MarkEnd()
						lexer.ResultSymbol = StringContent
						return true
					}
					lexer.MarkEnd()
					lexer.ResultSymbol = StringContent
					return true
				}
				// Non-triple
				if hasContent {
					lexer.ResultSymbol = StringContent
				} else {
					advance(lexer)
					s.delimiters = s.delimiters[:len(s.delimiters)-1]
					lexer.ResultSymbol = StringEnd
					s.insideInterpolatedString = false
				}
				lexer.MarkEnd()
				return true
			} else if lexer.Lookahead == '\n' && hasContent && !d.isTriple() {
				return false
			}
			advance(lexer)
			hasContent = true
		}
	}

	// Whitespace/newline/indent/dedent processing
	lexer.MarkEnd()

	foundEndOfLine := false
	indentLength := uint16(0)
	firstCommentIndentLength := int32(-1)

	for {
		switch {
		case lexer.Lookahead == '\n':
			foundEndOfLine = true
			indentLength = 0
			skip(lexer)
		case lexer.Lookahead == ' ':
			indentLength++
			skip(lexer)
		case lexer.Lookahead == '\r' || lexer.Lookahead == '\f':
			indentLength = 0
			skip(lexer)
		case lexer.Lookahead == '\t':
			indentLength += 8
			skip(lexer)
		case lexer.Lookahead == '#' && (validSymbols[Indent] || validSymbols[Dedent] ||
			validSymbols[Newline] || validSymbols[Except]):
			if !foundEndOfLine {
				return false
			}
			if firstCommentIndentLength == -1 {
				firstCommentIndentLength = int32(indentLength)
			}
			for !lexer.EOF() && lexer.Lookahead != '\n' {
				skip(lexer)
			}
			skip(lexer)
			indentLength = 0
		case lexer.Lookahead == '\\':
			skip(lexer)
			if lexer.Lookahead == '\r' {
				skip(lexer)
			}
			if lexer.Lookahead == '\n' || lexer.EOF() {
				skip(lexer)
			} else {
				return false
			}
		case lexer.EOF():
			indentLength = 0
			foundEndOfLine = true
			goto doneWhitespace
		default:
			goto doneWhitespace
		}
	}
doneWhitespace:

	if foundEndOfLine {
		if len(s.indents) > 0 {
			currentIndent := s.indents[len(s.indents)-1]

			if validSymbols[Indent] && indentLength > currentIndent {
				s.indents = append(s.indents, indentLength)
				lexer.ResultSymbol = Indent
				return true
			}

			nextTokIsStringStart :=
				lexer.Lookahead == '"' || lexer.Lookahead == '\'' || lexer.Lookahead == '`'

			if (validSymbols[Dedent] ||
				(!validSymbols[Newline] && !(validSymbols[StringStart] && nextTokIsStringStart) &&
					!withinBrackets)) &&
				indentLength < currentIndent && !s.insideInterpolatedString &&
				firstCommentIndentLength < int32(currentIndent) {
				s.indents = s.indents[:len(s.indents)-1]
				lexer.ResultSymbol = Dedent
				return true
			}
		}

		if validSymbols[Newline] && !errorRecoveryMode {
			lexer.ResultSymbol = Newline
			return true
		}
	}

	// STRING_START
	if firstCommentIndentLength == -1 && validSymbols[StringStart] {
		var d delimiter

		hasFlags := false
		for !lexer.EOF() {
			switch {
			case lexer.Lookahead == 'f' || lexer.Lookahead == 'F' ||
				lexer.Lookahead == 't' || lexer.Lookahead == 'T':
				d.setFormat()
			case lexer.Lookahead == 'r' || lexer.Lookahead == 'R':
				d.setRaw()
			case lexer.Lookahead == 'b' || lexer.Lookahead == 'B':
				d.setBytes()
			case lexer.Lookahead == 'u' || lexer.Lookahead == 'U':
				// Just a prefix, no flag to set.
			default:
				goto doneFlags
			}
			hasFlags = true
			advance(lexer)
		}
	doneFlags:

		if lexer.Lookahead == '`' {
			d.setEndCharacter('`')
			advance(lexer)
			lexer.MarkEnd()
		} else if lexer.Lookahead == '\'' {
			d.setEndCharacter('\'')
			advance(lexer)
			lexer.MarkEnd()
			if lexer.Lookahead == '\'' {
				advance(lexer)
				if lexer.Lookahead == '\'' {
					advance(lexer)
					lexer.MarkEnd()
					d.setTriple()
				}
			}
		} else if lexer.Lookahead == '"' {
			d.setEndCharacter('"')
			advance(lexer)
			lexer.MarkEnd()
			if lexer.Lookahead == '"' {
				advance(lexer)
				if lexer.Lookahead == '"' {
					advance(lexer)
					lexer.MarkEnd()
					d.setTriple()
				}
			}
		}

		if d.endCharacter() != 0 {
			s.delimiters = append(s.delimiters, d)
			lexer.ResultSymbol = StringStart
			s.insideInterpolatedString = d.isFormat()
			return true
		}
		if hasFlags {
			return false
		}
	}

	return false
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func advance(lexer *ts.Lexer) {
	lexer.Advance(false)
}

func skip(lexer *ts.Lexer) {
	lexer.Advance(true)
}
