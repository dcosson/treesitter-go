# Post-UMS Corpus Test Expectations

**Date**: 2026-02-16
**Author**: reviewer agent
**Purpose**: Predict per-test outcomes after the ums (condenseStack) fix lands,
to verify the fix has the expected impact.

## Current Baseline: 1511/1619 (93.3%)

108 failures: 69 structural, 19 empty, 12 internal-name, 8 timeout

## Prediction Summary

| Category | Current | Predicted Fix by UMS | Remaining |
|----------|---------|---------------------|-----------|
| Timeout (C++) | 8 | **8** (all) | 0 |
| Structural — GLR ambiguity | ~35 | **20-30** | 5-15 |
| Structural — non-GLR | ~34 | 0-3 | 31-34 |
| Empty/nil parse tree | 19 | **2-5** | 14-17 |
| Internal name leaking | 12 | 0 | 12 |
| **Total** | **108** | **30-46** | **62-78** |

**Predicted post-ums pass rate: ~1541-1557 / 1619 (95.2-96.2%)**

---

## Per-Test Predictions

### Confidence Levels

- **HIGH** — directly caused by missing version comparison/pruning
- **MEDIUM** — likely GLR-related but may have additional factors
- **LOW** — might improve but not primarily a GLR issue
- **NONE** — unrelated to GLR version comparison

---

### C++ (25 failures → predicted 15-20 fixed)

#### Timeouts (8) — ALL should fix (HIGH confidence)

| Test | Prediction | Confidence |
|------|-----------|------------|
| casts_vs_multiplications | **PASS** | HIGH |
| Noreturn_Type_Qualifier | **PASS** | HIGH |
| For_loops | **PASS** | HIGH |
| Switch_statements | **PASS** | HIGH |
| Concept_definition | **PASS** | HIGH |
| Compound_literals_without_parentheses | **PASS** | HIGH |
| Template_calls | **PASS** | HIGH |
| Parameter_pack_expansions | **PASS** | HIGH |

These are the canonical ums failures — version explosion from uncontrolled
GLR forking on template/cast ambiguities.

#### Structural (17) — predict 7-12 fixed

| Test | Cluster | Prediction | Confidence |
|------|---------|-----------|------------|
| template_functions_vs_relational | M (GLR) | **PASS** | MEDIUM — `x.foo < 0` template vs comparison |
| Assignments | K (struct binding) | **PASS** | MEDIUM — `h[i]=j` declaration vs expression |
| pointers | K (struct binding) | **PASS** | MEDIUM — same pattern |
| Variadic_templates | K (struct binding) | **PASS** | MEDIUM — same pattern |
| Common_constants | E (type_id) | **PASS** | MEDIUM — declaration vs expression |
| Type_modifiers | E (type_id) | **PASS** | MEDIUM — declaration vs expression |
| Primitive-typed_variable_declarations | E (type_id) | **PASS** | MEDIUM — declaration vs expression |
| static_assert_declarations | A (GLR) | **PASS** | MEDIUM — qualified template |
| Using_declarations | C (qualified id) | FAIL | NONE — truncation, not GLR |
| Assignment | C (qualified id) | FAIL | NONE — truncation |
| Cast_operator_overload_declarations | C (qualified id) | FAIL | NONE — truncation |
| Class_scope_cast_operator_overload_declarations | C (qualified id) | FAIL | NONE — truncation |
| Namespaced_types | C (qualified id) | FAIL | NONE — truncation |
| Nested_template_calls | C (qualified id) | FAIL | NONE — truncation |
| Templates_with_optional_anonymous_parameters | C (qualified id) | FAIL | NONE — truncation |
| Noexcept_specifier | M (sizeof) | FAIL | LOW — sizeof scope |
| Comments_after_for_loops | H (comment) | FAIL | NONE — extras placement |

### Go (8 failures → predicted 4-6 fixed)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| Type_switch_statements | A (call vs type_conv) | **PASS** | HIGH — classic GLR |
| Select_statements | A (call vs type_conv) | **PASS** | HIGH — classic GLR |
| For_statements | A (call vs type_conv) | **PASS** | HIGH — classic GLR |
| Type_conversion_expressions | A (call vs type_conv) | **PASS** | HIGH — classic GLR |
| Generic_call_expressions | A (call vs type_conv) | **PASS** | MEDIUM — more complex |
| Function_declarations | E/M (type_id) | FAIL | LOW — param declaration |
| Error_detected_at_globally_reserved_keyword | Empty | FAIL | NONE — keyword issue |
| String_literals | Empty | FAIL | NONE — lex issue |

### Perl (27 failures → predicted 5-10 fixed)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| Function_call_(0_args) | F (call vs ambiguous) | **PASS** | MEDIUM |
| Function_call_(1_arg) | F (call vs ambiguous) | **PASS** | MEDIUM |
| Function_call_(2_args) | F (call vs ambiguous) | **PASS** | MEDIUM |
| try_/_catch | F (call vs ambiguous) | **PASS** | MEDIUM |
| Extended_try_/_catch | F (call vs ambiguous) | **PASS** | MEDIUM |
| Defer | F (call vs ambiguous) | **PASS** | MEDIUM |
| not_confused_by_leading_whitespace | F (call vs ambiguous) | **PASS** | MEDIUM |
| ambiguous_funcs | F (arg grouping) | FAIL | LOW — arg precedence |
| ambiguous_funcs_indirect | F (arg grouping) | FAIL | LOW — arg precedence |
| sort_with_BLOCK | J (sort precedence) | FAIL | LOW — sort-specific |
| autoquoting_postfix | M (return precedence) | FAIL | LOW |
| autoquote_edge_cases | M (unary) | FAIL | LOW |
| Attribute_plus_signature | M (signature) | FAIL | NONE — scanner |
| EXPR_eq_EXPR_non_assoc | I (error recovery) | FAIL | LOW |
| EXPR_<_EXPR_non_assoc | I (error recovery) | FAIL | LOW |
| just_solidus_DOR_vs_regex | A (GLR) | FAIL | LOW — regex vs divide |
| Non-quoted_heredoc | D (internal name) | FAIL | NONE — scanner |
| Indented_heredocs | D (internal name) | FAIL | NONE — scanner |
| ''_strings | D (internal name) | FAIL | NONE — scanner |
| ""_strings | D (internal name) | FAIL | NONE — scanner |
| Interpolation_in_""_strings | D (internal name) | FAIL | NONE — scanner |
| qw()_lists | D (internal name) | FAIL | NONE — scanner |
| Array_element_interpolation | D (internal name) | FAIL | NONE — scanner |
| Hash_element_interpolation | D (internal name) | FAIL | NONE — scanner |
| Space_skips_interpolation | D (internal name) | FAIL | NONE — scanner |
| Double_dollar_edge_cases | Empty | FAIL | NONE — scanner |
| range_ops_-_nonassoc | Empty | FAIL | NONE — ERROR node |

### Python (11 failures → predicted 2-4 fixed)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| Print_used_as_an_identifier | G (print) | **PASS** | MEDIUM |
| Print_statements | G (print) | **PASS** | MEDIUM |
| Matching_specific_values | G (print) | **PASS** | MEDIUM |
| Adding_a_wild_card | G (print) | **PASS** | MEDIUM |
| Simple_Tuples | M (tuple) | FAIL | LOW — tuple vs assignment |
| Lists | L (splat scope) | FAIL | NONE — precedence |
| Format_strings | L (splat) | FAIL | NONE — pattern |
| With_statements | M (with) | FAIL | LOW |
| Raw_strings | M (raw) | FAIL | NONE — string content |
| Function_definitions | D (internal _dedent) | FAIL | NONE — scanner |
| An_error_before_string_literal | Empty | FAIL | NONE — ERROR node |

### C (7 failures → predicted 3-5 fixed)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| Identifiers | E (type_id) | **PASS** | MEDIUM |
| Common_constants | E (type_id) | **PASS** | MEDIUM |
| Type_modifiers | E (type_id) | **PASS** | MEDIUM |
| Primitive-typed_variable_declarations | E (type_id) | **PASS** | MEDIUM |
| Call_expressions_vs_empty_declarations | A/C | **PASS** | MEDIUM |
| Comments_after_for_loops | H (comment) | FAIL | NONE |
| Typedefs | Empty | FAIL | NONE |

### Ruby (8 failures → predicted 0-1 fixed)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| nested_unparenthesized_method_calls | M (GLR) | **PASS** | LOW — maybe |
| pattern_matching_with_fancy_string_literals | M (heredoc scope) | FAIL | NONE |
| newline-delimited_strings | Empty | FAIL | NONE |
| basic_heredocs | Empty | FAIL | NONE |
| heredocs_with_in_args | Empty | FAIL | NONE |
| heredocs_in_context | D (internal name) | FAIL | NONE |
| heredocs_with_interpolation | D (internal name) | FAIL | NONE |
| nested_strings_with_different_delimiters | D (internal name) | FAIL | NONE |

### HTML (7 failures → predicted 0)

All HTML failures are implicit close tag issues (scanner) or entity handling.
None are GLR-related.

| Test | Category | Prediction |
|------|----------|-----------|
| DT/DL elements | Empty (implicit close) | FAIL |
| Ruby annotation elements | Empty (implicit close) | FAIL |
| LI elements | Empty (wrong root) | FAIL |
| P elements | Empty (wrong root) | FAIL |
| TR/TD/TH elements | Empty (wrong root) | FAIL |
| COLGROUP | Structural | FAIL |
| comment | Structural | FAIL |

### Bash (4 failures → predicted 0-1)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| Variable_Expansions_Weird_Cases | Structural | FAIL | LOW |
| Words_containing_bare_'#' | H (comment) | FAIL | NONE |
| Words_containing_#_not_comments | H (comment) | FAIL | NONE |
| File_redirects | Empty | FAIL | NONE |

### JavaScript (4 failures → predicted 1-2)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| Objects | M (block vs object) | **PASS** | LOW — maybe GLR |
| Reserved_words_as_identifiers | M (await context) | FAIL | LOW |
| Alphabetical_infix_operators | Empty | FAIL | NONE |
| Extra_complex_literals | Empty | FAIL | NONE |

### Java (3 failures → predicted 0-1)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| type_arguments_with_generic_types | A (GLR) | **PASS** | MEDIUM |
| switch_with_unnamed_pattern_variable | Empty | FAIL | NONE |
| (other structural) | Structural | FAIL | LOW |

### TypeScript (2 failures → predicted 0-1)

| Test | Category | Prediction | Confidence |
|------|----------|-----------|------------|
| Classes_with_extensions | A (GLR) | **PASS** | MEDIUM |
| Type_arguments_in_JSX | Empty | FAIL | NONE |

### Rust (1 failure → predicted 0)

| Test | Category | Prediction |
|------|----------|-----------|
| Line_doc_comments | M (comment nesting) | FAIL |

### Lua (1 failure → predicted 0)

| Test | Category | Prediction |
|------|----------|-----------|
| comment | M (count mismatch) | FAIL |

---

## Verification Checklist

After ums lands, run `go test -run TestCorpus -v -timeout 600s .` and check:

### Must Pass (HIGH confidence) — 8 tests

- [ ] C++ casts_vs_multiplications
- [ ] C++ Noreturn_Type_Qualifier
- [ ] C++ For_loops
- [ ] C++ Switch_statements
- [ ] C++ Concept_definition
- [ ] C++ Compound_literals_without_parentheses
- [ ] C++ Template_calls
- [ ] C++ Parameter_pack_expansions

**If ANY of these still fail, the ums fix is incomplete.**

### Should Pass (MEDIUM confidence) — ~20 tests

- [ ] Go Type_switch_statements
- [ ] Go Select_statements
- [ ] Go For_statements
- [ ] Go Type_conversion_expressions
- [ ] Go Generic_call_expressions
- [ ] C Identifiers
- [ ] C Common_constants
- [ ] C Type_modifiers
- [ ] C Primitive-typed_variable_declarations
- [ ] C Call_expressions_vs_empty_declarations
- [ ] C++ Common_constants
- [ ] C++ Type_modifiers
- [ ] C++ Primitive-typed_variable_declarations
- [ ] C++ Assignments
- [ ] C++ pointers
- [ ] C++ Variadic_templates
- [ ] C++ static_assert_declarations
- [ ] C++ template_functions_vs_relational
- [ ] Python Print_used_as_an_identifier
- [ ] Python Print_statements
- [ ] Python Matching_specific_values
- [ ] Python Adding_a_wild_card
- [ ] Perl Function_call_(0_args)
- [ ] Perl Function_call_(1_arg)
- [ ] Perl Function_call_(2_args)
- [ ] Perl try_/_catch
- [ ] Perl Extended_try_/_catch
- [ ] Perl Defer
- [ ] Perl not_confused_by_leading_whitespace
- [ ] Java type_arguments_with_generic_types
- [ ] TS Classes_with_extensions

**If fewer than 15 of these pass, the compareVersions dynamic precedence
handling may need tuning.**

### Should Still Fail (unrelated to UMS)

- All 12 internal-name failures (Perl/Ruby/Python scanner bugs)
- All 7 HTML failures (implicit close tag scanner)
- All 5+ C++ qualified identifier truncation (Cluster C)
- Comment placement tests (Cluster H)
- Most empty/nil parse trees (scanner/lex issues)

### Bonus: Possible Surprise Fixes

Some empty/nil parse tree failures might also resolve if the parser was
previously timing out silently (producing nil tree) due to version explosion:
- Go Error_detected_at_globally_reserved_keyword
- Go String_literals
- C Typedefs
- Java switch_with_unnamed_pattern_variable

---

## Post-UMS Remaining Fix Priorities

After ums, the remaining ~62-78 failures will break down roughly as:

| Category | Count | Fix |
|----------|-------|-----|
| Internal name leaking | 12 | Parser/scanner wrong-root bugs (wcu.19) |
| Empty/nil parse tree | 14-17 | Mixed scanner/lex issues |
| Qualified identifier truncation | 7 | Separate parsing depth fix |
| Comment placement | 3 | Alt GLR path extras stripping |
| Long tail structural | 25-35 | Individual investigation |

The next highest-impact fix after ums would be the **qualified identifier
truncation** (~7 tests, likely a simpler parser change).
