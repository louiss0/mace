package formatter

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser"
	"github.com/louiss0/mace/internal/parser/ast"
)

var tAssert *assert.Assertions

func TestFormatter(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Formatter Suite")
}

func parseMaceFile(input string) (ast.File, error) {
	lexerInstance := lexer.New(input)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return ast.File{}, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			break
		}
	}

	return parser.New(tokens).ParseFile()
}

var _ = Describe("FormatFile", func() {
	DescribeTable("formats operator tokens",
		func(tokenType lexer.TokenType, lexeme string, precedence int) {
			tAssert.Equal(lexeme, tokenLexeme(tokenType))
			tAssert.Equal(precedence, infixPrecedence(tokenType))
		},
		Entry("plus", lexer.TokenPlus, "+", precedenceAdditive),
		Entry("minus", lexer.TokenMinus, "-", precedenceAdditive),
		Entry("star", lexer.TokenStar, "*", precedenceMultiplicative),
		Entry("slash", lexer.TokenSlash, "/", precedenceMultiplicative),
		Entry("percent", lexer.TokenPercent, "%", precedenceMultiplicative),
		Entry("double star", lexer.TokenDoubleStar, "**", precedenceExponent),
		Entry("less", lexer.TokenLess, "<", precedenceRelational),
		Entry("less equal", lexer.TokenLessEqual, "<=", precedenceRelational),
		Entry("merge", lexer.TokenMerge, "<>", precedenceMerge),
		Entry("greater", lexer.TokenGreater, ">", precedenceRelational),
		Entry("greater equal", lexer.TokenGreaterEqual, ">=", precedenceRelational),
		Entry("equal equal", lexer.TokenEqualEqual, "==", precedenceEquality),
		Entry("not equal", lexer.TokenNotEqual, "!=", precedenceEquality),
		Entry("ampersand", lexer.TokenAmpersand, "&", precedenceBitwiseAnd),
		Entry("caret", lexer.TokenCaret, "^", precedenceBitwiseXor),
		Entry("pipe", lexer.TokenPipe, "|", precedenceBitwiseOr),
		Entry("and and", lexer.TokenAndAnd, "&&", precedenceLogicalAnd),
		Entry("or or", lexer.TokenOrOr, "||", precedenceLogicalOr),
		Entry("shift left", lexer.TokenShiftLeft, "<<", precedenceShift),
		Entry("shift right", lexer.TokenShiftRight, ">>", precedenceShift),
		Entry("unsigned shift right", lexer.TokenShiftRightUnsigned, ">>>", precedenceShift),
	)

	DescribeTable("formats prefix-only tokens",
		func(tokenType lexer.TokenType, lexeme string) {
			tAssert.Equal(lexeme, tokenLexeme(tokenType))
			tAssert.Equal(precedenceLowest, infixPrecedence(tokenType))
		},
		Entry("bang", lexer.TokenBang, "!"),
		Entry("tilde", lexer.TokenTilde, "~"),
		Entry("unknown", lexer.TokenEOF, ""),
	)

	It("formats imports, script declarations, and output", func() {
		file, err := parseMaceFile(`|===|
from "./base.mace" import User, Config;
type Name: string;
type Fruit: choice["Apple", "strawberry"];
schema User: { name: string; age?: int; };
string user = "Ada";
|===|
[output = data, schema = User]
{ name: user; age: 1 + 2 * 3; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==========================================|
from "./base.mace" import User, Config;
type Name: string;
type Fruit: choice["Apple", "strawberry"];
schema User: {
  name: string,
  age?: int
}
string user = "Ada";
|==========================================|
[output = data, schema = User]
{
  name: user,
  age: 1 + 2 * 3
}`, output)
	})

	It("formats import aliases and optional output fields", func() {
		file, err := parseMaceFile(`|===|
from "./base.mace" import User:Person, Config;
|===|
[output = data]
{
  display_name?: "Ada" /# Optional display name;
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==============================================|
from "./base.mace" import User:Person, Config;
|==============================================|
[output = data]
{
  display_name?: "Ada" /# Optional display name
}`, output)
	})

	It("formats nullable declarations without initial values", func() {
		line, err := formatDeclaration(ast.VariableDeclaration{
			Nullable: true,
			Type:     ast.PrimitiveType{Name: "string"},
			Name:     "nickname",
		})

		tAssert.NoError(err)
		tAssert.Equal("nullable string nickname;", line)
	})

	It("formats import-as declarations", func() {
		file, err := parseMaceFile(`|===|
from "./base.mace" import-as Base;
|===|
[output = data]
{ result: Base.name; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==================================|
from "./base.mace" import-as Base;
|==================================|
[output = data]
{
  result: Base.name
}`, output)
	})

	It("formats empty data and schema output blocks", func() {
		dataFile, err := parseMaceFile(`[output = data] {}`)
		tAssert.NoError(err)

		dataOutput, err := FormatFile(dataFile)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{}`, dataOutput)

		schemaFile, err := parseMaceFile(`[output = schema] {}`)
		tAssert.NoError(err)

		schemaOutput, err := FormatFile(schemaFile)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{}`, schemaOutput)
	})

	It("formats record map type references", func() {
		file, err := parseMaceFile(`|===|
type Dependencies: record<string>;
record<string> deps = { foo: "bar"; };
|===|
[output = schema]
{ dependencies: record<string>; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==================================|
type Dependencies: record<string>;
record<string> deps = {
  foo: "bar"
};
|==================================|
[output = schema]
{
  dependencies: record<string>
}`, output)
	})

	It("formats all output directive kinds", func() {
		file, err := parseMaceFile(`[output = data, schema_file = "./schemas.mace", parse = Runtime, parse_file = "./input.mace"]
{ result: "ok"; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data, schema_file = "./schemas.mace", parse = Runtime, parse_file = "./input.mace"]
{
  result: "ok"
}`, output)
	})

	It("formats choice type declarations", func() {
		file, err := parseMaceFile(`|===|
 type Environment: choice["dev", "prod"];
 type Mode: choice[Environment, 1, true];
|===|
[output = data]
{ value: "dev"; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|========================================|
type Environment: choice["dev", "prod"];
type Mode: choice[Environment, 1, true];
|========================================|
[output = data]
{
  value: "dev"
}`, output)
	})

	It("formats documentation declarations with props", func() {
		file, err := parseMaceFile(`|===|
schema_doc User {
  summary: "User summary",
  props: {
    name: "Display name",
    age: "Age in years",
  };
};
|===|
[output = data] { result: "ok"; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==========================|
schema_doc User {
  summary: "User summary",
  props: {
    age: "Age in years",
    name: "Display name",
  };
};
|==========================|
[output = data]
{
  result: "ok"
}`, output)
	})

	It("formats script imports without duplicating flattened file imports", func() {
		file, err := parseMaceFile(`|===|
from "./shared.mace" import User;
string name = "Ada";
|===|
[output = data]
{ result: name; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|=================================|
from "./shared.mace" import User;
string name = "Ada";
|=================================|
[output = data]
{
  result: name
}`, output)
	})

	It("formats booleans, self references, prefixes, and comparisons", func() {
		file, err := parseMaceFile(`[output = data]
{
  enabled: true;
  disabled: false;
  current: $self.profile.name;
  inverse: !false;
  bits: ~1;
  comparison: 1 < 2 && 3 >= 2 || 4 != 5;
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  enabled: true,
  disabled: false,
  current: $self.profile.name,
  inverse: !false,
  bits: ~1,
  comparison: 1 < 2 && 3 >= 2 || 4 != 5
}`, output)
	})

	It("formats documentation declarations and inline output docs", func() {
		file, err := parseMaceFile(`|===|
schema User: { name: string; };
schema_doc User {
  summary: "Represents a user.",
  description: """
# User
""",
};
|===|
[output = schema]
"""
# Public User Output
"""
{ user: User; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|================================|
schema User: {
  name: string
}
schema_doc User {
  summary: "Represents a user.",
  description: """
# User
""",
};
|================================|
[output = schema]
"""
# Public User Output
"""
{
  user: User
}`, output)
	})

	It("preserves expression semantics with parentheses", func() {
		file, err := parseMaceFile(`[output = data] { result: (1 + 2) * (3 - 4 ? 5 : 6); }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  result: (1 + 2) * (3 - 4 ? 5 : 6)
}`, output)
	})

	It("formats array access expressions", func() {
		file, err := parseMaceFile(`[output = data] { result: users [ 0 ] . name; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  result: users[0].name
}`, output)
	})

	It("formats bare output blocks without injecting a directive", func() {
		file, err := parseMaceFile(`{ result: 1 + 2; }`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`{
  result: 1 + 2
}`, output)
	})

	It("keeps arrays and nested records expanded instead of collapsing them", func() {
		file, err := parseMaceFile(`[output = data]
{
  result: [{ profile: { name: "Ada"; }; }, { profile: { name: "Bob"; }; }];
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = data]
{
  result: [
    {
      profile: {
        name: "Ada"
      }
    },
    {
      profile: {
        name: "Bob"
      }
    }
  ]
}`, output)
	})

	It("formats schema-mode output blocks with type references", func() {
		file, err := parseMaceFile(`[output = schema]
{
  name: string;
  tags?: array<string>;
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`[output = schema]
{
  name: string,
  tags?: array<string>
}`, output)
	})

	It("formats variant type references", func() {
		file, err := parseMaceFile(`|===|
type Value: variant[string, int];
|===|
[output = schema]
{
  value: variant[string, int];
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|=================================|
type Value: variant[string, int];
|=================================|
[output = schema]
{
  value: variant[string, int]
}`, output)
	})

	It("formats hexadecimal literals and primitive types", func() {
		file, err := parseMaceFile(`|===|
hex_int mask = 0x00ff;
hex_float ratio = 0x02.80;
|===|
[output = data]
{
  mask: mask;
  ratio: ratio;
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==========================|
hex_int mask = 0x00ff;
hex_float ratio = 0x02.80;
|==========================|
[output = data]
{
  mask: mask,
  ratio: ratio
}`, output)
	})

	It("formats union type references", func() {
		file, err := parseMaceFile(`|===|
type Value: union[Profile, Audit];
|===|
[output = schema]
{
  value: union[Profile, Audit];
}`)
		tAssert.NoError(err)

		output, err := FormatFile(file)
		tAssert.NoError(err)
		tAssert.Equal(`|==================================|
type Value: union[Profile, Audit];
|==================================|
[output = schema]
{
  value: union[Profile, Audit]
}`, output)
	})

	It("formats flattened file imports when no script block is present", func() {
		output, err := FormatFile(ast.File{
			Imports: []ast.ImportDeclaration{{
				Path:        ast.StringLiteral{Lexeme: `"./base.mace"`},
				Identifiers: []ast.ImportedIdentifier{{Name: "User"}},
			}},
			Output: ast.OutputBlock{Mode: ast.OutputModeData},
		})

		tAssert.NoError(err)
		tAssert.Equal(`|===============================|
from "./base.mace" import User;
|===============================|
{}`, output)
	})

	It("returns errors for malformed formatter AST inputs", func() {
		_, err := FormatFile(ast.File{
			Script: &ast.ScriptBlock{
				Items: []ast.Declaration{nil},
			},
		})
		tAssert.ErrorContains(err, "format declaration")

		_, err = FormatFile(ast.File{
			Output: ast.OutputBlock{
				Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveKind(99)}},
			},
		})
		tAssert.ErrorContains(err, "format output directive")

		_, err = FormatFile(ast.File{
			Output: ast.OutputBlock{
				DataFields: []ast.OutputField{{
					Name:  "value",
					Value: ast.NullLiteral{},
				}},
			},
		})
		tAssert.ErrorContains(err, "format expression")

		_, err = FormatFile(ast.File{
			Output: ast.OutputBlock{
				Mode: ast.OutputModeSchema,
				SchemaFields: []ast.OutputSchemaField{{
					Name: "value",
				}},
			},
		})
		tAssert.ErrorContains(err, "format type reference")
	})

	It("returns errors for malformed declarations", func() {
		_, err := formatDeclaration(ast.VariableDeclaration{
			Type: ast.ArrayType{},
			Name: "value",
		})
		tAssert.ErrorContains(err, "format type reference")

		_, err = formatDeclaration(ast.VariableDeclaration{
			HasValue: true,
			Type:     ast.PrimitiveType{Name: "string"},
			Name:     "value",
			Value:    ast.NullLiteral{},
		})
		tAssert.ErrorContains(err, "format expression")

		_, err = formatDeclaration(ast.TypeDeclaration{
			Name: "Value",
			Type: ast.ArrayType{},
		})
		tAssert.ErrorContains(err, "format type reference")

		_, err = formatDeclaration(ast.SchemaDeclaration{
			Name: "User",
			Type: ast.RecordType{Fields: []ast.SchemaField{{
				Name: "value",
			}}},
		})
		tAssert.ErrorContains(err, "format type reference")
	})

	DescribeTable("returns errors for malformed type references",
		func(typeReference ast.TypeReference) {
			_, err := formatTypeReference(typeReference)

			tAssert.ErrorContains(err, "format type reference")
		},
		Entry("nil type reference", nil),
		Entry("array element", ast.ArrayType{}),
		Entry("record map value", ast.RecordMapType{}),
		Entry("union member", ast.UnionType{Members: []ast.TypeReference{nil}}),
		Entry("variant member", ast.VariantType{Members: []ast.TypeReference{nil}}),
	)

	It("returns errors for malformed choice type members", func() {
		_, err := formatTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.NullLiteral{}}})

		tAssert.ErrorContains(err, "format expression")
	})

	It("formats empty and described record types directly", func() {
		output, err := formatRecordType(ast.RecordType{}, 0)
		tAssert.NoError(err)
		tAssert.Equal("{}", output)

		output, err = formatTypeReference(ast.RecordType{})
		tAssert.NoError(err)
		tAssert.Equal("{}", output)

		output, err = formatRecordType(ast.RecordType{Fields: []ast.SchemaField{{
			Name:        "name",
			Optional:    true,
			Type:        ast.PrimitiveType{Name: "string"},
			Description: "Display name",
		}}}, 0)
		tAssert.NoError(err)
		tAssert.Equal(`{
  name?: string /# Display name
}`, output)
	})

	It("formats single-line literals and exponent expressions directly", func() {
		output, err := formatExpressionWithDepth(ast.ArrayLiteral{
			Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}},
		}, 0)
		tAssert.NoError(err)
		tAssert.Equal("[1]", output)

		output, err = formatExpressionWithDepth(ast.ArrayLiteral{}, 0)
		tAssert.NoError(err)
		tAssert.Equal("[]", output)

		output, err = formatExpressionWithDepth(ast.RecordLiteral{}, 0)
		tAssert.NoError(err)
		tAssert.Equal("{}", output)

		output, err = formatExpressionWithDepth(ast.FloatLiteral{Lexeme: "1.5"}, 0)
		tAssert.NoError(err)
		tAssert.Equal("1.5", output)

		output, err = formatExpressionWithDepth(ast.InfixExpression{
			Left:     ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "2"}, Operator: lexer.TokenDoubleStar, Right: ast.IntLiteral{Lexeme: "3"}},
			Operator: lexer.TokenDoubleStar,
			Right:    ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "4"}, Operator: lexer.TokenDoubleStar, Right: ast.IntLiteral{Lexeme: "5"}},
		}, 0)
		tAssert.NoError(err)
		tAssert.Equal("(2 ** 3) ** 4 ** 5", output)
	})

	It("returns errors for malformed expression children", func() {
		expressions := []ast.Expression{
			ast.MemberAccess{Target: ast.NullLiteral{}, Name: "value"},
			ast.ArrayAccess{Target: ast.NullLiteral{}, Index: ast.IntLiteral{Lexeme: "0"}},
			ast.ArrayLiteral{Elements: []ast.Expression{ast.NullLiteral{}}},
			ast.RecordLiteral{Fields: []ast.RecordField{{Name: "value", Value: ast.NullLiteral{}}}},
			ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.NullLiteral{}},
			ast.InfixExpression{Left: ast.NullLiteral{}, Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}},
			ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "1"}, Operator: lexer.TokenPlus, Right: ast.NullLiteral{}},
			ast.ConditionalExpression{Condition: ast.NullLiteral{}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.IntLiteral{Lexeme: "2"}},
			ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.IntLiteral{Lexeme: "2"}},
			ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.NullLiteral{}},
		}

		for _, expression := range expressions {
			_, err := formatExpressionWithDepth(expression, 0)
			tAssert.ErrorContains(err, "format expression")
		}
	})

	It("formats optional record literal fields directly", func() {
		output, err := formatExpressionWithDepth(ast.RecordLiteral{
			Fields: []ast.RecordField{{
				Name:     "name",
				Optional: true,
				Value:    ast.StringLiteral{Lexeme: `"Ada"`},
			}},
		}, 0)

		tAssert.NoError(err)
		tAssert.Equal(`{
  name?: "Ada"
}`, output)
	})
})
