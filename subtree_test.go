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
