package analyzer

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type outputExtractionTestCase struct {
	name     string
	text     string
	title    string
	expected []string
}

type primitiveOutputValue struct {
	name     string
	typeName string
	literal  string
}

func TestOutputValueExtractionCodeActionsInferPrimitiveAndCollectionTypes(t *testing.T) {
	for _, testCase := range outputPrimitiveExtractionTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			assertOutputExtractionAction(t, testCase)
		})
	}
}

func TestOutputValueExtractionCodeActionsCreateSchemasForMixedRecordTypes(t *testing.T) {
	for _, testCase := range outputSchemaExtractionTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			assertOutputExtractionAction(t, testCase)
		})
	}
}

func TestOutputBlockValueExtractionCodeActionsInferPrimitiveAndCollectionTypes(t *testing.T) {
	for _, testCase := range outputPrimitiveExtractionTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			assertOutputBlockExtractionAction(t, testCase)
		})
	}
}

func TestOutputBlockValueExtractionCodeActionsCreateSchemasForMixedRecordTypes(t *testing.T) {
	for _, testCase := range outputSchemaExtractionTestCases() {
		t.Run(testCase.name, func(t *testing.T) {
			assertOutputBlockExtractionAction(t, testCase)
		})
	}
}

func outputPrimitiveExtractionTestCases() []outputExtractionTestCase {
	primitiveValues := []primitiveOutputValue{
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
	return testCases
}

func outputSchemaExtractionTestCases() []outputExtractionTestCase {
	fieldValues := []primitiveOutputValue{
		{name: "text", typeName: "string", literal: "'Ada'"},
		{name: "count", typeName: "int", literal: "42"},
		{name: "ratio", typeName: "float", literal: "3.14"},
		{name: "hex_count", typeName: "hex_int", literal: "0x2a"},
		{name: "hex_ratio", typeName: "hex_float", literal: "0x2.a"},
		{name: "enabled", typeName: "boolean", literal: "true"},
		{name: "tags", typeName: "array<string>", literal: "['mace']"},
	}
	testCases := []outputExtractionTestCase{}
	for count := 2; count <= 7; count++ {
		fields := fieldValues[:count]
		fieldText := strings.Builder{}
		expectedSchemaFields := []string{}
		for _, field := range fields {
			fieldText.WriteString(field.name + ": " + field.literal + ", ")
			expectedSchemaFields = append(expectedSchemaFields, field.name+": "+field.typeName)
		}
		testCases = append(testCases, outputExtractionTestCase{
			name:     strconv.Itoa(count) + " distinct field types",
			text:     "[output = 'data'] { config: { " + fieldText.String() + "}, }",
			title:    "Extract output record into schema",
			expected: []string{"schema Config: { " + strings.Join(expectedSchemaFields, ", ") + ", };", "Config config =", "config: config"},
		})
	}
	return testCases
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
	if !containsOutputExtractionCandidate(actions, testCase.title, testCase.expected) {
		t.Fatalf("missing %q action", testCase.title)
	}
}

func assertOutputBlockExtractionAction(t *testing.T, testCase outputExtractionTestCase) {
	t.Helper()
	documentPath := filepath.Join(t.TempDir(), "document.mace")
	snapshot := AnalyzeDocumentAt(testCase.text, documentPath)
	if snapshot.file == nil || len(snapshot.file.Output.DataFields) == 0 {
		t.Fatal("expected a parsed data output block")
	}

	fieldRange := tokenProtocolRange(snapshot.file.Output.DataFields[0].NameToken)
	actions := snapshot.codeActions(pathURI(documentPath), fieldRange)
	if !containsOutputExtractionAction(actions, testCase.title, testCase.expected) {
		t.Fatalf("missing output-block %q action", testCase.title)
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

func containsOutputExtractionCandidate(actions []analysisCodeActionCandidate, title string, expected []string) bool {
	for _, action := range actions {
		if action.Action.Title == title && allStringsPresent(outputExtractionText(action.Action.Edit), expected) {
			return true
		}
	}
	return false
}

func containsOutputExtractionAction(actions []protocol.CodeAction, title string, expected []string) bool {
	for _, action := range actions {
		if action.Title == title && allStringsPresent(outputExtractionText(action.Edit), expected) {
			return true
		}
	}
	return false
}

func outputExtractionText(edit *protocol.WorkspaceEdit) string {
	for _, edits := range edit.Changes {
		if len(edits) == 1 {
			return edits[0].NewText
		}
	}
	return ""
}

func allStringsPresent(text string, expected []string) bool {
	for _, value := range expected {
		if !strings.Contains(text, value) {
			return false
		}
	}
	return true
}
