package processor

import (
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Script mechanics", func() {
	DescribeTable("processes valid script blocks",
		func(input string) {
			processor := New()
			if filepath.Ext(input) == ".mace" && !strings.Contains(input, "\n") {
				_, err := processor.ProcessFile(filepath.Clean(input))
				tAssert.NoError(err)
				return
			}
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)
		},
		Entry("type and schema declarations", wrapScriptWithOutput(`|===|
type Name: string;
schema User: { name: string; };
|===|`)),
		Entry("variables with literals", wrapScriptWithOutput(`|===|
string name = "Ada";
int age = 30;
float rate = 1.25;
hex_int mask = 0xFF;
hex_float ratio = 0x2.8;
boolean active = true;
|===|`)),
		Entry("string interpolation expressions", wrapScriptWithOutput(`|===|
int price = 3;
int quantity = 4;
schema User: { name: string; };
User user = { name: "Ada"; };
string total = "Total $(price * quantity) for $(user.name)";
|===|`)),
		Entry("single quoted and block strings", wrapScriptWithOutput(`|===|
string first = 'Ada';
string second = """Hello
World""";
|===|`)),
		Entry("nullable variable with null initializer", wrapScriptWithOutput(`|===|
nullable string env = null;
|===|`)),
		Entry("imports and script block", `|===|
from "fixtures/processor/imports/base.mace" import Name;
Name user = "Ada";
|===|
[output = data]
{ user: user; }`),
		Entry("unicode web server fixture", "../../fixtures/unicode/web_server.mace"),
		Entry("unicode database fixture", "../../fixtures/unicode/database.mace"),
		Entry("unicode docker services fixture", "../../fixtures/unicode/docker_services.mace"),
		Entry("unicode ci pipeline fixture", "../../fixtures/unicode/ci_pipeline.mace"),
		Entry("unicode theme fixture", "../../fixtures/unicode/theme.mace"),
		Entry("unicode kubernetes deployment fixture", "../../fixtures/unicode/kubernetes_deployment.mace"),
		Entry("unicode ai agent fixture", "../../fixtures/unicode/ai_agent.mace"),
		Entry("variant declarations and assignments", wrapScriptWithOutput(`|===|
type Scalar: variant[string, int];
Scalar value = "Ada";
|===|`)),
		Entry("documentation declarations", wrapScriptWithOutput(`|===|
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
  props: {
    name: "Profile name.",
  };
};

gen_doc Name {
  summary: "Represents a name.",
};

gen_doc greeting {
  summary: "Rendered greeting.",
};
|===|`)),
		Entry("line and block comments are ignored", `|===|
from "fixtures/processor/imports/base.mace" import Name; // trailing import comment
// line comment before declaration
schema Profile: {
  // line comment before field
  name: string; // trailing field comment
  /* block comment before optional field */
  age?: int; // trailing field comment
};

Profile user = {
  name: "Ada"; // trailing field comment
  /* block comment in record */
  age?: 30; // trailing field comment
};
|===|
[output = data]
{
  result: user.name; // trailing output comment
}`),
		Entry("inline descriptions before and after separators", `|===|
schema User: {
  name: string /# Name before separator,
  age?: int, /# Age after separator
};
User user = {
  name: "Ada" /# Record name before separator,
  age?: 27, /# Record age after separator
};
|===|
[output = data]
{
  user_name: user.name, /# Output value after separator
  user_age?: user.age /# Output value before separator
}`),
		Entry("schema output fields with inline descriptions before and after separators", `[output = schema]
{
  name: string /# Name before separator,
  age?: int, /# Age after separator
}`),
		Entry("doc fixtures", "../../fixtures/processor/doc_fixtures/public_contract.mace"),
	)

	DescribeTable("rejects invalid script blocks",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown type reference", wrapScriptWithOutput(`|===|
Unknown value = 1;
|===|`), "unknown type"),
		Entry("int type mismatch", wrapScriptWithOutput(`|===|
int total = 1.5;
|===|`), "type mismatch"),
		Entry("duplicate declaration name", wrapScriptWithOutput(`|===|
type User: string;
schema User: { name: string; };
|===|`), "duplicate declaration"),
		Entry("duplicate imports", `|===|
from "fixtures/processor/imports/base.mace" import User, User;
|===|
[output = data] {}`, "duplicate import"),
		Entry("interpolation rejects type references", wrapScriptWithOutput(`|===|
type UserName: string;
string value = "$(UserName)";
|===|`), "type reference"),
		Entry("schema_doc rejects duplicate keys", wrapScriptWithOutput(`|===|
schema User: { name: string; };

schema_doc User {
  summary: "One";
  summary: "Two";
};
|===|`), "duplicate schema_doc entry"),
		Entry("schema_doc rejects type targets", wrapScriptWithOutput(`|===|
type Status: string;

schema_doc Status {
  summary: "Invalid target.";
};
|===|`), "schema_doc target"),
		Entry("schema_doc rejects scalar variables", wrapScriptWithOutput(`|===|
string greeting = "Hello";

schema_doc greeting {
  summary: "Invalid target.";
};
|===|`), "schema_doc target \"greeting\" must reference a schema or object-valued variable"),
		Entry("gen_doc rejects object variables", wrapScriptWithOutput(`|===|
schema User: {
  name: string;
};

User profile = {
  name: "Ada";
};

gen_doc profile {
  summary: "Invalid target.";
};
|===|`), "gen_doc target \"profile\" must reference a type or non-object variable"),
		Entry("output inline doc requires a directive list", `"""
Invalid: no directive list
"""
{
}
`, "expected output directive"),
		Entry("output inline doc rejects interpolation", `[output = schema]
"""$(name)"""
{
  name: string;
}
`, "interpolation is not allowed"),
		Entry("type inline description conflicts with gen_doc", wrapScriptWithOutput(`|===|
type Name: string /# Duplicate inline docs;

gen_doc Name {
  summary: "Public name type";
};
|===|`), "already documented"),
		Entry("schema field inline description conflicts with schema_doc props", wrapScriptWithOutput(`|===|
schema User: {
  name: string /# Duplicate inline docs;
};

schema_doc User {
  props: {
    name: "The user's display name";
  };
};
|===|`), "already documented"),
		Entry("schema_doc props reject unknown schema fields", wrapScriptWithOutput(`|===|
schema User: {
  name: string;
};

schema_doc User {
  props: {
    age: "Unknown field";
  };
};
|===|`), "does not exist"),
		Entry("gen_doc props reject type targets", wrapScriptWithOutput(`|===|
type Name: string;

gen_doc Name {
  props: {
    value: "Nope";
  };
};
|===|`), "props entry is only allowed in schema_doc"),
		Entry("schema_doc must appear after its schema declaration", wrapScriptWithOutput(`|===|
schema_doc User {
  summary: "Late-bound docs";
};

schema User: {
  name: string;
};
|===|`), "must appear after its schema or object-valued variable declaration"),
		Entry("gen_doc must appear after its type declaration", wrapScriptWithOutput(`|===|
gen_doc Name {
  summary: "Late-bound docs";
};

type Name: string;
|===|`), "must appear after its type or non-object variable declaration"),
		Entry("gen_doc must appear after its variable declaration", wrapScriptWithOutput(`|===|
gen_doc name {
  summary: "Late-bound docs";
};

string name = "Ada";
|===|`), "must appear after its type or non-object variable declaration"),
	)
})
