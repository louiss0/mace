package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Documentation", func() {
	It("accepts documentation declarations", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema User: { name: string, };

type Status: choice["Pending"];
type Name: string;
string greeting = "Hello";
User profile = {
  name: greeting,
};

schema_doc User {
  summary: "Represents a user.";
};

gen_doc Status {
  summary: "Represents a status.";
};
|===|`))
		tAssert.NoError(err)
	})

	It("rejects invalid documentation targets", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Status: string;

schema_doc Status {
  summary: "Invalid target.";
};
|===|`))
		tAssert.Error(err)
		tAssert.ErrorContains(err, "schema_doc target")
	})
})
