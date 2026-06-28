package codec

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	burnttoml "github.com/BurntSushi/toml"
	"github.com/louiss0/mace/internal/processor"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type coverageStruct struct {
	Name       string
	Skip       string `json:"-"`
	Empty      string `json:",omitempty"`
	Hidden     string `json:"hidden"`
	unexported string
}

func TestCodecCoverageCheckBranches(t *testing.T) {
	require.Equal(t, CheckIssue{Path: "x"}, positionedCheckIssue(nil, CheckIssue{Path: "x"}))

	root := t.TempDir()
	require.Error(t, func() error { _, err := CheckFile(filepath.Join(root, "missing.json")); return err }())
	for name, body := range map[string]string{"a.json": "{\"x\":1}", "b.yaml": "x: 1\n", "c.toml": "x = 1\n", "d": "{\"x\":1}"} {
		p := filepath.Join(root, name)
		require.NoError(t, os.WriteFile(p, []byte(body), 0600))
		_, err := CheckFile(p)
		require.NoError(t, err)
	}
	p := filepath.Join(root, "bad.ext")
	require.NoError(t, os.WriteFile(p, []byte("nope"), 0600))
	_, err := CheckFile(p)
	require.Error(t, err)

	for _, s := range []string{"", "   ", "true", "[]"} {
		_ = CheckJSON(s)
	}
	_, err = decodeJSONAfterScan("{")
	require.Error(t, err)
	_, err = checkContents("bad", []byte(""))
	require.Error(t, err)
	_ = CheckJSON("{\"a\":1} trailing")
	_ = CheckJSON("{\"a\":1,\"a\":null,\"bad-key\":[null]}")
	scanner := newJSONCheckScanner("}", &CheckReport{})
	require.Error(t, scanner.scanValue("$"))
	require.Error(t, scanner.scanDelimiter(json.Delim(']'), "$"))
	_, err = jsonObjectKey(1)
	require.Error(t, err)
	scanner = newJSONCheckScanner("[1", &CheckReport{})
	require.Error(t, scanner.scanRoot())
	scanner = newJSONCheckScanner("", &CheckReport{})
	require.Error(t, scanner.scanObject("$"))
	scanner = newJSONCheckScanner("{,", &CheckReport{})
	_, _ = scanner.decoder.Token()
	require.Error(t, scanner.scanObject("$"))
	scanner = newJSONCheckScanner("[", &CheckReport{})
	require.Error(t, scanner.scanArray("$"))
	scanner = newJSONCheckScanner("{\"a\":", &CheckReport{})
	require.Error(t, scanner.scanRoot())

	_ = CheckYAML("a: &a\n  b: 1\nmerge:\n  <<: *a\n---\n# c\n? [x]\n: y\nbad: 2001-01-01\nblock: |\n  hi\nfold: >\n  hi\nnullv: null\n")
	_ = CheckYAML("bad: [")
	report := newCheckReport()
	checkYAMLNode(nil, "$", &report, map[*yaml.Node]struct{}{}, new(bool))
	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	require.True(t, isSupportedYAMLMergeValue(doc))
	require.False(t, isSupportedYAMLMergeValue(&yaml.Node{Kind: yaml.DocumentNode}))
	require.False(t, isSupportedYAMLMergeValue(&yaml.Node{Kind: yaml.SequenceNode}))
	require.True(t, isSupportedYAMLMergeValue(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!merge"}))
	require.False(t, isSupportedYAMLMergeValue(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}))
	checkYAMLMergeValue(nil, "$", &report)
	for _, n := range []*yaml.Node{nil, {Kind: yaml.DocumentNode}, {Kind: yaml.MappingNode}, {Kind: yaml.SequenceNode}, {Kind: yaml.ScalarNode}, {Kind: yaml.AliasNode}, {Kind: 99}} {
		_ = yamlNodeKindName(n)
	}
	require.Equal(t, ">", yamlScalarStyleName(yaml.FoldedStyle))
	require.Equal(t, "plain", yamlScalarStyleName(0))

	_ = CheckTOML("=")
	_ = yamlSyntaxIssue(errors.New("line 7: column 8: nope"))
	_ = yamlSyntaxIssue(errors.New("line x:"))
	var pe burnttoml.ParseError
	_ = tomlSyntaxIssue(pe)
	_ = objectCheckPath("", "abc")
	require.False(t, isMaceIdentifier("1bad"))
	require.False(t, isMaceIdentifier("a-b"))
	_ = jsonSyntaxIssue("{\"x\":1}", &json.UnmarshalTypeError{Value: "number", Type: reflect.TypeOf(""), Offset: 2})
}

func TestCodecCoveragePackageBranches(t *testing.T) {
	_, err := ParseWithInput("bad", nil)
	require.Error(t, err)
	_, err = ParseFileWithInput(filepath.Join(t.TempDir(), "missing.mace"), nil)
	require.Error(t, err)
	require.Error(t, UnmarshalWithInput("bad", nil, map[string]any{}))
	require.Error(t, UnmarshalWithInput("[output = data]\nstring data = \"x\";", nil, (*map[string]any)(nil)))
	require.Error(t, func() error { _, e := MarshalOutput("x"); return e }())
	_, err = ImportJSONFile(filepath.Join(t.TempDir(), "missing.json"))
	require.Error(t, err)
	_, err = ImportJSONSchema("{")
	require.Error(t, err)
	_, err = parseImportedJSON("{} {}")
	require.Error(t, err)
	_, err = importDocument(make(chan int))
	require.Error(t, err)
	_, err = marshalImportedOutput(map[string]any{"bad": make(chan int)})
	require.Error(t, err)
	_, err = importJSONSchemaDocument([]any{})
	require.Error(t, err)

	require.Nil(t, valueToAny(processor.Value{Kind: processor.ValueUnknown}))
	_, err = formatProcessorScalar(processor.Value{Kind: processor.ValueUnknown})
	require.Error(t, err)
	require.NotNil(t, valueToAny(processor.Value{Kind: processor.ValueHexInt, Int: 255}))
	_ = schemaTypeFromProcessor(processor.SchemaType{Kind: processor.SchemaTypeArray, Element: &processor.SchemaType{Kind: processor.SchemaTypePrimitive, Name: "string"}, Fields: map[processor.SchemaField]processor.SchemaType{{Name: "x"}: {Kind: processor.SchemaTypePrimitive, Name: "int"}}, Members: []processor.SchemaType{{Kind: processor.SchemaTypePrimitive, Name: "bool"}}})

	_, err = normalizeImportedValue(reflect.ValueOf(map[int]string{1: "x"}))
	require.Error(t, err)
	_, err = normalizeImportedValue(reflect.ValueOf([]any{make(chan int)}))
	require.Error(t, err)
	_, err = normalizeImportedValue(reflect.ValueOf(map[string]any{"x": make(chan int)}))
	require.Error(t, err)
	_, err = normalizeImportedValue(reflect.ValueOf(make(chan int)))
	require.Error(t, err)
	_, err = importedMapKey(reflect.Value{})
	require.Error(t, err)
	var sp *string
	_, err = importedMapKey(reflect.ValueOf(&sp).Elem())
	require.Error(t, err)
	_, err = importedJSONNumber(json.Number("nope"))
	require.Error(t, err)

	_, err = Marshal(nil)
	require.Error(t, err)
	_, err = Marshal((*string)(nil))
	require.Error(t, err)
	_, err = Marshal(map[int]string{1: "x"})
	require.Error(t, err)
	_, err = Marshal([]any{make(chan int)})
	require.Error(t, err)
	_, err = Marshal(map[string]any{"x": make(chan int)})
	require.Error(t, err)
	out, err := Marshal(coverageStruct{Name: "Ada", Empty: "", Hidden: "yes", unexported: "no"})
	require.NoError(t, err)
	require.Contains(t, out, "name")
	require.Equal(t, "array<string>", formatSchemaType(inferredType{kind: inferredTypeArray}, 0))
	require.Equal(t, "string", formatSchemaType(inferredType{kind: 99}, 0))
	require.Equal(t, "{}", formatSchemaRecord(nil, 0))

	require.False(t, looksLikeJSONSchemaDocument(map[string]any{}))
	for _, k := range []string{"properties", "$defs", "oneOf", "anyOf", "allOf", "enum", "const"} {
		require.True(t, looksLikeJSONSchemaDocument(map[string]any{k: []any{}}))
	}
	for _, v := range []any{"object", "array", "string", "integer", "number", "boolean", "null", []any{"string"}, 1} {
		_ = looksLikeJSONSchemaDocument(map[string]any{"type": v})
	}

	_, _, err = resolveJSONSchemaDocument(map[string]any{"$schema": "://bad"}, "")
	require.Error(t, err)
	_, ok, err := resolveJSONSchemaDocument([]any{}, "")
	require.NoError(t, err)
	require.False(t, ok)
	_, ok, err = resolveJSONSchemaDocument(map[string]any{"$schema": 1}, "")
	require.NoError(t, err)
	require.False(t, ok)
	_, err = loadJSONSchemaDocument(urlMust("file:"), "file:", "")
	require.Error(t, err)
	_, err = loadJSONSchemaDocument(urlMust("ftp://x"), "ftp://x", "")
	require.Error(t, err)
	_, err = loadJSONSchemaDocument(urlMust("c:foo.json"), "c:foo.json", "")
	require.Error(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", 500) }))
	defer server.Close()
	_, err = loadJSONSchemaDocument(urlMust(server.URL), server.URL, "")
	require.Error(t, err)

	ctx := newJSONSchemaContext(map[string]any{"$defs": map[string]any{"thing": map[string]any{"type": "object", "properties": map[string]any{"x": map[string]any{"type": "string"}}}}})
	_, err = ctx.record(map[string]any{"type": "string"}, nil)
	require.Error(t, err)
	_, err = ctx.record(map[string]any{"additionalProperties": true}, nil)
	require.Error(t, err)
	_, err = ctx.record(map[string]any{"additionalProperties": map[string]any{}}, nil)
	require.Error(t, err)
	_, err = ctx.record(map[string]any{"properties": []any{}}, nil)
	require.Error(t, err)
	_, err = ctx.record(map[string]any{"properties": map[string]any{"x": 1}}, nil)
	require.Error(t, err)
	_ = jsonSchemaRequiredNames([]any{"x", 1})
	_, err = jsonSchemaReference("bad", ctx.root)
	require.Error(t, err)
	_, err = jsonSchemaReference("#/x/y", ctx.root)
	require.Error(t, err)
	_, err = jsonSchemaReference("#/$defs/missing", ctx.root)
	require.Error(t, err)

	_, _, err = ctx.propertyType(map[string]any{"const": nil}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.propertyType(map[string]any{"enum": []any{nil}}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.propertyType(map[string]any{"oneOf": []any{1}}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.propertyType(map[string]any{"anyOf": []any{}}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.propertyType(map[string]any{"allOf": []any{map[string]any{"type": "string"}}}, []string{"x"})
	require.NoError(t, err)
	_, _, err = ctx.propertyType(map[string]any{"type": []any{"null", "string"}}, []string{"x"})
	require.NoError(t, err)
	_, _, err = ctx.propertyType(map[string]any{"type": []any{}}, []string{"x"})
	require.Error(t, err)

	_, err = ctx.schemaType(map[string]any{"const": nil}, nil)
	require.Error(t, err)
	_, err = ctx.typeNameSchemaType("array", map[string]any{"items": 1}, nil)
	require.Error(t, err)
	_, err = ctx.typeNameSchemaType("", map[string]any{}, nil)
	require.Error(t, err)
	_, err = ctx.referenceType("#/$defs/thing")
	require.NoError(t, err)
	_, err = ctx.referenceType("#/$defs/thing")
	require.NoError(t, err)
	_, err = ctx.referenceType("#/bad")
	require.Error(t, err)
	_, err = ctx.enumType([]any{"a", "a"}, []string{"same"})
	require.NoError(t, err)
	_, err = ctx.enumType([]any{"b"}, []string{"same"})
	require.NoError(t, err)
	ctx.addDeclaration("X", "one")
	ctx.addDeclaration("X", "two")
	require.Equal(t, "Generated", ctx.uniqueDeclarationName(""))
	require.NotEqual(t, ctx.uniqueDeclarationName("Dup"), ctx.uniqueDeclarationName("Dup"))
	require.Equal(t, "", jsonSchemaDefinitionName("#/properties/x"))
	require.Equal(t, "Generated", jsonSchemaIdentifier("---"))
	require.Equal(t, "Value123", jsonSchemaIdentifier("123"))
	_, _, err = jsonSchemaEnumDeclaration("E", []any{true})
	require.Error(t, err)
	_, _, err = jsonSchemaEnumMembers([]any{"x", int64(1)})
	require.Error(t, err)
	_, _, err = jsonSchemaEnumMembers([]any{1.2})
	require.Error(t, err)

	_, err = validateUnionMembers([]inferredType{{kind: inferredTypePrimitive}})
	require.Error(t, err)
	_, err = validateUnionMembers([]inferredType{{kind: inferredTypeNamed, namedCategory: "enum"}})
	require.Error(t, err)
	_, err = validateVariantMembers([]inferredType{{kind: inferredTypeNamed, namedCategory: "alias"}})
	require.Error(t, err)
	_, err = validateVariantMembers([]inferredType{{kind: inferredTypeNamed, namedCategory: "bad"}})
	require.Error(t, err)
	_, err = validateVariantMembers([]inferredType{{kind: inferredTypeNamed, namedCategory: "enum", backingType: "string"}, {kind: inferredTypeNamed, namedCategory: "enum", backingType: "int"}})
	require.Error(t, err)
	_, err = validateVariantMembers([]inferredType{{kind: inferredTypeNamed, namedCategory: "enum", backingType: "string"}, {kind: inferredTypePrimitive}})
	require.Error(t, err)

	require.True(t, isEmptyValue(reflect.ValueOf([0]int{})))
	require.False(t, isEmptyValue(reflect.ValueOf(struct{}{})))
	require.False(t, isEmptyValue(reflect.ValueOf(make(chan int))))
	var nested **map[string]any
	require.NoError(t, decodeRecord(map[string]processor.Value{}, reflect.ValueOf(&nested).Elem()))
	require.Error(t, decodeRecord(map[string]processor.Value{}, reflect.ValueOf(0)))
	require.Error(t, decodeRecord(map[string]processor.Value{"x": {Kind: processor.ValueString, String: "s"}}, reflect.ValueOf(&map[int]string{}).Elem()))
	_, err = decodeValue(processor.Value{Kind: processor.ValueUnknown}, reflect.TypeOf(""))
	require.Error(t, err)
	_, err = decodeInt(-1, reflect.TypeOf(uint(0)))
	require.Error(t, err)
	_, err = decodeInt(1, reflect.TypeOf(true))
	require.Error(t, err)
	_, err = decodeFloat(1, reflect.TypeOf(1))
	require.Error(t, err)
	_, err = decodeBool(true, reflect.TypeOf(""))
	require.Error(t, err)
	_, err = decodeArray([]processor.Value{}, reflect.TypeOf([1]int{}))
	require.Error(t, err)
	_, err = decodeArray([]processor.Value{{Kind: processor.ValueString, String: "x"}}, reflect.TypeOf([]int{}))
	require.Error(t, err)
	_, err = decodeArray([]processor.Value{}, reflect.TypeOf(1))
	require.Error(t, err)
	_, err = decodeRecordValue(map[string]processor.Value{}, reflect.TypeOf(map[int]string{}))
	require.Error(t, err)
	_, err = decodeRecordValue(map[string]processor.Value{"x": {Kind: processor.ValueString, String: "x"}}, reflect.TypeOf(map[string]int{}))
	require.Error(t, err)
	_, err = decodeRecordValue(map[string]processor.Value{}, reflect.TypeOf(1))
	require.Error(t, err)
	_ = structFieldMap(reflect.TypeOf(coverageStruct{}))
	_, _ = fmt.Println, err
}

func TestCodecCoverageRemainingBranches(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "unsupported.bin"), []byte("x"), 0600))
	_, err := CheckFile(filepath.Join(root, "unsupported.bin"))
	require.Error(t, err)
	_ = CheckJSON("[1]")

	report := newCheckReport()
	comment := false
	checkYAMLNode(&yaml.Node{Kind: yaml.MappingNode, Tag: "!custom"}, "$", &report, map[*yaml.Node]struct{}{}, &comment)
	checkYAMLNode(&yaml.Node{Kind: yaml.SequenceNode, Tag: "!seq"}, "$", &report, map[*yaml.Node]struct{}{}, &comment)
	checkYAMLNode(&yaml.Node{Kind: yaml.AliasNode, Alias: &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "x"}}, "$", &report, map[*yaml.Node]struct{}{}, &comment)
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: &yaml.Node{Kind: yaml.MappingNode}}
	require.True(t, isSupportedYAMLMergeValue(alias))
	require.False(t, isSupportedYAMLMergeValue((*yaml.Node)(nil)))
	self := &yaml.Node{Kind: yaml.SequenceNode}
	self.Content = []*yaml.Node{{Kind: yaml.MappingNode}}
	require.True(t, isSupportedYAMLMergeValue(self))

	scanner := newJSONCheckScanner("{} true", &CheckReport{})
	require.Error(t, scanner.scanRoot())
	scanner = newJSONCheckScanner("{\"x\"", &CheckReport{})
	require.Error(t, scanner.scanRoot())
	scanner = newJSONCheckScanner("{\"x\":1", &CheckReport{})
	require.Error(t, scanner.scanRoot())
	scanner = newJSONCheckScanner("[1", &CheckReport{})
	require.Error(t, scanner.scanRoot())
	scanner = newJSONCheckScanner("", &CheckReport{})
	require.Error(t, scanner.scanObject("$"))
	scanner = newJSONCheckScanner("{,", &CheckReport{})
	_, _ = scanner.decoder.Token()
	require.Error(t, scanner.scanObject("$"))
	scanner = newJSONCheckScanner("[", &CheckReport{})
	require.Error(t, scanner.scanArray("$"))
	require.False(t, isMaceIdentifier(""))
	_ = CheckYAML("bad-key: 1\nseq: !seq [1]\n")

	require.Error(t, UnmarshalWithInput("[output = data]\nstring data = \"x\";", nil, nil))
	_, err = MarshalOutput(make(chan int))
	require.Error(t, err)
	_, err = parseImportedJSON("{} [")
	require.Error(t, err)
	_, err = importDocument(map[string]any{"bad": make(chan int)})
	require.Error(t, err)
	_, err = importJSONSchemaDocument(map[string]any{"type": "string"})
	require.Error(t, err)
	_, err = importJSONSchemaDocument(map[string]any{"x": make(chan int)})
	require.Error(t, err)

	require.Equal(t, "0x1.8", valueToAny(processor.Value{Kind: processor.ValueHexFloat, Float: 1.5}))
	require.Equal(t, []any{nil}, valueToAny(processor.Value{Kind: processor.ValueArray, Array: []processor.Value{{Kind: processor.ValueUnknown}}}))
	require.Equal(t, float64(1.25), valueToAny(processor.Value{Kind: processor.ValueFloat, Float: 1.25}))
	_, err = normalizeImportedValue(reflect.ValueOf([]any{nil}))
	require.NoError(t, err)
	_, err = normalizeImportedValue(reflect.ValueOf(map[string]any{"x": nil}))
	require.NoError(t, err)
	_, err = normalizeImportedValue(reflect.ValueOf(uint(1)))
	require.NoError(t, err)
	_, err = normalizeImportedValue(reflect.ValueOf(float32(1.5)))
	require.NoError(t, err)
	_, err = Marshal(coverageStruct{Name: "Ada", Hidden: "yes", Empty: "x"})
	require.NoError(t, err)
	_, err = Marshal(false)
	require.NoError(t, err)
	_, err = Marshal(uint(2))
	require.NoError(t, err)
	_, err = Marshal(float32(1.5))
	require.NoError(t, err)
	_, err = Marshal(float64(2.5))
	require.NoError(t, err)
	_, err = Marshal(struct {
		Bad chan int `json:"bad"`
	}{})
	require.Error(t, err)

	oldClient := jsonSchemaHTTPClient
	jsonSchemaHTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: errReader{}}, nil
	})}
	_, err = loadJSONSchemaDocument(urlMust("https://example.com/schema.json"), "https://example.com/schema.json", "")
	require.Error(t, err)
	jsonSchemaHTTPClient = oldClient

	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{")) }))
	defer badServer.Close()
	_, err = loadJSONSchemaDocument(urlMust(badServer.URL), badServer.URL, "")
	require.Error(t, err)
	_, err = loadJSONSchemaFile(filepath.Join(root, "missing-schema.json"))
	require.Error(t, err)
	badSchemaPath := filepath.Join(root, "bad-schema.json")
	require.NoError(t, os.WriteFile(badSchemaPath, []byte("{"), 0600))
	_, err = loadJSONSchemaFile(badSchemaPath)
	require.Error(t, err)

	ctx := newJSONSchemaContext(map[string]any{"notmap": 1, "$defs": map[string]any{"bad": map[string]any{"type": "array", "items": map[string]any{"type": "nope"}}, "plain": map[string]any{"type": "string"}}})
	_, err = jsonSchemaReference("#/notmap", ctx.root)
	require.Error(t, err)
	_, err = jsonSchemaReference("#/notmap/x", ctx.root)
	require.Error(t, err)
	_, _, err = ctx.propertyType(map[string]any{"type": []any{1}}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.propertyType(map[string]any{"type": []any{"nope"}}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.variantSchemaType([]any{map[string]any{"type": "nope"}}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.variantSchemaType([]any{map[string]any{"type": "string"}}, []string{"x"})
	require.NoError(t, err)
	_, _, err = ctx.unionSchemaType([]any{1}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.unionSchemaType([]any{map[string]any{"type": "nope"}}, []string{"x"})
	require.Error(t, err)
	_, _, err = ctx.unionSchemaType([]any{map[string]any{"type": "object"}}, []string{"x"})
	require.NoError(t, err)
	_, _, err = ctx.unionSchemaType([]any{}, []string{"x"})
	require.Error(t, err)
	_, err = ctx.schemaType(map[string]any{"const": "x"}, []string{"const"})
	require.NoError(t, err)
	_, err = ctx.schemaType(map[string]any{"enum": []any{"x"}}, []string{"enum"})
	require.NoError(t, err)
	_, err = ctx.schemaType(map[string]any{"properties": map[string]any{}}, nil)
	require.NoError(t, err)
	_, err = ctx.typeNameSchemaType("array", map[string]any{}, nil)
	require.NoError(t, err)
	_, err = ctx.typeNameSchemaType("array", map[string]any{"items": map[string]any{"type": "nope"}}, nil)
	require.Error(t, err)
	_, err = ctx.typeNameSchemaType("object", map[string]any{"type": "string"}, nil)
	require.Error(t, err)
	_, err = ctx.referenceType("#/$defs/plain")
	require.NoError(t, err)
	ctx.root["plain"] = map[string]any{"type": "string"}
	_, err = ctx.referenceType("#/plain")
	require.NoError(t, err)
	_, err = ctx.referenceType("#/$defs/bad")
	require.Error(t, err)
	_, err = ctx.enumType([]any{true}, []string{"bad"})
	require.Error(t, err)
	_, err = ctx.enumType([]any{"cached"}, []string{"cached"})
	require.NoError(t, err)
	_, err = ctx.enumType([]any{"cached"}, []string{"cached"})
	require.NoError(t, err)
	_, _, err = ctx.declarationForSchema("Bad", map[string]any{"type": "nope"}, nil)
	require.Error(t, err)
	require.Equal(t, "A", jsonSchemaIdentifier("_a"))
	_, _, err = jsonSchemaEnumMembers([]any{})
	require.Error(t, err)
	_, err = validateUnionMembers(nil)
	require.Error(t, err)
	_, err = validateVariantMembers([]inferredType{{kind: inferredTypeRecord}})
	require.Error(t, err)

	var onlyUnknown struct {
		X int `json:"x"`
	}
	require.NoError(t, decodeRecord(map[string]processor.Value{"z": {Kind: processor.ValueString, String: "x"}}, reflect.ValueOf(&onlyUnknown).Elem()))
	var m map[string]int
	require.Error(t, decodeRecord(map[string]processor.Value{"x": {Kind: processor.ValueString, String: "x"}}, reflect.ValueOf(&m).Elem()))
	var st struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	require.Error(t, decodeRecord(map[string]processor.Value{"z": {Kind: processor.ValueString, String: "x"}, "x": {Kind: processor.ValueString, String: "x"}}, reflect.ValueOf(&st).Elem()))
	_, err = decodeValue(processor.Value{Kind: processor.ValueString, String: "x"}, reflect.TypeOf((*int)(nil)))
	require.Error(t, err)
	_, err = decodeFormattedScalar(processor.Value{Kind: processor.ValueUnknown}, reflect.TypeOf(""))
	require.Error(t, err)
	_, err = decodeValue(processor.Value{Kind: processor.ValueHexInt, Int: 16}, reflect.TypeOf(""))
	require.NoError(t, err)
	_, err = decodeValue(processor.Value{Kind: processor.ValueHexFloat, Float: 1.5}, reflect.TypeOf(""))
	require.NoError(t, err)
	_, err = decodeInt(1, reflect.TypeOf(float64(0)))
	require.NoError(t, err)
	var iface any = 0
	_, err = decodeInt(1, reflect.TypeOf(&iface).Elem())
	require.NoError(t, err)
	_, err = decodeArray([]processor.Value{{Kind: processor.ValueString, String: "x"}}, reflect.TypeOf([1]int{}))
	require.Error(t, err)
	_, err = decodeRecordValue(map[string]processor.Value{"x": {Kind: processor.ValueString, String: "x"}}, reflect.TypeOf(st))
	require.Error(t, err)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("read boom") }
func (errReader) Close() error             { return nil }

func urlMust(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
