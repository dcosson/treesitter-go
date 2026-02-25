# Remaining Corpus Failures Analysis

**Date**: 2026-02-17
**Main branch**: 563bedc (all P1 beads merged)
**Corpus**: 1525 PASS / 57 FAIL (1582 total, Lua SKIP'd)

## Summary by Category

| Category | Count | Description |
|----------|-------|-------------|
| **A. type_identifier vs identifier confusion** | 8 | Expression-position identifiers parsed as type_identifier |
| **B. Missing nested scope qualifiers** | 4 | Multi-level namespace chains flattened |
| **C. Missing template structure** | 5 | Template types stripped from qualified identifiers |
| **D. External scanner issues** | 8 | Heredoc/string scanner destroys AST structure |
| **E. DynPrec/GLR ambiguity** | 9 | Wrong production chosen in ambiguous contexts |
| **F. Non-associative operator failure** | 3 | Complete parse failure on chained non-assoc operators |
| **G. Error recovery failure** | 5 | Empty tree or missing ERROR nodes |
| **H. HTML implicit tag closing** | 2 | Missing HTML5 implicit end tag logic |
| **I. Grammar/scanner gaps** | 5 | Missing language features or scanner tokens |
| **J. Other** | 8 | Various one-off issues |

## Summary by Language

| Language | Failures | Main Issues |
|----------|----------|-------------|
| C | 5 | type_identifier confusion (4), empty output (1) |
| C++ | 15 | type_identifier (4), nested scopes (4), templates (5), other (2) |
| Perl | 16 | DynPrec ambiguity (7), non-assoc ops (3), scanner (2), grammar (2), other (2) |
| Ruby | 6 | Heredoc scanner (5), DynPrec (1) |
| Python | 4 | Error recovery (1), wrong node type (1), precedence (1), scanner (1) |
| Go | 2 | Error recovery (2) |
| HTML | 2 | Implicit tag closing (2) |
| Java | 2 | Type confusion (1), grammar gap (1) |
| JavaScript | 2 | ASI interaction (1), error recovery (1) |
| Rust | 1 | Scanner gap (1) |
| TypeScript | 2 | GLR ambiguity (1), grammar gap (1) |

---

## Category A: type_identifier vs identifier Confusion (8 tests)

**Affected**: C (4), C++ (4)
**Root cause**: The parser treats bare identifiers in statement position as `type_identifier` instead of wrapping them in `expression_statement (identifier)`. This is a declaration-vs-expression ambiguity that C tree-sitter resolves via its `_type_identifier` aliasing mechanism.

**Tests**:
- C/Cpp `Identifiers`: `_abc; d_EG123;` → `type_identifier` instead of `expression_statement (identifier)`
- C/Cpp `Unicode_Identifiers`: `µs; blah_accenté;` → same issue
- C/Cpp `Common_constants`: `true_value;` etc. → type_identifier for identifiers starting with keywords
- C/Cpp `Comments_after_for_loops_with_ambiguities`: identifiers in for-loop body → type_identifier

**Priority**: P2 — Systematic issue affecting C/C++ identifier resolution. Requires investigation of how C tree-sitter's `_type_identifier` aliasing works in declaration contexts.

## Category B: Missing Nested Scope Qualifiers (4 tests)

**Affected**: C++ (4)
**Root cause**: Multi-level namespace qualifications (`a::b::c`) are flattened to single level. The inner `qualified_identifier` is dropped.

**Tests**:
- Cpp `Assignment`: `a::b::c = 1` → loses `a::` scope
- Cpp `Using_declarations`: `using ::e::f::g` → loses intermediate scopes
- Cpp `Cast_operator_overload_declarations`: `A::B::operator C()` → loses `A::` scope
- Cpp `Class_scope_cast_operator_overload_declarations`: same as above in class context

**Priority**: P2 — Likely a reduce-loop issue where the parser doesn't build nested qualified_identifier nodes. May be related to how we handle left-recursive rules for scope resolution.

## Category C: Missing Template Structure (5 tests)

**Affected**: C++ (5)
**Root cause**: Template types with arguments are stripped from qualified identifiers. `std::vector<int>::size_type` becomes `std::size_type`.

**Tests**:
- Cpp `Concept_definition`: `std::is_base_of<U, T>::value` → loses template structure
- Cpp `Nested_template_calls`: `T::template Nested<int>::type` → loses dependent template
- Cpp `static_assert_declarations`: `std::is_constructible<A>::value` → same issue
- Cpp `Templates_with_optional_anonymous_parameters`: complex SFINAE template → completely collapsed
- Cpp `Namespaced_types`: `std::vector<int>::size_typ` → template stripped

**Priority**: P2 — Related to Category B (scope flattening). Template types in scope position of qualified_identifier are being dropped during reduce.

## Category D: External Scanner Issues (8 tests)

**Affected**: Ruby (5), Perl (2), Python (1)
**Root cause**: External scanner for heredocs and string literals modifies the token stream but fails to maintain AST structure context.

**Tests**:
- Ruby `basic_heredocs`: Multiple heredocs collapse entire AST structure
- Ruby `heredocs_in_context_starting_with_dot`: Method definitions lose structure
- Ruby `heredocs_with_interpolation`: Complex heredocs lose following statements
- Ruby `heredocs_with_in_args,_pairs,_and_arrays`: Nested heredocs destroy surrounding expressions
- Ruby `newline-delimited_strings`: Percent strings after newlines → complete failure
- Perl `Indented_heredocs`: Heredocs with `<<~\DELIM` silently dropped
- Perl `Space_skips_interpolation`: Space after `->` incorrectly allows subscripting
- Python `Raw_strings`: `r"\\"` generates incorrect string_content nodes

**Priority**: P2 — Scanner issues are language-specific and each needs targeted investigation.

## Category E: DynPrec/GLR Ambiguity Resolution (9 tests)

**Affected**: Perl (7), Ruby (1), TypeScript (1)
**Root cause**: The GLR parser picks the wrong production when multiple are viable. Dynamic precedence accumulation may still have edge cases.

**Tests**:
- Perl `Function_call_(0_args)`, `Function_call_(1_arg)`, `Function_call_(2_args)`: Parenthesized calls incorrectly classified as `ambiguous_function_call_expression`
- Perl `ambiguous_funcs`: Nested calls (`print print 'herro'`) fail — inner function treated as identifier
- Perl `ambiguous_funcs_-_indirect_object_fakeouts`: Similar to above
- Perl `sort_SUBNAME`: `sort +returns_list(...)` uses ambiguous instead of function_call
- Perl `autoquote_edge_cases`: Unary minus with autoquoting incorrect
- Ruby `nested_unparenthesized_method_calls`: `puts get_name self, true` fails
- TypeScript `Classes_with_extensions`: `B<C>(D)` in extends → binary_expression instead of call_expression

**Priority**: P2 — DynPrec fix helped significantly (+28 tests), but edge cases remain. May need per-grammar investigation.

## Category F: Non-Associative Operator Failure (3 tests)

**Affected**: Perl (3)
**Root cause**: Chaining non-associative operators causes complete parse failure with no recovery.

**Tests**:
- Perl `EXPR_<_EXPR_-_list/non_assoc`: `12 < 34 > 99` → complete failure
- Perl `EXPR_eq_EXPR_-_list/non_assoc`: `12 eq 34 eq 9002` → complete failure
- Perl `range_ops_-_nonassoc`: `1 .. 2 .. 3` → complete failure

**Priority**: P3 — These are intentional parse errors in Perl (non-assoc means chaining is illegal), but the parser should produce error recovery instead of empty output.

## Category G: Error Recovery Failure (5 tests)

**Affected**: Go (2), Python (1), JavaScript (1), C (1)
**Root cause**: Parser produces empty output or loses structure instead of generating ERROR nodes.

**Tests**:
- Go `Error_detected_at_globally_reserved_keyword`: Incomplete selector + keyword → flat tokens instead of ERROR wrapping
- Go `String_literals`: Unterminated string → loses const_declaration wrapper
- Python `An_error_before_a_string_literal`: `badthing "string"` → complete failure
- JavaScript `Extra_complex_literals_in_expressions`: Two object literals without operator → structural collapse
- C `Typedefs`: Large typedef test → complete empty output

**Priority**: P2 — Error recovery is critical for editor integration. These may indicate issues in handleError or the recover path.

## Category H: HTML Implicit Tag Closing (2 tests)

**Affected**: HTML (2)
**Root cause**: The HTML scanner doesn't implement all implicit end tag rules from the HTML5 spec.

**Tests**:
- HTML `COLGROUP_elements_without_end_tags`: `<colgroup>` not implicitly closed before `<tr>`
- HTML `TR,_TD,_and_TH_elements_without_end_tags`: `<th>` and `<td>` implicit closing rules missing

**Priority**: P3 — HTML scanner needs additional implicit end tag rules. Low impact.

## Category I: Grammar/Scanner Gaps (5 tests)

**Affected**: Perl (2), Java (1), Rust (1), TypeScript (1)
**Root cause**: Missing language features or scanner token types.

**Tests**:
- Perl `Extended_try_/_catch_of_Syntax::Keyword::Try`: Try/catch with stub expressions fail
- Perl `Double_dollar_edge_cases`: Special variables (`$$:`, `$$'`, `$[`) cause failure
- Java `switch_with_unnamed_pattern_variable`: Java 21+ record pattern with `_` → empty tree
- Rust `Line_doc_comments`: Doc comment markers (`//!`, `///`) not recognized → empty tree
- TypeScript `Type_arguments_in_JSX`: JSX with type arguments → empty tree

**Priority**: P3 — These require grammar/scanner additions for specific language features.

## Category J: Other (8 tests)

- Perl `Attribute_plus_signature`: Sub signature parsed as attribute value (grammar bug)
- Perl `not_confused_by_leading_whitespace`: Whitespace disambiguation issue
- Python `Format_strings`: F-string interpolations use wrong node types (pattern_list vs expression_list)
- Python `Lists`: Splat operator precedence wrong (`[*a.b]` parsed incorrectly)
- Java `method_references`: `foo.bar::method` → scoped_type_identifier instead of field_access (type confusion)
- JavaScript `Alphabetical_infix_operators_split_across_lines`: ASI interaction with `in`/`instanceof`
- Cpp `Noexcept_specifier`: sizeof(T) in template → type_descriptor instead of parenthesized_expression
- Cpp `Parameter_pack_expansions`: type_descriptor stripped from template_argument_list

---

## Recommended Fix Priority

### P1 — High Impact, Multiple Languages
1. **Error recovery improvements** (Cat G, 5 tests) — Affects Go, Python, JavaScript, C. Critical for editor integration.
2. **type_identifier vs identifier** (Cat A, 8 tests) — Affects all C/C++ code with bare identifier statements.

### P2 — Language-Specific, Important
3. **DynPrec/GLR edge cases** (Cat E, 9 tests) — Mostly Perl. May need per-grammar DynPrec tuning.
4. **C++ nested scopes + templates** (Cat B+C, 9 tests) — Systematic C++ issue, likely related reduce-loop/scope handling.
5. **External scanner fixes** (Cat D, 8 tests) — Ruby heredocs (5), Perl (2), Python (1). Each needs targeted work.

### P3 — Lower Priority
6. **Non-assoc operator recovery** (Cat F, 3 tests) — Perl only, error recovery rather than parse fix.
7. **HTML implicit tags** (Cat H, 2 tests) — Scanner extension needed.
8. **Grammar gaps** (Cat I, 5 tests) — Missing language features, individual fixes.
