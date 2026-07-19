# Diagnostic fixtures

Every top-level `.mace` file in this directory is intentionally invalid and is
named for the first diagnostic it triggers. Supporting files that must remain
valid live in `support/`. A multi-file error, such as a circular import, may use
more than one top-level fixture.

The executable fixture manifest is
`internal/processor/processor_diagnostics_test.go`. Its table pairs each error
fixture with the required diagnostic message fragment. Add both a fixture and a
table entry whenever the language gains a new normative error.

The suite covers syntax and file structure, directives, imports and secure
paths, declarations, types, schemas, operators, empty collections, fusions,
choices, variants, match expressions, documentation, `$self`, nullability,
optional member access, and null coalescing.

Run the focused conformance suite with:

```powershell
go test ./internal/processor -run TestProcessor -args --ginkgo.focus="Diagnostic fixtures"
```
