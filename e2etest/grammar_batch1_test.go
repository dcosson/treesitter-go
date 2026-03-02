package e2etest_test

import (
	"context"
	iparser "github.com/dcosson/treesitter-go/parser"
	"strings"
	"testing"

	ts "github.com/dcosson/treesitter-go"
	golanggrammar "github.com/dcosson/treesitter-go/internal/grammars/golang"
	pygrammar "github.com/dcosson/treesitter-go/internal/grammars/python"
	pyscanner "github.com/dcosson/treesitter-go/internal/scanners/python"
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
	p := iparser.NewParser()
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
	p := iparser.NewParser()
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
	p := iparser.NewParser()
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
	p := iparser.NewParser()
	p.SetLanguage(goLang())

	src := `package main

var _ = 0
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	sexp := root.String()
	if root.Type() != "source_file" {
		t.Errorf("root type = %q, want %q", root.Type(), "source_file")
	}
	if !strings.Contains(sexp, "var_declaration") {
		t.Errorf("expected var_declaration in: %s", sexp)
	}
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in: %s", sexp)
	}
}

func TestGoParseForLoop(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(goLang())

	src := `package main

func main() {
	for i := 0; i < 10; i++ {
		_ = i
	}
}
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "for_statement") {
		t.Errorf("expected for_statement in: %s", sexp)
	}
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in: %s", sexp)
	}
}

func TestGoParseIfCondition(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(goLang())

	src := `package main

func main() {
	x := 5
	if x > 0 {
		_ = x
	}
}
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "if_statement") {
		t.Errorf("expected if_statement in: %s", sexp)
	}
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in: %s", sexp)
	}
}

func TestGoParseMapLiteral(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(goLang())

	src := `package main

var m = map[int]int{}
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "map_type") {
		t.Errorf("expected map_type in: %s", sexp)
	}
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in: %s", sexp)
	}
}

func TestGoParsePrintln(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(goLang())

	src := `package main

func main() {
	println(42)
}
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "call_expression") {
		t.Errorf("expected call_expression in: %s", sexp)
	}
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR in: %s", sexp)
	}
}

// --- Python Integration Tests ---

func TestPythonParseFunction(t *testing.T) {
	p := iparser.NewParser()
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
	p := iparser.NewParser()
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
	p := iparser.NewParser()
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
