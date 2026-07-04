package processor

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Variables", func() {
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

var _ = Describe("Processor entrypoints", func() {
	It("covers processor entrypoint helpers", func() {
		processor := New()
		workspace, err := os.MkdirTemp("", "processor-entrypoints-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		file := writeFixtureFile(workspace, "input.mace", `|===|
int value = 1;
|===|
[output = data]
{ result: value; }`)

		_, err = processor.Process(`{ result: 1; }`)
		tAssert.NoError(err)
		_, err = processor.ProcessInDir(`{ result: 1; }`, "")
		tAssert.NoError(err)
		_, err = processor.ProcessInScope(`{ result: 1; }`, "", "")
		tAssert.NoError(err)

		scriptResult, err := processor.ProcessScriptBlock(`|===|
int value = 1;
|===|`)
		tAssert.NoError(err)
		_, err = processor.ProcessVariablesInDir(wrapScriptWithOutput(`|===|
int value = 1;
|===|`), "")
		tAssert.NoError(err)
		_, err = processor.ProcessVariablesInScope(wrapScriptWithOutput(`|===|
int value = 1;
|===|`), "", "")
		tAssert.NoError(err)
		_, err = processor.ProcessOutputBlock(`[output = data] { result: 1; }`, ScriptResult{})
		tAssert.NoError(err)
		_, err = processor.ProcessOutputBlock(`[output = data] { result: 1; }`, ScriptResult{context: newProcessContext("", "")})
		tAssert.NoError(err)
		_, err = processor.ProcessFile(filepath.Join(".", "does-not-exist.mace"))
		tAssert.Error(err)
		_, err = processor.ProcessFileInDir(filepath.Join(".", "does-not-exist.mace"), "")
		tAssert.Error(err)
		_, err = processor.ProcessFileInDir(file, workspace)
		tAssert.NoError(err)
		_, err = processor.processInput(`{ result: 1; }`, ".", ".", false)
		tAssert.NoError(err)
		_, err = processor.processScriptInput(`|===|
int value = 1;
|===|`, ".")
		tAssert.NoError(err)
		_, err = processor.processOutputInput(`[output = data] { result: 1; }`, scriptResult, ".")
		tAssert.NoError(err)
		_, err = processor.processInput(`{ result: 1; } garbage`, ".", ".", false)
		tAssert.Error(err)
		_, err = processor.processScriptInput(`|===|
int value = 1;
|===| garbage`, ".")
		tAssert.Error(err)
		_, err = processor.processOutputInput(`[output = data] { result: 1; } garbage`, scriptResult, ".")
		tAssert.Error(err)
		_, err = processor.processParsedOutput(ast.OutputBlock{}, ast.File{}, newProcessContext(".", "."))
		tAssert.NoError(err)
		_, err = processor.processParsedOutput(ast.OutputBlock{Mode: ast.OutputModeSchema}, ast.File{Output: ast.OutputBlock{Mode: ast.OutputModeSchema}}, newProcessContext(".", "."))
		tAssert.NoError(err)

		_, err = processor.ProcessOutputBlock(`[parse = schema] { result: 1; }`, ScriptResult{context: newProcessContext(".", ".")})
		tAssert.Error(err)
		_, err = processor.ProcessOutputBlock(`[parse_file = schema.mace] { result: 1; }`, ScriptResult{context: newProcessContext(".", ".")})
		tAssert.Error(err)

		ctx := newProcessContext(".", ".")
		cloned := ctx.clone()
		tAssert.NotNil(cloned.symbols)
	})

	It("falls back when the current working directory cannot be read", func() {
		workspace, err := os.MkdirTemp("", "processor-getwd-*")
		tAssert.NoError(err)
		cwd, err := os.Getwd()
		tAssert.NoError(err)
		tAssert.NoError(os.Chdir(workspace))
		defer func() {
			_ = os.Chdir(cwd)
			_ = os.RemoveAll(workspace)
		}()

		processor := New()
		_, err = processor.Process(`{ result: 1; }`)
		tAssert.NoError(err)
		_, err = processor.ProcessScriptBlock(`|===|
int value = 1;
|===|`)
		tAssert.NoError(err)
	})
})

var _ = Describe("Block processing", func() {
	It("processes variables in explicit directories", func() {
		processor := NewWithInjections(map[string]Value{
			"unused": {Kind: ValueInt, Int: 4},
		})
		variables, err := processor.ProcessVariablesInDir(`|===|
int base = 4;
int doubled = base * 2;
|===|
[output = data]
{ result: doubled; }`, "../..")
		tAssert.NoError(err)
		assertExpectedValue(variables["doubled"], expectedValue{kind: ValueInt, int64: 8})

		variables, err = processor.ProcessVariablesInScope(`|===|
int base = 4;
int tripled = base * 3;
|===|
[output = data]
{ result: tripled; }`, "../..", "../..")
		tAssert.NoError(err)
		assertExpectedValue(variables["tripled"], expectedValue{kind: ValueInt, int64: 12})
	})

	It("processes script blocks independently", func() {
		processor := New()
		result, err := processor.ProcessScriptBlock(`|===|
int base = 2 + 2;
string name = "Ada";
|===|`)
		tAssert.NoError(err)

		base := requireScriptVariable(result, "base")
		tAssert.Equal(ValueInt, base.Kind)
		tAssert.Equal(int64(4), base.Int)

		name := requireScriptVariable(result, "name")
		tAssert.Equal(ValueString, name.Kind)
		tAssert.Equal("Ada", name.String)
	})

	It("decodes unicode string escapes", func() {
		processor := New()
		result, err := processor.ProcessOutputBlock(`[output = data]
{
  accent: "\u00E9";
  rocket: "\U0001F680";
}`, ScriptResult{})
		tAssert.NoError(err)

		assertExpectedValue(requireOutputValue(result, "accent"), expectedValue{kind: ValueString, string: "é"})
		assertExpectedValue(requireOutputValue(result, "rocket"), expectedValue{kind: ValueString, string: "🚀"})
	})

	It("rejects invalid unicode string escapes", func() {
		processor := New()
		_, err := processor.ProcessOutputBlock(`[output = data]
{
  invalid: "\U00110000";
}`, ScriptResult{})
		tAssert.ErrorContains(err, "invalid unicode")
	})

	It("processes output blocks independently", func() {
		processor := New()
		result, err := processor.ProcessOutputBlock(`[output = schema]
"""
# Output Schema
"""
{
  name: string;
  age?: int;
}`, ScriptResult{})
		tAssert.NoError(err)

		assertExpectedSchema(result, map[expectedSchemaField]SchemaType{
			{name: "name"}:                schemaPrimitive("string"),
			{name: "age", optional: true}: schemaPrimitive("int"),
		})
	})

	It("processes output blocks with script context", func() {
		processor := New()
		scriptResult, err := processor.ProcessScriptBlock(`|===|
int base = 2 + 2;
|===|`)
		tAssert.NoError(err)

		result, err := processor.ProcessOutputBlock(`[output = data]
{
  result: base * 3;
}`, scriptResult)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "result")
		assertExpectedValue(actual, expectedValue{kind: ValueInt, int64: 12})
	})
})

var _ = Describe("Processor entrypoint coverage", func() {
	It("covers processor entrypoints and path helpers", func() {
		workspace, err := os.MkdirTemp("", "processor-entrypoints-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/remote.mace":
				_, _ = io.WriteString(writer, `[output = schema]
{ remote: string; }`)
			case "/broken.mace":
				writer.WriteHeader(http.StatusInternalServerError)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		proc := NewWithInput(map[string]Value{"seed": {Kind: ValueInt, Int: 1}})
		inputPath := writeFixtureFile(workspace, "input.mace", `[output = data]
{ result: seed; }`)
		_, err = proc.Process(`{ result: 1; }`)
		tAssert.NoError(err)
		_, err = proc.ProcessScriptBlock(`|===|
int value = 1;
|===|`)
		tAssert.NoError(err)
		_, err = proc.ProcessVariablesInScope(wrapScriptWithOutput(`|===|
int value = 1;
|===|`), workspace, workspace)
		tAssert.NoError(err)
		_, err = proc.ProcessFileInDir(inputPath, workspace)
		tAssert.Error(err)
		_, err = proc.ProcessFileInDir(filepath.Join(workspace, "missing.mace"), workspace)
		tAssert.Error(err)
		_, err = proc.ProcessFileInDir(inputPath, "")
		tAssert.Error(err)

		scriptResult, err := proc.ProcessScriptBlock(`|===|
int value = 1;
|===|`)
		tAssert.NoError(err)
		_, err = proc.ProcessOutputBlock(`[output = data] { result: 1; }`, scriptResult)
		tAssert.NoError(err)
		_, err = proc.ProcessOutputBlock(`[output = data] { result: 1; }`, ScriptResult{})
		tAssert.NoError(err)
		_, err = proc.processOutputInput(`[output = data] { result: 1; }`, ScriptResult{}, workspace)
		tAssert.NoError(err)

		_, err = ParseInputRecord(`{ name: "Ada"; }`)
		tAssert.NoError(err)
		_, err = ParseInputRecord(`1`)
		tAssert.Error(err)

		_, err = parseImportPath(ast.StringLiteral{Lexeme: `"` + server.URL + `/remote.mace"`})
		tAssert.NoError(err)
		resolved, err := resolveImportPath(server.URL, "remote.mace")
		tAssert.NoError(err)
		tAssert.Equal(server.URL+"/remote.mace", resolved)
		_, err = resolveBoundedPath(workspace, workspace, "../escape.mace")
		tAssert.Error(err)
		_, err = resolveBoundedRemotePath(workspace, server.URL, "remote.mace", server.URL+"/remote.mace")
		tAssert.NoError(err)
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal(server.URL, formatImportRoot(server.URL))
		parsed, ok := parseRemoteURL(server.URL)
		tAssert.True(ok)
		tAssert.NotNil(parsed)
		_, ok = parseRemoteURL("ftp://example.com")
		tAssert.False(ok)
		_, err = readMaceSource(server.URL + "/remote.mace")
		tAssert.NoError(err)
		_, err = readMaceSource(server.URL + "/broken.mace")
		tAssert.Error(err)

		cache := map[string]map[string]importedDeclaration{}
		stack := map[string]struct{}{}
		_, err = loadImportExports(server.URL+"/remote.mace", server.URL, false, cache, stack)
		tAssert.NoError(err)
	})

	It("covers processor wrapper fallback branches", func() {
		previousGetwd := getwd
		getwd = func() (string, error) {
			return "", errors.New("cwd unavailable")
		}
		defer func() {
			getwd = previousGetwd
		}()

		processor := New()
		_, err := processor.Process(`{ result: 1; }`)
		tAssert.NoError(err)
		_, err = processor.ProcessScriptBlock(`|===|
int value = 1;
|===|`)
		tAssert.NoError(err)
		_, err = processor.ProcessOutputBlock(`[output = data] { result: 1; }`, ScriptResult{})
		tAssert.NoError(err)
		_, err = processor.ProcessOutputBlock(`[output = data] { result: 1; }`, ScriptResult{context: newProcessContext("", "")})
		tAssert.NoError(err)
	})

	It("covers process pipeline error branches", func() {
		processor := New()
		_, err := processor.processInput("`", ".", ".", true)
		tAssert.Error(err)
		_, err = processor.processScriptInput("`", ".")
		tAssert.Error(err)
		_, err = processor.processOutputInput("`", ScriptResult{}, ".")
		tAssert.Error(err)
		script := &ast.ScriptBlock{Items: []ast.Declaration{
			ast.VariableDeclaration{Name: "name", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}},
			ast.VariableDeclaration{Name: "name", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Bob"`}},
		}}
		_, err = buildProcessContextWithState(nil, script, ".", ".", true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = prepareOutputContext(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}}, newProcessContext(".", "."))
		tAssert.NoError(err)
		_, err = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}}, newProcessContext(".", "."))
		tAssert.Error(err)
	})

	It("covers processor entrypoint and parser failure branches", func() {
		workspace, err := os.MkdirTemp("", "processor-entrypoint-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		writeFixtureFile(workspace, "base.mace", `[output = data]
{ User: "Ada"; }`)
		writeFixtureFile(workspace, "duplicate-import.mace", `from "./base.mace" import User, User; [output = data] {}`)
		writeFixtureFile(workspace, "script-dupe.mace", `|===|
int value = 1;
int value = 2;
|===|`)
		writeFixtureFile(workspace, "schema-variable.mace", `|===|
int value = 1;
|===|
[output = schema]
{ User: User; }`)

		processor := New()
		_, err = processor.ProcessVariablesInScope("`", workspace, workspace)
		tAssert.Error(err)
		_, err = processor.ProcessVariablesInScope(`from "./base.mace" import User, User;`, workspace, workspace)
		tAssert.Error(err)
		_, err = ParseInputRecord("`")
		tAssert.Error(err)
		_, err = ParseInputRecord(`1 + true`)
		tAssert.Error(err)
		_, err = ParseInputRecord(`1`)
		tAssert.Error(err)
		_, err = processor.processScriptInput("`", workspace)
		tAssert.Error(err)
		_, err = processor.processScriptInput(`|===|
int value = 1;
int value = 2;
|===|`, workspace)
		tAssert.Error(err)
		_, err = processor.processOutputInput("`", ScriptResult{}, workspace)
		tAssert.Error(err)
		_, err = processor.ProcessFileInDir(filepath.Join(workspace, "missing.mace"), workspace)
		tAssert.Error(err)
		_, _ = processor.processParsedOutput(ast.OutputBlock{Mode: ast.OutputModeSchema}, ast.File{Script: &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "int"}, HasValue: true, Value: ast.IntLiteral{Lexeme: "1"}}}}}, newProcessContext(workspace, workspace))
		_, _ = processor.processParsedOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}}, ast.File{}, newProcessContext(workspace, workspace))
		_, _ = importFileAsDeclaration("bad", map[string]importedDeclaration{"x": {name: "x", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}})
	})

	It("covers ProcessVariablesInScope build context errors", func() {
		processor := New()
		_, err := processor.ProcessVariablesInScope(`|===|
from "./schema.mace" import User;
from "./schema.mace" import User;
|===|`, "", "")
		tAssert.Error(err)
		_, err = buildProcessContext([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, nil, ".", ".", true, map[string]Value{})
		tAssert.Error(err)
		_, err = buildProcessContext([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.txt"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, nil, ".", ".", true, map[string]Value{})
		tAssert.Error(err)
		_, err = buildProcessContext(nil, &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: false}}}, ".", ".", true, map[string]Value{})
		tAssert.Error(err)
		_, err = buildProcessContext(nil, &ast.ScriptBlock{Items: []ast.Declaration{ast.DocDeclaration{Target: "missing"}}}, ".", ".", true, map[string]Value{})
		tAssert.Error(err)
		_, err = buildProcessContext([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `""`}}}, nil, ".", ".", true, map[string]Value{})
		tAssert.Error(err)
		_, err = processor.ProcessVariablesInScope(`|===============================|
from "./base.mace" import User;
from "./base.mace" import User;

string name = "Ada";

User result = {
  name: name,
  age: 27,
};
|===============================|

[output = data]
{ result: result, }`, "../../fixtures/processor/imports", "")
		tAssert.Error(err)
		consumer, err := os.ReadFile("../../fixtures/processor/imports/consumer.mace")
		tAssert.NoError(err)
		_, err = processor.ProcessVariablesInScope(string(consumer), "../../fixtures/processor/imports", "")
		tAssert.NoError(err)
	})
})
