package processor

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
)

func buildVariantMatchEntries() []TableEntry {
	entries := make([]TableEntry, 0, 11)
	for arity := 2; arity <= 12; arity++ {
		arity := arity
		entries = append(entries, Entry(fmt.Sprintf("%d members", arity), arity))
	}
	return entries
}

func buildDepthMatchEntries() []TableEntry {
	entries := make([]TableEntry, 0, 5)
	for depth := 1; depth <= 5; depth++ {
		depth := depth
		entries = append(entries, Entry(fmt.Sprintf("depth %d", depth), depth))
	}
	return entries
}

func variantMatchSource(arity int, armCount int) string {
	declarations := make([]string, 0, arity)
	members := make([]string, 0, arity)
	arms := make([]string, 0, armCount)
	for index := 1; index <= arity; index++ {
		name := fmt.Sprintf("Member%d", index)
		declarations = append(declarations, fmt.Sprintf("schema %s: { value%d: int, };", name, index))
		members = append(members, name)
		if index <= armCount {
			arms = append(arms, fmt.Sprintf("%s => %d,", name, index))
		}
	}
	return fmt.Sprintf(`|===|
%s
variant[%s] value = { value%d: %d };
int selected = match (value) { %s };
|===|
{ selected, }`, strings.Join(declarations, "\n"), strings.Join(members, ", "), arity, arity, strings.Join(arms, " "))
}

func nestedArrayType(depth int) string {
	value := "string"
	for range depth {
		value = "array<" + value + ">"
	}
	return value
}

func nestedArrayValue(depth int) string {
	value := `"matched"`
	for range depth {
		value = "[" + value + "]"
	}
	return value
}

func nestedRecordDeclarations(depth int) string {
	declarations := []string{"schema Record1: { value: string, };"}
	for level := 2; level <= depth; level++ {
		declarations = append(declarations, fmt.Sprintf("schema Record%d: { nested: Record%d, };", level, level-1))
	}
	return strings.Join(declarations, "\n")
}

func nestedRecordValue(depth int) string {
	value := `{ value: "matched" }`
	for level := 2; level <= depth; level++ {
		value = "{ nested: " + value + " }"
	}
	return value
}

var _ = Describe("Match expressions", func() {
	It("selects a variant arm by runtime member type", func() {
		result, err := New().Process(`|===|
variant[string, int] value = 7;
string selected = match (value) {
  string => "text",
  int => "number",
};
|===|
{ selected, }`)

		tAssert.NoError(err)
		tAssert.Equal("number", result.Output["selected"].String)
	})

	It("selects a choice arm by literal value", func() {
		result, err := New().Process(`|===|
type Status: choice["ready", "pending"];
Status status = "ready";
int selected = match (status) {
  "ready" => 1,
  "pending" => 2,
};
|===|
{ selected, }`)

		tAssert.NoError(err)
		tAssert.Equal(int64(1), result.Output["selected"].Int)
	})

	It("infers a variant from unlike arm results", func() {
		result, err := New().Process(`|===|
variant[string, int] value = "Mace";
variant[string, boolean] selected = match (value) {
  string => "text",
  int => false,
};
|===|
{ selected, }`)

		tAssert.NoError(err)
		tAssert.Equal("text", result.Output["selected"].String)
	})

	DescribeTable("matches exhaustive variants from 2 through 12 members",
		func(arity int) {
			result, err := New().Process(variantMatchSource(arity, arity))
			tAssert.NoError(err)
			tAssert.Equal(int64(arity), result.Output["selected"].Int)
		},
		buildVariantMatchEntries(),
	)

	DescribeTable("uses match expressions in conditional operators",
		func(source string, expected string) {
			result, err := New().Process(source)
			tAssert.NoError(err)
			tAssert.Equal(expected, result.Output["selected"].String)
		},
		Entry("as the condition", `|===|
variant[string, int] value = 7;
string selected = match (value) {
  string => false,
  int => true,
} ? "matched" : "fallback";
|===|
{ selected, }`, "matched"),
		Entry("with match at the question mark", `|===|
variant[string, int] value = 7;
string selected = true ? match (value) {
  string => "string",
  int => "integer",
} : "fallback";
|===|
{ selected, }`, "integer"),
		Entry("with match at the colon", `|===|
variant[string, int] value = 7;
string selected = false ? "fallback" : match (value) {
  string => "string",
  int => "integer",
};
|===|
{ selected, }`, "integer"),
	)

	DescribeTable("evaluates nullable conditions before conditional match expressions",
		func(source string, expected string) {
			result, err := New().Process(source)
			tAssert.NoError(err)
			tAssert.Equal(expected, result.Output["selected"].String)
		},
		Entry("with match at the question mark", `|===|
nullable boolean condition = true;
variant[string, int] value = 7;
string selected = condition ? match (value) {
  string => "string",
  int => "integer",
} : "fallback";
|===|
{ selected, }`, "integer"),
		Entry("with match at the colon", `|===|
nullable boolean condition = null;
variant[string, int] value = 7;
string selected = condition ? "fallback" : match (value) {
  string => "string",
  int => "integer",
};
|===|
{ selected, }`, "integer"),
	)

	DescribeTable("infers arms that return the matched variable",
		func(armExpression string) {
			source := fmt.Sprintf(`|===|
variant[string, int] value = "matched";
variant[string, int] selected = match (value) {
  string => %s,
  int => 0,
};
|===|
{ selected, }`, armExpression)

			result, err := New().Process(source)
			tAssert.NoError(err)
			tAssert.Equal("matched", result.Output["selected"].String)
		},
		Entry("directly", `value`),
		Entry("from the true ternary branch", `true ? value : 0`),
		Entry("from the false ternary branch", `false ? 0 : value`),
	)

	DescribeTable("rejects non-exhaustive variants from 2 through 12 members",
		func(arity int) {
			_, err := New().Process(variantMatchSource(arity, arity-1))
			tAssert.Error(err)
			tAssert.ErrorContains(err, "match expression must be exhaustive")
		},
		buildVariantMatchEntries(),
	)

	DescribeTable("matches arrays nested from 1 through 5 levels",
		func(depth int) {
			arrayType := nestedArrayType(depth)
			source := fmt.Sprintf(`|===|
variant[%s, int] value = %s;
string selected = match (value) { %s => "array", int => "int", };
|===|
{ selected, }`, arrayType, nestedArrayValue(depth), arrayType)

			result, err := New().Process(source)
			tAssert.NoError(err)
			tAssert.Equal("array", result.Output["selected"].String)
		},
		buildDepthMatchEntries(),
	)

	DescribeTable("matches records nested from 1 through 5 levels",
		func(depth int) {
			source := fmt.Sprintf(`|===|
%s
variant[Record%d, string] value = %s;
string selected = match (value) { Record%d => "record", string => "string", };
|===|
{ selected, }`, nestedRecordDeclarations(depth), depth, nestedRecordValue(depth), depth)

			result, err := New().Process(source)
			tAssert.NoError(err)
			tAssert.Equal("record", result.Output["selected"].String)
		},
		buildDepthMatchEntries(),
	)

	DescribeTable("rejects invalid matches",
		func(source, message string) {
			_, err := New().Process(wrapScriptWithOutput("|===|\n" + source + "\n|===|"))
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("concrete input", `string value = "Mace"; string selected = match (value) { string => "text", };`, "variant or choice"),
		Entry("missing variant arm", `variant[string, int] value = 1; string selected = match (value) { string => "text", };`, "exhaustive"),
		Entry("unknown variant arm", `variant[string, int] value = 1; string selected = match (value) { string => "text", boolean => "flag", int => "number", };`, "not a member"),
		Entry("duplicate variant arm", `variant[string, int] value = 1; string selected = match (value) { string => "text", string => "again", int => "number", };`, "duplicate"),
		Entry("wrong pattern kind", `variant[string, int] value = 1; string selected = match (value) { "text" => "text", int => "number", };`, "type pattern"),
		Entry("missing choice arm", `choice["on", "off"] value = "on"; int selected = match (value) { "on" => 1, };`, "exhaustive"),
		Entry("unknown choice arm", `choice["on", "off"] value = "on"; int selected = match (value) { "on" => 1, "off" => 0, "auto" => 2, };`, "not a member"),
		Entry("choice type pattern", `choice["on", "off"] value = "on"; int selected = match (value) { string => 1, "off" => 0, };`, "literal pattern"),
	)
})
