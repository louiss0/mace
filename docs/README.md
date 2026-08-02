# mace-docs

Documentation site for Mace, built with Astro and Starlight.

## What this repository contains

This repository publishes the Mace documentation site, including:

- tutorials
- how-to guides
- reference pages

The docs live under `src/content/docs/` and follow a Diátaxis-style structure.

## Local development

Install dependencies:

```bash
pnpm install
```

Start the docs site locally:

```bash
pnpm dev
```

Other useful commands:

| Command | Purpose |
| :-- | :-- |
| `pnpm dev` | Start the local dev server |
| `pnpm build` | Build the static site into `dist/` |
| `pnpm preview` | Preview the production build locally |
| `pnpm astro -- --help` | Show Astro CLI help |

## Project structure

```text
.
├── public/
├── src/
│   ├── assets/
│   ├── content/
│   │   └── docs/
│   └── content.config.ts
├── astro.config.mjs
├── package.json
└── tsconfig.json
```

## Content structure

Docs content is organized into these sections:

- `tutorials/`
- `how-to/`
- `reference/`

## Related repositories

- Main Mace repository: https://github.com/louiss0/mace
- Docs repository: https://github.com/louiss0/mace-docs

## Stack

- [Astro](https://astro.build/)
- [Starlight](https://starlight.astro.build/)
