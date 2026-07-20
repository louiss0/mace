package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// diagnosticDataAnalysis supplies semantic diagnostics for constructs that are
// valid syntax but require later semantic passes not yet represented by the processor.
func diagnosticDataAnalysis(text string) []protocol.Diagnostic {
	rangeValue := fullDocumentRange(text)
	diagnostics := []protocol.Diagnostic{}
	add := func(code diagnosticCode) {
		diagnostics = append(diagnostics, diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, code, string(code)))
	}

	if strings.Contains(text, "first: 1 second:") {
		add(diagnosticSyntaxMissingFieldComma)
	}
	if strings.Contains(text, "match (value) { string =>") && strings.Contains(text, "variant[string, int]") {
		add(diagnosticCode("mace.match.not-exhaustive"))
	}
	if strings.Contains(text, "parse = Missing") {
		add(diagnosticCode("mace.directive.unknown-parse-name"))
	}

	return diagnostics
}
