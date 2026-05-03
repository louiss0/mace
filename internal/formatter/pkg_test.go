package formatter

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser"
	"github.com/louiss0/mace/internal/parser/ast"
)

var tAssert *assert.Assertions

func TestFormatter(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Formatter Suite")
}

func parseMaceFile(input string) (ast.File, error) {
	lexerInstance := lexer.New(input)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return ast.File{}, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			break
		}
	}

	return parser.New(tokens).ParseFile()
}

var _ = Describe("FormatFile", func() {
	It("formats imports, script declarations, and output", func() {
		file, err := parseMaceFile(`from "./base.mace" import User, Config;
|===|
type Name: string;
schema User: { name: string; age?: int; };
enum Fruit: string {
  Apple /# Default apple,
  Strawberry = "strawberry" /# Explicit strawberry
};
injectable string user = "Ada";
|===|
[output = data, schema = User]
{ name: user; age: 1 + 2 * 3; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==================================================|
from "./base.mace" import User, Config;
type Name: string;
schema User: {
  name: string,
  age?: int
}
enum Fruit: string {
  Apple /# Default apple,
  Strawberry = "strawberry" /# Explicit strawberry
}
injectable string user = "Ada";
|==================================================|
[output = data, schema = User]
{
  name: user,
  age: 1 + 2 * 3
}`, output)
	})

	It("formats documentation declarations and inline output docs", func() {
		file, err := parseMaceFile(`|===|
schema User: { name: string; };
schema_doc User {
  summary: "Represents a user.",
  description: """
# User
""",
};
|===|
[output = schema]
"""
# Public User Output
"""
{ user: User; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|================================|
schema User: {
  name: string
}
schema_doc User {
  summary: "Represents a user.",
  description: """
# User
""",
};
|================================|
[output = schema]
"""
# Public User Output
"""
{
  user: User
}`, output)
	})

	It("preserves expression semantics with parentheses", func() {
		file, err := parseMaceFile(`[output = data] { result: (1 + 2) * (3 - 4 ? 5 : 6); }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  result: (1 + 2) * (3 - 4 ? 5 : 6)
}`, output)
	})

	It("formats array access expressions", func() {
		file, err := parseMaceFile(`[output = data] { result: users [ 0 ] . name; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  result: users[0].name
}`, output)
	})

	It("formats bare output blocks without injecting a directive", func() {
		file, err := parseMaceFile(`{ result: 1 + 2; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`{
  result: 1 + 2
}`, output)
	})

	It("keeps arrays and nested records expanded instead of collapsing them", func() {
		file, err := parseMaceFile(`[output = data]
{
  result: [{ profile: { name: "Ada"; }; }, { profile: { name: "Bob"; }; }];
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  result: [
    {
      profile: {
        name: "Ada"
      }
    },
    {
      profile: {
        name: "Bob"
      }
    }
  ]
}`, output)
	})

	It("formats schema-mode output blocks with type references", func() {
		file, err := parseMaceFile(`[output = schema]
{
  name: string;
  tags?: array<string>;
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  name: string,
  tags?: array<string>
}`, output)
	})

	It("formats variant type references", func() {
		file, err := parseMaceFile(`|===|
type Value: variant[string, int];
|===|
[output = schema]
{
  value: variant[string, int];
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|=================================|
type Value: variant[string, int];
|=================================|
[output = schema]
{
  value: variant[string, int]
}`, output)
	})

	It("formats union type references", func() {
		file, err := parseMaceFile(`|===|
type Value: union[Profile, Audit];
|===|
[output = schema]
{
  value: union[Profile, Audit];
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==================================|
type Value: union[Profile, Audit];
|==================================|
[output = schema]
{
  value: union[Profile, Audit]
}`, output)
	})
})
