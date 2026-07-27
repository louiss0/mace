# Mace Code Actions Implementation Guide

This directory contains the executable contract for Mace code actions. The suite is intentionally red until the language server implements the catalog.

- Phase 1 test checkpoint: `7ffe320`
- Complete catalog test checkpoint: `f6ef579`
- Numbered catalog sections: 252 specs
- Fix-all actions: 20 specs
- Cross-file actions: 25 specs
- Diagnostic-data contracts: 9 specs
- Total: 306 specs

## What is a code action?

A Language Server Protocol code action is an editor operation offered for a source range. It can be one of the following:

- A **quick fix** tied to a diagnostic, such as inserting a missing comma.
- A **rewrite refactor** that changes representation while preserving the user's intent.
- An **extract refactor** that creates a variable, alias, schema, or file.
- A **source action** that operates on a complete file, such as organizing imports.
- A **workspace action** that edits multiple Mace files atomically.
- A **command-backed action** when a plain `WorkspaceEdit` cannot represent interactive behavior.

Every action must have a stable title and kind. Diagnostic fixes must be selected by diagnostic code and structured semantic data, never by matching rendered diagnostic text.

## What every test requires

The shared contract runner is in `file_and_syntax_structure_test.go`. Depending on the action, it verifies:

1. Analysis emits the expected stable `mace.*` diagnostic code.
2. Requesting actions over the diagnostic range returns the expected title.
3. The action has the expected LSP kind.
4. Deterministic actions are marked preferred.
5. A diagnostic quick fix carries the diagnostic it resolves.
6. The action provides an edit or command.
7. Applying local edits produces the expected Mace source.
8. Cross-file edits target the expected URI and produce the expected source.
9. Structured diagnostic data contains the semantic IDs and candidates needed by the action.

Do not make tests green by weakening these assertions, changing titles, or replacing semantic behavior with message matching.

## Running the suite with Ginkgo

The project uses Ginkgo v2 as the test runner and Testify for assertions. The code-action package has one Ginkgo suite containing all section files. The suite bootstrap and shared contract runner live in `file_and_syntax_structure_test.go`.

### Install the matching Ginkgo CLI

The CLI is optional because `go test` can run Ginkgo suites, but the CLI provides better focus, watch, repetition, parallelism, and reporting commands.

Install the version pinned by `go.mod`:

```console
go install github.com/onsi/ginkgo/v2/ginkgo@v2.27.3
```

Ensure the Go binary directory is on `PATH`, then verify the command:

```console
ginkgo version
```

If installing a binary is undesirable, replace `ginkgo` in the examples with:

```console
go run github.com/onsi/ginkgo/v2/ginkgo
```

For example:

```console
go run github.com/onsi/ginkgo/v2/ginkgo --dry-run internal/analyzer/code_actions
```

### Compile without running the red contracts

Use this before handing work to agents or after resolving merge conflicts. It proves that every package and test file compiles without executing the intentionally failing specs:

```console
go test ./... -run '^$'
```

A successful result says `[no tests to run]` for the code-action package. This is a compilation check, not evidence that any code action works.

### Discover the suite without executing specs

From the repository root:

```console
ginkgo --dry-run internal/analyzer/code_actions
```

Use succinct output when only the count and hierarchy are needed:

```console
ginkgo --dry-run --succinct internal/analyzer/code_actions
```

The complete catalog should discover 306 specs. `--dry-run` builds the suite tree but does not execute the bodies of the `It` or table-entry specs.

### Run every code-action spec

From the repository root:

```console
ginkgo internal/analyzer/code_actions
```

From inside the package directory:

```console
cd internal/analyzer/code_actions
ginkgo .
```

The equivalent command without the Ginkgo CLI is:

```console
go test ./internal/analyzer/code_actions
```

Until implementations are added, the full suite is expected to exit nonzero. Agents should focus on their assigned section rather than treating unrelated red sections as regressions.

### Run one delegated section by container name

Each test file has a top-level `Describe` name. Pass a regular expression to `--focus`:

```console
ginkgo --focus='Array code actions' internal/analyzer/code_actions
ginkgo --focus='Match expression code actions' internal/analyzer/code_actions
ginkgo --focus='Cross-file code actions' internal/analyzer/code_actions
```

The `go test` equivalents are:

```console
go test ./internal/analyzer/code_actions --ginkgo.focus='Array code actions'
go test ./internal/analyzer/code_actions --ginkgo.focus='Match expression code actions'
```

Focus expressions match the full Ginkgo hierarchy, so a more specific regular expression can select one behavior:

```console
ginkgo \
  --focus='Array code actions.*changes mixed element type' \
  internal/analyzer/code_actions
```

Quote focus expressions so the shell does not interpret punctuation such as `$`, `?`, brackets, or backticks from action names.

### Run every spec declared in one file

`--focus-file` accepts a filename regular expression. This is usually the easiest command for delegated work because ownership is organized by test file:

```console
ginkgo \
  --focus-file='arrays_test\.go' \
  internal/analyzer/code_actions

ginkgo \
  --focus-file='optional_member_access_test\.go' \
  internal/analyzer/code_actions
```

A file and line can select the table or spec located at that line:

```console
ginkgo \
  --focus-file='arrays_test.go:8' \
  internal/analyzer/code_actions
```

Line focusing follows the location where Ginkgo registered the spec. For parameterized tests, focusing by the `Entry` name is often clearer than relying on a line number.

The `go test` form uses the Ginkgo test flag:

```console
go test ./internal/analyzer/code_actions \
  --ginkgo.focus-file='arrays_test\.go'
```

### Run one parameterized table entry

Most catalog sections use `DescribeTable` with one `Entry` per action. The first string passed to `Entry` is part of the spec name. Focus on it together with the section container:

```console
ginkgo \
  --focus='Primitive and literal type code actions.*widens an integer literal' \
  internal/analyzer/code_actions
```

This runs one contract while still compiling every test file in the package. A focused run does not create a separate Go test binary for the file.

### Re-run while implementing

Ginkgo's watch mode rebuilds and reruns the suite when Go files change:

```console
ginkgo watch \
  --focus='Array code actions' \
  internal/analyzer/code_actions
```

When working from `internal/analyzer/code_actions`:

```console
ginkgo watch --focus='Array code actions' .
```

Use a focused watch during the red-green loop. Running all 306 red specs after every save creates noise and hides progress.

### Stop at the first failure

A section can initially fail many contracts. `--fail-fast` is useful when implementing prerequisites in order:

```console
ginkgo \
  --fail-fast \
  --focus='Output directives and file-loaded schemas code actions' \
  internal/analyzer/code_actions
```

Remove `--fail-fast` before reporting completion so every assigned contract is executed.

### Reproduce a randomized run

Ginkgo prints a random seed at the beginning of each run. Reuse it if order-dependent behavior appears:

```console
ginkgo \
  --seed=1784573097 \
  internal/analyzer/code_actions
```

The specs use isolated temporary workspaces and should not depend on order. A failure that only occurs under one seed usually indicates leaked state or an unsafe shared cache.

### Repeat tests to find nondeterminism

Run a focused section a fixed number of additional times:

```console
ginkgo \
  --repeat=20 \
  --focus='Cross-file code actions' \
  internal/analyzer/code_actions
```

Run until the first failure:

```console
ginkgo \
  --until-it-fails \
  --focus='Cross-file code actions' \
  internal/analyzer/code_actions
```

Stop `--until-it-fails` manually after enough successful runs. It is intended for local stress testing, not as a normal CI command.

### Run in parallel

After a section passes serially, check that it does not rely on global mutable state:

```console
ginkgo \
  --procs=4 \
  --focus='Import' \
  internal/analyzer/code_actions
```

Use serial execution while debugging. Parallel output can interleave failures, and parallelizing a mostly red suite provides little value. Every spec creates its own temporary workspace, so completed sections should eventually be parallel-safe.

### Use verbose and trace output

Show each spec as it runs:

```console
ginkgo \
  -v \
  --focus='Variant code actions' \
  internal/analyzer/code_actions
```

Show full stack traces for failures:

```console
ginkgo \
  --trace \
  --focus='Variant code actions' \
  internal/analyzer/code_actions
```

Use succinct mode when only a summary is needed:

```console
ginkgo \
  --succinct \
  --focus='Variant code actions' \
  internal/analyzer/code_actions
```

### Interpret common failures

The contract runner deliberately stops after missing prerequisites, so the first failure indicates the current implementation layer:

| Failure | Meaning |
|---|---|
| `expected diagnostic code ...` | Analysis did not emit the required stable code, or parsing stopped before semantic validation. |
| `expected code action ...` | The diagnostic exists, but no provider returned the required action for its range. |
| Kind mismatch | The action exists but is categorized incorrectly as quick fix, rewrite, extract, or source action. |
| Preferred mismatch | A deterministic fix is not preferred, or an ambiguous action was incorrectly preferred. |
| Action does not carry the diagnostic | The action was generated globally rather than being associated with the diagnostic it resolves. |
| `workspace edit must target ...` | The edit uses the wrong document URI or omitted the current document. |
| Expected text missing | The edit range or generated Mace source is incorrect. |
| Expected external file text missing | A cross-file action did not edit the owning file atomically. |
| Missing diagnostic `data` key | The diagnostic code exists, but semantic metadata required by providers is absent. |

Implement failures from the top of this table downward. An action provider cannot be tested meaningfully until its diagnostic code and data exist.

### Generate machine-readable reports

Generate a JUnit report for CI or agent artifacts:

```console
ginkgo \
  --junit-report=code-actions.xml \
  --output-dir=test-reports \
  internal/analyzer/code_actions
```

Generate Ginkgo's JSON report:

```console
ginkgo \
  --json-report=code-actions.json \
  --output-dir=test-reports \
  internal/analyzer/code_actions
```

Reports are written under `test-reports`. Do not commit generated reports unless the repository explicitly adopts them as artifacts.

### Run with the race detector and coverage

Use the race detector for workspace indexes, caches, and parallel providers:

```console
ginkgo \
  --race \
  --focus='Cross-file code actions' \
  internal/analyzer/code_actions
```

Collect coverage after a section is green:

```console
ginkgo \
  --cover \
  --coverpkg=github.com/louiss0/mace/internal/analyzer/... \
  --coverprofile=code-actions.cover \
  --output-dir=test-reports \
  --focus='Array code actions' \
  internal/analyzer/code_actions
```

Coverage is secondary to behavior. Do not add implementation-only tests merely to increase the percentage.

### Recommended agent verification sequence

For an agent assigned `arrays_test.go`:

```console
# 1. Confirm all packages compile.
go test ./... -run '^$'

# 2. Discover only the owned file's contracts.
ginkgo --dry-run \
  --focus-file='arrays_test\.go' \
  internal/analyzer/code_actions

# 3. Run the owned section serially while implementing.
ginkgo --focus='Array code actions' \
  internal/analyzer/code_actions

# 4. Run every entry in the owned file without fail-fast.
ginkgo --focus-file='arrays_test\.go' \
  internal/analyzer/code_actions

# 5. Check for shared-state bugs.
ginkgo --procs=4 --focus='Array code actions' \
  internal/analyzer/code_actions

# 6. Compile the whole repository again.
go test ./... -run '^$'
```

Before claiming completion, the agent should report the number of focused specs run and passed, the exact focus command, and any unrelated red sections observed in a full-suite run.

### Inspect the contracts owned by a file

List action table entries and explicit diagnostic codes:

```console
rg 'Entry\(|diagnosticCode:' \
  internal/analyzer/code_actions/arrays_test.go
```

List every section and spec count:

```console
ginkgo --dry-run --succinct internal/analyzer/code_actions
```

## Existing entry points

The current public flow is:

1. `analyzer.AnalyzeDocumentAtInRoot` builds an analysis snapshot.
2. `analyzer.Diagnostics` exposes LSP diagnostics.
3. `analyzer.CodeActions` filters action candidates for a requested range.
4. `internal/analyzer/analysis.go` currently builds candidates.
5. `internal/analyzer/diagnostics.go` maps parser and processor failures to diagnostic codes.
6. `internal/processor/errors.go` carries processor error codes and fields.

Some existing action helpers in `analysis.go` inspect error messages. They must be migrated to stable codes and structured data before the catalog can be considered complete.

## Foundation work that should happen first

Parallel feature agents should not all modify the shared analyzer pipeline at once. Complete and commit these foundations before broad parallel work.

### 1. Structured diagnostic data

Create the Go equivalent of `MaceDiagnosticData` and attach it to LSP diagnostics. It needs fields for:

- Diagnostic code and syntax node ID
- Symbol ID
- Expected and actual type IDs
- Missing variant or choice members
- Missing and unknown schema fields
- Candidate symbols and files
- Related locations
- Output mode
- Selected output schema
- Selected parse schema

The required JSON field names are tested in `diagnostic_data_test.go`.

### 2. Stable diagnostic codes

Add all codes referenced by the section tests. Parser recovery and processor validation must emit those codes directly. Do not derive them from diagnostic messages.

A foundation agent should add the complete code registry up front so feature agents do not conflict while editing a central constants block.

### 3. Code-action dispatch

Dispatch providers by diagnostic code and semantic data. A provider should receive enough immutable document/workspace information to calculate edits without reparsing diagnostic messages.

Recommended separation:

```text
internal/analyzer/
  code_action_provider.go
  code_action_edits.go
  code_action_workspace.go
  code_action_syntax.go
  code_action_imports.go
  code_action_directives.go
  code_action_declarations.go
  code_action_types.go
  code_action_matches.go
  code_action_documentation.go
```

This is a suggested ownership boundary, not a requirement to fragment cohesive code. The important part is that parallel agents do not repeatedly edit `analysis.go`.

### 4. Edit utilities

Provide shared helpers for:

- UTF-16-safe LSP ranges
- Token and AST-node replacement
- Insertion before or after declarations
- Removing comma-separated members without damaging surrounding trivia
- Applying multiple non-overlapping edits in descending order
- Preserving indentation and canonical Mace formatting
- Building atomic multi-file `WorkspaceEdit` values
- Detecting edit conflicts before constructing fix-all actions

### 5. Workspace semantic index

Cross-file actions require an index of files, exports, declarations, imports, references, schemas, match sites, documentation fields, and directives. Build this once rather than having each action scan the workspace independently.

## Section implementation plans

### 1. File and syntax structure

**Tests:** `file_and_syntax_structure_test.go` — 16 specs  
**Suggested owner file:** `code_action_syntax.go`

To pass:

- Give parser recovery errors precise punctuation, delimiter, and structure codes.
- Preserve enough token information to insert commas and semicolons at the actual missing boundary.
- Track both script fence tokens so either side can be replaced or a closing fence inserted.
- Calculate declaration and output-block ranges for move, merge, and removal actions.
- Distinguish arithmetic grouping from forbidden or redundant grouping.
- Use token and symbol resolution to distinguish a kebab-case identifier from subtraction.
- Mark only deterministic fixes preferred.

### 2. Imports and relationships between files

**Tests:** `imports_and_file_relationships_test.go` — 22 specs  
**Suggested owner file:** `code_action_imports.go`

To pass:

- Index exported names and their declaring files.
- Model local import names, aliases, wildcard expansion, duplicate names, and shadowing.
- Resolve paths relative to the current file and canonical project root.
- Normalize separators and append `.mace` only when the resulting file is valid.
- Rank nearby files and similarly named exports deterministically.
- Understand whether an imported file exposes data or schema output before adding an export.
- Detect circular edges and determine whether imported symbols are actually used.
- Build atomic edits for exposing, moving, renaming, and cycle-breaking operations.
- Implement `source.organizeImports` separately from diagnostic quick fixes.

### 3. Output directives and file-loaded schemas

**Tests:** `output_directives_test.go` — 19 specs  
**Suggested owner file:** `code_action_directives.go`

To pass:

- Give each directive an AST range and stable identity.
- Detect missing, duplicate, unknown, misspelled, incompatible, and redundant directives.
- Infer data-shaped versus schema-shaped output without treating inference as a language feature.
- Resolve local and externally loaded schemas and parse schemas.
- Know which directives are data-only and remove all incompatible entries safely.
- Generate output schemas from resolved output field types, including empty collections.
- Add or create schema files with valid relative paths.
- Preserve the canonical single-quoted directive syntax.

### 4. Declarations, symbols, and explicit typing

**Tests:** `declarations_and_symbols_test.go` — 17 specs  
**Suggested owner file:** `code_action_declarations.go`

To pass:

- Calculate expression types for edits while preserving Mace's explicit declaration typing rule.
- Synthesize valid initializers from expected types.
- Offer safe literal-family conversions before broader type rewrites.
- Rank compatible nearby values and types by semantic kind and name distance.
- Create variables, aliases, and schemas in the correct script location.
- Distinguish parsed-input variables from local declarations.
- Use the workspace symbol index for auto-import actions.
- Inline or extract aliases without introducing type cycles.

### 5. Primitive and literal types

**Tests:** `primitive_and_literal_types_test.go` — 18 specs  
**Suggested owner file:** `code_action_literals.go`

To pass:

- Preserve literal source ranges and expected primitive families.
- Implement string quoting, interpolation conversion, escaping, and block-string generation.
- Convert boolean representations only when the expected type makes the conversion unambiguous.
- Convert integral and floating literals without changing non-integral values silently.
- Separate decimal and hexadecimal numeric families.
- Parse, validate, and canonicalize hexadecimal floats.
- Detect integer boundaries and keep clamping actions non-preferred.
- Respect operator-specific restrictions such as decimal-only complement operands.

### 6. `null` output actions

**Tests:** `null_output_test.go` — 7 specs  
**Suggested owner file:** `code_action_null.go`

To pass:

- Track the syntactic and semantic context of every `null` expression.
- Treat direct data-output `null` as omission, not as a nullable runtime value.
- Remove invalid declarations, array members, and nested record fields without breaking separators.
- Derive typed fallback values from the receiving context.
- Rewrite omission to a top-level output field only when that preserves the stated intent.
- Do not invent JSON serialization or nullable runtime semantics.

### 7. Arrays

**Tests:** `arrays_test.go` — 13 specs  
**Suggested owner file:** `code_action_arrays.go`

To pass:

- Resolve every array element type through aliases and schemas.
- Build normalized variant members from resolved types rather than literal syntax.
- Detect a single safely convertible or removable incompatible element.
- Infer explicit array types for empty arrays only from an established expected type.
- Generate and attach output schemas for schema-less empty collections.
- Type both conditional branches consistently.
- Flatten nested variant members and extract reusable aliases where requested.

### 8. Record maps, record literals, and closed schemas

**Tests:** `records_and_closed_schemas_test.go` — 18 specs  
**Suggested owner file:** `code_action_records.go`

To pass:

- Compute a schema difference containing missing, unknown, duplicate, and incompatible fields.
- Keep field IDs and declaration locations in diagnostic data.
- Generate defaults only for the safe finite cases described by the catalog.
- Offer separate actions for choice literals and variant members.
- Recursively generate required records only when recursion is finite.
- Edit schemas across files for intentional extensions and type widening.
- Handle optional markers only in schema contexts.
- Convert between broad `record<T>` and closed inline records using resolved field uniformity.

### 9. `$self` and recursive schemas

**Tests:** `self_and_recursive_schemas_test.go` — 10 specs  
**Suggested owner file:** `code_action_self.go`

To pass:

- Represent `$self` as a symbolic recursive type; never eagerly expand it.
- Distinguish runtime, output, parse-input, alias, and schema contexts.
- Build an output-field dependency graph for forward-reference reordering.
- Detect when reordering is unsafe and offer a direct-variable alternative.
- Recognize guarded recursion through containers.
- Convert alias cycles to schema recursion only when the shape demonstrates recursive record intent.

### 10. Variants

**Tests:** `variants_test.go` — 10 specs  
**Suggested owner file:** `code_action_variants.go`

To pass:

- Normalize members after alias resolution.
- Detect duplicate, nested, equivalent, overlapping, broad, and invalid members.
- Infer result variants for conditionals and matches.
- Keep representation-changing actions explicit in title and kind.
- Index every match site linked to a changed variant.
- Update all affected matches in one workspace edit when the domain changes.

### 11. Choices

**Tests:** `choices_test.go` — 10 specs  
**Suggested owner file:** `code_action_choices.go`

To pass:

- Normalize and deduplicate choice literals by value and numeric family.
- Reject types and arbitrary expressions as choice members.
- Produce one concrete replacement action per allowed value.
- Edit the declaration when a new literal is intentionally accepted.
- Detect copied composition and choice alias cycles.
- Update linked exhaustive match expressions after domain changes.

### 12. Fusions

**Tests:** `fusions_test.go` — 12 specs  
**Suggested owner file:** `code_action_fusions.go`

To pass:

- Classify every fusion as record-domain, choice-domain, or invalid mixed-domain.
- Detect field type and optionality conflicts after alias resolution.
- Never silently turn a record conflict into a variant.
- Offer explicit common-type, shared-variant, rename, remove, and split alternatives.
- Flatten nested record and choice fusions independently.
- Deduplicate composed choice domains.
- Extract repeated inline records when requested.

### 13. Match expressions

**Tests:** `match_expressions_test.go` — 17 specs  
**Suggested owner file:** `code_action_matches.go`

To pass:

- Attach the resolved variant or choice domain to match diagnostics.
- Compute missing, duplicate, overlapping, invalid, and out-of-domain patterns.
- Require type patterns for variants and literals for choices.
- Synthesize result expressions only when an expected type has a safe default.
- Use command-backed generation when placeholders are required.
- Infer and normalize arm result types.
- Link match sites to variant and choice declarations for workspace updates.

### 14. Optional member access and record depth

**Tests:** `optional_member_access_test.go` — 14 specs  
**Suggested owner file:** `code_action_optional_access.go`

To pass:

- Resolve the type and optionality of every member-access step.
- Replace only the separators that cross optional boundaries.
- Derive `??` fallback types from the final member.
- Offer each allowed choice fallback and each compatible in-scope symbol separately.
- Track record-map and closed-record depth.
- Compare depth across all variant members.
- Provide schema rewrites separately from local path-shortening fixes.

### 15. Conditional expressions

**Tests:** `conditional_expressions_test.go` — 10 specs  
**Suggested owner file:** `code_action_conditionals.go`

To pass:

- Identify nested conditionals and their exact subexpression ranges.
- Detect when the condition's source is a variant or choice suitable for `match`.
- Infer both branch result types and normalize the resulting variant.
- Convert one branch only when the conversion is safe.
- Carry expected collection types into empty branch collections.
- Constant-fold only statically known conditions.
- Extract repeated branch expressions without changing evaluation order.

### 16. Operators and operand relationships

**Tests:** `operators_test.go` — 15 specs  
**Suggested owner file:** `code_action_operators.go`

To pass:

- Attach operator, operand types, result type, and constant values to diagnostics.
- Map logical and bitwise alternatives based on resolved operands.
- Keep decimal and hexadecimal arithmetic families explicit.
- Respect precedence when inserting or removing grouping.
- Validate exponent and shift constraints.
- Use constant evaluation for known zero divisors and overflow.
- Keep value-changing alternatives non-preferred.
- Offer guarded division when no deterministic value replacement exists.

### 17. Interpolation

**Tests:** `interpolation_test.go` — 8 specs  
**Suggested owner file:** `code_action_interpolation.go`

To pass:

- Preserve interpolation segment ranges in the AST.
- Convert shorthand markers to `$(...)` without altering surrounding text.
- Resolve interpolated expression types and reject arrays and records.
- Offer scalar record fields as concrete choices.
- Narrow variants before interpolation when not every member is scalar.
- Coalesce optional and null-producing expressions.
- Never invent implicit JSON serialization.

### 18. Structured and inline documentation

**Tests:** `documentation_test.go` — 16 specs  
**Suggested owner file:** `code_action_documentation.go`

To pass:

- Resolve each documentation block to its semantic target.
- Distinguish record-valued targets from primitive, collection, alias, and choice targets.
- Track documentation key and field-entry ranges.
- Synchronize documented fields with schema field IDs, not spelling alone.
- Detect conflicts between inline and structured documentation.
- Escape forbidden interpolation markers.
- Implement full-file synchronization as a source action.

### High-value fix-all actions

**Tests:** `fix_all_test.go` — 20 specs  
**Suggested owner file:** `code_action_fix_all.go`

To pass:

- Reuse the deterministic local providers rather than duplicating their logic.
- Collect all edits under `source.fixAll.mace`.
- Sort edits and reject overlaps or conflicts.
- Never include actions that invent public fields, alter other files, change numeric values, or widen schemas.
- Keep fix-all output deterministic regardless of diagnostic order.

### Cross-file actions

**Tests:** `cross_file_actions_test.go` — 25 specs  
**Suggested owner file:** `code_action_workspace.go`

To pass:

- Build the workspace semantic index before starting this section.
- Track declaration IDs across imports, output exports, directives, member paths, match patterns, and docs.
- Propagate renames by symbol identity rather than token spelling.
- Link variant and choice declarations to exhaustive matches.
- Link schema fields to data outputs, `$self`, parsed input, and documentation.
- Support create, move, rename, and multi-file edit operations atomically.
- Revalidate importers after exported symbols or files change.
- Refuse a workspace action when any required file cannot be safely resolved or edited.

### Diagnostic data

**Tests:** `diagnostic_data_test.go` — 9 specs  
**Suggested owner files:** `diagnostics.go`, `internal/processor/errors.go`, and a dedicated diagnostic-data file

To pass:

- Serialize all tested semantic fields into `protocol.Diagnostic.Data`.
- Use stable JSON field names from the tests.
- Populate only real IDs and locations from analysis; do not place rendered messages into semantic fields.
- Ensure action diagnostics preserve the original code and data.

## Recommended delegation order

### Wave 1: serial foundation

Assign one agent to diagnostic codes/data and one agent to provider/edit infrastructure, but merge them serially because both touch shared plumbing.

1. **Diagnostic foundation agent**
   - Owns `diagnostic_data_test.go`.
   - Adds all code constants and diagnostic data structures.
   - Updates parser/processor error propagation.
2. **Provider foundation agent**
   - Adds dispatch and shared edit utilities.
   - Migrates action selection away from message matching.
   - Keeps compatibility with `analyzer.CodeActions`.

### Wave 2: parallel local sections

These groups have related semantics and can share one agent each:

| Agent | Test files |
|---|---|
| Syntax and directives | `file_and_syntax_structure_test.go`, `output_directives_test.go` |
| Imports | `imports_and_file_relationships_test.go` |
| Declarations and literals | `declarations_and_symbols_test.go`, `primitive_and_literal_types_test.go` |
| Collections and records | `arrays_test.go`, `records_and_closed_schemas_test.go` |
| Type domains | `variants_test.go`, `choices_test.go`, `fusions_test.go` |
| Expressions | `match_expressions_test.go`, `conditional_expressions_test.go` |
| Access and recursion | `optional_member_access_test.go`, `self_and_recursive_schemas_test.go` |
| Values and operators | `null_output_test.go`, `operators_test.go`, `interpolation_test.go` |
| Documentation | `documentation_test.go` |

### Wave 3: integration sections

Run these after local providers are merged:

1. **Fix-all agent** — owns `fix_all_test.go` and composes deterministic local fixes.
2. **Workspace agent** — owns `cross_file_actions_test.go` and builds atomic propagation actions.

## Rules for delegated agents

- Do not change test expectations without coordinator approval.
- Run the assigned focused suite before and after each change.
- Add implementation in an owned feature file instead of growing `analysis.go` further.
- Avoid editing shared registries after the foundation wave; request the coordinator to add missing shared entries.
- Do not match diagnostic messages.
- Do not mark ambiguous or value-changing actions preferred.
- Do not silently change Mace syntax.
- If a real syntax change becomes necessary, update the language docs, parser, and Zed extension together as required by the repository instructions.
- Keep workspace edits atomic and URI/range safe.
- Report which specs pass, which remain red, and any shared dependency that blocks the section.

## Delegation prompt template

```text
Implement the Mace code actions specified by:

  internal/analyzer/code_actions/<assigned-file>_test.go

Read internal/analyzer/code_actions/README.md first. Do not edit the tests or
match diagnostic messages. Use stable diagnostic codes and structured data.
Keep implementation in your assigned code-action feature file. Run:

  go test ./internal/analyzer/code_actions --ginkgo.focus='<section name>'
  go test ./... -run '^$'

Report passing specs, remaining failures, and any foundation dependency.
```

## Definition of done for a section

A section is complete only when:

- Every spec in its test file passes.
- No unrelated code-action specs regress.
- All packages compile.
- Diagnostics use stable codes and structured semantic data.
- Action metadata matches the catalog.
- Edits preserve valid Mace syntax and formatting.
- Ambiguous alternatives remain non-preferred.
- Cross-file edits are atomic where applicable.
