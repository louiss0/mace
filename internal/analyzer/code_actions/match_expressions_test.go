package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Match expression code actions", func() {
	It("adds a missing comma after a match arm", func() {
		source := `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text'
  int => 'number',
};
|===|
[output = 'data']
{
  selected: selected,
}`
		expected := `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text',
  int => 'number',
};
|===|
[output = 'data']
{
  selected: selected,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.syntax.missing-match-arm-comma",
			title:          "Add trailing comma to match arm",
			result:         expected,
		})
	})

	It("removes a duplicate match arm", func() {
		source := `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text',
  string => 'again',
  int => 'number',
};
|===|
[output = 'data']
{
  selected: selected,
}`
		expected := `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text',
  int => 'number',
};
|===|
[output = 'data']
{
  selected: selected,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.match.duplicate-pattern",
			title:          "Remove duplicate match arm",
			result:         expected,
		})
	})

	It("replaces a variant literal pattern with its resolved type pattern", func() {
		source := `|===|
variant[string, int] value = 1;
string selected = match (value) {
  'text' => 'text',
  int => 'number',
};
|===|
[output = 'data']
{
  selected: selected,
}`
		expected := `|===|
variant[string, int] value = 1;
string selected = match (value) {
  string => 'text',
  int => 'number',
};
|===|
[output = 'data']
{
  selected: selected,
}`

		newCodeActionFixture(source, nil).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.match.variant-literal-pattern",
			title:          "Replace literal pattern with type pattern",
			result:         expected,
		})
	})

	It("replaces a choice type pattern with a valid literal pattern", func() {
		source := `|===|
choice['on', 'off'] value = 'on';
int selected = match (value) {
  string => 1,
  'off' => 0,
};
|===|
[output = 'data']
{
  selected: selected,
}`
		expected := `|===|
choice['on', 'off'] value = 'on';
int selected = match (value) {
  'on' => 1,
  'off' => 0,
};
|===|
[output = 'data']
{
  selected: selected,
}`

		newCodeActionFixture(source, nil).requireQuickFix(expectedQuickFix{
			diagnosticCode: "mace.match.choice-type-pattern",
			title:          "Replace type pattern with 'on'",
			result:         expected,
		})
	})

	DescribeTable("satisfies every remaining match contract",
		testCodeActionContract,
		Entry("adds one arm", quickFix("Add missing match arm for `Type`", "mace.match.not-exhaustive", "|===|\nvariant[string, int] value = 1; string result = match (value) { string => 'text', };\n|===|\n[output = 'data'] { result: result, }", "int => ''")),
		Entry("adds all arms", quickFix("Add all missing match arms", "mace.match.not-exhaustive", "|===|\nvariant[string, int, boolean] value = 1; string result = match (value) { string => 'text', };\n|===|\n[output = 'data'] { result: result, }", "int => ''", "boolean => ''")),
		Entry("uses duplicate for missing coverage", preferredQuickFix("Replace duplicate pattern with missing member", "mace.match.duplicate-pattern", "|===|\nvariant[string, int] value = 1; string result = match (value) { string => 'a', string => 'b', };\n|===|\n[output = 'data'] { result: result, }", "int => 'b'")),
		Entry("splits overlap", rewrite("Split overlapping pattern into disjoint arms", "mace.match.overlapping-pattern", "|===|\nalias Text: choice['a', 'b']; variant[string, int] value = 'a'; string result = match (value) { string => 'text', Text => 'choice', int => 'number', };\n|===|\n[output = 'data'] { result: result, }", "'a' =>", "'b' =>")),
		Entry("selects a valid member", quickFix("Replace pattern with valid domain member", "mace.match.pattern-outside-domain", "|===|\nchoice['on', 'off'] value = 'on'; int result = match (value) { 'auto' => 1, 'off' => 0, };\n|===|\n[output = 'data'] { result: result, }", "'on' => 1")),
		Entry("removes an extra pattern", preferredQuickFix("Remove extra match pattern", "mace.match.pattern-outside-domain", "|===|\nchoice['on', 'off'] value = 'on'; int result = match (value) { 'on' => 1, 'off' => 0, 'auto' => 2, };\n|===|\n[output = 'data'] { result: result, }", "'off' => 0")),
		Entry("converts source to variant", rewrite("Convert source declaration to variant", "mace.match.concrete-input", "|===|\nstring value = 'text'; string result = match (value) { string => 'text', int => 'number', };\n|===|\n[output = 'data'] { result: result, }", "variant[string, int] value")),
		Entry("converts source to choice", rewrite("Convert source declaration to choice", "mace.match.concrete-input", "|===|\nstring value = 'on'; int result = match (value) { 'on' => 1, 'off' => 0, };\n|===|\n[output = 'data'] { result: result, }", "choice['on', 'off'] value")),
		Entry("removes unnecessary match", rewrite("Replace unnecessary match with direct expression", "mace.match.single-member-domain", "|===|\nchoice['on'] value = 'on'; int result = match (value) { 'on' => 1, };\n|===|\n[output = 'data'] { result: result, }", "int result = 1;")),
		Entry("widens result variant", quickFix("Expand receiving type for match result variant", "mace.type.match-result-mismatch", "|===|\nchoice['on', 'off'] value = 'on'; string result = match (value) { 'on' => 'yes', 'off' => 0, };\n|===|\n[output = 'data'] { result: result, }", "variant[string, int] result")),
		Entry("unifies arm results", quickFix("Unify match arm result types", "mace.type.match-result-mismatch", "|===|\nchoice['on', 'off'] value = 'on'; string result = match (value) { 'on' => 'yes', 'off' => 0, };\n|===|\n[output = 'data'] { result: result, }", "'off' => ''")),
		Entry("updates after variant change", rewrite("Update match after variant alias change", "mace.match.domain-changed", "|===|\nalias Value: variant[string, int, boolean]; Value value = true; string result = match (value) { string => 's', int => 'i', };\n|===|\n[output = 'data'] { result: result, }", "boolean =>")),
		Entry("updates after choice change", rewrite("Update match after choice alias change", "mace.match.domain-changed", "|===|\nalias Mode: choice['on', 'off', 'auto']; Mode mode = 'on'; int result = match (mode) { 'on' => 1, 'off' => 0, };\n|===|\n[output = 'data'] { result: result, }", "'auto' =>")),
	)
})
