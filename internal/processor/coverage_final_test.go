package processor

import (
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

func TestCoverageFinalBranches(t *testing.T) {
	mustErr := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error")
		}
	}
	mustOK := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	workspace := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/schema.mace":
			_, _ = io.WriteString(w, `[output = schema]
{ Foo: Foo; Bar: Bar; }`)
		case "/zero.mace":
			_, _ = io.WriteString(w, `[output = schema]
{ Foo: Bar; }`)
		case "/data.mace":
			_, _ = io.WriteString(w, `[output = data]
{ value = "x"; }`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	writeFixture := func(name, contents string) string {
		path := filepath.Join(workspace, name)
		mustOK(os.MkdirAll(filepath.Dir(path), 0o755))
		mustOK(os.WriteFile(path, []byte(contents), 0o644))
		return path
	}

	schemaMatch := writeFixture("schema-match.mace", `[output = schema]
{ Foo: Foo; Bar: Bar; }`)
	schemaZero := writeFixture("schema-zero.mace", `[output = schema]
{ Foo: Bar; }`)
	schemaOne := writeFixture("schema-one.mace", `[output = schema]
{ Foo: Foo; }`)
	dataFile := writeFixture("data.mace", `[output = data]
{ value = "x"; }`)
	badFile := writeFixture("bad.mace", `not valid`)
	badLex := writeFixture("bad.lex.mace", `"`)
	importFile := writeFixture("imports.mace", `from "./schema-one.mace" import Foo;
[output = schema]
{ Foo: string; }`)

	schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}}
	ctx := newProcessContext(workspace, workspace)
	ctx.schemas.Add("User", schema)
	ctx.symbols.Add("User", symbolKindSchema)
	ctx.symbols.Add("Thing", symbolKindType)
	ctx.symbols.Add("Imported", symbolKindImport)
	ctx.types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
	ctx.symbols.Add("record", symbolKindVariable)
	ctx.variables.Add("record", valueType{kind: ValueRecord, schemaName: "User", record: &schema})
	ctx.environment.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	ctx.symbols.Add("scalar", symbolKindVariable)
	ctx.variables.Add("scalar", valueType{kind: ValueString})
	ctx.environment.Add("scalar", Value{Kind: ValueString, String: "Ada"})

	if got, ok, err := outputParseSchemaName(nil, ctx); err != nil || ok || got != "" {
		t.Fatalf("unexpected parse schema result: %q %v %v", got, ok, err)
	}
	if got, ok, err := outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, ctx); err != nil || !ok || got != "User" {
		t.Fatalf("unexpected parse directive result: %q %v %v", got, ok, err)
	}
	if got, ok, err := outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaZero))}}, ctx); err != nil || !ok || got != "__parse_file" {
		t.Fatalf("unexpected parse_file result: %q %v %v", got, ok, err)
	}
	if _, _, err := outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaMatch))}}, ctx); err == nil {
		t.Fatal("expected ambiguous parse_file error")
	}
	if _, _, err := outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaOne))}, {Kind: ast.OutputDirectiveSchema, Value: "User"}}, ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveOutputSchemaNames(nil, ast.OutputDirectiveSchemaFile, workspace, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(schemaOne))}}, ast.OutputDirectiveSchemaFile, workspace, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaZero))}}, ast.OutputDirectiveParseFile, workspace, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(dataFile))}}, ast.OutputDirectiveSchemaFile, workspace, workspace); err == nil {
		t.Fatal("expected schema output error")
	}
	if _, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(badFile))}}, ast.OutputDirectiveSchemaFile, workspace, workspace); err == nil {
		t.Fatal("expected parse error")
	}

	guarded := extractGuardedNames(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, Right: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"opt"`}, Right: ast.Identifier{Name: "record"}}}, map[string]struct{}{})
	if len(guarded) != 2 {
		t.Fatalf("expected guarded names, got %#v", guarded)
	}
	_ = extractGuardedNames(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"`}, Right: ast.Identifier{Name: "record"}}, map[string]struct{}{"record": {}})

	if err := validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, ctx.symbols, map[string]struct{}{"record": {}}, map[string]struct{}{"record": {}}); err != nil {
		t.Fatal(err)
	}
	if err := validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, ctx.symbols, map[string]struct{}{"record": {}}, map[string]struct{}{}); err == nil {
		t.Fatal("expected optional field access error")
	}
	if err := validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, Right: ast.BooleanLiteral{Value: true}}, Then: ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, Else: ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}}, ctx.symbols, map[string]struct{}{"record": {}}, map[string]struct{}{"record": {}}); err != nil {
		t.Fatal(err)
	}

	if err := validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected missing schema error")
	}
	if err := validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected duplicate field error")
	}
	if err := validateOutputSchema("User", []ast.OutputField{{Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected unknown output field error")
	}

	if err := validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, schema, "User", ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected duplicate record field error")
	}
	if err := validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "extra", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, schema, "User", ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected unknown record field error")
	}

	choice := valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}
	variant := valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}
	arrayType := valueType{kind: ValueArray, element: &valueType{kind: ValueString}}
	recordType := valueType{kind: ValueRecord, record: &schema}
	if err := validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, choice, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, arrayType, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}}, recordType, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected record literal error")
	}
	mustErr(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, variant, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil))
	if err := validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}, {kind: ValueString}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected ambiguous variant error")
	}

	if err := validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateEvaluatedOutputSchema("Missing", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected missing schema error")
	}
	if err := validateEvaluatedOutputSchema("User", map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected unknown field error")
	}

	if err := validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, recordType, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if err := validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, arrayType, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	mustErr(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueArray}, ctx.symbols, ctx.types, ctx.schemas, nil))

	if _, err := coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, arrayType, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := coerceEvaluatedValueAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, Value{Kind: ValueString, String: "Ada"}, arrayType, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}

	collectCtx := newProcessContext(workspace, workspace)
	collectCtx.schemas.Add("User", schema)
	collectCtx.symbols.Add("User", symbolKindSchema)
	collectCtx.symbols.Add("Thing", symbolKindType)
	collectCtx.symbols.Add("Imported", symbolKindImport)
	collectCtx.types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
	collectCtx.variables.Add("record", valueType{kind: ValueRecord, schemaName: "User", record: &schema})
	collectCtx.environment.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	collectCtx.variables.Add("scalar", valueType{kind: ValueString})
	collectCtx.environment.Add("scalar", Value{Kind: ValueString, String: "Ada"})
	if _, err := collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}, {Name: "count", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}}}, collectCtx); err != nil {
		t.Fatal(err)
	}
	mustErr(func() error {
		_, err := collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "other", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, collectCtx)
		return err
	}())
	if _, err := collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, collectCtx); err == nil {
		t.Fatal("expected collect error")
	}

	if _, err := schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.NamedType{Name: "User"}}, collectCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "count", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, collectCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, collectCtx); err != nil {
		t.Fatal(err)
	}

	if _, err := exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, collectCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := exportedOutputFieldType(ast.OutputField{Name: "other", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, collectCtx); err != nil {
		t.Fatal(err)
	}

	if _, err := resolveExportedTypeReference(ast.ArrayType{Element: ast.NamedType{Name: "Thing"}}, collectCtx.types, collectCtx.schemas, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveExportedTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "Thing"}}, collectCtx.types, collectCtx.schemas, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.NamedType{Name: "Missing"}}}, collectCtx.types, collectCtx.schemas, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveExportedTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Thing"}}}}, collectCtx.types, collectCtx.schemas, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveExportedTypeReference(ast.NamedType{Name: "User"}, collectCtx.types, collectCtx.schemas, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveExportedTypeReference(ast.NamedType{Name: "Thing"}, collectCtx.types, collectCtx.schemas, map[string]struct{}{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}

	mustOK(validateTypeReference(ast.ArrayType{Element: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx.symbols, ctx.types, ctx.schemas, nil))
	mustOK(validateTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "Thing"}}, ctx.symbols, ctx.types, ctx.schemas, nil))
	if err := validateTypeReference(ast.NamedType{Name: "Missing"}, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected missing type error")
	}
	if err := validateDocDeclaration(ast.DocDeclaration{Target: "User", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema}); err != nil {
		t.Fatal(err)
	}
	if err := validateDocDeclaration(ast.DocDeclaration{Target: "scalar", Kind: ast.DocumentationKindGeneral, Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"sum"`}, Description: &ast.StringLiteral{Lexeme: `"""desc"""`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"scalar": symbolKindVariable}); err != nil {
		t.Fatal(err)
	}

	if _, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "Foo", Type: ast.PrimitiveType{Name: "string"}}}}, ctx.types); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, ctx.types); err != nil {
		t.Fatal(err)
	}

	if _, err := evaluateExpression(ast.Identifier{Name: "record"}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluateExpression(ast.Identifier{Name: "User"}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected type reference error")
	}
	if _, err := evaluateExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}

	if _, err := parseHexFloat("0x1.8"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseInterpolatedString(`"hello $(record.name)"`, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := parseInterpolatedString(`"hello $("`, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected interpolation error")
	}
	if _, _, err := unescapeSequence(`\u0041`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := unescapeSequence(`\x`); err == nil {
		t.Fatal("expected escape error")
	}
	if _, err := parseUnicodeEscape(`\u0041`, 4); err != nil {
		t.Fatal(err)
	}
	if _, err := parseUnicodeEscape(`\u12`, 4); err == nil {
		t.Fatal("expected unicode error")
	}

	if _, err := evaluateArrayAccess(ast.ArrayAccess{Target: ast.Identifier{Name: "record"}, Index: ast.IntLiteral{Lexeme: "0"}}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected array access error")
	}
	if _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}

	if !typesEqual(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}) {
		t.Fatal("expected typesEqual")
	}
	if err := ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString}); err != nil {
		t.Fatal(err)
	}

	_, _ = resolveBoundedPath(workspace, workspace, filepath.Base(schemaOne))
	_, _ = resolveBoundedPath(server.URL+"/", server.URL+"/", "./schema.mace")
	_, _ = readMaceSource(schemaOne)
	_, _ = readMaceSource(server.URL + "/schema.mace")
	_, _ = loadImportExports(schemaOne, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, _ = loadSchemaFileDeclarations(schemaOne, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
	_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(schemaOne))}, {Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaOne))}}, workspace, workspace)
	_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(schemaOne))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, err := parseHexFloat("0x")
	mustErr(err)
	_, err = parseInterpolatedString(`"$("`, ctx.environment, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
	mustErr(err)
	_ = server
	_ = badLex
	_ = importFile
}

func TestCoverageRemainingHelperBranches(t *testing.T) {
	mustErr := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error")
		}
	}
	mustOK := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}

	vars := newVariableRegistry()
	symbols := newSymbolTable()
	types := newTypeRegistry()
	schemas := newSchemaRegistry()
	record := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}}
	schemas.Add("User", record)
	symbols.Add("User", symbolKindSchema)
	symbols.Add("Thing", symbolKindType)
	types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
	types.AddAlias("LoopA", ast.NamedType{Name: "LoopB"})
	types.AddAlias("LoopB", ast.NamedType{Name: "LoopA"})
	vars.Add("record", valueType{kind: ValueRecord, schemaName: "User", record: &record})
	vars.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
	vars.Add("choice", valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}})
	vars.Add("variant", valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})

	mustOK(validateExpressionAgainstType(ast.ArrayLiteral{}, valueType{kind: ValueArray}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}, vars, symbols, types, schemas, nil))
	mustErr(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil))
	mustErr(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}}, valueType{kind: ValueRecord, record: &record}, vars, symbols, types, schemas, nil))
	mustErr(validateExpressionAgainstType(ast.Identifier{Name: "missing"}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueString}}}, vars, symbols, types, schemas, nil))

	mustOK(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil))
	mustOK(validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil))
	mustOK(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil))
	mustErr(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "Missing"}, symbols, types, schemas, nil))
	mustErr(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &record}, symbols, types, schemas, nil))

	mustOK(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "opt", Optional: true, Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, record, "User", vars, symbols, types, schemas, nil))
	mustErr(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, record, "User", vars, symbols, types, schemas, nil))
	mustErr(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, record, "User", vars, symbols, types, schemas, nil))

	mustOK(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "opt", Optional: true, Value: ast.StringLiteral{Lexeme: `"Bea"`}}}, vars, symbols, types, schemas, nil))
	mustErr(validateOutputSchema("User", []ast.OutputField{{Name: "name", Optional: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))
	mustErr(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}, vars, symbols, types, schemas, nil))
	mustErr(validateOutputSchema("User", []ast.OutputField{{Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))

	mustOK(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
	mustErr(validateEvaluatedOutputSchema("Missing", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
	mustErr(validateEvaluatedOutputSchema("User", map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))

	_, err := resolveValueType(ast.ArrayType{Element: ast.NamedType{Name: "Thing"}}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = resolveValueType(ast.RecordMapType{Value: ast.NamedType{Name: "Thing"}}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = resolveValueType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = resolveValueType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = resolveValueType(ast.NamedType{Name: "User"}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = resolveValueType(ast.NamedType{Name: "Thing"}, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := resolveValueType(ast.NamedType{Name: "LoopA"}, symbols, types, schemas, nil); return err }())

	_, err = schemaTypeFromTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, types)
	mustOK(err)
	_, err = schemaTypeFromTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, types)
	mustOK(err)
	_, err = schemaTypeFromTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, types)
	mustOK(err)
	_, err = schemaTypeFromTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, types)
	mustOK(err)
	_, err = schemaTypeFromTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, types)
	mustOK(err)
	_, err = schemaTypeFromTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
	mustOK(err)
	_, err = schemaTypeFromTypeReference(ast.NamedType{Name: "Thing"}, types)
	mustOK(err)
	mustErr(func() error { _, err := schemaTypeFromTypeReference(nil, types); return err }())

	_, err = resolveUnionRecordType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, symbols, types, schemas)
	mustOK(err)
	mustErr(func() error { _, err := resolveUnionRecordType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, symbols, types, schemas); return err }())

	_, err = inferArrayLiteralType(ast.ArrayLiteral{}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferArrayLiteralType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.StringLiteral{Lexeme: `"Bea"`}}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenEOF, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil); return err }())
	_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())

	mustOK(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}}, vars, symbols, types, schemas, nil))
	mustErr(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueInt}, {kind: ValueInt}}, vars, symbols, types, schemas, nil))
	mustErr(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}, {kind: ValueString}}, vars, symbols, types, schemas, nil))

	_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := evaluateLogicalAnd(ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := evaluateLogicalOr(ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := evaluateConditional(ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())

	interpEnv := newValueEnvironment()
	interpEnv.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	_, err = parseInterpolatedString(`"hello"`, interpEnv, Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = parseInterpolatedString(`"hello $(record.name)"`, interpEnv, Value{}, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := parseInterpolatedString(`"hello $("`, interpEnv, Value{}, symbols, types, schemas, nil); return err }())
	_, err = parseHexFloat("0x1.8")
	mustOK(err)
	mustErr(func() error { _, err := parseHexFloat("0x"); return err }())
	_, err = parseUnicodeEscape(`\u0041`, 4)
	mustOK(err)
	mustErr(func() error { _, err := parseUnicodeEscape(`\u12`, 4); return err }())

	mustOK(ensureAssignable(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}))
	if !typesEqual(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}) {
		t.Fatal("expected typesEqual")
	}
	mustErr(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueInt}))
	if ok, err := valuesEqual(Value{Kind: ValueString, String: "Ada"}, Value{Kind: ValueString, String: "Ada"}); err != nil || !ok {
		t.Fatal("expected valuesEqual")
	}
	mustErr(func() error { _, err := valuesEqual(Value{Kind: ValueRecord, Record: map[string]Value{}}, Value{Kind: ValueRecord, Record: map[string]Value{}}); return err }())
	mustOK(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil))
	mustOK(validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil))
	mustErr(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "Missing"}, symbols, types, schemas, nil))

	vars.Add("mapRecord", valueType{kind: ValueRecord, element: &valueType{kind: ValueString}})
	vars.Add("emptyRecord", valueType{kind: ValueRecord})
	vars.Add("unknownArray", valueType{kind: ValueArray})
	vars.Add("unknownValue", valueType{kind: ValueUnknown})
	_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "mapRecord"}, Name: "name"}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "emptyRecord"}, Name: "name"}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "unknownValue"}, Name: "name"}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "array"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "unknownArray"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "unknownValue"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenMinus, Left: ast.IntLiteral{Lexeme: "2"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenStar, Left: ast.IntLiteral{Lexeme: "2"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenSlash, Left: ast.IntLiteral{Lexeme: "2"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenDoubleStar, Left: ast.IntLiteral{Lexeme: "2"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenShiftRight, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenShiftRightUnsigned, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenPipe, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenCaret, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	if !typesEqual(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}) {
		t.Fatal("expected choice typesEqual")
	}
	if !typesEqual(valueType{members: []valueType{{kind: ValueString}}}, valueType{members: []valueType{{kind: ValueString}}}) {
		t.Fatal("expected member typesEqual")
	}
	mustOK(ensureAssignable(valueType{kind: ValueString, nullable: true}, valueType{kind: ValueNull}))
	mustErr(ensureAssignable(valueType{members: []valueType{{kind: ValueString}}}, valueType{kind: ValueInt}))
	mustErr(ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueBoolean}))
	mustOK(ensureAssignable(valueType{kind: ValueUnknown}, valueType{kind: ValueString}))

	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustOK(func() error { _, err := evaluateHexNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexInt, Int: 2}); return err }())
	mustOK(func() error { _, err := evaluateHexNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 2.5}); return err }())
	mustErr(func() error { _, err := evaluateHexNumeric(lexer.TokenEOF, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexInt, Int: 2}); return err }())
	mustOK(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustOK(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustOK(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustOK(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenLessEqual, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustOK(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenGreaterEqual, Left: ast.IntLiteral{Lexeme: "2"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustOK(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustOK(func() error { _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := evaluateExpression(ast.Identifier{Name: "User"}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())
}
