# Processor Test Matrix

## Supported spec types

- `string` -> `processor_scalar_types_test.go`
- `int` -> `processor_scalar_types_test.go`
- `float` -> `processor_scalar_types_test.go`
- `hex_int` -> `processor_scalar_types_test.go`
- `hex_float` -> `processor_scalar_types_test.go`
- `boolean` -> `processor_scalar_types_test.go`
- `nullable <type>` / `null` -> `processor_record_types_test.go`, `processor_scalar_types_test.go`
- `array<T>` -> `processor_array_types_test.go`
- `record<T>` -> `processor_record_types_test.go`
- schema record `{ ... }` -> `processor_record_types_test.go`
- named aliases -> `processor_type_system_test.go`
- `choice[...]` -> `processor_choice_types_test.go`
- `variant[...]` -> `processor_variant_types_test.go`
- `union[...]` -> `processor_variant_types_test.go`

## Supported processor mechanics

- input compatibility helpers -> `processor_input_mechanics_test.go`
- path resolution and bounded import roots -> `processor_path_mechanics_test.go`
- processor entrypoints and wrapper fallbacks -> `processor_entrypoints_mechanics_test.go`
- validation helpers and guarded references -> `processor_validation_mechanics_test.go`
- script parsing and script-only processing -> `processor_script_mechanics_test.go`
- imports, schema exports, and `import-as` flows -> `processor_import_mechanics_test.go`
- output directives, schema output, and output validation -> `processor_output_mechanics_test.go`
- runtime registries, diagnostics, and utility helpers -> `processor_runtime_helpers_test.go`
- type resolution, aliasing, assignability, and schema type conversion -> `processor_type_system_test.go`
