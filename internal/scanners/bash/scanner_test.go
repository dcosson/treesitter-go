package bash

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
	v := make([]bool, ErrorRecovery+1)
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
	bs := s.(*Scanner)
	if len(bs.heredocs) != 0 {
		t.Errorf("initial heredocs = %d, want 0", len(bs.heredocs))
	}
}

func TestSerializeDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n != 4 {
		t.Fatalf("Serialize empty = %d bytes, want 4", n)
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if s2.lastGlobParenDepth != 0 {
		t.Errorf("lastGlobParenDepth = %d, want 0", s2.lastGlobParenDepth)
	}
	if len(s2.heredocs) != 0 {
		t.Errorf("heredocs = %d, want 0", len(s2.heredocs))
	}
}

func TestSerializeDeserializeWithHeredocs(t *testing.T) {
	s := New().(*Scanner)
	s.lastGlobParenDepth = 3
	s.heredocs = []heredoc{
		{isRaw: true, started: false, allowsIndent: true, delimiter: []byte("EOF")},
		{isRaw: false, started: true, allowsIndent: false, delimiter: []byte("END")},
	}

	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n == 0 {
		t.Fatal("Serialize returned 0")
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])

	if s2.lastGlobParenDepth != 3 {
		t.Errorf("lastGlobParenDepth = %d, want 3", s2.lastGlobParenDepth)
	}
	if len(s2.heredocs) != 2 {
		t.Fatalf("heredocs = %d, want 2", len(s2.heredocs))
	}
	if !s2.heredocs[0].isRaw || !s2.heredocs[0].allowsIndent {
		t.Error("heredoc[0] state mismatch")
	}
	if string(s2.heredocs[0].delimiter) != "EOF" {
		t.Errorf("heredoc[0].delimiter = %q, want %q", s2.heredocs[0].delimiter, "EOF")
	}
	if string(s2.heredocs[1].delimiter) != "END" {
		t.Errorf("heredoc[1].delimiter = %q, want %q", s2.heredocs[1].delimiter, "END")
	}
}

func TestDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	s.lastGlobParenDepth = 5
	s.heredocs = []heredoc{{delimiter: []byte("X")}}
	s.Deserialize(nil) // empty data should reset
	if s.heredocs != nil {
		t.Errorf("expected heredocs to be nil after reset, got %v", s.heredocs)
	}
	if s.lastGlobParenDepth != 0 {
		t.Errorf("expected lastGlobParenDepth to be 0 after reset, got %d", s.lastGlobParenDepth)
	}
}

func TestScanConcat(t *testing.T) {
	lexer := newLexerForString("abc")
	s := New().(*Scanner)
	v := onlyValid(Concat)
	if !s.Scan(lexer, v) {
		t.Fatal("expected Concat to succeed")
	}
	if lexer.ResultSymbol != Concat {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, Concat)
	}
}

func TestScanConcatWhitespace(t *testing.T) {
	lexer := newLexerForString(" ")
	s := New().(*Scanner)
	v := onlyValid(Concat)
	// Whitespace should not produce Concat
	if s.Scan(lexer, v) {
		t.Error("expected Concat to fail for whitespace")
	}
}

func TestScanEmptyValue(t *testing.T) {
	lexer := newLexerForString(" ")
	s := New().(*Scanner)
	v := onlyValid(EmptyValue)
	if !s.Scan(lexer, v) {
		t.Fatal("expected EmptyValue to succeed on whitespace")
	}
	if lexer.ResultSymbol != EmptyValue {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, EmptyValue)
	}
}

func TestScanEmptyValueEOF(t *testing.T) {
	lexer := newLexerForString("")
	s := New().(*Scanner)
	v := onlyValid(EmptyValue)
	if !s.Scan(lexer, v) {
		t.Fatal("expected EmptyValue to succeed on EOF")
	}
	if lexer.ResultSymbol != EmptyValue {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, EmptyValue)
	}
}

func TestScanFileDescriptor(t *testing.T) {
	lexer := newLexerForString("2>")
	s := New().(*Scanner)
	v := onlyValid(FileDescriptor, VariableName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected FileDescriptor to succeed")
	}
	if lexer.ResultSymbol != FileDescriptor {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, FileDescriptor)
	}
}

func TestScanVariableName(t *testing.T) {
	lexer := newLexerForString("foo=")
	s := New().(*Scanner)
	v := onlyValid(VariableName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected VariableName to succeed")
	}
	if lexer.ResultSymbol != VariableName {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, VariableName)
	}
}

func TestScanHeredocArrow(t *testing.T) {
	lexer := newLexerForString("<<")
	s := New().(*Scanner)
	v := onlyValid(HeredocArrow, VariableName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HeredocArrow to succeed")
	}
	if lexer.ResultSymbol != HeredocArrow {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, HeredocArrow)
	}
	if len(s.heredocs) != 1 {
		t.Errorf("heredocs = %d, want 1", len(s.heredocs))
	}
	if s.heredocs[0].allowsIndent {
		t.Error("heredoc should not allow indent for <<")
	}
}

func TestScanHeredocArrowDash(t *testing.T) {
	lexer := newLexerForString("<<-")
	s := New().(*Scanner)
	v := onlyValid(HeredocArrow, HeredocArrowDash, VariableName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HeredocArrowDash to succeed")
	}
	if lexer.ResultSymbol != HeredocArrowDash {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, HeredocArrowDash)
	}
	if len(s.heredocs) != 1 {
		t.Errorf("heredocs = %d, want 1", len(s.heredocs))
	}
	if !s.heredocs[0].allowsIndent {
		t.Error("heredoc should allow indent for <<-")
	}
}

func TestScanImmediateDoubleHash(t *testing.T) {
	lexer := newLexerForString("##x")
	s := New().(*Scanner)
	v := onlyValid(ImmediateDoubleHash)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ImmediateDoubleHash to succeed")
	}
	if lexer.ResultSymbol != ImmediateDoubleHash {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ImmediateDoubleHash)
	}
}

func TestScanImmediateDoubleHashRejectsBrace(t *testing.T) {
	lexer := newLexerForString("##}")
	s := New().(*Scanner)
	v := onlyValid(ImmediateDoubleHash)
	if s.Scan(lexer, v) {
		t.Error("expected ImmediateDoubleHash to fail when followed by }")
	}
}

func TestScanBraceStart(t *testing.T) {
	lexer := newLexerForString("{1..10}")
	s := New().(*Scanner)
	v := onlyValid(BraceStart)
	if !s.Scan(lexer, v) {
		t.Fatal("expected BraceStart to succeed")
	}
	if lexer.ResultSymbol != BraceStart {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BraceStart)
	}
}

func TestScanBraceStartInvalid(t *testing.T) {
	lexer := newLexerForString("{abc}")
	s := New().(*Scanner)
	v := onlyValid(BraceStart)
	if s.Scan(lexer, v) {
		t.Error("expected BraceStart to fail for non-range brace")
	}
}

func TestScanTestOperator(t *testing.T) {
	lexer := newLexerForString("-eq ")
	s := New().(*Scanner)
	v := onlyValid(TestOperator)
	if !s.Scan(lexer, v) {
		t.Fatal("expected TestOperator to succeed")
	}
	if lexer.ResultSymbol != TestOperator {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, TestOperator)
	}
}

func TestScanBareDollar(t *testing.T) {
	lexer := newLexerForString("$ ")
	s := New().(*Scanner)
	v := onlyValid(BareDollar)
	if !s.Scan(lexer, v) {
		t.Fatal("expected BareDollar to succeed")
	}
	if lexer.ResultSymbol != BareDollar {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BareDollar)
	}
}

func TestScanExternalExpansionSymHash(t *testing.T) {
	lexer := newLexerForString("# }")
	s := New().(*Scanner)
	v := onlyValid(ExternalExpansionSymHash, ExternalExpansionSymBang, ExternalExpansionSymEqual)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ExternalExpansionSymHash to succeed")
	}
	if lexer.ResultSymbol != ExternalExpansionSymHash {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ExternalExpansionSymHash)
	}
}

func TestScanConcatEscape(t *testing.T) {
	lexer := newLexerForString(`\"`)
	s := New().(*Scanner)
	v := onlyValid(Concat)
	if !s.Scan(lexer, v) {
		t.Fatal("expected Concat to succeed for escaped quote")
	}
	if lexer.ResultSymbol != Concat {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, Concat)
	}
}

func TestScanErrorRecoveryDisables(t *testing.T) {
	lexer := newLexerForString("abc")
	s := New().(*Scanner)
	v := onlyValid(Concat, ErrorRecovery)
	if s.Scan(lexer, v) {
		t.Error("expected Concat to fail when in error recovery")
	}
}
