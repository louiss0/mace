package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Output schema", func() {
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
}`, map[expectedSchemaField]SchemaType{{name: "name"}: schemaPrimitive("string"), {name: "age", optional: true}: schemaPrimitive("int")}),
		Entry("nested array fields", `[output = schema]
{
  names: array<string>;
  matrix: array<array<int>>;
}`, map[expectedSchemaField]SchemaType{{name: "names"}: schemaArray(schemaPrimitive("string")), {name: "matrix"}: schemaArray(schemaArray(schemaPrimitive("int")))}),
	)

	It("accepts output that matches a schema", func() {
		processor := New()
		_, err := processor.Process(`|===|
schema User: { name: string; age?: int; };
string name = "Ada";
|===|
[output = data, schema = User]
{ name: (name); }`)
		tAssert.NoError(err)
	})

	It("rejects output that violates a schema", func() {
		processor := New()
		_, err := processor.Process(`|===|
schema User: { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; }`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "missing required field")
	})
})
