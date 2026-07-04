# Mace Language Implementation TODO

## Completed
- Make `record<T>` parse correctly in Tree-sitter wherever type references are accepted.
- Add source ranges to AST nodes so diagnostics, hover, rename, and definition can avoid token-scanning fallbacks.

## Next
- Replace analyzer string matching with structured diagnostic errors.
- Split `internal/processor/pkg.go` by responsibility.
- Extract semantic analysis from runtime processing.
- Add conformance fixtures shared by the Go parser, processor/analyzer, and Tree-sitter parser.
- Decide whether enum syntax is implemented, planned, or removed.

## Spec sync
- Resolve `schema_doc` naming drift between `fields` in the EBNF and `props` in the parser and Tree-sitter grammar.
- Document intentional optional semicolon behavior or tighten parser behavior to match the spec.
