package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("analyzer completion branch coverage", func() {
	It("covers low-level completion branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		schemaPath := filepath.Join(root, "schema.mace")
		dataPath := filepath.Join(root, "data.mace")
		importPath := filepath.Join(root, "imported.mace")
		scriptPath := filepath.Join(root, "script.mace")
		memberPath := filepath.Join(root, "member.mace")
		emptyPath := filepath.Join(root, "empty.mace")

		schemaText := `[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Shared: { value: string; };
  Choice: choice["Ada", 1, true];
}`
		dataText := `[output = data, schema = User]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
  count: 1;
  values: [1, 2];
}`
		importText := `[output = schema]
{
  Exported: { value: string; };
}`
		scriptText := `|===|
string value = "Ada";
int number = 1;
|===|`
		memberText := `[output = data, schema = User]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
  ref: user.profile.
}`
		emptyText := `[output = data]
{
  value:
}`

		tAssert.NoError(os.WriteFile(schemaPath, []byte(schemaText), 0o644))
		tAssert.NoError(os.WriteFile(dataPath, []byte(dataText), 0o644))
		tAssert.NoError(os.WriteFile(importPath, []byte(importText), 0o644))
		tAssert.NoError(os.WriteFile(scriptPath, []byte(scriptText), 0o644))
		tAssert.NoError(os.WriteFile(memberPath, []byte(memberText), 0o644))
		tAssert.NoError(os.WriteFile(emptyPath, []byte(emptyText), 0o644))

		schemaSnapshot := AnalyzeDocumentAt(schemaText, schemaPath)
		dataSnapshot := AnalyzeDocumentAt(dataText, dataPath)
		scriptSnapshot := AnalyzeDocumentAt(scriptText, scriptPath)
		memberSnapshot := AnalyzeDocumentAt(memberText, memberPath)
		emptySnapshot := AnalyzeDocumentAt(emptyText, emptyPath)

		schemaDoc := document{text: schemaText, analysis: schemaSnapshot}
		dataDoc := document{text: dataText, analysis: dataSnapshot}
		scriptDoc := document{text: scriptText, analysis: scriptSnapshot}
		memberDoc := document{text: memberText, analysis: memberSnapshot}
		emptyDoc := document{text: emptyText, analysis: emptySnapshot}
		uri := protocol.DocumentUri(fileURI(dataPath))
		_ = dataDoc
		_ = scriptDoc
		_ = uri
		memberURI := protocol.DocumentUri(fileURI(memberPath))
		scriptURI := protocol.DocumentUri(fileURI(scriptPath))

		_, _ = directivePrefix("[output = data]")
		_, _ = directivePrefix(" [output = data]")
		_, _ = directivePrefix("plain")
		_, _ = stringLiteralCompletionContext(`value = "Ada"`, protocol.Position{Line: 0, Character: 10})
		_, _ = stringLiteralCompletionContext(`value = Ada`, protocol.Position{Line: 0, Character: 8})
		_, _ = completionPlaceholderPosition("value =", protocol.Position{Line: 0, Character: 7}, "=:")
		_, _ = completionPlaceholderPosition("value", protocol.Position{Line: 0, Character: 5}, "=:")
		_, _, _ = selfCompletionContext("$self.user.profile")
		_, _, _ = selfCompletionContext("$self.")
		_, _, _ = selfCompletionContext("plain")
		_, _ = outputMemberAccessContext("user.profile.")
		_, _ = outputMemberAccessContext("$self.user.")
		_, _ = outputMemberAccessContext(".")
		_, _ = outputMemberAccessContext("$a.")
		_, _ = outputMemberAccessContext("plain")
		_, _ = trailingMemberAccessPath("user.profile.")
		_, _ = trailingMemberAccessPath("plain")
		_, _ = expressionPath(ast.Identifier{Name: "value"})
		_, _ = expressionPath(ast.Identifier{Name: ""})
		_, _ = placeholderPath(ast.Identifier{Name: completionPlaceholderIdentifier})
		_, _ = placeholderPath(ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: completionPlaceholderIdentifier})
		_, _ = placeholderPath(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}})
		_, _ = placeholderPath(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: completionPlaceholderIdentifier}}})
		_, _ = placeholderPath(ast.PrefixExpression{Right: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = placeholderPath(ast.InfixExpression{Left: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = placeholderPath(ast.ConditionalExpression{Then: ast.Identifier{Name: completionPlaceholderIdentifier}})
		_, _ = placeholderPath(ast.StringLiteral{Lexeme: `"Ada"`})

		_ = bareSelfCompletionItems(":", protocol.Position{Line: 0, Character: 0})
		_ = bareSelfCompletionItems("plain", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("$self", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("$self.", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("$x", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("plain", protocol.Position{Line: 0, Character: 0})

		_ = completionDeclarations(emptyDoc, scriptURI, protocol.Position{Line: 0, Character: 0}, "", completionScopeScript)
		_ = completionDeclarations(schemaDoc, scriptURI, protocol.Position{Line: 0, Character: 0}, "", completionScopeOutput)
		_ = completionDeclarations(schemaDoc, scriptURI, protocol.Position{Line: 0, Character: 0}, "", completionScopeFile)
		_ = completionDeclarations(document{text: scriptText, analysis: analysisSnapshot{}}, scriptURI, protocol.Position{Line: 1, Character: 0}, "", completionScopeScript)
		_ = completionDeclarations(document{text: "plain", analysis: analysisSnapshot{}}, scriptURI, protocol.Position{Line: 0, Character: 0}, "", completionScopeFile)
		_ = completionDeclarations(document{text: "plain", analysis: analysisSnapshot{}}, scriptURI, protocol.Position{Line: 0, Character: 0}, "", completionScope(99))

		_, _ = importCompletionItems(document{text: "from \"./", analysis: schemaSnapshot}, scriptURI, "from \"./",)
		_, _ = importCompletionItems(document{text: "from \"./schema.mace\" import Ex", analysis: schemaSnapshot}, scriptURI, `from "./schema.mace" import Ex`)
		_, _ = importCompletionItems(document{text: "import ", analysis: schemaSnapshot}, scriptURI, `import `)
		_, _ = importCompletionItems(document{text: "plain", analysis: schemaSnapshot}, scriptURI, `plain`)

		_, _ = directiveCompletionItems(schemaDoc, scriptURI, "[")
		_, _ = directiveCompletionItems(schemaDoc, scriptURI, "[output = ")
		_, _ = directiveCompletionItems(schemaDoc, scriptURI, "[output = data, ")
		_, _ = directiveCompletionItems(schemaDoc, scriptURI, "[output = schema, schema = ")
		_, _ = directiveCompletionItems(schemaDoc, scriptURI, "[output = schema, schema_file = \"./")
		_, _ = directiveCompletionItems(schemaDoc, scriptURI, "plain")
		stringLiteralText := "|===|\nstring value = \"Ada\";\n|===|"
		stringLiteralSnapshot := AnalyzeDocumentAt(stringLiteralText, scriptPath)
		outputStringText := "[output = data, schema = User]\n{\n  value: \"Ada\";\n}\n"
		outputStringSnapshot := AnalyzeDocumentAt(outputStringText, scriptPath)
		_, _ = stringLiteralInitializerCompletionItems(document{text: stringLiteralText, analysis: stringLiteralSnapshot}, scriptURI, protocol.Position{Line: 1, Character: protocol.UInteger(len("string value = \"Ada"))}, false)
		_, _ = stringLiteralInitializerCompletionItems(document{text: stringLiteralText, analysis: stringLiteralSnapshot}, scriptURI, protocol.Position{Line: 1, Character: protocol.UInteger(len("string value = \"Ada"))}, true)
		_, _ = stringLiteralInitializerCompletionItems(document{text: outputStringText, analysis: outputStringSnapshot}, scriptURI, protocol.Position{Line: 2, Character: protocol.UInteger(len("  value: \"Ada"))}, true)
		richOutputText := `|===|
schema User: { value: string; user: { profile: { city: string; }; }; };
|===|
[output = data, schema = User]
{
  value: "Ada";
  ref: user.profile.
}`
		richOutputSnapshot := AnalyzeDocumentAt(richOutputText, scriptPath)
		richOutputDoc := document{text: richOutputText, analysis: richOutputSnapshot}
		_, _ = stringLiteralInitializerCompletionItems(richOutputDoc, scriptURI, protocol.Position{Line: 5, Character: protocol.UInteger(len("  value: \"Ada"))}, true)
		_, _ = outputInitializerCompletionItems(document{text: stringLiteralText, analysis: stringLiteralSnapshot}, scriptURI, protocol.Position{Line: 1, Character: protocol.UInteger(len("string value = \"Ada"))})
		_, _ = outputInitializerCompletionItems(document{text: outputStringText, analysis: outputStringSnapshot}, scriptURI, protocol.Position{Line: 2, Character: protocol.UInteger(len("  value: \"Ada"))})
		_, _ = outputInitializerCompletionItems(richOutputDoc, scriptURI, protocol.Position{Line: 5, Character: protocol.UInteger(len("  value: \"Ada"))})
		_, _ = outputInitializerCompletionItems(richOutputDoc, scriptURI, protocol.Position{Line: 6, Character: protocol.UInteger(len("  ref: user.profile."))})
		parseOutputText := `|===|
schema User: { value: string; };
|===|
[output = data, parse = User]
{
  value: "Ada";
}`
		parseOutputSnapshot := AnalyzeDocumentAt(parseOutputText, scriptPath)
		parseOutputDoc := document{text: parseOutputText, analysis: parseOutputSnapshot}
		_, _ = outputInitializerCompletionItems(parseOutputDoc, scriptURI, protocol.Position{Line: 5, Character: protocol.UInteger(len("  value: \"Ada"))})
		parseEmptyText := `|===|
schema User: { value: string; };
|===|
[output = data, parse = User]
{
  value:
}`
		parseEmptySnapshot := AnalyzeDocumentAt(parseEmptyText, scriptPath)
		parseEmptyDoc := document{text: parseEmptyText, analysis: parseEmptySnapshot}
		_, _ = outputInitializerCompletionItems(parseEmptyDoc, scriptURI, protocol.Position{Line: 5, Character: protocol.UInteger(len("  value:"))})
		badInitText := "|===|\ntype Missing: Unknown;\nMissing value =\n|===|"
		badInitSnapshot := AnalyzeDocumentAt(badInitText, scriptPath)
		_, _ = initializerCompletionItems(document{text: badInitText, analysis: badInitSnapshot}, scriptURI, protocol.Position{Line: 2, Character: protocol.UInteger(len("Missing value ="))})
		_, _ = outputInitializerCompletionItems(memberDoc, memberURI, protocol.Position{Line: 3, Character: protocol.UInteger(len("  ref: user.profile."))})
		_, _ = outputInitializerCompletionItems(emptyDoc, scriptURI, protocol.Position{Line: 2, Character: protocol.UInteger(len("  value:"))})
		_, _ = outputInitializerCompletionItems(document{text: `[output = data, parse = User]
{
  value:
}
|===|
schema User: { name: string; };
|===|`, analysis: AnalyzeDocumentAt(`[output = data, parse = User]
{
  value:
}
|===|
schema User: { name: string; };
|===|`, root)}, scriptURI, protocol.Position{Line: 2, Character: protocol.UInteger(len("  value:"))})
		_ = bareSelfCompletionItems("", protocol.Position{Line: 0, Character: 0})
		_, _ = selfKeywordCompletionItems("", protocol.Position{Line: 0, Character: 0})

		_, _ = importableSymbols(scriptURI, root, "./schema.mace")
		_, _ = importableSymbols(protocol.DocumentUri("http://example.com"), root, "./schema.mace")
		_, _ = documentPathFromURI(protocol.DocumentUri(fileURI(schemaPath)))
		_, _ = documentPathFromURI(protocol.DocumentUri("http://example.com"))
		_ = relativePathItems(schemaDoc, protocol.DocumentUri(fileURI(schemaPath)), "", nil, true)
		_ = relativePathItems(document{text: "plain"}, protocol.DocumentUri("http://example.com"), "", nil, true)
		_ = availableSchemaNames(schemaDoc, protocol.DocumentUri(fileURI(schemaPath)), "[output = schema]")
		_ = availableSchemaNames(document{text: "plain"}, protocol.DocumentUri(fileURI(schemaPath)), "plain")
		_ = importedPaths(schemaDoc, "[output = data, schema_file = \"./schema.mace\"]")
		_ = currentImports(schemaDoc, "[output = data, schema_file = \"./schema.mace\"]")
		_ = completionFile(emptyDoc, "plain")
		_ = completionFile(schemaDoc, "plain")

		_ = partialScriptVariables(scriptText, scriptURI, protocol.Position{Line: 1, Character: 15})
		_ = partialScriptVariables("plain", scriptURI, protocol.Position{Line: 0, Character: 0})
		_ = scriptVariablesForOutput(scriptText, scriptURI)
		_, _ = selfCompletionValue(memberDoc, memberURI, protocol.Position{Line: 3, Character: 15}, []string{"user", "profile"})
		_, _ = selfCompletionValue(memberDoc, memberURI, protocol.Position{Line: 3, Character: 15}, []string{"missing"})
		_, _ = partialOutputResult(memberDoc, memberURI, protocol.Position{Line: 3, Character: 15})
		_, _ = partialOutputResult(memberDoc, memberURI, protocol.Position{Line: 0, Character: 0})
		_, _ = outputFieldRanges(memberText, lexAnalysisTokens(memberText), strings.Index(memberText, "{"))
		_, _ = outputFieldRanges("plain", lexAnalysisTokens("plain"), 0)
		_ = isOutputFieldHeader(lexAnalysisTokens("{ value: 1; }"), 1)
		_ = isOutputFieldHeader(lexAnalysisTokens("{ value ? : 1; }"), 1)
		_, _ = stringLiteralValue(ast.StringLiteral{Lexeme: `"Ada"`})
		_, _ = stringLiteralValue(ast.StringLiteral{Lexeme: `Ada`})
		_, _ = completionFileWithPlaceholder(scriptText, protocol.Position{Line: 1, Character: 15})
		_, _ = completionFileWithPlaceholder("plain", protocol.Position{Line: 0, Character: 0})
		_, _ = completionFileWithExpressionPlaceholder(`value = "Ada"`, 8, 13)
		_, _ = completionFileWithExpressionPlaceholder(`value = "Ada"`, -1, 13)
		_, _, _ = placeholderCompletionType(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", HasValue: true, Type: ast.PrimitiveType{Name: "string"}, Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}, completionModel{})
		_, _, _ = placeholderCompletionType(ast.File{}, completionModel{})
		_, _, _ = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}})
		_, _, _ = placeholderOutputCompletionType(ast.File{}, completionModel{})
		_, _, _ = placeholderParseInputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, root, root)
		_, _, _ = placeholderParseInputCompletionType(ast.File{}, completionModel{}, root, root)
		_, _ = completionOutputFieldType(ast.StringLiteral{Lexeme: `"Ada"`}, completionModel{})
		_, _ = completionOutputFieldType(ast.ArrayLiteral{}, completionModel{})
		_, _ = completionOutputFieldType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, completionModel{})
		_, _ = completionOutputFieldType(ast.Identifier{Name: "User"}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}})
		_, _ = completionOutputFieldType(ast.Identifier{Name: "Missing"}, completionModel{})
		_, _ = resolveCompletionValue(ast.Identifier{Name: "value"}, map[string]processor.Value{"value": {Kind: processor.ValueString, String: "Ada"}}, processor.Value{})
		_, _ = resolveCompletionValue(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"}, map[string]processor.Value{"value": {Kind: processor.ValueRecord, Record: map[string]processor.Value{"profile": {Kind: processor.ValueString, String: "LA"}}}}, processor.Value{})
		_, _ = resolveCompletionValue(ast.ArrayAccess{Target: ast.Identifier{Name: "value"}, Index: ast.IntLiteral{Lexeme: "0"}}, map[string]processor.Value{"value": {Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueString, String: "Ada"}}}}, processor.Value{})
		_, _ = resolveCompletionValue(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.SelfReference{Path: []string{"user"}}, nil, processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"user": {Kind: processor.ValueString, String: "Ada"}}})
		_, _ = resolveCompletionValue(ast.StringLiteral{Lexeme: `"Ada"`}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.IntLiteral{Lexeme: "1"}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.FloatLiteral{Lexeme: "1.0"}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.BooleanLiteral{Value: true}, nil, processor.Value{})
		_, _ = resolveCompletionValue(ast.Identifier{Name: "missing"}, nil, processor.Value{})
		_, _ = outputValueAtSegments(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"user": {Kind: processor.ValueString, String: "Ada"}}}, []string{"user"})
		_, _ = outputValueAtSegments(processor.Value{Kind: processor.ValueArray}, []string{"user"})
		_, _ = completionChoiceFromMembers([]ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}}, completionModel{}, map[string]struct{}{})
		_, _ = completionChoiceFromMembers([]ast.Expression{ast.Identifier{Name: "AliasChoice"}}, completionModel{aliases: map[string]ast.TypeReference{"AliasChoice": ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.StringLiteral{Lexeme: `"Bee"`}}}}}, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.StringLiteral{Lexeme: `"Ada"`}, completionModel{}, map[string]struct{}{})
		_, _ = completionChoiceMemberValues(ast.Identifier{Name: "AliasChoice"}, completionModel{}, map[string]struct{}{})
		_ = schemaLiteral(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "count", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "string"}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.PrimitiveType{Name: "int"}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, completionModel{}, map[string]struct{}{})
		_ = defaultLiteralForType(ast.NamedType{Name: "User"}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, map[string]struct{}{})
		_, _ = unquotedStringChoiceLabel(`"Ada"`)
		_, _ = unquotedStringChoiceLabel(`Ada`)
		_ = buildCompletionModel(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}}}, Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.TypeDeclaration{Name: "Alias", Type: ast.NamedType{Name: "User"}}, ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}, Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}}, filepath.Dir(schemaPath), root, map[string]completionModel{})
		mergeDirectiveCompletionModels(&completionModel{}, nil, root, root, map[string]completionModel{})
		_ = parseFileOutputDeclarationDefinitions([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputSchemaRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _ = parseFileOutputExportedRecord([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./imported.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Exported", Alias: "Shared"}}}}, []string{"Shared", "value"}, root, root, map[string]completionModel{})
		_, _, _ = importedMemberCompletionRootType(ast.File{}, []string{"Shared"}, root, root, map[string]completionModel{})
		_, _ = importAsSchemaRecord(ast.File{Output: ast.OutputBlock{SchemaFields: []ast.OutputSchemaField{{Name: "user", Type: ast.NamedType{Name: "User"}}}}}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}})
		_, _ = importAsSchemaRecord(ast.File{}, completionModel{})
		_, _ = importAsDataRecord(ast.File{Output: ast.OutputBlock{DataFields: []ast.OutputField{{Name: "user", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}}, completionModel{})
		_, _ = importAsDataRecord(ast.File{}, completionModel{})
		_ = resolveCompletionType(ast.PrimitiveType{Name: "string"}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.UnionType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, completionModel{}, map[string]struct{}{})
		_ = resolveCompletionType(ast.NamedType{Name: "User"}, completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, map[string]struct{}{})
		_ = resolveCompletionType(ast.NamedType{Name: "Missing"}, completionModel{}, map[string]struct{}{})
	})
})
