package ast

import (
	"unicode/utf8"

	"github.com/louiss0/mace/internal/lexer"
)

type SourcePosition struct {
	Line   int
	Column int
}

type SourceRange struct {
	Start SourcePosition
	End   SourcePosition
}

type Node interface {
	Range() SourceRange
}

func TokenRange(token lexer.Token) SourceRange {
	return SourceRange{
		Start: SourcePosition{Line: token.Line, Column: token.Column},
		End: SourcePosition{
			Line:   token.Line,
			Column: token.Column + utf8.RuneCountInString(token.Lexeme),
		},
	}
}

type Expression interface {
	Node
	expressionNode()
}

type Identifier struct {
	Token lexer.Token
	Name  string
}

func (Identifier) expressionNode() {
	_ = 0
}

func (i Identifier) Range() SourceRange {
	return TokenRange(i.Token)
}

type MemberAccess struct {
	Target   Expression
	Name     string
	Optional bool
}

func (MemberAccess) expressionNode() {
	_ = 0
}

func (m MemberAccess) Range() SourceRange {
	return m.Target.Range()
}

type ArrayAccess struct {
	Target Expression
	Index  IntLiteral
}

func (ArrayAccess) expressionNode() {
	_ = 0
}

func (a ArrayAccess) Range() SourceRange {
	return a.Target.Range()
}

type StringLiteral struct {
	Token  lexer.Token
	Lexeme string
}

func (StringLiteral) expressionNode() {
	_ = 0
}

func (s StringLiteral) Range() SourceRange {
	return TokenRange(s.Token)
}

type IntLiteral struct {
	Token  lexer.Token
	Lexeme string
}

func (IntLiteral) expressionNode() {
	_ = 0
}

func (i IntLiteral) Range() SourceRange {
	return TokenRange(i.Token)
}

type FloatLiteral struct {
	Token  lexer.Token
	Lexeme string
}

func (FloatLiteral) expressionNode() {
	_ = 0
}

func (f FloatLiteral) Range() SourceRange {
	return TokenRange(f.Token)
}

type HexIntLiteral struct {
	Token  lexer.Token
	Lexeme string
}

func (HexIntLiteral) expressionNode() {
	_ = 0
}

func (h HexIntLiteral) Range() SourceRange {
	return TokenRange(h.Token)
}

type HexFloatLiteral struct {
	Token  lexer.Token
	Lexeme string
}

func (HexFloatLiteral) expressionNode() {
	_ = 0
}

func (h HexFloatLiteral) Range() SourceRange {
	return TokenRange(h.Token)
}

type BooleanLiteral struct {
	Token lexer.Token
	Value bool
}

func (BooleanLiteral) expressionNode() {
	_ = 0
}

func (b BooleanLiteral) Range() SourceRange {
	return TokenRange(b.Token)
}

type NullLiteral struct {
	Token lexer.Token
}

func (NullLiteral) expressionNode() {
	_ = 0
}

func (n NullLiteral) Range() SourceRange {
	return TokenRange(n.Token)
}

type ArrayLiteral struct {
	StartToken lexer.Token
	Elements   []Expression
}

func (ArrayLiteral) expressionNode() {
	_ = 0
}

func (a ArrayLiteral) Range() SourceRange {
	return TokenRange(a.StartToken)
}

type RecordLiteral struct {
	StartToken lexer.Token
	Fields     []RecordField
}

func (RecordLiteral) expressionNode() {
	_ = 0
}

func (r RecordLiteral) Range() SourceRange {
	return TokenRange(r.StartToken)
}

type RecordField struct {
	NameToken lexer.Token
	Name      string
	Optional  bool
	Shorthand bool
	Value     Expression
}

func (r RecordField) Range() SourceRange {
	return TokenRange(r.NameToken)
}

type PrefixExpression struct {
	OperatorToken lexer.Token
	Operator      lexer.TokenType
	Right         Expression
}

func (PrefixExpression) expressionNode() {
	_ = 0
}

func (p PrefixExpression) Range() SourceRange {
	return TokenRange(p.OperatorToken)
}

type InfixExpression struct {
	Left     Expression
	Operator lexer.TokenType
	Right    Expression
}

func (InfixExpression) expressionNode() {
	_ = 0
}

func (i InfixExpression) Range() SourceRange {
	return i.Left.Range()
}

type TypeTestExpression struct {
	Expression Expression
	TargetType TypeReference
	EndToken   lexer.Token
}

func (TypeTestExpression) expressionNode() {
	_ = 0
}

func (t TypeTestExpression) Range() SourceRange {
	sourceRange := t.Expression.Range()
	sourceRange.End = TokenRange(t.EndToken).End
	return sourceRange
}

type ConditionalExpression struct {
	Condition Expression
	Then      Expression
	Else      Expression
}

func (ConditionalExpression) expressionNode() {
	_ = 0
}

func (c ConditionalExpression) Range() SourceRange {
	return c.Condition.Range()
}

type SelfReference struct {
	Token lexer.Token
	Path  []string
}

func (SelfReference) expressionNode() {
	_ = 0
}

func (s SelfReference) Range() SourceRange {
	return TokenRange(s.Token)
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
	Node
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

func (v VariableDeclaration) Range() SourceRange {
	return TokenRange(v.NameToken)
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

func (t TypeDeclaration) Range() SourceRange {
	return TokenRange(t.NameToken)
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

func (d DocDeclaration) Range() SourceRange {
	return TokenRange(d.TargetToken)
}

func (SchemaDeclaration) declarationNode() {
	_ = 0
}

func (s SchemaDeclaration) Range() SourceRange {
	return TokenRange(s.NameToken)
}

type TypeReference interface {
	Node
	typeReferenceNode()
}

type PrimitiveType struct {
	Token lexer.Token
	Name  string
}

func (PrimitiveType) typeReferenceNode() {
	_ = 0
}

func (p PrimitiveType) Range() SourceRange {
	return TokenRange(p.Token)
}

type ArrayType struct {
	Token   lexer.Token
	Element TypeReference
}

func (ArrayType) typeReferenceNode() {
	_ = 0
}

func (a ArrayType) Range() SourceRange {
	return TokenRange(a.Token)
}

type RecordMapType struct {
	Token lexer.Token
	Value TypeReference
}

func (RecordMapType) typeReferenceNode() {
	_ = 0
}

func (r RecordMapType) Range() SourceRange {
	return TokenRange(r.Token)
}

type UnionType struct {
	Token   lexer.Token
	Members []TypeReference
}

func (UnionType) typeReferenceNode() {
	_ = 0
}

func (u UnionType) Range() SourceRange {
	return TokenRange(u.Token)
}

type VariantType struct {
	Token   lexer.Token
	Members []TypeReference
}

func (VariantType) typeReferenceNode() {
	_ = 0
}

func (v VariantType) Range() SourceRange {
	return TokenRange(v.Token)
}

type ChoiceType struct {
	Token   lexer.Token
	Members []Expression
}

func (ChoiceType) typeReferenceNode() {
	_ = 0
}

func (c ChoiceType) Range() SourceRange {
	return TokenRange(c.Token)
}

type NamedType struct {
	Token lexer.Token
	Name  string
}

func (NamedType) typeReferenceNode() {
	_ = 0
}

func (n NamedType) Range() SourceRange {
	return TokenRange(n.Token)
}

type RecordType struct {
	StartToken lexer.Token
	Fields     []SchemaField
}

func (RecordType) typeReferenceNode() {
	_ = 0
}

func (r RecordType) Range() SourceRange {
	return TokenRange(r.StartToken)
}

type SchemaField struct {
	NameToken   lexer.Token
	Name        string
	Optional    bool
	Type        TypeReference
	Description string
}

func (s SchemaField) Range() SourceRange {
	return TokenRange(s.NameToken)
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
	Shorthand   bool
	Value       Expression
	Description string
}

func (o OutputField) Range() SourceRange {
	return TokenRange(o.NameToken)
}

type OutputSchemaField struct {
	NameToken   lexer.Token
	Name        string
	Optional    bool
	Type        TypeReference
	Description string
}

func (o OutputSchemaField) Range() SourceRange {
	return TokenRange(o.NameToken)
}
