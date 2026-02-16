# Corpus Test Failure Analysis

Generated: 2026-02-16

## Summary Table

| Language   | Total | Pass | Fail | Skip | Alias-Related | Field Annotation | Structural/Parse | Other |
|------------|-------|------|------|------|---------------|-----------------|------------------|-------|
| Go         | 67    | 1    | 66   | 0    | 38            | 7               | 19               | 2     |
| Python     | 115   | 74   | 41   | 0    | 0             | 29              | 10               | 2     |
| JavaScript | 116   | 46   | 70   | 0    | 46            | 16              | 7                | 1     |
| TypeScript | 112   | 8    | 104  | 0    | 83            | 10              | 8                | 3     |
| Lua        | 37    | 6    | 31   | 0    | 0             | 30              | 1                | 0     |

## Definitions

- **Alias-related**: Expected output uses specialized node names (`type_identifier`, `field_identifier`, `property_identifier`, `shorthand_property_identifier`, `shorthand_property_identifier_pattern`, `statement_identifier`, `label_name`, `package_identifier`, `interface_body` vs `object_type`, etc.) but our output uses the generic `identifier` or a different node name. These should be fixed by coder-3's alias sequences extraction fix.
- **Field annotation mismatch**: Expected output has field annotations (`name:`, `body:`, `value:`, `left:`, `right:`, `condition:`, `consequence:`, etc.) but our output omits them, OR the structural tree is correct but field annotations differ.
- **Structural/parse errors**: The tree structure is fundamentally different -- wrong node types, missing nodes, ERROR nodes, empty actual output, timeouts, parse failures, wrong grouping of nodes.
- **Other**: Failures that don't cleanly fit the above categories (comment placement issues, minor edge cases).

---

## Go (67 tests: 1 pass, 66 fail)

### Alias-Related: 38 failures

These are tests where the expected tree uses `type_identifier`, `field_identifier`, `package_identifier`, `label_name` etc. but our output uses `identifier`. This is the single biggest category.

**Examples:**
- `Package_clauses`: expected `(package_identifier)`, actual `(identifier)`
- `Single-line_function_declarations`: expected `(package_identifier)`, actual `(identifier)`
- `Variadic_function_declarations`: expected `(type_identifier)`, actual `(identifier)` (throughout)
- `Method_declarations`: expected `(field_identifier)` for method names, actual `(identifier)`
- `Selector_expressions`: expected `(field_identifier)`, actual `(identifier)`
- `Call_expressions`: expected `(package_identifier)`, actual `(identifier)`
- `Calls_to_'make'_and_'new'`: expected `(type_identifier)`, actual `(identifier)`
- `Generic_call_expressions`: expected `(type_identifier)`, actual `(identifier)`
- `Indexing_expressions`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Unary_expressions`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Declaration_statements`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Expression_statements`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Send_statements`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Increment/Decrement_statements`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Assignment_statements`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Short_var_declarations`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Empty_statements`: expected `(package_identifier)`, actual `(identifier)` (only alias diff)
- `Nested_control_statements`: expected `(package_identifier)` + `(field_identifier)`, actual `(identifier)`
- `Go_and_defer_statements`: expected `(package_identifier)` + `(field_identifier)`, actual `(identifier)`
- `Single_import_declarations`: expected `(package_identifier)`, actual `(identifier)`
- `Grouped_import_declarations`: expected `(package_identifier)`, actual `(identifier)`

**Also alias-related (plus additional structural issues):**
- `Function_declarations`: `type_identifier` -> `identifier`, `type_constraint` -> `type_elem`, plus some structural issues
- `For_statements`: `label_name` -> `identifier`
- `Labels_at_the_ends_of_statement_blocks`: `label_name` -> `identifier`

### Field Annotation Mismatch: 7 failures

These tests have field annotations in the expected output (`name:`, `body:`, `condition:`, etc.) that our output omits.

**Examples:**
- `If_statements`: expected has `condition:`, `consequence:`, `alternative:` annotations, actual omits them
- `Switch_statements`: expected has `value:`, `initializer:`, annotations, actual omits them
- `Type_switch_statements`: expected has `value:`, `alias:`, `type:` annotations, actual omits them
- `Select_statements`: expected has `communication:`, `channel:`, `value:`, `function:`, `field:` annotations, actual omits them

### Structural/Parse Errors: 19 failures

These have fundamentally different tree structures, empty outputs, or wrong node types.

**Examples (empty/minimal actual output -- grouped `const`/`var`/`type` blocks failing):**
- `Single_const_declarations_without_types`: actual is just `(int_literal)` -- completely wrong
- `Single_const_declarations_with_types`: actual is just `(int_literal)`
- `Grouped_const_declarations`: actual is just `)`
- `Const_declarations_with_implicit_values`: actual is just `)`
- `Grouped_var_declarations`: actual is just `)`
- `Type_declarations`: actual is just `)`
- `Int_literals`, `Float_literals`, `Rune_literals`, `Imaginary_literals`: actual is just `)` (grouped const blocks)
- `String_literals`: empty actual
- `Slice_literals`, `Array_literals_with_implicit_length`, `Map_literals`, `Struct_literals`, `Function_literals`: actual is `}`
- `Top-level_statements`: actual is `}`
- `Non-ascii_variable_names`: actual is `)` (grouped const block)
- `Type_Aliases`: actual is `]`
- `Error_detected_at_globally_reserved_keyword`: empty actual (error recovery test)

**Examples (wrong tree structure):**
- `Nested_call_expressions`: actual uses `type_conversion_expression` instead of `call_expression` for `c(d)`
- `Type_assertion_expressions`: parses `.` assertion as `type_conversion_expression` instead of `type_assertion_expression`
- `Type_conversion_expressions`: several misparses in complex expressions

### Other: 2 failures

- `Block_comments`: Block comments are not being parsed/preserved (comment dropped from tree)
- `Comments_with_asterisks`: Comment nodes not appearing in actual output

---

## Python (115 tests: 74 pass, 41 fail)

### Alias-Related: 0 failures

Python does not use identifier aliases like `type_identifier` etc., so no failures in this category.

### Field Annotation Mismatch: 29 failures

Python's tree-sitter grammar uses many field annotations (`left:`, `right:`, `condition:`, `body:`, `name:`, `value:`, `consequence:`, `alternative:`, `argument:`, etc.). Our output consistently omits these.

**Examples:**
- `Identifiers_with_Greek_letters`: expected `left:`, `right:` on assignment, actual omits
- `Operator_precedence`: expected `left:`, `right:`, `function:`, `arguments:`, `value:`, `subscript:`, `object:`, `attribute:`, actual omits
- `Control-flow_statements`: expected `condition:`, `body:`, actual omits
- `If_statements`: expected `condition:`, `consequence:`, actual omits
- `If_else_statements`: expected `condition:`, `consequence:`, `alternative:`, `body:`, actual omits
- `Nested_if_statements`: expected `condition:`, `consequence:`, `alternative:`, `body:`, actual omits
- `While_statements`: expected `condition:`, `body:`, `alternative:`, actual omits
- `For_statements`: expected `left:`, `right:`, `body:`, `alternative:`, `argument:`, actual omits
- `Try_statements`: expected `body:`, `value:`, `alias:`, actual omits
- `With_statements`: expected `value:`, `alias:`, actual omits but also structural differences
- `Async_Function_definitions`: expected `name:`, `parameters:`, `body:`, `type:`, `return_type:`, `pattern:`, actual omits
- `Function_definitions`: expected `name:`, `parameters:`, `body:`, etc., actual omits
- `Async_context_managers_and_iterators`: expected `value:`, `alias:`, `body:`, `left:`, `right:`, actual omits

### Structural/Parse Errors: 10 failures

**Examples:**
- `An_error_before_a_string_literal`: empty actual output (error test)
- `Print_used_as_an_identifier`: `print` parsed as `print_statement` instead of function call `(identifier)`
- `Exec_used_as_an_identifier`: `exec` missing as function name in call
- `Async_/_await_used_as_identifiers`: `async`/`await` not parsed as identifiers
- `A_function_called_match`: `match` not parsed as function name
- `Match_kwargs_2`: severely wrong parse
- `Match_is_match_but_not_pattern_matching`: `match` not treated as identifier, `[match]` drops the match content
- `Actually_not_match`: `match` keyword handling wrong
- `Strings`: empty/partial actual output
- `Empty_blocks`: missing comment nodes, wrong structure

### Other: 2 failures

- `Comments`: comment placement relative to statements
- `Comments_after_dedents`: comment attaches to wrong node
- `Comments_at_the_ends_of_indented_blocks`: comment placement difference
- `Escaped_newline`: `line_continuation` placement within argument_list vs outside
- Several `list_splat` vs `list_splat_pattern` differences (Tuples_with_splats, Splat_Inside_of_Expression_List)
- `Format_strings_with_format_specifiers`: `format_expression` vs `interpolation` node name in format specifier contexts
- Format strings: `expression_list` vs `pattern_list`

---

## JavaScript (116 tests: 46 pass, 70 fail)

### Alias-Related: 46 failures

JavaScript heavily uses `property_identifier`, `shorthand_property_identifier`, `shorthand_property_identifier_pattern`, and `statement_identifier`. Our output uses `identifier` for all of these.

**Examples:**
- `Object_destructuring_assignments`: expected `shorthand_property_identifier_pattern`, `property_identifier`, actual `identifier`
- `Object_destructuring_parameters`: expected `shorthand_property_identifier_pattern`, actual `identifier`
- `Array_destructuring_assignments`: expected `property_identifier`, actual `identifier`
- `Object_destructuring_patterns_w/_default_values`: expected `property_identifier`, `shorthand_property_identifier_pattern`, actual `identifier`
- `Template_strings`: expected `property_identifier`, actual `identifier`
- `Objects`: expected `property_identifier`, actual `identifier`
- `Objects_with_shorthand_properties`: expected `shorthand_property_identifier`, actual `identifier` (ALSO missing `get` identifier)
- `Objects_with_method_definitions`: expected `property_identifier`, actual `identifier`
- `Objects_with_reserved_words_for_keys`: expected `property_identifier`, actual `identifier`
- `Classes_with_reserved_words_as_methods`: expected `property_identifier`, actual `identifier`
- `Property_access`: expected `property_identifier`, actual `identifier`
- `Chained_Property_access`: expected `property_identifier`, actual `identifier`
- `Chained_callbacks`: expected `property_identifier`, actual `identifier`
- `Function_calls`: expected `property_identifier`, actual `identifier`
- `Optional_chaining_property_access`: expected `property_identifier`, actual `identifier`
- `Optional_function_calls`: expected `property_identifier`, actual `identifier`
- `Constructor_calls`: expected `property_identifier`, actual `identifier`
- `Async_Functions_and_Methods`: expected `property_identifier`, actual `identifier`
- `Assignments`: expected `property_identifier`, actual `identifier` (also `async` keyword handling)
- `The_comma_operator`: expected `property_identifier`, actual `identifier`
- `Ternaries`: expected `property_identifier`, actual `identifier`
- `The_delete_operator`: expected `property_identifier`, actual `identifier`
- `Augmented_assignments`: expected `property_identifier`, actual `identifier` (also `async` keyword)
- `Operator_precedence`: expected `property_identifier`, actual `identifier`
- `Simple_JSX_elements`: expected `property_identifier`, actual `identifier`
- `Expressions_within_JSX_elements`: expected `property_identifier`, actual `identifier`
- `Forward_slashes_after_parenthesized_expressions`: expected `property_identifier`, actual `identifier`
- `Yield_expressions`: expected `property_identifier`, actual `identifier`
- `JSX`: expected `property_identifier`, actual `identifier`
- `JSX#01`: expected `shorthand_property_identifier`, actual `identifier`
- `Unicode_identifiers`: expected `property_identifier`, actual `identifier`
- `JSX_strings_with_unescaped_newlines_for_TSX_attributes`: expected `property_identifier`, actual `identifier`
- `property_access_across_lines`: expected `property_identifier`, actual `identifier`
- `Multi-line_chained_expressions_in_var_declarations`: expected `property_identifier`, actual `identifier`
- `Comments`: expected `property_identifier`, actual `identifier`
- `Comments_between_statements`: expected `property_identifier`, actual `identifier`
- `Comments_with_asterisks`: expected `property_identifier`, actual `identifier`
- `Imports`: expected `property_identifier`, actual `identifier`
- `Exports`: expected `property_identifier`, actual `identifier`
- `Hash_bang_lines`: expected `property_identifier`, actual `identifier`
- `U+2028_as_a_line_terminator`: expected `property_identifier`, actual `identifier`
- `Labeled_statements`: expected `statement_identifier`, actual `identifier`

### Field Annotation Mismatch: 16 failures

**Examples:**
- `If_statements`: expected `condition:`, `consequence:`, `function:`, `arguments:`, actual omits
- `If-else_statements`: expected `condition:`, `consequence:`, `alternative:`, actual omits
- `For_statements`: expected `initializer:`, `condition:`, `increment:`, `body:`, actual omits
- `For-in_statements`: expected `left:`, `right:`, actual omits
- `For-of_statements`: expected field annotations, actual omits
- `While_statements`: expected `condition:`, `body:`, actual omits
- `Do_statements`: expected `body:`, `condition:`, actual omits
- `With_statements`: expected `object:`, `body:`, actual omits
- `Exports`: expected `name:`, `alias:`, `declaration:`, `value:`, `source:`, actual omits
- `Decorators_before_exports`: expected `decorator:`, `declaration:`, `name:`, actual omits
- `JSX`: expected `open_tag:`, `close_tag:`, `name:`, `attribute:`, `property:`, `object:`, actual omits

### Structural/Parse Errors: 7 failures

**Examples:**
- `Extra_complex_literals_in_expressions`: empty actual (error recovery test)
- `Classes`: empty actual output (parsing failure)
- `Class_Property_Fields`: empty actual output
- `Private_Class_Property_Fields`: empty actual output
- `Class_Decorators`: empty actual output
- `JSX_with_HTML_character_references_(entities)`: empty actual output (html_character_reference handling)
- `Non-breaking_spaces_as_whitespace`: empty actual output (zero-width character handling)
- `Reserved_words_as_identifiers`: `await` not parsed as identifier, heavily wrong tree

### Other: 1 failure

- `operator_expressions_split_across_lines`: comment placement difference
- `Return_statements`: `async` not treated as identifier in `return async`
- `Arrow_functions`: `async` keyword/identifier handling, set/kv arg loss
- Several tests have compound issues (alias + keyword handling)

---

## TypeScript (112 tests: 8 pass, 104 fail)

### Alias-Related: 83 failures

TypeScript is the most heavily affected by alias issues. Nearly every test uses `type_identifier`, `property_identifier`, or `interface_body` (which we output as `object_type`). This is pervasive.

**Key alias patterns:**
- `type_identifier` -> `identifier`: Appears in nearly every test that involves type declarations, class names, interface names, generic type parameters, type aliases, etc.
- `property_identifier` -> `identifier`: Appears in class fields, method names, object properties, enum members, etc.
- `interface_body` -> `object_type`: All interface declarations use `interface_body` in expected, `object_type` in actual
- `nested_identifier ... property` -> `nested_identifier ... identifier`: module paths

**Examples:**
- `Ambient_declarations`: `type_identifier` -> `identifier` for class names, `property_identifier` -> `identifier` for fields
- `Flow-style_ambient_class_declarations_with_commas`: `type_identifier` + `interface_body` -> `identifier` + `object_type`
- `Ambient_exports`: `shorthand_property_identifier` -> `identifier`, `type_identifier` -> `identifier`
- `Typeof_types`: `type_identifier` -> `identifier`, `property_identifier` -> `identifier`
- `Property_signatures_with_accessibility_modifiers`: `type_identifier` -> `identifier`, `property_identifier` -> `identifier`
- `Ambient_type_declarations`: `type_identifier` -> `identifier`
- `Ambient_module_declarations`: `type_identifier` -> `identifier`
- `Type_casts`: `type_identifier` -> `identifier`
- `Classes_with_method_signatures`: `type_identifier` -> `identifier`, `property_identifier` -> `identifier`
- `Classes_with_generic_parameters`: `type_identifier` -> `identifier`
- `Top-level_exports`: `type_identifier` -> `identifier`, `interface_body` -> `object_type`
- `Index_type_queries`: `type_identifier` -> `identifier`
- `Type_alias_declarations`: `type_identifier` -> `identifier`, `property_identifier` -> `identifier`
- `Enum_declarations`: `property_identifier` -> `identifier`
- `Interface_declarations`: `type_identifier` -> `identifier`, `property_identifier` -> `identifier`, `interface_body` -> `object_type`
- All `Generic_types`, `Conditional_types`, `Template_literal_types`, `Mapped_types`, `Lookup_types`, etc.

### Field Annotation Mismatch: 10 failures

**Examples:**
- `Ambient_declarations`: expected `name:`, `body:`, `pattern:`, `type:`, etc., actual omits
- `Exception_handling`: expected `body:`, `parameter:`, `type:`, actual omits
- `Abstract_classes`: expected `name:`, `body:`, `parameters:`, `return_type:`, `argument:`, `property:`, actual omits
- `Override_modifier`: expected `name:`, `body:`, `parameters:`, `return_type:`, actual omits
- `Functions_with_typed_parameters`: expected `name:`, `parameters:`, `body:`, `type_parameters:`, `return_type:`, `pattern:`, `type:`, actual omits
- `Arrow_functions_and_generators_with_call_signatures`: expected `name:`, `parameters:`, `body:`, `return_type:`, `type_parameters:`, actual omits
- `Super`: expected `name:`, `body:`, `value:`, `function:`, `arguments:`, `object:`, `property:`, actual omits

### Structural/Parse Errors: 8 failures

**Examples:**
- `Type_arguments_in_JSX`: empty actual output
- `Typeof_expressions`: `typeof module` loses the `module` identifier
- `Variable_named_'module'`: `module;` expression statement loses the identifier
- `The_'less_than'_operator`: `type.length` and `string.length` lose the `.length` member access
- `Subscript_expressions_in_if_statements`: `set[1]` loses the `set` identifier (keyword conflict)
- `Objects_with_reserved_words_as_keys`: `public:`, `private:`, `readonly:`, `static:` -- reserved words not usable as property keys
- `Classes_with_extensions`: `extends B<C>(D)` parsed as binary+parenthesized instead of generic call
- `Arrow_function_with_parameter_named_async`: `async => async` produces empty arrow function body
- `Read-only_arrays`: `readonly (readonly a[]) []` misparses as function_type

### Other: 3 failures

- `Accessibility_modifiers_as_pair_keywords`: `private` used as object key produces `(pair (identifier))` instead of `(pair (property_identifier) (identifier))`
- `Ambiguity_between_function_signature_and_function_declaration: comments_and_newlines`: comment placement
- `Indexed_Access_Precedence`: missing comment node
- Several keyword-as-identifier conflicts (`module`, `set`, `async`, `string`, `type`)

---

## Lua (37 tests: 6 pass, 31 fail)

### Alias-Related: 0 failures

Lua does not use identifier aliases. All identifiers are just `identifier` in both expected and actual.

### Field Annotation Mismatch: 30 failures

Lua's tree-sitter grammar uses extensive field annotations (`name:`, `arguments:`, `table:`, `field:`, `content:`, `left:`, `right:`, `operand:`, `condition:`, `body:`, `consequence:`, `alternative:`, `clause:`, `method:`, `value:`, `parameters:`, `local_declaration:`, `attribute:`, `start:`, `end:`, `step:`, etc.).

Our output consistently omits ALL field annotations. Additionally, Lua uses `(string content: (string_content))` but we sometimes produce `(string)` without `(string_content)` for quoted strings.

**Every failing Lua test** has field annotation differences. In most cases, the structural tree is correct but all field labels are missing.

**Examples:**
- `nil`, `false`, `true`, `number`, `vararg_expression`: correct structure but `name:` and `arguments:` missing
- `function_definition`: correct structure but `parameters:`, `body:`, `name:`, `value:` missing
- `variable_:::_identifier`: correct structure but `name:`, `value:` missing
- `variable_:::_bracket_index_expression`: correct but `table:`, `field:`, `name:`, `value:` missing
- `binary_expression`, `unary_expression`: correct but `left:`, `right:`, `operand:` missing
- `if_statement`: correct but `condition:`, `consequence:`, `alternative:` missing
- `for_statement`: correct but `clause:`, `body:`, `name:`, `start:`, `end:`, `step:` missing
- `table_constructor`: correct but `name:`, `value:`, `local_declaration:` missing
- `variable_declaration`: correct but `name:`, `value:`, `local_declaration:`, `attribute:` missing

### Structural/Parse Errors: 1 failure

- `comment`: Block comments (`--[[]]`, `--[[ ... ]]`) are not being parsed correctly. The parser is treating the entire block as arguments to a function call rather than as comment nodes. Single-line comments also have issues.

### Other: 0 failures

---

## Key Takeaways

### 1. Alias-Related Issues (coder-3's fix will address)

**Languages affected: Go (38), JavaScript (46), TypeScript (83)**

Total: ~167 failures across 3 languages. These are tests where our parser outputs `identifier` but the expected tree uses specialized alias names. This is the single largest category of failures and coder-3's alias sequences extraction fix should resolve the vast majority.

Key alias mappings needed:
- Go: `package_identifier`, `type_identifier`, `field_identifier`, `label_name`
- JS: `property_identifier`, `shorthand_property_identifier`, `shorthand_property_identifier_pattern`, `statement_identifier`
- TS: `type_identifier`, `property_identifier`, `interface_body` (vs `object_type`), `nested_type_identifier`

### 2. Field Annotation Mismatches

**Languages affected: All five -- Go (7), Python (29), JavaScript (16), TypeScript (10), Lua (30)**

Total: ~92 failures. Our parser does not emit field annotations. This is a separate issue from aliases and will need its own fix. Lua and Python are especially impacted since they don't have alias issues but do have heavy field annotation usage.

### 3. Structural/Parse Errors

**Languages affected: All five -- Go (19), Python (10), JavaScript (7), TypeScript (8), Lua (1)**

Total: ~45 failures. These represent actual parsing bugs:
- **Go**: Grouped declarations (const/var/type blocks with parentheses) consistently fail to parse
- **Go**: Block comments dropped, some complex type/call expression ambiguities
- **Python**: `print`/`exec`/`match`/`async`/`await` as identifiers (keyword reservation conflicts)
- **JavaScript**: Class bodies, decorators, private fields produce empty output
- **TypeScript**: Reserved word conflicts (`module`, `set`, `async`, `string`, `type`), JSX with type args
- **Lua**: Block comment parsing failure

### 4. Priority for Fixes

1. **Alias sequences** (coder-3): Will fix ~167 test failures (38% of all failures)
2. **Field annotations**: Will fix ~92 test failures (21% of all failures)
3. **Grouped declaration parsing (Go)**: Will fix ~14 Go structural failures
4. **Keyword-as-identifier handling**: Will fix ~15 failures across Python/JS/TS
5. **Comment handling**: Will fix ~10 failures across all languages
6. **Class/decorator parsing (JS)**: Will fix ~5 JS failures
