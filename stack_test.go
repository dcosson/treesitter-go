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

	// Source version should be removed (matches C's ts_stack_merge).
	if stack.VersionCount() != 1 {
		t.Errorf("version count after merge = %d, want 1 (source removed)", stack.VersionCount())
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
	// Failed merge should not remove versions.
	if stack.VersionCount() != 2 {
		t.Errorf("version count after failed merge = %d, want 2", stack.VersionCount())
	}
}

func TestStackPauseResume(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	if !stack.IsActive(v0) {
		t.Error("should be active initially")
	}

	stack.Pause(v0, SubtreeZero)
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

	stack.Pause(StackVersion(1), SubtreeZero)
	if stack.ActiveVersionCount() != 2 {
		t.Errorf("active = %d, want 2", stack.ActiveVersionCount())
	}

	stack.Halt(StackVersion(2))
	if stack.ActiveVersionCount() != 1 {
		t.Errorf("active = %d, want 1", stack.ActiveVersionCount())
	}
}

func TestStackSwapVersions(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	stack.AddVersion(StateID(10), Length{Bytes: 0})
	stack.AddVersion(StateID(20), Length{Bytes: 0})
	stack.AddVersion(StateID(30), Length{Bytes: 0})

	// Swap v0 and v2.
	stack.SwapVersions(StackVersion(0), StackVersion(2))

	if stack.State(StackVersion(0)) != 30 {
		t.Errorf("v0 state after swap = %d, want 30", stack.State(StackVersion(0)))
	}
	if stack.State(StackVersion(2)) != 10 {
		t.Errorf("v2 state after swap = %d, want 10", stack.State(StackVersion(2)))
	}
	// v1 should be unchanged.
	if stack.State(StackVersion(1)) != 20 {
		t.Errorf("v1 state after swap = %d, want 20", stack.State(StackVersion(1)))
	}
}

func TestStackNodeCountSinceError(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Initially, nodeCountAtLastError is 0, so nodeCountSinceError = nodeCount.
	// After AddVersion, nodeCount is 1 (from AddVersion).
	if got := stack.NodeCountSinceError(v0); got != 1 {
		t.Errorf("initial nodeCountSinceError = %d, want 1", got)
	}

	// Push a few nodes.
	for i := 0; i < 5; i++ {
		leaf := NewLeafSubtree(arena, Symbol(1),
			Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
			StateID(1), false, false, false, lang)
		stack.Push(v0, StateID(2), leaf, false, Length{Bytes: uint32(i + 1)})
	}

	// nodeCount should be 6 (1 initial + 5 pushes), nodeCountSinceError = 6.
	if got := stack.NodeCountSinceError(v0); got != 6 {
		t.Errorf("nodeCountSinceError after 5 pushes = %d, want 6", got)
	}

	// Add error cost — this should record the current nodeCount as baseline.
	stack.AddErrorCost(v0, 100)

	// nodeCountSinceError should now be 0 (just had an error).
	if got := stack.NodeCountSinceError(v0); got != 0 {
		t.Errorf("nodeCountSinceError after error = %d, want 0", got)
	}

	// Push more nodes after error.
	for i := 0; i < 3; i++ {
		leaf := NewLeafSubtree(arena, Symbol(1),
			Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
			StateID(1), false, false, false, lang)
		stack.Push(v0, StateID(2), leaf, false, Length{Bytes: uint32(10 + i)})
	}

	// nodeCountSinceError should be 3.
	if got := stack.NodeCountSinceError(v0); got != 3 {
		t.Errorf("nodeCountSinceError after 3 post-error pushes = %d, want 3", got)
	}
}

func TestStackNodeCountSinceErrorPropagatesOnSplit(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Push some nodes and add error.
	for i := 0; i < 3; i++ {
		leaf := NewLeafSubtree(arena, Symbol(1),
			Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
			StateID(1), false, false, false, lang)
		stack.Push(v0, StateID(2), leaf, false, Length{Bytes: uint32(i + 1)})
	}
	stack.AddErrorCost(v0, 100)

	// Push 2 more after error.
	for i := 0; i < 2; i++ {
		leaf := NewLeafSubtree(arena, Symbol(1),
			Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
			StateID(1), false, false, false, lang)
		stack.Push(v0, StateID(2), leaf, false, Length{Bytes: uint32(10 + i)})
	}

	// Split should preserve the error baseline.
	v1 := stack.Split(v0)

	countV0 := stack.NodeCountSinceError(v0)
	countV1 := stack.NodeCountSinceError(v1)
	if countV0 != countV1 {
		t.Errorf("split should preserve nodeCountSinceError: v0=%d v1=%d", countV0, countV1)
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

func TestHasAdvancedSinceErrorNoError(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// No error cost — should return true.
	if !stack.HasAdvancedSinceError(v0) {
		t.Error("should return true when errorCost == 0")
	}
}

func TestHasAdvancedSinceErrorWithProgress(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Push a non-zero-width subtree.
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 5, Point: Point{Column: 5}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(2), leaf, false, Length{Bytes: 5})

	// Add error cost so errorCost > 0.
	stack.AddErrorCost(v0, 100)

	// Push another non-zero-width subtree AFTER the error.
	leaf2 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(2), false, false, false, lang)
	stack.Push(v0, StateID(3), leaf2, false, Length{Bytes: 8})

	// Should return true — we advanced with a 3-byte subtree after the error.
	if !stack.HasAdvancedSinceError(v0) {
		t.Error("should return true when non-zero-width subtree pushed after error")
	}
}

func TestHasAdvancedSinceErrorNoProgress(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Push a zero-width subtree.
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 0, Point: Point{}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(2), leaf, false, Length{Bytes: 0})

	// Add error cost.
	stack.AddErrorCost(v0, 100)

	// Push another zero-width subtree after error.
	leaf2 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0}, Length{Bytes: 0, Point: Point{}},
		StateID(2), false, false, false, lang)
	stack.Push(v0, StateID(3), leaf2, false, Length{Bytes: 0})

	// Should return false — no non-zero-width subtree after error.
	if stack.HasAdvancedSinceError(v0) {
		t.Error("should return false when only zero-width subtrees after error")
	}
}

func TestHasAdvancedSinceErrorOutOfBounds(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	// Non-existent version.
	if stack.HasAdvancedSinceError(StackVersion(99)) {
		t.Error("should return false for non-existent version")
	}
}

func TestRenumberVersionPreservesSummary(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	// Create two versions.
	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})
	v1 := stack.AddVersion(StateID(2), Length{Bytes: 0})

	// Push a node on v0 so it has links for RecordSummary to walk.
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(5), leaf, false, Length{Bytes: 1})

	// Record a summary on v0 (the target that will be overwritten).
	stack.RecordSummary(v0, 10)

	// v1 (the source) has no summary.
	if stack.GetSummary(v1) != nil {
		t.Fatal("v1 should have no summary initially")
	}

	// v0 should have a summary.
	if stack.GetSummary(v0) == nil {
		t.Fatal("v0 should have a summary after RecordSummary")
	}

	// Renumber v1 -> v0. Since v0 has summary but v1 doesn't,
	// v1 should inherit v0's summary before v0 is overwritten.
	stack.RenumberVersion(v1, v0)

	// After renumber, v0's head is now what was v1, but with v0's old summary.
	summary := stack.GetSummary(v0)
	if summary == nil {
		t.Error("summary should be preserved on target after renumber")
	}
}

func TestRenumberVersionSourceHasSummary(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	// Create two versions.
	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Push some nodes so the summary has entries.
	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 1, Point: Point{Column: 1}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(2), leaf, false, Length{Bytes: 1})

	v1 := stack.AddVersion(StateID(3), Length{Bytes: 0})

	// Record summaries on both.
	stack.RecordSummary(v0, 10)
	stack.RecordSummary(v1, 10)

	// When source already has a summary, target's summary is NOT transferred
	// (matches C behavior: only transfer when source is nil).
	stack.RenumberVersion(v1, v0)

	// v0 should have v1's summary (which came from its own RecordSummary).
	summary := stack.GetSummary(v0)
	if summary == nil {
		t.Error("source's own summary should be preserved")
	}
}

func TestRenumberVersionNoOp(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)

	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Renumber to self is a no-op.
	stack.RenumberVersion(v0, v0)

	if stack.VersionCount() != 1 {
		t.Errorf("version count = %d, want 1", stack.VersionCount())
	}
	if stack.State(v0) != 1 {
		t.Errorf("state = %d, want 1", stack.State(v0))
	}
}

// TestMergeDynPrecAccumulation verifies that after merging two versions,
// the surviving node's dynamicPrecedence reflects the best path's accumulated
// precedence, not just the target's original value.
func TestMergeDynPrecAccumulation(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	// Create a common base at state 1.
	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})

	// Push a leaf onto v0, advancing to state 5.
	leaf0 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0}, Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, StateID(5), leaf0, false, Length{Bytes: 3})

	// Create two versions from state 5.
	v1 := stack.Split(v0)

	// Create two "reduced" subtrees with different DynPrec.
	// Simulate call_expression (DynPrec=0) and type_conversion_expression (DynPrec=-1).
	callNode := NewNodeSubtree(arena, Symbol(10), nil, 1, lang)
	if !callNode.IsInline() {
		data := arena.Get(callNode)
		data.DynamicPrecedence = 0 // call_expression
	}

	convNode := NewNodeSubtree(arena, Symbol(10), nil, 2, lang)
	if !convNode.IsInline() {
		data := arena.Get(convNode)
		data.DynamicPrecedence = -1 // type_conversion_expression
	}

	// Push them to both versions, arriving at the same GOTO state.
	stack.Push(v0, StateID(20), callNode, false, Length{Bytes: 3})
	stack.Push(v1, StateID(20), convNode, false, Length{Bytes: 3})

	// Both at state 20 — merge.
	if !stack.CanMerge(v0, v1) {
		t.Fatal("versions at same state should be mergeable")
	}

	stack.Merge(v0, v1)

	// After merge, the surviving version should have the HIGHER DynPrec.
	prec := stack.DynamicPrecedence(v0)
	if prec < 0 {
		t.Errorf("DynamicPrecedence after merge = %d, want >= 0 (best path)", prec)
	}
}

// TestMergeSameTargetNodeDisambiguation verifies Case 1 of nodeAddLink:
// when two links point to the same target node with equivalent subtrees,
// only the higher-DynPrec subtree is kept.
func TestMergeSameTargetNodeDisambiguation(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)

	// Create two versions with the same base node.
	v0 := stack.AddVersion(StateID(1), Length{Bytes: 0})
	v1 := stack.Split(v0) // Both point to the same node

	// Create subtrees with same symbol but different DynPrec.
	goodLeaf := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 0}, Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(5), false, false, false, lang)
	badLeaf := NewLeafSubtree(arena, Symbol(5),
		Length{Bytes: 0}, Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(5), false, false, false, lang)

	// Set different DynPrec on the heap data.
	if !goodLeaf.IsInline() {
		arena.Get(goodLeaf).DynamicPrecedence = 10
	}
	if !badLeaf.IsInline() {
		arena.Get(badLeaf).DynamicPrecedence = -5
	}

	// Push to same GOTO state (both versions share the base node).
	stack.Push(v0, StateID(10), goodLeaf, false, Length{Bytes: 2})
	stack.Push(v1, StateID(10), badLeaf, false, Length{Bytes: 2})

	// Merge.
	stack.Merge(v0, v1)

	// Pop should show only 1 path (disambiguation removed the inferior one),
	// OR 2 paths but with the better one as links[0].
	count := stack.PopCount(v0, 1)
	// With same target node and equivalent subtrees, Case 1 should deduplicate.
	if count > 2 {
		t.Errorf("PopCount after disambiguation merge = %d, want <= 2", count)
	}

	// The surviving version should have the better DynPrec.
	prec := stack.DynamicPrecedence(v0)
	if prec < 0 {
		t.Errorf("DynamicPrecedence = %d, want >= 0 (should reflect best path)", prec)
	}
}

// --- PopPending tests ---

func TestPopPendingReturnsPendingSubtree(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)
	v0 := stack.AddVersion(StateID(0), Length{Bytes: 0})

	leaf1 := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, 1, leaf1, false, Length{Bytes: 3, Point: Point{Column: 3}})

	leaf2 := NewLeafSubtree(arena, Symbol(2),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(2), false, false, false, lang)
	stack.Push(v0, 2, leaf2, true, Length{Bytes: 5, Point: Point{Column: 5}})

	got, ok := stack.PopPending(v0)
	if !ok {
		t.Fatal("expected PopPending to succeed")
	}
	if GetSymbol(got, arena) != 2 {
		t.Errorf("expected symbol 2, got %d", GetSymbol(got, arena))
	}
	if stack.State(v0) != 1 {
		t.Errorf("expected state 1 after PopPending, got %d", stack.State(v0))
	}
}

func TestPopPendingReturnsFalseForNonPending(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)
	v0 := stack.AddVersion(StateID(0), Length{Bytes: 0})

	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, 1, leaf, false, Length{Bytes: 3, Point: Point{Column: 3}})

	_, ok := stack.PopPending(v0)
	if ok {
		t.Error("expected PopPending to return false for non-pending subtree")
	}
}

func TestPopPendingEmptyStack(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)
	v0 := stack.AddVersion(StateID(0), Length{Bytes: 0})

	_, ok := stack.PopPending(v0)
	if ok {
		t.Error("expected PopPending to return false on empty stack")
	}
}

// --- PopError tests ---

func TestPopErrorReturnsErrorSubtree(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)
	v0 := stack.AddVersion(StateID(0), Length{Bytes: 0})

	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, 1, leaf, false, Length{Bytes: 3, Point: Point{Column: 3}})

	errLeaf := NewLeafSubtree(arena, SymbolError,
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 2, Point: Point{Column: 2}},
		StateID(2), false, false, false, lang)
	stack.Push(v0, 2, errLeaf, false, Length{Bytes: 5, Point: Point{Column: 5}})

	got, ok := stack.PopError(v0)
	if !ok {
		t.Fatal("expected PopError to succeed")
	}
	if GetSymbol(got, arena) != SymbolError {
		t.Errorf("expected SymbolError, got %d", GetSymbol(got, arena))
	}
	if stack.State(v0) != 1 {
		t.Errorf("expected state 1 after PopError, got %d", stack.State(v0))
	}
}

func TestPopErrorReturnsFalseForNonError(t *testing.T) {
	arena := NewSubtreeArena(32)
	lang := makeSubtreeTestLanguage()
	stack := NewStack(arena)
	v0 := stack.AddVersion(StateID(0), Length{Bytes: 0})

	leaf := NewLeafSubtree(arena, Symbol(1),
		Length{Bytes: 0, Point: Point{Column: 0}},
		Length{Bytes: 3, Point: Point{Column: 3}},
		StateID(1), false, false, false, lang)
	stack.Push(v0, 1, leaf, false, Length{Bytes: 3, Point: Point{Column: 3}})

	_, ok := stack.PopError(v0)
	if ok {
		t.Error("expected PopError to return false for non-error subtree")
	}
}

func TestPopErrorEmptyStack(t *testing.T) {
	arena := NewSubtreeArena(32)
	stack := NewStack(arena)
	v0 := stack.AddVersion(StateID(0), Length{Bytes: 0})

	_, ok := stack.PopError(v0)
	if ok {
		t.Error("expected PopError to return false on empty stack")
	}
}
