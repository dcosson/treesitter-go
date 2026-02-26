package treesitter

import "testing"

func TestTreeCursorBasic(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Should start at root.
	current := cursor.CurrentNode()
	if !current.Equal(root) {
		t.Errorf("cursor start = %q, want root (document)", current.Type())
	}
}

func TestTreeCursorGotoFirstChild(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// First child of document is object.
	if !cursor.GotoFirstChild() {
		t.Fatal("should have first child")
	}
	current := cursor.CurrentNode()
	if current.Type() != "object" {
		t.Errorf("first child = %q, want \"object\"", current.Type())
	}

	// First child of object is "{".
	if !cursor.GotoFirstChild() {
		t.Fatal("should have first child of object")
	}
	current = cursor.CurrentNode()
	if current.Type() != "{" {
		t.Errorf("first child of object = %q, want \"{\"", current.Type())
	}

	// "{" is a leaf — no children.
	if cursor.GotoFirstChild() {
		t.Error("leaf should have no children")
	}
}

func TestTreeCursorGotoNextSibling(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Navigate to object -> first child "{".
	cursor.GotoFirstChild() // document -> object
	cursor.GotoFirstChild() // object -> "{"

	// Next sibling of "{" is pair.
	if !cursor.GotoNextSibling() {
		t.Fatal("should have next sibling")
	}
	current := cursor.CurrentNode()
	if current.Type() != "pair" {
		t.Errorf("next sibling = %q, want \"pair\"", current.Type())
	}

	// Next sibling of pair is "}".
	if !cursor.GotoNextSibling() {
		t.Fatal("should have next sibling")
	}
	current = cursor.CurrentNode()
	if current.Type() != "}" {
		t.Errorf("next sibling = %q, want \"}\"", current.Type())
	}

	// No more siblings.
	if cursor.GotoNextSibling() {
		t.Error("should not have more siblings")
	}
}

func TestTreeCursorGotoParent(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Can't go up from root.
	if cursor.GotoParent() {
		t.Error("should not be able to go above root")
	}

	// Navigate down and back up.
	cursor.GotoFirstChild() // -> object
	cursor.GotoFirstChild() // -> "{"

	if !cursor.GotoParent() {
		t.Fatal("should go to parent")
	}
	current := cursor.CurrentNode()
	if current.Type() != "object" {
		t.Errorf("parent = %q, want \"object\"", current.Type())
	}

	if !cursor.GotoParent() {
		t.Fatal("should go to document")
	}
	current = cursor.CurrentNode()
	if current.Type() != "document" {
		t.Errorf("grandparent = %q, want \"document\"", current.Type())
	}
}

func TestTreeCursorFullTraversal(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Do a full depth-first traversal, collecting node types.
	var visited []string
	var traverse func()
	traverse = func() {
		current := cursor.CurrentNode()
		visited = append(visited, current.Type())

		if cursor.GotoFirstChild() {
			for {
				traverse()
				if !cursor.GotoNextSibling() {
					break
				}
			}
			cursor.GotoParent()
		}
	}
	traverse()

	// Expected DFS order: document, object, "{", pair, string, ":", number, "}"
	expected := []string{"document", "object", "{", "pair", "string", ":", "number", "}"}
	if len(visited) != len(expected) {
		t.Fatalf("visited %d nodes, want %d: %v", len(visited), len(expected), visited)
	}
	for i, v := range visited {
		if v != expected[i] {
			t.Errorf("visited[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestTreeCursorReset(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Navigate deep.
	cursor.GotoFirstChild()
	cursor.GotoFirstChild()
	cursor.GotoNextSibling()

	current := cursor.CurrentNode()
	if current.Type() != "pair" {
		t.Fatalf("before reset = %q, want \"pair\"", current.Type())
	}

	// Reset to root.
	cursor.Reset(root)
	current = cursor.CurrentNode()
	if current.Type() != "document" {
		t.Errorf("after reset = %q, want \"document\"", current.Type())
	}
}

func TestTreeCursorPositions(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Navigate to pair (byte 1-6).
	cursor.GotoFirstChild()  // object
	cursor.GotoFirstChild()  // "{"
	cursor.GotoNextSibling() // pair

	pair := cursor.CurrentNode()
	if pair.StartByte() != 1 {
		t.Errorf("pair startByte = %d, want 1", pair.StartByte())
	}
	if pair.EndByte() != 6 {
		t.Errorf("pair endByte = %d, want 6", pair.EndByte())
	}

	// Navigate to ":" (byte 4).
	cursor.GotoFirstChild()  // string
	cursor.GotoNextSibling() // ":"

	colon := cursor.CurrentNode()
	if colon.StartByte() != 4 {
		t.Errorf("colon startByte = %d, want 4", colon.StartByte())
	}
	if colon.EndByte() != 5 {
		t.Errorf("colon endByte = %d, want 5", colon.EndByte())
	}
}

// buildDeeplyNestedHiddenTree builds a tree where a visible node is nested
// behind multiple levels of hidden nodes:
//
//	document -> _hidden1 -> _hidden2 -> number
//
// This tests that cursor and node traversal handle arbitrary hidden depth.
func buildDeeplyNestedHiddenTree() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(32)
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: false, Named: false}, // 1: _hidden1
			{Visible: false, Named: false}, // 2: _hidden2
			{Visible: true, Named: true},   // 3: number
			{Visible: true, Named: true},   // 4: document
		},
		SymbolNames: []string{"end", "_hidden1", "_hidden2", "number", "document"},
	}

	// Leaf: number at byte 0, size 1
	num := NewLeafSubtree(arena, Symbol(3),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	// _hidden2 -> number
	hidden2 := NewNodeSubtree(arena, Symbol(2), []Subtree{num}, 0, lang)
	SummarizeChildren(hidden2, arena, lang)

	// _hidden1 -> _hidden2
	hidden1 := NewNodeSubtree(arena, Symbol(1), []Subtree{hidden2}, 0, lang)
	SummarizeChildren(hidden1, arena, lang)

	// document -> _hidden1
	doc := NewNodeSubtree(arena, Symbol(4), []Subtree{hidden1}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	tree := NewTree(doc, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}

// buildHiddenSiblingTree builds a tree where GotoNextSibling must traverse
// through a hidden node to find the next visible sibling:
//
//	container -> string, _hidden(children: [number]), ";"
//
// After visiting string, GotoNextSibling should find number (through _hidden),
// then GotoNextSibling again should find ";".
func buildHiddenSiblingTree() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(32)
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: true, Named: true},   // 1: string
			{Visible: true, Named: true},   // 2: number
			{Visible: false, Named: false}, // 3: _hidden
			{Visible: true, Named: false},  // 4: ";"
			{Visible: true, Named: true},   // 5: container
			{Visible: true, Named: true},   // 6: document
		},
		SymbolNames: []string{"end", "string", "number", "_hidden", ";", "container", "document"},
	}

	// string at byte 0, size 3
	str := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	// number at byte 3, size 2
	num := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(2), false, false, false, lang)

	// _hidden -> number
	hidden := NewNodeSubtree(arena, Symbol(3), []Subtree{num}, 0, lang)
	SummarizeChildren(hidden, arena, lang)

	// ";" at byte 5, size 1
	semi := NewLeafSubtree(arena, Symbol(4),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(3), false, false, false, lang)

	// container -> string, _hidden(number), ";"
	container := NewNodeSubtree(arena, Symbol(5), []Subtree{str, hidden, semi}, 0, lang)
	SummarizeChildren(container, arena, lang)

	// document -> container
	doc := NewNodeSubtree(arena, Symbol(6), []Subtree{container}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	tree := NewTree(doc, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}

func TestTreeCursorNextSiblingThroughHidden(t *testing.T) {
	tree, _ := buildHiddenSiblingTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Navigate to container -> string.
	cursor.GotoFirstChild() // document -> container
	if !cursor.GotoFirstChild() {
		t.Fatal("should find first child of container")
	}
	current := cursor.CurrentNode()
	if current.Type() != "string" {
		t.Fatalf("first child = %q, want \"string\"", current.Type())
	}

	// GotoNextSibling should find number through _hidden.
	if !cursor.GotoNextSibling() {
		t.Fatal("should find sibling through hidden node")
	}
	current = cursor.CurrentNode()
	if current.Type() != "number" {
		t.Errorf("sibling through hidden = %q, want \"number\"", current.Type())
	}

	// GotoNextSibling should find ";" after the hidden node.
	if !cursor.GotoNextSibling() {
		t.Fatal("should find semicolon sibling")
	}
	current = cursor.CurrentNode()
	if current.Type() != ";" {
		t.Errorf("next sibling = %q, want \";\"", current.Type())
	}

	// No more siblings.
	if cursor.GotoNextSibling() {
		t.Error("should not have more siblings")
	}

	// GotoParent should go back to container.
	if !cursor.GotoParent() {
		t.Fatal("should go to parent")
	}
	current = cursor.CurrentNode()
	if current.Type() != "container" {
		t.Errorf("parent = %q, want \"container\"", current.Type())
	}
}

func TestTreeCursorDeepHiddenNodes(t *testing.T) {
	tree, _ := buildDeeplyNestedHiddenTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Document -> (through 2 hidden layers) -> number
	if !cursor.GotoFirstChild() {
		t.Fatal("should find child through hidden layers")
	}
	current := cursor.CurrentNode()
	if current.Type() != "number" {
		t.Errorf("child through deep hidden = %q, want \"number\"", current.Type())
	}
	if current.StartByte() != 0 {
		t.Errorf("number startByte = %d, want 0", current.StartByte())
	}

	// Should be able to go back up to document.
	if !cursor.GotoParent() {
		t.Fatal("should go back to parent")
	}
	current = cursor.CurrentNode()
	if current.Type() != "document" {
		t.Errorf("parent = %q, want \"document\"", current.Type())
	}
}

// buildAliasedTree builds a tree where a production uses aliases:
//
//	wrapper (prodID=1) -> identifier (aliased to "name"), ":" (no alias), number (aliased to "value")
//	document -> wrapper
//
// Alias sequence for prodID 1: [name_sym, 0, value_sym] (structural indices 0, 1, 2)
func buildAliasedTree() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(32)
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: true, Named: true},   // 1: identifier
			{Visible: true, Named: false},  // 2: ":"
			{Visible: true, Named: true},   // 3: number
			{Visible: true, Named: true},   // 4: wrapper
			{Visible: true, Named: true},   // 5: document
			{Visible: true, Named: true},   // 6: name (alias)
			{Visible: true, Named: true},   // 7: value (alias)
		},
		SymbolNames:            []string{"end", "identifier", ":", "number", "wrapper", "document", "name", "value"},
		MaxAliasSequenceLength: 3,
		AliasSequences: []Symbol{
			// prodID 0: no aliases
			0, 0, 0,
			// prodID 1 (wrapper): child 0 -> name(6), child 1 -> no alias, child 2 -> value(7)
			6, 0, 7,
		},
	}

	// identifier at byte 0, size 3
	ident := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	// ":" at byte 3, size 1
	colon := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)

	// number at byte 4, size 2
	num := NewLeafSubtree(arena, Symbol(3),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(3), false, false, false, lang)

	// wrapper (prodID=1) -> identifier, ":", number
	wrapper := NewNodeSubtree(arena, Symbol(4), []Subtree{ident, colon, num}, 1, lang)
	SummarizeChildren(wrapper, arena, lang)

	// document -> wrapper
	doc := NewNodeSubtree(arena, Symbol(5), []Subtree{wrapper}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	tree := NewTree(doc, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}

func TestTreeCursorAliasResolution(t *testing.T) {
	tree, _ := buildAliasedTree()
	root := tree.RootNode()
	cursor := NewTreeCursor(root)

	// Navigate to wrapper.
	if !cursor.GotoFirstChild() {
		t.Fatal("should find wrapper")
	}
	current := cursor.CurrentNode()
	if current.Type() != "wrapper" {
		t.Fatalf("first child = %q, want \"wrapper\"", current.Type())
	}

	// Navigate to first child of wrapper: identifier aliased to "name".
	if !cursor.GotoFirstChild() {
		t.Fatal("should find first child of wrapper")
	}
	current = cursor.CurrentNode()
	if current.Type() != "name" {
		t.Errorf("aliased child 0 type = %q, want \"name\"", current.Type())
	}
	if current.Symbol() != 6 {
		t.Errorf("aliased child 0 symbol = %d, want 6 (name)", current.Symbol())
	}

	// Navigate to ":" (no alias).
	if !cursor.GotoNextSibling() {
		t.Fatal("should find colon sibling")
	}
	current = cursor.CurrentNode()
	if current.Type() != ":" {
		t.Errorf("child 1 type = %q, want \":\"", current.Type())
	}
	if current.Symbol() != 2 {
		t.Errorf("child 1 symbol = %d, want 2 (colon)", current.Symbol())
	}

	// Navigate to number aliased to "value".
	if !cursor.GotoNextSibling() {
		t.Fatal("should find number sibling")
	}
	current = cursor.CurrentNode()
	if current.Type() != "value" {
		t.Errorf("aliased child 2 type = %q, want \"value\"", current.Type())
	}
	if current.Symbol() != 7 {
		t.Errorf("aliased child 2 symbol = %d, want 7 (value)", current.Symbol())
	}
}

func TestNodeChildAliasResolution(t *testing.T) {
	tree, _ := buildAliasedTree()
	root := tree.RootNode()

	// Get wrapper node.
	wrapper := root.Child(0)
	if wrapper.IsNull() || wrapper.Type() != "wrapper" {
		t.Fatalf("child 0 = %q, want \"wrapper\"", wrapper.Type())
	}

	// Child(0) of wrapper: identifier aliased to "name".
	child0 := wrapper.Child(0)
	if child0.IsNull() {
		t.Fatal("wrapper child 0 should not be null")
	}
	if child0.Type() != "name" {
		t.Errorf("wrapper.Child(0) type = %q, want \"name\"", child0.Type())
	}
	if child0.Symbol() != 6 {
		t.Errorf("wrapper.Child(0) symbol = %d, want 6", child0.Symbol())
	}

	// Child(1) of wrapper: ":" (no alias).
	child1 := wrapper.Child(1)
	if child1.Type() != ":" {
		t.Errorf("wrapper.Child(1) type = %q, want \":\"", child1.Type())
	}

	// Child(2) of wrapper: number aliased to "value".
	child2 := wrapper.Child(2)
	if child2.IsNull() {
		t.Fatal("wrapper child 2 should not be null")
	}
	if child2.Type() != "value" {
		t.Errorf("wrapper.Child(2) type = %q, want \"value\"", child2.Type())
	}
	if child2.Symbol() != 7 {
		t.Errorf("wrapper.Child(2) symbol = %d, want 7", child2.Symbol())
	}

	// NamedChild should also resolve aliases.
	named0 := wrapper.NamedChild(0)
	if named0.Type() != "name" {
		t.Errorf("wrapper.NamedChild(0) type = %q, want \"name\"", named0.Type())
	}
	named1 := wrapper.NamedChild(1)
	if named1.Type() != "value" {
		t.Errorf("wrapper.NamedChild(1) type = %q, want \"value\"", named1.Type())
	}
}

// buildHiddenAliasedTree builds a tree where a hidden node carries the alias:
//
//	document -> _rule (hidden, prodID=1) -> number (aliased to "count" by _rule's production)
//
// This tests that alias resolution uses the immediate parent (even if hidden).
func buildHiddenAliasedTree() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(32)
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: false, Named: false}, // 1: _rule (hidden)
			{Visible: true, Named: true},   // 2: number
			{Visible: true, Named: true},   // 3: document
			{Visible: true, Named: true},   // 4: count (alias)
		},
		SymbolNames:            []string{"end", "_rule", "number", "document", "count"},
		MaxAliasSequenceLength: 1,
		AliasSequences: []Symbol{
			// prodID 0: no aliases
			0,
			// prodID 1 (_rule): child 0 -> count(4)
			4,
		},
	}

	// number at byte 0, size 2
	num := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(1), false, false, false, lang)

	// _rule (prodID=1) -> number
	rule := NewNodeSubtree(arena, Symbol(1), []Subtree{num}, 1, lang)
	SummarizeChildren(rule, arena, lang)

	// document -> _rule
	doc := NewNodeSubtree(arena, Symbol(3), []Subtree{rule}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	tree := NewTree(doc, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}

func TestAliasResolutionThroughHiddenNodes(t *testing.T) {
	tree, _ := buildHiddenAliasedTree()
	root := tree.RootNode()

	// TreeCursor: document -> (through hidden _rule) -> number aliased to "count"
	cursor := NewTreeCursor(root)
	if !cursor.GotoFirstChild() {
		t.Fatal("should find child through hidden layer")
	}
	current := cursor.CurrentNode()
	if current.Type() != "count" {
		t.Errorf("cursor child type = %q, want \"count\"", current.Type())
	}
	if current.Symbol() != 4 {
		t.Errorf("cursor child symbol = %d, want 4 (count)", current.Symbol())
	}

	// Node.Child: same alias should resolve.
	child := root.Child(0)
	if child.IsNull() {
		t.Fatal("child should not be null")
	}
	if child.Type() != "count" {
		t.Errorf("Node.Child(0) type = %q, want \"count\"", child.Type())
	}
	if child.Symbol() != 4 {
		t.Errorf("Node.Child(0) symbol = %d, want 4 (count)", child.Symbol())
	}

	// NamedChild should also get the alias.
	named := root.NamedChild(0)
	if named.Type() != "count" {
		t.Errorf("Node.NamedChild(0) type = %q, want \"count\"", named.Type())
	}
}

func TestNodeChildDeepHiddenNodes(t *testing.T) {
	tree, _ := buildDeeplyNestedHiddenTree()
	root := tree.RootNode()

	// Node.Child(0) should find number through 2 hidden layers.
	child := root.Child(0)
	if child.IsNull() {
		t.Fatal("child through deep hidden should not be null")
	}
	if child.Type() != "number" {
		t.Errorf("child type = %q, want \"number\"", child.Type())
	}

	// NamedChild(0) should also find it.
	named := root.NamedChild(0)
	if named.IsNull() {
		t.Fatal("named child through deep hidden should not be null")
	}
	if named.Type() != "number" {
		t.Errorf("named child type = %q, want \"number\"", named.Type())
	}
}

// buildAliasedTreeWithExtras builds a tree where an extra node (comment)
// appears between structural children. The alias sequence should use
// structural (non-extra) indices, so the comment should not shift alias offsets.
//
//	wrapper (prodID=1) -> identifier (alias:name), comment (extra), number (alias:value)
//	                       structural idx: 0                         structural idx: 1
//
// Alias sequence for prodID 1: [name_sym, value_sym]
func buildAliasedTreeWithExtras() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(32)
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: true, Named: true},   // 1: identifier
			{Visible: true, Named: true},   // 2: number
			{Visible: true, Named: true},   // 3: wrapper
			{Visible: true, Named: true},   // 4: document
			{Visible: true, Named: true},   // 5: name (alias)
			{Visible: true, Named: true},   // 6: value (alias)
			{Visible: true, Named: true},   // 7: comment (extra)
		},
		SymbolNames:            []string{"end", "identifier", "number", "wrapper", "document", "name", "value", "comment"},
		MaxAliasSequenceLength: 2,
		AliasSequences: []Symbol{
			// prodID 0: no aliases
			0, 0,
			// prodID 1 (wrapper): structural child 0 -> name(5), structural child 1 -> value(6)
			5, 6,
		},
	}

	// identifier at byte 0, size 3
	ident := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	// comment (extra) at byte 3, size 4
	// Use hasExternalTokens=true to force heap allocation so SetExtra works.
	comment := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 4, Point: Point{Column: 4}},
		StateID(2), true, false, false, lang)
	// Mark the comment as extra.
	SetExtra(comment, arena)

	// number at byte 7, size 2
	num := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(3), false, false, false, lang)

	// wrapper (prodID=1) -> identifier, comment(extra), number
	wrapper := NewNodeSubtree(arena, Symbol(3), []Subtree{ident, comment, num}, 1, lang)
	SummarizeChildren(wrapper, arena, lang)

	// document -> wrapper
	doc := NewNodeSubtree(arena, Symbol(4), []Subtree{wrapper}, 0, lang)
	SummarizeChildren(doc, arena, lang)

	tree := NewTree(doc, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}

func TestAliasResolutionSkipsExtras(t *testing.T) {
	tree, _ := buildAliasedTreeWithExtras()
	root := tree.RootNode()

	wrapper := root.Child(0)
	if wrapper.IsNull() || wrapper.Type() != "wrapper" {
		t.Fatalf("child 0 = %q, want \"wrapper\"", wrapper.Type())
	}

	// Child(0): identifier aliased to "name" (structural index 0).
	child0 := wrapper.Child(0)
	if child0.Type() != "name" {
		t.Errorf("Child(0) type = %q, want \"name\"", child0.Type())
	}

	// Child(1): comment (extra) — should NOT be aliased.
	child1 := wrapper.Child(1)
	if child1.Type() != "comment" {
		t.Errorf("Child(1) type = %q, want \"comment\"", child1.Type())
	}

	// Child(2): number aliased to "value" (structural index 1, NOT 2).
	// The comment (extra) should not have incremented the structural index.
	child2 := wrapper.Child(2)
	if child2.Type() != "value" {
		t.Errorf("Child(2) type = %q, want \"value\" (structural index should skip extras)", child2.Type())
	}

	// Same via TreeCursor.
	cursor := NewTreeCursor(root)
	cursor.GotoFirstChild() // -> wrapper
	cursor.GotoFirstChild() // -> identifier aliased to "name"
	current := cursor.CurrentNode()
	if current.Type() != "name" {
		t.Errorf("cursor child 0 type = %q, want \"name\"", current.Type())
	}

	cursor.GotoNextSibling() // -> comment (extra, no alias)
	current = cursor.CurrentNode()
	if current.Type() != "comment" {
		t.Errorf("cursor child 1 type = %q, want \"comment\"", current.Type())
	}

	cursor.GotoNextSibling() // -> number aliased to "value"
	current = cursor.CurrentNode()
	if current.Type() != "value" {
		t.Errorf("cursor child 2 type = %q, want \"value\"", current.Type())
	}
}

// TestTreeCursorNonZeroFirstChildPadding verifies that the cursor correctly
// handles trees where the first child has non-zero padding. This exercises
// the fix for the childBasePos double-count bug (wcu.11).
//
// Source: "  {1}" (2 spaces of leading whitespace)
//
//	document (padding=2, size=3) -> object (padding=2, size=3) -> "{" number "}"
//
// The first visible child "{" has padding=2. Without the fix, GotoFirstChild
// would double-count this padding, causing incorrect byte positions.
func TestTreeCursorNonZeroFirstChildPadding(t *testing.T) {
	arena := NewSubtreeArena(64)
	lang := makeSubtreeTestLanguage()

	// "{" at byte 2 (after 2 spaces of padding), size 1
	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 2, Point: Point{Column: 2}}, // padding = 2
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	// "1" (number) at byte 3, size 1 (no padding between { and 1)
	numVal := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(4), false, false, false, lang)

	// "}" at byte 4, size 1
	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(5), false, false, false, lang)

	// object -> "{" number "}"
	object := NewNodeSubtree(arena, Symbol(3), []Subtree{lbrace, numVal, rbrace}, 0, lang)
	SummarizeChildren(object, arena, lang)

	// document -> object (SummarizeChildren sets document.padding = object.padding = 2)
	document := NewNodeSubtree(arena, Symbol(8), []Subtree{object}, 0, lang)
	SummarizeChildren(document, arena, lang)

	tree := NewTree(document, lang, nil, []*SubtreeArena{arena})
	root := tree.RootNode()

	// Root (document) should start at byte 2 (content start, after 2 bytes padding).
	if root.StartByte() != 2 {
		t.Errorf("root startByte = %d, want 2", root.StartByte())
	}

	cursor := NewTreeCursor(root)

	// Descend to object (first visible child of document through hidden).
	if !cursor.GotoFirstChild() {
		t.Fatal("should have first child (object)")
	}
	obj := cursor.CurrentNode()
	if obj.Type() != "object" {
		t.Errorf("first child type = %q, want \"object\"", obj.Type())
	}
	if obj.StartByte() != 2 {
		t.Errorf("object startByte = %d, want 2", obj.StartByte())
	}

	// Descend to "{" (first visible child of object).
	if !cursor.GotoFirstChild() {
		t.Fatal("should have first child of object ({)")
	}
	brace := cursor.CurrentNode()
	if brace.Type() != "{" {
		t.Errorf("first child of object = %q, want \"{\"", brace.Type())
	}
	// This is the critical check: "{" should start at byte 2.
	// Without the wcu.11 fix, this would incorrectly be byte 4 (double-counted padding).
	if brace.StartByte() != 2 {
		t.Errorf("brace startByte = %d, want 2 (padding double-count bug!)", brace.StartByte())
	}

	// Next sibling: number at byte 3
	if !cursor.GotoNextSibling() {
		t.Fatal("should have next sibling (number)")
	}
	num := cursor.CurrentNode()
	if num.StartByte() != 3 {
		t.Errorf("number startByte = %d, want 3", num.StartByte())
	}

	// Next sibling: "}" at byte 4
	if !cursor.GotoNextSibling() {
		t.Fatal("should have next sibling (})")
	}
	rb := cursor.CurrentNode()
	if rb.StartByte() != 4 {
		t.Errorf("rbrace startByte = %d, want 4", rb.StartByte())
	}
}

// buildHiddenChildAliasedToVisible builds a tree that models the real-world case
// from Perl/Lua where a hidden symbol (e.g. _doublequote_string_content) is
// aliased to a visible symbol (string_content) by the parent's production.
//
//	string_literal (prodID=1) -> _string_content (hidden, aliased to string_content)
//
// The _string_content symbol is intrinsically hidden but the parent string_literal's
// production aliases it at structural index 0 to string_content (visible, named).
// Without alias-aware visibility, _string_content would be skipped as hidden and
// string_literal would appear to have no visible children.
func buildHiddenChildAliasedToVisible() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(32)
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: false, Named: false}, // 1: _string_content (hidden)
			{Visible: true, Named: true},   // 2: string_literal
			{Visible: true, Named: true},   // 3: source_file
			{Visible: true, Named: true},   // 4: string_content (alias target)
		},
		SymbolNames:            []string{"end", "_string_content", "string_literal", "source_file", "string_content"},
		MaxAliasSequenceLength: 1,
		AliasSequences: []Symbol{
			// prodID 0: no aliases
			0,
			// prodID 1 (string_literal): child 0 -> string_content(4)
			4,
		},
	}

	// _string_content at byte 0, size 5 (e.g. "hello")
	content := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(1), false, false, false, lang)

	// string_literal (prodID=1) -> _string_content
	strLit := NewNodeSubtree(arena, Symbol(2), []Subtree{content}, 1, lang)
	SummarizeChildren(strLit, arena, lang)

	// source_file -> string_literal
	root := NewNodeSubtree(arena, Symbol(3), []Subtree{strLit}, 0, lang)
	SummarizeChildren(root, arena, lang)

	tree := NewTree(root, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}

// TestHiddenChildAliasedToVisible verifies that a hidden child aliased to a
// visible symbol by its parent's production is treated as visible in all
// tree traversal methods. This is the core bug fixed by alias-aware visibility.
func TestHiddenChildAliasedToVisible(t *testing.T) {
	tree, arena := buildHiddenChildAliasedToVisible()
	root := tree.RootNode()

	// string_literal should report 1 visible child (the aliased _string_content).
	strLit := root.Child(0)
	if strLit.IsNull() {
		t.Fatal("string_literal should not be null")
	}
	if strLit.Type() != "string_literal" {
		t.Fatalf("root.Child(0) type = %q, want \"string_literal\"", strLit.Type())
	}

	// Check visible child count: SummarizeChildren should count the aliased child.
	strLitData := arena.Get(strLit.GetSubtree())
	if strLitData.VisibleChildCount != 1 {
		t.Errorf("string_literal.VisibleChildCount = %d, want 1", strLitData.VisibleChildCount)
	}
	if strLitData.NamedChildCount != 1 {
		t.Errorf("string_literal.NamedChildCount = %d, want 1", strLitData.NamedChildCount)
	}

	// Node.Child(0) should find the aliased child as "string_content".
	child := strLit.Child(0)
	if child.IsNull() {
		t.Fatal("string_literal.Child(0) should not be null")
	}
	if child.Type() != "string_content" {
		t.Errorf("string_literal.Child(0) type = %q, want \"string_content\"", child.Type())
	}
	if child.Symbol() != 4 {
		t.Errorf("string_literal.Child(0) symbol = %d, want 4 (string_content)", child.Symbol())
	}

	// Node.NamedChild(0) should also find it.
	named := strLit.NamedChild(0)
	if named.IsNull() {
		t.Fatal("string_literal.NamedChild(0) should not be null")
	}
	if named.Type() != "string_content" {
		t.Errorf("string_literal.NamedChild(0) type = %q, want \"string_content\"", named.Type())
	}

	// TreeCursor should also find it.
	cursor := NewTreeCursor(root)
	// Navigate to string_literal.
	if !cursor.GotoFirstChild() {
		t.Fatal("cursor: should find string_literal")
	}
	if cursor.CurrentNode().Type() != "string_literal" {
		t.Fatalf("cursor: first child = %q, want \"string_literal\"", cursor.CurrentNode().Type())
	}
	// Navigate into string_literal's children.
	if !cursor.GotoFirstChild() {
		t.Fatal("cursor: should find aliased child inside string_literal")
	}
	current := cursor.CurrentNode()
	if current.Type() != "string_content" {
		t.Errorf("cursor: string_literal child type = %q, want \"string_content\"", current.Type())
	}
	if current.Symbol() != 4 {
		t.Errorf("cursor: string_literal child symbol = %d, want 4", current.Symbol())
	}
}
