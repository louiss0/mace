package analyzer

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/parser/ast"
)

var _ = Describe("completion analysis", func() {
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

		tAssert.Equal([]string{"Fruit.Apple", "Fruit.Strawberry"}, labels)
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
})
