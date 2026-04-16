# Mace Language Specification

## Status

This document describes the language contract implemented in this
repository today. It avoids speculative features that are not supported by
the parser or processor.

## Goals

Mace is a typed configuration language for producing deterministic object
data. The current implementation parses Mace source, validates declarations
and output against schemas, evaluates expressions, returns an in-memory
object model, and exposes a CLI for JSON emission and source inspection.

## File Structure

A Mace file has this shape:

1. Zero or more import declarations.
2. An optional script block.
3. Exactly one output block.

Imports must appear before the script block and output block.

## Imports

Imports use this syntax:

```mace
from "./shared.mace" import Name, User;
```

Current import rules:

- Import paths are resolved relative to the importing file.
- Only named imports are supported.
- Only symbols exposed through the imported file's output block are importable.
- Top-level `type`, `enum`, `schema`, and variable declarations are internal unless surfaced through the output block.
- There is no explicit `export` keyword.
- Circular imports are rejected.

Imported symbols come from the referenced file's output mode:

- `output = schema` exposes named type-like fields for import.
- A field whose type is a record, or references a schema, is imported as a schema.
- Other schema output fields are imported as types or enums.
- `output = data` exposes named values for import.

## Script Block

The script block is delimited by matching pipe delimiters:

```mace
|===|
type Name: string;
schema User: { name: Name, age?: int };
string user_name = "Ada";
|===|
```

The implementation accepts one or more `=` characters as long as there are
at least three on each delimiter, for example `|===|` and `|====|`.

The script block can contain:

- `type` declarations
- `enum` declarations
- `schema` declarations
- `gen_doc` declarations
- `schema_doc` declarations
- typed variable declarations

### Variable Declarations

Variables are immutable and must have both a type and an initializer.
Variable bindings use `=`. Type declarations use `:`.

```mace
int age = 27;
```

Injectables don't need an initializer!
```mace
injectable string env;
```
`injectable` marks a variable as one where the value is determined at runtime.
If it has no value then an error will be shown! 

The Mace processor and CLI are responsible for making sure the values are injected.

Variables declared in the script block are available to later declarations
and to the output block.

Type inference for declarations is not part of the current language.

### Type Declarations

Type declarations use `:` after the declared name. `=` is reserved for variable
initializers and enum member values.

Type aliases can target:

- primitive types
- array types
- named types

Example:

```mace
type Name: string;
type Scores: array<int>;
```

### Schema Declarations

Schemas are type declarations, so they also use `:` after the declared name.
Schemas define record types with required and optional fields.

```mace
schema User: {
  name: string,
  age?: int
}
```

Field names must be unique within a schema.
Non-final schema fields must be separated with `,`.
A trailing comma on the final schema field is optional.

## Documentation

Mace supports four documentation forms:

- inline doc blocks on directive-based blocks
- `gen_doc` declarations attached to named `type` declarations and variables
- `schema_doc` declarations attached to named `schema` declarations and enums
- inline declaration descriptions written with `/#`

Documentation is metadata only and does not affect evaluation.

### Inline Doc Blocks

Inline doc blocks are static block strings placed immediately after an output
block directive list and before the opening `{`.

```mace
[output = data]
"""
# User Payload

This block emits user data.
"""
{
  name: "Ada";
}
```

Current inline doc block rules:

- Inline doc blocks are allowed only on output blocks.
- They are allowed only when a directive list is present.
- They must appear immediately after the directive list and before `{`.
- They must use a static block string.
- At most one inline doc block is allowed per block.
- They are metadata only and do not affect evaluation.

### Documentation Declarations

Documentation declarations attach structured metadata to named declarations in
the script block.

Use `gen_doc` for `type` declarations and variables:

```mace
type Name: string;
string greeting = "Hello";

gen_doc Name {
  summary: "A user display name.";
}

gen_doc greeting {
  summary: "Rendered greeting.";
}
```

Use `schema_doc` for `schema` declarations and enums:

```mace
schema User: {
  name: string,
  age?: int
}

enum Status: string {
  Pending,
  Running
}

schema_doc User {
  summary: "Represents a user.";
  description: """
# User

A reusable schema that models application users.
""";
  props: {
    name: "The user's display name";
    age: "Optional age";
  };
}

schema_doc Status {
  summary: "Processing status.";
}
```

Current documentation declaration rules:

- `gen_doc` must appear after the target `type` or variable declaration.
- `schema_doc` must appear after the target `schema` or `enum` declaration.
- A target may have at most one documentation declaration.
- Supported entries are `summary`, `description`, and `props`.
- Duplicate or unknown documentation entries are invalid.
- `summary` must be a static string literal.
- `description` must be a static block string.
- `props` is allowed only for `schema_doc` declarations targeting schemas.
- `props` keys must match fields on the target schema.
- Documentation declarations are metadata only and do not affect evaluation.

### Inline Declaration Descriptions

Inline declaration descriptions are written with `/#` on the declaration or
field entity being created. In Mace, `;` terminates completed declaration
entities, while record-style field entities can be separated with `,` or `;`.

```mace
type Name: string /# A user display name;

schema User: {
  name: string /# The user's display name,
  age?: int, /# Optional age in years
};

[output = data]
{
  name: "Ada", /# The emitted user name
  age?: 27 /# Optional emitted age
}
```

Current inline declaration description rules:

- They are allowed on `type` declarations, schema fields, output fields, and
  output schema fields.
- They are not allowed on variable declarations.
- They attach to the declaration or field entity they describe.
- On record-style fields, they may appear immediately before the separator
  (`value /# desc,`) or immediately after it (`value, /# desc`).
- They are raw text metadata, not string literals.
- At most one inline description is allowed per declaration or field.
- They are metadata only and do not affect evaluation.

### Documentation Conflict Rules

The same declaration should not be documented twice.

Current conflict rules:

- A `type` with a `gen_doc` declaration must not also use an inline `/#`
  description.
- A schema field documented through `schema_doc <Schema> { props: { ... } }`
  must not also use an inline `/#` description.
- When documentation already exists in a structured documentation declaration,
  users should extend that declaration instead of adding duplicate inline docs.

### Enum Declarations

Enums define named scalar types with a declared backing type and a fixed set
of unique member values.

```mace
enum Fruit: string {
  Apple,
  Strawberry,
  Pecan
}
```

```mace
enum Status: int {
  Pending,
  Running,
  Done
}
```

Current enum rules:

- Supported backing types are `string` and `int`.
- Enum member names must be unique within the enum.
- Enum values must be unique within the enum.
- Non-final enum members must be separated with `,`.
- A trailing comma on the final enum member is optional.
- `string` and `int` enums may use all-implicit members or all-explicit members.
- An implicit `string` member uses its member name exactly as written.
- An implicit `int` member uses its zero-based declaration index.
- Mixing implicit and explicit members in the same enum is invalid.
- Explicit `string` enum values must be string literals.
- Explicit `int` enum values must be integer literals.

Enums are named types. They may be used anywhere a named non-schema type can
be used, including variable declarations, schema fields, output schema
fields, and imports.

Enum values are accessed with the `.` operator using `EnumName.MemberName`.
When an enum value is required, raw backing values are not assignable.

```mace
enum Fruit: string {
  Apple,
  Strawberry
}

Fruit favorite = Fruit.Apple;
```

## Types

The current type system supports:

- `string`
- `int`
- `float`
- `boolean`
- `array<T>`
- `union[T1, T2, ...]`
- `variant[T1, T2, ...]`
- named type aliases
- named enums
- named schemas

Arrays must be homogeneous. Nested arrays and arrays of schemas are
supported.

Union and variant types may be written inline or behind named type aliases.

```mace
type User: union[Profile, Audit];
type Scalar: variant[string, int];

schema ValueBox: {
  value: variant[string, int];
};
```

Mace unions use schema-composition semantics.

- Union members must be schemas.
- A union combines all member schema fields into one closed record shape.
- Conflicting fields across member schemas are invalid.
- Required fields stay required unless every member marks the field optional.

Mace variants use closed alternative semantics.

- A variant value must match exactly one member.
- Record members are closed: unknown fields are rejected.
- Record values may not mix fields that belong to different variant members.
- If a value matches zero members or more than one member, validation fails.

This means Mace maps JSON Schema `allOf` into `union[...]`, and maps JSON
Schema `anyOf` and `oneOf` into the stricter Mace `variant[...]` behavior.
Schemas that rely on overlapping alternatives, non-structural exclusivity
rules, or external validation logic are not represented exactly in Mace.

For JSON Schema interoperability, `null` should be treated as field
optionality when converting schemas. For example, a JSON Schema property with
`type: ["string", "null"]` maps to an optional Mace field of type `string`.

## Strings

Mace supports three string literal forms:

- single-quoted strings: `'...'`
- double-quoted strings: `"..."`
- triple-double-quoted block strings: `"""..."""`

Single-quoted strings do not support interpolation.
Double-quoted strings and block strings support interpolation with `$(...)`.
The expression inside `$(...)` is parsed as a normal Mace expression and must
resolve to a runtime value. Type references are not valid interpolation
expressions.

Examples:

```mace
'hello'
"Hello $(name)"
"$(price * quantity)"
"$(user.name)"
"$(Status.Done)"
"$($self.name)"
"""
Name: $(user_name)
"""
```

Current string rules:

- Inline strings must not span multiple lines.
- Block strings may span multiple lines.
- Supported escapes are `\\`, `\'`, `\"`, `\n`, `\r`, and `\t`.
- Inline and block strings used as documentation must be static.

## Output Block

The output block is a record of output fields. It may be preceded by a
directive list and an optional inline doc block.
Non-final output fields must be separated with `,`.
A trailing comma on the final output field is optional.

```mace
[output = data, schema = User]
"""
# Public User Output
"""
{
  name: user_name,
  age: 27
}
```

If no output directive is present, the output mode defaults to `data`.

```mace
{
  name: user_name,
  age: 27
}
```

Inline doc block rules:

- An inline doc block is allowed only when a directive list is present.
- It must appear immediately after the directive list and before `{`.
- It must use a static block string.
- At most one inline doc block is allowed per block.
- It is metadata only and does not affect evaluation.

### Supported Directives

The explicit data form is:

```mace
[output = data]
{
  name: user_name,
  age: 27
}
```

The current implementation also supports schema validation with:

```mace
[output = data, schema = User]
{
  name: user_name,
  age: 27
}
```

When a `schema` directive is present, the output record is validated
against that schema.

Schema output is also supported with an anonymous schema block:

```mace
[output = schema]
{
  name: string,
  age?: int
}
```

In schema mode, output fields contain type references instead of
expressions. The processor currently returns those fields using their
declared names and optionality alongside the formatted type string.

External schema declarations can also be loaded for data validation:

```mace
[output = data, schema_file = "./schemas.mace"]
{
  name: user_name
}
```

When `schema_file` is present, the processor loads type and schema
declarations from the referenced Mace file before validating the output.

`output = schema` must be used alone.

`output = data` may be used alone, with `schema = <Name>`, or with
`schema_file = "<path>"`. Combining `schema` and `schema_file` in the same
directive list is invalid.

## Expressions

Expressions are pure and deterministic. The implementation supports:

- identifiers
- member access with `value.member`
- string, int, float, and boolean literals
- array literals
- record literals using comma-separated fields
- `$self` references
- unary operators: `!`, `~`, unary `+`, unary `-`
- arithmetic operators: `+`, `-`, `*`, `/`, `%`, `**`
- shift operators: `<<`, `>>`, `>>>`
- comparison operators: `<`, `<=`, `>`, `>=`
- equality operators: `==`, `!=`,
- bitwise operators: `&`, `|`, `^`
- logical operators: `&&`, `||`
- ternary conditional: `? :`

Operator precedence is implemented in the parser and matches the repository
tests.

Member access may refer to record fields or enum members. Enum members are
still resolved semantically against named enums, while other dotted expressions
are resolved as value member access.

## `$self`

`$self` refers to the output object currently being constructed.

Example:

```mace
[output = data]
{
  base: 4,
  doubled: $self.base * 2
}
```

Output fields are evaluated top to bottom. A `$self` reference may only read
fields that have already been evaluated.

## Validation

The processor validates:

- duplicate declarations
- duplicate enum member names
- duplicate enum values
- unknown type references
- duplicate schema fields
- duplicate output directives
- duplicate output fields
- duplicate or conflicting documentation on the same declaration or field
- import resolution failures
- circular imports
- invalid enum backing types
- invalid enum member literal types
- mixed implicit and explicit enum members
- type mismatches in variables and expressions
- enum-constrained values in variables and schema-validated output
- schema conformance for record literals and output blocks
- mixed-type array literals

## Comments

Comments use the `/=` prefix.

- Line comments continue to the end of the line.
- Block comments begin with `/=` and end with `=/`.
- Vertical block comments are supported when `/=` starts a line and `=/` ends
  the block on a later line.

Disambiguation rule:

- If a `=/` terminator appears before the next newline, the comment is a block
  comment.
- If `/=` is followed only by spaces/tabs before the next newline, it starts a
  vertical block comment that runs until a later `=/`.
- Otherwise the comment is a line comment and ends at the newline.

Example (vertical block comment):

```mace
/=

[output = data]
{
  product: "Mechanical Keyboard";
  unit_price: 129.99;
  quantity: 3;
  subtotal: $self.unit_price * $self.quantity;
  tax_rate: 0.08875;
  tax_amount: $self.subtotal * $self.tax_rate;
  total: $self.subtotal + $self.tax_amount;
}
=/
```

## Example

```mace
from "./base.mace" import Name, User;

|===|
Name name = "Ada";
User result = { name: name, age: 27 };
|===|

[output = data]
{
  result: result
}
```

## Not Implemented Yet

The following are not implemented today:

- explicit export declarations
- runtime injection for `injectable` variables
