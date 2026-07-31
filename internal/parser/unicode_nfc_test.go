package parser

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/louiss0/mace/internal/parser/ast"
)

var _ = Describe("Unicode NFC parser names", func() {
	It("normalizes semantic names while retaining raw token ranges", func() {
		file, err := parseFileInput("|===|\nstring café = \"ok\";\n|===|\n[output = 'data'] { café, }\n")
		if !tAssert.NoError(err) {
			return
		}
		declaration := file.Script.Items[0].(ast.VariableDeclaration)
		tAssert.Equal("café", declaration.Name)
		tAssert.Equal("café", declaration.NameToken.RawLexeme)
		rangeValue := ast.TokenRange(declaration.NameToken)
		tAssert.Equal(8, rangeValue.Start.Column)
		tAssert.Equal(13, rangeValue.End.Column)
	})
})
