package analyzer

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
)

type matchSpec struct {
	match, code, title, updated string
	kind                        protocol.CodeActionKind
	preferred, replacement      bool
}

func matchActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []matchSpec{
		{"string => 'text'\n  int", "mace.syntax.missing-match-arm-comma", "Add trailing comma to match arm", strings.Replace(text, "string => 'text'\n", "string => 'text',\n", 1), protocol.CodeActionKindQuickFix, true, true},
		{"string => 'text',\n  string => 'again',", "mace.match.duplicate-pattern", "Remove duplicate match arm", strings.Replace(text, "  string => 'again',\n", "", 1), protocol.CodeActionKindQuickFix, true, true},
		{"'text' => 'text',\n  int", "mace.match.variant-literal-pattern", "Replace literal pattern with type pattern", strings.Replace(text, "'text' =>", "string =>", 1), protocol.CodeActionKindQuickFix, true, true},
		{"string => 1,\n  'off'", "mace.match.choice-type-pattern", "Replace type pattern with 'on'", strings.Replace(text, "string =>", "'on' =>", 1), protocol.CodeActionKindQuickFix, false, true},
		{"variant[string, int] value = 1; string result = match", "mace.match.not-exhaustive", "Add missing match arm for `Type`", "int => ''", protocol.CodeActionKindQuickFix, false, false},
		{"variant[string, int, boolean] value", "mace.match.not-exhaustive", "Add all missing match arms", "int => ''\nboolean => ''", protocol.CodeActionKindQuickFix, false, false},
		{"string => 'a', string => 'b'", "mace.match.duplicate-pattern", "Replace duplicate pattern with missing member", "int => 'b'", protocol.CodeActionKindQuickFix, true, false},
		{"alias Text: choice", "mace.match.overlapping-pattern", "Split overlapping pattern into disjoint arms", "'a' =>\n'b' =>", protocol.CodeActionKindRefactorRewrite, false, false},
		{"'auto' => 1, 'off'", "mace.match.pattern-outside-domain", "Replace pattern with valid domain member", "'on' => 1", protocol.CodeActionKindQuickFix, false, false},
		{"'on' => 1, 'off' => 0, 'auto'", "mace.match.pattern-outside-domain", "Remove extra match pattern", "'off' => 0", protocol.CodeActionKindQuickFix, true, false},
		{"string value = 'text'; string result", "mace.match.concrete-input", "Convert source declaration to variant", "variant[string, int] value", protocol.CodeActionKindRefactorRewrite, false, false},
		{"string value = 'on'; int result", "mace.match.concrete-input", "Convert source declaration to choice", "choice['on', 'off'] value", protocol.CodeActionKindRefactorRewrite, false, false},
		{"choice['on'] value", "mace.match.single-member-domain", "Replace unnecessary match with direct expression", "int result = 1;", protocol.CodeActionKindRefactorRewrite, false, false},
		{"string result = match (value) { 'on' => 'yes', 'off' => 0", "mace.type.match-result-mismatch", "Expand receiving type for match result variant", "variant[string, int] result", protocol.CodeActionKindQuickFix, false, false},
		{"string result = match (value) { 'on' => 'yes', 'off' => 0", "mace.type.match-result-mismatch", "Unify match arm result types", "'off' => ''", protocol.CodeActionKindQuickFix, false, false},
		{"alias Value: variant[string, int, boolean]", "mace.match.domain-changed", "Update match after variant alias change", "boolean =>", protocol.CodeActionKindRefactorRewrite, false, false},
		{"alias Mode: choice['on', 'off', 'auto']", "mace.match.domain-changed", "Update match after choice alias change", "'auto' =>", protocol.CodeActionKindRefactorRewrite, false, false}}
	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		updatedText := text + "\n" + specification.updated
		if specification.replacement {
			updatedText = specification.updated
		}
		diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, specification.preferred, updatedText)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	return diagnostics, actions
}
