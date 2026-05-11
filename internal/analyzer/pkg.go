package analyzer

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"

	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
)

const serverName = "mace"

var diagnosticPositionPattern = regexp.MustCompile(`at (\d+):(\d+)`)

var keywordDocs = map[string]string{
	"array":      "Declares an array type like `array<string>`.",
	"enum":       "Declares a named scalar enum type backed by `string` or `int`.",
	"injectable": "Marks a script variable as overrideable through injections.",
	"type":       "Declares a reusable type alias.",
	"union":      "Declares schema composition like `union[Profile, Audit]`.",
	"variant":    "Declares a closed variant type like `variant[string, int]`.",
}

var directiveKeywordDocs = map[string]string{
	"output":      "Selects the output mode with `output = data` or `output = schema`.",
	"schema":      "Validates `output = data` against a named local or imported schema. It does not switch output mode.",
	"schema_file": "Loads declarations from another Mace file for output validation. It does not switch output mode.",
}

var declarationKeywordDocs = map[string]string{
	"enum":   "Declares a reusable enum type.",
	"schema": "Declares a reusable record schema.",
}

type document struct {
	text     string
	analysis analysisSnapshot
}

type completionDefinition struct {
	Label  string
	Kind   protocol.CompletionItemKind
	Detail string
}

type declarationDefinition struct {
	Name   string
	Kind   protocol.CompletionItemKind
	Detail string
}

type Snapshot = analysisSnapshot

func AnalyzeDocumentAt(text string, documentPath string) Snapshot {
	return analyzeDocumentAt(text, documentPath)
}

func AnalyzeDocumentAtInRoot(text string, documentPath string, importRootDir string) Snapshot {
	return analyzeDocumentAtInRoot(text, documentPath, importRootDir)
}

func AnalyzeCompletionContext(text string, documentPath string, position protocol.Position) Snapshot {
	return analyzeCompletionContext(text, documentPath, position)
}

func AnalyzeCompletionContextInRoot(text string, documentPath string, importRootDir string, position protocol.Position) Snapshot {
	return analyzeCompletionContextInRoot(text, documentPath, importRootDir, position)
}

func HasParsedFile(snapshot Snapshot) bool {
	return snapshot.file != nil
}

func Diagnostics(snapshot Snapshot) []protocol.Diagnostic {
	return snapshot.diagnostics
}

func CompletionItems(text string, snapshot Snapshot, uri protocol.DocumentUri, position protocol.Position) []protocol.CompletionItem {
	return completionItems(document{text: text, analysis: snapshot}, uri, position)
}

func Hover(text string, snapshot Snapshot, position protocol.Position) *protocol.Hover {
	identifier, found := identifierAt(text, position)
	if !found {
		return nil
	}

	if documentation, ok := keywordDocs[identifier]; ok {
		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.MarkupKindMarkdown,
				Value: documentation,
			},
		}
	}

	if isDirectivePosition(text, position) {
		if documentation, ok := directiveKeywordDocs[identifier]; ok {
			return &protocol.Hover{
				Contents: protocol.MarkupContent{
					Kind:  protocol.MarkupKindMarkdown,
					Value: documentation,
				},
			}
		}
	}

	if documentation, ok := declarationKeywordDocs[identifier]; ok {
		return &protocol.Hover{
			Contents: protocol.MarkupContent{
				Kind:  protocol.MarkupKindMarkdown,
				Value: documentation,
			},
		}
	}

	symbol, ok := snapshot.symbolAt(position)
	if !ok {
		return nil
	}

	value := "```mace\n" + symbol.Detail + "\n```"
	if symbol.Documentation != "" {
		value += "\n\n" + symbol.Documentation
	}

	return &protocol.Hover{
		Contents: protocol.MarkupContent{
			Kind:  protocol.MarkupKindMarkdown,
			Value: value,
		},
	}
}

func Definition(snapshot Snapshot, position protocol.Position) (protocol.Location, bool) {
	return snapshot.definitionAt(position)
}

func DocumentSymbols(text string, snapshot Snapshot) []protocol.DocumentSymbol {
	if snapshot.file == nil {
		return []protocol.DocumentSymbol{}
	}

	file := *snapshot.file
	symbols := lo.FilterMap(fileScriptItems(file), func(item ast.Declaration, _ int) (protocol.DocumentSymbol, bool) {
		switch declaration := item.(type) {
		case ast.TypeDeclaration:
			return newSymbol(text, declaration.Name, "type", protocol.SymbolKindClass, nil), true
		case ast.EnumDeclaration:
			children := lo.Map(declaration.Members, func(member ast.EnumMember, _ int) protocol.DocumentSymbol {
				detail := member.Name
				if member.HasValue {
					detail = expressionSummary(member.Value)
				}
				return newSymbol(text, member.Name, detail, protocol.SymbolKindEnumMember, nil)
			})
			return newSymbol(text, declaration.Name, declaration.BackingType.Name, protocol.SymbolKindEnum, children), true
		case ast.SchemaDeclaration:
			children := lo.Map(declaration.Type.Fields, func(field ast.SchemaField, _ int) protocol.DocumentSymbol {
				return newSymbol(text, field.Name, fieldTypeDetail(field.Type), protocol.SymbolKindField, nil)
			})
			return newSymbol(text, declaration.Name, "schema", protocol.SymbolKindStruct, children), true
		case ast.VariableDeclaration:
			return newSymbol(text, declaration.Name, typeReferenceDetail(declaration.Type), protocol.SymbolKindVariable, nil), true
		default:
			return protocol.DocumentSymbol{}, false
		}
	})

	if len(file.Output.DataFields) > 0 || len(file.Output.SchemaFields) > 0 {
		children := lo.Map(file.Output.DataFields, func(field ast.OutputField, _ int) protocol.DocumentSymbol {
			detail := "output field"
			if snapshot.result != nil {
				if value, ok := snapshot.result.Output[field.Name]; ok {
					detail = "output field = " + summarizeValue(value)
				}
			}

			return newSymbol(text, field.Name, detail, protocol.SymbolKindProperty, nil)
		})
		children = append(children, lo.Map(file.Output.SchemaFields, func(field ast.OutputSchemaField, _ int) protocol.DocumentSymbol {
			return newSymbol(text, field.Name, fieldTypeDetail(field.Type), protocol.SymbolKindProperty, nil)
		})...)
		symbols = append(symbols, newSymbol(text, "output", "output block", protocol.SymbolKindObject, children))
	}

	return symbols
}

func CodeActions(snapshot Snapshot, uri protocol.DocumentUri, targetRange protocol.Range) []protocol.CodeAction {
	return snapshot.codeActions(uri, targetRange)
}

func PrepareRename(snapshot Snapshot, position protocol.Position) (protocol.Range, bool) {
	if _, ok := snapshot.symbolAt(position); !ok {
		return protocol.Range{}, false
	}

	return identifierRangeAt(snapshot.text, position)
}

func Rename(text string, snapshot Snapshot, uri protocol.DocumentUri, position protocol.Position, newName string) (*protocol.WorkspaceEdit, bool) {
	symbol, ok := snapshot.symbolAt(position)
	if !ok || newName == "" {
		return nil, false
	}

	edits := map[protocol.DocumentUri][]protocol.TextEdit{}
	currentURI := uri
	if snapshot.documentURI != "" {
		currentURI = snapshot.documentURI
	}

	for index, token := range snapshot.tokens {
		if token.Type != lexer.TokenIdentifier || token.Lexeme != symbol.Name {
			continue
		}
		rangeValue := tokenProtocolRange(token)
		if !renameTokenMatchesSymbol(snapshot, index, rangeValue, symbol) {
			continue
		}
		edits[currentURI] = append(edits[currentURI], protocol.TextEdit{Range: rangeValue, NewText: newName})
	}

	if symbol.Origin == symbolOriginImport {
		definitionURI := symbol.Definition.URI
		if definitionURI != "" && definitionURI != currentURI {
			edits[definitionURI] = append(edits[definitionURI], protocol.TextEdit{Range: symbol.Definition.Range, NewText: newName})
		}
	}

	if len(edits) == 0 {
		return nil, false
	}
	return &protocol.WorkspaceEdit{Changes: edits}, true
}

func renameTokenMatchesSymbol(snapshot Snapshot, index int, rangeValue protocol.Range, symbol semanticSymbol) bool {
	if rangesEqual(rangeValue, symbol.Range) || sameLocation(protocol.Location{URI: snapshot.documentURI, Range: rangeValue}, symbol.Definition) {
		return true
	}

	if index+1 < len(snapshot.tokens) {
		switch snapshot.tokens[index+1].Type {
		case lexer.TokenColon, lexer.TokenQuestion:
			return false
		}
	}

	if tokenInEnumDeclaration(snapshot.tokens, index) {
		return false
	}

	location, ok := snapshot.definitionAt(rangeValue.Start)
	if ok && sameLocation(location, symbol.Definition) {
		return true
	}

	return symbol.Kind == protocol.CompletionItemKindVariable && !tokenInEnumDeclaration(snapshot.tokens, index)
}

func tokenInEnumDeclaration(tokens []lexer.Token, index int) bool {
	depth := 0
	for cursor := index; cursor >= 0; cursor-- {
		switch tokens[cursor].Type {
		case lexer.TokenRBrace:
			depth++
		case lexer.TokenLBrace:
			if depth == 0 {
				for check := cursor - 1; check >= 0; check-- {
					switch tokens[check].Type {
					case lexer.TokenEnum:
						return true
					case lexer.TokenSemicolon, lexer.TokenScriptDelimiter:
						return false
					}
				}
				return false
			}
			depth--
		}
	}
	return false
}

func sameLocation(left protocol.Location, right protocol.Location) bool {
	return left.URI == right.URI && rangesEqual(left.Range, right.Range)
}

func rangesEqual(left protocol.Range, right protocol.Range) bool {
	return left.Start.Line == right.Start.Line &&
		left.Start.Character == right.Start.Character &&
		left.End.Line == right.End.Line &&
		left.End.Character == right.End.Character
}

func FormatDocumentText(text string) string {
	return formatDocumentText(text)
}

func DiagnosticFromError(err error) protocol.Diagnostic {
	position := protocol.Position{}
	matches := diagnosticPositionPattern.FindStringSubmatch(err.Error())
	if len(matches) == 3 {
		line := parseUint(matches[1])
		column := parseUint(matches[2])
		if line > 0 {
			position.Line = line - 1
		}
		if column > 0 {
			position.Character = column - 1
		}
	}

	end := position
	end.Character++

	code := classifyDiagnosticCode(err.Error())
	message := err.Error()
	if diagnosticError, ok := processorDiagnosticError(err); ok {
		code = diagnosticCodeFromProcessorError(diagnosticError)
		message = diagnosticError.Message
	}

	return diagnosticWithCode(protocol.Range{
		Start: position,
		End:   end,
	}, protocol.DiagnosticSeverityError, code, message)
}

func DocumentPath(uri protocol.DocumentUri) string {
	path, ok := documentPathFromURI(uri)
	if !ok {
		return ""
	}

	return path
}

func diagnosticFromError(err error) protocol.Diagnostic {
	return DiagnosticFromError(err)
}

func processorDiagnosticError(err error) (processor.DiagnosticError, bool) {
	var diagnosticError processor.DiagnosticError
	if !errors.As(err, &diagnosticError) {
		return processor.DiagnosticError{}, false
	}

	return diagnosticError, true
}

func documentPath(uri protocol.DocumentUri) string {
	return DocumentPath(uri)
}

func resolveBoundedPath(importBaseDir string, importPath string) (string, error) {
	return resolveBoundedPathInRoot(importBaseDir, importBaseDir, importPath)
}

func resolveBoundedPathInRoot(importBaseDir string, _ string, importPath string) (string, error) {
	if filepath.IsAbs(importPath) {
		return "", fmt.Errorf("import path %q must be relative: base=%q", importPath, importBaseDir)
	}

	cleanPath := filepath.Clean(filepath.FromSlash(importPath))
	resolvedPath := filepath.Clean(filepath.Join(importBaseDir, cleanPath))
	return resolvedPath, nil
}

func parseUint(value string) protocol.UInteger {
	var parsed protocol.UInteger
	_, _ = fmt.Sscanf(value, "%d", &parsed)
	return parsed
}

func parseFile(text string) (ast.File, error) {
	tokens, err := lex(text)
	if err != nil {
		return ast.File{}, err
	}

	return parser.New(tokens).ParseFile()
}

func parseExpression(text string) (ast.Expression, error) {
	tokens, err := lex(text)
	if err != nil {
		return nil, err
	}

	return parser.New(tokens).ParseExpression()
}

func lex(text string) ([]lexer.Token, error) {
	lexerInstance := lexer.New(text)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			return tokens, nil
		}
	}
}

func typeReferenceDetail(typeReference ast.TypeReference) string {
	switch value := typeReference.(type) {
	case ast.PrimitiveType:
		return value.Name
	case ast.NamedType:
		return value.Name
	case ast.ArrayType:
		return fmt.Sprintf("array<%s>", typeReferenceDetail(value.Element))
	case ast.UnionType:
		parts := lo.Map(value.Members, func(member ast.TypeReference, _ int) string {
			return typeReferenceDetail(member)
		})
		return fmt.Sprintf("union[%s]", strings.Join(parts, ", "))
	case ast.VariantType:
		parts := lo.Map(value.Members, func(member ast.TypeReference, _ int) string {
			return typeReferenceDetail(member)
		})
		return fmt.Sprintf("variant[%s]", strings.Join(parts, ", "))
	case ast.RecordType:
		return recordTypeDetail(value)
	default:
		return "unknown"
	}
}

func fieldTypeDetail(typeReference ast.TypeReference) string {
	return typeReferenceDetail(typeReference)
}

func recordTypeDetail(record ast.RecordType) string {
	fields := lo.Map(record.Fields, func(field ast.SchemaField, _ int) string {
		name := field.Name
		if field.Optional {
			name += "?"
		}
		return fmt.Sprintf("%s: %s", name, typeReferenceDetail(field.Type))
	})

	return "{ " + strings.Join(fields, ", ") + " }"
}

func newSymbol(text string, name string, detail string, kind protocol.SymbolKind, children []protocol.DocumentSymbol) protocol.DocumentSymbol {
	start, end := nameRange(text, name)
	symbol := protocol.DocumentSymbol{
		Name: name,
		Kind: kind,
		Range: protocol.Range{
			Start: start,
			End:   end,
		},
		SelectionRange: protocol.Range{
			Start: start,
			End:   end,
		},
		Children: children,
	}

	if detail != "" {
		symbol.Detail = Ptr(detail)
	}

	return symbol
}

func nameRange(text string, name string) (protocol.Position, protocol.Position) {
	index := strings.Index(text, name)
	if index < 0 {
		return protocol.Position{}, protocol.Position{}
	}

	start := positionFromIndex(text, index)
	end := positionFromIndex(text, index+len(name))
	return start, end
}

func positionFromIndex(text string, index int) protocol.Position {
	line := protocol.UInteger(0)
	column := protocol.UInteger(0)

	for currentIndex, runeValue := range text {
		if currentIndex >= index {
			break
		}
		if runeValue == '\n' {
			line++
			column = 0
			continue
		}
		column++
	}

	return protocol.Position{
		Line:      line,
		Character: column,
	}
}

func identifierPrefixAt(text string, position protocol.Position) string {
	index := positionIndex(text, position)
	if index < 0 {
		return ""
	}

	start := index
	for start > 0 && isIdentifierCharacter(text[start-1]) {
		start--
	}

	return text[start:index]
}

func identifierAt(text string, position protocol.Position) (string, bool) {
	rangeValue, ok := identifierRangeAt(text, position)
	if !ok {
		return "", false
	}

	start := positionIndex(text, rangeValue.Start)
	end := positionIndex(text, rangeValue.End)
	if start < 0 || end < start || end > len(text) {
		return "", false
	}

	return text[start:end], true
}

func identifierRangeAt(text string, position protocol.Position) (protocol.Range, bool) {
	index := positionIndex(text, position)
	if index < 0 || index > len(text) {
		return protocol.Range{}, false
	}

	start := index
	for start > 0 && isIdentifierCharacter(text[start-1]) {
		start--
	}

	end := index
	for end < len(text) && isIdentifierCharacter(text[end]) {
		end++
	}

	if start == end {
		return protocol.Range{}, false
	}

	return protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}, true
}

func isDirectivePosition(text string, position protocol.Position) bool {
	index := positionIndex(text, position)
	if index < 0 || index > len(text) {
		return false
	}

	openIndex := strings.LastIndex(text[:index], "[")
	if openIndex < 0 {
		return false
	}

	closeIndex := strings.Index(text[openIndex:], "]")
	if closeIndex < 0 {
		return true
	}

	return openIndex+closeIndex >= index
}

func isIdentifierCharacter(value byte) bool {
	return value == '_' ||
		(value >= 'a' && value <= 'z') ||
		(value >= 'A' && value <= 'Z') ||
		(value >= '0' && value <= '9')
}

func positionIndex(text string, position protocol.Position) int {
	return clampPosition(text, position).IndexIn(text)
}

func clampPosition(text string, position protocol.Position) protocol.Position {
	lineStart, ok := lineStartIndex(text, position.Line)
	if !ok {
		return protocol.Position{}
	}

	lineLength := utf16LineLength(text[lineStart:])
	if position.Character > lineLength {
		position.Character = lineLength
	}

	return position
}

func lineStartIndex(text string, line protocol.UInteger) (int, bool) {
	index := 0
	for currentLine := protocol.UInteger(0); currentLine < line; currentLine++ {
		next := strings.IndexByte(text[index:], '\n')
		if next < 0 {
			return 0, false
		}
		index += next + 1
	}

	return index, true
}

func utf16LineLength(text string) protocol.UInteger {
	lineLength := protocol.UInteger(0)
	for _, runeValue := range text {
		if runeValue == '\n' {
			break
		}

		runeLength := utf16.RuneLen(runeValue)
		if runeLength < 0 {
			continue
		}
		lineLength += protocol.UInteger(runeLength)
	}

	return lineLength
}

func Ptr[T any](value T) *T {
	return &value
}

func fileScriptItems(file ast.File) []ast.Declaration {
	if file.Script == nil {
		return nil
	}

	return file.Script.Items
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}

	return (&url.URL{
		Scheme: "file",
		Path:   path,
	}).String()
}
