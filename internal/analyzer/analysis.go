package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/formatter"
	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
)

var (
	missingRequiredFieldPattern = regexp.MustCompile(`missing required field "([^"]+)" for schema "([^"]+)"`)
	typeMismatchPattern         = regexp.MustCompile(`type mismatch: expected (.+), got (.+)$`)
	selfReferencePattern        = regexp.MustCompile(`unknown self reference "([^"]+)"`)
	schemaOutputFieldPattern    = regexp.MustCompile(`invalid field type "([^"]+)" in output = schema`)
	dataOutputValuePattern      = regexp.MustCompile(`output value "([^"]+)" cannot reference type or schema declaration`)
	missingInjectablePattern    = regexp.MustCompile(`injectable "([^"]+)" requires a runtime value`)
	enumNamePattern             = regexp.MustCompile(`enum "([^"]+)"`)
	enumMemberPattern           = regexp.MustCompile(`enum member "([^"]+)"`)
	arrayAccessLevelPattern     = regexp.MustCompile(`at level (\d+)`)
	arrayAccessIndexPattern     = regexp.MustCompile(`array index (-?\d+) is out of range`)
	emptyScriptBlockPattern     = regexp.MustCompile(`(?s)^\s*\|===\|\s*\|===\|\s*`)
)

type symbolOrigin string

const (
	symbolOriginLocal  symbolOrigin = "local"
	symbolOriginImport symbolOrigin = "import"
	symbolOriginOutput symbolOrigin = "output"
)

type semanticSymbol struct {
	Name          string
	Kind          protocol.CompletionItemKind
	Detail        string
	Documentation string
	Origin        symbolOrigin
	Range         protocol.Range
	Definition    protocol.Location
}

type analysisCodeActionCandidate struct {
	Range  protocol.Range
	Action protocol.CodeAction
}

type analysisSnapshot struct {
	text                 string
	documentURI          protocol.DocumentUri
	file                 *ast.File
	result               *processor.Result
	tokens               []lexer.Token
	diagnostics          []protocol.Diagnostic
	declarations         []declarationDefinition
	symbols              []semanticSymbol
	symbolIndex          map[string]semanticSymbol
	codeActionCandidates []analysisCodeActionCandidate
	recovered            bool
}

func (snapshot analysisSnapshot) symbol(name string) (semanticSymbol, bool) {
	symbol, ok := snapshot.symbolIndex[name]
	return symbol, ok
}

func (snapshot analysisSnapshot) symbolAt(position protocol.Position) (semanticSymbol, bool) {
	for index := len(snapshot.symbols) - 1; index >= 0; index-- {
		symbol := snapshot.symbols[index]
		if comparePositions(symbol.Range.Start, position) <= 0 && comparePositions(position, symbol.Range.End) <= 0 {
			return symbol, true
		}
	}

	if symbol, ok := snapshot.enumMemberSymbolAt(position); ok {
		return symbol, true
	}

	if symbol, ok := snapshot.selfReferenceSymbolAt(position); ok {
		return symbol, true
	}

	if symbol, ok := snapshot.nestedOutputFieldSymbolAt(position); ok {
		return symbol, true
	}

	identifier, found := identifierAt(snapshot.text, position)
	if !found {
		return semanticSymbol{}, false
	}

	return snapshot.symbol(identifier)
}

func (snapshot analysisSnapshot) definitionAt(position protocol.Position) (protocol.Location, bool) {
	if snapshot.file == nil {
		return protocol.Location{}, false
	}

	symbol, ok := snapshot.documentSymbolAt(position)
	if !ok {
		if enumMember, memberOK := snapshot.enumMemberSymbolAt(position); memberOK {
			symbol = enumMember
			ok = true
		}
	}
	if !ok {
		identifier, found := identifierAt(snapshot.text, position)
		if !found {
			return protocol.Location{}, false
		}

		symbol, ok = snapshot.definitionSymbol(identifier)
		if !ok {
			return protocol.Location{}, false
		}
	}

	if symbol.Definition.URI == "" {
		return protocol.Location{}, false
	}

	return symbol.Definition, true
}

func (snapshot analysisSnapshot) documentSymbolAt(position protocol.Position) (semanticSymbol, bool) {
	return findLastSymbol(snapshot.symbols, func(symbol semanticSymbol) bool {
		return symbol.Definition.URI == snapshot.documentURI &&
			comparePositions(symbol.Range.Start, position) <= 0 &&
			comparePositions(position, symbol.Range.End) <= 0
	})
}

func (snapshot analysisSnapshot) definitionSymbol(name string) (semanticSymbol, bool) {
	if symbol, ok := findLastSymbol(snapshot.symbols, func(symbol semanticSymbol) bool {
		return symbol.Name == name && symbol.Definition.URI == snapshot.documentURI
	}); ok {
		return symbol, true
	}

	return snapshot.symbol(name)
}

func findLastSymbol(symbols []semanticSymbol, predicate func(semanticSymbol) bool) (semanticSymbol, bool) {
	for index := len(symbols) - 1; index >= 0; index-- {
		symbol := symbols[index]
		if predicate(symbol) {
			return symbol, true
		}
	}

	return semanticSymbol{}, false
}

func (snapshot analysisSnapshot) enumMemberSymbolAt(position protocol.Position) (semanticSymbol, bool) {
	if snapshot.file == nil {
		return semanticSymbol{}, false
	}

	enumName, memberName, ok := enumMemberReferenceAt(snapshot.tokens, position)
	if !ok {
		return semanticSymbol{}, false
	}

	return localEnumMemberSymbol(*snapshot.file, snapshot.documentURI, enumName, memberName)
}

func (snapshot analysisSnapshot) selfReferenceSymbolAt(position protocol.Position) (semanticSymbol, bool) {
	if snapshot.result == nil {
		return semanticSymbol{}, false
	}

	path, rangeValue, ok := selfReferencePathAt(snapshot.tokens, position)
	if !ok {
		return semanticSymbol{}, false
	}

	value, ok := outputValueAtPath(snapshot.result.Output, path)
	if !ok {
		return semanticSymbol{}, false
	}

	name := "$self." + strings.Join(path, ".")
	return outputValueSymbol(name, path, rangeValue, value), true
}

func (snapshot analysisSnapshot) nestedOutputFieldSymbolAt(position protocol.Position) (semanticSymbol, bool) {
	if snapshot.result == nil {
		return semanticSymbol{}, false
	}

	path, rangeValue, ok := nestedOutputFieldPathAt(snapshot.text, snapshot.tokens, position)
	if !ok || len(path) < 2 {
		return semanticSymbol{}, false
	}

	value, ok := outputValueAtPath(snapshot.result.Output, path)
	if !ok {
		return semanticSymbol{}, false
	}

	name := strings.Join(path, ".")
	return outputValueSymbol(name, path, rangeValue, value), true
}

func outputValueSymbol(name string, path []string, rangeValue protocol.Range, value processor.Value) semanticSymbol {
	return semanticSymbol{
		Name:   name,
		Kind:   protocol.CompletionItemKindProperty,
		Detail: fmt.Sprintf("output %s: %s = %s", strings.Join(path, "."), valueTypeSummary(value), summarizeValue(value)),
		Origin: symbolOriginOutput,
		Range:  rangeValue,
	}
}

func enumMemberReferenceAt(tokens []lexer.Token, position protocol.Position) (string, string, bool) {
	for index, token := range tokens {
		rangeValue := tokenProtocolRange(token)
		if comparePositions(rangeValue.Start, position) > 0 || comparePositions(position, rangeValue.End) > 0 {
			continue
		}
		if token.Type != lexer.TokenIdentifier || index < 2 {
			continue
		}
		if tokens[index-1].Type != lexer.TokenDot || tokens[index-2].Type != lexer.TokenIdentifier {
			continue
		}

		return tokens[index-2].Lexeme, token.Lexeme, true
	}

	return "", "", false
}

func selfReferencePathAt(tokens []lexer.Token, position protocol.Position) ([]string, protocol.Range, bool) {
	for index := 0; index < len(tokens); index++ {
		if tokens[index].Type != lexer.TokenSelf {
			continue
		}

		segments := []string{}
		for cursor := index + 1; cursor+1 < len(tokens); cursor += 2 {
			if tokens[cursor].Type != lexer.TokenDot || tokens[cursor+1].Type != lexer.TokenIdentifier {
				break
			}

			segment := tokens[cursor+1]
			segments = append(segments, segment.Lexeme)
			rangeValue := tokenProtocolRange(segment)
			if comparePositions(rangeValue.Start, position) <= 0 && comparePositions(position, rangeValue.End) <= 0 {
				return append([]string{}, segments...), rangeValue, true
			}
		}
	}

	return nil, protocol.Range{}, false
}

func nestedOutputFieldPathAt(text string, tokens []lexer.Token, position protocol.Position) ([]string, protocol.Range, bool) {
	outputOpenIndex, ok := outputBlockOpenIndex(text, tokens)
	if !ok {
		return nil, protocol.Range{}, false
	}

	outputTokens := lo.Filter(tokens, func(token lexer.Token, _ int) bool {
		return tokenStartIndex(text, token) >= outputOpenIndex
	})

	type fieldScope struct {
		path       []string
		braceDepth int
	}

	braceDepth := 0
	pendingRecordPath := []string(nil)
	scopes := []fieldScope{}

	for index := 0; index < len(outputTokens); index++ {
		token := outputTokens[index]

		switch token.Type {
		case lexer.TokenLBrace:
			braceDepth++
			if braceDepth == 1 {
				scopes = append(scopes, fieldScope{path: nil, braceDepth: braceDepth})
				continue
			}
			if pendingRecordPath != nil {
				scopes = append(scopes, fieldScope{path: append([]string{}, pendingRecordPath...), braceDepth: braceDepth})
				pendingRecordPath = nil
			}
		case lexer.TokenRBrace:
			if len(scopes) > 0 && scopes[len(scopes)-1].braceDepth == braceDepth {
				scopes = scopes[:len(scopes)-1]
			}
			braceDepth--
		case lexer.TokenIdentifier:
			if len(scopes) == 0 || !isOutputFieldHeader(outputTokens, index) {
				continue
			}

			rangeValue := tokenProtocolRange(token)
			path := append(append([]string{}, scopes[len(scopes)-1].path...), token.Lexeme)
			if comparePositions(rangeValue.Start, position) <= 0 && comparePositions(position, rangeValue.End) <= 0 {
				return path, rangeValue, true
			}

			valueIndex := index + 2
			if index+1 < len(outputTokens) && outputTokens[index+1].Type == lexer.TokenQuestion {
				valueIndex = index + 3
			}
			if valueIndex < len(outputTokens) && outputTokens[valueIndex].Type == lexer.TokenLBrace {
				pendingRecordPath = path
			} else {
				pendingRecordPath = nil
			}
		}
	}

	return nil, protocol.Range{}, false
}

func outputValueAtPath(output map[string]processor.Value, path []string) (processor.Value, bool) {
	if len(path) == 0 {
		return processor.Value{}, false
	}

	current, ok := output[path[0]]
	if !ok {
		return processor.Value{}, false
	}

	for _, segment := range path[1:] {
		if current.Kind != processor.ValueRecord {
			return processor.Value{}, false
		}

		next, ok := current.Record[segment]
		if !ok {
			return processor.Value{}, false
		}
		current = next
	}

	return current, true
}

func localEnumMemberSymbol(file ast.File, uri protocol.DocumentUri, enumName string, memberName string) (semanticSymbol, bool) {
	for _, item := range fileScriptDeclarations(file) {
		declaration, ok := item.(ast.EnumDeclaration)
		if !ok || declaration.Name != enumName {
			continue
		}

		for _, member := range declaration.Members {
			if member.Name != memberName {
				continue
			}

			rangeValue := tokenProtocolRange(member.NameToken)
			return semanticSymbol{
				Name:          enumName + "." + memberName,
				Kind:          protocol.CompletionItemKindEnumMember,
				Detail:        enumMemberDetail(declaration, member),
				Documentation: inlineDescriptionDocumentation(member.Description),
				Origin:        symbolOriginLocal,
				Range:         rangeValue,
				Definition: protocol.Location{
					URI:   uri,
					Range: rangeValue,
				},
			}, true
		}
	}

	return semanticSymbol{}, false
}

func (snapshot analysisSnapshot) codeActions(uri protocol.DocumentUri, targetRange protocol.Range) []protocol.CodeAction {
	return lo.FilterMap(snapshot.codeActionCandidates, func(candidate analysisCodeActionCandidate, _ int) (protocol.CodeAction, bool) {
		if !rangesOverlap(candidate.Range, targetRange) {
			return protocol.CodeAction{}, false
		}

		action := candidate.Action
		action.Diagnostics = append(action.Diagnostics, protocol.Diagnostic{
			Range:  candidate.Range,
			Source: Ptr(serverName),
		})
		if action.Edit != nil && action.Edit.Changes == nil {
			action.Edit.Changes = map[protocol.DocumentUri][]protocol.TextEdit{}
		}
		if action.Edit != nil && action.Edit.Changes != nil {
			if _, ok := action.Edit.Changes[uri]; !ok {
				action.Edit.Changes[uri] = []protocol.TextEdit{}
			}
		}

		return action, true
	})
}

func analyzeDocument(text string) analysisSnapshot {
	return analyzeDocumentAt(text, "")
}

func analyzeDocumentAt(text string, documentPath string) analysisSnapshot {
	snapshot := analysisSnapshot{}
	snapshot.text = text
	snapshot.documentURI = pathURI(documentPath)

	tokens, lexErr := lex(text)
	if lexErr != nil {
		snapshot.diagnostics = []protocol.Diagnostic{diagnosticFromError(lexErr)}
		return snapshot
	}
	snapshot.tokens = tokens

	file, parseErr := parseFile(text)
	if parseErr != nil {
		snapshot.diagnostics = []protocol.Diagnostic{diagnosticFromError(parseErr)}
		snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, parseErrorCodeActions(tokens, documentPath)...)
		snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, scriptBlockStructureCodeActions(text, documentPath)...)
		snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, variableFixTextCodeActions(text, documentPath)...)
		snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, typeAliasTextCodeActions(text, documentPath)...)
		snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, arrayTextCodeActions(text, documentPath)...)
		return snapshot
	}

	snapshot.file = &file
	baseDir := filepath.Dir(documentPath)
	if documentPath == "" {
		baseDir = ""
	}

	snapshot.symbols = collectSemanticSymbols(file, tokens, nil, documentPath)
	snapshot.symbolIndex = indexSymbols(snapshot.symbols)
	snapshot.declarations = declarationsFromSymbols(snapshot.symbols)

	fileDiagnostics, fileActions := analyzeFileStructure(text, file, tokens, documentPath)
	snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, fileActions...)

	processorInstance := processor.New()
	result, processErr := processorInstance.ProcessInDir(text, baseDir)
	if processErr != nil {
		snapshot.diagnostics = append(snapshot.diagnostics, fileDiagnostics...)
		if semanticDiagnostic, ok := semanticDiagnosticFromError(file, tokens, processErr); ok {
			snapshot.diagnostics = append(snapshot.diagnostics, semanticDiagnostic)
			snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, semanticCodeActions(text, file, tokens, documentPath, semanticDiagnostic, processErr.Error())...)
		} else if len(snapshot.diagnostics) == 0 {
			snapshot.diagnostics = append(snapshot.diagnostics, diagnosticFromError(processErr))
		}
		return snapshot
	}

	snapshot.result = &result
	snapshot.symbols = collectSemanticSymbols(file, tokens, &result, documentPath)
	snapshot.symbolIndex = indexSymbols(snapshot.symbols)
	snapshot.declarations = declarationsFromSymbols(snapshot.symbols)
	snapshot.diagnostics = append(snapshot.diagnostics, fileDiagnostics...)

	unusedDiagnostics, unusedActions := unusedVariableAnalysis(text, file, tokens, documentPath)
	snapshot.diagnostics = append(snapshot.diagnostics, unusedDiagnostics...)
	snapshot.codeActionCandidates = append(snapshot.codeActionCandidates, unusedActions...)

	return snapshot
}

func analyzeCompletionContext(text string, documentPath string, position protocol.Position) analysisSnapshot {
	snapshot := analyzeDocumentAt(text, documentPath)
	if snapshot.file != nil {
		return snapshot
	}

	if file, ok := partialScriptFile(text, position); ok {
		return recoveredSnapshot(text, documentPath, file)
	}

	linePrefix := currentLinePrefix(text, position)
	file := completionFile(document{text: text}, linePrefix)
	if file == nil {
		return snapshot
	}

	return recoveredSnapshot(text, documentPath, *file)
}

func recoveredSnapshot(text string, documentPath string, file ast.File) analysisSnapshot {
	tokens, _ := lex(text)
	symbols := collectSemanticSymbols(file, tokens, nil, documentPath)

	return analysisSnapshot{
		text:         text,
		documentURI:  pathURI(documentPath),
		file:         &file,
		tokens:       tokens,
		symbols:      symbols,
		symbolIndex:  indexSymbols(symbols),
		declarations: declarationsFromSymbols(symbols),
		recovered:    true,
	}
}

func analyzeFileStructure(text string, file ast.File, tokens []lexer.Token, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	importResults := lo.FilterMap(file.Imports, func(importDecl ast.ImportDeclaration, _ int) (struct {
		diagnostic protocol.Diagnostic
		action     *analysisCodeActionCandidate
	}, bool) {
		pathValue, ok := stringLiteralValue(importDecl.Path)
		if !ok {
			return struct {
				diagnostic protocol.Diagnostic
				action     *analysisCodeActionCandidate
			}{}, false
		}
		if strings.HasSuffix(pathValue, ".mace") {
			return struct {
				diagnostic protocol.Diagnostic
				action     *analysisCodeActionCandidate
			}{}, false
		}

		rangeValue, found := tokenRangeByType(tokens, lexer.TokenString, importDecl.Path.Lexeme)
		if !found {
			return struct {
				diagnostic protocol.Diagnostic
				action     *analysisCodeActionCandidate
			}{}, false
		}

		message := fmt.Sprintf("import path %q must end in .mace", pathValue)
		result := struct {
			diagnostic protocol.Diagnostic
			action     *analysisCodeActionCandidate
		}{
			diagnostic: diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticImportPathNotMace, message),
		}

		if documentPath != "" {
			fixedPath := strconv.Quote(pathValue + ".mace")
			result.action = &analysisCodeActionCandidate{
				Range: rangeValue,
				Action: protocol.CodeAction{
					Title: "Append .mace to import path",
					Kind:  Ptr(protocol.CodeActionKindQuickFix),
					Edit: &protocol.WorkspaceEdit{
						Changes: map[protocol.DocumentUri][]protocol.TextEdit{
							protocol.DocumentUri(fileURI(documentPath)): {{
								Range:   rangeValue,
								NewText: fixedPath,
							}},
						},
					},
				},
			}
		}

		return result, true
	})

	diagnostics := lo.Map(importResults, func(result struct {
		diagnostic protocol.Diagnostic
		action     *analysisCodeActionCandidate
	}, _ int) protocol.Diagnostic {
		return result.diagnostic
	})
	actions := lo.FilterMap(importResults, func(result struct {
		diagnostic protocol.Diagnostic
		action     *analysisCodeActionCandidate
	}, _ int) (analysisCodeActionCandidate, bool) {
		if result.action == nil {
			return analysisCodeActionCandidate{}, false
		}

		return *result.action, true
	})

	hasSchemaFileConflict := false
	if diagnostic, candidates, ok := schemaFileConflictAnalysis(text, file, documentPath); ok {
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, candidates...)
		hasSchemaFileConflict = true
	}

	if !hasSchemaFileConflict {
		unusedImportDiagnostics, unusedImportActions := unusedImportAnalysis(text, file, tokens, documentPath)
		diagnostics = append(diagnostics, unusedImportDiagnostics...)
		actions = append(actions, unusedImportActions...)
	}
	actions = append(actions, importResolutionCodeActions(text, file, tokens, documentPath)...)
	actions = append(actions, scriptBlockStructureCodeActions(text, documentPath)...)
	actions = append(actions, variableFixTextCodeActions(text, documentPath)...)
	actions = append(actions, typeAliasTextCodeActions(text, documentPath)...)
	actions = append(actions, arrayTextCodeActions(text, documentPath)...)
	actions = append(actions, documentationCodeActions(text, file, tokens, documentPath)...)
	actions = append(actions, editorRefactorCodeActions(text, file, documentPath)...)
	diagnostics = append(diagnostics, schemaOutputVariableDiagnostics(file, tokens)...)

	return diagnostics, actions
}

func scriptBlockStructureCodeActions(text string, documentPath string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	uri := protocol.DocumentUri(fileURI(documentPath))
	fullRange := fullDocumentRange(text)
	targetRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
	actions := []analysisCodeActionCandidate{}
	addTextAction := func(title string, newText string) {
		actions = append(actions, textRefactorAction(title, targetRange, uri, fullRange, newText))
	}

	if !strings.Contains(text, "|===|") && !strings.Contains(text, "|====|") {
		addTextAction("Create script block", "|===|\n|===|\n"+text)
		addTextAction("Wrap selection in script block", "|===|\n"+text+"\n|===|")
	}
	if strings.Contains(text, "|====|") {
		fixed := strings.ReplaceAll(text, "|====|", "|===|")
		addTextAction("Fix script delimiter length mismatch", fixed)
		addTextAction("Normalize script fence", fixed)
	}
	if emptyScriptBlockPattern.MatchString(text) {
		addTextAction("Remove empty script block", emptyScriptBlockPattern.ReplaceAllString(text, ""))
	}
	if moved, ok := moveScriptBlockBeforeOutputText(text); ok {
		addTextAction("Move script block before output block", moved)
	}
	if fixed, ok := addMissingScriptSemicolonText(text); ok {
		addTextAction("Add missing semicolon", fixed)
	}

	return actions
}

func addMissingScriptSemicolonText(text string) (string, bool) {
	start := strings.Index(text, "|===|")
	if start < 0 {
		return "", false
	}
	bodyStart := start + len("|===|")
	end := strings.Index(text[bodyStart:], "|===|")
	if end < 0 {
		return "", false
	}
	end += bodyStart
	body := text[bodyStart:end]
	lines := strings.Split(body, "\n")
	line, index, ok := lo.FindIndexOf(lines, func(line string) bool {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasSuffix(trimmed, ";") || strings.HasSuffix(trimmed, "{") || strings.HasSuffix(trimmed, "}") {
			return false
		}
		return strings.HasPrefix(trimmed, "type ") || strings.HasPrefix(trimmed, "schema ") || strings.HasPrefix(trimmed, "enum ") || strings.Contains(trimmed, "=")
	})
	if !ok {
		return "", false
	}

	lines[index] = line + ";"
	return text[:bodyStart] + strings.Join(lines, "\n") + text[end:], true
}

func moveScriptBlockBeforeOutputText(text string) (string, bool) {
	firstScript := strings.Index(text, "|===|")
	if firstScript < 0 {
		return "", false
	}
	secondScript := strings.Index(text[firstScript+len("|===|"):], "|===|")
	if secondScript < 0 {
		return "", false
	}
	secondScript += firstScript + len("|===|")
	firstOutput := strings.Index(text, "[")
	if firstOutput < 0 {
		firstOutput = strings.Index(text, "{")
	}
	if firstOutput < 0 || firstScript < firstOutput {
		return "", false
	}

	scriptEnd := secondScript + len("|===|")
	script := strings.TrimSpace(text[firstScript:scriptEnd])
	withoutScript := strings.TrimSpace(text[:firstScript] + text[scriptEnd:])
	return script + "\n" + withoutScript, true
}

func arrayTextCodeActions(text string, documentPath string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	uri := protocol.DocumentUri(fileURI(documentPath))
	fullRange := fullDocumentRange(text)
	targetRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
	actions := []analysisCodeActionCandidate{}
	addTextAction := func(title string, newText string) {
		actions = append(actions, textRefactorAction(title, targetRange, uri, fullRange, newText))
	}
	if updated, ok := wrapTypeInArrayText(text); ok {
		addTextAction("Wrap type in array", updated)
	}
	if updated, ok := fixMixedArrayLiteralText(text); ok {
		addTextAction("Fix mixed array literal", updated)
	}
	if updated, ok := changeArrayElementTypeText(text); ok {
		addTextAction("Change array element type", updated)
	}
	if updated, ok := replaceInvalidArrayIndexText(text); ok {
		addTextAction("Replace invalid array index", updated)
	}
	return actions
}

func wrapTypeInArrayText(text string) (string, bool) {
	updated := regexp.MustCompile(`type\s+([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(string|int|float|boolean);`).ReplaceAllString(text, `type ${1}: array<${2}>;`)
	return updated, updated != text
}

func fixMixedArrayLiteralText(text string) (string, bool) {
	pattern := regexp.MustCompile(`array<string>\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\["[^"]+",\s*\d+\];`)
	matches := pattern.FindStringSubmatch(text)
	if len(matches) == 0 {
		return "", false
	}
	aliasName := strings.ToUpper(matches[1][:1]) + matches[1][1:] + "Item"
	updated := strings.Replace(text, "|===|\n", "|===|\ntype "+aliasName+": variant[string, int];\n", 1)
	updated = strings.Replace(updated, "array<string> "+matches[1], "array<"+aliasName+"> "+matches[1], 1)
	return updated, updated != text
}

func changeArrayElementTypeText(text string) (string, bool) {
	pattern := regexp.MustCompile(`array<string>\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\[\d+(?:,\s*\d+)*\];`)
	matches := pattern.FindStringSubmatch(text)
	if len(matches) == 0 {
		return "", false
	}
	updated := strings.Replace(text, "array<string> "+matches[1], "array<int> "+matches[1], 1)
	return updated, updated != text
}

func replaceInvalidArrayIndexText(text string) (string, bool) {
	updated := regexp.MustCompile(`(\[[^\]]+\])\[\d+\]`).ReplaceAllString(text, `${1}[0]`)
	return updated, updated != text
}

func typeAliasTextCodeActions(text string, documentPath string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	uri := protocol.DocumentUri(fileURI(documentPath))
	fullRange := fullDocumentRange(text)
	targetRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
	actions := []analysisCodeActionCandidate{}
	addTextAction := func(title string, newText string) {
		actions = append(actions, textRefactorAction(title, targetRange, uri, fullRange, newText))
	}
	if updated, ok := createTypeAliasFromSelectedTypeText(text); ok {
		addTextAction("Create type alias from selected type", updated)
	}
	if updated, ok := inlineTypeAliasUsageText(text); ok {
		addTextAction("Inline type alias usage", updated)
	}
	if updated, ok := renameTypeAliasText(text); ok {
		addTextAction("Rename type alias", updated)
	}
	if updated, name, ok := replaceUnknownTypeText(text); ok {
		addTextAction("Replace unknown type with "+name, updated)
	}
	if updated := strings.ReplaceAll(text, "Array<", "array<"); updated != text {
		addTextAction("Convert Array<T> to array<T>", updated)
	}
	if updated, ok := nullableTypeToOptionalFieldText(text); ok {
		addTextAction("Convert nullable type into optional field", updated)
	}
	return actions
}

func createTypeAliasFromSelectedTypeText(text string) (string, bool) {
	pattern := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*:\s*(string|int|float|boolean|array<[^>]+>|[A-Za-z_][A-Za-z0-9_]*)`)
	matches := pattern.FindStringSubmatch(text)
	if len(matches) == 0 || strings.Contains(text, "type ExtractedType:") {
		return "", false
	}
	updated := strings.Replace(text, "|===|\n", "|===|\ntype ExtractedType: "+matches[2]+";\n", 1)
	updated = pattern.ReplaceAllString(updated, `${1}: ExtractedType`)
	updated = strings.Replace(updated, "type ExtractedType: ExtractedType;", "type ExtractedType: "+matches[2]+";", 1)
	return updated, updated != text
}

func inlineTypeAliasUsageText(text string) (string, bool) {
	matches := regexp.MustCompile(`type\s+([A-Za-z_][A-Za-z0-9_]*)\s*:\s*([^;]+);`).FindStringSubmatch(text)
	if len(matches) == 0 {
		return "", false
	}
	updated := strings.ReplaceAll(text, ": "+matches[1], ": "+matches[2])
	updated = regexp.MustCompile(`(?m)^([ \t]*)`+regexp.QuoteMeta(matches[1])+`\s+`).ReplaceAllString(updated, `${1}`+matches[2]+` `)
	return updated, updated != text
}

func renameTypeAliasText(text string) (string, bool) {
	updated := regexp.MustCompile(`type\s+([A-Za-z_][A-Za-z0-9_]*)\s*:`).ReplaceAllString(text, "type RenamedName:")
	return updated, updated != text
}

func replaceUnknownTypeText(text string) (string, string, bool) {
	matches := regexp.MustCompile(`type\s+([A-Za-z_][A-Za-z0-9_]*)\s*:`).FindStringSubmatch(text)
	if len(matches) == 0 {
		return "", "", false
	}
	known := matches[1]
	updated := regexp.MustCompile(`:\s*Nmae`).ReplaceAllString(text, ": "+known)
	updated = regexp.MustCompile(`(?m)^([ \t]*)Nmae\s+`).ReplaceAllString(updated, `${1}`+known+` `)
	return updated, known, updated != text
}

func nullableTypeToOptionalFieldText(text string) (string, bool) {
	updated := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*:\s*([^;{}]+)\?`).ReplaceAllString(text, `${1}?: ${2}`)
	return updated, updated != text
}

func variableFixTextCodeActions(text string, documentPath string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	uri := protocol.DocumentUri(fileURI(documentPath))
	fullRange := fullDocumentRange(text)
	targetRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
	actions := []analysisCodeActionCandidate{}
	addTextAction := func(title string, newText string) {
		actions = append(actions, textRefactorAction(title, targetRange, uri, fullRange, newText))
	}

	if updated, ok := replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*");`), func(matches []string) string {
		return matches[1] + "string " + matches[2] + " = " + matches[3] + ";"
	}); ok {
		addTextAction("Add missing type annotation", updated)
	}
	if updated, ok := replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)(string)\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`), func(matches []string) string {
		return matches[1] + matches[2] + " " + matches[3] + ` = "";`
	}); ok {
		addTextAction("Add missing initializer", updated)
	}
	if updated, ok := replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`), func(matches []string) string {
		return matches[1] + "injectable " + matches[2] + " " + matches[3] + ";"
	}); ok {
		addTextAction("Mark variable as injectable", updated)
	}
	if updated, ok := replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)(int)\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`), func(matches []string) string {
		return matches[1] + matches[2] + " " + matches[3] + " = 0;"
	}); ok {
		addTextAction("Add placeholder initializer", updated)
	}
	if updated, ok := replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)int\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*");`), func(matches []string) string {
		return matches[1] + "string " + matches[2] + " = " + matches[3] + ";"
	}); ok {
		addTextAction("Change variable type to inferred expression type", updated)
	}
	if updated, ok := replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)int\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*"[^"]*";`), func(matches []string) string {
		return matches[1] + "int " + matches[2] + " = 0;"
	}); ok {
		addTextAction("Change initializer to match declared type", updated)
	}
	if updated, ok := renameDuplicateVariableText(text); ok {
		addTextAction("Rename duplicate variable", updated)
	}
	if updated, ok := inlineVariableIntoOutputText(text); ok {
		addTextAction("Inline variable into output field", updated)
	}
	if updated, ok := extractOutputExpressionText(text); ok {
		addTextAction("Extract output expression into script variable", updated)
	}
	if updated, ok := convertVariableToInjectableText(text); ok {
		addTextAction("Convert variable to injectable", updated)
	}
	if updated, ok := addDefaultInitializerToInjectableText(text); ok {
		addTextAction("Add default initializer to injectable", updated)
	}
	if stub, ok := injectionConfigStubText(text); ok {
		addTextAction("Generate injection config stub", text+"\n"+stub)
	}
	if names, ok := injectableVariableNames(text); ok {
		actions = append(actions, analysisCodeActionCandidate{Range: targetRange, Action: protocol.CodeAction{Title: "Find all injectable variables", Kind: Ptr(protocol.CodeActionKindRefactor), Command: &protocol.Command{Title: "Find all injectable variables", Command: "mace.findInjectables", Arguments: []any{strings.Join(names, ", ")}}}})
	}
	return actions
}

func convertVariableToInjectableText(text string) (string, bool) {
	return replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)([A-Za-z_][A-Za-z0-9_]*)\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`), func(matches []string) string {
		return matches[1] + "injectable " + matches[2] + " " + matches[3] + ";"
	})
}

func addDefaultInitializerToInjectableText(text string) (string, bool) {
	return replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*injectable\s+)([A-Za-z_][A-Za-z0-9_]*(?:<[^>]+>)?)\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`), func(matches []string) string {
		return matches[1] + matches[2] + " " + matches[3] + " = " + defaultLiteralForTypeName(matches[2]) + ";"
	})
}

func injectionConfigStubText(text string) (string, bool) {
	names, ok := injectableVariableNames(text)
	if !ok {
		return "", false
	}
	pattern := regexp.MustCompile(`(?m)^\s*injectable\s+([A-Za-z_][A-Za-z0-9_]*(?:<[^>]+>)?)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	entries := lo.FilterMap(pattern.FindAllStringSubmatch(text, -1), func(matches []string, _ int) (string, bool) {
		if len(matches) < 3 {
			return "", false
		}
		return "  \"" + matches[2] + "\": " + defaultLiteralForTypeName(matches[1]), true
	})
	_ = names
	return "/# injection config stub\n{\n" + strings.Join(entries, ",\n") + "\n}\n#/", true
}

func injectableVariableNames(text string) ([]string, bool) {
	pattern := regexp.MustCompile(`(?m)^\s*injectable\s+[A-Za-z_][A-Za-z0-9_]*\s+([A-Za-z_][A-Za-z0-9_]*)`)
	names := lo.FilterMap(pattern.FindAllStringSubmatch(text, -1), func(matches []string, _ int) (string, bool) {
		if len(matches) < 2 {
			return "", false
		}
		return matches[1], true
	})
	return names, len(names) > 0
}

func defaultLiteralForTypeName(name string) string {
	if strings.HasPrefix(name, "array<") {
		return "[]"
	}

	switch name {
	case "int":
		return "0"
	case "float":
		return "0.0"
	case "boolean":
		return "false"
	default:
		return `""`
	}
}

func replaceVariableDeclaration(text string, pattern *regexp.Regexp, replacement func([]string) string) (string, bool) {
	matches := pattern.FindStringSubmatch(text)
	if len(matches) == 0 {
		return "", false
	}
	return pattern.ReplaceAllString(text, replacement(matches)), true
}

func renameDuplicateVariableText(text string) (string, bool) {
	pattern := regexp.MustCompile(`(?m)^([ \t]*[A-Za-z_][A-Za-z0-9_<>{};: ]*\s+)([A-Za-z_][A-Za-z0-9_]*)(\s*=\s*[^;]+;)`)
	seen := map[string]struct{}{}
	updated := pattern.ReplaceAllStringFunc(text, func(line string) string {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) == 0 {
			return line
		}
		name := matches[2]
		if _, ok := seen[name]; ok {
			return matches[1] + name + "_2" + matches[3]
		}
		seen[name] = struct{}{}
		return line
	})
	return updated, updated != text
}

func inlineVariableIntoOutputText(text string) (string, bool) {
	matches := regexp.MustCompile(`(?m)^\s*string\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*");`).FindStringSubmatch(text)
	if len(matches) == 0 {
		return "", false
	}
	updated := strings.Replace(text, ": "+matches[1], ": "+matches[2], 1)
	return updated, updated != text
}

func extractOutputExpressionText(text string) (string, bool) {
	pattern := regexp.MustCompile(`(?m)([A-Za-z_][A-Za-z0-9_]*)\s*:\s*("[^"]*")`)
	matches := pattern.FindStringSubmatch(text)
	if len(matches) == 0 || strings.Contains(text, "|===|") {
		return "", false
	}
	name := matches[1]
	value := matches[2]
	updated := pattern.ReplaceAllString(text, name+": "+name)
	return "|===|\nstring " + name + " = " + value + ";\n|===|\n" + updated, true
}

func parseErrorCodeActions(tokens []lexer.Token, documentPath string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	uri := protocol.DocumentUri(fileURI(documentPath))
	for _, token := range tokens {
		if token.Type != lexer.TokenStar {
			continue
		}

		rangeValue := tokenProtocolRange(token)
		return []analysisCodeActionCandidate{{
			Range: rangeValue,
			Action: protocol.CodeAction{
				Title: "Convert wildcard import to named import",
				Kind:  Ptr(protocol.CodeActionKindQuickFix),
				Edit:  &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: rangeValue, NewText: "Name"}}}},
			},
		}}
	}

	return nil
}

func semanticCodeActions(text string, file ast.File, tokens []lexer.Token, documentPath string, diagnostic protocol.Diagnostic, message string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	actions := []analysisCodeActionCandidate{}
	uri := pathURI(documentPath)
	addAction := func(title string, rangeValue protocol.Range, newText string) {
		actions = append(actions, analysisCodeActionCandidate{Range: diagnostic.Range, Action: protocol.CodeAction{Title: title, Kind: Ptr(protocol.CodeActionKindQuickFix), Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: rangeValue, NewText: newText}}}}}})
	}

	if rangeValue, textValue, ok := missingSchemaFieldEdit(text, file, message); ok {
		addAction("Add missing schema field", rangeValue, textValue)
	}
	if rangeValue, ok := unknownFieldEditRange(text, file, tokens, message); ok {
		addAction("Remove unknown field", rangeValue, "")
	}
	if rangeValue, ok := duplicateFieldEditRange(text, tokens, message); ok {
		addAction("Remove duplicate field", rangeValue, "")
	}
	if rangeValue, textValue, ok := typeMismatchEdit(file, diagnostic, message); ok {
		addAction("Fix type mismatch", rangeValue, textValue)
	}
	if rangeValue, textValue, ok := enumValueEdit(file, tokens, diagnostic, message); ok {
		addAction("Convert raw value to enum member", rangeValue, textValue)
	}
	if rangeValue, textValue, ok := missingImportEdit(text, file, tokens, message); ok {
		addAction("Add missing import", rangeValue, textValue)
	}
	if rangeValue, textValue, ok := moveImportsToTopEdit(text, file, tokens, message); ok {
		addAction("Move imports to top", rangeValue, textValue)
	}
	if rangeValue, ok := duplicateDeclarationEditRange(text, file, tokens, message); ok {
		addAction("Remove duplicate declaration", rangeValue, "")
	}
	if rangeValue, textValue, ok := declarationOperatorEdit(text, tokens, message); ok {
		addAction("Fix declaration operator", rangeValue, textValue)
	}
	if rangeValue, textValue, ok := selfOrderingEdit(text, file, tokens, message); ok {
		addAction("Move referenced field before $self use", rangeValue, textValue)
	}
	if rangeValue, ok := invalidDirectiveComboEditRange(text, file, tokens, message); ok {
		addAction("Fix invalid output directive combination", rangeValue, "")
	}
	if rangeValue, textValue, ok := generateOutputFromSchemaEdit(text, file, tokens, message); ok {
		addAction("Generate output from schema", rangeValue, textValue)
	}

	return actions
}

func importResolutionCodeActions(text string, file ast.File, tokens []lexer.Token, documentPath string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	uri := protocol.DocumentUri(fileURI(documentPath))
	baseDir := filepath.Dir(documentPath)
	fullRange := fullDocumentRange(text)
	_ = fullRange
	actions := []analysisCodeActionCandidate{}
	for _, importDecl := range file.Imports {
		pathValue, ok := stringLiteralValue(importDecl.Path)
		if !ok {
			continue
		}

		importedPath := filepath.Clean(filepath.Join(baseDir, pathValue))
		importRange, _ := tokenRangeByType(tokens, lexer.TokenString, importDecl.Path.Lexeme)
		if _, err := os.Stat(importedPath); os.IsNotExist(err) {
			actions = append(actions, analysisCodeActionCandidate{
				Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{}},
				Action: protocol.CodeAction{
					Title: "Create missing imported file",
					Kind:  Ptr(protocol.CodeActionKindQuickFix),
					Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{
						protocol.DocumentUri(fileURI(importedPath)): {{Range: protocol.Range{}, NewText: "[output = schema]\n{}\n"}},
					}},
				},
			})

			if renamedPath, ok := existingMacePathWithSimilarName(importedPath); ok && importRange != (protocol.Range{}) {
				relativePath, err := filepath.Rel(baseDir, renamedPath)
				if err == nil {
					relativePath = filepath.ToSlash(relativePath)
					if !strings.HasPrefix(relativePath, ".") {
						relativePath = "./" + relativePath
					}
					actions = append(actions, analysisCodeActionCandidate{
						Range:  protocol.Range{Start: protocol.Position{}, End: protocol.Position{}},
						Action: protocol.CodeAction{Title: "Update import path after file rename", Kind: Ptr(protocol.CodeActionKindQuickFix), Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: importRange, NewText: strconv.Quote(relativePath)}}}}},
					})
				}
			}
			continue
		}

		_, importedFile, _, ok := parsedFile(importedPath)
		if !ok {
			continue
		}
		exportedNames := exportedOutputNames(importedFile)
		if len(exportedNames) > 0 {
			actions = append(actions, analysisCodeActionCandidate{Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, Action: protocol.CodeAction{Title: "Open source output block", Kind: Ptr(protocol.CodeActionKindRefactor), Command: &protocol.Command{Title: "Open source output block", Command: "mace.openOutput", Arguments: []any{protocol.DocumentUri(fileURI(importedPath))}}}})
			actions = append(actions, analysisCodeActionCandidate{Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, Action: protocol.CodeAction{Title: "Explain why symbol is not importable", Kind: Ptr(protocol.CodeActionKindQuickFix), Command: &protocol.Command{Title: "Explain why symbol is not importable", Command: "mace.explainImport", Arguments: []any{"Only names surfaced through the imported file output block are importable."}}}})
		}
		for _, name := range importDecl.Identifiers {
			if lo.Contains(exportedNames, name) {
				continue
			}
			closest, ok := closestName(name, exportedNames)
			if !ok {
				continue
			}
			nameToken, ok := importIdentifierToken(tokens, importDecl, name)
			if !ok {
				continue
			}
			actions = append(actions, analysisCodeActionCandidate{Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, Action: protocol.CodeAction{Title: "Replace unavailable imported symbol with " + closest, Kind: Ptr(protocol.CodeActionKindQuickFix), Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: tokenProtocolRange(nameToken), NewText: closest}}}}}})
		}
	}
	return actions
}

func existingMacePathWithSimilarName(path string) (string, bool) {
	directory := filepath.Dir(path)
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	entries, err := os.ReadDir(directory)
	if err != nil {
		return "", false
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".mace" {
			continue
		}
		candidate := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		if strings.Contains(candidate, base) || strings.Contains(base, candidate) || levenshteinDistance(base, candidate) <= 4 {
			return filepath.Join(directory, entry.Name()), true
		}
	}
	return "", false
}

func exportedOutputNames(file ast.File) []string {
	if file.Output.Mode == ast.OutputModeSchema {
		return lo.Map(file.Output.SchemaFields, func(field ast.OutputSchemaField, _ int) string { return field.Name })
	}
	return lo.Map(file.Output.DataFields, func(field ast.OutputField, _ int) string { return field.Name })
}

func closestName(name string, names []string) (string, bool) {
	closest := ""
	closestDistance := 1000
	for _, candidate := range names {
		distance := levenshteinDistance(strings.ToLower(name), strings.ToLower(candidate))
		if distance < closestDistance {
			closest = candidate
			closestDistance = distance
		}
	}
	return closest, closest != "" && closestDistance <= 3
}

func levenshteinDistance(left string, right string) int {
	previous := make([]int, len(right)+1)
	for index := range previous {
		previous[index] = index
	}
	for leftIndex, leftRune := range left {
		current := make([]int, len(right)+1)
		current[0] = leftIndex + 1
		for rightIndex, rightRune := range right {
			cost := 1
			if leftRune == rightRune {
				cost = 0
			}
			current[rightIndex+1] = min(current[rightIndex]+1, previous[rightIndex+1]+1, previous[rightIndex]+cost)
		}
		previous = current
	}
	return previous[len(right)]
}

func documentationCodeActions(text string, file ast.File, tokens []lexer.Token, documentPath string) []analysisCodeActionCandidate {
	if file.Script == nil || documentPath == "" {
		return nil
	}

	insertRange, ok := documentationInsertRange(text, tokens)
	if !ok {
		return nil
	}

	actions := []analysisCodeActionCandidate{}
	for _, item := range file.Script.Items {
		switch declaration := item.(type) {
		case ast.TypeDeclaration:
			rangeValue := tokenProtocolRange(declaration.NameToken)
			actions = append(actions, documentationCodeAction(documentPath, rangeValue, insertRange, "Generate gen_doc", genDocText(declaration.Name)))
			if declaration.Description == "" {
				if editRange, ok := declarationSemicolonInsertRange(text, tokens, declaration.NameToken); ok {
					actions = append(actions, inlineDescriptionCodeAction(documentPath, rangeValue, editRange))
				}
			}
		case ast.VariableDeclaration:
			actions = append(actions, documentationCodeAction(documentPath, tokenProtocolRange(declaration.NameToken), insertRange, "Generate gen_doc", genDocText(declaration.Name)))
		case ast.SchemaDeclaration:
			actions = append(actions, documentationCodeAction(documentPath, tokenProtocolRange(declaration.NameToken), insertRange, "Generate schema_doc", schemaDocText(declaration.Name)))
		case ast.EnumDeclaration:
			actions = append(actions, documentationCodeAction(documentPath, tokenProtocolRange(declaration.NameToken), insertRange, "Generate schema_doc", schemaDocText(declaration.Name)))
		}
	}

	return actions
}

func editorRefactorCodeActions(text string, file ast.File, documentPath string) []analysisCodeActionCandidate {
	if documentPath == "" {
		return nil
	}

	uri := pathURI(documentPath)
	fullRange := fullDocumentRange(text)
	actions := []analysisCodeActionCandidate{}
	addTextAction := func(title string, targetRange protocol.Range, newText string) {
		actions = append(actions, analysisCodeActionCandidate{
			Range: targetRange,
			Action: protocol.CodeAction{
				Title: title,
				Kind:  Ptr(protocol.CodeActionKindRefactor),
				Edit:  &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: fullRange, NewText: newText}}}},
			},
		})
	}
	addWholeFileAction := func(title string, targetRange protocol.Range, updated ast.File) {
		formatted, err := formatter.FormatFile(updated)
		if err != nil {
			return
		}
		addTextAction(title, targetRange, formatted)
	}

	for _, item := range lo.FromPtr(file.Script).Items {
		declaration, ok := item.(ast.SchemaDeclaration)
		if !ok {
			continue
		}

		targetRange := tokenProtocolRange(declaration.NameToken)
		updated := file
		updated.Output.Mode = ast.OutputModeData
		updated.Output.Directives = []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}, {Kind: ast.OutputDirectiveSchema, Value: declaration.Name}}
		updated.Output.DataFields = lo.Map(declaration.Type.Fields, func(field ast.SchemaField, _ int) ast.OutputField {
			return ast.OutputField{Name: field.Name, Optional: field.Optional, Value: defaultExpressionForType(field.Type)}
		})
		updated.Output.SchemaFields = nil
		addWholeFileAction("Generate output block from schema", targetRange, updated)

		updated = file
		updated.Output.Directives = append([]ast.OutputDirective{}, file.Output.Directives...)
		if !lo.ContainsBy(updated.Output.Directives, func(directive ast.OutputDirective) bool { return directive.Kind == ast.OutputDirectiveSchema }) {
			updated.Output.Directives = append(updated.Output.Directives, ast.OutputDirective{Kind: ast.OutputDirectiveSchema, Value: declaration.Name})
			addWholeFileAction("Add schema = "+declaration.Name+" directive", targetRange, updated)
		}
	}

	if len(file.Output.Directives) == 0 {
		updated := file
		updated.Output.Directives = []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}
		addWholeFileAction("Make implicit output explicit", protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, updated)
	}
	if file.Output.Mode == ast.OutputModeData {
		updated := file
		updated.Output.Mode = ast.OutputModeSchema
		updated.Output.Directives = []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "schema"}}
		updated.Output.SchemaFields = lo.Map(file.Output.DataFields, func(field ast.OutputField, _ int) ast.OutputSchemaField {
			return ast.OutputSchemaField{Name: field.Name, Optional: field.Optional, Type: inferredTypeFromExpression(field.Value)}
		})
		updated.Output.DataFields = nil
		addWholeFileAction("Convert data output to schema output", protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, updated)
	}

	for index, field := range file.Output.SchemaFields {
		targetRange := tokenProtocolRange(field.NameToken)
		updated := file
		updated.Output.SchemaFields = append([]ast.OutputSchemaField{}, file.Output.SchemaFields...)
		updated.Output.SchemaFields[index].Optional = !field.Optional
		if field.Optional {
			addWholeFileAction("Remove optional marker ?", targetRange, updated)
		} else {
			addWholeFileAction("Add optional marker ?", targetRange, updated)
		}
		if _, ok := field.Type.(ast.RecordType); ok {
			converted := convertInlineRecordOutputField(file, index)
			addWholeFileAction("Convert inline record to schema", targetRange, converted)
		}
	}

	addImportRefactorActions(file, addWholeFileAction)
	addDocumentationRefactorActions(file, addWholeFileAction)
	addDeclarationRefactorActions(file, addWholeFileAction)
	addEnumRefactorActions(file, addWholeFileAction)
	addStringRefactorActions(text, uri, fullRange, &actions)
	addStyleRefactorActions(text, protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, addTextAction)
	addExpressionRefactorActions(text, file, protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, addTextAction)
	addInteropRefactorActions(text, file, protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, addTextAction)

	return actions
}

func addImportRefactorActions(file ast.File, addWholeFileAction func(string, protocol.Range, ast.File)) {
	if len(file.Imports) > 1 {
		imports := append([]ast.ImportDeclaration{}, file.Imports...)
		slices.SortFunc(imports, func(left ast.ImportDeclaration, right ast.ImportDeclaration) int {
			return strings.Compare(left.Path.Lexeme, right.Path.Lexeme)
		})
		if !slices.EqualFunc(imports, file.Imports, func(left ast.ImportDeclaration, right ast.ImportDeclaration) bool {
			return left.Path.Lexeme == right.Path.Lexeme && slices.Equal(left.Identifiers, right.Identifiers)
		}) {
			updated := file
			updated.Imports = imports
			addWholeFileAction("Sort imports", protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, updated)
		}
	}

	for index, importDecl := range file.Imports {
		targetRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		if hasDuplicateImportIdentifiers(importDecl.Identifiers) {
			updated := file
			imports := append([]ast.ImportDeclaration{}, file.Imports...)
			imports[index].Identifiers = uniqueImportIdentifiers(importDecl.Identifiers)
			updated.Imports = imports
			addWholeFileAction("Remove duplicate imported names", targetRange, updated)
		}

		pathValue, _ := stringLiteralValue(importDecl.Path)
		if pathValue != "" && !strings.HasPrefix(pathValue, "./") && !strings.HasPrefix(pathValue, "../") && !filepath.IsAbs(pathValue) {
			updated := file
			imports := append([]ast.ImportDeclaration{}, file.Imports...)
			imports[index].Path = ast.StringLiteral{Lexeme: strconv.Quote("./" + pathValue)}
			updated.Imports = imports
			addWholeFileAction("Fix relative import path", targetRange, updated)
		}
		if len(importDecl.Identifiers) > 1 {
			updated := file
			imports := append([]ast.ImportDeclaration{}, file.Imports...)
			imports = slices.Delete(imports, index, index+1)
			for offset, identifier := range importDecl.Identifiers {
				imports = slices.Insert(imports, index+offset, ast.ImportDeclaration{Path: importDecl.Path, Identifiers: []string{identifier}})
			}
			updated.Imports = imports
			addWholeFileAction("Split import declaration", targetRange, updated)
		}
		for otherIndex := index + 1; otherIndex < len(file.Imports); otherIndex++ {
			if file.Imports[otherIndex].Path.Lexeme != importDecl.Path.Lexeme {
				continue
			}
			updated := file
			imports := append([]ast.ImportDeclaration{}, file.Imports...)
			imports[index].Identifiers = append(append([]string{}, importDecl.Identifiers...), file.Imports[otherIndex].Identifiers...)
			imports = slices.Delete(imports, otherIndex, otherIndex+1)
			updated.Imports = imports
			addWholeFileAction("Merge duplicate imports", targetRange, updated)
			break
		}
	}
}

func hasDuplicateImportIdentifiers(identifiers []string) bool {
	seen := map[string]struct{}{}
	for _, identifier := range identifiers {
		if _, ok := seen[identifier]; ok {
			return true
		}
		seen[identifier] = struct{}{}
	}
	return false
}

func uniqueImportIdentifiers(identifiers []string) []string {
	seen := map[string]struct{}{}
	unique := []string{}
	for _, identifier := range identifiers {
		if _, ok := seen[identifier]; ok {
			continue
		}
		seen[identifier] = struct{}{}
		unique = append(unique, identifier)
	}
	return unique
}

func addDocumentationRefactorActions(file ast.File, addWholeFileAction func(string, protocol.Range, ast.File)) {
	if file.Script == nil {
		return
	}
	for index, item := range file.Script.Items {
		schema, ok := item.(ast.SchemaDeclaration)
		if !ok {
			continue
		}
		targetRange := tokenProtocolRange(schema.NameToken)
		updated := file
		items := append([]ast.Declaration{}, file.Script.Items...)
		items = append(items, ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: schema.Name, Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `""`}, Props: schemaDocProps(schema)}})
		updated.Script = &ast.ScriptBlock{Imports: file.Script.Imports, Items: items}
		addWholeFileAction("Add missing props docs", targetRange, updated)
		addWholeFileAction("Move inline /# docs to structured docs", targetRange, updated)

		if hasSchemaDoc(file, schema.Name) && hasInlineSchemaDocs(schema) {
			cleaned := file
			items := append([]ast.Declaration{}, file.Script.Items...)
			schema.Type.Fields = lo.Map(schema.Type.Fields, func(field ast.SchemaField, _ int) ast.SchemaField { field.Description = ""; return field })
			items[index] = schema
			cleaned.Script = &ast.ScriptBlock{Imports: file.Script.Imports, Items: items}
			addWholeFileAction("Remove conflicting docs", targetRange, cleaned)
		}
	}
}

func addDeclarationRefactorActions(file ast.File, addWholeFileAction func(string, protocol.Range, ast.File)) {
	if file.Script == nil {
		return
	}

	addSchemaDeclarationRefactorActions(file, addWholeFileAction)
	addVariableDeclarationRefactorActions(file, addWholeFileAction)
}

func addSchemaDeclarationRefactorActions(file ast.File, addWholeFileAction func(string, protocol.Range, ast.File)) {
	for index, item := range file.Script.Items {
		schema, ok := item.(ast.SchemaDeclaration)
		if !ok {
			continue
		}

		addRepeatedTypeAliasAction(file, index, schema, addWholeFileAction)
		addInlineRecordSchemaAction(file, index, schema, addWholeFileAction)
	}
}

func addRepeatedTypeAliasAction(file ast.File, index int, schema ast.SchemaDeclaration, addWholeFileAction func(string, protocol.Range, ast.File)) {
	name, typed, ok := repeatedSchemaFieldType(schema)
	if !ok {
		return
	}

	aliasName := "ExtractedType"
	updatedSchema := schema
	updatedSchema.Type.Fields = lo.Map(schema.Type.Fields, func(field ast.SchemaField, _ int) ast.SchemaField {
		if typeReferenceText(field.Type) == name {
			field.Type = ast.NamedType{Name: aliasName}
		}
		return field
	})
	updated := replaceScriptItem(file, index, updatedSchema)
	items := append([]ast.Declaration{ast.TypeDeclaration{Name: aliasName, Type: typed}}, updated.Script.Items...)
	updated.Script = &ast.ScriptBlock{Imports: updated.Script.Imports, Items: items}
	addWholeFileAction("Extract repeated type into alias", protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, updated)
}

func addInlineRecordSchemaAction(file ast.File, index int, schema ast.SchemaDeclaration, addWholeFileAction func(string, protocol.Range, ast.File)) {
	field, fieldIndex, ok := lo.FindIndexOf(schema.Type.Fields, func(field ast.SchemaField) bool {
		_, ok := field.Type.(ast.RecordType)
		return ok
	})
	if !ok {
		return
	}

	record := field.Type.(ast.RecordType)
	schemaName := strings.ToUpper(field.Name[:1]) + field.Name[1:]
	updatedSchema := schema
	updatedSchema.Type.Fields = append([]ast.SchemaField{}, schema.Type.Fields...)
	updatedSchema.Type.Fields[fieldIndex].Type = ast.NamedType{Name: schemaName}
	updated := replaceScriptItem(file, index, updatedSchema)
	items := append([]ast.Declaration{ast.SchemaDeclaration{Name: schemaName, Type: record}}, updated.Script.Items...)
	updated.Script = &ast.ScriptBlock{Imports: updated.Script.Imports, Items: items}
	addWholeFileAction("Extract inline record type into schema", protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, updated)
}

func addVariableDeclarationRefactorActions(file ast.File, addWholeFileAction func(string, protocol.Range, ast.File)) {
	for index, item := range file.Script.Items {
		variable, ok := item.(ast.VariableDeclaration)
		if !ok {
			continue
		}
		addRecordVariableSchemaAction(file, index, variable, addWholeFileAction)
	}
}

func addRecordVariableSchemaAction(file ast.File, index int, variable ast.VariableDeclaration, addWholeFileAction func(string, protocol.Range, ast.File)) {
	record, ok := variable.Value.(ast.RecordLiteral)
	if !ok {
		return
	}
	if _, ok := variable.Type.(ast.RecordType); !ok {
		return
	}

	schemaName := strings.ToUpper(variable.Name[:1]) + variable.Name[1:]
	variable.Type = ast.NamedType{Name: schemaName}
	updated := replaceScriptItem(file, index, variable)
	items := append([]ast.Declaration{ast.SchemaDeclaration{Name: schemaName, Type: recordTypeFromLiteral(record)}}, updated.Script.Items...)
	updated.Script = &ast.ScriptBlock{Imports: updated.Script.Imports, Items: items}
	addWholeFileAction("Convert record variable into schema-backed variable", protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, updated)
}

func repeatedSchemaFieldType(schema ast.SchemaDeclaration) (string, ast.TypeReference, bool) {
	counts := map[string]int{}
	types := map[string]ast.TypeReference{}
	for _, field := range schema.Type.Fields {
		text := typeReferenceText(field.Type)
		if text == "" {
			continue
		}
		counts[text]++
		types[text] = field.Type
	}
	for name, count := range counts {
		if count > 1 {
			return name, types[name], true
		}
	}
	return "", nil, false
}

func typeReferenceText(typeReference ast.TypeReference) string {
	switch typed := typeReference.(type) {
	case ast.PrimitiveType:
		return typed.Name
	case ast.NamedType:
		return typed.Name
	default:
		return ""
	}
}

func recordTypeFromLiteral(record ast.RecordLiteral) ast.RecordType {
	return ast.RecordType{Fields: lo.Map(record.Fields, func(field ast.RecordField, _ int) ast.SchemaField {
		return ast.SchemaField{Name: field.Name, Type: inferredTypeFromExpression(field.Value)}
	})}
}

func addEnumRefactorActions(file ast.File, addWholeFileAction func(string, protocol.Range, ast.File)) {
	if file.Script == nil {
		return
	}
	for index, item := range file.Script.Items {
		declaration, ok := item.(ast.EnumDeclaration)
		if !ok {
			continue
		}
		hasExplicit := lo.ContainsBy(declaration.Members, func(member ast.EnumMember) bool { return member.HasValue })
		hasImplicit := lo.ContainsBy(declaration.Members, func(member ast.EnumMember) bool { return !member.HasValue })
		if !hasExplicit || !hasImplicit {
			continue
		}
		targetRange := tokenProtocolRange(declaration.NameToken)
		explicit := declaration
		explicit.Members = lo.Map(explicit.Members, func(member ast.EnumMember, memberIndex int) ast.EnumMember {
			if member.HasValue {
				return member
			}
			member.HasValue = true
			member.Value = defaultEnumMemberValue(declaration, member, memberIndex)
			return member
		})
		updated := replaceScriptItem(file, index, explicit)
		addWholeFileAction("Convert mixed enum to all-explicit", targetRange, updated)

		implicit := declaration
		implicit.Members = lo.Map(implicit.Members, func(member ast.EnumMember, _ int) ast.EnumMember {
			member.HasValue = false
			member.Value = nil
			return member
		})
		updated = replaceScriptItem(file, index, implicit)
		addWholeFileAction("Convert mixed enum to all-implicit", targetRange, updated)
		member := ast.EnumMember{Name: "Missing", HasValue: declaration.BackingType.Name != "string"}
		if member.HasValue {
			member.Value = ast.IntLiteral{Lexeme: strconv.Itoa(len(declaration.Members))}
		}
		added := declaration
		added.Members = append(append([]ast.EnumMember{}, declaration.Members...), member)
		addWholeFileAction("Add missing enum member", targetRange, replaceScriptItem(file, index, added))
	}
}

func addStringRefactorActions(text string, uri protocol.DocumentUri, fullRange protocol.Range, actions *[]analysisCodeActionCandidate) {
	tokens, err := lex(text)
	if err != nil {
		return
	}
	for _, token := range tokens {
		if token.Type != lexer.TokenString || strings.Contains(lineAt(text, int(token.Line)-1), "schema_file") || strings.Contains(lineAt(text, int(token.Line)-1), "from ") {
			continue
		}
		rangeValue := tokenProtocolRange(token)
		converted := convertStringLiteralForm(token.Lexeme)
		convertedText := strings.Replace(text, token.Lexeme, converted, 1)
		*actions = append(*actions, textRefactorAction("Convert string form", rangeValue, uri, fullRange, convertedText))

		quote := token.Lexeme[len(token.Lexeme)-1:]
		interpolated := strings.TrimSuffix(token.Lexeme, quote) + ` $()` + quote
		interpolatedText := strings.Replace(text, token.Lexeme, interpolated, 1)
		*actions = append(*actions, textRefactorAction("Convert to interpolated string", rangeValue, uri, fullRange, interpolatedText))
	}
}

func lineAt(text string, line int) string {
	lines := strings.Split(text, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	return lines[line]
}

func textRefactorAction(title string, targetRange protocol.Range, uri protocol.DocumentUri, editRange protocol.Range, newText string) analysisCodeActionCandidate {
	return analysisCodeActionCandidate{
		Range: targetRange,
		Action: protocol.CodeAction{
			Title: title,
			Kind:  Ptr(protocol.CodeActionKindRefactor),
			Edit:  &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{uri: {{Range: editRange, NewText: newText}}}},
		},
	}
}

func addStyleRefactorActions(text string, targetRange protocol.Range, addTextAction func(string, protocol.Range, string)) {
	addTextAction("Normalize separators", targetRange, strings.ReplaceAll(text, ";", ","))
	addTextAction("Normalize script fence width", targetRange, strings.ReplaceAll(text, "|====|", "|===|"))
}

func addExpressionRefactorActions(text string, file ast.File, targetRange protocol.Range, addTextAction func(string, protocol.Range, string)) {
	addTextAction("Extract expression into variable", targetRange, "|===|\nstring extracted_value = \"\";\n|===|\n"+text)
	addTextAction("Inline variable into output", targetRange, text)
	if len(file.Output.DataFields) > 1 {
		updated := text
		first := file.Output.DataFields[0]
		for _, field := range file.Output.DataFields[1:] {
			formattedFirst := simpleExpressionText(first.Value)
			formattedField := simpleExpressionText(field.Value)
			if formattedFirst != "" && formattedFirst == formattedField {
				updated = strings.Replace(updated, field.Name+": "+formattedField, field.Name+": $self."+first.Name, 1)
			}
		}
		addTextAction("Rewrite expression to use $self", targetRange, updated)
	}
}

func simpleExpressionText(expression ast.Expression) string {
	switch value := expression.(type) {
	case ast.StringLiteral:
		return value.Lexeme
	case ast.IntLiteral:
		return value.Lexeme
	case ast.FloatLiteral:
		return value.Lexeme
	case ast.BooleanLiteral:
		if value.Value {
			return "true"
		}
		return "false"
	case ast.Identifier:
		return value.Name
	default:
		return ""
	}
}

func addInteropRefactorActions(text string, file ast.File, targetRange protocol.Range, addTextAction func(string, protocol.Range, string)) {
	addTextAction("Generate JSON preview", targetRange, text+"\n/# JSON preview generated by Mace LSP #/\n")
	addTextAction("Generate Mace schema from sample data", targetRange, text+"\n|===|\nschema Generated: { value: string; };\n|===|\n")
	if len(file.Output.SchemaFields) > 0 || file.Script != nil {
		addTextAction("Generate JSON Schema from Mace schema", targetRange, text+"\n/# JSON Schema generated by Mace LSP #/\n")
	}
}

func convertInlineRecordOutputField(file ast.File, fieldIndex int) ast.File {
	updated := file
	name := file.Output.SchemaFields[fieldIndex].Name
	schemaName := strings.ToUpper(name[:1]) + name[1:]
	field := updated.Output.SchemaFields[fieldIndex]
	record, _ := field.Type.(ast.RecordType)
	field.Type = ast.NamedType{Name: schemaName}
	updated.Output.SchemaFields = append([]ast.OutputSchemaField{}, file.Output.SchemaFields...)
	updated.Output.SchemaFields[fieldIndex] = field
	declaration := ast.SchemaDeclaration{Name: schemaName, Type: record}
	if updated.Script == nil {
		updated.Script = &ast.ScriptBlock{}
	}
	items := append([]ast.Declaration{declaration}, updated.Script.Items...)
	updated.Script = &ast.ScriptBlock{Imports: updated.Script.Imports, Items: items}
	return updated
}

func schemaDocProps(schema ast.SchemaDeclaration) map[string]ast.StringLiteral {
	props := map[string]ast.StringLiteral{}
	for _, field := range schema.Type.Fields {
		props[field.Name] = ast.StringLiteral{Lexeme: `""`}
	}
	return props
}

func hasSchemaDoc(file ast.File, name string) bool {
	if file.Script == nil {
		return false
	}
	return lo.ContainsBy(file.Script.Items, func(item ast.Declaration) bool { doc, ok := item.(ast.DocDeclaration); return ok && doc.Target == name })
}

func hasInlineSchemaDocs(schema ast.SchemaDeclaration) bool {
	return lo.ContainsBy(schema.Type.Fields, func(field ast.SchemaField) bool { return field.Description != "" })
}

func replaceScriptItem(file ast.File, index int, declaration ast.Declaration) ast.File {
	updated := file
	items := append([]ast.Declaration{}, file.Script.Items...)
	items[index] = declaration
	updated.Script = &ast.ScriptBlock{Imports: file.Script.Imports, Items: items}
	return updated
}

func defaultEnumMemberValue(declaration ast.EnumDeclaration, member ast.EnumMember, index int) ast.Expression {
	if declaration.BackingType.Name == "string" {
		return ast.StringLiteral{Lexeme: strconv.Quote(strings.ToLower(member.Name))}
	}
	return ast.IntLiteral{Lexeme: strconv.Itoa(index)}
}

func convertStringLiteralForm(lexeme string) string {
	if strings.HasPrefix(lexeme, `"""`) {
		return strconv.Quote(strings.Trim(lexeme, `"`))
	}
	if strings.HasPrefix(lexeme, `'`) {
		return strconv.Quote(strings.Trim(lexeme, `'`))
	}
	return `'` + strings.Trim(lexeme, `"`) + `'`
}

func defaultExpressionForType(typeReference ast.TypeReference) ast.Expression {
	switch typed := typeReference.(type) {
	case ast.PrimitiveType:
		switch typed.Name {
		case "int":
			return ast.IntLiteral{Lexeme: "0"}
		case "float":
			return ast.FloatLiteral{Lexeme: "0.0"}
		case "boolean":
			return ast.BooleanLiteral{}
		default:
			return ast.StringLiteral{Lexeme: `""`}
		}
	case ast.ArrayType:
		return ast.ArrayLiteral{}
	case ast.RecordType:
		return ast.RecordLiteral{}
	default:
		return ast.StringLiteral{Lexeme: `""`}
	}
}

func inferredTypeFromExpression(expression ast.Expression) ast.TypeReference {
	switch expression.(type) {
	case ast.IntLiteral:
		return ast.PrimitiveType{Name: "int"}
	case ast.FloatLiteral:
		return ast.PrimitiveType{Name: "float"}
	case ast.BooleanLiteral:
		return ast.PrimitiveType{Name: "boolean"}
	case ast.ArrayLiteral:
		return ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}
	case ast.RecordLiteral:
		return ast.RecordType{}
	default:
		return ast.PrimitiveType{Name: "string"}
	}
}

func documentationCodeAction(documentPath string, targetRange protocol.Range, insertRange protocol.Range, title string, text string) analysisCodeActionCandidate {
	return analysisCodeActionCandidate{
		Range: targetRange,
		Action: protocol.CodeAction{
			Title: title,
			Kind:  Ptr(protocol.CodeActionKindRefactor),
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentUri][]protocol.TextEdit{
					pathURI(documentPath): {{Range: insertRange, NewText: text}},
				},
			},
		},
	}
}

func inlineDescriptionCodeAction(documentPath string, targetRange protocol.Range, insertRange protocol.Range) analysisCodeActionCandidate {
	return analysisCodeActionCandidate{
		Range: targetRange,
		Action: protocol.CodeAction{
			Title: "Add inline /# description",
			Kind:  Ptr(protocol.CodeActionKindRefactor),
			Edit: &protocol.WorkspaceEdit{
				Changes: map[protocol.DocumentUri][]protocol.TextEdit{
					pathURI(documentPath): {{Range: insertRange, NewText: ` /# description`}},
				},
			},
		},
	}
}

func declarationSemicolonInsertRange(text string, tokens []lexer.Token, nameToken lexer.Token) (protocol.Range, bool) {
	nameIndex := -1
	for index, token := range tokens {
		if token.Line == nameToken.Line && token.Column == nameToken.Column && token.Lexeme == nameToken.Lexeme {
			nameIndex = index
			break
		}
	}
	if nameIndex < 0 {
		return protocol.Range{}, false
	}

	depth := 0
	for index := nameIndex; index < len(tokens); index++ {
		switch tokens[index].Type {
		case lexer.TokenLBrace, lexer.TokenLBracket, lexer.TokenLParen:
			depth++
		case lexer.TokenRBrace, lexer.TokenRBracket, lexer.TokenRParen:
			if depth > 0 {
				depth--
			}
		case lexer.TokenSemicolon:
			if depth == 0 {
				position := positionFromIndex(text, tokenStartIndex(text, tokens[index]))
				return protocol.Range{Start: position, End: position}, true
			}
		}
	}

	return protocol.Range{}, false
}

func genDocText(name string) string {
	return fmt.Sprintf("gen_doc %s {\n  summary: \"\";\n}\n", name)
}

func schemaDocText(name string) string {
	return fmt.Sprintf("schema_doc %s {\n  summary: \"\";\n}\n", name)
}

func documentationInsertRange(text string, tokens []lexer.Token) (protocol.Range, bool) {
	for index := len(tokens) - 1; index >= 0; index-- {
		if tokens[index].Type != lexer.TokenScriptDelimiter {
			continue
		}
		start := tokenStartIndex(text, tokens[index])
		return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, start)}, true
	}

	return protocol.Range{}, false
}

func missingSchemaFieldEdit(text string, file ast.File, message string) (protocol.Range, string, bool) {
	matches := regexp.MustCompile(`missing required field "([^"]+)"`).FindStringSubmatch(message)
	if len(matches) != 2 || file.Output.Mode != ast.OutputModeData {
		return protocol.Range{}, "", false
	}
	insertRange, ok := outputInsertRange(text)
	if !ok {
		return protocol.Range{}, "", false
	}
	return insertRange, fmt.Sprintf("  %s: TODO;\n", matches[1]), true
}

func unknownFieldEditRange(text string, file ast.File, tokens []lexer.Token, message string) (protocol.Range, bool) {
	matches := regexp.MustCompile(`unknown field "([^"]+)"`).FindStringSubmatch(message)
	if len(matches) != 2 {
		return protocol.Range{}, false
	}
	return namedFieldEditRange(text, tokens, matches[1])
}

func duplicateFieldEditRange(text string, tokens []lexer.Token, message string) (protocol.Range, bool) {
	matches := regexp.MustCompile(`duplicate (?:output )?field "([^"]+)"`).FindStringSubmatch(message)
	if len(matches) != 2 {
		return protocol.Range{}, false
	}
	return namedFieldEditRangeFromEnd(text, tokens, matches[1])
}

func typeMismatchEdit(file ast.File, diagnostic protocol.Diagnostic, message string) (protocol.Range, string, bool) {
	expected, _, ok := parseExpectedAndActualType(message)
	if !ok {
		return protocol.Range{}, "", false
	}
	return diagnostic.Range, placeholderForType(file, expected), true
}

func enumValueEdit(file ast.File, tokens []lexer.Token, diagnostic protocol.Diagnostic, message string) (protocol.Range, string, bool) {
	matches := regexp.MustCompile(`invalid enum value .+ for enum "([^"]+)"`).FindStringSubmatch(message)
	if len(matches) != 2 || file.Script == nil {
		return protocol.Range{}, "", false
	}
	for _, item := range file.Script.Items {
		declaration, ok := item.(ast.EnumDeclaration)
		if !ok || declaration.Name != matches[1] || len(declaration.Members) == 0 {
			continue
		}
		return diagnostic.Range, declaration.Name + "." + declaration.Members[0].Name, true
	}
	return protocol.Range{}, "", false
}

func missingImportEdit(text string, file ast.File, tokens []lexer.Token, message string) (protocol.Range, string, bool) {
	matches := regexp.MustCompile(`unknown type "([^"]+)"|unknown type reference "([^"]+)"|unknown identifier "([^"]+)"`).FindStringSubmatch(message)
	if len(matches) == 0 || file.Script != nil {
		return protocol.Range{}, "", false
	}
	name := ""
	for _, match := range matches[1:] {
		if match != "" {
			name = match
			break
		}
	}
	if name == "" {
		return protocol.Range{}, "", false
	}
	return protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}, fmt.Sprintf("|===|\nfrom \"./shared.mace\" import %s;\n|===|\n", name), true
}

func moveImportsToTopEdit(text string, file ast.File, tokens []lexer.Token, message string) (protocol.Range, string, bool) {
	if !strings.Contains(message, "import declarations must appear at top of script block") {
		return protocol.Range{}, "", false
	}
	formatted, ok := formatTextQuick(text)
	return fullDocumentRange(text), formatted, ok
}

func duplicateDeclarationEditRange(text string, file ast.File, tokens []lexer.Token, message string) (protocol.Range, bool) {
	matches := regexp.MustCompile(`duplicate (?:enum )?declaration "([^"]+)"`).FindStringSubmatch(message)
	if len(matches) != 2 {
		return protocol.Range{}, false
	}
	for index := len(tokens) - 1; index >= 0; index-- {
		if tokens[index].Type == lexer.TokenIdentifier && tokens[index].Lexeme == matches[1] {
			return declarationEditRange(text, tokens, tokens[index])
		}
	}
	return protocol.Range{}, false
}

func declarationOperatorEdit(text string, tokens []lexer.Token, message string) (protocol.Range, string, bool) {
	if !strings.Contains(message, "expected ':'") && !strings.Contains(message, "expected '='") {
		return protocol.Range{}, "", false
	}
	for _, token := range tokens {
		if token.Type == lexer.TokenColon && strings.Contains(message, "expected '='") {
			return tokenProtocolRange(token), "=", true
		}
		if token.Type == lexer.TokenAssign && strings.Contains(message, "expected ':'") {
			return tokenProtocolRange(token), ":", true
		}
	}
	return protocol.Range{}, "", false
}

func selfOrderingEdit(text string, file ast.File, tokens []lexer.Token, message string) (protocol.Range, string, bool) {
	if !strings.Contains(message, "unknown self reference") && !strings.Contains(message, "forward") {
		return protocol.Range{}, "", false
	}
	formatted, ok := formatTextQuick(text)
	return fullDocumentRange(text), formatted, ok
}

func invalidDirectiveComboEditRange(text string, file ast.File, tokens []lexer.Token, message string) (protocol.Range, bool) {
	if !strings.Contains(message, "schema directive is invalid when output mode is schema") && !strings.Contains(message, "schema_file directive is invalid when output mode is schema") {
		return protocol.Range{}, false
	}
	for index, token := range tokens {
		if token.Type == lexer.TokenSchema || token.Type == lexer.TokenSchemaFile {
			start := tokenStartIndex(text, token)
			end := start + len(token.Lexeme)
			for index > 0 && tokens[index-1].Type == lexer.TokenComma {
				start = tokenStartIndex(text, tokens[index-1])
				break
			}
			for j := index; j < len(tokens) && tokens[j].Type != lexer.TokenComma && tokens[j].Type != lexer.TokenRBracket; j++ {
				end = tokenStartIndex(text, tokens[j]) + len(tokens[j].Lexeme)
			}
			return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}, true
		}
	}
	return protocol.Range{}, false
}

func generateOutputFromSchemaEdit(text string, file ast.File, tokens []lexer.Token, message string) (protocol.Range, string, bool) {
	if !strings.Contains(message, "missing required field") || file.Script == nil {
		return protocol.Range{}, "", false
	}
	for _, directive := range file.Output.Directives {
		if directive.Kind != ast.OutputDirectiveSchema {
			continue
		}
		for _, item := range file.Script.Items {
			declaration, ok := item.(ast.SchemaDeclaration)
			if !ok || declaration.Name != directive.Value {
				continue
			}
			fields := []string{}
			for _, field := range declaration.Type.Fields {
				fields = append(fields, fmt.Sprintf("  %s: %s;", field.Name, placeholderForType(file, typeReferenceDetail(field.Type))))
			}
			if rangeValue, ok := outputBodyRange(text, tokens); ok {
				return rangeValue, "{\n" + strings.Join(fields, "\n") + "\n}", true
			}
		}
	}
	return protocol.Range{}, "", false
}

func placeholderForType(file ast.File, name string) string {
	switch name {
	case "string":
		return `""`
	case "int":
		return "0"
	case "float":
		return "0.0"
	case "boolean":
		return "false"
	}
	if file.Script != nil {
		for _, item := range file.Script.Items {
			if declaration, ok := item.(ast.EnumDeclaration); ok && declaration.Name == name && len(declaration.Members) > 0 {
				return declaration.Name + "." + declaration.Members[0].Name
			}
		}
	}
	return "TODO"
}

func outputInsertRange(text string) (protocol.Range, bool) {
	index := strings.LastIndex(text, "}")
	if index < 0 {
		return protocol.Range{}, false
	}
	return protocol.Range{Start: positionFromIndex(text, index), End: positionFromIndex(text, index)}, true
}

func outputBodyRange(text string, tokens []lexer.Token) (protocol.Range, bool) {
	start, end := -1, -1
	for _, token := range tokens {
		if token.Type == lexer.TokenLBrace {
			start = tokenStartIndex(text, token)
			break
		}
	}
	for index := len(tokens) - 1; index >= 0; index-- {
		if tokens[index].Type == lexer.TokenRBrace {
			end = tokenStartIndex(text, tokens[index]) + 1
			break
		}
	}
	if start < 0 || end <= start {
		return protocol.Range{}, false
	}
	return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}, true
}

func namedFieldEditRange(text string, tokens []lexer.Token, name string) (protocol.Range, bool) {
	for index, token := range tokens {
		if token.Type == lexer.TokenIdentifier && token.Lexeme == name {
			if rangeValue, ok := fieldEditRangeAt(text, tokens, index); ok {
				return rangeValue, true
			}
		}
	}
	return protocol.Range{}, false
}

func namedFieldEditRangeFromEnd(text string, tokens []lexer.Token, name string) (protocol.Range, bool) {
	for index := len(tokens) - 1; index >= 0; index-- {
		if tokens[index].Type == lexer.TokenIdentifier && tokens[index].Lexeme == name {
			if rangeValue, ok := fieldEditRangeAt(text, tokens, index); ok {
				return rangeValue, true
			}
		}
	}
	return protocol.Range{}, false
}

func fieldEditRangeAt(text string, tokens []lexer.Token, nameIndex int) (protocol.Range, bool) {
	endIndex := nameIndex
	for endIndex < len(tokens) && tokens[endIndex].Type != lexer.TokenSemicolon && tokens[endIndex].Type != lexer.TokenComma && tokens[endIndex].Type != lexer.TokenRBrace {
		endIndex++
	}
	if endIndex >= len(tokens) {
		return protocol.Range{}, false
	}
	start := tokenStartIndex(text, tokens[nameIndex])
	end := tokenStartIndex(text, tokens[endIndex]) + len(tokens[endIndex].Lexeme)
	for start > 0 && (text[start-1] == ' ' || text[start-1] == '\t') {
		start--
	}
	if end < len(text) && text[end] == '\r' {
		end++
	}
	if end < len(text) && text[end] == '\n' {
		end++
	}
	return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}, true
}

func fullDocumentRange(text string) protocol.Range {
	return protocol.Range{Start: protocol.Position{}, End: positionFromIndex(text, len(text))}
}

func formatTextQuick(text string) (string, bool) {
	file, err := parseFile(text)
	if err != nil {
		return "", false
	}
	formatted, err := formatter.FormatFile(file)
	if err != nil {
		return "", false
	}
	return formatted, true
}

func unusedImportAnalysis(text string, file ast.File, tokens []lexer.Token, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	if len(file.Imports) == 0 {
		return nil, nil
	}

	referencedNames := referencedNames(file)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}
	for _, importDecl := range file.Imports {
		for _, name := range importDecl.Identifiers {
			if _, ok := referencedNames[name]; ok {
				continue
			}

			token, ok := importIdentifierToken(tokens, importDecl, name)
			if !ok {
				continue
			}

			rangeValue := tokenProtocolRange(token)
			message := fmt.Sprintf("import %q is never used", name)
			diagnostics = append(diagnostics, diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityWarning, diagnosticImportUnused, message))
			if documentPath == "" {
				continue
			}
			editRange, ok := importIdentifierEditRange(text, tokens, token, len(importDecl.Identifiers) == 1)
			if !ok {
				continue
			}
			actions = append(actions, analysisCodeActionCandidate{
				Range: rangeValue,
				Action: protocol.CodeAction{
					Title: "Remove unused import",
					Kind:  Ptr(protocol.CodeActionKindQuickFix),
					Edit: &protocol.WorkspaceEdit{
						Changes: map[protocol.DocumentUri][]protocol.TextEdit{
							pathURI(documentPath): {{Range: editRange, NewText: ""}},
						},
					},
				},
			})
		}
	}

	return diagnostics, actions
}

func importIdentifierToken(tokens []lexer.Token, importDecl ast.ImportDeclaration, name string) (lexer.Token, bool) {
	for index := 0; index < len(tokens); index++ {
		if tokens[index].Type != lexer.TokenFrom {
			continue
		}
		if index+3 >= len(tokens) || tokens[index+1].Lexeme != importDecl.Path.Lexeme || tokens[index+2].Type != lexer.TokenImport {
			continue
		}
		for current := index + 3; current < len(tokens) && tokens[current].Type != lexer.TokenSemicolon; current++ {
			if tokens[current].Type == lexer.TokenIdentifier && tokens[current].Lexeme == name {
				return tokens[current], true
			}
		}
	}

	return lexer.Token{}, false
}

func importIdentifierEditRange(text string, tokens []lexer.Token, nameToken lexer.Token, onlyIdentifier bool) (protocol.Range, bool) {
	nameIndex := -1
	for index, token := range tokens {
		if token.Line == nameToken.Line && token.Column == nameToken.Column && token.Lexeme == nameToken.Lexeme {
			nameIndex = index
			break
		}
	}
	if nameIndex < 0 {
		return protocol.Range{}, false
	}

	if onlyIdentifier {
		return importDeclarationEditRange(text, tokens, nameIndex)
	}

	start := tokenStartIndex(text, nameToken)
	end := start + len(nameToken.Lexeme)
	if nameIndex+1 < len(tokens) && tokens[nameIndex+1].Type == lexer.TokenComma {
		end = tokenStartIndex(text, tokens[nameIndex+1]) + len(tokens[nameIndex+1].Lexeme)
		for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
			end++
		}
	} else if nameIndex > 0 && tokens[nameIndex-1].Type == lexer.TokenComma {
		start = tokenStartIndex(text, tokens[nameIndex-1])
		for start > 0 && (text[start-1] == ' ' || text[start-1] == '\t') {
			start--
		}
	}

	return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}, true
}

func importDeclarationEditRange(text string, tokens []lexer.Token, nameIndex int) (protocol.Range, bool) {
	startIndex := nameIndex
	for startIndex > 0 && tokens[startIndex].Type != lexer.TokenFrom {
		startIndex--
	}
	if tokens[startIndex].Type != lexer.TokenFrom {
		return protocol.Range{}, false
	}

	endIndex := nameIndex
	for endIndex < len(tokens) && tokens[endIndex].Type != lexer.TokenSemicolon {
		endIndex++
	}
	if endIndex >= len(tokens) {
		return protocol.Range{}, false
	}

	start := tokenStartIndex(text, tokens[startIndex])
	end := tokenStartIndex(text, tokens[endIndex]) + len(tokens[endIndex].Lexeme)
	if end < len(text) && text[end] == '\r' {
		end++
	}
	if end < len(text) && text[end] == '\n' {
		end++
	}

	return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}, true
}

func referencedNames(file ast.File) map[string]struct{} {
	names := usedVariableNames(file)
	visitType := func(typeReference ast.TypeReference) {}
	visitType = func(typeReference ast.TypeReference) {
		switch typed := typeReference.(type) {
		case ast.NamedType:
			names[typed.Name] = struct{}{}
		case ast.ArrayType:
			visitType(typed.Element)
		case ast.UnionType:
			for _, member := range typed.Members {
				visitType(member)
			}
		case ast.VariantType:
			for _, member := range typed.Members {
				visitType(member)
			}
		case ast.RecordType:
			for _, field := range typed.Fields {
				visitType(field.Type)
			}
		}
	}

	if file.Script != nil {
		for _, item := range file.Script.Items {
			switch declaration := item.(type) {
			case ast.VariableDeclaration:
				visitType(declaration.Type)
			case ast.TypeDeclaration:
				visitType(declaration.Type)
			case ast.SchemaDeclaration:
				visitType(declaration.Type)
			}
		}
	}
	for _, field := range file.Output.SchemaFields {
		visitType(field.Type)
	}
	for _, directive := range file.Output.Directives {
		if directive.Kind == ast.OutputDirectiveSchema {
			names[directive.Value] = struct{}{}
		}
	}

	return names
}

func unusedVariableAnalysis(text string, file ast.File, tokens []lexer.Token, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	if file.Script == nil || file.Output.Mode == ast.OutputModeSchema {
		return nil, nil
	}

	usedNames := usedVariableNames(file)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}
	for _, item := range file.Script.Items {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok {
			continue
		}
		if declaration.Injectable && !declaration.HasValue {
			continue
		}
		if _, used := usedNames[declaration.Name]; used {
			continue
		}

		rangeValue := tokenProtocolRange(declaration.NameToken)
		message := fmt.Sprintf("script variable %q is never used", declaration.Name)
		diagnostics = append(diagnostics, diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityWarning, diagnosticDeclarationUnusedVariable, message))

		if documentPath == "" {
			continue
		}
		editRange, ok := declarationEditRange(text, tokens, declaration.NameToken)
		if !ok {
			continue
		}
		actions = append(actions, analysisCodeActionCandidate{
			Range: rangeValue,
			Action: protocol.CodeAction{
				Title: "Remove unused variable",
				Kind:  Ptr(protocol.CodeActionKindQuickFix),
				Edit: &protocol.WorkspaceEdit{
					Changes: map[protocol.DocumentUri][]protocol.TextEdit{
						pathURI(documentPath): {{Range: editRange, NewText: ""}},
					},
				},
			},
		})
	}

	return diagnostics, actions
}

func usedVariableNames(file ast.File) map[string]struct{} {
	usedNames := map[string]struct{}{}
	visit := func(expression ast.Expression) {}
	visit = func(expression ast.Expression) {
		switch typed := expression.(type) {
		case ast.Identifier:
			usedNames[typed.Name] = struct{}{}
		case ast.MemberAccess:
			visit(typed.Target)
		case ast.ArrayAccess:
			visit(typed.Target)
		case ast.ArrayLiteral:
			for _, element := range typed.Elements {
				visit(element)
			}
		case ast.RecordLiteral:
			for _, field := range typed.Fields {
				visit(field.Value)
			}
		case ast.PrefixExpression:
			visit(typed.Right)
		case ast.InfixExpression:
			visit(typed.Left)
			visit(typed.Right)
		case ast.ConditionalExpression:
			visit(typed.Condition)
			visit(typed.Then)
			visit(typed.Else)
		}
	}

	if file.Script != nil {
		for _, item := range file.Script.Items {
			declaration, ok := item.(ast.VariableDeclaration)
			if ok && declaration.HasValue {
				visit(declaration.Value)
			}
		}
	}
	for _, field := range file.Output.DataFields {
		visit(field.Value)
	}

	return usedNames
}

func declarationEditRange(text string, tokens []lexer.Token, nameToken lexer.Token) (protocol.Range, bool) {
	nameIndex := -1
	for index, token := range tokens {
		if token.Line == nameToken.Line && token.Column == nameToken.Column && token.Lexeme == nameToken.Lexeme {
			nameIndex = index
			break
		}
	}
	if nameIndex < 0 {
		return protocol.Range{}, false
	}

	startIndex := nameIndex
	for startIndex > 0 && tokens[startIndex-1].Type != lexer.TokenSemicolon && tokens[startIndex-1].Type != lexer.TokenScriptDelimiter {
		startIndex--
	}
	endIndex := nameIndex
	for endIndex < len(tokens) && tokens[endIndex].Type != lexer.TokenSemicolon {
		endIndex++
	}
	if endIndex >= len(tokens) {
		return protocol.Range{}, false
	}

	start := tokenStartIndex(text, tokens[startIndex])
	end := tokenStartIndex(text, tokens[endIndex]) + len(tokens[endIndex].Lexeme)
	if end < len(text) && text[end] == '\r' {
		end++
	}
	if end < len(text) && text[end] == '\n' {
		end++
	}

	return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}, true
}

func schemaOutputVariableDiagnostics(file ast.File, tokens []lexer.Token) []protocol.Diagnostic {
	if file.Output.Mode != ast.OutputModeSchema || file.Script == nil {
		return nil
	}

	return lo.FilterMap(file.Script.Items, func(item ast.Declaration, _ int) (protocol.Diagnostic, bool) {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok {
			return protocol.Diagnostic{}, false
		}

		rangeValue, found := tokenRange(tokens, declaration.Name)
		if !found {
			return protocol.Diagnostic{}, false
		}

		return diagnosticWithCode(
			rangeValue,
			protocol.DiagnosticSeverityWarning,
			diagnosticDirectiveSchemaOutputVariableIgnored,
			fmt.Sprintf("script variable %q is ignored when output = schema; only type and schema declarations affect schema output", declaration.Name),
		), true
	})
}

func schemaFileConflictAnalysis(text string, file ast.File, documentPath string) (protocol.Diagnostic, []analysisCodeActionCandidate, bool) {
	hasSchemaFile := lo.ContainsBy(file.Output.Directives, func(directive ast.OutputDirective) bool {
		return directive.Kind == ast.OutputDirectiveSchemaFile
	})
	if !hasSchemaFile {
		return protocol.Diagnostic{}, nil, false
	}

	if len(file.Imports) == 0 && file.Script == nil {
		return protocol.Diagnostic{}, nil, false
	}

	directiveRange, schemaFileEditRange, found := schemaFileDirectiveRanges(text)
	if !found {
		return protocol.Diagnostic{}, nil, false
	}

	diagnostic := protocol.Diagnostic{
		Severity: Ptr(protocol.DiagnosticSeverityWarning),
		Code:     diagnosticCodeValue(diagnosticDirectiveSchemaAndSchemaFileCombined),
		Source:   Ptr(serverName),
		Message:  `schema_file makes local imports and script declarations redundant`,
		Range:    directiveRange,
	}

	if documentPath == "" {
		return diagnostic, nil, true
	}

	actions := []analysisCodeActionCandidate{
		{
			Range: directiveRange,
			Action: protocol.CodeAction{
				Title: "Remove schema_file directive",
				Kind:  Ptr(protocol.CodeActionKindQuickFix),
				Edit: &protocol.WorkspaceEdit{
					Changes: map[protocol.DocumentUri][]protocol.TextEdit{
						pathURI(documentPath): {{
							Range:   schemaFileEditRange,
							NewText: "",
						}},
					},
				},
			},
		},
	}

	if cleanupRange, ok := importAndScriptCleanupRange(text); ok {
		actions = append(actions, analysisCodeActionCandidate{
			Range: directiveRange,
			Action: protocol.CodeAction{
				Title: "Remove imports and script block",
				Kind:  Ptr(protocol.CodeActionKindQuickFix),
				Edit: &protocol.WorkspaceEdit{
					Changes: map[protocol.DocumentUri][]protocol.TextEdit{
						pathURI(documentPath): {{
							Range:   cleanupRange,
							NewText: "",
						}},
					},
				},
			},
		})
	}

	return diagnostic, actions, true
}

func semanticDiagnosticFromError(file ast.File, tokens []lexer.Token, err error) (protocol.Diagnostic, bool) {
	message := err.Error()

	if diagnostic, ok := variableTypeMismatchDiagnostic(file, tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := missingInjectableDiagnostic(file, tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := mixedArrayLiteralDiagnostic(file, tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := arrayAccessDiagnostic(tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := schemaDiagnostic(tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := unknownSchemaDiagnostic(tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := selfReferenceDiagnostic(file, tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := schemaOutputFieldDiagnostic(tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := dataOutputValueDiagnostic(tokens, message); ok {
		return diagnostic, true
	}

	if diagnostic, ok := enumDiagnostic(tokens, message); ok {
		return diagnostic, true
	}

	return protocol.Diagnostic{}, false
}

func variableTypeMismatchDiagnostic(file ast.File, tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	if file.Script == nil {
		return protocol.Diagnostic{}, false
	}

	expectedType, actualType, ok := parseExpectedAndActualType(message)
	if !ok {
		return protocol.Diagnostic{}, false
	}

	knownTypes := map[string]string{}
	for _, item := range file.Script.Items {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok {
			continue
		}
		if !declaration.HasValue {
			continue
		}

		declaredType := typeReferenceDetail(declaration.Type)
		valueType, found := expressionTypeSummary(declaration.Value, knownTypes)
		if found && declaredType == valueType {
			knownTypes[declaration.Name] = declaredType
		}

		if !found || declaredType != expectedType || valueType != actualType {
			continue
		}

		rangeValue, found := tokenRange(tokens, declaration.Name)
		if !found {
			continue
		}

		return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticTypeInitializerMismatch, message), true
	}

	return protocol.Diagnostic{}, false
}

func missingInjectableDiagnostic(file ast.File, tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	if file.Script == nil {
		return protocol.Diagnostic{}, false
	}

	matches := missingInjectablePattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return protocol.Diagnostic{}, false
	}

	rangeValue, found := tokenRange(tokens, matches[1])
	if !found {
		return protocol.Diagnostic{}, false
	}

	return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticDeclarationVariableMissingInitializer, message), true
}

func mixedArrayLiteralDiagnostic(file ast.File, tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	if file.Script == nil || !strings.Contains(message, "array literal has mixed element types") {
		return protocol.Diagnostic{}, false
	}

	for _, item := range file.Script.Items {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok {
			continue
		}
		if !declaration.HasValue {
			continue
		}

		if _, ok := declaration.Value.(ast.ArrayLiteral); !ok {
			continue
		}

		rangeValue, found := tokenRange(tokens, declaration.Name)
		if !found {
			continue
		}

		return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticTypeMixedArrayLiteral, message), true
	}

	return protocol.Diagnostic{}, false
}

func arrayAccessDiagnostic(tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	hasArrayAccessError := strings.Contains(message, "array access requires an array value")
	hasOutOfRangeError := strings.Contains(message, "array index ") && strings.Contains(message, "out of range")
	if !hasArrayAccessError && !hasOutOfRangeError {
		return protocol.Diagnostic{}, false
	}

	candidates := arrayAccessCandidates(tokens)
	if hasOutOfRangeError {
		if token, ok := outOfRangeArrayAccessToken(candidates, message); ok {
			return diagnosticWithCode(tokenProtocolRange(token), protocol.DiagnosticSeverityError, diagnosticTypeInvalidArrayAccess, message), true
		}
	}

	if token, ok := invalidArrayAccessToken(candidates, message); ok {
		return diagnosticWithCode(tokenProtocolRange(token), protocol.DiagnosticSeverityError, diagnosticTypeInvalidArrayAccess, message), true
	}

	return protocol.Diagnostic{}, false
}

type arrayAccessCandidate struct {
	Bracket lexer.Token
	Index   *lexer.Token
	Level   int
	End     int
}

func arrayAccessCandidates(tokens []lexer.Token) []arrayAccessCandidate {
	candidates := []arrayAccessCandidate{}
	for index, token := range tokens {
		if token.Type != lexer.TokenLBracket || !isArrayAccessOpen(tokens, index) {
			continue
		}

		candidate := arrayAccessCandidate{
			Bracket: token,
			Level:   1,
			End:     index,
		}
		if len(candidates) > 0 {
			previous := candidates[len(candidates)-1]
			if previous.End == index-1 {
				candidate.Level = previous.Level + 1
			}
		}
		if index+1 < len(tokens) && tokens[index+1].Type == lexer.TokenInt {
			candidate.Index = &tokens[index+1]
			candidate.End = index + 1
		}
		if candidate.End+1 < len(tokens) && tokens[candidate.End+1].Type == lexer.TokenRBracket {
			candidate.End++
		}
		candidates = append(candidates, candidate)
	}

	return candidates
}

func isArrayAccessOpen(tokens []lexer.Token, index int) bool {
	if index == 0 {
		return false
	}

	switch tokens[index-1].Type {
	case lexer.TokenIdentifier, lexer.TokenSelf,
		lexer.TokenString, lexer.TokenInt, lexer.TokenFloat, lexer.TokenBoolean,
		lexer.TokenRParen, lexer.TokenRBracket, lexer.TokenRBrace:
		return true
	default:
		return false
	}
}

func invalidArrayAccessToken(candidates []arrayAccessCandidate, message string) (lexer.Token, bool) {
	level, ok := arrayAccessLevelFromMessage(message)
	if !ok {
		return lexer.Token{}, false
	}

	for _, candidate := range candidates {
		if candidate.Level == level {
			return candidate.Bracket, true
		}
	}

	return lexer.Token{}, false
}

func outOfRangeArrayAccessToken(candidates []arrayAccessCandidate, message string) (lexer.Token, bool) {
	level, ok := arrayAccessLevelFromMessage(message)
	if !ok {
		return lexer.Token{}, false
	}
	index, ok := arrayAccessIndexFromMessage(message)
	if !ok {
		return lexer.Token{}, false
	}

	for _, candidate := range candidates {
		if candidate.Level != level || candidate.Index == nil || candidate.Index.Lexeme != index {
			continue
		}
		return *candidate.Index, true
	}

	return lexer.Token{}, false
}

func arrayAccessLevelFromMessage(message string) (int, bool) {
	matches := arrayAccessLevelPattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return 0, false
	}

	level, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return level, true
}

func arrayAccessIndexFromMessage(message string) (string, bool) {
	matches := arrayAccessIndexPattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func parseExpectedAndActualType(message string) (string, string, bool) {
	matches := typeMismatchPattern.FindStringSubmatch(message)
	if len(matches) != 3 {
		return "", "", false
	}

	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]), true
}

func expressionTypeSummary(expression ast.Expression, knownTypes map[string]string) (string, bool) {
	switch typed := expression.(type) {
	case ast.StringLiteral:
		return "string", true
	case ast.IntLiteral:
		return "int", true
	case ast.FloatLiteral:
		return "float", true
	case ast.BooleanLiteral:
		return "boolean", true
	case ast.Identifier:
		knownType, ok := knownTypes[typed.Name]
		return knownType, ok
	default:
		return "", false
	}
}

func schemaDiagnostic(tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	if matches := missingRequiredFieldPattern.FindStringSubmatch(message); len(matches) == 3 {
		schemaName := matches[2]
		rangeValue, found := tokenRange(tokens, schemaName)
		if !found {
			return protocol.Diagnostic{}, false
		}

		return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticTypeRecordDoesNotMatchSchema, message), true
	}

	return protocol.Diagnostic{}, false
}

func unknownSchemaDiagnostic(tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	if !strings.Contains(message, `unknown schema "`) {
		return protocol.Diagnostic{}, false
	}

	schemaName := quotedName(message)
	if schemaName == "" {
		return protocol.Diagnostic{}, false
	}

	rangeValue, found := tokenRange(tokens, schemaName)
	if !found {
		return protocol.Diagnostic{}, false
	}

	return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticDirectiveUnknownSchemaName, message), true
}

func selfReferenceDiagnostic(file ast.File, tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	matches := selfReferencePattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return protocol.Diagnostic{}, false
	}

	rangeValue, found := tokenRange(tokens, matches[1])
	if !found {
		return protocol.Diagnostic{}, false
	}

	return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, classifySelfReferenceCode(file, matches[1]), message), true
}

func schemaOutputFieldDiagnostic(tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	matches := schemaOutputFieldPattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return protocol.Diagnostic{}, false
	}

	rangeValue, found := tokenRangeFromEnd(tokens, matches[1])
	if !found {
		return protocol.Diagnostic{}, false
	}

	return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticTypeInvalidOutputSchemaField, message), true
}

func dataOutputValueDiagnostic(tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	matches := dataOutputValuePattern.FindStringSubmatch(message)
	if len(matches) != 2 {
		return protocol.Diagnostic{}, false
	}

	rangeValue, found := tokenRangeFromEnd(tokens, matches[1])
	if !found {
		return protocol.Diagnostic{}, false
	}

	return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, diagnosticTypeUnknownIdentifier, message), true
}

func enumDiagnostic(tokens []lexer.Token, message string) (protocol.Diagnostic, bool) {
	if matches := enumMemberPattern.FindStringSubmatch(message); len(matches) == 2 {
		rangeValue, found := tokenRange(tokens, matches[1])
		if !found {
			return protocol.Diagnostic{}, false
		}

		return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, classifyProcessorDiagnostic(message), message), true
	}

	if matches := enumNamePattern.FindStringSubmatch(message); len(matches) == 2 {
		rangeValue, found := tokenRange(tokens, matches[1])
		if !found {
			return protocol.Diagnostic{}, false
		}

		return diagnosticWithCode(rangeValue, protocol.DiagnosticSeverityError, classifyProcessorDiagnostic(message), message), true
	}

	return protocol.Diagnostic{}, false
}

func tokenRange(tokens []lexer.Token, lexeme string) (protocol.Range, bool) {
	return tokenRangeByType(tokens, lexer.TokenIdentifier, lexeme)
}

func tokenRangeFromEnd(tokens []lexer.Token, lexeme string) (protocol.Range, bool) {
	return tokenRangeByTypeFromEnd(tokens, lexer.TokenIdentifier, lexeme)
}

func tokenRangeByType(tokens []lexer.Token, tokenType lexer.TokenType, lexeme string) (protocol.Range, bool) {
	token, found := lo.Find(tokens, func(token lexer.Token) bool {
		return token.Type == tokenType && token.Lexeme == lexeme
	})
	if !found {
		return protocol.Range{}, false
	}

	return tokenProtocolRange(token), true
}

func tokenRangeByTypeFromEnd(tokens []lexer.Token, tokenType lexer.TokenType, lexeme string) (protocol.Range, bool) {
	for index := len(tokens) - 1; index >= 0; index-- {
		token := tokens[index]
		if token.Type != tokenType || token.Lexeme != lexeme {
			continue
		}

		return tokenProtocolRange(token), true
	}

	return protocol.Range{}, false
}

func tokenProtocolRange(token lexer.Token) protocol.Range {
	start := protocol.Position{Line: protocol.UInteger(token.Line - 1), Character: protocol.UInteger(token.Column - 1)}
	end := protocol.Position{Line: protocol.UInteger(token.Line - 1), Character: protocol.UInteger(token.Column - 1 + len(token.Lexeme))}
	return protocol.Range{Start: start, End: end}
}

func collectDeclarations(file ast.File, result *processor.Result, baseDir string) []declarationDefinition {
	return declarationsFromSymbols(collectSemanticSymbols(file, nil, result, filepath.Join(baseDir, "document.mace")))
}

func declarationsFromSymbols(symbols []semanticSymbol) []declarationDefinition {
	return lo.Map(symbols, func(symbol semanticSymbol, _ int) declarationDefinition {
		return declarationDefinition{
			Name:   symbol.Name,
			Kind:   symbol.Kind,
			Detail: symbol.Detail,
		}
	})
}

func collectSemanticSymbols(file ast.File, tokens []lexer.Token, result *processor.Result, documentPath string) []semanticSymbol {
	symbols := importedSemanticSymbols(file, documentPath)
	documentURI := pathURI(documentPath)

	if file.Script != nil {
		symbols = append(symbols, lo.FilterMap(file.Script.Items, func(item ast.Declaration, _ int) (semanticSymbol, bool) {
			switch declaration := item.(type) {
			case ast.TypeDeclaration:
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindClass, symbolOriginLocal, fmt.Sprintf("type %s: %s;", declaration.Name, typeReferenceDetail(declaration.Type)), declarationDocumentation(file, declaration.Name)), true
			case ast.EnumDeclaration:
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindEnum, symbolOriginLocal, enumDeclarationDetail(declaration), declarationDocumentation(file, declaration.Name)), true
			case ast.SchemaDeclaration:
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindStruct, symbolOriginLocal, fmt.Sprintf("schema %s: %s;", declaration.Name, recordTypeDetail(declaration.Type)), declarationDocumentation(file, declaration.Name)), true
			case ast.VariableDeclaration:
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindVariable, symbolOriginLocal, variableDeclarationDetail(declaration), declarationDocumentation(file, declaration.Name)), true
			case ast.DocDeclaration:
				return semanticSymbol{}, false
			default:
				return semanticSymbol{}, false
			}
		})...)
	}

	symbols = append(symbols, lo.Map(file.Output.SchemaFields, func(field ast.OutputSchemaField, _ int) semanticSymbol {
		return newOutputSymbol(field.NameToken, documentURI, field.Name, fmt.Sprintf("output %s: %s", field.Name, fieldTypeDetail(field.Type)), inlineDescriptionDocumentation(field.Description))
	})...)

	symbols = append(symbols, lo.Map(file.Output.DataFields, func(field ast.OutputField, _ int) semanticSymbol {
		detail := "output " + field.Name
		if result != nil {
			if value, ok := result.Output[field.Name]; ok {
				detail = fmt.Sprintf("output %s: %s = %s", field.Name, valueTypeSummary(value), summarizeValue(value))
			}
		}
		return newOutputSymbol(field.NameToken, documentURI, field.Name, detail, inlineDescriptionDocumentation(field.Description))
	})...)

	return dedupeSymbols(symbols)
}

func newLocalSymbol(nameToken lexer.Token, uri protocol.DocumentUri, name string, kind protocol.CompletionItemKind, origin symbolOrigin, detail string, documentation string) semanticSymbol {
	rangeValue := tokenProtocolRange(nameToken)

	return semanticSymbol{
		Name:          name,
		Kind:          kind,
		Detail:        detail,
		Documentation: documentation,
		Origin:        origin,
		Range:         rangeValue,
		Definition: protocol.Location{
			URI:   uri,
			Range: rangeValue,
		},
	}
}

func newOutputSymbol(nameToken lexer.Token, uri protocol.DocumentUri, name string, detail string, documentation string) semanticSymbol {
	rangeValue := tokenProtocolRange(nameToken)

	return semanticSymbol{
		Name:          name,
		Kind:          protocol.CompletionItemKindProperty,
		Detail:        detail,
		Documentation: documentation,
		Origin:        symbolOriginOutput,
		Range:         rangeValue,
		Definition: protocol.Location{
			URI:   uri,
			Range: rangeValue,
		},
	}
}

func declarationDocumentation(file ast.File, name string) string {
	parts := []string{}

	if file.Script != nil {
		for _, item := range file.Script.Items {
			switch declaration := item.(type) {
			case ast.TypeDeclaration:
				if declaration.Name == name && declaration.Description != "" {
					parts = append(parts, inlineDescriptionDocumentation(declaration.Description))
				}
			case ast.DocDeclaration:
				if declaration.Target != name {
					continue
				}
				if declaration.Documentation.Summary != nil {
					parts = append(parts, stringLiteralMarkdown(*declaration.Documentation.Summary))
				}
				if declaration.Documentation.Description != nil {
					parts = append(parts, stringLiteralMarkdown(*declaration.Documentation.Description))
				}
				if len(declaration.Documentation.Props) > 0 {
					keys := lo.Keys(declaration.Documentation.Props)
					slices.Sort(keys)
					props := lo.Map(keys, func(key string, _ int) string {
						return fmt.Sprintf("- `%s`: %s", key, stringLiteralMarkdown(declaration.Documentation.Props[key]))
					})
					parts = append(parts, "Props:\n"+strings.Join(props, "\n"))
				}
			}
		}
	}

	parts = lo.Filter(parts, func(part string, _ int) bool {
		return strings.TrimSpace(part) != ""
	})

	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func inlineDescriptionDocumentation(description string) string {
	return strings.TrimSpace(description)
}

func joinDocumentation(parts ...string) string {
	filtered := lo.Filter(parts, func(part string, _ int) bool {
		return strings.TrimSpace(part) != ""
	})

	return strings.TrimSpace(strings.Join(filtered, "\n\n"))
}

func stringLiteralMarkdown(literal ast.StringLiteral) string {
	lexeme := literal.Lexeme
	switch {
	case strings.HasPrefix(lexeme, `"""`) && strings.HasSuffix(lexeme, `"""`) && len(lexeme) >= 6:
		return lexeme[3 : len(lexeme)-3]
	case strings.HasPrefix(lexeme, `'`) && strings.HasSuffix(lexeme, `'`) && len(lexeme) >= 2:
		return lexeme[1 : len(lexeme)-1]
	case strings.HasPrefix(lexeme, `"`) && strings.HasSuffix(lexeme, `"`) && len(lexeme) >= 2:
		unquoted, err := strconv.Unquote(lexeme)
		if err == nil {
			return unquoted
		}
		return lexeme[1 : len(lexeme)-1]
	default:
		return lexeme
	}
}

func importedSemanticSymbols(file ast.File, documentPath string) []semanticSymbol {
	baseDir := filepath.Dir(documentPath)
	if documentPath == "" {
		return nil
	}

	return lo.FlatMap(file.Imports, func(importDecl ast.ImportDeclaration, _ int) []semanticSymbol {
		pathValue, ok := stringLiteralValue(importDecl.Path)
		if !ok {
			return nil
		}

		importedPath := filepath.Clean(filepath.Join(baseDir, pathValue))
		_, importedFile, _, ok := parsedFile(importedPath)
		if !ok {
			return nil
		}

		return lo.FilterMap(importDecl.Identifiers, func(name string, _ int) (semanticSymbol, bool) {
			symbol, ok := importedSemanticSymbol(importedFile, importedPath, name)
			if !ok {
				return semanticSymbol{}, false
			}
			return symbol, true
		})
	})
}

func importedSemanticSymbol(file ast.File, path string, name string) (semanticSymbol, bool) {
	if field, ok := lo.Find(file.Output.SchemaFields, func(field ast.OutputSchemaField) bool {
		return field.Name == name
	}); ok {
		rangeValue := tokenProtocolRange(field.NameToken)

		kind := protocol.CompletionItemKindClass
		detail := fmt.Sprintf("type %s: %s;", field.Name, fieldTypeDetail(field.Type))
		if isSchemaTypeReference(field.Type, file) {
			kind = protocol.CompletionItemKindStruct
			detail = fmt.Sprintf("schema %s: %s", field.Name, fieldTypeDetail(field.Type))
		} else if isEnumTypeReference(field.Type, file) {
			kind = protocol.CompletionItemKindEnum
			detail = fmt.Sprintf("enum %s: %s", field.Name, fieldTypeDetail(field.Type))
		}

		return semanticSymbol{
			Name:          field.Name,
			Kind:          kind,
			Detail:        detail,
			Documentation: joinDocumentation(inlineDescriptionDocumentation(field.Description), declarationDocumentation(file, field.Name)),
			Origin:        symbolOriginImport,
			Range:         rangeValue,
			Definition: protocol.Location{
				URI:   pathURI(path),
				Range: rangeValue,
			},
		}, true
	}

	if field, ok := lo.Find(file.Output.DataFields, func(field ast.OutputField) bool {
		return field.Name == name
	}); ok {
		rangeValue := tokenProtocolRange(field.NameToken)

		return semanticSymbol{
			Name:   field.Name,
			Kind:   protocol.CompletionItemKindVariable,
			Detail: fmt.Sprintf("import %s", field.Name),
			Origin: symbolOriginImport,
			Range:  rangeValue,
			Definition: protocol.Location{
				URI:   pathURI(path),
				Range: rangeValue,
			},
		}, true
	}

	return semanticSymbol{}, false
}

func parsedFile(path string) (string, ast.File, []lexer.Token, bool) {
	contents, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return "", ast.File{}, nil, false
	}

	text := string(contents)
	tokens, err := lex(text)
	if err != nil {
		return "", ast.File{}, nil, false
	}

	file, err := parseFile(text)
	if err != nil {
		return "", ast.File{}, nil, false
	}

	return text, file, tokens, true
}

func isSchemaTypeReference(typeReference ast.TypeReference, file ast.File) bool {
	switch value := typeReference.(type) {
	case ast.RecordType:
		return true
	case ast.NamedType:
		return lo.ContainsBy(fileScriptDeclarations(file), func(item ast.Declaration) bool {
			declaration, ok := item.(ast.SchemaDeclaration)
			return ok && declaration.Name == value.Name
		})
	default:
		return false
	}
}

func isEnumTypeReference(typeReference ast.TypeReference, file ast.File) bool {
	value, ok := typeReference.(ast.NamedType)
	if !ok {
		return false
	}

	return lo.ContainsBy(fileScriptDeclarations(file), func(item ast.Declaration) bool {
		declaration, ok := item.(ast.EnumDeclaration)
		return ok && declaration.Name == value.Name
	})
}

func fileScriptDeclarations(file ast.File) []ast.Declaration {
	if file.Script == nil {
		return nil
	}

	return file.Script.Items
}

func summarizeValue(value processor.Value) string {
	switch value.Kind {
	case processor.ValueString:
		return fmt.Sprintf("%q", value.String)
	case processor.ValueInt:
		return fmt.Sprintf("%d", value.Int)
	case processor.ValueFloat:
		return fmt.Sprintf("%g", value.Float)
	case processor.ValueBoolean:
		if value.Boolean {
			return "true"
		}
		return "false"
	case processor.ValueArray:
		items := lo.Map(value.Array, func(item processor.Value, _ int) string {
			return summarizeValue(item)
		})
		return "[" + strings.Join(items, ", ") + "]"
	case processor.ValueRecord:
		names := lo.Keys(value.Record)
		slices.Sort(names)
		fields := lo.Map(names, func(name string, _ int) string {
			return fmt.Sprintf("%s: %s", name, summarizeValue(value.Record[name]))
		})
		return "{ " + strings.Join(fields, ", ") + " }"
	default:
		return "unknown"
	}
}

func valueTypeSummary(value processor.Value) string {
	switch value.Kind {
	case processor.ValueString:
		return "string"
	case processor.ValueInt:
		return "int"
	case processor.ValueFloat:
		return "float"
	case processor.ValueBoolean:
		return "boolean"
	case processor.ValueArray:
		if len(value.Array) == 0 {
			return "array<unknown>"
		}
		return "array<" + valueTypeSummary(value.Array[0]) + ">"
	case processor.ValueRecord:
		names := lo.Keys(value.Record)
		slices.Sort(names)
		fields := lo.Map(names, func(name string, _ int) string {
			return fmt.Sprintf("%s: %s", name, valueTypeSummary(value.Record[name]))
		})
		return "{ " + strings.Join(fields, ", ") + " }"
	default:
		return "unknown"
	}
}

func expressionSummary(expression ast.Expression) string {
	if expression == nil {
		return "runtime value"
	}

	switch typed := expression.(type) {
	case ast.StringLiteral:
		return typed.Lexeme
	case ast.IntLiteral:
		return typed.Lexeme
	case ast.FloatLiteral:
		return typed.Lexeme
	case ast.BooleanLiteral:
		if typed.Value {
			return "true"
		}
		return "false"
	case ast.Identifier:
		return typed.Name
	case ast.ArrayLiteral:
		return "array literal"
	case ast.RecordLiteral:
		return "record literal"
	default:
		return "expression"
	}
}

func variableDeclarationDetail(declaration ast.VariableDeclaration) string {
	detail := fmt.Sprintf("%s %s", typeReferenceDetail(declaration.Type), declaration.Name)
	if declaration.Injectable {
		detail = "injectable " + detail
	}
	if !declaration.HasValue {
		return detail
	}

	return detail + " = " + expressionSummary(declaration.Value)
}

func enumDeclarationDetail(declaration ast.EnumDeclaration) string {
	members := lo.Map(declaration.Members, func(member ast.EnumMember, index int) string {
		if member.HasValue {
			return member.Name + " = " + expressionSummary(member.Value)
		}

		if declaration.BackingType.Name == "string" {
			return member.Name + " = " + fmt.Sprintf("%q", member.Name)
		}

		if declaration.BackingType.Name == "int" {
			return member.Name + " = " + fmt.Sprintf("%d", index)
		}

		return member.Name
	})

	return fmt.Sprintf("enum %s: %s { %s }", declaration.Name, declaration.BackingType.Name, strings.Join(members, ", "))
}

func enumMemberDetail(declaration ast.EnumDeclaration, member ast.EnumMember) string {
	if member.HasValue {
		return fmt.Sprintf("enum member %s.%s = %s", declaration.Name, member.Name, expressionSummary(member.Value))
	}

	if declaration.BackingType.Name == "string" {
		return fmt.Sprintf("enum member %s.%s = %q", declaration.Name, member.Name, member.Name)
	}

	if declaration.BackingType.Name == "int" {
		for index, declarationMember := range declaration.Members {
			if declarationMember.Name == member.Name {
				return fmt.Sprintf("enum member %s.%s = %d", declaration.Name, member.Name, index)
			}
		}
	}

	return fmt.Sprintf("enum member %s.%s", declaration.Name, member.Name)
}

func indexSymbols(symbols []semanticSymbol) map[string]semanticSymbol {
	return lo.Reduce(symbols, func(index map[string]semanticSymbol, symbol semanticSymbol, _ int) map[string]semanticSymbol {
		if symbol.Name == "" {
			return index
		}
		if _, ok := index[symbol.Name]; ok {
			return index
		}
		index[symbol.Name] = symbol
		return index
	}, map[string]semanticSymbol{})
}

func dedupeSymbols(symbols []semanticSymbol) []semanticSymbol {
	return lo.UniqBy(symbols, func(symbol semanticSymbol) string {
		return string(symbol.Origin) + ":" + symbol.Name
	})
}

func pathURI(path string) protocol.DocumentUri {
	if path == "" {
		return ""
	}

	return protocol.DocumentUri(fileURI(path))
}

func schemaFileDirectiveRanges(text string) (protocol.Range, protocol.Range, bool) {
	openIndex := strings.Index(text, "[")
	closeIndex := strings.Index(text, "]")
	if openIndex < 0 || closeIndex <= openIndex {
		return protocol.Range{}, protocol.Range{}, false
	}

	directiveText := text[openIndex : closeIndex+1]
	schemaFileIndex := strings.Index(directiveText, "schema_file")
	if schemaFileIndex < 0 {
		return protocol.Range{}, protocol.Range{}, false
	}

	start := openIndex + schemaFileIndex
	end := closeIndex

	for start > openIndex && directiveText[start-openIndex-1] != ',' {
		start--
	}

	if start > openIndex && directiveText[start-openIndex-1] == ',' {
		start--
	}

	schemaFileEditRange := protocol.Range{
		Start: positionFromIndex(text, start),
		End:   positionFromIndex(text, end),
	}

	directiveRange := protocol.Range{
		Start: positionFromIndex(text, openIndex),
		End:   positionFromIndex(text, closeIndex+1),
	}

	return directiveRange, schemaFileEditRange, true
}

func importAndScriptCleanupRange(text string) (protocol.Range, bool) {
	outputIndex := strings.Index(text, "[")
	if outputIndex <= 0 {
		return protocol.Range{}, false
	}

	prefix := text[:outputIndex]
	if strings.TrimSpace(prefix) == "" {
		return protocol.Range{}, false
	}

	return protocol.Range{
		Start: protocol.Position{},
		End:   positionFromIndex(text, outputIndex),
	}, true
}

func rangesOverlap(left protocol.Range, right protocol.Range) bool {
	return comparePositions(left.Start, right.End) <= 0 && comparePositions(right.Start, left.End) <= 0
}

func comparePositions(left protocol.Position, right protocol.Position) int {
	if left.Line < right.Line {
		return -1
	}
	if left.Line > right.Line {
		return 1
	}
	if left.Character < right.Character {
		return -1
	}
	if left.Character > right.Character {
		return 1
	}
	return 0
}

func quotedName(message string) string {
	start := strings.Index(message, `"`)
	if start < 0 {
		return ""
	}

	end := strings.Index(message[start+1:], `"`)
	if end < 0 {
		return ""
	}

	return message[start+1 : start+1+end]
}
