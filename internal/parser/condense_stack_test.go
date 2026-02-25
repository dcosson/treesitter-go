package parser

import "testing"

// setupCondenseVersion creates a version with specific properties for testing condenseStack.
// This directly manipulates internal fields to precisely control the version's error status.
//
// Parameters:
//   - state: parse state (state 0 means "in error recovery")
//   - pos: byte position
//   - errorCost: accumulated error cost
//   - nodeCount: total node count (also used as NodeCountSinceError since nodeCountAtLastError=0)
//   - dynPrec: dynamic precedence
func setupCondenseVersion(stack *Stack, state StateID, pos uint32, errorCost uint32, nodeCount uint32, dynPrec int32) StackVersion {
	v := stack.AddVersion(state, Length{Bytes: pos})
	stack.SetNodeMetrics(v, errorCost, nodeCount, dynPrec)
	return v
}

// TestCondenseStackTakeLeft verifies that when compareVersions returns TakeLeft
// (j is decisively better), version i is removed.
func TestCondenseStackTakeLeft(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0 (j): cost=0, nodeCountSinceError=20
	// v1 (i): cost=200, nodeCountSinceError=0
	// compareVersions(v0, v1): costDiff=200, 200*(1+20)=4200 > 1600 → TakeLeft
	// So v1 should be removed.
	setupCondenseVersion(p.stack, 1, 0, 0, 20, 0)
	setupCondenseVersion(p.stack, 2, 0, 200, 1, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 1 {
		t.Fatalf("expected 1 version after TakeLeft, got %d", p.stack.VersionCount())
	}
	// The surviving version should be v0 (state=1).
	if p.stack.State(0) != 1 {
		t.Errorf("surviving version state = %d, want 1", p.stack.State(0))
	}
}

// TestCondenseStackTakeRight verifies that when compareVersions returns TakeRight
// (i is decisively better), version j is removed.
func TestCondenseStackTakeRight(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0 (j): cost=200, nodeCountSinceError=0
	// v1 (i): cost=0, nodeCountSinceError=20
	// compareVersions(v0, v1): costDiff=200, 200*(1+20)=4200 > 1600 → TakeRight
	// So v0 should be removed.
	setupCondenseVersion(p.stack, 1, 0, 200, 1, 0)
	setupCondenseVersion(p.stack, 2, 0, 0, 20, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 1 {
		t.Fatalf("expected 1 version after TakeRight, got %d", p.stack.VersionCount())
	}
	// The surviving version should be v1 (state=2).
	if p.stack.State(0) != 2 {
		t.Errorf("surviving version state = %d, want 2", p.stack.State(0))
	}
}

// TestCondenseStackPreferLeftMerge verifies that when compareVersions returns
// PreferLeft and versions share the same state, they are merged.
func TestCondenseStackPreferLeftMerge(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// Both versions must have same state, position, errorCost (CanMerge 5-condition).
	// v0 (j): cost=100, nodeCountSinceError=5, dynPrec=10
	// v1 (i): cost=100, nodeCountSinceError=5, dynPrec=0
	// compareVersions(v0, v1): same cost → check dynPrec → v0 (10) > v1 (0) → PreferLeft
	// Both at state 5, pos 0, errorCost 100 → can merge → v1 removed.
	setupCondenseVersion(p.stack, 5, 0, 100, 5, 10)
	setupCondenseVersion(p.stack, 5, 0, 100, 5, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 1 {
		t.Fatalf("expected 1 version after PreferLeft merge, got %d", p.stack.VersionCount())
	}
	if p.stack.State(0) != 5 {
		t.Errorf("surviving version state = %d, want 5", p.stack.State(0))
	}
}

// TestCondenseStackPreferLeftNoMerge verifies that when compareVersions returns
// PreferLeft but versions have different states, both survive (no merge possible).
func TestCondenseStackPreferLeftNoMerge(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0 (j): cost=0, nodeCountSinceError=5, state=1
	// v1 (i): cost=200, nodeCountSinceError=0, state=2
	// compareVersions(v0, v1): costDiff=200, 200*(1+5)=1200 < 1600 → PreferLeft
	// Different states → cannot merge → both survive.
	setupCondenseVersion(p.stack, 1, 0, 0, 5, 0)
	setupCondenseVersion(p.stack, 2, 0, 200, 1, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 2 {
		t.Fatalf("expected 2 versions (PreferLeft, no merge), got %d", p.stack.VersionCount())
	}
	// Ordering should be preserved (no swap on PreferLeft).
	if p.stack.State(0) != 1 {
		t.Errorf("v0 state = %d, want 1", p.stack.State(0))
	}
	if p.stack.State(StackVersion(1)) != 2 {
		t.Errorf("v1 state = %d, want 2", p.stack.State(StackVersion(1)))
	}
}

// TestCondenseStackNoneMerge verifies that when compareVersions returns None
// (equal costs and precedence) and versions share the same state, they merge.
func TestCondenseStackNoneMerge(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0 and v1: identical cost, nodeCount, dynPrec → None.
	// Same state → can merge.
	setupCondenseVersion(p.stack, 5, 0, 0, 10, 0)
	setupCondenseVersion(p.stack, 5, 0, 0, 10, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 1 {
		t.Fatalf("expected 1 version after None merge, got %d", p.stack.VersionCount())
	}
}

// TestCondenseStackPreferRightSwap verifies that when compareVersions returns
// PreferRight and versions have different states, they are swapped so the
// better version (i) gets the lower index.
func TestCondenseStackPreferRightSwap(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0 (j): cost=200, nodeCountSinceError=5, state=1
	// v1 (i): cost=0, nodeCountSinceError=5, state=2
	// compareVersions(v0, v1): v0.cost > v1.cost, costDiff=200, 200*(1+5)=1200 < 1600 → PreferRight
	// Different states → cannot merge → SWAP.
	// After swap: v0 should have state=2, v1 should have state=1.
	setupCondenseVersion(p.stack, 1, 0, 200, 5, 0)
	setupCondenseVersion(p.stack, 2, 0, 0, 5, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 2 {
		t.Fatalf("expected 2 versions after PreferRight swap, got %d", p.stack.VersionCount())
	}
	// Versions should be swapped: the better version (originally v1/state=2)
	// should now be at index 0.
	if p.stack.State(0) != 2 {
		t.Errorf("v0 state after swap = %d, want 2 (the better version)", p.stack.State(0))
	}
	if p.stack.State(StackVersion(1)) != 1 {
		t.Errorf("v1 state after swap = %d, want 1 (the worse version)", p.stack.State(StackVersion(1)))
	}
}

// TestCondenseStackPreferRightMerge verifies that when compareVersions returns
// PreferRight and versions share the same state, they merge (merge takes
// priority over swap).
func TestCondenseStackPreferRightMerge(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// Both versions must have same state, position, errorCost (CanMerge 5-condition).
	// v0 (j): cost=100, nodeCountSinceError=5, dynPrec=0
	// v1 (i): cost=100, nodeCountSinceError=5, dynPrec=10
	// compareVersions(v0, v1): same cost → check dynPrec → v1 (10) > v0 (0) → PreferRight
	// Same state, pos, errorCost → merge instead of swap → v1 removed, links merged into v0.
	setupCondenseVersion(p.stack, 5, 0, 100, 5, 0)
	setupCondenseVersion(p.stack, 5, 0, 100, 5, 10)

	p.condenseStack()

	if p.stack.VersionCount() != 1 {
		t.Fatalf("expected 1 version after PreferRight merge, got %d", p.stack.VersionCount())
	}
}

// TestCondenseStackPreferRightDynPrec verifies that PreferRight is triggered
// by dynamic precedence tie-breaking (equal costs, i has higher dynPrec).
func TestCondenseStackPreferRightDynPrec(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0 (j): dynPrec=0, state=1
	// v1 (i): dynPrec=10, state=2
	// Equal costs → Rule 3: v1.dynPrec > v0.dynPrec → PreferRight
	// Different states → swap.
	setupCondenseVersion(p.stack, 1, 0, 0, 5, 0)
	setupCondenseVersion(p.stack, 2, 0, 0, 5, 10)

	p.condenseStack()

	if p.stack.VersionCount() != 2 {
		t.Fatalf("expected 2 versions after dynPrec swap, got %d", p.stack.VersionCount())
	}
	// v1 (higher dynPrec) should be swapped to index 0.
	if p.stack.State(0) != 2 {
		t.Errorf("v0 state after dynPrec swap = %d, want 2 (higher dynPrec)", p.stack.State(0))
	}
	if p.stack.State(StackVersion(1)) != 1 {
		t.Errorf("v1 state after dynPrec swap = %d, want 1", p.stack.State(StackVersion(1)))
	}
}

// TestCondenseStackHaltedRemoval verifies that halted versions are removed
// at the start of the loop, before any comparisons.
func TestCondenseStackHaltedRemoval(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	setupCondenseVersion(p.stack, 1, 0, 0, 5, 0)
	v1 := setupCondenseVersion(p.stack, 2, 0, 0, 5, 0)
	setupCondenseVersion(p.stack, 3, 0, 0, 5, 0)

	// Halt v1 before calling condenseStack.
	p.stack.Halt(v1)

	p.condenseStack()

	if p.stack.VersionCount() != 2 {
		t.Fatalf("expected 2 versions after halted removal, got %d", p.stack.VersionCount())
	}
	// v0 and v2 should survive (v2 shifted to index 1).
	if p.stack.State(0) != 1 {
		t.Errorf("v0 state = %d, want 1", p.stack.State(0))
	}
	if p.stack.State(StackVersion(1)) != 3 {
		t.Errorf("v1 state = %d, want 3 (was v2)", p.stack.State(StackVersion(1)))
	}
}

// TestCondenseStackHardCap verifies that when more than MaxVersionCount versions
// survive comparison, the excess is removed from the end.
func TestCondenseStackHardCap(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// Create MaxVersionCount + 2 versions, all with different states
	// and similar costs (so no decisive kills or soft preferences).
	// All at cost=0, nodeCount=1, dynPrec=0 → compareVersions returns None.
	// Different states → cannot merge → all survive comparison phase.
	for i := 0; i < MaxVersionCount+2; i++ {
		setupCondenseVersion(p.stack, StateID(i+1), 0, 0, 1, 0)
	}

	p.condenseStack()

	if p.stack.VersionCount() != MaxVersionCount {
		t.Fatalf("expected %d versions after hard cap, got %d", MaxVersionCount, p.stack.VersionCount())
	}
	// The first MaxVersionCount versions should survive (removed from end).
	for i := 0; i < MaxVersionCount; i++ {
		want := StateID(i + 1)
		if p.stack.State(StackVersion(i)) != want {
			t.Errorf("v%d state = %d, want %d", i, p.stack.State(StackVersion(i)), want)
		}
	}
}

// TestCondenseStackSwapOrdering verifies that swaps from PreferRight
// correctly reorder versions so the hard cap removes the right ones.
// This tests the interaction between swaps and the hard cap.
func TestCondenseStackSwapOrdering(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// Create MaxVersionCount + 1 versions. The last version (highest index)
	// has the best dynPrec, triggering PreferRight swaps to move it
	// toward index 0. After swaps + hard cap, the worst version at the
	// end should be removed.
	for i := 0; i < MaxVersionCount+1; i++ {
		setupCondenseVersion(p.stack, StateID(i+1), 0, 0, 5, 0)
	}
	// Make the last version have higher dynamic precedence.
	lastIdx := MaxVersionCount
	p.stack.SetNodeMetrics(StackVersion(lastIdx), 0, 5, 100)

	p.condenseStack()

	if p.stack.VersionCount() != MaxVersionCount {
		t.Fatalf("expected %d versions after hard cap, got %d", MaxVersionCount, p.stack.VersionCount())
	}

	// The last version (state MaxVersionCount+1, dynPrec=100) should
	// have been swapped to a lower index and survived the hard cap.
	found := false
	for i := 0; i < p.stack.VersionCount(); i++ {
		if p.stack.State(StackVersion(i)) == StateID(lastIdx+1) {
			found = true
			break
		}
	}
	if !found {
		t.Error("high-dynPrec version should survive after swap + hard cap")
		for i := 0; i < p.stack.VersionCount(); i++ {
			t.Logf("  v%d: state=%d, dynPrec=%d", i, p.stack.State(StackVersion(i)),
				p.stack.DynamicPrecedence(StackVersion(i)))
		}
	}
}

// TestCondenseStackMultipleKills verifies that condenseStack correctly handles
// multiple decisive kills in a single pass — both killing i (TakeLeft from
// multiple j's) and killing multiple j's (TakeRight from a single i).
func TestCondenseStackMultipleKills(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0: cost=0, nodeCount=20 (established, good version)
	// v1: cost=500, nodeCount=1 (bad, should be killed by v0)
	// v2: cost=400, nodeCount=1 (bad, should be killed by v0)
	// v3: cost=0, nodeCount=20 (established, good version)
	// 500*(1+20)=10500 > 1600 → TakeLeft v1 by v0
	// 400*(1+20)=8400 > 1600 → TakeLeft v2 by v0
	// v0 and v3 have same cost → None, different states → survive
	setupCondenseVersion(p.stack, 1, 0, 0, 20, 0)
	setupCondenseVersion(p.stack, 2, 0, 500, 1, 0)
	setupCondenseVersion(p.stack, 3, 0, 400, 1, 0)
	setupCondenseVersion(p.stack, 4, 0, 0, 20, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 2 {
		t.Fatalf("expected 2 versions after multiple kills, got %d", p.stack.VersionCount())
	}
	if p.stack.State(0) != 1 {
		t.Errorf("v0 state = %d, want 1", p.stack.State(0))
	}
	if p.stack.State(StackVersion(1)) != 4 {
		t.Errorf("v1 state = %d, want 4", p.stack.State(StackVersion(1)))
	}
}

// TestCondenseStackChainedSwaps verifies that multiple PreferRight outcomes
// cause cascading swaps, moving the best version toward index 0.
func TestCondenseStackChainedSwaps(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0: dynPrec=0, state=1
	// v1: dynPrec=5, state=2
	// v2: dynPrec=10, state=3
	// All at same cost=0, same nodeCount → Rule 3 decides.
	//
	// When i=1, j=0: compare(v0, v1) → dynPrec 0 < 5 → PreferRight → swap(v0,v1)
	//   After: [v1(dp=5,s=2), v0(dp=0,s=1)]
	// When i=2, j=0: compare(v1_swapped(dp=5), v2(dp=10)) → PreferRight → swap(v0,v2)
	//   After: [v2(dp=10,s=3), v0(dp=0,s=1), v1_swapped(dp=5,s=2)]
	//   Wait, need to re-think. After first swap:
	//     slot0=state2/dp5, slot1=state1/dp0
	//   Then i=2, j=0: compare(slot0=dp5, v2=dp10) → PreferRight → swap(0,2)
	//     slot0=state3/dp10, slot1=state1/dp0, slot2=state2/dp5
	//   Then i=2, j=1: compare(slot1=dp0, slot2=dp5) → PreferRight → swap(1,2)
	//     slot0=state3/dp10, slot1=state2/dp5, slot2=state1/dp0
	//
	// Result: sorted by dynPrec descending.
	setupCondenseVersion(p.stack, 1, 0, 0, 5, 0)
	setupCondenseVersion(p.stack, 2, 0, 0, 5, 5)
	setupCondenseVersion(p.stack, 3, 0, 0, 5, 10)

	p.condenseStack()

	if p.stack.VersionCount() != 3 {
		t.Fatalf("expected 3 versions, got %d", p.stack.VersionCount())
	}
	// Verify sorted order: highest dynPrec at lowest index.
	if p.stack.State(0) != 3 {
		t.Errorf("v0 state = %d, want 3 (highest dynPrec)", p.stack.State(0))
	}
	if p.stack.State(StackVersion(1)) != 2 {
		t.Errorf("v1 state = %d, want 2 (middle dynPrec)", p.stack.State(StackVersion(1)))
	}
	if p.stack.State(StackVersion(2)) != 1 {
		t.Errorf("v2 state = %d, want 1 (lowest dynPrec)", p.stack.State(StackVersion(2)))
	}
}

// TestCondenseStackMergeRemovesVersion verifies that merge (via
// PreferLeft/None) removes the source version (not just halts it),
// matching the C behavior of ts_stack_merge.
func TestCondenseStackMergeRemovesVersion(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0: state=5, cost=0
	// v1: state=5, cost=0 (same state → can merge)
	// v2: state=10 (different state, should survive)
	setupCondenseVersion(p.stack, 5, 0, 0, 5, 0)
	setupCondenseVersion(p.stack, 5, 0, 0, 5, 0)
	setupCondenseVersion(p.stack, 10, 0, 0, 5, 0)

	p.condenseStack()

	// v0 and v1 should merge → 2 versions remain.
	if p.stack.VersionCount() != 2 {
		t.Fatalf("expected 2 versions after merge + non-mergeable, got %d", p.stack.VersionCount())
	}
	if p.stack.State(0) != 5 {
		t.Errorf("v0 state = %d, want 5", p.stack.State(0))
	}
	if p.stack.State(StackVersion(1)) != 10 {
		t.Errorf("v1 state = %d, want 10", p.stack.State(StackVersion(1)))
	}
}

// TestCondenseStackErrorStateVsNonError verifies that non-error versions
// are preferred over in-error versions (state 0 = error recovery).
func TestCondenseStackErrorStateVsNonError(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0: state=0 (in-error), cost=50
	// v1: state=5 (non-error), cost=30
	// Rule 1: non-error beats in-error with lower cost → TakeRight → v0 removed.
	setupCondenseVersion(p.stack, 0, 0, 50, 5, 0)
	setupCondenseVersion(p.stack, 5, 0, 30, 5, 0)

	p.condenseStack()

	if p.stack.VersionCount() != 1 {
		t.Fatalf("expected 1 version after error vs non-error, got %d", p.stack.VersionCount())
	}
	if p.stack.State(0) != 5 {
		t.Errorf("surviving version state = %d, want 5 (non-error)", p.stack.State(0))
	}
}

// TestCondenseStackPausedVersion verifies that paused versions are treated
// as in-error (with cost penalty) and handled correctly.
func TestCondenseStackPausedVersion(t *testing.T) {
	p := NewParser()
	p.stack = NewStack(p.arena)

	// v0: state=5, active, cost=0
	// v1: state=10, paused, cost=0 → effective cost = ErrorCostPerSkippedTree=100, isInError=true
	// Rule 1: non-error (v0) vs in-error (v1), v0.cost(0) < v1.cost(100) → TakeLeft → v1 removed.
	setupCondenseVersion(p.stack, 5, 0, 0, 20, 0)
	v1 := setupCondenseVersion(p.stack, 10, 0, 0, 1, 0)
	p.stack.Pause(v1, SubtreeZero)

	p.condenseStack()

	if p.stack.VersionCount() != 1 {
		t.Fatalf("expected 1 version after paused removal, got %d", p.stack.VersionCount())
	}
	if p.stack.State(0) != 5 {
		t.Errorf("surviving version state = %d, want 5", p.stack.State(0))
	}
}
