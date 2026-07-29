# TOML Interoperability

TOML and Mace are both configuration-oriented, but their type systems are not
identical. TOML import lowers the parsed table tree to Mace records and arrays;
it does not preserve TOML's concrete table syntax.

## Parser used

The CLI importer and `mace check` use
`github.com/BurntSushi/toml` v1.6. The importer decodes into a
`map[string]any` and uses parser metadata to reconstruct declaration order where
possible. The public Go `codec.ImportTOML` helper uses
`github.com/pelletier/go-toml/v2` unmarshalling and then performs value-level
normalization.

The distinction matters: parser metadata can preserve table/key order for CLI
migration, but neither path preserves comments, exact token spelling, or the
choice between dotted keys, inline tables, and table headers.

## Type conversion

| TOML input | Generated Mace | Technical consequence |
| --- | --- | --- |
| basic/literal string | `string` | Delimiter, escape spelling, and literal/basic distinction are lost. |
| multiline string | triple-quoted `string` | String value remains; TOML trimming and delimiter presentation do not. |
| boolean | `boolean` | Direct mapping. |
| integer | `int` | TOML integers are signed 64-bit; radix, underscores, and explicit sign normalize to decimal. |
| finite float | `float` | Underscores, exponent spelling, and signed-zero presentation normalize through `float64`. |
| offset date-time | `string` | Formatted as RFC 3339 with available fractional precision. |
| local date/time/date-time | `string` | The parser's temporal value is stringified; temporal type identity is lost. |
| array | array value | Order remains; Mace requires homogeneous element types. |
| inline table / table | record value | Both become the same record shape. |
| array of tables | array of records | Header structure is replaced by a value array. |
| dotted key | nested records | Dotted-key syntax and table syntax become indistinguishable. |

TOML has no null value. Mace optionality therefore cannot be inferred from a
TOML data document. Temporal values are intentionally converted to strings
because Mace has no date, time, or date-time primitive.

## Structural conversions

The TOML parser has already expanded table headers and dotted keys before Mace
source is generated:

```toml
[server.tls]
enabled = true

[[server.routes]]
path = "/health"
```

is lowered conceptually to:

```mace
{
  server: {
    tls: { enabled: true },
    routes: [{ path: "/health" }],
  },
}
```

This is value-equivalent for supported types, not syntax-equivalent. On a later
export there is no information saying whether `server.tls` was introduced by a
dotted key, a table header, or an inline table.

A schema comment of this form is recognized by the CLI importer:

```toml
#:schema ./service.schema.toml
```

The referenced path is rebased relative to the generated file, changed to a
`.mace` path, and emitted as the `schema_file` output directive. This special
comment is converted into metadata. Other comments are discarded.

## Features subverted or lost

- comments, whitespace, quote style, numeric radix, underscores, and exact float
  spelling are lost;
- dotted keys, regular tables, inline tables, and arrays of tables are reduced to
  records and arrays;
- date and time families become strings and no longer support temporal
  interpretation at the Mace type level;
- source ordering is preserved on a best-effort basis by CLI metadata, not as a
  semantic guarantee;
- quoted keys lose their quotes and still have to satisfy Mace's identifier
  grammar;
- duplicate/redefined keys are TOML parse errors rather than values that can be
  resolved during conversion.

TOML supports positive/negative infinity and NaN as floating-point syntax. Mace
requires finite numbers. These values cannot become Mace floats; unlike YAML's
explicit string conversion, TOML import currently fails when the generated
non-finite token is validated as Mace source.

## Syntax not accepted

The following require changes before direct Mace use:

- bare or quoted keys outside `[A-Za-z_][A-Za-z0-9_]*`, including keys with
  spaces, hyphens, dots-as-literal-characters, or a leading digit;
- duplicate keys, table redefinitions, invalid dotted-key/table relationships,
  and other TOML parse errors;
- `inf`, `+inf`, `-inf`, and `nan` floating-point values;
- arrays whose values cannot satisfy one Mace element type;
- empty arrays or empty inline tables where Mace receives no expected type;
- any decoded parser value outside strings, booleans, finite numbers, temporal
  values, arrays, and string-keyed tables.

A quoted key such as `"service"` is accepted because its decoded value is a
valid Mace identifier. Quoting does **not** make `"service-name"` acceptable;
Mace has no general quoted-field-name syntax.

## Type-system implications

TOML is a data format, so import produces `[output = 'data']`, not type
declarations. Tables do not automatically become named schemas, temporal values
do not become aliases, and arrays of tables do not become `array<Schema>` until
a Mace schema or declaration supplies that expected type. Use `schema_file` or
add declarations when imported data needs closed shape validation.

## Related formats

- [JSON Interoperability](JSON.md)
- [YAML Interoperability](YAML.md)
- [Mace Language Specification](Mace%20Language%20Spec%20RFC.md)