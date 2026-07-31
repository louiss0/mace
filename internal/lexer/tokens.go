package lexer

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdentifier
	TokenSelf
	TokenDollar

	TokenFrom
	TokenImport
	TokenBind
	TokenAliasKeyword
	TokenSchema
	TokenGenDoc
	TokenSchemaDoc
	TokenArray
	TokenUnion
	TokenVariant
	TokenChoice
	TokenMatch
	TokenRecord
	TokenStringType
	TokenIntType
	TokenFloatType
	TokenHexIntType
	TokenHexFloatType
	TokenBooleanType
	TokenOutput
	TokenSchemaFile
	TokenParse
	TokenParseFile
	TokenData
	TokenNull

	TokenString
	TokenInt
	TokenFloat
	TokenHexInt
	TokenHexFloat
	TokenBoolean

	TokenAssign
	TokenArrow
	TokenSemicolon
	TokenComma
	TokenColon
	TokenQuestion
	TokenCoalesce
	TokenOptionalDot
	TokenDot

	TokenPlus
	TokenMinus
	TokenStar
	TokenSlash
	TokenPercent
	TokenDoubleStar
	TokenBang
	TokenTilde

	TokenLess
	TokenLessEqual
	TokenGreater
	TokenGreaterEqual
	TokenEqualEqual
	TokenNotEqual

	TokenAmpersand
	TokenCaret
	TokenPipe
	TokenAndAnd
	TokenOrOr

	TokenShiftLeft
	TokenShiftRight
	TokenShiftRightUnsigned

	TokenLParen
	TokenRParen
	TokenLBrace
	TokenRBrace
	TokenLBracket
	TokenRBracket

	TokenScriptDelimiter
	TokenInlineDescription
)

type Token struct {
	Type TokenType
	// Lexeme is the semantic spelling. For identifiers it is NFC-normalized.
	Lexeme string
	// RawLexeme preserves the exact source spelling for source ranges and display.
	RawLexeme string
	Line      int
	Column    int
}

// SourceLexeme returns the bytes occupied by this token in the original source.
func (token Token) SourceLexeme() string {
	if token.RawLexeme != "" {
		return token.RawLexeme
	}
	return token.Lexeme
}

func (token Token) SourceLength() int {
	return len(token.SourceLexeme())
}
