// Package cpp provides the external scanner for tree-sitter-cpp.
//
// Ported from tree-sitter-cpp/src/scanner.c.
// The C++ external scanner handles 2 context-sensitive token types for raw
// string literals (R"delimiter(content)delimiter"):
//   - RAW_STRING_DELIMITER: The d-char-sequence before ( and after )
//   - RAW_STRING_CONTENT: The body between ( and )delimiter"
//
// The scanner tracks the opening delimiter to match against the closing one.
//
// NOTE: The C implementation stores delimiters as wchar_t (platform-dependent
// width). In Go, we store them as int32 (runes) which matches wchar_t on
// Unix systems (4 bytes). Serialization uses little-endian uint32 encoding
// for each rune, matching the C memcpy behavior on little-endian systems.
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
package cpp

import (
	"encoding/binary"

	ts "github.com/treesitter-go/treesitter"
)

// Token types matching the externals array in grammar.js.
const (
	RawStringDelimiter = iota
	RawStringContent
)

// maxDelimiterLength is the C++ spec limit for raw string delimiters.
const maxDelimiterLength = 16

// wcharSize is the size of wchar_t for serialization compatibility.
// On Unix/macOS (where this code runs), wchar_t is 4 bytes.
const wcharSize = 4

// Scanner implements ts.ExternalScanner for C++.
type Scanner struct {
	delimiter []int32
}

// New creates a new C++ external scanner.
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner state to buf.
// Format: delimiter runes encoded as little-endian uint32 (wchar_t compat).
func (s *Scanner) Serialize(buf []byte) uint32 {
	size := uint32(0)
	for _, r := range s.delimiter {
		if int(size)+wcharSize > len(buf) {
			break
		}
		binary.LittleEndian.PutUint32(buf[size:], uint32(r))
		size += wcharSize
	}
	return size
}

// Deserialize restores the scanner state from data.
func (s *Scanner) Deserialize(data []byte) {
	s.delimiter = nil
	if len(data) == 0 || len(data)%wcharSize != 0 {
		return
	}
	count := len(data) / wcharSize
	s.delimiter = make([]int32, count)
	for i := 0; i < count; i++ {
		s.delimiter[i] = int32(binary.LittleEndian.Uint32(data[i*wcharSize:]))
	}
}

// Scan dispatches to the appropriate scanning function based on validSymbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	if validSymbols[RawStringDelimiter] && validSymbols[RawStringContent] {
		// Error recovery mode.
		return false
	}

	if validSymbols[RawStringDelimiter] {
		lexer.ResultSymbol = RawStringDelimiter
		return s.scanRawStringDelimiter(lexer)
	}

	if validSymbols[RawStringContent] {
		lexer.ResultSymbol = RawStringContent
		return s.scanRawStringContent(lexer)
	}

	return false
}

// scanRawStringDelimiter scans the d-char-sequence in R"delim(...)delim".
func (s *Scanner) scanRawStringDelimiter(lexer *ts.Lexer) bool {
	if len(s.delimiter) > 0 {
		// Closing delimiter: must exactly match the opening delimiter.
		for i := 0; i < len(s.delimiter); i++ {
			if lexer.Lookahead != s.delimiter[i] {
				return false
			}
			advance(lexer)
		}
		s.delimiter = nil
		return true
	}

	// Opening delimiter: record the d-char-sequence up to (.
	// d-char is any basic character except parens, backslashes, and spaces.
	for {
		if len(s.delimiter) >= maxDelimiterLength || lexer.EOF() ||
			lexer.Lookahead == '\\' || isSpace(lexer.Lookahead) {
			return false
		}
		if lexer.Lookahead == '(' {
			// Rather than create a token for an empty delimiter, we fail and
			// let the grammar fall back to a delimiter-less rule.
			return len(s.delimiter) > 0
		}
		s.delimiter = append(s.delimiter, lexer.Lookahead)
		advance(lexer)
	}
}

// scanRawStringContent scans the body between ( and )delimiter".
func (s *Scanner) scanRawStringContent(lexer *ts.Lexer) bool {
	delimiterIndex := -1

	for {
		if lexer.EOF() {
			lexer.MarkEnd()
			return true
		}

		if delimiterIndex >= 0 {
			if delimiterIndex == len(s.delimiter) {
				if lexer.Lookahead == '"' {
					return true
				}
				delimiterIndex = -1
			} else {
				if lexer.Lookahead == s.delimiter[delimiterIndex] {
					delimiterIndex++
				} else {
					delimiterIndex = -1
				}
			}
		}

		if delimiterIndex == -1 && lexer.Lookahead == ')' {
			// The content doesn't include the )delimiter" part.
			// We must still scan through it, but exclude it from the token.
			lexer.MarkEnd()
			delimiterIndex = 0
		}

		advance(lexer)
	}
}

func advance(lexer *ts.Lexer) {
	lexer.Advance(false)
}

func isSpace(ch int32) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '\v'
}
