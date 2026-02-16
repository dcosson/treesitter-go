package corpustest

import (
	"fmt"
	"strings"
	"testing"
)

// mockParse returns a ParseFunc that always produces the given S-expression.
func mockParse(sexp string) ParseFunc {
	return func(input []byte) (string, error) {
		return sexp, nil
	}
}

// mockParseErr returns a ParseFunc that always returns an error.
func mockParseErr() ParseFunc {
	return func(input []byte) (string, error) {
		return "", fmt.Errorf("parse failed")
	}
}

func TestRunCorpus_Pass(t *testing.T) {
	cases := []TestCase{
		{
			Name:     "simple",
			Input:    []byte("[1]"),
			Expected: "(document (array (number)))",
			Attributes: TestAttributes{
				Platform:  true,
				Languages: []string{""},
			},
		},
	}

	// Should pass: mock parse returns matching output.
	RunCorpus(t, cases, mockParse("(document (array (number)))"))
}

func TestRunCorpus_SkippedTest(t *testing.T) {
	skippedRan := false
	cases := []TestCase{
		{
			Name:     "skipped",
			Input:    []byte("whatever"),
			Expected: "(x)",
			Attributes: TestAttributes{
				Skip:      true,
				Platform:  true,
				Languages: []string{""},
			},
		},
	}

	parse := func(input []byte) (string, error) {
		skippedRan = true
		return "(wrong)", nil
	}

	// Run in a sub-test so we can inspect skipped status.
	tt := &testing.T{}
	_ = tt // Just verify it doesn't panic with the real test runner.
	RunCorpus(t, cases, parse)

	if skippedRan {
		t.Error("skipped test should not have called the parse function")
	}
}

func TestRunCorpus_ErrorTest(t *testing.T) {
	cases := []TestCase{
		{
			Name:     "error expected",
			Input:    []byte("{bad"),
			Expected: "(document (object (ERROR)))",
			Attributes: TestAttributes{
				Error:     true,
				Platform:  true,
				Languages: []string{""},
			},
		},
	}

	// Should pass: mock parse returns output with ERROR node.
	RunCorpus(t, cases, mockParse("(document\n  (object\n    (ERROR)))"))
}

func TestRunCorpus_FieldStripping(t *testing.T) {
	cases := []TestCase{
		{
			Name:      "no fields in expected",
			Input:     []byte("func foo() {}"),
			Expected:  "(program (function_declaration (identifier) (statement_block)))",
			HasFields: false,
			Attributes: TestAttributes{
				Platform:  true,
				Languages: []string{""},
			},
		},
	}

	// Parser returns output with fields, but expected has no fields.
	// The runner should strip fields from actual before comparing.
	RunCorpus(t, cases, mockParse("(program (function_declaration name: (identifier) body: (statement_block)))"))
}

func TestRunCorpus_PlatformSkip(t *testing.T) {
	parseRan := false
	cases := []TestCase{
		{
			Name:     "other platform",
			Input:    []byte("x"),
			Expected: "(x)",
			Attributes: TestAttributes{
				Platform:  false,
				Languages: []string{""},
			},
		},
	}

	parse := func(input []byte) (string, error) {
		parseRan = true
		return "(x)", nil
	}

	RunCorpus(t, cases, parse)

	if parseRan {
		t.Error("platform-excluded test should not have called the parse function")
	}
}

func TestContainsErrorNode(t *testing.T) {
	tests := []struct {
		sexp string
		want bool
	}{
		{"(document (object))", false},
		{"(document (ERROR))", true},
		{"(document (ERROR (identifier)))", true},
		{"(document (object (MISSING \";\" )))", true},
		{"(document (array (number)))", false},
	}

	for _, tt := range tests {
		got := containsErrorNode(tt.sexp)
		if got != tt.want {
			t.Errorf("containsErrorNode(%q) = %v, want %v", tt.sexp, got, tt.want)
		}
	}
}

func TestAbbreviate(t *testing.T) {
	if got := abbreviate("short", 10); got != "short" {
		t.Errorf("expected 'short', got %q", got)
	}
	long := strings.Repeat("a", 300)
	got := abbreviate(long, 200)
	if len(got) != 203 { // 200 + "..."
		t.Errorf("expected length 203, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Error("expected '...' suffix")
	}
}
