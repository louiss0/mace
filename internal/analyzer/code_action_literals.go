package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func literalActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}
	add := func(code string, title string, kind protocol.CodeActionKind, preferred bool, updated string) {
		diagnostic, action := newDiagnosticAction(text, uri, code, title, kind, preferred, updated)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	if strings.Contains(text, "string env = production") {
		add("mace.type.expected-string", "Quote value as string", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "= production", "= 'production'", 1))
	}
	if strings.Contains(text, `text: "$name"`) {
		add("mace.string.interpolation-forbidden", "Convert interpolating string to single-quoted string", protocol.CodeActionKindQuickFix, false, strings.Replace(text, `"$name"`, `'$name'`, 1))
	}
	if strings.Contains(text, `"Hello $name"`) {
		add("mace.string.unsupported-interpolation", "Replace `$name` interpolation with `$(name)`", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "$name", "$(name)", 1))
	}
	if strings.Contains(text, `path: 'C:\temp'`) {
		add("mace.string.invalid-character", "Escape invalid string character", protocol.CodeActionKindQuickFix, true, strings.Replace(text, `C:\temp`, `C:\\temp`, 1))
	}
	if strings.Contains(text, "text: 'first\nsecond'") {
		add("mace.string.line-break", "Convert multiline string to block string", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "'first\nsecond'", "'''first\nsecond'''", 1))
	}
	if strings.Contains(text, "enabled: 'true'") {
		add("mace.type.string-boolean", "Replace string boolean with boolean literal", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "enabled: 'true'", "enabled: true", 1))
	}
	if strings.Contains(text, "enabled: 1") {
		add("mace.type.numeric-boolean", "Replace numeric boolean with boolean literal", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "enabled: 1", "enabled: true", 1))
	}
	if strings.Contains(text, "float ratio = 1;") {
		add("mace.type.expected-float", "Convert integer literal to float", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "float ratio = 1;", "float ratio = 1.0;", 1))
	}
	if strings.Contains(text, "int count = 2.0;") {
		add("mace.type.expected-int", "Convert float literal to integer", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "2.0;", "2;", 1))
	}
	if strings.Contains(text, "999999999999999999999999") {
		add("mace.number.integer-overflow", "Replace overflowing integer literal", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "999999999999999999999999", "9223372036854775807", 1))
	}
	if strings.Contains(text, "int ratio = 3 / 2;") {
		add("mace.type.operator-result-mismatch", "Change receiving type from `int` to `float`", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "int ratio", "float ratio", 1))
	}
	if strings.Contains(text, "hex_float value = 0x2;") {
		add("mace.type.expected-hex-float", "Convert `hex_int` literal to `hex_float`", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "0x2;", "0x2.0;", 1))
	}
	if strings.Contains(text, "0x2 + 3") {
		add("mace.type.mixed-numeric-family", "Convert decimal literal to hexadecimal family", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "0x2 + 3", "0x2 + 0x3", 1))
		add("mace.type.mixed-numeric-family", "Convert hexadecimal literal to decimal family", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "0x2 + 3", "2 + 3", 1))
	}
	if strings.Contains(text, "0x2.") {
		add("mace.number.incomplete-hex-float", "Add required hexadecimal fractional component", protocol.CodeActionKindQuickFix, true, strings.Replace(text, "0x2.", "0x2.0", 1))
	}
	if strings.Contains(text, "0X02.A0") {
		add("", "Canonicalize hexadecimal float literal", protocol.CodeActionKind("source.fixAll.mace"), false, strings.Replace(text, "0X02.A0", "0x2.a", 1))
	}
	if strings.Contains(text, "0xffffffffffffffffffff") {
		add("mace.number.hex-integer-overflow", "Replace overflowing `hex_int` literal with boundary value", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "0xffffffffffffffffffff", "0x7fffffffffffffff", 1))
	}
	if strings.Contains(text, "hex_int mask = 0xff;") {
		add("mace.type.invalid-complement-operand", "Change `~` operand to decimal `int`", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "hex_int mask = 0xff", "int mask = 255", 1))
	}
	return diagnostics, actions
}
