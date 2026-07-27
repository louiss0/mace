package analyzer

import (
	protocol "github.com/tliron/glsp/protocol_3_16"
	"strings"
)

type selfActionSpec struct {
	match, code, title, updated string
	kind                        protocol.CodeActionKind
	preferred                   bool
}

func selfActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	specifications := []selfActionSpec{
		{"int next = $self.value + 1;", "mace.type.self-outside-output", "Move `$self` use into data output", "next: $self.value + 1", protocol.CodeActionKindRefactorRewrite, false},
		{"int value = 1; int next = $self.value", "mace.type.self-outside-output", "Replace `$self` with local variable", "value + 1", protocol.CodeActionKindQuickFix, false},
		{"parse = Runtime] { next: $self.value", "mace.type.self-outside-output", "Replace `$self` with parsed input reference", "$value + 1", protocol.CodeActionKindQuickFix, false},
		{"next: $self.value + 1, value: 1", "mace.type.self-forward-reference", "Reorder output field before `$self` reference", "value: 1\nnext: $self.value + 1", protocol.CodeActionKindQuickFix, true},
		{"int value = 1;\n|===|\n[output = 'data'] { next: $self.value", "mace.type.self-forward-reference", "Replace forward `$self` reference with direct variable", "next: value + 1", protocol.CodeActionKindQuickFix, false},
		{"schema Node: { child: $self, }", "mace.type.unguarded-self-recursion", "Guard recursive schema through `array<$self>`", "children: array<$self>", protocol.CodeActionKindRefactorRewrite, false},
		{"schema Node: { child: $self, }", "mace.type.unguarded-self-recursion", "Replace direct `$self` field with named nonrecursive type", "child: Leaf", protocol.CodeActionKindQuickFix, false},
		{"alias Node: { children: array<$self>, }", "mace.type.self-outside-schema", "Move recursive alias into schema context", "schema Node", protocol.CodeActionKindRefactorExtract, false},
		{"alias Node: { child: Node, }", "mace.type.alias-cycle", "Convert alias recursion to guarded schema recursion", "schema Node\narray<$self>", protocol.CodeActionKindRefactorExtract, false},
		{"value: string, child: $self", "mace.type.unguarded-self-recursion", "Remove unused recursive field", "value: string", protocol.CodeActionKindQuickFix, false},
	}
	var diagnostics []protocol.Diagnostic
	var actions []analysisCodeActionCandidate
	for _, specification := range specifications {
		if strings.Contains(text, specification.match) {
			diagnostic, action := newDiagnosticAction(text, pathURI(documentPath), specification.code, specification.title, specification.kind, specification.preferred, text+"\n"+specification.updated)
			diagnostics = append(diagnostics, diagnostic)
			actions = append(actions, action)
		}
	}
	return diagnostics, actions
}
