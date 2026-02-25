# Review: Parser/Stack Package Split Spike (coder-2)

Date: 2026-02-25
Reviewed doc: `docs/plan-parser-stack-packaging.md`

## Findings

1. Missing ownership plan for `reusable_node.go` and potential cycle risk.
   - The plan says `internal/parser` depends on `internal/reusable_node`, but `reusable_node.go` today references parser-adjacent types and subtree internals.
   - Recommendation: explicitly define whether `reusable_node` depends on `core` only, or on `parser`. If it depends on `parser`, move it into `internal/parser` to avoid an `parser <-> reusable_node` cycle.

2. `language.go`/`tree.go`/`query*.go` boundary is unspecified, but parser signatures likely leak types.
   - A parser package split may require exported constructor/signature changes if `Parser` currently returns or accepts root-package types directly.
   - Recommendation: add a "public API compatibility strategy" section listing which exported symbols remain in root package and whether wrappers/adapters are needed.

3. Test relocation strategy needs deterministic mapping.
   - "Move/split all test files" is high level; current tests are mixed black-box and white-box, and many will span parser+stack+lexer behavior.
   - Recommendation: add a file-by-file migration table with destination package and rationale (`white-box`, `black-box`, `integration`) to avoid test loss and duplicate rewrites across agents.

4. No anti-regression gate for split correctness.
   - The split is primarily structural; behavior must be proven unchanged.
   - Recommendation: define mandatory pre/post equivalence checks (same failing/passing test set, scanner trace parity, parse corpus parity, benchmark deltas with tolerance thresholds).

5. Export policy for `internal/core` is unclear.
   - If `core` becomes a dumping ground for shared items, layering will erode quickly.
   - Recommendation: define strict criteria for what can live in `core` (no parser policy, no lexer state machine details, no high-level orchestration), with a lint/checklist to enforce.

## Suggested Additions to Main Plan

- Add a package dependency contract section:
  - `core`: data model and pure low-level tree/length ops only.
  - `stack`: GSS implementation only.
  - `parser`: orchestration and parse algorithm.
  - `lexer`: lex mode / token advancement only.
  - `reusable_node`: either folded into parser or constrained to `core` interfaces.

- Add a migration matrix section:
  - source file
  - target package
  - owner agent
  - depends on task(s)
  - done criteria

- Add validation gates section:
  - `go test ./...`
  - corpus tests unchanged
  - scanner trace tests unchanged
  - benchmark max regression threshold (e.g., <= 3% on hot paths)

## Parallelization-Friendly Work Split (Conflict Avoidance)

If investigators want safe parallel work now, these slices can be assigned with low overlap:

1. `internal/core` extraction slice:
   - files: `types.go`, `subtree.go`, `subtree_edit.go` (core-only moves), plus new `internal/core/*`
2. `internal/stack` slice:
   - files: `stack.go`, `stack_test.go`, `condense_stack_test.go`, plus new `internal/stack/*`
3. `internal/lexer` slice:
   - files: `lexer.go`, `lexer_test.go`, plus new `internal/lexer/*`
4. parser orchestration slice:
   - files: `parser.go`, `parser_test.go`, `parser_integration_test.go`, plus new `internal/parser/*`

Each later slice should depend on core extraction to reduce conflicts.
