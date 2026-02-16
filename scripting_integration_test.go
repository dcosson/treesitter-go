package treesitter_test

import (
	"context"
	"strings"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	cssgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/css"
	htmlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/html"
	javagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/java"
	cssscanner "github.com/treesitter-go/treesitter/scanners/css"
	htmlscanner "github.com/treesitter-go/treesitter/scanners/html"
)

func cssLang() *ts.Language {
	lang := cssgrammar.CssLanguage()
	lang.NewExternalScanner = cssscanner.New
	return lang
}

func htmlLang() *ts.Language {
	lang := htmlgrammar.HtmlLanguage()
	lang.NewExternalScanner = htmlscanner.New
	return lang
}

func javaLang() *ts.Language {
	return javagrammar.JavaLanguage()
}

// --- CSS Integration Tests ---

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

func TestCSSParseDescendantSelector(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(cssLang())

	src := "div p { color: blue; }"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil (possible timeout — parser may be looping)")
	}
	root := tree.RootNode()
	sexp := root.String()
	if !strings.Contains(sexp, "descendant_selector") {
		t.Errorf("expected descendant_selector in: %s", sexp)
	}
}

func TestCSSParsePseudoClass(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(cssLang())

	src := "a:hover { text-decoration: underline; }"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexp := root.String()
	if !strings.Contains(sexp, "pseudo_class_selector") {
		t.Errorf("expected pseudo_class_selector in: %s", sexp)
	}
}

func TestCSSParseMediaQuery(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(cssLang())

	src := "@media (max-width: 600px) { body { font-size: 14px; } }"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexp := root.String()
	if !strings.Contains(sexp, "media_statement") && !strings.Contains(sexp, "@media") {
		t.Errorf("expected media statement in: %s", sexp)
	}
}

// --- HTML Integration Tests ---

func TestHTMLParseSimpleElement(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(htmlLang())

	src := "<div>hello</div>"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexp := root.String()
	if !strings.Contains(sexp, "element") {
		t.Errorf("expected element in: %s", sexp)
	}
}

func TestHTMLParseNestedElements(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(htmlLang())

	src := "<div><p>text</p></div>"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexp := root.String()
	if !strings.Contains(sexp, "element") {
		t.Errorf("expected element in: %s", sexp)
	}
}

func TestHTMLParseSelfClosingTag(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(htmlLang())

	src := "<br/>"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexp := root.String()
	if !strings.Contains(sexp, "element") && !strings.Contains(sexp, "self_closing") {
		t.Errorf("expected element or self_closing in: %s", sexp)
	}
}

func TestHTMLParseVoidElement(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"img", `<img>`},
		{"img_attr", `<img src="test.png">`},
		{"br", `<br>`},
		{"hr", `<hr>`},
		{"input", `<input type="text">`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pp := ts.NewParser()
			pp.SetLanguage(htmlLang())
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
			defer cancel()
			tree := pp.ParseString(ctx, []byte(tc.src))
			if tree == nil {
				t.Fatalf("nil tree for %q", tc.src)
			}
			root := tree.RootNode()
			sexp := root.String()
			t.Logf("sexp=%s", sexp)
			if root.Type() != "document" {
				t.Errorf("root type = %q, want document", root.Type())
			}
			if !strings.Contains(sexp, "element") {
				t.Errorf("expected element in: %s", sexp)
			}
		})
	}
}

// --- Java Integration Tests ---

func TestJavaParseHelloWorld(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(javaLang())

	// Full test
	src := `public class Hello {
    public static void main(String[] args) {
        System.out.println("Hello, world!");
    }
}`
	// Try simpler variants first
	for _, tc := range []struct{name, code string}{
		{"empty_class", `class Hello {}`},
		{"with_field", `class Hello { int x; }`},
		{"with_method", `class Hello { void main() {} }`},
		{"with_args", `class Hello { void main(String[] args) {} }`},
		{"method_call", `class Hello { void main() { foo(); } }`},
		{"field_access", `class Hello { void main() { System.out; } }`},
		{"chained_call", `class Hello { void main() { System.out.println(); } }`},
		{"string_arg", `class Hello { void main() { println("hi"); } }`},
		{"full_call", `class Hello { void main() { System.out.println("Hello"); } }`},
		{"full", src},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pp := ts.NewParser()
			pp.SetLanguage(javaLang())
			ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel2()
			tree2 := pp.ParseString(ctx2, []byte(tc.code))
			if tree2 == nil {
				t.Fatalf("timeout parsing: %s", tc.code)
			}
			t.Logf("OK: %s", tree2.RootNode().String())
		})
	}
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "program" {
		t.Errorf("root type = %q, want %q", root.Type(), "program")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "class_declaration") {
		t.Errorf("expected class_declaration in: %s", sexp)
	}
}

func TestJavaParseInterface(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(javaLang())

	src := `interface Readable {
    String read();
}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexp := root.String()
	if !strings.Contains(sexp, "interface_declaration") {
		t.Errorf("expected interface_declaration in: %s", sexp)
	}
}

// --- Helpers ---

func testTimeout() time.Duration {
	return 5 * time.Second
}
