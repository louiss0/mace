package processor

import (
	"fmt"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
)

var _ = Describe("Variant type narrowing", func() {
	type nestedTernaryMember struct {
		typeReference string
		initializer   string
		result        string
		branch        string
	}

	members := []nestedTernaryMember{
		{typeReference: "string", initializer: `"text"`, result: "string", branch: `"string"`},
		{typeReference: "int", initializer: "42", result: "int", branch: `"int"`},
		{typeReference: "float", initializer: "3.5", result: "float", branch: `"float"`},
		{typeReference: "boolean", initializer: "true", result: "boolean", branch: `"boolean"`},
		{typeReference: "hex_int", initializer: "0xA", result: "hex_int", branch: `"hex_int"`},
		{typeReference: "hex_float", initializer: "0x1.8", result: "hex_float", branch: `"hex_float"`},
		{typeReference: "array<string>", initializer: `["item"]`, result: "array", branch: `"array"`},
		{typeReference: "AlphaConfig", initializer: `{ details: { source: "alpha" } }`, result: "alpha", branch: "$value.details.source"},
		{typeReference: "BetaConfig", initializer: `{ metadata: { source: "beta" } }`, result: "beta", branch: "$value.metadata.source"},
		{typeReference: "GammaConfig", initializer: `{ payload: { source: "gamma" } }`, result: "gamma", branch: "$value.payload.source"},
	}

	buildNestedTernaryFixture := func(arity int) string {
		activeMembers := members[:arity]
		typeReferences := lo.Map(activeMembers, func(member nestedTernaryMember, _ int) string {
			return member.typeReference
		})
		declarations := lo.Map(activeMembers, func(member nestedTernaryMember, index int) string {
			variableName := fmt.Sprintf("value%d", index)
			resultName := fmt.Sprintf("result%d", index)
			branches := lo.Map(activeMembers, func(candidate nestedTernaryMember, _ int) string {
				branch := strings.ReplaceAll(candidate.branch, "$value", variableName)
				return fmt.Sprintf("%s is %s ? %s :", variableName, candidate.typeReference, branch)
			})
			branches = append(branches, `"unmatched"`)

			return fmt.Sprintf("NestedValue %s = %s;\nstring %s = %s;", variableName, member.initializer, resultName, strings.Join(branches, "\n  "))
		})
		outputs := lo.Map(activeMembers, func(_ nestedTernaryMember, index int) string {
			resultName := fmt.Sprintf("result%d", index)
			return fmt.Sprintf("%s: %s", resultName, resultName)
		})

		return fmt.Sprintf(`|===|
schema AlphaConfig: { details: { source: string } };
schema BetaConfig: { metadata: { source: string } };
schema GammaConfig: { payload: { source: string } };
type NestedValue: variant[%s];
%s
|===| { %s }`, strings.Join(typeReferences, ", "), strings.Join(declarations, "\n"), strings.Join(outputs, ", "))
	}

	buildDepthEntries := func(start int, count int) []any {
		return lo.Map(lo.RangeFrom(start, count), func(depth int, _ int) any {
			return Entry(fmt.Sprintf("%d levels", depth), depth)
		})
	}

	buildNestedArrayFixture := func(depth int) string {
		typeReference := strings.Repeat("array<", depth) + "string" + strings.Repeat(">", depth)
		initializer := strings.Repeat("[", depth) + `"item"` + strings.Repeat("]", depth)

		return fmt.Sprintf(`|===|
type NestedArray: variant[%s, string];
NestedArray arrayValue = %s;
NestedArray stringValue = "fallback";
string matched = arrayValue is %s ? "matched" : arrayValue;
string fallback = stringValue is %s ? "matched" : stringValue;
|===| { matched: matched, fallback: fallback }`, typeReference, initializer, typeReference, typeReference)
	}

	buildNestedRecordFixture := func(depth int) string {
		fieldNames := lo.Map(lo.Range(depth-1), func(index int, _ int) string {
			return fmt.Sprintf("level%d", index+1)
		})
		nestedFields := strings.Join(lo.Map(fieldNames, func(name string, _ int) string {
			return name + ": { "
		}), "")
		schemaFields := nestedFields + "value: string" + strings.Repeat(" }", len(fieldNames))
		initializer := "{ " + nestedFields + `value: "record"` + strings.Repeat(" }", len(fieldNames)) + " }"
		valuePath := "recordValue." + strings.Join(lo.Concat(fieldNames, []string{"value"}), ".")

		return fmt.Sprintf(`|===|
schema NestedRecord: { %s };
type NestedRecordValue: variant[NestedRecord, string];
NestedRecordValue recordValue = %s;
NestedRecordValue stringValue = "fallback";
string matched = recordValue is NestedRecord ? %s : "unmatched";
string fallback = stringValue is NestedRecord ? "matched" : stringValue;
|===| { matched: matched, fallback: fallback }`, schemaFields, initializer, valuePath)
	}

	DescribeTable("narrows every runtime value kind through nested ternaries",
		func(arity int) {
			result, err := New().Process(buildNestedTernaryFixture(arity))

			tAssert.NoError(err)
			lo.ForEach(members[:arity], func(member nestedTernaryMember, index int) {
				tAssert.Equal(member.result, result.Output[fmt.Sprintf("result%d", index)].String)
			})
		},
		Entry("2 nested ternaries", 2),
		Entry("3 nested ternaries", 3),
		Entry("4 nested ternaries", 4),
		Entry("5 nested ternaries", 5),
		Entry("6 nested ternaries", 6),
		Entry("7 nested ternaries", 7),
		Entry("8 nested ternaries", 8),
		Entry("9 nested ternaries", 9),
		Entry("10 nested ternaries", 10),
	)

	arrayTests := append([]any{
		func(depth int) {
			result, err := New().Process(buildNestedArrayFixture(depth))

			tAssert.NoError(err)
			tAssert.Equal("matched", result.Output["matched"].String)
			tAssert.Equal("fallback", result.Output["fallback"].String)
		},
	}, buildDepthEntries(1, 10)...)
	DescribeTable("narrows variants containing arrays nested up to ten levels", arrayTests...)

	recordTests := append([]any{
		func(depth int) {
			result, err := New().Process(buildNestedRecordFixture(depth))

			tAssert.NoError(err)
			tAssert.Equal("record", result.Output["matched"].String)
			tAssert.Equal("fallback", result.Output["fallback"].String)
		},
	}, buildDepthEntries(2, 9)...)
	DescribeTable("narrows variants containing records nested up to ten levels", recordTests...)

	It("processes all five schema members in the narrowing fixture", func() {
		result, err := New().ProcessFile("../../fixtures/processor/type_narrowing/five_configs.mace")

		tAssert.NoError(err)
		tAssert.Equal("./mace.toml", result.Output["local"].String)
		tAssert.Equal("https://example.com/mace", result.Output["remote"].String)
		tAssert.Equal("MACE_CONFIG", result.Output["environment"].String)
		tAssert.Equal("mace/config", result.Output["secret"].String)
		tAssert.Equal("inline configuration", result.Output["inline"].String)
	})

	It("preserves narrowed members when a conditional returns the original variant", func() {
		result, err := New().Process(`|===|
variant[string, int] value = "Mace";
variant[string, int] selected = value is string ? value : value;
|===| { selected: selected }`)

		tAssert.NoError(err)
		tAssert.Equal("Mace", result.Output["selected"].String)
	})

	It("narrows primitive variant members in both conditional branches", func() {
		result, err := New().Process(`|===|
variant[string, int] text = "Mace";
variant[string, int] number = 42;
string narrowedText = text is string ? text : "fallback";
int narrowedNumber = number is string ? 0 : number;
|===| {
    text: narrowedText,
    number: narrowedNumber,
}`)

		tAssert.NoError(err)
		tAssert.Equal("Mace", result.Output["text"].String)
		tAssert.Equal(int64(42), result.Output["number"].Int)
	})

	It("accepts meaningful concrete and choice type tests", func() {
		result, err := New().Process(`|===|
type Status: choice["ready", "pending"];
string name = "Mace";
variant[Status, int] status = "ready";
boolean concrete = name is string;
boolean selected = status is Status;
|===| { concrete: concrete, selected: selected }`)

		tAssert.NoError(err)
		tAssert.True(result.Output["concrete"].Boolean)
		tAssert.True(result.Output["selected"].Boolean)
	})

	It("narrows aliases and nested conditional branches", func() {
		result, err := New().Process(`|===|
type Name: string;
variant[Name, int, boolean] value = 7;
string result = value is Name ? value : value is int ? "$(value)" : value ? "true" : "false";
|===| { result: result }`)

		tAssert.NoError(err)
		tAssert.Equal("7", result.Output["result"].String)
	})

	It("narrows stable member paths", func() {
		result, err := New().Process(`|===|
schema Holder: { value: variant[string, int] };
Holder holder = { value: "Mace" };
string result = holder.value is string ? holder.value : "fallback";
|===| { result: result }`)

		tAssert.NoError(err)
		tAssert.Equal("Mace", result.Output["result"].String)
	})

	It("narrows closed schema variants before member access", func() {
		result, err := New().Process(`|===|
schema LocalConfig: { path: string };
schema RemoteConfig: { url: string };
variant[LocalConfig, RemoteConfig] config = { path: "/tmp" };
string source = config is LocalConfig ? config.path : config.url;
|===| { source: source }`)

		tAssert.NoError(err)
		tAssert.Equal("/tmp", result.Output["source"].String)
	})

	It("narrows nullable values to their present type", func() {
		result, err := New().Process(`|===|
nullable string value = "present";
string result = value is string ? value : "fallback";
|===| { result: result }`)

		tAssert.NoError(err)
		tAssert.Equal("present", result.Output["result"].String)
	})

	It("reports impossible and repeated narrowing", func() {
		_, err := New().Process(`|===|
variant[string, int] value = "Mace";
boolean result = value is boolean;
|===| { result: result }`)
		tAssert.Error(err)
		diagnostic, ok := err.(DiagnosticError)
		tAssert.True(ok)
		tAssert.Equal(CodeImpossibleNarrowing, diagnostic.Code)
		tAssert.Contains(diagnostic.Message, "variant[string, int]")
		tAssert.Contains(diagnostic.Message, "boolean")

		_, err = New().Process(`|===|
variant[string, int] value = "Mace";
string result = value is string ? value : value is string ? value : "fallback";
|===| { result: result }`)
		tAssert.Error(err)
		diagnostic, ok = err.(DiagnosticError)
		tAssert.True(ok)
		tAssert.Equal(CodeImpossibleNarrowing, diagnostic.Code)
	})

	It("does not leak narrowing or track stored booleans", func() {
		_, err := New().Process(`|===|
variant[string, int] value = "Mace";
string first = value is string ? value : "fallback";
string second = value;
|===| { first: first }`)
		tAssert.Error(err)

		_, err = New().Process(`|===|
variant[string, int] value = "Mace";
boolean check = value is string;
string result = check ? value : "fallback";
|===| { result: result }`)
		tAssert.Error(err)
	})

	DescribeTable("rejects member access before and in the wrong narrowing branch",
		func(source string, expectedType string) {
			_, err := New().Process(source)

			tAssert.Error(err)
			if expectedType != "" {
				tAssert.Contains(err.Error(), expectedType)
			}
		},
		Entry("before narrowing", `|===|
schema LocalConfig: { path: string };
schema RemoteConfig: { url: string };
variant[LocalConfig, RemoteConfig] config = { path: "/tmp" };
string source = config.path;
|===| { source: source }`, ""),
		Entry("true branch", `|===|
schema LocalConfig: { path: string };
schema RemoteConfig: { url: string };
variant[LocalConfig, RemoteConfig] config = { path: "/tmp" };
string source = config is LocalConfig ? config.url : config.path;
|===| { source: source }`, "LocalConfig"),
		Entry("false branch", `|===|
schema LocalConfig: { path: string };
schema RemoteConfig: { url: string };
variant[LocalConfig, RemoteConfig] config = { path: "/tmp" };
string source = config is LocalConfig ? config.path : config.path;
|===| { source: source }`, "RemoteConfig"),
	)

	It("does not narrow negated type tests", func() {
		_, err := New().Process(`|===|
variant[string, int] value = "Mace";
string result = !(value is string) ? value : "fallback";
|===| { result: result }`)
		tAssert.Error(err)
	})

	It("evaluates false and nullable-absent tests without weakening static checks", func() {
		result, err := New().Process(`|===|
variant[string, int] value = 10;
nullable string missing = null;
boolean differentMember = value is string;
boolean absent = missing is string;
|===| { differentMember: differentMember, absent: absent }`)

		tAssert.NoError(err)
		tAssert.False(result.Output["differentMember"].Boolean)
		tAssert.False(result.Output["absent"].Boolean)
	})
})
