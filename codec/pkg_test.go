package codec

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestBinding(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Binding Suite")
}

type userProfile struct {
	Nickname string `json:"nickname,omitempty"`
	Level    int    `json:"level"`
}

type userConfig struct {
	Name    string      `json:"name"`
	Enabled bool        `json:"enabled"`
	Profile userProfile `json:"profile"`
	Scores  []int       `json:"scores,omitempty"`
	private string
}

type decodedConfig struct {
	Name    string                 `json:"name"`
	Enabled bool                   `json:"enabled"`
	Profile decodedProfile         `json:"profile"`
	Flags   []bool                 `json:"flags"`
	Meta    map[string]interface{} `json:"meta"`
}

type decodedProfile struct {
	Level int     `json:"level"`
	Alias *string `json:"alias,omitempty"`
}

var _ = Describe("OutputMap", func() {
	It("converts evaluated output to nested Go maps and slices", func() {
		result, err := Parse(`|===|
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

var _ = Describe("Parse", func() {
	It("returns schema outputs through the public binding result", func() {
		result, err := Parse(`[output = schema]
{
  name: string;
  age?: int;
}`)
		tAssert.NoError(err)
		tAssert.Empty(result.Data)
		tAssert.Equal(map[SchemaField]string{
			{Name: "name"}:                "string",
			{Name: "age", Optional: true}: "int",
		}, result.Schema)
	})

	It("parses files through the public binding result", func() {
		tempDir, err := os.MkdirTemp("", "mace-binding-*")
		tAssert.NoError(err)

		path := filepath.Join(tempDir, "config.mace")
		err = os.WriteFile(path, []byte(`[output = data] { value: 2 + 2; }`), 0o600)
		tAssert.NoError(err)

		result, err := ParseFile(path)
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{"value": int64(4)}, result.Data)
	})
})

var _ = Describe("Marshal", func() {
	It("marshals native Go maps into canonical Mace records", func() {
		source, err := Marshal(map[string]any{
			"name":   "Ada",
			"active": true,
			"scores": []int{1, 2, 3},
			"profile": map[string]any{
				"level": 2,
			},
		})
		tAssert.NoError(err)
		tAssert.Equal(`{
  active: true;
  name: "Ada";
  profile: {
    level: 2;
  };
  scores: [1, 2, 3];
}`, source)
	})

	It("marshals exported struct fields and respects json tags", func() {
		source, err := Marshal(userConfig{
			Name:    "Ada",
			Enabled: true,
			Profile: userProfile{Level: 3},
		})
		tAssert.NoError(err)
		tAssert.Equal(`{
  name: "Ada";
  enabled: true;
  profile: {
    level: 3;
  };
}`, source)
	})

	It("rejects nil values", func() {
		_, err := Marshal(map[string]any{"value": nil})
		tAssert.Error(err)
	})
})

var _ = Describe("MarshalOutput", func() {
	It("returns a bare output record when data is implicit", func() {
		source, err := MarshalOutput(map[string]any{
			"name": "Ada",
			"age":  27,
		})
		tAssert.NoError(err)
		tAssert.Equal(`{
  age: 27;
  name: "Ada";
}`, source)
	})

	It("rejects non-record roots", func() {
		_, err := MarshalOutput([]int{1, 2, 3})
		tAssert.Error(err)
	})
})

var _ = Describe("Unmarshal", func() {
	It("unmarshals output records into structs", func() {
		input := `[output = data]
{
  name: "Ada";
  enabled: true;
  profile: {
    level: 3;
  };
  flags: [true, false];
  meta: {
    retries: 2;
  };
}`

		var config decodedConfig
		err := Unmarshal(input, &config)
		tAssert.NoError(err)
		tAssert.Equal("Ada", config.Name)
		tAssert.Equal(true, config.Enabled)
		tAssert.Equal(3, config.Profile.Level)
		tAssert.Nil(config.Profile.Alias)
		tAssert.Equal([]bool{true, false}, config.Flags)
		tAssert.Equal(map[string]interface{}{"retries": int64(2)}, config.Meta)
	})

	It("unmarshals output records into maps", func() {
		input := `[output = data]
{
  age: 27;
  name: "Ada";
}`

		target := map[string]any{}
		err := Unmarshal(input, &target)
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{
			"age":  int64(27),
			"name": "Ada",
		}, target)
	})

	It("rejects non-pointer targets", func() {
		err := Unmarshal(`[output = data] { value: 1; }`, map[string]any{})
		tAssert.Error(err)
	})
})
