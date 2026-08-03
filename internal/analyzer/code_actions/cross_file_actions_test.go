package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Cross-file code actions", func() {
	sharedData := "[output = 'data'] { OldName: 1, old_field: 'value', }"
	sharedSchema := "|===|\nschema User: { old_field: string, };\nalias Value: variant[string, int];\nalias Mode: choice['dev', 'prod'];\n|===|\n[output = 'schema'] { User: User, Value: Value, Mode: Mode, }"

	DescribeTable("propagates renames through every semantic relationship",
		testCodeActionContract,
		Entry("renames declarations and imports", withWorkspace(rewrite("Rename declaration and update imports", "mace.workspace.rename", "|===|\nfrom './shared.mace' import OldName;\n|===|\n[output = 'data'] { value: OldName, }", "NewName"), map[string]string{"shared.mace": sharedData}, map[string][]string{"shared.mace": {"NewName"}})),
		Entry("renames exported fields", withWorkspace(rewrite("Rename exported output field and update importers", "mace.workspace.rename", "|===|\nfrom './shared.mace' import OldName;\n|===|\n[output = 'data'] { value: OldName, }", "NewName"), map[string]string{"shared.mace": sharedData}, map[string][]string{"shared.mace": {"NewName:"}})),
		Entry("renames schema directives", rewrite("Rename schema and update `schema` directives", "mace.workspace.rename", "|===|\nschema OldUser: { name: string, };\n|===|\n[output = 'data', schema = OldUser] { name: 'Mace', }", "schema User", "schema = User")),
		Entry("renames parse directives", rewrite("Rename parse schema and update `parse` directives", "mace.workspace.rename", "|===|\nschema OldRuntime: { env: string, };\n|===|\n[output = 'data', parse = OldRuntime] { env: $env, }", "schema Runtime", "parse = Runtime")),
		Entry("renames schema output fields", rewrite("Rename schema field and update data outputs", "mace.workspace.rename", "|===|\nschema User: { old_field: string, }; User user = { old_field: 'value', };\n|===|\n[output = 'data'] { value: user.old_field, }", "new_field")),
		Entry("renames self paths", rewrite("Rename schema field and update `$self` paths", "mace.workspace.rename", "[output = 'data'] { old_field: 'value', copied: $self.old_field, }", "new_field", "$self.new_field")),
		Entry("renames parsed paths", rewrite("Rename schema field and update parsed-input paths", "mace.workspace.rename", "|===|\nschema Runtime: { old_field: string, };\n|===|\n[output = 'data', parse = Runtime] { value: $old_field, }", "new_field", "$new_field")),
		Entry("renames documentation fields", rewrite("Rename schema field and update `schema_doc.fields`", "mace.workspace.rename", "|===|\nschema User: { old_field: string, }; schema_doc User { fields: { old_field: 'Field', }, };\n|===|\n[output = 'schema'] { User: User, }", "new_field")),
		Entry("renames domains and patterns", rewrite("Rename variant or choice alias and update match patterns", "mace.workspace.rename", "|===|\nalias OldValue: variant[string, int]; OldValue value = 1; string result = match (value) { string => 's', int => 'i', };\n|===|\n[output = 'data'] { result: result, }", "Value")),
	)

	DescribeTable("propagates type-domain changes atomically",
		testCodeActionContract,
		Entry("adds variant member", withWorkspace(rewrite("Add a variant member and update every exhaustive match", "mace.workspace.variant-domain-change", "|===|\nfrom './shared.mace' import Value; Value value = 1; string result = match (value) { string => 's', int => 'i', };\n|===|\n[output = 'data'] { result: result, }", "boolean =>"), map[string]string{"shared.mace": sharedSchema}, map[string][]string{"shared.mace": {"variant[string, int, boolean]"}})),
		Entry("removes variant member", withWorkspace(rewrite("Remove a variant member and remove unreachable match arms", "mace.workspace.variant-domain-change", "|===|\nfrom './shared.mace' import Value; Value value = 1; string result = match (value) { string => 's', int => 'i', };\n|===|\n[output = 'data'] { result: result, }", "int => 'i'"), map[string]string{"shared.mace": sharedSchema}, map[string][]string{"shared.mace": {"variant[string]"}})),
		Entry("adds choice member", withWorkspace(rewrite("Add a choice member and update every exhaustive choice match", "mace.workspace.choice-domain-change", "|===|\nfrom './shared.mace' import Mode; Mode mode = 'dev'; int result = match (mode) { 'dev' => 1, 'prod' => 2, };\n|===|\n[output = 'data'] { result: result, }", "'test' =>"), map[string]string{"shared.mace": sharedSchema}, map[string][]string{"shared.mace": {"choice['dev', 'prod', 'test']"}})),
		Entry("removes choice member", withWorkspace(rewrite("Remove a choice member and remove invalid match arms", "mace.workspace.choice-domain-change", "|===|\nfrom './shared.mace' import Mode; Mode mode = 'dev'; int result = match (mode) { 'dev' => 1, 'prod' => 2, };\n|===|\n[output = 'data'] { result: result, }", "'dev' => 1"), map[string]string{"shared.mace": sharedSchema}, map[string][]string{"shared.mace": {"choice['dev']"}})),
		Entry("changes field types", withWorkspace(rewrite("Change a schema field type and update incompatible outputs", "mace.workspace.schema-field-change", "|===|\nfrom './shared.mace' import User; User user = { old_field: '1', };\n|===|\n[output = 'data'] { user: user, }", "old_field: 1"), map[string]string{"shared.mace": sharedSchema}, map[string][]string{"shared.mace": {"old_field: int"}})),
		Entry("makes field optional", withWorkspace(rewrite("Mark a schema field optional and convert plain access to `?.`", "mace.workspace.schema-field-change", "|===|\nfrom './shared.mace' import User; User user = { old_field: 'value', };\n|===|\n[output = 'data'] { value: user.old_field, }", "user?.old_field"), map[string]string{"shared.mace": sharedSchema}, map[string][]string{"shared.mace": {"old_field?: string"}})),
		Entry("requires field", withWorkspace(rewrite("Make a schema field required and identify outputs missing it", "mace.workspace.schema-field-change", "|===|\nfrom './shared.mace' import User; User user = {};\n|===|\n[output = 'data'] { user: user, }", "old_field: ''"), map[string]string{"shared.mace": "[output = 'schema'] { User: { old_field?: string, }, }"}, map[string][]string{"shared.mace": {"old_field: string"}})),
		Entry("increases nesting", withWorkspace(rewrite("Increase record nesting and revalidate member paths", "mace.workspace.record-depth-change", "|===|\nfrom './shared.mace' import User; User user = { old_field: {}, };\n|===|\n[output = 'data'] { value: user.old_field.first.second, }", "old_field.first.second"), map[string]string{"shared.mace": sharedSchema}, map[string][]string{"shared.mace": {"record<record<string>>"}})),
		Entry("revalidates importers", withWorkspace(rewrite("Change an exported symbol and revalidate all importers", "mace.workspace.export-change", "|===|\nfrom './shared.mace' import OldName;\n|===|\n[output = 'data'] { value: OldName, }", "value: 0"), map[string]string{"shared.mace": sharedData}, map[string][]string{"shared.mace": {"OldName: 0"}})),
	)

	DescribeTable("refactors file relationships atomically",
		testCodeActionContract,
		Entry("moves declarations", withWorkspace(extract("Move declaration to another Mace file and add imports", "mace.workspace.move-declaration", "|===|\nschema User: { name: string, }; User user = { name: 'Mace', };\n|===|\n[output = 'data'] { user: user, }", "from './types.mace' import User;"), map[string]string{"types.mace": "[output = 'schema'] {}"}, map[string][]string{"types.mace": {"schema User"}})),
		Entry("breaks cycles", withWorkspace(extract("Extract declarations into a shared file to break a cycle", "mace.import.circular", "|===|\nfrom './b.mace' import B; schema A: { b: B, };\n|===|\n[output = 'schema'] { A: A, }", "from './shared.mace' import"), map[string]string{"b.mace": "|===|\nfrom './document.mace' import A; schema B: { a: A, };\n|===|\n[output = 'schema'] { B: B, }", "shared.mace": "[output = 'schema'] {}"}, map[string][]string{"shared.mace": {"schema"}})),
		Entry("creates schema file", withWorkspace(extract("Create a schema file from an inline schema", "mace.workspace.extract-schema-file", "[output = 'schema'] { User: { name: string, }, }", "schema_file = './user.mace'"), map[string]string{"user.mace": "[output = 'schema'] {}"}, map[string][]string{"user.mace": {"User: { name: string, }"}})),
		Entry("exposes declaration", withWorkspace(rewrite("Expose a declaration from its owning file", "mace.import.name-not-exposed", "|===|\nfrom './shared.mace' import User;\n|===|\n[output = 'schema'] { User: User, }"), map[string]string{"shared.mace": "|===|\nschema User: { name: string, };\n|===|\n[output = 'schema'] {}"}, map[string][]string{"shared.mace": {"User: User"}})),
		Entry("updates moved files", withWorkspace(sourceAction("Update references after moving or renaming a Mace file", "source", "|===|\nfrom './old/shared.mace' import OldName;\n|===|\n[output = 'data'] { value: OldName, }", "from './new/shared.mace'"), map[string]string{"new/shared.mace": sharedData}, nil)),
	)
})
