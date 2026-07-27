package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type fusionActionSpec struct {
	match     string
	code      string
	title     string
	kind      protocol.CodeActionKind
	preferred bool
	updated   string
}

func fusionActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []fusionActionSpec{
		{"fusion[User, int]", "mace.type.invalid-fusion-member", "Remove invalid fusion member", protocol.CodeActionKindQuickFix, false, "fusion[User]"},
		{"fusion[User, Mode]", "mace.type.fusion-domain-mismatch", "Replace fusion with variant", protocol.CodeActionKindRefactorRewrite, false, "variant[User, Mode]"},
		{"fusion[User, Mode]", "mace.type.fusion-domain-mismatch", "Split mixed fusion into two aliases", protocol.CodeActionKindRefactorExtract, false, "alias CombinedRecord\nalias CombinedChoice"},
		{"schema B: { id: float, }", "mace.type.fusion-field-conflict", "Change conflicting field type to common type", protocol.CodeActionKindRefactorRewrite, false, "id: float"},
		{"schema B: { id: string, }", "mace.type.fusion-field-conflict", "Expand conflicting fields to the same variant", protocol.CodeActionKindRefactorRewrite, false, "id: variant[int, string]"},
		{"schema B: { id: string, }", "mace.type.fusion-field-conflict", "Rename conflicting field in one member", protocol.CodeActionKindRefactorRewrite, false, "external_id: string"},
		{"schema B: { id: string, }", "mace.type.fusion-field-conflict", "Remove conflicting fusion member", protocol.CodeActionKindQuickFix, false, "fusion[A]"},
		{"schema B: { id?: int, }", "mace.type.fusion-optionality-conflict", "Mark optional fusion field required", protocol.CodeActionKindQuickFix, false, "id: int"},
		{"alias A: choice['a']; alias B: choice['a', 'b'];", "", "Deduplicate fused choice domain", protocol.CodeActionKind("source.fixAll.mace"), false, "choice['a', 'b']"},
		{"schema A: { a: string, }; schema B:", "mace.type.nested-fusion", "Flatten nested record fusion", protocol.CodeActionKindQuickFix, true, "fusion[A, B, C]"},
		{"alias A: choice['a']; alias B: choice['b'];", "mace.type.nested-fusion", "Flatten nested choice fusion", protocol.CodeActionKindQuickFix, true, "fusion[A, B, C]"},
		{"fusion[{ id: int, name: string, },", "mace.refactor.repeated-fusion-record", "Convert repeated inline record to schema", protocol.CodeActionKindRefactorExtract, false, "schema Base\nfusion[Base"},
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
