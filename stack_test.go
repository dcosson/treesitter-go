package treesitter

import "testing"

func TestStackNewAndEmpty(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	if stack.VersionCount() != 0 {
		t.Errorf("version count = %d, want 0", stack.VersionCount())
	}
	if stack.ActiveVersionCount() != 0 {
		t.Errorf("active version count = %d, want 0", stack.ActiveVersionCount())
	}
}

func TestStackAddVersion(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	if v0 != 0 {
		t.Errorf("version = %d, want 0", v0)
	}
	if stack.VersionCount() != 1 {
		t.Errorf("version count = %d, want 1", stack.VersionCount())
	}
	if stack.State(v0) != 1 {
		t.Errorf("state = %d, want 1", stack.State(v0))
	}
	if stack.Position(v0).Bytes != 0 {
		t.Errorf("position = %d, want 0", stack.Position(v0).Bytes)
	}
	if !stack.IsActive(v0) {
		t.Error("version should be active")
	}
}

func TestStackPush(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Push a leaf subtree (simulating a shift).
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)

	stack.Push(v0, StateID(5), leaf, false, Length{Bytes: 1, Point: Point{Column: 1}})

	if stack.State(v0) != 5 {
		t.Errorf("state after push = %d, want 5", stack.State(v0))
	}
	if stack.Position(v0).Bytes != 1 {
		t.Errorf("position after push = %d, want 1", stack.Position(v0).Bytes)
	}
	if stack.NodeCount(v0) != 2 {
		t.Errorf("nodeCount = %d, want 2", stack.NodeCount(v0))
	}
}

func TestStackPop(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Push two states.
	leaf1 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(2), leaf1, false, Length{Bytes: 1})

	leaf2 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)
	stack.Push(v0, StateID(3), leaf2, false, Length{Bytes: 2})

	// Pop 2 items.
	results := stack.Pop(v0, 2)
	if len(results) != 1 {
		t.Fatalf("pop results = %d, want 1", len(results))
	}

	result := results[0]
	if len(result.subtrees) != 2 {
		t.Fatalf("subtree count = %d, want 2", len(result.subtrees))
	}
	if result.depth != 2 {
		t.Errorf("depth = %d, want 2", result.depth)
	}

	// The bottom node should be the initial state.
	if result.node.state != 1 {
		t.Errorf("bottom state = %d, want 1", result.node.state)
	}

	// After pop, the head should point to the bottom node.
	if stack.State(v0) != 1 {
		t.Errorf("head state after pop = %d, want 1", stack.State(v0))
	}
}

func TestStackSplit(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(2), leaf, false, Length{Bytes: 1})

	// Split.
	v1 := stack.Split(v0)

	if stack.VersionCount() != 2 {
		t.Errorf("version count = %d, want 2", stack.VersionCount())
	}
	if stack.State(v0) != stack.State(v1) {
		t.Errorf("states differ: v0=%d v1=%d", stack.State(v0), stack.State(v1))
	}

	// Push different states on each version.
	leaf2 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)
	stack.Push(v0, StateID(10), leaf2, false, Length{Bytes: 2})

	leaf3 := NewLeafSubtree(arena, Symbol(3),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(3), false, false, false, lang)
	stack.Push(v1, StateID(20), leaf3, false, Length{Bytes: 2})

	if stack.State(v0) != 10 {
		t.Errorf("v0 state = %d, want 10", stack.State(v0))
	}
	if stack.State(v1) != 20 {
		t.Errorf("v1 state = %d, want 20", stack.State(v1))
	}
}

func TestStackMerge(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	// Create two versions that converge to the same state.
	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})
	v1 := stack.AddVersion(StateID(2), Length{Bytes: 0})

	leaf0 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(5), leaf0, false, Length{Bytes: 1})

	leaf1 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)
	stack.Push(v1, StateID(5), leaf1, false, Length{Bytes: 1})

	// Both at state 5 — can merge.
	if !stack.CanMerge(v0, v1) {
		t.Fatal("versions at same state should be mergeable")
	}

	ok := stack.Merge(v0, v1)
	if !ok {
		t.Fatal("merge should succeed")
	}

	// Source version should be halted.
	if !stack.IsHalted(v1) {
		t.Error("source version should be halted after merge")
	}

	// Pop from merged version should produce 2 paths.
	count := stack.PopCount(v0, 1)
	if count != 2 {
		t.Errorf("pop count = %d, want 2 (merged paths)", count)
	}

	results := stack.Pop(v0, 1)
	if len(results) != 2 {
		t.Fatalf("pop results = %d, want 2", len(results))
	}
}

func TestStackMergeDifferentStates(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})
	v1 := stack.AddVersion(StateID(2), Length{Bytes: 0})

	// Different states — cannot merge.
	if stack.CanMerge(v0, v1) {
		t.Error("different states should not be mergeable")
	}

	ok := stack.Merge(v0, v1)
	if ok {
		t.Error("merge of different states should fail")
	}
}

func TestStackPauseResume(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	if !stack.IsActive(v0) {
		t.Error("should be active initially")
	}

	stack.Pause(v0)
	if !stack.IsPaused(v0) {
		t.Error("should be paused")
	}
	if stack.IsActive(v0) {
		t.Error("should not be active when paused")
	}

	stack.Resume(v0)
	if !stack.IsActive(v0) {
		t.Error("should be active after resume")
	}
}

func TestStackHalt(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	stack.Halt(v0)
	if !stack.IsHalted(v0) {
		t.Error("should be halted")
	}

	// Resume should not change halted to active.
	stack.Resume(v0)
	if stack.IsActive(v0) {
		t.Error("should not resume a halted version")
	}
}

func TestStackRemoveVersion(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})
	v1 := stack.AddVersion(StateID(2), Length{Bytes: 0})
	_ = stack.AddVersion(StateID(3), Length{Bytes: 0})

	stack.Halt(v1)
	stack.RemoveVersion(v1)

	if stack.VersionCount() != 2 {
		t.Errorf("version count after remove = %d, want 2", stack.VersionCount())
	}
	// v0 should still be state 1, but now the second slot is state 3.
	if stack.State(v0) != 1 {
		t.Errorf("v0 state = %d, want 1", stack.State(v0))
	}
	if stack.State(StackVersion(1)) != 3 {
		t.Errorf("v1 state = %d, want 3 (shifted)", stack.State(StackVersion(1)))
	}
}

func TestStackClear(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	stack.AddVersion(StateID(1), Length{Bytes: 0})
	stack.AddVersion(StateID(2), Length{Bytes: 0})

	stack.Clear()
	if stack.VersionCount() != 0 {
		t.Errorf("version count after clear = %d, want 0", stack.VersionCount())
	}
}

func TestStackTopSubtree(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// No links yet.
	top := stack.TopSubtree(v0)
	if !top.IsZero() {
		t.Error("empty stack should have zero top subtree")
	}

	// Push a leaf.
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(2), leaf, false, Length{Bytes: 1})

	top = stack.TopSubtree(v0)
	if top.IsZero() {
		t.Error("top subtree should not be zero after push")
	}
	if GetSymbol(top, arena) != 1 {
		t.Errorf("top subtree symbol = %d, want 1", GetSymbol(top, arena))
	}
}

func TestStackPopMultiplePathsMerged(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	// Build a diamond pattern:
	// State 1 -> (via leaf_a) -> State 5
	// State 2 -> (via leaf_b) -> State 5
	// Then State 5 -> (via leaf_c) -> State 10
	// Pop 2 from State 10 should give 2 paths.

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})
	v1 := stack.AddVersion(StateID(2), Length{Bytes: 0})

	leafA := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(5), leafA, false, Length{Bytes: 1})

	leafB := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(2), false, false, false, lang)
	stack.Push(v1, StateID(5), leafB, false, Length{Bytes: 1})

	// Merge v1 into v0.
	stack.Merge(v0, v1)

	// Push another level.
	leafC := NewLeafSubtree(arena, Symbol(3),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(5), false, false, false, lang)
	stack.Push(v0, StateID(10), leafC, false, Length{Bytes: 2})

	// Pop 2 should produce 2 paths (through the merge).
	results := stack.Pop(v0, 2)
	if len(results) != 2 {
		t.Fatalf("pop results = %d, want 2", len(results))
	}

	// Both paths should have 2 subtrees.
	for i, r := range results {
		if len(r.subtrees) != 2 {
			t.Errorf("result[%d] subtrees = %d, want 2", i, len(r.subtrees))
		}
	}
}

func TestStackCompactHaltedVersions(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	stack.AddVersion(StateID(1), Length{Bytes: 0})
	stack.AddVersion(StateID(2), Length{Bytes: 0})
	stack.AddVersion(StateID(3), Length{Bytes: 0})

	stack.Halt(StackVersion(1))

	stack.CompactHaltedVersions()

	if stack.VersionCount() != 2 {
		t.Errorf("version count = %d, want 2", stack.VersionCount())
	}
	if stack.State(StackVersion(0)) != 1 {
		t.Errorf("v0 state = %d, want 1", stack.State(StackVersion(0)))
	}
	if stack.State(StackVersion(1)) != 3 {
		t.Errorf("v1 state = %d, want 3", stack.State(StackVersion(1)))
	}
}

func TestStackLastExternalToken(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Initially no external token.
	token := stack.LastExternalToken(v0)
	if !token.IsZero() {
		t.Error("initial external token should be zero")
	}

	// Set an external token.
	leaf := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 0}, Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)
	stack.SetLastExternalToken(v0, leaf)

	token = stack.LastExternalToken(v0)
	if token.IsZero() {
		t.Error("external token should not be zero after set")
	}
	if GetSymbol(token, arena) != 5 {
		t.Errorf("external token symbol = %d, want 5", GetSymbol(token, arena))
	}
}

func TestStackActiveVersionCount(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	stack.AddVersion(StateID(1), Length{Bytes: 0})
	stack.AddVersion(StateID(2), Length{Bytes: 0})
	stack.AddVersion(StateID(3), Length{Bytes: 0})

	if stack.ActiveVersionCount() != 3 {
		t.Errorf("active = %d, want 3", stack.ActiveVersionCount())
	}

	stack.Pause(StackVersion(1))
	if stack.ActiveVersionCount() != 2 {
		t.Errorf("active = %d, want 2", stack.ActiveVersionCount())
	}

	stack.Halt(StackVersion(2))
	if stack.ActiveVersionCount() != 1 {
		t.Errorf("active = %d, want 1", stack.ActiveVersionCount())
	}
}

func TestStackPopEmpty(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	// Pop from non-existent version.
	results := stack.Pop(StackVersion(0), 1)
	if results != nil {
		t.Error("pop from empty stack should return nil")
	}

	// Pop from version with no links.
	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})
	results = stack.Pop(v0, 1)
	if len(results) != 0 {
		t.Errorf("pop from version with no links = %d, want 0", len(results))
	}
}
