package treesitter

import (
	"context"
	
	"testing"
	"time"
)

// JSON symbol IDs (copied from internal/testgrammars to avoid import cycle).
const (
	symEnd                Symbol = 0
	symLBrace             Symbol = 1
	symComma              Symbol = 2
	symRBrace             Symbol = 3
	symColon              Symbol = 4
	symLBrack             Symbol = 5
	symRBrack             Symbol = 6
	symDQuote             Symbol = 7
	symStringContent      Symbol = 8
	symEscapeSequence     Symbol = 9
	symNumber             Symbol = 10
	symTrue               Symbol = 11
	symFalse              Symbol = 12
	symNull               Symbol = 13
	symComment            Symbol = 14
	symDocument           Symbol = 15
	symValue              Symbol = 16
	symObject             Symbol = 17
	symPair               Symbol = 18
	symArray              Symbol = 19
	symString             Symbol = 20
	symAuxStringContent   Symbol = 21
	symAuxDocRepeat1      Symbol = 22
	symAuxObjRepeat1      Symbol = 23
	symAuxArrRepeat1      Symbol = 24
	numJSONSymbols        uint32 = 25
)

// jsonLexFn is a hand-written lex function for the JSON grammar.
func jsonLexFn(lexer *Lexer, state StateID) bool {
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
		lexer.AcceptToken(symLBrace)
		return true
	case ch == '}':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(symRBrace)
		return true
	case ch == '[':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(symLBrack)
		return true
	case ch == ']':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(symRBrack)
		return true
	case ch == ',':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(symComma)
		return true
	case ch == ':':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(symColon)
		return true
	case ch == '"':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(symDQuote)
		return true
	case ch == 't':
		return jsonLexKW(lexer, "true", symTrue)
	case ch == 'f':
		return jsonLexKW(lexer, "false", symFalse)
	case ch == 'n':
		return jsonLexKW(lexer, "null", symNull)
	case ch == '-' || (ch >= '0' && ch <= '9'):
		return jsonLexNum(lexer)
	case ch == '/':
		return jsonLexCmt(lexer)
	}

	return false
}

func jsonLexStringContent(lexer *Lexer) bool {
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
		lexer.AcceptToken(symEscapeSequence)
		return true
	}
	if ch == '"' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(symDQuote)
		return true
	}
	for !lexer.EOF() && lexer.Lookahead != '"' && lexer.Lookahead != '\\' {
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	lexer.AcceptToken(symStringContent)
	return true
}

func jsonLexKW(lexer *Lexer, keyword string, symbol Symbol) bool {
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

func jsonLexNum(lexer *Lexer) bool {
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
	lexer.AcceptToken(symNumber)
	return true
}

func jsonLexCmt(lexer *Lexer) bool {
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
		lexer.AcceptToken(symComment)
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
					lexer.AcceptToken(symComment)
					return true
				}
			} else {
				lexer.Advance(false)
			}
		}
		lexer.MarkEnd()
		lexer.AcceptToken(symComment)
		return true
	}
	return false
}

// testJSONLanguage returns the JSON language with lex function for parser tests.
// This builds the language inline to avoid import cycles with internal/testgrammars.
func testJSONLanguage() *Language {
	lang := buildTestJSONLanguage()
	lang.LexFn = jsonLexFn
	return lang
}

// buildTestJSONLanguage constructs a stub JSON grammar language struct
// for parser API lifecycle tests (set language, reset, cancellation).
// Real parsing tests use the complete grammar in parser_integration_test.go.
func buildTestJSONLanguage() *Language {
	symbolMetadata := make([]SymbolMetadata, numJSONSymbols)
	for _, sym := range []Symbol{symLBrace, symComma, symRBrace, symColon, symLBrack, symRBrack, symDQuote} {
		symbolMetadata[sym] = SymbolMetadata{Visible: true, Named: false}
	}
	for _, sym := range []Symbol{symStringContent, symEscapeSequence, symNumber, symTrue, symFalse, symNull, symComment} {
		symbolMetadata[sym] = SymbolMetadata{Visible: true, Named: true}
	}
	symbolMetadata[symDocument] = SymbolMetadata{Visible: true, Named: true}
	symbolMetadata[symValue] = SymbolMetadata{Visible: false, Named: true}
	symbolMetadata[symObject] = SymbolMetadata{Visible: true, Named: true}
	symbolMetadata[symPair] = SymbolMetadata{Visible: true, Named: true}
	symbolMetadata[symArray] = SymbolMetadata{Visible: true, Named: true}
	symbolMetadata[symString] = SymbolMetadata{Visible: true, Named: true}

	symbolNames := []string{
		"end", "{", ",", "}", ":", "[", "]", "\"",
		"string_content", "escape_sequence", "number",
		"true", "false", "null", "comment",
		"document", "_value", "object", "pair", "array", "string",
		"_string_content", "document_repeat1", "object_repeat1", "array_repeat1",
	}

	return &Language{
		Version:        15,
		SymbolCount:    numJSONSymbols,
		SymbolMetadata: symbolMetadata,
		SymbolNames:    symbolNames,
	}
}

// --- Tests ---

func TestParseNull(t *testing.T) {
	// Use external test to avoid import cycle. For now, test basic parser lifecycle.
	p := NewParser()
	if p.Language() != nil {
		t.Error("expected nil language initially")
	}

	lang := testJSONLanguage()
	p.SetLanguage(lang)
	if p.Language() != lang {
		t.Error("SetLanguage didn't take effect")
	}
}

func TestParserNoLanguage(t *testing.T) {
	p := NewParser()
	tree := p.ParseString(context.Background(), []byte("null"))
	if tree != nil {
		t.Error("expected nil tree without language")
	}
}

func TestParserContextCancellation(t *testing.T) {
	p := NewParser()
	lang := testJSONLanguage()
	p.SetLanguage(lang)
	p.cancellationCheckInterval = 1

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	tree := p.ParseString(ctx, []byte("null"))
	if tree != nil {
		t.Error("expected nil tree after cancellation")
	}
}

func TestParserContextTimeout(t *testing.T) {
	p := NewParser()
	lang := testJSONLanguage()
	p.SetLanguage(lang)
	p.cancellationCheckInterval = 1

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond) // Let timeout expire.

	tree := p.ParseString(ctx, []byte(`{"a": [1, 2, 3]}`))
	if tree != nil {
		t.Error("expected nil tree after timeout")
	}
}

func TestParserReset(t *testing.T) {
	p := NewParser()
	lang := testJSONLanguage()
	p.SetLanguage(lang)

	// Verify reset clears state.
	p.acceptCount = 5
	p.operationCount = 100
	p.cachedTokenValid = true
	p.Reset()

	if p.acceptCount != 0 {
		t.Errorf("expected acceptCount 0, got %d", p.acceptCount)
	}
	if p.operationCount != 0 {
		t.Errorf("expected operationCount 0, got %d", p.operationCount)
	}
	if p.cachedTokenValid {
		t.Error("expected cachedTokenValid false")
	}
	if !p.finishedTree.IsZero() {
		t.Error("expected finishedTree zero")
	}
}

func TestCompareVersions(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name string
		a, b errorStatus
		want errorComparison
	}{
		{
			name: "non-error beats in-error with lower cost",
			a:    errorStatus{cost: 50, isInError: false},
			b:    errorStatus{cost: 100, isInError: true},
			want: errorComparisonTakeLeft,
		},
		{
			name: "non-error beats in-error with equal cost",
			a:    errorStatus{cost: 100, isInError: false},
			b:    errorStatus{cost: 100, isInError: true},
			want: errorComparisonPreferLeft,
		},
		{
			name: "non-error beats in-error with higher cost",
			a:    errorStatus{cost: 200, isInError: false},
			b:    errorStatus{cost: 100, isInError: true},
			want: errorComparisonPreferLeft,
		},
		{
			name: "in-error loses to non-error (symmetric)",
			a:    errorStatus{cost: 100, isInError: true},
			b:    errorStatus{cost: 200, isInError: false},
			want: errorComparisonPreferRight,
		},
		{
			name: "cost amplification - decisive kill",
			// cost_diff=100, nodeCount=20: 100 * 21 = 2100 > 1600
			a: errorStatus{cost: 100, nodeCount: 20},
			b: errorStatus{cost: 200, nodeCount: 0},
			want: errorComparisonTakeLeft,
		},
		{
			name: "cost amplification - soft preference",
			// cost_diff=100, nodeCount=5: 100 * 6 = 600 < 1600
			a: errorStatus{cost: 100, nodeCount: 5},
			b: errorStatus{cost: 200, nodeCount: 0},
			want: errorComparisonPreferLeft,
		},
		{
			name: "cost amplification - symmetric decisive kill",
			// cost_diff=100, nodeCount=20: 100 * 21 = 2100 > 1600
			a: errorStatus{cost: 200, nodeCount: 0},
			b: errorStatus{cost: 100, nodeCount: 20},
			want: errorComparisonTakeRight,
		},
		{
			name: "equal cost - dynamic precedence tiebreak left",
			a:    errorStatus{cost: 50, dynamicPrecedence: 10},
			b:    errorStatus{cost: 50, dynamicPrecedence: 5},
			want: errorComparisonPreferLeft,
		},
		{
			name: "equal cost - dynamic precedence tiebreak right",
			a:    errorStatus{cost: 50, dynamicPrecedence: 5},
			b:    errorStatus{cost: 50, dynamicPrecedence: 10},
			want: errorComparisonPreferRight,
		},
		{
			name: "completely equal",
			a:    errorStatus{cost: 50, nodeCount: 10, dynamicPrecedence: 5},
			b:    errorStatus{cost: 50, nodeCount: 10, dynamicPrecedence: 5},
			want: errorComparisonNone,
		},
		{
			name: "both in error - cost decides",
			a:    errorStatus{cost: 50, isInError: true},
			b:    errorStatus{cost: 200, isInError: true},
			want: errorComparisonPreferLeft,
		},
		{
			name: "zero cost difference with large nodeCount",
			a:    errorStatus{cost: 0, nodeCount: 1000},
			b:    errorStatus{cost: 0, nodeCount: 0},
			want: errorComparisonNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.compareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareVersions(%+v, %+v) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestFindActiveVersionRoundRobin(t *testing.T) {
	p := NewParser()
	p.arena = NewSubtreeArena(0)
	p.stack = NewStack(p.arena)

	// Create two versions at different positions.
	// v0 at position 10, v1 at position 20.
	p.stack.AddVersion(1, Length{Bytes: 10, Point: Point{Row: 0, Column: 10}})
	p.stack.AddVersion(2, Length{Bytes: 20, Point: Point{Row: 0, Column: 20}})

	// findActiveVersion should initially pick v0 (lowest position).
	v := p.findActiveVersion()
	if v != 0 {
		t.Fatalf("expected v0, got v%d", v)
	}

	// Call findActiveVersion repeatedly without advancing v0's position.
	// After maxStaleSelections, it should rotate to v1.
	for i := 0; i < maxStaleSelections-1; i++ {
		v = p.findActiveVersion()
		if v != 0 {
			t.Fatalf("iteration %d: expected v0 (stale count %d < %d), got v%d",
				i, i+1, maxStaleSelections, v)
		}
	}

	// This call should trigger rotation to v1.
	v = p.findActiveVersion()
	if v != 1 {
		t.Fatalf("expected round-robin rotation to v1, got v%d", v)
	}

	// After rotation, counter resets. Next call picks v0 again (lowest pos).
	v = p.findActiveVersion()
	if v != 0 {
		t.Fatalf("expected v0 after rotation reset, got v%d", v)
	}
}
