package processor

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
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

var _ = Describe("Input records", func() {
	It("parses injection records through the compatibility helper", func() {
		record, err := ParseInjectionRecord(`{ name: "Ada"; enabled: true; }`)
		tAssert.NoError(err)
		assertExpectedValue(record["name"], expectedValue{kind: ValueString, string: "Ada"})
		assertExpectedValue(record["enabled"], expectedValue{kind: ValueBoolean, bool: true})
	})
	It("rejects trailing tokens after the record literal", func() {
		_, err := ParseInputRecord(`{ a: 1; } garbage`)
		tAssert.ErrorContains(err, "unexpected token after expression")
	})
})

var _ = Describe("Imports", func() {
	DescribeTable("merges imported declarations",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("imports types and schemas", `|===|
from "fixtures/processor/imports/base.mace" import Name, User;
Name name = "Ada";
User result = { name: name; age: 30; };
|===|
[output = data]
{ result: result; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("imports values surfaced through output", `|===|
from "fixtures/processor/imports/values.mace" import count;
|===|
[output = data]
{ result: count + 2; }`, expectedValue{kind: ValueInt, int64: 5}),
		Entry("imports schemas and aliases from a public contract fixture", `|===|
from "fixtures/processor/imports/contracts.mace" import ID, Team;
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

	It("imports variant aliases reused across files", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-variant-import-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "shared.mace", `|===|
type Identity: variant[string, int];
|===|
[output = schema]
{
  Identity: Identity;
}`)
		processor := New()
		result, err := processor.ProcessFile(writeFixtureFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import Identity;
Identity first = "Ada";
Identity second = 42;
|===|
[output = data]
{
  result: {
    first: first;
    second: second;
  };
}`))
		tAssert.NoError(err)

		actual := requireOutputValue(result, "result")
		assertExpectedValue(actual, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"first":  {kind: ValueString, string: "Ada"},
			"second": {kind: ValueInt, int64: 42},
		}})
	})

	It("rejects imported schema output files that declare script variables", func() {
		workspace, err := os.MkdirTemp("", "mace-import-schema-output-variable-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "producer.mace", `|===|
schema User: { name: string; };
string local = "Ada";
|===|
[output = schema]
{
  User: User;
}`)
		consumer := writeFixtureFile(workspace, "consumer.mace", `|===|
from "./producer.mace" import User;
|===|
[output = data]
{
  result: { name: "Ada"; };
}`)

		processor := New()
		_, err = processor.ProcessFile(consumer)
		tAssert.Error(err)
		tAssert.ErrorContains(err, `script variable "local" is not allowed when output = schema`)
	})

	DescribeTable("keeps hidden declarations internal",
		func(file string, message string) {
			processor := New()
			_, err := processor.ProcessInDir(file, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("hidden type is not importable", `|===|
from "fixtures/processor/imports/base.mace" import Internal;
|===|
[output = data] {}`, "imported identifier"),
		Entry("hidden schema is not importable", `|===|
from "fixtures/processor/imports/base.mace" import Secret;
|===|
[output = data] {}`, "imported identifier"),
		Entry("hidden value is not importable", `|===|
from "fixtures/processor/imports/values.mace" import hidden;
|===|
[output = data] {}`, "imported identifier"),
		Entry("hidden schema in a data fixture is not importable", `|===|
from "fixtures/processor/imports/metrics.mace" import Hidden;
|===|
[output = data] {}`, "imported identifier"),
	)

	DescribeTable("processes imported files",
		func(path string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessFileInDir(path, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("resolves imports relative to file", "../../fixtures/processor/imports/consumer.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 27},
		}}),
		Entry("resolves schema_file relative to file", "../../fixtures/processor/schema_file/consumer.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
		}}),
	)

	DescribeTable("processes practical choice fixtures",
		func(path string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessFileInDir(path, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("deployment environment choices", "../../fixtures/processor/choices/deployment.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"app":         {kind: ValueString, string: "billing-api"},
			"environment": {kind: ValueString, string: "prod"},
			"region":      {kind: ValueString, string: "us-east-1"},
			"replicas":    {kind: ValueInt, int64: 4},
		}}),
		Entry("nested permission choices", "../../fixtures/processor/choices/permissions.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"role":       {kind: ValueString, string: "admin"},
			"permission": {kind: ValueString, string: "approve"},
			"resource":   {kind: ValueString, string: "invoice"},
		}}),
		Entry("mixed scalar shipping choices", "../../fixtures/processor/choices/shipping.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"order_id":           {kind: ValueString, string: "ORD-1001"},
			"method":             {kind: ValueString, string: "express"},
			"package_tier":       {kind: ValueInt, int64: 2},
			"signature_required": {kind: ValueBoolean, bool: true},
		}}),
		Entry("composed contact channel choices", "../../fixtures/processor/choices/mixed_choices.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"customer_id": {kind: ValueString, string: "CUST-42"},
			"preferred":   {kind: ValueString, string: "email"},
			"fallback":    {kind: ValueString, string: "chat"},
		}}),
		Entry("choice nested inside variant", "../../fixtures/processor/choices/choice_variant.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"reviewer": {kind: ValueString, string: "ada"},
			"outcome":  {kind: ValueString, string: "approved"},
			"note":     {kind: ValueString, string: "ready to ship"},
		}}),
	)

	It("processes nested variable array access fixtures", func() {
		processor := New()
		result, err := processor.Process(`|============================================================|
array<int> level1 = [1];
array<array<int>> level2 = [[2]];
array<array<array<int>>> level3 = [[[3]]];
array<array<array<array<int>>>> level4 = [[[[4]]]];
array<array<array<array<array<int>>>>> level5 = [[[[[5]]]]];
|============================================================|
[output = data]
{
  level1: level1[0],
  level2: level2[0][0],
  level3: level3[0][0][0],
  level4: level4[0][0][0][0],
  level5: level5[0][0][0][0][0],
}
`)
		tAssert.NoError(err)
		assertExpectedOutput(result, map[string]expectedValue{
			"level1": {kind: ValueInt, int64: 1},
			"level2": {kind: ValueInt, int64: 2},
			"level3": {kind: ValueInt, int64: 3},
			"level4": {kind: ValueInt, int64: 4},
			"level5": {kind: ValueInt, int64: 5},
		})
	})

	DescribeTable("rejects circular imports",
		func(path string) {
			processor := New()
			_, err := processor.ProcessFileInDir(path, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, "circular import")
		},
		Entry("cycle detected", "../../fixtures/processor/imports/cycle_a.mace"),
	)

	DescribeTable("rejects invalid imports",
		func(file string, message string) {
			processor := New()
			_, err := processor.ProcessInDir(file, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown imported identifier", `|===|
from "fixtures/processor/imports/base.mace" import Missing;
|===|
[output = data] {}`, "imported identifier"),
		Entry("duplicate import across declarations", `|===|
from "fixtures/processor/imports/base.mace" import Name;
from "fixtures/processor/imports/other.mace" import Name;
|===|
[output = data] {}`, "duplicate import"),
		Entry("import file missing", `|===|
from "fixtures/processor/imports/missing.mace" import Name;
|===|
[output = data] {}`, "unable to read import file"),
		Entry("import collides with local declaration", `|===|
from "fixtures/processor/imports/base.mace" import Name;
type Name: string;
|===|
[output = data] {}`, "duplicate declaration"),
	)

	It("rejects imports that escape the activation directory", func() {
		workspace, err := os.MkdirTemp("", "mace-import-root-boundary-*")
		tAssert.NoError(err)

		outsidePath := writeFixtureFile(workspace, "shared.mace", `[output = schema]
{
  User: string;
}`)
		consumerDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(consumerDir, 0o755))
		consumerPath := writeFixtureFile(consumerDir, "consumer.mace", `|===|
from "../shared.mace" import User;
|===|
[output = data]
{}`)

		processor := New()
		_, err = processor.ProcessFileInDir(consumerPath, consumerDir)
		tAssert.Error(err)
		tAssert.ErrorContains(err, `import path "../shared.mace" escapes root:`)
		tAssert.FileExists(outsidePath)
	})

	It("allows parent-relative imports during scoped processing", func() {
		workspace, err := os.MkdirTemp("", "mace-import-scope-parent-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "shared.mace", `[output = data]
{
  value: "Ada";
}`)

		consumerDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(consumerDir, 0o755))
		input := `|===|
from "../shared.mace" import value;
|===|
[output = data]
{
  result: value;
}`

		processor := New()
		result, err := processor.ProcessInScope(input, consumerDir, consumerDir)
		tAssert.NoError(err)
		assertExpectedOutput(result, map[string]expectedValue{
			"result": {kind: ValueString, string: "Ada"},
		})
	})

	DescribeTable("validates local schema_file output schema structure",
		func(schemaFile string, validOutput string, invalidOutput string, message string) {
			workspace, err := os.MkdirTemp("", "mace-schema-file-validation-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			writeFixtureFile(workspace, "schema.mace", schemaFile)

			processor := New()
			for _, directive := range []string{`[schema_file = "./schema.mace"]`, `[output = data, schema_file = "./schema.mace"]`} {
				_, err = processor.ProcessInDir(directive+"\n"+validOutput, workspace)
				tAssert.NoError(err)
			}

			_, err = processor.ProcessInDir(`[schema_file = "./schema.mace"]`+"\n"+invalidOutput, workspace)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("top-level fields with optional fields", `[output = schema]
{
  name: string;
  version: string;
  exports?: record<string>;
}`, `{
  name: "mace";
  version: "1.0.0";
}`, `{
  name: "mace";
}`, `missing required field "version"`),
		Entry("nested fields with optional fields", `[output = schema]
{
  user: {
    name: string;
    age?: int;
    personality: choice["nice", "naive", "hateful"];
  };
}`, `{
  user: {
    name: "Ada";
    personality: "nice";
  };
}`, `{
  name: "Ada";
  personality: "nice";
}`, `missing required field "user"`),
		Entry("many fields with records of known types", `|===|
schema Service: {
  image: string;
  replicas?: int;
};
|===|
[output = schema]
{
  services: record<Service>;
  labels?: record<string>;
  ports: record<int>;
}`, `{
  services: {
    api: { image: "nginx"; replicas: 2; };
    worker: { image: "worker"; };
  };
  ports: {
    api: 8080;
    worker: 9090;
  };
}`, `{
  services: {
    api: { image: "nginx"; replicas: "two"; };
  };
  ports: {
    api: 8080;
  };
}`, `type mismatch`),
		Entry("fields that have records as types", `[output = schema]
{
  user: {
    name: string;
    age?: int;
  };
  package: {
    name: string;
    version: string;
    exports: record<string>;
  };
  audit?: {
    created_by: string;
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: "./dist/index.js";
    };
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: 1;
    };
  };
}`, `type mismatch`),
	)

	It("rejects schema_file paths that escape the activation directory", func() {
		workspace, err := os.MkdirTemp("", "mace-schema-file-root-boundary-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "shared.mace", `|===|
schema User: { name: string; };
|===|
[output = schema]
{
  User: User;
}`)
		consumerDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(consumerDir, 0o755))
		consumerPath := writeFixtureFile(consumerDir, "consumer.mace", `[output = data, schema_file = "../shared.mace"]
{}`)

		processor := New()
		_, err = processor.ProcessFileInDir(consumerPath, consumerDir)
		tAssert.Error(err)
		tAssert.ErrorContains(err, `import path "../shared.mace" escapes root:`)
	})

	It("imports choice aliases exposed through schema output", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-choice-import-*")
		tAssert.NoError(err)

		sharedPath := writeFixtureFile(workspace, "shared.mace", `|===|
 type Fruit: choice["Apple", "Strawberry"];
|===|
[output = schema]
{
  Fruit: Fruit;
}`)
		consumerPath := writeFixtureFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import Fruit;
Fruit result = "Apple";
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

	It("imports remote mace files over http", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/shared.mace":
				_, _ = writer.Write([]byte(`[output = data]
{
  value: "Ada";
}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		input := fmt.Sprintf(`|===|
from %q import value;
|===|
[output = data]
{
  result: value;
}`, server.URL+"/shared.mace")

		processor := New()
		result, err := processor.ProcessInDir(input, "../..")
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Ada"})
	})

	DescribeTable("validates remote schema_file output schema structure over http",
		func(schemaFile string, validOutput string, invalidOutput string, message string) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/schema.mace":
					_, _ = writer.Write([]byte(schemaFile))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			processor := New()
			for _, directive := range []string{
				fmt.Sprintf(`[schema_file = %q]`, server.URL+"/schema.mace"),
				fmt.Sprintf(`[output = data, schema_file = %q]`, server.URL+"/schema.mace"),
			} {
				_, err := processor.ProcessInDir(directive+"\n"+validOutput, "../..")
				tAssert.NoError(err)
			}

			_, err := processor.ProcessInDir(fmt.Sprintf(`[schema_file = %q]`, server.URL+"/schema.mace")+"\n"+invalidOutput, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("top-level fields with optional fields", `[output = schema]
{
  name: string;
  version: string;
  exports?: record<string>;
}`, `{
  name: "mace";
  version: "1.0.0";
}`, `{
  name: "mace";
}`, `missing required field "version"`),
		Entry("nested fields with optional fields", `[output = schema]
{
  user: {
    name: string;
    age?: int;
    personality: choice["nice", "naive", "hateful"];
  };
}`, `{
  user: {
    name: "Ada";
    personality: "nice";
  };
}`, `{
  name: "Ada";
  personality: "nice";
}`, `missing required field "user"`),
		Entry("many fields with records of known types", `|===|
schema Service: {
  image: string;
  replicas?: int;
};
|===|
[output = schema]
{
  services: record<Service>;
  labels?: record<string>;
  ports: record<int>;
}`, `{
  services: {
    api: { image: "nginx"; replicas: 2; };
    worker: { image: "worker"; };
  };
  ports: {
    api: 8080;
    worker: 9090;
  };
}`, `{
  services: {
    api: { image: "nginx"; replicas: "two"; };
  };
  ports: {
    api: 8080;
  };
}`, `type mismatch`),
		Entry("fields that have records as types", `[output = schema]
{
  user: {
    name: string;
    age?: int;
  };
  package: {
    name: string;
    version: string;
    exports: record<string>;
  };
  audit?: {
    created_by: string;
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: "./dist/index.js";
    };
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: 1;
    };
  };
}`, `type mismatch`),
	)

	It("loads remote parse_file output schema records over http", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/shared.mace":
				_, _ = writer.Write([]byte(`[output = schema]
{
  User: { name: string; };
}`))
			case "/schema.mace":
				_, _ = writer.Write([]byte(`|===|
from "./shared.mace" import User;
|===|
[output = schema]
{
  user: User;
}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		processor := NewWithInput(map[string]Value{
			"user": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			}},
		})
		result, err := processor.ProcessInDir(fmt.Sprintf(`[output = data, parse_file = %q]
{
  result: user.name;
}`, server.URL+"/schema.mace"), server.URL)
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Ada"})
	})

	It("resolves relative imports inside remote mace files", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/shared/base.mace":
				_, _ = writer.Write([]byte(`[output = data]
{
  value: "Ada";
}`))
			case "/entry.mace":
				_, _ = writer.Write([]byte(`|===|
from "./shared/base.mace" import value;
|===|
[output = data]
{
  result: value;
}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		input := fmt.Sprintf(`|===|
from %q import result;
|===|
[output = data]
{
  result: result;
}`, server.URL+"/entry.mace")

		processor := New()
		result, err := processor.ProcessInDir(input, "../..")
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Ada"})
	})

	It("rejects remote import urls without a .mace suffix", func() {
		processor := New()
		_, err := processor.Process(`|===|
from "https://example.com/shared" import value;
|===|
[output = data] {}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "must end in .mace")
	})

	It("rejects remote schema_file urls without a .mace suffix", func() {
		processor := New()
		_, err := processor.Process(`[output = data, schema = User, schema_file = "https://example.com/schema"] {}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "must end in .mace")
	})

	It("imports a schema output as a named schema with import-as", func() {
		processor := NewWithInput(map[string]Value{
			"name":    {Kind: ValueString, String: "@code-fixer-23/cn-efs"},
			"version": {Kind: ValueString, String: "1.0.0"},
			"type":    {Kind: ValueString, String: "commonjs"},
		})
		result, err := processor.ProcessFile("../../fixtures/processor/import_as/consumer.mace")
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "@code-fixer-23/cn-efs"})
		assertExpectedValue(result.Output["version"], expectedValue{kind: ValueString, string: "1.0.0"})
		assertExpectedValue(result.Output["type"], expectedValue{kind: ValueString, string: "commonjs"})
	})

	It("imports a data output as a named record with import-as", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-import-as-data-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		sharedPath := filepath.Join(workspace, "shared.mace")
		tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  project: {
    name: "pi-prompt-form";
    root: "libs/pi-prompt-form";
  };
  workspace: {
    root: ".";
  };
}`), 0o644))

		documentPath := filepath.Join(workspace, "document.mace")
		tAssert.NoError(os.WriteFile(documentPath, []byte(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data]
{
  name: Shared.project.name;
  root: Shared.project.root;
  cwd: Shared.workspace.root;
}`), 0o644))

		result, err := New().ProcessFile(documentPath)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "pi-prompt-form"})
		assertExpectedValue(result.Output["root"], expectedValue{kind: ValueString, string: "libs/pi-prompt-form"})
		assertExpectedValue(result.Output["cwd"], expectedValue{kind: ValueString, string: "."})
	})

	DescribeTable("imports data outputs with import-as across nested levels",
		func(accessor string, expected expectedValue) {
			workspace, err := os.MkdirTemp("", "mace-processor-import-as-data-depth-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  level1: {
    value: "one";
    level2: {
      value: "two";
      level3: {
        value: "three";
        level4: {
          value: "four";
          level5: {
            value: "five";
          };
        };
      };
    };
  };
}`), 0o644))

			documentPath := filepath.Join(workspace, "document.mace")
			tAssert.NoError(os.WriteFile(documentPath, []byte(fmt.Sprintf(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data]
{
  result: %s;
}`, accessor)), 0o644))

			result, err := New().ProcessFile(documentPath)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expected)
		},
		Entry("level 1", "Shared.level1.value", expectedValue{kind: ValueString, string: "one"}),
		Entry("level 2", "Shared.level1.level2.value", expectedValue{kind: ValueString, string: "two"}),
		Entry("level 3", "Shared.level1.level2.level3.value", expectedValue{kind: ValueString, string: "three"}),
		Entry("level 4", "Shared.level1.level2.level3.level4.value", expectedValue{kind: ValueString, string: "four"}),
		Entry("level 5", "Shared.level1.level2.level3.level4.level5.value", expectedValue{kind: ValueString, string: "five"}),
	)

	DescribeTable("imports schema outputs with import-as across nested levels",
		func(accessor string, input Value, expected expectedValue) {
			workspace, err := os.MkdirTemp("", "mace-processor-import-as-schema-depth-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = schema]
{
  level1: {
    value: string;
    level2: {
      value: string;
      level3: {
        value: string;
        level4: {
          value: string;
          level5: {
            value: string;
          };
        };
      };
    };
  };
}`), 0o644))

			documentPath := filepath.Join(workspace, "document.mace")
			tAssert.NoError(os.WriteFile(documentPath, []byte(fmt.Sprintf(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data, parse = Shared]
{
  result: %s;
}`, accessor)), 0o644))

			processor := NewWithInput(map[string]Value{"level1": input})
			result, err := processor.ProcessFile(documentPath)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expected)
		},
		Entry("level 1", "level1.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "one"}),
		Entry("level 2", "level1.level2.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "two"}),
		Entry("level 3", "level1.level2.level3.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "three"}),
		Entry("level 4", "level1.level2.level3.level4.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "four"}),
		Entry("level 5", "level1.level2.level3.level4.level5.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "five"}),
	)

})

var _ = Describe("Import helper coverage", func() {
	It("covers export resolution helpers", func() {
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})

		resolved, err := resolveExportedTypeReference(ast.NamedType{Name: "Alias"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, resolved)
		resolved, err = resolveExportedTypeReference(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, resolved)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "Alias"}, types, schemas, map[string]struct{}{"Alias": {}}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{"User": {}})
		tAssert.Error(err)

		fields := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
		resolvedRecord, err := resolveExportedRecordType(fields, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(fields, resolvedRecord)

		_, err = resolveExportedTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "Missing"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "Alias"}, types, schemas, map[string]struct{}{"Alias": {}}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{"User": {}})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "Unknown"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(nil, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
	})

	It("covers import and schema export helpers", func() {
		workspace, err := os.MkdirTemp("", "processor-imports-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		schemaPath := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ name: string; }`)
		consumerPath := writeFixtureFile(workspace, "consumer.mace", `[output = data, schema_file = "schema.mace"]
{ name: "Ada"; }`)
		badPath := writeFixtureFile(workspace, "bad.mace", `{ name: 1; }`)
		invalidOutputPath := writeFixtureFile(workspace, "invalid-output.mace", `[output = data]
{ name: "Ada"; }`)
		circularA := writeFixtureFile(workspace, "circular-a.mace", `import "circular-b.mace";`)
		_ = writeFixtureFile(workspace, "circular-b.mace", `import "circular-a.mace";`)

		context := newProcessContext(workspace, workspace)
		declarations, err := loadSchemaFileDeclarations(schemaPath, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.NotNil(declarations)
		_, err = loadSchemaFileDeclarations(schemaPath, workspace, map[string]map[string]ast.Declaration{schemaPath: declarations}, map[string]struct{}{})
		tAssert.NoError(err)

		outputDecls, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		tAssert.NoError(err)
		tAssert.NotNil(outputDecls)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)
		tAssert.Error(err)

		loaded, err := loadOutputSchemaRecord(schemaPath, workspace, "schema_file")
		tAssert.NoError(err)
		tAssert.NotEmpty(loaded.Fields)
		_, err = loadOutputSchemaRecord(badPath, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(invalidOutputPath, workspace, "schema_file")
		tAssert.Error(err)

		exports, err := collectImportExports(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, context)
		tAssert.NoError(err)
		tAssert.NotNil(exports)
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}}, context)
		tAssert.Error(err)

		fieldDecl, err := schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "item", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "value", Type: ast.PrimitiveType{Name: "string"}}}}}, context)
		tAssert.NoError(err)
		tAssert.Equal(symbolKindSchema, fieldDecl.kind)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "item", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, context)
		tAssert.NoError(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "item", Type: ast.NamedType{Name: "Missing"}}, context)
		tAssert.NoError(err)

		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Mode: ast.OutputModeData}, context)
		tAssert.NoError(err)
		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.Identifier{Name: "missing"}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, context)
		tAssert.Error(err)

		_, err = loadImportExports(consumerPath, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = loadImportExports(circularA, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		_, err = loadSchemaFileDeclarations(writeFixtureFile(workspace, "circular-check.mace", `import "circular-check.mace";`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)

		imported, err := resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Schema"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.NotEmpty(imported)

		ctx := newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.variables.Add("name", valueType{kind: ValueString})
		ctx.environment.Add("name", Value{Kind: ValueString, String: "Ada"})
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.NamedType{Name: "User"}}, {Name: "Map", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}}}, ctx)
		tAssert.NoError(err)
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "Broken", Type: ast.NamedType{Name: "Broken"}}}}, ctx)
		tAssert.Error(err)
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeData, DataFields: []ast.OutputField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, ctx)
		tAssert.NoError(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "map", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx)
		tAssert.NoError(err)
		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.Identifier{Name: "name"}}, ast.OutputBlock{Mode: ast.OutputModeData}, ctx)
		tAssert.NoError(err)
		_, err = importFileAsDeclaration("Local", map[string]importedDeclaration{"bad": {kind: symbolKindImport}})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"bad path"`}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		ctx.schemas.Add("Broken", ast.RecordType{Fields: []ast.SchemaField{{Name: "broken", Type: ast.NamedType{Name: "Missing"}}, {Name: "ok", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("Broken", symbolKindSchema)
		ctx.environment.Add("broken", Value{Kind: ValueString, String: "x"})
		ctx.variables.Add("broken", valueType{kind: ValueString})
		_, ok := ctx.schemas.Get("Broken")
		tAssert.True(ok)
		proc := NewWithInput(map[string]Value{"broken": {Kind: ValueString, String: "x"}})
		err = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Broken"}}}, &ctx)
		tAssert.Error(err)
		ctx = newProcessContext(workspace, workspace)
		ctx.schemas.Add("Broken", ast.RecordType{Fields: []ast.SchemaField{{Name: "broken", Type: ast.NamedType{Name: "Missing"}}}})
		ctx.symbols.Add("Broken", symbolKindSchema)
		err = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Broken"}}}, &ctx)
		tAssert.Error(err)
		ctx = newProcessContext(workspace, workspace)
		ctx.schemas.Add("Broken", ast.RecordType{Fields: []ast.SchemaField{{Name: "broken", Type: ast.NamedType{Name: "Missing"}}}})
		ctx.symbols.Add("Broken", symbolKindSchema)
		ctx.environment.Add("broken", Value{Kind: ValueString, String: "x"})
		ctx.variables.Add("broken", valueType{kind: ValueString})
		err = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Broken"}}}, &ctx)
		tAssert.Error(err)
		proc = NewWithInput(map[string]Value{"broken": {Kind: ValueString, String: "x"}, "input": {Kind: ValueString, String: "x"}})
		err = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Broken"}}}, &ctx)
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"bad path"`}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./scriptonly.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}, {Path: ast.StringLiteral{Lexeme: `"./scriptonly.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
	})

	It("covers remaining import and parse error branches", func() {
		workspace, err := os.MkdirTemp("", "processor-errors-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		badParse := writeFixtureFile(workspace, "bad-parse.mace", `not valid`)
		badOutput := writeFixtureFile(workspace, "bad-output.mace", `[output = data]
{ result: 1; }`)
		schemaOutput := writeFixtureFile(workspace, "schema-output.mace", `[output = schema]
{ User: { name: string, }, }`)

		_, err = loadOutputSchemaRecord(badParse, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(badOutput, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(schemaOutput, workspace, "schema_file")
		tAssert.NoError(err)

		proc := NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}})
		ctx := newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.symbols.Add("input", symbolKindVariable)
		ctx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		ctx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		err = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx)
		tAssert.Error(err)
		ctx = newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.variables.Add("name", valueType{kind: ValueString})
		ctx.environment.Add("name", Value{Kind: ValueString, String: "Ada"})
		err = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx)
		tAssert.NoError(err)
		proc2 := NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "required": {Kind: ValueInt, Int: 1}})
		ctx = newProcessContext(workspace, workspace)
		ctx.schemas.Add("input", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "required", Type: ast.PrimitiveType{Name: "int"}}, {Name: "missing", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}})
		err = proc2.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "input"}}}, &ctx)
		tAssert.NoError(err)
		ctx = newProcessContext(workspace, workspace)
		ctx.schemas.Add("input", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("input", symbolKindVariable)
		ctx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "input"})
		ctx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		err = proc2.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "input"}}}, &ctx)
		tAssert.Error(err)
		ctx = newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.symbols.Add("name", symbolKindVariable)
		ctx.variables.Add("name", valueType{kind: ValueString})
		ctx.environment.Add("name", Value{Kind: ValueString, String: "Ada"})
		err = proc2.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx)
		tAssert.Error(err)
	})

	It("covers remaining import, directive, and validation branches", func() {
		workspace, setupErr := os.MkdirTemp("", "processor-remaining-*")
		tAssert.NoError(setupErr)
		defer func() { _ = os.RemoveAll(workspace) }()

		remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/schema.mace" {
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer remoteServer.Close()

		localSchema := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ Local: string; }`)
		localParse := writeFixtureFile(workspace, "parse.mace", `[output = schema]
{ Parsed: string; }`)
		_ = writeFixtureFile(workspace, "cycle-a.mace", `from "./cycle-b.mace" import User;
[output = schema]
{ User: string; }`)
		_ = writeFixtureFile(workspace, "cycle-b.mace", `from "./cycle-a.mace" import User;
[output = schema]
{ User: string; }`)
		_ = writeFixtureFile(workspace, "bad-script.mace", `|===|
string value = "a";
|===|
[output = schema]
{ value: string; }`)
		_ = writeFixtureFile(workspace, "bad-parse.mace", `this is not valid mace`)

		oldGetwd := getwd
		getwd = func() (string, error) { return "", errors.New("cwd failure") }
		_, err := New().ProcessOutputBlock(`[output = data] {}`, ScriptResult{})
		tAssert.NoError(err)
		getwd = oldGetwd

		_, err = resolveImportPath(workspace, filepath.Join(workspace, "abs.mace"))
		tAssert.Error(err)
		_, err = resolveImportPath(remoteServer.URL+"/", "./schema.mace")
		tAssert.NoError(err)
		_, err = resolveBoundedPath(workspace, workspace, "../escape.mace")
		tAssert.Error(err)
		_, err = resolveBoundedPath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace")
		tAssert.NoError(err)
		_, _ = resolveBoundedRemotePath(remoteServer.URL+"/", remoteServer.URL+"/", "../escape.mace", remoteServer.URL+"/escape.mace")
		_, _ = resolveBoundedRemotePath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace", "https://other.example.com/schema.mace")
		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal(remoteServer.URL+"/", formatImportRoot(remoteServer.URL+"/"))
		tAssert.Contains(formatImportRoot(workspace), filepath.Base(workspace))
		_, ok := parseRemoteURL("ftp://example.com/file.mace")
		tAssert.False(ok)
		_, ok = parseRemoteURL("https:///missing-host")
		tAssert.False(ok)
		_, ok = parseRemoteURL(remoteServer.URL + "/schema.mace")
		tAssert.True(ok)
		_, err = readMaceSource(filepath.Join(workspace, "missing.mace"))
		tAssert.Error(err)
		_, err = readMaceSource(remoteServer.URL + "/missing.mace")
		tAssert.Error(err)

		cache := map[string]map[string]importedDeclaration{localSchema: {"Local": {name: "Local", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}}}
		decls, err := loadImportExports(localSchema, workspace, true, cache, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Contains(decls, "Local")
		_, err = loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadImportExports(writeFixtureFile(workspace, "invalid-import.mace", `from "./bad-parse.mace" import Missing;
[output = schema]
{ Thing: string; }`), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadImportExports(writeFixtureFile(workspace, "script-var.mace", `|===|
string value = "a";
|===|
[output = schema]
{ Thing: string; }`), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadImportExports(writeFixtureFile(workspace, "parse-error.mace", `not valid`), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadImportExports(localSchema, workspace, true, cache, map[string]struct{}{localSchema: {}})
		tAssert.NoError(err)

		_, err = loadSchemaFileDeclarations(filepath.Join(workspace, "missing-schema.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(writeFixtureFile(workspace, "schema-parse-error.mace", `not valid`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(writeFixtureFile(workspace, "schema-cycle-a.mace", `from "./schema-cycle-b.mace" import User;
[output = schema]
{ User: string; }`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)

		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		tAssert.Error(err)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.txt"`}}, workspace, workspace)
		tAssert.Error(err)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"bad-parse.mace"`}}, workspace, workspace)
		tAssert.Error(err)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)

		_, _ = loadOutputSchemaRecord(localSchema, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localParse, workspace, "schema_file")

		ctx := newProcessContext(workspace, workspace)
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("Thing", symbolKindType)
		ctx.types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
		ctx.symbols.Add("record", symbolKindVariable)
		ctx.variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		ctx.environment.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		ctx.symbols.Add("input", symbolKindVariable)
		ctx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		ctx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})

		_, _ = prepareOutputContext(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}}, ctx)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctx)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctx)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}}, newProcessContext(workspace, workspace))

		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, nil, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}, nil, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState(nil, &ast.ScriptBlock{Items: []ast.Declaration{nil}}, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		fields := []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, ctx)
		tAssert.NoError(err)
		_, err = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: fields}, ctx)
		tAssert.NoError(err)

		schemaField := ast.OutputSchemaField{Name: "profile", Type: ast.NamedType{Name: "User"}}
		_, err = schemaFieldImportDeclaration(schemaField, ctx)
		tAssert.NoError(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "count", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx)
		tAssert.NoError(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "invalid", Type: nil}, ctx)
		tAssert.Error(err)

		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx)
		tAssert.NoError(err)
		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, ctx)
		tAssert.Error(err)
		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: nil}, ast.OutputBlock{}, ctx)
		tAssert.Error(err)

		_ = sanitizeImportedValueType(valueType{kind: ValueRecord, schemaName: "User", element: &valueType{kind: ValueString}, members: []valueType{{kind: ValueInt}}}, ctx.schemas)
		_ = typeReferenceFromValueType(valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, element: &valueType{kind: ValueInt}})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}})
		_ = typeReferenceFromValueType(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}})
		_ = typeReferenceFromValueType(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})
	})

	It("covers remaining import and output helper branches", func() {
		root, err := os.MkdirTemp("", "processor-cover-*")
		tAssert.NoError(err)
		defer func() { tAssert.NoError(os.RemoveAll(root)) }()

		baseDir := filepath.Join(root, "imports")
		tAssert.NoError(os.MkdirAll(baseDir, 0o755))

		baseSource := `|===|
schema User: {
  name: string,
};
|===|
[output = schema]
{
  User: User,
}`
		tAssert.NoError(os.WriteFile(filepath.Join(baseDir, "base.mace"), []byte(baseSource), 0o644))

		consumerSource := `|===|
from "./base.mace" import User;
string name = "Ada";
User result = {
  name: name,
};
|===|
[output = data]
{ result: result, }`
		tAssert.NoError(os.WriteFile(filepath.Join(baseDir, "consumer.mace"), []byte(consumerSource), 0o644))

		file := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./base.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}
		imports, err := resolveImportsWithState(file, baseDir, baseDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Len(imports, 1)

		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./missing.txt"`}}}}, baseDir, baseDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		_, err = loadImportExports(filepath.Join(baseDir, "consumer.mace"), baseDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = loadSchemaFileDeclarations(filepath.Join(baseDir, "base.mace"), baseDir, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.NoError(err)

		directives := []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"./base.mace"`}}
		loaded, err := resolveSchemaFileDeclarations(directives, baseDir, baseDir)
		tAssert.NoError(err)
		tAssert.NotEmpty(loaded)

		_, err = loadOutputSchemaRecord(filepath.Join(baseDir, "base.mace"), baseDir, "schema_file")
		tAssert.NoError(err)

		context := newProcessContext(baseDir, baseDir)
		context.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		context.variables.Add("name", valueType{kind: ValueString})
		context.environment.Add("name", Value{Kind: ValueString, String: "Ada"})
		exported, err := schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, context)
		tAssert.NoError(err)
		tAssert.Equal(symbolKindSchema, exported.kind)

		fieldType, err := exportedOutputFieldType(ast.OutputField{Name: "result", Value: ast.Identifier{Name: "name"}}, ast.OutputBlock{}, context)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, fieldType.kind)
	})
})

var _ = Describe("Path helpers", func() {
	It("clones and preserves nested contexts", func() {
		original := newProcessContext("/base", "/root")
		original.optionalParseVars["x"] = struct{}{}
		cloned := original.clone()
		tAssert.Equal(original.importBaseDir, cloned.importBaseDir)
		tAssert.Equal(original.importRootDir, cloned.importRootDir)
		tAssert.NotNil(cloned.symbols)
		tAssert.NotNil(cloned.types)
		tAssert.NotNil(cloned.schemas)
		tAssert.NotNil(cloned.variables)
		tAssert.NotNil(cloned.environment)
		tAssert.Contains(cloned.optionalParseVars, "x")
	})

	It("formats local and remote import roots", func() {
		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("workspace/", formatImportRoot(filepath.Join("/tmp", "workspace")))
		tAssert.Equal("https://example.com/root/", formatImportRoot("https://example.com/root/"))
	})

	It("clones empty process contexts safely", func() {
		var empty processContext
		cloned := empty.clone()
		tAssert.Equal(processContext{}, cloned)
	})

	It("parses remote URLs and derives base directories", func() {
		remote, ok := parseRemoteURL("https://example.com/root/file.mace")
		tAssert.True(ok)
		tAssert.Equal("https", remote.Scheme)
		tAssert.Equal("example.com", remote.Host)

		_, ok = parseRemoteURL("file:///tmp/file.mace")
		tAssert.False(ok)
		tAssert.Equal("https://example.com/root/", basePathDir("https://example.com/root/file.mace"))
		tAssert.Equal(filepath.Dir("/tmp/file.mace"), basePathDir("/tmp/file.mace"))
	})

	It("resolves import paths within and outside bounded scopes", func() {
		resolved, err := resolveImportPath("/workspace", "nested/file.mace")
		tAssert.NoError(err)
		tAssert.Contains(resolved, "nested")

		resolved, err = resolveImportPath("https://example.com/root/", "child/file.mace")
		tAssert.NoError(err)
		tAssert.Equal("https://example.com/root/child/file.mace", resolved)

		absolutePath, pathErr := filepath.Abs("absolute/file.mace")
		tAssert.NoError(pathErr)
		_, err = resolveImportPath("/workspace", absolutePath)
		tAssert.ErrorContains(err, "must be relative")

		bounded, err := resolveImportPathInScope("/workspace", "/workspace", "nested/file.mace", true)
		tAssert.NoError(err)
		tAssert.Contains(bounded, "nested")

		_, err = resolveBoundedPath("/workspace", "/workspace", "../escape.mace")
		tAssert.ErrorContains(err, "escapes root")

		boundedRemote, err := resolveBoundedRemotePath("https://example.com/root/", "https://example.com/root/", "child/file.mace", "https://example.com/root/child/file.mace")
		tAssert.NoError(err)
		tAssert.Equal("https://example.com/root/child/file.mace", boundedRemote)
		_, err = resolveBoundedRemotePath("https://example.com/root/", "https://example.com/root/", "child/file.mace", "https://evil.example.com/root/child/file.mace")
		tAssert.ErrorContains(err, "escapes root")
	})

	It("validates mace source paths", func() {
		tAssert.NoError(validateMaceSourcePath("config.mace"))
		tAssert.ErrorContains(validateMaceSourcePath("config.txt"), "must end in .mace")
	})

	It("reads local and remote mace sources", func() {
		localDir, err := os.MkdirTemp("", "mace-local-*")
		tAssert.NoError(err)
		localPath := filepath.Join(localDir, "config.mace")
		tAssert.NoError(os.WriteFile(localPath, []byte("local"), 0o600))

		contents, err := readMaceSource(localPath)
		tAssert.NoError(err)
		tAssert.Equal("local", contents)

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte("remote"))
		}))
		defer server.Close()

		contents, err = readMaceSource(server.URL + "/config.mace")
		tAssert.NoError(err)
		tAssert.Equal("remote", contents)
	})
})

var _ = Describe("Path helper coverage", func() {
	It("covers remaining path and import helper branches", func() {
		workspace, setupErr := os.MkdirTemp("", "processor-paths-*")
		tAssert.NoError(setupErr)
		var err error
		defer func() { _ = os.RemoveAll(workspace) }()

		emptyImports, err := resolveImportsWithState(ast.File{}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Nil(emptyImports)

		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"bad path"`}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		_, err = loadImportExports(filepath.Join(workspace, "does-not-exist.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		brokenContext := newProcessContext(workspace, workspace)
		brokenContext.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: nil}}})
		brokenContext.symbols.Add("User", symbolKindSchema)
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: `"User"`}}}, brokenContext)
		tAssert.Error(err)

		remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/root/imports/base.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Thing: string; }`)
			case "/root/imports/child.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Child: string; }`)
			case "/import.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer remoteServer.Close()

		localFile := writeFixtureFile(workspace, "source.mace", `[output = schema]
{ Local: string; }`)
		_, ok := parseRemoteURL("https://example.com/root/file.mace")
		tAssert.True(ok)
		_, err = resolveImportPath(workspace, "child.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath(remoteServer.URL+"/root/", "./imports/base.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath(workspace, localFile)
		tAssert.Error(err)
		_, err = resolveBoundedPath(workspace, workspace, "../escape.mace")
		tAssert.Error(err)
		resolvedRemote, err := resolveBoundedPath(remoteServer.URL+"/root/", remoteServer.URL+"/root/", "./imports/base.mace")
		tAssert.NoError(err)
		tAssert.Contains(resolvedRemote, "/root/imports/base.mace")
		_, err = resolveBoundedRemotePath(remoteServer.URL+"/root/", remoteServer.URL+"/root/", "../escape.mace", remoteServer.URL+"/escape.mace")
		tAssert.Error(err)
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal(remoteServer.URL+"/root/", formatImportRoot(remoteServer.URL+"/root/"))
		tAssert.Contains(formatImportRoot(workspace), filepath.Base(workspace))
		localContents, err := readMaceSource(localFile)
		tAssert.NoError(err)
		tAssert.Contains(localContents, "Local")
		_, err = readMaceSource(remoteServer.URL + "/missing.mace")
		tAssert.Error(err)
		_, ok = parseRemoteURL("ftp://example.com/file.mace")
		tAssert.False(ok)
		_, ok = parseRemoteURL("https:///missing-host")
		tAssert.False(ok)

		imports := []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Local"}}}}
		resolvedImports, err := resolveImportsWithState(ast.File{Imports: imports}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Len(resolvedImports, 1)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}, {Path: ast.StringLiteral{Lexeme: `"./source.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		decl, err := importFileAsDeclaration("thing", map[string]importedDeclaration{"Local": {name: "Local", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}})
		tAssert.NoError(err)
		tAssert.Equal(symbolKindVariable, decl.kind)
		decl, err = importFileAsDeclaration("thing", map[string]importedDeclaration{"Local": {name: "Local", kind: symbolKindSchema, record: ast.RecordType{Fields: []ast.SchemaField{{Name: "Local", Type: ast.PrimitiveType{Name: "string"}}}}}})
		tAssert.NoError(err)
		tAssert.Equal(symbolKindSchema, decl.kind)
		_, err = importFileAsDeclaration("thing", map[string]importedDeclaration{"Local": {name: "Local", kind: symbolKind(99)}})
		tAssert.Error(err)

		ref := typeReferenceFromValueType(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{kind: ValueArray, element: &valueType{kind: ValueInt}})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{kind: ValueRecord, schemaName: "User"})
		tAssert.NotNil(ref)
		ref = typeReferenceFromValueType(valueType{})
		tAssert.NotNil(ref)
	})

	It("covers remaining path and import edge branches", func() {
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("root/", formatImportRoot("/tmp/root"))
		tAssert.Equal("https://example.com/root", formatImportRoot("https://example.com/root"))
		_, ok := parseRemoteURL("not-a-url")
		tAssert.False(ok)
		absPath, err := filepath.Abs("path.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath(".", absPath)
		tAssert.Error(err)
		root, err := os.MkdirTemp("", "processor-remote-*")
		tAssert.NoError(err)
		defer func() { tAssert.NoError(os.RemoveAll(root)) }()
		_, err = resolveBoundedPath(root, root, "../outside.mace")
		tAssert.Error(err)
		_, err = resolveBoundedRemotePath("https://example.com/base", "https://example.com/root", "https://evil.com/x.mace", "https://evil.com/x.mace")
		tAssert.Error(err)

		missing := filepath.Join(root, "missing.mace")
		_, err = readMaceSource(missing)
		tAssert.Error(err)
		_, err = loadImportExports(missing, root, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(missing, root, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(missing, root, "schema_file")
		tAssert.Error(err)

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, "hello")
		}))
		defer httpServer.Close()
		body, err := readMaceSource(httpServer.URL)
		tAssert.NoError(err)
		tAssert.Equal("hello", body)

		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./base.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./base.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}, root, root, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./base.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, root, root, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
	})
})

var _ = Describe("Validation helpers", func() {
	It("extracts guarded names and validates guarded output expressions", func() {
		guarded := extractGuardedNames(ast.InfixExpression{
			Left:     ast.StringLiteral{Lexeme: `"profile"`},
			Operator: lexer.TokenIn,
			Right:    ast.Identifier{Name: "record"},
		}, map[string]struct{}{})
		tAssert.Contains(guarded, "profile")

		guarded = extractGuardedNames(ast.InfixExpression{
			Left: ast.InfixExpression{
				Left:     ast.StringLiteral{Lexeme: `"profile"`},
				Operator: lexer.TokenIn,
				Right:    ast.Identifier{Name: "record"},
			},
			Operator: lexer.TokenAndAnd,
			Right: ast.InfixExpression{
				Left:     ast.StringLiteral{Lexeme: `"age"`},
				Operator: lexer.TokenIn,
				Right:    ast.Identifier{Name: "record"},
			},
		}, map[string]struct{}{})
		tAssert.Contains(guarded, "profile")
		tAssert.Contains(guarded, "age")

		symbols := newSymbolTable()
		symbols.Add("TypeName", symbolKindType)
		optional := map[string]struct{}{"record": {}}
		err := validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "value"}, symbols, optional, map[string]struct{}{})
		tAssert.ErrorContains(err, "requires a presence check")

		err = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "value"}, symbols, optional, map[string]struct{}{"record": {}})
		tAssert.NoError(err)

		err = validateDataOutputExpression(ast.Identifier{Name: "TypeName"}, symbols, optional, map[string]struct{}{})
		tAssert.ErrorContains(err, "cannot reference type or schema declaration")
	})

	It("resolves parse-file schema names from imported files", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-parse-file-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		path := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{
  Profile: Profile;
  Alias: Alias;
  ignore: string;
}`)
		_ = path

		directives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}
		names, err := resolveParseFileExportedSchemaNames(directives, workspace, workspace)
		tAssert.NoError(err)
		tAssert.Equal([]string{"Alias", "Profile"}, names)

		directives = []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./missing.txt"`}}
		_, err = resolveParseFileExportedSchemaNames(directives, workspace, workspace)
		tAssert.Error(err)
	})
})

var _ = Describe("Validation helper coverage", func() {
	It("covers validation and evaluation branches", func() {
		workspace, err := os.MkdirTemp("", "processor-validation-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas := newSchemaRegistry()
		schemas.Add("User", schema)
		types := newTypeRegistry()
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		symbols.Add("name", symbolKindVariable)
		vars.Add("name", valueType{kind: ValueString})

		tAssert.NoError(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "opt", Value: ast.IntLiteral{Lexeme: "7"}}}}, valueType{kind: ValueRecord, schemaName: "User"}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "unknown", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "7"}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))

		tAssert.NoError(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "opt": {Kind: ValueInt, Int: 7}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "x"}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("Missing", map[string]Value{}, symbols, types, schemas, nil))

		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Bea"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueInt, Int: 7}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"extra": {Kind: ValueString, String: "x"}}}, valueType{kind: ValueRecord, record: &schema}, symbols, types, schemas, nil))
	})

	It("covers validation helper branches", func() {
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas.Add("User", schema)
		symbols.Add("name", symbolKindVariable)
		vars.Add("name", valueType{kind: ValueString})

		tAssert.NoError(validateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, "User", vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordLiteral(ast.RecordLiteral{}, "Missing", vars, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "7"}}}}, schema, "", vars, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueString}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueInt}}, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueInt}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordType(schema, symbols, types, schemas, nil))
	})

	It("covers declaration and output validation branches", func() {
		symbols := newSymbolTable()
		symbols.Add("User", symbolKindSchema)
		symbols.Add("Alias", symbolKindType)
		symbols.Add("value", symbolKindVariable)
		types := newTypeRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		schemas := newSchemaRegistry()
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}})
		variables := newVariableRegistry()
		variables.Add("value", valueType{kind: ValueString})

		tAssert.Error(validateDeclaration(ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{}))
		tAssert.NoError(validateDeclaration(ast.VariableDeclaration{Name: "name", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{}))
		tAssert.Error(validateDeclaration(ast.TypeDeclaration{Name: "Alias", Type: ast.PrimitiveType{Name: "string"}, Description: "doc"}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{"Alias": {}}, map[string]symbolKind{"Alias": symbolKindType}))
		tAssert.Error(validateDeclaration(ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}, Description: "doc"}}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{"User": {Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}}, map[string]symbolKind{"User": symbolKindSchema}))
		tAssert.NoError(validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"User": symbolKindSchema}))
		tAssert.Error(validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "value", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"value": symbolKindVariable}))
		tAssert.NoError(validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "Alias", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"sum"`}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"Alias": symbolKindType}))

		tAssert.NoError(validateTypeReference(ast.PrimitiveType{Name: "string"}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, symbols, types, schemas, nil))
		symbols.Add("ImportName", symbolKindImport)
		tAssert.NoError(validateTypeReference(ast.NamedType{Name: "User"}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.NamedType{Name: "ImportName"}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(nil, symbols, types, schemas, nil))

		_, err := resolveValueType(ast.NamedType{Name: "User"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil)
		tAssert.Error(err)

		workspace, err := os.MkdirTemp("", "processor-output-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()
		writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ User: User; }`)
		writeFixtureFile(workspace, "parse.mace", `[output = schema]
{ User: User; Other: Other; }`)
		writeFixtureFile(workspace, "not-schema.mace", `[output = data]
{ result: 1; }`)
		context := newProcessContext(workspace, workspace)
		name, ok, err := outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("User", name)
		name, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("User", name)
		_, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"parse.mace"`}}, context)
		tAssert.Error(err)
		tAssert.False(ok)
		names, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		tAssert.NoError(err)
		tAssert.Equal([]string{"User"}, names)
		_, err = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"not-schema.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)
		tAssert.Error(err)

		tAssert.NoError(validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"doc"`}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Schema"}, {Kind: ast.OutputDirectiveSchema, Value: "Schema"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Schema"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Schema"}, {Kind: ast.OutputDirectiveParseFile, Value: "schema.mace"}}}))

		tAssert.NoError(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "value"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateSchemaOutputFieldType(ast.NamedType{Name: "value"}, symbols))
		tAssert.NoError(validateSchemaOutputFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, symbols))

		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil))
		tAssert.Error(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}, variables, symbols, types, schemas, nil))
		tAssert.Error(validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil))
	})

	It("covers remaining validation and inference branches", func() {
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas.Add("User", schema)
		symbols.Add("User", symbolKindSchema)
		symbols.Add("Thing", symbolKindType)
		types.AddAlias("Alias", ast.NamedType{Name: "User"})
		vars.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		vars.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		vars.Add("flag", valueType{kind: ValueBoolean})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "opt", Value: ast.NullLiteral{}}}, vars, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"opt": {Kind: ValueNull}}, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("Missing", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "unknown": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, vars, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		_ = typesEqual(valueType{kind: ValueRecord}, valueType{kind: ValueRecord, schemaName: "User"})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString})
	})

	It("covers remaining validation and inference branches", func() {
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		symbols := newSymbolTable()
		variables := newVariableRegistry()
		symbols.Add("User", symbolKindSchema)
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		variables.Add("name", valueType{kind: ValueString})

		var seenDocs map[string]struct{}
		var docsByTarget map[string]ast.DocDeclaration
		var declaredKinds map[string]symbolKind

		tAssert.NoError(validateDeclaration(ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"x"`}}, symbols, types, schemas, nil, variables, seenDocs, docsByTarget, declaredKinds))
		tAssert.Error(validateDeclaration(ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}, HasValue: false}, symbols, types, schemas, nil, variables, seenDocs, docsByTarget, declaredKinds))
		tAssert.NoError(validateTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.Error(validateDocDeclaration(ast.DocDeclaration{Target: "value", Documentation: ast.Documentation{}}, symbols, schemas, variables, seenDocs, declaredKinds))
		_, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: `"User"`}}, ast.OutputDirectiveSchema, ".", ".")
		tAssert.Error(err)
		tAssert.NoError(validateDataOutputExpression(ast.Identifier{Name: "name"}, symbols, map[string]struct{}{}, map[string]struct{}{}))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.Identifier{Name: "name"}}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.Identifier{Name: "name"}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.InfixExpression{Left: ast.Identifier{Name: "name"}, Operator: lexer.TokenEqualEqual, Right: ast.Identifier{Name: "name"}}, valueType{kind: ValueBoolean}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, valueType{kind: ValueBoolean}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, "User", variables, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		fieldMap, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.NamedType{Name: "User"}}}}, types)
		tAssert.NoError(err)
		tAssert.NotNil(fieldMap)
		_, err = coerceEvaluatedValueAgainstType(ast.Identifier{Name: "name"}, Value{Kind: ValueString, String: "x"}, valueType{kind: ValueString}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "x"}, valueType{kind: ValueString}, symbols, types, schemas, nil))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString}))
		tAssert.True(typesEqual(valueType{kind: ValueString}, valueType{kind: ValueString}))
	})

	It("covers output and validation edge branches", func() {
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		symbols := newSymbolTable()
		variables := newVariableRegistry()
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		symbols.Add("User", symbolKindSchema)
		variables.Add("name", valueType{kind: ValueString})
		tAssert.NoError(validateDeclaration(ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, symbols, types, schemas, nil, variables, nil, map[string]ast.DocDeclaration{}, map[string]symbolKind{}))
		tAssert.Error(validateTypeReference(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil))
		tAssert.NoError(validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, symbols, map[string]struct{}{}, map[string]struct{}{}))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "name"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, variables, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.Identifier{Name: "name"}, valueType{kind: ValueInt}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, "User", variables, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("Missing", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("User", map[string]Value{"unknown": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		fields, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.NamedType{Name: "User"}}}}, types)
		tAssert.NoError(err)
		tAssert.NotNil(fields)
		coerced, err := coerceEvaluatedValueAgainstType(ast.Identifier{Name: "name"}, Value{Kind: ValueString, String: "x"}, valueType{kind: ValueString}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, coerced.Kind)
		environment := newValueEnvironment()
		environment.Add("name", Value{Kind: ValueString, String: "Ada"})
		_, err = evaluateExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateExpression(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Operator: lexer.TokenAndAnd, Right: ast.BooleanLiteral{Value: false}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateExpression(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Operator: lexer.TokenOrOr, Right: ast.BooleanLiteral{Value: false}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "name"}, ast.StringLiteral{Lexeme: `"b"`}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Left: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"a"`}}}}, Operator: lexer.TokenMerge, Right: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"b"`}}}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateExpression(ast.BooleanLiteral{Value: true}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueInt})
		tAssert.Error(err)
		_, _ = parseInt("123")
		_, _ = parseFloat("1.25")
		_, _ = parseHexInt("0xzz")
		_, _ = parseHexFloat("0x1.8")
		_, err = parseInterpolatedString(`"$("`, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = parseUnicodeEscape(`\u12`, 4)
		tAssert.Error(err)
	})
})

var _ = Describe("Registry helpers", func() {
	It("clones and queries symbol, type, schema, and variable registries", func() {
		symbols := newSymbolTable()
		symbols.Add("input", symbolKindImport)
		tAssert.True(symbols.IsImport("input"))
		tAssert.False(symbols.IsVariable("input"))

		types := newTypeRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		typeClone := types.Clone()
		tAssert.Equal(types.aliases["Alias"], typeClone.aliases["Alias"])

		schemas := newSchemaRegistry()
		schemas.Add("User", ast.RecordType{})
		schemaClone := schemas.Clone()
		tAssert.True(schemaClone != nil)
		record, ok := schemaClone.Get("User")
		tAssert.True(ok)
		tAssert.Equal(ast.RecordType{}, record)

		variables := newVariableRegistry()
		variables.Add("value", valueType{kind: ValueString})
		variableClone := variables.Clone()
		value, ok := variableClone.Get("value")
		tAssert.True(ok)
		tAssert.Equal(ValueString, value.kind)
	})
})

var _ = Describe("Runtime helper coverage", func() {
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

	It("covers remaining utility and branch helpers", func() {
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

	It("covers coverage gap branches from prior standalone tests", func() {
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
