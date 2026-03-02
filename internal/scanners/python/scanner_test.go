package python

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

// onlyValid returns a validSymbols slice where only the given tokens are valid.
func onlyValid(tokens ...int) []bool {
	v := make([]bool, Except+1)
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
	ps := s.(*Scanner)
	if len(ps.indents) != 1 || ps.indents[0] != 0 {
		t.Errorf("initial indents = %v, want [0]", ps.indents)
	}
	if len(ps.delimiters) != 0 {
		t.Errorf("initial delimiters = %d, want 0", len(ps.delimiters))
	}
}

func TestSerializeDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n != 2 {
		t.Fatalf("Serialize empty = %d bytes, want 2", n)
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if len(s2.indents) != 1 || s2.indents[0] != 0 {
		t.Errorf("indents = %v, want [0]", s2.indents)
	}
	if len(s2.delimiters) != 0 {
		t.Errorf("delimiters = %d, want 0", len(s2.delimiters))
	}
	if s2.insideInterpolatedString {
		t.Error("insideInterpolatedString should be false")
	}
}

func TestSerializeDeserializeWithState(t *testing.T) {
	s := New().(*Scanner)
	s.insideInterpolatedString = true
	s.indents = []uint16{0, 4, 8}
	var d delimiter
	d.setEndCharacter('"')
	d.setFormat()
	s.delimiters = []delimiter{d}

	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n == 0 {
		t.Fatal("Serialize returned 0")
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])

	if !s2.insideInterpolatedString {
		t.Error("insideInterpolatedString should be true")
	}
	if len(s2.delimiters) != 1 {
		t.Fatalf("delimiters = %d, want 1", len(s2.delimiters))
	}
	if !s2.delimiters[0].isFormat() {
		t.Error("delimiter should be format")
	}
	if s2.delimiters[0].endCharacter() != '"' {
		t.Errorf("delimiter endChar = %c, want '\"'", s2.delimiters[0].endCharacter())
	}
	// indents: [0] is always implicit, plus 4 and 8
	if len(s2.indents) != 3 {
		t.Fatalf("indents = %v, want [0 4 8]", s2.indents)
	}
	if s2.indents[1] != 4 || s2.indents[2] != 8 {
		t.Errorf("indents = %v, want [0 4 8]", s2.indents)
	}
}

func TestDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	s.insideInterpolatedString = true
	s.indents = []uint16{0, 4}
	s.Deserialize(nil)

	if s.insideInterpolatedString {
		t.Error("insideInterpolatedString should be false after empty deserialize")
	}
	if len(s.indents) != 1 || s.indents[0] != 0 {
		t.Errorf("indents = %v, want [0] after empty deserialize", s.indents)
	}
	if s.delimiters != nil {
		t.Errorf("delimiters should be nil after empty deserialize")
	}
}

func TestSerializeSmallBuffer(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 1)
	n := s.Serialize(buf)
	if n != 0 {
		t.Errorf("Serialize with tiny buffer = %d, want 0", n)
	}
}

func TestScanNewline(t *testing.T) {
	lexer := newLexerForString("\n")
	s := New().(*Scanner)
	v := onlyValid(Newline)
	if !s.Scan(lexer, v) {
		t.Fatal("expected Newline to succeed")
	}
	if lexer.ResultSymbol != Newline {
		t.Errorf("ResultSymbol = %d, want %d (Newline)", lexer.ResultSymbol, Newline)
	}
}

func TestScanIndent(t *testing.T) {
	lexer := newLexerForString("\n    x")
	s := New().(*Scanner)
	v := onlyValid(Indent, Dedent, Newline)
	if !s.Scan(lexer, v) {
		t.Fatal("expected Indent to succeed")
	}
	if lexer.ResultSymbol != Indent {
		t.Errorf("ResultSymbol = %d, want %d (Indent)", lexer.ResultSymbol, Indent)
	}
	if len(s.indents) != 2 || s.indents[1] != 4 {
		t.Errorf("indents = %v, want [0 4]", s.indents)
	}
}

func TestScanDedent(t *testing.T) {
	s := New().(*Scanner)
	s.indents = []uint16{0, 4}

	lexer := newLexerForString("\nx")
	v := onlyValid(Indent, Dedent, Newline)
	if !s.Scan(lexer, v) {
		t.Fatal("expected Dedent to succeed")
	}
	if lexer.ResultSymbol != Dedent {
		t.Errorf("ResultSymbol = %d, want %d (Dedent)", lexer.ResultSymbol, Dedent)
	}
	if len(s.indents) != 1 {
		t.Errorf("indents = %v, want [0]", s.indents)
	}
}

func TestScanStringStartSingle(t *testing.T) {
	lexer := newLexerForString("'hello'")
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed")
	}
	if lexer.ResultSymbol != StringStart {
		t.Errorf("ResultSymbol = %d, want %d (StringStart)", lexer.ResultSymbol, StringStart)
	}
	if len(s.delimiters) != 1 {
		t.Fatalf("delimiters = %d, want 1", len(s.delimiters))
	}
	if s.delimiters[0].endCharacter() != '\'' {
		t.Errorf("delimiter endChar = %c, want '", s.delimiters[0].endCharacter())
	}
}

func TestScanStringStartDouble(t *testing.T) {
	lexer := newLexerForString(`"hello"`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed")
	}
	if lexer.ResultSymbol != StringStart {
		t.Errorf("ResultSymbol = %d, want %d (StringStart)", lexer.ResultSymbol, StringStart)
	}
	if len(s.delimiters) != 1 {
		t.Fatalf("delimiters = %d, want 1", len(s.delimiters))
	}
	if s.delimiters[0].endCharacter() != '"' {
		t.Errorf("delimiter endChar = %c, want \"", s.delimiters[0].endCharacter())
	}
}

func TestScanStringStartTriple(t *testing.T) {
	lexer := newLexerForString(`"""content"""`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed")
	}
	if lexer.ResultSymbol != StringStart {
		t.Errorf("ResultSymbol = %d, want %d (StringStart)", lexer.ResultSymbol, StringStart)
	}
	if len(s.delimiters) != 1 {
		t.Fatalf("delimiters = %d, want 1", len(s.delimiters))
	}
	if !s.delimiters[0].isTriple() {
		t.Error("expected triple string delimiter")
	}
}

func TestScanStringStartFString(t *testing.T) {
	lexer := newLexerForString(`f"hello {name}"`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed")
	}
	if lexer.ResultSymbol != StringStart {
		t.Errorf("ResultSymbol = %d, want %d (StringStart)", lexer.ResultSymbol, StringStart)
	}
	if len(s.delimiters) != 1 {
		t.Fatalf("delimiters = %d, want 1", len(s.delimiters))
	}
	if !s.delimiters[0].isFormat() {
		t.Error("expected format string delimiter")
	}
	if s.delimiters[0].endCharacter() != '"' {
		t.Errorf("delimiter endChar = %c, want \"", s.delimiters[0].endCharacter())
	}
	if !s.insideInterpolatedString {
		t.Error("expected insideInterpolatedString to be true for f-string")
	}
}

func TestScanStringStartRaw(t *testing.T) {
	lexer := newLexerForString(`r"raw\nstring"`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed")
	}
	if !s.delimiters[0].isRaw() {
		t.Error("expected raw string delimiter")
	}
}

func TestScanStringStartBytes(t *testing.T) {
	lexer := newLexerForString(`b"bytes"`)
	s := New().(*Scanner)
	v := onlyValid(StringStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringStart to succeed")
	}
	if !s.delimiters[0].isBytes() {
		t.Error("expected bytes string delimiter")
	}
}

func TestScanStringContentAndEnd(t *testing.T) {
	// Simulate: after StringStart for a single-quoted string, scan content then end.
	s := New().(*Scanner)
	var d delimiter
	d.setEndCharacter('\'')
	s.delimiters = []delimiter{d}

	lexer := newLexerForString("hello'")
	v := onlyValid(StringContent, StringEnd)
	if !s.Scan(lexer, v) {
		t.Fatal("expected StringContent to succeed")
	}
	if lexer.ResultSymbol != StringContent {
		t.Errorf("ResultSymbol = %d, want %d (StringContent)", lexer.ResultSymbol, StringContent)
	}

	// Now scan for the string end.
	lexer2 := newLexerForString("'")
	if !s.Scan(lexer2, v) {
		t.Fatal("expected StringEnd to succeed")
	}
	if lexer2.ResultSymbol != StringEnd {
		t.Errorf("ResultSymbol = %d, want %d (StringEnd)", lexer2.ResultSymbol, StringEnd)
	}
	if len(s.delimiters) != 0 {
		t.Errorf("delimiters = %d, want 0 after StringEnd", len(s.delimiters))
	}
}

func TestScanEscapeInterpolation(t *testing.T) {
	s := New().(*Scanner)
	var d delimiter
	d.setEndCharacter('"')
	d.setFormat()
	s.delimiters = []delimiter{d}

	lexer := newLexerForString("{{")
	v := onlyValid(EscapeInterpolation, StringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected EscapeInterpolation to succeed")
	}
	if lexer.ResultSymbol != EscapeInterpolation {
		t.Errorf("ResultSymbol = %d, want %d (EscapeInterpolation)", lexer.ResultSymbol, EscapeInterpolation)
	}
}

func TestScanEscapeInterpolationCloseBrace(t *testing.T) {
	s := New().(*Scanner)
	var d delimiter
	d.setEndCharacter('"')
	d.setFormat()
	s.delimiters = []delimiter{d}

	lexer := newLexerForString("}}")
	v := onlyValid(EscapeInterpolation, StringContent)
	if !s.Scan(lexer, v) {
		t.Fatal("expected EscapeInterpolation to succeed for }}")
	}
	if lexer.ResultSymbol != EscapeInterpolation {
		t.Errorf("ResultSymbol = %d, want %d (EscapeInterpolation)", lexer.ResultSymbol, EscapeInterpolation)
	}
}

func TestScanTabIndent(t *testing.T) {
	lexer := newLexerForString("\n\tx")
	s := New().(*Scanner)
	v := onlyValid(Indent, Dedent, Newline)
	if !s.Scan(lexer, v) {
		t.Fatal("expected Indent to succeed with tab")
	}
	if lexer.ResultSymbol != Indent {
		t.Errorf("ResultSymbol = %d, want %d (Indent)", lexer.ResultSymbol, Indent)
	}
	// Tab counts as 8 spaces
	if len(s.indents) != 2 || s.indents[1] != 8 {
		t.Errorf("indents = %v, want [0 8]", s.indents)
	}
}

func TestDelimiterFlags(t *testing.T) {
	var d delimiter

	d.setEndCharacter('\'')
	if d.endCharacter() != '\'' {
		t.Errorf("endChar = %c, want '", d.endCharacter())
	}

	d = 0
	d.setEndCharacter('"')
	if d.endCharacter() != '"' {
		t.Errorf("endChar = %c, want \"", d.endCharacter())
	}

	d = 0
	d.setEndCharacter('`')
	if d.endCharacter() != '`' {
		t.Errorf("endChar = %c, want `", d.endCharacter())
	}

	d.setFormat()
	if !d.isFormat() {
		t.Error("expected format flag")
	}

	d.setRaw()
	if !d.isRaw() {
		t.Error("expected raw flag")
	}

	d.setTriple()
	if !d.isTriple() {
		t.Error("expected triple flag")
	}

	d.setBytes()
	if !d.isBytes() {
		t.Error("expected bytes flag")
	}
}
