package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Choice code actions", func() {
	It("removes a duplicate literal from a choice domain", func() {
		source := `|===|
alias Environment: choice['dev', 'test', 'dev'];
Environment environment = 'dev';
|===|
[output = 'data']
{
  environment: environment,
}`
		expected := `|===|
alias Environment: choice['dev', 'test'];
Environment environment = 'dev';
|===|
[output = 'data']
{
  environment: environment,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.duplicate-choice-member",
			title:          "Remove duplicate choice member",
			result:         expected,
		})
	})

	DescribeTable("satisfies every remaining choice contract",
		testCodeActionContract,
		Entry("replaces an invalid member", quickFix("Replace invalid choice member with literal", "mace.type.invalid-choice-member", "|===|\nalias Environment: choice[string, 'prod'];\n|===|\n[output = 'schema'] { Environment: Environment, }", "choice['dev', 'prod']")),
		Entry("converts numeric family", quickFix("Convert literal to choice’s numeric family", "mace.type.choice-numeric-family", "|===|\nalias Code: choice[0x1, 2];\n|===|\n[output = 'schema'] { Code: Code, }", "0x2")),
		Entry("selects an allowed member", quickFix("Replace value with allowed choice member", "mace.type.value-outside-choice", "|===|\nalias Environment: choice['dev', 'prod']; Environment env = 'staging';\n|===|\n[output = 'data'] { env: env, }", "'dev'")),
		Entry("adds a literal", rewrite("Add literal to choice declaration", "mace.type.value-outside-choice", "|===|\nalias Environment: choice['dev', 'prod']; Environment env = 'staging';\n|===|\n[output = 'data'] { env: env, }", "choice['dev', 'prod', 'staging']")),
		Entry("replaces copied composition", rewrite("Replace duplicate choice composition with fusion", "mace.refactor.copied-choice-composition", "|===|\nalias A: choice['a']; alias B: choice['b']; alias AB: choice['a', 'b'];\n|===|\n[output = 'schema'] { AB: AB, }", "fusion[A, B]")),
		Entry("fuses aliases", rewrite("Fuse choice aliases", "mace.refactor.choice-composition", "|===|\nalias A: choice['a']; alias B: choice['b'];\n|===|\n[output = 'schema'] { A: A, B: B, }", "alias Combined: fusion[A, B]")),
		Entry("removes a cycle edge", quickFix("Remove cyclic choice alias edge", "mace.type.choice-alias-cycle", "|===|\nalias A: choice['a', B]; alias B: choice['b', A];\n|===|\n[output = 'schema'] { A: A, }", "alias B: choice['b']")),
		Entry("inlines to break a cycle", rewrite("Inline choice alias to break cycle", "mace.type.choice-alias-cycle", "|===|\nalias A: choice['a', B]; alias B: choice['b', A];\n|===|\n[output = 'schema'] { A: A, }", "choice['a', 'b']")),
		Entry("updates affected matches", rewrite("Update choice matches after domain change", "mace.match.non-exhaustive-after-domain-change", "|===|\nalias Environment: choice['dev', 'prod', 'test']; Environment env = 'dev'; int result = match (env) { 'dev' => 1, 'prod' => 2, };\n|===|\n[output = 'data'] { result: result, }", "'test' =>")),
	)
})
