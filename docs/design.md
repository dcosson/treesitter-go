# Design Document: treesitter-go — A Pure-Go Tree-sitter Implementation

## 1. Scope and Goals

### What We're Building

A pure-Go implementation of tree-sitter's parser runtime that can parse source code
using tree-sitter grammars with zero CGo dependency. The project targets compatibility
with the existing tree-sitter grammar ecosystem (~300+ grammars).

### In Scope

- **Parser runtime**: The GLR parsing engine (equivalent to C `lib/src/parser.c`,
  `lexer.c`, `stack.c`, `subtree.c`, `tree.c`)
- **Tree API**: Node traversal, tree cursors, S-expression output (equivalent to
  `node.c`, `tree_cursor.c`)
- **Query system**: S-expression pattern matching on parse trees (equivalent to
  `query.c`)
- **Incremental parsing**: Re-parsing after edits by reusing unchanged subtrees
- **Grammar loading**: A mechanism to load compiled tree-sitter grammars into Go
- **Grammar code generation**: A tool to compile tree-sitter grammars into Go code

### Out of Scope (for now)

- **Grammar authoring DSL**: We will not port the grammar.js JavaScript DSL. Grammars
  will continue to be authored using the existing tree-sitter CLI and then compiled
  to Go.
- **WASM support**: The C runtime's WASM language loading (`wasm_store.c`) is not
  needed.
- **Syntax highlighting / tags**: Higher-level features built on top of queries.
  These can be added later as pure Go once the query system works.

### Target Languages

The top 15 languages to support with compiled grammar packages, in priority order:

1. **JSON** — simplest grammar, no external scanner, ideal for bootstrapping
2. **Go** — primary use case
3. **JavaScript** — large user base, exercises external scanners (template literals)
4. **TypeScript** — extends JavaScript, stress-tests large grammar tables (~4000 lex states)
5. **Python** — indentation-sensitive (complex external scanner)
6. **Bash** — shell use case, complex external scanner (~1050 lines: heredocs, variable expansion, globs)
7. **Rust** — popular systems language
8. **C** — foundational, relatively simple grammar
9. **C++** — complex external scanner (~2000 lines), stress-tests the port
10. **Ruby** — popular scripting language
11. **Java** — enterprise lingua franca
12. **HTML** — exercises language injection (embedded JS/CSS)
13. **CSS** — commonly injected within HTML
14. **Zsh** — shell use case (separate grammar from Bash; external scanner extends Bash's)
15. **Perl** — scripting language

JSON is the MVP grammar (Phase 1). Go and JavaScript are the primary validation
targets (Phase 4-5). Bash and Zsh are key for shell tooling. The remaining
grammars validate robustness and are prioritized by user demand.

### Target API

The public API mirrors tree-sitter's C API (`api.h`) translated to idiomatic Go:

```go
// Core types
type Parser struct { ... }
type Tree struct { ... }
type Node struct { ... }       // small value type, like C's TSNode
type TreeCursor struct { ... }
type Language struct { ... }   // holds compiled parse tables
type Query struct { ... }
type QueryCursor struct { ... }

// Positional types
type Point struct { Row, Column uint32 }
type Range struct { StartPoint, EndPoint Point; StartByte, EndByte uint32 }
type InputEdit struct { StartByte, OldEndByte, NewEndByte uint32; StartPoint, OldEndPoint, NewEndPoint Point }

// Parser
func NewParser() *Parser
func (p *Parser) SetLanguage(lang *Language) error
func (p *Parser) Parse(ctx context.Context, oldTree *Tree, input Input) *Tree
func (p *Parser) ParseString(ctx context.Context, oldTree *Tree, source []byte) *Tree
func (p *Parser) SetIncludedRanges(ranges []Range)
func (p *Parser) Reset()

// Tree
func (t *Tree) RootNode() Node
func (t *Tree) Edit(edit *InputEdit)
func (t *Tree) ChangedRanges(other *Tree) []Range
func (t *Tree) Language() *Language

// Node (value type — no pointer receiver)
func (n Node) Type() string
func (n Node) Symbol() Symbol
func (n Node) StartByte() uint32
func (n Node) EndByte() uint32
func (n Node) StartPoint() Point
func (n Node) EndPoint() Point
func (n Node) ChildCount() uint32
func (n Node) NamedChildCount() uint32
func (n Node) Child(index int) Node
func (n Node) NamedChild(index int) Node
func (n Node) ChildByFieldName(name string) Node
func (n Node) Parent() Node
func (n Node) NextSibling() Node
func (n Node) PrevSibling() Node
func (n Node) String() string  // S-expression

// Input interface
type Input interface {
    Read(byteOffset uint32, position Point) []byte
}

// Query
func NewQuery(language *Language, pattern string) (*Query, error)
func NewQueryCursor() *QueryCursor
func (qc *QueryCursor) Exec(query *Query, node Node)
func (qc *QueryCursor) NextMatch() (*QueryMatch, bool)
func (qc *QueryCursor) NextCapture() (*QueryMatch, uint32, bool)
```

### Compatibility Goals

- **Grammar compatibility**: Any grammar compiled with tree-sitter generate should
  work after compilation to Go. The C runtime supports ABI versions 13-15 (version
  14 added primary state deduplication; version 15 added reserved words and
  supertypes). We target **ABI version 15 only** — grammars compiled with older
  tree-sitter versions can be recompiled.
- **Parse tree compatibility**: Given the same input and grammar, the Go parser
  must produce the same concrete syntax tree (same node types, same structure,
  same byte ranges) as the C parser.
- **Query compatibility**: The same query patterns should match the same nodes.

---

## 2. Architecture

### C Architecture Overview

The C tree-sitter runtime (`lib/src/`, 10 `.c` files, ~12.7K lines of core source;
~15.9K including all headers) implements a GLR (Generalized LR) parser based on
Wagner & Graham's incremental parsing algorithm. The key architectural insight is
that the parser is split into a **grammar-independent runtime** and
**grammar-specific compiled tables + lexer functions**.

The C source files and their responsibilities:

| File | Lines | Role |
|------|-------|------|
| `parser.c` | 2262 | Core GLR parsing engine, main loop, error recovery |
| `query.c` | 4496 | S-expression query compiler and cursor |
| `subtree.c` | 1034 | Tree node creation, ref counting, edit, serialization |
| `stack.c` | 912 | Graph-Structured Stack for GLR parsing |
| `node.c` | ~800 | Public Node API, child iteration |
| `tree_cursor.c` | ~720 | Stack-based tree traversal cursor |
| `get_changed_ranges.c` | 557 | Parallel tree walk for incremental change detection |
| `lexer.c` | ~480 | Chunked input reader, Unicode decoding, token scanning |
| `language.c` | 289 | Parse table lookup, symbol/field accessors |
| `tree.c` | ~130 | Tree wrapper (root subtree + language + ranges) |

### Go Package Structure

```
treesitter-go/
├── go.mod
├── Makefile
├── docs/
│   └── design.md
├── parser.go          // Parser struct, main parse loop
├── lexer.go           // Lexer struct, token scanning
├── stack.go           // Graph-Structured Stack (GLR)
├── subtree.go         // Subtree types, creation, edit
├── tree.go            // Tree, Node value type
├── tree_cursor.go     // TreeCursor traversal
├── query.go           // Query compiler and QueryCursor
├── language.go        // Language struct, parse table lookup
├── types.go           // Point, Range, InputEdit, Symbol, etc.
├── changed_ranges.go  // Incremental change detection
├── *_test.go          // Tests alongside each file
└── cmd/
    └── tsgo-generate/ // Grammar-to-Go code generator
        └── main.go
```

All runtime code lives in a single `treesitter` package (or `ts`). This avoids
the overhead of inter-package interfaces for what are tightly-coupled internal
data structures. The code generator is a separate binary.

### Core Types and Their Relationships

```
Language (compiled grammar tables)
    │
    ├── parse_table []uint16        (dense, for large states)
    ├── small_parse_table []uint16  (compressed, for small states)
    ├── parse_actions []ParseActionEntry
    ├── lex_modes []LexMode
    ├── lex_fn func(*Lexer, StateID) bool
    ├── keyword_lex_fn func(*Lexer, StateID) bool
    ├── external_scanner ExternalScanner (interface)
    ├── symbol_metadata []SymbolMetadata
    ├── field_map_slices []MapSlice
    ├── field_map_entries []FieldMapEntry
    ├── alias_sequences []Symbol
    └── ... (names, supertype maps, reserved words)

Parser
    ├── language *Language
    ├── lexer Lexer
    ├── stack Stack              (Graph-Structured Stack)
    ├── reusableNode ReusableNode (old tree iterator)
    ├── oldTree *Subtree
    ├── finishedTree *Subtree
    └── tokenCache TokenCache

Stack (GSS - Graph-Structured Stack)
    ├── heads []StackHead        (active parse versions)
    └── node graph: StackNode -> []StackLink -> StackNode
        Each StackLink carries a Subtree

Subtree (parse tree node — index into SubtreeArena, or inline 8-byte value)
    ├── symbol Symbol
    ├── padding, size Length      (byte offset + row/col)
    ├── children []Subtree       (value type indices; nil for leaves)
    ├── parseState StateID
    ├── childCount uint32
    ├── visibleChildCount uint32
    ├── namedChildCount uint32
    ├── visibleDescendantCount uint32  (for efficient NamedChild(index))
    ├── firstLeaf {Symbol, ParseState}  (incremental reuse lex mode check)
    ├── repeatDepth uint16       (tree balancing for left-recursive repetitions)
    ├── productionID uint16      (field map lookups, alias sequences)
    ├── flags (visible, named, extra, hasChanges, fragileLeft, fragileRight, ...)
    ├── errorCost uint32
    ├── dynamicPrecedence int32
    └── externalScannerState []byte  (for external token leaves)

Tree (public handle)
    ├── root *Subtree
    ├── language *Language
    └── includedRanges []Range

Node (lightweight value type, 32 bytes)
    ├── context [4]uint32  // [startByte, startRow, startCol, aliasSymbol]
    ├── id unsafe.Pointer  // -> Subtree (identity)
    └── tree *Tree

Query (compiled pattern)
    ├── steps []QueryStep
    ├── patterns []QueryPattern
    ├── captureNames []string
    └── predicateSteps []PredicateStep

QueryCursor (stateful matcher)
    ├── query *Query
    ├── cursor TreeCursor
    ├── states []QueryState
    └── captureListPool CaptureListPool
```

### Memory Management: Go GC vs C Ref Counting

The C runtime uses manual reference counting on `SubtreeHeapData` with atomic
increment/decrement, a free-list pool (`SubtreePool`, max 32 entries), and a
clever single-allocation trick where children are stored immediately before the
parent node's metadata in memory. It also uses an inline subtree optimization
where leaf tokens that fit in 8 bytes are stored directly as values (no heap
allocation) — this covers ~60-80% of leaf tokens.

**Go approach**: Eliminate all ref counting. Go's garbage collector handles
lifetime. The `ts_subtree_retain`/`ts_subtree_release` machinery is removed
entirely. We address the resulting GC pressure through three complementary
strategies:

**1. Arena allocation (primary strategy)**. Allocate subtree nodes from a
per-parse `SubtreeArena` — a `[]SubtreeHeapData` backing array that the parser
slices from. This reduces thousands of individual heap allocations to a handful
of slice allocations, improving GC performance (GC sees one slice header per
block instead of thousands of individual pointers) and cache locality (nodes
created during one parse are contiguous in memory):

```go
type SubtreeArena struct {
    blocks    [][]SubtreeHeapData
    current   int
    blockSize int
}

func (a *SubtreeArena) alloc() *SubtreeHeapData { /* slice from current block */ }
```

When a tree is discarded, setting the arena slices to nil frees all nodes at
once in the next GC cycle.

**2. Inline subtree optimization (designed in from the start)**. The `Subtree`
type is a small value type that discriminates between inline leaf data and a
reference to heap-allocated `SubtreeHeapData`:

```go
type Subtree struct {
    // If inline is true, leaf data is packed into these fields directly.
    // If inline is false, index refers to a SubtreeHeapData in the arena.
    inline     bool
    // Inline fields (used when inline == true)
    symbol     uint8
    parseState uint16
    // ... packed size/padding fields
    // Arena reference (used when inline == false)
    data       *SubtreeHeapData
}
```

This preserves the C optimization's value: no pointer chase or heap allocation
for the majority of leaf tokens. Children are stored as `[]Subtree` (values,
not pointers), keeping children contiguous in memory with their parent's data
and avoiding an extra level of indirection.

**3. Immutable trees for copy-on-write**. Without ref counting, we cannot check
"am I the only owner?" (`refcount == 1`) like C does in `ts_subtree_make_mut`.
Instead, we treat all trees as immutable. `Tree.Edit()` creates a new tree
sharing unchanged subtrees by cloning only nodes on the edit path (the spine
from root to edited leaf). Since the edit path is O(depth) = O(log N), this
is cheap — for a 10K-line file with ~15 tree depth, we clone ~15 nodes. Go's
GC automatically keeps shared subtrees alive as long as either tree references
them. This eliminates an entire class of bugs (mutation of shared data) and
aligns with Go idioms.

See `docs/addendum-hot-paths.md` for detailed analysis of memory allocation
hot paths and optimization tiers.

---

## 3. Parse Table Format

### How C Grammars Encode Tables

The tree-sitter grammar compiler (Rust, `crates/generate/`) transforms a
`grammar.js` file through this pipeline:

1. **JS execution** → JSON AST
2. **Grammar preparation** → separate `SyntaxGrammar` + `LexicalGrammar`
3. **Table building** → LALR(1) parse table + DFA lex tables
4. **Code generation** → `parser.c` with embedded static arrays

The generated `parser.c` contains:

- **Parse tables**: Dense 2D `uint16` array for "large" states (many valid symbols),
  compressed grouped format for "small" states (few valid symbols)
- **Parse actions**: Flat `TSParseActionEntry` array (union of header + action)
- **Lex function**: A C function implementing a DFA state machine using
  goto-based macros (`ADVANCE`, `SKIP`, `ACCEPT_TOKEN`)
- **Lex modes**: Per-parse-state mapping to lex state + external lex state
- **Symbol metadata**: Names, visibility, field maps, alias sequences
- **External scanner declarations**: 5 function pointer slots (`create`, `destroy`,
  `scan`, `serialize`, `deserialize`) plus a state validity table

### Options for Getting Grammars into Go

**Option A: Compile grammars to Go source code (Recommended)**

Write a `tsgo-generate` tool that takes a tree-sitter grammar (either the
`grammar.js` or the intermediate JSON) and produces a Go file with:

```go
package lang_go

import ts "github.com/example/treesitter-go"

func Language() *ts.Language {
    return &ts.Language{
        SymbolCount:    250,
        TokenCount:     120,
        StateCount:     1500,
        LargeStateCount: 45,
        // Parse tables as Go slice literals
        ParseTable:     []uint16{ ... },
        SmallParseTable: []uint16{ ... },
        SmallParseTableMap: []uint32{ ... },
        ParseActions:   []ts.ParseActionEntry{ ... },
        LexModes:       []ts.LexMode{ ... },
        SymbolNames:    []string{ ... },
        SymbolMetadata: []ts.SymbolMetadata{ ... },
        // Lex function as Go code
        LexFn:          lexMain,
        KeywordLexFn:   lexKeywords,
        // ...
    }
}

func lexMain(lexer *ts.Lexer, state ts.StateID) bool {
    for {
        switch state {
        case 0:
            if lexer.EOF() { lexer.AcceptToken(ts.SymEnd); return true }
            switch {
            case lexer.Lookahead() == '(':
                lexer.Advance(false)
                state = 5
            case lexer.Lookahead() >= 'a' && lexer.Lookahead() <= 'z':
                lexer.Advance(false)
                state = 20
            default:
                return false
            }
        case 5:
            lexer.AcceptToken(symLParen)
            return true
        // ...
        }
    }
}
```

**Pros**:
- Zero runtime parsing overhead — tables are compiled Go data
- Lex function is native Go code with full compiler optimization
- Grammar packages are regular Go modules, installed via `go get`
- Type safety — invalid tables are caught at compile time

**Cons**:
- Requires building a Go code generator (parallel to the Rust `render.rs`)
- Generated files can be large (some grammars produce 500KB+ of C tables)
- Updating a grammar requires regeneration

**Option B: Binary format with go:embed**

Define a binary serialization format for the Language struct. Grammars are
compiled to `.tsb` files and loaded at runtime via `go:embed` or file I/O.

**Pros**:
- Smaller generated files (binary is denser than source code)
- Could reuse the existing C-compiled parser.c by extracting tables from the
  compiled object

**Cons**:
- The lex function cannot be serialized as data in a portable way — it would need
  to be a table-driven DFA interpreter, which is significantly slower than compiled
  code
- Runtime deserialization cost
- More complex versioning / ABI management

**Option C: Interpret C parse tables directly**

Parse the generated `parser.c` file to extract table data, and use a table-driven
DFA interpreter for the lexer.

**Pros**:
- Works with any existing grammar without a new compiler
- Minimal tooling requirement

**Cons**:
- Parsing C source to extract data is fragile
- Table-driven DFA interpretation is 3-5x slower than compiled state machines
- No type safety

### Recommended Approach

**Option A (compile to Go source)** is the clear winner. The lex function is
performance-critical (called for every token) and must be compiled code, not
interpreted. The table data is straightforward to emit as Go literals.

The code generator can be built incrementally:
1. Start by hand-translating one grammar (e.g., JSON) to validate the runtime
2. Build the generator to automate this for any grammar
3. The generator reads the same JSON intermediate format that the Rust compiler
   produces, so it can reuse the existing tree-sitter CLI for grammar preparation

### Parse Table Representation in Go

The dual large/small state encoding from C maps directly:

```go
type Language struct {
    // Metadata
    symbolCount    uint32
    tokenCount     uint32
    stateCount     uint32
    largeStateCount uint32

    // Parse tables — same encoding as C
    parseTable       []uint16  // Dense 2D: [state * symbolCount + symbol] -> action index
    smallParseTable  []uint16  // Compressed: [groupCount, (value, symCount, sym...)+]
    smallParseTableMap []uint32 // Small state index -> offset into smallParseTable

    // Actions — flat array, header + N actions per entry
    parseActions []ParseActionEntry
    // ...
}

// Table lookup — mirrors ts_language_lookup() from language.h
func (l *Language) lookup(state StateID, symbol Symbol) uint16 {
    if uint32(state) < l.largeStateCount {
        return l.parseTable[uint32(state)*l.symbolCount+uint32(symbol)]
    }
    idx := l.smallParseTableMap[uint32(state)-l.largeStateCount]
    data := l.smallParseTable[idx:]
    groupCount := data[0]
    pos := uint32(1)
    for i := uint16(0); i < groupCount; i++ {
        value := data[pos]
        symCount := data[pos+1]
        pos += 2
        for j := uint16(0); j < symCount; j++ {
            if data[pos] == uint16(symbol) {
                return value
            }
            pos++
        }
    }
    return 0
}
```

### ParseActionEntry Encoding

C uses a union (`TSParseActionEntry`) where the same 8 bytes are either a header
(`{count, reusable}`) or a `TSParseAction` (shift/reduce/accept/recover). In Go,
we cannot use unions, so we have several options:

**Chosen approach**: Use a Go struct for clarity and simplicity:

```go
type ParseActionEntry struct {
    Type             uint8  // 0=header, 1=shift, 2=reduce, 3=accept, 4=recover
    // Header fields
    Count            uint8
    Reusable         bool
    // Shift fields
    ShiftState       StateID
    ShiftExtra       bool
    ShiftRepetition  bool
    // Reduce fields
    ReduceSymbol     Symbol
    ReduceChildCount uint8
    ReduceDynPrec    int16
    ReduceProdID     uint16
}
```

The struct approach wastes ~10 bytes per entry compared to the C union's 8 bytes,
but is simpler and the total table size is still modest (typical grammars have
2K-20K entries = 20-200KB overhead). The actual hot path is `tableEntry()`
which returns a `TableEntry` (pointer + count + reusable flag) — the per-entry
decoding happens once at lookup time, not in an inner loop. If profiling shows
this overhead matters, a `uint64` bit-packed encoding can be substituted without
changing the API.

---

## 4. External Scanners

### The Problem

About 50% of tree-sitter grammars use external scanners — custom C functions that
handle context-sensitive lexing that the regular DFA cannot express. Examples:
- **Heredocs** (Ruby, Bash): Delimiter-terminated strings where the delimiter is
  defined at the start
- **Indentation** (Python, YAML): Tracking indent levels across lines
- **Template literals** (JavaScript): Nested `${...}` expressions inside strings
- **String interpolation** (most languages): Various escape/interpolation rules

The C external scanner interface consists of 5 functions:

```c
void *create(void);                                    // Allocate scanner state
void destroy(void *payload);                           // Free scanner state
bool scan(void *payload, TSLexer *lexer, const bool *valid_symbols); // Scan for a token
unsigned serialize(void *payload, char *buffer);       // Save state (max 1024 bytes)
void deserialize(void *payload, const char *buffer, unsigned length); // Restore state
```

The scanner is stateful: it maintains internal state (e.g., a stack of indent
levels) that must be serialized/deserialized for incremental parsing to work.

### Go External Scanner Interface

Define a Go interface:

```go
// ExternalScanner handles context-sensitive lexing that the regular DFA cannot express.
// Implementations must be safe for concurrent use if the Parser is used concurrently.
type ExternalScanner interface {
    // Scan attempts to recognize a token. validSymbols[i] is true if external
    // token i is valid in the current parse state. Returns true if a token was
    // recognized (and lexer.AcceptToken was called).
    Scan(lexer *Lexer, validSymbols []bool) bool

    // Serialize writes the scanner's internal state to buf (max 1024 bytes).
    // Returns the number of bytes written.
    Serialize(buf []byte) uint32

    // Deserialize restores scanner state from data. len(data) == 0 means
    // initialize to default state.
    Deserialize(data []byte)
}

// ExternalScannerFactory creates new ExternalScanner instances.
// Each Parser gets its own scanner instance.
type ExternalScannerFactory func() ExternalScanner
```

Each compiled grammar package that uses an external scanner must provide a Go
implementation of `ExternalScanner`. The `Language` struct holds a factory:

```go
type Language struct {
    // ...
    ExternalTokenCount    uint32
    ExternalScannerStates []bool  // Flat 2D: [extLexState * extTokenCount + tokenIdx]
    ExternalSymbolMap     []Symbol
    NewExternalScanner    ExternalScannerFactory // nil if no external scanner
}
```

### Strategies for Handling Existing C Scanners

**Strategy 1: Manual port to Go (primary approach)**

For each grammar that has an external scanner, port the `scanner.c` (or
`scanner.cc`) to Go. Most scanners are 200-800 lines of relatively straightforward
C. The `Lexer` interface is simple (advance, lookahead, mark_end, accept_token).

This is labor-intensive but produces the best result — pure Go, no CGo, full
type safety.

**Strategy 2: Auto-translation tool (future optimization)**

Build a tool that mechanically translates C scanner code to Go. Most scanners use
a limited subset of C: arrays, simple structs, switch statements, and the TSLexer
API. An AST-based translator could handle 70-80% of scanners automatically, with
manual fixup for the rest.

**Strategy 3: CGo fallback (escape hatch)**

For complex scanners that resist porting (e.g., the C++ scanner at ~2000 lines),
provide a CGo adapter that wraps the C scanner behind the Go `ExternalScanner`
interface. This sacrifices the "pure Go" goal for pragmatism but keeps the rest
of the runtime pure Go.

### Recommended Approach

Start with Strategy 1 for the MVP grammars (JSON has no scanner; Go, JavaScript,
Python, and TypeScript do). Build the auto-translation tool (Strategy 2) when
enough patterns are understood. Keep Strategy 3 as a documented escape hatch.

The scanner porting effort is bounded: the interface is only 3 methods, the
`Lexer` API is only ~5 methods, and the serialization buffer is capped at 1024
bytes. The hard part is understanding each scanner's semantics, not the
mechanical translation.

### WASM Code Paths

The C parser has WASM-conditional code paths for calling external scanners
via the WASM runtime. Since WASM grammar loading is out of scope, these code
paths will be cleanly excised from the Go port — they should not appear as
dead code or stub implementations.

---

## 5. Input Model and Language Injection

### Chunked Input (TSInput Callback)

The C parser uses a callback-based input model (`TSInput.read`) that supports
reading from ropes, gap buffers, and other non-contiguous data structures
without requiring the entire source to be in a contiguous `[]byte`. The lexer
manages this by maintaining a current "chunk" — a contiguous byte slice provided
by the callback — and requesting new chunks when it crosses a chunk boundary.

The Go `Input` interface mirrors this:

```go
type Input interface {
    Read(byteOffset uint32, position Point) []byte
}
```

The `Lexer` struct is responsible for:
1. Calling `Input.Read()` when the current position crosses a chunk boundary
2. Buffering the returned chunk and tracking the chunk's start offset
3. Decoding UTF-8 characters from the current position within the chunk
4. Handling the transition between chunks seamlessly (a multi-byte UTF-8
   character may span a chunk boundary)

For the common case of parsing a single `[]byte`, the `ParseString` convenience
method wraps the slice in a trivial `Input` that returns the entire source on
the first call and nil thereafter. In this case, there is only one chunk and
no chunk-boundary overhead.

### Included Ranges and Language Injection

`Parser.SetIncludedRanges(ranges []Range)` restricts the parser to only
consider specified byte ranges of the input, skipping everything in between.
This enables **language injection** — parsing embedded languages within a host
document (e.g., JavaScript inside HTML `<script>` tags, CSS inside `<style>`
tags, SQL inside string literals).

The mechanism affects multiple components:

- **Lexer**: Must detect when the current position exits an included range and
  jump to the start of the next included range. At range boundaries, the lexer
  reports `is_at_included_range_start`, which allows the parser to handle
  transitions between host and embedded language regions.
- **Incremental parser**: When included ranges change between parses (because
  the host document was re-parsed and the embedded regions shifted), the parser
  computes `included_range_differences` and treats nodes overlapping those
  differences as non-reusable, forcing re-parse of affected regions.
- **Changed ranges**: The parallel tree walk must account for included range
  differences when comparing old and new trees.

For the MVP, included ranges with a single range covering the full input is the
default. Multi-range support (for language injection) is deferred to Phase 5
alongside external scanners, since language injection is typically used together
with external scanners.

---

## 6. Incremental Parsing

### How C Tree-sitter Does It

Incremental parsing is tree-sitter's defining feature. When a user edits source
code, the parser reuses unchanged subtrees from the previous parse, only re-parsing
the regions that changed. This makes re-parsing a 10,000-line file after a
single-character edit take microseconds instead of milliseconds.

The algorithm has three phases:

**Phase 1: Edit the old tree** (`subtree.c: ts_subtree_edit`)

When the user calls `Tree.Edit(edit)`, the edit (byte range replacement) is
propagated through the old tree using an iterative stack-based traversal:

1. Each node checks whether the edit overlaps its byte range
2. Affected nodes get `has_changes = true`
3. Padding and size are adjusted based on the edit's byte delta
4. Nodes with `depends_on_column` are invalidated if the edit is on the same line
5. Copy-on-write: nodes are cloned only when they need modification

**Phase 2: Reuse subtrees during parsing** (`parser.c: ts_parser__reuse_node`)

During parsing, the `ReusableNode` iterator walks the old tree in document order.
At each parse position, it checks if the old subtree at that position can be
reused. A subtree is reusable if ALL of these hold:

- Its `has_changes` flag is false (not in an edited region)
- It is not an error node or missing node
- It is not fragile (not produced during GLR ambiguity)
- Its external scanner state matches the current scanner state
- It does not overlap any `included_range_difference` (for language injection)
- Its first leaf token's lex mode matches the current state's lex mode
- The table entry for its first leaf in the current state is marked `reusable`

When a subtree is reusable, the parser pushes the **entire subtree** onto the
stack in one step, skipping all the text it covers. This is the key optimization:
for a 10,000-line file with a 1-line edit, ~99.99% of subtrees are reused.

**Phase 3: Detect changed ranges** (`get_changed_ranges.c`)

After parsing, `Tree.ChangedRanges(oldTree)` walks both trees simultaneously
using parallel depth-first iterators. At each position, it compares subtrees by
symbol, size, parse state, error cost, and external scanner state. Differing
regions are accumulated into a `[]Range` result with adjacent ranges merged.

### Go Implementation Strategy

The incremental parsing algorithm ports directly — the logic is well-encapsulated
and does not depend on C-specific memory tricks. Key considerations:

**ReusableNode iterator**: A struct with a `[]stackEntry` slice for the traversal
stack. Identical algorithm to C. This is the only component that accesses the old
tree during parsing.

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
```

**Subtree edit**: The C code uses `ts_subtree_make_mut` (copy-on-write via ref
counting). In Go, since there's no ref counting, we need a different approach:

1. **Option A**: Always deep-copy the old tree before editing. Simple but
   potentially expensive for large trees.
2. **Option B (recommended)**: Use structural sharing. The `Edit` method clones
   only the spine of the tree (root → edited leaf path). Sibling subtrees are
   shared by pointer. This is O(depth) = O(log N) clones, not O(N).

```go
func (t *Tree) Edit(edit *InputEdit) {
    // Clone the root and walk down, cloning only nodes on the edit path.
    // Unaffected siblings are shared by pointer (Go's GC keeps them alive).
    t.root = editSubtree(t.root, edit)
}
```

**Changed range detection**: A straightforward port. Two `Iterator` structs walk
old and new trees. The `[]Range` result uses append-and-merge semantics.

**GC implications**: In C, the old tree is explicitly retained during parsing and
released after. In Go, the old tree stays alive as long as the parser holds a
reference. Since subtrees from the old tree may be incorporated into the new tree
(shared pointers), the GC correctly keeps them alive. No manual intervention
needed.

### Can We Defer Incremental Parsing?

Yes, and we should for the MVP. Incremental parsing is a performance optimization,
not a correctness requirement. The parser can always parse from scratch.

**Phase 1 (MVP)**: Parse from scratch every time. Ignore `oldTree` parameter.
**Phase 2**: Add incremental parsing. The runtime already needs the `has_changes`
flag and position tracking on subtrees, so the incremental infrastructure is
partially in place from the start. The main additions are the `ReusableNode`
iterator and the reusability checks in `ts_parser__advance`.

---

## 7. Query System

### Overview

The query system (`query.c`, ~4800 lines in C) lets users match structural patterns
against parse trees using an S-expression syntax:

```scheme
; Match function definitions with a name and body
(function_definition
  name: (identifier) @function.name
  body: (block) @function.body)

; Match if statements
(if_statement
  condition: (_) @condition
  consequence: (_) @consequence)

; Alternation
[(true) (false)] @boolean

; Quantifiers (tree-sitter 0.22+)
(call_expression
  arguments: (arguments (expression)+ @args))
```

### Architecture

The query system has two major components:

**1. Query Compiler** (`ts_query_new`): Parses the S-expression pattern string
into a compiled `Query` struct using a recursive descent parser (`Stream` struct
for scanning). The compiler produces:

- `[]QueryStep`: Compiled pattern nodes. Each step has a symbol to match, field
  constraints, capture IDs, and control flow indices (`alternativeIndex` for
  branching). Steps form a flat bytecode-like array where the cursor executes
  forward, with alternativeIndex providing branching.
- `[]QueryPattern`: Per-pattern metadata (offset into steps, predicate info)
- `[]PredicateStep`: Predicate data (e.g., `#eq?`, `#match?`) stored separately
- `CaptureListPool`: Pooled allocation for in-progress capture lists

**2. Query Cursor** (`TSQueryCursor`): Walks the tree using a `TreeCursor` and
matches patterns against nodes. The matching algorithm:

1. **Descend phase**: At each tree node, look up which patterns could start here
   (via a sorted pattern map keyed by node symbol). Create new `QueryState` entries
   for each matching pattern start.
2. **Advance phase**: For each active `QueryState`, try to advance it by matching
   the current node against the next `QueryStep`. Handle captures, field checks,
   alternations, and quantifiers.
3. **Ascend phase**: When leaving a node, check if any states completed their
   pattern. Completed matches are yielded to the caller.
4. **Deduplication**: The `primary_state_ids` table maps equivalent parse states
   to a canonical representative, preventing duplicate matches in merged GLR states.

### Go Implementation

The query system is largely self-contained and can be ported as a distinct module.
Key types:

```go
type Query struct {
    steps           []queryStep
    patterns        []queryPattern
    captureNames    []string
    predicateSteps  []PredicateStep
    patternMap      []patternEntry  // sorted by symbol for binary search
    language        *Language
}

type queryStep struct {
    symbol           Symbol
    field            FieldID
    captureIDs       [4]uint32  // up to 4 captures per step (matches C)
    alternativeIndex uint16     // branching for alternation/quantifiers
    // Bit flags: isNamed, isImmediate, isLastChild, isPassThrough,
    //            isDeadEnd, rootPatternGuaranteed, etc.
    flags            uint16
}

type QueryCursor struct {
    query            *Query
    cursor           TreeCursor
    states           []queryState
    finishedStates   []queryState
    captureListPool  captureListPool
    startByte        uint32
    endByte          uint32
    // ...
}
```

The recursive descent pattern parser is straightforward to port — it's a simple
scanner over the S-expression string with no external dependencies.

The `CaptureListPool` in C uses a free-list over a flat array. In Go, use a
`sync.Pool` or a simple slice-based pool:

```go
type captureListPool struct {
    lists [][]QueryCapture  // all allocated lists
    free  []uint32          // indices of free lists
}
```

### Predicate Handling

Tree-sitter queries support predicates like `#eq?`, `#match?`, `#any-of?`.
The query compiler stores predicate steps but does **not** evaluate them — that's
the caller's responsibility (the runtime only provides captured text). We should
provide a helper for the standard predicates:

```go
func (q *Query) PredicatesForPattern(patternIndex uint32) [][]PredicateStep
// Caller evaluates predicates by inspecting captured node text
```

---

## 8. Performance

### Performance Targets

The primary goal is **correctness and usability**, not matching C performance.
A 2-5x slowdown vs C tree-sitter is acceptable for the initial release. Key
benchmarks:

| Operation | C (typical) | Go target |
|-----------|-------------|-----------|
| Parse 1KB JSON | ~50 μs | < 200 μs |
| Parse 10KB Go source | ~500 μs | < 2 ms |
| Incremental reparse (1 char edit, 10KB file) | ~5 μs | < 50 μs |
| Query match (simple pattern, 10KB file) | ~100 μs | < 500 μs |

### Go vs C Performance Considerations

**GC pressure**: The biggest concern. Tree-sitter creates many small objects
(one `Subtree` per token). A 10KB file might have 2,000-5,000 subtree nodes.

Mitigations (in priority order):
- **Arena allocation** (primary): Allocate subtree nodes from per-parse
  `SubtreeArena` backing arrays. Reduces thousands of individual allocations to
  a handful of slice allocations. See §2 Memory Management.
- **Inline subtrees**: Small leaf tokens stored as 8-byte values (no heap
  allocation), eliminating ~60-80% of subtree allocations entirely.
- Keep `Node` as a small value type (no allocation on tree traversal)
- `sync.Pool` for `StackNode` and other short-lived allocations where arena
  allocation doesn't apply. Note: `sync.Pool` items can be collected at any
  GC cycle, so it provides only best-effort pooling.

**Bounds checking**: Go arrays have bounds checks. Parse table lookups are in
the hot path.

Mitigations:
- Use `_ = slice[maxIndex]` bounds check elimination hints where applicable
- The compiler eliminates many bounds checks when the index is provably in range

**Function call overhead**: The C lex function uses `goto` for zero-overhead
state transitions. Go's `for/switch` loop has slightly more overhead.

Mitigations:
- Use concrete types (not interfaces) for the lexer on hot paths — `*Lexer`
  directly, not through an interface
- The Go compiler inlines small cases well
- For very hot lex states (e.g., identifier scanning), consider loop unrolling
  in the code generator
- Profile before optimizing — the lexer may not be the bottleneck

**Parse table lookup**: The C lookup is a single array dereference for large states.
Go is the same (slice index).

Mitigations: None needed — Go slice access is essentially the same as C array
access after bounds check elimination.

**Stack (GSS) node allocation**: The GLR stack creates and discards nodes
frequently during ambiguity resolution. Use a `sync.Pool` or manual free-list
(matching C's 50-node pool) for `StackNode`.

### Profiling Strategy

1. **Benchmark suite**: Parse a corpus of files in various languages (JSON, Go,
   JavaScript, Python) at various sizes (1KB, 10KB, 100KB, 1MB).
2. **CPU profiling**: `pprof` to identify hot functions.
3. **Memory profiling**: `pprof` alloc profiling to find allocation hotspots.
4. **GC tracing**: `GODEBUG=gctrace=1` to monitor GC pause times.
5. **Comparison benchmark**: Run the same files through C tree-sitter and compare
   wall-clock times.

---

## 9. Implementation Phases

Each phase produces a working, testable artifact. Earlier phases are prerequisites
for later ones.

### Phase 1: Core Types and Table Loading (2 weeks)

**Goal**: Define all data types, implement parse table lookup, and hand-compile
one grammar (JSON) to validate the representation.

**Deliverables**:
- `types.go`: `Symbol`, `StateID`, `FieldID`, `Point`, `Range`, `Length`, `InputEdit`
- `language.go`: `Language` struct with all table fields, `lookup()`, `tableEntry()`,
  `nextState()`
- `subtree.go`: `Subtree` struct (leaf creation, accessors, size/padding arithmetic)
- Hand-compiled JSON grammar as `testdata/json_language.go`
- Tests: table lookup correctness, symbol metadata access

### Phase 2: Lexer (1 week)

**Goal**: Lex tokens from source text using generated lex functions.

**Deliverables**:
- `lexer.go`: `Lexer` struct with `Advance`, `Skip`, `MarkEnd`, `AcceptToken`,
  `Lookahead`, `EOF`, position tracking, included ranges
- `Input` interface and `[]byte` convenience adapter
- Tests: lex JSON tokens, verify positions, test included ranges

### Phase 3: Non-Incremental Parser (3 weeks)

**Goal**: Parse source text from scratch (no incremental reuse) with the full
GLR algorithm.

**Deliverables**:
- `stack.go`: Graph-Structured Stack with versions, splitting, merging, push, pop
- `parser.go`: Main parse loop, advance function, shift, reduce, accept
- Error recovery (handle_error, recover, do_all_potential_reductions, condense_stack)
- `tree.go`: `Tree` and `Node` types
- `tree_cursor.go`: `TreeCursor` with parent/child/sibling navigation
- Tests: parse JSON files, verify S-expression output matches C tree-sitter output,
  test error recovery on malformed input

### Phase 4: Grammar Code Generator (2 weeks)

**Goal**: Automatically compile tree-sitter grammars to Go packages.

**Deliverables**:
- `cmd/tsgo-generate/`: Reads grammar JSON + node-types.json, produces Go source
- Generates: parse tables, lex function, symbol metadata, external scanner stub
- Validate by generating JSON grammar and comparing output to hand-compiled version
- Generate 2-3 more grammars (Go, JavaScript) to test robustness

### Phase 5: External Scanners (2 weeks)

**Goal**: Support grammars with external scanners.

**Deliverables**:
- `ExternalScanner` interface integrated into parser and lexer
- External scanner state serialization/deserialization on subtrees
- Hand-port external scanners for Go and JavaScript grammars
- Tests: parse files that exercise external scanner tokens (template literals,
  raw strings, etc.)

### Phase 6: Incremental Parsing (2 weeks)

**Goal**: Re-parse efficiently after edits.

**Deliverables**:
- `Subtree.edit()` with structural sharing (clone-on-edit-path)
- `ReusableNode` iterator
- Reusability checks in `parser.advance()`
- `changed_ranges.go`: parallel tree walk for change detection
- Tree balancing (`subtree.compress()`): Left-recursive repetitions (e.g., a file
  with 1000 top-level statements) produce degenerate left-leaning trees. The
  `compress` function performs AVL-like rotations on repetition nodes (tracked by
  `repeat_depth`) to keep depth O(log N). This is critical for incremental parsing
  performance (the reusable node iterator traverses to depth) and for
  `Node.Child(index)` performance on long lists.
- Tests: edit-and-reparse cycles, verify results match from-scratch parse,
  benchmark incremental vs full parse

### Phase 7: Query System (2 weeks)

**Goal**: Full S-expression pattern matching.

**Deliverables**:
- `query.go`: `Query` compiler (S-expression recursive descent parser), `QueryCursor`
- Pattern matching: named nodes, anonymous nodes, wildcards, fields, captures,
  alternations, quantifiers, anchors, negated fields
- Predicate step extraction (not evaluation)
- Standard predicate helpers (`#eq?`, `#match?`, `#any-of?`)
- Tests: query patterns from real tree-sitter highlight/tag queries

### Phase 8: Polish and Ecosystem (ongoing)

**Goal**: Production readiness.

**Deliverables**:
- Comprehensive benchmark suite with comparisons to C
- Performance optimization based on profiling
- API documentation
- Pre-compiled grammar packages for popular languages
- CI pipeline for grammar generation

---

## 10. Key Risks and Open Questions

### Risk: GLR Stack Complexity

The Graph-Structured Stack (`stack.c`, ~900 lines) is the most complex data
structure in tree-sitter. It supports version forking (on ambiguity), merging
(when versions converge to the same state), pausing (for error recovery), and
halting (when a version is discarded). The pop operation fans out across merged
predecessor links, bounded by `MAX_ITERATOR_COUNT = 64`.

**Mitigation**: Port `stack.c` methodically, function by function. Write extensive
tests for each operation: push, pop with multiple paths, version merging,
version halting. Use the C parser as a reference oracle — for any input, both
implementations must produce the same tree.

### Risk: Lex Function Code Generation

The C lex function uses `goto`-based macros that the C compiler optimizes into
efficient jump tables. The Go equivalent (`for/switch`) may be significantly
slower for grammars with many lex states (e.g., TypeScript has ~4000 lex states).

**Mitigation**: Profile early. If the `for/switch` pattern is too slow, consider:
- Generating a flat state-transition table and using a table-driven DFA interpreter
  (trades code size for data size)
- Using `goto`-like patterns via labeled loops (Go doesn't have goto to labels
  in switches, but `break`/`continue` with labels can help)
- For the `ADVANCE_MAP` macro (many single-character transitions), generate a
  lookup table indexed by rune value

### Risk: External Scanner Porting Effort

~50% of grammars need external scanners. Some scanners are complex (TypeScript:
~1000 lines, C++: ~2000 lines). Each must be manually ported to Go.

**Mitigation**: Prioritize grammars by popularity. The top 10 languages cover
90%+ of use cases. Build an auto-translation tool once patterns are understood.
Offer CGo fallback for extreme cases.

### Risk: Parse Tree Compatibility

The Go parser must produce byte-for-byte identical parse trees to the C parser
for correctness. Any divergence means syntax highlighting, code navigation, and
structural queries will behave differently.

**Mitigation**: Build a differential testing framework early. For each grammar:
1. Parse a corpus of files with both C and Go parsers
2. Compare S-expression output
3. Compare node byte ranges
4. Compare query match results

Use the tree-sitter test corpus (`test/corpus/` in each grammar repo) as the
primary validation set.

### Risk: GC Pauses During Large Parses

Parsing a 1MB file creates ~100K subtree objects. GC scanning this object graph
could cause noticeable pauses.

**Mitigation**:
- Arena allocation (primary) — reduces individual allocations to a handful of
  slice allocations, directly reducing GC tracing work
- Inline subtrees eliminate ~60-80% of heap allocations for leaf tokens
- `sync.Pool` for `StackNode` and other non-arena allocations
- If GC pauses are measured and problematic, explore `GOGC` tuning or
  `runtime.SetMemoryLimit`

### Open Question: Grammar Distribution

How should compiled grammar packages be distributed?

**Option A**: Each grammar is its own Go module (e.g.,
`github.com/treesitter-go/tree-sitter-go`). Users `go get` the grammars they need.

**Option B**: A monorepo with all grammars (e.g.,
`github.com/treesitter-go/grammars/go`). Single import path, versioned together.

**Option C**: Users run `tsgo-generate` themselves against the upstream grammar
repo and vendor the output.

Recommendation: Start with Option C (users generate their own) for the MVP, then
move to Option A (individual modules) for popular grammars.

### Decision: Concurrency Model

`Parser` is **not** goroutine-safe — a single `Parser` must not be used from
multiple goroutines concurrently. This matches the C parser's threading model.
`Tree`, `Node`, `Query`, and `Language` are safe for concurrent read access
(they are immutable after creation). Users who need concurrent parsing should
create separate `Parser` instances.

### Decision: Cancellation via context.Context

The `Parser.Parse()` and `Parser.ParseString()` methods accept a
`context.Context` parameter. The parser's main loop checks `ctx.Done()` at the
same points where the C parser checks its cancellation flag
(`ts_parser_set_timeout_micros` / `ts_parser_set_cancellation_flag`). This is
the idiomatic Go approach and should be integrated from day one rather than
retrofitted.

### Error Recovery Fidelity

Tree-sitter's error recovery explores multiple strategies (missing token insertion,
speculative reductions, token skipping) as parallel stack versions, pruned by an
error cost model. The cost constants are:

```
ERROR_COST_PER_RECOVERY    = 500
ERROR_COST_PER_MISSING_TREE = 110
ERROR_COST_PER_SKIPPED_TREE = 100
ERROR_COST_PER_SKIPPED_LINE = 30
ERROR_COST_PER_SKIPPED_CHAR = 1
MAX_VERSION_COUNT          = 6
MAX_COST_DIFFERENCE        = 1800
```

These constants are empirically tuned. We should use the same values to match
the C parser's error recovery behavior, then re-evaluate if differential testing
reveals cases where the Go parser recovers differently.
