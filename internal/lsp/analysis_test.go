package lsp

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

func writeAnalysisFile(root string, relativePath string, contents string) string {
	path := filepath.Join(root, relativePath)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	tAssert.NoError(err)
	err = os.WriteFile(path, []byte(contents), 0o600)
	tAssert.NoError(err)
	return path
}

func declarationNames(snapshot analysisSnapshot) []string {
	return lo.Map(snapshot.declarations, func(definition declarationDefinition, _ int) string {
		return definition.Name
	})
}

func requireDefinition(snapshot analysisSnapshot, position protocol.Position) protocol.Location {
	location, ok := snapshot.definitionAt(position)
	tAssert.True(ok)
	if !ok {
		return protocol.Location{}
	}

	return location
}

func requireCodeAction(snapshot analysisSnapshot, uri protocol.DocumentUri, targetRange protocol.Range, title string) protocol.CodeAction {
	action, ok := lo.Find(snapshot.codeActions(uri, targetRange), func(action protocol.CodeAction) bool {
		return action.Title == title
	})
	tAssert.True(ok)
	if !ok {
		return protocol.CodeAction{}
	}

	return action
}

func requireDiagnosticCode(diagnostic protocol.Diagnostic) string {
	tAssert.NotNil(diagnostic.Code)
	if diagnostic.Code == nil {
		return ""
	}

	value, ok := diagnostic.Code.Value.(string)
	tAssert.True(ok)
	if !ok {
		return ""
	}

	return value
}

var _ = Describe("LSP analysis", func() {
	It("surfaces only LSP-visible declarations from imports, script, and output", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-visible-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `|===|
type Hidden = string;
schema User = { name: string; };
string local = "Ada";
|===|
[output = schema]
{
  User: User;
  exported_name: string;
}`)

		snapshot := analyzeDocumentAt(`from "./shared.mace" import User;
|===|
schema Local = { id: int; };
User current = { name: "Ada"; };
|===|
[output = data]
{
  result: current;
}`, filepath.Join(workspace, "consumer.mace"))

		names := declarationNames(snapshot)
		tAssert.Contains(names, "User")
		tAssert.Contains(names, "Local")
		tAssert.Contains(names, "current")
		tAssert.Contains(names, "result")
		tAssert.NotContains(names, "Hidden")
		tAssert.NotContains(names, "local")
	})

	It("translates symbol lookups into definition locations for imported and local names", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-definition-*")
		tAssert.NoError(err)

		importPath := writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`from "./shared.mace" import User;
|===|
User current = { name: "Ada"; };
|===|
[output = data]
{
  result: current;
}`, documentPath)

		importedDefinition := requireDefinition(snapshot, protocol.Position{Line: 2, Character: 1})
		tAssert.Equal(protocol.DocumentUri(fileURI(importPath)), importedDefinition.URI)

		localDefinition := requireDefinition(snapshot, protocol.Position{Line: 6, Character: 12})
		tAssert.Equal(protocol.DocumentUri(fileURI(documentPath)), localDefinition.URI)
		tAssert.Equal(protocol.UInteger(2), localDefinition.Range.Start.Line)
	})

	It("translates import path validation into an LSP diagnostic and quick fix", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-import-fix-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = data] { name: "Ada"; }`)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`from "./shared" import name;
[output = data]
{
  result: name;
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "must end in .mace")
			tAssert.Equal(protocol.DiagnosticSeverityError, *snapshot.diagnostics[0].Severity)
			tAssert.Equal(string(diagnosticImportPathNotMace), requireDiagnosticCode(snapshot.diagnostics[0]))
		}

		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 21},
		}, "Append .mace to import path")

		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Equal(`"./shared.mace"`, edits[0].NewText)
		}
	})

	It("translates processor type mismatch errors into token-scoped diagnostics", func() {
		snapshot := analyzeDocument(`|===|
int count = "Ada";
|===|
[output = data]
{
  result: count;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "type mismatch")
			tAssert.Equal(protocol.UInteger(1), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(4), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticTypeInitializerMismatch), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("translates schema output value exports into schema-field diagnostics", func() {
		snapshot := analyzeDocument(`|===|
type Name = string;
schema User = { name: Name; age: int; };
int local = 1;
|===|
[output = schema]
{
  Name: Name;
  User: User;
  foo: local;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "invalid field type")
			tAssert.Equal(protocol.UInteger(9), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(string(diagnosticTypeInvalidOutputSchemaField), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("translates data output type exports into value diagnostics", func() {
		snapshot := analyzeDocument(`|===|
type Name = string;
schema User = { name: string; };
string value = "Ada";
|===|
{
  Name: Name;
  User: User;
  value: value;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "cannot reference type or schema declaration")
			tAssert.Equal(protocol.UInteger(6), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(string(diagnosticTypeUnknownIdentifier), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("translates processor schema validation errors into schema-scoped diagnostics", func() {
		snapshot := analyzeDocument(`|===|
schema Point = { x: int; y: int; };
schema Plot = { points: array<Point>; };
|===|
[output = data, schema = Plot]
{
  points: [
    { x: 1; y: 2; },
    { x: 3; }
  ];
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, `missing required field "y"`)
			tAssert.Equal(protocol.UInteger(1), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(7), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticTypeRecordDoesNotMatchSchema), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("warns when schema_file overlaps with local imports and script context and offers two cleanup fixes", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-schema-file-conflict-*")
		tAssert.NoError(err)

		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`from "./shared.mace" import User;
|===|
schema User = { name: string; };
|===|
[output = data, schema = User, schema_file = "./shared.mace"]
{
  result: { name: "Ada"; };
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "redundant")
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *snapshot.diagnostics[0].Severity)
			tAssert.Equal(string(diagnosticDirectiveSchemaAndSchemaFileCombined), requireDiagnosticCode(snapshot.diagnostics[0]))
		}

		diagnosticRange := snapshot.diagnostics[0].Range
		removeDirective := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnosticRange, "Remove schema_file directive")
		removeContext := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnosticRange, "Remove imports and script block")

		tAssert.NotEmpty(removeDirective.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))])
		tAssert.NotEmpty(removeContext.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))])
	})

	It("translates processor self-reference failures into output-field diagnostics", func() {
		snapshot := analyzeDocument(`[output = data]
{
  result: $self.base;
  base: 4;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "unknown self reference")
			tAssert.Equal(protocol.UInteger(2), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(string(diagnosticTypeSelfForwardReference), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("recovers visible declarations for incomplete edits used by interactive LSP features", func() {
		snapshot := analyzeCompletionContext(`|===|
schema User = { name: string; };
Us`, "", protocol.Position{Line: 2, Character: 2})

		tAssert.True(snapshot.recovered)
		tAssert.Contains(declarationNames(snapshot), "User")
	})
})
