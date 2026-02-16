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

// TestParseSetContainsCompound tests that compound !eof && set_contains conditions
// are correctly parsed with EOFGuard.
func TestParseSetContainsCompound(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantSet  string
		wantSkip bool
		wantEOF  bool
	}{
		{
			name:    "simple set_contains",
			line:    "if (set_contains(sym_identifier_character_set_1, 669, lookahead)) ADVANCE(191);",
			wantSet: "sym_identifier_character_set_1",
			wantEOF: false,
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
			wantEOF:  false,
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
