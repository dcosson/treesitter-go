# UMS: C++ GLR Timeout Analysis

**Bead**: tree-sitter-go-ums
**Date**: 2026-02-16
**Author**: reviewer agent
**Status**: Analysis complete, implementation plan ready

## Problem Statement

8 C++ corpus tests time out at exactly 10.00s, all in GLR ambiguity resolution:
- casts_vs_multiplications
- Noreturn_Type_Qualifier
- For_loops
- Switch_statements
- Concept_definition
- Compound_literals_without_parentheses
- Template_calls
- Parameter_pack_expansions

These inputs trigger heavy GLR splitting (e.g., `(Type)expr` is ambiguous between
cast and multiplication). The parser creates many version forks but fails to
prune them efficiently, leading to version explosion and an effective infinite loop.

## Root Cause Summary

Our `condenseStack()` is a simplified version of C's `ts_parser__condense_stack()`.
The C implementation uses a sophisticated pair-wise comparison algorithm
(`ts_parser__compare_versions`) that considers 4 dimensions of version quality
and can *decisively* kill inferior versions. Our implementation only merges
same-state versions and applies a simple cost threshold, which is insufficient
for the C++ grammar's heavy ambiguity patterns.

---

## C Reference: How It Works

### Data Structures

```c
// C reference: error_costs.h
#define ERROR_STATE 0
#define ERROR_COST_PER_RECOVERY    500
#define ERROR_COST_PER_MISSING_TREE 110
#define ERROR_COST_PER_SKIPPED_TREE 100
#define ERROR_COST_PER_SKIPPED_LINE  30
#define ERROR_COST_PER_SKIPPED_CHAR   1

// C reference: parser.c
static const unsigned MAX_VERSION_COUNT = 6;
static const unsigned MAX_COST_DIFFERENCE = 16 * ERROR_COST_PER_SKIPPED_TREE; // = 1600

typedef struct {
  unsigned cost;
  unsigned node_count;       // nodes since last error
  int dynamic_precedence;
  bool is_in_error;          // paused OR in ERROR_STATE
} ErrorStatus;

typedef enum {
  ErrorComparisonTakeLeft,    // definitively kill right
  ErrorComparisonPreferLeft,  // prefer left, but try merge first
  ErrorComparisonNone,        // equivalent, try merge
  ErrorComparisonPreferRight, // prefer right, swap or merge
  ErrorComparisonTakeRight,   // definitively kill left
} ErrorComparison;
```

### C reference: StackHead

```c
typedef struct {
  StackNode *node;
  StackSummary *summary;
  unsigned node_count_at_last_error;  // ← WE ARE MISSING THIS
  Subtree last_external_token;
  Subtree lookahead_when_paused;
  StackStatus status;
} StackHead;
```

### ts_parser__version_status (build status for comparison)

```c
static ErrorStatus ts_parser__version_status(TSParser *self, StackVersion version) {
  unsigned cost = ts_stack_error_cost(self->stack, version);
  bool is_paused = ts_stack_is_paused(self->stack, version);
  if (is_paused) cost += ERROR_COST_PER_SKIPPED_TREE;
  return (ErrorStatus) {
    .cost = cost,
    .node_count = ts_stack_node_count_since_error(self->stack, version),
    .dynamic_precedence = ts_stack_dynamic_precedence(self->stack, version),
    .is_in_error = is_paused || ts_stack_state(self->stack, version) == ERROR_STATE
  };
}
```

### ts_stack_node_count_since_error

```c
unsigned ts_stack_node_count_since_error(const Stack *self, StackVersion version) {
  StackHead *head = array_get(&self->heads, version);
  if (head->node->node_count < head->node_count_at_last_error) {
    head->node_count_at_last_error = head->node->node_count;
  }
  return head->node->node_count - head->node_count_at_last_error;
}
```

### ts_parser__compare_versions (THE KEY FUNCTION)

```c
static ErrorComparison ts_parser__compare_versions(
  TSParser *self, ErrorStatus a, ErrorStatus b
) {
  // Rule 1: Non-error vs in-error — strong preference for non-error
  if (!a.is_in_error && b.is_in_error) {
    return (a.cost < b.cost) ? TakeLeft : PreferLeft;
  }
  if (a.is_in_error && !b.is_in_error) {
    return (b.cost < a.cost) ? TakeRight : PreferRight;
  }

  // Rule 2: Cost comparison with node_count amplification
  if (a.cost < b.cost) {
    if ((b.cost - a.cost) * (1 + a.node_count) > MAX_COST_DIFFERENCE) {
      return TakeLeft;    // DECISIVE KILL
    } else {
      return PreferLeft;  // soft preference
    }
  }
  if (b.cost < a.cost) {
    if ((a.cost - b.cost) * (1 + b.node_count) > MAX_COST_DIFFERENCE) {
      return TakeRight;   // DECISIVE KILL
    } else {
      return PreferRight; // soft preference
    }
  }

  // Rule 3: Equal cost — break ties by dynamic precedence
  if (a.dynamic_precedence > b.dynamic_precedence) return PreferLeft;
  if (b.dynamic_precedence > a.dynamic_precedence) return PreferRight;

  return None;
}
```

**Critical insight**: The `(cost_diff) * (1 + node_count)` formula is an
amplification factor. As a version accumulates more nodes since its last error,
even a small cost difference becomes decisive. This prevents long-lived
error-recovery forks from lingering. For example:
- Cost difference of 100, node_count of 20 → `100 * 21 = 2100 > 1600` → TAKE (decisive kill)
- Cost difference of 100, node_count of 5 → `100 * 6 = 600 < 1600` → PREFER (try merge first)

### ts_parser__condense_stack (THE ALGORITHM)

```c
static unsigned ts_parser__condense_stack(TSParser *self) {
  unsigned min_error_cost = UINT_MAX;

  for (StackVersion i = 0; i < version_count; i++) {
    // Step 1: Remove halted versions immediately
    if (is_halted(i)) { remove(i); i--; continue; }

    ErrorStatus status_i = version_status(i);
    if (!status_i.is_in_error && status_i.cost < min_error_cost) {
      min_error_cost = status_i.cost;
    }

    // Step 2: Compare version i against ALL prior versions j < i
    for (StackVersion j = 0; j < i; j++) {
      ErrorStatus status_j = version_status(j);
      switch (compare_versions(status_j, status_i)) {

        case TakeLeft:
          // j is decisively better — KILL i
          remove(i); i--; j = i; break;

        case PreferLeft:
        case None:
          // j is better or equal — try merge (same state required)
          if (merge(j, i)) { i--; j = i; }
          break;

        case PreferRight:
          // i is better — try merge, or SWAP positions
          if (merge(j, i)) { i--; j = i; }
          else { swap_versions(i, j); }  // ← SWAP!
          break;

        case TakeRight:
          // i is decisively better — KILL j
          remove(j); i--; j--; break;
      }
    }
  }

  // Step 3: Hard cap — remove from the END (highest index = newest/worst)
  while (version_count > MAX_VERSION_COUNT) {
    remove(MAX_VERSION_COUNT);
  }

  // Step 4: Resume one paused version (error recovery)
  // ...
}
```

### ts_stack_swap_versions

```c
void ts_stack_swap_versions(Stack *self, StackVersion v1, StackVersion v2) {
  StackHead temporary_head = self->heads.contents[v1];
  self->heads.contents[v1] = self->heads.contents[v2];
  self->heads.contents[v2] = temporary_head;
}
```

---

## Our Implementation: What We Have

### condenseStack (parser.go:1053-1136)

```go
func (p *Parser) condenseStack() {
    // Phase 1: Merge compatible versions (same state only)
    for i := range versions {
        for j := i+1; range versions {
            if canMerge(i, j) { merge(i, j) }
        }
    }

    // Phase 2: Prune by absolute cost threshold
    bestCost := min(all active costs)
    for each active version v {
        if errorCost(v) > bestCost + MaxCostDifference {
            halt(v)
        }
    }

    // Phase 3: Hard cap with worst-version search
    if activeCount > MaxVersionCount {
        find and halt versions with worst (highest cost, lowest prec)
    }

    // Phase 4: Remove halted versions
    compactHaltedVersions()
}
```

### Merge (stack.go:408-454)

```go
func (s *Stack) Merge(target, source StackVersion) bool {
    // States must match
    // Copy source's links to target
    // Swap links[0] if new link has higher dynamic precedence
    // Halt source
}
```

---

## Gap Analysis: 7 Critical Differences

### Gap 1: No pair-wise version comparison (CRITICAL)

**C**: Compares every version `i` against every prior version `j < i` using
the 5-way `ErrorComparison` result. This allows decisive kills (TakeLeft/TakeRight)
even when versions DON'T share the same state.

**Ours**: Only merges versions with the same state. Versions with different states
are never compared at all — they can only be pruned by the absolute cost threshold.

**Impact**: C++ templates create many versions at different states that are
clearly inferior but never get killed because they're at different states.
They accumulate without bound until the hard cap removes them arbitrarily.

### Gap 2: No node_count amplification (CRITICAL)

**C**: `(cost_diff) * (1 + node_count) > MAX_COST_DIFFERENCE` means even tiny
cost differences become decisive as a version accumulates nodes. A version
with cost +50 and 40 nodes since error → `50 * 41 = 2050 > 1600` → killed.

**Ours**: Pure absolute threshold `cost > bestCost + 1800`. A version needs
to be 1800+ cost units worse to get pruned, regardless of how many nodes
it has accumulated.

**Impact**: Slightly-worse versions (cost difference < 1800) survive indefinitely,
creating exponential version growth on ambiguous inputs.

### Gap 3: No `is_in_error` concept (MODERATE)

**C**: A version is "in error" if it's paused or in ERROR_STATE (state 0).
Non-error versions always beat error versions: `(!a.is_in_error && b.is_in_error)`
→ TakeLeft or PreferLeft.

**Ours**: No concept of "in error" state. Error-recovery versions are treated
the same as healthy versions during comparison. We do add cost penalties for
error recovery, but the status check provides an additional fast-path for killing
doomed versions.

**Impact**: Error-recovery forks linger longer than they should, consuming
version slots that healthy forks could use.

### Gap 4: No version swapping (MODERATE)

**C**: When version `i` is preferred over version `j` but they can't merge
(different states), C swaps their positions: `swap_versions(i, j)`. This
moves the better version to the lower index, which matters because the hard
cap removes versions from the END (highest index).

**Ours**: No swap operation. When the hard cap fires, we search for the worst
version and halt it. This is similar in effect but less efficient and doesn't
preserve the ordering invariant that C relies on.

**Impact**: Lower impact since our hard cap does search for worst, but the
ordering during pair-wise comparison affects which versions survive later
comparisons. Without swapping, a good version at a high index could be
compared unfavorably against a worse version at a lower index.

### Gap 5: Missing `nodeCountAtLastError` tracking (MODERATE)

**C**: `StackHead` tracks `node_count_at_last_error`. The function
`ts_stack_node_count_since_error` returns `node.node_count - head.node_count_at_last_error`,
giving the number of nodes parsed since the last error.

**Ours**: `StackHead` has no `nodeCountAtLastError` field. We have `NodeCount()`
which returns the total `head.node.nodeCount`, but NOT the count since last error.

**Impact**: Required for Gap 2 (node_count amplification). Without this,
we can't implement the `(cost_diff) * (1 + node_count)` formula correctly.

### Gap 6: MaxCostDifference value mismatch (MINOR)

**C**: `MAX_COST_DIFFERENCE = 16 * 100 = 1600`
**Ours**: `MaxCostDifference = 1800`

**Impact**: Our threshold is 12.5% more lenient. With node_count amplification
this matters less (the amplification dominates), but without it (our current state)
this makes pruning even weaker.

### Gap 7: Hard cap removes wrong versions (MINOR)

**C**: Hard cap removes from the end: `while (count > MAX) remove(MAX)`.
This relies on the ordering invariant maintained by swapping — better versions
are at lower indices.

**Ours**: Hard cap searches for worst version by score. This is arguably
better in isolation, but without the pair-wise comparison and swapping that
C does first, our hard cap is the ONLY pruning mechanism for different-state
versions, so it fires much more often and may make suboptimal choices.

---

## Why C++ Specifically

C++ has the densest ambiguity patterns of any supported grammar:

1. **Cast vs multiplication**: `(Type)expr` — is `(Type)` a C-style cast or
   is `Type` being multiplied? Creates 2 versions per occurrence.

2. **Template vs comparison**: `a<b>c` — is this `template<args>` or
   `(a < b) > c`? Creates 2 versions per occurrence.

3. **Nesting**: These patterns nest. `(A<B>)(C<D>)e` creates 2^4 = 16
   potential parse paths. Without proper pruning, versions explode
   exponentially.

The C reference handles this because:
- Pair-wise comparison kills clearly-inferior versions immediately
- Node count amplification ensures slightly-worse versions die quickly
  once they've been running for a while
- Version swapping ensures the best versions occupy low indices and
  survive the hard cap

Our implementation handles this poorly because:
- Only same-state merging occurs — different-state versions survive forever
- Absolute cost threshold (1800) is never reached for slightly-worse
  error-free versions (they have cost 0)
- Hard cap fires every cycle but just removes arbitrary versions, so
  the explosion continues at a capped rate

---

## Implementation Plan

### Step 1: Add `nodeCountAtLastError` to StackHead

**File**: `stack.go`

1. Add field to `StackHead`:
   ```go
   type StackHead struct {
       node                  *StackNode
       status                StackStatus
       summary               StackSummary
       lastExternalToken     Subtree
       nodeCountAtLastError  uint32  // ← NEW
   }
   ```

2. Add `NodeCountSinceError` method:
   ```go
   func (s *Stack) NodeCountSinceError(version StackVersion) uint32 {
       head := &s.heads[version]
       if head.node == nil { return 0 }
       if head.node.nodeCount < head.nodeCountAtLastError {
           head.nodeCountAtLastError = head.node.nodeCount
       }
       return head.node.nodeCount - head.nodeCountAtLastError
   }
   ```

3. Update `nodeCountAtLastError` when entering error state — in `AddErrorCost`
   and/or wherever error recovery starts. Look at when C sets this
   (likely in `ts_stack_push` when pushing error nodes, or in
   `ts_parser__recover`). The key is: when a version enters error
   recovery, record the current node count so `NodeCountSinceError`
   resets to 0.

4. Propagate `nodeCountAtLastError` on `Split`/`ForkAtNode` — when
   a version forks, the child should inherit the parent's value.

### Step 2: Add `SwapVersions` to Stack

**File**: `stack.go`

```go
func (s *Stack) SwapVersions(v1, v2 StackVersion) {
    s.heads[v1], s.heads[v2] = s.heads[v2], s.heads[v1]
}
```

Trivial swap of StackHead entries.

### Step 3: Fix MaxCostDifference

**File**: `parser.go`

```go
MaxCostDifference = 16 * ErrorCostPerSkippedTree  // = 1600, matches C
```

### Step 4: Implement `versionStatus` and `compareVersions`

**File**: `parser.go`

```go
type errorStatus struct {
    cost              uint32
    nodeCount         uint32
    dynamicPrecedence int32
    isInError         bool
}

type errorComparison int

const (
    errorComparisonTakeLeft    errorComparison = iota
    errorComparisonPreferLeft
    errorComparisonNone
    errorComparisonPreferRight
    errorComparisonTakeRight
)

func (p *Parser) versionStatus(version StackVersion) errorStatus {
    cost := p.stack.ErrorCost(version)
    isPaused := p.stack.IsPaused(version)
    if isPaused {
        cost += ErrorCostPerSkippedTree
    }
    return errorStatus{
        cost:              cost,
        nodeCount:         p.stack.NodeCountSinceError(version),
        dynamicPrecedence: p.stack.DynamicPrecedence(version),
        isInError:         isPaused || p.stack.State(version) == 0, // ERROR_STATE = 0
    }
}

func (p *Parser) compareVersions(a, b errorStatus) errorComparison {
    // Rule 1: Non-error beats in-error
    if !a.isInError && b.isInError {
        if a.cost < b.cost { return errorComparisonTakeLeft }
        return errorComparisonPreferLeft
    }
    if a.isInError && !b.isInError {
        if b.cost < a.cost { return errorComparisonTakeRight }
        return errorComparisonPreferRight
    }

    // Rule 2: Cost comparison with node_count amplification
    if a.cost < b.cost {
        if (b.cost-a.cost)*(1+a.nodeCount) > MaxCostDifference {
            return errorComparisonTakeLeft
        }
        return errorComparisonPreferLeft
    }
    if b.cost < a.cost {
        if (a.cost-b.cost)*(1+b.nodeCount) > MaxCostDifference {
            return errorComparisonTakeRight
        }
        return errorComparisonPreferRight
    }

    // Rule 3: Equal cost — dynamic precedence tiebreak
    if a.dynamicPrecedence > b.dynamicPrecedence { return errorComparisonPreferLeft }
    if b.dynamicPrecedence > a.dynamicPrecedence { return errorComparisonPreferRight }

    return errorComparisonNone
}
```

### Step 5: Rewrite `condenseStack` to match C algorithm

**File**: `parser.go`

Replace the current `condenseStack` entirely:

```go
func (p *Parser) condenseStack() uint32 {
    minErrorCost := uint32(math.MaxUint32)

    for i := 0; i < p.stack.VersionCount(); i++ {
        vi := StackVersion(i)

        // Remove halted versions immediately
        if p.stack.IsHalted(vi) {
            p.stack.RemoveVersion(vi)
            i--
            continue
        }

        statusI := p.versionStatus(vi)
        if !statusI.isInError && statusI.cost < minErrorCost {
            minErrorCost = statusI.cost
        }

        // Compare against all prior versions
        for j := 0; j < i; j++ {
            vj := StackVersion(j)
            statusJ := p.versionStatus(vj)

            switch p.compareVersions(statusJ, statusI) {
            case errorComparisonTakeLeft:
                // j is decisively better — kill i
                p.stack.RemoveVersion(vi)
                i--
                j = i // break inner loop
            case errorComparisonPreferLeft, errorComparisonNone:
                // Try merge (requires same state)
                if p.stack.Merge(vj, vi) {
                    i--
                    j = i
                }
            case errorComparisonPreferRight:
                // i is better — merge or swap
                if p.stack.Merge(vj, vi) {
                    i--
                    j = i
                } else {
                    p.stack.SwapVersions(vi, vj)
                }
            case errorComparisonTakeRight:
                // i is decisively better — kill j
                p.stack.RemoveVersion(vj)
                i--
                j--
            }
        }
    }

    // Hard cap: remove from the end
    for p.stack.VersionCount() > MaxVersionCount {
        p.stack.RemoveVersion(StackVersion(MaxVersionCount))
    }

    // Resume paused version handling (existing logic if any)
    // ... (check if we have paused version handling — may need to port)

    return minErrorCost
}
```

**Note**: The return type changes from `void` to `uint32` (returns
`minErrorCost`). Check if the caller uses this — C uses it to track
minimum error cost. If we don't use it, can keep void.

### Step 6: Update Merge to handle Remove correctly

**File**: `stack.go`

The current `Merge` halts the source version. In the new `condenseStack`,
merged versions are removed by index (the loop adjusts indices after merge).
Verify that `Merge` + `RemoveVersion` vs `Merge` + halt/compact are
compatible. The C `ts_stack_merge` removes the source by calling
`ts_stack_remove_version` internally after merging links. Our `Merge`
just sets status to halted.

**Option A**: Keep our Merge as-is (halt source), and have condenseStack
call RemoveVersion on halted sources explicitly.

**Option B**: Change Merge to call RemoveVersion internally, matching C.
This is cleaner but changes the index math.

**Recommendation**: Option B — change Merge to remove the source version
rather than just halting it. This matches C behavior and simplifies
condenseStack's index management. When Merge returns true, the source
version is gone and indices shift down.

```go
func (s *Stack) Merge(target, source StackVersion) bool {
    // ... existing link copying and swap logic ...

    // Remove the source version (not just halt)
    s.RemoveVersion(source)
    return true
}
```

### Step 7: Write tests

**File**: `parser_test.go` or new `condense_stack_test.go`

1. **Unit test `compareVersions`**: Test all 5 comparison outcomes with
   various cost/node_count/precedence/isInError combinations.

2. **Unit test `condenseStack`**: Create stacks with known version
   configurations and verify correct pruning behavior.

3. **Integration test**: Run the 8 C++ timeout test cases with a
   reasonable timeout (e.g., 5s) and verify they complete.

4. **Regression test**: Run full corpus to verify no regressions.

### Step 8: Verify and adjust

After implementing, run:
1. `make test` — all existing tests pass
2. C++ corpus tests — all 8 timeouts should resolve
3. Full corpus — verify no regressions (should stay at 1511+ / 1619)

---

## Risk Assessment

**Risk: Merge behavior change breaking existing passes**

Changing Merge from halt-source to remove-source changes the semantics.
All callers of Merge need to be aware that indices shift. The main caller
is `condenseStack` which we're rewriting. Check for other callers.

**Risk: nodeCountAtLastError initialization**

Must be initialized correctly on version creation and updated on error.
If wrong, the amplification formula could prematurely kill good versions.
Start by testing with the 8 C++ timeouts and verifying they don't also
fail for wrong reasons.

**Risk: Comparison formula edge cases**

The `(cost_diff) * (1 + node_count)` multiplication could overflow for
large node counts. Use uint64 for the multiplication or add an overflow
guard. C uses `unsigned` which can overflow, but in practice node counts
stay manageable.

---

## Estimated Impact

- **Direct**: +8 tests (all C++ timeouts resolve)
- **Indirect**: Possibly +2-5 structural mismatch tests if better version
  pruning leads to correct GLR resolution in other languages
- **Performance**: Should actually IMPROVE parse time for ambiguous grammars
  since fewer versions are tracked

## Dependencies

- None — this is independent of qkj (NonTerminalAliasMap) and other beads
- Should be done on a feature branch and tested thoroughly before merge
