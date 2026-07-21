package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func directiveActionAnalysis(text string, documentPath string) ([]protocol.Diagnostic, []analysisCodeActionCandidate) {
	uri := pathURI(documentPath)
	diagnostics := []protocol.Diagnostic{}
	actions := []analysisCodeActionCandidate{}

	add := func(code string, title string, kind protocol.CodeActionKind, preferred bool, updated string) {
		diagnostic, action := newDiagnosticAction(text, uri, code, title, kind, preferred, updated)
		diagnostics = append(diagnostics, diagnostic)
		actions = append(actions, action)
	}

	if strings.HasPrefix(strings.TrimSpace(text), "[]") {
		mode := "data"
		if strings.Contains(text, "User: {") {
			mode = "schema"
		}
		add("mace.directive.missing-output", "Add `output = '"+mode+"'` directive", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "[]", "[output = '"+mode+"']", 1))
	}
	if strings.Contains(text, "output = 'data', output = 'data'") {
		add("mace.directive.duplicate-key", "Remove duplicate output directive", protocol.CodeActionKindQuickFix, true, strings.Replace(text, ", output = 'data'", "", 1))
	}
	if strings.Contains(text, "output = 'schema'") && strings.Contains(text, "schema_file =") && strings.Contains(text, "parse_file =") {
		start := strings.Index(text, "[")
		end := strings.Index(text, "]")
		if start >= 0 && end > start {
			add("mace.directive.data-only-in-schema-output", "Remove data-only directives from schema output", protocol.CodeActionKindQuickFix, true, text[:start]+"[output = 'schema']"+text[end+1:])
		}
	}
	if strings.Contains(text, "scheam = User") {
		add("mace.directive.unknown-key", "Remove unknown directive", protocol.CodeActionKindQuickFix, true, strings.Replace(text, ", scheam = User", "", 1))
	}
	if strings.Contains(text, "outpt = 'data'") {
		add("mace.directive.unknown-key", "Rename directive to nearest known directive", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "outpt", "output", 1))
	}
	if strings.Contains(text, "output = 'schema', schema = User") && strings.Contains(text, "name: 'Mace'") {
		add("mace.directive.data-only-in-schema-output", "Switch output to data mode", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "output = 'schema'", "output = 'data'", 1))
	}
	outputText := text
	if outputStart := strings.Index(text, "[output"); outputStart >= 0 {
		outputText = text[outputStart:]
	}
	if strings.Contains(outputText, "output = 'data'") && strings.Contains(outputText, "name: string") && strings.Contains(outputText, "age: int") {
		add("mace.directive.schema-shaped-data-output", "Switch output to schema mode", protocol.CodeActionKindRefactorRewrite, false, strings.Replace(text, "output = 'data'", "output = 'schema'", 1))
	}
	if strings.Contains(text, "schema = Usre") && strings.Contains(text, "schema User:") {
		add("mace.directive.unknown-schema-name", "Select matching local schema", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "schema = Usre", "schema = User", 1))
	}
	if strings.Contains(text, "schema = User") && !strings.Contains(text, "schema_file =") && !strings.Contains(text, "schema User:") {
		add("mace.directive.schema-file-required", "Add `schema_file` for selected schema", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "schema = User", "schema = User, schema_file = './schema.mace'", 1))
	}
	if strings.Contains(text, "schema_file = './schema.mace', schema = Usre") {
		add("mace.directive.unknown-schema-name", "Select schema from loaded schema file", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "schema = Usre", "schema = User", 1))
	}
	if strings.Contains(text, "schema_file = './schema.mace', schema = User") {
		add("mace.directive.redundant-schema", "Remove redundant `schema` directive", protocol.CodeActionKindQuickFix, false, strings.Replace(text, ", schema = User", "", 1))
	}
	if strings.Contains(text, "parse = Runtime") && !strings.Contains(text, "parse_file =") {
		add("mace.directive.parse-file-required", "Add `parse_file` for parse schema", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "parse = Runtime", "parse = Runtime, parse_file = './runtime.mace'", 1))
	}
	if strings.Contains(text, "parse_file = './runtime.mace', parse = Runtim") {
		add("mace.directive.unknown-parse-name", "Select parse schema from loaded file", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "parse = Runtim", "parse = Runtime", 1))
	}
	if strings.Contains(text, "parse = Runtime, parse_file = './runtime.mace'] {}") {
		add("mace.directive.incompatible-parse", "Remove incompatible parse directive", protocol.CodeActionKindQuickFix, false, strings.Replace(text, ", parse = Runtime", "", 1))
	}
	if strings.Contains(text, "names: []") {
		add("mace.type.untyped-output-collection", "Generate output schema from data fields", protocol.CodeActionKindRefactorExtract, false, "|===|\nschema Output: { names: array<string>, };\n|===|\n"+text)
	}
	if strings.Contains(text, "schema Output:") && strings.Contains(text, "[output = 'data']") {
		add("mace.directive.generated-schema-not-selected", "Attach generated schema to output", protocol.CodeActionKindQuickFix, false, strings.Replace(text, "[output = 'data']", "[output = 'data', schema = Output]", 1))
	}
	if strings.Contains(text, "[output = 'data'] { name: 'Mace', }") && !strings.Contains(text, "schema Output:") {
		add("mace.type.output-needs-reusable-schema", "Create schema file from output shape", protocol.CodeActionKindRefactorExtract, false, strings.Replace(text, "[output = 'data']", "[output = 'data', schema_file = './output.schema.mace']", 1))
	}
	if strings.HasPrefix(strings.TrimSpace(text), "output_doc") {
		add("mace.documentation.output-directives-missing", "Add directive list for output documentation", protocol.CodeActionKindQuickFix, true, "[output = 'data']\n"+text)
	}

	return diagnostics, actions
}
