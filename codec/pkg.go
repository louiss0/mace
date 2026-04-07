package codec

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"

	"github.com/louiss0/mace/internal/processor"
)

type Result struct {
	Data   map[string]any
	Schema map[SchemaField]SchemaType
}

type SchemaField struct {
	Name     string
	Optional bool
}

type SchemaTypeKind int

const (
	SchemaTypeUnknown SchemaTypeKind = iota
	SchemaTypePrimitive
	SchemaTypeNamed
	SchemaTypeArray
	SchemaTypeRecord
)

type SchemaType struct {
	Kind    SchemaTypeKind
	Name    string
	Element *SchemaType
	Fields  map[SchemaField]SchemaType
}

func Parse(input string) (Result, error) {
	return ParseWithInjections(input, nil)
}

func ParseWithInjections(input string, injections map[string]any) (Result, error) {
	processed, err := newProcessor(injections).Process(input)
	if err != nil {
		return Result{}, err
	}

	return newResult(processed), nil
}

func ParseFile(path string) (Result, error) {
	return ParseFileWithInjections(path, nil)
}

func ParseFileWithInjections(path string, injections map[string]any) (Result, error) {
	processed, err := newProcessor(injections).ProcessFile(path)
	if err != nil {
		return Result{}, err
	}

	return newResult(processed), nil
}

func OutputMap(result Result) map[string]any {
	return result.Data
}

func Unmarshal(input string, target any) error {
	return UnmarshalWithInjections(input, nil, target)
}

func UnmarshalWithInjections(input string, injections map[string]any, target any) error {
	if target == nil {
		return fmt.Errorf("unmarshal mace: target is required")
	}

	result, err := newProcessor(injections).Process(input)
	if err != nil {
		return err
	}

	targetValue := reflect.ValueOf(target)
	if targetValue.Kind() != reflect.Pointer || targetValue.IsNil() {
		return fmt.Errorf("unmarshal mace: target must be a non-nil pointer")
	}

	return decodeRecord(result.Output, targetValue.Elem())
}

func Marshal(value any) (string, error) {
	marshaller := newMarshaller()
	return marshaller.marshalValue(reflect.ValueOf(value), 0)
}

func MarshalOutput(value any) (string, error) {
	marshaled, err := Marshal(value)
	if err != nil {
		return "", err
	}

	if !strings.HasPrefix(marshaled, "{") {
		return "", fmt.Errorf("marshal output: expected record value at root")
	}

	return marshaled, nil
}

func ImportJSON(input string) (string, error) {
	value, err := parseImportedJSON(input)
	if err != nil {
		return "", err
	}

	if isJSONSchemaDocument(value) {
		return importJSONSchemaDocument(value)
	}

	return importDocument(value)
}

func ImportYAML(input string) (string, error) {
	value, err := parseImportedYAML(input)
	if err != nil {
		return "", err
	}

	return importDocument(value)
}

func ImportTOML(input string) (string, error) {
	value, err := parseImportedTOML(input)
	if err != nil {
		return "", err
	}

	return importDocument(value)
}

func ImportJSONSchema(input string) (string, error) {
	value, err := parseImportedJSON(input)
	if err != nil {
		return "", err
	}

	return importJSONSchemaDocument(value)
}

func parseImportedJSON(input string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("import json: %w", err)
	}

	return value, nil
}

func parseImportedYAML(input string) (any, error) {
	var value any
	if err := yaml.Unmarshal([]byte(input), &value); err != nil {
		return nil, fmt.Errorf("import yaml: %w", err)
	}

	return value, nil
}

func parseImportedTOML(input string) (any, error) {
	var value any
	if err := toml.Unmarshal([]byte(input), &value); err != nil {
		return nil, fmt.Errorf("import toml: %w", err)
	}

	return value, nil
}

func importDocument(value any) (string, error) {
	normalized, err := normalizeImportedValue(reflect.ValueOf(value))
	if err != nil {
		return "", err
	}

	output, err := MarshalOutput(normalized)
	if err != nil {
		return "", fmt.Errorf("import mace: expected record root")
	}

	return "[output = data]\n" + output, nil
}

func importJSONSchemaDocument(value any) (string, error) {
	normalized, err := normalizeImportedValue(reflect.ValueOf(value))
	if err != nil {
		return "", err
	}

	record, ok := normalized.(map[string]any)
	if !ok {
		return "", fmt.Errorf("import mace schema: expected record root")
	}

	schema, err := jsonSchemaRecord(record)
	if err != nil {
		return "", err
	}

	return "[output = schema]\n" + formatSchemaRecord(schema.fields, 0), nil
}

type marshaller struct{}

func newMarshaller() *marshaller {
	return &marshaller{}
}

func newResult(processed processor.Result) Result {
	return Result{
		Data:   valuesToMap(processed.Output),
		Schema: schemaToMap(processed.Schema),
	}
}

func newProcessor(injections map[string]any) *processor.Processor {
	if len(injections) == 0 {
		return processor.New()
	}

	converted := make(map[string]processor.Value, len(injections))
	for name, value := range injections {
		converted[name] = toProcessorValue(value)
	}

	return processor.NewWithInjections(converted)
}

func valuesToMap(values map[string]processor.Value) map[string]any {
	output := make(map[string]any, len(values))
	for name, value := range values {
		output[name] = valueToAny(value)
	}

	return output
}

func valueToAny(value processor.Value) any {
	switch value.Kind {
	case processor.ValueString:
		return value.String
	case processor.ValueInt:
		return value.Int
	case processor.ValueFloat:
		return value.Float
	case processor.ValueBoolean:
		return value.Boolean
	case processor.ValueArray:
		items := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			items = append(items, valueToAny(item))
		}
		return items
	case processor.ValueRecord:
		return valuesToMap(value.Record)
	default:
		return nil
	}
}

func schemaToMap(fields map[processor.SchemaField]processor.SchemaType) map[SchemaField]SchemaType {
	schema := make(map[SchemaField]SchemaType, len(fields))
	for field, valueType := range fields {
		schema[SchemaField{
			Name:     field.Name,
			Optional: field.Optional,
		}] = schemaTypeFromProcessor(valueType)
	}

	return schema
}

func schemaTypeFromProcessor(schemaType processor.SchemaType) SchemaType {
	result := SchemaType{
		Kind: SchemaTypeKind(schemaType.Kind),
		Name: schemaType.Name,
	}

	if schemaType.Element != nil {
		element := schemaTypeFromProcessor(*schemaType.Element)
		result.Element = &element
	}

	if len(schemaType.Fields) > 0 {
		fields := make(map[SchemaField]SchemaType, len(schemaType.Fields))
		for field, fieldType := range schemaType.Fields {
			fields[SchemaField{Name: field.Name, Optional: field.Optional}] = schemaTypeFromProcessor(fieldType)
		}
		result.Fields = fields
	}

	return result
}

func toProcessorValue(value any) processor.Value {
	return processorValueFromReflect(reflect.ValueOf(value))
}

func processorValueFromReflect(value reflect.Value) processor.Value {
	if !value.IsValid() {
		return processor.Value{Kind: processor.ValueUnknown}
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return processor.Value{Kind: processor.ValueUnknown}
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.String:
		return processor.Value{Kind: processor.ValueString, String: value.String()}
	case reflect.Bool:
		return processor.Value{Kind: processor.ValueBoolean, Boolean: value.Bool()}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return processor.Value{Kind: processor.ValueInt, Int: value.Int()}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return processor.Value{Kind: processor.ValueInt, Int: int64(value.Uint())}
	case reflect.Float32, reflect.Float64:
		return processor.Value{Kind: processor.ValueFloat, Float: value.Float()}
	case reflect.Slice, reflect.Array:
		items := make([]processor.Value, 0, value.Len())
		for index := 0; index < value.Len(); index++ {
			items = append(items, processorValueFromReflect(value.Index(index)))
		}
		return processor.Value{Kind: processor.ValueArray, Array: items}
	case reflect.Map:
		record := map[string]processor.Value{}
		for _, key := range value.MapKeys() {
			record[key.String()] = processorValueFromReflect(value.MapIndex(key))
		}
		return processor.Value{Kind: processor.ValueRecord, Record: record}
	default:
		return processor.Value{Kind: processor.ValueUnknown}
	}
}

func normalizeImportedValue(value reflect.Value) (any, error) {
	if !value.IsValid() {
		return nil, fmt.Errorf("import mace: nil is not supported")
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, fmt.Errorf("import mace: nil is not supported")
		}
		value = value.Elem()
	}

	if number, ok := value.Interface().(json.Number); ok {
		return importedJSONNumber(number)
	}

	if timestamp, ok := value.Interface().(time.Time); ok {
		return timestamp.Format(time.RFC3339Nano), nil
	}

	switch value.Kind() {
	case reflect.String:
		return value.String(), nil
	case reflect.Bool:
		return value.Bool(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return int64(value.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return value.Float(), nil
	case reflect.Slice, reflect.Array:
		items := make([]any, 0, value.Len())
		for index := 0; index < value.Len(); index++ {
			item, err := normalizeImportedValue(value.Index(index))
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	case reflect.Map:
		record := map[string]any{}
		for _, key := range value.MapKeys() {
			name, err := importedMapKey(key)
			if err != nil {
				return nil, err
			}

			item, err := normalizeImportedValue(value.MapIndex(key))
			if err != nil {
				return nil, err
			}
			record[name] = item
		}
		return record, nil
	default:
		return nil, fmt.Errorf("import mace: unsupported value %T", value.Interface())
	}
}

func importedMapKey(value reflect.Value) (string, error) {
	if !value.IsValid() {
		return "", fmt.Errorf("import mace: invalid map key")
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", fmt.Errorf("import mace: invalid map key")
		}
		value = value.Elem()
	}

	if value.Kind() != reflect.String {
		return "", fmt.Errorf("import mace: maps must use string keys")
	}

	return value.String(), nil
}

func importedJSONNumber(value json.Number) (any, error) {
	if integer, err := value.Int64(); err == nil {
		return integer, nil
	}

	floatValue, err := value.Float64()
	if err != nil {
		return nil, fmt.Errorf("import mace: invalid json number %q", value.String())
	}

	return floatValue, nil
}

func (m *marshaller) marshalValue(value reflect.Value, depth int) (string, error) {
	if !value.IsValid() {
		return "", fmt.Errorf("marshal mace: nil is not supported")
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return "", fmt.Errorf("marshal mace: nil is not supported")
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.String:
		return strconv.Quote(value.String()), nil
	case reflect.Bool:
		if value.Bool() {
			return "true", nil
		}
		return "false", nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(value.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(value.Uint(), 10), nil
	case reflect.Float32:
		return strconv.FormatFloat(value.Float(), 'f', -1, 32), nil
	case reflect.Float64:
		return strconv.FormatFloat(value.Float(), 'f', -1, 64), nil
	case reflect.Slice, reflect.Array:
		return m.marshalArray(value, depth)
	case reflect.Map:
		return m.marshalMap(value, depth)
	case reflect.Struct:
		return m.marshalStruct(value, depth)
	default:
		return "", fmt.Errorf("marshal mace: unsupported kind %s", value.Kind())
	}
}

func (m *marshaller) marshalArray(value reflect.Value, depth int) (string, error) {
	if value.Len() == 0 {
		return "[]", nil
	}

	items := make([]string, 0, value.Len())
	for index := 0; index < value.Len(); index++ {
		item, err := m.marshalValue(value.Index(index), depth+1)
		if err != nil {
			return "", err
		}
		items = append(items, item)
	}

	return "[" + strings.Join(items, ", ") + "]", nil
}

func (m *marshaller) marshalMap(value reflect.Value, depth int) (string, error) {
	if value.Type().Key().Kind() != reflect.String {
		return "", fmt.Errorf("marshal mace: maps must use string keys")
	}

	keys := value.MapKeys()
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		names = append(names, key.String())
	}
	slices.Sort(names)

	fields := make([]recordField, 0, len(names))
	for _, name := range names {
		fieldValue, err := m.marshalValue(value.MapIndex(reflect.ValueOf(name)), depth+1)
		if err != nil {
			return "", err
		}
		fields = append(fields, recordField{name: name, value: fieldValue})
	}

	return formatRecord(fields, depth), nil
}

func (m *marshaller) marshalStruct(value reflect.Value, depth int) (string, error) {
	fields := []recordField{}
	valueType := value.Type()

	for index := 0; index < value.NumField(); index++ {
		fieldType := valueType.Field(index)
		if !fieldType.IsExported() {
			continue
		}

		name, omitEmpty := fieldName(fieldType)
		if name == "-" {
			continue
		}

		fieldValue := value.Field(index)
		if omitEmpty && isEmptyValue(fieldValue) {
			continue
		}

		marshaled, err := m.marshalValue(fieldValue, depth+1)
		if err != nil {
			return "", err
		}

		fields = append(fields, recordField{name: name, value: marshaled})
	}

	return formatRecord(fields, depth), nil
}

type recordField struct {
	name  string
	value string
}

type inferredTypeKind int

const (
	inferredTypePrimitive inferredTypeKind = iota
	inferredTypeArray
	inferredTypeRecord
)

type schemaField struct {
	name     string
	optional bool
	value    inferredType
}

type recordSchema struct {
	fields []schemaField
}

type inferredType struct {
	kind      inferredTypeKind
	primitive string
	element   *inferredType
	record    recordSchema
}

func formatRecord(fields []recordField, depth int) string {
	if len(fields) == 0 {
		return "{}"
	}

	indent := strings.Repeat("  ", depth+1)
	closingIndent := strings.Repeat("  ", depth)
	lines := []string{"{"}

	for _, field := range fields {
		lines = append(lines, fmt.Sprintf("%s%s: %s;", indent, field.name, field.value))
	}

	lines = append(lines, closingIndent+"}")
	return strings.Join(lines, "\n")
}

func formatSchemaRecord(fields []schemaField, depth int) string {
	if len(fields) == 0 {
		return "{}"
	}

	indent := strings.Repeat("  ", depth+1)
	closingIndent := strings.Repeat("  ", depth)
	lines := []string{"{"}

	for _, field := range fields {
		optionalMarker := ""
		if field.optional {
			optionalMarker = "?"
		}
		lines = append(lines, fmt.Sprintf("%s%s%s: %s;", indent, field.name, optionalMarker, formatSchemaType(field.value, depth+1)))
	}

	lines = append(lines, closingIndent+"}")
	return strings.Join(lines, "\n")
}

func formatSchemaType(value inferredType, depth int) string {
	switch value.kind {
	case inferredTypePrimitive:
		return value.primitive
	case inferredTypeArray:
		if value.element == nil {
			return "array<string>"
		}
		return fmt.Sprintf("array<%s>", formatSchemaType(*value.element, depth))
	case inferredTypeRecord:
		return formatSchemaRecord(value.record.fields, depth)
	default:
		return "string"
	}
}

func inferRecordSchemaSamples(records []map[string]any) (recordSchema, error) {
	fieldNames := map[string]struct{}{}
	for _, record := range records {
		for name := range record {
			fieldNames[name] = struct{}{}
		}
	}

	names := make([]string, 0, len(fieldNames))
	for name := range fieldNames {
		names = append(names, name)
	}
	slices.Sort(names)

	fields := make([]schemaField, 0, len(names))
	for _, name := range names {
		samples := make([]any, 0, len(records))
		occurrences := 0
		for _, record := range records {
			value, ok := record[name]
			if !ok {
				continue
			}
			occurrences++
			samples = append(samples, value)
		}

		valueType, err := inferMergedSchemaType(samples)
		if err != nil {
			return recordSchema{}, fmt.Errorf("infer schema for %q: %w", name, err)
		}
		fields = append(fields, schemaField{
			name:     name,
			optional: occurrences < len(records),
			value:    valueType,
		})
	}

	return recordSchema{fields: fields}, nil
}

func inferMergedSchemaType(samples []any) (inferredType, error) {
	if len(samples) == 0 {
		return inferredType{}, fmt.Errorf("missing samples")
	}

	merged, err := inferSchemaType(samples[0])
	if err != nil {
		return inferredType{}, err
	}

	for index := 1; index < len(samples); index++ {
		nextType, err := inferSchemaType(samples[index])
		if err != nil {
			return inferredType{}, err
		}

		merged, err = mergeSchemaTypes(merged, nextType)
		if err != nil {
			return inferredType{}, err
		}
	}

	return merged, nil
}

func inferSchemaType(value any) (inferredType, error) {
	switch typed := value.(type) {
	case string:
		return inferredType{kind: inferredTypePrimitive, primitive: "string"}, nil
	case bool:
		return inferredType{kind: inferredTypePrimitive, primitive: "boolean"}, nil
	case int, int8, int16, int32, int64:
		return inferredType{kind: inferredTypePrimitive, primitive: "int"}, nil
	case uint, uint8, uint16, uint32, uint64, uintptr:
		return inferredType{kind: inferredTypePrimitive, primitive: "int"}, nil
	case float32, float64:
		return inferredType{kind: inferredTypePrimitive, primitive: "float"}, nil
	case []any:
		if len(typed) == 0 {
			return inferredType{
				kind:    inferredTypeArray,
				element: &inferredType{kind: inferredTypePrimitive, primitive: "string"},
			}, nil
		}

		elementType, err := inferMergedSchemaType(typed)
		if err != nil {
			return inferredType{}, err
		}

		return inferredType{kind: inferredTypeArray, element: &elementType}, nil
	case map[string]any:
		record, err := inferRecordSchemaSamples([]map[string]any{typed})
		if err != nil {
			return inferredType{}, err
		}
		return inferredType{kind: inferredTypeRecord, record: record}, nil
	default:
		return inferredType{}, fmt.Errorf("unsupported value %T", value)
	}
}

func mergeSchemaTypes(left inferredType, right inferredType) (inferredType, error) {
	if left.kind != right.kind {
		return inferredType{}, fmt.Errorf("heterogeneous array")
	}

	switch left.kind {
	case inferredTypePrimitive:
		if left.primitive != right.primitive {
			return inferredType{}, fmt.Errorf("heterogeneous array")
		}
		return left, nil
	case inferredTypeArray:
		if left.element == nil {
			return right, nil
		}
		if right.element == nil {
			return left, nil
		}
		element, err := mergeSchemaTypes(*left.element, *right.element)
		if err != nil {
			return inferredType{}, err
		}
		return inferredType{kind: inferredTypeArray, element: &element}, nil
	case inferredTypeRecord:
		return mergeRecordTypes(left.record, right.record)
	default:
		return inferredType{}, fmt.Errorf("heterogeneous array")
	}
}

func mergeRecordTypes(left recordSchema, right recordSchema) (inferredType, error) {
	leftFields := schemaFieldIndex(left.fields)
	rightFields := schemaFieldIndex(right.fields)
	fieldNames := map[string]struct{}{}
	for name := range leftFields {
		fieldNames[name] = struct{}{}
	}
	for name := range rightFields {
		fieldNames[name] = struct{}{}
	}

	names := make([]string, 0, len(fieldNames))
	for name := range fieldNames {
		names = append(names, name)
	}
	slices.Sort(names)

	fields := make([]schemaField, 0, len(names))
	for _, name := range names {
		leftField, hasLeft := leftFields[name]
		rightField, hasRight := rightFields[name]

		optional := !hasLeft || !hasRight || leftField.optional || rightField.optional
		switch {
		case hasLeft && hasRight:
			mergedType, err := mergeSchemaTypes(leftField.value, rightField.value)
			if err != nil {
				return inferredType{}, err
			}
			fields = append(fields, schemaField{name: name, optional: optional, value: mergedType})
		case hasLeft:
			fields = append(fields, schemaField{name: name, optional: true, value: leftField.value})
		case hasRight:
			fields = append(fields, schemaField{name: name, optional: true, value: rightField.value})
		}
	}

	return inferredType{kind: inferredTypeRecord, record: recordSchema{fields: fields}}, nil
}

func schemaFieldIndex(fields []schemaField) map[string]schemaField {
	index := make(map[string]schemaField, len(fields))
	for _, field := range fields {
		index[field.name] = field
	}
	return index
}

func isJSONSchemaDocument(value any) bool {
	record, ok := value.(map[string]any)
	if !ok {
		return false
	}

	_, hasSchema := record["$schema"]
	return hasSchema
}

func jsonSchemaRecord(record map[string]any) (recordSchema, error) {
	typeName, _ := record["type"].(string)
	if typeName != "" && typeName != "object" {
		return recordSchema{}, fmt.Errorf("import mace schema: root schema must use type object")
	}

	propertiesValue, ok := record["properties"]
	if !ok {
		return recordSchema{fields: []schemaField{}}, nil
	}

	properties, ok := propertiesValue.(map[string]any)
	if !ok {
		return recordSchema{}, fmt.Errorf("import mace schema: properties must be an object")
	}

	requiredNames := jsonSchemaRequiredNames(record["required"])
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	slices.Sort(names)

	fields := make([]schemaField, 0, len(names))
	for _, name := range names {
		property, ok := properties[name].(map[string]any)
		if !ok {
			return recordSchema{}, fmt.Errorf("import mace schema: property %q must be an object", name)
		}

		valueType, err := jsonSchemaType(property)
		if err != nil {
			return recordSchema{}, fmt.Errorf("import mace schema: property %q: %w", name, err)
		}

		_, required := requiredNames[name]
		fields = append(fields, schemaField{name: name, optional: !required, value: valueType})
	}

	return recordSchema{fields: fields}, nil
}

func jsonSchemaRequiredNames(value any) map[string]struct{} {
	requiredNames := map[string]struct{}{}
	items, ok := value.([]any)
	if !ok {
		return requiredNames
	}

	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			continue
		}
		requiredNames[name] = struct{}{}
	}

	return requiredNames
}

func jsonSchemaType(record map[string]any) (inferredType, error) {
	typeName, _ := record["type"].(string)
	if typeName == "" {
		if _, ok := record["properties"]; ok {
			typeName = "object"
		}
	}

	switch typeName {
	case "string":
		return inferredType{kind: inferredTypePrimitive, primitive: "string"}, nil
	case "integer":
		return inferredType{kind: inferredTypePrimitive, primitive: "int"}, nil
	case "number":
		return inferredType{kind: inferredTypePrimitive, primitive: "float"}, nil
	case "boolean":
		return inferredType{kind: inferredTypePrimitive, primitive: "boolean"}, nil
	case "array":
		itemsValue, ok := record["items"]
		if !ok {
			return inferredType{kind: inferredTypeArray, element: &inferredType{kind: inferredTypePrimitive, primitive: "string"}}, nil
		}

		itemsRecord, ok := itemsValue.(map[string]any)
		if !ok {
			return inferredType{}, fmt.Errorf("items must be an object")
		}

		elementType, err := jsonSchemaType(itemsRecord)
		if err != nil {
			return inferredType{}, err
		}
		return inferredType{kind: inferredTypeArray, element: &elementType}, nil
	case "object":
		nestedRecord, err := jsonSchemaRecord(record)
		if err != nil {
			return inferredType{}, err
		}
		return inferredType{kind: inferredTypeRecord, record: nestedRecord}, nil
	default:
		return inferredType{}, fmt.Errorf("unsupported json schema type %q", typeName)
	}
}

func fieldName(field reflect.StructField) (string, bool) {
	tag := field.Tag.Get("json")
	if tag == "" {
		return lowerLeading(field.Name), false
	}

	parts := strings.Split(tag, ",")
	name := parts[0]
	if name == "" {
		name = lowerLeading(field.Name)
	}

	omitEmpty := false
	for _, part := range parts[1:] {
		if part == "omitempty" {
			omitEmpty = true
		}
	}

	return name, omitEmpty
}

func lowerLeading(value string) string {
	if value == "" {
		return ""
	}

	return strings.ToLower(value[:1]) + value[1:]
}

func isEmptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	case reflect.Bool:
		return !value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return value.Float() == 0
	case reflect.Interface, reflect.Pointer:
		return value.IsNil()
	case reflect.Struct:
		return false
	default:
		return false
	}
}

func decodeRecord(fields map[string]processor.Value, target reflect.Value) error {
	target = ensureTargetValue(target)

	switch target.Kind() {
	case reflect.Map:
		if target.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("unmarshal mace: maps must use string keys")
		}
		if target.IsNil() {
			target.Set(reflect.MakeMap(target.Type()))
		}
		for name, value := range fields {
			item, err := decodeValue(value, target.Type().Elem())
			if err != nil {
				return err
			}
			target.SetMapIndex(reflect.ValueOf(name), item)
		}
		return nil
	case reflect.Struct:
		fieldMap := structFieldMap(target.Type())
		for name, value := range fields {
			index, ok := fieldMap[name]
			if !ok {
				continue
			}
			field := target.Field(index)
			decoded, err := decodeValue(value, field.Type())
			if err != nil {
				return err
			}
			field.Set(decoded)
		}
		return nil
	default:
		return fmt.Errorf("unmarshal mace: target must point to a map or struct")
	}
}

func decodeValue(value processor.Value, targetType reflect.Type) (reflect.Value, error) {
	if targetType.Kind() == reflect.Pointer {
		decoded, err := decodeValue(value, targetType.Elem())
		if err != nil {
			return reflect.Value{}, err
		}
		pointer := reflect.New(targetType.Elem())
		pointer.Elem().Set(decoded)
		return pointer, nil
	}

	switch value.Kind {
	case processor.ValueString:
		return decodeString(value.String, targetType)
	case processor.ValueInt:
		return decodeInt(value.Int, targetType)
	case processor.ValueFloat:
		return decodeFloat(value.Float, targetType)
	case processor.ValueBoolean:
		return decodeBool(value.Boolean, targetType)
	case processor.ValueArray:
		return decodeArray(value.Array, targetType)
	case processor.ValueRecord:
		return decodeRecordValue(value.Record, targetType)
	default:
		return reflect.Value{}, fmt.Errorf("unmarshal mace: unsupported value kind")
	}
}

func decodeString(value string, targetType reflect.Type) (reflect.Value, error) {
	if targetType.Kind() != reflect.String && targetType.Kind() != reflect.Interface {
		return reflect.Value{}, fmt.Errorf("unmarshal mace: cannot assign string to %s", targetType)
	}
	if targetType.Kind() == reflect.Interface {
		return reflect.ValueOf(value), nil
	}
	result := reflect.New(targetType).Elem()
	result.SetString(value)
	return result, nil
}

func decodeInt(value int64, targetType reflect.Type) (reflect.Value, error) {
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		result := reflect.New(targetType).Elem()
		result.SetInt(value)
		return result, nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if value < 0 {
			return reflect.Value{}, fmt.Errorf("unmarshal mace: cannot assign negative int to %s", targetType)
		}
		result := reflect.New(targetType).Elem()
		result.SetUint(uint64(value))
		return result, nil
	case reflect.Float32, reflect.Float64:
		result := reflect.New(targetType).Elem()
		result.SetFloat(float64(value))
		return result, nil
	case reflect.Interface:
		return reflect.ValueOf(value), nil
	default:
		return reflect.Value{}, fmt.Errorf("unmarshal mace: cannot assign int to %s", targetType)
	}
}

func decodeFloat(value float64, targetType reflect.Type) (reflect.Value, error) {
	switch targetType.Kind() {
	case reflect.Float32, reflect.Float64:
		result := reflect.New(targetType).Elem()
		result.SetFloat(value)
		return result, nil
	case reflect.Interface:
		return reflect.ValueOf(value), nil
	default:
		return reflect.Value{}, fmt.Errorf("unmarshal mace: cannot assign float to %s", targetType)
	}
}

func decodeBool(value bool, targetType reflect.Type) (reflect.Value, error) {
	if targetType.Kind() != reflect.Bool && targetType.Kind() != reflect.Interface {
		return reflect.Value{}, fmt.Errorf("unmarshal mace: cannot assign boolean to %s", targetType)
	}
	if targetType.Kind() == reflect.Interface {
		return reflect.ValueOf(value), nil
	}
	result := reflect.New(targetType).Elem()
	result.SetBool(value)
	return result, nil
}

func decodeArray(values []processor.Value, targetType reflect.Type) (reflect.Value, error) {
	switch targetType.Kind() {
	case reflect.Slice:
		result := reflect.MakeSlice(targetType, len(values), len(values))
		for index, item := range values {
			decoded, err := decodeValue(item, targetType.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(index).Set(decoded)
		}
		return result, nil
	case reflect.Array:
		if len(values) != targetType.Len() {
			return reflect.Value{}, fmt.Errorf("unmarshal mace: array length mismatch for %s", targetType)
		}
		result := reflect.New(targetType).Elem()
		for index, item := range values {
			decoded, err := decodeValue(item, targetType.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			result.Index(index).Set(decoded)
		}
		return result, nil
	case reflect.Interface:
		items := make([]any, 0, len(values))
		for _, item := range values {
			items = append(items, valueToAny(item))
		}
		return reflect.ValueOf(items), nil
	default:
		return reflect.Value{}, fmt.Errorf("unmarshal mace: cannot assign array to %s", targetType)
	}
}

func decodeRecordValue(fields map[string]processor.Value, targetType reflect.Type) (reflect.Value, error) {
	switch targetType.Kind() {
	case reflect.Map:
		if targetType.Key().Kind() != reflect.String {
			return reflect.Value{}, fmt.Errorf("unmarshal mace: maps must use string keys")
		}
		result := reflect.MakeMap(targetType)
		for name, value := range fields {
			decoded, err := decodeValue(value, targetType.Elem())
			if err != nil {
				return reflect.Value{}, err
			}
			result.SetMapIndex(reflect.ValueOf(name), decoded)
		}
		return result, nil
	case reflect.Struct:
		result := reflect.New(targetType).Elem()
		if err := decodeRecord(fields, result); err != nil {
			return reflect.Value{}, err
		}
		return result, nil
	case reflect.Interface:
		return reflect.ValueOf(valuesToMap(fields)), nil
	default:
		return reflect.Value{}, fmt.Errorf("unmarshal mace: cannot assign record to %s", targetType)
	}
}

func ensureTargetValue(value reflect.Value) reflect.Value {
	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		value = value.Elem()
	}
	return value
}

func structFieldMap(targetType reflect.Type) map[string]int {
	fields := map[string]int{}
	for index := 0; index < targetType.NumField(); index++ {
		field := targetType.Field(index)
		if !field.IsExported() {
			continue
		}
		name, _ := fieldName(field)
		if name == "-" {
			continue
		}
		fields[name] = index
	}
	return fields
}
