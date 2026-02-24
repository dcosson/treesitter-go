package treesitter_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
	cgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cgrammar"
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

func TestIntegrationParseComments(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(jsonLanguageWithLex())
	input := "{\"a\": 1,\n/*c1*/\n/*c2*/\n\"b\": 2\n}"
	tree := p.ParseString(context.Background(), []byte(input))
	if tree == nil {
		t.Fatal("expected tree")
	}
	root := tree.RootNode()
	sexpr := root.String()
	if !strings.Contains(sexpr, "comment") {
		t.Errorf("expected 'comment' in S-expression, got %q", sexpr)
	}
	// Both pairs must be present.
	pairCount := strings.Count(sexpr, "pair")
	if pairCount != 2 {
		t.Errorf("expected 2 pairs, got %d in %q", pairCount, sexpr)
	}
	commentCount := strings.Count(sexpr, "comment")
	if commentCount != 2 {
		t.Errorf("expected 2 comments, got %d in %q", commentCount, sexpr)
	}
}

// --- Incremental Parsing Integration Tests ---

// incrementalParseTest is a helper that:
//  1. Parses the original source from scratch
//  2. Applies an edit to the tree
//  3. Re-parses the new source with the old (edited) tree
//  4. Parses the new source from scratch
//  5. Verifies both produce the same S-expression
func incrementalParseTest(t *testing.T, lang *ts.Language, oldSource, newSource string, edit *ts.InputEdit) {
	t.Helper()
	ctx := context.Background()

	// Step 1: Parse original from scratch.
	p := ts.NewParser()
	p.SetLanguage(lang)
	tree1 := p.ParseString(ctx, []byte(oldSource))
	if tree1 == nil {
		t.Fatal("initial parse returned nil")
	}

	// Step 2: Apply edit to the tree.
	editedTree := tree1.Edit(edit)

	// Step 3: Re-parse new source with old tree (incremental).
	p2 := ts.NewParser()
	p2.SetLanguage(lang)
	tree2 := p2.ParseString(ctx, []byte(newSource), editedTree)
	if tree2 == nil {
		t.Fatal("incremental parse returned nil")
	}

	// Step 4: Parse new source from scratch.
	p3 := ts.NewParser()
	p3.SetLanguage(lang)
	tree3 := p3.ParseString(ctx, []byte(newSource))
	if tree3 == nil {
		t.Fatal("from-scratch parse of new source returned nil")
	}

	// Step 5: Compare S-expressions.
	sexpr2 := tree2.RootNode().String()
	sexpr3 := tree3.RootNode().String()
	if sexpr2 != sexpr3 {
		t.Errorf("incremental vs from-scratch mismatch:\n  incremental: %s\n  from-scratch: %s", sexpr2, sexpr3)
	}

	// Also verify byte spans match.
	if tree2.RootNode().EndByte() != tree3.RootNode().EndByte() {
		t.Errorf("end byte mismatch: incremental=%d, from-scratch=%d",
			tree2.RootNode().EndByte(), tree3.RootNode().EndByte())
	}
}

func TestIncrementalReplaceValue(t *testing.T) {
	// "null" -> "true"
	incrementalParseTest(t, jsonLanguageWithLex(),
		"null", "true",
		&ts.InputEdit{
			StartByte: 0, OldEndByte: 4, NewEndByte: 4,
			StartPoint:  ts.Point{Row: 0, Column: 0},
			OldEndPoint: ts.Point{Row: 0, Column: 4},
			NewEndPoint: ts.Point{Row: 0, Column: 4},
		})
}

func TestIncrementalInsertInObject(t *testing.T) {
	// {"a": 1} -> {"a": 12}  (insert '2' at position 7)
	incrementalParseTest(t, jsonLanguageWithLex(),
		`{"a": 1}`, `{"a": 12}`,
		&ts.InputEdit{
			StartByte: 7, OldEndByte: 7, NewEndByte: 8,
			StartPoint:  ts.Point{Row: 0, Column: 7},
			OldEndPoint: ts.Point{Row: 0, Column: 7},
			NewEndPoint: ts.Point{Row: 0, Column: 8},
		})
}

func TestIncrementalDeleteInArray(t *testing.T) {
	// [1, 2, 3] -> [1, 3]  (delete ", 2")
	incrementalParseTest(t, jsonLanguageWithLex(),
		`[1, 2, 3]`, `[1, 3]`,
		&ts.InputEdit{
			StartByte: 2, OldEndByte: 5, NewEndByte: 2,
			StartPoint:  ts.Point{Row: 0, Column: 2},
			OldEndPoint: ts.Point{Row: 0, Column: 5},
			NewEndPoint: ts.Point{Row: 0, Column: 2},
		})
}

func TestIncrementalInsertNewPair(t *testing.T) {
	// {"a": 1} -> {"a": 1, "b": 2}
	incrementalParseTest(t, jsonLanguageWithLex(),
		`{"a": 1}`, `{"a": 1, "b": 2}`,
		&ts.InputEdit{
			StartByte: 7, OldEndByte: 7, NewEndByte: 15,
			StartPoint:  ts.Point{Row: 0, Column: 7},
			OldEndPoint: ts.Point{Row: 0, Column: 7},
			NewEndPoint: ts.Point{Row: 0, Column: 15},
		})
}

func TestIncrementalNoopEdit(t *testing.T) {
	// "42" -> "42" (no change, noop edit)
	incrementalParseTest(t, jsonLanguageWithLex(),
		"42", "42",
		&ts.InputEdit{
			StartByte: 1, OldEndByte: 1, NewEndByte: 1,
			StartPoint:  ts.Point{Row: 0, Column: 1},
			OldEndPoint: ts.Point{Row: 0, Column: 1},
			NewEndPoint: ts.Point{Row: 0, Column: 1},
		})
}

func TestIncrementalReplaceEntireSource(t *testing.T) {
	// "null" -> "[1, 2]"
	incrementalParseTest(t, jsonLanguageWithLex(),
		"null", "[1, 2]",
		&ts.InputEdit{
			StartByte: 0, OldEndByte: 4, NewEndByte: 6,
			StartPoint:  ts.Point{Row: 0, Column: 0},
			OldEndPoint: ts.Point{Row: 0, Column: 4},
			NewEndPoint: ts.Point{Row: 0, Column: 6},
		})
}

func TestIncrementalEditNestedJSON(t *testing.T) {
	// {"a": [1, 2]} -> {"a": [1, 2, 3]}
	old := `{"a": [1, 2]}`
	new_ := `{"a": [1, 2, 3]}`
	incrementalParseTest(t, jsonLanguageWithLex(),
		old, new_,
		&ts.InputEdit{
			StartByte: 11, OldEndByte: 11, NewEndByte: 14,
			StartPoint:  ts.Point{Row: 0, Column: 11},
			OldEndPoint: ts.Point{Row: 0, Column: 11},
			NewEndPoint: ts.Point{Row: 0, Column: 14},
		})
}

func TestIncrementalEditPreservesUnchanged(t *testing.T) {
	// Verify that the incremental parse tree has has_changes on the root
	// but the structure matches from-scratch.
	ctx := context.Background()
	lang := jsonLanguageWithLex()

	p := ts.NewParser()
	p.SetLanguage(lang)
	tree := p.ParseString(ctx, []byte(`{"a": 1, "b": 2}`))
	if tree == nil {
		t.Fatal("initial parse returned nil")
	}

	// Edit: change value of "a" from 1 to 99
	// {"a": 1, "b": 2} -> {"a": 99, "b": 2}
	editedTree := tree.Edit(&ts.InputEdit{
		StartByte: 6, OldEndByte: 7, NewEndByte: 8,
		StartPoint:  ts.Point{Row: 0, Column: 6},
		OldEndPoint: ts.Point{Row: 0, Column: 7},
		NewEndPoint: ts.Point{Row: 0, Column: 8},
	})

	// The edited tree should have has_changes on root.
	if !editedTree.RootNode().HasChanges() {
		t.Error("edited tree root should have has_changes")
	}

	// Re-parse incrementally.
	p2 := ts.NewParser()
	p2.SetLanguage(lang)
	tree2 := p2.ParseString(ctx, []byte(`{"a": 99, "b": 2}`), editedTree)
	if tree2 == nil {
		t.Fatal("incremental parse returned nil")
	}

	// From-scratch parse.
	p3 := ts.NewParser()
	p3.SetLanguage(lang)
	tree3 := p3.ParseString(ctx, []byte(`{"a": 99, "b": 2}`))
	if tree3 == nil {
		t.Fatal("from-scratch parse returned nil")
	}

	if tree2.RootNode().String() != tree3.RootNode().String() {
		t.Errorf("mismatch:\n  incremental: %s\n  from-scratch: %s",
			tree2.RootNode().String(), tree3.RootNode().String())
	}
}

func TestIncrementalMultipleEdits(t *testing.T) {
	// Apply two sequential edits and verify final parse matches from-scratch.
	ctx := context.Background()
	lang := jsonLanguageWithLex()

	// Parse "null"
	p := ts.NewParser()
	p.SetLanguage(lang)
	tree1 := p.ParseString(ctx, []byte("null"))
	if tree1 == nil {
		t.Fatal("initial parse returned nil")
	}

	// Edit 1: "null" -> "42"
	edited1 := tree1.Edit(&ts.InputEdit{
		StartByte: 0, OldEndByte: 4, NewEndByte: 2,
		StartPoint:  ts.Point{Row: 0, Column: 0},
		OldEndPoint: ts.Point{Row: 0, Column: 4},
		NewEndPoint: ts.Point{Row: 0, Column: 2},
	})

	p2 := ts.NewParser()
	p2.SetLanguage(lang)
	tree2 := p2.ParseString(ctx, []byte("42"), edited1)
	if tree2 == nil {
		t.Fatal("first incremental parse returned nil")
	}

	// Edit 2: "42" -> "420"
	edited2 := tree2.Edit(&ts.InputEdit{
		StartByte: 2, OldEndByte: 2, NewEndByte: 3,
		StartPoint:  ts.Point{Row: 0, Column: 2},
		OldEndPoint: ts.Point{Row: 0, Column: 2},
		NewEndPoint: ts.Point{Row: 0, Column: 3},
	})

	p3 := ts.NewParser()
	p3.SetLanguage(lang)
	tree3 := p3.ParseString(ctx, []byte("420"), edited2)
	if tree3 == nil {
		t.Fatal("second incremental parse returned nil")
	}

	// From-scratch parse of "420".
	p4 := ts.NewParser()
	p4.SetLanguage(lang)
	tree4 := p4.ParseString(ctx, []byte("420"))
	if tree4 == nil {
		t.Fatal("from-scratch parse returned nil")
	}

	if tree3.RootNode().String() != tree4.RootNode().String() {
		t.Errorf("mismatch after 2 edits:\n  incremental: %s\n  from-scratch: %s",
			tree3.RootNode().String(), tree4.RootNode().String())
	}
}

func TestIncrementalWithNilOldTree(t *testing.T) {
	// ParseString with explicit nil old tree should work identically to no old tree.
	ctx := context.Background()
	lang := jsonLanguageWithLex()

	p := ts.NewParser()
	p.SetLanguage(lang)
	tree := p.ParseString(ctx, []byte("null"), nil)
	if tree == nil {
		t.Fatal("expected tree with nil old tree")
	}
	if tree.RootNode().Type() != "document" {
		t.Errorf("expected 'document', got %q", tree.RootNode().Type())
	}
}

func TestCTypedefKeywordDebug(t *testing.T) {
	lang := cgrammar.CLanguage()
	p := ts.NewParser()
	p.SetLanguage(lang)
	p.SetDebugKeywords(true)
	p.SetDebug(true)

	input := []byte("typedef unsigned long int;\n")
	tree := p.ParseString(context.Background(), input)
	if tree == nil {
		t.Fatal("parse returned nil")
	}
	sexp := tree.RootNode().String()
	t.Logf("Result: %s", sexp)
	if !strings.Contains(sexp, "primitive_type") {
		t.Errorf("expected primitive_type in declarator, got: %s", sexp)
	}

	// Dump state 1410 actions
	t.Logf("State 1410 token actions:")
	for sym := uint32(0); sym < lang.TokenCount; sym++ {
		val := lang.ExportLookup(1410, ts.Symbol(sym))
		if val != 0 {
			name := ""
			if int(sym) < len(lang.SymbolNames) {
				name = lang.SymbolNames[sym]
			}
			t.Logf("  sym=%d (%s) -> action=%d", sym, name, val)
		}
	}
	// Also check for non-terminal goto entries
	t.Logf("State 1410 non-terminal gotos:")
	for sym := lang.TokenCount; sym < lang.SymbolCount; sym++ {
		val := lang.ExportLookup(1410, ts.Symbol(sym))
		if val != 0 {
			name := ""
			if int(sym) < len(lang.SymbolNames) {
				name = lang.SymbolNames[sym]
			}
			t.Logf("  sym=%d (%s) -> goto=%d", sym, name, val)
		}
	}

	// Check lex modes for both states
	t.Logf("LexMode for state 1136: %+v", lang.LexModes[1136])
	t.Logf("LexMode for state 1410: %+v", lang.LexModes[1410])

	// Check what actions state 1136 has for primitive_type and identifier
	t.Logf("State 1136 lookup(primitive_type=93)=%d", lang.ExportLookup(1136, 93))
	t.Logf("State 1136 lookup(identifier=1)=%d", lang.ExportLookup(1136, 1))

	// Check the actual table entry (actions) for state 1136 + primitive_type
	for _, checkState := range []uint16{1136, 1114} {
		entry := lang.ExportTableEntry(checkState, 93)
		t.Logf("State %d table_entry for primitive_type: ActionCount=%d Reusable=%v", checkState, entry.ActionCount, entry.Reusable)
		for i, action := range entry.Actions {
			rsym := ""
			if int(action.ReduceSymbol) < len(lang.SymbolNames) {
				rsym = lang.SymbolNames[action.ReduceSymbol]
			}
			t.Logf("  action[%d]: Type=%d ShiftState=%d ShiftExtra=%v ReduceSymbol=%d(%s) ReduceCount=%d DynPrec=%d",
				i, action.Type, action.ShiftState, action.ShiftExtra, action.ReduceSymbol, rsym, action.ReduceChildCount, action.ReduceDynPrec)
		}
	}
}
