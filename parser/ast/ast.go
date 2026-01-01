package ast

import "github.com/louiss0/mace/lexer"

type Expression interface {
	expressionNode()
}

type Identifier struct {
	Name string
}

func (Identifier) expressionNode() {}

type StringLiteral struct {
	Lexeme string
}

func (StringLiteral) expressionNode() {}

type IntLiteral struct {
	Lexeme string
}

func (IntLiteral) expressionNode() {}

type FloatLiteral struct {
	Lexeme string
}

func (FloatLiteral) expressionNode() {}

type BooleanLiteral struct {
	Value bool
}

func (BooleanLiteral) expressionNode() {}

type PrefixExpression struct {
	Operator lexer.TokenType
	Right    Expression
}

func (PrefixExpression) expressionNode() {}

type InfixExpression struct {
	Left     Expression
	Operator lexer.TokenType
	Right    Expression
}

func (InfixExpression) expressionNode() {}

type ConditionalExpression struct {
	Condition Expression
	Then      Expression
	Else      Expression
}

func (ConditionalExpression) expressionNode() {}

type File struct {
	Expression Expression
}
