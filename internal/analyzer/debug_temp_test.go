package analyzer

import (
	"os"
	"testing"

	protocol "github.com/tliron/glsp/protocol_3_16"
)

func TestDebugCompletion(t *testing.T) {
	text := `[output = data, schema = User, schema_file = "./schema.mace"]
{
  value: "A";
}
`
	pos := protocol.Position{Line: 2, Character: 10}
	if err := os.WriteFile("/tmp/schema.mace", []byte(`[output = schema]
{
  User: { value: string; };
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, ok := stringLiteralCompletionContext(text, pos)
	t.Logf("ctx=%+v ok=%v", ctx, ok)
	file, ok := completionFileWithExpressionPlaceholder(text, ctx.start, ctx.end)
	t.Logf("file=%v ok=%v start=%d end=%d", file != nil, ok, ctx.start, ctx.end)
	if file != nil {
		model := buildCompletionModel(*file, "/tmp", "/tmp", map[string]completionModel{})
		value, path, ok := placeholderOutputCompletionType(*file, model)
		t.Logf("placeholderOutput value=%v path=%v ok=%v", value, path, ok)
	}
	items, handled := outputInitializerCompletionItems(document{text: text, analysis: analysisSnapshot{}}, protocol.DocumentUri("file:///tmp/test.mace"), pos)
	t.Logf("output items=%d handled=%v", len(items), handled)
	items2, handled2 := stringLiteralInitializerCompletionItems(document{text: text, analysis: analysisSnapshot{}}, protocol.DocumentUri("file:///tmp/test.mace"), pos, true)
	t.Logf("string items=%d handled=%v", len(items2), handled2)
	_ = items2
}
