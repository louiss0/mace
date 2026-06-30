package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
		tAssert.NoError(err)
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

var _ = Describe("analyzer branch boost helpers", func() {
	It("covers additional utility branches directly", func() {
		workspace := GinkgoT().TempDir()
		tAssert.Equal("x", stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"x"`}))
		tAssert.Equal("value", expressionSummary(ast.Identifier{Name: "value"}))
		tAssert.Equal("{  }", summarizeValue(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{}}))
		tAssert.NotEmpty(indexSymbols([]semanticSymbol{{Name: "value"}}))
		_, ok := outputDirectiveListRange("[output = data]")
		tAssert.True(ok)
		_, _, ok = schemaFileDirectiveRanges("[output = data]")
		tAssert.False(ok)
		_, ok = importAndScriptCleanupRange("from \"./shared.mace\" import User;\n|===|\n|===|\n[output = data]\n{}")
		tAssert.True(ok)
		tAssert.Equal("Name", quotedName(`unknown schema "Name"`))
		docPath := writeAnalysisFile(workspace, "doc.mace", "[output = data]\n{\n  value: 1;\n}\n")
		writeAnalysisFile(workspace, "shared.mace", "[output = data]\n{\n  User: 1;\n}\n")
		updated, ok := addMissingScriptSemicolonText("|===|\nfoo\n|===|")
		tAssert.False(ok)
		tAssert.Empty(updated)
		updated, ok = addMissingScriptSemicolonText("|===|\nschema User: { name: string; };\n|===|")
		tAssert.False(ok)
		updated, ok = moveScriptBlockBeforeOutputText("prefix\n|===|\nstring value = \"x\";\n|===|")
		tAssert.False(ok)
		updated, ok = extractRecordLiteralIntoSchemaText("|===|\nProfile record = {};\n|===|")
		tAssert.False(ok)
		updated, ok = createSchemaFromValidationErrorText("[output = data, schema = User]\n{}")
		tAssert.False(ok)
		tAssert.IsType(ast.BooleanLiteral{}, defaultExpressionForType(ast.PrimitiveType{Name: "boolean"}))
		tAssert.IsType(ast.ArrayLiteral{}, defaultExpressionForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}))
		tAssert.IsType(ast.PrimitiveType{}, inferredTypeFromExpression(ast.BooleanLiteral{Value: true}))
		tAssert.IsType(ast.PrimitiveType{}, inferredTypeFromExpression(ast.StringLiteral{Lexeme: `"x"`}))
		tAssert.Equal([]string{"count: int", "items: array<string>"}, inferRecordSchemaFields("count: 1; items: [\"x\"];"))
		tAssert.False(lo.Contains(inferRecordSchemaFields("bad {}"), "anything"))

		invalidRecord, ok := replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, nil, ast.NamedType{Name: "Alias"})
		tAssert.False(ok)
		tAssert.Equal(1, len(invalidRecord.Fields))
		missingRecord, ok := replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, []string{"missing"}, ast.NamedType{Name: "Alias"})
		tAssert.False(ok)
		tAssert.Equal(1, len(missingRecord.Fields))

		formatted, ok := formatTextQuick("[output = data]\n{}")
		tAssert.True(ok)
		tAssert.Contains(formatted, "output")
		_, ok = formatTextQuick("\x00")
		tAssert.False(ok)

		rangeValue, ok := outputBodyRange("[output = data]\n{}", lexAnalysisTokens("[output = data]\n{}"))
		tAssert.True(ok)
		tAssert.NotEqual(protocol.Range{}, rangeValue)
		_, ok = outputBodyRange("[output = data]", lexAnalysisTokens("[output = data]"))
		tAssert.False(ok)

		_, _, ok = missingSchemaFieldEdit("[output = data]", ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData}}, `missing required field "name"`)
		tAssert.False(ok)
		_, ok = namedFieldEditRangeFromEnd("[output = data]\n{\n}\n", lexAnalysisTokens("[output = data]\n{\n}\n"), "missing")
		tAssert.False(ok)
		_, _, ok = declarationOperatorEdit("", lexAnalysisTokens("name value"), "expected '='")
		tAssert.False(ok)
		_, _, ok = missingImportEdit("", ast.File{}, nil, "other")
		tAssert.False(ok)
		_, ok = duplicateDeclarationEditRange("", ast.File{}, lexAnalysisTokens("string value = \"x\";"), `duplicate declaration "missing"`)
		tAssert.False(ok)

		aliasToken, ok := importAliasToken(lexAnalysisTokens(`from "./a.mace" import One: Local;`), ast.ImportDeclaration{Path: ast.StringLiteral{Lexeme: `"./a.mace"`}}, ast.ImportedIdentifier{Name: "One", Alias: "Different"})
		tAssert.False(ok)
		tAssert.Equal("", aliasToken.Lexeme)
		_, ok = importDeclarationEditRange(`value`, lexAnalysisTokens(`value`), 0)
		tAssert.False(ok)

		references := referencedNames(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{
			ast.TypeDeclaration{Name: "Alias", Type: ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.ArrayType{Element: ast.NamedType{Name: "Tag"}}}}},
			ast.SchemaDeclaration{Name: "Doc", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "field", Type: ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "First"}, ast.NamedType{Name: "Second"}}}}}}},
		}}, Output: ast.OutputBlock{SchemaFields: []ast.OutputSchemaField{{Name: "root", Type: ast.NamedType{Name: "Root"}}}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "OutputSchema"}}}})
		for _, name := range []string{"User", "Tag", "First", "Second", "Root", "OutputSchema"} {
			_, found := references[name]
			tAssert.True(found)
		}

		_, ok = nullUsageDiagnostic(lexAnalysisTokens("value"), processor.DiagnosticError{Code: processor.CodeInvalidNullUsage, Message: "null bad"})
		tAssert.False(ok)
		_, ok = unknownSchemaDiagnostic(lexAnalysisTokens("schema Missing"), `unknown schema "Other"`)
		tAssert.False(ok)
		_, ok = dataOutputValueDiagnostic(lexAnalysisTokens("[output = data]\n{\n  field: 1;\n}\n"), processor.DiagnosticError{Code: processor.CodeOutputValueDeclaration, Fields: processor.DiagnosticFields{Name: "missing"}}, "msg")
		tAssert.False(ok)

		_, _, _ = nestedOutputFieldPathAt("[output = data]\n{\n  outer?: { inner: 1; };\n}\n", lexAnalysisTokens("[output = data]\n{\n  outer?: { inner: 1; };\n}\n"), protocol.Position{Line: 2, Character: 12})
		_, _ = analyzeDocumentAtInRoot("[output = data]\n{\n}\n", docPath, workspace).definitionAt(protocol.Position{Line: 1, Character: 2})
		_, _ = directivePathDiagnostics(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./ok.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `bad`}}}}, lexAnalysisTokens(`[output = data, schema_file = "./ok.mace"]`), docPath)
		_, _ = parseDirectiveWarningDiagnostic("[output = data]", ast.File{})
		_, _ = semanticDiagnosticFromError(ast.File{}, nil, fmt.Errorf("plain"))
		_, _ = importResolutionCodeActions(`from "./shared.mace" import User;`, ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}}, lexAnalysisTokens(`from "./shared.mace" import User;`), docPath), unavailableImportDiagnostics(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}}, lexAnalysisTokens(`from "./shared.mace" import User;`), docPath)
		_ = unavailableImportNameSet(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}}, docPath)
		_, _ = documentationCodeActions("|===|\ntype Alias: string;\n|===|\n[output = data]\n{}", ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.TypeDeclaration{Name: "Alias", NameToken: lexer.Token{Line: 2, Column: 6, Lexeme: "Alias"}, Type: ast.PrimitiveType{Name: "string"}}}}}, lexAnalysisTokens("|===|\ntype Alias: string;\n|===|\n[output = data]\n{}"), docPath), editorRefactorCodeActions("[output = data]\n{\n  value: 1;\n}\n", ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "value", Value: ast.IntLiteral{Lexeme: "1"}}}}}, lexAnalysisTokens("[output = data]\n{\n  value: 1;\n}\n"), docPath)
		updated, ok = replaceVariableDeclaration("nothing", regexp.MustCompile(`missing`), func(matches []string) string { return "x" })
		tAssert.False(ok)
		tAssert.Equal("nothing", updated)
		updated, ok = renameDuplicateVariableText("|===|\nstring first = \"x\";\nstring second = \"y\";\n|===|")
		tAssert.False(ok)
		tAssert.Equal("|===|\nstring first = \"x\";\nstring second = \"y\";\n|===|", updated)
		tAssert.Equal("true", simpleExpressionText(ast.BooleanLiteral{Value: true}))
		tAssert.IsType(ast.StringLiteral{}, defaultExpressionForType(nil))
		tAssert.IsType(ast.PrimitiveType{}, inferredTypeFromExpression(ast.StringLiteral{Lexeme: `"x"`}))
		tAssert.Equal("[1, 2]", summarizeValue(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueInt, Int: 1}, {Kind: processor.ValueInt, Int: 2}}}))
		tAssert.Equal("0x1", summarizeValue(processor.Value{Kind: processor.ValueHexInt, Int: 1}))
		_, _, _, _ = parsedFile(docPath)
		tAssert.Equal("string", resolvedImportedTypeDetail(ast.NamedType{Name: "Alias"}, ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.TypeDeclaration{Name: "Alias", Type: ast.PrimitiveType{Name: "string"}}}}}))
	})
})

var _ = Describe("analyzer remaining coverage helpers", func() {
	It("covers direct branch helpers and snapshot fallbacks", func() {
		workspace := GinkgoT().TempDir()
		documentPath := writeAnalysisFile(workspace, "doc.mace", `|===|
string name = "Ada";
schema User: { profile: { age: int; }; tags: string; };
Profile record = { age: 1; };
|===|
[output = data]
{
  name: "Ada";
  nested: {
    child: 1;
  };
  later: $self.name;
}`)
		textBytes, err := os.ReadFile(documentPath)
		tAssert.NoError(err)
		text := string(textBytes)
		tokens := lexAnalysisTokens(text)
		result := processor.Result{Output: map[string]processor.Value{
			"name": {Kind: processor.ValueString, String: "Ada"},
			"nested": {Kind: processor.ValueRecord, Record: map[string]processor.Value{"child": {Kind: processor.ValueInt, Int: 1}}},
			"later": {Kind: processor.ValueString, String: "Ada"},
		}}
		definition := protocol.Location{URI: pathURI(documentPath), Range: protocol.Range{Start: protocol.Position{Line: 1, Character: 7}, End: protocol.Position{Line: 1, Character: 11}}}
		nameRange, _ := tokenRange(tokens, "name")
		childRange, _ := tokenRangeFromEnd(tokens, "child")
		snapshot := analysisSnapshot{
			text:        text,
			documentURI: pathURI(documentPath),
			file:        &ast.File{},
			result:      &result,
			tokens:      tokens,
			symbols: []semanticSymbol{{Name: "name", Range: nameRange, Definition: definition}, {Name: "name", Definition: definition}},
			symbolIndex: map[string]semanticSymbol{"name": {Name: "name", Definition: definition}},
		}
		_, ok := snapshot.definitionAt(nameRange.Start)
		tAssert.True(ok)
		_, ok = (analysisSnapshot{}).definitionAt(protocol.Position{})
		tAssert.False(ok)
		selfNameRange, _ := tokenRangeFromEnd(tokens, "name")
		_, ok = snapshot.selfReferenceSymbolAt(selfNameRange.Start)
		tAssert.True(ok)
		_, ok = snapshot.nestedOutputFieldSymbolAt(childRange.Start)
		tAssert.True(ok)
		_, ok = outputValueAtPath(result.Output, []string{"nested", "child"})
		tAssert.True(ok)
		_, ok = outputValueAtPath(result.Output, []string{"nested", "missing"})
		tAssert.False(ok)
		_, ok = outputValueAtPath(result.Output, []string{"name", "child"})
		tAssert.False(ok)

		actions := []analysisCodeActionCandidate{{Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{Line: 20}}, Action: protocol.CodeAction{Title: "edit", Edit: &protocol.WorkspaceEdit{}}}, {Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{Line: 20}}, Action: protocol.CodeAction{Title: "command"}}}
		resolved := (analysisSnapshot{codeActionCandidates: actions}).codeActions(pathURI(documentPath), protocol.Range{Start: protocol.Position{}, End: protocol.Position{Line: 1}})
		tAssert.Len(resolved, 2)
		tAssert.NotNil(resolved[0].Edit)
		tAssert.NotNil(resolved[0].Edit.Changes)
		tAssert.NotEmpty(resolved[0].Diagnostics)
		tAssert.Empty((analysisSnapshot{codeActionCandidates: actions}).codeActions(pathURI(documentPath), protocol.Range{Start: protocol.Position{Line: 50}, End: protocol.Position{Line: 51}}))

		path, _, ok := nestedOutputFieldPathAt(text, tokens, childRange.Start)
		tAssert.True(ok)
		tAssert.Equal([]string{"nested", "child"}, path)
		path, _, ok = selfReferencePathAt(tokens, selfNameRange.Start)
		tAssert.True(ok)
		tAssert.Equal([]string{"name"}, path)

		tAssert.Equal("value", simpleExpressionText(ast.Identifier{Name: "value"}))
		tAssert.IsType(ast.StringLiteral{}, defaultExpressionForType(ast.NamedType{Name: "User"}))
		tAssert.IsType(ast.PrimitiveType{}, inferredTypeFromExpression(ast.Identifier{Name: "value"}))

		updated, ok := addMissingScriptSemicolonText("|===|\nstring value = \"x\"\n|===|\n[output = data]{}")
		tAssert.True(ok)
		tAssert.Contains(updated, `string value = "x";`)
		updated, ok = moveScriptBlockBeforeOutputText("[output = data]\n{}\n|===|\nstring value = \"x\";\n|===|")
		tAssert.True(ok)
		tAssert.True(strings.HasPrefix(updated, "|===|"))
		updated, ok = extractRecordLiteralIntoSchemaText("|===|\nProfile record = { age: 1; active: true; };\n|===|\n[output = data]{}")
		tAssert.True(ok)
		tAssert.Contains(updated, "schema Profile")
		updated, ok = createSchemaFromValidationErrorText("[output = data, schema = User]\n{\n  age: 1;\n  active: true;\n  tags: [\"x\"];\n}")
		tAssert.True(ok)
		tAssert.Contains(updated, "schema User")
		tAssert.Equal([]string{"age: int", "active: boolean", "tags: array<string>"}, inferOutputSchemaFields("age: 1; active: true; tags: [\"x\"]; schema: ignored;"))
		tAssert.Equal([]string{"name: string", "score: float"}, inferRecordSchemaFields("name: \"Ada\"; score: 1.5;"))
		updated, ok = replaceVariableDeclaration("|===|\nstring old = \"x\";\n|===|", regexp.MustCompile(`(?m)^([ \t]*)(string)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*");`), func(matches []string) string {
			return matches[1] + matches[2] + " fresh = " + matches[4] + ";"
		})
		tAssert.True(ok)
		tAssert.Contains(updated, "fresh")
		updated, ok = renameDuplicateVariableText("|===|\nstring value = \"x\";\nstring value = \"y\";\n|===|")
		tAssert.True(ok)
		tAssert.Contains(updated, "value_2")

		record, ok := replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}, []string{"profile", "name"}, ast.NamedType{Name: "Alias"})
		tAssert.True(ok)
		tAssert.IsType(ast.NamedType{}, record.Fields[0].Type.(ast.RecordType).Fields[0].Type)
		_, ok = replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.PrimitiveType{Name: "string"}}}}, []string{"profile", "name"}, ast.NamedType{Name: "Alias"})
		tAssert.False(ok)
		tAssert.Equal("ExampleType3", nextExampleTypeAliasName(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.TypeDeclaration{Name: "ExampleType"}, ast.TypeDeclaration{Name: "ExampleType2"}}}}))

		stringActions := []analysisCodeActionCandidate{}
		addStringRefactorActions("value: \"x\";\nschema_file: \"skip\";\nfrom \"./skip.mace\" import Name;", pathURI(documentPath), fullDocumentRange(text), &stringActions)
		tAssert.Len(stringActions, 2)

		rangeValue, textValue, ok := missingSchemaFieldEdit("[output = data]\n{\n}\n", ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData}}, `missing required field "name"`)
		tAssert.True(ok)
		tAssert.NotEqual(protocol.Range{}, rangeValue)
		tAssert.Equal("  name: TODO;\n", textValue)
		rangeValue, ok = invalidDirectiveComboEditRange("[output = schema, schema = User]", ast.File{}, lexAnalysisTokens("[output = schema, schema = User]"), "schema directive is invalid when output mode is schema")
		tAssert.True(ok)
		tAssert.NotEqual(protocol.Range{}, rangeValue)
		rangeValue, textValue, ok = generateOutputFromSchemaEdit("|===|\nschema User: { name: string; age: int; };\n|===|\n[output = data, schema = User]\n{}", ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Type: ast.PrimitiveType{Name: "int"}}}}}}}, Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}}, lexAnalysisTokens("|===|\nschema User: { name: string; age: int; };\n|===|\n[output = data, schema = User]\n{}"), "missing required field")
		tAssert.True(ok)
		tAssert.NotEqual(protocol.Range{}, rangeValue)
		tAssert.Contains(textValue, "name")

		rangeValue, ok = namedFieldEditRangeFromEnd("[output = data]\n{\n  field: 1;\n  field: 2;\n}", lexAnalysisTokens("[output = data]\n{\n  field: 1;\n  field: 2;\n}"), "field")
		tAssert.True(ok)
		importText := `from "./a.mace" import One, Two;`
		importTokens := lexAnalysisTokens(importText)
		nameToken, found := importIdentifierToken(importTokens, ast.ImportDeclaration{Path: ast.StringLiteral{Lexeme: `"./a.mace"`}}, "One")
		tAssert.True(found)
		rangeValue, ok = importIdentifierEditRange(importText, importTokens, nameToken, false)
		tAssert.True(ok)
		rangeValue, ok = importDeclarationEditRange(`from "./a.mace" import One;`, lexAnalysisTokens(`from "./a.mace" import One;`), 3)
		tAssert.True(ok)
		rangeValue, ok = declarationEditRange("|===|\nstring value = \"x\";\n|===|", lexAnalysisTokens("|===|\nstring value = \"x\";\n|===|"), lexer.Token{Line: 2, Column: 8, Lexeme: "value"})
		tAssert.True(ok)
		_, ok = importAliasToken(lexAnalysisTokens(`from "./a.mace" import One: Local;`), ast.ImportDeclaration{Path: ast.StringLiteral{Lexeme: `"./a.mace"`}}, ast.ImportedIdentifier{Name: "One", Alias: "Local"})
		tAssert.True(ok)
	})

	It("covers direct helper branch combinations", func() {
		workspace := GinkgoT().TempDir()
		documentPath := filepath.Join(workspace, "helpers.mace")
		conflictText := "from \"./shared.mace\" import User;\n|===|\nstring value = \"x\";\n|===|\n[output = data, schema_file = \"./schema.mace\"]\n{}"
		conflictFile := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}, Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value"}}}, Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema.mace"`}}}}
		diagnosticConflict, conflictActions, ok := schemaFileConflictAnalysis(conflictText, conflictFile, documentPath)
		tAssert.True(ok)
		tAssert.NotEmpty(conflictActions)
		tAssert.Equal(string(diagnosticDirectiveSchemaAndSchemaFileCombined), requireDiagnosticCode(diagnosticConflict))
		diagnosticConflict, conflictActions, ok = schemaFileConflictAnalysis(conflictText, conflictFile, "")
		tAssert.True(ok)
		tAssert.Nil(conflictActions)
		_, _, ok = schemaFileConflictAnalysis("[output = data]{}", ast.File{Output: ast.OutputBlock{}}, documentPath)
		tAssert.False(ok)
		warning, ok := parseDirectiveWarningDiagnostic("[output = data, parse_file = \"./input.mace\"]", ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./input.mace"`}}}})
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticDirectiveParseValuesUnknown), requireDiagnosticCode(warning))
		_, ok = semanticDiagnosticFromError(ast.File{}, nil, processor.DiagnosticError{Code: processor.CodeInvalidNullUsage, Message: "null bad"})
		tAssert.False(ok)
		helperDocumentPath := filepath.Join(GinkgoT().TempDir(), "helpers.mace")
		text := "|===|\nint count = \"x\";\nint count = 1;\n|===|\n[output = data]\n{\n  missing: 1;\n  dup: 1;\n  dup: 2;\n}\n"
		tokens := lexAnalysisTokens(text)
		file := ast.File{
			Script: &ast.ScriptBlock{Items: []ast.Declaration{
				ast.VariableDeclaration{Name: "count", HasValue: true, Type: ast.PrimitiveType{Name: "int"}, Value: ast.StringLiteral{Lexeme: `"x"`}},
				ast.VariableDeclaration{Name: "list", HasValue: true, Type: ast.ArrayType{Element: ast.PrimitiveType{Name: "int"}}, Value: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"x"`}}}},
			}},
			Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "missing", Value: ast.Identifier{Name: "missing"}}, {Name: "dup", Value: ast.IntLiteral{Lexeme: "1"}}}},
		}
		diagnostic := protocol.Diagnostic{Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}}
		actions := semanticCodeActions(text, file, tokens, helperDocumentPath, diagnostic, `unknown field "missing"`)
		tAssert.NotEmpty(actions)
		actions = semanticCodeActions(text, file, tokens, helperDocumentPath, diagnostic, `duplicate field "dup"`)
		tAssert.NotEmpty(actions)
		actions = semanticCodeActions(text, file, tokens, helperDocumentPath, diagnostic, `processor: type mismatch: expected int, got string`)
		tAssert.NotEmpty(actions)
		actions = semanticCodeActions("[output = schema, schema = User]", ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, lexAnalysisTokens("[output = schema, schema = User]"), helperDocumentPath, diagnostic, "schema directive is invalid when output mode is schema")
		tAssert.NotEmpty(actions)
		actions = semanticCodeActions(text, ast.File{}, tokens, helperDocumentPath, diagnostic, `duplicate declaration "count"`)
		tAssert.NotEmpty(actions)
		actions = semanticCodeActions("a: b", ast.File{}, lexAnalysisTokens("a: b"), helperDocumentPath, diagnostic, "expected '='")
		tAssert.NotEmpty(actions)
		actions = semanticCodeActions("[output = data]\n{\n  b: $self.c;\n  c: 1;\n}", ast.File{}, lexAnalysisTokens("[output = data]\n{\n  b: $self.c;\n  c: 1;\n}"), helperDocumentPath, diagnostic, "forward unknown self reference")
		tAssert.NotEmpty(actions)
		_, _ = variableTypeMismatchDiagnostic(file, tokens, processor.DiagnosticError{Code: processor.CodeTypeMismatch, Message: "bad", Fields: processor.DiagnosticFields{Expected: "int", Actual: "string"}})
		_, _ = mixedArrayLiteralDiagnostic(file, tokens, "array literal has mixed element types")
		_, _ = schemaDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeMissingRequiredField, Fields: processor.DiagnosticFields{Schema: "count"}}, "msg")
		_, _ = selfReferenceDiagnostic(ast.File{Output: ast.OutputBlock{DataFields: []ast.OutputField{{Name: "missing"}}}}, lexAnalysisTokens("[output = data]\n{\n  a: $self.missing;\n}\n"), processor.DiagnosticError{Code: processor.CodeSelfReferenceUnknown, Fields: processor.DiagnosticFields{Name: "missing"}}, "msg")
	})

	It("covers symbol lookup edge cases", func() {
		location := protocol.Location{URI: "file:///doc.mace", Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{Character: 1}}}
		snapshot := analysisSnapshot{
			text:        "alpha beta",
			documentURI: "file:///doc.mace",
			file:        &ast.File{},
			symbols: []semanticSymbol{{Name: "alpha", Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{Character: 5}}, Definition: location}, {Name: "beta", Definition: location}},
			symbolIndex: map[string]semanticSymbol{"beta": {Name: "beta", Definition: location}, "gamma": {Name: "gamma"}},
		}
		resolved, ok := snapshot.definitionAt(protocol.Position{Character: 2})
		tAssert.True(ok)
		tAssert.Equal(location, resolved)
		resolved, ok = snapshot.definitionAt(protocol.Position{Character: 6})
		tAssert.True(ok)
		tAssert.Equal(location, resolved)
		_, _ = snapshot.definitionAt(protocol.Position{Character: 20})
		_, _ = (analysisSnapshot{text: "zeta", file: &ast.File{}, symbolIndex: map[string]semanticSymbol{"zeta": {Name: "zeta"}}}).definitionAt(protocol.Position{Character: 1})

		_, ok = (analysisSnapshot{}).selfReferenceSymbolAt(protocol.Position{})
		tAssert.False(ok)
		selfTokens := lexAnalysisTokens("[output = data]\n{\n  later: $self.missing;\n}\n")
		_, ok = (analysisSnapshot{result: &processor.Result{}, tokens: selfTokens}).selfReferenceSymbolAt(protocol.Position{Line: 2, Character: 17})
		tAssert.False(ok)
		_, ok = (analysisSnapshot{result: &processor.Result{Output: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "x"}}}, tokens: selfTokens}).selfReferenceSymbolAt(protocol.Position{Line: 2, Character: 17})
		tAssert.False(ok)
		nestedText := "[output = data]\n{\n  nested: { child: 1; };\n}\n"
		nestedTokens := lexAnalysisTokens(nestedText)
		_, ok = (analysisSnapshot{result: &processor.Result{Output: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "x"}}}, text: nestedText, tokens: nestedTokens}).nestedOutputFieldSymbolAt(protocol.Position{Line: 2, Character: 12})
		tAssert.False(ok)
		_, _, ok = nestedOutputFieldPathAt("value", lexAnalysisTokens("value"), protocol.Position{})
		tAssert.False(ok)
		_, ok = outputValueAtPath(map[string]processor.Value{}, nil)
		tAssert.False(ok)
	})

	It("covers analysis integration branches", func() {
		workspace := GinkgoT().TempDir()
		writeAnalysisFile(workspace, "exports.mace", "[output = schema]\n{\n  User: string;\n}\n")
		writeAnalysisFile(workspace, "renamed.mace", "[output = schema]\n{}\n")
		documentPath := filepath.Join(workspace, "main.mace")
		text := `from "./exports" import Uzer;
|===|
string value = "x";
string value = "y";
|===|
[output = schema, schema_file = "./schema"]
{
  value: string;
}`
		tokens := lexAnalysisTokens(text)
		file := ast.File{
			Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./exports"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Uzer"}}}},
			Script:  &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{NameToken: lexer.Token{Line: 3, Column: 8, Lexeme: "value"}, Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"x"`}}, ast.VariableDeclaration{NameToken: lexer.Token{Line: 4, Column: 8, Lexeme: "value"}, Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"y"`}}}},
			Output:  ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema"`}}, SchemaFields: []ast.OutputSchemaField{{Name: "value", Type: ast.PrimitiveType{Name: "string"}}}},
		}
		diagnostics, actions := analyzeFileStructure(text, file, tokens, documentPath)
		tAssert.NotEmpty(diagnostics)
		tAssert.NotEmpty(actions)
		directiveDiagnostics, directiveActions := directivePathDiagnostics(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"./parse"`}}}}, lexAnalysisTokens(`[output = data, schema_file = "./schema", parse_file = "./parse"]`), documentPath)
		tAssert.Len(directiveDiagnostics, 2)
		tAssert.Len(directiveActions, 2)
		snapshot := analyzeDocumentAtInRoot("*", documentPath, workspace)
		tAssert.NotNil(snapshot.diagnostics)
		tAssert.NotEmpty(snapshot.codeActionCandidates)
		snapshot = analyzeDocumentAtInRoot("\x00", documentPath, workspace)
		tAssert.NotNil(snapshot.diagnostics)
		snapshot = analyzeDocumentAtInRoot("[output = data]\n{\n  name: $self.missing;\n  missing: \"x\";\n}\n", documentPath, workspace)
		tAssert.NotNil(snapshot.file)
		tAssert.NotEmpty(snapshot.diagnostics)
		snapshot = analyzeDocumentAtInRoot("[output = data]\n{\n  name: \"Ada\";\n}\n", documentPath, workspace)
		tAssert.NotNil(snapshot.result)
		tAssert.Empty(unavailableImportNameSet(ast.File{}, ""))
		parseWarning, ok := parseDirectiveWarningDiagnostic("[output = data, parse = User]\n{\n  name: \"Ada\";\n}\n", ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}})
		tAssert.True(ok)
		tAssert.Equal(string(diagnosticDirectiveParseValuesUnknown), requireDiagnosticCode(parseWarning))
	})
})
