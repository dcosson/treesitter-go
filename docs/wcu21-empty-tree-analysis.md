# wcu.21: Empty/Nil Parse Tree Root Cause Analysis

**Date**: 2026-02-17
**Bead**: tree-sitter-go-wcu.21
**Branch**: main (f74d615), investigation worktree at /tmp/wcu21-empty-trees

## Summary

5 corpus tests produce completely nil parse trees (parser returns nil instead of a tree with ERROR nodes). The root cause is a fundamental semantic difference between Go's `Stack.Pop` and C's `ts_stack_pop_count`:

- **C**: `ts_stack_pop_count` creates **new versions** (via `ts_stack__add_version`) and leaves the original version untouched.
- **Go**: `Stack.Pop` modifies the version **in place** (line 426: `head.node = results[0].node`).

This breaks `recover()` → `recoverToState()` flow: after recovery pops back to a goal state, Strategy 2 (token skipping) overwrites the recovery on the same version, destroying the parser's only viable path.

## Affected Tests (5 nil trees, excluding Category G)

| Test | Language | Category |
|------|----------|----------|
| `Alphabetical_infix_operators_split_across_lines` | JavaScript | J (ASI) |
| `Type_arguments_in_JSX` | TypeScript | I (grammar gap) |
| `switch_with_unnamed_pattern_variable` | Java | I (grammar gap) |
| `newline-delimited_strings` | Ruby | D (external scanner) |
| `comment` (Lua, long input only) | Lua | Wrong root |

Note: The bead described "19 tests" but that count included wrong-structure failures (15) and wrong-root failures (4). Only 5 produce truly nil trees. Category G (5 more nil/wrong-root tests: C/Typedefs, Go×2, Python/An_error_before, JS/Extra_complex_literals) overlaps with coder-1's wcu.22.

## Root Cause: Stack.Pop In-Place Modification

### The C Behavior (correct)

In C tree-sitter, `ts_stack_pop_count(stack, version, count)`:
1. Iterates through the stack DAG from `version`'s head
2. For each destination node reached after `count` links, calls `ts_stack__add_slice` → `ts_stack__add_version`
3. New versions are **appended** to the `heads` array
4. The **original version is never modified**
5. Returns `StackSliceArray` with `{subtrees, new_version}` pairs

This means after popping:
- Original version still points to its original stack head
- New version(s) point to the popped-to nodes
- The caller can work with new versions while the original is preserved

### The Go Behavior (buggy)

In Go, `Stack.Pop(version, count)`:
1. Iterates through the stack DAG (BFS through links)
2. Collects pop results
3. **Modifies the original version**: `head.node = results[0].node` (stack.go:426)
4. Returns results referencing the same version

This means after popping:
- Original version has been mutated to point to the popped-to node
- The caller cannot distinguish the "new" version from the original
- Any code that expected the original to be unchanged is broken

### Impact on Error Recovery

The `recover()` function implements two parallel strategies:

1. **Strategy 1 (popback)**: Call `recoverToState(version, depth, goalState)` to pop back to a previous state where the lookahead is valid. Creates an ERROR node wrapping skipped content.

2. **Strategy 2 (token skip)**: Skip the current lookahead token by pushing it as `error_repeat` with ERROR_STATE (state 0).

In C, these operate on **different versions**: Strategy 1 creates new version(s) from the pop, Strategy 2 operates on the unchanged original. Both paths survive in parallel (GLR-style), and cost-based comparison eventually prunes the worse one.

In Go, both strategies operate on the **same version**. When Strategy 1 succeeds (pops back to goalState and pushes ERROR node), Strategy 2 then overwrites this by pushing error_repeat with state 0 on top. The recovery is destroyed.

### Trace Evidence (JS/Alphabetical_infix_operators)

```
op=13: handleError at state=447, pos=18, lookahead=in
       → recover: skip 'in' token, push error_repeat at state 0
op=14: advanceVersion at state=0, pos=24
       → lexes ';', gets RECOVER action
       → recover: didRecover=true (recoverToState succeeded!)
       → BUT: then skips ';' token on same version, overwriting recovery
op=15: no active versions → nil tree
```

Before the fix, the recovered version was overwritten. After the split-before-pop fix, the parser survives past op=15 but `recoverToState` fails with state mismatches because the summary depth entries don't match actual stack depths (related to coder-1's Fix #4).

## Fix Approach

Two complementary fixes are needed:

### Fix 1: Split before Pop in recoverToState (this investigation)

```go
func (p *Parser) recoverToState(version StackVersion, depth uint32, goalState StateID) bool {
    // Split so original version stays in ERROR_STATE for Strategy 2
    recoveryVersion := p.stack.Split(version)
    if recoveryVersion < 0 {
        return false
    }
    results := p.stack.Pop(recoveryVersion, depth)
    // ... rest operates on recoveryVersion, not version
}
```

This preserves C semantics where the original version is unchanged.

### Fix 2: Push resets nodeCountAtLastError for SubtreeZero (coder-1's Fix #4)

In C, `ts_stack_push` has:
```c
if (subtree.ptr == NULL) {
    head->node_count_at_last_error = head->node->node_count;
}
```

When pushing SubtreeZero (as handleError does for ERROR_STATE), the node count at last error is reset. This affects the `nodeCountSinceError` calculation which adds +1 to depth in recover(). Without this reset, depth is over-counted, causing `recoverToState` to pop past the goal state.

### Fix 3 (longer term): Align Stack.Pop with C semantics

The most correct fix would be to make `Stack.Pop` create new versions like C does, rather than modifying in place. This would fix not just recoverToState but any other caller that assumes the original version is preserved after a pop. However, this is a larger change that needs careful audit of all Pop callers:

**Pop callers audit** (all in parser.go):

1. **`doReduce` (line 753)**: `Pop(version, childCount)` — **OK with in-place**. doReduce intentionally modifies the version (pops children, pushes new node back). Alt paths from merged stacks use `ForkAtNode` to create separate versions. This works correctly.

2. **`recover` Strategy 2 (line 1266)**: `Pop(version, 1)` — **OK with in-place**. Pops existing error_repeat to merge with new skipped token, then pushes back to same version. Intentional modification.

3. **`recoverToState` (line 1301)**: `Pop(version, depth)` — **BROKEN**. The caller (`recover`) expects the original version to stay in ERROR_STATE for Strategy 2 token skipping. Fix: split before pop.

**Conclusion**: Only `recoverToState` needs the split-before-pop fix. A broader `Pop` semantic change is not needed for correctness, but could be done for cleanliness in the future.

## Lua/comment: Separate Issue (Scanner Bug)

The Lua/comment test is not a nil-tree failure — it produces a tree but with the wrong root (flat nodes instead of `chunk` wrapper) and wrong content (block comments not recognized, causing cascading parse failures). This is a **Lua scanner bug**, not a parser core issue:

- Expected: `(chunk (comment ...) (comment ...) (function_call ...) ...)`
- Actual: `(comment ...) (comment ...) (identifier) (string ...) ...` — no `chunk` wrapper, block comments (`--[[...]]`) not recognized, `print(...)` becomes `(identifier) (string ...)`

The short 2-line comment input parses correctly. The failure only occurs with longer input containing Lua block comments (`--[[...]]`, `--[==[...]==]`). This suggests the Lua scanner's block comment recognition has edge cases with multi-level long brackets.

**Priority**: P3 — Lua scanner issue, not related to the Stack.Pop bug.

## Test Plan

After applying fixes:
1. Run all 5 affected tests individually to confirm non-nil trees
2. Run full corpus tests to check for regressions (some tests may shift between categories)
3. Compare failure counts against baseline (25 total failures on main)
4. Specifically verify that existing passing tests still pass (no regressions from the recovery changes)

## Coordination

- **coder-1** (wcu.22): Working on error recovery fixes at /tmp/error-recovery-fix. Has Fix #4 (Push nodeCountAtLastError reset) applied. The split-before-pop fix and Fix #4 are complementary.
- **Fix ownership**: coder-1 should apply the split-before-pop fix to their branch and test both together. The combined fix should resolve both wcu.21 and wcu.22 nil-tree failures.
