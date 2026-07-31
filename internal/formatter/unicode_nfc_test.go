package formatter

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser"
)

var _ = Describe("Unicode NFC formatting", func() {
	It("rewrites identifier spellings but preserves strings, paths, comments, and documentation", func() {
		source := "|===| // café comment\nfrom 'café.mace' import café;\nstring café = \"café\" /# café description;\n|===|\n[output = 'data'] { value: café, }"
		lexerInstance := lexer.New(source)
		tokens := []lexer.Token{}
		for {
			token, err := lexerInstance.NextToken()
			if !tAssert.NoError(err) {
				return
			}
			tokens = append(tokens, token)
			if token.Type == lexer.TokenEOF {
				break
			}
		}
		file, err := parser.New(tokens).ParseFile()
		if !tAssert.NoError(err) {
			return
		}
		formatted, err := FormatFile(file)
		if !tAssert.NoError(err) {
			return
		}
		tAssert.Contains(formatted, "café")
		tAssert.Contains(formatted, "\"café\"")
		tAssert.Contains(formatted, "'café.mace'")
		tAssert.Contains(formatted, "café description")

		secondLexer := lexer.New(formatted)
		secondTokens := []lexer.Token{}
		for {
			token, err := secondLexer.NextToken()
			if !tAssert.NoError(err) {
				return
			}
			secondTokens = append(secondTokens, token)
			if token.Type == lexer.TokenEOF {
				break
			}
		}
		secondFile, err := parser.New(secondTokens).ParseFile()
		if !tAssert.NoError(err) {
			return
		}
		secondFormatted, err := FormatFile(secondFile)
		if !tAssert.NoError(err) {
			return
		}
		tAssert.Equal(formatted, secondFormatted)
	})
})
