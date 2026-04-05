package parser

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
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

func parseFileInput(input string) (ast.File, error) {
	tokens, err := lexInput(input)
	if err != nil {
		return ast.File{}, err
	}

	return New(tokens).ParseFile()
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

func requireEnumMemberAccess(expression ast.Expression, enumName string, memberName string) ast.EnumMemberAccess {
	access, ok := expression.(ast.EnumMemberAccess)
	tAssert.True(ok)
	if !ok {
		return ast.EnumMemberAccess{}
	}
	tAssert.Equal(enumName, access.EnumName)
	tAssert.Equal(memberName, access.MemberName)
	return access
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

func requireStringLiteral(expression ast.Expression, lexeme string) ast.StringLiteral {
	literal, ok := expression.(ast.StringLiteral)
	tAssert.True(ok)
	if !ok {
		return ast.StringLiteral{}
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

func requireArrayLiteral(expression ast.Expression, length int) ast.ArrayLiteral {
	array, ok := expression.(ast.ArrayLiteral)
	tAssert.True(ok)
	if !ok {
		return ast.ArrayLiteral{}
	}
	tAssert.Len(array.Elements, length)
	return array
}

func requireRecordLiteral(expression ast.Expression, length int) ast.RecordLiteral {
	record, ok := expression.(ast.RecordLiteral)
	tAssert.True(ok)
	if !ok {
		return ast.RecordLiteral{}
	}
	tAssert.Len(record.Fields, length)
	return record
}

func requireSelfReference(expression ast.Expression, path []string) ast.SelfReference {
	selfRef, ok := expression.(ast.SelfReference)
	tAssert.True(ok)
	if !ok {
		return ast.SelfReference{}
	}
	tAssert.Equal(path, selfRef.Path)
	return selfRef
}

func requireEnumMemberValue(member ast.EnumMember, lexeme string) {
	literal, ok := member.Value.(ast.StringLiteral)
	if ok {
		tAssert.Equal(lexeme, literal.Lexeme)
		return
	}

	intLiteral, ok := member.Value.(ast.IntLiteral)
	tAssert.True(ok)
	if ok {
		tAssert.Equal(lexeme, intLiteral.Lexeme)
	}
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
		Entry("enum member access", "Fruit.Apple", func(expression ast.Expression) {
			requireEnumMemberAccess(expression, "Fruit", "Apple")
		}),
		Entry("int literal", "42", func(expression ast.Expression) {
			requireIntLiteral(expression, "42")
		}),
	)

	DescribeTable("parses collection literals",
		func(input string, assertExpression func(ast.Expression)) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)
			assertExpression(expression)
		},
		Entry("array literal", "[1, 2, 3]", func(expression ast.Expression) {
			array := requireArrayLiteral(expression, 3)
			requireIntLiteral(array.Elements[0], "1")
			requireIntLiteral(array.Elements[1], "2")
			requireIntLiteral(array.Elements[2], "3")
		}),
		Entry("record literal", "{ name?: \"Ada\"; }", func(expression ast.Expression) {
			record := requireRecordLiteral(expression, 1)
			tAssert.Equal("name", record.Fields[0].Name)
			tAssert.True(record.Fields[0].Optional)
			requireStringLiteral(record.Fields[0].Value, "\"Ada\"")
		}),
	)

	DescribeTable("parses self references",
		func(input string, expected []string) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)
			requireSelfReference(expression, expected)
		},
		Entry("self reference chain", "$self.user.name", []string{"user", "name"}),
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

	Describe("parses a full file", func() {
		It("parses imports, script block, and output block", func() {
			input := `from "base.mace" import User, Config;
|===|
type Name = string;
schema User = { name: string; age?: int; };
string user = "Ada";
|===|
[output = data, schema = User]
{ name: user; }`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.Len(file.Imports, 1) {
				tAssert.Equal("\"base.mace\"", file.Imports[0].Path.Lexeme)
				tAssert.Equal([]string{"User", "Config"}, file.Imports[0].Identifiers)
			}

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 3) {
				_, ok := file.Script.Items[0].(ast.TypeDeclaration)
				tAssert.True(ok)

				schemaDecl, ok := file.Script.Items[1].(ast.SchemaDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("User", schemaDecl.Name)
					tAssert.Equal(4, schemaDecl.NameToken.Line)
					tAssert.Equal(8, schemaDecl.NameToken.Column)
					if tAssert.Len(schemaDecl.Type.Fields, 2) {
						tAssert.Equal("name", schemaDecl.Type.Fields[0].Name)
						tAssert.False(schemaDecl.Type.Fields[0].Optional)
						tAssert.Equal("age", schemaDecl.Type.Fields[1].Name)
						tAssert.True(schemaDecl.Type.Fields[1].Optional)
					}
				}

				varDecl, ok := file.Script.Items[2].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.False(varDecl.Injectable)
					tAssert.Equal("user", varDecl.Name)
					tAssert.Equal(5, varDecl.NameToken.Line)
					tAssert.Equal(8, varDecl.NameToken.Column)
					requireStringLiteral(varDecl.Value, "\"Ada\"")
				}
			}

			if tAssert.Len(file.Output.Directives, 2) {
				tAssert.Equal(ast.OutputDirectiveOutput, file.Output.Directives[0].Kind)
				tAssert.Equal("data", file.Output.Directives[0].Value)
				tAssert.Equal(ast.OutputDirectiveSchema, file.Output.Directives[1].Kind)
				tAssert.Equal("User", file.Output.Directives[1].Value)
			}

			tAssert.Equal(ast.OutputModeData, file.Output.Mode)
			if tAssert.Len(file.Output.DataFields, 1) {
				tAssert.Equal("name", file.Output.DataFields[0].Name)
				tAssert.Equal(8, file.Output.DataFields[0].NameToken.Line)
				tAssert.Equal(3, file.Output.DataFields[0].NameToken.Column)
				requireIdentifier(file.Output.DataFields[0].Value, "user")
			}
			tAssert.Empty(file.Output.SchemaFields)
		})

		It("parses injectable declarations without an initializer", func() {
			input := `|===|
injectable string env;
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				varDecl, ok := file.Script.Items[0].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.True(varDecl.Injectable)
					tAssert.Equal("env", varDecl.Name)
					tAssert.False(varDecl.HasValue)
					tAssert.Nil(varDecl.Value)
				}
			}
		})

		It("parses nested array type references without spacing between closers", func() {
			input := `|===|
type Matrix = array<array<int>>;
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				typeDecl, ok := file.Script.Items[0].(ast.TypeDeclaration)
				tAssert.True(ok)
				if ok {
					outerArray, ok := typeDecl.Type.(ast.ArrayType)
					tAssert.True(ok)
					if ok {
						innerArray, ok := outerArray.Element.(ast.ArrayType)
						tAssert.True(ok)
						if ok {
							primitive, ok := innerArray.Element.(ast.PrimitiveType)
							tAssert.True(ok)
							if ok {
								tAssert.Equal("int", primitive.Name)
							}
						}
					}
				}
			}
		})

		It("parses enum declarations with implicit and explicit members", func() {
			input := `|===|
enum Fruit: string {
  Apple,
  Strawberry = "strawberry",
}
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				enumDecl, ok := file.Script.Items[0].(ast.EnumDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("Fruit", enumDecl.Name)
					tAssert.Equal("string", enumDecl.BackingType.Name)
					if tAssert.Len(enumDecl.Members, 2) {
						tAssert.Equal("Apple", enumDecl.Members[0].Name)
						tAssert.False(enumDecl.Members[0].HasValue)
						tAssert.Equal("Strawberry", enumDecl.Members[1].Name)
						tAssert.True(enumDecl.Members[1].HasValue)
						requireEnumMemberValue(enumDecl.Members[1], "\"strawberry\"")
					}
				}
			}
		})

		It("parses a bare output block as default data output", func() {
			file, err := parseFileInput(`{ result: 1 + 2; }`)
			tAssert.NoError(err)
			tAssert.Empty(file.Output.Directives)
			tAssert.Equal(ast.OutputModeData, file.Output.Mode)
			if tAssert.Len(file.Output.DataFields, 1) {
				tAssert.Equal("result", file.Output.DataFields[0].Name)
				tAssert.Equal(1, file.Output.DataFields[0].NameToken.Line)
				tAssert.Equal(3, file.Output.DataFields[0].NameToken.Column)
			}
		})

		It("parses schema-mode output blocks as schema fields", func() {
			file, err := parseFileInput(`[output = schema]
{
  name: string;
  age?: int;
}`)
			tAssert.NoError(err)

			tAssert.Equal(ast.OutputModeSchema, file.Output.Mode)
			tAssert.Empty(file.Output.DataFields)
			if tAssert.Len(file.Output.SchemaFields, 2) {
				tAssert.Equal("name", file.Output.SchemaFields[0].Name)
				tAssert.False(file.Output.SchemaFields[0].Optional)
				tAssert.Equal(3, file.Output.SchemaFields[0].NameToken.Line)
				tAssert.Equal(3, file.Output.SchemaFields[0].NameToken.Column)

				nameType, ok := file.Output.SchemaFields[0].Type.(ast.PrimitiveType)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("string", nameType.Name)
				}

				tAssert.Equal("age", file.Output.SchemaFields[1].Name)
				tAssert.True(file.Output.SchemaFields[1].Optional)
				tAssert.Equal(4, file.Output.SchemaFields[1].NameToken.Line)
				tAssert.Equal(3, file.Output.SchemaFields[1].NameToken.Column)

				ageType, ok := file.Output.SchemaFields[1].Type.(ast.PrimitiveType)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("int", ageType.Name)
				}
			}
		})
	})
})
