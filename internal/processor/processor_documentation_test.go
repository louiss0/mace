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

alias Status: choice["Pending"];
alias Name: string;
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
alias UserName: string;
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
alias Status: string;

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
alias Name: string /# Duplicate inline docs;

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
alias Name: string;

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

alias Name: string;
|===|`, "must appear after its type or non-object variable declaration"),
		Entry("gen_doc must appear after its variable declaration", `|===|
gen_doc name {
  summary: "Late-bound docs",
};

string name = "Ada";
|===|`, "must appear after its type or non-object variable declaration"),
	)

	DescribeTable("accepts output doc directives for each string variation",
		func(mode, documentation string) {
			field := `name: "Ada",`
			if mode == "schema" {
				field = "name: string,"
			}

			_, err := New().Process(`[output = '` + mode + `', doc = ` + documentation + `]
{
  ` + field + `
}`)
			tAssert.NoError(err)
		},
		Entry("single-quoted string in data output", "data", `'Public output'`),
		Entry("double-quoted string in data output", "data", `"Public output"`),
		Entry("block string in data output", "data", `"""
# Public output
"""`),
		Entry("single-quoted string in schema output", "schema", `'Public output'`),
		Entry("double-quoted string in schema output", "schema", `"Public output"`),
		Entry("block string in schema output", "schema", `"""
# Public output
"""`),
	)

	It("accepts data output doc directives that reference variables", func() {
		_, err := New().Process(`|===|
string documentation = "Public output";
|===|
[output = 'data', doc = documentation]
{
  name: "Ada",
}`)
		tAssert.NoError(err)
	})

	DescribeTable("rejects invalid output doc directives",
		func(input, message string) {
			_, err := New().Process(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("non-string doc value", `[output = 'data', doc = 1]
{
}`, "doc directive must evaluate to a string"),
	)

	It("loads doc fixtures", func() {
		processor := New()
		_, err := processor.ProcessFile("../../fixtures/processor/doc_fixtures/public_contract.mace")
		tAssert.NoError(err)
	})
})
