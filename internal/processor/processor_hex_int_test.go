package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("hex_int", func() {
	It("formats scalar helper values", func() {
		valueKey := scalarValueKey
		valueDisplay := scalarValueDisplay
		floatLiteral := decimalFloatLiteral

		key, ok := valueKey(Value{Kind: ValueFloat, Float: 1.5})
		tAssert.True(ok)
		tAssert.Contains(key, "float:")
		_, ok = valueKey(Value{Kind: ValueRecord})
		tAssert.False(ok)
		tAssert.Equal("null", valueDisplay(Value{Kind: ValueNull}))
		tAssert.Equal("unknown", valueDisplay(Value{Kind: ValueRecord}))
		tAssert.Equal("2.0", floatLiteral(2))
		tAssert.Equal("1.5", floatLiteral(1.5))
		key, ok = valueKey(Value{Kind: ValueNull})
		tAssert.True(ok)
		tAssert.Equal("null", key)
	})

	It("compares scalar values for equality", func() {
		equalValues := valuesEqual

		equal, err := equalValues(Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		tAssert.NoError(err)
		tAssert.True(equal)

		equal, err = equalValues(Value{Kind: ValueFloat, Float: 1.5}, Value{Kind: ValueFloat, Float: 2.5})
		tAssert.NoError(err)
		tAssert.False(equal)

		equal, err = equalValues(Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexFloat, Float: 2})
		tAssert.NoError(err)
		tAssert.True(equal)

		equal, err = equalValues(Value{Kind: ValueHexFloat, Float: 3}, Value{Kind: ValueHexInt, Int: 2})
		tAssert.NoError(err)
		tAssert.False(equal)

		equal, err = equalValues(Value{Kind: ValueBoolean, Boolean: true}, Value{Kind: ValueBoolean, Boolean: true})
		tAssert.NoError(err)
		tAssert.True(equal)

		equal, err = equalValues(Value{Kind: ValueString, String: "Ada"}, Value{Kind: ValueString, String: "Bob"})
		tAssert.NoError(err)
		tAssert.False(equal)

		_, err = equalValues(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		tAssert.ErrorContains(err, "unsupported equality")
	})
})
