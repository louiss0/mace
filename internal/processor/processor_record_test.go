package processor

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Record", func() {
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
