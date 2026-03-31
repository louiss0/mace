package lsp

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
)

var (
	missingRequiredFieldPattern = regexp.MustCompile(`missing required field "([^"]+)" for schema "([^"]+)"`)
)

type analysisSnapshot struct {
	file         *ast.File
	result       *processor.Result
	diagnostics  []protocol.Diagnostic
	declarations []declarationDefinition
}

func analyzeDocument(text string) analysisSnapshot {
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
	result, processErr := processorInstance.Process(text)
	if processErr != nil {
		snapshot.diagnostics = []protocol.Diagnostic{diagnosticFromError(processErr)}
		if semanticDiagnostic, ok := semanticDiagnosticFromError(file, tokens, processErr); ok {
			snapshot.diagnostics = []protocol.Diagnostic{semanticDiagnostic}
		}
		snapshot.declarations = collectDeclarations(file, nil)
		return snapshot
	}

	snapshot.result = &result
	snapshot.declarations = collectDeclarations(file, &result)

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
	if file.Script == nil || !strings.Contains(message, "type mismatch") {
		return protocol.Diagnostic{}, false
	}

	for _, item := range file.Script.Items {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok {
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
	for _, token := range tokens {
		if token.Type != lexer.TokenIdentifier || token.Lexeme != lexeme {
			continue
		}

		start := protocol.Position{Line: protocol.UInteger(token.Line - 1), Character: protocol.UInteger(token.Column - 1)}
		end := protocol.Position{Line: protocol.UInteger(token.Line - 1), Character: protocol.UInteger(token.Column - 1 + len(token.Lexeme))}
		return protocol.Range{Start: start, End: end}, true
	}

	return protocol.Range{}, false
}

func collectDeclarations(file ast.File, result *processor.Result) []declarationDefinition {
	declarations := []declarationDefinition{}
	if file.Script != nil {
		for _, item := range file.Script.Items {
			switch declaration := item.(type) {
			case ast.TypeDeclaration:
				declarations = append(declarations, declarationDefinition{
					Name:   declaration.Name,
					Kind:   protocol.CompletionItemKindClass,
					Detail: fmt.Sprintf("type %s = %s;", declaration.Name, typeReferenceDetail(declaration.Type)),
				})
			case ast.SchemaDeclaration:
				declarations = append(declarations, declarationDefinition{
					Name:   declaration.Name,
					Kind:   protocol.CompletionItemKindStruct,
					Detail: fmt.Sprintf("schema %s = %s;", declaration.Name, recordTypeDetail(declaration.Type)),
				})
			case ast.VariableDeclaration:
				detail := fmt.Sprintf("%s %s = %s", typeReferenceDetail(declaration.Type), declaration.Name, expressionSummary(declaration.Value))
				declarations = append(declarations, declarationDefinition{
					Name:   declaration.Name,
					Kind:   protocol.CompletionItemKindVariable,
					Detail: detail,
				})
			}
		}
	}

	if result != nil {
		for _, field := range file.Output.DataFields {
			value, ok := result.Output[field.Name]
			if !ok {
				continue
			}
			declarations = append(declarations, declarationDefinition{
				Name:   field.Name,
				Kind:   protocol.CompletionItemKindProperty,
				Detail: fmt.Sprintf("output %s: %s = %s", field.Name, valueTypeSummary(value), summarizeValue(value)),
			})
		}
	}

	return declarations
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
		items := make([]string, 0, len(value.Array))
		for _, item := range value.Array {
			items = append(items, summarizeValue(item))
		}
		return "[" + strings.Join(items, ", ") + "]"
	case processor.ValueRecord:
		names := make([]string, 0, len(value.Record))
		for name := range value.Record {
			names = append(names, name)
		}
		sort.Strings(names)
		fields := make([]string, 0, len(names))
		for _, name := range names {
			fields = append(fields, fmt.Sprintf("%s: %s", name, summarizeValue(value.Record[name])))
		}
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
		names := make([]string, 0, len(value.Record))
		for name := range value.Record {
			names = append(names, name)
		}
		sort.Strings(names)
		fields := make([]string, 0, len(names))
		for _, name := range names {
			fields = append(fields, fmt.Sprintf("%s: %s", name, valueTypeSummary(value.Record[name])))
		}
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
