package treesitter_test

import (
	"context"
	"strings"
	"testing"

	ts "github.com/treesitter-go/treesitter"
	perlgrammar "github.com/treesitter-go/treesitter/internal/testgrammars/perl"
	perlscanner "github.com/treesitter-go/treesitter/scanners/perl"
)

func perlLang() *ts.Language {
	lang := perlgrammar.PerlLanguage()
	lang.NewExternalScanner = perlscanner.New
	return lang
}

// --- Perl Integration Tests ---

func TestPerlParseSubroutine(t *testing.T) {
	p := ts.NewParser()
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
	p := ts.NewParser()
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
	p := ts.NewParser()
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
	p := ts.NewParser()
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
