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

func TestCoverageTargetedBranches(t *testing.T) {
	workspace := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/schema.mace":
			_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
		case "/nested/schema.mace":
			_, _ = io.WriteString(w, `[output = schema]
{ Nested: string; }`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	writeFile := func(relativePath, contents string) string {
		path := filepath.Join(workspace, relativePath)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	goodSchema := writeFile("schema.mace", `[output = schema]
{ Foo: string; Bar: string; }`)
	goodSchemaWithScript := writeFile("schema-script.mace", `|===|
type Foo: string;
|===|
[output = schema]
{ Foo: string; }`)
	goodData := writeFile("data.mace", `[output = data]
{ value = "x"; }`)
	badParse := writeFile("bad-parse.mace", `not valid`)
	badLex := writeFile("bad-lex.mace", `"`)
	missingImport := writeFile("missing-imports.mace", `|===|
from "./missing.mace" import Foo;
|===|
[output = schema]
{ Foo: string; }`)
	escapeImport := writeFile("escape-import.mace", `|===|
from "../escape.mace" import Foo;
|===|
[output = schema]
{ Foo: string; }`)
	dupAliasImport := writeFile("dup-alias-import.mace", `|===|
from "./schema.mace" import Foo as Alias;
from "./schema.mace" import Bar as Alias;
|===|
[output = schema]
{ Foo: string; }`)
	parseSchema := writeFile("parse-schema.mace", `|===|
type Foo: string;
type Bar: string;
|===|
[output = schema]
{ Foo: Foo; Bar: Bar; }`)

	proc := New()
	if _, err := proc.ProcessVariablesInScope(`not valid`, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}
	if _, err := proc.ProcessVariablesInScope(`from "./missing.mace" import Foo;`, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}

	ctx := newProcessContext(workspace, workspace)
	ctx.symbols.Add("Foo", symbolKindVariable)
	ctx.symbols.Add("User", symbolKindSchema)
	ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
	ctx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User", record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}})
	ctx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})

	if _, err := prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(goodSchemaWithScript))}}}, ctx); err == nil {
		t.Fatal("expected error")
	}
	if _, err := prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(goodSchema))}}}, newProcessContext(workspace, workspace)); err != nil {
		t.Fatal(err)
	}

	procWithInput := NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}})
	dupFieldCtx := newProcessContext(workspace, workspace)
	dupFieldCtx.symbols.Add("User", symbolKindSchema)
	dupFieldCtx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
	dupFieldCtx.symbols.Add("name", symbolKindVariable)
	dupFieldCtx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
	dupFieldCtx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	if err := procWithInput.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &dupFieldCtx); err == nil {
		t.Fatal("expected error")
	}

	nilTypeCtx := newProcessContext(workspace, workspace)
	nilTypeCtx.symbols.Add("User", symbolKindSchema)
	nilTypeCtx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: nil}}})
	nilTypeCtx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
	nilTypeCtx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	if err := procWithInput.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &nilTypeCtx); err == nil {
		t.Fatal("expected error")
	}
	badFieldTypeCtx := newProcessContext(workspace, workspace)
	badFieldTypeCtx.symbols.Add("User", symbolKindSchema)
	badFieldTypeCtx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
	badFieldTypeCtx.symbols.Add("name", symbolKindVariable)
	badFieldTypeCtx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
	badFieldTypeCtx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	if err := procWithInput.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &badFieldTypeCtx); err == nil {
		t.Fatal("expected error")
	}

	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote("foo.txt")}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(goodSchema))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}, {Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(goodSchema))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(missingImport))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(escapeImport))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(dupAliasImport))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}

	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(goodData))}}, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(goodData))}}, workspace, workspace); err == nil {
		t.Fatal("expected error")
	}
	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(goodSchema))}}, workspace, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(parseSchema))}}, workspace, workspace); err != nil {
		t.Fatal(err)
	}

	if _, err := loadOutputSchemaRecord(goodData, workspace, "schema_file"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadOutputSchemaRecord(badParse, workspace, "schema_file"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file"); err == nil {
		t.Fatal("expected error")
	}

	if _, err := loadSchemaFileDeclarations(badLex, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadSchemaFileDeclarations(badParse, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := loadSchemaFileDeclarations(missingImport, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err == nil {
		t.Fatal("expected error")
	}

	collectCtx := newProcessContext(workspace, workspace)
	collectCtx.symbols.Add("User", symbolKindSchema)
	collectCtx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
	if _, err := collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, collectCtx); err != nil {
		t.Fatal(err)
	}
	if _, err := collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, newProcessContext(workspace, workspace)); err == nil {
		t.Fatal("expected error")
	}

	if _, err := schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: nil}, newProcessContext(workspace, workspace)); err == nil {
		t.Fatal("expected error")
	}
	if _, err := exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, newProcessContext(workspace, workspace)); err == nil {
		t.Fatal("expected error")
	}

	validationSymbols := newSymbolTable()
	validationTypes := newTypeRegistry()
	validationSchemas := newSchemaRegistry()
	validationVariables := newVariableRegistry()
	validationSchema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}}
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
	if err := validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"doc"`}}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}}); err == nil {
		t.Fatal("expected error")
	}
	if err := validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}); err == nil {
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
	if _, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, validationTypes); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, validationTypes); err != nil {
		t.Fatal(err)
	}

	_, _ = parseHexFloat("0x1.8")
	_, _ = parseHexFloat("0x1")
	_, _ = parseInterpolatedString(`"hello $(1)"`, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = parseInterpolatedString(`"hello $(name)"`, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _, _ = unescapeSequence(`\u0041`)
	_, _, _ = unescapeSequence(`\x`)
	_, _ = parseUnicodeEscape(`\u0041`, 4)
	_, _ = parseUnicodeEscape(`\u0`, 4)
	_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenCaret, Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, validationVariables, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, validationVariables, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, validationVariables, validationSymbols, validationTypes, validationSchemas, nil)
	_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, validationVariables, validationSymbols, validationTypes, validationSchemas, nil)
	_ = typesEqual(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
	_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString})
	_ = ensureAssignable(valueType{kind: ValueString, nullable: true}, valueType{kind: ValueNull})
	_ = ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"})
}
