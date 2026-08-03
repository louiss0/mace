---
title: How to Validate Output with a Schema
description: Validate a data output against a local, local-file, or HTTP(S) Mace schema.
---

A data output can select one closed record contract. Validation occurs after its
fields evaluate and before any result is returned.

## Use a schema from the same file

Declare a named schema in the script block:

```mace
|===|
schema User: {
  name: string,
  age?: int,
};
|===|

[output = 'data', schema = User]
{
  name: "Ada",
  age: 27,
}
```

`name` is required. `age` may be omitted, but if present it must be an `int`.
Unknown output fields are rejected because schemas are closed.

## Use the exported shape of another file

Create `user-schema.mace`:

```mace
[output = 'schema']
{
  name: string,
  age?: int,
}
```

Load its schema-output body directly:

```mace
[output = 'data', schema_file = './user-schema.mace']
{
  name: "Ada",
}
```

The referenced file must use schema output. Paths are static, single-quoted, and
must end in `.mace`.

## Import a named schema from another file

A schema file may declare more than one reusable contract:

```mace
// account-schemas.mace
|===|
schema User: {
  name: string,
};

schema Team: {
  name: string,
  members: array<User>,
};
|===|

[output = 'schema']
{
  User: User,
  Team: Team,
}
```

Import the declarations needed by the selected schema, then use `schema` on its
own:

```mace
|===|
from './account-schemas.mace' import User, Team;
|===|

[output = 'data', schema = Team]
{
  name: "Compiler",
  members: [
    { name: "Ada" },
  ],
}
```

`schema` selects an available named declaration. `schema_file` instead uses the
referenced file's complete schema-output body. The two directives cannot be used
together.

## Use an HTTP(S) schema source

Explicit remote Mace sources are supported:

```mace
[output = 'data', schema_file = 'https://config.example/schemas/user.mace']
{
  name: "Ada",
}
```

Only `http://` and `https://` URLs ending in `.mace` are accepted. Relative
imports inside a remote source resolve relative to that source and remain within
its remote root. Retrieval or validation failures stop processing without a
partial result.

## Understand failures

Validation fails when:

- a required field is missing;
- an unknown field is present;
- a field value has the wrong type;
- a nested record or array element violates its declared shape;
- `schema` names an unavailable declaration;
- `schema_file` does not resolve to schema output;
- `schema` and `schema_file` are used together;
- a local path escapes the project root or a remote dependency escapes its root;
- source loading, parsing, or dependency resolution fails.

Run the file with `mace json` to receive a diagnostic and a non-zero exit code:

```bash
mace json ./user.mace
```

## Related docs

- [Schemas](/reference/schemas/)
- [Imports and source paths](/reference/imports/)
- [CLI Reference](/how-to/cli-reference/)
