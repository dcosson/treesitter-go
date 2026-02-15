# Addendum: Incremental Parsing Optimization for treesitter-go

*Addendum to [docs/design.md](design.md) — focused on understanding C tree-sitter's incremental parsing internals and designing advanced algorithms to surpass its performance in Go.*

---

## 1. How C Tree-sitter's Incremental Parsing Actually Works

### Overview

Incremental parsing is tree-sitter's defining feature. When source code is edited, the parser reuses unchanged subtrees from the previous parse, only re-parsing regions that changed. For a single-character edit in a 10,000-line file, ~99.99% of subtrees are reused, making re-parse take microseconds instead of milliseconds.

The algorithm has three major phases:

1. **Edit the old tree** — propagate edit information through the tree structure
2. **Reuse subtrees during parsing** — skip unchanged regions by pushing entire old subtrees onto the parse stack
3. **Detect changed ranges** — walk both old and new trees in parallel to compute what changed

### Phase 1: Edit Propagation (`subtree.c: ts_subtree_edit`)

When the user calls `Tree.Edit(edit)`, the edit (an `InputEdit` with start/old_end/new_end byte+point coordinates) is propagated through the old tree using an **iterative stack-based traversal**. The C implementation in `ts_subtree_edit` (subtree.c:633-786) works as follows:

```c
// Stack entry for iterative tree edit
typedef struct {
  Subtree *tree;
  Edit edit;
} EditEntry;
```

The algorithm pushes the root and its edit onto a stack, then iterates:

**For each node on the stack:**

1. **Check for no-op**: If `old_end == start == new_end`, the edit doesn't affect this node (skip).

2. **Adjust padding and size based on edit position relative to the node:**
   - **Edit entirely in padding** (`old_end <= padding.bytes`): Shift the node — adjust padding by the edit delta without changing size.
   - **Edit starts in padding, extends into content** (`start < padding.bytes`): Shrink content by the overlap amount, set padding to `new_end`.
   - **Edit within content** (`start < total_size.bytes`): Resize the content to reflect the insertion/deletion.

3. **Copy-on-write via `ts_subtree_make_mut`**: The node is cloned only if its reference count is >1 (shared). If the node is exclusively owned (refcount == 1), it's modified in-place.

4. **Set `has_changes = true`**: This flag is the primary signal to the parser that this subtree needs re-parsing.

5. **Propagate to children**: For each child that overlaps the edit range:
   - Transform the edit coordinates into the child's local coordinate space
   - Handle `depends_on_column` invalidation: if the edit shifts column positions and a child depends on its column (e.g., Python indentation), continue invalidating past the edit
   - Push the child and its transformed edit onto the stack
   - The first child that overlaps the edit "absorbs" the insertion (`edit.new_end = edit.start` for subsequent children) — later children only shrink

**Key property**: Only nodes on the path from root to the edited range are cloned. Sibling subtrees are shared by pointer. This is O(depth) = O(log N) work for a tree with N nodes.

**Column dependency handling**: The `depends_on_column` flag (set during `ts_subtree_summarize_children`) tracks whether a subtree's interpretation depends on its column position. This is critical for languages like Python where indentation is significant. When an edit shifts columns, child nodes with this flag set are invalidated even if they don't directly overlap the edit.

### Phase 2: Subtree Reuse During Parsing (`parser.c: ts_parser__reuse_node`)

During parsing, when an `old_tree` is provided, the parser uses a `ReusableNode` iterator to walk the old tree in document order. The `ReusableNode` (defined in `reusable_node.h`) maintains a stack of `(tree, child_index, byte_offset)` entries for depth-first traversal:

```c
typedef struct {
  Array(StackEntry) stack;
  Subtree last_external_token;
} ReusableNode;
```

The reuse logic in `ts_parser__reuse_node` (parser.c:753-830) works as follows:

**At each parse position**, the parser checks the old tree for a reusable subtree:

```
while (result = reusable_node_tree(&self->reusable_node)).ptr:
    byte_offset = reusable_node_byte_offset(...)

    // 1. Skip past nodes that end before current position
    if byte_offset < position:
        descend or advance past this node
        continue

    // 2. No reusable node at this position
    if byte_offset > position:
        break  // fall through to lexer

    // 3. Node starts at current position — check reusability
    // A subtree is NOT reusable if ANY of:
    //   a) External scanner state doesn't match
    //   b) has_changes flag is true
    //   c) Is an error node
    //   d) Is a missing node
    //   e) Is fragile (from GLR ambiguity)
    //   f) Overlaps an included_range_difference

    if (not reusable):
        descend into children (may find reusable subtrees within)
        continue

    // 4. Check first leaf compatibility
    leaf_symbol = ts_subtree_leaf_symbol(result)
    table_entry = lookup(state, leaf_symbol)
    if !ts_parser__can_reuse_first_leaf(state, result, table_entry):
        advance past this leaf
        break

    // 5. Reuse! Push entire subtree onto the parse stack
    return result
```

**The `ts_parser__can_reuse_first_leaf` check** (parser.c:470-502) is crucial. It verifies:
- The lex mode for the current parse state matches the lex mode for the state where this token was originally created
- The token is not a keyword that needs re-checking (keyword capture tokens with different parse states)
- For empty tokens: only reusable in states with the same lookaheads
- For non-empty tokens: reusable if the current state has no external tokens and the table entry is marked `reusable`

**When a subtree is reused**, the parser calls `ts_parser__shift` which pushes the entire subtree onto the parse stack in one step, effectively jumping over all the text it covers. The `ReusableNode` iterator then advances past this subtree.

**When a subtree is NOT reusable**, the parser descends into its children looking for smaller reusable subtrees, or falls through to the lexer to re-scan the text.

### How the Parser Integrates Reuse (`parser.c: ts_parser__advance`)

The main parse loop in `ts_parser__advance` (parser.c:1557-1764) integrates incremental reuse:

```c
bool ts_parser__advance(TSParser *self, StackVersion version, bool allow_node_reuse) {
    TSStateId state = ts_stack_state(self->stack, version);
    uint32_t position = ts_stack_position(self->stack, version).bytes;

    // Step 1: Try to reuse a node from the old tree
    if (allow_node_reuse) {
        lookahead = ts_parser__reuse_node(self, version, &state, position, ...);
    }

    // Step 2: Try the token cache
    if (!lookahead.ptr) {
        lookahead = ts_parser__get_cached_token(self, state, position, ...);
    }

    // Step 3: Fall back to lexing
    if (!lookahead.ptr) {
        lookahead = ts_parser__lex(self, version, state);
    }

    // Step 4: Process parse actions (shift/reduce/accept/recover)
    // ... standard GLR processing ...
}
```

**Note**: `allow_node_reuse` is only true when there's a single stack version (`version_count == 1`). During GLR ambiguity (multiple active versions), reuse is disabled because each version may need different tokens.

### Phase 3: Changed Range Detection (`get_changed_ranges.c`)

After parsing with an old tree, `ts_subtree_get_changed_ranges` (get_changed_ranges.c:413-557) computes which regions of the document changed between the old and new trees. This is exposed via `Tree.ChangedRanges(oldTree)`.

The algorithm uses two `Iterator` structs that walk old and new trees simultaneously in a parallel depth-first traversal:

```c
typedef struct {
  TreeCursor cursor;           // stack-based tree traversal
  const TSLanguage *language;
  unsigned visible_depth;      // current depth in visible nodes
  bool in_padding;             // currently in a node's padding region?
  Subtree prev_external_token; // for external scanner state comparison
} Iterator;
```

**The comparison logic** (`iterator_compare`, get_changed_ranges.c:348-395) classifies each pair of nodes:

- **`IteratorMatches`**: Nodes are identical (same symbol, size, parse state, error cost, no changes, matching external scanner state)
- **`IteratorMayDiffer`**: Nodes might differ internally (different size, different state, has_changes set, or different external scanner state) — descend to find the actual differences
- **`IteratorDiffers`**: Nodes are definitely different (different symbols or alias symbols)

**The main loop:**

1. Compare the current old and new subtrees
2. If they match: skip to the end of both subtrees (no change)
3. If they may differ: descend into both trees to find the actual change boundary
4. If they differ: record this range as changed, advance both iterators
5. Keep both iterators at the same visible depth by ascending as needed
6. Merge adjacent changed ranges

### Memory Management Details

**Reference counting and copy-on-write**: C tree-sitter uses atomic reference counting on `SubtreeHeapData`. The `ts_subtree_make_mut` function (subtree.c:284-290) checks if `ref_count == 1`. If so, it's safe to modify in-place. Otherwise, it clones the subtree and decrements the original's refcount:

```c
MutableSubtree ts_subtree_make_mut(SubtreePool *pool, Subtree self) {
  if (self.data.is_inline) return (MutableSubtree) {self.data};
  if (self.ptr->ref_count == 1) return ts_subtree_to_mut_unsafe(self);
  MutableSubtree result = ts_subtree_clone(self);
  ts_subtree_release(pool, self);
  return result;
}
```

**Subtree cloning** (`ts_subtree_clone`, subtree.c:260-277): Allocates `children_count * sizeof(Subtree) + sizeof(SubtreeHeapData)` bytes, copies the entire children array and heap data, retains all children, and sets `ref_count = 1`.

**SubtreePool**: A free-list of up to 32 `SubtreeHeapData` entries. `ts_subtree_pool_allocate` pops from the free list (or mallocs). `ts_subtree_pool_free` pushes back (or frees if pool is full).

**Subtree release** (`ts_subtree_release`, subtree.c:565-594): Uses an iterative (not recursive) approach with a stack to avoid stack overflow on deep trees. Decrements refcount; if it reaches 0, pushes children onto the release stack and either returns the node to the pool (leaves) or frees the children array (internal nodes).

### Position Update Propagation

Tree-sitter tracks positions as `Length` structs containing both byte offsets and `{row, column}` points. During edit propagation:

- **Byte offsets**: Updated arithmetically based on `new_end - old_end` delta
- **Row/column points**: Updated using `length_add` and `length_sub` which handle row-wrapping correctly (adding a length with row > 0 resets the column to the new length's column)
- **Saturating subtraction**: `length_saturating_sub` prevents underflow when edits shrink content

The dual tracking (bytes + points) is essential because editors work in both coordinate systems, and tree-sitter needs to report changes in both.

### Tree Balancing (`parser.c: ts_parser__balance_subtree`)

After parsing completes, tree-sitter rebalances the tree to handle pathological cases from left-recursive repetitions. The `ts_subtree_compress` function (subtree.c:292-336) performs AVL-like rotations on repetition nodes:

For a left-heavy tree like:
```
    R
   / \
  R   c
 / \
R   b
```

It rotates to:
```
    R
   / \
  a   R
     / \
    b   c
```

This ensures that cursor traversal and editing remain O(log N) even for deeply nested repetitions (e.g., a file with thousands of consecutive statements).

---

## 2. Baseline: Direct Port to Go

### What a Faithful Port Looks Like

A direct Go port would replicate the C algorithm almost exactly:

```go
type reusableNode struct {
    stack              []reusableNodeEntry
    lastExternalToken  *Subtree
}

type reusableNodeEntry struct {
    tree       *Subtree
    childIndex uint32
    byteOffset uint32
}

func (p *Parser) reuseNode(version StackVersion, state *StateID,
    position uint32, lastExternalToken *Subtree, tableEntry *TableEntry) *Subtree {
    for {
        result := p.reusableNode.tree()
        if result == nil { return nil }
        byteOffset := p.reusableNode.byteOffset()
        // ... same logic as C ...
    }
}
```

The parse tables, lex functions, and grammar data are identical — the incremental parsing algorithm is entirely in the runtime, not the grammar.

### Go GC vs C Reference Counting

**C approach**: Manual reference counting with atomic increment/decrement. `ts_subtree_retain` increments, `ts_subtree_release` decrements and frees when count reaches 0. This gives deterministic destruction and enables the copy-on-write optimization (check refcount == 1 for exclusive ownership).

**Go approach**: Eliminate reference counting entirely. Go's garbage collector handles lifetime. The key difference is how we detect exclusive ownership for copy-on-write:

| Aspect | C | Go |
|--------|---|-----|
| Lifetime management | Manual refcounting | GC |
| Copy-on-write check | `refcount == 1` | Must use alternative strategy |
| Deallocation | Immediate when refcount hits 0 | Deferred to GC cycle |
| Memory overhead | 4 bytes per node (refcount field) | GC metadata (~16 bytes per pointer) |
| Concurrency cost | Atomic inc/dec per retain/release | None (GC is concurrent) |

**The copy-on-write problem**: In Go, we can't check "am I the only owner?" because there's no refcount. Options:

1. **Always clone on edit path** (recommended for Phase 0): During `Tree.Edit()`, always clone nodes on the edit path. Since the edit path is O(depth) = O(log N), this is cheap — for a 10K-line file with ~15 tree depth, we clone ~15 nodes.

2. **Explicit ownership flag**: Set an `owned` flag when a tree is created by the parser. Clear it when `Tree.Edit()` is called (marking the tree as shared). Trees with `owned == true` can be modified in place. This approximates C's refcount optimization without the overhead.

3. **Immutable trees**: Treat all trees as immutable. `Tree.Edit()` always creates a new tree sharing unchanged subtrees. This is the cleanest approach and aligns with Go idioms.

**Recommendation**: Option 3 (immutable trees). The cost of cloning ~15 nodes per edit is negligible compared to the re-parse work. This eliminates an entire class of bugs (mutation of shared data) and simplifies the GC's job (no mutation means better cache behavior during GC marking).

### sync.Pool for Subtree Allocation vs C's SubtreePool

C's `SubtreePool` is a free-list of up to 32 `SubtreeHeapData` entries. This avoids malloc/free for the hot path of creating and releasing subtree leaves.

In Go, `sync.Pool` serves a similar purpose:

```go
var subtreePool = sync.Pool{
    New: func() any { return new(Subtree) },
}
```

**Key difference**: `sync.Pool` items may be collected between GC cycles, so it doesn't guarantee reuse. During incremental parsing, the time between creating new nodes and releasing old nodes is short (within one parse call), so pool items are likely to be reused.

**Alternative — arena allocation**: Pre-allocate a `[]Subtree` backing array and slice from it. This avoids individual heap allocations entirely:

```go
type subtreeArena struct {
    nodes []Subtree
    pos   int
}

func (a *subtreeArena) alloc() *Subtree {
    if a.pos >= len(a.nodes) {
        a.nodes = make([]Subtree, max(1024, len(a.nodes)*2))
        a.pos = 0
    }
    s := &a.nodes[a.pos]
    a.pos++
    return s
}
```

Arena allocation has much better GC characteristics: the GC sees one large slice instead of thousands of individual pointers. See Section 3f for a detailed analysis.

### Interface Dispatch Overhead for the Tree Cursor

The `TreeCursor` in C uses direct struct access — no virtual dispatch. In Go, if we define `TreeCursor` operations on an interface, each method call incurs ~2ns of overhead (indirect call through interface vtable).

For tree cursor traversal used during incremental change detection and query execution, this adds up: walking a 10K-node tree with 4 method calls per node = 40K interface calls = ~80 microseconds of pure dispatch overhead.

**Mitigation**: Use concrete types, not interfaces, on hot paths. The `TreeCursor` should be a concrete struct with direct method calls:

```go
type TreeCursor struct {
    tree  *Tree
    stack []treeCursorEntry
}

// Direct method — no interface dispatch
func (c *TreeCursor) GotoFirstChild() bool { ... }
```

### Goroutine Scheduling vs C's Single-Threaded Model

C tree-sitter is single-threaded. The parser runs to completion without yielding. In Go, goroutines can be preempted at function call points. For a long parse (~10ms), the goroutine might be preempted and rescheduled multiple times.

This is unlikely to be a significant issue: Go's scheduler has ~1 microsecond context-switch overhead, and preemption happens at ~10ms intervals. For incremental re-parses (which complete in microseconds), preemption won't occur.

For from-scratch parses of large files (100ms+), preemption is actually beneficial — it prevents one parse from monopolizing a CPU core and blocking other goroutines.

---

## 3. Advanced Algorithms for Beating C's Incremental Parsing

### 3a. Self-Adjusting Computation (Acar et al.)

**The Adapton framework** models computations as dependency graphs where inputs flow through intermediate computations to outputs. When an input changes, only computations that transitively depend on the changed input are re-executed. The framework automatically tracks dependencies and invalidates stale computations.

**Modeling GLR parsing as self-adjusting computation**: The parser's state at each token position depends on:
- The current token (from the lexer)
- The parse state (from the stack, which depends on all preceding tokens)

A self-adjusting parser would memoize the mapping `(parse_state, token_position) -> (new_parse_state, parse_actions)`. When an edit changes tokens at positions [i, j], only parse steps that depend on those positions would be re-executed.

**Theoretical minimum re-parse work**: For a k-character edit, the minimum work is:
- O(k) to re-lex the changed region
- O(k + d) to re-parse, where d is the "change depth" — how far up the tree the edit's effects propagate
- In the best case (edit doesn't change tree structure), d = O(log N) for tree spine updates
- In the worst case (edit changes a global delimiter, e.g., removing a `{`), d = O(N)

**Practical assessment**: Tree-sitter's existing incremental parsing is already close to the theoretical minimum for common edits:
- It re-lexes only the changed region
- It reuses all subtrees outside the changed region
- The overhead is primarily in the reusability checks, not redundant parsing

Self-adjusting computation adds infrastructure overhead (dependency graph maintenance, memoization tables) that would exceed the savings for tree-sitter's use case. The key difference: Adapton is designed for arbitrary DAG computations, while tree-sitter's computation is a linear sequence of parse actions — the existing incremental approach is already near-optimal for this structure.

**Verdict: Not practical for parsing. The existing subtree-reuse approach is already optimal for the linear structure of LR parsing. Self-adjusting computation is better suited for DAG-structured computations like spreadsheets or build systems.**

*References: Acar et al. "Adaptive Functional Programming" POPL 2002; Hammer et al. "Adapton: Composable, Demand-Driven Incremental Computation" PLDI 2014.*

### 3b. Hash-Consed / Persistent Parse Trees

**The idea**: Use structural sharing between old and new trees by hash-consing subtrees. Two subtrees with the same structure (symbol, children, etc.) share the same memory, identified by a hash. After re-parsing, compare the new tree's root hash with the old tree's root hash — if equal, nothing changed structurally.

**How Go's GC enables this**: In C, structural sharing requires reference counting to manage lifetimes — each shared subtree needs its refcount incremented. In Go, the GC automatically keeps shared subtrees alive as long as either the old or new tree references them. This eliminates the per-node retain/release overhead that C pays.

**Hash-consing implementation**:

```go
type subtreeHash uint64

func hashLeaf(symbol Symbol, size, padding Length) subtreeHash {
    // FNV-1a or wyhash over fixed fields
    h := fnvBasis
    h = fnvMix(h, uint64(symbol))
    h = fnvMix(h, uint64(size.bytes))
    // ... padding, parse_state
    return subtreeHash(h)
}

func hashNode(symbol Symbol, childHashes []subtreeHash) subtreeHash {
    h := fnvBasis
    h = fnvMix(h, uint64(symbol))
    for _, ch := range childHashes {
        h = fnvMix(h, uint64(ch))
    }
    return subtreeHash(h)
}
```

**Detecting "no structural change" after re-parse**: If `newTree.rootHash == oldTree.rootHash`, the edit changed whitespace or positions but not the tree structure. This can be checked in O(1) and avoids the O(N) tree walk in `ChangedRanges`.

**Hash-based subtree reuse**: During re-parsing, when the parser creates a new subtree, check if its hash matches an old subtree's hash. If so, reuse the old subtree's pointer (sharing the entire subgraph). This extends tree-sitter's existing reuse mechanism from positional matching to structural matching.

**Where this helps beyond C tree-sitter**: C tree-sitter reuses subtrees based on position and parse state. If an edit inserts a blank line at the top of a file, all subtrees shift down by one line — their byte offsets change, so none are position-reusable, even though they're structurally identical. Hash-consing would detect that the subtrees below the inserted line are structurally unchanged and could be reused.

**Expected benefit**: Significant for edits that shift large portions of the tree without changing structure (inserting/deleting lines at the top, reformatting). Modest for typical character edits (where positional reuse already works well).

**Implementation complexity**: Low for the basic hash comparison. Medium for full hash-consed tree construction (requires computing hashes bottom-up for every new subtree).

**Verdict: High value for the "no structural change" fast path. Medium value for structural reuse during parsing. Implement hash comparison in Phase 1, full hash-consing in Phase 2.**

*References: Appel & Goncalves "Hash-consing garbage collection" 1993; Filliâtre & Conchon "Type-safe modular hash-consing" ML Workshop 2006.*

### 3c. Interval Tree / Spatial Indexing for Edit Region Identification

**The problem**: When an edit occurs, tree-sitter walks the tree top-down to find affected nodes. This is O(depth) per edit. For multiple edits, each edit triggers a separate walk.

**The idea**: Build an interval tree over node byte ranges. After an edit, query the interval tree for all nodes that overlap the edited range. This gives O(log N + k) lookup where k is the number of overlapping nodes, compared to O(depth) for tree walking.

**Analysis for typical parse trees**: Parse trees have depth ~15-30. The interval tree gives O(log N + k) where N might be 5000-50000 nodes. Since log(50000) ≈ 16, the asymptotic improvement is marginal.

**The constant factor**: Building and maintaining the interval tree adds:
- O(N log N) to build initially
- O(log N) per tree modification to update
- Memory for the index structure (~32 bytes per node for augmented BST)

For a single edit, the tree walk is already fast (O(depth) ≈ 15 node visits). The interval tree's advantage only appears with many simultaneous edits.

**When this wins**: Batch edits (find-replace across a file, multi-cursor editing). With M edits, tree walking costs O(M * depth) while an interval tree query costs O(M * log N + k_total). For M > 10 edits, the interval tree wins.

**When this loses**: Single edits (the common case in interactive editing). The tree walk is simpler, has lower constant factors, and requires no auxiliary data structure.

**Verdict: Not recommended for Phase 0-1. Consider for Phase 2 as part of the parallel multi-edit optimization (Section 3d). The common case of single edits doesn't benefit.**

### 3d. Parallel Multi-Edit Re-parsing

**The idea**: When multiple edits arrive simultaneously (common in LSP's `textDocument/didChange` with multiple content changes), process them in parallel on separate goroutines if their affected regions don't overlap.

**Current C behavior**: C tree-sitter processes edits sequentially. Multiple edits are applied one at a time with `ts_tree_edit`, each requiring a tree walk and position adjustment. Then a single re-parse handles all changes.

**Go opportunity**: With goroutines, we can parallelize:

1. **Edit application**: Apply non-overlapping edits to the tree simultaneously. Each goroutine handles one edit, cloning nodes on its edit path. Since non-overlapping edits touch different parts of the tree, there are no conflicts.

2. **Multi-region re-parsing**: If the changed regions are far apart (e.g., editing line 10 and line 500 in a 1000-line file), the parser could spawn goroutines to re-parse each region independently, then merge the results.

**Constraints on merging**: Parse state at the boundary between regions must be consistent. This works when:
- The regions are separated by at least one reusable subtree
- The parse state at the start of each region is determined by the (unchanged) context before it

In practice, most multi-edit operations (find-replace, multi-cursor) produce edits separated by unchanged code, so the constraints are satisfied.

**Implementation sketch**:

```go
func (p *Parser) parseMultiEdit(oldTree *Tree, edits []InputEdit, input Input) *Tree {
    // Sort edits by position
    sort.Slice(edits, func(i, j int) bool {
        return edits[i].StartByte < edits[j].StartByte
    })

    // Identify independent regions
    regions := identifyIndependentRegions(oldTree, edits)

    if len(regions) <= 1 {
        // Single region — standard incremental parse
        return p.parse(oldTree, input)
    }

    // Parse each region in parallel
    results := make([]*regionResult, len(regions))
    var wg sync.WaitGroup
    for i, region := range regions {
        wg.Add(1)
        go func(i int, r Region) {
            defer wg.Done()
            results[i] = p.parseRegion(oldTree, r, input)
        }(i, region)
    }
    wg.Wait()

    // Merge results
    return mergeRegionResults(oldTree, results)
}
```

**Expected speedup**: For M independent edit regions on a machine with P cores, theoretical speedup is min(M, P). For common multi-edit operations:
- Find-replace with 10 matches: up to 10x (limited by core count)
- Multi-cursor editing: up to cursor-count speedup
- Formatting: edits are often adjacent, limiting parallelism to ~2-4x

**Verdict: Medium-high value for multi-edit workflows. Implementation complexity is high due to region merging. Phase 2 optimization.**

*References: Wagner & Graham "Incremental Analysis of Real Programming Languages" 1998; Tim Wagner's PhD thesis on incremental parsing.*

### 3e. SIMD-Accelerated Position Updates

**The problem**: After an edit, all nodes after the edit point need their byte offsets and row/column positions adjusted. In C tree-sitter, this happens during the edit walk — only nodes on the edit path are adjusted (children of edited nodes inherit their parent's adjustments).

**Why SIMD doesn't help here**: Tree-sitter's position update is already optimal:
- Only O(depth) nodes are visited during edit propagation
- Each node's position is stored as `{bytes, {row, column}}` — 12 bytes
- The update is a simple addition: `new_position = old_position + delta`

For ~15 nodes on the edit path, SIMD parallelism over 15 twelve-byte updates yields no meaningful speedup — the operation completes in ~50 nanoseconds regardless.

**Where SIMD could help**: The `ChangedRanges` walk compares node positions in both trees. If this walk visits thousands of nodes, SIMD comparison of position arrays could help. But in practice, the `ChangedRanges` walk is also O(changed nodes), which is typically small.

**Bulk position adjustment scenario**: If we adopted the immutable tree approach (Section 2) and needed to update positions for all nodes below the edit point, we'd have N/2 nodes to update on average. For N = 10,000 nodes with 12-byte positions:
- Scalar: 10K iterations × ~2ns = 20 microseconds
- SIMD (4 nodes per iteration): 2.5K iterations × ~1ns = 2.5 microseconds

The 8x speedup sounds significant but 20 microseconds vs 2.5 microseconds is unlikely to be the bottleneck. The parse itself takes 100-1000 microseconds.

**Verdict: Not recommended. Position updates are O(depth) in tree-sitter's design, and SIMD won't improve a 15-iteration loop. The architecture of edit propagation is already optimal.**

### 3f. Arena Allocation with Generational Collection

**The problem**: During incremental re-parsing, the parser creates new subtree nodes for the changed region while the old tree's nodes are still referenced. This creates a "generational" pattern: old nodes (from the previous parse) are long-lived, while new nodes (from re-parsing) are either short-lived (error recovery attempts that get discarded) or medium-lived (become part of the new tree).

**C's approach**: The `SubtreePool` free-list (max 32 entries) reuses recently freed nodes. Nodes beyond the pool capacity are freed via `ts_free`. The pool exploits temporal locality: nodes freed during reduce operations are quickly reused for new reduces.

**Go's GC challenge**: The Go GC doesn't distinguish between old and new subtree nodes. A full parse creates ~10K nodes; incremental re-parse creates ~100 new nodes and invalidates ~100 old ones. But the GC sees all ~10K live nodes on every cycle, spending time scanning nodes that haven't changed.

**Arena allocation strategy**:

```go
// Each parse operation gets a fresh arena for new nodes
type subtreeArena struct {
    blocks [][]Subtree    // blocks of pre-allocated subtrees
    current int           // index into current block
    blockSize int         // size of each block
}

func newSubtreeArena(estimatedNodes int) *subtreeArena {
    blockSize := max(256, estimatedNodes)
    return &subtreeArena{
        blocks: [][]Subtree{make([]Subtree, blockSize)},
        blockSize: blockSize,
    }
}

func (a *subtreeArena) alloc() *Subtree {
    if a.current >= len(a.blocks[len(a.blocks)-1]) {
        a.blocks = append(a.blocks, make([]Subtree, a.blockSize))
        a.current = 0
    }
    block := a.blocks[len(a.blocks)-1]
    node := &block[a.current]
    a.current++
    return node
}
```

**Benefits**:
- **Fewer allocations**: One `make([]Subtree, N)` vs N individual `new(Subtree)` calls
- **Better cache locality**: Nodes created during one parse are contiguous in memory
- **Reduced GC pressure**: GC sees one slice header per block instead of N individual pointers
- **Bulk deallocation**: When an arena is no longer needed, setting the slice to nil frees all nodes at once (in the next GC cycle)

**Interaction with Go's GC**: Arena-allocated nodes still contain pointers (to children, to other subtrees). The GC must trace these pointers. However, arena allocation reduces the number of root pointers the GC needs to scan from O(N) to O(blocks), and improves cache performance during GC marking.

**Generational strategy for incremental parsing**:

```
Parse 1: Create arena A1 with ~10K nodes → Tree T1 (references A1)
Edit:    Tree T1 → Tree T1' (shared structure with T1)
Parse 2: Create arena A2 with ~100 new nodes → Tree T2 (references A1 and A2)

At this point:
  - A1 holds nodes shared between T1 and T2
  - A2 holds only new nodes from parse 2
  - If T1 is discarded, GC will eventually collect A1 nodes not referenced by T2
```

**Expected GC reduction**: For incremental re-parses, the parser allocates ~100-500 new nodes. With arena allocation, this is 1-2 slice allocations instead of 100-500 individual allocations. GC scanning is reduced proportionally.

**Go 1.22+ arena package**: Go added an experimental `arena` package. However, it was removed in Go 1.23 due to complexity concerns. The manual arena approach above is more portable.

**Verdict: High value, low complexity. Implement arena allocation in Phase 1. The reduced allocation count directly improves incremental re-parse latency.**

### 3g. Incremental Chart Parsing / Memoized Parse Forests

**Wagner & Graham's algorithm** (1998): The foundational algorithm for LR-based incremental parsing, which tree-sitter is based on. The key insight is memoizing parse decisions at token boundaries. The memo table maps `(position, parse_state) -> (action_taken, resulting_state)`.

On edit, entries in the changed byte range are invalidated. Entries outside the range are reused — the parser doesn't need to re-derive parse actions for unchanged tokens.

**Tree-sitter's implementation is a specific instance of this**: Rather than an explicit memo table, tree-sitter uses the old parse tree as an implicit memo structure. A reusable subtree at position P with parse state S encodes "starting from state S at position P, the parser will produce this subtree." Reusing the subtree skips all the parse actions that produced it.

**Where an explicit memo table could help**: Tree-sitter's reuse is all-or-nothing — either an entire subtree is reused, or it's re-parsed from scratch. An explicit memo table could provide finer granularity:

- **Token-level memoization**: Even if a parent node isn't reusable (e.g., because one child has changes), the parse actions for unchanged children could be memoized individually.
- **State-level memoization**: If the parser reaches the same state at the same position during re-parse, it can skip to the previously computed next state.

**Practical assessment**: Tree-sitter's subtree reuse already provides most of this benefit. When a parent isn't reusable, the parser descends into its children and tries to reuse those. The only scenario where explicit memoization helps is when the parser reaches the same position and state via a different path (e.g., after error recovery). This is rare in incremental parsing.

**Packrat-style memoization for GLR**: In a GLR parser with multiple active versions, different versions may reach the same position with the same state. Tree-sitter already handles this via stack merging (`ts_stack_merge`). An explicit memo table would be redundant.

**Verdict: Not recommended. Tree-sitter's subtree reuse is already an implicit memoization mechanism. An explicit memo table adds memory overhead without significant benefit for the common case.**

*References: Wagner & Graham "Incremental Analysis of Real Programming Languages" 1998; Tim Wagner PhD thesis, UC Berkeley 1997.*

### 3h. Speculative Subtree Reuse with LSH

**The idea**: Use locality-sensitive hashing (LSH) to quickly detect when a re-parsed subtree is structurally identical to an old subtree at a different position. After re-parsing, compare the new subtree's LSH fingerprint with old subtrees' fingerprints to find matches. If a match is found, reuse the old subtree's identity (pointer) — this is useful for consumers that track node identity (e.g., incremental highlighting that tracks which nodes changed).

**How often does this happen?** Consider:
- Insert a blank line at the top of a file: All subtrees shift down. They're structurally identical but at different positions. Tree-sitter re-parses all tokens after the insertion because the byte offsets don't match.
- Rename a variable: The identifier token changes, but all surrounding subtrees are identical.

In the first case, structural matching would avoid re-parsing entirely. In the second case, the changed token forces re-parsing regardless.

**Estimated frequency**: Based on typical editing patterns:
- **Line insertion/deletion**: ~10% of edits. Benefits from structural matching.
- **Character edits**: ~80% of edits. Positional reuse already works.
- **Multi-line edits**: ~10% of edits. Mixed benefit.

**LSH implementation**: MinHash over the sequence of (symbol, child_count) pairs in a subtree's prefix:

```go
func subtreeFingerprint(t *Subtree) uint64 {
    h := uint64(0)
    h = mix(h, uint64(t.symbol))
    h = mix(h, uint64(len(t.children)))
    for i, child := range t.children {
        if i >= 4 { break } // only first 4 children
        h = mix(h, uint64(child.symbol))
    }
    return h
}
```

**Cost**: Computing fingerprints for all nodes: O(N). Building a hash table: O(N). Querying: O(1) per subtree. Total: O(N) per parse, using O(N) additional memory.

**This is equivalent to Section 3b (hash-consed trees)** but with probabilistic matching instead of exact matching. Since exact structural hashing (Section 3b) is equally simple and provides guaranteed correctness, LSH doesn't offer an advantage here.

**Verdict: Superseded by hash-consed trees (Section 3b). Use exact structural hashing instead of LSH — it's equally fast and doesn't have false positive/negative issues.**

---

## 4. Evaluation and Phased Plan

### Technique Summary

| Technique | Expected Speedup | Complexity | Helps With | Phase |
|-----------|-----------------|------------|------------|-------|
| Immutable trees (always clone edit path) | 1.0-1.1x | Low | Correctness, simplicity | 0 |
| Arena allocation for subtrees | 1.2-2x (GC reduction) | Low | All incremental parses | 1 |
| Structural hash for "no change" detection | 10-100x (when applicable) | Low-Med | No-structural-change edits | 1 |
| Concrete types on hot paths (no interfaces) | 1.1-1.2x | Low | All traversal/reuse | 0 |
| Hash-consed subtree reuse | 2-10x (shifted trees) | Medium | Line insert/delete edits | 2 |
| Parallel multi-edit re-parsing | 2-8x (multi-edit) | High | Find-replace, multi-cursor | 2 |
| Interval tree for edit lookup | 1.0-1.5x (batch edits) | Medium | Batch edit operations | 3 |
| Self-adjusting computation | Marginal | Very High | N/A for LR parsing | Skip |
| SIMD position updates | Marginal | Medium | N/A (O(depth) updates) | Skip |
| Explicit memoization table | Marginal | High | N/A (implicit in tree) | Skip |
| LSH subtree matching | Superseded | Medium | Use hash-consing instead | Skip |

### Phase 0: Match C Tree-sitter Behavior (Faithful Port)

*Target: Identical behavior, correct incremental parsing.*

- **Immutable tree approach**: `Tree.Edit()` creates a new tree sharing unchanged subtrees. Clone only nodes on the edit path.
- **ReusableNode iterator**: Direct port from C. Stack-based DFS with position tracking.
- **Reusability checks**: Same conditions as C — `has_changes`, error, missing, fragile, external scanner state, lex mode compatibility, `is_reusable` table flag.
- **ChangedRanges**: Direct port of the parallel tree walk algorithm.
- **Concrete types everywhere**: No interfaces on the incremental parsing hot path.

**Verification**: For any input file and any sequence of edits, the Go parser must produce identical parse trees to C tree-sitter. Use differential testing with the C parser as oracle.

### Phase 1: Exploit Go Advantages

*Target: Faster incremental re-parse than C for common edit patterns.*

**1a. Arena allocation**: Allocate subtrees from per-parse arenas instead of individual heap allocations. Expected 1.2-2x improvement in incremental re-parse latency from reduced GC pressure.

**1b. Structural hash for fast "no change" detection**: Compute bottom-up structural hashes during tree construction. After re-parsing, compare root hashes — if equal, return immediately without walking the tree. This handles the common case where an edit changes whitespace or comments without affecting tree structure.

```go
// During tree construction
func (s *Subtree) computeHash() {
    if s.childCount == 0 {
        s.structuralHash = hashLeaf(s.symbol, s.size)
    } else {
        h := uint64(s.symbol)
        for _, child := range s.children {
            h = mixHash(h, child.structuralHash)
        }
        s.structuralHash = h
    }
}

// Fast "no change" check
func (t *Tree) ChangedRanges(other *Tree) []Range {
    if t.root.structuralHash == other.root.structuralHash {
        return nil  // O(1) — no structural change
    }
    // Fall through to full tree walk...
}
```

**1c. Optimized reusable node descent**: When descending into a non-reusable subtree, use binary search on child positions instead of linear scan. Children are stored in sorted order by position, so we can find the first child after the current position in O(log children) instead of O(children).

### Phase 2: Parallel Multi-Edit and Structural Reuse

*Target: Significant speedup for multi-edit workflows.*

**2a. Hash-consed subtree reuse**: Extend structural hashing to enable reuse of structurally identical subtrees at different positions. After an edit that shifts code (e.g., inserting a line), the parser can detect that subtrees below the insertion are structurally identical to old subtrees and reuse them, avoiding re-parsing.

**2b. Parallel multi-edit**: When multiple edits arrive simultaneously:
1. Partition edits into independent regions (separated by reusable subtrees)
2. Spawn goroutines to re-parse each region
3. Merge results

This requires a thread-safe (or per-goroutine) parser state, including separate lexers and parse stacks per region.

**2c. Lazy position update**: Instead of eagerly updating all positions during edit, store position updates as lazy deltas. Resolve positions on demand when they're accessed. This makes edit application O(1) instead of O(depth), deferring the O(depth) cost to the first access.

### Phase 3: Advanced Indexing and Aggressive Reuse

*Target: Sub-microsecond incremental re-parse for simple edits.*

**3a. Token-level reuse cache**: Cache the mapping `(byte_position, parse_state) -> token` across parses. For edits that don't change the token stream (e.g., inserting a space within whitespace), the cache provides instant token reuse without calling the lexer.

**3b. Interval tree for batch edits**: Build an interval tree over node spans. For batch operations (find-replace with 50 matches), query the interval tree to identify all affected subtrees in O(50 * log N) instead of O(50 * depth).

**3c. Predictive reuse**: Use heuristics to predict which subtrees will be reusable based on edit location. For example, if the edit is within a function body, predict that all subtrees outside that function are reusable and skip the reusability check for them. This requires understanding grammar structure at a level beyond what the generic runtime currently has.

---

## 5. Benchmark Design

### Edit-Reparse Latency

The primary metric: time from calling `Parser.Parse(oldTree, input)` to receiving the new tree.

**Microbenchmark protocol**:
```go
func BenchmarkIncrementalReparse(b *testing.B) {
    parser := NewParser()
    parser.SetLanguage(goLang)
    source := loadFile("kubernetes/pkg/api/types.go") // ~5000 lines

    // Initial parse
    tree := parser.ParseString(nil, source)

    // Apply a single-character edit in the middle
    editPos := len(source) / 2
    editedSource := source[:editPos] + "x" + source[editPos:]
    tree.Edit(&InputEdit{
        StartByte: uint32(editPos),
        OldEndByte: uint32(editPos),
        NewEndByte: uint32(editPos + 1),
        // ... points
    })

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        newTree := parser.ParseString(tree, editedSource)
        tree.Edit(&InputEdit{ /* undo */ })
        tree = newTree
    }
}
```

**Edit patterns to benchmark**:
- Single character insertion (middle of file)
- Single character deletion
- Single line insertion (top, middle, bottom)
- Single line deletion
- Multi-line edit (replace 5 lines)
- Multi-cursor edit (10 positions)
- Find-replace (50 occurrences)
- Formatting change (add indentation to 100 lines)

### Subtree Reuse Rate

Percentage of old tree nodes that are reused in the new tree.

```go
func measureReuseRate(oldTree, newTree *Tree) float64 {
    totalOldNodes := countNodes(oldTree)
    reusedNodes := countSharedNodes(oldTree, newTree) // pointer comparison
    return float64(reusedNodes) / float64(totalOldNodes)
}
```

Target: ≥99% reuse rate for single-character edits in large files.

### Memory Churn Per Edit

Bytes allocated during incremental re-parse.

```go
func BenchmarkIncrementalAllocations(b *testing.B) {
    // ... setup ...
    b.ReportAllocs()
    for i := 0; i < b.N; i++ {
        newTree := parser.ParseString(tree, editedSource)
        // ...
    }
}
```

Target: <10KB allocated per single-character incremental reparse (compared to ~500KB for full parse of a 10KB file).

### Multi-Edit Throughput

Edits processed per second for batch operations.

```go
func BenchmarkMultiEditThroughput(b *testing.B) {
    // Apply 50 edits (simulating find-replace)
    edits := generateFindReplaceEdits(source, "oldName", "newName")
    // ...
}
```

### Comparison Methodology vs C Tree-sitter

1. **Same file, same edit, same machine**: Run both implementations on identical inputs
2. **Measure independently**: Use Go's `testing.B` and C's `clock_gettime(CLOCK_MONOTONIC)`
3. **Report ratio**: `goTime / cTime` with confidence intervals
4. **Vary parameters**: File sizes (1KB, 10KB, 100KB, 1MB), edit sizes (1 char, 1 line, 10 lines), edit positions (beginning, middle, end)

**C comparison harness**:
```c
void bench_incremental(const char *source, size_t len) {
    TSParser *parser = ts_parser_new();
    ts_parser_set_language(parser, tree_sitter_go());

    TSTree *tree = ts_parser_parse_string(parser, NULL, source, len);

    // Apply edit
    TSInputEdit edit = { .start_byte = len/2, .old_end_byte = len/2, .new_end_byte = len/2 + 1, ... };
    ts_tree_edit(tree, &edit);

    // Measure re-parse time
    struct timespec start, end;
    clock_gettime(CLOCK_MONOTONIC, &start);
    for (int i = 0; i < iterations; i++) {
        TSTree *new_tree = ts_parser_parse_string(parser, tree, edited_source, len + 1);
        ts_tree_delete(new_tree);
    }
    clock_gettime(CLOCK_MONOTONIC, &end);
}
```

### Expected Performance Targets

| Phase | Edit-Reparse (10KB file, 1 char edit) | Go/C Ratio |
|-------|---------------------------------------|------------|
| Phase 0 (faithful port) | ~10-50 microseconds | 1.5-3x slower |
| Phase 1 (arena + hash) | ~5-20 microseconds | 0.8-1.5x |
| Phase 2 (parallel + structural) | ~3-10 microseconds | 0.5-1.0x |
| Phase 3 (aggressive reuse) | ~1-5 microseconds | 0.3-0.8x |

C tree-sitter baseline for reference: ~5-10 microseconds for a single-character reparse of a 10KB Go source file.

---

## 6. Key References

### Papers
- Wagner, Tim & Graham, Susan. "Incremental Analysis of Real Programming Languages." PLDI 1998.
- Acar, Umut et al. "Adaptive Functional Programming." POPL 2002.
- Hammer, Matthew et al. "Adapton: Composable, Demand-Driven Incremental Computation." PLDI 2014.
- Appel, Andrew & Goncalves, Manuel. "Hash-consing Garbage Collection." 1993.
- Filliâtre, Jean-Christophe & Conchon, Sylvain. "Type-Safe Modular Hash-Consing." ML Workshop 2006.

### Tree-sitter Source Code
- Incremental parsing entry: `parser.c:ts_parser_parse` (lines 2074-2199)
- Subtree reuse: `parser.c:ts_parser__reuse_node` (lines 753-830)
- First leaf check: `parser.c:ts_parser__can_reuse_first_leaf` (lines 470-502)
- Edit propagation: `subtree.c:ts_subtree_edit` (lines 633-786)
- Changed ranges: `get_changed_ranges.c:ts_subtree_get_changed_ranges` (lines 413-557)
- Reusable node iterator: `reusable_node.h` (full file)
- Copy-on-write: `subtree.c:ts_subtree_make_mut` (lines 284-290)
- Subtree pool: `subtree.c:ts_subtree_pool_allocate/free` (lines 137-151)
- Reference counting: `subtree.c:ts_subtree_retain/release` (lines 558-594)
- Tree balancing: `subtree.c:ts_subtree_compress` (lines 292-336)
- Tree balancing (parser): `parser.c:ts_parser__balance_subtree` (lines 1867-1922)
