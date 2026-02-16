package treesitter

import (
	"testing"
)

// makeTestLanguage creates a minimal language for testing parse table lookup.
// It has 5 symbols, 4 states (2 large, 2 small), and some parse actions.
func makeTestLanguage() *Language {
	// Symbols: 0=end, 1='{', 2='}', 3=value, 4=document
	symbolCount := uint32(5)

	// Parse actions table (flat array):
	// Index 0: empty (lookup returns 0 for no action)
	// Index 1: header(count=1, reusable=false) + shift(state=2)
	// Index 2: header(count=1, reusable=true) + reduce(symbol=4, children=1)
	// Index 3: header(count=1, reusable=false) + accept
	// Index 4: header(count=1, reusable=false) + shift(state=3)
	parseActions := []ParseActionEntry{
		// [0]: sentinel/empty
		{Type: ParseActionTypeHeader, Count: 0},
		// [1]: shift to state 2
		{Type: ParseActionTypeHeader, Count: 1, Reusable: false},
		{Type: ParseActionTypeShift, ShiftState: 2},
		// [3]: reduce to symbol 4, consuming 1 child
		{Type: ParseActionTypeHeader, Count: 1, Reusable: true},
		{Type: ParseActionTypeReduce, ReduceSymbol: 4, ReduceChildCount: 1, ReduceProdID: 1},
		// [5]: accept
		{Type: ParseActionTypeHeader, Count: 1, Reusable: false},
		{Type: ParseActionTypeAccept},
		// [7]: shift to state 3
		{Type: ParseActionTypeHeader, Count: 1, Reusable: false},
		{Type: ParseActionTypeShift, ShiftState: 3},
	}

	// Large state table (2 large states):
	// State 0: symbol 1 ('{') -> action index 1
	// State 1: symbol 4 (document) -> action index 5 (accept), symbol 3 (value) -> action index 7
	parseTable := make([]uint16, 2*symbolCount)
	// State 0, symbol 1 -> action 1
	parseTable[0*symbolCount+1] = 1
	// State 1, symbol 4 -> action 5
	parseTable[1*symbolCount+4] = 5
	// State 1, symbol 3 -> action 7
	parseTable[1*symbolCount+3] = 7

	// Small state table (2 small states mapped at state 2 and state 3):
	// State 2 (small index 0): symbol 2 ('}') -> action index 3
	// State 3 (small index 1): symbol 0 (end) -> action index 5
	smallParseTable := []uint16{
		// State 2 (small index 0, at offset 0):
		1,    // groupCount = 1
		3, 1, // group: value=3, symCount=1
		2,    // symbol 2 ('}')
		// State 3 (small index 1, at offset 4):
		1,    // groupCount = 1
		5, 1, // group: value=5, symCount=1
		0,    // symbol 0 (end)
	}
	smallParseTableMap := []uint32{0, 4} // small state 0 -> offset 0, small state 1 -> offset 4

	return &Language{
		SymbolCount:     symbolCount,
		StateCount:      4,
		LargeStateCount: 2,
		ParseTable:      parseTable,
		SmallParseTable: smallParseTable,
		SmallParseTableMap: smallParseTableMap,
		ParseActions:    parseActions,
		SymbolNames:     []string{"end", "{", "}", "value", "document"},
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // end
			{Visible: true, Named: false},  // {
			{Visible: true, Named: false},  // }
			{Visible: true, Named: true},   // value
			{Visible: true, Named: true},   // document
		},
	}
}

func TestLanguageLookupLargeState(t *testing.T) {
	lang := makeTestLanguage()

	// State 0 (large), symbol 1 -> action index 1.
	got := lang.lookup(0, 1)
	if got != 1 {
		t.Errorf("lookup(0, 1) = %d, want 1", got)
	}

	// State 0 (large), symbol 0 -> no action (0).
	got = lang.lookup(0, 0)
	if got != 0 {
		t.Errorf("lookup(0, 0) = %d, want 0", got)
	}

	// State 1 (large), symbol 4 -> action index 5.
	got = lang.lookup(1, 4)
	if got != 5 {
		t.Errorf("lookup(1, 4) = %d, want 5", got)
	}

	// State 1 (large), symbol 3 -> action index 7.
	got = lang.lookup(1, 3)
	if got != 7 {
		t.Errorf("lookup(1, 3) = %d, want 7", got)
	}
}

func TestLanguageLookupSmallState(t *testing.T) {
	lang := makeTestLanguage()

	// State 2 (small index 0), symbol 2 -> action index 3.
	got := lang.lookup(2, 2)
	if got != 3 {
		t.Errorf("lookup(2, 2) = %d, want 3", got)
	}

	// State 2 (small index 0), symbol 0 -> no action (0).
	got = lang.lookup(2, 0)
	if got != 0 {
		t.Errorf("lookup(2, 0) = %d, want 0", got)
	}

	// State 3 (small index 1), symbol 0 -> action index 5.
	got = lang.lookup(3, 0)
	if got != 5 {
		t.Errorf("lookup(3, 0) = %d, want 5", got)
	}

	// State 3 (small index 1), symbol 1 -> no action (0).
	got = lang.lookup(3, 1)
	if got != 0 {
		t.Errorf("lookup(3, 1) = %d, want 0", got)
	}
}

func TestLanguageTableEntry(t *testing.T) {
	lang := makeTestLanguage()

	// Normal shift action.
	entry := lang.tableEntry(0, 1)
	if entry.ActionCount != 1 {
		t.Fatalf("tableEntry(0,1).ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Reusable {
		t.Error("tableEntry(0,1) should not be reusable")
	}
	if entry.Actions[0].Type != ParseActionTypeShift {
		t.Errorf("tableEntry(0,1) action type = %d, want shift", entry.Actions[0].Type)
	}
	if entry.Actions[0].ShiftState != 2 {
		t.Errorf("tableEntry(0,1) shift state = %d, want 2", entry.Actions[0].ShiftState)
	}

	// Reduce action (reusable).
	entry = lang.tableEntry(2, 2)
	if entry.ActionCount != 1 {
		t.Fatalf("tableEntry(2,2).ActionCount = %d, want 1", entry.ActionCount)
	}
	if !entry.Reusable {
		t.Error("tableEntry(2,2) should be reusable")
	}
	if entry.Actions[0].Type != ParseActionTypeReduce {
		t.Errorf("tableEntry(2,2) action type = %d, want reduce", entry.Actions[0].Type)
	}
	if entry.Actions[0].ReduceSymbol != 4 {
		t.Errorf("tableEntry(2,2) reduce symbol = %d, want 4", entry.Actions[0].ReduceSymbol)
	}

	// Accept action.
	entry = lang.tableEntry(1, 4)
	if entry.ActionCount != 1 {
		t.Fatalf("tableEntry(1,4).ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Actions[0].Type != ParseActionTypeAccept {
		t.Errorf("tableEntry(1,4) action type = %d, want accept", entry.Actions[0].Type)
	}

	// No action.
	entry = lang.tableEntry(0, 0)
	if entry.ActionCount != 0 {
		t.Errorf("tableEntry(0,0).ActionCount = %d, want 0", entry.ActionCount)
	}

	// Error symbol should return empty.
	entry = lang.tableEntry(0, SymbolError)
	if entry.ActionCount != 0 {
		t.Errorf("tableEntry(0, error).ActionCount = %d, want 0", entry.ActionCount)
	}
	entry = lang.tableEntry(0, SymbolErrorRepeat)
	if entry.ActionCount != 0 {
		t.Errorf("tableEntry(0, error_repeat).ActionCount = %d, want 0", entry.ActionCount)
	}
}

func TestLanguageNextState(t *testing.T) {
	lang := makeTestLanguage()

	// State 0, symbol 1 -> shift to state 2.
	got := lang.nextState(0, 1)
	if got != 2 {
		t.Errorf("nextState(0, 1) = %d, want 2", got)
	}

	// State 1, symbol 3 -> shift to state 3.
	got = lang.nextState(1, 3)
	if got != 3 {
		t.Errorf("nextState(1, 3) = %d, want 3", got)
	}

	// No action -> state 0.
	got = lang.nextState(0, 0)
	if got != 0 {
		t.Errorf("nextState(0, 0) = %d, want 0", got)
	}

	// Error symbol -> state 0.
	got = lang.nextState(0, SymbolError)
	if got != 0 {
		t.Errorf("nextState(0, error) = %d, want 0", got)
	}
}

func TestLanguageSymbolMetadata(t *testing.T) {
	lang := makeTestLanguage()

	tests := []struct {
		symbol  Symbol
		name    string
		visible bool
		named   bool
	}{
		{0, "end", false, false},
		{1, "{", true, false},
		{2, "}", true, false},
		{3, "value", true, true},
		{4, "document", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lang.SymbolName(tt.symbol); got != tt.name {
				t.Errorf("SymbolName(%d) = %q, want %q", tt.symbol, got, tt.name)
			}
			if got := lang.SymbolIsVisible(tt.symbol); got != tt.visible {
				t.Errorf("SymbolIsVisible(%d) = %v, want %v", tt.symbol, got, tt.visible)
			}
			if got := lang.SymbolIsNamed(tt.symbol); got != tt.named {
				t.Errorf("SymbolIsNamed(%d) = %v, want %v", tt.symbol, got, tt.named)
			}
		})
	}
}

func TestLanguageFieldMapForProduction(t *testing.T) {
	lang := &Language{
		FieldMapSlices: []FieldMapSlice{
			{Index: 0, Length: 0}, // prod 0: no fields
			{Index: 0, Length: 2}, // prod 1: 2 entries
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
			{FieldID: 2, ChildIndex: 1, Inherited: false},
		},
		FieldNames: []string{"", "key", "value"},
	}

	// Production 0: no fields.
	entries := lang.FieldMapForProduction(0)
	if len(entries) != 0 {
		t.Errorf("prod 0: got %d entries, want 0", len(entries))
	}

	// Production 1: 2 entries.
	entries = lang.FieldMapForProduction(1)
	if len(entries) != 2 {
		t.Fatalf("prod 1: got %d entries, want 2", len(entries))
	}
	if entries[0].FieldID != 1 {
		t.Errorf("entries[0].FieldID = %d, want 1", entries[0].FieldID)
	}
	if entries[1].FieldID != 2 {
		t.Errorf("entries[1].FieldID = %d, want 2", entries[1].FieldID)
	}

	// Out-of-range production.
	entries = lang.FieldMapForProduction(999)
	if entries != nil {
		t.Errorf("out-of-range: got %v, want nil", entries)
	}
}

func TestLanguageAliasForProduction(t *testing.T) {
	lang := &Language{
		MaxAliasSequenceLength: 3,
		AliasSequences: []Symbol{
			// prod 0: not used (prod 0 is skipped by the AliasForProduction check)
			0, 0, 0,
			// prod 1: alias child 0 to symbol 10, child 1 to symbol 11
			10, 11, 0,
		},
	}

	// prod 0 is always skipped.
	if got := lang.AliasForProduction(0, 0); got != 0 {
		t.Errorf("alias(0, 0) = %d, want 0", got)
	}

	// prod 1, child 0 -> symbol 10.
	if got := lang.AliasForProduction(1, 0); got != 10 {
		t.Errorf("alias(1, 0) = %d, want 10", got)
	}

	// prod 1, child 1 -> symbol 11.
	if got := lang.AliasForProduction(1, 1); got != 11 {
		t.Errorf("alias(1, 1) = %d, want 11", got)
	}

	// prod 1, child 2 -> 0 (no alias).
	if got := lang.AliasForProduction(1, 2); got != 0 {
		t.Errorf("alias(1, 2) = %d, want 0", got)
	}
}

func TestLanguageReservedWords(t *testing.T) {
	lang := &Language{
		ReservedWordCount:    3,
		ReservedWordSetCount: 2,
		ReservedWords: []bool{
			// set 0: tokens 0, 1, 2
			true, false, true,
			// set 1: tokens 0, 1, 2
			false, true, false,
		},
	}

	if !lang.IsReservedWord(0, 0) {
		t.Error("set 0, token 0 should be reserved")
	}
	if lang.IsReservedWord(0, 1) {
		t.Error("set 0, token 1 should not be reserved")
	}
	if !lang.IsReservedWord(0, 2) {
		t.Error("set 0, token 2 should be reserved")
	}
	if lang.IsReservedWord(1, 0) {
		t.Error("set 1, token 0 should not be reserved")
	}
	if !lang.IsReservedWord(1, 1) {
		t.Error("set 1, token 1 should be reserved")
	}

	// Out-of-range set.
	if lang.IsReservedWord(5, 0) {
		t.Error("out-of-range set should return false")
	}
}
