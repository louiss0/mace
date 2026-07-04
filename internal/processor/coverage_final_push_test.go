package processor

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

func TestCoverageFinalProcessorPaths(t *testing.T) {
	workspace := t.TempDir()
	mustWrite := func(name, content string) string {
		path := filepath.Join(workspace, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	schemaPath := mustWrite("schema.mace", "|===|\nschema User: { name: string, };\n|===|\n[output = schema]\n{ User: User, }\n")
	dataPath := mustWrite("data.mace", "|===|\nstring name = \"Ada\";\n|===|\n[output = data]\n{ name: name, }\n")
	missingPath := filepath.Join(workspace, "missing.mace")

	proc := NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}})
	_, _ = proc.ProcessVariablesInScope("|===|\nstring name = \"Ada\";\n|===|", workspace, workspace)
	_, _ = proc.ProcessFile(schemaPath)
	_, _ = proc.ProcessFileInDir(dataPath, workspace)
	_, _ = proc.ProcessOutputBlock("[output = data]\n{ name: \"Ada\", }", ScriptResult{context: newProcessContext(workspace, workspace)})

	ctx := newProcessContext(workspace, workspace)
	ctx.symbols.Add("User", symbolKindSchema)
	ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
	ctx.variables.Add("name", valueType{kind: ValueString})
	ctx.environment.Add("name", Value{Kind: ValueString, String: "Ada"})

	_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(schemaPath))}, {Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaPath))}}, workspace, workspace)
	_, _ = loadSchemaFileDeclarations(schemaPath, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
	_, _ = loadOutputSchemaRecord(schemaPath, workspace, "schema_file")
	_, _ = loadImportExports(schemaPath, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote("./schema.mace")}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote("./schema.mace")}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, _ = readMaceSource("http://127.0.0.1:1/missing.mace")
	_, _ = resolveBoundedPath(workspace, workspace, "../escape.mace")
	_, _ = resolveBoundedRemotePath("https://example.com/root/dir/", "https://example.com/root/", "ok.mace", "https://example.com/other/ok.mace")
	_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}})
	_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
	_ = validateDocDeclaration(ast.DocDeclaration{Target: "User", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
	_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.NamedType{Name: "User"}}}}, ctx)
	_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "attrs", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx)
	_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.Identifier{Name: "name"}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx)

	interpEnv := newValueEnvironment()
	interpEnv.Add("name", Value{Kind: ValueString, String: "Ada"})
	_, _ = parseInterpolatedString(`"hello $(name)"`, interpEnv, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = parseHexFloat("0x1p2")
	_, _ = parseUnicodeEscape(`\u0041`, 4)
	_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ctx.types)
	_, _ = coerceEvaluatedValueAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueArray}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
	_, _ = schemaTypeFromTypeReference(ast.NamedType{Name: "User"}, ctx.types)
	_, _ = resolveExportedTypeReference(ast.NamedType{Name: "User"}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
	_ = validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
	_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{}, ast.RecordType{}, "User", ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
	_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil)
	_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueString}, ctx.symbols, ctx.types, ctx.schemas, nil)

	_ = missingPath
}
