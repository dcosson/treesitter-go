package rust

import (
	"testing"

	ts "github.com/dcosson/treesitter-go"
)

func newLexerForString(s string) *ts.Lexer {
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(s)))
	lexer.Start(ts.Length{})
	return lexer
}

func onlyValid(tokens ...int) []bool {
	v := make([]bool, ErrorSentinel+1)
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
	if rs.openingHashCount != 0 {
		t.Errorf("initial openingHashCount = %d, want 0", rs.openingHashCount)
	}
}

func TestSerializeDeserialize(t *testing.T) {
	s := New().(*Scanner)
	s.openingHashCount = 3
	buf := make([]byte, 16)
	n := s.Serialize(buf)
	if n != 1 {
		t.Fatalf("Serialize = %d bytes, want 1", n)
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if s2.openingHashCount != 3 {
		t.Errorf("openingHashCount = %d, want 3", s2.openingHashCount)
	}
}

func TestDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	s.openingHashCount = 5
	s.Deserialize(nil)
	if s.openingHashCount != 0 {
		t.Errorf("openingHashCount = %d, want 0 after empty deserialize", s.openingHashCount)
	}
}

func TestSerializeSmallBuffer(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 0)
	n := s.Serialize(buf)
	if n != 0 {
		t.Errorf("Serialize with empty buffer = %d, want 0", n)
	}
}

func TestErrorSentinel(t *testing.T) {
	lexer := newLexerForString("anything")
	s := New().(*Scanner)
	v := make([]bool, ErrorSentinel+1)
	for i := range v {
		v[i] = true
	}
	if s.Scan(lexer, v) {
		t.Error("expected Scan to return false when ErrorSentinel is valid")
	}
}

func TestScanStringContent(t *testing.T) {
	lexer := newLexerForString(`hello world"`)
	s := New().(*Scanner)
	v := onlyValid(StringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringContent to succeed")
	}
	if lexer.ResultSymbol != StringContent {
		t.Errorf("ResultSymbol = %d, want %d (StringContent)", lexer.ResultSymbol, StringContent)
	}
}

func TestScanStringContentEscape(t *testing.T) {
	lexer := newLexerForString(`\n`)
	s := New().(*Scanner)
	v := onlyValid(StringContent)
	// Backslash should stop the string content (escape starts)
	if s.Scan(lexer, v) {
		t.Error("expected StringContent to return false (no content before escape)")
	}
}

func TestScanStringContentEmpty(t *testing.T) {
	lexer := newLexerForString(`"rest`)
	s := New().(*Scanner)
	v := onlyValid(StringContent)
	// Quote immediately — no content
	if s.Scan(lexer, v) {
		t.Error("expected StringContent to return false when quote is first")
	}
}

func TestScanRawStringStart(t *testing.T) {
	lexer := newLexerForString(`r#"content"#`)
	s := New().(*Scanner)
	v := onlyValid(RawStringLiteralStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringLiteralStart to succeed")
	}
	if lexer.ResultSymbol != RawStringLiteralStart {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RawStringLiteralStart)
	}
	if s.openingHashCount != 1 {
		t.Errorf("openingHashCount = %d, want 1", s.openingHashCount)
	}
}

func TestScanRawStringStartMultipleHashes(t *testing.T) {
	lexer := newLexerForString(`r###"content"###`)
	s := New().(*Scanner)
	v := onlyValid(RawStringLiteralStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringLiteralStart to succeed")
	}
	if s.openingHashCount != 3 {
		t.Errorf("openingHashCount = %d, want 3", s.openingHashCount)
	}
}

func TestScanRawStringStartNoHash(t *testing.T) {
	lexer := newLexerForString(`r"content"`)
	s := New().(*Scanner)
	v := onlyValid(RawStringLiteralStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringLiteralStart to succeed")
	}
	if s.openingHashCount != 0 {
		t.Errorf("openingHashCount = %d, want 0", s.openingHashCount)
	}
}

func TestScanRawStringStartByteString(t *testing.T) {
	lexer := newLexerForString(`br"content"`)
	s := New().(*Scanner)
	v := onlyValid(RawStringLiteralStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringLiteralStart for byte raw string")
	}
	if s.openingHashCount != 0 {
		t.Errorf("openingHashCount = %d, want 0", s.openingHashCount)
	}
}

func TestScanRawStringContent(t *testing.T) {
	s := New().(*Scanner)
	s.openingHashCount = 1

	lexer := newLexerForString(`some content "# more`)
	v := onlyValid(RawStringLiteralContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringLiteralContent to succeed")
	}
	if lexer.ResultSymbol != RawStringLiteralContent {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RawStringLiteralContent)
	}
}

func TestScanRawStringEnd(t *testing.T) {
	s := New().(*Scanner)
	s.openingHashCount = 2

	lexer := newLexerForString(`"## rest`)
	v := onlyValid(RawStringLiteralEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringLiteralEnd to succeed")
	}
	if lexer.ResultSymbol != RawStringLiteralEnd {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RawStringLiteralEnd)
	}
}

func TestScanFloatLiteral(t *testing.T) {
	lexer := newLexerForString("3.14")
	s := New().(*Scanner)
	v := onlyValid(FloatLiteral)
	if !s.Scan(lexer, v) {
		t.Fatal("expected FloatLiteral to succeed")
	}
	if lexer.ResultSymbol != FloatLiteral {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, FloatLiteral)
	}
}

func TestScanFloatLiteralExponent(t *testing.T) {
	lexer := newLexerForString("1e10")
	s := New().(*Scanner)
	v := onlyValid(FloatLiteral)
	if !s.Scan(lexer, v) {
		t.Fatal("expected FloatLiteral with exponent to succeed")
	}
	if lexer.ResultSymbol != FloatLiteral {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, FloatLiteral)
	}
}

func TestScanFloatLiteralMethodCall(t *testing.T) {
	// 1.max(2) should NOT be a float (dot followed by letter)
	lexer := newLexerForString("1.max")
	s := New().(*Scanner)
	v := onlyValid(FloatLiteral)
	if s.Scan(lexer, v) {
		t.Error("expected FloatLiteral to return false for method call 1.max")
	}
}

func TestScanFloatLiteralRange(t *testing.T) {
	// 1.. should NOT be a float (range operator)
	lexer := newLexerForString("1..")
	s := New().(*Scanner)
	v := onlyValid(FloatLiteral)
	if s.Scan(lexer, v) {
		t.Error("expected FloatLiteral to return false for range 1..")
	}
}

func TestScanFloatLiteralWithSuffix(t *testing.T) {
	lexer := newLexerForString("3.14f64")
	s := New().(*Scanner)
	v := onlyValid(FloatLiteral)
	if !s.Scan(lexer, v) {
		t.Fatal("expected FloatLiteral with suffix to succeed")
	}
}

func TestScanLineDocContent(t *testing.T) {
	lexer := newLexerForString("some doc content\n")
	s := New().(*Scanner)
	v := onlyValid(LineDocContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected LineDocContent to succeed")
	}
	if lexer.ResultSymbol != LineDocContent {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, LineDocContent)
	}
}

func TestScanLineDocContentEOF(t *testing.T) {
	lexer := newLexerForString("doc at EOF")
	s := New().(*Scanner)
	v := onlyValid(LineDocContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected LineDocContent to succeed at EOF")
	}
}

func TestScanBlockInnerDocMarker(t *testing.T) {
	lexer := newLexerForString("! doc content */")
	s := New().(*Scanner)
	v := onlyValid(BlockInnerDocMarker, BlockCommentContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected BlockInnerDocMarker to succeed")
	}
	if lexer.ResultSymbol != BlockInnerDocMarker {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockInnerDocMarker)
	}
}

func TestScanBlockOuterDocMarker(t *testing.T) {
	lexer := newLexerForString("* doc content */")
	s := New().(*Scanner)
	v := onlyValid(BlockOuterDocMarker, BlockCommentContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected BlockOuterDocMarker to succeed")
	}
	if lexer.ResultSymbol != BlockOuterDocMarker {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockOuterDocMarker)
	}
}

func TestScanBlockCommentContent(t *testing.T) {
	lexer := newLexerForString("comment body */")
	s := New().(*Scanner)
	v := onlyValid(BlockCommentContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected BlockCommentContent to succeed")
	}
	if lexer.ResultSymbol != BlockCommentContent {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockCommentContent)
	}
}

func TestScanBlockCommentNested(t *testing.T) {
	// Nested: /* inner */ still in outer
	lexer := newLexerForString("/* nested */ end */")
	s := New().(*Scanner)
	v := onlyValid(BlockCommentContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected BlockCommentContent to succeed with nesting")
	}
}

func TestScanEmptyDocBlockComment(t *testing.T) {
	// /** */ — outer doc marker followed by empty content
	lexer := newLexerForString("*/")
	s := New().(*Scanner)
	v := onlyValid(BlockOuterDocMarker, BlockCommentContent)
	// '*' followed by '/' means empty block comment
	if s.Scan(lexer, v) {
		t.Error("expected false for empty block comment /**/")
	}
}
