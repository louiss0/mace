---
title: How to Print Canonical Mace Source
description: Parse a Mace file and print the formatter's canonical source representation.
---

Use `mace output` when you want normalized Mace **source code**, not the evaluated
configuration value.

## Print canonical source

Given `service.mace`:

```mace
|=====|
string name="search";
|=====|
[output='data']{name:name}
```

Run:

```bash
mace output ./service.mace
```

The command parses the file and writes formatter-generated Mace source to stdout:

```mace
|=======================|
string name = "search";
|=======================|
[output = 'data']
{
  name: name
}
```

The formatter normalizes spacing, indentation, separators, and script-fence width
while preserving the program's meaning.

## Redirect the result

Write the canonical source to another file with ordinary shell redirection:

```bash
mace output ./service.mace > ./service.formatted.mace
```

To replace a file safely, write to a temporary path and move it only after the
command succeeds:

```bash
mace output ./service.mace > ./service.mace.tmp && mv ./service.mace.tmp ./service.mace
```

On PowerShell:

```powershell
mace output ./service.mace | Set-Content ./service.formatted.mace
```

`mace output` does not edit the input file itself.

## Know when to use `output` or `json`

| Goal | Command |
| --- | --- |
| Reformat Mace source | `mace output <path>` |
| Evaluate a configuration | `mace json <path>` |
| Supply runtime input and evaluate | `mace json <path> --input '<record>'` |
| Inspect parser nodes | `mace nodes <path>` |

`output` parses and formats syntax. It does not resolve the file's runtime result,
validate host input, or convert the output record to JSON. Expressions therefore
remain expressions in its output.

## Handle failures

Malformed source produces a diagnostic on stderr and a non-zero exit status. No
canonical source is written after the parse or formatting error.

Use this in CI to verify that files can be parsed and formatted, or integrate the
Mace language server when you want formatting directly in an editor.

## Related docs

- [CLI Reference](/how-to/cli-reference/)
- [How to Run the Mace LSP](/how-to/run-the-mace-lsp/)
