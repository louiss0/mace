package code_actions_test

import (
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
	diagnostic, found := findDiagnosticByCode(analyzer.Diagnostics(fixture.snapshot), expected.diagnosticCode)
	if !assertions.True(found, "expected diagnostic code %q", expected.diagnosticCode) {
		return protocol.CodeAction{}
	}

	actions := analyzer.CodeActions(fixture.snapshot, fixture.uri, diagnostic.Range)
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

	assertions.Len(action.Edit.Changes, 1, "Phase 1 local fixes must only edit the current document")
	edits, found := action.Edit.Changes[uri]
	if !assertions.True(found, "workspace edit must target %s", uri) {
		return source
	}

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

	It("removes parentheses that do not alter arithmetic precedence", func() {
		source := `|===|
int count = (1);
|===|
[output = 'data']
{
  count: count,
}`
		expected := `|===|
int count = 1;
|===|
[output = 'data']
{
  count: count,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.syntax.redundant-parentheses",
			title:          "Remove redundant parentheses",
			result:         expected,
		})
	})
})
