package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
	. "github.com/onsi/ginkgo/v2"
	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"
)

var _ = Describe("completion analysis", func() {
	It("suggests $self in an empty output expression", func() {
		text := `[output = data]
{
  base: 1,
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
  foo: 1,
  result: true ? $self.foo : $
}`

		position := protocol.Position{
			Line:      3,
			Character: uint32(len(`  result: true ? $self.foo : $`)),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"$self"}, labels)
	})

	It("completes schema variable fields in the output block", func() {
		text := `|===|
schema User: { name: string, email: string, };
User user = { name: "Ada", email: "ada@example.com", };
|===|
[output = data]
{
  result: user.
}`

		position := protocol.Position{
			Line:      6,
			Character: uint32(len("  result: user.")),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"email", "name"}, labels)
	})

	DescribeTable("completes schema variable fields through nested schemas",
		func(depth int) {
			schemaDeclarations := lo.Map(lo.Range(depth), func(index int, _ int) string {
				name := fmt.Sprintf("Level%d", index+1)
				if index == depth-1 {
					return fmt.Sprintf("schema %s: { value: string, };", name)
				}
				return fmt.Sprintf("schema %s: { next: Level%d, };", name, index+2)
			})
			memberPath := strings.Repeat("next.", depth-1)
			rootValue := strings.Repeat("{ next: ", depth-1) + `{ value: "", }` + strings.Repeat(", }", depth-1)
			text := fmt.Sprintf(`|===|
%s
Level1 root = %s;
|===|
[output = data]
{
  result: root.%s
}`,
				strings.Join(schemaDeclarations, "\n"), rootValue, memberPath)

			position := protocol.Position{
				Line:      uint32(depth + 5),
				Character: uint32(len("  result: root." + memberPath)),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Equal([]string{"value"}, labels)
		},
		Entry("one nested schema", 1),
		Entry("two nested schemas", 2),
		Entry("three nested schemas", 3),
		Entry("four nested schemas", 4),
		Entry("five nested schemas", 5),
		Entry("six nested schemas", 6),
		Entry("seven nested schemas", 7),
		Entry("eight nested schemas", 8),
		Entry("nine nested schemas", 9),
		Entry("ten nested schemas", 10),
		Entry("eleven nested schemas", 11),
		Entry("twelve nested schemas", 12),
		Entry("thirteen nested schemas", 13),
		Entry("fourteen nested schemas", 14),
		Entry("fifteen nested schemas", 15),
	)

	DescribeTable("completes schema variable fields while initializing variables",
		func(depth int) {
			schemaDeclarations := lo.Map(lo.Range(depth), func(index int, _ int) string {
				name := fmt.Sprintf("Level%d", index+1)
				if index == depth-1 {
					return fmt.Sprintf("schema %s: { value: string, };", name)
				}
				return fmt.Sprintf("schema %s: { next: Level%d, };", name, index+2)
			})
			memberPath := strings.Repeat("next.", depth-1)
			rootValue := strings.Repeat("{ next: ", depth-1) + `{ value: "", }` + strings.Repeat(", }", depth-1)
			text := fmt.Sprintf(`|===|
%s
Level1 root = %s;
string result = root.%s
|===|
[output = data]
{ value: result, }`,
				strings.Join(schemaDeclarations, "\n"), rootValue, memberPath)

			position := protocol.Position{
				Line:      uint32(depth + 2),
				Character: uint32(len("string result = root." + memberPath)),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Equal([]string{"value"}, labels)
		},
		Entry("one nested schema", 1),
		Entry("two nested schemas", 2),
		Entry("three nested schemas", 3),
		Entry("four nested schemas", 4),
		Entry("five nested schemas", 5),
		Entry("six nested schemas", 6),
		Entry("seven nested schemas", 7),
		Entry("eight nested schemas", 8),
		Entry("nine nested schemas", 9),
		Entry("ten nested schemas", 10),
		Entry("eleven nested schemas", 11),
		Entry("twelve nested schemas", 12),
		Entry("thirteen nested schemas", 13),
		Entry("fourteen nested schemas", 14),
		Entry("fifteen nested schemas", 15),
	)

	DescribeTable("completes variant schema variable fields in is-narrowed initializers",
		func(depth int) {
			schemaDeclarations := lo.Map(lo.Range(depth), func(index int, _ int) string {
				name := fmt.Sprintf("Level%d", index+1)
				if index == depth-1 {
					return fmt.Sprintf("schema %s: { value: string, };", name)
				}
				return fmt.Sprintf("schema %s: { next: Level%d, };", name, index+2)
			})
			memberPath := strings.Repeat("next.", depth-1)
			rootValue := strings.Repeat("{ next: ", depth-1) + `{ value: "", }` + strings.Repeat(", }", depth-1)
			text := fmt.Sprintf(`|===|
%s
schema Other: { other: string, };
variant[Level1, Other] root = %s;
string result = root is Level1 ? root.%s
|===|
[output = data]
{ value: result, }`,
				strings.Join(schemaDeclarations, "\n"), rootValue, memberPath)

			position := protocol.Position{
				Line:      uint32(depth + 3),
				Character: uint32(len("string result = root is Level1 ? root." + memberPath)),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Equal([]string{"value"}, labels)
		},
		Entry("one nested schema", 1),
		Entry("two nested schemas", 2),
		Entry("three nested schemas", 3),
		Entry("four nested schemas", 4),
		Entry("five nested schemas", 5),
		Entry("six nested schemas", 6),
		Entry("seven nested schemas", 7),
		Entry("eight nested schemas", 8),
		Entry("nine nested schemas", 9),
		Entry("ten nested schemas", 10),
		Entry("eleven nested schemas", 11),
		Entry("twelve nested schemas", 12),
		Entry("thirteen nested schemas", 13),
		Entry("fourteen nested schemas", 14),
		Entry("fifteen nested schemas", 15),
	)

	It("completes nullable schema variable fields in a truthy initializer branch", func() {
		text := `|===|
schema User: { name: string, email: string, };
nullable User user = null;
string result = user ? user.
|===|
[output = data]
{ value: result, }`

		position := protocol.Position{
			Line:      3,
			Character: uint32(len("string result = user ? user.")),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"email", "name"}, labels)
	})

	It("completes nullable schema variable fields in a truthy branch", func() {
		text := `|===|
schema User: { name: string, email: string, };
nullable User user = null;
|===|
[output = data]
{
  result: user ? user.
}`

		position := protocol.Position{
			Line:      6,
			Character: uint32(len("  result: user ? user.")),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
		labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
			return item.Label
		})

		tAssert.Equal([]string{"email", "name"}, labels)
	})

	It("does not complete nullable schema variable fields outside a truthy branch", func() {
		text := `|===|
schema User: { name: string, email: string, };
nullable User user = null;
|===|
[output = data]
{
  result: user.
}`

		position := protocol.Position{
			Line:      6,
			Character: uint32(len("  result: user.")),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)

		tAssert.Empty(items)
	})

	It("returns no global completions after a dot in the script block", func() {
		text := `|===|
schema User: { name: string, };
User.
|===|
[output = data] {}`

		position := protocol.Position{
			Line:      2,
			Character: uint32(len("User.")),
		}
		documentPath := filepath.Join("workspace", "document.mace")
		snapshot := AnalyzeCompletionContext(text, documentPath, position)

		items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)

		tAssert.Empty(items)
	})

	It("keeps typed output completions alongside $self in output schema fields", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit, };
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

	Describe("Parse completions", func() {
		It("suggests parsed fields as output variables", func() {
			text := `|===|
schema Runtime: {
  env: string,
  profile: { name: string, email: string, },
};
|===|
[output = data, parse = Runtime]
{
  result:
}`

			position := protocol.Position{
				Line:      7,
				Character: uint32(len(`  result: `)),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})
			details := map[string]string{}
			for _, item := range items {
				if item.Detail != nil {
					details[item.Label] = *item.Detail
				}
			}

			tAssert.Contains(labels, "$env")
			tAssert.Contains(labels, "$profile")
			tAssert.Equal("string", details["$env"])
			tAssert.Equal("{ name: string, email: string }", details["$profile"])
		})

		It("suggests parse_file fields as output variables", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-*")
			tAssert.NoError(err)
			defer func() {
				_ = os.RemoveAll(workspace)
			}()

			runtimePath := filepath.Join(workspace, "runtime.mace")
			tAssert.NoError(os.WriteFile(runtimePath, []byte(`[output = schema]
{
  Runtime: { env: string, region: string, },
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

			tAssert.Contains(labels, "$env")
			tAssert.Contains(labels, "$region")
		})

		It("only suggests top-level parse fields as output variables", func() {
			text := `|===|
schema Runtime: {
  env: string,
  profile: { name: string, email: string, },
};
|===|
[output = data, parse = Runtime]
{
  result:
}`

			position := protocol.Position{
				Line:      8,
				Character: uint32(len(`  result: `)),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})
			details := map[string]string{}
			for _, item := range items {
				if item.Detail != nil {
					details[item.Label] = *item.Detail
				}
			}

			tAssert.Contains(labels, "$env")
			tAssert.Contains(labels, "$profile")
			tAssert.Equal("string", details["$env"])
			tAssert.Equal("{ name: string, email: string }", details["$profile"])
		})

		It("only suggests top-level parse_file fields as output variables", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-top-level-*")
			tAssert.NoError(err)
			defer func() {
				_ = os.RemoveAll(workspace)
			}()

			runtimePath := filepath.Join(workspace, "runtime.mace")
			tAssert.NoError(os.WriteFile(runtimePath, []byte(`[output = schema]
{
  Runtime: {
    env: string,
    profile: { name: string, email: string, },
  },
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

			tAssert.NotContains(labels, "env")
			tAssert.NotContains(labels, "profile")
			tAssert.NotContains(labels, "name")
			tAssert.NotContains(labels, "email")
			tAssert.Contains(labels, "$env")
			tAssert.Contains(labels, "$profile")
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
  Runtime: { user: { name: string, home: { street: string, city: string, }, }, },
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `[output = data, parse_file = "./runtime.mace"]
{
  result: $user.
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))
			position := protocol.Position{
				Line:      2,
				Character: uint32(len(`  result: $user.`)),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "name")
			tAssert.Contains(labels, "home")
		})

		It("does not suggest schema names as output expressions", func() {
			text := `|===|
schema Runtime: { env: string, };
|===|
[output = data]
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

			tAssert.NotContains(labels, "Runtime")
		})

		It("does not suggest imported schema names as output expressions", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-output-schema-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = schema]
{
  Runtime: { env: string, },
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `|===|
from "./shared.mace" import Runtime;
|===|
[output = data]
{
  result:
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      5,
				Character: uint32(len(`  result: `)),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.NotContains(labels, "Runtime")
		})

		It("suggests import-as data aliases as output variables", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-import-as-data-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  project: {
    name: "pi-prompt-form",
    root: "libs/pi-prompt-form",
  },
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `|===|
from "./shared.mace" import-as Shared;
|===|
[output = data]
{
  result:
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      5,
				Character: uint32(len(`  result: `)),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "Shared")
			tAssert.NotContains(labels, "project")
		})

		It("completes members for import-as data aliases", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-import-as-data-members-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  project: {
    name: "pi-prompt-form",
    root: "libs/pi-prompt-form",
  },
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `|===|
from "./shared.mace" import-as Shared;
|===|
[output = data]
{
  result: Shared.project.
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      5,
				Character: uint32(len(`  result: Shared.project.`)),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "name")
			tAssert.Contains(labels, "root")
			tAssert.NotContains(labels, "project")
		})

		DescribeTable("completes import-as data aliases across nested levels",
			func(cursorExpr string, expectedLabels []string) {
				workspace, err := os.MkdirTemp("", "mace-analyzer-import-as-data-depth-*")
				tAssert.NoError(err)
				defer func() { _ = os.RemoveAll(workspace) }()

				sharedPath := filepath.Join(workspace, "shared.mace")
				tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  level1: {
    value: "one",
    level2: {
      value: "two",
      level3: {
        value: "three",
        level4: {
          value: "four",
          level5: {
            value: "five",
          },
        },
      },
    },
  },
}`), 0o644))
				documentPath := filepath.Join(workspace, "document.mace")
				text := fmt.Sprintf(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data]
{
  result: %s
}`, cursorExpr)
				tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

				position := protocol.Position{
					Line:      5,
					Character: uint32(len("  result: ") + len(cursorExpr)),
				}
				snapshot := AnalyzeCompletionContext(text, documentPath, position)

				items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
				labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
					return item.Label
				})

				for _, label := range expectedLabels {
					tAssert.Contains(labels, label)
				}
			},
			Entry("level 1", "Shared.level1.", []string{"value", "level2"}),
			Entry("level 2", "Shared.level1.level2.", []string{"value", "level3"}),
			Entry("level 3", "Shared.level1.level2.level3.", []string{"value", "level4"}),
			Entry("level 4", "Shared.level1.level2.level3.level4.", []string{"value", "level5"}),
			Entry("level 5", "Shared.level1.level2.level3.level4.level5.", []string{"value"}),
		)

		DescribeTable("completes import-as schema aliases through parse across nested levels",
			func(cursorExpr string, expectedLabels []string) {
				workspace, err := os.MkdirTemp("", "mace-analyzer-import-as-schema-depth-*")
				tAssert.NoError(err)
				defer func() { _ = os.RemoveAll(workspace) }()

				sharedPath := filepath.Join(workspace, "shared.mace")
				tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = schema]
{
  level1: {
    value: string,
    level2: {
      value: string,
      level3: {
        value: string,
        level4: {
          value: string,
          level5: {
            value: string,
          },
        },
      },
    },
  },
}`), 0o644))
				documentPath := filepath.Join(workspace, "document.mace")
				text := fmt.Sprintf(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data, parse = Shared]
{
  result: %s
}`, cursorExpr)
				tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

				position := protocol.Position{
					Line:      5,
					Character: uint32(len("  result: ") + len(cursorExpr)),
				}
				snapshot := AnalyzeCompletionContext(text, documentPath, position)

				items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
				labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
					return item.Label
				})

				for _, label := range expectedLabels {
					tAssert.Contains(labels, label)
				}
			},
			Entry("level 1", "$level1.", []string{"value", "level2"}),
			Entry("level 2", "$level1.level2.", []string{"value", "level3"}),
			Entry("level 3", "$level1.level2.level3.", []string{"value", "level4"}),
			Entry("level 4", "$level1.level2.level3.level4.", []string{"value", "level5"}),
			Entry("level 5", "$level1.level2.level3.level4.level5.", []string{"value"}),
		)

		It("suggests import-as schema aliases in directive completions", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-import-as-schema-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = schema]
{
  Package: {
    name: string,
    version: string,
  },
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `|===|
from "./shared.mace" import-as Shared;
|===|
[output = data, parse = ]
{
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      3,
				Character: uint32(len(`[output = data, parse = `)),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "Shared")
		})

		It("suggests parsed fields when previous output fields use commas", func() {
			text := `|===|
schema Runtime: {
  env: string,
  region: string,
};
|===|
[output = data, parse = Runtime]
{
  name: "mace",
  result:
}`

			position := protocol.Position{
				Line:      7,
				Character: uint32(len(`  result: `)),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "$env")
			tAssert.Contains(labels, "$region")
		})

		It("suggests parse_file fields when previous output fields use commas", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-commas-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			runtimePath := filepath.Join(workspace, "runtime.mace")
			tAssert.NoError(os.WriteFile(runtimePath, []byte(`[output = schema]
{
  Runtime: { env: string, region: string, },
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `[output = data, parse_file = "./runtime.mace"]
{
  name: "mace",
  result:
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      3,
				Character: uint32(len(`  result: `)),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "$env")
			tAssert.Contains(labels, "$region")
		})

		DescribeTable("completes recursive nested parse values through member access",
			func(cursorExpr string, expectedLabels []string) {
				text := fmt.Sprintf(`|===|
schema Contact: {
  email: string,
  phone: string,
};
schema Profile: {
  title: string,
  contact: Contact,
};
schema User: {
  name: string,
  manager: User,
  profile: Profile,
};
|===|

[output = data, parse = User]
{
  result: %s
}`, cursorExpr)

				position := protocol.Position{
					Line:      18,
					Character: uint32(len("  result: ") + len(cursorExpr)),
				}
				documentPath := filepath.Join("workspace", "document.mace")
				snapshot := AnalyzeCompletionContext(text, documentPath, position)

				items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
				labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
					return item.Label
				})

				for _, label := range expectedLabels {
					tAssert.Contains(labels, label)
				}
			},
			Entry("completes fields on recursive parsed record", "$manager.", []string{"manager", "name", "profile"}),
			Entry("completes fields on second recursive level", "$manager.manager.", []string{"manager", "name", "profile"}),
			Entry("completes nested profile fields on recursive parsed record", "$manager.profile.", []string{"contact", "title"}),
			Entry("completes nested contact fields on second recursive level", "$manager.manager.profile.contact.", []string{"email", "phone"}),
			Entry("completes nested contact fields on deep recursive level without infinite traversal", "$manager.manager.manager.manager.profile.contact.", []string{"email", "phone"}),
		)

		It("completes nested record fields via member access on parse variable", func() {
			text := `|===|
schema Address: {
  street: string,
  city: string,
};
schema User: {
  name: string,
  home: Address,
};
|===|

[output = data, parse = User]
{
  result: $home.
}`

			position := protocol.Position{
				Line:      13,
				Character: uint32(len("  result: $home.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "street")
			tAssert.Contains(labels, "city")
			tAssert.NotContains(labels, "name")
			tAssert.NotContains(labels, "home")
		})

		It("completes fields through multi-hop member access on parse variables", func() {
			text := `|===|
schema Location: {
  lat: float,
  lon: float,
};
schema Address: {
  street: string,
  location: Location,
};
schema User: {
  name: string,
  home: Address,
};
|===|

[output = data, parse = User]
{
  result: $home.location.
}`

			position := protocol.Position{
				Line:      17,
				Character: uint32(len("  result: $home.location.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "lat")
			tAssert.Contains(labels, "lon")
			tAssert.NotContains(labels, "street")
			tAssert.NotContains(labels, "name")
		})

		It("returns no completions for member access on a primitive parse variable field", func() {
			text := `|===|
schema User: {
  name: string,
  age: int,
};
|===|

[output = data, parse = User]
{
  result: $name.
}`

			position := protocol.Position{
				Line:      9,
				Character: uint32(len("  result: $name.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)

			tAssert.Empty(items)
		})

		It("returns no completions for member access on an unknown output value", func() {
			text := `[output = data]
{
  result: unknown.
}`

			position := protocol.Position{
				Line:      2,
				Character: uint32(len("  result: unknown.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)

			tAssert.Empty(items)
		})

		It("returns no completions for member access on an array parse variable field", func() {
			text := `|===|
schema User: {
  name: string,
  tags: array<string>,
};
|===|

[output = data, parse = User]
{
  result: $tags.
}`

			position := protocol.Position{
				Line:      9,
				Character: uint32(len("  result: $tags.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)

			tAssert.Empty(items)
		})

		It("completes parse_file schema fields via member access chain", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-member-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			tAssert.NoError(os.WriteFile(filepath.Join(workspace, "schema.mace"), []byte(`[output = schema]
{
  User: { name: string, home: { street: string, city: string, }, },
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `[output = data, parse_file = "./schema.mace"]
{
  result: $home.
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      2,
				Character: uint32(len("  result: $home.")),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "street")
			tAssert.Contains(labels, "city")
		})

		It("completes members for exported parse_file props", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-export-members-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			tAssert.NoError(os.WriteFile(filepath.Join(workspace, "schema.mace"), []byte(`|===|
schema Project: {
  name: string,
  root: string,
};
schema Workspace: {
  name: string,
  root: string,
};
|===|
[output = schema]
{
  project: Project,
  workspace: Workspace,
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `[output = data, parse_file = "./schema.mace"]
{
  result: $project.
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      2,
				Character: uint32(len("  result: $project.")),
			}
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "name")
			tAssert.Contains(labels, "root")
			tAssert.NotContains(labels, "project")
			tAssert.NotContains(labels, "workspace")
		})

		It("returns no completions for unguarded optional parse field member access", func() {
			text := `|===|
schema User: {
  name: string,
  manager?: User,
};
|===|

[output = data, parse = User]
{
  result: $manager.
}`
			position := protocol.Position{
				Line:      9,
				Character: uint32(len("  result: $manager.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			tAssert.Empty(items)
		})

		It("provides completions for optional parse field member access inside 'in' guard", func() {
			text := `|===|
schema User: {
  name: string,
  manager?: User,
};
|===|

[output = data, parse = User]
{
  result: "manager" in $manager ? $manager.manager.
}`
			position := protocol.Position{
				Line:      9,
				Character: uint32(len(`  result: "manager" in $manager ? $manager.manager.`)),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string {
				return item.Label
			})

			tAssert.Contains(labels, "name")
			tAssert.Contains(labels, "manager")
		})
	})
	It("suggests choice values for output block schema fields", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite_fruit: Fruit, };
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

	It("suggests choice values after earlier self member access", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { previous: Fruit, favorite_fruit: Fruit, };
|===|
[output = data, schema = Basket]
{
  favorite_fruit: true ? $self.previous :
}`

		position := protocol.Position{
			Line:      6,
			Character: uint32(len(`  favorite_fruit: true ? $self.previous : `)),
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
 schema Basket: { favorite_fruit: Fruit, };
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
 schema Basket: { favorite_fruit: Fruit, };
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
 schema User: { name: string, };
 type Identity: variant[Role, User];
 schema Envelope: { value: Identity, };
 schema Response: { payload: Envelope, };
|===|
[output = data, schema = Response]
{
  payload: {
    value:
  },
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
  user: User,
  role: Role,
  label: Alias,
  count: int,
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
  flavor: Flavor,
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
  name: "Ada",
  age: 30,
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
  name: "Ada",
  age: 30,
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
  name: "Ada",
  age: 30,
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
  name: "Ada",
  age: 30,
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
    name: "Ada",
  },
}`)

		documentDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(documentDir, 0o755))
		documentPath := filepath.Join(documentDir, "consumer.mace")
		text := `|===|
from "../shared.mace" import base;
|===|
[output = data]
{
  base: base,
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
  User: string,
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

	Describe("completion helpers", func() {

		It("builds completion values and member items", func() {
			model := completionModel{
				aliases: map[string]ast.TypeReference{
					"UserList": ast.ArrayType{Element: ast.NamedType{Name: "User"}},
				},
				schemas: map[string]ast.RecordType{
					"User": {Fields: []ast.SchemaField{
						{Name: "name", Type: ast.PrimitiveType{Name: "string"}},
						{Name: "active", Type: ast.PrimitiveType{Name: "boolean"}},
					}},
				},
			}

			value := syntheticCompletionValue(ast.NamedType{Name: "UserList"}, model, 3)
			tAssert.Equal(processor.ValueArray, value.Kind)
			tAssert.Equal(processor.ValueRecord, value.Array[0].Kind)

			items := completionItemsForValueMembers(value.Array[0])
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })
			tAssert.Equal([]string{"active", "name"}, labels)

			tAssert.Equal(processor.ValueUnknown, syntheticCompletionValue(ast.NamedType{Name: "User"}, model, 0).Kind)
		})

		It("builds default literals for completion types", func() {
			model := completionModel{
				aliases: map[string]ast.TypeReference{
					"Mode": ast.ChoiceType{Members: []ast.Expression{
						ast.StringLiteral{Lexeme: `"auto"`},
					}},
				},
				schemas: map[string]ast.RecordType{
					"Node": {Fields: []ast.SchemaField{{
						Name: "child",
						Type: ast.NamedType{Name: "Node"},
					}}},
				},
			}

			tAssert.Equal(`"auto"`, defaultLiteralForType(ast.NamedType{Name: "Mode"}, model, map[string]struct{}{}))
			tAssert.Equal("[]", defaultLiteralForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, map[string]struct{}{}))
			tAssert.Equal("{ child: {} }", defaultLiteralForType(ast.NamedType{Name: "Node"}, model, map[string]struct{}{}))
			tAssert.Equal("{}", defaultLiteralForType(ast.NamedType{Name: "Missing"}, model, map[string]struct{}{}))
		})

		It("finds expression and placeholder paths", func() {
			path, ok := expressionPath(ast.MemberAccess{
				Target: ast.MemberAccess{
					Target: ast.Identifier{Name: "user"},
					Name:   "profile",
				},
				Name: "email",
			})
			tAssert.True(ok)
			tAssert.Equal([]string{"user", "profile", "email"}, path)

			path, ok = placeholderPath(ast.ConditionalExpression{
				Condition: ast.BooleanLiteral{Value: false},
				Then: ast.ArrayLiteral{Elements: []ast.Expression{
					ast.MemberAccess{
						Target: ast.Identifier{Name: "user"},
						Name:   completionPlaceholderIdentifier,
					},
				}},
				Else: ast.Identifier{Name: "fallback"},
			})
			tAssert.True(ok)
			tAssert.Equal([]string{completionArrayPathSegment, "user"}, path)
		})

		It("normalizes import path completions", func() {
			tAssert.Equal("./", normalizedRelativePathPrefix(""))
			tAssert.Equal("./schema", normalizedRelativePathPrefix("schema"))
			tAssert.Equal("../schema", normalizedRelativePathPrefix("../schema"))
			tAssert.Equal("./schema/", joinImportPath("./", "schema", true))
			tAssert.Equal("./schema.mace", joinImportPath("./", "schema.mace", false))
		})

		It("completes output field types from expressions", func() {
			model := completionModel{
				aliases: map[string]ast.TypeReference{"Alias": ast.PrimitiveType{Name: "string"}},
				schemas: map[string]ast.RecordType{"Profile": {Fields: []ast.SchemaField{{
					Name: "email",
					Type: ast.PrimitiveType{Name: "string"},
				}}}},
				variables: map[string]ast.TypeReference{"count": ast.PrimitiveType{Name: "int"}},
			}
			fn := completionOutputFieldType

			tests := []struct {
				expression ast.Expression
				detail     string
				ok         bool
			}{
				{ast.StringLiteral{Lexeme: `"Ada"`}, "string", true},
				{ast.IntLiteral{Lexeme: "1"}, "int", true},
				{ast.FloatLiteral{Lexeme: "1.2"}, "float", true},
				{ast.HexIntLiteral{Lexeme: "0x1"}, "hex_int", true},
				{ast.HexFloatLiteral{Lexeme: "0x1.2"}, "hex_float", true},
				{ast.BooleanLiteral{Value: true}, "boolean", true},
				{ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, "array<int>", true},
				{ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, "{ name: string }", true},
				{ast.Identifier{Name: "Alias"}, "string", true},
				{ast.Identifier{Name: "Profile"}, "{ email: string }", true},
				{ast.Identifier{Name: "count"}, "int", true},
				{ast.ConditionalExpression{
					Condition: ast.BooleanLiteral{Value: true},
					Then:      ast.StringLiteral{Lexeme: `"primary"`},
					Else: ast.ConditionalExpression{
						Condition: ast.BooleanLiteral{Value: false},
						Then:      ast.IntLiteral{Lexeme: "10"},
						Else:      ast.BooleanLiteral{Value: true},
					},
				}, "variant[string, int, boolean]", true},
				{ast.ConditionalExpression{
					Condition: ast.BooleanLiteral{Value: true},
					Then: ast.ArrayLiteral{Elements: []ast.Expression{
						ast.IntLiteral{Lexeme: "1"},
					}},
					Else: ast.StringLiteral{Lexeme: `"fallback"`},
				}, "variant[array<int>, string]", true},
				{ast.ConditionalExpression{
					Condition: ast.BooleanLiteral{Value: true},
					Then: ast.RecordLiteral{Fields: []ast.RecordField{{
						Name:  "name",
						Value: ast.StringLiteral{Lexeme: `"Ada"`},
					}}},
					Else: ast.BooleanLiteral{Value: false},
				}, "variant[{ name: string }, boolean]", true},
				{ast.ConditionalExpression{
					Condition: ast.BooleanLiteral{Value: true},
					Then:      ast.Identifier{Name: "Profile"},
					Else:      ast.RecordLiteral{},
				}, "{ email: string }", true},
				{ast.ArrayLiteral{}, "", false},
				{ast.Identifier{Name: "missing"}, "", false},
			}

			for _, test := range tests {
				fieldType, ok := fn(test.expression, model)
				tAssert.Equal(test.ok, ok)
				if ok {
					tAssert.Equal(test.detail, typeReferenceDetail(fieldType))
				}
			}
		})

		DescribeTable("recursively infers conditional types through fifteen ternaries",
			func(level int, expected string) {
				alternatives := []ast.Expression{
					ast.StringLiteral{Lexeme: `"one"`},
					ast.IntLiteral{Lexeme: "2"},
					ast.BooleanLiteral{Value: true},
					ast.FloatLiteral{Lexeme: "4.5"},
					ast.HexIntLiteral{Lexeme: "0x5"},
					ast.HexFloatLiteral{Lexeme: "0x6.8"},
				}
				expression := alternatives[level%len(alternatives)]
				for branch := level - 1; branch >= 0; branch-- {
					expression = ast.ConditionalExpression{
						Condition: ast.BooleanLiteral{Value: false},
						Then:      alternatives[branch%len(alternatives)],
						Else:      expression,
					}
				}

				fieldType, ok := completionOutputFieldType(expression, completionModel{})
				tAssert.True(ok)
				if ok {
					tAssert.Equal(expected, typeReferenceDetail(fieldType))
				}
			},
			Entry("three ternaries", 3, "variant[string, int, boolean, float]"),
			Entry("four ternaries", 4, "variant[string, int, boolean, float, hex_int]"),
			Entry("five ternaries", 5, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("six ternaries", 6, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("seven ternaries", 7, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("eight ternaries", 8, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("nine ternaries", 9, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("ten ternaries", 10, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("eleven ternaries", 11, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("twelve ternaries", 12, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("thirteen ternaries", 13, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("fourteen ternaries", 14, "variant[string, int, boolean, float, hex_int, hex_float]"),
			Entry("fifteen ternaries", 15, "variant[string, int, boolean, float, hex_int, hex_float]"),
		)

		DescribeTable("recursively infers array variants through ten ternaries",
			func(level int) {
				arrayExpression := func(element ast.Expression) ast.Expression {
					return ast.ArrayLiteral{Elements: []ast.Expression{element}}
				}
				alternatives := []ast.Expression{
					arrayExpression(ast.StringLiteral{Lexeme: `"one"`}),
					arrayExpression(ast.IntLiteral{Lexeme: "2"}),
					arrayExpression(ast.BooleanLiteral{Value: true}),
					arrayExpression(ast.FloatLiteral{Lexeme: "4.5"}),
					arrayExpression(ast.HexIntLiteral{Lexeme: "0x5"}),
					arrayExpression(ast.HexFloatLiteral{Lexeme: "0x6.8"}),
					arrayExpression(arrayExpression(ast.StringLiteral{Lexeme: `"seven"`})),
					arrayExpression(arrayExpression(ast.IntLiteral{Lexeme: "8"})),
					arrayExpression(arrayExpression(ast.BooleanLiteral{Value: false})),
					arrayExpression(arrayExpression(ast.FloatLiteral{Lexeme: "10.5"})),
					arrayExpression(arrayExpression(ast.HexIntLiteral{Lexeme: "0xB"})),
				}
				typeNames := []string{
					"array<string>",
					"array<int>",
					"array<boolean>",
					"array<float>",
					"array<hex_int>",
					"array<hex_float>",
					"array<array<string>>",
					"array<array<int>>",
					"array<array<boolean>>",
					"array<array<float>>",
					"array<array<hex_int>>",
				}
				expression := alternatives[level]
				for branch := level - 1; branch >= 0; branch-- {
					expression = ast.ConditionalExpression{
						Condition: ast.BooleanLiteral{Value: false},
						Then:      alternatives[branch],
						Else:      expression,
					}
				}

				fieldType, ok := completionOutputFieldType(expression, completionModel{})
				tAssert.True(ok)
				if ok {
					expected := "variant[" + strings.Join(typeNames[:level+1], ", ") + "]"
					tAssert.Equal(expected, typeReferenceDetail(fieldType))
				}
			},
			Entry("three ternaries", 3),
			Entry("four ternaries", 4),
			Entry("five ternaries", 5),
			Entry("six ternaries", 6),
			Entry("seven ternaries", 7),
			Entry("eight ternaries", 8),
			Entry("nine ternaries", 9),
			Entry("ten ternaries", 10),
		)

		DescribeTable("recursively infers record variants through ten ternaries",
			func(level int) {
				recordExpression := func(name string, value ast.Expression) ast.Expression {
					return ast.RecordLiteral{Fields: []ast.RecordField{{Name: name, Value: value}}}
				}
				alternatives := []ast.Expression{
					recordExpression("text", ast.StringLiteral{Lexeme: `"one"`}),
					recordExpression("count", ast.IntLiteral{Lexeme: "2"}),
					recordExpression("enabled", ast.BooleanLiteral{Value: true}),
					recordExpression("ratio", ast.FloatLiteral{Lexeme: "4.5"}),
					recordExpression("mask", ast.HexIntLiteral{Lexeme: "0x5"}),
					recordExpression("fraction", ast.HexFloatLiteral{Lexeme: "0x6.8"}),
					recordExpression("region", ast.StringLiteral{Lexeme: `"seven"`}),
					recordExpression("retries", ast.IntLiteral{Lexeme: "8"}),
					recordExpression("cached", ast.BooleanLiteral{Value: false}),
					recordExpression("timeout", ast.FloatLiteral{Lexeme: "10.5"}),
					recordExpression("flags", ast.HexIntLiteral{Lexeme: "0xB"}),
				}
				typeNames := []string{
					"{ text: string }",
					"{ count: int }",
					"{ enabled: boolean }",
					"{ ratio: float }",
					"{ mask: hex_int }",
					"{ fraction: hex_float }",
					"{ region: string }",
					"{ retries: int }",
					"{ cached: boolean }",
					"{ timeout: float }",
					"{ flags: hex_int }",
				}
				expression := alternatives[level]
				for branch := level - 1; branch >= 0; branch-- {
					expression = ast.ConditionalExpression{
						Condition: ast.BooleanLiteral{Value: false},
						Then:      alternatives[branch],
						Else:      expression,
					}
				}

				fieldType, ok := completionOutputFieldType(expression, completionModel{})
				tAssert.True(ok)
				if ok {
					expected := "variant[" + strings.Join(typeNames[:level+1], ", ") + "]"
					tAssert.Equal(expected, typeReferenceDetail(fieldType))
				}
			},
			Entry("three ternaries", 3),
			Entry("four ternaries", 4),
			Entry("five ternaries", 5),
			Entry("six ternaries", 6),
			Entry("seven ternaries", 7),
			Entry("eight ternaries", 8),
			Entry("nine ternaries", 9),
			Entry("ten ternaries", 10),
		)

		It("resolves record map types and their member paths", func() {
			model := completionModel{
				aliases:   map[string]ast.TypeReference{},
				schemas:   map[string]ast.RecordType{},
				variables: map[string]ast.TypeReference{},
			}
			recordType := ast.RecordMapType{Value: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}

			resolved := resolveCompletionType(recordType, model, map[string]struct{}{})
			tAssert.Equal(completionTypeRecordMap, resolved.kind)
			tAssert.Equal(`record<record<string>>`, typeReferenceDetail(recordType))
			tAssert.Equal("{}", defaultLiteralForType(recordType, model, map[string]struct{}{}))

			memberType, ok := completionTypeAtPath(recordType, []string{"codefixer", "cn_efs"}, model)
			tAssert.True(ok)
			tAssert.Equal("string", typeReferenceDetail(memberType))

			memberType, ok, guarded := parseMemberCompletionType(recordType, []string{"codefixer", "cn_efs"}, model, map[string]struct{}{})
			tAssert.True(ok)
			tAssert.False(guarded)
			tAssert.Equal("string", typeReferenceDetail(memberType))
		})

		It("handles directives and completion delimiters", func() {
			directives := []ast.OutputDirective{{
				Kind:  ast.OutputDirectiveSchema,
				Value: "User",
			}}
			tAssert.True(hasOutputDirective(directives, ast.OutputDirectiveSchema))
			tAssert.False(hasOutputDirective(directives, ast.OutputDirectiveParse))

			stack := []byte{'(', '['}
			stack = popCompletionDelimiter(stack, '[')
			tAssert.Equal([]byte{'('}, stack)
			tAssert.Equal([]byte{'('}, popCompletionDelimiter(stack, '{'))

			tAssert.Equal("name_1", trailingIdentifierPrefix("user.name_1"))
			tAssert.Equal("", trailingIdentifierPrefix("user."))
		})

		It("computes next output directive definitions", func() {
			empty := nextDirectiveDefinitions(nil)
			tAssert.Empty(empty)

			options := nextDirectiveDefinitions([]string{"output = data"})
			labels := lo.Map(options, func(item completionDefinition, _ int) string { return item.Label })
			tAssert.Equal([]string{"schema", "schema_file", "parse", "parse_file"}, labels)

			options = nextDirectiveDefinitions([]string{"output = data", "schema = User"})
			labels = lo.Map(options, func(item completionDefinition, _ int) string { return item.Label })
			tAssert.Equal([]string{"parse", "parse_file"}, labels)
		})

		It("processes variables and importable identifiers", func() {
			workspace, err := os.MkdirTemp("", "mace-completion-importable-identifiers-*")
			tAssert.NoError(err)
			defer func() {
				_ = os.RemoveAll(workspace)
			}()

			documentPath := filepath.Join(workspace, "document.mace")
			tAssert.NoError(os.WriteFile(documentPath, []byte(`[output = data] {}`), 0o644))
			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  name: "Ada",
}`), 0o644))

			variables := processVariablesInDocument(`|===|
int count = 1;
|===|
[output = data] {}`, protocol.DocumentUri(fileURI(documentPath)))
			tAssert.Equal(processor.ValueInt, variables["count"].Kind)

			partial := partialScriptVariables("|===|\nint count = 1;\n|===|\n[output = data] {}", protocol.DocumentUri(fileURI(documentPath)), protocol.Position{Line: 1, Character: 3})
			_ = partial

			scriptVariables := scriptVariablesForOutput("|===|\nint count = 1;\n|===|\n[output = data] {}", protocol.DocumentUri(fileURI(documentPath)))
			_ = scriptVariables

			names, ok := importableIdentifiers(protocol.DocumentUri(fileURI(documentPath)), workspace, "./shared.mace")
			tAssert.True(ok)
			tAssert.Equal([]string{"name"}, names)

			_, ok = importableIdentifiers(protocol.DocumentUri("not a file"), workspace, "./shared.mace")
			tAssert.False(ok)
		})

		It("reads imported paths and directory entries", func() {
			workspace, err := os.MkdirTemp("", "mace-completion-directory-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			tAssert.NoError(os.MkdirAll(filepath.Join(workspace, "schemas", "nested"), 0o755))
			tAssert.NoError(os.WriteFile(filepath.Join(workspace, "schemas", "profile.mace"), []byte(``), 0o644))
			tAssert.NoError(os.WriteFile(filepath.Join(workspace, "schemas", "skip.txt"), []byte(``), 0o644))

			doc := document{analysis: Snapshot{file: &ast.File{Imports: []ast.ImportDeclaration{
				{Path: ast.StringLiteral{Lexeme: `"./schemas/profile.mace"`}},
				{Path: ast.StringLiteral{Lexeme: `"./schemas/nested/"`}},
			}}}}
			paths := importedPaths(doc, `from "./schemas/n`)
			tAssert.Equal([]string{"./schemas/profile.mace", "./schemas/nested/"}, paths)

			items, err := directoryEntries(workspace, workspace, "", nil, false)
			tAssert.NoError(err)
			labels := lo.Map(items, func(item protocol.CompletionItem, _ int) string { return item.Label })
			tAssert.Contains(labels, "./schemas/")
			tAssert.NotContains(labels, "./skip.txt")
		})

		It("merges completion fusion records", func() {
			model := completionModel{
				schemas: map[string]ast.RecordType{
					"Base": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}},
					"Extra": {Fields: []ast.SchemaField{
						{Name: "name", Type: ast.PrimitiveType{Name: "string"}, Optional: true},
						{Name: "age", Type: ast.PrimitiveType{Name: "int"}},
					}},
					"Conflict": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}},
				},
			}

			record, ok := completionUnionRecord([]ast.TypeReference{
				ast.NamedType{Name: "Base"},
				ast.NamedType{Name: "Extra"},
			}, model, map[string]struct{}{})
			tAssert.True(ok)
			tAssert.Len(record.Fields, 2)
			tAssert.False(record.Fields[0].Optional)

			_, ok = completionUnionRecord([]ast.TypeReference{
				ast.NamedType{Name: "Base"},
				ast.NamedType{Name: "Conflict"},
			}, model, map[string]struct{}{})
			tAssert.False(ok)

			_, ok = completionUnionRecord([]ast.TypeReference{
				ast.PrimitiveType{Name: "string"},
			}, model, map[string]struct{}{})
			tAssert.False(ok)
		})

		It("covers completion choice members and directory helpers", func() {
			model := completionModel{aliases: map[string]ast.TypeReference{
				"Mode": ast.ChoiceType{Members: []ast.Expression{
					ast.StringLiteral{Lexeme: `"auto"`},
					ast.IntLiteral{Lexeme: "1"},
				}},
			}}

			members, ok := completionChoiceMemberValues(ast.Identifier{Name: "Mode"}, model, map[string]struct{}{})
			tAssert.True(ok)
			tAssert.Equal("\"auto\"", members[0].Label)

			members, ok = completionChoiceMemberValues(ast.BooleanLiteral{Value: true}, model, map[string]struct{}{})
			tAssert.True(ok)
			tAssert.Equal("true", members[0].Label)

			_, ok = completionChoiceMemberValues(ast.Identifier{Name: "Missing"}, model, map[string]struct{}{})
			tAssert.False(ok)

			choice, ok := completionChoiceFromMembers([]ast.Expression{ast.Identifier{Name: "Mode"}}, model, map[string]struct{}{})
			tAssert.True(ok)
			tAssert.NotEmpty(choice.members)

			_, ok = completionChoiceFromMembers([]ast.Expression{ast.Identifier{Name: "Mode"}, ast.Identifier{Name: "Mode"}}, model, map[string]struct{}{})
			tAssert.True(ok)

			tAssert.Equal("", completionExpressionClosers("x", 0))
			tAssert.Equal(")", completionExpressionClosers("($self.foo", len("($self.foo")))

			workspace, err := os.MkdirTemp("", "mace-completion-directory-root-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()
			tAssert.NoError(os.WriteFile(filepath.Join(workspace, "alpha.mace"), []byte(``), 0o644))
			items, err := directoryEntries(workspace, workspace, "", nil, false)
			tAssert.NoError(err)
			tAssert.NotEmpty(items)
		})

		It("covers string literal and completion context helpers", func() {
			context, ok := stringLiteralCompletionContext(`"hello"`, protocol.Position{Line: 0, Character: 4})
			tAssert.True(ok)
			tAssert.Equal("hel", context.prefix)

			context, ok = stringLiteralCompletionContext(`'hello'`, protocol.Position{Line: 0, Character: 4})
			tAssert.True(ok)
			tAssert.Equal("hel", context.prefix)

			_, ok = stringLiteralCompletionContext("hello", protocol.Position{Line: 0, Character: 2})
			tAssert.False(ok)
		})

	})
})

var _ = Describe("completion coverage helpers", func() {
	It("covers directive and literal helper branches directly", func() {
		items, handled := directiveCompletionItems(document{}, "file:///doc.mace", "  [", protocol.Position{})
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "  [output = ", protocol.Position{})
		tAssert.True(handled)
		tAssert.Len(items, 2)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "  [output = schema, schema = ", protocol.Position{})
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "  [output = data,", protocol.Position{})
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		_, handled = directiveCompletionItems(document{}, "file:///doc.mace", "not a directive", protocol.Position{})
		tAssert.False(handled)

		content, ok := directivePrefix("  [schema")
		tAssert.True(ok)
		tAssert.Equal("schema", content)
		_, ok = directivePrefix("x [schema")
		tAssert.False(ok)
		_, ok = directivePrefix("  [schema]")
		tAssert.False(ok)
		state := parseDirectiveState([]string{"output = data", "schema = User", "schema_file = \"s", "parse = Input", "parse_file = \"p"})
		tAssert.Equal("data", state.outputMode)
		tAssert.True(state.seenSchema)
		tAssert.True(state.seenSchemaFile)
		tAssert.True(state.seenParse)
		tAssert.True(state.seenParseFile)
		tAssert.Empty(nextDirectiveDefinitions([]string{"output = schema"}))
		tAssert.NotEmpty(nextDirectiveDefinitions([]string{"output"}))

		tAssert.Equal("])", completionExpressionClosers("value: call([{}", len("value: call([{}")))
		tAssert.Equal("", completionExpressionClosers("abc", -1))
		tAssert.Equal([]byte{'('}, popCompletionDelimiter([]byte{'('}, '['))
		tAssert.Empty(popCompletionDelimiter([]byte{'('}, '('))

		model := completionModel{aliases: map[string]ast.TypeReference{"Alias": ast.PrimitiveType{Name: "int"}}, schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}
		tAssert.Equal("0", defaultLiteralForType(ast.NamedType{Name: "Alias"}, model, nil))
		tAssert.Equal("[]", defaultLiteralForType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, model, nil))
		tAssert.Equal("false", defaultLiteralForType(ast.PrimitiveType{Name: "boolean"}, model, nil))
		tAssert.Equal("0x0", defaultLiteralForType(ast.PrimitiveType{Name: "hex_int"}, model, nil))
		tAssert.Equal("0x0.0", defaultLiteralForType(ast.PrimitiveType{Name: "hex_float"}, model, nil))
		tAssert.Equal("\"on\"", defaultLiteralForType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"on"`}}}, model, nil))
		tAssert.Contains(defaultLiteralForType(ast.NamedType{Name: "User"}, model, nil), "name")
		tAssert.Equal("{}", defaultLiteralForType(ast.NamedType{Name: "Missing"}, model, nil))
	})

	It("covers import completion and filesystem helper branches directly", func() {
		workspace, err := os.MkdirTemp("", "mace-completion-coverage-*")
		tAssert.NoError(err)
		defer func() { tAssert.NoError(os.RemoveAll(workspace)) }()
		shared := filepath.Join(workspace, "shared.mace")
		tAssert.NoError(os.WriteFile(shared, []byte("|===|\nschema User: { name: string, };\n|===|\n[output = schema] { User: User, }\n"), 0o600))
		tAssert.NoError(os.Mkdir(filepath.Join(workspace, "nested"), 0o700))
		docPath := filepath.Join(workspace, "doc.mace")
		doc := document{text: "", analysis: analyzeDocumentAtInRoot("", docPath, workspace)}
		uri := protocol.DocumentUri(fileURI(docPath))

		items, handled := importCompletionItems(doc, `from "./`, uri, protocol.Position{})
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		items, handled = importCompletionItems(doc, `from "./shared.mace" import U`, uri, protocol.Position{})
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		items, handled = importCompletionItems(doc, `from "./shared.mace" imp`, uri, protocol.Position{})
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		items, handled = importCompletionItems(doc, `from "./shared.mace" nope`, uri, protocol.Position{})
		tAssert.True(handled)
		tAssert.Empty(items)
		_, handled = importCompletionItems(doc, `let x`, uri, protocol.Position{})
		tAssert.False(handled)

		entries, err := directoryEntries(workspace, workspace, "./", nil, true)
		tAssert.NoError(err)
		tAssert.NotEmpty(entries)
		_, err = directoryEntries(workspace, workspace, "./missing/", nil, true)
		tAssert.Error(err)
	})
})

var _ = Describe("completion remaining low-coverage helpers", func() {
	It("covers synthetic values and choice label helpers", func() {
		model := completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}
		tAssert.Equal(processor.ValueUnknown, syntheticCompletionValue(ast.PrimitiveType{Name: "string"}, model, 0).Kind)
		tAssert.Equal(processor.ValueInt, syntheticCompletionValue(ast.PrimitiveType{Name: "hex_int"}, model, 2).Kind)
		tAssert.Equal(processor.ValueFloat, syntheticCompletionValue(ast.PrimitiveType{Name: "hex_float"}, model, 2).Kind)
		tAssert.Equal(processor.ValueRecord, syntheticCompletionValue(ast.NamedType{Name: "User"}, model, 2).Kind)
		tAssert.Equal(processor.ValueString, syntheticCompletionValue(ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}, ast.PrimitiveType{Name: "string"}}}, model, 2).Kind)
		label, ok := unquotedStringChoiceLabel(`"choice"`)
		tAssert.True(ok)
		tAssert.Equal("choice", label)
		_, ok = unquotedStringChoiceLabel("choice")
		tAssert.False(ok)
		_, ok = unquotedStringChoiceLabel(`"bad`)
		tAssert.False(ok)
	})

	It("covers additional directive completion branches", func() {
		items, handled := directiveCompletionItems(document{}, "file:///doc.mace", "  [output", protocol.Position{})
		tAssert.True(handled)
		tAssert.NotEmpty(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "  [output = data, schema = ", protocol.Position{})
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "  [output = data, schema_file = \"", protocol.Position{})
		tAssert.True(handled)
		_ = items
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "  [output = data, parse = ", protocol.Position{})
		tAssert.True(handled)
		tAssert.Empty(items)
		items, handled = directiveCompletionItems(document{}, "file:///doc.mace", "  [output = data, parse_file = \"", protocol.Position{})
		tAssert.True(handled)
		_ = items
	})
})

var _ = Describe("completion delimiter coverage helpers", func() {
	It("covers quoted and mismatched expression closer branches", func() {
		tAssert.Equal("}", completionExpressionClosers(`value: { text: "unterminated`, len(`value: { text: "unterminated`)))
		tAssert.Equal(")", completionExpressionClosers(`value: ("escaped\\"`, len(`value: ("escaped\\"`)))
		tAssert.Equal("", completionExpressionClosers("value: []", len("value: []")))
		tAssert.Equal("", completionExpressionClosers("abc", len("abc")+1))
	})
})

var _ = Describe("completion import-as coverage helpers", func() {
	It("covers import-as data record helper branches directly", func() {
		model := completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}
		record, ok := importAsDataRecord(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}}, model)
		tAssert.True(ok)
		tAssert.Len(record.Fields, 1)
		record, ok = importAsDataRecord(ast.File{Output: ast.OutputBlock{DataFields: []ast.OutputField{{Name: "value", Value: ast.IntLiteral{Lexeme: "1"}}}}}, completionModel{})
		tAssert.True(ok)
		tAssert.Equal("value", record.Fields[0].Name)
		_, ok = importAsDataRecord(ast.File{}, completionModel{})
		tAssert.False(ok)
	})

	It("covers imported member root failure branches directly", func() {
		_, _, ok := importedMemberCompletionRootType(ast.File{}, nil, ".", ".", nil)
		tAssert.False(ok)
		file := ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./missing.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "data"}}}}
		_, _, ok = importedMemberCompletionRootType(file, []string{"other"}, ".", ".", nil)
		tAssert.False(ok)
		_, _, ok = importedMemberCompletionRootType(file, []string{"data"}, ".", ".", nil)
		tAssert.False(ok)
	})
})

var _ = Describe("completion line and placeholder coverage helpers", func() {
	It("covers current-line and placeholder helper branches", func() {
		text := "first\nsecond line"
		tAssert.Equal("sec", currentLinePrefix(text, protocol.Position{Line: 1, Character: 3}))
		tAssert.Equal("", currentLinePrefix(text, protocol.Position{Line: 9, Character: 1}))
		tAssert.Equal("ond line", currentLineSuffix(text, protocol.Position{Line: 1, Character: 3}))
		tAssert.Equal("first", currentLineSuffix(text, protocol.Position{Line: 9, Character: 1}))
		pos, ok := completionPlaceholderPosition("a + b", protocol.Position{Character: 3}, "+")
		tAssert.True(ok)
		tAssert.Equal(protocol.Position{Character: 3}, pos)
		_, ok = completionPlaceholderPosition("a + b", protocol.Position{Character: 5}, "+")
		tAssert.False(ok)
		_, ok = completionPlaceholderPosition("a + b", protocol.Position{Line: 2}, "+")
		tAssert.False(ok)
	})
})

var _ = Describe("completion placeholder path coverage helpers", func() {
	It("covers placeholder and expression path branches", func() {
		path, ok := placeholderPath(ast.Identifier{Name: completionPlaceholderIdentifier})
		tAssert.True(ok)
		tAssert.Empty(path)
		path, ok = placeholderPath(ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: completionPlaceholderIdentifier})
		tAssert.True(ok)
		tAssert.Equal([]string{"user"}, path)
		path, ok = placeholderPath(ast.MemberAccess{Target: ast.Identifier{Name: completionPlaceholderIdentifier}, Name: "name"})
		tAssert.True(ok)
		tAssert.Equal([]string{"name"}, path)
		path, ok = placeholderPath(ast.ArrayLiteral{Elements: []ast.Expression{ast.RecordLiteral{Fields: []ast.RecordField{{Name: "field", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}})
		tAssert.True(ok)
		tAssert.Equal([]string{completionArrayPathSegment, "field"}, path)
		path, ok = placeholderPath(ast.InfixExpression{Left: ast.Identifier{Name: "x"}, Right: ast.PrefixExpression{Right: ast.Identifier{Name: completionPlaceholderIdentifier}}})
		tAssert.True(ok)
		tAssert.Empty(path)
		path, ok = placeholderPath(ast.ConditionalExpression{Condition: ast.Identifier{Name: "condition"}, Then: ast.Identifier{Name: "then"}, Else: ast.Identifier{Name: completionPlaceholderIdentifier}})
		tAssert.True(ok)
		tAssert.Empty(path)
		_, ok = placeholderPath(ast.StringLiteral{})
		tAssert.False(ok)
		_, ok = expressionPath(ast.Identifier{})
		tAssert.False(ok)
		_, ok = expressionPath(ast.MemberAccess{Target: ast.Identifier{Name: "user"}})
		tAssert.False(ok)
		path, ok = expressionPath(ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: "name"})
		tAssert.True(ok)
		tAssert.Equal([]string{"user", "name"}, path)
	})

	It("covers placeholder completion type helpers directly", func() {
		model := completionModel{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}
		file := ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{HasValue: true, Type: ast.NamedType{Name: "User"}, Value: ast.MemberAccess{Target: ast.Identifier{Name: completionPlaceholderIdentifier}, Name: "name"}}}}}
		_, path, ok := placeholderCompletionType(file, model)
		tAssert.True(ok)
		tAssert.Equal([]string{"name"}, path)
		_, _, ok = placeholderCompletionType(ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{HasValue: true, Type: ast.NamedType{Name: "Missing"}, Value: ast.MemberAccess{Target: ast.Identifier{Name: completionPlaceholderIdentifier}, Name: "name"}}}}}, model)
		tAssert.False(ok)
		outputFile := ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.Identifier{Name: completionPlaceholderIdentifier}}}}}
		_, path, ok = placeholderOutputCompletionType(outputFile, model)
		tAssert.True(ok)
		tAssert.Equal([]string{"name"}, path)
		_, _, ok = placeholderOutputCompletionType(ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, model)
		tAssert.False(ok)
	})
})

var _ = Describe("completion parse-file helper coverage", func() {
	It("covers parse input and member type helper branches directly", func() {
		model := completionModel{schemas: map[string]ast.RecordType{"Input": {Fields: []ast.SchemaField{{Name: "name", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}}}}
		var typeRef ast.TypeReference
		record, ok := parseInputCompletionRecord(ast.File{Output: ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Input"}}}}, model, ".", ".", nil)
		tAssert.True(ok)
		tAssert.Len(record.Fields, 1)
		_, ok = parseInputCompletionRecord(ast.File{}, model, ".", ".", nil)
		tAssert.False(ok)
		_, ok, guarded := parseMemberCompletionType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}}, []string{"name"}, completionModel{}, map[string]struct{}{})
		tAssert.False(ok)
		tAssert.True(guarded)
		_, ok, guarded = parseMemberCompletionType(ast.PrimitiveType{Name: "string"}, []string{"name"}, completionModel{}, nil)
		tAssert.False(ok)
		tAssert.False(guarded)
		typeRef, ok, guarded = parseMemberCompletionType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Optional: true, Type: ast.PrimitiveType{Name: "string"}}}}, []string{"name"}, completionModel{}, map[string]struct{}{"name": {}})
		tAssert.True(ok)
		tAssert.False(guarded)
		tAssert.IsType(ast.PrimitiveType{}, typeRef)
	})

	It("covers parse_file imported record helpers with temporary files", func() {
		workspace, err := os.MkdirTemp("", "mace-parse-file-completion-*")
		tAssert.NoError(err)
		defer func() { tAssert.NoError(os.RemoveAll(workspace)) }()
		writeCompletionFile := func(name string, contents string) string {
			path := filepath.Join(workspace, name)
			tAssert.NoError(os.WriteFile(path, []byte(contents), 0o600))
			return path
		}
		_ = writeCompletionFile("schema.mace", `|===|
schema User: { name: string, };
|===|
[output = schema]
{
  User: User,
}`)
		directives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}
		defs := parseFileOutputDeclarationDefinitions(directives, workspace, workspace, map[string]completionModel{})
		tAssert.NotEmpty(defs)
		record, ok := parseFileOutputSchemaRecord(directives, workspace, workspace, map[string]completionModel{})
		tAssert.True(ok)
		tAssert.Len(record.Fields, 1)
		record, ok = parseFileOutputExportedRecord(directives, workspace, workspace, map[string]completionModel{})
		tAssert.True(ok)
		tAssert.Len(record.Fields, 1)
		badDirectives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./missing.mace"`}}
		tAssert.Empty(parseFileOutputDeclarationDefinitions(badDirectives, workspace, workspace, map[string]completionModel{}))
		_, ok = parseFileOutputSchemaRecord(badDirectives, workspace, workspace, map[string]completionModel{})
		tAssert.False(ok)
		_, ok = parseFileOutputExportedRecord(badDirectives, workspace, workspace, map[string]completionModel{})
		tAssert.False(ok)
		tAssert.True(hasParseFileOnlyOutput(directives))
		tAssert.False(hasParseFileOnlyOutput(append(directives, ast.OutputDirective{Kind: ast.OutputDirectiveSchema, Value: "User"})))
		tAssert.True(outputKeyCompletionContext("  name"))
		tAssert.False(outputKeyCompletionContext("  name:"))
	})
})
