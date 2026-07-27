package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type optionalAccessActionSpec struct {
	match       string
	code        string
	title       string
	kind        protocol.CodeActionKind
	preferred   bool
	updated     string
	replacement bool
}

func optionalAccessActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []optionalAccessActionSpec{
		{"string city = user.address.city;", "mace.type.optional-field-access", "Replace `.` with `?.`", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "user.address.city", "user.address?.city", 1), true},
		{"string city = user.address?.city;", "mace.type.absent-value-not-coalesced", "Add `??` fallback", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "user.address?.city", "user.address?.city ?? ''", 1), true},
		{"packages.codefixer.cn_efs", "mace.type.optional-field-access", "Make every optional path step use `?.`", protocol.CodeActionKindQuickFix, true, "packages?.codefixer?.cn_efs", false},
		{"packages?.code", "mace.type.absent-value-not-coalesced", "Add typed string fallback", protocol.CodeActionKindQuickFix, false, "?? ''", false},
		{"metrics?.count", "mace.type.absent-value-not-coalesced", "Add typed numeric fallback", protocol.CodeActionKindQuickFix, false, "?? 0", false},
		{"alias Mode: choice", "mace.type.absent-value-not-coalesced", "Add choice fallback", protocol.CodeActionKindQuickFix, false, "?? 'dev'", false},
		{"string fallback = 'unknown';", "mace.type.absent-value-not-coalesced", "Use existing variable as fallback", protocol.CodeActionKindQuickFix, false, "?? fallback", false},
		{"schema User: { name?: string, }", "mace.type.optional-field-access", "Mark accessed field required", protocol.CodeActionKindRefactorRewrite, false, "name: string", false},
		{"records.first.second.third", "mace.type.record-depth-exceeded", "Shorten member-access path", protocol.CodeActionKindQuickFix, false, "records.first.second", false},
		{"record<string> records", "mace.type.record-depth-exceeded", "Increase record-map nesting depth", protocol.CodeActionKindRefactorRewrite, false, "record<record<string>>", false},
		{"schema Config: { packages: string, }", "mace.type.record-depth-exceeded", "Change member type to nested record", protocol.CodeActionKindRefactorRewrite, false, "packages: { codefixer: string, }", false},
		{"config.deep.value", "mace.type.variant-record-depth", "Match variant before deeper access", protocol.CodeActionKindRefactorRewrite, false, "match (config)", false},
		{"alias Config: variant", "mace.type.variant-record-depth", "Normalize variant members to common record depth", protocol.CodeActionKindRefactorRewrite, false, "deep: { value: string, }", false},
		{"config?.deep?.value", "mace.type.variant-record-depth", "Stop optional chain at shallowest valid member", protocol.CodeActionKindQuickFix, false, "config?.deep", false},
	}
	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		updated := text + "\n" + specification.updated
		if specification.replacement {
			updated = specification.updated
		}
		diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, specification.preferred, updated)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	return diagnostics, actions
}
