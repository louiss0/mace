package binding

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/processor"
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
	It("wraps root records in a Mace output block", func() {
		source, err := MarshalOutput(map[string]any{
			"name": "Ada",
			"age":  27,
		})
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  age: 27;
  name: "Ada";
}`, source)
	})

	It("rejects non-record roots", func() {
		_, err := MarshalOutput([]int{1, 2, 3})
		tAssert.Error(err)
	})
})
