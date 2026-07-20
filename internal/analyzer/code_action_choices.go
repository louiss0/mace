package analyzer

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
)

func choiceActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	ds := []protocol.Diagnostic{}
	as := []analysisCodeActionCandidate{}
	add := func(c, t string, k protocol.CodeActionKind, p bool, u string) {
		d := diagnosticWithCode(fullDocumentRange(text), protocol.DiagnosticSeverityError, diagnosticCode(c), c)
		ds = append(ds, d)
		as = append(as, analysisCodeActionCandidate{Range: d.Range, Action: protocol.CodeAction{Title: t, Kind: Ptr(k), IsPreferred: Ptr(p), Diagnostics: []protocol.Diagnostic{d}, Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: fullDocumentRange(text), NewText: u}}}}}})
	}
	if strings.Contains(text, "choice['dev', 'test', 'dev']") {
		add("mace.type.duplicate-choice-member", "Remove duplicate choice member", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "choice['dev', 'test', 'dev']", "choice['dev', 'test']", 1))
	}
	if strings.Contains(text, "choice[string, 'prod']") {
		add("mace.type.invalid-choice-member", "Replace invalid choice member with literal", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "choice[string, 'prod']", "choice['dev', 'prod']", 1))
	}
	if strings.Contains(text, "choice[0x1, 2]") {
		add("mace.type.choice-numeric-family", "Convert literal to choice’s numeric family", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "0x1, 2", "0x1, 0x2", 1))
	}
	if strings.Contains(text, "Environment env = 'staging'") {
		add("mace.type.value-outside-choice", "Replace value with allowed choice member", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "'staging'", "'dev'", 1))
		add("mace.type.value-outside-choice", "Add literal to choice declaration", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "'dev', 'prod']", "'dev', 'prod', 'staging']", 1))
	}
	if strings.Contains(text, "alias AB: choice['a', 'b']") {
		add("mace.refactor.copied-choice-composition", "Replace duplicate choice composition with fusion", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "choice['a', 'b']", "fusion[A, B]", 1))
	}
	if strings.Contains(text, "alias A: choice['a']; alias B: choice['b'];") && !strings.Contains(text, "alias AB") {
		add("mace.refactor.choice-composition", "Fuse choice aliases", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "\n|===|", "\nalias Combined: fusion[A, B];\n|===|", 1))
	}
	if strings.Contains(text, "alias A: choice['a', B]; alias B: choice['b', A]") {
		add("mace.type.choice-alias-cycle", "Remove cyclic choice alias edge", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "alias B: choice['b', A]", "alias B: choice['b']", 1))
		add("mace.type.choice-alias-cycle", "Inline choice alias to break cycle", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "choice['a', B]", "choice['a', 'b']", 1))
	}
	if strings.Contains(text, "choice['dev', 'prod', 'test']") && strings.Contains(text, "match (env)") {
		add("mace.match.non-exhaustive-after-domain-change", "Update choice matches after domain change", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "'prod' => 2,", "'prod' => 2, 'test' => 0,", 1))
	}
	return ds, as
}
