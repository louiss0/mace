package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Operator and operand relationship code actions", func() {
	DescribeTable("satisfies every operator contract without guessing intent",
		testCodeActionContract,
		Entry("uses bitwise and", quickFix("Replace logical operator with bitwise operator", "mace.type.invalid-binary-operator", "[output = 'data'] { value: 1 && 2, }", "1 & 2")),
		Entry("uses logical and", quickFix("Replace bitwise operator with logical operator", "mace.type.invalid-binary-operator", "[output = 'data'] { value: true & false, }", "true && false")),
		Entry("uses boolean negation", quickFix("Replace `~` with `!`", "mace.type.invalid-unary-operator", "[output = 'data'] { value: ~true, }", "!true")),
		Entry("matches numeric family", quickFix("Convert operand to matching numeric family", "mace.type.mixed-numeric-family", "[output = 'data'] { value: 0x2 + 3, }", "0x2 + 0x3")),
		Entry("uses result type", preferredQuickFix("Change receiving type to operator result type", "mace.type.operator-result-mismatch", "|===|\nhex_int value = 0x4 / 0x2;\n|===|\n[output = 'data'] { value: value, }", "hex_float value")),
		Entry("selects compatible operator", quickFix("Replace invalid operator with compatible operator", "mace.type.invalid-binary-operator", "[output = 'data'] { value: true + false, }", "true || false")),
		Entry("groups arithmetic", rewrite("Add arithmetic grouping that changes precedence", "mace.expression.suspicious-precedence", "[output = 'data'] { value: 1 + 2 * 3, }", "(1 + 2) * 3")),
		Entry("fixes exponent", quickFix("Make exponent non-negative integer", "mace.operator.invalid-exponent", "[output = 'data'] { value: 2 ** -1, }", "2 ** 1")),
		Entry("fixes shift type", quickFix("Convert shift amount to integer literal", "mace.operator.invalid-shift", "[output = 'data'] { value: 1 << 2.0, }", "1 << 2")),
		Entry("fixes negative shift", quickFix("Replace negative shift amount", "mace.operator.invalid-shift", "[output = 'data'] { value: 1 << -2, }", "1 << 0")),
		Entry("guards division", rewrite("Guard division with conditional", "mace.operator.possible-division-by-zero", "[output = 'data'] { value: total / count, }", "count == 0 ?")),
		Entry("replaces zero divisor", quickFix("Replace known zero divisor", "mace.operator.division-by-zero", "[output = 'data'] { value: 10 / 0, }", "10 / 1")),
		Entry("clamps overflow", quickFix("Replace overflowing constant expression with literal result boundary", "mace.operator.constant-overflow", "[output = 'data'] { value: 9223372036854775807 + 1, }", "9223372036854775807")),
		Entry("changes arithmetic to float", rewrite("Change arithmetic family to float", "mace.operator.integer-result-insufficient", "|===|\nint value = 3 / 2;\n|===|\n[output = 'data'] { value: value, }", "float value = 3.0 / 2.0")),
	)
})
