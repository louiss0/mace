package parser

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser/ast"
)

var tAssert *assert.Assertions

func TestParser(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Parser Suite")
}

func lexInput(input string) ([]lexer.Token, error) {
	lexerInstance := lexer.New(input)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			return tokens, nil
		}
	}
}

func parseExpressionInput(input string) (ast.Expression, error) {
	tokens, err := lexInput(input)
	if err != nil {
		return nil, err
	}

	return New(tokens).ParseExpression()
}

func requireIdentifier(expression ast.Expression, name string) ast.Identifier {
	identifier, ok := expression.(ast.Identifier)
	tAssert.True(ok)
	if !ok {
		return ast.Identifier{}
	}
	tAssert.Equal(name, identifier.Name)
	return identifier
}

func requireIntLiteral(expression ast.Expression, lexeme string) ast.IntLiteral {
	literal, ok := expression.(ast.IntLiteral)
	tAssert.True(ok)
	if !ok {
		return ast.IntLiteral{}
	}
	tAssert.Equal(lexeme, literal.Lexeme)
	return literal
}

func requirePrefix(expression ast.Expression, operator lexer.TokenType) ast.PrefixExpression {
	prefix, ok := expression.(ast.PrefixExpression)
	tAssert.True(ok)
	if !ok {
		return ast.PrefixExpression{}
	}
	tAssert.Equal(operator, prefix.Operator)
	return prefix
}

func requireInfix(expression ast.Expression, operator lexer.TokenType) ast.InfixExpression {
	infix, ok := expression.(ast.InfixExpression)
	tAssert.True(ok)
	if !ok {
		return ast.InfixExpression{}
	}
	tAssert.Equal(operator, infix.Operator)
	return infix
}

func requireConditional(expression ast.Expression) ast.ConditionalExpression {
	conditional, ok := expression.(ast.ConditionalExpression)
	tAssert.True(ok)
	if !ok {
		return ast.ConditionalExpression{}
	}
	return conditional
}

var _ = Describe("Parser", func() {
	DescribeTable("parses identifiers and literals",
		func(input string, assertExpression func(ast.Expression)) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)
			assertExpression(expression)
		},
		Entry("identifier", "user_name", func(expression ast.Expression) {
			requireIdentifier(expression, "user_name")
		}),
		Entry("int literal", "42", func(expression ast.Expression) {
			requireIntLiteral(expression, "42")
		}),
	)

	DescribeTable("parses prefix expressions",
		func(input string, operator lexer.TokenType, rightName string) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)

			prefix := requirePrefix(expression, operator)
			requireIdentifier(prefix.Right, rightName)
		},
		Entry("minus identifier", "-value", lexer.TokenMinus, "value"),
	)

	DescribeTable("parses infix precedence",
		func(input string) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)

			root := requireInfix(expression, lexer.TokenPlus)
			requireIntLiteral(root.Left, "1")

			right := requireInfix(root.Right, lexer.TokenStar)
			requireIntLiteral(right.Left, "2")
			requireIntLiteral(right.Right, "3")
		},
		Entry("add with multiply", "1 + 2 * 3"),
	)

	DescribeTable("parses grouped expressions",
		func(input string) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)

			root := requireInfix(expression, lexer.TokenStar)
			left := requireInfix(root.Left, lexer.TokenPlus)
			requireIntLiteral(left.Left, "1")
			requireIntLiteral(left.Right, "2")
			requireIntLiteral(root.Right, "3")
		},
		Entry("grouped add then multiply", "(1 + 2) * 3"),
	)

	DescribeTable("parses right associative exponentiation",
		func(input string) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)

			root := requireInfix(expression, lexer.TokenDoubleStar)
			requireIntLiteral(root.Left, "2")

			right := requireInfix(root.Right, lexer.TokenDoubleStar)
			requireIntLiteral(right.Left, "3")
			requireIntLiteral(right.Right, "4")
		},
		Entry("double star associates right", "2 ** 3 ** 4"),
	)

	DescribeTable("parses conditional expressions",
		func(input string) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)

			root := requireConditional(expression)
			requireIdentifier(root.Condition, "a")
			requireIdentifier(root.Then, "b")

			elseConditional := requireConditional(root.Else)
			requireIdentifier(elseConditional.Condition, "c")
			requireIdentifier(elseConditional.Then, "d")
			requireIdentifier(elseConditional.Else, "e")
		},
		Entry("nested ternary", "a ? b : c ? d : e"),
	)

	DescribeTable("returns an error when expressions are malformed",
		func(input string) {
			_, err := parseExpressionInput(input)
			tAssert.Error(err)
		},
		Entry("unterminated group", "(1 + 2"),
	)
})
