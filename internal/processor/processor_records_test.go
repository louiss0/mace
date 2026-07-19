package processor

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Record maps", func() {
	It("processes kebab-case declarations, references, and record keys", func() {
		result, err := New().Process(`|===|
schema user-profile: {
  display-name: string,
};
user-profile current-user = {
  display-name: "Ada",
};
string greeting-message = current-user.display-name;
|===|
[output = 'data']
{
  greeting-message,
  user-profile: {
    display-name: current-user.display-name,
    nested-record: {
      record-key: greeting-message,
    },
  },
}`)

		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "greeting-message"), expectedValue{kind: ValueString, string: "Ada"})
		assertExpectedValue(requireOutputValue(result, "user-profile"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"display-name": {kind: ValueString, string: "Ada"},
			"nested-record": {kind: ValueRecord, record: map[string]expectedValue{
				"record-key": {kind: ValueString, string: "Ada"},
			}},
		}})
	})

	It("preserves multiple record entries and resolves their member values", func() {
		result, err := New().Process(`|===|
alias Dependencies: record<string>;
Dependencies dependencies = {
  pi_prompt_guard: "^1.0.0",
  pi_prompt_form: "^1.0.0",
  pi_prompt: "^1.0.0",
};
|===|
[output = 'data']
{
  dependencies: dependencies,
  form: dependencies.pi_prompt_form,
}`)

		tAssert.NoError(err)
		assertExpectedValue(result.Output["dependencies"], expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"pi_prompt_guard": {kind: ValueString, string: "^1.0.0"},
			"pi_prompt_form":  {kind: ValueString, string: "^1.0.0"},
			"pi_prompt":       {kind: ValueString, string: "^1.0.0"},
		}})
		assertExpectedValue(result.Output["form"], expectedValue{kind: ValueString, string: "^1.0.0"})
	})

	It("rejects record entries that do not match their value type", func() {
		_, err := New().Process(`|===|
alias Dependencies: record<string>;
Dependencies dependencies = { pi_prompt_guard: 1, };
|===|
[output = 'data']
{ dependencies: dependencies, }`)

		tAssert.ErrorContains(err, "type mismatch")
	})

	It("evaluates inline record literals", func() {
		result, err := New().Process(`[output = 'data'] { result: { name: "Ada", age: 30, }, }`)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["result"], expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}})
	})

	DescribeTable("accepts supported record value types",
		func(declarations string, valueType string, literal string) {
			_, err := New().Process(fmt.Sprintf(`|===|
%s
alias Packages: record<%s>;
Packages packages = %s;
|===|
[output = 'data']
{ packages: packages, }`, declarations, valueType, literal))

			tAssert.NoError(err)
		},
		Entry("string", "", "string", `{ codefixer: "enabled", }`),
		Entry("int", "", "int", `{ retries: 3, }`),
		Entry("float", "", "float", `{ ratio: 1.5, }`),
		Entry("hex int", "", "hex_int", `{ mask: 0x1f, }`),
		Entry("hex float", "", "hex_float", `{ ratio: 0x1.8, }`),
		Entry("boolean", "", "boolean", `{ enabled: true, }`),
		Entry("array", "", "array<string>", `{ tags: ["api"], }`),
		Entry("nested record", "", "record<string>", `{ service: { name: "api", }, }`),
		Entry("variant", "", "variant[string, int]", `{ name: "api", retries: 3, }`),
		Entry("choice", "", `choice["enabled", "disabled"]`, `{ mode: "enabled", }`),
		Entry("inline record", "", `{ name: string, }`, `{ service: { name: "api", }, }`),
		Entry("schema", "schema Service: { name: string, };", "Service", `{ service: { name: "api", }, }`),
		Entry("fusion", `
schema Service: { name: string, };
schema Versioned: { version: int, };
alias Deployment: fusion[Service, Versioned];`, "Deployment", `{ api: { name: "api", version: 1, }, }`),
	)

	DescribeTable("accepts every primitive variant combination as record values",
		func(valueType string, literal string) {
			_, err := New().Process(fmt.Sprintf(`|===|
alias Values: record<variant[%s]>;
Values values = %s;
|===|
[output = 'data']
{ values: values, }`, valueType, literal))

			tAssert.NoError(err)
		},
		Entry("string and int", "string, int", `{ name: "api", retries: 3, }`),
		Entry("string and float", "string, float", `{ name: "api", ratio: 1.5, }`),
		Entry("string and hex int", "string, hex_int", `{ name: "api", mask: 0x1f, }`),
		Entry("string and hex float", "string, hex_float", `{ name: "api", ratio: 0x1.8, }`),
		Entry("string and boolean", "string, boolean", `{ name: "api", enabled: true, }`),
		Entry("int and float", "int, float", `{ retries: 3, ratio: 1.5, }`),
		Entry("int and hex int", "int, hex_int", `{ retries: 3, mask: 0x1f, }`),
		Entry("int and hex float", "int, hex_float", `{ retries: 3, ratio: 0x1.8, }`),
		Entry("int and boolean", "int, boolean", `{ retries: 3, enabled: true, }`),
		Entry("float and hex int", "float, hex_int", `{ ratio: 1.5, mask: 0x1f, }`),
		Entry("float and hex float", "float, hex_float", `{ ratio: 1.5, hex_ratio: 0x1.8, }`),
		Entry("float and boolean", "float, boolean", `{ ratio: 1.5, enabled: true, }`),
		Entry("hex int and hex float", "hex_int, hex_float", `{ mask: 0x1f, ratio: 0x1.8, }`),
		Entry("hex int and boolean", "hex_int, boolean", `{ mask: 0x1f, enabled: true, }`),
		Entry("hex float and boolean", "hex_float, boolean", `{ ratio: 0x1.8, enabled: true, }`),
	)

	DescribeTable("accepts nested record maps",
		func(depth int) {
			valueType := recordMapTypeText(depth, "string")
			literal := nestedRecordMapLiteralText(depth, `"enabled"`)
			_, err := New().Process(fmt.Sprintf(`|===|
alias Packages: %s;
Packages packages = %s;
|===|
[output = 'data']
{ packages: packages, }`, valueType, literal))

			tAssert.NoError(err)
		},
		Entry("one level", 1),
		Entry("two levels", 2),
		Entry("three levels", 3),
		Entry("four levels", 4),
		Entry("five levels", 5),
		Entry("six levels", 6),
		Entry("seven levels", 7),
		Entry("eight levels", 8),
		Entry("nine levels", 9),
		Entry("ten levels", 10),
	)

	DescribeTable("accepts alternating record and array nesting",
		func(depth int, startsWithRecord bool) {
			valueType := alternatingRecordArrayTypeText(depth, startsWithRecord)
			literal := alternatingRecordArrayLiteralText(depth, startsWithRecord, `"enabled"`)
			_, err := New().Process(fmt.Sprintf(`|===|
alias Packages: %s;
Packages packages = %s;
|===|
[output = 'data']
{ packages: packages, }`, valueType, literal))

			tAssert.NoError(err)
		},
		Entry("record then array at one level", 1, true),
		Entry("record then array at two levels", 2, true),
		Entry("record then array at three levels", 3, true),
		Entry("record then array at four levels", 4, true),
		Entry("record then array at five levels", 5, true),
		Entry("record then array at six levels", 6, true),
		Entry("record then array at seven levels", 7, true),
		Entry("record then array at eight levels", 8, true),
		Entry("record then array at nine levels", 9, true),
		Entry("record then array at ten levels", 10, true),
		Entry("array then record at one level", 1, false),
		Entry("array then record at two levels", 2, false),
		Entry("array then record at three levels", 3, false),
		Entry("array then record at four levels", 4, false),
		Entry("array then record at five levels", 5, false),
		Entry("array then record at six levels", 6, false),
		Entry("array then record at seven levels", 7, false),
		Entry("array then record at eight levels", 8, false),
		Entry("array then record at nine levels", 9, false),
		Entry("array then record at ten levels", 10, false),
	)

	DescribeTable("selects a variant record member at each nesting depth",
		func(depth int) {
			valueType := recordMapTypeText(depth, "string")
			literal := nestedRecordMapLiteralText(depth, `"enabled"`)
			_, err := New().Process(fmt.Sprintf(`|===|
alias Packages: variant[%s, %s];
Packages packages = %s;
|===|
[output = 'data']
{ packages: packages, }`, valueType, recordMapTypeText(depth+1, "string"), literal))

			tAssert.NoError(err)
		},
		Entry("one level", 1),
		Entry("two levels", 2),
		Entry("three levels", 3),
		Entry("four levels", 4),
		Entry("five levels", 5),
		Entry("six levels", 6),
		Entry("seven levels", 7),
		Entry("eight levels", 8),
		Entry("nine levels", 9),
		Entry("ten levels", 10),
	)
})

func recordMapTypeText(depth int, valueType string) string {
	for range depth {
		valueType = "record<" + valueType + ">"
	}

	return valueType
}

func nestedRecordMapLiteralText(depth int, value string) string {
	for range depth {
		value = "{ value: " + value + ", }"
	}

	return value
}

func alternatingRecordArrayTypeText(depth int, startsWithRecord bool) string {
	valueType := "string"
	for offset := range depth {
		level := depth - offset - 1
		if (level%2 == 0) == startsWithRecord {
			valueType = "record<" + valueType + ">"
			continue
		}
		valueType = "array<" + valueType + ">"
	}

	return valueType
}

func alternatingRecordArrayLiteralText(depth int, startsWithRecord bool, value string) string {
	for offset := range depth {
		level := depth - offset - 1
		if (level%2 == 0) == startsWithRecord {
			value = "{ value: " + value + ", }"
			continue
		}
		value = "[" + value + "]"
	}

	return value
}
