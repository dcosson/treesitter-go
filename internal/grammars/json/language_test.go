package json

import (
	"testing"

	ts "github.com/treesitter-go/treesitter"
)

func TestJSONLanguageConstants(t *testing.T) {
	lang := JsonLanguage()

	if lang.SymbolCount != 25 {
		t.Errorf("SymbolCount = %d, want 25", lang.SymbolCount)
	}
	if lang.TokenCount != 15 {
		t.Errorf("TokenCount = %d, want 15", lang.TokenCount)
	}
	if lang.StateCount != 32 {
		t.Errorf("StateCount = %d, want 32", lang.StateCount)
	}
	if lang.LargeStateCount != 7 {
		t.Errorf("LargeStateCount = %d, want 7", lang.LargeStateCount)
	}
	if lang.ExternalTokenCount != 0 {
		t.Errorf("ExternalTokenCount = %d, want 0", lang.ExternalTokenCount)
	}
	if lang.FieldCount != 2 {
		t.Errorf("FieldCount = %d, want 2", lang.FieldCount)
	}
	if lang.ProductionIDCount != 2 {
		t.Errorf("ProductionIDCount = %d, want 2", lang.ProductionIDCount)
	}
}

func TestJSONLanguageSymbolNames(t *testing.T) {
	lang := JsonLanguage()

	tests := []struct {
		sym  ts.Symbol
		name string
	}{
		{SymEnd, "end"},
		{SymLBrace, "{"},
		{SymComma, ","},
		{SymRBrace, "}"},
		{SymColon, ":"},
		{SymLBrack, "["},
		{SymRBrack, "]"},
		{SymDQuote, "\""},
		{SymStringContent, "string_content"},
		{SymEscapeSequence, "escape_sequence"},
		{SymNumber, "number"},
		{SymTrue, "true"},
		{SymFalse, "false"},
		{SymNull, "null"},
		{SymComment, "comment"},
		{SymDocument, "document"},
		{SymAuxValue, "_value"},
		{SymObject, "object"},
		{SymPair, "pair"},
		{SymArray, "array"},
		{SymString, "string"},
	}

	for _, tt := range tests {
		if got := lang.SymbolName(tt.sym); got != tt.name {
			t.Errorf("SymbolName(%d) = %q, want %q", tt.sym, got, tt.name)
		}
	}
}

func TestJSONLanguageSymbolMetadata(t *testing.T) {
	lang := JsonLanguage()

	// Named, visible nodes (user-facing).
	namedVisible := []ts.Symbol{
		SymDocument, SymObject, SymPair, SymArray, SymString,
		SymNumber, SymTrue, SymFalse, SymNull, SymComment,
		SymStringContent, SymEscapeSequence,
	}
	for _, sym := range namedVisible {
		if !lang.SymbolIsNamed(sym) {
			t.Errorf("symbol %d (%s) should be named", sym, lang.SymbolName(sym))
		}
		if !lang.SymbolIsVisible(sym) {
			t.Errorf("symbol %d (%s) should be visible", sym, lang.SymbolName(sym))
		}
	}

	// Anonymous, visible (punctuation like {, }, [, ], etc.)
	punctuation := []ts.Symbol{SymLBrace, SymRBrace, SymLBrack, SymRBrack, SymComma, SymColon, SymDQuote}
	for _, sym := range punctuation {
		if lang.SymbolIsNamed(sym) {
			t.Errorf("symbol %d (%s) should not be named", sym, lang.SymbolName(sym))
		}
		if !lang.SymbolIsVisible(sym) {
			t.Errorf("symbol %d (%s) should be visible", sym, lang.SymbolName(sym))
		}
	}

	// Hidden nodes (not visible).
	hidden := []ts.Symbol{SymAuxValue, SymAuxStringContent, SymDocumentRepeat1, SymObjectRepeat1, SymArrayRepeat1}
	for _, sym := range hidden {
		if lang.SymbolIsVisible(sym) {
			t.Errorf("symbol %d (%s) should not be visible", sym, lang.SymbolName(sym))
		}
	}

	// _value is a supertype.
	if !lang.SymbolMetadata[SymAuxValue].Supertype {
		t.Error("_value should be a supertype")
	}
}

func TestJSONLanguageLargeStateLookup(t *testing.T) {
	lang := JsonLanguage()

	// State 1 (large state): expecting a value.
	tests := []struct {
		name   string
		state  ts.StateID
		symbol ts.Symbol
		want   uint16
	}{
		{"state1 end -> reduce doc", 1, SymEnd, 5},
		{"state1 { -> shift", 1, SymLBrace, 7},
		{"state1 [ -> shift", 1, SymLBrack, 9},
		{"state1 quote -> shift", 1, SymDQuote, 11},
		{"state1 number -> shift", 1, SymNumber, 13},
		{"state1 true -> shift", 1, SymTrue, 13},
		{"state1 comment -> extra", 1, SymComment, 3},
		{"state1 } -> no action", 1, SymRBrace, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lang.ExportLookup(tt.state, tt.symbol)
			if got != tt.want {
				t.Errorf("lookup(%d, %d) = %d, want %d", tt.state, tt.symbol, got, tt.want)
			}
		})
	}
}

func TestJSONLanguageSmallStateLookup(t *testing.T) {
	lang := JsonLanguage()

	tests := []struct {
		name   string
		state  ts.StateID
		symbol ts.Symbol
		want   uint16
	}{
		// State 7 (small index 0): REDUCE(sym_object, 2) — empty object "{}"
		// In C: ACTIONS(3) for comment, ACTIONS(37) for all other terminals.
		{"state7 comment -> extra", 7, SymComment, 3},
		{"state7 end -> reduce", 7, SymEnd, 37},
		{"state7 comma -> reduce", 7, SymComma, 37},
		{"state7 } -> reduce", 7, SymRBrace, 37},
		{"state7 ] -> reduce", 7, SymRBrack, 37},
		{"state7 { -> reduce", 7, SymLBrace, 37},

		// State 9 (small index 2): REDUCE(sym_array, 2) — empty array "[]"
		// In C: ACTIONS(3) for comment, ACTIONS(41) for all other terminals.
		{"state9 comment -> extra", 9, SymComment, 3},
		{"state9 } -> reduce", 9, SymRBrace, 41},
		{"state9 ] -> reduce", 9, SymRBrack, 41},
		{"state9 [ -> reduce", 9, SymLBrack, 41},

		// State 11 (small index 4): REDUCE(sym_object, 3) — object with pairs
		// In C: ACTIONS(3) for comment, ACTIONS(43) for all other terminals.
		{"state11 comment -> extra", 11, SymComment, 3},
		{"state11 end -> reduce", 11, SymEnd, 43},

		// State 30 (small index 23): accept state
		// In C: SMALL_STATE(30) at offset 346: ACTIONS(3) for comment, ACTIONS(92) for end.
		{"state30 end -> accept", 30, SymEnd, 92},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lang.ExportLookup(tt.state, tt.symbol)
			if got != tt.want {
				t.Errorf("lookup(%d, %d) = %d, want %d", tt.state, tt.symbol, got, tt.want)
			}
		})
	}
}

func TestJSONLanguageTableEntry(t *testing.T) {
	lang := JsonLanguage()

	// State 1, '{' -> shift to state 16.
	entry := lang.ExportTableEntry(1, SymLBrace)
	if entry.ActionCount != 1 {
		t.Fatalf("ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Actions[0].Type != ts.ParseActionTypeShift {
		t.Errorf("action type = %d, want shift", entry.Actions[0].Type)
	}
	if entry.Actions[0].ShiftState != 16 {
		t.Errorf("shift state = %d, want 16", entry.Actions[0].ShiftState)
	}

	// State 1, end -> reduce(document, 0 children).
	entry = lang.ExportTableEntry(1, SymEnd)
	if entry.ActionCount != 1 {
		t.Fatalf("ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Actions[0].Type != ts.ParseActionTypeReduce {
		t.Errorf("action type = %d, want reduce", entry.Actions[0].Type)
	}
	if entry.Actions[0].ReduceSymbol != SymDocument {
		t.Errorf("reduce symbol = %d, want %d (document)", entry.Actions[0].ReduceSymbol, SymDocument)
	}
	if entry.Actions[0].ReduceChildCount != 0 {
		t.Errorf("reduce child count = %d, want 0", entry.Actions[0].ReduceChildCount)
	}

	// State 30 (small), end -> accept.
	entry = lang.ExportTableEntry(30, SymEnd)
	if entry.ActionCount != 1 {
		t.Fatalf("ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Actions[0].Type != ts.ParseActionTypeAccept {
		t.Errorf("action type = %d, want accept", entry.Actions[0].Type)
	}

	// No action.
	entry = lang.ExportTableEntry(1, SymRBrace)
	if entry.ActionCount != 0 {
		t.Errorf("ActionCount = %d, want 0 (no action for } in state 1)", entry.ActionCount)
	}
}

func TestJSONLanguageFieldMap(t *testing.T) {
	lang := JsonLanguage()

	// Production 1 is the pair rule: key:value.
	entries := lang.FieldMapForProduction(1)
	if len(entries) != 2 {
		t.Fatalf("pair field entries = %d, want 2", len(entries))
	}

	if entries[0].FieldID != FieldKey {
		t.Errorf("entries[0].FieldID = %d, want %d (key)", entries[0].FieldID, FieldKey)
	}
	if entries[0].ChildIndex != 0 {
		t.Errorf("entries[0].ChildIndex = %d, want 0", entries[0].ChildIndex)
	}
	if lang.FieldName(entries[0].FieldID) != "key" {
		t.Errorf("field name = %q, want %q", lang.FieldName(entries[0].FieldID), "key")
	}

	if entries[1].FieldID != FieldValue {
		t.Errorf("entries[1].FieldID = %d, want %d (value)", entries[1].FieldID, FieldValue)
	}
	if entries[1].ChildIndex != 2 {
		t.Errorf("entries[1].ChildIndex = %d, want 2", entries[1].ChildIndex)
	}
	if lang.FieldName(entries[1].FieldID) != "value" {
		t.Errorf("field name = %q, want %q", lang.FieldName(entries[1].FieldID), "value")
	}
}

func TestJSONLanguageLexModes(t *testing.T) {
	lang := JsonLanguage()

	// Most states use lex state 0.
	for i := 0; i < 17; i++ {
		if lang.LexModes[i].LexState != 0 {
			t.Errorf("LexModes[%d].LexState = %d, want 0", i, lang.LexModes[i].LexState)
		}
	}

	// String content states use lex state 1.
	for i := 17; i <= 19; i++ {
		if lang.LexModes[i].LexState != 1 {
			t.Errorf("LexModes[%d].LexState = %d, want 1", i, lang.LexModes[i].LexState)
		}
	}
}

func TestJSONLanguageSupertypes(t *testing.T) {
	lang := JsonLanguage()

	if len(lang.SupertypeSymbols) != 1 {
		t.Fatalf("SupertypeSymbols length = %d, want 1", len(lang.SupertypeSymbols))
	}
	if lang.SupertypeSymbols[0] != SymAuxValue {
		t.Errorf("SupertypeSymbols[0] = %d, want %d (_value)", lang.SupertypeSymbols[0], SymAuxValue)
	}
}
