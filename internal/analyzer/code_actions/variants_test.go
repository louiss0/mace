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
})
