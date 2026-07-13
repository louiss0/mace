package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Variant type narrowing", func() {
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

	It("narrows array variants and evaluates array access", func() {
		result, err := New().Process(`|===|
variant[array<string>, string] value = ["first"];
string result = value is array<string> ? value[0] : value;
|===| { result: result }`)

		tAssert.NoError(err)
		tAssert.Equal("first", result.Output["result"].String)
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

	It("rejects member access before and in the wrong narrowing branch", func() {
		_, err := New().Process(`|===|
schema LocalConfig: { path: string };
schema RemoteConfig: { url: string };
variant[LocalConfig, RemoteConfig] config = { path: "/tmp" };
string source = config.path;
|===| { source: source }`)
		tAssert.Error(err)

		_, err = New().Process(`|===|
schema LocalConfig: { path: string };
schema RemoteConfig: { url: string };
variant[LocalConfig, RemoteConfig] config = { path: "/tmp" };
string source = config is LocalConfig ? config.url : config.path;
|===| { source: source }`)
		tAssert.Error(err)
		tAssert.Contains(err.Error(), "LocalConfig")

		_, err = New().Process(`|===|
schema LocalConfig: { path: string };
schema RemoteConfig: { url: string };
variant[LocalConfig, RemoteConfig] config = { path: "/tmp" };
string source = config is LocalConfig ? config.path : config.path;
|===| { source: source }`)
		tAssert.Error(err)
		tAssert.Contains(err.Error(), "RemoteConfig")
	})

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
