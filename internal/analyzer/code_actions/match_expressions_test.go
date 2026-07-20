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
})
