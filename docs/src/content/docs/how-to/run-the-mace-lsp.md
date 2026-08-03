---
title: How to Run the Mace LSP
description: Start the built-in Language Server Protocol server from the CLI.
---

Mace ships its LSP server inside the CLI.

## Start the server

```bash
mace lsp
```

The server runs over stdio, which makes it suitable for editor integration.

## What it supports

The current server supports:

- diagnostics
- completions
- hover
- definitions
- document symbols
- code actions
- reanalysis when watched `.mace` files change
- fence-only formatting that preserves source text and resizes script fences to the widest enclosed line

## Related docs

- [CLI Reference](/how-to/cli-reference/)
- [First Mace File](/tutorials/first-mace-file/)
