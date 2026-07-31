# Mace 1.0.0 release-readiness audit

**Status: ready to tag after GitHub secrets and external publishing access are
confirmed.** Core tooling, editor integrations, grammar, docs, and release
configuration were verified locally.

## Changes made

- The release workflow waits for successful tag CI before publishing and uses
  the tested commit and its resolved tag.
- Zed invokes downloaded release binaries as `mace lsp`.
- Tree-sitter recognizes Unicode identifiers and has a regression corpus test.
- The formal root and documentation-site specifications no longer include the
  interoperability section.
- Added the MIT license for Shelton Louis and linked it from the README.

## Verification

| Area | Evidence |
| --- | --- |
| Core | `go test ./...`, `go vet ./...`, and GoReleaser validation passed. |
| Workflows | `actionlint` accepted every root and grammar workflow. |
| Docs | Astro production build passed. |
| Tree-sitter | `tree-sitter test` passed all 130 corpus cases; the Unicode fixture parses without errors. |
| VS Code | Build and 17 tests passed. |
| Zed | `cargo check` and `cargo test` passed. |

## Interoperability command behavior

`mace import` still lowers JSON, YAML, and TOML to canonical Mace data-output
files. JSON/YAML `null` values are omitted; TOML timestamps become strings.
`mace check` emits a Mace incompatibility report and exits non-zero when issues
are present.

## Publication prerequisite

A real GitHub release still requires configured and authorized GoReleaser deploy
keys, 1Password access, and the editor-extension repository token. Those external
resources cannot be tested from this local workspace.

All newly authored, non-generated files in this audit are under 1,500 lines.