package processor

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Schemas", func() {
	It("accepts nullable primitive initializers", func() {
		_, err := New().ProcessInDir(wrapScriptWithOutput(`|===|
nullable string env = "dev";
|===|`), "../..")
		tAssert.NoError(err)
	})

	It("accepts schema member access in schema-validated output", func() {
		processor := New()
		_, err := processor.Process(`|===|
schema User: {
  id: string,
  name: string,
};

User user = {
  id: "user_1",
  name: "Ada",
};
|===|
[output = data, schema = User]
{
  id: user.id,
  name: user.name,
}`)
		tAssert.NoError(err)
	})

	It("validates parse input without exposing schema fields in the output block", func() {
		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		_, err := processor.Process(`|===|
schema Runtime: { env: string, };
|===|
[output = data, parse = Runtime]
{
  env: env,
}`)
		tAssert.ErrorContains(err, "unknown identifier")
	})

	It("rejects parse directives without required input fields", func() {
		processor := New()

		_, err := processor.Process(`|===|
schema Runtime: { env: string, };
|===|
[output = data, parse = Runtime]
{
  env: $env,
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "missing required field")
	})

	It("rejects parse directives with an unknown schema", func() {
		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		_, err := processor.Process(`[output = data, parse = MissingSchema] {}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "unknown schema")
	})

	It("rejects parse_file with a missing schema file", func() {
		processor := New()

		_, err := processor.ProcessInDir(`[output = data, parse_file = "./missing.mace"] {}`, ".")
		tAssert.Error(err)
		tAssert.ErrorContains(err, "unable to read import file")
	})

	It("validates parse_file input without exposing schema fields", func() {
		workspace, err := os.MkdirTemp("", "mace-parse-file-fixture-*")
		tAssert.NoError(err)
		defer func() {
			_ = os.RemoveAll(workspace)
		}()

		writeFixtureFile(workspace, "runtime.mace", `|===|
schema Runtime: { env: string, };
schema Meta: { source: string, };
|===|
[output = schema]
{
  Runtime: Runtime,
}`)

		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		_, err = processor.ProcessInDir(`[output = data, parse_file = "./runtime.mace"]
{
  env: env,
}`, workspace)
		tAssert.ErrorContains(err, "unknown identifier")
	})

	It("does not surface parsed schema fields as variables", func() {
		processor := NewWithInput(map[string]Value{
			"project": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "pi-prompt-form"},
				"root": {Kind: ValueString, String: "libs/pi-prompt-form"},
			}},
			"workspace": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "workspace"},
				"root": {Kind: ValueString, String: "."},
			}},
		})
		_, err := processor.ProcessFile("../../fixtures/processor/import_as/nx_consumer.mace")
		tAssert.ErrorContains(err, "unknown identifier")
	})

	It("does not expose record keyword schema fields as values", func() {
		processor := NewWithInput(map[string]Value{
			"record": {Kind: ValueString, String: "value"},
		})
		_, err := processor.Process(`|===|
schema Input: { record: string, };
|===|
[output = data, parse = Input]
{
  record: record,
}`)
		tAssert.ErrorContains(err, "unknown identifier")
	})

	It("resolves imported types in parse_file output schemas", func() {
		dir, err := os.MkdirTemp("", "mace-parse-file-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(dir) }()
		tAssert.NoError(os.WriteFile(filepath.Join(dir, "shared.mace"), []byte(`[output = schema]
{
  User: { name: string, },
}`), 0o644))
		tAssert.NoError(os.WriteFile(filepath.Join(dir, "schema.mace"), []byte(`|===|
from "./shared.mace" import User;
|===|
[output = schema]
{
  user: User,
}`), 0o644))

		processor := NewWithInput(map[string]Value{
			"user": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			}},
		})
		_, err = processor.ProcessInDir(`[output = data, parse_file = "./schema.mace"]
{
  name: user.name,
}`, dir)
		tAssert.ErrorContains(err, "unknown identifier")
	})

	Describe("parse input output scope", func() {
		const guardSchema = `|===|
schema User: {
  name: string,
  manager?: User,
};
|===|
`

		It("does not expose input for presence checks", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  has_manager: "manager" in input,
}`)
			tAssert.ErrorContains(err, "unknown identifier")
		})

		It("does not expose input when optional fields are absent", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  has_manager: "manager" in input,
}`)
			tAssert.ErrorContains(err, "unknown identifier")
		})

		It("rejects direct access to parsed optional fields", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: manager.name,
}`)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "unknown identifier")
			tAssert.ErrorContains(err, "manager")
		})

		It("rejects parsed optional fields inside guards", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input ? manager.name : "none",
}`)
			tAssert.ErrorContains(err, "unknown identifier")
		})

		It("rejects guards when parsed optional fields are absent", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input ? manager.name : "none",
}`)
			tAssert.ErrorContains(err, "unknown identifier")
		})

		It("does not expose the lowercase schema-name variable", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in user ? manager.name : "none",
}`)
			tAssert.ErrorContains(err, "unknown identifier")
		})

		It("does not expose nested parsed optional fields", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{
					"name":    {Kind: ValueString, String: "Bob"},
					"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Carol"}}},
				}},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input && "manager" in manager ? manager.manager.name : "none",
}`)
			tAssert.ErrorContains(err, "unknown identifier")
		})
	})

})

var _ = Describe("Schema and processor helpers", func() {
	It("reports diagnostic helper details", func() {
		kindName := directiveKindName
		cause := errors.New("root cause")
		err := DiagnosticError{Message: "wrapped", Cause: cause}

		tAssert.Equal(cause, errors.Unwrap(err))
		tAssert.Equal("missing required field \"name\"", strings.TrimPrefix(missingRequiredFieldError("name", "").Error(), "processor: "))
		tAssert.Equal("output", kindName(ast.OutputDirectiveOutput))
		tAssert.Equal("schema_file", kindName(ast.OutputDirectiveSchemaFile))
		tAssert.Equal("schema", kindName(ast.OutputDirectiveSchema))
		tAssert.Equal("parse", kindName(ast.OutputDirectiveParse))
		tAssert.Equal("parse_file", kindName(ast.OutputDirectiveParseFile))
		tAssert.Equal("unknown", kindName(ast.OutputDirectiveKind(99)))
		tAssert.Equal(ErrorDoc, inferErrorKind("documentation block"))
		tAssert.Equal(ErrorImport, inferErrorKind("import path"))
		tAssert.Equal(ErrorDirective, inferErrorKind("directive mismatch"))
		tAssert.Equal(ErrorDeclaration, inferErrorKind("type alias declaration"))
		tAssert.Equal(ErrorOperator, inferErrorKind("operator operands"))
		tAssert.Equal(ErrorType, inferErrorKind("unknown type reference"))
		tAssert.Equal(ErrorSchema, inferErrorKind("schema field"))
		tAssert.Equal(ErrorRuntime, inferErrorKind("runtime failure"))
		tAssert.Equal(ErrorValue, inferErrorKind("literal value expression"))
		tAssert.Equal(ErrorInternal, inferErrorKind("something else"))
	})

	It("validates schema loading and import edge cases", func() {
		workspace, err := os.MkdirTemp("", "processor-gap-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		schemaPath := filepath.Join(workspace, "schema.mace")
		badPath := filepath.Join(workspace, "bad.mace")
		dataPath := filepath.Join(workspace, "data.mace")
		missingPath := filepath.Join(workspace, "missing.mace")
		tAssert.NoError(os.WriteFile(schemaPath, []byte("[output = schema]\n{ User: { name: string, }, }\n"), 0o600))
		tAssert.NoError(os.WriteFile(badPath, []byte("[output = data]\n{ name: \"Ada\", }\n"), 0o600))
		tAssert.NoError(os.WriteFile(dataPath, []byte("not valid"), 0o600))
		circularPath := filepath.Join(workspace, "circular.mace")
		tAssert.NoError(os.WriteFile(circularPath, []byte("from \"./circular.mace\" import User;\n[output = schema]\n{ User: string, }"), 0o600))

		_, err = loadOutputSchemaRecord(schemaPath, workspace, "schema_file")
		tAssert.NoError(err)
		_, err = loadOutputSchemaRecord(badPath, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(dataPath, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(missingPath, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(circularPath, workspace, "schema_file")
		tAssert.Error(err)

		cache := map[string]map[string]ast.Declaration{}
		stack := map[string]struct{}{}
		_, err = loadSchemaFileDeclarations(schemaPath, workspace, cache, stack)
		tAssert.NoError(err)
		cache[schemaPath] = map[string]ast.Declaration{}
		_, err = loadSchemaFileDeclarations(schemaPath, workspace, cache, stack)
		tAssert.NoError(err)
		_, err = loadSchemaFileDeclarations(missingPath, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(circularPath, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)

		ctx := newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.variables.Add("name", valueType{kind: ValueString})
		ctx.environment.Add("name", Value{Kind: ValueString, String: "Ada"})
		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}, {Name: "count", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}}}, ctx)
		_, err = importFileAsDeclaration("Local", map[string]importedDeclaration{"bad": {kind: symbolKindImport}})
		tAssert.Error(err)
		_, err = parseImportPath(ast.StringLiteral{Lexeme: `not-a-string`})
		tAssert.Error(err)
		proc := NewWithInput(map[string]Value{"broken": {Kind: ValueString, String: "x"}})
		ctx = newProcessContext(workspace, workspace)
		ctx.schemas.Add("Broken", ast.RecordType{Fields: []ast.SchemaField{{Name: "broken", Type: nil}}})
		ctx.symbols.Add("Broken", symbolKindSchema)
		err = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Broken"}}}, &ctx)
		tAssert.Error(err)
		_, _ = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ctx)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}}}, ctx)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "count", Type: ast.RecordMapType{Value: ast.NamedType{Name: "Missing"}}}, ctx)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "broken", Type: nil}, ctx)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "other", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, ctx)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote("schema.mace")}}, workspace, workspace)
	})
})
