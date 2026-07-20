package code_actions_test

import (
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Imports and relationships between files code actions", func() {
	It("moves a misplaced import to the top of the script block", func() {
		source := `|===|
string local = 'local';
from './shared.mace' import Shared;
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		expected := `|===|
from './shared.mace' import Shared;
string local = 'local';
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		files := map[string]string{
			"shared.mace": `[output = 'data']
{
  Shared: 'shared',
}`,
		}

		newCodeActionFixture(source, files).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.import.not-at-top",
			title:          "Move import to top of script block",
			result:         expected,
		})
	})

	It("appends the Mace extension to a resolvable import path", func() {
		source := `|===|
from './shared' import Shared;
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		expected := `|===|
from './shared.mace' import Shared;
string result = Shared;
|===|
[output = 'data']
{
  result: result,
}`
		files := map[string]string{
			"shared.mace": `[output = 'data']
{
  Shared: 'shared',
}`,
		}

		newCodeActionFixture(source, files).requirePreferredQuickFix(expectedQuickFix{
			diagnosticCode: "mace.import.path-not-mace",
			title:          "Append `.mace` to import path",
			result:         expected,
		})
	})
})
