package lsp

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
)

type symbolOrigin string

const (
	symbolOriginLocal  symbolOrigin = "local"
	symbolOriginImport symbolOrigin = "import"
	symbolOriginOutput symbolOrigin = "output"
)

type semanticSymbol struct {
	Name       string
	Kind       protocol.CompletionItemKind
	Detail     string
	Origin     symbolOrigin
	Range      protocol.Range
	Definition protocol.Location
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

	if diagnostic, candidates, ok := schemaFileConflictAnalysis(text, file, documentPath); ok {
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, candidates...)
	}

	diagnostics = append(diagnostics, schemaOutputVariableDiagnostics(file, tokens)...)

	return diagnostics, actions
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
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindClass, symbolOriginLocal, fmt.Sprintf("type %s = %s;", declaration.Name, typeReferenceDetail(declaration.Type))), true
			case ast.EnumDeclaration:
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindEnum, symbolOriginLocal, enumDeclarationDetail(declaration)), true
			case ast.SchemaDeclaration:
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindStruct, symbolOriginLocal, fmt.Sprintf("schema %s = %s;", declaration.Name, recordTypeDetail(declaration.Type))), true
			case ast.VariableDeclaration:
				return newLocalSymbol(declaration.NameToken, documentURI, declaration.Name, protocol.CompletionItemKindVariable, symbolOriginLocal, variableDeclarationDetail(declaration)), true
			default:
				return semanticSymbol{}, false
			}
		})...)
	}

	symbols = append(symbols, lo.Map(file.Output.SchemaFields, func(field ast.OutputSchemaField, _ int) semanticSymbol {
		return newOutputSymbol(field.NameToken, documentURI, field.Name, fmt.Sprintf("output %s: %s", field.Name, fieldTypeDetail(field.Type)))
	})...)

	symbols = append(symbols, lo.Map(file.Output.DataFields, func(field ast.OutputField, _ int) semanticSymbol {
		detail := "output " + field.Name
		if result != nil {
			if value, ok := result.Output[field.Name]; ok {
				detail = fmt.Sprintf("output %s: %s = %s", field.Name, valueTypeSummary(value), summarizeValue(value))
			}
		}
		return newOutputSymbol(field.NameToken, documentURI, field.Name, detail)
	})...)

	return dedupeSymbols(symbols)
}

func newLocalSymbol(nameToken lexer.Token, uri protocol.DocumentUri, name string, kind protocol.CompletionItemKind, origin symbolOrigin, detail string) semanticSymbol {
	rangeValue := tokenProtocolRange(nameToken)

	return semanticSymbol{
		Name:   name,
		Kind:   kind,
		Detail: detail,
		Origin: origin,
		Range:  rangeValue,
		Definition: protocol.Location{
			URI:   uri,
			Range: rangeValue,
		},
	}
}

func newOutputSymbol(nameToken lexer.Token, uri protocol.DocumentUri, name string, detail string) semanticSymbol {
	rangeValue := tokenProtocolRange(nameToken)

	return semanticSymbol{
		Name:   name,
		Kind:   protocol.CompletionItemKindProperty,
		Detail: detail,
		Origin: symbolOriginOutput,
		Range:  rangeValue,
		Definition: protocol.Location{
			URI:   uri,
			Range: rangeValue,
		},
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
		detail := fmt.Sprintf("type %s = %s;", field.Name, fieldTypeDetail(field.Type))
		if isSchemaTypeReference(field.Type, file) {
			kind = protocol.CompletionItemKindStruct
			detail = fmt.Sprintf("schema %s = %s;", field.Name, fieldTypeDetail(field.Type))
		} else if isEnumTypeReference(field.Type, file) {
			kind = protocol.CompletionItemKindEnum
			detail = fmt.Sprintf("enum %s: %s;", field.Name, fieldTypeDetail(field.Type))
		}

		return semanticSymbol{
			Name:   field.Name,
			Kind:   kind,
			Detail: detail,
			Origin: symbolOriginImport,
			Range:  rangeValue,
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
		return "{ " + strings.Join(fields, "; ") + " }"
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
		return "{ " + strings.Join(fields, "; ") + " }"
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
	members := lo.Map(declaration.Members, func(member ast.EnumMember, _ int) string {
		if !member.HasValue {
			return member.Name
		}

		return member.Name + " = " + expressionSummary(member.Value)
	})

	return fmt.Sprintf("enum %s: %s { %s }", declaration.Name, declaration.BackingType.Name, strings.Join(members, ", "))
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
