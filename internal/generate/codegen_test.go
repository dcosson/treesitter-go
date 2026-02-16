package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateGoJSON(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	goSrc := GenerateGo(g, "jsongrammar")

	// Basic sanity checks on generated source.
	if !strings.Contains(goSrc, "package jsongrammar") {
		t.Error("generated source missing package declaration")
	}
	if !strings.Contains(goSrc, "func JsonLanguage()") {
		t.Error("generated source missing language function")
	}
	if !strings.Contains(goSrc, "SymbolCount:") {
		t.Error("generated source missing SymbolCount")
	}
	if !strings.Contains(goSrc, "func tsLex(") {
		t.Error("generated source missing lex function")
	}
	if !strings.Contains(goSrc, "parseActions") {
		t.Error("generated source missing parseActions")
	}
}

func TestGenerateGoCompiles(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	goSrc := GenerateGo(g, "jsongrammar")

	// Write to a temp directory and try to compile.
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "jsongrammar")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write go.mod.
	goMod := `module testmod

go 1.24.4

require github.com/treesitter-go/treesitter v0.0.0

replace github.com/treesitter-go/treesitter => ` + findRepoRoot(t) + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write generated source.
	if err := os.WriteFile(filepath.Join(pkgDir, "json_language.go"), []byte(goSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Try to compile.
	cmd := exec.Command("go", "build", "./jsongrammar/")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated code does not compile: %v\n%s", err, output)
	}
}

func TestGenerateGoMatchesHandCompiled(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	// Verify extracted constants match hand-compiled.
	if g.SymbolCount != 25 {
		t.Errorf("SymbolCount = %d, want 25", g.SymbolCount)
	}
	if g.StateCount != 32 {
		t.Errorf("StateCount = %d, want 32", g.StateCount)
	}
	if g.LargeStateCount != 7 {
		t.Errorf("LargeStateCount = %d, want 7", g.LargeStateCount)
	}

	// Verify parse table matches for key lookups.
	// State 1, symbol 0 (end) -> action 5 (REDUCE doc, 0)
	idx := 1*g.SymbolCount + 0
	if g.ParseTable[idx] != 5 {
		t.Errorf("ParseTable[1][end] = %d, want 5", g.ParseTable[idx])
	}

	// State 1, symbol 1 ({) -> action 7 (SHIFT 16)
	idx = 1*g.SymbolCount + 1
	if g.ParseTable[idx] != 7 {
		t.Errorf("ParseTable[1][{] = %d, want 7", g.ParseTable[idx])
	}

	// State 1, symbol 14 (comment) -> action 3 (SHIFT_EXTRA)
	idx = 1*g.SymbolCount + 14
	if g.ParseTable[idx] != 3 {
		t.Errorf("ParseTable[1][comment] = %d, want 3", g.ParseTable[idx])
	}

	// Verify parse actions.
	// Action[0]: header, count=0, reusable=false
	if !g.ParseActions[0].IsHeader || g.ParseActions[0].Count != 0 || g.ParseActions[0].Reusable {
		t.Errorf("ParseActions[0] = %+v, want header count=0 reusable=false", g.ParseActions[0])
	}

	// Action[1]: header, count=1, reusable=false
	if !g.ParseActions[1].IsHeader || g.ParseActions[1].Count != 1 || g.ParseActions[1].Reusable {
		t.Errorf("ParseActions[1] = %+v, want header count=1 reusable=false", g.ParseActions[1])
	}

	// Action[2]: RECOVER
	if g.ParseActions[2].ActionType != "recover" {
		t.Errorf("ParseActions[2] = %+v, want recover", g.ParseActions[2])
	}

	// Action[3]: header, count=1, reusable=true
	if !g.ParseActions[3].IsHeader || g.ParseActions[3].Count != 1 || !g.ParseActions[3].Reusable {
		t.Errorf("ParseActions[3] = %+v, want header count=1 reusable=true", g.ParseActions[3])
	}

	// Action[4]: SHIFT_EXTRA
	if g.ParseActions[4].ActionType != "shift" || !g.ParseActions[4].ShiftExtra {
		t.Errorf("ParseActions[4] = %+v, want shift extra", g.ParseActions[4])
	}

	// Action[5]: header, count=1, reusable=true
	if !g.ParseActions[5].IsHeader || g.ParseActions[5].Count != 1 || !g.ParseActions[5].Reusable {
		t.Errorf("ParseActions[5] = %+v, want header count=1 reusable=true", g.ParseActions[5])
	}

	// Action[6]: REDUCE(sym_document, 0, 0, 0)
	if g.ParseActions[6].ActionType != "reduce" || g.ParseActions[6].ReduceSymbol != 15 || g.ParseActions[6].ReduceChildCount != 0 {
		t.Errorf("ParseActions[6] = %+v, want reduce sym_document=15 count=0", g.ParseActions[6])
	}

	// Action[7]: header for SHIFT(16)
	if !g.ParseActions[7].IsHeader || g.ParseActions[7].Count != 1 {
		t.Errorf("ParseActions[7] = %+v, want header count=1", g.ParseActions[7])
	}

	// Action[8]: SHIFT(16)
	if g.ParseActions[8].ActionType != "shift" || g.ParseActions[8].ShiftState != 16 {
		t.Errorf("ParseActions[8] = %+v, want shift state=16", g.ParseActions[8])
	}

	// Action[90]: REDUCE(sym_pair, 3, 0, 1) — prod ID 1
	if len(g.ParseActions) > 91 {
		if g.ParseActions[90].IsHeader {
			// Check action at 91
			if g.ParseActions[91].ActionType != "reduce" || g.ParseActions[91].ReduceSymbol != 18 || g.ParseActions[91].ReduceProdID != 1 {
				t.Errorf("ParseActions[91] = %+v, want reduce sym_pair=18 prodID=1", g.ParseActions[91])
			}
		}
	}

	// Verify lex states count.
	if len(g.LexStates) < 40 {
		t.Errorf("LexStates = %d, want >= 40", len(g.LexStates))
	}

	// Verify some accept states.
	for _, s := range g.LexStates {
		switch s.ID {
		case 22:
			// Should accept anon_sym_LBRACE = 1
			if s.AcceptToken != 1 {
				t.Errorf("LexState[22].AcceptToken = %d, want 1", s.AcceptToken)
			}
		case 23:
			// Should accept anon_sym_COMMA = 2
			if s.AcceptToken != 2 {
				t.Errorf("LexState[23].AcceptToken = %d, want 2", s.AcceptToken)
			}
		case 35:
			// Should accept sym_number = 10
			if s.AcceptToken != 10 {
				t.Errorf("LexState[35].AcceptToken = %d, want 10", s.AcceptToken)
			}
		}
	}

	// Verify small parse table values.
	if len(g.SmallParseTable) < 100 {
		t.Errorf("SmallParseTable length = %d, want >= 100", len(g.SmallParseTable))
	}

	// First small state entry: groupCount should be 2.
	if g.SmallParseTable[0] != 2 {
		t.Errorf("SmallParseTable[0] = %d, want 2 (state 7 group count)", g.SmallParseTable[0])
	}
}

// findRepoRoot finds the repository root (the worktree root).
func findRepoRoot(t *testing.T) string {
	t.Helper()
	// Walk up from the test file location.
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root")
		}
		dir = parent
	}
}
