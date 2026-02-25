package corpustest

import (
	"strings"
	"testing"
)

// Runner executes corpus test cases against a parser.
// It is parameterized by a ParseFunc that parses input bytes and returns
// an S-expression string of the parse tree.
//
// This design decouples the corpus test infrastructure from the actual
// parser types (which are in package treesitter), avoiding an import cycle.
// When the parser is ready, callers pass a closure like:
//
//	corpustest.RunCorpus(t, cases, func(input []byte) (string, error) {
//	    tree := parser.ParseString(ctx, nil, input)
//	    return tree.RootNode().String(), nil
//	})
type ParseFunc func(input []byte) (sexp string, err error)

// RunCorpusWithLanguages runs test cases using language-specific parse functions.
// The langParsers map keys are language names matching :language(name) attributes
// in corpus files. The empty string "" key is the default parser used when no
// :language attribute is specified (or when the attribute matches no key).
func RunCorpusWithLanguages(t *testing.T, cases []TestCase, langParsers map[string]ParseFunc) {
	t.Helper()
	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.Name, func(t *testing.T) {
			t.Helper()

			// Determine which parser to use based on :language attribute.
			var parse ParseFunc
			for _, lang := range tc.Attributes.Languages {
				if p, ok := langParsers[lang]; ok {
					parse = p
					break
				}
			}
			if parse == nil {
				parse = langParsers[""]
			}
			if parse == nil {
				t.Skip("no parser available for language(s): ", tc.Attributes.Languages)
				return
			}

			runSingleCorpusTest(t, tc, parse)
		})
	}
}

// RunCorpus runs all test cases using the provided parse function.
func RunCorpus(t *testing.T, cases []TestCase, parse ParseFunc) {
	t.Helper()
	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.Name, func(t *testing.T) {
			t.Helper()
			runSingleCorpusTest(t, tc, parse)
		})
	}
}

// runSingleCorpusTest executes a single corpus test case with the given parse function.
func runSingleCorpusTest(t *testing.T, tc TestCase, parse ParseFunc) {
	t.Helper()
	if tc.Attributes.Skip {
		t.Skip("corpus test marked :skip")
	}
	if !tc.Attributes.Platform {
		t.Skip("corpus test not for this platform")
	}

	actual, err := parse(tc.Input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Normalize the actual output the same way we normalized expected.
	var normalizedActual string
	if tc.Attributes.CST {
		normalizedActual = strings.TrimSpace(actual)
	} else {
		normalizedActual, _ = normalizeSExpression(actual)
	}

	// When the expected output has field annotations, compare with
	// fields (our parser now emits them). When the expected output
	// does not have fields, strip fields from the actual output
	// so the comparison is field-blind.
	expected := tc.Expected
	if !tc.Attributes.CST && !tc.HasFields {
		normalizedActual = StripFields(normalizedActual)
	}

	if tc.Attributes.Error {
		// For :error tests, verify ERROR or MISSING appears.
		if !containsErrorNode(normalizedActual) {
			t.Errorf("expected ERROR or MISSING node in parse tree\nactual: %s", normalizedActual)
		}
		// Still compare the tree structure if expected is provided.
		if expected != "" && normalizedActual != expected {
			t.Errorf("parse tree mismatch (error test)\nexpected:\n  %s\nactual:\n  %s",
				expected, normalizedActual)
		}
		return
	}

	if normalizedActual != expected {
		t.Errorf("parse tree mismatch\ninput:\n  %s\nexpected:\n  %s\nactual:\n  %s",
			abbreviate(string(tc.Input), 200), expected, normalizedActual)
	}
}

// containsErrorNode checks if an S-expression contains ERROR or MISSING nodes.
func containsErrorNode(sexp string) bool {
	return strings.Contains(sexp, "(ERROR") || strings.Contains(sexp, "MISSING")
}

// abbreviate truncates a string to maxLen, appending "..." if truncated.
func abbreviate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
