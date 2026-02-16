# Corpus Test Failure Analysis

Updated: 2026-02-16 (post alias fix 9e978d1 + trailing newline fix d312604)

## Current State: 1276/1619 passing (78.8%)

Key fixes applied:
- Alias sequence extraction (9e978d1): resolved ~167 alias-related failures
- Trailing newline preservation (d312604): resolved ~38 failures, especially Go grouped decls

The corpus test runner strips field annotations, so field mismatches are not counted.

## Per-Language Results

| Language    | Total | Pass | Fail | Pass Rate |
|-------------|------:|-----:|-----:|----------:|
| JSON        |     6 |    6 |    0 |   100.0%  |
| TypeScript  |   112 |  104 |    8 |    92.9%  |
| Java        |   108 |   97 |   11 |    89.8%  |
| Bash        |   100 |   89 |   11 |    89.0%  |
| JavaScript  |   116 |   98 |   18 |    84.5%  |
| CSS         |    38 |   32 |    6 |    84.2%  |
| Rust        |   147 |  122 |   25 |    83.0%  |
| Ruby        |   290 |  241 |   49 |    83.1%  |
| Go          |    67 |   55 |   12 |    82.1%  |
| Python      |   115 |   92 |   23 |    80.0%  |
| C           |    85 |   66 |   19 |    77.6%  |
| Lua         |    37 |   26 |   11 |    70.3%  |
| C++         |   179 |  126 |   53 |    70.4%  |
| Perl        |   199 |  115 |   84 |    57.8%  |
| HTML        |    20 |    7 |   13 |    35.0%  |

## Failure Categories (343 total)

| Category                      | Count | % of Failures |
|-------------------------------|------:|--------------:|
| Other structural              |  ~100 |        ~29.2% |
| Comment placement             |   ~60 |        ~17.5% |
| String/regexp content missing |   ~67 |        ~19.5% |
| Empty/truncated parse         |   ~65 |        ~19.0% |
| Alias/node name mismatch     |   ~27 |         ~7.9% |
| Preprocessor                  |    ~4 |         ~1.2% |

Note: The trailing newline fix (d312604) resolved ~30 Go empty-parse failures
(grouped declarations) and ~8 other failures across languages.

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

## Projected Impact of Top Fixes

| Fix                                     | Tests Recovered | New Overall Rate |
|-----------------------------------------|----------------:|-----------------:|
| Current baseline (post d312604)         |               — |            78.8% |
| + Alias child visibility (4cw)          |           ~67   |            82.9% |
| + Trailing extras in doReduce (vuv)     |           ~60   |            86.7% |
| + Negated range lex extraction (w3k)    |         ~30-60  |          ~90-93% |
| + Ruby call/bare_string aliases         |           ~17   |          ~91-94% |
| + Rust doc comments                     |            ~8   |          ~92-95% |
| + HTML implicit close tags              |           ~10   |          ~92-95% |
| P1 fixes only (vuv+4cw+w3k)            |       ~160-190  |          ~91-93% |

## Beads Tracking These Fixes

- **tree-sitter-go-vuv** (P1): Fix trailing extras stripping in doReduce
- **tree-sitter-go-4cw** (P1): Fix alias-based child visibility in tree traversal
- **tree-sitter-go-w3k** (P1): Fix negated range extraction in lex DFA parser
