package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Primitive and literal type code actions", func() {
	DescribeTable("satisfies every string, boolean, decimal, and hexadecimal contract",
		testCodeActionContract,
		Entry("quotes a value", quickFix("Quote value as string", "mace.type.expected-string", "|===|\nstring env = production;\n|===|\n[output = 'data'] { env: env, }", "'production'")),
		Entry("converts an interpolating string", quickFix("Convert interpolating string to single-quoted string", "mace.string.interpolation-forbidden", "[output = 'data'] { text: \"$name\", }", "'$name'")),
		Entry("expands string interpolation", preferredQuickFix("Replace `$name` interpolation with `$(name)`", "mace.string.unsupported-interpolation", "|===|\nstring name = 'Mace';\n|===|\n[output = 'data'] { text: \"Hello $name\", }", "$(name)")),
		Entry("escapes an invalid character", preferredQuickFix("Escape invalid string character", "mace.string.invalid-character", "[output = 'data'] { path: 'C:\\temp', }", "C:\\\\temp")),
		Entry("converts multiline text", quickFix("Convert multiline string to block string", "mace.string.line-break", "[output = 'data'] { text: 'first\nsecond', }", "'''", "first\nsecond")),
		Entry("converts a string boolean", preferredQuickFix("Replace string boolean with boolean literal", "mace.type.string-boolean", "[output = 'data'] { enabled: 'true', }", "enabled: true")),
		Entry("converts a numeric boolean", quickFix("Replace numeric boolean with boolean literal", "mace.type.numeric-boolean", "[output = 'data'] { enabled: 1, }", "enabled: true")),
		Entry("widens an integer literal", preferredQuickFix("Convert integer literal to float", "mace.type.expected-float", "|===|\nfloat ratio = 1;\n|===|\n[output = 'data'] { ratio: ratio, }", "1.0")),
		Entry("narrows an integral float", quickFix("Convert float literal to integer", "mace.type.expected-int", "|===|\nint count = 2.0;\n|===|\n[output = 'data'] { count: count, }", "2;")),
		Entry("replaces an overflowing integer", quickFix("Replace overflowing integer literal", "mace.number.integer-overflow", "[output = 'data'] { value: 999999999999999999999999, }", "9223372036854775807")),
		Entry("changes int receiving type", rewrite("Change receiving type from `int` to `float`", "mace.type.operator-result-mismatch", "|===|\nint ratio = 3 / 2;\n|===|\n[output = 'data'] { ratio: ratio, }", "float ratio")),
		Entry("widens hex integer", preferredQuickFix("Convert `hex_int` literal to `hex_float`", "mace.type.expected-hex-float", "|===|\nhex_float value = 0x2;\n|===|\n[output = 'data'] { value: value, }", "0x2.0")),
		Entry("converts decimal to hex", quickFix("Convert decimal literal to hexadecimal family", "mace.type.mixed-numeric-family", "[output = 'data'] { value: 0x2 + 3, }", "0x3")),
		Entry("converts hex to decimal", quickFix("Convert hexadecimal literal to decimal family", "mace.type.mixed-numeric-family", "[output = 'data'] { value: 0x2 + 3, }", "2 + 3")),
		Entry("completes a hex fraction", preferredQuickFix("Add required hexadecimal fractional component", "mace.number.incomplete-hex-float", "[output = 'data'] { value: 0x2., }", "0x2.0")),
		Entry("canonicalizes hex float", sourceAction("Canonicalize hexadecimal float literal", "source.fixAll.mace", "[output = 'data'] { value: 0X02.A0, }", "0x2.a")),
		Entry("clamps a hex integer", quickFix("Replace overflowing `hex_int` literal with boundary value", "mace.number.hex-integer-overflow", "[output = 'data'] { value: 0xffffffffffffffffffff, }", "0x7fffffffffffffff")),
		Entry("changes complement operand", quickFix("Change `~` operand to decimal `int`", "mace.type.invalid-complement-operand", "|===|\nhex_int mask = 0xff;\nint value = ~mask;\n|===|\n[output = 'data'] { value: value, }", "int mask = 255")),
	)
})
