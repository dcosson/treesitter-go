# Addendum: Advanced Lexer Performance for treesitter-go

*Addendum to [docs/design.md](design.md) — focused on lexer code generation and achieving better-than-native tree-sitter performance.*

---

## 1. How Tree-sitter's C Lexer Actually Works

### Architecture: Runtime + Generated Code

Tree-sitter's lexer is split into two components:

**Runtime lexer** (`lib/src/lexer.c`, ~480 lines): Manages chunked input reading, Unicode
decoding (UTF-8/UTF-16), position tracking (byte offset + row/column), included range
boundaries, and the `TSLexer` interface (function pointers for `advance`, `mark_end`,
`get_column`, `eof`, `is_at_included_range_start`). This code is grammar-independent.

**Generated lex function** (in each grammar's `parser.c`): A grammar-specific DFA
state machine implementing tokenization. This is the performance-critical code — it's
called for every token in the input. The generated code uses preprocessor macros
defined in `lib/src/parser.h` that expand to `goto`-based state transitions.

### Generated Code Structure

Examining tree-sitter-javascript's generated `parser.c` (94,268 lines, 1,870 parse
states, 279 lex states, 134 token types):

```c
static bool ts_lex(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 0:
      if (eof) ADVANCE(126);
      ADVANCE_MAP(
        '!', 226,
        '"', 159,
        '#', 4,
        '$', 273,
        // ... ~30 character → state mappings
      );
      if (('1' <= lookahead && lookahead <= '9')) ADVANCE(259);
      if (set_contains(extras_character_set_1, 10, lookahead)) SKIP(123);
      if (lookahead > '@') ADVANCE(275);
      END_STATE();
    case 1:
      // ...
    // ... 278 more cases
  }
}
```

### Key Dispatch Mechanisms

**1. `ADVANCE_MAP(char, state, ...)` — Linear scan through pairs:**
```c
#define ADVANCE_MAP(...)                                              \
  {                                                                   \
    static const uint16_t map[] = { __VA_ARGS__ };                    \
    for (uint32_t i = 0; i < sizeof(map) / sizeof(map[0]); i += 2) { \
      if (map[i] == lookahead) {                                      \
        state = map[i + 1];                                           \
        goto next_state;                                              \
      }                                                               \
    }                                                                 \
  }
```
This is a **linear scan** through a static array of `(character, next_state)` pairs.
For the JavaScript grammar, `ADVANCE_MAP` entries typically contain 15-30 pairs per
state — making this O(15-30) comparisons per state transition. This is the dominant
character dispatch mechanism.

**2. `set_contains(ranges, len, lookahead)` — Binary search through sorted ranges:**
```c
static inline bool set_contains(const TSCharacterRange *ranges, uint32_t len,
                                int32_t lookahead) {
  // Binary search over sorted [start, end] ranges
  uint32_t index = 0;
  uint32_t size = len - index;
  while (size > 1) {
    uint32_t half_size = size / 2;
    uint32_t mid_index = index + half_size;
    if (lookahead >= ranges[mid_index].start && lookahead <= ranges[mid_index].end)
      return true;
    else if (lookahead > ranges[mid_index].end)
      index = mid_index;
    size -= half_size;
  }
  return (lookahead >= ranges[index].start && lookahead <= ranges[index].end);
}
```
Used for Unicode character classes (e.g., identifier characters, whitespace). The
JavaScript grammar defines sets like:
```c
static const TSCharacterRange extras_character_set_1[] = {
  {'\t', '\r'}, {' ', ' '}, {0xa0, 0xa0}, {0x1680, 0x1680},
  {0x2000, 0x200b}, {0x2028, 0x2029}, {0x202f, 0x202f},
  {0x205f, 0x2060}, {0x3000, 0x3000}, {0xfeff, 0xfeff},
};
```

**3. `ADVANCE(state)` / `SKIP(state)` — `goto`-based state transitions:**
```c
#define START_LEXER()           \
  bool result = false;          \
  bool skip = false;            \
  int32_t lookahead;            \
  goto start;                   \
  next_state:                   \
  lexer->advance(lexer, skip);  \
  start:                        \
  skip = false;                 \
  lookahead = lexer->lookahead;

#define ADVANCE(state_value) { state = state_value; goto next_state; }
```
The `goto next_state` jumps directly to the character-reading code, then falls
through to `start:` which re-enters the main `switch`. The C compiler can optimize
the `switch` into a jump table for dense cases.

### Keyword Recognition (Separate DFA)

Tree-sitter uses a two-phase approach for keywords:

1. The main `ts_lex` function recognizes identifiers as a single token type
   (`keyword_capture_token`).
2. If the result is an identifier, the parser resets the lexer position and runs
   `ts_lex_keywords` — a second, smaller DFA that specifically recognizes keywords.
3. If the keyword DFA matches and the keyword is valid in the current parse state,
   the token is reclassified.

The keyword DFA is essentially a trie: each state advances on a single character,
branching to match keyword prefixes:
```c
static bool ts_lex_keywords(TSLexer *lexer, TSStateId state) {
  // ...
  case 0:
    ADVANCE_MAP('a', 1, 'b', 2, 'c', 3, 'd', 4, /* ... */);
  case 1:
    if (lookahead == 's') ADVANCE(20);  // "as"
    if (lookahead == 'w') ADVANCE(21);  // "aw" (await)
  // ...
}
```

### Parser-Lexer Coupling

The parser drives the lexer with context:

```c
// parser.c — simplified flow
ts_lexer_start(&self->lexer);
found_token = self->language->lex_fn(&self->lexer.data, lex_mode.lex_state);
ts_lexer_finish(&self->lexer, &lookahead_end_byte);
```

Each parse state maps to a `TSLexerMode` containing `{lex_state, external_lex_state,
reserved_word_set_id}`. The `lex_state` selects which DFA start state to use, meaning
different parser contexts run different subsets of the lexer. This context-awareness
is important: **the lexer is not a standalone tokenizer** — it produces different
tokens depending on the parser state.

### Performance Characteristics of the C Implementation

**Strengths:**
- `goto`-based dispatch eliminates loop overhead (no `for` iteration, no `switch`
  re-evaluation)
- C compiler generates jump tables for dense `switch` cases
- Static data arrays are in read-only memory, cache-friendly
- Keyword trie is compact and branch-predictor-friendly (few branches per character)

**Weaknesses (opportunities for us):**
- `ADVANCE_MAP` uses **linear scan** — O(N) per state for N transition pairs.
  With 15-30 pairs typical, this is 15-30 comparisons per character in many states.
- `set_contains` uses **binary search** — O(log N) for N ranges. Better than linear
  but still multiple comparisons and branches.
- No SIMD acceleration for character classification, whitespace skipping, or string
  scanning.
- No profile-guided optimization of branch ordering within states.
- No special-casing for the ASCII fast path (>99.99% of source code characters).

---

## 2. Baseline: Generated Go Switch

### The Approach

Our baseline code generator (described in the design doc) compiles each grammar's
lexer to a Go function using nested `for`/`switch` statements:

```go
func lexMain(lexer *ts.Lexer, state ts.StateID) bool {
    for {
        switch state {
        case 0:
            if lexer.EOF() { lexer.AcceptToken(ts.SymEnd); return true }
            switch lexer.Lookahead() {
            case '(': lexer.Advance(false); state = 5
            case ')': lexer.Advance(false); state = 6
            // ... more cases
            default:
                if lexer.Lookahead() >= 'a' && lexer.Lookahead() <= 'z' {
                    lexer.Advance(false); state = 20
                } else {
                    return false
                }
            }
        case 5:
            lexer.AcceptToken(symLParen); return true
        // ... more states
        }
    }
}
```

### What the Go Compiler Does

**Dense switch → jump table**: The Go compiler (gc) translates a `switch` on an
integer variable into a jump table when the cases are sufficiently dense. For a
`switch` with cases 0-278 (like the JavaScript grammar's lex states), the compiler
emits an indirect branch through a table of code pointers. This is comparable to C's
`switch` jump table.

**Inner character switch**: The character-level `switch` within each state also
gets jump table treatment when cases are dense ASCII values. For cases like
`'(', ')', '*', '+', ','` etc., the Go compiler emits an efficient jump table
since these are dense in the 0x28-0x7E range.

**Bounds check elimination**: For table lookups like `table[state * width + class]`,
the Go compiler can often eliminate bounds checks when the index is provably in range.
The `_ = table[maxIndex]` pattern explicitly teaches the compiler the slice bounds.

### Performance Gap vs C

The main sources of overhead in the Go baseline vs C:

| Source | Estimated Overhead | Notes |
|--------|-------------------|-------|
| `for/switch` vs `goto` | ~5-15% | Go's switch has loop iteration + bounds check |
| Function call for `Advance` | ~5-10% | C uses inline function via pointer, Go method call |
| Bounds checks on table access | ~2-5% | Partially eliminated by compiler |
| Garbage collector pauses | Variable | Not in the lexer itself, but affects overall parse |
| No computed goto | ~10-20% | C's goto eliminates re-dispatching through the switch |

**Estimated total baseline overhead: 20-40% slower than C tree-sitter's lexer.**

This is already acceptable (the design doc targets 2-5x for the full parser). But
we want to go further — we want to **beat** C tree-sitter's lexer.

### Why Beating C Is Possible

C tree-sitter's lexer has algorithmic weaknesses we can exploit:

1. **`ADVANCE_MAP` is O(N) linear scan** — we can use O(1) lookup tables
2. **No SIMD** — we can classify 16-32 bytes at once
3. **No profile-guided optimization** — we can reorder branches based on real-world
   character frequencies
4. **No fast-path specialization** — we can generate specialized scanners for common
   token patterns (identifiers, whitespace, strings)
5. **The keyword trie re-scans the token** — we can use O(1) perfect hash functions

---

## 3. Advanced Algorithms to Beat C

### 3a. Data-Parallel Finite-State Machines

*Mytkowicz, Musuvathi, Schulte — "Data-Parallel Finite-State Machines" (ASPLOS 2014)*

**The algorithm.** The fundamental problem is that FSM computation is inherently
sequential: state `q_i = δ(q_{i-1}, s_i)` depends on the previous state. The insight
is to reformulate this by tracking a **state vector** — an array `S` where `S[j]`
is the state the FSM would be in if it had started from state `j`. For each input
character, apply the transition function to every element simultaneously:

```
S'[j] = δ(S[j], c)   for all states j
```

State transition functions form a **monoid under composition**, enabling parallel
prefix computation. The input is divided into chunks processed independently; each
chunk produces a function from start states to end states. These functions are
composed via parallel prefix sum in O(log P) depth.

**SIMD implementation.** If the FSM has ≤16 states, the state vector fits in one
SSE register. The PSHUFB (packed shuffle bytes) instruction performs 16 parallel
table lookups in a single cycle — applying one transition for all 16 starting states
simultaneously. With AVX2 (VPSHUFB), this extends to 32-byte registers (though
VPSHUFB operates on two independent 128-bit lanes). For FSMs with >16 states, range
coalescing encodes state names so that lookup tables remain ≤16 entries.

**Convergence.** When running from all N starting states, distinct active states
converge rapidly: ~90% of real-world DFAs converge to ≤16 active states within ~10
input characters. This occurs because real FSMs have absorbing structures — common
characters (spaces, letters) are handled identically across many states.

**Application to tree-sitter lexing.** Tree-sitter's context-aware, per-token lexing
model is a **poor fit** for full parallel DFA execution:
- The valid token set changes at every token boundary
- Most tokens are short (1-10 characters) — SIMD setup overhead exceeds benefit
- The FSM effectively restarts after each token

**Paper results:** 2.3x on HTML tokenization, ~2x on Huffman decoding, up to 3x on
regex matching (single-core SIMD).

**Verdict: Not recommended for the main lexer loop.** The technique is designed for
long, uniform FSM runs (hundreds+ characters), not tree-sitter's short per-token
scans. However, the underlying SIMD character classification primitives (PSHUFB
lookups) are extremely valuable — see section 3b.

*References: Mytkowicz et al. ASPLOS 2014; Jiang & Agrawal PPoPP 2017; Li & Taura,
SimdFSM PDCAT 2022.*

### 3b. SIMD-Accelerated Character Classification

*Based on simdjson (Langdale & Lemire, VLDB 2019), Hyperscan (Wang et al., NSDI
2019), and Chromium's HTML scanner.*

**The core technique: nibble lookup.** Decompose each input byte into high nibble
(bits 7-4) and low nibble (bits 3-0). Use two VPSHUFB lookups — one indexed by
the low nibble, one by the high nibble — then AND the results. This classifies
32 bytes simultaneously in ~5 instructions:

```
// Classify 32 bytes of input into character categories
input_lo = VPAND(input, 0x0F)           // extract low nibbles
input_hi = VPSRLW(input, 4)             // shift right 4
input_hi = VPAND(input_hi, 0x0F)        // mask high nibbles
class_lo = VPSHUFB(low_table, input_lo) // lookup in 16-entry table
class_hi = VPSHUFB(high_table, input_hi)// lookup in 16-entry table
classes  = VPAND(class_lo, class_hi)    // combine: bit k set if byte ∈ class k
```

Each result byte is a **bitmask** — up to 8 character classes can be distinguished
simultaneously (one bit per class). For a lex state with transitions on
`{letters, digits, underscore, whitespace, operators, other}`, a single pair of
VPSHUFB instructions classifies the entire block.

The VPSHUFB instruction has a throughput of 1 per cycle and latency of 1 cycle on
modern x86. The full classification of 32 bytes completes in ~2-3 cycles — processing
at **10-16 bytes per cycle**.

**Whitespace scanning.** The simplest and highest-impact application. Source code is
~20-25% whitespace by byte count. Using SIMD:

```
// Find first non-whitespace in 32 bytes
ws_mask = VPCMPEQB(input, broadcast(' '))
ws_mask = VPOR(ws_mask, VPCMPEQB(input, broadcast('\t')))
ws_mask = VPOR(ws_mask, VPCMPEQB(input, broadcast('\n')))
ws_mask = VPOR(ws_mask, VPCMPEQB(input, broadcast('\r')))
bits = VPMOVMSKB(ws_mask)               // 32-bit mask
bits = ~bits                             // invert: 1 = non-whitespace
first_nonws = TZCNT(bits)               // count trailing zeros
```

If `first_nonws == 32`, all 32 bytes are whitespace — advance by 32 and repeat.
Expected speedup for whitespace-heavy runs: **5-10x** vs scalar character-by-character
scanning. Lemire's benchmarks show 0.25 cycles/byte for whitespace processing with
SSE vs ~2.4 cycles/byte scalar.

**Identifier scanning.** Fast scan for `[a-zA-Z0-9_]` runs. Either use the nibble
classification approach or SSE4.2's PCMPISTRI instruction, which checks 16 bytes
against up to 8 character ranges in a single instruction. For identifiers:

```
// SSE4.2 ranges: 'a'-'z', 'A'-'Z', '0'-'9', '_'-'_'
ranges = {'0', '9', 'A', 'Z', 'a', 'z', '_', '_'}
index = PCMPISTRI(ranges, input, 0x04)  // first non-matching byte
```

**String literal scanning.** Fast scan for unescaped string content — find the next
`"` or `\`:

```
q_mask = VPCMPEQB(input, broadcast('"'))
b_mask = VPCMPEQB(input, broadcast('\\'))
combined = VPOR(q_mask, b_mask)
bits = VPMOVMSKB(combined)
if bits == 0: advance 32; continue
offset = TZCNT(bits)                    // first terminator
```

This processes 32 bytes in ~4 instructions per iteration. For long strings (common
in JSON, config files, test fixtures), this provides dramatic speedup.

**Newline tracking.** When SIMD-skipping whitespace, we need to count newlines for
position tracking. After computing the whitespace mask, extract newline positions:
`nl_mask = VPCMPEQB(input, '\n'); nl_bits = VPMOVMSKB(nl_mask); count = POPCNT(nl_bits)`.
This adds ~3 instructions to the inner loop.

**ARM64 equivalent.** The ARM NEON `TBL` instruction is equivalent to VPSHUFB — it
performs 16-byte table lookup. NEON's TBL can actually index into up to 4 concatenated
registers (64 entries), more flexible than x86's 16-entry limit. The overall approach
is architecture-portable.

**Production precedents:**
- simdjson: 2.5 GB/s JSON parsing using this technique
- Chromium HTML scanner: 6.8 GB/s character classification (vs 2.0 GB/s scalar)
- simdjson-go (Minio): 10x faster than `encoding/json` in Go, using c2goasm-generated
  Plan9 assembly
- simd-lexing (Mateuszd6): 6.8x faster than flex on GCC source code with AVX2

**Expected speedup for tree-sitter lexing:** 1.5-3x for the lexer hot path. Since
the lexer is one part of total parse time, overall parse speedup: **1.1-1.5x**.
Whitespace-heavy and string-heavy inputs benefit most.

**Verdict: High-value, moderate complexity. Implement in Phase 1 optimizations.**

*References: Langdale & Lemire arXiv:1902.08318; Mula 0x80.pl/notesen/2018-10-18;
Lemire lemire.me/blog/2017/01/20; simdjson-go github.com/minio/simdjson-go.*

### 3c. Succinct Automata / Entropy-Coded Transition Tables

*Chakraborty et al. — "Succinct Representation for (Non)Deterministic Finite
Automata"; Kumar et al. — D2FA; Navarro — compact data structures.*

**Succinct automata** encode DFA transition tables using rank/select bit vectors.
For n states over alphabet size σ, the space is `(σ-1)·n·log(n) + O(n·log(σ))` bits
with O(1) transition lookups via rank operations (count of 1-bits up to position i).

**Practical assessment for tree-sitter:** For a grammar with n=2000 states and σ=128
character classes:
- Succinct representation: ~350 KB (with rank/select overhead)
- Naive flat table: 2000 × 128 × 2 = 512 KB

The 30% space savings come at the cost of **multiple rank operations per transition**
instead of a single array index. At tree-sitter's scale (hundreds to low thousands
of states), the constant-factor overhead of rank/select indirection makes this a
net loss for the hot path. Succinct representations shine for million-state DFAs
(network intrusion detection), not thousand-state lexers.

**D2FA (Delayed DFA)** exploits the observation that many DFA states share most
transitions. A "default transition" redirects from state A to a similar state B,
storing only the differing transitions explicitly. This achieves >90% space reduction
on network intrusion DFAs, but adds O(chain_depth) overhead per character from
following default transition chains. Not appropriate for lexer hot loops.

**Cache behavior analysis for real grammars:**

| Grammar | Lex States | Char Classes | Table Size | Cache Fit |
|---------|-----------|-------------|-----------|-----------|
| JSON | ~30 | ~20 | ~1.2 KB | L1 |
| Go | ~300-500 | ~60-80 | 40-80 KB | L2 |
| C/C++ | ~500-1000 | ~80-100 | 80-200 KB | L2 |
| TypeScript | ~2000-4000 | ~100-128 | 400 KB-1 MB | L2/L3 |

The hot working set is much smaller than the full table. A typical lexer spends
80-90% of time in 10-20% of states (identifiers, whitespace, operators, strings).
For a 500-state Go lexer, the hot ~50-100 states occupy 4-13 KB — easily fits in
L1's 32-48 KB.

**What's actually worth doing:**
- Equivalence classes (already in tree-sitter's design) — reduces σ from 256 to ~60-128
- Flat indexed table with premultiplied offsets: `table[state_offset + class]`
- BFS state numbering so hot states cluster at low indices
- For extreme grammars only: adaptive dense/sparse (dense table for hot states,
  sparse for cold states)

**Verdict: Not recommended. The flat table already has excellent cache behavior for
most grammars. Compression adds complexity without meaningful benefit at this scale.**

*References: Chakraborty et al. arXiv:1907.09271; BurntSushi aho-corasick DESIGN.md;
Kumar et al. D2FA.*

### 3d. Statistical / Adaptive Methods

*Profile-guided optimization; Hindle et al. — "On the Naturalness of Software"
(ICSE 2012); Zipf's Law in source code.*

**Statistical properties of real code.** Source code token distributions follow
Zipf's law. Rough breakdown for a typical language:

| Token Category | Share | Avg Length |
|---------------|-------|-----------|
| Identifiers | 25-40% | 4-12 chars |
| Whitespace/newlines | 15-25% | 1-8 chars |
| Punctuation `(){}[];,.` | 15-25% | 1 char |
| Keywords | 8-15% | 2-8 chars |
| Operators | 5-10% | 1-3 chars |
| String/numeric literals | 5-10% | Variable |
| Comments | 2-8% | Variable |

Source code is **more predictable than natural language**: cross-entropy of 3-4
bits/token (vs 8-10 for English), meaning 8-16 guesses predict the next token.

At the character level, source code is >99.99% ASCII. The most frequent characters
are space, `e`, `t`, `a`, `i`, `n`, `o`, `s`, `r` (from identifiers), followed by
punctuation `(){}[];,.`.

**Profile-guided optimizations for code generation:**

**1. Hot-path branch reordering.** In each lex state's character dispatch, reorder
comparisons so the most frequent characters are checked first. For a Go identifier
state: check `a-z` before `A-Z` before `_` before `0-9` before Unicode. This directly
addresses `ADVANCE_MAP`'s linear scan — even in a Go switch, the compiler benefits
from knowing the common cases.

**2. Specialized fast paths.** If 60% of lexer invocations produce an identifier,
generate a "try identifier first" fast path that handles the common case
(all-lowercase-ASCII identifier) without entering the full DFA:

```go
// Fast path: most common token is an identifier starting with lowercase
if c >= 'a' && c <= 'z' {
    for src[pos] >= 'a' && src[pos] <= 'z' || src[pos] >= '0' && src[pos] <= '9' ||
        src[pos] == '_' || src[pos] >= 'A' && src[pos] <= 'Z' {
        pos++
    }
    lexer.AcceptToken(symIdentifier)
    // Check for keyword match
    return true
}
// Fall through to full DFA
```

**3. Token-specific scanners.** Rather than a monolithic DFA, generate specialized
scanner functions for the 5-6 most common token types. The main dispatch reads one
character and calls the appropriate scanner. This gives the Go compiler maximum
opportunity for inlining and register allocation.

**4. Keyword perfect hash.** Instead of tree-sitter's keyword trie (which rescans
the entire token character by character), use a compile-time-generated perfect hash
function. For Go's 25 keywords, a perfect hash can identify the keyword with a
single hash computation + one string comparison:

```go
func matchKeyword(s []byte) Symbol {
    if len(s) < 2 || len(s) > 11 { return 0 }
    h := keywordHash(s)          // perfect hash → index
    kw := keywordTable[h]
    if string(s) == kw.text {
        return kw.symbol
    }
    return 0
}
```

**5. ASCII fast path.** Since >99.99% of source code is ASCII, avoid UTF-8 rune
decoding on the hot path. Read bytes directly, only falling back to `utf8.DecodeRune`
for bytes ≥ 0x80:

```go
c := src[pos]
if c < 0x80 {
    // ASCII fast path — no decoding needed
    class = asciiClassTable[c]   // 128-byte lookup table
} else {
    r, size = utf8.DecodeRune(src[pos:])
    class = unicodeClass(r)      // binary search through ranges
}
```

**Markov-chain prediction.** Source code character bigrams are highly predictable
(after `{` → newline; after `if` → space). However, the DFA already encodes order-1
predictions (each state implicitly represents the history), and hardware branch
predictors learn order-2+ patterns in hot loops. Software prediction adds overhead
without meaningful improvement over what the DFA + CPU provide.

**Grammar-specific compile-time optimizations:**
- Go: semicolons are implicit (injected at newlines after certain tokens) — a single
  bit flag test, not a DFA state
- Python: indentation is significant — specialized whitespace-at-line-start scanner
- JavaScript: template literals require context-dependent `` ` `` scanning

**Expected speedup:** 20-40% from branch reordering + fast paths + ASCII optimization,
directly closing the gap with C's `goto`-based dispatch.

**Verdict: High value, low-medium complexity. Core of our Phase 1 strategy.**

*References: Hindle et al. ICSE 2012; Eli Bendersky's Go lexer benchmarks; re2c
benchmarks re2c.org/benchmarks; nothings.org/computer/lexing.html.*

### 3e. Vectorized String Matching / Keyword Detection

*Teddy algorithm (Intel Hyperscan); BurntSushi aho-corasick; SIMD keyword matching.*

**The Teddy algorithm** (from Hyperscan) uses SIMD for multi-pattern short-string
matching. Patterns are grouped into buckets (up to 8 with SSE, 16 with AVX2).
For each pattern, short fingerprints (1-3 byte prefixes) are extracted. Two
nibble-indexed VPSHUFB masks encode which buckets match each nibble value:

```
For each 16-byte input block:
  B_lo = B AND 0x0F                    // low nibbles
  B_hi = (B >> 4) AND 0x0F            // high nibbles
  C0 = PSHUFB(A0_mask, B_lo)          // bucket matches for low nibble
  C1 = PSHUFB(A1_mask, B_hi)          // bucket matches for high nibble
  C = C0 AND C1                        // both nibbles must match
  if PMOVMSKB(C) != 0:                // any candidates?
    verify_full_match(...)
```

**Application to keyword recognition.** For a language with 50 keywords, Teddy
could simultaneously check 16 bytes of input against all keyword fingerprints.
After the main lexer identifies an identifier, Teddy checks if it matches any
keyword in a single SIMD pass.

**BurntSushi's aho-corasick layered strategy:**
1. For 1-3 distinct patterns: `memchr` (SIMD byte search)
2. For ≤100 patterns: Teddy prefilter
3. For large sets: full Aho-Corasick DFA

**However, for tree-sitter keyword matching specifically**, simpler approaches are
competitive:
- **Switch on length, then first character**: For <100 keywords, a two-level dispatch
  (length → first byte → string compare) requires 2-3 comparisons and no SIMD.
- **Perfect hash function**: O(1) lookup for any keyword count. Fits keywords into
  a single cache line.
- **Compile-time trie with early exit**: Matches tree-sitter's current approach but
  generated as Go code with optimized branch ordering.

Since the lexer has already scanned the identifier (it knows the start, end, and
content), keyword matching doesn't need to scan the input again — it just needs
to classify a known string of length 2-11 into one of ~25-80 buckets.

**Expected speedup over tree-sitter's keyword trie:** 2-4x for the keyword matching
phase, but keywords are only 8-15% of tokens, so the global impact is modest (~5-10%
overall lexer speedup).

**Verdict: The perfect hash approach gives most of the benefit with minimal
complexity. Teddy is overkill for keyword matching but useful if we build general
SIMD infrastructure.**

*References: BurntSushi aho-corasick DESIGN.md; Hyperscan NSDI 2019; branchfree.org
Teddy analysis.*

### 3f. JIT / Runtime Code Generation

**What a JIT lexer would emit.** Native machine code specialized per DFA state:
branch-free character classification via lookup tables, computed gotos via label
addresses, inlined token actions. Each state becomes a labeled block:

```asm
state_42:
    movzx eax, byte [rsi]            ; load character
    movzx eax, byte [class_table+rax]; lookup equivalence class
    jmp [dispatch_42+rax*8]          ; computed goto to handler
```

**Go's JIT options:**
1. **Plan9 assembly at build time** (not JIT, but enables SIMD + custom dispatch)
2. **mmap executable memory** — proven by nelhage's gojit and wazero. Allocate with
   `syscall.Mmap(PROT_READ|PROT_WRITE)`, write machine code, change to
   `PROT_READ|PROT_EXEC` (W^X). Call via assembly trampoline. ~4-5ns function call
   overhead.
3. **Go plugin system** — `go build -buildmode=plugin` + `plugin.Open`. Not true JIT,
   requires exact toolchain version match.

**Is JIT worth it?** For a tree-sitter lexer that's generated at compile time: **no**.

Arguments against:
- Most benefit is already captured by compile-time code generation. A well-generated
  Go lexer with computed dispatch is already within 10-20% of hand-optimized C.
- JIT provides ~10-30% improvement over already-compiled code (based on PCRE2-JIT
  vs PCRE2 compiled).
- Lexing is rarely the bottleneck — memory allocation and parser state management
  dominate.
- JIT in Go requires unsafe operations, breaks the safety model, complicates
  debugging/profiling, and introduces platform-specific code.
- Go's compiler improves over time — generated Go code automatically benefits.

The one scenario where JIT helps: **dynamic grammar loading** at runtime without
recompilation. But table-driven interpretation is acceptable for this use case
(interactive editing with small files and incremental parsing).

**Verdict: Not recommended. Compile-time code generation captures the benefit
without the complexity. Plan9 assembly for SIMD hot paths is the right level of
"close to the metal."**

*References: nelhage gojit; wazero optimizing compiler; mathetake JIT-in-Go blog
post; PCRE2-JIT benchmarks.*

### 3g. Cache-Oblivious / Cache-Aware State Layout

**Van Emde Boas layout** optimizes for tree-shaped access patterns by recursively
splitting trees and placing subtrees contiguously. However, DFA traversal follows
arbitrary graph transitions determined by input data, not tree paths. VEB layout
does not directly help.

**The useful insight is state reordering.** If states frequently visited in sequence
are close in memory, cache locality improves. Approaches:

**1. BFS state numbering:** Number DFA states in BFS order from the start state.
Hot states (reachable via common transitions) get low IDs and cluster together
in memory. Zero implementation cost.

**2. Reverse Cuthill-McKee (RCM):** Reorder states to minimize bandwidth of the
transition graph. Available in standard graph libraries. Reduces the maximum distance
between connected states in the ordering.

**3. Profile-guided reordering:** Run the lexer on representative code, record
transition frequencies, cluster hot states together. This is a compile-time
optimization in the code generator.

**Working set analysis for real grammars:**
- **Hot states** (whitespace, identifiers, operators, strings): 20-50 states,
  ~5-15% of total
- **Hot state table size**: 50 states × 128 classes × 2 bytes = 12.8 KB → L1 (32-48 KB)
- **Full table**: 2000 states × 128 × 2 = 512 KB → may spill L2

The hot working set almost always fits in L1. The practical bottleneck is more often
**instruction cache pressure** from large generated switch statements than data
cache misses on the transition table.

**Adaptive dense/sparse representation:** Dense flat table for hot states (0..K),
sparse sorted transition lists for cold states (K+1..N). If K covers the hot
~50-100 states, the dense region is 12-25 KB (L1-friendly). Cold states add memory
proportional to actual transitions, not `state × σ`. The inner loop needs a branch
(`if state < K { dense } else { sparse }`), but the cold branch is well-predicted.

This is worth considering only for extreme grammars (TypeScript, Zig scale) where
the full table exceeds L2.

**Expected benefit:** Modest for most grammars. BFS numbering is free. Profile-guided
reordering helps for L2-spilling grammars by ~10-30%.

**Verdict: BFS state numbering is free and should always be applied. Adaptive
dense/sparse is a Phase 2 optimization for large grammars only.**

*References: BurntSushi aho-corasick DESIGN.md (contiguous NFA); Cuthill-McKee
algorithm; nothings.org lexer benchmarks.*

### 3h. DFA Decomposition

*Krohn-Rhodes theory; Kupferman & Mosheiff — "Prime Languages".*

**Kronecker product decomposition** expresses a DFA as the intersection of smaller
sub-automata whose state spaces form a Cartesian product. If DFA A (n states) can
be decomposed into A1 (n1 states) × A2 (n2 states), we store n1 + n2 states where
n1 × n2 ≥ n. The Krohn-Rhodes theorem guarantees any finite automaton decomposes
into cascades of "reset" and "permutation" automata, but this algebraic decomposition
doesn't yield compact representations.

**Practical assessment for programming language lexers: decomposition does not apply.**

1. Lexer DFAs encode complex interactions between token types — keyword vs identifier
   disambiguation mixes concerns across states
2. The DFA is already minimized by the grammar compiler, removing structural redundancy
3. Lexer DFAs are not strongly connected (they have absorbing accept/error states)
4. Finding valid decompositions is computationally hard — at minimum NP-hard for
   general DFAs, with a doubly-exponential gap between known bounds
5. Running multiple sub-automata and intersecting results adds per-character overhead
6. Tree-sitter already decomposes by context: `lex_modes` maps each parser state to
   a specific lex start state, giving each parser context its own lexer entry point

**Verdict: Not applicable. Tree-sitter's existing lex_modes mechanism already
achieves practical context-dependent decomposition. Formal DFA decomposition adds
complexity with no benefit.**

*References: Kupferman & Mosheiff, Theoretical Computer Science 2015; Jecker et al.
CONCUR 2021.*

---

## 4. Evaluation Summary

For each technique, the expected impact vs implementation cost:

| Technique | Expected Speedup | Complexity | Helps With | Phase |
|-----------|-----------------|------------|------------|-------|
| ASCII fast path (no UTF-8 decode) | 10-20% | Low | Everything | 0 |
| Equivalence class lookup table | 15-25% | Low | Replace ADVANCE_MAP | 0 |
| BFS state numbering | 2-5% | Zero | Cache locality | 0 |
| Profile-guided branch ordering | 10-20% | Low-Med | All dispatch code | 1 |
| Specialized token scanners | 15-30% | Medium | Identifiers, strings | 1 |
| Keyword perfect hash | 5-10% | Low | Keyword recognition | 1 |
| SIMD whitespace scanning | 20-40% (ws path) | Medium | Whitespace-heavy code | 1 |
| SIMD string/comment scanning | 30-60% (str path) | Medium | String-heavy code | 1 |
| SIMD character classification | 15-30% | Med-High | General lexer dispatch | 2 |
| SIMD identifier scanning | 20-40% (id path) | Medium | Identifier-heavy code | 2 |
| Adaptive dense/sparse tables | 5-15% (large grammars) | Medium | TypeScript, Zig | 2 |
| SIMD keyword matching (Teddy) | 2-5% | High | Keyword-dense code | 3 |
| Data-parallel FSM | Marginal | Very High | N/A for tree-sitter | Skip |
| Succinct automata | Net negative | Very High | N/A at this scale | Skip |
| DFA decomposition | Net negative | Very High | N/A for lexers | Skip |
| JIT code generation | 10-30% | Very High | Dynamic grammars only | Skip |

---

## 5. Phased Implementation Plan

### Phase 0: Generated Go Switch (Baseline)

*Fallback that always works. Target: within 2x of C tree-sitter.*

- Generate Go lex functions as `for`/`switch` state machines
- Use character equivalence classes with O(1) lookup table (128-byte ASCII table +
  binary search for Unicode)
- Number states in BFS order from start state
- Implement `set_contains` equivalent using binary search over sorted ranges (same
  as C tree-sitter)

```go
// Equivalence class lookup — O(1) for ASCII, O(log N) for Unicode
var asciiClass [128]uint8  // pre-computed at grammar compile time

func charClass(c int32) uint8 {
    if c < 128 {
        return asciiClass[c]
    }
    return unicodeCharClass(c)  // binary search over ranges
}

// State dispatch — flat lookup table replaces ADVANCE_MAP's linear scan
var transitions [stateCount * classCount]uint16

func lexMain(lexer *Lexer, startState StateID) bool {
    state := startState
    for {
        c := lexer.Lookahead
        class := charClass(c)
        next := transitions[uint32(state)*classCount + uint32(class)]
        // Decode action from next: advance, skip, accept, or fail
        // ...
    }
}
```

**Key win over C tree-sitter:** The O(1) table lookup replaces `ADVANCE_MAP`'s O(N)
linear scan. For states with 15-30 transition pairs, this is a direct 15-30x
improvement on the character dispatch step. Even accounting for the table's cache
footprint, this should make the Go baseline **competitive with or faster than C
tree-sitter** for the per-character dispatch portion.

### Phase 1: First Optimizations

*Highest impact, moderate effort. Target: match or beat C tree-sitter on average.*

**1a. Specialized token scanners.** Generate dedicated scanner functions for the
most common token patterns:

```go
// Fast identifier scanner — handles the 60% common case
func scanIdentifier(src []byte, pos int) int {
    // ASCII fast path
    for pos < len(src) {
        c := src[pos]
        if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
           c >= '0' && c <= '9' || c == '_' {
            pos++
            continue
        }
        if c >= 0x80 {
            // Unicode fallback
            r, size := utf8.DecodeRune(src[pos:])
            if unicode.IsLetter(r) || unicode.IsDigit(r) {
                pos += size
                continue
            }
        }
        break
    }
    return pos
}
```

**1b. Keyword perfect hash.** Replace the keyword trie with a compile-time-generated
perfect hash function. Use GNU gperf-style minimal perfect hashing, generated by
our `tsgo-generate` tool:

```go
// Generated for Go's 25 keywords
func keywordLookup(s []byte) Symbol {
    if len(s) < 2 || len(s) > 11 { return 0 }
    h := uint32(s[0]) + uint32(len(s))*37
    h = h % 41  // table size
    entry := keywordTable[h]
    if entry.len == uint8(len(s)) && bytesEqual(s, entry.text) {
        return entry.symbol
    }
    return 0
}
```

**1c. Profile-guided branch ordering.** Instrument the generated lexer on real
codebases (Go standard library, top GitHub repos per language). Reorder character
class checks in each state so the most frequent transitions are checked first. For
the outer state dispatch, order the `switch` cases by frequency.

**1d. SIMD whitespace and string scanning.** Add optional SIMD-accelerated scanning
for the two highest-impact token patterns:

```go
// file: simd_amd64.go
//go:nosplit
func findNonWhitespace(src []byte) int  // returns offset of first non-ws byte
func findStringEnd(src []byte) int      // returns offset of first " or \

// file: simd_amd64.s
// AVX2 implementation using VPCMPEQB + VPMOVMSKB + TZCNT

// file: simd_generic.go (fallback)
func findNonWhitespace(src []byte) int {
    for i, c := range src {
        if c != ' ' && c != '\t' && c != '\n' && c != '\r' { return i }
    }
    return len(src)
}
```

Architecture support:
- **amd64**: AVX2 (VPSHUFB, VPCMPEQB, VPMOVMSKB) via Plan9 assembly or Go 1.26
  `simd/archsimd`
- **arm64**: NEON (TBL, CMEQ, bitmask extraction) via Plan9 assembly
- **Fallback**: Pure Go scalar implementation (always available)

Write SIMD code using the Avo code generator (Go → Plan9 ASM) for amd64, hand-written
Plan9 for arm64. Always provide a pure-Go fallback (unlike simdjson-go).

### Phase 2: Advanced Optimizations

*Deeper wins for specific workloads. Target: consistently beat C tree-sitter.*

**2a. Full SIMD character classification.** Build the nibble-lookup infrastructure
(section 3b) as a general-purpose character classifier. Each lex state gets a pair
of 16-byte lookup tables generated at compile time. The classifier processes 32 bytes
per iteration, producing equivalence class IDs for the entire block.

Use this to accelerate the general DFA inner loop: instead of classifying one
character at a time, classify a block and process transitions in bulk.

**2b. SIMD identifier scanning.** Extend the SIMD infrastructure to scan identifier
runs (`[a-zA-Z0-9_]`) at 32 bytes/iteration. Combined with the nibble classifier,
this also handles number literal scanning and operator scanning.

**2c. Adaptive dense/sparse table representation.** For large grammars (TypeScript,
Zig) where the full transition table exceeds L2 cache:

```go
const denseStateCount = 100  // covers hot states

func transition(state StateID, class uint8) StateID {
    if state < denseStateCount {
        return denseTable[uint32(state)*classCount + uint32(class)]
    }
    // Sparse lookup for cold states
    return sparseTransition(state, class)
}
```

The threshold `denseStateCount` is determined by profiling: include all states that
account for 90% of transitions.

**2d. Inlined whitespace loop.** For languages where whitespace is not significant
(Go, C, Java, Rust), bypass the DFA entirely for whitespace:

```go
// Before entering the DFA, skip whitespace directly
for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t' ||
    src[pos] == '\n' || src[pos] == '\r') {
    if src[pos] == '\n' { row++; col = 0 } else { col++ }
    pos++
}
// Now enter DFA for the actual token
```

This eliminates the overhead of DFA state transitions, token creation, and lexer
start/finish for the most common "token" in most languages.

### Phase 3: Exotic / Experimental

*Marginal gains, high complexity. Only if profiling shows clear need.*

**3a. SIMD keyword matching (Teddy).** Build Teddy fingerprint masks for keyword
detection. After scanning an identifier, check against all keywords in a single
SIMD pass. Only worthwhile if the perfect hash from Phase 1 proves insufficient
(unlikely for <100 keywords).

**3b. Grammar-specific loop unrolling.** For the hottest lex states (identified by
profiling), unroll the state transition loop to process 2-4 characters per iteration.
The code generator emits specialized code for common state sequences (e.g.,
"identifier_start → identifier_continue → identifier_continue → ...").

**3c. Prefetch hints.** After determining the next DFA state, issue a prefetch for
that state's transition table row. Only helps for L2+-resident tables (large grammars).
Go's `runtime.Prefetch` or assembly `PREFETCHT0` instruction.

**3d. Lock-free token stream.** For very large files, lex ahead into a bounded buffer
on a separate goroutine. The parser consumes tokens from the buffer. This overlaps
lexer and parser computation. Requires careful synchronization for tree-sitter's
context-dependent lexing (the parser must communicate valid token sets to the lexer).

---

## 6. Benchmark Design

### What to Measure

**Primary metrics:**
- **Tokens/second**: Total tokens produced divided by wall-clock time. This is the
  end-user-visible metric.
- **Bytes/second**: Input bytes processed divided by wall-clock time. Allows
  comparison across different languages (a Go file produces fewer, longer tokens
  than a JavaScript file with the same byte count).
- **Cycles/byte**: CPU cycles per input byte. The most precise metric, immune to
  clock frequency variation. Measured via `RDTSC` or Go's `testing.B`.

**Secondary metrics:**
- **Time-to-first-token**: Latency to produce the first token. Relevant for
  interactive/incremental use cases.
- **Memory allocations per token**: Tracks GC pressure.
- **L1/L2 cache miss rate**: Via `perf stat` on Linux. Validates cache behavior
  assumptions.
- **Branch misprediction rate**: Via `perf stat`. Validates branch ordering optimizations.

### Real-World Corpus Selection

For each target language, select files from popular GitHub repositories that
represent realistic coding patterns:

| Language | Repos | Files | Total Size |
|----------|-------|-------|------------|
| JavaScript | react, lodash, express, webpack | 50 files | ~2 MB |
| Python | django, requests, flask, numpy | 50 files | ~2 MB |
| Go | kubernetes, docker, prometheus, etcd | 50 files | ~2 MB |
| Rust | servo, ripgrep, tokio, serde | 50 files | ~2 MB |
| C | linux kernel, redis, sqlite, curl | 50 files | ~2 MB |
| TypeScript | angular, vscode, typescript compiler | 50 files | ~2 MB |

Include pathological cases:
- Very large files (>100 KB single file)
- Minified JavaScript (one long line, no whitespace)
- String-heavy files (JSON data, test fixtures with long strings)
- Comment-heavy files (documentation, license headers)
- Unicode-heavy files (internationalized strings, CJK identifiers)

### Statistical Methodology

**Warm cache vs cold cache:**
- **Cold**: Clear CPU caches between runs (`echo 3 > /proc/sys/vm/drop_caches` on
  Linux, or allocate and traverse a large buffer)
- **Warm**: Run the benchmark in a tight loop; report the steady-state throughput
  (excluding the first 3 iterations)
- Report both, as they measure different things (cold = first-parse latency,
  warm = incremental-parse throughput)

**Variance handling:**
- Run each benchmark at least 20 times
- Report median and P95 (not mean — GC pauses create outliers)
- Report the coefficient of variation (CV); if CV > 10%, investigate noise sources
- Pin to a single CPU core (`taskset` on Linux) to reduce scheduling noise
- Disable frequency scaling (`cpupower frequency-set -g performance`)

**Comparison protocol:**
- Run C tree-sitter and Go implementation on the same machine, same input, same
  session
- Alternate runs (C, Go, C, Go, ...) to average out thermal effects
- Report the ratio (Go time / C time) with confidence intervals

### Comparison Harness

```go
// bench_test.go
func BenchmarkLexJavaScript(b *testing.B) {
    corpus := loadCorpus("javascript")
    lang := javascript.Language()
    parser := ts.NewParser()
    parser.SetLanguage(lang)

    b.ResetTimer()
    b.SetBytes(int64(corpus.totalBytes))
    for i := 0; i < b.N; i++ {
        for _, file := range corpus.files {
            tree := parser.ParseString(nil, file.content)
            _ = tree  // force full parse including lexing
        }
    }
}
```

C tree-sitter comparison:
```c
// bench_c.c — linked against tree-sitter and tree-sitter-javascript
void benchmark_lex_javascript(const corpus_t *corpus, int iterations) {
    TSParser *parser = ts_parser_new();
    ts_parser_set_language(parser, tree_sitter_javascript());
    for (int i = 0; i < iterations; i++) {
        for (int f = 0; f < corpus->file_count; f++) {
            TSTree *tree = ts_parser_parse_string(parser, NULL,
                corpus->files[f].content, corpus->files[f].length);
            ts_tree_delete(tree);
        }
    }
    ts_parser_delete(parser);
}
```

### Microbenchmarks

In addition to end-to-end parsing benchmarks, isolate lexer performance:

- **Lex-only benchmark**: Call the lex function repeatedly on a file, measuring
  only tokenization time (no parse table lookups, no tree construction)
- **Character classification**: Time the equivalence class lookup alone
- **Whitespace scanning**: Time whitespace-skip on a whitespace-heavy input
- **Identifier scanning**: Time identifier scanning on an identifier-heavy input
- **Keyword lookup**: Time keyword detection on a list of identifiers/keywords

### Target: Beat C Tree-sitter

Our target is to beat C tree-sitter **on average** across the six target languages
when lexing real-world code. "Beat" means the Go lexer is faster in bytes/second
on the warm-cache benchmark median, measured on the same hardware.

Expected path to this target:

| Phase | Expected Ratio (Go / C) | Cumulative Approach |
|-------|------------------------|---------------------|
| Baseline (switch) | 1.2-1.4x slower | Flat table replaces ADVANCE_MAP |
| Phase 0 (equiv classes) | 0.9-1.1x | O(1) lookup beats O(N) linear scan |
| Phase 1 (fast paths + SIMD ws) | 0.7-0.9x | Specialized scanners + SIMD |
| Phase 2 (full SIMD) | 0.5-0.8x | SIMD classification + identifier scan |

The key insight: C tree-sitter's `ADVANCE_MAP` linear scan is a significant weakness.
Replacing it with O(1) table lookup in Phase 0 already closes most of the gap. The
specialized scanners and SIMD optimizations in Phases 1-2 push us ahead.

---

## 7. Key References

### Papers
- Mytkowicz, Musuvathi, Schulte. "Data-Parallel Finite-State Machines." ASPLOS 2014.
- Langdale, Lemire. "Parsing Gigabytes of JSON per Second." VLDB Journal, 2019. arXiv:1902.08318.
- Wang et al. "Hyperscan: A Fast Multi-pattern Regex Matcher for Modern CPUs." NSDI 2019.
- Hindle et al. "On the Naturalness of Software." ICSE 2012.
- Jiang, Agrawal. "Combining SIMD and Many/Multi-core Parallelism for FSMs." PPoPP 2017.
- Chakraborty et al. "Succinct Representation for (Non)Deterministic Finite Automata." arXiv:1907.09271.
- Kupferman, Mosheiff. "Prime Languages." Theoretical Computer Science, 2015.

### Implementation References
- simdjson: github.com/simdjson/simdjson
- simdjson-go: github.com/minio/simdjson-go
- BurntSushi aho-corasick: github.com/BurntSushi/aho-corasick (DESIGN.md for architecture)
- simd-lexing: github.com/Mateuszd6/simd-lexing
- Avo (Go assembly generator): github.com/mmcloughlin/avo
- re2c (lexer generator with Go backend): re2c.org
- Wazero (Go JIT compilation): github.com/tetratelabs/wazero

### Benchmarks and Analysis
- re2c benchmarks: re2c.org/benchmarks
- Lemire: "How quickly can you remove spaces?" lemire.me/blog/2017/01/20
- nothings.org: "Fast Lexical Analysis" nothings.org/computer/lexing.html
- Mula: "SIMDized byte set membership" 0x80.pl/notesen/2018-10-18-simd-byte-lookup.html
- Go archsimd package (Go 1.26): pkg.go.dev/simd/archsimd

### Tree-sitter Source Code
- Runtime lexer: tree-sitter lib/src/lexer.c, lib/src/lexer.h
- Generated code macros: tree-sitter lib/src/parser.h (START_LEXER, ADVANCE, ADVANCE_MAP, etc.)
- Generated JavaScript parser: tree-sitter-javascript src/parser.c (94K lines, 279 lex states)
