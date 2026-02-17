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

All comment placement, alias visibility, and trailing extras issues have been
resolved. The remaining 145 failures break down into four categories:

| Category                          | Count | % of 145 |
|-----------------------------------|------:|---------:|
| Structural mismatch               |    85 |    58.6% |
| Empty/nil parse tree              |    50 |    34.5% |
| Timeout/infinite loop             |     8 |     5.5% |
| Missing alias content             |     2 |     1.4% |

Comment misplacement is now **zero** — the b5dc3bc fix resolved all of them.

### Per-Language Failure Breakdown

| Language | Failures | Empty | Structural | Timeout | Missing Alias |
|----------|----------|-------|------------|---------|---------------|
| Perl     |       28 |     4 |         24 |       0 |             0 |
| C++      |       26 |     1 |         17 |       8 |             0 |
| Rust     |       13 |    11 |          2 |       0 |             0 |
| HTML     |       12 |    10 |          2 |       0 |             0 |
| Python   |       12 |     1 |         10 |       0 |             1 |
| Ruby     |       11 |     5 |          6 |       0 |             0 |
| C        |        8 |     2 |          6 |       0 |             0 |
| Go       |        8 |     2 |          6 |       0 |             0 |
| JS       |        8 |     6 |          2 |       0 |             0 |
| Java     |        7 |     4 |          3 |       0 |             0 |
| Bash     |        5 |     1 |          3 |       0 |             1 |
| CSS      |        4 |     3 |          1 |       0 |             0 |
| TS       |        2 |     1 |          1 |       0 |             0 |
| Lua      |        1 |     0 |          1 |       0 |             0 |
| **Total**| **145** | **50**|      **85**|    **8**|         **2** |

## Failure Category Details

### Category 1: Empty/Nil Parse Tree (50)

The parser returns no output at all. Sub-patterns:

**HTML implicit close tags (10)**: All 10 remaining HTML failures produce empty
trees. The external scanner's `scanImplicitEndTag` doesn't handle many HTML5
optional-close rules. Tests: Nested_tags, Custom_tags, LI/DT/P/Ruby/TR/TD
elements without close tags, named/numeric/multiple entities.

**String content/alias nodes (8)**: Expected trees contain alias nodes like
`string_content`, `string_fragment`, `template_expression` that require the
NonTerminalAliasMap to be emitted. Affects Go (String_literals), Java
(string_interpolation, text_block), JavaScript (Class_Decorators, Classes,
Extra_complex_literals, JSX_entities), Ruby (newline-delimited_strings).

**External scanner patterns (4)**: Tests involving `command_substitution`
(Bash), `token_tree`/attribute macros (Rust) that depend on external scanner
features.

**Ruby heredocs (2)**: basic_heredocs, heredocs_with_in_args.

**Other empty (26)**: Various root causes including multi-example corpus tests
where parser fails on earlier example, missing grammar features (Rust generics,
Perl edge cases), external scanner dependencies. Spans C, C++, CSS, Go, Java,
JS, Perl, Ruby, Rust, TS.

### Category 2: Structural Mismatch (85)

Parser produces output but tree structure differs from expected.

**Internal/underscore node names leaking (12)**: Internal rule names like
`_qq_string_content`, `_q_string_content`, `_heredoc_delimiter`,
`_heredoc_body_start`, `_line_break`, `_dedent` appear where the grammar
expects them aliased to public names. This is the **NonTerminalAliasMap** issue
(bead qkj). Affects Perl (8), Python (1), Ruby (3).

**type_identifier confusion (10)**: `type_identifier` appears where expected
tree has `identifier` or different type. Suggests keyword/identifier extraction
misclassification. Affects C (4: Common_constants, Identifiers,
Primitive-typed_variable_declarations, Type_modifiers), C++ (4: same tests),
Go (2: For_statements, Select_statements).

**Python keyword/print issues (4)**: Python 2 `print_statement` vs Python 3
`call` confusion, `tuple` vs `parenthesized_expression` classification.

**Go type_conversion_expression (1)**: `type_conversion_expression` vs
`call_expression` ambiguity in Type_switch_statements.

**Remaining structural (58)**: Various GLR resolution, precedence handling,
and ambiguity resolution differences across all languages. This is the long
tail of parser correctness.

### Category 3: Timeout/Infinite Loop (8)

All **C++ only**, all at exactly 10.00s (test timeout). Parser gets stuck in
GLR ambiguity resolution: casts_vs_multiplications, Noreturn_Type_Qualifier,
For_loops, Switch_statements, Concept_definition,
Compound_literals_without_parentheses, Template_calls, Parameter_pack_expansions.

Likely related to `ts_parser__compare_versions` gap (bead ums/wcu.18).

### Category 4: Missing Alias Content (2)

Expected nodes present in expected tree but absent from actual:
Bash/File_redirects (string_content), Python/An_error_before_a_string_literal
(string_content).

## Remaining High-Impact Fix Priorities

### 1. NonTerminalAliasMap emission (bead qkj, P1)

Would fix 12 internal-name structural mismatches directly and likely contribute
to fixing many of the 50 empty-tree cases where alias nodes are expected.
Estimated impact: +15-25 tests.

Note: Originally predicted Perl +52 and Lua +10, but the comment extras fix
(b5dc3bc) already resolved most of those. The qkj fix addresses a different
subset — the 12 `_` prefixed internal names leaking through.

### 2. Negated range lex extraction (bead w3k, P1)

259+ unhandled `(lookahead < X || Y < lookahead)` patterns across 14 grammars.
These cause incorrect lex transitions, producing wrong tokens or empty parses.
Estimated impact: +20-40 tests.

### 3. HTML implicit close tags (~10 tests)

External scanner needs work on HTML5 optional-close rules. 10 of 12 HTML
failures are empty trees due to this.

### 4. C++ GLR timeout resolution (bead ums, P2)

8 C++ tests timeout in GLR ambiguity resolution. Porting
`ts_parser__compare_versions` from C reference would fix the version pruning
logic.

### 5. type_identifier confusion (10 tests across C, C++, Go)

Keyword extraction or symbol resolution issue causing `type_identifier` to
appear where `identifier` is expected. Needs investigation.

### 6. Go type_conversion_expression (bead nlb, P2)

`Type(expr)` ambiguity resolved incorrectly in some contexts.

---

## Per-Language Breakdown (Post ef86aca)

### C (85 tests: 77 pass, 8 fail)

- **type_identifier confusion (4)**: Common_constants, Identifiers,
  Primitive-typed_variable_declarations, Type_modifiers
- **Empty parse (2)**: Object-like_macro_definitions, Typedefs
- **Other structural (2)**: Call_expressions_vs_empty_declarations,
  Comments_after_for_loops

### C++ (179 tests: 153 pass, 26 fail)

- **Timeout (8)**: casts_vs_multiplications, Noreturn_Type_Qualifier,
  For_loops, Switch_statements, Concept_definition,
  Compound_literals_without_parentheses, Template_calls,
  Parameter_pack_expansions
- **type_identifier confusion (4)**: Same tests as C
- **Empty parse (1)**: Object-like_macro_definitions
- **Other structural (13)**: Template parsing, structured binding, declaration
  vs expression ambiguity

### Rust (147 tests: 134 pass, 13 fail)

- **Empty parse (11)**: Attribute_macros, Immediate_inner_attribute,
  Macro_invocation, For_expressions, Function_types, GATs, Generic_types,
  Inherent_Impls, Loop_expressions, Trait_impls, Where_clauses
- **Structural (2)**: Minor differences

### Bash (100 tests: 95 pass, 5 fail)

- **Empty parse (1)**: Command_substitutions
- **Missing alias (1)**: File_redirects (string_content)
- **Structural (3)**: Various redirections/expansions

### Ruby (290 tests: 279 pass, 11 fail)

- **Empty parse (5)**: basic_heredocs, heredocs_with_in_args, newline_strings,
  calls_on_negated_literals, minus_call_exponential_range
- **Internal names leaking (3)**: heredocs_with_interpolation, nested_strings,
  heredocs_in_context_starting_with_dot
- **Other structural (3)**: Various

### Perl (199 tests: 171 pass, 28 fail)

- **Internal names leaking (8)**: `_qq_string_content`, `_q_string_content`
  in ""_strings, ''_strings, Array/Hash_element_interpolation,
  Indented_heredocs, Interpolation, Space_skips, qw()_lists
- **Structural (16)**: Signatures, ambiguous function call resolution,
  stub_expression differences
- **Empty parse (4)**: Double_dollar_edge_cases, Labels, autoquote_edge_cases,
  range_ops

### CSS (38 tests: 34 pass, 4 fail)

- **Empty parse (3)**: Numbers, Binary_arithmetic, Comments_after_numbers
- **Structural (1)**: Media statements

### HTML (20 tests: 8 pass, 12 fail)

- **Empty parse (10)**: Implicit close tags not handled for LI, DT, P, Ruby,
  TR/TD/TH elements, entities
- **Structural (2)**: COLGROUP, comment

### Java (108 tests: 101 pass, 7 fail)

- **Empty parse (4)**: string_interpolation, text_block,
  record_declaration_inside_class, switch_with_unnamed_pattern
- **Structural (3)**: Minor differences

### Go (67 tests: 59 pass, 8 fail)

- **type_identifier confusion (2)**: For_statements, Select_statements
- **type_conversion_expression (1)**: Type_switch_statements
- **Empty parse (2)**: Error_at_reserved_keyword, String_literals
- **Other structural (3)**: Various

### Python (115 tests: 103 pass, 12 fail)

- **Structural (10)**: print_statement/call confusion, match/case, patterns,
  _dedent internal name
- **Missing alias (1)**: string_content in error context
- **Empty parse (1)**: Various

### JavaScript (116 tests: 108 pass, 8 fail)

- **Empty parse (6)**: Class_Decorators, Classes, Extra_complex_literals,
  Class_Property_Fields, Private_Class_Property_Fields, JSX_entities
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
