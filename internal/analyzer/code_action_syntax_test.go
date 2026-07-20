package analyzer

import "testing"

func TestSyntaxStructureAnalysisIgnoresImportAsAndKebabFieldNames(t *testing.T) {
	testCases := []struct {
		name string
		text string
		code diagnosticCode
	}{
		{
			name: "import-as is not subtraction",
			text: "|==========|\nfrom './shared.mace' import-as Shared;\n|==========|\n[output = 'data'] { value: Shared, }",
			code: diagnosticSyntaxKebabIdentifierUsedAsSubtraction,
		},
		{
			name: "kebab field is not subtraction",
			text: "[output = 'data'] { display-name: 'Mace', }",
			code: diagnosticSyntaxKebabIdentifierUsedAsSubtraction,
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
			for _, diagnostic := range diagnostics {
				if diagnostic.Code != nil && diagnostic.Code.Value == string(testCase.code) {
					t.Fatalf("unexpected diagnostic %q", testCase.code)
				}
			}
		})
	}
}
