package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Declarations, symbols, and explicit typing code actions", func() {
	It("replaces an unknown value with the nearest compatible symbol", func() {
		source := `|===|
string greeting = 'hello';
|===|
[output = 'data']
{
  message: greting,
}`
		expected := `|===|
string greeting = 'hello';
|===|
[output = 'data']
{
  message: greeting,
}`

		newCodeActionFixture(source, nil).requireQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.unknown-identifier",
			title:          "Replace unknown value with ‘greeting’",
			result:         expected,
		})
	})
})
