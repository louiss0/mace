package code_actions_test

import (
	"github.com/louiss0/mace/internal/analyzer"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
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

	It("does not diagnose an untyped output field as an unknown schema field", func() {
		source := "[output = 'data'] { extra: 1, }"
		fixture := newCodeActionFixture(source, nil)

		_, found := findDiagnosticByCode(analyzer.Diagnostics(fixture.snapshot), "mace.type.record-does-not-match-schema")
		assert.New(GinkgoT()).False(found)
	})

	DescribeTable("satisfies every remaining closed-schema contract",
		testCodeActionContract,
		Entry("adds one required field", quickFix("Add missing required schema field", "mace.type.record-does-not-match-schema", "|===|\nschema User: { name: string, age: int, };\nUser user = { name: 'Mace', };\n|===|\n[output = 'data'] { user: user, }", "age: 0")),
		Entry("adds every required field", quickFix("Add all missing required fields", "mace.type.record-does-not-match-schema", "|===|\nschema User: { name: string, age: int, active: boolean, };\nUser user = {};\n|===|\n[output = 'data'] { user: user, }", "name: ''", "age: 0", "active: false")),
		Entry("removes an unknown field", preferredQuickFix("Remove unknown record field", "mace.type.record-does-not-match-schema", "|===|\nschema User: { name: string, };\nUser user = { name: 'Mace', extra: 1, };\n|===|\n[output = 'data'] { user: user, }", "name: 'Mace'")),
		Entry("renames a field", quickFix("Rename field to nearest schema field", "mace.type.record-does-not-match-schema", "|===|\nschema User: { name: string, };\nUser user = { nmae: 'Mace', };\n|===|\n[output = 'data'] { user: user, }", "name: 'Mace'")),
		Entry("extends a schema", rewrite("Add field to schema", "mace.type.record-does-not-match-schema", "|===|\nschema User: { name: string, };\nUser user = { name: 'Mace', age: 1, };\n|===|\n[output = 'data'] { user: user, }", "age: int")),
		Entry("changes a field value", quickFix("Change field value to expected type", "mace.type.record-field-mismatch", "|===|\nschema User: { age: int, };\nUser user = { age: '1', };\n|===|\n[output = 'data'] { user: user, }", "age: 1")),
		Entry("widens schema field type", rewrite("Change schema field type to actual type", "mace.type.record-field-mismatch", "|===|\nschema User: { age: int, };\nUser user = { age: 1.5, };\n|===|\n[output = 'data'] { user: user, }", "age: float")),
		Entry("makes a schema field variant", rewrite("Expand schema field to a variant", "mace.type.record-field-mismatch", "|===|\nschema User: { id: int, };\nUser first = { id: 1, }; User second = { id: 'two', };\n|===|\n[output = 'data'] { first: first, second: second, }", "id: variant[int, string]")),
		Entry("renames a duplicate schema field", rewrite("Rename duplicate schema field", "mace.declaration.duplicate-schema-field", "|===|\nschema User: { name: string, name: string, };\n|===|\n[output = 'schema'] { User: User, }", "name_2")),
		Entry("marks a field optional", rewrite("Mark schema field optional", "mace.type.repeated-missing-field", "|===|\nschema User: { nickname: string, };\n|===|\n[output = 'schema'] { User: User, }", "nickname?: string")),
		Entry("requires an optional field", rewrite("Make optional field required", "mace.type.fusion-optionality-conflict", "|===|\nschema User: { id?: int, };\n|===|\n[output = 'schema'] { User: User, }", "id: int")),
		Entry("types an empty record", preferredQuickFix("Add expected type for empty record", "mace.type.untyped-empty-record", "|===|\nuser = {};\n|===|\n[output = 'data'] { user: user, }", "User user = {}")),
		Entry("attaches schema for empty record", preferredQuickFix("Attach output schema for empty record", "mace.type.untyped-output-collection", "[output = 'data'] { user: {}, }", "schema = Output")),
		Entry("closes a broad record", rewrite("Convert record map to closed inline record", "mace.type.record-map-too-broad", "|===|\nrecord<string> user = { name: 'Mace', };\n|===|\n[output = 'data'] { user: user, }", "{ name: string, } user")),
		Entry("broadens a uniform record", rewrite("Convert uniform inline record to `record<T>`", "mace.refactor.uniform-inline-record", "|===|\n{ first: string, second: string, } values = { first: 'a', second: 'b', };\n|===|\n[output = 'data'] { values: values, }", "record<string> values")),
	)
})
