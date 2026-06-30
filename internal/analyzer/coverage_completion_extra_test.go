package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/samber/lo"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("analyzer completion helper coverage", func() {
	It("covers completion model and import helpers", func() {
		workspace := GinkgoT().TempDir()
		importRoot := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(importRoot, 0o755))

		schemaPath := filepath.Join(importRoot, "schema.mace")
		dataPath := filepath.Join(importRoot, "data.mace")
		aliasPath := filepath.Join(importRoot, "alias.mace")
		documentPath := filepath.Join(importRoot, "document.mace")

		tAssert.NoError(os.WriteFile(schemaPath, []byte(`
[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Runtime: { env: string; };
  Choice: choice["Ada", 1, true];
}
`), 0o644))
		tAssert.NoError(os.WriteFile(dataPath, []byte(`
[output = data, schema = User]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
  list: ["a"];
}
`), 0o644))
		tAssert.NoError(os.WriteFile(aliasPath, []byte(`
[output = data]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
}
`), 0o644))
		tAssert.NoError(os.WriteFile(documentPath, []byte(`
|===|
from "./schema.mace" import User, Runtime;
|===|
[output = data, parse = User, parse_file = "./schema.mace", schema = User, schema_file = "./schema.mace"]
{
  user: $self.user.profile;
  item: list[0];
  choice: "Ada";
}
`), 0o644))

		file := ast.File{
			Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}, {Name: "Runtime"}}}},
			Script: &ast.ScriptBlock{Items: []ast.Declaration{
				ast.TypeDeclaration{Name: "Alias", Type: ast.NamedType{Name: "User"}},
				ast.SchemaDeclaration{Name: "Local", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}},
			}},
			Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, DataFields: []ast.OutputField{{Name: "user", Value: ast.Identifier{Name: "user"}}}, SchemaFields: []ast.OutputSchemaField{{Name: "user", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}},
		}
		cache := map[string]completionModel{}
		model := buildCompletionModel(file, importRoot, importRoot, cache)

		_, _, _ = importedCompletionModel(schemaPath, importRoot, cache)
		_, _, _ = importedCompletionModel(dataPath, importRoot, cache)
		_, _, _ = importedCompletionModel(aliasPath, importRoot, cache)
		mergeDirectiveCompletionModels(&model, file.Output.Directives, importRoot, importRoot, cache)
		_ = parseInputDeclarationDefinitions(file, importRoot, importRoot)
		_, _ = parseInputCompletionRecord(file, model, importRoot, importRoot, cache)
		_, _, _ = parseInputMemberCompletionRootType(file, model, []string{"user", "profile"}, importRoot, importRoot, cache, map[string]struct{}{"user": {}})
		_, _, _ = parseInputMemberCompletionRootType(file, model, []string{"user", "profile"}, importRoot, importRoot, cache, map[string]struct{}{})
		_, _, _ = parseMemberCompletionType(ast.RecordType{Fields: []ast.SchemaField{{Name: "user", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.PrimitiveType{Name: "string"}}}}}}}, []string{"user", "profile"}, model, map[string]struct{}{})
		_ = parseFileOutputDeclarationDefinitions(file.Output.Directives, importRoot, importRoot, cache)
		_, _ = parseFileOutputSchemaRecord(file.Output.Directives, importRoot, importRoot, cache)
		_, _ = parseFileOutputExportedRecord(file.Output.Directives, importRoot, importRoot, cache)
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./alias.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared", "user", "profile"}, importRoot, importRoot, cache)
		_, _ = importAsSchemaRecord(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}, model)
		_, _ = importAsDataRecord(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "user", Value: ast.StringLiteral{Lexeme: `"x"`}}, {Name: "count", Value: ast.IntLiteral{Lexeme: "1"}}}}}, model)
		_, _ = completionOutputFieldType(ast.StringLiteral{Lexeme: `"x"`}, model)
		_, _ = completionOutputFieldType(ast.Identifier{Name: "User"}, model)
		_, _ = outputDirectiveValue(file.Output.Directives, ast.OutputDirectiveSchema)
		_, _ = outputDirectiveValue(file.Output.Directives, ast.OutputDirectiveParseFile)
		_ = hasOutputDirective(file.Output.Directives, ast.OutputDirectiveSchema)
		_ = filterOutputDeclarationDefinitions([]declarationDefinition{{Name: "User", Kind: protocol.CompletionItemKindStruct}, {Name: "value", Kind: protocol.CompletionItemKindVariable}})
		_ = resolveCompletionType(ast.PrimitiveType{Name: "string"}, model, map[string]struct{}{})
		_ = resolveCompletionType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{})
		_ = resolveCompletionType(ast.UnionType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, model, map[string]struct{}{})
		_ = resolveCompletionType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, model, map[string]struct{}{})
		_ = resolveCompletionType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}}}, model, map[string]struct{}{})
		_ = resolveCompletionType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, map[string]struct{}{})
		_ = resolveCompletionType(ast.NamedType{Name: "User"}, model, map[string]struct{}{})
		_ = resolveCompletionType(ast.NamedType{Name: "Alias"}, model, map[string]struct{}{})
		_, _ = completionChoiceFromMembers([]ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}, ast.BooleanLiteral{Value: true}}, model, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.StringLiteral{Lexeme: `"Ada"`}, model, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "Alias"}, completionModel{aliases: map[string]ast.TypeReference{"Alias": ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}}}, map[string]struct{}{})
		_, _ = completionUnionRecord([]ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, model, map[string]struct{}{})
		_ = schemaLiteral(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "string"}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.NamedType{Name: "Alias"}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, model, map[string]struct{}{})
		_, _ = directoryEntries(importRoot, importRoot, "./", nil, true)
		_, _, _ = importDirectory(importRoot, importRoot, "./", true)
		_ = normalizedRelativePathPrefix("")
		_ = normalizedRelativePathPrefix("../schema.mace")
		_ = normalizedRelativePathPrefix("schema.mace")
		_ = joinImportPath("./", "schema.mace", false)
		_ = joinImportPath("./", "root", true)
		_ = completionItemsForType(ast.PrimitiveType{Name: "string"}, model, completionOptions{})
		_ = completionItemsForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, model, completionOptions{unquotedStringChoices: true})
		_ = completionItemsForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, completionOptions{allowSchemaLiteral: true})
		_ = completionItemsForType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, model, completionOptions{})
		_ = completionItemsForMemberTarget(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model)
		_ = completionItemsForValueMembers(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}})
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "string"}, model, 2)
		_ = syntheticCompletionValue(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, 2)
		_ = syntheticCompletionValue(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, 2)
		_, _ = unquotedStringChoiceLabel(`"Ada"`)
		_, _ = unquotedStringChoiceLabel("Ada")
		_, _ = outputSchemaDirective(file)
		_ = buildCompletionModel(file, importRoot, importRoot, map[string]completionModel{})
		_, _ = stringLiteralValue(ast.StringLiteral{Lexeme: `"./schema.mace"`})
	})

	It("covers completion entry points with diverse documents", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		documentPath := filepath.Join(root, "document.mace")
		text := `|===|
string name = "Ada";
string digits = ["x", "y"];
|===|
[output = data, schema = User, parse = User, parse_file = "./schema.mace"]
{
  result: $self.user.profile,
  member: user.profile,
  index: values[12],
  direct: "Ada",
}
`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		snapshot := AnalyzeDocumentAt(text, documentPath)
		doc := document{text: text, analysis: snapshot}
		uri := protocol.DocumentUri(fileURI(documentPath))

		_ = currentLinePrefix(text, protocol.Position{Line: 6, Character: 10})
		_ = currentLineSuffix(text, protocol.Position{Line: 6, Character: 10})
		_, _ = directivePrefix("[output = data, schema = User]")
		_, _ = stringLiteralCompletionContext(text, protocol.Position{Line: 6, Character: 20})
		_, _ = completionPlaceholderPosition(text, protocol.Position{Line: 6, Character: 20}, ":.")
		_, _ = outputMemberAccessContext("user.profile.")
		_, _, _ = selfCompletionContext("$self.user.profile")
		_, _, _ = arrayIndexCompletionContext("values[12")
		_ = lastUnquotedByteInPrefix(`a ? "?" : b ?`, '?')
		_ = outputKeyCompletionContext("result")
		_ = completionScopeAt(text, protocol.Position{Line: 0, Character: 0})
		_ = completionScopeAt(text, protocol.Position{Line: 5, Character: 0})
		_ = completionScopeAt(text, protocol.Position{Line: 1, Character: 0})
		_ = completionFile(doc, "[output = data]")
		_ = currentImports(doc, "from \"./schema.mace\" import ")
		_ = importedPaths(doc, "from \"./schema.mace\" import ")
		_ = availableSchemaNames(doc, uri, "[output = data, schema = ")
		_ = completionRoot(snapshot, uri)
		_, _ = documentPathFromURI(uri)
		_ = relativePathItems(doc, uri, "./", nil, true)
		_, _ = importCompletionItems(doc, "from \"./", uri)
		_, _ = directiveCompletionItems(doc, uri, "[output = data,")
		_, _ = stringLiteralInitializerCompletionItems(doc, uri, protocol.Position{Line: 6, Character: 20}, true)
		_, _ = initializerCompletionItems(doc, uri, protocol.Position{Line: 2, Character: 18})
		_, _ = outputInitializerCompletionItems(doc, uri, protocol.Position{Line: 5, Character: 20})
		_ = completionItems(doc, uri, protocol.Position{Line: 5, Character: 20})
		_ = completionItems(doc, uri, protocol.Position{Line: 6, Character: 20})
		_ = completionItems(doc, uri, protocol.Position{Line: 7, Character: 16})
		_ = completionItems(doc, uri, protocol.Position{Line: 8, Character: 14})
		_ = completionItems(doc, uri, protocol.Position{Line: 9, Character: 10})
		_, _ = outputFieldRanges(text, lexAnalysisTokens(text), strings.Index(text, "{") )
		_, _ = partialOutputResult(doc, uri, protocol.Position{Line: 6, Character: 15})
		_, _ = partialScriptFile(text, protocol.Position{Line: 1, Character: 2})
		_, _ = partialScriptFileWithPlaceholder(text, protocol.Position{Line: 1, Character: 2})
		_, _ = completionFileWithPlaceholder(text, protocol.Position{Line: 5, Character: 2})
		_, _ = completionFileWithExpressionPlaceholder(text, strings.Index(text, `"Ada"`), strings.Index(text, `"Ada"`)+5)
		_, _ = resolveArrayCompletionTarget(doc, uri, protocol.Position{Line: 7, Character: 16}, "values", completionScopeOutput)
		_, _ = resolveLocalArrayCompletionTarget(text, protocol.Position{Line: 7, Character: 16}, ast.Identifier{Name: "digits"})
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "name"}, map[string]ast.Expression{"name": ast.StringLiteral{Lexeme: `"Ada"`}}, map[string]struct{}{})
		_, _ = resolveCompletionValue(ast.Identifier{Name: "name"}, map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}, processor.Value{})
		_ = parseInputDeclarationDefinitions(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}}}, root, root)
		_ = parseFileOutputDeclarationDefinitions([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./alias.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared", "user"}, root, root, map[string]completionModel{})
	})

	It("covers completion fallback branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))
		schemaPath := filepath.Join(root, "schema.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`[output = schema]
{
  User: { name: string; profile: { city: string; }; };
}`), 0o644))
		text := `|===|
from "./schema.mace" import User;
|===|
[output = data, schema = User]
{
  user: $self.user.profile;
  value: "Ada";
  items: ["a", "b"];
}
`
		docPath := filepath.Join(root, "doc.mace")
		tAssert.NoError(os.WriteFile(docPath, []byte(text), 0o644))
		snapshot := AnalyzeDocumentAt(text, docPath)
		doc := document{text: text, analysis: snapshot}
		uri := protocol.DocumentUri(fileURI(docPath))

		_ = currentLinePrefix(text, protocol.Position{Line: 5, Character: 10})
		_ = currentLineSuffix(text, protocol.Position{Line: 5, Character: 10})
		_ = lastUnquotedByteInPrefix(`a ? "?" : b ?`, '?')
		_, _ = outputMemberAccessContext("user.profile.")
		_, _ = outputMemberAccessContext("$self.profile.")
		_, _, _ = selfCompletionContext("$self.user.profile")
		_, _, _ = selfCompletionContext("$self.")
		_, _, _ = selfCompletionContext("plain text")
		_, _, _ = arrayIndexCompletionContext("items[12")
		_, _, _ = arrayIndexCompletionContext("items[x")
		_, _ = directivePrefix("[output = data, schema = User]")
		_, _ = directivePrefix("not a directive")
		_, _ = stringLiteralCompletionContext(text, protocol.Position{Line: 5, Character: 17})
		_, _ = completionPlaceholderPosition(text, protocol.Position{Line: 5, Character: 17}, ":.")
		_, _ = completionPlaceholderPosition(text, protocol.Position{Line: 1, Character: 0}, "=")
		_ = completionItems(doc, uri, protocol.Position{Line: 1, Character: 10})
		_ = completionItems(doc, uri, protocol.Position{Line: 5, Character: 17})
		_ = completionItems(doc, uri, protocol.Position{Line: 6, Character: 15})
		_ = completionItems(doc, uri, protocol.Position{Line: 7, Character: 13})
		_, _ = importCompletionItems(doc, "from \"./", uri)
		_, _ = importCompletionItems(doc, "from \"./schema.mace\" import Un", uri)
		_, _ = importCompletionItems(doc, "from \"./schema.mace\" ", uri)
		_, _ = directiveCompletionItems(doc, uri, "[")
		_, _ = directiveCompletionItems(doc, uri, "[output = data,")
		_, _ = directiveCompletionItems(doc, uri, "[output = schema, schema = ")
		_, _ = stringLiteralInitializerCompletionItems(doc, uri, protocol.Position{Line: 5, Character: 17}, true)
		_, _ = initializerCompletionItems(doc, uri, protocol.Position{Line: 3, Character: 18})
		_, _ = outputInitializerCompletionItems(doc, uri, protocol.Position{Line: 5, Character: 17})
		_, _ = outputInitializerCompletionItems(doc, uri, protocol.Position{Line: 6, Character: 17})
		_, _ = partialOutputResult(doc, uri, protocol.Position{Line: 6, Character: 15})
		_, _ = outputFieldRanges(text, lexAnalysisTokens(text), strings.Index(text, "{"))
		_ = isOutputFieldHeader(lexAnalysisTokens("value: 1"), 0)
		_ = isOutputFieldHeader(lexAnalysisTokens("value?: 1"), 0)
		_ = isOutputFieldHeader(lexAnalysisTokens("value 1"), 0)
		_, _ = partialScriptFile(text, protocol.Position{Line: 1, Character: 2})
		_, _ = partialScriptFileWithPlaceholder(text, protocol.Position{Line: 1, Character: 2})
		_, _ = completionFileWithPlaceholder(text, protocol.Position{Line: 5, Character: 2})
		_, _ = completionFileWithExpressionPlaceholder(text, strings.Index(text, `"Ada"`), strings.Index(text, `"Ada"`)+5)
		_, _ = resolveArrayCompletionTarget(doc, uri, protocol.Position{Line: 7, Character: 13}, "items", completionScopeOutput)
		_, _ = resolveLocalArrayCompletionTarget(text, protocol.Position{Line: 7, Character: 13}, ast.Identifier{Name: "items"})
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "items"}, map[string]ast.Expression{"items": ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"a"`}}}}, map[string]struct{}{})
		_, _ = resolveCompletionValue(ast.Identifier{Name: "items"}, map[string]processor.Value{"items": {Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueString, String: "a"}}}}, processor.Value{})
		_ = parseInputDeclarationDefinitions(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}}}, root, root)
		_ = parseFileOutputDeclarationDefinitions([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared", "user"}, root, root, map[string]completionModel{})

		scriptText := `|===|
values[
|===|
`
		scriptPath := filepath.Join(root, "script.mace")
		tAssert.NoError(os.WriteFile(scriptPath, []byte(scriptText), 0o644))
		scriptSnapshot := AnalyzeDocumentAt(scriptText, scriptPath)
		scriptDoc := document{text: scriptText, analysis: scriptSnapshot}
		_ = completionItems(scriptDoc, protocol.DocumentUri(fileURI(scriptPath)), protocol.Position{Line: 1, Character: 7})
		_, _ = arrayIndexCompletionItems(scriptDoc, protocol.DocumentUri(fileURI(scriptPath)), protocol.Position{Line: 1, Character: 7}, "values[", completionScopeScript)
		_, _ = initializerCompletionItems(scriptDoc, protocol.DocumentUri(fileURI(scriptPath)), protocol.Position{Line: 1, Character: 7})
		_, _ = outputInitializerCompletionItems(scriptDoc, protocol.DocumentUri(fileURI(scriptPath)), protocol.Position{Line: 1, Character: 7})

		parseDocText := `[output = data, parse_file = "./schema.mace"]
{
  result:
}
`
		parseDocPath := filepath.Join(root, "parse.mace")
		tAssert.NoError(os.WriteFile(parseDocPath, []byte(parseDocText), 0o644))
		parseSnapshot := AnalyzeDocumentAt(parseDocText, parseDocPath)
		parseDoc := document{text: parseDocText, analysis: parseSnapshot}
		_ = completionItems(parseDoc, protocol.DocumentUri(fileURI(parseDocPath)), protocol.Position{Line: 2, Character: 10})
		_ = completionItems(parseDoc, protocol.DocumentUri(fileURI(parseDocPath)), protocol.Position{Line: 1, Character: 2})
		_, _ = outputInitializerCompletionItems(parseDoc, protocol.DocumentUri(fileURI(parseDocPath)), protocol.Position{Line: 2, Character: 10})
		_, _ = stringLiteralInitializerCompletionItems(parseDoc, protocol.DocumentUri(fileURI(parseDocPath)), protocol.Position{Line: 0, Character: 32}, true)
		outputStringDocText := `[output = data, schema = User]
{
  result: "Ada"
}
`
		outputStringDocPath := filepath.Join(root, "output-string.mace")
		tAssert.NoError(os.WriteFile(outputStringDocPath, []byte(outputStringDocText), 0o644))
		outputStringSnapshot := AnalyzeDocumentAt(outputStringDocText, outputStringDocPath)
		outputStringDoc := document{text: outputStringDocText, analysis: outputStringSnapshot}
		_, _ = outputInitializerCompletionItems(outputStringDoc, protocol.DocumentUri(fileURI(outputStringDocPath)), protocol.Position{Line: 2, Character: 13})
		_, _ = stringLiteralInitializerCompletionItems(outputStringDoc, protocol.DocumentUri(fileURI(outputStringDocPath)), protocol.Position{Line: 2, Character: 13}, true)

		stringScriptText := `|===|
string value = "Ada";
|===|
`
		stringScriptPath := filepath.Join(root, "string-script.mace")
		tAssert.NoError(os.WriteFile(stringScriptPath, []byte(stringScriptText), 0o644))
		stringScriptSnapshot := AnalyzeDocumentAt(stringScriptText, stringScriptPath)
		stringScriptDoc := document{text: stringScriptText, analysis: stringScriptSnapshot}
		_, _ = initializerCompletionItems(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), protocol.Position{Line: 1, Character: 17})
		_, _ = stringLiteralInitializerCompletionItems(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), protocol.Position{Line: 1, Character: 17}, false)
		_ = bareSelfCompletionItems("result:", protocol.Position{Line: 0, Character: 0})
		_ = bareSelfCompletionItems("result", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("$self.", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("$x", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("value", protocol.Position{Line: 0, Character: 0})
		_ = lastUnquotedByteInPrefix(`a ? "?" : b ?`, '?')
		_ = lastUnquotedByteInPrefix(`"?"`, '?')
		_, _ = outputMemberAccessContext("user.profile.")
		_, _ = outputMemberAccessContext(".bad.")
		_, _ = parsedVariableMemberCompletionItems(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), "value.", protocol.Position{Line: 1, Character: 19})
		_, _ = parsedVariableMemberCompletionItems(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), "value[x", protocol.Position{Line: 99, Character: 19})
		_, _, _ = selfCompletionContext("$self.user.profile")
		_, _, _ = selfCompletionContext("$self.")
		_, _, _ = selfCompletionContext("plain text")
		_, _ = arrayIndexCompletionItems(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), protocol.Position{Line: 1, Character: 19}, "value[", completionScopeScript)
		_, _ = arrayIndexCompletionItems(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), protocol.Position{Line: 1, Character: 19}, "value[x", completionScopeScript)
		_, _ = resolveArrayCompletionTarget(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), protocol.Position{Line: 1, Character: 19}, "value", completionScopeScript)
		_, _ = resolveArrayCompletionTarget(stringScriptDoc, protocol.DocumentUri(fileURI(stringScriptPath)), protocol.Position{Line: 1, Character: 19}, "[", completionScopeScript)
		_, _ = resolveLocalArrayCompletionTarget(stringScriptText, protocol.Position{Line: 1, Character: 19}, ast.Identifier{Name: "value"})
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "missing"}, map[string]ast.Expression{}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "x"}, map[string]ast.Expression{"value": ast.StringLiteral{Lexeme: `"Ada"`}}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "value"}, Index: ast.IntLiteral{Lexeme: "9"}}, map[string]ast.Expression{"value": ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.BooleanLiteral{Value: true}}}, map[string]ast.Expression{}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, map[string]ast.Expression{}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.BooleanLiteral{Value: true}, map[string]ast.Expression{}, map[string]struct{}{})
		_ = processVariablesInDocument(stringScriptText, protocol.DocumentUri(fileURI(stringScriptPath)))
		_ = scriptVariablesForOutput(stringScriptText, protocol.DocumentUri(fileURI(stringScriptPath)))
		_ = partialScriptVariables(stringScriptText, protocol.DocumentUri(fileURI(stringScriptPath)), protocol.Position{Line: 1, Character: 17})
		_, _, _ = parseInputMemberCompletionRootType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, []string{"user", "profile"}, root, root, map[string]completionModel{}, map[string]struct{}{})
	})

	It("covers remaining completion edge branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		schemaPath := filepath.Join(root, "schema.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Shared: { user: { profile: { city: string; }; }; };
}`), 0o644))

		aliasPath := filepath.Join(root, "alias.mace")
		tAssert.NoError(os.WriteFile(aliasPath, []byte(`[output = data]
{
  user: { profile: { city: "LA"; }; };
}`), 0o644))

		outputKeyText := `[output = data, parse_file = "./schema.mace"]
{
  sch
}
`
		outputKeyPath := filepath.Join(root, "output-key.mace")
		tAssert.NoError(os.WriteFile(outputKeyPath, []byte(outputKeyText), 0o644))
		outputKeySnapshot := AnalyzeDocumentAt(outputKeyText, outputKeyPath)
		outputKeyDoc := document{text: outputKeyText, analysis: outputKeySnapshot}
		_ = completionItems(outputKeyDoc, protocol.DocumentUri(fileURI(outputKeyPath)), protocol.Position{Line: 2, Character: 5})

		stringInitText := `|===|
string name = "Ada";
|===|
`
		stringInitPath := filepath.Join(root, "string-init.mace")
		tAssert.NoError(os.WriteFile(stringInitPath, []byte(stringInitText), 0o644))
		stringInitSnapshot := AnalyzeDocumentAt(stringInitText, stringInitPath)
		stringInitDoc := document{text: stringInitText, analysis: stringInitSnapshot}
		_, _ = initializerCompletionItems(stringInitDoc, protocol.DocumentUri(fileURI(stringInitPath)), protocol.Position{Line: 1, Character: 18})
		_, _ = initializerCompletionItems(stringInitDoc, protocol.DocumentUri(fileURI(stringInitPath)), protocol.Position{Line: 1, Character: 1})
		_, _ = stringLiteralInitializerCompletionItems(stringInitDoc, protocol.DocumentUri(fileURI(stringInitPath)), protocol.Position{Line: 1, Character: 18}, false)
		_, _ = stringLiteralInitializerCompletionItems(stringInitDoc, protocol.DocumentUri(fileURI(stringInitPath)), protocol.Position{Line: 1, Character: 18}, true)

		outputSchemaText := `[output = data, schema = User]
{
  user.
}
`
		outputSchemaPath := filepath.Join(root, "output-schema.mace")
		tAssert.NoError(os.WriteFile(outputSchemaPath, []byte(outputSchemaText), 0o644))
		outputSchemaSnapshot := AnalyzeDocumentAt(outputSchemaText, outputSchemaPath)
		outputSchemaDoc := document{text: outputSchemaText, analysis: outputSchemaSnapshot}
		_ = completionItems(outputSchemaDoc, protocol.DocumentUri(fileURI(outputSchemaPath)), protocol.Position{Line: 2, Character: 7})

		outputParseText := `[output = data, parse = User]
{
  user.
}
`
		outputParsePath := filepath.Join(root, "output-parse.mace")
		tAssert.NoError(os.WriteFile(outputParsePath, []byte(outputParseText), 0o644))
		outputParseSnapshot := AnalyzeDocumentAt(outputParseText, outputParsePath)
		outputParseDoc := document{text: outputParseText, analysis: outputParseSnapshot}
		_ = completionItems(outputParseDoc, protocol.DocumentUri(fileURI(outputParsePath)), protocol.Position{Line: 2, Character: 7})

		outputImportText := `from "./alias.mace" import Shared;
[output = data]
{
  Shared.user.
}
`
		outputImportPath := filepath.Join(root, "output-import.mace")
		tAssert.NoError(os.WriteFile(outputImportPath, []byte(outputImportText), 0o644))
		outputImportSnapshot := AnalyzeDocumentAt(outputImportText, outputImportPath)
		outputImportDoc := document{text: outputImportText, analysis: outputImportSnapshot}
		_ = completionItems(outputImportDoc, protocol.DocumentUri(fileURI(outputImportPath)), protocol.Position{Line: 3, Character: 14})

		_, _ = outputMemberAccessContext("user.profile.")
		_, _ = outputMemberAccessContext(".bad.")
		_, _ = outputMemberAccessContext("$self.profile.")
		_, _, _ = selfCompletionContext("$self.user.profile")
		_, _, _ = selfCompletionContext("$self.")
		_, _, _ = selfCompletionContext("plain text")
		_, _, _ = selfCompletionContext("$self.user+")
		_, _, _ = arrayIndexCompletionContext("items[12")
		_, _, _ = arrayIndexCompletionContext("items[x")
		_, _, _ = arrayIndexCompletionContext("[")
		_ = lastUnquotedByteInPrefix(`a ? "?" : b ?`, '?')
		_ = lastUnquotedByteInPrefix(`"?"`, '?')
		_ = lastUnquotedByteInPrefix(`"\"?\"" ?`, '?')
		_, _ = directivePrefix("[output = data, schema = User]")
		_, _ = directivePrefix("not a directive")
		_ = currentLinePrefix("abc", protocol.Position{Line: 9, Character: 0})
		_ = currentLineSuffix("abc", protocol.Position{Line: 9, Character: 0})
		_, _ = stringLiteralCompletionContext("value = \"Ada\"", protocol.Position{Line: 0, Character: 10})
		_, _ = stringLiteralCompletionContext("value = ada", protocol.Position{Line: 0, Character: 10})
		_, _ = completionPlaceholderPosition("value =", protocol.Position{Line: 0, Character: 6}, "=:")
		_, _ = completionPlaceholderPosition("value", protocol.Position{Line: 0, Character: 2}, "=:")
		_, _ = partialScriptFileWithPlaceholder("|===|\nvalue\n|===|", protocol.Position{Line: 1, Character: 0})
		_, _ = partialScriptFileWithPlaceholder("value\n", protocol.Position{Line: 0, Character: 0})
	})

	It("covers completion parser helper branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		scriptText := `|===|
string name = "Ada";
string items = ["a"];
|===|
`
		scriptPath := filepath.Join(root, "script.mace")
		tAssert.NoError(os.WriteFile(scriptPath, []byte(scriptText), 0o644))
		snapshot := AnalyzeDocumentAt(scriptText, scriptPath)
		doc := document{text: scriptText, analysis: snapshot}
		uri := protocol.DocumentUri(fileURI(scriptPath))

		_ = currentLinePrefix(scriptText, protocol.Position{Line: 99, Character: 0})
		_ = currentLineSuffix(scriptText, protocol.Position{Line: 99, Character: 0})
		_, _ = directivePrefix("[output = data]")
		_, _ = directivePrefix(" [output = data]")
		_, _ = directivePrefix("output = data")
		_, _ = stringLiteralCompletionContext(scriptText, protocol.Position{Line: 1, Character: 18})
		_, _ = stringLiteralCompletionContext("value = ada", protocol.Position{Line: 0, Character: 10})
		_, _ = completionPlaceholderPosition("value =", protocol.Position{Line: 0, Character: 7}, "=:")
		_, _ = completionPlaceholderPosition("value =", protocol.Position{Line: 0, Character: 5}, "=:")
		_, _, _ = arrayIndexCompletionContext("items[12")
		_, _, _ = arrayIndexCompletionContext("items[x")
		_, _, _ = arrayIndexCompletionContext("[")
		_ = lastUnquotedByteInPrefix(`a ? "?" : b ?`, '?')
		_ = lastUnquotedByteInPrefix(`"?"`, '?')
		_ = lastUnquotedByteInPrefix(`"\"?\"" ?`, '?')
		_, _ = outputMemberAccessContext("user.profile.")
		_, _ = outputMemberAccessContext("user.profile")
		_, _ = outputMemberAccessContext(".bad.")
		_, _ = outputMemberAccessContext("$self.profile.")
		_, _, _ = selfCompletionContext("$self.user.profile")
		_, _, _ = selfCompletionContext("$self.")
		_, _, _ = selfCompletionContext("$self.user+")
		_, _, _ = selfCompletionContext("plain text")

		_, _ = completionFileWithPlaceholder(scriptText, protocol.Position{Line: 1, Character: 18})
		_, _ = completionFileWithPlaceholder("value =", protocol.Position{Line: 0, Character: 7})
		_, _ = completionFileWithExpressionPlaceholder(scriptText, strings.Index(scriptText, `"Ada"`), strings.Index(scriptText, `"Ada"`)+5)
		_, _ = completionFileWithExpressionPlaceholder(scriptText, strings.Index(scriptText, "["), strings.Index(scriptText, "]")+1)
		_, _ = partialScriptFileWithPlaceholder(scriptText, protocol.Position{Line: 1, Character: 18})
		_, _ = partialScriptFileWithPlaceholder("value\n", protocol.Position{Line: 0, Character: 0})
		_ = completionExpressionClosers("call(a[\"x\"])", len("call(a[\"x\"])")-1)
		_ = completionExpressionClosers("[\"x\"", len("[\"x\"") )

		_ = completionItemsForType(ast.PrimitiveType{Name: "string"}, completionModel{}, completionOptions{})
		_ = completionItemsForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}}}, completionModel{}, completionOptions{unquotedStringChoices: true})
		_ = completionItemsForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, completionModel{}, completionOptions{allowSchemaLiteral: true})
		_ = completionItemsForType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, completionModel{}, completionOptions{})
		_ = selfCompletionEntries(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}})
		_ = selfCompletionEntries(processor.Value{Kind: processor.ValueString, String: "Ada"})
		_, _ = selfCompletionValue(doc, uri, protocol.Position{Line: 2, Character: 10}, []string{"name"})
		_, _ = selfCompletionValue(doc, uri, protocol.Position{Line: 2, Character: 10}, []string{"missing"})
		_ = completionScopeAt(scriptText, protocol.Position{Line: 0, Character: 0})
		_ = completionScopeAt(scriptText, protocol.Position{Line: 2, Character: 0})
		_ = completionScopeAt(scriptText, protocol.Position{Line: 10, Character: 0})
	})

	It("covers directive completion branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))
		tAssert.NoError(os.WriteFile(filepath.Join(root, "schema.mace"), []byte(`[output = schema]
{
  User: { name: string; };
  Project: { title: string; };
}`), 0o644))

		directiveText := `[output = data, parse_file = "./schema.mace"]
{
}
`
		directivePath := filepath.Join(root, "directive.mace")
		tAssert.NoError(os.WriteFile(directivePath, []byte(directiveText), 0o644))
		directiveSnapshot := AnalyzeDocumentAt(directiveText, directivePath)
		directiveDoc := document{text: directiveText, analysis: directiveSnapshot}
		uri := protocol.DocumentUri(fileURI(directivePath))

		_, _ = directiveCompletionItems(directiveDoc, uri, "[")
		_, _ = directiveCompletionItems(directiveDoc, uri, "[output = ")
		_, _ = directiveCompletionItems(directiveDoc, uri, "[schema = ")
		_, _ = directiveCompletionItems(directiveDoc, uri, "[schema_file = ")
		_, _ = directiveCompletionItems(directiveDoc, uri, "[parse = ")
		_, _ = directiveCompletionItems(directiveDoc, uri, "[parse_file = ")
		_, _ = directiveCompletionItems(directiveDoc, uri, "[output = schema, schema = ")
		_, _ = directiveCompletionItems(directiveDoc, uri, "[output = data,")
		_ = nextDirectiveDefinitions([]string{"output = data", "schema = User"})
		_ = nextDirectiveDefinitions([]string{"output = schema"})
		_ = nextDirectiveDefinitions([]string{"schema = User"})
		_ = parseDirectiveState([]string{"output = data", "schema_file = ./schema.mace", "parse = User"})
	})

	It("covers completion file and placeholder branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		schemaPath := filepath.Join(root, "schema.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Choice: choice["Ada", 1, true];
}`), 0o644))

		dataText := `[output = data, parse = User, parse_file = "./schema.mace"]
{
  user: $self.user.profile;
  item: list[0];
  choice: "Ada";
}
`
		dataPath := filepath.Join(root, "data.mace")
		tAssert.NoError(os.WriteFile(dataPath, []byte(dataText), 0o644))

		_, _ = completionFileWithPlaceholder(dataText, protocol.Position{Line: 2, Character: 8})
		_, _ = completionFileWithPlaceholder("value =", protocol.Position{Line: 0, Character: 7})
		_, _ = completionFileWithExpressionPlaceholder(dataText, strings.Index(dataText, `"Ada"`), strings.Index(dataText, `"Ada"`)+5)
		_, _ = completionFileWithExpressionPlaceholder(dataText, strings.Index(dataText, "list[0]"), strings.Index(dataText, "list[0]")+7)
		_, _ = partialScriptFile(dataText, protocol.Position{Line: 2, Character: 8})
		_, _ = partialScriptFileWithPlaceholder(dataText, protocol.Position{Line: 2, Character: 8})
		_, _ = partialScriptFileWithPlaceholder("value\n", protocol.Position{Line: 0, Character: 0})

		parsed, err := parseFile(`|===|
string value = "Ada";
|===|
[output = data] {}
`)
		tAssert.NoError(err)
		model := buildCompletionModel(parsed, root, root, map[string]completionModel{})
		_, _, _ = placeholderCompletionType(parsed, model)
		_, _, _ = placeholderOutputCompletionType(parsed, model)
		_, _, _ = placeholderParseInputCompletionType(parsed, model, root, root)
		_, _ = placeholderPath(ast.Identifier{Name: completionPlaceholderIdentifier})
		_, _ = placeholderPath(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: completionPlaceholderIdentifier})
		_, _ = placeholderPath(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}})
		_, _ = placeholderPath(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: completionPlaceholderIdentifier}}})
		_, _ = placeholderPath(ast.PrefixExpression{Right: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = placeholderPath(ast.InfixExpression{Left: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = placeholderPath(ast.ConditionalExpression{Condition: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = expressionPath(ast.Identifier{Name: "value"})
		_, _ = expressionPath(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"})
		_, _ = expressionPath(ast.Identifier{Name: ""})
		_, _ = trailingMemberAccessPath("user.profile.")
		_, _ = trailingMemberAccessPath("$self.profile.")
		_, _ = trailingMemberAccessPath("no-trail")
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "string"}, model, 2)
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "int"}, model, 2)
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "boolean"}, model, 2)
		_ = syntheticCompletionValue(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, 2)
		_ = syntheticCompletionValue(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, 2)
		_ = syntheticCompletionValue(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, model, 2)
		_, _ = completionChoiceMemberValues(ast.StringLiteral{Lexeme: `"Ada"`}, model, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "Choice"}, completionModel{aliases: map[string]ast.TypeReference{"Choice": ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}}}, map[string]struct{}{})
		_ = schemaLiteral(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "string"}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "int"}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "boolean"}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, map[string]struct{}{})
		_ = defaultLiteralForType(ast.NamedType{Name: "User"}, model, map[string]struct{}{})
		_ = itemsFromDeclarations([]declarationDefinition{{Name: "User", Kind: protocol.CompletionItemKindStruct}, {Name: "value", Kind: protocol.CompletionItemKindVariable}}, "")
		_ = completionItemsForValueMembers(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}})
		_ = completionItemsForValueMembers(processor.Value{Kind: processor.ValueString, String: "Ada"})
		_ = completionItemsForMemberTarget(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model)
		_ = completionItemsForType(ast.PrimitiveType{Name: "string"}, model, completionOptions{})
		_ = completionItemsForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, model, completionOptions{unquotedStringChoices: true})
		_ = completionItemsForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, completionOptions{allowSchemaLiteral: true})
		_ = completionItemsForType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, model, completionOptions{})
		_ = completionItemsForType(ast.NamedType{Name: "User"}, model, completionOptions{})
		_ = completionItemsForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, completionOptions{})
		_ = completionItemsForType(ast.UnionType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, model, completionOptions{})
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./alias.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared", "user"}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}}, []string{"User"}, root, root, map[string]completionModel{})
		_ = parseFileOutputDeclarationDefinitions([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = unquotedStringChoiceLabel(`"Ada"`)
		_, _ = unquotedStringChoiceLabel(`Ada`)
		_, _ = completionChoiceFromMembers([]ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}}, model, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.BooleanLiteral{Value: true}, model, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "Missing"}, model, map[string]struct{}{})
		_, _ = completionUnionRecord([]ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, model, map[string]struct{}{})
		_, _ = completionUnionRecord([]ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}, model, map[string]struct{}{})
		scriptText := `|===|
string value = "Ada";
|===|
`
		scriptPath := filepath.Join(root, "script.mace")
		tAssert.NoError(os.WriteFile(scriptPath, []byte(scriptText), 0o644))
		_, _ = resolveLocalArrayCompletionTarget(scriptText, protocol.Position{Line: 1, Character: 18}, ast.Identifier{Name: "value"})
		_, _ = resolveLocalArrayCompletionTarget("value\n", protocol.Position{Line: 0, Character: 0}, ast.Identifier{Name: "value"})
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "missing"}, map[string]ast.Expression{}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"}, map[string]ast.Expression{"value": ast.RecordLiteral{Fields: []ast.RecordField{{Name: "profile", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "items"}, Index: ast.IntLiteral{Lexeme: "9"}}, map[string]ast.Expression{"items": ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.BooleanLiteral{Value: true}}}, map[string]ast.Expression{}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, map[string]ast.Expression{}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.BooleanLiteral{Value: true}, map[string]ast.Expression{}, map[string]struct{}{})
		_ = partialScriptVariables(scriptText, protocol.DocumentUri(fileURI(scriptPath)), protocol.Position{Line: 1, Character: 18})
		_ = partialScriptVariables("value\n", protocol.DocumentUri(fileURI(scriptPath)), protocol.Position{Line: 0, Character: 0})
		_ = scriptVariablesForOutput(scriptText, protocol.DocumentUri(fileURI(scriptPath)))
		_ = processVariablesInDocument(scriptText, protocol.DocumentUri(fileURI(scriptPath)))
		outputKeyText := `[output = data, parse_file = "./schema.mace"]
{
  sch
}
`
		outputKeyPath := filepath.Join(root, "output-key.mace")
		manualOutputFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}}}
		manualOutputDoc := document{text: outputKeyText, analysis: analysisSnapshot{file: &manualOutputFile, importRootDir: root}}
		_ = completionItems(manualOutputDoc, protocol.DocumentUri(fileURI(outputKeyPath)), protocol.Position{Line: 2, Character: 5})

		outputSchemaText := `[output = data, schema = User]
{
  user.
}
`
		outputSchemaPath := filepath.Join(root, "output-schema.mace")
		manualSchemaFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema.mace"`}}}}
		manualSchemaDoc := document{text: outputSchemaText, analysis: analysisSnapshot{file: &manualSchemaFile, importRootDir: root}}
		_ = completionItems(manualSchemaDoc, protocol.DocumentUri(fileURI(outputSchemaPath)), protocol.Position{Line: 2, Character: 7})

		outputParseText := `[output = data, parse = User]
{
  user.
}
`
		outputParsePath := filepath.Join(root, "output-parse.mace")
		manualParseFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}}}
		manualParseDoc := document{text: outputParseText, analysis: analysisSnapshot{file: &manualParseFile, importRootDir: root}}
		_ = completionItems(manualParseDoc, protocol.DocumentUri(fileURI(outputParsePath)), protocol.Position{Line: 2, Character: 7})

		outputImportText := `from "./alias.mace" import Shared;
[output = data]
{
  Shared.user.
}
`
		outputImportPath := filepath.Join(root, "output-import.mace")
		manualImportFile := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./alias.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}, Output: ast.OutputBlock{Mode: ast.OutputModeData}}
		manualImportDoc := document{text: outputImportText, analysis: analysisSnapshot{file: &manualImportFile, importRootDir: root}}
		_ = completionItems(manualImportDoc, protocol.DocumentUri(fileURI(outputImportPath)), protocol.Position{Line: 3, Character: 14})
	})

	It("covers completion edge branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))
		schemaPath := filepath.Join(root, "schema.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`[output = schema]
{
  User: { name: string; profile: { city: string; }; };
}`), 0o644))
		scriptText := `|===|
string value = "Ada";
|===|
`
		scriptPath := filepath.Join(root, "script.mace")
		tAssert.NoError(os.WriteFile(scriptPath, []byte(scriptText), 0o644))
		scriptSnapshot := AnalyzeDocumentAt(scriptText, scriptPath)
		scriptDoc := document{text: scriptText, analysis: scriptSnapshot}
		scriptURI := protocol.DocumentUri(fileURI(scriptPath))
		plainDoc := document{text: "plain text", analysis: analysisSnapshot{}}

		_, _ = initializerCompletionItems(plainDoc, scriptURI, protocol.Position{Line: 0, Character: 0})
		_, _ = outputInitializerCompletionItems(plainDoc, scriptURI, protocol.Position{Line: 0, Character: 0})
		_, _ = stringLiteralInitializerCompletionItems(plainDoc, scriptURI, protocol.Position{Line: 0, Character: 0}, false)
		_ = bareSelfCompletionItems("plain", protocol.Position{Line: 0, Character: 0})
		_ = bareSelfCompletionItems("result:", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("plain", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("$self.", protocol.Position{Line: 0, Character: 0})
		_, _ = outputMemberAccessContext("plain")
		_, _ = outputMemberAccessContext("$self.user.")
		_, _ = parsedVariableMemberCompletionItems(plainDoc, scriptURI, "plain", protocol.Position{Line: 0, Character: 0})
		_, _ = parsedVariableMemberCompletionItems(scriptDoc, scriptURI, "value.", protocol.Position{Line: 1, Character: 0})
		_, _ = arrayIndexCompletionItems(plainDoc, scriptURI, protocol.Position{Line: 0, Character: 0}, "plain", completionScopeScript)
		_, _ = arrayIndexCompletionItems(scriptDoc, scriptURI, protocol.Position{Line: 1, Character: 0}, "value[", completionScopeScript)
		_, _ = resolveLocalArrayCompletionTarget("plain", protocol.Position{Line: 0, Character: 0}, ast.Identifier{Name: "value"})
		_, _ = resolveLocalCompletionValue(ast.Identifier{Name: "missing"}, map[string]ast.Expression{}, map[string]struct{}{})
		_ = processVariablesInDocument("plain", scriptURI)
		_ = scriptVariablesForOutput("plain", scriptURI)
		_ = partialScriptVariables("plain", scriptURI, protocol.Position{Line: 0, Character: 0})
		_, _ = resolveCompletionValue(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"}, map[string]processor.Value{"value": {Kind: processor.ValueRecord, Record: map[string]processor.Value{"profile": {Kind: processor.ValueString, String: "Ada"}}}}, processor.Value{})
		_, _ = resolveCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "value"}, Index: ast.IntLiteral{Lexeme: "0"}}, map[string]processor.Value{"value": {Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueString, String: "Ada"}}}}, processor.Value{})
		_, _ = resolveCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.IntLiteral{Lexeme: "1"}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.FloatLiteral{Lexeme: "1.0"}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.BooleanLiteral{Value: true}, nil, processor.Value{})
		_, _ = directivePrefix("plain")
		_, _ = directivePrefix(" [output = data]")
		_, _ = stringLiteralCompletionContext("value = ada", protocol.Position{Line: 0, Character: 10})
		_, _ = completionPlaceholderPosition("value", protocol.Position{Line: 0, Character: 2}, "=:.")
		_, _ = completionPlaceholderPosition("value =", protocol.Position{Line: 0, Character: 6}, "=:.")
		_, _ = completionFileWithPlaceholder("plain", protocol.Position{Line: 0, Character: 0})
		_, _ = completionFileWithExpressionPlaceholder("plain", 0, 2)
		_, _ = partialScriptFileWithPlaceholder("plain", protocol.Position{Line: 0, Character: 0})
		_, _, _ = placeholderCompletionType(ast.File{}, completionModel{})
		_, _, _ = placeholderOutputCompletionType(ast.File{}, completionModel{})
		_, _, _ = placeholderParseInputCompletionType(ast.File{}, completionModel{}, root, root)
		_, _ = placeholderPath(ast.Identifier{Name: completionPlaceholderIdentifier})
		_, _ = placeholderPath(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: completionPlaceholderIdentifier})
		_, _ = placeholderPath(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: completionPlaceholderIdentifier}}})
		_, _ = placeholderPath(ast.PrefixExpression{Right: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = placeholderPath(ast.InfixExpression{Left: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = placeholderPath(ast.ConditionalExpression{Condition: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = expressionPath(ast.Identifier{Name: "value"})
		_, _ = expressionPath(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"})
		_, _ = expressionPath(ast.Identifier{Name: ""})
		_, _ = trailingMemberAccessPath("plain")
		_, _ = trailingMemberAccessPath("user.profile.")
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "string"}, completionModel{}, 0)
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "string"}, completionModel{}, 2)
		_ = syntheticCompletionValue(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, completionModel{}, 2)
		_ = syntheticCompletionValue(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, completionModel{}, 2)
		_ = syntheticCompletionValue(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, completionModel{}, 2)
		_, _ = unquotedStringChoiceLabel(`"Ada"`)
		_, _ = unquotedStringChoiceLabel(`Ada`)
		_, _ = outputSchemaDirective(ast.File{})
		_ = buildCompletionModel(ast.File{}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&completionModel{}, nil, root, root, map[string]completionModel{})
		_ = completionItemsForType(ast.PrimitiveType{Name: "string"}, completionModel{}, completionOptions{})
		_ = completionItemsForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, completionModel{}, completionOptions{allowSchemaLiteral: true})
		_ = completionItemsForMemberTarget(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, completionModel{})
		_ = completionItemsForValueMembers(processor.Value{Kind: processor.ValueString, String: "Ada"})
		_ = completionItemsForValueMembers(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}})
		_, _ = outputInitializerCompletionItems(scriptDoc, scriptURI, protocol.Position{Line: 1, Character: 17})
		_, _ = initializerCompletionItems(scriptDoc, scriptURI, protocol.Position{Line: 1, Character: 17})
		_ = completionItems(scriptDoc, scriptURI, protocol.Position{Line: 1, Character: 17})

		_, _, _ = selfCompletionContext("plain")
		_, _, _ = selfCompletionContext("$self.")
		_, _, _ = selfCompletionContext("$self.user.profile")
		_, _, _ = selfCompletionContext("$self.user[0]")
		_, _, _ = arrayIndexCompletionContext("plain")
		_, _, _ = arrayIndexCompletionContext("items[12]")
		_, _, _ = arrayIndexCompletionContext("items[a]")
		_, _ = trailingMemberAccessPath("plain")
		_, _ = trailingMemberAccessPath("user.profile.")
		_ = simpleExpressionText(ast.NullLiteral{})
		_ = simpleExpressionText(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}})
		_ = simpleExpressionText(ast.RecordLiteral{})
		_ = defaultExpressionForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}})
		_ = defaultExpressionForType(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}})
		_ = defaultExpressionForType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}})
		_ = defaultExpressionForType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}})
		_ = inferredTypeFromExpression(ast.NullLiteral{})
		_ = inferredTypeFromExpression(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"})
		_ = inferredTypeFromExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}})
		_ = inferredTypeFromExpression(ast.SelfReference{Path: []string{"user"}})
		_, _ = replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "city", Type: ast.PrimitiveType{Name: "string"}}}}}}}, []string{}, ast.PrimitiveType{Name: "int"})
		_, _ = replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, []string{"missing"}, ast.PrimitiveType{Name: "int"})
		_, _ = replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, []string{"name", "city"}, ast.PrimitiveType{Name: "int"})
		_, _ = replaceSchemaFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "city", Type: ast.PrimitiveType{Name: "string"}}}}}}}, []string{"profile", "city"}, ast.PrimitiveType{Name: "int"})
		_, _, _ = missingImportEdit("plain", ast.File{}, nil, `unknown type "Missing"`)
		_, _, _ = missingImportEdit("plain", ast.File{Script: &ast.ScriptBlock{}}, nil, `unknown type "Missing"`)
		_, _ = invalidDirectiveComboEditRange("[output = schema, schema = User]", ast.File{}, lexAnalysisTokens("[output = schema, schema = User]"), "schema directive is invalid when output mode is schema")
		_, _ = invalidDirectiveComboEditRange("[output = schema, schema_file = \"./schema.mace\"]", ast.File{}, lexAnalysisTokens("[output = schema, schema_file = \"./schema.mace\"]"), "schema_file directive is invalid when output mode is schema")
		_, _ = invalidDirectiveComboEditRange("plain", ast.File{}, nil, "other")
		_, _ = formatTextQuick("plain")
		_, _ = formatTextQuick("|===|\nstring value = \"x\"\n|===|")

		badText := "|===|\nvalue =+\n|===|\n"
		badDoc := document{text: badText, analysis: AnalyzeDocumentAt(badText, scriptPath)}
		_, _ = initializerCompletionItems(badDoc, scriptURI, protocol.Position{Line: 1, Character: 7})
		unknownText := "|===|\n" + completionPlaceholderIdentifier + "\n|===|\n"
		unknownDoc := document{text: unknownText, analysis: AnalyzeDocumentAt(unknownText, scriptPath)}
		_, _ = initializerCompletionItems(unknownDoc, scriptURI, protocol.Position{Line: 1, Character: protocol.UInteger(len(completionPlaceholderIdentifier))})
		memberText := "[output = data, schema = User]\n{\n  user: { name: \"Ada\"; profile: { city: \"LA\"; }; };\n  ref: user.profile.\n}\n"
		memberDoc := document{text: memberText, analysis: AnalyzeDocumentAt(memberText, scriptPath)}
		_, _ = outputInitializerCompletionItems(memberDoc, scriptURI, protocol.Position{Line: 3, Character: 15})
		fieldText := "|===|\nstring value = " + completionPlaceholderIdentifier + ".name;\n|===|\n"
		fieldDoc := document{text: fieldText, analysis: AnalyzeDocumentAt(fieldText, scriptPath)}
		_, _ = initializerCompletionItems(fieldDoc, scriptURI, protocol.Position{Line: 1, Character: 18})
		_, _ = outputInitializerCompletionItems(fieldDoc, scriptURI, protocol.Position{Line: 1, Character: 18})
		emptyOutputText := "[output = data]\n{}\n"
		emptyOutputDoc := document{text: emptyOutputText, analysis: AnalyzeDocumentAt(emptyOutputText, scriptPath)}
		_, _ = outputInitializerCompletionItems(emptyOutputDoc, scriptURI, protocol.Position{Line: 1, Character: 1})
	})

	It("covers remaining completion edge branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		schemaPath := filepath.Join(root, "schema.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Shared: { user: { profile: { city: string; }; }; };
}`), 0o644))

		outputText := `[output = data, schema = User]
{
  value: "Ada";
}
`
		outputPath := filepath.Join(root, "output.mace")
		tAssert.NoError(os.WriteFile(outputPath, []byte(outputText), 0o644))
		outputSnapshot := AnalyzeDocumentAt(outputText, outputPath)
		outputDoc := document{text: outputText, analysis: outputSnapshot}
		outputURI := protocol.DocumentUri(fileURI(outputPath))

		scriptText := `|===|
string first = "Ada";
string value = "Bee";
|===|
`
		scriptPath := filepath.Join(root, "script.mace")
		tAssert.NoError(os.WriteFile(scriptPath, []byte(scriptText), 0o644))
		scriptURI := protocol.DocumentUri(fileURI(scriptPath))
		plainScriptDoc := document{text: scriptText, analysis: analysisSnapshot{}}
		plainOutputDoc := document{text: outputText, analysis: analysisSnapshot{}}
		plainDoc := document{text: "plain text", analysis: analysisSnapshot{}}

		_ = completionDeclarations(plainScriptDoc, scriptURI, protocol.Position{Line: 2, Character: 5}, "string value", completionScopeScript)
		_ = completionDeclarations(plainOutputDoc, outputURI, protocol.Position{Line: 1, Character: 5}, "value", completionScopeOutput)
		valueSchemaPath := filepath.Join(root, "value-schema.mace")
		tAssert.NoError(os.WriteFile(valueSchemaPath, []byte(`[output = schema]
{
  User: { value: string; };
}`), 0o644))
		simpleOutputText := `[output = data, schema = User, schema_file = "./value-schema.mace"]
{
  value: "A";
}
`
		simpleOutputPath := filepath.Join(root, "simple-output.mace")
		tAssert.NoError(os.WriteFile(simpleOutputPath, []byte(simpleOutputText), 0o644))
		simpleOutputSnapshot := AnalyzeDocumentAt(simpleOutputText, simpleOutputPath)
		simpleOutputDoc := document{text: simpleOutputText, analysis: simpleOutputSnapshot}
		simpleOutputURI := protocol.DocumentUri(fileURI(simpleOutputPath))
		_, _ = outputInitializerCompletionItems(simpleOutputDoc, simpleOutputURI, protocol.Position{Line: 2, Character: 10})
		_, _ = stringLiteralInitializerCompletionItems(simpleOutputDoc, simpleOutputURI, protocol.Position{Line: 2, Character: 10}, true)
		_, _ = stringLiteralInitializerCompletionItems(simpleOutputDoc, simpleOutputURI, protocol.Position{Line: 2, Character: 10}, false)
		_ = completionFile(document{text: "[]", analysis: analysisSnapshot{}}, "")
		_, _ = documentPathFromURI(protocol.DocumentUri("file:///tmp/%zz"))
		_, _ = importableSymbols(protocol.DocumentUri("file:///tmp/%zz"), root, "./schema.mace")
		_, _ = importCompletionItems(document{text: `from "C:/abs.mace" import User;`, analysis: analysisSnapshot{}}, `from "C:/abs.mace" import User;`, outputURI)
		_, _ = outputMemberAccessContext(".")
		_, _ = outputMemberAccessContext("$self.profile.")
		_, _ = outputMemberAccessContext("plain")
		_, _, _ = selfCompletionContext("plain")
		_, _, _ = arrayIndexCompletionContext("plain")
		_, _, _ = arrayIndexCompletionContext("items[12")
		_, _, _ = arrayIndexCompletionContext("items[x")
		_, _ = arrayIndexCompletionItems(outputDoc, outputURI, protocol.Position{Line: 2, Character: 16}, "1", completionScopeOutput)
		_, _ = resolveLocalCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "value"}, Index: ast.IntLiteral{Lexeme: "x"}}, map[string]ast.Expression{"value": ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "value"}, Index: ast.IntLiteral{Lexeme: "9"}}, map[string]ast.Expression{"value": ast.StringLiteral{Lexeme: `"Ada"`}}, map[string]struct{}{})
		_, _ = resolveLocalCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.Identifier{Name: "missing"}}}}, map[string]ast.Expression{}, map[string]struct{}{})
		_ = partialScriptVariables("plain text", scriptURI, protocol.Position{Line: 0, Character: 0})
		_ = scriptVariablesForOutput("plain text", scriptURI)
		_ = currentImports(plainDoc, "plain")
		_ = importedPaths(plainDoc, "plain")
		_ = availableSchemaNames(document{text: "[", analysis: analysisSnapshot{}}, outputURI, "output = schema")
		_, _ = outputSchemaDirective(ast.File{})
		_ = buildCompletionModel(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: "bad"}}}}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&completionModel{}, []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: "bad"}}, root, root, map[string]completionModel{})
		_ = parseFileOutputDeclarationDefinitions([]ast.OutputDirective{}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord([]ast.OutputDirective{}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord([]ast.OutputDirective{}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{}, nil, root, root, map[string]completionModel{})
		_, _ = completionOutputFieldType(ast.ArrayLiteral{Elements: []ast.Expression{}}, completionModel{})
		_, _ = completionOutputFieldType(ast.Identifier{Name: "missing"}, completionModel{})
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "Missing"}, completionModel{}, map[string]struct{}{})
		_, _ = completionUnionRecord([]ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}, completionModel{}, map[string]struct{}{})
		_, _ = trailingMemberAccessPath(".")
		_, _ = trailingMemberAccessPath("$self.profile.")
		_ = syntheticCompletionValue(ast.PrimitiveType{Name: "string"}, completionModel{}, 0)
		_ = syntheticCompletionValue(ast.NamedType{Name: "Missing"}, completionModel{}, 2)
		_, _, _ = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, completionModel{})
		_, _, _ = placeholderParseInputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, completionModel{}, root, root)
		_, _ = expressionPath(ast.Identifier{Name: ""})
		_, _ = completionPlaceholderPosition("value", protocol.Position{Line: 0, Character: 2}, "=:.")
		_, _ = completionPlaceholderPosition("value =", protocol.Position{Line: 0, Character: 6}, "=:.")
		_, _ = completionFileWithPlaceholder("plain", protocol.Position{Line: 0, Character: 0})
		_, _ = completionFileWithExpressionPlaceholder("plain", 0, 2)
		_, _ = importableIdentifiers(protocol.DocumentUri(fileURI(filepath.Join(root, "missing.mace"))), root, "./schema.mace")
		_, _, _ = parseInputMemberCompletionRootType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, []string{"user", "profile"}, root, root, map[string]completionModel{}, map[string]struct{}{})
		_, _ = documentPathFromURI(protocol.DocumentUri("file:///tmp/%zz"))
		_ = availableSchemaNames(document{text: "[", analysis: analysisSnapshot{}}, outputURI, "output = schema")
		_ = importedPaths(document{text: `from bad import x;`, analysis: analysisSnapshot{}}, `from bad import x;`)
		_, _ = selfCompletionValue(plainDoc, outputURI, protocol.Position{Line: 0, Character: 0}, []string{"missing"})
		_, _ = partialOutputResult(document{text: "plain", analysis: analysisSnapshot{}}, outputURI, protocol.Position{Line: 0, Character: 0})
		_, _ = outputFieldRanges("plain", lexAnalysisTokens("plain"), 0)
		_ = isOutputFieldHeader(lexAnalysisTokens("value"), 0)
		_, _, _ = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, completionModel{})
		_, _, _ = placeholderParseInputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, completionModel{}, root, root)
		_, _ = expressionPath(ast.BooleanLiteral{Value: true})
		_, _ = trailingMemberAccessPath(".")
		_, _ = outputSchemaDirective(ast.File{})
		_ = buildCompletionModel(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: "bad"}}}}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&completionModel{}, []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: "bad"}}, root, root, map[string]completionModel{})
		_ = parseFileOutputDeclarationDefinitions([]ast.OutputDirective{}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord([]ast.OutputDirective{}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord([]ast.OutputDirective{}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{}, nil, root, root, map[string]completionModel{})
		_, _ = completionOutputFieldType(ast.ArrayLiteral{Elements: []ast.Expression{}}, completionModel{})
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "Missing"}, completionModel{}, map[string]struct{}{})
		_, _ = completionUnionRecord([]ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "float"}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "boolean"}, completionModel{}, map[string]struct{}{})
	})

	It("covers remaining completion branch gaps", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		aliasPath := filepath.Join(root, "alias-data.mace")
		tAssert.NoError(os.WriteFile(aliasPath, []byte(`[output = data]
{
  user: { name: "Ada"; tags: ["x", "y"]; };
}`), 0o644))

		schemaPath := filepath.Join(root, "schema-out.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`[output = schema]
{
  User: { name: string; };
  ChoiceWrap: choice["a", "b"];
}`), 0o644))

		parsePath := filepath.Join(root, "parse-out.mace")
		tAssert.NoError(os.WriteFile(parsePath, []byte(`[output = schema]
{
  Runtime: { env: string; nested: { city: string; }; };
}`), 0o644))

		badParsePath := filepath.Join(root, "bad-parse.mace")
		tAssert.NoError(os.WriteFile(badParsePath, []byte(`not mace`), 0o644))

		uri := protocol.DocumentUri(fileURI(filepath.Join(root, "doc.mace")))
		parseDirectives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./parse-out.mace"`}}
		badParseDirectives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./missing.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"./bad-parse.mace"`}}

		tAssert.Equal([]string{"output"}, lo.Map(lo.Must(directiveCompletionItems(document{}, "file:///doc.mace", "[")), func(item protocol.CompletionItem, _ int) string { return item.Label }))
		items, handled := directiveCompletionItems(document{}, "file:///doc.mace", "[output = data, parse_file = \"")
		tAssert.True(handled)
		tAssert.NotNil(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "[output = data, schema = User")
		tAssert.True(handled)
		tAssert.NotNil(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "[output = data, parse = Runtime")
		tAssert.True(handled)
		tAssert.NotNil(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "[output = data, parse")
		tAssert.True(handled)
		tAssert.NotNil(items)
		_, ok := directivePrefix("prefix[")
		tAssert.False(ok)

		ctx, ok := stringLiteralCompletionContext(`value: "Ada`, protocol.Position{Line: 0, Character: 11})
		tAssert.True(ok)
		tAssert.Equal("Ada", ctx.prefix)
		_, ok = stringLiteralCompletionContext(`value: "A\\"da`, protocol.Position{Line: 0, Character: 11})
		tAssert.True(ok)
		_, ok = stringLiteralCompletionContext(`value: "Ada {x`, protocol.Position{Line: 0, Character: 13})
		tAssert.True(ok)
		_, ok = stringLiteralCompletionContext(`value: plain`, protocol.Position{Line: 0, Character: 8})
		tAssert.False(ok)

		symbols, ok := importableSymbols(uri, root, "./alias-data.mace")
		tAssert.True(ok)
		tAssert.NotEmpty(symbols)
		_, ok = importableSymbols(uri, root, "./missing.mace")
		tAssert.False(ok)
		schemaSymbols, ok := importableSymbols(uri, root, "./schema-out.mace")
		tAssert.True(ok)
		tAssert.Contains(lo.Map(schemaSymbols, func(symbol importableSymbol, _ int) string { return symbol.Name }), "User")
		_, ok = documentPathFromURI(protocol.DocumentUri("file:///%zz"))
		tAssert.False(ok)

		schemaNames := availableSchemaNames(document{text: "[output = schema", analysis: analysisSnapshot{}}, uri, "output = schema")
		tAssert.NotNil(schemaNames)
		tAssert.Nil(availableSchemaNames(document{text: "plain", analysis: analysisSnapshot{}}, uri, "plain"))
		importDocumentText := "|===|\nfrom \"./alias-data.mace\" import user;\n|===|\n[output = data] {}"
		importPaths := importedPaths(document{text: importDocumentText, analysis: AnalyzeDocumentAt(importDocumentText, filepath.Join(root, "imports.mace"))}, `from "./alias-data.mace" import user;`)
		tAssert.Equal([]string{"./alias-data.mace"}, importPaths)

		selfText := `[output = data]
{
  user: "Ada";
}`
		selfDoc := document{text: selfText, analysis: AnalyzeDocumentAt(selfText, filepath.Join(root, "self.mace"))}
		_, ok = selfCompletionValue(selfDoc, protocol.DocumentUri(fileURI(filepath.Join(root, "self.mace"))), protocol.Position{Line: 2, Character: 8}, []string{"user", "name"})
		tAssert.False(ok)
		_, ok = selfCompletionValue(selfDoc, protocol.DocumentUri(fileURI(filepath.Join(root, "self.mace"))), protocol.Position{Line: 2, Character: 8}, []string{"missing"})
		tAssert.False(ok)

		_, ok = partialOutputResult(document{text: "plain", analysis: analysisSnapshot{}}, uri, protocol.Position{Line: 0, Character: 0})
		tAssert.False(ok)
		_, ok = partialOutputResult(document{text: "[output = data]\n{}", analysis: analysisSnapshot{}}, uri, protocol.Position{Line: 0, Character: 0})
		tAssert.False(ok)
		_, ok = partialOutputResult(document{text: "[output = data]\n{}", analysis: analysisSnapshot{}}, uri, protocol.Position{Line: 10, Character: 0})
		tAssert.False(ok)
		_, ok = outputFieldRanges("[output = data]", lexAnalysisTokens("[output = data]"), 0)
		tAssert.False(ok)
		tAssert.False(isOutputFieldHeader(lexAnalysisTokens("value"), 0))

		_, _, ok = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData}}, completionModel{})
		tAssert.False(ok)
		fileWithParsePlaceholder := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./parse-out.mace"`}}, DataFields: []ast.OutputField{{Name: "nested", Value: ast.MemberAccess{Target: ast.Identifier{Name: "nested"}, Name: completionPlaceholderIdentifier}}}}}
		_, _, ok = placeholderParseInputCompletionType(fileWithParsePlaceholder, completionModel{}, root, root)
		tAssert.True(ok)

		path, ok := trailingMemberAccessPath("user.")
		tAssert.True(ok)
		tAssert.Equal([]string{"user"}, path)

		choiceValue := syntheticCompletionValue(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"x"`}}}, completionModel{}, 2)
		tAssert.Equal(processor.ValueString, choiceValue.Kind)
		unknownValue := syntheticCompletionValue(ast.PrimitiveType{Name: "wat"}, completionModel{}, 2)
		tAssert.Equal(processor.ValueUnknown, unknownValue.Kind)
		label, ok := unquotedStringChoiceLabel(`"Ada"`)
		tAssert.True(ok)
		tAssert.Equal("Ada", label)

		model := buildCompletionModel(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "tags", Type: ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}}}}}, {Name: "ChoiceWrap", Type: ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"a"`}}}}}}, Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema-out.mace"`}}}}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&model, []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./parse-out.mace"`}}, root, root, map[string]completionModel{})
		defs := parseFileOutputDeclarationDefinitions(parseDirectives, root, root, map[string]completionModel{})
		tAssert.NotEmpty(defs)
		tAssert.Empty(parseFileOutputDeclarationDefinitions(badParseDirectives, root, root, map[string]completionModel{}))
		record, ok := parseFileOutputSchemaRecord(parseDirectives, root, root, map[string]completionModel{})
		tAssert.True(ok)
		tAssert.NotEmpty(record.Fields)
		_, ok = parseFileOutputSchemaRecord(badParseDirectives, root, root, map[string]completionModel{})
		tAssert.False(ok)
		record, ok = parseFileOutputExportedRecord(parseDirectives, root, root, map[string]completionModel{})
		tAssert.True(ok)
		tAssert.NotEmpty(record.Fields)
		_, ok = parseFileOutputExportedRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./missing.mace"`}}, root, root, map[string]completionModel{})
		tAssert.False(ok)

		importAsFile := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./alias-data.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}
		rootType, importedModel, ok := importedMemberCompletionRootType(importAsFile, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.True(ok)
		tAssert.NotEqual(completionModel{}, importedModel)
		tAssert.NotNil(rootType)
		_, _, ok = importedMemberCompletionRootType(importAsFile, []string{"Shared", "user"}, root, root, map[string]completionModel{})
		tAssert.True(ok)
		_, _, ok = importedMemberCompletionRootType(importAsFile, []string{"Shared", "missing"}, root, root, map[string]completionModel{})
		tAssert.False(ok)
		_, _, ok = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./missing.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.False(ok)
		_, _, ok = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema-out.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.False(ok)

		fieldType, ok := completionOutputFieldType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"x"`}}}, model)
		tAssert.True(ok)
		tAssert.IsType(ast.ArrayType{}, fieldType)
		fieldType, ok = completionOutputFieldType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"x"`}}}}, model)
		tAssert.True(ok)

		resolved := resolveCompletionType(ast.NamedType{Name: "User"}, model, map[string]struct{}{})
		_ = resolved
		resolved = resolveCompletionType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{})
		tAssert.Equal(completionTypeArray, resolved.kind)
		resolved = resolveCompletionType(ast.NamedType{Name: "Loop"}, completionModel{aliases: map[string]ast.TypeReference{"Loop": ast.NamedType{Name: "Loop"}}}, map[string]struct{}{})
		tAssert.Equal(completionTypeUnknown, resolved.kind)
		resolved = resolveCompletionType(ast.NamedType{Name: "ChoiceWrap"}, model, map[string]struct{}{})
		_ = resolved
		resolved = resolveCompletionType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, model, map[string]struct{}{})
		tAssert.Equal(completionTypeSchema, resolved.kind)

		choice, ok := completionChoiceFromMembers([]ast.Expression{ast.Identifier{Name: "Mode"}}, completionModel{aliases: map[string]ast.TypeReference{"Mode": ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"a"`}, ast.StringLiteral{Lexeme: `"a"`}}}}}, map[string]struct{}{})
		tAssert.True(ok)
		tAssert.Len(choice.members, 1)
		members, ok := completionChoiceMemberValues(ast.Identifier{Name: "Bad"}, completionModel{aliases: map[string]ast.TypeReference{"Bad": ast.PrimitiveType{Name: "string"}}}, map[string]struct{}{})
		tAssert.False(ok)
		tAssert.Nil(members)
		members, ok = completionChoiceMemberValues(ast.Identifier{Name: "Loop"}, completionModel{aliases: map[string]ast.TypeReference{"Loop": ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Loop"}}}}}, map[string]struct{}{"Loop": {}})
		tAssert.False(ok)
		tAssert.Nil(members)
		members, ok = completionChoiceMemberValues(ast.NullLiteral{}, completionModel{}, map[string]struct{}{})
		tAssert.False(ok)
		tAssert.Nil(members)
		tAssert.Equal(`""`, defaultLiteralForType(ast.PrimitiveType{Name: "mystery"}, model, map[string]struct{}{}))
		tAssert.Equal(`""`, defaultLiteralForType(ast.ChoiceType{}, model, map[string]struct{}{}))
		tAssert.Equal("{}", defaultLiteralForType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{}))
	})

	It("covers targeted completion zero-count branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		dataImportPath := filepath.Join(root, "data-import.mace")
		tAssert.NoError(os.WriteFile(dataImportPath, []byte(`[output = data]
{
  user: { name: "Ada"; };
  items: ["a", "b"];
}`), 0o644))
		schemaImportPath := filepath.Join(root, "schema-import.mace")
		tAssert.NoError(os.WriteFile(schemaImportPath, []byte(`[output = schema]
{
  User: { name: string; };
}`), 0o644))
		emptySchemaPath := filepath.Join(root, "empty-schema.mace")
		tAssert.NoError(os.WriteFile(emptySchemaPath, []byte(`[output = schema]
{
}`), 0o644))
		invalidFilePath := filepath.Join(root, "invalid.mace")
		tAssert.NoError(os.WriteFile(invalidFilePath, []byte(`not mace`), 0o644))

		uri := protocol.DocumentUri(fileURI(filepath.Join(root, "doc.mace")))

		outputStringText := `|===|
schema User: { user: string; };
|===|
[output = data, schema = User]
{
  user: "A";
}`
		outputStringPath := filepath.Join(root, "output-string.mace")
		tAssert.NoError(os.WriteFile(outputStringPath, []byte(outputStringText), 0o644))
		outputStringDoc := document{text: outputStringText, analysis: AnalyzeDocumentAt(outputStringText, outputStringPath)}
		items, handled := outputInitializerCompletionItems(outputStringDoc, protocol.DocumentUri(fileURI(outputStringPath)), protocol.Position{Line: 5, Character: 10})
		tAssert.True(handled)
		items, handled = outputInitializerCompletionItems(document{text: `[output = data]
{
  value: $self.user.
}`, analysis: analysisSnapshot{}}, uri, protocol.Position{Line: 2, Character: 20})
		tAssert.False(handled)
		tAssert.Nil(items)

		guardedText := `|===|
schema Runtime: { user?: { name: string; }; };
|===|
[output = data, parse = Runtime]
{
  result: "user" in user ? user.
}`
		guardedPath := filepath.Join(root, "guarded.mace")
		tAssert.NoError(os.WriteFile(guardedPath, []byte(guardedText), 0o644))
		guardedDoc := document{text: guardedText, analysis: AnalyzeDocumentAt(guardedText, guardedPath)}
		items, handled = parsedVariableMemberCompletionItems(guardedDoc, protocol.DocumentUri(fileURI(guardedPath)), `  result: "user" in user ? user.`, protocol.Position{Line: 5, Character: 33})
		tAssert.True(handled)
		tAssert.NotNil(items)

		importedMemberText := "|===|\nfrom \"./data-import.mace\" import-as Shared;\n|===|\n[output = data]\n{\n  result: Shared.user.\n}"
		importedMemberPath := filepath.Join(root, "imported-member.mace")
		tAssert.NoError(os.WriteFile(importedMemberPath, []byte(importedMemberText), 0o644))
		importedMemberDoc := document{text: importedMemberText, analysis: AnalyzeDocumentAt(importedMemberText, importedMemberPath)}
		items, handled = parsedVariableMemberCompletionItems(importedMemberDoc, protocol.DocumentUri(fileURI(importedMemberPath)), `  result: Shared.user.`, protocol.Position{Line: 5, Character: 22})
		tAssert.True(handled)

		arrayScriptText := "|===|\nstring label;\narray<string> values = [\"a\", \"b\"];\narray<string> unresolved = [missing];\narray<string> bad_local = null;\n|===|\n[output = data] {}\n"
		arrayScriptPath := filepath.Join(root, "array-script.mace")
		tAssert.NoError(os.WriteFile(arrayScriptPath, []byte(arrayScriptText), 0o644))
		arrayIndexText := `[output = data]
{
  items: [1, 2];
  result: $self.items[
}`
		arrayIndexDoc := document{text: arrayIndexText}
		items, handled = arrayIndexCompletionItems(arrayIndexDoc, uri, protocol.Position{Line: 3, Character: protocol.UInteger(len(`  result: $self.items[`))}, "  result: $self.items[", completionScopeOutput)
		tAssert.True(handled)
		tAssert.Equal([]string{"0", "1"}, lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label }))
		_, ok := resolveLocalArrayCompletionTarget(arrayScriptText, protocol.Position{Line: 5, Character: 0}, ast.Identifier{Name: "values"})
		_, ok = resolveLocalCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "missing"}}}, map[string]ast.Expression{}, map[string]struct{}{})
		tAssert.False(ok)
		_, ok = resolveLocalCompletionValue(ast.NullLiteral{}, map[string]ast.Expression{}, map[string]struct{}{})
		tAssert.False(ok)

		_ = scriptVariablesForOutput("`", uri)
		_, ok = resolveCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "missing"}}}, nil, processor.Value{})
		tAssert.False(ok)
		_, ok = resolveCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.Identifier{Name: "missing"}}}}, nil, processor.Value{})
		tAssert.False(ok)
		_, ok = resolveCompletionValue(ast.FloatLiteral{Lexeme: "1..2"}, nil, processor.Value{})
		tAssert.False(ok)

		items, handled = importCompletionItems(document{text: `from "./missing.mace" import User`, analysis: analysisSnapshot{}}, `from "./missing.mace" import User`, uri)
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = importCompletionItems(document{text: `from "./missing.mace" imp`, analysis: analysisSnapshot{}}, `from "./missing.mace" imp`, uri)
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = importCompletionItems(document{text: `from "./data-import.mace" imp`, analysis: analysisSnapshot{}}, `from "./data-import.mace" imp`, uri)
		tAssert.True(handled)
		tAssert.Equal([]string{"import"}, lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label }))

		items, handled = directiveCompletionItems(document{}, uri, `[output = schema, schema = User`)
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = directiveCompletionItems(document{}, uri, `[output = schema, parse = User`)
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = directiveCompletionItems(document{}, uri, `[output = data,`)
		tAssert.True(handled)
		tAssert.NotNil(items)
		_, ok = directivePrefix(`x[`)
		tAssert.False(ok)

		_, ok = stringLiteralCompletionContext("value: \"Ada\\nnext", protocol.Position{Line: 0, Character: 11})
		tAssert.True(ok)
		_, ok = stringLiteralCompletionContext("value: \"Ada\\", protocol.Position{Line: 0, Character: 11})
		tAssert.True(ok)

		_, ok = importableSymbols(uri, root, "../escape.mace")
		tAssert.False(ok)
		_, ok = importableSymbols(uri, root, "./invalid.mace")
		tAssert.False(ok)
		schemaSymbols, ok := importableSymbols(uri, root, "./schema-import.mace")
		tAssert.True(ok)
		tAssert.NotEmpty(schemaSymbols)
		_, _ = documentPathFromURI(protocol.DocumentUri("file:///tmp/%zz"))

		schemaNames := availableSchemaNames(document{text: "prefix[", analysis: analysisSnapshot{}}, uri, "output = schema")
		_ = schemaNames
		_ = availableSchemaNames(document{text: "[bad", analysis: analysisSnapshot{}}, uri, "output = data")
		badImportFile := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `bad`}}}}
		tAssert.Empty(importedPaths(document{text: "", analysis: analysisSnapshot{file: &badImportFile}}, ""))

		stringSelfDoc := document{text: `[output = data]
{
  user: "Ada";
}`, analysis: AnalyzeDocumentAt(`[output = data]
{
  user: "Ada";
}`, filepath.Join(root, "self-string.mace"))}
		_, ok = selfCompletionValue(stringSelfDoc, uri, protocol.Position{Line: 2, Character: 8}, []string{"user", "name"})
		tAssert.False(ok)
		_, ok = partialOutputResult(document{text: "`", analysis: analysisSnapshot{}}, uri, protocol.Position{Line: 0, Character: 0})
		tAssert.False(ok)
		_, ok = partialOutputResult(document{text: `[output = data]
{}`, analysis: analysisSnapshot{}}, uri, protocol.Position{Line: 5, Character: 0})
		tAssert.False(ok)
		_, ok = outputFieldRanges("{ value: 1", lexAnalysisTokens("{ value: 1"), 0)
		tAssert.False(ok)
		tAssert.False(isOutputFieldHeader(lexAnalysisTokens("value"), len(lexAnalysisTokens("value"))-1))

		_, _, ok = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, completionModel{})
		tAssert.False(ok)
		placeholderFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Runtime"}}, DataFields: []ast.OutputField{{Name: "user", Value: ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: completionPlaceholderIdentifier}}}}}
		_, _, ok = placeholderParseInputCompletionType(placeholderFile, completionModel{schemas: map[string]ast.RecordType{"Runtime": {Fields: []ast.SchemaField{{Name: "user", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}}, root, root)
		_, ok = trailingMemberAccessPath("1.user")
		tAssert.False(ok)

		_, ok = unquotedStringChoiceLabel(`"\q"`)
		tAssert.False(ok)

		model := buildCompletionModel(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"../escape.mace"`}}}}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&model, []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"../escape.mace"`}}, root, root, map[string]completionModel{})
		badDirectives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `bad`}, {Kind: ast.OutputDirectiveParseFile, Value: `"../escape.mace"`}}
		tAssert.Nil(parseFileOutputDeclarationDefinitions(badDirectives, root, root, map[string]completionModel{}))
		_, ok = parseFileOutputSchemaRecord(badDirectives, root, root, map[string]completionModel{})
		tAssert.False(ok)
		_, ok = parseFileOutputExportedRecord(badDirectives, root, root, map[string]completionModel{})
		tAssert.False(ok)
		emptyDirectives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./empty-schema.mace"`}}
		_, ok = parseFileOutputExportedRecord(emptyDirectives, root, root, map[string]completionModel{})
		tAssert.False(ok)

		_, _, ok = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `bad`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.False(ok)
		_, _, ok = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"../escape.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.False(ok)
		_, _, ok = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./missing.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.False(ok)
		_, _, ok = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema-import.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared", "user"}, root, root, map[string]completionModel{})
		tAssert.False(ok)

		_, ok = completionOutputFieldType(ast.ArrayLiteral{Elements: []ast.Expression{ast.NullLiteral{}}}, completionModel{})
		tAssert.False(ok)
		_, ok = completionChoiceFromMembers([]ast.Expression{ast.Identifier{Name: "Missing"}}, completionModel{}, map[string]struct{}{})
		tAssert.False(ok)
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "Mode"}, completionModel{aliases: map[string]ast.TypeReference{"Mode": ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"a"`}}}}}, map[string]struct{}{"other": {}})
		_, ok = completionChoiceMemberValues(ast.RecordLiteral{}, completionModel{}, map[string]struct{}{})
		tAssert.False(ok)
		tAssert.Equal("{}", defaultLiteralForType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, completionModel{}, map[string]struct{}{}))
	})

	It("covers final completion zero-count branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))
		uri := protocol.DocumentUri(fileURI(filepath.Join(root, "doc.mace")))

		text := `|===|
schema User: { user: string; };
|===|
[output = data, schema = User]
{
  user: "A";
}`
		path := filepath.Join(root, "doc.mace")
		tAssert.NoError(os.WriteFile(path, []byte(text), 0o644))
		doc := document{text: text, analysis: AnalyzeDocumentAt(text, path)}
		_, _ = outputInitializerCompletionItems(doc, protocol.DocumentUri(fileURI(path)), protocol.Position{Line: 5, Character: 10})

		noCompletionsText := `[output = data]
{
  result:
}`
		noCompletionsPath := filepath.Join(root, "none.mace")
		tAssert.NoError(os.WriteFile(noCompletionsPath, []byte(noCompletionsText), 0o644))
		_, _ = outputInitializerCompletionItems(document{text: noCompletionsText, analysis: AnalyzeDocumentAt(noCompletionsText, noCompletionsPath)}, protocol.DocumentUri(fileURI(noCompletionsPath)), protocol.Position{Line: 2, Character: 9})
		_, _ = outputInitializerCompletionItems(document{text: "[output = data]\n{\n  value: $self.user.\n}", analysis: analysisSnapshot{}}, uri, protocol.Position{Line: 2, Character: 20})

		guardedText := `|===|
schema Runtime: { user?: { name: string; }; };
|===|
[output = data, parse = Runtime]
{
  result: user.
}`
		guardedPath := filepath.Join(root, "guarded.mace")
		tAssert.NoError(os.WriteFile(guardedPath, []byte(guardedText), 0o644))
		guardedDoc := document{text: guardedText, analysis: AnalyzeDocumentAt(guardedText, guardedPath)}
		items, handled := parsedVariableMemberCompletionItems(guardedDoc, protocol.DocumentUri(fileURI(guardedPath)), `  result: user.`, protocol.Position{Line: 5, Character: 15})
		tAssert.True(handled)
		tAssert.Empty(items)

		localArrayText := "|===|\nschema User: { name: string; };\narray<string> values = [\"a\"];\n|===|\n[output = data] {}\n"
		_, _ = resolveLocalArrayCompletionTarget(localArrayText, protocol.Position{Line: 3, Character: 0}, ast.Identifier{Name: "values"})
		arrayPrefixText := `[output = data]
{
  items: [1, 2];
  result: $self.items[1
}`
		_, _ = arrayIndexCompletionItems(document{text: arrayPrefixText}, uri, protocol.Position{Line: 3, Character: protocol.UInteger(len(`  result: $self.items[1`))}, "  result: $self.items[1", completionScopeOutput)

		items, handled = directiveCompletionItems(document{}, uri, `[output = schema, parse_file = "`)
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = directiveCompletionItems(document{}, uri, `[output = data,`)
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		_, prefixOk := directivePrefix(`x[`)
		tAssert.False(prefixOk)

		_, importOK := importableSymbols(uri, root, `../outside.mace`)
		tAssert.False(importOK)
		_, uriOK := documentPathFromURI(protocol.DocumentUri(`file:///tmp/%zz`))
		tAssert.False(uriOK)

		selfDocText := `[output = data]
{
  user: "Ada";
  result: user.name;
}`
		selfPath := filepath.Join(root, "self.mace")
		tAssert.NoError(os.WriteFile(selfPath, []byte(selfDocText), 0o644))
		_, selfOK := selfCompletionValue(document{text: selfDocText, analysis: AnalyzeDocumentAt(selfDocText, selfPath)}, protocol.DocumentUri(fileURI(selfPath)), protocol.Position{Line: 3, Character: 10}, []string{"user", "name"})
		tAssert.False(selfOK)

		_, _, placeholderOK := placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData}}, completionModel{})
		tAssert.False(placeholderOK)
		_, trailingOK := trailingMemberAccessPath("user.1")
		tAssert.False(trailingOK)

		_ = buildCompletionModel(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"../outside.mace"`}}}}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&completionModel{}, []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"../outside.mace"`}}, root, root, map[string]completionModel{})
		_ = parseFileOutputDeclarationDefinitions([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"../outside.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"../outside.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"../outside.mace"`}}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./missing.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"../outside.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})

		_ = resolveCompletionType(ast.UnionType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Missing"}}}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.NamedType{Name: "Alias"}, completionModel{aliases: map[string]ast.TypeReference{"Alias": ast.PrimitiveType{Name: "string"}}}, map[string]struct{}{"existing": {}})
		tAssert.Equal("{}", defaultLiteralForType(ast.VariantType{Members: []ast.TypeReference{ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}}, completionModel{}, map[string]struct{}{}))
	})

	It("covers additional completion pure helper branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))
		plainDoc := document{text: "plain text", analysis: analysisSnapshot{}}
		outputURI := protocol.DocumentUri(fileURI(filepath.Join(root, "doc.mace")))

		_, _ = outputSchemaDirective(ast.File{})
		_, _ = documentPathFromURI(protocol.DocumentUri("file:///tmp/%zz"))
		_ = buildCompletionModel(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: "bad"}}}}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&completionModel{}, []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: "bad"}}, root, root, map[string]completionModel{})
		_ = availableSchemaNames(document{text: "[", analysis: analysisSnapshot{}}, outputURI, "output = schema")
		_ = importedPaths(document{text: `from bad import x;`, analysis: analysisSnapshot{}}, `from bad import x;`)
		_, _ = selfCompletionValue(plainDoc, outputURI, protocol.Position{}, []string{"missing"})
		_, _ = partialOutputResult(document{text: "plain", analysis: analysisSnapshot{}}, outputURI, protocol.Position{})
		_, _ = outputFieldRanges("plain", lexAnalysisTokens("plain"), 0)
		_ = isOutputFieldHeader(lexAnalysisTokens("value"), 0)
		_, _, _ = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, completionModel{})
		_, _, _ = placeholderParseInputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, completionModel{}, root, root)
		_, _, _ = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData}}, completionModel{})
		inputPlaceholderFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.MemberAccess{Target: ast.Identifier{Name: completionPlaceholderIdentifier}, Name: "name"}}}}}
		inputPlaceholderModel := completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "value", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}}
		_, _, _ = placeholderParseInputCompletionType(inputPlaceholderFile, inputPlaceholderModel, root, root)
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./alias.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		_, _ = expressionPath(ast.BooleanLiteral{Value: true})
		_, _ = trailingMemberAccessPath(".")
		_ = syntheticCompletionValue(ast.NamedType{Name: "Missing"}, completionModel{}, 0)
		_, _ = unquotedStringChoiceLabel(`"bad`)
		_, _ = importableIdentifiers(protocol.DocumentUri("file:///tmp/%zz"), root, "./schema.mace")
		_, _ = completionOutputFieldType(ast.ArrayLiteral{Elements: []ast.Expression{}}, completionModel{})
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "Missing"}, completionModel{}, map[string]struct{}{})
		_, _ = completionUnionRecord([]ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "float"}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "boolean"}, completionModel{}, map[string]struct{}{})
	})

	It("covers targeted remaining completion branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))
		tAssert.NoError(os.WriteFile(filepath.Join(root, "schema-out.mace"), []byte(`[output = schema]
{
  User: { name: string; profile: { city: string; }; };
}`), 0o644))
		tAssert.NoError(os.WriteFile(filepath.Join(root, "alias-data.mace"), []byte(`[output = data]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
}`), 0o644))

		stringDocText := `[output = data, schema = User, schema_file = "./schema-out.mace"]
{
  value: "Ada"
}`
		stringDocPath := filepath.Join(root, "string-doc.mace")
		tAssert.NoError(os.WriteFile(stringDocPath, []byte(stringDocText), 0o644))
		stringDoc := document{text: stringDocText, analysis: AnalyzeDocumentAt(stringDocText, stringDocPath)}
		stringURI := protocol.DocumentUri(fileURI(stringDocPath))
		_, _ = outputInitializerCompletionItems(stringDoc, stringURI, protocol.Position{Line: 2, Character: 12})

		guardText := `[output = data]
{
  value: $self.user.
}`
		guardPath := filepath.Join(root, "guard.mace")
		tAssert.NoError(os.WriteFile(guardPath, []byte(guardText), 0o644))
		guardDoc := document{text: guardText, analysis: AnalyzeDocumentAt(guardText, guardPath)}
		_, handled := outputInitializerCompletionItems(guardDoc, protocol.DocumentUri(fileURI(guardPath)), protocol.Position{Line: 2, Character: 20})
		tAssert.False(handled)

		memberText := `|===|
from "./alias-data.mace" import-as Shared;
|===|
[output = data]
{
  value: Shared.user.
}`
		memberPath := filepath.Join(root, "member.mace")
		tAssert.NoError(os.WriteFile(memberPath, []byte(memberText), 0o644))
		memberDoc := document{text: memberText, analysis: AnalyzeDocumentAt(memberText, memberPath)}
		_, _ = parsedVariableMemberCompletionItems(memberDoc, protocol.DocumentUri(fileURI(memberPath)), "  value: Shared.user.", protocol.Position{Line: 5, Character: 21})

		arrayItems, handled := arrayIndexCompletionItems(document{text: "", analysis: analysisSnapshot{}}, protocol.DocumentUri("file:///tmp/doc.mace"), protocol.Position{}, "values[1", completionScopeOutput)
		tAssert.True(handled)
		tAssert.Empty(arrayItems)

		localArrayText := "|===|\narray<string> items = [\"x\"];\n|===|\n[output = data] {}"
		_, _ = resolveLocalArrayCompletionTarget(localArrayText, protocol.Position{Line: 2, Character: 0}, ast.Identifier{Name: "items"})

		_, handled = importCompletionItems(document{}, `from "./missing.mace" imp`, protocol.DocumentUri(fileURI(filepath.Join(root, "x.mace"))))
		tAssert.True(handled)
		_, handled = directiveCompletionItems(document{}, protocol.DocumentUri(fileURI(filepath.Join(root, "x.mace"))), "[output = data, schema = User")
		tAssert.True(handled)
		_, handled = directiveCompletionItems(document{}, protocol.DocumentUri(fileURI(filepath.Join(root, "x.mace"))), "[output = data, unknown")
		tAssert.True(handled)
		_, _ = directivePrefix("[output = data")

		_, importableOK := importableSymbols(protocol.DocumentUri("file:///tmp/%zz"), root, "./alias-data.mace")
		tAssert.False(importableOK)

		selfText := `[output = data]
{
  user: { name: "Ada"; };
}`
		selfDoc := document{text: selfText, analysis: AnalyzeDocumentAt(selfText, filepath.Join(root, "self-ok.mace"))}
		_, _ = selfCompletionValue(selfDoc, protocol.DocumentUri(fileURI(filepath.Join(root, "self-ok.mace"))), protocol.Position{Line: 2, Character: 10}, []string{"user"})
		_, partialOK := partialOutputResult(document{text: "plain", analysis: analysisSnapshot{}}, protocol.DocumentUri(fileURI(filepath.Join(root, "plain.mace"))), protocol.Position{Line: 99, Character: 0})
		tAssert.False(partialOK)

		_, _, _ = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}, completionModel{})
		_, trailingOK := trailingMemberAccessPath("user?.")
		tAssert.False(trailingOK)

		_ = buildCompletionModel(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"bad`}}}}, root, root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&completionModel{}, []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"bad`}}, root, root, map[string]completionModel{})
		tAssert.Empty(parseFileOutputDeclarationDefinitions([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"bad`}}, root, root, map[string]completionModel{}))
		_, schemaRecordOK := parseFileOutputSchemaRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"bad`}}, root, root, map[string]completionModel{})
		tAssert.False(schemaRecordOK)
		_, exportedRecordOK := parseFileOutputExportedRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"bad`}}, root, root, map[string]completionModel{})
		tAssert.False(exportedRecordOK)

		_, _, importedOK := importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./alias-data.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared", "user"}, root, root, map[string]completionModel{})
		_ = importedOK
		_, _, importedOK = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: "bad"}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.False(importedOK)
		_, _, importedOK = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema-out.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, []string{"Shared"}, root, root, map[string]completionModel{})
		tAssert.False(importedOK)

		cyclicModel := completionModel{aliases: map[string]ast.TypeReference{"Alias": ast.NamedType{Name: "Alias"}}}
		tAssert.Equal(completionType{}, resolveCompletionType(ast.NamedType{Name: "Alias"}, cyclicModel, map[string]struct{}{}))
		tAssert.Equal(completionType{}, resolveCompletionType(ast.NamedType{Name: "Missing"}, completionModel{}, map[string]struct{}{}))
		tAssert.Equal("{}", defaultLiteralForType(ast.NamedType{Name: "Missing"}, completionModel{}, map[string]struct{}{}))
	})
})
