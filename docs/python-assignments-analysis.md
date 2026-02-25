# Python/Assignments Investigation — coder-1

## Status: In Progress

## The Flat Loop IS Non-Destructive (Checklist Item #1 Verified)

The flat loop at parser.go:338-380 DOES use split-then-reduce:

```go
case ParseActionTypeReduce:
    splitVersion := p.stack.Split(version)  // Non-destructive: creates copy
    if splitVersion < 0 {
        continue
    }
    p.doReduce(splitVersion, action, nullLookahead)  // Only modifies the SPLIT
    didReduce = true
    lastReductionVersion = splitVersion
```

The original `version` is never modified by doReduce. This matches C's non-destructive
`ts_stack_pop_count`. ✅

## Key Difference: C's Inline Merge vs Go's Deferred Merge

**C's ts_parser__reduce** (parser.c:1033-1039) does INLINE MERGING after each reduce:
```c
// After push, try to merge with every earlier version
for (StackVersion j = 0; j < slice_version; j++) {
    if (j == version) continue;
    if (ts_stack_merge(self->stack, j, slice_version)) {
        removed_version_count++;
        break;
    }
}
```

This means: when reduce 2 (list_splat_pattern) creates a new version that reaches the
same state as reduce 1 (list_splat), **C immediately merges them inside ts_parser__reduce**.

**Go's doReduce** does NO inline merging. Both split versions survive independently.
Merging only happens later during `condenseStack`.

## C Reduce Return Value vs Go

**C**: `ts_parser__reduce` returns `STACK_VERSION_NONE` when ALL new versions merged
into existing ones. Only non-NONE returns update `last_reduction_version`.

**Go**: `doReduce` is void. `lastReductionVersion` is ALWAYS set to `splitVersion`,
even if that version was merged or halted during doReduce.

This matters when reduce 2 merges into reduce 1:
- **C**: reduce 2 returns NONE → `last_reduction_version` = reduce 1's version
- **Go**: reduce 2 sets `lastReductionVersion` = splitVersion2 (which still exists independently)

## What Happens for Python/Assignments

Two reduces at the same parse state:
1. REDUCE list_splat (sym=149)
2. REDUCE list_splat_pattern (sym=184)

Both reductions pop the same children and push to the SAME goto state.

### C flow:
1. reduce(version=0, list_splat) → creates version V1, pushes to state S
2. reduce(version=0, list_splat_pattern) → creates version V2, pushes to state S
   - Inline merge: `ts_stack_merge(V1, V2)` succeeds → V2 merged INTO V1
   - nodeAddLink Case 1 or Case 2 fires, keeping both subtrees as alternatives
   - Return NONE (all merged)
3. `last_reduction_version = V1` (from step 1)
4. `renumber(V1, 0)` → version 0 now has the MERGED node (both alternatives)

### Go flow:
1. Split(0)→1, doReduce(1, list_splat) → version 1 at state S
2. Split(0)→2, doReduce(2, list_splat_pattern) → version 2 at state S
3. NO inline merge — both versions survive independently
4. `lastReductionVersion = 2`
5. `RenumberVersion(2, 0)` → version 0 = list_splat_pattern, version 1 = list_splat
6. Later: condenseStack merges them... but version 0 is now list_splat_pattern

Wait — this should actually be correct for the version ordering. Let me check what
condenseStack does with the merge direction...

## TODO
- [ ] Add trace logging to see exact reduce/merge flow for Python/Assignments
- [ ] Check condenseStack merge direction vs C's inline merge direction
- [ ] Check if the issue is in doReduce's merge behavior or later condenseStack
- [ ] Verify that RenumberVersion handles correctly when splitVersion was already merged
