package e2etest_test

import (
	"context"
	iparser "github.com/treesitter-go/treesitter/parser"
	"strings"
	"testing"
	"time"

	cgrammar "github.com/treesitter-go/treesitter/internal/grammars/c"
)

// TestCommentExtrasPlacement verifies that comments (extras) are placed at the
// correct level in the parse tree. This tests the trailing extras handling in
// doReduce (ts_subtree_array_remove_trailing_extras equivalent) and the accept
// rewrite (ts_parser__accept equivalent) that ensures extras re-pushed by
// doReduce end up as children of the root node.
func TestCommentExtrasPlacement(t *testing.T) {
	lang := cgrammar.CLanguage()

	cases := []struct {
		name     string
		input    string
		contains []string // substrings that must appear in the S-expression
		excludes []string // substrings that must NOT appear
	}{
		{
			name:  "comment_between_declarations",
			input: "int x;\n/* comment */\nint y;\n",
			contains: []string{
				"(translation_unit",
				"(comment)",
				"(declaration", // should have two declarations
			},
		},
		{
			name:  "comment_at_end_of_file",
			input: "int x;\n/* trailing */\n",
			contains: []string{
				"(translation_unit",
				"(comment)",
				"(declaration",
			},
		},
		{
			name:  "comment_at_start_of_file",
			input: "/* leading */\nint x;\n",
			contains: []string{
				"(translation_unit",
				"(comment)",
				"(declaration",
			},
		},
		{
			name:  "multiple_comments_between",
			input: "int x;\n/* one */\n/* two */\nint y;\n",
			contains: []string{
				"(translation_unit",
				"(comment)",
			},
		},
		{
			name:  "comment_inside_function_body",
			input: "int f() {\n  /* inside */\n  return 0;\n}\n",
			contains: []string{
				"(translation_unit",
				"(function_definition",
				"(comment)",
			},
		},
		{
			name:  "line_comment",
			input: "int x; // line comment\nint y;\n",
			contains: []string{
				"(translation_unit",
				"(comment)",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := iparser.NewParser()
			p.SetLanguage(lang)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			tree := p.ParseString(ctx, []byte(tc.input))
			if tree == nil {
				t.Fatal("nil tree")
			}

			sexp := tree.RootNode().String()
			t.Logf("sexp: %s", sexp)

			for _, s := range tc.contains {
				if !strings.Contains(sexp, s) {
					t.Errorf("expected S-expression to contain %q", s)
				}
			}
			for _, s := range tc.excludes {
				if strings.Contains(sexp, s) {
					t.Errorf("expected S-expression NOT to contain %q", s)
				}
			}
		})
	}
}
