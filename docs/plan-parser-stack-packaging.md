# Plan: Parser/Stack Package Split Spike

**Date**: 2026-02-25
**Goal**: Allow `parser` to depend on `stack`, but not vice versa, by moving shared types/constants into a common package.

## Summary

Today `parser.go` and `stack.go` live in the same package and share many core types and helpers. A direct split into `parser` and `stack` packages would introduce a circular dependency because `stack` currently depends on constants and types defined in `parser` (notably `ErrorCostPerRecovery`). The clean way to split is to introduce a small **core** package that owns shared types/constants and low-level subtree/length helpers. Then:

- `parser` imports `stack` and `core`
- `stack` imports `core`
- no import cycle

## Proposed Package Boundaries

### `internal/core`
Owns foundational types and operations that are used broadly:
- Types: `Subtree`, `SubtreeArena`, `Length`, `Point`, `StateID`, `Symbol`, etc.
- Subtree ops: `GetSymbol`, `GetErrorCost`, `GetDynamicPrecedence`, `GetPadding`, `GetSize`, `GetExternalScannerState`, `IsExtra`, etc.
- Error-cost constants: `ErrorCostPerRecovery`, `ErrorCostPerMissingTree`, `ErrorCostPerSkippedTree`, `ErrorCostPerSkippedLine`, `ErrorCostPerSkippedChar`.

### `internal/stack`
Owns all GSS logic (`stack.go`) and depends only on `core`:
- `Stack`, `StackNode`, `StackLink`, `StackHead`
- `StackVersion`, `StackStatus`
- `Push`, `Pop`, `PopAll`, `PopPending`, `PopError`, `Merge`, `Split`, etc.

### `internal/parser`
Owns parsing logic (`parser.go`) and depends on `stack`, `core`, `lexer`, `language`, `reusable_node`:
- `Parser`
- all `ts_parser__*` logic

### `internal/lexer`
`lexer.go` moves to `internal/lexer` and is imported by `parser` only. This is required to keep `core` lean and avoid cross-layer leakage.

## Items That Must Move to Avoid Cycles

The following are currently defined outside `stack.go` but required by it:

- `ErrorCostPerRecovery` (defined in `parser.go` today)
- Core types: `Subtree`, `SubtreeArena`, `Length`, `Symbol`, `StateID` (currently in shared files like `types.go`, `subtree.go`, `length.go`)
- Subtree helpers used by `stack.go`:
  - `GetErrorCost`, `GetDynamicPrecedence`, `GetSymbol`
  - `GetPadding`, `GetSize`, `GetTotalBytes`, `GetVisibleDescendantCount`
  - `GetExternalScannerState`, `IsExtra`
  - `SubtreeZero`, `SymbolError`, `SymbolErrorRepeat`

These should live in `internal/core` so `stack` can compile without importing `parser`.

## Expected Import Graph (Target)

- `internal/core` (no internal deps)
- `internal/stack` -> `internal/core`
- `internal/parser` -> `internal/stack`, `internal/core`, `internal/lexer`, `internal/language`, `internal/reusable_node`

## Notes / Risks

- The repo is currently a single `treesitter` package; this split will be a **large refactor** with many import rewrites.
- Tests that currently use unexported types/functions may need to move into the new packages or be updated to use exported APIs.
- There will be **no transitional phases or type aliases**. Everything moves to its new package directly, even if that requires wide changes.
- Tests will be **moved/split to the new packages** (e.g., parser tests into `internal/parser`, stack tests into `internal/stack`, lexer tests into `internal/lexer`). We will not leave tests in the root `treesitter` package with only import updates.

## Suggested Spike Steps (No Code Yet)

1. Create `internal/core` and move constants/types/helpers that `stack` needs.
2. Move `stack.go` to `internal/stack` and adjust imports.
3. Move `parser.go` to `internal/parser` and adjust imports.
4. Move `lexer.go` to `internal/lexer` and update all parser imports.
5. Move/split all test files into the new packages and update imports accordingly; do not add aliases or re-exports.
