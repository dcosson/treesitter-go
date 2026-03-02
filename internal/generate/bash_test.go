package generate

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// testBashParserC returns the Bash grammar parser.c content for testing.
func testBashParserC(t *testing.T) string {
	t.Helper()
	paths := []string{
		filepath.Join("..", "..", "testdata", "grammars", "bash", "src", "parser.c"),
		"/tmp/tree-sitter-bash/src/parser.c",
	}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err == nil {
			return string(data)
		}
	}
	t.Skip("Bash grammar parser.c not found")
	return ""
}

// TestExtractBashGrammar tests that the Bash grammar can be fully extracted.
func TestExtractBashGrammar(t *testing.T) {
	src := testBashParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	if g.Name != "bash" {
		t.Errorf("Name = %q, want %q", g.Name, "bash")
	}
	if g.SymbolCount < 200 {
		t.Errorf("SymbolCount = %d, want >= 200", g.SymbolCount)
	}
	if g.StateCount < 2000 {
		t.Errorf("StateCount = %d, want >= 2000", g.StateCount)
	}
	if g.ExternalTokenCount < 20 {
		t.Errorf("ExternalTokenCount = %d, want >= 20", g.ExternalTokenCount)
	}

	t.Logf("Bash grammar: %d symbols, %d states (%d large), %d lex states, %d keyword lex states, %d fields, %d external tokens, %d parse actions",
		g.SymbolCount, g.StateCount, g.LargeStateCount,
		len(g.LexStates), len(g.KeywordLexStates),
		g.FieldCount, g.ExternalTokenCount, len(g.ParseActions))
}

// TestGenerateBashGrammarCompiles tests that the generated Bash grammar compiles.
func TestGenerateBashGrammarCompiles(t *testing.T) {
	src := testBashParserC(t)
	g, err := ExtractGrammar(src)
	if err != nil {
		t.Fatalf("ExtractGrammar: %v", err)
	}

	goSrc := GenerateGo(g, "bash")

	tmpDir := t.TempDir()
	pkgDir := filepath.Join(tmpDir, "bash")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}

	goMod := `module testmod

go 1.24.4

require github.com/dcosson/treesitter-go v0.0.0

replace github.com/dcosson/treesitter-go => ` + findRepoRoot(t) + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(pkgDir, "bash_language.go"), []byte(goSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("go", "build", "./bash/")
	cmd.Dir = tmpDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		snippet := goSrc
		if len(snippet) > 2000 {
			snippet = snippet[:2000]
		}
		t.Fatalf("generated Bash grammar does not compile: %v\n%s\nGenerated source (first 2000 chars):\n%s", err, output, snippet)
	}
	t.Logf("Bash grammar generated %d bytes of Go source, compiles OK", len(goSrc))
}
