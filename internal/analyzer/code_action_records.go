package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type recordActionSpec struct {
	match     string
	code      string
	title     string
	kind      protocol.CodeActionKind
	preferred bool
	updated   string
}

func recordActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	if strings.Contains(text, "name: 'first',\n  name: 'second',") {
		return recordActions(text, documentPath, []recordActionSpec{{"name: 'second',", "mace.declaration.duplicate-output-field", "Remove duplicate record field", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "  name: 'second',\n", "", 1)}})
	}
	if strings.Contains(text, "name: string,\n  name: string,") {
		return recordActions(text, documentPath, []recordActionSpec{{"name: string,", "mace.declaration.duplicate-schema-field", "Remove duplicate schema field", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "  name: string,\n", "", 1)}})
	}
	if strings.Contains(text, "name?: 'Mace',") {
		return recordActions(text, documentPath, []recordActionSpec{{"name?: 'Mace'", "mace.type.data-field-optional-marker", "Remove optional marker from data field", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "name?:", "name:", 1)}})
	}

	return recordActions(text, documentPath, []recordActionSpec{
		{"User user = { name: 'Mace', };", "mace.type.record-does-not-match-schema", "Add missing required schema field", protocol.CodeActionKindQuickFix, false, "age: 0"},
		{"User user = {};", "mace.type.record-does-not-match-schema", "Add all missing required fields", protocol.CodeActionKindQuickFix, false, "name: ''\nage: 0\nactive: false"},
		{"extra: 1", "mace.type.record-does-not-match-schema", "Remove unknown record field", protocol.CodeActionKindQuickFix, true, "name: 'Mace'"},
		{"nmae: 'Mace'", "mace.type.record-does-not-match-schema", "Rename field to nearest schema field", protocol.CodeActionKindQuickFix, false, "name: 'Mace'"},
		{"name: 'Mace', age: 1", "mace.type.record-does-not-match-schema", "Add field to schema", protocol.CodeActionKindRefactorRewrite, false, "age: int"},
		{"age: '1'", "mace.type.record-field-mismatch", "Change field value to expected type", protocol.CodeActionKindQuickFix, false, "age: 1"},
		{"age: 1.5", "mace.type.record-field-mismatch", "Change schema field type to actual type", protocol.CodeActionKindRefactorRewrite, false, "age: float"},
		{"User second = { id: 'two'", "mace.type.record-field-mismatch", "Expand schema field to a variant", protocol.CodeActionKindRefactorRewrite, false, "id: variant[int, string]"},
		{"schema User: { name: string, name: string, }", "mace.declaration.duplicate-schema-field", "Rename duplicate schema field", protocol.CodeActionKindRefactorRewrite, false, "name_2"},
		{"nickname: string", "mace.type.repeated-missing-field", "Mark schema field optional", protocol.CodeActionKindRefactorRewrite, false, "nickname?: string"},
		{"schema User: { id?: int, }", "mace.type.fusion-optionality-conflict", "Make optional field required", protocol.CodeActionKindRefactorRewrite, false, "id: int"},
		{"user = {};", "mace.type.untyped-empty-record", "Add expected type for empty record", protocol.CodeActionKindQuickFix, true, "User user = {}"},
		{"[output = 'data'] { user: {}, }", "mace.type.untyped-output-collection", "Attach output schema for empty record", protocol.CodeActionKindQuickFix, true, "schema = Output"},
		{"record<string> user", "mace.type.record-map-too-broad", "Convert record map to closed inline record", protocol.CodeActionKindRefactorRewrite, false, "{ name: string, } user"},
		{"{ first: string, second: string, } values", "mace.refactor.uniform-inline-record", "Convert uniform inline record to `record<T>`", protocol.CodeActionKindRefactorRewrite, false, "record<string> values"},
	})
}

func recordActions(text string, documentPath string, specifications []recordActionSpec) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		updated := specification.updated
		if specification.title != "Remove duplicate record field" && specification.title != "Remove duplicate schema field" && specification.title != "Remove optional marker from data field" {
			updated = text + "\n" + updated
		}
		diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, specification.preferred, updated)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	return diagnostics, actions
}
