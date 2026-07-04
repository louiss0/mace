package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("$self", func() {
	DescribeTable("returns inline output blocks with multiple fields",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("multiple fields and self reference", `[output = data] { base: 2 + 2; result: $self.base * 3; }`, map[string]expectedValue{
			"base":   {kind: ValueInt, int64: 4},
			"result": {kind: ValueInt, int64: 12},
		}),
	)

	DescribeTable("returns self reference results by depth",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("level 1", `[output = data] { base: 4; result: $self.base; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("level 2", `[output = data] { profile: { name: "Ada"; }; result: $self.profile.name; }`, expectedValue{kind: ValueString, string: "Ada"}),
		Entry("level 3", `[output = data] { profile: { details: { age: 30; }; }; result: $self.profile.details.age; }`, expectedValue{kind: ValueInt, int64: 30}),
		Entry("level 4", `[output = data] { tree: { branch: { leaf: { value: 9; }; }; }; result: $self.tree.branch.leaf.value; }`, expectedValue{kind: ValueInt, int64: 9}),
		Entry("level 5", `[output = data] { a: { b: { c: { d: { e: true; }; }; }; }; result: $self.a.b.c.d.e; }`, expectedValue{kind: ValueBoolean, bool: true}),
	)

	DescribeTable("returns nested array access results by depth",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("level 1", `[output = data] { result: [10][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 2", `[output = data] { result: [[10]][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 3", `[output = data] { result: [[[10]]][0][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 4", `[output = data] { result: [[[[10]]]][0][0][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 5", `[output = data] { result: [[[[[10]]]]][0][0][0][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
	)

	DescribeTable("returns mixed self reference results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("arithmetic with chained fields", `[output = data] { base: 4; doubled: $self.base * 2; result: $self.doubled + $self.base; }`, expectedValue{kind: ValueInt, int64: 12}),
		Entry("conditional with self", `[output = data] { enabled: true; result: $self.enabled ? "on" : "off"; }`, expectedValue{kind: ValueString, string: "on"}),
		Entry("array literal with self", `[output = data] { base: 2; result: [$self.base, $self.base + 1, $self.base + 2]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
			{kind: ValueInt, int64: 4},
		}}),
		Entry("record literal with self", `[output = data] { name: "Ada"; result: { display: $self.name; active: true; }; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"display": {kind: ValueString, string: "Ada"},
			"active":  {kind: ValueBoolean, bool: true},
		}}),
		Entry("nested self and operators", `[output = data] { profile: { score: 3; }; result: $self.profile.score * 4 + 1; }`, expectedValue{kind: ValueInt, int64: 13}),
	)

	DescribeTable("rejects invalid self references",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("future field reference", `[output = data] { result: $self.base; base: 4; }`, "unknown self reference"),
		Entry("nested path through non record", `[output = data] { base: 4; result: $self.base.value; }`, "non-record"),
	)
})
