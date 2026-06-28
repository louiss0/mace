package processor

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestProcessor(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Processor Suite")
}

func wrapScriptWithOutput(script string) string {
	return script + "\n[output = data] {}"
}

func wrapScriptWithOutputFields(script string, fields string) string {
	return script + "\n[output = data]\n{\n" + fields + "\n}"
}

type expectedValue struct {
	kind   ValueKind
	int64  int64
	float  float64
	bool   bool
	string string
	array  []expectedValue
	record map[string]expectedValue
}

type expectedSchemaField struct {
	name     string
	optional bool
}

func schemaPrimitive(name string) SchemaType {
	return SchemaType{Kind: SchemaTypePrimitive, Name: name}
}

func schemaNamed(name string) SchemaType {
	return SchemaType{Kind: SchemaTypeNamed, Name: name}
}

func schemaArray(element SchemaType) SchemaType {
	return SchemaType{Kind: SchemaTypeArray, Element: &element}
}

func schemaRecord(fields map[expectedSchemaField]SchemaType) SchemaType {
	recordFields := make(map[SchemaField]SchemaType, len(fields))
	for field, fieldType := range fields {
		recordFields[SchemaField{Name: field.name, Optional: field.optional}] = fieldType
	}

	return SchemaType{Kind: SchemaTypeRecord, Fields: recordFields}
}

func requireOutputValue(result Result, name string) Value {
	value, ok := result.Output[name]
	tAssert.True(ok)
	if !ok {
		return Value{}
	}
	return value
}

func assertExpectedValue(actual Value, expected expectedValue) {
	tAssert.Equal(expected.kind, actual.Kind)
	switch expected.kind {
	case ValueInt:
		tAssert.Equal(expected.int64, actual.Int)
	case ValueFloat:
		tAssert.InDelta(expected.float, actual.Float, 0.000001)
	case ValueHexInt, ValueHexFloat:
		formatted, err := FormatScalarValue(actual)
		tAssert.NoError(err)
		tAssert.Equal(expected.string, formatted)
	case ValueBoolean:
		tAssert.Equal(expected.bool, actual.Boolean)
	case ValueString:
		tAssert.Equal(expected.string, actual.String)
	case ValueArray:
		tAssert.Equal(len(expected.array), len(actual.Array))
		for index, value := range expected.array {
			if index >= len(actual.Array) {
				return
			}
			assertExpectedValue(actual.Array[index], value)
		}
	case ValueRecord:
		tAssert.Equal(len(expected.record), len(actual.Record))
		for name, value := range expected.record {
			actualValue, ok := actual.Record[name]
			tAssert.True(ok)
			if !ok {
				continue
			}
			assertExpectedValue(actualValue, value)
		}
	}
}

func assertExpectedOutput(result Result, expected map[string]expectedValue) {
	for name, value := range expected {
		actual := requireOutputValue(result, name)
		assertExpectedValue(actual, value)
	}
}

func assertExpectedSchema(result Result, expected map[expectedSchemaField]SchemaType) {
	tAssert.Len(result.Output, 0)
	tAssert.Len(result.Schema, len(expected))

	for field, expectedType := range expected {
		actualType, ok := result.Schema[SchemaField{Name: field.name, Optional: field.optional}]
		tAssert.True(ok)
		if !ok {
			continue
		}

		tAssert.Equal(expectedType, actualType)
	}
}

func assertProcessedResult(input string, expected expectedValue) {
	processor := New()
	result, err := processor.ProcessInDir(input, "../..")
	tAssert.NoError(err)

	actual := requireOutputValue(result, "result")
	assertExpectedValue(actual, expected)
}

func requireScriptVariable(result ScriptResult, name string) Value {
	value, ok := result.Variables[name]
	tAssert.True(ok)
	if !ok {
		return Value{}
	}

	return value
}

func writeFixtureFile(root string, relativePath string, contents string) string {
	path := filepath.Join(root, relativePath)
	tAssert.NoError(os.MkdirAll(filepath.Dir(path), 0o755))
	tAssert.NoError(os.WriteFile(path, []byte(contents), 0o644))
	return path
}

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

var _ = Describe("Path helpers", func() {
	It("clones and preserves nested contexts", func() {
		original := newProcessContext("/base", "/root")
		original.optionalParseVars["x"] = struct{}{}
		cloned := original.clone()
		tAssert.Equal(original.importBaseDir, cloned.importBaseDir)
		tAssert.Equal(original.importRootDir, cloned.importRootDir)
		tAssert.NotNil(cloned.symbols)
		tAssert.NotNil(cloned.types)
		tAssert.NotNil(cloned.schemas)
		tAssert.NotNil(cloned.variables)
		tAssert.NotNil(cloned.environment)
		tAssert.Contains(cloned.optionalParseVars, "x")
	})

	It("formats local and remote import roots", func() {
		tAssert.Equal("./", formatImportRoot(""))
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("workspace/", formatImportRoot(filepath.Join("/tmp", "workspace")))
		tAssert.Equal("https://example.com/root/", formatImportRoot("https://example.com/root/"))
	})

	It("clones empty process contexts safely", func() {
		var empty processContext
		cloned := empty.clone()
		tAssert.Equal(processContext{}, cloned)
	})

	It("parses remote URLs and derives base directories", func() {
		remote, ok := parseRemoteURL("https://example.com/root/file.mace")
		tAssert.True(ok)
		tAssert.Equal("https", remote.Scheme)
		tAssert.Equal("example.com", remote.Host)

		_, ok = parseRemoteURL("file:///tmp/file.mace")
		tAssert.False(ok)
		tAssert.Equal("https://example.com/root/", basePathDir("https://example.com/root/file.mace"))
		tAssert.Equal(filepath.Dir("/tmp/file.mace"), basePathDir("/tmp/file.mace"))
	})

	It("resolves import paths within and outside bounded scopes", func() {
		resolved, err := resolveImportPath("/workspace", "nested/file.mace")
		tAssert.NoError(err)
		tAssert.Contains(resolved, "nested")

		resolved, err = resolveImportPath("https://example.com/root/", "child/file.mace")
		tAssert.NoError(err)
		tAssert.Equal("https://example.com/root/child/file.mace", resolved)

		_, err = resolveImportPath("/workspace", "/absolute/file.mace")
		tAssert.ErrorContains(err, "must be relative")

		bounded, err := resolveImportPathInScope("/workspace", "/workspace", "nested/file.mace", true)
		tAssert.NoError(err)
		tAssert.Contains(bounded, "nested")

		_, err = resolveBoundedPath("/workspace", "/workspace", "../escape.mace")
		tAssert.ErrorContains(err, "escapes root")

		boundedRemote, err := resolveBoundedRemotePath("https://example.com/root/", "https://example.com/root/", "child/file.mace", "https://example.com/root/child/file.mace")
		tAssert.NoError(err)
		tAssert.Equal("https://example.com/root/child/file.mace", boundedRemote)
		_, err = resolveBoundedRemotePath("https://example.com/root/", "https://example.com/root/", "child/file.mace", "https://evil.example.com/root/child/file.mace")
		tAssert.ErrorContains(err, "escapes root")
	})

	It("validates mace source paths", func() {
		tAssert.NoError(validateMaceSourcePath("config.mace"))
		tAssert.ErrorContains(validateMaceSourcePath("config.txt"), "must end in .mace")
	})

	It("reads local and remote mace sources", func() {
		localDir, err := os.MkdirTemp("", "mace-local-*")
		tAssert.NoError(err)
		localPath := filepath.Join(localDir, "config.mace")
		tAssert.NoError(os.WriteFile(localPath, []byte("local"), 0o600))

		contents, err := readMaceSource(localPath)
		tAssert.NoError(err)
		tAssert.Equal("local", contents)

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, _ = writer.Write([]byte("remote"))
		}))
		defer server.Close()

		contents, err = readMaceSource(server.URL + "/config.mace")
		tAssert.NoError(err)
		tAssert.Equal("remote", contents)
	})
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

var _ = Describe("Validation helpers", func() {
	It("extracts guarded names and validates guarded output expressions", func() {
		guarded := extractGuardedNames(ast.InfixExpression{
			Left:     ast.StringLiteral{Lexeme: `"profile"`},
			Operator: lexer.TokenIn,
			Right:    ast.Identifier{Name: "record"},
		}, map[string]struct{}{})
		tAssert.Contains(guarded, "profile")

		guarded = extractGuardedNames(ast.InfixExpression{
			Left: ast.InfixExpression{
				Left:     ast.StringLiteral{Lexeme: `"profile"`},
				Operator: lexer.TokenIn,
				Right:    ast.Identifier{Name: "record"},
			},
			Operator: lexer.TokenAndAnd,
			Right: ast.InfixExpression{
				Left:     ast.StringLiteral{Lexeme: `"age"`},
				Operator: lexer.TokenIn,
				Right:    ast.Identifier{Name: "record"},
			},
		}, map[string]struct{}{})
		tAssert.Contains(guarded, "profile")
		tAssert.Contains(guarded, "age")

		symbols := newSymbolTable()
		symbols.Add("TypeName", symbolKindType)
		optional := map[string]struct{}{"record": {}}
		err := validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "value"}, symbols, optional, map[string]struct{}{})
		tAssert.ErrorContains(err, "requires a presence check")

		err = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "value"}, symbols, optional, map[string]struct{}{"record": {}})
		tAssert.NoError(err)

		err = validateDataOutputExpression(ast.Identifier{Name: "TypeName"}, symbols, optional, map[string]struct{}{})
		tAssert.ErrorContains(err, "cannot reference type or schema declaration")
	})

	It("resolves parse-file schema names from imported files", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-parse-file-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		path := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{
  Profile: Profile;
  Alias: Alias;
  ignore: string;
}`)
		_ = path

		directives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}
		names, err := resolveParseFileExportedSchemaNames(directives, workspace, workspace)
		tAssert.NoError(err)
		tAssert.Equal([]string{"Alias", "Profile"}, names)

		directives = []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./missing.txt"`}}
		_, err = resolveParseFileExportedSchemaNames(directives, workspace, workspace)
		tAssert.Error(err)
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

var _ = Describe("Script block", func() {
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

	DescribeTable("accepts primitive variant alternatives",
		func(typeReference string, firstValue string, secondValue string) {
			processor := New()
			_, err := processor.Process(wrapScriptWithOutput(fmt.Sprintf(`|===|
type Value: %s;
Value first = %s;
Value second = %s;
|===|`, typeReference, firstValue, secondValue)))
			tAssert.NoError(err)
		},
		Entry("string-int", "variant[string, int]", `"Ada"`, `42`),
		Entry("string-float", "variant[string, float]", `"Ada"`, `1.5`),
		Entry("string-boolean", "variant[string, boolean]", `"Ada"`, `true`),
		Entry("int-float", "variant[int, float]", `42`, `1.5`),
		Entry("int-boolean", "variant[int, boolean]", `42`, `true`),
		Entry("float-boolean", "variant[float, boolean]", `1.5`, `true`),
	)

	It("accepts schema and primitive variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema User: { name: string; };
type Value: variant[User, string];
Value first = { name: "Ada"; };
Value second = "fallback";
|===|`))
		tAssert.NoError(err)
	})

	It("accepts array variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Value: variant[array<string>, array<int>];
Value names = ["Ada", "Lin"];
Value counts = [1, 2];
|===|`))
		tAssert.NoError(err)
	})

	It("accepts nested array variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Value: variant[array<array<string>>, array<array<array<int>>>];
Value tags = [["api"]];
Value matrix = [[[1]]];
|===|`))
		tAssert.NoError(err)
	})

	It("accepts nested variant aliases", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Scalar: variant[string, int];
type Value: variant[Scalar, boolean];
Value first = "Ada";
Value second = 42;
Value third = true;
|===|`))
		tAssert.NoError(err)
	})

	DescribeTable("accepts choice variants with primitive literal fallbacks",
		func(choiceType string, primitiveType string, presetValue string, fallbackValue string) {
			processor := New()
			_, err := processor.Process(wrapScriptWithOutput(fmt.Sprintf(`|===|
type Preset: %s;
type Value: variant[Preset, %s];
Value preset = %s;
Value fallback = %s;
|===|`, choiceType, primitiveType, presetValue, fallbackValue)))
			tAssert.NoError(err)
		},
		Entry("string preset with string fallback", `choice["approved"]`, "string", `"approved"`, `"custom"`),
		Entry("int preset with int fallback", `choice[1]`, "int", `1`, `2`),
		Entry("float preset with float fallback", `choice[1.5]`, "float", `1.5`, `2.5`),
		Entry("hex int preset with hex int fallback", `choice[0x1]`, "hex_int", `0x1`, `0x2`),
		Entry("hex float preset with hex float fallback", `choice[0x1.8]`, "hex_float", `0x1.8`, `0x2.8`),
		Entry("boolean preset with boolean fallback", `choice[true]`, "boolean", `true`, `false`),
	)

	It("accepts union schema composition aliases", func() {
		processor := New()
		result, err := processor.Process(`|===|
schema Profile: { name: string; };
schema Audit: { created_at: string; };
type User: union[Profile, Audit];
User value = {
  name: "Ada";
  created_at: "2026-04-08";
};
|===|
[output = data]
{
  result: value;
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "result")
		assertExpectedValue(actual, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name":       {kind: ValueString, string: "Ada"},
			"created_at": {kind: ValueString, string: "2026-04-08"},
		}})
	})

	It("rejects union schema composition with non-schema members", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Broken: union[string, int];
|===|`))
		tAssert.ErrorContains(err, "union members must be schemas")
	})

	It("rejects union schema composition with conflicting fields", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema Profile: { id: string; };
schema Audit: { id: int; };
type Broken: union[Profile, Audit];
|===|`))
		tAssert.ErrorContains(err, "conflicting field")
	})

	It("rejects variant variables with non-matching values", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Scalar: variant[string, int];
Scalar value = true;
|===|`))
		tAssert.ErrorContains(err, "type mismatch")
	})

	It("rejects record literals that mix fields across variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema EmailLogin: { email: string; password: string; };
schema ApiKeyLogin: { api_key: string; };
type Login: variant[EmailLogin, ApiKeyLogin];
Login value = {
  email: "ada@example.com";
  password: "secret";
  api_key: "token";
};
|===|`))
		tAssert.ErrorContains(err, "type mismatch")
	})

	It("rejects record literals that match multiple variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema Named: { id: string; };
schema OptionallyNamed: { id: string; nickname?: string; };
type Identity: variant[Named, OptionallyNamed];
Identity value = { id: "u1"; };
|===|`))
		tAssert.ErrorContains(err, "exactly one variant member")
	})

	DescribeTable("accepts schema record literals",
		func(input string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)
		},
		Entry("optional fields omitted", wrapScriptWithOutput(`|===|
schema User: { name: string; age?: int; };
User user = { name: "Ada"; };
|===|`)),
		Entry("array of schema records", wrapScriptWithOutput(`|===|
schema Point: { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; y: 4; }
];
|===|`)),
		Entry("nullable string initializer", wrapScriptWithOutput(`|===|
nullable string env = "dev";
|===|`)),
	)

	DescribeTable("rejects schema record literal mismatches",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", wrapScriptWithOutput(`|===|
schema User: { name: string; age: int; };
User user = { name: "Ada"; };
|===|`), "missing required field"),
		Entry("unknown field", wrapScriptWithOutput(`|===|
schema User: { name: string; };
User user = { name: "Ada"; age: 30; };
|===|`), "unknown field"),
		Entry("optional field mismatch", wrapScriptWithOutput(`|===|
schema User: { name: string; age: int; };
User user = { name: "Ada"; age?: 30; };
|===|`), "not optional"),
		Entry("field type mismatch", wrapScriptWithOutput(`|===|
schema User: { name: string; age: int; };
User user = { name: 5; age: 30; };
|===|`), "type mismatch"),
		Entry("array element schema mismatch", wrapScriptWithOutput(`|===|
schema Point: { x: int; y: int; };
array<Point> points = [
  { x: 1; y: 2; },
  { x: 3; }
];
|===|`), "missing required field"),
	)

	It("accepts schema member access in schema-validated output", func() {
		processor := New()
		_, err := processor.Process(`|===|
schema User: {
  id: string;
  name: string;
};

User user = {
  id: "user_1";
  name: "Ada";
};
|===|
[output = data, schema = User]
{
  id: user.id;
  name: user.name;
}`)
		tAssert.NoError(err)
	})

	It("uses parse input to expose schema fields in the output block", func() {
		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		result, err := processor.Process(`|===|
schema Runtime: { env: string; };
|===|
[output = data, parse = Runtime]
{
  env: env;
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "env")
		assertExpectedValue(actual, expectedValue{kind: ValueString, string: "prod"})
	})

	It("omits output fields that evaluate to null through nullable variables", func() {
		processor := New()

		result, err := processor.Process(`|===|
nullable string env = null;
|===|
[output = data]
{
  env: env;
}`)
		tAssert.NoError(err)
		tAssert.Empty(result.Output)
	})

	It("accepts null for optional schema fields", func() {
		processor := New()

		result, err := processor.Process(`|===|
schema User: { nickname?: string; };
User user = { nickname: null; };
|===|
[output = data]
{
  user: user;
}`)
		tAssert.NoError(err)

		actual := requireOutputValue(result, "user")
		assertExpectedValue(actual, expectedValue{kind: ValueRecord, record: map[string]expectedValue{}})
	})

	It("rejects direct null output fields", func() {
		processor := New()

		_, err := processor.Process(`[output = data]
{
  env: null;
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects parse directives without required input fields", func() {
		processor := New()

		_, err := processor.Process(`|===|
schema Runtime: { env: string; };
|===|
[output = data, parse = Runtime]
{
  env: env;
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "missing required field")
	})

	It("rejects null assignments to non-nullable variables", func() {
		processor := New()

		_, err := processor.Process(wrapScriptWithOutput(`|===|
string env = null;
|===|`))
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects nullable conditional assignments to non-nullable variables", func() {
		processor := New()

		_, err := processor.Process(wrapScriptWithOutput(`|===|
string env = false ? null : "prod";
|===|`))
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects nullable conditional assignments to required schema fields", func() {
		processor := New()

		_, err := processor.Process(`|===|
schema Runtime: { env: string; };
Runtime config = { env: false ? null : "prod"; };
|===|
[output = data]
{
  config: config;
}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "null can only be assigned to nullable variables and optional schema fields")
	})

	It("rejects parse directives with an unknown schema", func() {
		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		_, err := processor.Process(`[output = data, parse = MissingSchema] {}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "unknown schema")
	})

	It("rejects parse_file with a missing schema file", func() {
		processor := New()

		_, err := processor.ProcessInDir(`[output = data, parse_file = "./missing.mace"] {}`, ".")
		tAssert.Error(err)
		tAssert.ErrorContains(err, "unable to read import file")
	})

	It("uses parse_file without a schema directive when one schema is available", func() {
		workspace, err := os.MkdirTemp("", "mace-parse-file-fixture-*")
		tAssert.NoError(err)
		defer func() {
			_ = os.RemoveAll(workspace)
		}()

		writeFixtureFile(workspace, "runtime.mace", `|===|
schema Runtime: { env: string; };
schema Meta: { source: string; };
|===|
[output = schema]
{
  Runtime: Runtime;
}`)

		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		result, err := processor.ProcessInDir(`[output = data, parse_file = "./runtime.mace"]
{
  env: env;
}`, workspace)
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "env"), expectedValue{kind: ValueString, string: "prod"})
	})

	It("rejects parse_file without a schema directive when multiple schemas are available", func() {
		workspace, err := os.MkdirTemp("", "mace-parse-file-ambiguous-*")
		tAssert.NoError(err)
		defer func() {
			_ = os.RemoveAll(workspace)
		}()

		writeFixtureFile(workspace, "runtime.mace", `|===|
schema Runtime: { env: string; };
schema Backup: { env: string; };
|===|
[output = schema]
{
  Runtime: Runtime;
  Backup: Backup;
}`)

		processor := NewWithInput(map[string]Value{
			"env": {Kind: ValueString, String: "prod"},
		})

		_, err = processor.ProcessInDir(`[output = data, parse_file = "./runtime.mace"] {}`, workspace)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "parse_file directive is ambiguous without a schema directive")
	})

	DescribeTable("processes valid choice declarations",
		func(input string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("choice string literal", `|===|
 type Fruit: choice["Apple", "Strawberry"];
 Fruit result = "Apple";
|===|
[output = data]
{
  result: result;
}`, expectedValue{kind: ValueString, string: "Apple"}),
		Entry("choice aliases can be mixed", `|===|
 type Environment: choice["dev", "prod"];
 type Numeric: choice[1, 2];
 type Mode: choice[Environment, Numeric, true];
 Mode result = 2;
|===|
[output = data]
{
  result: result;
}`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("choice float members preserve precision", `|===|
 type Ratio: choice[1.04, 1.0];
 Ratio first = 1.04;
 Ratio second = 1.0;
|===|
[output = data]
{
  result: { first: first; second: second; };
}`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"first":  {kind: ValueFloat, float: 1.04},
			"second": {kind: ValueFloat, float: 1.0},
		}}),
	)

	DescribeTable("rejects invalid choice declarations and assignments",
		func(input string, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown choice alias", wrapScriptWithOutput(`|===|
 type Fruit: choice[MissingChoice];
|===|`), "unknown choice member"),
		Entry("non-choice alias in choice members", wrapScriptWithOutput(`|===|
 type Name: string;
 type Fruit: choice[Name];
|===|`), "must resolve to a choice type"),
		Entry("value outside choice domain", `|===|
 type Fruit: choice["Apple", "Strawberry"];
 Fruit result = "Pear";
|===|
[output = data]
{
  result: result;
}`, "type mismatch: expected choice[\"Apple\", \"Strawberry\"], got \"Pear\""),
		Entry("conditional branch outside choice domain", `|===|
 boolean enabled = true;
 type Fruit: choice["Apple", "Strawberry"];
 Fruit result = (enabled ? "Pear" : "Apple");
|===|
[output = data]
{
  result: result;
}`, "type mismatch: expected choice[\"Apple\", \"Strawberry\"], got \"Pear\""),
	)

})

var _ = Describe("Imports", func() {
	DescribeTable("merges imported declarations",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("imports types and schemas", `|===|
from "fixtures/processor/imports/base.mace" import Name, User;
Name name = "Ada";
User result = { name: name; age: 30; };
|===|
[output = data]
{ result: result; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("imports values surfaced through output", `|===|
from "fixtures/processor/imports/values.mace" import count;
|===|
[output = data]
{ result: count + 2; }`, expectedValue{kind: ValueInt, int64: 5}),
		Entry("imports schemas and aliases from a public contract fixture", `|===|
from "fixtures/processor/imports/contracts.mace" import ID, Team;
ID team_name = "core";
Team result = { name: team_name; members: [{ id: "u1"; role: "owner"; }]; };
|===|
[output = data]
{ result: result; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "core"},
			"members": {kind: ValueArray, array: []expectedValue{
				{kind: ValueRecord, record: map[string]expectedValue{
					"id":   {kind: ValueString, string: "u1"},
					"role": {kind: ValueString, string: "owner"},
				}},
			}},
		}}),
	)

	It("imports variant aliases reused across files", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-variant-import-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "shared.mace", `|===|
type Identity: variant[string, int];
|===|
[output = schema]
{
  Identity: Identity;
}`)
		processor := New()
		result, err := processor.ProcessFile(writeFixtureFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import Identity;
Identity first = "Ada";
Identity second = 42;
|===|
[output = data]
{
  result: {
    first: first;
    second: second;
  };
}`))
		tAssert.NoError(err)

		actual := requireOutputValue(result, "result")
		assertExpectedValue(actual, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"first":  {kind: ValueString, string: "Ada"},
			"second": {kind: ValueInt, int64: 42},
		}})
	})

	It("rejects imported schema output files that declare script variables", func() {
		workspace, err := os.MkdirTemp("", "mace-import-schema-output-variable-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "producer.mace", `|===|
schema User: { name: string; };
string local = "Ada";
|===|
[output = schema]
{
  User: User;
}`)
		consumer := writeFixtureFile(workspace, "consumer.mace", `|===|
from "./producer.mace" import User;
|===|
[output = data]
{
  result: { name: "Ada"; };
}`)

		processor := New()
		_, err = processor.ProcessFile(consumer)
		tAssert.Error(err)
		tAssert.ErrorContains(err, `script variable "local" is not allowed when output = schema`)
	})

	DescribeTable("keeps hidden declarations internal",
		func(file string, message string) {
			processor := New()
			_, err := processor.ProcessInDir(file, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("hidden type is not importable", `|===|
from "fixtures/processor/imports/base.mace" import Internal;
|===|
[output = data] {}`, "imported identifier"),
		Entry("hidden schema is not importable", `|===|
from "fixtures/processor/imports/base.mace" import Secret;
|===|
[output = data] {}`, "imported identifier"),
		Entry("hidden value is not importable", `|===|
from "fixtures/processor/imports/values.mace" import hidden;
|===|
[output = data] {}`, "imported identifier"),
		Entry("hidden schema in a data fixture is not importable", `|===|
from "fixtures/processor/imports/metrics.mace" import Hidden;
|===|
[output = data] {}`, "imported identifier"),
	)

	DescribeTable("processes imported files",
		func(path string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessFileInDir(path, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("resolves imports relative to file", "../../fixtures/processor/imports/consumer.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 27},
		}}),
		Entry("resolves schema_file relative to file", "../../fixtures/processor/schema_file/consumer.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
		}}),
	)

	DescribeTable("processes practical choice fixtures",
		func(path string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessFileInDir(path, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("deployment environment choices", "../../fixtures/processor/choices/deployment.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"app":         {kind: ValueString, string: "billing-api"},
			"environment": {kind: ValueString, string: "prod"},
			"region":      {kind: ValueString, string: "us-east-1"},
			"replicas":    {kind: ValueInt, int64: 4},
		}}),
		Entry("nested permission choices", "../../fixtures/processor/choices/permissions.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"role":       {kind: ValueString, string: "admin"},
			"permission": {kind: ValueString, string: "approve"},
			"resource":   {kind: ValueString, string: "invoice"},
		}}),
		Entry("mixed scalar shipping choices", "../../fixtures/processor/choices/shipping.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"order_id":           {kind: ValueString, string: "ORD-1001"},
			"method":             {kind: ValueString, string: "express"},
			"package_tier":       {kind: ValueInt, int64: 2},
			"signature_required": {kind: ValueBoolean, bool: true},
		}}),
		Entry("composed contact channel choices", "../../fixtures/processor/choices/mixed_choices.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"customer_id": {kind: ValueString, string: "CUST-42"},
			"preferred":   {kind: ValueString, string: "email"},
			"fallback":    {kind: ValueString, string: "chat"},
		}}),
		Entry("choice nested inside variant", "../../fixtures/processor/choices/choice_variant.mace", expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"reviewer": {kind: ValueString, string: "ada"},
			"outcome":  {kind: ValueString, string: "approved"},
			"note":     {kind: ValueString, string: "ready to ship"},
		}}),
	)

	It("processes nested variable array access fixtures", func() {
		processor := New()
		result, err := processor.Process(`|============================================================|
array<int> level1 = [1];
array<array<int>> level2 = [[2]];
array<array<array<int>>> level3 = [[[3]]];
array<array<array<array<int>>>> level4 = [[[[4]]]];
array<array<array<array<array<int>>>>> level5 = [[[[[5]]]]];
|============================================================|
[output = data]
{
  level1: level1[0],
  level2: level2[0][0],
  level3: level3[0][0][0],
  level4: level4[0][0][0][0],
  level5: level5[0][0][0][0][0],
}
`)
		tAssert.NoError(err)
		assertExpectedOutput(result, map[string]expectedValue{
			"level1": {kind: ValueInt, int64: 1},
			"level2": {kind: ValueInt, int64: 2},
			"level3": {kind: ValueInt, int64: 3},
			"level4": {kind: ValueInt, int64: 4},
			"level5": {kind: ValueInt, int64: 5},
		})
	})

	DescribeTable("rejects circular imports",
		func(path string) {
			processor := New()
			_, err := processor.ProcessFileInDir(path, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, "circular import")
		},
		Entry("cycle detected", "../../fixtures/processor/imports/cycle_a.mace"),
	)

	DescribeTable("rejects invalid imports",
		func(file string, message string) {
			processor := New()
			_, err := processor.ProcessInDir(file, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown imported identifier", `|===|
from "fixtures/processor/imports/base.mace" import Missing;
|===|
[output = data] {}`, "imported identifier"),
		Entry("duplicate import across declarations", `|===|
from "fixtures/processor/imports/base.mace" import Name;
from "fixtures/processor/imports/other.mace" import Name;
|===|
[output = data] {}`, "duplicate import"),
		Entry("import file missing", `|===|
from "fixtures/processor/imports/missing.mace" import Name;
|===|
[output = data] {}`, "unable to read import file"),
		Entry("import collides with local declaration", `|===|
from "fixtures/processor/imports/base.mace" import Name;
type Name: string;
|===|
[output = data] {}`, "duplicate declaration"),
	)

	It("rejects imports that escape the activation directory", func() {
		workspace, err := os.MkdirTemp("", "mace-import-root-boundary-*")
		tAssert.NoError(err)

		outsidePath := writeFixtureFile(workspace, "shared.mace", `[output = schema]
{
  User: string;
}`)
		consumerDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(consumerDir, 0o755))
		consumerPath := writeFixtureFile(consumerDir, "consumer.mace", `|===|
from "../shared.mace" import User;
|===|
[output = data]
{}`)

		processor := New()
		_, err = processor.ProcessFileInDir(consumerPath, consumerDir)
		tAssert.Error(err)
		tAssert.ErrorContains(err, `import path "../shared.mace" escapes root:`)
		tAssert.FileExists(outsidePath)
	})

	It("allows parent-relative imports during scoped processing", func() {
		workspace, err := os.MkdirTemp("", "mace-import-scope-parent-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "shared.mace", `[output = data]
{
  value: "Ada";
}`)

		consumerDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(consumerDir, 0o755))
		input := `|===|
from "../shared.mace" import value;
|===|
[output = data]
{
  result: value;
}`

		processor := New()
		result, err := processor.ProcessInScope(input, consumerDir, consumerDir)
		tAssert.NoError(err)
		assertExpectedOutput(result, map[string]expectedValue{
			"result": {kind: ValueString, string: "Ada"},
		})
	})

	DescribeTable("validates local schema_file output schema structure",
		func(schemaFile string, validOutput string, invalidOutput string, message string) {
			workspace, err := os.MkdirTemp("", "mace-schema-file-validation-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			writeFixtureFile(workspace, "schema.mace", schemaFile)

			processor := New()
			for _, directive := range []string{`[schema_file = "./schema.mace"]`, `[output = data, schema_file = "./schema.mace"]`} {
				_, err = processor.ProcessInDir(directive+"\n"+validOutput, workspace)
				tAssert.NoError(err)
			}

			_, err = processor.ProcessInDir(`[schema_file = "./schema.mace"]`+"\n"+invalidOutput, workspace)
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("top-level fields with optional fields", `[output = schema]
{
  name: string;
  version: string;
  exports?: record<string>;
}`, `{
  name: "mace";
  version: "1.0.0";
}`, `{
  name: "mace";
}`, `missing required field "version"`),
		Entry("nested fields with optional fields", `[output = schema]
{
  user: {
    name: string;
    age?: int;
    personality: choice["nice", "naive", "hateful"];
  };
}`, `{
  user: {
    name: "Ada";
    personality: "nice";
  };
}`, `{
  name: "Ada";
  personality: "nice";
}`, `missing required field "user"`),
		Entry("many fields with records of known types", `|===|
schema Service: {
  image: string;
  replicas?: int;
};
|===|
[output = schema]
{
  services: record<Service>;
  labels?: record<string>;
  ports: record<int>;
}`, `{
  services: {
    api: { image: "nginx"; replicas: 2; };
    worker: { image: "worker"; };
  };
  ports: {
    api: 8080;
    worker: 9090;
  };
}`, `{
  services: {
    api: { image: "nginx"; replicas: "two"; };
  };
  ports: {
    api: 8080;
  };
}`, `type mismatch`),
		Entry("fields that have records as types", `[output = schema]
{
  user: {
    name: string;
    age?: int;
  };
  package: {
    name: string;
    version: string;
    exports: record<string>;
  };
  audit?: {
    created_by: string;
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: "./dist/index.js";
    };
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: 1;
    };
  };
}`, `type mismatch`),
	)

	It("rejects schema_file paths that escape the activation directory", func() {
		workspace, err := os.MkdirTemp("", "mace-schema-file-root-boundary-*")
		tAssert.NoError(err)

		writeFixtureFile(workspace, "shared.mace", `|===|
schema User: { name: string; };
|===|
[output = schema]
{
  User: User;
}`)
		consumerDir := filepath.Join(workspace, "nested")
		tAssert.NoError(os.MkdirAll(consumerDir, 0o755))
		consumerPath := writeFixtureFile(consumerDir, "consumer.mace", `[output = data, schema_file = "../shared.mace"]
{}`)

		processor := New()
		_, err = processor.ProcessFileInDir(consumerPath, consumerDir)
		tAssert.Error(err)
		tAssert.ErrorContains(err, `import path "../shared.mace" escapes root:`)
	})

	It("imports choice aliases exposed through schema output", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-choice-import-*")
		tAssert.NoError(err)

		sharedPath := writeFixtureFile(workspace, "shared.mace", `|===|
 type Fruit: choice["Apple", "Strawberry"];
|===|
[output = schema]
{
  Fruit: Fruit;
}`)
		consumerPath := writeFixtureFile(workspace, "consumer.mace", `|===|
from "./shared.mace" import Fruit;
Fruit result = "Apple";
|===|
[output = data]
{
  result: result;
}`)

		processor := New()
		result, err := processor.ProcessFile(consumerPath)
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Apple"})
		tAssert.FileExists(sharedPath)
	})

	It("imports remote mace files over http", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/shared.mace":
				_, _ = writer.Write([]byte(`[output = data]
{
  value: "Ada";
}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		input := fmt.Sprintf(`|===|
from %q import value;
|===|
[output = data]
{
  result: value;
}`, server.URL+"/shared.mace")

		processor := New()
		result, err := processor.ProcessInDir(input, "../..")
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Ada"})
	})

	DescribeTable("validates remote schema_file output schema structure over http",
		func(schemaFile string, validOutput string, invalidOutput string, message string) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/schema.mace":
					_, _ = writer.Write([]byte(schemaFile))
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			processor := New()
			for _, directive := range []string{
				fmt.Sprintf(`[schema_file = %q]`, server.URL+"/schema.mace"),
				fmt.Sprintf(`[output = data, schema_file = %q]`, server.URL+"/schema.mace"),
			} {
				_, err := processor.ProcessInDir(directive+"\n"+validOutput, "../..")
				tAssert.NoError(err)
			}

			_, err := processor.ProcessInDir(fmt.Sprintf(`[schema_file = %q]`, server.URL+"/schema.mace")+"\n"+invalidOutput, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("top-level fields with optional fields", `[output = schema]
{
  name: string;
  version: string;
  exports?: record<string>;
}`, `{
  name: "mace";
  version: "1.0.0";
}`, `{
  name: "mace";
}`, `missing required field "version"`),
		Entry("nested fields with optional fields", `[output = schema]
{
  user: {
    name: string;
    age?: int;
    personality: choice["nice", "naive", "hateful"];
  };
}`, `{
  user: {
    name: "Ada";
    personality: "nice";
  };
}`, `{
  name: "Ada";
  personality: "nice";
}`, `missing required field "user"`),
		Entry("many fields with records of known types", `|===|
schema Service: {
  image: string;
  replicas?: int;
};
|===|
[output = schema]
{
  services: record<Service>;
  labels?: record<string>;
  ports: record<int>;
}`, `{
  services: {
    api: { image: "nginx"; replicas: 2; };
    worker: { image: "worker"; };
  };
  ports: {
    api: 8080;
    worker: 9090;
  };
}`, `{
  services: {
    api: { image: "nginx"; replicas: "two"; };
  };
  ports: {
    api: 8080;
  };
}`, `type mismatch`),
		Entry("fields that have records as types", `[output = schema]
{
  user: {
    name: string;
    age?: int;
  };
  package: {
    name: string;
    version: string;
    exports: record<string>;
  };
  audit?: {
    created_by: string;
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: "./dist/index.js";
    };
  };
}`, `{
  user: {
    name: "Ada";
  };
  package: {
    name: "mace";
    version: "1.0.0";
    exports: {
      main: 1;
    };
  };
}`, `type mismatch`),
	)

	It("loads remote parse_file output schema records over http", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/shared.mace":
				_, _ = writer.Write([]byte(`[output = schema]
{
  User: { name: string; };
}`))
			case "/schema.mace":
				_, _ = writer.Write([]byte(`|===|
from "./shared.mace" import User;
|===|
[output = schema]
{
  user: User;
}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		processor := NewWithInput(map[string]Value{
			"user": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			}},
		})
		result, err := processor.ProcessInDir(fmt.Sprintf(`[output = data, parse_file = %q]
{
  result: user.name;
}`, server.URL+"/schema.mace"), server.URL)
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Ada"})
	})

	It("resolves relative imports inside remote mace files", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			switch request.URL.Path {
			case "/shared/base.mace":
				_, _ = writer.Write([]byte(`[output = data]
{
  value: "Ada";
}`))
			case "/entry.mace":
				_, _ = writer.Write([]byte(`|===|
from "./shared/base.mace" import value;
|===|
[output = data]
{
  result: value;
}`))
			default:
				http.NotFound(writer, request)
			}
		}))
		defer server.Close()

		input := fmt.Sprintf(`|===|
from %q import result;
|===|
[output = data]
{
  result: result;
}`, server.URL+"/entry.mace")

		processor := New()
		result, err := processor.ProcessInDir(input, "../..")
		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "result"), expectedValue{kind: ValueString, string: "Ada"})
	})

	It("rejects remote import urls without a .mace suffix", func() {
		processor := New()
		_, err := processor.Process(`|===|
from "https://example.com/shared" import value;
|===|
[output = data] {}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "must end in .mace")
	})

	It("rejects remote schema_file urls without a .mace suffix", func() {
		processor := New()
		_, err := processor.Process(`[output = data, schema = User, schema_file = "https://example.com/schema"] {}`)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "must end in .mace")
	})
})

var _ = Describe("Output block", func() {
	DescribeTable("rejects invalid directives",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("duplicate output directive", "[output = data, output = schema] {}", "duplicate output directive"),
		Entry("unknown schema in directive", "[output = data, schema = Missing] {}", "unknown schema"),
		Entry("schema directive is invalid in schema mode", "[output = schema, schema = User] {}", "schema directive"),
		Entry("schema_file directive is invalid in schema mode", `[output = schema, schema_file = "./user.mace"] {}`, "schema_file"),
		Entry("parse directive is invalid in schema mode", `[output = schema, parse = User] {}`, "parse directive is invalid when output mode is schema"),
		Entry("parse_file directive is invalid in schema mode", `[output = schema, parse_file = "./user.mace"] {}`, "parse_file directive is invalid when output mode is schema"),
	)

	DescribeTable("returns schema output fields",
		func(input string, expected map[expectedSchemaField]SchemaType) {
			processor := New()
			result, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)

			assertExpectedSchema(result, expected)
		},
		Entry("primitive and optional fields", `[output = schema]
{
  name: string;
  age?: int;
}`, map[expectedSchemaField]SchemaType{
			{name: "name"}:                schemaPrimitive("string"),
			{name: "age", optional: true}: schemaPrimitive("int"),
		}),
		Entry("nested array fields", `[output = schema]
{
  names: array<string>;
  matrix: array<array<int>>;
}`, map[expectedSchemaField]SchemaType{
			{name: "names"}:  schemaArray(schemaPrimitive("string")),
			{name: "matrix"}: schemaArray(schemaArray(schemaPrimitive("int"))),
		}),
		Entry("record fields", `|===|
schema User: { name: string; };
|===|
[output = schema]
{
  profile: { name: string; age?: int; };
  user: User;
}`, map[expectedSchemaField]SchemaType{
			{name: "profile"}: schemaRecord(map[expectedSchemaField]SchemaType{
				{name: "name"}:                schemaPrimitive("string"),
				{name: "age", optional: true}: schemaPrimitive("int"),
			}),
			{name: "user"}: schemaNamed("User"),
		}),
		Entry("variant fields", `[output = schema]
{
  value: variant[string, int];
}`, map[expectedSchemaField]SchemaType{
			{name: "value"}: {Kind: SchemaTypeVariant, Members: []SchemaType{schemaPrimitive("string"), schemaPrimitive("int")}},
		}),
		Entry("choice fields resolve nested choice aliases", `|===|
 type Environment: choice["dev", "prod"];
 type Numeric: choice[1, 2];
|===|
[output = schema]
{
  mode: choice[Environment, Numeric];
}`, map[expectedSchemaField]SchemaType{
			{name: "mode"}: schemaNamed(`choice["dev", "prod", 1, 2]`),
		}),
	)

	DescribeTable("accepts output that matches schema",
		func(input string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)
		},
		Entry("optional field omitted", `|===|
schema User: { name: string; age?: int; };
string name = "Ada";
|===|
[output = data, schema = User]
{ name: name; }`),
		Entry("nested record literal", `|===|
schema Profile: { age: int; };
schema User: { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { age: 30; }; }`),
		Entry("variant array field", `|===|
schema Team: { values: array<variant[string, int]>; };
|===|
[output = data, schema = Team]
{ values: ["Ada", 1]; }`),
		Entry("bare output block defaults to data", `{ result: 1 + 2; }`),
	)

	DescribeTable("rejects output that violates schema",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("missing required field", `|===|
schema User: { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; }`, "missing required field"),
		Entry("unknown output field", `|===|
schema User: { name: string; };
|===|
[output = data, schema = User]
{ name: "Ada"; extra: 1; }`, "unknown output field"),
		Entry("optional output mismatch", `|===|
schema User: { name: string; age: int; };
|===|
[output = data, schema = User]
{ name: "Ada"; age?: 30; }`, "not optional"),
		Entry("nested record mismatch", `|===|
schema Profile: { age: int; };
schema User: { profile: Profile; };
|===|
[output = data, schema = User]
{ profile: { }; }`, "missing required field"),
		Entry("array element mismatch", `|===|
schema Point: { x: int; y: int; };
schema Plot: { points: array<Point>; };
|===|
[output = data, schema = Plot]
{ points: [ { x: 1; y: 2; }, { x: 3; } ]; }`, "missing required field"),
		Entry("choice field rejects values outside the domain", `|===|
 type Fruit: choice["Apple", "Strawberry"];
 schema Basket: { favorite: Fruit; };
|===|
[output = data, schema = Basket]
{ favorite: "Pear"; }`, "type mismatch: expected choice[\"Apple\", \"Strawberry\"], got \"Pear\""),
	)

	DescribeTable("rejects output surface mismatches",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("schema output cannot export variable declarations", `|===|
type Name: string;
schema User: { name: Name; age: int; };
int local = 1;
|===|
[output = schema]
{
  Name: Name;
  User: User;
  foo: local;
}`, `script variable "local" is not allowed when output = schema`),
		Entry("data output cannot export type declarations as values", `|===|
type Name: string;
schema User: { name: string; };
string value = "Ada";
|===|
{
  Name: Name;
  User: User;
  value: value;
}`, "cannot reference type or schema declaration"),
	)

	DescribeTable("returns individual operator results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("unary plus", `[output = data] { result: +7; }`, expectedValue{kind: ValueInt, int64: 7}),
		Entry("unary minus", `[output = data] { result: -5; }`, expectedValue{kind: ValueInt, int64: -5}),
		Entry("logical not", `[output = data] { result: !false; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise not", `[output = data] { result: ~1; }`, expectedValue{kind: ValueInt, int64: ^int64(1)}),
		Entry("hex unary minus", `[output = data] { result: -0xA; }`, expectedValue{kind: ValueHexInt, string: "-0xA"}),
		Entry("addition", `[output = data] { result: 1 + 2; }`, expectedValue{kind: ValueInt, int64: 3}),
		Entry("hex addition", `[output = data] { result: 0x0F + 0x01; }`, expectedValue{kind: ValueHexInt, string: "0x10"}),
		Entry("subtraction", `[output = data] { result: 5 - 3; }`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("multiplication", `[output = data] { result: 2 * 3; }`, expectedValue{kind: ValueInt, int64: 6}),
		Entry("hex multiplication overflow", `[output = data] { result: 0x4000000000000000 * 0x2; }`, expectedValue{kind: ValueHexInt, string: "-0x8000000000000000"}),
		Entry("division", `[output = data] { result: 8 / 2; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("hex division", `[output = data] { result: 0x05 / 0x02; }`, expectedValue{kind: ValueHexFloat, string: "0x2.8"}),
		Entry("modulo", `[output = data] { result: 9 % 4; }`, expectedValue{kind: ValueInt, int64: 1}),
		Entry("hex modulo", `[output = data] { result: 0x05 % 0x02; }`, expectedValue{kind: ValueHexInt, string: "0x1"}),
		Entry("mixed modulo", `[output = data] { result: 9 % 2.5; }`, expectedValue{kind: ValueFloat, float: 1.5}),
		Entry("exponentiation", `[output = data] { result: 2 ** 3; }`, expectedValue{kind: ValueInt, int64: 8}),
		Entry("shift left", `[output = data] { result: 1 << 3; }`, expectedValue{kind: ValueInt, int64: 8}),
		Entry("shift right", `[output = data] { result: 8 >> 1; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("unsigned shift right", `[output = data] { result: 8 >>> 1; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("hex shift left", `[output = data] { result: 0x01 << 0x04; }`, expectedValue{kind: ValueHexInt, string: "0x10"}),
		Entry("less than", `[output = data] { result: 1 < 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("hex greater than", `[output = data] { result: 0x10 > 0x0F; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("less than or equal", `[output = data] { result: 2 <= 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("greater than", `[output = data] { result: 3 > 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("greater than or equal", `[output = data] { result: 2 >= 2; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("equal", `[output = data] { result: 3 == 3; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("not equal", `[output = data] { result: 3 != 4; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise and", `[output = data] { result: 6 & 3; }`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("bitwise xor", `[output = data] { result: 5 ^ 3; }`, expectedValue{kind: ValueInt, int64: 6}),
		Entry("bitwise or", `[output = data] { result: 5 | 2; }`, expectedValue{kind: ValueInt, int64: 7}),
		Entry("hex bitwise or", `[output = data] { result: 0x0F | 0x10; }`, expectedValue{kind: ValueHexInt, string: "0x1F"}),
		Entry("logical and", `[output = data] { result: true && false; }`, expectedValue{kind: ValueBoolean, bool: false}),
		Entry("logical or", `[output = data] { result: true || false; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("ternary", `[output = data] { result: true ? 1 : 2; }`, expectedValue{kind: ValueInt, int64: 1}),
		Entry("array merge", `[output = data] { result: [1, 2] <> [3, 4]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 1},
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
			{kind: ValueInt, int64: 4},
		}}),
		Entry("variant array merge", `|===|
type Scalar: variant[string, int];
array<Scalar> left = [1];
array<Scalar> right = ["x"];
|===|
[output = data] { result: left <> right; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 1},
			{kind: ValueString, string: "x"},
		}}),
		Entry("record merge", `[output = data] { result: { name: "Ada"; nested: { left: 1; shared: 1; }; tags: [1]; } <> { age: 30; nested: { shared: 2; right: 3; }; tags: [2]; }; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
			"nested": {kind: ValueRecord, record: map[string]expectedValue{
				"left":   {kind: ValueInt, int64: 1},
				"shared": {kind: ValueInt, int64: 2},
				"right":  {kind: ValueInt, int64: 3},
			}},
			"tags": {kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 1},
				{kind: ValueInt, int64: 2},
			}},
		}}),
	)

	DescribeTable("returns mixed operator results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("arithmetic precedence", `[output = data] { result: 1 + 2 * 3 - 4; }`, expectedValue{kind: ValueInt, int64: 3}),
		Entry("shift and additive precedence", `[output = data] { result: 1 + 2 << 2; }`, expectedValue{kind: ValueInt, int64: 12}),
		Entry("bitwise precedence", `[output = data] { result: 7 & 3 ^ 1 | 8; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("comparison and logic precedence", `[output = data] { result: 1 < 2 && 3 > 2 || false; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("conditional with logical expression", `[output = data] { result: false || true ? 5 : 2; }`, expectedValue{kind: ValueInt, int64: 5}),
	)

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

	DescribeTable("rejects invalid merge expressions",
		func(input string, expected string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.Contains(err.Error(), expected)
		},
		Entry("different kinds", `[output = data] { result: { name: "Ada"; } <> [1]; }`, "merge operands must have the same type"),
		Entry("primitive operands", `[output = data] { result: 1 <> 2; }`, "expected identifier, array literal, or record literal before '<>'"),
		Entry("different array element types", `|===|
array<int> left = [1];
array<string> right = ["two"];
|===|
[output = data] { result: left <> right; }`, "merge operands must have the same type"),
	)

	DescribeTable("returns math results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("addition and multiplication", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 * 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("subtraction and division", wrapScriptWithOutputFields(`|===|
int result = 20 - 4 / 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 18}),
		Entry("modulo", wrapScriptWithOutputFields(`|===|
int result = 9 % 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("exponentiation", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 8}),
		Entry("unary minus", wrapScriptWithOutputFields(`|===|
int result = -5;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: -5}),
		Entry("unary plus", wrapScriptWithOutputFields(`|===|
int result = +7;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("float arithmetic", wrapScriptWithOutputFields(`|===|
float result = 1.5 + 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("float division", wrapScriptWithOutputFields(`|===|
float result = 7.5 / 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 3.0}),
		Entry("mixed numeric addition", wrapScriptWithOutputFields(`|===|
float result = 1 + 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 3.5}),
		Entry("mixed numeric exponentiation", wrapScriptWithOutputFields(`|===|
float result = 2 ** 3.0;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 8.0}),
		Entry("mixed numeric modulo", wrapScriptWithOutputFields(`|===|
float result = 5 % 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 0.0}),
	)

	DescribeTable("returns operator precedence results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("unary before exponent", wrapScriptWithOutputFields(`|===|
int result = -2 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 4}),
		Entry("exponent is right associative", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 512}),
		Entry("shift after additive", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 << 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 12}),
		Entry("relational after shift", wrapScriptWithOutputFields(`|===|
boolean result = 1 << 2 > 3;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("equality after relational", wrapScriptWithOutputFields(`|===|
boolean result = 1 < 2 == true;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise and before or", wrapScriptWithOutputFields(`|===|
int result = 1 | 2 & 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("bitwise and before xor", wrapScriptWithOutputFields(`|===|
int result = 5 ^ 2 & 1;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
		Entry("logical and before or", wrapScriptWithOutputFields(`|===|
boolean result = true || false && false;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("conditional after logical or", wrapScriptWithOutputFields(`|===|
int result = false || true ? 5 : 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
	)

	DescribeTable("accepts non-math operators in script variables",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("bitwise operators", wrapScriptWithOutputFields(`|===|
int masked = 6 & 3;
int combined = 5 | 2;
int toggled = 5 ^ 3;
int inverted = ~1;
|===|`, "masked: masked;\ncombined: combined;\ntoggled: toggled;\ninverted: inverted;"), map[string]expectedValue{
			"masked":   {kind: ValueInt, int64: 2},
			"combined": {kind: ValueInt, int64: 7},
			"toggled":  {kind: ValueInt, int64: 6},
			"inverted": {kind: ValueInt, int64: ^int64(1)},
		}),
		Entry("shift operators", wrapScriptWithOutputFields(`|===|
int left = 1 << 3;
int right = 8 >> 1;
int logical = 8 >>> 1;
|===|`, "left: left;\nright: right;\nlogical: logical;"), map[string]expectedValue{
			"left":    {kind: ValueInt, int64: 8},
			"right":   {kind: ValueInt, int64: 4},
			"logical": {kind: ValueInt, int64: 4},
		}),
		Entry("comparisons", wrapScriptWithOutputFields(`|===|
boolean less = 3 < 5;
boolean greater = 5 > 3;
|===|`, "less: less;\ngreater: greater;"), map[string]expectedValue{
			"less":    {kind: ValueBoolean, bool: true},
			"greater": {kind: ValueBoolean, bool: true},
		}),
		Entry("equality operators", wrapScriptWithOutputFields(`|===|
boolean equal = 3 == 3;
boolean not_equal = 3 != 4;
|===|`, "equal: equal;\nnot_equal: not_equal;"), map[string]expectedValue{
			"equal":     {kind: ValueBoolean, bool: true},
			"not_equal": {kind: ValueBoolean, bool: true},
		}),
		Entry("logical operators", wrapScriptWithOutputFields(`|===|
boolean result = true && false || true;
boolean not = !false;
|===|`, "result: result;\nnot: not;"), map[string]expectedValue{
			"result": {kind: ValueBoolean, bool: true},
			"not":    {kind: ValueBoolean, bool: true},
		}),
		Entry("ternary operator", wrapScriptWithOutputFields(`|===|
int value = true ? 1 : 2;
|===|`, "value: value;"), map[string]expectedValue{
			"value": {kind: ValueInt, int64: 1},
		}),
	)

	DescribeTable("rejects invalid operator usage",
		func(file string) {
			processor := New()
			_, err := processor.ProcessInDir(file, "../..")
			tAssert.Error(err)
		},
		Entry("boolean with bitwise", wrapScriptWithOutputFields(`|===|
int value = true & false;
|===|`, "value: value;")),
		Entry("numeric with logical", wrapScriptWithOutputFields(`|===|
boolean value = 1 && 2;
|===|`, "value: value;")),
		Entry("string comparison", wrapScriptWithOutputFields(`|===|
boolean value = "a" < "b";
|===|`, "value: value;")),
		Entry("mixed equality", wrapScriptWithOutputFields(`|===|
boolean value = 1 == true;
|===|`, "value: value;")),
		Entry("ternary branch mismatch", wrapScriptWithOutputFields(`|===|
int value = true ? 1 : 2.0;
|===|`, "value: value;")),
	)

	DescribeTable("returns array and record results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("array literal", wrapScriptWithOutputFields(`|===|
int base = 2 + 3;
array<int> result = [base, base + 1, base + 2];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 5},
			{kind: ValueInt, int64: 6},
			{kind: ValueInt, int64: 7},
		}}),
		Entry("string arrays support all string literal forms", wrapScriptWithOutputFields(`|===|
array<string> result = ['Kyle', "Tyrone", """Luke"""];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueString, string: "Kyle"},
			{kind: ValueString, string: "Tyrone"},
			{kind: ValueString, string: "Luke"},
		}}),
		Entry("variant arrays", wrapScriptWithOutputFields(`|===|
array<variant[string, int]> result = ["Ada", 1];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueString, string: "Ada"},
			{kind: ValueInt, int64: 1},
		}}),
		Entry("negative int arrays", wrapScriptWithOutputFields(`|===|
array<int> result = [-1, -2, -3];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: -1},
			{kind: ValueInt, int64: -2},
			{kind: ValueInt, int64: -3},
		}}),
		Entry("nested arrays", wrapScriptWithOutputFields(`|===|
int base = 1 + 2;
array<array<int> > result = [[base, base + 1], [base + 2, base + 3]];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 3},
				{kind: ValueInt, int64: 4},
			}},
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 5},
				{kind: ValueInt, int64: 6},
			}},
		}}),
		Entry("record literal", wrapScriptWithOutputFields(`|===|
schema User: { name: string; age: int; };
int base = 20 + 10;
User result = { name: "Ada"; age: base; };
|===|`, "result: result;"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("nested record literal", wrapScriptWithOutputFields(`|===|
schema Inner: { value: int; };
schema Outer: { inner: Inner; };
int base = 8 + 2;
Outer result = { inner: { value: base; }; };
|===|`, "result: result;"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"inner": {kind: ValueRecord, record: map[string]expectedValue{
				"value": {kind: ValueInt, int64: 10},
			}},
		}}),
		Entry("array of records", wrapScriptWithOutputFields(`|===|
schema Point: { x: int; y: int; };
int base = 1 + 1;
array<Point> result = [
  { x: base; y: base + 1; },
  { x: base + 2; y: base + 3; }
];
|===|`, "result: result;"), expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueRecord, record: map[string]expectedValue{
				"x": {kind: ValueInt, int64: 2},
				"y": {kind: ValueInt, int64: 3},
			}},
			{kind: ValueRecord, record: map[string]expectedValue{
				"x": {kind: ValueInt, int64: 4},
				"y": {kind: ValueInt, int64: 5},
			}},
		}}),
		Entry("primitive array access", wrapScriptWithOutputFields(`|===|
array<int> numbers = [5, 6, 7];
int result = numbers[1];
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 6}),
		Entry("record array access with member access", wrapScriptWithOutputFields(`|===|
schema User: { name: string; age: int; };
array<User> users = [
  { name: "Ada"; age: 30; },
  { name: "Linus"; age: 55; }
];
string result = users[0].name;
|===|`, "result: result;"), expectedValue{kind: ValueString, string: "Ada"}),
		Entry("self reference", wrapScriptWithOutputFields(`|===|
int base = 3 * 4;
|===|`, "base: base;\nresult: $self.base + base;"), expectedValue{kind: ValueInt, int64: 24}),
	)

	DescribeTable("returns inline output expressions",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("inline int expression", `[output = data] { result: 2 + 3 * 4; }`, expectedValue{kind: ValueInt, int64: 14}),
		Entry("inline float expression", `[output = data] { result: 2.5 + 1.5; }`, expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("inline boolean expression", `[output = data] { result: 2 < 3 && true; }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("inline string expression", `[output = data] { result: "hello"; }`, expectedValue{kind: ValueString, string: "hello"}),
		Entry("inline record expression", `[output = data] { result: { name: "Ada"; age: 30; }; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"name": {kind: ValueString, string: "Ada"},
			"age":  {kind: ValueInt, int64: 30},
		}}),
		Entry("inline array expression", `[output = data] { result: [1, 2, 3]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 1},
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
		}}),
		Entry("inline nested array expression", `[output = data] { result: [[1, 2], [3, 4]]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 1},
				{kind: ValueInt, int64: 2},
			}},
			{kind: ValueArray, array: []expectedValue{
				{kind: ValueInt, int64: 3},
				{kind: ValueInt, int64: 4},
			}},
		}}),
		Entry("inline negative float array expression", `[output = data] { result: [-1.5, -2.5]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueFloat, float: -1.5},
			{kind: ValueFloat, float: -2.5},
		}}),
		Entry("inline primitive array access", `[output = data] { result: [1, 2, 3][0]; }`, expectedValue{kind: ValueInt, int64: 1}),
		Entry("inline record array access", `[output = data] { result: [{ name: "Ada"; }, { name: "Linus"; }][1].name; }`, expectedValue{kind: ValueString, string: "Linus"}),
		Entry("inline optional output field", `[output = data] { result?: 1 + 1; }`, expectedValue{kind: ValueInt, int64: 2}),
	)

	DescribeTable("returns inline output blocks with multiple fields",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("multiple fields and self reference", `[output = data] { base: 2 + 2; result: $self.base * 3; }`, map[string]expectedValue{
			"base":   {kind: ValueInt, int64: 4},
			"result": {kind: ValueInt, int64: 12},
		}),
	)

	DescribeTable("returns self reference results by depth",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("level 1", `[output = data] { base: 4; result: $self.base; }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("level 2", `[output = data] { profile: { name: "Ada"; }; result: $self.profile.name; }`, expectedValue{kind: ValueString, string: "Ada"}),
		Entry("level 3", `[output = data] { profile: { details: { age: 30; }; }; result: $self.profile.details.age; }`, expectedValue{kind: ValueInt, int64: 30}),
		Entry("level 4", `[output = data] { tree: { branch: { leaf: { value: 9; }; }; }; result: $self.tree.branch.leaf.value; }`, expectedValue{kind: ValueInt, int64: 9}),
		Entry("level 5", `[output = data] { a: { b: { c: { d: { e: true; }; }; }; }; result: $self.a.b.c.d.e; }`, expectedValue{kind: ValueBoolean, bool: true}),
	)

	DescribeTable("returns nested array access results by depth",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("level 1", `[output = data] { result: [10][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 2", `[output = data] { result: [[10]][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 3", `[output = data] { result: [[[10]]][0][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 4", `[output = data] { result: [[[[10]]]][0][0][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("level 5", `[output = data] { result: [[[[[10]]]]][0][0][0][0][0]; }`, expectedValue{kind: ValueInt, int64: 10}),
	)

	DescribeTable("returns mixed self reference results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("arithmetic with chained fields", `[output = data] { base: 4; doubled: $self.base * 2; result: $self.doubled + $self.base; }`, expectedValue{kind: ValueInt, int64: 12}),
		Entry("conditional with self", `[output = data] { enabled: true; result: $self.enabled ? "on" : "off"; }`, expectedValue{kind: ValueString, string: "on"}),
		Entry("array literal with self", `[output = data] { base: 2; result: [$self.base, $self.base + 1, $self.base + 2]; }`, expectedValue{kind: ValueArray, array: []expectedValue{
			{kind: ValueInt, int64: 2},
			{kind: ValueInt, int64: 3},
			{kind: ValueInt, int64: 4},
		}}),
		Entry("record literal with self", `[output = data] { name: "Ada"; result: { display: $self.name; active: true; }; }`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"display": {kind: ValueString, string: "Ada"},
			"active":  {kind: ValueBoolean, bool: true},
		}}),
		Entry("nested self and operators", `[output = data] { profile: { score: 3; }; result: $self.profile.score * 4 + 1; }`, expectedValue{kind: ValueInt, int64: 13}),
	)

	DescribeTable("rejects invalid self references",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("future field reference", `[output = data] { result: $self.base; base: 4; }`, "unknown self reference"),
		Entry("nested path through non record", `[output = data] { base: 4; result: $self.base.value; }`, "non-record"),
	)

	DescribeTable("rejects invalid array access",
		func(input, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("non array target", `[output = data] { result: 1[0]; }`, "array access requires an array value at level 1"),
		Entry("out of range index", `[output = data] { result: [1, 2][3]; }`, "out of range at level 1"),
		Entry("wrong nested level", `[output = data] { result: [[1]][0][0][0]; }`, "array access requires an array value at level 3"),
	)

	DescribeTable("rejects arrays that do not match declared element types",
		func(file string) {
			processor := New()
			_, err := processor.ProcessInDir(file, "../..")
			tAssert.Error(err)
		},
		Entry("primitive type mismatch", `|===|
array<int> result = [1, "two"];
|===|
[output = data] { result: result; }`),
		Entry("numeric type mismatch", `|===|
array<int> result = [1, 2.0];
|===|
[output = data] { result: result; }`),
		Entry("nested array type mismatch", `|===|
array<array<int>> result = [[1], ["two"]];
|===|
[output = data] { result: result; }`),
	)

	It("imports a schema output as a named schema with import-as", func() {
		processor := NewWithInput(map[string]Value{
			"name":    {Kind: ValueString, String: "@code-fixer-23/cn-efs"},
			"version": {Kind: ValueString, String: "1.0.0"},
			"type":    {Kind: ValueString, String: "commonjs"},
		})
		result, err := processor.ProcessFile("../../fixtures/processor/import_as/consumer.mace")
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "@code-fixer-23/cn-efs"})
		assertExpectedValue(result.Output["version"], expectedValue{kind: ValueString, string: "1.0.0"})
		assertExpectedValue(result.Output["type"], expectedValue{kind: ValueString, string: "commonjs"})
	})

	It("imports a data output as a named record with import-as", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-import-as-data-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		sharedPath := filepath.Join(workspace, "shared.mace")
		tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  project: {
    name: "pi-prompt-form";
    root: "libs/pi-prompt-form";
  };
  workspace: {
    root: ".";
  };
}`), 0o644))

		documentPath := filepath.Join(workspace, "document.mace")
		tAssert.NoError(os.WriteFile(documentPath, []byte(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data]
{
  name: Shared.project.name;
  root: Shared.project.root;
  cwd: Shared.workspace.root;
}`), 0o644))

		result, err := New().ProcessFile(documentPath)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "pi-prompt-form"})
		assertExpectedValue(result.Output["root"], expectedValue{kind: ValueString, string: "libs/pi-prompt-form"})
		assertExpectedValue(result.Output["cwd"], expectedValue{kind: ValueString, string: "."})
	})

	DescribeTable("imports data outputs with import-as across nested levels",
		func(accessor string, expected expectedValue) {
			workspace, err := os.MkdirTemp("", "mace-processor-import-as-data-depth-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = data]
{
  level1: {
    value: "one";
    level2: {
      value: "two";
      level3: {
        value: "three";
        level4: {
          value: "four";
          level5: {
            value: "five";
          };
        };
      };
    };
  };
}`), 0o644))

			documentPath := filepath.Join(workspace, "document.mace")
			tAssert.NoError(os.WriteFile(documentPath, []byte(fmt.Sprintf(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data]
{
  result: %s;
}`, accessor)), 0o644))

			result, err := New().ProcessFile(documentPath)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expected)
		},
		Entry("level 1", "Shared.level1.value", expectedValue{kind: ValueString, string: "one"}),
		Entry("level 2", "Shared.level1.level2.value", expectedValue{kind: ValueString, string: "two"}),
		Entry("level 3", "Shared.level1.level2.level3.value", expectedValue{kind: ValueString, string: "three"}),
		Entry("level 4", "Shared.level1.level2.level3.level4.value", expectedValue{kind: ValueString, string: "four"}),
		Entry("level 5", "Shared.level1.level2.level3.level4.level5.value", expectedValue{kind: ValueString, string: "five"}),
	)

	DescribeTable("imports schema outputs with import-as across nested levels",
		func(accessor string, input Value, expected expectedValue) {
			workspace, err := os.MkdirTemp("", "mace-processor-import-as-schema-depth-*")
			tAssert.NoError(err)
			defer func() { _ = os.RemoveAll(workspace) }()

			sharedPath := filepath.Join(workspace, "shared.mace")
			tAssert.NoError(os.WriteFile(sharedPath, []byte(`[output = schema]
{
  level1: {
    value: string;
    level2: {
      value: string;
      level3: {
        value: string;
        level4: {
          value: string;
          level5: {
            value: string;
          };
        };
      };
    };
  };
}`), 0o644))

			documentPath := filepath.Join(workspace, "document.mace")
			tAssert.NoError(os.WriteFile(documentPath, []byte(fmt.Sprintf(`|===|
from "./shared.mace" import-as Shared;
|===|
[output = data, parse = Shared]
{
  result: %s;
}`, accessor)), 0o644))

			processor := NewWithInput(map[string]Value{"level1": input})
			result, err := processor.ProcessFile(documentPath)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expected)
		},
		Entry("level 1", "level1.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "one"}),
		Entry("level 2", "level1.level2.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "two"}),
		Entry("level 3", "level1.level2.level3.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "three"}),
		Entry("level 4", "level1.level2.level3.level4.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "four"}),
		Entry("level 5", "level1.level2.level3.level4.level5.value", Value{Kind: ValueRecord, Record: map[string]Value{
			"value": {Kind: ValueString, String: "one"},
			"level2": {Kind: ValueRecord, Record: map[string]Value{
				"value": {Kind: ValueString, String: "two"},
				"level3": {Kind: ValueRecord, Record: map[string]Value{
					"value": {Kind: ValueString, String: "three"},
					"level4": {Kind: ValueRecord, Record: map[string]Value{
						"value": {Kind: ValueString, String: "four"},
						"level5": {Kind: ValueRecord, Record: map[string]Value{
							"value": {Kind: ValueString, String: "five"},
						}},
					}},
				}},
			}},
		}}, expectedValue{kind: ValueString, string: "five"}),
	)

	It("surfaces only top-level parsed schema fields as variables", func() {
		processor := NewWithInput(map[string]Value{
			"project": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "pi-prompt-form"},
				"root": {Kind: ValueString, String: "libs/pi-prompt-form"},
			}},
			"workspace": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "workspace"},
				"root": {Kind: ValueString, String: "."},
			}},
		})
		result, err := processor.ProcessFile("../../fixtures/processor/import_as/nx_consumer.mace")
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "pi-prompt-form"})
		assertExpectedValue(result.Output["root"], expectedValue{kind: ValueString, string: "libs/pi-prompt-form"})
		assertExpectedValue(result.Output["cwd"], expectedValue{kind: ValueString, string: "."})
	})

	It("validates arbitrary record keys against a record value type", func() {
		input := `|===|
type Dependencies: record<string>;
schema PackageJSON: {
  name: string,
  dependencies: Dependencies,
}
|===|
[schema=PackageJSON]
{
  name: "pkg",
  dependencies: {
    pi_prompt_guard: "^1.0.0",
    pi_prompt_form: "^1.0.0",
  },
}`
		result, err := New().Process(input)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["dependencies"], expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"pi_prompt_guard": {kind: ValueString, string: "^1.0.0"},
			"pi_prompt_form":  {kind: ValueString, string: "^1.0.0"},
		}})
	})

	It("allows record keyword schema fields to be referenced as values", func() {
		processor := NewWithInput(map[string]Value{
			"record": {Kind: ValueString, String: "value"},
		})
		result, err := processor.Process(`|===|
schema Input: { record: string; };
|===|
[output = data, parse = Input]
{
  record: record;
}`)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["record"], expectedValue{kind: ValueString, string: "value"})
	})

	It("infers member access types for record map values", func() {
		input := `|===|
record<string> deps = { foo: "bar"; };
string foo = deps.foo;
|===|
[output = data]
{
  foo: foo;
}`
		result, err := New().Process(input)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["foo"], expectedValue{kind: ValueString, string: "bar"})
	})

	It("resolves imported types in parse_file output schemas", func() {
		dir, err := os.MkdirTemp("", "mace-parse-file-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(dir) }()
		tAssert.NoError(os.WriteFile(filepath.Join(dir, "shared.mace"), []byte(`[output = schema]
{
  User: { name: string; };
}`), 0o644))
		tAssert.NoError(os.WriteFile(filepath.Join(dir, "schema.mace"), []byte(`|===|
from "./shared.mace" import User;
|===|
[output = schema]
{
  user: User;
}`), 0o644))

		processor := NewWithInput(map[string]Value{
			"user": {Kind: ValueRecord, Record: map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			}},
		})
		result, err := processor.ProcessInDir(`[output = data, parse_file = "./schema.mace"]
{
  name: user.name;
}`, dir)
		tAssert.NoError(err)
		assertExpectedValue(result.Output["name"], expectedValue{kind: ValueString, string: "Ada"})
	})

	Describe("optional field presence guards", func() {
		const guardSchema = `|===|
schema User: {
  name: string;
  manager?: User;
};
|===|
`

		It("evaluates 'in' expression to true when optional field exists in input", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  has_manager: "manager" in input,
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["has_manager"], expectedValue{kind: ValueBoolean, bool: true})
		})

		It("evaluates 'in' expression to false when optional field is absent from input", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  has_manager: "manager" in input,
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["has_manager"], expectedValue{kind: ValueBoolean, bool: false})
		})

		It("rejects unguarded member access on optional parse variable", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			_, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: manager.name,
}`)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "optional field")
			tAssert.ErrorContains(err, "manager")
		})

		It("allows member access on optional parse variable inside 'in' guard", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input ? manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "Bob"})
		})

		It("uses the else branch when the guarded optional field is absent", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input ? manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "none"})
		})

		It("supports 'in' guards with the lowercase schema-name variable", func() {
			processor := NewWithInput(map[string]Value{
				"name":    {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Bob"}}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in user ? manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "Bob"})
		})

		It("validates nested optional access with nested 'in' guards via &&", func() {
			processor := NewWithInput(map[string]Value{
				"name": {Kind: ValueString, String: "Ada"},
				"manager": {Kind: ValueRecord, Record: map[string]Value{
					"name":    {Kind: ValueString, String: "Bob"},
					"manager": {Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Carol"}}},
				}},
			})
			result, err := processor.Process(guardSchema + `[output = data, parse = User]
{
  result: "manager" in input && "manager" in manager ? manager.manager.name : "none",
}`)
			tAssert.NoError(err)
			assertExpectedValue(result.Output["result"], expectedValue{kind: ValueString, string: "Carol"})
		})
	})

	It("rejects record values that do not match the record value type", func() {
		input := `|===|
type Dependencies: record<string>;
schema PackageJSON: {
  dependencies: Dependencies,
}
|===|
[schema=PackageJSON]
{
  dependencies: {
    pi_prompt_guard: 1,
  },
}`
		_, err := New().Process(input)
		tAssert.Error(err)
		tAssert.ErrorContains(err, "type mismatch")
	})
})

var _ = Describe("Registry helpers", func() {
	It("clones and queries symbol, type, schema, and variable registries", func() {
		symbols := newSymbolTable()
		symbols.Add("input", symbolKindImport)
		tAssert.True(symbols.IsImport("input"))
		tAssert.False(symbols.IsVariable("input"))

		types := newTypeRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		typeClone := types.Clone()
		tAssert.Equal(types.aliases["Alias"], typeClone.aliases["Alias"])

		schemas := newSchemaRegistry()
		schemas.Add("User", ast.RecordType{})
		schemaClone := schemas.Clone()
		tAssert.True(schemaClone != nil)
		record, ok := schemaClone.Get("User")
		tAssert.True(ok)
		tAssert.Equal(ast.RecordType{}, record)

		variables := newVariableRegistry()
		variables.Add("value", valueType{kind: ValueString})
		variableClone := variables.Clone()
		value, ok := variableClone.Get("value")
		tAssert.True(ok)
		tAssert.Equal(ValueString, value.kind)
	})
})

var _ = Describe("Processor helpers", func() {
	It("covers export resolution helpers", func() {
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})

		resolved, err := resolveExportedTypeReference(ast.NamedType{Name: "Alias"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, resolved)
		resolved, err = resolveExportedTypeReference(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, resolved)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "Alias"}, types, schemas, map[string]struct{}{"Alias": {}}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{"User": {}})
		tAssert.Error(err)

		fields := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
		resolvedRecord, err := resolveExportedRecordType(fields, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(fields, resolvedRecord)
	})

	It("covers import and schema export helpers", func() {
		workspace, err := os.MkdirTemp("", "processor-imports-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		schemaPath := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ name: string; }`)
		consumerPath := writeFixtureFile(workspace, "consumer.mace", `[output = data, schema_file = "schema.mace"]
{ name: "Ada"; }`)
		badPath := writeFixtureFile(workspace, "bad.mace", `{ name: 1; }`)
		invalidOutputPath := writeFixtureFile(workspace, "invalid-output.mace", `[output = data]
{ name: "Ada"; }`)
		circularA := writeFixtureFile(workspace, "circular-a.mace", `import "circular-b.mace";`)
		_ = writeFixtureFile(workspace, "circular-b.mace", `import "circular-a.mace";`)

		context := newProcessContext(workspace, workspace)
		declarations, err := loadSchemaFileDeclarations(schemaPath, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.NotNil(declarations)
		_, err = loadSchemaFileDeclarations(schemaPath, workspace, map[string]map[string]ast.Declaration{schemaPath: declarations}, map[string]struct{}{})
		tAssert.NoError(err)

		outputDecls, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		tAssert.NoError(err)
		tAssert.NotNil(outputDecls)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)
		tAssert.Error(err)

		loaded, err := loadOutputSchemaRecord(schemaPath, workspace, "schema_file")
		tAssert.NoError(err)
		tAssert.NotEmpty(loaded.Fields)
		_, err = loadOutputSchemaRecord(badPath, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(invalidOutputPath, workspace, "schema_file")
		tAssert.Error(err)

		exports, err := collectImportExports(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, context)
		tAssert.NoError(err)
		tAssert.NotNil(exports)
		_, err = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}}, context)
		tAssert.Error(err)

		fieldDecl, err := schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "item", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "value", Type: ast.PrimitiveType{Name: "string"}}}}}, context)
		tAssert.NoError(err)
		tAssert.Equal(symbolKindSchema, fieldDecl.kind)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "item", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, context)
		tAssert.NoError(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "item", Type: ast.NamedType{Name: "Missing"}}, context)
		tAssert.NoError(err)

		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Mode: ast.OutputModeData}, context)
		tAssert.NoError(err)
		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.Identifier{Name: "missing"}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, context)
		tAssert.Error(err)

		_, err = loadImportExports(consumerPath, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = loadImportExports(circularA, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)

		_, err = loadSchemaFileDeclarations(writeFixtureFile(workspace, "circular-check.mace", `import "circular-check.mace";`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
	})

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

	It("validates output directive shapes and references", func() {
		symbols := newSymbolTable()
		symbols.Add("Schema", symbolKindSchema)
		tAssert.NoError(validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Schema"}, {Kind: ast.OutputDirectiveSchema, Value: "Schema"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Schema"}}}))
		tAssert.NoError(validateOutputDirectiveReferences(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Schema"}}}, symbols))
		tAssert.Error(validateOutputDirectiveReferences(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, symbols))
		tAssert.NoError(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}, symbols, newTypeRegistry(), newSchemaRegistry(), nil))
		tAssert.Error(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}, symbols, newTypeRegistry(), newSchemaRegistry(), nil))
	})

	It("compares and displays choice values by scalar keys", func() {
		valuesEqual := choiceValuesEqual
		valueKeys := choiceValueKeys
		typeName := choiceTypeName
		containsValue := choiceContainsValue
		left := []Value{
			{Kind: ValueString, String: "Ada"},
			{Kind: ValueInt, Int: 7},
			{Kind: ValueBoolean, Boolean: true},
		}
		right := []Value{
			{Kind: ValueBoolean, Boolean: true},
			{Kind: ValueString, String: "Ada"},
			{Kind: ValueInt, Int: 7},
		}

		tAssert.True(valuesEqual(left, right))
		tAssert.False(valuesEqual(left, right[:2]))
		tAssert.Equal([]string{"boolean:true", "int:7", "string:Ada"}, valueKeys(left))
		tAssert.Empty(valueKeys([]Value{{Kind: ValueRecord}}))
		tAssert.Equal(`choice["Ada", 7, true]`, typeName(left))
		tAssert.True(containsValue(left, Value{Kind: ValueString, String: "Ada"}))
		tAssert.False(containsValue(left, Value{Kind: ValueRecord}))
	})

	It("falls back to source labels for unresolved choice schema members", func() {
		typeNameForSchema := choiceTypeNameForSchema
		reference := ast.ChoiceType{Members: []ast.Expression{
			ast.Identifier{Name: "Shared"},
			ast.StringLiteral{Lexeme: `"Ada"`},
			ast.IntLiteral{Lexeme: "7"},
			ast.FloatLiteral{Lexeme: "1.5"},
			ast.HexIntLiteral{Lexeme: "0xFF"},
			ast.HexFloatLiteral{Lexeme: "0x2.8"},
			ast.BooleanLiteral{Value: false},
			ast.RecordLiteral{},
		}}

		name := typeNameForSchema(reference, newTypeRegistry())

		tAssert.Contains(name, "Shared")
		tAssert.Contains(name, `"Ada"`)
		tAssert.Contains(name, "7")
		tAssert.Contains(name, "1.5")
		tAssert.Contains(name, "0xFF")
		tAssert.Contains(name, "0x2.8")
		tAssert.Contains(name, "false")
		tAssert.Contains(name, "ast.RecordLiteral")
	})

	It("covers validation and evaluation branches", func() {
		workspace, err := os.MkdirTemp("", "processor-validation-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas := newSchemaRegistry()
		schemas.Add("User", schema)
		types := newTypeRegistry()
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		symbols.Add("name", symbolKindVariable)
		vars.Add("name", valueType{kind: ValueString})

		tAssert.NoError(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "opt", Value: ast.IntLiteral{Lexeme: "7"}}}}, valueType{kind: ValueRecord, schemaName: "User"}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "unknown", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "7"}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))

		tAssert.NoError(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "opt": {Kind: ValueInt, Int: 7}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "x"}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("Missing", map[string]Value{}, symbols, types, schemas, nil))

		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Bea"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueInt, Int: 7}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"extra": {Kind: ValueString, String: "x"}}}, valueType{kind: ValueRecord, record: &schema}, symbols, types, schemas, nil))
	})

	It("covers validation helper branches", func() {
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas.Add("User", schema)
		symbols.Add("name", symbolKindVariable)
		vars.Add("name", valueType{kind: ValueString})

		tAssert.NoError(validateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, "User", vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordLiteral(ast.RecordLiteral{}, "Missing", vars, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "7"}}}}, schema, "", vars, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueString}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueInt}}, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueInt}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordType(schema, symbols, types, schemas, nil))
	})

	It("covers evaluation branches", func() {
		vars := newValueEnvironment()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		vars.Add("name", Value{Kind: ValueString, String: "Ada"})
		symbols.Add("name", symbolKindVariable)
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})

		_, err := evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.BooleanLiteral{Value: true}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.BooleanLiteral{Value: true}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.BooleanLiteral{Value: true}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: false}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenQuestion, Right: ast.BooleanLiteral{Value: false}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)

		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.StringLiteral{Lexeme: `"a"`}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.StringLiteral{Lexeme: `"a"`}, Right: ast.StringLiteral{Lexeme: `"a"`}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)

		_, err = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		tAssert.Error(err)
		_, err = evaluateComparison(lexer.TokenLess, Value{Kind: ValueString, String: "x"}, Value{Kind: ValueInt, Int: 1})
		tAssert.Error(err)
		_, err = compareNumbers(lexer.TokenPlus, 1, 2)
		tAssert.Error(err)
		_, err = evaluateSelfReference(ast.SelfReference{Path: []string{"missing"}}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		tAssert.Error(err)
		_, err = evaluateSelfReference(ast.SelfReference{Path: []string{"name"}}, Value{Kind: ValueString, String: "Ada"})
		tAssert.Error(err)
		tAssert.Error(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueInt}, {kind: ValueInt}}, newVariableRegistry(), symbols, types, schemas, nil))
		_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateConditional(ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bob"`}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bob"`}}}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
	})

	It("converts runtime value types back to AST type references", func() {
		typeReference := typeReferenceFromValueType
		recordType := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}

		tAssert.Equal(ast.ChoiceType{Members: []ast.Expression{
			ast.StringLiteral{Lexeme: `"Ada"`},
			ast.IntLiteral{Lexeme: "7"},
		}}, typeReference(valueType{choiceValues: []Value{
			{Kind: ValueString, String: "Ada"},
			{Kind: ValueInt, Int: 7},
		}}))
		tAssert.Equal(ast.VariantType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.PrimitiveType{Name: "int"},
		}}, typeReference(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}))
		tAssert.Equal(ast.ArrayType{Element: ast.PrimitiveType{Name: "boolean"}}, typeReference(valueType{kind: ValueArray, element: &valueType{kind: ValueBoolean}}))
		tAssert.Equal(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, typeReference(valueType{kind: ValueArray}))
		tAssert.Equal(ast.NamedType{Name: "User"}, typeReference(valueType{kind: ValueRecord, schemaName: "User"}))
		tAssert.Equal(ast.RecordMapType{Value: ast.PrimitiveType{Name: "float"}}, typeReference(valueType{kind: ValueRecord, element: &valueType{kind: ValueFloat}}))
		tAssert.Equal(recordType, typeReference(valueType{kind: ValueRecord, record: &recordType}))
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, typeReference(valueType{kind: ValueUnknown}))
	})

	It("converts runtime scalar values back to AST expressions", func() {
		expression := expressionFromValue

		tAssert.Equal(ast.StringLiteral{Lexeme: `"Ada"`}, expression(Value{Kind: ValueString, String: "Ada"}))
		tAssert.Equal(ast.IntLiteral{Lexeme: "7"}, expression(Value{Kind: ValueInt, Int: 7}))
		tAssert.Equal(ast.FloatLiteral{Lexeme: "1.5"}, expression(Value{Kind: ValueFloat, Float: 1.5}))
		tAssert.Equal(ast.HexIntLiteral{Lexeme: "0xFF"}, expression(Value{Kind: ValueHexInt, String: "0xFF"}))
		tAssert.Equal(ast.HexFloatLiteral{Lexeme: "0x2.8"}, expression(Value{Kind: ValueHexFloat, String: "0x2.8"}))
		tAssert.Equal(ast.BooleanLiteral{Value: true}, expression(Value{Kind: ValueBoolean, Boolean: true}))
		tAssert.Equal(ast.StringLiteral{Lexeme: `"null"`}, expression(Value{Kind: ValueNull}))
	})

	It("reports diagnostic helper details", func() {
		kindName := directiveKindName
		cause := errors.New("root cause")
		err := DiagnosticError{Message: "wrapped", Cause: cause}

		tAssert.Equal(cause, errors.Unwrap(err))
		tAssert.Equal("missing required field \"name\"", strings.TrimPrefix(missingRequiredFieldError("name", "").Error(), "processor: "))
		tAssert.Equal("output", kindName(ast.OutputDirectiveOutput))
		tAssert.Equal("schema_file", kindName(ast.OutputDirectiveSchemaFile))
		tAssert.Equal("schema", kindName(ast.OutputDirectiveSchema))
		tAssert.Equal("parse", kindName(ast.OutputDirectiveParse))
		tAssert.Equal("parse_file", kindName(ast.OutputDirectiveParseFile))
		tAssert.Equal("unknown", kindName(ast.OutputDirectiveKind(99)))
		tAssert.Equal(ErrorDoc, inferErrorKind("documentation block"))
		tAssert.Equal(ErrorImport, inferErrorKind("import path"))
		tAssert.Equal(ErrorDirective, inferErrorKind("directive mismatch"))
		tAssert.Equal(ErrorDeclaration, inferErrorKind("type alias declaration"))
		tAssert.Equal(ErrorOperator, inferErrorKind("operator operands"))
		tAssert.Equal(ErrorType, inferErrorKind("unknown type reference"))
		tAssert.Equal(ErrorSchema, inferErrorKind("schema field"))
		tAssert.Equal(ErrorRuntime, inferErrorKind("runtime failure"))
		tAssert.Equal(ErrorValue, inferErrorKind("literal value expression"))
		tAssert.Equal(ErrorInternal, inferErrorKind("something else"))
	})

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

	It("derives value types and kind names from evaluated values", func() {
		valueTypeFor := valueTypeFromValue
		kindNameFor := Value.kindName

		arrayType := valueTypeFor(Value{Kind: ValueArray})
		tAssert.Equal(ValueArray, arrayType.kind)
		if tAssert.NotNil(arrayType.element) {
			tAssert.Equal(ValueUnknown, arrayType.element.kind)
		}

		arrayType = valueTypeFor(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}})
		tAssert.Equal(ValueArray, arrayType.kind)
		if tAssert.NotNil(arrayType.element) {
			tAssert.Equal(ValueString, arrayType.element.kind)
		}

		tAssert.Equal(ValueRecord, valueTypeFor(Value{Kind: ValueRecord}).kind)
		tAssert.Equal(ValueBoolean, valueTypeFor(Value{Kind: ValueBoolean}).kind)

		tAssert.Equal("array", kindNameFor(Value{Kind: ValueArray}))
		tAssert.Equal("int", kindNameFor(Value{Kind: ValueInt}))
		tAssert.Equal("float", kindNameFor(Value{Kind: ValueFloat}))
		tAssert.Equal("hex_int", kindNameFor(Value{Kind: ValueHexInt}))
		tAssert.Equal("hex_float", kindNameFor(Value{Kind: ValueHexFloat}))
		tAssert.Equal("boolean", kindNameFor(Value{Kind: ValueBoolean}))
		tAssert.Equal("record", kindNameFor(Value{Kind: ValueRecord}))
		tAssert.Equal("null", kindNameFor(Value{Kind: ValueNull}))
		tAssert.Equal("string", kindNameFor(Value{Kind: ValueString}))
		tAssert.Equal("unknown", kindNameFor(Value{Kind: ValueUnknown}))
	})

	It("converts AST type references to public schema types", func() {
		schemaType := schemaTypeFromTypeReference
		types := newTypeRegistry()
		types.AddAlias("ChoiceAlias", ast.ChoiceType{Members: []ast.Expression{
			ast.StringLiteral{Lexeme: `"Ada"`},
			ast.StringLiteral{Lexeme: `"Bob"`},
		}})

		result, err := schemaType(ast.PrimitiveType{Name: "string"}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaPrimitive("string"), result)

		result, err = schemaType(ast.NamedType{Name: "User"}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaNamed("User"), result)

		result, err = schemaType(ast.ArrayType{Element: ast.PrimitiveType{Name: "int"}}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaArray(schemaPrimitive("int")), result)

		result, err = schemaType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "boolean"}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeRecordMap, Element: &SchemaType{Kind: SchemaTypePrimitive, Name: "boolean"}}, result)

		result, err = schemaType(ast.UnionType{Members: []ast.TypeReference{
			ast.NamedType{Name: "User"},
			ast.NamedType{Name: "Audit"},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeUnion, Members: []SchemaType{schemaNamed("User"), schemaNamed("Audit")}}, result)

		result, err = schemaType(ast.VariantType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.PrimitiveType{Name: "int"},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeVariant, Members: []SchemaType{schemaPrimitive("string"), schemaPrimitive("int")}}, result)

		result, err = schemaType(ast.ChoiceType{Members: []ast.Expression{
			ast.Identifier{Name: "ChoiceAlias"},
			ast.StringLiteral{Lexeme: `"Carol"`},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeNamed, Name: `choice["Ada", "Bob", "Carol"]`}, result)

		result, err = schemaType(ast.RecordType{Fields: []ast.SchemaField{
			{Name: "name", Type: ast.PrimitiveType{Name: "string"}},
			{Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaRecord(map[expectedSchemaField]SchemaType{
			{name: "name"}:                schemaPrimitive("string"),
			{name: "age", optional: true}: schemaPrimitive("int"),
		}), result)

		_, err = schemaType(ast.ArrayType{Element: nil}, types)
		tAssert.ErrorContains(err, "unknown type reference")
	})

	It("infers merge and numeric binary result types", func() {
		mergeType := inferMergeType
		numericType := inferNumericBinary

		recordType := valueType{kind: ValueRecord, schemaName: "User"}
		result, err := mergeType(recordType, recordType)
		tAssert.NoError(err)
		tAssert.Equal(recordType, result)

		arrayElement := valueType{kind: ValueString}
		arrayType := valueType{kind: ValueArray, element: &arrayElement}
		result, err = mergeType(arrayType, arrayType)
		tAssert.NoError(err)
		tAssert.Equal(arrayType, result)

		_, err = mergeType(valueType{kind: ValueString}, recordType)
		tAssert.ErrorContains(err, "records or arrays")

		_, err = mergeType(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Audit"})
		tAssert.ErrorContains(err, "same type")

		result, err = numericType(lexer.TokenPlus, valueType{kind: ValueInt}, valueType{kind: ValueInt})
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, result.kind)

		result, err = numericType(lexer.TokenPlus, valueType{kind: ValueInt}, valueType{kind: ValueFloat})
		tAssert.NoError(err)
		tAssert.Equal(ValueFloat, result.kind)

		result, err = numericType(lexer.TokenSlash, valueType{kind: ValueHexInt}, valueType{kind: ValueHexInt})
		tAssert.NoError(err)
		tAssert.Equal(ValueHexFloat, result.kind)

		result, err = numericType(lexer.TokenPlus, valueType{kind: ValueHexInt}, valueType{kind: ValueHexFloat})
		tAssert.NoError(err)
		tAssert.Equal(ValueHexFloat, result.kind)

		_, err = numericType(lexer.TokenPlus, valueType{kind: ValueString}, valueType{kind: ValueInt})
		tAssert.ErrorContains(err, "numeric operands")

		_, err = numericType(lexer.TokenPlus, valueType{kind: ValueHexInt}, valueType{kind: ValueInt})
		tAssert.ErrorContains(err, "hexadecimal operands")
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

	It("resolves chained and cyclic type aliases", func() {
		resolveReference := (*typeRegistry).resolveTypeReference
		registry := newTypeRegistry()
		registry.AddAlias("Name", ast.PrimitiveType{Name: "string"})
		registry.AddAlias("DisplayName", ast.NamedType{Name: "Name"})
		registry.AddAlias("External", ast.NamedType{Name: "Missing"})
		registry.AddAlias("Loop", ast.NamedType{Name: "Loop"})

		resolved, err := resolveReference(registry, ast.NamedType{Name: "DisplayName"}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, resolved)

		resolved, err = resolveReference(registry, ast.NamedType{Name: "External"}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.NamedType{Name: "Missing"}, resolved)

		resolved, err = resolveReference(registry, ast.PrimitiveType{Name: "int"}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.PrimitiveType{Name: "int"}, resolved)

		_, err = resolveReference(registry, ast.NamedType{Name: "Loop"}, map[string]struct{}{})
		tAssert.ErrorContains(err, "cyclic type alias")
	})

	It("sanitizes imported value types and resolves exported references", func() {
		schemas := newSchemaRegistry()
		schemas.Add("Local", ast.RecordType{})

		recordType := valueType{kind: ValueRecord, schemaName: "Local"}
		arrayType := valueType{kind: ValueArray, element: &recordType}
		variantType := valueType{kind: ValueUnknown, members: []valueType{
			{kind: ValueRecord, schemaName: "External"},
			arrayType,
		}}

		sanitized := sanitizeImportedValueType(variantType, schemas)
		tAssert.Equal("External", sanitized.members[0].schemaName)
		if tAssert.NotNil(sanitized.members[1].element) {
			tAssert.Empty(sanitized.members[1].element.schemaName)
		}

		types := newTypeRegistry()
		types.AddAlias("Name", ast.PrimitiveType{Name: "string"})
		types.AddAlias("Names", ast.ArrayType{Element: ast.NamedType{Name: "Name"}})
		types.AddAlias("Loop", ast.NamedType{Name: "Loop"})
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{
			Name: "name",
			Type: ast.NamedType{Name: "Name"},
		}}})

		resolveExport := resolveExportedTypeReference
		resolved, err := resolveExport(ast.NamedType{Name: "Names"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, resolved)

		resolved, err = resolveExport(ast.RecordMapType{Value: ast.NamedType{Name: "Name"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, resolved)

		resolved, err = resolveExport(ast.UnionType{Members: []ast.TypeReference{
			ast.NamedType{Name: "Name"},
			ast.NamedType{Name: "Missing"},
		}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.UnionType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.NamedType{Name: "Missing"},
		}}, resolved)

		resolved, err = resolveExport(ast.VariantType{Members: []ast.TypeReference{
			ast.NamedType{Name: "Name"},
			ast.PrimitiveType{Name: "int"},
		}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.VariantType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.PrimitiveType{Name: "int"},
		}}, resolved)

		resolved, err = resolveExport(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.RecordType{Fields: []ast.SchemaField{{
			Name: "name",
			Type: ast.PrimitiveType{Name: "string"},
		}}}, resolved)

		_, err = resolveExport(ast.NamedType{Name: "Loop"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.ErrorContains(err, "cyclic type alias")
		_, err = resolveExport(nil, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.ErrorContains(err, "unknown type reference")
	})

	It("exports output field types from schema and inferred values", func() {
		context := newProcessContext(".", ".")
		context.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{
			Name: "name",
			Type: ast.PrimitiveType{Name: "string"},
		}}})
		output := ast.OutputBlock{Directives: []ast.OutputDirective{{
			Kind:  ast.OutputDirectiveSchema,
			Value: "User",
		}}}
		fieldType := exportedOutputFieldType

		result, err := fieldType(ast.OutputField{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}, output, context)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, result.kind)

		result, err = fieldType(ast.OutputField{
			Name:  "age",
			Value: ast.IntLiteral{Lexeme: "42"},
		}, ast.OutputBlock{}, context)
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, result.kind)

		_, err = fieldType(ast.OutputField{Name: "name"}, ast.OutputBlock{Directives: []ast.OutputDirective{{
			Kind:  ast.OutputDirectiveSchema,
			Value: "Missing",
		}}}, context)
		tAssert.ErrorContains(err, "unknown schema")
	})

	It("formats scalar values and evaluates member/prefix/merge helpers", func() {
		formatValue := stringifyValue
		formatted, err := formatValue(Value{Kind: ValueString, String: "Ada"})
		tAssert.NoError(err)
		tAssert.Equal("Ada", formatted)
		formatted, err = formatValue(Value{Kind: ValueHexFloat, Float: -31.5})
		tAssert.NoError(err)
		tAssert.Equal("-0x1F.8", formatted)
		_, err = formatValue(Value{Kind: ValueArray})
		tAssert.ErrorContains(err, "scalar value")

		environment := newValueEnvironment()
		environment.Add("user", Value{Kind: ValueRecord, Record: map[string]Value{
			"name": {Kind: ValueString, String: "Ada"},
		}})
		member, err := evaluateMemberAccess(ast.MemberAccess{
			Target: ast.Identifier{Name: "user"},
			Name:   "name",
		}, environment, Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Equal("Ada", member.String)

		_, err = evaluateMemberAccess(ast.MemberAccess{
			Target: ast.Identifier{Name: "user"},
			Name:   "missing",
		}, environment, Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.ErrorContains(err, "unknown member")

		prefix, err := evaluatePrefix(ast.PrefixExpression{
			Operator: lexer.TokenBang,
			Right:    ast.BooleanLiteral{Value: false},
		}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.True(prefix.Boolean)

		prefix, err = evaluatePrefix(ast.PrefixExpression{
			Operator: lexer.TokenMinus,
			Right:    ast.HexFloatLiteral{Lexeme: "0x1.8"},
		}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueHexFloat, prefix.Kind)
		tAssert.Equal(-1.5, prefix.Float)

		contains, err := evaluateContains(Value{Kind: ValueString, String: "name"}, Value{Kind: ValueRecord, Record: map[string]Value{
			"name": {Kind: ValueString, String: "Ada"},
		}})
		tAssert.NoError(err)
		tAssert.True(contains.Boolean)

		merged, err := evaluateMerge(
			Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}},
			Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Bob"}}},
		)
		tAssert.NoError(err)
		tAssert.Len(merged.Array, 2)

		_, err = evaluateMerge(Value{Kind: ValueString}, Value{Kind: ValueString})
		tAssert.ErrorContains(err, "records or arrays")
	})

	It("evaluates numeric helper operations", func() {
		hexNumeric := evaluateHexNumeric
		floatNumeric := evaluateFloatNumeric
		shiftValue := evaluateShift
		bitwiseValue := evaluateBitwise

		result, err := hexNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexInt, string: "0x5"})

		result, err = hexNumeric(lexer.TokenMinus, Value{Kind: ValueHexFloat, Float: 3.5}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexFloat, string: "0x2.8"})

		result, err = hexNumeric(lexer.TokenStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexFloat, Float: 2.5})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexFloat, string: "0x5.0"})

		result, err = hexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexInt, string: "0x8"})

		result, err = hexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexFloat, Float: 2}, Value{Kind: ValueHexFloat, Float: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexFloat, string: "0x8.0"})

		_, err = hexNumeric(lexer.TokenSlash, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 0})
		tAssert.ErrorContains(err, "division by zero")

		_, err = hexNumeric(lexer.TokenPercent, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.ErrorContains(err, "unknown numeric operator")

		result, err = floatNumeric(lexer.TokenSlash, 7.5, 2.5)
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueFloat, float: 3})

		result, err = floatNumeric(lexer.TokenDoubleStar, 2, 3)
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueFloat, float: 8})

		_, err = floatNumeric(lexer.TokenSlash, 1, 0)
		tAssert.ErrorContains(err, "division by zero")

		_, err = floatNumeric(lexer.TokenPercent, 1, 1)
		tAssert.ErrorContains(err, "unknown numeric operator")

		result, err = shiftValue(lexer.TokenShiftLeft, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueInt, int64: 8})

		result, err = shiftValue(lexer.TokenShiftRightUnsigned, Value{Kind: ValueHexInt, Int: -8}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.NoError(err)
		tAssert.Equal(ValueHexInt, result.Kind)

		_, err = shiftValue(lexer.TokenShiftLeft, Value{Kind: ValueHexFloat, Float: 1}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.ErrorContains(err, "hex_int operands")

		_, err = shiftValue(lexer.TokenShiftLeft, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: -1})
		tAssert.ErrorContains(err, "negative shift")

		_, err = shiftValue(lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		tAssert.ErrorContains(err, "unknown shift")

		result, err = bitwiseValue(lexer.TokenPipe, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueInt, int64: 3})

		result, err = bitwiseValue(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 3}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexInt, string: "0x2"})

		_, err = bitwiseValue(lexer.TokenPipe, Value{Kind: ValueHexFloat, Float: 1}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.ErrorContains(err, "hex_int operands")

		_, err = bitwiseValue(lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		tAssert.ErrorContains(err, "unknown bitwise")
	})

	It("covers string escaping and conditional inference helpers", func() {
		parsedDoc, err := parseDocString(`"""docs"""`)
		tAssert.NoError(err)
		tAssert.Equal("docs", parsedDoc.String)
		_, err = parseDocString(`"""docs`)
		tAssert.Error(err)

		unescaped, length, err := unescapeSequence(`\u0041`)
		tAssert.NoError(err)
		tAssert.Equal("A", unescaped)
		tAssert.Equal(6, length)

		_, _, err = unescapeSequence(`\u00ZZ`)
		tAssert.Error(err)

		runeValue, err := parseUnicodeEscape(`\u0041`, 4)
		tAssert.NoError(err)
		tAssert.Equal('A', runeValue)

		tAssert.Equal("0x1.8", formatHexFloat(1.5))
		tAssert.Equal("0x2.0", formatHexFloat(2))

		result, err := inferConditionalType(ast.ConditionalExpression{
			Condition: ast.BooleanLiteral{Value: true},
			Then:      ast.IntLiteral{Lexeme: "1"},
			Else:      ast.IntLiteral{Lexeme: "2"},
		}, &variableRegistry{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, result.kind)
	})

	It("covers arithmetic, parsing, and type resolution helpers", func() {
		_, err := parseInterpolatedString("unterminated", newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.Error(err)

		intValue, err := parseInt("42")
		tAssert.NoError(err)
		tAssert.Equal(int64(42), intValue.Int)

		floatValue, err := parseFloat("1.5")
		tAssert.NoError(err)
		tAssert.Equal(1.5, floatValue.Float)

		hexInt, err := parseHexInt("0x10")
		tAssert.NoError(err)
		tAssert.Equal(int64(16), hexInt.Int)

		hexFloat, err := parseHexFloat("0x1.8")
		tAssert.NoError(err)
		tAssert.Equal(1.5, hexFloat.Float)

		_, err = parseHexFloat("0x1")
		tAssert.Error(err)

		_, err = parseInt("bad")
		tAssert.Error(err)
		_, err = parseFloat("bad")
		tAssert.Error(err)
		hexIntBad, err := parseHexInt("bad")
		tAssert.NoError(err)
		tAssert.Equal(int64(2989), hexIntBad.Int)
		_, err = parseHexFloat("bad")
		tAssert.Error(err)

		tAssert.True(arrayMergeTypesMatch(Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 2}}}))
		tAssert.False(arrayMergeTypesMatch(Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "a"}}}))

		_, err = resolveUnionRecordType(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry())
		tAssert.ErrorContains(err, "union members must be schemas")
	})

	It("covers numeric and boolean evaluation helpers", func() {
		result, err := evaluateModulo(Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 3})
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, result.Kind)

		_, err = evaluateModulo(Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 0})
		tAssert.Error(err)

		result, err = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 7})
		tAssert.NoError(err)
		tAssert.True(result.Boolean)

		result, err = evaluateComparison(lexer.TokenLess, Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 8})
		tAssert.NoError(err)
		tAssert.True(result.Boolean)

		result, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.False(result.Boolean)

		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)

		schemaResult, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "Profile"}}}}, newTypeRegistry())
		tAssert.NoError(err)
		tAssert.Len(schemaResult, 1)

		_, err = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, newTypeRegistry())
		tAssert.NoError(err)

		fields, err := evaluateOutputFields([]ast.OutputField{{Name: "value", Value: ast.NullLiteral{}}}, newValueEnvironment(), newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Empty(fields)

		coerced, err := coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueArray, coerced.Kind)

		processor := New()
		_, err = processor.processInput(`{ value: 1; }`, ".", ".", false)
		tAssert.NoError(err)

		_, err = processor.processScriptInput(`|===|
int base = 1;
|===|`, ".")
		tAssert.NoError(err)

		scriptResult := ScriptResult{}
		_, err = processor.processOutputInput(`[output = data] { result: 1; }`, scriptResult, ".")
		tAssert.NoError(err)

		_, err = evaluateExpression(ast.Identifier{Name: "missing"}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.Error(err)
	})

	It("covers escaped string content helpers", func() {
		content, _, err := stringContent(`"""abc\n"""`)
		tAssert.NoError(err)
		tAssert.Contains(content, "abc")
		_, _, err = stringContent(`"""\x`)
		tAssert.Error(err)
		_, err = decodeStringLexeme(`"hello"`, false, func(s string) (string, error) { return s, nil })
		tAssert.NoError(err)
		_, err = decodeStringLexeme(`"unterminated`, false, func(s string) (string, error) { return s, nil })
		tAssert.Error(err)
	})

	It("covers validation and inference branches", func() {
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		variables := newVariableRegistry()
		symbols.Add("name", symbolKindVariable)
		variables.Add("name", valueType{kind: ValueString})
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		symbols.Add("Missing", symbolKindType)
		types.AddAlias("Missing", ast.PrimitiveType{Name: "string"})

		tAssert.NoError(validateDataOutputExpression(ast.Identifier{Name: "name"}, symbols, map[string]struct{}{}, map[string]struct{}{}))
		tAssert.NoError(validateDataOutputExpression(ast.Identifier{Name: "missing"}, symbols, map[string]struct{}{}, map[string]struct{}{}))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil))
		_, err := resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.Identifier{Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.BooleanLiteral{Value: true}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.NullLiteral{}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.RecordLiteral{}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.SelfReference{}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "1"}, Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Operator: lexer.TokenAndAnd, Right: ast.BooleanLiteral{Value: false}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueString}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
	})
})
