# Structural Mismatch Cluster Analysis

Date: 2026-02-16

Analysis of the 69 structural mismatch failures to identify clusters of similar
root causes that could be addressed together.

## Summary of Clusters

| Cluster | Count | Languages | Root Cause |
|---------|------:|-----------|------------|
| A. GLR Ambiguity: call vs type_conversion | 10 | Go(5), C++(1), Perl(1), C(1), Java(1), TS(1) | GLR picks wrong alternative for `Name(args)` |
| B. GLR Timeout (infinite loop) | 8 | C++(8) | condenseStack gap in version pruning |
| C. Qualified/Nested Identifier Truncation | 7 | C++(5), C(1), Java(1) | `a::b::c` stops at `a::b`, missing final segment |
| D. Internal Name Leaking (scanner roots) | 8 | Perl(6), Perl overlap with structural(2) | `_qq_string_content`/`_q_string_content` as root |
| E. type_identifier vs identifier Confusion | 6 | C(3), C++(2), Go(1) | Identifiers in expression context parsed as types |
| F. Perl function_call vs ambiguous_function_call | 6 | Perl(6) | GLR prefers ambiguous when unambiguous expected |
| G. Python print_statement vs call | 4 | Python(4) | Python 2/3 `print` keyword vs function ambiguity |
| H. Comment Placement (inside vs outside) | 3 | Bash(2), C(1) | Comment attached inside block vs after it |
| I. Perl Non-Associative Operator Errors | 2 | Perl(2) | Missing ERROR node for non-assoc violations |
| J. Perl Sort/List Precedence | 1 | Perl(1) | sort greedily captures only first arg |
| K. C++ Structured Binding False Positive | 3 | C++(3) | `h[i] = j` parsed as structured binding |
| L. Python Splat Scope in Patterns | 2 | Python(2) | `*a.b` scoping: `(list_splat (attr))` vs `(attr (list_splat))` |
| M. One-Off GLR/Precedence Issues | 7 | Various | Unique per-language issues |
| **Total** | **~69** | | |

---

## Cluster A: GLR Ambiguity - call_expression vs type_conversion_expression (10)

**Root cause**: When the parser sees `Name(args)`, it must decide whether this
is a function call or a type conversion (cast). In the C reference parser, GLR
version comparison (`ts_parser__compare_versions`) prunes the wrong
alternative. Our port lacks this or resolves it differently.

### Go (5 tests)

| Test | Difference |
|------|------------|
| `Type_switch_statements` | `printString("x")` -> `type_conversion_expression(type_identifier, interpreted_string_literal)` instead of `call_expression(identifier, argument_list)` |
| `Select_statements` | `println(x)` -> `type_conversion_expression(type_identifier, identifier)` instead of `call_expression(identifier, argument_list)` |
| `For_statements` | `a(v)` in range loop -> `type_conversion_expression` instead of `call_expression` |
| `Type_conversion_expressions` | `a(b)` forms -> `type_conversion_expression` where `call_expression` expected |
| `Generic_call_expressions` | `a[b[c], d](e[f])` -> last expr resolves as `call_expression` instead of `type_conversion_expression` |

### C++ (1 test)

| Test | Difference |
|------|------------|
| `static_assert_declarations` | `std::is_constructible<A>::value` -> parsed as `binary_expression` chain instead of qualified template type |

### Java (1 test)

| Test | Difference |
|------|------------|
| `type_arguments_with_generic_types` | `Collections.< T >emptyList()` -> parsed as `binary_expression(class_literal, identifier)` instead of method invocation with type arguments |

### TypeScript (1 test)

| Test | Difference |
|------|------------|
| `Classes_with_extensions` | `extends B<C>(D)` -> parsed as `binary_expression` instead of `call_expression` with type_arguments |

### Perl (1 test)

| Test | Difference |
|------|------------|
| `just_solidus_-_DOR_vs_regex` | `sum //, 2, 3` -> parsed as `bareword` instead of `ambiguous_function_call_expression(function, ...)` |

### C (1 test)

| Test | Difference |
|------|------------|
| `Call_expressions_vs_empty_declarations` | `b(a)` parsed as `macro_type_specifier` instead of `call_expression` |

**Fix approach**: This is the GLR version comparison gap (bead ums). Porting
`ts_parser__compare_versions` should resolve many of these. For Go specifically,
the type_conversion vs call ambiguity (bead nlb) may also need attention.

---

## Cluster B: GLR Timeout / Infinite Loop (8)

All C++ only. Parser gets stuck in GLR ambiguity resolution. All at exactly
10.00s (test timeout).

| Test | Notes |
|------|-------|
| `casts_vs_multiplications` | Cast vs multiply ambiguity |
| `Noreturn_Type_Qualifier` | Empty output |
| `For_loops` | Empty output |
| `Switch_statements` | Empty output |
| `Concept_definition` | Empty output |
| `Compound_literals_without_parentheses` | Empty output |
| `Template_calls` | Empty output |
| `Parameter_pack_expansions` | Empty output |

**Fix approach**: Port `ts_parser__compare_versions` for proper GLR version
pruning (bead ums). Already documented in `docs/ums-glr-timeout-analysis.md`.

---

## Cluster C: Qualified/Nested Identifier Truncation (7)

**Root cause**: When parsing `a::b::c`, our parser produces `a::b` and loses
the final `::c` segment. This is a recursive qualified_identifier construction
issue where the parser doesn't go deep enough.

### C++ (5 tests)

| Test | Difference |
|------|------------|
| `Using_declarations` | `::e::f::g` -> produces `qualified_identifier(namespace_identifier)` missing final segments |
| `Assignment` | `a::b::c = 1` -> `qualified_identifier(namespace_identifier, namespace_identifier)` missing final `identifier` |
| `Cast_operator_overload_declarations` | `A::B::operator C()` -> `qualified_identifier(namespace_identifier, namespace_identifier)` missing operator_cast |
| `Class_scope_cast_operator_overload_declarations` | Same as above, inside class body |
| `Namespaced_types` | `std::vector<int>::size_typ` -> loses `::size_typ` after template_type |
| `Nested_template_calls` | `T::template Nested<int>::type()` -> field_initializer loses `::type` path |

### Java (1 test)

| Test | Difference |
|------|------------|
| `method_references` | `foo.bar::method` -> parsed as `scoped_type_identifier` instead of `method_reference(field_access)` |

### C (1 test)

| Test | Difference |
|------|------------|
| `Call_expressions_vs_empty_declarations` | Two expressions collapsed into one |

**Fix approach**: The qualified_identifier recursion depth or the scoped
identifier resolution needs to handle arbitrary nesting. May be related to
how the GLR parser merges alternatives for the `::` operator.

---

## Cluster D: Internal Name Leaking as Root (8 intersecting with structural)

**Root cause**: The parser returns `_qq_string_content` or `_q_string_content`
as the root node instead of `source_file`. This is the NonTerminalAliasMap
gap (bead qkj).

Several of these are counted as "internal name leaking" in the existing
analysis, but they also show structural differences in the nodes below root,
so they show up in structural failure counts too.

### Perl (6 clearly affected)

| Test | Root Node Produced |
|------|-------------------|
| `''_strings` | `_q_string_content` — also truncates content (only 2 escape_sequence instead of full tree) |
| `""_strings` | `_qq_string_content` — truncates after first string, remaining are bare escape_sequence |
| `Interpolation_in_""_strings` | `_qq_string_content` — loses string structure after first entry |
| `qw()_lists` | `_q_string_content` — truncates list parsing |
| `Array_element_interpolation` | `_qq_string_content` — sub-expressions become bare nodes |
| `Hash_element_interpolation` | `_qq_string_content` — same pattern |
| `Space_skips_interpolation` | `_qq_string_content` — same pattern |
| `Indented_heredocs` | `_heredoc_delimiter` — heredoc root wrong |

**Fix approach**: Emit NonTerminalAliasMap in generated language.go (bead qkj).
This would fix the root node, and the structural differences within are likely
cascading from the wrong root.

---

## Cluster E: type_identifier vs identifier Confusion (6)

**Root cause**: In expression context, identifiers like `true_value`, `$f`,
`_abc` are parsed as `type_identifier` instead of `identifier`. This happens
because the keyword extraction or symbol resolution assigns the wrong symbol
type when the identifier could be either a type name or a value name.

### C (3 tests)

| Test | Difference |
|------|------------|
| `Identifiers` | `_abc`, `d_EG123`, `$f` -> `type_identifier` instead of `identifier` in expression_statement |
| `Common_constants` | `true_value`, `false_value`, `NULL_value` -> `type_identifier` instead of `identifier` |
| `Type_modifiers` | `unsigned v1` -> `sized_type_specifier(type_identifier)` instead of `sized_type_specifier identifier` |

### C++ (2 tests)

| Test | Difference |
|------|------------|
| `Common_constants` | Same as C: `true_value` etc. -> `type_identifier` |
| `Type_modifiers` | Same as C: `unsigned v1` -> `type_identifier` |

### Go (1 test)

| Test | Difference |
|------|------------|
| `Function_declarations` | `type_identifier` appearing with `(identifier MISSING)` in unnamed param contexts |

**Fix approach**: The issue is in how the keyword/symbol map distinguishes
type vs value contexts. In C/C++, identifiers in expression position should
be `identifier`, not `type_identifier`. This is likely a GLR ambiguity
resolution issue where the "declaration" interpretation wins over the
"expression" interpretation.

---

## Cluster F: Perl function_call vs ambiguous_function_call (6)

**Root cause**: The parser produces `ambiguous_function_call_expression` where
`function_call_expression` is expected. Perl has a GLR ambiguity between
`foo()` meaning "definitely a function call" vs `foo ARGS` meaning "ambiguous
function call". When parentheses are used (`foo()`), the reference parser
resolves to `function_call_expression`, but our port keeps it as `ambiguous`.

| Test | Difference |
|------|------------|
| `Function_call_(0_args)` | `foo()` -> `ambiguous_function_call_expression(function, stub_expression)` instead of `function_call_expression(function)` |
| `Function_call_(1_arg)` | `foo(123)` -> `ambiguous_function_call_expression` instead of `function_call_expression` |
| `Function_call_(2_args)` | `foo(12, 34)` -> `ambiguous_function_call_expression` instead of `function_call_expression` |
| `not_confused_by_leading_whitespace` | `=head1()` -> `ambiguous_function_call_expression(function, stub_expression)` instead of `function_call_expression(function)` |
| `try / catch` | `A()` inside catch block -> `ambiguous_function_call_expression` |
| `Extended_try / catch` | `A()` inside try/catch -> same pattern |
| `Defer` | `A()` inside defer block -> same pattern |

Additional sub-pattern: in `ambiguous_funcs` and `ambiguous_funcs_-_indirect_object_fakeouts`,
the tree structure differs in how arguments are grouped (precedence of list vs function call).

**Fix approach**: This is a GLR ambiguity resolution issue. The reference parser
uses dynamic precedence or version comparison to choose `function_call_expression`
when parentheses are present. Our port either lacks the dynamic precedence
scoring or the version comparison that would prune the ambiguous alternative.

---

## Cluster G: Python print_statement vs call (4)

**Root cause**: Python 2 `print` is a keyword (print_statement), Python 3
`print` is a function (call). The grammar has GLR ambiguity here. Our parser
picks `print_statement` in cases where the reference picks `call`.

| Test | Difference |
|------|------------|
| `Print_used_as_an_identifier` | `print()` -> `print_statement(tuple)` instead of `call(identifier, argument_list)` |
| `Print_used_as_an_identifier` | `print(a)` -> `print_statement(parenthesized_expression)` instead of `call` |
| `Print_statements` | `print not True` -> `comparison_operator(identifier, true)` instead of `print_statement(not_operator(true))` |
| `Matching_specific_values` | `print("Goodbye!")` inside match/case -> `print_statement(parenthesized_expression)` instead of `call` |
| `Adding_a_wild_card` | Same pattern in match/case context |

**Fix approach**: Same GLR version comparison gap. The reference parser uses
dynamic precedence to prefer `call` over `print_statement` in Python 3 mode
contexts. Our port doesn't correctly resolve this ambiguity.

---

## Cluster H: Comment Placement (3)

**Root cause**: Comments are placed inside a block/expression instead of after
it, or vice versa. This is a comment-as-extras attachment heuristic difference.

| Language | Test | Difference |
|----------|------|------------|
| C | `Comments_after_for_loops_with_ambiguities` | Comment inside `compound_statement` instead of after it |
| C++ | `Comments_after_for_loops_with_ambiguities` | Same as C |
| Bash | `Words_containing_bare_'#'` | `# comment with space` attached inside command instead of after |
| Bash | `Words_containing_#_that_are_not_comments` | `(comment)` inside string instead of as sibling |

**Fix approach**: The extras attachment algorithm differs slightly from the C
reference. May need tuning of how comments are assigned as trailing vs leading.

---

## Cluster I: Perl Non-Associative Operator Error Recovery (2)

**Root cause**: For non-associative operators (`eq`, `cmp`, `<`, `isa`),
chaining like `a eq b eq c` should produce an ERROR node for the invalid
third operand. Our parser either doesn't produce the ERROR or restructures
the tree differently.

| Test | Difference |
|------|------------|
| `EXPR_eq_EXPR_-_list/non_assoc` | `12 cmp 34 cmp 56` -> chained `equality_expression` instead of `equality_expression + ERROR(number)` |
| `EXPR_<_EXPR_-_list/non_assoc` | `12 isa 34 isa 56` -> chained `relational_expression` instead of `relational_expression + ERROR(number)` |

**Fix approach**: Error recovery for non-associative operators needs the
parser to detect the invalid chain and insert an ERROR node. This may
require the GLR comparison/pruning fix.

---

## Cluster J: Perl Sort/List Precedence (1)

| Test | Difference |
|------|------------|
| `sort_-_with_and_without_a_BLOCK` | `sort 1, 2, 3` -> `list_expression(sort_expression(1), 2, 3)` instead of `sort_expression(list_expression(1, 2, 3))` |

**Fix approach**: Precedence resolution for sort vs comma. The sort expression
should greedily capture the entire list as its argument.

---

## Cluster K: C++ Structured Binding False Positive (3)

**Root cause**: `h[i] = j` is parsed as a structured binding declaration
(`declaration(type_identifier, structured_binding_declarator)`) instead of
an assignment expression. This is a C++ declaration-vs-expression ambiguity
that our GLR resolver gets wrong.

| Test | Difference |
|------|------------|
| `Assignments` | `h[i] = j` -> `declaration(type_identifier, init_declarator(structured_binding_declarator, identifier))` |
| `pointers` | `c[i] = expr` in for loop -> `declaration(type_identifier, structured_binding_declarator)` |
| `Variadic_templates` | `func3(nullptr)` inside template method -> `declaration(type_identifier, parenthesized_declarator)` |

**Fix approach**: GLR version comparison should prefer the expression
interpretation in statement contexts where a declaration is invalid or
less likely.

---

## Cluster L: Python Splat Scope in Patterns (2)

**Root cause**: The `*` (splat) operator's scope is resolved differently.
`*a.b` is parsed as `list_splat(attribute(a, b))` by our parser but expected
as `attribute(list_splat(a), b)` by the reference.

| Test | Difference |
|------|------------|
| `Lists` | `[*a.b]` -> `list_splat(attribute(a, b))` instead of `attribute(list_splat(a), b)` |
| `Format_strings` | `expression_list(pattern_list)` vs `expression_list(identifier)` — pattern vs expression in string interpolation |

**Fix approach**: Precedence of the splat operator relative to attribute access.

---

## Cluster M: One-Off GLR/Precedence Issues (7)

These are unique issues that don't cluster:

| Language | Test | Difference |
|----------|------|------------|
| Python | `Simple_Tuples` | `(a, b, c,)` -> `assignment(tuple_pattern, type(tuple))` instead of `tuple` |
| Python | `With_statements` | Multi-line `with` -> extra `tuple` wrapper around `with_item` list |
| Python | `Function_definitions` | `_dedent` root (internal name leak) with cascading structure loss |
| Python | `Raw_strings` | `r"\\"` -> `(string_start) (string_end)` (empty) vs `(string_start) (string_content) (string_end)` (missing empty content) |
| Rust | `Line_doc_comments` | Multiple `line_comment` nodes nested instead of flat siblings |
| Lua | `comment` | Expected 19 comments but got 18 — missing one `--[[]]` empty block comment |
| Go | `Function_declarations` | `parameter_declaration(identifier MISSING)` for unnamed params in slice/map type positions |
| Ruby | `nested_unparenthesized_method_calls` | `puts get_name self, true` -> `puts` gets `(identifier, self)` instead of `(call(identifier, (self, true)))` |
| Ruby | `pattern_matching_with_fancy_string_literals` | `heredoc_body` placed inside `case_match` instead of as sibling |
| JS | `Objects` | `{ g: h }` -> `statement_block(labeled_statement)` instead of `object(pair)` |
| JS | `Reserved_words_as_identifiers` | `await: await(...)` -> `call_expression` instead of `await_expression` |
| Perl | `autoquoting_postfix` | `list_expression(return_expression(...))` precedence vs `return_expression(list_expression(...))` |
| Perl | `autoquote_edge_cases` | `unary_expression(autoquoted_bareword)` instead of just `autoquoted_bareword` |
| Perl | `Attribute_plus_signature` | `($sig)` parsed as `attribute_value` instead of `signature` |
| C++ | `Templates_with_optional_anonymous_parameters` | `qualified_identifier` loses one nesting level after template_type |
| C++ | `Noexcept_specifier` | `sizeof(T)` -> `sizeof_expression(type_descriptor)` instead of `sizeof_expression(parenthesized_expression(identifier))` |
| C++ | `template_functions_vs_relational_expressions` | `x.foo < 0 || bar >= 1` -> parsed as template instead of comparison |

---

## Prioritized Fix Impact

### Tier 1: High-impact, single fix resolves cluster

| Fix | Cluster | Tests Fixed | Effort |
|-----|---------|-------------|--------|
| GLR version comparison (ums) | B + partial A, E, F, G, K | ~20-25 | High |
| NonTerminalAliasMap (qkj) | D | ~8 | Medium |

### Tier 2: Medium-impact, targeted fixes

| Fix | Cluster | Tests Fixed | Effort |
|-----|---------|-------------|--------|
| Qualified identifier depth | C | ~7 | Medium |
| Perl function_call resolution | F | ~6 | Medium |
| C/C++ type_identifier confusion | E | ~6 | Low-Medium |
| Python print ambiguity | G | ~4 | Low |

### Tier 3: Low-impact, individual fixes

| Fix | Cluster | Tests Fixed | Effort |
|-----|---------|-------------|--------|
| Comment placement | H | ~3 | Low |
| C++ structured binding | K | ~3 | Low |
| Perl non-assoc errors | I | ~2 | Low |
| Python splat scope | L | ~2 | Low |
| One-off fixes | M | ~7 | Varies |

---

## Key Insight

The **GLR version comparison** (bead ums) is the single most impactful fix.
It directly resolves Cluster B (8 C++ timeouts) and is the likely root cause
behind the wrong-alternative selection in Clusters A, E, F, G, and K (another
~25 tests). Many of the "wrong node type" issues are symptoms of the parser
keeping the wrong GLR version alive and pruning the correct one.

The **Qualified Identifier Truncation** (Cluster C, 7 tests) was initially
thought to be a separate parsing rule issue, but investigation confirmed it
IS a GLR version comparison issue (same root cause as ums). The C reference
parser handles `a::b::c` correctly with the same grammar tables — our parser
prematurely reduces `a::b` because condenseStack picks the shorter/lower-cost
version over the deeper recursive one. Reclassified as MEDIUM confidence for
ums fix. See post-ums-expectations.md for updated predictions.

The **NonTerminalAliasMap** (bead qkj, 8 tests) is a clean, well-scoped fix
that resolves a distinct category.

---

## Addendum: wcu.19 Internal Name Leaking Investigation

**Date**: 2026-02-16 (reviewer agent)

The 12 internal-name failures (wcu.19) where hidden non-terminals appear as
root were investigated. Key findings:

### Confirmed Internal Name Root Failures

| Test | Root Produced | Expected Root |
|------|--------------|---------------|
| Perl qw()_lists | `_q_string_content` | `source_file` |
| Perl Interpolation_in_""_strings | `_qq_string_content` | `source_file` |
| Perl ''_strings | `_q_string_content` | `source_file` |
| Perl ""_strings | `_qq_string_content` | `source_file` |
| Python Function_definitions | `_dedent` | `module` |
| Ruby heredocs_in_context | `_heredoc_*` (TBD) | `program` |

### Root Cause

These are NOT display issues (qkj/NonTerminalAliasMap wouldn't help). The
parser's GLR version selection produces a tree rooted at a hidden scanner
token instead of the start symbol. Verified: C reference parser produces
correct `source_file`/`module` root with the same grammar tables.

### Possible ums Overlap

Coder-2 independently found that the ums condenseStack rewrite caused a
similar regression: Python `_newline` becoming root due to PreferRight swap.
This confirms the class of bug is version-selection-related. Some wcu.19
failures may improve with the ums fix.

### Post-UMS Assessment Needed

After ums lands, re-test all 12 internal-name failures. Those that still
fail will need scanner-specific investigation:
- Perl: External scanner string content token production
- Ruby: Heredoc scanner interactions
- Python: Indent/dedent scanner interactions
