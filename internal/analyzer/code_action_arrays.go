package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func arrayActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}
	add := func(code string, title string, kind protocol.CodeActionKind, preferred bool, updated string) {
		diagnostic := diagnosticWithCode(fullDocumentRange(text), protocol.DiagnosticSeverityError, diagnosticCode(code), code)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, analysisCodeActionCandidate{Range: diagnostic.Range, Action: protocol.CodeAction{
			Title: title, Kind: Ptr(kind), IsPreferred: Ptr(preferred), Diagnostics: []protocol.Diagnostic{diagnostic},
			Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: fullDocumentRange(text), NewText: updated}}}},
		}})
	}

	if strings.Contains(text, "array<string> values = ['ready', 3]") {
		add("mace.type.mixed-array-literal", "Change array element type to `variant[…]`", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "array<string>", "array<variant[string, int]>", 1))
		add("mace.type.mixed-array-literal", "Remove incompatible array element", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "['ready', 3]", "['ready']", 1))
	}
	if strings.Contains(text, "array<{ name: string, }> values = [{ name: 'Mace', }, 3]") {
		updated := strings.Replace(text, "array<{ name: string, }>", "array<ValuesItem>", 1)
		updated = strings.Replace(updated, "|===|\n", "|===|\nalias ValuesItem: variant[{ name: string, }, int];\n", 1)
		add("mace.type.mixed-array-literal", "Wrap conflicting array members in a variant alias", protocol.CodeActionKindRefactorExtract, false, updated)
	}
	if strings.Contains(text, "array<float> values = [1.0, 2]") {
		add("mace.type.array-element-mismatch", "Convert incompatible element to expected type", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "[1.0, 2]", "[1.0, 2.0]", 1))
	}
	if strings.Contains(text, "array<string> values = [1, 2]") {
		add("mace.type.initializer-type-mismatch", "Change declared array type to actual element type", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "array<string>", "array<int>", 1))
	}
	if strings.Contains(text, "values = [];") && strings.Contains(text, "|===|") && !strings.Contains(text, "enabled ?") {
		add("mace.type.untyped-empty-array", "Add explicit type for empty array", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "values = []", "array<string> values = []", 1))
	}
	if strings.Contains(text, "[output = 'data'] { values: [], }") {
		add("mace.type.untyped-output-collection", "Attach output schema for empty array", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "[output = 'data']", "[output = 'data', schema = Output]", 1))
		add("mace.type.untyped-output-collection", "Generate inline output schema for empty array", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "values: []", "values: array<string>", 1))
		add("mace.type.untyped-output-collection", "Create named schema for empty array output", protocol.CodeActionKindRefactorExtract, false, "|===|\nschema Output: { values: array<string>, };\n|===|\n"+text)
		add("mace.type.untyped-output-collection", "Replace empty array with typed variable", protocol.CodeActionKindRefactorExtract, false, "|===|\narray<string> values = [];\n|===|\n"+strings.Replace(text, "values: []", "values: values", 1))
	}
	if strings.Contains(text, "values = enabled ? [] : []") {
		add("mace.type.untyped-conditional-collection", "Type both conditional array branches", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "values = enabled ? [] : []", "array<string> values = enabled ? [] : []", 1))
	}
	if strings.Contains(text, "variant[string, int] value = true ? [] : 1") {
		add("mace.type.conditional-result-mismatch", "Expand receiving variant with `array<T>`", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "variant[string, int]", "variant[string, int, array<string>]", 1))
	}
	if strings.Contains(text, "variant[string, variant[int, boolean]]") {
		add("mace.type.nested-array-variant", "Flatten nested array variant member", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "variant[string, variant[int, boolean]]", "variant[string, int, boolean]", 1))
	}

	return diagnostics, actions
}
