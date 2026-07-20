package analyzer

import "testing"

func TestCrossFileActionAnalysisDoesNotDiagnoseEmptyDataOutputs(t *testing.T) {
	diagnostics, _ := crossFileActionAnalysis("[output = 'data'] {}", "document.mace")
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != nil && diagnostic.Code.Value == "mace.workspace.stop-export" {
			t.Fatal("empty data output must not report a stop-export diagnostic")
		}
	}
}
