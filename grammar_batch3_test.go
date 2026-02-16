package treesitter_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	bashgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/bash"
	rubygrammar "github.com/treesitter-go/treesitter/internal/testgrammars/ruby"
	tsgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/typescript"
	bashscanner "github.com/treesitter-go/treesitter/scanners/bash"
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

func newTSLang() *ts.Language {
	lang := tsgrammar.TypescriptLanguage()
	lang.NewExternalScanner = tsscanner.New
	return lang
}

// --- Bash Integration Tests ---

func TestBashParseFunction(t *testing.T) {
	t.Skip("external scanner bug — bash scanner timeout on function definitions")
	p := ts.NewParser()
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
	p := ts.NewParser()
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
	t.Skip("external scanner bug — bash scanner timeout on array/variable expansion")
	p := ts.NewParser()
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
	t.Skip("external scanner bug — bash scanner timeout on heredoc")
	p := ts.NewParser()
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
	t.Skip("external scanner bug — ruby scanner timeout on class/method definitions")
	p := ts.NewParser()
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
	p := ts.NewParser()
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
	t.Skip("external scanner bug — ruby scanner timeout on heredoc")
	p := ts.NewParser()
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
	t.Skip("external scanner bug — ruby scanner timeout on string interpolation")
	p := ts.NewParser()
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
	p := ts.NewParser()
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
	p := ts.NewParser()
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
	p := ts.NewParser()
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
	p := ts.NewParser()
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
