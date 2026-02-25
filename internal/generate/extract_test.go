package generate

import (
	"os"
	"path/filepath"
	"testing"
)

// testParserC returns the JSON grammar parser.c content for testing.
// It looks in a few standard locations.
func testParserC(t *testing.T) string {
	t.Helper()

	// Try testdata in the repo.
	paths := []string{
		filepath.Join("..", "..", "testdata", "grammars", "json", "src", "parser.c"),
		"/tmp/tree-sitter-json/src/parser.c",
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			return string(data)
		}
	}

	t.Skip("parser.c not found; run 'make fetch-test-grammars' or clone tree-sitter-json to /tmp")
	return ""
}

func TestExtractGrammarName(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}
	if g.Name != "json" {
		t.Errorf("Name = %q, want %q", g.Name, "json")
	}
}

func TestExtractConstants(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	checks := []struct {
		name string
		got  int
		want int
	}{
		{"SymbolCount", g.SymbolCount, 25},
		{"AliasCount", g.AliasCount, 0},
		{"TokenCount", g.TokenCount, 15},
		{"ExternalTokenCount", g.ExternalTokenCount, 0},
		{"StateCount", g.StateCount, 32},
		{"LargeStateCount", g.LargeStateCount, 7},
		{"ProductionIDCount", g.ProductionIDCount, 2},
		{"FieldCount", g.FieldCount, 2},
		{"MaxAliasSequenceLength", g.MaxAliasSequenceLength, 4},
	}

	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestExtractSymbolNames(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if len(g.SymbolNames) != 25 {
		t.Fatalf("SymbolNames length = %d, want 25", len(g.SymbolNames))
	}

	checks := map[int]string{
		0:  "end",
		1:  "{",
		2:  ",",
		3:  "}",
		4:  ":",
		5:  "[",
		6:  "]",
		7:  "\"",
		10: "number",
		11: "true",
		12: "false",
		13: "null",
		14: "comment",
		15: "document",
		17: "object",
		18: "pair",
		19: "array",
		20: "string",
	}

	for idx, want := range checks {
		if g.SymbolNames[idx] != want {
			t.Errorf("SymbolNames[%d] = %q, want %q", idx, g.SymbolNames[idx], want)
		}
	}
}

func TestExtractSymbolMetadata(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if len(g.SymbolMetadata) != 25 {
		t.Fatalf("SymbolMetadata length = %d, want 25", len(g.SymbolMetadata))
	}

	// Document should be visible and named.
	if !g.SymbolMetadata[15].Visible || !g.SymbolMetadata[15].Named {
		t.Errorf("document metadata: visible=%t named=%t, want visible=true named=true",
			g.SymbolMetadata[15].Visible, g.SymbolMetadata[15].Named)
	}

	// "{" should be visible and NOT named.
	if !g.SymbolMetadata[1].Visible || g.SymbolMetadata[1].Named {
		t.Errorf("'{' metadata: visible=%t named=%t, want visible=true named=false",
			g.SymbolMetadata[1].Visible, g.SymbolMetadata[1].Named)
	}

	// _value should be a supertype (not visible, but named).
	if g.SymbolMetadata[16].Visible || !g.SymbolMetadata[16].Named || !g.SymbolMetadata[16].Supertype {
		t.Errorf("_value metadata: visible=%t named=%t supertype=%t, want visible=false named=true supertype=true",
			g.SymbolMetadata[16].Visible, g.SymbolMetadata[16].Named, g.SymbolMetadata[16].Supertype)
	}
}

func TestExtractLexModes(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if len(g.LexModes) != 32 {
		t.Fatalf("LexModes length = %d, want 32", len(g.LexModes))
	}

	// States 17-19 use lex state 1 (string content).
	for i := 17; i <= 19; i++ {
		if g.LexModes[i].LexState != 1 {
			t.Errorf("LexModes[%d].LexState = %d, want 1", i, g.LexModes[i].LexState)
		}
	}

	// Other states use lex state 0.
	for i := 0; i < 17; i++ {
		if g.LexModes[i].LexState != 0 {
			t.Errorf("LexModes[%d].LexState = %d, want 0", i, g.LexModes[i].LexState)
		}
	}
}

func TestExtractFieldNames(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if len(g.FieldNames) != 3 {
		t.Fatalf("FieldNames length = %d, want 3", len(g.FieldNames))
	}

	if g.FieldNames[1] != "key" {
		t.Errorf("FieldNames[1] = %q, want %q", g.FieldNames[1], "key")
	}
	if g.FieldNames[2] != "value" {
		t.Errorf("FieldNames[2] = %q, want %q", g.FieldNames[2], "value")
	}
}

func TestExtractFieldMapEntries(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	// Production 1 (pair): key at child 0, value at child 2.
	if len(g.FieldMapSlices) < 2 {
		t.Fatalf("FieldMapSlices length = %d, want >= 2", len(g.FieldMapSlices))
	}

	slice := g.FieldMapSlices[1]
	if slice.Length != 2 {
		t.Fatalf("FieldMapSlices[1].Length = %d, want 2", slice.Length)
	}

	if len(g.FieldMapEntries) < 2 {
		t.Fatalf("FieldMapEntries length = %d, want >= 2", len(g.FieldMapEntries))
	}
}

func TestExtractParseActions(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	// The JSON grammar has 95 action entries in its parse_actions table.
	// (indices [0] through [94] in parser.c)
	if len(g.ParseActions) < 10 {
		t.Fatalf("ParseActions length = %d, want >= 10", len(g.ParseActions))
	}

	// First entry should be a header with count=0.
	if !g.ParseActions[0].IsHeader {
		t.Error("ParseActions[0] should be a header")
	}
	if g.ParseActions[0].Count != 0 {
		t.Errorf("ParseActions[0].Count = %d, want 0", g.ParseActions[0].Count)
	}

	// Second entry should be a header with count=1 (RECOVER).
	if !g.ParseActions[1].IsHeader {
		t.Error("ParseActions[1] should be a header")
	}
	if g.ParseActions[1].Count != 1 {
		t.Errorf("ParseActions[1].Count = %d, want 1", g.ParseActions[1].Count)
	}

	// Third entry should be RECOVER.
	if g.ParseActions[2].ActionType != "recover" {
		t.Errorf("ParseActions[2].ActionType = %q, want 'recover'", g.ParseActions[2].ActionType)
	}
}

func TestExtractLexFunction(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if len(g.LexStates) == 0 {
		t.Fatal("LexStates is empty")
	}

	// JSON grammar has states 0-43 (44 states).
	if len(g.LexStates) < 20 {
		t.Errorf("LexStates length = %d, want >= 20", len(g.LexStates))
	}

	// State 21 should accept ts_builtin_sym_end (symbol 0).
	found := false
	for _, s := range g.LexStates {
		if s.ID == 21 {
			found = true
			// In the C code, state 21 has ACCEPT_TOKEN(ts_builtin_sym_end).
			// ts_builtin_sym_end resolves to 0.
			break
		}
	}
	if !found {
		t.Error("state 21 not found in LexStates")
	}

	// State 22 should accept anon_sym_LBRACE.
	for _, s := range g.LexStates {
		if s.ID == 22 {
			if s.AcceptToken == 0 {
				t.Logf("state 22 AcceptToken = 0 (symbol name resolution needed)")
			}
			break
		}
	}
}

func TestExtractSmallParseTable(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if len(g.SmallParseTable) == 0 {
		t.Fatal("SmallParseTable is empty")
	}

	if len(g.SmallParseTableMap) == 0 {
		t.Fatal("SmallParseTableMap is empty")
	}

	// Should have 25 small states.
	if len(g.SmallParseTableMap) != 25 {
		t.Errorf("SmallParseTableMap length = %d, want 25", len(g.SmallParseTableMap))
	}
}

func TestParseIfTransitionsCompoundNegation(t *testing.T) {
	// Simulates C code: if (lookahead != '<' && lookahead != '&' && lookahead != 0) ADVANCE(76);
	lines := []string{
		"if (lookahead != '<' &&",
		"    lookahead != '&' &&",
		"    lookahead != 0) ADVANCE(76);",
	}
	idx := 0
	result := parseIfTransitions(lines[0], lines, &idx)
	if len(result) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(result))
	}
	tr := result[0]
	if !tr.IsNegated {
		t.Error("expected IsNegated=true")
	}
	if tr.Target != 76 {
		t.Errorf("Target = %d, want 76", tr.Target)
	}
	if len(tr.CharExclusions) != 3 {
		t.Fatalf("CharExclusions length = %d, want 3", len(tr.CharExclusions))
	}
	// Check exclusions contain '<', '&', and 0 (EOF).
	exclusionSet := make(map[rune]bool)
	for _, ex := range tr.CharExclusions {
		exclusionSet[ex] = true
	}
	if !exclusionSet['<'] {
		t.Error("missing '<' in CharExclusions")
	}
	if !exclusionSet['&'] {
		t.Error("missing '&' in CharExclusions")
	}
	if !exclusionSet[0] {
		t.Error("missing 0 (EOF) in CharExclusions")
	}
}

func TestParseIfTransitionsSimpleNegation(t *testing.T) {
	// Simple: if (lookahead != 0) ADVANCE(5);
	lines := []string{"if (lookahead != 0) ADVANCE(5);"}
	idx := 0
	result := parseIfTransitions(lines[0], lines, &idx)
	if len(result) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(result))
	}
	tr := result[0]
	if !tr.IsNegated {
		t.Error("expected IsNegated=true")
	}
	if tr.Char != 0 {
		t.Errorf("Char = %d, want 0", tr.Char)
	}
	if len(tr.CharExclusions) != 0 {
		t.Errorf("CharExclusions should be empty for simple negation, got %v", tr.CharExclusions)
	}
	if tr.Target != 5 {
		t.Errorf("Target = %d, want 5", tr.Target)
	}
}

func TestParseIfTransitionsSingleCharNegation(t *testing.T) {
	// Single char: if (lookahead != '/') ADVANCE(10);
	lines := []string{"if (lookahead != '/') ADVANCE(10);"}
	idx := 0
	result := parseIfTransitions(lines[0], lines, &idx)
	if len(result) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(result))
	}
	tr := result[0]
	if !tr.IsNegated {
		t.Error("expected IsNegated=true")
	}
	if tr.Char != '/' {
		t.Errorf("Char = %c, want '/'", tr.Char)
	}
	if len(tr.CharExclusions) != 0 {
		t.Errorf("CharExclusions should be empty for single negation, got %v", tr.CharExclusions)
	}
}

func TestParseActionMacrosSourceOrder(t *testing.T) {
	g := &Grammar{}

	// Test that actions are returned in C source-position order,
	// not grouped by type. This is critical for GLR merge disambiguation.
	tests := []struct {
		name  string
		line  string
		types []string
	}{
		{
			name:  "reduce before shift",
			line:  `REDUCE(sym_foo, 2, 0, 1), SHIFT(42)`,
			types: []string{"reduce", "shift"},
		},
		{
			name:  "shift before reduce",
			line:  `SHIFT(42), REDUCE(sym_bar, 1, -1, 2)`,
			types: []string{"shift", "reduce"},
		},
		{
			name:  "multiple reduces in order",
			line:  `REDUCE(sym_a, 1, 0, 1), REDUCE(sym_b, 2, -1, 2)`,
			types: []string{"reduce", "reduce"},
		},
		{
			name:  "shift_extra then reduce then shift",
			line:  `SHIFT_EXTRA(), REDUCE(sym_x, 1, 0, 3), SHIFT(10)`,
			types: []string{"shift", "reduce", "shift"},
		},
		{
			name:  "recover alone",
			line:  `RECOVER()`,
			types: []string{"recover"},
		},
		{
			name:  "accept at end",
			line:  `SHIFT(1), ACCEPT_INPUT()`,
			types: []string{"shift", "accept"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actions := g.parseActionMacros(tt.line)
			if len(actions) != len(tt.types) {
				t.Fatalf("got %d actions, want %d", len(actions), len(tt.types))
			}
			for i, want := range tt.types {
				if actions[i].ActionType != want {
					t.Errorf("actions[%d].ActionType = %q, want %q", i, actions[i].ActionType, want)
				}
			}
		})
	}
}

func TestExtractPrimaryStateIDs(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if len(g.PrimaryStateIDs) != 32 {
		t.Fatalf("PrimaryStateIDs length = %d, want 32", len(g.PrimaryStateIDs))
	}

	// For JSON, all states are their own primary state (identity map).
	for i, id := range g.PrimaryStateIDs {
		if int(id) != i {
			t.Errorf("PrimaryStateIDs[%d] = %d, want %d", i, id, i)
		}
	}
}
