# Corpus Test Failure Analysis

Updated: 2026-02-16 (post all P1 fixes through ef86aca)

## Current State: 1474/1619 passing (91.0%)

Key fixes applied (cumulative):
- Alias sequence extraction (9e978d1): resolved ~167 alias-related failures
- Trailing newline preservation (d312604): resolved ~38 failures
- Dynamic precedence handling (65c6252): Go 55→57, C++ +27
- Alias-aware visibility (044e755): +95 tests across 10 languages
- Comment extras placement (b5dc3bc): +103 tests across 12 languages

The corpus test runner strips field annotations, so field mismatches are not counted.

## Per-Language Results

| Language    | Total | Pass | Fail | Pass Rate |
|-------------|------:|-----:|-----:|----------:|
| JSON        |     6 |    6 |    0 |   100.0%  |
| TypeScript  |   112 |  110 |    2 |    98.2%  |
| Lua         |    37 |   36 |    1 |    97.3%  |
| Ruby        |   290 |  279 |   11 |    96.2%  |
| Bash        |   100 |   95 |    5 |    95.0%  |
| Java        |   108 |  101 |    7 |    93.5%  |
| JavaScript  |   116 |  108 |    8 |    93.1%  |
| Rust        |   147 |  134 |   13 |    91.2%  |
| C           |    85 |   77 |    8 |    90.6%  |
| CSS         |    38 |   34 |    4 |    89.5%  |
| Python      |   115 |  103 |   12 |    89.6%  |
| Go          |    67 |   59 |    8 |    88.1%  |
| Perl        |   199 |  171 |   28 |    85.9%  |
| C++         |   179 |  153 |   26 |    85.5%  |
| HTML        |    20 |    8 |   12 |    40.0%  |

## Remaining Failures (145 total)

Most comment, alias visibility, and trailing extras issues have been resolved.
Remaining failures are primarily structural/parse differences, scanner gaps,
and language-specific edge cases.

| Language | Remaining Failures |
|----------|-------------------:|
| Perl     |                 28 |
| C++      |                 26 |
| Rust     |                 13 |
| HTML     |                 12 |
| Python   |                 12 |
| C        |                  8 |
| Go       |                  8 |
| JS       |                  8 |
| Java     |                  7 |
| Bash     |                  5 |
| CSS      |                  4 |
| TS       |                  2 |
| Lua      |                  1 |
| JSON     |                  0 |

Next highest-impact fix: negated range extraction in lex DFA (bead w3k) —
500 unhandled lex patterns across 14 grammars, expected to improve Bash,
Ruby, TypeScript, and others.

## High-Impact Fix Priorities

### 1. Perl string_content / regexp_content (~57 tests → Perl 58%→86%)

Perl's dominant failure mode: 41 tests missing `string_content` inside string
literals, 7 missing `regexp_content`, 7 have internal alias names leaking
(`_qq_string_content`, `_q_string_content`). The external scanner emits token
types for content scanning, but the content wrapper node is not generated.
Likely a single alias resolution or scanner token mapping issue.

### 2. Go remaining structural issues (~12 tests, Go now 82%)

The trailing newline fix (d312604) resolved most grouped declaration failures.
Remaining Go failures include: type_conversion_expression disambiguation,
block comment handling, and a few structural misparses. Go jumped from 37%→82%.

### 3. Comment placement / extras attachment (~56 tests across 10 languages)

Comments consistently attach to the wrong parent node. Expected: comment as
sibling of statements. Actual: comment nested inside preceding or following
statement. Affects C (11), C++ (15), JS (8), Python (6), TS (5), Bash (3),
Ruby (2), CSS (2), Java (3), Perl (2). Likely a single parser-level fix in
how extras are assigned during reduce operations.

### 4. Alias-based child visibility (~67 tests → Perl 58%→86%, Lua 70%→97%)

Bead: tree-sitter-go-4cw. The runtime never consults alias sequences when
determining child visibility. Hidden symbols aliased to visible names (e.g.,
`_doublequote_string_content` → `string_content`) are treated as hidden and
skipped. Fix needed at 4 locations in subtree.go, tree.go, tree_cursor.go:
resolve alias from parent production ID + structural child index before
checking visibility.

### 5. Lua string_content child (~10 tests → Lua 70%→97%)

Same class as Perl: `(string (string_content))` expected but `(string)`
produced without content child. Short strings like `'a'` produce bare
`(string)`.

### 6. Rust doc/block comments (~8 tests)

`doc_comment`, `line_comment`, `block_comment` and marker children missing
from parse output. May be alias resolution specific to Rust's comment grammar.

### 7. Go type_conversion_expression (~5 tests)

`Type(expr)` ambiguity resolved incorrectly: `a(b)` parsed as
`type_conversion_expression` instead of `call_expression` in some contexts.

### 8. HTML implicit close tags (~10 tests)

The scanner's `scanImplicitEndTag` doesn't handle many HTML5 optional-close
rules (LI, DT, DD, P, Ruby annotations, TR/TD/TH, entities).

### 9. Ruby call/bare_string/bare_symbol aliases (~17 tests)

Three alias types not resolving: `call` (7 tests, method calls with `::`,
`&.`, etc.), `bare_string` (5 tests, `%w`/`%W` word arrays), `bare_symbol`
(5 tests, `%i`/`%I` symbol arrays).

### 10. C/C++ escaped newline comments (~4 tests)

Comments with `\` continuation lines not recognized as continuing the comment.

---

## Per-Language Breakdown

### C (85 tests: 63 pass, 22 fail)

- **Comment placement (11)**: Comments attach to wrong parent
- **Empty parse (3)**: Complex expressions produce empty output
- **Preprocessor (2)**: Multi-line macro definitions fail
- **String content (1)**: Missing content node
- **Structural (5)**: Declaration/expression ambiguity, type parsing

### C++ (179 tests: 123 pass, 56 fail)

- **Comment placement (15)**: Same as C
- **Empty parse (10)**: Templates, casts, for-loops, concepts
- **Template parsing (7)**: Angle brackets parsed as relational operators
- **Structured binding (3)**: `a[b]` parsed as structured_binding_declarator
- **Preprocessor (2)**: Same as C
- **String content (1)**: Missing content node
- **Other structural (18)**: Declaration vs expression ambiguity, subtle nesting

### Rust (147 tests: 121 pass, 26 fail)

- **Doc/block comments (11)**: Comment node types not emitted
- **Empty parse (11)**: Attribute macros, impls with where clauses, GATs
- **Structural (4)**: Loop/for expression differences

### Bash (100 tests: 89 pass, 11 fail)

- **Comment placement (3)**: Comments in wrong position
- **Empty parse (2)**: Complex constructs fail
- **Structural (6)**: Various issues with redirections, expansions

### Ruby (290 tests: 240 pass, 50 fail)

- **Alias mismatch (17)**: `call`, `bare_string`, `bare_symbol` aliases
- **Empty parse (15)**: Block comments, heredocs in complex contexts
- **Structural (16)**: Heredoc handling (7), method call differences
- **Comment placement (2)**: Minor

### Perl (199 tests: 115 pass, 84 fail)

- **String/regexp content (57)**: Dominant failure — content wrapper nodes missing
- **Alias leaking (9)**: Internal alias names in output
- **Empty parse (4)**: `data_section`, heredocs
- **Structural (12)**: Signatures, ERROR node differences
- **Comment placement (2)**: Minor

### CSS (38 tests: 32 pass, 6 fail)

- **Empty parse (3)**: Complex selectors or at-rules
- **Comment placement (2)**: Comments in wrong position
- **Structural (1)**: Minor

### HTML (20 tests: 7 pass, 13 fail)

- **Empty parse (10)**: Implicit close tags not handled for many elements
- **Structural (2)**: Entity references, void elements
- **Comment (1)**: Missing comment node

### Java (108 tests: 97 pass, 11 fail)

- **Empty parse (4)**: Complex generic/annotation constructs
- **Comment placement (3)**: Comments in wrong position
- **Structural (4)**: Minor differences

### Go (67 tests: 55 pass, 12 fail)

- **type_conversion_expression (5)**: Call/type conversion ambiguity
- **Structural (4)**: Various parsing differences in complex expressions
- **Comment (1)**: Block comment dropped
- **Empty parse (2)**: Complex constructs fail

### Python (115 tests: 92 pass, 23 fail)

- **Structural (15)**: print_statement, match/case parsing, pattern differences
- **Comment placement (6)**: Comment positioning after dedents
- **Alias (1)**: as_pattern_target
- **String (1)**: Format string handling

### JavaScript (116 tests: 98 pass, 18 fail)

- **Comment placement (8)**: Comments in wrong position
- **Empty parse (7)**: Classes, decorators, private fields
- **Structural (3)**: Async/await handling, JSX edge cases

### TypeScript (112 tests: 104 pass, 8 fail)

- **Comment placement (5)**: Comments in wrong position
- **Structural (2)**: Readonly arrays, arrow function with async parameter
- **Empty parse (1)**: JSX with type arguments

### Lua (37 tests: 26 pass, 11 fail)

- **String content (10)**: Missing `string_content` child node
- **Structural (1)**: Block comment parsing

---

## Fix History and Impact

| Fix                                     | Tests Recovered | Overall Rate |
|-----------------------------------------|----------------:|-------------:|
| Initial baseline (alias seq 9e978d1)    |          ~167   |        76.5% |
| + Trailing newline (d312604)            |           ~38   |        78.8% |
| + Dynamic precedence (65c6252)          |           ~10   |        79.4% |
| + Alias visibility (044e755)            |           ~95   |        84.7% |
| + Comment extras (b5dc3bc)              |          ~103   |        91.0% |
| **Current**                             |               — |    **91.0%** |

## Remaining High-Impact Fixes

| Fix                                     | Est. Tests | New Overall Rate |
|-----------------------------------------|-----------:|-----------------:|
| Negated range lex extraction (w3k)      |     ~20-40 |          ~92-94% |
| NonTerminalAliasMap emission (qkj)      |      ~5-15 |          ~93-95% |
| HTML implicit close tags                |       ~10  |          ~94-96% |

## Beads Tracking Remaining Fixes

- **tree-sitter-go-w3k** (P1): Fix negated range extraction in lex DFA parser — in progress
- **tree-sitter-go-qkj** (P2): Emit NonTerminalAliasMap in generated language.go
- **tree-sitter-go-ums** (P2): Port ts_parser__compare_versions for GLR correctness
- **tree-sitter-go-nlb** (P2): Call vs type_conversion_expression ambiguity
- **tree-sitter-go-wcu.18** (P3): Merge link ordering to match C reference

## Closed Beads (Fixes Merged)

- **tree-sitter-go-dhw**: Alias sequence extraction (9e978d1)
- **tree-sitter-go-vuv**: Comment extras placement (b5dc3bc)
- **tree-sitter-go-4cw**: Alias-aware visibility (044e755)
