package analyzer

import (
	"strings"
	"testing"
)

func TestOutputValueExtractionCodeActionsExtractValuesAndRecords(t *testing.T) {
	testCases := []struct {
		name     string
		text     string
		title    string
		expected []string
	}{
		{
			name:     "literal value",
			text:     "[output = 'data'] { value: 'Ada', }",
			title:    "Extract output value into script variable",
			expected: []string{"string value = 'Ada';", "value: value"},
		},
		{
			name:     "uniform record",
			text:     "[output = 'data'] { labels: { first: 'A', second: 'B', }, }",
			title:    "Extract output record into script variable",
			expected: []string{"record<string> labels = { first: 'A', second: 'B', };", "labels: labels"},
		},
		{
			name:     "mixed record",
			text:     "[output = 'data'] { user: { name: 'Ada', age: 1, }, }",
			title:    "Extract output record into schema",
			expected: []string{"schema User: { name: string, age: int, };", "User user = { name: 'Ada', age: 1, };", "user: user"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
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
		})
	}
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
