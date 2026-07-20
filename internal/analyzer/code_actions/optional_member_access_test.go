package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Optional member access and record depth code actions", func() {
	It("replaces plain access after an optional field with optional access", func() {
		source := `|===|
schema Address: { city: string, };
schema User: { address?: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address.city;
|===|
[output = 'data']
{
  city: city,
}`
		expected := `|===|
schema Address: { city: string, };
schema User: { address?: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address?.city;
|===|
[output = 'data']
{
  city: city,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.optional-field-access",
			title:          "Replace `.` with `?.`",
			result:         expected,
		})
	})

	It("adds a fallback derived from the final optional member type", func() {
		source := `|===|
schema Address: { city?: string, };
schema User: { address: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address?.city;
|===|
[output = 'data']
{
  city: city,
}`
		expected := `|===|
schema Address: { city?: string, };
schema User: { address: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address?.city ?? '';
|===|
[output = 'data']
{
  city: city,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.absent-value-not-coalesced",
			title:          "Add `??` fallback",
			result:         expected,
		})
	})
})
