package treesitter

import (
	"testing"
	"unsafe"
)

func TestSubtreeInlineCreation(t *testing.T) {
	s := newInlineSubtree(
		Symbol(42),
		StateID(100),
		Length{Bytes: 3, Point: Point{Row: 0, Column: 3}},
		Length{Bytes: 5, Point: Point{Row: 0, Column: 5}},
		true,  // visible
		true,  // named
		false, // extra
		false, // isKeyword
	)

	if !s.IsInline() {
		t.Fatal("expected inline subtree")
	}
	if s.IsZero() {
		t.Fatal("expected non-zero subtree")
	}

	if got := s.InlineSymbol(); got != 42 {
		t.Errorf("symbol = %d, want 42", got)
	}
	if got := s.InlineParseState(); got != 100 {
		t.Errorf("parseState = %d, want 100", got)
	}

	padding := s.InlinePadding()
	if padding.Bytes != 3 {
		t.Errorf("padding.Bytes = %d, want 3", padding.Bytes)
	}
	if padding.Point.Column != 3 {
		t.Errorf("padding.Point.Column = %d, want 3", padding.Point.Column)
	}
	if padding.Point.Row != 0 {
		t.Errorf("padding.Point.Row = %d, want 0", padding.Point.Row)
	}

	size := s.InlineSize()
	if size.Bytes != 5 {
		t.Errorf("size.Bytes = %d, want 5", size.Bytes)
	}
	if size.Point.Column != 5 {
		t.Errorf("size.Point.Column = %d, want 5", size.Point.Column)
	}
	if size.Point.Row != 0 {
		t.Errorf("size.Point.Row = %d, want 0", size.Point.Row)
	}

	if !s.InlineVisible() {
		t.Error("expected visible")
	}
	if !s.InlineNamed() {
		t.Error("expected named")
	}
	if s.InlineExtra() {
		t.Error("expected not extra")
	}
	if s.InlineIsKeyword() {
		t.Error("expected not keyword")
	}
}

func TestSubtreeInlineFlags(t *testing.T) {
	// Test with extra=true, keyword=true, named=false
	s := newInlineSubtree(
		Symbol(10),
		StateID(0),
		Length{},
		Length{Bytes: 1, Point: Point{Column: 1}},
		true,  // visible
		false, // named
		true,  // extra
		true,  // isKeyword
	)

	if s.InlineNamed() {
		t.Error("expected not named")
	}
	if !s.InlineExtra() {
		t.Error("expected extra")
	}
	if !s.InlineIsKeyword() {
		t.Error("expected keyword")
	}
	if !s.InlineVisible() {
		t.Error("expected visible")
	}
}

func TestSubtreeInlineMaxValues(t *testing.T) {
	// Test with maximum values that fit inline.
	s := newInlineSubtree(
		Symbol(255),
		StateID(65535),
		Length{Bytes: 255, Point: Point{Column: 255}},
		Length{Bytes: 255, Point: Point{Column: 255}},
		true, true, true, true,
	)

	if got := s.InlineSymbol(); got != 255 {
		t.Errorf("symbol = %d, want 255", got)
	}
	if got := s.InlineParseState(); got != 65535 {
		t.Errorf("parseState = %d, want 65535", got)
	}
	if got := s.InlinePadding(); got.Bytes != 255 || got.Point.Column != 255 {
		t.Errorf("padding = %+v, want bytes=255 col=255", got)
	}
	if got := s.InlineSize(); got.Bytes != 255 || got.Point.Column != 255 {
		t.Errorf("size = %+v, want bytes=255 col=255", got)
	}
}

func TestSubtreeCanInline(t *testing.T) {
	tests := []struct {
		name     string
		padding  Length
		size     Length
		symbol   Symbol
		external bool
		want     bool
	}{
		{
			name:    "fits inline",
			padding: Length{Bytes: 10, Point: Point{Column: 10}},
			size:    Length{Bytes: 5, Point: Point{Column: 5}},
			symbol:  100,
			want:    true,
		},
		{
			name:    "symbol too large",
			padding: Length{},
			size:    Length{Bytes: 1, Point: Point{Column: 1}},
			symbol:  256,
			want:    false,
		},
		{
			name:     "has external tokens",
			padding:  Length{},
			size:     Length{Bytes: 1, Point: Point{Column: 1}},
			symbol:   1,
			external: true,
			want:     false,
		},
		{
			name:    "padding has row",
			padding: Length{Bytes: 10, Point: Point{Row: 1, Column: 5}},
			size:    Length{Bytes: 5, Point: Point{Column: 5}},
			symbol:  1,
			want:    false,
		},
		{
			name:    "padding bytes too large",
			padding: Length{Bytes: 256},
			size:    Length{Bytes: 1, Point: Point{Column: 1}},
			symbol:  1,
			want:    false,
		},
		{
			name:    "size has row (multi-line)",
			padding: Length{},
			size:    Length{Bytes: 100, Point: Point{Row: 1, Column: 5}},
			symbol:  1,
			want:    false,
		},
		{
			name:    "size bytes too large",
			padding: Length{},
			size:    Length{Bytes: 256, Point: Point{Column: 5}},
			symbol:  1,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := subtreeCanInline(tt.padding, tt.size, tt.symbol, tt.external)
			if got != tt.want {
				t.Errorf("subtreeCanInline = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubtreeArenaAlloc(t *testing.T) {
	arena := NewSubtreeArena(4) // Small block size for testing growth.

	if arena.BlockCount() != 1 {
		t.Fatalf("initial blocks = %d, want 1", arena.BlockCount())
	}
	if arena.TotalAllocated() != 0 {
		t.Fatalf("initial allocated = %d, want 0", arena.TotalAllocated())
	}

	// Allocate 4 items (fills first block).
	refs := make([]Subtree, 0, 4)
	for i := 0; i < 4; i++ {
		st, data := arena.Alloc()
		data.Symbol = Symbol(i + 1)
		data.ParseState = StateID(i * 10)
		refs = append(refs, st)
	}

	if arena.TotalAllocated() != 4 {
		t.Fatalf("allocated = %d, want 4", arena.TotalAllocated())
	}
	if arena.BlockCount() != 1 {
		t.Fatalf("blocks = %d, want 1", arena.BlockCount())
	}

	// Verify data through Get.
	for i, ref := range refs {
		if ref.IsInline() {
			t.Fatalf("ref[%d] is inline, expected arena ref", i)
		}
		data := arena.Get(ref)
		if data.Symbol != Symbol(i+1) {
			t.Errorf("ref[%d] symbol = %d, want %d", i, data.Symbol, i+1)
		}
		if data.ParseState != StateID(i*10) {
			t.Errorf("ref[%d] parseState = %d, want %d", i, data.ParseState, i*10)
		}
	}

	// Allocate one more — should trigger new block.
	st5, data5 := arena.Alloc()
	data5.Symbol = Symbol(99)

	if arena.BlockCount() != 2 {
		t.Fatalf("blocks after overflow = %d, want 2", arena.BlockCount())
	}
	if arena.TotalAllocated() != 5 {
		t.Fatalf("allocated after overflow = %d, want 5", arena.TotalAllocated())
	}

	// Verify the 5th item.
	got5 := arena.Get(st5)
	if got5.Symbol != 99 {
		t.Errorf("5th symbol = %d, want 99", got5.Symbol)
	}

	// Verify old refs still work after new block.
	for i, ref := range refs {
		data := arena.Get(ref)
		if data.Symbol != Symbol(i+1) {
			t.Errorf("after overflow, ref[%d] symbol = %d, want %d", i, data.Symbol, i+1)
		}
	}
}

func TestSubtreeArenaGrowth(t *testing.T) {
	arena := NewSubtreeArena(8)

	// Allocate 100 items.
	for i := 0; i < 100; i++ {
		st, data := arena.Alloc()
		data.Symbol = Symbol(i)
		// Verify immediately.
		got := arena.Get(st)
		if got.Symbol != Symbol(i) {
			t.Fatalf("i=%d: got symbol=%d, want %d", i, got.Symbol, i)
		}
	}

	if arena.TotalAllocated() != 100 {
		t.Errorf("total allocated = %d, want 100", arena.TotalAllocated())
	}
	// 100 / 8 = 12 full blocks + 1 partial = 13 blocks.
	expectedBlocks := (100 + 7) / 8 // ceil(100/8) = 13
	if arena.BlockCount() != expectedBlocks {
		t.Errorf("blocks = %d, want %d", arena.BlockCount(), expectedBlocks)
	}
}

func TestSubtreeInlineVsHeapDiscrimination(t *testing.T) {
	arena := NewSubtreeArena(16)

	// Create an inline subtree.
	inl := newInlineSubtree(Symbol(5), StateID(10), Length{Bytes: 1, Point: Point{Column: 1}}, Length{Bytes: 2, Point: Point{Column: 2}}, true, true, false, false)

	// Create a heap subtree.
	heap, data := arena.Alloc()
	data.Symbol = Symbol(5)
	data.ParseState = StateID(10)
	data.Padding = Length{Bytes: 1, Point: Point{Column: 1}}
	data.Size = Length{Bytes: 2, Point: Point{Column: 2}}

	// Inline check.
	if !inl.IsInline() {
		t.Error("inline subtree should be inline")
	}
	if heap.IsInline() {
		t.Error("heap subtree should not be inline")
	}

	// Both should have the same logical data.
	if GetSymbol(inl, arena) != GetSymbol(heap, arena) {
		t.Errorf("symbols differ: inline=%d heap=%d", GetSymbol(inl, arena), GetSymbol(heap, arena))
	}
	if GetParseState(inl, arena) != GetParseState(heap, arena) {
		t.Errorf("parseStates differ: inline=%d heap=%d", GetParseState(inl, arena), GetParseState(heap, arena))
	}
	if GetPadding(inl, arena) != GetPadding(heap, arena) {
		t.Errorf("padding differs: inline=%v heap=%v", GetPadding(inl, arena), GetPadding(heap, arena))
	}
	if GetSize(inl, arena) != GetSize(heap, arena) {
		t.Errorf("size differs: inline=%v heap=%v", GetSize(inl, arena), GetSize(heap, arena))
	}

	// Child count: inline always 0, heap also 0 (leaf).
	if GetChildCount(inl, arena) != 0 {
		t.Error("inline child count should be 0")
	}
	if GetChildCount(heap, arena) != 0 {
		t.Error("heap leaf child count should be 0")
	}
}

func TestSubtreeIDEquality(t *testing.T) {
	arena := NewSubtreeArena(16)

	st1, _ := arena.Alloc()
	st2, _ := arena.Alloc()

	id1 := SubtreeIDOf(st1)
	id2 := SubtreeIDOf(st2)
	id1_copy := SubtreeIDOf(st1)

	if !id1.Equal(id1_copy) {
		t.Error("same arena ref should be equal")
	}
	if id1.Equal(id2) {
		t.Error("different arena refs should not be equal")
	}

	// Inline subtrees: same data = same ID, different data = different ID.
	inl1 := newInlineSubtree(Symbol(1), StateID(0), Length{}, Length{Bytes: 1, Point: Point{Column: 1}}, true, false, false, false)
	inl2 := newInlineSubtree(Symbol(2), StateID(0), Length{}, Length{Bytes: 1, Point: Point{Column: 1}}, true, false, false, false)
	inl1_dup := newInlineSubtree(Symbol(1), StateID(0), Length{}, Length{Bytes: 1, Point: Point{Column: 1}}, true, false, false, false)

	idInl1 := SubtreeIDOf(inl1)
	idInl2 := SubtreeIDOf(inl2)
	idInl1Dup := SubtreeIDOf(inl1_dup)

	if !idInl1.Equal(idInl1Dup) {
		t.Error("same inline data should be equal")
	}
	if idInl1.Equal(idInl2) {
		t.Error("different inline data should not be equal")
	}

	// Inline vs arena should never be equal.
	if idInl1.Equal(id1) {
		t.Error("inline and arena IDs should not be equal")
	}
}

func TestSubtreeHeapDataFlags(t *testing.T) {
	d := &SubtreeHeapData{}

	// All flags should be off initially.
	for _, f := range []SubtreeFlags{
		SubtreeFlagVisible, SubtreeFlagNamed, SubtreeFlagExtra,
		SubtreeFlagHasChanges, SubtreeFlagMissing,
		SubtreeFlagFragileLeft, SubtreeFlagFragileRight,
		SubtreeFlagHasExternalTokens, SubtreeFlagDependsOnColumn,
		SubtreeFlagIsKeyword,
	} {
		if d.HasFlag(f) {
			t.Errorf("flag %d should be off initially", f)
		}
	}

	// Set some flags.
	d.SetFlag(SubtreeFlagVisible, true)
	d.SetFlag(SubtreeFlagFragileLeft, true)
	d.SetFlag(SubtreeFlagIsKeyword, true)

	if !d.HasFlag(SubtreeFlagVisible) {
		t.Error("visible should be set")
	}
	if !d.HasFlag(SubtreeFlagFragileLeft) {
		t.Error("fragileLeft should be set")
	}
	if !d.HasFlag(SubtreeFlagIsKeyword) {
		t.Error("isKeyword should be set")
	}
	if d.HasFlag(SubtreeFlagNamed) {
		t.Error("named should not be set")
	}

	// Clear a flag.
	d.SetFlag(SubtreeFlagVisible, false)
	if d.HasFlag(SubtreeFlagVisible) {
		t.Error("visible should be cleared")
	}
	if !d.HasFlag(SubtreeFlagFragileLeft) {
		t.Error("fragileLeft should still be set after clearing visible")
	}
}

func TestSubtreeNewLeaf(t *testing.T) {
	arena := NewSubtreeArena(16)

	// Create a minimal language for testing.
	lang := &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: true, Named: false},  // 1: "{"
			{Visible: true, Named: true},   // 2: object
		},
	}

	// Leaf that fits inline.
	s := NewLeafSubtree(arena, Symbol(1), Length{Bytes: 5, Point: Point{Column: 5}}, Length{Bytes: 1, Point: Point{Column: 1}}, StateID(3), false, false, false, lang)
	if !s.IsInline() {
		t.Fatal("expected inline for small leaf")
	}
	if GetSymbol(s, arena) != 1 {
		t.Errorf("symbol = %d, want 1", GetSymbol(s, arena))
	}
	if !IsVisible(s, arena) {
		t.Error("expected visible")
	}
	if IsNamed(s, arena) {
		t.Error("expected not named")
	}

	// Leaf that doesn't fit inline (symbol > 255).
	// Extend metadata to cover symbol 300.
	for len(lang.SymbolMetadata) <= 300 {
		lang.SymbolMetadata = append(lang.SymbolMetadata, SymbolMetadata{})
	}
	lang.SymbolMetadata[300] = SymbolMetadata{Visible: true, Named: true}
	sHeap := NewLeafSubtree(arena, Symbol(300), Length{}, Length{Bytes: 10, Point: Point{Column: 10}}, StateID(5), false, false, false, lang)
	if sHeap.IsInline() {
		t.Fatal("expected heap for large symbol")
	}
	if GetSymbol(sHeap, arena) != 300 {
		t.Errorf("symbol = %d, want 300", GetSymbol(sHeap, arena))
	}

	// Leaf with external tokens (never inline).
	sExt := NewLeafSubtree(arena, Symbol(2), Length{}, Length{Bytes: 1, Point: Point{Column: 1}}, StateID(0), true, false, false, lang)
	if sExt.IsInline() {
		t.Fatal("expected heap for external token leaf")
	}
	data := arena.Get(sExt)
	if !data.HasFlag(SubtreeFlagHasExternalTokens) {
		t.Error("expected HasExternalTokens flag")
	}
}

func TestSubtreeSize(t *testing.T) {
	// Verify Subtree is exactly 8 bytes.
	var s Subtree
	if size := unsafe.Sizeof(s); size != 8 {
		t.Errorf("Subtree size = %d bytes, want 8", size)
	}
}

// makeSubtreeTestLanguage creates a minimal Language for testing.
func makeSubtreeTestLanguage() *Language {
	return &Language{
		SymbolMetadata: []SymbolMetadata{
			{Visible: false, Named: false}, // 0: end
			{Visible: true, Named: false},  // 1: "{"
			{Visible: true, Named: false},  // 2: "}"
			{Visible: true, Named: true},   // 3: object
			{Visible: true, Named: true},   // 4: pair
			{Visible: true, Named: true},   // 5: string
			{Visible: true, Named: false},  // 6: ":"
			{Visible: true, Named: true},   // 7: number
			{Visible: true, Named: true},   // 8: document
			{Visible: false, Named: false}, // 9: _value (hidden)
			{Visible: true, Named: false},  // 10: ","
			{Visible: true, Named: true},   // 11: comment (extra)
		},
		SymbolNames: []string{
			"end", "{", "}", "object", "pair", "string",
			":", "number", "document", "_value", ",", "comment",
		},
		FieldNames: []string{
			"",      // 0: no field
			"key",   // 1
			"value", // 2
		},
		FieldMapSlices: []FieldMapSlice{
			{},                    // prodID 0: no fields
			{Index: 0, Length: 2}, // prodID 1: pair -> key:0 value:2
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0}, // key -> child 0
			{FieldID: 2, ChildIndex: 2}, // value -> child 2
		},
	}
}

func TestNewNodeSubtree(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Create leaf children: { and }
	lbrace := NewLeafSubtree(arena, Symbol(1), Length{Bytes: 0, Point: Point{Column: 0}}, Length{Bytes: 1, Point: Point{Column: 1}}, StateID(1), false, false, false, lang)
	rbrace := NewLeafSubtree(arena, Symbol(2), Length{Bytes: 0, Point: Point{Column: 0}}, Length{Bytes: 1, Point: Point{Column: 1}}, StateID(2), false, false, false, lang)

	// Create an internal node: object -> { }
	children := []Subtree{lbrace, rbrace}
	obj := NewNodeSubtree(arena, Symbol(3), children, 0, lang)

	if obj.IsInline() {
		t.Fatal("internal node should not be inline")
	}
	if GetSymbol(obj, arena) != 3 {
		t.Errorf("symbol = %d, want 3", GetSymbol(obj, arena))
	}
	if GetChildCount(obj, arena) != 2 {
		t.Errorf("childCount = %d, want 2", GetChildCount(obj, arena))
	}
	if !IsVisible(obj, arena) {
		t.Error("object should be visible")
	}
	if !IsNamed(obj, arena) {
		t.Error("object should be named")
	}

	// Verify children are accessible.
	gotChildren := GetChildren(obj, arena)
	if len(gotChildren) != 2 {
		t.Fatalf("children len = %d, want 2", len(gotChildren))
	}
	if GetSymbol(gotChildren[0], arena) != 1 {
		t.Errorf("child[0] symbol = %d, want 1", GetSymbol(gotChildren[0], arena))
	}
	if GetSymbol(gotChildren[1], arena) != 2 {
		t.Errorf("child[1] symbol = %d, want 2", GetSymbol(gotChildren[1], arena))
	}
}

func TestSummarizeChildrenEmpty(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Create an internal node with no children.
	node := NewNodeSubtree(arena, Symbol(3), nil, 0, lang)
	SummarizeChildren(node, arena, lang)

	if GetPadding(node, arena) != LengthZero {
		t.Errorf("padding = %+v, want zero", GetPadding(node, arena))
	}
	if GetSize(node, arena) != LengthZero {
		t.Errorf("size = %+v, want zero", GetSize(node, arena))
	}
}

func TestSummarizeChildrenSimple(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Build: object -> "{" "}"
	// "{" at byte 0, size 1
	// "}" at byte 1, size 1 (padding=0 after "{")
	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)

	children := []Subtree{lbrace, rbrace}
	obj := NewNodeSubtree(arena, Symbol(3), children, 0, lang)
	SummarizeChildren(obj, arena, lang)

	data := arena.Get(obj)

	// Padding should be first child's padding (0).
	if data.Padding != (Length{Bytes: 0, Point: Point{Column: 0}}) {
		t.Errorf("padding = %+v, want zero", data.Padding)
	}

	// Size should span both children: 1 + 0 + 1 = 2 bytes.
	if data.Size.Bytes != 2 {
		t.Errorf("size.Bytes = %d, want 2", data.Size.Bytes)
	}
	if data.Size.Point.Column != 2 {
		t.Errorf("size.Point.Column = %d, want 2", data.Size.Point.Column)
	}

	// Both children are visible and non-extra, non-named.
	if data.VisibleChildCount != 2 {
		t.Errorf("visibleChildCount = %d, want 2", data.VisibleChildCount)
	}
	// "{" and "}" are visible but not named.
	if data.NamedChildCount != 0 {
		t.Errorf("namedChildCount = %d, want 0", data.NamedChildCount)
	}
	// Each visible child contributes 1 to visible descendants.
	if data.VisibleDescendantCount != 2 {
		t.Errorf("visibleDescendantCount = %d, want 2", data.VisibleDescendantCount)
	}

	// FirstLeaf should come from the first child.
	if data.FirstLeaf.Symbol != 1 {
		t.Errorf("firstLeaf.Symbol = %d, want 1", data.FirstLeaf.Symbol)
	}
	if data.FirstLeaf.ParseState != 1 {
		t.Errorf("firstLeaf.ParseState = %d, want 1", data.FirstLeaf.ParseState)
	}
}

func TestSummarizeChildrenWithPadding(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Build: object -> "{" "}"
	// "{" at byte 2 (2 bytes padding), size 1
	// "}" at byte 4 (1 byte padding after "{"), size 1
	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 2, Point: Point{Column: 2}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 1, Point: Point{Column: 1}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)

	children := []Subtree{lbrace, rbrace}
	obj := NewNodeSubtree(arena, Symbol(3), children, 0, lang)
	SummarizeChildren(obj, arena, lang)

	data := arena.Get(obj)

	// Node's padding = first child's padding.
	if data.Padding.Bytes != 2 {
		t.Errorf("padding.Bytes = %d, want 2", data.Padding.Bytes)
	}

	// Node's size = first child size + second child (padding + size) = 1 + 1 + 1 = 3.
	if data.Size.Bytes != 3 {
		t.Errorf("size.Bytes = %d, want 3", data.Size.Bytes)
	}
}

func TestSummarizeChildrenMultiline(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Build a node whose children span multiple lines:
	// "{" at (0,0) size 1
	// "\n" implicit via padding
	// "}" at (1,0) padding crosses newline, size 1
	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Row: 0, Column: 0}},
		Length{Bytes: 1, Point: Point{Row: 0, Column: 1}},
		StateID(1), false, false, false, lang)
	// "}" has padding that spans a newline: 1 byte (\n), row+1, col back to 0.
	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 1, Point: Point{Row: 1, Column: 0}},
		Length{Bytes: 1, Point: Point{Row: 0, Column: 1}},
		StateID(2), false, false, false, lang)

	children := []Subtree{lbrace, rbrace}
	obj := NewNodeSubtree(arena, Symbol(3), children, 0, lang)
	SummarizeChildren(obj, arena, lang)

	data := arena.Get(obj)

	// Total size = child0.size(1b, col1) + child1.padding(1b, row1 col0) + child1.size(1b, col1)
	// = 3 bytes, row 1, column 1
	if data.Size.Bytes != 3 {
		t.Errorf("size.Bytes = %d, want 3", data.Size.Bytes)
	}
	if data.Size.Point.Row != 1 {
		t.Errorf("size.Point.Row = %d, want 1", data.Size.Point.Row)
	}
	if data.Size.Point.Column != 1 {
		t.Errorf("size.Point.Column = %d, want 1", data.Size.Point.Column)
	}
}

func TestSummarizeChildrenNestedVisibleDescendants(t *testing.T) {
	arena := NewSubtreeArena(64)
	lang := makeSubtreeTestLanguage()

	// Build: object -> "{" pair "}"
	// pair -> string ":" number
	// This tests that visible descendant counting works through nesting.

	// Leaf tokens
	lbrace := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	strKey := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(3), false, false, false, lang)

	colon := NewLeafSubtree(arena, Symbol(6),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(4), false, false, false, lang)

	numVal := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(5), false, false, false, lang)

	rbrace := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(6), false, false, false, lang)

	// Build pair -> string ":" number
	pairChildren := []Subtree{strKey, colon, numVal}
	pair := NewNodeSubtree(arena, Symbol(4), pairChildren, 1, lang)
	SummarizeChildren(pair, arena, lang)

	pairData := arena.Get(pair)
	// pair has 3 children: string(visible,named), ":"(visible), number(visible,named)
	if pairData.VisibleChildCount != 3 {
		t.Errorf("pair.visibleChildCount = %d, want 3", pairData.VisibleChildCount)
	}
	if pairData.NamedChildCount != 2 {
		t.Errorf("pair.namedChildCount = %d, want 2", pairData.NamedChildCount)
	}
	// Visible descendants: 3 (the three visible children, none of which have visible descendants of their own)
	if pairData.VisibleDescendantCount != 3 {
		t.Errorf("pair.visibleDescendantCount = %d, want 3", pairData.VisibleDescendantCount)
	}

	// Build object -> "{" pair "}"
	objChildren := []Subtree{lbrace, pair, rbrace}
	obj := NewNodeSubtree(arena, Symbol(3), objChildren, 0, lang)
	SummarizeChildren(obj, arena, lang)

	objData := arena.Get(obj)
	// object has 3 children: "{"(visible), pair(visible,named), "}"(visible)
	if objData.VisibleChildCount != 3 {
		t.Errorf("obj.visibleChildCount = %d, want 3", objData.VisibleChildCount)
	}
	if objData.NamedChildCount != 1 {
		t.Errorf("obj.namedChildCount = %d, want 1 (pair)", objData.NamedChildCount)
	}
	// Visible descendants: 3 (direct) + pair's 3 visible descendants = 6
	if objData.VisibleDescendantCount != 6 {
		t.Errorf("obj.visibleDescendantCount = %d, want 6", objData.VisibleDescendantCount)
	}

	// pair size should be 3 + 1 + 1 = 5 bytes
	if pairData.Size.Bytes != 5 {
		t.Errorf("pair.size.Bytes = %d, want 5", pairData.Size.Bytes)
	}
}

func TestSummarizeChildrenRepeatDepth(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Simulate left-recursive repetition: object -> object ","  number
	// The inner object also has symbol 3, so repeat depth should increment.

	inner := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)

	// Inner "object" node wrapping a single number.
	innerObj := NewNodeSubtree(arena, Symbol(3), []Subtree{inner}, 0, lang)
	SummarizeChildren(innerObj, arena, lang)

	comma := NewLeafSubtree(arena, Symbol(10),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)

	num := NewLeafSubtree(arena, Symbol(7),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(3), false, false, false, lang)

	// Outer "object" node: object -> object "," number
	outerObj := NewNodeSubtree(arena, Symbol(3), []Subtree{innerObj, comma, num}, 0, lang)
	SummarizeChildren(outerObj, arena, lang)

	outerData := arena.Get(outerObj)
	// First child (innerObj) has same symbol (3), so repeat depth = innerObj's depth + 1.
	// innerObj's first child is a number (sym 7), so innerObj's repeat depth = 0.
	// outerObj's repeat depth should be 1.
	if outerData.RepeatDepth != 1 {
		t.Errorf("repeatDepth = %d, want 1", outerData.RepeatDepth)
	}
}

func TestSummarizeChildrenHiddenChild(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// "_value" (symbol 9) is hidden (Visible: false).
	// When counting visible children, hidden children should not be counted.
	value := NewLeafSubtree(arena, Symbol(9),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)

	parent := NewNodeSubtree(arena, Symbol(8), []Subtree{value}, 0, lang)
	SummarizeChildren(parent, arena, lang)

	data := arena.Get(parent)
	// _value is not visible, so it should not be counted.
	if data.VisibleChildCount != 0 {
		t.Errorf("visibleChildCount = %d, want 0", data.VisibleChildCount)
	}
}

func TestSummarizeChildrenFirstLeaf(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()

	// Build a nested tree and verify FirstLeaf propagates from leftmost leaf.
	leaf := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 1, Point: Point{Column: 1}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(42), false, false, false, lang)

	innerNode := NewNodeSubtree(arena, Symbol(4), []Subtree{leaf}, 0, lang)
	SummarizeChildren(innerNode, arena, lang)

	outerNode := NewNodeSubtree(arena, Symbol(3), []Subtree{innerNode}, 0, lang)
	SummarizeChildren(outerNode, arena, lang)

	outerData := arena.Get(outerNode)
	if outerData.FirstLeaf.Symbol != 5 {
		t.Errorf("firstLeaf.Symbol = %d, want 5", outerData.FirstLeaf.Symbol)
	}
	if outerData.FirstLeaf.ParseState != 42 {
		t.Errorf("firstLeaf.ParseState = %d, want 42", outerData.FirstLeaf.ParseState)
	}
}

func TestSubtreeAccessorsMissing(t *testing.T) {
	arena := NewSubtreeArena(16)

	st, data := arena.Alloc()
	data.Symbol = Symbol(5)
	data.SetFlag(SubtreeFlagMissing, true)

	if !IsMissing(st, arena) {
		t.Error("expected missing")
	}

	// Inline subtrees are never missing.
	inl := newInlineSubtree(Symbol(5), StateID(0), Length{}, Length{Bytes: 1, Point: Point{Column: 1}}, true, false, false, false)
	if IsMissing(inl, arena) {
		t.Error("inline should not be missing")
	}
}

func TestSubtreeAccessorsFragile(t *testing.T) {
	arena := NewSubtreeArena(16)

	st, data := arena.Alloc()
	data.SetFlag(SubtreeFlagFragileLeft, true)
	data.SetFlag(SubtreeFlagFragileRight, true)

	if !IsFragileLeft(st, arena) {
		t.Error("expected fragile left")
	}
	if !IsFragileRight(st, arena) {
		t.Error("expected fragile right")
	}

	// Inline subtrees are never fragile.
	inl := newInlineSubtree(Symbol(1), StateID(0), Length{}, Length{Bytes: 1, Point: Point{Column: 1}}, true, false, false, false)
	if IsFragileLeft(inl, arena) {
		t.Error("inline should not be fragile left")
	}
	if IsFragileRight(inl, arena) {
		t.Error("inline should not be fragile right")
	}
}
