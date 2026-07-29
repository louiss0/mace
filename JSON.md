# JSON Interoperability

Mace treats JSON interoperability as a typed lowering, not as a lossless source
rewrite. `mace import file.json` parses one JSON document and generates either a
Mace data output or, when `$schema` identifies a schema, a Mace schema output.
Use `mace check file.json` first when migration loss matters.

## Implementation boundary

JSON is parsed with Go's `encoding/json` decoder in strict JSON mode and with
`UseNumber` enabled. The importer rejects a second document or any trailing
non-whitespace content. The checker scans the token stream separately so it can
report duplicate object keys before decoding would overwrite them.

This means the accepted language is JSON, not JavaScript object-literal syntax:
comments, single-quoted strings, bare keys, trailing commas, `NaN`, and
`Infinity` are syntax errors.

## Data type conversion

| JSON input | Generated Mace | Technical consequence |
| --- | --- | --- |
| string | `string` literal | Escapes are decoded and re-escaped; spelling is not preserved. |
| `true` / `false` | `boolean` | Direct mapping. |
| integer fitting signed 64-bit | `int` | Decimal spelling, exponent notation, and negative-zero spelling are normalized. |
| other JSON number | `float` | Conversion uses binary `float64`; precision can be lost, and out-of-range values fail. |
| array | array value | Order is preserved; Mace later requires a homogeneous element type. |
| object | record value | Object member order is not preserved by the data importer. |
| `null` | omitted value | A field disappears; a null array item is removed and later indexes shift. |

The imported root must become a non-empty record. A root array, scalar, or
`null` is rejected. An object containing only null-valued fields is also rejected
because omission would produce an empty output block.

JSON permits every string as an object key, while Mace record fields require
`[A-Za-z_][A-Za-z0-9_]*`. Quoting a JSON key does not make a key such as
`"log-level"`, `"1st"`, or `""` representable in Mace.

### Semantic subversions

- Duplicate object keys have no stable record meaning. `mace check` reports
  them; ordinary decoding otherwise retains only one value.
- `null` is not imported as Mace's direct-output `null`. It is interpreted as
  absence and omitted recursively.
- JSON has one number category. Mace splits decimal integers and floats and also
  has hexadecimal types. JSON import cannot recover hexadecimal identity.
- Lexical details—member order, whitespace, escape spelling, exponent spelling,
  and number spelling—are discarded.

In the other direction, `mace json` emits computed values as JSON. Mace
`hex_int` and `hex_float` values are emitted as strings (for example `"0xFF"`),
so JSON consumers do not silently reinterpret their type or precision.

## JSON Schema conversion

A JSON object with a string `$schema` is treated as schema-oriented input. A
document containing schema keywords is imported directly; otherwise the
`$schema` URL is loaded from a relative path, `file:`, `http:`, or `https:` URL.
This behavior is intentionally different from preserving `$schema` as ordinary
data.

| JSON Schema construct | Mace type or constraint |
| --- | --- |
| `string` | `string` |
| `integer` | `int` |
| `number` | `float` |
| `boolean` | `boolean` |
| `array` + one schema in `items` | `array<T>` |
| `object` + `properties` | closed inline record or named `schema` |
| `required` | required fields; all other properties are optional |
| `null` in a type, enum, or alternative | field optionality |
| homogeneous string or integer `enum` / non-null `const` | `choice[...]` |
| `oneOf` or `anyOf` | closed `variant[...]` |
| `allOf` of record schemas | `fusion[...]` |
| local `$ref` into the document | generated schema or alias declaration |
| `$defs` reached by `$ref` | named Mace declarations with sanitized names |

Several mappings deliberately strengthen or change the source semantics:

- `oneOf` (exactly one match) and `anyOf` (at least one match) both become the
  same closed Mace variant. Their overlap distinction is not preserved.
- `allOf` becomes Mace schema composition, not arbitrary JSON Schema predicate
  intersection.
- JSON Schema nullability becomes field presence. Mace does not create a
  nullable value type.
- Mace records are closed. `additionalProperties: true` and
  `additionalProperties: { ... }` are rejected; `false` is compatible. When the
  keyword is absent, JSON Schema's open-by-default behavior is silently
  strengthened to a closed Mace record.
- Validation-only keywords such as numeric bounds, string patterns, `format`,
  tuple constraints, conditional schemas, and unevaluated-property rules are
  not translated. Importing a schema must not be read as preserving those
  constraints.

## Syntax and schemas not accepted

The following require migration before import:

- JSON5/JSONC or JavaScript syntax, including comments and trailing commas;
- more than one JSON document or trailing content;
- non-record data roots and roots emptied by null omission;
- object/property names that are not Mace identifiers;
- non-finite or numerically out-of-range values;
- null-only schema types, enums, or constants;
- mixed, boolean, object, array, or floating-point enum domains;
- tuple-form `items`, unconstrained `additionalProperties`, and unsupported
  schema type names;
- non-local `$ref` values inside a schema (only `#/...` references are lowered);
- variants or fusions whose members violate Mace's type rules.

`mace check` categorizes syntax, key, type, and structural incompatibilities,
but successful parsing alone does not prove that every JSON Schema constraint
was preserved.

## Related formats

- [YAML Interoperability](YAML.md)
- [TOML Interoperability](TOML.md)
- [Mace Language Specification](Mace%20Language%20Spec%20RFC.md)