package lexer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIdentifierNFCNormalizationPreservesSourceSpelling(t *testing.T) {
	tokens, err := collectTokens("cafe\u0301")
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, TokenIdentifier, tokens[0].Type)
	assert.Equal(t, "café", tokens[0].Lexeme)
	assert.Equal(t, "cafe\u0301", tokens[0].RawLexeme)
	assert.Equal(t, len("cafe\u0301"), tokens[0].SourceLength())
}
