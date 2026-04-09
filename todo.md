# Handoff

## Tests that are failing
- No automated tests are currently failing.
- `go test ./...` passes.

## What bugs are present
- No confirmed open bugs are currently tracked.
- The recent split between `union[...]` and `variant[...]` changed core type
  semantics, so editor-facing surfaces may still hide mismatches that are not
  covered by current end-to-end tests.

## What to do next
- Add end-to-end LSP coverage for `union[...]` and `variant[...]`, especially
  completions, type details, and diagnostics in real editor-like request flows.
- Audit public docs and examples for stale wording from the older union-only
  model and keep only the current `union` versus `variant` semantics.
- Add targeted regression tests around imports and schema conversion for
  `allOf`, `anyOf`, and `oneOf` so the codec behavior stays aligned with the
  new type split.
