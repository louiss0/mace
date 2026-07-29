# Mace Specification Change Report

## Status and conformance

- Reasserted deterministic, all-or-nothing processing and record-only roots.
- Distinguished syntax, static semantics, and evaluation semantics.
- Added the normative processing order.
- Positive: a fully validated record is emitted once.
- Negative: emitting fields before a later validation failure is forbidden.

## Canonical grammar and synchronization

- Made `mace.ebnf` the canonical repository grammar.
- Replaced the RFC grammar block with an exact, unescaped copy.
- Added `internal/spec/spec_sync_test.go` so CI fails on divergence, HTML
  entities, or newline-export artifacts.
- Positive: changing both copies identically passes.
- Negative: editing only the RFC block fails the synchronization test.

## Lexical rules

- Preserved Unicode and permissive kebab identifiers.
- Selected exact-code-point identifier comparison because the implementation
  does not normalize identifiers.
- Defined global versus contextual keywords and complete-token matching.
- Stopped descriptions before `,`, `;`, `}`, and line endings.
- Excluded backslashes from ordinary block-string content.
- Positive: `用户配置`, `foo-1`, `foo-_internal`, and `matcher`.
- Negative: globally reserved `match` as a variable name; descriptions that
  attempt to consume a following declaration.

## Strings, choices, and documentation

- Introduced complete non-interpolating literal string productions.
- Restricted choice strings and structured documentation to literal strings.
- Preserved evaluated string expressions only for the output `doc` directive.
- Positive: `choice['dev', "prod"]`.
- Negative: `choice["$(environment)"]` and interpolated `gen_doc` values.

## Document structure and declarations

- Made import and schema semicolons consistently mandatory.
- Preserved one optional script block and exactly one output block.
- Clarified closed schemas, empty collection typing, fusion, and guarded `$self`.
- Positive: `schema Empty: {};`.
- Negative: `schema Empty: {}` without `;`.

## Variants and match expressions

- Replaced globally disjoint pattern sets with declared and effective domains.
- Added literal > choice > primitive specificity and source-order independence.
- Allowed a variant literal pattern only through a nested choice domain.
- Defined residual primitive domains and required explicit choice coverage.
- Positive: literal `"dev"`, choice `choice["dev", "prod"]`, and residual
  `string` arms can coexist.
- Negative: unrelated literal, equal-specificity overlap, or omitted `"prod"`
  coverage.

## Output modes, shorthand, and null

- Made output-field parsing explicitly contextual by selected mode.
- Defined shorthand as exact local/imported identifier lookup.
- Removed `null` from `primary_atom`; direct data-output fields alone accept it.
- Defined omission before schema validation and required-field failure.
- Positive: optional schema field emitted as direct `null` is omitted.
- Negative: null in a variable, nested record, array, conditional, comparison,
  interpolation, or coalescing expression.

## Operators and expressions

- Added a complete normative operator table.
- Defined decimal-family and hexadecimal-family promotion without cross-family
  mixing; string concatenation/ordering; deep equality; checked integers;
  modulo signs; exponent constraints; shifts; laziness; and conditional typing.
- Corrected precedence to include null coalescing.
- Positive: `1 + 1.5`, `0x1 + 0x1.8`, and lazy `false && ...`.
- Negative: `1 + 0x1`, `true & false`, shift count 64, and cross-type equality.

## Imports and file directives

- Defined named import, renamed import, and whole-record `bind`.
- Restricted bind to successfully processed data-output files.
- Defined file exposure and private declarations.
- Defined selector scope for `schema_file`/`schema` and `parse_file`/`parse`.
- Unified cycle and path-security treatment.
- Positive: binding a data-output record or selecting a schema from its file.
- Negative: binding schema output, selector leakage to unrelated locals,
  collisions, cycles, and canonical-root escapes.

## Hexadecimal floating point

- Removed saturation language.
- Selected IEEE-754 round-to-nearest, ties-to-even, with overflow-to-infinity
  rejected.
- Defined underflow, subnormals, signed zero, and canonical round trips.
- Positive: finite fixed-point values and signed zero round-trip.
- Negative: a literal whose nearest binary64 value is infinity.

## Diagnostics

- Removed the contradictory blanket prohibition on variant literal patterns.
- Added categories for invalid literal domains, equal-specificity overlaps,
  missing choice residuals, illegal null use, interpolation in literal metadata,
  bind/file failures, keywords, descriptions, and operator types.
- Identified existing fixtures and expectations that must change rather than
  claiming implementation work not performed.

## Remaining author decisions

None were required for the revised documents. Conservative rules were selected
and recorded for identifier comparison, numeric promotion, string operators,
deep equality, shift behavior, and hex-float overflow.
