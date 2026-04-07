package processor

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestProcessor(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Processor Suite")
}

func wrapScriptWithOutput(script string) string {
	return script + "\n[output = data] {}"
}

func wrapScriptWithOutputFields(script string, fields string) string {
	return script + "\n[output = data]\n{\n" + fields + "\n}"
}

type expectedValue struct {
	kind   ValueKind
	int64  int64
	float  float64
	bool   bool
	string string
	array  []expectedValue
	record map[string]expectedValue
}

type expectedSchemaField struct {
	name     string
	optional bool
}

func schemaPrimitive(name string) SchemaType {
	return SchemaType{Kind: SchemaTypePrimitive, Name: name}
}

func schemaNamed(name string) SchemaType {
	return SchemaType{Kind: SchemaTypeNamed, Name: name}
}

func schemaArray(element SchemaType) SchemaType {
	return SchemaType{Kind: SchemaTypeArray, Element: &element}
}

func schemaRecord(fields map[expectedSchemaField]SchemaType) SchemaType {
	recordFields := make(map[SchemaField]SchemaType, len(fields))
	for field, fieldType := range fields {
		recordFields[SchemaField{Name: field.name, Optional: field.optional}] = fieldType
	}

	return SchemaType{Kind: SchemaTypeRecord, Fields: recordFields}
}

func requireOutputValue(result Result, name string) Value {
	value, ok := result.Output[name]
	tAssert.True(ok)
	if !ok {
		return Value{}
	}
	return value
}

func assertExpectedValue(actual Value, expected expectedValue) {
	tAssert.Equal(expected.kind, actual.Kind)
	switch expected.kind {
	case ValueInt:
		tAssert.Equal(expected.int64, actual.Int)
	case ValueFloat:
		tAssert.InDelta(expected.float, actual.Float, 0.000001)
	case ValueBoolean:
		tAssert.Equal(expected.bool, actual.Boolean)
	case ValueString:
		tAssert.Equal(expected.string, actual.String)
	case ValueArray:
		tAssert.Equal(len(expected.array), len(actual.Array))
		for index, value := range expected.array {
			if index >= len(actual.Array) {
				return
			}
			assertExpectedValue(actual.Array[index], value)
		}
	case ValueRecord:
		tAssert.Equal(len(expected.record), len(actual.Record))
		for name, value := range expected.record {
			actualValue, ok := actual.Record[name]
			tAssert.True(ok)
			if !ok {
				continue
			}
			assertExpectedValue(actualValue, value)
		}
	}
}

func assertExpectedOutput(result Result, expected map[string]expectedValue) {
	for name, value := range expected {
		actual := requireOutputValue(result, name)
		assertExpectedValue(actual, value)
	}
}

func assertExpectedSchema(result Result, expected map[expectedSchemaField]SchemaType) {
	tAssert.Len(result.Output, 0)
	tAssert.Len(result.Schema, len(expected))

	for field, expectedType := range expected {
		actualType, ok := result.Schema[SchemaField{Name: field.name, Optional: field.optional}]
		tAssert.True(ok)
		if !ok {
			continue
		}

		tAssert.Equal(expectedType, actualType)
	}
}

func assertProcessedResult(input string, expected expectedValue) {
	processor := New()
	result, err := processor.Process(input)
	tAssert.NoError(err)

	actual := requireOutputValue(result, "result")
	assertExpectedValue(actual, expected)
}

func requireScriptVariable(result ScriptResult, name string) Value {
	value, ok := result.Variables[name]
	tAssert.True(ok)
	if !ok {
		return Value{}
	}

	return value
}

func writeFixtureFile(root string, relativePath string, contents string) string {
	path := filepath.Join(root, relativePath)
	tAssert.NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	tAssert.NoError(os.WriteFile(path, []byte(contents), 0o644))
	return path
}

var _ = Describe("Block processing", func() {
	It("processes script blocks independently", func() {
		processor := New()
		result, err := processor.ProcessScriptBlock(`|===|
int base = 2 + 2;
string name = "Ada";
|===|`)
		tAssert.NoError(err)

		base := requireScriptVariable(result, "base")
		tAssert.Equal(ValueInt, base.Kind)
		tAssert.Equal(int64(4), base.Int)

		name := requireScriptVariable(result, "name")
		tAssert.Equal(ValueString, name.Kind)
		tAssert.Equal("Ada", name.String)
	})

	It("processes output blocks independently", func() {
		processor := New()
		result, err := processor.ProcessOutputBlock(`[output = schema]
{
  name: string;
  age?: int;
}`, ScriptResult{})
		tAssert.NoError(err)

		assertExpectedSchema(result, map[expectedSchemaField]SchemaType{
			{name: "name"}:                schemaPrimitive("string"),
			{name: "age", optional: true}: schemaPrimitive("int"),
		})
	})

	It("processes output blocks with script context", func() {
		processor := New()
		scriptResult, err := processor.ProcessScriptBlock(`|===|
int base = 2 + 2;
|===|`)
		tAssert.NoError(err)

		result, err := processor.ProcessOutputBlock(`[output = data]
{
  result: base * 3;
}`, scriptResult)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "result")
		assertExpectedValue(actual, expectedValue{kind: ValueInt, int64: 12})
	})
})

var _ = Describe("Script block", func() {
	DescribeTable("processes valid script blocks",
		func(input string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.NoError(err)
		},
		Entry("type and schema declarations", wrapScriptWithOutput(`|===|
type Name = string;
schema User = { name: string; };
|===|`)),
		Entry("variables with literals", wrapScriptWithOutput(`|===|
string name = "Ada";
int age = 30;
float rate = 1.25;
boolean active = true;
|===|`)),
		Entry("injectable without initializer when unused", wrapScriptWithOutput(`|===|
injectable string env;
|===|`)),
		Entry("imports and script block", `from "testdata/imports/base.mace" import Name;
|===|
Name user = "Ada";
|===|
[output = data]
{ user: user; }`),
	)

	DescribeTable("rejects invalid script blocks",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown type reference", wrapScriptWithOutput(`|===|
Unknown value = 1;
|===|`), "unknown type"),
		Entry("int type mismatch", wrapScriptWithOutput(`|===|
int total = 1.5;
|===|`), "type mismatch"),
		Entry("mixed numeric expression", wrapScriptWithOutput(`|===|
float total = 1 + 2.0;
|===|`), "type mismatch"),
		Entry("duplicate declaration name", wrapScriptWithOutput(`|===|
type User = string;
schema User = { name: string; };
|===|`), "duplicate declaration"),
		Entry("duplicate imports", `from "testdata/imports/base.mace" import User, User;
[output = data] {}`, "duplicate import"),
	)

	DescribeTable("accepts schema record literals",
		func(input string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.NoError(err)
		},
		Entry("optional fields omitted", wrapScriptWithOutput(`|===|
schema User = { name: string; age?: int; };
User user = { name: "Ada"; };
|===|`)),
		Entry("array of schema records", wrapScriptWithOutput(`|===|
schema Point = { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; y: 4; }
];
|===|`)),
		Entry("injectable fallback initializer", wrapScriptWithOutput(`|===|
injectable string env = "dev";
|===|`)),
	)

	DescribeTable("rejects schema record literal mismatches",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", wrapScriptWithOutput(`|===|
schema User = { name: string; age: int; };
User user = { name: "Ada"; };
|===|`), "missing required field"),
		Entry("unknown field", wrapScriptWithOutput(`|===|
schema User = { name: string; };
User user = { name: "Ada"; age: 30; };
|===|`), "unknown field"),
		Entry("optional field mismatch", wrapScriptWithOutput(`|===|
schema User = { name: string; age: int; };
User user = { name: "Ada"; age?: 30; };
|===|`), "not optional"),
		Entry("field type mismatch", wrapScriptWithOutput(`|===|
schema User = { name: string; age: int; };
User user = { name: 5; age: 30; };
|===|`), "type mismatch"),
		Entry("array element schema mismatch", wrapScriptWithOutput(`|===|
schema Point = { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; }
];
|===|`), "missing required field"),
	)

	It("uses injected values for injectable variables", func() {
		processor := NewWithInjections(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		result, err := processor.Process(`|===|
injectable string env = "dev";
|===|
[output = data]
{
  env: env;
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "env")
		assertExpectedValue(actual, expectedValue{kind: ValueString, string: "prod"})
	})

	It("uses an initializer when an injectable value is not provided", func() {
		processor := New()

		result, err := processor.Process(`|===|
injectable string env = "dev";
|===|
[output = data]
{
  env: env;
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "env")
		assertExpectedValue(actual, expectedValue{kind: ValueString, string: "dev"})
	})

	It("rejects injectables without a provided value or initializer", func() {
		processor := New()

		_, err := processor.Process(`|===|
injectable string env;
|===|
[output = data]
{
  env: env;
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "injectable")
		tAssert.ErrorContains(err, "requires a runtime value")
	})

	It("rejects unknown injected values", func() {
		processor := NewWithInjections(map[string]Value{
			"missing": {Kind: ValueString, String: "prod"},
		})

		_, err := processor.Process(`|===|
injectable string env = "dev";
|===|
[output = data] {}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "unknown injectable")
	})

	DescribeTable("processes valid enum declarations",
		func(input string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(input)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("string enum with implicit values", `|===|
enum Fruit: string {
  Apple,
  Strawberry,
}
Fruit result = Fruit.Apple;
|===|
[output = data]
{
  result: result;
}`, expectedValue{kind: ValueString, string: "Apple"}),
		Entry("int enum with implicit values", `|===|
enum Status: int {
  Pending,
  Running,
}
Status result = Status.Running;
|===|
[output = data]
{
  result: result;
}`, expectedValue{kind: ValueInt, int64: 1}),
		Entry("int enum with explicit values", `|===|
enum Status: int {
  Pending = 0,
  Running = 1,
}
Status result = Status.Running;
|===|
[output = data]
{
  result: result;
}`, expectedValue{kind: ValueInt, int64: 1}),
	)

	DescribeTable("rejects invalid enum declarations and assignments",
		func(input string, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("duplicate enum member name", wrapScriptWithOutput(`|===|
enum Fruit: string {
  Apple,
  Apple,
}
|===|`), "duplicate enum member"),
		Entry("duplicate enum value", wrapScriptWithOutput(`|===|
enum Fruit: string {
  Apple = "fruit",
  Strawberry = "fruit",
}
|===|`), "duplicate enum value"),
		Entry("mixed implicit and explicit enum members", wrapScriptWithOutput(`|===|
enum Fruit: string {
  Apple = "apple",
  Strawberry,
}
|===|`), "mixes implicit and explicit"),
		Entry("enum explicit value type mismatch", wrapScriptWithOutput(`|===|
enum Status: int {
  Pending = "pending",
}
|===|`), "must use an int literal"),
		Entry("raw enum backing value is not assignable", `|===|
enum Fruit: string {
  Apple,
  Strawberry,
}
Fruit result = "Pear";
|===|
[output = data]
{
  result: result;
}`, "type mismatch: expected Fruit, got string"),
		Entry("unknown enum member", `|===|
enum Fruit: string {
  Apple,
  Strawberry,
}
Fruit result = Fruit.Pear;
|===|
[output = data]
{
  result: result;
}`, "unknown enum member"),
	)
})

var _ = Describe("Imports", func() {
	DescribeTable("merges imported declarations",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("imports types and schemas", `from "testdata/imports/base.mace" import Name, User;
|===|
Name name = "Ada";
User result = { name: name; age: 30; };
|===|
[output = data]
{ result: result; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("imports values surfaced through output", `from "testdata/imports/values.mace" import count;
[output = data]
{ result: count + 2; }`, expectedValue{kind: ValueInt, int64: 5}),
		Entry("imports schemas and aliases from a public contract fixture", `from "testdata/imports/contracts.mace" import ID, Team;
|===|
ID team_name = "core";
Team result = { name: team_name; members: [{ id: "u1"; role: "owner"; }]; };
|===|
[output = data]
{ result: result; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "core"},
			"members": {kind: ValueArray, array: []expectedValue{
				{kind: ValueRecord, record: map[string]expectedValue{
					"id":   {kind: ValueString, string: "u1"},
					"role": {kind: ValueString, string: "owner"},
				}},
			}},
		}}),
	)

	DescribeTable("keeps hidden declarations internal",
		func(file string, message string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("hidden type is not importable", `from "testdata/imports/base.mace" import Internal;
[output = data] {}`, "imported identifier"),
		Entry("hidden schema is not importable", `from "testdata/imports/base.mace" import Secret;
[output = data] {}`, "imported identifier"),
		Entry("hidden variable is not importable", `from "testdata/imports/base.mace" import local;
[output = data] {}`, "imported identifier"),
		Entry("hidden value is not importable", `from "testdata/imports/values.mace" import hidden;
[output = data] {}`, "imported identifier"),
		Entry("hidden schema in a data fixture is not importable", `from "testdata/imports/metrics.mace" import Hidden;
[output = data] {}`, "imported identifier"),
	)

	DescribeTable("processes imported files",
		func(path string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessFile(path)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("resolves imports relative to file", "testdata/imports/consumer.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 27},
		}}),
		Entry("resolves schema_file relative to file", "testdata/schema_file/consumer.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
		}}),
	)

	DescribeTable("rejects circular imports",
		func(path string) {
			processor := New()
			_, err := processor.ProcessFile(path)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "circular import")
		},
		Entry("cycle detected", "testdata/imports/cycle_a.mace"),
	)

	DescribeTable("rejects invalid imports",
		func(file string, message string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown imported identifier", `from "testdata/imports/base.mace" import Missing;
[output = data] {}`, "imported identifier"),
		Entry("duplicate import across declarations", `from "testdata/imports/base.mace" import Name;
from "testdata/imports/other.mace" import Name;
[output = data] {}`, "duplicate import"),
		Entry("import file missing", `from "testdata/imports/missing.mace" import Name;
[output = data] {}`, "unable to read import file"),
		Entry("import collides with local declaration", `from "testdata/imports/base.mace" import Name;
|===|
type Name = string;
|===|
[output = data] {}`, "duplicate declaration"),
	)

	It("imports enums exposed through schema output", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-enum-import-*")
		tAssert.NoError(err)

		sharedPath := writeFixtureFile(workspace, "shared.mace", `|===|
enum Fruit: string {
  Apple,
  Strawberry,
}
|===|
[output = schema]
{
  Fruit: Fruit;
}`)
		consumerPath := writeFixtureFile(workspace, "consumer.mace", `from "./shared.mace" import Fruit;
|===|
Fruit result = Fruit.Apple;
|===|
[output = data]
{
  result: result;
}`)

		processor := New()
		result, err := processor.ProcessFile(consumerPath)
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Apple"})
		tAssert.FileExists(sharedPath)
	})
})

var _ = Describe("Output block", func() {
	DescribeTable("rejects invalid directives",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing output directive", `|===|
schema User = { name: string; };
|===|
[schema = User] {}`, "missing output directive"),
		Entry("duplicate output directive", "[output = data, output = schema] {}", "duplicate output directive"),
		Entry("unknown schema in directive", "[output = data, schema = Missing] {}", "unknown schema"),
		Entry("schema directive is invalid in schema mode", "[output = schema, schema = User] {}", "schema directive"),
		Entry("schema_file directive is invalid in schema mode", `[output = schema, schema_file = "./user.mace"] {}`, "schema_file"),
	)

	DescribeTable("returns schema output fields",
		func(input string, expected map[expectedSchemaField]SchemaType) {
			processor := New()
			result, err := processor.Process(input)
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
schema User = { name: string; };
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
	)

	DescribeTable("accepts output that matches schema",
		func(input string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.NoError(err)
		},
		Entry("optional field omitted", `|===|
schema User = { name: string; age?: int; };
string name = "Ada";
|===|
[output = data, schema = User]
{ name: name; }`),
		Entry("nested record literal", `|===|
schema Profile = { age: int; };
schema User = { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { age: 30; }; }`),
		Entry("bare output block defaults to data", `{ result: 1 + 2; }`),
	)

	DescribeTable("rejects output that violates schema",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", `|===|
schema User = { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; }`, "missing required field"),
		Entry("unknown output field", `|===|
schema User = { name: string; };
|===|
[output = data, schema = User]
{ name: "Ada"; extra: 1; }`, "unknown output field"),
		Entry("optional output mismatch", `|===|
schema User = { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; age?: 30; }`, "not optional"),
		Entry("nested record mismatch", `|===|
schema Profile = { age: int; };
schema User = { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { }; }`, "missing required field"),
		Entry("array element mismatch", `|===|
schema Point = { x: int; y: int; };
schema Plot = { points: array<Point>; };
|===|
[output = data, schema = Plot]
{ points: [ { x: 1; y: 2; }, { x: 3; } ]; }`, "missing required field"),
		Entry("enum field requires member access", `|===|
enum Fruit: string {
  Apple,
  Strawberry,
}
schema Basket = { favorite: Fruit; };
|===|
[output = data, schema = Basket]
{ favorite: "Pear"; }`, "type mismatch: expected Fruit, got string"),
		Entry("enum field rejects unknown member", `|===|
enum Fruit: string {
  Apple,
  Strawberry,
}
schema Basket = { favorite: Fruit; };
|===|
[output = data, schema = Basket]
{ favorite: Fruit.Pear; }`, "unknown enum member"),
	)

	DescribeTable("rejects output surface mismatches",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("schema output cannot export variable declarations", `|===|
type Name = string;
schema User = { name: Name; age: int; };
int local = 1;
|===|
[output = schema]
{
  Name: Name;
  User: User;
  foo: local;
}`, "invalid field type"),
		Entry("data output cannot export type declarations as values", `|===|
type Name = string;
schema User = { name: string; };
string value = "Ada";
|===|
{
  Name: Name;
  User: User;
  value: value;
}`, "cannot reference type or schema declaration"),
	)

	DescribeTable("returns individual operator results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("unary plus", `[output = data] { result: +7; }`, expectedValue{kind: ValueInt, int64: 7}),
		Entry("unary minus", `[output = data] { result: -5; }`, expectedValue{kind: ValueInt, int64: -5}),
		Entry("logical not", `[output = data] { result: !false; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise not", `[output = data] { result: ~1; }`, expectedValue{kind: ValueInt, int64: ^int64(1)}),
		Entry("addition", `[output = data] { result: 1 + 2; }`, expectedValue{kind: ValueInt, int64: 3}),
		Entry("subtraction", `[output = data] { result: 5 - 3; }`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("multiplication", `[output = data] { result: 2 * 3; }`, expectedValue{kind: ValueInt, int64: 6}),
		Entry("division", `[output = data] { result: 8 / 2; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("modulo", `[output = data] { result: 9 % 4; }`, expectedValue{kind: ValueInt, int64: 1}),
		Entry("exponentiation", `[output = data] { result: 2 ** 3; }`, expectedValue{kind: ValueInt, int64: 8}),
		Entry("shift left", `[output = data] { result: 1 << 3; }`, expectedValue{kind: ValueInt, int64: 8}),
		Entry("shift right", `[output = data] { result: 8 >> 1; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("unsigned shift right", `[output = data] { result: 8 >>> 1; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("less than", `[output = data] { result: 1 < 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("less than or equal", `[output = data] { result: 2 <= 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("greater than", `[output = data] { result: 3 > 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("greater than or equal", `[output = data] { result: 2 >= 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("equal", `[output = data] { result: 3 == 3; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("not equal", `[output = data] { result: 3 != 4; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("strict equal", `[output = data] { result: 3 === 3; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("strict not equal", `[output = data] { result: 3 !== 4; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise and", `[output = data] { result: 6 & 3; }`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("bitwise xor", `[output = data] { result: 5 ^ 3; }`, expectedValue{kind: ValueInt, int64: 6}),
		Entry("bitwise or", `[output = data] { result: 5 | 2; }`, expectedValue{kind: ValueInt, int64: 7}),
		Entry("logical and", `[output = data] { result: true && false; }`, expectedValue{kind: ValueBoolean, bool: false}),
		Entry("logical or", `[output = data] { result: true || false; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("ternary", `[output = data] { result: true ? 1 : 2; }`, expectedValue{kind: ValueInt, int64: 1}),
	)

	DescribeTable("returns mixed operator results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("arithmetic precedence", `[output = data] { result: 1 + 2 * 3 - 4; }`, expectedValue{kind: ValueInt, int64: 3}),
		Entry("shift and additive precedence", `[output = data] { result: 1 + 2 << 2; }`, expectedValue{kind: ValueInt, int64: 12}),
		Entry("bitwise precedence", `[output = data] { result: 7 & 3 ^ 1 | 8; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("comparison and logic precedence", `[output = data] { result: 1 < 2 && 3 > 2 || false; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("conditional with logical expression", `[output = data] { result: false || true ? 5 : 2; }`, expectedValue{kind: ValueInt, int64: 5}),
	)

	DescribeTable("returns math results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("addition and multiplication", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 * 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("subtraction and division", wrapScriptWithOutputFields(`|===|
int result = 20 - 4 / 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 18}),
		Entry("modulo", wrapScriptWithOutputFields(`|===|
int result = 9 % 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("exponentiation", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 8}),
		Entry("unary minus", wrapScriptWithOutputFields(`|===|
int result = -5;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: -5}),
		Entry("unary plus", wrapScriptWithOutputFields(`|===|
int result = +7;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("float arithmetic", wrapScriptWithOutputFields(`|===|
float result = 1.5 + 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("float division", wrapScriptWithOutputFields(`|===|
float result = 7.5 / 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 3.0}),
	)

	DescribeTable("returns operator precedence results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("unary before exponent", wrapScriptWithOutputFields(`|===|
int result = -2 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 4}),
		Entry("exponent is right associative", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 512}),
		Entry("shift after additive", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 << 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 12}),
		Entry("relational after shift", wrapScriptWithOutputFields(`|===|
boolean result = 1 << 2 > 3;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("equality after relational", wrapScriptWithOutputFields(`|===|
boolean result = 1 < 2 == true;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise and before or", wrapScriptWithOutputFields(`|===|
int result = 1 | 2 & 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("bitwise and before xor", wrapScriptWithOutputFields(`|===|
int result = 5 ^ 2 & 1;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
		Entry("logical and before or", wrapScriptWithOutputFields(`|===|
boolean result = true || false && false;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("conditional after logical or", wrapScriptWithOutputFields(`|===|
int result = false || true ? 5 : 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
	)

	DescribeTable("rejects invalid math operators",
		func(file string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
		},
		Entry("mixed numeric addition", wrapScriptWithOutputFields(`|===|
int total = 1 + 2.0;
|===|`, "total: total;")),
		Entry("mixed numeric exponentiation", wrapScriptWithOutputFields(`|===|
float total = 2 ** 3.0;
|===|`, "total: total;")),
		Entry("modulo with float", wrapScriptWithOutputFields(`|===|
int total = 5 % 2.5;
|===|`, "total: total;")),
	)

	DescribeTable("accepts non-math operators in script variables",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("bitwise operators", wrapScriptWithOutputFields(`|===|
int masked = 6 & 3;
int combined = 5 | 2;
int toggled = 5 ^ 3;
int inverted = ~1;
|===|`, "masked: masked;\ncombined: combined;\ntoggled: toggled;\ninverted: inverted;"), map[string]expectedValue{
			"masked":   {kind: ValueInt, int64: 2},
			"combined": {kind: ValueInt, int64: 7},
			"toggled":  {kind: ValueInt, int64: 6},
			"inverted": {kind: ValueInt, int64: ^int64(1)},
		}),
		Entry("shift operators", wrapScriptWithOutputFields(`|===|
int left = 1 << 3;
int right = 8 >> 1;
int logical = 8 >>> 1;
|===|`, "left: left;\nright: right;\nlogical: logical;"), map[string]expectedValue{
			"left":    {kind: ValueInt, int64: 8},
			"right":   {kind: ValueInt, int64: 4},
			"logical": {kind: ValueInt, int64: 4},
		}),
		Entry("comparisons", wrapScriptWithOutputFields(`|===|
boolean less = 3 < 5;
boolean greater = 5 > 3;
|===|`, "less: less;\ngreater: greater;"), map[string]expectedValue{
			"less":    {kind: ValueBoolean, bool: true},
			"greater": {kind: ValueBoolean, bool: true},
		}),
		Entry("equality operators", wrapScriptWithOutputFields(`|===|
boolean equal = 3 == 3;
boolean strict = 3 === 3;
boolean not_equal = 3 != 4;
boolean strict_not = 3 !== 4;
|===|`, "equal: equal;\nstrict: strict;\nnot_equal: not_equal;\nstrict_not: strict_not;"), map[string]expectedValue{
			"equal":      {kind: ValueBoolean, bool: true},
			"strict":     {kind: ValueBoolean, bool: true},
			"not_equal":  {kind: ValueBoolean, bool: true},
			"strict_not": {kind: ValueBoolean, bool: true},
		}),
		Entry("logical operators", wrapScriptWithOutputFields(`|===|
boolean result = true && false || true;
boolean not = !false;
|===|`, "result: result;\nnot: not;"), map[string]expectedValue{
			"result": {kind: ValueBoolean, bool: true},
			"not":    {kind: ValueBoolean, bool: true},
		}),
		Entry("ternary operator", wrapScriptWithOutputFields(`|===|
int value = true ? 1 : 2;
|===|`, "value: value;"), map[string]expectedValue{
			"value": {kind: ValueInt, int64: 1},
		}),
	)

	DescribeTable("rejects invalid operator usage",
		func(file string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
		},
		Entry("boolean with bitwise", wrapScriptWithOutputFields(`|===|
int value = true & false;
|===|`, "value: value;")),
		Entry("numeric with logical", wrapScriptWithOutputFields(`|===|
boolean value = 1 && 2;
|===|`, "value: value;")),
		Entry("string comparison", wrapScriptWithOutputFields(`|===|
boolean value = "a" < "b";
|===|`, "value: value;")),
		Entry("mixed equality", wrapScriptWithOutputFields(`|===|
boolean value = 1 == true;
|===|`, "value: value;")),
		Entry("ternary branch mismatch", wrapScriptWithOutputFields(`|===|
int value = true ? 1 : 2.0;
|===|`, "value: value;")),
	)

	DescribeTable("returns array and record results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("array literal", wrapScriptWithOutputFields(`|===|
int base = 2 + 3;
array<int> result = [base, base + 1, base + 2];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 5},
			{kind: ValueInt, int64: 6},
			{kind: ValueInt, int64: 7},
		}}),
		Entry("nested arrays", wrapScriptWithOutputFields(`|===|
int base = 1 + 2;
array<array<int> > result = [[base, base + 1], [base + 2, base + 3]];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 3},
				{kind: ValueInt, int64: 4},
			}},
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 5},
				{kind: ValueInt, int64: 6},
			}},
		}}),
		Entry("record literal", wrapScriptWithOutputFields(`|===|
schema User = { name: string; age: int; };
int base = 20 + 10;
User result = { name: "Ada"; age: base; };
|===|`, "result: result;"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("nested record literal", wrapScriptWithOutputFields(`|===|
schema Inner = { value: int; };
schema Outer = { inner: Inner; };
int base = 8 + 2;
Outer result = { inner: { value: base; }; };
|===|`, "result: result;"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"inner": {kind: ValueRecord, record: map[string]expectedValue{
				"value": {kind: ValueInt, int64: 10},
			}},
		}}),
		Entry("array of records", wrapScriptWithOutputFields(`|===|
schema Point = { x: int; y: int; };
int base = 1 + 1;
array<Point> result = [
  { x: base; y: base + 1; },
  { x: base + 2; y: base + 3; }
];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueRecord, record: map[string]expectedValue{
				"x": {kind: ValueInt, int64: 2},
				"y": {kind: ValueInt, int64: 3},
			}},
			{kind: ValueRecord, record: map[string]expectedValue{
				"x": {kind: ValueInt, int64: 4},
				"y": {kind: ValueInt, int64: 5},
			}},
		}}),
		Entry("self reference", wrapScriptWithOutputFields(`|===|
int base = 3 * 4;
|===|`, "base: base;\nresult: $self.base + base;"), expectedValue{kind: ValueInt, int64: 24}),
	)

	DescribeTable("returns inline output expressions",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("inline int expression", `[output = data] { result: 2 + 3 * 4; }`, expectedValue{kind: ValueInt, int64: 14}),
		Entry("inline float expression", `[output = data] { result: 2.5 + 1.5; }`, expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("inline boolean expression", `[output = data] { result: 2 < 3 && true; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("inline string expression", `[output = data] { result: "hello"; }`, expectedValue{kind: ValueString, string: "hello"}),
		Entry("inline record expression", `[output = data] { result: { name: "Ada"; age: 30; }; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("inline array expression", `[output = data] { result: [1, 2, 3]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 1},
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
		}}),
		Entry("inline nested array expression", `[output = data] { result: [[1, 2], [3, 4]]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 1},
				{kind: ValueInt, int64: 2},
			}},
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 3},
				{kind: ValueInt, int64: 4},
			}},
		}}),
		Entry("inline optional output field", `[output = data] { result?: 1 + 1; }`, expectedValue{kind: ValueInt, int64: 2}),
	)

	DescribeTable("returns inline output blocks with multiple fields",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("multiple fields and self reference", `[output = data] { base: 2 + 2; result: $self.base * 3; }`, map[string]expectedValue{
			"base":   {kind: ValueInt, int64: 4},
			"result": {kind: ValueInt, int64: 12},
		}),
	)

	DescribeTable("returns self reference results by depth",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("level 1", `[output = data] { base: 4; result: $self.base; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("level 2", `[output = data] { profile: { name: "Ada"; }; result: $self.profile.name; }`, expectedValue{kind: ValueString, string: "Ada"}),
		Entry("level 3", `[output = data] { profile: { details: { age: 30; }; }; result: $self.profile.details.age; }`, expectedValue{kind: ValueInt, int64: 30}),
		Entry("level 4", `[output = data] { tree: { branch: { leaf: { value: 9; }; }; }; result: $self.tree.branch.leaf.value; }`, expectedValue{kind: ValueInt, int64: 9}),
		Entry("level 5", `[output = data] { a: { b: { c: { d: { e: true; }; }; }; }; result: $self.a.b.c.d.e; }`, expectedValue{kind: ValueBoolean, bool: true}),
	)

	DescribeTable("returns mixed self reference results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("arithmetic with chained fields", `[output = data] { base: 4; doubled: $self.base * 2; result: $self.doubled + $self.base; }`, expectedValue{kind: ValueInt, int64: 12}),
		Entry("conditional with self", `[output = data] { enabled: true; result: $self.enabled ? "on" : "off"; }`, expectedValue{kind: ValueString, string: "on"}),
		Entry("array literal with self", `[output = data] { base: 2; result: [$self.base, $self.base + 1, $self.base + 2]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
			{kind: ValueInt, int64: 4},
		}}),
		Entry("record literal with self", `[output = data] { name: "Ada"; result: { display: $self.name; active: true; }; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"display": {kind: ValueString, string: "Ada"},
			"active":  {kind: ValueBoolean, bool: true},
		}}),
		Entry("nested self and operators", `[output = data] { profile: { score: 3; }; result: $self.profile.score * 4 + 1; }`, expectedValue{kind: ValueInt, int64: 13}),
	)

	DescribeTable("rejects invalid self references",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("future field reference", `[output = data] { result: $self.base; base: 4; }`, "unknown self reference"),
		Entry("nested path through non record", `[output = data] { base: 4; result: $self.base.value; }`, "non-record"),
	)

	DescribeTable("rejects inline arrays with mixed types",
		func(file string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
		},
		Entry("mixed primitive types", `[output = data] { result: [1, "two"]; }`),
		Entry("mixed numeric types", `[output = data] { result: [1, 2.0]; }`),
		Entry("mixed nested array types", `[output = data] { result: [[1], ["two"]]; }`),
	)
})
