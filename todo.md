# Handoff

## Tests that are failing
- No known failing tests at handoff time.
- Verified on April 1, 2026 with `go test ./internal/lsp ./internal/processor/...`.

## What bugs are present
- `textDocument/hover` now prefers output fields by cursor position, but
  `textDocument/definition` still resolves by identifier name only. If an output
  field shares a name with a schema, type, or variable, go-to-definition can
  still jump to the wrong declaration.
- Output field ranges are inferred from a token scan in
  `outputFieldHeaderRanges`, so the position-aware behavior is currently a
  narrow fix around top-level output headers rather than a general symbol
  resolution path.

## What to do next
- Apply the same position-aware symbol resolution used by hover to
  `definitionAt`, then add LSP tests for same-name collisions in output fields.
- Decide whether output field locations should come from the parser/AST instead
  of token scanning, and refactor if that is the intended long-term direction.
- After the definition fix lands, run the full Go test suite before handing off
  again.
