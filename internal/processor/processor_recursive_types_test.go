package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Recursive record types", func() {
	It("accepts an array of the enclosing schema shape", func() {
		document := `|===|
schema Node: {
  value: string,
  children?: array<$self>,
};

Node root = {
  value: "root",
  children: [
    { value: "leaf", },
  ],
};
|===|
[output = 'data'] { root: root, }`

		result, err := New().ProcessInDir(document, "../..")
		tAssert.NoError(err)

		assertExpectedValue(requireOutputValue(result, "root"), expectedValue{
			kind: ValueRecord,
			record: map[string]expectedValue{
				"value": {kind: ValueString, string: "root"},
				"children": {kind: ValueArray, array: []expectedValue{
					{kind: ValueRecord, record: map[string]expectedValue{
						"value": {kind: ValueString, string: "leaf"},
					}},
				}},
			},
		})
	})

	It("accepts an array of the enclosing inline record shape", func() {
		document := `|===|
{ value: string, children?: array<$self>, } root = {
  value: "root",
  children: [{ value: "leaf", }],
};
|===|
[output = 'data'] { root: root, }`

		result, err := New().ProcessInDir(document, "../..")
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "root"), expectedValue{
			kind: ValueRecord,
			record: map[string]expectedValue{
				"value": {kind: ValueString, string: "root"},
				"children": {kind: ValueArray, array: []expectedValue{
					{kind: ValueRecord, record: map[string]expectedValue{
						"value": {kind: ValueString, string: "leaf"},
					}},
				}},
			},
		})
	})
})
