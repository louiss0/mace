package parser

import (
	"fmt"
	"strings"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser/ast"
)

type Parser struct {
	tokens   []lexer.Token
	position int
}

func New(tokens []lexer.Token) *Parser {
	return &Parser{
		tokens: tokens,
	}
}

func (p *Parser) ParseFile() (ast.File, error) {
	expression, err := p.ParseExpression()
	if err != nil {
		return ast.File{}, err
	}

	if !p.isAtEnd() {
		return ast.File{}, p.unexpectedTokenError("parser: unexpected token after expression")
	}

	return ast.File{Expression: expression}, nil
}

func (p *Parser) ParseExpression() (ast.Expression, error) {
	if len(p.tokens) == 0 {
		return nil, fmt.Errorf("parser: empty token stream")
	}

	return p.parseExpression(precedenceLowest)
}

func (p *Parser) parseExpression(precedence int) (ast.Expression, error) {
	token := p.current()
	left, err := p.parsePrefix(token)
	if err != nil {
		return nil, err
	}

	for !p.isAtEnd() && precedence < p.currentPrecedence() {
		operator := p.current()
		p.advance()

		if operator.Type == lexer.TokenQuestion {
			left, err = p.parseConditionalExpression(left, operator)
		} else {
			left, err = p.parseInfixExpression(left, operator)
		}

		if err != nil {
			return nil, err
		}
	}

	return left, nil
}

func (p *Parser) parsePrefix(token lexer.Token) (ast.Expression, error) {
	switch token.Type {
	case lexer.TokenIdentifier:
		p.advance()
		return ast.Identifier{Name: token.Lexeme}, nil
	case lexer.TokenString:
		p.advance()
		return ast.StringLiteral{Lexeme: token.Lexeme}, nil
	case lexer.TokenInt:
		p.advance()
		return ast.IntLiteral{Lexeme: token.Lexeme}, nil
	case lexer.TokenFloat:
		p.advance()
		return ast.FloatLiteral{Lexeme: token.Lexeme}, nil
	case lexer.TokenBoolean:
		p.advance()
		return ast.BooleanLiteral{Value: token.Lexeme == "true"}, nil
	case lexer.TokenLParen:
		p.advance()
		expression, err := p.parseExpression(precedenceLowest)
		if err != nil {
			return nil, err
		}
		if err := p.expect(lexer.TokenRParen, "parser: expected ')' after expression"); err != nil {
			return nil, err
		}
		return expression, nil
	case lexer.TokenBang, lexer.TokenTilde, lexer.TokenPlus, lexer.TokenMinus:
		p.advance()
		right, err := p.parseExpression(precedencePrefix)
		if err != nil {
			return nil, err
		}
		return ast.PrefixExpression{
			Operator: token.Type,
			Right:    right,
		}, nil
	default:
		return nil, p.unexpectedTokenError("parser: expected expression")
	}
}

func (p *Parser) parseInfixExpression(left ast.Expression, operator lexer.Token) (ast.Expression, error) {
	precedence := p.precedenceFor(operator.Type)
	rightPrecedence := precedence
	if operator.Type == lexer.TokenDoubleStar {
		rightPrecedence = precedence - 1
	}

	right, err := p.parseExpression(rightPrecedence)
	if err != nil {
		return nil, err
	}

	return ast.InfixExpression{
		Left:     left,
		Operator: operator.Type,
		Right:    right,
	}, nil
}

func (p *Parser) parseConditionalExpression(left ast.Expression, operator lexer.Token) (ast.Expression, error) {
	if operator.Type != lexer.TokenQuestion {
		return nil, p.unexpectedTokenError("parser: expected '?' for conditional expression")
	}

	thenExpression, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return nil, err
	}

	if err := p.expect(lexer.TokenColon, "parser: expected ':' in conditional expression"); err != nil {
		return nil, err
	}

	elseExpression, err := p.parseExpression(precedenceTernary - 1)
	if err != nil {
		return nil, err
	}

	return ast.ConditionalExpression{
		Condition: left,
		Then:      thenExpression,
		Else:      elseExpression,
	}, nil
}

func (p *Parser) expect(tokenType lexer.TokenType, message string) error {
	if p.current().Type != tokenType {
		return p.unexpectedTokenError(message)
	}
	p.advance()
	return nil
}

func (p *Parser) current() lexer.Token {
	if len(p.tokens) == 0 {
		return lexer.Token{Type: lexer.TokenEOF}
	}

	if p.position >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}

	return p.tokens[p.position]
}

func (p *Parser) advance() {
	if !p.isAtEnd() {
		p.position++
	}
}

func (p *Parser) isAtEnd() bool {
	if len(p.tokens) == 0 {
		return true
	}

	return p.current().Type == lexer.TokenEOF
}

func (p *Parser) currentPrecedence() int {
	return p.precedenceFor(p.current().Type)
}

func (p *Parser) precedenceFor(tokenType lexer.TokenType) int {
	switch tokenType {
	case lexer.TokenQuestion:
		return precedenceTernary
	case lexer.TokenOrOr:
		return precedenceOr
	case lexer.TokenAndAnd:
		return precedenceAnd
	case lexer.TokenPipe:
		return precedenceBitwiseOr
	case lexer.TokenCaret:
		return precedenceBitwiseXor
	case lexer.TokenAmpersand:
		return precedenceBitwiseAnd
	case lexer.TokenEqualEqual, lexer.TokenNotEqual, lexer.TokenStrictEqual, lexer.TokenStrictNotEqual:
		return precedenceEquality
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		return precedenceRelational
	case lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned:
		return precedenceShift
	case lexer.TokenPlus, lexer.TokenMinus:
		return precedenceAdditive
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return precedenceMultiplicative
	case lexer.TokenDoubleStar:
		return precedenceExponent
	default:
		return precedenceLowest
	}
}

func (p *Parser) unexpectedTokenError(message string) error {
	token := p.current()
	if token.Type == lexer.TokenEOF {
		return fmt.Errorf("%s: EOF", message)
	}

	sanitizedLexeme := strings.ReplaceAll(token.Lexeme, "\n", "\\n")
	sanitizedLexeme = strings.ReplaceAll(sanitizedLexeme, "\r", "\\r")

	return fmt.Errorf("%s at %d:%d near %q", message, token.Line, token.Column, sanitizedLexeme)
}

const (
	precedenceLowest = iota
	precedenceTernary
	precedenceOr
	precedenceAnd
	precedenceBitwiseOr
	precedenceBitwiseXor
	precedenceBitwiseAnd
	precedenceEquality
	precedenceRelational
	precedenceShift
	precedenceAdditive
	precedenceMultiplicative
	precedenceExponent
	precedencePrefix
)
