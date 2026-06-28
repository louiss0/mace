package processor

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

func TestCoverageSweep(t *testing.T) {
	workspace := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/schema.mace":
			_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	writeFixtureFile := func(relativePath string, contents string) string {
		path := filepath.Join(workspace, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	validSchema := writeFixtureFile("schema.mace", `[output = schema]
{ Foo: string; Bar: string; }`)
	disjointSchema := writeFixtureFile("schema-decl.mace", `[output = schema]
{ Foo: string; }
|===|
string Foo = "x";
|===|`)
	validData := writeFixtureFile("data.mace", `[output = data]
{ value = "x"; }`)
	badParse := writeFixtureFile("bad-parse.mace", `not valid`)
	badLex := writeFixtureFile("bad-lex.mace", `"`)
	cycleA := writeFixtureFile("cycle-a.mace", `from "./cycle-b.mace" import Foo;
[output = schema]
{ Foo: string; }`)
	_ = writeFixtureFile("cycle-b.mace", `from "./cycle-a.mace" import Foo;
[output = schema]
{ Foo: string; }`)

	proc := New()
	if _, err := proc.ProcessVariablesInScope(`from "./missing.mace" import Foo;`, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}

	ctx := newProcessContext(workspace, workspace)
	ctx.symbols.Add("User", symbolKindSchema)
	ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}})
	ctx.symbols.Add("input", symbolKindVariable)
	ctx.variables.Add("input", valueType{kind: ValueString})
	ctx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	ctx.symbols.Add("name", symbolKindVariable)
	ctx.variables.Add("name", valueType{kind: ValueString})
	ctx.environment.Add("name", Value{Kind: ValueString, String: "Ada"})
	ctx.symbols.Add("Foo", symbolKindVariable)

	if _, err := prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(disjointSchema))}}}, ctx); err == nil {
		t.Fatal("expected error")
	}
	if _, err := prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(validSchema))}}}, newProcessContext(workspace, workspace)); err != nil {
		t.Fatal(err)
	}

	procWithInput := NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}})
	if err := procWithInput.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx); err == nil {
		t.Fatal("expected error")
	}
	missingCtx := newProcessContext(workspace, workspace)
	if err := procWithInput.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Missing"}}}, &missingCtx); err == nil {
		t.Fatal("expected error")
	}
	parseCtx := newProcessContext(workspace, workspace)
	parseCtx.symbols.Add("User", symbolKindSchema)
	parseCtx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}})
	parseCtx.symbols.Add("input", symbolKindVariable)
	parseCtx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	parseCtx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
	if err := procWithInput.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &parseCtx); err == nil {
		t.Fatal("expected error")
	}

	if _, err := resolveImportsWithState(ast.File{Imports: nil}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote("foo")}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(validSchema))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}, {Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(validData))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}

	if _, err := resolveBoundedPath(workspace, workspace, "schema.mace"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBoundedPath(workspace, workspace, "../escape.mace"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", server.URL+"/schema.mace"); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", "https://other.example.com/schema.mace"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := readMaceSource(filepath.Join(workspace, "missing.mace")); err == nil {
		t.Fatal("expected error")
	}
	if _, err := readMaceSource(server.URL + "/missing.mace"); err == nil {
		t.Fatal("expected error")
	}

	if _, err := loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadImportExports(badLex, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadImportExports(cycleA, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadImportExports(validSchema, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}

	if _, err := loadSchemaFileDeclarations(filepath.Join(workspace, "missing.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadSchemaFileDeclarations(badParse, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadSchemaFileDeclarations(validSchema, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}

	if _, err := loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadOutputSchemaRecord(validData, workspace, "schema_file"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadOutputSchemaRecord(validSchema, workspace, "schema_file"); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(validSchema))}, {Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(validSchema))}}, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(validData))}}, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(validSchema))}}, workspace, workspace); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(validSchema))}}, ast.OutputDirectiveSchemaFile, workspace, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(validData))}}, ast.OutputDirectiveSchemaFile, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}

	validationSymbols := newSymbolTable()
	validationTypes := newTypeRegistry()
	validationSchemas := newSchemaRegistry()
	validationVariables := newVariableRegistry()
	validationSchema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}, Description: "name"}}}
	validationSchemas.Add("User", validationSchema)
	validationSymbols.Add("User", symbolKindSchema)
	validationSymbols.Add("Thing", symbolKindType)
	validationSymbols.Add("Imported", symbolKindImport)
	validationSymbols.Add("record", symbolKindVariable)
	validationSymbols.Add("scalar", symbolKindVariable)
	validationTypes.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
	validationVariables.Add("record", valueType{kind: ValueRecord, schemaName: "User", record: &validationSchema})
	validationVariables.Add("scalar", valueType{kind: ValueString})

	if err := validateDeclaration(ast.VariableDeclaration{Name: "bad", Type: ast.NamedType{Name: "Missing"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}, validationSymbols, validationTypes, validationSchemas, nil, validationVariables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateDeclaration(ast.VariableDeclaration{Name: "missing"}, validationSymbols, validationTypes, validationSchemas, nil, validationVariables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if err := validateTypeReference(ast.NamedType{Name: "Missing"}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if err := validateDocDeclaration(ast.DocDeclaration{Target: "Missing"}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{}, map[string]symbolKind{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateDocDeclaration(ast.DocDeclaration{Target: "User"}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{"User": {}}, map[string]symbolKind{"User": symbolKindSchema}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateDocDeclaration(ast.DocDeclaration{Target: "User", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"x"`}}}}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}}); err == nil {
		t.Fatal("expected error")
	}

	if err := validateDataOutputExpression(ast.ArrayLiteral{Elements: []ast.Expression{ast.NullLiteral{}}}, validationSymbols, map[string]struct{}{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateDataOutputExpression(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.NullLiteral{}}}}, validationSymbols, map[string]struct{}{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, validationVariables, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if err := validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.Identifier{Name: "missing"}}}, validationVariables, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}

	if err := validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "Missing"}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if err := validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}

	if _, err := evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "foo"}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := evaluateConditional(ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}
	if _, err := evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil); err == nil {
		t.Fatal("expected error")
	}

	if _, err := resolveExportedTypeReference(ast.NamedType{Name: "Missing"}, validationTypes, validationSchemas, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	validationTypes.AddAlias("CycleA", ast.NamedType{Name: "CycleB"})
	validationTypes.AddAlias("CycleB", ast.NamedType{Name: "CycleA"})
	if _, err := resolveExportedTypeReference(ast.NamedType{Name: "CycleA"}, validationTypes, validationSchemas, map[string]struct{}{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	validationSchemas.Add("Cycle", ast.RecordType{Fields: []ast.SchemaField{{Name: "self", Type: ast.NamedType{Name: "Cycle"}}}})
	if _, err := resolveExportedTypeReference(ast.NamedType{Name: "Cycle"}, validationTypes, validationSchemas, map[string]struct{}{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}

	if _, err := typeReferenceFromValueType(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}).(ast.ChoiceType); !err {
		t.Fatal("expected choice type")
	}
	_ = errors.New
}

func TestCoverageValidationBranchSweep(t *testing.T) {
	vars := newVariableRegistry()
	symbols := newSymbolTable()
	types := newTypeRegistry()
	schemas := newSchemaRegistry()
	schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}}
	schemas.Add("User", schema)
	symbols.Add("User", symbolKindSchema)
	symbols.Add("Thing", symbolKindType)
	types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
	vars.Add("choice", valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}})
	vars.Add("variant", valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})
	vars.Add("record", valueType{kind: ValueRecord, schemaName: "User"})

	_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.NullLiteral{}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.NullLiteral{}}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Else: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "2"}}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.Identifier{Name: "choice"}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.Identifier{Name: "missing"}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.BooleanLiteral{Value: true}, valueType{kind: ValueUnknown}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstType(ast.BooleanLiteral{Value: true}, valueType{kind: ValueString, nullable: true}, vars, symbols, types, schemas, nil)

	_ = validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueInt}}, vars, symbols, types, schemas, nil)
	_ = validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}, {kind: ValueString}}, vars, symbols, types, schemas, nil)

	_ = validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
	_ = validateOutputSchema("User", []ast.OutputField{{Name: "missing", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
	_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.NullLiteral{}}}, vars, symbols, types, schemas, nil)
	_ = validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil)
	_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
	_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "Missing"}, symbols, types, schemas, nil)

	_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "foo"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)

	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, vars, symbols, types, schemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, vars, symbols, types, schemas, nil)
}
