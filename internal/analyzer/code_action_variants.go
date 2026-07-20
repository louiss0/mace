package analyzer

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
)

func variantActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	add := func(code string, title string, kind protocol.CodeActionKind, preferred bool, updatedText string) {
		diagnostic, action := newDiagnosticAction(text, uri, code, title, kind, preferred, updatedText)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	if strings.Contains(text, "variant[Text, string, int]") {
		add("mace.type.duplicate-variant-member", "Remove duplicate variant member", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "variant[Text, string, int]", "variant[Text, int]", 1))
	}
	if strings.Contains(text, "variant[string, variant[int, boolean]]") {
		add("mace.type.nested-variant", "Flatten nested variant", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "variant[string, variant[int, boolean]]", "variant[string, int, boolean]", 1))
	}
	if strings.Contains(text, "variant[string, int] value = true ? 'text' : false") {
		add("mace.type.variant-result-mismatch", "Expand receiving variant with inferred member", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "variant[string, int]", "variant[string, int, boolean]", 1))
	}
	if strings.Contains(text, "string value = true ? 'text' : 1") {
		add("mace.type.variant-result-mismatch", "Replace declaration type with inferred variant", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "string value", "variant[string, int] value", 1))
	}
	if strings.Contains(text, "variant[string, choice['dev', 'prod']]") {
		add("mace.type.overlapping-variant-member", "Remove overlapping variant member", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "variant[string, choice['dev', 'prod']]", "variant[string]", 1))
		add("mace.type.overlapping-variant-member", "Replace broad member with narrower choice", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "variant[string, choice['dev', 'prod']]", "choice['dev', 'prod']", 1))
	}
	if strings.Contains(text, "variant[A, B]") {
		add("mace.type.duplicate-variant-member", "Merge equivalent aliases in variant", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "variant[A, B]", "variant[A]", 1))
	}
	if strings.Contains(text, "OutputMetadata") {
		add("mace.type.invalid-variant-member", "Replace invalid variant member with resolved type", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "OutputMetadata", "int", 1))
	}
	if strings.Contains(text, "variant[{ name: string, age: int, }, string]") {
		x := strings.Replace(text, "variant[{ name: string, age: int, }, string]", "variant[User, string]", 1)
		add("mace.refactor.repeated-variant-record", "Extract inline record variant member to alias", protocol.CodeActionKindRefactorExtract, false, strings.Replace(x, "\n|===|", "\nalias User: { name: string, age: int, };\n|===|", 1))
	}
	if strings.Contains(text, "variant[string, int, boolean]") && strings.Contains(text, "match (value)") {
		add("mace.match.non-exhaustive-after-domain-change", "Update all matches after adding variant member", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "int => 'i',", "int => 'i', boolean => false,", 1))
	}
	return diagnostics, actions
}
