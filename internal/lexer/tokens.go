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
	Type   TokenType
	Lexeme string
	Line   int
	Column int
}
