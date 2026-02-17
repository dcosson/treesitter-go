# UMS Plan vs Implementation Audit

**Date**: 2026-02-16
**Author**: reviewer agent
**Requested by**: dcosson via concierge

## Summary

The 8-step plan in `docs/ums-glr-timeout-analysis.md` was **partially implemented**.
Steps 1-4 were done faithfully. Step 5 (condenseStack rewrite) was done **differently
from the plan** — using a phased approach that omits key behaviors. Step 6 was not
done. Steps 7-8 were partially done. A round-robin mechanism **not in the plan** was
added to compensate for limitations of the partial Step 5 implementation.

**Result**: Timeouts are resolved (the primary goal), but structural ambiguity
resolution is significantly worse than the C reference parser. The round-robin fix
is a band-aid for a problem created by the incomplete condenseStack port.

---

## Step-by-Step Comparison

### Step 1: Add `nodeCountAtLastError` to StackHead ✅ DONE

**Plan**: Add field, add `NodeCountSinceError` method, update on error, propagate on Split.

**Implementation** (stack.go):
- Field added at line 86: `nodeCountAtLastError uint32`
- `NodeCountSinceError()` at line 204 — matches C reference exactly
- Updated in `AddErrorCost()` at line 600 — sets baseline on error
- Propagated in `Split()` at line 418 — inherited from parent

**Verdict**: Faithful to plan and C reference. No issues.

### Step 2: Add `SwapVersions` to Stack ✅ DONE

**Plan**: Simple swap of StackHead entries.

**Implementation** (stack.go line 224):
```go
func (s *Stack) SwapVersions(v1, v2 StackVersion) {
    s.heads[v1], s.heads[v2] = s.heads[v2], s.heads[v1]
}
```

**Verdict**: Implemented exactly as planned. However, **SwapVersions is never called**
anywhere in the codebase. It exists but is dead code. See Step 5.

### Step 3: Fix MaxCostDifference ✅ DONE

**Plan**: Change from 1800 to `16 * ErrorCostPerSkippedTree = 1600`.

**Implementation** (parser.go line 70):
```go
MaxCostDifference = 16 * ErrorCostPerSkippedTree // = 1600, matches C
```

**Verdict**: Done. Matches C reference.

### Step 4: Implement `versionStatus` and `compareVersions` ✅ DONE

**Plan**: Port `ts_parser__version_status` and `ts_parser__compare_versions` from C.

**Implementation** (parser.go lines 1182-1243):
- `versionStatus()` matches C exactly (cost + paused penalty, nodeCount, dynPrec, isInError)
- `compareVersions()` matches C exactly (3 rules: error state, cost amplification, dynPrec)
- **Improvement over plan**: Uses `uint64` for overflow protection in the amplification
  formula. The plan mentioned overflow risk; the implementation handles it.

**Verdict**: Faithful to plan and C reference. Well-tested (TestCompareVersions has
12 test cases covering all comparison paths).

### Step 5: Rewrite `condenseStack` to match C algorithm ⚠️ PARTIALLY DONE — KEY DIVERGENCE

**Plan**: Replace condenseStack with C's single-pass algorithm:
```
for each version i:
    remove if halted
    for each prior version j < i:
        compare(j, i) → TakeLeft/PreferLeft/None/PreferRight/TakeRight
        TakeLeft: kill i
        PreferLeft/None: try merge
        PreferRight: try merge, else SWAP
        TakeRight: kill j
    hard cap: remove from end
```

**What was actually implemented** (parser.go lines 1255-1386):

A **5-phase approach** instead of the single-pass algorithm:

| Phase | What it does | In C's algorithm? |
|-------|-------------|-------------------|
| Phase 1 | Merge same-state versions | Embedded in single-pass |
| Phase 2 | Pair-wise decisive kills only (TakeLeft/TakeRight) | Yes, but C also handles PreferLeft/Right/None |
| Phase 3 | Absolute cost threshold (`cost > bestCost + 1600`) | **NOT in C** — leftover from old code |
| Phase 4 | Hard cap with worst-version search | C removes from end, we search for worst |
| Phase 5 | Compact halted versions | Implicit in C's remove-by-index |

**5 critical differences from the plan:**

1. **No PreferLeft/PreferRight/None handling in Phase 2**: The plan's PreferLeft and
   None outcomes trigger merge attempts. PreferRight triggers merge-or-swap. In our
   implementation, these outcomes are **silently ignored**. Only TakeLeft/TakeRight
   (decisive kills) are acted on. The comment at line 1280-1282 explains:
   > "swaps change findActiveVersion tie-breaking, causing regressions"

2. **Same-position restriction on Phase 2**: Our Phase 2 only compares versions at
   the same byte position (line 1309). C compares ALL pairs regardless of position.
   This was added to fix an infinite loop regression where cross-position decisive
   kills prevented error recovery versions from ever catching up.

3. **SwapVersions never called**: Despite being implemented (Step 2), it's dead code.
   The PreferRight case that would call it is not handled.

4. **Phase 3 is vestigial**: The absolute cost threshold pruning is not in C's
   algorithm. It's kept from the pre-UMS code as a safety net. It's the ONLY
   mechanism for cross-position pruning (since Phase 2 is restricted to same-position).

5. **Phase 4 searches for worst vs C's remove-from-end**: C relies on swap ordering
   to ensure better versions are at lower indices, then removes from the end. Without
   swaps, our hard cap has to search for the worst version explicitly.

**Impact**: The decisive-kills-only approach effectively handles error recovery timeouts
(where versions have large cost differences that become decisive quickly). But it fails
on **structural ambiguity** where versions have equal or similar costs and differ only
by dynamic precedence. The soft preferences (PreferLeft/Right) with swaps are precisely
the mechanism C uses to resolve these.

### Step 6: Update Merge to handle Remove correctly ❌ NOT DONE

**Plan**: Change Merge from halt-source to remove-source (Option B recommended).

**Implementation**: Merge still halts the source (line 487: `sourceHead.status = StackStatusHalted`).
CompactHaltedVersions is called at the end of condenseStack (Phase 5).

**Verdict**: Not done. Lower priority since the phased approach works around it,
but it means index management is different from C.

### Step 7: Write tests ⚠️ PARTIALLY DONE

| Test | Planned | Done? |
|------|---------|-------|
| `compareVersions` unit test | Yes | ✅ TestCompareVersions (12 cases) |
| `condenseStack` unit test | Yes | ❌ Not done |
| C++ timeout integration tests | Yes | ✅ Via corpus tests |
| Regression test (full corpus) | Yes | ✅ 1519/100/1619 |
| `findActiveVersion` round-robin | Not planned | ✅ TestFindActiveVersionRoundRobin |

### Step 8: Verify and adjust ✅ DONE

- All existing tests pass
- C++ timeouts mostly resolved (6 of 8 fixed, 2 had pre-existing structural issues)
- Full corpus: 1519 pass / 100 fail / 1619 total (93.9%)

---

## The Round-Robin Band-Aid

### What it is

`findActiveVersion()` (parser.go line 233) now includes starvation detection:
after `maxStaleSelections=4` consecutive selections of the same version without
position progress, it rotates to the next active version.

Three new fields on Parser: `lastActiveVersion`, `lastActivePosition`,
`staleSelectionCount`.

### Why it was needed

The phased condenseStack creates a problem the C single-pass doesn't have:

1. **C's approach**: In the single-pass, PreferRight triggers a swap, moving the
   better version to a lower index. findActiveVersion picks the lowest-position
   version, and with swaps, this naturally cycles through versions because the
   "best" version keeps getting swapped to lower indices.

2. **Our approach**: Without swaps, a low-position error recovery version that
   never advances can monopolize findActiveVersion forever. The decisive-kills-only
   Phase 2 can't help because:
   - If versions are at different positions: Phase 2 skips them (same-position restriction)
   - If cost amplification isn't decisive yet: nothing happens
   - The better version's nodeCount never grows because it never gets selected

3. **Result without round-robin**: Parser loops 496K+ times selecting the same stuck
   version before the 10s timeout. Seen in Complex_fold_expression (3 versions,
   v2 at pos=53 always selected over v0/v1 at pos=58).

### Is it a band-aid?

**Yes**. The round-robin is compensating for missing swap behavior in condenseStack.
If condenseStack properly implemented PreferRight → swap, the better version would
naturally get promoted to a lower index and eventually selected. The round-robin
wouldn't be needed.

However, the round-robin is a **safe** band-aid — it can't cause incorrect results
(it just gives another version a turn), and it prevents a real class of infinite loops.
The proper fix is completing the condenseStack port.

---

## Current Bugs Caused by Incomplete Port

### Go call_expression vs type_conversion_expression (bead nlb)

**Symptom**: `T(x)`, `(*Point)(p)`, `e.f(g)`, `(e.f)(g)` all parse as
type_conversion_expression instead of call_expression, despite call_expression
having higher dynamic precedence (0 vs -1).

**Root cause**: When multiple ambiguous statements are in the same function body,
the parser merges versions at each statement boundary. The merge correctly puts
the higher-dynPrec (call) subtree in link[0]. But this correct ordering is never
USED to actually resolve the ambiguity:

- In C: PreferRight/PreferLeft soft preferences, combined with the single-pass
  structure, influence which version survives to accept. The swap operation ensures
  the preferred interpretation gets priority.
- In ours: All soft preferences are ignored. The merge link ordering is correct but
  has no effect because at accept time, the competing trees have identical total
  dynPrec (-6 for all paths) due to how merge flattening works through PopAll.

**My debug trace** shows: with a single `a(b)` statement, dynPrec correctly resolves
(call dp=0 beats type_conv dp=-2). With 4 ambiguous statements, all accepted trees
have dp=-6 — the dynPrec discrimination is lost.

### Structural mismatches generally

Of the 100 failing corpus tests:
- ~8 are timeouts (version explosion, some still unsolved for C++ GLR-heavy inputs)
- ~92 are structural mismatches where the parser produces a valid tree but picks
  the wrong interpretation

Many of these structural mismatches involve ambiguity resolution where C would use
soft preferences + swaps to pick the correct alternative. Our decisive-kills-only
approach can't distinguish between equally-costed alternatives.

---

## Path to Completing the Faithful C condenseStack Port

### Option A: Complete the single-pass port (recommended)

Replace the 5-phase condenseStack with the C single-pass algorithm as originally
planned in Step 5. This means:

1. Handle PreferLeft/None: try merge (already done in Phase 1, just needs to
   be integrated into the comparison loop)
2. Handle PreferRight: try merge, else swap. **This is the key missing piece.**
   The swap will reorder versions so better interpretations have priority.
3. Remove Phase 3 (vestigial absolute threshold) — the pair-wise comparison
   subsumes it
4. Change Phase 4 to remove-from-end (relies on swap ordering)
5. Remove the same-position restriction on Phase 2
6. Test whether round-robin is still needed (it might not be with proper swaps)

**Risk**: The comment says "swaps change findActiveVersion tie-breaking, causing
regressions." This needs investigation — what specific test regressed? Was it
a real regression or a test that was passing for the wrong reasons?

### Option B: Keep phased approach, add soft preference handling

Keep the 5-phase structure but enhance Phase 2:

1. Handle PreferLeft/PreferRight in Phase 2 (not just TakeLeft/TakeRight)
2. For PreferRight: add swap call
3. Carefully test each change against the full corpus
4. May need to lift the same-position restriction for soft preferences
   (only keep it for decisive kills)

**Risk**: More incremental but may introduce subtle ordering differences
from C that compound across phases.

### Investigation needed before either option

1. **Why did swaps cause regressions?** Find the specific test(s) that broke.
   Was it a real regression or a test that was coincidentally passing?
2. **Can we add soft preferences without swaps?** Test if just handling
   PreferLeft/Right merge attempts (without swap) improves structural accuracy.
3. **Round-robin interaction**: Will swaps make round-robin unnecessary, or
   do they need to coexist?

---

## Summary Table

| Step | Plan | Status | Notes |
|------|------|--------|-------|
| 1 | nodeCountAtLastError | ✅ Done | Matches C exactly |
| 2 | SwapVersions | ✅ Done | But never called (dead code) |
| 3 | Fix MaxCostDifference | ✅ Done | 1600 matches C |
| 4 | compareVersions | ✅ Done | Matches C + overflow protection |
| 5 | Rewrite condenseStack | ⚠️ Partial | Phased, decisive-kills-only, no swaps |
| 6 | Update Merge | ❌ Not done | Still halts instead of removes |
| 7 | Write tests | ⚠️ Partial | compareVersions tested, condenseStack not |
| 8 | Verify | ✅ Done | 1519/100/1619 (93.9%) |
| — | Round-robin (unplanned) | ✅ Done | Band-aid for missing swaps |
