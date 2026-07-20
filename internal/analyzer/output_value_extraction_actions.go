package analyzer

import (
	"strconv"
	"strings"
	"unicode"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

func outputValueExtractionCodeActions(text string, file ast.File, tokens []lexer.Token, documentPath string) []analysisCodeActionCandidate {
	if file.Output.Mode != ast.OutputModeData {
		return nil
	}

	declaredNames, variableTypes := outputExtractionDeclarations(file)
	actions := []analysisCodeActionCandidate{}
	for _, field := range file.Output.DataFields {
		if _, isIdentifier := field.Value.(ast.Identifier); isIdentifier {
			continue
		}
		fieldRange, valueText, ok := outputFieldSource(text, tokens, field)
		if !ok {
			continue
		}

		variableName := nextOutputValueName(field.Name, declaredNames)
		declaration, title, ok := outputValueDeclaration(field, variableName, valueText, declaredNames, variableTypes)
		if !ok {
			continue
		}
		updated := text[:fieldRange.startIndex] + field.Name + ": " + variableName + text[fieldRange.endIndex:]
		updated, ok = insertScriptDeclaration(updated, declaration)
		if !ok {
			continue
		}
		actions = append(actions, outputValueExtractionAction(text, documentPath, field.NameToken, title, updated))
	}

	return actions
}

type outputFieldSourceRange struct {
	startIndex int
	endIndex   int
}

func outputFieldSource(text string, tokens []lexer.Token, field ast.OutputField) (outputFieldSourceRange, string, bool) {
	fieldIndex := -1
	for index, token := range tokens {
		if token.Line == field.NameToken.Line && token.Column == field.NameToken.Column && token.Lexeme == field.NameToken.Lexeme {
			fieldIndex = index
			break
		}
	}
	if fieldIndex < 0 || fieldIndex+1 >= len(tokens) || tokens[fieldIndex+1].Type != lexer.TokenColon {
		return outputFieldSourceRange{}, "", false
	}

	valueIndex := fieldIndex + 2
	depth := 0
	for index := valueIndex; index < len(tokens); index++ {
		switch tokens[index].Type {
		case lexer.TokenLBracket, lexer.TokenLBrace, lexer.TokenLParen:
			depth++
		case lexer.TokenRBracket, lexer.TokenRParen:
			if depth > 0 {
				depth--
			}
		case lexer.TokenRBrace:
			if depth == 0 {
				return outputFieldSourceRange{startIndex: tokenStartIndex(text, tokens[fieldIndex]), endIndex: tokenStartIndex(text, tokens[index])}, strings.TrimSpace(text[tokenStartIndex(text, tokens[valueIndex]):tokenStartIndex(text, tokens[index])]), true
			}
			depth--
		case lexer.TokenComma, lexer.TokenInlineDescription:
			if depth == 0 {
				return outputFieldSourceRange{startIndex: tokenStartIndex(text, tokens[fieldIndex]), endIndex: tokenStartIndex(text, tokens[index])}, strings.TrimSpace(text[tokenStartIndex(text, tokens[valueIndex]):tokenStartIndex(text, tokens[index])]), true
			}
		}
	}
	return outputFieldSourceRange{}, "", false
}

func outputValueDeclaration(field ast.OutputField, variableName string, valueText string, declaredNames map[string]bool, variableTypes map[string]string) (string, string, bool) {
	if record, isRecord := field.Value.(ast.RecordLiteral); isRecord {
		fields, uniformType, uniform := outputRecordTypes(record, variableTypes)
		if len(fields) == 0 {
			return "", "", false
		}
		if uniform {
			return "record<" + uniformType + "> " + variableName + " = " + valueText + ";", "Extract output record into script variable", true
		}
		schemaName := nextOutputSchemaName(field.Name, declaredNames)
		return "schema " + schemaName + ": { " + strings.Join(fields, ", ") + ", };\n" + schemaName + " " + variableName + " = " + valueText + ";", "Extract output record into schema", true
	}

	typeName, ok := outputExpressionType(field.Value, variableTypes)
	if !ok {
		return "", "", false
	}
	return typeName + " " + variableName + " = " + valueText + ";", "Extract output value into script variable", true
}

func outputRecordTypes(record ast.RecordLiteral, variableTypes map[string]string) ([]string, string, bool) {
	fields := []string{}
	fieldTypes := []string{}
	for _, field := range record.Fields {
		typeName, ok := outputExpressionType(field.Value, variableTypes)
		if !ok {
			return nil, "", false
		}
		fields = append(fields, field.Name+": "+typeName)
		fieldTypes = append(fieldTypes, typeName)
	}
	if len(fieldTypes) == 0 {
		return fields, "", false
	}
	for _, typeName := range fieldTypes[1:] {
		if typeName != fieldTypes[0] {
			return fields, "", false
		}
	}
	return fields, fieldTypes[0], true
}

func outputExpressionType(expression ast.Expression, variableTypes map[string]string) (string, bool) {
	switch typed := expression.(type) {
	case ast.StringLiteral:
		return "string", true
	case ast.IntLiteral:
		return "int", true
	case ast.FloatLiteral:
		return "float", true
	case ast.HexIntLiteral:
		return "hex_int", true
	case ast.HexFloatLiteral:
		return "hex_float", true
	case ast.BooleanLiteral:
		return "boolean", true
	case ast.Identifier:
		typeName, ok := variableTypes[typed.Name]
		return typeName, ok
	case ast.ArrayLiteral:
		return outputArrayType(typed, variableTypes)
	case ast.RecordLiteral:
		fields, uniformType, uniform := outputRecordTypes(typed, variableTypes)
		if uniform {
			return "record<" + uniformType + ">", true
		}
		return "{ " + strings.Join(fields, ", ") + ", }", len(fields) > 0
	default:
		return "", false
	}
}

func outputArrayType(array ast.ArrayLiteral, variableTypes map[string]string) (string, bool) {
	if len(array.Elements) == 0 {
		return "", false
	}
	types := []string{}
	for _, element := range array.Elements {
		typeName, ok := outputExpressionType(element, variableTypes)
		if !ok {
			return "", false
		}
		if !containsString(types, typeName) {
			types = append(types, typeName)
		}
	}
	if len(types) == 1 {
		return "array<" + types[0] + ">", true
	}
	return "array<variant[" + strings.Join(types, ", ") + "]>", true
}

func outputExtractionDeclarations(file ast.File) (map[string]bool, map[string]string) {
	declaredNames := map[string]bool{}
	variableTypes := map[string]string{}
	if file.Script == nil {
		return declaredNames, variableTypes
	}
	for _, item := range file.Script.Items {
		switch declaration := item.(type) {
		case ast.VariableDeclaration:
			declaredNames[declaration.Name] = true
			variableTypes[declaration.Name] = typeReferenceDetail(declaration.Type)
		case ast.TypeDeclaration:
			declaredNames[declaration.Name] = true
		case ast.SchemaDeclaration:
			declaredNames[declaration.Name] = true
		}
	}
	return declaredNames, variableTypes
}

func nextOutputValueName(fieldName string, declaredNames map[string]bool) string {
	if !declaredNames[fieldName] {
		return fieldName
	}
	for suffix := 2; ; suffix++ {
		candidate := fieldName + "Value" + strconv.Itoa(suffix)
		if !declaredNames[candidate] {
			return candidate
		}
	}
}

func nextOutputSchemaName(fieldName string, declaredNames map[string]bool) string {
	baseName := upperFirst(fieldName)
	if !declaredNames[baseName] {
		return baseName
	}
	for suffix := 2; ; suffix++ {
		candidate := baseName + "Schema" + strconv.Itoa(suffix)
		if !declaredNames[candidate] {
			return candidate
		}
	}
}

func upperFirst(value string) string {
	for index, runeValue := range value {
		return string(unicode.ToUpper(runeValue)) + value[index+len(string(runeValue)):]
	}
	return value
}

func insertScriptDeclaration(text string, declaration string) (string, bool) {
	fences := scriptFencePattern.FindAllStringIndex(text, -1)
	if len(fences) >= 2 {
		return text[:fences[1][0]] + declaration + "\n" + text[fences[1][0]:], true
	}
	if outputStart := strings.Index(text, "[output"); outputStart >= 0 {
		return "|===|\n" + declaration + "\n|===|\n" + text[:outputStart] + text[outputStart:], true
	}
	return "", false
}

func outputValueExtractionAction(text string, documentPath string, token lexer.Token, title string, updated string) analysisCodeActionCandidate {
	rangeValue := tokenProtocolRange(token)
	return analysisCodeActionCandidate{
		Range: rangeValue,
		Action: protocol.CodeAction{
			Title: title,
			Kind:  Ptr(protocol.CodeActionKindRefactorExtract),
			Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{
				pathURI(documentPath): {{Range: fullDocumentRange(text), NewText: updated}},
			}},
		},
	}
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
