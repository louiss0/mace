package ast

import (
	"testing"

	"github.com/louiss0/mace/internal/lexer"
)

func TestExpressionNodes(t *testing.T) {
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
}

func TestDeclarationNodes(t *testing.T) {
	declarations := []Declaration{
		VariableDeclaration{Name: "name", Type: PrimitiveType{Name: "string"}},
		TypeDeclaration{Name: "Name", Type: PrimitiveType{Name: "string"}},
		SchemaDeclaration{Name: "Profile", Type: RecordType{}},
		DocDeclaration{Kind: DocumentationKindGeneral, Target: "Profile"},
	}

	for _, declaration := range declarations {
		declaration.declarationNode()
	}
}

func TestTypeReferenceNodes(t *testing.T) {
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
}

func TestImportedIdentifierLocalName(t *testing.T) {
	withoutAlias := ImportedIdentifier{Name: "Remote"}
	if withoutAlias.LocalName() != "Remote" {
		t.Fatalf("expected imported name without alias")
	}

	withAlias := ImportedIdentifier{Name: "Remote", Alias: "Local"}
	if withAlias.LocalName() != "Local" {
		t.Fatalf("expected imported alias")
	}
}
