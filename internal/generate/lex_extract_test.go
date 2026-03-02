package generate

import (
	"testing"
)

func TestSplitTopLevelAnd(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  int // number of parts
	}{
		{"simple two conditions", "lookahead != '\\'' && lookahead != 0", 2},
		{"with parenthesized exclusion range", "lookahead > '#' && (lookahead < '%' || '@' < lookahead) && lookahead != '`'", 3},
		{"multiple exclusion ranges", "lookahead > '^' && lookahead != '`' && (lookahead < '{' || '~' < lookahead)", 3},
		{"single condition", "lookahead != 0", 1},
		{"pattern 5: != + exclusion range", "lookahead != 0 && (lookahead < '\\t' || '\\r' < lookahead) && lookahead != '\"' && lookahead != '\\\\'", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitTopLevelAnd(tt.input)
			if len(got) != tt.want {
				t.Fatalf("splitTopLevelAnd(%q) returned %d parts, want %d:\ngot: %v",
					tt.input, len(got), tt.want, got)
			}
		})
	}
}

func TestDedupeRunes(t *testing.T) {
	tests := []struct {
		name  string
		input []rune
		want  []rune
	}{
		{"empty", nil, nil},
		{"single", []rune{'a'}, []rune{'a'}},
		{"no dupes", []rune{'a', 'b', 'c'}, []rune{'a', 'b', 'c'}},
		{"with dupes", []rune{'a', 'b', 'a', 'c', 'b'}, []rune{'a', 'b', 'c'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeRunes(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("dedupeRunes(%v) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("dedupeRunes(%v)[%d] = %c, want %c", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestParseSetContainsCompound tests that compound set_contains conditions
// are correctly parsed with EOFGuard, exclusions, OR'd chars, and exclude ranges.
func TestParseSetContainsCompound(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		wantSet        string
		wantSkip       bool
		wantEOF        bool
		wantOrChars    []rune
		wantExclusions []rune
		wantExclRanges []RuneRange
	}{
		{
			name:    "simple set_contains",
			line:    "if (set_contains(sym_identifier_character_set_1, 669, lookahead)) ADVANCE(191);",
			wantSet: "sym_identifier_character_set_1",
		},
		{
			name:    "compound !eof && set_contains",
			line:    "if ((!eof && set_contains(sym_identifier_character_set_2, 763, lookahead))) ADVANCE(961);",
			wantSet: "sym_identifier_character_set_2",
			wantEOF: true,
		},
		{
			name:     "set_contains with SKIP",
			line:     "if (set_contains(sym_word_character_set_1, 100, lookahead)) SKIP(5);",
			wantSet:  "sym_word_character_set_1",
			wantSkip: true,
		},
		{
			name:           "set_contains with char exclusion",
			line:           `if ((set_contains(extras_character_set_1, 10, lookahead)) && lookahead != '\n') ADVANCE(262);`,
			wantSet:        "extras_character_set_1",
			wantExclusions: []rune{'\n'},
		},
		{
			name:           "set_contains with multiple char exclusions",
			line:           `if ((set_contains(extras_character_set_1, 10, lookahead)) && lookahead != '\n' && lookahead != '\r') ADVANCE(233);`,
			wantSet:        "extras_character_set_1",
			wantExclusions: []rune{'\n', '\r'},
		},
		{
			name:           "set_contains OR'd with char plus exclusions",
			line:           `if ((set_contains(sym_substitution_regexp_modifiers_character_set_1, 9, lookahead) || lookahead == 'n') && lookahead != 'e' && lookahead != 'r') ADVANCE(300);`,
			wantSet:        "sym_substitution_regexp_modifiers_character_set_1",
			wantOrChars:    []rune{'n'},
			wantExclusions: []rune{'e', 'r'},
		},
		{
			name:           "set_contains with exclude ranges and char exclusion",
			line:           `if ((set_contains(extras_character_set_1, 10, lookahead)) && (lookahead < '\t' || '\r' < lookahead) && lookahead != ' ') ADVANCE(413);`,
			wantSet:        "extras_character_set_1",
			wantExclusions: []rune{' '},
			wantExclRanges: []RuneRange{{Low: '\t', High: '\r'}},
		},
		{
			name:           "set_contains with compound exclude ranges",
			line:           `if ((set_contains(sym__identifier_character_set_1, 668, lookahead)) && (lookahead < 'A' || 'Z' < lookahead) && lookahead != '_' && (lookahead < 'a' || 'z' < lookahead)) ADVANCE(273);`,
			wantSet:        "sym__identifier_character_set_1",
			wantExclusions: []rune{'_'},
			wantExclRanges: []RuneRange{{Low: 'A', High: 'Z'}, {Low: 'a', High: 'z'}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := parseSetContains(tt.line)
			if tr == nil {
				t.Fatal("parseSetContains returned nil")
			}
			if tr.CharSetName != tt.wantSet {
				t.Errorf("CharSetName = %q, want %q", tr.CharSetName, tt.wantSet)
			}
			if tr.Skip != tt.wantSkip {
				t.Errorf("Skip = %v, want %v", tr.Skip, tt.wantSkip)
			}
			if tr.EOFGuard != tt.wantEOF {
				t.Errorf("EOFGuard = %v, want %v", tr.EOFGuard, tt.wantEOF)
			}
			// Check OR'd chars.
			if len(tr.CharSetOrChars) != len(tt.wantOrChars) {
				t.Fatalf("CharSetOrChars has %d entries, want %d: %v", len(tr.CharSetOrChars), len(tt.wantOrChars), tr.CharSetOrChars)
			}
			for i, ch := range tr.CharSetOrChars {
				if ch != tt.wantOrChars[i] {
					t.Errorf("CharSetOrChars[%d] = %c, want %c", i, ch, tt.wantOrChars[i])
				}
			}
			// Check exclusions.
			if len(tr.CharExclusions) != len(tt.wantExclusions) {
				t.Fatalf("CharExclusions has %d entries, want %d: %v", len(tr.CharExclusions), len(tt.wantExclusions), tr.CharExclusions)
			}
			for i, ex := range tr.CharExclusions {
				if ex != tt.wantExclusions[i] {
					t.Errorf("CharExclusions[%d] = %c (%d), want %c (%d)", i, ex, ex, tt.wantExclusions[i], tt.wantExclusions[i])
				}
			}
			// Check exclude ranges.
			if len(tr.ExcludeRanges) != len(tt.wantExclRanges) {
				t.Fatalf("ExcludeRanges has %d entries, want %d", len(tr.ExcludeRanges), len(tt.wantExclRanges))
			}
			for i, er := range tr.ExcludeRanges {
				if er.Low != tt.wantExclRanges[i].Low || er.High != tt.wantExclRanges[i].High {
					t.Errorf("ExcludeRanges[%d] = {%c, %c}, want {%c, %c}",
						i, er.Low, er.High, tt.wantExclRanges[i].Low, tt.wantExclRanges[i].High)
				}
			}
		})
	}
}

// TestParseIfTransitionsCompound tests compound conditions with >, !=, and exclusion ranges.
func TestParseIfTransitionsCompound(t *testing.T) {
	tests := []struct {
		name           string
		line           string
		wantNegated    bool
		wantLowBound   rune
		wantExclusions []rune
		wantExclRanges []RuneRange
	}{
		{
			name:           "simple != char",
			line:           "if (lookahead != '<') ADVANCE(5);",
			wantNegated:    true,
			wantExclusions: []rune{'<'},
		},
		{
			name:           "!= with zero",
			line:           "if (lookahead != 0) ADVANCE(5);",
			wantNegated:    true,
			wantExclusions: []rune{0},
		},
		{
			name:           "compound != chain",
			line:           "if (lookahead != '<' && lookahead != 0) ADVANCE(5);",
			wantNegated:    true,
			wantExclusions: []rune{'<', 0},
		},
		{
			name:           "Bug 2: lookahead > X && lookahead != Y",
			line:           "if (lookahead > '^' && lookahead != '`') ADVANCE(321);",
			wantNegated:    true,
			wantLowBound:   '^',
			wantExclusions: []rune{'`'},
		},
		{
			name:           "Bug 3: != with exclusion range",
			line:           "if (lookahead > '^' && lookahead != '`' && (lookahead < '{' || '~' < lookahead)) ADVANCE(321);",
			wantNegated:    true,
			wantLowBound:   '^',
			wantExclusions: []rune{'`'},
			wantExclRanges: []RuneRange{{Low: '{', High: '~'}},
		},
		{
			name:         "Pattern 4: > with multiple exclusion ranges",
			line:         "if (lookahead > '#' && (lookahead < '%' || '@' < lookahead) && (lookahead < '[' || '^' < lookahead)) ADVANCE(275);",
			wantNegated:  true,
			wantLowBound: '#',
			wantExclRanges: []RuneRange{
				{Low: '%', High: '@'},
				{Low: '[', High: '^'},
			},
		},
		{
			name:           "Pattern 5: != with exclusion ranges",
			line:           `if (lookahead != 0 && (lookahead < '\t' || '\r' < lookahead) && lookahead != '"') ADVANCE(33);`,
			wantNegated:    true,
			wantExclusions: []rune{0, '"'},
			wantExclRanges: []RuneRange{{Low: '\t', High: '\r'}},
		},
		{
			name:           "escaped single quote in !=",
			line:           `if (lookahead != 0 && lookahead != '&' && lookahead != '\'') ADVANCE(169);`,
			wantNegated:    true,
			wantExclusions: []rune{0, '&', '\''},
		},
		{
			name:           "escaped single quote standalone",
			line:           `if (lookahead != '\'') ADVANCE(160);`,
			wantNegated:    true,
			wantExclusions: []rune{'\''},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := []string{tt.line}
			idx := 0
			transitions := parseIfTransitions(tt.line, lines, &idx)

			if len(transitions) != 1 {
				t.Fatalf("got %d transitions, want 1", len(transitions))
			}
			tr := transitions[0]

			if tr.IsNegated != tt.wantNegated {
				t.Errorf("IsNegated = %v, want %v", tr.IsNegated, tt.wantNegated)
			}
			if tr.LowBound != tt.wantLowBound {
				t.Errorf("LowBound = %c (%d), want %c (%d)", tr.LowBound, tr.LowBound, tt.wantLowBound, tt.wantLowBound)
			}

			// Check exclusions.
			if tt.wantExclusions == nil {
				if tr.Char != 0 && len(tr.CharExclusions) > 0 {
					t.Errorf("expected no exclusions, got Char=%c, CharExclusions=%v", tr.Char, tr.CharExclusions)
				}
			} else if len(tt.wantExclusions) == 1 {
				if tr.Char != tt.wantExclusions[0] {
					t.Errorf("Char = %c (%d), want %c (%d)", tr.Char, tr.Char, tt.wantExclusions[0], tt.wantExclusions[0])
				}
			} else {
				if len(tr.CharExclusions) != len(tt.wantExclusions) {
					t.Fatalf("CharExclusions has %d entries, want %d: %v", len(tr.CharExclusions), len(tt.wantExclusions), tr.CharExclusions)
				}
				for i, ex := range tr.CharExclusions {
					if ex != tt.wantExclusions[i] {
						t.Errorf("CharExclusions[%d] = %c (%d), want %c (%d)", i, ex, ex, tt.wantExclusions[i], tt.wantExclusions[i])
					}
				}
			}

			// Check exclusion ranges.
			if len(tr.ExcludeRanges) != len(tt.wantExclRanges) {
				t.Fatalf("ExcludeRanges has %d entries, want %d", len(tr.ExcludeRanges), len(tt.wantExclRanges))
			}
			for i, er := range tr.ExcludeRanges {
				if er.Low != tt.wantExclRanges[i].Low || er.High != tt.wantExclRanges[i].High {
					t.Errorf("ExcludeRanges[%d] = {%c, %c}, want {%c, %c}",
						i, er.Low, er.High, tt.wantExclRanges[i].Low, tt.wantExclRanges[i].High)
				}
			}
		})
	}
}

// TestParseCaseBlockMultiLineSetContains tests that multi-line set_contains
// if-statements are properly joined and parsed.
func TestParseCaseBlockMultiLineSetContains(t *testing.T) {
	// Reproduces the Perl parser.c state 300 pattern.
	body := `
      ACCEPT_TOKEN(sym_match_regexp_modifiers);
      if ((set_contains(sym_substitution_regexp_modifiers_character_set_1, 9, lookahead) ||
          lookahead == 'n') &&
          lookahead != 'e' &&
          lookahead != 'r') ADVANCE(300);
      END_STATE();
`
	state, err := parseCaseBlock(300, body, nil)
	if err != nil {
		t.Fatalf("parseCaseBlock: %v", err)
	}

	if len(state.Transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(state.Transitions))
	}
	tr := state.Transitions[0]
	if tr.CharSetName != "sym_substitution_regexp_modifiers_character_set_1" {
		t.Errorf("CharSetName = %q, want sym_substitution_regexp_modifiers_character_set_1", tr.CharSetName)
	}
	if tr.Target != 300 {
		t.Errorf("Target = %d, want 300", tr.Target)
	}
	if len(tr.CharSetOrChars) != 1 || tr.CharSetOrChars[0] != 'n' {
		t.Errorf("CharSetOrChars = %v, want ['n']", tr.CharSetOrChars)
	}
	wantExcl := []rune{'e', 'r'}
	if len(tr.CharExclusions) != len(wantExcl) {
		t.Fatalf("CharExclusions has %d entries, want %d", len(tr.CharExclusions), len(wantExcl))
	}
	for i, ex := range tr.CharExclusions {
		if ex != wantExcl[i] {
			t.Errorf("CharExclusions[%d] = %c, want %c", i, ex, wantExcl[i])
		}
	}
}

// TestParseCaseBlockEOFOrdering ensures that !eof && set_contains is NOT
// misinterpreted as a standalone eof check (Bug 1).
func TestParseCaseBlockEOFOrdering(t *testing.T) {
	body := `
      if ((!eof && set_contains(sym_identifier_character_set_2, 763, lookahead))) ADVANCE(961);
      END_STATE();
`
	state, err := parseCaseBlock(961, body, nil)
	if err != nil {
		t.Fatalf("parseCaseBlock: %v", err)
	}

	if state.HasEOFCheck {
		t.Error("HasEOFCheck is true; compound !eof condition was misinterpreted as standalone eof check")
	}

	if len(state.Transitions) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(state.Transitions))
	}
	tr := state.Transitions[0]
	if tr.CharSetName != "sym_identifier_character_set_2" {
		t.Errorf("CharSetName = %q, want sym_identifier_character_set_2", tr.CharSetName)
	}
	if !tr.EOFGuard {
		t.Error("EOFGuard should be true for !eof && set_contains compound")
	}
	if tr.Target != 961 {
		t.Errorf("Target = %d, want 961", tr.Target)
	}
}

// TestParseIfTransitionsHexEquality tests hex literal matching in || chains.
func TestParseIfTransitionsHexEquality(t *testing.T) {
	// Pattern D: lookahead == 0xNNNN in || chain (from Python parser.c).
	line := `if (('\t' <= lookahead && lookahead <= '\f') || lookahead == ' ' || lookahead == 0x200b || lookahead == 0x2060 || lookahead == 0xfeff) SKIP(51);`
	lines := []string{line}
	idx := 0
	transitions := parseIfTransitions(line, lines, &idx)

	// Should produce: 1 range + 1 char + 3 hex chars = 5 transitions.
	if len(transitions) != 5 {
		t.Fatalf("got %d transitions, want 5", len(transitions))
	}

	// Check the range transition.
	if !transitions[0].IsRange || transitions[0].Low != '\t' || transitions[0].High != '\f' {
		t.Errorf("transition 0: expected range [\\t, \\f], got IsRange=%v Low=%d High=%d",
			transitions[0].IsRange, transitions[0].Low, transitions[0].High)
	}
	// Check ' '.
	if transitions[1].Char != ' ' {
		t.Errorf("transition 1: Char = %d, want %d (' ')", transitions[1].Char, ' ')
	}
	// Check hex values.
	if transitions[2].Char != 0x200b {
		t.Errorf("transition 2: Char = 0x%x, want 0x200b", transitions[2].Char)
	}
	if transitions[3].Char != 0x2060 {
		t.Errorf("transition 3: Char = 0x%x, want 0x2060", transitions[3].Char)
	}
	if transitions[4].Char != 0xfeff {
		t.Errorf("transition 4: Char = 0x%x, want 0xfeff", transitions[4].Char)
	}
	// All should be SKIP.
	for i, tr := range transitions {
		if !tr.Skip {
			t.Errorf("transition %d: Skip = false, want true", i)
		}
		if tr.Target != 51 {
			t.Errorf("transition %d: Target = %d, want 51", i, tr.Target)
		}
	}
}

// TestParseIfTransitionsHexNegation tests hex literal matching in && chains.
func TestParseIfTransitionsHexNegation(t *testing.T) {
	// Pattern E: lookahead != 0xNNNN in && chain (from JavaScript parser.c).
	line := `if (lookahead != 0 && lookahead != '\n' && lookahead != '\r' && lookahead != 0x2028 && lookahead != 0x2029) ADVANCE(250);`
	lines := []string{line}
	idx := 0
	transitions := parseIfTransitions(line, lines, &idx)

	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	tr := transitions[0]
	if !tr.IsNegated {
		t.Error("IsNegated = false, want true")
	}
	// Should have 5 exclusions: 0, '\n', '\r', 0x2028, 0x2029.
	want := []rune{0, '\n', '\r', 0x2028, 0x2029}
	if len(tr.CharExclusions) != len(want) {
		t.Fatalf("CharExclusions has %d entries, want %d: %v", len(tr.CharExclusions), len(want), tr.CharExclusions)
	}
	for i, ex := range tr.CharExclusions {
		if ex != want[i] {
			t.Errorf("CharExclusions[%d] = 0x%x, want 0x%x", i, ex, want[i])
		}
	}
}

// TestParseIfTransitionsEOFGuardWithBareInt tests !eof && lookahead == 00 (Pattern F).
func TestParseIfTransitionsEOFGuardWithBareInt(t *testing.T) {
	// Pattern F from Python parser.c: (!eof && lookahead == 00) || lookahead == '\n'
	line := `if ((!eof && lookahead == 00) || lookahead == '\n') ADVANCE(168);`
	lines := []string{line}
	idx := 0
	transitions := parseIfTransitions(line, lines, &idx)

	// Should produce 2 transitions: '\n' (from charRe) and bare int 0 (from bareIntEqRe).
	if len(transitions) != 2 {
		t.Fatalf("got %d transitions, want 2", len(transitions))
	}
	// Both should have EOFGuard since the line contains !eof.
	if !transitions[0].EOFGuard {
		t.Error("transition 0: EOFGuard = false, want true")
	}
	// charRe matches '\n' first, then bareIntEqRe matches 00.
	if transitions[0].Char != '\n' {
		t.Errorf("transition 0: Char = %d, want '\\n' (%d)", transitions[0].Char, '\n')
	}
	if transitions[1].Char != 0 {
		t.Errorf("transition 1: Char = %d, want 0", transitions[1].Char)
	}
}

// TestParseIfTransitionsEOFGuardStandalone tests !eof && lookahead == 00 standalone.
func TestParseIfTransitionsEOFGuardStandalone(t *testing.T) {
	// Pattern F standalone from Python parser.c.
	line := `if ((!eof && lookahead == 00)) ADVANCE(136);`
	lines := []string{line}
	idx := 0
	transitions := parseIfTransitions(line, lines, &idx)

	if len(transitions) != 1 {
		t.Fatalf("got %d transitions, want 1", len(transitions))
	}
	tr := transitions[0]
	if !tr.EOFGuard {
		t.Error("EOFGuard = false, want true")
	}
	if tr.Char != 0 {
		t.Errorf("Char = %d, want 0", tr.Char)
	}
	if tr.Target != 136 {
		t.Errorf("Target = %d, want 136", tr.Target)
	}
}
