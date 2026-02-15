# Addendum: Hot Path Predictions and SIMD Optimization Plan

## Purpose

This document predicts where a Go port of tree-sitter will be slowest relative to C, and proposes specific optimizations — including SIMD/assembly — for each hot path. These are pre-measurement predictions based on reading the C source code (`lib/src/` in tree-sitter v0.25.x). They will be validated with profiling (`go test -bench` + `perf stat`) once the implementation exists.

The predictions focus on where Go's runtime model (GC, bounds checks, interface dispatch, escape analysis) creates measurable overhead versus C's direct memory control, and where targeted Go-level or assembly-level optimizations can close or eliminate the gap.

---

## 1. Hot Path Inventory

### 1a. Lexer Inner Loop — Generated `lex_fn`

**What it does:** The generated `lex_fn` is a massive state machine (switch-on-state, switch-on-character) that recognizes tokens. For each lex state, it checks the current lookahead character against a set of valid transitions using the `ADVANCE_MAP` macro, which stores (character, state) pairs in a flat array and performs a *linear scan* to find the matching entry.

**Call frequency:** Once per token. But internally, the lexer advances character-by-character through the input, so the total work is proportional to file size.

**Why it's fast in C:**
- The `ADVANCE_MAP` linear scan has good cache locality for small maps (the common case — most lex states have <10 transitions)
- `set_contains` uses a hand-written binary search on sorted ranges
- The character-to-state dispatch compiles to dense jump tables via `switch`
- No bounds checks, no function call overhead for `ADVANCE`

**Why a naive Go port is slower:**
- Go `switch` statements compile to binary search on sparse cases, not jump tables
- Bounds checks on every array access in the map scan
- The `set_contains` binary search has bounds checks on each iteration
- Interface dispatch if the lexer is called through an interface

**Already covered in detail:** See `docs/addendum-lexer-performance.md`. Key insight: C's `ADVANCE_MAP` linear scan is actually a *weakness* — our Go port can use O(1) equivalence-class table lookups + SIMD to beat C here.

**Predicted gap:** Moderate (20-50%) for a naive port, but **Go can be faster** with the Phase 0-2 optimizations from the lexer addendum.

---

### 1b. Character Reading — `ts_lexer__do_advance`

**What it does:** Advances the lexer's position by one character, updating byte offset, row, and column. Then decodes the next Unicode character from the input chunk. This is the innermost operation in lexing.

**C implementation** (`lexer.c:200-250`):
```c
static void ts_lexer__do_advance(Lexer *self, bool skip) {
  if (self->lookahead_size) {
    if (self->data.lookahead == '\n') {
      self->current_position.extent.row++;
      self->current_position.extent.column = 0;
    } else {
      self->current_position.extent.column += self->lookahead_size;
    }
    self->current_position.bytes += self->lookahead_size;
  }
  // ... range boundary check ...
  if (current_range) {
    // ... chunk boundary check ...
    ts_lexer__get_lookahead(self);
  }
}
```

The `ts_lexer__get_lookahead` function (`lexer.c:106-136`) decodes UTF-8/16:
```c
static void ts_lexer__get_lookahead(Lexer *self) {
  uint32_t position_in_chunk = self->current_position.bytes - self->chunk_start;
  const uint8_t *chunk = (const uint8_t *)self->chunk + position_in_chunk;
  self->lookahead_size = decode(chunk, size, &self->data.lookahead);
}
```

**Call frequency:** Once per character — millions of times per file. For a 100KB file, this is called ~100,000 times.

**Why it's fast in C:**
- Direct pointer arithmetic into the chunk buffer
- The decode function pointer is resolved once and called without indirection overhead (it's a local variable, not a vtable)
- The position struct fields are updated with simple integer ops, no allocation
- The `included_ranges` boundary check is almost never triggered (most files have a single range)
- The compiler inlines `ts_lexer__get_lookahead` into `ts_lexer__do_advance`

**Why a naive Go port is slower:**
- Slice bounds checking on `chunk[position_in_chunk:]` on every character
- UTF-8 decoding via `utf8.DecodeRune` has an extra bounds check internally
- Position struct updates should be fine (value types, no allocation)
- The chunk boundary check involves a comparison against `chunk_start + chunk_size`; in Go, this is a slice length check which is cheap

**Specific Go concerns:**
- The main overhead is bounds checking. In a tight loop, Go's BCE (bounds check elimination) can remove these if the compiler can prove the index is in range. But the `position_in_chunk` calculation crosses function boundaries, making BCE difficult.
- The chunked input model means we call the `Read` callback (an interface method) every time we cross a chunk boundary. In typical use (parsing a single string), there's only one chunk, so this is not a hot path.

**Predicted gap:** Moderate (20-50%).

**Mitigation:**
- Use `_ = chunk[position_in_chunk]` pattern to hoist bounds checks
- Inline the UTF-8 decode for ASCII fast path (first byte < 0x80, single byte)
- The ASCII fast path covers >90% of characters in typical source code and is a single comparison + assignment, avoiding `utf8.DecodeRune` entirely
- Consider using `unsafe.Pointer` arithmetic if profiling shows bounds checks dominate

---

### 1c. External Scanner Dispatch

**What it does:** Calls the external scanner's `scan`, `serialize`, and `deserialize` functions. External scanners handle context-sensitive tokens (e.g., Python indentation, template literals, heredocs).

**C implementation** (`parser.c:443-468`):
```c
static bool ts_parser__external_scanner_scan(TSParser *self, TSStateId external_lex_state) {
  const bool *valid_external_tokens = ts_language_enabled_external_tokens(
    self->language, external_lex_state
  );
  return self->language->external_scanner.scan(
    self->external_scanner_payload,
    &self->lexer.data,
    valid_external_tokens
  );
}
```

The scanner state is serialized after every successful external token (`parser.c:550`):
```c
external_scanner_state_len = ts_parser__external_scanner_serialize(self);
```

And deserialized before every external scan attempt (`parser.c:544`):
```c
ts_parser__external_scanner_deserialize(self, external_token);
```

**Call frequency:** Once per token position where external tokens are valid. For languages like JavaScript with template literals, this can be once per token. For languages without external scanners (like JSON), this is never called. For Python, it's called at every indentation-sensitive position.

**Why it's fast in C:**
- Direct function pointer call (`language->external_scanner.scan`)
- The serialized state is stored inline on the subtree (up to 24 bytes in `short_data`, heap-allocated for larger states)
- `memcpy` for serialization/deserialization
- The `valid_external_tokens` is a simple `bool[]` array lookup

**Why a naive Go port is slower:**
- Go interface dispatch for `ExternalScanner.Scan()` — this involves loading the itab, then indirect function call. Roughly 2-5ns overhead per call.
- The scanner state serialization in Go would use `[]byte` allocation unless carefully pooled
- Each `Deserialize` call might allocate if the scanner creates internal state
- The `valid_external_tokens` would be a `[]bool` slice — same performance

**Predicted gap:** Minimal to Moderate (10-30%).

**Rationale:** The external scanner's own work (scanning tokens, managing state) dominates the dispatch overhead. The interface call overhead of ~3ns is negligible compared to the scanner's typical ~100ns+ of work per invocation. The serialization/deserialization overhead is the bigger concern.

**Mitigation:**
- Use concrete types where possible (avoid interface dispatch on hot paths)
- Pool `[]byte` buffers for serialization with `sync.Pool`
- For the most common case (state < 24 bytes), use a fixed-size `[24]byte` array embedded in the subtree struct, matching C's `ExternalScannerState.short_data`

---

### 1d. Parse Table Lookup — `ts_language_table_entry`

**What it does:** Looks up the parse action(s) for a given (state, symbol) pair in the parse action table. This is the core of the LR parsing algorithm — it tells the parser whether to shift, reduce, or report an error.

**C implementation** (`language.c:68-86`):
```c
void ts_language_table_entry(
  const TSLanguage *self, TSStateId state, TSSymbol symbol, TableEntry *result
) {
  if (symbol == ts_builtin_sym_error || symbol == ts_builtin_sym_error_repeat) {
    result->action_count = 0;
    result->is_reusable = false;
    result->actions = NULL;
  } else {
    uint32_t action_index = ts_language_lookup(self, state, symbol);
    const TSParseActionEntry *entry = &self->parse_actions[action_index];
    result->action_count = entry->entry.count;
    result->is_reusable = entry->entry.reusable;
    result->actions = (const TSParseAction *)(entry + 1);
  }
}
```

The `ts_language_lookup` function performs a two-level table lookup. For small state counts, it's a direct 2D array index `parse_table[state * symbol_count + symbol]`. For larger grammars, it uses a compressed sparse row format with binary search.

**Call frequency:** Once per token per parse state. In GLR parsing with ambiguities, this may be called multiple times for the same token (once per active stack version). Typically 1-3 times per token.

**Why it's fast in C:**
- Direct array indexing with pointer arithmetic
- The parse table is typically in L2/L3 cache (it's accessed repeatedly with moderate locality)
- The `TSParseActionEntry` union is accessed via pointer cast, avoiding copies
- For the common case (unambiguous LR), there's exactly one action per (state, symbol) pair

**Why a naive Go port is slower:**
- Bounds checking on the `parse_actions` slice access
- The compressed format's binary search has bounds checks per iteration
- If `TableEntry` is returned by value (likely), Go will copy the struct — but it's only 16 bytes, which is fine
- If the parse actions array is a `[]ParseAction`, slice header overhead is trivial

**Predicted gap:** Minimal (<20%).

**Rationale:** The lookup is a simple array index or binary search. Go's bounds checks add ~1ns per lookup, but the lookup itself takes ~5-10ns. The ratio is acceptable. The parse table data fits in cache (typical grammar has ~1000 states × ~200 symbols, stored compressed to ~100KB).

**Mitigation:**
- Use the same compressed format as C (sparse row with binary search)
- Hoist bounds checks: `_ = parseActions[actionIndex]`
- For uncompressed tables: precompute `state * symbolCount` outside the inner loop

---

### 1e. Subtree Creation and Manipulation

**What it does:** Creates leaf nodes for tokens and internal nodes for parse reductions. This is the tree-building step of the parser.

**C implementation — leaf creation** (`subtree.c:166-224`):
```c
Subtree ts_subtree_new_leaf(SubtreePool *pool, TSSymbol symbol, Length padding,
    Length size, uint32_t lookahead_bytes, TSStateId parse_state,
    bool has_external_tokens, bool depends_on_column,
    bool is_keyword, const TSLanguage *language) {
  bool is_inline = (
    symbol <= UINT8_MAX &&
    !has_external_tokens &&
    ts_subtree_can_inline(padding, size, lookahead_bytes)
  );
  if (is_inline) {
    return (Subtree) {{ /* 8-byte inline data */ }};
  } else {
    SubtreeHeapData *data = ts_subtree_pool_allocate(pool);
    *data = (SubtreeHeapData) { /* ~64 bytes */ };
    return (Subtree) {.ptr = data};
  }
}
```

**The inline subtree optimization** is key. `Subtree` is a union:
```c
typedef union {
  SubtreeInlineData data;  // 8 bytes, stored directly
  const SubtreeHeapData *ptr;  // pointer to heap
} Subtree;
```

`SubtreeInlineData` fits in 8 bytes and stores: `is_inline` flag, symbol (8 bits), parse_state (16 bits), padding/size (8 bits each), visibility flags. This avoids heap allocation for the majority of leaf tokens.

**C implementation — internal node creation** (`subtree.c:480-530`):
```c
MutableSubtree ts_subtree_new_node(TSSymbol symbol, SubtreeArray *children,
    unsigned production_id, const TSLanguage *language) {
  // Allocate at the end of the children array
  SubtreeHeapData *data = (SubtreeHeapData *)&children->contents[children->size];
  *data = (SubtreeHeapData) { .child_count = children->size, ... };
  ts_subtree_summarize_children(self, language);
  return (MutableSubtree) {.ptr = data};
}
```

Internal nodes are *allocated at the end of the children array*. This means the `SubtreeHeapData` header and all children pointers live in a single contiguous allocation. This is a clever layout optimization for cache locality.

**Call frequency:**
- Leaf creation: once per token (~N times for N tokens)
- Internal node creation: once per grammar reduction (~N times, roughly equal to token count)
- `ts_subtree_summarize_children`: once per internal node creation, iterates all children

**Why it's fast in C:**
- **Inline subtrees**: ~60-80% of leaf tokens fit the inline representation (8 bytes, zero heap allocation). The `Subtree` union is exactly pointer-sized, so arrays of subtrees have no overhead.
- **SubtreePool free-list**: Heap-allocated subtrees come from a free-list pool (`ts_subtree_pool_allocate`), avoiding `malloc`/`free` overhead. Pool size is capped at `TS_MAX_TREE_POOL_SIZE` (32).
- **Contiguous children**: Internal nodes pack `SubtreeHeapData` at the end of the children array, so parent + children are one allocation.
- **Reference counting**: `ref_count` is a simple atomic increment/decrement.

**Why a naive Go port is slower:**

This is one of the most significant performance challenges.

- **No tagged unions in Go**: Go has no way to store an 8-byte inline value or a pointer in the same 8-byte word with a discriminator bit. The closest is `interface{}`, which is 16 bytes (type pointer + data pointer) and always heap-allocates the data.
- **GC vs ref counting**: Go's GC must scan all live subtrees during collection. A large parse tree (100K+ nodes) creates significant GC pressure.
- **No contiguous parent+children layout**: Go's allocator doesn't support "allocate at the end of a slice." Each internal node would be a separate allocation.
- **sync.Pool is imperfect**: `sync.Pool` provides free-list semantics, but items can be reclaimed by GC at any time, and there's per-P overhead.

**Predicted gap:** Large (50-200%) for subtree-heavy operations.

**Mitigation strategies (see §3 and §4):**
- Use a struct-based discriminated union: `type Subtree struct { data SubtreeInlineData; ptr *SubtreeHeapData }` with a separate `isInline` flag. This is 16 bytes per subtree instead of 8, but avoids interface dispatch.
- Better: use `unsafe.Pointer` to implement C-style tagged pointer. Store inline data in the pointer bits when `isInline` is set (use the low bit of the pointer as the tag, since Go pointers are aligned). This gets back to 8 bytes.
- Pre-allocate children arrays with extra capacity for the parent node header (matching C's layout).
- Use arena allocation (see `docs/addendum-incremental-parsing.md`) for subtree nodes to reduce GC pressure.

---

### 1f. Stack Operations — GLR Stack Push/Pop/Merge

**What it does:** The Graph-Structured Stack (GSS) maintains multiple parse stack versions simultaneously for GLR parsing. Operations include push (adding a node), pop (removing nodes and collecting subtrees), split (forking a version), and merge (combining equivalent versions).

**C implementation** (`stack.c`):

Key data structures:
```c
struct StackNode {
  TSStateId state;
  Length position;
  StackLink links[MAX_LINK_COUNT];  // MAX_LINK_COUNT = 8
  short unsigned int link_count;
  uint32_t ref_count;
  unsigned error_cost;
  unsigned node_count;
  int dynamic_precedence;
};

typedef struct {
  StackNode *node;
  Subtree subtree;
  bool is_pending;
} StackLink;
```

Each `StackNode` has up to 8 links, allowing the DAG structure needed for GLR. The `stack__iter` function (`stack.c:324-419`) is the core iterator, using a callback pattern:

```c
static StackSliceArray stack__iter(Stack *self, StackVersion version,
    StackCallback callback, void *payload, int goal_subtree_count) {
  // Uses self->iterators array (max 64 iterators)
  // Walks links, calling callback at each step
  // Collects subtrees along the way
}
```

**Call frequency:** Once per parse action (shift or reduce). The `stack__iter` is called during every pop operation (reductions), and the pop operation collects all paths through the DAG.

**Why it's fast in C:**
- `StackNode` is a fixed-size struct (~112 bytes with 8 links), allocated from a pool (`node_pool`, max 50 nodes)
- `stack_node_retain`/`stack_node_release` are simple ref-count ops, compiled to a single `inc`/`dec` instruction
- The `links` array is inline (not heap-allocated), giving excellent cache locality
- The iterator array is pre-allocated (`MAX_ITERATOR_COUNT = 64`)
- `forceinline` attribute on hot accessors

**Why a naive Go port is slower:**
- `StackNode` with 8 `StackLink` entries is ~200 bytes in Go (each `StackLink` has a pointer, a `Subtree` union/struct, and a bool). Still value-typed and cache-friendly if allocated in an array.
- No ref counting needed (GC handles it), but GC must trace all the pointers in every `StackNode`
- The callback pattern in `stack__iter` would use an interface or function value in Go, adding indirect call overhead per iteration
- Slice bounds checks in the iterator loop

**Predicted gap:** Moderate (20-50%).

**Rationale:** The GSS is typically small (MAX_VERSION_COUNT = 6 active versions). Most parse operations are unambiguous LR, meaning there's only one version. The stack operations are not the bottleneck — the lexer is. The main concern is GC tracing of the pointer-heavy `StackNode` graph.

**Mitigation:**
- Use a pool (`sync.Pool` or manual free-list) for `StackNode` allocation
- Replace the callback pattern with a direct loop + enum return, avoiding indirect calls
- Use `[8]StackLink` fixed array, not slice, for the links
- Keep iterator array on the stack (it's only used during a single `pop` operation)

---

### 1g. Tree Cursor Traversal

**What it does:** The tree cursor provides an iterative, stack-based traversal of the parse tree. It's used internally during incremental re-parsing (finding reusable subtrees) and externally by API consumers walking the tree.

**C implementation** (`tree_cursor.c`):

The cursor maintains a stack of `TreeCursorEntry`:
```c
typedef struct {
  const Subtree *subtree;
  Length position;
  uint32_t child_index;
  uint32_t structural_child_index;
  uint32_t descendant_index;
} TreeCursorEntry;

typedef struct {
  const TSTree *tree;
  Array(TreeCursorEntry) stack;
  TSSymbol root_alias_symbol;
} TreeCursor;
```

The core operation `ts_tree_cursor_child_iterator_next` (`tree_cursor.c:60-97`) iterates children of the current node:
```c
static inline bool ts_tree_cursor_child_iterator_next(
    CursorChildIterator *self, TreeCursorEntry *result, bool *visible) {
  const Subtree *child = &ts_subtree_children(self->parent)[self->child_index];
  *result = (TreeCursorEntry) { .subtree = child, .position = self->position, ... };
  self->position = length_add(self->position, ts_subtree_size(*child));
  self->child_index++;
  // ... visibility/alias checks ...
}
```

**Call frequency:**
- During incremental re-parsing: O(edited_region_size) — proportional to the size of the changed region, not the whole file
- During query execution: once per visible node in the query range
- During API tree walking: once per node visited by the user

**Why it's fast in C:**
- `TreeCursorEntry` is 32 bytes, stack-allocated
- `ts_subtree_children(parent)` is a pointer offset from the parent's heap data — zero-cost
- `length_add` is an inline function, likely compiled to 3 integer additions
- The visibility check is a simple bitfield test
- The stack is a dynamic array but typically stays small (depth of the parse tree, usually 10-30)

**Why a naive Go port is slower:**
- `TreeCursorEntry` would be ~40 bytes in Go (due to alignment), allocated on the stack's backing array
- `ts_subtree_children` requires dereferencing the parent pointer and computing an offset — Go adds a nil check and bounds check
- The `Array(TreeCursorEntry)` stack is a slice in Go — growing it may cause allocation
- Accessing `subtree.ptr.child_count` through a pointer requires a nil check in Go

**Predicted gap:** Minimal to Moderate (10-30%).

**Rationale:** Tree cursor operations are not the innermost loop. They're called once per node, not once per character. The overhead per call is small (~5-10ns of checks), and each call does meaningful work (computing positions, checking visibility). The cursor stack is typically shallow.

**Mitigation:**
- Pre-allocate the cursor stack with capacity 32 (covers most parse tree depths)
- Use `unsafe.Pointer` for the `ts_subtree_children` offset computation if it shows up in profiles
- Inline `length_add` (Go compiler should do this automatically for small functions)

---

### 1h. Query / Pattern Matching

**What it does:** Executes tree-sitter queries (S-expression patterns) against a parse tree. The query cursor walks the tree using the tree cursor and maintains a set of in-progress match states, checking each node against pattern steps.

**C implementation** (`query.c:3870-4050`):

The core loop enters each node and:
1. Looks up patterns whose root symbol matches the current node (`ts_query__pattern_map_search` — binary search)
2. Creates new `QueryState` entries for matching patterns
3. Updates all in-progress states by checking the current node against their next step
4. Records captures for matching steps
5. Prunes states that can no longer match

Each `QueryState` is 12 bytes:
```c
typedef struct {
  uint32_t id;
  uint32_t capture_list_id;
  uint16_t start_depth;
  uint16_t step_index;
  uint16_t pattern_index;
  uint16_t consumed_capture_count: 12;
  bool seeking_immediate_match: 1;
  // ...
} QueryState;
```

The `QueryStep` has symbol, field, capture IDs, depth, and various flags (18 bytes).

**Call frequency:** Once per visible node in the query range. For each node, iterates all active states (typically < 20). The total cost is O(nodes × active_states × steps_to_check).

**Why it's fast in C:**
- `QueryState` and `QueryStep` are small, cache-friendly structs
- The pattern map binary search is O(log P) where P is the number of patterns
- State array is compacted in-place (deleted_count pattern), avoiding allocation
- Captures are stored in a pool of pre-allocated lists (`CaptureListPool`)
- Direct array indexing for `array_get(&self->states, i)`

**Why a naive Go port is slower:**
- The state array compaction loop (`deleted_count` pattern) requires careful bounds check elimination
- The capture list pool needs Go adaptation (slice of slices, with pooling)
- The `ts_tree_cursor_current_status` call within the query loop fetches multiple properties — if implemented as separate method calls, there's repeated dispatch overhead
- The `QueryState` struct in Go is ~16 bytes (due to alignment), slightly larger than C's 12

**Predicted gap:** Minimal to Moderate (15-30%).

**Rationale:** Query execution is dominated by tree traversal and pattern matching logic, not by low-level operations. The state management is cache-friendly in both C and Go. The main overhead is indirect: Go's GC needs to trace the capture lists, and the state array may trigger GC write barriers on updates.

**Mitigation:**
- Use a fixed-size array for the active states (e.g., `[256]QueryState`) to avoid slice growth
- Pool capture lists using `sync.Pool` or a manual free-list
- Batch the `current_status` call to fetch all properties at once
- Use the same in-place compaction pattern as C (this is natural in Go too)

---

### 1i. Memory Allocation Patterns

**What it does:** Tree-sitter's C implementation uses several specialized allocation strategies that are critical to its performance:

1. **SubtreePool** (`subtree.h:171-174`, `subtree.c:121-151`): A free-list of up to 32 `SubtreeHeapData` structs. `ts_subtree_pool_allocate` checks the free list first, falling back to `malloc`. `ts_subtree_pool_free` returns structs to the free list.

2. **StackNodePool** (`stack.c:12`, `stack.c:138-179`): A free-list of up to 50 `StackNode` structs. Same pattern as SubtreePool.

3. **Inline subtrees** (`subtree.h:50-101`): ~60-80% of leaf tokens are stored as 8-byte inline structs, requiring *zero* heap allocation.

4. **Contiguous node+children** (`subtree.c:490-494`): Internal nodes allocate `SubtreeHeapData` at the end of the children array, so one `malloc` covers both the children and the parent metadata.

5. **Reference counting** (`subtree.c:558-594`): `ts_subtree_retain` increments, `ts_subtree_release` decrements and frees to pool. This gives deterministic memory reclamation with minimal latency.

**Allocation frequency:**
- **Leaf tokens**: ~0 heap allocs for inline tokens, ~1 pool alloc for heap tokens
- **Internal nodes**: ~1 realloc per reduction (growing children array to fit parent data)
- **Stack nodes**: ~1 pool alloc per push (amortized)
- **Total**: For a 10K-token file, roughly 5K-15K allocations, most served from pools

**Why Go is fundamentally different:**
- **GC vs ref counting**: Go's GC gives us automatic memory management but adds pause latency and throughput overhead. For a parse tree with 100K+ nodes, each with pointers, GC tracing is significant.
- **No inline unions**: Can't store 8-byte inline data and a pointer in the same 8-byte word without `unsafe`.
- **Escape analysis limitations**: Go's escape analysis can keep small, short-lived allocations on the stack, but parse tree nodes live beyond the function that creates them, so they always escape to the heap.
- **Write barriers**: Every pointer write in Go has a ~1-2ns write barrier overhead for the GC. For building a parse tree (thousands of pointer writes), this adds up.

**Predicted gap:** Large to Fundamental (100-300%).

**Rationale:** Memory allocation is the area where Go's runtime model differs most fundamentally from C's. Every pointer write has a write barrier. Every object the GC must trace adds to pause time. C's pools + ref counting + inline subtrees give it a significant structural advantage.

**Mitigation (detailed in §3 and §4):**
- Arena allocation for subtree nodes (batch allocate, batch free)
- `unsafe.Pointer`-based tagged pointer for inline subtrees
- `sync.Pool` for `SubtreeHeapData` and `StackNode`
- Reduce pointer count in hot structs (use indices into arenas instead of pointers)
- Use `GOGC` tuning and `debug.SetMemoryLimit` to control GC frequency

---

## 2. Go vs C Gap Predictions

| Hot Path | Frequency | Predicted Gap | Category |
|---|---|---|---|
| 1a. Lexer inner loop | Per-character | 20-50% (naive) → **faster** (optimized) | Moderate → Go advantage |
| 1b. Character reading | Per-character | 20-50% | Moderate |
| 1c. External scanner | Per-external-token | 10-30% | Minimal-Moderate |
| 1d. Parse table lookup | Per-token | <20% | Minimal |
| 1e. Subtree creation | Per-token + per-reduction | 50-200% | Large |
| 1f. Stack operations | Per-parse-action | 20-50% | Moderate |
| 1g. Tree cursor | Per-node | 10-30% | Minimal-Moderate |
| 1h. Query matching | Per-visible-node | 15-30% | Minimal-Moderate |
| 1i. Memory allocation | Per-allocation | 100-300% | Large-Fundamental |

### Summary by Gap Category

**Minimal gap (<20%):** Parse table lookup, tree cursor (basic traversal). These are simple array accesses where Go's overhead is bounded and small.

**Moderate gap (20-50%):** Lexer inner loop (before optimization), character reading, stack operations. These involve tight loops with bounds checks that add constant overhead per iteration.

**Large gap (50-200%):** Subtree creation, memory allocation. These are structurally different — C uses tagged unions, free-list pools, and contiguous allocation that don't have direct Go equivalents.

**Fundamental gap (>200%):** Overall memory allocation patterns when GC tracing dominates. This is the area requiring the most architectural work.

---

## 3. SIMD/Assembly Optimization Plan

### 3a. Lexer Character Classification (AVX2/NEON)

**Covered in detail in `docs/addendum-lexer-performance.md`.** Summary:

**Target operation:** Classify input characters into equivalence classes for the lex state machine.

**SIMD approach (AVX2):**
- `VPSHUFB` for nibble-indexed table lookup (classifies 32 bytes at once)
- 4-instruction sequence: load, low-nibble shuffle, high-nibble shuffle, AND
- Expected throughput: 32 bytes/cycle → 30+ GB/s on modern CPUs

**SIMD approach (ARM NEON):**
- `TBL`/`TBX` for 16-byte table lookup
- Similar nibble-decomposition approach
- Expected throughput: 16 bytes/cycle

**Go assembly:**
- Use Avo code generator for maintainable Plan9 assembly
- Separate files: `lexer_amd64.s`, `lexer_arm64.s`
- Fallback: `lexer_generic.go` with pure-Go implementation

**Expected speedup:** 3-10x for the lexer character classification step. This is the single biggest optimization opportunity and can make Go's lexer *faster than C's*.

**Implementation complexity:** Medium. The Avo generator makes AVX2/NEON assembly maintainable. The main challenge is ensuring correct behavior at chunk boundaries and for multi-byte UTF-8 characters.

---

### 3b. SIMD-Accelerated Position Updates After Edit

**Target operation:** After an edit, all subtree nodes beyond the edit point need their byte offsets and row/column positions updated. In C, this is done by `ts_subtree_edit` walking the tree recursively. The position update itself is just integer addition.

**Current C approach** (`subtree.c:633-786`): Iterative stack-based walk, updating `padding` and `size` fields of each affected node.

**SIMD approach (AVX2):**
- If subtree nodes are stored in contiguous arena memory, the position fields (byte_offset, row, column) can be updated in bulk
- Pack position data as `(uint32_t offset, uint32_t row, uint32_t column)` = 12 bytes per node
- `VPADDD` (packed 32-bit add): update 8 nodes' offsets simultaneously
- For row/column updates: only applicable when the edit doesn't cross line boundaries (the common case for single-character edits)

**Data layout requirement:**
- Positions must be stored in a Structure-of-Arrays layout for SIMD to help:
  - `offsets: [N]uint32` — byte offsets
  - `rows: [N]uint32` — row numbers
  - `columns: [N]uint32` — column numbers
- This conflicts with the natural AoS (Array-of-Structures) layout of subtree nodes

**When this helps:**
- Large files with edits near the beginning (many nodes need updating)
- Bulk edit operations (e.g., find-replace changing line lengths)
- The common case of single-line edits (only byte offsets change, no row/column adjustment needed for nodes on other lines)

**When this doesn't help:**
- Small edits near the end of the file (few nodes to update)
- Multi-line edits (row/column update logic is complex, not easily vectorized)
- Edits during incremental re-parse where the tree walk is needed anyway

**Expected speedup:** 2-4x for the position update phase, but only when many nodes need updating. This is typically a small fraction of total parse time.

**Implementation complexity:** Medium-High. Requires either SoA layout for positions (complicating the main data structures) or a gather/scatter approach (which reduces SIMD benefits). Likely only worthwhile after profiling confirms position updates are a bottleneck.

**Verdict:** Tier 3 — only if profiling confirms.

---

### 3c. SIMD-Accelerated UTF-8 Validation and Decoding

**Target operation:** Decode UTF-8 characters from the input stream. Current C implementation (`ts_decode_utf8`) decodes one character at a time.

**SIMD approach (AVX2):**
- Use the algorithm from simdjson / simdutf: classify leading bytes with `VPSHUFB`, validate continuation bytes with `VPCMPEQB` + mask operations
- For pure ASCII detection: `VPMOVMSKB` on 32 bytes, if all zero → all ASCII, skip decoding entirely
- When all-ASCII, advance 32 characters in one shot instead of decoding one at a time

**Expected speedup for ASCII:** 10-30x (32 characters at once vs 1 at a time). Since >90% of source code is ASCII, this is extremely effective.

**Expected speedup for mixed UTF-8:** 2-4x. Still validates 32 bytes at once, but extraction of individual codepoints requires scalar work.

**Go assembly approach:**
```
// Pure-ASCII fast path (Plan9 assembly pseudocode)
// VMOVDQU chunk, 0(input)     // load 32 bytes
// VPMOVMSKB mask, chunk       // extract high bits
// TESTL mask, mask             // if zero, all ASCII
// JNZ slow_path
// (advance by 32, return all-ASCII indicator)
```

**Implementation complexity:** Medium. The ASCII fast path is simple. Full UTF-8 validation with SIMD is complex but well-documented (simdutf library provides reference).

**Verdict:** Tier 1 for ASCII fast path (simple, huge win), Tier 2 for full SIMD UTF-8 validation.

---

### 3d. SIMD-Accelerated Whitespace Skipping

**Target operation:** The lexer skips whitespace at the beginning of each token (spaces, tabs, newlines). In C, this is done character-by-character in the generated `lex_fn`.

**SIMD approach (AVX2):**
- Load 32 bytes from input
- Compare against space (0x20), tab (0x09), newline (0x0A), carriage return (0x0D) using `VPCMPEQB` × 4 + `VPOR`
- `VPMOVMSKB` to get a bitmask of whitespace bytes
- `TZCNT` (trailing zero count) to find the first non-whitespace byte
- Single-pass: skip entire whitespace run in one operation

**Expected speedup:** 5-20x for whitespace-heavy code (Python, formatted code with lots of indentation). For code with minimal whitespace (minified JS), negligible benefit.

**Implementation complexity:** Low. This is one of the simplest SIMD optimizations.

**Verdict:** Tier 2 — easy to implement, helps specific workloads.

---

## 4. Structural Optimizations (Go-Level)

### 4a. Struct Layout and Cache Alignment

**Problem:** Go structs have different alignment rules than C. Field ordering matters for cache line utilization.

**Specific changes:**

**SubtreeHeapData layout:**
```go
// Optimized field ordering for 64-byte cache line alignment
type SubtreeHeapData struct {
    // Hot fields (accessed on every node visit) — first cache line
    symbol     uint16     // 2 bytes
    parseState uint16     // 2 bytes
    childCount uint32     // 4 bytes
    padding    Length     // 12 bytes (bytes uint32, row uint32, col uint32)
    size       Length     // 12 bytes
    flags      uint32     // 4 bytes (bitfield: visible, named, extra, etc.)
    refCount   uint32     // 4 bytes (if using ref counting hybrid)
    // Total: 40 bytes — fits in one cache line with room to spare

    // Cold fields (accessed rarely) — second cache line
    errorCost              uint32
    lookaheadBytes         uint32
    visibleChildCount      uint32
    namedChildCount        uint32
    visibleDescendantCount uint32
    dynamicPrecedence      int32
    repeatDepth            uint16
    productionID           uint16
    firstLeaf              struct{ Symbol uint16; ParseState uint16 }
}
```

**Key insight:** Hot fields (symbol, parseState, childCount, padding, size) are accessed on nearly every node operation. Cold fields (errorCost, dynamicPrecedence) are only accessed during error recovery or ambiguity resolution. Separating them across cache lines means the common path only loads one cache line.

**StackNode layout:**
```go
type StackNode struct {
    // Hot fields
    state             uint16
    linkCount         uint16
    position          Length   // 12 bytes
    links             [8]StackLink

    // Cold fields
    refCount          uint32
    errorCost         uint32
    nodeCount         uint32
    dynamicPrecedence int32
}
```

---

### 4b. Reducing Interface Dispatch on Hot Paths

**Problem:** Go interface method calls are ~3-5x slower than direct method calls (itab lookup + indirect branch). On hot paths called millions of times, this matters.

**Where interface dispatch occurs:**
1. **TSLexer**: In C, the `advance` and `eof` functions are stored as function pointers in the `TSLexer` struct. In Go, this would naturally be an interface.
2. **ExternalScanner**: The `Scan`, `Serialize`, `Deserialize` methods.
3. **Input reader**: The `Read` callback that provides chunks of source code.

**Mitigation:**

1. **TSLexer**: Don't use an interface. Use a concrete `Lexer` struct with direct methods. The generated `lex_fn` receives a `*Lexer`, not a `TSLexer` interface. This eliminates interface dispatch from the innermost loop.

2. **ExternalScanner**: Accept the interface dispatch here — it's called once per external token, and the scanner's own work dominates. But provide a type assertion fast path:
```go
// In the parser's lex function:
if scanner, ok := p.externalScanner.(*ConcreteScanner); ok {
    // Direct call — no interface dispatch
    scanner.Scan(lexer, validTokens)
} else {
    // Interface dispatch — fallback
    p.externalScanner.Scan(lexer, validTokens)
}
```

3. **Input reader**: For the common case (parsing a string), use a concrete `StringInput` type. Only fall back to the interface for streaming input.

---

### 4c. Escape Analysis and Stack Allocation

**Problem:** Go's escape analysis determines whether an allocation can live on the stack (cheap) or must go on the heap (expensive, GC-tracked). Parse tree nodes always escape because they're stored in arrays and returned to callers.

**Where to focus:**
- **Temporary buffers**: The iterator arrays in `stack__iter`, the scratch trees in reduce operations, and the cursor stack in tree traversal can all be stack-allocated if they don't escape.
- **Length structs**: `Length{bytes, row, col}` should be passed by value, not by pointer, to keep them on the stack.
- **TableEntry results**: Parse table lookup results should be returned by value.

**Techniques:**
```go
// BAD: array escapes to heap
func (s *Stack) iter(version int) []StackSlice {
    iterators := make([]StackIterator, 0, 64)  // escapes
    // ...
    return slices
}

// GOOD: pre-allocated on the struct, no escape
func (s *Stack) iter(version int) []StackSlice {
    s.iterators = s.iterators[:0]  // reuse existing backing array
    // ...
    return s.slices
}
```

**Specific functions to optimize:**
- `Stack.iter` — reuse `stack.iterators` slice on the `Stack` struct
- `TreeCursor.iterateChildren` — the `CursorChildIterator` is a value type, keep it on the stack
- `Parser.lex` — the local variables `padding`, `size`, `lookaheadEndByte` are all value types, stay on stack automatically

---

### 4d. `sync.Pool` Usage Patterns

**Where to use `sync.Pool`:**

1. **SubtreeHeapData**: Pool of `*SubtreeHeapData`. Get/Put on create/release.
```go
var subtreePool = sync.Pool{
    New: func() interface{} { return new(SubtreeHeapData) },
}
```

2. **StackNode**: Pool of `*StackNode`. Same pattern.

3. **Children arrays**: Pool of `[]Subtree` slices at common sizes (4, 8, 16, 32).
```go
var childrenPool4 = sync.Pool{
    New: func() interface{} { return make([]Subtree, 0, 4) },
}
```

**Where NOT to use `sync.Pool`:**
- **Inline subtrees**: These are 8-byte values, no allocation needed
- **TreeCursorEntry**: Stack-allocated, no pooling needed
- **QueryState**: Stored in a reusable array on the cursor, no pooling needed

**Caveat:** `sync.Pool` items can be reclaimed at any GC cycle. For sustained parsing workloads, this is fine — the pool stays warm. For one-shot parse-then-discard workloads, the pool doesn't help much. Consider a manual free-list (fixed-size array of pointers) for the subtree pool, matching C's `TS_MAX_TREE_POOL_SIZE` = 32 cap.

---

### 4e. Unsafe Tricks for Hot Paths

These should only be used if profiling confirms the overhead they address:

**1. Tagged pointer for inline subtrees:**
```go
// Subtree is a single unsafe.Pointer
// If lowest bit is 1, it's inline data (shift right 1 to get data)
// If lowest bit is 0, it's a *SubtreeHeapData pointer
type Subtree struct {
    raw uintptr
}

func (s Subtree) isInline() bool {
    return s.raw&1 != 0
}

func (s Subtree) inlineData() SubtreeInlineData {
    return *(*SubtreeInlineData)(unsafe.Pointer(s.raw &^ 1))
}
```

**Risk:** Unsafe. GC can't trace the pointer if it's been manipulated. This requires either:
- Keeping a separate `[]SubtreeHeapData` arena that the GC can trace, and using indices instead of pointers
- Using `runtime.KeepAlive` carefully

**Better approach:** Use an index-based scheme:
```go
type Subtree struct {
    index  uint32  // index into SubtreeArena, or inline data
    inline bool
}
```
This is 8 bytes, safe, and GC-friendly (the arena holds the actual data).

**2. Bounds-check-free slice access:**
```go
func unsafeSliceIndex(s []Subtree, i int) *Subtree {
    return (*Subtree)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(s)), uintptr(i)*unsafe.Sizeof(Subtree{})))
}
```

**Risk:** Out-of-bounds access causes memory corruption instead of panic. Only use in code proven correct by invariants (e.g., `i < len(s)` checked once at loop entry).

---

## 5. Prioritized Optimization Plan

### Priority Score: Frequency × Gap × Feasibility

| Rank | Hot Path | Score | Reasoning |
|---|---|---|---|
| 1 | Lexer inner loop (SIMD) | High × High × High | Per-char, can beat C, well-understood |
| 2 | Memory allocation (arena) | High × Very High × Medium | Affects everything, fundamental gap |
| 3 | Subtree creation (inline) | High × High × Medium | Per-token, tagged pointer/arena critical |
| 4 | Character reading (ASCII) | High × Moderate × High | Per-char, ASCII fast path is simple |
| 5 | UTF-8 SIMD fast path | High × Moderate × Medium | Per-char, huge win for ASCII |
| 6 | Stack operations (pooling) | Medium × Moderate × High | Per-action, straightforward pool |
| 7 | Whitespace SIMD skip | Medium × Moderate × Medium | Per-token, easy SIMD |
| 8 | External scanner dispatch | Low-Medium × Low × High | Per-ext-token, already small gap |
| 9 | Tree cursor | Low × Low × High | Per-node, gap is minimal |
| 10 | Position updates (SIMD) | Low × Moderate × Low | Incremental only, complex layout |

### Tier 1: Fix Before v1.0

These have the largest impact and should be addressed in the initial implementation:

1. **Arena allocation for subtree nodes** (§4d, §1i)
   - Implement `SubtreeArena` that batch-allocates `SubtreeHeapData` structs
   - Use index-based `Subtree` type (uint32 index into arena) instead of pointers
   - This eliminates GC write barriers for subtree pointer updates and reduces GC tracing cost
   - **Expected impact:** 2-3x reduction in GC pressure, 30-50% faster tree building

2. **Inline subtree representation** (§1e)
   - Implement 8-byte inline data for small leaf tokens (matching C's `SubtreeInlineData`)
   - Use either tagged-pointer (`unsafe`) or index-based discrimination
   - **Expected impact:** Eliminates 60-80% of subtree heap allocations

3. **Lexer equivalence-class tables** (§1a, see lexer addendum Phase 0)
   - Replace C's `ADVANCE_MAP` linear scan with O(1) table lookup
   - This alone makes the Go lexer competitive with C
   - **Expected impact:** Lexer 20-40% faster than C's `ADVANCE_MAP` approach

4. **Concrete types on hot paths** (§4b)
   - Use `*Lexer` (not interface) for the generated lex function
   - Use `*StringInput` for the common string-parsing case
   - **Expected impact:** Eliminates ~3ns per lex call (millions of calls per file)

5. **ASCII fast path for character reading** (§1b)
   - Before calling `utf8.DecodeRune`, check `byte < 0x80`
   - For ASCII: `lookahead = int32(byte); lookaheadSize = 1` — one comparison + assignment
   - **Expected impact:** 2-3x faster character reading for typical source code

### Tier 2: Fix in v1.x

These are significant optimizations but require more implementation work or SIMD:

6. **SIMD lexer character classification** (§3a)
   - AVX2 `VPSHUFB` for 32-byte-at-a-time character classification
   - NEON `TBL` for ARM equivalent
   - Go assembly via Avo code generator
   - **Expected impact:** 3-10x faster lexer, definitively faster than C

7. **SIMD ASCII detection** (§3c)
   - `VPMOVMSKB` to detect all-ASCII chunks of 32 bytes
   - Skip per-character UTF-8 decode entirely for ASCII runs
   - **Expected impact:** 10-30x for ASCII content (which is >90% of source code)

8. **Stack node pooling** (§1f, §4d)
   - Manual free-list for `StackNode` (matching C's 50-node pool)
   - Reuse iterator arrays across operations
   - **Expected impact:** 20-30% faster GLR operations, reduced GC pressure

9. **SIMD whitespace skipping** (§3d)
   - Batch-skip whitespace with SIMD comparisons
   - **Expected impact:** 5-20x for whitespace-heavy code, minimal for dense code

10. **Struct layout optimization** (§4a)
    - Reorder fields for cache line alignment
    - Separate hot/cold fields in `SubtreeHeapData`
    - **Expected impact:** 5-15% improvement in cache hit rate

### Tier 3: Only If Profiling Confirms

These are predicted but uncertain — implement only after benchmarks validate the prediction:

11. **SIMD position updates** (§3b)
    - Bulk update byte offsets after edits
    - Requires SoA layout or gather/scatter, complex
    - **Expected impact:** 2-4x for position updates, but small fraction of total time

12. **Unsafe bounds-check elimination** (§4e)
    - Replace bounds-checked slice access with pointer arithmetic on proven-safe hot paths
    - **Expected impact:** 5-10% on inner loops, but high maintenance cost

13. **Query execution optimization** (§1h)
    - Fixed-size state arrays, pooled capture lists
    - **Expected impact:** 10-20% faster queries

---

## 6. Validation Methodology

### Benchmark Design

For each hot path, create targeted micro-benchmarks:

```go
// Example: Character reading benchmark
func BenchmarkLexerAdvance(b *testing.B) {
    input := loadTestFile("large_javascript.js")  // ~500KB
    lexer := NewLexer(input)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        lexer.Reset()
        for !lexer.EOF() {
            lexer.Advance(false)
        }
    }
}
```

### Measurement Tools

1. **`go test -bench` with `-benchmem`**: Allocation counts and bytes per operation
2. **`go tool pprof` CPU profiles**: Identify actual hotspots vs predictions
3. **`perf stat`** (Linux) or **Instruments** (macOS): Hardware performance counters
   - Cache miss rate (L1/L2/L3)
   - Branch misprediction rate
   - Instructions per cycle
4. **`go tool trace`**: GC pause frequency and duration
5. **`runtime.MemStats`**: Heap size, GC cycles, allocation rate

### Comparison vs C Tree-Sitter

```bash
# C baseline (using tree-sitter CLI)
time tree-sitter parse large_file.js --quiet

# Go implementation
time go-tree-sitter parse large_file.js

# Detailed comparison
hyperfine --warmup 3 \
  'tree-sitter parse large_file.js --quiet' \
  'go-tree-sitter parse large_file.js'
```

### Expected Results by Tier

| Tier | Without Optimization | With Optimization |
|---|---|---|
| Naive port (no optimizations) | 2-4x slower than C | — |
| Tier 1 complete | — | 1.0-1.5x slower than C |
| Tier 1 + Tier 2 complete | — | 0.7-1.2x of C (parity or faster) |
| All tiers complete | — | 0.5-0.9x of C (definitively faster) |

The key insight: C tree-sitter has known performance weaknesses (linear-scan `ADVANCE_MAP`, no SIMD, single-threaded). By combining Go's strengths (efficient concurrency, simple GC for persistent trees) with targeted assembly for hot paths, we can build a parser that is not just competitive with C but measurably faster for common workloads.

---

## 7. Key References

### C Tree-Sitter Source (Analyzed)
- `lib/src/parser.c` — GLR parsing engine, subtree reuse, token caching
- `lib/src/lexer.c` — Character reading, UTF-8 decode, position tracking
- `lib/src/subtree.c` — Subtree creation, ref counting, edit propagation
- `lib/src/subtree.h` — Inline subtree layout, SubtreePool definition
- `lib/src/stack.c` — Graph-Structured Stack, node pooling, merge/split
- `lib/src/language.c` — Parse table lookup, state transitions
- `lib/src/tree_cursor.c` — Stack-based tree traversal
- `lib/src/query.c` — Query pattern matching, state machine execution

### Go Performance
- "High Performance Go" (Dave Cheney) — escape analysis, bounds check elimination
- Go compiler source: `cmd/compile/internal/ssa` — understanding Go's optimization passes
- `unsafe` package documentation — when and how to use unsafe operations correctly
- Avo project (https://github.com/mmcloughlin/avo) — Go assembly code generator

### SIMD References
- Intel Intrinsics Guide — AVX2 instruction reference
- ARM NEON Programmer's Guide — NEON instruction reference
- simdjson project — practical SIMD techniques for parsing
- simdutf project — SIMD UTF-8 validation algorithms
