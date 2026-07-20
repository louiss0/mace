package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Record maps, record literals, and closed schemas code actions", func() {
	It("removes the later duplicate runtime record field", func() {
		source := `[output = 'data']
{
  name: 'first',
  name: 'second',
}`
		expected := `[output = 'data']
{
  name: 'first',
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.declaration.duplicate-output-field",
			title:          "Remove duplicate record field",
			result:         expected,
		})
	})

	It("removes the later duplicate schema field", func() {
		source := `|===|
schema User: {
  name: string,
  name: string,
};
|===|
[output = 'schema']
{
  User: User,
}`
		expected := `|===|
schema User: {
  name: string,
};
|===|
[output = 'schema']
{
  User: User,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.declaration.duplicate-schema-field",
			title:          "Remove duplicate schema field",
			result:         expected,
		})
	})

	It("removes an optional marker from a data output field", func() {
		source := `[output = 'data']
{
  name?: 'Mace',
}`
		expected := `[output = 'data']
{
  name: 'Mace',
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.data-field-optional-marker",
			title:          "Remove optional marker from data field",
			result:         expected,
		})
	})
})
