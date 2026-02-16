package treesitter_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	golanggrammar "github.com/treesitter-go/treesitter/internal/testgrammars/golang"
	pygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/python"
	pyscanner "github.com/treesitter-go/treesitter/scanners/python"
)

func goLang() *ts.Language {
	return golanggrammar.GoLanguage()
}

func pyLang() *ts.Language {
	lang := pygrammar.PythonLanguage()
	lang.NewExternalScanner = pyscanner.New
	return lang
}

// --- Go Integration Tests ---

func TestGoParsePackageAndImport(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(goLang())

	src := `package main

import (
	"fmt"
	"os"
)
`
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
	if !strings.Contains(sexp, "package_clause") {
		t.Errorf("expected package_clause in: %s", sexp)
	}
	if !strings.Contains(sexp, "import_declaration") {
		t.Errorf("expected import_declaration in: %s", sexp)
	}
	if !strings.Contains(sexp, "import_spec_list") {
		t.Errorf("expected import_spec_list in: %s", sexp)
	}
}

func TestGoParseInterface(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(goLang())

	src := "package io\n\ntype Reader interface {\n\tRead(p []byte) (n int, err error)\n}\n"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "type_declaration") {
		t.Errorf("expected type_declaration in: %s", sexp)
	}
	if !strings.Contains(sexp, "interface_type") {
		t.Errorf("expected interface_type in: %s", sexp)
	}
}

func TestGoParseFunctionWithExpressions(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(goLang())

	src := `package main

func compute() int {
	x := 1
	y := 2
	return x + y
}
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "function_declaration") {
		t.Errorf("expected function_declaration in: %s", sexp)
	}
	if !strings.Contains(sexp, "short_var_declaration") {
		t.Errorf("expected short_var_declaration in: %s", sexp)
	}
	if !strings.Contains(sexp, "return_statement") {
		t.Errorf("expected return_statement in: %s", sexp)
	}
	if !strings.Contains(sexp, "binary_expression") {
		t.Errorf("expected binary_expression in: %s", sexp)
	}
}

func TestGoParseBlankIdentifier(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(goLang())

	t.Skip("blank_identifier requires reserved word support — coder-1 feat/grammar-gen-remaining")
	src := "package main\n\nfunc f() {\n\t_ = 1\n}\n"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	t.Logf("sexp: %s", sexp)
	if !strings.Contains(sexp, "blank_identifier") {
		t.Errorf("expected blank_identifier in: %s", sexp)
	}
}

func TestGoParseForLoop(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(goLang())

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"c_style_for", "package main\n\nfunc main() {\n\tfor i := 0; i < 10; i++ {\n\t\tfmt.Println(i)\n\t}\n}\n", "for_statement"},
		{"range_for", "package main\n\nfunc f() {\n\tfor i, v := range items {\n\t\tx = v\n\t}\n}\n", "for_statement"},
		{"infinite_for", "package main\n\nfunc f() {\n\tfor {\n\t\tbreak\n\t}\n}\n", "for_statement"},
		{"while_for", "package main\n\nfunc f() {\n\tfor x > 0 {\n\t\tx--\n\t}\n}\n", "for_statement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
			defer cancel()
			tree := p.ParseString(ctx, []byte(tc.src))
			if tree == nil {
				t.Fatal("expected tree, got nil")
			}
			sexp := tree.RootNode().String()
			t.Logf("sexp: %s", sexp)
			if !strings.Contains(sexp, tc.want) {
				t.Errorf("expected %s in: %s", tc.want, sexp)
			}
		})
	}
}

func TestGoParseMapLiteral(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(goLang())

	cases := []struct {
		name string
		src  string
		want string
	}{
		{"map_string_int", "package main\n\nvar m = map[string]int{\n\t\"one\": 1,\n\t\"two\": 2,\n}\n", "map_type"},
		{"map_inline", `package main

func f() {
	x := map[string]int{"a": 1}
}
`, "map_type"},
		{"map_int_string", "package main\n\nvar m = map[int]string{\n\t1: \"one\",\n}\n", "map_type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
			defer cancel()
			tree := p.ParseString(ctx, []byte(tc.src))
			if tree == nil {
				t.Fatal("expected tree, got nil")
			}
			sexp := tree.RootNode().String()
			t.Logf("sexp: %s", sexp)
			if !strings.Contains(sexp, tc.want) {
				t.Errorf("expected %s in: %s", tc.want, sexp)
			}
		})
	}
}

// --- Python Integration Tests ---

func TestPythonParseFunction(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(pyLang())

	src := `def greet(name):
    return f"Hello, {name}!"
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "module" {
		t.Errorf("root type = %q, want %q", root.Type(), "module")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "function_definition") {
		t.Errorf("expected function_definition in: %s", sexp)
	}
	if !strings.Contains(sexp, "return_statement") {
		t.Errorf("expected return_statement in: %s", sexp)
	}
}

func TestPythonParseClass(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(pyLang())

	src := `class Animal:
    def __init__(self, name):
        self.name = name

    def speak(self):
        return self.name
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "class_definition") {
		t.Errorf("expected class_definition in: %s", sexp)
	}
	if !strings.Contains(sexp, "function_definition") {
		t.Errorf("expected function_definition in: %s", sexp)
	}
}

func TestPythonParseListComprehension(t *testing.T) {
	p := ts.NewParser()
	p.SetLanguage(pyLang())

	src := `squares = [x * x for x in range(10) if x % 2 == 0]
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "list_comprehension") {
		t.Errorf("expected list_comprehension in: %s", sexp)
	}
}
