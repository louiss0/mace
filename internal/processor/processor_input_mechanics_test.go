package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Input records", func() {
	It("parses injection records through the compatibility helper", func() {
		record, err := ParseInjectionRecord(`{ name: "Ada"; enabled: true; }`)
		tAssert.NoError(err)
		assertExpectedValue(record["name"], expectedValue{kind: ValueString, string: "Ada"})
		assertExpectedValue(record["enabled"], expectedValue{kind: ValueBoolean, bool: true})
	})
	It("rejects trailing tokens after the record literal", func() {
		_, err := ParseInputRecord(`{ a: 1; } garbage`)
		tAssert.ErrorContains(err, "unexpected token after expression")
	})
})
