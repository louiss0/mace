package analyzer

import "testing"

func TestLiteralActionAnalysisOnlyReportsIncompleteHexFloats(t *testing.T) {
	testCases := []struct {
		name              string
		text              string
		expectsDiagnostic bool
	}{
		{name: "complete hexadecimal fraction", text: "hex_float fraction = 0x2.8;"},
		{name: "hexadecimal fraction in array", text: "array<hex_float> values = [0x1.0, 0x2.8];"},
		{name: "incomplete hexadecimal fraction", text: "[output = 'data'] { value: 0x2., }", expectsDiagnostic: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			diagnostics, _ := literalActionAnalysis(testCase.text, "")
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code != nil && diagnostic.Code.Value == "mace.number.incomplete-hex-float" {
					found = true
				}
			}
			if found != testCase.expectsDiagnostic {
				t.Fatalf("incomplete hex diagnostic found=%t, want %t", found, testCase.expectsDiagnostic)
			}
		})
	}
}
