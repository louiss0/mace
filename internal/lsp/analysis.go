package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
)

type analysisSnapshot struct {
	file         *ast.File
	result       *processor.Result
	diagnostics  []protocol.Diagnostic
	declarations []declarationDefinition
}

func analyzeDocument(text string) analysisSnapshot {
	return analyzeDocumentAt(text, "")
}

func analyzeDocumentAt(text string, documentPath string) analysisSnapshot {
	snapshot := analysisSnapshot{}

	file, parseErr := parseFile(text)
	if parseErr != nil {
		snapshot.diagnostics = []protocol.Diagnostic{diagnosticFromError(parseErr)}
		return snapshot
	}

	tokens, lexErr := lex(text)
	if lexErr != nil {
		snapshot.diagnostics = []protocol.Diagnostic{diagnosticFromError(lexErr)}
		return snapshot
	}

	snapshot.file = &file

	processorInstance := processor.New()
	baseDir := filepath.Dir(documentPath)
	if documentPath == "" {
		baseDir = ""
	}

	result, processErr := processorInstance.ProcessInDir(text, baseDir)
	if processErr != nil {
		snapshot.diagnostics = []protocol.Diagnostic{diagnosticFromError(processErr)}
		if semanticDiagnostic, ok := semanticDiagnosticFromError(file, tokens, processErr); ok {
			snapshot.diagnostics = []protocol.Diagnostic{semanticDiagnostic}
		}
		snapshot.declarations = collectDeclarations(file, nil, filepath.Dir(documentPath))
		return snapshot
	}

	snapshot.result = &result
	snapshot.declarations = collectDeclarations(file, &result, filepath.Dir(documentPath))

	return snapshot
}

func semanticDiagnosticFromError(file ast.File, tokens []lexer.Token, err error) (protocol.Diagnostic, bool) {
	if diagnostic, ok := variableTypeMismatchDiagnostic(file, tokens, err.Error()); ok {
		return diagnostic, true
	}

	if diagnostic, ok := schemaDiagnostic(tokens, err.Error()); ok {
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

		return protocol.Diagnostic{
			Severity: Ptr(protocol.DiagnosticSeverityError),
			Source:   Ptr(serverName),
			Message:  message,
			Range:    rangeValue,
		}, true
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

		return protocol.Diagnostic{
			Severity: Ptr(protocol.DiagnosticSeverityError),
			Source:   Ptr(serverName),
			Message:  message,
			Range:    rangeValue,
		}, true
	}

	return protocol.Diagnostic{}, false
}

func tokenRange(tokens []lexer.Token, lexeme string) (protocol.Range, bool) {
	token, found := lo.Find(tokens, func(token lexer.Token) bool {
		return token.Type == lexer.TokenIdentifier && token.Lexeme == lexeme
	})
	if !found {
		return protocol.Range{}, false
	}

	start := protocol.Position{Line: protocol.UInteger(token.Line - 1), Character: protocol.UInteger(token.Column - 1)}
	end := protocol.Position{Line: protocol.UInteger(token.Line - 1), Character: protocol.UInteger(token.Column - 1 + len(token.Lexeme))}
	return protocol.Range{Start: start, End: end}, true
}

func collectDeclarations(file ast.File, result *processor.Result, baseDir string) []declarationDefinition {
	declarations := importedDeclarationDefinitions(file, baseDir)
	if file.Script != nil {
		declarations = append(declarations, lo.FilterMap(file.Script.Items, func(item ast.Declaration, _ int) (declarationDefinition, bool) {
			switch declaration := item.(type) {
			case ast.TypeDeclaration:
				return declarationDefinition{
					Name:   declaration.Name,
					Kind:   protocol.CompletionItemKindClass,
					Detail: fmt.Sprintf("type %s = %s;", declaration.Name, typeReferenceDetail(declaration.Type)),
				}, true
			case ast.SchemaDeclaration:
				return declarationDefinition{
					Name:   declaration.Name,
					Kind:   protocol.CompletionItemKindStruct,
					Detail: fmt.Sprintf("schema %s = %s;", declaration.Name, recordTypeDetail(declaration.Type)),
				}, true
			case ast.VariableDeclaration:
				return declarationDefinition{
					Name:   declaration.Name,
					Kind:   protocol.CompletionItemKindVariable,
					Detail: fmt.Sprintf("%s %s = %s", typeReferenceDetail(declaration.Type), declaration.Name, expressionSummary(declaration.Value)),
				}, true
			default:
				return declarationDefinition{}, false
			}
		})...)
	}

	if result == nil {
		return declarations
	}

	return append(declarations, lo.FilterMap(file.Output.DataFields, func(field ast.OutputField, _ int) (declarationDefinition, bool) {
		value, ok := result.Output[field.Name]
		if !ok {
			return declarationDefinition{}, false
		}

		return declarationDefinition{
			Name:   field.Name,
			Kind:   protocol.CompletionItemKindProperty,
			Detail: fmt.Sprintf("output %s: %s = %s", field.Name, valueTypeSummary(value), summarizeValue(value)),
		}, true
	})...)
}

func importedDeclarationDefinitions(file ast.File, baseDir string) []declarationDefinition {
	if baseDir == "" {
		return nil
	}

	return lo.FlatMap(file.Imports, func(importDecl ast.ImportDeclaration, _ int) []declarationDefinition {
		pathValue, ok := stringLiteralValue(importDecl.Path)
		if !ok {
			return nil
		}

		importedFile, ok := importedFile(filepath.Join(baseDir, pathValue))
		if !ok {
			return nil
		}

		return lo.FilterMap(importDecl.Identifiers, func(name string, _ int) (declarationDefinition, bool) {
			return importedDeclarationDefinition(importedFile, name)
		})
	})
}

func importedFile(path string) (ast.File, bool) {
	contents, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return ast.File{}, false
	}

	file, err := parseFile(string(contents))
	if err != nil {
		return ast.File{}, false
	}

	return file, true
}

func importedDeclarationDefinition(file ast.File, name string) (declarationDefinition, bool) {
	if field, ok := lo.Find(file.Output.SchemaFields, func(field ast.OutputSchemaField) bool {
		return field.Name == name
	}); ok {
		kind := protocol.CompletionItemKindClass
		detail := fmt.Sprintf("type %s = %s;", field.Name, fieldTypeDetail(field.Type))
		if isSchemaTypeReference(field.Type, file) {
			kind = protocol.CompletionItemKindStruct
			detail = fmt.Sprintf("schema %s = %s;", field.Name, fieldTypeDetail(field.Type))
		}

		return declarationDefinition{
			Name:   field.Name,
			Kind:   kind,
			Detail: detail,
		}, true
	}

	if field, ok := lo.Find(file.Output.DataFields, func(field ast.OutputField) bool {
		return field.Name == name
	}); ok {
		return declarationDefinition{
			Name:   field.Name,
			Kind:   protocol.CompletionItemKindVariable,
			Detail: fmt.Sprintf("import %s", field.Name),
		}, true
	}

	return declarationDefinition{}, false
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
