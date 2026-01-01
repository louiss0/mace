## 📘 Mace Language Specification (Draft)

### 1. Introduction

Mace is a strongly-typed configuration language designed for defining variables, schemas, and data outputs. It supports nested schemas, typed arrays, and expressions with immediate calculations.

### 2. Lexical Structure

* **Identifiers**: Consist of letters, digits, and underscores.
* **Literals**:

  * **String literals**: Enclosed in double quotes (`"..."`).
  * **Integer literals**: A sequence of digits.
  * **Float literals**: A sequence of digits, a decimal point, and more digits.
  * **Boolean literals**: `true` or `false`.
* **Operators**: Include arithmetic (`+`, `-`, `*`, `/`, `%`), comparison (`==`, `!=`, `<`, `<=`, `>`, `>=`), logical (`&&`, `||`), bitwise (`&`, `|`, `^`, `<<`, `>>`), and ternary (`? :`).
* **Comments**:

  * **Line comments**: Begin with `/=` and continue to the end of the line.
  * **Block comments**: Begin with `/=` and end with `=/`.
* **Whitespace**: Spaces, tabs, and newlines are used for separation and are generally ignored.

### 3. Syntactic Structure

* **Top-Level Constructs**:

  * **Script Block**: Defines variables, types, schemas, and includes comments.
  * **Output Block**: Specifies the output directives and associated expressions.
* **Statements**:

  * **Variable Declaration**: Defines a variable with an optional type.
  * **Type Declaration**: Defines a new type alias.
  * **Schema Declaration**: Defines a new schema with fields.
  * **Comment**: Provides inline documentation or notes.

### 4. Semantic Rules

* **Variables**: Must be assigned a value upon declaration.
* **Types**: Include primitive types (`string`, `int`, `float`, `boolean`), arrays, and records (schemas).
* **Schemas**: Can contain fields that are either required or optional. Optional fields are denoted with a `?` before the colon.
* **Expressions**: Evaluated based on operator precedence and associativity.

### 5. Evaluation & Type System

* **Types**:

  * **Primitive Types**: Basic data types like `string`, `int`, `float`, and `boolean`.
  * **Array**: An ordered collection of elements of a specified type.
  * **Record (Schema)**: A collection of key-value pairs, where keys are identifiers and values are expressions.
* **Type Constraints**: Arrays must contain elements of the specified type. Records must adhere to their defined schema.
* **Optional Fields**: Fields in schemas and output blocks can be marked as optional using the `?` symbol.

### 6. Operators and Expressions

* **Operator Precedence**:

  1. Parentheses `()`
  2. Exponentiation `**`
  3. Unary Operators `+`, `-`, `!`, `~`
  4. Multiplicative `*`, `/`, `%`
  5. Additive `+`, `-`
  6. Shift `<<`, `>>`
  7. Relational `<`, `<=`, `>`, `>=`
  8. Equality `==`, `!=`, `===`, `!==`
  9. Bitwise AND `&`
  10. Bitwise XOR `^`
  11. Bitwise OR `|`
  12. Logical AND `&&`
  13. Logical OR `||`
  14. Ternary `? :`
* **Associativity**:

  * Most operators are left-associative.
  * Exponentiation (`**`) is right-associative.
* **Evaluation**: Expressions are evaluated based on the above precedence and associativity rules.

### 7. Imports & Modules

* **Import Statement**: Used to import schemas or types from other files.

  * Syntax: `from <path> import <identifier>;`
* **Module Resolution**: The path specified in the import statement is resolved relative to the current file's location.

### 8. Examples / Usage

* **Script Block Example**:

  ```mace
  |===
  type User = {
    name: string;
    age: int;
  };

  schema Admin = {
    permissions?: string[];
  };
  ===|
  ```
* **Output Block Example**:

  ```mace
  [output = data, schema = User]
  {
    name: "Alice";
    age: 30;
  }
  ```
* **Expression Example**:

  ```mace
  [output = data]
  {
    isAdult: age >= 18 ? true : false;
  }
  ```

### 9. Formal Grammar Section

* **EBNF Grammar**: A detailed EBNF grammar defining the syntax of Mace.

### 10. Error Handling

* **Parser Errors**: Errors encountered during parsing are reported with line and column information.
* **Validation Errors**: Errors related to type mismatches or missing required fields are reported with descriptive messages.

Oh yeah — this is the part that turns a grammar into a **real specification**.
EBNF tells you *what is shaped like what*; **semantic rules** tell you *what it means* and *what is allowed*. This is exactly how JSON, TOML, YAML, and ECMAScript do it.

Below is a **professional, spec-style Semantic Rules section** for **Mace**, written in normative language. You could drop this straight into a published spec.

I’ll use **RFC 2119 / 8174 terminology** (`MUST`, `MUST NOT`, `SHOULD`, `MAY`) like real standards.

---

# 📘 Mace Language Specification

## 12. Semantic Rules

### 12.1 General Conformance

* A **Mace document** is conforming if it:

  * Parses successfully according to the Mace EBNF grammar.
  * Satisfies all semantic rules defined in this section.
* A **Mace processor** MUST reject any document that violates a MUST or MUST NOT rule.
* A **Mace processor** SHOULD provide human-friendly, structured error messages when rejecting a document.

---

## 12.2 File Structure Semantics

1. A Mace file:

   * MAY contain zero or more import declarations.
   * MAY contain a script block.
   * MUST contain exactly one output block.
2. Import declarations:

   * MUST appear before any script or output block.
   * MUST reference valid Mace files.
3. At most one script block and one output block MAY appear in a file.

---

## 12.3 Imports

1. The `from <path> import <identifiers>` statement:

   * MUST resolve `<path>` to a readable `.mace` or `.mc` file.
   * MUST import only explicitly named identifiers.
   * MUST NOT support wildcard imports.
2. Imported identifiers:

   * MUST refer to exported schemas or types from the target file.
   * MUST NOT cause name collisions in the importing scope.
3. Import resolution:

   * SHOULD be deterministic and relative to the importing file unless otherwise specified by the implementation.

---

## 12.4 Script Block Semantics

### 12.4.1 Variables

1. Variables declared in the script block:

   * MUST be constants (immutable).
   * MUST have a type.
   * MUST have an initializer.
2. Variables marked `injectable`:

   * MAY be overridden by an external injection mechanism.
   * MUST still satisfy their declared type after injection.
3. Variables declared in the script block:

   * MUST NOT be directly referenced from the output block unless explicitly allowed by the implementation (default: forbidden).

---

### 12.4.2 Types

1. A `type` declaration:

   * Defines a type alias.
   * MUST NOT define record/structural types.
2. Type aliases:

   * MUST ultimately resolve to a valid primitive, array, or schema type.
   * MAY be nested or chained.

---

### 12.4.3 Schemas

1. A `schema` declaration:

   * Defines a record (object) type.
   * MAY contain nested schemas or references to other schemas.
2. Schema fields:

   * Fields without `?` are **required**.
   * Fields with `?` are **optional**.
3. Recursive schemas:

   * MAY be allowed.
   * Implementations SHOULD detect infinite recursion during validation.

---

### 12.4.4 Arrays

1. Arrays:

   * MUST be homogeneous.
   * MUST NOT contain `null`.
2. Array element types:

   * MAY be primitive types.
   * MAY be type aliases.
   * MAY be schemas.
   * MAY be nested arrays.
3. Arrays of schemas:

   * MUST validate each element against the schema definition.

---

## 12.5 Output Block Semantics

### 12.5.1 Output Directive

1. The output directive:

   * MUST appear immediately before the output block.
2. If `output = schema`:

   * No other directive keys MAY appear.
   * The output block MUST contain a valid schema definition.
3. If `output = data`:

   * Exactly one of `schema` or `schema_file` MUST be present.
   * The output block MUST evaluate to an object compatible with the specified schema.

---

### 12.5.2 Output Object

1. The output block:

   * MUST evaluate to a single object (record).
2. Output fields:

   * Fields without `?` are required.
   * Fields with `?` are optional.
3. If a schema is specified:

   * All required schema fields MUST be present in the output.
   * Optional schema fields MAY be omitted.
   * Output MUST NOT contain fields not defined in the schema unless explicitly allowed by the implementation.

---

## 12.6 `$self` Evaluation Semantics

1. `$self`:

   * Refers to the output object currently being constructed.
2. Evaluation order:

   * Output fields MUST be evaluated top-to-bottom.
   * `$self` MAY reference only fields that have already been successfully evaluated.
3. `$self` supports deep access:

   * `$self.a.b.c` is valid if each level exists.
4. Invalid `$self` access:

   * MUST result in a semantic error.
   * MUST include a clear error message identifying the missing path.

---

## 12.7 Expressions

1. Expressions:

   * MUST be pure (no side effects).
   * MUST evaluate deterministically.
2. Supported operators:

   * Arithmetic, comparison, logical, bitwise, and ternary operators.
3. Operator precedence:

   * MUST follow JavaScript operator precedence rules.
4. Division by zero, invalid comparisons, or invalid operations:

   * MUST produce a semantic error.

---

## 12.8 Validation

1. When `output = data`:

* The evaluated output MUST be validated against the resolved schema.
2. Validation failures:

   * MUST cause the document to be rejected.
   * SHOULD include the exact location and reason for failure.
3. Validation MUST include:

   * Missing required fields.
   * Invalid types.
   * Invalid array element types.
   * Invalid nested schema structures.

---

## 12.9 Error Handling

1. Errors returned by a Mace processor:

   * MUST be structured entities.
   * SHOULD be human-friendly.
   * SHOULD include:

     * Error type
     * Location (line/column if available)
     * Description
2. A Mace processor MUST NOT silently ignore errors.

---

## 12.10 Determinism

* Given the same input and the same injected values:

  * A Mace document MUST always produce the same output.
* Implementations MUST NOT introduce hidden state or execution order differences.

---

## 12.11 Security Considerations (Normative)

* Mace expressions MUST NOT:

  * Perform I/O
  * Mutate state
  * Execute arbitrary code
* Mace is a declarative configuration language, not a scripting language.


Fantastic 😄 — then let’s **finish this like a real standards document**.
Below is the **rest of the Mace specification**, written in the same professional tone and structure you’d see in JSON, TOML, or YAML.

At the end of this message, you’ll have a **complete, publishable language spec**.

---

# 📘 Mace Language Specification (Continued)

## 13. Conformance and Compliance

### 13.1 Conforming Documents

A **conforming Mace document**:

* MUST conform to the EBNF grammar.
* MUST satisfy all semantic rules defined in Section 12.
* MUST contain exactly one output block.
* MUST be deterministic (see Section 12.10).

A document that violates any MUST or MUST NOT requirement is **invalid**.

---

### 13.2 Conforming Implementations

A **conforming Mace processor**:

* MUST correctly parse all conforming documents.
* MUST reject invalid documents.
* MUST perform schema validation when required.
* MUST implement `$self` evaluation exactly as specified.
* MUST return structured, human-friendly errors.

A processor MAY support additional features, provided they do not change the meaning of valid documents.

---

## 14. Error Model

### 14.1 Error Categories

A Mace processor SHOULD classify errors into the following categories:

1. **Lexical Errors**

   * Invalid characters
   * Unterminated strings or comments

2. **Syntax Errors**

   * Invalid grammar structure
   * Missing delimiters (`;`, `}`, `]`, etc.)

3. **Semantic Errors**

   * Invalid `$self` access
   * Missing required fields
   * Invalid operator usage
   * Directive rule violations

4. **Validation Errors**

   * Schema mismatches
   * Invalid array contents
   * Type incompatibilities

---

### 14.2 Error Requirements

* Errors MUST NOT be silently ignored.
* Errors SHOULD:

  * Include a descriptive message
  * Include a source location when available
  * Explain *why* the error occurred
  * Suggest a fix when reasonable

#### Example (non-normative):

```text
Error: Missing required field "age"
Location: output.profile.age
Hint: Add "age: int;" to the output or mark it optional in the schema.
```

---

## 15. Evaluation Model

### 15.1 Evaluation Phases

A conforming processor SHOULD evaluate a Mace document in the following order:

1. Lexing
2. Parsing
3. Import resolution
4. Script block evaluation
5. Output block evaluation
6. `$self` resolution
7. Schema validation
8. Final output emission

---

### 15.2 Deterministic Execution

* Expressions MUST evaluate deterministically.
* No expression may depend on external state unless injected.
* Evaluation order MUST be stable and reproducible.

---

## 16. Injection Model (Normative)

### 16.1 Injectable Variables

* Variables marked `injectable` MAY be overridden by external systems.
* Injected values:

  * MUST match the declared type.
  * MUST be applied before output evaluation.
* If no injection occurs, the declared default value MUST be used.

---

### 16.2 Injection Failures

* Injected values that violate type constraints MUST cause a semantic error.
* Injection MUST NOT introduce non-determinism.

---

## 17. Security Considerations

Mace is intentionally **non-Turing-complete**.

A conforming implementation MUST ensure:

* No file system access from expressions
* No network access
* No mutation of state
* No execution of arbitrary code
* No reflection or dynamic typing

This makes Mace safe for:

* Configuration
* Templates
* Build pipelines
* Declarative data generation

---

## 18. Compatibility and Versioning

### 18.1 Forward Compatibility

* Unknown directive keys SHOULD cause an error.
* Unknown keywords MUST cause an error.
* New language features SHOULD be additive.

---

### 18.2 Versioning (Recommended)

Implementations SHOULD support a version declaration in the future, for example:

```mace
[ version = "1.0" ]
```

(Versioning is reserved but not required in this specification.)

---

## 19. Appendix A — Complete Example (Non-Normative)

```mace
from "./schemas/user.mace" import User;

|===|
schema Profile = {
  age: int;
  bio?: string;
};

schema User = {
  name: string;
  profile: Profile;
};

injectable string env = "dev";
|===|

[ output = data, schema = User ]
{
  name: "Alice";
  profile: {
    age: (18 + 2);
    bio?: ($self.profile.age > 18 ? "Adult user" : "Minor");
  };
}
```
