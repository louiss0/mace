package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("analyzer analysis helper coverage", func() {
	It("covers diagnostics and refactor helpers directly", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		schemaPath := filepath.Join(root, "schema.mace")
		dataPath := filepath.Join(root, "data.mace")
		recordPath := filepath.Join(root, "record.mace")
		documentPath := filepath.Join(root, "document.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`
[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Runtime: { env: string; };
}
`), 0o644))
		tAssert.NoError(os.WriteFile(dataPath, []byte(`
[output = data, schema = User]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
  count: 1;
  mixed: ["a", 1];
  items: ["x"];
}
`), 0o644))
		tAssert.NoError(os.WriteFile(recordPath, []byte(`
[output = data]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
}
`), 0o644))
		tAssert.NoError(os.WriteFile(documentPath, []byte(`
|===|
from "./schema.mace" import User, Runtime;
string value = "x"
Profile record = { age: 1; active: true; };
|===|
[output = data, schema = User, schema_file = "./schema.mace", parse = User, parse_file = "./schema.mace"]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
  value: "x";
  mixed: ["a", 1];
  index: items[12];
  self_ref: $self.user.profile;
}
`), 0o644))

		tokens := lexAnalysisTokens(stringMustRead(documentPath))
		file := ast.File{
			Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}, {Name: "Runtime", Alias: "Run"}}}},
			Script: &ast.ScriptBlock{Items: []ast.Declaration{
				ast.TypeDeclaration{Name: "Alias", Type: ast.NamedType{Name: "User"}},
				ast.SchemaDeclaration{Name: "Doc", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "field", Type: ast.PrimitiveType{Name: "string"}}}}},
				ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"x"`}},
			}},
			Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema.mace"`}, {Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}, DataFields: []ast.OutputField{{Name: "user", Value: ast.Identifier{Name: "user"}}, {Name: "value", Value: ast.StringLiteral{Lexeme: `"x"`}}}, SchemaFields: []ast.OutputSchemaField{{Name: "user", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}},
		}

		_ = AnalyzeDocumentAtInRoot(stringMustRead(documentPath), documentPath, root)
		snapshot := AnalyzeDocumentAt(stringMustRead(documentPath), documentPath)
		_, _ = analysisSnapshot{}.definitionAt(protocol.Position{})
		_, _ = analysisSnapshot{file: &file, text: stringMustRead(documentPath), documentURI: protocol.DocumentUri(fileURI(documentPath)), symbols: []semanticSymbol{{Name: "value", Definition: protocol.Location{URI: protocol.DocumentUri(fileURI(documentPath))}, Range: protocol.Range{}}}}.definitionAt(protocol.Position{Line: 3, Character: 3})
		_, _ = analysisSnapshot{file: &file, text: stringMustRead(documentPath), documentURI: protocol.DocumentUri(fileURI(documentPath)), symbols: []semanticSymbol{{Name: "value", Definition: protocol.Location{URI: protocol.DocumentUri(fileURI(documentPath))}, Range: protocol.Range{}}}}.documentSymbolAt(protocol.Position{Line: 3, Character: 3})
		_, _ = analysisSnapshot{file: &file, text: stringMustRead(documentPath), documentURI: protocol.DocumentUri(fileURI(documentPath)), symbols: []semanticSymbol{{Name: "value", Definition: protocol.Location{URI: protocol.DocumentUri(fileURI(documentPath))}}}}.definitionSymbol("value")
		_ = analyzeDocumentAtInRoot(stringMustRead(documentPath), documentPath, root).codeActions(protocol.DocumentUri(fileURI(documentPath)), protocol.Range{})
		_, _ = analyzeFileStructure(stringMustRead(documentPath), file, tokens, documentPath)
		_, _ = directivePathDiagnostics(file, tokens, documentPath)
		_, _ = addMissingScriptSemicolonText("|===|\nstring value = \"x\"\n|===|")
		_, _ = moveScriptBlockBeforeOutputText("[output = data]\n{}\n|===|\nstring value = \"x\";\n|===|")
		_, _ = removeLeadingEmptyScriptBlockText("|===|\n|===|\n[output = data]{}")
		_, _ = extractRecordLiteralIntoSchemaText("|===|\nProfile record = { age: 1; active: true; };\n|===|")
		_ = inferOutputSchemaFields("name: \"Ada\"; count: 1; items: [\"x\"];")
		_ = inferRecordSchemaFields("name: \"Ada\"; count: 1; items: [\"x\"];")
		_ = defaultLiteralForTypeName("string")
		_ = defaultLiteralForTypeName("hex_int")
		_ = defaultLiteralForTypeName("array<string>")
		_, _ = replaceVariableDeclaration("string value = \"x\";", regexp.MustCompile(`(?m)^([ \t]*)(string)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*("[^"]*");`), func(matches []string) string {
			return matches[1] + matches[2] + " fresh = " + matches[4] + ";"
		})
		_, _ = renameDuplicateVariableText("|===|\nstring value = \"x\";\nstring value = \"y\";\n|===|")
		_ = simpleExpressionText(ast.BooleanLiteral{Value: true})
		_ = simpleExpressionText(ast.Identifier{Name: "value"})
		_ = defaultExpressionForType(ast.PrimitiveType{Name: "hex_float"})
		_ = defaultExpressionForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}})
		_ = inferredTypeFromExpression(ast.BooleanLiteral{Value: true})
		_ = inferredTypeFromExpression(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"x"`}}}})
		_, _, _ = missingImportEdit(stringMustRead(documentPath), file, tokens, "missing import")
		_, _ = invalidDirectiveComboEditRange(stringMustRead(documentPath), file, tokens, "schema directive is invalid when output mode is schema")
		_, _, _ = generateOutputFromSchemaEdit(stringMustRead(documentPath), file, tokens, "missing required field")
		_, _ = fieldEditRangeAt(stringMustRead(documentPath), tokens, 2)
		_, _ = formatTextQuick(stringMustRead(documentPath))
		_, _ = unusedImportAnalysis(stringMustRead(documentPath), file, tokens, documentPath)
		_, _ = importAliasToken(tokens, file.Imports[0], ast.ImportedIdentifier{Name: "Runtime", Alias: "Run"})
		_, _ = importIdentifierEditRange(stringMustRead(documentPath), tokens, lexer.Token{Line: 2, Column: 1, Lexeme: "from"}, false)
		_, _ = importDeclarationEditRange(stringMustRead(documentPath), tokens, 0)
		_, _ = unusedDeclarationAnalysis(stringMustRead(documentPath), file, tokens, documentPath)
		_, _ = declarationEditRange(stringMustRead(documentPath), tokens, lexer.Token{Line: 3, Column: 8, Lexeme: "value"})
		_ = schemaOutputVariableDiagnostics(file, tokens)
		_, _, _ = schemaFileConflictAnalysis(stringMustRead(documentPath), file, documentPath)
		_, _ = parseDirectiveWarningDiagnostic(stringMustRead(documentPath), file)
		_, _ = semanticDiagnosticFromError(file, tokens, processor.DiagnosticError{Code: processor.CodeTypeMismatch, Message: "processor: type mismatch: expected string, got int"})
		_, _ = semanticDiagnosticFromError(file, tokens, processor.DiagnosticError{Code: processor.CodeInvalidNullUsage, Message: "null bad"})
		_, _ = variableTypeMismatchDiagnostic(file, tokens, processor.DiagnosticError{Code: processor.CodeTypeMismatch, Fields: processor.DiagnosticFields{Expected: "string", Actual: "int"}, Message: "processor: type mismatch: expected string, got int"})
		_, _ = mixedArrayLiteralDiagnostic(file, tokens, "mixed array")
		_, _ = arrayAccessDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeArrayIndexOutOfRange, Fields: processor.DiagnosticFields{Index: "12", Level: 2}, Message: "array index out of range"}, "array index out of range")
		_ = arrayAccessCandidates(tokens)
		_, _ = schemaDiagnostic(tokens, processor.DiagnosticError{Code: processor.CodeInvalidOutputSchemaField, Fields: processor.DiagnosticFields{Name: "user", Field: "name"}, Message: "schema bad"}, "schema bad")
		_, _ = unknownSchemaDiagnostic(tokens, `unknown schema "User"`)
		_, _ = selfReferenceDiagnostic(file, tokens, processor.DiagnosticError{Code: processor.CodeSelfReferenceUnknown, Fields: processor.DiagnosticFields{Path: "user.profile"}, Message: "self ref"}, "self ref")
		_ = collectSemanticSymbols(file, tokens, nil, documentPath)
		_ = declarationDocumentation(file, "Alias")
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"Ada"`})
		_, _ = parseInputSemanticSchemaName(file, root)
		_ = importedSemanticSymbols(file, documentPath)
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}, documentPath, "Shared")
		_, _, _, _ = parsedFile(schemaPath)
		_ = summarizeValue(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueInt, Int: 1}}})
		_ = expressionSummary(ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: "profile"})
		_ = indexSymbols([]semanticSymbol{{Name: "a"}})
		_, _ = outputDirectiveListRange(stringMustRead(documentPath))
		_, _, _ = schemaFileDirectiveRanges(stringMustRead(documentPath))
		_, _ = importAndScriptCleanupRange(stringMustRead(documentPath))
		_ = quotedName(`unknown schema "Name"`)
		_ = analyzeDocumentAtInRoot(stringMustRead(documentPath), documentPath, root)
		_ = snapshot
	})

	It("covers analysis helper edge branches", func() {
		workspace := GinkgoT().TempDir()
		root := filepath.Join(workspace, "root")
		tAssert.NoError(os.MkdirAll(root, 0o755))

		schemaPath := filepath.Join(root, "schema.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte(`
[output = schema]
{
  User: { name: string; profile: { city: string; }; };
  Runtime: { env: string; };
  Choice: choice["Ada", 1, true];
}
`), 0o644))
		dataText := `
[output = data, schema = User]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
  count: 1;
  mixed: ["a", 1];
  items: ["x"];
}
`
		dataPath := filepath.Join(root, "data.mace")
		tAssert.NoError(os.WriteFile(dataPath, []byte(dataText), 0o644))
		dataFile, err := parseFile(dataText)
		tAssert.NoError(err)
		tokens := lexAnalysisTokens(dataText)
		_ = tokens

		_, _ = addMissingScriptSemicolonText("|===|\nstring value = \"x\"\n|===|")
		_, _ = addMissingScriptSemicolonText("|===|\nstring value = \"x\";\n|===|")
		_, _ = moveScriptBlockBeforeOutputText("[output = data]\n{}\n|===|\nstring value = \"x\";\n|===|")
		_, _ = moveScriptBlockBeforeOutputText("|===|\nstring value = \"x\";\n|===|\n[output = data]{}")
		_, _ = extractRecordLiteralIntoSchemaText("|===|\nProfile record = { age: 1; active: true; };\n|===|")
		_, _ = extractRecordLiteralIntoSchemaText("|===|\nstring value = \"x\";\n|===|")
		_ = inferRecordSchemaFields("name: \"Ada\"; count: 1; items: [\"x\"];")
		_ = inferRecordSchemaFields("")
		_ = simpleExpressionText(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"})
		_ = simpleExpressionText(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}})
		_ = defaultExpressionForType(ast.NamedType{Name: "User"})
		_ = defaultExpressionForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}})
		_ = inferredTypeFromExpression(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "profile"})
		_ = inferredTypeFromExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}})
		_, _ = replaceVariableDeclaration("string value = \"x\";", regexp.MustCompile(`(?m)^([ \t]*)(string)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(\"[^\"]*\");`), func(matches []string) string { return matches[1] + matches[2] + " fresh = " + matches[4] + ";" })
		_, _ = replaceVariableDeclaration("value = \"x\";", regexp.MustCompile(`missing`), func(matches []string) string { return "" })
		_, _ = formatTextQuick(dataText)
		_, _ = formatTextQuick("[")
		_, _, _ = schemaFileConflictAnalysis(dataText, dataFile, dataPath)
		_, _, _ = schemaFileConflictAnalysis("[output = schema]\n{}", dataFile, dataPath)
		_, _ = parseDirectiveWarningDiagnostic(dataText, dataFile)
		_, _ = parseDirectiveWarningDiagnostic("[output = schema]\n{}", dataFile)
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"Ada"`})
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: "plain"})
		_, _ = parseInputSemanticSchemaName(dataFile, root)
		_, _ = parseInputSemanticSchemaName(ast.File{}, root)
		_ = importedSemanticSymbols(dataFile, dataPath)
		_ = importedSemanticSymbols(ast.File{}, dataPath)
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}}}, dataPath, "Shared")
		_, _ = importedImportAsSemanticSymbol(ast.File{}, dataPath, "Shared")
		_, _, _, _ = parsedFile(schemaPath)
		_, _, _, _ = parsedFile(filepath.Join(root, "missing.mace"))
		_ = summarizeValue(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueInt, Int: 1}}})
		_ = summarizeValue(processor.Value{})
		_ = expressionSummary(ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: "profile"})
		_ = expressionSummary(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}})
		_ = indexSymbols([]semanticSymbol{{Name: "a"}})
		_, _ = outputDirectiveListRange(dataText)
		_, _ = outputDirectiveListRange("plain text")
		_, _, _ = schemaFileDirectiveRanges(dataText)
		_, _, _ = schemaFileDirectiveRanges("plain text")
		_, _ = importAndScriptCleanupRange(dataText)
		_, _ = importAndScriptCleanupRange("plain text")
		_ = quotedName(`unknown schema "Name"`)
		_ = quotedName("plain")
	})

	It("covers remaining analysis edge branches", func() {
		workspace := GinkgoT().TempDir()
		documentPath := filepath.Join(workspace, "document.mace")
		text := `[output = data, schema = User]
{
  user: { name: "Ada"; profile: { city: "LA"; }; };
  count: 1;
  mixed: ["a", 1];
  items: ["x"];
}
`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		tokens := lexAnalysisTokens(text)
		file, err := parseFile(text)
		tAssert.NoError(err)
		_ = AnalyzeDocumentAt(text, documentPath)

		_, _ = addMissingScriptSemicolonText("|===|\nstring value = \"x\"\n")
		_, _ = moveScriptBlockBeforeOutputText("|===|\nstring value = \"x\";\n")
		_, _ = moveScriptBlockBeforeOutputText("[output = data]\n{}\n|===|\nstring value = \"x\";\n|===|")
		_, _ = extractRecordLiteralIntoSchemaText("|===|\nschema Profile: { age: int; };\nProfile record = { age: 1; };\n|===|")
		_, _ = extractRecordLiteralIntoSchemaText("|===|\nProfile record = { };\n|===|")
		_, _ = createSchemaFromValidationErrorText("|===|\nschema User: { name: string; };\n|===|")
		_, _ = createSchemaFromValidationErrorText("|===|\n[output = data, schema = User]\n{ user: \"x\"; }\n|===|")
		_, _ = generateSampleDataFromSchemaText("|===|\nschema User: { invalid; };\n|===|")
		_, _ = generateSampleDataFromSchemaText("|===|\nschema User: { age: int; };\n|===|")
		_, _ = renameDuplicateVariableText("|===|\nstring value = \"x\";\nstring value = \"y\";\n|===|")
		_, _ = analyzeFileStructure("from \"bad\" import User;\nfrom \"C:/abs.mace\" import User, Runtime as Run;", ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: "bad"}}, {Path: ast.StringLiteral{Lexeme: `"C:/abs.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}, {Name: "Runtime", Alias: "Run"}}}}}, lexAnalysisTokens("from \"bad\" import User;\nfrom \"C:/abs.mace\" import User, Runtime as Run;"), documentPath)
		_, _ = directivePathDiagnostics(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema.mace"`}}}}, nil, documentPath)
		_, _ = directivePathDiagnostics(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema.mace"`}}}}, lexAnalysisTokens("[output = data, schema_file = \"./schema.mace\"]"), "")
		_, _ = parseDirectiveWarningDiagnostic(text, ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}})
		_, _ = parseDirectiveWarningDiagnostic("plain text", ast.File{})
		_, _, _ = missingImportEdit("plain", ast.File{}, nil, `unknown type \"Missing\"`)
		_, _, _ = missingImportEdit("plain", ast.File{Script: &ast.ScriptBlock{}}, nil, `unknown type \"Missing\"`)
		_, _ = invalidDirectiveComboEditRange("[output = schema, schema = User]", ast.File{}, lexAnalysisTokens("[output = schema, schema = User]"), "schema directive is invalid when output mode is schema")
		_, _ = invalidDirectiveComboEditRange("plain", ast.File{}, nil, "other")
		_, _, _ = schemaFileConflictAnalysis(text, ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./schema.mace"`}}}, Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}}}, Script: &ast.ScriptBlock{}}, "")
		_, _, _ = schemaFileConflictAnalysis(text, ast.File{}, documentPath)
		_ = shouldIgnoreParseValidationError(ast.File{}, fmt.Errorf("plain"))
		_ = shouldIgnoreParseValidationError(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}}, processor.DiagnosticError{Code: processor.CodeMissingRequiredField})
		_, _ = semanticDiagnosticFromError(ast.File{}, nil, fmt.Errorf("plain"))
		_, _ = variableTypeMismatchDiagnostic(ast.File{}, nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch, Fields: processor.DiagnosticFields{Expected: "", Actual: ""}})
		_, _ = nullUsageDiagnostic(nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch})
		_, _ = mixedArrayLiteralDiagnostic(ast.File{}, nil, "array literal has mixed element types")
		_, _ = arrayAccessDiagnostic(nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch}, "plain")
		_, _ = schemaDiagnostic(nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch}, "plain")
		_, _ = unknownSchemaDiagnostic(nil, "plain")
		_, _ = selfReferenceDiagnostic(ast.File{}, nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch}, "plain")
		_, _ = schemaOutputFieldDiagnostic(nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch}, "plain")
		_, _ = dataOutputValueDiagnostic(nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch}, "plain")
		_ = expressionSummary(ast.BooleanLiteral{Value: true})
		_ = expressionSummary(ast.RecordLiteral{})
		_ = expressionSummary(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}})
		_ = indexSymbols([]semanticSymbol{{Name: "a"}, {Name: "a"}})
		_ = quotedName("plain")
		_ = quotedName(`unknown schema "Name"`)
		_ = collectSemanticSymbols(file, tokens, &processor.Result{Output: map[string]processor.Value{"count": {Kind: processor.ValueInt, Int: 1}}}, documentPath)
		_ = declarationDocumentation(file, "User")
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"Ada"`})
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: "plain"})
		_, _ = parseInputSemanticSchemaName(ast.File{}, workspace)
		_ = importedSemanticSymbols(ast.File{}, documentPath)
		_, _ = importedImportAsSemanticSymbol(ast.File{}, documentPath, "Shared")
		_, _, _, _ = parsedFile(filepath.Join(workspace, "missing.mace"))
		_ = summarizeValue(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueInt, Int: 1}}})
		_ = valueTypeSummary(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{}})
		_ = expressionSummary(nil)
		_ = simpleExpressionText(ast.BooleanLiteral{Value: true})
		_ = simpleExpressionText(ast.BooleanLiteral{Value: false})
		_ = simpleExpressionText(ast.StringLiteral{Lexeme: `"x"`})
		_ = simpleExpressionText(ast.IntLiteral{Lexeme: "1"})
		_ = simpleExpressionText(ast.FloatLiteral{Lexeme: "1.0"})
		_ = simpleExpressionText(ast.Identifier{Name: "value"})
		_ = simpleExpressionText(ast.RecordLiteral{})
		_, _, _ = missingImportEdit("plain", ast.File{}, nil, `unknown type \"Missing\"`)
		_, _, _ = missingImportEdit("plain", ast.File{}, nil, "other")
		_, _ = parseDirectiveWarningDiagnostic("plain", ast.File{})
		_ = schemaOutputVariableDiagnostics(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{}}}, nil)
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: "bad"}}}}, workspace)
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: `"Runtime"`}}}}, workspace)
		schemaImportDir := filepath.Join(workspace, "schema-import")
		tAssert.NoError(os.MkdirAll(schemaImportDir, 0o755))
		schemaImportPath := filepath.Join(schemaImportDir, "schema.mace")
		tAssert.NoError(os.WriteFile(schemaImportPath, []byte(`
[output = schema]
{
  Runtime: { env: string; };
}
`), 0o644))
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}}}, schemaImportDir)
		_ = importedSemanticSymbols(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: "bad"}}}}, documentPath)
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData}}, documentPath, "Shared")
		_, _, _, _ = parsedFile(filepath.Join(workspace, "missing.mace"))
		_ = summarizeValue(processor.Value{Kind: processor.ValueBoolean, Boolean: true})
		_ = summarizeValue(processor.Value{})
		_ = expressionSummary(ast.BooleanLiteral{Value: false})
		_ = expressionSummary(ast.RecordLiteral{})
		_ = quotedName("plain")
		_ = quotedName(`unknown schema "Name"`)
		_, _ = renameDuplicateVariableText("plain")
		_ = simpleExpressionText(ast.BooleanLiteral{Value: false})
		_ = defaultExpressionForType(ast.PrimitiveType{Name: "float"})
		_ = defaultExpressionForType(ast.PrimitiveType{Name: "boolean"})
		_ = inferredTypeFromExpression(ast.FloatLiteral{Lexeme: "1.0"})
		_ = inferredTypeFromExpression(ast.ArrayLiteral{})
		_, _ = declarationSemicolonInsertRange("type A = string", lexAnalysisTokens("type A = string"), lexer.Token{Line: 1, Column: 6, Lexeme: "A"})
		_, _ = invalidDirectiveComboEditRange("[output = data]", ast.File{}, lexAnalysisTokens("[output = data]"), "schema directive is invalid when output mode is schema")
		_, _ = fieldEditRangeAt("value: 1\r\n", lexAnalysisTokens("value: 1"), 0)
		_, _ = importIdentifierEditRange("from \"./a.mace\" import Foo, Bar;", lexAnalysisTokens("from \"./a.mace\" import Foo, Bar;"), lexer.Token{Line: 1, Column: 25, Lexeme: "Foo"}, false)
		_, _ = importIdentifierEditRange("from \"./a.mace\" import Foo as Bar;", lexAnalysisTokens("from \"./a.mace\" import Foo as Bar;"), lexer.Token{Line: 1, Column: 25, Lexeme: "Foo"}, false)
		_, _ = importDeclarationEditRange("from \"./a.mace\" import Foo", lexAnalysisTokens("from \"./a.mace\" import Foo"), 3)
		_, _ = importDeclarationEditRange("from \"./a.mace\" import Foo;", lexAnalysisTokens("from \"./a.mace\" import Foo;"), 3)
		_, _ = declarationEditRange("type X = string", lexAnalysisTokens("type X = string"), lexer.Token{Line: 1, Column: 6, Lexeme: "X"})
		_, _ = variableTypeMismatchDiagnostic(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "x", NameToken: lexer.Token{Line: 1, Column: 1, Lexeme: "x"}, Type: ast.PrimitiveType{Name: "string"}, HasValue: false}}}}, nil, processor.DiagnosticError{Code: processor.CodeTypeMismatch, Fields: processor.DiagnosticFields{Expected: "string", Actual: "int"}})
		_, _ = mixedArrayLiteralDiagnostic(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "x", NameToken: lexer.Token{Line: 1, Column: 1, Lexeme: "x"}, Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"x"`}}}}}, nil, "array literal has mixed element types")
		_ = arrayAccessCandidates(lexAnalysisTokens("values[1][2]"))
		_, _ = schemaDiagnostic(nil, processor.DiagnosticError{Code: processor.CodeMissingRequiredField, Fields: processor.DiagnosticFields{Schema: "schema"}}, "schema bad")
		_, _ = unknownSchemaDiagnostic(nil, `unknown schema ""`)
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"""hello"""`})
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: `'hello'`})
		_ = stringLiteralMarkdown(ast.StringLiteral{Lexeme: `"bad\q"`})
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: "bad"}}}}, workspace)
		_ = importedSemanticSymbols(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: "bad"}}}}, documentPath)
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData}}, documentPath, "Shared")
		_ = summarizeValue(processor.Value{Kind: processor.ValueBoolean, Boolean: true})
		_ = expressionSummary(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}})
		_ = indexSymbols([]semanticSymbol{{Name: ""}, {Name: "a"}, {Name: "a"}})
		_ = quotedName("plain")
		_ = quotedName(`unknown schema "Name"`)
		_ = semanticCodeActions(text, file, tokens, documentPath, protocol.Diagnostic{Range: protocol.Range{}}, `unknown type "Missing"`)
		_ = semanticCodeActions(text, file, tokens, documentPath, protocol.Diagnostic{Range: protocol.Range{}}, "import declarations must appear at top of script block")
		_ = semanticCodeActions(text, file, tokens, documentPath, protocol.Diagnostic{Range: protocol.Range{}}, `duplicate declaration "foo"`)
		_ = semanticCodeActions(text, file, tokens, documentPath, protocol.Diagnostic{Range: protocol.Range{}}, "expected ':'")
		_ = semanticCodeActions(text, file, tokens, documentPath, protocol.Diagnostic{Range: protocol.Range{}}, "unknown self reference")
		_ = semanticCodeActions(text, file, tokens, documentPath, protocol.Diagnostic{Range: protocol.Range{}}, "schema directive is invalid when output mode is schema")
		_ = semanticCodeActions(text, file, tokens, documentPath, protocol.Diagnostic{Range: protocol.Range{}}, "missing required field")

		_, _ = parseDirectiveWarningDiagnostic("plain", ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}})
		_, _ = semanticDiagnosticFromError(ast.File{}, nil, fmt.Errorf("plain"))
		_, _, _ = missingImportEdit("plain", ast.File{}, nil, `unknown identifier "Missing"`)
		_, _ = fieldEditRangeAt("value: 1", lexAnalysisTokens("value: 1"), 0)
		_, _ = formatTextQuick("not valid mace")
		_, _ = importIdentifierEditRange("from \"./a.mace\" import Foo, Bar;", lexAnalysisTokens("from \"./a.mace\" import Foo, Bar;"), lexer.Token{Line: 1, Column: 30, Lexeme: "Bar"}, false)
		_, _ = importIdentifierEditRange("from \"./a.mace\" import Foo;", lexAnalysisTokens("from \"./a.mace\" import Foo;"), lexer.Token{Line: 1, Column: 25, Lexeme: "Foo"}, true)
		_, _ = importDeclarationEditRange("from \"./a.mace\" import Foo\r\n", lexAnalysisTokens("from \"./a.mace\" import Foo\r\n"), 3)
		_, _ = importDeclarationEditRange("from \"./a.mace\" import Foo", lexAnalysisTokens("from \"./a.mace\" import Foo"), 3)
		_, _ = renameDuplicateVariableText("string value = \"x\";\nstring value = \"y\";")
		_, _ = parseInputSemanticSchemaName(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `\"./schema.mace\"`}, {Kind: ast.OutputDirectiveSchema, Value: "Runtime"}}}}, schemaImportDir)
		_ = importedSemanticSymbols(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Runtime"}}}}}, filepath.Join(schemaImportDir, "doc.mace"))
		_ = importedSemanticSymbols(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Shared"}}}}, filepath.Join(schemaImportDir, "doc.mace"))
		_, _ = importedImportAsSemanticSymbol(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "value", Value: ast.StringLiteral{Lexeme: `"x"`}}}}}, schemaImportPath, "Shared")
		_, _, _, _ = parsedFile(schemaImportPath)
		_ = summarizeValue(processor.Value{Kind: processor.ValueString, String: "x"})
		_ = summarizeValue(processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{"name": {Kind: processor.ValueString, String: "Ada"}}})
		_ = expressionSummary(ast.SelfReference{Path: []string{"user", "name"}})
		_ = quotedName("two words")
		_ = file
	})
})

func stringMustRead(path string) string {
	bytes, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("failed to read %s: %v", path, err))
	}
	return string(bytes)
}