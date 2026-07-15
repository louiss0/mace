package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Fusions", func() {
	It("accepts fusion schema composition aliases", func() {
		processor := New()
		result, err := processor.Process(`|===|
schema Profile: { name: string, };
schema Audit: { created_at: string, };
alias User: fusion[Profile, Audit];
User value = {
  name: "Ada",
  created_at: "2026-04-08",
};
|===|
[output = 'data']
{
  result: value,
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "result")
		assertExpectedValue(actual, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name":       {kind: ValueString, string: "Ada"},
			"created_at": {kind: ValueString, string: "2026-04-08"},
		}})
	})

	It("merges choice aliases and deduplicates their values", func() {
		processor := New()
		result, err := processor.Process(`|===|
alias Access: choice["read", "write"];
alias Feature: choice["write", "execute"];
alias Permission: fusion[Access, Feature];
Permission value = "execute";
|===|
[output = 'data']
{
  value: value,
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "value")
		assertExpectedValue(actual, expectedValue{kind: ValueString, string: "execute"})
	})

	It("rejects fusion schema composition with non-schema members", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
alias Broken: fusion[string, int];
|===|`))
		tAssert.ErrorContains(err, "fusion members must be schemas")
	})

	It("rejects fusion schema composition with conflicting fields", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema Profile: { id: string, };
schema Audit: { id: int, };
alias Broken: fusion[Profile, Audit];
|===|`))
		tAssert.ErrorContains(err, "conflicting field")
	})
})
