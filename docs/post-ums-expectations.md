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
| Structural — Cluster C (qualified id) | 7 | **5-7** | 0-2 |
| Structural — non-GLR | ~27 | 0-3 | 24-27 |
| Empty/nil parse tree | 19 | **2-5** | 14-17 |
| Internal name leaking | 12 | 0 | 12 |
| **Total** | **108** | **35-53** | **55-73** |

**Predicted post-ums pass rate: ~1546-1564 / 1619 (95.5-96.6%)**

> **UPDATE 2026-02-16**: Investigation confirmed Cluster C (qualified identifier
> truncation, 7 C++ tests) IS a GLR version comparison issue, not a separate
> grammar bug. The C reference parser handles `a::b::c` correctly with the same
> parse tables — our parser prematurely reduces `a::b` because condenseStack
> picks the shorter/lower-cost version. Reclassified from NONE→MEDIUM confidence.

---

## Per-Test Predictions

### Confidence Levels

- **HIGH** — directly caused by missing version comparison/pruning
- **MEDIUM** — likely GLR-related but may have additional factors
- **LOW** — might improve but not primarily a GLR issue
- **NONE** — unrelated to GLR version comparison

---

### C++ (25 failures → predicted 15-27 fixed)

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
| Using_declarations | C (qualified id) | **PASS** | MEDIUM — GLR version prunes nested qualified_id |
| Assignment | C (qualified id) | **PASS** | MEDIUM — GLR version prunes `a::b::c` |
| Cast_operator_overload_declarations | C (qualified id) | **PASS** | MEDIUM — GLR version prunes `A::B::operator` |
| Class_scope_cast_operator_overload_declarations | C (qualified id) | **PASS** | MEDIUM — same pattern |
| Namespaced_types | C (qualified id) | **PASS** | MEDIUM — GLR prunes `vector<int>::type` |
| Nested_template_calls | C (qualified id) | **PASS** | MEDIUM — GLR prunes `T::template X<int>::type` |
| Templates_with_optional_anonymous_parameters | C (qualified id) | **PASS** | MEDIUM — GLR prunes nested scope |
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

### Should Also Pass (MEDIUM confidence, Cluster C reclassified) — 7 tests

- [ ] C++ Using_declarations
- [ ] C++ Assignment
- [ ] C++ Cast_operator_overload_declarations
- [ ] C++ Class_scope_cast_operator_overload_declarations
- [ ] C++ Namespaced_types
- [ ] C++ Nested_template_calls
- [ ] C++ Templates_with_optional_anonymous_parameters

**Verified via C reference parser**: same grammar tables produce correct nested
qualified_identifiers. Our parser truncates at depth 2 due to GLR version pruning.

### Should Still Fail (unrelated to UMS)

- All 12 internal-name failures (Perl/Ruby/Python scanner bugs)
- All 7 HTML failures (implicit close tag scanner)
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

After ums, the remaining ~55-73 failures will break down roughly as:

| Category | Count | Fix |
|----------|-------|-----|
| Internal name leaking | 12 | Parser/scanner wrong-root bugs (wcu.19) |
| Empty/nil parse tree | 14-17 | Mixed scanner/lex issues |
| Qualified identifier truncation | 0-2 | Should be mostly fixed by ums (reclassified) |
| Comment placement | 3 | Alt GLR path extras stripping |
| Long tail structural | 25-35 | Individual investigation |

The next highest-impact fix after ums would be **internal name leaking**
(wcu.19, 12 tests) — investigating Perl/Ruby/Python scanner wrong-root
production. If Cluster C qualified identifier truncation is NOT fully fixed
by ums, it would be the next target (~0-2 remaining tests).

> **UPDATE**: Previously said qualified identifier truncation was the next fix.
> Investigation showed it shares ums root cause — reclassified as ums-fixable.

---

## Post-UMS Actual Results (2026-02-16, main at 48e73ff)

**Actual: 1532 pass / 102 fail / 1634 total (93.8%)**

Note: 15 new test cases added since baseline (1619→1634), so raw numbers aren't
directly comparable. Net improvement: +6 (9 improvements - 3 regressions).

### Prediction vs Actual

| Category | Predicted Fixes | Actual Fixes | Notes |
|----------|:-:|:-:|-------|
| Timeout (C++) | 8 | **6** | Concept_definition still times out; Parameter_pack_expansions became structural |
| Structural — GLR ambiguity | 20-30 | **0** | NONE of the predicted GLR fixes happened |
| Structural — Cluster C | 5-7 | **0** | Needs soft preferences (not just decisive kills) |
| Structural — non-GLR | 0-3 | **3** | Ruby nested_strings, HTML comment, Java type_arguments |
| Empty/nil parse tree | 2-5 | **0** | None fixed |
| Internal name leaking | 0 | **0** | As predicted |
| **Total improvements** | **35-53** | **9** | Significantly below prediction |
| **Regressions** | 0 | **3** | 1 true (Complex_fold_expression), 2 pre-existing |
| **Net** | +35-53 | **+6** | |

### HIGH Confidence Predictions: 6/8 Correct

| Test | Predicted | Actual | Notes |
|------|-----------|--------|-------|
| C++ casts_vs_multiplications | PASS | **PASS** | Correct |
| C++ Noreturn_Type_Qualifier | PASS | **PASS** | Correct |
| C++ For_loops | PASS | **PASS** | Correct |
| C++ Switch_statements | PASS | **PASS** | Correct |
| C++ Concept_definition | PASS | **FAIL** | Still times out (wrong) |
| C++ Compound_literals_without_parentheses | PASS | **PASS** | Correct |
| C++ Template_calls | PASS | **PASS** | Correct |
| C++ Parameter_pack_expansions | PASS | **FAIL** | Now structural instead of timeout (partial) |

### MEDIUM Confidence: 3/31 Correct (10% hit rate!)

Surprise improvements not predicted at MEDIUM:
- Ruby nested_strings_with_different_delimiters (predicted NONE → actually PASS)
- HTML comment (predicted NONE → actually PASS)
- Java type_arguments_with_generic_types (predicted MEDIUM → PASS, correct)

All other MEDIUM predictions (Go call/type_conv, C type_id, C++ Cluster C,
Python print, Perl function_call) did NOT improve. The decisive-kills-only
approach cannot resolve dynamic-precedence-based ambiguities.

### Regressions (PASS → FAIL)

| Test | Type | Root Cause |
|------|------|-----------|
| C++ Complex_fold_expression | **True UMS regression** | Error recovery infinite loop (findActiveVersion starvation) |
| C++ template_functions_vs_relational | **Severity worsened** | Structural → timeout (same root cause) |
| HTML Void_tags | Pre-existing | Was already failing |
| Java method_references | Pre-existing | Was already failing |

### Key Takeaway

The decisive-kills-only UMS approach was highly effective at resolving
**cost-based version explosion** (timeouts) but has no effect on
**dynamic-precedence-based ambiguity resolution** (structural mismatches).
The predicted 35-53 improvements assumed soft preferences would also work,
but those were deliberately deferred due to findActiveVersion regressions.

The remaining 102 failures require a different approach:
1. **Soft preference handling** (for Cluster C, call vs type_conv, etc.)
2. **Scanner/lex fixes** (for empty trees, internal name leaking)
3. **Per-grammar investigation** (for unique structural issues)
