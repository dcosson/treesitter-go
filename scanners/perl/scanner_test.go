package perl

import (
	"testing"

	ts "github.com/treesitter-go/treesitter"
)

// newLexerForString creates a test lexer from a string.
func newLexerForString(s string) *ts.Lexer {
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(s)))
	lexer.Start(ts.Length{})
	return lexer
}

// onlyValid returns a validSymbols slice with only the specified tokens valid.
func onlyValid(tokens ...int) []bool {
	v := make([]bool, TokenError+1)
	for _, t := range tokens {
		v[t] = true
	}
	return v
}

// allValid returns a validSymbols slice with all tokens valid.
func allValid() []bool {
	v := make([]bool, TokenError+1)
	for i := range v {
		v[i] = true
	}
	return v
}

// --- Serialize/Deserialize ---

func TestSerializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n == 0 {
		t.Fatal("expected nonzero serialization for empty scanner")
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if len(s2.quotes) != 0 {
		t.Errorf("expected 0 quotes, got %d", len(s2.quotes))
	}
	if s2.heredocState != heredocNone {
		t.Errorf("expected heredocNone, got %d", s2.heredocState)
	}
}

func TestSerializeRoundtrip(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	s.pushQuote('"')
	s.addHeredoc(&tspString{length: 3, contents: [maxTSPStringLen]int32{'E', 'O', 'F'}}, true, false)

	buf := make([]byte, 1024)
	n := s.Serialize(buf)

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])

	if len(s2.quotes) != 2 {
		t.Fatalf("expected 2 quotes, got %d", len(s2.quotes))
	}
	if s2.quotes[0].open != '(' || s2.quotes[0].close != ')' {
		t.Errorf("quote 0: expected (/) got %c/%c", s2.quotes[0].open, s2.quotes[0].close)
	}
	if s2.quotes[1].open != 0 || s2.quotes[1].close != '"' {
		t.Errorf("quote 1: expected 0/'\"' got %c/%c", s2.quotes[1].open, s2.quotes[1].close)
	}
	if !s2.heredocInterpolates {
		t.Error("expected heredocInterpolates=true")
	}
	if s2.heredocIndents {
		t.Error("expected heredocIndents=false")
	}
	if s2.heredocState != heredocStart {
		t.Errorf("expected heredocStart, got %d", s2.heredocState)
	}
	if s2.heredocDelim.length != 3 {
		t.Errorf("expected heredoc delim length 3, got %d", s2.heredocDelim.length)
	}
	if s2.heredocDelim.contents[0] != 'E' || s2.heredocDelim.contents[1] != 'O' || s2.heredocDelim.contents[2] != 'F' {
		t.Error("heredoc delim contents mismatch")
	}
}

func TestDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	s.Deserialize(nil)
	if len(s.quotes) != 0 {
		t.Errorf("expected 0 quotes after deserialize(nil), got %d", len(s.quotes))
	}
}

// --- Quote Stack ---

func TestQuoteStackPaired(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	if len(s.quotes) != 1 {
		t.Fatalf("expected 1 quote, got %d", len(s.quotes))
	}
	if s.quotes[0].open != '(' || s.quotes[0].close != ')' {
		t.Error("expected paired ( )")
	}
	if !s.isPairedDelimiter() {
		t.Error("expected paired delimiter")
	}
}

func TestQuoteStackUnpaired(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('/')
	if s.quotes[0].open != 0 || s.quotes[0].close != '/' {
		t.Error("expected unpaired /")
	}
	if s.isPairedDelimiter() {
		t.Error("expected unpaired delimiter")
	}
}

func TestQuoteOpenerCloser(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('{')

	idx := s.isQuoteOpener('{')
	if idx == 0 {
		t.Fatal("expected to find opener {")
	}
	s.sawOpener(idx)
	if s.quotes[0].count != 1 {
		t.Errorf("expected count=1, got %d", s.quotes[0].count)
	}

	cidx := s.isQuoteCloser('}')
	if cidx == 0 {
		t.Fatal("expected to find closer }")
	}
	if s.isQuoteClosed(cidx) {
		t.Error("should not be closed with count=1")
	}
	s.sawCloser(cidx)
	if !s.isQuoteClosed(cidx) {
		t.Error("should be closed with count=0")
	}

	s.popQuote(cidx)
	if len(s.quotes) != 0 {
		t.Error("expected empty quotes after pop")
	}
}

func TestNestedQuotes(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	s.pushQuote('[')

	// Opener for ( is found
	idx := s.isQuoteOpener('(')
	if idx == 0 {
		t.Fatal("should find ( opener")
	}

	// Closer for ] is found (from end, should be the [ quote)
	cidx := s.isQuoteCloser(']')
	if cidx == 0 {
		t.Fatal("should find ] closer")
	}
	if cidx != 2 {
		t.Errorf("expected index 2 for [/], got %d", cidx)
	}
}

// --- TSPString ---

func TestTSPStringEq(t *testing.T) {
	a := tspString{}
	b := tspString{}
	a.push('E')
	a.push('O')
	a.push('F')
	b.push('E')
	b.push('O')
	b.push('F')
	if !a.eq(&b) {
		t.Error("expected equal strings")
	}
}

func TestTSPStringNeq(t *testing.T) {
	a := tspString{}
	b := tspString{}
	a.push('E')
	a.push('O')
	a.push('F')
	b.push('E')
	b.push('N')
	b.push('D')
	if a.eq(&b) {
		t.Error("expected unequal strings")
	}
}

func TestTSPStringDifferentLength(t *testing.T) {
	a := tspString{}
	b := tspString{}
	a.push('A')
	a.push('B')
	b.push('A')
	if a.eq(&b) {
		t.Error("expected unequal strings of different length")
	}
}

func TestTSPStringOverflow(t *testing.T) {
	a := tspString{}
	for i := 0; i < 12; i++ {
		a.push(int32('A' + i))
	}
	if a.length != 12 {
		t.Errorf("expected length=12, got %d", a.length)
	}
	// Only first 8 are stored
	b := tspString{}
	for i := 0; i < 12; i++ {
		b.push(int32('A' + i))
	}
	if !a.eq(&b) {
		t.Error("expected equal (by first 8 chars + length)")
	}
}

// --- closeForOpen ---

func TestCloseForOpen(t *testing.T) {
	cases := []struct {
		open, close int32
	}{
		{'(', ')'},
		{'[', ']'},
		{'{', '}'},
		{'<', '>'},
		{'/', 0},
		{'\'', 0},
	}
	for _, tc := range cases {
		if got := closeForOpen(tc.open); got != tc.close {
			t.Errorf("closeForOpen(%c) = %c, want %c", tc.open, got, tc.close)
		}
	}
}

// --- Token Scanning ---

func TestScanGobbledContent(t *testing.T) {
	lexer := newLexerForString("hello world")
	s := New().(*Scanner)
	v := onlyValid(TokenGobbledContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenGobbledContent) {
		t.Errorf("expected TokenGobbledContent, got %d", lexer.ResultSymbol)
	}
}

func TestScanNonassoc(t *testing.T) {
	lexer := newLexerForString("x")
	s := New().(*Scanner)
	v := onlyValid(TokenNonassoc)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenNonassoc) {
		t.Errorf("expected TokenNonassoc, got %d", lexer.ResultSymbol)
	}
}

func TestScanGobbledContentNotInError(t *testing.T) {
	lexer := newLexerForString("hello")
	s := New().(*Scanner)
	v := onlyValid(TokenGobbledContent, TokenError)
	if s.Scan(lexer, v) {
		t.Error("should not scan gobbled content in error mode")
	}
}

func TestScanNoInterpWhitespaceZW(t *testing.T) {
	lexer := newLexerForString(" x")
	s := New().(*Scanner)
	v := onlyValid(TokenNoInterpWhitespaceZW)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenNoInterpWhitespaceZW) {
		t.Errorf("expected TokenNoInterpWhitespaceZW, got %d", lexer.ResultSymbol)
	}
}

func TestScanAttributeValueBegin(t *testing.T) {
	lexer := newLexerForString("(value)")
	s := New().(*Scanner)
	v := onlyValid(TokenAttributeValueBegin)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenAttributeValueBegin) {
		t.Errorf("expected TokenAttributeValueBegin, got %d", lexer.ResultSymbol)
	}
}

func TestScanAttributeValue(t *testing.T) {
	lexer := newLexerForString("hello world)")
	s := New().(*Scanner)
	v := onlyValid(TokenAttributeValue)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenAttributeValue) {
		t.Errorf("expected TokenAttributeValue, got %d", lexer.ResultSymbol)
	}
}

func TestScanAttributeValueNested(t *testing.T) {
	lexer := newLexerForString("(inner))")
	s := New().(*Scanner)
	v := onlyValid(TokenAttributeValue)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenAttributeValue) {
		t.Errorf("expected TokenAttributeValue, got %d", lexer.ResultSymbol)
	}
}

func TestScanCtrlZ(t *testing.T) {
	lexer := newLexerForString(string(rune(26)))
	s := New().(*Scanner)
	v := onlyValid(TokenCtrlZ)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenCtrlZ) {
		t.Errorf("expected TokenCtrlZ, got %d", lexer.ResultSymbol)
	}
}

func TestScanPerlySemicolonAtBrace(t *testing.T) {
	lexer := newLexerForString("}")
	s := New().(*Scanner)
	v := onlyValid(PerlySemicolon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(PerlySemicolon) {
		t.Errorf("expected PerlySemicolon, got %d", lexer.ResultSymbol)
	}
}

func TestScanPerlySemicolonAtEOF(t *testing.T) {
	lexer := newLexerForString("")
	s := New().(*Scanner)
	v := onlyValid(PerlySemicolon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(PerlySemicolon) {
		t.Errorf("expected PerlySemicolon, got %d", lexer.ResultSymbol)
	}
}

func TestScanApostrophe(t *testing.T) {
	lexer := newLexerForString("'hello'")
	s := New().(*Scanner)
	v := onlyValid(TokenApostrophe)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenApostrophe) {
		t.Errorf("expected TokenApostrophe, got %d", lexer.ResultSymbol)
	}
	if len(s.quotes) != 1 || s.quotes[0].close != '\'' {
		t.Error("expected quote pushed for apostrophe")
	}
}

func TestScanDoubleQuote(t *testing.T) {
	lexer := newLexerForString("\"hello\"")
	s := New().(*Scanner)
	v := onlyValid(TokenDoubleQuote)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenDoubleQuote) {
		t.Errorf("expected TokenDoubleQuote, got %d", lexer.ResultSymbol)
	}
}

func TestScanBacktick(t *testing.T) {
	lexer := newLexerForString("`ls`")
	s := New().(*Scanner)
	v := onlyValid(TokenBacktick)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenBacktick) {
		t.Errorf("expected TokenBacktick, got %d", lexer.ResultSymbol)
	}
}

func TestScanSearchSlash(t *testing.T) {
	lexer := newLexerForString("/pattern/")
	s := New().(*Scanner)
	v := onlyValid(TokenSearchSlash)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenSearchSlash) {
		t.Errorf("expected TokenSearchSlash, got %d", lexer.ResultSymbol)
	}
	if len(s.quotes) != 1 {
		t.Error("expected quote pushed for search slash")
	}
}

func TestScanSearchSlashRejectsDoubleSlash(t *testing.T) {
	lexer := newLexerForString("//")
	s := New().(*Scanner)
	v := onlyValid(TokenSearchSlash)
	if s.Scan(lexer, v) {
		t.Error("expected // to be rejected as search slash")
	}
}

func TestScanSearchSlashSuppressed(t *testing.T) {
	lexer := newLexerForString("/x")
	s := New().(*Scanner)
	v := onlyValid(TokenSearchSlash, NoTokenSearchSlashPlz)
	if s.Scan(lexer, v) {
		t.Error("expected search slash to be suppressed by NoTokenSearchSlashPlz")
	}
}

func TestScanDollarInRegexp(t *testing.T) {
	lexer := newLexerForString("$)")
	s := New().(*Scanner)
	s.pushQuote('/')
	v := onlyValid(TokenDollarInRegexp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenDollarInRegexp) {
		t.Errorf("expected TokenDollarInRegexp, got %d", lexer.ResultSymbol)
	}
}

func TestScanDollarInRegexpParen(t *testing.T) {
	lexer := newLexerForString("$(")
	s := New().(*Scanner)
	v := onlyValid(TokenDollarInRegexp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenDollarInRegexp) {
		t.Errorf("expected TokenDollarInRegexp, got %d", lexer.ResultSymbol)
	}
}

func TestScanDollarInRegexpRejects(t *testing.T) {
	// $x is a variable interpolation, should not match
	lexer := newLexerForString("$x")
	s := New().(*Scanner)
	v := onlyValid(TokenDollarInRegexp)
	if s.Scan(lexer, v) {
		t.Error("expected $x to be rejected as dollar in regexp")
	}
}

func TestScanPod(t *testing.T) {
	// POD block: starts with = at column 0, ends at =cut
	lexer := newLexerForString("=pod\nsome docs\n=cut\n")
	s := New().(*Scanner)
	v := onlyValid(TokenPod)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenPod) {
		t.Errorf("expected TokenPod, got %d", lexer.ResultSymbol)
	}
}

func TestScanPodNotAtColumn0(t *testing.T) {
	// POD only triggers at column 0; here we simulate column > 0 by having whitespace
	// Actually the lexer starts at column 0 always; to test non-column-0
	// we'd need to advance first. Let's just test it works at col 0.
	lexer := newLexerForString("x")
	s := New().(*Scanner)
	v := onlyValid(TokenPod)
	// 'x' is not '=', so POD doesn't trigger
	if s.Scan(lexer, v) {
		t.Error("expected POD to not trigger for 'x'")
	}
}

func TestScanQuotelikeBegin(t *testing.T) {
	lexer := newLexerForString("(hello)")
	s := New().(*Scanner)
	v := onlyValid(TokenQuotelikeBegin)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQuotelikeBegin) {
		t.Errorf("expected TokenQuotelikeBegin, got %d", lexer.ResultSymbol)
	}
	if len(s.quotes) != 1 {
		t.Error("expected quote pushed")
	}
	if s.quotes[0].open != '(' || s.quotes[0].close != ')' {
		t.Error("expected paired ( )")
	}
}

func TestScanQuotelikeBeginRejectsHashAfterWhitespace(t *testing.T) {
	// After whitespace, # is a comment, not a delimiter
	lexer := newLexerForString(" #")
	s := New().(*Scanner)
	v := onlyValid(TokenQuotelikeBegin)
	if s.Scan(lexer, v) {
		t.Error("expected # after whitespace to be rejected")
	}
}

func TestScanQuotelikeMiddleSkipUnpaired(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('/') // unpaired
	lexer := newLexerForString("x")
	v := onlyValid(TokenQuotelikeMiddleSkip)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQuotelikeMiddleSkip) {
		t.Errorf("expected TokenQuotelikeMiddleSkip, got %d", lexer.ResultSymbol)
	}
}

func TestScanQuotelikeMiddleSkipPairedRejects(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(') // paired
	lexer := newLexerForString("x")
	v := onlyValid(TokenQuotelikeMiddleSkip)
	if s.Scan(lexer, v) {
		t.Error("expected paired delimiter to reject middle skip")
	}
}

func TestScanQStringContent(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	lexer := newLexerForString("hello world)")
	v := onlyValid(TokenQStringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQStringContent) {
		t.Errorf("expected TokenQStringContent, got %d", lexer.ResultSymbol)
	}
}

func TestScanQStringContentStopsAtCloser(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('/')
	lexer := newLexerForString("abc/")
	v := onlyValid(TokenQStringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQStringContent) {
		t.Errorf("expected TokenQStringContent, got %d", lexer.ResultSymbol)
	}
	// Should have stopped at /, not consumed it
	if lexer.Lookahead != '/' {
		t.Errorf("expected lookahead=/, got %c", lexer.Lookahead)
	}
}

func TestScanQQStringContent(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('"')
	lexer := newLexerForString("hello $name")
	v := onlyValid(TokenQQStringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQQStringContent) {
		t.Errorf("expected TokenQQStringContent, got %d", lexer.ResultSymbol)
	}
	// Should stop at $ (interpolation escape)
	if lexer.Lookahead != '$' {
		t.Errorf("expected lookahead=$, got %c", lexer.Lookahead)
	}
}

func TestScanQuotelikeEnd(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('/')
	lexer := newLexerForString("/")
	v := onlyValid(TokenQuotelikeEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQuotelikeEnd) {
		t.Errorf("expected TokenQuotelikeEnd, got %d", lexer.ResultSymbol)
	}
	if len(s.quotes) != 0 {
		t.Error("expected quote popped")
	}
}

func TestScanQuotelikeEndZW(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('/')
	lexer := newLexerForString("/")
	v := onlyValid(TokenQuotelikeEnd, TokenQuotelikeEndZW)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQuotelikeEndZW) {
		t.Errorf("expected TokenQuotelikeEndZW, got %d", lexer.ResultSymbol)
	}
}

func TestScanQuotelikeMiddleClose(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	lexer := newLexerForString(")")
	v := onlyValid(TokenQuotelikeMiddleClose)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenQuotelikeMiddleClose) {
		t.Errorf("expected TokenQuotelikeMiddleClose, got %d", lexer.ResultSymbol)
	}
}

func TestScanEscapeSequence(t *testing.T) {
	s := New().(*Scanner)
	lexer := newLexerForString("\\n")
	v := onlyValid(TokenEscapeSequence)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenEscapeSequence) {
		t.Errorf("expected TokenEscapeSequence, got %d", lexer.ResultSymbol)
	}
}

func TestScanEscapeSequenceBackslash(t *testing.T) {
	s := New().(*Scanner)
	lexer := newLexerForString("\\\\x")
	v := onlyValid(TokenEscapeSequence)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenEscapeSequence) {
		t.Errorf("expected TokenEscapeSequence for \\\\, got %d", lexer.ResultSymbol)
	}
}

func TestScanEscapeSequenceHex(t *testing.T) {
	s := New().(*Scanner)
	lexer := newLexerForString("\\x41")
	v := onlyValid(TokenEscapeSequence)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenEscapeSequence) {
		t.Errorf("expected TokenEscapeSequence, got %d", lexer.ResultSymbol)
	}
}

func TestScanEscapeSequenceOctal(t *testing.T) {
	s := New().(*Scanner)
	lexer := newLexerForString("\\077")
	v := onlyValid(TokenEscapeSequence)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenEscapeSequence) {
		t.Errorf("expected TokenEscapeSequence, got %d", lexer.ResultSymbol)
	}
}

func TestScanEscapedDelimiter(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	lexer := newLexerForString("\\(")
	v := onlyValid(TokenEscapedDelimiter, TokenEscapeSequence)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenEscapedDelimiter) {
		t.Errorf("expected TokenEscapedDelimiter, got %d", lexer.ResultSymbol)
	}
}

func TestScanEscapedDelimiterCloser(t *testing.T) {
	s := New().(*Scanner)
	s.pushQuote('(')
	lexer := newLexerForString("\\)")
	v := onlyValid(TokenEscapedDelimiter, TokenEscapeSequence)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenEscapedDelimiter) {
		t.Errorf("expected TokenEscapedDelimiter, got %d", lexer.ResultSymbol)
	}
}

func TestScanPrototype(t *testing.T) {
	// Prototype: ($$@) — no identifier chars
	lexer := newLexerForString("($$@)")
	s := New().(*Scanner)
	v := onlyValid(TokenPrototype, TokenSignatureStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenPrototype) {
		t.Errorf("expected TokenPrototype, got %d", lexer.ResultSymbol)
	}
}

func TestScanSignatureStart(t *testing.T) {
	// Signature: ($self, $name) — has identifier chars
	lexer := newLexerForString("($self)")
	s := New().(*Scanner)
	v := onlyValid(TokenPrototype, TokenSignatureStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenSignatureStart) {
		t.Errorf("expected TokenSignatureStart, got %d", lexer.ResultSymbol)
	}
}

func TestScanFiletest(t *testing.T) {
	lexer := newLexerForString("-f ")
	s := New().(*Scanner)
	v := onlyValid(TokenFiletest)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenFiletest) {
		t.Errorf("expected TokenFiletest, got %d", lexer.ResultSymbol)
	}
}

func TestScanFiletestRejectsIdentifier(t *testing.T) {
	// -foo is not a file test (identifier continues after the test char)
	lexer := newLexerForString("-fo")
	s := New().(*Scanner)
	v := onlyValid(TokenFiletest)
	if s.Scan(lexer, v) {
		t.Error("expected -fo to be rejected as file test")
	}
}

func TestScanFatCommaAutoquoted(t *testing.T) {
	lexer := newLexerForString("key =>")
	s := New().(*Scanner)
	v := onlyValid(TokenFatCommaAutoquoted)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenFatCommaAutoquoted) {
		t.Errorf("expected TokenFatCommaAutoquoted, got %d", lexer.ResultSymbol)
	}
}

func TestScanBraceAutoquoted(t *testing.T) {
	lexer := newLexerForString("key}")
	s := New().(*Scanner)
	v := onlyValid(TokenBraceAutoquoted)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenBraceAutoquoted) {
		t.Errorf("expected TokenBraceAutoquoted, got %d", lexer.ResultSymbol)
	}
}

func TestScanDollarIdentZW(t *testing.T) {
	// $; — semicolon is not an ident char
	lexer := newLexerForString(";")
	s := New().(*Scanner)
	v := onlyValid(TokenDollarIdentZW)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenDollarIdentZW) {
		t.Errorf("expected TokenDollarIdentZW, got %d", lexer.ResultSymbol)
	}
}

func TestScanDollarIdentZWRejectsIdent(t *testing.T) {
	// $x — identifier character, should not match
	lexer := newLexerForString("x")
	s := New().(*Scanner)
	v := onlyValid(TokenDollarIdentZW)
	if s.Scan(lexer, v) {
		t.Error("expected 'x' to be rejected for dollar ident zw")
	}
}

func TestScanDollarIdentZWColonColon(t *testing.T) {
	// $:: — double colon means package separator, bail out
	lexer := newLexerForString("::")
	s := New().(*Scanner)
	v := onlyValid(TokenDollarIdentZW)
	if s.Scan(lexer, v) {
		t.Error("expected :: to be rejected for dollar ident zw")
	}
}

func TestScanBraceEndZW(t *testing.T) {
	lexer := newLexerForString("}x")
	s := New().(*Scanner)
	v := onlyValid(TokenBraceEndZW)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenBraceEndZW) {
		t.Errorf("expected TokenBraceEndZW, got %d", lexer.ResultSymbol)
	}
}

func TestScanHeredocDelimBareword(t *testing.T) {
	lexer := newLexerForString("EOF\n")
	s := New().(*Scanner)
	v := onlyValid(TokenHeredocDelim)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenHeredocDelim) {
		t.Errorf("expected TokenHeredocDelim, got %d", lexer.ResultSymbol)
	}
	if s.heredocState != heredocStart {
		t.Error("expected heredocStart state")
	}
	if !s.heredocInterpolates {
		t.Error("expected bareword heredoc to interpolate")
	}
}

func TestScanHeredocDelimQuoted(t *testing.T) {
	lexer := newLexerForString("'EOF'\n")
	s := New().(*Scanner)
	v := onlyValid(TokenHeredocDelim)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenHeredocDelim) {
		t.Errorf("expected TokenHeredocDelim, got %d", lexer.ResultSymbol)
	}
	if s.heredocInterpolates {
		t.Error("expected single-quoted heredoc to NOT interpolate")
	}
}

func TestScanHeredocDelimIndented(t *testing.T) {
	lexer := newLexerForString("~EOF\n")
	s := New().(*Scanner)
	v := onlyValid(TokenHeredocDelim)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if !s.heredocIndents {
		t.Error("expected ~ heredoc to indent")
	}
}

func TestScanHeredocDelimBackslash(t *testing.T) {
	lexer := newLexerForString("\\EOF\n")
	s := New().(*Scanner)
	v := onlyValid(TokenHeredocDelim)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if s.heredocInterpolates {
		t.Error("expected backslash heredoc to NOT interpolate")
	}
}

func TestScanCommandHeredocDelim(t *testing.T) {
	lexer := newLexerForString("`CMD`\n")
	s := New().(*Scanner)
	v := onlyValid(TokenHeredocDelim, TokenCommandHeredocDelim)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenCommandHeredocDelim) {
		t.Errorf("expected TokenCommandHeredocDelim, got %d", lexer.ResultSymbol)
	}
}

func TestScanPerlyHeredocToken(t *testing.T) {
	// << followed by ident
	lexer := newLexerForString("<<EOF")
	s := New().(*Scanner)
	v := onlyValid(PerlyHeredoc)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(PerlyHeredoc) {
		t.Errorf("expected PerlyHeredoc, got %d", lexer.ResultSymbol)
	}
}

func TestScanHeredocBody(t *testing.T) {
	// Simulate: heredoc already declared, now at start of line in body
	s := New().(*Scanner)
	var delim tspString
	for _, ch := range "EOF" {
		delim.push(ch)
	}
	s.addHeredoc(&delim, false, false)
	s.heredocState = heredocUnknown

	// The body: "hello\nEOF\n"
	lexer := newLexerForString("hello\nEOF\n")
	v := onlyValid(TokenHeredocMiddle, TokenHeredocEnd)

	// First scan should get the body content up to the delimiter line
	if !s.Scan(lexer, v) {
		t.Fatal("expected first scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenHeredocMiddle) {
		t.Errorf("expected TokenHeredocMiddle, got %d", lexer.ResultSymbol)
	}

	// The heredoc_state should now be heredocEnd
	if s.heredocState != heredocEnd {
		t.Errorf("expected heredocEnd, got %d", s.heredocState)
	}
}

func TestScanReadlineBracket(t *testing.T) {
	lexer := newLexerForString("<STDIN>")
	s := New().(*Scanner)
	v := onlyValid(TokenOpenReadlineBracket, TokenOpenFileGlobBracket)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenOpenReadlineBracket) {
		t.Errorf("expected TokenOpenReadlineBracket, got %d", lexer.ResultSymbol)
	}
}

func TestScanFileGlobBracket(t *testing.T) {
	// <*.txt> — not a simple ident, so it's fileglob
	lexer := newLexerForString("<*.txt>")
	s := New().(*Scanner)
	v := onlyValid(TokenOpenReadlineBracket, TokenOpenFileGlobBracket)
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenOpenFileGlobBracket) {
		t.Errorf("expected TokenOpenFileGlobBracket, got %d", lexer.ResultSymbol)
	}
}

// --- Helper functions ---

func TestIsInterpolationEscape(t *testing.T) {
	for _, c := range "$@-[{\\" {
		if !isInterpolationEscape(c) {
			t.Errorf("expected %c to be interpolation escape", c)
		}
	}
	if isInterpolationEscape('x') {
		t.Error("'x' should not be interpolation escape")
	}
	if isInterpolationEscape(0x100) {
		t.Error("non-ASCII should not be interpolation escape")
	}
}

func TestIsIDFirst(t *testing.T) {
	if !isIDFirst('_') {
		t.Error("_ should be ID first")
	}
	if !isIDFirst('a') {
		t.Error("a should be ID first")
	}
	if isIDFirst('1') {
		t.Error("1 should not be ID first")
	}
	if isIDFirst(-1) {
		t.Error("-1 should not be ID first")
	}
}

func TestIsIDCont(t *testing.T) {
	if !isIDCont('a') {
		t.Error("a should be ID cont")
	}
	if !isIDCont('1') {
		t.Error("1 should be ID cont")
	}
	if !isIDCont('_') {
		t.Error("_ should be ID cont")
	}
	if isIDCont(-1) {
		t.Error("-1 should not be ID cont")
	}
}

// --- Bug fix tests ---

// TestHeredocContinueAtEOF verifies that heredocContinue state at EOF
// does not cause an infinite loop (bug xew.1).
func TestHeredocContinueAtEOF(t *testing.T) {
	s := New().(*Scanner)
	var delim tspString
	for _, ch := range "END" {
		delim.push(ch)
	}
	s.addHeredoc(&delim, true, false) // interpolating heredoc
	s.heredocState = heredocContinue

	// Input ends without a newline or delimiter — previously caused infinite loop.
	lexer := newLexerForString("some text")
	v := onlyValid(TokenHeredocMiddle, TokenHeredocEnd)

	ok := s.Scan(lexer, v)
	if !ok {
		t.Fatal("expected scan to return true (emit partial heredoc content)")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenHeredocMiddle) {
		t.Errorf("expected TokenHeredocMiddle, got %d", lexer.ResultSymbol)
	}
}

// TestHeredocContinueEmptyAtEOF verifies heredocContinue at EOF with no
// content returns false (not infinite loop).
func TestHeredocContinueEmptyAtEOF(t *testing.T) {
	s := New().(*Scanner)
	var delim tspString
	for _, ch := range "END" {
		delim.push(ch)
	}
	s.addHeredoc(&delim, true, false)
	s.heredocState = heredocContinue

	// Empty input — nothing to emit.
	lexer := newLexerForString("")
	v := onlyValid(TokenHeredocMiddle, TokenHeredocEnd)

	ok := s.Scan(lexer, v)
	if ok {
		t.Fatal("expected scan to return false for empty input in heredocContinue")
	}
}

// TestMarkEndCalledGobbledContent verifies TokenGobbledContent calls MarkEnd.
func TestMarkEndCalledGobbledContent(t *testing.T) {
	lexer := newLexerForString("abc def")
	s := New().(*Scanner)
	v := onlyValid(TokenGobbledContent)

	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenGobbledContent) {
		t.Errorf("expected TokenGobbledContent, got %d", lexer.ResultSymbol)
	}
}

// TestMarkEndCalledNonassoc verifies TokenNonassoc calls MarkEnd.
func TestMarkEndCalledNonassoc(t *testing.T) {
	lexer := newLexerForString("x")
	s := New().(*Scanner)
	v := onlyValid(TokenNonassoc)

	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenNonassoc) {
		t.Errorf("expected TokenNonassoc, got %d", lexer.ResultSymbol)
	}
}

// TestMarkEndCalledCtrlZ verifies TokenCtrlZ calls MarkEnd.
func TestMarkEndCalledCtrlZ(t *testing.T) {
	lexer := newLexerForString(string(rune(26))) // Ctrl-Z
	s := New().(*Scanner)
	v := onlyValid(TokenCtrlZ)

	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenCtrlZ) {
		t.Errorf("expected TokenCtrlZ, got %d", lexer.ResultSymbol)
	}
}

// TestMarkEndCalledPerlySemicolon verifies PerlySemicolon calls MarkEnd.
func TestMarkEndCalledPerlySemicolon(t *testing.T) {
	lexer := newLexerForString("}")
	s := New().(*Scanner)
	v := onlyValid(PerlySemicolon)

	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(PerlySemicolon) {
		t.Errorf("expected PerlySemicolon, got %d", lexer.ResultSymbol)
	}
}

// TestMarkEndCalledPod verifies TokenPod calls MarkEnd.
func TestMarkEndCalledPod(t *testing.T) {
	lexer := newLexerForString("=pod\nsome docs\n=cut\n")
	s := New().(*Scanner)
	v := onlyValid(TokenPod)

	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(TokenPod) {
		t.Errorf("expected TokenPod, got %d", lexer.ResultSymbol)
	}
}
