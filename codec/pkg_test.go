package codec

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

var tAssert *assert.Assertions

func codecPrimitive(name string) SchemaType {
	return SchemaType{Kind: SchemaTypePrimitive, Name: name}
}

func codecRecord(fields map[SchemaField]SchemaType) SchemaType {
	return SchemaType{Kind: SchemaTypeRecord, Fields: fields}
}

func codecUnion(members ...SchemaType) SchemaType {
	return SchemaType{Kind: SchemaTypeUnion, Members: members}
}

func codecVariant(members ...SchemaType) SchemaType {
	return SchemaType{Kind: SchemaTypeVariant, Members: members}
}

func writeCodecTempFile(root string, relativePath string, contents string) string {
	path := filepath.Join(root, filepath.FromSlash(relativePath))
	err := os.MkdirAll(filepath.Dir(path), 0o755)
	tAssert.NoError(err)
	err = os.WriteFile(path, []byte(contents), 0o600)
	tAssert.NoError(err)
	return path
}

func TestBinding(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Binding Suite")
}

type userProfile struct {
	Nickname string `json:"nickname,omitempty"`
	Level    int    `json:"level"`
}

type userConfig struct {
	Name    string      `json:"name"`
	Enabled bool        `json:"enabled"`
	Profile userProfile `json:"profile"`
	Scores  []int       `json:"scores,omitempty"`
}

type decodedConfig struct {
	Name    string                 `json:"name"`
	Enabled bool                   `json:"enabled"`
	Profile decodedProfile         `json:"profile"`
	Flags   []bool                 `json:"flags"`
	Meta    map[string]interface{} `json:"meta"`
}

type decodedProfile struct {
	Level int     `json:"level"`
	Alias *string `json:"alias,omitempty"`
}

var _ = Describe("OutputMap", func() {
	It("converts evaluated output to nested Go maps and slices", func() {
		result, err := Parse(`|===|
int age = 27;
|===|
[output = data]
{
  name: "Ada";
  profile: { age: age; active: true; };
  scores: [1, 2, 3];
}`)
		tAssert.NoError(err)

		output := OutputMap(result)
		tAssert.Equal(map[string]any{
			"name": "Ada",
			"profile": map[string]any{
				"age":    int64(27),
				"active": true,
			},
			"scores": []any{int64(1), int64(2), int64(3)},
		}, output)
	})
})

var _ = Describe("Parse", func() {
	It("parses with injected input values through compatibility helpers", func() {
		result, err := ParseWithInjections(`|===|
schema Runtime: { name: string; enabled: boolean; };
|===|
[output = data, parse = Runtime]
{
  name: name;
  enabled: enabled;
}`, map[string]any{
			"name":    "Ada",
			"enabled": true,
		})
		tAssert.NoError(err)
		tAssert.Equal("Ada", result.Data["name"])
		tAssert.Equal(true, result.Data["enabled"])
	})

	It("parses files with injected input values through compatibility helpers", func() {
		workspace, err := os.MkdirTemp("", "mace-codec-injections-*")
		tAssert.NoError(err)
		path := writeCodecTempFile(workspace, "input.mace", `|===|
schema Runtime: { name: string; };
|===|
[output = data, parse = Runtime]
{
  name: name;
}`)

		result, err := ParseFileWithInjections(path, map[string]any{
			"name": "Ada",
		})
		tAssert.NoError(err)
		tAssert.Equal("Ada", result.Data["name"])
	})

	It("unmarshals with injected input values through compatibility helpers", func() {
		target := struct {
			Name string `json:"name"`
		}{}

		err := UnmarshalWithInjections(`|===|
schema Runtime: { name: string; };
|===|
[output = data, parse = Runtime]
{
  name: name;
}`, map[string]any{
			"name": "Ada",
		}, &target)
		tAssert.NoError(err)
		tAssert.Equal("Ada", target.Name)
	})
	It("returns schema outputs through the public binding result", func() {
		result, err := Parse(`[output = schema]
{
  name: string;
  age?: int;
}`)
		tAssert.NoError(err)
		tAssert.Empty(result.Data)
		tAssert.Equal(map[SchemaField]SchemaType{
			{Name: "name"}:                codecPrimitive("string"),
			{Name: "age", Optional: true}: codecPrimitive("int"),
		}, result.Schema)
	})

	It("returns structured record schema outputs", func() {
		result, err := Parse(`[output = schema]
{
  profile: { name: string; age?: int; };
}`)
		tAssert.NoError(err)
		tAssert.Equal(map[SchemaField]SchemaType{
			{Name: "profile"}: codecRecord(map[SchemaField]SchemaType{
				{Name: "name"}:                codecPrimitive("string"),
				{Name: "age", Optional: true}: codecPrimitive("int"),
			}),
		}, result.Schema)
	})

	It("returns structured variant schema outputs", func() {
		result, err := Parse(`[output = schema]
{
  value: variant[string, int]
}`)
		tAssert.NoError(err)
		tAssert.Equal(map[SchemaField]SchemaType{
			{Name: "value"}: codecVariant(codecPrimitive("string"), codecPrimitive("int")),
		}, result.Schema)
	})

	It("returns structured union schema outputs", func() {
		result, err := Parse(`|===|
schema Profile: { name: string; };
schema Audit: { created_at: string; };
|===|
[output = schema]
{
  value: union[Profile, Audit];
}`)
		tAssert.NoError(err)
		tAssert.Equal(map[SchemaField]SchemaType{
			{Name: "value"}: codecUnion(
				SchemaType{Kind: SchemaTypeNamed, Name: "Profile"},
				SchemaType{Kind: SchemaTypeNamed, Name: "Audit"},
			),
		}, result.Schema)
	})

	It("parses files through the public binding result", func() {
		tempDir, err := os.MkdirTemp("", "mace-binding-*")
		tAssert.NoError(err)

		path := filepath.Join(tempDir, "config.mace")
		err = os.WriteFile(path, []byte(`[output = data] { value: 2 + 2; }`), 0o600)
		tAssert.NoError(err)

		result, err := ParseFile(path)
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{"value": int64(4)}, result.Data)
	})

	It("returns hexadecimal values as strings in public output maps", func() {
		result, err := Parse(`|===|
hex_int mask = 0xFF;
hex_float ratio = 0x2.8;
hex_float whole = 0x2.0;
|===|
[output = data]
{
  mask: mask;
  ratio: ratio;
  whole: whole;
}`)
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{
			"mask":  "0xFF",
			"ratio": "0x2.8",
			"whole": "0x2.0",
		}, result.Data)
	})

	It("applies parse input values from a Go map", func() {
		result, err := ParseWithInput(`|===|
schema Runtime: { env: string; };
|===|
[output = data, parse = Runtime]
{
  env: env;
}`, map[string]any{
			"env": "prod",
		})
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{"env": "prod"}, result.Data)
	})

	It("evaluates interpolated strings through the public binding", func() {
		result, err := Parse(`|===|
int price = 3;
int quantity = 4;
schema User: { name: string; };
User user = { name: "Ada"; };
string summary = "$(user.name): $(price * quantity)";
|===|
[output = data]
{
  summary: summary;
}`)
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{"summary": "Ada: 12"}, result.Data)
	})
})

var _ = Describe("Marshal", func() {
	It("marshals native Go maps into canonical Mace records", func() {
		source, err := Marshal(map[string]any{
			"name":   "Ada",
			"active": true,
			"scores": []int{1, 2, 3},
			"profile": map[string]any{
				"level": 2,
			},
		})
		tAssert.NoError(err)
		tAssert.Equal(`{
  active: true,
  name: "Ada",
  profile: {
    level: 2
  },
  scores: [1, 2, 3]
}`, source)
	})

	It("marshals exported struct fields and respects json tags", func() {
		source, err := Marshal(userConfig{
			Name:    "Ada",
			Enabled: true,
			Profile: userProfile{Level: 3},
		})
		tAssert.NoError(err)
		tAssert.Equal(`{
  name: "Ada",
  enabled: true,
  profile: {
    level: 3
  }
}`, source)
	})

	It("rejects nil values", func() {
		_, err := Marshal(map[string]any{"value": nil})
		tAssert.Error(err)
	})
})

var _ = Describe("MarshalOutput", func() {
	It("returns a bare output record when data is implicit", func() {
		source, err := MarshalOutput(map[string]any{
			"name": "Ada",
			"age":  27,
		})
		tAssert.NoError(err)
		tAssert.Equal(`{
  age: 27,
  name: "Ada"
}`, source)
	})

	It("rejects non-record roots", func() {
		_, err := MarshalOutput([]int{1, 2, 3})
		tAssert.Error(err)
	})
})

var _ = Describe("Check", func() {
	It("reports whether compatibility issues are present", func() {
		tAssert.False(newCheckReport().HasIssues())
		tAssert.True(CheckReport{
			Syntax: []CheckIssue{{
				Path:   "$",
				Reason: "invalid",
				Format: "json",
			}},
		}.HasIssues())
		tAssert.True(CheckReport{
			KeyIncompatibility: []CheckIssue{{
				Path:   "$[\"foo-bar\"]",
				Reason: "key is not a valid Mace identifier",
				Format: "json",
				Key:    "foo-bar",
			}},
		}.HasIssues())
		tAssert.True(CheckReport{
			TypeIncompatibility: []CheckIssue{{
				Path:   "$.name",
				Reason: "null values are omitted during Mace conversion",
				Format: "json",
				Actual: "null",
			}},
		}.HasIssues())
		tAssert.True(CheckReport{
			StructureIncompatibility: []CheckIssue{{
				Path:   "$.name",
				Reason: "duplicate object key",
				Format: "json",
				Key:    "name",
			}},
		}.HasIssues())
	})

	It("reports JSON key incompatibilities in Mace syntax", func() {
		report := CheckJSON(`{
  "name": "Ada",
  "foo-bar": true,
  "profile": {
    "😀": 1
  }
}`)

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [{
      path: "$[\"foo-bar\"]",
      reason: "key is not a valid Mace identifier",
      format: "json",
      key: "foo-bar"
    }, {
      path: "$.profile[\"😀\"]",
      reason: "key is not a valid Mace identifier",
      format: "json",
      key: "😀"
    }],
  type_incompatibility: [],
  structure_incompatibility: []
}`, source)
	})

	It("reports JSON syntax locations and root type mismatches", func() {
		syntaxReport := CheckJSON("{\n  \"name\": \n}")
		tAssert.Len(syntaxReport.Syntax, 1)
		tAssert.Equal("json", syntaxReport.Syntax[0].Format)
		tAssert.NotZero(syntaxReport.Syntax[0].Line)
		tAssert.NotZero(syntaxReport.Syntax[0].Column)

		arrayReport := CheckJSON(`[1, 2, 3]`)
		tAssert.Empty(arrayReport.Syntax)
		if tAssert.Len(arrayReport.StructureIncompatibility, 1) {
			issue := arrayReport.StructureIncompatibility[0]
			tAssert.Equal("$", issue.Path)
			tAssert.Equal("array", issue.Actual)
			tAssert.Equal("record", issue.Expected)
		}
	})

	It("detects extensionless JSON files by parsing their contents", func() {
		root := GinkgoT().TempDir()
		path := writeCodecTempFile(root, "config", "{\n  \"foo-bar\": true\n}\n")

		report, err := CheckFile(path)
		tAssert.NoError(err)
		tAssert.Equal("json", report.Format)
		tAssert.Len(report.Errors.KeyIncompatibility, 1)
		tAssert.Equal("foo-bar", report.Errors.KeyIncompatibility[0].Key)
	})

	It("rejects unsupported check file formats", func() {
		root := GinkgoT().TempDir()
		path := writeCodecTempFile(root, "config.txt", "name = \"Ada\"\n")

		_, err := CheckFile(path)

		tAssert.ErrorContains(err, "could not detect check format")
	})

	It("formats multiple file check reports", func() {
		source, err := FormatFileCheckReports([]FileCheckReport{
			{
				Path:   "config.json",
				Format: "json",
				Errors: newCheckReport(),
			},
		})

		tAssert.NoError(err)
		tAssert.Equal(`{
  files: [{
      path: "config.json",
      format: "json",
      errors: {
        syntax: [],
        key_incompatibility: [],
        type_incompatibility: [],
        structure_incompatibility: []
      }
    }]
}`, source)
	})

	It("reports JSON duplicate keys and null values", func() {
		report := CheckJSON(`{
  "name": null,
  "items": [1, null],
  "name": "Ada"
}`)

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [],
  type_incompatibility: [{
      path: "$.name",
      reason: "null values are omitted during Mace conversion",
      format: "json",
      actual: "null"
    }, {
      path: "$.items[1]",
      reason: "null values are omitted during Mace conversion",
      format: "json",
      actual: "null"
    }],
  structure_incompatibility: [{
      path: "$.name",
      reason: "duplicate object key",
      format: "json",
      key: "name",
      expected: "unique keys"
    }]
}`, source)
	})

	It("reports YAML syntax locations", func() {
		report := CheckYAML("created_at: 2026-05-17T12:30:00Z\nname: Ada\ninvalid: [1, 2\n")
		tAssert.Len(report.Syntax, 1)
		tAssert.Equal("yaml", report.Syntax[0].Format)
		tAssert.NotZero(report.Syntax[0].Line)
		tAssert.Empty(report.KeyIncompatibility)
		tAssert.Empty(report.TypeIncompatibility)
	})

	It("reports YAML timestamp coercion and non-string keys", func() {
		report := CheckYAML("created_at: 2026-05-17T12:30:00Z\n1: Ada\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [{
      path: "$",
      reason: "mapping keys must be strings",
      format: "yaml",
      key: "1",
      line: 2,
      column: 1
    }],
  type_incompatibility: [{
      path: "$.created_at",
      reason: "YAML timestamp values do not map directly to Mace scalars",
      format: "yaml",
      line: 1,
      column: 13,
      actual: "!!timestamp"
    }],
  structure_incompatibility: []
}`, source)
	})

	It("reports YAML duplicate keys, nulls, comments, and block scalar style loss", func() {
		report := CheckYAML("note: |\n  hello\nname: null\nname: Ada # keep me\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [],
  type_incompatibility: [{
      path: "$.name",
      reason: "null values are omitted during Mace conversion",
      format: "yaml",
      line: 3,
      column: 7,
      actual: "!!null"
    }],
  structure_incompatibility: [{
      path: "$.note",
      reason: "block scalar presentation is not preserved during Mace conversion",
      format: "yaml",
      line: 1,
      column: 7,
      actual: "|"
    }, {
      path: "$.name",
      reason: "duplicate mapping key",
      format: "yaml",
      key: "name",
      line: 4,
      column: 1,
      actual: "first declared at 3:1",
      expected: "unique keys"
    }, {
      path: "$.name",
      reason: "comments are not preserved during Mace conversion",
      format: "yaml",
      line: 4,
      column: 7,
      actual: "comment"
    }]
}`, source)
	})

	It("reports YAML folded scalars and invalid merge sequences", func() {
		report := CheckYAML("base: &base\n  name: Ada\nprofile:\n  <<: [*base, 1]\nnote: >\n  hello\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [],
  type_incompatibility: [],
  structure_incompatibility: [{
      path: "$.profile[\"<<\"]",
      reason: "merge values must resolve to records or sequences of records",
      format: "yaml",
      line: 4,
      column: 7,
      actual: "sequence",
      expected: "record or sequence of records"
    }, {
      path: "$.note",
      reason: "block scalar presentation is not preserved during Mace conversion",
      format: "yaml",
      line: 5,
      column: 7,
      actual: ">"
    }]
}`, source)
	})

	It("reports YAML merge values that do not resolve to records", func() {
		report := CheckYAML("base: &base 1\nprofile:\n  <<: *base\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [],
  type_incompatibility: [],
  structure_incompatibility: [{
      path: "$.profile[\"<<\"]",
      reason: "merge values must resolve to records or sequences of records",
      format: "yaml",
      line: 3,
      column: 7,
      actual: "alias",
      expected: "record or sequence of records"
    }]
}`, source)
	})

	It("names YAML node kinds used in merge diagnostics", func() {
		tAssert.Equal("unknown", yamlNodeKindName(nil))
		tAssert.Equal("unknown", yamlNodeKindName(&yaml.Node{}))
	})

	It("accepts YAML aliases and merge keys that can map to Mace merge semantics", func() {
		report := CheckYAML("base: &base\n  name: Ada\nprofile:\n  <<: *base\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [],
  type_incompatibility: [],
  structure_incompatibility: []
}`, source)
	})

	It("detects JSON-looking check input conservatively", func() {
		tAssert.False(isJSONCheckInput(nil))
		tAssert.False(isJSONCheckInput([]byte("")))
		tAssert.False(isJSONCheckInput([]byte("name = \"Ada\"")))
		tAssert.False(isJSONCheckInput([]byte("{")))
		tAssert.True(isJSONCheckInput([]byte(`{"name":"Ada"}`)))
	})

	It("computes line and column positions from byte offsets", func() {
		line, column := lineColumnAtOffset("one\ntwo\n", 6)
		tAssert.Equal(2, line)
		tAssert.Equal(2, column)

		line, column = lineColumnAtOffset("one\n", 0)
		tAssert.Equal(0, line)
		tAssert.Equal(0, column)

		line, column = lineColumnAtOffset("one\n", 99)
		tAssert.Equal(2, line)
		tAssert.Equal(1, column)
	})

	It("names imported value types for diagnostics", func() {
		tAssert.Equal("null", importedValueTypeName(nil))
		tAssert.Equal("record", importedValueTypeName(map[string]any{}))
		tAssert.Equal("array", importedValueTypeName([]any{}))
		tAssert.Equal("string", importedValueTypeName("Ada"))
		tAssert.Equal("boolean", importedValueTypeName(true))
		tAssert.Equal("number", importedValueTypeName(json.Number("1")))
		tAssert.Equal("number", importedValueTypeName(uint(1)))
		tAssert.Equal("struct", importedValueTypeName(struct{}{}))
	})

	It("reports multi-document YAML as a structural migration concern", func() {
		report := CheckYAML("name: Ada\n---\nname: Bob\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [],
  type_incompatibility: [],
  structure_incompatibility: [{
      path: "$",
      reason: "multiple YAML documents require migration before direct Mace use",
      format: "yaml",
      actual: "2 documents",
      expected: "single document"
    }]
}`, source)
	})

	It("accepts TOML timestamps and flags invalid quoted nested keys", func() {
		report := CheckTOML("updated_at = 2026-05-08T09:00:00Z\nsite.\"google.com\" = true\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [{
      path: "$.site[\"google.com\"]",
      reason: "key is not a valid Mace identifier",
      format: "toml",
      key: "google.com"
    }],
  type_incompatibility: [],
  structure_incompatibility: []
}`, source)
	})

	It("reports TOML duplicate keys as syntax errors", func() {
		report := CheckTOML("name = \"Ada\"\nname = \"Bob\"\n")
		tAssert.Len(report.Syntax, 1)
		tAssert.Equal("toml", report.Syntax[0].Format)
		tAssert.Contains(strings.ToLower(report.Syntax[0].Reason), "name")
	})

	It("reports TOML quoted key incompatibilities", func() {
		report := CheckTOML("\"foo-bar\" = \"Ada\"\n")

		source, err := FormatCheckReport(report)
		tAssert.NoError(err)
		tAssert.Equal(`{
  syntax: [],
  key_incompatibility: [{
      path: "$[\"foo-bar\"]",
      reason: "key is not a valid Mace identifier",
      format: "toml",
      key: "foo-bar"
    }],
  type_incompatibility: [],
  structure_incompatibility: []
}`, source)
	})
})

var _ = Describe("Import", func() {
	It("imports JSON objects into a Mace output block", func() {
		source, err := ImportJSON(`{
  "name": "Ada",
  "enabled": true,
  "scores": [1, 2, 3],
  "profile": {
    "level": 2
  }
}`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  enabled: true,
  name: "Ada",
  profile: {
    level: 2
  },
  scores: [1, 2, 3]
}`, source)
	})

	It("imports YAML mappings into a Mace output block", func() {
		source, err := ImportYAML(`name: Ada
enabled: true
profile:
  level: 2
`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  enabled: true,
  name: "Ada",
  profile: {
    level: 2
  }
}`, source)
	})

	It("imports TOML tables into a Mace output block", func() {
		source, err := ImportTOML(`name = "Ada"
enabled = true
scores = [1, 2, 3]

[profile]
level = 2
`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  enabled: true,
  name: "Ada",
  profile: {
    level: 2
  },
  scores: [1, 2, 3]
}`, source)
	})

	It("rejects trailing JSON content", func() {
		_, err := ImportJSON(`{"a":1} {"b":2}`)
		tAssert.ErrorContains(err, "unexpected content after JSON document")
	})

	It("omits null fields from imported JSON data", func() {
		source, err := ImportJSON(`{
  "name": "Ada",
  "nickname": null
}`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  name: "Ada"
}`, source)
	})

	It("rejects empty output blocks after omitting null fields", func() {
		_, err := ImportJSON(`{
  "nickname": null
}`)
		tAssert.ErrorContains(err, "output block is empty")
	})

	It("rejects non-record roots", func() {
		_, err := ImportJSON(`[1, 2, 3]`)
		tAssert.ErrorContains(err, "record root")
	})

	It("imports a schema from an https $schema URL", func() {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			_, err := writer.Write([]byte(`{
  "$id": "mock-schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" }
  },
  "required": ["name"]
}`))
			tAssert.NoError(err)
		}))
		defer server.Close()

		workspace, err := os.MkdirTemp("", "mace-codec-schema-ref-*")
		tAssert.NoError(err)
		jsonPath := writeCodecTempFile(workspace, "requests/https-schema.json", fmt.Sprintf(`{
  "$schema": %q,
  "name": "Ada"
}`, server.URL+"/draft-2020-12/schema.json"))

		source, err := ImportJSONFile(jsonPath)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  name: string
}`, source)
	})

	DescribeTable("imports a schema from local $schema file paths",
		func(referenceKind string, documentPath string) {
			workspace, err := os.MkdirTemp("", "mace-codec-schema-ref-*")
			tAssert.NoError(err)

			schemaPath := "schemas/draft-2020-12/schema.json"
			switch referenceKind {
			case "relative":
				schemaPath = "requests/schemas/draft-2020-12/schema.json"
			case "one-up", "two-up":
				schemaPath = "requests/schemas/draft-2020-12/schema.json"
			}

			schemaFilePath := writeCodecTempFile(workspace, schemaPath, `{
  "$id": "mock-schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" }
  },
  "required": ["name"]
}`)

			var schemaRef string
			switch referenceKind {
			case "file-url":
				urlValue := url.URL{Scheme: "file", Path: filepath.ToSlash(schemaFilePath)}
				schemaRef = urlValue.String()
			case "relative":
				schemaRef = "./schemas/draft-2020-12/schema.json"
			case "one-up":
				schemaRef = "../schemas/draft-2020-12/schema.json"
			case "two-up":
				schemaRef = "../../schemas/draft-2020-12/schema.json"
			case "absolute-unix":
				schemaRef = filepath.ToSlash(schemaFilePath)
			case "absolute-windows":
				if runtime.GOOS != "windows" {
					Skip("windows absolute path syntax requires Windows")
				}
				schemaRef = filepath.ToSlash(schemaFilePath)
			default:
				Fail("unknown schema reference kind")
			}

			jsonPath := writeCodecTempFile(workspace, documentPath, fmt.Sprintf(`{
  "$schema": %q,
  "name": "Ada"
}`, schemaRef))

			source, err := ImportJSONFile(jsonPath)
			tAssert.NoError(err)
			tAssert.Equal(`[output = schema]
{
  name: string
}`, source)
		},
		Entry("file URL", "file-url", "requests/file-url-schema.json"),
		Entry("relative path", "relative", "requests/relative-schema.json"),
		Entry("one folder up", "one-up", "requests/nested/one-up-schema.json"),
		Entry("two folders up", "two-up", "requests/nested/deeper/two-up-schema.json"),
		Entry("absolute unix path", "absolute-unix", "requests/absolute-unix-schema.json"),
		Entry("absolute windows path", "absolute-windows", "requests/absolute-windows-schema.json"),
	)

	It("strips URI fragments before resolving local $schema files", func() {
		workspace, err := os.MkdirTemp("", "mace-codec-schema-fragment-*")
		tAssert.NoError(err)

		writeCodecTempFile(workspace, "requests/schemas/schema.json", `{
  "$id": "mock-schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" }
  },
  "required": ["name"]
}`)
		jsonPath := writeCodecTempFile(workspace, "requests/input.json", `{
  "$schema": "./schemas/schema.json#/$defs/User",
  "name": "Ada"
}`)

		source, err := ImportJSONFile(jsonPath)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  name: string
}`, source)
	})

	It("times out slow remote $schema fetches", func() {
		previousClient := jsonSchemaHTTPClient
		jsonSchemaHTTPClient = &http.Client{Timeout: 20 * time.Millisecond}
		defer func() {
			jsonSchemaHTTPClient = previousClient
		}()

		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			time.Sleep(100 * time.Millisecond)
			_, err := writer.Write([]byte(`{
  "$id": "slow-schema",
  "type": "object",
  "properties": {
    "name": { "type": "string" }
  },
  "required": ["name"]
}`))
			tAssert.NoError(err)
		}))
		defer server.Close()

		workspace, err := os.MkdirTemp("", "mace-codec-schema-timeout-*")
		tAssert.NoError(err)
		jsonPath := writeCodecTempFile(workspace, "requests/slow-schema.json", fmt.Sprintf(`{
  "$schema": %q,
  "name": "Ada"
}`, server.URL+"/draft-2020-12/schema.json"))

		_, err = ImportJSONFile(jsonPath)
		tAssert.ErrorContains(err, "fetch $schema")
	})

	It("rejects invalid $schema URLs during JSON import", func() {
		_, err := ImportJSON(`{
  "$schema": "://",
  "type": "object",
  "properties": {
    "name": { "type": "string" }
  }
}`)
		tAssert.ErrorContains(err, "invalid $schema URL")
	})

	DescribeTable("detects JSON Schema composition keywords during JSON import",
		func(keyword string, expected string) {
			source, err := ImportJSON(fmt.Sprintf(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Profile": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      },
      "required": ["name"]
    },
    "Audit": {
      "type": "object",
      "properties": {
        "created_at": { "type": "string" }
      },
      "required": ["created_at"]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "%s": [
        { "$ref": "#/$defs/Profile" },
        { "$ref": "#/$defs/Audit" }
      ]
    }
  },
  "required": ["value"]
}`, keyword))
			tAssert.NoError(err)
			tAssert.Equal(expected, source)
		},
		Entry("oneOf becomes a variant", "oneOf", `|===|
schema Profile: {
  name: string
}
schema Audit: {
  created_at: string
}
|===|
[output = schema]
{
  value: variant[Profile, Audit]
}`),
		Entry("anyOf becomes a variant", "anyOf", `|===|
schema Profile: {
  name: string
}
schema Audit: {
  created_at: string
}
|===|
[output = schema]
{
  value: variant[Profile, Audit]
}`),
		Entry("allOf becomes a union", "allOf", `|===|
schema Profile: {
  name: string
}
schema Audit: {
  created_at: string
}
|===|
[output = schema]
{
  value: union[Profile, Audit]
}`),
	)
})

var _ = Describe("ImportSchema", func() {
	DescribeTable("maps primitive variant alternatives inline",
		func(types string, expected string) {
			source, err := ImportJSONSchema(fmt.Sprintf(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "properties": {
    "value": {
      "type": %s
    }
  },
  "required": ["value"]
}`, types))
			tAssert.NoError(err)
			tAssert.Equal(expected, source)
		},
		Entry("string-int", `["string", "integer"]`, `[output = schema]
{
  value: variant[string, int]
}`),
		Entry("string-float", `["string", "number"]`, `[output = schema]
{
  value: variant[string, float]
}`),
		Entry("string-boolean", `["string", "boolean"]`, `[output = schema]
{
  value: variant[string, boolean]
}`),
		Entry("int-float", `["integer", "number"]`, `[output = schema]
{
  value: variant[int, float]
}`),
		Entry("int-boolean", `["integer", "boolean"]`, `[output = schema]
{
  value: variant[int, boolean]
}`),
		Entry("float-boolean", `["number", "boolean"]`, `[output = schema]
{
  value: variant[float, boolean]
}`),
	)

	It("imports JSON schema documents into a Mace output schema block", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "properties": {
    "name": { "type": "string" },
    "age": { "type": "integer" }
  },
  "required": ["name"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  age?: int,
  name: string
}`, source)
	})

	It("maps nullable fields to optional schema fields", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "properties": {
    "nickname": {
      "type": ["string", "null"]
    }
  }
}`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  nickname?: string
}`, source)
	})

	It("maps multi-type variant alternatives inline", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "properties": {
    "value": {
      "type": ["string", "integer"]
    }
  },
  "required": ["value"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  value: variant[string, int]
}`, source)
	})

	It("imports nested objects and array items from JSON schema", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "properties": {
    "users": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" }
        },
        "required": ["name"]
      }
    }
  },
  "required": ["users"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  users: array<{
    name: string
  }>
}`, source)
	})

	It("maps inline enums to Mace enums", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "properties": {
    "status": {
      "enum": ["draft", "published"]
    }
  },
  "required": ["status"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
enum Status: string {
  Draft = "draft",
  Published = "published"
}
|===|
[output = schema]
{
  status: Status
}`, source)
	})

	It("maps $defs references into reusable Mace declarations", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Profile": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      },
      "required": ["name"]
    },
    "Role": {
      "enum": ["admin", "member"]
    }
  },
  "type": "object",
  "properties": {
    "profile": {
      "$ref": "#/$defs/Profile"
    },
    "role": {
      "$ref": "#/$defs/Role"
    }
  },
  "required": ["profile", "role"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
schema Profile: {
  name: string
}
enum Role: string {
  Admin = "admin",
  Member = "member"
}
|===|
[output = schema]
{
  profile: Profile,
  role: Role
}`, source)
	})

	It("maps const values to single-member enums", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "properties": {
    "status": {
      "const": "draft"
    }
  },
  "required": ["status"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
enum Status: string {
  Draft = "draft"
}
|===|
[output = schema]
{
  status: Status
}`, source)
	})

	It("maps primitive and array $defs into Mace type aliases", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Name": {
      "type": "string"
    },
    "Count": {
      "type": "integer"
    },
    "Ratio": {
      "type": "number"
    },
    "Enabled": {
      "type": "boolean"
    },
    "Tags": {
      "type": "array",
      "items": {
        "type": "string"
      }
    }
  },
  "type": "object",
  "properties": {
    "name": {
      "$ref": "#/$defs/Name"
    },
    "count": {
      "$ref": "#/$defs/Count"
    },
    "ratio": {
      "$ref": "#/$defs/Ratio"
    },
    "enabled": {
      "$ref": "#/$defs/Enabled"
    },
    "tags": {
      "$ref": "#/$defs/Tags"
    }
  },
  "required": ["name", "count", "ratio", "enabled", "tags"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
type Count: int;
type Enabled: boolean;
type Name: string;
type Ratio: float;
type Tags: array<string>;
|===|
[output = schema]
{
  count: Count,
  enabled: Enabled,
  name: Name,
  ratio: Ratio,
  tags: Tags
}`, source)
	})

	It("maps variant $defs into type aliases", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Value": {
      "type": ["string", "integer"]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "$ref": "#/$defs/Value"
    }
  },
  "required": ["value"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
type Value: variant[string, int];
|===|
[output = schema]
{
  value: Value
}`, source)
	})

	It("maps same-backing enum variant alternatives", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Role": {
      "enum": ["admin", "member"]
    },
    "State": {
      "enum": ["active", "paused"]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "oneOf": [
        { "$ref": "#/$defs/Role" },
        { "$ref": "#/$defs/State" }
      ]
    }
  },
  "required": ["value"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
enum Role: string {
  Admin = "admin",
  Member = "member"
}
enum State: string {
  Active = "active",
  Paused = "paused"
}
|===|
[output = schema]
{
  value: variant[Role, State]
}`, source)
	})

	It("maps anyOf alternatives into variants", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Profile": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      },
      "required": ["name"]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "anyOf": [
        { "$ref": "#/$defs/Profile" },
        { "type": "string" }
      ]
    }
  },
  "required": ["value"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
schema Profile: {
  name: string
}
|===|
[output = schema]
{
  value: variant[Profile, string]
}`, source)
	})

	It("maps allOf composition into unions", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Profile": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      },
      "required": ["name"]
    },
    "Audit": {
      "type": "object",
      "properties": {
        "created_at": { "type": "string" }
      },
      "required": ["created_at"]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "allOf": [
        { "$ref": "#/$defs/Profile" },
        { "$ref": "#/$defs/Audit" }
      ]
    }
  },
  "required": ["value"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
schema Profile: {
  name: string
}
schema Audit: {
  created_at: string
}
|===|
[output = schema]
{
  value: union[Profile, Audit]
}`, source)
	})

	It("rejects enum variant alternatives with mixed backing types", func() {
		_, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Role": {
      "enum": ["admin", "member"]
    },
    "Status": {
      "enum": [0, 1]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "oneOf": [
        { "$ref": "#/$defs/Role" },
        { "$ref": "#/$defs/Status" }
      ]
    }
  },
  "required": ["value"]
}`)
		tAssert.ErrorContains(err, "same backing type")
	})

	It("maps schema and primitive variant alternatives", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Profile": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      },
      "required": ["name"]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "oneOf": [
        { "$ref": "#/$defs/Profile" },
        { "type": "string" }
      ]
    }
  },
  "required": ["value"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
schema Profile: {
  name: string
}
|===|
[output = schema]
{
  value: variant[Profile, string]
}`, source)
	})

	It("rejects schema and enum variant alternatives", func() {
		_, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Profile": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      },
      "required": ["name"]
    },
    "Role": {
      "enum": ["admin", "member"]
    }
  },
  "type": "object",
  "properties": {
    "value": {
      "oneOf": [
        { "$ref": "#/$defs/Profile" },
        { "$ref": "#/$defs/Role" }
      ]
    }
  },
  "required": ["value"]
}`)
		tAssert.ErrorContains(err, "enum variants")
	})

	It("supports recursive $defs schema references", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Node": {
      "type": "object",
      "properties": {
        "name": { "type": "string" },
        "child": { "$ref": "#/$defs/Node" }
      },
      "required": ["name"]
    }
  },
  "type": "object",
  "properties": {
    "root": {
      "$ref": "#/$defs/Node"
    }
  },
  "required": ["root"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
schema Node: {
  child?: Node,
  name: string
}
|===|
[output = schema]
{
  root: Node
}`, source)
	})

	It("maps object and array-of-object $defs into schemas and aliases", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "User": {
      "type": "object",
      "properties": {
        "name": { "type": "string" }
      },
      "required": ["name"]
    },
    "Users": {
      "type": "array",
      "items": {
        "$ref": "#/$defs/User"
      }
    }
  },
  "type": "object",
  "properties": {
    "users": {
      "$ref": "#/$defs/Users"
    }
  },
  "required": ["users"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
schema User: {
  name: string
}
type Users: array<User>;
|===|
[output = schema]
{
  users: Users
}`, source)
	})

	It("maps integer $defs enums into Mace int enums", func() {
		source, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "$defs": {
    "Status": {
      "enum": [0, 1]
    }
  },
  "type": "object",
  "properties": {
    "status": {
      "$ref": "#/$defs/Status"
    }
  },
  "required": ["status"]
}`)
		tAssert.NoError(err)
		tAssert.Equal(`|===|
enum Status: int {
  Value0 = 0,
  Value1 = 1
}
|===|
[output = schema]
{
  status: Status
}`, source)
	})

	It("rejects unsupported additionalProperties schemas", func() {
		_, err := ImportJSONSchema(`{
  "$schema": "./schemas/draft-2020-12/schema.json",
  "type": "object",
  "additionalProperties": {
    "type": "string"
  }
}`)
		tAssert.ErrorContains(err, "additionalProperties")
	})

	It("treats TOML datetimes as strings in output values", func() {
		source, err := ImportTOML(`created_at = 1979-05-27T07:32:00Z`)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  created_at: "1979-05-27T07:32:00Z"
}`, source)
	})
})

var _ = Describe("Unmarshal", func() {
	It("unmarshals output records into structs", func() {
		input := `[output = data]
{
  name: "Ada";
  enabled: true;
  profile: {
    level: 3;
  };
  flags: [true, false];
  meta: {
    retries: 2;
  };
}`

		var config decodedConfig
		err := Unmarshal(input, &config)
		tAssert.NoError(err)
		tAssert.Equal("Ada", config.Name)
		tAssert.Equal(true, config.Enabled)
		tAssert.Equal(3, config.Profile.Level)
		tAssert.Nil(config.Profile.Alias)
		tAssert.Equal([]bool{true, false}, config.Flags)
		tAssert.Equal(map[string]interface{}{"retries": int64(2)}, config.Meta)
	})

	It("unmarshals output records into maps", func() {
		input := `[output = data]
{
  age: 27,
  name: "Ada"
}`

		target := map[string]any{}
		err := Unmarshal(input, &target)
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{
			"age":  int64(27),
			"name": "Ada",
		}, target)
	})

	It("rejects non-pointer targets", func() {
		err := Unmarshal(`[output = data] { value: 1; }`, map[string]any{})
		tAssert.Error(err)
	})

	It("uses parse input values during unmarshal", func() {
		var target map[string]any
		err := UnmarshalWithInput(`|===|
schema Runtime: { env: string; };
|===|
[output = data, parse = Runtime]
{
  env: env;
}`, map[string]any{
			"env": "prod",
		}, &target)
		tAssert.NoError(err)
		tAssert.Equal(map[string]any{"env": "prod"}, target)
	})
})
