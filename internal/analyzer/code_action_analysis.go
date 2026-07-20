package analyzer

import (
	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

type codeActionProvider func(string, string) ([]protocol.Diagnostic, []analysisCodeActionCandidate)

type codeActionAnalysis struct {
	diagnostics []protocol.Diagnostic
	actions     []analysisCodeActionCandidate
}

func newDiagnosticAction(
	text string,
	uri protocol.DocumentUri,
	code string,
	title string,
	kind protocol.CodeActionKind,
	preferred bool,
	updated string,
) (protocol.Diagnostic, analysisCodeActionCandidate) {
	diagnostic := diagnosticWithCode(fullDocumentRange(text), protocol.DiagnosticSeverityError, diagnosticCode(code), code)
	return diagnostic, analysisCodeActionCandidate{
		Range: diagnostic.Range,
		Action: protocol.CodeAction{
			Title:       title,
			Kind:        Ptr(kind),
			IsPreferred: Ptr(preferred),
			Diagnostics: []protocol.Diagnostic{diagnostic},
			Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{
				uri: {{Range: fullDocumentRange(text), NewText: updated}},
			}},
		},
	}
}

func newSourceAction(text string, uri protocol.DocumentUri, title string, kind protocol.CodeActionKind, updatedText string) analysisCodeActionCandidate {
	return analysisCodeActionCandidate{
		Range: fullDocumentRange(text),
		Action: protocol.CodeAction{
			Title: title,
			Kind:  Ptr(kind),
			Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{
				uri: {{Range: fullDocumentRange(text), NewText: updatedText}},
			}},
		},
	}
}

func collectCodeActionAnalysis(text string, documentPath string, providers ...codeActionProvider) codeActionAnalysis {
	analyses := lo.Map(providers, func(provider codeActionProvider, _ int) codeActionAnalysis {
		diagnostics, actions := provider(text, documentPath)
		return codeActionAnalysis{diagnostics: diagnostics, actions: actions}
	})

	return codeActionAnalysis{
		diagnostics: lo.FlatMap(analyses, func(analysis codeActionAnalysis, _ int) []protocol.Diagnostic {
			return analysis.diagnostics
		}),
		actions: lo.FlatMap(analyses, func(analysis codeActionAnalysis, _ int) []analysisCodeActionCandidate {
			return analysis.actions
		}),
	}
}
