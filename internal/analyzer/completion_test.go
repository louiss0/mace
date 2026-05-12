package analyzer

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/parser/ast"
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

	It("resolves enum field types for output schema placeholders", func() {
		text := `|===|
enum Fruit: string {
  Apple,
  Strawberry = "strawberry",
};
schema Basket: { favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
{
  favorite_fruit:
}`

		file, ok := completionFileWithPlaceholder(text, protocol.Position{
			Line:      9,
			Character: uint32(len(`  favorite_fruit: `)),
		})
		tAssert.True(ok)
		if !ok {
			return
		}

		model := buildCompletionModel(*file, filepath.Dir("document.mace"), filepath.Dir("document.mace"), map[string]completionModel{})
		expectedType, path, ok := placeholderOutputCompletionType(*file, model)
		tAssert.True(ok)
		tAssert.Equal([]string{"favorite_fruit"}, path)

		namedType, ok := expectedType.(ast.NamedType)
		tAssert.True(ok)
		if ok {
			tAssert.Equal("Fruit", namedType.Name)
		}
	})

	It("keeps typed output completions alongside $self in output schema fields", func() {
		text := `|===|
enum Fruit: string {
  Apple,
  Strawberry = "strawberry",
};
schema Basket: { favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
{
  favorite_fruit: 
}`

		position := protocol.Position{
			Line:      9,
			Character: uint32(len(`  favorite_fruit: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, "Fruit.Apple")
		tAssert.Contains(labels, "Fruit.Strawberry")
	})

	It("suggests enum values for output block schema fields", func() {
		text := `|===|
enum Fruit: string {
  Apple,
  Strawberry = "strawberry",
};
schema Basket: { favorite_fruit: Fruit; };
|===|
[output = data, schema = Basket]
{
  favorite_fruit:
}`

		position := protocol.Position{
			Line:      9,
			Character: uint32(len(`  favorite_fruit: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, "Fruit.Apple")
		tAssert.Contains(labels, "Fruit.Strawberry")
	})

	It("suggests variant members for nested output schema aliases", func() {
		text := `|===|
enum Role: string {
  Admin,
};
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
			Line:      12,
			Character: uint32(len(`    value: `)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Contains(labels, "$self")
		tAssert.Contains(labels, "Role.Admin")
		tAssert.Contains(labels, `{ name: "" }`)
	})

	It("suggests enum values for incomplete enum variable initializers", func() {
		text := `|===|
	enum Fruit: string {
	  Apple,
	  Strawberry = "strawberry",
	};
	Fruit selected =`

		position := protocol.Position{
			Line:      5,
			Character: uint32(len(`Fruit selected =`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"Fruit.Apple", "Fruit.Strawberry"}, labels)
	})

	It("suggests enum members after a dot for local enums", func() {
		text := `|===|
	enum Personality: string {
	  is_type,
	  has_defaults,
	};
	Personality value = Personality.`

		position := protocol.Position{
			Line:      5,
			Character: uint32(len(`	Personality value = Personality.`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"has_defaults", "is_type"}, labels)
		tAssert.Equal(`enum member Personality.has_defaults = "has_defaults"`, lo.FromPtr(items[0].Detail))
	})

	It("suggests enum members after a dot for union enum aliases", func() {
		text := `|===|
	enum Access: int {
	  Read,
	  Write,
	};
	enum Feature: int {
	  Write,
	  Execute,
	};
	type Permission: union[Access, Feature];
	Permission value = Permission.`

		position := protocol.Position{
			Line:      10,
			Character: uint32(len(`	Permission value = Permission.`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"Execute", "Read", "Write"}, labels)
	})

	It("shows implicit int enum values in completion details", func() {
		text := `|===|
	enum Status: int {
	  Pending,
	  Running,
	};
	Status current = Status.`

		position := protocol.Position{
			Line:      5,
			Character: uint32(len(`	Status current = Status.`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"Pending", "Running"}, labels)
		tAssert.Equal("enum member Status.Pending = 0", lo.FromPtr(items[0].Detail))
		tAssert.Equal("enum member Status.Running = 1", lo.FromPtr(items[1].Detail))
	})

	It("shows implicit float enum values in completion details", func() {
		text := `|===|
	enum Status: float {
	  Pending,
	  Running,
	};
	Status current = Status.`

		position := protocol.Position{
			Line:      5,
			Character: uint32(len(`	Status current = Status.`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"Pending", "Running"}, labels)
		tAssert.Equal("enum member Status.Pending = 0.0", lo.FromPtr(items[0].Detail))
		tAssert.Equal("enum member Status.Running = 0.1", lo.FromPtr(items[1].Detail))
	})

	It("uses imported enum aliases in member completion details", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-imported-enum-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `|===|
enum InternalStatus: int {
  Pending,
  Running,
};
|===|
[output = schema]
{
  Status: InternalStatus;
}`)

		text := `|===|
from "./shared.mace" import Status;
Status current = Status.`

		position := protocol.Position{
			Line:      2,
			Character: uint32(len(`Status current = Status.`)),
		}
		documentPath := filepath.Join(workspace, "consumer.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"Pending", "Running"}, labels)
		tAssert.Equal("enum member Status.Pending = 0", lo.FromPtr(items[0].Detail))
		tAssert.Equal("enum member Status.Running = 1", lo.FromPtr(items[1].Detail))
	})

	It("returns correct importable symbol kinds from an exported output block", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-importable-symbols-*")
		tAssert.NoError(err)

		writeAnalysisFile(workspace, "shared.mace", `|===|
enum Role: string { Admin };
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
		tAssert.Equal(protocol.CompletionItemKindEnum, kinds["role"])
		tAssert.Equal(protocol.CompletionItemKindClass, kinds["label"])
		tAssert.Equal(protocol.CompletionItemKindClass, kinds["count"])
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
