package parser

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/louiss0/mace/internal/parser/ast"
)

var _ = Describe("canonical grammar conformance", func() {
	It("requires semicolons after imports", func() {
		_, err := parseFileInput("|===| from 'source.mace' import Value |===| {}")
		tAssert.ErrorContains(err, "expected ';' after import declaration")
	})

	It("requires semicolons after schema declarations", func() {
		_, err := parseFileInput("|===| schema Empty: {} |===| {}")
		tAssert.ErrorContains(err, "expected ';' after schema declaration")
	})

	It("accepts trailing commas in variant and choice types", func() {
		file, err := parseFileInput("|===| alias Value: variant[string, int,]; alias Mode: choice['dev', 'prod',]; |===| {}")
		if !tAssert.NoError(err) {
			return
		}
		tAssert.IsType(ast.VariantType{}, file.Script.Items[0].(ast.TypeDeclaration).Type)
		tAssert.IsType(ast.ChoiceType{}, file.Script.Items[1].(ast.TypeDeclaration).Type)
	})

})
