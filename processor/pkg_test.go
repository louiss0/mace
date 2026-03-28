package processor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestProcessor(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Processor Suite")
}

func wrapScriptWithOutput(script string) string {
	return script + "\n[output = data] {}"
}

func wrapScriptWithOutputFields(script string, fields string) string {
	return script + "\n[output = data]\n{\n" + fields + "\n}"
}

type expectedValue struct {
	kind   ValueKind
	int64  int64
	float  float64
	bool   bool
	string string
	array  []expectedValue
	record map[string]expectedValue
}

func requireOutputValue(result Result, name string) Value {
	value, ok := result.Output[name]
	tAssert.True(ok)
	if !ok {
		return Value{}
	}
	return value
}

func assertExpectedValue(actual Value, expected expectedValue) {
	tAssert.Equal(expected.kind, actual.Kind)
	switch expected.kind {
	case ValueInt:
		tAssert.Equal(expected.int64, actual.Int)
	case ValueFloat:
		tAssert.InDelta(expected.float, actual.Float, 0.000001)
	case ValueBoolean:
		tAssert.Equal(expected.bool, actual.Boolean)
	case ValueString:
		tAssert.Equal(expected.string, actual.String)
	case ValueArray:
		tAssert.Equal(len(expected.array), len(actual.Array))
		for index, value := range expected.array {
			if index >= len(actual.Array) {
				return
			}
			assertExpectedValue(actual.Array[index], value)
		}
	case ValueRecord:
		tAssert.Equal(len(expected.record), len(actual.Record))
		for name, value := range expected.record {
			actualValue, ok := actual.Record[name]
			tAssert.True(ok)
			if !ok {
				continue
			}
			assertExpectedValue(actualValue, value)
		}
	}
}

func assertExpectedOutput(result Result, expected map[string]expectedValue) {
	for name, value := range expected {
		actual := requireOutputValue(result, name)
		assertExpectedValue(actual, value)
	}
}

var _ = Describe("Script block", func() {
	DescribeTable("processes valid script blocks",
		func(input string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.NoError(err)
		},
		Entry("type and schema declarations", wrapScriptWithOutput(`|===|
type Name = string;
schema User = { name: string; };
|===|`)),
		Entry("variables with literals", wrapScriptWithOutput(`|===|
string name = "Ada";
int age = 30;
float rate = 1.25;
boolean active = true;
|===|`)),
		Entry("imports and script block", `from "testdata/imports/base.mace" import Name;
|===|
Name user = "Ada";
|===|
[output = data]
{ user: user; }`),
	)

	DescribeTable("rejects invalid script blocks",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown type reference", wrapScriptWithOutput(`|===|
Unknown value = 1;
|===|`), "unknown type"),
		Entry("int type mismatch", wrapScriptWithOutput(`|===|
int total = 1.5;
|===|`), "type mismatch"),
		Entry("mixed numeric expression", wrapScriptWithOutput(`|===|
float total = 1 + 2.0;
|===|`), "type mismatch"),
		Entry("duplicate declaration name", wrapScriptWithOutput(`|===|
type User = string;
schema User = { name: string; };
|===|`), "duplicate declaration"),
		Entry("duplicate imports", `from "testdata/imports/base.mace" import User, User;
[output = data] {}`, "duplicate import"),
	)

	DescribeTable("accepts schema record literals",
		func(input string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.NoError(err)
		},
		Entry("optional fields omitted", wrapScriptWithOutput(`|===|
schema User = { name: string; age?: int; };
User user = { name: "Ada"; };
|===|`)),
		Entry("array of schema records", wrapScriptWithOutput(`|===|
schema Point = { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; y: 4; }
];
|===|`)),
	)

	DescribeTable("rejects schema record literal mismatches",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", wrapScriptWithOutput(`|===|
schema User = { name: string; age: int; };
User user = { name: "Ada"; };
|===|`), "missing required field"),
		Entry("unknown field", wrapScriptWithOutput(`|===|
schema User = { name: string; };
User user = { name: "Ada"; age: 30; };
|===|`), "unknown field"),
		Entry("optional field mismatch", wrapScriptWithOutput(`|===|
schema User = { name: string; age: int; };
User user = { name: "Ada"; age?: 30; };
|===|`), "not optional"),
		Entry("field type mismatch", wrapScriptWithOutput(`|===|
schema User = { name: string; age: int; };
User user = { name: 5; age: 30; };
|===|`), "type mismatch"),
		Entry("array element schema mismatch", wrapScriptWithOutput(`|===|
schema Point = { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; }
];
|===|`), "missing required field"),
	)
})

var _ = Describe("Imports", func() {
	DescribeTable("merges imported declarations",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("imports types and schemas", `from "testdata/imports/base.mace" import Name, User;
|===|
Name name = "Ada";
User result = { name: name; age: 30; };
|===|
[output = data]
{ result: result; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
	)

	DescribeTable("processes imported files",
		func(path string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessFile(path)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("resolves imports relative to file", "testdata/imports/consumer.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 27},
		}}),
	)

	DescribeTable("rejects circular imports",
		func(path string) {
			processor := New()
			_, err := processor.ProcessFile(path)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "circular import")
		},
		Entry("cycle detected", "testdata/imports/cycle_a.mace"),
	)

	DescribeTable("rejects invalid imports",
		func(file string, message string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown imported identifier", `from "testdata/imports/base.mace" import Missing;
[output = data] {}`, "imported identifier"),
		Entry("imported variable", `from "testdata/imports/base.mace" import local;
[output = data] {}`, "type or schema"),
		Entry("duplicate import across declarations", `from "testdata/imports/base.mace" import Name;
from "testdata/imports/other.mace" import Name;
[output = data] {}`, "duplicate import"),
		Entry("import file missing", `from "testdata/imports/missing.mace" import Name;
[output = data] {}`, "unable to read import file"),
		Entry("import collides with local declaration", `from "testdata/imports/base.mace" import Name;
|===|
type Name = string;
|===|
[output = data] {}`, "duplicate declaration"),
	)
})

var _ = Describe("Output block", func() {
	DescribeTable("rejects invalid directives",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing output directive", `|===|
schema User = { name: string; };
|===|
[schema = User] {}`, "missing output directive"),
		Entry("duplicate output directive", "[output = data, output = schema] {}", "duplicate output directive"),
		Entry("unknown schema in directive", "[output = data, schema = Missing] {}", "unknown schema"),
		Entry("schema output mode is unsupported", "[output = schema] {}", "reserved and not implemented"),
		Entry("schema_file directive is unsupported", `[output = data, schema_file = "./user.mace"] {}`, "reserved and not implemented"),
	)

	DescribeTable("accepts output that matches schema",
		func(input string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.NoError(err)
		},
		Entry("optional field omitted", `|===|
schema User = { name: string; age?: int; };
string name = "Ada";
|===|
[output = data, schema = User]
{ name: name; }`),
		Entry("nested record literal", `|===|
schema Profile = { age: int; };
schema User = { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { age: 30; }; }`),
	)

	DescribeTable("rejects output that violates schema",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", `|===|
schema User = { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; }`, "missing required field"),
		Entry("unknown output field", `|===|
schema User = { name: string; };
|===|
[output = data, schema = User]
{ name: "Ada"; extra: 1; }`, "unknown output field"),
		Entry("optional output mismatch", `|===|
schema User = { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; age?: 30; }`, "not optional"),
		Entry("nested record mismatch", `|===|
schema Profile = { age: int; };
schema User = { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { }; }`, "missing required field"),
		Entry("array element mismatch", `|===|
schema Point = { x: int; y: int; };
schema Plot = { points: array<Point>; };
|===|
[output = data, schema = Plot]
{ points: [ { x: 1; y: 2; }, { x: 3; } ]; }`, "missing required field"),
	)

	DescribeTable("returns math results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("addition and multiplication", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 * 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("subtraction and division", wrapScriptWithOutputFields(`|===|
int result = 20 - 4 / 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 18}),
		Entry("modulo", wrapScriptWithOutputFields(`|===|
int result = 9 % 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("exponentiation", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 8}),
		Entry("unary minus", wrapScriptWithOutputFields(`|===|
int result = -5;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: -5}),
		Entry("unary plus", wrapScriptWithOutputFields(`|===|
int result = +7;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("float arithmetic", wrapScriptWithOutputFields(`|===|
float result = 1.5 + 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("float division", wrapScriptWithOutputFields(`|===|
float result = 7.5 / 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 3.0}),
	)

	DescribeTable("returns operator precedence results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("unary before exponent", wrapScriptWithOutputFields(`|===|
int result = -2 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 4}),
		Entry("exponent is right associative", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 512}),
		Entry("shift after additive", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 << 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 12}),
		Entry("relational after shift", wrapScriptWithOutputFields(`|===|
boolean result = 1 << 2 > 3;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("equality after relational", wrapScriptWithOutputFields(`|===|
boolean result = 1 < 2 == true;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise and before or", wrapScriptWithOutputFields(`|===|
int result = 1 | 2 & 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("bitwise and before xor", wrapScriptWithOutputFields(`|===|
int result = 5 ^ 2 & 1;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
		Entry("logical and before or", wrapScriptWithOutputFields(`|===|
boolean result = true || false && false;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("conditional after logical or", wrapScriptWithOutputFields(`|===|
int result = false || true ? 5 : 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
	)

	DescribeTable("rejects invalid math operators",
		func(file string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
		},
		Entry("mixed numeric addition", wrapScriptWithOutputFields(`|===|
int total = 1 + 2.0;
|===|`, "total: total;")),
		Entry("mixed numeric exponentiation", wrapScriptWithOutputFields(`|===|
float total = 2 ** 3.0;
|===|`, "total: total;")),
		Entry("modulo with float", wrapScriptWithOutputFields(`|===|
int total = 5 % 2.5;
|===|`, "total: total;")),
	)

	DescribeTable("accepts non-math operators in script variables",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("bitwise operators", wrapScriptWithOutputFields(`|===|
int masked = 6 & 3;
int combined = 5 | 2;
int toggled = 5 ^ 3;
int inverted = ~1;
|===|`, "masked: masked;\ncombined: combined;\ntoggled: toggled;\ninverted: inverted;"), map[string]expectedValue{
			"masked":   {kind: ValueInt, int64: 2},
			"combined": {kind: ValueInt, int64: 7},
			"toggled":  {kind: ValueInt, int64: 6},
			"inverted": {kind: ValueInt, int64: ^int64(1)},
		}),
		Entry("shift operators", wrapScriptWithOutputFields(`|===|
int left = 1 << 3;
int right = 8 >> 1;
int logical = 8 >>> 1;
|===|`, "left: left;\nright: right;\nlogical: logical;"), map[string]expectedValue{
			"left":    {kind: ValueInt, int64: 8},
			"right":   {kind: ValueInt, int64: 4},
			"logical": {kind: ValueInt, int64: 4},
		}),
		Entry("comparisons", wrapScriptWithOutputFields(`|===|
boolean less = 3 < 5;
boolean greater = 5 > 3;
|===|`, "less: less;\ngreater: greater;"), map[string]expectedValue{
			"less":    {kind: ValueBoolean, bool: true},
			"greater": {kind: ValueBoolean, bool: true},
		}),
		Entry("equality operators", wrapScriptWithOutputFields(`|===|
boolean equal = 3 == 3;
boolean strict = 3 === 3;
boolean not_equal = 3 != 4;
boolean strict_not = 3 !== 4;
|===|`, "equal: equal;\nstrict: strict;\nnot_equal: not_equal;\nstrict_not: strict_not;"), map[string]expectedValue{
			"equal":      {kind: ValueBoolean, bool: true},
			"strict":     {kind: ValueBoolean, bool: true},
			"not_equal":  {kind: ValueBoolean, bool: true},
			"strict_not": {kind: ValueBoolean, bool: true},
		}),
		Entry("logical operators", wrapScriptWithOutputFields(`|===|
boolean result = true && false || true;
boolean not = !false;
|===|`, "result: result;\nnot: not;"), map[string]expectedValue{
			"result": {kind: ValueBoolean, bool: true},
			"not":    {kind: ValueBoolean, bool: true},
		}),
		Entry("ternary operator", wrapScriptWithOutputFields(`|===|
int value = true ? 1 : 2;
|===|`, "value: value;"), map[string]expectedValue{
			"value": {kind: ValueInt, int64: 1},
		}),
	)

	DescribeTable("rejects invalid operator usage",
		func(file string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
		},
		Entry("boolean with bitwise", wrapScriptWithOutputFields(`|===|
int value = true & false;
|===|`, "value: value;")),
		Entry("numeric with logical", wrapScriptWithOutputFields(`|===|
boolean value = 1 && 2;
|===|`, "value: value;")),
		Entry("string comparison", wrapScriptWithOutputFields(`|===|
boolean value = "a" < "b";
|===|`, "value: value;")),
		Entry("mixed equality", wrapScriptWithOutputFields(`|===|
boolean value = 1 == true;
|===|`, "value: value;")),
		Entry("ternary branch mismatch", wrapScriptWithOutputFields(`|===|
int value = true ? 1 : 2.0;
|===|`, "value: value;")),
	)

	DescribeTable("returns array and record results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
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
		Entry("record literal", wrapScriptWithOutputFields(`|===|
schema User = { name: string; age: int; };
int base = 20 + 10;
User result = { name: "Ada"; age: base; };
|===|`, "result: result;"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("nested record literal", wrapScriptWithOutputFields(`|===|
schema Inner = { value: int; };
schema Outer = { inner: Inner; };
int base = 8 + 2;
Outer result = { inner: { value: base; }; };
|===|`, "result: result;"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"inner": {kind: ValueRecord, record: map[string]expectedValue{
				"value": {kind: ValueInt, int64: 10},
			}},
		}}),
		Entry("array of records", wrapScriptWithOutputFields(`|===|
schema Point = { x: int; y: int; };
int base = 1 + 1;
array<Point> result = [
  { x: base; y: base + 1; },
  { x: base + 2; y: base + 3; }
];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueRecord, record: map[string]expectedValue{
				"x": {kind: ValueInt, int64: 2},
				"y": {kind: ValueInt, int64: 3},
			}},
			{kind: ValueRecord, record: map[string]expectedValue{
				"x": {kind: ValueInt, int64: 4},
				"y": {kind: ValueInt, int64: 5},
			}},
		}}),
		Entry("self reference", wrapScriptWithOutputFields(`|===|
int base = 3 * 4;
|===|`, "base: base;\nresult: $self.base + base;"), expectedValue{kind: ValueInt, int64: 24}),
	)

	DescribeTable("returns inline output expressions",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("inline int expression", `[output = data] { result: 2 + 3 * 4; }`, expectedValue{kind: ValueInt, int64: 14}),
		Entry("inline float expression", `[output = data] { result: 2.5 + 1.5; }`, expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("inline boolean expression", `[output = data] { result: 2 < 3 && true; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("inline string expression", `[output = data] { result: "hello"; }`, expectedValue{kind: ValueString, string: "hello"}),
		Entry("inline record expression", `[output = data] { result: { name: "Ada"; age: 30; }; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("inline array expression", `[output = data] { result: [1, 2, 3]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 1},
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
		}}),
		Entry("inline nested array expression", `[output = data] { result: [[1, 2], [3, 4]]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 1},
				{kind: ValueInt, int64: 2},
			}},
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 3},
				{kind: ValueInt, int64: 4},
			}},
		}}),
		Entry("inline optional output field", `[output = data] { result?: 1 + 1; }`, expectedValue{kind: ValueInt, int64: 2}),
	)

	DescribeTable("returns inline output blocks with multiple fields",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.Process(file)
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("multiple fields and self reference", `[output = data] { base: 2 + 2; result: $self.base * 3; }`, map[string]expectedValue{
			"base":   {kind: ValueInt, int64: 4},
			"result": {kind: ValueInt, int64: 12},
		}),
	)

	DescribeTable("rejects inline arrays with mixed types",
		func(file string) {
			processor := New()
			_, err := processor.Process(file)
			tAssert.Error(err)
		},
		Entry("mixed primitive types", `[output = data] { result: [1, "two"]; }`),
		Entry("mixed numeric types", `[output = data] { result: [1, 2.0]; }`),
		Entry("mixed nested array types", `[output = data] { result: [[1], ["two"]]; }`),
	)
})
