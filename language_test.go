package treesitter

import (
	"testing"
)

// makeTestLanguage creates a minimal language for testing parse table lookup.
// It has 5 symbols, 4 states (2 large, 2 small), and some parse actions.
//
// This follows the C tree-sitter encoding:
//   - Terminals (symbols < TokenCount): table entries are action indices into ParseActions
//   - Non-terminals (symbols >= TokenCount): table entries are raw state IDs (goto targets)
func makeTestLanguage() *Language {
	// Symbols: 0=end, 1='{', 2='}' (terminals); 3=value, 4=document (non-terminals)
	symbolCount := uint32(5)
	tokenCount := uint32(3) // 0, 1, 2 are terminals

	// Parse actions table (flat array):
	// Index 0: empty (lookup returns 0 for no action)
	// Index 1: header(count=1, reusable=false) + shift(state=2)
	// Index 3: header(count=1, reusable=true) + reduce(symbol=4, children=1)
	// Index 5: header(count=1, reusable=false) + accept
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
	}

	// Large state table (2 large states):
	// For terminal symbols (0-2): entries are action indices.
	// For non-terminal symbols (3-4): entries are raw state IDs (goto targets).
	parseTable := make([]uint16, 2*symbolCount)
	// State 0, symbol 1 ('{', terminal) -> action index 1
	parseTable[0*symbolCount+1] = 1
	// State 1, symbol 0 (end, terminal) -> action index 5 (accept)
	parseTable[1*symbolCount+0] = 5
	// State 1, symbol 3 (value, non-terminal) -> goto state 3 (raw state ID)
	parseTable[1*symbolCount+3] = 3

	// Small state table (2 small states mapped at state 2 and state 3):
	// State 2 (small index 0): symbol 2 ('}') -> action index 3
	// State 3 (small index 1): symbol 0 (end) -> action index 5
	smallParseTable := []uint16{
		// State 2 (small index 0, at offset 0):
		1,    // groupCount = 1
		3, 1, // group: value=3, symCount=1
		2, // symbol 2 ('}')
		// State 3 (small index 1, at offset 4):
		1,    // groupCount = 1
		5, 1, // group: value=5, symCount=1
		0, // symbol 0 (end)
	}
	smallParseTableMap := []uint32{0, 4} // small state 0 -> offset 0, small state 1 -> offset 4

	return &Language{
		SymbolCount:        symbolCount,
		TokenCount:         tokenCount,
		StateCount:         4,
		LargeStateCount:    2,
		ParseTable:         parseTable,
		SmallParseTable:    smallParseTable,
		SmallParseTableMap: smallParseTableMap,
		ParseActions:       parseActions,
		SymbolNames:        []string{"end", "{", "}", "value", "document"},
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

	// State 0 (large), symbol 1 (terminal) -> action index 1.
	got := lang.Lookup(0, 1)
	if got != 1 {
		t.Errorf("lookup(0, 1) = %d, want 1", got)
	}

	// State 0 (large), symbol 0 -> no action (0).
	got = lang.Lookup(0, 0)
	if got != 0 {
		t.Errorf("lookup(0, 0) = %d, want 0", got)
	}

	// State 1 (large), symbol 0 (terminal) -> action index 5 (accept).
	got = lang.Lookup(1, 0)
	if got != 5 {
		t.Errorf("lookup(1, 0) = %d, want 5", got)
	}

	// State 1 (large), symbol 3 (non-terminal) -> raw state ID 3 (goto target).
	got = lang.Lookup(1, 3)
	if got != 3 {
		t.Errorf("lookup(1, 3) = %d, want 3", got)
	}
}

func TestLanguageLookupSmallState(t *testing.T) {
	lang := makeTestLanguage()

	// State 2 (small index 0), symbol 2 -> action index 3.
	got := lang.Lookup(2, 2)
	if got != 3 {
		t.Errorf("lookup(2, 2) = %d, want 3", got)
	}

	// State 2 (small index 0), symbol 0 -> no action (0).
	got = lang.Lookup(2, 0)
	if got != 0 {
		t.Errorf("lookup(2, 0) = %d, want 0", got)
	}

	// State 3 (small index 1), symbol 0 -> action index 5.
	got = lang.Lookup(3, 0)
	if got != 5 {
		t.Errorf("lookup(3, 0) = %d, want 5", got)
	}

	// State 3 (small index 1), symbol 1 -> no action (0).
	got = lang.Lookup(3, 1)
	if got != 0 {
		t.Errorf("lookup(3, 1) = %d, want 0", got)
	}
}

func TestLanguageTableEntry(t *testing.T) {
	lang := makeTestLanguage()

	// Normal shift action.
	entry := lang.TableEntry(0, 1)
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
	entry = lang.TableEntry(2, 2)
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

	// Accept action (terminal symbol 0 = end).
	entry = lang.TableEntry(1, 0)
	if entry.ActionCount != 1 {
		t.Fatalf("tableEntry(1,0).ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Actions[0].Type != ParseActionTypeAccept {
		t.Errorf("tableEntry(1,0) action type = %d, want accept", entry.Actions[0].Type)
	}

	// No action.
	entry = lang.TableEntry(0, 0)
	if entry.ActionCount != 0 {
		t.Errorf("tableEntry(0,0).ActionCount = %d, want 0", entry.ActionCount)
	}

	// Error symbol should return empty.
	entry = lang.TableEntry(0, SymbolError)
	if entry.ActionCount != 0 {
		t.Errorf("tableEntry(0, error).ActionCount = %d, want 0", entry.ActionCount)
	}
	entry = lang.TableEntry(0, SymbolErrorRepeat)
	if entry.ActionCount != 0 {
		t.Errorf("tableEntry(0, error_repeat).ActionCount = %d, want 0", entry.ActionCount)
	}
}

func TestLanguageNextState(t *testing.T) {
	lang := makeTestLanguage()

	// State 0, symbol 1 (terminal '{') -> shift to state 2 (via action table).
	got := lang.NextState(0, 1)
	if got != 2 {
		t.Errorf("nextState(0, 1) = %d, want 2", got)
	}

	// State 1, symbol 3 (non-terminal 'value') -> goto state 3 (raw state ID).
	got = lang.NextState(1, 3)
	if got != 3 {
		t.Errorf("nextState(1, 3) = %d, want 3", got)
	}

	// No action -> state 0.
	got = lang.NextState(0, 0)
	if got != 0 {
		t.Errorf("nextState(0, 0) = %d, want 0", got)
	}

	// Error symbol -> state 0.
	got = lang.NextState(0, SymbolError)
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

func TestLanguagePublicSymbol(t *testing.T) {
	lang := &Language{
		PublicSymbolMap: []Symbol{
			0, // 0 -> 0 (identity)
			1, // 1 -> 1 (identity)
			1, // 2 -> 1 (sym__declare_scalar -> sym_scalar)
			3, // 3 -> 3 (identity)
			3, // 4 -> 3 (another variant)
		},
		SymbolNames: []string{"end", "scalar", "_declare_scalar", "array", "_declare_array"},
	}

	// Identity mapping.
	if got := lang.PublicSymbol(0); got != 0 {
		t.Errorf("PublicSymbol(0) = %d, want 0", got)
	}
	if got := lang.PublicSymbol(1); got != 1 {
		t.Errorf("PublicSymbol(1) = %d, want 1", got)
	}

	// Non-identity: internal variant -> public symbol.
	if got := lang.PublicSymbol(2); got != 1 {
		t.Errorf("PublicSymbol(2) = %d, want 1 (scalar)", got)
	}
	if got := lang.PublicSymbol(4); got != 3 {
		t.Errorf("PublicSymbol(4) = %d, want 3 (array)", got)
	}

	// SymbolErrorRepeat is always preserved.
	if got := lang.PublicSymbol(SymbolErrorRepeat); got != SymbolErrorRepeat {
		t.Errorf("PublicSymbol(SymbolErrorRepeat) = %d, want %d", got, SymbolErrorRepeat)
	}

	// Out-of-range falls back to identity.
	if got := lang.PublicSymbol(99); got != 99 {
		t.Errorf("PublicSymbol(99) = %d, want 99", got)
	}

	// Nil map falls back to identity.
	lang2 := &Language{}
	if got := lang2.PublicSymbol(5); got != 5 {
		t.Errorf("PublicSymbol(5) with nil map = %d, want 5", got)
	}
}

func TestLanguageNonTerminalAliases(t *testing.T) {
	// Perl-like alias map:
	// sym_block (10) has 3 aliases: 10, 20, 30
	// sym__term (40) has 2 aliases: 40, 50
	// terminated by 0
	lang := &Language{
		NonTerminalAliasMap: []uint16{
			10, 3, 10, 20, 30,
			40, 2, 40, 50,
			0,
		},
	}

	// sym_block has aliases.
	aliases := lang.NonTerminalAliases(10)
	if len(aliases) != 3 {
		t.Fatalf("NonTerminalAliases(10) = %v, want 3 aliases", aliases)
	}
	if aliases[0] != 10 || aliases[1] != 20 || aliases[2] != 30 {
		t.Errorf("NonTerminalAliases(10) = %v, want [10, 20, 30]", aliases)
	}

	// sym__term has aliases.
	aliases = lang.NonTerminalAliases(40)
	if len(aliases) != 2 {
		t.Fatalf("NonTerminalAliases(40) = %v, want 2 aliases", aliases)
	}
	if aliases[0] != 40 || aliases[1] != 50 {
		t.Errorf("NonTerminalAliases(40) = %v, want [40, 50]", aliases)
	}

	// Unknown symbol has no aliases.
	aliases = lang.NonTerminalAliases(99)
	if aliases != nil {
		t.Errorf("NonTerminalAliases(99) = %v, want nil", aliases)
	}

	// HasNonTerminalAliases.
	if !lang.HasNonTerminalAliases(10) {
		t.Error("HasNonTerminalAliases(10) should be true")
	}
	if !lang.HasNonTerminalAliases(40) {
		t.Error("HasNonTerminalAliases(40) should be true")
	}
	if lang.HasNonTerminalAliases(99) {
		t.Error("HasNonTerminalAliases(99) should be false")
	}

	// Nil map.
	lang2 := &Language{}
	if lang2.HasNonTerminalAliases(10) {
		t.Error("HasNonTerminalAliases with nil map should be false")
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
