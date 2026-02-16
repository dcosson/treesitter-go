package treesitter_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	cssscanner "github.com/treesitter-go/treesitter/scanners/css"
	htmlscanner "github.com/treesitter-go/treesitter/scanners/html"
	_ "github.com/treesitter-go/treesitter/internal/testgrammars/html"
	_ "github.com/treesitter-go/treesitter/internal/testgrammars/java"
	_ "github.com/treesitter-go/treesitter/scanners/html"
)

// Verify scanner packages compile.
var _ = htmlscanner.New
var _ = cssscanner.New

func cssLang() *ts.Language {
	lang := cssgrammar.CssLanguage()
	lang.NewExternalScanner = cssscanner.New
	return lang
}

func TestCSSParseSimpleRule(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(cssLang())

	src := "body { color: red; }"
	tree := p.ParseString(context.Background(), []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "stylesheet" {
		t.Errorf("root type = %q, want %q", root.Type(), "stylesheet")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "rule_set") {
		t.Errorf("expected rule_set: %s", sexp)
	}
}

// TODO(0ab): Enable once parser handleError/external-scanner interaction
// is fixed — currently the parser enters an infinite loop in error recovery
// for inputs requiring external scanner tokens (descendant_op, implicit_end_tag).
// Tests for: CSS descendant selector, pseudo-class, media queries;
// HTML elements, self-closing tags, script/style, void elements;
// Java hello world, interface, enum, generics.
