# Corpus Test Failure Analysis

Updated: 2026-02-16 (post starvation fix e5de4de)

## Current State: 1519/1619 passing (93.9%)

Key fixes applied (cumulative):
- Alias sequence extraction (9e978d1): resolved ~167 alias-related failures
- Trailing newline preservation (d312604): resolved ~38 failures
- Dynamic precedence handling (65c6252): Go 55→57, C++ +27
- Alias-aware visibility (044e755): +95 tests across 10 languages
- Comment extras placement (b5dc3bc): +103 tests across 12 languages
- Scanner result variable / END_STATE (9bdafd3): +37 tests, CSS→100%, Rust→99.3%
- GLR version pruning / ums (e9aeaa3): +6 net (9 improvements, 1 regression)
- Starvation fix (e5de4de): +2 (round-robin findActiveVersion, same-position kills)

The corpus test runner strips field annotations, so field mismatches are not counted.

## Per-Language Results

| Language    | Total | Pass | Fail | Pass Rate |
|-------------|------:|-----:|-----:|----------:|
| JSON        |     6 |    6 |    0 |   100.0%  |
| CSS         |    38 |   38 |    0 |   100.0%  |
| Rust        |   147 |  146 |    1 |    99.3%  |
| TypeScript  |   112 |  110 |    2 |    98.2%  |
| Ruby        |   290 |  283 |    7 |    97.6%  |
| Lua         |    37 |   36 |    1 |    97.3%  |
| Java        |   108 |  105 |    3 |    97.2%  |
| JavaScript  |   116 |  112 |    4 |    96.6%  |
| Bash        |   100 |   96 |    4 |    96.0%  |
| Python      |   115 |  105 |   10 |    91.3%  |
| C           |    85 |   78 |    7 |    91.8%  |
| C++         |   179 |  160 |   19 |    89.4%  |
| Go          |    67 |   59 |    8 |    88.1%  |
| Perl        |   199 |  172 |   27 |    86.4%  |
| HTML        |    20 |   13 |    7 |    65.0%  |

## Remaining Failures (100 total)

All comment placement, alias visibility, trailing extras, and scanner END_STATE
issues have been resolved. The remaining 100 failures break down into four categories:

| Category                          | Count | % of 100 |
|-----------------------------------|------:|---------:|
| Structural mismatch               |    68 |    68.0% |
| Empty/nil parse tree              |    19 |    19.0% |
| Internal name leaking             |    11 |    11.0% |
| Timeout/infinite loop             |     2 |     2.0% |

Comment misplacement is **zero**. The scanner result variable fix (9bdafd3)
resolved 37 failures caused by codegen generating `return false` for
non-accepting lex states even when a prior state had accepted.

### Per-Language Failure Breakdown

| Language | Failures | Empty | Internal | Structural | Timeout |
|----------|----------|-------|----------|------------|---------|
| Perl     |       27 |     2 |        8 |         17 |       0 |
| C++      |       19 |     0 |        0 |         17 |       2 |
| Python   |       10 |     1 |        1 |          8 |       0 |
| Go       |        8 |     2 |        0 |          6 |       0 |
| Ruby     |        7 |     3 |        2 |          2 |       0 |
| C        |        7 |     1 |        0 |          6 |       0 |
| HTML     |        7 |     5 |        0 |          2 |       0 |
| Bash     |        4 |     1 |        0 |          3 |       0 |
| JS       |        4 |     2 |        0 |          2 |       0 |
| Java     |        3 |     1 |        0 |          2 |       0 |
| TS       |        2 |     1 |        0 |          1 |       0 |
| Rust     |        1 |     0 |        0 |          1 |       0 |
| Lua      |        1 |     0 |        0 |          1 |       0 |
| **Total**| **100** | **19**|     **11**|     **68**|    **2**|

## Failure Category Details

### Category 1: Empty/Nil Parse Tree (19)

The parser returns no output or a wrong root type.

**Blank output (15)**: Parser produced nothing. Includes:
- HTML (2): DT/DL elements, Ruby annotation elements without close tags
- Ruby (3): basic_heredocs, heredocs_with_in_args, newline-delimited_strings
- JS (2): Alphabetical_infix_operators_split_across_lines, Extra_complex_literals
- Go (2): Error_detected_at_globally_reserved_keyword, String_literals
- Perl (2): Double_dollar_edge_cases, range_ops (both expect ERROR nodes)
- C (1): Typedefs
- Java (1): switch_with_unnamed_pattern_variable
- Python (1): An_error_before_a_string_literal (expects ERROR)
- TS (1): Type_arguments_in_JSX

**Wrong/garbage root type (4)**: Parser returned a token fragment:
- Bash: File_redirects → `heredoc_redirect_token1`
- HTML (3): LI/P/TR elements → `>` (scanner not emitting implicit end tags)

### Category 2: Internal Name Leaking (12)

Internal `_`-prefixed rule names appear as the root parse result instead of
the proper `source_file`/`module`/`program` root. This is the
**NonTerminalAliasMap** issue (bead qkj).

**Perl (8)**: Scanner-based string/heredoc internal nodes:
- `_q_string_content`: ''_strings, qw()_lists
- `_qq_string_content`: ""_strings, Interpolation_in_""_strings,
  Array/Hash_element_interpolation, Space_skips_interpolation
- `_heredoc_delimiter`: Indented_heredocs

**Ruby (3)**: Scanner-based heredoc/string internal nodes:
- `_heredoc_body_start`: heredocs_in_context, heredocs_with_interpolation
- `_line_break`: nested_strings_with_different_delimiters

**Python (1)**: Indent/dedent internal node:
- `_dedent`: Function_definitions

### Category 3: Structural Mismatch (69)

Parser produces a tree but structure differs from expected.

**Perl ambiguous function calls (5+)**: `function_call_expression` vs
`ambiguous_function_call_expression` disambiguation. Perl-specific issue.

**C/C++ type_identifier confusion (6)**: `type_identifier` where `identifier`
expected. C: Common_constants, Identifiers, Primitive-typed_variable_declarations,
Type_modifiers. C++ inherits same issues.

**Python keyword/print issues (4)**: Python 2 `print_statement` vs Python 3
`call` confusion, `tuple` vs `parenthesized_expression`.

**Go type_conversion_expression (1)**: `type_conversion_expression` vs
`call_expression` in Type_switch_statements.

**Remaining structural (53)**: Various GLR resolution, precedence handling,
and ambiguity resolution differences across all languages. The long tail of
parser correctness issues.

### Category 4: Timeout/Infinite Loop (8)

All **C++ only**, all at exactly 10.00s (test timeout). Parser gets stuck in
GLR ambiguity resolution: casts_vs_multiplications, Noreturn_Type_Qualifier,
For_loops, Switch_statements, Concept_definition,
Compound_literals_without_parentheses, Template_calls, Parameter_pack_expansions.

Likely related to `ts_parser__compare_versions` gap (bead ums/wcu.18).

## Remaining High-Impact Fix Priorities

### 1. NonTerminalAliasMap emission (bead qkj, P1)

Would fix 12 internal-name failures where `_`-prefixed rule names appear as
the root parse result instead of `source_file`/`module`. Directly affects
Perl (8), Ruby (3), Python (1). May also unblock some of the 19 empty-tree
failures. Estimated impact: +12-20 tests.

### 2. C++ GLR timeout resolution (bead ums, P2)

8 C++ tests timeout in GLR ambiguity resolution. Porting
`ts_parser__compare_versions` from C reference would fix version pruning
and allow C++ to reach ~96%. Estimated impact: +8 tests.

### 3. HTML implicit close tags (~5 tests)

External scanner needs work on HTML5 optional-close rules. 5 of 7 remaining
HTML failures are empty trees from unhandled implicit close tags (DT/DL,
Ruby annotations, LI, P, TR/TD/TH). Estimated impact: +5 tests.

### 4. C/C++ type_identifier confusion (~6 tests)

Keyword extraction or symbol resolution issue causing `type_identifier` to
appear where `identifier` is expected. Affects Common_constants, Identifiers,
Primitive-typed_variable_declarations, Type_modifiers in both C and C++.

### 5. Perl function call disambiguation (~5 tests)

`function_call_expression` vs `ambiguous_function_call_expression` resolution.
Perl-specific GLR ambiguity issue.

### 6. Go type_conversion_expression (bead nlb, P2)

`Type(expr)` ambiguity resolved incorrectly in some contexts.

---

## Per-Language Breakdown (Post 9bdafd3)

### C (85 tests: 78 pass, 7 fail)

- **type_identifier confusion (4)**: Common_constants, Identifiers,
  Primitive-typed_variable_declarations, Type_modifiers
- **Empty parse (1)**: Typedefs
- **Other structural (2)**: Call_expressions_vs_empty_declarations,
  Comments_after_for_loops

### C++ (179 tests: 154 pass, 25 fail)

- **Timeout (8)**: casts_vs_multiplications, Noreturn_Type_Qualifier,
  For_loops, Switch_statements, Concept_definition,
  Compound_literals_without_parentheses, Template_calls,
  Parameter_pack_expansions
- **Structural (17)**: type_identifier confusion, template parsing,
  structured binding, declaration vs expression ambiguity

### Rust (147 tests: 146 pass, 1 fail)

- **Structural (1)**: Minor difference

### Bash (100 tests: 96 pass, 4 fail)

- **Empty parse (1)**: Command_substitutions
- **Structural (3)**: File_redirects (wrong root), other redirections

### Ruby (290 tests: 282 pass, 8 fail)

- **Empty parse (3)**: basic_heredocs, heredocs_with_in_args, newline_strings
- **Internal names leaking (3)**: heredocs_with_interpolation, nested_strings,
  heredocs_in_context_starting_with_dot
- **Structural (2)**: Various

### Perl (199 tests: 172 pass, 27 fail)

- **Internal names leaking (8)**: `_qq_string_content`, `_q_string_content`,
  `_heredoc_delimiter` in string/heredoc tests
- **Structural (17)**: Signatures, ambiguous function call resolution,
  stub_expression differences
- **Empty parse (2)**: Double_dollar_edge_cases, range_ops

### CSS (38 tests: 38 pass, 0 fail) -- 100%

### HTML (20 tests: 13 pass, 7 fail)

- **Empty parse (5)**: DT/DL, Ruby annotations, LI, P, TR/TD/TH elements
  (implicit close tags not handled)
- **Structural (2)**: COLGROUP, comment

### Java (108 tests: 105 pass, 3 fail)

- **Empty parse (1)**: switch_with_unnamed_pattern_variable
- **Structural (2)**: Minor differences

### Go (67 tests: 59 pass, 8 fail)

- **type_identifier confusion (2)**: For_statements, Select_statements
- **type_conversion_expression (1)**: Type_switch_statements
- **Empty parse (2)**: Error_at_reserved_keyword, String_literals
- **Other structural (3)**: Various

### Python (115 tests: 104 pass, 11 fail)

- **Structural (9)**: print_statement/call confusion, match/case, patterns
- **Internal name (1)**: _dedent in Function_definitions
- **Empty parse (1)**: An_error_before_a_string_literal

### JavaScript (116 tests: 112 pass, 4 fail)

- **Empty parse (2)**: Alphabetical_infix_operators, Extra_complex_literals
- **Structural (2)**: Minor

### TypeScript (112 tests: 110 pass, 2 fail)

- **Structural (1)**: Minor
- **Empty parse (1)**: Type_arguments_in_JSX

### Lua (37 tests: 36 pass, 1 fail)

- **Structural (1)**: Block comment parsing (comment count mismatch)

---

## Fix History and Impact

| Fix                                     | Tests Recovered | Overall Rate |
|-----------------------------------------|----------------:|-------------:|
| Initial baseline (alias seq 9e978d1)    |          ~167   |        76.5% |
| + Trailing newline (d312604)            |           ~38   |        78.8% |
| + Dynamic precedence (65c6252)          |           ~10   |        79.4% |
| + Alias visibility (044e755)            |           ~95   |        84.7% |
| + Comment extras (b5dc3bc)              |          ~103   |        91.0% |
| + Scanner result variable (9bdafd3)     |           ~37   |        93.3% |
| + GLR version pruning (e9aeaa3)         |            ~+6  |        93.7% |
| + Starvation fix (e5de4de)              |             +2  |        93.9% |
| **Current**                             |               — |    **93.9%** |

## UMS + Starvation Fix Details

**GLR version pruning (e9aeaa3)**: Ported ts_parser__compare_versions with
cost amplification and error-state decisive kills. +6 net improvement.

**Starvation fix (e5de4de)**: Two-part fix for ums regression:
1. Same-position restriction on Phase 2 decisive kills (prevents premature
   cross-position kills of error recovery versions)
2. Round-robin findActiveVersion starvation detection (after 4 stale selections
   of same version without position progress, rotates to next active version)

Result: Complex_fold_expression regression resolved, template_functions_vs_relational
restored to structural (not timeout). C++ timeouts reduced from 8 to 2.

## Remaining High-Impact Fixes

| Fix                                     | Est. Tests | Notes |
|-----------------------------------------|-----------:|-------|
| wcu.19 (Perl/Ruby wrong-root)           |     ~11    | Parser/scanner interaction |
| Soft preference GLR behavior            |    ~20-30  | Blocked by Python regression |
| HTML implicit close tags                |       ~5   | Scanner work needed |
| C/C++ type_identifier confusion         |       ~6   | Keyword extraction issue |

## Beads Tracking Remaining Fixes

- **tree-sitter-go-ot2** (P1): Post-UMS regression — RESOLVED (e5de4de)
- **tree-sitter-go-wcu.19** (P2): Perl/Ruby wrong-root production investigation
- **tree-sitter-go-wcu.18** (P3): Merge link ordering to match C reference
- **tree-sitter-go-nlb** (P2): Call vs type_conversion_expression ambiguity

## Closed Beads (Fixes Merged)

- **tree-sitter-go-dhw**: Alias sequence extraction (9e978d1)
- **tree-sitter-go-vuv**: Comment extras placement (b5dc3bc)
- **tree-sitter-go-4cw**: Alias-aware visibility (044e755)
- **tree-sitter-go-w3k**: Negated range lex (closed — not needed)
- **tree-sitter-go-qkj**: NonTerminalAliasMap emission (d50fe7a)
- **tree-sitter-go-ums**: GLR version pruning (e9aeaa3)
