package treesitter_test

import (
	"context"
	iparser "github.com/treesitter-go/treesitter/parser"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	cgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cgrammar"
	cppgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/cppgrammar"
	rustgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/rustgrammar"
	cppscanner "github.com/treesitter-go/treesitter/scanners/cpp"
	rustscanner "github.com/treesitter-go/treesitter/scanners/rust"
)

func cLang() *ts.Language {
	return cgrammar.CLanguage()
}

func cppLang() *ts.Language {
	lang := cppgrammar.CppLanguage()
	lang.NewExternalScanner = cppscanner.New
	return lang
}

func rustLang() *ts.Language {
	lang := rustgrammar.RustLanguage()
	lang.NewExternalScanner = rustscanner.New
	return lang
}

// --- C Integration Tests ---

func TestCParseStruct(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(cLang())

	src := `struct Point {
    int x;
    int y;
};`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "translation_unit" {
		t.Errorf("root type = %q, want %q", root.Type(), "translation_unit")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "struct_specifier") {
		t.Errorf("expected struct_specifier in: %s", sexp)
	}
}

func TestCParsePointers(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(cLang())

	src := `int *ptr = &x;`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "pointer_declarator") {
		t.Errorf("expected pointer_declarator in: %s", sexp)
	}
}

func TestCParseFunction(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(cLang())

	src := `int add(int a, int b) { return a + b; }`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "function_definition") {
		t.Errorf("expected function_definition in: %s", sexp)
	}
}

func TestCParseTypedef(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(cLang())

	src := `typedef unsigned long size_t;`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "type_definition") {
		t.Errorf("expected type_definition in: %s", sexp)
	}
}

// --- C++ Integration Tests ---

func TestCppParseClass(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(cppLang())

	src := `class Shape {
public:
    virtual double area() const = 0;
};`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "class_specifier") {
		t.Errorf("expected class_specifier in: %s", sexp)
	}
}

func TestCppParseTemplate(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(cppLang())

	src := `template <typename T>
T max(T a, T b) { return a > b ? a : b; }`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "template_declaration") {
		t.Errorf("expected template_declaration in: %s", sexp)
	}
}

func TestCppParseNamespace(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(cppLang())

	src := `namespace math { int add(int a, int b) { return a + b; } }`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "namespace_definition") {
		t.Errorf("expected namespace_definition in: %s", sexp)
	}
}

// --- Rust Integration Tests ---

func TestRustParseStruct(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(rustLang())

	src := `struct Point {
    x: f64,
    y: f64,
}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "source_file" {
		t.Errorf("root type = %q, want %q", root.Type(), "source_file")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "struct_item") {
		t.Errorf("expected struct_item in: %s", sexp)
	}
}

func TestRustParseImpl(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(rustLang())

	src := `impl Point {
    fn new(x: f64, y: f64) -> Self {
        Point { x, y }
    }
}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "impl_item") {
		t.Errorf("expected impl_item in: %s", sexp)
	}
}

func TestRustParseMatch(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(rustLang())

	src := `fn check(x: i32) -> i32 {
    match x {
        0 => 1,
        _ => 2,
    }
}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "match_expression") {
		t.Errorf("expected match_expression in: %s", sexp)
	}
}

func TestRustParseLifetimes(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(rustLang())

	src := `fn longest<'a>(x: &'a str, y: &'a str) -> &'a str { x }`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "lifetime") {
		t.Errorf("expected lifetime in: %s", sexp)
	}
}
