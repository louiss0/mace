package processor

import (
	"os"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Output data", func() {
	DescribeTable("rejects invalid directives",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("duplicate output directive", "[output = data, output = schema] {}", "duplicate output directive"),
		Entry("unknown schema in directive", "[output = data, schema = Missing] {}", "unknown schema"),
		Entry("schema directive is invalid in schema mode", "[output = schema, schema = User] {}", "schema directive"),
		Entry("schema_file directive is invalid in schema mode", `[output = schema, schema_file = "./user.mace"] {}`, "schema_file"),
		Entry("parse directive is invalid in schema mode", `[output = schema, parse = User] {}`, "parse directive is invalid when output mode is schema"),
		Entry("parse_file directive is invalid in schema mode", `[output = schema, parse_file = "./user.mace"] {}`, "parse_file directive is invalid when output mode is schema"),
	)

	DescribeTable("returns schema output fields",
		func(input string, expected map[expectedSchemaField]SchemaType) {
			processor := New()
			result, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)

			assertExpectedSchema(result, expected)
		},
		Entry("primitive and optional fields", `[output = schema]
{
  name: string;
  age?: int;
}`, map[expectedSchemaField]SchemaType{
			{name: "name"}:                schemaPrimitive("string"),
			{name: "age", optional: true}: schemaPrimitive("int"),
		}),
		Entry("nested array fields", `[output = schema]
{
  names: array<string>;
  matrix: array<array<int>>;
}`, map[expectedSchemaField]SchemaType{
			{name: "names"}:  schemaArray(schemaPrimitive("string")),
			{name: "matrix"}: schemaArray(schemaArray(schemaPrimitive("int"))),
		}),
		Entry("record fields", `|===|
schema User: { name: string; };
|===|
[output = schema]
{
  profile: { name: string; age?: int; };
  user: User;
}`, map[expectedSchemaField]SchemaType{
			{name: "profile"}: schemaRecord(map[expectedSchemaField]SchemaType{
				{name: "name"}:                schemaPrimitive("string"),
				{name: "age", optional: true}: schemaPrimitive("int"),
			}),
			{name: "user"}: schemaNamed("User"),
		}),
		Entry("variant fields", `[output = schema]
{
  value: variant[string, int];
}`, map[expectedSchemaField]SchemaType{
			{name: "value"}: {Kind: SchemaTypeVariant, Members: []SchemaType{schemaPrimitive("string"), schemaPrimitive("int")}},
		}),
		Entry("choice fields resolve nested choice aliases", `|===|
 type Environment: choice["dev", "prod"];
 type Numeric: choice[1, 2];
|===|
[output = schema]
{
  mode: choice[Environment, Numeric];
}`, map[expectedSchemaField]SchemaType{
			{name: "mode"}: schemaNamed(`choice["dev", "prod", 1, 2]`),
		}),
	)

	DescribeTable("accepts output that matches schema",
		func(input string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)
		},
		Entry("optional field omitted", `|===|
schema User: { name: string; age?: int; };
string name = "Ada";
|===|
[output = data, schema = User]
{ name: name; }`),
		Entry("nested record literal", `|===|
schema Profile: { age: int; };
schema User: { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { age: 30; }; }`),
		Entry("variant array field", `|===|
schema Team: { values: array<variant[string, int]>; };
|===|
[output = data, schema = Team]
{ values: ["Ada", 1]; }`),
		Entry("bare output block defaults to data", `{ result: 1 + 2; }`),
	)

	DescribeTable("rejects output that violates schema",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", `|===|
schema User: { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; }`, "missing required field"),
		Entry("unknown output field", `|===|
schema User: { name: string; };
|===|
[output = data, schema = User]
{ name: "Ada"; extra: 1; }`, "unknown output field"),
		Entry("optional output mismatch", `|===|
schema User: { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; age?: 30; }`, "not optional"),
		Entry("nested record mismatch", `|===|
schema Profile: { age: int; };
schema User: { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { }; }`, "missing required field"),
		Entry("array element mismatch", `|===|
schema Point: { x: int; y: int; };
schema Plot: { points: array<Point>; };
|===|
[output = data, schema = Plot]
{ points: [ { x: 1; y: 2; }, { x: 3; } ]; }`, "missing required field"),
		Entry("choice field rejects values outside the domain", `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite: Fruit; };
|===|
[output = data, schema = Basket]
{ favorite: "Pear"; }`, "type mismatch: expected choice[\"Apple\", \"Strawberry\"], got \"Pear\""),
	)
})

var _ = Describe("Data output helpers", func() {
	It("covers output directive and type validation branches", func() {
		workspace, err := os.MkdirTemp("", "processor-output-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		writeFixtureFile(workspace, "schema-names.mace", `[output = schema]
{ User: User; Other: Other; }`)
		writeFixtureFile(workspace, "schema-empty.mace", `[output = schema]
{ title: string; }`)
		writeFixtureFile(workspace, "parse-names.mace", `[output = schema]
{ User: User; Other: Other; }`)
		writeFixtureFile(workspace, "parse-one.mace", `[output = schema]
{ User: User; }`)
		writeFixtureFile(workspace, "parse-empty.mace", `[output = schema]
{ title: string; }`)

		context := newProcessContext(workspace, workspace)
		name, ok, err := outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("User", name)
		name, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"parse-one.mace"`}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("User", name)
		name, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"parse-one.mace"`}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("User", name)
		name, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"parse-empty.mace"`}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("__parse_file", name)
		_, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"parse-names.mace"`}}, context)
		tAssert.Error(err)
		tAssert.False(ok)

		names, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema-names.mace"`}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		tAssert.NoError(err)
		tAssert.Equal([]string{"Other", "User"}, names)
		names, err = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"parse-empty.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)
		tAssert.NoError(err)
		tAssert.Empty(names)

		symbols := newSymbolTable()
		symbols.Add("record", symbolKindVariable)
		symbols.Add("recordType", symbolKindType)
		tAssert.Error(validateDataOutputExpression(ast.NullLiteral{}, symbols))
		tAssert.Error(validateDataOutputExpression(ast.Identifier{Name: "recordType"}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "record"}}}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "record"}}}}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.PrefixExpression{Right: ast.Identifier{Name: "record"}}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.InfixExpression{Left: ast.Identifier{Name: "record"}, Right: ast.Identifier{Name: "record"}}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"opt"`}, Right: ast.Identifier{Name: "record"}}, Then: ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, Else: ast.Identifier{Name: "record"}}, symbols))

		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}})
		symbols.Add("User", symbolKindSchema)
		variables := newVariableRegistry()
		variables.Add("name", valueType{kind: ValueString})
		tAssert.Error(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueInt}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, schemaName: "User"}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bob"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bob"}}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Else: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Bob"`}}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "opt", Value: ast.IntLiteral{Lexeme: "1"}}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, variables, symbols, types, schemas, nil))

		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Bob"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"unknown": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, symbols, types, schemas, nil))

		result, err := inferExpressionType(ast.Identifier{Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, result.kind)
		_, err = inferExpressionType(ast.Identifier{Name: "User"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "name"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "name"}, Index: ast.IntLiteral{Lexeme: "0"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.StringLiteral{Lexeme: `"Bob"`}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.RecordLiteral{}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bob"`}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(nil, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
	})

	It("validates output directive shapes and references", func() {
		symbols := newSymbolTable()
		symbols.Add("Schema", symbolKindSchema)
		tAssert.NoError(validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Schema"}, {Kind: ast.OutputDirectiveSchema, Value: "Schema"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Schema"}}}))
		tAssert.NoError(validateOutputDirectiveReferences(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Schema"}}}, symbols))
		tAssert.Error(validateOutputDirectiveReferences(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, symbols))
		tAssert.NoError(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}, symbols, newTypeRegistry(), newSchemaRegistry(), nil))
		tAssert.Error(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}, symbols, newTypeRegistry(), newSchemaRegistry(), nil))
	})

	It("exports output field types from schema and inferred values", func() {
		context := newProcessContext(".", ".")
		context.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{
			Name: "name",
			Type: ast.PrimitiveType{Name: "string"},
		}}})
		output := ast.OutputBlock{Directives: []ast.OutputDirective{{
			Kind:  ast.OutputDirectiveSchema,
			Value: "User",
		}}}
		fieldType := exportedOutputFieldType

		result, err := fieldType(ast.OutputField{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}, output, context)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, result.kind)

		result, err = fieldType(ast.OutputField{
			Name:  "age",
			Value: ast.IntLiteral{Lexeme: "42"},
		}, ast.OutputBlock{}, context)
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, result.kind)

		_, err = fieldType(ast.OutputField{Name: "name"}, ast.OutputBlock{Directives: []ast.OutputDirective{{
			Kind:  ast.OutputDirectiveSchema,
			Value: "Missing",
		}}}, context)
		tAssert.ErrorContains(err, "unknown schema")
	})
})
