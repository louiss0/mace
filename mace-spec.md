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
- Only top-level `type` and `schema` declarations are importable.
- Variables are not importable.
- There is no explicit `export` keyword.
- Type and schema declarations in a file are implicitly importable by name.
- Circular imports are rejected.

## Script Block

The script block is delimited by matching pipe delimiters:

```mace
|===|
type Name = string;
schema User = { name: Name; age?: int; };
string user_name = "Ada";
|===|
```

The implementation accepts one or more `=` characters as long as there are
at least three on each delimiter, for example `|===|` and `|====|`.

The script block can contain:

- `type` declarations
- `schema` declarations
- typed variable declarations

### Variable Declarations

Variables are immutable and must have both a type and an initializer.

```mace
int age = 27;
injectable string env = "dev";
```

`injectable` marks a variable whose runtime value may be overridden by an
external injection. If no injected value is provided, the declared
initializer is evaluated normally and used as the fallback value.

Variables declared in the script block are available to later declarations
and to the output block.

Type inference for declarations is not part of the current language.

### Type Declarations

Type aliases can target:

- primitive types
- array types
- named types

Example:

```mace
type Name = string;
type Scores = array<int>;
```

### Schema Declarations

Schemas define record types with required and optional fields.

```mace
schema User = {
  name: string;
  age?: int;
};
```

Field names must be unique within a schema.

## Types

The current implementation supports:

- `string`
- `int`
- `float`
- `boolean`
- `array<T>`
- named type aliases
- named schemas

Arrays must be homogeneous. Nested arrays and arrays of schemas are
supported.

## Output Block

The output block is a record of output fields. It may be preceded by a
directive list.

```mace
[output = data, schema = User]
{
  name: user_name;
  age: 27;
}
```

If no output directive is present, the output mode defaults to `data`.

```mace
{
  name: user_name;
  age: 27;
}
```

### Supported Directives

The explicit data form is:

```mace
[output = data]
{
  name: user_name;
  age: 27;
}
```

The current implementation also supports schema validation with:

```mace
[output = data, schema = User]
{
  name: user_name;
  age: 27;
}
```

When a `schema` directive is present, the output record is validated
against that schema.

Schema output is also supported with an anonymous schema block:

```mace
[output = schema]
{
  name: string;
  age?: int;
}
```

In schema mode, output fields contain type references instead of
expressions. The processor currently returns those fields using their
declared names and optionality alongside the formatted type string.

External schema declarations can also be loaded for data validation:

```mace
[output = data, schema_file = "./schemas.mace"]
{
  name: user_name;
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
- string, int, float, and boolean literals
- array literals
- record literals
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

## `$self`

`$self` refers to the output object currently being constructed.

Example:

```mace
[output = data]
{
  base: 4;
  doubled: $self.base * 2;
}
```

Output fields are evaluated top to bottom. A `$self` reference may only read
fields that have already been evaluated.

## Validation

The processor validates:

- duplicate declarations
- unknown type references
- duplicate schema fields
- duplicate output directives
- duplicate output fields
- import resolution failures
- circular imports
- type mismatches in variables and expressions
- schema conformance for record literals and output blocks
- mixed-type array literals

## Comments

Comments use the `/=` prefix.

- Line comments continue to the end of the line.
- Block comments begin with `/=` and end with `=/`.

Disambiguation rule:

- If a `=/` terminator appears before the next newline, the comment is a
  block comment.
- Otherwise the comment is a line comment and ends at the newline.

## Example

```mace
from "./base.mace" import Name, User;

|===|
Name name = "Ada";
User result = { name: name; age: 27; };
|===|

[output = data]
{
  result: result;
}
```

## Not Implemented Yet

The following are not implemented today:

- explicit export declarations
- runtime injection for `injectable` variables
