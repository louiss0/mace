package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Conditional expressions", func() {
	It("evaluates one conditional level", func() {
		result, err := New().Process(`|===|
string selected = true ? "primary" : "fallback";
|===|
{ selected, }`)

		tAssert.NoError(err)
		tAssert.Equal("primary", result.Output["selected"].String)
	})

	It("infers a variant from unlike branches", func() {
		result, err := New().Process(`|===|
variant[string, int] selected = false ? "primary" : 7;
|===|
{ selected, }`)

		tAssert.NoError(err)
		tAssert.Equal(int64(7), result.Output["selected"].Int)
	})

	DescribeTable("rejects nested conditionals",
		func(expression string) {
			_, err := New().Process(wrapScriptWithOutput(`|===|
string selected = ` + expression + `;
|===|`))

			tAssert.Error(err)
			tAssert.ErrorContains(err, "nested conditional expressions are not allowed")
		},
		Entry("then branch", `true ? false ? "one" : "two" : "three"`),
		Entry("else branch", `true ? "one" : false ? "two" : "three"`),
		Entry("condition", `(true ? true : false) ? "one" : "two"`),
	)

	It("requires every inferred branch type in the declared variant", func() {
		_, err := New().Process(wrapScriptWithOutput(`|===|
variant[string, int] selected = true ? "primary" : false;
|===|`))

		tAssert.Error(err)
		tAssert.ErrorContains(err, "boolean")
	})
})
