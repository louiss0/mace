package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("hex_float", func() {
	DescribeTable("rejects invalid hexadecimal expressions",
		func(input string, expected string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.Contains(err.Error(), expected)
		},
		Entry("mixed decimal and hex arithmetic", wrapScriptWithOutput(`|===|
hex_int a = 0x10;
int b = 2;
hex_int c = a + b;
|===|`), "expected hexadecimal operands for operator"),
		Entry("hex float modulo", wrapScriptWithOutput(`|===|
hex_float a = 0x2.8;
hex_float b = 0x0.8;
hex_float c = a % b;
|===|`), "requires hex_int operands"),
		Entry("hex and decimal comparison", `[output = data] { result: 0x10 > 16; }`, "expected operands from the same numeric family"),
		Entry("hex bitwise not", `[output = data] { result: ~0x0F; }`, "expected int after '~'"),
	)
})
