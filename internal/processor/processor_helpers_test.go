package processor

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func wrapScriptWithOutput(script string) string {
	return script + "\n[output = data] {}"
}

var bareOutputExpressionPattern = regexp.MustCompile(`(?m)^(\s*[A-Za-z_][A-Za-z0-9_]*\??:\s*)(\$self\.[A-Za-z_][A-Za-z0-9_\.\[\]]*|[A-Za-z_][A-Za-z0-9_\.\[\]]*)(\s*[,;])$`)

func wrapScriptWithOutputFields(script string, fields string) string {
	normalizedFields := strings.ReplaceAll(fields, ";", ",")
	normalizedFields = bareOutputExpressionPattern.ReplaceAllString(normalizedFields, `${1}(${2})${3}`)
	return script + "\n[output = data]\n{\n" + normalizedFields + "\n}"
}

type expectedValue struct {
	kind   ValueKind
	int64  int64
	float  float64
	bool   bool
	string string
	array  []expectedValue
	record map[string]expectedValue
}

type expectedSchemaField struct {
	name     string
	optional bool
}

func schemaPrimitive(name string) SchemaType {
	return SchemaType{Kind: SchemaTypePrimitive, Name: name}
}

func schemaNamed(name string) SchemaType {
	return SchemaType{Kind: SchemaTypeNamed, Name: name}
}

func schemaArray(element SchemaType) SchemaType {
	return SchemaType{Kind: SchemaTypeArray, Element: &element}
}

func schemaRecord(fields map[expectedSchemaField]SchemaType) SchemaType {
	recordFields := make(map[SchemaField]SchemaType, len(fields))
	for field, fieldType := range fields {
		recordFields[SchemaField{Name: field.name, Optional: field.optional}] = fieldType
	}

	return SchemaType{Kind: SchemaTypeRecord, Fields: recordFields}
}

func requireOutputValue(result Result, name string) Value {
	value, ok := result.Output[name]
	tAssert.True(ok)
	if !ok {
		return Value{}
	}
	return value
}

func assertExpectedValue(actual Value, expected expectedValue) {
	tAssert.Equal(expected.kind, actual.Kind)
	switch expected.kind {
	case ValueInt:
		tAssert.Equal(expected.int64, actual.Int)
	case ValueFloat:
		tAssert.InDelta(expected.float, actual.Float, 0.000001)
	case ValueHexInt, ValueHexFloat:
		formatted, err := FormatScalarValue(actual)
		tAssert.NoError(err)
		tAssert.Equal(expected.string, formatted)
	case ValueBoolean:
		tAssert.Equal(expected.bool, actual.Boolean)
	case ValueString:
		tAssert.Equal(expected.string, actual.String)
	case ValueArray:
		tAssert.Equal(len(expected.array), len(actual.Array))
		for index, value := range expected.array {
			if index >= len(actual.Array) {
				return
			}
			assertExpectedValue(actual.Array[index], value)
		}
	case ValueRecord:
		tAssert.Equal(len(expected.record), len(actual.Record))
		for name, value := range expected.record {
			actualValue, ok := actual.Record[name]
			tAssert.True(ok)
			if !ok {
				continue
			}
			assertExpectedValue(actualValue, value)
		}
	}
}

func assertExpectedOutput(result Result, expected map[string]expectedValue) {
	for name, value := range expected {
		actual := requireOutputValue(result, name)
		assertExpectedValue(actual, value)
	}
}

func assertExpectedSchema(result Result, expected map[expectedSchemaField]SchemaType) {
	tAssert.Len(result.Output, 0)
	tAssert.Len(result.Schema, len(expected))

	for field, expectedType := range expected {
		actualType, ok := result.Schema[SchemaField{Name: field.name, Optional: field.optional}]
		tAssert.True(ok)
		if !ok {
			continue
		}

		tAssert.Equal(expectedType, actualType)
	}
}

func assertProcessedResult(input string, expected expectedValue) {
	input = bareOutputExpressionPattern.ReplaceAllString(input, `${1}(${2})${3}`)

	processor := New()
	result, err := processor.ProcessInDir(input, "../..")
	tAssert.NoError(err)

	actual := requireOutputValue(result, "result")
	assertExpectedValue(actual, expected)
}

func requireScriptVariable(result ScriptResult, name string) Value {
	value, ok := result.Variables[name]
	tAssert.True(ok)
	if !ok {
		return Value{}
	}

	return value
}

func writeFixtureFile(root string, relativePath string, contents string) string {
	path := filepath.Join(root, relativePath)
	tAssert.NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	tAssert.NoError(os.WriteFile(path, []byte(contents), 0o644))
	return path
}

var _ = Describe("Helpers", func() {
	It("formats scalar helper values", func() {
		valueKey := scalarValueKey
		valueDisplay := scalarValueDisplay
		floatLiteral := decimalFloatLiteral

		key, ok := valueKey(Value{Kind: ValueFloat, Float: 1.5})
		tAssert.True(ok)
		tAssert.Contains(key, "float:")
		_, ok = valueKey(Value{Kind: ValueRecord})
		tAssert.False(ok)
		tAssert.Equal("null", valueDisplay(Value{Kind: ValueNull}))
		tAssert.Equal("unknown", valueDisplay(Value{Kind: ValueRecord}))
		tAssert.Equal("2.0", floatLiteral(2))
		tAssert.Equal("1.5", floatLiteral(1.5))
		key, ok = valueKey(Value{Kind: ValueNull})
		tAssert.True(ok)
		tAssert.Equal("null", key)
	})

	It("compares scalar values for equality", func() {
		equalValues := valuesEqual

		equal, err := equalValues(Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		tAssert.NoError(err)
		tAssert.True(equal)

		equal, err = equalValues(Value{Kind: ValueFloat, Float: 1.5}, Value{Kind: ValueFloat, Float: 2.5})
		tAssert.NoError(err)
		tAssert.False(equal)

		equal, err = equalValues(Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexFloat, Float: 2})
		tAssert.NoError(err)
		tAssert.True(equal)

		equal, err = equalValues(Value{Kind: ValueHexFloat, Float: 3}, Value{Kind: ValueHexInt, Int: 2})
		tAssert.NoError(err)
		tAssert.False(equal)

		equal, err = equalValues(Value{Kind: ValueBoolean, Boolean: true}, Value{Kind: ValueBoolean, Boolean: true})
		tAssert.NoError(err)
		tAssert.True(equal)

		equal, err = equalValues(Value{Kind: ValueString, String: "Ada"}, Value{Kind: ValueString, String: "Bob"})
		tAssert.NoError(err)
		tAssert.False(equal)

		_, err = equalValues(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		tAssert.ErrorContains(err, "unsupported equality")
	})
})

