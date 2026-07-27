package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Variant code actions", func() {
	It("removes an equivalent variant member after type resolution", func() {
		source := `|===|
alias Text: string;
alias Value: variant[Text, string, int];
Value value = 'Mace';
|===|
[output = 'data']
{
  value: value,
}`
		expected := `|===|
alias Text: string;
alias Value: variant[Text, int];
Value value = 'Mace';
|===|
[output = 'data']
{
  value: value,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.duplicate-variant-member",
			title:          "Remove duplicate variant member",
			result:         expected,
		})
	})

	DescribeTable("satisfies every remaining variant contract",
		testCodeActionContract,
		Entry("flattens a nested variant", preferredQuickFix("Flatten nested variant", "mace.type.nested-variant", "|===|\nalias Value: variant[string, variant[int, boolean]];\n|===|\n[output = 'schema'] { Value: Value, }", "variant[string, int, boolean]")),
		Entry("expands a receiving variant", quickFix("Expand receiving variant with inferred member", "mace.type.variant-result-mismatch", "|===|\nvariant[string, int] value = true ? 'text' : false;\n|===|\n[output = 'data'] { value: value, }", "variant[string, int, boolean]")),
		Entry("uses an inferred variant", rewrite("Replace declaration type with inferred variant", "mace.type.variant-result-mismatch", "|===|\nstring value = true ? 'text' : 1;\n|===|\n[output = 'data'] { value: value, }", "variant[string, int] value")),
		Entry("removes overlap", quickFix("Remove overlapping variant member", "mace.type.overlapping-variant-member", "|===|\nalias Value: variant[string, choice['dev', 'prod']];\n|===|\n[output = 'schema'] { Value: Value, }", "variant[string]")),
		Entry("narrows a broad member", rewrite("Replace broad member with narrower choice", "mace.type.overlapping-variant-member", "|===|\nalias Value: variant[string, choice['dev', 'prod']];\n|===|\n[output = 'schema'] { Value: Value, }", "choice['dev', 'prod']")),
		Entry("merges aliases", quickFix("Merge equivalent aliases in variant", "mace.type.duplicate-variant-member", "|===|\nalias A: string; alias B: string; alias Value: variant[A, B];\n|===|\n[output = 'schema'] { Value: Value, }", "variant[A]")),
		Entry("resolves an invalid member", quickFix("Replace invalid variant member with resolved type", "mace.type.invalid-variant-member", "|===|\nalias Value: variant[string, OutputMetadata];\n|===|\n[output = 'schema'] { Value: Value, }", "variant[string, int]")),
		Entry("extracts a record member", extract("Extract inline record variant member to alias", "mace.refactor.repeated-variant-record", "|===|\nalias Value: variant[{ name: string, age: int, }, string];\n|===|\n[output = 'schema'] { Value: Value, }", "alias User", "variant[User, string]")),
		Entry("updates affected matches", rewrite("Update all matches after adding variant member", "mace.match.non-exhaustive-after-domain-change", "|===|\nalias Value: variant[string, int, boolean];\nValue value = true;\nstring result = match (value) { string => 's', int => 'i', };\n|===|\n[output = 'data'] { result: result, }", "boolean =>")),
	)
})
