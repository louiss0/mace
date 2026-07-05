package processor

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Schemas", func() {
	DescribeTable("accepts schema record literals",
		func(input string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)
		},
		Entry("optional fields omitted", wrapScriptWithOutput(`|===|
schema User: { name: string; age?: int; };
User user = { name: "Ada"; };
|===|`)),
		Entry("array of schema records", wrapScriptWithOutput(`|===|
schema Point: { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; y: 4; }
];
|===|`)),
		Entry("nullable string initializer", wrapScriptWithOutput(`|===|
nullable string env = "dev";
|===|`)),
	)

	DescribeTable("rejects schema record literal mismatches",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", wrapScriptWithOutput(`|===|
schema User: { name: string; age: int; };
User user = { name: "Ada"; };
|===|`), "missing required field"),
		Entry("unknown field", wrapScriptWithOutput(`|===|
schema User: { name: string; };
User user = { name: "Ada"; age: 30; };
|===|`), "unknown field"),
		Entry("optional field mismatch", wrapScriptWithOutput(`|===|
schema User: { name: string; age: int; };
User user = { name: "Ada"; age?: 30; };
|===|`), "not optional"),
		Entry("field type mismatch", wrapScriptWithOutput(`|===|
schema User: { name: string; age: int; };
User user = { name: 5; age: 30; };
|===|`), "type mismatch"),
		Entry("array element schema mismatch", wrapScriptWithOutput(`|===|
schema Point: { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; }
];
|===|`), "missing required field"),
	)

	It("accepts schema member access in schema-validated output", func() {
		processor := New()
		_, err := processor.Process(`|===|
schema User: {
  id: string;
  name: string;
};

User user = {
  id: "user_1";
  name: "Ada";
};
|===|
[output = data, schema = User]
{
  id: user.id;
  name: user.name;
}`)
		tAssert.NoError(err)
	})

	It("uses parse input to expose schema fields in the output block", func() {
		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		result, err := processor.Process(`|===|
schema Runtime: { env: string; };
|===|
[output = data, parse = Runtime]
{
  env: env;
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "env")
		assertExpectedValue(actual, expectedValue{kind: ValueString, string: "prod"})
	})

	It("rejects parse directives without required input fields", func() {
		processor := New()

		_, err := processor.Process(`|===|
schema Runtime: { env: string; };
|===|
[output = data, parse = Runtime]
{
  env: env;
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

	It("uses parse_file without a schema directive when one schema is available", func() {
		workspace, err := os.MkdirTemp("", "mace-parse-file-fixture-*")
		tAssert.NoError(err)
		defer func() {
			_ = os.RemoveAll(workspace)
		}()

		writeFixtureFile(workspace, "runtime.mace", `|===|
schema Runtime: { env: string; };
schema Meta: { source: string; };
|===|
[output = schema]
{
  Runtime: Runtime;
}`)

		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		result, err := processor.ProcessInDir(`[output = data, parse_file = "./runtime.mace"]
{
  env: env;
}`, workspace)
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "env"), expectedValue{kind: ValueString, string: "prod"})
	})

	It("surfaces only top-level parsed schema fields as variables", func() {
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
		result, err := processor.ProcessFile("../../fixtures/processor/import_as/nx_consumer.mace")
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "pi-prompt-form"})
		assertExpectedValue(result.Output["root"], expectedValue{kind: ValueString, string: "libs/pi-prompt-form"})
		assertExpectedValue(result.Output["cwd"], expectedValue{kind: ValueString, string: "."})
	})

	It("validates arbitrary record keys against a record value type", func() {
		input := `|===|
type Dependencies: record<string>;
schema PackageJSON: {
  name: string,
  dependencies: Dependencies,
}
|===|
[schema=PackageJSON]
{
  name: "pkg",
  dependencies: {
    pi_prompt_guard: "^1.0.0",
    pi_prompt_form: "^1.0.0",
  },
}`
		result, err := New().Process(input)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["dependencies"], expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"pi_prompt_guard": {kind: ValueString, string: "^1.0.0"},
			"pi_prompt_form":  {kind: ValueString, string: "^1.0.0"},
		}})
	})

	It("allows record keyword schema fields to be referenced as values", func() {
		processor := NewWithInput(map[string]Value{
			"record": {Kind: ValueString, String: "value"},
		})
		result, err := processor.Process(`|===|
schema Input: { record: string; };
|===|
[output = data, parse = Input]
{
  record: record;
}`)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["record"], expectedValue{kind: ValueString, string: "value"})
	})

	It("infers member access types for record map values", func() {
		input := `|===|
record<string> deps = { foo: "bar"; };
string foo = deps.foo;
|===|
[output = data]
{
  foo: foo;
}`
		result, err := New().Process(input)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["foo"], expectedValue{kind: ValueString, string: "bar"})
	})

	It("resolves imported types in parse_file output schemas", func() {
		dir, err := os.MkdirTemp("", "mace-parse-file-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(dir) }()
		tAssert.NoError(os.WriteFile(filepath.Join(dir, "shared.mace"), []byte(`[output = schema]
{
  User: { name: string; };
}`), 0o644))
		tAssert.NoError(os.WriteFile(filepath.Join(dir, "schema.mace"), []byte(`|===|
from "./shared.mace" import User;
|===|
[output = schema]
{
  user: User;
}`), 0o644))

		processor := NewWithInput(map[string]Value{
			"user": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			}},
		})
		result, err := processor.ProcessInDir(`[output = data, parse_file = "./schema.mace"]
{
  name: user.name;
}`, dir)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "Ada"})
	})

	Describe("optional field presence guards", func() {
		const guardSchema = `|===|
schema User: {
  name: string;
  manager?: User;
};
|===|
`

		It("evaluates 'in' expression to true when optional field exists in input", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  has_manager: "manager" in input,
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["has_manager"], expectedValue{kind: ValueBoolean, bool: true})
		})

		It("evaluates 'in' expression to false when optional field is absent from input", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  has_manager: "manager" in input,
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["has_manager"], expectedValue{kind: ValueBoolean, bool: false})
		})

		It("rejects unguarded member access on optional parse variable", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: manager.name,
}`)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "optional field")
			tAssert.ErrorContains(err, "manager")
		})

		It("allows member access on optional parse variable inside 'in' guard", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input ? manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "Bob"})
		})

		It("uses the else branch when the guarded optional field is absent", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input ? manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "none"})
		})

		It("supports 'in' guards with the lowercase schema-name variable", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in user ? manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "Bob"})
		})

		It("validates nested optional access with nested 'in' guards via &&", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{
					"name":    {Kind: ValueString, String: "Bob"},
					"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Carol"}}},
				}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input && "manager" in manager ? manager.manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "Carol"})
		})
	})

	It("rejects record values that do not match the record value type", func() {
		input := `|===|
type Dependencies: record<string>;
schema PackageJSON: {
  dependencies: Dependencies,
}
|===|
[schema=PackageJSON]
{
  dependencies: {
    pi_prompt_guard: 1,
  },
}`
		_, err := New().Process(input)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "type mismatch")
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

	It("validates utility helpers and branch behavior", func() {
		workspace, err := os.MkdirTemp("", "processor-branch-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./missing.mace"`}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_ = writeFixtureFile(workspace, "profile.mace", `|============================================|
type Age: int;
schema Profile: { age: Age, bio?: string, };
|============================================|
[output = schema]
{
  Age: Age,
  Profile: Profile,
}`)
		_ = writeFixtureFile(workspace, "base.mace", `|======================================================|
from "./profile.mace" import Profile:UserProfile, Age;
type Name: string;

schema User: {
  name: Name,
  age: Age,
  profile?: UserProfile,
};

schema Secret: {
  token: int,
};

|======================================================|

[output = schema]
{
  Name: Name,
  User: User,
}`)
		_ = writeFixtureFile(workspace, "scriptonly.mace", `|======================================================|
from "./base.mace" import User;
|======================================================|

[output = schema]
{
  User: User,
}`)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./scriptonly.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "missing"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./scriptonly.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "User"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./scriptonly.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}, {Path: ast.StringLiteral{Lexeme: `"./scriptonly.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"bad path"`}, Identifiers: []ast.ImportedIdentifier{{Name: "missing"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"missing.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"missing.mace"`}}, workspace, workspace)
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(filepath.Join(workspace, "missing.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"/abs.mace"`}}}}, "", workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadImportExports(filepath.Join(workspace, "still-missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		noScriptFile := writeFixtureFile(workspace, "noscript.mace", `[output = schema]
{}`)
		_, err = loadSchemaFileDeclarations(noScriptFile, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.NoError(err)

		decls, err := loadSchemaFileDeclarations(writeFixtureFile(workspace, "scriptonly.mace", `[output = schema]
{}`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.NotNil(decls)

		fullDecls := writeFixtureFile(workspace, "fulldecls.mace", `|======================================================|
from "./scriptonly.mace" import User;
int value = 1;
type Alias: string;
schema User: {
  name: string,
};
|======================================================|

[output = schema]
{
  User: User,
}`)
		loadedDecls, err := loadSchemaFileDeclarations(fullDecls, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Contains(loadedDecls, "value")
		tAssert.Contains(loadedDecls, "Alias")
		tAssert.Contains(loadedDecls, "User")
		_, err = loadSchemaFileDeclarations(writeFixtureFile(workspace, "badimport.mace", `|===|
import "bad path";
|===|

[output = schema]
{}`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(writeFixtureFile(workspace, "escape.mace", `|===|
import "../escape.mace";
|===|

[output = schema]
{}`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(writeFixtureFile(workspace, "withimport.mace", `|===|
from "./scriptonly.mace" import User;
|===|

[output = schema]
{}`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.NoError(err)

		symbols := newSymbolTable()
		symbols.Add("import", symbolKindImport)
		symbols.Add("Alias", symbolKindType)
		symbols.Add("User", symbolKindSchema)
		symbols.Add("name", symbolKindVariable)
		tAssert.True(symbols.Has("Alias"))
		tAssert.True(symbols.IsImport("import"))
		tAssert.True(symbols.IsType("Alias"))
		tAssert.True(symbols.IsSchema("User"))
		tAssert.True(symbols.IsVariable("name"))
		tAssert.False(symbols.IsVariable("missing"))
		kind, ok := symbols.Get("Alias")
		tAssert.True(ok)
		tAssert.Equal(symbolKindType, kind)
		tAssert.NotNil(symbols.Clone())

		types := newTypeRegistry()
		types.AddAlias("Base", ast.PrimitiveType{Name: "string"})
		types.AddAlias("Choice", ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "7"}}})
		types.AddAlias("Alias", ast.NamedType{Name: "Base"})
		types.AddAlias("LoopA", ast.NamedType{Name: "LoopB"})
		types.AddAlias("LoopB", ast.NamedType{Name: "LoopA"})
		resolvedType, ok, err := types.Resolve("Alias")
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, resolvedType)
		_, ok, err = types.Resolve("Missing")
		tAssert.NoError(err)
		tAssert.False(ok)
		_, _, err = types.Resolve("LoopA")
		tAssert.Error(err)
		tAssert.NotNil(types.Clone())

		resolvedChoice, err := resolveChoiceType(ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Choice"}, ast.StringLiteral{Lexeme: `"Ada"`}, ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "7"}}}, types)
		tAssert.NoError(err)
		tAssert.True(choiceContainsValue(resolvedChoice.choiceValues, Value{Kind: ValueString, String: "Ada"}))
		tAssert.True(choiceContainsValue(resolvedChoice.choiceValues, Value{Kind: ValueInt, Int: 7}))
		_, err = resolveChoiceValues([]ast.Expression{ast.RecordLiteral{}}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.Identifier{Name: "Missing"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.Identifier{Name: "LoopA"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.Identifier{Name: "Alias"}, types, map[string]struct{}{})
		tAssert.Error(err)
		choiceName := choiceTypeNameForSchema(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.BooleanLiteral{Value: false}}}, types)
		tAssert.Contains(choiceName, `"Ada"`)
		tAssert.Contains(choiceName, "false")

		tAssert.Equal("string", (valueType{kind: ValueString}).name())
		tAssert.Equal("array<int>", (valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}).name())
		tAssert.Equal("record<string>", (valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}).name())
		tAssert.Equal("User", (valueType{kind: ValueRecord, schemaName: "User"}).name())
		tAssert.Contains((valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}).name(), "variant[")
		tAssert.Contains((valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}, nullable: true}).name(), "nullable choice")
		tAssert.Equal("unknown", (valueType{}).name())

		tAssert.True(typesEqual(valueType{kind: ValueRecord}, valueType{kind: ValueRecord, schemaName: "User"}))
		tAssert.False(typesEqual(valueType{kind: ValueArray}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}))
		tAssert.True(typesEqual(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}))
		tAssert.True(typesEqual(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}))
		tAssert.False(typesEqual(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Bea"}}}))

		tAssert.Error(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString, nullable: true}))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueString, nullable: true}, valueType{kind: ValueNull}))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueUnknown}, valueType{kind: ValueInt}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueInt}, valueType{kind: ValueUnknown}))
		tAssert.NoError(ensureAssignable(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}, valueType{kind: ValueString}))
		tAssert.Error(ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueString, exactValue: &Value{Kind: ValueString, String: "Bea"}}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueRecord, schemaName: "A"}, valueType{kind: ValueRecord, schemaName: "B"}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}))

		for _, typeName := range []string{"string", "int", "float", "hex_int", "hex_float", "boolean"} {
			resolved, err := primitiveValueType(typeName)
			tAssert.NoError(err)
			tAssert.NotEqual(ValueUnknown, resolved.kind)
		}
		_, err = primitiveValueType("missing")
		tAssert.Error(err)

		_, err = schemaTypeFromTypeReference(ast.PrimitiveType{Name: "string"}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.NamedType{Name: "User"}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(nil, types)
		tAssert.Error(err)

		variables := newVariableRegistry()
		variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		variables.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		variables.Add("recordValue", valueType{kind: ValueRecord, element: &valueType{kind: ValueString}})
		variables.Add("bool", valueType{kind: ValueBoolean})
		variables.Add("nullable", valueType{kind: ValueString, nullable: true})
		schemas := newSchemaRegistry()
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}})

		resultType, err := inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, resultType.kind)
		_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "missing"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		resultType, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "recordValue"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, resultType.kind)
		_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "nullable"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "array"}, Index: ast.IntLiteral{Lexeme: "0"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "record"}, Index: ast.IntLiteral{Lexeme: "0"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenQuestion, Right: ast.BooleanLiteral{Value: false}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.BooleanLiteral{Value: true}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.Identifier{Name: "record"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "4"}, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.HexIntLiteral{Lexeme: "0x4"}, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenShiftRightUnsigned, Left: ast.HexIntLiteral{Lexeme: "0x4"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.StringLiteral{Lexeme: `"a"`}, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenQuestion, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "missing"}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)

		numericType, err := inferNumericBinary(lexer.TokenPlus, valueType{kind: ValueInt}, valueType{kind: ValueInt})
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, numericType.kind)
		_, err = inferNumericBinary(lexer.TokenSlash, valueType{kind: ValueHexInt}, valueType{kind: ValueHexInt})
		tAssert.NoError(err)
		_, err = inferNumericBinary(lexer.TokenPlus, valueType{kind: ValueHexInt}, valueType{kind: ValueInt})
		tAssert.Error(err)
		_, err = inferNumericBinary(lexer.TokenPlus, valueType{kind: ValueString}, valueType{kind: ValueInt})
		tAssert.Error(err)

		tAssert.NoError(validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "Alias", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"sum"`}}}, symbols, schemas, variables, map[string]struct{}{}, map[string]symbolKind{"Alias": symbolKindType}))
		tAssert.Error(validateDocDeclaration(ast.DocDeclaration{Target: "missing", Documentation: ast.Documentation{}}, symbols, schemas, variables, map[string]struct{}{}, map[string]symbolKind{}))
		tAssert.NoError(validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Schema"}, {Kind: ast.OutputDirectiveSchema, Value: "Schema"}}}))
		tAssert.NoError(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}, symbols, types, schemas, nil))
		err = validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)

		_, err = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.NoError(err)
		_, err = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.Error(err)
		_, err = evaluateHexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 3})
		tAssert.NoError(err)
		_, err = evaluateIntNumeric(lexer.TokenSlash, 4, 0)
		tAssert.Error(err)
		_, err = evaluateFloatNumeric(lexer.TokenSlash, 4, 0)
		tAssert.Error(err)
		_, err = evaluateIntPower(2, -1)
		tAssert.Error(err)
		_, err = evaluateModulo(Value{Kind: ValueHexInt, Int: 7}, Value{Kind: ValueHexInt, Int: 3})
		tAssert.NoError(err)
		_, err = evaluateModulo(Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 0})
		tAssert.Error(err)
		_, err = evaluateShift(lexer.TokenShiftRightUnsigned, Value{Kind: ValueHexInt, Int: 8}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.NoError(err)
		_, err = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueInt, Int: 8}, Value{Kind: ValueInt, Int: -1})
		tAssert.Error(err)
		_, err = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.NoError(err)
		_, err = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.Error(err)
		_, err = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 7})
		tAssert.NoError(err)
		_, err = evaluateEquality(lexer.TokenNotEqual, Value{Kind: ValueString, String: "Ada"}, Value{Kind: ValueString, String: "Bob"})
		tAssert.NoError(err)
		_, err = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		tAssert.Error(err)
		_, err = valuesEqual(Value{Kind: ValueBoolean, Boolean: true}, Value{Kind: ValueBoolean, Boolean: false})
		tAssert.NoError(err)
		_, err = valuesEqual(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		tAssert.Error(err)
		_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bob"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.NullLiteral{}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bob"`}}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		contains, err := evaluateContains(Value{Kind: ValueString, String: "name"}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		tAssert.NoError(err)
		tAssert.True(contains.Boolean)
		_, err = evaluateContains(Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueRecord})
		tAssert.Error(err)
		tAssert.True(arrayMergeTypesMatch(Value{Kind: ValueArray, Type: &valueType{kind: ValueString}}, Value{Kind: ValueArray, Type: &valueType{kind: ValueString}}))
		tAssert.False(arrayMergeTypesMatch(Value{Kind: ValueArray, Type: &valueType{kind: ValueString}}, Value{Kind: ValueArray, Type: &valueType{kind: ValueInt}}))

		_, err = parseImportPath(ast.StringLiteral{Lexeme: `"unterminated`})
		tAssert.Error(err)
		_, err = parseHexFloat("bad")
		tAssert.Error(err)
		_, err = parseInterpolatedString("unterminated", newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, _, err = unescapeSequence(`\`)
		tAssert.Error(err)
		_, _, err = unescapeSequence(`\u00ZZ`)
		tAssert.Error(err)
		_, err = parseUnicodeEscape(`\uD800`, 4)
		tAssert.Error(err)
		stringified, err := stringifyValue(Value{Kind: ValueInt, Int: 7})
		tAssert.NoError(err)
		tAssert.Equal("7", stringified)
		stringified, err = stringifyValue(Value{Kind: ValueFloat, Float: 1.25})
		tAssert.NoError(err)
		tAssert.Contains(stringified, "1.25")
		stringified, err = stringifyValue(Value{Kind: ValueHexInt, Int: 7})
		tAssert.NoError(err)
		tAssert.Equal("0x7", stringified)
		stringified, err = stringifyValue(Value{Kind: ValueHexFloat, Float: 1.5})
		tAssert.NoError(err)
		tAssert.Contains(stringified, "0x1.")
		stringified, err = stringifyValue(Value{Kind: ValueBoolean, Boolean: true})
		tAssert.NoError(err)
		tAssert.Equal("true", stringified)
		_, err = stringifyValue(Value{Kind: ValueRecord})
		tAssert.Error(err)
		tAssert.Contains(formatHexFloat(0.1), "0x0.")
		tAssert.Contains(formatHexFloat(1.25), "0x1.")
		tAssert.Contains(formatHexFloat(-1.25), "-0x1.")
		tAssert.Equal("0x0.0", formatHexFloat(0))
		tAssert.Equal("0x2.0", formatHexFloat(2))
		tAssert.Contains(formatHexFloat(1.5), "0x1.")
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
		tAssert.NoError(os.WriteFile(circularPath, []byte("from \"./circular.mace\" import User;\n[output = schema]\n{ User: string; }"), 0o600))

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
