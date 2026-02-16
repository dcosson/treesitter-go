package corpustest

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseCorpusFile_Basic(t *testing.T) {
	data, err := os.ReadFile("testdata/basic.txt")
	if err != nil {
		t.Fatal(err)
	}

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 2 {
		t.Fatalf("expected 2 test cases, got %d", len(cases))
	}

	// Test case 1: Arrays
	tc := cases[0]
	if tc.Name != "Arrays" {
		t.Errorf("expected name 'Arrays', got %q", tc.Name)
	}
	if string(tc.Input) != "[1, 2, 3]" {
		t.Errorf("unexpected input: %q", string(tc.Input))
	}
	expected := "(document (array (number) (number) (number)))"
	if tc.Expected != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, tc.Expected)
	}
	if tc.Attributes.Skip || tc.Attributes.Error {
		t.Error("expected no special attributes")
	}

	// Test case 2: Strings
	tc = cases[1]
	if tc.Name != "Strings" {
		t.Errorf("expected name 'Strings', got %q", tc.Name)
	}
	if string(tc.Input) != `["hello", "world"]` {
		t.Errorf("unexpected input: %q", string(tc.Input))
	}
	expected = "(document (array (string (string_content)) (string (string_content))))"
	if tc.Expected != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, tc.Expected)
	}
}

func TestParseCorpusFile_Attributes(t *testing.T) {
	data, err := os.ReadFile("testdata/attributes.txt")
	if err != nil {
		t.Fatal(err)
	}

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 4 {
		t.Fatalf("expected 4 test cases, got %d", len(cases))
	}

	// Normal test
	if cases[0].Name != "Normal test" {
		t.Errorf("expected 'Normal test', got %q", cases[0].Name)
	}
	if cases[0].Attributes.Skip || cases[0].Attributes.Error {
		t.Error("normal test should have no special attributes")
	}

	// Skipped test
	if cases[1].Name != "Skipped test" {
		t.Errorf("expected 'Skipped test', got %q", cases[1].Name)
	}
	if !cases[1].Attributes.Skip {
		t.Error("expected skip=true")
	}
	// :skip implies error=false
	if cases[1].Attributes.Error {
		t.Error("skip should suppress error attribute")
	}

	// Error test
	if cases[2].Name != "Error test" {
		t.Errorf("expected 'Error test', got %q", cases[2].Name)
	}
	if !cases[2].Attributes.Error {
		t.Error("expected error=true")
	}

	// Platform test
	if cases[3].Name != "Platform test" {
		t.Errorf("expected 'Platform test', got %q", cases[3].Name)
	}
	// Platform should be true only on linux.
	expectPlatform := runtime.GOOS == "linux"
	if cases[3].Attributes.Platform != expectPlatform {
		t.Errorf("expected platform=%v on %s, got %v", expectPlatform, runtime.GOOS, cases[3].Attributes.Platform)
	}
}

func TestParseCorpusFile_ShortDelimiters(t *testing.T) {
	data, err := os.ReadFile("testdata/short_delimiters.txt")
	if err != nil {
		t.Fatal(err)
	}

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 2 {
		t.Fatalf("expected 2 test cases, got %d", len(cases))
	}

	if cases[0].Name != "Short delimiters" {
		t.Errorf("expected 'Short delimiters', got %q", cases[0].Name)
	}
	if cases[0].Expected != "(document (null))" {
		t.Errorf("unexpected expected: %q", cases[0].Expected)
	}

	if cases[1].Name != "Multiple top-level objects" {
		t.Errorf("expected 'Multiple top-level objects', got %q", cases[1].Name)
	}
	if cases[1].Expected != "(document (object) (object))" {
		t.Errorf("unexpected expected: %q", cases[1].Expected)
	}
}

func TestParseCorpusFile_HyphensInSource(t *testing.T) {
	data, err := os.ReadFile("testdata/hyphens_in_source.txt")
	if err != nil {
		t.Fatal(err)
	}

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(cases))
	}

	tc := cases[0]
	if tc.Name != "Source with hyphens" {
		t.Errorf("expected 'Source with hyphens', got %q", tc.Name)
	}

	// The input should include the short --- line and the "NOT the divider" text,
	// because the longest divider (the 80-char one) is the real separator.
	if !strings.Contains(string(tc.Input), "---") {
		t.Error("input should contain the short --- line")
	}
	if !strings.Contains(string(tc.Input), "NOT the divider") {
		t.Error("input should contain the 'NOT the divider' text")
	}

	// Expected output should be properly normalized.
	expected := "(document (object (pair (string (string_content)) (string (string_content))) (pair (string (string_content)) (string (string_content)))))"
	if tc.Expected != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, tc.Expected)
	}
}

func TestParseCorpusFile_CommentsStripped(t *testing.T) {
	data, err := os.ReadFile("testdata/with_comments.txt")
	if err != nil {
		t.Fatal(err)
	}

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(cases))
	}

	// Comments should be stripped from the expected output.
	expected := "(document (array (number) (number)))"
	if cases[0].Expected != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, cases[0].Expected)
	}
}

func TestParseCorpusFile_FieldDetection(t *testing.T) {
	data, err := os.ReadFile("testdata/with_fields.txt")
	if err != nil {
		t.Fatal(err)
	}

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(cases))
	}

	if !cases[0].HasFields {
		t.Error("expected HasFields=true for output with field annotations")
	}

	expected := "(program (function_declaration name: (identifier) body: (statement_block)))"
	if cases[0].Expected != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, cases[0].Expected)
	}
}

func TestParseCorpusDir(t *testing.T) {
	cases, err := ParseCorpusDir("testdata")
	if err != nil {
		t.Fatal(err)
	}

	// Should find tests from all .txt files in testdata/.
	if len(cases) < 5 {
		t.Errorf("expected at least 5 test cases from testdata/, got %d", len(cases))
	}

	// Verify we got tests from multiple files by checking names.
	names := make(map[string]bool)
	for _, tc := range cases {
		names[tc.Name] = true
	}
	for _, want := range []string{"Arrays", "Strings", "Normal test", "Short delimiters", "Source with hyphens"} {
		if !names[want] {
			t.Errorf("missing test case %q", want)
		}
	}
}

func TestNormalizeSExpression(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		hasFields bool
	}{
		{
			name:  "basic normalization",
			input: "(document\n  (array\n    (number)\n    (number)))",
			want:  "(document (array (number) (number)))",
		},
		{
			name:  "comment stripping",
			input: "; comment\n(document\n  ; another\n  (object))",
			want:  "(document (object))",
		},
		{
			name:  "space before close paren",
			input: "(document (object )  )",
			want:  "(document (object))",
		},
		{
			name:      "field annotations preserved",
			input:     "(function_declaration\n  name: (identifier)\n  body: (block))",
			want:      "(function_declaration name: (identifier) body: (block))",
			hasFields: true,
		},
		{
			name:  "point annotations stripped",
			input: "(document [0, 0] - [1, 0]\n  (number [0, 0] - [0, 3]))",
			want:  "(document (number))",
		},
		{
			name:  "multiple whitespace collapsed",
			input: "(document    (array\n\n    (number)\n\t\t(number)))",
			want:  "(document (array (number) (number)))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, hasFields := NormalizeSExpression(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeSExpression(%q)\n  got:  %s\n  want: %s", tt.input, got, tt.want)
			}
			if hasFields != tt.hasFields {
				t.Errorf("HasFields: got %v, want %v", hasFields, tt.hasFields)
			}
		})
	}
}

func TestStripFields(t *testing.T) {
	input := "(function_declaration name: (identifier) body: (block))"
	want := "(function_declaration (identifier) (block))"
	got := StripFields(input)
	if got != want {
		t.Errorf("StripFields(%q)\n  got:  %s\n  want: %s", input, got, want)
	}
}

func TestParseCorpusFile_Empty(t *testing.T) {
	cases, err := ParseCorpusFile([]byte{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Errorf("expected 0 cases for empty input, got %d", len(cases))
	}
}

func TestParseCorpusFile_NoHeaders(t *testing.T) {
	cases, err := ParseCorpusFile([]byte("just some random text\nno headers here\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Errorf("expected 0 cases for input without headers, got %d", len(cases))
	}
}

func TestParseCorpusFile_LanguageAttribute(t *testing.T) {
	data := []byte(`================================================================================
Multi-language test
:language(javascript)
================================================================================

var x = 1;

---

(program (variable_declaration))
`)

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 1 {
		t.Fatalf("expected 1 test case, got %d", len(cases))
	}

	if len(cases[0].Attributes.Languages) != 1 || cases[0].Attributes.Languages[0] != "javascript" {
		t.Errorf("expected languages=[javascript], got %v", cases[0].Attributes.Languages)
	}
}

func TestParseCorpusFile_InputCopied(t *testing.T) {
	// Verify that modifications to the returned Input don't affect the original data.
	data := []byte("===\nTest\n===\n\nfoo\n\n---\n\n(x)\n")
	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 1 {
		t.Fatalf("expected 1 case, got %d", len(cases))
	}
	if string(cases[0].Input) != "foo" {
		t.Fatalf("expected input 'foo', got %q", string(cases[0].Input))
	}
	// Modify the returned input.
	cases[0].Input[0] = 'Z'
	// Original data should be unmodified — find "foo" in the original.
	origStr := string(data)
	if !strings.Contains(origStr, "foo") {
		t.Error("modifying Input should not affect original data")
	}
}

func TestParseCorpusFile_RealJSONCorpus(t *testing.T) {
	// Test against a snapshot of the actual tree-sitter-json corpus format,
	// matching what we fetched from the real repo.
	data := []byte(`================================================================================
Arrays
================================================================================

[
  345,
  10.1,
  10,
  -10,
  null,
  true,
  false,
  { "stuff": "good" }
]

--------------------------------------------------------------------------------

(document
  (array
    (number)
    (number)
    (number)
    (number)
    (null)
    (true)
    (false)
    (object
      (pair
        (string
          (string_content))
        (string
          (string_content))))))

================================================================================
String content
================================================================================

[
  "",
  "abc",
  "def\n",
  "ghi\t",
  "jkl\f",
  "//",
  "/**/"
]

--------------------------------------------------------------------------------

(document
  (array
    (string)
    (string
      (string_content))
    (string
      (string_content)
      (escape_sequence))
    (string
      (string_content)
      (escape_sequence))
    (string
      (string_content)
      (escape_sequence))
    (string
      (string_content))
    (string
      (string_content))))

===========================================
Multiple top-level objects
===========================================

{}
{}

---

(document
  (object)
  (object))
`)

	cases, err := ParseCorpusFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if len(cases) != 3 {
		t.Fatalf("expected 3 test cases, got %d", len(cases))
	}

	// Verify Arrays test
	if cases[0].Name != "Arrays" {
		t.Errorf("case[0] name: got %q, want 'Arrays'", cases[0].Name)
	}
	if !strings.HasPrefix(string(cases[0].Input), "[") {
		t.Error("case[0] input should start with [")
	}
	if !strings.Contains(cases[0].Expected, "(document (array") {
		t.Error("case[0] expected should contain (document (array")
	}

	// Verify that the short-delimiter test at the end parses correctly
	if cases[2].Name != "Multiple top-level objects" {
		t.Errorf("case[2] name: got %q, want 'Multiple top-level objects'", cases[2].Name)
	}
	if cases[2].Expected != "(document (object) (object))" {
		t.Errorf("case[2] expected: got %q", cases[2].Expected)
	}
}

func TestParseCorpusDir_Nonexistent(t *testing.T) {
	_, err := ParseCorpusDir(filepath.Join("testdata", "nonexistent"))
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}

func TestParseCorpusDir_FetchedJSONGrammar(t *testing.T) {
	// Integration test: parse the real tree-sitter-json corpus if it has been fetched.
	// Run `make fetch-test-grammars` first to populate testdata/grammars/.
	corpusDir := filepath.Join("..", "..", "testdata", "grammars", "tree-sitter-json", "test", "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		t.Skip("tree-sitter-json corpus not fetched; run `make fetch-test-grammars` first")
	}

	cases, err := ParseCorpusDir(corpusDir)
	if err != nil {
		t.Fatal(err)
	}

	// The real JSON grammar has 6 test cases in main.txt.
	if len(cases) < 5 {
		t.Errorf("expected at least 5 test cases from real JSON corpus, got %d", len(cases))
	}

	// Verify we parsed known test names.
	names := make(map[string]bool)
	for _, tc := range cases {
		names[tc.Name] = true
		// Every test case should have non-empty expected output.
		if tc.Expected == "" {
			t.Errorf("test %q has empty expected output", tc.Name)
		}
		// All JSON corpus tests should start with "(document".
		if !strings.HasPrefix(tc.Expected, "(document") {
			t.Errorf("test %q expected output should start with (document, got: %s", tc.Name, tc.Expected[:min(50, len(tc.Expected))])
		}
	}

	for _, want := range []string{"Arrays", "String content", "Top-level numbers", "Top-level null", "Comments"} {
		if !names[want] {
			t.Errorf("missing expected test case %q", want)
		}
	}

	t.Logf("parsed %d test cases from real JSON corpus", len(cases))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
