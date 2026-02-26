package e2etest_test

import (
	"context"
	iparser "github.com/treesitter-go/treesitter/parser"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	jsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/javascript"
	rubygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/ruby"
	tsxgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/tsxgrammar"
	tsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/typescript"
	bashscanner "github.com/treesitter-go/treesitter/scanners/bash"
	jsscanner "github.com/treesitter-go/treesitter/scanners/javascript"
	rubyscanner "github.com/treesitter-go/treesitter/scanners/ruby"
	tsscanner "github.com/treesitter-go/treesitter/scanners/typescript"
)

func newBashLang() *ts.Language {
	lang := bashgrammar.BashLanguage()
	lang.NewExternalScanner = bashscanner.New
	return lang
}

func newRubyLang() *ts.Language {
	lang := rubygrammar.RubyLanguage()
	lang.NewExternalScanner = rubyscanner.New
	return lang
}

func jsLang() *ts.Language {
	lang := jsgrammar.JavascriptLanguage()
	lang.NewExternalScanner = jsscanner.New
	return lang
}

func newTSLang() *ts.Language {
	lang := tsgrammar.TypescriptLanguage()
	lang.NewExternalScanner = tsscanner.New
	return lang
}

func newTSXLang() *ts.Language {
	lang := tsxgrammar.TsxLanguage()
	lang.NewExternalScanner = tsscanner.New
	return lang
}

// --- Bash Integration Tests ---

func TestBashParseFunction(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newBashLang())

	src := `greet() {
  echo "Hello, $1"
}
greet World`
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
	if !strings.Contains(sexp, "function_definition") {
		t.Errorf("expected function_definition in: %s", sexp)
	}
}

func TestBashParsePipeline(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newBashLang())

	src := `cat /etc/passwd | grep root | wc -l`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "pipeline") {
		t.Errorf("expected pipeline in: %s", sexp)
	}
}

func TestBashParseArray(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newBashLang())

	src := `arr=(one two three)
echo "${arr[1]}"`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "array") {
		t.Errorf("expected array in: %s", sexp)
	}
}

func TestBashParseHeredoc(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newBashLang())

	src := `cat <<EOF
Hello World
EOF`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "heredoc") {
		t.Errorf("expected heredoc in: %s", sexp)
	}
}

// --- Ruby Integration Tests ---

func TestRubyParseClass(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newRubyLang())

	src := `class Dog
  def bark
    puts "Woof!"
  end
end`
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
	if !strings.Contains(sexp, "class") {
		t.Errorf("expected class in: %s", sexp)
	}
}

func TestRubyParseBlock(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newRubyLang())

	src := `[1, 2, 3].each do |x|
  puts x
end`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "do_block") || !strings.Contains(sexp, "block_parameters") {
		t.Errorf("expected do_block with block_parameters in: %s", sexp)
	}
}

func TestRubyParseHeredoc(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newRubyLang())

	src := `text = <<~HEREDOC
  Hello
  World
HEREDOC`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "heredoc") {
		t.Errorf("expected heredoc in: %s", sexp)
	}
}

func TestRubyParseStringInterpolation(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newRubyLang())

	src := `name = "World"
puts "Hello, #{name}!"`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "interpolation") {
		t.Errorf("expected interpolation in: %s", sexp)
	}
}

// --- TypeScript Integration Tests ---

func TestTypeScriptParseInterface(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newTSLang())

	src := `interface User {
  name: string;
  age: number;
}`
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
	if !strings.Contains(sexp, "interface_declaration") {
		t.Errorf("expected interface_declaration in: %s", sexp)
	}
}

func TestTypeScriptParseGenerics(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newTSLang())

	src := `function identity<T>(arg: T): T {
  return arg;
}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "type_parameter") {
		t.Errorf("expected type_parameter in: %s", sexp)
	}
}

func TestTypeScriptParseTypeAnnotations(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newTSLang())

	src := `const greeting: string = "hello";
let count: number = 42;
let active: boolean = true;`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "type_annotation") {
		t.Errorf("expected type_annotation in: %s", sexp)
	}
}

func TestTypeScriptParseEnum(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(newTSLang())

	src := `enum Color {
  Red,
  Green,
  Blue,
}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "enum_declaration") {
		t.Errorf("expected enum_declaration in: %s", sexp)
	}
}

// --- JavaScript Integration Tests ---

func TestJSParseVariableDeclaration(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := `const x = 42;`
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
	if !strings.Contains(sexp, "lexical_declaration") {
		t.Errorf("expected lexical_declaration in: %s", sexp)
	}
}

func TestJSParseFunction(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := `function add(a, b) { return a + b; }`
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
}

func TestJSParseArrowFunction(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := `const greet = (name) => "Hello, " + name;`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "arrow_function") {
		t.Errorf("expected arrow_function in: %s", sexp)
	}
}

func TestJSParseClass(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := `class Shape {
  constructor(name) {
    this.name = name;
  }
  area() {
    return 0;
  }
}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "class_declaration") {
		t.Errorf("expected class_declaration in: %s", sexp)
	}
}

func TestJSParseImportExport(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := `import { foo } from 'bar';
export default function main() {}`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "import_statement") {
		t.Errorf("expected import_statement in: %s", sexp)
	}
	if !strings.Contains(sexp, "export_statement") {
		t.Errorf("expected export_statement in: %s", sexp)
	}
}

func TestJSParseTemplateLiteral(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := "const msg = `Hello, ${name}!`;"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "template_string") {
		t.Errorf("expected template_string in: %s", sexp)
	}
}

func TestJSParseAsyncAwait(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := `async function fetchData() {
  const result = await fetch('/api');
  return result;
}`
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
	if !strings.Contains(sexp, "await_expression") {
		t.Errorf("expected await_expression in: %s", sexp)
	}
}

func TestJSParseDestructuring(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(jsLang())

	src := `const { a, b } = obj;
const [x, y] = arr;`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "object_pattern") {
		t.Errorf("expected object_pattern in: %s", sexp)
	}
	if !strings.Contains(sexp, "array_pattern") {
		t.Errorf("expected array_pattern in: %s", sexp)
	}
}
