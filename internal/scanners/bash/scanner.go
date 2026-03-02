// Package bash provides the external scanner for tree-sitter-bash.
//
// Ported from tree-sitter-bash/src/scanner.c.
// The Bash external scanner handles 29 context-sensitive token types including:
//   - Heredocs (start, body, content, end) with raw/interpolated/indented variants
//   - File descriptors (numeric redirections like 2>)
//   - Variable names and empty values
//   - Concatenation tokens
//   - Test operators (-eq, -ne, etc.)
//   - Regex patterns (for [[ ]] contexts)
//   - Extended glob patterns (@(), *(), +(), etc.)
//   - Expansion words
//   - Bare dollar signs
//   - Brace expansions ({1..10})
//   - Parameter expansion operators (##, !, =)
//   - Heredoc arrows (<<, <<-)
//   - Newlines, opening parens, esac
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
// All EOF checks use lexer.EOF() instead of checking for 0.
package bash

import (
	"encoding/binary"
	"unicode"
	"unicode/utf8"

	ts "github.com/treesitter-go/treesitter"
)

// Token types matching the externals array in grammar.js.
const (
	HeredocStart = iota
	SimpleHeredocBody
	HeredocBodyBeginning
	HeredocContent
	HeredocEnd
	FileDescriptor
	EmptyValue
	Concat
	VariableName
	TestOperator
	Regex
	RegexNoSlash
	RegexNoSpace
	ExpansionWord
	ExtglobPattern
	BareDollar
	BraceStart
	ImmediateDoubleHash
	ExternalExpansionSymHash
	ExternalExpansionSymBang
	ExternalExpansionSymEqual
	ClosingBrace
	ClosingBracket
	HeredocArrow
	HeredocArrowDash
	Newline
	OpeningParen
	Esac
	ErrorRecovery
)

// heredoc represents a pending heredoc with its delimiter and state.
type heredoc struct {
	isRaw              bool
	started            bool
	allowsIndent       bool
	delimiter          []byte
	currentLeadingWord []byte
}

// Scanner implements ts.ExternalScanner for Bash.
type Scanner struct {
	lastGlobParenDepth  uint8
	extWasInDoubleQuote bool
	extSawOutsideQuote  bool
	heredocs            []heredoc
}

// New creates a new Bash external scanner (ts.ExternalScannerFactory).
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner state to buf and returns the number of bytes written.
func (s *Scanner) Serialize(buf []byte) uint32 {
	size := uint32(0)

	if len(buf) < 4 {
		return 0
	}

	buf[size] = s.lastGlobParenDepth
	size++
	buf[size] = boolByte(s.extWasInDoubleQuote)
	size++
	buf[size] = boolByte(s.extSawOutsideQuote)
	size++
	buf[size] = byte(len(s.heredocs))
	size++

	for i := range s.heredocs {
		h := &s.heredocs[i]
		needed := 3 + 4 + len(h.delimiter)
		if int(size)+needed > len(buf) {
			return 0
		}

		buf[size] = boolByte(h.isRaw)
		size++
		buf[size] = boolByte(h.started)
		size++
		buf[size] = boolByte(h.allowsIndent)
		size++

		delimLen := uint32(len(h.delimiter))
		binary.LittleEndian.PutUint32(buf[size:], delimLen)
		size += 4
		if delimLen > 0 {
			copy(buf[size:], h.delimiter)
			size += delimLen
		}
	}

	return size
}

// Deserialize restores the scanner state from data.
func (s *Scanner) Deserialize(data []byte) {
	if len(data) < 4 {
		s.reset()
		return
	}

	size := uint32(0)
	n := uint32(len(data))

	s.lastGlobParenDepth = data[size]
	size++
	s.extWasInDoubleQuote = data[size] != 0
	size++
	s.extSawOutsideQuote = data[size] != 0
	size++
	heredocCount := int(data[size])
	size++

	// Grow or shrink the heredocs slice.
	for len(s.heredocs) < heredocCount {
		s.heredocs = append(s.heredocs, heredoc{})
	}
	s.heredocs = s.heredocs[:heredocCount]

	for i := 0; i < heredocCount; i++ {
		if size+7 > n { // 3 bools + 4 byte delimLen
			s.heredocs = s.heredocs[:i]
			break
		}
		h := &s.heredocs[i]
		h.isRaw = data[size] != 0
		size++
		h.started = data[size] != 0
		size++
		h.allowsIndent = data[size] != 0
		size++

		delimLen := binary.LittleEndian.Uint32(data[size:])
		size += 4
		if delimLen > n-size {
			h.delimiter = nil
			s.heredocs = s.heredocs[:i+1]
			break
		}
		if delimLen > 0 {
			h.delimiter = make([]byte, delimLen)
			copy(h.delimiter, data[size:size+delimLen])
			size += delimLen
		} else {
			h.delimiter = nil
		}
	}
}

// Scan dispatches to the appropriate scanning function based on validSymbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	return s.scan(lexer, validSymbols)
}

func (s *Scanner) reset() {
	s.lastGlobParenDepth = 0
	s.extWasInDoubleQuote = false
	s.extSawOutsideQuote = false
	s.heredocs = nil
}

func (s *Scanner) resetHeredoc(h *heredoc) {
	h.isRaw = false
	h.started = false
	h.allowsIndent = false
	h.delimiter = nil
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func inErrorRecovery(validSymbols []bool) bool {
	return validSymbols[ErrorRecovery]
}

// advance moves past the current lookahead without skipping (character is part of token).
func advance(lexer *ts.Lexer) {
	lexer.Advance(false)
}

// skip moves past the current lookahead as whitespace (not part of token).
func skip(lexer *ts.Lexer) {
	lexer.Advance(true)
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

// advanceWord consumes a "word" in POSIX parlance and returns it unquoted.
// This is an approximate implementation that doesn't deal with POSIX-mandated
// substitution, and assumes the default value for IFS.
func advanceWord(lexer *ts.Lexer) ([]byte, bool) {
	var unquotedWord []byte
	empty := true

	var quote int32
	if lexer.Lookahead == '\'' || lexer.Lookahead == '"' {
		quote = lexer.Lookahead
		advance(lexer)
	}

	for !lexer.EOF() {
		if quote != 0 {
			if lexer.Lookahead == quote || lexer.Lookahead == '\r' || lexer.Lookahead == '\n' {
				break
			}
		} else {
			if isSpace(lexer.Lookahead) {
				break
			}
		}

		if lexer.Lookahead == '\\' {
			advance(lexer)
			if lexer.EOF() {
				return unquotedWord, false
			}
		}
		empty = false
		if lexer.Lookahead >= 0 {
			var encBuf [4]byte
			n := utf8.EncodeRune(encBuf[:], rune(lexer.Lookahead))
			unquotedWord = append(unquotedWord, encBuf[:n]...)
		}
		advance(lexer)
	}

	// C appends '\0' to the word (included in String.size for serialization).
	unquotedWord = append(unquotedWord, 0)

	if quote != 0 && lexer.Lookahead == quote {
		advance(lexer)
	}

	return unquotedWord, !empty
}

func (s *Scanner) scanBareDollar(lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) && lexer.Lookahead != '\n' && !lexer.EOF() {
		skip(lexer)
	}

	if lexer.Lookahead == '$' {
		advance(lexer)
		lexer.ResultSymbol = BareDollar
		lexer.MarkEnd()
		return isSpace(lexer.Lookahead) || lexer.EOF() || lexer.Lookahead == '"'
	}

	return false
}

func (s *Scanner) scanHeredocStart(h *heredoc, lexer *ts.Lexer) bool {
	for isSpace(lexer.Lookahead) {
		skip(lexer)
	}

	lexer.ResultSymbol = HeredocStart
	h.isRaw = lexer.Lookahead == '\'' || lexer.Lookahead == '"' || lexer.Lookahead == '\\'

	delimiter, found := advanceWord(lexer)
	if !found {
		h.delimiter = nil
		return false
	}
	h.delimiter = delimiter
	return true
}

func (s *Scanner) scanHeredocEndIdentifier(h *heredoc, lexer *ts.Lexer) bool {
	h.currentLeadingWord = nil

	if len(h.delimiter) == 0 {
		return false
	}

	// Scan characters to see if they match the heredoc delimiter.
	// Delimiter is stored as UTF-8 bytes; Lookahead is a Unicode codepoint.
	// Decode runes from the delimiter to compare with the codepoint.
	offset := 0
	for !lexer.EOF() && lexer.Lookahead != '\n' && offset < len(h.delimiter) {
		r, runeSize := utf8.DecodeRune(h.delimiter[offset:])
		if r != rune(lexer.Lookahead) {
			break
		}
		var encBuf [4]byte
		n := utf8.EncodeRune(encBuf[:], rune(lexer.Lookahead))
		h.currentLeadingWord = append(h.currentLeadingWord, encBuf[:n]...)
		advance(lexer)
		offset += runeSize
	}

	// C appends '\0' to current_leading_word, then uses strcmp (which
	// compares up to the first null). Our delimiter also has '\0' from
	// advanceWord, so including it here makes the comparison match.
	h.currentLeadingWord = append(h.currentLeadingWord, 0)

	return string(h.currentLeadingWord) == string(h.delimiter)
}

func (s *Scanner) scanHeredocContent(lexer *ts.Lexer, middleType, endType int) bool {
	didAdvance := false
	h := &s.heredocs[len(s.heredocs)-1]

	for {
		switch {
		case lexer.EOF():
			if didAdvance {
				s.resetHeredoc(h)
				lexer.ResultSymbol = ts.Symbol(endType)
				return true
			}
			return false

		case lexer.Lookahead == '\\':
			didAdvance = true
			advance(lexer)
			if !lexer.EOF() {
				advance(lexer)
			}

		case lexer.Lookahead == '$':
			if h.isRaw {
				didAdvance = true
				advance(lexer)
				continue
			}
			if didAdvance {
				lexer.MarkEnd()
				lexer.ResultSymbol = ts.Symbol(middleType)
				h.started = true
				advance(lexer)
				if isAlpha(lexer.Lookahead) || lexer.Lookahead == '{' || lexer.Lookahead == '(' {
					return true
				}
				continue
			}
			if middleType == HeredocBodyBeginning && lexer.CurrentPosition().Point.Column == 0 {
				lexer.ResultSymbol = ts.Symbol(middleType)
				h.started = true
				return true
			}
			return false

		case lexer.Lookahead == '\n':
			if !didAdvance {
				skip(lexer)
			} else {
				advance(lexer)
			}
			didAdvance = true
			if h.allowsIndent {
				for isSpace(lexer.Lookahead) {
					advance(lexer)
				}
			}
			if h.started {
				lexer.ResultSymbol = ts.Symbol(middleType)
			} else {
				lexer.ResultSymbol = ts.Symbol(endType)
			}
			lexer.MarkEnd()
			if s.scanHeredocEndIdentifier(h, lexer) {
				if lexer.ResultSymbol == ts.Symbol(HeredocEnd) {
					s.heredocs = s.heredocs[:len(s.heredocs)-1]
				}
				return true
			}

		default:
			if lexer.CurrentPosition().Point.Column == 0 {
				for isSpace(lexer.Lookahead) {
					if didAdvance {
						advance(lexer)
					} else {
						skip(lexer)
					}
				}
				if endType != SimpleHeredocBody {
					lexer.ResultSymbol = ts.Symbol(middleType)
					if s.scanHeredocEndIdentifier(h, lexer) {
						return true
					}
				}
				if endType == SimpleHeredocBody {
					lexer.ResultSymbol = ts.Symbol(endType)
					lexer.MarkEnd()
					if s.scanHeredocEndIdentifier(h, lexer) {
						return true
					}
				}
			}
			didAdvance = true
			advance(lexer)
		}
	}
}

func (s *Scanner) scan(lexer *ts.Lexer, validSymbols []bool) bool {
	// CONCAT
	if validSymbols[Concat] && !inErrorRecovery(validSymbols) {
		if !(lexer.EOF() || isSpace(lexer.Lookahead) || lexer.Lookahead == '>' ||
			lexer.Lookahead == '<' || lexer.Lookahead == ')' || lexer.Lookahead == '(' ||
			lexer.Lookahead == ';' || lexer.Lookahead == '&' || lexer.Lookahead == '|' ||
			(lexer.Lookahead == '}' && validSymbols[ClosingBrace]) ||
			(lexer.Lookahead == ']' && validSymbols[ClosingBracket])) {
			lexer.ResultSymbol = Concat

			// For a`b`, check if the 2nd backtick has whitespace after it.
			if lexer.Lookahead == '`' {
				lexer.MarkEnd()
				advance(lexer)
				for lexer.Lookahead != '`' && !lexer.EOF() {
					advance(lexer)
				}
				if lexer.EOF() {
					return false
				}
				if lexer.Lookahead == '`' {
					advance(lexer)
				}
				return isSpace(lexer.Lookahead) || lexer.EOF()
			}

			// Strings with expansions containing escaped quotes or backslashes.
			if lexer.Lookahead == '\\' {
				lexer.MarkEnd()
				advance(lexer)
				if lexer.Lookahead == '"' || lexer.Lookahead == '\'' || lexer.Lookahead == '\\' {
					return true
				}
				if lexer.EOF() {
					return false
				}
			} else {
				return true
			}
		}
		if isSpace(lexer.Lookahead) && validSymbols[ClosingBrace] && !validSymbols[ExpansionWord] {
			lexer.ResultSymbol = Concat
			return true
		}
	}

	// IMMEDIATE_DOUBLE_HASH
	if validSymbols[ImmediateDoubleHash] && !inErrorRecovery(validSymbols) {
		if lexer.Lookahead == '#' {
			lexer.MarkEnd()
			advance(lexer)
			if lexer.Lookahead == '#' {
				advance(lexer)
				if lexer.Lookahead != '}' {
					lexer.ResultSymbol = ImmediateDoubleHash
					lexer.MarkEnd()
					return true
				}
			}
		}
	}

	// EXTERNAL_EXPANSION_SYM_HASH / BANG / EQUAL
	if validSymbols[ExternalExpansionSymHash] && !inErrorRecovery(validSymbols) {
		if lexer.Lookahead == '#' || lexer.Lookahead == '=' || lexer.Lookahead == '!' {
			if lexer.Lookahead == '#' {
				lexer.ResultSymbol = ExternalExpansionSymHash
			} else if lexer.Lookahead == '!' {
				lexer.ResultSymbol = ExternalExpansionSymBang
			} else {
				lexer.ResultSymbol = ExternalExpansionSymEqual
			}
			advance(lexer)
			lexer.MarkEnd()
			for lexer.Lookahead == '#' || lexer.Lookahead == '=' || lexer.Lookahead == '!' {
				advance(lexer)
			}
			for isSpace(lexer.Lookahead) {
				skip(lexer)
			}
			return lexer.Lookahead == '}'
		}
	}

	// EMPTY_VALUE
	if validSymbols[EmptyValue] {
		if isSpace(lexer.Lookahead) || lexer.EOF() || lexer.Lookahead == ';' || lexer.Lookahead == '&' {
			lexer.ResultSymbol = EmptyValue
			return true
		}
	}

	// HEREDOC_BODY_BEGINNING / SIMPLE_HEREDOC_BODY
	if (validSymbols[HeredocBodyBeginning] || validSymbols[SimpleHeredocBody]) &&
		len(s.heredocs) > 0 && !s.heredocs[len(s.heredocs)-1].started &&
		!inErrorRecovery(validSymbols) {
		return s.scanHeredocContent(lexer, HeredocBodyBeginning, SimpleHeredocBody)
	}

	// HEREDOC_END
	if validSymbols[HeredocEnd] && len(s.heredocs) > 0 {
		h := &s.heredocs[len(s.heredocs)-1]
		if s.scanHeredocEndIdentifier(h, lexer) {
			h.currentLeadingWord = nil
			h.delimiter = nil
			s.heredocs = s.heredocs[:len(s.heredocs)-1]
			lexer.ResultSymbol = HeredocEnd
			return true
		}
	}

	// HEREDOC_CONTENT
	if validSymbols[HeredocContent] && len(s.heredocs) > 0 &&
		s.heredocs[len(s.heredocs)-1].started && !inErrorRecovery(validSymbols) {
		return s.scanHeredocContent(lexer, HeredocContent, HeredocEnd)
	}

	// HEREDOC_START
	if validSymbols[HeredocStart] && !inErrorRecovery(validSymbols) && len(s.heredocs) > 0 {
		return s.scanHeredocStart(&s.heredocs[len(s.heredocs)-1], lexer)
	}

	// TEST_OPERATOR
	if validSymbols[TestOperator] && !validSymbols[ExpansionWord] {
		for isSpace(lexer.Lookahead) && lexer.Lookahead != '\n' {
			skip(lexer)
		}

		if lexer.Lookahead == '\\' {
			if validSymbols[ExtglobPattern] {
				goto extglobPattern
			}
			if validSymbols[RegexNoSpace] {
				goto regexLabel
			}
			skip(lexer)

			if lexer.EOF() {
				return false
			}

			if lexer.Lookahead == '\r' {
				skip(lexer)
				if lexer.Lookahead == '\n' {
					skip(lexer)
				}
			} else if lexer.Lookahead == '\n' {
				skip(lexer)
			} else {
				return false
			}

			for isSpace(lexer.Lookahead) {
				skip(lexer)
			}
		}

		if lexer.Lookahead == '\n' && !validSymbols[Newline] {
			skip(lexer)
			for isSpace(lexer.Lookahead) {
				skip(lexer)
			}
		}

		if lexer.Lookahead == '-' {
			advance(lexer)

			advancedOnce := false
			for isAlpha(lexer.Lookahead) {
				advancedOnce = true
				advance(lexer)
			}

			if isSpace(lexer.Lookahead) && advancedOnce {
				lexer.MarkEnd()
				advance(lexer)
				if lexer.Lookahead == '}' && validSymbols[ClosingBrace] {
					if validSymbols[ExpansionWord] {
						lexer.MarkEnd()
						lexer.ResultSymbol = ExpansionWord
						return true
					}
					return false
				}
				lexer.ResultSymbol = TestOperator
				return true
			}
			if isSpace(lexer.Lookahead) && validSymbols[ExtglobPattern] {
				lexer.ResultSymbol = ExtglobPattern
				return true
			}
		}

		if validSymbols[BareDollar] && !inErrorRecovery(validSymbols) && s.scanBareDollar(lexer) {
			return true
		}
	}

	// VARIABLE_NAME / FILE_DESCRIPTOR / HEREDOC_ARROW
	if (validSymbols[VariableName] || validSymbols[FileDescriptor] || validSymbols[HeredocArrow]) &&
		!validSymbols[RegexNoSlash] && !inErrorRecovery(validSymbols) {
		for {
			if (lexer.Lookahead == ' ' || lexer.Lookahead == '\t' || lexer.Lookahead == '\r' ||
				(lexer.Lookahead == '\n' && !validSymbols[Newline])) &&
				!validSymbols[ExpansionWord] {
				skip(lexer)
			} else if lexer.Lookahead == '\\' {
				skip(lexer)

				if lexer.EOF() {
					lexer.MarkEnd()
					lexer.ResultSymbol = VariableName
					return true
				}

				if lexer.Lookahead == '\r' {
					skip(lexer)
				}
				if lexer.Lookahead == '\n' {
					skip(lexer)
				} else {
					if lexer.Lookahead == '\\' && validSymbols[ExpansionWord] {
						goto expansionWord
					}
					return false
				}
			} else {
				break
			}
		}

		// no '*', '@', '?', '-', '$', '0', '_'
		if !validSymbols[ExpansionWord] &&
			(lexer.Lookahead == '*' || lexer.Lookahead == '@' || lexer.Lookahead == '?' ||
				lexer.Lookahead == '-' || lexer.Lookahead == '0' || lexer.Lookahead == '_') {
			lexer.MarkEnd()
			advance(lexer)
			if lexer.Lookahead == '=' || lexer.Lookahead == '[' || lexer.Lookahead == ':' ||
				lexer.Lookahead == '-' || lexer.Lookahead == '%' || lexer.Lookahead == '#' ||
				lexer.Lookahead == '/' {
				return false
			}
			if validSymbols[ExtglobPattern] && isSpace(lexer.Lookahead) {
				lexer.MarkEnd()
				lexer.ResultSymbol = ExtglobPattern
				return true
			}
		}

		// HEREDOC_ARROW
		if validSymbols[HeredocArrow] && lexer.Lookahead == '<' {
			advance(lexer)
			if lexer.Lookahead == '<' {
				advance(lexer)
				if lexer.Lookahead == '-' {
					advance(lexer)
					h := heredoc{allowsIndent: true}
					s.heredocs = append(s.heredocs, h)
					lexer.ResultSymbol = HeredocArrowDash
				} else if lexer.Lookahead == '<' || lexer.Lookahead == '=' {
					return false
				} else {
					h := heredoc{}
					s.heredocs = append(s.heredocs, h)
					lexer.ResultSymbol = HeredocArrow
				}
				return true
			}
			return false
		}

		isNumber := true
		if isDigit(lexer.Lookahead) {
			advance(lexer)
		} else if isAlpha(lexer.Lookahead) || lexer.Lookahead == '_' {
			isNumber = false
			advance(lexer)
		} else {
			if lexer.Lookahead == '{' {
				goto braceStart
			}
			if validSymbols[ExpansionWord] {
				goto expansionWord
			}
			if validSymbols[ExtglobPattern] {
				goto extglobPattern
			}
			return false
		}

		for {
			if isDigit(lexer.Lookahead) {
				advance(lexer)
			} else if isAlpha(lexer.Lookahead) || lexer.Lookahead == '_' {
				isNumber = false
				advance(lexer)
			} else {
				break
			}
		}

		if isNumber && validSymbols[FileDescriptor] && (lexer.Lookahead == '>' || lexer.Lookahead == '<') {
			lexer.ResultSymbol = FileDescriptor
			return true
		}

		if validSymbols[VariableName] {
			if lexer.Lookahead == '+' {
				lexer.MarkEnd()
				advance(lexer)
				if lexer.Lookahead == '=' || lexer.Lookahead == ':' || validSymbols[ClosingBrace] {
					lexer.ResultSymbol = VariableName
					return true
				}
				return false
			}
			if lexer.Lookahead == '/' {
				return false
			}
			if lexer.Lookahead == '=' || lexer.Lookahead == '[' ||
				(lexer.Lookahead == ':' && !validSymbols[ClosingBrace] && !validSymbols[OpeningParen]) ||
				lexer.Lookahead == '%' ||
				(lexer.Lookahead == '#' && !isNumber) || lexer.Lookahead == '@' ||
				(lexer.Lookahead == '-' && validSymbols[ClosingBrace]) {
				lexer.MarkEnd()
				lexer.ResultSymbol = VariableName
				return true
			}

			if lexer.Lookahead == '?' {
				lexer.MarkEnd()
				advance(lexer)
				lexer.ResultSymbol = VariableName
				return isAlpha(lexer.Lookahead)
			}
		}

		return false
	}

	// BARE_DOLLAR (second chance)
	if validSymbols[BareDollar] && !inErrorRecovery(validSymbols) && s.scanBareDollar(lexer) {
		return true
	}

regexLabel:
	// REGEX / REGEX_NO_SLASH / REGEX_NO_SPACE
	if (validSymbols[Regex] || validSymbols[RegexNoSlash] || validSymbols[RegexNoSpace]) &&
		!inErrorRecovery(validSymbols) {
		if validSymbols[Regex] || validSymbols[RegexNoSpace] {
			for isSpace(lexer.Lookahead) {
				skip(lexer)
			}
		}

		if (lexer.Lookahead != '"' && lexer.Lookahead != '\'') ||
			((lexer.Lookahead == '$' || lexer.Lookahead == '\'') && validSymbols[RegexNoSlash]) ||
			(lexer.Lookahead == '\'' && validSymbols[RegexNoSpace]) {

			if lexer.Lookahead == '$' && validSymbols[RegexNoSlash] {
				lexer.MarkEnd()
				advance(lexer)
				if lexer.Lookahead == '(' {
					return false
				}
			}

			lexer.MarkEnd()

			type regexState struct {
				done                         bool
				advancedOnce                 bool
				foundNonAlnumDollarUnderDash bool
				lastWasEscape                bool
				inSingleQuote                bool
				parenDepth                   uint32
				bracketDepth                 uint32
				braceDepth                   uint32
			}
			st := regexState{}

			for !st.done {
				if st.inSingleQuote {
					if lexer.Lookahead == '\'' {
						st.inSingleQuote = false
						advance(lexer)
						lexer.MarkEnd()
					}
				}
				switch lexer.Lookahead {
				case '\\':
					st.lastWasEscape = true
				case -1: // EOF check
					return false
				case 0: // NUL
					return false
				case '(':
					st.parenDepth++
					st.lastWasEscape = false
				case '[':
					st.bracketDepth++
					st.lastWasEscape = false
				case '{':
					if !st.lastWasEscape {
						st.braceDepth++
					}
					st.lastWasEscape = false
				case ')':
					if st.parenDepth == 0 {
						st.done = true
					} else {
						st.parenDepth--
					}
					st.lastWasEscape = false
				case ']':
					if st.bracketDepth == 0 {
						st.done = true
					} else {
						st.bracketDepth--
					}
					st.lastWasEscape = false
				case '}':
					if st.braceDepth == 0 {
						st.done = true
					} else {
						st.braceDepth--
					}
					st.lastWasEscape = false
				case '\'':
					st.inSingleQuote = !st.inSingleQuote
					advance(lexer)
					st.advancedOnce = true
					st.lastWasEscape = false
					continue
				default:
					st.lastWasEscape = false
				}

				if !st.done {
					if validSymbols[Regex] {
						wasSpace := !st.inSingleQuote && isSpace(lexer.Lookahead)
						advance(lexer)
						st.advancedOnce = true
						if !wasSpace || st.parenDepth > 0 {
							lexer.MarkEnd()
						}
					} else if validSymbols[RegexNoSlash] {
						if lexer.Lookahead == '/' {
							lexer.MarkEnd()
							lexer.ResultSymbol = RegexNoSlash
							return st.advancedOnce
						}
						if lexer.Lookahead == '\\' {
							advance(lexer)
							st.advancedOnce = true
							if !lexer.EOF() && lexer.Lookahead != '[' && lexer.Lookahead != '/' {
								advance(lexer)
								lexer.MarkEnd()
							}
						} else {
							wasSpace := !st.inSingleQuote && isSpace(lexer.Lookahead)
							advance(lexer)
							st.advancedOnce = true
							if !wasSpace {
								lexer.MarkEnd()
							}
						}
					} else if validSymbols[RegexNoSpace] {
						if lexer.Lookahead == '\\' {
							st.foundNonAlnumDollarUnderDash = true
							advance(lexer)
							if !lexer.EOF() {
								advance(lexer)
							}
						} else if lexer.Lookahead == '$' {
							lexer.MarkEnd()
							advance(lexer)
							if lexer.Lookahead == '(' {
								return false
							}
							if isSpace(lexer.Lookahead) {
								lexer.ResultSymbol = RegexNoSpace
								lexer.MarkEnd()
								return true
							}
						} else {
							wasSpace := !st.inSingleQuote && isSpace(lexer.Lookahead)
							if wasSpace && st.parenDepth == 0 {
								lexer.MarkEnd()
								lexer.ResultSymbol = RegexNoSpace
								return st.foundNonAlnumDollarUnderDash
							}
							if !isAlnum(lexer.Lookahead) && lexer.Lookahead != '$' &&
								lexer.Lookahead != '-' && lexer.Lookahead != '_' {
								st.foundNonAlnumDollarUnderDash = true
							}
							advance(lexer)
						}
					}
				}
			}

			if validSymbols[RegexNoSlash] {
				lexer.ResultSymbol = RegexNoSlash
			} else if validSymbols[RegexNoSpace] {
				lexer.ResultSymbol = RegexNoSpace
			} else {
				lexer.ResultSymbol = Regex
			}
			if validSymbols[Regex] && !st.advancedOnce {
				return false
			}
			return true
		}
	}

extglobPattern:
	// EXTGLOB_PATTERN
	if validSymbols[ExtglobPattern] && !inErrorRecovery(validSymbols) {
		for isSpace(lexer.Lookahead) {
			skip(lexer)
		}

		if lexer.Lookahead == '?' || lexer.Lookahead == '*' || lexer.Lookahead == '+' ||
			lexer.Lookahead == '@' || lexer.Lookahead == '!' || lexer.Lookahead == '-' ||
			lexer.Lookahead == ')' || lexer.Lookahead == '\\' || lexer.Lookahead == '.' ||
			lexer.Lookahead == '[' || isAlpha(lexer.Lookahead) {

			if lexer.Lookahead == '\\' {
				advance(lexer)
				if (isSpace(lexer.Lookahead) || lexer.Lookahead == '"') &&
					lexer.Lookahead != '\r' && lexer.Lookahead != '\n' {
					advance(lexer)
				} else {
					return false
				}
			}

			if lexer.Lookahead == ')' && s.lastGlobParenDepth == 0 {
				lexer.MarkEnd()
				advance(lexer)
				if isSpace(lexer.Lookahead) {
					return false
				}
			}

			lexer.MarkEnd()
			wasNonAlpha := !isAlpha(lexer.Lookahead)

			if lexer.Lookahead != '[' {
				// no esac
				if lexer.Lookahead == 'e' {
					lexer.MarkEnd()
					advance(lexer)
					if lexer.Lookahead == 's' {
						advance(lexer)
						if lexer.Lookahead == 'a' {
							advance(lexer)
							if lexer.Lookahead == 'c' {
								advance(lexer)
								if isSpace(lexer.Lookahead) {
									return false
								}
							}
						}
					}
				} else {
					advance(lexer)
				}
			}

			// -\w is just a word, find something else special
			if lexer.Lookahead == '-' {
				lexer.MarkEnd()
				advance(lexer)
				for isAlnum(lexer.Lookahead) {
					advance(lexer)
				}
				if lexer.Lookahead == ')' || lexer.Lookahead == '\\' || lexer.Lookahead == '.' {
					return false
				}
				lexer.MarkEnd()
			}

			// case item -) or *)
			if lexer.Lookahead == ')' && s.lastGlobParenDepth == 0 {
				lexer.MarkEnd()
				advance(lexer)
				if isSpace(lexer.Lookahead) {
					lexer.ResultSymbol = ExtglobPattern
					return wasNonAlpha
				}
			}

			if isSpace(lexer.Lookahead) {
				lexer.MarkEnd()
				lexer.ResultSymbol = ExtglobPattern
				s.lastGlobParenDepth = 0
				return true
			}

			if lexer.Lookahead == '$' {
				lexer.MarkEnd()
				advance(lexer)
				if lexer.Lookahead == '{' || lexer.Lookahead == '(' {
					lexer.ResultSymbol = ExtglobPattern
					return true
				}
			}

			if lexer.Lookahead == '|' {
				lexer.MarkEnd()
				advance(lexer)
				lexer.ResultSymbol = ExtglobPattern
				return true
			}

			if !isAlnum(lexer.Lookahead) && lexer.Lookahead != '(' && lexer.Lookahead != '"' &&
				lexer.Lookahead != '[' && lexer.Lookahead != '?' && lexer.Lookahead != '/' &&
				lexer.Lookahead != '\\' && lexer.Lookahead != '_' && lexer.Lookahead != '*' {
				return false
			}

			type extglobState struct {
				done           bool
				sawNonAlphaDot bool
				parenDepth     uint32
				bracketDepth   uint32
				braceDepth     uint32
			}
			est := extglobState{
				sawNonAlphaDot: wasNonAlpha,
				parenDepth:     uint32(s.lastGlobParenDepth),
			}

			for !est.done {
				switch lexer.Lookahead {
				case -1, 0:
					return false
				case '(':
					est.parenDepth++
				case '[':
					est.bracketDepth++
				case '{':
					est.braceDepth++
				case ')':
					if est.parenDepth == 0 {
						est.done = true
					} else {
						est.parenDepth--
					}
				case ']':
					if est.bracketDepth == 0 {
						est.done = true
					} else {
						est.bracketDepth--
					}
				case '}':
					if est.braceDepth == 0 {
						est.done = true
					} else {
						est.braceDepth--
					}
				}

				if lexer.Lookahead == '|' {
					lexer.MarkEnd()
					advance(lexer)
					if est.parenDepth == 0 && est.bracketDepth == 0 && est.braceDepth == 0 {
						lexer.ResultSymbol = ExtglobPattern
						return true
					}
				}

				if !est.done {
					wasSpace := isSpace(lexer.Lookahead)
					if lexer.Lookahead == '$' {
						lexer.MarkEnd()
						if !isAlpha(lexer.Lookahead) && lexer.Lookahead != '.' && lexer.Lookahead != '\\' {
							est.sawNonAlphaDot = true
						}
						advance(lexer)
						if lexer.Lookahead == '(' || lexer.Lookahead == '{' {
							lexer.ResultSymbol = ExtglobPattern
							s.lastGlobParenDepth = uint8(est.parenDepth)
							return est.sawNonAlphaDot
						}
					}
					if wasSpace {
						lexer.MarkEnd()
						lexer.ResultSymbol = ExtglobPattern
						s.lastGlobParenDepth = 0
						return est.sawNonAlphaDot
					}
					if lexer.Lookahead == '"' {
						lexer.MarkEnd()
						lexer.ResultSymbol = ExtglobPattern
						s.lastGlobParenDepth = 0
						return est.sawNonAlphaDot
					}
					if lexer.Lookahead == '\\' {
						if !isAlpha(lexer.Lookahead) && lexer.Lookahead != '.' && lexer.Lookahead != '\\' {
							est.sawNonAlphaDot = true
						}
						advance(lexer)
						if isSpace(lexer.Lookahead) || lexer.Lookahead == '"' {
							advance(lexer)
						}
					} else {
						if !isAlpha(lexer.Lookahead) && lexer.Lookahead != '.' && lexer.Lookahead != '\\' {
							est.sawNonAlphaDot = true
						}
						advance(lexer)
					}
					if !wasSpace {
						lexer.MarkEnd()
					}
				}
			}

			lexer.ResultSymbol = ExtglobPattern
			s.lastGlobParenDepth = 0
			return est.sawNonAlphaDot
		}
		s.lastGlobParenDepth = 0
		return false
	}

expansionWord:
	// EXPANSION_WORD
	if validSymbols[ExpansionWord] {
		advancedOnce := false
		advanceOnceSpace := false
		for {
			if lexer.Lookahead == '"' {
				return false
			}
			if lexer.Lookahead == '$' {
				lexer.MarkEnd()
				advance(lexer)
				if lexer.Lookahead == '{' || lexer.Lookahead == '(' || lexer.Lookahead == '\'' ||
					isAlnum(lexer.Lookahead) {
					lexer.ResultSymbol = ExpansionWord
					return advancedOnce
				}
				advancedOnce = true
			}

			if lexer.Lookahead == '}' {
				lexer.MarkEnd()
				lexer.ResultSymbol = ExpansionWord
				return advancedOnce || advanceOnceSpace
			}

			if lexer.Lookahead == '(' && !(advancedOnce || advanceOnceSpace) {
				lexer.MarkEnd()
				advance(lexer)
				for lexer.Lookahead != ')' && !lexer.EOF() {
					if lexer.Lookahead == '$' {
						lexer.MarkEnd()
						advance(lexer)
						if lexer.Lookahead == '{' || lexer.Lookahead == '(' || lexer.Lookahead == '\'' ||
							isAlnum(lexer.Lookahead) {
							lexer.ResultSymbol = ExpansionWord
							return advancedOnce
						}
						advancedOnce = true
					} else {
						advancedOnce = advancedOnce || !isSpace(lexer.Lookahead)
						advanceOnceSpace = advanceOnceSpace || isSpace(lexer.Lookahead)
						advance(lexer)
					}
				}
				lexer.MarkEnd()
				if lexer.Lookahead == ')' {
					advancedOnce = true
					advance(lexer)
					lexer.MarkEnd()
					if lexer.Lookahead == '}' {
						return false
					}
				} else {
					return false
				}
			}

			if lexer.Lookahead == '\'' {
				return false
			}

			if lexer.EOF() {
				return false
			}
			advancedOnce = advancedOnce || !isSpace(lexer.Lookahead)
			advanceOnceSpace = advanceOnceSpace || isSpace(lexer.Lookahead)
			advance(lexer)
		}
	}

braceStart:
	// BRACE_START
	if validSymbols[BraceStart] && !inErrorRecovery(validSymbols) {
		for isSpace(lexer.Lookahead) {
			skip(lexer)
		}

		if lexer.Lookahead != '{' {
			return false
		}

		advance(lexer)
		lexer.MarkEnd()

		for lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
			advance(lexer)
		}

		if lexer.Lookahead != '.' {
			return false
		}
		advance(lexer)

		if lexer.Lookahead != '.' {
			return false
		}
		advance(lexer)

		for lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
			advance(lexer)
		}

		if lexer.Lookahead != '}' {
			return false
		}

		lexer.ResultSymbol = BraceStart
		return true
	}

	return false
}
