# handleError: Side-by-Side Comparison with C

**Reviewer**: reviewer agent
**Date**: 2026-02-17
**Branch**: glr-advance-all at /tmp/glr-advance-all (commit 48caea2)

---

## Executive Summary

Our `handleError` is **structurally different** from C's `ts_parser__handle_error` + `ts_parser__recover` in 8 significant ways. The most impactful are:

1. **No pause/resume pattern** — error recovery runs inline, not deferred
2. **No summary-based popback recovery** — C's primary recovery strategy is completely missing
3. **Recover action just shifts** — C uses Recover to attempt state popback; we just push the token
4. **No ERROR_STATE transition** — C pushes ERROR_STATE onto the stack; we don't

These are likely the root cause of the 77 regressions across all languages.

---

## Architecture: How C Handles Errors

C has a **two-phase** error recovery architecture:

### Phase 1: Error Detection (in `ts_parser__advance`)
```
advance(version) → no valid action → PAUSE version with lookahead → return true
```
The version is paused, NOT handled immediately. Other versions continue advancing. condenseStack may kill the paused version entirely if a better version exists.

### Phase 2: Error Recovery (in `ts_parser__condense_stack` → `ts_parser__handle_error`)
```
condenseStack → no unpaused version exists → RESUME one paused version → handleError()
handleError:
  1. do_all_potential_reductions(version, symbol=0)   // try all reductions
  2. Try inserting missing tokens                      // with reduction follow-up
  3. Push ERROR_STATE onto ALL versions                // enter error state
  4. Merge new versions back into original             // consolidate
  5. Record stack summary                              // snapshot for popback
  6. Call recover(version, lookahead)                   // popback + skip
```

### Phase 3: Ongoing Recovery (in `ts_parser__advance` → Recover action)
```
advance(error_version) → Recover action → call recover(version, lookahead)
  recover:
    Strategy 1: Use summary to find previous valid state → pop to it
    Strategy 2: Skip token (wrap in ERROR), stay in ERROR_STATE
```

### Our Architecture (Single Phase)
```
advanceVersion → no valid action → handleError() IMMEDIATELY
handleError:
  1. doAllPotentialReductions(version, token)  // try all reductions
  2. Skip token (split, push ERROR)            // Strategy 2 only
  3. tryMissingTokens(version, token)          // insert missing
  4. Halt or last-resort skip                  // cleanup
```

---

## Difference 1: No Pause/Resume Pattern (CRITICAL)

### C Behavior
```c
// In ts_parser__advance — at the end, when no valid action:
ts_stack_pause(self->stack, version, lookahead);
return true;
```
The version becomes inactive. Other versions get to advance and condense. If another version succeeds, the paused version is simply removed by condenseStack without ever entering expensive error recovery.

condenseStack only resumes ONE paused version when NO active versions exist:
```c
// In condenseStack paused handling:
if (!has_unpaused_version && self->accept_count < MAX_VERSION_COUNT) {
    ts_stack_resume(self->stack, i);
    ts_parser__handle_error(self, i, ts_stack_resume_lookahead(self->stack, i));
    has_unpaused_version = true;
}
```

### Our Behavior
```go
// In advanceVersion — when no valid action:
return p.handleError(version, token)
```
Error recovery runs immediately, creating split versions (skip, missing tokens, reductions) BEFORE condenseStack has a chance to prune. This front-loads expensive recovery work that may be unnecessary.

### Impact
- In C, a GLR parse with 3 versions where 1 hits an error: the error version is paused, the other 2 advance, condense kills the error version → no recovery needed.
- In ours: the error version immediately creates 2-3 split versions via recovery, then condense has to deal with 5+ versions instead of 3.
- This causes version explosion in ambiguous grammars, leading to the 77 regressions.

---

## Difference 2: No Summary-Based Popback Recovery (CRITICAL)

### C Behavior
C has a `ts_stack_record_summary` function that records a snapshot of ALL previous states on the stack:
```c
// Called in handleError after pushing ERROR_STATE:
ts_stack_record_summary(self->stack, version, MAX_SUMMARY_DEPTH);
```

The summary is an array of `{state, position, depth}` entries — one for each state on the stack path. This is the key data structure for C's **primary** error recovery strategy.

`ts_parser__recover` uses this summary:
```c
// In recover — Strategy 1:
for (i = 0; i < summary->size; i++) {
    entry = summary[i];
    if (entry.state == ERROR_STATE) continue;
    if (entry.position == current_position) continue;

    // Check if the lookahead would be valid in this previous state
    if (ts_language_has_actions(language, entry.state, lookahead_symbol)) {
        // Pop back to that state, wrapping popped nodes in ERROR
        ts_parser__recover_to_state(self, version, entry.depth, entry.state);
        break;
    }
}
```

### Our Behavior
- `StackSummary` is defined as `{errorCost, nodeCount, dynamicPrecedence}` — aggregate metadata only, NOT a state history.
- No `RecordSummary` function is ever called.
- No `recover` function exists.
- **C's primary error recovery strategy — finding a valid previous state and popping back to it — is completely absent.**

### Impact
This is arguably the single most impactful missing feature. Without popback recovery:
- Error versions stay in error state indefinitely, accumulating skipped tokens
- The parser can never "snap back" to a valid parse state after an error
- All error recovery is limited to: skip token, insert missing token, or try reductions from current state
- In C, most error recoveries use popback — it's the mechanism that lets the parser continue normally after an error

### Our StackSummary vs C's StackSummary

| Field | C's StackSummary (per entry) | Our StackSummary |
|-------|------------------------------|------------------|
| state | ✅ StateID | ❌ Not present |
| position | ✅ Length (bytes + extent) | ❌ Not present |
| depth | ✅ unsigned | ❌ Not present |
| errorCost | ❌ | ✅ |
| nodeCount | ❌ | ✅ |
| dynamicPrecedence | ❌ | ✅ |

C's summary is a **stack snapshot** (array of historical states). Ours is **aggregate stats**. Completely different purpose.

---

## Difference 3: Recover Action Just Shifts (CRITICAL)

### C Behavior
When a version in ERROR_STATE encounters a token, the parse table contains a `Recover` action. In `ts_parser__advance`:
```c
case TSParseActionTypeRecover: {
    if (ts_subtree_child_count(lookahead) > 0) {
        ts_parser__breakdown_lookahead(self, &lookahead, ERROR_STATE, &self->reusable_node);
    }
    ts_parser__recover(self, version, lookahead);  // ← Full recovery!
    return true;
}
```
This calls `recover()` which:
1. Checks the summary for a previous state that can handle the lookahead → popback
2. Checks cost bounds (`better_version_exists`)
3. Skips the token if no popback possible

### Our Behavior
```go
case ParseActionTypeRecover:
    if tokenSymbol == SymbolEnd {
        p.stack.Halt(version)
        return false
    }
    p.doShift(version, action, token)  // ← Just shifts!
    return true
```
We just shift the token. No popback attempt, no cost checking, no wrapping in ERROR. The error version accumulates tokens without ever trying to recover to a valid state.

### Impact
In C, every time an error version encounters a token, it tries to recover. After skipping 2-3 tokens, it usually finds a valid state and pops back. In ours, the error version just keeps shifting tokens in ERROR_STATE forever, never recovering. This means:
- Error versions consume version slots indefinitely
- The parser produces large ERROR nodes instead of recovering
- Parse trees have much worse error recovery quality

---

## Difference 4: No ERROR_STATE Push (MODERATE)

### C Behavior
After trying reductions and missing tokens, `handleError` pushes ERROR_STATE (state 0) onto ALL relevant versions:
```c
for (StackVersion v = version; v < version_count;) {
    // ... try missing tokens on v ...
    ts_stack_push(self->stack, v, NULL_SUBTREE, false, ERROR_STATE);
    v = (v == version) ? previous_version_count : v + 1;
}
```
This transitions the version into error state, which triggers Recover actions in subsequent advances.

### Our Behavior
We never push ERROR_STATE. After `handleError`, the version is either:
- Halted (most paths)
- Has an ERROR node pushed at the CURRENT state (not ERROR_STATE)

### Impact
Without transitioning to ERROR_STATE:
- The parse table's Recover actions are never triggered for this version
- The version's `isInError` flag (used by `compareVersions`) may not be set correctly
- The version doesn't enter C's error recovery loop (advance → Recover → recover → skip/popback)

---

## Difference 5: doAllPotentialReductions Structure (MODERATE)

### C Behavior
```c
static bool ts_parser__do_all_potential_reductions(
    self, starting_version, lookahead_symbol) {
    // 1. Collect ALL reduce actions across all symbols into reduce_actions array
    // 2. Apply all reductions (each may create new version via ts_parser__reduce)
    // 3. For each new version: check merge, check shift capability
    // 4. Complex version management: renumber, remove, merge with existing
    // Key: lookahead_symbol=0 means check ALL symbols for reductions
}
```

When called from handleError with `symbol=0`:
- C checks ALL terminal symbols (1..token_count) for reduce actions
- Collects all reductions first, then applies them
- Uses `ts_parser__reduce` which handles the full reduction (pop children, create parent, push new state, handle fragile/extra flags)
- Complex version tracking: `starting_version` / `version` / `reduction_version` with renumbering

### Our Behavior
```go
func (p *Parser) doAllPotentialReductions(version StackVersion, lookahead Subtree) bool {
    // For each symbol → for each reduce action:
    //   Split → doReduce → check if new state can shift lookahead
    //   If yes: mark recovered, add error cost
    //   If no: halt test version
}
```

Key differences:
1. **We always check against the specific lookahead** — not "any symbol" like C does with symbol=0
2. **We split per-reduction** instead of collecting and applying all
3. **Our `doReduce`** may not match C's `ts_parser__reduce` in handling fragile nodes, non-terminal extras, etc.
4. **No version renumbering** — C uses renumbering to replace the original version with a reduction result

### Impact
Our approach tests fewer recovery paths (only ones that directly enable the lookahead, not intermediate reductions that might eventually enable it through further reductions).

---

## Difference 6: Missing Token Insertion (MODERATE)

### C Behavior
```c
// Inside handleError, for each version from do_all_potential_reductions:
for (missing_symbol = 1; missing_symbol < token_count; missing_symbol++) {
    state_after_missing = next_state(state, missing_symbol);
    if (state_after_missing == 0 || state_after_missing == state) continue;

    // Key: Checks if a REDUCE action exists for the lookahead in the post-missing state
    if (ts_language_has_reduce_action(language, state_after_missing, lookahead)) {
        copy_version → push missing token → do_all_potential_reductions(copy, lookahead)
        // Only keeps if the copy can shift the lookahead AFTER reductions
    }
}
```

C does missing token insertion BEFORE pushing ERROR_STATE, and follows each insertion with `do_all_potential_reductions` to chase through reductions.

### Our Behavior
```go
func (p *Parser) tryMissingTokens(version StackVersion, lookahead Subtree) bool {
    for sym := 0; sym < SymbolCount; sym++ {
        // Check if shifting this missing symbol is valid
        entry := tableEntry(state, sym)
        if entry.Actions[0].Type != ParseActionTypeShift { continue }
        // Split → shift missing → check if new state can shift lookahead
    }
}
```

Differences:
1. **No follow-up reductions** — C calls `do_all_potential_reductions` after inserting the missing token. We just check if the post-missing state can directly shift the lookahead.
2. **C checks for reduce actions** specifically (`has_reduce_action`), not just any action.
3. **C only tries missing tokens where the resulting state differs** from the current state and isn't state 0.
4. **Strategy ordering**: C does missing tokens BEFORE skip; we do skip BEFORE missing tokens.

### Impact
We miss recovery paths where: insert missing → reduce → valid state. Only direct insert → shift paths are found.

---

## Difference 7: No `better_version_exists` Cost Bounds (MINOR)

### C Behavior
`ts_parser__recover` checks cost bounds before pursuing recovery:
```c
unsigned new_cost = current_error_cost + entry.depth * ERROR_COST_PER_SKIPPED_TREE + ...;
if (ts_parser__better_version_exists(self, version, false, new_cost)) break;
```

And before skipping:
```c
new_cost = current_error_cost + ERROR_COST_PER_SKIPPED_TREE + ...;
if (ts_parser__better_version_exists(self, version, false, new_cost)) {
    ts_stack_halt(self->stack, version);
    return;
}
```

### Our Behavior
No equivalent. Recovery paths are pursued regardless of cost.

### Impact
Minor — condenseStack will eventually prune costly versions anyway. But C's approach is more efficient by avoiding creating versions that will immediately be pruned.

---

## Difference 8: Parse Loop Structure (MODERATE)

### C Behavior
```c
do {
    for (version = 0; version < version_count; version++) {
        while (ts_stack_is_active(self->stack, version)) {
            ts_parser__advance(self, version, allow_node_reuse);
            if (position > last_position || (version > 0 && position == last_position)) {
                break;
            }
        }
    }
    ts_parser__condense_stack(self);
} while (version_count != 0);
```

C advances ALL versions in order, with an inner while loop to catch up to `last_position`, then condenses once.

### Our Behavior
```go
for {
    version := p.findActiveVersion()  // pick lowest-position version
    p.advanceVersion(StackVersion(version))  // advance once
    p.condenseStack()  // condense
}
```

We advance ONE version, then condense. This condenses much more frequently than C (after every single advance vs once per round).

### Impact
More frequent condensation means more comparisons and pruning. This could be beneficial (prune bad versions sooner) or harmful (may prune versions that would have been useful if advanced further). The main effect is different version survival patterns compared to C.

---

## Summary: Priority-Ordered Fixes

| Priority | Difference | Impact | Difficulty |
|----------|-----------|--------|------------|
| P0 | #1: No pause/resume | Version explosion, wasted recovery work | Medium |
| P0 | #2: No summary-based popback | Primary recovery strategy missing | High |
| P0 | #3: Recover action just shifts | Error versions never recover | Medium |
| P1 | #4: No ERROR_STATE push | Recover actions never triggered | Low |
| P1 | #5: doAllPotentialReductions scope | Fewer recovery paths found | Medium |
| P1 | #6: Missing token follow-up reductions | Misses multi-step recoveries | Medium |
| P2 | #7: No cost bounds | Inefficient but correct | Low |
| P2 | #8: Parse loop structure | Different but functional | Low-Medium |

### Recommended Implementation Order

**Phase 1 (Unblock error recovery)**:
1. Add `Pause` in `advanceVersion` when no valid action exists (instead of calling handleError)
2. In condenseStack's paused handling, call `handleError` on the resumed version
3. In `handleError`, push ERROR_STATE after trying reductions/missing tokens
4. In `advanceVersion`'s Recover case, call a proper `recover()` function instead of just shifting

**Phase 2 (Port summary-based popback)**:
5. Implement C-style `StackSummary` as array of `{state, position, depth}` entries
6. Implement `RecordSummary` to snapshot the stack when entering error state
7. Implement `recover()` with summary-based popback (Strategy 1) and skip (Strategy 2)
8. Implement `betterVersionExists` cost bounds

**Phase 3 (Match C precisely)**:
9. Refactor `doAllPotentialReductions` to match C's collect-then-apply pattern
10. Add follow-up reductions in missing token insertion
11. Align parse loop to advance-all-in-order (if needed after Phase 1-2)

---

## Appendix: C Function Call Graph for Error Recovery

```
ts_parser__advance()
    → no valid action: ts_stack_pause(version, lookahead); return
    → Recover action: ts_parser__recover(version, lookahead); return

ts_parser__condense_stack()
    → resume paused version
    → ts_parser__handle_error(version, lookahead)

ts_parser__handle_error(version, lookahead)
    → ts_parser__do_all_potential_reductions(version, 0)
    → try inserting missing tokens (with do_all_potential_reductions follow-up)
    → push ERROR_STATE onto all versions
    → merge new versions back
    → ts_stack_record_summary(version, MAX_SUMMARY_DEPTH)
    → ts_parser__recover(version, lookahead)

ts_parser__recover(version, lookahead)
    → Strategy 1: for each summary entry:
        → check lookahead valid in entry.state
        → ts_parser__recover_to_state(version, depth, state)  // popback
    → Strategy 2: skip lookahead token (wrap in ERROR, stay in ERROR_STATE)
    → cost checks via ts_parser__better_version_exists
```
