package processor

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Documentation", func() {
	DescribeTable("accepts documentation declarations",
		func(input string) {
			processor := New()
			_, err := processor.Process(wrapScriptWithOutput(input))
			tAssert.NoError(err)
		},
		Entry("documentation declarations", `|===|
schema User: { name: string, };

type Status: choice["Pending"];
type Name: string;
string greeting = "Hello";
User profile = {
  name: greeting,
};

schema_doc User {
  summary: "Represents a user.",
  description: """
# User
""",
};

gen_doc Status {
  summary: "Represents a status.",
};

schema_doc profile {
  summary: "Profile object.",
  fields: {
    name: "Profile name.",
  },
};

gen_doc Name {
  summary: "Represents a name.",
};

gen_doc greeting {
  summary: "Rendered greeting.",
};
|===|`),
	)

	DescribeTable("rejects invalid documentation declarations",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(wrapScriptWithOutput(input))
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("interpolation rejects type references", `|===|
type UserName: string;
string value = "$(UserName)";
|===|`, "type reference"),
		Entry("schema_doc rejects duplicate keys", `|===|
schema User: { name: string, };

schema_doc User {
  summary: "One",
  summary: "Two",
};
|===|`, "duplicate schema_doc entry"),
		Entry("schema_doc rejects type targets", `|===|
type Status: string;

schema_doc Status {
  summary: "Invalid target.",
};
|===|`, "schema_doc target"),
		Entry("schema_doc rejects scalar variables", `|===|
string greeting = "Hello";

schema_doc greeting {
  summary: "Invalid target.",
};
|===|`, "schema_doc target \"greeting\" must reference a schema or object-valued variable"),
		Entry("gen_doc rejects object variables", `|===|
schema User: {
  name: string,
};

User profile = {
  name: "Ada",
};

gen_doc profile {
  summary: "Invalid target.",
};
|===|`, "gen_doc target \"profile\" must reference a type or non-object variable"),
		Entry("type inline description conflicts with gen_doc", `|===|
type Name: string /# Duplicate inline docs;

gen_doc Name {
  summary: "Public name type",
};
|===|`, "already documented"),
		Entry("schema field inline description conflicts with schema_doc fields", `|===|
schema User: {
  name: string /# Duplicate inline docs,
};

schema_doc User {
  fields: {
    name: "The user's display name",
  },
};
|===|`, "already documented"),
		Entry("schema_doc fields reject unknown schema fields", `|===|
schema User: {
  name: string,
};

schema_doc User {
  fields: {
    age: "Unknown field",
  },
};
|===|`, "does not exist"),
		Entry("gen_doc fields reject type targets", `|===|
type Name: string;

gen_doc Name {
  fields: {
    value: "Nope",
  },
};
|===|`, "fields entry is only allowed in schema_doc"),
		Entry("schema_doc must appear after its schema declaration", `|===|
schema_doc User {
  summary: "Late-bound docs",
};

schema User: {
  name: string,
};
|===|`, "must appear after its schema or object-valued variable declaration"),
		Entry("gen_doc must appear after its type declaration", `|===|
gen_doc Name {
  summary: "Late-bound docs",
};

type Name: string;
|===|`, "must appear after its type or non-object variable declaration"),
		Entry("gen_doc must appear after its variable declaration", `|===|
gen_doc name {
  summary: "Late-bound docs",
};

string name = "Ada";
|===|`, "must appear after its type or non-object variable declaration"),
	)

	DescribeTable("rejects invalid output documentation",
		func(input, message string) {
			processor := New()
			_, err := processor.Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("output inline doc requires a directive list", `"""
Invalid: no directive list
"""
{
}
`, "expected output directive"),
		Entry("output inline doc rejects interpolation", `[output = 'schema']
"""$(name)"""
{
  name: string,
}
`, "interpolation is not allowed"),
	)

	It("loads doc fixtures", func() {
		processor := New()
		_, err := processor.ProcessFile("../../fixtures/processor/doc_fixtures/public_contract.mace")
		tAssert.NoError(err)
	})
})
