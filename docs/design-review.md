# Design Review: treesitter-go

## Incorporation Status

*Updated 2026-02-15. All actionable feedback from this review has been
incorporated back into `design.md`. Summary of changes:*

| Issue | Status | Notes |
|-------|--------|-------|
| ABI version range "14-15" | **Fixed** | Corrected to 13-15, targeting 15 only |
| Source file line counts | **Fixed** | Updated to accurate counts |
| Inline subtree optimization | **Incorporated** | Designed in from the start; `Subtree` is a value type with inline/heap discrimination |
| Missing Subtree fields | **Incorporated** | Added `visibleDescendantCount`, `firstLeaf`, `fragileLeft/Right`, `repeatDepth`, `productionID` |
| Copy-on-write under-specified | **Incorporated** | Decided: immutable trees, always clone edit path (O(depth)) |
| Children `[]*Subtree` → `[]Subtree` | **Incorporated** | Children stored as `[]Subtree` values for cache locality |
| Tree balancing not explained | **Incorporated** | Added explanation in Phase 6 deliverables |
| TSInput callback model | **Incorporated** | New §5 covering chunked input and lexer buffering |
| `included_ranges` / language injection | **Incorporated** | New §5 covering mechanism and component impact |
| `context.Context` for cancellation | **Incorporated** | Added to API (`Parse`/`ParseString` accept `ctx`); new Decision section |
| WASM code paths | **Incorporated** | Note added to §4 (External Scanners) |
| ParseActionEntry bit-packing | **Incorporated** | Struct is now primary; bit-packing deferred to profiling |
| `sync.Pool` caveats / arena allocation | **Incorporated** | Arena is now primary strategy; `sync.Pool` is secondary |
| Concurrency "open question" | **Incorporated** | Converted to Decision: Parser not goroutine-safe, immutable objects safe for concurrent reads |
| ABI version "open question" | **Incorporated** | Merged into Compatibility Goals (ABI 15 only) |
| Phase 3 time estimate | **Noted** | Review suggested 4-5 weeks; not changed in design (estimates are guidance, not commitments) |

---

## What's Good

**Strong architectural direction.** The overall approach — a pure-Go port of the
tree-sitter runtime with grammars compiled to Go source — is sound. The design
correctly identifies the key C components, maps them to a reasonable Go package
structure, and proposes a phased implementation that builds incrementally from
types to parser to code generator to queries.

**Grammar loading via code generation (Option A) is the right call.** The analysis
of Options A/B/C is thorough and the conclusion is well-justified. The lex
function must be compiled code for performance — a table-driven interpreter would
be a non-starter for production use. Emitting Go source also integrates naturally
with the Go module ecosystem.

**Memory management strategy is well-reasoned.** Eliminating ref counting in favor
of Go's GC is the right default. The recognition that `sync.Pool` and arena
allocation may be needed later (guided by profiling) shows appropriate pragmatism.
The structural sharing approach for `Tree.Edit()` (clone only the spine) is
correct and O(log N) is the right complexity target.

**External scanner interface design is clean.** The three-method Go interface
(`Scan`, `Serialize`, `Deserialize`) maps directly to the C scanner API with no
unnecessary complexity. The factory function approach for per-parser instances is
appropriate. The phased strategy (manual port first, auto-translation later, CGo
escape hatch) is practical.

**Phased delivery plan is realistic.** Each phase produces a testable artifact.
Deferring incremental parsing and the query system to later phases is correct —
they're additive features, not prerequisites for basic parsing.

**Performance targets are honest.** Accepting a 2-5x slowdown vs C for the initial
release is the right framing. Trying to match C performance would distort the
design toward premature optimization.

---

## Issues, Gaps, and Inaccuracies

### Factual Inaccuracies

**ABI version range is wrong.** Section 1 says grammars "compiled with tree-sitter
generate (ABI version 14-15)" — the actual minimum compatible version is **13**,
not 14:

```c
// api.h
#define TREE_SITTER_LANGUAGE_VERSION 15
#define TREE_SITTER_MIN_COMPATIBLE_LANGUAGE_VERSION 13
```

Version 14 introduced primary state deduplication; version 15 added reserved words.
The recommendation to target only version 15 is fine, but the claim about "14-15"
is inaccurate.

**Source file line counts are approximate in a misleading way.** Several counts
are off enough to matter:

| File | Design Doc | Actual | Delta |
|------|-----------|--------|-------|
| parser.c | 2263 | 2262 | -1 |
| query.c | ~4800 | 4496 | -304 |
| subtree.c | ~1100 | 1034 | -66 |
| stack.c | ~900 | 912 | +12 |
| get_changed_ranges.c | ~530 | 557 | +27 |
| language.c | ~260 | 289 | +29 |

The "~14 files, ~25K lines" characterization overstates. The core C source is
10 `.c` files totaling ~12.7K lines; including all headers brings it to ~15.9K.
Even adding `wasm_store.c` (1937 lines, out of scope) doesn't reach 25K.

### Structural Gaps

**The inline subtree optimization is dismissed too quickly.** The design says all
subtrees will be `*Subtree` pointers and "Go's generational GC makes [the inline
optimization] less critical than in C." This understates the problem.

In C, the `Subtree` union is exactly pointer-sized (8 bytes on 64-bit). Leaf
nodes are stored *inline* in this 8-byte space — no pointer chase, no heap
allocation, excellent cache locality. The `SubtreeInlineData` struct packs
symbol, parse state, padding, and size into those 8 bytes using bit fields.

In the proposed Go design, every leaf becomes a `*Subtree` heap allocation. For a
10KB source file with ~3000 leaf tokens, that's 3000 extra pointer dereferences
during tree traversal. This isn't just GC pressure — it's a fundamental cache
locality regression. Every `Node.Child()` call follows a pointer instead of
reading inline data.

**Recommendation:** Consider a tagged union approach in Go:

```go
type Subtree struct {
    // If inline is true, data is packed into these fields directly
    // If inline is false, heap points to SubtreeData
    inline bool
    // Inline fields (used when inline == true)
    symbol     uint8
    parseState uint16
    // ... packed size/padding fields
    // Heap pointer (used when inline == false)
    heap *SubtreeData
}
```

This preserves the inline optimization's value (no pointer chase for leaves)
while remaining idiomatic Go. Profile before committing to either approach, but
don't assume the optimization is unnecessary.

**The Subtree struct is incomplete.** The design's `Subtree` struct omits several
fields present in the C `SubtreeHeapData` that are important for correctness:

- `visible_descendant_count` — essential for efficient `Node.NamedChild(index)`
  and `Node.ChildCount()` on the public API (these skip invisible nodes using
  this count)
- `first_leaf` (`{symbol, parse_state}`) — used by the incremental parser to
  check whether a reused subtree's first token matches the current lex mode;
  without it, incremental reusability checks are incomplete
- `fragile_left` / `fragile_right` — set on subtrees produced during GLR
  ambiguity resolution; the incremental parser refuses to reuse fragile subtrees
- `repeat_depth` — used by the tree balancing algorithm (`ts_subtree_balance`)
  to keep left-recursive repetition trees O(log N) depth
- `production_id` — needed for field map lookups and alias sequence indexing

These aren't optional fields — they're required for API correctness and
incremental parsing.

**Copy-on-write without ref counting is under-specified.** The design says "we
track mutability through the parse algorithm's control flow (the parser knows
when it owns a subtree exclusively)." In C, ownership is determined by checking
`ref_count == 1`. Without ref counting, the Go implementation needs an explicit
ownership model.

The structural sharing approach for `Tree.Edit()` is correct (clone the spine),
but during parsing, the parser frequently calls `ts_subtree_make_mut` on subtrees
that may be shared between old and new trees. The design needs to specify how the
parser determines whether a subtree is exclusively owned. Options:

1. An explicit `owned bool` field on Subtree (checked before mutation)
2. Always copy on mutation (simple but wasteful)
3. Track ownership at the parser level (a set of known-owned pointers)

This is a correctness risk if left ambiguous.

**Children storage layout has performance implications not discussed.** In C,
children are allocated *immediately before* the parent's `SubtreeHeapData` in a
single allocation:

```c
#define ts_subtree_children(self) \
  ((Subtree *)((self).ptr) - (self).ptr->child_count)
```

This means accessing a parent and its children touches a single contiguous memory
region. The Go design uses `children []*Subtree` which means: (1) an indirection
through the slice header, (2) an array of pointers, (3) each pointer to a
separate heap object. Three levels of indirection vs one. For tree traversal
(the hot path for queries, highlighting, and node navigation), this may be
significant.

**Recommendation:** Consider storing children as `[]Subtree` (values, not
pointers) for non-leaf nodes, keeping children contiguous in memory with their
parent's data.

**The tree balancing algorithm is mentioned but not explained.** Phase 6
deliverables include `subtree.compress()` but the design never explains what tree
balancing does or why it's needed. Left-recursive repetitions (e.g., a file with
1000 statements) produce degenerate left-leaning trees. The C implementation
rotates these into balanced trees to keep depth O(log N). This is critical for
incremental parsing performance (the reusable node iterator traverses to depth)
and for `Node.Child(index)` performance on long lists. The algorithm is non-trivial
and should be described in the design.

### Missing Topics

**No discussion of the `TSInput` callback model.** The C parser uses a callback-based
input model (`TSInput.read(payload, byte_offset, position, &bytes_read)`) that
supports reading from ropes, gaps buffers, and other non-contiguous data structures.
The design defines an `Input` interface but doesn't discuss how the lexer buffers
and manages chunks from this callback, which is one of `lexer.c`'s primary
responsibilities.

**No discussion of `included_ranges` for language injection.** The design mentions
`SetIncludedRanges` in the API but never explains how it works. Language injection
(e.g., JavaScript inside HTML, CSS inside `<style>` tags) requires the parser to
skip over byte ranges that belong to a different language. This affects the lexer
(which must handle range boundaries), the incremental parser (which must detect
`included_range_difference`), and the changed ranges algorithm. This is a
significant feature that touches multiple components.

**No discussion of `ts_parser_set_timeout_micros` / cancellation.** The C parser
supports cancellation via timeout or a cancellation flag checked during parsing.
The Go parser should support `context.Context` for cancellation, which is the
idiomatic Go equivalent. This should be part of the API design.

**WASM-related code paths in the parser are not discussed.** While WASM *loading*
is correctly out of scope, `parser.c` has WASM-conditional code paths for calling
external scanners. The port should ensure these are cleanly excised, not left as
dead code paths.

### Minor Issues

**ParseActionEntry encoding complexity may not be worth it.** The design proposes
a `uint64` bit-packed encoding "because it's the hottest data structure." But the
actual hot path is `ts_language_table_entry()`, which returns a `TableEntry`
(pointer + count + reusable flag). The bit-packing happens once at lookup time.
The simpler struct approach is probably fine — use it first, profile, and only
switch to bit-packing if it shows up in profiles.

**`sync.Pool` for `Subtree` allocation has caveats.** `sync.Pool` items can be
collected at any GC cycle. Unlike C's `SubtreePool` which provides stable,
predictable reuse, `sync.Pool` provides only best-effort pooling. For
allocation-heavy workloads, an arena allocator (pre-allocate a `[]Subtree` backing
array) is likely more effective. The design mentions this as an option but buries
it in the risks section rather than presenting it as the primary mitigation for
GC pressure.

**The concurrency model section should be more definitive.** The "open question"
framing is unnecessary — the answer is clearly stated (Parser is not goroutine-safe,
immutable objects are safe for concurrent reads). This should be a design decision,
not an open question.

---

## Alternative Approaches Worth Considering

**Arena allocation for subtrees during a single parse.** Allocate all subtrees
from a single `[]SubtreeData` backing array, indexed by offset. After parsing,
the tree holds references into this arena. The arena is freed when the tree is
GC'd (via a finalizer). This eliminates per-subtree allocation, gives excellent
cache locality, and avoids `sync.Pool` semantics entirely. The tradeoff is
complexity in the tree sharing model (old and new trees after incremental parsing
share arena regions).

**Code generation from `node-types.json` + `grammar.json` instead of `parser.c`.**
The design mentions reading "the same JSON intermediate format that the Rust
compiler produces." This is the right approach, but it should be explicit: the
input is `src/grammar.json` (generated by `tree-sitter generate`) and
`src/node-types.json`, NOT the JavaScript `grammar.js`. The code generator
should depend on `tree-sitter generate` having already been run, not try to
replicate the grammar preparation pipeline.

**Consider using Go generics for the Subtree/Node abstraction.** The distinction
between inline and heap-allocated subtrees could potentially be expressed as a
generic type parameter, though this may not work cleanly with Go's type system.
Worth spiking on.

**Consider `context.Context` integration from the start.** Rather than
retrofitting cancellation support, the `Parser.Parse()` method should accept a
`context.Context` from day one. The parser's main loop can check `ctx.Done()` at
the same points where the C parser checks its cancellation flag.

---

## Overall Assessment

This is a **solid design document** that demonstrates a thorough understanding of
tree-sitter's architecture. The major architectural decisions (compile grammars to
Go, eliminate ref counting, use GC, phase the implementation) are correct.

The primary concerns are:

1. **The inline subtree optimization should not be deferred.** It affects the
   fundamental data structure design. If added later, it requires changing the
   `Subtree` type everywhere. Design the type to support it from the start, even
   if the initial implementation doesn't populate inline data.

2. **The Subtree struct must include all fields from `SubtreeHeapData`.** Missing
   `visible_descendant_count`, `first_leaf`, `fragile_left/right`, and
   `repeat_depth` will cause bugs that are hard to diagnose later.

3. **Copy-on-write semantics need a concrete specification.** "The parser knows
   when it owns a subtree" is too vague for a correctness-critical operation.

4. **Language injection (included ranges) needs design attention.** It's listed
   in the API but not discussed, and it touches the lexer, parser, and changed
   ranges algorithm.

The implementation phases are well-structured, but Phase 3 (non-incremental
parser, 3 weeks) is likely underestimated given the complexity of error recovery.
The GLR stack and error recovery together are the hardest part of the port — I'd
budget 4-5 weeks for Phase 3.

The document is ready to guide implementation after addressing the gaps identified
above. The recommended first step is to finalize the `Subtree` type design
(including the inline optimization decision and missing fields) since it's the
foundational data structure that everything else builds on.
