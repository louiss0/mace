# Mace Language Specification

## Status and conformance

Mace is a deterministic, strongly typed configuration language. A conforming
processor produces either a fully valid data record or schema metadata, or an
error; it never emits a partial result. Source is UTF-8 and string values may
contain Unicode. Mace has no general `null` runtime value. The `null` literal
is permitted only to initialize a `nullable` variable; it is not an ordinary
value that can be emitted, stored in records or arrays, or compared.

This RFC is normative. **[`mace.ebnf`](.&#x2f;mace.ebnf) is the canonical grammar
source.** The grammar excerpt below is an exact copy of that file and MUST
remain synchronized with it. EBNF describes syntax; constraints explicitly
marked as semantic are also normative.

A file has zero or one script block followed by exactly one record-shaped output
block. Output arrays are not document roots, though arrays may be field values.
Variables are immutable, explicitly typed, and initialized. Types, schemas and
choices are metadata, not runtime values; documentation is metadata and cannot
affect evaluation.

## Lexical rules

Identifiers are case-sensitive and consist of a Unicode letter followed by zero
or more Unicode letters, Unicode decimal digits, or `_`. Casing styles are
non-normative: tools may warn about unusual casing, but must accept every
identifier satisfying that lexical rule. `is` is a reserved keyword and is
never split from a containing identifier such as `island` or `isReady`.

Line endings are LF, CRLF, or CR; CRLF is one newline. `&#x2f;&#x2f;` comments run to a
line ending or EOF. `&#x2f;*` comments run to the next `*&#x2f;`, may span lines, and do
not require surrounding newlines. Double-quoted and block strings interpolate
with exactly `$(expression)`; single-quoted strings do not interpolate.

A `path_literal` is a non-interpolated, single-line double-quoted path. It is
not a general string and may not be a block string. Import, `schema_file`, and
`parse_file` use it exclusively.

## Canonical grammar

```ebnf
(* Mace canonical grammar. Semantic constraints are marked in comments. *)

(* LEXICAL STRUCTURE *)
whitespace = &quot; &quot; | &quot;\t&quot; | newline ;
newline = &quot;\r\n&quot; | &quot;\n&quot; | &quot;\r&quot; ;
line_comment = &quot;&#x2f;&#x2f;&quot; , { ? any character except a line terminator ? } ;
block_comment = &quot;&#x2f;*&quot; , { ? any character sequence not containing &quot;*&#x2f;&quot; ? } , &quot;*&#x2f;&quot; ;
comment = line_comment | block_comment ;
ws0 = { whitespace | comment } ;
ws1 = ( whitespace | comment ) , ws0 ;

unicode_letter = ? any Unicode character in category Letter ? ;
unicode_digit = ? any Unicode character in category Number, decimal digit ? ;
digit = &quot;0&quot;…&quot;9&quot; ;
hex_digit = digit | &quot;a&quot;…&quot;f&quot; | &quot;A&quot;…&quot;F&quot; ;
identifier = unicode_letter , { unicode_letter | unicode_digit | &quot;_&quot; } ;

inline_description = &quot;&#x2f;#&quot; , ws0 , description_text ;
description_text = ? text up to, but not including, a comma or line terminator ? ;

single_string = &quot;&#x27;&quot; , { single_character } , &quot;&#x27;&quot; ;
double_string = &#x27;&quot;&#x27; , { double_part } , &#x27;&quot;&#x27; ;
block_string = &#x27;&quot;&quot;&quot;&#x27; , { block_part } , &#x27;&quot;&quot;&quot;&#x27; ;
string_literal = single_string | double_string | block_string ;
single_character = ? any character except &#x27;, a line terminator, or backslash ? | escape_sequence ;
double_part = ? any character except &quot;, a line terminator, backslash, or the start of &quot;$(&quot; ? | escape_sequence | interpolation ;
block_part = ? any character sequence not containing &#x27;&quot;&quot;&quot;&#x27; or the start of &quot;$(&quot; ? | escape_sequence | interpolation ;
escape_sequence = &quot;\\&quot; , ( &quot;\\&quot; | &quot;&#x27;&quot; | &#x27;&quot;&#x27; | &quot;n&quot; | &quot;r&quot; | &quot;t&quot; | unicode_escape ) ;
unicode_escape = &quot;u&quot; , hex_digit , hex_digit , hex_digit , hex_digit
               | &quot;U&quot; , hex_digit , hex_digit , hex_digit , hex_digit , hex_digit , hex_digit , hex_digit , hex_digit ;
(* A path literal is single-line double-quoted text with no escapes or interpolation. *)
path_literal = &#x27;&quot;&#x27; , { path_character } , &#x27;&quot;&#x27; ;
path_character = ? any character except &quot;, a line terminator, or the start of &quot;$(&quot; ? ;

int_literal = digit , { digit } ;
float_literal = digit , { digit } , &quot;.&quot; , digit , { digit } ;
hex_int_literal = ( &quot;0x&quot; | &quot;0X&quot; ) , hex_digit , { hex_digit } ;
hex_float_literal = ( &quot;0x&quot; | &quot;0X&quot; ) , hex_digit , { hex_digit } , &quot;.&quot; , hex_digit , { hex_digit } ;
boolean_literal = &quot;true&quot; | &quot;false&quot; ;
null_literal = &quot;null&quot; ;

(* DOCUMENT STRUCTURE *)
mace_file = ws0 , [ script_block , ws0 ] , output_block , ws0 ;
script_block = script_delimiter , ws0 , { import_declaration , ws0 } , { declaration , ws0 } , script_delimiter ;
script_delimiter = &quot;|&quot; , &quot;=&quot; , &quot;=&quot; , &quot;=&quot; , { &quot;=&quot; } , &quot;|&quot; ;
(* The opening and closing script_delimiter must have the same number of &#x27;=&#x27; characters. *)

import_declaration = &quot;from&quot; , ws1 , path_literal , ws1 , &quot;import&quot; , ws1 , import_list , ws0 , &quot;;&quot; ;
import_list = imported_identifier , { ws0 , &quot;,&quot; , ws0 , imported_identifier } ;
imported_identifier = identifier , [ ws0 , &quot;:&quot; , ws0 , identifier ] ;

declaration = variable_declaration | type_declaration | schema_declaration
            | gen_doc_declaration | schema_doc_declaration ;
variable_declaration = [ &quot;nullable&quot; , ws1 ] , type_reference , ws1 , identifier , ws0 , &quot;=&quot; , ws0 , expression , [ ws0 , inline_description ] , ws0 , &quot;;&quot; ;
type_declaration = &quot;type&quot; , ws1 , identifier , ws0 , &quot;:&quot; , ws0 , type_reference , [ ws0 , inline_description ] , ws0 , &quot;;&quot; ;
schema_declaration = &quot;schema&quot; , ws1 , identifier , ws0 , &quot;:&quot; , ws0 , record_type , ws0 , &quot;;&quot; ;

gen_doc_declaration = &quot;gen_doc&quot; , ws1 , identifier , ws0 , &quot;{&quot; , ws0 , [ gen_doc_entry , { ws0 , &quot;,&quot; , ws0 , gen_doc_entry } , [ ws0 , &quot;,&quot; ] ] , ws0 , &quot;}&quot; , ws0 , &quot;;&quot; ;
gen_doc_entry = &quot;summary&quot; , ws0 , &quot;:&quot; , ws0 , string_literal
              | &quot;description&quot; , ws0 , &quot;:&quot; , ws0 , block_string ;
schema_doc_declaration = &quot;schema_doc&quot; , ws1 , identifier , ws0 , &quot;{&quot; , ws0 , [ schema_doc_entry , { ws0 , &quot;,&quot; , ws0 , schema_doc_entry } , [ ws0 , &quot;,&quot; ] ] , ws0 , &quot;}&quot; , ws0 , &quot;;&quot; ;
schema_doc_entry = &quot;summary&quot; , ws0 , &quot;:&quot; , ws0 , string_literal
                 | &quot;description&quot; , ws0 , &quot;:&quot; , ws0 , block_string
                 | &quot;fields&quot; , ws0 , &quot;:&quot; , ws0 , &quot;{&quot; , ws0 , [ documentation_field , { ws0 , &quot;,&quot; , ws0 , documentation_field } , [ ws0 , &quot;,&quot; ] ] , ws0 , &quot;}&quot; ;
documentation_field = identifier , ws0 , &quot;:&quot; , ws0 , string_literal ;

(* TYPES *)
type_reference = primitive_type | array_type | record_map_type | record_type | fusion_type | variant_type | choice_type | identifier ;
primitive_type = &quot;string&quot; | &quot;int&quot; | &quot;float&quot; | &quot;hex_int&quot; | &quot;hex_float&quot; | &quot;boolean&quot; ;
array_type = &quot;array&quot; , ws0 , &quot;&lt;&quot; , ws0 , schema_type_reference , ws0 , &quot;&gt;&quot; ;
record_map_type = &quot;record&quot; , ws0 , &quot;&lt;&quot; , ws0 , schema_type_reference , ws0 , &quot;&gt;&quot; ;
record_type = &quot;{&quot; , ws0 , [ schema_field , { ws0 , &quot;,&quot; , ws0 , schema_field } , [ ws0 , &quot;,&quot; ] ] , ws0 , &quot;}&quot; ;
schema_field = identifier , [ &quot;?&quot; ] , ws0 , &quot;:&quot; , ws0 , schema_type_reference , [ ws0 , inline_description ] ;
schema_type_reference = type_reference | self_type_reference ;
self_type_reference = &quot;$self&quot; ;
fusion_type = &quot;fusion&quot; , ws0 , &quot;[&quot; , ws0 , type_reference , { ws0 , &quot;,&quot; , ws0 , type_reference } , [ ws0 , &quot;,&quot; ] , ws0 , &quot;]&quot; ;
variant_type = &quot;variant&quot; , ws0 , &quot;[&quot; , ws0 , type_reference , { ws0 , &quot;,&quot; , ws0 , type_reference } , [ ws0 , &quot;,&quot; ] , ws0 , &quot;]&quot; ;
choice_type = &quot;choice&quot; , ws0 , &quot;[&quot; , ws0 , choice_member , { ws0 , &quot;,&quot; , ws0 , choice_member } , [ ws0 , &quot;,&quot; ] , ws0 , &quot;]&quot; ;
choice_member = string_literal | int_literal | float_literal | hex_int_literal | hex_float_literal | boolean_literal | identifier ;

(* OUTPUT *)
output_block = [ output_directive_list , ws0 , [ block_string , ws0 ] ] , &quot;{&quot; , ws0 , [ output_field , { ws0 , &quot;,&quot; , ws0 , output_field } , [ ws0 , &quot;,&quot; ] ] , ws0 , &quot;}&quot; ;
output_directive_list = &quot;[&quot; , ws0 , directive_pair , { ws0 , &quot;,&quot; , ws0 , directive_pair } , ws0 , &quot;]&quot; ;
directive_pair = &quot;output&quot; , ws0 , &quot;=&quot; , ws0 , ( &quot;data&quot; | &quot;schema&quot; )
               | &quot;schema&quot; , ws0 , &quot;=&quot; , ws0 , identifier
               | &quot;schema_file&quot; , ws0 , &quot;=&quot; , ws0 , path_literal
               | &quot;parse&quot; , ws0 , &quot;=&quot; , ws0 , identifier
               | &quot;parse_file&quot; , ws0 , &quot;=&quot; , ws0 , path_literal ;
(* The output directive controls output_field interpretation: data uses data_output_field;
   schema uses schema_output_field. This contextual distinction is semantic. *)
output_field = data_output_field | schema_output_field ;
data_output_field = identifier , ws0 , &quot;:&quot; , ws0 , expression , [ ws0 , inline_description ]
                  | identifier , [ ws0 , inline_description ] ;
schema_output_field = identifier , [ &quot;?&quot; ] , ws0 , &quot;:&quot; , ws0 , type_reference , 
[ ws0 , inline_description ] ;

(* EXPRESSIONS *)
expression = conditional_expression ;
conditional_expression = coalescing_expression , [ ws0 , &quot;?&quot; , ws0 , expression , ws0 , &quot;:&quot; , ws0 , conditional_expression ] ;
coalescing_expression = logical_or_expression , [ ws0 , &quot;??&quot; , ws0 , coalescing_expression ] ;
logical_or_expression = logical_and_expression , { ws0 , &quot;||&quot; , ws0 , logical_and_expression } ;
logical_and_expression = bitwise_or_expression , { ws0 , &quot;&amp;&amp;&quot; , ws0 , bitwise_or_expression } ;
bitwise_or_expression = bitwise_xor_expression , { ws0 , &quot;|&quot; , ws0 , bitwise_xor_expression } ;
bitwise_xor_expression = bitwise_and_expression , { ws0 , &quot;^&quot; , ws0 , bitwise_and_expression } ;
bitwise_and_expression = merge_expression , { ws0 , &quot;&amp;&quot; , ws0 , merge_expression } ;
merge_expression = equality_expression , { ws0 , &quot;&lt;&gt;&quot; , ws0 , equality_expression } ;
equality_expression = type_test_expression , { ws0 , ( &quot;==&quot; | &quot;!=&quot; ) , ws0 , type_test_expression } ;
type_test_expression = relational_expression , [ ws1 , &quot;is&quot; , ws1 , type_reference ] ;
(* Type tests are non-chainable; their right operand is a type reference, not an expression. *)
relational_expression = shift_expression , { ws0 , ( &quot;&lt;&quot; | &quot;&lt;=&quot; | &quot;&gt;&quot; | &quot;&gt;=&quot; ) , ws0 , shift_expression } ;
shift_expression = additive_expression , { ws0 , ( &quot;&lt;&lt;&quot; | &quot;&gt;&gt;&quot; | &quot;&gt;&gt;&gt;&quot; ) , ws0 , additive_expression } ;
additive_expression = multiplicative_expression , { ws0 , ( &quot;+&quot; | &quot;-&quot; ) , ws0 , multiplicative_expression } ;
multiplicative_expression = exponent_expression , { ws0 , ( &quot;*&quot; | &quot;&#x2f;&quot; | &quot;%&quot; ) , ws0 , exponent_expression } ;
exponent_expression = unary_expression , [ ws0 , &quot;**&quot; , ws0 , exponent_expression ] ;
unary_expression = ( &quot;!&quot; | &quot;~&quot; | &quot;+&quot; | &quot;-&quot; ) , ws0 , unary_expression | postfix_expression ;
postfix_expression = primary_atom , { postfix_suffix } ;
primary_atom = identifier | self_reference | parsed_input_reference | int_literal | float_literal | hex_int_literal | hex_float_literal | string_literal | boolean_literal | null_literal | array_literal | record_literal | grouped_expression ;
postfix_suffix = ws0 , ( &quot;.&quot; | &quot;?.&quot; ) , ws0 , identifier ;
self_reference = &quot;$self&quot; , &quot;.&quot; , identifier , { &quot;.&quot; , identifier } ;
parsed_input_reference = &quot;$&quot; , identifier ;
grouped_expression = &quot;(&quot; , ws0 , expression , ws0 , &quot;)&quot; ;
(* Grouped expressions are restricted by static semantics to operator-precedence control. *)
array_literal = &quot;[&quot; , ws0 , [ expression , { ws0 , &quot;,&quot; , ws0 , expression } , [ ws0 , &quot;,&quot; ] ] , ws0 , &quot;]&quot; ;
record_literal = &quot;{&quot; , ws0 , [ record_field , { ws0 , &quot;,&quot; , ws0 , record_field } , [ ws0 , &quot;,&quot; ] ] , ws0 , &quot;}&quot; ;
record_field = identifier , ws0 , &quot;:&quot; , ws0 , expression , [ ws0 , inline_description ]
             | identifier , [ ws0 , inline_description ] ;

interpolation = &quot;$&quot; , &quot;(&quot; , expression , &quot;)&quot; ;
```

## Output directives and fields

A directive list is optional; without it, output is data. When brackets are
written they MUST include exactly one `output` directive. Unknown or duplicate
directives are errors.

| Output mode | `schema` | `schema_file` | `parse` | `parse_file` |
| --- | --- | --- | --- | --- |
| omitted &#x2f; `output = 'data'` | allowed | allowed | allowed | allowed |
| `output = 'schema'` | forbidden | forbidden | forbidden | forbidden |

In data mode, `schema` selects the schema used to validate the produced output,
and `parse` selects the schema used to validate host runtime input. These roles
are distinct. `schema` and `schema_file` may be combined; `parse` and
`parse_file` may be combined. A file directive contributes declarations from
its file. Without `schema`, a `schema_file` directive uses the referenced
file's schema output body as the active output shape; when both directives are
present, `schema` selects the named schema.

In schema mode the same `{ ... }` output body is interpreted as schema fields:
its right sides are type references and `?` is permitted. In data mode its right
sides are expressions; shorthand (`name`) is permitted only in this mode and
`?` is forbidden. This interpretation is selected by the output directive, not
by a second brace syntax. Schema output cannot contain variable declarations.

Every record, schema, documentation, and output field list uses commas. A
single trailing comma before `}` is allowed. Semicolons terminate script
declarations only. A missing separator diagnostic MUST say `expected &#x27;,&#x27; after
field` or `expected &#x27;,&#x27; or &#x27;}&#x27; after field`, never suggest `;`.

## Types and static semantics

The primitive types are `string`, `int`, `float`, `hex_int`, `hex_float`, and
`boolean`. `int` is signed 64-bit. `float` is IEEE-754 binary64. `hex_int` has
the same signed 64-bit range as `int` and `hex_float` uses binary64 storage,
but both hexadecimal types are distinct from the decimal family. NaN and
infinity have no literal syntax. Integer overflow, division by zero, and any
operation producing a non-finite float are evaluation errors. `int &#x2f; int`
truncates toward zero. Hexadecimal fractional digits are powers of 16, so
`0xA.F` is `10.9375`; hexadecimal scientific notation is not supported.
Decimal and hexadecimal values never mix implicitly. `hex_int &#x2f; hex_int`
returns `hex_float`; `~` accepts only decimal `int`.

Arrays are homogeneous unless their element type is a permitting `variant`.
`record&lt;T&gt;` describes a record whose values conform to `T`. `[]` requires an expected array type because it has no inferable element type.
`{}` is an empty record and requires an expected empty, all-optional, compatible
inline, or otherwise known record type. Every empty collection literal in a
data output requires an output shape supplied through `schema` or `schema_file`,
even when another conditional branch has a known collection type. Empty schemas
are valid.

Conditional branch types are inferred recursively. Branches of the same type
produce that type. Different branch types produce a flattened, deduplicated
`variant[...]` containing every type returned by nested conditionals, and the
receiving variable or output schema MUST accept every inferred member. A schema
and a populated `record&lt;T&gt;` remain distinct variant members.

When a conditional combines an empty array or empty record with a different
branch type, the empty collection has no inferable member type. In a data output,
that conditional MUST be validated by an output shape supplied through the
`schema` or `schema_file` directive. In a variable initializer, the variable
declaration MUST use an explicit type that supplies the collection type. For
example:

```mace
|===|
boolean enabled = true;
variant[string, array<string>] value = enabled ? "configured" : [];
schema Result: { value: variant[string, array<string>], };
|===|
[output = 'data', schema = Result]
{ value: enabled ? "configured" : [], }
```

An untyped data-output conditional such as `{ value: enabled ? "configured" : [], }`
is a static error. Direct and nested empty collections in schema-less data
outputs are also static errors, including conditionals between a known
`record&lt;T&gt;` value and `{}`.

A `record_type` is valid wherever a type reference is accepted, including type
aliases, arrays, schema fields, variants, and fusions. Schemas are closed:
required fields must be present, optional fields may be omitted, and unknown
fields are rejected. Optionality is a record-shape property only; it cannot
annotate a runtime record or data-output field.

A fusion member must resolve to a record type: a named schema, a type alias to
a record, an inline record type, or another valid fusion. For equal field names,
equal resolved types are compatible. Required plus optional yields required;
optional plus optional yields optional. Different resolved types conflict. A
fusion never silently creates a variant.

Choice members are scalar literals or identifiers resolving to choices. A
literal repeated directly within one choice is an error. Values repeated only
through composed aliases are deduplicated in the resolved domain. Choice alias
cycles are errors.

Variant identity is the unordered set of resolved members, so member order does
not matter and duplicate equivalent members are invalid. A value normally must
match exactly one member and provable overlap should be rejected. The sole
explicit overlap exception is `string` with `choice[...]`: the choice supplies
suggested values while `string` permits additional values. Implementations must
not use first-match behavior, and must accept a value assignable to at least one
member.

### Type tests and branch-local narrowing

`expression is Type` is a boolean type test. Its right operand MUST be a type
reference; literals and runtime expressions are invalid there. A type test may
appear wherever a boolean expression is accepted, but it narrows a stable
identifier or member path only when it directly controls a conditional
expression. Parentheses around that direct condition do not change this rule.
Negated tests and booleans stored in variables do not carry narrowing.

For a source `variant[A, B, C]`, the true branch retains, in declaration order,
all source members assignable to the resolved target type. The false branch
removes those members. A one-member result is simplified to that member, and
aliases are resolved before overlap is calculated. Nested conditionals build on
the environment of their containing branch. The declared symbol type is never
mutated, and neither branch environment survives the conditional expression.

```mace
variant[string, int, boolean] value = 7;
string result = value is string
    ? value
    : value is int
        ? "$(value)"
        : value ? "true" : "false";
```

A nullable value behaves as its present type plus absence for narrowing:
`nullable string` tested with `is string` is `string` in the true branch, while
an absent runtime value makes the test false. A non-nullable concrete value may
be tested against its own type and the result is statically known to be true.
An incompatible concrete test, or a variant test whose target overlaps no
remaining member, is a static `mace.type.impossible-narrowing` error rather than
a constant false. This also rejects a repeated test for a member already removed
by an enclosing false branch.

Schemas retain closed, nominal branch types. Runtime schema conformance checks
required and optional fields, nested field types, and rejects unknown fields;
it does not select a schema merely because host values share a map
representation. Existing exact-one-member variant validation still applies.
Arrays compare resolved element types. Choice targets denote their complete
finite literal domain, not equality with one choice member.

```mace
schema LocalConfig: { path: string, };
schema RemoteConfig: { url: string, };
type Name: string;
variant[LocalConfig, RemoteConfig] config = { path: "/tmp", };
variant[array<string>, Name] payload = ["first"];

string source = config is LocalConfig ? config.path : config.url;
boolean hasStringItems = payload is array<string>;
```

At runtime, `is` evaluates its left operand once and returns true exactly when
the value conforms to the resolved target under the same primitive, choice,
array, and closed-schema rules used by assignment validation. Decimal and
hexadecimal numeric families remain distinct. Type tests do not weaken or
bypass variant ambiguity diagnostics.

Pure alias cycles are invalid. Within a schema or inline record type, `$self`
means the enclosing schema type and may be used only as a structurally guarded
reference (for example `array&lt;$self&gt;`). It is forbidden in a type alias outside
a schema context, forbidden as a runtime recursion mechanism, and a direct
field such as `self: $self` is invalid. Resolvers must keep it symbolic and
terminate on finite values.

## Expressions and evaluation

Operator precedence, highest to lowest, is postfix access; unary; exponent;
multiplication&#x2f;division&#x2f;modulo; addition&#x2f;subtraction; shifts; relational;
type test `is`; equality; merge `&lt;&gt;`; bitwise AND; XOR; OR; logical AND;
logical OR; and `?:`.
Merge operands are ordinary expressions, enabling `base &lt;&gt; overrides` and
`base &lt;&gt; defaults &lt;&gt; local`, but static checking requires each evaluated
operand to be a compatible record or array. Record merges are deep, arrays
concatenate, and a scalar conflict takes the right value. A merge still must
pass its eventual schema validation.

Postfix member access may follow any primary expression where its resulting
type supports access. Parentheses remain syntactically available but are only
for mathematical&#x2f;operator precedence control: they MUST NOT be a general
expression wrapper or be used to wrap records or other non-mathematical values
solely to enable access. Thus `({ user: { name: &quot;Ada&quot; } }).user.name` is
invalid.

`$self` is the current data-output record and can access only fields al
ready
evaluated. Parsed input references are separate: `$name`, `$user.name`, and
`$user.address.street` refer to typed fields from validated runtime input. `$self` is available only during data-output construction.

## Imports, paths, and processing

Imports are named-only, appear before every other script declaration, and can
import only symbols exposed through the imported file&#x27;s output block. Local
names cannot shadow imported names. Wildcard imports are invalid.

Every path literal must use the `.mace` extension and resolves relative to the
containing file. Before access, processors normalize `.` and `..`, normalize
platform separators, resolve symlinks, canonicalize both path and project root,
and reject paths outside the canonical project root. This applies equally to
imports, `schema_file`, and `parse_file`; raw string-prefix checks are
insufficient.

Processing has this observable order:

```text
parse source → resolve imports → resolve schema and parse files → build declarations
→ resolve types → resolve the parse schema → introduce typed parsed-input symbols
→ type-check declarations and output expressions → validate actual runtime input
→ bind parsed values → evaluate output → validate output → emit result
```

If a selected parse schema requires runtime input and none is supplied,
evaluation fails before any output expression runs. Expressions are pure and
deterministic: no shell execution, arbitrary host calls, networking, or
unrestricted filesystem access is permitted.

## Documentation

Inline descriptions may appear on type declarations, variables, schema fields,
inline record-type fields, record fields, and output fields. They are metadata
only. A declaration or field cannot receive both inline and structured
conflicting documentation.

| Target | `gen_doc` | `schema_doc` |
| --- | --- | --- |
| Primitive variable | yes | no |
| Array variable | yes | no |
| Record-valued variable | no | yes |
| Type alias | yes | no |
| Choice alias | yes | no |
| Schema | no | yes |

Structured documentation fields are comma-separated; unknown, duplicate, or
inapplicable targets are errors.

## Diagnostics and security

Processors report a specific diagnostic for an inconsistent script delimiter:
`script block delimiters must match`. They report malformed field separators as
specified above, and diagnose duplicate&#x2f;unknown directives, invalid path
literals, path escape, missing runtime input, type errors, and schema errors.

Mace treats imports, file-loaded schemas, runtime input, documentation, strings,
and emitted values as data, never executable code. Processors must reject
circular imports and path escapes, validate runtime input before binding parsed
references, and avoid leaking partial output after any failure.

## Interoperability

Mace emits structured data or schema metadata in a host-defined representation.
JSON, YAML, and TOML conversion is lossy with respect to comments, formatting,
quoted&#x2f;non-identifier keys, duplicate keys, null as a normal runtime value, and
non-record data roots. No conversion grants executable behavior.

## Nullability

`nullable T name = null;` declares a variable that may be unavailable during
evaluation. `null` is falsy. Before accessing a nullable variable, an
expression MUST guard it with a truthiness condition; the true branch treats
that variable as non-null.

```mace
user ? user.profile.address?.city ?? "" : ""
```

Using either `user.member` or `user?.member` without that guard is invalid.
`null` cannot be placed in an array or record, emitted, imported, exported,
compared, or interpolated.

## Optional member access

`target?.member` is optional member access. Plain `.` is invalid when `target`
is nullable or `member` is an optional schema field. Optional record map lookups
also use `?.`; every optional step in a nested path requires `?.`. Optional
member access produces an absent evaluation result when its member is absent.
The result must be resolved with `??` before it is stored, emitted, or otherwise
used as a value.

Nested record access follows the declared record value type. For example,
`packages.codefixer.cn_efs` requires `packages` to be declared as
`record<record<string>>`; `record<string>` only supports one member lookup.

For `variant` record members, an optional chain can proceed only while every
variant member at that position is a record. The permitted depth is therefore
the shallowest variant member's record depth.
