package cpp

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
	v := make([]bool, RawStringContent+1)
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
	cs := s.(*Scanner)
	if len(cs.delimiter) != 0 {
		t.Errorf("initial delimiter = %v, want empty", cs.delimiter)
	}
}

func TestSerializeDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 256)
	n := s.Serialize(buf)
	if n != 0 {
		t.Fatalf("Serialize empty = %d bytes, want 0", n)
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if len(s2.delimiter) != 0 {
		t.Errorf("delimiter = %v, want empty", s2.delimiter)
	}
}

func TestSerializeDeserializeWithDelimiter(t *testing.T) {
	s := New().(*Scanner)
	s.delimiter = []int32{'f', 'o', 'o'}

	buf := make([]byte, 256)
	n := s.Serialize(buf)
	if n != 12 { // 3 runes * 4 bytes each
		t.Fatalf("Serialize = %d bytes, want 12", n)
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if len(s2.delimiter) != 3 {
		t.Fatalf("delimiter length = %d, want 3", len(s2.delimiter))
	}
	if string([]rune{rune(s2.delimiter[0]), rune(s2.delimiter[1]), rune(s2.delimiter[2])}) != "foo" {
		t.Errorf("delimiter = %v, want [f o o]", s2.delimiter)
	}
}

func TestDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	s.delimiter = []int32{'x'}
	s.Deserialize(nil)
	if s.delimiter != nil {
		t.Errorf("delimiter = %v, want nil after empty deserialize", s.delimiter)
	}
}

func TestDeserializeBadLength(t *testing.T) {
	s := New().(*Scanner)
	// 3 bytes is not a multiple of wcharSize (4)
	s.Deserialize([]byte{1, 2, 3})
	if s.delimiter != nil {
		t.Errorf("delimiter = %v, want nil for bad length", s.delimiter)
	}
}

func TestSerializeSmallBuffer(t *testing.T) {
	s := New().(*Scanner)
	s.delimiter = []int32{'a', 'b', 'c'}
	// Buffer only fits 1 rune (5 bytes) but needs 12; should fail
	buf := make([]byte, 5)
	n := s.Serialize(buf)
	if n != 0 {
		t.Errorf("Serialize with small buffer = %d, want 0 (buffer too small)", n)
	}
}

func TestErrorRecovery(t *testing.T) {
	lexer := newLexerForString("anything")
	s := New().(*Scanner)
	v := onlyValid(RawStringDelimiter, RawStringContent)
	if s.Scan(lexer, v) {
		t.Error("expected false in error recovery mode (both valid)")
	}
}

func TestScanOpeningDelimiter(t *testing.T) {
	lexer := newLexerForString("foo(")
	s := New().(*Scanner)
	v := onlyValid(RawStringDelimiter)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringDelimiter to succeed")
	}
	if lexer.ResultSymbol != RawStringDelimiter {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RawStringDelimiter)
	}
	if len(s.delimiter) != 3 {
		t.Fatalf("delimiter length = %d, want 3", len(s.delimiter))
	}
	for i, expected := range []int32{'f', 'o', 'o'} {
		if s.delimiter[i] != expected {
			t.Errorf("delimiter[%d] = %c, want %c", i, s.delimiter[i], expected)
		}
	}
}

func TestScanEmptyDelimiter(t *testing.T) {
	// Empty delimiter (just open paren) should return false
	lexer := newLexerForString("(content)")
	s := New().(*Scanner)
	v := onlyValid(RawStringDelimiter)
	if s.Scan(lexer, v) {
		t.Error("expected false for empty delimiter")
	}
}

func TestScanDelimiterWithSpace(t *testing.T) {
	// Space is not a valid d-char
	lexer := newLexerForString("foo bar(")
	s := New().(*Scanner)
	v := onlyValid(RawStringDelimiter)
	if s.Scan(lexer, v) {
		t.Error("expected false for delimiter with space")
	}
}

func TestScanDelimiterFailureResetsState(t *testing.T) {
	// After a failed opening scan (e.g., backslash mid-delimiter),
	// the delimiter should be cleared so a subsequent call takes the
	// opening path, not the closing path.
	s := New().(*Scanner)
	v := onlyValid(RawStringDelimiter)

	lexer1 := newLexerForString("abc\\def(")
	if s.Scan(lexer1, v) {
		t.Error("expected false for delimiter with backslash")
	}
	if s.delimiter != nil {
		t.Errorf("delimiter should be nil after failed open, got %v", s.delimiter)
	}

	// Now a valid opening should work (takes opening path, not closing)
	lexer2 := newLexerForString("xyz(")
	if !s.Scan(lexer2, v) {
		t.Fatal("expected successful opening delimiter after prior failure")
	}
	if len(s.delimiter) != 3 {
		t.Errorf("delimiter = %v, want [x y z]", s.delimiter)
	}
}

func TestScanDelimiterWithBackslash(t *testing.T) {
	lexer := newLexerForString("foo\\bar(")
	s := New().(*Scanner)
	v := onlyValid(RawStringDelimiter)
	if s.Scan(lexer, v) {
		t.Error("expected false for delimiter with backslash")
	}
}

func TestScanDelimiterTooLong(t *testing.T) {
	lexer := newLexerForString("abcdefghijklmnopq(") // 17 chars, max is 16
	s := New().(*Scanner)
	v := onlyValid(RawStringDelimiter)
	if s.Scan(lexer, v) {
		t.Error("expected false for delimiter exceeding max length")
	}
}

func TestScanClosingDelimiter(t *testing.T) {
	s := New().(*Scanner)
	s.delimiter = []int32{'f', 'o', 'o'}

	lexer := newLexerForString("foo")
	v := onlyValid(RawStringDelimiter)
	if !s.Scan(lexer, v) {
		t.Fatal("expected closing delimiter to match")
	}
	if s.delimiter != nil {
		t.Errorf("delimiter should be nil after closing match, got %v", s.delimiter)
	}
}

func TestScanClosingDelimiterMismatch(t *testing.T) {
	s := New().(*Scanner)
	s.delimiter = []int32{'f', 'o', 'o'}

	lexer := newLexerForString("bar")
	v := onlyValid(RawStringDelimiter)
	if s.Scan(lexer, v) {
		t.Error("expected false for closing delimiter mismatch")
	}
}

func TestScanRawStringContent(t *testing.T) {
	// Content until )delimiter"
	s := New().(*Scanner)
	s.delimiter = []int32{'d'}

	lexer := newLexerForString("hello world)d\"")
	v := onlyValid(RawStringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringContent to succeed")
	}
	if lexer.ResultSymbol != RawStringContent {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RawStringContent)
	}
}

func TestScanRawStringContentEmptyDelimiter(t *testing.T) {
	// R"(content)" — empty delimiter
	s := New().(*Scanner)
	s.delimiter = nil // empty delimiter

	lexer := newLexerForString("hello)\"")
	v := onlyValid(RawStringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringContent to succeed with empty delimiter")
	}
}

func TestScanRawStringContentEOF(t *testing.T) {
	s := New().(*Scanner)
	s.delimiter = []int32{'d'}

	lexer := newLexerForString("incomplete content")
	v := onlyValid(RawStringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringContent to succeed at EOF (incomplete)")
	}
}

func TestScanRawStringContentWithFalseClose(t *testing.T) {
	// Content has ) but not followed by delimiter"
	s := New().(*Scanner)
	s.delimiter = []int32{'x'}

	lexer := newLexerForString("hello ) world)x\"")
	v := onlyValid(RawStringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected RawStringContent to succeed past false close")
	}
}

func TestFullRawStringLifecycle(t *testing.T) {
	// Simulate R"foo(content)foo" lifecycle:
	// 1. Scan opening delimiter "foo("
	// 2. Scan content "content)"
	// 3. Scan closing delimiter "foo"
	s := New().(*Scanner)

	// Step 1: Opening delimiter
	lexer1 := newLexerForString("foo(")
	v := onlyValid(RawStringDelimiter)
	if !s.Scan(lexer1, v) {
		t.Fatal("step 1: opening delimiter failed")
	}
	if len(s.delimiter) != 3 {
		t.Fatalf("step 1: delimiter = %v, want [f o o]", s.delimiter)
	}

	// Step 2: Content
	lexer2 := newLexerForString("hello world)foo\"")
	v2 := onlyValid(RawStringContent)
	if !s.Scan(lexer2, v2) {
		t.Fatal("step 2: content scan failed")
	}

	// Step 3: Closing delimiter
	lexer3 := newLexerForString("foo")
	v3 := onlyValid(RawStringDelimiter)
	if !s.Scan(lexer3, v3) {
		t.Fatal("step 3: closing delimiter failed")
	}
	if s.delimiter != nil {
		t.Errorf("step 3: delimiter should be nil after close, got %v", s.delimiter)
	}
}
