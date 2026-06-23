# Handoff

## Tests that are failing
- `go test ./internal/processor -count=1 -coverprofile=processorcov.out` passes, but processor coverage is still below 100%.

## What bugs are present
- No functional regression is known from the last verified test run.
- The remaining issue is incomplete coverage in `internal/processor/pkg.go`.

## What to do next
- Add targeted tests in `internal/processor/pkg_test.go` (inside `Describe("Processor helpers")`) for the remaining uncovered branches in `pkg.go`.
- Re-run `go test ./internal/processor -count=1 -coverprofile=processorcov.out` and `go tool cover -func=processorcov.out` until every `pkg.go` function reports `100.0%`.
- If needed, add more focused tests for entrypoints, import handling, and expression/type inference branches.
