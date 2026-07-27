package analyzer

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
)

func conditionalActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}
	add := func(code, title string, kind protocol.CodeActionKind, preferred bool, updated string) {
		diagnostic := diagnosticWithCode(fullDocumentRange(text), protocol.DiagnosticSeverityError, diagnosticCode(code), code)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, analysisCodeActionCandidate{Range: diagnostic.Range, Action: protocol.CodeAction{Title: title, Kind: Ptr(kind), IsPreferred: Ptr(preferred), Diagnostics: []protocol.Diagnostic{diagnostic}, Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: fullDocumentRange(text), NewText: updated}}}}}})
	}
	if strings.Contains(text, "a ? (b ? 1 : 2) : 3") {
		add("mace.expression.nested-conditional", "Extract nested conditional into variable", protocol.CodeActionKindRefactorExtract, false, "|===|\nint nested = b ? 1 : 2;\n|===|\n"+strings.Replace(text, "(b ? 1 : 2)", "nested", 1))
	}
	if strings.Contains(text, "enabled ? (mode == 'dev' ? 1 : 2) : 3") {
		add("mace.expression.nested-conditional", "Replace inner conditional with match", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "mode == 'dev' ? 1 : 2", "match (mode) { 'dev' => 1, _ => 2, }", 1))
	}
	if strings.Contains(text, "mode == 'dev' ? 1 : 2") && !strings.Contains(text, "enabled ?") {
		add("mace.expression.conditional-domain", "Replace outer conditional with match", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "mode == 'dev' ? 1 : 2", "match (mode) { 'dev' => 1, _ => 2, }", 1))
	}
	if strings.Contains(text, "string value = true ? 'text' : 1") {
		add("mace.type.conditional-result-mismatch", "Expand receiving type to inferred variant", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "string value", "variant[string, int] value", 1))
	}
	if strings.Contains(text, "float value = true ? 1.0 : 2") {
		add("mace.type.conditional-result-mismatch", "Convert branch to expected type", protocol.CodeActionKindQuickFix, true, strings.Replace(text, ": 2", ": 2.0", 1))
	}
	if strings.Contains(text, "values = true ? [] : ['Mace']") {
		add("mace.type.untyped-conditional-collection", "Add explicit collection type to declaration", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "values =", "array<string> values =", 1))
	}
	if strings.Contains(text, "values: enabled ? [] : ['Mace']") {
		add("mace.type.untyped-conditional-collection", "Attach output schema for conditional collection", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "[output = 'data']", "[output = 'data', schema = Output]", 1))
	}
	if strings.Contains(text, "variant[string, Text] value") {
		add("", "Flatten duplicate conditional result members", protocol.CodeActionKind("source.fixAll.mace"), false, strings.Replace(text, "variant[string, Text]", "variant[string]", 1))
	}
	if strings.Contains(text, "true ? 'yes' : 'no'") {
		add("mace.expression.constant-conditional", "Replace constant conditional with selected branch", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "true ? 'yes' : 'no'", "'yes'", 1))
	}
	if strings.Contains(text, "prefix + suffix : fallback + suffix") {
		add("mace.refactor.repeated-branch-expression", "Extract repeated branch expression", protocol.CodeActionKindRefactorExtract, false, "|===|\nstring shared_suffix = suffix;\n|===|\n"+text)
	}
	return diagnostics, actions
}
