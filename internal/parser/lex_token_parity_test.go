package parser

import "testing"

func makeLexTokenParityLanguage(reusableState2 bool) *Language {
	stateCount := uint32(3)  // 0 = error, 1 = normal, 2 = alt
	symbolCount := uint32(4) // 0 = end, 1 = tok, 2 = alt_tok, 3 = aux
	tokenCount := uint32(4)

	parseTable := make([]uint16, stateCount*symbolCount)
	parseActions := []ParseActionEntry{{}} // index 0 = no action

	addShift := func(state StateID, symbol Symbol, reusable bool) {
		actionIndex := uint16(len(parseActions))
		parseActions = append(parseActions,
			ParseActionEntry{Type: ParseActionTypeHeader, Count: 1, Reusable: reusable},
			ParseActionEntry{Type: ParseActionTypeShift, ShiftState: state},
		)
		parseTable[uint32(state)*symbolCount+uint32(symbol)] = actionIndex
	}

	// State 1 and state 2 both have actions for token 1.
	addShift(1, 1, true)
	addShift(2, 1, reusableState2)

	return &Language{
		Version:         15,
		SymbolCount:     symbolCount,
		TokenCount:      tokenCount,
		StateCount:      stateCount,
		LargeStateCount: stateCount,
		ParseTable:      parseTable,
		ParseActions:    parseActions,
		LexModes: []LexMode{
			{LexState: 0, ExternalLexState: 0}, // error state
			{LexState: 1, ExternalLexState: 0}, // normal state
			{LexState: 2, ExternalLexState: 0}, // alternate state
		},
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // end
			{Visible: true, Named: false},  // tok
			{Visible: true, Named: false},  // alt_tok
			{Visible: false, Named: false}, // aux
		},
		SymbolNames: []string{"end", "tok", "alt_tok", "aux"},
	}
}

func makeExternalTokenWithState(arena *SubtreeArena, b byte) Subtree {
	st, data := arena.Alloc()
	data.Symbol = 1
	data.SetFlag(SubtreeFlagHasExternalTokens, true)
	SetExternalScannerState(st, arena, []byte{b})
	return st
}

func TestLexTokenCacheInvalidatesOnLastExternalTokenState(t *testing.T) {
	lang := makeLexTokenParityLanguage(true)
	lexCalls := 0
	lang.LexFn = func(lexer *Lexer, _ StateID) bool {
		lexCalls++
		if lexer.EOF() {
			return false
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(1)
		return true
	}

	p := NewParser()
	p.SetLanguage(lang)
	p.lexer.SetInput(NewStringInput([]byte("a")))
	version := p.stack.AddVersion(1, LengthZero)

	p.stack.SetLastExternalToken(version, makeExternalTokenWithState(p.arena, 1))
	_ = p.lexToken(version, 1, LengthZero)
	if lexCalls != 1 {
		t.Fatalf("first lex calls = %d, want 1", lexCalls)
	}

	_ = p.lexToken(version, 1, LengthZero)
	if lexCalls != 1 {
		t.Fatalf("cache hit lex calls = %d, want 1", lexCalls)
	}

	p.stack.SetLastExternalToken(version, makeExternalTokenWithState(p.arena, 2))
	_ = p.lexToken(version, 1, LengthZero)
	if lexCalls != 2 {
		t.Fatalf("cache invalidation lex calls = %d, want 2", lexCalls)
	}
}

func TestLexTokenCacheReuseWhenLexModeDiffersButTokenReusable(t *testing.T) {
	t.Run("reusable", func(t *testing.T) {
		lang := makeLexTokenParityLanguage(true)
		lexCalls := 0
		lang.LexFn = func(lexer *Lexer, _ StateID) bool {
			lexCalls++
			if lexer.EOF() {
				return false
			}
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.AcceptToken(1)
			return true
		}

		p := NewParser()
		p.SetLanguage(lang)
		p.lexer.SetInput(NewStringInput([]byte("a")))
		version := p.stack.AddVersion(1, LengthZero)

		_ = p.lexToken(version, 1, LengthZero)
		_ = p.lexToken(version, 2, LengthZero)
		if lexCalls != 1 {
			t.Fatalf("lex calls = %d, want 1 (cache reuse across lex mode)", lexCalls)
		}
	})

	t.Run("not_reusable", func(t *testing.T) {
		lang := makeLexTokenParityLanguage(false)
		lexCalls := 0
		lang.LexFn = func(lexer *Lexer, _ StateID) bool {
			lexCalls++
			if lexer.EOF() {
				return false
			}
			lexer.Advance(false)
			lexer.MarkEnd()
			lexer.AcceptToken(1)
			return true
		}

		p := NewParser()
		p.SetLanguage(lang)
		p.lexer.SetInput(NewStringInput([]byte("a")))
		version := p.stack.AddVersion(1, LengthZero)

		_ = p.lexToken(version, 1, LengthZero)
		_ = p.lexToken(version, 2, LengthZero)
		if lexCalls != 2 {
			t.Fatalf("lex calls = %d, want 2 (no cache reuse when not reusable)", lexCalls)
		}
	})
}

func TestLexTokenErrorModeRespectsIncludedRangeTransitions(t *testing.T) {
	lang := makeLexTokenParityLanguage(true)
	lang.LexFn = func(lexer *Lexer, state StateID) bool {
		// Normal state: no token, force error-mode fallback.
		if state == 1 {
			return false
		}
		// Error state: recognize 'z' only.
		if lexer.EOF() || lexer.Lookahead != 'z' {
			return false
		}
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.AcceptToken(2)
		return true
	}

	p := NewParser()
	p.SetLanguage(lang)
	p.lexer.SetInput(NewStringInput([]byte("xYz")))
	p.lexer.SetIncludedRanges([]Range{
		{StartByte: 0, EndByte: 1, StartPoint: Point{Row: 0, Column: 0}, EndPoint: Point{Row: 0, Column: 1}},
		{StartByte: 2, EndByte: 3, StartPoint: Point{Row: 0, Column: 2}, EndPoint: Point{Row: 0, Column: 3}},
	})
	version := p.stack.AddVersion(1, LengthZero)

	token := p.lexToken(version, 1, LengthZero)
	if got := GetSymbol(token, p.arena); got != SymbolError {
		t.Fatalf("symbol = %d, want SymbolError", got)
	}
	// Skipped span should include x and the excluded gap transition to z.
	if got := GetSize(token, p.arena).Bytes; got != 2 {
		t.Fatalf("error token size = %d, want 2", got)
	}
}
