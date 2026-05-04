# Handoff

## Tests that are failing

## What bugs are present

## What to do next

Absolutely. Mace has *great* Code Action potential because it has a lot of “structured mistakes” the LSP can safely fix: imports, docs, directives, schema validation, enum rules, `$self`, etc. Tiny compiler gremlin playground. 

# Mace LSP Code Actions Checklist

## Imports

### Add / Fix Imports

* [ ] **Add missing import** for unresolved schema/type/enum/value.
* [ ] **Remove unused import**.
* [ ] **Sort imports** alphabetically.
* [ ] **Move imports to top of file**.
* [ ] **Fix import path extension** by adding `.mace`.
* [ ] **Convert invalid wildcard import** into named imports.
* [ ] **Remove duplicate imported names**.
* [ ] **Split import declaration** into one import per file.
* [ ] **Merge imports from same file**.

### Import Resolution

* [ ] **Create missing imported file**.
* [ ] **Update import path after file rename**.
* [ ] **Replace unavailable imported symbol** with closest matching exported output symbol.
* [ ] **Open source output block** for importable symbols.
* [ ] **Explain why symbol is not importable** when it exists internally but is not surfaced through output.

---

## Script Block

### Script Block Structure

* [ ] **Create script block** above output block.
* [ ] **Wrap selected declarations in script block**.
* [ ] **Fix script delimiter length mismatch**.
* [ ] **Normalize script fence** to `|===|`.
* [ ] **Move script block before output block**.
* [ ] **Remove empty script block**.

### Declarations

* [ ] **Change `=` to `:`** for `type`, `schema`, and `enum` declarations.
* [ ] **Change `:` to `=`** for variable initializers.
* [ ] **Add missing semicolon** after script declaration.
* [ ] **Rename duplicate declaration**.
* [ ] **Extract repeated type into `type` alias**.
* [ ] **Extract inline record type into `schema`**.
* [ ] **Convert record variable into schema-backed variable**.

---

## Variables

### Variable Fixes

* [ ] **Add missing type annotation**.
* [ ] **Add missing initializer** for non-injectable variable.
* [ ] **Mark variable as `injectable`** when initializer is intentionally absent.
* [ ] **Add placeholder initializer** based on declared type.
* [ ] **Change variable type to inferred expression type**.
* [ ] **Change initializer to match declared type**.
* [ ] **Rename variable to avoid duplicate declaration**.
* [ ] **Inline variable into output field**.
* [ ] **Extract output expression into script variable**.

### Injectable Variables

* [ ] **Convert variable to injectable**.
* [ ] **Add default initializer to injectable**.
* [ ] **Generate injection config stub** for CLI/processor usage.
* [ ] **Find all injectable variables**.

---

## Types

### Type Aliases

* [ ] **Create type alias from selected type**.
* [ ] **Inline type alias usage**.
* [ ] **Rename type alias**.
* [ ] **Replace unknown type with closest known type**.
* [ ] **Convert `Array<T>` style to `array<T>`** if user writes the wrong casing.
* [ ] **Convert invalid nullable type into optional field**.

### Arrays

* [ ] **Wrap type in `array<...>`**.
* [ ] **Fix mixed array literal** by converting values or splitting into `variant[...]`.
* [ ] **Change array element type** to match literal values.
* [ ] **Replace invalid array index** with valid index suggestions.

---

## Schemas

### Schema Creation / Extraction

* [ ] **Extract output block shape into schema**.
* [ ] **Extract record literal into schema**.
* [ ] **Create schema from selected fields**.
* [ ] **Create schema from validation error**.
* [ ] **Generate output block from schema**.
* [ ] **Generate sample data from schema**.

### Schema Fields

* [ ] **Add missing required field**.
* [ ] **Remove unknown field**.
* [ ] **Mark field optional** with `?`.
* [ ] **Make optional field required** by removing `?`.
* [ ] **Rename duplicate schema field**.
* [ ] **Add comma between schema fields**.
* [ ] **Remove redundant trailing separator** if style prefers none.
* [ ] **Change field type to match assigned data**.
* [ ] **Change output value to match schema field type**.

### Schema Validation

* [ ] **Add `schema = Name` directive** to validate output.
* [ ] **Remove invalid `schema = Name` directive**.
* [ ] **Create missing schema referenced by directive**.
* [ ] **Change schema directive to closest schema name**.
* [ ] **Convert output data block to schema mode**.
* [ ] **Convert schema output block to data mode**.

---

## Enums

### Enum Declaration Fixes

* [ ] **Add enum backing type**: `string` or `int`.
* [ ] **Change invalid enum backing type** to `string` or `int`.
* [ ] **Rename duplicate enum member**.
* [ ] **Fix duplicate enum value**.
* [ ] **Add missing comma between enum members**.
* [ ] **Convert all enum members to explicit values**.
* [ ] **Convert all enum members to implicit values**.
* [ ] **Fix mixed implicit/explicit enum members**.

### Enum Usage

* [ ] **Replace raw enum value with enum member access**.
* [ ] **Change invalid enum member to closest match**.
* [ ] **Create enum member from usage**.
* [ ] **Extract repeated string/int literals into enum**.
* [ ] **Convert string union-like type into enum**.

---

## Output Block

### Output Structure

* [ ] **Create missing output block**.
* [ ] **Move output block after imports/script block**.
* [ ] **Add `[output = data]` directive**.
* [ ] **Add `[output = schema]` directive**.
* [ ] **Remove duplicate output directive**.
* [ ] **Fix invalid directive combination**.
* [ ] **Remove `schema` when `schema_file` is present**.
* [ ] **Remove extra directives when `output = schema` is used**.

### Output Fields

* [ ] **Rename duplicate output field**.
* [ ] **Add comma between output fields**.
* [ ] **Convert semicolon separators to commas**.
* [ ] **Add optional marker `?`**.
* [ ] **Remove optional marker `?`**.
* [ ] **Extract output field value into variable**.
* [ ] **Inline script variable into output field**.
* [ ] **Generate missing fields from schema**.
* [ ] **Remove fields not allowed by schema**.

---

## `$self`

### `$self` References

* [ ] **Replace identifier with `$self.field`** when referencing prior output field.
* [ ] **Move referenced field above current field**.
* [ ] **Extract referenced value into script variable**.
* [ ] **Fix unknown `$self` field with closest prior field**.
* [ ] **Remove invalid forward `$self` reference**.
* [ ] **Convert repeated expression into `$self` reference**.
* [ ] **Add safe previous field suggestion** for autocomplete/code action.

---

## Documentation

### Inline Declaration Descriptions

* [ ] **Add `/#` inline description**.
* [ ] **Remove duplicate inline description**.
* [ ] **Move inline description before separator**.
* [ ] **Move inline description after separator**.
* [ ] **Convert inline description to `gen_doc`**.
* [ ] **Convert inline description to `schema_doc` prop**.
* [ ] **Remove inline description when structured doc already exists**.

### `gen_doc`

* [ ] **Generate `gen_doc` for type**.
* [ ] **Generate `gen_doc` for variable**.
* [ ] **Move `gen_doc` after target declaration**.
* [ ] **Rename `gen_doc` target after symbol rename**.
* [ ] **Remove duplicate `gen_doc`**.
* [ ] **Fix unknown `gen_doc` target**.
* [ ] **Convert non-static summary to static string**.

### `schema_doc`

* [ ] **Generate `schema_doc` for schema**.
* [ ] **Generate `schema_doc` for enum**.
* [ ] **Add missing `props` entries from schema fields**.
* [ ] **Remove unknown `props` keys**.
* [ ] **Rename doc prop after schema field rename**.
* [ ] **Move `schema_doc` after target declaration**.
* [ ] **Remove duplicate `schema_doc`**.

### Output Doc Blocks

* [ ] **Add output inline doc block**.
* [ ] **Move output doc block after directive list**.
* [ ] **Add directive list before output doc block**.
* [ ] **Convert dynamic block string to static doc block**.
* [ ] **Remove duplicate output doc block**.

---

## Strings

### String Fixes

* [ ] **Convert single string to double string** when interpolation is used.
* [ ] **Convert double string to single string** when interpolation is not needed.
* [ ] **Convert multiline string to block string**.
* [ ] **Escape invalid string characters**.
* [ ] **Remove invalid interpolation from documentation string**.
* [ ] **Replace type interpolation with value interpolation**.
* [ ] **Fix malformed interpolation syntax**.

---

## Expressions

### Type / Operator Fixes

* [ ] **Cast-like rewrite suggestion** where Mace requires compatible numeric types.
* [ ] **Replace bitwise operator operands with `int` values**.
* [ ] **Replace invalid array index with integer literal**.
* [ ] **Wrap expression in parentheses** to clarify precedence.
* [ ] **Simplify constant expression**.
* [ ] **Extract expression into variable**.
* [ ] **Replace unresolved identifier with closest symbol**.
* [ ] **Replace invalid member access with valid member**.

---

## Unions and Variants

### Union Fixes

* [ ] **Convert schema composition into `union[...]`**.
* [ ] **Extract repeated schema fields into union member schemas**.
* [ ] **Fix union member that is not a schema**.
* [ ] **Remove conflicting fields from union members**.
* [ ] **Rename conflicting union fields**.

### Variant Fixes

* [ ] **Convert alternative record shapes into `variant[...]`**.
* [ ] **Remove fields from mixed variant alternatives**.
* [ ] **Split ambiguous variant member into distinct schema**.
* [ ] **Change value to match exactly one variant member**.
* [ ] **Replace invalid variant member with schema**.

---

## Comments

### Comment Actions

* [ ] **Toggle line comment** using `/=`.
* [ ] **Toggle block comment** using `/= ... =/`.
* [ ] **Convert line comments to block comment**.
* [ ] **Convert block comment to line comments**.
* [ ] **Fix unterminated block comment**.
* [ ] **Normalize vertical block comment**.

---

## File-Wide Cleanup

### Organize / Format

* [ ] **Format document**.
* [ ] **Organize imports**.
* [ ] **Sort declarations by kind**.
* [ ] **Remove unused declarations**.
* [ ] **Remove unreachable/internal-only declarations**.
* [ ] **Add missing separators**.
* [ ] **Normalize commas vs semicolons**.
* [ ] **Normalize script fence style**.
* [ ] **Normalize directive spacing**.
* [ ] **Normalize type casing**.

### Refactors

* [ ] **Rename symbol across file**.
* [ ] **Rename symbol across imports**.
* [ ] **Extract schema**.
* [ ] **Extract type alias**.
* [ ] **Extract enum**.
* [ ] **Extract variable**.
* [ ] **Inline variable**.
* [ ] **Move declaration to imported file**.
* [ ] **Expose declaration through output block**.

---

# Best First Code Actions to Build

Start with these. They’ll feel magical without requiring your LSP to become a tiny wizard with a mortgage.

* [ ] Add missing required schema field.
* [ ] Remove unknown output field.
* [ ] Replace unknown symbol with closest match.
* [ ] Add missing import.
* [ ] Remove unused import.
* [ ] Move imports to top.
* [ ] Fix `=` vs `:` in declarations.
* [ ] Add missing comma.
* [ ] Fix duplicate output directive.
* [ ] Fix invalid directive combination.
* [ ] Replace raw enum value with `Enum.Member`.
* [ ] Move `$self` referenced field above current field.
* [ ] Generate `schema_doc` props from schema fields.
* [ ] Convert inline record output into named schema.
* [ ] Format document.
