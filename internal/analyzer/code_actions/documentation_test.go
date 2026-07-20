package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Structured and inline documentation code actions", func() {
	DescribeTable("synchronizes documentation with its semantic target",
		testCodeActionContract,
		Entry("moves documentation", preferredQuickFix("Move documentation after target declaration", "mace.documentation.before-target", "|===|\ngen_doc Name { summary: 'Name', };\nalias Name: string;\n|===|\n[output = 'schema'] { Name: Name, }", "alias Name: string;\ngen_doc Name")),
		Entry("uses schema docs", preferredQuickFix("Change `gen_doc` to `schema_doc`", "mace.documentation.target-mismatch", "|===|\nschema User: { name: string, }; gen_doc User { summary: 'User', };\n|===|\n[output = 'schema'] { User: User, }", "schema_doc User")),
		Entry("uses general docs", preferredQuickFix("Change `schema_doc` to `gen_doc`", "mace.documentation.target-mismatch", "|===|\nalias Name: string; schema_doc Name { summary: 'Name', };\n|===|\n[output = 'schema'] { Name: Name, }", "gen_doc Name")),
		Entry("removes duplicate key", preferredQuickFix("Remove duplicate documentation key", "mace.documentation.duplicate-key", "|===|\nalias Name: string; gen_doc Name { summary: 'Name', summary: 'Other', };\n|===|\n[output = 'schema'] { Name: Name, }", "summary: 'Name'")),
		Entry("renames unknown key", quickFix("Rename unknown documentation key", "mace.documentation.unknown-key", "|===|\nalias Name: string; gen_doc Name { summry: 'Name', };\n|===|\n[output = 'schema'] { Name: Name, }", "summary: 'Name'")),
		Entry("moves fields", rewrite("Move `fields` into `schema_doc`", "mace.documentation.fields-outside-schema-doc", "|===|\nschema User: { name: string, }; gen_doc User { fields: { name: 'Name', }, };\n|===|\n[output = 'schema'] { User: User, }", "schema_doc User")),
		Entry("removes invalid fields", preferredQuickFix("Remove invalid `fields` entry", "mace.documentation.invalid-fields", "|===|\nalias Name: string; gen_doc Name { fields: { value: 'Value', }, };\n|===|\n[output = 'schema'] { Name: Name, }", "gen_doc Name")),
		Entry("renames documented field", quickFix("Rename documented field to schema field", "mace.documentation.unknown-field", "|===|\nschema User: { name: string, }; schema_doc User { fields: { nmae: 'Name', }, };\n|===|\n[output = 'schema'] { User: User, }", "name: 'Name'")),
		Entry("removes stale field docs", quickFix("Remove documentation for nonexistent field", "mace.documentation.unknown-field", "|===|\nschema User: { name: string, }; schema_doc User { fields: { old: 'Old', }, };\n|===|\n[output = 'schema'] { User: User, }", "schema_doc User")),
		Entry("adds documented schema field", rewrite("Add documented field to schema", "mace.documentation.unknown-field", "|===|\nschema User: { name: string, }; schema_doc User { fields: { age: 'Age', }, };\n|===|\n[output = 'schema'] { User: User, }", "age: int")),
		Entry("removes inline conflict", preferredQuickFix("Remove conflicting inline description", "mace.documentation.conflict", "|===|\nschema User: { name: string /# Inline, }; schema_doc User { fields: { name: 'Structured', }, };\n|===|\n[output = 'schema'] { User: User, }", "name: string")),
		Entry("moves inline docs", rewrite("Move inline description into structured documentation", "mace.documentation.conflict", "|===|\nschema User: { name: string /# Name, };\n|===|\n[output = 'schema'] { User: User, }", "schema_doc User", "name: 'Name'")),
		Entry("moves summary inline", rewrite("Move structured summary into inline description", "mace.documentation.can-inline", "|===|\nalias Name: string; gen_doc Name { summary: 'A name', };\n|===|\n[output = 'schema'] { Name: Name, }", "/# A name")),
		Entry("escapes interpolation", quickFix("Escape interpolation marker in documentation", "mace.documentation.interpolation-forbidden", "output_doc { summary: \"Value $(name)\", };\n[output = 'data'] {}", "Value \\$(name)")),
		Entry("adds output directives", preferredQuickFix("Add output directive list", "mace.documentation.output-directives-missing", "output_doc { summary: 'Output', };\n{ value: 1, }", "[output = 'data']")),
		Entry("synchronizes field names", sourceAction("Synchronize documentation fields with schema", "source.fixAll.mace", "|===|\nschema User: { name: string, age: int, }; schema_doc User { fields: { nmae: 'Name', old: 'Old', }, };\n|===|\n[output = 'schema'] { User: User, }", "name: 'Name'", "age:")),
	)
})
