package analyzer

import (
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

	It("resolves enum field types for output schema placeholders", func() {
		text := `|===|
enum Fruit: string {
  Apple,
  Strawberry = "strawberry",
}
schema Basket = { favorite_fruit: Fruit; };
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

		model := buildCompletionModel(*file, filepath.Dir("document.mace"), map[string]completionModel{})
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
}
schema Basket = { favorite_fruit: Fruit; };
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
}
schema Basket = { favorite_fruit: Fruit; };
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

	It("suggests enum values for incomplete enum variable initializers", func() {
		text := `|===|
	enum Fruit: string {
	  Apple,
	  Strawberry = "strawberry",
	}
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
	}
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

	It("shows implicit int enum values in completion details", func() {
		text := `|===|
	enum Status: int {
	  Pending,
	  Running,
	}
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
})
