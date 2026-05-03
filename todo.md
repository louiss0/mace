# Handoff

## Tests that are failing

## What bugs are present

## What to do next

- Look at the chart below and implement the code actions specified! 

| Area        | Code Action                                  | What it does                                                |     |     |      |              |
| ----------- | -------------------------------------------- | ----------------------------------------------------------- | --- | --- | ---- | ------------ |
| Schema      | **Generate output block from schema**        | Creates fields from a selected schema                       |     |     |      |              |
| Schema      | **Add optional marker `?`**                  | Turns `age: int` into `age?: int`                           |     |     |      |              |
| Schema      | **Remove optional marker `?`**               | Turns optional field into required                          |     |     |      |              |
| Schema      | **Reorder fields to match schema**           | Makes output match schema declaration order                 |     |     |      |              |
| Schema      | **Insert default placeholders**              | Adds `""`, `0`, `false`, `[]`, `{}`                         |     |     |      |              |
| Schema      | **Convert inline record to schema**          | Extracts `{ name: string }` into `schema User`              |     |     |      |              |
| Output      | **Add `schema = Name` directive**            | Connects output data to a schema                            |     |     |      |              |
| Output      | **Convert data output to schema output**     | `[output = data]` → `[output = schema]`                     |     |     |      |              |
| Output      | **Make implicit output explicit**            | Adds `[output = data]`                                      |     |     |      |              |
| Imports     | **Merge duplicate imports**                  | Combines repeated imports from same file                    |     |     |      |              |
| Imports     | **Split import declaration**                 | Separates imports for readability                           |     |     |      |              |
| Imports     | **Fix relative import path**                 | Converts `shared.mace` → `./shared.mace`                    |     |     |      |              |
| Docs        | **Generate `schema_doc` block**              | Creates structured docs for schema/enum                     |     |     |      |              |
| Docs        | **Generate `gen_doc` block**                 | Creates docs for type declarations/variables                |     |     |      |              |
| Docs        | **Add missing `props` docs**                 | Fills missing schema field docs                             |     |     |      |              |
| Docs        | **Move inline `/#` docs to structured docs** | Refactor docs into `schema_doc`                             |     |     |      |              |
| Docs        | **Remove conflicting docs**                  | Fixes duplicate doc sources                                 |     |     |      |              |
| Enum        | **Convert mixed enum to all-explicit**       | Makes every enum member have a value                        |     |     |      |              |
| Enum        | **Convert mixed enum to all-implicit**       | Removes explicit values where safe                          |     |     |      |              |
| Enum        | **Add missing enum member**                  | Adds a member from a known invalid value                    |     |     |      |              |
| Strings     | **Convert string form**                      | Single ↔ double ↔ triple block string                       |     |     |      |              |
| Strings     | **Convert to interpolated string**           | Makes string support `$()`                                  |     |     |      |              |
| Expressions | **Extract expression into variable**         | Moves repeated/large expression into script block           |     |     |      |              |
| Expressions | **Inline variable into output**              | Replaces variable reference with value/expression           |     |     |      |              |
| `$self`     | **Rewrite expression to use `$self`**        | Turns repeated previous field expression into `$self.field` |     |     |      |              |
| Style       | **Normalize separators**                     | Converts semicolons/commas based on context                 |     |     |      |              |
| Style       | **Normalize script fence width**             | Makes `                                                     | === | `/` | ==== | ` consistent |
| Interop     | **Generate JSON preview**                    | Shows emitted JSON from current output                      |     |     |      |              |
| Interop     | **Generate Mace schema from sample data**    | Bootstraps schema from an object                            |     |     |      |              |
| Interop     | **Generate JSON Schema from Mace schema**    | Great for ecosystem compatibility                           |     |     |      |              |

- When it comes to imports 
  - Make sure the user can change the name of the Key's in the output block and have all references changes
  - Make sure the user can change the references in the imports and have the key changed
-
