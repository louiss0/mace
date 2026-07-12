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

	It("requires optional chaining before accessing a nullable record variable", func() {
		_, err := New().Process(optionalChainDocument(userSchemas, `nullable User user = null;`, `
  records: user.records,
`))

		requireOptionalFieldAccessError(err)
	})

	It("requires optional chaining when accessing an optional property", func() {
		_, err := New().Process(optionalChainDocument(userSchemas, `User user = { records: {}, };`, `
  profile: user.profile,
`))

		requireOptionalFieldAccessError(err)
	})

	It("allows optional chaining on a nullable record variable", func() {
		result, err := New().Process(optionalChainDocument(userSchemas, `nullable User user = null;`, `
  records?: user?.records,
`))

		tAssert.NoError(err)
		tAssert.NotContains(result.Output, "records")
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
