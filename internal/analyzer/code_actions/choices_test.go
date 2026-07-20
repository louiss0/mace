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
})
