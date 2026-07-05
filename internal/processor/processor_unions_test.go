package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Unions", func() {
	It("accepts union schema composition aliases", func() {
		processor := New()
		result, err := processor.Process(`|===|
schema Profile: { name: string; };
schema Audit: { created_at: string; };
type User: union[Profile, Audit];
User value = {
  name: "Ada";
  created_at: "2026-04-08";
};
|===|
[output = data]
{
  result: value;
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "result")
		assertExpectedValue(actual, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name":       {kind: ValueString, string: "Ada"},
			"created_at": {kind: ValueString, string: "2026-04-08"},
		}})
	})

	It("rejects union schema composition with non-schema members", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Broken: union[string, int];
|===|`))
		tAssert.ErrorContains(err, "union members must be schemas")
	})

	It("rejects union schema composition with conflicting fields", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema Profile: { id: string; };
schema Audit: { id: int; };
type Broken: union[Profile, Audit];
|===|`))
		tAssert.ErrorContains(err, "conflicting field")
	})
})
