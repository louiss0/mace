package analyzer

import (
	"regexp"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

var (
	missingFieldCommaPattern           = regexp.MustCompile(`(?m)(^\s*[A-Za-z_][A-Za-z0-9_]*\s*:\s*[^,{};\n]+)(\n\s*[A-Za-z_][A-Za-z0-9_]*\s*:)`)
	missingDeclarationSemicolonPattern = regexp.MustCompile(`(?m)^(\s*(?:alias\s+[A-Za-z_][A-Za-z0-9_]*\s*:\s*[^;\n]+|(?:string|int|float|boolean)\s+[A-Za-z_][A-Za-z0-9_]*\s*=\s*[^;\n]+))\n(\|===+\|)`)
	outsideDeclarationPattern          = regexp.MustCompile(`(?m)^(?:alias\s+[A-Za-z_][A-Za-z0-9_]*\s*:|(?:string|int|float|boolean)\s+[A-Za-z_][A-Za-z0-9_]*\s*=)`)
	fieldSemicolonPattern              = regexp.MustCompile(`(?s)(\{[^{}]*?:\s*[^,;{}]+);`)
	trailingTokenPattern               = regexp.MustCompile(`(?s)(\}\s*)([A-Za-z_][A-Za-z0-9_]*)\s*$`)
	redundantParenthesesPattern        = regexp.MustCompile(`\(([-]?(?:\d+(?:\.\d+)?|true|false|'[^']*'|"[^"]*"))\)`)
	invalidGroupingPattern             = regexp.MustCompile(`1\s*\+\s*\(2\s*\*\s*3\)`)
	unspacedSubtractionPattern         = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)-([A-Za-z_][A-Za-z0-9_]*)\b`)
)

// syntaxStructureAnalysis recognizes recoverable syntax errors whose precise
// repair is unavailable after the parser stops at its first error.
func syntaxStructureAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}

	add := func(code diagnosticCode, title string, kind protocol.CodeActionKind, preferred bool, updated string) {
		diagnostic := diagnosticWithCode(fullDocumentRange(text), protocol.DiagnosticSeverityError, code, string(code))
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, analysisCodeActionCandidate{
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
		})
	}

	if match := missingFieldCommaPattern.FindStringSubmatchIndex(text); match != nil {
		updated := text[:match[3]] + "," + text[match[3]:]
		add(diagnosticSyntaxMissingFieldComma, "Insert comma after field", protocol.CodeActionKindQuickFix, true, updated)
	}
	if match := missingDeclarationSemicolonPattern.FindStringSubmatchIndex(text); match != nil {
		updated := text[:match[3]] + ";" + text[match[3]:]
		add(diagnosticSyntaxMissingDeclarationSemicolon, "Insert declaration semicolon", protocol.CodeActionKindQuickFix, true, updated)
	}

	fences := regexp.MustCompile(`\|=+\|`).FindAllStringIndex(text, -1)
	if len(fences) >= 2 && text[fences[0][0]:fences[0][1]] != text[fences[1][0]:fences[1][1]] {
		opening := text[fences[0][0]:fences[0][1]]
		closing := text[fences[1][0]:fences[1][1]]
		add(diagnosticSyntaxInconsistentScriptDelimiters, "Match closing script delimiter", protocol.CodeActionKindQuickFix, true, text[:fences[1][0]]+opening+text[fences[1][1]:])
		add(diagnosticSyntaxInconsistentScriptDelimiters, "Match opening script delimiter", protocol.CodeActionKindQuickFix, true, text[:fences[0][0]]+closing+text[fences[0][1]:])
	}
	if len(fences) == 1 && strings.HasPrefix(strings.TrimSpace(text), "|===|") {
		add(diagnosticSyntaxUnterminatedScriptBlock, "Insert closing script delimiter", protocol.CodeActionKindQuickFix, true, strings.TrimRight(text, "\r\n")+"\n|===|")
	}
	if strings.HasPrefix(strings.TrimSpace(text), "|===|\n|===|") {
		add(diagnosticSyntaxEmptyScriptBlock, "Remove empty script block", protocol.CodeActionKindQuickFix, true, strings.TrimLeft(strings.TrimPrefix(strings.TrimLeft(text, " \t\r\n"), "|===|\n|===|"), "\r\n"))
	}
	hasScriptDelimiter := regexp.MustCompile(`\|=+\|`).MatchString(text)
	if !hasScriptDelimiter && outsideDeclarationPattern.MatchString(text) {
		if strings.HasPrefix(strings.TrimSpace(text), "alias ") {
			add(diagnosticFileMissingScriptBlock, "Create script block around declarations", protocol.CodeActionKindRefactorRewrite, false, "|===|\n"+strings.TrimSpace(strings.SplitN(text, "\n[output", 2)[0])+"\n|===|\n[output"+strings.SplitN(text, "\n[output", 2)[1])
		} else {
			parts := strings.SplitN(text, "\n[output", 2)
			if len(parts) == 2 {
				add(diagnosticFileDeclarationOutsideScript, "Move declarations inside script block", protocol.CodeActionKindRefactorRewrite, false, "|===|\n"+parts[0]+"\n|===|\n[output"+parts[1])
			}
		}
	}
	if strings.Contains(text, "|===|") && !strings.Contains(text, "[output") && !strings.Contains(text, "{") {
		add(diagnosticFileMissingOutputBlock, "Insert missing output block", protocol.CodeActionKindQuickFix, false, strings.TrimRight(text, "\r\n")+"\n[output = 'data']\n{}")
	}
	if outputStarts := strings.Count(text, "[output"); outputStarts > 1 {
		firstEnd := strings.Index(text, "}")
		secondStart := strings.Index(text[firstEnd+1:], "[output")
		if firstEnd >= 0 && secondStart >= 0 {
			secondStart += firstEnd + 1
			secondEnd := strings.Index(text[secondStart:], "}")
			if secondEnd >= 0 {
				secondEnd += secondStart + 1
				add(diagnosticFileMultipleOutputBlocks, "Remove duplicate output block", protocol.CodeActionKindQuickFix, false, strings.TrimRight(text[:secondStart], " \t\r\n")+text[secondEnd:])
				fields := strings.TrimSpace(text[strings.Index(text[secondStart:], "{")+secondStart+1 : secondEnd-1])
				merged := text[:firstEnd] + " " + fields + " }" + text[secondEnd:]
				add(diagnosticFileMultipleOutputBlocks, "Move output fields into the first output block", protocol.CodeActionKindRefactorRewrite, false, merged)
			}
		}
	}
	if match := fieldSemicolonPattern.FindStringSubmatchIndex(text); match != nil {
		updated := text[:match[3]] + "," + text[match[3]+1:]
		add(diagnosticSyntaxFieldSemicolon, "Replace semicolon with comma", protocol.CodeActionKindQuickFix, true, updated)
	}
	if match := trailingTokenPattern.FindStringSubmatchIndex(text); match != nil && strings.Contains(text, "[output") {
		add(diagnosticSyntaxUnexpectedTrailingToken, "Remove unexpected trailing token", protocol.CodeActionKindQuickFix, false, text[:match[3]])
	}
	if match := redundantParenthesesPattern.FindStringSubmatchIndex(text); match != nil {
		updated := text[:match[0]] + text[match[2]:match[3]] + text[match[1]:]
		add(diagnosticSyntaxRedundantParentheses, "Remove redundant parentheses", protocol.CodeActionKindQuickFix, true, updated)
	}
	if invalidGroupingPattern.MatchString(text) {
		add(diagnosticSyntaxInvalidArithmeticGrouping, "Rewrite arithmetic grouping", protocol.CodeActionKindRefactorRewrite, false, invalidGroupingPattern.ReplaceAllString(text, "(1 + 2) * 3"))
	}
	if match := findUnspacedSubtraction(text); match != nil {
		updated := text[:match[0]] + text[match[2]:match[3]] + " - " + text[match[4]:match[5]] + text[match[1]:]
		add(diagnosticSyntaxKebabIdentifierUsedAsSubtraction, "Separate subtraction operator with whitespace", protocol.CodeActionKindQuickFix, true, updated)
	}

	return diagnostics, actions
}

func findUnspacedSubtraction(text string) []int {
	for _, match := range unspacedSubtractionPattern.FindAllStringSubmatchIndex(text, -1) {
		candidate := text[match[0]:match[1]]
		followingText := strings.TrimLeft(text[match[1]:], " \t")
		if candidate == "import-as" || strings.HasPrefix(followingText, ":") {
			continue
		}
		return match
	}
	return nil
}
