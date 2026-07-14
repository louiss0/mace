package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Arrays", func() {
	DescribeTable("returns array results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("array literal", wrapScriptWithOutputFields(`|===|
int base = 2 + 3;
array<int> result = [base, base + 1, base + 2];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 5},
			{kind: ValueInt, int64: 6},
			{kind: ValueInt, int64: 7},
		}}),
		Entry("string arrays support all string literal forms", wrapScriptWithOutputFields(`|===|
array<string> result = ['Kyle', "Tyrone", """Luke"""];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueString, string: "Kyle"},
			{kind: ValueString, string: "Tyrone"},
			{kind: ValueString, string: "Luke"},
		}}),
		Entry("variant arrays", wrapScriptWithOutputFields(`|===|
array<variant[string, int]> result = ["Ada", 1];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueString, string: "Ada"},
			{kind: ValueInt, int64: 1},
		}}),
		Entry("negative int arrays", wrapScriptWithOutputFields(`|===|
array<int> result = [-1, -2, -3];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: -1},
			{kind: ValueInt, int64: -2},
			{kind: ValueInt, int64: -3},
		}}),
		Entry("nested arrays", wrapScriptWithOutputFields(`|===|
int base = 1 + 2;
array<array<int> > result = [[base, base + 1], [base + 2, base + 3]];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 3},
				{kind: ValueInt, int64: 4},
			}},
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 5},
				{kind: ValueInt, int64: 6},
			}},
		}}),
		Entry("self reference", wrapScriptWithOutputFields(`|===|
int base = 3 * 4;
|===|`, "base: base;\nresult: $self.base + base;"), expectedValue{kind: ValueInt, int64: 24}),
	)

	DescribeTable("returns inline output expressions",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("inline int expression", `[output = "data"] { result: 2 + 3 * 4, }`, expectedValue{kind: ValueInt, int64: 14}),
		Entry("inline float expression", `[output = "data"] { result: 2.5 + 1.5, }`, expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("inline boolean expression", `[output = "data"] { result: 2 < 3 && true, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("inline string expression", `[output = "data"] { result: "hello", }`, expectedValue{kind: ValueString, string: "hello"}),
		Entry("inline array expression", `[output = "data"] { result: [1, 2, 3], }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 1},
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
		}}),
		Entry("inline nested array expression", `[output = "data"] { result: [[1, 2], [3, 4]], }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 1},
				{kind: ValueInt, int64: 2},
			}},
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 3},
				{kind: ValueInt, int64: 4},
			}},
		}}),
		Entry("inline negative float array expression", `[output = "data"] { result: [-1.5, -2.5], }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueFloat, float: -1.5},
			{kind: ValueFloat, float: -2.5},
		}}),
		Entry("inline optional output field", `[output = "data"] { result?: 1 + 1, }`, expectedValue{kind: ValueInt, int64: 2}),
	)

})
