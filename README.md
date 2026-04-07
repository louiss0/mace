# mace

Mace is a typed configuration language and Go toolkit for producing
deterministic object data.

This repository contains:

- a parser, evaluator, and validator for `.mace` files
- a CLI for inspecting, formatting, and evaluating Mace documents
- a language server for editor integrations
- a public Go package for parsing, unmarshalling, and marshalling Mace data

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

## Language overview

A Mace file can contain:

- zero or more `from ... import ...;` declarations
- an optional script block
- exactly one output block

Example:

```mace
from "./shared.mace" import User;

|===|
enum Environment: string {
  Dev,
  Prod,
};

Environment env = Environment.Prod;
User current = {
  name: "Ada";
  age: 27;
};
|===|

[output = data]
{
  env: env;
  current: current;
}
```

Mace supports:

- `:` for type declarations (`type`, `schema`, `enum`)
- `=` for variable initializers and enum member values
- primitive types: `string`, `int`, `float`, `boolean`
- arrays: `array<T>`
- named type aliases
- schemas
- enums backed by `string` or `int`, with implicit or explicit member values
- enum member access with `EnumName.MemberName`
- record, array, arithmetic, logical, and conditional expressions
- `$self` references inside output evaluation

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
mace nodes <path>
mace source <path>
mace lsp
```

### `mace json <path>`

Evaluates a Mace file and prints the output block as JSON.

```bash
mace json ./config.mace
```

You can provide injectable values with `--inject` using a Mace record literal:

```bash
mace json ./config.mace --inject '{ env: "prod"; token: "abc"; }'
```

Example input:

```mace
|===|
injectable string env;
int base = 2 + 2;
|===|
[output = data]
{
  env: env;
  base: base;
}
```

Example output:

```json
{
  "base": 4,
  "env": "prod"
}
```

### `mace nodes <path>`

Parses a file and prints its AST-like node structure. This is useful when
working on the language itself.

```bash
mace nodes ./config.mace
```

### `mace source <path>`

Parses a file and prints canonical Mace source.

```bash
mace source ./config.mace
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
  name: "Ada";
  enabled: true;
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
  env: env;
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
