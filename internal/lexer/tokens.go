package lexer

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdentifier
	TokenSelf

	TokenFrom
	TokenImport
	TokenTypeKeyword
	TokenSchema
	TokenGenDoc
	TokenSchemaDoc
	TokenArray
	TokenUnion
	TokenVariant
	TokenChoice
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
	TokenInjectable
	TokenNullable
	TokenIn
	TokenNull

	TokenString
	TokenInt
	TokenFloat
	TokenHexInt
	TokenHexFloat
	TokenBoolean

	TokenAssign
	TokenSemicolon
	TokenComma
	TokenColon
	TokenQuestion
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
	TokenMerge
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
