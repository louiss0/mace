package analyzer

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("completion analysis", func() {
	It("suggests $self in an empty output expression", func() {
		text := `[output = data]
{
  base: 1;
  result: 
}`

		position := protocol.Position{
			Line:      3,
			Character: uint32(len(`  result: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "$self")
	})

	It("suggests $self after typing a dollar in the output block", func() {
		text := `[output = data]
{
  result: $
}`

		position := protocol.Position{
			Line:      2,
			Character: uint32(len(`  result: $`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"$self"}, labels)
	})

	It("replaces the typed dollar when completing $self", func() {
		text := `[output = data]
{
  result: $
}`

		position := protocol.Position{
			Line:      2,
			Character: uint32(len(`  result: $`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		tAssert.Len(items, 1)

		edit, ok := items[0].TextEdit.(protocol.TextEdit)
		tAssert.True(ok)
		if !ok {
			return
		}

		tAssert.Equal("$self", edit.NewText)
		tAssert.Equal(protocol.Range{
			Start: protocol.Position{Line: 2, Character: uint32(len(`  result: `))},
			End:   position,
		}, edit.Range)
	})

	It("suggests $self after earlier self references on the same line", func() {
		text := `[output = data]
{
  foo: 1;
  result: (true ? $self.foo : $)
}`

		position := protocol.Position{
			Line:      3,
			Character: uint32(len(`  result: (true ? $self.foo : $`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"$self"}, labels)
	})

	It("keeps typed output completions alongside $self in output schema fields", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
{
  favorite_fruit: 
}`

		position := protocol.Position{
			Line:      6,
			Character: uint32(len(`  favorite_fruit: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests parse schema fields as output variables", func() {
		text := `|===|
schema Runtime: { env: string; region: string; };
|===|
[output = data, parse = Runtime]
{
  result: 
}`

		position := protocol.Position{
			Line:      5,
			Character: uint32(len(`  result: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "env")
		tAssert.Contains(labels, "region")
	})

	It("suggests parse_file output schema fields as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-*")
		tAssert.NoError(err)
		defer func() {
			_ = os.RemoveAll(workspace)
		}()

		runtimePath := filepath.Join(workspace, "runtime.mace")
		tAssert.NoError(os.WriteFile(runtimePath, []byte(`[output = schema]
{
  user: { name: string; home: { street: string; city: string; }; };
  app: { env: string; region: string; };
}`), 0o644))
		documentPath := filepath.Join(workspace, "document.mace")
		text := `[output = data, parse_file = "./runtime.mace"]
{
  result: 
}`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		position := protocol.Position{
			Line:      2,
			Character: uint32(len(`  result: `)),
		}
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "user")
		tAssert.Contains(labels, "app")
		tAssert.NotContains(labels, "name")
		tAssert.NotContains(labels, "home")
	})

	It("suggests parse_file output schema field members as output variables", func() {
		workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-members-*")
		tAssert.NoError(err)
		defer func() {
			_ = os.RemoveAll(workspace)
		}()

		runtimePath := filepath.Join(workspace, "runtime.mace")
		tAssert.NoError(os.WriteFile(runtimePath, []byte(`[output = schema]
{
  user: { name: string; home: { street: string; city: string; }; };
}`), 0o644))
		documentPath := filepath.Join(workspace, "document.mace")
		text := `[output = data, parse_file = "./runtime.mace"]
{
  result: user.
}`
		tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
		position := protocol.Position{
			Line:      2,
			Character: uint32(len(`  result: user.`)),
		}
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "name")
		tAssert.Contains(labels, "home")
	})

	It("suggests choice values for output block schema fields", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
{
  favorite_fruit:
}`

		position := protocol.Position{
			Line:      6,
			Character: uint32(len(`  favorite_fruit: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests choice values for script variable initializers", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 Fruit favorite =
|===|
[output = data] {}`

		position := protocol.Position{
			Line:      2,
			Character: uint32(len(` Fruit favorite =`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests unquoted choice values inside script strings", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 Fruit favorite = "A
|===|
[output = data] {}`

		position := protocol.Position{
			Line:      2,
			Character: uint32(len(` Fruit favorite = "A`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "Apple")
		tAssert.NotContains(labels, `"Apple"`)
		tAssert.NotContains(labels, "Strawberry")
	})

	It("suggests choice values inside script variable variants", func() {
		text := `|===|
 type Status: choice["pending", "approved"];
 type Label: variant[Status, string];
 Label label =
|===|
[output = data] {}`

		position := protocol.Position{
			Line:      3,
			Character: uint32(len(` Label label =`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, `"approved"`)
		tAssert.Contains(labels, `"pending"`)
	})

	It("suggests choice values for script variable record fields", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit; };
 Basket basket = {
   favorite_fruit:
 };
|===|
[output = data] {}`

		position := protocol.Position{
			Line:      4,
			Character: uint32(len(`   favorite_fruit: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, `"Apple"`)
		tAssert.Contains(labels, `"Strawberry"`)
	})

	It("suggests unquoted choice values inside record field strings", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit; };
 Basket basket = {
   favorite_fruit: "Str
 };
|===|
[output = data] {}`

		position := protocol.Position{
			Line:      4,
			Character: uint32(len(`   favorite_fruit: "Str`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"Strawberry"}, labels)
	})

	It("suggests unquoted choice values inside array element strings", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 array<Fruit> favorites = ["A
|===|
[output = data] {}`

		position := protocol.Position{
			Line:      2,
			Character: uint32(len(` array<Fruit> favorites = ["A`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "Apple")
		tAssert.NotContains(labels, `"Apple"`)
		tAssert.NotContains(labels, "Strawberry")
	})

	It("suggests choice values inside variants while keeping imprecise alternatives", func() {
		text := `|===|
 type Role: choice["Admin", "Member"];
 schema User: { name: string; };
 type Identity: variant[Role, User];
 schema Envelope: { value: Identity; };
 schema Response: { payload: Envelope; };
|===|
[output = data, schema = Response]
{
  payload: {
    value:
  };
}`

		position := protocol.Position{
			Line:      10,
			Character: uint32(len(`    value: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, `"Admin"`)
		tAssert.Contains(labels, `"Member"`)
		tAssert.Contains(labels, `{ name: "" }`)
	})

	It("returns correct importable symbol kinds from an exported output block", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-importable-symbols-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `|===|
 type Role: choice["Admin"];
 schema User: { name: string };
 type Alias: string;
|===|
[output = schema]
{
  user: User;
  role: Role;
  label: Alias;
  count: int;
}`)

		documentPath := filepath.Join(workspace, "consumer.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		symbols, ok := importableSymbols(uri, filepath.Dir(documentPath), "./shared.mace")
		tAssert.True(ok)

		kinds := map[string]protocol.CompletionItemKind{}
		for _, s := range symbols {
			kinds[s.Name] = s.Kind
		}

		tAssert.Equal(protocol.CompletionItemKindStruct, kinds["user"])
		tAssert.Equal(protocol.CompletionItemKindClass, kinds["role"])
		tAssert.Equal(protocol.CompletionItemKindClass, kinds["label"])
		tAssert.Equal(protocol.CompletionItemKindClass, kinds["count"])
	})

	It("returns choice aliases as type importables from an exported output block", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-importable-choice-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `|===|
 type Flavor: choice["Vanilla", "Chocolate"];
|===|
[output = schema]
{
  flavor: Flavor;
}`)

		documentPath := filepath.Join(workspace, "consumer.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		symbols, ok := importableSymbols(uri, filepath.Dir(documentPath), "./shared.mace")
		tAssert.True(ok)
		if !ok || !tAssert.Len(symbols, 1) {
			return
		}

		tAssert.Equal("flavor", symbols[0].Name)
		tAssert.Equal(protocol.CompletionItemKindClass, symbols[0].Kind)
	})

	It("returns data fields as variables from a data output block", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-importable-data-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = data]
{
  name: "Ada";
  age: 30;
}`)

		documentPath := filepath.Join(workspace, "consumer.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		symbols, ok := importableSymbols(uri, filepath.Dir(documentPath), "./shared.mace")
		tAssert.True(ok)

		names := lo.Map(symbols, func(s importableSymbol, _ int) string { return s.Name })
		kinds := lo.Map(symbols, func(s importableSymbol, _ int) protocol.CompletionItemKind { return s.Kind })

		tAssert.Equal([]string{"name", "age"}, names)
		tAssert.Equal([]protocol.CompletionItemKind{protocol.CompletionItemKindVariable, protocol.CompletionItemKindVariable}, kinds)
	})

	It("completes import identifiers from exported output keys", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-import-identifiers-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = data]
{
  name: "Ada";
  age: 30;
}`)

		documentPath := filepath.Join(workspace, "consumer.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		line := `from "./shared.mace" import `
		text := "|===|\n" + line + "\n|===|\n[output = data]\n{}"
		position := protocol.Position{Line: 1, Character: protocol.UInteger(len(line))}
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, uri, position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })

		tAssert.Equal([]string{"age", "name"}, labels)
	})

	It("completes import identifiers after comma separators", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-import-comma-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = data]
{
  name: "Ada";
  age: 30;
}`)

		documentPath := filepath.Join(workspace, "consumer.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		line := `from "./shared.mace" import name, `
		text := "|===|\n" + line + "\n|===|\n[output = data]\n{}"
		position := protocol.Position{Line: 1, Character: protocol.UInteger(len(line))}
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, uri, position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })

		tAssert.Equal([]string{"age", "name"}, labels)
	})

	It("completes script block import identifiers from exported output keys", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-script-import-identifiers-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = data]
{
  name: "Ada";
  age: 30;
}`)

		documentPath := filepath.Join(workspace, "consumer.mace")
		uri := protocol.DocumentUri(fileURI(documentPath))
		text := `|===|
from "./shared.mace" import 
|===|
[output = data]
{}`
		position := protocol.Position{Line: 1, Character: protocol.UInteger(len(`from "./shared.mace" import `))}
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, uri, position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })

		tAssert.Equal([]string{"age", "name"}, labels)
	})

	It("completes $self fields for parent-relative imports", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-parent-import-self-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = data]
{
  base: {
    name: "Ada";
  };
}`)

		documentDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(documentDir, 0o755))
		documentPath := filepath.Join(documentDir, "consumer.mace")
		text := `|===|
from "../shared.mace" import base;
|===|
[output = data]
{
  base: base;
  result: $self.base.
}`
		position := protocol.Position{Line: 6, Character: protocol.UInteger(len(`  result: $self.base.`))}
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })

		tAssert.Equal([]string{"name"}, labels)
	})

	It("keeps schema_file completion root-bounded", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-schema-file-root-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `[output = schema]
{
  User: string;
}`)

		documentDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(documentDir, 0o755))
		documentPath := filepath.Join(documentDir, "consumer.mace")
		line := `[output = data, schema_file = "../`
		position := protocol.Position{Line: 0, Character: protocol.UInteger(len(line))}
		snapshot := AnalyzeCompletionContext(line, documentPath, position)

		items := CompletionItems(line, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })

		tAssert.NotContains(labels, "../shared.mace")
	})
})
