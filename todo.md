# Handoff

## Tests that are failing
- No known failing tests remain.
- `go test ./...` was green at handoff time.

## What bugs are present
- LSP semantic diagnostics currently use a fallback range because `processor`
  returns plain errors instead of structured diagnostics with source positions.
- Hover and completion are analysis-backed, but they still expose only a thin
  semantic view of variables and output fields.

## What to do next
- Teach `processor` or the analyzer layer to return structured semantic
  diagnostics with exact line and column ranges, then wire those into the LSP.
- Expand analyzer-backed hover and completion so they surface richer semantic
  information for variables, schemas, arrays, and record outputs.
- Add targeted LSP tests once semantic ranges exist to prove diagnostics land on
  the right tokens after `didChange`.
