package ast

import "github.com/louiss0/mace/internal/lexer"

type Expression interface {
	expressionNode()
}

type Identifier struct {
	Name string
}

func (Identifier) expressionNode() {
	_ = 0
}

type MemberAccess struct {
	Target Expression
	Name   string
}

func (MemberAccess) expressionNode() {
	_ = 0
}

type ArrayAccess struct {
	Target Expression
	Index  IntLiteral
}

func (ArrayAccess) expressionNode() {
	_ = 0
}

type StringLiteral struct {
	Lexeme string
}

func (StringLiteral) expressionNode() {
	_ = 0
}

type IntLiteral struct {
	Lexeme string
}

func (IntLiteral) expressionNode() {
	_ = 0
}

type FloatLiteral struct {
	Lexeme string
}

func (FloatLiteral) expressionNode() {
	_ = 0
}

type HexIntLiteral struct {
	Lexeme string
}

func (HexIntLiteral) expressionNode() {
	_ = 0
}

type HexFloatLiteral struct {
	Lexeme string
}

func (HexFloatLiteral) expressionNode() {
	_ = 0
}

type BooleanLiteral struct {
	Value bool
}

func (BooleanLiteral) expressionNode() {
	_ = 0
}

type NullLiteral struct{}

func (NullLiteral) expressionNode() {
	_ = 0
}

type ArrayLiteral struct {
	Elements []Expression
}

func (ArrayLiteral) expressionNode() {
	_ = 0
}

type RecordLiteral struct {
	Fields []RecordField
}

func (RecordLiteral) expressionNode() {
	_ = 0
}

type RecordField struct {
	Name     string
	Optional bool
	Value    Expression
}

type PrefixExpression struct {
	Operator lexer.TokenType
	Right    Expression
}

func (PrefixExpression) expressionNode() {
	_ = 0
}

type InfixExpression struct {
	Left     Expression
	Operator lexer.TokenType
	Right    Expression
}

func (InfixExpression) expressionNode() {
	_ = 0
}

type ConditionalExpression struct {
	Condition Expression
	Then      Expression
	Else      Expression
}

func (ConditionalExpression) expressionNode() {
	_ = 0
}

type SelfReference struct {
	Path []string
}

func (SelfReference) expressionNode() {
	_ = 0
}

type File struct {
	Imports []ImportDeclaration
	Script  *ScriptBlock
	Output  OutputBlock
}

type ImportedIdentifier struct {
	Name  string // exported name (the name in the source file)
	Alias string // local alias (empty if no alias)
}

func (i ImportedIdentifier) LocalName() string {
	if i.Alias != "" {
		return i.Alias
	}
	return i.Name
}

type ImportDeclaration struct {
	Path        StringLiteral
	Identifiers []ImportedIdentifier
	ImportAs    *ImportedIdentifier
}

type ScriptBlock struct {
	Imports []ImportDeclaration
	Items   []Declaration
}

type Documentation struct {
	Summary     *StringLiteral
	Description *StringLiteral
	Props       map[string]StringLiteral
}

type DocumentationKind int

const (
	DocumentationKindGeneral DocumentationKind = iota
	DocumentationKindSchema
)

type Declaration interface {
	declarationNode()
}

type VariableDeclaration struct {
	Nullable  bool
	HasValue  bool
	Type      TypeReference
	NameToken lexer.Token
	Name      string
	Value     Expression
}

func (VariableDeclaration) declarationNode() {
	_ = 0
}

type TypeDeclaration struct {
	NameToken   lexer.Token
	Name        string
	Type        TypeReference
	Description string
}

func (TypeDeclaration) declarationNode() {
	_ = 0
}

type SchemaDeclaration struct {
	NameToken lexer.Token
	Name      string
	Type      RecordType
}

type DocDeclaration struct {
	Kind          DocumentationKind
	KeywordToken  lexer.Token
	TargetToken   lexer.Token
	Target        string
	Documentation Documentation
}

func (DocDeclaration) declarationNode() {
	_ = 0
}

func (SchemaDeclaration) declarationNode() {
	_ = 0
}

type TypeReference interface {
	typeReferenceNode()
}

type PrimitiveType struct {
	Name string
}

func (PrimitiveType) typeReferenceNode() {
	_ = 0
}

type ArrayType struct {
	Element TypeReference
}

func (ArrayType) typeReferenceNode() {
	_ = 0
}

type RecordMapType struct {
	Value TypeReference
}

func (RecordMapType) typeReferenceNode() {
	_ = 0
}

type UnionType struct {
	Members []TypeReference
}

func (UnionType) typeReferenceNode() {
	_ = 0
}

type VariantType struct {
	Members []TypeReference
}

func (VariantType) typeReferenceNode() {
	_ = 0
}

type ChoiceType struct {
	Members []Expression
}

func (ChoiceType) typeReferenceNode() {
	_ = 0
}

type NamedType struct {
	Name string
}

func (NamedType) typeReferenceNode() {
	_ = 0
}

type RecordType struct {
	Fields []SchemaField
}

func (RecordType) typeReferenceNode() {
	_ = 0
}

type SchemaField struct {
	Name        string
	Optional    bool
	Type        TypeReference
	Description string
}

type OutputBlock struct {
	Directives   []OutputDirective
	Doc          *StringLiteral
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
	OutputDirectiveParse
	OutputDirectiveParseFile
)

type OutputDirective struct {
	Kind  OutputDirectiveKind
	Value string
}

type OutputField struct {
	NameToken   lexer.Token
	Name        string
	Optional    bool
	Value       Expression
	Description string
}

type OutputSchemaField struct {
	NameToken   lexer.Token
	Name        string
	Optional    bool
	Type        TypeReference
	Description string
}
