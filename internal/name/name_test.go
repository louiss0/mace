package name

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeName(t *testing.T) {
	tests := map[string]struct {
		raw      string
		expected Name
	}{
		"ascii":                          {raw: "name", expected: "name"},
		"nfc":                            {raw: "naïve", expected: "naïve"},
		"nfd":                            {raw: "naïve", expected: "naïve"},
		"multiple marks":                 {raw: "A\u030a\u0301", expected: "Ǻ"},
		"no composed form":               {raw: "a\u0483", expected: "a\u0483"},
		"non latin":                      {raw: "ユーザー", expected: "ユーザー"},
		"case remains distinct":          {raw: "Name", expected: "Name"},
		"compatibility remains distinct": {raw: "①", expected: "①"},
		"confusable remains distinct":    {raw: "раураl", expected: "раураl"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.expected, NormalizeName(test.raw))
		})
	}

	assert.Equal(t, NormalizeName("café"), NormalizeName("cafe\u0301"))
	assert.NotEqual(t, NormalizeName("café"), NormalizeName("cafe"))
}
