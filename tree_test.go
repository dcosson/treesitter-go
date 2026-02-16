package treesitter

import "testing"

// buildTestTree constructs a small JSON-like tree for testing:
//
//	document -> object -> "{" pair "}"
//	pair -> string ":" number
//
// Source: {"a":1}
// Bytes:  01234567
func buildTestTree() (*Tree, *SubtreeArena) {
	arena := NewSubtreeArena(64)
	lang := makeSubtreeTestLanguage()

	// Leaf tokens:
	// "{" at byte 0, size 1
	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	// "a" (string) at byte 1, size 3 ("a")
	strKey := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(2), false, false, false, lang)

	// ":" at byte 4, size 1
	colon := NewLeafSubtree(arena, Symbol(6),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(3), false, false, false, lang)

	// "1" (number) at byte 5, size 1
	numVal := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(4), false, false, false, lang)

	// "}" at byte 6, size 1
	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(5), false, false, false, lang)

	// pair -> string ":" number (prodID 1 for field mapping)
	pair := NewNodeSubtree(arena, Symbol(4), []Subtree{strKey, colon, numVal}, 1, lang)
	SummarizeChildren(pair, arena, lang)

	// object -> "{" pair "}"
	object := NewNodeSubtree(arena, Symbol(3), []Subtree{lbrace, pair, rbrace}, 0, lang)
	SummarizeChildren(object, arena, lang)

	// document -> object
	document := NewNodeSubtree(arena, Symbol(8), []Subtree{object}, 0, lang)
	SummarizeChildren(document, arena, lang)

	tree := NewTree(document, lang, nil, []*SubtreeArena{arena})
	return tree, arena
}

func TestTreeRootNode(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()

	if root.IsNull() {
		t.Fatal("root node should not be null")
	}
	if root.Type() != "document" {
		t.Errorf("root type = %q, want \"document\"", root.Type())
	}
	if root.Symbol() != 8 {
		t.Errorf("root symbol = %d, want 8", root.Symbol())
	}
	if root.StartByte() != 0 {
		t.Errorf("root startByte = %d, want 0", root.StartByte())
	}
	if root.EndByte() != 7 {
		t.Errorf("root endByte = %d, want 7", root.EndByte())
	}
	if root.StartPoint() != (Point{Row: 0, Column: 0}) {
		t.Errorf("root startPoint = %+v, want (0,0)", root.StartPoint())
	}
	if root.EndPoint() != (Point{Row: 0, Column: 7}) {
		t.Errorf("root endPoint = %+v, want (0,7)", root.EndPoint())
	}
}

func TestNodeChildCount(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()

	// document has 1 visible child: object
	if root.ChildCount() != 1 {
		t.Errorf("document childCount = %d, want 1", root.ChildCount())
	}
	if root.NamedChildCount() != 1 {
		t.Errorf("document namedChildCount = %d, want 1", root.NamedChildCount())
	}

	// Get the object child.
	obj := root.Child(0)
	if obj.IsNull() {
		t.Fatal("object child should not be null")
	}
	if obj.Type() != "object" {
		t.Errorf("object type = %q, want \"object\"", obj.Type())
	}

	// object has 3 visible children: "{", pair, "}"
	if obj.ChildCount() != 3 {
		t.Errorf("object childCount = %d, want 3", obj.ChildCount())
	}
	// Only pair is named.
	if obj.NamedChildCount() != 1 {
		t.Errorf("object namedChildCount = %d, want 1", obj.NamedChildCount())
	}
}

func TestNodeChildNavigation(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	obj := root.Child(0)

	lbrace := obj.Child(0)
	if lbrace.IsNull() {
		t.Fatal("lbrace should not be null")
	}
	if lbrace.Type() != "{" {
		t.Errorf("child 0 type = %q, want \"{\"", lbrace.Type())
	}
	if lbrace.StartByte() != 0 {
		t.Errorf("lbrace startByte = %d, want 0", lbrace.StartByte())
	}
	if lbrace.EndByte() != 1 {
		t.Errorf("lbrace endByte = %d, want 1", lbrace.EndByte())
	}

	pair := obj.Child(1)
	if pair.IsNull() {
		t.Fatal("pair should not be null")
	}
	if pair.Type() != "pair" {
		t.Errorf("child 1 type = %q, want \"pair\"", pair.Type())
	}
	if pair.StartByte() != 1 {
		t.Errorf("pair startByte = %d, want 1", pair.StartByte())
	}
	if pair.EndByte() != 6 {
		t.Errorf("pair endByte = %d, want 6", pair.EndByte())
	}

	rbrace := obj.Child(2)
	if rbrace.IsNull() {
		t.Fatal("rbrace should not be null")
	}
	if rbrace.Type() != "}" {
		t.Errorf("child 2 type = %q, want \"}\"", rbrace.Type())
	}
	if rbrace.StartByte() != 6 {
		t.Errorf("rbrace startByte = %d, want 6", rbrace.StartByte())
	}
}

func TestNodeNamedChild(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	obj := root.Child(0)

	// Named child 0 of object should be "pair".
	pair := obj.NamedChild(0)
	if pair.IsNull() {
		t.Fatal("named child 0 should not be null")
	}
	if pair.Type() != "pair" {
		t.Errorf("named child 0 type = %q, want \"pair\"", pair.Type())
	}

	// pair's named children: string (at index 0), number (at index 1).
	strChild := pair.NamedChild(0)
	if strChild.IsNull() {
		t.Fatal("pair named child 0 should not be null")
	}
	if strChild.Type() != "string" {
		t.Errorf("pair named child 0 type = %q, want \"string\"", strChild.Type())
	}

	numChild := pair.NamedChild(1)
	if numChild.IsNull() {
		t.Fatal("pair named child 1 should not be null")
	}
	if numChild.Type() != "number" {
		t.Errorf("pair named child 1 type = %q, want \"number\"", numChild.Type())
	}
}

func TestNodeChildByFieldName(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	obj := root.Child(0)
	pair := obj.NamedChild(0)

	// pair has field "key" -> child 0 (string) and "value" -> child 2 (number).
	key := pair.ChildByFieldName("key")
	if key.IsNull() {
		t.Fatal("field 'key' should not be null")
	}
	if key.Type() != "string" {
		t.Errorf("field 'key' type = %q, want \"string\"", key.Type())
	}

	val := pair.ChildByFieldName("value")
	if val.IsNull() {
		t.Fatal("field 'value' should not be null")
	}
	if val.Type() != "number" {
		t.Errorf("field 'value' type = %q, want \"number\"", val.Type())
	}

	// Non-existent field.
	none := pair.ChildByFieldName("nonexistent")
	if !none.IsNull() {
		t.Error("nonexistent field should be null")
	}
}

func TestNodeIsNamed(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	obj := root.Child(0)

	// "{" is not named.
	lbrace := obj.Child(0)
	if lbrace.IsNamed() {
		t.Error("'{' should not be named")
	}

	// pair is named.
	pair := obj.Child(1)
	if !pair.IsNamed() {
		t.Error("pair should be named")
	}
}

func TestNodeOutOfRange(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()

	// Child out of range.
	outOfRange := root.Child(100)
	if !outOfRange.IsNull() {
		t.Error("out of range child should be null")
	}

	negativeIdx := root.Child(-1)
	if !negativeIdx.IsNull() {
		t.Error("negative index child should be null")
	}

	namedOutOfRange := root.NamedChild(100)
	if !namedOutOfRange.IsNull() {
		t.Error("out of range named child should be null")
	}
}

func TestNodeEqual(t *testing.T) {
	tree, _ := buildTestTree()
	root1 := tree.RootNode()
	root2 := tree.RootNode()

	if !root1.Equal(root2) {
		t.Error("same root node should be equal")
	}

	obj1 := root1.Child(0)
	obj2 := root2.Child(0)
	if !obj1.Equal(obj2) {
		t.Error("same object node should be equal")
	}

	if root1.Equal(obj1) {
		t.Error("root and object should not be equal")
	}
}

func TestNullNode(t *testing.T) {
	var n Node
	if !n.IsNull() {
		t.Error("zero-value Node should be null")
	}
	if n.Type() != "" {
		t.Error("null node type should be empty")
	}
	if n.ChildCount() != 0 {
		t.Error("null node childCount should be 0")
	}
	if n.String() != "" {
		t.Error("null node String should be empty")
	}
}

func TestNodeParent(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	obj := root.Child(0)
	pair := obj.NamedChild(0)

	// pair's parent should be object.
	pairParent := pair.Parent()
	if pairParent.IsNull() {
		t.Fatal("pair parent should not be null")
	}
	if !pairParent.Equal(obj) {
		t.Errorf("pair parent type = %q, want \"object\"", pairParent.Type())
	}

	// object's parent should be document.
	objParent := obj.Parent()
	if objParent.IsNull() {
		t.Fatal("object parent should not be null")
	}
	if !objParent.Equal(root) {
		t.Errorf("object parent type = %q, want \"document\"", objParent.Type())
	}

	// root has no parent.
	rootParent := root.Parent()
	if !rootParent.IsNull() {
		t.Error("root parent should be null")
	}
}

func TestNodeNextPrevSibling(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	obj := root.Child(0)

	lbrace := obj.Child(0)
	pair := obj.Child(1)
	rbrace := obj.Child(2)

	// Next sibling of "{" is pair.
	next := lbrace.NextSibling()
	if next.IsNull() {
		t.Fatal("lbrace next sibling should not be null")
	}
	if !next.Equal(pair) {
		t.Errorf("lbrace next sibling = %q, want pair", next.Type())
	}

	// Prev sibling of "}" is pair.
	prev := rbrace.PrevSibling()
	if prev.IsNull() {
		t.Fatal("rbrace prev sibling should not be null")
	}
	if !prev.Equal(pair) {
		t.Errorf("rbrace prev sibling = %q, want pair", prev.Type())
	}

	// First child has no prev sibling.
	first := lbrace.PrevSibling()
	if !first.IsNull() {
		t.Error("first child should have no prev sibling")
	}

	// Last child has no next sibling.
	last := rbrace.NextSibling()
	if !last.IsNull() {
		t.Error("last child should have no next sibling")
	}
}

func TestNodeStringSExpression(t *testing.T) {
	tree, _ := buildTestTree()
	root := tree.RootNode()
	s := root.String()

	// Should produce something like: (document (object (pair (string) (number))))
	if s == "" {
		t.Fatal("S-expression should not be empty")
	}
	// Verify it starts with (document and contains key subtree names.
	if s[:10] != "(document " {
		t.Errorf("S-expression starts with %q, want \"(document \"", s[:10])
	}
	if !containsSubstring(s, "object") {
		t.Errorf("S-expression missing 'object': %s", s)
	}
	if !containsSubstring(s, "pair") {
		t.Errorf("S-expression missing 'pair': %s", s)
	}
	if !containsSubstring(s, "string") {
		t.Errorf("S-expression missing 'string': %s", s)
	}
	if !containsSubstring(s, "number") {
		t.Errorf("S-expression missing 'number': %s", s)
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestTreeLanguage(t *testing.T) {
	tree, _ := buildTestTree()
	if tree.Language() == nil {
		t.Fatal("tree language should not be nil")
	}
}
