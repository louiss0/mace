package processor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestProcessor(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Processor Suite")
}

var _ = Describe("Processor", func() {
	DescribeTable("processes valid files",
		func(input string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.NoError(err)
		},
		Entry("minimal output block", "[output = data] {}"),
		Entry("int variable with arithmetic", `|===|
int total = 1 + 2 * 3;
|===|
[output = data] {}`),
		Entry("imports and script block", `from "base.mace" import BaseUser;
|===|
type Name = string;
schema User = { name: string; };
string user = "Ada";
|===|
[output = data, schema = User]
{ name: user; }`),
	)

	DescribeTable("rejects invalid directives",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing output directive", `|===|
schema User = { name: string; };
|===|
[schema = User] {}`, "missing output directive"),
		Entry("duplicate output directive", "[output = data, output = schema] {}", "duplicate output directive"),
		Entry("unknown schema in directive", "[output = data, schema = Missing] {}", "unknown schema"),
	)

	DescribeTable("rejects invalid declarations",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown type reference", `|===|
Unknown value = 1;
|===|
[output = data] {}`, "unknown type"),
		Entry("int type mismatch", `|===|
int total = 1.5;
|===|
[output = data] {}`, "type mismatch"),
		Entry("mixed numeric expression", `|===|
float total = 1 + 2.0;
|===|
[output = data] {}`, "type mismatch"),
		Entry("duplicate imports", `from "base.mace" import User, User;
[output = data] {}`, "duplicate import"),
		Entry("duplicate declaration name", `|===|
type User = string;
schema User = { name: string; };
|===|
[output = data] {}`, "duplicate declaration"),
	)
})
