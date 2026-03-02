package ruby

import (
	"testing"

	ts "github.com/treesitter-go/treesitter"
)

// newLexerForString creates a lexer initialized with the given string input.
func newLexerForString(s string) *ts.Lexer {
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(s)))
	lexer.Start(ts.Length{})
	return lexer
}

// newLexerForStringPastRangeStart creates a lexer with the given string input,
// but starts it at a non-zero byte offset so that IsAtIncludedRangeStart()
// returns false. This simulates a real parse where the scanner is called
// after some initial tokens have been consumed, which is the normal case
// for line-break detection (line breaks only matter between statements).
func newLexerForStringPastRangeStart(s string) *ts.Lexer {
	// Prepend a space that we skip over during start, so the lexer is past
	// byte 0 and the range-start flag is cleared.
	padded := " " + s
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(padded)))
	lexer.Start(ts.Length{})
	// Skip past the leading space
	lexer.Advance(true)
	return lexer
}

// allValid returns a validSymbols slice where all tokens are valid (except None).
func allValid() []bool {
	v := make([]bool, None+1)
	for i := range v {
		v[i] = true
	}
	v[None] = false
	return v
}

// onlyValid returns a validSymbols slice where only the given tokens are valid.
func onlyValid(tokens ...int) []bool {
	v := make([]bool, None+1)
	for _, t := range tokens {
		v[t] = true
	}
	return v
}

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
	rs := s.(*Scanner)
	if len(rs.literalStack) != 0 {
		t.Errorf("initial literalStack = %d, want 0", len(rs.literalStack))
	}
	if len(rs.openHeredocs) != 0 {
		t.Errorf("initial openHeredocs = %d, want 0", len(rs.openHeredocs))
	}
}

// --- Serialize/Deserialize tests ---

func TestSerializeDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n != 2 {
		t.Fatalf("Serialize empty = %d bytes, want 2", n)
	}
	if buf[0] != 0 || buf[1] != 0 {
		t.Fatalf("expected [0, 0], got [%d, %d]", buf[0], buf[1])
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if len(s2.literalStack) != 0 {
		t.Errorf("literalStack = %d, want 0", len(s2.literalStack))
	}
	if len(s2.openHeredocs) != 0 {
		t.Errorf("openHeredocs = %d, want 0", len(s2.openHeredocs))
	}
}

func TestDeserializeNilResetsState(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{{tokenType: StringStart, nestingDepth: 1}}
	s.openHeredocs = []heredoc{{word: []byte("EOF")}}
	s.hasLeadingWhitespace = true

	s.Deserialize(nil)
	if s.hasLeadingWhitespace {
		t.Error("expected hasLeadingWhitespace to be false")
	}
	if len(s.literalStack) != 0 {
		t.Errorf("literalStack = %d, want 0", len(s.literalStack))
	}
	if len(s.openHeredocs) != 0 {
		t.Errorf("openHeredocs = %d, want 0", len(s.openHeredocs))
	}
}

func TestSerializeDeserializeWithLiteralStack(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: StringStart, openDelimiter: '"', closeDelimiter: '"', nestingDepth: 1, allowsInterpolation: true},
		{tokenType: RegexStart, openDelimiter: '/', closeDelimiter: '/', nestingDepth: 1, allowsInterpolation: true},
		{tokenType: StringStart, openDelimiter: '(', closeDelimiter: ')', nestingDepth: 2, allowsInterpolation: false},
	}

	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n == 0 {
		t.Fatal("Serialize returned 0")
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])

	if len(s2.literalStack) != 3 {
		t.Fatalf("literalStack = %d, want 3", len(s2.literalStack))
	}
	// Verify first literal
	if s2.literalStack[0].tokenType != StringStart {
		t.Errorf("literal[0].tokenType = %d, want %d", s2.literalStack[0].tokenType, StringStart)
	}
	if s2.literalStack[0].openDelimiter != '"' {
		t.Errorf("literal[0].openDelimiter = %c, want '\"'", s2.literalStack[0].openDelimiter)
	}
	if !s2.literalStack[0].allowsInterpolation {
		t.Error("literal[0].allowsInterpolation = false, want true")
	}
	// Verify third literal
	if s2.literalStack[2].openDelimiter != '(' {
		t.Errorf("literal[2].openDelimiter = %c, want '('", s2.literalStack[2].openDelimiter)
	}
	if s2.literalStack[2].closeDelimiter != ')' {
		t.Errorf("literal[2].closeDelimiter = %c, want ')'", s2.literalStack[2].closeDelimiter)
	}
	if s2.literalStack[2].nestingDepth != 2 {
		t.Errorf("literal[2].nestingDepth = %d, want 2", s2.literalStack[2].nestingDepth)
	}
	if s2.literalStack[2].allowsInterpolation {
		t.Error("literal[2].allowsInterpolation = true, want false")
	}
}

func TestSerializeDeserializeWithHeredocState(t *testing.T) {
	s := New().(*Scanner)
	s.openHeredocs = []heredoc{
		{word: []byte("EOF"), endWordIndentationAllowed: true, allowsInterpolation: true, started: false},
		{word: []byte("SQL"), endWordIndentationAllowed: false, allowsInterpolation: false, started: true},
	}

	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n == 0 {
		t.Fatal("Serialize returned 0")
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])

	if len(s2.openHeredocs) != 2 {
		t.Fatalf("openHeredocs = %d, want 2", len(s2.openHeredocs))
	}
	if string(s2.openHeredocs[0].word) != "EOF" {
		t.Errorf("heredoc[0].word = %q, want %q", s2.openHeredocs[0].word, "EOF")
	}
	if !s2.openHeredocs[0].endWordIndentationAllowed {
		t.Error("heredoc[0].endWordIndentationAllowed = false, want true")
	}
	if !s2.openHeredocs[0].allowsInterpolation {
		t.Error("heredoc[0].allowsInterpolation = false, want true")
	}
	if s2.openHeredocs[0].started {
		t.Error("heredoc[0].started = true, want false")
	}
	if string(s2.openHeredocs[1].word) != "SQL" {
		t.Errorf("heredoc[1].word = %q, want %q", s2.openHeredocs[1].word, "SQL")
	}
	if s2.openHeredocs[1].endWordIndentationAllowed {
		t.Error("heredoc[1].endWordIndentationAllowed = true, want false")
	}
	if s2.openHeredocs[1].allowsInterpolation {
		t.Error("heredoc[1].allowsInterpolation = true, want false")
	}
	if !s2.openHeredocs[1].started {
		t.Error("heredoc[1].started = false, want true")
	}
}

func TestSerializeDeserializeWithBothStacksPopulated(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: SymbolStart, openDelimiter: '\'', closeDelimiter: '\'', nestingDepth: 1, allowsInterpolation: false},
	}
	s.openHeredocs = []heredoc{
		{word: []byte("HEREDOC"), endWordIndentationAllowed: true, allowsInterpolation: true, started: true},
	}

	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n == 0 {
		t.Fatal("Serialize returned 0")
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])

	if len(s2.literalStack) != 1 {
		t.Fatalf("literalStack = %d, want 1", len(s2.literalStack))
	}
	if len(s2.openHeredocs) != 1 {
		t.Fatalf("openHeredocs = %d, want 1", len(s2.openHeredocs))
	}
	if s2.literalStack[0].tokenType != SymbolStart {
		t.Errorf("literal[0].tokenType = %d, want %d", s2.literalStack[0].tokenType, SymbolStart)
	}
	if string(s2.openHeredocs[0].word) != "HEREDOC" {
		t.Errorf("heredoc[0].word = %q, want %q", s2.openHeredocs[0].word, "HEREDOC")
	}
}

func TestSerializeBufferTooSmall(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: StringStart, openDelimiter: '"', closeDelimiter: '"', nestingDepth: 1, allowsInterpolation: true},
	}
	// Buffer too small: need at least 1 (count) + 5 (literal) + 1 (heredoc count) = 7 bytes
	buf := make([]byte, 3)
	n := s.Serialize(buf)
	if n != 0 {
		t.Errorf("Serialize with small buffer = %d, want 0", n)
	}
}

// --- String scanning tests ---

func TestScanDoubleQuotedString(t *testing.T) {
	lexer := newLexerForString(`"hello"`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed for double-quoted string")
	}
	if lexer.ResultSymbol != ts.Symbol(StringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringStart)
	}
	if len(s.literalStack) != 1 {
		t.Fatalf("literalStack = %d, want 1", len(s.literalStack))
	}
	if !s.literalStack[0].allowsInterpolation {
		t.Error("expected double-quoted string to allow interpolation")
	}
}

func TestScanSingleQuotedString(t *testing.T) {
	lexer := newLexerForString(`'hello'`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed for single-quoted string")
	}
	if lexer.ResultSymbol != ts.Symbol(StringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringStart)
	}
	if len(s.literalStack) != 1 {
		t.Fatalf("literalStack = %d, want 1", len(s.literalStack))
	}
	if s.literalStack[0].allowsInterpolation {
		t.Error("expected single-quoted string to not allow interpolation")
	}
}

// --- Percent-string scanning tests ---

func TestScanPercentStringW(t *testing.T) {
	lexer := newLexerForString(`%w(foo bar)`)
	s := New().(*Scanner)
	v := onlyValid(StringArrayStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringArrayStart to succeed for percent-w")
	}
	if lexer.ResultSymbol != ts.Symbol(StringArrayStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringArrayStart)
	}
	if len(s.literalStack) != 1 {
		t.Fatalf("literalStack = %d, want 1", len(s.literalStack))
	}
	lit := s.literalStack[0]
	if lit.openDelimiter != '(' || lit.closeDelimiter != ')' {
		t.Errorf("delimiters = (%c, %c), want ('(', ')')", lit.openDelimiter, lit.closeDelimiter)
	}
	if lit.allowsInterpolation {
		t.Error("expected percent-w to not allow interpolation")
	}
}

func TestScanPercentStringQ(t *testing.T) {
	lexer := newLexerForString(`%q[hello]`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed for percent-q")
	}
	if lexer.ResultSymbol != ts.Symbol(StringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringStart)
	}
	if len(s.literalStack) != 1 {
		t.Fatalf("literalStack = %d, want 1", len(s.literalStack))
	}
	lit := s.literalStack[0]
	if lit.openDelimiter != '[' || lit.closeDelimiter != ']' {
		t.Errorf("delimiters = (%c, %c), want ('[', ']')", lit.openDelimiter, lit.closeDelimiter)
	}
	if lit.allowsInterpolation {
		t.Error("expected percent-q to not allow interpolation")
	}
}

func TestScanPercentStringCapQ(t *testing.T) {
	lexer := newLexerForString(`%Q{hello}`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed for percent-Q")
	}
	if lexer.ResultSymbol != ts.Symbol(StringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringStart)
	}
	lit := s.literalStack[0]
	if lit.openDelimiter != '{' || lit.closeDelimiter != '}' {
		t.Errorf("delimiters = (%c, %c), want ('{', '}')", lit.openDelimiter, lit.closeDelimiter)
	}
	if !lit.allowsInterpolation {
		t.Error("expected %Q to allow interpolation")
	}
}

func TestScanPercentStringR(t *testing.T) {
	lexer := newLexerForString(`%r<pattern>`)
	s := New().(*Scanner)
	v := onlyValid(RegexStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RegexStart to succeed for percent-r")
	}
	if lexer.ResultSymbol != ts.Symbol(RegexStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RegexStart)
	}
	lit := s.literalStack[0]
	if lit.openDelimiter != '<' || lit.closeDelimiter != '>' {
		t.Errorf("delimiters = (%c, %c), want ('<', '>')", lit.openDelimiter, lit.closeDelimiter)
	}
}

func TestScanPercentStringX(t *testing.T) {
	lexer := newLexerForString(`%x(ls)`)
	s := New().(*Scanner)
	v := onlyValid(SubshellStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected SubshellStart to succeed for percent-x")
	}
	if lexer.ResultSymbol != ts.Symbol(SubshellStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SubshellStart)
	}
}

func TestScanPercentStringS(t *testing.T) {
	lexer := newLexerForString(`%s(foo)`)
	s := New().(*Scanner)
	v := onlyValid(SimpleSymbol, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected SymbolStart to succeed for percent-s")
	}
	if lexer.ResultSymbol != ts.Symbol(SymbolStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SymbolStart)
	}
}

func TestScanPercentStringI(t *testing.T) {
	lexer := newLexerForString(`%i(foo bar)`)
	s := New().(*Scanner)
	v := onlyValid(SymbolArrayStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected SymbolArrayStart to succeed for percent-i")
	}
	if lexer.ResultSymbol != ts.Symbol(SymbolArrayStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SymbolArrayStart)
	}
	if s.literalStack[0].allowsInterpolation {
		t.Error("expected percent-i to not allow interpolation")
	}
}

func TestScanPercentStringCapI(t *testing.T) {
	lexer := newLexerForString(`%I(foo bar)`)
	s := New().(*Scanner)
	v := onlyValid(SymbolArrayStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected SymbolArrayStart to succeed for percent-I")
	}
	if lexer.ResultSymbol != ts.Symbol(SymbolArrayStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SymbolArrayStart)
	}
	if !s.literalStack[0].allowsInterpolation {
		t.Error("expected percent-I to allow interpolation")
	}
}

func TestScanPercentStringCapW(t *testing.T) {
	lexer := newLexerForString(`%W(foo bar)`)
	s := New().(*Scanner)
	v := onlyValid(StringArrayStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringArrayStart to succeed for percent-W")
	}
	if lexer.ResultSymbol != ts.Symbol(StringArrayStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringArrayStart)
	}
	if !s.literalStack[0].allowsInterpolation {
		t.Error("expected percent-W to allow interpolation")
	}
}

// --- Heredoc scanning tests ---

func TestScanHeredocStart(t *testing.T) {
	lexer := newLexerForString("<<EOF")
	s := New().(*Scanner)
	v := onlyValid(StringStart, HeredocStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HeredocStart to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(HeredocStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, HeredocStart)
	}
	if len(s.openHeredocs) != 1 {
		t.Fatalf("openHeredocs = %d, want 1", len(s.openHeredocs))
	}
	if string(s.openHeredocs[0].word) != "EOF" {
		t.Errorf("heredoc word = %q, want %q", s.openHeredocs[0].word, "EOF")
	}
	if !s.openHeredocs[0].allowsInterpolation {
		t.Error("expected bare heredoc to allow interpolation")
	}
	if s.openHeredocs[0].endWordIndentationAllowed {
		t.Error("expected bare heredoc to not allow indentation")
	}
}

func TestScanHeredocStartIndented(t *testing.T) {
	lexer := newLexerForString("<<~RUBY")
	s := New().(*Scanner)
	v := onlyValid(StringStart, HeredocStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HeredocStart to succeed for <<~")
	}
	if lexer.ResultSymbol != ts.Symbol(HeredocStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, HeredocStart)
	}
	if !s.openHeredocs[0].endWordIndentationAllowed {
		t.Error("expected <<~ heredoc to allow indentation")
	}
}

func TestScanHeredocStartQuoted(t *testing.T) {
	lexer := newLexerForString("<<'SQL'")
	s := New().(*Scanner)
	v := onlyValid(StringStart, HeredocStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HeredocStart to succeed for <<'SQL'")
	}
	if lexer.ResultSymbol != ts.Symbol(HeredocStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, HeredocStart)
	}
	if string(s.openHeredocs[0].word) != "SQL" {
		t.Errorf("heredoc word = %q, want %q", s.openHeredocs[0].word, "SQL")
	}
	if s.openHeredocs[0].allowsInterpolation {
		t.Error("expected single-quoted heredoc to not allow interpolation")
	}
}

// --- Line break detection tests ---

func TestScanLineBreak(t *testing.T) {
	lexer := newLexerForStringPastRangeStart("\nx")
	s := New().(*Scanner)
	v := onlyValid(LineBreak, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected LineBreak to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(LineBreak) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, LineBreak)
	}
}

func TestScanNoLineBreakBeforeDot(t *testing.T) {
	// Use newLexerForStringPastRangeStart so IsAtIncludedRangeStart is false.
	lexer := newLexerForStringPastRangeStart("\n.method")
	s := New().(*Scanner)
	v := onlyValid(LineBreak, StringStart)
	// A newline followed by `.` (method call) should NOT produce a line break
	if s.Scan(lexer, v) {
		if lexer.ResultSymbol == ts.Symbol(LineBreak) {
			t.Error("expected no LineBreak before method call dot")
		}
	}
}

func TestScanLineBreakBeforeRange(t *testing.T) {
	lexer := newLexerForStringPastRangeStart("\n..x")
	s := New().(*Scanner)
	v := onlyValid(LineBreak, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected LineBreak to succeed before range operator")
	}
	if lexer.ResultSymbol != ts.Symbol(LineBreak) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, LineBreak)
	}
}

// --- Operator disambiguation tests ---

func TestScanSplatStar(t *testing.T) {
	// Leading whitespace is needed so hasLeadingWhitespace gets set during scan
	lexer := newLexerForString(" *arg")
	s := New().(*Scanner)
	v := onlyValid(SplatStar, BinaryStar)
	if !s.Scan(lexer, v) {
		t.Fatal("expected star token to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(SplatStar) {
		t.Errorf("ResultSymbol = %d, want %d (SplatStar)", lexer.ResultSymbol, SplatStar)
	}
}

func TestScanBinaryStar(t *testing.T) {
	// Binary star: no leading whitespace
	lexer := newLexerForString("*arg")
	s := New().(*Scanner)
	s.hasLeadingWhitespace = false
	v := onlyValid(BinaryStar, SplatStar)
	if !s.Scan(lexer, v) {
		t.Fatal("expected star token to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(BinaryStar) {
		t.Errorf("ResultSymbol = %d, want %d (BinaryStar)", lexer.ResultSymbol, BinaryStar)
	}
}

func TestScanHashSplatStarStar(t *testing.T) {
	// Leading whitespace so hasLeadingWhitespace gets set during scan
	lexer := newLexerForString(" **opts")
	s := New().(*Scanner)
	v := onlyValid(HashSplatStarStar, BinaryStarStar)
	if !s.Scan(lexer, v) {
		t.Fatal("expected double star token to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(HashSplatStarStar) {
		t.Errorf("ResultSymbol = %d, want %d (HashSplatStarStar)", lexer.ResultSymbol, HashSplatStarStar)
	}
}

func TestScanBinaryStarStar(t *testing.T) {
	// No leading whitespace => BinaryStarStar
	lexer := newLexerForString("**2")
	s := New().(*Scanner)
	s.hasLeadingWhitespace = false
	v := onlyValid(BinaryStarStar, HashSplatStarStar)
	if !s.Scan(lexer, v) {
		t.Fatal("expected double star token to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(BinaryStarStar) {
		t.Errorf("ResultSymbol = %d, want %d (BinaryStarStar)", lexer.ResultSymbol, BinaryStarStar)
	}
}

func TestScanUnaryMinus(t *testing.T) {
	lexer := newLexerForString(" -x")
	s := New().(*Scanner)
	v := onlyValid(UnaryMinus, BinaryMinus, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected minus token to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(UnaryMinus) {
		t.Errorf("ResultSymbol = %d, want %d (UnaryMinus)", lexer.ResultSymbol, UnaryMinus)
	}
}

func TestScanBinaryMinus(t *testing.T) {
	lexer := newLexerForString("-x")
	s := New().(*Scanner)
	v := onlyValid(BinaryMinus, UnaryMinus)
	if !s.Scan(lexer, v) {
		t.Fatal("expected minus token to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(BinaryMinus) {
		t.Errorf("ResultSymbol = %d, want %d (BinaryMinus)", lexer.ResultSymbol, BinaryMinus)
	}
}

func TestScanUnaryMinusNum(t *testing.T) {
	lexer := newLexerForString(" -5")
	s := New().(*Scanner)
	v := onlyValid(UnaryMinusNum, BinaryMinus, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected unary minus num to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(UnaryMinusNum) {
		t.Errorf("ResultSymbol = %d, want %d (UnaryMinusNum)", lexer.ResultSymbol, UnaryMinusNum)
	}
}

func TestScanBlockAmpersand(t *testing.T) {
	lexer := newLexerForString("&method")
	s := New().(*Scanner)
	v := onlyValid(BlockAmpersand)
	if !s.Scan(lexer, v) {
		t.Fatal("expected BlockAmpersand to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockAmpersand) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockAmpersand)
	}
}

func TestScanBlockAmpersandRejectsDoubleAmpersand(t *testing.T) {
	lexer := newLexerForString("&&")
	s := New().(*Scanner)
	v := onlyValid(BlockAmpersand)
	if s.Scan(lexer, v) {
		t.Error("expected BlockAmpersand to fail for &&")
	}
}

// --- Symbol scanning tests ---

func TestScanSimpleSymbol(t *testing.T) {
	lexer := newLexerForString(":foo")
	s := New().(*Scanner)
	v := onlyValid(SymbolStart, SimpleSymbol, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected symbol scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(SimpleSymbol) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SimpleSymbol)
	}
}

func TestScanSymbolStartDoubleQuoted(t *testing.T) {
	lexer := newLexerForString(`:""`)
	s := New().(*Scanner)
	v := onlyValid(SymbolStart, SimpleSymbol, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected symbol start to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(SymbolStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SymbolStart)
	}
	if len(s.literalStack) != 1 {
		t.Fatalf("literalStack = %d, want 1", len(s.literalStack))
	}
	if !s.literalStack[0].allowsInterpolation {
		t.Error("expected :\"...\" symbol to allow interpolation")
	}
}

func TestScanSymbolStartSingleQuoted(t *testing.T) {
	lexer := newLexerForString(":'foo'")
	s := New().(*Scanner)
	v := onlyValid(SymbolStart, SimpleSymbol, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected symbol start to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(SymbolStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SymbolStart)
	}
	if s.literalStack[0].allowsInterpolation {
		t.Error("expected :'...' symbol to not allow interpolation")
	}
}

func TestScanSimpleSymbolOperator(t *testing.T) {
	lexer := newLexerForString(":+")
	s := New().(*Scanner)
	v := onlyValid(SymbolStart, SimpleSymbol, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected operator symbol scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(SimpleSymbol) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SimpleSymbol)
	}
}

// --- Hash key symbol test ---

func TestScanHashKeySymbol(t *testing.T) {
	lexer := newLexerForString("key:")
	s := New().(*Scanner)
	v := onlyValid(HashKeySymbol, IdentifierSuffix, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HashKeySymbol to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(HashKeySymbol) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, HashKeySymbol)
	}
}

func TestScanHashKeySymbolRejectsDoubleColon(t *testing.T) {
	lexer := newLexerForString("Foo::")
	s := New().(*Scanner)
	v := onlyValid(HashKeySymbol, IdentifierSuffix, ConstantSuffix, StringStart)
	if s.Scan(lexer, v) {
		if lexer.ResultSymbol == ts.Symbol(HashKeySymbol) {
			t.Error("expected HashKeySymbol to fail for double colon")
		}
	}
}

// --- Short interpolation test ---

func TestScanShortInterpolationInString(t *testing.T) {
	// Simulate being inside a string with allows_interpolation=true.
	// We set up the literal stack and then scan `#@var`.
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: StringStart, openDelimiter: '"', closeDelimiter: '"', nestingDepth: 1, allowsInterpolation: true},
	}

	lexer := newLexerForString("#@var\"")
	v := onlyValid(StringContent, StringEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected short interpolation scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(ShortInterpolation) {
		t.Errorf("ResultSymbol = %d, want %d (ShortInterpolation)", lexer.ResultSymbol, ShortInterpolation)
	}
}

func TestScanShortInterpolationDollarInString(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: StringStart, openDelimiter: '"', closeDelimiter: '"', nestingDepth: 1, allowsInterpolation: true},
	}

	lexer := newLexerForString("#$var\"")
	v := onlyValid(StringContent, StringEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected short interpolation scan to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(ShortInterpolation) {
		t.Errorf("ResultSymbol = %d, want %d (ShortInterpolation)", lexer.ResultSymbol, ShortInterpolation)
	}
}

// --- Regex start test ---

func TestScanRegexStart(t *testing.T) {
	lexer := newLexerForString("/pattern/")
	s := New().(*Scanner)
	v := onlyValid(RegexStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RegexStart to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(RegexStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RegexStart)
	}
}

func TestScanRegexStartRejectsWhenForwardSlashValid(t *testing.T) {
	// When ForwardSlash is also valid and no leading whitespace, regex should fail
	lexer := newLexerForString("/x")
	s := New().(*Scanner)
	s.hasLeadingWhitespace = false
	v := onlyValid(RegexStart, ForwardSlash, StringStart)
	if s.Scan(lexer, v) {
		t.Error("expected RegexStart to fail when ForwardSlash valid and no leading whitespace")
	}
}

// --- Element reference bracket test ---

func TestScanElementReferenceBracket(t *testing.T) {
	lexer := newLexerForString("[0]")
	s := New().(*Scanner)
	s.hasLeadingWhitespace = false
	v := onlyValid(ElementReferenceBracket, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ElementReferenceBracket to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(ElementReferenceBracket) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ElementReferenceBracket)
	}
}

func TestScanElementReferenceBracketWithWhitespace(t *testing.T) {
	// With whitespace and StringStart valid, bracket should NOT be element reference
	lexer := newLexerForString(" [0]")
	s := New().(*Scanner)
	v := onlyValid(ElementReferenceBracket, StringStart)
	if s.Scan(lexer, v) {
		if lexer.ResultSymbol == ts.Symbol(ElementReferenceBracket) {
			t.Error("expected ElementReferenceBracket to fail with whitespace and StringStart valid")
		}
	}
}

// --- Identifier/constant suffix tests ---

func TestScanIdentifierSuffix(t *testing.T) {
	lexer := newLexerForString("foo!")
	s := New().(*Scanner)
	v := onlyValid(IdentifierSuffix, HashKeySymbol, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected IdentifierSuffix to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(IdentifierSuffix) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, IdentifierSuffix)
	}
}

func TestScanConstantSuffix(t *testing.T) {
	lexer := newLexerForString("Foo!")
	s := New().(*Scanner)
	v := onlyValid(ConstantSuffix, IdentifierSuffix, HashKeySymbol, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ConstantSuffix to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(ConstantSuffix) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ConstantSuffix)
	}
}

func TestScanIdentifierSuffixRejectsBangEquals(t *testing.T) {
	// foo!= should not match (it's !=)
	lexer := newLexerForString("foo!=")
	s := New().(*Scanner)
	v := onlyValid(IdentifierSuffix, HashKeySymbol, StringStart)
	if s.Scan(lexer, v) {
		if lexer.ResultSymbol == ts.Symbol(IdentifierSuffix) {
			t.Error("expected IdentifierSuffix to fail for foo!=")
		}
	}
}

// --- Singleton class << test ---

func TestScanSingletonClassLeftAngle(t *testing.T) {
	lexer := newLexerForString("<<")
	s := New().(*Scanner)
	v := onlyValid(SingletonClassLeftAngleLeftAngle)
	if !s.Scan(lexer, v) {
		t.Fatal("expected SingletonClassLeftAngleLeftAngle to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(SingletonClassLeftAngleLeftAngle) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SingletonClassLeftAngleLeftAngle)
	}
}

func TestScanSingletonClassLeftAngleRejectsSingle(t *testing.T) {
	lexer := newLexerForString("< x")
	s := New().(*Scanner)
	v := onlyValid(SingletonClassLeftAngleLeftAngle)
	if s.Scan(lexer, v) {
		t.Error("expected SingletonClassLeftAngleLeftAngle to fail for single <")
	}
}

// --- Subshell test ---

func TestScanSubshellStart(t *testing.T) {
	lexer := newLexerForString("`ls`")
	s := New().(*Scanner)
	v := onlyValid(SubshellStart, StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected SubshellStart to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(SubshellStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SubshellStart)
	}
}

// --- String content and end tests ---

func TestScanStringContent(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: StringStart, openDelimiter: '\'', closeDelimiter: '\'', nestingDepth: 1, allowsInterpolation: false},
	}

	lexer := newLexerForString("hello'")
	v := onlyValid(StringContent, StringEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringContent to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(StringContent) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringContent)
	}
}

func TestScanStringEnd(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: StringStart, openDelimiter: '\'', closeDelimiter: '\'', nestingDepth: 1, allowsInterpolation: false},
	}

	lexer := newLexerForString("'rest")
	v := onlyValid(StringContent, StringEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringEnd to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(StringEnd) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringEnd)
	}
	if len(s.literalStack) != 0 {
		t.Errorf("literalStack = %d, want 0 after StringEnd", len(s.literalStack))
	}
}

// --- Star equals rejection test ---

func TestScanStarEqualsRejects(t *testing.T) {
	// *= should not be treated as SplatStar or BinaryStar
	lexer := newLexerForString("*=")
	s := New().(*Scanner)
	v := onlyValid(SplatStar, BinaryStar)
	if s.Scan(lexer, v) {
		t.Error("expected star= to be rejected")
	}
}

// --- Backslash line continuation in whitespace ---

func TestScanBackslashLineContinuation(t *testing.T) {
	lexer := newLexerForStringPastRangeStart("\\\n  x")
	s := New().(*Scanner)
	v := onlyValid(LineBreak, StringStart)
	// Line continuation should skip the newline and continue scanning
	result := s.Scan(lexer, v)
	// After line continuation, we should reach 'x' without producing a line break
	if result && lexer.ResultSymbol == ts.Symbol(LineBreak) {
		t.Error("expected line continuation to suppress line break")
	}
}

// --- Percent string with unbalanced delimiters ---

func TestScanPercentStringUnbalancedDelimiter(t *testing.T) {
	lexer := newLexerForString(`%|hello|`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed for %|...|")
	}
	if lexer.ResultSymbol != ts.Symbol(StringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringStart)
	}
	lit := s.literalStack[0]
	if lit.openDelimiter != '|' || lit.closeDelimiter != '|' {
		t.Errorf("delimiters = (%c, %c), want ('|', '|')", lit.openDelimiter, lit.closeDelimiter)
	}
}

// --- Regex end with flags ---

func TestScanRegexEndWithFlags(t *testing.T) {
	s := New().(*Scanner)
	s.literalStack = []literal{
		{tokenType: RegexStart, openDelimiter: '/', closeDelimiter: '/', nestingDepth: 1, allowsInterpolation: true},
	}

	lexer := newLexerForString("/im rest")
	v := onlyValid(StringContent, StringEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected regex end to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(StringEnd) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StringEnd)
	}
	if len(s.literalStack) != 0 {
		t.Errorf("literalStack = %d, want 0 after regex end", len(s.literalStack))
	}
}

// --- isIdenChar test ---

func TestIsIdenChar(t *testing.T) {
	// Identifier characters
	if !isIdenChar('a') {
		t.Error("expected 'a' to be iden char")
	}
	if !isIdenChar('Z') {
		t.Error("expected 'Z' to be iden char")
	}
	if !isIdenChar('9') {
		t.Error("expected '9' to be iden char")
	}
	if !isIdenChar('_') {
		t.Error("expected '_' to be iden char")
	}
	// Non-identifier characters
	if isIdenChar(':') {
		t.Error("expected ':' to not be iden char")
	}
	if isIdenChar(' ') {
		t.Error("expected ' ' to not be iden char")
	}
	if isIdenChar('(') {
		t.Error("expected '(' to not be iden char")
	}
	if isIdenChar(-1) {
		t.Error("expected -1 (EOF) to not be iden char")
	}
}
