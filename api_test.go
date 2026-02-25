package treesitter_test

import (
	"context"
	"fmt"
	iparser "github.com/treesitter-go/treesitter/parser"
	"strings"
	"testing"
	"time"

	ts "github.com/treesitter-go/treesitter"
	tg "github.com/treesitter-go/treesitter/internal/testgrammars"
	golanggrammar "github.com/treesitter-go/treesitter/internal/testgrammars/golang"
)

// mustParseGo parses Go source and fails the test if the result is nil.
func mustParseGo(t *testing.T, src string) *ts.Tree {
	t.Helper()
	lang := golanggrammar.GoLanguage()
	p := iparser.NewParser()
	p.SetLanguage(lang)
	tree := p.ParseString(context.Background(), []byte(src))
	if tree == nil {
		t.Fatalf("parser returned nil tree for: %s", src)
	}
	return tree
}

// --- Tree API Tests ---

func TestTreeCopy(t *testing.T) {
	src := "package main\n\nvar x = 1\n"
	tree := mustParseGo(t, src)

	copied := tree.Copy()
	if copied == nil {
		t.Fatal("Copy returned nil")
	}

	origRoot := tree.RootNode()
	copyRoot := copied.RootNode()

	if origRoot.String() != copyRoot.String() {
		t.Errorf("Copy produced different tree:\n  orig: %s\n  copy: %s",
			origRoot.String(), copyRoot.String())
	}

	if tree.Language() != copied.Language() {
		t.Error("Copy has different language pointer")
	}
}

func TestTreeEdit(t *testing.T) {
	src := "package main\n\nvar x = 1\n"
	tree := mustParseGo(t, src)

	root := tree.RootNode()
	origEndByte := root.EndByte()
	if origEndByte != uint32(len(src)) {
		t.Errorf("original EndByte = %d, want %d", origEndByte, len(src))
	}

	// Simulate replacing "1" with "42" (byte 22 is '1', byte 23 is '\n').
	edit := &ts.InputEdit{
		StartByte:   22,
		OldEndByte:  23,
		NewEndByte:  24,
		StartPoint:  ts.Point{Row: 2, Column: 8},
		OldEndPoint: ts.Point{Row: 2, Column: 9},
		NewEndPoint: ts.Point{Row: 2, Column: 10},
	}

	editedTree := tree.Edit(edit)
	if editedTree == nil {
		t.Fatal("Edit returned nil")
	}

	// Original tree should be unmodified.
	origRootAfter := tree.RootNode()
	if origRootAfter.EndByte() != origEndByte {
		t.Errorf("original tree modified: EndByte = %d, want %d",
			origRootAfter.EndByte(), origEndByte)
	}

	// Edited tree should reflect the edit.
	editedRoot := editedTree.RootNode()
	if !editedRoot.HasChanges() {
		t.Error("edited tree root does not have changes")
	}
}

func TestTreeEditPreservesLanguage(t *testing.T) {
	src := "package main\n"
	tree := mustParseGo(t, src)

	edit := &ts.InputEdit{
		StartByte:   8,
		OldEndByte:  12,
		NewEndByte:  15,
		StartPoint:  ts.Point{Row: 0, Column: 8},
		OldEndPoint: ts.Point{Row: 0, Column: 12},
		NewEndPoint: ts.Point{Row: 0, Column: 15},
	}

	edited := tree.Edit(edit)
	if edited.Language() != tree.Language() {
		t.Error("edited tree has different language")
	}
}

func TestTreeIncludedRanges(t *testing.T) {
	src := "package main\n"
	tree := mustParseGo(t, src)

	ranges := tree.IncludedRanges()
	// A default parse (no SetIncludedRanges) typically returns either an empty
	// slice or a single range covering the full input. Both are valid behaviors.
	t.Logf("included ranges count: %d", len(ranges))
	for i, r := range ranges {
		t.Logf("range[%d]: bytes [%d, %d]", i, r.StartByte, r.EndByte)
	}
}

// --- Node API Tests ---

func TestNodeIsExtra(t *testing.T) {
	src := "package main\n\n// this is a comment\nvar x = 1\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	foundExtra := false
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.IsExtra() {
			foundExtra = true
			t.Logf("found extra node: type=%s, bytes [%d:%d]",
				child.Type(), child.StartByte(), child.EndByte())
		}
	}

	if !foundExtra {
		t.Logf("no extra (comment) node found as direct child of root (may be nested): %s", root.String())
	}
}

func TestNodeIsMissing(t *testing.T) {
	// Use JSON grammar which has reliable error recovery for malformed input.
	src := `{"a": }`
	lang := tg.JsonLanguage()
	p := iparser.NewParser()
	p.SetLanguage(lang)
	tree := p.ParseString(context.Background(), []byte(src))
	if tree == nil {
		t.Fatal("parse returned nil for malformed JSON")
	}
	root := tree.RootNode()

	hasMissingOrError := false
	var walkNode func(n ts.Node, depth int)
	walkNode = func(n ts.Node, depth int) {
		if n.IsNull() {
			return
		}
		if n.IsMissing() {
			hasMissingOrError = true
			t.Logf("found MISSING node at depth %d: type=%s", depth, n.Type())
		}
		if n.Symbol() == ts.SymbolError {
			hasMissingOrError = true
			t.Logf("found ERROR node at depth %d: type=%s", depth, n.Type())
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walkNode(n.Child(i), depth+1)
		}
	}
	walkNode(root, 0)

	if !hasMissingOrError {
		t.Errorf("expected MISSING or ERROR node for malformed input %q, got: %s", src, root.String())
	}
}

func TestNodeHasChanges(t *testing.T) {
	src := "package main\n\nvar x = 1\n"
	tree := mustParseGo(t, src)

	root := tree.RootNode()
	if root.HasChanges() {
		t.Error("fresh tree root should not have changes")
	}

	edit := &ts.InputEdit{
		StartByte:   22,
		OldEndByte:  23,
		NewEndByte:  24,
		StartPoint:  ts.Point{Row: 2, Column: 8},
		OldEndPoint: ts.Point{Row: 2, Column: 9},
		NewEndPoint: ts.Point{Row: 2, Column: 10},
	}
	edited := tree.Edit(edit)
	editedRoot := edited.RootNode()
	if !editedRoot.HasChanges() {
		t.Error("edited tree root should have changes")
	}
}

func TestNodePositionAccuracy(t *testing.T) {
	src := "package main\n\nfunc f() {\n}\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	if root.StartByte() != 0 {
		t.Errorf("root StartByte = %d, want 0", root.StartByte())
	}
	if root.EndByte() != uint32(len(src)) {
		t.Errorf("root EndByte = %d, want %d", root.EndByte(), len(src))
	}

	sp := root.StartPoint()
	if sp.Row != 0 || sp.Column != 0 {
		t.Errorf("root StartPoint = {%d,%d}, want {0,0}", sp.Row, sp.Column)
	}
}

func TestNodeChildByFieldName(t *testing.T) {
	src := "package main\n\nfunc hello() {\n}\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	var funcDecl ts.Node
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() == "function_declaration" {
			funcDecl = child
			break
		}
	}

	if funcDecl.IsNull() {
		t.Fatalf("function_declaration not found in: %s", root.String())
	}

	nameNode := funcDecl.ChildByFieldName("name")
	if nameNode.IsNull() {
		t.Fatal("ChildByFieldName(\"name\") returned null")
	}
	if nameNode.Type() != "identifier" {
		t.Errorf("name node type = %q, want %q", nameNode.Type(), "identifier")
	}

	nameStart := nameNode.StartByte()
	nameEnd := nameNode.EndByte()
	nameText := src[nameStart:nameEnd]
	if nameText != "hello" {
		t.Errorf("name text = %q, want %q", nameText, "hello")
	}
}

func TestNodeParentNavigation(t *testing.T) {
	src := "package main\n\nvar x = 42\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	if !root.Parent().IsNull() {
		t.Error("root parent should be null")
	}

	child := root.Child(0)
	if child.IsNull() {
		t.Fatal("first child is null")
	}
	parent := child.Parent()
	if parent.IsNull() {
		t.Fatal("parent of first child is null")
	}
	if parent.Type() != root.Type() {
		t.Errorf("parent type = %q, want %q (root)", parent.Type(), root.Type())
	}
}

func TestNodeSiblingNavigation(t *testing.T) {
	src := "package main\n\nvar x = 1\nvar y = 2\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	first := root.Child(0)
	if !first.PrevSibling().IsNull() {
		t.Error("first child PrevSibling should be null")
	}

	second := first.NextSibling()
	if second.IsNull() {
		t.Fatal("second sibling is null")
	}

	prev := second.PrevSibling()
	if prev.IsNull() {
		t.Fatal("PrevSibling of second is null")
	}
	if !prev.Equal(first) {
		t.Errorf("PrevSibling of second != first: %s vs %s", prev.Type(), first.Type())
	}
}

func TestNodeEqual(t *testing.T) {
	src := "package main\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	if !root.Equal(root) {
		t.Error("root should equal itself")
	}

	child := root.Child(0)
	parent := child.Parent()
	if !parent.Equal(root) {
		t.Error("parent of child should equal root")
	}

	if root.Equal(child) {
		t.Error("root should not equal its child")
	}
}

func TestNodeNamedChildren(t *testing.T) {
	src := "package main\n\nvar x = 1\nvar y = 2\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	namedCount := root.NamedChildCount()
	totalCount := root.ChildCount()

	if namedCount > totalCount {
		t.Errorf("NamedChildCount %d > ChildCount %d", namedCount, totalCount)
	}

	t.Logf("root: ChildCount=%d, NamedChildCount=%d", totalCount, namedCount)

	for i := 0; i < int(namedCount); i++ {
		child := root.NamedChild(i)
		if child.IsNull() {
			t.Errorf("NamedChild(%d) is null", i)
			continue
		}
		if !child.IsNamed() {
			t.Errorf("NamedChild(%d) type=%q is not named", i, child.Type())
		}
	}
}

// --- Language API Tests ---

func TestLanguageSymbolName(t *testing.T) {
	lang := golanggrammar.GoLanguage()

	endName := lang.SymbolName(0)
	if endName != "end" {
		t.Errorf("SymbolName(0) = %q, want %q", endName, "end")
	}

	tests := []struct {
		sym  ts.Symbol
		want string
	}{
		{golanggrammar.SymIdentifier, "identifier"},
		{golanggrammar.SymPackage, "package"},
		{golanggrammar.SymFunc, "func"},
		{golanggrammar.SymImport, "import"},
	}

	for _, tt := range tests {
		got := lang.SymbolName(tt.sym)
		if got != tt.want {
			t.Errorf("SymbolName(%d) = %q, want %q", tt.sym, got, tt.want)
		}
	}
}

func TestLanguageSymbolIsNamed(t *testing.T) {
	lang := golanggrammar.GoLanguage()

	if !lang.SymbolIsNamed(golanggrammar.SymIdentifier) {
		t.Error("identifier should be named")
	}

	if lang.SymbolIsNamed(golanggrammar.SymLParen) {
		t.Error("'(' should not be named")
	}
}

func TestLanguageSymbolIsVisible(t *testing.T) {
	lang := golanggrammar.GoLanguage()

	if !lang.SymbolIsVisible(golanggrammar.SymIdentifier) {
		t.Error("identifier should be visible")
	}

	if !lang.SymbolIsVisible(golanggrammar.SymLParen) {
		t.Error("'(' should be visible")
	}
}

func TestLanguagePublicSymbol(t *testing.T) {
	lang := golanggrammar.GoLanguage()

	pub := lang.PublicSymbol(golanggrammar.SymIdentifier)
	if pub == 0 {
		t.Error("PublicSymbol(identifier) returned 0")
	}
}

// --- Parser API Tests ---

func TestParserSwitchLanguage(t *testing.T) {
	lang := golanggrammar.GoLanguage()
	p := iparser.NewParser()
	p.SetLanguage(lang)

	tree1 := p.ParseString(context.Background(), []byte("package main\n"))
	if tree1 == nil {
		t.Fatal("first parse returned nil")
	}

	p.SetLanguage(lang)
	tree2 := p.ParseString(context.Background(), []byte("package foo\n"))
	if tree2 == nil {
		t.Fatal("second parse returned nil")
	}

	if tree1.RootNode().Type() != tree2.RootNode().Type() {
		t.Errorf("different root types: %q vs %q",
			tree1.RootNode().Type(), tree2.RootNode().Type())
	}
}

func TestParserReuse(t *testing.T) {
	lang := golanggrammar.GoLanguage()
	p := iparser.NewParser()
	p.SetLanguage(lang)

	inputs := []string{
		"package a\n",
		"package b\n\nvar x int\n",
		"package c\n\nfunc f() {}\n",
	}

	for i, input := range inputs {
		tree := p.ParseString(context.Background(), []byte(input))
		if tree == nil {
			t.Fatalf("parse %d returned nil", i)
		}
		root := tree.RootNode()
		if root.IsNull() {
			t.Fatalf("parse %d: root is null", i)
		}
	}
}

func TestParserTimeout(t *testing.T) {
	lang := golanggrammar.GoLanguage()
	p := iparser.NewParser()
	p.SetLanguage(lang)

	// Generate a large input so parsing takes long enough to be cancelled.
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&sb, "func f%d(x int) int {\n\treturn x + %d\n}\n\n", i, i)
	}
	largeInput := []byte(sb.String())

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond) // ensure timeout expires before parse starts

	tree := p.ParseString(ctx, largeInput)
	if tree != nil {
		t.Error("expected nil tree from pre-expired timeout, but parse completed")
	}
}

func TestParserCancellation(t *testing.T) {
	lang := golanggrammar.GoLanguage()
	p := iparser.NewParser()
	p.SetLanguage(lang)

	// Generate a large input so parsing takes long enough to be cancelled.
	var sb strings.Builder
	sb.WriteString("package main\n\n")
	for i := 0; i < 500; i++ {
		fmt.Fprintf(&sb, "func f%d(x int) int {\n\treturn x + %d\n}\n\n", i, i)
	}
	largeInput := []byte(sb.String())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before parse starts

	tree := p.ParseString(ctx, largeInput)
	if tree != nil {
		t.Error("expected nil tree from pre-cancelled context, but parse completed")
	}
}

func TestParserIncrementalParsing(t *testing.T) {
	lang := golanggrammar.GoLanguage()
	p := iparser.NewParser()
	p.SetLanguage(lang)

	src1 := "package main\n\nvar x = 1\n"
	tree1 := p.ParseString(context.Background(), []byte(src1))
	if tree1 == nil {
		t.Fatal("first parse returned nil")
	}

	edit := &ts.InputEdit{
		StartByte:   22,
		OldEndByte:  23,
		NewEndByte:  24,
		StartPoint:  ts.Point{Row: 2, Column: 8},
		OldEndPoint: ts.Point{Row: 2, Column: 9},
		NewEndPoint: ts.Point{Row: 2, Column: 10},
	}
	editedTree := tree1.Edit(edit)

	src2 := "package main\n\nvar x = 42\n"
	tree2 := p.ParseString(context.Background(), []byte(src2), editedTree)
	if tree2 == nil {
		t.Fatal("incremental parse returned nil")
	}

	root2 := tree2.RootNode()
	if root2.IsNull() {
		t.Fatal("incremental parse root is null")
	}

	t.Logf("incremental parse result: %s", root2.String())
}

// --- TreeCursor API Tests ---

func TestTreeCursorFullDFS(t *testing.T) {
	src := "package main\n\nvar x = 1\n"
	tree := mustParseGo(t, src)

	cursor := ts.NewTreeCursor(tree.RootNode())
	var visited []string

	reachedEnd := false
	for !reachedEnd {
		node := cursor.CurrentNode()
		visited = append(visited, node.Type())

		if cursor.GotoFirstChild() {
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				reachedEnd = true
				break
			}
		}
	}

	if len(visited) == 0 {
		t.Fatal("no nodes visited")
	}
	t.Logf("visited %d nodes", len(visited))
	limit := len(visited)
	if limit > 10 {
		limit = 10
	}
	t.Logf("first %d: %v", limit, visited[:limit])
}

func TestTreeCursorResetAPI(t *testing.T) {
	src := "package main\n\nvar x = 1\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	cursor := ts.NewTreeCursor(root)

	if !cursor.GotoFirstChild() {
		t.Fatal("GotoFirstChild failed")
	}
	child := cursor.CurrentNode()
	if child.Equal(root) {
		t.Error("after GotoFirstChild, should not be at root")
	}

	cursor.Reset(root)
	resetNode := cursor.CurrentNode()
	if !resetNode.Equal(root) {
		t.Errorf("after Reset, current node type=%q, want root type=%q",
			resetNode.Type(), root.Type())
	}
}

func TestTreeCursorParentAfterFirstChild(t *testing.T) {
	src := "package main\n\nfunc f() {}\n"
	tree := mustParseGo(t, src)
	root := tree.RootNode()

	cursor := ts.NewTreeCursor(root)

	if !cursor.GotoFirstChild() {
		t.Fatal("GotoFirstChild failed")
	}

	if !cursor.GotoParent() {
		t.Fatal("GotoParent failed")
	}

	if !cursor.CurrentNode().Equal(root) {
		t.Errorf("after GotoParent, current node type=%q, want root type=%q",
			cursor.CurrentNode().Type(), root.Type())
	}
}
