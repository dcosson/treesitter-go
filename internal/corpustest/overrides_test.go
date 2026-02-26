package corpustest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOverridesFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "overrides.json")
	content := `{
  "tree-sitter-perl": {
    "Double dollar edge cases": {
      "expected": "(source_file (ERROR (ERROR)))"
    }
  }
}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write overrides file: %v", err)
	}

	overrides, err := ParseOverridesFile(path)
	if err != nil {
		t.Fatalf("ParseOverridesFile failed: %v", err)
	}
	if _, ok := overrides["tree-sitter-perl"]["Double dollar edge cases"]; !ok {
		t.Fatal("expected perl override to be present")
	}
}

func TestApplyOverrides(t *testing.T) {
	t.Parallel()

	cases := []TestCase{
		{
			Name:     "Double dollar edge cases",
			Input:    []byte("$$';\n"),
			Expected: "(source_file (ERROR (UNEXPECTED ''')))",
		},
		{
			Name:     "another",
			Input:    []byte("x\n"),
			Expected: "(source_file)",
		},
	}

	overrides := CorpusOverrides{
		"tree-sitter-perl": {
			"Double dollar edge cases": {
				Expected: "(source_file (ERROR (ERROR)))",
			},
			"another": {
				Skip: true,
			},
		},
	}

	got, err := ApplyOverrides(cases, "tree-sitter-perl", overrides)
	if err != nil {
		t.Fatalf("ApplyOverrides failed: %v", err)
	}

	if got[0].Expected != "(source_file (ERROR (ERROR)))" {
		t.Fatalf("unexpected overridden expected: %q", got[0].Expected)
	}
	if !got[1].Attributes.Skip {
		t.Fatal("expected second test case to be marked skip")
	}
}
