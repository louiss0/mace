# YAML Interoperability

YAML has a representation graph and a presentation layer; Mace has typed values
and references. Consequently, YAML import is not a formatting-preserving
transpile. It decodes YAML nodes, lowers representable graph features, and emits
canonical Mace source.

## Parsers used

The CLI importer uses `github.com/goccy/go-yaml`'s lexer, parser, and AST. The
AST is necessary because ordinary unmarshalling would erase anchors, aliases,
merge keys, scalar style, and document boundaries before conversion.

`mace check` uses `gopkg.in/yaml.v3` nodes instead. That parser exposes resolved
tags, comments, duplicate keys, line/column positions, aliases, and document
boundaries for compatibility diagnostics. The public Go `codec.ImportYAML`
helper also uses `yaml.v3` unmarshalling and therefore performs the simpler
value-level conversion; the CLI AST importer is the authoritative path for
feature-preserving migration.

## Type conversion

| YAML node | Generated Mace | Technical consequence |
| --- | --- | --- |
| `!!str` | `string` | Quote and escape style are normalized. |
| literal/folded block scalar | triple-quoted `string` | Resulting text is kept, but `|`/`>` presentation and chomping syntax are not. |
| `!!bool` | `boolean` | Alternate YAML spellings normalize to `true` or `false`. |
| `!!int` | `int` | YAML radix, separators, signs, and spelling are normalized. |
| finite `!!float` | `float` | Converted to binary `float64` and canonical decimal Mace syntax. |
| `.inf`, `-.inf`, `.nan` | `string` | Mace forbids non-finite floats, so the YAML token is quoted rather than treated as numeric. |
| `!!null` | omitted value | Mapping entries disappear; sequence items are removed. |
| sequence | array value | Order is preserved, but Mace requires one element type. |
| mapping | record value | Keys must be strings and valid Mace identifiers. |
| timestamp-like scalar | string-oriented migration | Mace has no date/time scalar; `mace check` reports resolved `!!timestamp` values as a type mismatch. |

Removing null sequence items changes indexes. Removing null mapping entries
changes presence. This is a semantic conversion, not merely a spelling change.
Empty and heterogeneous YAML collections can be rendered, but Mace still needs
an expected homogeneous type when the generated file is checked or evaluated.

## Anchors, aliases, and merge keys

Anchors and aliases are lowered to Mace references when a stable named value can
represent them:

```yaml
defaults: &defaults
  host: localhost
service: *defaults
```

becomes conceptually:

```mace
{
  defaults: { host: "localhost" },
  service: $self.defaults,
}
```

Top-level anchor targets may be hoisted and reordered before fields that depend
on them. Alias identity is not preserved as mutable/shared storage—Mace values
are immutable—but repeated references preserve the value relationship.

YAML `<<` merges are resolved into records during import. Later fields override
merged fields. A merge source must resolve to a mapping or a sequence of
mappings. Unknown aliases, scalar merge sources, and cyclic top-level reference
graphs are rejected.

Custom tags are not executable hooks. The importer descends into the tagged
value and discards the tag; `mace check` reports tags that do not map directly to
Mace scalar, sequence, or record types. Conversion therefore cannot construct
application-specific objects or grant executable behavior.

## Documents and roots

A single mapping document becomes the output record directly. A single scalar
or sequence root is wrapped as `document_1`. Multiple YAML documents are
converted to fields named `document_1`, `document_2`, and so on, with each
mapping nested under its document field.

The checker still reports a multi-document stream as a structural migration
concern. The wrapping is a Mace-specific representation and is not an invisible
round trip to a YAML stream.

A YAML language-server schema comment of this form is recognized by the CLI:

```yaml
# yaml-language-server: $schema=./service.schema.yaml
```

Its path is rebased to the generated file, its extension is changed to `.mace`,
and it becomes Mace's `schema_file` output directive. It is not retained as a
comment. Other comments are discarded and reported by `mace check`.

## Features subverted or lost

- comments, directives, whitespace, quoting, flow/block style, explicit tags,
  anchors' source locations, and scalar presentation are not preserved;
- duplicate keys cannot survive as a Mace record and are reported;
- merge graphs become resolved records and Mace references;
- non-finite numbers become strings;
- null means omission rather than a runtime value;
- timestamps and application-specific tags do not create new Mace types;
- quoted YAML keys do not bypass Mace's identifier grammar.

## Syntax not accepted

The following cannot be converted directly:

- non-string or complex mapping keys;
- key strings outside `[A-Za-z_][A-Za-z0-9_]*`;
- unknown aliases, aliases without a representable named anchor target, or
  cyclic top-level anchor dependencies;
- merge values that do not resolve to one or more records;
- parser-invalid YAML and unsupported AST node kinds;
- structures which render but then violate Mace typing, such as heterogeneous
  arrays or untyped empty collections without an expected type.

Some syntax is accepted but converted with loss rather than rejected: comments,
block-scalar style, tags, timestamps, non-finite floats, nulls, and document
streams. Run `mace check` to distinguish these migration concerns before import.

## Related formats

- [JSON Interoperability](JSON.md)
- [TOML Interoperability](TOML.md)
- [Mace Language Specification](Mace%20Language%20Spec%20RFC.md)