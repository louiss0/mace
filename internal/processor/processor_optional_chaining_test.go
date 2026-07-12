package processor

import . "github.com/onsi/ginkgo/v2"

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
		result, err := New().ProcessFile("../../fixtures/processor/optional_chaining/nullable_user.mace")

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

	It("propagates absence through nested optional properties", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `User user = { records: {}, profile: {}, };`, `
  city?: user?.profile?.address?.city,
`))

		tAssert.NoError(err)
		tAssert.NotContains(result.Output, "city")
	})

	It("resolves a fully present nested optional-property chain", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `User user = {
  records: {},
  profile: { address: { city: "Paris", }, },
};`, `
  city?: user?.profile?.address?.city,
`))

		tAssert.NoError(err)
		tAssert.Equal("Paris", result.Output["city"].String)
	})
})

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
