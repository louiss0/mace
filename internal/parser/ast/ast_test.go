package ast

import (
	"testing"

	"github.com/louiss0/mace/internal/lexer"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestAST(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "AST Suite")
}

var _ = Describe("AST nodes", func() {
	It("invokes expression node methods", func() {
		expressions := []Expression{
			Identifier{Name: "name"},
			MemberAccess{Target: Identifier{Name: "user"}, Name: "name"},
			ArrayAccess{Target: Identifier{Name: "items"}, Index: IntLiteral{Lexeme: "0"}},
			StringLiteral{Lexeme: "\"value\""},
			IntLiteral{Lexeme: "1"},
			FloatLiteral{Lexeme: "1.5"},
			HexIntLiteral{Lexeme: "0xFF"},
			HexFloatLiteral{Lexeme: "0xF.F"},
			BooleanLiteral{Value: true},
			NullLiteral{},
			ArrayLiteral{Elements: []Expression{IntLiteral{Lexeme: "1"}}},
			RecordLiteral{Fields: []RecordField{{Name: "value", Value: IntLiteral{Lexeme: "1"}}}},
			PrefixExpression{Operator: lexer.TokenBang, Right: BooleanLiteral{Value: true}},
			InfixExpression{Left: IntLiteral{Lexeme: "1"}, Operator: lexer.TokenPlus, Right: IntLiteral{Lexeme: "2"}},
			ConditionalExpression{Condition: BooleanLiteral{Value: true}, Then: IntLiteral{Lexeme: "1"}, Else: IntLiteral{Lexeme: "0"}},
			SelfReference{Path: []string{"user", "name"}},
		}

		for _, expression := range expressions {
			expression.expressionNode()
		}
	})

	It("invokes declaration node methods", func() {
		declarations := []Declaration{
			VariableDeclaration{Name: "name", Type: PrimitiveType{Name: "string"}},
			TypeDeclaration{Name: "Name", Type: PrimitiveType{Name: "string"}},
			SchemaDeclaration{Name: "Profile", Type: RecordType{}},
			DocDeclaration{Kind: DocumentationKindGeneral, Target: "Profile"},
		}

		for _, declaration := range declarations {
			declaration.declarationNode()
		}
	})

	It("invokes type reference node methods", func() {
		typeReferences := []TypeReference{
			PrimitiveType{Name: "string"},
			ArrayType{Element: PrimitiveType{Name: "string"}},
			RecordMapType{Value: PrimitiveType{Name: "string"}},
			UnionType{Members: []TypeReference{PrimitiveType{Name: "string"}, PrimitiveType{Name: "int"}}},
			VariantType{Members: []TypeReference{PrimitiveType{Name: "string"}, PrimitiveType{Name: "int"}}},
			ChoiceType{Members: []Expression{StringLiteral{Lexeme: "\"on\""}}},
			NamedType{Name: "Profile"},
			RecordType{Fields: []SchemaField{{Name: "name", Type: PrimitiveType{Name: "string"}}}},
		}

		for _, typeReference := range typeReferences {
			typeReference.typeReferenceNode()
		}
	})

	It("covers all AST node methods", func() {
		expressions := []Expression{Identifier{}, MemberAccess{}, ArrayAccess{}, StringLiteral{}, IntLiteral{}, FloatLiteral{}, HexIntLiteral{}, HexFloatLiteral{}, BooleanLiteral{}, NullLiteral{}, ArrayLiteral{}, RecordLiteral{}, PrefixExpression{}, InfixExpression{}, ConditionalExpression{}, SelfReference{}}
		for _, expression := range expressions {
			expression.expressionNode()
		}

		declarations := []Declaration{VariableDeclaration{}, TypeDeclaration{}, SchemaDeclaration{}, DocDeclaration{}}
		for _, declaration := range declarations {
			declaration.declarationNode()
		}

		types := []TypeReference{PrimitiveType{}, ArrayType{}, RecordMapType{}, UnionType{}, VariantType{}, ChoiceType{}, NamedType{}, RecordType{}}
		for _, typ := range types {
			typ.typeReferenceNode()
		}
	})

	It("returns the local name for imported identifiers", func() {
		withoutAlias := ImportedIdentifier{Name: "Remote"}
		tAssert.Equal("Remote", withoutAlias.LocalName())

		withAlias := ImportedIdentifier{Name: "Remote", Alias: "Local"}
		tAssert.Equal("Local", withAlias.LocalName())
	})
})
