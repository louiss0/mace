package processor

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
)

func expectedArrayValue(elements ...expectedValue) expectedValue {
	return expectedValue{kind: ValueArray, array: elements}
}

func expectedRecordValue(fields map[string]expectedValue) expectedValue {
	return expectedValue{kind: ValueRecord, record: fields}
}

var _ = Describe("Conditional type inference", func() {
	Describe("variables", func() {
		It("rejects a non-variant variable when its branches infer a variant", func() {
			_, err := New().Process(wrapScriptWithOutput(`|===|
string deployment_setting = true ? "local" : 10;
|===|`))

			tAssert.Error(err)
			tAssert.ErrorContains(err, "type mismatch: expected string, got variant[string, int]")
		})

		It("accepts a variant variable when both branches infer one member type", func() {
			result, err := New().Process(`|===|
variant[string, int] deployment_setting = true ? "primary" : "fallback";
|===|
[output = data]
{
  deployment_setting,
}`)

			tAssert.NoError(err)
			assertExpectedValue(
				requireOutputValue(result, "deployment_setting"),
				expectedValue{kind: ValueString, string: "primary"},
			)
		})

		It("infers every alternative returned by nested conditional children", func() {
			result, err := New().Process(`|===|
variant[string, int, boolean] deployment_setting = false
  ? true
  : true
    ? 10
    : "local";
|===|
[output = data]
{
  deployment_setting,
}`)

			tAssert.NoError(err)
			assertExpectedValue(
				requireOutputValue(result, "deployment_setting"),
				expectedValue{kind: ValueInt, int64: 10},
			)
		})

		DescribeTable("supports every primitive and nested collection variant",
			func(typeReference, expression string, expected expectedValue) {
				input := fmt.Sprintf(`|===|
%s inferred = %s;
|===|
[output = data]
{
  inferred,
}`, typeReference, expression)

				result, err := New().Process(input)
				tAssert.NoError(err)
				assertExpectedValue(requireOutputValue(result, "inferred"), expected)
			},
			Entry("string and int", `variant[string, int]`, `true ? "configured" : 1`,
				expectedValue{kind: ValueString, string: "configured"}),
			Entry("int and boolean", `variant[int, boolean]`, `true ? 1 : false`,
				expectedValue{kind: ValueInt, int64: 1}),
			Entry("float and boolean", `variant[float, boolean]`, `true ? 1.5 : false`,
				expectedValue{kind: ValueFloat, float: 1.5}),
			Entry("hex int and boolean", `variant[hex_int, boolean]`, `true ? 0xA : false`,
				expectedValue{kind: ValueHexInt, string: "0xA"}),
			Entry("hex float and boolean", `variant[hex_float, boolean]`, `true ? 0x1.8 : false`,
				expectedValue{kind: ValueHexFloat, string: "0x1.8"}),
			Entry("boolean and string", `variant[boolean, string]`, `true ? true : "disabled"`,
				expectedValue{kind: ValueBoolean, bool: true}),
			Entry("array depth one and string", `variant[array<int>, string]`,
				`false ? "disabled" : [1]`,
				expectedArrayValue(expectedValue{kind: ValueInt, int64: 1})),
			Entry("array depth two and boolean", `variant[array<array<float>>, boolean]`,
				`false ? true : [[1.5]]`,
				expectedArrayValue(expectedArrayValue(expectedValue{kind: ValueFloat, float: 1.5}))),
			Entry("array depth three and int", `variant[array<array<array<string>>>, int]`,
				`false ? 7 : [[["nested"]]]`,
				expectedArrayValue(expectedArrayValue(expectedArrayValue(
					expectedValue{kind: ValueString, string: "nested"},
				)))),
			Entry("record depth one and string", `variant[{ value: int, }, string]`,
				`false ? "disabled" : { value: 1, }`,
				expectedRecordValue(map[string]expectedValue{
					"value": {kind: ValueInt, int64: 1},
				})),
			Entry("record depth two and boolean",
				`variant[{ nested: { value: float, }, }, boolean]`,
				`false ? true : { nested: { value: 1.5, }, }`,
				expectedRecordValue(map[string]expectedValue{
					"nested": expectedRecordValue(map[string]expectedValue{
						"value": {kind: ValueFloat, float: 1.5},
					}),
				})),
			Entry("record depth three and int",
				`variant[{ nested: { deeper: { value: string, }, }, }, int]`,
				`false ? 7 : { nested: { deeper: { value: "nested", }, }, }`,
				expectedRecordValue(map[string]expectedValue{
					"nested": expectedRecordValue(map[string]expectedValue{
						"deeper": expectedRecordValue(map[string]expectedValue{
							"value": {kind: ValueString, string: "nested"},
						}),
					}),
				})),
		)

		DescribeTable("supports schema and record alternatives",
			func(recordExpression string, expected expectedValue) {
				input := fmt.Sprintf(`|===|
schema Deployment: { name: string, };
Deployment deployment = { name: "api", };
variant[Deployment, record<string>] selected = false ? deployment : %s;
|===|
[output = data]
{
  selected,
}`, recordExpression)

				result, err := New().Process(input)
				tAssert.NoError(err)
				assertExpectedValue(requireOutputValue(result, "selected"), expected)
			},
			Entry(
				"populated record",
				`{ region: "local", }`,
				expectedRecordValue(map[string]expectedValue{
					"region": {kind: ValueString, string: "local"},
				}),
			),
			Entry("empty record", `{}`, expectedRecordValue(map[string]expectedValue{})),
		)

		It("accepts every value returned by a nested choice conditional", func() {
			result, err := New().Process(`|===|
type Environment: choice["production", "testing", "local"];
Environment environment = false
  ? "production"
  : false
    ? "testing"
    : "local";
|===|
[output = data]
{
  environment,
}`)

			tAssert.NoError(err)
			assertExpectedValue(
				requireOutputValue(result, "environment"),
				expectedValue{kind: ValueString, string: "local"},
			)
		})
	})

	Describe("output blocks", func() {
		It("rejects a non-variant schema field when its branches infer a variant", func() {
			_, err := New().Process(`|===|
schema Result: { deployment_setting: string, };
|===|
[output = data, schema = Result]
{
  deployment_setting: true ? "local" : 10,
}`)

			tAssert.Error(err)
			tAssert.ErrorContains(err, "type mismatch: expected string, got variant[string, int]")
		})

		It("accepts a variant schema field when both branches infer one member type", func() {
			result, err := New().Process(`|===|
schema Result: { deployment_setting: variant[string, int], };
|===|
[output = data, schema = Result]
{
  deployment_setting: true ? "primary" : "fallback",
}`)

			tAssert.NoError(err)
			assertExpectedValue(
				requireOutputValue(result, "deployment_setting"),
				expectedValue{kind: ValueString, string: "primary"},
			)
		})

		DescribeTable("supports every primitive and up to three nested collection levels",
			func(typeReference, expression string, expected expectedValue) {
				input := fmt.Sprintf(`|===|
schema Result: { inferred: %s, };
|===|
[output = data, schema = Result]
{
  inferred: %s,
}`, typeReference, expression)

				result, err := New().Process(input)
				tAssert.NoError(err)
				assertExpectedValue(requireOutputValue(result, "inferred"), expected)
			},
			Entry("string and int", `variant[string, int]`, `true ? "configured" : 1`,
				expectedValue{kind: ValueString, string: "configured"}),
			Entry("int and boolean", `variant[int, boolean]`, `true ? 1 : false`,
				expectedValue{kind: ValueInt, int64: 1}),
			Entry("float and boolean", `variant[float, boolean]`, `true ? 1.5 : false`,
				expectedValue{kind: ValueFloat, float: 1.5}),
			Entry("hex int and boolean", `variant[hex_int, boolean]`, `true ? 0xA : false`,
				expectedValue{kind: ValueHexInt, string: "0xA"}),
			Entry("hex float and boolean", `variant[hex_float, boolean]`, `true ? 0x1.8 : false`,
				expectedValue{kind: ValueHexFloat, string: "0x1.8"}),
			Entry("boolean and string", `variant[boolean, string]`, `true ? true : "disabled"`,
				expectedValue{kind: ValueBoolean, bool: true}),
			Entry("array depth one and string", `variant[array<int>, string]`,
				`false ? "disabled" : [1]`,
				expectedArrayValue(expectedValue{kind: ValueInt, int64: 1})),
			Entry("array depth two and boolean", `variant[array<array<float>>, boolean]`,
				`false ? true : [[1.5]]`,
				expectedArrayValue(expectedArrayValue(expectedValue{kind: ValueFloat, float: 1.5}))),
			Entry("array depth three and int", `variant[array<array<array<string>>>, int]`,
				`false ? 7 : [[["nested"]]]`,
				expectedArrayValue(expectedArrayValue(expectedArrayValue(
					expectedValue{kind: ValueString, string: "nested"},
				)))),
			Entry("record depth one and string", `variant[{ value: int, }, string]`,
				`false ? "disabled" : { value: 1, }`,
				expectedRecordValue(map[string]expectedValue{
					"value": {kind: ValueInt, int64: 1},
				})),
			Entry("record depth two and boolean",
				`variant[{ nested: { value: float, }, }, boolean]`,
				`false ? true : { nested: { value: 1.5, }, }`,
				expectedRecordValue(map[string]expectedValue{
					"nested": expectedRecordValue(map[string]expectedValue{
						"value": {kind: ValueFloat, float: 1.5},
					}),
				})),
			Entry("record depth three and int",
				`variant[{ nested: { deeper: { value: string, }, }, }, int]`,
				`false ? 7 : { nested: { deeper: { value: "nested", }, }, }`,
				expectedRecordValue(map[string]expectedValue{
					"nested": expectedRecordValue(map[string]expectedValue{
						"deeper": expectedRecordValue(map[string]expectedValue{
							"value": {kind: ValueString, string: "nested"},
						}),
					}),
				})),
		)

		DescribeTable("infers variants through one to ten nested conditional levels",
			func(typeReference, expression string, expected expectedValue) {
				input := fmt.Sprintf(`|===|
schema Result: { value: %s, };
|===|
[output = data, schema = Result]
{
  value: %s,
}`, typeReference, expression)

				result, err := New().Process(input)
				tAssert.NoError(err)
				assertExpectedValue(requireOutputValue(result, "value"), expected)
			},
			Entry("one level", `variant[string, int]`, `false ? "one" : 1`,
				expectedValue{kind: ValueInt, int64: 1}),
			Entry("two levels", `variant[string, int, boolean]`,
				`false ? "one" : false ? 1 : true`,
				expectedValue{kind: ValueBoolean, bool: true}),
			Entry("three levels", `variant[string, int, boolean, float]`,
				`false ? "one" : false ? 1 : false ? true : 1.5`,
				expectedValue{kind: ValueFloat, float: 1.5}),
			Entry("four levels", `variant[string, int, boolean, float, hex_int]`,
				`false ? "one" : false ? 1 : false ? true : false ? 1.5 : 0xA`,
				expectedValue{kind: ValueHexInt, string: "0xA"}),
			Entry("five levels", `variant[string, int, boolean, float, hex_int, hex_float]`,
				`false ? "one" : false ? 1 : false ? true
					: false ? 1.5 : false ? 0xA : 0x1.8`,
				expectedValue{kind: ValueHexFloat, string: "0x1.8"}),
			Entry("six levels", `variant[string, int, boolean, float, hex_int, hex_float]`,
				`false ? "one" : false ? 1 : false ? true : false ? 1.5
					: false ? 0xA : false ? 0x1.8 : "seven"`,
				expectedValue{kind: ValueString, string: "seven"}),
			Entry("seven levels", `variant[string, int, boolean, float, hex_int, hex_float]`,
				`false ? "one" : false ? 1 : false ? true : false ? 1.5
					: false ? 0xA : false ? 0x1.8 : false ? "seven" : 8`,
				expectedValue{kind: ValueInt, int64: 8}),
			Entry("eight levels", `variant[string, int, boolean, float, hex_int, hex_float]`,
				`false ? "one" : false ? 1 : false ? true : false ? 1.5
					: false ? 0xA : false ? 0x1.8 : false ? "seven" : false ? 8 : false`,
				expectedValue{kind: ValueBoolean, bool: false}),
			Entry("nine levels", `variant[string, int, boolean, float, hex_int, hex_float]`,
				`false ? "one" : false ? 1 : false ? true : false ? 1.5
					: false ? 0xA : false ? 0x1.8 : false ? "seven"
					: false ? 8 : false ? false : 10.5`,
				expectedValue{kind: ValueFloat, float: 10.5}),
			Entry("ten levels", `variant[string, int, boolean, float, hex_int, hex_float]`,
				`false ? "one" : false ? 1 : false ? true : false ? 1.5
					: false ? 0xA : false ? 0x1.8 : false ? "seven"
					: false ? 8 : false ? false : false ? 10.5 : 0xB`,
				expectedValue{kind: ValueHexInt, string: "0xB"}),
		)

		DescribeTable("supports schema and record alternatives",
			func(recordExpression string, expected expectedValue) {
				input := fmt.Sprintf(`|===|
schema Deployment: { name: string, };
schema Result: { selected: variant[Deployment, record<string>], };
Deployment deployment = { name: "api", };
|===|
[output = data, schema = Result]
{
  selected: false ? deployment : %s,
}`, recordExpression)

				result, err := New().Process(input)
				tAssert.NoError(err)
				assertExpectedValue(requireOutputValue(result, "selected"), expected)
			},
			Entry(
				"populated record",
				`{ region: "local", }`,
				expectedRecordValue(map[string]expectedValue{
					"region": {kind: ValueString, string: "local"},
				}),
			),
			Entry("empty record", `{}`, expectedRecordValue(map[string]expectedValue{})),
		)

		It("accepts every value returned by a nested choice conditional", func() {
			result, err := New().Process(`|===|
type Environment: choice["production", "testing", "local"];
schema Result: { environment: Environment, };
|===|
[output = data, schema = Result]
{
  environment: false
    ? "production"
    : false
      ? "testing"
      : "local",
}`)

			tAssert.NoError(err)
			assertExpectedValue(
				requireOutputValue(result, "environment"),
				expectedValue{kind: ValueString, string: "local"},
			)
		})
	})
})
