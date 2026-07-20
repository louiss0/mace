package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Optional member access and record depth code actions", func() {
	It("replaces plain access after an optional field with optional access", func() {
		source := `|===|
schema Address: { city: string, };
schema User: { address?: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address.city;
|===|
[output = 'data']
{
  city: city,
}`
		expected := `|===|
schema Address: { city: string, };
schema User: { address?: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address?.city;
|===|
[output = 'data']
{
  city: city,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.optional-field-access",
			title:          "Replace `.` with `?.`",
			result:         expected,
		})
	})

	It("adds a fallback derived from the final optional member type", func() {
		source := `|===|
schema Address: { city?: string, };
schema User: { address: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address?.city;
|===|
[output = 'data']
{
  city: city,
}`
		expected := `|===|
schema Address: { city?: string, };
schema User: { address: Address, };
User user = { address: { city: 'Paris', }, };
string city = user.address?.city ?? '';
|===|
[output = 'data']
{
  city: city,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.type.absent-value-not-coalesced",
			title:          "Add `??` fallback",
			result:         expected,
		})
	})

	DescribeTable("satisfies every remaining optional-access contract",
		testCodeActionContract,
		Entry("fixes every optional step", preferredQuickFix("Make every optional path step use `?.`", "mace.type.optional-field-access", "[output = 'data'] { code: packages.codefixer.cn_efs, }", "packages?.codefixer?.cn_efs")),
		Entry("adds a string fallback", quickFix("Add typed string fallback", "mace.type.absent-value-not-coalesced", "[output = 'data'] { code: packages?.code, }", "?? ''")),
		Entry("adds a numeric fallback", quickFix("Add typed numeric fallback", "mace.type.absent-value-not-coalesced", "[output = 'data'] { count: metrics?.count, }", "?? 0")),
		Entry("adds a choice fallback", quickFix("Add choice fallback", "mace.type.absent-value-not-coalesced", "|===|\nalias Mode: choice['dev', 'prod'];\n|===|\n[output = 'data'] { mode: config?.mode, }", "?? 'dev'")),
		Entry("uses a variable fallback", quickFix("Use existing variable as fallback", "mace.type.absent-value-not-coalesced", "|===|\nstring fallback = 'unknown';\n|===|\n[output = 'data'] { city: user?.city, }", "?? fallback")),
		Entry("requires a field", rewrite("Mark accessed field required", "mace.type.optional-field-access", "|===|\nschema User: { name?: string, };\n|===|\n[output = 'schema'] { User: User, }", "name: string")),
		Entry("shortens an overdeep path", quickFix("Shorten member-access path", "mace.type.record-depth-exceeded", "[output = 'data'] { value: records.first.second.third, }", "records.first.second")),
		Entry("increases map depth", rewrite("Increase record-map nesting depth", "mace.type.record-depth-exceeded", "|===|\nrecord<string> records = {};\n|===|\n[output = 'data'] { value: records.first.second, }", "record<record<string>>")),
		Entry("nests a closed field", rewrite("Change member type to nested record", "mace.type.record-depth-exceeded", "|===|\nschema Config: { packages: string, };\n|===|\n[output = 'schema'] { Config: Config, }", "packages: { codefixer: string, }")),
		Entry("matches before deep access", rewrite("Match variant before deeper access", "mace.type.variant-record-depth", "[output = 'data'] { value: config.deep.value, }", "match (config)")),
		Entry("normalizes variant depth", rewrite("Normalize variant members to common record depth", "mace.type.variant-record-depth", "|===|\nalias Config: variant[{ value: string, }, { deep: { value: string, }, }];\n|===|\n[output = 'schema'] { Config: Config, }", "deep: { value: string, }")),
		Entry("stops an optional chain", quickFix("Stop optional chain at shallowest valid member", "mace.type.variant-record-depth", "[output = 'data'] { value: config?.deep?.value, }", "config?.deep")),
	)
})
