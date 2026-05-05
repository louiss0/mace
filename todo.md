# Handoff

## Tests that are failing

None known.

Last local validation run:

```bash
go test ./...
```

passed after commit `eabd630 test(analyzer): check import cleanup removal`.

`golangci-lint` was not installed locally, but the reported CI lint issue for unchecked `os.RemoveAll` was fixed and pushed.

## What bugs are present

- Some newer LSP code actions may still emit semicolons inside pair-style structures. The spec says canonical Mace now uses commas for schema fields, output fields, record literal fields, enum members, and documentation entries. Top-level declarations still end with semicolons. The next agent should audit recently added code actions before expanding more actions.
- Several recently added code actions are text/regex-based quick fixes. They pass current tests, but they should be reviewed when touched to avoid overly broad rewrites.

## What to do next

- Check PR #18 status after the pushed lint fix: https://github.com/louiss0/mace/pull/18
- If CI is green and review is clear, merge when appropriate.
- If continuing the LSP code action checklist, start at the next incomplete section:
  - `Schemas > Schema Fields`
  - Implement from top to bottom:
    - Mark field optional with `?`
    - Make optional field required by removing `?`
    - Rename duplicate schema field
    - Add comma between schema fields
    - Remove redundant trailing separator if style prefers none
    - Change field type to match assigned data
    - Change output value to match schema field type
- Before adding more actions, update generated edits to prefer canonical comma separators inside delimited structures, according to `mace-spec.md` and `docs/src/content/docs/reference/language.md`.
