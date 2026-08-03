---
title: How to Run the Mace LSP
description: Start the built-in language server and connect an editor over stdio.
---

Mace ships its Language Server Protocol implementation inside the CLI.

## Verify the command

```bash
mace version
mace lsp --help
```

Your editor must be able to find the same `mace` executable through its process
`PATH`. When that is unreliable, configure the editor with the executable's
absolute path.

## Start the server

```bash
mace lsp
```

The server communicates over stdin and stdout. Running it directly appears to
wait because it expects framed LSP messages from an editor client. Stop a manual
session with your terminal's interrupt key.

<Aside type="caution">

Do not wrap `mace lsp` with a command that writes banners or logs to stdout.
Stdout is reserved for the LSP protocol. Editor configuration should launch the
command as a subprocess, not through a terminal panel.

</Aside>

## Configure an editor client

Use these values in an editor's generic language-server configuration:

| Setting | Value |
| --- | --- |
| Command | `mace` |
| Arguments | `lsp` |
| Transport | stdio |
| Language/file type | Mace |
| File extension | `.mace` |
| Workspace root | Project directory containing the edited file |

The exact setting names differ between editors. Prefer an
[official Mace extension](/installation/#official-editor-extensions) when one is
available because it can also register the file type, syntax grammar, and
language-server command.

## Supported capabilities

The current server supports:

- syntax, type, schema, import, and evaluation diagnostics;
- completion for declarations, fields, choices, directives, and visible input;
- hover information;
- go-to-definition;
- document symbols;
- quick fixes and refactoring code actions;
- reanalysis when watched `.mace` dependencies change;
- formatting that preserves source meaning and resizes script fences.

Files using `parse` or `parse_file` may show an informational diagnostic when no
runtime input is available in the editor. The CLI supplies that input at
evaluation time with `mace json --input`.

## Troubleshoot startup

1. Run `mace version` from the editor's environment to confirm `PATH` resolution.
2. Run `mace lsp --help` to confirm the installed version includes the server.
3. Verify the client uses stdio and passes `lsp` as an argument.
4. Verify `.mace` files are assigned the Mace language identifier.
5. Restart the language server after changing the executable or workspace root.
6. Check the editor's LSP log for stderr diagnostics; stdout should contain only
   protocol messages.

## Related docs

- [CLI Reference](/how-to/cli-reference/)
- [First Mace File](/tutorials/first-mace-file/)
- [How to Print Canonical Mace Source](/how-to/print-canonical-mace-output/)
