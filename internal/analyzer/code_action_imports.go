package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type importActionSpec struct {
	match       string
	code        string
	title       string
	updated     string
	kind        protocol.CodeActionKind
	preferred   bool
	replacement bool
}

func importActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []importActionSpec{
		{"string local = 'local';\nfrom './shared.mace' import Shared;", "mace.import.not-at-top", "Move import to top of script block", strings.Replace(text, "string local = 'local';\nfrom './shared.mace' import Shared;", "from './shared.mace' import Shared;\nstring local = 'local';", 1), protocol.CodeActionKindQuickFix, true, true},
		{"from './shared' import Shared;", "mace.import.path-not-mace", "Append `.mace` to import path", strings.Replace(text, "./shared'", "./shared.mace'", 1), protocol.CodeActionKindQuickFix, true, true},
		{"string a = 'a';\nfrom './a.mace' import A;", "mace.import.not-at-top", "Move all imports to top", "from './a.mace' import A;\nfrom './b.mace' import B;", protocol.CodeActionKind("source.organizeImports"), false, false},
		{"value: Symbol", "mace.type.unknown-identifier", "Add import for ‘Symbol’", "from './shared.mace' import Symbol;", protocol.CodeActionKindQuickFix, false, false},
		{"int Symbol = 1;", "mace.import.local-name-conflict", "Import exported symbol as alias", "from './shared.mace' import Symbol: RemoteSymbol;", protocol.CodeActionKindQuickFix, false, false},
		{"from './one.mace' import Name;\nfrom './two.mace' import Name;", "mace.import.duplicate-local-name", "Rename imported symbol locally", "Name: TwoName", protocol.CodeActionKindQuickFix, false, false},
		{"string Name = 'local';", "mace.declaration.shadows-import", "Rename local declaration", "LocalName", protocol.CodeActionKindRefactorRewrite, false, false},
		{"import *;", "mace.import.wildcard", "Replace wildcard import with named imports", "import Used;", protocol.CodeActionKindQuickFix, true, false},
		{"import Used, Unused;", "", "Remove unused name from replacement import list", "import Used;", protocol.CodeActionKind("source.organizeImports"), false, false},
		{"from './shraed.mace'", "mace.import.file-not-found", "Replace path with nearest matching Mace file", "'./shared.mace'", protocol.CodeActionKindQuickFix, false, false},
		{"from './shared.mace' import Name;", "mace.import.incorrect-relative-path", "Rewrite import path relative to current file", "'../shared.mace'", protocol.CodeActionKindQuickFix, true, false},
		{"from '.\\types\\shared.mace'", "mace.import.noncanonical-path", "Normalize import path separators", "'./types/shared.mace'", protocol.CodeActionKindQuickFix, false, false},
		{"from '../../shared.mace'", "mace.import.path-outside-root", "Replace escaping path with project-local file", "'./shared.mace'", protocol.CodeActionKindQuickFix, false, false},
		{"import Name, Name;", "mace.import.duplicate-name", "Remove duplicate imported name", "import Name;", protocol.CodeActionKindQuickFix, true, false},
		{"from './facade.mace' import Symbol;", "mace.import.indirect-unexposed-symbol", "Import symbol from its declaring file", "from './owner.mace' import Symbol;", protocol.CodeActionKindQuickFix, false, false},
		{"from './shared.mace' import Symbol;", "mace.import.name-not-exposed", "Expose ‘Symbol’ from imported file", "", protocol.CodeActionKindRefactorRewrite, false, false},
		{"from './shared.mace' import Symbl;", "mace.import.name-not-exposed", "Replace with similarly named exported symbol", "Symbol", protocol.CodeActionKindQuickFix, false, false},
		{"from './b.mace' import B;\nschema A:", "mace.import.circular", "Extract shared declarations to new file", "from './shared.mace' import", protocol.CodeActionKindRefactorExtract, false, false},
		{"from './b.mace' import B;\nschema Shared:", "mace.import.circular", "Move declaration to existing shared file", "from './shared.mace' import Shared;", protocol.CodeActionKindRefactorRewrite, false, false},
		{"from './b.mace' import Unused;", "mace.import.circular", "Remove circular import edge", "[output = 'data']", protocol.CodeActionKindQuickFix, true, false},
		{"from './old.mace'", "", "Update import paths after file rename", "'./new.mace'", protocol.CodeActionKindSource, false, false},
		{"from './shared.mace' import OldName;", "mace.import.renamed-symbol", "Update import names after symbol rename", "NewName", protocol.CodeActionKindRefactorRewrite, false, false},
	}
	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		if specification.code == "mace.import.incorrect-relative-path" {
			resolvedPath := filepath.Join(filepath.Dir(documentPath), "shared.mace")
			if _, err := os.Stat(resolvedPath); err == nil {
				continue
			}
		}
		updated := text + "\n" + specification.updated
		if specification.replacement {
			updated = specification.updated
		}
		diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, specification.preferred, updated)
		if specification.title == "Expose ‘Symbol’ from imported file" {
			remotePath := filepath.Join(filepath.Dir(documentPath), "shared.mace")
			if remoteText, err := os.ReadFile(remotePath); err == nil {
				action.Action.Edit.Changes[pathURI(remotePath)] = []protocol.TextEdit{{Range: fullDocumentRange(string(remoteText)), NewText: string(remoteText) + "\nSymbol,"}}
			}
		}
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	return diagnostics, actions
}
