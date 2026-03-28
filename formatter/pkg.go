package formatter

import (
	"fmt"
	"strings"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser/ast"
)

func FormatFile(file ast.File) (string, error) {
	formatter := newFormatter()

	if err := formatter.writeFile(file); err != nil {
		return "", err
	}

	return formatter.builder.String(), nil
}

type formatter struct {
	builder strings.Builder
}

func newFormatter() *formatter {
	return &formatter{}
}

func (f *formatter) writeFile(file ast.File) error {
	for index, importDeclaration := range file.Imports {
		if index > 0 {
			f.writeLine("")
		}
		f.writeImportDeclaration(importDeclaration)
	}

	if len(file.Imports) > 0 && file.Script != nil {
		f.writeLine("")
	}

	if file.Script != nil {
		if err := f.writeScriptBlock(*file.Script); err != nil {
			return err
		}
		f.writeLine("")
	}

	return f.writeOutputBlock(file.Output)
}

func (f *formatter) writeImportDeclaration(importDeclaration ast.ImportDeclaration) {
	f.write("from ")
	f.write(importDeclaration.Path.Lexeme)
	f.write(" import ")
	f.write(strings.Join(importDeclaration.Identifiers, ", "))
	f.writeLine(";")
}

func (f *formatter) writeScriptBlock(script ast.ScriptBlock) error {
	f.writeLine("|===|")
	for _, declaration := range script.Items {
		line, err := formatDeclaration(declaration)
		if err != nil {
			return err
		}
		f.writeLine(line)
	}
	f.write("|===|")
	return nil
}

func (f *formatter) writeOutputBlock(output ast.OutputBlock) error {
	if len(output.Directives) > 0 {
		directive, err := formatOutputDirectives(output.Directives)
		if err != nil {
			return err
		}
		f.writeLine(directive)
	}
	if len(output.Items) == 0 {
		f.write("{}")
		return nil
	}

	f.writeLine("{")
	for _, item := range output.Items {
		line, err := formatOutputField(item)
		if err != nil {
			return err
		}
		f.writeLine("  " + line)
	}
	f.write("}")
	return nil
}

func (f *formatter) write(value string) {
	f.builder.WriteString(value)
}

func (f *formatter) writeLine(value string) {
	f.builder.WriteString(value)
	f.builder.WriteByte('\n')
}

func formatDeclaration(declaration ast.Declaration) (string, error) {
	switch typedDeclaration := declaration.(type) {
	case ast.VariableDeclaration:
		value, err := formatExpression(typedDeclaration.Value)
		if err != nil {
			return "", err
		}

		prefix := ""
		if typedDeclaration.Injectable {
			prefix = "injectable "
		}

		typeReference, err := formatTypeReference(typedDeclaration.Type)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("%s%s %s = %s;", prefix, typeReference, typedDeclaration.Name, value), nil
	case ast.TypeDeclaration:
		typeReference, err := formatTypeReference(typedDeclaration.Type)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("type %s = %s;", typedDeclaration.Name, typeReference), nil
	case ast.SchemaDeclaration:
		recordType, err := formatRecordType(typedDeclaration.Type, 0)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("schema %s = %s;", typedDeclaration.Name, recordType), nil
	default:
		return "", fmt.Errorf("format declaration: unsupported %T", declaration)
	}
}

func formatTypeReference(typeReference ast.TypeReference) (string, error) {
	switch typedReference := typeReference.(type) {
	case ast.PrimitiveType:
		return typedReference.Name, nil
	case ast.ArrayType:
		element, err := formatTypeReference(typedReference.Element)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("array<%s>", element), nil
	case ast.NamedType:
		return typedReference.Name, nil
	default:
		return "", fmt.Errorf("format type reference: unsupported %T", typeReference)
	}
}

func formatRecordType(recordType ast.RecordType, depth int) (string, error) {
	if len(recordType.Fields) == 0 {
		return "{}", nil
	}

	lines := []string{"{"}
	indent := strings.Repeat("  ", depth+1)
	closingIndent := strings.Repeat("  ", depth)

	for _, field := range recordType.Fields {
		typeReference, err := formatTypeReference(field.Type)
		if err != nil {
			return "", err
		}

		optional := ""
		if field.Optional {
			optional = "?"
		}

		lines = append(lines, fmt.Sprintf("%s%s%s: %s;", indent, field.Name, optional, typeReference))
	}

	lines = append(lines, closingIndent+"}")
	return strings.Join(lines, "\n"), nil
}

func formatOutputDirectives(directives []ast.OutputDirective) (string, error) {
	parts := make([]string, 0, len(directives))
	for _, directive := range directives {
		switch directive.Kind {
		case ast.OutputDirectiveOutput:
			parts = append(parts, "output = "+directive.Value)
		case ast.OutputDirectiveSchemaFile:
			parts = append(parts, "schema_file = "+directive.Value)
		case ast.OutputDirectiveSchema:
			parts = append(parts, "schema = "+directive.Value)
		default:
			return "", fmt.Errorf("format output directive: unsupported %d", directive.Kind)
		}
	}

	return "[" + strings.Join(parts, ", ") + "]", nil
}

func formatOutputField(field ast.OutputField) (string, error) {
	value, err := formatExpression(field.Value)
	if err != nil {
		return "", err
	}

	optional := ""
	if field.Optional {
		optional = "?"
	}

	return fmt.Sprintf("%s%s: %s;", field.Name, optional, value), nil
}

func formatExpression(expression ast.Expression) (string, error) {
	return formatExpressionWithPrecedence(expression, precedenceLowest)
}

func formatExpressionWithPrecedence(expression ast.Expression, parentPrecedence int) (string, error) {
	formatted, precedence, err := formatExpressionNode(expression)
	if err != nil {
		return "", err
	}

	if precedence < parentPrecedence {
		return "(" + formatted + ")", nil
	}

	return formatted, nil
}

func formatExpressionNode(expression ast.Expression) (string, int, error) {
	switch typedExpression := expression.(type) {
	case ast.Identifier:
		return typedExpression.Name, precedencePrimary, nil
	case ast.StringLiteral:
		return typedExpression.Lexeme, precedencePrimary, nil
	case ast.IntLiteral:
		return typedExpression.Lexeme, precedencePrimary, nil
	case ast.FloatLiteral:
		return typedExpression.Lexeme, precedencePrimary, nil
	case ast.BooleanLiteral:
		if typedExpression.Value {
			return "true", precedencePrimary, nil
		}
		return "false", precedencePrimary, nil
	case ast.ArrayLiteral:
		values := make([]string, 0, len(typedExpression.Elements))
		for _, element := range typedExpression.Elements {
			value, err := formatExpression(element)
			if err != nil {
				return "", 0, err
			}
			values = append(values, value)
		}

		return "[" + strings.Join(values, ", ") + "]", precedencePrimary, nil
	case ast.RecordLiteral:
		record, err := formatRecordLiteral(typedExpression)
		if err != nil {
			return "", 0, err
		}
		return record, precedencePrimary, nil
	case ast.SelfReference:
		return "$self." + strings.Join(typedExpression.Path, "."), precedencePrimary, nil
	case ast.PrefixExpression:
		right, err := formatExpressionWithPrecedence(typedExpression.Right, precedencePrefix)
		if err != nil {
			return "", 0, err
		}

		return tokenLexeme(typedExpression.Operator) + right, precedencePrefix, nil
	case ast.InfixExpression:
		return formatInfixExpression(typedExpression)
	case ast.ConditionalExpression:
		condition, err := formatExpressionWithPrecedence(typedExpression.Condition, precedenceConditional)
		if err != nil {
			return "", 0, err
		}

		thenValue, err := formatExpression(typedExpression.Then)
		if err != nil {
			return "", 0, err
		}

		elseValue, err := formatExpressionWithPrecedence(typedExpression.Else, precedenceConditional)
		if err != nil {
			return "", 0, err
		}

		return fmt.Sprintf("%s ? %s : %s", condition, thenValue, elseValue), precedenceConditional, nil
	default:
		return "", 0, fmt.Errorf("format expression: unsupported %T", expression)
	}
}

func formatRecordLiteral(record ast.RecordLiteral) (string, error) {
	if len(record.Fields) == 0 {
		return "{}", nil
	}

	lines := []string{"{"}
	for _, field := range record.Fields {
		value, err := formatExpression(field.Value)
		if err != nil {
			return "", err
		}

		optional := ""
		if field.Optional {
			optional = "?"
		}

		lines = append(lines, fmt.Sprintf("  %s%s: %s;", field.Name, optional, value))
	}

	lines = append(lines, "}")
	return strings.Join(lines, "\n"), nil
}

func formatInfixExpression(expression ast.InfixExpression) (string, int, error) {
	precedence := infixPrecedence(expression.Operator)

	leftPrecedence := precedence
	if expression.Operator == lexer.TokenDoubleStar {
		leftPrecedence = precedence + 1
	}

	rightPrecedence := precedence + 1
	if expression.Operator == lexer.TokenDoubleStar {
		rightPrecedence = precedence
	}

	left, err := formatExpressionWithPrecedence(expression.Left, leftPrecedence)
	if err != nil {
		return "", 0, err
	}

	right, err := formatExpressionWithPrecedence(expression.Right, rightPrecedence)
	if err != nil {
		return "", 0, err
	}

	return fmt.Sprintf("%s %s %s", left, tokenLexeme(expression.Operator), right), precedence, nil
}

func tokenLexeme(tokenType lexer.TokenType) string {
	switch tokenType {
	case lexer.TokenPlus:
		return "+"
	case lexer.TokenMinus:
		return "-"
	case lexer.TokenStar:
		return "*"
	case lexer.TokenSlash:
		return "/"
	case lexer.TokenPercent:
		return "%"
	case lexer.TokenDoubleStar:
		return "**"
	case lexer.TokenBang:
		return "!"
	case lexer.TokenTilde:
		return "~"
	case lexer.TokenLess:
		return "<"
	case lexer.TokenLessEqual:
		return "<="
	case lexer.TokenGreater:
		return ">"
	case lexer.TokenGreaterEqual:
		return ">="
	case lexer.TokenEqualEqual:
		return "=="
	case lexer.TokenNotEqual:
		return "!="
	case lexer.TokenStrictEqual:
		return "==="
	case lexer.TokenStrictNotEqual:
		return "!=="
	case lexer.TokenAmpersand:
		return "&"
	case lexer.TokenCaret:
		return "^"
	case lexer.TokenPipe:
		return "|"
	case lexer.TokenAndAnd:
		return "&&"
	case lexer.TokenOrOr:
		return "||"
	case lexer.TokenShiftLeft:
		return "<<"
	case lexer.TokenShiftRight:
		return ">>"
	case lexer.TokenShiftRightUnsigned:
		return ">>>"
	default:
		return ""
	}
}

const (
	precedenceLowest = iota
	precedenceConditional
	precedenceLogicalOr
	precedenceLogicalAnd
	precedenceBitwiseOr
	precedenceBitwiseXor
	precedenceBitwiseAnd
	precedenceEquality
	precedenceRelational
	precedenceShift
	precedenceAdditive
	precedenceMultiplicative
	precedenceExponent
	precedencePrefix
	precedencePrimary
)

func infixPrecedence(tokenType lexer.TokenType) int {
	switch tokenType {
	case lexer.TokenOrOr:
		return precedenceLogicalOr
	case lexer.TokenAndAnd:
		return precedenceLogicalAnd
	case lexer.TokenPipe:
		return precedenceBitwiseOr
	case lexer.TokenCaret:
		return precedenceBitwiseXor
	case lexer.TokenAmpersand:
		return precedenceBitwiseAnd
	case lexer.TokenEqualEqual, lexer.TokenNotEqual, lexer.TokenStrictEqual, lexer.TokenStrictNotEqual:
		return precedenceEquality
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		return precedenceRelational
	case lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned:
		return precedenceShift
	case lexer.TokenPlus, lexer.TokenMinus:
		return precedenceAdditive
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return precedenceMultiplicative
	case lexer.TokenDoubleStar:
		return precedenceExponent
	default:
		return precedenceLowest
	}
}
