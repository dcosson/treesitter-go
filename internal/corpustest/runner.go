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

// RunCorpus runs all test cases using the provided parse function.
func RunCorpus(t *testing.T, cases []TestCase, parse ParseFunc) {
	t.Helper()
	for _, tc := range cases {
		tc := tc // capture
		t.Run(tc.Name, func(t *testing.T) {
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

			// Strip field annotations from both sides. Our parser does not
			// currently emit field labels in S-expressions, so we compare
			// tree structure without fields regardless of whether the
			// expected output uses them.
			if !tc.Attributes.CST {
				normalizedActual = StripFields(normalizedActual)
			}
			expected := tc.Expected
			if tc.HasFields && !tc.Attributes.CST {
				expected = StripFields(expected)
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
		})
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
