# C vs Go Mismatches Checklist

**Date**: 2026-02-25
**Branch**: debug/assignments-trace (HEAD: 5bb7b28)
**Status**: 11 corpus failures remaining

Comprehensive list of known architectural and behavioral differences between
C tree-sitter and our Go port. Checked items are fixed and committed.

## Naming Convention

Go functions should match C function names converted to CamelCase.
This makes cross-referencing and auditing trivial.

### Parser Functions (C: 50 in parser.c, Go: 33 in parser.go)

| C Name (parser.c) | Line | Go Name (parser.go) | Line | Status |
|---|---|---|---|---|
| ts_parser_new | 1935 | NewParser | 74 | MISMATCH — no wasm/included-range init |
| ts_parser_delete | 1958 | — | — | N/A (GC) |
| ts_parser_set_language | 1988 | SetLanguage | 91 | MISMATCH — no reset/ABI/wasm handling |
| ts_parser_language | 1984 | Language | 100 | OK |
| ts_parser_reset | 2047 | Reset | 105 | MISMATCH — no lexer/scanner/opts reset |
| ts_parser_parse | 2074 | Parse | 123 | MISMATCH — no wasm/included ranges/balancing |
| ts_parser_parse_string | 2215 | ParseString | 194 | MISMATCH — no encoding/included ranges |
| ts_parser__advance | 1557 | advanceVersion | 231 | MISMATCH — flat loop added but inline merge missing |
| ts_parser__reduce | 931 | doReduce | 794 | MISMATCH — no inline merge, no return value |
| ts_parser__shift | 908 | doShift | 756 | OK |
| ts_parser__accept | 1048 | doAccept | 961 | FIXED (43ee822) |
| ts_parser__lex | 505 | lexToken | 409 | MISMATCH — no error-mode lex loop/included ranges |
| ts_parser__get_cached_token | 703 | — (inline in lexToken?) | — | MISMATCH — cache ignores last ext token/reuse |
| ts_parser__set_cached_token | 724 | — (inline in lexToken?) | — | MISMATCH — caches token only (no ext token) |
| ts_parser__handle_error | 1439 | handleError | 1129 | FIXED (3da71fa) |
| ts_parser__recover | 1250 | recover | 1384 | MISMATCH — Strategy 2 multi-path |
| ts_parser__recover_to_state | 1191 | recoverToState | 1560 | FIXED (8ce6698) |
| ts_parser__condense_stack | 1766 | condenseStack | 1826 | MISMATCH — no paused minErrorCost update |
| ts_parser__do_all_potential_reductions | 1101 | doAllPotentialReductions | 1249 | FIXED (de8d8b8) |
| ts_parser__select_tree | 836 | selectTree | 985 | OK |
| ts_parser__select_children | 883 | selectChildren | 932 | OK |
| ts_parser__breakdown_top_of_stack | 176 | — | — | MISSING — no pending stack breakdown |
| ts_parser__breakdown_lookahead | 224 | breakdownLookahead | 739 | OK |
| ts_parser__compare_versions | 246 | compareVersions | 1779 | OK |
| ts_parser__version_status | 289 | versionStatus | 1760 | MISMATCH — paused cost not added |
| ts_parser__better_version_exists | 304 | betterVersionExists | 1667 | MISMATCH — no finished tree/pos/merge checks |
| ts_parser__can_reuse_first_leaf | 470 | — (inline?) | — | MISMATCH — reuse checks incomplete |
| ts_parser__reuse_node | 753 | tryReuseNode | 643 | MISMATCH — no range diffs/breakdown_top |
| ts_parser__check_progress | 1536 | — | — | MISSING — no progress callback checks |
| ts_parser__balance_subtree | 1867 | — | — | MISSING — no subtree balancing |
| ts_parser__log | 157 | — | — | MISSING — no parser log/dot graph |
| ts_parser__call_main_lex_fn | 341 | — (inline?) | — | MISMATCH — no wasm lex dispatch |
| ts_parser__call_keyword_lex_fn | 349 | — (inline?) | — | MISMATCH — no wasm keyword lex |
| ts_parser__external_scanner_create | 357 | — | — | MISMATCH — no scanner create hook |
| ts_parser__external_scanner_destroy | 374 | — | — | MISMATCH — no scanner destroy hook |
| ts_parser__external_scanner_serialize | 390 | externalScannerSerialize | 1925 | MISMATCH — no wasm scanner support |
| ts_parser__external_scanner_deserialize | 413 | externalScannerDeserialize | 1935 | MISMATCH — no wasm scanner support |
| ts_parser__external_scanner_scan | 443 | — (inline?) | — | MISMATCH — no wasm scanner support |
| ts_parser__has_included_range_difference | 740 | — | — | N/A (no incremental yet) |
| ts_parser_has_outstanding_parse | 1924 | — | — | MISSING — no resume parse tracking |
| ts_string_input_read | 138 | — | — | OK (StringInput.Read) |

**Go-only functions (no C equivalent):**
| Go Name | Line | Notes |
|---|---|---|
| findActiveVersion | 210 | OK — helper for version selection |
| doReduceForPotential | 1362 | Split+reduce for error recovery — C does this inline |
| buildAcceptTree | 1077 | C logic is inline in ts_parser__accept |
| subtreeCompare | 1032 | OK — port of ts_subtree_compare |
| createErrorNode | 1684 | OK — helper for error node creation |
| createErrorRepeatNode | 1715 | OK — helper for error_repeat node |
| createMissingToken | 1722 | OK — helper for missing leaf |

### Stack Functions (C: 41 in stack.c, Go: 45 in stack.go)

| C Name (stack.c) | Line | Go Name (stack.go) | Line | Status |
|---|---|---|---|---|
| ts_stack_new | 421 | NewStack | 129 | ? |
| ts_stack_delete | 440 | — | — | N/A (GC) |
| ts_stack_version_count | 459 | VersionCount | 137 | ? |
| ts_stack_halted_version_count | 463 | — | — | ? missing? |
| ts_stack_state | 474 | State | 153 | ? |
| ts_stack_position | 478 | Position | 165 | ? |
| ts_stack_last_external_token | 482 | LastExternalToken | 890 | ? |
| ts_stack_set_last_external_token | 486 | SetLastExternalToken | 882 | ? |
| ts_stack_error_cost | 493 | ErrorCost | 177 | ? |
| ts_stack_node_count_since_error | 504 | NodeCountSinceError | 224 | ? |
| ts_stack_has_advanced_since_error | 647 | HasAdvancedSinceError | 245 | ? |
| ts_stack_dynamic_precedence | 643 | DynamicPrecedence | 211 | ? |
| ts_stack_push | — (inline) | Push | 310 | ? |
| ts_stack_pop_count | 534 | Pop / PopCount | 359/926 | **CRITICAL MISMATCH** — see below |
| ts_stack_pop_all | 597 | PopAll | 443 | FIXED (316754c) |
| ts_stack_pop_pending | 552 | PopPending | 513 | ? |
| ts_stack_pop_error | 575 | PopError | 537 | ? |
| ts_stack_merge | 708 | Merge | 737 | ? |
| ts_stack_can_merge | 722 | CanMerge | 768 | ? |
| ts_stack_halt | 734 | Halt | 831 | OK |
| ts_stack_pause | 738 | Pause | 801 | OK |
| ts_stack_resume | 757 | Resume | 817 | MISMATCH — Go does not assert status==Paused, uses if-check instead; functionally OK |
| ts_stack_is_active | 745 | IsActive | 295 | ? |
| ts_stack_is_halted | 749 | IsHalted | 305 | ? |
| ts_stack_is_paused | 753 | IsPaused | 300 | ? |
| ts_stack_remove_version | 671 | RemoveVersion | 840 | ? |
| ts_stack_renumber_version | 676 | RenumberVersion | 1076 | ? |
| ts_stack_swap_versions | 691 | SwapVersions | 279 | ? |
| ts_stack_copy_version | 697 | — | — | ? missing? |
| ts_stack_record_summary | 624 | RecordSummary | 980 | ? |
| ts_stack_get_summary | 639 | GetSummary | 1065 | ? |
| ts_stack_clear | 766 | Clear | 848 | ? |
| ts_stack_print_dot_graph | 780 | — | — | N/A (debug) |
| stack_node_retain | 82 | — | — | N/A (GC) |
| stack__subtree_node_count | 126 | subtreeNodeCount | 612 | ? |
| stack__subtree_is_equivalent | 181 | subtreeIsEquivalent | 628 | ? |
| stack_node_add_link (internal) | ~100 | nodeAddLink | 665 | ? |
| stack__iter (internal) | 324 | — | — | **MISSING** — core iterator, all pop funcs built on it |
| ts_stack__add_version (internal) | 286 | AddVersion | 854 | ? |
| ts_stack__add_slice (internal) | 304 | — | — | **MISSING** — groups paths by node, creates versions |

**Go-only functions (no C equivalent):**
| Go Name | Line | Notes |
|---|---|---|
| ActiveVersionCount | 142 | ? |
| NodeCount | 199 | ? |
| Split | 572 | C uses ts_stack_copy_version? |
| ForkAtNode | 593 | Go workaround for not having stack__iter |
| AddErrorCost | 899 | REMOVED double-count? verify |
| CompactHaltedVersions | 913 | ? |
| TopSubtree | 870 | ? |
| Status | 287 | ? |

Rename as you touch functions, not as a bulk rename.

---

## Parser Core: advanceVersion / ts_parser__advance

- [x] **Action ordering in parse tables** (ed34665)
  - `extract.go:parseActionMacros` grouped actions by type (SHIFT before REDUCE) instead of preserving C source order
  - Fixed: position-based sort preserves C ordering
  - Affects: all languages (tables regenerated in 601398f)

- [x] **Flat action loop** (5bb7b28)
  - C's loop: SHIFT/ACCEPT/RECOVER terminate immediately (`return true`); REDUCE is non-destructive
  - Go's old loop: processed ALL actions via splitting, destructive in-place reduce, `lastReduceIdx` swap hack
  - Fixed: flat loop matching C's `ts_parser__advance` (parser.c:1620-1677)

- [ ] **Non-destructive reduce + renumber**
  - C: `ts_parser__reduce` calls `ts_stack_pop_count` (non-destructive), tracks `last_reduction_version`, then `ts_stack_renumber_version` replaces original
  - Go: need to verify flat loop implementation actually does split-then-reduce (like `doReduceForPotential`) vs in-place reduce
  - May explain why Python/Assignments reduce/reduce ordering still fails
  - Ref: parser.c:1640-1670

- [ ] **SHIFT_REPEAT handling in flat loop**
  - C treats SHIFT_REPEAT differently from regular SHIFT (doesn't terminate loop?)
  - Go: need to verify SHIFT_REPEAT behavior matches C
  - Ref: parser.c:1625-1635

- [ ] **Version halting after all-reduces-merged**
  - C: if all reductions merged into existing versions (no `last_reduction_version`), the version is halted
  - Go: need to verify this halt path exists in flat loop
  - Ref: parser.c:1668-1675

---

## Parser Core: doReduce / ts_parser__reduce

- [ ] **Reduce slice grouping and selectChildren**
  - C: `ts_parser__reduce` groups pop slices by version, calls `selectChildren` on groups with same base
  - Go: `doReduce` groups by base node — verify grouping matches C exactly
  - Ref: parser.c:931-1046

- [ ] **Inline merge inside reduce** (CRITICAL — likely cause of 3+ failures)
  - C: after each reduce's push, loops over all earlier versions and merges if same state (parser.c:1033-1039)
  - Go: NO inline merge in doReduce — both split versions survive independently
  - C's inline merge means: reduce 2 merging into reduce 1 returns NONE, so last_reduction_version = reduce 1's version
  - Go sets lastReductionVersion to the last split regardless of whether it merged
  - This affects Python/Assignments, likely Perl/ambiguous_funcs, Perl/Double_dollar
  - Ref: parser.c:1033-1039

- [ ] **doReduce return value tracking**
  - C: `ts_parser__reduce` returns `STACK_VERSION_NONE` when all new versions merged
  - Go: `doReduce` is void, always sets lastReductionVersion
  - Fix: doReduce should return whether the version survived, and only update lastReductionVersion if it did
  - Ref: parser.c:1040-1046

- [ ] **DynPrec in reduce**
  - C: tracks `dynamic_precedence` through reduce, applies to merged versions
  - Go: verify DynPrec propagation matches C
  - Ref: parser.c:990-1010

---

## Stack: nodeAddLink / ts_stack_node_add_link

- [ ] **Case 1 merge when DynPrec equal**
  - C: when DynPrec is equal, existing tree wins (Case 1)
  - Go: same logic but version ordering may differ, causing wrong tree to be "existing"
  - Affects: Python/Assignments (list_splat vs list_splat_pattern)
  - Note: coder-2 confirmed Case 1 never fires for Assignments — it's actually a reduce/reduce ordering issue in advanceVersion
  - Ref: stack.c:120-160

---

## Error Recovery: handleError / ts_parser__handle_error

- [x] **handleError restructure** (3da71fa)
  - Restructured to match C's `ts_parser__handle_error` line-by-line
  - Fixed: removed Go-only Step 1.5

- [x] **recover() checks** (7f382c8)
  - Added missing `recover()` checks matching C tree-sitter

---

## Error Recovery: recoverToState / ts_parser__recover_to_state

- [x] **Multi-path GSS traversal** (8ce6698)
  - C: iterates ALL pop results, creates separate versions, keeps matching goal state, halts non-matching
  - Go: only checked first result
  - Fixed: iterates all paths

- [ ] **Trailing extras handling**
  - C: `ts_subtree_array_remove_trailing_extras` separates comments/whitespace from ERROR node, pushes them individually
  - Go: code written (8ce6698) but may not take effect until GSS structure is correct
  - Ref: parser.c:1120-1140

- [x] **Error cost double-counting** (8ce6698)
  - Go had explicit `AddErrorCost` that C doesn't have (cost embedded in `ts_subtree_new_error_node`)
  - Fixed: removed double-count

- [x] **ForkAtNode nodeCountAtLastError** (8ce6698)
  - Fixed in same commit as multi-path traversal

---

## Error Recovery: recover / ts_parser__recover

- [ ] **Strategy 1 depth++ overshoot**
  - `depth++` when `nodeCountSinceError > 0` overshoots by 1 because summary recorded after ERROR_STATE push
  - Partially mitigated by multi-path recoverToState (iterates all paths at given depth)
  - May need further adjustment
  - Ref: parser.c:1170-1195

- [ ] **Strategy 2 multi-path Pop**
  - C: handles `pop.size > 1` in Strategy 2 (keep first, discard rest, renumber)
  - Go: doesn't handle this
  - Ref: parser.c:1210-1230

- [ ] **has_error tracking after recover**
  - C: updates `self->has_error` flag after recovery
  - Go: doesn't track this (minor)

---

## Accept Path: doAccept / ts_parser__accept

- [x] **EOF marked as extra** (43ee822)
  - C: `bool extra = symbol == ts_builtin_sym_end` in `ts_subtree_new_leaf`
  - Go: wasn't marking EOF as extra
  - Fixed

- [x] **EOF pushed before PopAll** (43ee822)
  - C: pushes EOF lookahead at state 1 before calling PopAll
  - Go: wasn't pushing EOF
  - Fixed

- [x] **buildAcceptTree search direction** (43ee822)
  - C: searches right-to-left for root node (parser.c:1061, `j=trees.size-1` going down)
  - Go: was searching left-to-right
  - Fixed

- [x] **Recover EOF empty ERROR node** (43ee822)
  - C: pushes empty ERROR node at EOF
  - Go: was using SubtreeZero
  - Fixed

---

## Stack Operations

- [ ] **Pop vs ts_stack_pop_count — FUNDAMENTAL MISMATCH** (CRITICAL)
  - C's `ts_stack_pop_count` (via `stack__iter` + `ts_stack__add_slice`):
    - Creates NEW version in heads array for each GSS path
    - Returns `StackSliceArray` where each element has `.version` and `.subtrees`
    - Groups paths that reach the same node into the same version
  - Go's `Pop`:
    - Returns `[]StackIterator` with node + subtrees
    - Modifies first head IN-PLACE (`head.node = results[0].node`)
    - Does NOT create new versions for multi-path results
    - `doReduce` handles extra paths via `ForkAtNode` — a workaround, not a match
  - Go's `PopCount`: only returns a count, doesn't collect subtrees (used differently)
  - This is likely the root cause of multiple failures — without proper version-per-path creation, inline merge inside reduce can't work, and the version ordering is wrong
  - Ref: stack.c:304-322 (ts_stack__add_slice), stack.c:324-400 (stack__iter)

- [x] **PopAll multi-path traversal** (316754c)
  - Fixed to properly traverse all GSS paths in doAccept iteration

- [x] **RecordSummary depth counting** (b96c68a)
  - SubtreeZero links must increment depth (matching C stack.c:410-413)
  - Fixed

- [x] **condenseStack resume condition** (42b2bdd)
  - Was using VersionCount(), should use MAX_VERSION_COUNT
  - Fixed

- [x] **doAllPotentialReductions** (de8d8b8)
  - Properly ported: iterative with chaining, dedup, merge

---

## Lexer / Keywords

- [x] **Token cache keyword reusability** (c3ab642)
  - Cache keyed by (lex_state, position) but keyword acceptance is parse-state-dependent
  - Fixed

- [x] **Keyword demotion** (26978b6)
  - Matching C parser.c:1716-1742
  - Fixed

---

## Scanners

- [x] **Bash: null-terminated heredoc delimiters** (concierge, scanner branch)
- [x] **Python: indent serialization (2 bytes vs 1)** (concierge, scanner branch)
- [x] **Perl: heredoc struct format** (concierge, scanner branch)
- [x] **Ruby: serialization format + logic** (concierge, scanner branch)
- All scanner golden tests passing (30,366/30,366)

---

## Remaining Failures → Likely Root Cause

| Failure | Likely Mismatch |
|---------|----------------|
| Perl/ambiguous_funcs | Non-destructive reduce + renumber in flat loop |
| Perl/Double_dollar | Non-destructive reduce + renumber in flat loop |
| Perl/3x non_assoc | Strategy 1 depth++ + Strategy 2 multi-path |
| Python/Assignments | Non-destructive reduce + renumber in flat loop |
| Go/Error_keyword | Error recovery pipeline (needs correct GSS) |
| Go/String_literals | Error recovery pipeline (needs correct GSS) |
| JS/Extra_complex_literals | Error recovery pipeline (needs correct GSS) |
| Python/error_before_string | Error recovery pipeline (needs correct GSS) |
| TS/Arrow_functions_call_sig | NEW regression from flat loop — unknown |
