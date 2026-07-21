package processor

import (
	"fmt"
	"os"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Output data", func() {
	It("attaches invalid output optionality errors to the field name", func() {
		_, err := New().Process(`|===|
schema User: { name: string, };
|===|
[output = 'data', schema = User]
{ name?: 'Ada', }`)
		tAssert.Error(err)

		var diagnostic DiagnosticError
		if tAssert.ErrorAs(err, &diagnostic) {
			tAssert.Equal(ErrorCode("mace.type.data-field-optional-marker"), diagnostic.Code)
			tAssert.Equal(5, diagnostic.Range.Start.Line)
			tAssert.Equal(3, diagnostic.Range.Start.Column)
			tAssert.Equal(5, diagnostic.Range.End.Line)
			tAssert.Equal(7, diagnostic.Range.End.Column)
		}
	})

	It("attaches invalid null usage errors to the null literal", func() {
		_, err := New().Process(`|===|
string value = null;
|===|
[output = 'data'] { value: value, }`)
		tAssert.Error(err)

		var diagnostic DiagnosticError
		if tAssert.ErrorAs(err, &diagnostic) {
			tAssert.Equal(CodeInvalidNullUsage, diagnostic.Code)
			tAssert.Equal(2, diagnostic.Range.Start.Line)
			tAssert.Equal(16, diagnostic.Range.Start.Column)
			tAssert.Equal(2, diagnostic.Range.End.Line)
			tAssert.Equal(20, diagnostic.Range.End.Column)
		}
	})

	DescribeTable("rejects invalid directives",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("duplicate output directive", "[output = 'data', output = 'schema'] {}", "duplicate output directive"),
		Entry("unknown schema in directive", "[output = 'data', schema = Missing] {}", "unknown schema"),
		Entry("schema directive is invalid in schema mode", "[output = 'schema', schema = User] {}", "schema directive"),
		Entry("schema_file directive is invalid in schema mode", `[output = 'schema', schema_file = './user.mace'] {}`, "schema_file"),
		Entry("parse directive is invalid in schema mode", `[output = 'schema', parse = User] {}`, "parse directive is invalid when output mode is schema"),
		Entry("parse_file directive is invalid in schema mode", `[output = 'schema', parse_file = './user.mace'] {}`, "parse_file directive is invalid when output mode is schema"),
	)

	DescribeTable("returns schema output fields",
		func(input string, expected map[expectedSchemaField]SchemaType) {
			processor := New()
			result, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)

			assertExpectedSchema(result, expected)
		},
		Entry("primitive and optional fields", `[output = 'schema']
{
  name: string,
  age?: int,
}`, map[expectedSchemaField]SchemaType{
			{name: "name"}:                schemaPrimitive("string"),
			{name: "age", optional: true}: schemaPrimitive("int"),
		}),
		Entry("nested array fields", `[output = 'schema']
{
  names: array<string>,
  matrix: array<array<int>>,
}`, map[expectedSchemaField]SchemaType{
			{name: "names"}:  schemaArray(schemaPrimitive("string")),
			{name: "matrix"}: schemaArray(schemaArray(schemaPrimitive("int"))),
		}),
		Entry("record fields", `|===|
schema User: { name: string, };
|===|
[output = 'schema']
{
  profile: { name: string, age?: int, },
  user: User,
}`, map[expectedSchemaField]SchemaType{
			{name: "profile"}: schemaRecord(map[expectedSchemaField]SchemaType{
				{name: "name"}:                schemaPrimitive("string"),
				{name: "age", optional: true}: schemaPrimitive("int"),
			}),
			{name: "user"}: schemaNamed("User"),
		}),
		Entry("variant fields", `[output = 'schema']
{
  value: variant[string, int],
}`, map[expectedSchemaField]SchemaType{
			{name: "value"}: {Kind: SchemaTypeVariant, Members: []SchemaType{schemaPrimitive("string"), schemaPrimitive("int")}},
		}),
		Entry("fusion fields resolve and deduplicate choices", `|===|
 alias Environment: choice["dev", "prod"];
 alias Numeric: choice[1, 2];
|===|
[output = 'schema']
{
  mode: fusion[Environment, Numeric],
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
schema User: { name: string, age?: int, };
string name = "Ada";
|===|
[output = 'data', schema = User]
{ name: name, }`),
		Entry("nested record literal", `|===|
schema Profile: { age: int, };
schema User: { profile: Profile, };
|===|
[output = 'data', schema = User]
{ profile: { age: 30, }, }`),
		Entry("variant array field", `|===|
schema Team: { values: array<variant[string, int]>, };
|===|
[output = 'data', schema = Team]
{ values: ["Ada", 1], }`),
		Entry("bare output block defaults to data", `{ result: 1 + 2, }`),
		Entry("output shorthand satisfies schema", `|===|
schema User: { name: string, };
string name = "Ada";
|===|
[output = 'data', schema = User]
{ name, }`),
		Entry("record shorthand satisfies schema", `|===|
string name = "Ada";
schema User: { name: string, };
schema Wrapper: { user: User, };
User user = { name, };
|===|
[output = 'data', schema = Wrapper]
{ user: user, }`),
	)

	DescribeTable("evaluates shorthand record and output fields",
		func(input string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)

			for name, value := range expected {
				assertExpectedValue(requireOutputValue(result, name), value)
			}
		},
		Entry("string shorthand", `|===|
string name = "Mary";
|===|
[output = 'data']
{ name, }`, map[string]expectedValue{
			"name": {kind: ValueString, string: "Mary"},
		}),
		Entry("number shorthand", `|===|
int age = 42;
|===|
[output = 'data']
{ age, }`, map[string]expectedValue{
			"age": {kind: ValueInt, int64: 42},
		}),
		Entry("array shorthand", `|===|
array<string> names = ["Ada", "Linus"];
|===|
[output = 'data']
{ names, }`, map[string]expectedValue{
			"names": {kind: ValueArray, array: []expectedValue{{kind: ValueString, string: "Ada"}, {kind: ValueString, string: "Linus"}}},
		}),
		Entry("array with records shorthand", `|===|
array<{ name: string, }> users = [{ name: "Ada", }, { name: "Linus", }];
|===|
[output = 'data']
{ users, }`, map[string]expectedValue{
			"users": {kind: ValueArray, array: []expectedValue{{kind: ValueRecord, record: map[string]expectedValue{"name": {kind: ValueString, string: "Ada"}}}, {kind: ValueRecord, record: map[string]expectedValue{"name": {kind: ValueString, string: "Linus"}}}}},
		}),
		Entry("record shorthand depth 1", `|===|
string city = "Paris";
{ city: string, } wrapper = { city, };
|===|
[output = 'data']
{ wrapper, }`, map[string]expectedValue{
			"wrapper": {kind: ValueRecord, record: map[string]expectedValue{"city": {kind: ValueString, string: "Paris"}}},
		}),
		Entry("record shorthand depth 2", `|===|
string city = "Paris";
{ location: { city: string, }, } wrapper = { location: { city, }, };
|===|
[output = 'data']
{ wrapper, }`, map[string]expectedValue{
			"wrapper": {kind: ValueRecord, record: map[string]expectedValue{"location": {kind: ValueRecord, record: map[string]expectedValue{"city": {kind: ValueString, string: "Paris"}}}}},
		}),
		Entry("record shorthand depth 3", `|===|
string city = "Paris";
{ a: { b: { city: string, }, }, } wrapper = { a: { b: { city, }, }, };
|===|
[output = 'data']
{ wrapper, }`, map[string]expectedValue{
			"wrapper": {kind: ValueRecord, record: map[string]expectedValue{"a": {kind: ValueRecord, record: map[string]expectedValue{"b": {kind: ValueRecord, record: map[string]expectedValue{"city": {kind: ValueString, string: "Paris"}}}}}}},
		}),
		Entry("record shorthand depth 4", `|===|
string city = "Paris";
{ a: { b: { c: { city: string, }, }, }, } wrapper = { a: { b: { c: { city, }, }, }, };
|===|
[output = 'data']
{ wrapper, }`, map[string]expectedValue{
			"wrapper": {kind: ValueRecord, record: map[string]expectedValue{"a": {kind: ValueRecord, record: map[string]expectedValue{"b": {kind: ValueRecord, record: map[string]expectedValue{"c": {kind: ValueRecord, record: map[string]expectedValue{"city": {kind: ValueString, string: "Paris"}}}}}}}}},
		}),
		Entry("record shorthand depth 5", `|===|
string city = "Paris";
{ a: { b: { c: { d: { city: string, }, }, }, }, } wrapper = { a: { b: { c: { d: { city, }, }, }, }, };
|===|
[output = 'data']
{ wrapper, }`, map[string]expectedValue{
			"wrapper": {kind: ValueRecord, record: map[string]expectedValue{"a": {kind: ValueRecord, record: map[string]expectedValue{"b": {kind: ValueRecord, record: map[string]expectedValue{"c": {kind: ValueRecord, record: map[string]expectedValue{"d": {kind: ValueRecord, record: map[string]expectedValue{"city": {kind: ValueString, string: "Paris"}}}}}}}}}}},
		}),
		Entry("multiple shorthand fields", `|===|
string first = "Ada";
int count = 2;
array<string> tags = ["math", "logic"];
|===|
[output = 'data']
{ first, count, tags, }`, map[string]expectedValue{
			"first": {kind: ValueString, string: "Ada"},
			"count": {kind: ValueInt, int64: 2},
			"tags":  {kind: ValueArray, array: []expectedValue{{kind: ValueString, string: "math"}, {kind: ValueString, string: "logic"}}},
		}),
		Entry("multiple record values", `|===|
string name = "Ada";
int age = 30;
{ name: string, } profile = { name, };
{ age: int, } stats = { age, };
|===|
[output = 'data']
{ profile, stats, }`, map[string]expectedValue{
			"profile": {kind: ValueRecord, record: map[string]expectedValue{"name": {kind: ValueString, string: "Ada"}}},
			"stats":   {kind: ValueRecord, record: map[string]expectedValue{"age": {kind: ValueInt, int64: 30}}},
		}),
		Entry("record shorthand composes deeply nested records from earlier shorthand records", `|===|
string name = "Ada";
{ name: string, } profile = { name, };
{ profile: { name: string, }, } layer1 = { profile, };
{ layer1: { profile: { name: string, }, }, } layer2 = { layer1, };
{ layer2: { layer1: { profile: { name: string, }, }, }, } layer3 = { layer2, };
|===|
[output = 'data']
{ layer3, }`, map[string]expectedValue{
			"layer3": {kind: ValueRecord, record: map[string]expectedValue{"layer2": {kind: ValueRecord, record: map[string]expectedValue{"layer1": {kind: ValueRecord, record: map[string]expectedValue{"profile": {kind: ValueRecord, record: map[string]expectedValue{"name": {kind: ValueString, string: "Ada"}}}}}}}}},
		}),
	)

	DescribeTable("rejects output that violates schema",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", `|===|
schema User: { name: string, age: int, };
|===|
[output = 'data', schema = User]
{ name: "Ada", }`, "missing required field"),
		Entry("unknown output field", `|===|
schema User: { name: string, };
|===|
[output = 'data', schema = User]
{ name: "Ada", extra: 1, }`, "unknown output field"),
		Entry("optional output mismatch", `|===|
schema User: { name: string, age: int, };
|===|
[output = 'data', schema = User]
{ name: "Ada", age?: 30, }`, "not optional"),
		Entry("nested record mismatch", `|===|
schema Profile: { age: int, };
schema User: { profile: Profile, };
|===|
[output = 'data', schema = User]
{ profile: { }, }`, "missing required field"),
		Entry("array element mismatch", `|===|
schema Point: { x: int, y: int, };
schema Plot: { points: array<Point>, };
|===|
[output = 'data', schema = Plot]
{ points: [ { x: 1, y: 2, }, { x: 3, } ], }`, "missing required field"),
		Entry("choice field rejects values outside the domain", `|===|
 alias Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite: Fruit, };
|===|
[output = 'data', schema = Basket]
{ favorite: "Pear", }`, "type mismatch: expected choice[\"Apple\", \"Strawberry\"], got \"Pear\""),
		Entry("output shorthand rejects unknown variables", `[output = 'data']
{ missing, }`, "unknown identifier \"missing\""),
		Entry("record shorthand rejects unknown variables", `[output = 'data']
{ value: { kind, }, }`, "unknown identifier \"kind\""),
		Entry("record shorthand rejects missing required fields", `|===|
schema User: { name: string, };
User user = { missing, };
|===|
[output = 'data']
{ user: user, }`, "missing required field \"name\""),
		Entry("record shorthand rejects values for required fields", `|===|
string name = null;
schema User: { name: string, };
User user = { name, };
|===|
[output = 'data']
{ user: user, }`, "null is only allowed in output"),
	)

	DescribeTable("rejects null below the output field root",
		func(expression string) {
			_, err := New().Process(fmt.Sprintf(`[output = 'data']
{
  value: %s,
}`, expression))
			tAssert.ErrorContains(err, "null is only allowed in output")
		},
		Entry("array member", `[null]`),
		Entry("record field", `{ nested: null }`),
	)

	DescribeTable("requires context for empty collection output literals",
		func(expression string) {
			_, err := New().Process(fmt.Sprintf(`[output = 'data']
{
  value: %s,
}`, expression))
			tAssert.Error(err)
			tAssert.ErrorContains(err, "requires an output schema")
		},
		Entry("empty array", `[]`),
		Entry("empty record", `{}`),
		Entry("conditional empty array", `false ? "configured" : []`),
		Entry("conditional empty record", `false ? "configured" : {}`),
		Entry("two empty array branches", `true ? [] : []`),
		Entry("two empty record branches", `true ? {} : {}`),
		Entry("nested empty array", `[[1], []]`),
		Entry("nested empty record", `{ nested: {}, }`),
	)

	DescribeTable("requires an output schema when a typed collection branch has an empty fallback",
		func(input string) {
			_, err := New().Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "requires an output schema")
		},
		Entry("record map", `|===|
boolean configured = true;
record<string> records = { primary: "active", };
|===|
[output = 'data']
{
  value: configured ? records : {},
}`),
		Entry("array", `|===|
boolean configured = true;
array<string> values = ["configured"];
|===|
[output = 'data']
{
  value: configured ? values : [],
}`),
	)

	DescribeTable("uses schema context for conditional output with an empty collection",
		func(fieldType string, expression string, expected expectedValue) {
			result, err := New().Process(fmt.Sprintf(`|===|
schema Result: { value: %s, };
|===|
[output = 'data', schema = Result]
{
  value: %s,
}`, fieldType, expression))
			tAssert.NoError(err)
			assertExpectedValue(requireOutputValue(result, "value"), expected)
		},
		Entry(
			"variant with an empty array branch",
			`variant[string, array<string>]`,
			`false ? "configured" : []`,
			expectedValue{kind: ValueArray, array: []expectedValue{}},
		),
		Entry(
			"array with an empty array branch",
			`array<string>`,
			`true ? ["configured"] : []`,
			expectedValue{kind: ValueArray, array: []expectedValue{{kind: ValueString, string: "configured"}}},
		),
		Entry(
			"record with an empty record branch",
			`{ name?: string, }`,
			`false ? { name: "Ada", } : {}`,
			expectedValue{kind: ValueRecord, record: map[string]expectedValue{}},
		),
	)

	DescribeTable("uses schema_file context for conditional output with an empty collection",
		func(fieldType string, expression string, expected expectedValue) {
			workspace, err := os.MkdirTemp("", "mace-output-empty-schema-file-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			writeFixtureFile(workspace, "schema.mace", fmt.Sprintf(`[output = 'schema']
{
  value: %s,
}`, fieldType))
			result, err := New().ProcessInDir(fmt.Sprintf(`[output = 'data', schema_file = './schema.mace']
{
  value: %s,
}`, expression), workspace)
			tAssert.NoError(err)
			assertExpectedValue(requireOutputValue(result, "value"), expected)
		},
		Entry(
			"array with an empty array branch",
			`array<string>`,
			`false ? ["configured"] : []`,
			expectedValue{kind: ValueArray, array: []expectedValue{}},
		),
		Entry(
			"record with an empty record branch",
			`{ name?: string, }`,
			`false ? { name: "Ada", } : {}`,
			expectedValue{kind: ValueRecord, record: map[string]expectedValue{}},
		),
	)
})

var _ = Describe("Data output helpers", func() {
	It("covers output directive and type validation branches", func() {
		workspace, err := os.MkdirTemp("", "processor-output-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		writeFixtureFile(workspace, "schema-names.mace", `[output = 'schema']
{ User: User, Other: Other, }`)
		writeFixtureFile(workspace, "schema-empty.mace", `[output = 'schema']
{ title: string, }`)
		writeFixtureFile(workspace, "parse-names.mace", `[output = 'schema']
{ User: User, Other: Other, }`)
		writeFixtureFile(workspace, "parse-one.mace", `[output = 'schema']
{ User: User, }`)
		writeFixtureFile(workspace, "parse-empty.mace", `[output = 'schema']
{ title: string, }`)

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
		tAssert.NoError(validateDataOutputExpression(ast.NullLiteral{}, symbols))
		tAssert.Error(validateDataOutputExpression(ast.Identifier{Name: "recordType"}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "record"}}}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "record"}}}}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.PrefixExpression{Right: ast.Identifier{Name: "record"}}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.InfixExpression{Left: ast.Identifier{Name: "record"}, Right: ast.Identifier{Name: "record"}}, symbols))

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
