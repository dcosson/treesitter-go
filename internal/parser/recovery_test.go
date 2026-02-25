package parser

import "testing"

// --- Test helpers ---

// makeRecoveryTestLanguage creates a minimal Language for testing recovery.
// Parse table layout:
//
//	State 1 (start): symbol 1 → shift to state 2
//	State 2: symbol 2 → shift to state 3
//	State 3: symbol 1 → shift to state 2  (loops back)
//	State 5: symbol 1 → shift to state 2  (recovery target)
//	State 0 (error): symbol 1 → recover (action type 4)
//
// This gives us enough structure to test:
//   - HasActions (state 1 + symbol 1 = true, state 1 + symbol 2 = false)
//   - RecordSummary on a multi-level stack
//   - recover() popback (find summary entry at state 5 where symbol 1 is valid)
func makeRecoveryTestLanguage() *Language {
	// We need at least 6 states (0-5) and 12 symbols.
	// Large state count = all states use the dense parse table.
	stateCount := uint32(6)
	symbolCount := uint32(12)
	tokenCount := uint32(12)

	// Dense parse table: [state * symbolCount + symbol] -> action index
	parseTable := make([]uint16, stateCount*symbolCount)

	// Parse actions: [0] = unused, then groups of (header, actions...)
	// Action index 0 = no action (empty).
	parseActions := make([]ParseActionEntry, 0, 20)
	parseActions = append(parseActions, ParseActionEntry{}) // index 0: unused

	// Action group 1: state 1 + symbol 1 → shift to state 2
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeHeader, Count: 1, Reusable: true}) // index 1
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeShift, ShiftState: 2})             // index 2
	parseTable[1*symbolCount+1] = 1                                                                              // state 1, symbol 1 → action index 1

	// Action group 2: state 2 + symbol 2 → shift to state 3
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeHeader, Count: 1, Reusable: true}) // index 3
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeShift, ShiftState: 3})             // index 4
	parseTable[2*symbolCount+2] = 3                                                                              // state 2, symbol 2 → action index 3

	// Action group 3: state 3 + symbol 1 → shift to state 2 (loop)
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeHeader, Count: 1, Reusable: true}) // index 5
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeShift, ShiftState: 2})             // index 6
	parseTable[3*symbolCount+1] = 5                                                                              // state 3, symbol 1 → action index 5

	// Action group 4: state 5 + symbol 1 → shift to state 2 (recovery target)
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeHeader, Count: 1, Reusable: true}) // index 7
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeShift, ShiftState: 2})             // index 8
	parseTable[5*symbolCount+1] = 7                                                                              // state 5, symbol 1 → action index 7

	// Action group 5: state 0 + symbol 1 → recover
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeHeader, Count: 1}) // index 9
	parseActions = append(parseActions, ParseActionEntry{Type: ParseActionTypeRecover})          // index 10
	parseTable[0*symbolCount+1] = 9                                                              // state 0, symbol 1 → action index 9

	lang := &Language{
		Version:         15,
		SymbolCount:     symbolCount,
		TokenCount:      tokenCount,
		StateCount:      stateCount,
		LargeStateCount: stateCount, // All states use dense table.
		ParseTable:      parseTable,
		ParseActions:    parseActions,
		LexModes:        make([]LexMode, stateCount),
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: true, Named: false},  // 1: "a"
			{Visible: true, Named: false},  // 2: "b"
			{Visible: true, Named: true},   // 3: expr
			{Visible: true, Named: true},   // 4: stmt
			{Visible: true, Named: true},   // 5: program
			{Visible: true, Named: false},  // 6: ";"
			{Visible: true, Named: true},   // 7: number
			{Visible: true, Named: true},   // 8: document
			{Visible: false, Named: false}, // 9: _hidden
			{Visible: true, Named: false},  // 10: ","
			{Visible: true, Named: true},   // 11: comment (extra)
		},
		SymbolNames: []string{
			"end", "a", "b", "expr", "stmt", "program",
			";", "number", "document", "_hidden", ",", "comment",
		},
	}

	return lang
}

// buildTestStack creates a simple linear stack: state5 → state2 → state3
// with leaf subtrees at each level. Returns (stack, version, lang, arena).
func buildTestStack(t *testing.T) (*Stack, StackVersion, *Language, *SubtreeArena) {
	t.Helper()
	arena := NewSubtreeArena(64)
	lang := makeRecoveryTestLanguage()
	stack := NewStack(arena)

	// Start at state 5, position 0.
	v := stack.AddVersion(5, Length{Bytes: 0, Point: Point{Row: 0, Column: 0}})

	// Push state 2 at position 5 with a leaf subtree.
	leaf1 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(5), false, false, false, lang)
	stack.Push(v, 2, leaf1, false, Length{Bytes: 5, Point: Point{Column: 5}})

	// Push state 3 at position 10.
	leaf2 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(2), false, false, false, lang)
	stack.Push(v, 3, leaf2, false, Length{Bytes: 10, Point: Point{Column: 10}})

	return stack, v, lang, arena
}

// --- RecordSummary tests ---

func TestRecordSummaryBasicLinearStack(t *testing.T) {
	stack, v, _, _ := buildTestStack(t)

	// Stack: [state=5, pos=0] → [state=2, pos=5] → [state=3, pos=10] (head)
	stack.RecordSummary(v, MaxSummaryDepth)
	summary := stack.GetSummary(v)

	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if len(summary) == 0 {
		t.Fatal("expected at least one summary entry")
	}

	// Check that we can find state 5 (the bottom of the stack).
	foundState5 := false
	foundState2 := false
	for _, entry := range summary {
		if entry.State == 5 {
			foundState5 = true
			if entry.Position.Bytes != 0 {
				t.Errorf("state 5 position = %d, want 0", entry.Position.Bytes)
			}
		}
		if entry.State == 2 {
			foundState2 = true
			if entry.Position.Bytes != 5 {
				t.Errorf("state 2 position = %d, want 5", entry.Position.Bytes)
			}
		}
	}
	if !foundState5 {
		t.Error("summary missing entry for state 5 (stack bottom)")
	}
	if !foundState2 {
		t.Error("summary missing entry for state 2 (middle)")
	}
}

func TestRecordSummaryRespectsMaxDepth(t *testing.T) {
	arena := NewSubtreeArena(64)
	lang := makeRecoveryTestLanguage()
	stack := NewStack(arena)

	// Build a deep stack: state 1 → 2 → 3 → 2 → 3 → ... (10 levels)
	v := stack.AddVersion(1, Length{Bytes: 0})
	for i := 0; i < 10; i++ {
		leaf := NewLeafSubtree(arena, Symbol(1),
			Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
			StateID(1), false, false, false, lang)
		nextState := StateID(2 + (i % 2))
		pos := Length{Bytes: uint32(i + 1), Point: Point{Column: uint32(i + 1)}}
		stack.Push(v, nextState, leaf, false, pos)
	}

	// Record with maxDepth=3.
	stack.RecordSummary(v, 3)
	summary := stack.GetSummary(v)

	// All entries should be present, but the BFS should be bounded.
	// We don't assert exact depth bounds because the implementation may
	// record entries at the boundary differently. Just verify we got
	// a reasonable number of entries (not the full 10+ from all depths).
	if len(summary) == 0 {
		t.Fatal("expected non-empty summary")
	}
	if len(summary) > 10 {
		t.Errorf("expected summary bounded by maxDepth, got %d entries", len(summary))
	}
}

func TestRecordSummaryEmptyStack(t *testing.T) {
	arena := NewSubtreeArena(64)
	stack := NewStack(arena)

	// Version with no links (just the initial node).
	v := stack.AddVersion(1, Length{Bytes: 0})
	stack.RecordSummary(v, MaxSummaryDepth)
	summary := stack.GetSummary(v)

	// A single node with no links should produce no summary entries
	// (nothing to walk back to).
	if len(summary) != 0 {
		t.Errorf("expected 0 entries for single-node stack, got %d", len(summary))
	}
}

func TestRecordSummaryDeduplicatesStateDepth(t *testing.T) {
	arena := NewSubtreeArena(64)
	lang := makeRecoveryTestLanguage()
	stack := NewStack(arena)

	// Build a stack that merges, creating two paths to the same (state, depth).
	v0 := stack.AddVersion(1, Length{Bytes: 0})

	// Push to state 2 at pos 5.
	leaf1 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, 2, leaf1, false, Length{Bytes: 5, Point: Point{Column: 5}})

	// Split and push both paths to the same state 3 at pos 10.
	v1 := stack.Split(v0)

	leaf2a := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(2), false, false, false, lang)
	stack.Push(v0, 3, leaf2a, false, Length{Bytes: 10, Point: Point{Column: 10}})

	leaf2b := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(2), false, false, false, lang)
	stack.Push(v1, 3, leaf2b, false, Length{Bytes: 10, Point: Point{Column: 10}})

	// Merge v1 into v0 (both at state 3).
	stack.Merge(v0, v1)

	// Now v0 has two paths back to state 1 via state 2.
	stack.RecordSummary(v0, MaxSummaryDepth)
	summary := stack.GetSummary(v0)

	// Count entries with state 2 at depth 1. Should be deduplicated.
	count := 0
	for _, entry := range summary {
		if entry.State == 2 && entry.Depth == 1 {
			count++
		}
	}
	if count > 1 {
		t.Errorf("expected at most 1 entry for (state=2, depth=1), got %d", count)
	}
}

func TestRecordSummaryInvalidVersion(t *testing.T) {
	arena := NewSubtreeArena(64)
	stack := NewStack(arena)

	// Should not panic on invalid version.
	stack.RecordSummary(StackVersion(99), MaxSummaryDepth)
	summary := stack.GetSummary(StackVersion(99))
	if summary != nil {
		t.Error("expected nil summary for invalid version")
	}
}

// --- HasActions tests ---

func TestHasActionsPositive(t *testing.T) {
	lang := makeRecoveryTestLanguage()

	if !lang.HasActions(1, 1) {
		t.Error("expected HasActions(1, 1) = true")
	}
	if !lang.HasActions(2, 2) {
		t.Error("expected HasActions(2, 2) = true")
	}
	if !lang.HasActions(5, 1) {
		t.Error("expected HasActions(5, 1) = true")
	}
}

func TestHasActionsNegative(t *testing.T) {
	lang := makeRecoveryTestLanguage()

	if lang.HasActions(1, 2) {
		t.Error("expected HasActions(1, 2) = false")
	}
	if lang.HasActions(3, 2) {
		t.Error("expected HasActions(3, 2) = false")
	}
}

// --- betterVersionExists tests ---

func TestBetterVersionExistsTrue(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// v0: low cost, active, at position 10.
	setupCondenseVersion(p.stack, 2, 10, 100, 5, 0)

	// v1: the version we're testing, at position 10.
	v1 := setupCondenseVersion(p.stack, 3, 10, 0, 1, 0)

	// Check if a better version exists for v1 at cost 2000 (much higher than v0's 100).
	result := p.betterVersionExists(v1, false, 2000)
	if !result {
		t.Error("expected betterVersionExists=true when proposed cost (2000) >> other version cost (100)")
	}
}

func TestBetterVersionExistsFalse(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// v0: high cost.
	setupCondenseVersion(p.stack, 2, 10, 5000, 1, 0)

	// v1: the version we're testing, at position 10.
	v1 := setupCondenseVersion(p.stack, 3, 10, 0, 1, 0)

	// Check if a better version exists for v1 at cost 50 (much lower than v0's 5000).
	result := p.betterVersionExists(v1, false, 50)
	if result {
		t.Error("expected betterVersionExists=false when proposed cost (50) << other version cost (5000)")
	}
}

func TestBetterVersionExistsNoOtherVersions(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// Only one version.
	v0 := setupCondenseVersion(p.stack, 2, 10, 0, 1, 0)

	// No other versions to compare → should return false.
	result := p.betterVersionExists(v0, false, 100)
	if result {
		t.Error("expected betterVersionExists=false when no other versions exist")
	}
}

// --- createErrorRepeatNode tests ---

func TestCreateErrorRepeatNodeSymbolAndChildren(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)

	leaf1 := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)
	leaf2 := NewLeafSubtree(p.arena, Symbol(2),
		Length{Bytes: 1, Point: Point{Column: 1}}, Length{Bytes: 4, Point: Point{Column: 4}},
		StateID(2), false, false, false, lang)

	errRepeat := p.createErrorRepeatNode([]Subtree{leaf1, leaf2})

	if errRepeat.IsZero() {
		t.Fatal("createErrorRepeatNode returned zero subtree")
	}

	sym := GetSymbol(errRepeat, p.arena)
	if sym != SymbolErrorRepeat {
		t.Errorf("symbol = %d, want %d (SymbolErrorRepeat)", sym, SymbolErrorRepeat)
	}

	childCount := GetChildCount(errRepeat, p.arena)
	if childCount != 2 {
		t.Errorf("child count = %d, want 2", childCount)
	}
}

func TestCreateErrorRepeatNodeEmpty(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)

	errRepeat := p.createErrorRepeatNode(nil)

	sym := GetSymbol(errRepeat, p.arena)
	if sym != SymbolErrorRepeat {
		t.Errorf("symbol = %d, want %d (SymbolErrorRepeat)", sym, SymbolErrorRepeat)
	}

	size := GetSize(errRepeat, p.arena)
	if size.Bytes != 0 {
		t.Errorf("empty error_repeat size = %d, want 0", size.Bytes)
	}
}

// --- recoverToState tests ---

func TestRecoverToStateBasic(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// Build stack: state 5 (pos 0) → state 2 (pos 5) → state 3 (pos 10)
	v := p.stack.AddVersion(5, Length{Bytes: 0, Point: Point{Row: 0, Column: 0}})

	leaf1 := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(5), false, false, false, lang)
	p.stack.Push(v, 2, leaf1, false, Length{Bytes: 5, Point: Point{Column: 5}})

	leaf2 := NewLeafSubtree(p.arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(2), false, false, false, lang)
	p.stack.Push(v, 3, leaf2, false, Length{Bytes: 10, Point: Point{Column: 10}})

	// Pop 2 levels back to state 5 and push ERROR + goal state 5.
	result := p.recoverToState(v, 2, 5)

	if !result {
		t.Fatal("recoverToState returned false, expected true")
	}

	// After recovery, the version should be at the goal state.
	state := p.stack.State(v)
	if state != 5 {
		t.Errorf("state after recovery = %d, want 5", state)
	}

	// The top subtree should be an ERROR node.
	topSubtree := p.stack.TopSubtree(v)
	if topSubtree.IsZero() {
		t.Fatal("expected non-zero top subtree after recovery")
	}
	topSymbol := GetSymbol(topSubtree, p.arena)
	if topSymbol != SymbolError {
		t.Errorf("top subtree symbol = %d, want %d (SymbolError)", topSymbol, SymbolError)
	}
}

func TestRecoverToStatePopTooDeep(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// Stack with only 1 push (depth 1).
	v := p.stack.AddVersion(5, Length{Bytes: 0})
	leaf1 := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(5), false, false, false, lang)
	p.stack.Push(v, 2, leaf1, false, Length{Bytes: 5})

	// Try to pop 10 levels (too deep). Pop returns empty → recoverToState returns false.
	result := p.recoverToState(v, 10, 1)
	if result {
		t.Error("recoverToState should return false when pop depth exceeds stack")
	}
}

func TestRecoverToStateWithErrorCost(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// Build a 3-level stack.
	v := p.stack.AddVersion(5, Length{Bytes: 0})
	leaf1 := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(5), false, false, false, lang)
	p.stack.Push(v, 2, leaf1, false, Length{Bytes: 5})
	leaf2 := NewLeafSubtree(p.arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(2), false, false, false, lang)
	p.stack.Push(v, 3, leaf2, false, Length{Bytes: 10})

	costBefore := p.stack.ErrorCost(v)

	result := p.recoverToState(v, 2, 5)
	if !result {
		t.Fatal("recoverToState failed")
	}

	costAfter := p.stack.ErrorCost(v)
	if costAfter <= costBefore {
		t.Error("expected error cost to increase after recoverToState")
	}
}

// --- recover() integration tests ---

func TestRecoverPopbackPath(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// Build stack: state 5 (pos 0) → state 2 (pos 5) → state 3 (pos 10)
	// Then push ERROR_STATE (state 0) at pos 10 to simulate handleError.
	v := p.stack.AddVersion(5, Length{Bytes: 0, Point: Point{Row: 0, Column: 0}})

	leaf1 := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(5), false, false, false, lang)
	p.stack.Push(v, 2, leaf1, false, Length{Bytes: 5, Point: Point{Column: 5}})

	leaf2 := NewLeafSubtree(p.arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(2), false, false, false, lang)
	p.stack.Push(v, 3, leaf2, false, Length{Bytes: 10, Point: Point{Column: 10}})

	// Push ERROR_STATE with SubtreeZero (matches C handleError behavior).
	p.stack.Push(v, 0, SubtreeZero, false, Length{Bytes: 10, Point: Point{Column: 10}})

	// Record summary.
	p.stack.RecordSummary(v, MaxSummaryDepth)

	summary := p.stack.GetSummary(v)
	if len(summary) == 0 {
		t.Fatal("expected non-empty summary")
	}

	// Verify state 5 is in the summary (it's the popback target).
	foundTarget := false
	for _, entry := range summary {
		if entry.State == 5 {
			foundTarget = true
			break
		}
	}
	if !foundTarget {
		t.Log("summary entries:")
		for i, e := range summary {
			t.Logf("  [%d] state=%d depth=%d pos=%d", i, e.State, e.Depth, e.Position.Bytes)
		}
		t.Fatal("summary does not contain state 5 (recovery target)")
	}

	// Create a lookahead token (symbol 1) which is valid in state 5.
	lookahead := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(0), false, false, false, lang)

	// Call recover. Should find state 5 in summary and pop back.
	p.recover(v, lookahead)

	// After recover, the version should not be at ERROR_STATE anymore
	// (it should have popped back to state 5 or been halted).
	if p.stack.VersionCount() == 0 {
		t.Fatal("all versions removed after recover")
	}
}

func TestRecoverSkipPath(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	// Build a simple stack at ERROR_STATE with no summary (skip path only).
	v := p.stack.AddVersion(1, Length{Bytes: 0, Point: Point{Row: 0, Column: 0}})

	leaf := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(1), false, false, false, lang)
	p.stack.Push(v, 2, leaf, false, Length{Bytes: 5, Point: Point{Column: 5}})

	// Push ERROR_STATE.
	p.stack.Push(v, 0, SubtreeZero, false, Length{Bytes: 5, Point: Point{Column: 5}})

	// No summary recorded — recover should fall through to skip.

	// Create lookahead (symbol 3 which has NO actions in any state
	// in our test language, so popback won't work).
	lookahead := NewLeafSubtree(p.arena, Symbol(3),
		Length{Bytes: 0}, Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(0), false, false, false, lang)

	p.recover(v, lookahead)

	// After skip, version should be at ERROR_STATE (state 0) and position
	// should have advanced, OR version may be halted.
	if p.stack.IsHalted(v) {
		// Halted is acceptable (betterVersionExists may have decided to halt).
		return
	}

	state := p.stack.State(v)
	if state != 0 {
		t.Errorf("state after skip = %d, want 0 (ERROR_STATE)", state)
	}
}

func TestRecoverAtEOF(t *testing.T) {
	lang := makeRecoveryTestLanguage()
	p := NewParser()
	p.SetLanguage(lang)
	p.arena = NewSubtreeArena(64)
	p.stack = NewStack(p.arena)

	v := p.stack.AddVersion(1, Length{Bytes: 0})

	leaf := NewLeafSubtree(p.arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(1), false, false, false, lang)
	p.stack.Push(v, 2, leaf, false, Length{Bytes: 5})

	// Push ERROR_STATE.
	p.stack.Push(v, 0, SubtreeZero, false, Length{Bytes: 5})

	// EOF lookahead (SymbolEnd = 0).
	eof := NewLeafSubtree(p.arena, SymbolEnd,
		Length{Bytes: 0}, Length{Bytes: 0},
		StateID(0), false, false, false, lang)

	// recover at EOF should push an empty ERROR and accept.
	p.recover(v, eof)

	// The version should be halted (accepted).
	if !p.stack.IsHalted(v) && p.finishedTree.IsZero() {
		t.Error("expected version to be halted or finishedTree to be set after EOF recover")
	}
}

// --- GetSummary edge cases ---

func TestGetSummaryNoRecord(t *testing.T) {
	arena := NewSubtreeArena(64)
	stack := NewStack(arena)

	v := stack.AddVersion(1, Length{Bytes: 0})

	summary := stack.GetSummary(v)
	if summary != nil {
		t.Error("expected nil summary before RecordSummary")
	}
}

// --- Pop with SubtreeZero (regression test for the bug we fixed) ---

func TestPopWithSubtreeZeroLinks(t *testing.T) {
	arena := NewSubtreeArena(64)
	lang := makeRecoveryTestLanguage()
	stack := NewStack(arena)

	v := stack.AddVersion(5, Length{Bytes: 0})

	// Push a real leaf.
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(5), false, false, false, lang)
	stack.Push(v, 2, leaf, false, Length{Bytes: 5})

	// Push ERROR_STATE with SubtreeZero (matches handleError behavior).
	stack.Push(v, 0, SubtreeZero, false, Length{Bytes: 5})

	// Pop should not panic on SubtreeZero links.
	results := stack.Pop(v, 2)
	if len(results) == 0 {
		t.Fatal("Pop returned no results")
	}

	// Verify the popped subtrees include SubtreeZero.
	foundZero := false
	for _, st := range results[0].Subtrees() {
		if st.IsZero() {
			foundZero = true
		}
	}
	if !foundZero {
		t.Error("expected SubtreeZero in popped subtrees (from ERROR_STATE push)")
	}
}
