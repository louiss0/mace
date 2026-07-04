# Processor Test Matrix

Aligned to `Mace Language Spec RFC.md`.

## Spec-type files

- `variables` -> `processor_variables_test.go`
- `string` -> `processor_strings_test.go`
- `int` -> `processor_integers_test.go`
- `null` -> `processor_null_test.go`
- `hex_int` -> `processor_hex_int_test.go`
- `hex_float` -> `processor_hex_float_test.go`
- `array` -> `processor_array_test.go`
- `record` / schema-backed records -> `processor_record_test.go`
- `variant` -> `processor_variant_test.go`
- `union` -> `processor_union_test.go`
- `choice` -> `processor_choice_test.go`
- `$self` -> `processor_self_test.go`
- `output_block` -> `processor_output_block_test.go`

## Supporting processor mechanics

- input compatibility helpers -> `processor_input_mechanics_test.go`
- path resolution and import bounds -> `processor_path_mechanics_test.go`
- processor entrypoints and wrappers -> `processor_entrypoints_mechanics_test.go`
- validation helpers -> `processor_validation_mechanics_test.go`
- imports and import exports -> `processor_import_mechanics_test.go`
- runtime registries and diagnostics -> `processor_runtime_helpers_test.go`
- type resolution and assignability helpers -> `processor_type_system_test.go`
