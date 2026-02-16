package html

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

// onlyValid returns a validSymbols slice with only the specified tokens enabled.
func onlyValid(tokens ...int) []bool {
	v := make([]bool, Comment+1)
	for _, t := range tokens {
		v[t] = true
	}
	return v
}

// --- Tag tests ---

func TestTagTypeForName(t *testing.T) {
	tests := []struct {
		name string
		want TagType
	}{
		{"DIV", Div},
		{"SCRIPT", Script},
		{"STYLE", Style},
		{"BR", Br},
		{"IMG", Img},
		{"P", P},
		{"UNKNOWN", Custom},
		{"MY-COMPONENT", Custom},
	}
	for _, tt := range tests {
		got := tagTypeForName(tt.name)
		if got != tt.want {
			t.Errorf("tagTypeForName(%q) = %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestTagIsVoid(t *testing.T) {
	voidTags := []TagType{Area, Base, Br, Col, Embed, Hr, Img, Input, Link, Meta, Param, Source, Track, Wbr}
	for _, tt := range voidTags {
		tag := Tag{Type: tt}
		if !tag.isVoid() {
			t.Errorf("tag type %d should be void", tt)
		}
	}
	nonVoidTags := []TagType{Div, P, Span, Script, Style, A}
	for _, tt := range nonVoidTags {
		tag := Tag{Type: tt}
		if tag.isVoid() {
			t.Errorf("tag type %d should not be void", tt)
		}
	}
}

func TestTagEq(t *testing.T) {
	div1 := Tag{Type: Div}
	div2 := Tag{Type: Div}
	span := Tag{Type: Span}
	custom1 := Tag{Type: Custom, CustomTagName: "MY-TAG"}
	custom2 := Tag{Type: Custom, CustomTagName: "MY-TAG"}
	custom3 := Tag{Type: Custom, CustomTagName: "OTHER-TAG"}

	if !div1.eq(&div2) {
		t.Error("same type tags should be equal")
	}
	if div1.eq(&span) {
		t.Error("different type tags should not be equal")
	}
	if !custom1.eq(&custom2) {
		t.Error("custom tags with same name should be equal")
	}
	if custom1.eq(&custom3) {
		t.Error("custom tags with different names should not be equal")
	}
}

func TestTagCanContain(t *testing.T) {
	li := Tag{Type: Li}
	liChild := Tag{Type: Li}
	div := Tag{Type: Div}
	p := Tag{Type: P}
	h1 := Tag{Type: H1}
	colgroup := Tag{Type: Colgroup}
	col := Tag{Type: Col}
	tr := Tag{Type: Tr}
	td := Tag{Type: Td}

	if li.canContain(&liChild) {
		t.Error("li should not contain li")
	}
	if !li.canContain(&div) {
		t.Error("li should contain div")
	}
	if p.canContain(&h1) {
		t.Error("p should not contain h1")
	}
	if !p.canContain(&Tag{Type: Span}) {
		t.Error("p should contain span")
	}
	if !colgroup.canContain(&col) {
		t.Error("colgroup should contain col")
	}
	if colgroup.canContain(&div) {
		t.Error("colgroup should not contain div")
	}
	if tr.canContain(&Tag{Type: Tr}) {
		t.Error("tr should not contain tr")
	}
	if td.canContain(&Tag{Type: Td}) {
		t.Error("td should not contain td")
	}
	if td.canContain(&Tag{Type: Tr}) {
		t.Error("td should not contain tr")
	}
}

// --- Scanner lifecycle tests ---

func TestNew(t *testing.T) {
	s := New()
	if s == nil {
		t.Fatal("New() returned nil")
	}
}

func TestSerializeDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n != 4 {
		t.Fatalf("Serialize empty = %d bytes, want 4 (header only)", n)
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])
	if len(s2.tags) != 0 {
		t.Errorf("tags = %d, want 0", len(s2.tags))
	}
}

func TestSerializeDeserializeWithTags(t *testing.T) {
	s := New().(*Scanner)
	s.tags = []Tag{
		{Type: Div},
		{Type: P},
		{Type: Custom, CustomTagName: "MY-COMPONENT"},
		{Type: Span},
	}

	buf := make([]byte, 1024)
	n := s.Serialize(buf)
	if n == 0 {
		t.Fatal("Serialize returned 0")
	}

	s2 := New().(*Scanner)
	s2.Deserialize(buf[:n])

	if len(s2.tags) != 4 {
		t.Fatalf("tags = %d, want 4", len(s2.tags))
	}
	if s2.tags[0].Type != Div {
		t.Errorf("tags[0].Type = %d, want Div", s2.tags[0].Type)
	}
	if s2.tags[1].Type != P {
		t.Errorf("tags[1].Type = %d, want P", s2.tags[1].Type)
	}
	if s2.tags[2].Type != Custom || s2.tags[2].CustomTagName != "MY-COMPONENT" {
		t.Errorf("tags[2] = %v, want Custom MY-COMPONENT", s2.tags[2])
	}
	if s2.tags[3].Type != Span {
		t.Errorf("tags[3].Type = %d, want Span", s2.tags[3].Type)
	}
}

func TestDeserializeEmpty(t *testing.T) {
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Div}}
	s.Deserialize(nil) // empty data should reset
	if len(s.tags) != 0 {
		t.Errorf("tags = %d, want 0 after empty deserialize", len(s.tags))
	}
}

// --- Comment tests ---

func TestScanComment(t *testing.T) {
	// "<!-- hello -->" — scanner sees '<', advances, sees '!', advances,
	// then scanComment sees "-- hello -->"
	lexer := newLexer("<!-- hello -->")
	s := New().(*Scanner)
	v := onlyValid(Comment, ImplicitEndTag, StartTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected comment to be found")
	}
	if lexer.ResultSymbol != ts.Symbol(Comment) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, Comment)
	}
}

func TestScanCommentNotComment(t *testing.T) {
	// "<!DOCTYPE html>" — not a comment ('D' != '-').
	lexer := newLexer("<!DOCTYPE html>")
	s := New().(*Scanner)
	v := onlyValid(Comment, ImplicitEndTag, StartTagName)
	if s.Scan(lexer, v) {
		t.Error("expected false for <!DOCTYPE (not a comment)")
	}
}

// --- StartTagName tests ---

func TestScanStartTagNameDiv(t *testing.T) {
	lexer := newLexer("div")
	s := New().(*Scanner)
	v := onlyValid(StartTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected start tag name")
	}
	if lexer.ResultSymbol != ts.Symbol(StartTagName) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StartTagName)
	}
	if len(s.tags) != 1 || s.tags[0].Type != Div {
		t.Errorf("tags = %v, want [Div]", s.tags)
	}
}

func TestScanStartTagNameScript(t *testing.T) {
	lexer := newLexer("script")
	s := New().(*Scanner)
	v := onlyValid(StartTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected script start tag name")
	}
	if lexer.ResultSymbol != ts.Symbol(ScriptStartTagName) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ScriptStartTagName)
	}
}

func TestScanStartTagNameStyle(t *testing.T) {
	lexer := newLexer("style")
	s := New().(*Scanner)
	v := onlyValid(StartTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected style start tag name")
	}
	if lexer.ResultSymbol != ts.Symbol(StyleStartTagName) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, StyleStartTagName)
	}
}

func TestScanStartTagNameEmpty(t *testing.T) {
	lexer := newLexer("")
	s := New().(*Scanner)
	v := onlyValid(StartTagName)
	if s.Scan(lexer, v) {
		t.Error("expected false for empty input with only StartTagName")
	}
}

// --- EndTagName tests ---

func TestScanEndTagNameMatching(t *testing.T) {
	lexer := newLexer("div")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Div}}
	v := onlyValid(EndTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected end tag name")
	}
	if lexer.ResultSymbol != ts.Symbol(EndTagName) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, EndTagName)
	}
	if len(s.tags) != 0 {
		t.Errorf("tags should be empty after matching end tag, got %v", s.tags)
	}
}

func TestScanEndTagNameErroneous(t *testing.T) {
	lexer := newLexer("span")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Div}}
	v := onlyValid(EndTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected erroneous end tag name")
	}
	if lexer.ResultSymbol != ts.Symbol(ErroneousEndTagName) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ErroneousEndTagName)
	}
	if len(s.tags) != 1 {
		t.Errorf("tags should still have 1 element, got %d", len(s.tags))
	}
}

// --- SelfClosingTagDelimiter tests ---

func TestScanSelfClosingTagDelimiter(t *testing.T) {
	lexer := newLexer("/>")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Img}}
	v := onlyValid(SelfClosingTagDelimiter)
	if !s.Scan(lexer, v) {
		t.Fatal("expected self-closing tag delimiter")
	}
	if lexer.ResultSymbol != ts.Symbol(SelfClosingTagDelimiter) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, SelfClosingTagDelimiter)
	}
	if len(s.tags) != 0 {
		t.Errorf("tags should be empty after self-closing, got %v", s.tags)
	}
}

func TestScanSelfClosingNotComplete(t *testing.T) {
	lexer := newLexer("/x")
	s := New().(*Scanner)
	v := onlyValid(SelfClosingTagDelimiter)
	if s.Scan(lexer, v) {
		t.Error("expected false for /x (not />)")
	}
}

// --- ImplicitEndTag tests ---

func TestImplicitEndTagVoidElement(t *testing.T) {
	lexer := newLexer("<div")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Br}}
	v := onlyValid(ImplicitEndTag, StartTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected implicit end tag for void element")
	}
	if lexer.ResultSymbol != ts.Symbol(ImplicitEndTag) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ImplicitEndTag)
	}
	if len(s.tags) != 0 {
		t.Errorf("tags should be empty after void element auto-close, got %v", s.tags)
	}
}

func TestImplicitEndTagCantContain(t *testing.T) {
	lexer := newLexer("<div")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: P}}
	v := onlyValid(ImplicitEndTag, StartTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected implicit end tag for p when div opens")
	}
	if lexer.ResultSymbol != ts.Symbol(ImplicitEndTag) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ImplicitEndTag)
	}
}

func TestImplicitEndTagClosingTagDeep(t *testing.T) {
	lexer := newLexer("</div")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: HTML}, {Type: Body}, {Type: Div}, {Type: P}}
	v := onlyValid(ImplicitEndTag, StartTagName, EndTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected implicit end tag for deep closing")
	}
	if lexer.ResultSymbol != ts.Symbol(ImplicitEndTag) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, ImplicitEndTag)
	}
	if len(s.tags) != 3 {
		t.Errorf("tags = %d, want 3", len(s.tags))
	}
}

func TestImplicitEndTagClosingMatchesTop(t *testing.T) {
	lexer := newLexer("</div")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: HTML}, {Type: Body}, {Type: Div}}
	v := onlyValid(ImplicitEndTag, StartTagName, EndTagName)
	if s.Scan(lexer, v) {
		t.Error("expected false when closing tag matches top of stack")
	}
}

func TestImplicitEndTagAtEOFHTML(t *testing.T) {
	lexer := newLexer("")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: HTML}}
	v := onlyValid(ImplicitEndTag, StartTagName)
	if !s.Scan(lexer, v) {
		t.Fatal("expected implicit end tag at EOF for html element")
	}
}

// --- RawText tests ---

func TestScanRawTextScript(t *testing.T) {
	lexer := newLexer("var x = 1;</script>")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Script}}
	v := onlyValid(RawText)
	if !s.Scan(lexer, v) {
		t.Fatal("expected raw text for script")
	}
	if lexer.ResultSymbol != ts.Symbol(RawText) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RawText)
	}
}

func TestScanRawTextStyle(t *testing.T) {
	lexer := newLexer("body { color: red; }</style>")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Style}}
	v := onlyValid(RawText)
	if !s.Scan(lexer, v) {
		t.Fatal("expected raw text for style")
	}
	if lexer.ResultSymbol != ts.Symbol(RawText) {
		t.Errorf("ResultSymbol = %d, want %d", lexer.ResultSymbol, RawText)
	}
}

func TestScanRawTextEmptyStack(t *testing.T) {
	lexer := newLexer("some text</script>")
	s := New().(*Scanner)
	v := onlyValid(RawText)
	if s.Scan(lexer, v) {
		t.Error("expected false for raw text with empty tag stack")
	}
}

func TestScanRawTextCaseInsensitive(t *testing.T) {
	lexer := newLexer("code</Script>")
	s := New().(*Scanner)
	s.tags = []Tag{{Type: Script}}
	v := onlyValid(RawText)
	if !s.Scan(lexer, v) {
		t.Fatal("expected raw text with case-insensitive close tag")
	}
}

// --- Short validSymbols ---

func TestScanShortValidSymbols(t *testing.T) {
	lexer := newLexer("<div>")
	s := New()
	if s.Scan(lexer, []bool{true}) {
		t.Error("expected false for too-short validSymbols")
	}
}

// --- Helper function tests ---

func TestIsAlnumOrDash(t *testing.T) {
	tests := []struct {
		ch   int32
		want bool
	}{
		{'a', true}, {'Z', true}, {'0', true}, {'-', true}, {':', true},
		{' ', false}, {'<', false}, {'>', false}, {-1, false},
	}
	for _, tt := range tests {
		got := isAlnumOrDash(tt.ch)
		if got != tt.want {
			t.Errorf("isAlnumOrDash(%d) = %v, want %v", tt.ch, got, tt.want)
		}
	}
}

func TestToUpper(t *testing.T) {
	if toUpper('a') != 'A' {
		t.Errorf("toUpper('a') = %c, want 'A'", toUpper('a'))
	}
	if toUpper('Z') != 'Z' {
		t.Errorf("toUpper('Z') = %c, want 'Z'", toUpper('Z'))
	}
	if toUpper('5') != '5' {
		t.Errorf("toUpper('5') = %c, want '5'", toUpper('5'))
	}
}
