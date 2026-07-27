package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Self and recursive schema code actions", func() {
	DescribeTable("keeps self references symbolic and correctly scoped",
		testCodeActionContract,
		Entry("moves self into output", rewrite("Move `$self` use into data output", "mace.type.self-outside-output", "|===|\nint next = $self.value + 1;\n|===|\n[output = 'data'] { value: 1, next: next, }", "next: $self.value + 1")),
		Entry("uses a local variable", quickFix("Replace `$self` with local variable", "mace.type.self-outside-output", "|===|\nint value = 1; int next = $self.value + 1;\n|===|\n[output = 'data'] { next: next, }", "value + 1")),
		Entry("uses parsed input", quickFix("Replace `$self` with parsed input reference", "mace.type.self-outside-output", "|===|\nschema Runtime: { value: int, };\n|===|\n[output = 'data', parse = Runtime] { next: $self.value + 1, }", "$value + 1")),
		Entry("reorders a referenced field", preferredQuickFix("Reorder output field before `$self` reference", "mace.type.self-forward-reference", "[output = 'data'] { next: $self.value + 1, value: 1, }", "value: 1", "next: $self.value + 1")),
		Entry("uses a direct variable", quickFix("Replace forward `$self` reference with direct variable", "mace.type.self-forward-reference", "|===|\nint value = 1;\n|===|\n[output = 'data'] { next: $self.value + 1, value: value, }", "next: value + 1")),
		Entry("guards recursion in an array", rewrite("Guard recursive schema through `array<$self>`", "mace.type.unguarded-self-recursion", "|===|\nschema Node: { child: $self, };\n|===|\n[output = 'schema'] { Node: Node, }", "children: array<$self>")),
		Entry("replaces direct recursion", quickFix("Replace direct `$self` field with named nonrecursive type", "mace.type.unguarded-self-recursion", "|===|\nschema Node: { child: $self, };\n|===|\n[output = 'schema'] { Node: Node, }", "child: Leaf")),
		Entry("moves recursive alias", extract("Move recursive alias into schema context", "mace.type.self-outside-schema", "|===|\nalias Node: { children: array<$self>, };\n|===|\n[output = 'schema'] { Node: Node, }", "schema Node")),
		Entry("converts an alias cycle", extract("Convert alias recursion to guarded schema recursion", "mace.type.alias-cycle", "|===|\nalias Node: { child: Node, };\n|===|\n[output = 'schema'] { Node: Node, }", "schema Node", "array<$self>")),
		Entry("removes unused recursion", quickFix("Remove unused recursive field", "mace.type.unguarded-self-recursion", "|===|\nschema Node: { value: string, child: $self, };\n|===|\n[output = 'schema'] { Node: Node, }", "value: string")),
	)
})
