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

	It("attaches optional field access errors to the access operator", func() {
		_, err := New().Process(`|===|
schema Address: {
  city: string,
};
schema User: {
  address?: Address,
};
User user = { address: { city: 'Paris', }, };
string city = user.address.city;
|===|
[output = 'data'] { city: city, }`)
		tAssert.Error(err)

		var diagnostic DiagnosticError
		if tAssert.ErrorAs(err, &diagnostic) {
			tAssert.Equal(CodeOptionalFieldAccess, diagnostic.Code)
			tAssert.Equal(9, diagnostic.Range.Start.Line)
			tAssert.Equal(27, diagnostic.Range.Start.Column)
			tAssert.Equal(9, diagnostic.Range.End.Line)
			tAssert.Equal(28, diagnostic.Range.End.Column)
		}
	})

	It("requires optional chaining when accessing an optional property", func() {
		_, err := New().Process(optionalChainDocument(userSchemas, `User user = { records: {}, };`, `
  profile: user.profile,
`))

		requireOptionalFieldAccessError(err)
	})

	It("uses the false conditional branch for a boolean condition", func() {
		result, err := New().Process(optionalChainDocumentWithOutputSchema(
			userSchemas,
			`User user = { records: {}, };`,
			`schema Result: { records: record<string>, };`,
			`
  records: user ? user.records : {},
`,
		))

		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "records"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{}})
	})

	It("allows a record variable after a truthiness check", func() {
		result, err := New().Process(optionalChainDocumentWithOutputSchema(
			userSchemas,
			`User user = {
  records: { primary: "active", },
};`,
			`schema Result: { records: record<string>, };`,
			`
  records: user ? user.records : {},
`,
		))

		tAssert.NoError(err)
		assertExpectedValue(requireOutputValue(result, "records"), expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"primary": {kind: ValueString, string: "active"},
		}})
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
string value = packages.codefixer.cn_efs;
|===|
[output = 'data'] { value, }`)

		tAssert.ErrorContains(err, `member "cn_efs" cannot be accessed because its target is not a record`)

		var diagnostic DiagnosticError
		if tAssert.ErrorAs(err, &diagnostic) {
			tAssert.Equal(3, diagnostic.Range.Start.Line)
			tAssert.Equal(34, diagnostic.Range.Start.Column)
			tAssert.Equal(3, diagnostic.Range.End.Line)
			tAssert.Equal(41, diagnostic.Range.End.Column)
		}
	})

	It("requires optional chaining at each nested record level", func() {
		_, err := New().Process(`|===|
record<record<string>> packages = {};
string fallback = "missing";
|===|
[output = 'data']
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
[output = 'data']
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
[output = 'data']
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

func optionalChainDocument(schemas string, declaration string, fields string) string {
	return schemas + declaration + "\n|===|\n[output = 'data']\n{\n" + fields + "\n}"
}

func optionalChainDocumentWithOutputSchema(
	schemas string,
	declaration string,
	outputSchema string,
	fields string,
) string {
	return schemas + declaration + "\n" + outputSchema +
		"\n|===|\n[output = 'data', schema = Result]\n{\n" + fields + "\n}"
}

func requireOptionalFieldAccessError(err error) {
	requireDiagnosticCode(err, CodeOptionalFieldAccess)
}

func requireAbsentValueError(err error) {
	requireDiagnosticCode(err, CodeAbsentValueNotCoalesced)
}

func requireDiagnosticCode(err error, code ErrorCode) {
	var diagnostic DiagnosticError
	if !tAssert.ErrorAs(err, &diagnostic) {
		return
	}

	tAssert.Equal(code, diagnostic.Code, diagnostic.Message)
}
