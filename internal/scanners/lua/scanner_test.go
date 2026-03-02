package lua

import (
	"testing"

	ts "github.com/treesitter-go/treesitter"
)

// makeLexer creates a Lexer positioned at the start of input.
func makeLexer(input string) *ts.Lexer {
	l := ts.NewLexer()
	l.SetInput(ts.NewStringInput([]byte(input)))
	l.Start(ts.Length{})
	return l
}

// allValid returns a validSymbols slice with all 6 tokens valid.
func allValid() []bool {
	return []bool{true, true, true, true, true, true}
}

// onlyValid returns a validSymbols slice with only the specified tokens valid.
func onlyValid(tokens ...int) []bool {
	v := make([]bool, 6)
	for _, t := range tokens {
		v[t] = true
	}
	return v
}

func TestSerializeDeserialize(t *testing.T) {
	s := &Scanner{endingChar: '\n', levelCount: 3}
	buf := make([]byte, 256)
	n := s.Serialize(buf)
	if n != 2 {
		t.Fatalf("Serialize returned %d, want 2", n)
	}

	s2 := &Scanner{}
	s2.Deserialize(buf[:n])
	if s2.endingChar != '\n' {
		t.Errorf("endingChar = %d, want %d", s2.endingChar, '\n')
	}
	if s2.levelCount != 3 {
		t.Errorf("levelCount = %d, want 3", s2.levelCount)
	}
}

func TestDeserializeEmpty(t *testing.T) {
	s := &Scanner{endingChar: 42, levelCount: 7}
	s.Deserialize([]byte{})
	if s.endingChar != 0 || s.levelCount != 0 {
		t.Errorf("after empty deserialize: endingChar=%d, levelCount=%d; want 0,0",
			s.endingChar, s.levelCount)
	}
}

func TestDeserializePartial(t *testing.T) {
	s := &Scanner{}
	s.Deserialize([]byte{99})
	if s.endingChar != 99 {
		t.Errorf("endingChar = %d, want 99", s.endingChar)
	}
	if s.levelCount != 0 {
		t.Errorf("levelCount = %d, want 0", s.levelCount)
	}
}

func TestBlockStringStartSimple(t *testing.T) {
	s := &Scanner{}
	lexer := makeLexer("[[hello]]")
	valid := onlyValid(BlockStringStart)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for [[ block string start")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockStringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockStringStart)
	}
	if s.levelCount != 0 {
		t.Errorf("levelCount = %d, want 0", s.levelCount)
	}
}

func TestBlockStringStartWithLevel(t *testing.T) {
	s := &Scanner{}
	lexer := makeLexer("[==[content]==]")
	valid := onlyValid(BlockStringStart)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for [==[ block string start")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockStringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockStringStart)
	}
	if s.levelCount != 2 {
		t.Errorf("levelCount = %d, want 2", s.levelCount)
	}
}

func TestBlockStringContent(t *testing.T) {
	s := &Scanner{levelCount: 0}
	lexer := makeLexer("hello world]]")
	valid := onlyValid(BlockStringContent)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for block string content")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockStringContent) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockStringContent)
	}
}

func TestBlockStringContentWithLevel(t *testing.T) {
	s := &Scanner{levelCount: 1}
	// Content has ]] which should NOT end level-1, then ]=] which should end it
	lexer := makeLexer("data]]more]=]")
	valid := onlyValid(BlockStringContent)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for block string content with level")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockStringContent) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockStringContent)
	}
}

func TestBlockStringEnd(t *testing.T) {
	s := &Scanner{levelCount: 0}
	lexer := makeLexer("]]rest")
	valid := onlyValid(BlockStringEnd)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for ]] block string end")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockStringEnd) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockStringEnd)
	}
	if s.levelCount != 0 || s.endingChar != 0 {
		t.Errorf("state not reset: endingChar=%d, levelCount=%d", s.endingChar, s.levelCount)
	}
}

func TestBlockStringEndWithLevel(t *testing.T) {
	s := &Scanner{levelCount: 2}
	lexer := makeLexer("]==]rest")
	valid := onlyValid(BlockStringEnd)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for ]==] block string end")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockStringEnd) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockStringEnd)
	}
}

func TestBlockCommentStart(t *testing.T) {
	s := &Scanner{}
	lexer := makeLexer("--[[comment]]")
	valid := onlyValid(BlockCommentStart)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for --[[ block comment start")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockCommentStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockCommentStart)
	}
	if s.levelCount != 0 {
		t.Errorf("levelCount = %d, want 0", s.levelCount)
	}
}

func TestBlockCommentStartWithLevel(t *testing.T) {
	s := &Scanner{}
	lexer := makeLexer("--[=[comment]=]")
	valid := onlyValid(BlockCommentStart)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for --[=[ block comment start")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockCommentStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockCommentStart)
	}
	if s.levelCount != 1 {
		t.Errorf("levelCount = %d, want 1", s.levelCount)
	}
}

func TestBlockCommentContent(t *testing.T) {
	s := &Scanner{endingChar: 0, levelCount: 0}
	lexer := makeLexer("comment text]]")
	valid := onlyValid(BlockCommentContent)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for block comment content")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockCommentContent) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockCommentContent)
	}
}

func TestBlockCommentEnd(t *testing.T) {
	s := &Scanner{endingChar: 0, levelCount: 0}
	lexer := makeLexer("]]")
	valid := onlyValid(BlockCommentEnd)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true for ]] block comment end")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockCommentEnd) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockCommentEnd)
	}
}

func TestNoMatchReturnsFalse(t *testing.T) {
	s := &Scanner{}
	lexer := makeLexer("hello world")
	valid := onlyValid(BlockStringStart, BlockCommentStart)

	if s.Scan(lexer, valid) {
		t.Error("expected Scan to return false for non-matching input")
	}
}

func TestBlockStringMismatchedLevel(t *testing.T) {
	// Level 1 scanner trying to match ]] (level 0 end) — should not match
	s := &Scanner{levelCount: 1}
	lexer := makeLexer("]]")
	valid := onlyValid(BlockStringEnd)

	if s.Scan(lexer, valid) {
		t.Error("expected Scan to return false for mismatched level")
	}
}

func TestBlockStringContentEOF(t *testing.T) {
	// Content without closing delimiter — scanner should return false
	s := &Scanner{levelCount: 0}
	lexer := makeLexer("unterminated content")
	valid := onlyValid(BlockStringContent)

	if s.Scan(lexer, valid) {
		t.Error("expected Scan to return false for unterminated content")
	}
}

func TestSkipWhitespaceBeforeBlockString(t *testing.T) {
	s := &Scanner{}
	lexer := makeLexer("   [[hello]]")
	valid := onlyValid(BlockStringStart)

	if !s.Scan(lexer, valid) {
		t.Fatal("expected Scan to return true after skipping whitespace")
	}
	if lexer.ResultSymbol != ts.Symbol(BlockStringStart) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, BlockStringStart)
	}
}
