package analyzer

import (
	"unicode"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

func unusedVariableExportActions(text string, file ast.File, tokens []lexer.Token, documentPath string, diagnostic protocol.Diagnostic, declaration ast.VariableDeclaration) []analysisCodeActionCandidate {
	if file.Output.Mode != ast.OutputModeData {
		return nil
	}

	fieldText := declaration.Name + ": " + declaration.Name + ","
	actions := []analysisCodeActionCandidate{}
	if updated, ok := appendToOutputBlock(text, tokens, fieldText); ok {
		actions = append(actions, unusedDeclarationExportAction(text, documentPath, diagnostic, "Export unused variable from output", updated))
	}
	if record, ok := nearestOutputRecord(file, declaration.NameToken); ok {
		if updated, ok := appendToDelimitedBlock(text, tokens, record.StartToken, fieldText); ok {
			actions = append(actions, unusedDeclarationExportAction(text, documentPath, diagnostic, "Add unused variable to nearest output record", updated))
		}
	}

	return actions
}

func unusedTypeExportActions(text string, file ast.File, tokens []lexer.Token, documentPath string, diagnostic protocol.Diagnostic, declaration ast.TypeDeclaration) []analysisCodeActionCandidate {
	fieldText := lowerFirst(declaration.Name) + ": " + declaration.Name + ","
	actions := []analysisCodeActionCandidate{}
	if file.Output.Mode == ast.OutputModeSchema {
		if updated, ok := appendToOutputBlock(text, tokens, declaration.Name+": "+declaration.Name+","); ok {
			actions = append(actions, unusedDeclarationExportAction(text, documentPath, diagnostic, "Export unused type from output", updated))
		}
	}
	if schema, ok := nearestSchema(file, declaration.NameToken); ok {
		if updated, ok := appendToDelimitedBlock(text, tokens, schema.StartToken, fieldText); ok {
			actions = append(actions, unusedDeclarationExportAction(text, documentPath, diagnostic, "Add unused type to nearest schema", updated))
		}
	}

	return actions
}

func unusedDeclarationExportAction(text string, documentPath string, diagnostic protocol.Diagnostic, title string, updated string) analysisCodeActionCandidate {
	return analysisCodeActionCandidate{
		Range: diagnostic.Range,
		Action: protocol.CodeAction{
			Title:       title,
			Kind:        Ptr(protocol.CodeActionKindRefactorRewrite),
			IsPreferred: Ptr(false),
			Diagnostics: []protocol.Diagnostic{diagnostic},
			Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{
				pathURI(documentPath): {{Range: fullDocumentRange(text), NewText: updated}},
			}},
		},
	}
}

func appendToOutputBlock(text string, tokens []lexer.Token, fieldText string) (string, bool) {
	for tokenIndex, token := range tokens {
		if token.Type != lexer.TokenIdentifier || token.Lexeme != "output" || tokenIndex == 0 || tokens[tokenIndex-1].Type != lexer.TokenLBracket {
			continue
		}
		for index := tokenIndex + 1; index < len(tokens); index++ {
			if tokens[index].Type == lexer.TokenLBrace {
				return appendToDelimitedBlock(text, tokens, tokens[index], fieldText)
			}
		}
	}
	return "", false
}

func appendToDelimitedBlock(text string, tokens []lexer.Token, openingToken lexer.Token, fieldText string) (string, bool) {
	openingIndex := -1
	for index, token := range tokens {
		if token.Type == lexer.TokenLBrace && token.Line == openingToken.Line && token.Column == openingToken.Column {
			openingIndex = index
			break
		}
	}
	if openingIndex < 0 {
		return "", false
	}

	depth := 0
	for index := openingIndex; index < len(tokens); index++ {
		switch tokens[index].Type {
		case lexer.TokenLBrace:
			depth++
		case lexer.TokenRBrace:
			depth--
			if depth == 0 {
				closingIndex := tokenStartIndex(text, tokens[index])
				return text[:closingIndex] + "\n  " + fieldText + "\n" + text[closingIndex:], true
			}
		}
	}
	return "", false
}

func nearestOutputRecord(file ast.File, declarationToken lexer.Token) (ast.RecordLiteral, bool) {
	records := []ast.RecordLiteral{}
	var visitExpression func(ast.Expression)
	visitExpression = func(expression ast.Expression) {
		switch typed := expression.(type) {
		case ast.RecordLiteral:
			records = append(records, typed)
			for _, field := range typed.Fields {
				visitExpression(field.Value)
			}
		case ast.ArrayLiteral:
			for _, element := range typed.Elements {
				visitExpression(element)
			}
		}
	}
	for _, field := range file.Output.DataFields {
		visitExpression(field.Value)
	}
	return nearestRecord(records, declarationToken)
}

func nearestRecord(records []ast.RecordLiteral, declarationToken lexer.Token) (ast.RecordLiteral, bool) {
	if len(records) == 0 {
		return ast.RecordLiteral{}, false
	}
	closest := records[0]
	closestDistance := tokenDistance(closest.StartToken, declarationToken)
	for _, record := range records[1:] {
		if distance := tokenDistance(record.StartToken, declarationToken); distance < closestDistance {
			closest = record
			closestDistance = distance
		}
	}
	return closest, true
}

type schemaContainer struct {
	StartToken lexer.Token
}

func nearestSchema(file ast.File, declarationToken lexer.Token) (schemaContainer, bool) {
	schemas := []schemaContainer{}
	if file.Script == nil {
		return schemaContainer{}, false
	}
	for _, item := range file.Script.Items {
		switch declaration := item.(type) {
		case ast.SchemaDeclaration:
			schemas = append(schemas, schemaContainer{StartToken: declaration.Type.StartToken})
		case ast.TypeDeclaration:
			if recordType, ok := declaration.Type.(ast.RecordType); ok {
				schemas = append(schemas, schemaContainer{StartToken: recordType.StartToken})
			}
		}
	}
	if len(schemas) == 0 {
		return schemaContainer{}, false
	}
	closest := schemas[0]
	closestDistance := tokenDistance(closest.StartToken, declarationToken)
	for _, schema := range schemas[1:] {
		if distance := tokenDistance(schema.StartToken, declarationToken); distance < closestDistance {
			closest = schema
			closestDistance = distance
		}
	}
	return closest, true
}

func tokenDistance(left lexer.Token, right lexer.Token) int {
	distance := left.Line - right.Line
	if distance < 0 {
		return -distance
	}
	return distance
}

func lowerFirst(value string) string {
	for index, runeValue := range value {
		return string(unicode.ToLower(runeValue)) + value[index+len(string(runeValue)):]
	}
	return value
}
