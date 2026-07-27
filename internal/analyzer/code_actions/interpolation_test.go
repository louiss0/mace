package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Interpolation code actions", func() {
	DescribeTable("keeps interpolation scalar and explicit",
		testCodeActionContract,
		Entry("expands shorthand", preferredQuickFix("Replace shorthand interpolation with `$(…)`", "mace.string.unsupported-interpolation", "|===|\nstring name = 'Mace';\n|===|\n[output = 'data'] { text: \"Hello $name\", }", "$(name)")),
		Entry("uses a scalar record field", quickFix("Use scalar field from record", "mace.string.nonscalar-interpolation", "[output = 'data'] { text: \"$(user)\", }", "$(user.name)")),
		Entry("matches a variant", rewrite("Match variant before interpolation", "mace.string.nonscalar-interpolation", "[output = 'data'] { text: \"$(value)\", }", "match (value)")),
		Entry("coalesces optional interpolation", preferredQuickFix("Coalesce optional value before interpolation", "mace.string.absent-interpolation", "[output = 'data'] { text: \"$(user?.name)\", }", "$(user?.name ?? '')")),
		Entry("coalesces null interpolation", quickFix("Replace null-producing expression with fallback", "mace.string.null-interpolation", "[output = 'data'] { text: \"$(maybe_null)\", }", "$(maybe_null ?? '')")),
		Entry("makes interpolation literal", preferredQuickFix("Convert interpolating string to literal string", "mace.string.unintended-interpolation", "[output = 'data'] { text: \"$HOME\", }", "'$HOME'")),
		Entry("extracts a scalar", extract("Extract scalar expression before interpolation", "mace.string.complex-interpolation", "[output = 'data'] { text: \"$(match (value) { string => value, int => value, })\", }", "string interpolated_value")),
		Entry("removes nonscalar interpolation", quickFix("Remove nonscalar interpolation", "mace.string.nonscalar-interpolation", "[output = 'data'] { text: \"prefix $(values) suffix\", }", "'prefix  suffix'")),
	)
})
