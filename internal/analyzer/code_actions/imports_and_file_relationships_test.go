package code_actions_test

import (
	"github.com/louiss0/mace/internal/analyzer"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var _ = Describe("Imports and relationships between files code actions", func() {
	It("moves a misplaced import to the top of the script block", func() {
		source := `|===|
string local = 'local';
from './shared.mace' import Shared;
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		expected := `|===|
from './shared.mace' import Shared;
string local = 'local';
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		files := map[string]string{
			"shared.mace": `[output = 'data']
{
  Shared: 'shared',
}`,
		}

		newCodeActionFixture(source, files).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.import.not-at-top",
			title:          "Move import to top of script block",
			result:         expected,
		})
	})

	It("appends the Mace extension to a resolvable import path", func() {
		source := `|===|
from './shared' import Shared;
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		expected := `|===|
from './shared.mace' import Shared;
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		files := map[string]string{
			"shared.mace": `[output = 'data']
{
  Shared: 'shared',
}`,
		}

		newCodeActionFixture(source, files).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.import.path-not-mace",
			title:          "Append `.mace` to import path",
			result:         expected,
		})
	})

	It("does not diagnose a resolved local identifier as an import error", func() {
		fixture := newCodeActionFixture(`|===|
int Symbol = 1;
|===|
[output = 'data'] { value: Symbol, }`, nil)

		_, found := findDiagnosticByCode(analyzer.Diagnostics(fixture.snapshot), "mace.type.unknown-identifier")
		assert.New(GinkgoT()).False(found)
	})

	It("does not diagnose an exposed imported identifier as unavailable", func() {
		fixture := newCodeActionFixture(`|===|
from './shared.mace' import Symbol;
|===|
[output = 'data'] { value: Symbol, }`, map[string]string{
			"shared.mace": "[output = 'data'] { Symbol: 1, }",
		})

		_, found := findDiagnosticByCode(analyzer.Diagnostics(fixture.snapshot), "mace.import.name-not-exposed")
		assert.New(GinkgoT()).False(found)
	})

	It("inserts a generated import before the output block", func() {
		fixture := newCodeActionFixture("[output = 'data'] { value: Symbol, }", map[string]string{
			"shared.mace": "[output = 'data'] { Symbol: 1, }",
		})
		fixture.requireQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.unknown-identifier",
			title:          "Add import for ‘Symbol’",
			result:         "|===|\nfrom './shared.mace' import Symbol;\n|===|\n[output = 'data'] { value: Symbol, }",
		})
	})

	DescribeTable("satisfies every remaining import contract",
		testCodeActionContract,
		Entry("moves all imports", codeActionContract{title: "Move all imports to top", diagnosticCode: "mace.import.not-at-top", kind: "source.organizeImports", source: "|===|\nstring a = 'a';\nfrom './a.mace' import A;\nstring b = 'b';\nfrom './b.mace' import B;\n|===|\n[output = 'data'] { a: A, b: B, }", expectedText: []string{"from './a.mace' import A;\nfrom './b.mace' import B;"}}),
		Entry("adds an import", withWorkspace(quickFix("Add import for ‘Symbol’", "mace.type.unknown-identifier", "[output = 'data'] { value: Symbol, }", "from './shared.mace' import Symbol;"), map[string]string{"shared.mace": "[output = 'data'] { Symbol: 1, }"}, nil)),
		Entry("imports with an alias", withWorkspace(quickFix("Import exported symbol as alias", "mace.import.local-name-conflict", "|===|\nint Symbol = 1;\n|===|\n[output = 'data'] { value: RemoteSymbol, }", "from './shared.mace' import Symbol: RemoteSymbol;"), map[string]string{"shared.mace": "[output = 'data'] { Symbol: 2, }"}, nil)),
		Entry("renames an imported symbol locally", quickFix("Rename imported symbol locally", "mace.import.duplicate-local-name", "|===|\nfrom './one.mace' import Name;\nfrom './two.mace' import Name;\n|===|\n[output = 'data'] {}", "Name: TwoName")),
		Entry("renames a shadowing declaration", rewrite("Rename local declaration", "mace.declaration.shadows-import", "|===|\nfrom './shared.mace' import Name;\nstring Name = 'local';\n|===|\n[output = 'data'] { value: Name, }", "LocalName")),
		Entry("expands a wildcard", preferredQuickFix("Replace wildcard import with named imports", "mace.import.wildcard", "|===|\nfrom './shared.mace' import *;\n|===|\n[output = 'data'] { value: Used, }", "import Used;")),
		Entry("removes unused wildcard names", sourceAction("Remove unused name from replacement import list", "source.organizeImports", "|===|\nfrom './shared.mace' import Used, Unused;\n|===|\n[output = 'data'] { value: Used, }", "import Used;")),
		Entry("chooses a nearby file", quickFix("Replace path with nearest matching Mace file", "mace.import.file-not-found", "|===|\nfrom './shraed.mace' import Name;\n|===|\n[output = 'data'] { value: Name, }", "'./shared.mace'")),
		Entry("makes a path relative", preferredQuickFix("Rewrite import path relative to current file", "mace.import.incorrect-relative-path", "|===|\nfrom './shared.mace' import Name;\n|===|\n[output = 'data'] { value: Name, }", "'../shared.mace'")),
		Entry("normalizes path separators", quickFix("Normalize import path separators", "mace.import.noncanonical-path", "|===|\nfrom '.\\types\\shared.mace' import Name;\n|===|\n[output = 'data'] { value: Name, }", "'./types/shared.mace'")),
		Entry("replaces an escaping path", quickFix("Replace escaping path with project-local file", "mace.import.path-outside-root", "|===|\nfrom '../../shared.mace' import Name;\n|===|\n[output = 'data'] { value: Name, }", "'./shared.mace'")),
		Entry("removes a duplicate imported name", preferredQuickFix("Remove duplicate imported name", "mace.import.duplicate-name", "|===|\nfrom './shared.mace' import Name, Name;\n|===|\n[output = 'data'] { value: Name, }", "import Name;")),
		Entry("imports from the declaring file", quickFix("Import symbol from its declaring file", "mace.import.indirect-unexposed-symbol", "|===|\nfrom './facade.mace' import Symbol;\n|===|\n[output = 'data'] { value: Symbol, }", "from './owner.mace' import Symbol;")),
		Entry("exposes a symbol", withWorkspace(rewrite("Expose ‘Symbol’ from imported file", "mace.import.name-not-exposed", "|===|\nfrom './shared.mace' import Symbol;\n|===|\n[output = 'data'] { value: Symbol, }"), map[string]string{"shared.mace": "|===|\nstring Symbol = 'value';\n|===|\n[output = 'data'] {}"}, map[string][]string{"shared.mace": {"Symbol,"}})),
		Entry("uses a similarly named export", quickFix("Replace with similarly named exported symbol", "mace.import.name-not-exposed", "|===|\nfrom './shared.mace' import Symbl;\n|===|\n[output = 'data'] { value: Symbl, }", "Symbol")),
		Entry("extracts shared declarations", extract("Extract shared declarations to new file", "mace.import.circular", "|===|\nfrom './b.mace' import B;\nschema A: { b: B, };\n|===|\n[output = 'schema'] { A: A, }", "from './shared.mace' import")),
		Entry("moves to an existing shared file", rewrite("Move declaration to existing shared file", "mace.import.circular", "|===|\nfrom './b.mace' import B;\nschema Shared: { b: B, };\n|===|\n[output = 'schema'] { Shared: Shared, }", "from './shared.mace' import Shared;")),
		Entry("removes an unused circular edge", preferredQuickFix("Remove circular import edge", "mace.import.circular", "|===|\nfrom './b.mace' import Unused;\n|===|\n[output = 'data'] {}", "[output = 'data']")),
		Entry("updates renamed file paths", sourceAction("Update import paths after file rename", "source", "|===|\nfrom './old.mace' import Name;\n|===|\n[output = 'data'] { value: Name, }", "'./new.mace'")),
		Entry("updates renamed import names", rewrite("Update import names after symbol rename", "mace.import.renamed-symbol", "|===|\nfrom './shared.mace' import OldName;\n|===|\n[output = 'data'] { value: OldName, }", "NewName")),
	)
})
