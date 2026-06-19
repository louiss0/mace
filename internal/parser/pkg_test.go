package parser

import (
	"fmt"
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

func requireArrayAccess(expression ast.Expression, expectedIndex string) ast.ArrayAccess {
	access, ok := expression.(ast.ArrayAccess)
	tAssert.True(ok)
	if !ok {
		return ast.ArrayAccess{}
	}
	tAssert.Equal(expectedIndex, access.Index.Lexeme)
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

func requireHexIntLiteral(expression ast.Expression, lexeme string) ast.HexIntLiteral {
	literal, ok := expression.(ast.HexIntLiteral)
	tAssert.True(ok)
	if !ok {
		return ast.HexIntLiteral{}
	}
	tAssert.Equal(lexeme, literal.Lexeme)
	return literal
}

func requireHexFloatLiteral(expression ast.Expression, lexeme string) ast.HexFloatLiteral {
	literal, ok := expression.(ast.HexFloatLiteral)
	tAssert.True(ok)
	if !ok {
		return ast.HexFloatLiteral{}
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
		Entry("array access", "names[0]", func(expression ast.Expression) {
			access := requireArrayAccess(expression, "0")
			requireIdentifier(access.Target, "names")
		}),
		Entry("int literal", "42", func(expression ast.Expression) {
			requireIntLiteral(expression, "42")
		}),
		Entry("hex int literal", "0xFF", func(expression ast.Expression) {
			requireHexIntLiteral(expression, "0xFF")
		}),
		Entry("hex float literal", "0x2.8", func(expression ast.Expression) {
			requireHexFloatLiteral(expression, "0x2.8")
		}),
		Entry("float literal", "3.14", func(expression ast.Expression) {
			literal, ok := expression.(ast.FloatLiteral)
			tAssert.True(ok)
			if ok {
				tAssert.Equal("3.14", literal.Lexeme)
			}
		}),
		Entry("boolean literal", "true", func(expression ast.Expression) {
			literal, ok := expression.(ast.BooleanLiteral)
			tAssert.True(ok)
			if ok {
				tAssert.True(literal.Value)
			}
		}),
		Entry("null literal", "null", func(expression ast.Expression) {
			_, ok := expression.(ast.NullLiteral)
			tAssert.True(ok)
		}),
		Entry("record keyword identifier", "record", func(expression ast.Expression) {
			requireIdentifier(expression, "record")
		}),
	)

	It("rejects trailing tokens after an expression", func() {
		_, err := parseExpressionInput("{ a: 1; } garbage")
		tAssert.ErrorContains(err, "unexpected token after expression")
	})

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
		Entry("array access with member access", "users[0].name", func(expression ast.Expression) {
			outer, ok := expression.(ast.MemberAccess)
			tAssert.True(ok)
			if !ok {
				return
			}
			tAssert.Equal("name", outer.Name)
			inner := requireArrayAccess(outer.Target, "0")
			requireIdentifier(inner.Target, "users")
		}),
	)

	DescribeTable("parses nested variable array access by depth",
		func(input string, expectedDepth int) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)

			current := expression
			for depth := expectedDepth; depth >= 1; depth-- {
				access := requireArrayAccess(current, "0")
				current = access.Target
			}

			requireIdentifier(current, "matrix")
		},
		Entry("level 1", "matrix[0]", 1),
		Entry("level 2", "matrix[0][0]", 2),
		Entry("level 3", "matrix[0][0][0]", 3),
		Entry("level 4", "matrix[0][0][0][0]", 4),
		Entry("level 5", "matrix[0][0][0][0][0]", 5),
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
		Entry("plus identifier", "+value", lexer.TokenPlus, "value"),
		Entry("bang identifier", "!value", lexer.TokenBang, "value"),
		Entry("tilde identifier", "~value", lexer.TokenTilde, "value"),
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

	DescribeTable("parses merge expressions",
		func(input string) {
			expression, err := parseExpressionInput(input)
			tAssert.NoError(err)

			root := requireInfix(expression, lexer.TokenMerge)
			requireIdentifier(root.Left, "base")
			requireIdentifier(root.Right, "override")
		},
		Entry("structural merge", "base <> override"),
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
		Entry("array access requires integer index", "names[value]"),
		Entry("array access requires closing bracket", "names[0"),
		Entry("merge rejects scalar left operand", "1 <> right"),
		Entry("merge rejects scalar right operand", "left <> 2"),
		Entry("merge rejects member access operand", "base.value <> override"),
		Entry("self reference requires dot", "$self"),
		Entry("self reference requires first identifier", "$self."),
		Entry("self reference requires later identifier", "$self.user."),
		Entry("array literal rejects trailing comma", "[1,]"),
		Entry("record literal requires field separator", "{ name \"Ada\" }"),
		Entry("conditional requires colon", "ready ? yes"),
		Entry("conditional requires then expression", "ready ? : no"),
		Entry("conditional requires else expression", "ready ? yes :"),
		Entry("member access requires identifier", "user."),
		Entry("array literal requires close", "[1"),
		Entry("record literal requires close", "{ name: \"Ada\";"),
		Entry("prefix expression requires operand", "!"),
		Entry("infix expression requires right operand", "1 +"),
		Entry("grouped expression rejects missing expression", "()"),
	)

	It("parses chained merge expressions", func() {
		expression, err := parseExpressionInput("base <> middle <> override")
		tAssert.NoError(err)

		root := requireInfix(expression, lexer.TokenMerge)
		requireIdentifier(root.Right, "override")
		left := requireInfix(root.Left, lexer.TokenMerge)
		requireIdentifier(left.Left, "base")
		requireIdentifier(left.Right, "middle")
	})

	It("covers parser helper edge branches directly", func() {
		tAssert.True(New(nil).isAtEnd())
		tAssert.Equal(lexer.TokenEOF, New(nil).current().Type)

		tokens, err := lexInput("name")
		tAssert.NoError(err)
		parser := New(tokens)
		parser.position = len(tokens)
		tAssert.Equal(lexer.TokenEOF, parser.current().Type)

		tAssert.Equal(precedenceLowest, parser.precedenceFor(lexer.TokenEOF))

		_, err = New(tokens).parseConditionalExpression(ast.Identifier{Name: "ready"}, lexer.Token{Type: lexer.TokenPlus})
		tAssert.ErrorContains(err, "expected '?'")

		_, err = New(tokens).parseArrayLiteral()
		tAssert.ErrorContains(err, "expected '['")

		_, err = New(tokens).parseRecordLiteral()
		tAssert.ErrorContains(err, "expected '{'")

		_, err = New(tokens).parseSelfReference()
		tAssert.ErrorContains(err, "expected '$self'")

		_, err = New(tokens).parseChoiceType()
		tAssert.ErrorContains(err, "expected 'choice'")

		_, err = New(tokens).parseRecordType()
		tAssert.ErrorContains(err, "expected '{'")

		recordTypeTokens, err := lexInput(`{ name: string;`)
		tAssert.NoError(err)
		_, err = New(recordTypeTokens).parseRecordType()
		tAssert.ErrorContains(err, "expected '}' to close record type")

		_, err = New(tokens).parseOutputDirective()
		tAssert.ErrorContains(err, "expected '['")

		_, err = New(tokens).parseTypeDeclaration()
		tAssert.ErrorContains(err, "expected 'type'")

		_, err = New(tokens).parseSchemaDeclaration()
		tAssert.ErrorContains(err, "expected 'schema'")

		_, err = New(tokens).parseDocDeclaration(ast.DocumentationKindGeneral, lexer.TokenGenDoc, "gen_doc")
		tAssert.ErrorContains(err, "expected 'gen_doc'")

		_, err = New(tokens).parseDirectivePair()
		tAssert.ErrorContains(err, "expected directive pair")

		_, err = New(tokens).parseScriptBlock()
		tAssert.ErrorContains(err, "expected script delimiter")
	})

	It("covers parser separator helpers directly", func() {
		commaTokens, err := lexInput(",")
		tAssert.NoError(err)
		tAssert.NoError(New(commaTokens).consumePairSeparator("entry"))

		nameTokens, err := lexInput("name")
		tAssert.NoError(err)
		tAssert.ErrorContains(New(nameTokens).consumePairSeparator("entry"), "expected ',' after entry")
		tAssert.ErrorContains(New(nameTokens).consumeRecordSeparator("field"), "expected ',' after field")

		descriptionTokens, err := lexInput("name")
		tAssert.NoError(err)
		_, _, err = New(descriptionTokens).consumeRecordSeparatorWithInlineDescription("field")
		tAssert.ErrorContains(err, "expected ',' after field")

		tAssert.False(isFieldNameToken(lexer.TokenEOF))
	})

	It("covers parser precedence branches directly", func() {
		parser := New(nil)

		cases := map[lexer.TokenType]int{
			lexer.TokenQuestion:           precedenceTernary,
			lexer.TokenOrOr:               precedenceOr,
			lexer.TokenAndAnd:             precedenceAnd,
			lexer.TokenPipe:               precedenceBitwiseOr,
			lexer.TokenCaret:              precedenceBitwiseXor,
			lexer.TokenAmpersand:          precedenceBitwiseAnd,
			lexer.TokenEqualEqual:         precedenceEquality,
			lexer.TokenNotEqual:           precedenceEquality,
			lexer.TokenMerge:              precedenceMerge,
			lexer.TokenLess:               precedenceRelational,
			lexer.TokenLessEqual:          precedenceRelational,
			lexer.TokenGreater:            precedenceRelational,
			lexer.TokenGreaterEqual:       precedenceRelational,
			lexer.TokenIn:                 precedenceRelational,
			lexer.TokenShiftLeft:          precedenceShift,
			lexer.TokenShiftRight:         precedenceShift,
			lexer.TokenShiftRightUnsigned: precedenceShift,
			lexer.TokenPlus:               precedenceAdditive,
			lexer.TokenMinus:              precedenceAdditive,
			lexer.TokenStar:               precedenceMultiplicative,
			lexer.TokenSlash:              precedenceMultiplicative,
			lexer.TokenPercent:            precedenceMultiplicative,
			lexer.TokenDoubleStar:         precedenceExponent,
			lexer.TokenDot:                precedenceMember,
			lexer.TokenLBracket:           precedenceMember,
			lexer.TokenEOF:                precedenceLowest,
		}

		for tokenType, precedence := range cases {
			tAssert.Equal(precedence, parser.precedenceFor(tokenType))
		}
	})

	It("covers parser import helper errors directly", func() {
		cases := []struct {
			input string
			call  func(*Parser) error
		}{
			{`name`, func(p *Parser) error {
				_, err := p.parseImportDeclaration()
				return err
			}},
			{`from "./base.mace" name`, func(p *Parser) error {
				_, err := p.parseImportDeclaration()
				return err
			}},
			{`from "./base.mace" import-`, func(p *Parser) error {
				_, err := p.parseImportDeclaration()
				return err
			}},
			{`from "./base.mace" import-as Base`, func(p *Parser) error {
				_, err := p.parseImportDeclaration()
				return err
			}},
			{`from "./base.mace" import User:`, func(p *Parser) error {
				_, err := p.parseImportDeclaration()
				return err
			}},
			{`from "./base.mace" import User, Config:`, func(p *Parser) error {
				_, err := p.parseImportDeclaration()
				return err
			}},
			{`from "./base.mace" import ;`, func(p *Parser) error {
				_, err := p.parseImportDeclaration()
				return err
			}},
		}

		for _, item := range cases {
			tokens, err := lexInput(item.input)
			tAssert.NoError(err)
			tAssert.Error(item.call(New(tokens)))
		}

		tokens, err := lexInput(`from "./base.mace" import User |===|`)
		tAssert.NoError(err)
		_, err = New(tokens).parseImportDeclaration()
		tAssert.NoError(err)
	})

	It("covers parser declaration helper errors directly", func() {
		cases := []struct {
			input string
			call  func(*Parser) error
		}{
			{`string name =`, func(p *Parser) error {
				_, err := p.parseVariableDeclaration()
				return err
			}},
			{`string name;`, func(p *Parser) error {
				_, err := p.parseVariableDeclaration()
				return err
			}},
			{`string name = "Ada"`, func(p *Parser) error {
				_, err := p.parseVariableDeclaration()
				return err
			}},
			{`type Name: string`, func(p *Parser) error {
				_, err := p.parseTypeDeclaration()
				return err
			}},
			{`schema User: { name: string`, func(p *Parser) error {
				_, err := p.parseSchemaDeclaration()
				return err
			}},
			{`gen_doc User { summary "User"; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindGeneral, lexer.TokenGenDoc, "gen_doc")
				return err
			}},
			{`gen_doc User { summary: 1; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindGeneral, lexer.TokenGenDoc, "gen_doc")
				return err
			}},
			{`gen_doc User { summary: "User" };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindGeneral, lexer.TokenGenDoc, "gen_doc")
				return err
			}},
			{`schema_doc User { props: { name "Name"; }; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
			{`schema_doc User { props: "Name"; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
			{`schema_doc User { props: { name: "Name" }; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
			{`schema_doc User { props: { name: "Name"; ;`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
			{`schema_doc User { props: { name: "Name";`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
			{`schema_doc User { props: { name: "Name"; }; }`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
			{`gen_doc User { ; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindGeneral, lexer.TokenGenDoc, "gen_doc")
				return err
			}},
			{`schema_doc User { props: { ; }; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
			{`schema_doc User { props: { name: "Name"; };`, func(p *Parser) error {
				_, err := p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
				return err
			}},
		}

		for _, item := range cases {
			tokens, err := lexInput(item.input)
			tAssert.NoError(err)
			tAssert.Error(item.call(New(tokens)))
		}
	})

	It("covers parser type reference helper errors directly", func() {
		cases := []string{
			`array<`,
			`record<`,
			`record<string`,
			`union[`,
			`union[string`,
			`variant`,
			`variant[string`,
			`choice[string`,
			`choice["a"`,
		}

		for _, input := range cases {
			tokens, err := lexInput(input)
			tAssert.NoError(err)
			_, err = New(tokens).parseTypeReference()
			tAssert.Error(err)
		}
	})

	It("covers parser directive helper errors directly", func() {
		cases := []string{
			`output data`,
			`schema_file "./schema.mace"`,
			`parse Runtime`,
			`parse_file "./runtime.mace"`,
			`schema User`,
			`[output = data, ]`,
			`[output = data`,
		}

		for _, input := range cases {
			tokens, err := lexInput(input)
			tAssert.NoError(err)
			if tokens[0].Type == lexer.TokenLBracket {
				_, err = New(tokens).parseOutputDirective()
			} else {
				_, err = New(tokens).parseDirectivePair()
			}
			tAssert.Error(err)
		}
	})

	It("covers parser field helper errors directly", func() {
		cases := []struct {
			input string
			call  func(*Parser) error
		}{
			{`name:`, func(p *Parser) error {
				_, err := p.parseOutputField()
				return err
			}},
			{`name: "Ada" /# first, /# second`, func(p *Parser) error {
				_, err := p.parseOutputField()
				return err
			}},
			{`name: "Ada" next`, func(p *Parser) error {
				_, err := p.parseOutputField()
				return err
			}},
			{`name:`, func(p *Parser) error {
				_, err := p.parseSchemaField()
				return err
			}},
			{`name: string /# first, /# second`, func(p *Parser) error {
				_, err := p.parseSchemaField()
				return err
			}},
			{`name: string next`, func(p *Parser) error {
				_, err := p.parseSchemaField()
				return err
			}},
			{`name: string next`, func(p *Parser) error {
				_, err := p.parseOutputSchemaField()
				return err
			}},
			{`name:`, func(p *Parser) error {
				_, err := p.parseRecordField()
				return err
			}},
			{`name: "Ada" next`, func(p *Parser) error {
				_, err := p.parseRecordField()
				return err
			}},
			{`name: "Ada" /# first, /# second`, func(p *Parser) error {
				_, err := p.parseRecordField()
				return err
			}},
			{`: "Ada"`, func(p *Parser) error {
				_, err := p.parseOutputField()
				return err
			}},
		}

		for _, item := range cases {
			tokens, err := lexInput(item.input)
			tAssert.NoError(err)
			tAssert.Error(item.call(New(tokens)))
		}
	})

	Describe("parses a full file", func() {
		It("returns errors for empty public parser entry points", func() {
			_, err := New(nil).ParseFile()
			tAssert.ErrorContains(err, "empty token stream")

			_, err = New(nil).ParseScriptBlock()
			tAssert.ErrorContains(err, "empty token stream")

			_, err = New(nil).ParseOutputBlock()
			tAssert.ErrorContains(err, "empty token stream")

			_, err = New(nil).ParseExpression()
			tAssert.ErrorContains(err, "empty token stream")
		})

		It("rejects trailing tokens from public script and output entry points", func() {
			scriptTokens, err := lexInput(`|===|
string name = "Ada";
|===|
extra`)
			tAssert.NoError(err)

			_, err = New(scriptTokens).ParseScriptBlock()
			tAssert.ErrorContains(err, "unexpected token after script block")

			outputTokens, err := lexInput(`[output = data] {} extra`)
			tAssert.NoError(err)

			_, err = New(outputTokens).ParseOutputBlock()
			tAssert.ErrorContains(err, "unexpected token after output block")

			_, err = parseFileInput(`[output = data] {} extra`)
			tAssert.ErrorContains(err, "unexpected token after output block")
		})

		It("returns public entry parse errors from malformed blocks", func() {
			scriptTokens, err := lexInput(`name`)
			tAssert.NoError(err)
			_, err = New(scriptTokens).ParseScriptBlock()
			tAssert.ErrorContains(err, "expected script delimiter")

			scriptTokens, err = lexInput(`|===|
string name = "Ada";`)
			tAssert.NoError(err)
			_, err = New(scriptTokens).ParseScriptBlock()
			tAssert.ErrorContains(err, "expected closing script delimiter")

			outputTokens, err := lexInput(`[output = data]`)
			tAssert.NoError(err)
			_, err = New(outputTokens).ParseOutputBlock()
			tAssert.ErrorContains(err, "expected '{' to start output block")

			outputTokens, err = lexInput(`{ name: "Ada";`)
			tAssert.NoError(err)
			_, err = New(outputTokens).ParseOutputBlock()
			tAssert.ErrorContains(err, "expected '}' to close output block")
		})

		It("rejects public output parsing when the next token is not an output block", func() {
			tokens, err := lexInput(`name`)
			tAssert.NoError(err)

			_, err = New(tokens).ParseOutputBlock()
			tAssert.ErrorContains(err, "expected output block")
		})

		It("parses public script and output block entry points", func() {
			scriptTokens, err := lexInput(`|===|
string name = "Ada";
|===|`)
			tAssert.NoError(err)
			script, err := New(scriptTokens).ParseScriptBlock()
			tAssert.NoError(err)
			tAssert.Len(script.Items, 1)

			outputTokens, err := lexInput(`[output = data]
{ name: "Ada"; }`)
			tAssert.NoError(err)
			output, err := New(outputTokens).ParseOutputBlock()
			tAssert.NoError(err)
			tAssert.Len(output.DataFields, 1)
		})

		It("parses import-as declarations and import aliases", func() {
			input := `|===|
from "./base.mace" import-as Base;
from "./shared.mace" import User:Person, Config:Settings;
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.Len(file.Imports, 2) {
				tAssert.NotNil(file.Imports[0].ImportAs)
				if file.Imports[0].ImportAs != nil {
					tAssert.Equal("Base", file.Imports[0].ImportAs.Name)
				}
				tAssert.Equal([]ast.ImportedIdentifier{
					{Name: "User", Alias: "Person"},
					{Name: "Config", Alias: "Settings"},
				}, file.Imports[1].Identifiers)
			}
		})

		It("parses all output directive kinds", func() {
			file, err := parseFileInput(`[output = data, schema = User, schema_file = "./schema.mace", parse = Runtime, parse_file = "./runtime.mace"] {}`)
			tAssert.NoError(err)

			if tAssert.Len(file.Output.Directives, 5) {
				tAssert.Equal(ast.OutputDirectiveOutput, file.Output.Directives[0].Kind)
				tAssert.Equal(ast.OutputDirectiveSchema, file.Output.Directives[1].Kind)
				tAssert.Equal(ast.OutputDirectiveSchemaFile, file.Output.Directives[2].Kind)
				tAssert.Equal(ast.OutputDirectiveParse, file.Output.Directives[3].Kind)
				tAssert.Equal(ast.OutputDirectiveParseFile, file.Output.Directives[4].Kind)
			}
		})

		It("rejects malformed import declarations", func() {
			cases := []string{
				`|===|
import "./base.mace" User;
|===|
[output = data] {}`,
				`|===|
from import User;
|===|
[output = data] {}`,
				`|===|
from "./base.mace" import- nope Base;
|===|
[output = data] {}`,
				`|===|
from "./base.mace" import-as;
|===|
[output = data] {}`,
				`|===|
from "./base.mace" import User:
|===|
[output = data] {}`,
				`|===|
from "./base.mace" import User,;
|===|
[output = data] {}`,
				`|===|
from "./base.mace" import User
string name = "Ada";
|===|
[output = data] {}`,
			}

			for _, input := range cases {
				_, err := parseFileInput(input)
				tAssert.Error(err)
			}
		})

		It("rejects imports after declarations and missing script closers", func() {
			_, err := parseFileInput(`|===|
string name = "Ada";
from "./base.mace" import User;
|===|
[output = data] {}`)
			tAssert.ErrorContains(err, "import declarations must appear at top")

			_, err = parseFileInput(`|===|
string name = "Ada";
[output = data] {}`)
			tAssert.Error(err)
		})

		It("returns an error for an empty script block", func() {
			_, err := parseFileInput(`|===|
|===|
[output = data]
{}`)
			tAssert.Error(err)
			tAssert.Contains(err.Error(), "empty script block")
		})
		It("parses script imports, declarations, and output block", func() {
			input := `|===|
from "base.mace" import User, Config;
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
				tAssert.Equal([]ast.ImportedIdentifier{{Name: "User"}, {Name: "Config"}}, file.Imports[0].Identifiers)
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
			input := `|===|
from "base.mace" import User; // trailing import comment
// line comment before declarations
schema Profile: {
  // line comment before field
  name: string; // trailing line comment
  /* block comment before optional field */
  age?: int; // trailing line comment
};

/* block comment between declarations */
Profile current = {
  name: "Ada"; // trailing field comment
  /* comment before optional field */
  age?: 30; // trailing field comment
};
|===|
[output = data]
{
  // line comment before output field
  result: current.name; // trailing output comment
  profile: {
    // line comment inside nested record
    age?: current.age; // trailing nested comment
  }; // trailing record comment
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.Len(file.Imports, 1) {
				tAssert.Equal("base.mace", file.Imports[0].Path.Lexeme[1:len(file.Imports[0].Path.Lexeme)-1])
				tAssert.Equal([]ast.ImportedIdentifier{{Name: "User"}}, file.Imports[0].Identifiers)
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

		It("ignores block comments that wrap script and output blocks", func() {
			input := `/*
|===|
type Hidden: string;
|===|
[output = data]
{
  hidden: "ignore me";
}
*/
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

		It("ignores block comments that wrap script imports", func() {
			input := `/*
from "./ignored.mace" import Ignored;
*/
|===|
from "./base.mace" import User;
/*
from "./also_ignored.mace" import AlsoIgnored;
*/
|===|
[output = data]
{
  result: 1;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)
			if tAssert.Len(file.Imports, 1) {
				tAssert.Equal("\"./base.mace\"", file.Imports[0].Path.Lexeme)
				tAssert.Equal([]ast.ImportedIdentifier{{Name: "User"}}, file.Imports[0].Identifiers)
			}
		})

		It("rejects top-level imports", func() {
			_, err := parseFileInput(`from "./base.mace" import User;
[output = data]
{ result: 1; }`)

			tAssert.Error(err)
			tAssert.ErrorContains(err, "expected output directive")
		})

		It("ignores block comments around type and schema declarations", func() {
			input := `|===|
/*
type Hidden: string;
schema HiddenUser: {
  name: string;
};
*/
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

		It("ignores block comments around documentation declarations", func() {
			input := `|===|
schema User: {
  name: string;
};
/*
schema_doc User {
  summary: "Ignore this doc";
};
*/
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

		It("ignores block comments inside output fields", func() {
			input := `[output = data]
{
  subtotal: 129.99 * 3;
/*
  total: $self.subtotal * 1.08875;
*/
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

		It("parses nullable declarations with null initializers", func() {
			input := `|===|
nullable string env = null;
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 1) {
				varDecl, ok := file.Script.Items[0].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					tAssert.True(varDecl.Nullable)
					tAssert.Equal("env", varDecl.Name)
					tAssert.True(varDecl.HasValue)
					_, ok := varDecl.Value.(ast.NullLiteral)
					tAssert.True(ok)
				}
			}
		})

		It("parses hex primitive type references", func() {
			input := `|===|
hex_int mask = 0xFF;
hex_float ratio = 0x2.8;
|===|
[output = schema]
{
  mask: hex_int;
  ratio: hex_float;
}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 2) {
				maskDecl, ok := file.Script.Items[0].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					maskType, ok := maskDecl.Type.(ast.PrimitiveType)
					tAssert.True(ok)
					if ok {
						tAssert.Equal("hex_int", maskType.Name)
					}
					requireHexIntLiteral(maskDecl.Value, "0xFF")
				}

				ratioDecl, ok := file.Script.Items[1].(ast.VariableDeclaration)
				tAssert.True(ok)
				if ok {
					ratioType, ok := ratioDecl.Type.(ast.PrimitiveType)
					tAssert.True(ok)
					if ok {
						tAssert.Equal("hex_float", ratioType.Name)
					}
					requireHexFloatLiteral(ratioDecl.Value, "0x2.8")
				}
			}

			if tAssert.Len(file.Output.SchemaFields, 2) {
				maskType, ok := file.Output.SchemaFields[0].Type.(ast.PrimitiveType)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("hex_int", maskType.Name)
				}

				ratioType, ok := file.Output.SchemaFields[1].Type.(ast.PrimitiveType)
				tAssert.True(ok)
				if ok {
					tAssert.Equal("hex_float", ratioType.Name)
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

		It("parses array members in variant type references", func() {
			input := `|===|
type Value: variant[array<string>, array<int>];
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
						_, firstIsArray := variantType.Members[0].(ast.ArrayType)
						_, secondIsArray := variantType.Members[1].(ast.ArrayType)
						tAssert.True(firstIsArray)
						tAssert.True(secondIsArray)
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

		It("parses choice types with literals and choice aliases", func() {
			input := `|===|
 type Environment: choice["dev", "prod"];
 type Mode: choice[Environment, 1, true, 1.5, 0xFF, 0x2.8, record];
|===|
[output = data] {}`

			file, err := parseFileInput(input)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 2) {
				choiceDecl, ok := file.Script.Items[1].(ast.TypeDeclaration)
				tAssert.True(ok)
				if ok {
					choiceType, ok := choiceDecl.Type.(ast.ChoiceType)
					tAssert.True(ok)
					if ok && tAssert.Len(choiceType.Members, 7) {
						_, ok = choiceType.Members[0].(ast.Identifier)
						tAssert.True(ok)
						requireIntLiteral(choiceType.Members[1], "1")
						booleanLiteral, ok := choiceType.Members[2].(ast.BooleanLiteral)
						tAssert.True(ok)
						if ok {
							tAssert.True(booleanLiteral.Value)
						}
						floatLiteral, ok := choiceType.Members[3].(ast.FloatLiteral)
						tAssert.True(ok)
						if ok {
							tAssert.Equal("1.5", floatLiteral.Lexeme)
						}
						requireHexIntLiteral(choiceType.Members[4], "0xFF")
						requireHexFloatLiteral(choiceType.Members[5], "0x2.8")
						requireIdentifier(choiceType.Members[6], "record")
					}
				}
			}
		})

		It("parses record map and inline record type references", func() {
			file, err := parseFileInput(`|===|
type Lookup: record<string>;
type Inline: { name: string; };
|===|
[output = schema]
{
  values: record<int>;
  inline: { enabled: boolean; };
}`)
			tAssert.NoError(err)

			if tAssert.NotNil(file.Script) && tAssert.Len(file.Script.Items, 2) {
				typeDecl, ok := file.Script.Items[0].(ast.TypeDeclaration)
				tAssert.True(ok)
				if ok {
					_, ok = typeDecl.Type.(ast.RecordMapType)
					tAssert.True(ok)
				}
				inlineDecl, ok := file.Script.Items[1].(ast.TypeDeclaration)
				tAssert.True(ok)
				if ok {
					_, ok = inlineDecl.Type.(ast.RecordType)
					tAssert.True(ok)
				}
			}

			if tAssert.Len(file.Output.SchemaFields, 2) {
				_, ok := file.Output.SchemaFields[0].Type.(ast.RecordMapType)
				tAssert.True(ok)
				_, ok = file.Output.SchemaFields[1].Type.(ast.RecordType)
				tAssert.True(ok)
			}
		})

		It("rejects malformed declarations, docs, directives, and type references", func() {
			cases := []string{
				`|===|
string = "Ada";
|===|
[output = data] {}`,
				`|===|
type : string;
|===|
[output = data] {}`,
				`|===|
type Name string;
|===|
[output = data] {}`,
				`|===|
type Name: ;
|===|
[output = data] {}`,
				`|===|
schema : {};
|===|
[output = data] {}`,
				`|===|
schema User {};
|===|
[output = data] {}`,
				`|===|
schema User: { name string; };
|===|
[output = data] {}`,
				`|===|
schema User: { name: string;
|===|
[output = data] {}`,
				`|===|
schema_doc {
};
|===|
[output = data] {}`,
				`|===|
schema_doc User
};
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  summary: "One";
  summary: "Two";
};
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  unknown: "Nope";
};
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  props: {
    name: "One";
    name: "Two";
  };
};
|===|
[output = data] {}`,
				`[output data] {}`,
				`[output = nope] {}`,
				`[schema_file = User] {}`,
				`[parse = "Runtime"] {}`,
				`[parse_file = Runtime] {}`,
				`[schema = "User"] {}`,
				`[output = data] "single line doc" {}`,
				`[output = data] { name "Ada"; }`,
				`[output = schema] { name string; }`,
				`[output = schema] { name: ; }`,
				`[output = schema] { name: string /# first, /# second }`,
				`|===|
type Names: array;
|===|
[output = data] {}`,
				`|===|
type Names: array<string;
|===|
[output = data] {}`,
				`|===|
type Names: record;
|===|
[output = data] {}`,
				`|===|
type Names: union;
|===|
[output = data] {}`,
				`|===|
type Names: variant[string,];
|===|
[output = data] {}`,
				`|===|
type Names: choice;
|===|
[output = data] {}`,
				`|===|
type Names: choice[null];
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  summary: 1;
};
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  props: {
    name: 1;
  };
};
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  props: {
    name: "Name";
  }
};
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  summary: "User";
}
|===|
[output = data] {}`,
				`|===|
schema_doc User {
  summary: "User";
};
|===|
[output = data]`,
			}

			for _, input := range cases {
				_, err := parseFileInput(input)
				tAssert.Error(err)
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
			file, err := parseFileInput(`|===|
from "./shared.mace" import Name, User;
type Alias: string;
nullable string env = null;
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

type Status: choice["Active"];

schema_doc User {
  summary: "Represents a user.",
  description: """
# User
""",
};

gen_doc Status {
  summary: "Represents a status choice.",
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

				choiceDoc, ok := file.Script.Items[3].(ast.DocDeclaration)
				tAssert.True(ok)
				if ok && tAssert.NotNil(choiceDoc.Documentation.Summary) {
					tAssert.Equal("Status", choiceDoc.Target)
					tAssert.Equal("\"Represents a status choice.\"", choiceDoc.Documentation.Summary.Lexeme)
				}
			}
		})

		It("rejects props entries in gen_doc declarations", func() {
			input := `|===|
type Name: string;

gen_doc Name {
  props: {
    value: "Nope",
  },
};
|===|
[output = data]
{}`

			_, err := parseFileInput(input)
			tAssert.Error(err)
			tAssert.ErrorContains(err, "props entry is only allowed in schema_doc")
		})

		It("parses documentation fixtures with props and inline descriptions", func() {
			file, err := parseFileInput(`|===|
schema User: {
  name: string;
};

string greeting = "Hello";

gen_doc greeting {
  summary: "Rendered greeting";
};

schema_doc User {
  summary: "Represents a user";
  description: """
# User

Hover should surface this documentation.
""";
  props: {
    name: "The user's display name";
  };
};
|===|
[output = schema]
"""
# User Output
"""
{
  user: User /# Public user schema;
}
`)
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

		It("parses nested variable array access fixtures", func() {
			file, err := parseFileInput(`|============================================================|
array<int> level1 = [1];
array<array<int>> level2 = [[2]];
array<array<array<int>>> level3 = [[[3]]];
array<array<array<array<int>>>> level4 = [[[[4]]]];
array<array<array<array<array<int>>>>> level5 = [[[[[5]]]]];
|============================================================|
[output = data]
{
  level1: level1[0],
  level2: level2[0][0],
  level3: level3[0][0][0],
  level4: level4[0][0][0][0],
  level5: level5[0][0][0][0][0],
}
`)
			tAssert.NoError(err)
			if !tAssert.NotNil(file.Script) {
				return
			}
			if !tAssert.Len(file.Output.DataFields, 5) {
				return
			}

			for depth, field := range file.Output.DataFields {
				current := field.Value
				for level := depth + 1; level >= 1; level-- {
					access := requireArrayAccess(current, "0")
					current = access.Target
				}
				requireIdentifier(current, fmt.Sprintf("level%d", depth+1))
			}
		})
	})
})
