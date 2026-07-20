package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Output directives and file-loaded schemas code actions", func() {
	It("adds a data output directive to an empty directive list", func() {
		source := `[]
{
  name: 'Mace',
}`
		expected := `[output = 'data']
{
  name: 'Mace',
}`

		newCodeActionFixture(source, nil).requireQuickFix(expectedQuickFix{
			diagnosticCode: "mace.directive.missing-output",
			title:          "Add `output = 'data'` directive",
			result:         expected,
		})
	})

	It("removes the later duplicate output directive", func() {
		source := `[output = 'data', output = 'data']
{
  name: 'Mace',
}`
		expected := `[output = 'data']
{
  name: 'Mace',
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.directive.duplicate-key",
			title:          "Remove duplicate output directive",
			result:         expected,
		})
	})

	It("removes every data-only directive from schema output", func() {
		source := `[output = 'schema', schema = User, schema_file = './schema.mace', parse = Runtime, parse_file = './runtime.mace']
{
  User: { name: string, },
}`
		expected := `[output = 'schema']
{
  User: { name: string, },
}`
		files := map[string]string{
			"schema.mace":  "[output = 'schema']\n{\n  User: { name: string, },\n}",
			"runtime.mace": "[output = 'schema']\n{\n  Runtime: { env: string, },\n}",
		}

		newCodeActionFixture(source, files).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.directive.data-only-in-schema-output",
			title:          "Remove data-only directives from schema output",
			result:         expected,
		})
	})
})
