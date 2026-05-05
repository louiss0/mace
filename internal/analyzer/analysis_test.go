package analyzer

import (
	"os"
	"path/filepath"
	"strings"

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
type Hidden: string;
schema User: { name: string; };
string local = "Ada";
|===|
[output = schema]
{
  User: User;
  exported_name: string;
}`)

		snapshot := analyzeDocumentAt(`from "./shared.mace" import User;
|===|
schema Local: { id: int; };
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

	It("prefers output field definitions over same-named schema declarations", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-output-definition-*")
		tAssert.NoError(err)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
|===|
[output = data]
{
  User: { name: "Ada"; };
}`, documentPath)

		definition := requireDefinition(snapshot, protocol.Position{Line: 5, Character: 3})
		tAssert.Equal(protocol.DocumentUri(fileURI(documentPath)), definition.URI)
		tAssert.Equal(protocol.UInteger(5), definition.Range.Start.Line)
		tAssert.Equal(protocol.UInteger(2), definition.Range.Start.Character)
	})

	It("prefers current document definitions over imported symbols with matching coordinates", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-definition-coordinates-*")
		tAssert.NoError(err)

		importPath := writeAnalysisFile(workspace, "shared.mace", `[output = data]
{




       qux: 1;
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`from "./shared.mace" import qux;
|===|
int qux = 2;
|===|

{
  bar: qux;
}`, documentPath)

		definition := requireDefinition(snapshot, protocol.Position{Line: 6, Character: 7})
		tAssert.Equal(protocol.DocumentUri(fileURI(documentPath)), definition.URI)
		tAssert.NotEqual(protocol.DocumentUri(fileURI(importPath)), definition.URI)
		tAssert.Equal(protocol.UInteger(2), definition.Range.Start.Line)
		tAssert.Equal(protocol.UInteger(4), definition.Range.Start.Character)
	})

	It("resolves enum member definitions from usage sites", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-enum-member-definition-*")
		tAssert.NoError(err)
		documentPath := filepath.Join(workspace, "consumer.mace")

		snapshot := analyzeDocumentAt(`|===|
enum Fruit: string {
  Apple,
  Strawberry = "strawberry",
};
Fruit selected = Fruit.Apple;
|===|
[output = data]
{
  selected: selected;
}`, documentPath)

		definition := requireDefinition(snapshot, protocol.Position{Line: 5, Character: 23})
		tAssert.Equal(protocol.DocumentUri(fileURI(documentPath)), definition.URI)
		tAssert.Equal(protocol.UInteger(2), definition.Range.Start.Line)
		tAssert.Equal(protocol.UInteger(2), definition.Range.Start.Character)
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

	It("does not report diagnostics for an unused injectable without an initializer", func() {
		snapshot := analyzeDocument(`|===|
injectable string env;
|===|
[output = data] {}`)

		tAssert.Empty(snapshot.diagnostics)
	})

	It("offers documentation generation actions for declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
|===|
[output = schema]
{
  User: User;
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 7},
			End:   protocol.Position{Line: 1, Character: 11},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Generate schema_doc")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `schema_doc User`)
		}
	})

	It("offers schema output generation from schema declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string, age: int, active: boolean, tags: array<string>, meta: { id: string } };
|===|
[output = data]
{}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 7},
			End:   protocol.Position{Line: 1, Character: 11},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Generate output block from schema")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = data, schema = User]`)
			tAssert.Contains(edits[0].NewText, `name: ""`)
			tAssert.Contains(edits[0].NewText, `age: 0`)
			tAssert.Contains(edits[0].NewText, `active: false`)
			tAssert.Contains(edits[0].NewText, `tags: []`)
			tAssert.Contains(edits[0].NewText, `meta: {}`)
		}
	})

	It("offers schema directive insertion for data outputs", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
|===|
[output = data]
{
  name: "Ada";
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 7},
			End:   protocol.Position{Line: 1, Character: 11},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Add schema = User directive")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = data, schema = User]`)
		}
	})

	It("offers explicit output directives for implicit outputs", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`{
  name: "Ada";
}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Make implicit output explicit")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = data]`)
		}
	})

	It("offers conversion from data output to schema output", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = data]
{
  name: "Ada";
  age: 42;
  active: true;
}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Convert data output to schema output")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Contains(edits[0].NewText, `[output = schema]`)
			tAssert.Contains(edits[0].NewText, `name: string`)
			tAssert.Contains(edits[0].NewText, `age: int`)
			tAssert.Contains(edits[0].NewText, `active: boolean`)
		}
	})

	It("offers optional marker toggles for schema output fields", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = schema]
{
  name: string;
  age?: int;
}`, documentPath)

		nameRange := protocol.Range{
			Start: protocol.Position{Line: 2, Character: 2},
			End:   protocol.Position{Line: 2, Character: 6},
		}
		addAction := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), nameRange, "Add optional marker ?")
		addEdits := addAction.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(addEdits, 1) {
			tAssert.Contains(addEdits[0].NewText, `name?: string`)
		}

		ageRange := protocol.Range{
			Start: protocol.Position{Line: 3, Character: 2},
			End:   protocol.Position{Line: 3, Character: 5},
		}
		removeAction := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), ageRange, "Remove optional marker ?")
		removeEdits := removeAction.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(removeEdits, 1) {
			tAssert.Contains(removeEdits[0].NewText, `age: int`)
		}
	})

	It("offers import refactor actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`from "shared.mace" import User, Profile;
from "shared.mace" import Role;
[output = schema]
{}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		uri := protocol.DocumentUri(fileURI(documentPath))
		fixAction := requireCodeAction(snapshot, uri, rangeValue, "Fix relative import path")
		tAssert.Contains(fixAction.Edit.Changes[uri][0].NewText, `from "./shared.mace" import User, Profile;`)
		splitAction := requireCodeAction(snapshot, uri, rangeValue, "Split import declaration")
		tAssert.Contains(splitAction.Edit.Changes[uri][0].NewText, `from "shared.mace" import User;`)
		mergeAction := requireCodeAction(snapshot, uri, rangeValue, "Merge duplicate imports")
		tAssert.Contains(mergeAction.Edit.Changes[uri][0].NewText, `from "shared.mace" import User, Profile, Role;`)
	})

	It("offers import resolution actions", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-import-resolution-*")
		tAssert.NoError(err)
		defer os.RemoveAll(workspace)

		sharedPath := writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: string;
  Role: string;
}`)
		documentPath := writeAnalysisFile(workspace, "document.mace", `from "./missing.mace" import User;
from "./shared.mace" import Usre;
from "./shared-old.mace" import Role;
[output = schema]
{}`)
		contents, err := os.ReadFile(documentPath)
		tAssert.NoError(err)
		snapshot := analyzeDocumentAt(string(contents), documentPath)
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		createAction := requireCodeAction(snapshot, uri, rangeValue, "Create missing imported file")
		tAssert.Contains(createAction.Edit.Changes[protocol.DocumentUri(fileURI(filepath.Join(workspace, "missing.mace")))][0].NewText, "[output = schema]")

		renameAction := requireCodeAction(snapshot, uri, rangeValue, "Update import path after file rename")
		tAssert.Equal(`"./shared.mace"`, renameAction.Edit.Changes[uri][0].NewText)

		replaceAction := requireCodeAction(snapshot, uri, rangeValue, "Replace unavailable imported symbol with User")
		tAssert.Equal("User", replaceAction.Edit.Changes[uri][0].NewText)

		openAction := requireCodeAction(snapshot, uri, rangeValue, "Open source output block")
		tAssert.Equal(protocol.DocumentUri(fileURI(sharedPath)), openAction.Command.Arguments[0])

		explainAction := requireCodeAction(snapshot, uri, rangeValue, "Explain why symbol is not importable")
		tAssert.Contains(explainAction.Command.Arguments[0], "Only names surfaced through the imported file output block are importable")
	})

	It("offers remaining add and fix import actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))

		snapshot := analyzeDocumentAt(`from "shared" import User;
from "zeta.mace" import Zed;
from "alpha.mace" import User;
from "dupes.mace" import User, User, Role;
[output = schema]
{}`, documentPath)
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		extensionAction := requireCodeAction(snapshot, uri, protocol.Range{Start: protocol.Position{Line: 0, Character: 5}, End: protocol.Position{Line: 0, Character: 13}}, "Append .mace to import path")
		tAssert.Equal(`"shared.mace"`, extensionAction.Edit.Changes[uri][0].NewText)

		sortAction := requireCodeAction(snapshot, uri, rangeValue, "Sort imports")
		tAssert.Contains(sortAction.Edit.Changes[uri][0].NewText, "from \"alpha.mace\" import User;\nfrom \"dupes.mace\" import User, User, Role;")

		duplicateAction := requireCodeAction(snapshot, uri, rangeValue, "Remove duplicate imported names")
		tAssert.Contains(duplicateAction.Edit.Changes[uri][0].NewText, `from "dupes.mace" import User, Role;`)

		wildcardSnapshot := analyzeDocumentAt(`from "shared.mace" import *;
[output = schema]
{}`, documentPath)
		wildcardAction := requireCodeAction(wildcardSnapshot, uri, protocol.Range{Start: protocol.Position{Line: 0, Character: 26}, End: protocol.Position{Line: 0, Character: 27}}, "Convert wildcard import to named import")
		tAssert.Equal("Name", wildcardAction.Edit.Changes[uri][0].NewText)
	})

	Describe("schema creation actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("extracts output block shapes into schemas", func() {
			snapshot := analyzeDocumentAt(`[output = data]
{ name: "Ada"; age: 30; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract output block shape into schema")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema Output:")
			tAssert.Contains(text, "name: string")
			tAssert.Contains(text, "schema = Output")
		})

		It("extracts record literals into schemas", func() {
			snapshot := analyzeDocumentAt(`|===|
User user = { name: "Ada"; };
|===|
[output = data]
{ value: user; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract record literal into schema")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema User:")
			tAssert.Contains(text, `User user = { name: "Ada"; };`)
		})

		It("creates schemas from selected fields", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; age: int; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Create schema from selected fields")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema Extracted:")
			tAssert.Contains(text, "name: string")
		})

		It("creates schemas from validation errors", func() {
			snapshot := analyzeDocumentAt(`[output = data, schema = User]
{ name: "Ada"; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Create schema from validation error")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "schema User:")
		})

		It("generates sample data from schemas", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; age: int; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Generate sample data from schema")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, `[output = data, schema = User]`)
			tAssert.Contains(text, `name: ""`)
		})
	})

	Describe("array actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("wraps types in arrays", func() {
			snapshot := analyzeDocumentAt(`|===|
type Name: string;
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Wrap type in array")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "type Name: array<string>;")
		})

		It("fixes mixed array literals with variants", func() {
			snapshot := analyzeDocumentAt(`|===|
array<string> values = ["Ada", 1];
|===|
[output = data]
{ value: values; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Fix mixed array literal")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ValuesItem: variant[string, int];")
			tAssert.Contains(text, "array<ValuesItem> values")
		})

		It("changes array element types to match literals", func() {
			snapshot := analyzeDocumentAt(`|===|
array<string> values = [1, 2];
|===|
[output = data]
{ value: values; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Change array element type")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "array<int> values")
		})

		It("replaces invalid array indexes", func() {
			snapshot := analyzeDocumentAt(`[output = data]
{ value: ["Ada"][3]; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Replace invalid array index")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `value: ["Ada"][0]`)
		})
	})

	Describe("type alias actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("creates type aliases from selected schema field types", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Create type alias from selected type")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ExtractedType: string;")
			tAssert.Contains(text, "name: ExtractedType")
		})

		It("inlines type alias usage in schema fields", func() {
			snapshot := analyzeDocumentAt(`|===|
type Name: string;
schema User: { name: Name; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Inline type alias usage")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "name: string")
		})

		It("renames type aliases", func() {
			snapshot := analyzeDocumentAt(`|===|
type Name: string;
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Rename type alias")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "type RenamedName: string;")
		})

		It("replaces unknown types with closest known types", func() {
			snapshot := analyzeDocumentAt(`|===|
type Name: string;
schema User: { name: Nmae; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Replace unknown type with Name")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "name: Name")
		})

		It("converts Array casing to array", func() {
			snapshot := analyzeDocumentAt(`|===|
type Names: Array<string>;
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Convert Array<T> to array<T>")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "array<string>")
		})

		It("converts invalid nullable types into optional fields", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string?; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Convert nullable type into optional field")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "name?: string")
		})
	})

	Describe("injectable variable actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("converts variables to injectable", func() {
			snapshot := analyzeDocumentAt(`|===|
string name;
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Convert variable to injectable")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `injectable string name;`)
		})

		DescribeTable("adds default initializers to injectables by type", func(typeName string, variableName string, literal string) {
			snapshot := analyzeDocumentAt(`|===|
injectable `+typeName+` `+variableName+`;
|===|
[output = data]
{ value: `+variableName+`; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add default initializer to injectable")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `injectable `+typeName+` `+variableName+` = `+literal+`;`)
		},
			Entry("string", "string", "name", `""`),
			Entry("int", "int", "count", "0"),
			Entry("float", "float", "ratio", "0.0"),
			Entry("boolean", "boolean", "enabled", "false"),
			Entry("array", "array<string>", "names", "[]"),
		)

		It("generates injection config stubs", func() {
			snapshot := analyzeDocumentAt(`|===|
injectable string name;
injectable int count;
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Generate injection config stub")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, `"name": ""`)
			tAssert.Contains(text, `"count": 0`)
		})

		It("finds all injectable variables", func() {
			snapshot := analyzeDocumentAt(`|===|
injectable string name;
injectable int count;
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Find all injectable variables")
			tAssert.Contains(action.Command.Arguments[0], "name, count")
		})
	})

	Describe("variable fix actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("adds missing type annotations", func() {
			snapshot := analyzeDocumentAt(`|===|
name = "Ada";
title = "Engineer";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add missing type annotation")
			text := action.Edit.Changes[uri][0].NewText

			tAssert.Contains(text, `string name = "Ada";`)
			tAssert.Contains(text, `string title = "Engineer";`)
		})

		It("adds missing initializers", func() {
			snapshot := analyzeDocumentAt(`|===|
string name;
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add missing initializer")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `string name = "";`)
		})

		It("marks variables injectable", func() {
			snapshot := analyzeDocumentAt(`|===|
string name;
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Mark variable as injectable")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `injectable string name;`)
		})

		It("adds placeholder initializers", func() {
			snapshot := analyzeDocumentAt(`|===|
int count;
|===|
[output = data]
{ value: count; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add placeholder initializer")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `int count = 0;`)
		})

		It("changes variable type to inferred expression type", func() {
			snapshot := analyzeDocumentAt(`|===|
int name = "Ada";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Change variable type to inferred expression type")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `string name = "Ada";`)
		})

		It("changes initializers to match declared types", func() {
			snapshot := analyzeDocumentAt(`|===|
int count = "Ada";
|===|
[output = data]
{ value: count; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Change initializer to match declared type")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `int count = 0;`)
		})

		It("renames duplicate variables", func() {
			snapshot := analyzeDocumentAt(`|===|
string name = "Ada";
string name = "Grace";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Rename duplicate variable")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `string name_2 = "Grace";`)
		})

		It("inlines variables into output fields", func() {
			snapshot := analyzeDocumentAt(`|===|
string name = "Ada";
|===|
[output = data]
{ value: name; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Inline variable into output field")
			tAssert.Contains(action.Edit.Changes[uri][0].NewText, `value: "Ada"`)
		})

		It("extracts output expressions into script variables", func() {
			snapshot := analyzeDocumentAt(`[output = data]
{ value: "Ada"; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract output expression into script variable")
			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, `string value = "Ada";`)
			tAssert.Contains(text, `value: value`)
		})
	})

	Describe("declaration actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("adds missing semicolons after script declarations", func() {
			snapshot := analyzeDocumentAt(`|===|
type Name: string
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Add missing semicolon")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "type Name: string;")
		})

		It("extracts repeated type references into an alias", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { name: string; email: string; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract repeated type into alias")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "type ExtractedType: string;")
			tAssert.Contains(text, "name: ExtractedType")
		})

		It("extracts inline record types into schemas", func() {
			snapshot := analyzeDocumentAt(`|===|
schema User: { profile: { name: string; }; };
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Extract inline record type into schema")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema Profile:")
			tAssert.Contains(text, "profile: Profile")
		})

		It("converts record variables into schema-backed variables", func() {
			snapshot := analyzeDocumentAt(`|===|
{ name: string; } user = { name: "Ada"; };
|===|
[output = data]
{ value: user; }`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Convert record variable into schema-backed variable")

			text := action.Edit.Changes[uri][0].NewText
			tAssert.Contains(text, "schema User:")
			tAssert.Contains(text, "User user =")
		})
	})

	Describe("script block structure actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		rangeValue := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}

		It("creates a script block above the output block", func() {
			snapshot := analyzeDocumentAt(`[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Create script block")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\n|===|\n[output = schema]")
		})

		It("wraps the document in a script block", func() {
			snapshot := analyzeDocumentAt(`[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Wrap selection in script block")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\n[output = schema]")
		})

		It("fixes mismatched script delimiter widths", func() {
			snapshot := analyzeDocumentAt(`|====|
type Name: string;
|====|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Fix script delimiter length mismatch")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\ntype Name: string;")
		})

		It("normalizes script fences", func() {
			snapshot := analyzeDocumentAt(`|====|
type Name: string;
|====|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Normalize script fence")

			tAssert.Contains(action.Edit.Changes[uri][0].NewText, "|===|\ntype Name: string;")
		})

		It("removes empty script blocks", func() {
			snapshot := analyzeDocumentAt(`|===|
|===|
[output = schema]
{}`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Remove empty script block")

			tAssert.NotContains(action.Edit.Changes[uri][0].NewText, "|===|\n|===|")
		})

		It("moves script blocks before output blocks", func() {
			snapshot := analyzeDocumentAt(`[output = schema]
{}
|===|
type Name: string;
|===|`, documentPath)
			action := requireCodeAction(snapshot, uri, rangeValue, "Move script block before output block")

			tAssert.True(strings.HasPrefix(action.Edit.Changes[uri][0].NewText, "|===|\ntype Name: string;"))
		})
	})

	It("offers documentation cleanup actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
schema User: { name: string /# inline; age: int; };
schema_doc User {
  summary: "Existing";
};
|===|
[output = schema]
{}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{Line: 1, Character: 0}, End: protocol.Position{Line: 1, Character: 60}}
		uri := protocol.DocumentUri(fileURI(documentPath))
		propsAction := requireCodeAction(snapshot, uri, rangeValue, "Add missing props docs")
		tAssert.Contains(propsAction.Edit.Changes[uri][0].NewText, `props: {`)
		moveAction := requireCodeAction(snapshot, uri, rangeValue, "Move inline /# docs to structured docs")
		tAssert.Contains(moveAction.Edit.Changes[uri][0].NewText, `name: ""`)
		removeAction := requireCodeAction(snapshot, uri, rangeValue, "Remove conflicting docs")
		tAssert.NotContains(removeAction.Edit.Changes[uri][0].NewText, `/# inline`)
	})

	It("offers enum normalization actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
enum Status: string { Pending, Done = "done" };
|===|
[output = schema]
{}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{Line: 1, Character: 5}, End: protocol.Position{Line: 1, Character: 11}}
		uri := protocol.DocumentUri(fileURI(documentPath))
		explicitAction := requireCodeAction(snapshot, uri, rangeValue, "Convert mixed enum to all-explicit")
		tAssert.Contains(explicitAction.Edit.Changes[uri][0].NewText, `Pending = "pending"`)
		implicitAction := requireCodeAction(snapshot, uri, rangeValue, "Convert mixed enum to all-implicit")
		tAssert.Contains(implicitAction.Edit.Changes[uri][0].NewText, `Pending,`)
		tAssert.NotContains(implicitAction.Edit.Changes[uri][0].NewText, `Done =`)
		missingAction := requireCodeAction(snapshot, uri, rangeValue, "Add missing enum member")
		tAssert.Contains(missingAction.Edit.Changes[uri][0].NewText, `Missing`)
	})

	It("offers string and style actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|====|
string name = "Ada";
|====|
[output = data]
{
  name: name;
}`, documentPath)

		uri := protocol.DocumentUri(fileURI(documentPath))
		stringRange := protocol.Range{Start: protocol.Position{Line: 1, Character: 14}, End: protocol.Position{Line: 1, Character: 19}}
		stringAction := requireCodeAction(snapshot, uri, stringRange, "Convert string form")
		tAssert.Contains(stringAction.Edit.Changes[uri][0].NewText, `'Ada'`)
		interpolatedAction := requireCodeAction(snapshot, uri, stringRange, "Convert to interpolated string")
		tAssert.Contains(interpolatedAction.Edit.Changes[uri][0].NewText, `"Ada $()"`)
		globalRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		separatorAction := requireCodeAction(snapshot, uri, globalRange, "Normalize separators")
		tAssert.Contains(separatorAction.Edit.Changes[uri][0].NewText, `name: name,`)
		fenceAction := requireCodeAction(snapshot, uri, globalRange, "Normalize script fence width")
		tAssert.Contains(fenceAction.Edit.Changes[uri][0].NewText, `|===|`)
	})

	It("offers expression and self refactor actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = data]
{
  first: "Ada";
  repeated: "Ada";
}`, documentPath)

		uri := protocol.DocumentUri(fileURI(documentPath))
		globalRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		extractAction := requireCodeAction(snapshot, uri, globalRange, "Extract expression into variable")
		tAssert.Contains(extractAction.Edit.Changes[uri][0].NewText, `extracted_value`)
		inlineAction := requireCodeAction(snapshot, uri, globalRange, "Inline variable into output")
		tAssert.NotNil(inlineAction.Edit)
		selfAction := requireCodeAction(snapshot, uri, globalRange, "Rewrite expression to use $self")
		tAssert.Contains(selfAction.Edit.Changes[uri][0].NewText, `$self.first`)
	})

	It("offers interop generation actions", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = schema]
{
  name: string;
}`, documentPath)

		uri := protocol.DocumentUri(fileURI(documentPath))
		globalRange := protocol.Range{Start: protocol.Position{}, End: protocol.Position{}}
		jsonAction := requireCodeAction(snapshot, uri, globalRange, "Generate JSON preview")
		tAssert.Contains(jsonAction.Edit.Changes[uri][0].NewText, `JSON preview`)
		maceAction := requireCodeAction(snapshot, uri, globalRange, "Generate Mace schema from sample data")
		tAssert.Contains(maceAction.Edit.Changes[uri][0].NewText, `schema Generated`)
		schemaAction := requireCodeAction(snapshot, uri, globalRange, "Generate JSON Schema from Mace schema")
		tAssert.Contains(schemaAction.Edit.Changes[uri][0].NewText, `JSON Schema`)
	})

	It("offers inline record extraction", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`[output = schema]
{
  user: { name: string; };
}`, documentPath)

		rangeValue := protocol.Range{Start: protocol.Position{Line: 2, Character: 2}, End: protocol.Position{Line: 2, Character: 6}}
		uri := protocol.DocumentUri(fileURI(documentPath))
		action := requireCodeAction(snapshot, uri, rangeValue, "Convert inline record to schema")
		tAssert.Contains(action.Edit.Changes[uri][0].NewText, `schema User`)
		tAssert.Contains(action.Edit.Changes[uri][0].NewText, `user: User`)
	})

	It("offers inline description actions for type declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
type Name: string;
|===|
[output = schema]
{
  Name: Name;
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 5},
			End:   protocol.Position{Line: 1, Character: 9},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Add inline /# description")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Equal(` /# description`, edits[0].NewText)
		}
	})

	It("treats schema directives as import usages", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-schema-directive-import-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import User;
|===|
[output = data, schema = User]
{
  name: "Ada";
}`, documentPath)

		tAssert.Empty(snapshot.diagnostics)
	})

	It("inserts inline descriptions after complex type declarations", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
type User: { name: string; };
|===|
[output = schema]
{
  User: User;
}`, documentPath)

		rangeValue := protocol.Range{
			Start: protocol.Position{Line: 1, Character: 5},
			End:   protocol.Position{Line: 1, Character: 9},
		}
		action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), rangeValue, "Add inline /# description")
		edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
		if tAssert.Len(edits, 1) {
			tAssert.Equal(protocol.UInteger(1), edits[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(28), edits[0].Range.Start.Character)
		}
	})

	It("warns about unused imports and offers removal", func() {
		workspace, err := os.MkdirTemp("", "mace-analysis-unused-import-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: { name: string; };
  Config: { enabled: boolean; };
}`)
		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := analyzeDocumentAt(`|===|
from "./shared.mace" import User, Config;
User user = { name: "Ada"; };
|===|
[output = data]
{
  user: user;
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			diagnostic := snapshot.diagnostics[0]
			tAssert.Contains(diagnostic.Message, `import "Config" is never used`)
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *diagnostic.Severity)
			tAssert.Equal(string(diagnosticImportUnused), requireDiagnosticCode(diagnostic))

			action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnostic.Range, "Remove unused import")
			edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
			if tAssert.Len(edits, 1) {
				tAssert.Equal(``, edits[0].NewText)
				tAssert.Equal(protocol.UInteger(1), edits[0].Range.Start.Line)
				tAssert.Equal(protocol.UInteger(32), edits[0].Range.Start.Character)
				tAssert.Equal(protocol.UInteger(40), edits[0].Range.End.Character)
			}
		}
	})

	It("warns about unused script variables and offers removal", func() {
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := analyzeDocumentAt(`|===|
string unused = "Ada";
string name = "Grace";
|===|
[output = data]
{
  result: name;
}`, documentPath)

		if tAssert.Len(snapshot.diagnostics, 1) {
			diagnostic := snapshot.diagnostics[0]
			tAssert.Contains(diagnostic.Message, `script variable "unused" is never used`)
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *diagnostic.Severity)
			tAssert.Equal(string(diagnosticDeclarationUnusedVariable), requireDiagnosticCode(diagnostic))

			action := requireCodeAction(snapshot, protocol.DocumentUri(fileURI(documentPath)), diagnostic.Range, "Remove unused variable")
			edits := action.Edit.Changes[protocol.DocumentUri(fileURI(documentPath))]
			if tAssert.Len(edits, 1) {
				tAssert.Equal(``, edits[0].NewText)
				tAssert.Equal(protocol.UInteger(1), edits[0].Range.Start.Line)
				tAssert.Equal(protocol.UInteger(0), edits[0].Range.Start.Character)
				tAssert.Equal(protocol.UInteger(2), edits[0].Range.End.Line)
				tAssert.Equal(protocol.UInteger(0), edits[0].Range.End.Character)
			}
		}
	})

	It("translates mixed array literal errors in script declarations into token-scoped diagnostics", func() {
		snapshot := analyzeDocument(`|===|
array<int> foo = ["4", 6];
|===|
[output = data]
{
  result: 1;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, "array literal has mixed element types")
			tAssert.Equal(protocol.UInteger(1), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(11), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticTypeMixedArrayLiteral), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
	})

	It("translates schema output value exports into schema-field diagnostics", func() {
		snapshot := analyzeDocument(`|===|
type Name: string;
schema User: { name: Name; age: int; };
int local = 1;
|===|
[output = schema]
{
  Name: Name;
  User: User;
  foo: local;
}`)

		diagnostic, ok := lo.Find(snapshot.diagnostics, func(diagnostic protocol.Diagnostic) bool {
			return requireDiagnosticCode(diagnostic) == string(diagnosticTypeInvalidOutputSchemaField)
		})
		if tAssert.True(ok) {
			tAssert.Contains(diagnostic.Message, "invalid field type")
			tAssert.Equal(protocol.UInteger(9), diagnostic.Range.Start.Line)
		}
	})

	It("translates data output type exports into value diagnostics", func() {
		snapshot := analyzeDocument(`|===|
type Name: string;
schema User: { name: string; };
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
schema Point: { x: int; y: int; };
schema Plot: { points: array<Point>; };
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
schema User: { name: string; };
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

	It("warns when script variables are present in schema output mode", func() {
		snapshot := analyzeDocument(`|===|
schema User: { name: string; };
string value = "Ada";
|===|
[output = schema]
{
  User: User;
}`)

		if tAssert.Len(snapshot.diagnostics, 1) {
			tAssert.Contains(snapshot.diagnostics[0].Message, `script variable "value" is ignored`)
			tAssert.Equal(protocol.DiagnosticSeverityWarning, *snapshot.diagnostics[0].Severity)
			tAssert.Equal(protocol.UInteger(2), snapshot.diagnostics[0].Range.Start.Line)
			tAssert.Equal(protocol.UInteger(7), snapshot.diagnostics[0].Range.Start.Character)
			tAssert.Equal(string(diagnosticDirectiveSchemaOutputVariableIgnored), requireDiagnosticCode(snapshot.diagnostics[0]))
		}
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
schema User: { name: string; };
Us`, "", protocol.Position{Line: 2, Character: 2})

		tAssert.True(snapshot.recovered)
		tAssert.Contains(declarationNames(snapshot), "User")
	})
})
