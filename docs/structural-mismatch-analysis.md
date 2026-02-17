# Structural Mismatch Failure Analysis

**Date**: 2026-02-16
**Author**: reviewer agent
**Scope**: 69 structural mismatch failures across 13 languages

## Summary

The 69 structural mismatches cluster into **6 distinct root cause patterns**.
Two patterns account for ~50% of failures and are potentially fixable with
targeted changes. The remaining ~50% are individual GLR ambiguity resolution
differences that will largely be addressed by the ums (condenseStack) fix.

---

## Pattern Clusters

### Cluster 1: `call_expression` vs `type_conversion_expression` (Go) — ~6 tests

**Languages**: Go (6)
**Tests**: Type_switch_statements, Select_statements, For_statements, Generic_call_expressions, Type_conversion_expressions, Function_declarations

**Pattern**: Our parser resolves `identifier(args)` as `type_conversion_expression`
instead of `call_expression` in contexts where the identifier follows a case
clause, appears inside a select/for body, or involves generics.

**Examples**:
- `println(x)` → we produce `type_conversion_expression(type_identifier, identifier)`,
  expected `call_expression(identifier, argument_list(identifier))`
- `a[b, c](d)` → we produce `call_expression` with wrong generic structure,
  expected `type_conversion_expression(generic_type(...))`

**Root cause**: Go's grammar has inherent ambiguity between type conversions and
function calls: `Type(expr)` vs `func(args)`. The correct resolution depends on
GLR version selection — the version with `call_expression` should win over
`type_conversion_expression` in most contexts. This is directly related to
**ums (condenseStack)** — with proper version pruning via `compareVersions`,
the C parser picks the right resolution.

**Fix**: Will likely be addressed by ums fix. Also possibly bead nlb
(type_conversion_expression ambiguity).

---

### Cluster 2: `type_identifier` instead of `identifier` (C/C++) — ~8 tests

**Languages**: C (4), C++ (4, shared tests)
**Tests**: Identifiers, Common_constants, Primitive-typed_variable_declarations, Type_modifiers

**Pattern**: Expressions in statement context produce `type_identifier` where
`identifier` is expected. The parser is treating standalone identifiers in
expression position as type names.

**Examples**:
- `_abc;` → `(type_identifier)` instead of `(expression_statement (identifier))`
- `true_value;` → `(type_identifier)` instead of `(expression_statement (identifier))`
- `unsigned f;` → `(sized_type_specifier (type_identifier))` instead of
  `(declaration (sized_type_specifier) (identifier))`

**Root cause**: C/C++ grammars use GLR to resolve declaration-vs-expression
ambiguity. `_abc;` could be a declaration of type `_abc` or an expression
statement using identifier `_abc`. The correct resolution depends on
`ts_parser__compare_versions` choosing the expression interpretation over
the declaration interpretation. Without proper version comparison, the
wrong fork wins.

**Fix**: Directly related to **ums (condenseStack)**. With pair-wise
version comparison and dynamic precedence tracking, the expression
interpretation should be preferred.

---

### Cluster 3: Perl function call disambiguation — ~7 tests

**Languages**: Perl (7)
**Tests**: Function_call_(0_args), Function_call_(1_arg), Function_call_(2_args),
ambiguous_funcs, not_confused_by_leading_whitespace, Defer, sort_with_BLOCK

**Pattern**: Multiple related sub-patterns:
- `function_call_expression` vs `ambiguous_function_call_expression` — the
  parser picks the wrong disambiguation
- `ambiguous_function_call_expression` argument grouping differs — arguments
  grouped under the wrong function call
- `stub_expression` appears where it shouldn't

**Examples**:
- `A()` inside block → `ambiguous_function_call_expression(function, stub_expression)`
  instead of `function_call_expression(function)`
- `print 'things', sum 1, 2, 3` → arguments split differently between
  the nested `print` and `sum` calls
- `=head1()` → `ambiguous_function_call_expression` instead of
  `function_call_expression`

**Root cause**: Perl's grammar has heavy ambiguity around function calls
(parenthesized vs unparenthesized, with vs without indirect objects). The
C parser uses `prec.dynamic()` to prefer certain resolutions. Our GLR
version pruning doesn't respect these dynamic precedence values correctly
during version comparison, so the wrong parse path survives.

**Fix**: Partially **ums (condenseStack)** — the dynamic precedence
tiebreaker in `compareVersions` will help. But some may also need
grammar-specific investigation.

---

### Cluster 4: Python `print` as statement vs function — ~4 tests

**Languages**: Python (4)
**Tests**: Print_used_as_an_identifier, Print_statements, Matching_specific_values,
Adding_a_wild_card

**Pattern**: `print(...)` parsed as `print_statement` instead of function `call`.
Python 2 `print` statement grammar rule activates when it shouldn't.

**Examples**:
- `print(a)` → `print_statement(parenthesized_expression(identifier))`
  instead of `call(identifier, argument_list(identifier))`
- `print(d, e)` → `print_statement(tuple(identifier, identifier))`
  instead of `call(identifier, argument_list(identifier, identifier))`
- `print("Goodbye!")` inside `match/case` → `print_statement`
  instead of `call`

**Root cause**: Python's tree-sitter grammar supports both Python 2 and 3.
`print` can be either a statement (Python 2) or a function call (Python 3).
The grammar uses GLR with dynamic precedence to prefer the function call
interpretation. Our parser picks the wrong fork.

**Fix**: **ums (condenseStack)** — proper dynamic precedence comparison
will select the `call` interpretation over `print_statement`.

---

### Cluster 5: C++ template/declaration ambiguity — ~8 tests

**Languages**: C++ (8 non-timeout structural)
**Tests**: template_functions_vs_relational_expressions, Assignments,
Using_declarations, Variadic_templates, Templates_with_optional_anonymous_parameters,
static_assert_declarations, Cast_operator_overload_declarations,
Class_scope_cast_operator_overload_declarations, Nested_template_calls,
Noexcept_specifier, Namespaced_types, Assignment, pointers

**Pattern**: Multiple related sub-patterns, all involving C++ ambiguities:
- `template_method` instead of relational comparison (`x.foo < 0` → template)
- `structured_binding_declarator` instead of subscript (`h[i] = j` → structured binding)
- `sizeof(T)` → `sizeof_expression(type_descriptor)` instead of
  `sizeof_expression(parenthesized_expression(identifier))`
- Wrong template argument grouping

**Root cause**: C++ has the densest ambiguity of any grammar. Every `<`
can be a template or comparison, every `(Type)` can be a cast or
multiplication. These are resolved by GLR version selection. Our
condenseStack can't properly prune the wrong interpretations.

**Fix**: **ums (condenseStack)** is the primary fix. The 8 timeouts
AND most of these 8 structural mismatches share the same root cause.

---

### Cluster 6: Comment/extras placement edge cases — ~3 tests

**Languages**: C (1), C++ (1), Bash (1)
**Tests**: Comments_after_for_loops_with_ambiguities, Words_containing_bare_'#'

**Pattern**: Comments attach to the wrong parent node.

**Examples**:
- C/C++: Comment after `for` loop's `}` attaches inside `compound_statement`
  instead of as sibling
- Bash: `# comment` at end of line attaches as child of `command` instead
  of sibling

**Root cause**: Trailing extras stripping in `doReduce` handles the primary
path but the comment still lands wrong due to GLR alt path extras handling.
The alt GLR path gap identified during comment extras review.

**Fix**: Fix the alt GLR paths (lines 730-761 in parser.go) to also strip
trailing extras. This is a follow-up to the original comment extras fix.

---

### Cluster 7: Long tail — miscellaneous (~33 tests)

**Languages**: All remaining
**Pattern**: Individual parser correctness issues, each with a unique root cause:

**Sub-groups**:

| Sub-pattern | Count | Languages | Description |
|---|---|---|---|
| Heredoc/string scoping | ~6 | Ruby(3), Perl(4) | Heredoc body as child vs sibling, string internal nodes |
| Python tuple/assignment | ~3 | Python | `(a, b, c,)` parsed as assignment instead of tuple |
| JS object/block ambiguity | ~2 | JS | `{ g: h }` parsed as labeled statement instead of object |
| Perl operator precedence | ~4 | Perl | `eq`/`<`/range operator associativity wrong |
| Perl autoquoting | ~2 | Perl | `return unless =>` — return nesting wrong |
| Go generic call | ~1 | Go | Nested generic type arg resolution |
| JS await context | ~1 | JS | `await` as reserved word vs identifier |
| Bash redirect | ~2 | Bash | Redirect token handling, variable expansion |
| Rust | ~1 | Rust | Minor structural |
| Java | ~2 | Java | Minor structural |
| TS | ~1 | TS | Minor structural |
| HTML | ~2 | HTML | COLGROUP nesting, comment |
| Lua | ~1 | Lua | Comment count mismatch |

Most of these will benefit from the **ums fix** as improved version pruning
helps the parser pick the right GLR resolution globally.

---

## Impact Prediction

| Fix | Tests Addressed | Clusters |
|-----|----------------|----------|
| **ums (condenseStack)** | ~35-45 | 1, 2, 3 (partial), 4, 5, 7 (partial) |
| **Alt path extras stripping** | ~3 | 6 |
| **Perl-specific grammar tuning** | ~5-7 | 3 (remainder) |
| **Remaining long tail** | ~15-20 | 7 (remainder) |

**Key insight**: The ums fix (condenseStack rewrite) is expected to address
~50-65% of structural mismatches in addition to the 8 timeouts. This is because
most structural mismatches are GLR ambiguity resolution issues where our parser
picks the wrong fork due to insufficient pruning.

---

## Recommended Fix Priority

1. **ums (condenseStack)** — already analyzed, implementation plan ready.
   Expected to fix 8 timeouts + 35-45 structural mismatches = 43-53 tests.
   This alone could push the corpus from 93.3% to ~96-97%.

2. **Alt path trailing extras** — small targeted fix in parser.go doReduce,
   lines 730-761. Expected to fix ~3 comment placement tests.

3. **Perl function call resolution** — may need grammar-specific investigation
   after ums lands. The dynamic precedence tiebreaker in compareVersions
   should fix most, but some Perl-specific issues may remain.

4. **Python print disambiguation** — likely fixed by ums. If not,
   investigate dynamic precedence values in Python grammar.

5. **Long tail** — address individually after ums and other fixes land.
   Re-run corpus and re-categorize remaining failures.
