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

	DescribeTable("satisfies every remaining declaration and symbol contract",
		testCodeActionContract,
		Entry("inserts a string initializer", quickFix("Insert initializer", "mace.declaration.variable-missing-initializer", "|===|\nstring name;\n|===|\n[output = 'data'] { name: name, }", "string name = '';")),
		Entry("inserts an explicit type", quickFix("Insert explicit variable type", "mace.declaration.variable-missing-type", "|===|\nname = 'Mace';\n|===|\n[output = 'data'] { name: name, }", "string name")),
		Entry("changes the receiving type", quickFix("Change declaration type to inferred value type", "mace.type.initializer-type-mismatch", "|===|\nint name = 'Mace';\n|===|\n[output = 'data'] { name: name, }", "string name")),
		Entry("converts an initializer", preferredQuickFix("Convert initializer to declared type family", "mace.type.initializer-type-mismatch", "|===|\nfloat count = 1;\n|===|\n[output = 'data'] { count: count, }", "1.0")),
		Entry("renames a duplicate", rewrite("Rename duplicate declaration", "mace.declaration.duplicate-variable", "|===|\nstring name = 'first';\nstring name = 'second';\n|===|\n[output = 'data'] { name: name, }", "name_2")),
		Entry("removes an equivalent duplicate", quickFix("Remove duplicate declaration", "mace.declaration.duplicate-variable", "|===|\nstring name = 'Mace';\nstring name = 'Mace';\n|===|\n[output = 'data'] { name: name, }", "string name = 'Mace';")),
		Entry("replaces an unknown type", quickFix("Replace unknown type with nearest type", "mace.declaration.unknown-type-reference", "|===|\nalias Username: string;\nUsernme name = 'Mace';\n|===|\n[output = 'data'] { name: name, }", "Username name")),
		Entry("creates a variable", extract("Create variable ‘name’", "mace.type.unknown-identifier", "[output = 'data'] { value: name, }", "|===|", "string name = '';")),
		Entry("creates a type alias", extract("Create type alias ‘Name’", "mace.declaration.unknown-type-reference", "|===|\nName name = 'Mace';\n|===|\n[output = 'data'] { name: name, }", "alias Name: string;")),
		Entry("creates a schema", extract("Create schema ‘Name’ from record literal", "mace.declaration.unknown-type-reference", "|===|\nName name = { value: 'Mace', };\n|===|\n[output = 'data'] { name: name, }", "schema Name", "value: string")),
		Entry("prefixes parsed input", preferredQuickFix("Prefix parsed input reference with `$`", "mace.type.parsed-input-missing-prefix", "|===|\nschema Runtime: { env: string, };\n|===|\n[output = 'data', parse = Runtime] { env: env, }", "$env")),
		Entry("removes a local prefix", preferredQuickFix("Remove `$` from local variable reference", "mace.type.local-variable-input-prefix", "|===|\nstring env = 'dev';\n|===|\n[output = 'data'] { env: $env, }", "env: env")),
		Entry("imports an unresolved type", withWorkspace(quickFix("Import unresolved type", "mace.declaration.unknown-type-reference", "|===|\nUser user = { name: 'Mace', };\n|===|\n[output = 'data'] { user: user, }", "from './types.mace' import User;"), map[string]string{"types.mace": "[output = 'schema'] { User: { name: string, }, }"}, nil)),
		Entry("imports an unresolved value", withWorkspace(quickFix("Import unresolved value", "mace.type.unknown-identifier", "[output = 'data'] { value: Shared, }", "from './shared.mace' import Shared;"), map[string]string{"shared.mace": "[output = 'data'] { Shared: 1, }"}, nil)),
		Entry("inlines an alias", rewrite("Inline type alias", "mace.type.alias-cycle", "|===|\nalias Name: string;\nName name = 'Mace';\n|===|\n[output = 'data'] { name: name, }", "string name")),
		Entry("extracts an inline type", extract("Extract inline type into alias", "mace.refactor.repeated-inline-type", "|===|\n{ name: string, age: int, } user = { name: 'Mace', age: 1, };\n|===|\n[output = 'data'] { user: user, }", "alias User", "User user")),
	)
})
