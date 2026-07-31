package code_actions_test

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/louiss0/mace/internal/analyzer"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestCodeActions(t *testing.T) {
	RunSpecs(t, "Code Actions Suite")
}

type codeActionFixture struct {
	source   string
	path     string
	uri      protocol.DocumentUri
	snapshot analyzer.Snapshot
}

type expectedQuickFix struct {
	diagnosticCode string
	title          string
	result         string
}

type codeActionContract struct {
	title            string
	diagnosticCode   string
	kind             protocol.CodeActionKind
	preferred        *bool
	source           string
	workspaceFiles   map[string]string
	expectedText     []string
	unexpectedText   []string
	expectedFileText map[string][]string
	expectedCommand  string
}

func preferredQuickFix(title string, code string, source string, expectedText ...string) codeActionContract {
	preferred := true
	return codeActionContract{
		title:          title,
		diagnosticCode: code,
		kind:           protocol.CodeActionKindQuickFix,
		preferred:      &preferred,
		source:         source,
		expectedText:   expectedText,
	}
}

func quickFix(title string, code string, source string, expectedText ...string) codeActionContract {
	return codeActionContract{
		title:          title,
		diagnosticCode: code,
		kind:           protocol.CodeActionKindQuickFix,
		source:         source,
		expectedText:   expectedText,
	}
}

func rewrite(title string, code string, source string, expectedText ...string) codeActionContract {
	return codeActionContract{
		title:          title,
		diagnosticCode: code,
		kind:           protocol.CodeActionKindRefactorRewrite,
		source:         source,
		expectedText:   expectedText,
	}
}

func extract(title string, code string, source string, expectedText ...string) codeActionContract {
	return codeActionContract{
		title:          title,
		diagnosticCode: code,
		kind:           protocol.CodeActionKindRefactorExtract,
		source:         source,
		expectedText:   expectedText,
	}
}

func sourceAction(title string, kind protocol.CodeActionKind, source string, expectedText ...string) codeActionContract {
	return codeActionContract{
		title:        title,
		kind:         kind,
		source:       source,
		expectedText: expectedText,
	}
}

func withWorkspace(contract codeActionContract, files map[string]string, expectedFileText map[string][]string) codeActionContract {
	contract.workspaceFiles = files
	contract.expectedFileText = expectedFileText
	return contract
}

func testCodeActionContract(contract codeActionContract) {
	assertions := assert.New(GinkgoT())
	fixture := newCodeActionFixture(contract.source, contract.workspaceFiles)
	targetRange := protocol.Range{
		Start: protocol.Position{},
		End:   protocol.Position{Line: protocol.UInteger(len(strings.Split(contract.source, "\n")) + 1)},
	}

	action, found := findCodeActionByTitle(analyzer.CodeActions(fixture.snapshot, fixture.uri, targetRange), contract.title)
	if !assertions.True(found, "expected code action %q", contract.title) {
		return
	}

	if assertions.NotNil(action.Kind) {
		assertions.Equal(contract.kind, *action.Kind)
	}
	if contract.preferred != nil && assertions.NotNil(action.IsPreferred) {
		assertions.Equal(*contract.preferred, *action.IsPreferred)
	}
	if contract.diagnosticCode != "" {
		assertions.True(codeActionResolvesDiagnostic(action, contract.diagnosticCode))
	}
	if contract.expectedCommand != "" {
		if assertions.NotNil(action.Command) {
			assertions.Equal(contract.expectedCommand, action.Command.Command)
		}
		return
	}
	if !assertions.NotNil(action.Edit) {
		return
	}

	if len(contract.expectedText) > 0 || len(contract.unexpectedText) > 0 {
		result := applyDocumentEdits(fixture.source, fixture.uri, action)
		for _, expected := range contract.expectedText {
			assertions.Contains(result, expected)
		}
		for _, unexpected := range contract.unexpectedText {
			assertions.NotContains(result, unexpected)
		}
	}
	for relativePath, expectedValues := range contract.expectedFileText {
		uri := documentURI(filepath.Join(filepath.Dir(fixture.path), relativePath))
		contents, err := os.ReadFile(filepath.Join(filepath.Dir(fixture.path), relativePath))
		if !assertions.NoError(err) {
			continue
		}
		updated := applyEdits(string(contents), action.Edit.Changes[uri])
		for _, expected := range expectedValues {
			assertions.Contains(updated, expected)
		}
	}
}

func newCodeActionFixture(source string, workspaceFiles map[string]string) codeActionFixture {
	testingT := GinkgoT()
	assertions := assert.New(testingT)
	workspace := testingT.TempDir()

	for relativePath, contents := range workspaceFiles {
		path := filepath.Join(workspace, relativePath)
		assertions.NoError(os.MkdirAll(filepath.Dir(path), 0o755))
		assertions.NoError(os.WriteFile(path, []byte(contents), 0o600))
	}

	documentPath := filepath.Join(workspace, "document.mace")
	assertions.NoError(os.WriteFile(documentPath, []byte(source), 0o600))
	uri := documentURI(documentPath)

	return codeActionFixture{
		source:   source,
		path:     documentPath,
		uri:      uri,
		snapshot: analyzer.AnalyzeDocumentAtInRoot(source, documentPath, workspace),
	}
}

func (fixture codeActionFixture) requirePreferredQuickFix(expected expectedQuickFix) protocol.CodeAction {
	action := fixture.requireQuickFix(expected)
	if action.Title == "" {
		return action
	}

	assertions := assert.New(GinkgoT())
	if assertions.NotNil(action.IsPreferred, "deterministic Phase 1 fix %q must be preferred", expected.title) {
		assertions.True(*action.IsPreferred)
	}

	return action
}

func (fixture codeActionFixture) requireQuickFix(expected expectedQuickFix) protocol.CodeAction {
	assertions := assert.New(GinkgoT())
	targetRange := protocol.Range{
		Start: protocol.Position{},
		End:   protocol.Position{Line: protocol.UInteger(len(strings.Split(fixture.source, "\n")) + 1)},
	}
	actions := analyzer.CodeActions(fixture.snapshot, fixture.uri, targetRange)
	action, found := findCodeActionByTitle(actions, expected.title)
	if !assertions.True(found, "expected code action %q for diagnostic %q", expected.title, expected.diagnosticCode) {
		return protocol.CodeAction{}
	}

	if assertions.NotNil(action.Kind) {
		assertions.Equal(protocol.CodeActionKindQuickFix, *action.Kind)
	}
	assertions.True(
		codeActionResolvesDiagnostic(action, expected.diagnosticCode),
		"code action %q must carry diagnostic code %q",
		expected.title,
		expected.diagnosticCode,
	)

	result := applyDocumentEdits(fixture.source, fixture.uri, action)
	assertions.Equal(expected.result, result)
	fixedSnapshot := analyzer.AnalyzeDocumentAtInRoot(result, fixture.path, filepath.Dir(fixture.path))
	_, diagnosticRemains := findDiagnosticByCode(analyzer.Diagnostics(fixedSnapshot), expected.diagnosticCode)
	assertions.False(diagnosticRemains, "code action %q must resolve diagnostic %q", expected.title, expected.diagnosticCode)

	return action
}

func findDiagnosticByCode(diagnostics []protocol.Diagnostic, code string) (protocol.Diagnostic, bool) {
	for _, diagnostic := range diagnostics {
		if diagnosticCode(diagnostic) == code {
			return diagnostic, true
		}
	}

	return protocol.Diagnostic{}, false
}

func findCodeActionByTitle(actions []protocol.CodeAction, title string) (protocol.CodeAction, bool) {
	for _, action := range actions {
		if action.Title == title {
			return action, true
		}
	}

	return protocol.CodeAction{}, false
}

func codeActionResolvesDiagnostic(action protocol.CodeAction, code string) bool {
	for _, diagnostic := range action.Diagnostics {
		if diagnosticCode(diagnostic) == code {
			return true
		}
	}

	return false
}

func diagnosticCode(diagnostic protocol.Diagnostic) string {
	if diagnostic.Code == nil {
		return ""
	}

	code, ok := diagnostic.Code.Value.(string)
	if !ok {
		return ""
	}

	return code
}

func applyDocumentEdits(source string, uri protocol.DocumentUri, action protocol.CodeAction) string {
	assertions := assert.New(GinkgoT())
	if !assertions.NotNil(action.Edit, "code action must provide a workspace edit") {
		return source
	}

	edits, found := action.Edit.Changes[uri]
	if !assertions.True(found, "workspace edit must target %s", uri) {
		return source
	}

	return applyEdits(source, edits)
}

func applyEdits(source string, edits []protocol.TextEdit) string {
	assertions := assert.New(GinkgoT())
	type indexedEdit struct {
		start   int
		end     int
		newText string
	}
	indexedEdits := make([]indexedEdit, 0, len(edits))
	for _, edit := range edits {
		start, end := edit.Range.IndexesIn(source)
		if !assertions.GreaterOrEqual(start, 0) || !assertions.GreaterOrEqual(end, start) || !assertions.LessOrEqual(end, len(source)) {
			continue
		}
		indexedEdits = append(indexedEdits, indexedEdit{start: start, end: end, newText: edit.NewText})
	}

	sort.Slice(indexedEdits, func(left int, right int) bool {
		return indexedEdits[left].start > indexedEdits[right].start
	})

	result := source
	lastStart := len(source) + 1
	for _, edit := range indexedEdits {
		if !assertions.LessOrEqual(edit.end, lastStart, "workspace edits must not overlap") {
			continue
		}
		result = result[:edit.start] + edit.newText + result[edit.end:]
		lastStart = edit.start
	}

	return result
}

func documentURI(path string) protocol.DocumentUri {
	path = filepath.ToSlash(path)
	if len(path) >= 2 && path[1] == ':' {
		path = "/" + path
	}

	return protocol.DocumentUri((&url.URL{Scheme: "file", Path: path}).String())
}

var _ = Describe("File and syntax structure code actions", func() {
	It("inserts a missing output-field comma", func() {
		source := `[output = 'data']
{
  first: 1
  second: 2,
}`
		expected := `[output = 'data']
{
  first: 1,
  second: 2,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.syntax.missing-field-comma",
			title:          "Insert comma after field",
			result:         expected,
		})
	})

	It("inserts a missing declaration semicolon", func() {
		source := `|===|
alias Name: string
|===|
[output = 'schema']
{
  Name: Name,
}`
		expected := `|===|
alias Name: string;
|===|
[output = 'schema']
{
  Name: Name,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.syntax.missing-declaration-semicolon",
			title:          "Insert declaration semicolon",
			result:         expected,
		})
	})

	It("matches a mismatched closing script delimiter", func() {
		source := `|===|
alias Name: string;
|====|
[output = 'schema']
{
  Name: Name,
}`
		expected := `|===|
alias Name: string;
|===|
[output = 'schema']
{
  Name: Name,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.syntax.inconsistent-script-delimiters",
			title:          "Match closing script delimiter",
			result:         expected,
		})
	})

	DescribeTable("satisfies the remaining file and syntax contracts",
		testCodeActionContract,
		Entry("matches the opening delimiter", preferredQuickFix("Match opening script delimiter", "mace.syntax.inconsistent-script-delimiters", "|====|\nalias Name: string;\n|===|\n[output = 'schema'] { Name: Name, }", "|===|")),
		Entry("inserts a closing delimiter", preferredQuickFix("Insert closing script delimiter", "mace.syntax.unterminated-script-block", "|===|\nalias Name: string;", "\n|===|")),
		Entry("removes an empty script block", preferredQuickFix("Remove empty script block", "mace.syntax.empty-script-block", "|===|\n|===|\n[output = 'data'] {}", "[output = 'data']")),
		Entry("moves declarations into script", rewrite("Move declarations inside script block", "mace.file-structure.declaration-outside-script", "string name = 'Mace';\n[output = 'data'] { name: name, }", "|===|", "string name = 'Mace';")),
		Entry("creates a script around declarations", rewrite("Create script block around declarations", "mace.file-structure.missing-script-block", "alias Name: string;\n[output = 'schema'] { Name: Name, }", "|===|\nalias Name: string;\n|===|")),
		Entry("inserts an output block", quickFix("Insert missing output block", "mace.file-structure.missing-output-block", "|===|\nstring name = 'Mace';\n|===|", "[output = 'data']", "{}")),
		Entry("removes a duplicate output", quickFix("Remove duplicate output block", "mace.file-structure.multiple-output-blocks", "[output = 'data'] { first: 1, }\n[output = 'data'] { second: 2, }", "first: 1")),
		Entry("merges duplicate output fields", rewrite("Move output fields into the first output block", "mace.file-structure.multiple-output-blocks", "[output = 'data'] { first: 1, }\n[output = 'data'] { second: 2, }", "first: 1", "second: 2")),
		Entry("replaces a field semicolon", preferredQuickFix("Replace semicolon with comma", "mace.syntax.field-semicolon", "[output = 'data'] { first: 1; second: 2, }", "first: 1,")),
		Entry("removes a trailing token", quickFix("Remove unexpected trailing token", "mace.syntax.unexpected-trailing-token", "[output = 'data'] { value: 1, } garbage", "[output = 'data'] { value: 1, }")),
		Entry("separates subtraction", preferredQuickFix("Separate subtraction operator with whitespace", "mace.syntax.kebab-identifier-used-as-subtraction", "|===|\nint first = 3; int second = 1; int result = first-second;\n|===|\n[output = 'data'] { result: result, }", "first - second")),
	)
})
