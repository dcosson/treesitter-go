package treesitter_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
)

// jsonLexFn is a hand-written lex function for the JSON grammar.
func jsonLexFn(lexer *ts.Lexer, state ts.StateID) bool {
	// Skip whitespace.
	for !lexer.EOF() && (lexer.Lookahead == ' ' || lexer.Lookahead == '\t' ||
		lexer.Lookahead == '\n' || lexer.Lookahead == '\r') {
		lexer.Skip()
	}

	if lexer.EOF() {
		return false
	}

	ch := lexer.Lookahead

	// String content mode (lex state 1): inside a string.
	if state == 1 {
		return jsonLexStringContent(lexer)
	}

	switch {
	case ch == '{':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymLBrace))
		return true
	case ch == '}':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymRBrace))
		return true
	case ch == '[':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymLBrack))
		return true
	case ch == ']':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymRBrack))
		return true
	case ch == ',':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymComma))
		return true
	case ch == ':':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymColon))
		return true
	case ch == '"':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymDQuote))
		return true
	case ch == 't':
		return jsonLexKW(lexer, "true", ts.Symbol(tg.SymTrue))
	case ch == 'f':
		return jsonLexKW(lexer, "false", ts.Symbol(tg.SymFalse))
	case ch == 'n':
		return jsonLexKW(lexer, "null", ts.Symbol(tg.SymNull))
	case ch == '-' || (ch >= '0' && ch <= '9'):
		return jsonLexNum(lexer)
	case ch == '/':
		return jsonLexCmt(lexer)
	}

	return false
}

func jsonLexStringContent(lexer *ts.Lexer) bool {
	if lexer.EOF() {
		return false
	}
	ch := lexer.Lookahead
	if ch == '\\' {
		lexer.Advance(false)
		if !lexer.EOF() {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymEscapeSequence))
		return true
	}
	if ch == '"' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymDQuote))
		return true
	}
	for !lexer.EOF() && lexer.Lookahead != '"' && lexer.Lookahead != '\\' {
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	lexer.AcceptToken(ts.Symbol(tg.SymStringContent))
	return true
}

func jsonLexKW(lexer *ts.Lexer, keyword string, symbol ts.Symbol) bool {
	for _, expected := range keyword {
		if lexer.EOF() || lexer.Lookahead != expected {
			return false
		}
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	lexer.AcceptToken(symbol)
	return true
}

func jsonLexNum(lexer *ts.Lexer) bool {
	if lexer.Lookahead == '-' {
		lexer.Advance(false)
	}
	if lexer.EOF() || lexer.Lookahead < '0' || lexer.Lookahead > '9' {
		return false
	}
	for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
		lexer.Advance(false)
	}
	if !lexer.EOF() && lexer.Lookahead == '.' {
		lexer.Advance(false)
		for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
			lexer.Advance(false)
		}
	}
	if !lexer.EOF() && (lexer.Lookahead == 'e' || lexer.Lookahead == 'E') {
		lexer.Advance(false)
		if !lexer.EOF() && (lexer.Lookahead == '+' || lexer.Lookahead == '-') {
			lexer.Advance(false)
		}
		for !lexer.EOF() && lexer.Lookahead >= '0' && lexer.Lookahead <= '9' {
			lexer.Advance(false)
		}
	}
	lexer.MarkEnd()
	lexer.AcceptToken(ts.Symbol(tg.SymNumber))
	return true
}

func jsonLexCmt(lexer *ts.Lexer) bool {
	if lexer.Lookahead != '/' {
		return false
	}
	lexer.Advance(false)
	if lexer.EOF() {
		return false
	}
	if lexer.Lookahead == '/' {
		for !lexer.EOF() && lexer.Lookahead != '\n' {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymComment))
		return true
	}
	if lexer.Lookahead == '*' {
		lexer.Advance(false)
		for !lexer.EOF() {
			if lexer.Lookahead == '*' {
				lexer.Advance(false)
				if !lexer.EOF() && lexer.Lookahead == '/' {
					lexer.Advance(false)
					lexer.MarkEnd()
					lexer.AcceptToken(ts.Symbol(tg.SymComment))
					return true
				}
			} else {
				lexer.Advance(false)
			}
		}
		lexer.MarkEnd()
		lexer.AcceptToken(ts.Symbol(tg.SymComment))
		return true
	}
	return false
}

func jsonLanguageWithLex() *ts.Language {
	lang := tg.JSONLanguage()
	lang.LexFn = jsonLexFn
	return lang
}

// --- End-to-end parser tests ---

func TestIntegrationParseNull(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())
	tree := p.ParseString(context.Background(), []byte("null"))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}

	root := tree.RootNode()
	if root.IsNull() {
		t.Fatal("expected root node, got null")
	}
	if root.Type() != "document" {
		t.Errorf("expected root type 'document', got %q", root.Type())
	}
}

func TestIntegrationParseNumber(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte("42"))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "document" {
		t.Errorf("expected root type 'document', got %q", root.Type())
	}
}

func TestIntegrationParseBooleans(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	for _, input := range []string{"true", "false"} {
		tree := p.ParseString(context.Background(), []byte(input))
		if tree == nil {
			t.Fatalf("parse %q: expected tree", input)
		}
		root := tree.RootNode()
		if root.Type() != "document" {
			t.Errorf("parse %q: expected 'document', got %q", input, root.Type())
		}
	}
}

func TestIntegrationParseEmptyObject(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte("{}"))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "document" {
		t.Errorf("expected root type 'document', got %q", root.Type())
	}
	sexpr := root.String()
	if !strings.Contains(sexpr, "object") {
		t.Errorf("expected 'object' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationParseEmptyArray(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte("[]"))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "document" {
		t.Errorf("expected root type 'document', got %q", root.Type())
	}
}

func TestIntegrationParseString(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`"hello"`))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "document" {
		t.Errorf("expected 'document', got %q", root.Type())
	}
	sexpr := root.String()
	if !strings.Contains(sexpr, "string") {
		t.Errorf("expected 'string' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationParseObjectWithPair(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`{"key": "value"}`))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexpr := root.String()
	if !strings.Contains(sexpr, "object") {
		t.Errorf("expected 'object' in S-expression, got %q", sexpr)
	}
	if !strings.Contains(sexpr, "pair") {
		t.Errorf("expected 'pair' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationParseArray(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`[1, 2, 3]`))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexpr := root.String()
	if !strings.Contains(sexpr, "array") {
		t.Errorf("expected 'array' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationParseNestedJSON(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	input := `{"a": [1, true, null], "b": {"c": "d"}}`
	tree := p.ParseString(context.Background(), []byte(input))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexpr := root.String()
	if !strings.Contains(sexpr, "object") {
		t.Errorf("expected 'object', got %q", sexpr)
	}
	if !strings.Contains(sexpr, "array") {
		t.Errorf("expected 'array', got %q", sexpr)
	}
	if !strings.Contains(sexpr, "pair") {
		t.Errorf("expected 'pair', got %q", sexpr)
	}
}

func TestIntegrationMultipleParses(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	inputs := []string{
		"null", "true", "false", "42", `"hello"`, "[]", "{}",
		`[1, 2, 3]`, `{"a": 1}`,
	}
	for _, input := range inputs {
		tree := p.ParseString(context.Background(), []byte(input))
		if tree == nil {
			t.Errorf("parse %q: expected tree, got nil", input)
			continue
		}
		root := tree.RootNode()
		if root.Type() != "document" {
			t.Errorf("parse %q: expected 'document', got %q", input, root.Type())
		}
	}
}

func TestIntegrationParseWithWhitespace(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte("  null  "))
	if tree == nil {
		t.Fatal("expected tree")
	}
	root := tree.RootNode()
	if root.Type() != "document" {
		t.Errorf("expected 'document', got %q", root.Type())
	}
}

func TestIntegrationNodePositions(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte("null"))
	if tree == nil {
		t.Fatal("expected tree")
	}
	root := tree.RootNode()
	if root.StartByte() != 0 {
		t.Errorf("root start byte: expected 0, got %d", root.StartByte())
	}
	endByte := root.EndByte()
	if endByte != 4 {
		t.Errorf("root end byte: expected 4, got %d", endByte)
	}
}

func TestIntegrationParseSExprNull(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte("null"))
	if tree == nil {
		t.Fatal("expected tree")
	}
	sexpr := tree.RootNode().String()
	// null is a named leaf — the document wraps it.
	if !strings.Contains(sexpr, "null") {
		t.Errorf("expected 'null' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationParseSExprObject(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`{"key": "value"}`))
	if tree == nil {
		t.Fatal("expected tree")
	}
	sexpr := tree.RootNode().String()
	for _, want := range []string{"document", "object", "pair", "string"} {
		if !strings.Contains(sexpr, want) {
			t.Errorf("expected %q in S-expression, got %q", want, sexpr)
		}
	}
}

func TestIntegrationParseSExprNestedArray(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`[[1, 2], [3, 4]]`))
	if tree == nil {
		t.Fatal("expected tree")
	}
	sexpr := tree.RootNode().String()
	if !strings.Contains(sexpr, "array") {
		t.Errorf("expected 'array' in S-expression, got %q", sexpr)
	}
	if !strings.Contains(sexpr, "number") {
		t.Errorf("expected 'number' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationParseEscapeSequence(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`"hello\nworld"`))
	if tree == nil {
		t.Fatal("expected tree")
	}
	sexpr := tree.RootNode().String()
	if !strings.Contains(sexpr, "string") {
		t.Errorf("expected 'string' in S-expression, got %q", sexpr)
	}
	if !strings.Contains(sexpr, "escape_sequence") {
		t.Errorf("expected 'escape_sequence' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationParseObjectMultiplePairs(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`{"a": 1, "b": 2, "c": 3}`))
	if tree == nil {
		t.Fatal("expected tree")
	}
	root := tree.RootNode()
	sexpr := root.String()
	if !strings.Contains(sexpr, "object") {
		t.Errorf("expected 'object', got %q", sexpr)
	}
	// Count pair occurrences.
	count := strings.Count(sexpr, "pair")
	if count < 3 {
		t.Errorf("expected at least 3 'pair' nodes, got %d in %q", count, sexpr)
	}
}

func TestIntegrationParseDeepNesting(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	input := `{"a": {"b": {"c": {"d": [1, 2, {"e": true}]}}}}`
	tree := p.ParseString(context.Background(), []byte(input))
	if tree == nil {
		t.Fatal("expected tree")
	}
	root := tree.RootNode()
	if root.Type() != "document" {
		t.Errorf("expected 'document', got %q", root.Type())
	}
	sexpr := root.String()
	for _, want := range []string{"object", "pair", "array", "number", "true"} {
		if !strings.Contains(sexpr, want) {
			t.Errorf("expected %q in S-expression, got %q", want, sexpr)
		}
	}
}

func TestIntegrationParserReuse(t *testing.T) {
	// Verify that the same parser can parse multiple inputs sequentially.
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	inputs := []string{
		`{"a": 1}`,
		`[true, false, null]`,
		`"hello"`,
		`42`,
		`{"nested": {"array": [1, 2, 3]}}`,
	}
	for _, input := range inputs {
		tree := p.ParseString(context.Background(), []byte(input))
		if tree == nil {
			t.Errorf("parse %q: expected tree, got nil", input)
			continue
		}
		root := tree.RootNode()
		if root.Type() != "document" {
			t.Errorf("parse %q: expected 'document', got %q", input, root.Type())
		}
	}
}

func TestIntegrationParseNodeChildNavigation(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`[1, 2, 3]`))
	if tree == nil {
		t.Fatal("expected tree")
	}
	root := tree.RootNode()
	// Root is document, it should have at least 1 child (the array).
	if root.ChildCount() == 0 {
		t.Fatal("expected document to have children")
	}

	// First named child should be the array.
	arrayNode := root.NamedChild(0)
	if arrayNode.IsNull() {
		t.Fatal("expected array node")
	}
	if arrayNode.Type() != "array" {
		t.Errorf("expected 'array', got %q", arrayNode.Type())
	}
}

func TestIntegrationParseFieldAccess(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte(`{"name": "Alice"}`))
	if tree == nil {
		t.Fatal("expected tree")
	}
	root := tree.RootNode()
	sexpr := root.String()
	if !strings.Contains(sexpr, "pair") {
		t.Errorf("expected 'pair' in S-expression, got %q", sexpr)
	}
}

// --- External Scanner Integration Tests ---

func extScannerLanguageWithLex() *ts.Language {
	return tg.ExtScannerLanguageWithLex()
}

func TestIntegrationExternalScannerNumber(t *testing.T) {
	// Parse a simple number — no external scanner invoked.
	p := ts.NewParser()
	p.SetLanguage(extScannerLanguageWithLex())

	tree := p.ParseString(context.Background(), []byte("42"))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "program" {
		t.Errorf("expected root type 'program', got %q", root.Type())
	}
	sexpr := root.String()
	if !strings.Contains(sexpr, "number") {
		t.Errorf("expected 'number' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationExternalScannerHeredoc(t *testing.T) {
	// Parse a heredoc — exercises external scanner.
	p := ts.NewParser()
	p.SetLanguage(extScannerLanguageWithLex())

	input := "<<\nhello world\nEND\n"
	tree := p.ParseString(context.Background(), []byte(input))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "program" {
		t.Errorf("expected root type 'program', got %q", root.Type())
	}
	sexpr := root.String()
	if !strings.Contains(sexpr, "heredoc") {
		t.Errorf("expected 'heredoc' in S-expression, got %q", sexpr)
	}
	if !strings.Contains(sexpr, "heredoc_body") {
		t.Errorf("expected 'heredoc_body' in S-expression, got %q", sexpr)
	}
}

func TestIntegrationExternalScannerStateOnSubtree(t *testing.T) {
	// Parse a heredoc and verify the scanner state is attached to the subtree.
	p := ts.NewParser()
	p.SetLanguage(extScannerLanguageWithLex())

	input := "<<\nfoo\nEND\n"
	tree := p.ParseString(context.Background(), []byte(input))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}

	// Walk the tree looking for the heredoc_body node.
	root := tree.RootNode()
	found := false
	var walk func(n ts.Node)
	walk = func(n ts.Node) {
		if n.Type() == "heredoc_body" {
			found = true
		}
		for i := uint32(0); i < n.ChildCount(); i++ {
			child := n.Child(int(i))
			if !child.IsNull() {
				walk(child)
			}
		}
	}
	walk(root)

	if !found {
		t.Error("expected to find heredoc_body node in tree")
	}
}

func TestIntegrationExternalScannerMultipleParses(t *testing.T) {
	// Verify parser reuse with external scanner across multiple parses.
	p := ts.NewParser()
	p.SetLanguage(extScannerLanguageWithLex())

	inputs := []string{
		"42",
		"<<\nfoo\nEND\n",
		"99",
		"<<\nbar\nbaz\nEND\n",
	}
	for _, input := range inputs {
		tree := p.ParseString(context.Background(), []byte(input))
		if tree == nil {
			t.Errorf("parse %q: expected tree, got nil", input)
			continue
		}
		root := tree.RootNode()
		if root.Type() != "program" {
			t.Errorf("parse %q: expected 'program', got %q", input, root.Type())
		}
	}
}
