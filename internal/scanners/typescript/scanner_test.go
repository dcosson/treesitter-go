package typescript

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
}

func TestSerializeStateless(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 16)
	n := s.Serialize(buf)
	if n != 0 {
		t.Errorf("Serialize = %d, want 0 (stateless)", n)
	}
}

func TestDeserializeStateless(t *testing.T) {
	s := New().(*Scanner)
	// Should not panic
	s.Deserialize(nil)
	s.Deserialize([]byte{1, 2, 3})
}

func TestScanTemplateChars(t *testing.T) {
	lexer := newLexerForString("hello world`")
	s := New().(*Scanner)
	v := onlyValid(TemplateChars)
	if !s.Scan(lexer, v) {
		t.Fatal("expected TemplateChars to succeed")
	}
	if lexer.ResultSymbol != TemplateChars {
		t.Errorf("ResultSymbol = %d, want %d (TemplateChars)", lexer.ResultSymbol, TemplateChars)
	}
}

func TestScanTemplateCharsInterpolation(t *testing.T) {
	lexer := newLexerForString("hello ${")
	s := New().(*Scanner)
	v := onlyValid(TemplateChars)
	if !s.Scan(lexer, v) {
		t.Fatal("expected TemplateChars to succeed before interpolation")
	}
}

func TestScanTemplateCharsEscape(t *testing.T) {
	lexer := newLexerForString(`hello\n`)
	s := New().(*Scanner)
	v := onlyValid(TemplateChars)
	if !s.Scan(lexer, v) {
		t.Fatal("expected TemplateChars to succeed before escape")
	}
}

func TestScanTemplateCharsEmpty(t *testing.T) {
	lexer := newLexerForString("`")
	s := New().(*Scanner)
	v := onlyValid(TemplateChars)
	if s.Scan(lexer, v) {
		t.Error("expected false for empty template chars")
	}
}

func TestScanTemplateCharsRejectsASI(t *testing.T) {
	lexer := newLexerForString("hello`")
	s := New().(*Scanner)
	v := onlyValid(TemplateChars, AutomaticSemicolon)
	if s.Scan(lexer, v) {
		t.Error("expected false when both TemplateChars and ASI are valid")
	}
}

func TestScanAutomaticSemicolonEOF(t *testing.T) {
	lexer := newLexerForString("")
	s := New().(*Scanner)
	v := onlyValid(AutomaticSemicolon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ASI at EOF")
	}
	if lexer.ResultSymbol != AutomaticSemicolon {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, AutomaticSemicolon)
	}
}

func TestScanAutomaticSemicolonCloseBrace(t *testing.T) {
	lexer := newLexerForString("}")
	s := New().(*Scanner)
	v := onlyValid(AutomaticSemicolon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ASI before }")
	}
}

func TestScanAutomaticSemicolonNewline(t *testing.T) {
	lexer := newLexerForString("\nx")
	s := New().(*Scanner)
	v := onlyValid(AutomaticSemicolon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ASI after newline")
	}
}

func TestScanAutomaticSemicolonRejectsOperator(t *testing.T) {
	lexer := newLexerForString("\n+x")
	s := New().(*Scanner)
	v := onlyValid(AutomaticSemicolon)
	// + (not ++) should NOT trigger ASI
	if s.Scan(lexer, v) {
		t.Error("expected no ASI before binary +")
	}
}

func TestScanAutomaticSemicolonIncrement(t *testing.T) {
	lexer := newLexerForString("\n++x")
	s := New().(*Scanner)
	v := onlyValid(AutomaticSemicolon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ASI before ++")
	}
}

func TestScanAutomaticSemicolonNotEquals(t *testing.T) {
	lexer := newLexerForString("\n!=")
	s := New().(*Scanner)
	v := onlyValid(AutomaticSemicolon)
	// != should NOT trigger ASI
	if s.Scan(lexer, v) {
		t.Error("expected no ASI before !=")
	}
}

func TestScanAutomaticSemicolonBang(t *testing.T) {
	lexer := newLexerForString("\n!x")
	s := New().(*Scanner)
	v := onlyValid(AutomaticSemicolon)
	if !s.Scan(lexer, v) {
		t.Fatal("expected ASI before unary !")
	}
}

func TestScanTernaryQmark(t *testing.T) {
	lexer := newLexerForString("?x")
	s := New().(*Scanner)
	v := onlyValid(TernaryQmark)
	if !s.Scan(lexer, v) {
		t.Fatal("expected TernaryQmark to succeed")
	}
	if lexer.ResultSymbol != TernaryQmark {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, TernaryQmark)
	}
}

func TestScanTernaryQmarkOptionalChaining(t *testing.T) {
	lexer := newLexerForString("?.x")
	s := New().(*Scanner)
	v := onlyValid(TernaryQmark)
	if s.Scan(lexer, v) {
		t.Error("expected false for optional chaining ?.")
	}
}

func TestScanTernaryQmarkNullishCoalescing(t *testing.T) {
	lexer := newLexerForString("??x")
	s := New().(*Scanner)
	v := onlyValid(TernaryQmark)
	if s.Scan(lexer, v) {
		t.Error("expected false for nullish coalescing ??")
	}
}

func TestScanTernaryQmarkOptionalParam(t *testing.T) {
	lexer := newLexerForString("?:")
	s := New().(*Scanner)
	v := onlyValid(TernaryQmark)
	if s.Scan(lexer, v) {
		t.Error("expected false for optional parameter ?:")
	}
}

func TestScanHTMLCommentOpen(t *testing.T) {
	lexer := newLexerForString("<!-- comment")
	s := New().(*Scanner)
	v := onlyValid(HTMLComment)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HTMLComment to succeed for <!--")
	}
	if lexer.ResultSymbol != HTMLComment {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, HTMLComment)
	}
}

func TestScanHTMLCommentClose(t *testing.T) {
	lexer := newLexerForString("--> comment")
	s := New().(*Scanner)
	v := onlyValid(HTMLComment)
	if !s.Scan(lexer, v) {
		t.Fatal("expected HTMLComment to succeed for -->")
	}
}

func TestScanHTMLCommentRejectsContext(t *testing.T) {
	lexer := newLexerForString("<!-- comment")
	s := New().(*Scanner)
	v := onlyValid(HTMLComment, LogicalOr)
	if s.Scan(lexer, v) {
		t.Error("expected false when LogicalOr is valid")
	}
}

func TestScanJSXText(t *testing.T) {
	lexer := newLexerForString("hello world<")
	s := New().(*Scanner)
	v := onlyValid(JSXText)
	if !s.Scan(lexer, v) {
		t.Fatal("expected JSXText to succeed")
	}
	if lexer.ResultSymbol != JSXText {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, JSXText)
	}
}

func TestScanJSXTextWhitespaceOnly(t *testing.T) {
	lexer := newLexerForString("\n  \n  <")
	s := New().(*Scanner)
	v := onlyValid(JSXText)
	if s.Scan(lexer, v) {
		t.Error("expected false for whitespace-only JSX text")
	}
}

func TestScanJSXTextWithNewlines(t *testing.T) {
	lexer := newLexerForString("\nhello\n<")
	s := New().(*Scanner)
	v := onlyValid(JSXText)
	if !s.Scan(lexer, v) {
		t.Fatal("expected JSXText to succeed with newlines and text")
	}
}
