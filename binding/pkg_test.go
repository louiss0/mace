package binding

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser"
	"github.com/louiss0/mace/parser/ast"
	"github.com/louiss0/mace/processor"
)

var tAssert *assert.Assertions

func TestBinding(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Binding Suite")
}

func parseFile(input string) (ast.File, error) {
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

var _ = Describe("OutputMap", func() {
	It("converts evaluated output to nested Go maps and slices", func() {
		result, err := processor.New().Process(`|===|
int age = 27;
|===|
[output = data]
{
  name: "Ada";
  profile: { age: age; active: true; };
  scores: [1, 2, 3];
}`)
		tAssert.NoError(err)

		output := OutputMap(result)
		tAssert.Equal(map[string]any{
			"name": "Ada",
			"profile": map[string]any{
				"age":    int64(27),
				"active": true,
			},
			"scores": []any{int64(1), int64(2), int64(3)},
		}, output)
	})
})

var _ = Describe("GenerateStructs", func() {
	It("generates Go structs from schemas and type aliases", func() {
		file, err := parseFile(`|===|
type Name = string;
type Scores = array<int>;
schema Profile = { nickname?: Name; scores: Scores; };
schema User = { name: Name; profile: Profile; enabled?: boolean; };
|===|
[output = data] {}`)
		tAssert.NoError(err)

		source, err := GenerateStructs(file, "models")
		tAssert.NoError(err)
		tAssert.Equal(`package models

type Profile struct {
	Nickname *string `+"`json:\"nickname,omitempty\"`"+`
	Scores   []int64 `+"`json:\"scores\"`"+`
}

type User struct {
	Name    string  `+"`json:\"name\"`"+`
	Profile Profile `+"`json:\"profile\"`"+`
	Enabled *bool   `+"`json:\"enabled,omitempty\"`"+`
}
`, source)
	})

	It("fails when no schemas are available", func() {
		file, err := parseFile(`[output = data] {}`)
		tAssert.NoError(err)

		_, err = GenerateStructs(file, "models")
		tAssert.Error(err)
	})
})
