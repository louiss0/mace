package code_actions_test

import (
	"github.com/louiss0/mace/internal/analyzer"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

const fixAllKind = "source.fixAll.mace"

var _ = Describe("High-value fix-all code actions", func() {
	It("replaces malformed delimiters instead of appending a second document", func() {
		source := "|====|\nalias A: string;\n|===|\n[output = 'schema'] { A: A, }"
		fixture := newCodeActionFixture(source, nil)
		targetRange := protocol.Range{End: protocol.Position{Line: 5}}
		action, found := findCodeActionByTitle(analyzer.CodeActions(fixture.snapshot, fixture.uri, targetRange), "Match all script delimiters")

		assert.New(GinkgoT()).True(found)
		assert.New(GinkgoT()).Equal("|===|\nalias A: string;\n|===|\n[output = 'schema'] { A: A, }", applyDocumentEdits(source, fixture.uri, action))
	})

	DescribeTable("only combines deterministic nonconflicting edits",
		testCodeActionContract,
		Entry("fixes commas", sourceAction("Fix all missing field commas", fixAllKind, "[output = 'data'] { first: 1 second: 2 third: 3, }", "first: 1,", "second: 2,")),
		Entry("fixes semicolons", sourceAction("Fix all missing declaration semicolons", fixAllKind, "|===|\nalias A: string\nalias B: int\n|===|\n[output = 'schema'] { A: A, B: B, }", "alias A: string;", "alias B: int;")),
		Entry("matches delimiters", sourceAction("Match all script delimiters", fixAllKind, "|====|\nalias A: string;\n|===|\n[output = 'schema'] { A: A, }", "|===|")),
		Entry("removes parentheses", sourceAction("Remove all redundant parentheses", fixAllKind, "[output = 'data'] { first: (1), second: (2), }", "first: 1", "second: 2")),
		Entry("moves imports", sourceAction("Move all imports to the top", fixAllKind, "|===|\nstring x = 'x'; from './a.mace' import A; from './b.mace' import B;\n|===|\n[output = 'data'] { a: A, b: B, }", "from './a.mace' import A;\nfrom './b.mace' import B;")),
		Entry("removes duplicate imports", sourceAction("Remove duplicate imports", fixAllKind, "|===|\nfrom './a.mace' import A, A; from './a.mace' import A;\n|===|\n[output = 'data'] { a: A, }", "from './a.mace' import A;")),
		Entry("normalizes paths", sourceAction("Normalize all Mace file paths", fixAllKind, "|===|\nfrom '.\\a.mace' import A; from '.\\b.mace' import B;\n|===|\n[output = 'data'] { a: A, b: B, }", "'./a.mace'", "'./b.mace'")),
		Entry("adds extensions", sourceAction("Add `.mace` to valid file paths", fixAllKind, "|===|\nfrom './a' import A; from './b' import B;\n|===|\n[output = 'data'] { a: A, b: B, }", "'./a.mace'", "'./b.mace'")),
		Entry("deduplicates choices", sourceAction("Remove duplicate choice members", fixAllKind, "|===|\nalias A: choice['a', 'a']; alias B: choice[1, 1];\n|===|\n[output = 'schema'] { A: A, B: B, }", "choice['a']", "choice[1]")),
		Entry("deduplicates variants", sourceAction("Remove equivalent variant members", fixAllKind, "|===|\nalias A: variant[string, string]; alias B: variant[int, int];\n|===|\n[output = 'schema'] { A: A, B: B, }", "variant[string]", "variant[int]")),
		Entry("flattens variants", sourceAction("Flatten nested variants", fixAllKind, "|===|\nalias A: variant[string, variant[int, boolean]];\n|===|\n[output = 'schema'] { A: A, }", "variant[string, int, boolean]")),
		Entry("flattens fusions", sourceAction("Flatten nested fusions", fixAllKind, "|===|\nalias A: choice['a']; alias B: choice['b']; alias C: choice['c']; alias ABC: fusion[A, fusion[B, C]];\n|===|\n[output = 'schema'] { ABC: ABC, }", "fusion[A, B, C]")),
		Entry("fixes optional access", sourceAction("Replace invalid optional accesses with `?.`", fixAllKind, "[output = 'data'] { city: user.address.city, zip: user.address.zip, }", "address?.city", "address?.zip")),
		Entry("adds fallbacks", sourceAction("Add obvious `??` fallbacks", fixAllKind, "[output = 'data'] { city: user?.city, count: metrics?.count, }", "user?.city ?? ''", "metrics?.count ?? 0")),
		Entry("fixes match commas", sourceAction("Add missing match-arm commas", fixAllKind, "|===|\nchoice['a', 'b'] v = 'a'; int r = match (v) { 'a' => 1 'b' => 2 };\n|===|\n[output = 'data'] { r: r, }", "'a' => 1,", "'b' => 2,")),
		Entry("removes duplicate arms", sourceAction("Remove duplicate match arms", fixAllKind, "|===|\nvariant[string, int] v = 1; string r = match (v) { string => 'a', string => 'b', int => 'i', };\n|===|\n[output = 'data'] { r: r, }", "string => 'a'", "int => 'i'")),
		Entry("removes data optionals", sourceAction("Remove invalid data-field optional markers", fixAllKind, "[output = 'data'] { first?: 1, second?: 2, }", "first: 1", "second: 2")),
		Entry("canonicalizes hex", sourceAction("Canonicalize hexadecimal floats", fixAllKind, "[output = 'data'] { first: 0X01.A0, second: 0x02.B0, }", "0x1.a", "0x2.b")),
		Entry("synchronizes docs", sourceAction("Synchronize documentation field names", fixAllKind, "|===|\nschema User: { name: string, age: int, }; schema_doc User { fields: { nmae: 'Name', ag: 'Age', }, };\n|===|\n[output = 'schema'] { User: User, }", "name: 'Name'", "age: 'Age'")),
		Entry("removes schema directives", sourceAction("Remove forbidden directives from schema output", fixAllKind, "[output = 'schema', schema = User, parse = Runtime] { User: { name: string, }, }", "[output = 'schema']")),
	)
})
