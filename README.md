# mace

Mace is a typed configuration language and Go toolkit for producing
deterministic object data.

This repository contains:

- a parser, evaluator, and validator for `.mace` files
- a CLI for inspecting, formatting, and evaluating Mace documents
- a language server for editor integrations
- a public Go package for parsing, unmarshalling, and marshalling Mace data
- official Node and Python binding packages under `bindings&#x2f;`

## Status

Mace is actively implemented in this repository. The current language contract
is documented in [the formal specification](.&#x2f;docs&#x2f;src&#x2f;content&#x2f;docs&#x2f;reference&#x2f;spec.mdx).

## Features

- Typed script declarations for `alias`, `schema`, and variables
- Literal `choice[...]` types for user-selectable value domains
- Choice-aware editor completions for literal domains and variants
- Deterministic expression evaluation
- Output validation against local schemas or external schema files in implicit or explicit data outputs
- Relative imports between Mace files and remote imports over HTTP(S)
- Schema-validated runtime input through `parse = &lt;Schema&gt;` and `parse_file = &quot;&lt;path&gt;&quot;` in data outputs, including remote schema files over HTTP(S); parsed fields are exposed as `$`-prefixed variables, `parse` selects an already-available schema, and `parse_file` loads schema declarations and can infer the schema when the referenced file exports exactly one schema
- Canonical source formatting
- Language Server Protocol support over stdio
- Go bindings for parsing, unmarshalling, and marshalling
- Node and Python bindings that wrap the official CLI

## Language overview

A Mace file can contain:

- an optional script block
- exactly one output block

Imports use `from ... import ...;` and must appear at the top of the script
block before other declarations. Imported names may optionally define a
local alias with `Name:Alias`. Use `from &quot;.&#x2f;schema.mace&quot; bind Name;` to bind an output schema file as a single schema or an output data file as a single record variable.

Example:

```mace
|===|
from &quot;.&#x2f;shared.mace&quot; import User:ProfileUser;

alias Environment: choice[&quot;dev&quot;, &quot;prod&quot;];

Environment env = &quot;prod&quot;;
ProfileUser current = {
  name: &quot;Ada&quot;,
  age: 27
};
|===|

[output = 'data']
{
  env: env,
  current: current
}
```

Aliases only rename the local reference inside the importing file. They do
not rename the exported key in the imported file.

Mace supports:

- `:` for alias declarations (`alias`, `schema`)
- `=` for variable initializers
- primitive types: `string`, `int`, `float`, `hex_int`, `hex_float`, `boolean`
- arrays: `array&lt;T&gt;`
- open records: `record&lt;T&gt;` for arbitrary keys whose values must match `T`
- fusions: `fusion[T1, T2, ...]`
- variants: `variant[T1, T2, ...]`
- choices: `choice[&quot;a&quot;, 1, true, OtherChoice]`
- named type aliases
- schemas
- literal `choice[...]` aliases with mixed scalar members, reusable choice aliases,
  and variant-friendly autocomplete
- record, array, arithmetic, logical, and conditional expressions
- record and data output field shorthand: `{ name, }` expands to `{ name: name, }`, and it works for strings, numbers, arrays, nested records, and output blocks
- output fields evaluate expressions directly; parentheses are for grouping math and other expressions
- commas separate record, schema, and output fields; semicolons terminate declarations and statements
- `$self` references inside output evaluation
- hexadecimal integer and fractional numeric types with canonical string JSON output

Fusion and variant types are first-class across the language, including named
aliases, output schema validation, imports, formatter output, and editor
tooling.


Mace treats variants as closed alternatives: values must match exactly one
member, record members reject unknown fields, and record values may not
combine fields from different variant branches.

```mace
|===|
alias Identity: variant[string, int];
alias Values: variant[array&lt;string&gt;, array&lt;int&gt;];
Identity primary = &quot;Ada&quot;;
Identity fallback = 42;
Values tags = [&quot;api&quot;, &quot;web&quot;];
|===|
[output = 'data']
{
  primary: primary,
  fallback: fallback,
  tags: tags
}
```

Mace treats fusions as composition: schema members are combined into one closed
record shape.

```mace
|===|
schema Profile: { name: string };
schema Audit: { created_at: string };
alias User: fusion[Profile, Audit];
User value = {
  name: &quot;Ada&quot;,
  created_at: &quot;2026-04-08&quot;
};
|===|
[output = 'data']
{
  value: value
}
```

Choices define finite literal domains directly in the type system.
Choice aliases can be merged with `fusion[...]` and embedded inside variants.

```mace
|===|
alias Access: choice[&quot;read&quot;, &quot;write&quot;];
alias Feature: choice[&quot;write&quot;, &quot;execute&quot;];
alias Permission: fusion[Access, Feature];
Permission value = &quot;execute&quot;;
|===|
[output = 'data']
{
  value: value
}
```

Hexadecimal values stay distinct from decimal numerics. When emitted through
`mace json`, `hex_int` and `hex_float` values are serialized as strings such as
`&quot;0xFF&quot;` and `&quot;0x2.8&quot;` so their hexadecimal spelling is preserved.
`hex_int` is signed 64-bit; arithmetic and overflowing left shifts fail rather
than wrap, and its minimum value is written `-0x8000000000000000`.
`hex_float` accepts arbitrarily long fixed-point hexadecimal components and is
serialized as an exact, uppercase, fixed-point binary64 expansion with a
required fractional component. This makes every finite value round-trip
without precision loss. The largest finite literal is reproducibly constructed
as `&quot;0x&quot; + strings.Repeat(&quot;F&quot;, 256) + &quot;.0&quot;` and represents
`math.MaxFloat64`.

For the exact rules and currently supported syntax, see
[the formal specification](.&#x2f;docs&#x2f;src&#x2f;content&#x2f;docs&#x2f;reference&#x2f;spec.mdx).

## Installation

### Build locally

```bash
go build .&#x2f;cmd
```

### Install the CLI

```bash
go install github.com&#x2f;louiss0&#x2f;mace&#x2f;cmd@latest
```

Package managers will also be supported through Homebrew, Winget, and Nix.

If you are working on this repository directly, you can also run:

```bash
go run .&#x2f;cmd --help
```

## CLI

The root command is `mace`.

```text
mace json &lt;path&gt;
mace import &lt;path&gt;
mace check &lt;path&gt;
mace nodes &lt;path&gt;
mace output &lt;path&gt;
mace lsp
```

### `mace json &lt;path&gt;`

Evaluates a Mace file and prints the computed output block as JSON.

```bash
mace json .&#x2f;config.mace
```

You can provide runtime parse input with `--input` using a Mace record literal:

```bash
mace json .&#x2f;config.mace --input &#x27;{ env: &quot;prod&quot;, token: &quot;abc&quot; }&#x27;
```

Example input:

```mace
|===|
schema Runtime: { env: string; };
int base = 2 + 2;
|===|
[output = 'data', parse = Runtime]
{
  env: $env,
  base: base
}
```

Example output:

```json
{
  &quot;base&quot;: 4,
  &quot;env&quot;: &quot;prod&quot;
}
```

### `mace import &lt;path&gt; [path...]`

Converts JSON, YAML, and TOML files into `.mace` files.

- input format is determined from each file extension
- generated files are written next to the source files by default
- JSON files with a `$schema` key are converted into Mace output schema blocks
- other JSON, YAML, and TOML files are converted into Mace output data blocks
- JSON Schema `null` maps to field optionality during schema conversion
- JSON Schema `anyOf` and `oneOf` alternatives can be emitted as Mace
  `variant[...]` types during import
- JSON Schema `allOf` schema composition can be emitted as Mace `fusion[...]`
  types during import
- imported `variant[...]` types use Mace&#x27;s closed variant semantics rather than
  preserving a distinct `anyOf` versus `oneOf` behavior
- imported `fusion[...]` types represent schema composition and require schema
  members only
- imported `variant[...]` and `fusion[...]` types remain regular Mace types that
  work in scripts, schema validation, formatting, and LSP tooling
- when multiple files are imported, successful files are still written even if
  some files fail

```bash
mace import .&#x2f;config.yaml
mace import .&#x2f;config.toml
mace import .&#x2f;config.json
mace import .&#x2f;config.json .&#x2f;config.yaml .&#x2f;config.toml
```

Use `--output-dir` to write generated files to a different directory:

```bash
mac
e import .&#x2f;config.json --output-dir .&#x2f;generated
```

### `mace check &lt;path&gt; [path...]`

Checks JSON, YAML, and TOML files for Mace compatibility issues and prints a
Mace record report.

- input format is determined from the file extension when available
- JSON can fall back to content detection when no supported extension is present
- syntax problems are reported under `syntax`
- incompatible keys are reported under `key_incompatibility`
- `null` values and YAML scalar&#x2f;tag mismatches are reported under
  `type_incompatibility`
- duplicate keys, YAML multi-document files, comments, block scalar style loss,
  and structural mismatches such as non-record JSON roots are reported under
  `structure_incompatibility`
- multiple files are emitted as a `files` array of per-file reports

```bash
mace check .&#x2f;config.json
mace check .&#x2f;config.yaml .&#x2f;config.toml
```

Example output:

```mace
{
  syntax: [],
  key_incompatibility: [{
      path: &quot;$[\&quot;foo-bar\&quot;]&quot;,
      reason: &quot;key is not a valid Mace identifier&quot;,
      format: &quot;json&quot;,
      key: &quot;foo-bar&quot;
    }],
  type_incompatibility: [],
  structure_incompatibility: []
}
```

### `mace nodes &lt;path&gt;`

Parses a file and prints its AST-like node structure. This is useful when
working on the language itself.

```bash
mace nodes .&#x2f;config.mace
```

### `mace output &lt;path&gt;`

Parses a file and prints canonical Mace source.

This command does not evaluate the file into runtime JSON output.

```bash
mace output .&#x2f;config.mace
```

This is useful for inspecting how the formatter normalizes script delimiters,
records, choice aliases, and expressions.

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

The public Go API lives in [`.&#x2f;codec`](.&#x2f;codec).

### Parse Mace into generic Go data

```go
package main

import (
	&quot;fmt&quot;

	&quot;github.com&#x2f;louiss0&#x2f;mace&#x2f;codec&quot;
)

func main() {
	result, err := codec.Parse(`[output = 'data']
{
  name: &quot;Ada&quot;,
  enabled: true
}`)
	if err != nil {
		panic(err)
	}

	fmt.Println(result.Data[&quot;name&quot;])
}
```

### Parse with runtime input

```go
result, err := codec.ParseWithInput(`|===|
schema Runtime: { env: string; };
|===|
[output = 'data', parse = Runtime]
{
  env: $env
}`, map[string]any{
	&quot;env&quot;: &quot;prod&quot;,
})
```

### Unmarshal into a struct

```go
type Config struct {
	Name    string `json:&quot;name&quot;`
	Enabled bool   `json:&quot;enabled&quot;`
}

var config Config
err := codec.Unmarshal(`[output = 'data']
{
  name: &quot;Ada&quot;;
  enabled: true;
}`, &amp;config)
```

### Marshal Go values back to Mace

```go
source, err := codec.Marshal(map[string]any{
	&quot;name&quot;: &quot;Ada&quot;,
	&quot;enabled&quot;: true,
	&quot;scores&quot;: []int{1, 2, 3},
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
  &quot;$schema&quot;: &quot;https:&#x2f;&#x2f;json-schema.org&#x2f;draft&#x2f;2020-12&#x2f;schema&quot;,
  &quot;type&quot;: &quot;object&quot;,
  &quot;properties&quot;: {
    &quot;name&quot;: { &quot;type&quot;: &quot;string&quot; }
  },
  &quot;required&quot;: [&quot;name&quot;]
}`)
```

For schema output, `codec.Parse` also returns structured schema metadata in
`Result.Schema`.

## Development

### Run tests

```bash
go test .&#x2f;...
```

### Repository layout

- `cmd&#x2f;` - CLI entrypoints and the LSP server command
- `codec&#x2f;` - public Go API for parsing and marshalling
- `internal&#x2f;lexer&#x2f;` - tokenization
- `internal&#x2f;parser&#x2f;` - parsing and AST construction
- `internal&#x2f;processor&#x2f;` - validation, imports, evaluation, and schema checks
- `internal&#x2f;analyzer&#x2f;` - editor analysis, diagnostics, hover, completion,
  definitions, symbols, code actions, and formatting helpers
- `internal&#x2f;formatter&#x2f;` - canonical source formatting
- `docs&#x2f;src&#x2f;content&#x2f;docs&#x2f;reference&#x2f;spec.mdx` - current language
  specification
- `mace.ebnf` - grammar reference

## Notes

A few language areas are intentionally still in progress. At the time of
writing, the specification lists these as not yet implemented:

- explicit export declarations

## License

Add a license file if you intend to publish or distribute this project.

## Optional chaining

Use `?.` for optional schema properties and record keys that may be absent.
Resolve an optional access with `??` before placing it in output.

```mace
city: user ? user.profile.address?.city ?? "" : "",
packages: record<record<string>>,
value: packages?.codefixer?.cn_efs ?? "",
```

Each nested record lookup requires a corresponding nested record type. For
example, `packages.codefixer.cn_efs` requires `record<record<string>>`; it is
invalid for `record<string>`.

For a record variant, the permitted chain depth is the common record depth of
every variant member. For example,
`variant[record<string>, record<record<string>>]` permits one optional lookup
but rejects a second because the first member is already a `string`.

Accessing an optional schema field with `.` reports
`mace.type.optional-field-access`.
