# Handoff

## Tests that are failing
- No automated tests are currently failing.
- `go test ./...` passes.

## What bugs are present
- No confirmed open bugs are currently tracked after the union type work.
- Watch for editor or tooling surfaces that may still assume only primitive,
  array, named, and record type references.

## What to do next
- Add end-to-end coverage for union types in user-facing tooling, especially any
  analyzer, completion, or editor integration paths that present type details.
- Review docs and examples to ensure they match the now-implemented runtime
  `union[...]` support instead of describing it as importer-only behavior.
- Add targeted regression tests for any remaining union edge cases not yet
  covered, such as nested unions, union aliases reused across files, and output
  schema paths that consume named union declarations.
