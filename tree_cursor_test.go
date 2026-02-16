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
