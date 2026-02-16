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
	cursor.GotoFirstChild() // object
	cursor.GotoFirstChild() // "{"
	cursor.GotoNextSibling() // pair

	pair := cursor.CurrentNode()
	if pair.StartByte() != 1 {
		t.Errorf("pair startByte = %d, want 1", pair.StartByte())
	}
	if pair.EndByte() != 6 {
		t.Errorf("pair endByte = %d, want 6", pair.EndByte())
	}

	// Navigate to ":" (byte 4).
	cursor.GotoFirstChild() // string
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
//   document -> _hidden1 -> _hidden2 -> number
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
