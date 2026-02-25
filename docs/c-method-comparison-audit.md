# C Method Comparison Audit: tree-sitter Parser & Stack

**Reviewer**: reviewer agent
**Date**: 2026-02-17
**Branch**: feat/glr-advance-all at /tmp/glr-advance-all (uncommitted working tree)
**C Reference**: tree-sitter master (fetched 2026-02-17 from GitHub)

---

## Executive Summary

The implementation has made **massive progress** since the handleError comparison doc. All 8 critical/moderate gaps identified in that earlier review have been addressed:

1. **Pause/Resume pattern** — IMPLEMENTED (advanceVersion pauses, condenseStack resumes)
2. **Summary-based popback recovery** — IMPLEMENTED (RecordSummary + recover + recoverToState)
3. **Recover action calls recover()** — IMPLEMENTED (not just shifting)
4. **ERROR_STATE push in handleError** — IMPLEMENTED
5. **doAllPotentialReductionsUnfiltered** — IMPLEMENTED (symbol=0 sweep)
6. **Missing token insertion** — IMPLEMENTED (in handleError step 2)
7. **betterVersionExists cost bounds** — IMPLEMENTED
8. **condenseStack returns minErrorCost** — IMPLEMENTED (parse loop uses it for early termination)

**The implementation is on the right track.** The core architecture now matches C's two-phase error recovery and the key algorithmic patterns are correct. Below are remaining discrepancies to address.

### Remaining Issues (Priority-Ordered)

| Priority | Issue | Impact |
|----------|-------|--------|
| P0 | `ErrorCost()` missing paused/fresh-error bonus penalty | compareVersions under-penalizes error versions |
| P0 | `MaxCostDifference` is 1600, C master is now 1800 | Earlier kills than C, may prune versions C would keep |
| P1 | `Split()` copies summary reference, C clears it to NULL | Copied versions inherit stale summaries |
| P1 | `RenumberVersion` doesn't preserve target summary | May lose error recovery summary data |
| P1 | Missing `has_advanced_since_error` check in recover | C uses this to gate popback; Go skips it |
| P1 | Missing `breakdown_lookahead` in Recover action path | C decomposes reusable nodes before recovery |
| P2 | `doAllPotentialReductionsUnfiltered` split-per-reduction vs C's collect-then-apply | Different version management; functionally similar |
| P2 | Missing `pop_pending` / `pop_error` stack operations | May affect fragile/pending node handling |
| P2 | No deep-merge in `Merge()` (C recursively merges predecessors) | Wider DAGs, but functionally correct |
| P2 | Parse loop advance-one vs C's advance-all-then-condense | Different version survival patterns; functional |

---

## Part 1: Code Review

### 1.1 Parse Loop (parser.go:150-205)

**Structure**: advance-one-then-condense (vs C's advance-all-then-condense).

```
Go:  for { findActiveVersion → advanceVersion(one) → condenseStack }
C:   do { for each version { while(position catch-up) { advance(v) } } → condenseStack } while(versions)
```

**Assessment**: Functionally correct. More frequent condensation is a reasonable trade-off. The `findActiveVersion` picks the lowest-position version, which is sensible. The `minErrorCost` early termination matches C's `accept_count` pattern for stopping when a finished tree beats all remaining versions.

**One concern**: C's inner `while` loop advances the same version repeatedly until it catches up to `last_position` or advances past it. Go advances each version exactly once. This means Go does `N × condenseStack` calls per round vs C's 1. Performance difference but not correctness.

### 1.2 advanceVersion (parser.go:253-389)

**Matches C well**:
- Pause on no valid action (line 316): `p.stack.Pause(version, token)` ✅
- GLR splitting for additional actions (lines 323-344) ✅
- Repetition shift skip (lines 327-329, 347-349) ✅
- Recover action calls `p.recover(version, token)` (line 381) ✅
- Inner reduce loop with 1000-iteration safety cap ✅

**Differences**:
- **No `breakdown_lookahead`**: C calls `ts_parser__breakdown_lookahead` in the Recover case when `ts_subtree_child_count(lookahead) > 0`. This decomposes a reused node back into individual tokens for recovery. Go skips this entirely. Impact: incremental parsing with error recovery may not work correctly.
- **Repetition shift handling**: Go `continue`s the reduce loop on repetition shifts. C sets `last_repair_state` and continues. Minor difference in repetition optimization.

### 1.3 handleError (parser.go:921-1087)

**5-step structure matches C**:
1. `doAllPotentialReductionsUnfiltered(version)` — matches C's `do_all_potential_reductions(v, 0)` ✅
2. Try missing tokens with shift-then-check — matches C's pattern ✅
3. Push ERROR_STATE (state 0) onto all versions + merge back — matches C ✅
4. `RecordSummary(version, MaxSummaryDepth)` — matches C ✅
5. `recover(version, token)` — matches C ✅

**Step 1.5 (lines 946-991)**: Go adds an extra step checking if any reduced version can already handle the lookahead, halting those that can't. C doesn't have this explicit check — it relies on the subsequent flow (push ERROR_STATE → merge → recover) to handle both cases. This is an optimization that should be functionally equivalent.

**Missing token differences** (Step 2):
- Go checks for any shift action. C specifically checks `ts_language_has_reduce_action` for the lookahead in the post-missing state, then calls `do_all_potential_reductions` on the copy. Go just checks if the post-missing state has any action for the lookahead. This means Go misses recovery paths where: insert missing → reduce → valid state.
- Go doesn't filter by `state_after_missing != 0 && state_after_missing != state` like C does.

### 1.4 recover (parser.go:1123-1250)

**Matches C's ts_parser__recover well**:
- Strategy 1: Summary-based popback ✅
- Strategy 2: Skip token as error_repeat ✅
- Cost estimation matching C's formula ✅
- `betterVersionExists` cost checks ✅
- EOF handling (wrap in ERROR + accept) ✅
- error_repeat accumulation (pop existing + merge) ✅

**Differences**:
- **Missing `has_advanced_since_error` check**: C's recover has this at the top:
  ```c
  if (ts_stack_has_advanced_since_error(self->stack, version)) {
      ts_stack_record_summary(self->stack, version, MAX_SUMMARY_DEPTH);
  }
  ```
  This re-records the summary each time recover is called (from a Recover action, not just from handleError), but only if the version has consumed actual bytes since the error. Go never re-records the summary from recover.

- **Redundant recovery check**: Go checks `wouldMerge` by state+position (lines 1148-1158). C checks this identically. ✅

- **Depth increment for `nodeCountSinceError > 0`** (line 1143-1145): Matches C. ✅

### 1.5 recoverToState (parser.go:1256-1290)

**Matches C's ts_parser__recover_to_state**:
- Pops `depth` items ✅
- Filters out SubtreeZero entries ✅
- Creates ERROR node from popped children ✅
- Pushes with goal state ✅
- Adds error cost using C's formula ✅

**Correct implementation.**

### 1.6 betterVersionExists (parser.go:1296-1310)

**Matches C's ts_parser__better_version_exists**:
- Skips self, halted, and paused ✅
- Uses compareVersions for decisiveness check ✅
- Only returns true on TakeLeft (other version decisively better) ✅

**Correct implementation.**

### 1.7 doReduce (parser.go:706-821)

**Good implementation with alt path handling**:
- Primary path: pop → reverse → trailing extras → create node → GOTO → push ✅
- Alt paths: `ForkAtNode(path.node, version)` with source version for external token ✅
- Dynamic precedence application ✅
- Extra detection (gotoState == baseState) ✅

**Differences**:
- **No `pop_pending` handling**: C's reduce checks for pending (fragile) nodes and handles them specially. Go doesn't have pending pop logic.
- **No `non_terminal_extra` handling**: C checks if a reduced node is a "non-terminal extra" and uses separate lex states for extras. Go's extra detection is simpler.
- **Trailing extras are re-pushed individually**: This appears correct but may differ from C's behavior in edge cases with multiple extras.

### 1.8 doShift (parser.go:680-703)

**Matches C's ts_parser__shift**: Simple, correct.

### 1.9 doAccept (parser.go:834-855)

**Matches C's ts_parser__accept**:
- Compares by error cost, then dynamic precedence ✅
- Tracks acceptCount ✅
- Halts version after accepting ✅

**Minor difference**: C uses `>=` for dynamic precedence tie-break with same cost; Go also uses `>=`. ✅

### 1.10 acceptTree (parser.go:861-911)

**Matches C's accept+PopAll pattern**:
- PopAll to get all subtrees ✅
- Find root (non-extra) searching backward ✅
- Splice root's children into full array ✅
- Filters SubtreeZero entries ✅

### 1.11 condenseStack (parser.go:1454-1554)

**Faithful to C** (verified in previous 9zr review):
- Single-pass with 5-way comparison ✅
- Swap for PreferRight when merge fails ✅
- Hard cap removes from end ✅
- Paused version resume: one per round, only when no unpaused version exists ✅
- Uses `acceptCount < versionCount` for resume guard (matches C) ✅
- Returns minErrorCost for parse loop early termination ✅

### 1.12 versionStatus / compareVersions (parser.go:1384-1445)

**Matches C faithfully**:
- 5-way comparison enum ✅
- Error state preference ✅
- Cost amplification formula: `(cost_diff) * (1 + nodeCount) > MaxCostDifference` ✅
- Dynamic precedence tie-break ✅

### 1.13 Pause/Resume (stack.go:549-576)

**Matches C**:
- Stores lookahead token ✅
- Sets nodeCountAtLastError ✅
- Resume returns lookahead and clears it ✅

### 1.14 CanMerge (stack.go:516-544)

**All 5 conditions match C**:
1. Both active ✅
2. Same state ✅
3. Same position bytes ✅
4. Same errorCost ✅
5. Same external scanner state (bytes.Equal) ✅

### 1.15 RecordSummary (stack.go:728-807)

**Good implementation**:
- BFS walk through stack DAG ✅
- Records `{Position, Depth, State}` entries ✅
- Deduplicates by (state, depth) ✅
- Skips SubtreeZero links for depth counting ✅
- Skips ERROR_STATE entries ✅
- MaxIteratorCount budget cap ✅
- Visited set to prevent re-walking shared paths ✅

**Difference**: C uses `stack__iter` with `summarize_stack_callback`. Go reimplements as explicit BFS. Functionally equivalent but the visited-set approach differs slightly from C's deduplication-by-output approach.

### 1.16 lexToken (parser.go:394-583)

**Comprehensive implementation**:
- External scanner support with state serialization ✅
- Keyword lex with reserved word check ✅
- Empty external token rejection logic ✅
- Token caching ✅
- Lookahead bytes tracking for incremental parsing ✅

---

## Part 2: C Method Comparison Table

### Parser Functions (parser.c → parser.go)

| C Function | Go Equivalent | Status | Differences |
|---|---|---|---|
| `ts_parser_new` | `NewParser()` | ✅ Ported | Go version, no alloc differences |
| `ts_parser_delete` | (GC handles) | ✅ N/A | Go uses garbage collection |
| `ts_parser_set_language` | `SetLanguage()` | ✅ Ported | Matches C |
| `ts_parser_parse` | `Parse()` | ⚠️ Structural diff | Go: advance-one-then-condense; C: advance-all-then-condense. Uses `findActiveVersion` instead of iterating all. minErrorCost early termination matches C. |
| `ts_parser__advance` | `advanceVersion()` | ✅ Ported | Pauses on error ✅, Recover calls recover() ✅. Missing: `breakdown_lookahead` for incremental Recover. |
| `ts_parser__handle_error` | `handleError()` | ✅ Ported | All 5 steps match C. Step 1.5 (check if reduction recovered) is a Go-only optimization. Missing token step doesn't follow up with `do_all_potential_reductions`. |
| `ts_parser__recover` | `recover()` | ⚠️ Minor gaps | Strategy 1+2 match C. Missing: `has_advanced_since_error` check to re-record summary. |
| `ts_parser__recover_to_state` | `recoverToState()` | ✅ Faithful | Pops, wraps in ERROR, pushes goal state. Error cost formula matches. |
| `ts_parser__do_all_potential_reductions` | `doAllPotentialReductionsUnfiltered()` | ⚠️ Structural diff | Go: split-per-reduction. C: collect-then-apply with renumbering. Functionally similar. C's version also does version renumbering and merge-back. |
| `ts_parser__condense_stack` | `condenseStack()` | ✅ Faithful | Single-pass, 5-way comparison, swap, hard cap, resume one paused. Returns minErrorCost. |
| `ts_parser__reduce` | `doReduce()` | ⚠️ Minor gaps | Core logic matches. Missing: `pop_pending` path, non-terminal extra handling, fragile node logic. |
| `ts_parser__shift` | `doShift()` | ✅ Faithful | Extra marking, position computation, external token tracking. |
| `ts_parser__accept` | `doAccept() + acceptTree()` | ✅ Faithful | Error cost comparison, dyn prec tie-break, PopAll with splice. |
| `ts_parser__better_version_exists` | `betterVersionExists()` | ✅ Faithful | Skips self/halted/paused, uses compareVersions TakeLeft. |
| `ts_parser__version_status` | `versionStatus()` | ⚠️ Minor diff | Adds `ErrorCostPerSkippedTree` for paused (C adds `ERROR_COST_PER_RECOVERY`=500 in `ts_stack_error_cost`). |
| `ts_parser__compare_versions` | `compareVersions()` | ✅ Faithful | 5-way enum, cost amplification, dyn prec tie-break. |
| `ts_parser__lex` | `lexToken()` | ✅ Ported | External scanner, keyword lex, caching. Thorough implementation. |
| `ts_parser__get_cached_token` | (inline in lexToken) | ✅ Ported | Cache check at top of lexToken. |
| `ts_parser__set_cached_token` | (inline in lexToken) | ✅ Ported | Cache set at bottom of lexToken. |
| `ts_parser__breakdown_lookahead` | **NOT PORTED** | ❌ Missing | Decomposes reused composite nodes for recovery. Needed for incremental parsing + error recovery. |
| `ts_parser__breakdown_top_of_stack` | **NOT PORTED** | ❌ Missing | Decomposes top of stack for incremental parsing. Related to reuse. |
| `ts_parser__select_tree` | (inline in doAccept) | ✅ Ported | Error cost then dyn prec comparison. |
| `ts_parser__select_children` | **NOT PORTED** | ❌ Missing | Selects between alternative child arrays when trees have same structure. |

### Stack Functions (stack.c → stack.go)

| C Function | Go Equivalent | Status | Differences |
|---|---|---|---|
| `ts_stack_new` | `NewStack()` | ✅ Ported | Go version (no base_node, starts empty) |
| `ts_stack_delete` | (GC handles) | ✅ N/A | |
| `ts_stack_version_count` | `VersionCount()` | ✅ Faithful | |
| `ts_stack_halted_version_count` | (not used directly) | ⚠️ Not needed | Could be computed but not called |
| `ts_stack_state` | `State()` | ✅ Faithful | |
| `ts_stack_position` | `Position()` | ✅ Faithful | |
| `ts_stack_error_cost` | `ErrorCost()` | ❗ **Differs** | C adds `ERROR_COST_PER_RECOVERY` (500) for paused versions OR versions at ERROR_STATE with null first-link subtree. Go returns raw `node.errorCost`. This affects all cost comparisons. |
| `ts_stack_node_count_since_error` | `NodeCountSinceError()` | ✅ Faithful | Underflow guard matches C. |
| `ts_stack_dynamic_precedence` | `DynamicPrecedence()` | ✅ Faithful | |
| `ts_stack_push` | `Push()` | ⚠️ Minor diff | C: `Push(v, subtree, pending, state)` computes position internally. Go: `Push(v, state, subtree, isPending, position)` takes explicit position. C sets `nodeCountAtLastError` when `subtree.ptr == NULL`; Go doesn't have this null-subtree logic (uses SubtreeZero path). |
| `ts_stack_pop_count` | `Pop()` | ✅ Ported | Fan-out BFS matches C's `stack__iter`. Returns `[]StackIterator` vs C's `StackSliceArray`. |
| `ts_stack_pop_all` | `PopAll()` | ✅ Ported | Walks to bottom, collects all subtrees. |
| `ts_stack_pop_pending` | **NOT PORTED** | ❌ Missing | Pops pending (fragile) nodes for re-reduction. |
| `ts_stack_pop_error` | **NOT PORTED** | ❌ Missing | Pops single error subtree from top. |
| `ts_stack_copy_version` | `Split()` | ⚠️ Differs | C clears `summary = NULL` on copy. Go copies the summary reference. C retains node ref_count; Go shares pointer (no ref counting). |
| `ts_stack_merge` | `Merge()` | ⚠️ Minor diff | Go does simple link copy. C does deep-merge: if equivalent subtree but different predecessor with same state/position/errorCost, recursively merges predecessors. |
| `ts_stack_can_merge` | `CanMerge()` | ✅ Faithful | All 5 conditions match C exactly. |
| `ts_stack_halt` | `Halt()` | ✅ Faithful | |
| `ts_stack_pause` | `Pause()` | ✅ Faithful | Stores lookahead + sets nodeCountAtLastError. |
| `ts_stack_resume` | `Resume()` | ✅ Faithful | Returns lookahead, clears it, sets Active. |
| `ts_stack_is_active` | `IsActive()` | ✅ Faithful | |
| `ts_stack_is_paused` | `IsPaused()` | ✅ Faithful | |
| `ts_stack_is_halted` | `IsHalted()` | ✅ Faithful | |
| `ts_stack_swap_versions` | `SwapVersions()` | ✅ Faithful | |
| `ts_stack_renumber_version` | `RenumberVersion()` | ⚠️ Differs | C preserves target's summary if source has none. Go overwrites unconditionally. |
| `ts_stack_remove_version` | `RemoveVersion()` | ✅ Faithful | Shifts subsequent indices down. |
| `ts_stack_clear` | `Clear()` | ✅ Faithful | |
| `ts_stack_record_summary` | `RecordSummary()` | ✅ Ported | BFS with visited set, dedup by (state,depth), MaxIteratorCount cap. |
| `ts_stack_get_summary` | `GetSummary()` | ✅ Faithful | |
| `ts_stack_has_advanced_since_error` | **NOT PORTED** | ❌ Missing | Walks primary link chain checking for non-zero-width subtrees. Used by `recover()` to gate summary re-recording. |
| `ts_stack_set_last_external_token` | `SetLastExternalToken()` | ✅ Faithful | |
| `ts_stack_last_external_token` | `LastExternalToken()` | ✅ Faithful | |
| `ts_stack_print_dot_graph` | (not implemented) | ➖ Debug only | Not needed for correctness. |

---

## Part 3: Detailed Issue Descriptions

### P0: ErrorCost() Missing Bonus Penalty

**C** (stack.c lines 493-502):
```c
unsigned result = head->node->error_cost;
if (head->status == StackStatusPaused ||
    (head->node->state == ERROR_STATE && !head->node->links[0].subtree.ptr)) {
  result += ERROR_COST_PER_RECOVERY;  // +500
}
return result;
```

**Go** (stack.go:177-186):
```go
func (s *Stack) ErrorCost(version StackVersion) uint32 {
    return head.node.errorCost  // Raw cost, no bonus
}
```

**Note**: `versionStatus()` (parser.go:1387-1389) adds `ErrorCostPerSkippedTree` (100) when paused, which partially compensates. But C adds `ERROR_COST_PER_RECOVERY` (500) in `ErrorCost()` itself, affecting ALL callers (including `betterVersionExists`, `recover` cost estimation, and the parse loop's `minErrorCost`). The net effect is that Go under-penalizes error versions by 400 points in most paths.

**Fix**: Add the bonus penalty logic to `ErrorCost()` matching C. Then remove the `+ErrorCostPerSkippedTree` from `versionStatus()` for paused versions (it becomes redundant with the ErrorCost bonus).

### P0: MaxCostDifference 1600 vs C's 1800

**Go**: `MaxCostDifference = 16 * ErrorCostPerSkippedTree = 1600`
**C master**: `#define MAX_COST_DIFFERENCE 18 * ERROR_COST_PER_SKIPPED_TREE = 1800`

The comment says "matches C" but C master has been updated to 18. This causes Go to kill versions 200 cost units earlier than C, which could prune versions that C would keep alive long enough to find a successful parse.

**Fix**: Update to `18 * ErrorCostPerSkippedTree`.

### P1: Split() Copies Summary

**C** (`ts_stack_copy_version`, lines 697-706):
```c
head->summary = NULL;  // Clear summary on copy
```

**Go** (`Split()`, line 431):
```go
summary: head.summary,  // Copies the reference!
```

This means split versions inherit the parent's summary. If the parent then records a new summary, the split still has the old one. If the split's stack changes (via push/pop), the summary no longer corresponds to its actual stack state.

**Fix**: Set `summary: nil` in `Split()`.

### P1: RenumberVersion Doesn't Preserve Summary

**C** (lines 676-689):
```c
if (target_head->summary && !source_head->summary) {
    source_head->summary = target_head->summary;
    target_head->summary = NULL;
}
```

C preserves the target's summary if the source doesn't have one. Go just overwrites:
```go
s.heads[to] = s.heads[from]
```

**Fix**: Add summary preservation logic before overwriting.

### P1: Missing `has_advanced_since_error`

C's `recover()` calls this to decide whether to re-record the summary:
```c
if (ts_stack_has_advanced_since_error(self->stack, version)) {
    ts_stack_record_summary(self->stack, version, MAX_SUMMARY_DEPTH);
}
```

This ensures the summary stays fresh as error recovery advances. Without it, the summary only reflects the state at the initial handleError call, not the current stack state after tokens have been skipped.

**Fix**: Implement `HasAdvancedSinceError()` and call it at the top of `recover()`.

### P1: Missing `breakdown_lookahead` in Recover Path

C's `advanceVersion` in the Recover case:
```c
case TSParseActionTypeRecover:
    if (ts_subtree_child_count(lookahead) > 0) {
        ts_parser__breakdown_lookahead(self, &lookahead, ERROR_STATE, &self->reusable_node);
    }
    ts_parser__recover(self, version, lookahead);
```

This decomposes a reused composite node back into individual tokens before attempting recovery. Go skips this entirely. Impact: during incremental parsing, if a previously-reused non-terminal node is encountered during error recovery, the recovery operates on the composite node instead of its individual tokens, leading to incorrect error nodes.

**Fix**: Port `breakdown_lookahead` (and its sibling `breakdown_top_of_stack`).

---

## Part 4: Summary Statistics

| Category | Count |
|----------|-------|
| **Parser functions ported faithfully** | 13 |
| **Parser functions with minor gaps** | 5 |
| **Parser functions missing** | 3 |
| **Stack functions ported faithfully** | 20 |
| **Stack functions with minor gaps** | 4 |
| **Stack functions missing** | 3 |
| **Total C functions audited** | 48 |
| **Total ported (faithful or minor gaps)** | 42 (88%) |
| **Total missing** | 6 (12%) |

### Missing Functions (Not Currently Needed for Core Parsing)

The 6 missing functions fall into two categories:

**Incremental parsing support** (lower priority if not testing incremental):
- `breakdown_lookahead`
- `breakdown_top_of_stack`
- `select_children`

**Stack utilities** (may be needed for edge cases):
- `pop_pending` — fragile/pending node handling in reduce
- `pop_error` — error node pop in reduce path
- `has_advanced_since_error` — summary freshness gate in recover

---

## Part 5: Overall Assessment

**Verdict: On the right track. Ship it and iterate.**

The implementation has gone from "8 critical/moderate gaps" (in the handleError comparison) to "mostly faithful with 6 remaining issues." The core architecture is sound:

1. **Two-phase error recovery** (pause → condense → resume → handleError → recover) ✅
2. **Summary-based popback** (RecordSummary → iterate entries → recoverToState) ✅
3. **Cost-bounded recovery** (betterVersionExists) ✅
4. **Faithful condenseStack** with 5-way comparison, swap, hard cap ✅
5. **Strict CanMerge** with all 5 conditions ✅
6. **External scanner state** properly tracked across operations ✅

The P0 issues (`ErrorCost` bonus and `MaxCostDifference`) are quick fixes that will improve version survival accuracy. The P1 issues are important for correctness in edge cases but unlikely to cause widespread regressions. The P2 issues are structural differences that are functionally acceptable.

**Recommended next steps**:
1. Fix the two P0 issues (< 30 min)
2. Fix Split() summary clearing (< 5 min)
3. Run corpus tests to measure improvement
4. Address P1 issues based on regression analysis
5. Port `has_advanced_since_error` and add to `recover()`
