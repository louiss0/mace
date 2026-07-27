package analyzer

import "testing"

func TestSyntaxStructureAnalysisClassifiesUnspacedSubtraction(t *testing.T) {
	testCases := []struct {
		name              string
		text              string
		code              diagnosticCode
		expectsDiagnostic bool
	}{
		{
			name: "bind is not subtraction",
			text: "|==========|\nfrom './shared.mace' bind Shared;\n|==========|\n[output = 'data'] { value: Shared, }",
			code: diagnosticSyntaxKebabIdentifierUsedAsSubtraction,
		},
		{
			name: "kebab field is not subtraction",
			text: "[output = 'data'] { display-name: 'Mace', }",
			code: diagnosticSyntaxKebabIdentifierUsedAsSubtraction,
		},
		{
			name: "hyphenated schema file path is not subtraction",
			text: "[output = 'data', schema_file = './working-schema.mace'] { result: { name: 'Ada', }, }",
			code: diagnosticSyntaxKebabIdentifierUsedAsSubtraction,
		},
		{
			name:              "adjacent identifier operands are subtraction",
			text:              "|===|\nint first = 3; int second = 1; int result = first-second;\n|===|\n[output = 'data'] { result: result, }",
			code:              diagnosticSyntaxKebabIdentifierUsedAsSubtraction,
			expectsDiagnostic: true,
		},
		{
			name: "long script delimiter scopes declarations",
			text: "|==========|\nstring name = 'Mace';\n|==========|\n[output = 'data'] { name: name, }",
			code: diagnosticFileDeclarationOutsideScript,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			diagnostics, _ := syntaxStructureAnalysis(testCase.text, "")
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code != nil && diagnostic.Code.Value == string(testCase.code) {
					found = true
				}
			}
			if found != testCase.expectsDiagnostic {
				t.Fatalf("diagnostic %q found=%t, want %t", testCase.code, found, testCase.expectsDiagnostic)
			}
		})
	}
}
