---
title: First Mace File
description: Create, run, and understand a minimal Mace file.
---

This tutorial walks through the smallest useful Mace workflow and gives you a
working result quickly.

## Install the CLI

```bash
go install github.com/louiss0/mace/cmd@latest
```

## Create a file

Create `hello.mace`:

```mace
|===|
string name = "Ada";
|===|
[output = 'data']
{
  greeting: "Hello $(name)",
}
```

This file uses Mace's basic structure:

- an optional script block
- exactly one output block

Imports are optional and, when used, go at the top of the script block.

## Evaluate the file

```bash
mace json ./hello.mace
```

Expected output:

```json
{
  "greeting": "Hello Ada"
}
```

## What happened

- The script block declared `name`.
- The output block emitted one field named `greeting`.
- The string used interpolation with `$(name)`.

## What to read next

- [How to validate output with a schema](/how-to/validate-output-with-a-schema/)
- [How to print canonical Mace source](/how-to/print-canonical-mace-output/)
- [Motivation](/tour/motivation/)
