package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func writeAnalysisFile(root string, relativePath string, contents string) string {
	path := filepath.Join(root, relativePath)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	tAssert.NoError(err)
	err = os.WriteFile(path, []byte(contents), 0o600)
	tAssert.NoError(err)
	return path
}

func declarationNames(snapshot analysisSnapshot) []string {
	return lo.Map(snapshot.declarations, func(definition declarationDefinition, _ int) string {
		return definition.Name
	})
}

func requireDefinition(snapshot analysisSnapshot, position protocol.Position) protocol.Location {
	location, ok := snapshot.definitionAt(position)
	tAssert.True(ok)
	if !ok {
		return protocol.Location{}
	}

	return location
}

func requireCodeAction(snapshot analysisSnapshot, uri protocol.DocumentUri, targetRange protocol.Range, title string) protocol.CodeAction {
	action, ok := lo.Find(snapshot.codeActions(uri, targetRange), func(action protocol.CodeAction) bool {
		return action.Title == title
	})
	tAssert.True(ok)
	if !ok {
		return protocol.CodeAction{}
	}

	return action
}

func requireDiagnosticCode(diagnostic protocol.Diagnostic) string {
	tAssert.NotNil(diagnostic.Code)
	if diagnostic.Code == nil {
		return ""
	}

	value, ok := diagnostic.Code.Value.(string)
	tAssert.True(ok)
	if !ok {
		return ""
	}

	return value
}

var _ = Describe("LSP analysis", func() {
	It("covers literal and type helper functions", func() {
		tAssert.NotEmpty(defaultLiteralForTypeName("string"))
		tAssert.NotEmpty(defaultLiteralForTypeName("boolean"))
		tAssert.NotEmpty(defaultLiteralForTypeName("int"))
		tAssert.NotEmpty(defaultLiteralForTypeName("float"))
		tAssert.NotEmpty(defaultLiteralForTypeName("[]int"))
		tAssert.NotEmpty(defaultLiteralForTypeName("Profile"))
		tAssert.NotEmpty(convertStringLiteralForm("Ada"))
		tAssert.NotEmpty(convertStringLiteralForm(`"Ada"`))
		tAssert.NotEmpty(simpleExpressionText(ast.StringLiteral{Lexeme: `"Ada"`}))
		tAssert.NotEmpty(simpleExpressionText(ast.IntLiteral{Lexeme: "7"}))
		tAssert.NotEmpty(simpleExpressionText(ast.BooleanLiteral{Value: true}))
		tAssert.NotEmpty(defaultExpressionForType(ast.PrimitiveType{Name: "string"}))
		tAssert.NotEmpty(inferredTypeFromExpression(ast.StringLiteral{Lexeme: `"Ada"`}))
		tAssert.NotEmpty(placeholderForType(ast.File{}, "Title"))
	})

	It("covers text and declaration edit helpers", func() {
		text := "first\nsecond\nthird"
		tAssert.NotEmpty(lineAt(text, 1))
		tAssert.Equal("", lineAt(text, 9))

		file := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}}}}
		tokens := lexAnalysisTokens("from \"./shared.mace\" import Remote: Local;")
		_, _, _ = moveImportsToTopEdit("from \"./shared.mace\" import Remote: Local;", file, tokens, "move")
		_, _ = duplicateDeclarationEditRange("type Name: string; type Name: int;", file, tokens, "duplicate")
		_, _, _ = declarationOperatorEdit("type Name: string;", tokens, "operator")
		_, _, _ = selfOrderingEdit("[output = data] { result: $self.name; }", file, tokens, "self")
	})

	It("covers analysis helpers for diagnostics and symbols", func() {
		tAssert.NotEmpty(diagnosticCodeFromProcessorError(processor.DiagnosticError{Kind: processor.ErrorImport}))
		tAssert.NotEmpty(diagnosticCodeFromProcessorError(processor.DiagnosticError{Code: processor.CodeTypeMismatch}))
		tAssert.NotEmpty(expressionSummary(ast.StringLiteral{Lexeme: `"Ada"`}))
		tAssert.NotEmpty(expressionSummary(ast.Identifier{Name: "name"}))
		tAssert.NotEmpty(summarizeValue(processor.Value{Kind: processor.ValueString, String: "Ada"}))
		tAssert.NotEmpty(summarizeValue(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueInt, Int: 1}}}))
		tAssert.NotEmpty(fileScriptDeclarations(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}))
	})

	It("covers schema documentation and refactor helpers", func() {
		file := ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Profile"}}}}
		tAssert.NotPanics(func() { _ = hasSchemaDoc(file, "Profile") })
		tAssert.NotPanics(func() { _ = hasSchemaDoc(file, "Missing") })
		tAssert.NotEmpty(placeholderForType(file, "name"))
		_, _ = existingMacePathWithSimilarName("./profile.mace")
	})

	It("computes output and field edit ranges", func() {
		insertRange := outputInsertRange
		bodyRange := outputBodyRange
		namedRange := namedFieldEditRange
		namedRangeFromEnd := namedFieldEditRangeFromEnd
		fieldRangeAt := fieldEditRangeAt

		text := "[output = data]\n{\n  name: \"Ada\";\n  age: 27;\n  name: \"Bob\";\n}"
		tokens := lexAnalysisTokens(text)

		rangeValue, ok := insertRange(text)
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Line: 5, Character: 0}, rangeValue.Start)
		tAssert.Equal(rangeValue.Start, rangeValue.End)

		rangeValue, ok = bodyRange(text, tokens)
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Line: 1, Character: 0}, rangeValue.Start)
		tAssert.Equal(protocol.Position{Line: 5, Character: 1}, rangeValue.End)

		rangeValue, ok = namedRange(text, tokens, "name")
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Line: 2, Character: 0}, rangeValue.Start)
		tAssert.Equal(protocol.Position{Line: 3, Character: 0}, rangeValue.End)

		rangeValue, ok = namedRangeFromEnd(text, tokens, "name")
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Line: 4, Character: 0}, rangeValue.Start)
		tAssert.Equal(protocol.Position{Line: 5, Character: 0}, rangeValue.End)

		_, ok = namedRange(text, tokens, "missing")
		tAssert.False(ok)

		_, ok = fieldRangeAt(text, tokens, len(tokens)+1)
		tAssert.False(ok)

		_, ok = insertRange("[output = data]\n")
		tAssert.False(ok)
	})

	It("classifies parser and processor diagnostics", func() {
		parseCases := map[string]diagnosticCode{
			"parser: empty script block":                     diagnosticSyntaxEmptyScriptBlock,
			"parser: expected closing script delimiter EOF":  diagnosticSyntaxUnterminatedScriptBlock,
			"parser: script delimiter mismatch":              diagnosticSyntaxInconsistentScriptDelimiters,
			"parser: malformed import":                       diagnosticSyntaxMalformedImport,
			"parser: malformed directive":                    diagnosticSyntaxMalformedDirectiveList,
			"parser: malformed schema declaration":           diagnosticSyntaxMalformedSchema,
			"parser: expected integer index in array access": diagnosticSyntaxInvalidArrayAccessIndex,
			"parser: not allowed when output = schema":       diagnosticDirectiveSchemaOutputVariableIgnored,
			"parser: malformed variable declaration":         diagnosticSyntaxMalformedVariableDeclaration,
			"parser: malformed output field":                 diagnosticSyntaxMalformedOutputField,
			"parser: unexpected token":                       diagnosticSyntaxUnexpectedToken,
		}
		for message, code := range parseCases {
			tAssert.Equal(code, classifyDiagnosticCode(message))
		}

		processorCases := map[string]diagnosticCode{
			"processor: duplicate output directive":                                                 diagnosticDirectiveDuplicateKey,
			"processor: duplicate documentation declaration":                                        diagnosticSyntaxUnexpectedToken,
			"processor: unknown output directive":                                                   diagnosticDirectiveUnknownKey,
			"processor: schema directive is invalid when output mode is schema":                     diagnosticDirectiveOutputSchemaCombined,
			"processor: unknown schema User":                                                        diagnosticDirectiveUnknownSchemaName,
			"processor: unable to read import file":                                                 diagnosticImportFileNotFound,
			"processor: unable to parse import file":                                                diagnosticImportFileFailedParse,
			"processor: circular import":                                                            diagnosticImportCircular,
			"processor: duplicate import":                                                           diagnosticImportDuplicateName,
			"processor: imported identifier missing":                                                diagnosticImportNameNotExposed,
			"processor: schema_doc target missing":                                                  diagnosticSyntaxUnexpectedToken,
			"processor: unknown type Profile":                                                       diagnosticDeclarationUnknownTypeReference,
			"processor: variable requires an initializer":                                           diagnosticDeclarationVariableMissingInitializer,
			"processor: duplicate declaration":                                                      diagnosticDeclarationDuplicateVariable,
			"processor: already documented by a documentation declaration":                          diagnosticSyntaxUnexpectedToken,
			"processor: duplicate field":                                                            diagnosticDeclarationDuplicateSchemaField,
			"processor: duplicate output field":                                                     diagnosticDeclarationDuplicateOutputField,
			"processor: array literal has mixed element types":                                      diagnosticTypeMixedArrayLiteral,
			"processor: array index 3 out of range":                                                 diagnosticTypeInvalidArrayAccess,
			"processor: unknown identifier":                                                         diagnosticTypeUnknownIdentifier,
			"processor: unknown self reference":                                                     diagnosticTypeUnknownSelfField,
			"processor: invalid field type when output = schema":                                    diagnosticTypeInvalidOutputSchemaField,
			"processor: cannot reference type or schema declaration":                                diagnosticTypeUnknownIdentifier,
			"processor: expected boolean after '!'":                                                 diagnosticTypeInvalidUnaryOperator,
			"processor: expected numeric operands":                                                  diagnosticTypeInvalidBinaryOperator,
			"processor: null can only be assigned to nullable variables and optional schema fields": diagnosticTypeInvalidNullUsage,
			"processor: type mismatch":                                                              diagnosticTypeInitializerMismatch,
			"processor: missing required field":                                                     diagnosticTypeRecordDoesNotMatchSchema,
			"processor: something else":                                                             diagnosticSyntaxUnexpectedToken,
		}
		for message, code := range processorCases {
			tAssert.Equal(code, classifyDiagnosticCode(message))
		}

		tAssert.Equal(diagnosticSyntaxUnexpectedToken, classifyDiagnosticCode("plain error"))
	})

	It("covers diagnostic error code and token helpers", func() {
		errorCases := []struct {
			err  processor.DiagnosticError
			code diagnosticCode
		}{
			{processor.DiagnosticError{Code: processor.CodeArrayIndexOutOfRange}, diagnosticTypeInvalidArrayAccess},
			{processor.DiagnosticError{Code: processor.CodeArrayValueRequired}, diagnosticTypeInvalidArrayAccess},
			{processor.DiagnosticError{Code: processor.CodeInvalidNullUsage}, diagnosticTypeInvalidNullUsage},
			{processor.DiagnosticError{Code: processor.CodeInvalidOutputSchemaField}, diagnosticTypeInvalidOutputSchemaField},
			{processor.DiagnosticError{Code: processor.CodeMissingRequiredField}, diagnosticTypeRecordDoesNotMatchSchema},
			{processor.DiagnosticError{Code: processor.CodeOutputValueDeclaration}, diagnosticTypeUnknownIdentifier},
			{processor.DiagnosticError{Code: processor.CodeSelfReferenceUnknown}, diagnosticTypeUnknownSelfField},
			{processor.DiagnosticError{Code: processor.CodeTypeMismatch}, diagnosticTypeInitializerMismatch},
			{processor.DiagnosticError{Kind: processor.ErrorImport}, diagnosticImportFileFailedParse},
			{processor.DiagnosticError{Kind: processor.ErrorDirective}, diagnosticDirectiveUnknownKey},
			{processor.DiagnosticError{Kind: processor.ErrorDeclaration}, diagnosticDeclarationDuplicateVariable},
			{processor.DiagnosticError{Kind: processor.ErrorType}, diagnosticTypeInitializerMismatch},
			{processor.DiagnosticError{Kind: processor.ErrorValue}, diagnosticTypeUnknownIdentifier},
			{processor.DiagnosticError{Kind: processor.ErrorOperator}, diagnosticTypeInvalidBinaryOperator},
			{processor.DiagnosticError{Kind: processor.ErrorSchema}, diagnosticTypeRecordDoesNotMatchSchema},
			{processor.DiagnosticError{Message: "duplicate output directive"}, diagnosticDirectiveDuplicateKey},
		}
		for _, test := range errorCases {
			tAssert.Equal(test.code, diagnosticCodeFromProcessorError(test.err))
		}

		bracket := lexer.Token{Type: lexer.TokenLBracket, Lexeme: "[", Line: 1, Column: 4}
		index := lexer.Token{Type: lexer.TokenInt, Lexeme: "3", Line: 1, Column: 5}
		candidates := []arrayAccessCandidate{{Level: 2, Bracket: bracket, Index: &index}}
		token, ok := invalidArrayAccessToken(candidates, processor.DiagnosticError{
			Fields: processor.DiagnosticFields{Level: 2},
		})
		tAssert.True(ok)
		tAssert.Equal(bracket, token)

		token, ok = outOfRangeArrayAccessToken(candidates, processor.DiagnosticError{
			Fields: processor.DiagnosticFields{Level: 2, Index: "3"},
		})
		tAssert.True(ok)
		tAssert.Equal(index, token)

		_, ok = invalidArrayAccessToken(candidates, processor.DiagnosticError{
			Fields: processor.DiagnosticFields{Level: 1},
		})
		tAssert.False(ok)
	})

	It("covers public root wrappers and range helpers", func() {
		workspace, err := os.MkdirTemp("", "mace-analyzer-root-wrappers-*")
		tAssert.NoError(err)
		documentPath := filepath.Join(workspace, "document.mace")
		text := `[output = data]
{
  name: "Ada";
}`
		snapshot := AnalyzeDocumentAtInRoot(text, documentPath, workspace)
		tAssert.True(HasParsedFile(snapshot))
		tAssert.Empty(Diagnostics(snapshot))

		completion := AnalyzeCompletionContextInRoot(text, documentPath, workspace, protocol.Position{
			Line:      2,
			Character: protocol.UInteger(len(`  name`)),
		})
		tAssert.True(HasParsedFile(completion))

		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 2, Character: 2},
			End:   protocol.Position{Line: 2, Character: 6},
		}
		tAssert.Empty(CodeActions(snapshot, uri, rangeValue))
		tAssert.True(hasTextEditAtRange([]protocol.TextEdit{{Range: rangeValue, NewText: "title"}}, rangeValue))
		tAssert.False(hasTextEditAtRange(nil, rangeValue))
	})

	It("finds import alias tokens", func() {
		text := `from "./shared.mace" import Remote: Local;`
		tokens := lexAnalysisTokens(text)
		importDecl := ast.ImportDeclaration{
			Path: ast.StringLiteral{Lexeme: `"./shared.mace"`},
		}
		identifier := ast.ImportedIdentifier{Name: "Remote", Alias: "Local"}

		token, ok := importAliasToken(tokens, importDecl, identifier)
		tAssert.True(ok)
		tAssert.Equal("Local", token.Lexeme)

		_, ok = importAliasToken(tokens, importDecl, ast.ImportedIdentifier{Name: "Remote"})
		tAssert.False(ok)
	})

	It("quick-formats parseable documents", func() {
		quickFormat := formatTextQuick

		formatted, ok := quickFormat(`[output = data]{result:1+2;}`)
		tAssert.True(ok)
		tAssert.Equal(`[output = data]
{
  result: 1 + 2
}`, formatted)

		_, ok = quickFormat(`[output = data] { result: ; }`)
		tAssert.False(ok)
	})

	It("resolves output value symbols from self references and nested fields", func() {
		selfPathAt := selfReferencePathAt
		nestedPathAt := nestedOutputFieldPathAt
		valueAtPath := outputValueAtPath
		valueSymbol := outputValueSymbol

		text := `[output = data]
{
  profile: {
    name: "Ada";
  };
  result: $self.profile.name;
}`
		tokens := lexAnalysisTokens(text)

		path, rangeValue, ok := selfPathAt(tokens, positionFromIndex(text, strings.Index(text, "$self.profile.name")+len("$self.profile.")))
		tAssert.True(ok)
		tAssert.Equal([]string{"profile", "name"}, path)
		tAssert.Equal(protocol.Position{Line: 5, Character: 24}, rangeValue.Start)

		path, rangeValue, ok = nestedPathAt(text, tokens, positionFromIndex(text, strings.Index(text, "name:")))
		tAssert.True(ok)
		tAssert.Equal([]string{"profile", "name"}, path)
		tAssert.Equal(protocol.Position{Line: 3, Character: 4}, rangeValue.Start)

		output := map[string]processor.Value{
			"profile": {Kind: processor.ValueRecord, Record: map[string]processor.Value{
				"name": {Kind: processor.ValueString, String: "Ada"},
			}},
		}

		value, ok := valueAtPath(output, []string{"profile", "name"})
		tAssert.True(ok)
		tAssert.Equal(processor.ValueString, value.Kind)
		tAssert.Equal("Ada", value.String)

		_, ok = valueAtPath(output, nil)
		tAssert.False(ok)
		_, ok = valueAtPath(output, []string{"profile", "missing"})
		tAssert.False(ok)
		_, ok = valueAtPath(output, []string{"profile", "name", "first"})
		tAssert.False(ok)

		symbol := valueSymbol("$self.profile.name", []string{"profile", "name"}, rangeValue, value)
		tAssert.Equal("$self.profile.name", symbol.Name)
		tAssert.Contains(symbol.Detail, `output profile.name: string = "Ada"`)
		tAssert.Equal(symbolOriginOutput, symbol.Origin)
	})

	It("finds output symbols through analysis snapshots", func() {
		text := `[output = data]
{
  profile: {
    name: "Ada";
  };
  result: $self.profile.name;
}`
		snapshot := analyzeDocument(text)

		symbol, ok := snapshot.symbolAt(positionFromIndex(text, strings.Index(text, "$self.profile.name")+len("$self.profile.")))
		tAssert.True(ok)
		tAssert.Equal("$self.profile.name", symbol.Name)

		symbol, ok = snapshot.symbolAt(positionFromIndex(text, strings.Index(text, "name:")))
		tAssert.True(ok)
		tAssert.Equal("profile.name", symbol.Name)

		location, ok := snapshot.definitionAt(positionFromIndex(text, strings.Index(text, "profile:")))
		tAssert.False(ok)
		tAssert.Equal(protocol.Location{}, location)

		definitionText := `|===|
int count = 1;
|===|
[output = data]
{ result: count; }`
		definitionSnapshot := analyzeDocument(definitionText)
		symbol, ok = definitionSnapshot.symbolAt(positionFromIndex(definitionText, strings.Index(definitionText, "count;")))
		tAssert.True(ok)
		tAssert.Equal("count", symbol.Name)
	})

	It("builds quick-fix edits for schema and directive diagnostics", func() {
		missingField := missingSchemaFieldEdit
		missingImport := missingImportEdit
		invalidDirectiveRange := invalidDirectiveComboEditRange
		generateFromSchema := generateOutputFromSchemaEdit

		text := `|===|
schema User: {
  name: string;
  age: int;
};
|===|
[output = data, schema = User]
{
  name: "Ada";
}`
		file, err := parseFile(text)
		tAssert.NoError(err)
		tokens := lexAnalysisTokens(text)

		rangeValue, newText, ok := missingField(text, file, `processor: missing required field "age" for schema "User"`)
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Line: 9, Character: 0}, rangeValue.Start)
		tAssert.Equal("  age: TODO;\n", newText)

		rangeValue, newText, ok = generateFromSchema(text, file, tokens, `processor: missing required field "age" for schema "User"`)
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Line: 1, Character: 13}, rangeValue.Start)
		tAssert.Contains(newText, `name: ""`)
		tAssert.Contains(newText, `age: 0`)

		schemaText := `[output = schema, schema = User]
{
  name: string;
}`
		schemaFile, err := parseFile(schemaText)
		tAssert.NoError(err)
		schemaTokens := lexAnalysisTokens(schemaText)

		rangeValue, ok = invalidDirectiveRange(schemaText, schemaFile, schemaTokens, "schema directive is invalid when output mode is schema")
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Line: 0, Character: 10}, rangeValue.Start)

		importText := `[output = data] { result: value; }`
		importFile, err := parseFile(importText)
		tAssert.NoError(err)
		rangeValue, newText, ok = missingImport(importText, importFile, lexAnalysisTokens(importText), `processor: unknown identifier "SharedValue"`)
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{}, rangeValue.Start)
		tAssert.Contains(newText, `from "./shared.mace" import SharedValue;`)

		_, _, ok = missingImport(text, file, tokens, `processor: unknown identifier "SharedValue"`)
		tAssert.False(ok)
	})

	It("exercises public analysis helpers", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-public-*")
		tAssert.NoError(err)
		documentPath := filepath.Join(workspace, "public.mace")
		uri := protocol.DocumentUri("file://" + filepath.ToSlash(documentPath))
		text := `|===|
schema User: { name: string; };
string greeting = "hello";
|===|
[output = data]
{
  name: greeting;
}`

		snapshot := AnalyzeDocumentAt(text, documentPath)
		tAssert.True(HasParsedFile(snapshot))
		tAssert.Empty(Diagnostics(snapshot))
		tAssert.NotEmpty(DocumentSymbols(text, snapshot))
		tAssert.NotNil(Hover(text, snapshot, protocol.Position{Line: 1, Character: 1}))
		_, ok := Definition(snapshot, protocol.Position{Line: 6, Character: 9})
		tAssert.True(ok)
		_, ok = PrepareRename(snapshot, protocol.Position{Line: 2, Character: 8})
		tAssert.True(ok)
		edit, ok := Rename(text, snapshot, uri, protocol.Position{Line: 2, Character: 8}, "message")
		tAssert.True(ok)
		tAssert.NotNil(edit)
		tAssert.NotEmpty(FormatDocumentText(text))
		tAssert.NotEmpty(DiagnosticFromError(fmt.Errorf("parser: expected expression at 3:5")).Message)

		completionSnapshot := AnalyzeCompletionContext(text, documentPath, protocol.Position{Line: 7, Character: 2})
		_ = CompletionItems(text, completionSnapshot, uri, protocol.Position{Line: 7, Character: 2})
	})
	It("surfaces parsed variables as LSP-visible declarations", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-parse-declarations-*")
		tAssert.NoError(err)

		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`|===|
		schema Runtime: {
		  env: string;
		  profile: { name: string; };
		};
		|===|
		[output = data, parse = Runtime]
		{
		  result: profile.name;
		}`, documentPath)

		names := declarationNames(snapshot)
		tAssert.Contains(names, "env")
		tAssert.Contains(names, "profile")
		tAssert.Contains(names, "input")
		tAssert.Contains(names, "runtime")
	})

	It("returns hover details for parsed variables", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-parse-hover-*")
		tAssert.NoError(err)

		documentPath := filepath.Join(workspace, "consumer.mace")
		text := `|===|
	schema Runtime: {
	  env: string;
	  profile: { name: string; };
	};
	|===|
	[output = data, parse = Runtime]
	{
	  result: profile.name;
	}`
		snapshot := analyzeDocumentAt(text, documentPath)
		hover := Hover(text, snapshot, protocol.Position{Line: 8, Character: 11})
		tAssert.NotNil(hover)
		if hover == nil {
			return
		}

		content, ok := hover.Contents.(protocol.MarkupContent)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.Contains(content.Value, "parse profile: { name: string }")
	})

	It("surfaces only LSP-visible declarations from imports, script, and output", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-visible-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `|===|
type Hidden: string;
schema User: { name: string; };
string local = "Ada";
|===|
[output = schema]
{
  User: User;
  exported_name: string;
}`)

		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import User;
schema Local: { id: int; };
User current = { name: "Ada"; };
|===|
[output = data]
{
  result: current;
}`, filepath.Join(workspace, "consumer.mace"))

		names := declarationNames(snapshot)
		tAssert.Contains(names, "User")
		tAssert.Contains(names, "Local")
		tAssert.Contains(names, "current")
		tAssert.Contains(names, "result")
		tAssert.NotContains(names, "Hidden")
		tAssert.NotContains(names, "local")
	})

	It("translates symbol lookups into definition locations for imported and local names", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-definition-*")
		tAssert.NoError(err)

		importPath := writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import User;
User current = { name: "Ada"; };
|===|
[output = data]
{
  result: current;
}`, documentPath)

		importedDefinition := requireDefinition(snapshot, protocol.Position{Line: 2, Character: 1})
		tAssert.Equal(protocol.DocumentUri(fileURI(importPath)), importedDefinition.URI)

		localDefinition := requireDefinition(snapshot, protocol.Position{Line: 6, Character: 12})
		tAssert.Equal(protocol.DocumentUri(fileURI(documentPath)), localDefinition.URI)
		tAssert.Equal(protocol.UInteger(2), localDefinition.Range.Start.Line)
	})

	It("prefers output field definitions over same-named schema declarations", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-output-definition-*")
		tAssert.NoError(err)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
|===|
[output = data]
{
  User: { name: "Ada"; };
}`, documentPath)

		definition := requireDefinition(snapshot, protocol.Position{Line: 5, Character: 3})
		tAssert.Equal(protocol.DocumentUri(fileURI(documentPath)), definition.URI)
		tAssert.Equal(protocol.UInteger(5), definition.Range.Start.Line)
		tAssert.Equal(protocol.UInteger(2), definition.Range.Start.Character)
	})

	It("prefers current document definitions over imported symbols with matching coordinates", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-definition-coordinates-*")
		tAssert.NoError(err)

		importPath := writeAnalysisFile(workspace, "shared.mace", `[output = data]
{




       qux: 1;
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import qux;
int qux = 2;
|===|

{
  bar: qux;
}`, documentPath)

		definition := requireDefinition(snapshot, protocol.Position{Line: 6, Character: 7})
		tAssert.Equal(protocol.DocumentUri(fileURI(documentPath)), definition.URI)
		tAssert.NotEqual(protocol.DocumentUri(fileURI(importPath)), definition.URI)
		tAssert.Equal(protocol.UInteger(2), definition.Range.Start.Line)
		tAssert.Equal(protocol.UInteger(4), definition.Range.Start.Character)
	})

	It("translates import path validation into an LSP diagnostic and quick fix", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-import-fix-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = data] { name: "Ada"; }`)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`|===|
from "./shared" import name;
|===|
[output = data]
{
  result: name;
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "must end in .mace")
			tAssert.Equal(protocol.DiagnosticSeverityError, *snapshot.diagnostics[0].Severity)
			tAssert.Equal(string(diagnosticImportPathNotMace), requireDiagnosticCode(snapshot.diagnostics[0]))
		}

		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), protocol.Range{
			Start: protocol.Position{Line: 1, Character: 0},
			End:   protocol.Position{Line: 1, Character: 21},
		}, "Append .mace to import path")

		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Equal(`"./shared.mace"`, edits[0].NewText)
		}
	})

	It("rejects remote directive urls without a .mace suffix in LSP diagnostics", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-directive-url-fix-*")
		tAssert.NoError(err)

		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`[output = data, schema = User, schema_file = "https://example.com/schema"]
{
  name: "Ada";
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "must end in .mace")
			tAssert.Equal(string(diagnosticImportPathNotMace), requireDiagnosticCode(snapshot.diagnostics[0]))
		}

		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 74},
		}, "Append .mace to import path")

		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Equal(`"https://example.com/schema.mace"`, edits[0].NewText)
		}
	})

	It("translates processor type mismatch errors into token-scoped diagnostics", func() {
		snapshot := analyzeDocument(`|===|
int count = "Ada";
|===|
[output = data]
{
  result: count;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "type mismatch")
			tAssert.Equal(protocol.UInteger(1), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(4), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticTypeInitializerMismatch), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("does not report diagnostics for a used nullable variable", func() {
		snapshot := analyzeDocument(`|===|
nullable string env = null;
|===|
[output = data] { value: env; }`)

		tAssert.Empty(snapshot.diagnostics)
	})

	It("warns when parse directives inject unknown runtime values", func() {
		snapshot := analyzeDocument(`|===|
schema Package: { name: string; project: string; };
|===|
[output = data, parse = Package]
{
  result: "ok";
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			diagnostic := snapshot.diagnostics[0]
			tAssert.Contains(diagnostic.Message, "The analyzer cannot know which runtime values will be injected")
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *diagnostic.Severity)
			tAssert.Equal(string(diagnosticDirectiveParseValuesUnknown), requireDiagnosticCode(diagnostic))
		}
	})

	It("warns when parse_file directives inject unknown runtime values", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-parse-file-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		writeAnalysisFile(workspace, "runtime.mace", `[output = schema]
{
  Package: { project: string; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`[output = data, parse_file = "./runtime.mace"]
{
  result: "ok";
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			diagnostic := snapshot.diagnostics[0]
			tAssert.Contains(diagnostic.Message, "The analyzer cannot know which runtime values will be injected")
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *diagnostic.Severity)
			tAssert.Equal(string(diagnosticDirectiveParseValuesUnknown), requireDiagnosticCode(diagnostic))
		}
	})

	It("reports direct null output fields", func() {
		snapshot := analyzeDocument(`[output = data]
{
  value: null;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "null can only be assigned to nullable variables and optional schema fields")
			tAssert.Equal(string(diagnosticTypeInvalidNullUsage), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("reports empty script blocks as syntax errors", func() {
		snapshot := analyzeDocument(`|===|
|===|
[output = data]
{}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "empty script block")
			tAssert.Equal(protocol.DiagnosticSeverityError, *snapshot.diagnostics[0].Severity)
			tAssert.Equal(string(diagnosticSyntaxEmptyScriptBlock), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("offers documentation generation actions for declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
|===|
[output = schema]
{
  User: User;
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 7},
			End:   protocol.Position{Line: 1, Character: 11},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Generate schema_doc")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `schema_doc User`)
		}
	})

	It("offers schema_doc generation for object-valued variables", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
User profile = { name: "Ada"; };
|===|
[output = data]
{
  profile: profile;
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 2, Character: 5},
			End:   protocol.Position{Line: 2, Character: 12},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Generate schema_doc")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `schema_doc profile`)
		}
	})

	It("offers schema output generation from schema declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string, age: int, active: boolean, tags: array<string>, meta: { id: string } };
|===|
[output = data]
{}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 7},
			End:   protocol.Position{Line: 1, Character: 11},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Generate output block from schema")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = data, schema = User]`)
			tAssert.Contains(edits[0].NewText, `name: ""`)
			tAssert.Contains(edits[0].NewText, `age: 0`)
			tAssert.Contains(edits[0].NewText, `active: false`)
			tAssert.Contains(edits[0].NewText, `tags: []`)
			tAssert.Contains(edits[0].NewText, `meta: {}`)
		}
	})

	It("offers schema directive insertion for data outputs", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
|===|
[output = data]
{
  name: "Ada";
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 7},
			End:   protocol.Position{Line: 1, Character: 11},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Add schema = User directive")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = data, schema = User]`)
		}
	})

	It("offers explicit output directives for implicit outputs", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`{
  name: "Ada";
}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Make implicit output explicit")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = data]`)
		}
	})

	It("offers conversion from data output to schema output", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = data]
{
  name: "Ada";
  age: 42;
  active: true;
}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Convert data output to schema output")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = schema]`)
			tAssert.Contains(edits[0].NewText, `name: string`)
			tAssert.Contains(edits[0].NewText, `age: int`)
			tAssert.Contains(edits[0].NewText, `active: boolean`)
		}
	})

	It("offers optional marker toggles for schema output fields", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = schema]
{
  name: string;
  age?: int;
}`, documentPath)

		nameRange := protocol.Range{
			Start: protocol.Position{Line: 2, Character: 2},
			End:   protocol.Position{Line: 2, Character: 6},
		}
		addAction := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), nameRange, "Add optional marker ?")
		addEdits := addAction.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(addEdits, 1) {
			tAssert.Contains(addEdits[0].NewText, `name?: string`)
		}

		ageRange := protocol.Range{
			Start: protocol.Position{Line: 3, Character: 2},
			End:   protocol.Position{Line: 3, Character: 5},
		}
		removeAction := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), ageRange, "Remove optional marker ?")
		removeEdits := removeAction.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(removeEdits, 1) {
			tAssert.Contains(removeEdits[0].NewText, `age: int`)
		}
	})

	It("offers import refactor actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
from "shared.mace" import User, Profile;
from "shared.mace" import Role;
|===|
[output = schema]
{}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		uri := protocol.DocumentUri(fileURI(documentPath))
		fixAction := requireCodeAction(snapshot, uri, rangeValue, "Fix relative import path")
		tAssert.Contains(fixAction.Edit.Changes[uri][0].NewText, `from "./shared.mace" import User, Profile;`)
		splitAction := requireCodeAction(snapshot, uri, rangeValue, "Split import declaration")
		tAssert.Contains(splitAction.Edit.Changes[uri][0].NewText, `from "shared.mace" import User;`)
		mergeAction := requireCodeAction(snapshot, uri, rangeValue, "Merge duplicate imports")
		tAssert.Contains(mergeAction.Edit.Changes[uri][0].NewText, `from "shared.mace" import User, Profile, Role;`)
	})

	It("offers import resolution actions", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-import-resolution-*")
		tAssert.NoError(err)
		defer func() {
			tAssert.NoError(os.RemoveAll(workspace))
		}()

		sharedPath := writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: string;
  Role: string;
}`)
		documentPath := writeAnalysisFile(workspace, "document.mace", `|===|
from "./missing.mace" import User;
from "./shared.mace" import Usre;
from "./shared-old.mace" import Role;
|===|
[output = schema]
{}`)
		contents, err := os.ReadFile(documentPath)
		tAssert.NoError(err)
		snapshot := analyzeDocumentAt(string(contents), documentPath)
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		createAction := requireCodeAction(snapshot, uri, rangeValue, "Create missing imported file")
		tAssert.Contains(createAction.Edit.Changes[protocol.DocumentUri(fileURI(filepath.Join(workspace, "missing.mace")))][0].NewText, "[output = schema]")

		renameAction := requireCodeAction(snapshot, uri, rangeValue, "Update import path after file rename")
		tAssert.Equal(`"./shared.mace"`, renameAction.Edit.Changes[uri][0].NewText)

		replaceAction := requireCodeAction(snapshot, uri, rangeValue, "Replace unavailable imported symbol with User")
		tAssert.Equal("User", replaceAction.Edit.Changes[uri][0].NewText)

		openAction := requireCodeAction(snapshot, uri, rangeValue, "Open source output block")
		tAssert.Equal(protocol.DocumentUri(fileURI(sharedPath)), openAction.Command.Arguments[0])

		explainAction := requireCodeAction(snapshot, uri, rangeValue, "Explain why symbol is not importable")
		tAssert.Contains(explainAction.Command.Arguments[0], "Only names surfaced through the imported file output block are importable")
	})

	It("reports unavailable imported output keys", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-import-unavailable-*")
		tAssert.NoError(err)
		defer func() {
			tAssert.NoError(os.RemoveAll(workspace))
		}()

		writeAnalysisFile(workspace, "shared.mace", `[output = data]
{
  name: "Ada";
}`)
		documentPath := writeAnalysisFile(workspace, "document.mace", `|===|
from "./shared.mace" import age;
|===|
[output = data]
{
  result: "ok";
}`)
		contents, err := os.ReadFile(documentPath)
		tAssert.NoError(err)
		snapshot := analyzeDocumentAt(string(contents), documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			diagnostic := snapshot.diagnostics[0]
			tAssert.Equal("mace.import.name-not-exposed", diagnostic.Code.Value)
			tAssert.Contains(diagnostic.Message, `imported key "age" is not exported by "./shared.mace"`)
			tAssert.Equal(protocol.Position{Line: 1, Character: 28}, diagnostic.Range.Start)
			tAssert.Equal(protocol.Position{Line: 1, Character: 31}, diagnostic.Range.End)
		}
	})

	It("allows parent-relative imports in analyzer contexts", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-import-boundary-*")
		tAssert.NoError(err)
		defer func() {
			tAssert.NoError(os.RemoveAll(workspace))
		}()

		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: string;
}`)
		consumerDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(consumerDir, 0o755))
		documentPath := writeAnalysisFile(consumerDir, "document.mace", `|===|
from "../shared.mace" import User;
|===|
[output = data]
{}`)
		contents, err := os.ReadFile(documentPath)
		tAssert.NoError(err)
		snapshot := analyzeDocumentAt(string(contents), documentPath)

		messages := lo.Map(snapshot.diagnostics, func(diagnostic protocol.Diagnostic, _ int) string {
			return diagnostic.Message
		})
		tAssert.False(lo.ContainsBy(messages, func(message string) bool {
			return strings.Contains(message, `escapes root`)
		}))
	})

	It("offers remaining add and fix import actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))

		snapshot := analyzeDocumentAt(`|===|
from "shared" import User;
from "zeta.mace" import Zed;
from "alpha.mace" import User;
from "dupes.mace" import User, User, Role;
|===|
[output = schema]
{}`, documentPath)
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		extensionAction := requireCodeAction(snapshot, uri, protocol.Range{Start: protocol.Position{Line: 1, Character: 5}, End: protocol.Position{Line: 1, Character: 13}}, "Append .mace to import path")
		tAssert.Equal(`"shared.mace"`, extensionAction.Edit.Changes[uri][0].NewText)

		sortAction := requireCodeAction(snapshot, uri, rangeValue, "Sort imports")
		tAssert.Contains(sortAction.Edit.Changes[uri][0].NewText, "from \"alpha.mace\" import User;\nfrom \"dupes.mace\" import User, User, Role;")

		duplicateAction := requireCodeAction(snapshot, uri, rangeValue, "Remove duplicate imported names")
		tAssert.Contains(duplicateAction.Edit.Changes[uri][0].NewText, `from "dupes.mace" import User, Role;`)

		wildcardSnapshot := analyzeDocumentAt(`|===|
from "shared.mace" import *;
|===|
[output = schema]
{}`, documentPath)
		wildcardAction := requireCodeAction(wildcardSnapshot, uri, protocol.Range{Start: protocol.Position{Line: 1, Character: 26}, End: protocol.Position{Line: 1, Character: 27}}, "Convert wildcard import to named import")
		tAssert.Equal("Name", wildcardAction.Edit.Changes[uri][0].NewText)
	})

	Describe("schema creation actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("extracts output block shapes into schemas", func() {
			snapshot := analyzeDocumentAt(`[output = data]
{ name: "Ada"; age: 30; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract output block shape into schema")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema Output:")
			tAssert.Contains(text, "name: string")
			tAssert.Contains(text, "schema = Output")
		})

		It("extracts record literals into schemas", func() {
			snapshot := analyzeDocumentAt(`|===|
User user = { name: "Ada"; };
|===|
[output = data]
{ value: user; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract record literal into schema")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema User:")
			tAssert.Contains(text, `User user = { name: "Ada"; };`)
		})

		It("creates schemas from selected fields", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; age: int; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Create schema from selected fields")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema Extracted:")
			tAssert.Contains(text, "name: string")
		})

		It("creates schemas from validation errors", func() {
			snapshot := analyzeDocumentAt(`[output = data, schema = User]
{ name: "Ada"; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Create schema from validation error")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "schema User:")
		})

		It("generates sample data from schemas", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; age: int; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Generate sample data from schema")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, `[output = data, schema = User]`)
			tAssert.Contains(text, `name: ""`)
		})
	})

	Describe("array actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("wraps types in arrays", func() {
			snapshot := analyzeDocumentAt(`|===|
type Name: string;
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Wrap type in array")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "type Name: array<string>;")
		})

		It("fixes mixed array literals with variants", func() {
			snapshot := analyzeDocumentAt(`|===|
array<string> values = ["Ada", 1];
|===|
[output = data]
{ value: values; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Fix mixed array literal")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ValuesItem: variant[string, int];")
			tAssert.Contains(text, "array<ValuesItem> values")
		})

		It("changes array element types to match literals", func() {
			snapshot := analyzeDocumentAt(`|===|
array<string> values = [1, 2];
|===|
[output = data]
{ value: values; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Change array element type")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "array<int> values")
		})

		It("replaces invalid array indexes", func() {
			snapshot := analyzeDocumentAt(`[output = data]
{ value: ["Ada"][3]; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Replace invalid array index")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `value: ["Ada"][0]`)
		})
	})

	Describe("variable fix actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("adds missing type annotations", func() {
			snapshot := analyzeDocumentAt(`|===|
name = "Ada";
title = "Engineer";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add missing type annotation")
			text := action.Edit.Changes[uri][0].NewText

			tAssert.Contains(text, `string name = "Ada";`)
			tAssert.Contains(text, `string title = "Engineer";`)
		})

		It("adds missing initializers", func() {
			snapshot := analyzeDocumentAt(`|===|
string name;
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add missing initializer")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `string name = "";`)
		})

		It("adds placeholder initializers", func() {
			snapshot := analyzeDocumentAt(`|===|
int count;
|===|
[output = data]
{ value: count; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add placeholder initializer")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `int count = 0;`)
		})

		It("changes variable type to inferred expression type", func() {
			snapshot := analyzeDocumentAt(`|===|
int name = "Ada";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Change variable type to inferred expression type")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `string name = "Ada";`)
		})

		It("changes initializers to match declared types", func() {
			snapshot := analyzeDocumentAt(`|===|
int count = "Ada";
|===|
[output = data]
{ value: count; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Change initializer to match declared type")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `int count = 0;`)
		})

		It("renames duplicate variables", func() {
			snapshot := analyzeDocumentAt(`|===|
string name = "Ada";
string name = "Grace";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Rename duplicate variable")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `string name_2 = "Grace";`)
		})

		It("inlines variables into output fields", func() {
			snapshot := analyzeDocumentAt(`|===|
string name = "Ada";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Inline variable into output field")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `value: "Ada"`)
		})

		It("extracts output expressions into script variables", func() {
			snapshot := analyzeDocumentAt(`[output = data]
{ value: "Ada"; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract output expression into script variable")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, `string value = "Ada";`)
			tAssert.Contains(text, `value: value`)
		})
	})

	Describe("declaration actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("adds missing semicolons after script declarations", func() {
			snapshot := analyzeDocumentAt(`|===|
type Name: string
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add missing semicolon")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "type Name: string;")
		})

		It("extracts repeated type references into an alias", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; email: string; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract repeated type into alias")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ExtractedType: string;")
			tAssert.Contains(text, "name: ExtractedType")
		})

		It("extracts variable type references into aliases", func() {
			snapshot := analyzeDocumentAt(`|===|
string name = "Ada";
|===|
[output = data]
{ value: name; }`, documentPath)
			targetRange := protocol.Range{Start: protocol.Position{Line: 1, Character: 1}, End: protocol.Position{Line: 1, Character: 1}}
			action := requireCodeAction(snapshot, uri, targetRange, "Extract variable type into alias")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ExampleType: string;")
			tAssert.Contains(text, "ExampleType name = \"Ada\";")
		})

		It("extracts individual schema field type references into aliases", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; age: int; profile: string; };
|===|
[output = schema]
{}`, documentPath)
			targetRange := protocol.Range{Start: protocol.Position{Line: 1, Character: 22}, End: protocol.Position{Line: 1, Character: 22}}
			action := requireCodeAction(snapshot, uri, targetRange, "Extract schema field name type into alias")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ExampleType: string;")
			tAssert.Contains(text, "name: ExampleType")
			tAssert.Contains(text, "age: int")
			tAssert.Contains(text, "profile: string")
		})

		It("extracts nested schema field type references into aliases", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { profile: { name: string; }; };
|===|
[output = schema]
{}`, documentPath)
			targetRange := protocol.Range{Start: protocol.Position{Line: 1, Character: 34}, End: protocol.Position{Line: 1, Character: 34}}
			action := requireCodeAction(snapshot, uri, targetRange, "Extract schema field profile.name type into alias")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ExampleType: string;")
			tAssert.Contains(text, "name: ExampleType")
			tAssert.Contains(text, "profile: {")
		})

		It("extracts inline record types into schemas", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { profile: { name: string; }; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract inline record type into schema")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema Profile:")
			tAssert.Contains(text, "profile: Profile")
		})

		It("converts record variables into schema-backed variables", func() {
			snapshot := analyzeDocumentAt(`|===|
{ name: string; } user = { name: "Ada"; };
|===|
[output = data]
{ value: user; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Convert record variable into schema-backed variable")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema User:")
			tAssert.Contains(text, "User user =")
		})
	})

	Describe("script block structure actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("creates a script block above the output block", func() {
			snapshot := analyzeDocumentAt(`[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Create script block")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\n|===|\n[output = schema]")
		})

		It("wraps the document in a script block", func() {
			snapshot := analyzeDocumentAt(`[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Wrap selection in script block")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\n[output = schema]")
		})

		It("fixes mismatched script delimiter widths", func() {
			snapshot := analyzeDocumentAt(`|====|
type Name: string;
|====|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Fix script delimiter length mismatch")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\ntype Name: string;")
		})

		It("normalizes script fences", func() {
			snapshot := analyzeDocumentAt(`|====|
type Name: string;
|====|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Normalize script fence")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\ntype Name: string;")
		})

		It("removes empty script blocks", func() {
			snapshot := analyzeDocumentAt(`|===|
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Remove empty script block")

			tAssert.NotContains(action.Edit.Changes[uri][0].NewText, "|===|\n|===|")
		})

		It("moves script blocks before output blocks", func() {
			snapshot := analyzeDocumentAt(`[output = schema]
{}
|===|
type Name: string;
|===|`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Move script block before output block")

			tAssert.True(strings.HasPrefix(action.Edit.Changes[uri][0].NewText, "|===|\ntype Name: string;"))
		})
	})

	It("offers documentation cleanup actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string /# inline; age: int; };
schema_doc User {
  summary: "Existing";
};
|===|
[output = schema]
{}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{Line: 1, Character: 0}, End: protocol.Position{Line: 1, Character: 60}}
		uri := protocol.DocumentUri(fileURI(documentPath))
		propsAction := requireCodeAction(snapshot, uri, rangeValue, "Add missing props docs")
		tAssert.Contains(propsAction.Edit.Changes[uri][0].NewText, `props: {`)
		moveAction := requireCodeAction(snapshot, uri, rangeValue, "Move inline /# docs to structured docs")
		tAssert.Contains(moveAction.Edit.Changes[uri][0].NewText, `name: ""`)
		removeAction := requireCodeAction(snapshot, uri, rangeValue, "Remove conflicting docs")
		tAssert.NotContains(removeAction.Edit.Changes[uri][0].NewText, `/# inline`)
	})

	It("offers string and style actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|====|
string name = "Ada";
|====|
[output = data]
{
  name: name;
}`, documentPath)

		uri := protocol.DocumentUri(fileURI(documentPath))
		stringRange := protocol.Range{Start: protocol.Position{Line: 1, Character: 14}, End: protocol.Position{Line: 1, Character: 19}}
		stringAction := requireCodeAction(snapshot, uri, stringRange, "Convert string form")
		tAssert.Contains(stringAction.Edit.Changes[uri][0].NewText, `'Ada'`)
		interpolatedAction := requireCodeAction(snapshot, uri, stringRange, "Convert to interpolated string")
		tAssert.Contains(interpolatedAction.Edit.Changes[uri][0].NewText, `"Ada $()"`)
		globalRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		separatorAction := requireCodeAction(snapshot, uri, globalRange, "Normalize separators")
		tAssert.Contains(separatorAction.Edit.Changes[uri][0].NewText, `name: name,`)
		fenceAction := requireCodeAction(snapshot, uri, globalRange, "Normalize script fence width")
		tAssert.Contains(fenceAction.Edit.Changes[uri][0].NewText, `|===|`)
	})

	It("offers expression and self refactor actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = data]
{
  first: "Ada";
  repeated: "Ada";
}`, documentPath)

		uri := protocol.DocumentUri(fileURI(documentPath))
		globalRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		extractAction := requireCodeAction(snapshot, uri, globalRange, "Extract expression into variable")
		tAssert.Contains(extractAction.Edit.Changes[uri][0].NewText, `extracted_value`)
		inlineAction := requireCodeAction(snapshot, uri, globalRange, "Inline variable into output")
		tAssert.NotNil(inlineAction.Edit)
		selfAction := requireCodeAction(snapshot, uri, globalRange, "Rewrite expression to use $self")
		tAssert.Contains(selfAction.Edit.Changes[uri][0].NewText, `$self.first`)
	})

	It("offers interop generation actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = schema]
{
  name: string;
}`, documentPath)

		uri := protocol.DocumentUri(fileURI(documentPath))
		globalRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		jsonAction := requireCodeAction(snapshot, uri, globalRange, "Generate JSON preview")
		tAssert.Contains(jsonAction.Edit.Changes[uri][0].NewText, `JSON preview`)
		maceAction := requireCodeAction(snapshot, uri, globalRange, "Generate Mace schema from sample data")
		tAssert.Contains(maceAction.Edit.Changes[uri][0].NewText, `schema Generated`)
		schemaAction := requireCodeAction(snapshot, uri, globalRange, "Generate JSON Schema from Mace schema")
		tAssert.Contains(schemaAction.Edit.Changes[uri][0].NewText, `JSON Schema`)
	})

	It("offers inline record extraction", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = schema]
{
  user: { name: string; };
}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{Line: 2, Character: 2}, End: protocol.Position{Line: 2, Character: 6}}
		uri := protocol.DocumentUri(fileURI(documentPath))
		action := requireCodeAction(snapshot, uri, rangeValue, "Convert inline record to schema")
		tAssert.Contains(action.Edit.Changes[uri][0].NewText, `schema User`)
		tAssert.Contains(action.Edit.Changes[uri][0].NewText, `user: User`)
	})

	It("offers inline description actions for type declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
type Name: string;
|===|
[output = schema]
{
  Name: Name;
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 5},
			End:   protocol.Position{Line: 1, Character: 9},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Add inline /# description")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Equal(` /# description`, edits[0].NewText)
		}
	})

	It("treats schema directives as import usages", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-schema-directive-import-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import User;
|===|
[output = data, schema = User]
{
  name: "Ada";
}`, documentPath)

		tAssert.Empty(snapshot.diagnostics)
	})

	It("inserts inline descriptions after complex type declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
type User: { name: string; };
|===|
[output = schema]
{
  User: User;
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 5},
			End:   protocol.Position{Line: 1, Character: 9},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Add inline /# description")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Equal(protocol.UInteger(1), edits[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(28), edits[0].Range.Start.Character)
		}
	})

	It("warns about unused imports and offers removal", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-unused-import-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: { enabled: boolean; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import User, Config;
User user = { name: "Ada"; };
|===|
[output = data]
{
  user: user;
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			diagnostic := snapshot.diagnostics[0]
			tAssert.Contains(diagnostic.Message, `import "Config" is never used`)
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *diagnostic.Severity)
			tAssert.Equal(string(diagnosticImportUnused), requireDiagnosticCode(diagnostic))

			action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnostic.Range, "Remove unused import")
			edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
			if tAssert.Len(edits, 1) {
				tAssert.Equal(``, edits[0].NewText)
				tAssert.Equal(protocol.UInteger(1), edits[0].Range.Start.Line)
				tAssert.Equal(protocol.UInteger(32), edits[0].Range.Start.Character)
				tAssert.Equal(protocol.UInteger(40), edits[0].Range.End.Character)
			}
		}
	})

	It("warns about unused script variables and offers removal", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
string unused = "Ada";
string name = "Grace";
|===|
[output = data]
{
  result: name;
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			diagnostic := snapshot.diagnostics[0]
			tAssert.Contains(diagnostic.Message, `script variable "unused" is never used`)
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *diagnostic.Severity)
			tAssert.Equal(string(diagnosticDeclarationUnusedVariable), requireDiagnosticCode(diagnostic))

			action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnostic.Range, "Remove unused variable")
			edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
			if tAssert.Len(edits, 1) {
				tAssert.Equal(``, edits[0].NewText)
				tAssert.Equal(protocol.UInteger(1), edits[0].Range.Start.Line)
				tAssert.Equal(protocol.UInteger(0), edits[0].Range.Start.Character)
				tAssert.Equal(protocol.UInteger(2), edits[0].Range.End.Line)
				tAssert.Equal(protocol.UInteger(0), edits[0].Range.End.Character)
			}
		}
	})

	It("warns about unused type declarations and offers removal", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
type Unused: string;
type Name: string;
schema User: { name: Name; };
|===|
[output = schema]
{
  User: User;
}`, documentPath)

		diagnostic, ok := lo.Find(snapshot.diagnostics, func(diagnostic protocol.Diagnostic) bool {
			return requireDiagnosticCode(diagnostic) == string(diagnosticDeclarationUnusedType)
		})
		if tAssert.True(ok) {
			tAssert.Contains(diagnostic.Message, `type "Unused" is never used`)
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *diagnostic.Severity)

			action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnostic.Range, "Remove unused type")
			edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
			if tAssert.Len(edits, 1) {
				tAssert.Equal(``, edits[0].NewText)
			}
		}
	})

	It("translates array literal initializer mismatches into token-scoped diagnostics", func() {
		snapshot := analyzeDocument(`|===|
array<int> foo = ["4", 6];
|===|
[output = data]
{
  result: 1;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "type mismatch: expected array<int>, got array<variant[string, int]>")
			tAssert.Equal(protocol.UInteger(1), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(11), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticTypeInitializerMismatch), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("translates schema output script variables into variable diagnostics", func() {
		snapshot := analyzeDocument(`|===|
type Name: string;
schema User: { name: Name; age: int; };
int local = 1;
|===|
[output = schema]
{
  Name: Name;
  User: User;
}`)

		diagnostic, ok := lo.Find(snapshot.diagnostics, func(diagnostic protocol.Diagnostic) bool {
			return requireDiagnosticCode(diagnostic) == string(diagnosticDirectiveSchemaOutputVariableIgnored)
		})
		if tAssert.True(ok) {
			tAssert.Contains(diagnostic.Message, `script variable "local" is not allowed`)
			tAssert.Equal(protocol.DiagnosticSeverityError, *diagnostic.Severity)
		}
	})

	It("translates data output type exports into value diagnostics", func() {
		snapshot := analyzeDocument(`|===|
type Name: string;
schema User: { name: string; };
string value = "Ada";
|===|
{
  Name: Name;
  User: User;
  value: value;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "cannot reference type or schema declaration")
			tAssert.Equal(protocol.UInteger(6), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(string(diagnosticTypeUnknownIdentifier), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("translates processor schema validation errors into schema-scoped diagnostics", func() {
		snapshot := analyzeDocument(`|===|
schema Point: { x: int; y: int; };
schema Plot: { points: array<Point>; };
|===|
[output = data, schema = Plot]
{
  points: [
    { x: 1; y: 2; },
    { x: 3; }
  ];
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, `missing required field "y"`)
			tAssert.Equal(protocol.UInteger(1), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(7), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticTypeRecordDoesNotMatchSchema), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("warns when schema_file overlaps with local imports and script context and offers two cleanup fixes", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-schema-file-conflict-*")
		tAssert.NoError(err)

		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import User;
schema User: { name: string; };
|===|
[output = data, schema = User, schema_file = "./shared.mace"]
{
  result: { name: "Ada"; };
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "redundant")
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *snapshot.diagnostics[0].Severity)
			tAssert.Equal(string(diagnosticDirectiveSchemaAndSchemaFileCombined), requireDiagnosticCode(snapshot.diagnostics[0]))
		}

		diagnosticRange := snapshot.diagnostics[0].Range
		removeDirective := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnosticRange, "Remove schema_file directive")
		removeContext := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnosticRange, "Remove imports and script block")

		tAssert.NotEmpty(removeDirective.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))])
		tAssert.NotEmpty(removeContext.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))])
	})

	It("errors when script variables are present in schema output mode", func() {
		snapshot := analyzeDocument(`|===|
schema User: { name: string; };
string value = "Ada";
|===|
[output = schema]
{
  User: User;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, `script variable "value" is not allowed when output = schema`)
			tAssert.Equal(protocol.DiagnosticSeverityError, *snapshot.diagnostics[0].Severity)
			tAssert.Equal(protocol.UInteger(2), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(7), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticDirectiveSchemaOutputVariableIgnored), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("translates processor self-reference failures into output-field diagnostics", func() {
		snapshot := analyzeDocument(`[output = data]
{
  result: $self.base;
  base: 4;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "unknown self reference")
			tAssert.Equal(protocol.UInteger(2), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(string(diagnosticTypeSelfForwardReference), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("recovers visible declarations for incomplete edits used by interactive LSP features", func() {
		snapshot := analyzeCompletionContext(`|===|
schema User: { name: string; };
Us`, "", protocol.Position{Line: 2, Character: 2})

		tAssert.True(snapshot.recovered)
		tAssert.Contains(declarationNames(snapshot), "User")
	})
})

func lexAnalysisTokens(text string) []lexer.Token {
	instance := lexer.New(text)
	tokens := []lexer.Token{}
	for {
		token, err := instance.NextToken()
		tAssert.NoError(err)
		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			return tokens
		}
	}
}

var _ = Describe("analyzer coverage helpers", func() {
	It("covers remaining small public and helper branches", func() {
		text := "[output = data]\n{\n  alpha: 1;\n}\n"
		snapshot := analyzeDocument(text)

		tAssert.Nil(Hover("", snapshot, protocol.Position{}))
		tAssert.NotNil(Hover("array", snapshot, protocol.Position{Character: 1}))
		tAssert.NotNil(Hover("[schema = User]", snapshot, protocol.Position{Character: 2}))
		tAssert.NotNil(Hover("schema User", snapshot, protocol.Position{Character: 1}))
		tAssert.Nil(Hover("missing", snapshot, protocol.Position{Character: 1}))
		tAssert.Empty(DocumentSymbols("", analysisSnapshot{}))
		_, ok := PrepareRename(snapshot, protocol.Position{Line: 99})
		tAssert.False(ok)
		_, ok = Rename(text, snapshot, "file:///doc.mace", protocol.Position{Line: 99}, "renamed")
		tAssert.False(ok)
		_, ok = Rename(text, snapshot, "file:///doc.mace", protocol.Position{Line: 2, Character: 4}, "")
		tAssert.False(ok)

		_, err := resolveBoundedPathInRoot("/tmp", "/tmp", "/abs.mace")
		tAssert.Error(err)
		_, err = resolveRootBoundedPathInRoot("/tmp/root/sub", "/tmp/root", "other.mace")
		tAssert.NoError(err)
		_, err = resolveRootBoundedPathInRoot("/tmp/root", "/tmp/root", "../outside.mace")
		tAssert.Error(err)
		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal("/", formatImportRoot(string(filepath.Separator)))
		tAssert.Contains(fileURI("C:/tmp/doc.mace"), "file:///")
		_, err = parseFile("\x00")
		tAssert.Error(err)
		_, err = parseExpression("\x00")
		tAssert.Error(err)

		tAssert.Equal("union[string, int]", typeReferenceDetail(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}))
		tAssert.Equal("variant[string, int]", typeReferenceDetail(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}))
		tAssert.Equal("choice[\"a\", 1]", typeReferenceDetail(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"a"`}, ast.IntLiteral{Lexeme: "1"}}}))
		tAssert.Equal("unknown", typeReferenceDetail(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}))
		start, end := nameRange("abc", "missing")
		tAssert.Equal(protocol.Position{}, start)
		tAssert.Equal(protocol.Position{}, end)
		tAssert.Empty(identifierPrefixAt("abc", protocol.Position{Line: 2}))
		identifier, ok := identifierAt("abc", protocol.Position{Line: 2})
		tAssert.True(ok)
		tAssert.Equal("abc", identifier)
		_, ok = identifierRangeAt("..", protocol.Position{Character: 1})
		tAssert.False(ok)
		tAssert.False(isDirectivePosition("abc", protocol.Position{Line: 2}))
		tAssert.False(isDirectivePosition("abc", protocol.Position{Character: 1}))
		tAssert.False(isDirectivePosition("[x] abc", protocol.Position{Character: 5}))
		tAssert.Equal(protocol.UInteger(3), utf16LineLength("a😀\nrest"))
	})

	It("covers refactor and edit helper branches", func() {
		file := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "a", Value: ast.IntLiteral{Lexeme: "1"}}, {Name: "b", Value: ast.IntLiteral{Lexeme: "1"}}}}}
		actions := []string{}
		addExpressionRefactorActions("[output = data]\n{\n  a: 1;\n  b: 1;\n}", file, protocol.Range{}, func(title string, _ protocol.Range, _ string) { actions = append(actions, title) })
		tAssert.Contains(actions, "Rewrite expression to use $self")
		tAssert.Equal("", simpleExpressionText(ast.ArrayLiteral{}))
		tAssert.Equal("false", simpleExpressionText(ast.BooleanLiteral{}))
		tAssert.Equal("'abc'", convertStringLiteralForm(`"abc"`))
		tAssert.Equal(`"abc"`, convertStringLiteralForm(`'abc'`))
		tAssert.Equal(`"abc"`, convertStringLiteralForm(`"""abc"""`))
		tAssert.IsType(ast.HexIntLiteral{}, defaultExpressionForType(ast.PrimitiveType{Name: "hex_int"}))
		tAssert.IsType(ast.HexFloatLiteral{}, defaultExpressionForType(ast.PrimitiveType{Name: "hex_float"}))
		tAssert.IsType(ast.RecordLiteral{}, defaultExpressionForType(ast.RecordType{}))
		tAssert.IsType(ast.PrimitiveType{}, inferredTypeFromExpression(ast.HexIntLiteral{}))
		tAssert.IsType(ast.PrimitiveType{}, inferredTypeFromExpression(ast.HexFloatLiteral{}))
		tAssert.IsType(ast.ArrayType{}, inferredTypeFromExpression(ast.ArrayLiteral{}))
		tAssert.IsType(ast.RecordType{}, inferredTypeFromExpression(ast.RecordLiteral{}))

		tokens := lexAnalysisTokens("|===|\nstring value = \"x\";\n|===|\n[output = data]\n{\n}\n")
		_, ok := documentationInsertRange("no script", nil)
		tAssert.False(ok)
		_, ok = declarationSemicolonInsertRange("", tokens, lexer.Token{Line: 99, Column: 1, Lexeme: "missing"})
		tAssert.False(ok)
		_, _, ok = missingSchemaFieldEdit("{}", ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, `missing required field "x"`)
		tAssert.False(ok)
		_, ok = unknownFieldEditRange("", ast.File{}, nil, "no match")
		tAssert.False(ok)
		_, ok = duplicateFieldEditRange("", nil, "no match")
		tAssert.False(ok)
		_, _, ok = missingImportEdit("", ast.File{Script: &ast.ScriptBlock{}}, nil, `unknown type "User"`)
		tAssert.False(ok)
		_, _, ok = moveImportsToTopEdit("", ast.File{}, nil, "other")
		tAssert.False(ok)
		_, ok = duplicateDeclarationEditRange("", ast.File{}, nil, "other")
		tAssert.False(ok)
		_, _, ok = declarationOperatorEdit("", lexAnalysisTokens("a: b"), "expected '='")
		tAssert.True(ok)
		_, _, ok = declarationOperatorEdit("", lexAnalysisTokens("a = b"), "expected ':'")
		tAssert.True(ok)
		_, _, ok = selfOrderingEdit("", ast.File{}, nil, "other")
		tAssert.False(ok)
		_, ok = invalidDirectiveComboEditRange("[output = schema]", ast.File{}, lexAnalysisTokens("[output = schema]"), "other")
		tAssert.False(ok)
		tAssert.Equal("TODO", placeholderForType(ast.File{}, "Custom"))
	})
})

var _ = Describe("analyzer diagnostic coverage helpers", func() {
	It("covers semantic diagnostic helper branches directly", func() {
		tokens := lexAnalysisTokens("|===|\nint count = \"bad\";\narray<int> values = [1, 2];\nnullable string maybe = null;\n|===|\n[output = data]\n{\n  result: values[4];\n  field: missing;\n}\n")
		file := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{
			ast.VariableDeclaration{HasValue: true, Type: ast.PrimitiveType{Name: "int"}, Name: "count", Value: ast.StringLiteral{Lexeme: `"bad"`}},
			ast.VariableDeclaration{HasValue: true, Type: ast.ArrayType{Element: ast.PrimitiveType{Name: "int"}}, Name: "values", Value: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"two"`}}}},
		}}, Output: ast.OutputBlock{DataFields: []ast.OutputField{{Name: "result"}}}}

		diagnostic, ok := variableTypeMismatchDiagnostic(file, tokens, processor.DiagnosticError{Code: processor.CodeTypeMismatch, Message: "bad", Fields: processor.DiagnosticFields{Expected: "int", Actual: "string"}})
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeInitializerMismatch), requireDiagnosticCode(diagnostic))
		_, ok = variableTypeMismatchDiagnostic(ast.File{}, tokens, processor.DiagnosticError{Code: processor.CodeTypeMismatch})
		tAssert.False(ok)
		_, ok = variableTypeMismatchDiagnostic(file, tokens, processor.DiagnosticError{Code: processor.CodeInternal})
		tAssert.False(ok)

		diagnostic, ok = nullUsageDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInvalidNullUsage, Message: "null bad"})
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeInvalidNullUsage), requireDiagnosticCode(diagnostic))
		_, ok = nullUsageDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInternal})
		tAssert.False(ok)

		diagnostic, ok = mixedArrayLiteralDiagnostic(file, tokens, "array literal has mixed element types")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeMixedArrayLiteral), requireDiagnosticCode(diagnostic))
		_, ok = mixedArrayLiteralDiagnostic(ast.File{}, tokens, "array literal has mixed element types")
		tAssert.False(ok)
		_, ok = mixedArrayLiteralDiagnostic(file, tokens, "other")
		tAssert.False(ok)

		candidates := arrayAccessCandidates(tokens)
		tAssert.NotEmpty(candidates)
		token, ok := invalidArrayAccessToken(candidates, processor.DiagnosticError{Fields: processor.DiagnosticFields{Level: 1}})
		tAssert.True(ok)
		tAssert.Equal("[", token.Lexeme)
		token, ok = outOfRangeArrayAccessToken(candidates, processor.DiagnosticError{Fields: processor.DiagnosticFields{Level: 1, Index: "4"}})
		tAssert.True(ok)
		tAssert.Equal("4", token.Lexeme)
		diagnostic, ok = arrayAccessDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeArrayIndexOutOfRange, Message: "range", Fields: processor.DiagnosticFields{Level: 1, Index: "4"}}, "range")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeInvalidArrayAccess), requireDiagnosticCode(diagnostic))
		diagnostic, ok = arrayAccessDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeArrayValueRequired, Message: "array", Fields: processor.DiagnosticFields{Level: 1}}, "array")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeInvalidArrayAccess), requireDiagnosticCode(diagnostic))
		_, ok = arrayAccessDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInternal}, "other")
		tAssert.False(ok)

		expected, actual, ok := parseExpectedAndActualType("processor: type mismatch: expected int, got string")
		tAssert.True(ok)
		tAssert.Equal("int", expected)
		tAssert.Equal("string", actual)
		_, _, ok = parseExpectedAndActualType("processor: type mismatch: expected int")
		tAssert.False(ok)

		diagnostic, ok = schemaDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeMissingRequiredField, Message: "missing", Fields: processor.DiagnosticFields{Schema: "count"}}, "missing")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeRecordDoesNotMatchSchema), requireDiagnosticCode(diagnostic))
		_, ok = schemaDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeMissingRequiredField}, "missing")
		tAssert.False(ok)
		diagnostic, ok = unknownSchemaDiagnostic(tokens, `unknown schema "count"`)
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticDirectiveUnknownSchemaName), requireDiagnosticCode(diagnostic))
		_, ok = unknownSchemaDiagnostic(tokens, "other")
		tAssert.False(ok)
		diagnostic, ok = selfReferenceDiagnostic(file, tokens, processor.DiagnosticError{Code: processor.CodeSelfReferenceUnknown, Message: "self", Fields: processor.DiagnosticFields{Name: "result"}}, "self")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeSelfForwardReference), requireDiagnosticCode(diagnostic))
		_, ok = selfReferenceDiagnostic(file, tokens, processor.DiagnosticError{Code: processor.CodeInternal}, "self")
		tAssert.False(ok)
		diagnostic, ok = dataOutputValueDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeOutputValueDeclaration, Message: "data", Fields: processor.DiagnosticFields{Name: "field"}}, "data")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeUnknownIdentifier), requireDiagnosticCode(diagnostic))
		_, ok = schemaOutputFieldDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInternal}, "schema")
		tAssert.False(ok)
	})

	It("covers summary and documentation helper branches directly", func() {
		known := map[string]string{"name": "string"}
		typeName, ok := expressionTypeSummary(ast.Identifier{Name: "name"}, known)
		tAssert.True(ok)
		tAssert.Equal("string", typeName)
		typeName, ok = arrayExpressionTypeSummary(ast.ArrayLiteral{}, known)
		tAssert.True(ok)
		tAssert.Equal("array<unknown>", typeName)
		typeName, ok = arrayExpressionTypeSummary(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"x"`}}}, known)
		tAssert.True(ok)
		tAssert.Equal("array<variant[int, string]>", typeName)
		_, ok = expressionTypeSummary(ast.RecordLiteral{}, known)
		tAssert.False(ok)

		file := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{
			ast.TypeDeclaration{Name: "Alias", Description: "Inline docs", Type: ast.PrimitiveType{Name: "string"}},
			ast.DocDeclaration{Target: "Alias", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"Summary"`}, Description: &ast.StringLiteral{Lexeme: `'''`}, Props: map[string]ast.StringLiteral{"field": {Lexeme: `"Field docs"`}}}},
		}}}
		tAssert.Contains(declarationDocumentation(file, "Alias"), "Inline docs")
		tAssert.Equal("multi", stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"""multi"""`}))
		tAssert.Equal("single", stringLiteralMarkdown(ast.StringLiteral{Lexeme: `'single'`}))
		tAssert.Equal(`"bad`, stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"bad`}))
		tAssert.Equal("one\n\ntwo", joinDocumentation("one", " ", "two"))
		tAssert.Equal("nullable string maybe", variableDeclarationDetail(ast.VariableDeclaration{Nullable: true, Type: ast.PrimitiveType{Name: "string"}, Name: "maybe"}))
		tAssert.Equal("string name = \"Ada\"", variableDeclarationDetail(ast.VariableDeclaration{HasValue: true, Type: ast.PrimitiveType{Name: "string"}, Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}))

		tAssert.Equal("unknown", summarizeValue(processor.Value{Kind: processor.ValueKind(999)}))
		tAssert.Equal("[1, false]", summarizeValue(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueInt, Int: 1}, {Kind: processor.ValueBoolean}}}))
		tAssert.Equal("{ a: string, b: array<unknown> }", valueTypeSummary(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"b": {Kind: processor.ValueArray}, "a": {Kind: processor.ValueString}}}))
		tAssert.Equal("runtime value", expressionSummary(nil))
		tAssert.Equal("expression", expressionSummary(ast.PrefixExpression{}))
		tAssert.Nil(fileScriptDeclarations(ast.File{}))
		tAssert.NotNil(fileScriptDeclarations(file))
	})
})

var _ = Describe("analyzer formatting coverage helpers", func() {
	It("covers CRLF formatting and script delimiter validation", func() {
		formatted := formatDocumentText("|=====|\r\nstring value = \"x\";\r\n|=====|\r\n")
		tAssert.Contains(formatted, "\r\n")
		tAssert.False(isScriptDelimiterLine("|==x|"))
		tAssert.False(isScriptDelimiterLine("====="))
		tAssert.True(isScriptDelimiterLine("|=====|"))
	})
})

var _ = Describe("analyzer rename coverage helpers", func() {
	It("renames imported alias references and external imported definitions", func() {
		workspace, err := os.MkdirTemp("", "mace-rename-coverage-*")
		tAssert.NoError(err)
		sharedPath := writeAnalysisFile(workspace, "shared.mace", `|===|
string remote = "value";
|===|
[output = data]
{
  remote: remote;
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")
		text := `|===|
from "./shared.mace" import remote: local;
|===|
[output = data]
{
  value: local;
}`
		snapshot := AnalyzeDocumentAtInRoot(text, documentPath, workspace)
		tAssert.Empty(Diagnostics(snapshot))
		uri := protocol.DocumentUri(fileURI(documentPath))

		edit, ok := Rename(text, snapshot, uri, protocol.Position{Line: 5, Character: 10}, "renamed")
		tAssert.True(ok)
		tAssert.Contains(edit.Changes, uri)
		tAssert.Len(edit.Changes[uri], 2)
		target := localImportRenameTarget(snapshot, protocol.Position{Line: 5, Character: 10}, semanticSymbol{Name: "local"})
		tAssert.False(target.ok)

		text = `|===|
from "./shared.mace" import remote;
|===|
[output = data]
{
  value: remote;
}`
		snapshot = AnalyzeDocumentAtInRoot(text, documentPath, workspace)
		tAssert.Empty(Diagnostics(snapshot))
		edit, ok = Rename(text, snapshot, uri, protocol.Position{Line: 5, Character: 10}, "renamed")
		tAssert.True(ok)
		tAssert.Contains(edit.Changes, protocol.DocumentUri(fileURI(sharedPath)))
	})

	It("covers direct rename token matching edge branches", func() {
		tokens := lexAnalysisTokens("field: value\nvalue")
		symbol := semanticSymbol{Name: "value", Kind: protocol.CompletionItemKindFunction, Range: tokenProtocolRange(tokens[2]), Definition: protocol.Location{Range: tokenProtocolRange(tokens[2])}}
		snapshot := analysisSnapshot{tokens: tokens}
		tAssert.False(renameTokenMatchesSymbol(snapshot, 0, tokenProtocolRange(tokens[0]), symbol))
		symbol.Kind = protocol.CompletionItemKindVariable
		tAssert.True(renameTokenMatchesSymbol(snapshot, 3, tokenProtocolRange(tokens[3]), symbol))
		tAssert.True(sameLocation(protocol.Location{URI: "file:///a", Range: tokenProtocolRange(tokens[0])}, protocol.Location{URI: "file:///a", Range: tokenProtocolRange(tokens[0])}))
		tAssert.False(rangesEqual(tokenProtocolRange(tokens[0]), tokenProtocolRange(tokens[2])))
	})
})

var _ = Describe("analyzer remaining low-coverage helpers", func() {
	It("covers successful quick-fix edit helper branches", func() {
		text := `|===|
string a = "x";
string a = "y";
|===|
[output = data]{result: a;}`
		tokens := lexAnalysisTokens(text)
		_, formatted, ok := moveImportsToTopEdit(text, ast.File{}, tokens, "import declarations must appear at top of script block")
		tAssert.True(ok)
		tAssert.NotEmpty(formatted)
		_, ok = duplicateDeclarationEditRange(text, ast.File{}, tokens, `duplicate declaration "a"`)
		tAssert.True(ok)
		_, _, ok = selfOrderingEdit(text, ast.File{}, tokens, "unknown self reference forward")
		tAssert.True(ok)
		tAssert.Equal(`""`, placeholderForType(ast.File{}, "string"))
		tAssert.Equal("0", placeholderForType(ast.File{}, "int"))
		tAssert.Equal("0.0", placeholderForType(ast.File{}, "float"))
		tAssert.Equal("0x0", placeholderForType(ast.File{}, "hex_int"))
		tAssert.Equal("0x0.0", placeholderForType(ast.File{}, "hex_float"))
		tAssert.Equal("false", placeholderForType(ast.File{}, "boolean"))
	})

	It("covers parse validation filtering branches", func() {
		file := ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Input"}}}}
		tAssert.True(shouldIgnoreParseValidationError(file, processor.DiagnosticError{Code: processor.CodeMissingRequiredField, Message: "missing"}))
		tAssert.True(shouldIgnoreParseValidationError(file, processor.DiagnosticError{Code: processor.CodeInternal, Message: "unknown field \"x\""}))
		tAssert.True(shouldIgnoreParseValidationError(file, processor.DiagnosticError{Code: processor.CodeInternal, Message: "field is not optional in schema"}))
		tAssert.False(shouldIgnoreParseValidationError(file, processor.DiagnosticError{Code: processor.CodeInternal, Message: "other"}))
		tAssert.False(shouldIgnoreParseValidationError(ast.File{}, processor.DiagnosticError{Code: processor.CodeMissingRequiredField, Message: "missing"}))
		tAssert.False(shouldIgnoreParseValidationError(file, fmt.Errorf("plain")))
		tAssert.True(hasParseValidationDirective([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile}}))
		diagnostic, ok := parseDirectiveWarningDiagnostic("[parse = Input]{}", file)
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticDirectiveParseValuesUnknown), requireDiagnosticCode(diagnostic))
	})
})

var _ = Describe("analyzer scalar diagnostic coverage helpers", func() {
	It("covers remaining scalar expression and output diagnostic branches", func() {
		tokens := lexAnalysisTokens("[output = schema]\n{\n  broken: Missing;\n}\n")
		diagnostic, ok := schemaOutputFieldDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInvalidOutputSchemaField, Message: "invalid", Fields: processor.DiagnosticFields{Name: "broken"}}, "invalid")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeInvalidOutputSchemaField), requireDiagnosticCode(diagnostic))
		_, ok = schemaOutputFieldDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInvalidOutputSchemaField, Fields: processor.DiagnosticFields{Name: "missing"}}, "invalid")
		tAssert.False(ok)

		for _, expression := range []ast.Expression{ast.FloatLiteral{}, ast.HexIntLiteral{}, ast.HexFloatLiteral{}, ast.BooleanLiteral{}} {
			_, ok = expressionTypeSummary(expression, nil)
			tAssert.True(ok)
		}
		_, ok = expressionTypeSummary(ast.Identifier{Name: "missing"}, map[string]string{})
		tAssert.False(ok)
		_, ok = arrayExpressionTypeSummary(ast.ArrayLiteral{Elements: []ast.Expression{ast.RecordLiteral{}}}, nil)
		tAssert.False(ok)
		tAssert.Equal("float", valueTypeSummary(processor.Value{Kind: processor.ValueFloat}))
		tAssert.Equal("hex_int", valueTypeSummary(processor.Value{Kind: processor.ValueHexInt}))
		tAssert.Equal("hex_float", valueTypeSummary(processor.Value{Kind: processor.ValueHexFloat}))
		tAssert.Equal("boolean", valueTypeSummary(processor.Value{Kind: processor.ValueBoolean}))
		tAssert.Equal("array<string>", valueTypeSummary(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueString}}}))
		tAssert.Equal("unknown", valueTypeSummary(processor.Value{Kind: processor.ValueKind(999)}))
		tAssert.Equal("true", expressionSummary(ast.BooleanLiteral{Value: true}))
		tAssert.Equal("array literal", expressionSummary(ast.ArrayLiteral{}))
		tAssert.Equal("record literal", expressionSummary(ast.RecordLiteral{}))
	})
})

var _ = Describe("analyzer declaration utility coverage helpers", func() {
	It("covers declaration, import, and usage helper branches", func() {
		tAssert.Equal("[]", defaultLiteralForTypeName("array<string>"))
		tAssert.Equal("0", defaultLiteralForTypeName("int"))
		tAssert.Equal("0.0", defaultLiteralForTypeName("float"))
		tAssert.Equal("0x0", defaultLiteralForTypeName("hex_int"))
		tAssert.Equal("0x0.0", defaultLiteralForTypeName("hex_float"))
		tAssert.Equal("false", defaultLiteralForTypeName("boolean"))
		tAssert.Equal(`""`, defaultLiteralForTypeName("string"))

		declaration := ast.VariableDeclaration{Name: "inserted", Type: ast.PrimitiveType{Name: "string"}}
		newScript := prependScriptItem(nil, declaration)
		tAssert.Len(newScript.Items, 1)
		existing := prependScriptItem(&ast.ScriptBlock{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./a.mace"`}}}, Items: []ast.Declaration{ast.VariableDeclaration{Name: "old"}}}, declaration)
		tAssert.Len(existing.Imports, 1)
		tAssert.Len(existing.Items, 2)

		text := `from "./shared.mace" import Remote: Local, Other;`
		tokens := lexAnalysisTokens(text)
		importDecl := ast.ImportDeclaration{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}}
		token, ok := importEntryToken(tokens, importDecl, ast.ImportedIdentifier{Name: "Remote", Alias: "Local"})
		tAssert.True(ok)
		tAssert.Equal("Local", token.Lexeme)
		token, ok = importEntryToken(tokens, importDecl, ast.ImportedIdentifier{Name: "Other"})
		tAssert.True(ok)
		tAssert.Equal("Other", token.Lexeme)
		_, ok = importIdentifierToken(tokens, importDecl, "Local")
		tAssert.False(ok)

		file := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{HasValue: true, Name: "local", Value: ast.InfixExpression{Left: ast.Identifier{Name: "left"}, Right: ast.ArrayAccess{Target: ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "field"}}}}}}, Output: ast.OutputBlock{DataFields: []ast.OutputField{{Name: "out", Value: ast.ConditionalExpression{Condition: ast.Identifier{Name: "cond"}, Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "then"}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "else", Value: ast.PrefixExpression{Right: ast.Identifier{Name: "otherwise"}}}}}}}}}}
		used := usedVariableNames(file)
		for _, name := range []string{"left", "record", "cond", "then", "otherwise"} {
			tAssert.Contains(used, name)
		}
	})
})

var _ = Describe("analyzer small branch coverage helpers", func() {
	It("covers residual token, diagnostic, and summary branches", func() {
		candidates := []arrayAccessCandidate{{Bracket: lexer.Token{Type: lexer.TokenLBracket, Lexeme: "["}, Index: &lexer.Token{Type: lexer.TokenInt, Lexeme: "1"}, Level: 2}}
		_, ok := outOfRangeArrayAccessToken(candidates, processor.DiagnosticError{Fields: processor.DiagnosticFields{Level: 1, Index: "1"}})
		tAssert.False(ok)
		_, ok = outOfRangeArrayAccessToken(candidates, processor.DiagnosticError{Fields: processor.DiagnosticFields{Level: 2, Index: "2"}})
		tAssert.False(ok)
		_, ok = outOfRangeArrayAccessToken([]arrayAccessCandidate{{Level: 1}}, processor.DiagnosticError{Fields: processor.DiagnosticFields{Level: 1, Index: "1"}})
		tAssert.False(ok)

		file := ast.File{Output: ast.OutputBlock{DataFields: []ast.OutputField{{Name: "known"}}}}
		tAssert.Equal(diagnosticTypeSelfForwardReference, classifySelfReferenceCode(file, "known"))
		tAssert.Equal(diagnosticTypeUnknownSelfField, classifySelfReferenceCode(file, "missing"))
		tAssert.Equal(0, outputFieldIndex(file, "known"))
		tAssert.Equal(-1, outputFieldIndex(file, "missing"))

		text := "[output = data]\n{\n  unknown: 1;\n  duplicate: 1;\n  duplicate: 2;\n}\n"
		tokens := lexAnalysisTokens(text)
		_, ok = unknownFieldEditRange(text, ast.File{}, tokens, `unknown field "unknown"`)
		tAssert.True(ok)
		_, ok = duplicateFieldEditRange(text, tokens, `duplicate field "duplicate"`)
		tAssert.True(ok)
		_, ok = unknownSchemaDiagnostic(tokens, `unknown schema "Missing"`)
		tAssert.False(ok)

		tAssert.Equal(`"hi"`, summarizeValue(processor.Value{Kind: processor.ValueString, String: "hi"}))
		tAssert.Equal("2", summarizeValue(processor.Value{Kind: processor.ValueInt, Int: 2}))
		tAssert.Equal("1.5", summarizeValue(processor.Value{Kind: processor.ValueFloat, Float: 1.5}))
		tAssert.Equal("true", summarizeValue(processor.Value{Kind: processor.ValueBoolean, Boolean: true}))
		tAssert.Equal("{ a: 1 }", summarizeValue(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"a": {Kind: processor.ValueInt, Int: 1}}}))
		tAssert.Empty(DocumentPath(protocol.DocumentUri("not a uri")))
		tAssert.Equal("ab", identifierPrefixAt("abc", protocol.Position{Character: 2}))
	})
})

var _ = Describe("analyzer code action coverage helpers", func() {
	It("covers import resolution diagnostics and quick fixes", func() {
		workspace, err := os.MkdirTemp("", "mace-import-actions-*")
		tAssert.NoError(err)
		defer os.RemoveAll(workspace)
		documentPath := filepath.Join(workspace, "consumer.mace")
		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		writeAnalysisFile(workspace, "renamed.mace", `[output = schema]
{
}`)
		text := `|===|
from "./shared.mace" import Usr;
from "./rename.mace" import Missing;
|===|
[output = data]
{
}`
		tokens := lexAnalysisTokens(text)
		file := ast.File{Imports: []ast.ImportDeclaration{
			{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Usr"}}},
			{Path: ast.StringLiteral{Lexeme: `"./rename.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}},
		}}
		actions := importResolutionCodeActions(text, file, tokens, documentPath)
		titles := lo.Map(actions, func(action analysisCodeActionCandidate, _ int) string { return action.Action.Title })
		tAssert.Contains(titles, "Open source output block")
		tAssert.Contains(titles, "Explain why symbol is not importable")
		tAssert.Contains(titles, "Create missing imported file")
		tAssert.Contains(titles, "Update import path after file rename")
		tAssert.Contains(titles, "Replace unavailable imported symbol with User")
		diagnostics := unavailableImportDiagnostics(file, tokens, documentPath)
		tAssert.NotEmpty(diagnostics)
		unavailable := unavailableImportNameSet(file, documentPath)
		tAssert.Contains(unavailable, importNameKey("./shared.mace", "Usr"))
		tAssert.Equal("./shared.mace\x00Usr", importNameKey("./shared.mace", "Usr"))
		closest, ok := closestName("Usr", []string{"User"})
		tAssert.True(ok)
		tAssert.Equal("User", closest)
		_, ok = closestName("CompletelyDifferent", []string{"User"})
		tAssert.False(ok)
		tAssert.Equal(0, levenshteinDistance("same", "same"))
		tAssert.NotEmpty(exportedOutputNames(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "value"}}}}))
	})

	It("covers semantic and documentation code action aggregators", func() {
		documentPath := filepath.Join(os.TempDir(), "mace-actions.mace")
		text := `|===|
type Alias: string;
string value = "x";
schema User: { name: string; };
|===|
[output = data, schema = User]
{
}`
		tokens := lexAnalysisTokens(text)
		file := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{
			ast.TypeDeclaration{NameToken: lexer.Token{Line: 2, Column: 6, Lexeme: "Alias"}, Name: "Alias", Type: ast.PrimitiveType{Name: "string"}},
			ast.VariableDeclaration{NameToken: lexer.Token{Line: 3, Column: 8, Lexeme: "value"}, Name: "value", HasValue: true, Type: ast.PrimitiveType{Name: "string"}, Value: ast.StringLiteral{Lexeme: `"x"`}},
			ast.SchemaDeclaration{NameToken: lexer.Token{Line: 4, Column: 8, Lexeme: "User"}, Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}},
		}}, Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}}
		diagnostic := diagnosticWithCode(protocol.Range{}, protocol.DiagnosticSeverityError, diagnosticTypeRecordDoesNotMatchSchema, `missing required field "name"`)
		actions := semanticCodeActions(text, file, tokens, documentPath, diagnostic, `missing required field "name"`)
		tAssert.NotEmpty(actions)
		tAssert.NotNil(documentationCodeActions(text, file, tokens, documentPath))
		tAssert.Nil(documentationCodeActions(text, file, tokens, ""))
		candidates := editorRefactorCodeActions(text, file, tokens, documentPath)
		tAssert.NotEmpty(candidates)
		tAssert.Nil(editorRefactorCodeActions(text, file, tokens, ""))
	})
})

var _ = Describe("analyzer removal action coverage helpers", func() {
	It("covers remove declaration action branches", func() {
		text := "|===|\nstring unused = \"x\";\n|===|\n[output = data]{}\n"
		tokens := lexAnalysisTokens(text)
		nameToken := lexer.Token{Line: 2, Column: 8, Lexeme: "unused"}
		actions := removeDeclarationAction(text, tokens, filepath.Join(os.TempDir(), "remove.mace"), protocol.Range{}, nameToken, "Remove unused variable")
		tAssert.NotEmpty(actions)
		tAssert.Nil(removeDeclarationAction(text, tokens, "", protocol.Range{}, nameToken, "Remove"))
		tAssert.Nil(removeDeclarationAction(text, tokens, filepath.Join(os.TempDir(), "remove.mace"), protocol.Range{}, lexer.Token{Line: 99, Column: 1, Lexeme: "missing"}, "Remove"))
	})
})

var _ = Describe("Analyzer package edge helpers", func() {
	It("covers path resolution and URI helper edge cases", func() {
		workspace, err := os.MkdirTemp("", "mace-analyzer-paths-*")
		tAssert.NoError(err)
		subdir := filepath.Join(workspace, "project", "nested")
		tAssert.NoError(os.MkdirAll(subdir, 0o755))

		resolved, err := resolveBoundedPath(subdir, "./schema.mace")
		tAssert.NoError(err)
		tAssert.Equal(filepath.Join(subdir, "schema.mace"), resolved)

		_, err = resolveBoundedPath(subdir, filepath.Join(string(filepath.Separator), "tmp", "schema.mace"))
		tAssert.Error(err)

		_, err = resolveRootBoundedPathInRoot(subdir, filepath.Join(workspace, "project"), "../outside.mace")
		tAssert.Error(err)

		resolved, err = resolveRootBoundedPathInRoot(subdir, filepath.Join(workspace, "project"), "./schema.mace")
		tAssert.NoError(err)
		tAssert.Equal(filepath.Join(subdir, "schema.mace"), resolved)

		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("project/", formatImportRoot(filepath.Join(workspace, "project")))

		windowsURI := fileURI("C:/Users/Ada/schema.mace")
		tAssert.Contains(windowsURI, "file:///C:/Users/Ada/schema.mace")
		tAssert.Equal("", DocumentPath(protocol.DocumentUri("not a uri")))
	})

	It("covers identifier, position, and hover edge cases", func() {
		text := "[output = data]\n{\n  title: \"hi😀\";\n}"
		tAssert.Equal("out", identifierPrefixAt(text, protocol.Position{Line: 0, Character: 4}))
		identifier, ok := identifierAt(text, protocol.Position{Line: 0, Character: 2})
		tAssert.True(ok)
		tAssert.Equal("output", identifier)
		_, ok = identifierAt(text, protocol.Position{Line: 1, Character: 0})
		tAssert.False(ok)

		rangeValue, ok := identifierRangeAt(text, protocol.Position{Line: 99, Character: 0})
		tAssert.False(ok)
		tAssert.Equal(protocol.Range{}, rangeValue)
		tAssert.True(isDirectivePosition(text, protocol.Position{Line: 0, Character: 2}))
		tAssert.False(isDirectivePosition(text, protocol.Position{Line: 2, Character: 2}))
		tAssert.Equal(protocol.UInteger(5), utf16LineLength("hi😀!\nignored"))

		snapshot := analyzeDocument(text)
		tAssert.NotNil(Hover(text, snapshot, protocol.Position{Line: 0, Character: 2}))
		tAssert.Nil(Hover(text, snapshot, protocol.Position{Line: 1, Character: 0}))
	})

	It("covers public symbol and rename fallback branches", func() {
		tAssert.Empty(DocumentSymbols("", analysisSnapshot{}))
		text := `|===|
type Alias: string;
schema User: { name: string; };
|===|
[output = schema]
{
  name: string;
}`
		snapshot := analyzeDocument(text)
		symbols := DocumentSymbols(text, snapshot)
		tAssert.NotEmpty(symbols)
		_, ok := PrepareRename(snapshot, protocol.Position{Line: 99, Character: 0})
		tAssert.False(ok)
		edit, ok := Rename(text, snapshot, protocol.DocumentUri("file:///tmp/doc.mace"), protocol.Position{Line: 1, Character: 5}, "")
		tAssert.False(ok)
		tAssert.Nil(edit)
	})

	It("covers hover documentation and rename edit edge cases", func() {
		text := "Imported"
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{Character: 8}}
		documented := analysisSnapshot{
			text: text,
			symbols: []semanticSymbol{{
				Name:          "Imported",
				Detail:        "import Imported: string",
				Documentation: "Imported documentation.",
				Range:         rangeValue,
			}},
		}
		hover := Hover(text, documented, protocol.Position{Character: 2})
		tAssert.NotNil(hover)
		if hover != nil {
			content, ok := hover.Contents.(protocol.MarkupContent)
			tAssert.True(ok)
			if ok {
				tAssert.Contains(content.Value, "Imported documentation.")
			}
		}

		snapshot := analysisSnapshot{
			text:        text,
			documentURI: protocol.DocumentUri("file:///workspace/current.mace"),
			tokens:      []lexer.Token{{Type: lexer.TokenIdentifier, Lexeme: "Imported", Line: 1, Column: 1}},
			symbols: []semanticSymbol{{
				Name:   "Imported",
				Origin: symbolOriginImport,
				Range:  rangeValue,
				Definition: protocol.Location{
					URI:   protocol.DocumentUri("file:///workspace/shared.mace"),
					Range: rangeValue,
				},
			}},
		}
		edit, ok := Rename(text, snapshot, protocol.DocumentUri("file:///workspace/fallback.mace"), protocol.Position{Character: 2}, "Renamed")
		tAssert.True(ok)
		if edit != nil {
			tAssert.Contains(edit.Changes, protocol.DocumentUri("file:///workspace/current.mace"))
			tAssert.Contains(edit.Changes, protocol.DocumentUri("file:///workspace/shared.mace"))
		}
	})

	It("covers additional directive and root-boundary helper branches", func() {
		tAssert.False(isDirectivePosition("output = data]", protocol.Position{Character: 3}))
		tAssert.True(isDirectivePosition("[output = data", protocol.Position{Character: 5}))
		_, err := resolveRootBoundedPathInRoot("/workspace", "/workspace", filepath.Join(string(filepath.Separator), "tmp", "schema.mace"))
		tAssert.Error(err)
	})

})

var _ = Describe("analyzer completion coverage helpers", func() {
	completionAt := func(textWithCursor string, documentPath string, root string) []protocol.CompletionItem {
		cursor := strings.Index(textWithCursor, "•")
		tAssert.NotEqual(-1, cursor)
		text := strings.Replace(textWithCursor, "•", "", 1)
		position := positionFromIndex(text, cursor)
		snapshot := AnalyzeCompletionContextInRoot(text, documentPath, root, position)
		return CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
	}

	It("covers import, directive, initializer, and self completion paths", func() {
		workspace, err := os.MkdirTemp("", "mace-analyzer-completion-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; tags: array<string>; };
  Role: string;
}`)
		writeAnalysisFile(workspace, "runtime.mace", `[output = schema]
{
  Runtime: { env: string; profile: { name: string; }; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")

		items := completionAt(`|===|
from "./sh•
|===|
[output = data]
{}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`|===|
from "./shared.mace" import U•
|===|
[output = data]
{}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`|===|
from "./shared.mace" import User;
schema Local: { name: string; tags: array<string>; };
|===|
[output = data, schema = L•]
{
  name: "Ada";
}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`|===|
from "./shared.mace" import User;
|===|
[output = data, schema_file = "./sh•"]
{}`, documentPath, workspace)
		tAssert.Empty(items)

		items = completionAt(`|===|
schema User: { name: string; tags: array<string>; };
|===|
[output = data, schema = User]
{
  name: •
}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`|===|
schema Runtime: { env: string; profile: { name: string; }; };
|===|
[output = data, parse = Runtime]
{
  value: profile.•
}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`[output = data]
{
  profile: { name: "Ada"; };
  value: $self.profile.•
}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`|===|
array<string> names = ["Ada"];
string first = names[•];
|===|
[output = data]
{}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`|===|
type Color: choice["red", "blue"];
Color color = "r•";
|===|
[output = data]
{}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`|===|
type Color: choice["red", "blue"];
schema User: { favorite: Color; };
|===|
[output = data, schema = User]
{
  favorite: "b•";
}`, documentPath, workspace)
		tAssert.NotEmpty(items)

		items = completionAt(`[output = data]
{
  value: $se•
}`, documentPath, workspace)
		tAssert.NotEmpty(items)
	})

	It("covers direct completion helper edge branches", func() {
		model := completionModel{
			aliases: map[string]ast.TypeReference{
				"Alias": ast.PrimitiveType{Name: "string"},
			},
			schemas: map[string]ast.RecordType{
				"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "tags", Type: ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}}}},
			},
			variables: map[string]ast.TypeReference{},
		}

		position, ok := completionPlaceholderPosition("name  : value", protocol.Position{Character: 4}, ":")
		tAssert.True(ok)
		tAssert.Equal(protocol.UInteger(7), position.Character)
		_, ok = completionPlaceholderPosition("name value", protocol.Position{Character: 4}, ":")
		tAssert.False(ok)

		tAssert.Equal("}])", completionExpressionClosers("value: call([{", len("value: call([{")))
		tAssert.Equal("", completionExpressionClosers("value", -1))
		tAssert.Equal([]byte{'('}, popCompletionDelimiter([]byte{'('}, '['))
		tAssert.Empty(popCompletionDelimiter([]byte{'('}, '('))

		path, ok := trailingMemberAccessPath("  profile.name.")
		tAssert.True(ok)
		tAssert.Equal([]string{"profile", "name"}, path)
		_, ok = trailingMemberAccessPath("$self.profile.")
		tAssert.False(ok)
		_, ok = trailingMemberAccessPath("profile..")
		tAssert.False(ok)

		path, ok = placeholderPath(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "profile", Value: ast.ArrayLiteral{Elements: []ast.Expression{ast.MemberAccess{Target: ast.Identifier{Name: completionPlaceholderIdentifier}, Name: "name"}}}}}})
		tAssert.True(ok)
		tAssert.Equal([]string{"profile", completionArrayPathSegment, "name"}, path)
		path, ok = placeholderPath(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "other"}, Else: ast.Identifier{Name: completionPlaceholderIdentifier}})
		tAssert.True(ok)
		tAssert.Empty(path)
		_, ok = expressionPath(ast.MemberAccess{Target: ast.Identifier{}, Name: "name"})
		tAssert.False(ok)

		typeRef, ok := completionTypeAtPath(ast.NamedType{Name: "User"}, []string{"tags", completionArrayPathSegment}, model)
		tAssert.True(ok)
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, typeRef)
		_, ok = completionTypeAtPath(ast.NamedType{Name: "User"}, []string{"missing"}, model)
		tAssert.False(ok)

		items := completionItemsForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"red"`}, ast.StringLiteral{Lexeme: `"blue"`}}}, model, completionOptions{unquotedStringChoices: true, unquotedStringChoiceText: "r"})
		tAssert.Len(items, 1)
		tAssert.Equal("red", items[0].Label)
		items = completionItemsForType(ast.NamedType{Name: "User"}, model, completionOptions{allowSchemaLiteral: true})
		tAssert.NotEmpty(items)
		tAssert.NotEmpty(completionItemsForMemberTarget(ast.NamedType{Name: "User"}, model))
		tAssert.Empty(completionItemsForValueMembers(processor.Value{Kind: processor.ValueString}))
		tAssert.NotEmpty(completionItemsForValueMembers(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString}}}))

		value := syntheticCompletionValue(ast.NamedType{Name: "User"}, model, 3)
		tAssert.Equal(processor.ValueRecord, value.Kind)
		tAssert.Equal(processor.ValueUnknown, syntheticCompletionValue(ast.NamedType{Name: "User"}, model, 0).Kind)
		tAssert.Equal(`""`, defaultLiteralForType(ast.PrimitiveType{Name: "unknown"}, model, map[string]struct{}{}))
		tAssert.Equal("[]", defaultLiteralForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{}))
		tAssert.Equal("{}", defaultLiteralForType(ast.NamedType{Name: "User"}, model, map[string]struct{}{recordTypeDetail(model.schemas["User"]): {}}))
	})
	It("covers filesystem and partial-output completion helpers", func() {
		workspace, err := os.MkdirTemp("", "mace-analyzer-completion-fs-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()
		sharedPath := writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Alias: string;
}`)
		writeAnalysisFile(workspace, "nested/child.mace", `[output = data] { value: 1; }`)
		documentPath := filepath.Join(workspace, "consumer.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))

		pathValue, ok := documentPathFromURI(uri)
		tAssert.True(ok)
		tAssert.Equal(documentPath, pathValue)
		_, ok = documentPathFromURI(protocol.DocumentUri("https://example.com/file.mace"))
		tAssert.False(ok)
		_, ok = documentPathFromURI(protocol.DocumentUri("file:///%ZZ"))
		tAssert.False(ok)

		items := relativePathItems(document{}, uri, "./", nil, true)
		tAssert.NotEmpty(items)
		items = relativePathItems(document{}, protocol.DocumentUri("not a uri"), "./", nil, true)
		tAssert.Empty(items)

		symbols, ok := importableSymbols(uri, workspace, "./shared.mace")
		tAssert.True(ok)
		tAssert.NotEmpty(symbols)
		identifiers, ok := importableIdentifiers(uri, workspace, "./shared.mace")
		tAssert.True(ok)
		tAssert.Contains(identifiers, "User")
		_, ok = importableSymbols(protocol.DocumentUri("not a uri"), workspace, "./shared.mace")
		tAssert.False(ok)
		_, ok = importableSymbols(uri, workspace, "./missing.mace")
		tAssert.False(ok)

		text := `|===|
from "./shared.mace" import User;
schema Local: { id: int; };
|===|
[output = data, schema = `
		doc := document{text: text, analysis: AnalyzeDocumentAtInRoot(text+"Local] {}", documentPath, workspace)}
		names := availableSchemaNames(doc, uri, "[output = data, schema = ")
		tAssert.Contains(names, "Local")
		tAssert.NotNil(completionFile(doc, "[output = data"))
		tAssert.Nil(completionFile(document{text: "no output here"}, ""))
		tAssert.Nil(completionFile(document{text: "[output = data]"}, "[output = data]"))
		tAssert.Contains(importedPaths(doc, "[output = data"), "./shared.mace")
		tAssert.Empty(importedPaths(document{text: "[output = data]"}, "[output = data"))
		tAssert.Equal(workspace, completionRoot(analysisSnapshot{importRootDir: workspace}, uri))

		partialText := `[output = data]
{
  profile: { name: "Ada"; };
  value: $self.profile.
}`
		position := positionFromIndex(partialText, strings.Index(partialText, "$self.profile.")+len("$self.profile."))
		result, ok := partialOutputResult(document{text: partialText}, uri, position)
		tAssert.True(ok)
		tAssert.Contains(result.Output, "profile")
		value, ok := selfCompletionValue(document{text: partialText}, uri, position, []string{"profile"})
		tAssert.True(ok)
		tAssert.Equal(processor.ValueRecord, value.Kind)
		tAssert.Contains(selfCompletionEntries(value), "name")
		_, ok = selfCompletionValue(document{text: partialText}, uri, position, []string{"profile", "missing"})
		tAssert.False(ok)
		tAssert.Nil(selfCompletionEntries(processor.Value{Kind: processor.ValueString}))

		tokens := lexAnalysisTokens(partialText)
		openIndex, ok := outputBlockOpenIndex(partialText, tokens)
		tAssert.True(ok)
		ranges, ok := outputFieldRanges(partialText, tokens, openIndex)
		tAssert.True(ok)
		tAssert.NotEmpty(ranges)
		_, ok = outputBlockOpenIndex("|===|\n{\n|===|", lexAnalysisTokens("|===|\n{\n|===|"))
		tAssert.False(ok)
		_, ok = outputFieldRanges("{}", lexAnalysisTokens("{}"), 0)
		tAssert.False(ok)
		tAssert.False(isOutputFieldHeader([]lexer.Token{{Type: lexer.TokenIdentifier}}, 0))

		_ = sharedPath
	})

	It("covers remaining focused analyzer helper fallbacks", func() {
		tokens := lexAnalysisTokens("value: null;")
		_, ok := nullUsageDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInvalidNullUsage, Message: "null"})
		tAssert.True(ok)
		_, ok = nullUsageDiagnostic(lexAnalysisTokens("value: 1;"), processor.DiagnosticError{Code: processor.CodeInvalidNullUsage, Message: "null"})
		tAssert.False(ok)

		file := ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./runtime.mace"`}}}}
		_, ok = parseInputSemanticSchemaName(file, ".")
		tAssert.False(ok)
		file.Output.Directives = append(file.Output.Directives, ast.OutputDirective{Kind: ast.OutputDirectiveSchema, Value: "Runtime"})
		name, ok := parseInputSemanticSchemaName(file, ".")
		tAssert.True(ok)
		tAssert.Equal("Runtime", name)

		path, prefix, ok := selfCompletionContext("value: $self.profile.na")
		tAssert.True(ok)
		tAssert.Equal([]string{"profile"}, path)
		tAssert.Equal("na", prefix)
		path, prefix, ok = selfCompletionContext("value: $self.profile.")
		tAssert.True(ok)
		tAssert.Equal([]string{"profile"}, path)
		tAssert.Equal("", prefix)
		_, _, ok = selfCompletionContext("value: $self.profile[0]")
		tAssert.False(ok)

		model := completionModel{aliases: map[string]ast.TypeReference{}, schemas: map[string]ast.RecordType{}, variables: map[string]ast.TypeReference{}}
		_, _, ok = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, model)
		tAssert.False(ok)
		_, _, ok = placeholderParseInputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, model, ".", ".")
		tAssert.False(ok)
	})

	It("covers additional analysis helper fallback branches directly", func() {
		record := ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}
		updated, ok := replaceSchemaFieldType(record, []string{"profile", "name"}, ast.PrimitiveType{Name: "int"})
		tAssert.True(ok)
		tAssert.Equal(ast.PrimitiveType{Name: "int"}, updated.Fields[0].Type.(ast.RecordType).Fields[0].Type)
		_, ok = replaceSchemaFieldType(record, nil, ast.PrimitiveType{Name: "int"})
		tAssert.False(ok)
		_, ok = replaceSchemaFieldType(record, []string{"missing"}, ast.PrimitiveType{Name: "int"})
		tAssert.False(ok)
		_, ok = replaceSchemaFieldType(record, []string{"profile", "missing"}, ast.PrimitiveType{Name: "int"})
		tAssert.False(ok)

		schemaFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}
		symbol, ok := importedImportAsSemanticSymbol(schemaFile, filepath.Join(os.TempDir(), "shared.mace"), "Shared")
		tAssert.True(ok)
		tAssert.Equal(protocol.CompletionItemKindStruct, symbol.Kind)
		dataFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"x"`}}}}}
		symbol, ok = importedImportAsSemanticSymbol(dataFile, filepath.Join(os.TempDir(), "data.mace"), "Data")
		tAssert.True(ok)
		tAssert.Equal(protocol.CompletionItemKindVariable, symbol.Kind)
		_, ok = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, filepath.Join(os.TempDir(), "empty.mace"), "Empty")
		tAssert.False(ok)

		_, ok = importAndScriptCleanupRange("[output = data]{}")
		tAssert.False(ok)
		_, ok = importAndScriptCleanupRange("   [output = data]{}")
		tAssert.False(ok)
		rangeValue, ok := importAndScriptCleanupRange("|===|\nstring value = \"x\";\n|===|\n[output = data]{}")
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{}, rangeValue.Start)
		tAssert.NotEqual(protocol.Position{}, rangeValue.End)
		tAssert.Equal("", quotedName("no quotes"))
		tAssert.Equal("", quotedName("unterminated \"quote"))
		tAssert.Equal("name", quotedName(`unknown "name" here`))
	})

	It("covers choice, union, and default literal completion helpers", func() {
		model := completionModel{
			aliases: map[string]ast.TypeReference{
				"Color": ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"red"`}, ast.StringLiteral{Lexeme: `"blue"`}}},
				"Loop":  ast.NamedType{Name: "Loop"},
			},
			schemas: map[string]ast.RecordType{
				"Left":  {Fields: []ast.SchemaField{{Name: "id", Type: ast.PrimitiveType{Name: "string"}, Optional: true}}},
				"Right": {Fields: []ast.SchemaField{{Name: "id", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Type: ast.PrimitiveType{Name: "int"}}}},
			},
			variables: map[string]ast.TypeReference{},
		}

		members, ok := completionChoiceMemberValues(ast.Identifier{Name: "Color"}, model, map[string]struct{}{})
		tAssert.True(ok)
		tAssert.Len(members, 2)
		_, ok = completionChoiceMemberValues(ast.Identifier{Name: "Missing"}, model, map[string]struct{}{})
		tAssert.False(ok)
		_, ok = completionChoiceMemberValues(ast.Identifier{Name: "Loop"}, model, map[string]struct{}{})
		tAssert.False(ok)
		_, ok = completionChoiceMemberValues(ast.RecordLiteral{}, model, map[string]struct{}{})
		tAssert.False(ok)

		choice, ok := completionChoiceFromMembers([]ast.Expression{ast.Identifier{Name: "Color"}, ast.StringLiteral{Lexeme: `"red"`}}, model, map[string]struct{}{})
		tAssert.True(ok)
		tAssert.Len(choice.members, 2)
		_, ok = completionChoiceFromMembers([]ast.Expression{ast.Identifier{Name: "Missing"}}, model, map[string]struct{}{})
		tAssert.False(ok)

		record, ok := completionUnionRecord([]ast.TypeReference{ast.NamedType{Name: "Left"}, ast.NamedType{Name: "Right"}}, model, map[string]struct{}{})
		tAssert.True(ok)
		tAssert.Len(record.Fields, 2)
		_, ok = completionUnionRecord([]ast.TypeReference{ast.NamedType{Name: "Left"}, ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{})
		tAssert.False(ok)
		_, ok = completionUnionRecord([]ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "id", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "id", Type: ast.PrimitiveType{Name: "int"}}}}}, model, map[string]struct{}{})
		tAssert.False(ok)

		tAssert.Equal(`"red"`, defaultLiteralForType(ast.NamedType{Name: "Color"}, model, map[string]struct{}{}))
		tAssert.Equal(`""`, defaultLiteralForType(ast.ChoiceType{}, model, map[string]struct{}{}))
		tAssert.Equal(`"red"`, defaultLiteralForType(ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "Color"}, ast.NamedType{Name: "Left"}}}, model, map[string]struct{}{}))
		tAssert.Equal("{}", defaultLiteralForType(ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, model, map[string]struct{}{}))
		tAssert.Equal("{}", defaultLiteralForType(ast.NamedType{Name: "Missing"}, model, map[string]struct{}{}))
	})

})

var _ = Describe("analyzer lowest-percentage completion helpers", func() {
	It("targets placeholder file construction and line context fallbacks", func() {
		text := "|===|\nstring name = \n"
		position := protocol.Position{Line: 1, Character: protocol.UInteger(len("string name = "))}
		file, ok := completionFileWithPlaceholder(text, position)
		tAssert.True(ok)
		tAssert.NotNil(file)
		file, ok = completionFileWithExpressionPlaceholder(text, len(text), len(text))
		tAssert.True(ok)
		tAssert.NotNil(file)
		_, ok = completionFileWithPlaceholder("no script", protocol.Position{Line: 9})
		tAssert.False(ok)
		_, ok = completionFileWithExpressionPlaceholder("abc", -1, 1)
		tAssert.False(ok)
		_, ok = completionFileWithExpressionPlaceholder("abc", 2, 1)
		tAssert.False(ok)
		_, ok = completionFileWithExpressionPlaceholder("abc", 0, 99)
		tAssert.False(ok)
		_, ok = partialScriptFileWithPlaceholder("no delimiter here", protocol.Position{})
		tAssert.False(ok)
		_, ok = partialScriptFileWithPlaceholder("|===|\nstring name = completion;", protocol.Position{Line: 1, Character: 14})
		tAssert.True(ok)

		tAssert.Equal("", currentLinePrefix("one", protocol.Position{Line: 2}))
		tAssert.Equal("one", currentLineSuffix("one", protocol.Position{Line: 2}))
		value, ok := stringLiteralValue(ast.StringLiteral{Lexeme: `"unterminated`})
		tAssert.False(ok)
		tAssert.Equal("", value)
		_, ok = stringLiteralCompletionContext("value: \"unterminated", protocol.Position{Character: 8})
		tAssert.True(ok)
		_, ok = stringLiteralCompletionContext("value: no-string", protocol.Position{Character: 8})
		tAssert.False(ok)
	})

	It("targets directive and semantic action low-coverage branches", func() {
		workspace, err := os.MkdirTemp("", "mace-analyzer-lowest-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()
		documentPath := filepath.Join(workspace, "doc.mace")
		text := `|===|
schema User: { name: string; };
|===|
[output = data, schema = User]
{
}`
		file, err := parseFile(text)
		tAssert.NoError(err)
		tokens := lexAnalysisTokens(text)
		diagnostic := diagnosticWithCode(protocol.Range{}, protocol.DiagnosticSeverityError, diagnosticTypeRecordDoesNotMatchSchema, `missing required field "name"`)
		actions := semanticCodeActions(text, file, tokens, documentPath, diagnostic, `missing required field "name"`)
		tAssert.NotEmpty(actions)
		tAssert.Nil(semanticCodeActions(text, file, tokens, "", diagnostic, `missing required field "name"`))

		doc := document{text: text, analysis: AnalyzeCompletionContextInRoot(text, documentPath, workspace, protocol.Position{Line: 3, Character: 28})}
		uri := protocol.DocumentUri(fileURI(documentPath))
		items, handled := directiveCompletionItems(doc, uri, "[output = ")
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		items, handled = directiveCompletionItems(doc, uri, "[output = data, ")
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		items, handled = directiveCompletionItems(doc, uri, "[output = schema, schema = ")
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = directiveCompletionItems(doc, uri, "[output = data, schema = U")
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		_, handled = directiveCompletionItems(doc, uri, "not a directive")
		tAssert.False(handled)
	})
})

var _ = Describe("analyzer low-percentage helper follow-up coverage", func() {
	It("covers remaining low-percentage analysis helper branches", func() {
		tAssert.Equal("1.5", simpleExpressionText(ast.FloatLiteral{Lexeme: "1.5"}))
		tAssert.Equal("value", simpleExpressionText(ast.Identifier{Name: "value"}))

		text := `[output = schema, schema_file = "./schema.mace"]
{}`
		rangeValue, ok := invalidDirectiveComboEditRange(text, ast.File{}, lexAnalysisTokens(text), "schema_file directive is invalid when output mode is schema")
		tAssert.True(ok)
		tAssert.NotEqual(protocol.Range{}, rangeValue)

		tokens := lexAnalysisTokens(`from "./shared.mace" import Remote: Local;`)
		_, ok = importAliasToken(tokens, ast.ImportDeclaration{Path: ast.StringLiteral{Lexeme: `"./other.mace"`}}, ast.ImportedIdentifier{Name: "Remote", Alias: "Local"})
		tAssert.False(ok)
		_, ok = importAliasToken(tokens, ast.ImportDeclaration{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}}, ast.ImportedIdentifier{Name: "Missing", Alias: "Local"})
		tAssert.False(ok)

		file := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{
			ast.TypeDeclaration{Name: "Alias", Type: ast.PrimitiveType{Name: "string"}},
			ast.VariableDeclaration{Name: "skip", HasValue: false, Type: ast.ArrayType{Element: ast.PrimitiveType{Name: "int"}}},
			ast.VariableDeclaration{Name: "notArray", HasValue: true, Type: ast.PrimitiveType{Name: "int"}, Value: ast.IntLiteral{Lexeme: "1"}},
			ast.VariableDeclaration{Name: "values", HasValue: true, Type: ast.ArrayType{Element: ast.PrimitiveType{Name: "int"}}, Value: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"x"`}}}},
		}}}
		diagnostic, ok := mixedArrayLiteralDiagnostic(file, lexAnalysisTokens(`array<int> values = [1, "x"];`), "array literal has mixed element types")
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticTypeMixedArrayLiteral), requireDiagnosticCode(diagnostic))
		_, ok = mixedArrayLiteralDiagnostic(file, lexAnalysisTokens(`array<int> other = [1, "x"];`), "array literal has mixed element types")
		tAssert.False(ok)
	})
})

var _ = Describe("analyzer semantic action branch coverage", func() {
	It("routes additional diagnostics through semantic code actions", func() {
		documentPath := filepath.Join(os.TempDir(), "semantic-actions.mace")
		cases := []struct {
			text    string
			message string
			title   string
		}{
			{`[output = data]
{
  name: "Ada";
  name: "Bea";
}`, `duplicate output field "name"`, "Remove duplicate field"},
			{`|===|
string value: "x";
|===|
[output = data]
{}`, "expected '='", "Fix declaration operator"},
			{`[output = schema, schema = User]
{}`, "schema directive is invalid when output mode is schema", "Fix invalid output directive combination"},
			{`[output = data]
{
  result: $self.later;
  later: 1;
}`, "unknown self reference forward", "Move referenced field before $self use"},
		}

		for _, test := range cases {
			file, _ := parseFile(test.text)
			tokens := lexAnalysisTokens(test.text)
			actions := semanticCodeActions(test.text, file, tokens, documentPath, protocol.Diagnostic{Range: fullDocumentRange(test.text)}, test.message)
			_, ok := lo.Find(actions, func(action analysisCodeActionCandidate) bool { return action.Action.Title == test.title })
			tAssert.True(ok, test.title)
		}
	})
})

var _ = Describe("analyzer parse input completion follow-up coverage", func() {
	It("covers parse-file declaration and member completion helpers", func() {
		workspace, err := os.MkdirTemp("", "mace-parse-input-completion-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()
		writeAnalysisFile(workspace, "runtime.mace", `[output = schema]
{
  Runtime: { env: string; profile?: { name: string; }; };
  Other: string;
}`)
		writeAnalysisFile(workspace, "data.mace", `[output = data]
{
  profile: { name: "Ada"; };
}`)

		file := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./runtime.mace"`}}}}
		cache := map[string]completionModel{}
		model := completionModel{aliases: map[string]ast.TypeReference{}, schemas: map[string]ast.RecordType{}, variables: map[string]ast.TypeReference{}}
		defs := parseInputDeclarationDefinitions(file, workspace, workspace)
		tAssert.NotEmpty(defs)
		record, ok := parseFileOutputSchemaRecord(file.Output.Directives, workspace, workspace, cache)
		tAssert.True(ok)
		tAssert.NotEmpty(record.Fields)
		exported, ok := parseFileOutputExportedRecord(file.Output.Directives, workspace, workspace, cache)
		tAssert.True(ok)
		tAssert.Len(exported.Fields, 2)
		rootType, typeOK, guarded := parseInputMemberCompletionRootType(file, model, []string{"Runtime", "profile"}, workspace, workspace, cache, map[string]struct{}{})
		tAssert.False(typeOK)
		tAssert.True(guarded)
		tAssert.Nil(rootType)
		rootType, typeOK, guarded = parseInputMemberCompletionRootType(file, model, []string{"Runtime", "profile"}, workspace, workspace, cache, map[string]struct{}{"profile": {}})
		tAssert.True(typeOK)
		tAssert.False(guarded)
		tAssert.NotNil(rootType)

		file.Output.Directives = []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./missing.mace"`}}
		tAssert.Empty(parseInputDeclarationDefinitions(file, workspace, workspace))
		_, ok = parseFileOutputSchemaRecord(file.Output.Directives, workspace, workspace, map[string]completionModel{})
		tAssert.False(ok)
		_, ok = parseFileOutputExportedRecord(file.Output.Directives, workspace, workspace, map[string]completionModel{})
		tAssert.False(ok)

		importFile := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./data.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Data"}}}}
		rootType, importedModel, ok := importedMemberCompletionRootType(importFile, []string{"Data"}, workspace, workspace, map[string]completionModel{})
		tAssert.True(ok)
		tAssert.NotNil(rootType)
		tAssert.NotNil(importedModel.schemas)
		rootType, _, ok = importedMemberCompletionRootType(importFile, []string{"Data", "profile"}, workspace, workspace, map[string]completionModel{})
		tAssert.True(ok)
		tAssert.NotNil(rootType)
		_, _, ok = importedMemberCompletionRootType(importFile, nil, workspace, workspace, map[string]completionModel{})
		tAssert.False(ok)
		_, _, ok = importedMemberCompletionRootType(importFile, []string{"Missing"}, workspace, workspace, map[string]completionModel{})
		tAssert.False(ok)
	})
})
