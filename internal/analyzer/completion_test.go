package analyzer

import (
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
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
})
