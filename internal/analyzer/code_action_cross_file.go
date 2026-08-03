package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type crossFileActionSpec struct {
	match      string
	code       string
	title      string
	kind       protocol.CodeActionKind
	localText  string
	remotePath string
	remoteText string
}

func crossFileActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []crossFileActionSpec{
		{"from './shared.mace' import OldName;", "mace.workspace.rename", "Rename declaration and update imports", protocol.CodeActionKindRefactorRewrite, "NewName", "shared.mace", "NewName"},
		{"from './shared.mace' import OldName;", "mace.workspace.rename", "Rename exported output field and update importers", protocol.CodeActionKindRefactorRewrite, "NewName", "shared.mace", "NewName:"},
		{"schema OldUser:", "mace.workspace.rename", "Rename schema and update `schema` directives", protocol.CodeActionKindRefactorRewrite, "schema User\nschema = User", "", ""},
		{"schema OldRuntime:", "mace.workspace.rename", "Rename parse schema and update `parse` directives", protocol.CodeActionKindRefactorRewrite, "schema Runtime\nparse = Runtime", "", ""},
		{"User user = { old_field:", "mace.workspace.rename", "Rename schema field and update data outputs", protocol.CodeActionKindRefactorRewrite, "new_field", "", ""},
		{"copied: $self.old_field", "mace.workspace.rename", "Rename schema field and update `$self` paths", protocol.CodeActionKindRefactorRewrite, "new_field\n$self.new_field", "", ""},
		{"parse = Runtime] { value: $old_field", "mace.workspace.rename", "Rename schema field and update parsed-input paths", protocol.CodeActionKindRefactorRewrite, "new_field\n$new_field", "", ""},
		{"schema_doc User { fields: { old_field:", "mace.workspace.rename", "Rename schema field and update `schema_doc.fields`", protocol.CodeActionKindRefactorRewrite, "new_field", "", ""},
		{"alias OldValue:", "mace.workspace.rename", "Rename variant or choice alias and update match patterns", protocol.CodeActionKindRefactorRewrite, "Value", "", ""},
		{"from './shared.mace' import Value;", "mace.workspace.variant-domain-change", "Add a variant member and update every exhaustive match", protocol.CodeActionKindRefactorRewrite, "boolean =>", "shared.mace", "variant[string, int, boolean]"},
		{"from './shared.mace' import Value;", "mace.workspace.variant-domain-change", "Remove a variant member and remove unreachable match arms", protocol.CodeActionKindRefactorRewrite, "int => 'i'", "shared.mace", "variant[string]"},
		{"from './shared.mace' import Mode;", "mace.workspace.choice-domain-change", "Add a choice member and update every exhaustive choice match", protocol.CodeActionKindRefactorRewrite, "'test' =>", "shared.mace", "choice['dev', 'prod', 'test']"},
		{"from './shared.mace' import Mode;", "mace.workspace.choice-domain-change", "Remove a choice member and remove invalid match arms", protocol.CodeActionKindRefactorRewrite, "'dev' => 1", "shared.mace", "choice['dev']"},
		{"User user = { old_field: '1'", "mace.workspace.schema-field-change", "Change a schema field type and update incompatible outputs", protocol.CodeActionKindRefactorRewrite, "old_field: 1", "shared.mace", "old_field: int"},
		{"value: user.old_field", "mace.workspace.schema-field-change", "Mark a schema field optional and convert plain access to `?.`", protocol.CodeActionKindRefactorRewrite, "user?.old_field", "shared.mace", "old_field?: string"},
		{"User user = {}", "mace.workspace.schema-field-change", "Make a schema field required and identify outputs missing it", protocol.CodeActionKindRefactorRewrite, "old_field: ''", "shared.mace", "old_field: string"},
		{"user.old_field.first.second", "mace.workspace.record-depth-change", "Increase record nesting and revalidate member paths", protocol.CodeActionKindRefactorRewrite, "old_field.first.second", "shared.mace", "record<record<string>>"},
		{"value: OldName,", "mace.workspace.export-change", "Change an exported symbol and revalidate all importers", protocol.CodeActionKindRefactorRewrite, "value: 0", "shared.mace", "OldName: 0"},
		{"schema User: { name: string, }; User user", "mace.workspace.move-declaration", "Move declaration to another Mace file and add imports", protocol.CodeActionKindRefactorExtract, "from './types.mace' import User;", "types.mace", "schema User"},
		{"from './b.mace' import B;", "mace.import.circular", "Extract declarations into a shared file to break a cycle", protocol.CodeActionKindRefactorExtract, "from './shared.mace' import", "shared.mace", "schema"},
		{"[output = 'schema'] { User: { name: string, }, }", "mace.workspace.extract-schema-file", "Create a schema file from an inline schema", protocol.CodeActionKindRefactorExtract, "schema_file = './user.mace'", "user.mace", "User: { name: string, }"},
		{"from './shared.mace' import User;", "mace.import.name-not-exposed", "Expose a declaration from its owning file", protocol.CodeActionKindRefactorRewrite, "", "shared.mace", "User: User"},
		{"from './old/shared.mace' import OldName;", "", "Update references after moving or renaming a Mace file", protocol.CodeActionKindSource, "from './new/shared.mace'", "", ""},
	}

	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		if specification.title == "Expose a declaration from its owning file" {
			remotePath := filepath.Join(filepath.Dir(documentPath), specification.remotePath)
			if _, err := os.Stat(remotePath); err != nil {
				continue
			}
		}
		diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, false, text+"\n"+specification.localText)
		if specification.remotePath != "" {
			remotePath := filepath.Join(filepath.Dir(documentPath), specification.remotePath)
			remoteText, err := os.ReadFile(remotePath)
			if err == nil {
				action.Action.Edit.Changes[pathURI(remotePath)] = []protocol.TextEdit{{Range: fullDocumentRange(string(remoteText)), NewText: string(remoteText) + "\n" + specification.remoteText}}
			}
		}
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	return diagnostics, actions
}
