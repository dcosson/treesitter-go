package difftest

import (
	"context"
	"flag"
	iparser "github.com/treesitter-go/treesitter/parser"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/treesitter-go/treesitter/internal/corpustest"
	golanggrammar "github.com/treesitter-go/treesitter/internal/testgrammars/golang"
)

var tsCLIFlag = flag.String("ts-cli", "", "path to tree-sitter CLI binary")

func TestMain(m *testing.M) {
	flag.Parse()
	if *tsCLIFlag != "" {
		TreeSitterCLI = *tsCLIFlag
	} else if envPath := os.Getenv("TS_CLI_PATH"); envPath != "" {
		TreeSitterCLI = envPath
	}
	os.Exit(m.Run())
}

// requireCLI skips the test if the tree-sitter CLI is not available.
func requireCLI(t *testing.T) {
	t.Helper()
	if TreeSitterCLI == "" {
		t.Skip("tree-sitter CLI not configured — pass -ts-cli or set TS_CLI_PATH")
	}
	if _, err := os.Stat(TreeSitterCLI); err != nil {
		if _, err2 := exec.LookPath(TreeSitterCLI); err2 != nil {
			t.Skipf("tree-sitter CLI not found at %q: %v", TreeSitterCLI, err)
		}
	}
}

// requireCLIScope skips the test if the CLI doesn't support the given scope.
func requireCLIScope(t *testing.T, scope string) {
	t.Helper()
	requireCLI(t)
	// Try parsing a minimal input to see if the scope is supported.
	_, err := ParseBytesWithCLI([]byte(" "), scope)
	if err != nil && strings.Contains(err.Error(), "Unknown scope") {
		t.Skipf("tree-sitter CLI does not support scope %q (grammar not installed)", scope)
	}
}

// goParseFunc returns a ParseFunc for the Go language grammar.
func goParseFunc() corpustest.ParseFunc {
	lang := golanggrammar.GoLanguage()
	return func(input []byte) (string, error) {
		p := iparser.NewParser()
		p.SetLanguage(lang)
		tree := p.ParseString(context.Background(), input)
		if tree == nil {
			return "", nil
		}
		return tree.RootNode().String(), nil
	}
}

// --- Unit tests (no CLI required) ---

func TestNormalizeCLIOutput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			"simple",
			"(document (object (pair key: (string) value: (number))))",
			"(document (object (pair (string) (number))))",
		},
		{
			"with point ranges",
			"(document [0, 0] - [1, 0]\n  (number [0, 0] - [0, 2]))",
			"(document (number))",
		},
		{
			"extra whitespace",
			"(document\n  (object\n    (pair\n      (string)\n      (number))))",
			"(document (object (pair (string) (number))))",
		},
		{
			"empty",
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeCLIOutput(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeCLIOutput(%q)\n  got:  %s\n  want: %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestFindFirstDivergence(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
	}{
		{"identical", "(document (number))", "(document (number))"},
		{"different node", "(document (number))", "(document (string))"},
		{"extra child", "(document (a) (b))", "(document (a))"},
		{"length mismatch", "(doc)", "(document)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findFirstDivergence(tt.a, tt.b)
			if tt.a == tt.b {
				if result == "" {
					t.Error("findFirstDivergence returned empty for identical strings")
				}
			} else {
				if result == "" {
					t.Error("findFirstDivergence returned empty for different strings")
				}
				t.Logf("divergence: %s", result)
			}
		})
	}
}

func TestAbbreviate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short", "hello", 10, "hello"},
		{"exact", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := abbreviate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("abbreviate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestCompareResult_Divergence(t *testing.T) {
	// Test Compare with a mock parseFunc. The mock output differs from CLI output,
	// so we verify the divergence reporting works correctly.
	mockParse := func(sexp string) corpustest.ParseFunc {
		return func(input []byte) (string, error) {
			return sexp, nil
		}
	}

	result, err := Compare([]byte(`{"a": 1}`), "source.json", mockParse("(document (object (pair (string) (number))))"))
	if err != nil {
		t.Skipf("Compare requires CLI: %v", err)
	}

	t.Logf("Go (mock): %s", result.GoNorm)
	t.Logf("C (CLI):   %s", result.CNorm)

	// We don't assert Match here — the mock may not exactly match the CLI output.
	// This test verifies that Compare runs to completion and fills in the fields.
	if result.GoNorm == "" {
		t.Error("GoNorm is empty")
	}
	if result.CNorm == "" {
		t.Error("CNorm is empty")
	}
	if !result.Match && result.Divergence == "" {
		t.Error("Divergence is empty when Match is false")
	}
}

func TestWriteRegressionFile(t *testing.T) {
	dir := t.TempDir()

	err := WriteRegressionFile(dir, "test_case", []byte("input data"), "(expected)")
	if err != nil {
		t.Fatalf("WriteRegressionFile: %v", err)
	}

	inputPath := filepath.Join(dir, "test_case.input")
	expectedPath := filepath.Join(dir, "test_case.expected")

	input, err := os.ReadFile(inputPath)
	if err != nil {
		t.Fatalf("reading input file: %v", err)
	}
	if string(input) != "input data" {
		t.Errorf("input file content = %q, want %q", input, "input data")
	}

	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("reading expected file: %v", err)
	}
	if string(expected) != "(expected)" {
		t.Errorf("expected file content = %q, want %q", expected, "(expected)")
	}
}

func TestRunRegressionTests_WithFixtures(t *testing.T) {
	dir := t.TempDir()

	input := []byte("package main\n\nvar x = 1\n")

	parse := goParseFunc()
	sexp, err := parse(input)
	if err != nil {
		t.Fatal(err)
	}

	os.WriteFile(filepath.Join(dir, "var_decl.input"), input, 0644)
	os.WriteFile(filepath.Join(dir, "var_decl.expected"), []byte(sexp), 0644)

	RunRegressionTests(t, dir, parse)
}

func TestRunRegressionTests_MissingDir(t *testing.T) {
	parse := goParseFunc()
	RunRegressionTests(t, "/nonexistent/path/that/should/not/exist", parse)
}

// --- CLI integration tests (require tree-sitter CLI) ---

func TestParseBytesWithCLI_JSON(t *testing.T) {
	requireCLI(t)

	input := []byte(`{"name": "test", "value": 42}`)
	sexp, err := ParseBytesWithCLI(input, "source.json")
	if err != nil {
		t.Fatalf("ParseBytesWithCLI: %v", err)
	}

	if sexp == "" {
		t.Fatal("ParseBytesWithCLI returned empty output")
	}

	t.Logf("CLI output: %s", sexp)

	norm := NormalizeCLIOutput(sexp)
	if len(norm) < 5 || norm[0] != '(' {
		t.Errorf("normalized output doesn't look like S-expression: %q", norm)
	}
}

func TestParseWithCLI_ErrorInput(t *testing.T) {
	requireCLI(t)

	tmpFile, err := os.CreateTemp("", "difftest-error-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Write([]byte(`{"unclosed": `))
	tmpFile.Close()

	sexp, err := ParseWithCLI(tmpFile.Name(), "source.json")
	if err != nil {
		t.Fatalf("ParseWithCLI on malformed input: %v", err)
	}

	if sexp == "" {
		t.Fatal("expected non-empty output for malformed JSON")
	}
	t.Logf("CLI error recovery output: %s", sexp)
}

func TestCompareWithCLI_Go(t *testing.T) {
	requireCLIScope(t, "source.go")

	input := []byte("package main\n\nfunc main() {\n}\n")
	result, err := Compare(input, "source.go", goParseFunc())
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}

	t.Logf("Go parser: %s", result.GoNorm)
	t.Logf("C CLI:     %s", result.CNorm)
	t.Logf("Match:     %v", result.Match)

	if !result.Match {
		t.Errorf("Go and C parsers disagree:\n  Go:  %s\n  C:   %s\n  diff: %s",
			result.GoNorm, result.CNorm, result.Divergence)
	}
}

func TestRunDifferentialFile_Go(t *testing.T) {
	requireCLIScope(t, "source.go")

	dir := t.TempDir()
	goFile := filepath.Join(dir, "test.go")
	os.WriteFile(goFile, []byte("package foo\n\nvar x = 42\n"), 0644)

	RunDifferentialFile(t, goFile, goParseFunc())
}

func TestRunDifferentialDir_Go(t *testing.T) {
	requireCLIScope(t, "source.go")

	dir := t.TempDir()
	files := map[string]string{
		"a.go": "package a\n\nvar x int\n",
		"b.go": "package b\n\nfunc f() {}\n",
	}
	for name, content := range files {
		os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	}

	RunDifferentialDir(t, dir, []string{".go"}, goParseFunc())
}

func TestRunDifferentialCorpus_Go(t *testing.T) {
	requireCLIScope(t, "source.go")

	cases := []corpustest.TestCase{
		{
			Name:       "simple function",
			Input:      []byte("package main\n\nfunc f() {}\n"),
			Attributes: corpustest.TestAttributes{Platform: true},
		},
		{
			Name:       "variable declaration",
			Input:      []byte("package main\n\nvar x = 42\n"),
			Attributes: corpustest.TestAttributes{Platform: true},
		},
	}

	RunDifferentialCorpus(t, cases, "source.go", goParseFunc())
}
