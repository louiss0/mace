# Unicode NFC conformance fixtures

These fixtures use `café` (U+00E9) and `café` (U+0065 U+0301).
Implementations must compare the semantic name as NFC while preserving the raw
source spelling and ranges.

| Fixture | Result | Expected semantic names / output / diagnostic |
| --- | --- | --- |
| `nfc-reference-nfd.mace` | success | `café` resolves to output key `value` with string `ok`; semantic name is `café` |
| `nfd-reference-nfc.mace` | success | `café` resolves to output key `value` with string `ok`; semantic name is `café` |
| `duplicate-canonical-equivalence.mace` | failure | duplicate-declaration diagnostic for the second declaration; primary range is raw token line 3, columns 8..13 (1-based) |
| `import-canonical-equivalence/` | mixed | `consumer.mace`, `nfc-consumer.mace`, `alias-consumer.mace`, `bind-consumer.mace`, and `schema-file-consumer.mace` succeed; `duplicate-aliases.mace` and `local-import-collision.mace` fail with their existing duplicate diagnostic categories |
| `import-canonical-equivalence/` | success | NFC and NFD exports, import aliases, bind names, schema selectors, and cross-file output keys all use NFC semantic keys; paths remain unchanged |
| `schema-field-canonical-equivalence.mace` | success | the NFD schema field is selected by NFC output spelling; normalized output key is `café` and value is `ok` |
| `string-not-normalized.mace` | success | output string remains the decomposed bytes `café` (semantic output key is `value`) |
| `path-not-normalized.mace` | success | the import path remains `./café.mace`; only the imported identifier is NFC |
| `source-range-after-combining-mark.mace` | success | raw identifier range is line 2, columns 8..13 (1-based); the following `next` token starts at line 3, column 8 |

The fixtures deliberately do not use compatibility characters, case folding, or
confusable-character substitution.
