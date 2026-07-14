package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Declarations", func() {
	It("omits output fields that evaluate to null through nullable variables", func() {
		processor := New()

		result, err := processor.Process(`|===|
nullable string env = null;
|===|
[output = 'data']
{
  env: env,
}`)
		tAssert.NoError(err)
		tAssert.Empty(result.Output)
	})

	It("accepts null for optional schema fields", func() {
		processor := New()

		result, err := processor.Process(`|===|
schema User: { nickname?: string, };
User user = { nickname: null, };
|===|
[output = 'data']
{
  user: user,
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "user")
		assertExpectedValue(actual, expectedValue{kind: ValueRecord, record: map[string]expectedValue{}})
	})

	It("rejects direct null output fields", func() {
		processor := New()

		_, err := processor.Process(`[output = 'data']
{
  env: null,
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects null assignments to non-nullable variables", func() {
		processor := New()

		_, err := processor.Process(wrapScriptWithOutput(`|===|
string env = null;
|===|`))
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects nullable conditional assignments to non-nullable variables", func() {
		processor := New()

		_, err := processor.Process(wrapScriptWithOutput(`|===|
string env = false ? null : "prod";
|===|`))
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects nullable conditional assignments to required schema fields", func() {
		processor := New()

		_, err := processor.Process(`|===|
schema Runtime: { env: string, };
Runtime config = { env: false ? null : "prod", };
|===|
[output = 'data']
{
  config: config,
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects nullable shorthand assignments to required schema fields", func() {
		processor := New()

		_, err := processor.Process(`|===|
nullable string env = false ? null : "prod";
schema Runtime: { env: string, };
Runtime config = { env, };
|===|
[output = 'data']
{
  config: config,
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("accepts empty conditional branches with an explicit variant type", func() {
		result, err := New().Process(`|===|
variant[string, array<string>] value = false ? "configured" : [];
|===|
[output = 'data']
{
  value: value,
}`)
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "value"), expectedValue{kind: ValueArray, array: []expectedValue{}})
	})

	It("requires a variant type for ambiguous conditional variables", func() {
		_, err := New().Process(wrapScriptWithOutput(`|===|
array<string> value = false ? "configured" : [];
|===|`))
		tAssert.Error(err)
		tAssert.ErrorContains(err, "requires a variant variable type")
	})
})
