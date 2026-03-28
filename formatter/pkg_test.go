package formatter

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser"
	"github.com/louiss0/mace/parser/ast"
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
type Name = string;
schema User = { name: string; age?: int; };
injectable string user = "Ada";
|===|
[output = data, schema = User]
{ name: user; age: 1 + 2 * 3; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`from "./base.mace" import User, Config;

|===|
type Name = string;
schema User = {
  name: string;
  age?: int;
};
injectable string user = "Ada";
|===|
[output = data, schema = User]
{
  name: user;
  age: 1 + 2 * 3;
}`, output)
	})

	It("preserves expression semantics with parentheses", func() {
		file, err := parseMaceFile(`[output = data] { result: (1 + 2) * (3 - 4 ? 5 : 6); }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  result: (1 + 2) * (3 - 4 ? 5 : 6);
}`, output)
	})
})
