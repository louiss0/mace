package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Fusion code actions", func() {
	DescribeTable("makes fusion domain and conflict changes explicit",
		testCodeActionContract,
		Entry("removes an invalid member", quickFix("Remove invalid fusion member", "mace.type.invalid-fusion-member", "|===|\nschema User: { name: string, }; alias Combined: fusion[User, int];\n|===|\n[output = 'schema'] { Combined: Combined, }", "fusion[User]")),
		Entry("replaces a mixed fusion", rewrite("Replace fusion with variant", "mace.type.fusion-domain-mismatch", "|===|\nschema User: { name: string, }; alias Mode: choice['dev']; alias Combined: fusion[User, Mode];\n|===|\n[output = 'schema'] { Combined: Combined, }", "variant[User, Mode]")),
		Entry("splits mixed domains", extract("Split mixed fusion into two aliases", "mace.type.fusion-domain-mismatch", "|===|\nschema User: { name: string, }; alias Mode: choice['dev']; alias Combined: fusion[User, Mode];\n|===|\n[output = 'schema'] { Combined: Combined, }", "alias CombinedRecord", "alias CombinedChoice")),
		Entry("uses a common field type", rewrite("Change conflicting field type to common type", "mace.type.fusion-field-conflict", "|===|\nschema A: { id: int, }; schema B: { id: float, }; alias AB: fusion[A, B];\n|===|\n[output = 'schema'] { AB: AB, }", "id: float")),
		Entry("expands both conflicting fields", rewrite("Expand conflicting fields to the same variant", "mace.type.fusion-field-conflict", "|===|\nschema A: { id: int, }; schema B: { id: string, }; alias AB: fusion[A, B];\n|===|\n[output = 'schema'] { AB: AB, }", "id: variant[int, string]")),
		Entry("renames a conflicting field", rewrite("Rename conflicting field in one member", "mace.type.fusion-field-conflict", "|===|\nschema A: { id: int, }; schema B: { id: string, }; alias AB: fusion[A, B];\n|===|\n[output = 'schema'] { AB: AB, }", "external_id: string")),
		Entry("removes a conflicting member", quickFix("Remove conflicting fusion member", "mace.type.fusion-field-conflict", "|===|\nschema A: { id: int, }; schema B: { id: string, }; alias AB: fusion[A, B];\n|===|\n[output = 'schema'] { AB: AB, }", "fusion[A]")),
		Entry("requires an optional fusion field", quickFix("Mark optional fusion field required", "mace.type.fusion-optionality-conflict", "|===|\nschema A: { id: int, }; schema B: { id?: int, }; alias AB: fusion[A, B];\n|===|\n[output = 'schema'] { AB: AB, }", "id: int")),
		Entry("deduplicates choice domain", sourceAction("Deduplicate fused choice domain", "source.fixAll.mace", "|===|\nalias A: choice['a']; alias B: choice['a', 'b']; alias AB: fusion[A, B];\n|===|\n[output = 'schema'] { AB: AB, }", "choice['a', 'b']")),
		Entry("flattens record fusion", preferredQuickFix("Flatten nested record fusion", "mace.type.nested-fusion", "|===|\nschema A: { a: string, }; schema B: { b: string, }; schema C: { c: string, }; alias ABC: fusion[A, fusion[B, C]];\n|===|\n[output = 'schema'] { ABC: ABC, }", "fusion[A, B, C]")),
		Entry("flattens choice fusion", preferredQuickFix("Flatten nested choice fusion", "mace.type.nested-fusion", "|===|\nalias A: choice['a']; alias B: choice['b']; alias C: choice['c']; alias ABC: fusion[A, fusion[B, C]];\n|===|\n[output = 'schema'] { ABC: ABC, }", "fusion[A, B, C]")),
		Entry("extracts repeated records", extract("Convert repeated inline record to schema", "mace.refactor.repeated-fusion-record", "|===|\nalias Combined: fusion[{ id: int, name: string, }, { id: int, active: boolean, }];\n|===|\n[output = 'schema'] { Combined: Combined, }", "schema Base", "fusion[Base")),
	)
})
