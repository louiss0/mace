package analyzer

import (
	"fmt"
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

	Describe("Parse completions", func() {
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
			details := map[string]string{}
			for _, item := range items {
				if item.Detail != nil {
					details[item.Label] = *item.Detail
				}
			}

			tAssert.Contains(labels, "env")
			tAssert.Contains(labels, "region")
			tAssert.Equal("string", details["env"])
			tAssert.Equal("string", details["region"])
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
  Runtime: { env: string; region: string; };
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
			tAssert.NotContains(labels, "region")
			tAssert.Contains(labels, "Runtime")
		})

		It("only suggests top-level parse schema fields as output variables", func() {
			text := `|===|
schema Runtime: {
  env: string;
  profile: { name: string; email: string; };
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

			tAssert.Contains(labels, "env")
			tAssert.Contains(labels, "profile")
			tAssert.Equal("string", details["env"])
			tAssert.Equal("{ name: string, email: string }", details["profile"])
		})

		It("only suggests top-level parse_file schema fields as output variables", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-top-level-*")
			tAssert.NoError(err)
			defer func() {
				_ = os.RemoveAll(workspace)
			}()

			runtimePath := filepath.Join(workspace, "runtime.mace")
			tAssert.NoError(os.WriteFile(runtimePath, []byte(`[output = schema]
{
  Runtime: {
    env: string;
    profile: { name: string; email: string; };
  };
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
			tAssert.Contains(labels, "Runtime")
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
  Runtime: { user: { name: string; home: { street: string; city: string; }; }; };
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

		It("does not suggest schema names as output expressions", func() {
			text := `|===|
schema Runtime: { env: string; };
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
  Runtime: { env: string; };
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
    name: "pi-prompt-form";
    root: "libs/pi-prompt-form";
  };
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
    name: "pi-prompt-form";
    root: "libs/pi-prompt-form";
  };
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
    value: "one";
    level2: {
      value: "two";
      level3: {
        value: "three";
        level4: {
          value: "four";
          level5: {
            value: "five";
          };
        };
      };
    };
  };
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
    value: string;
    level2: {
      value: string;
      level3: {
        value: string;
        level4: {
          value: string;
          level5: {
            value: string;
          };
        };
      };
    };
  };
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
			Entry("level 1", "level1.", []string{"value", "level2"}),
			Entry("level 2", "level1.level2.", []string{"value", "level3"}),
			Entry("level 3", "level1.level2.level3.", []string{"value", "level4"}),
			Entry("level 4", "level1.level2.level3.level4.", []string{"value", "level5"}),
			Entry("level 5", "level1.level2.level3.level4.level5.", []string{"value"}),
		)

		It("suggests import-as schema aliases in directive completions", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-import-as-schema-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = schema]
{
  Package: {
    name: string;
    version: string;
  };
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

		It("suggests parse variables when previous output fields use commas", func() {
			text := `|===|
schema Runtime: { env: string; region: string; };
|===|
[output = data, parse = Runtime]
{
  name: "mace",
  result:
}`

			position := protocol.Position{
				Line:      6,
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

		It("suggests parse_file variables when previous output fields use commas", func() {
			workspace, err := os.MkdirTemp("", "mace-analyzer-parse-file-commas-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			runtimePath := filepath.Join(workspace, "runtime.mace")
			tAssert.NoError(os.WriteFile(runtimePath, []byte(`[output = schema]
{
  Runtime: { env: string; region: string; };
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

			tAssert.Contains(labels, "Runtime")
		})

		DescribeTable("completes recursive nested parse values through member access",
			func(cursorExpr string, expectedLabels []string) {
				text := fmt.Sprintf(`|===|
schema Contact: {
  email: string;
  phone: string;
};
schema Profile: {
  title: string;
  contact: Contact;
};
schema User: {
  name: string;
  manager: User;
  profile: Profile;
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
			Entry("completes fields on recursive parsed record", "manager.", []string{"manager", "name", "profile"}),
			Entry("completes fields on second recursive level", "manager.manager.", []string{"manager", "name", "profile"}),
			Entry("completes nested profile fields on recursive parsed record", "manager.profile.", []string{"contact", "title"}),
			Entry("completes nested contact fields on second recursive level", "manager.manager.profile.contact.", []string{"email", "phone"}),
			Entry("completes nested contact fields on deep recursive level without infinite traversal", "manager.manager.manager.manager.profile.contact.", []string{"email", "phone"}),
		)

		It("completes nested record fields via member access on parse variable", func() {
			text := `|===|
schema Address: {
  street: string;
  city: string;
};
schema User: {
  name: string;
  home: Address;
};
|===|

[output = data, parse = User]
{
  result: home.
}`

			position := protocol.Position{
				Line:      13,
				Character: uint32(len("  result: home.")),
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
  lat: float;
  lon: float;
};
schema Address: {
  street: string;
  location: Location;
};
schema User: {
  name: string;
  home: Address;
};
|===|

[output = data, parse = User]
{
  result: home.location.
}`

			position := protocol.Position{
				Line:      17,
				Character: uint32(len("  result: home.location.")),
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
  name: string;
  age: int;
};
|===|

[output = data, parse = User]
{
  result: name.
}`

			position := protocol.Position{
				Line:      9,
				Character: uint32(len("  result: name.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)

			tAssert.Empty(items)
		})

		It("returns no completions for member access on an array parse variable field", func() {
			text := `|===|
schema User: {
  name: string;
  tags: array<string>;
};
|===|

[output = data, parse = User]
{
  result: tags.
}`

			position := protocol.Position{
				Line:      9,
				Character: uint32(len("  result: tags.")),
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
  User: { name: string; home: { street: string; city: string; }; };
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `[output = data, parse_file = "./schema.mace"]
{
  result: User.home.
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      2,
				Character: uint32(len("  result: User.home.")),
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
  name: string;
  root: string;
};
schema Workspace: {
  name: string;
  root: string;
};
|===|
[output = schema]
{
  project: Project;
  workspace: Workspace;
}`), 0o644))
			documentPath := filepath.Join(workspace, "document.mace")
			text := `[output = data, parse_file = "./schema.mace"]
{
  result: project.
}`
			tAssert.NoError(os.WriteFile(documentPath, []byte(text), 0o644))

			position := protocol.Position{
				Line:      2,
				Character: uint32(len("  result: project.")),
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
  name: string;
  manager?: User;
};
|===|

[output = data, parse = User]
{
  result: manager.
}`
			position := protocol.Position{
				Line:      9,
				Character: uint32(len("  result: manager.")),
			}
			documentPath := filepath.Join("workspace", "document.mace")
			snapshot := AnalyzeCompletionContext(text, documentPath, position)

			items := CompletionItems(text, snapshot, protocol.DocumentUri(fileURI(documentPath)), position)
			tAssert.Empty(items)
		})

		It("provides completions for optional parse field member access inside 'in' guard", func() {
			text := `|===|
schema User: {
  name: string;
  manager?: User;
};
|===|

[output = data, parse = User]
{
  result: "manager" in input ? manager.
}`
			position := protocol.Position{
				Line:      9,
				Character: uint32(len(`  result: "manager" in input ? manager.`)),
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

	It("suggests choice values after earlier self member access", func() {
		text := `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { previous: Fruit; favorite_fruit: Fruit; };
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
