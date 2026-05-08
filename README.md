# mace

Mace is a typed configuration language and Go toolkit for producing
deterministic object data.

This repository contains:

- a parser, evaluator, and validator for `.mace` files
- a CLI for inspecting, formatting, and evaluating Mace documents
- a language server for editor integrations
- a public Go package for parsing, unmarshalling, and marshalling Mace data
- official Node and Python binding packages under `bindings/`

## Status

Mace is actively implemented in this repository. The current language contract
is documented in [`mace-spec.md`](./mace-spec.md).

## Features

- Typed script declarations for `type`, `schema`, `enum`, and variables
- Enum member access with `EnumName.MemberName`
- Deterministic expression evaluation
- Output validation against local schemas or external schema files
- Relative imports between Mace files
- Runtime injectables for environment-specific values
- Canonical source formatting
- Language Server Protocol support over stdio
- Go bindings for parsing, unmarshalling, and marshalling
- Node and Python bindings that wrap the official CLI

## Language overview

A Mace file can contain:

- an optional script block
- exactly one output block

Imports use `from ... import ...;` and must appear at the top of the script
block before other declarations.

Example:

```mace
|===|
from "./shared.mace" import User;

enum Environment: string {
  Dev,
  Prod
}

Environment env = Environment.Prod;
User current = {
  name: "Ada",
  age: 27
};
|===|

[output = data]
{
  env: env,
  current: current
}
```

Mace supports:

- `:` for type declarations (`type`, `schema`, `enum`)
- `=` for variable initializers and enum member values
- primitive types: `string`, `int`, `float`, `boolean`
- arrays: `array<T>`
- unions: `union[T1, T2, ...]`
- variants: `variant[T1, T2, ...]`
- named type aliases
- schemas
- enums backed by `string` or `int`, with implicit or explicit member values
- enum member access with `EnumName.MemberName`
- record, array, arithmetic, logical, merge, and conditional expressions
- `$self` references inside output evaluation

Union and variant types are first-class across the language, including named
aliases, output schema validation, imports, formatter output, and editor
tooling.

The merge operator `<>` combines two values of the same mergeable type. Records
merge deeply, colliding scalar fields use the right-hand value, and colliding
nested records or arrays merge recursively. Arrays concatenate in left-to-right
order.

```mace
[output = data]
{
  result: { profile: { name: "Ada" }; tags: ["base"]; }
    <> { profile: { active: true }; tags: ["override"]; }
}
```

Mace treats variants as closed alternatives: values must match exactly one
member, record members reject unknown fields, and record values may not
combine fields from different variant branches.

```mace
|===|
type Identity: variant[string, int];
Identity primary = "Ada";
Identity fallback = 42;
|===|
[output = data]
{
  primary: primary,
  fallback: fallback
}
```

Mace treats unions as composition: schema members are combined into one closed
record shape, and enum members are combined into one enum alias.

```mace
|===|
schema Profile: { name: string };
schema Audit: { created_at: string };
type User: union[Profile, Audit];
User value = {
  name: "Ada",
  created_at: "2026-04-08"
};
|===|
[output = data]
{
  value: value
}
```

Enum unions create merged same-backing enums. Inline enum unions rewrite source
enum values through an anonymous merged enum in expected-type contexts, while
named enum union aliases merge under the alias name. Later enum members replace
earlier members with the same name; duplicate `int` values are reassigned to the
next available integer, duplicate `float` values are reassigned by `0.1`, and
duplicate `string` values on different keys are rewritten to the member key
value. Enum variants remain source alternatives, but
all keys must be unique and same-backing enum values are shifted through an
anonymous enum so conflicting values do not collide.

```mace
|===|
enum Access: int { Read, Write };
enum Feature: int { Write, Execute };
type Permission: union[Access, Feature];
Permission value = Permission.Execute;
|===|
[output = data]
{
  value: value
}
```

For the exact rules and currently supported syntax, see
[`mace-spec.md`](./mace-spec.md).

## Installation

### Build locally

```bash
go build ./cmd
```

### Install the CLI

```bash
go install github.com/louiss0/mace/cmd@latest
```

If you are working on this repository directly, you can also run:

```bash
go run ./cmd --help
```

## CLI

The root command is `mace`.

```text
mace json <path>
mace import <path>
mace nodes <path>
mace output <path>
mace lsp
```

### `mace json <path>`

Evaluates a Mace file and prints the computed output block as JSON.

```bash
mace json ./config.mace
```

You can provide injectable values with `--inject` using a Mace record literal:

```bash
mace json ./config.mace --inject '{ env: "prod", token: "abc" }'
```

Example input:

```mace
|===|
injectable string env;
int base = 2 + 2;
|===|
[output = data]
{
  env: env,
  base: base
}
```

Example output:

```json
{
  "base": 4,
  "env": "prod"
}
```

### `mace import <path> [path...]`

Converts JSON, YAML, and TOML files into `.mace` files.

- input format is determined from each file extension
- generated files are written next to the source files by default
- JSON files with a `$schema` key are converted into Mace output schema blocks
- other JSON, YAML, and TOML files are converted into Mace output data blocks
- JSON Schema `null` maps to field optionality during schema conversion
- JSON Schema `anyOf` and `oneOf` alternatives can be emitted as Mace
  `variant[...]` types during import
- JSON Schema `allOf` schema composition can be emitted as Mace `union[...]`
  types during import
- imported `variant[...]` types use Mace's closed variant semantics rather than
  preserving a distinct `anyOf` versus `oneOf` behavior
- imported `union[...]` types represent schema composition and require schema
  members only
- imported `variant[...]` and `union[...]` types remain regular Mace types that
  work in scripts, schema validation, formatting, and LSP tooling
- when multiple files are imported, successful files are still written even if
  some files fail

```bash
mace import ./config.yaml
mace import ./config.toml
mace import ./config.json
mace import ./config.json ./config.yaml ./config.toml
```

Use `--output-dir` to write generated files to a different directory:

```bash
mace import ./config.json --output-dir ./generated
```

### `mace nodes <path>`

Parses a file and prints its AST-like node structure. This is useful when
working on the language itself.

```bash
mace nodes ./config.mace
```

### `mace output <path>`

Parses a file and prints canonical Mace source.

This command does not evaluate the file into runtime JSON output.

```bash
mace output ./config.mace
```

This is useful for inspecting how the formatter normalizes script delimiters,
records, enums, enum member access, and expressions.

### `mace lsp`

Starts the Mace language server over stdio.

```bash
mace lsp
```

The server currently supports:

- diagnostics
- completions
- hover
- go to definition
- document symbols
- code actions
- document formatting

## Go package usage

The public Go API lives in [`./codec`](./codec).

### Parse Mace into generic Go data

```go
package main

import (
	"fmt"

	"github.com/louiss0/mace/codec"
)

func main() {
	result, err := codec.Parse(`[output = data]
{
  name: "Ada",
  enabled: true
}`)
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Data["name"])
}
```

### Parse with injections

```go
result, err := codec.ParseWithInjections(`|===|
injectable string env;
|===|
[output = data]
{
  env: env
}`, map[string]any{
	"env": "prod",
})
```

### Unmarshal into a struct

```go
type Config struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

var config Config
err := codec.Unmarshal(`[output = data]
{
  name: "Ada";
  enabled: true;
}`, &config)
```

### Marshal Go values back to Mace

```go
source, err := codec.Marshal(map[string]any{
	"name": "Ada",
	"enabled": true,
	"scores": []int{1, 2, 3},
})
```

### Import JSON, YAML, or TOML into Mace

```go
source, err := codec.ImportYAML(`name: Ada
enabled: true
profile:
  level: 2
`)

schemaSource, err := codec.ImportJSONSchema(`{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" }
  },
  "required": ["name"]
}`)
```

For schema output, `codec.Parse` also returns structured schema metadata in
`Result.Schema`.

## Development

### Run tests

```bash
go test ./...
```

### Repository layout

- `cmd/` - CLI entrypoints and the LSP server command
- `codec/` - public Go API for parsing and marshalling
- `internal/lexer/` - tokenization
- `internal/parser/` - parsing and AST construction
- `internal/processor/` - validation, imports, evaluation, and schema checks
- `internal/analyzer/` - editor analysis, diagnostics, hover, completion,
  definitions, symbols, code actions, and formatting helpers
- `internal/formatter/` - canonical source formatting
- `mace-spec.md` - current language specification
- `mace.ebnf` - grammar reference

## Notes

A few language areas are intentionally still in progress. At the time of
writing, the specification lists these as not yet implemented:

- explicit export declarations
- runtime injection beyond the processor and CLI injection mechanisms

## License

Add a license file if you intend to publish or distribute this project.
