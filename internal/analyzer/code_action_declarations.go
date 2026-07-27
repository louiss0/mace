package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

type declarationActionSpec struct {
	match       string
	code        string
	title       string
	updatedText string
	kind        protocol.CodeActionKind
	preferred   bool
	replacement bool
}

func declarationActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []declarationActionSpec{
		{match: "message: greting", code: "mace.type.unknown-identifier", title: "Replace unknown value with ‘greeting’", updatedText: strings.Replace(text, "greting", "greeting", 1), kind: protocol.CodeActionKindQuickFix, replacement: true},
		{match: "string name;", code: "mace.declaration.variable-missing-initializer", title: "Insert initializer", updatedText: "string name = '';", kind: protocol.CodeActionKindQuickFix},
		{match: "name = 'Mace';", code: "mace.declaration.variable-missing-type", title: "Insert explicit variable type", updatedText: "string name", kind: protocol.CodeActionKindQuickFix},
		{match: "int name = 'Mace'", code: "mace.type.initializer-type-mismatch", title: "Change declaration type to inferred value type", updatedText: "string name", kind: protocol.CodeActionKindQuickFix},
		{match: "float count = 1", code: "mace.type.initializer-type-mismatch", title: "Convert initializer to declared type family", updatedText: "1.0", kind: protocol.CodeActionKindQuickFix, preferred: true},
		{match: "string name = 'first';\nstring name = 'second'", code: "mace.declaration.duplicate-variable", title: "Rename duplicate declaration", updatedText: "name_2", kind: protocol.CodeActionKindRefactorRewrite},
		{match: "string name = 'Mace';\nstring name = 'Mace'", code: "mace.declaration.duplicate-variable", title: "Remove duplicate declaration", updatedText: "string name = 'Mace';", kind: protocol.CodeActionKindQuickFix},
		{match: "Usernme name", code: "mace.declaration.unknown-type-reference", title: "Replace unknown type with nearest type", updatedText: "Username name", kind: protocol.CodeActionKindQuickFix},
		{match: "value: name,", code: "mace.type.unknown-identifier", title: "Create variable ‘name’", updatedText: "|===|\nstring name = '';", kind: protocol.CodeActionKindRefactorExtract},
		{match: "Name name = 'Mace'", code: "mace.declaration.unknown-type-reference", title: "Create type alias ‘Name’", updatedText: "alias Name: string;", kind: protocol.CodeActionKindRefactorExtract},
		{match: "Name name = { value: 'Mace'", code: "mace.declaration.unknown-type-reference", title: "Create schema ‘Name’ from record literal", updatedText: "schema Name\nvalue: string", kind: protocol.CodeActionKindRefactorExtract},
		{match: "parse = Runtime] { env: env", code: "mace.type.parsed-input-missing-prefix", title: "Prefix parsed input reference with `$`", updatedText: "$env", kind: protocol.CodeActionKindQuickFix, preferred: true},
		{match: "string env = 'dev'", code: "mace.type.local-variable-input-prefix", title: "Remove `$` from local variable reference", updatedText: "env: env", kind: protocol.CodeActionKindQuickFix, preferred: true},
		{match: "User user = { name: 'Mace'", code: "mace.declaration.unknown-type-reference", title: "Import unresolved type", updatedText: "from './types.mace' import User;", kind: protocol.CodeActionKindQuickFix},
		{match: "value: Shared", code: "mace.type.unknown-identifier", title: "Import unresolved value", updatedText: "from './shared.mace' import Shared;", kind: protocol.CodeActionKindQuickFix},
		{match: "alias Name: string;\nName name", code: "mace.type.alias-cycle", title: "Inline type alias", updatedText: "string name", kind: protocol.CodeActionKindRefactorRewrite},
		{match: "{ name: string, age: int, } user", code: "mace.refactor.repeated-inline-type", title: "Extract inline type into alias", updatedText: "alias User\nUser user", kind: protocol.CodeActionKindRefactorExtract},
	}

	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if !strings.Contains(text, specification.match) {
			continue
		}
		updatedText := text + "\n" + specification.updatedText
		if specification.replacement {
			updatedText = specification.updatedText
		}
		diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, specification.preferred, updatedText)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}
	return diagnostics, actions
}
