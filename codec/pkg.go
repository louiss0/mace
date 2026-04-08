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
	SchemaTypeUnion
	SchemaTypeVariant
)

type SchemaType struct {
	Kind    SchemaTypeKind
	Name    string
	Element *SchemaType
	Fields  map[SchemaField]SchemaType
	Members []SchemaType
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

	record, ok := normalized.(map[string]any)
	if !ok {
		return "", fmt.Errorf("import mace: expected record root")
	}
	if len(record) == 0 {
		return "", fmt.Errorf("import mace: output block is empty after omitting null values")
	}

	output, err := MarshalOutput(record)
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

	context := newJSONSchemaContext(record)
	schema, err := context.record(record, nil)
	if err != nil {
		return "", err
	}

	if len(context.declarations) == 0 {
		return "[output = schema]\n" + formatSchemaRecord(schema.fields, 0), nil
	}

	declarations := strings.Join(context.declarations, "\n")
	return fmt.Sprintf("|===|\n%s\n|===|\n[output = schema]\n%s", declarations, formatSchemaRecord(schema.fields, 0)), nil
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

	if len(schemaType.Members) > 0 {
		members := make([]SchemaType, 0, len(schemaType.Members))
		for _, member := range schemaType.Members {
			members = append(members, schemaTypeFromProcessor(member))
		}
		result.Members = members
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

type omittedValue struct{}

func normalizeImportedValue(value reflect.Value) (any, error) {
	if !value.IsValid() {
		return omittedValue{}, nil
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return omittedValue{}, nil
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
			if _, ok := item.(omittedValue); ok {
				continue
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
			if _, ok := item.(omittedValue); ok {
				continue
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
	inferredTypeNamed
	inferredTypeUnion
	inferredTypeVariant
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
	kind          inferredTypeKind
	primitive     string
	name          string
	namedCategory string
	backingType   string
	element       *inferredType
	record        recordSchema
	members       []inferredType
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
	case inferredTypeNamed:
		return value.name
	case inferredTypeArray:
		if value.element == nil {
			return "array<string>"
		}
		return fmt.Sprintf("array<%s>", formatSchemaType(*value.element, depth))
	case inferredTypeRecord:
		return formatSchemaRecord(value.record.fields, depth)
	case inferredTypeUnion:
		parts := make([]string, 0, len(value.members))
		for _, member := range value.members {
			parts = append(parts, formatSchemaType(member, depth))
		}
		return fmt.Sprintf("union[%s]", strings.Join(parts, ", "))
	case inferredTypeVariant:
		parts := make([]string, 0, len(value.members))
		for _, member := range value.members {
			parts = append(parts, formatSchemaType(member, depth))
		}
		return fmt.Sprintf("variant[%s]", strings.Join(parts, ", "))
	default:
		return "string"
	}
}

func isJSONSchemaDocument(value any) bool {
	record, ok := value.(map[string]any)
	if !ok {
		return false
	}

	_, hasSchema := record["$schema"]
	return hasSchema
}

type jsonSchemaContext struct {
	root                map[string]any
	declarations        []string
	declarationIndex    map[string]int
	definitionNames     map[string]string
	definitionTypes     map[string]inferredType
	inlineEnumTypes     map[string]inferredType
	usedDeclarationName map[string]struct{}
}

func newJSONSchemaContext(root map[string]any) *jsonSchemaContext {
	return &jsonSchemaContext{
		root:                root,
		declarationIndex:    map[string]int{},
		definitionNames:     map[string]string{},
		definitionTypes:     map[string]inferredType{},
		inlineEnumTypes:     map[string]inferredType{},
		usedDeclarationName: map[string]struct{}{},
	}
}

func (context *jsonSchemaContext) record(record map[string]any, path []string) (recordSchema, error) {
	typeName, _ := record["type"].(string)
	if typeName != "" && typeName != "object" {
		return recordSchema{}, fmt.Errorf("import mace schema: root schema must use type object")
	}

	if additionalProperties, ok := record["additionalProperties"]; ok {
		switch typed := additionalProperties.(type) {
		case bool:
			if typed {
				return recordSchema{}, fmt.Errorf("import mace schema: additionalProperties=true is not supported")
			}
		case map[string]any:
			return recordSchema{}, fmt.Errorf("import mace schema: additionalProperties schemas are not supported")
		}
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

		valueType, nullable, err := context.propertyType(property, append(append([]string{}, path...), name))
		if err != nil {
			return recordSchema{}, fmt.Errorf("import mace schema: property %q: %w", name, err)
		}

		_, required := requiredNames[name]
		fields = append(fields, schemaField{name: name, optional: !required || nullable, value: valueType})
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

func jsonSchemaReference(path string, root map[string]any) (map[string]any, error) {
	if !strings.HasPrefix(path, "#/") {
		return nil, fmt.Errorf("unsupported $ref %q", path)
	}

	current := any(root)
	for _, segment := range strings.Split(strings.TrimPrefix(path, "#/"), "/") {
		record, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid $ref %q", path)
		}

		next, ok := record[segment]
		if !ok {
			return nil, fmt.Errorf("unknown $ref %q", path)
		}
		current = next
	}

	resolved, ok := current.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid $ref %q", path)
	}

	return resolved, nil
}

func (context *jsonSchemaContext) propertyType(record map[string]any, path []string) (inferredType, bool, error) {
	nullable := false

	if constValue, ok := record["const"]; ok {
		if constValue == nil {
			return inferredType{}, true, fmt.Errorf("null-only const is not representable")
		}
		valueType, err := context.enumType([]any{constValue}, path)
		return valueType, false, err
	}

	if enumValues, ok := record["enum"].([]any); ok && len(enumValues) > 0 {
		filtered := make([]any, 0, len(enumValues))
		for _, value := range enumValues {
			if value == nil {
				nullable = true
				continue
			}
			filtered = append(filtered, value)
		}
		if len(filtered) == 0 {
			return inferredType{}, false, fmt.Errorf("null-only enum is not representable")
		}
		valueType, err := context.enumType(filtered, path)
		return valueType, nullable, err
	}

	if variants, ok := record["oneOf"].([]any); ok {
		valueType, variantNullable, err := context.variantSchemaType(variants, path)
		return valueType, variantNullable, err
	}
	if variants, ok := record["anyOf"].([]any); ok {
		valueType, variantNullable, err := context.variantSchemaType(variants, path)
		return valueType, variantNullable, err
	}
	if variants, ok := record["allOf"].([]any); ok {
		valueType, variantNullable, err := context.unionSchemaType(variants, path)
		return valueType, variantNullable, err
	}

	typeArray, hasTypeArray := record["type"].([]any)
	if hasTypeArray {
		members := make([]inferredType, 0, len(typeArray))
		for _, item := range typeArray {
			typeName, ok := item.(string)
			if !ok {
				return inferredType{}, false, fmt.Errorf("type arrays must contain strings")
			}
			if typeName == "null" {
				nullable = true
				continue
			}
			memberType, err := context.typeNameSchemaType(typeName, record, path)
			if err != nil {
				return inferredType{}, false, err
			}
			members = append(members, memberType)
		}
		if len(members) == 0 {
			return inferredType{}, false, fmt.Errorf("null-only variants are not representable")
		}
		if len(members) == 1 {
			return members[0], nullable, nil
		}
		variantType, err := validateVariantMembers(members)
		return variantType, nullable, err
	}

	valueType, err := context.schemaType(record, path)
	return valueType, nullable, err
}

func (context *jsonSchemaContext) variantSchemaType(variants []any, path []string) (inferredType, bool, error) {
	nullable := false
	members := make([]inferredType, 0, len(variants))
	for index, variant := range variants {
		record, ok := variant.(map[string]any)
		if !ok {
			return inferredType{}, false, fmt.Errorf("variant alternatives must be objects")
		}

		valueType, variantNullable, err := context.propertyType(record, append(append([]string{}, path...), fmt.Sprintf("variant%d", index+1)))
		if err != nil {
			return inferredType{}, false, err
		}
		nullable = nullable || variantNullable
		members = append(members, valueType)
	}

	if len(members) == 0 {
		return inferredType{}, false, fmt.Errorf("empty variants are not representable")
	}
	if len(members) == 1 {
		return members[0], nullable, nil
	}

	variantType, err := validateVariantMembers(members)
	return variantType, nullable, err
}

func (context *jsonSchemaContext) unionSchemaType(variants []any, path []string) (inferredType, bool, error) {
	nullable := false
	members := make([]inferredType, 0, len(variants))
	for index, variant := range variants {
		record, ok := variant.(map[string]any)
		if !ok {
			return inferredType{}, false, fmt.Errorf("union members must be objects")
		}

		valueType, variantNullable, err := context.propertyType(record, append(append([]string{}, path...), fmt.Sprintf("member%d", index+1)))
		if err != nil {
			return inferredType{}, false, err
		}
		nullable = nullable || variantNullable
		members = append(members, valueType)
	}

	if len(members) == 0 {
		return inferredType{}, false, fmt.Errorf("empty unions are not representable")
	}
	if len(members) == 1 {
		return members[0], nullable, nil
	}

	unionType, err := validateUnionMembers(members)
	return unionType, nullable, err
}

func (context *jsonSchemaContext) schemaType(record map[string]any, path []string) (inferredType, error) {
	if refValue, ok := record["$ref"].(string); ok {
		return context.referenceType(refValue)
	}

	if constValue, ok := record["const"]; ok {
		if constValue == nil {
			return inferredType{}, fmt.Errorf("null-only const is not representable")
		}
		return context.enumType([]any{constValue}, path)
	}

	if enumValues, ok := record["enum"].([]any); ok && len(enumValues) > 0 {
		return context.enumType(enumValues, path)
	}

	typeName, _ := record["type"].(string)
	if typeName == "" {
		if _, ok := record["properties"]; ok {
			typeName = "object"
		}
	}

	return context.typeNameSchemaType(typeName, record, path)
}

func (context *jsonSchemaContext) typeNameSchemaType(typeName string, record map[string]any, path []string) (inferredType, error) {
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

		elementType, err := context.schemaType(itemsRecord, append(append([]string{}, path...), "item"))
		if err != nil {
			return inferredType{}, err
		}
		return inferredType{kind: inferredTypeArray, element: &elementType}, nil
	case "object":
		nestedRecord, err := context.record(record, path)
		if err != nil {
			return inferredType{}, err
		}
		return inferredType{kind: inferredTypeRecord, record: nestedRecord}, nil
	case "":
		return inferredType{}, fmt.Errorf("unsupported json schema type %q", typeName)
	default:
		return inferredType{}, fmt.Errorf("unsupported json schema type %q", typeName)
	}
}

func (context *jsonSchemaContext) referenceType(path string) (inferredType, error) {
	if declarationType, ok := context.definitionTypes[path]; ok {
		return declarationType, nil
	}

	resolved, err := jsonSchemaReference(path, context.root)
	if err != nil {
		return inferredType{}, err
	}

	baseName := jsonSchemaDefinitionName(path)
	if baseName == "" {
		return context.schemaType(resolved, nil)
	}

	name, ok := context.definitionNames[path]
	if !ok {
		name = context.uniqueDeclarationName(baseName)
		context.definitionNames[path] = name
	}

	context.definitionTypes[path] = inferredType{kind: inferredTypeNamed, name: name, namedCategory: "schema"}
	declarationType, declarationSource, err := context.declarationForSchema(name, resolved, []string{name})
	if err != nil {
		delete(context.definitionTypes, path)
		return inferredType{}, err
	}
	context.addDeclaration(name, declarationSource)
	context.definitionTypes[path] = declarationType
	return declarationType, nil
}

func (context *jsonSchemaContext) enumType(values []any, path []string) (inferredType, error) {
	name := context.uniqueDeclarationName(jsonSchemaPathName(path))
	if cached, ok := context.inlineEnumTypes[name]; ok {
		return cached, nil
	}

	declarationSource, declarationType, err := jsonSchemaEnumDeclaration(name, values)
	if err != nil {
		return inferredType{}, err
	}
	context.addDeclaration(name, declarationSource)
	context.inlineEnumTypes[name] = declarationType
	return declarationType, nil
}

func (context *jsonSchemaContext) declarationForSchema(name string, record map[string]any, path []string) (inferredType, string, error) {
	if enumValues, ok := record["enum"].([]any); ok && len(enumValues) > 0 {
		declarationSource, declarationType, err := jsonSchemaEnumDeclaration(name, enumValues)
		return declarationType, declarationSource, err
	}

	valueType, _, err := context.propertyType(record, path)
	if err != nil {
		return inferredType{}, "", err
	}

	switch valueType.kind {
	case inferredTypeRecord:
		return inferredType{kind: inferredTypeNamed, name: name, namedCategory: "schema"}, fmt.Sprintf("schema %s: %s;", name, formatSchemaRecord(valueType.record.fields, 0)), nil
	default:
		return inferredType{kind: inferredTypeNamed, name: name, namedCategory: "alias"}, fmt.Sprintf("type %s: %s;", name, formatSchemaType(valueType, 0)), nil
	}
}

func (context *jsonSchemaContext) addDeclaration(name string, source string) {
	if index, ok := context.declarationIndex[name]; ok {
		context.declarations[index] = source
		return
	}

	context.declarationIndex[name] = len(context.declarations)
	context.declarations = append(context.declarations, source)
}

func (context *jsonSchemaContext) uniqueDeclarationName(base string) string {
	if base == "" {
		base = "Generated"
	}

	candidate := base
	index := 2
	for {
		if _, exists := context.usedDeclarationName[candidate]; !exists {
			context.usedDeclarationName[candidate] = struct{}{}
			return candidate
		}
		candidate = fmt.Sprintf("%s%d", base, index)
		index++
	}
}

func jsonSchemaDefinitionName(path string) string {
	segments := strings.Split(strings.TrimPrefix(path, "#/"), "/")
	if len(segments) != 2 || segments[0] != "$defs" {
		return ""
	}

	return jsonSchemaIdentifier(segments[1])
}

func jsonSchemaPathName(path []string) string {
	parts := make([]string, 0, len(path))
	for _, segment := range path {
		parts = append(parts, jsonSchemaIdentifier(segment))
	}
	return strings.Join(parts, "")
}

func jsonSchemaIdentifier(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
	if len(parts) == 0 {
		return "Generated"
	}

	builder := strings.Builder{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			builder.WriteString(part[1:])
		}
	}

	result := builder.String()
	if result == "" {
		return "Generated"
	}
	if result[0] >= '0' && result[0] <= '9' {
		return "Value" + result
	}
	return result
}

func jsonSchemaEnumDeclaration(name string, values []any) (string, inferredType, error) {
	backingType, members, err := jsonSchemaEnumMembers(values)
	if err != nil {
		return "", inferredType{}, err
	}

	lines := []string{fmt.Sprintf("enum %s: %s {", name, backingType)}
	for _, member := range members {
		lines = append(lines, fmt.Sprintf("  %s = %s,", member.name, member.value))
	}
	lines = append(lines, "};")

	return strings.Join(lines, "\n"), inferredType{kind: inferredTypeNamed, name: name, namedCategory: "enum", backingType: backingType}, nil
}

type jsonSchemaEnumMember struct {
	name  string
	value string
}

func jsonSchemaEnumMembers(values []any) (string, []jsonSchemaEnumMember, error) {
	members := make([]jsonSchemaEnumMember, 0, len(values))
	usedNames := map[string]struct{}{}
	backingType := ""

	for index, value := range values {
		member, memberType, err := jsonSchemaEnumMemberValue(value, index)
		if err != nil {
			return "", nil, err
		}
		if backingType == "" {
			backingType = memberType
		} else if backingType != memberType {
			return "", nil, fmt.Errorf("mixed enum value types are not supported")
		}

		name := member.name
		suffix := 2
		for {
			if _, exists := usedNames[name]; !exists {
				usedNames[name] = struct{}{}
				member.name = name
				break
			}
			name = fmt.Sprintf("%s%d", member.name, suffix)
			suffix++
		}
		members = append(members, member)
	}

	if backingType != "string" && backingType != "int" {
		return "", nil, fmt.Errorf("unsupported enum backing type %q", backingType)
	}

	return backingType, members, nil
}

func jsonSchemaEnumMemberValue(value any, index int) (jsonSchemaEnumMember, string, error) {
	switch typed := value.(type) {
	case string:
		return jsonSchemaEnumMember{name: jsonSchemaIdentifier(typed), value: strconv.Quote(typed)}, "string", nil
	case int64:
		return jsonSchemaEnumMember{name: fmt.Sprintf("Value%d", typed), value: strconv.FormatInt(typed, 10)}, "int", nil
	case int:
		return jsonSchemaEnumMember{name: fmt.Sprintf("Value%d", typed), value: strconv.Itoa(typed)}, "int", nil
	default:
		return jsonSchemaEnumMember{}, "", fmt.Errorf("unsupported enum value %v at index %d", value, index)
	}
}

func validateUnionMembers(members []inferredType) (inferredType, error) {
	hasSchema := false

	for _, member := range members {
		switch member.kind {
		case inferredTypeRecord:
			hasSchema = true
		case inferredTypeNamed:
			if member.namedCategory != "schema" {
				return inferredType{}, fmt.Errorf("union members must be schemas")
			}
			hasSchema = true
		default:
			return inferredType{}, fmt.Errorf("union members must be schemas")
		}
	}

	if !hasSchema {
		return inferredType{}, fmt.Errorf("union members must be schemas")
	}

	return inferredType{kind: inferredTypeUnion, members: members}, nil
}

func validateVariantMembers(members []inferredType) (inferredType, error) {
	hasEnum := false
	hasSchema := false
	hasPrimitive := false
	enumBacking := ""

	for _, member := range members {
		switch member.kind {
		case inferredTypePrimitive:
			hasPrimitive = true
		case inferredTypeNamed:
			switch member.namedCategory {
			case "enum":
				hasEnum = true
				if enumBacking == "" {
					enumBacking = member.backingType
				} else if enumBacking != member.backingType {
					return inferredType{}, fmt.Errorf("enum variants require the same backing type")
				}
			case "schema":
				hasSchema = true
			case "alias":
				return inferredType{}, fmt.Errorf("variant members cannot include type aliases")
			default:
				return inferredType{}, fmt.Errorf("unsupported named variant member")
			}
		default:
			return inferredType{}, fmt.Errorf("variant members must be primitives, schemas, or enums")
		}
	}

	if hasEnum && (hasPrimitive || hasSchema) {
		return inferredType{}, fmt.Errorf("enum variants may only combine enums with the same backing type")
	}

	return inferredType{kind: inferredTypeVariant, members: members}, nil
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
