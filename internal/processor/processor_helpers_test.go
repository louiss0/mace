package processor

import (
	"os"
	"path/filepath"

	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func wrapScriptWithOutput(script string) string {
	return script + "\n[output = data] {}"
}

func wrapScriptWithOutputFields(script string, fields string) string {
	return script + "\n[output = data]\n{\n" + fields + "\n}"
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

