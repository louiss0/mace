# @code-fixer-23/mace-node

Official Node.js bindings for Mace.

## Status

This package currently wraps the `mace` CLI from this repository.
It is intended as the first official Node binding surface while the core
language remains implemented in Go.

## Development

This package was scaffolded with Vite via `jpd create vite`.
Scoped npm packages are published with `publishConfig.access = "public"`.

```bash
cd bindings/node
jpd install
jpd run build
```

## Usage

```ts
import { json, output } from '@code-fixer-23/mace-node'

const value = await json('./config.mace')
const formatted = await output('./config.mace')
```

## API

- `json(path, options?)`
- `jsonText(path, options?)`
- `output(path, options?)`
- `nodes(path, options?)`
- `importJson(input, options?)`
- `importYaml(input, options?)`
- `importToml(input, options?)`
- `importFile(path, options?)`
