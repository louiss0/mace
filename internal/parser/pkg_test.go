package parser

import (
	"os"
	"path/filepath"
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

func parseFixtureFile(path string) (ast.File, error) {
	contents, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return ast.File{}, err
	}

	return parseFileInput(string(contents))
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

func requireMemberAccess(expression ast.Expression, targetName string, memberName string) ast.MemberAccess {
	access, ok := expression.(ast.MemberAccess)
	tAssert.True(ok)
	if !ok {
		return ast.MemberAccess{}
	}
	requireIdentifier(access.Target, targetName)
	tAssert.Equal(memberName, access.Name)
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
		Entry("member access", "Fruit.Apple", func(expression ast.Expression) {
			requireMemberAccess(expression, "Fruit", "Apple")
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

	DescribeTable("parses string and access expressions",
		func(input string, assertExpression func(ast.Expression)) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)
			assertExpression(expression)
		},
		Entry("single quoted string", "'Ada'", func(expression ast.Expression) {
			requireStringLiteral(expression, "'Ada'")
		}),
		Entry("block string", "\"\"\"Ada\nLovelace\"\"\"", func(expression ast.Expression) {
			requireStringLiteral(expression, "\"\"\"Ada\nLovelace\"\"\"")
		}),
		Entry("nested member access", "user.profile.name", func(expression ast.Expression) {
			outer, ok := expression.(ast.MemberAccess)
			tAssert.True(ok)
			if !ok {
				return
			}
			tAssert.Equal("name", outer.Name)
			inner, ok := outer.Target.(ast.MemberAccess)
			tAssert.True(ok)
			if !ok {
				return
			}
			tAssert.Equal("profile", inner.Name)
			requireIdentifier(inner.Target, "user")
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
type Name: string;
schema User: { name: string; age?: int; };
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

		It("ignores line and block comment content while parsing", func() {
			input := `from "base.mace" import User; /= trailing import comment
|===|
/= line comment before declarations
schema Profile: {
  /= line comment before field
  name: string; /= trailing line comment
  /= block comment before optional field =/
  age?: int; /= trailing line comment
};

/= block comment between declarations =/
Profile current = {
  name: "Ada"; /= trailing field comment
  /= comment before optional field =/
  age?: 30; /= trailing field comment
};
|===|
[output = data]
{
  /= line comment before output field
  result: current.name; /= trailing output comment
  profile: {
    /= line comment inside nested record
    age?: current.age; /= trailing nested comment
  }; /= trailing record comment
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.Len(file.Imports, 1) {
				tAssert.Equal("base.mace", file.Imports[0].Path.Lexeme[1:len(file.Imports[0].Path.Lexeme)-1])
				tAssert.Equal([]string{"User"}, file.Imports[0].Identifiers)
			}

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 2) {
				schemaDecl, ok := file.Script.Items[0].(ast.SchemaDeclaration)
				tAssert.True(ok)
				if ok && tAssert.Len(schemaDecl.Type.Fields, 2) {
					tAssert.Equal("name", schemaDecl.Type.Fields[0].Name)
					tAssert.Equal("age", schemaDecl.Type.Fields[1].Name)
					tAssert.True(schemaDecl.Type.Fields[1].Optional)
				}

				varDecl, ok := file.Script.Items[1].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("current", varDecl.Name)
				}
			}

			if tAssert.Len(file.Output.DataFields, 2) {
				tAssert.Equal("result", file.Output.DataFields[0].Name)
				tAssert.Equal("profile", file.Output.DataFields[1].Name)
			}
		})

		It("ignores vertical block comments that wrap script and output blocks", func() {
			input := `/=
|===|
type Hidden: string;
|===|
[output = data]
{
  hidden: "ignore me";
}
=/
|===|
string visible = "ok";
|===|
[output = data]
{
  result: visible;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				variable, ok := file.Script.Items[0].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("visible", variable.Name)
				}
			}
			if tAssert.Len(file.Output.DataFields, 1) {
				tAssert.Equal("result", file.Output.DataFields[0].Name)
			}
		})

		It("ignores vertical block comments that wrap imports", func() {
			input := `/=
from "./ignored.mace" import Ignored;
=/
from "./base.mace" import User;
/=
from "./also_ignored.mace" import AlsoIgnored;
=/
[output = data]
{
  result: 1;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.Len(file.Imports, 1) {
				tAssert.Equal("\"./base.mace\"", file.Imports[0].Path.Lexeme)
				tAssert.Equal([]string{"User"}, file.Imports[0].Identifiers)
			}
		})

		It("ignores vertical block comments around type and schema declarations", func() {
			input := `|===|
/=
type Hidden: string;
schema HiddenUser: {
  name: string;
};
=/
type Name: string;
schema User: {
  name: Name;
};
|===|
[output = data]
{
  result: "ok";
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 2) {
				typeDecl, ok := file.Script.Items[0].(ast.TypeDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("Name", typeDecl.Name)
				}

				schemaDecl, ok := file.Script.Items[1].(ast.SchemaDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("User", schemaDecl.Name)
				}
			}
		})

		It("ignores vertical block comments around enum declarations", func() {
			input := `|===|
/=
enum HiddenStatus: string {
  Hidden,
};
=/
enum Status: string {
  Ready,
};
Status status = Status.Ready;
|===|
[output = data]
{
  result: status;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 2) {
				enumDecl, ok := file.Script.Items[0].(ast.EnumDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("Status", enumDecl.Name)
				}

				varDecl, ok := file.Script.Items[1].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("status", varDecl.Name)
				}
			}
		})

		It("ignores vertical block comments around documentation declarations", func() {
			input := `|===|
schema User: {
  name: string;
};
/=
schema_doc User {
  summary: "Ignore this doc";
};
=/
schema_doc User {
  summary: "Visible doc";
};
|===|
[output = schema]
{
  user: User;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 2) {
				docDecl, ok := file.Script.Items[1].(ast.DocDeclaration)
				tAssert.True(ok)
				if ok && tAssert.NotNil(docDecl.Documentation.Summary) {
					tAssert.Equal("\"Visible doc\"", docDecl.Documentation.Summary.Lexeme)
				}
			}
		})

		It("ignores vertical block comments inside output fields", func() {
			input := `[output = data]
{
  subtotal: 129.99 * 3;
/=
  total: $self.subtotal * 1.08875;
=/
  result: $self.subtotal;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.Len(file.Output.DataFields, 2) {
				tAssert.Equal("subtotal", file.Output.DataFields[0].Name)
				tAssert.Equal("result", file.Output.DataFields[1].Name)
			}
		})

		It("rejects inline descriptions on variable declarations", func() {
			_, err := parseFileInput(`|===|
string greeting = "Hello $(name)" /# Rendered greeting;
|===|
[output = data] {}`)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "inline descriptions are not allowed on variable declarations")
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

		It("parses variant type references", func() {
			input := `|===|
type Value: variant[string, int];
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				typeDecl, ok := file.Script.Items[0].(ast.TypeDeclaration)
				tAssert.True(ok)
				if ok {
					variantType, ok := typeDecl.Type.(ast.VariantType)
					tAssert.True(ok)
					if ok && tAssert.Len(variantType.Members, 2) {
						_, firstIsPrimitive := variantType.Members[0].(ast.PrimitiveType)
						_, secondIsPrimitive := variantType.Members[1].(ast.PrimitiveType)
						tAssert.True(firstIsPrimitive)
						tAssert.True(secondIsPrimitive)
					}
				}
			}
		})

		It("parses union type references", func() {
			input := `|===|
type Value: union[Profile, Audit];
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				typeDecl, ok := file.Script.Items[0].(ast.TypeDeclaration)
				tAssert.True(ok)
				if ok {
					unionType, ok := typeDecl.Type.(ast.UnionType)
					tAssert.True(ok)
					if ok && tAssert.Len(unionType.Members, 2) {
						_, firstIsNamed := unionType.Members[0].(ast.NamedType)
						_, secondIsNamed := unionType.Members[1].(ast.NamedType)
						tAssert.True(firstIsNamed)
						tAssert.True(secondIsNamed)
					}
				}
			}
		})

		It("parses nested array type references without spacing between closers", func() {
			input := `|===|
type Matrix: array<array<int>>;
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
  Apple /# Default apple,
  Strawberry = "strawberry" /# Explicit strawberry
};
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
						tAssert.Equal("Default apple", enumDecl.Members[0].Description)
						tAssert.Equal("Strawberry", enumDecl.Members[1].Name)
						tAssert.True(enumDecl.Members[1].HasValue)
						tAssert.Equal("Explicit strawberry", enumDecl.Members[1].Description)
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

		It("parses inline descriptions before and after separators across schema, output, and record fields", func() {
			input := `|===|
schema User: {
  name: string /# Name before separator,
  age?: int, /# Age after separator
};
|===|
[output = data]
{
  user: {
    name: "Ada" /# Record name before separator,
    age?: 27, /# Record age after separator
  }, /# User record after separator
  greeting: "Hello" /# Greeting before separator
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				schemaDecl, ok := file.Script.Items[0].(ast.SchemaDeclaration)
				tAssert.True(ok)
				if ok && tAssert.Len(schemaDecl.Type.Fields, 2) {
					tAssert.Equal("Name before separator", schemaDecl.Type.Fields[0].Description)
					tAssert.Equal("Age after separator", schemaDecl.Type.Fields[1].Description)
				}
			}

			if tAssert.Len(file.Output.DataFields, 2) {
				tAssert.Equal("User record after separator", file.Output.DataFields[0].Description)
				tAssert.Equal("Greeting before separator", file.Output.DataFields[1].Description)

				record := requireRecordLiteral(file.Output.DataFields[0].Value, 2)
				tAssert.Equal("name", record.Fields[0].Name)
				tAssert.Equal("age", record.Fields[1].Name)
				tAssert.True(record.Fields[1].Optional)
			}
		})

		It("parses output schema field descriptions before and after separators", func() {
			file, err := parseFileInput(`[output = schema]
{
  name: string /# Name before separator,
  age?: int, /# Age after separator
}`)
			tAssert.NoError(err)

			if tAssert.Len(file.Output.SchemaFields, 2) {
				tAssert.Equal("Name before separator", file.Output.SchemaFields[0].Description)
				tAssert.Equal("Age after separator", file.Output.SchemaFields[1].Description)
			}
		})

		It("rejects duplicate inline descriptions on the same field", func() {
			_, err := parseFileInput(`[output = schema]
{
  name: string /# First description, /# Second description
}`)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "duplicate inline description on output schema field")
		})

		It("parses comma separators across declarations", func() {
			file, err := parseFileInput(`from "./shared.mace" import Name, User;
|===|
type Alias: string;
injectable string env;
schema User: {
  name: string,
};
gen_doc Alias {
  summary: "Alias docs.",
};
|===|
[output = data] {
  result: env,
}`)
			tAssert.NoError(err)
			if tAssert.NotNil(file.Script) {
				tAssert.Len(file.Script.Items, 4)
			}
		})

		It("parses output inline doc blocks", func() {
			input := `[output = schema]
"""
# Public User Output
"""
{
  name: string;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.NotNil(file.Output.Doc) {
				tAssert.Equal("\"\"\"\n# Public User Output\n\"\"\"", file.Output.Doc.Lexeme)
			}
		})

		It("parses documentation declarations", func() {
			input := `|===|
schema User: {
  name: string,
};

enum Status: string {
  Active,
};

schema_doc User {
  summary: "Represents a user.",
  description: """
# User
""",
};

schema_doc Status {
  summary: "Represents a status enum.",
};
|===|
[output = schema]
{ user: User, status: Status }`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 4) {
				docDecl, ok := file.Script.Items[2].(ast.DocDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal(ast.DocumentationKindSchema, docDecl.Kind)
					tAssert.Equal("User", docDecl.Target)
					if tAssert.NotNil(docDecl.Documentation.Summary) {
						tAssert.Equal("\"Represents a user.\"", docDecl.Documentation.Summary.Lexeme)
					}
					if tAssert.NotNil(docDecl.Documentation.Description) {
						tAssert.Equal("\"\"\"\n# User\n\"\"\"", docDecl.Documentation.Description.Lexeme)
					}
				}

				enumDoc, ok := file.Script.Items[3].(ast.DocDeclaration)
				tAssert.True(ok)
				if ok && tAssert.NotNil(enumDoc.Documentation.Summary) {
					tAssert.Equal("Status", enumDoc.Target)
					tAssert.Equal("\"Represents a status enum.\"", enumDoc.Documentation.Summary.Lexeme)
				}
			}
		})

		It("parses documentation fixtures with props and inline descriptions", func() {
			file, err := parseFixtureFile(filepath.Join("..", "analyzer", "testdata", "docs", "hover.mace"))
			tAssert.NoError(err)
			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 4) {
				docDecl, ok := file.Script.Items[3].(ast.DocDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.Equal(ast.DocumentationKindSchema, docDecl.Kind)
					if tAssert.NotNil(docDecl.Documentation.Summary) {
						tAssert.Equal("\"Represents a user\"", docDecl.Documentation.Summary.Lexeme)
					}
					if tAssert.NotNil(docDecl.Documentation.Description) {
						tAssert.Contains(docDecl.Documentation.Description.Lexeme, "Hover should surface this documentation")
					}
					if tAssert.Contains(docDecl.Documentation.Props, "name") {
						tAssert.Equal("\"The user's display name\"", docDecl.Documentation.Props["name"].Lexeme)
					}
				}

				schemaDecl, ok := file.Script.Items[0].(ast.SchemaDeclaration)
				tAssert.True(ok)
				if ok && tAssert.Len(schemaDecl.Type.Fields, 1) {
					tAssert.Empty(schemaDecl.Type.Fields[0].Description)
				}
			}

			if tAssert.NotNil(file.Output.Doc) {
				tAssert.Contains(file.Output.Doc.Lexeme, "# User Output")
			}
			if tAssert.Len(file.Output.SchemaFields, 1) {
				tAssert.Equal("Public user schema", file.Output.SchemaFields[0].Description)
			}
		})
	})
})
