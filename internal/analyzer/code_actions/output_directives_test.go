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

	DescribeTable("satisfies every remaining output-directive contract",
		testCodeActionContract,
		Entry("chooses schema output", quickFix("Add `output = 'schema'` directive", "mace.directive.missing-output", "[] { User: { name: string, }, }", "[output = 'schema']")),
		Entry("removes an unknown directive", preferredQuickFix("Remove unknown directive", "mace.directive.unknown-key", "[output = 'data', scheam = User] { name: 'Mace', }", "[output = 'data']")),
		Entry("renames a directive", quickFix("Rename directive to nearest known directive", "mace.directive.unknown-key", "[outpt = 'data'] { name: 'Mace', }", "output = 'data'")),
		Entry("switches to data", rewrite("Switch output to data mode", "mace.directive.data-only-in-schema-output", "[output = 'schema', schema = User] { name: 'Mace', }", "output = 'data'")),
		Entry("switches to schema", rewrite("Switch output to schema mode", "mace.directive.schema-shaped-data-output", "[output = 'data'] { name: string, age: int, }", "output = 'schema'")),
		Entry("selects a local schema", quickFix("Select matching local schema", "mace.directive.unknown-schema-name", "|===|\nschema User: { name: string, };\n|===|\n[output = 'data', schema = Usre] { name: 'Mace', }", "schema = User")),
		Entry("adds external parse file", withWorkspace(quickFix("Add `parse_file` for parse schema", "mace.directive.parse-file-required", "[output = 'data', parse = Runtime] { value: $env, }", "parse_file = './runtime.mace'"), map[string]string{"runtime.mace": "[output = 'schema'] { Runtime: { env: string, }, }"}, nil)),
		Entry("selects parse schema", quickFix("Select parse schema from loaded file", "mace.directive.unknown-parse-name", "[output = 'data', parse_file = './runtime.mace', parse = Runtim] { value: $env, }", "parse = Runtime")),
		Entry("removes incompatible parse", quickFix("Remove incompatible parse directive", "mace.directive.incompatible-parse", "[output = 'data', parse = Runtime, parse_file = './runtime.mace'] {}", "parse_file = './runtime.mace'")),
		Entry("generates output schema", extract("Generate output schema from data fields", "mace.type.untyped-output-collection", "[output = 'data'] { names: [], }", "schema Output", "names: array<string>")),
		Entry("attaches generated schema", quickFix("Attach generated schema to output", "mace.directive.generated-schema-not-selected", "|===|\nschema Output: { name: string, };\n|===|\n[output = 'data'] { name: 'Mace', }", "schema = Output")),
		Entry("creates a schema file", extract("Create schema file from output shape", "mace.type.output-needs-reusable-schema", "[output = 'data'] { name: 'Mace', }", "schema_file = './output.schema.mace'")),
	)
})
