package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type documentationActionSpec struct {
	match, code, title, updated string
	kind                        protocol.CodeActionKind
	preferred                   bool
}

func documentationActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []documentationActionSpec{
		{"gen_doc Name { summary", "mace.documentation.before-target", "Move documentation after target declaration", "alias Name: string;\ngen_doc Name", protocol.CodeActionKindQuickFix, true},
		{"schema User: { name: string, }; gen_doc User", "mace.documentation.target-mismatch", "Change `gen_doc` to `schema_doc`", "schema_doc User", protocol.CodeActionKindQuickFix, true},
		{"alias Name: string; schema_doc Name", "mace.documentation.target-mismatch", "Change `schema_doc` to `gen_doc`", "gen_doc Name", protocol.CodeActionKindQuickFix, true},
		{"summary: 'Name', summary: 'Other'", "mace.documentation.duplicate-key", "Remove duplicate documentation key", "summary: 'Name'", protocol.CodeActionKindQuickFix, true},
		{"summry: 'Name'", "mace.documentation.unknown-key", "Rename unknown documentation key", "summary: 'Name'", protocol.CodeActionKindQuickFix, false},
		{"gen_doc User { fields", "mace.documentation.fields-outside-schema-doc", "Move `fields` into `schema_doc`", "schema_doc User", protocol.CodeActionKindRefactorRewrite, false},
		{"alias Name: string; gen_doc Name { fields", "mace.documentation.invalid-fields", "Remove invalid `fields` entry", "gen_doc Name", protocol.CodeActionKindQuickFix, true},
		{"fields: { nmae: 'Name'", "mace.documentation.unknown-field", "Rename documented field to schema field", "name: 'Name'", protocol.CodeActionKindQuickFix, false},
		{"fields: { old: 'Old'", "mace.documentation.unknown-field", "Remove documentation for nonexistent field", "schema_doc User", protocol.CodeActionKindQuickFix, false},
		{"fields: { age: 'Age'", "mace.documentation.unknown-field", "Add documented field to schema", "age: int", protocol.CodeActionKindRefactorRewrite, false},
		{"name: string /# Inline", "mace.documentation.conflict", "Remove conflicting inline description", "name: string", protocol.CodeActionKindQuickFix, true},
		{"name: string /# Name", "mace.documentation.conflict", "Move inline description into structured documentation", "schema_doc User\nname: 'Name'", protocol.CodeActionKindRefactorRewrite, false},
		{"summary: 'A name'", "mace.documentation.can-inline", "Move structured summary into inline description", "/# A name", protocol.CodeActionKindRefactorRewrite, false},
		{"Value $(name)", "mace.documentation.interpolation-forbidden", "Escape interpolation marker in documentation", "Value \\$(name)", protocol.CodeActionKindQuickFix, false},
		{"output_doc { summary: 'Output'", "mace.documentation.output-directives-missing", "Add output directive list", "[output = 'data']", protocol.CodeActionKindQuickFix, true},
		{"fields: { nmae: 'Name', old: 'Old'", "", "Synchronize documentation fields with schema", "name: 'Name'\nage:", protocol.CodeActionKind("source.fixAll.mace"), false},
	}
	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, specification.preferred, text+"\n"+specification.updated)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	return diagnostics, actions
}
