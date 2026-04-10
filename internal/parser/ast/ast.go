package ast

import "github.com/louiss0/mace/internal/lexer"

type Expression interface {
	expressionNode()
}

type Identifier struct {
	Name string
}

func (Identifier) expressionNode() {}

type MemberAccess struct {
	Target Expression
	Name   string
}

func (MemberAccess) expressionNode() {}

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

type ArrayLiteral struct {
	Elements []Expression
}

func (ArrayLiteral) expressionNode() {}

type RecordLiteral struct {
	Fields []RecordField
}

func (RecordLiteral) expressionNode() {}

type RecordField struct {
	Name     string
	Optional bool
	Value    Expression
}

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

type SelfReference struct {
	Path []string
}

func (SelfReference) expressionNode() {}

type File struct {
	Imports []ImportDeclaration
	Script  *ScriptBlock
	Output  OutputBlock
}

type ImportDeclaration struct {
	Path        StringLiteral
	Identifiers []string
}

type ScriptBlock struct {
	Items []Declaration
}

type Declaration interface {
	declarationNode()
}

type VariableDeclaration struct {
	Injectable bool
	HasValue   bool
	Type       TypeReference
	NameToken  lexer.Token
	Name       string
	Value      Expression
}

func (VariableDeclaration) declarationNode() {}

type TypeDeclaration struct {
	NameToken lexer.Token
	Name      string
	Type      TypeReference
}

func (TypeDeclaration) declarationNode() {}

type SchemaDeclaration struct {
	NameToken lexer.Token
	Name      string
	Type      RecordType
}

func (SchemaDeclaration) declarationNode() {}

type EnumDeclaration struct {
	NameToken   lexer.Token
	Name        string
	BackingType PrimitiveType
	Members     []EnumMember
}

func (EnumDeclaration) declarationNode() {}

type EnumMember struct {
	NameToken lexer.Token
	Name      string
	HasValue  bool
	Value     Expression
}

type TypeReference interface {
	typeReferenceNode()
}

type PrimitiveType struct {
	Name string
}

func (PrimitiveType) typeReferenceNode() {}

type ArrayType struct {
	Element TypeReference
}

func (ArrayType) typeReferenceNode() {}

type UnionType struct {
	Members []TypeReference
}

func (UnionType) typeReferenceNode() {}

type VariantType struct {
	Members []TypeReference
}

func (VariantType) typeReferenceNode() {}

type NamedType struct {
	Name string
}

func (NamedType) typeReferenceNode() {}

type RecordType struct {
	Doc    *StringLiteral
	Fields []SchemaField
}

func (RecordType) typeReferenceNode() {}

type SchemaField struct {
	Name     string
	Optional bool
	Type     TypeReference
}

type OutputBlock struct {
	Directives   []OutputDirective
	Mode         OutputMode
	DataFields   []OutputField
	SchemaFields []OutputSchemaField
}

type OutputMode int

const (
	OutputModeData OutputMode = iota
	OutputModeSchema
)

type OutputDirectiveKind int

const (
	OutputDirectiveOutput OutputDirectiveKind = iota
	OutputDirectiveSchemaFile
	OutputDirectiveSchema
)

type OutputDirective struct {
	Kind  OutputDirectiveKind
	Value string
}

type OutputField struct {
	NameToken lexer.Token
	Name      string
	Optional  bool
	Value     Expression
}

type OutputSchemaField struct {
	NameToken lexer.Token
	Name      string
	Optional  bool
	Type      TypeReference
}
