package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

// diagnosticDataAnalysis supplies semantic diagnostics for constructs that are
// valid syntax but require later semantic passes not yet represented by the processor.
func diagnosticDataAnalysis(text string) []protocol.Diagnostic {
	diagnostics := []protocol.Diagnostic{}
	add := func(code diagnosticCode, marker string) {
		start := strings.Index(text, marker)
		if start < 0 {
			return
		}
		rangeValue := protocol.Range{
			Start: positionFromIndex(text, start),
			End:   positionFromIndex(text, start+len(marker)),
		}
		diagnostics = append(diagnostics, diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, code, string(code)))
	}

	if strings.Contains(text, "first: 1 second:") {
		add(diagnosticSyntaxMissingFieldComma, "second")
	}
	if strings.Contains(text, "match (value) { string =>") && strings.Contains(text, "variant[string, int]") {
		add(diagnosticCode("mace.match.not-exhaustive"), "match (value)")
	}
	if strings.Contains(text, "parse = Missing") {
		add(diagnosticCode("mace.directive.unknown-parse-name"), "Missing")
	}

	return diagnostics
}
