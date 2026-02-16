// Package perl provides the external scanner for tree-sitter-perl.
//
// Ported from tree-sitter-perl/src/scanner.c (tree-sitter-perl org).
// The Perl external scanner handles 34 context-sensitive token types:
//   - Quote-like operators (q, qq, s, tr, etc.) with paired/unpaired delimiters
//   - String content scanning (q-strings, qq-strings with interpolation)
//   - Heredocs (interpolating, non-interpolating, indented)
//   - Escape sequences and escaped delimiters
//   - POD documentation blocks
//   - Prototypes vs signatures disambiguation
//   - Fat comma autoquoting, brace autoquoting
//   - File test operators (-f, -d, etc.)
//   - Various zero-width lookahead tokens
//
// NOTE: In the C implementation, EOF is indicated by lookahead == 0.
// In our Go Lexer, EOF is indicated by Lookahead == -1 (lexer.EOF() returns true).
package perl

import (
	"strings"
	"unicode"

	ts "github.com/treesitter-go/treesitter"
)

// Token types matching the externals array in grammar.js.
const (
	TokenApostrophe = iota
	TokenDoubleQuote
	TokenBacktick
	TokenSearchSlash
	NoTokenSearchSlashPlz
	TokenOpenReadlineBracket
	TokenOpenFileGlobBracket
	PerlySemicolon
	PerlyHeredoc
	TokenCtrlZ
	// immediates
	TokenQuotelikeBegin
	TokenQuotelikeMiddleClose
	TokenQuotelikeMiddleSkip
	TokenQuotelikeEndZW
	TokenQuotelikeEnd
	TokenQStringContent
	TokenQQStringContent
	TokenEscapeSequence
	TokenEscapedDelimiter
	TokenDollarInRegexp
	TokenPod
	TokenGobbledContent
	TokenAttributeValueBegin
	TokenAttributeValue
	TokenPrototype
	TokenSignatureStart
	TokenHeredocDelim
	TokenCommandHeredocDelim
	TokenHeredocStart
	TokenHeredocMiddle
	TokenHeredocEnd
	TokenFatCommaAutoquoted
	TokenFiletest
	TokenBraceAutoquoted
	// zero-width
	TokenBraceEndZW
	TokenDollarIdentZW
	TokenNoInterpWhitespaceZW
	// zero-width high priority
	TokenNonassoc
	// error
	TokenError
)

// maxTSPStringLen is the max chars we track for heredoc delimiter comparison.
const maxTSPStringLen = 8

// tspString is a fixed-length comparison string (first 8 codepoints + length).
type tspString struct {
	length   int
	contents [maxTSPStringLen]int32
}

func (s *tspString) push(c int32) {
	if s.length < maxTSPStringLen {
		s.contents[s.length] = c
	}
	s.length++
}

func (s *tspString) eq(other *tspString) bool {
	if s.length != other.length {
		return false
	}
	maxLen := s.length
	if maxLen > maxTSPStringLen {
		maxLen = maxTSPStringLen
	}
	for i := 0; i < maxLen; i++ {
		if s.contents[i] != other.contents[i] {
			return false
		}
	}
	return true
}

func (s *tspString) reset() { s.length = 0 }

// closeForOpen returns the matching close delimiter for paired openers.
func closeForOpen(c int32) int32 {
	switch c {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	default:
		return 0
	}
}

// tspQuote tracks a quoting delimiter with nesting depth for paired delimiters.
type tspQuote struct {
	open, close int32
	count       int32
}

// Heredoc states.
const (
	heredocNone     = iota
	heredocStart    // Just saw the heredoc declaration, haven't started body yet
	heredocUnknown  // Inside heredoc body, scanning from start of line
	heredocContinue // Inside heredoc body, continuing mid-line (for interpolation)
	heredocEnd      // Found the delimiter line, about to emit end token
)

// Scanner implements ts.ExternalScanner for Perl.
type Scanner struct {
	quotes             []tspQuote
	heredocInterpolates bool
	heredocIndents      bool
	heredocState        int
	heredocDelim        tspString
}

// New creates a new Perl external scanner.
func New() ts.ExternalScanner {
	return &Scanner{}
}

// Serialize writes the scanner state to buf.
func (s *Scanner) Serialize(buf []byte) uint32 {
	size := uint32(0)

	// Serialize quote count
	quoteCount := len(s.quotes)
	if quoteCount > 255 {
		quoteCount = 255
	}
	if int(size) >= len(buf) {
		return size
	}
	buf[size] = byte(quoteCount)
	size++

	// Serialize each quote (open: 4 bytes, close: 4 bytes, count: 4 bytes = 12 bytes)
	for i := 0; i < quoteCount; i++ {
		if int(size)+12 > len(buf) {
			break
		}
		q := &s.quotes[i]
		buf[size] = byte(q.open)
		buf[size+1] = byte(q.open >> 8)
		buf[size+2] = byte(q.open >> 16)
		buf[size+3] = byte(q.open >> 24)
		size += 4
		buf[size] = byte(q.close)
		buf[size+1] = byte(q.close >> 8)
		buf[size+2] = byte(q.close >> 16)
		buf[size+3] = byte(q.close >> 24)
		size += 4
		buf[size] = byte(q.count)
		buf[size+1] = byte(q.count >> 8)
		buf[size+2] = byte(q.count >> 16)
		buf[size+3] = byte(q.count >> 24)
		size += 4
	}

	// Serialize heredoc state
	if int(size)+3 > len(buf) {
		return size
	}
	buf[size] = boolByte(s.heredocInterpolates)
	size++
	buf[size] = boolByte(s.heredocIndents)
	size++
	buf[size] = byte(s.heredocState)
	size++

	// Serialize heredoc delimiter (length as int32 + contents)
	if int(size)+4 > len(buf) {
		return size
	}
	buf[size] = byte(s.heredocDelim.length)
	buf[size+1] = byte(s.heredocDelim.length >> 8)
	buf[size+2] = byte(s.heredocDelim.length >> 16)
	buf[size+3] = byte(s.heredocDelim.length >> 24)
	size += 4

	maxLen := s.heredocDelim.length
	if maxLen > maxTSPStringLen {
		maxLen = maxTSPStringLen
	}
	for i := 0; i < maxLen; i++ {
		if int(size)+4 > len(buf) {
			break
		}
		v := s.heredocDelim.contents[i]
		buf[size] = byte(v)
		buf[size+1] = byte(v >> 8)
		buf[size+2] = byte(v >> 16)
		buf[size+3] = byte(v >> 24)
		size += 4
	}

	return size
}

// Deserialize restores the scanner state from data.
func (s *Scanner) Deserialize(data []byte) {
	s.quotes = nil
	s.heredocInterpolates = false
	s.heredocIndents = false
	s.heredocState = heredocNone
	s.heredocDelim.reset()

	if len(data) == 0 {
		return
	}

	off := 0

	// Deserialize quotes
	quoteCount := int(data[off])
	off++

	if quoteCount > 0 {
		s.quotes = make([]tspQuote, quoteCount)
		for i := 0; i < quoteCount && off+12 <= len(data); i++ {
			s.quotes[i].open = int32(data[off]) | int32(data[off+1])<<8 | int32(data[off+2])<<16 | int32(data[off+3])<<24
			off += 4
			s.quotes[i].close = int32(data[off]) | int32(data[off+1])<<8 | int32(data[off+2])<<16 | int32(data[off+3])<<24
			off += 4
			s.quotes[i].count = int32(data[off]) | int32(data[off+1])<<8 | int32(data[off+2])<<16 | int32(data[off+3])<<24
			off += 4
		}
	}

	// Deserialize heredoc state
	if off+3 > len(data) {
		return
	}
	s.heredocInterpolates = data[off] != 0
	off++
	s.heredocIndents = data[off] != 0
	off++
	s.heredocState = int(data[off])
	off++

	// Deserialize heredoc delimiter
	if off+4 > len(data) {
		return
	}
	s.heredocDelim.length = int(data[off]) | int(data[off+1])<<8 | int(data[off+2])<<16 | int(data[off+3])<<24
	off += 4

	maxLen := s.heredocDelim.length
	if maxLen > maxTSPStringLen {
		maxLen = maxTSPStringLen
	}
	for i := 0; i < maxLen && off+4 <= len(data); i++ {
		s.heredocDelim.contents[i] = int32(data[off]) | int32(data[off+1])<<8 | int32(data[off+2])<<16 | int32(data[off+3])<<24
		off += 4
	}
}

// pushQuote adds a new quoting context.
func (s *Scanner) pushQuote(opener int32) {
	q := tspQuote{}
	closer := closeForOpen(opener)
	if closer != 0 {
		q.open = opener
		q.close = closer
	} else {
		q.open = 0
		q.close = opener
	}
	s.quotes = append(s.quotes, q)
}

// isQuoteOpener checks if c matches any quote opener, searching from end.
// Returns index+1 (so 0 = not found).
func (s *Scanner) isQuoteOpener(c int32) int {
	for i := len(s.quotes) - 1; i >= 0; i-- {
		if s.quotes[i].open != 0 && c == s.quotes[i].open {
			return i + 1
		}
	}
	return 0
}

// sawOpener increments the nesting count for a matched opener.
func (s *Scanner) sawOpener(idx int) {
	s.quotes[idx-1].count++
}

// isQuoteCloser checks if c matches any quote closer, searching from end.
// Returns index+1 (so 0 = not found).
func (s *Scanner) isQuoteCloser(c int32) int {
	for i := len(s.quotes) - 1; i >= 0; i-- {
		if s.quotes[i].close != 0 && c == s.quotes[i].close {
			return i + 1
		}
	}
	return 0
}

// sawCloser decrements the nesting count for a matched closer.
func (s *Scanner) sawCloser(idx int) {
	if s.quotes[idx-1].count > 0 {
		s.quotes[idx-1].count--
	}
}

// isQuoteClosed returns true if the quote at idx has count == 0.
func (s *Scanner) isQuoteClosed(idx int) bool {
	return s.quotes[idx-1].count == 0
}

// popQuote removes the quote at idx.
func (s *Scanner) popQuote(idx int) {
	s.quotes = append(s.quotes[:idx-1], s.quotes[idx:]...)
}

// isPairedDelimiter returns true if the current (top) quote has a distinct opener.
func (s *Scanner) isPairedDelimiter() bool {
	if len(s.quotes) == 0 {
		return false
	}
	return s.quotes[len(s.quotes)-1].open != 0
}

// addHeredoc records a pending heredoc.
func (s *Scanner) addHeredoc(delim *tspString, interp, indent bool) {
	s.heredocDelim = *delim
	s.heredocInterpolates = interp
	s.heredocIndents = indent
	s.heredocState = heredocStart
}

// finishHeredoc clears the heredoc state.
func (s *Scanner) finishHeredoc() {
	s.heredocDelim.length = 0
	s.heredocState = heredocNone
}

// Scan dispatches to the appropriate scanning logic based on validSymbols.
func (s *Scanner) Scan(lexer *ts.Lexer, validSymbols []bool) bool {
	isError := validSymbols[TokenError]
	skippedWhitespace := false

	c := lexer.Lookahead

	// TOKEN_GOBBLED_CONTENT: consume everything until EOF
	if !isError && validSymbols[TokenGobbledContent] {
		for !lexer.EOF() {
			advance(lexer)
		}
		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TokenGobbledContent)
		return true
	}

	// TOKEN_NONASSOC: force tree-sitter to stay on error branch (zero-width)
	if !isError && validSymbols[TokenNonassoc] {
		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TokenNonassoc)
		return true
	}

	// HEREDOC_MIDDLE: whitespace-sensitive, must go before any skipping
	if validSymbols[TokenHeredocMiddle] && !isError {
		if s.heredocState != heredocContinue {
			var line tspString
			// Read as many lines as we can
			for !lexer.EOF() {
				line.reset()
				isValidStartPos := s.heredocState == heredocEnd || lexer.CurrentPosition().Point.Column == 0
				sawEscape := false

				if isValidStartPos && s.heredocIndents {
					skipWhitespace(lexer)
					c = lexer.Lookahead
				}
				// May be doing lookahead now
				lexer.MarkEnd()
				// Read whole line
				for c != '\n' && !lexer.EOF() {
					if c == '\r' {
						advance(lexer)
						c = lexer.Lookahead
						if c == '\n' {
							break
						}
						line.push('\r')
					}
					line.push(c)
					if c == '$' || c == '@' || c == '\\' {
						sawEscape = true
					}
					advance(lexer)
					c = lexer.Lookahead
				}
				if isValidStartPos && line.eq(&s.heredocDelim) {
					if s.heredocState != heredocEnd {
						s.heredocState = heredocEnd
						lexer.ResultSymbol = ts.Symbol(TokenHeredocMiddle)
						return true
					}
					lexer.MarkEnd()
					s.finishHeredoc()
					lexer.ResultSymbol = ts.Symbol(TokenHeredocEnd)
					return true
				}
				if sawEscape && s.heredocInterpolates {
					s.heredocState = heredocContinue
					lexer.ResultSymbol = ts.Symbol(TokenHeredocMiddle)
					return true
				}
				// Eat the \n and loop again
				advance(lexer)
				c = lexer.Lookahead
			}
		} else {
			// Continue case: read ahead until \n or interpolation escape
			sawChars := false
			for {
				if lexer.EOF() {
					// EOF mid-heredoc-continue: emit what we have or bail.
					if sawChars {
						lexer.MarkEnd()
						lexer.ResultSymbol = ts.Symbol(TokenHeredocMiddle)
						return true
					}
					return false
				}
				if isInterpolationEscape(c) {
					lexer.MarkEnd()
					break
				}
				if c == '\n' {
					lexer.MarkEnd()
					s.heredocState = heredocUnknown
					lexer.ResultSymbol = ts.Symbol(TokenHeredocMiddle)
					return true
				}
				sawChars = true
				advance(lexer)
				c = lexer.Lookahead
			}
			if sawChars {
				lexer.ResultSymbol = ts.Symbol(TokenHeredocMiddle)
				return true
			}
		}
	}

	// TOKEN_NO_INTERP_WHITESPACE_ZW (zero-width)
	if isTSPWhitespace(c) && validSymbols[TokenNoInterpWhitespaceZW] {
		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TokenNoInterpWhitespaceZW)
		return true
	}

	// Skip whitespace to end of line
	skipWsToEOL(lexer)

	// TOKEN_HEREDOC_START (zero-width — marks start of heredoc body)
	if validSymbols[TokenHeredocStart] {
		if s.heredocState == heredocStart && lexer.CurrentPosition().Point.Column == 0 {
			s.heredocState = heredocUnknown
			lexer.MarkEnd()
			lexer.ResultSymbol = ts.Symbol(TokenHeredocStart)
			return true
		}
	}

	// NOTE: Do NOT update c here. The C code keeps c stale (original value from
	// function entry) until the main whitespace-skipping block below. This is
	// important because the skipped_whitespace flag depends on c still being the
	// original whitespace character. Attribute value checks read lexer.Lookahead
	// directly instead.

	// TOKEN_ATTRIBUTE_VALUE_BEGIN (zero-width — lookahead found '(')
	if !isError && validSymbols[TokenAttributeValueBegin] && lexer.Lookahead == '(' {
		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TokenAttributeValueBegin)
		return true
	}

	// TOKEN_ATTRIBUTE_VALUE
	if !isError && validSymbols[TokenAttributeValue] {
		c = lexer.Lookahead // refresh c for this self-contained block
		delimCount := 0
		for !lexer.EOF() {
			if c == '\\' {
				advance(lexer)
				c = lexer.Lookahead
				// Ignore next char
			} else if c == '(' {
				delimCount++
			} else if c == ')' {
				if delimCount > 0 {
					delimCount--
				} else {
					break
				}
			}
			advance(lexer)
			c = lexer.Lookahead
		}
		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TokenAttributeValue)
		return true
	}

	// Skip remaining whitespace
	if isTSPWhitespace(c) {
		skippedWhitespace = true
		skipWhitespace(lexer)
		c = lexer.Lookahead
	}

	// CTRL-Z (zero-width — lexer hasn't consumed the character)
	if c == 26 && validSymbols[TokenCtrlZ] {
		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TokenCtrlZ)
		return true
	}

	// PERLY_SEMICOLON (zero-width — implicit semicolon before } or at EOF)
	if validSymbols[PerlySemicolon] {
		if c == '}' || lexer.EOF() {
			if isError || !validSymbols[TokenBraceEndZW] {
				lexer.MarkEnd()
				lexer.ResultSymbol = ts.Symbol(PerlySemicolon)
				return true
			}
		}
	}
	if lexer.EOF() {
		return false
	}

	// READLINE / FILEGLOB / HEREDOC (< handling)
	if validSymbols[TokenOpenFileGlobBracket] || validSymbols[TokenOpenReadlineBracket] || validSymbols[PerlyHeredoc] {
		if c == '<' {
			advance(lexer)
			c = lexer.Lookahead
			lexer.MarkEnd()

			if c == '<' {
				// This is a heredoc <<; jump to heredoc handling
				return s.scanHeredocToken(lexer)
			}
			if c == '$' {
				advance(lexer)
				c = lexer.Lookahead
			}
			// Zoom through ident chars
			for isIDCont(c) {
				advance(lexer)
				c = lexer.Lookahead
			}
			if c == '>' {
				lexer.ResultSymbol = ts.Symbol(TokenOpenReadlineBracket)
				return true
			}
			// It's a fileglob operator
			s.pushQuote('<')
			lexer.ResultSymbol = ts.Symbol(TokenOpenFileGlobBracket)
			return true
		}
	}

	// TOKEN_DOLLAR_IDENT_ZW
	if validSymbols[TokenDollarIdentZW] {
		if !isIDCont(c) && c != '$' && c != '{' {
			if c == ':' {
				lexer.MarkEnd()
				advance(lexer)
				c = lexer.Lookahead
				if c == ':' {
					return false
				}
			}
			lexer.ResultSymbol = ts.Symbol(TokenDollarIdentZW)
			return true
		}
	}

	// TOKEN_SEARCH_SLASH
	if validSymbols[TokenSearchSlash] && c == '/' && !validSymbols[NoTokenSearchSlashPlz] {
		advance(lexer)
		c = lexer.Lookahead
		lexer.MarkEnd()
		if c != '/' {
			s.pushQuote('/')
			lexer.ResultSymbol = ts.Symbol(TokenSearchSlash)
			return true
		}
		return false
	}

	// TOKEN_APOSTROPHE
	if validSymbols[TokenApostrophe] && c == '\'' {
		advance(lexer)
		s.pushQuote('\'')
		lexer.ResultSymbol = ts.Symbol(TokenApostrophe)
		return true
	}

	// TOKEN_DOUBLE_QUOTE
	if validSymbols[TokenDoubleQuote] && c == '"' {
		advance(lexer)
		s.pushQuote('"')
		lexer.ResultSymbol = ts.Symbol(TokenDoubleQuote)
		return true
	}

	// TOKEN_BACKTICK
	if validSymbols[TokenBacktick] && c == '`' {
		advance(lexer)
		s.pushQuote('`')
		lexer.ResultSymbol = ts.Symbol(TokenBacktick)
		return true
	}

	// TOKEN_DOLLAR_IN_REGEXP
	if validSymbols[TokenDollarInRegexp] && c == '$' {
		advance(lexer)
		c = lexer.Lookahead
		if s.isQuoteCloser(c) != 0 {
			lexer.ResultSymbol = ts.Symbol(TokenDollarInRegexp)
			return true
		}
		switch c {
		case '(', ')', '|':
			lexer.ResultSymbol = ts.Symbol(TokenDollarInRegexp)
			return true
		}
		return false
	}

	// TOKEN_POD
	if validSymbols[TokenPod] {
		column := lexer.CurrentPosition().Point.Column
		if column == 0 && c == '=' {
			cutMarker := "=cut"
			stage := -1
			for !lexer.EOF() {
				if c == '\r' {
					// ignore
				} else if stage < 1 && c == '\n' {
					stage = 0
				} else if stage >= 0 && stage < 4 && c == int32(cutMarker[stage]) {
					stage++
				} else if stage == 4 && (c == ' ' || c == '\t') {
					stage = 5
				} else if stage == 4 && c == '\n' {
					stage = 6
				} else {
					stage = -1
				}
				if stage > 4 {
					break
				}
				advance(lexer)
				c = lexer.Lookahead
			}
			if stage < 6 {
				for !lexer.EOF() {
					if c == '\n' {
						break
					}
					advance(lexer)
					c = lexer.Lookahead
				}
			}
			lexer.MarkEnd()
			lexer.ResultSymbol = ts.Symbol(TokenPod)
			return true
		}
	}

	// Past this point, bail on error recovery
	if isError {
		return false
	}

	// TOKEN_HEREDOC_DELIM / TOKEN_COMMAND_HEREDOC_DELIM
	if validSymbols[TokenHeredocDelim] || validSymbols[TokenCommandHeredocDelim] {
		shouldIndent := false
		shouldInterpolate := true
		var delim tspString
		delim.reset()

		if !skippedWhitespace {
			if c == '~' {
				advance(lexer)
				c = lexer.Lookahead
				shouldIndent = true
			}
			if c == '\\' {
				advance(lexer)
				c = lexer.Lookahead
				shouldInterpolate = false
			}
			if isIDFirst(c) {
				for isIDCont(c) {
					delim.push(c)
					advance(lexer)
					c = lexer.Lookahead
				}
				s.addHeredoc(&delim, shouldInterpolate, shouldIndent)
				lexer.ResultSymbol = ts.Symbol(TokenHeredocDelim)
				return true
			}
		}
		// If we picked up a ~ before, skip whitespace
		if shouldIndent {
			skipWhitespace(lexer)
			c = lexer.Lookahead
		}
		// Quoted heredoc delimiter
		if shouldInterpolate && (c == '\'' || c == '"' || c == '`') {
			delimOpen := c
			if c == '\'' {
				shouldInterpolate = false
			}
			advance(lexer)
			c = lexer.Lookahead
			for c != delimOpen && !lexer.EOF() {
				if c == '\\' {
					toAdd := c
					advance(lexer)
					c = lexer.Lookahead
					if c == delimOpen {
						toAdd = delimOpen
						advance(lexer)
						c = lexer.Lookahead
					}
					delim.push(toAdd)
				} else {
					delim.push(c)
					advance(lexer)
					c = lexer.Lookahead
				}
			}
			if delim.length > 0 {
				// Consume closing delimiter
				advance(lexer)
				c = lexer.Lookahead
				s.addHeredoc(&delim, shouldInterpolate, shouldIndent)
				if delimOpen == '`' {
					lexer.ResultSymbol = ts.Symbol(TokenCommandHeredocDelim)
					return true
				}
				lexer.ResultSymbol = ts.Symbol(TokenHeredocDelim)
				return true
			}
		}
	}

	// TOKEN_QUOTELIKE_MIDDLE_SKIP
	if validSymbols[TokenQuotelikeMiddleSkip] {
		if !s.isPairedDelimiter() {
			lexer.ResultSymbol = ts.Symbol(TokenQuotelikeMiddleSkip)
			return true
		}
	}

	// TOKEN_QUOTELIKE_BEGIN
	if validSymbols[TokenQuotelikeBegin] {
		delim := c
		if skippedWhitespace && c == '#' {
			return false
		}
		lexer.MarkEnd()
		advance(lexer)
		c = lexer.Lookahead

		// Handle $hash{q} case
		if validSymbols[TokenBraceEndZW] && delim == '}' {
			lexer.ResultSymbol = ts.Symbol(TokenBraceEndZW)
			return true
		}
		lexer.MarkEnd()

		s.pushQuote(delim)
		lexer.ResultSymbol = ts.Symbol(TokenQuotelikeBegin)
		return true
	}

	// Backslash handling (escape sequences / escaped delimiters)
	if c == '\\' &&
		!(validSymbols[TokenQuotelikeEnd] && s.isQuoteCloser('\\') != 0) {
		advance(lexer)
		c = lexer.Lookahead
		escC := c
		// If we escaped a whitespace, the space comes through
		if !isTSPWhitespace(c) {
			advance(lexer)
			c = lexer.Lookahead
		}

		if validSymbols[TokenEscapedDelimiter] {
			if s.isQuoteOpener(escC) != 0 || s.isQuoteCloser(escC) != 0 {
				lexer.MarkEnd()
				lexer.ResultSymbol = ts.Symbol(TokenEscapedDelimiter)
				return true
			}
		}

		if validSymbols[TokenEscapeSequence] {
			lexer.MarkEnd()
			if escC == '\\' {
				lexer.ResultSymbol = ts.Symbol(TokenEscapeSequence)
				return true
			}

			if validSymbols[TokenQStringContent] {
				// Inside q() string, only \\ is valid escape; all else is literal
				lexer.ResultSymbol = ts.Symbol(TokenQStringContent)
				return true
			}

			switch escC {
			case 'x':
				if c == '{' {
					skipBraced(lexer)
				} else {
					skipHexDigits(lexer, 2)
				}
			case 'N':
				skipBraced(lexer)
			case 'o':
				skipBraced(lexer)
			case '0':
				skipOctDigits(lexer, 3)
			}

			lexer.ResultSymbol = ts.Symbol(TokenEscapeSequence)
			return true
		}
	}

	// TOKEN_Q_STRING_CONTENT / TOKEN_QQ_STRING_CONTENT
	if validSymbols[TokenQStringContent] || validSymbols[TokenQQStringContent] {
		isQQ := validSymbols[TokenQQStringContent]
		valid := false

		for !lexer.EOF() && c != 0 {
			if c == '\\' {
				break
			}
			quoteIndex := s.isQuoteOpener(c)
			if quoteIndex != 0 {
				s.sawOpener(quoteIndex)
			} else {
				quoteIndex = s.isQuoteCloser(c)
				if quoteIndex != 0 {
					if s.isQuoteClosed(quoteIndex) {
						break
					}
					s.sawCloser(quoteIndex)
				} else if isQQ && isInterpolationEscape(c) {
					break
				}
			}
			valid = true
			advance(lexer)
			c = lexer.Lookahead
		}

		if valid {
			lexer.MarkEnd()
			if isQQ {
				lexer.ResultSymbol = ts.Symbol(TokenQQStringContent)
			} else {
				lexer.ResultSymbol = ts.Symbol(TokenQStringContent)
			}
			return true
		}
	}

	// TOKEN_QUOTELIKE_MIDDLE_CLOSE
	if validSymbols[TokenQuotelikeMiddleClose] {
		quoteIndex := s.isQuoteCloser(c)
		if quoteIndex != 0 && s.isQuoteClosed(quoteIndex) {
			advance(lexer)
			lexer.ResultSymbol = ts.Symbol(TokenQuotelikeMiddleClose)
			return true
		}
	}

	// TOKEN_QUOTELIKE_END
	if validSymbols[TokenQuotelikeEnd] {
		quoteIndex := s.isQuoteCloser(c)
		if quoteIndex != 0 {
			if validSymbols[TokenQuotelikeEndZW] {
				lexer.ResultSymbol = ts.Symbol(TokenQuotelikeEndZW)
				return true
			}
			advance(lexer)
			s.popQuote(quoteIndex)
			lexer.ResultSymbol = ts.Symbol(TokenQuotelikeEnd)
			return true
		}
	}

	// TOKEN_PROTOTYPE / TOKEN_SIGNATURE_START
	if c == '(' && (validSymbols[TokenPrototype] || validSymbols[TokenSignatureStart]) {
		advance(lexer)
		c = lexer.Lookahead
		lexer.MarkEnd()

		count := 0
		for !lexer.EOF() {
			if c == ')' && count == 0 {
				advance(lexer)
				c = lexer.Lookahead
				break
			} else if c == ')' {
				count--
			} else if c == '(' {
				count++
			} else if isIDContinue(c) {
				lexer.ResultSymbol = ts.Symbol(TokenSignatureStart)
				return true
			}
			advance(lexer)
			c = lexer.Lookahead
		}

		lexer.MarkEnd()
		lexer.ResultSymbol = ts.Symbol(TokenPrototype)
		return true
	}

	// TOKEN_FILETEST / FAT_COMMA_AUTOQUOTED / BRACE_AUTOQUOTED / HEREDOC
	c1 := c
	if c == '-' && validSymbols[TokenFiletest] {
		advance(lexer)
		c = lexer.Lookahead
		if strings.ContainsRune("rwxoRWXOezsfdlpSbctugkTBMAC", c) {
			advance(lexer)
			c = lexer.Lookahead
			if !isIDCont(c) {
				lexer.ResultSymbol = ts.Symbol(TokenFiletest)
				return true
			}
		}
		return false
	}

	if isIDFirst(c) && (validSymbols[TokenFatCommaAutoquoted] || validSymbols[TokenBraceAutoquoted]) {
		// Zip through identifier
		for {
			advance(lexer)
			c = lexer.Lookahead
			if lexer.EOF() || !isIDCont(c) {
				break
			}
		}
		lexer.MarkEnd()

		// Skip whitespace and comments after the identifier
		for isTSPWhitespace(c) || c == '#' {
			for isTSPWhitespace(c) {
				advance(lexer)
				c = lexer.Lookahead
			}
			if c == '#' {
				advance(lexer)
				c = lexer.Lookahead
				for lexer.CurrentPosition().Point.Column != 0 {
					advance(lexer)
					c = lexer.Lookahead
				}
			}
			if lexer.EOF() {
				return false
			}
		}
		c1 = lexer.Lookahead
		advance(lexer)
		c = lexer.Lookahead
		if validSymbols[TokenFatCommaAutoquoted] {
			if c1 == '=' && c == '>' {
				lexer.ResultSymbol = ts.Symbol(TokenFatCommaAutoquoted)
				return true
			}
		}
		if validSymbols[TokenBraceAutoquoted] {
			if c1 == '}' {
				lexer.ResultSymbol = ts.Symbol(TokenBraceAutoquoted)
				return true
			}
		}
	} else {
		// ZW lookahead
		lexer.MarkEnd()
		advance(lexer)
		c2 := lexer.Lookahead
		if lexer.EOF() {
			return false
		}

		// Check for <<
		if c1 == '<' && c2 == '<' {
			return s.scanHeredocToken(lexer)
		}

		if validSymbols[TokenBraceEndZW] {
			if c1 == '}' {
				lexer.ResultSymbol = ts.Symbol(TokenBraceEndZW)
				return true
			}
		}
	}

	return false
}

// scanHeredocToken handles the << heredoc detection.
func (s *Scanner) scanHeredocToken(lexer *ts.Lexer) bool {
	advance(lexer)
	c := lexer.Lookahead
	lexer.MarkEnd()
	if c == '\\' || c == '~' || isIDFirst(c) {
		lexer.ResultSymbol = ts.Symbol(PerlyHeredoc)
		return true
	}
	skipWhitespace(lexer)
	c = lexer.Lookahead
	if c == '\'' || c == '"' || c == '`' {
		lexer.ResultSymbol = ts.Symbol(PerlyHeredoc)
		return true
	}
	return false
}

// Helper functions

func advance(lexer *ts.Lexer) {
	lexer.Advance(false)
}

func skip(lexer *ts.Lexer) {
	lexer.Advance(true)
}

func boolByte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

// isTSPWhitespace checks if c is whitespace per Perl's Unicode whitespace ranges.
func isTSPWhitespace(c int32) bool {
	if c < 0 {
		return false
	}
	return unicode.IsSpace(rune(c))
}

// isIDFirst checks if c can start a Perl identifier.
func isIDFirst(c int32) bool {
	if c < 0 {
		return false
	}
	return c == '_' || unicode.Is(unicode.L, rune(c)) || unicode.Is(unicode.Nl, rune(c))
}

// isIDCont checks if c can continue a Perl identifier.
func isIDCont(c int32) bool {
	if c < 0 {
		return false
	}
	return c == '_' || unicode.Is(unicode.L, rune(c)) || unicode.Is(unicode.N, rune(c)) ||
		unicode.Is(unicode.Mn, rune(c)) || unicode.Is(unicode.Mc, rune(c))
}

// isIDContinue is an alias for prototype/signature detection (uses unicode.Is_ID_Continue).
func isIDContinue(c int32) bool {
	if c < 0 {
		return false
	}
	// is_tsp_id_continue from tsp_unicode.h matches Unicode ID_Continue
	return c == '_' || unicode.Is(unicode.L, rune(c)) || unicode.Is(unicode.N, rune(c)) ||
		unicode.Is(unicode.Mn, rune(c)) || unicode.Is(unicode.Mc, rune(c))
}

// isInterpolationEscape checks if c triggers interpolation in qq strings.
func isInterpolationEscape(c int32) bool {
	if c < 0 || c >= 256 {
		return false
	}
	return strings.ContainsRune("$@-[{\\", c)
}

// skipWhitespace skips all whitespace characters.
func skipWhitespace(lexer *ts.Lexer) {
	for !lexer.EOF() {
		c := lexer.Lookahead
		if isTSPWhitespace(c) {
			skip(lexer)
		} else {
			return
		}
	}
}

// skipWsToEOL skips whitespace up to and including the first newline.
func skipWsToEOL(lexer *ts.Lexer) {
	for !lexer.EOF() {
		c := lexer.Lookahead
		if isTSPWhitespace(c) {
			skip(lexer)
			if c == '\n' {
				return
			}
		} else {
			return
		}
	}
}

// skipBraced advances past a {…} sequence.
func skipBraced(lexer *ts.Lexer) {
	c := lexer.Lookahead
	if c != '{' {
		return
	}
	advance(lexer)
	c = lexer.Lookahead
	for !lexer.EOF() && c != '}' {
		advance(lexer)
		c = lexer.Lookahead
	}
	advance(lexer)
}

// skipHexDigits advances past up to maxLen hex digits.
func skipHexDigits(lexer *ts.Lexer, maxLen int) {
	skipChars(lexer, maxLen, "0123456789ABCDEFabcdef")
}

// skipOctDigits advances past up to maxLen octal digits.
func skipOctDigits(lexer *ts.Lexer, maxLen int) {
	skipChars(lexer, maxLen, "01234567")
}

// skipChars advances past up to maxLen characters that appear in allow.
func skipChars(lexer *ts.Lexer, maxLen int, allow string) {
	c := lexer.Lookahead
	for maxLen != 0 {
		if lexer.EOF() {
			return
		}
		if strings.ContainsRune(allow, c) {
			advance(lexer)
			c = lexer.Lookahead
			if maxLen > 0 {
				maxLen--
			}
		} else {
			break
		}
	}
}
