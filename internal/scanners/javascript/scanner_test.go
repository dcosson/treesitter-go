package javascript

import (
	"testing"

	ts "github.com/dcosson/treesitter-go"
)

// helper creates a lexer from a string and starts it.
func makeLexer(input string) *ts.Lexer {
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(input)))
	lexer.Start(ts.LengthZero)
	return lexer
}

// makeLexerForASI creates a lexer for ASI tests. It prefixes the input with
// a space and advances past it to clear the atIncludedRangeStart flag, which
// would otherwise be true at position 0 and interfere with ASI logic.
func makeLexerForASI(input string) *ts.Lexer {
	prefixed := " " + input
	lexer := ts.NewLexer()
	lexer.SetInput(ts.NewStringInput([]byte(prefixed)))
	lexer.Start(ts.LengthZero)
	lexer.Skip() // advance past the space, clearing atIncludedRangeStart
	return lexer
}

// validOnly returns a validSymbols slice with only the specified token enabled.
func validOnly(tokens ...int) []bool {
	vs := make([]bool, 8)
	for _, t := range tokens {
		vs[t] = true
	}
	return vs
}

// --- Scanner lifecycle tests ---

func TestNewScanner(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("expected non-nil scanner")
	}
}

func TestSerializeDeserialize(t *testing.T) {
	s := New().(*Scanner)

	// Serialize returns 0 (stateless).
	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n != 0 {
		t.Errorf("expected 0 bytes serialized, got %d", n)
	}

	// Deserialize is a no-op.
	s.Deserialize(nil)
	s.Deserialize([]byte{1, 2, 3})
	// No panic = success.
}

// --- Template chars tests ---

func TestScanTemplateCharsSimple(t *testing.T) {
	lexer := makeLexer("hello world`")
	found := scanTemplateChars(lexer)
	if !found {
		t.Fatal("expected template chars to be found")
	}
	if lexer.ResultSymbol != ts.Symbol(TemplateChars) {
		t.Errorf("expected symbol %d, got %d", TemplateChars, lexer.ResultSymbol)
	}
}

func TestScanTemplateCharsWithExpression(t *testing.T) {
	lexer := makeLexer("prefix${expr}")
	found := scanTemplateChars(lexer)
	if !found {
		t.Fatal("expected template chars to be found")
	}
}

func TestScanTemplateCharsEmpty(t *testing.T) {
	lexer := makeLexer("`")
	found := scanTemplateChars(lexer)
	if found {
		t.Error("expected empty template chars to return false")
	}
}

func TestScanTemplateCharsWithEscape(t *testing.T) {
	lexer := makeLexer("text\\n")
	found := scanTemplateChars(lexer)
	if !found {
		t.Error("expected template chars before escape to be found")
	}
}

func TestScanTemplateCharsEOF(t *testing.T) {
	lexer := makeLexer("")
	found := scanTemplateChars(lexer)
	if found {
		t.Error("expected false at EOF")
	}
}

func TestScanTemplateCharsDollarNotBrace(t *testing.T) {
	// $ not followed by { should be included in content.
	lexer := makeLexer("$x`")
	found := scanTemplateChars(lexer)
	if !found {
		t.Fatal("expected template chars to be found")
	}
}

// --- Automatic semicolon tests ---

func TestScanAutoSemicolonAtEOF(t *testing.T) {
	lexer := makeLexer("")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI at EOF")
	}
}

func TestScanAutoSemicolonBeforeCloseBrace(t *testing.T) {
	lexer := makeLexer("}")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI before }")
	}
}

func TestScanAutoSemicolonAfterNewline(t *testing.T) {
	lexer := makeLexerForASI("\nfoo")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI after newline before identifier")
	}
}

func TestScanAutoSemicolonNoNewline(t *testing.T) {
	lexer := makeLexerForASI("foo")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if found {
		t.Error("expected no ASI before identifier without newline")
	}
}

func TestScanAutoSemicolonNotBeforeComma(t *testing.T) {
	lexer := makeLexerForASI("\n,")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if found {
		t.Error("expected no ASI before comma")
	}
}

func TestScanAutoSemicolonNotBeforePlus(t *testing.T) {
	lexer := makeLexerForASI("\n+")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if found {
		t.Error("expected no ASI before binary +")
	}
}

func TestScanAutoSemicolonBeforePlusPlus(t *testing.T) {
	lexer := makeLexerForASI("\n++")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI before ++")
	}
}

func TestScanAutoSemicolonBeforeMinusMinus(t *testing.T) {
	lexer := makeLexerForASI("\n--")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI before --")
	}
}

func TestScanAutoSemicolonNotBeforeIn(t *testing.T) {
	lexer := makeLexerForASI("\nin ")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if found {
		t.Error("expected no ASI before 'in'")
	}
}

func TestScanAutoSemicolonNotBeforeInstanceof(t *testing.T) {
	lexer := makeLexerForASI("\ninstanceof ")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if found {
		t.Error("expected no ASI before 'instanceof'")
	}
}

func TestScanAutoSemicolonBeforeOtherIdStartingWithI(t *testing.T) {
	lexer := makeLexerForASI("\nif")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI before 'if' (not 'in' or 'instanceof')")
	}
}

func TestScanAutoSemicolonBeforeExclamation(t *testing.T) {
	lexer := makeLexerForASI("\n!")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI before unary !")
	}
}

func TestScanAutoSemicolonNotBeforeNotEqual(t *testing.T) {
	lexer := makeLexerForASI("\n!=")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if found {
		t.Error("expected no ASI before !=")
	}
}

func TestScanAutoSemicolonBeforeDecimalLiteral(t *testing.T) {
	lexer := makeLexerForASI("\n.5")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if !found {
		t.Error("expected ASI before decimal literal .5")
	}
}

func TestScanAutoSemicolonNotBeforeDot(t *testing.T) {
	lexer := makeLexerForASI("\n.foo")
	scanned := false
	found := scanAutomaticSemicolon(lexer, true, &scanned)
	if found {
		t.Error("expected no ASI before .foo (property access)")
	}
}

// --- Ternary qmark tests ---

func TestScanTernaryQmarkSimple(t *testing.T) {
	lexer := makeLexer("? :")
	found := scanTernaryQmark(lexer)
	if !found {
		t.Error("expected ternary qmark")
	}
	if lexer.ResultSymbol != ts.Symbol(TernaryQmark) {
		t.Errorf("expected symbol %d, got %d", TernaryQmark, lexer.ResultSymbol)
	}
}

func TestScanTernaryQmarkNullishCoalescing(t *testing.T) {
	lexer := makeLexer("??")
	found := scanTernaryQmark(lexer)
	if found {
		t.Error("expected false for ?? (nullish coalescing)")
	}
}

func TestScanTernaryQmarkOptionalChaining(t *testing.T) {
	lexer := makeLexer("?.foo")
	found := scanTernaryQmark(lexer)
	if found {
		t.Error("expected false for ?.foo (optional chaining)")
	}
}

func TestScanTernaryQmarkBeforeDecimal(t *testing.T) {
	// ?.5 should be ternary qmark (not optional chaining)
	// because .5 is a numeric literal.
	lexer := makeLexer("?.5")
	found := scanTernaryQmark(lexer)
	if !found {
		t.Error("expected true for ?.5 (ternary before decimal literal)")
	}
}

func TestScanTernaryQmarkNotPresent(t *testing.T) {
	lexer := makeLexer("foo")
	found := scanTernaryQmark(lexer)
	if found {
		t.Error("expected false when no ? present")
	}
}

func TestScanTernaryQmarkWithWhitespace(t *testing.T) {
	lexer := makeLexer("  ? :")
	found := scanTernaryQmark(lexer)
	if !found {
		t.Error("expected ternary qmark with leading whitespace")
	}
}

// --- HTML comment tests ---

func TestScanHTMLCommentOpen(t *testing.T) {
	lexer := makeLexer("<!-- comment text")
	found := scanHTMLComment(lexer)
	if !found {
		t.Error("expected HTML comment <!-- to be found")
	}
	if lexer.ResultSymbol != ts.Symbol(HTMLComment) {
		t.Errorf("expected symbol %d, got %d", HTMLComment, lexer.ResultSymbol)
	}
}

func TestScanHTMLCommentClose(t *testing.T) {
	lexer := makeLexer("--> comment text")
	found := scanHTMLComment(lexer)
	if !found {
		t.Error("expected HTML comment --> to be found")
	}
}

func TestScanHTMLCommentPartial(t *testing.T) {
	lexer := makeLexer("<!-x")
	found := scanHTMLComment(lexer)
	if found {
		t.Error("expected false for partial <!-x")
	}
}

func TestScanHTMLCommentNonComment(t *testing.T) {
	lexer := makeLexer("foo")
	found := scanHTMLComment(lexer)
	if found {
		t.Error("expected false for non-comment")
	}
}

// --- JSX text tests ---

func TestScanJSXTextSimple(t *testing.T) {
	lexer := makeLexer("hello world<")
	found := scanJSXText(lexer)
	if !found {
		t.Error("expected JSX text to be found")
	}
	if lexer.ResultSymbol != ts.Symbol(JSXText) {
		t.Errorf("expected symbol %d, got %d", JSXText, lexer.ResultSymbol)
	}
}

func TestScanJSXTextWhitespaceOnly(t *testing.T) {
	lexer := makeLexer("   <")
	found := scanJSXText(lexer)
	if !found {
		t.Error("expected JSX text with whitespace to be found (spaces are significant)")
	}
}

func TestScanJSXTextNewlineOnly(t *testing.T) {
	lexer := makeLexer("\n<")
	found := scanJSXText(lexer)
	if found {
		t.Error("expected false for newline-only JSX text")
	}
}

func TestScanJSXTextNewlineThenWhitespace(t *testing.T) {
	lexer := makeLexer("\n  <")
	found := scanJSXText(lexer)
	if found {
		t.Error("expected false for newline-then-whitespace JSX text")
	}
}

func TestScanJSXTextNewlineThenContent(t *testing.T) {
	lexer := makeLexer("\nfoo<")
	found := scanJSXText(lexer)
	if !found {
		t.Error("expected JSX text with content after newline")
	}
}

func TestScanJSXTextStopsAtBrace(t *testing.T) {
	lexer := makeLexer("text{")
	found := scanJSXText(lexer)
	if !found {
		t.Error("expected JSX text before {")
	}
}

func TestScanJSXTextStopsAtAmpersand(t *testing.T) {
	lexer := makeLexer("text&amp;")
	found := scanJSXText(lexer)
	if !found {
		t.Error("expected JSX text before &")
	}
}

func TestScanJSXTextEmpty(t *testing.T) {
	lexer := makeLexer("<")
	found := scanJSXText(lexer)
	if found {
		t.Error("expected false for empty JSX text")
	}
}

// --- Dispatcher (Scan) tests ---

func TestScanDispatchTemplateChars(t *testing.T) {
	s := New()
	lexer := makeLexer("hello`")
	found := s.Scan(lexer, validOnly(TemplateChars))
	if !found {
		t.Error("expected scan to dispatch to template chars")
	}
}

func TestScanDispatchTemplateCharsBlockedByASI(t *testing.T) {
	// When both TemplateChars and AutomaticSemicolon are valid, return false.
	s := New()
	lexer := makeLexer("hello`")
	found := s.Scan(lexer, validOnly(TemplateChars, AutomaticSemicolon))
	if found {
		t.Error("expected false when both template_chars and auto_semicolon valid")
	}
}

func TestScanDispatchASI(t *testing.T) {
	s := New()
	lexer := makeLexer("")
	found := s.Scan(lexer, validOnly(AutomaticSemicolon))
	if !found {
		t.Error("expected ASI at EOF")
	}
}

func TestScanDispatchTernaryQmark(t *testing.T) {
	s := New()
	lexer := makeLexer("? :")
	found := s.Scan(lexer, validOnly(TernaryQmark))
	if !found {
		t.Error("expected ternary qmark dispatch")
	}
}

func TestScanDispatchHTMLComment(t *testing.T) {
	s := New()
	lexer := makeLexer("<!-- comment")
	found := s.Scan(lexer, validOnly(HTMLComment))
	if !found {
		t.Error("expected HTML comment dispatch")
	}
}

func TestScanDispatchHTMLCommentBlockedByLogicalOr(t *testing.T) {
	s := New()
	lexer := makeLexer("<!-- comment")
	found := s.Scan(lexer, validOnly(HTMLComment, LogicalOr))
	if found {
		t.Error("expected false when logical_or is also valid")
	}
}

func TestScanDispatchJSXText(t *testing.T) {
	s := New()
	lexer := makeLexer("hello<")
	found := s.Scan(lexer, validOnly(JSXText))
	if !found {
		t.Error("expected JSX text dispatch")
	}
}

func TestScanDispatchNoMatch(t *testing.T) {
	s := New()
	lexer := makeLexer("hello")
	found := s.Scan(lexer, validOnly(LogicalOr))
	if found {
		t.Error("expected false for only logical_or valid")
	}
}

func TestScanDispatchShortValidSymbols(t *testing.T) {
	s := New()
	lexer := makeLexer("hello")
	found := s.Scan(lexer, []bool{true})
	if found {
		t.Error("expected false for too-short valid symbols")
	}
}

// --- Helper function tests ---

func TestIsWhitespace(t *testing.T) {
	if !isWhitespace(' ') {
		t.Error("space should be whitespace")
	}
	if !isWhitespace('\t') {
		t.Error("tab should be whitespace")
	}
	if !isWhitespace('\n') {
		t.Error("newline should be whitespace")
	}
	if isWhitespace('a') {
		t.Error("'a' should not be whitespace")
	}
	// Unicode non-breaking space.
	if !isWhitespace(0x00A0) {
		t.Error("NBSP should be whitespace")
	}
}

func TestIsAlpha(t *testing.T) {
	if !isAlpha('a') {
		t.Error("'a' should be alpha")
	}
	if !isAlpha('Z') {
		t.Error("'Z' should be alpha")
	}
	if !isAlpha('_') {
		t.Error("'_' should be alpha")
	}
	if isAlpha('0') {
		t.Error("'0' should not be alpha")
	}
}

func TestIsDigit(t *testing.T) {
	if !isDigit('0') {
		t.Error("'0' should be digit")
	}
	if !isDigit('9') {
		t.Error("'9' should be digit")
	}
	if isDigit('a') {
		t.Error("'a' should not be digit")
	}
}
