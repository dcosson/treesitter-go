package stack

import (
	ts "github.com/dcosson/treesitter-go"
	"testing"
)

func testLang() *ts.Language {
	return &ts.Language{
		SymbolMetadata: []ts.SymbolMetadata{
			{Visible: false, Named: false},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
			{Visible: true, Named: true},
		},
	}
}

func makeLeaf(arena *SubtreeArena, sym Symbol, state StateID, size uint32) Subtree {
	return ts.NewLeafSubtree(
		arena,
		sym,
		Length{Bytes: 0, Point: ts.Point{Column: 0}},
		Length{Bytes: size, Point: ts.Point{Column: size}},
		state,
		false,
		false,
		false,
		testLang(),
	)
}

func TestPopCountSlicesGroupsSameBaseNode(t *testing.T) {
	arena := ts.NewSubtreeArena(64)
	stack := NewStack(arena)

	v0 := stack.AddVersion(1, Length{Bytes: 0})
	stack.Push(v0, 2, makeLeaf(arena, 1, 1, 1), false, Length{Bytes: 1})

	v1 := stack.Split(v0)
	stack.Push(v0, 3, makeLeaf(arena, 2, 2, 1), false, Length{Bytes: 2})
	stack.Push(v1, 3, makeLeaf(arena, 3, 2, 1), false, Length{Bytes: 2})
	if !stack.Merge(v0, v1) {
		t.Fatal("expected merge to succeed")
	}

	initialVersions := stack.VersionCount()
	slices := stack.PopCountSlices(v0, 1)
	if len(slices) != 2 {
		t.Fatalf("expected 2 slices, got %d", len(slices))
	}
	if slices[0].Version() != slices[1].Version() {
		t.Fatalf("expected slices to share version for same base node, got %d and %d", slices[0].Version(), slices[1].Version())
	}
	if stack.VersionCount() != initialVersions+1 {
		t.Fatalf("expected one new version for shared base node, got delta=%d", stack.VersionCount()-initialVersions)
	}
}

func TestPopCountSlicesCreatesVersionPerDistinctBaseNode(t *testing.T) {
	arena := ts.NewSubtreeArena(64)
	stack := NewStack(arena)

	v0 := stack.AddVersion(1, Length{Bytes: 0})
	stack.Push(v0, 2, makeLeaf(arena, 1, 1, 1), false, Length{Bytes: 1})

	v1 := stack.Split(v0)
	// Diverge base nodes before creating mergeable top nodes.
	stack.Push(v1, 4, makeLeaf(arena, 2, 2, 1), false, Length{Bytes: 2})

	stack.Push(v0, 3, makeLeaf(arena, 3, 2, 1), false, Length{Bytes: 3})
	stack.Push(v1, 3, makeLeaf(arena, 1, 4, 1), false, Length{Bytes: 3})
	if !stack.Merge(v0, v1) {
		t.Fatal("expected merge to succeed")
	}

	initialVersions := stack.VersionCount()
	slices := stack.PopCountSlices(v0, 1)
	if len(slices) != 2 {
		t.Fatalf("expected 2 slices, got %d", len(slices))
	}
	if slices[0].Version() == slices[1].Version() {
		t.Fatalf("expected distinct versions for distinct base nodes, got %d", slices[0].Version())
	}
	if stack.VersionCount() != initialVersions+2 {
		t.Fatalf("expected two new versions for distinct base nodes, got delta=%d", stack.VersionCount()-initialVersions)
	}
}
