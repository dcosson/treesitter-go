package css

import (
	"testing"

	ts "github.com/treesitter-go/treesitter"
)

// newLexer creates a Lexer initialized with the given string input.
func newLexer(s string) *ts.Lexer {
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(s)))
	lexer.Start(ts.Length{})
	return lexer
}

// allValid returns a validSymbols slice with all non-error-recovery tokens enabled.
func allValid() []bool {
	v := make([]bool, ErrorRecovery+1)
	for i := range v {
		v[i] = true
	}
	v[ErrorRecovery] = false
	return v
}

// onlyValid returns a validSymbols slice with only the specified tokens enabled.
func onlyValid(tokens ...int) []bool {
	v := make([]bool, ErrorRecovery+1)
	for _, t := range tokens {
		v[t] = true
	}
	return v
}

// --- Scanner lifecycle tests ---

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestSerializeDeserialize(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n != 0 {
		t.Errorf("Serialize = %d, want 0 (stateless scanner)", n)
	}
	// Deserialize is a no-op — just ensure no panic.
	s.Deserialize(nil)
	s.Deserialize([]byte{1, 2, 3})
}

// --- ErrorRecovery guard ---

func TestScanErrorRecoveryDisables(t *testing.T) {
	lexer := newLexer(" div")
	s := New()
	v := allValid()
	v[ErrorRecovery] = true
	if s.Scan(lexer, v) {
		t.Error("expected false during error recovery")
	}
}

func TestScanShortValidSymbols(t *testing.T) {
	lexer := newLexer(" div")
	s := New()
	if s.Scan(lexer, []bool{true}) {
		t.Error("expected false for too-short validSymbols")
	}
}

// --- DescendantOp tests ---

func TestDescendantOpSimpleSelector(t *testing.T) {
	// "div p" — whitespace between two selectors should produce DescendantOp.
	// The lexer is positioned at the space: " p"
	lexer := newLexer(" p")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp to succeed")
	}
	if lexer.ResultSymbol != ts.Symbol(DescendantOp) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, DescendantOp)
	}
}

func TestDescendantOpHash(t *testing.T) {
	lexer := newLexer(" #id")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp for # selector")
	}
}

func TestDescendantOpDot(t *testing.T) {
	lexer := newLexer(" .class")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp for . selector")
	}
}

func TestDescendantOpBracket(t *testing.T) {
	lexer := newLexer(" [attr]")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp for [ selector")
	}
}

func TestDescendantOpDash(t *testing.T) {
	lexer := newLexer(" -webkit-thing")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp for - selector")
	}
}

func TestDescendantOpStar(t *testing.T) {
	lexer := newLexer(" *")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp for * selector")
	}
}

func TestDescendantOpMultipleSpaces(t *testing.T) {
	// Multiple spaces should be consumed, still producing DescendantOp.
	lexer := newLexer("   div")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp with multiple spaces")
	}
}

func TestDescendantOpColonSelectorContext(t *testing.T) {
	// " :hover { ... }" — colon followed by '{' means selector context.
	lexer := newLexer(" :hover{")
	s := New()
	v := onlyValid(DescendantOp)
	if !s.Scan(lexer, v) {
		t.Fatal("expected DescendantOp for :hover in selector context (has {)")
	}
}

func TestDescendantOpColonPropertyContext(t *testing.T) {
	// " : red;" — colon followed by ';' means property context, not descendant.
	lexer := newLexer(" :red;")
	s := New()
	v := onlyValid(DescendantOp)
	if s.Scan(lexer, v) {
		t.Error("expected false for colon in property context (has ;)")
	}
}

func TestDescendantOpColonPropertyCloseBrace(t *testing.T) {
	// " : red}" — '}' also indicates property context.
	lexer := newLexer(" :red}")
	s := New()
	v := onlyValid(DescendantOp)
	if s.Scan(lexer, v) {
		t.Error("expected false for colon in property context (has })")
	}
}

func TestDescendantOpColonFollowedBySpace(t *testing.T) {
	// " : " — colon followed by space means property declaration.
	lexer := newLexer(" : red")
	s := New()
	v := onlyValid(DescendantOp)
	if s.Scan(lexer, v) {
		t.Error("expected false for colon followed by space (property declaration)")
	}
}

func TestDescendantOpNoWhitespace(t *testing.T) {
	// No leading whitespace — should not produce DescendantOp.
	lexer := newLexer("div")
	s := New()
	v := onlyValid(DescendantOp)
	if s.Scan(lexer, v) {
		t.Error("expected false without leading whitespace")
	}
}

func TestDescendantOpNotValid(t *testing.T) {
	// DescendantOp not in valid set.
	lexer := newLexer(" div")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	// Skip whitespace — the PseudoClassSelectorColon path skips spaces.
	// But since the next char is 'd' (not ':'), it shouldn't match either.
	if s.Scan(lexer, v) {
		t.Error("expected false when DescendantOp not valid and next char is not ':'")
	}
}

func TestDescendantOpFollowedByNonSelector(t *testing.T) {
	// " ;" — semicolon is not a valid selector start.
	lexer := newLexer(" ;")
	s := New()
	v := onlyValid(DescendantOp)
	if s.Scan(lexer, v) {
		t.Error("expected false when next char after space is ';'")
	}
}

// --- PseudoClassSelectorColon tests ---

func TestPseudoClassSelectorColonHover(t *testing.T) {
	// ":hover {" — selector context, should produce PseudoClassSelectorColon.
	lexer := newLexer(":hover{")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected PseudoClassSelectorColon for :hover")
	}
	if lexer.ResultSymbol != ts.Symbol(PseudoClassSelectorColon) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, PseudoClassSelectorColon)
	}
}

func TestPseudoClassSelectorColonWithLeadingSpaces(t *testing.T) {
	lexer := newLexer("  :hover{")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected PseudoClassSelectorColon with leading spaces")
	}
}

func TestPseudoClassSelectorColonProperty(t *testing.T) {
	// ":red;" — ';' means property context, not pseudo-class.
	lexer := newLexer(":red;")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	if s.Scan(lexer, v) {
		t.Error("expected false for property context (has ;)")
	}
}

func TestPseudoClassSelectorColonPropertyCloseBrace(t *testing.T) {
	// ":red}" — '}' means property context.
	lexer := newLexer(":red}")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	if s.Scan(lexer, v) {
		t.Error("expected false for property context (has })")
	}
}

func TestPseudoClassSelectorColonDoubleColon(t *testing.T) {
	// "::before" — pseudo-element, not pseudo-class.
	lexer := newLexer("::before{")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	if s.Scan(lexer, v) {
		t.Error("expected false for pseudo-element ::")
	}
}

func TestPseudoClassSelectorColonAtEOF(t *testing.T) {
	// ":hover" then EOF — should return true for better error recovery.
	lexer := newLexer(":hover")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	if !s.Scan(lexer, v) {
		t.Error("expected true for :hover at EOF (error recovery)")
	}
}

func TestPseudoClassSelectorColonNoColon(t *testing.T) {
	// No colon — should return false.
	lexer := newLexer("div")
	s := New()
	v := onlyValid(PseudoClassSelectorColon)
	if s.Scan(lexer, v) {
		t.Error("expected false when no colon present")
	}
}

// --- Both tokens valid ---

func TestBothValidDescendantAndPseudo(t *testing.T) {
	// " :hover{" — should match DescendantOp (checked first).
	lexer := newLexer(" :hover{")
	s := New()
	v := allValid()
	if !s.Scan(lexer, v) {
		t.Fatal("expected scan to succeed with both tokens valid")
	}
	if lexer.ResultSymbol != ts.Symbol(DescendantOp) {
		t.Errorf("ResultSymbol = %d, want %d (DescendantOp checked first)", lexer.ResultSymbol, DescendantOp)
	}
}

// --- Helper function tests ---

func TestIsSpace(t *testing.T) {
	tests := []struct {
		ch   int32
		want bool
	}{
		{' ', true},
		{'\t', true},
		{'\n', true},
		{'\r', true},
		{'a', false},
		{'0', false},
		{-1, false}, // EOF
	}
	for _, tt := range tests {
		got := isSpace(tt.ch)
		if got != tt.want {
			t.Errorf("isSpace(%d) = %v, want %v", tt.ch, got, tt.want)
		}
	}
}

func TestIsAlnum(t *testing.T) {
	tests := []struct {
		ch   int32
		want bool
	}{
		{'a', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'-', false},
		{' ', false},
		{-1, false}, // EOF
	}
	for _, tt := range tests {
		got := isAlnum(tt.ch)
		if got != tt.want {
			t.Errorf("isAlnum(%d) = %v, want %v", tt.ch, got, tt.want)
		}
	}
}
