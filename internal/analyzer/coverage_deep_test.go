package analyzer

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("analyzer deep helper coverage", func() {
	It("covers package utility branches", func() {
		workspace := GinkgoT().TempDir()
		validPath := filepath.Join(workspace, "valid.mace")
		tAssert.NoError(os.WriteFile(validPath, []byte("[output = data]\n{}"), 0o644))

		_, _ = documentPathFromURI(protocol.DocumentUri("http://example.com/doc.mace"))
		_, _ = documentPathFromURI(protocol.DocumentUri("file:///%zz"))
		pathValue, ok := documentPathFromURI(protocol.DocumentUri(fileURI(validPath)))
		tAssert.True(ok)
		tAssert.Equal(validPath, pathValue)

		absPath, err := filepath.Abs("abs.mace")
		tAssert.NoError(err)
		_, _ = resolveBoundedPathInRoot(workspace, workspace, absPath)
		resolvedPath, err := resolveBoundedPathInRoot(workspace, workspace, "./nested/file.mace")
		tAssert.NoError(err)
		tAssert.Equal(filepath.Join(workspace, "nested", "file.mace"), resolvedPath)
		resolvedPath, err = resolveRootBoundedPathInRoot(workspace, workspace, "./nested/file.mace")
		tAssert.NoError(err)
		tAssert.Equal(filepath.Join(workspace, "nested", "file.mace"), resolvedPath)
		_, _ = resolveRootBoundedPathInRoot(workspace, workspace, "../outside.mace")
		_ = formatImportRoot("")
		_ = formatImportRoot(".")
		_ = formatImportRoot(filepath.Join(workspace, "root"))
		_ = parseUint("42")
		_ = parseUint("nope")
		_ = fileScriptItems(ast.File{})
		_ = fileScriptItems(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.TypeDeclaration{Name: "Demo"}}}})
		_ = typeReferenceDetail(ast.PrimitiveType{Name: "int"})
		_ = typeReferenceDetail(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}})
		_ = typeReferenceDetail(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"x"`}}})
		_ = typeReferenceDetail(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		_ = fieldTypeDetail(ast.NamedType{Name: "Alias"})
		_ = recordTypeDetail(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}})
		_ = Ptr(1)
	})

	It("covers completion prefix and context helpers", func() {
		text := `|===|
from "./shared.mace" import User;
string value = "x";
|===|
[output = data, schema = User]
{
  profile: user.profile,
  digits: numbers[12],
  guarded: ("name" in user && 'age' in profile ? 1 : 0),
  literal: "a \"quote\" b",
}
`
		position := protocol.Position{Line: 6, Character: 21}
		_ = currentLinePrefix(text, position)
		_ = currentLineSuffix(text, position)
		_ = lastUnquotedByteInPrefix(`foo ? "?" ?`, '?')
		_ = lastUnquotedByteInPrefix(`"?"`, '?')
		_ = outputGuardedNames(`"name" in user && 'age' in profile ?`)
		_ = outputGuardedNames("no guard here")
		_, _ = outputMemberAccessContext("user.profile.")
		_, _ = outputMemberAccessContext("$self.profile.")
		_, _, _ = selfCompletionContext("$self.user.pro")
		_, _, _ = selfCompletionContext("plain text")
		_, _, _ = arrayIndexCompletionContext("numbers[12")
		_, _, _ = arrayIndexCompletionContext("numbers[x")
		_, _ = directivePrefix("[output = data, schema = User]")
		_, _ = directivePrefix("not a directive")
		_ = trailingIdentifierPrefix("abc")
		_ = trailingIdentifierPrefix("123!")
		_, _ = stringLiteralCompletionContext(text, protocol.Position{Line: 6, Character: 15})
		_, _ = completionPlaceholderPosition(text, protocol.Position{Line: 6, Character: 15}, ":.")
		_, _ = completionPlaceholderPosition(text, protocol.Position{Line: 1, Character: 0}, "=")
	})

	It("covers completion file and path helpers", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		sharedPath := filepath.Join(root, "shared.mace")
		tAssert.NoError(os.WriteFile(sharedPath, []byte(`
[output = schema]
{
  User: { name: string; home: { city: string; }; };
  Runtime: { env: string; };
}
`), 0o644))

		documentPath := filepath.Join(root, "document.mace")
		text := `|===|
from "./shared.mace" import User, Runtime;
|===|
[output = data, schema = User]
{
  user: User,
}`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		snapshot := AnalyzeDocumentAt(text, documentPath)
		doc := document{text: text, analysis: snapshot}
		uri := protocol.DocumentUri(fileURI(documentPath))

		_ = completionFile(doc, "")
		_ = currentImports(doc, "")
		_ = completionRoot(snapshot, uri)
		_ = relativePathItems(doc, uri, "./", nil, true)
		_ = schemaFileItems(doc, uri, "", "./")
		_ = schemaReferenceItems(doc, uri, "", "Us")
		_ = availableSchemaNames(doc, uri, "")
		_ = importedPaths(doc, "")
		_, _ = documentPathFromURI(uri)
		_, _ = completionFileWithPlaceholder(text, protocol.Position{Line: 4, Character: 2})
		_, _ = completionFileWithExpressionPlaceholder(text, 0, 3)
		_, _ = partialScriptFileWithPlaceholder(text, protocol.Position{Line: 1, Character: 0})
		placeholderModel := completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}
		_, _, _ = placeholderCompletionType(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", HasValue: true, Type: ast.PrimitiveType{Name: "string"}, Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}, completionModel{})
		_, _, _ = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}, placeholderModel)
		_, _, _ = placeholderParseInputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Runtime"}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}, completionModel{schemas: map[string]ast.RecordType{"Runtime": {Fields: []ast.SchemaField{{Name: "env", Type: ast.PrimitiveType{Name: "string"}}}}}}, root, root)
		_, _ = placeholderPath(ast.SelfReference{Path: []string{"x"}})
		_, _ = expressionPath(ast.MemberAccess{Target: ast.Identifier{Name: "x"}, Name: "y"})
		_, _ = trailingMemberAccessPath("user.profile.")
		_, _ = completionTypeAtPath(ast.NamedType{Name: "User"}, []string{"profile"}, placeholderModel)
		_ = completionItemsForType(ast.PrimitiveType{Name: "string"}, completionModel{}, completionOptions{})
		_ = completionItemsForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, completionModel{}, completionOptions{unquotedStringChoices: true})
		_ = completionItemsForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, completionModel{}, completionOptions{allowSchemaLiteral: true})
		_ = completionItemsForType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, completionModel{}, completionOptions{})
		_ = completionItemsForValueMembers(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}})
		_ = completionItemsForValueMembers(processor.Value{Kind: processor.ValueString, String: "Ada"})
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "string"}, completionModel{}, 0)
		_ = syntheticCompletionValue(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, completionModel{}, 2)
		_, _ = unquotedStringChoiceLabel(`"Ada"`)
		_, _ = unquotedStringChoiceLabel("Ada")
		_ = buildCompletionModel(ast.File{}, root, root, map[string]completionModel{})
		_, _, _ = importedCompletionModel(sharedPath, root, map[string]completionModel{})
		_ = parseFileOutputDeclarationDefinitions(snapshot.file.Output.Directives, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord(snapshot.file.Output.Directives, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord(snapshot.file.Output.Directives, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(*snapshot.file, []string{"User"}, root, root, map[string]completionModel{})
		_, _ = importAsSchemaRecord(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}})
		_, _ = importAsDataRecord(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "user", Value: ast.StringLiteral{Lexeme: `"x"`}}}}}, completionModel{})
		_ = parseInputDeclarationDefinitions(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./shared.mace"`}}}}, root, root)
		_, _ = parseInputCompletionRecord(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveSchema, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"./shared.mace"`}}}}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, root, root, map[string]completionModel{})
		_, _ = completionChoiceFromMembers([]ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}}, completionModel{}, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.StringLiteral{Lexeme: `"Ada"`}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.NamedType{Name: "Alias"}, completionModel{aliases: map[string]ast.TypeReference{"Alias": ast.PrimitiveType{Name: "string"}}}, map[string]struct{}{})
		_, _ = directoryEntries(root, root, "./", nil, true)
		_ = itemsFromDeclarations([]declarationDefinition{{Name: "User", Kind: protocol.CompletionItemKindStruct, Detail: "schema"}}, "Us")
	})

	It("covers resolution and completion value helpers", func() {
		workspace := GinkgoT().TempDir()
		sharedPath := filepath.Join(workspace, "shared.mace")
		tAssert.NoError(os.WriteFile(sharedPath, []byte(`
[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Numbers: { values: array<string>; };
}
`), 0o644))
		documentPath := filepath.Join(workspace, "doc.mace")
		text := `|===|
string name = "Ada";
string digits = ["x", "y"];
|===|
[output = data, schema = User]
{
  user: $self.user.profile,
  list: digits[0],
  items: ["a", "b"],
}
`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		uri := protocol.DocumentUri(fileURI(documentPath))
		snapshot := AnalyzeDocumentAt(text, documentPath)
		doc := document{text: text, analysis: snapshot}

		userValue := processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"profile": {Kind: processor.ValueString, String: "Ada"}}}
		selfValue := processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"user": userValue}}
		_, _ = outputValueAtSegments(selfValue, []string{"user", "profile"})
		_, _ = outputValueAtSegments(selfValue, []string{"missing"})
		_, _ = outputValueAtSegments(processor.Value{Kind: processor.ValueString, String: "Ada"}, []string{"user"})
		_, _ = resolveCompletionValue(ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: "profile"}, map[string]processor.Value{"user": userValue}, selfValue)
		_, _ = resolveCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "items"}, Index: ast.IntLiteral{Lexeme: "x"}}, map[string]processor.Value{"items": {Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueString, String: "a"}}}}, selfValue)
		_, _ = resolveCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"a"`}, ast.BooleanLiteral{Value: true}}}, nil, selfValue)
		_, _ = resolveCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, nil, selfValue)
		_, _ = resolveCompletionValue(ast.Identifier{Name: "missing"}, nil, selfValue)
		_, _ = resolveCompletionValue(ast.SelfReference{Path: []string{"user", "profile"}}, nil, selfValue)

		locals := map[string]ast.Expression{
			"name": ast.StringLiteral{Lexeme: `"Ada"`},
			"user": ast.RecordLiteral{Fields: []ast.RecordField{{Name: "profile", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}},
			"items": ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"a"`}, ast.StringLiteral{Lexeme: `"b"`}}},
		}
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "name"}, locals, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "missing"}, locals, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "name"}, locals, map[string]struct{}{"name": {}})
		_, _ = resolveLocalCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "items"}, Index: ast.IntLiteral{Lexeme: "10"}}, locals, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "name"}, ast.StringLiteral{Lexeme: `"Ada"`}}}, locals, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, locals, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.BooleanLiteral{Value: true}, locals, map[string]struct{}{})

		_, _ = partialScriptFile(text, protocol.Position{Line: 1, Character: 2})
		_, _ = partialScriptFile("[output = data]\n{}", protocol.Position{Line: 1, Character: 0})
		_ = scriptVariablesForOutput(text, uri)
		_ = processVariablesInDocument(text, uri)
		_, _ = partialOutputResult(doc, uri, protocol.Position{Line: 6, Character: 10})
		_, _ = partialOutputResult(doc, uri, protocol.Position{Line: 0, Character: 0})
		_, _ = outputFieldRanges(text, lexAnalysisTokens(text), strings.Index(text, "{"))
		_ = isOutputFieldHeader(lexAnalysisTokens("user: value"), 0)
		_ = isOutputFieldHeader(lexAnalysisTokens("user?: value"), 0)
		_ = isOutputFieldHeader(lexAnalysisTokens("user value"), 0)
		_, _ = selfCompletionValue(doc, uri, protocol.Position{Line: 6, Character: 10}, []string{"user", "profile"})
		_ = selfCompletionEntries(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"b": {}, "a": {}}})
		_, _ = selfCompletionValue(doc, uri, protocol.Position{Line: 6, Character: 10}, []string{"missing"})
		_, _ = resolveArrayCompletionTarget(doc, uri, protocol.Position{Line: 7, Character: 12}, "digits", completionScopeScript)
		_, _ = resolveLocalArrayCompletionTarget(text, protocol.Position{Line: 7, Character: 12}, ast.Identifier{Name: "digits"})
		_, _ = arrayIndexCompletionItems(doc, uri, protocol.Position{Line: 7, Character: 12}, "items[", completionScopeOutput)
		_, _ = arrayIndexCompletionItems(doc, uri, protocol.Position{Line: 7, Character: 12}, "items[x", completionScopeOutput)
		_, _ = parsedVariableMemberCompletionItems(doc, uri, "user.profile.", protocol.Position{Line: 6, Character: 18})
	})

	It("covers analysis helper branches", func() {
		workspace := GinkgoT().TempDir()
		sharedPath := filepath.Join(workspace, "shared.mace")
		tAssert.NoError(os.WriteFile(sharedPath, []byte(`
[output = schema]
{
  User: { name: string; profile: { city: string; }; tags: string; };
  Runtime: { env: string; };
}
`), 0o644))
		documentPath := filepath.Join(workspace, "doc.mace")
		text := `|===|
from "./shared.mace" import User, Runtime;
type Alias: string;
schema Doc: { field: string; };
Profile record = { age: 1; active: true; };
|===|
[output = data, schema = User, schema_file = "./shared.mace", parse = Runtime, parse_file = "./shared.mace"]
{
  user: { name: "Ada"; profile: { city: "LA"; }; tags: "x"; };
  list: ["a", "b"];
  mixed: ["a", 1];
  item: values[0];
  self_ref: $self.user.profile;
}
`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		snapshot := AnalyzeDocumentAtInRoot(text, documentPath, workspace)
		uri := protocol.DocumentUri(fileURI(documentPath))
		_ = AnalyzeDocumentAt(text, documentPath)
		_ = Diagnostics(snapshot)
		_ = DocumentSymbols(text, snapshot)
		_ = Hover(text, snapshot, protocol.Position{Line: 8, Character: 10})
		_, _ = Definition(snapshot, protocol.Position{Line: 2, Character: 6})
		_ = CodeActions(snapshot, uri, protocol.Range{})
		_ = simpleExpressionText(ast.BooleanLiteral{Value: true})
		_ = simpleExpressionText(ast.Identifier{Name: "value"})
		_ = defaultExpressionForType(ast.PrimitiveType{Name: "hex_int"})
		_ = inferredTypeFromExpression(ast.HexFloatLiteral{Lexeme: "0x0.0"})
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"Ada"`})
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Runtime"}}}}, workspace)
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./shared.mace"`}}}}, workspace)
		_ = importedSemanticSymbols(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}, {Name: "Runtime"}}}}}, documentPath)
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User"}}}}, documentPath, "Shared")
		_, _, _, _ = parsedFile(documentPath)
		_ = summarizeValue(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueInt, Int: 1}}})
		_ = expressionSummary(ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: "profile"})
		_ = indexSymbols([]semanticSymbol{{Name: "a"}})
		_, _ = outputDirectiveListRange("[output = data, schema = User]")
		_, _, _ = schemaFileDirectiveRanges("[output = data, schema_file = \"./shared.mace\"]")
		_, _ = importAndScriptCleanupRange(text)
		_ = quotedName(`unknown schema "Name"`)
		_, _ = addMissingScriptSemicolonText(text)
		_, _ = moveScriptBlockBeforeOutputText(text)
		_, _ = extractRecordLiteralIntoSchemaText(text)
		_ = inferRecordSchemaFields("count: 1; items: [\"x\"];")
		_, _ = replaceVariableDeclaration(text, regexp.MustCompile(`(?m)^([ \t]*)(string)\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`), func(matches []string) string {
			return matches[1] + matches[2] + " " + matches[3] + " = \"\";"
		})
		_, _ = renameDuplicateVariableText(text)
		_ = semanticCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath, protocol.Diagnostic{}, "unknown field \"field\"")
		_ = semanticCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath, protocol.Diagnostic{}, "duplicate field \"field\"")
		_ = semanticCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath, protocol.Diagnostic{}, "processor: type mismatch: expected string, got int")
		_ = semanticCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath, protocol.Diagnostic{}, "schema directive is invalid when output mode is schema")
		_ = semanticCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath, protocol.Diagnostic{}, "missing required field \"field\"")
		_ = semanticCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath, protocol.Diagnostic{}, "import path not found")
		_ = importResolutionCodeActions(text, ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, lexAnalysisTokens(text), documentPath)
		_ = unavailableImportDiagnostics(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, lexAnalysisTokens(text), documentPath)
		_ = unavailableImportNameSet(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./shared.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, documentPath)
		_ = documentationCodeActions(text, ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.TypeDeclaration{Name: "Alias", NameToken: lexer.Token{Line: 3, Column: 6, Lexeme: "Alias"}, Type: ast.PrimitiveType{Name: "string"}}, ast.VariableDeclaration{Name: "value", NameToken: lexer.Token{Line: 3, Column: 6, Lexeme: "value"}, Type: ast.PrimitiveType{Name: "string"}}}}}, lexAnalysisTokens(text), documentPath)
		_ = editorRefactorCodeActions(text, ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.SchemaDeclaration{Name: "Doc", NameToken: lexer.Token{Line: 4, Column: 8, Lexeme: "Doc"}, Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "field", Type: ast.PrimitiveType{Name: "string"}}}}}, ast.VariableDeclaration{Name: "record", NameToken: lexer.Token{Line: 5, Column: 1, Lexeme: "record"}, Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "age", Type: ast.PrimitiveType{Name: "int"}}}}, Value: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "age", Value: ast.IntLiteral{Lexeme: "1"}}}}}}}, Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}, DataFields: []ast.OutputField{{Name: "user", Value: ast.Identifier{Name: "user"}}}}}, lexAnalysisTokens(text), documentPath)
		addStringRefactorActions(text, uri, fullDocumentRange(text), &[]analysisCodeActionCandidate{})
		schemaFile := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.SchemaDeclaration{Name: "Doc", NameToken: lexer.Token{Line: 4, Column: 8, Lexeme: "Doc"}, Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "field", Type: ast.PrimitiveType{Name: "string"}}, {Name: "field", Type: ast.PrimitiveType{Name: "string"}}}}}}}}
		addSchemaDeclarationRefactorActions(text, schemaFile, lexAnalysisTokens(text), func(string, protocol.Range, ast.File) {})
		variableFile := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "record", NameToken: lexer.Token{Line: 5, Column: 1, Lexeme: "record"}, Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "age", Type: ast.PrimitiveType{Name: "int"}}}}, Value: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "age", Value: ast.IntLiteral{Lexeme: "1"}}}}}}}}
		addVariableDeclarationRefactorActions(text, variableFile, lexAnalysisTokens(text), func(string, protocol.Range, ast.File) {})
		_, _ = replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}, []string{"profile", "name"}, ast.NamedType{Name: "Alias"})
		_, _ = replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.PrimitiveType{Name: "string"}}}}, []string{"profile", "name"}, ast.NamedType{Name: "Alias"})
		_ = documentationCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath)
		_ = editorRefactorCodeActions(text, ast.File{}, lexAnalysisTokens(text), documentPath)
		_ = analyzeDocumentAtInRoot(text, documentPath, workspace)

		singleSchemaPath := filepath.Join(workspace, "single.mace")
		tAssert.NoError(os.WriteFile(singleSchemaPath, []byte(`
[output = schema]
{
  Only: { value: string; };
}
`), 0o644))
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./single.mace"`}}}}, workspace)
		_ = importedSemanticSymbols(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./single.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Only"}}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, documentPath)
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "Only"}}}}, documentPath, "Alias")
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "Only", Value: ast.StringLiteral{Lexeme: `"x"`}}}}}, documentPath, "Alias")
		_, _ = addMissingScriptSemicolonText("|===|\ntype Alias: string\n|===|")
		_, _ = moveScriptBlockBeforeOutputText("[output = data]\n{}\n|===|\ntype Alias: string;\n|===|")
		_, _ = extractRecordLiteralIntoSchemaText("|===|\nProfile record = { age: 1; active: true; };\n|===|")
		_, _ = replaceVariableDeclaration("string value = \"x\";", regexp.MustCompile(`(?m)^([ \t]*)(string)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*");`), func(matches []string) string { return matches[1] + matches[2] + " fresh = " + matches[4] + ";" })
		_, _ = semanticDiagnosticFromError(ast.File{}, nil, processor.DiagnosticError{Code: processor.CodeInvalidNullUsage, Message: "null bad"})
		_, _ = semanticDiagnosticFromError(ast.File{}, nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch, Message: "processor: type mismatch: expected string, got int"})
		_ = importResolutionCodeActions(text, ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./single.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Only"}}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, lexAnalysisTokens(text), documentPath)
		_ = unavailableImportDiagnostics(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./single.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, lexAnalysisTokens(text), documentPath)
		_ = unavailableImportNameSet(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./single.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, documentPath)
		_ = documentationCodeActions(text, ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.TypeDeclaration{Name: "Alias", NameToken: lexer.Token{Line: 3, Column: 6, Lexeme: "Alias"}, Type: ast.PrimitiveType{Name: "string"}}, ast.SchemaDeclaration{Name: "Doc", NameToken: lexer.Token{Line: 4, Column: 8, Lexeme: "Doc"}, Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "field", Type: ast.PrimitiveType{Name: "string"}}}}}, ast.VariableDeclaration{Name: "value", NameToken: lexer.Token{Line: 3, Column: 6, Lexeme: "value"}, Type: ast.PrimitiveType{Name: "string"}}}}}, lexAnalysisTokens(text), documentPath)
		_ = editorRefactorCodeActions(text, ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.SchemaDeclaration{Name: "Doc", NameToken: lexer.Token{Line: 4, Column: 8, Lexeme: "Doc"}, Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "field", Type: ast.PrimitiveType{Name: "string"}}}}}, ast.VariableDeclaration{Name: "record", NameToken: lexer.Token{Line: 5, Column: 1, Lexeme: "record"}, Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "age", Type: ast.PrimitiveType{Name: "int"}}}}, Value: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "age", Value: ast.IntLiteral{Lexeme: "1"}}}}}}}, Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}, DataFields: []ast.OutputField{{Name: "user", Value: ast.Identifier{Name: "user"}}}}}, lexAnalysisTokens(text), documentPath)
	})

	It("covers completion item entry points", func() {
		workspace := GinkgoT().TempDir()
		sharedPath := filepath.Join(workspace, "shared.mace")
		tAssert.NoError(os.WriteFile(sharedPath, []byte(`
[output = schema]
{
  User: { name: string; home: { city: string; }; };
  Runtime: { env: string; };
}
`), 0o644))
		documentPath := filepath.Join(workspace, "doc.mace")
		text := `from "./shared.mace" import 
|===|
string value = "x";
|===|
[output = data, parse_file = "./shared.mace"]
{
  result: 
  member: user.
  index: values[
  selfy: $self.user.
}
`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		snapshot := AnalyzeDocumentAt(text, documentPath)
		doc := document{text: text, analysis: snapshot}
		uri := protocol.DocumentUri(fileURI(documentPath))

		_, _ = importCompletionItems(doc, "from \"./", uri)
		_, _ = importCompletionItems(doc, "from \"./shared.mace\" import Un", uri)
		_, _ = importCompletionItems(doc, "from \"./shared.mace\" ", uri)
		_, _ = directiveCompletionItems(doc, uri, "[")
		_, _ = directiveCompletionItems(doc, uri, "[output = data,")
		_, _ = directiveCompletionItems(doc, uri, "[output = schema, schema = ")
		_, _ = stringLiteralInitializerCompletionItems(doc, uri, protocol.Position{Line: 0, Character: 13}, false)
		_, _ = stringLiteralInitializerCompletionItems(doc, uri, protocol.Position{Line: 4, Character: 12}, true)
		_, _ = initializerCompletionItems(doc, uri, protocol.Position{Line: 2, Character: 18})
		_, _ = outputInitializerCompletionItems(doc, uri, protocol.Position{Line: 5, Character: 11})
		_ = completionItems(doc, uri, protocol.Position{Line: 0, Character: 5})
		_ = completionItems(doc, uri, protocol.Position{Line: 2, Character: 16})
		_ = completionItems(doc, uri, protocol.Position{Line: 5, Character: 11})
		_ = completionItems(doc, uri, protocol.Position{Line: 6, Character: 14})
		_ = completionItems(doc, uri, protocol.Position{Line: 7, Character: 18})
		_ = completionItems(doc, uri, protocol.Position{Line: 8, Character: 17})
	})
})
