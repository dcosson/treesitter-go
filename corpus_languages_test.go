package treesitter_test

import (
	"context"
	"fmt"
	iparser "github.com/treesitter-go/treesitter/parser"
	"os"
	"path/filepath"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	"github.com/treesitter-go/treesitter/internal/corpustest"
	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	cgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cgrammar"
	cppgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cppgrammar"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/java"
	perlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/perl"
	rubygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/ruby"
	rustgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/rustgrammar"
	bashscanner "github.com/treesitter-go/treesitter/scanners/bash"
	cppscanner "github.com/treesitter-go/treesitter/scanners/cpp"
	cssscanner "github.com/treesitter-go/treesitter/scanners/css"
	htmlscanner "github.com/treesitter-go/treesitter/scanners/html"
	perlscanner "github.com/treesitter-go/treesitter/scanners/perl"
	rubyscanner "github.com/treesitter-go/treesitter/scanners/ruby"
	rustscanner "github.com/treesitter-go/treesitter/scanners/rust"
)

// perTestTimeout is the maximum time allowed for parsing a single corpus test input.
const perTestTimeout = 10 * time.Second
const corpusOverridesPath = "testdata/corpus-overrides.json"

// makeCorpusParseFunc creates a ParseFunc for the given language with a per-test timeout.
func makeCorpusParseFunc(lang *ts.Language) corpustest.ParseFunc {
	return func(input []byte) (sexp string, err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("panic during parse: %v", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), perTestTimeout)
		defer cancel()
		p := iparser.NewParser()
		p.SetLanguage(lang)
		tree := p.ParseString(ctx, input)
		if tree == nil {
			return "", nil
		}
		return tree.RootNode().String(), nil
	}
}

// runCorpusForLanguage is a shared helper that loads corpus test cases
// from the given grammar repo subdirectory and runs them.
func runCorpusForLanguage(t *testing.T, repoName string, lang *ts.Language) {
	t.Helper()
	corpusDir := filepath.Join(corpusGrammarsDir(), repoName, "test", "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		t.Skipf("%s corpus not found at %s — run 'make fetch-test-grammars' first", repoName, corpusDir)
	}

	cases, err := corpustest.ParseCorpusDir(corpusDir)
	if err != nil {
		t.Fatalf("failed to parse corpus: %v", err)
	}
	cases = applyCorpusOverrides(t, repoName, cases)

	if len(cases) == 0 {
		t.Fatal("no corpus test cases found")
	}
	t.Logf("loaded %d corpus test cases for %s", len(cases), repoName)

	corpustest.RunCorpus(t, cases, makeCorpusParseFunc(lang))
}

func applyCorpusOverrides(t *testing.T, repoName string, cases []corpustest.TestCase) []corpustest.TestCase {
	t.Helper()

	overrides, err := corpustest.ParseOverridesFile(corpusOverridesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return cases
		}
		t.Fatalf("failed to parse corpus overrides: %v", err)
	}

	updated, err := corpustest.ApplyOverrides(cases, repoName, overrides)
	if err != nil {
		t.Fatalf("failed to apply corpus overrides: %v", err)
	}
	return updated
}

// TestCorpusC runs the tree-sitter-c corpus tests.
func TestCorpusC(t *testing.T) {
	lang := cgrammar.CLanguage()
	runCorpusForLanguage(t, "tree-sitter-c", lang)
}

// TestCorpusCpp runs the tree-sitter-cpp corpus tests.
func TestCorpusCpp(t *testing.T) {
	lang := cppgrammar.CppLanguage()
	lang.NewExternalScanner = cppscanner.New
	runCorpusForLanguage(t, "tree-sitter-cpp", lang)
}

// TestCorpusRust runs the tree-sitter-rust corpus tests.
func TestCorpusRust(t *testing.T) {
	lang := rustgrammar.RustLanguage()
	lang.NewExternalScanner = rustscanner.New
	runCorpusForLanguage(t, "tree-sitter-rust", lang)
}

// TestCorpusBash runs the tree-sitter-bash corpus tests.
func TestCorpusBash(t *testing.T) {
	lang := bashgrammar.BashLanguage()
	lang.NewExternalScanner = bashscanner.New
	runCorpusForLanguage(t, "tree-sitter-bash", lang)
}

// TestCorpusRuby runs the tree-sitter-ruby corpus tests.
func TestCorpusRuby(t *testing.T) {
	lang := rubygrammar.RubyLanguage()
	lang.NewExternalScanner = rubyscanner.New
	runCorpusForLanguage(t, "tree-sitter-ruby", lang)
}

// TestCorpusPerl runs the tree-sitter-perl corpus tests.
func TestCorpusPerl(t *testing.T) {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New
	runCorpusForLanguage(t, "tree-sitter-perl", lang)
}

// TestCorpusCSS runs the tree-sitter-css corpus tests.
func TestCorpusCSS(t *testing.T) {
	lang := cssgrammar.CssLanguage()
	lang.NewExternalScanner = cssscanner.New
	runCorpusForLanguage(t, "tree-sitter-css", lang)
}

// TestCorpusHTML runs the tree-sitter-html corpus tests.
func TestCorpusHTML(t *testing.T) {
	lang := htmlgrammar.HtmlLanguage()
	lang.NewExternalScanner = htmlscanner.New
	runCorpusForLanguage(t, "tree-sitter-html", lang)
}

// TestCorpusJava runs the tree-sitter-java corpus tests.
func TestCorpusJava(t *testing.T) {
	lang := javagrammar.JavaLanguage()
	runCorpusForLanguage(t, "tree-sitter-java", lang)
}

// TestCorpusGo runs the tree-sitter-go corpus tests.
func TestCorpusGo(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-go", goLang())
}

// TestCorpusPython runs the tree-sitter-python corpus tests.
func TestCorpusPython(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-python", pyLang())
}

// TestCorpusJavaScript runs the tree-sitter-javascript corpus tests.
func TestCorpusJavaScript(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-javascript", jsLang())
}

// TestCorpusTypeScript runs the tree-sitter-typescript corpus tests.
// The TypeScript corpus includes tests for both TypeScript and TSX grammars,
// distinguished by :language(typescript) and :language(tsx) attributes.
func TestCorpusTypeScript(t *testing.T) {
	t.Helper()
	corpusDir := filepath.Join(corpusGrammarsDir(), "tree-sitter-typescript", "test", "corpus")
	if _, err := os.Stat(corpusDir); os.IsNotExist(err) {
		t.Skipf("tree-sitter-typescript corpus not found at %s — run 'make fetch-test-grammars' first", corpusDir)
	}

	cases, err := corpustest.ParseCorpusDir(corpusDir)
	if err != nil {
		t.Fatalf("failed to parse corpus: %v", err)
	}
	cases = applyCorpusOverrides(t, "tree-sitter-typescript", cases)
	if len(cases) == 0 {
		t.Fatal("no corpus test cases found")
	}
	t.Logf("loaded %d corpus test cases for tree-sitter-typescript", len(cases))

	langParsers := map[string]corpustest.ParseFunc{
		"":           makeCorpusParseFunc(newTSLang()),
		"typescript": makeCorpusParseFunc(newTSLang()),
		"tsx":        makeCorpusParseFunc(newTSXLang()),
	}
	corpustest.RunCorpusWithLanguages(t, cases, langParsers)
}

// TestCorpusLua runs the tree-sitter-lua corpus tests.
func TestCorpusLua(t *testing.T) {
	runCorpusForLanguage(t, "tree-sitter-lua", luaLang())
}

// TestPerlFunctionCallExpression tests that parenthesized Perl function calls
// like foo() produce function_call_expression (not ambiguous_function_call_expression).
// This is a regression test for a bug where doAccept's selectTree logic used
// >= for dynamic precedence comparison instead of >, causing the last-accepted
// GLR version to always win. C tree-sitter's ts_parser__select_tree uses
// ts_subtree_compare as a final tiebreaker when costs and dynPrec are equal.
func TestPerlFunctionCallExpression(t *testing.T) {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New
	parseFn := makeCorpusParseFunc(lang)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "zero args",
			input:    "foo();",
			expected: "(source_file (expression_statement (function_call_expression (function))))",
		},
		{
			name:     "one arg",
			input:    "foo(123);",
			expected: "(source_file (expression_statement (function_call_expression (function) (number))))",
		},
		{
			name:     "two args",
			input:    "foo(12, 34);",
			expected: "(source_file (expression_statement (function_call_expression (function) (list_expression (number) (number)))))",
		},
		{
			name:     "sort with unary plus function call",
			input:    "sort +returns_list(1, 2, 3);",
			expected: "(source_file (expression_statement (sort_expression (unary_expression (function_call_expression (function) (list_expression (number) (number) (number)))))))",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := parseFn([]byte(tc.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			normalizedActualRaw, _ := corpustest.NormalizeSExpression(actual)
			normalizedActual := corpustest.StripFields(normalizedActualRaw)
			normalizedExpected, _ := corpustest.NormalizeSExpression(tc.expected)
			if normalizedActual != normalizedExpected {
				t.Errorf("function_call_expression mismatch\ninput: %s\nexpected: %s\nactual:   %s",
					tc.input, normalizedExpected, normalizedActual)
			}
		})
	}
}

// TestCppNestedQualifiedIdentifier tests that multi-level namespace
// qualifications like a::b::c produce nested qualified_identifier nodes
// rather than being flattened. This is a regression test for a bug where
// doReduce unconditionally marked reduced nodes as "extra" when
// gotoState == baseState, without checking endOfNonTerminalExtra.
func TestCppNestedQualifiedIdentifier(t *testing.T) {
	lang := cppgrammar.CppLanguage()
	lang.NewExternalScanner = cppscanner.New
	parseFn := makeCorpusParseFunc(lang)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:  "two-level namespace",
			input: "a::b::c = 1;",
			expected: "(translation_unit (expression_statement (assignment_expression" +
				" (qualified_identifier (namespace_identifier) (qualified_identifier (namespace_identifier) (identifier)))" +
				" (number_literal))))",
		},
		{
			name:  "three-level namespace in using",
			input: "using ::e::f::g;",
			expected: "(translation_unit (using_declaration" +
				" (qualified_identifier (qualified_identifier (namespace_identifier) (qualified_identifier (namespace_identifier) (identifier))))))",
		},
		{
			name:  "template in scope position",
			input: "std::vector<int>::size_typ my_string;",
			expected: "(translation_unit (declaration" +
				" (qualified_identifier (namespace_identifier) (qualified_identifier (template_type (type_identifier) (template_argument_list (type_descriptor (primitive_type)))) (type_identifier)))" +
				" (identifier)))",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := parseFn([]byte(tc.input))
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			normalizedActualRaw, _ := corpustest.NormalizeSExpression(actual)
			normalizedActual := corpustest.StripFields(normalizedActualRaw)
			normalizedExpected, _ := corpustest.NormalizeSExpression(tc.expected)
			if normalizedActual != normalizedExpected {
				t.Errorf("nested qualified_identifier mismatch\ninput: %s\nexpected: %s\nactual:   %s",
					tc.input, normalizedExpected, normalizedActual)
			}
		})
	}
}
