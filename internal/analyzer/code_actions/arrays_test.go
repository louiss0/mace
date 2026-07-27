package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Array code actions", func() {
	DescribeTable("satisfies every array contract using resolved element types",
		testCodeActionContract,
		Entry("changes mixed element type", quickFix("Change array element type to `variant[…]`", "mace.type.mixed-array-literal", "|===|\narray<string> values = ['ready', 3];\n|===|\n[output = 'data'] { values: values, }", "array<variant[string, int]>")),
		Entry("extracts a variant alias", extract("Wrap conflicting array members in a variant alias", "mace.type.mixed-array-literal", "|===|\narray<{ name: string, }> values = [{ name: 'Mace', }, 3];\n|===|\n[output = 'data'] { values: values, }", "alias ValuesItem: variant", "array<ValuesItem>")),
		Entry("converts one member", preferredQuickFix("Convert incompatible element to expected type", "mace.type.array-element-mismatch", "|===|\narray<float> values = [1.0, 2];\n|===|\n[output = 'data'] { values: values, }", "2.0")),
		Entry("removes one member", quickFix("Remove incompatible array element", "mace.type.mixed-array-literal", "|===|\narray<string> values = ['ready', 3];\n|===|\n[output = 'data'] { values: values, }", "['ready']")),
		Entry("changes declared element type", quickFix("Change declared array type to actual element type", "mace.type.initializer-type-mismatch", "|===|\narray<string> values = [1, 2];\n|===|\n[output = 'data'] { values: values, }", "array<int>")),
		Entry("types an empty array", preferredQuickFix("Add explicit type for empty array", "mace.type.untyped-empty-array", "|===|\nvalues = [];\n|===|\n[output = 'data'] { values: values, }", "array<string> values")),
		Entry("attaches output schema", preferredQuickFix("Attach output schema for empty array", "mace.type.untyped-output-collection", "[output = 'data'] { values: [], }", "schema = Output")),
		Entry("generates inline output schema", quickFix("Generate inline output schema for empty array", "mace.type.untyped-output-collection", "[output = 'data'] { values: [], }", "values: array<string>")),
		Entry("creates named output schema", extract("Create named schema for empty array output", "mace.type.untyped-output-collection", "[output = 'data'] { values: [], }", "schema Output", "values: array<string>")),
		Entry("types both conditional branches", quickFix("Type both conditional array branches", "mace.type.untyped-conditional-collection", "|===|\nboolean enabled = true;\nvalues = enabled ? [] : [];\n|===|\n[output = 'data'] { values: values, }", "array<string> values")),
		Entry("expands variant with array", quickFix("Expand receiving variant with `array<T>`", "mace.type.conditional-result-mismatch", "|===|\nvariant[string, int] value = true ? [] : 1;\n|===|\n[output = 'data'] { value: value, }", "variant[string, int, array<string>]")),
		Entry("flattens nested array variant", preferredQuickFix("Flatten nested array variant member", "mace.type.nested-array-variant", "|===|\nalias Item: variant[string, variant[int, boolean]];\narray<Item> values = [];\n|===|\n[output = 'data'] { values: values, }", "variant[string, int, boolean]")),
		Entry("extracts typed empty array", extract("Replace empty array with typed variable", "mace.type.untyped-output-collection", "[output = 'data'] { values: [], }", "array<string> values = [];", "values: values")),
	)
})
