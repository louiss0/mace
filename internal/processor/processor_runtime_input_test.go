package processor

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Runtime input", func() {
	It("parses input records through the compatibility helper", func() {
		record, err := ParseInjectionRecord(`{ name: "Ada", enabled: true, }`)
		tAssert.NoError(err)
		assertExpectedValue(record["name"], expectedValue{kind: ValueString, string: "Ada"})
		assertExpectedValue(record["enabled"], expectedValue{kind: ValueBoolean, bool: true})
	})

	It("rejects trailing tokens after the record literal", func() {
		_, err := ParseInputRecord(`{ a: 1, } garbage`)
		tAssert.ErrorContains(err, "unexpected token after expression")
	})

	It("exposes parsed input fields with $ prefixes", func() {
		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
			"profile": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			}},
		})

		result, err := processor.Process(`|===|
schema Runtime: {
  env: string,
  profile: { name: string, },
};
|===|
[output = 'data', parse = Runtime]
{
  env: $env,
  profile_name: $profile.name,
}`)
		tAssert.NoError(err)
		assertExpectedOutput(result, map[string]expectedValue{
			"env":          {kind: ValueString, string: "prod"},
			"profile_name": {kind: ValueString, string: "Ada"},
		})
	})

	It("uses parse_file fields with $ prefixes", func() {
		workspace, err := os.MkdirTemp("", "mace-parse-file-fixture-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		writeExampleFile(workspace, "runtime.mace", `|===|
schema Runtime: {
  env: string,
  profile: { name: string, },
};
|===|
[output = 'schema']
{
  Runtime: Runtime,
}`)

		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
			"profile": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			}},
		})

		result, err := processor.ProcessInDir(`[output = 'data', parse_file = './runtime.mace']
{
  env: $env,
  profile_name: $profile.name,
}`, workspace)
		tAssert.NoError(err)
		assertExpectedOutput(result, map[string]expectedValue{
			"env":          {kind: ValueString, string: "prod"},
			"profile_name": {kind: ValueString, string: "Ada"},
		})
	})

	It("rejects undefined variables before reporting unavailable parsed input", func() {
		processor := New()
		_, err := processor.Process(`|===|
schema Runtime: { env: string, };
|===|
[output = 'data', parse = Runtime]
{
  result: missing,
}`)
		tAssert.ErrorContains(err, `unknown identifier "missing"`)
	})

	It("rejects parse directives without required input fields", func() {
		processor := New()
		_, err := processor.Process(`|===|
schema Runtime: { env: string, };
|===|
[output = 'data', parse = Runtime]
{
  env: $env,
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "missing required field")
	})
})
