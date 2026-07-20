package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type operatorActionSpec struct {
	match, code, title, updated string
	kind                        protocol.CodeActionKind
	preferred                   bool
}

func operatorActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []operatorActionSpec{
		{"1 && 2", "mace.type.invalid-binary-operator", "Replace logical operator with bitwise operator", "1 & 2", protocol.CodeActionKindQuickFix, false},
		{"true & false", "mace.type.invalid-binary-operator", "Replace bitwise operator with logical operator", "true && false", protocol.CodeActionKindQuickFix, false},
		{"~true", "mace.type.invalid-unary-operator", "Replace `~` with `!`", "!true", protocol.CodeActionKindQuickFix, false},
		{"0x2 + 3", "mace.type.mixed-numeric-family", "Convert operand to matching numeric family", "0x2 + 0x3", protocol.CodeActionKindQuickFix, false},
		{"hex_int value = 0x4 / 0x2", "mace.type.operator-result-mismatch", "Change receiving type to operator result type", "hex_float value", protocol.CodeActionKindQuickFix, true},
		{"true + false", "mace.type.invalid-binary-operator", "Replace invalid operator with compatible operator", "true || false", protocol.CodeActionKindQuickFix, false},
		{"1 + 2 * 3", "mace.expression.suspicious-precedence", "Add arithmetic grouping that changes precedence", "(1 + 2) * 3", protocol.CodeActionKindRefactorRewrite, false},
		{"(true && false)", "mace.syntax.forbidden-grouping", "Remove forbidden non-arithmetic grouping", "true && false", protocol.CodeActionKindQuickFix, true},
		{"2 ** -1", "mace.operator.invalid-exponent", "Make exponent non-negative integer", "2 ** 1", protocol.CodeActionKindQuickFix, false},
		{"1 << 2.0", "mace.operator.invalid-shift", "Convert shift amount to integer literal", "1 << 2", protocol.CodeActionKindQuickFix, false},
		{"1 << -2", "mace.operator.invalid-shift", "Replace negative shift amount", "1 << 0", protocol.CodeActionKindQuickFix, false},
		{"total / count", "mace.operator.possible-division-by-zero", "Guard division with conditional", "count == 0 ?", protocol.CodeActionKindRefactorRewrite, false},
		{"10 / 0", "mace.operator.division-by-zero", "Replace known zero divisor", "10 / 1", protocol.CodeActionKindQuickFix, false},
		{"9223372036854775807 + 1", "mace.operator.constant-overflow", "Replace overflowing constant expression with literal result boundary", "9223372036854775807", protocol.CodeActionKindQuickFix, false},
		{"int value = 3 / 2", "mace.operator.integer-result-insufficient", "Change arithmetic family to float", "float value = 3.0 / 2.0", protocol.CodeActionKindRefactorRewrite, false},
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
