package analyzer

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
)

func interpolationActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}
	add := func(code, title string, kind protocol.CodeActionKind, preferred bool, updated string) {
		d := diagnosticWithCode(fullDocumentRange(text), protocol.DiagnosticSeverityError, diagnosticCode(code), code)
		diagnostics = append(diagnostics, d)
		actions = append(actions, analysisCodeActionCandidate{Range: d.Range, Action: protocol.CodeAction{Title: title, Kind: Ptr(kind), IsPreferred: Ptr(preferred), Diagnostics: []protocol.Diagnostic{d}, Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: fullDocumentRange(text), NewText: updated}}}}}})
	}
	if strings.Contains(text, `"Hello $name"`) {
		add("mace.string.unsupported-interpolation", "Replace shorthand interpolation with `$(…)`", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "$name", "$(name)", 1))
	}
	if strings.Contains(text, `"$(user)"`) {
		add("mace.string.nonscalar-interpolation", "Use scalar field from record", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "$(user)", "$(user.name)", 1))
	}
	if strings.Contains(text, `"$(value)"`) {
		add("mace.string.nonscalar-interpolation", "Match variant before interpolation", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "$(value)", "$(match (value) { string => value, int => value, })", 1))
	}
	if strings.Contains(text, "$(user?.name)") {
		add("mace.string.absent-interpolation", "Coalesce optional value before interpolation", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "$(user?.name)", "$(user?.name ?? '')", 1))
	}
	if strings.Contains(text, "$(maybe_null)") {
		add("mace.string.null-interpolation", "Replace null-producing expression with fallback", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "$(maybe_null)", "$(maybe_null ?? '')", 1))
	}
	if strings.Contains(text, `text: "$HOME"`) {
		add("mace.string.unintended-interpolation", "Convert interpolating string to literal string", protocol.CodeActionKindQuickFix, true, strings.Replace(text, `"$HOME"`, `'$HOME'`, 1))
	}
	if strings.Contains(text, "$(match (value)") {
		add("mace.string.complex-interpolation", "Extract scalar expression before interpolation", protocol.CodeActionKindRefactorExtract, false, "|===|\nstring interpolated_value = '';\n|===|\n"+text)
	}
	if strings.Contains(text, "prefix $(values) suffix") {
		add("mace.string.nonscalar-interpolation", "Remove nonscalar interpolation", protocol.CodeActionKindQuickFix, false, strings.Replace(text, `"prefix $(values) suffix"`, `'prefix  suffix'`, 1))
	}
	return diagnostics, actions
}
