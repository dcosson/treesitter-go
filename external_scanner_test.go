package treesitter

import (
	"bytes"
	"testing"
)

// --- External scanner state on subtrees ---

func TestExternalScannerStateSetGet(t *testing.T) {
	arena := NewSubtreeArena(0)
	st, _ := arena.Alloc()

	// Initially no state.
	state := GetExternalScannerState(st, arena)
	if state != nil {
		t.Errorf("expected nil initial state, got %v", state)
	}

	// Set state.
	SetExternalScannerState(st, arena, []byte{1, 2, 3})
	state = GetExternalScannerState(st, arena)
	if !bytes.Equal(state, []byte{1, 2, 3}) {
		t.Errorf("expected [1,2,3], got %v", state)
	}

	// Overwrite state.
	SetExternalScannerState(st, arena, []byte{4, 5})
	state = GetExternalScannerState(st, arena)
	if !bytes.Equal(state, []byte{4, 5}) {
		t.Errorf("expected [4,5], got %v", state)
	}

	// Clear state.
	SetExternalScannerState(st, arena, nil)
	state = GetExternalScannerState(st, arena)
	if state != nil {
		t.Errorf("expected nil after clear, got %v", state)
	}
}

func TestExternalScannerStateEqual(t *testing.T) {
	arena := NewSubtreeArena(0)
	arena.Alloc() // skip zero-offset
	st, _ := arena.Alloc()

	SetExternalScannerState(st, arena, []byte{1, 2, 3})

	// Equal.
	if !ExternalScannerStateEqual(st, arena, []byte{1, 2, 3}, 3) {
		t.Error("expected equal")
	}

	// Different length.
	if ExternalScannerStateEqual(st, arena, []byte{1, 2}, 2) {
		t.Error("expected not equal (different length)")
	}

	// Different content.
	if ExternalScannerStateEqual(st, arena, []byte{1, 2, 4}, 3) {
		t.Error("expected not equal (different content)")
	}

	// Zero subtree.
	if ExternalScannerStateEqual(SubtreeZero, arena, []byte{1}, 1) {
		t.Error("expected not equal for zero subtree")
	}

	// Inline subtree.
	inline := newInlineSubtree(1, 0, LengthZero, LengthZero, true, true, false, false)
	if ExternalScannerStateEqual(inline, arena, []byte{1}, 1) {
		t.Error("expected not equal for inline subtree")
	}
}

func TestExternalScannerStateOnInlineSubtree(t *testing.T) {
	arena := NewSubtreeArena(0)
	inline := newInlineSubtree(1, 0, LengthZero, LengthZero, true, true, false, false)

	// Setting state on inline should be no-op.
	SetExternalScannerState(inline, arena, []byte{1, 2})
	state := GetExternalScannerState(inline, arena)
	if state != nil {
		t.Errorf("expected nil for inline subtree, got %v", state)
	}
}

func TestExternalScannerStateCopy(t *testing.T) {
	arena := NewSubtreeArena(0)
	arena.Alloc() // skip zero-offset
	st, _ := arena.Alloc()

	// Verify the state is a copy, not a reference.
	original := []byte{1, 2, 3}
	SetExternalScannerState(st, arena, original)

	// Modify original — should not affect the stored state.
	original[0] = 99
	state := GetExternalScannerState(st, arena)
	if state[0] != 1 {
		t.Errorf("expected state[0]=1 (independent copy), got %d", state[0])
	}
}

func TestHasExternalTokensFlag(t *testing.T) {
	arena := NewSubtreeArena(0)
	arena.Alloc() // skip zero-offset
	st, data := arena.Alloc()

	if HasExternalTokens(st, arena) {
		t.Error("expected no external tokens initially")
	}

	data.SetFlag(SubtreeFlagHasExternalTokens, true)
	if !HasExternalTokens(st, arena) {
		t.Error("expected external tokens after setting flag")
	}

	// Inline subtrees never have external tokens.
	inline := newInlineSubtree(1, 0, LengthZero, LengthZero, true, true, false, false)
	if HasExternalTokens(inline, arena) {
		t.Error("expected no external tokens for inline subtree")
	}
}

func TestHasExternalScannerStateChangeFlag(t *testing.T) {
	arena := NewSubtreeArena(0)
	arena.Alloc() // skip zero-offset
	st, data := arena.Alloc()

	if data.HasFlag(SubtreeFlagHasExternalScannerStateChange) {
		t.Error("expected no state change initially")
	}

	data.SetFlag(SubtreeFlagHasExternalScannerStateChange, true)
	if !data.HasFlag(SubtreeFlagHasExternalScannerStateChange) {
		t.Error("expected state change flag set")
	}

	// Doesn't conflict with other flags.
	data.SetFlag(SubtreeFlagHasExternalTokens, true)
	if !data.HasFlag(SubtreeFlagHasExternalScannerStateChange) {
		t.Error("state change flag lost after setting other flag")
	}
	if !data.HasFlag(SubtreeFlagHasExternalTokens) {
		t.Error("external tokens flag not set")
	}

	_ = st // ensure arena ref is valid
}

// --- EnabledExternalTokens ---

func TestEnabledExternalTokens(t *testing.T) {
	lang := &Language{
		ExternalTokenCount: 3,
		ExternalScannerStates: []bool{
			// State 0: no tokens enabled (initial)
			false, false, false,
			// State 1: token 0 and 2 enabled
			true, false, true,
			// State 2: only token 1 enabled
			false, true, false,
		},
	}

	// State 0 returns nil (no external tokens).
	result := lang.EnabledExternalTokens(0)
	if result != nil {
		t.Errorf("state 0: expected nil, got %v", result)
	}

	// State 1.
	result = lang.EnabledExternalTokens(1)
	if len(result) != 3 || result[0] != true || result[1] != false || result[2] != true {
		t.Errorf("state 1: expected [true,false,true], got %v", result)
	}

	// State 2.
	result = lang.EnabledExternalTokens(2)
	if len(result) != 3 || result[0] != false || result[1] != true || result[2] != false {
		t.Errorf("state 2: expected [false,true,false], got %v", result)
	}

	// Out of range.
	result = lang.EnabledExternalTokens(99)
	if result != nil {
		t.Errorf("state 99: expected nil, got %v", result)
	}

	// Zero external token count.
	lang2 := &Language{ExternalTokenCount: 0}
	result = lang2.EnabledExternalTokens(1)
	if result != nil {
		t.Errorf("zero token count: expected nil, got %v", result)
	}
}

// --- Mock external scanner for unit tests ---

type mockScanner struct {
	scanCalls       int
	serializeCalls  int
	deserializeCalls int
	lastValidSymbols []bool
	lastDeserializeData []byte
	scanResult      bool
	scanSymbol      Symbol
	state           byte // simple 1-byte state for testing
}

func (s *mockScanner) Scan(lexer *Lexer, validSymbols []bool) bool {
	s.scanCalls++
	s.lastValidSymbols = validSymbols
	if s.scanResult {
		// Advance at least one char, mark end, accept.
		if !lexer.EOF() {
			lexer.Advance(false)
		}
		lexer.MarkEnd()
		lexer.AcceptToken(s.scanSymbol)
		return true
	}
	return false
}

func (s *mockScanner) Serialize(buf []byte) uint32 {
	s.serializeCalls++
	if len(buf) > 0 {
		buf[0] = s.state
		return 1
	}
	return 0
}

func (s *mockScanner) Deserialize(data []byte) {
	s.deserializeCalls++
	s.lastDeserializeData = data
	s.state = 0
	if len(data) > 0 {
		s.state = data[0]
	}
}

// --- Parser external scanner lifecycle ---

func TestParserExternalScannerCreation(t *testing.T) {
	scanner := &mockScanner{}
	lang := &Language{
		SymbolCount:    5,
		TokenCount:     3,
		StateCount:     2,
		LargeStateCount: 2,
		SymbolMetadata: make([]SymbolMetadata, 5),
		SymbolNames:    make([]string, 5),
		ParseTable:     make([]uint16, 10),
		LexModes:       make([]LexMode, 2),
		ParseActions:   []ParseActionEntry{{Type: ParseActionTypeHeader}},
		NewExternalScanner: func() ExternalScanner { return scanner },
	}

	p := NewParser()
	p.SetLanguage(lang)

	if p.externalScanner == nil {
		t.Fatal("expected external scanner to be created")
	}
	if p.externalScanner != scanner {
		t.Error("expected scanner to match factory output")
	}
}

func TestParserNoExternalScanner(t *testing.T) {
	lang := &Language{
		SymbolCount:    5,
		TokenCount:     3,
		StateCount:     2,
		LargeStateCount: 2,
		SymbolMetadata: make([]SymbolMetadata, 5),
		SymbolNames:    make([]string, 5),
		ParseTable:     make([]uint16, 10),
		LexModes:       make([]LexMode, 2),
		ParseActions:   []ParseActionEntry{{Type: ParseActionTypeHeader}},
	}

	p := NewParser()
	p.SetLanguage(lang)

	if p.externalScanner != nil {
		t.Error("expected nil scanner for language without factory")
	}
}

func TestParserSwitchLanguageClearsScanner(t *testing.T) {
	scanner1 := &mockScanner{}
	scanner2 := &mockScanner{}

	lang1 := &Language{
		SymbolCount:    5,
		TokenCount:     3,
		StateCount:     2,
		LargeStateCount: 2,
		SymbolMetadata: make([]SymbolMetadata, 5),
		SymbolNames:    make([]string, 5),
		ParseTable:     make([]uint16, 10),
		LexModes:       make([]LexMode, 2),
		ParseActions:   []ParseActionEntry{{Type: ParseActionTypeHeader}},
		NewExternalScanner: func() ExternalScanner { return scanner1 },
	}
	lang2 := &Language{
		SymbolCount:    5,
		TokenCount:     3,
		StateCount:     2,
		LargeStateCount: 2,
		SymbolMetadata: make([]SymbolMetadata, 5),
		SymbolNames:    make([]string, 5),
		ParseTable:     make([]uint16, 10),
		LexModes:       make([]LexMode, 2),
		ParseActions:   []ParseActionEntry{{Type: ParseActionTypeHeader}},
		NewExternalScanner: func() ExternalScanner { return scanner2 },
	}

	p := NewParser()
	p.SetLanguage(lang1)
	if p.externalScanner != scanner1 {
		t.Error("expected scanner1")
	}

	p.SetLanguage(lang2)
	if p.externalScanner != scanner2 {
		t.Error("expected scanner2 after language switch")
	}
}

// --- HeredocScanner unit tests ---

func TestHeredocScannerSerializeDeserialize(t *testing.T) {
	scanner := &testHeredocScanner{}

	// Default state.
	buf := make([]byte, 10)
	n := scanner.Serialize(buf)
	if n != 1 || buf[0] != 0 {
		t.Errorf("default state: expected [0], got buf[0]=%d, n=%d", buf[0], n)
	}

	// After marking seen.
	scanner.markerSeen = true
	n = scanner.Serialize(buf)
	if n != 1 || buf[0] != 1 {
		t.Errorf("after seen: expected [1], got buf[0]=%d, n=%d", buf[0], n)
	}

	// Deserialize restores.
	scanner.markerSeen = false
	scanner.Deserialize([]byte{1})
	if !scanner.markerSeen {
		t.Error("expected markerSeen after deserialize [1]")
	}

	// Deserialize empty resets.
	scanner.Deserialize(nil)
	if scanner.markerSeen {
		t.Error("expected !markerSeen after deserialize nil")
	}
}

func TestHeredocScannerRoundtrip(t *testing.T) {
	scanner1 := &testHeredocScanner{markerSeen: true}
	buf := make([]byte, TreeSitterSerializationBufferSize)
	n := scanner1.Serialize(buf)

	scanner2 := &testHeredocScanner{}
	scanner2.Deserialize(buf[:n])

	if scanner1.markerSeen != scanner2.markerSeen {
		t.Errorf("state mismatch after roundtrip: %v vs %v", scanner1.markerSeen, scanner2.markerSeen)
	}
}

// testHeredocScanner is a local implementation of the heredoc external scanner
// for unit testing within the treesitter package.
type testHeredocScanner struct {
	markerSeen bool
}

func (s *testHeredocScanner) Scan(lexer *Lexer, validSymbols []bool) bool {
	if len(validSymbols) == 0 || !validSymbols[0] {
		return false
	}

	// Skip initial newline after <<.
	if !lexer.EOF() && lexer.Lookahead == '\n' {
		lexer.Advance(true)
	}

	atLineStart := true
	bodyLen := 0

	for !lexer.EOF() {
		ch := lexer.Lookahead
		if atLineStart && ch == 'E' {
			lexer.MarkEnd()
			lexer.Advance(false)
			if !lexer.EOF() && lexer.Lookahead == 'N' {
				lexer.Advance(false)
				if !lexer.EOF() && lexer.Lookahead == 'D' {
					lexer.Advance(false)
					if lexer.EOF() || lexer.Lookahead == '\n' {
						if !lexer.EOF() {
							lexer.Advance(false)
						}
						lexer.MarkEnd()
						lexer.AcceptToken(0)
						s.markerSeen = true
						return true
					}
				}
			}
			atLineStart = false
			bodyLen++
			continue
		}
		if ch == '\n' {
			atLineStart = true
		} else {
			atLineStart = false
		}
		lexer.Advance(false)
		bodyLen++
	}

	if bodyLen == 0 {
		return false
	}
	lexer.MarkEnd()
	lexer.AcceptToken(0)
	s.markerSeen = true
	return true
}

func (s *testHeredocScanner) Serialize(buf []byte) uint32 {
	if len(buf) < 1 {
		return 0
	}
	if s.markerSeen {
		buf[0] = 1
	} else {
		buf[0] = 0
	}
	return 1
}

func (s *testHeredocScanner) Deserialize(data []byte) {
	s.markerSeen = false
	if len(data) >= 1 && data[0] == 1 {
		s.markerSeen = true
	}
}

func TestHeredocScannerLexing(t *testing.T) {
	scanner := &testHeredocScanner{}
	lexer := NewLexer()
	lexer.SetInput(NewStringInput([]byte("hello\nworld\nEND\n")))
	lexer.Start(LengthZero)

	validSymbols := []bool{true}
	found := scanner.Scan(lexer, validSymbols)
	if !found {
		t.Fatal("expected scanner to find heredoc body")
	}
	if !scanner.markerSeen {
		t.Error("expected markerSeen after scan")
	}
}

func TestHeredocScannerNoValidSymbols(t *testing.T) {
	scanner := &testHeredocScanner{}
	lexer := NewLexer()
	lexer.SetInput(NewStringInput([]byte("hello\nEND\n")))
	lexer.Start(LengthZero)

	// Empty valid symbols.
	found := scanner.Scan(lexer, []bool{})
	if found {
		t.Error("expected false with empty valid symbols")
	}

	// Valid symbol disabled.
	found = scanner.Scan(lexer, []bool{false})
	if found {
		t.Error("expected false with disabled valid symbol")
	}
}

// --- Stack external token tracking ---

func TestStackExternalTokenTracking(t *testing.T) {
	arena := NewSubtreeArena(0)
	arena.Alloc() // skip zero-offset
	stack := NewStack(arena)
	v := stack.AddVersion(1, LengthZero)

	// Initially zero.
	token := stack.LastExternalToken(v)
	if !token.IsZero() {
		t.Error("expected zero initial external token")
	}

	// Set a token.
	st, data := arena.Alloc()
	data.Symbol = 5
	data.SetFlag(SubtreeFlagHasExternalTokens, true)
	stack.SetLastExternalToken(v, st)

	token = stack.LastExternalToken(v)
	if token.IsZero() {
		t.Error("expected non-zero token")
	}
	if GetSymbol(token, arena) != 5 {
		t.Errorf("expected symbol 5, got %d", GetSymbol(token, arena))
	}
}

func TestStackSplitPreservesExternalToken(t *testing.T) {
	arena := NewSubtreeArena(0)
	arena.Alloc() // skip zero-offset
	stack := NewStack(arena)
	v := stack.AddVersion(1, LengthZero)

	st, data := arena.Alloc()
	data.Symbol = 5
	data.SetFlag(SubtreeFlagHasExternalTokens, true)
	stack.SetLastExternalToken(v, st)

	// Split should preserve the external token.
	v2 := stack.Split(v)
	token := stack.LastExternalToken(v2)
	if token.IsZero() {
		t.Error("expected split to preserve external token")
	}
	if GetSymbol(token, arena) != 5 {
		t.Errorf("expected symbol 5 on split version, got %d", GetSymbol(token, arena))
	}
}

// --- Serialization buffer size ---

func TestSerializationBufferSize(t *testing.T) {
	if TreeSitterSerializationBufferSize != 1024 {
		t.Errorf("expected buffer size 1024, got %d", TreeSitterSerializationBufferSize)
	}
}
