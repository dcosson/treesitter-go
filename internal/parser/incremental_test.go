package parser

import "testing"

// --- Arena Fork tests ---

func TestArenaFork(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Allocate a leaf in the original arena.
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	// Fork the arena.
	forked := arena.Fork()

	// Old leaf should be accessible from forked arena.
	sym := GetSymbol(leaf, forked)
	if sym != 1 {
		t.Errorf("expected symbol 1 from forked arena, got %d", sym)
	}
	size := GetSize(leaf, forked)
	if size.Bytes != 3 {
		t.Errorf("expected size 3 from forked arena, got %d", size.Bytes)
	}
}

func TestArenaForkNewAllocations(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Fill the original arena with some nodes.
	leaf1 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	forked := arena.Fork()

	// Allocate a new node in the forked arena.
	leaf2 := NewLeafSubtree(forked, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(2), false, false, false, lang)

	// Both old and new nodes accessible from forked arena.
	if GetSymbol(leaf1, forked) != 1 {
		t.Error("old leaf not accessible from forked arena")
	}
	if GetSymbol(leaf2, forked) != 2 {
		t.Error("new leaf not accessible from forked arena")
	}
	if GetSize(leaf2, forked).Bytes != 2 {
		t.Error("new leaf size incorrect in forked arena")
	}
}

func TestArenaForkIndependence(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Create a heap-allocated node in the original arena.
	origLeaf := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 7, Point: Point{Column: 7}},
		StateID(1), false, false, false, lang)

	forked := arena.Fork()

	// Allocate in forked — should not corrupt original data.
	NewLeafSubtree(forked, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 100, Point: Point{Column: 100}},
		StateID(1), false, false, false, lang)

	// Original leaf should still be readable with correct data from both arenas.
	if GetSymbol(origLeaf, arena) != 5 {
		t.Error("original leaf symbol corrupted after fork allocation")
	}
	if GetSize(origLeaf, arena).Bytes != 7 {
		t.Error("original leaf size corrupted after fork allocation")
	}
	if GetSymbol(origLeaf, forked) != 5 {
		t.Error("original leaf not accessible from forked arena")
	}

	// Parent arena is frozen after fork: allocating in it creates a new block
	// that doesn't affect the forked arena's shared blocks.
	parentLeaf := NewLeafSubtree(arena, Symbol(9),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	if GetSymbol(parentLeaf, arena) != 9 {
		t.Error("post-fork parent allocation failed")
	}
	// Original data still intact in both arenas.
	if GetSymbol(origLeaf, forked) != 5 {
		t.Error("original leaf corrupted after post-fork parent allocation")
	}
}

// --- ReusableNode with edited tree test ---

func TestReusableNodeWithEditedTree(t *testing.T) {
	// Build a simple tree: document -> null (inline leaf)
	// Edit it so the null leaf has has_changes, then verify the
	// ReusableNode correctly reports has_changes and doesn't reuse it.
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	leaf := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 4, Point: Point{Column: 4}},
		StateID(1), false, false, false, lang)

	doc := NewNodeSubtree(arena, Symbol(8), []Subtree{leaf}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	// Fork and edit: replace bytes 0-4 with 0-4.
	forked := arena.Fork()
	edit := &InputEdit{
		StartByte: 0, OldEndByte: 4, NewEndByte: 4,
		StartPoint:  Point{Row: 0, Column: 0},
		OldEndPoint: Point{Row: 0, Column: 4},
		NewEndPoint: Point{Row: 0, Column: 4},
	}
	newRoot := editSubtree(doc, edit, forked)

	// Fork again (simulates Parse forking the edited arena).
	parseArena := forked.Fork()

	// Create ReusableNode with the edited root.
	rn := NewReusableNode(newRoot, parseArena)

	// The first tree should be the edited root.
	if rn.Done() {
		t.Fatal("ReusableNode should not be done at start")
	}
	rootTree := rn.Tree()
	if !HasChanges(rootTree, parseArena) {
		t.Error("root should have has_changes")
	}

	// Descend into the root's children.
	rn.Descend()
	if rn.Done() {
		t.Fatal("ReusableNode should have children after descending root")
	}

	childTree := rn.Tree()
	t.Logf("child inline=%v", childTree.IsInline())
	t.Logf("child HasChanges=%v", HasChanges(childTree, parseArena))
	t.Logf("child InlineHasChanges=%v", childTree.InlineHasChanges())

	if !HasChanges(childTree, parseArena) {
		t.Error("inline child should have has_changes after edit")
	}
}

// --- Inline has_changes test ---

func TestEditSubtreeInlineHasChanges(t *testing.T) {
	// Verify that editing an inline leaf sets has_changes on it.
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Create a leaf: symbol 7, padding=0, size=4 (like "null").
	leaf := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 4, Point: Point{Column: 4}},
		StateID(1), false, false, false, lang)

	// Create parent with this leaf.
	doc := NewNodeSubtree(arena, Symbol(8), []Subtree{leaf}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	// Fork and edit: replace bytes 0-4 with bytes 0-4 (same size).
	forked := arena.Fork()
	edit := &InputEdit{
		StartByte: 0, OldEndByte: 4, NewEndByte: 4,
		StartPoint:  Point{Row: 0, Column: 0},
		OldEndPoint: Point{Row: 0, Column: 4},
		NewEndPoint: Point{Row: 0, Column: 4},
	}
	newRoot := editSubtree(doc, edit, forked)
	if newRoot.IsZero() {
		t.Fatal("editSubtree returned zero subtree")
	}

	// Root should have has_changes.
	if !HasChanges(newRoot, forked) {
		t.Error("edited root should have has_changes")
	}

	// Check the child (inline leaf).
	children := GetChildren(newRoot, forked)
	if len(children) == 0 {
		t.Fatal("edited root should have children")
	}
	child := children[0]
	if !child.IsInline() {
		t.Log("child is heap-allocated, not inline")
	}
	if !HasChanges(child, forked) {
		t.Error("edited inline child should have has_changes")
	}
}

// --- editSubtree child 0 padding test (P1 regression) ---

func TestEditSubtreeChildZeroPadding(t *testing.T) {
	// Verify that an edit within child 0's padding correctly sets
	// has_changes on child 0. This tests the fix where childOffset
	// must start at 0 (not padding.Bytes), because per SummarizeChildren
	// convention, parent.Padding == child0.Padding.
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Create child0 with padding=2, size=3 (like "  foo")
	child0 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 2, Point: Point{Column: 2}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	// Create child1 with padding=1, size=2 (like " ab")
	child1 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 1, Point: Point{Column: 1}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(2), false, false, false, lang)

	// Parent: children = [child0, child1]
	// After SummarizeChildren: parent.Padding = child0.Padding = 2, parent.Size = 3+1+2 = 6
	// Total bytes = 2 + 6 = 8
	parent := NewNodeSubtree(arena, Symbol(8), []Subtree{child0, child1}, 0, lang)
	SummarizeChildren(parent, arena, lang)

	// Edit within child0's padding: insert at byte 1 (within the 2-byte padding).
	forked := arena.Fork()
	edit := &InputEdit{
		StartByte: 1, OldEndByte: 1, NewEndByte: 2,
		StartPoint:  Point{Row: 0, Column: 1},
		OldEndPoint: Point{Row: 0, Column: 1},
		NewEndPoint: Point{Row: 0, Column: 2},
	}
	newParent := editSubtree(parent, edit, forked)

	// Parent should have has_changes.
	if !HasChanges(newParent, forked) {
		t.Error("edited parent should have has_changes")
	}

	// Child 0 should also have has_changes (edit is within its span).
	children := GetChildren(newParent, forked)
	if len(children) < 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}
	if !HasChanges(children[0], forked) {
		t.Error("child 0 should have has_changes (edit is within its padding)")
	}
}

// --- Tree.Edit tests ---

func TestTreeEditReturnsNewTree(t *testing.T) {
	tree, _ := buildTestTree()

	// Insert a character at byte 5.
	edited := tree.Edit(&InputEdit{
		StartByte:   5,
		OldEndByte:  5,
		NewEndByte:  6,
		StartPoint:  Point{Row: 0, Column: 5},
		OldEndPoint: Point{Row: 0, Column: 5},
		NewEndPoint: Point{Row: 0, Column: 6},
	})

	if edited == tree {
		t.Fatal("Edit should return a new tree, not modify the original")
	}

	// Original tree should be unchanged.
	origRoot := tree.RootNode()
	if origRoot.EndByte() != 7 {
		t.Errorf("original tree end byte changed: got %d, want 7", origRoot.EndByte())
	}
}

func TestTreeEditInsertCharacter(t *testing.T) {
	tree, _ := buildTestTree()
	// Source: {"a":1}  (7 bytes, 0-6)
	// Insert 'x' at byte 5 (before '1'): {"a":x1}  (8 bytes)

	edited := tree.Edit(&InputEdit{
		StartByte:   5,
		OldEndByte:  5,
		NewEndByte:  6,
		StartPoint:  Point{Row: 0, Column: 5},
		OldEndPoint: Point{Row: 0, Column: 5},
		NewEndPoint: Point{Row: 0, Column: 6},
	})

	root := edited.RootNode()
	if root.IsNull() {
		t.Fatal("edited tree root should not be null")
	}

	// Root's total span should grow by 1 byte.
	if root.EndByte() != 8 {
		t.Errorf("edited tree end byte = %d, want 8", root.EndByte())
	}

	// Root should have has_changes set.
	if !root.HasChanges() {
		t.Error("edited tree root should have has_changes = true")
	}
}

func TestTreeEditDeleteCharacter(t *testing.T) {
	tree, _ := buildTestTree()
	// Source: {"a":1}  (7 bytes)
	// Delete the ':' at byte 4: {"a"1}  (6 bytes)

	edited := tree.Edit(&InputEdit{
		StartByte:   4,
		OldEndByte:  5,
		NewEndByte:  4,
		StartPoint:  Point{Row: 0, Column: 4},
		OldEndPoint: Point{Row: 0, Column: 5},
		NewEndPoint: Point{Row: 0, Column: 4},
	})

	root := edited.RootNode()
	if root.EndByte() != 6 {
		t.Errorf("edited tree end byte = %d, want 6", root.EndByte())
	}
}

func TestTreeEditReplaceRange(t *testing.T) {
	tree, _ := buildTestTree()
	// Source: {"a":1}  (7 bytes)
	// Replace bytes 1-4 ("a":) with "bb": {"bb"1}  (6 bytes)
	// 3 bytes removed, 2 inserted -> delta = -1

	edited := tree.Edit(&InputEdit{
		StartByte:   1,
		OldEndByte:  5,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 1},
		OldEndPoint: Point{Row: 0, Column: 5},
		NewEndPoint: Point{Row: 0, Column: 3},
	})

	root := edited.RootNode()
	// Original 7 bytes - 4 bytes + 2 bytes = 5 bytes
	if root.EndByte() != 5 {
		t.Errorf("edited tree end byte = %d, want 5", root.EndByte())
	}
}

func TestTreeEditPreservesUnchangedSubtrees(t *testing.T) {
	tree, _ := buildTestTree()
	origArena := tree.Arena()

	// Edit at the end (byte 6-7, the '}').
	edited := tree.Edit(&InputEdit{
		StartByte:   6,
		OldEndByte:  7,
		NewEndByte:  8,
		StartPoint:  Point{Row: 0, Column: 6},
		OldEndPoint: Point{Row: 0, Column: 7},
		NewEndPoint: Point{Row: 0, Column: 8},
	})

	editedArena := edited.Arena()

	// The original arena's leaf at byte 0 ("{") should be shared.
	// We verify by checking that the forked arena has the same blocks.
	if origArena.BlockCount() > editedArena.BlockCount() {
		t.Error("forked arena should have at least as many blocks as original")
	}
}

func TestTreeEditNoopEdit(t *testing.T) {
	tree, _ := buildTestTree()

	// A no-op edit (start == old_end == new_end) should return the tree as-is.
	edited := tree.Edit(&InputEdit{
		StartByte:   3,
		OldEndByte:  3,
		NewEndByte:  3,
		StartPoint:  Point{Row: 0, Column: 3},
		OldEndPoint: Point{Row: 0, Column: 3},
		NewEndPoint: Point{Row: 0, Column: 3},
	})

	// Should still be a valid tree with same size.
	root := edited.RootNode()
	if root.EndByte() != 7 {
		t.Errorf("noop edit should preserve size: got %d, want 7", root.EndByte())
	}
}

func TestTreeEditEmptyTree(t *testing.T) {
	tree := &Tree{}
	edited := tree.Edit(&InputEdit{
		StartByte: 0, OldEndByte: 0, NewEndByte: 1,
	})
	// Should not panic on empty tree.
	if edited.RootSubtree().IsZero() {
		// Expected for empty tree.
	}
}

func TestTreeEditMultipleEdits(t *testing.T) {
	tree, _ := buildTestTree()
	// Source: {"a":1}  (7 bytes)

	// First edit: insert at byte 5 -> 8 bytes
	edited1 := tree.Edit(&InputEdit{
		StartByte:   5,
		OldEndByte:  5,
		NewEndByte:  6,
		StartPoint:  Point{Row: 0, Column: 5},
		OldEndPoint: Point{Row: 0, Column: 5},
		NewEndPoint: Point{Row: 0, Column: 6},
	})

	// Second edit on the already-edited tree: insert at byte 0 -> 9 bytes
	edited2 := edited1.Edit(&InputEdit{
		StartByte:   0,
		OldEndByte:  0,
		NewEndByte:  1,
		StartPoint:  Point{Row: 0, Column: 0},
		OldEndPoint: Point{Row: 0, Column: 0},
		NewEndPoint: Point{Row: 0, Column: 1},
	})

	root := edited2.RootNode()
	if root.EndByte() != 9 {
		t.Errorf("after 2 edits, end byte = %d, want 9", root.EndByte())
	}

	// All three trees should still be valid.
	if tree.RootNode().EndByte() != 7 {
		t.Error("original tree modified")
	}
	if edited1.RootNode().EndByte() != 8 {
		t.Error("first edited tree modified")
	}
}

func TestTreeEditInPadding(t *testing.T) {
	// Create a tree where a node has padding > 0.
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// "x" with 2 bytes of padding (whitespace before it).
	leaf := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 2, Point: Point{Column: 2}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	doc := NewNodeSubtree(arena, Symbol(8), []Subtree{leaf}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	tree := NewTree(doc, lang, nil, []*SubtreeArena{arena})
	// Total: 2 (padding) + 1 (size) = 3 bytes

	// Insert 1 byte in padding area (byte 0-1).
	edited := tree.Edit(&InputEdit{
		StartByte:   1,
		OldEndByte:  1,
		NewEndByte:  2,
		StartPoint:  Point{Row: 0, Column: 1},
		OldEndPoint: Point{Row: 0, Column: 1},
		NewEndPoint: Point{Row: 0, Column: 2},
	})

	root := edited.RootNode()
	// Padding grows by 1, total = 4.
	if root.EndByte() != 4 {
		t.Errorf("after padding edit, end byte = %d, want 4", root.EndByte())
	}
}

func TestTreeCopy(t *testing.T) {
	tree, _ := buildTestTree()
	cp := tree.Copy()

	if cp == tree {
		t.Error("Copy should return a different *Tree")
	}
	if cp.RootSubtree() != tree.RootSubtree() {
		t.Error("Copy should share the same root subtree")
	}
	if cp.Language() != tree.Language() {
		t.Error("Copy should share the same language")
	}
}

// --- saturatingSub tests ---

func TestSaturatingSub(t *testing.T) {
	if saturatingSub(5, 3) != 2 {
		t.Error("5-3 should be 2")
	}
	if saturatingSub(3, 5) != 0 {
		t.Error("3-5 should saturate to 0")
	}
	if saturatingSub(0, 0) != 0 {
		t.Error("0-0 should be 0")
	}
}

func TestLengthSaturatingSub(t *testing.T) {
	a := Length{Bytes: 10, Point: Point{Row: 2, Column: 5}}
	b := Length{Bytes: 3, Point: Point{Row: 1, Column: 10}}
	result := LengthSaturatingSub(a, b)

	if result.Bytes != 7 {
		t.Errorf("bytes: got %d, want 7", result.Bytes)
	}
	if result.Point.Row != 1 {
		t.Errorf("row: got %d, want 1", result.Point.Row)
	}
	// Different rows -> column comes from a.
	if result.Point.Column != 5 {
		t.Errorf("column: got %d, want 5", result.Point.Column)
	}
}

func TestLengthSaturatingSubSameRow(t *testing.T) {
	a := Length{Bytes: 10, Point: Point{Row: 2, Column: 8}}
	b := Length{Bytes: 3, Point: Point{Row: 2, Column: 3}}
	result := LengthSaturatingSub(a, b)

	if result.Point.Column != 5 {
		t.Errorf("same-row column: got %d, want 5", result.Point.Column)
	}
}

func TestLengthSaturatingSubUnderflow(t *testing.T) {
	a := Length{Bytes: 2, Point: Point{Row: 0, Column: 2}}
	b := Length{Bytes: 5, Point: Point{Row: 0, Column: 5}}
	result := LengthSaturatingSub(a, b)

	if result.Bytes != 0 {
		t.Errorf("bytes should saturate to 0, got %d", result.Bytes)
	}
	if result.Point.Column != 0 {
		t.Errorf("column should saturate to 0, got %d", result.Point.Column)
	}
}

// --- breakdownLookahead tests ---

// buildCompositeTree creates a 3-level tree for breakdown testing:
//
//	document (symbol 8, parseState 10)
//	  ├── object (symbol 3, parseState 5)
//	  │     ├── "{" (symbol 1, parseState 2, leaf)
//	  │     └── "}" (symbol 2, parseState 3, leaf)
//	  └── number (symbol 7, parseState 7, leaf)
func buildCompositeTree(arena *SubtreeArena, lang *Language) Subtree {
	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)

	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(3), false, false, false, lang)

	object := NewNodeSubtree(arena, Symbol(3), []Subtree{lbrace, rbrace}, 0, lang)
	SummarizeChildren(object, arena, lang)
	SetParseState(object, arena, StateID(5))

	number := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(7), false, false, false, lang)

	doc := NewNodeSubtree(arena, Symbol(8), []Subtree{object, number}, 0, lang)
	SummarizeChildren(doc, arena, lang)
	SetParseState(doc, arena, StateID(10))

	return doc
}

func TestBreakdownLookaheadDescendsToMatchingState(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	doc := buildCompositeTree(arena, lang)

	// Create a parser with a reusable node rooted at the composite tree.
	p := &Parser{
		arena:        arena,
		language:     lang,
		reusableNode: NewReusableNode(doc, arena),
	}

	// Lookahead is the document node (parseState=10, children>0).
	// Request breakdown with target state 5 (matching the object child).
	lookahead := doc
	p.breakdownLookahead(&lookahead, StateID(5))

	// Should have descended to the object node (parseState=5).
	if GetSymbol(lookahead, arena) != Symbol(3) {
		t.Errorf("expected symbol 3 (object), got %d", GetSymbol(lookahead, arena))
	}
	if GetParseState(lookahead, arena) != StateID(5) {
		t.Errorf("expected parseState 5, got %d", GetParseState(lookahead, arena))
	}
}

func TestBreakdownLookaheadDescendsToLeaf(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	doc := buildCompositeTree(arena, lang)

	p := &Parser{
		arena:        arena,
		language:     lang,
		reusableNode: NewReusableNode(doc, arena),
	}

	// Request breakdown with target state 99 (no node matches).
	// Should descend all the way to the first leaf "{" (parseState=2, no children).
	lookahead := doc
	p.breakdownLookahead(&lookahead, StateID(99))

	// Should stop at the first leaf (no more children to descend into).
	if GetChildCount(lookahead, arena) != 0 {
		t.Errorf("expected leaf (childCount=0), got childCount=%d", GetChildCount(lookahead, arena))
	}
	if GetSymbol(lookahead, arena) != Symbol(1) {
		t.Errorf("expected symbol 1 ({), got %d", GetSymbol(lookahead, arena))
	}
}

func TestBreakdownLookaheadNoopWhenStateMatches(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	doc := buildCompositeTree(arena, lang)

	p := &Parser{
		arena:        arena,
		language:     lang,
		reusableNode: NewReusableNode(doc, arena),
	}

	// Request breakdown with target state 10 (matches document's parseState).
	// Should NOT descend because the state already matches.
	lookahead := doc
	origSymbol := GetSymbol(lookahead, arena)
	p.breakdownLookahead(&lookahead, StateID(10))

	if GetSymbol(lookahead, arena) != origSymbol {
		t.Errorf("expected no change (symbol %d), got symbol %d",
			origSymbol, GetSymbol(lookahead, arena))
	}
}

func TestBreakdownLookaheadNoopForLeaf(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Create a single leaf node.
	leaf := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	p := &Parser{
		arena:        arena,
		language:     lang,
		reusableNode: NewReusableNode(leaf, arena),
	}

	// A leaf has childCount=0, so the while loop body never executes.
	lookahead := leaf
	p.breakdownLookahead(&lookahead, StateID(99))

	// Lookahead should be unchanged.
	if GetSymbol(lookahead, arena) != Symbol(7) {
		t.Errorf("expected leaf unchanged (symbol 7), got %d", GetSymbol(lookahead, arena))
	}
}

func TestBreakdownLookaheadNoopWhenNoReusableNode(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	doc := buildCompositeTree(arena, lang)

	// Parser with no reusable node (non-incremental parse).
	p := &Parser{
		arena:    arena,
		language: lang,
	}

	lookahead := doc
	origSymbol := GetSymbol(lookahead, arena)
	p.breakdownLookahead(&lookahead, StateID(0))

	if GetSymbol(lookahead, arena) != origSymbol {
		t.Errorf("expected no change without reusableNode, got symbol %d",
			GetSymbol(lookahead, arena))
	}
}
