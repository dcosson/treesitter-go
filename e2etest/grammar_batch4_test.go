package e2etest_test

import (
	"context"
	iparser "github.com/treesitter-go/treesitter/parser"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	luagrammar "github.com/treesitter-go/treesitter/internal/testgrammars/lua"
	perlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/perl"
	luascanner "github.com/treesitter-go/treesitter/scanners/lua"
	perlscanner "github.com/treesitter-go/treesitter/scanners/perl"
)

func luaLang() *ts.Language {
	lang := luagrammar.LuaLanguage()
	lang.NewExternalScanner = luascanner.New
	return lang
}

func perlLang() *ts.Language {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New
	return lang
}

// --- Lua Integration Tests ---

func TestLuaParseFunction(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(luaLang())

	src := `function factorial(n)
  if n <= 1 then
    return 1
  end
  return n * factorial(n - 1)
end
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "chunk" {
		t.Errorf("root type = %q, want %q", root.Type(), "chunk")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "function_declaration") {
		t.Errorf("expected function_declaration in: %s", sexp)
	}
	if !strings.Contains(sexp, "if_statement") {
		t.Errorf("expected if_statement in: %s", sexp)
	}
	if !strings.Contains(sexp, "return_statement") {
		t.Errorf("expected return_statement in: %s", sexp)
	}
}

func TestLuaParseLocalVariables(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(luaLang())

	src := `local x = 10
local y = 20
local sum = x + y
print(sum)
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "chunk" {
		t.Errorf("root type = %q, want %q", root.Type(), "chunk")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "variable_declaration") {
		t.Errorf("expected variable_declaration in: %s", sexp)
	}
	if !strings.Contains(sexp, "function_call") {
		t.Errorf("expected function_call in: %s", sexp)
	}
}

func TestLuaParseForLoop(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(luaLang())

	src := `local t = {1, 2, 3, 4, 5}
local sum = 0
for i = 1, #t do
  sum = sum + t[i]
end

for k, v in pairs(t) do
  print(k, v)
end
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "chunk" {
		t.Errorf("root type = %q, want %q", root.Type(), "chunk")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "for_numeric_statement") {
		// Some grammars may use for_statement — check both
		if !strings.Contains(sexp, "for_statement") {
			t.Errorf("expected for_numeric_statement or for_statement in: %s", sexp)
		}
	}
	if !strings.Contains(sexp, "for_generic_statement") {
		// Fall back to checking for_in_statement
		if !strings.Contains(sexp, "for_in_statement") && !strings.Contains(sexp, "for_generic") {
			t.Errorf("expected for_generic_statement or for_in_statement in: %s", sexp)
		}
	}
}

func TestLuaParseTable(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(luaLang())

	src := `local config = {1, 2, 3}
local nested = {x = 1, y = 2}
print(config)
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "chunk" {
		t.Errorf("root type = %q, want %q", root.Type(), "chunk")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "table_constructor") {
		t.Errorf("expected table_constructor in: %s", sexp)
	}
	if !strings.Contains(sexp, "field") {
		t.Errorf("expected field in: %s", sexp)
	}
}

func TestLuaParseBlockComment(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(luaLang())

	// Block comment followed by real code — verifies the scanner correctly
	// tokenizes the block comment so the parser can skip it and parse the rest.
	src := `--[[
This is a block comment
spanning multiple lines
]]
local x = 42
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "chunk" {
		t.Errorf("root type = %q, want %q", root.Type(), "chunk")
	}
	sexp := root.String()
	// Comments are extras and may not appear in s-expression.
	// The key test is that the code after the comment parses correctly.
	if !strings.Contains(sexp, "variable_declaration") {
		t.Errorf("expected variable_declaration after block comment in: %s", sexp)
	}
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR node in: %s", sexp)
	}
}

func TestLuaParseBlockString(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(luaLang())

	src := `local s = [=[
This is a block string
with level 1 delimiters
containing ]] which is not the end
]=]
print(s)
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "chunk" {
		t.Errorf("root type = %q, want %q", root.Type(), "chunk")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "string") {
		t.Errorf("expected string in: %s", sexp)
	}
	if !strings.Contains(sexp, "function_call") {
		t.Errorf("expected function_call in: %s", sexp)
	}
}

func TestLuaParseBlockCommentWithLevel(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(luaLang())

	// Level-2 block comment containing ]] and ]=] which are NOT the end.
	// This tests that the scanner correctly matches the closing delimiter level.
	src := `--[==[
Block comment with level 2
containing ]] and ]=] which are not the end
]==]
local y = 100
`
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	root := tree.RootNode()
	if root.Type() != "chunk" {
		t.Errorf("root type = %q, want %q", root.Type(), "chunk")
	}
	sexp := root.String()
	if !strings.Contains(sexp, "variable_declaration") {
		t.Errorf("expected variable_declaration after level-2 block comment in: %s", sexp)
	}
	if strings.Contains(sexp, "ERROR") {
		t.Errorf("unexpected ERROR node in: %s", sexp)
	}
}

// --- Perl Integration Tests ---

func TestPerlParseSubroutine(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(perlLang())

	// Uses numeric expressions only — string literals have lex routing bugs (tracked)
	src := "sub add { my ($a, $b) = @_; return $a + $b; }\n"
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
	if !strings.Contains(sexp, "subroutine_declaration_statement") {
		t.Errorf("expected subroutine_declaration_statement in: %s", sexp)
	}
	if !strings.Contains(sexp, "return_expression") {
		t.Errorf("expected return_expression in: %s", sexp)
	}
}

func TestPerlParsePackage(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(perlLang())

	src := "package Math;\n\nsub square { my ($n) = @_; return $n * $n; }\n\n1;\n"
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout())
	defer cancel()
	tree := p.ParseString(ctx, []byte(src))
	if tree == nil {
		t.Fatal("expected tree, got nil")
	}
	sexp := tree.RootNode().String()
	if !strings.Contains(sexp, "package_statement") {
		t.Errorf("expected package_statement in: %s", sexp)
	}
	if !strings.Contains(sexp, "subroutine_declaration_statement") {
		t.Errorf("expected subroutine_declaration_statement in: %s", sexp)
	}
}

func TestPerlParseControlFlow(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(perlLang())

	src := `use strict;

my $x = 10;
if ($x > 5) {
    $x = $x + 1;
} elsif ($x > 0) {
    $x = 0;
} else {
    $x = -1;
}

for my $i (1..10) {
    next if $i == 3;
    last if $i == 7;
}

while ($x > 0) {
    $x = $x - 1;
}
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
	if !strings.Contains(sexp, "conditional_statement") {
		t.Errorf("expected conditional_statement in: %s", sexp)
	}
	if !strings.Contains(sexp, "for_statement") {
		t.Errorf("expected for_statement in: %s", sexp)
	}
	if !strings.Contains(sexp, "loop_statement") {
		t.Errorf("expected loop_statement (while) in: %s", sexp)
	}
}

func TestPerlParseDataStructures(t *testing.T) {
	p := iparser.NewParser()
	p.SetLanguage(perlLang())

	src := `my @nums = (1, 2, 3, 4, 5);
my %config = (max => 100, min => 0, step => 5);
my $h = {x => 1, y => 2};
my @sorted = sort { $a <=> $b } @nums;
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
	if !strings.Contains(sexp, "array") {
		t.Errorf("expected array in: %s", sexp)
	}
	if !strings.Contains(sexp, "hash") {
		t.Errorf("expected hash in: %s", sexp)
	}
	if !strings.Contains(sexp, "sort_expression") {
		t.Errorf("expected sort_expression in: %s", sexp)
	}
}
