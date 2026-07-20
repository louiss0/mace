package analyzer

import (
	"strconv"
	"strings"
	"testing"
)

type outputExtractionTestCase struct {
	name     string
	text     string
	title    string
	expected []string
}

func TestOutputValueExtractionCodeActionsInferPrimitiveAndCollectionTypes(t *testing.T) {
	primitiveValues := []struct {
		name     string
		typeName string
		literal  string
	}{
		{name: "string", typeName: "string", literal: "'Ada'"},
		{name: "int", typeName: "int", literal: "42"},
		{name: "float", typeName: "float", literal: "3.14"},
		{name: "hex int", typeName: "hex_int", literal: "0x2a"},
		{name: "hex float", typeName: "hex_float", literal: "0x2.a"},
		{name: "boolean", typeName: "boolean", literal: "true"},
	}
	testCases := []outputExtractionTestCase{}
	for _, primitive := range primitiveValues {
		testCases = append(testCases, outputExtractionTestCase{
			name:     "primitive " + primitive.name,
			text:     "[output = 'data'] { value: " + primitive.literal + ", }",
			title:    "Extract output value into script variable",
			expected: []string{primitive.typeName + " value = " + primitive.literal + ";", "value: value"},
		})
		for depth := 2; depth <= 5; depth++ {
			arrayValue := nestedArrayLiteral(primitive.literal, depth)
			testCases = append(testCases, outputExtractionTestCase{
				name:     primitive.name + " array at depth " + strconv.Itoa(depth),
				text:     "[output = 'data'] { value: " + arrayValue + ", }",
				title:    "Extract output value into script variable",
				expected: []string{nestedType("array", primitive.typeName, depth) + " value = " + arrayValue + ";", "value: value"},
			})

			recordValue := nestedUniformRecordLiteral(primitive.literal, depth)
			testCases = append(testCases, outputExtractionTestCase{
				name:     primitive.name + " record at depth " + strconv.Itoa(depth),
				text:     "[output = 'data'] { payload: " + recordValue + ", }",
				title:    "Extract output record into script variable",
				expected: []string{nestedType("record", primitive.typeName, depth) + " payload = " + recordValue + ";", "payload: payload"},
			})
		}
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertOutputExtractionAction(t, testCase)
		})
	}
}

func TestOutputValueExtractionCodeActionsCreateSchemasForMixedRecordTypes(t *testing.T) {
	fieldValues := []struct {
		name     string
		typeName string
		literal  string
	}{
		{name: "text", typeName: "string", literal: "'Ada'"},
		{name: "count", typeName: "int", literal: "42"},
		{name: "ratio", typeName: "float", literal: "3.14"},
		{name: "hex_count", typeName: "hex_int", literal: "0x2a"},
		{name: "hex_ratio", typeName: "hex_float", literal: "0x2.a"},
		{name: "enabled", typeName: "boolean", literal: "true"},
		{name: "tags", typeName: "array<string>", literal: "['mace']"},
	}

	for count := 2; count <= 7; count++ {
		count := count
		t.Run(strconv.Itoa(count)+" distinct field types", func(t *testing.T) {
			fields := fieldValues[:count]
			fieldText := strings.Builder{}
			expectedSchemaFields := []string{}
			for _, field := range fields {
				fieldText.WriteString(field.name + ": " + field.literal + ", ")
				expectedSchemaFields = append(expectedSchemaFields, field.name+": "+field.typeName)
			}
			assertOutputExtractionAction(t, outputExtractionTestCase{
				name:     strconv.Itoa(count) + " distinct field types",
				text:     "[output = 'data'] { config: { " + fieldText.String() + "}, }",
				title:    "Extract output record into schema",
				expected: []string{"schema Config: { " + strings.Join(expectedSchemaFields, ", ") + ", };", "Config config =", "config: config"},
			})
		})
	}
}

func assertOutputExtractionAction(t *testing.T, testCase outputExtractionTestCase) {
	t.Helper()
	file, err := parseFile(testCase.text)
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := lex(testCase.text)
	if err != nil {
		t.Fatal(err)
	}
	actions := outputValueExtractionCodeActions(testCase.text, file, tokens, "document.mace")
	if !containsOutputExtractionAction(actions, testCase.title, testCase.expected) {
		t.Fatalf("missing %q action", testCase.title)
	}
}

func nestedArrayLiteral(value string, depth int) string {
	for level := 0; level < depth; level++ {
		value = "[" + value + "]"
	}
	return value
}

func nestedUniformRecordLiteral(value string, depth int) string {
	for level := 0; level < depth; level++ {
		value = "{ first: " + value + ", second: " + value + ", }"
	}
	return value
}

func nestedType(container string, typeName string, depth int) string {
	for level := 0; level < depth; level++ {
		typeName = container + "<" + typeName + ">"
	}
	return typeName
}

func containsOutputExtractionAction(actions []analysisCodeActionCandidate, title string, expected []string) bool {
	for _, action := range actions {
		if action.Action.Title != title {
			continue
		}
		text := action.Action.Edit.Changes[pathURI("document.mace")][0].NewText
		if allStringsPresent(text, expected) {
			return true
		}
	}
	return false
}

func allStringsPresent(text string, expected []string) bool {
	for _, value := range expected {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}
