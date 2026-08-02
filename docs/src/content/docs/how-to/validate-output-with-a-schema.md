---
title: How to Validate Output with a Schema
description: Validate a data output block against a named schema.
---

Use `output = data, schema = <Name>` when you want your emitted object to be
checked against a schema declared in the same file.

## Declare a schema

```mace
|===|
schema User: {
  name: string,
  age?: int
};
|===|
```

## Validate the output block against it

```mace
|===|
schema User: {
  name: string,
  age?: int
};
|===|

[output = data, schema = User]
{
  name: "Ada",
  age: 27
}
```

## Use `schema_file` when the schema lives elsewhere

```mace
[output = data, schema_file = "./schemas.mace"]
{
  name: "Ada"
}
```

`schema_file` also accepts `http://` and `https://` URLs when the schema is
hosted remotely.

## Related docs

- [Schemas](/reference/schemas/)
- [Doc Syntax](/tour/doc-syntax/)
