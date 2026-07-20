package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Conditional expression code actions", func() {
	DescribeTable("satisfies every conditional contract",
		testCodeActionContract,
		Entry("extracts a nested conditional", extract("Extract nested conditional into variable", "mace.expression.nested-conditional", "[output = 'data'] { value: a ? (b ? 1 : 2) : 3, }", "int nested = b ? 1 : 2;")),
		Entry("matches the inner condition", rewrite("Replace inner conditional with match", "mace.expression.nested-conditional", "[output = 'data'] { value: enabled ? (mode == 'dev' ? 1 : 2) : 3, }", "match (mode)")),
		Entry("matches the outer condition", rewrite("Replace outer conditional with match", "mace.expression.conditional-domain", "[output = 'data'] { value: mode == 'dev' ? 1 : 2, }", "match (mode)")),
		Entry("widens receiving type", quickFix("Expand receiving type to inferred variant", "mace.type.conditional-result-mismatch", "|===|\nstring value = true ? 'text' : 1;\n|===|\n[output = 'data'] { value: value, }", "variant[string, int] value")),
		Entry("converts one branch", preferredQuickFix("Convert branch to expected type", "mace.type.conditional-result-mismatch", "|===|\nfloat value = true ? 1.0 : 2;\n|===|\n[output = 'data'] { value: value, }", "2.0")),
		Entry("types a branch collection", preferredQuickFix("Add explicit collection type to declaration", "mace.type.untyped-conditional-collection", "|===|\nvalues = true ? [] : ['Mace'];\n|===|\n[output = 'data'] { values: values, }", "array<string> values")),
		Entry("attaches collection schema", preferredQuickFix("Attach output schema for conditional collection", "mace.type.untyped-conditional-collection", "[output = 'data'] { values: enabled ? [] : ['Mace'], }", "schema = Output")),
		Entry("flattens duplicate members", sourceAction("Flatten duplicate conditional result members", "source.fixAll.mace", "|===|\nalias Text: string; variant[string, Text] value = true ? 'a' : 'b';\n|===|\n[output = 'data'] { value: value, }", "variant[string]")),
		Entry("selects a constant branch", preferredQuickFix("Replace constant conditional with selected branch", "mace.expression.constant-conditional", "[output = 'data'] { value: true ? 'yes' : 'no', }", "value: 'yes'")),
		Entry("extracts repeated branches", extract("Extract repeated branch expression", "mace.refactor.repeated-branch-expression", "[output = 'data'] { value: enabled ? prefix + suffix : fallback + suffix, }", "string shared_suffix = suffix")),
	)
})
