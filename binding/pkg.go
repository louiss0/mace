package binding

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/louiss0/mace/processor"
)

func OutputMap(result processor.Result) map[string]any {
	return valuesToMap(result.Output)
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

	return "[output = data]\n" + marshaled, nil
}

type marshaller struct{}

func newMarshaller() *marshaller {
	return &marshaller{}
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
