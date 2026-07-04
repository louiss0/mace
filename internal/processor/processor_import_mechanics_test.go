package processor

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

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
