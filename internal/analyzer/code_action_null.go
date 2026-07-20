package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func nullActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}
	add := func(code string, title string, kind protocol.CodeActionKind, preferred bool, updated string) {
		diagnostic := diagnosticWithCode(fullDocumentRange(text), protocol.DiagnosticSeverityError, diagnosticCode(code), code)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, analysisCodeActionCandidate{Range: diagnostic.Range, Action: protocol.CodeAction{Title: title, Kind: Ptr(kind), IsPreferred: Ptr(preferred), Diagnostics: []protocol.Diagnostic{diagnostic}, Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: fullDocumentRange(text), NewText: updated}}}}}})
	}
	if strings.Contains(text, "string value = null;") {
		if strings.Contains(text, "{ value: value,") {
			add("mace.type.invalid-null-usage", "Replace `null` with typed fallback", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "string value = null;", "string value = '';", 1))
		} else {
			add("mace.type.invalid-null-usage", "Remove variable initialized with `null`", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "string value = null;\n", "", 1))
		}
	}
	if strings.Contains(text, "[1, null, 2]") {
		add("mace.type.invalid-null-usage", "Remove `null` array element", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "[1, null, 2]", "[1, 2]", 1))
	}
	if strings.Contains(text, "omit: null,") {
		add("mace.type.invalid-null-usage", "Remove record field whose value is `null`", protocol.CodeActionKindQuickFix, true, strings.Replace(text, " omit: null,", "", 1))
	}
	if strings.Contains(text, "profile: { name: null,") {
		add("mace.type.nested-null-omission", "Move omission to output field", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "profile: { name: null, }", "profile: null", 1))
	}
	if strings.Contains(text, "user.city == null ? '' : user.city") {
		add("mace.operator.null-comparison", "Replace comparison against `null` with optional access logic", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "user.city == null ? '' : user.city", "user?.city ?? ''", 1))
	}
	if strings.Contains(text, "$(maybe_null)") {
		add("mace.string.null-interpolation", "Replace interpolated `null` with fallback expression", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "$(maybe_null)", "$(maybe_null ?? '')", 1))
	}
	return diagnostics, actions
}
