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

func buildPairedMatchEntries(start int) []any {
	entries := make([]any, 0, 10)
	for index := range 10 {
		memberCount := start + index*2
		entries = append(entries, Entry(fmt.Sprintf("%d-member source variant", memberCount), memberCount))
	}
	return entries
}

func buildCompositeRedundancyEntries(kind string) []any {
	primitives := []struct {
		typeReference string
		initializer   string
	}{
		{typeReference: "string", initializer: `"text"`},
		{typeReference: "int", initializer: "42"},
		{typeReference: "float", initializer: "3.5"},
		{typeReference: "hex_int", initializer: "0xA"},
		{typeReference: "hex_float", initializer: "0x1.8"},
		{typeReference: "boolean", initializer: "true"},
	}

	entries := make([]any, 0, 10)
	for index := range 10 {
		primitive := primitives[index%len(primitives)]
		depth := index%5 + 1
		entries = append(entries, Entry(
			fmt.Sprintf("%s depth %d with %s", kind, depth, primitive.typeReference),
			kind,
			depth,
			primitive.typeReference,
			primitive.initializer,
		))
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

func nestedPrimitiveArray(depth int, typeReference string, initializer string) (string, string) {
	arrayType := typeReference
	arrayValue := initializer
	for range depth {
		arrayType = "array<" + arrayType + ">"
		arrayValue = "[" + arrayValue + "]"
	}
	return arrayType, arrayValue
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

func pairedMatchSource(memberCount int) string {
	schemas := make([]string, 0, memberCount)
	members := make([]string, 0, memberCount)
	for index := range memberCount {
		schemas = append(schemas, fmt.Sprintf("schema Member%d: { field%d: { value: string } };", index, index))
		members = append(members, fmt.Sprintf("Member%d", index))
	}

	pairCount := memberCount / 2
	pairs := make([]string, 0, pairCount)
	arms := make([]string, 0, pairCount+memberCount%2)
	for index := range pairCount {
		pairs = append(pairs, fmt.Sprintf("alias Pair%d: variant[Member%d, Member%d];", index, index*2, index*2+1))
		arms = append(arms, fmt.Sprintf("Pair%d => \"pair%d\",", index, index))
	}

	declarations := make([]string, 0, memberCount)
	outputs := make([]string, 0, memberCount)
	for index := range memberCount {
		matchArms := append([]string{}, arms...)
		if memberCount%2 == 1 {
			last := memberCount - 1
			matchArms = append(matchArms, fmt.Sprintf("Member%d => value%d.field%d.value,", last, index, last))
		}
		declarations = append(declarations, fmt.Sprintf(`PairedValue value%d = { field%d: { value: "member%d" } };
string result%d = match (value%d) {
  %s
};`, index, index, index, index, index, strings.Join(matchArms, "\n  ")))
		outputs = append(outputs, fmt.Sprintf("result%d: result%d", index, index))
	}

	return fmt.Sprintf(`|===|
%s
alias PairedValue: variant[%s];
%s
%s
|===|
{ %s }`, strings.Join(schemas, "\n"), strings.Join(members, ", "), strings.Join(pairs, "\n"), strings.Join(declarations, "\n"), strings.Join(outputs, ", "))
}

func redundantArrayMatchSource(depth int) string {
	arrayType := nestedArrayType(depth)
	return fmt.Sprintf(`|===|
alias Covered: variant[%s, int];
variant[%s, int, string] value = %s;
string selected = match (value) {
  Covered => "covered",
  %s => "redundant",
  string => "string",
};
|===|
{ selected, }`, arrayType, arrayType, nestedArrayValue(depth), arrayType)
}

func redundantRecordMatchSource(depth int) string {
	return fmt.Sprintf(`|===|
%s
alias Covered: variant[Record%d, string];
variant[Record%d, string, int] value = %s;
string selected = match (value) {
  Covered => "covered",
  Record%d => "redundant",
  int => "integer",
};
|===|
{ selected, }`, nestedRecordDeclarations(depth), depth, depth, nestedRecordValue(depth), depth)
}

func redundantPrimitiveMatchSource(typeReference string, initializer string) string {
	return fmt.Sprintf(`|===|
schema Marker: { value: string };
alias Covered: variant[%s, Marker];
variant[%s, Marker, array<string>] value = %s;
string selected = match (value) {
  Covered => "covered",
  %s => "redundant",
  array<string> => "array",
};
|===|
{ selected, }`, typeReference, typeReference, initializer, typeReference)
}

func redundantPrimitiveArrayMatchSource(depth int, typeReference string, initializer string) (string, string) {
	arrayType, arrayValue := nestedPrimitiveArray(depth, typeReference, initializer)
	return fmt.Sprintf(`|===|
schema Marker: { value: string };
alias Covered: variant[%s, Marker];
variant[%s, Marker, string] value = %s;
string selected = match (value) {
  Covered => "covered",
  %s => "redundant",
  string => "string",
};
|===|
{ selected, }`, arrayType, arrayType, arrayValue, arrayType), arrayType
}

func redundantPrimitiveRecordMatchSource(depth int, typeReference string, initializer string) (string, string) {
	recordName := fmt.Sprintf("PrimitiveRecord%d", depth)
	declarations := []string{fmt.Sprintf("schema PrimitiveRecord1: { value: %s };", typeReference)}
	recordValue := "{ value: " + initializer + " }"
	for level := 2; level <= depth; level++ {
		declarations = append(declarations, fmt.Sprintf("schema PrimitiveRecord%d: { nested: PrimitiveRecord%d };", level, level-1))
		recordValue = "{ nested: " + recordValue + " }"
	}

	return fmt.Sprintf(`|===|
%s
alias Covered: variant[%s, array<string>];
variant[%s, array<string>, boolean] value = %s;
string selected = match (value) {
  Covered => "covered",
  %s => "redundant",
  boolean => "boolean",
};
|===|
	{ selected, }`, strings.Join(declarations, "\n"), recordName, recordName, recordValue, recordName), recordName
}

func redundantCompositeMatchSource(kind string, depth int, typeReference string, initializer string) (string, string) {
	switch kind {
	case "array":
		return redundantPrimitiveArrayMatchSource(depth, typeReference, initializer)
	case "record":
		return redundantPrimitiveRecordMatchSource(depth, typeReference, initializer)
	default:
		return "", ""
	}
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
alias Status: choice["ready", "pending"];
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

	DescribeTable("evaluates conditions before conditional match expressions",
		func(source string, expected string) {
			result, err := New().Process(source)
			tAssert.NoError(err)
			tAssert.Equal(expected, result.Output["selected"].String)
		},
		Entry("with match at the question mark", `|===|
boolean condition = true;
variant[string, int] value = 7;
string selected = condition ? match (value) {
  string => "string",
  int => "integer",
} : "fallback";
|===|
{ selected, }`, "integer"),
		Entry("with match at the colon", `|===|
boolean condition = null;
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

	assertPairedMatchNarrowing := func(memberCount int) {
		result, err := New().Process(pairedMatchSource(memberCount))
		tAssert.NoError(err)
		for index := range memberCount {
			expected := fmt.Sprintf("pair%d", index/2)
			if memberCount%2 == 1 && index == memberCount-1 {
				expected = fmt.Sprintf("member%d", index)
			}
			tAssert.Equal(expected, result.Output[fmt.Sprintf("result%d", index)].String)
		}
	}

	evenPairedMatchTests := append([]any{assertPairedMatchNarrowing}, buildPairedMatchEntries(4)...)
	DescribeTable("exhausts even source variants with distinct two-member match targets", evenPairedMatchTests...)

	oddPairedMatchTests := append([]any{assertPairedMatchNarrowing}, buildPairedMatchEntries(5)...)
	DescribeTable("narrows the final odd match arm to one concrete remaining type", oddPairedMatchTests...)

	DescribeTable("rejects redundant nested array patterns covered by variant arms",
		func(depth int) {
			_, err := New().Process(redundantArrayMatchSource(depth))
			tAssert.Error(err)
			tAssert.ErrorContains(err, "duplicate match pattern "+nestedArrayType(depth))
		},
		buildDepthMatchEntries(),
	)

	DescribeTable("rejects redundant nested record patterns covered by variant arms",
		func(depth int) {
			_, err := New().Process(redundantRecordMatchSource(depth))
			tAssert.Error(err)
			tAssert.ErrorContains(err, fmt.Sprintf("duplicate match pattern Record%d", depth))
		},
		buildDepthMatchEntries(),
	)

	DescribeTable("rejects redundant primitive patterns covered by variant arms",
		func(typeReference string, initializer string) {
			_, err := New().Process(redundantPrimitiveMatchSource(typeReference, initializer))
			tAssert.Error(err)
			tAssert.ErrorContains(err, "duplicate match pattern "+typeReference)
		},
		Entry("string", "string", `"text"`),
		Entry("int", "int", "42"),
		Entry("float", "float", "3.5"),
		Entry("hex int", "hex_int", "0xA"),
		Entry("hex float", "hex_float", "0x1.8"),
		Entry("boolean", "boolean", "true"),
	)

	compositeRedundancyEntries := append(buildCompositeRedundancyEntries("array"), buildCompositeRedundancyEntries("record")...)
	compositeRedundancyTests := append([]any{
		func(kind string, depth int, typeReference string, initializer string) {
			source, redundantType := redundantCompositeMatchSource(kind, depth, typeReference, initializer)
			tAssert.NotEmpty(source)

			_, err := New().Process(source)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "duplicate match pattern "+redundantType)
		},
	}, compositeRedundancyEntries...)
	DescribeTable("rejects 20 redundant primitive array and record patterns", compositeRedundancyTests...)

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
		Entry("overlapping grouped variant arm", `alias Pair: variant[string, int]; variant[string, int, boolean] value = 1; string selected = match (value) { Pair => "pair", int => "number", boolean => "flag", };`, "duplicate"),
		Entry("wrong pattern kind", `variant[string, int] value = 1; string selected = match (value) { "text" => "text", int => "number", };`, "type pattern"),
		Entry("missing choice arm", `choice["on", "off"] value = "on"; int selected = match (value) { "on" => 1, };`, "exhaustive"),
		Entry("unknown choice arm", `choice["on", "off"] value = "on"; int selected = match (value) { "on" => 1, "off" => 0, "auto" => 2, };`, "not a member"),
		Entry("choice type pattern", `choice["on", "off"] value = "on"; int selected = match (value) { string => 1, "off" => 0, };`, "literal pattern"),
	)
})
