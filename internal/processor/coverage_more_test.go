package processor

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/louiss0/mace/internal/parser/ast"
)

func TestCoverageMoreProcessorPaths(t *testing.T) {
	workspace := t.TempDir()

	mustWrite := func(name, content string) string {
		path := filepath.Join(workspace, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}

	mustWrite("schema.mace", "|===|\nschema User: { name: string, };\n|===|\n[output = schema]\n{ User: User, }\n")
	mustWrite("data.mace", "|===|\nstring name = \"Ada\";\n|===|\n[output = data]\n{ name: name, }\n")
	mustWrite("imported.mace", "|===|\nfrom \"./schema.mace\" import User;\n|===|\n[output = schema]\n{ User: User, }\n")

	proc := NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}})
	if _, err := proc.ProcessVariablesInScope("|===|\nstring name = \"Ada\";\n|===|\n[output = data]\n{ name: name, }", workspace, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := proc.ProcessFile(filepath.Join(workspace, "data.mace")); err != nil {
		t.Fatal(err)
	}
	if _, err := proc.ProcessFileInDir(filepath.Join(workspace, "schema.mace"), workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := proc.ProcessOutputBlock("[output = data]\n{ name: \"Ada\", }", ScriptResult{context: newProcessContext(workspace, workspace)}); err != nil {
		t.Fatal(err)
	}

	ctx := newProcessContext(workspace, workspace)
	ctx.symbols.Add("User", symbolKindSchema)
	ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
	ctx.variables.Add("name", valueType{kind: ValueString})
	ctx.environment.Add("name", Value{Kind: ValueString, String: "Ada"})

	if _, err := collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.NamedType{Name: "User"}}, {Name: "Attrs", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}}}, ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := collectImportExports(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(filepath.Join(workspace, "schema.mace")))}} , workspace, workspace); err != nil {
		t.Fatal(err)
	}
	if _, err := loadSchemaFileDeclarations(filepath.Join(workspace, "schema.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOutputSchemaRecord(filepath.Join(workspace, "schema.mace"), workspace, "schema_file"); err != nil {
		t.Fatal(err)
	}

	cache := map[string]map[string]importedDeclaration{}
	stack := map[string]struct{}{}
	if _, err := loadImportExports(filepath.Join(workspace, "schema.mace"), workspace, true, cache, stack); err != nil {
		t.Fatal(err)
	}
	if _, err := loadImportExports(filepath.Join(workspace, "schema.mace"), workspace, true, cache, stack); err != nil {
		t.Fatal(err)
	}
	if _, err := readMaceSource("http://127.0.0.1:1/missing.mace"); err == nil {
		t.Fatal("expected remote fetch error")
	}

	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote("./schema.mace")}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote("./schema.mace")}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}

	if err := validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil); err == nil {
		t.Fatal("expected duplicate output field error")
	}
	if err := validateDocDeclaration(ast.DocDeclaration{Target: "User", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema}); err != nil {
		t.Fatal(err)
	}
	interpEnv := newValueEnvironment()
	interpEnv.Add("name", Value{Kind: ValueString, String: "Ada"})
	if _, err := parseInterpolatedString(`"hello $(name)"`, interpEnv, Value{}, ctx.symbols, ctx.types, ctx.schemas, nil); err != nil {
		t.Fatal(err)
	}
}
