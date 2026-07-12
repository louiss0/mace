package processor

import (
	"path/filepath"

	"github.com/louiss0/mace/internal/parser/ast"

	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("optional chaining", func() {
	const userSchemas = `|===|
schema Address: {
  city: string,
};
schema Profile: {
  address?: Address,
};
schema User: {
  records: record<string>,
  profile?: Profile,
};
`

	It("requires a truthiness check before accessing a nullable record variable", func() {
		_, err := New().Process(optionalChainDocument(userSchemas, `nullable User user = {
  records: { primary: "active", },
};`, `
  records: user.records,
`))

		requireOptionalFieldAccessError(err)
	})

	It("rejects optional chaining on a nullable record variable", func() {
		_, err := New().Process(optionalChainDocument(userSchemas, `nullable User user = {
  records: { primary: "active", },
};`, `
  records?: user?.records,
`))

		requireOptionalFieldAccessError(err)
	})

	It("requires optional chaining when accessing an optional property", func() {
		_, err := New().Process(optionalChainDocument(userSchemas, `User user = { records: {}, };`, `
  profile: user.profile,
`))

		requireOptionalFieldAccessError(err)
	})

	It("uses the fallback when a nullable record variable is null", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `nullable User user = null;`, `
  records: user ? user.records : {},
`))

		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "records"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{}})
	})

	It("allows a nullable record variable after a truthiness check", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `nullable User user = {
  records: { primary: "active", },
};`, `
  records: user ? user.records : {},
`))

		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "records"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"primary": {kind: ValueString, string: "active"},
		}})
	})

	It("processes the nullable user fixture", func() {
		fixture, err := filepath.Abs("../../fixtures/processor/optional_chaining/nullable_user.mace")
		tAssert.NoError(err)
		result, err := New().ProcessFile(fixture)

		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "fallback_records"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{}})
		assertExpectedValue(requireOutputValue(result, "records"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"primary": {kind: ValueString, string: "active"},
		}})
		tAssert.Equal("Paris", requireOutputValue(result, "city").String)
	})

	It("requires every optional property in a nested chain to be guarded", func() {
		_, err := New().Process(optionalChainDocument(userSchemas, `User user = { records: {}, profile: {}, };`, `
  city?: user?.profile.address?.city,
`))

		requireOptionalFieldAccessError(err)
	})

	It("resolves absence through a coalescing fallback", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `User user = { records: {}, profile: {}, };
string fallback = "";`, `
  city: user?.profile?.address?.city ?? fallback,
`))

		tAssert.NoError(err)
		tAssert.Equal("", requireOutputValue(result, "city").String)
	})

	It("allows a literal coalescing fallback", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `User user = { records: {}, profile: {}, };`, `
  city: user?.profile?.address?.city ?? "unknown",
`))

		tAssert.NoError(err)
		tAssert.Equal("unknown", requireOutputValue(result, "city").String)
	})

	It("processes the null coalescing fixture", func() {
		fixture, err := filepath.Abs("../../fixtures/processor/null_coalescing/optional_profile.mace")
		tAssert.NoError(err)
		result, err := New().ProcessFile(fixture)

		tAssert.NoError(err)
		tAssert.Equal("unknown", requireOutputValue(result, "city").String)
		tAssert.Equal("unknown", requireOutputValue(result, "literal_city").String)
	})

	It("resolves a fully present nested optional-property chain", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `User user = {
  records: {},
  profile: { address: { city: "Paris", }, },
};
string fallback = "";`, `
  city: user?.profile?.address?.city ?? fallback,
`))

		tAssert.NoError(err)
		tAssert.Equal("Paris", result.Output["city"].String)
	})

	It("requires records to declare every traversed nesting level", func() {
		_, err := New().Process(`|===|
record<string> packages = {};
|===|
[output = data]
{ value: packages.codefixer.cn_efs, }`)

		tAssert.ErrorContains(err, `member "cn_efs" cannot be accessed because its target is not a record`)
	})

	It("requires optional chaining at each nested record level", func() {
		_, err := New().Process(`|===|
record<record<string>> packages = {};
string fallback = "missing";
|===|
[output = data]
{ value: packages?.codefixer.cn_efs ?? fallback, }`)

		requireOptionalFieldAccessError(err)
	})

	It("resolves optional access at every nested record level", func() {
		result, err := New().Process(`|===|
record<record<string>> packages = {
  codefixer: { cn_efs: "enabled", },
};
string fallback = "missing";
|===|
[output = data]
{
  present: packages?.codefixer?.cn_efs ?? fallback,
  missing: packages?.codefixer?.missing ?? fallback,
}`)

		tAssert.NoError(err)
		tAssert.Equal("enabled", requireOutputValue(result, "present").String)
		tAssert.Equal("missing", requireOutputValue(result, "missing").String)
	})

	It("rejects optional chaining past the fixture's record depth", func() {
		fixture, err := filepath.Abs("../../fixtures/processor/optional_chaining/nested_records.mace")
		tAssert.NoError(err)
		_, err = New().ProcessFile(fixture)

		tAssert.Error(err)
	})

	DescribeTable("rejects optional chaining past terminal record value types",
		func(valueType string) {
			_, err := New().Process(`|===|
record<` + valueType + `> packages = {};
string fallback = "missing";
|===|
[output = data]
{ value: packages?.codefixer?.cn_efs ?? fallback, }`)

			tAssert.ErrorContains(err, "cannot be accessed because its target is not a record")
		},
		Entry("string", "string"),
		Entry("int", "int"),
		Entry("float", "float"),
		Entry("hex int", "hex_int"),
		Entry("hex float", "hex_float"),
		Entry("boolean", "boolean"),
		Entry("array", "array<string>"),
		Entry("choice", `choice["enabled", "disabled"]`),
		Entry("terminal variant", "variant[string, int]"),
	)

	DescribeTable("allows optional chaining through the common variant record depth",
		func(depth int) {
			variables := newVariableRegistry()
			symbols := newSymbolTable()
			types := newTypeRegistry()
			schemas := newSchemaRegistry()
			declaredType := ast.VariantType{Members: []ast.TypeReference{
				nestedRecordMapType(depth),
				nestedRecordMapType(depth + 1),
			}}
			resolvedType, err := resolveValueType(declaredType, symbols, types, schemas, nil)
			tAssert.NoError(err)
			variables.Add("packages", resolvedType)

			resolvedPath, err := inferExpressionType(optionalRecordPath(depth), variables, symbols, types, schemas, nil)
			tAssert.NoError(err)
			tAssert.Equal(presencePossiblyAbsent, resolvedPath.presence)

			_, err = inferExpressionType(optionalRecordPath(depth+1), variables, symbols, types, schemas, nil)
			tAssert.ErrorContains(err, "cannot be accessed because its target is not a record")

			_, err = New().Process(`|===|
type Packages: variant[` + nestedRecordMapTypeText(depth) + `, ` + nestedRecordMapTypeText(depth+1) + `];
nullable Packages packages = null;
string fallback = "missing";
|===|
[output = data]
{ value: packages ? ` + optionalRecordPathText(depth+1) + ` ?? fallback : fallback, }`)
			tAssert.ErrorContains(err, "cannot be accessed because its target is not a record")
		},
		Entry("one level", 1),
		Entry("two levels", 2),
		Entry("three levels", 3),
		Entry("four levels", 4),
		Entry("five levels", 5),
		Entry("six levels", 6),
		Entry("seven levels", 7),
		Entry("eight levels", 8),
		Entry("nine levels", 9),
		Entry("ten levels", 10),
	)

	It("selects one variant member for a record map literal", func() {
		_, err := New().Process(`|===|
type Packages: variant[record<string>, record<record<string>>];
Packages packages = { codefixer: "enabled", };
|===|
[output = data]
{}`)

		tAssert.NoError(err)
	})
})

func nestedRecordMapType(depth int) ast.TypeReference {
	valueType := ast.TypeReference(ast.PrimitiveType{Name: "string"})
	for range depth {
		valueType = ast.RecordMapType{Value: valueType}
	}
	return valueType
}

func optionalRecordPath(depth int) ast.Expression {
	var expression ast.Expression = ast.Identifier{Name: "packages"}
	for range depth {
		expression = ast.MemberAccess{Target: expression, Name: "value", Optional: true}
	}
	return expression
}

func nestedRecordMapTypeText(depth int) string {
	valueType := "string"
	for range depth {
		valueType = "record<" + valueType + ">"
	}
	return valueType
}

func optionalRecordPathText(depth int) string {
	path := "packages"
	for range depth {
		path += "?.value"
	}
	return path
}

func optionalChainDocument(schemas string, declaration string, fields string) string {
	return schemas + declaration + "\n|===|\n[output = data]\n{\n" + fields + "\n}"
}

func requireOptionalFieldAccessError(err error) {
	var diagnostic DiagnosticError
	if !tAssert.ErrorAs(err, &diagnostic) {
		return
	}

	tAssert.Equal(CodeOptionalFieldAccess, diagnostic.Code, diagnostic.Message)
}
