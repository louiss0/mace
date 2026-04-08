package lexer

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestLexer(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Lexer Suite")
}

type expectedToken struct {
	tokenType TokenType
	lexeme    string
	line      int
	column    int
}

func collectTokens(input string) ([]Token, error) {
	lexerInstance := New(input)
	tokens := []Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
		if token.Type == TokenEOF {
			return tokens, nil
		}
	}
}

func assertTokenSequence(actual []Token, expected []expectedToken) {
	if !tAssert.Equal(len(expected), len(actual)) {
		return
	}

	for index, expectedToken := range expected {
		actualToken := actual[index]
		tAssert.Equal(expectedToken.tokenType, actualToken.Type)
		tAssert.Equal(expectedToken.lexeme, actualToken.Lexeme)
	}
}

func assertTokenTypes(actual []Token, expected []TokenType) {
	if !tAssert.Equal(len(expected), len(actual)) {
		return
	}

	for index, expectedToken := range expected {
		actualToken := actual[index]
		tAssert.Equal(expectedToken, actualToken.Type)
	}
}

func assertTokenPositions(actual []Token, expected []expectedToken) {
	if !tAssert.Equal(len(expected), len(actual)) {
		return
	}

	for index, expectedToken := range expected {
		actualToken := actual[index]
		tAssert.Equal(expectedToken.tokenType, actualToken.Type)
		tAssert.Equal(expectedToken.lexeme, actualToken.Lexeme)
		tAssert.Equal(expectedToken.line, actualToken.Line)
		tAssert.Equal(expectedToken.column, actualToken.Column)
	}
}

var _ = Describe("Lexer", func() {
	DescribeTable("lexes identifiers and keywords",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("keywords and identifiers", "from import type schema enum array string int float boolean output schema_file data injectable user_1", []expectedToken{
			{tokenType: TokenFrom, lexeme: "from"},
			{tokenType: TokenImport, lexeme: "import"},
			{tokenType: TokenTypeKeyword, lexeme: "type"},
			{tokenType: TokenSchema, lexeme: "schema"},
			{tokenType: TokenEnum, lexeme: "enum"},
			{tokenType: TokenArray, lexeme: "array"},
			{tokenType: TokenStringType, lexeme: "string"},
			{tokenType: TokenIntType, lexeme: "int"},
			{tokenType: TokenFloatType, lexeme: "float"},
			{tokenType: TokenBooleanType, lexeme: "boolean"},
			{tokenType: TokenOutput, lexeme: "output"},
			{tokenType: TokenSchemaFile, lexeme: "schema_file"},
			{tokenType: TokenData, lexeme: "data"},
			{tokenType: TokenInjectable, lexeme: "injectable"},
			{tokenType: TokenIdentifier, lexeme: "user_1"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("lexes literals",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("string and numbers", "\"hello\" 0 42 3.14 10.0 true false", []expectedToken{
			{tokenType: TokenString, lexeme: "\"hello\""},
			{tokenType: TokenInt, lexeme: "0"},
			{tokenType: TokenInt, lexeme: "42"},
			{tokenType: TokenFloat, lexeme: "3.14"},
			{tokenType: TokenFloat, lexeme: "10.0"},
			{tokenType: TokenBoolean, lexeme: "true"},
			{tokenType: TokenBoolean, lexeme: "false"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("lexes operators and punctuation",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("operators", "= ; , : ? . + - * / % ** ! ~ < <= > >= == != === !== & ^ | && || << >> >>> ( ) { } [ ]", []expectedToken{
			{tokenType: TokenAssign, lexeme: "="},
			{tokenType: TokenSemicolon, lexeme: ";"},
			{tokenType: TokenComma, lexeme: ","},
			{tokenType: TokenColon, lexeme: ":"},
			{tokenType: TokenQuestion, lexeme: "?"},
			{tokenType: TokenDot, lexeme: "."},
			{tokenType: TokenPlus, lexeme: "+"},
			{tokenType: TokenMinus, lexeme: "-"},
			{tokenType: TokenStar, lexeme: "*"},
			{tokenType: TokenSlash, lexeme: "/"},
			{tokenType: TokenPercent, lexeme: "%"},
			{tokenType: TokenDoubleStar, lexeme: "**"},
			{tokenType: TokenBang, lexeme: "!"},
			{tokenType: TokenTilde, lexeme: "~"},
			{tokenType: TokenLess, lexeme: "<"},
			{tokenType: TokenLessEqual, lexeme: "<="},
			{tokenType: TokenGreater, lexeme: ">"},
			{tokenType: TokenGreaterEqual, lexeme: ">="},
			{tokenType: TokenEqualEqual, lexeme: "=="},
			{tokenType: TokenNotEqual, lexeme: "!="},
			{tokenType: TokenStrictEqual, lexeme: "==="},
			{tokenType: TokenStrictNotEqual, lexeme: "!=="},
			{tokenType: TokenAmpersand, lexeme: "&"},
			{tokenType: TokenCaret, lexeme: "^"},
			{tokenType: TokenPipe, lexeme: "|"},
			{tokenType: TokenAndAnd, lexeme: "&&"},
			{tokenType: TokenOrOr, lexeme: "||"},
			{tokenType: TokenShiftLeft, lexeme: "<<"},
			{tokenType: TokenShiftRight, lexeme: ">>"},
			{tokenType: TokenShiftRightUnsigned, lexeme: ">>>"},
			{tokenType: TokenLParen, lexeme: "("},
			{tokenType: TokenRParen, lexeme: ")"},
			{tokenType: TokenLBrace, lexeme: "{"},
			{tokenType: TokenRBrace, lexeme: "}"},
			{tokenType: TokenLBracket, lexeme: "["},
			{tokenType: TokenRBracket, lexeme: "]"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("lexes script delimiters",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("script delimiters", "|===| |====|", []expectedToken{
			{tokenType: TokenScriptDelimiter, lexeme: "|===|"},
			{tokenType: TokenScriptDelimiter, lexeme: "|====|"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("skips whitespace and comments",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("line comments", "  /= comment line\nname value", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "name"},
			{tokenType: TokenIdentifier, lexeme: "value"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
		Entry("block comments", "name /= block comment =/ value", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "name"},
			{tokenType: TokenIdentifier, lexeme: "value"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("tracks line and column positions",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenPositions(tokens, expected)
		},
		Entry("newline positions", "alpha\n  beta", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "alpha", line: 1, column: 1},
			{tokenType: TokenIdentifier, lexeme: "beta", line: 2, column: 3},
			{tokenType: TokenEOF, lexeme: "", line: 2, column: 7},
		}),
		Entry("crlf positions", "alpha\r\nbeta", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "alpha", line: 1, column: 1},
			{tokenType: TokenIdentifier, lexeme: "beta", line: 2, column: 1},
			{tokenType: TokenEOF, lexeme: "", line: 2, column: 5},
		}),
	)

	DescribeTable("treats dot without a trailing digit as punctuation",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("dot punctuation", "10. foo", []expectedToken{
			{tokenType: TokenInt, lexeme: "10"},
			{tokenType: TokenDot, lexeme: "."},
			{tokenType: TokenIdentifier, lexeme: "foo"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("lexes string literals",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("escaped newline stays in lexeme", "\"line 1\\nline 2\"", []expectedToken{
			{tokenType: TokenString, lexeme: "\"line 1\\nline 2\""},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("rejects unterminated string literals",
		func(input string) {
			_, err := collectTokens(input)
			tAssert.Error(err)
		},
		Entry("unterminated string", "\"unterminated"),
	)

	DescribeTable("lexes $self as a dedicated token",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("self token", "$self.value", []expectedToken{
			{tokenType: TokenSelf, lexeme: "$self"},
			{tokenType: TokenDot, lexeme: "."},
			{tokenType: TokenIdentifier, lexeme: "value"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("lexes schema declarations with multiple fields",
		func(input string, expected []TokenType) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenTypes(tokens, expected)
		},
		Entry("single schema with required and optional fields", "schema User: { name: string; age?: int; };", []TokenType{
			TokenSchema, TokenIdentifier, TokenColon, TokenLBrace,
			TokenIdentifier, TokenColon, TokenStringType, TokenSemicolon,
			TokenIdentifier, TokenQuestion, TokenColon, TokenIntType, TokenSemicolon,
			TokenRBrace, TokenSemicolon, TokenEOF,
		}),
		Entry("multiple schemas with nested references", "schema Address: { street: string; }; schema User: { address: Address; tags: array<string>; };", []TokenType{
			TokenSchema, TokenIdentifier, TokenColon, TokenLBrace,
			TokenIdentifier, TokenColon, TokenStringType, TokenSemicolon,
			TokenRBrace, TokenSemicolon,
			TokenSchema, TokenIdentifier, TokenColon, TokenLBrace,
			TokenIdentifier, TokenColon, TokenIdentifier, TokenSemicolon,
			TokenIdentifier, TokenColon, TokenArray, TokenLess, TokenStringType, TokenGreater, TokenSemicolon,
			TokenRBrace, TokenSemicolon, TokenEOF,
		}),
		Entry("schema inside a script block", "|===| schema Config: { nested: array<Profile>; flags: array<boolean>; }; |===|", []TokenType{
			TokenScriptDelimiter,
			TokenSchema, TokenIdentifier, TokenColon, TokenLBrace,
			TokenIdentifier, TokenColon, TokenArray, TokenLess, TokenIdentifier, TokenGreater, TokenSemicolon,
			TokenIdentifier, TokenColon, TokenArray, TokenLess, TokenBooleanType, TokenGreater, TokenSemicolon,
			TokenRBrace, TokenSemicolon,
			TokenScriptDelimiter, TokenEOF,
		}),
	)

	DescribeTable("lexes union type references",
		func(input string, expected []TokenType) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenTypes(tokens, expected)
		},
		Entry("primitive union", "type Value: union[string, int];", []TokenType{
			TokenTypeKeyword, TokenIdentifier, TokenColon, TokenUnion, TokenLBracket,
			TokenStringType, TokenComma, TokenIntType, TokenRBracket, TokenSemicolon, TokenEOF,
		}),
		Entry("union with array and identifier", "type Value: union[array<string>, User];", []TokenType{
			TokenTypeKeyword, TokenIdentifier, TokenColon, TokenUnion, TokenLBracket,
			TokenArray, TokenLess, TokenStringType, TokenGreater, TokenComma, TokenIdentifier, TokenRBracket, TokenSemicolon, TokenEOF,
		}),
	)

	DescribeTable("lexes array type references with nested schemas",
		func(input string, expected []TokenType) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenTypes(tokens, expected)
		},
		Entry("array of schema identifiers", "schema User: { tags: array<Tag>; };", []TokenType{
			TokenSchema, TokenIdentifier, TokenColon, TokenLBrace,
			TokenIdentifier, TokenColon, TokenArray, TokenLess, TokenIdentifier, TokenGreater, TokenSemicolon,
			TokenRBrace, TokenSemicolon, TokenEOF,
		}),
		Entry("nested array of schema identifiers", "schema User: { tags: array<array<Tag>>; };", []TokenType{
			TokenSchema, TokenIdentifier, TokenColon, TokenLBrace,
			TokenIdentifier, TokenColon, TokenArray, TokenLess, TokenArray, TokenLess, TokenIdentifier,
			TokenShiftRight, TokenSemicolon,
			TokenRBrace, TokenSemicolon, TokenEOF,
		}),
		Entry("array of primitive and schema references", "schema User: { flags: array<boolean>; roles: array<Role>; };", []TokenType{
			TokenSchema, TokenIdentifier, TokenColon, TokenLBrace,
			TokenIdentifier, TokenColon, TokenArray, TokenLess, TokenBooleanType, TokenGreater, TokenSemicolon,
			TokenIdentifier, TokenColon, TokenArray, TokenLess, TokenIdentifier, TokenGreater, TokenSemicolon,
			TokenRBrace, TokenSemicolon, TokenEOF,
		}),
	)

	DescribeTable("rejects invalid identifiers and characters",
		func(input string) {
			_, err := collectTokens(input)
			tAssert.Error(err)
		},
		Entry("leading underscore", "_hidden"),
		Entry("unterminated string", "\"missing"),
		Entry("invalid character", "`"),
	)

	DescribeTable("lexes mixed shift operators by length",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("shift operators", ">>> >> >", []expectedToken{
			{tokenType: TokenShiftRightUnsigned, lexeme: ">>>"},
			{tokenType: TokenShiftRight, lexeme: ">>"},
			{tokenType: TokenGreater, lexeme: ">"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("treats keyword prefixes as identifiers",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("keyword prefixes", "schemaFile output_data", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "schemaFile"},
			{tokenType: TokenIdentifier, lexeme: "output_data"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("treats short script delimiters as pipes",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("short script delimiters", "|==|", []expectedToken{
			{tokenType: TokenPipe, lexeme: "|"},
			{tokenType: TokenEqualEqual, lexeme: "=="},
			{tokenType: TokenPipe, lexeme: "|"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)

	DescribeTable("lexes comments around tokens",
		func(input string, expected []expectedToken) {
			tokens, err := collectTokens(input)
			tAssert.NoError(err)
			assertTokenSequence(tokens, expected)
		},
		Entry("block comment before newline", "/= block =/ value", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "value"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
		Entry("line comment at eof", "name /= comment", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "name"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
		Entry("line comment does not consume later block closer", "/= comment\n/= block =/ value", []expectedToken{
			{tokenType: TokenIdentifier, lexeme: "value"},
			{tokenType: TokenEOF, lexeme: ""},
		}),
	)
})
