package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestE2EGenerateAndLoadJSON is an end-to-end test that:
// 1. Extracts the JSON grammar from parser.c
// 2. Generates a Go package
// 3. Compiles it with a test harness that exercises the Language
// 4. Runs the test harness to verify correctness
func TestE2EGenerateAndLoadJSON(t *testing.T) {
	src := testParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	goSrc := GenerateGo(g, "jsonlang")

	// Set up temp project.
	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "jsonlang")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	repoRoot := findRepoRoot(t)

	goMod := `module e2etest

go 1.24.4

require github.com/dcosson/treesitter-go v0.0.0

replace github.com/dcosson/treesitter-go => ` + repoRoot + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write generated language.
	if err := os.WriteFile(filepath.Join(pkgDir, "json_language.go"), []byte(goSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write test harness that validates the Language struct.
	testSrc := `package jsonlang

import (
	"testing"

	ts "github.com/dcosson/treesitter-go"
)

func TestLanguageBasics(t *testing.T) {
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
	if lang.FieldCount != 2 {
		t.Errorf("FieldCount = %d, want 2", lang.FieldCount)
	}
}

func TestLanguageLookups(t *testing.T) {
	lang := JsonLanguage()

	// State 1, end (sym 0) -> action 5
	if got := lang.ExportLookup(1, 0); got != 5 {
		t.Errorf("lookup(1, 0) = %d, want 5", got)
	}

	// State 1, { (sym 1) -> action 7
	if got := lang.ExportLookup(1, 1); got != 7 {
		t.Errorf("lookup(1, 1) = %d, want 7", got)
	}

	// State 30 (small), end (sym 0) -> action 92
	if got := lang.ExportLookup(30, 0); got != 92 {
		t.Errorf("lookup(30, 0) = %d, want 92", got)
	}

	// State 7 (small), comment (sym 14) -> action 3
	if got := lang.ExportLookup(7, 14); got != 3 {
		t.Errorf("lookup(7, 14) = %d, want 3", got)
	}
}

func TestLanguageTableEntry(t *testing.T) {
	lang := JsonLanguage()

	// State 1, '{' -> shift to state 16.
	entry := lang.ExportTableEntry(1, 1)
	if entry.ActionCount != 1 {
		t.Fatalf("ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Actions[0].Type != ts.ParseActionTypeShift || entry.Actions[0].ShiftState != 16 {
		t.Errorf("action = %+v, want shift to 16", entry.Actions[0])
	}

	// State 30, end -> accept.
	entry = lang.ExportTableEntry(30, 0)
	if entry.ActionCount != 1 {
		t.Fatalf("ActionCount = %d, want 1", entry.ActionCount)
	}
	if entry.Actions[0].Type != ts.ParseActionTypeAccept {
		t.Errorf("action = %+v, want accept", entry.Actions[0])
	}
}

func TestLanguageSymbolNames(t *testing.T) {
	lang := JsonLanguage()

	tests := map[ts.Symbol]string{
		0:  "end",
		1:  "{",
		2:  ",",
		3:  "}",
		4:  ":",
		10: "number",
		15: "document",
		17: "object",
		18: "pair",
	}
	for sym, want := range tests {
		got := lang.SymbolName(sym)
		if got != want {
			t.Errorf("SymbolName(%d) = %q, want %q", sym, got, want)
		}
	}
}

func TestLanguageLexFn(t *testing.T) {
	lang := JsonLanguage()
	if lang.LexFn == nil {
		t.Fatal("LexFn is nil")
	}
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "json_language_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run the test.
	cmd := exec.Command("go", "test", "-v", "-race", "./jsonlang/")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("e2e test failed: %v\n%s", err, output)
	}

	t.Logf("e2e test output:\n%s", output)
}
