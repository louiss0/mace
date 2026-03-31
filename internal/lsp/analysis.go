package lsp

import (
	"fmt"
	"sort"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
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

	snapshot.file = &file

	processorInstance := processor.New()
	result, processErr := processorInstance.Process(text)
	if processErr != nil {
		snapshot.diagnostics = []protocol.Diagnostic{diagnosticFromError(processErr)}
		snapshot.declarations = collectDeclarations(file, nil)
		return snapshot
	}

	snapshot.result = &result
	snapshot.declarations = collectDeclarations(file, &result)

	return snapshot
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
				detail := fmt.Sprintf("%s %s", typeReferenceDetail(declaration.Type), declaration.Name)
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
				Detail: "output " + field.Name + " = " + summarizeValue(value),
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
