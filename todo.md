# Handoff

## Tests that are failing

None known.

Last local validation run:

```bash
go test ./internal/analyzer/... -count=1
go test ./internal/processor/... -count=1
```

Both passed after commit `c1920ca test(analyzer): add tests for expanded code actions and import completions`.

The tree-sitter corpus was also migrated to commas and passes.

`golangci-lint` was not run locally; run it before opening a PR if available.

## What bugs are present

- Some regex-based code actions in `analysis.go` (e.g. `extractOutputExpressionText`, `createTypeAliasFromSelectedTypeText`) were expanded to handle more cases, but they can still be overly broad on multiline or nested expressions. Review when adding new action variants.
- A few existing code-action generators may still emit semicolons inside braces in edge cases not covered by recent edits. All touched generators now prefer commas, but a full audit of `analysis.go` text-refactor actions is recommended before expanding the checklist further.

## What to do next

- Run `go test ./...` to confirm nothing else is broken after the comma migration.
- Run `golangci-lint` (or `go vet ./...`) and fix any new issues.
- Continue the LSP code action checklist at the next incomplete section:
  - `Schemas > Schema Fields`
  - Implement from top to bottom:
    - Mark field optional with `?`
    - Make optional field required by removing `?`
    - Rename duplicate schema field
    - Add comma between schema fields
    - Remove redundant trailing separator if style prefers none
    - Change field type to match assigned data
    - Change output value to match schema field type
- Before adding more actions, do a quick audit of existing text-refactor code actions to ensure they emit canonical comma separators inside delimited structures, per `mace-spec.md` and `docs/src/content/docs/reference/language.md`.
