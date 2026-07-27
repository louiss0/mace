package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Null output code actions", func() {
	DescribeTable("preserves null's output-omission semantics",
		testCodeActionContract,
		Entry("removes a null variable", quickFix("Remove variable initialized with `null`", "mace.type.invalid-null-usage", "|===|\nstring value = null;\n|===|\n[output = 'data'] {}", "[output = 'data']")),
		Entry("inserts a typed fallback", quickFix("Replace `null` with typed fallback", "mace.type.invalid-null-usage", "|===|\nstring value = null;\n|===|\n[output = 'data'] { value: value, }", "string value = '';")),
		Entry("removes a null array member", preferredQuickFix("Remove `null` array element", "mace.type.invalid-null-usage", "[output = 'data'] { values: [1, null, 2], }", "[1, 2]")),
		Entry("removes a null record field", preferredQuickFix("Remove record field whose value is `null`", "mace.type.invalid-null-usage", "[output = 'data'] { record: { keep: 1, omit: null, }, }", "record: { keep: 1, }")),
		Entry("moves omission outward", rewrite("Move omission to output field", "mace.type.nested-null-omission", "[output = 'data'] { profile: { name: null, }, }", "profile: null")),
		Entry("rewrites a null comparison", rewrite("Replace comparison against `null` with optional access logic", "mace.operator.null-comparison", "[output = 'data'] { city: user.city == null ? '' : user.city, }", "user?.city ?? ''")),
		Entry("coalesces null interpolation", quickFix("Replace interpolated `null` with fallback expression", "mace.string.null-interpolation", "[output = 'data'] { text: \"$(maybe_null)\", }", "$(maybe_null ?? '')")),
	)
})
