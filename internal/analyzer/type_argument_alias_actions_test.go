package analyzer

import (
	"strings"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestTypeArgumentAliasCodeActionsExtractFromEveryTypeConstructor(t *testing.T) {
	testCases := []struct {
		name        string
		text        string
		expected    string
		declaration string
	}{
		{
			name:        "variant member",
			text:        "|===|\nalias Value: variant[string, int];\n|===|\n[output = 'schema'] { Value: Value, }",
			expected:    "variant[ExtractedType, int]",
			declaration: "alias ExtractedType: string;",
		},
		{
			name:        "array element",
			text:        "|===|\nalias Values: array<variant[string, int]>;\n|===|\n[output = 'schema'] { Values: Values, }",
			expected:    "array<ExtractedType>",
			declaration: "alias ExtractedType: variant[string, int];",
		},
		{
			name:        "record value",
			text:        "|===|\nalias Labels: record<array<string>>;\n|===|\n[output = 'schema'] { Labels: Labels, }",
			expected:    "record<ExtractedType>",
			declaration: "alias ExtractedType: array<string>;",
		},
		{
			name:        "fusion member",
			text:        "|===|\nalias Combined: fusion[array<string>, record<int>];\n|===|\n[output = 'schema'] { Combined: Combined, }",
			expected:    "fusion[ExtractedType, record<int>]",
			declaration: "alias ExtractedType: array<string>;",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tokens, err := lex(testCase.text)
			if err != nil {
				t.Fatal(err)
			}
			actions := typeArgumentAliasCodeActions(testCase.text, tokens, "document.mace")
			if !containsTypeAliasAction(actions, testCase.expected, testCase.declaration) {
				t.Fatalf("missing action containing %q and %q", testCase.expected, testCase.declaration)
			}
		})
	}
}

func TestTypeArgumentAliasCodeActionsCreateScriptForOutputTypes(t *testing.T) {
	text := "[output = 'schema'] { Values: array<string>, }"
	tokens, err := lex(text)
	if err != nil {
		t.Fatal(err)
	}

	actions := typeArgumentAliasCodeActions(text, tokens, "document.mace")
	if !containsTypeAliasAction(actions, "array<ExtractedType>", "alias ExtractedType: string;") {
		t.Fatal("expected extraction action to create a script block")
	}
}

func containsTypeAliasAction(actions []analysisCodeActionCandidate, expected string, declaration string) bool {
	for _, action := range actions {
		if action.Action.Title != "Extract type argument into alias ‘ExtractedType’" {
			continue
		}
		edits := action.Action.Edit.Changes[protocol.DocumentUri(pathURI("document.mace"))]
		if len(edits) == 1 && strings.Contains(edits[0].NewText, expected) && strings.Contains(edits[0].NewText, declaration) {
			return true
		}
	}
	return false
}
