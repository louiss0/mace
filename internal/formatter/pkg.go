package formatter

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
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
	lines := make([]string, 0, len(script.Items))
	for _, declaration := range script.Items {
		line, err := formatDeclaration(declaration)
		if err != nil {
			return err
		}
		lines = append(lines, line)
	}

	delimiter := formatScriptDelimiter(lines)
	f.writeLine(delimiter)
	for _, line := range lines {
		f.writeLine(line)
	}
	f.write(delimiter)
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
	if output.Doc != nil {
		f.writeLine(output.Doc.Lexeme)
	}
	if output.Mode == ast.OutputModeSchema {
		return f.writeSchemaOutputBlock(output.SchemaFields)
	}

	return f.writeDataOutputBlock(output.DataFields)
}

func (f *formatter) writeDataOutputBlock(fields []ast.OutputField) error {
	if len(fields) == 0 {
		f.write("{}")
		return nil
	}

	f.writeLine("{")
	for _, item := range fields {
		line, err := formatOutputField(item)
		if err != nil {
			return err
		}
		f.writeLine("  " + line)
	}
	f.write("}")
	return nil
}

func (f *formatter) writeSchemaOutputBlock(fields []ast.OutputSchemaField) error {
	if len(fields) == 0 {
		f.write("{}")
		return nil
	}

	f.writeLine("{")
	for _, field := range fields {
		line, err := formatOutputSchemaField(field)
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
		prefix := ""
		if typedDeclaration.Injectable {
			prefix = "injectable "
		}

		typeReference, err := formatTypeReference(typedDeclaration.Type)
		if err != nil {
			return "", err
		}

		if !typedDeclaration.HasValue {
			return fmt.Sprintf("%s%s %s;", prefix, typeReference, typedDeclaration.Name), nil
		}

		value, err := formatExpressionWithDepth(typedDeclaration.Value, 0)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("%s%s %s = %s;", prefix, typeReference, typedDeclaration.Name, value), nil
	case ast.TypeDeclaration:
		typeReference, err := formatTypeReference(typedDeclaration.Type)
		if err != nil {
			return "", err
		}

		description := formatInlineDescription(typedDeclaration.Description)
		return fmt.Sprintf("type %s: %s%s;", typedDeclaration.Name, typeReference, description), nil
	case ast.SchemaDeclaration:
		recordType, err := formatRecordType(typedDeclaration.Type, 0)
		if err != nil {
			return "", err
		}

		return fmt.Sprintf("schema %s: %s;", typedDeclaration.Name, recordType), nil
	case ast.DocDeclaration:
		return formatDocDeclaration(typedDeclaration)
	case ast.EnumDeclaration:
		return formatEnumDeclaration(typedDeclaration)
	default:
		return "", fmt.Errorf("format declaration: unsupported %T", declaration)
	}
}

func formatEnumDeclaration(declaration ast.EnumDeclaration) (string, error) {
	if len(declaration.Members) == 0 {
		return fmt.Sprintf("enum %s: %s {};", declaration.Name, declaration.BackingType.Name), nil
	}

	lines := []string{fmt.Sprintf("enum %s: %s {", declaration.Name, declaration.BackingType.Name)}
	for _, member := range declaration.Members {
		value, err := formatEnumMember(member)
		if err != nil {
			return "", err
		}
		lines = append(lines, "  "+value)
	}
	lines = append(lines, "};")
	return strings.Join(lines, "\n"), nil
}

func formatEnumMember(member ast.EnumMember) (string, error) {
	if !member.HasValue {
		return member.Name + ",", nil
	}

	value, err := formatExpressionWithDepth(member.Value, 0)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s = %s,", member.Name, value), nil
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
	case ast.UnionType:
		members := make([]string, 0, len(typedReference.Members))
		for _, member := range typedReference.Members {
			formatted, err := formatTypeReference(member)
			if err != nil {
				return "", err
			}
			members = append(members, formatted)
		}
		return fmt.Sprintf("union[%s]", strings.Join(members, ", ")), nil
	case ast.VariantType:
		members := make([]string, 0, len(typedReference.Members))
		for _, member := range typedReference.Members {
			formatted, err := formatTypeReference(member)
			if err != nil {
				return "", err
			}
			members = append(members, formatted)
		}
		return fmt.Sprintf("variant[%s]", strings.Join(members, ", ")), nil
	case ast.RecordType:
		return formatRecordType(typedReference, 0)
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

		description := formatInlineDescription(field.Description)
		lines = append(lines, fmt.Sprintf("%s%s%s: %s%s;", indent, field.Name, optional, typeReference, description))
	}

	lines = append(lines, closingIndent+"}")
	return strings.Join(lines, "\n"), nil
}

func formatDocDeclaration(declaration ast.DocDeclaration) (string, error) {
	keyword := "gen_doc"
	if declaration.Kind == ast.DocumentationKindSchema {
		keyword = "schema_doc"
	}

	lines := []string{fmt.Sprintf("%s %s {", keyword, declaration.Target)}
	if declaration.Documentation.Summary != nil {
		lines = append(lines, fmt.Sprintf("  summary: %s;", declaration.Documentation.Summary.Lexeme))
	}
	if declaration.Documentation.Description != nil {
		lines = append(lines, fmt.Sprintf("  description: %s;", declaration.Documentation.Description.Lexeme))
	}
	if len(declaration.Documentation.Props) > 0 {
		lines = append(lines, "  props: {")
		keys := lo.Keys(declaration.Documentation.Props)
		slices.Sort(keys)
		for _, key := range keys {
			lines = append(lines, fmt.Sprintf("    %s: %s;", key, declaration.Documentation.Props[key].Lexeme))
		}
		lines = append(lines, "  };")
	}
	lines = append(lines, "}")
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
	value, err := formatExpressionWithDepth(field.Value, 1)
	if err != nil {
		return "", err
	}

	optional := ""
	if field.Optional {
		optional = "?"
	}

	description := formatInlineDescription(field.Description)
	return fmt.Sprintf("%s%s: %s%s;", field.Name, optional, value, description), nil
}

func formatOutputSchemaField(field ast.OutputSchemaField) (string, error) {
	typeReference, err := formatTypeReference(field.Type)
	if err != nil {
		return "", err
	}

	optional := ""
	if field.Optional {
		optional = "?"
	}

	description := formatInlineDescription(field.Description)
	return fmt.Sprintf("%s%s: %s%s;", field.Name, optional, typeReference, description), nil
}

func formatInlineDescription(description string) string {
	if description == "" {
		return ""
	}

	return " /# " + description
}

func formatExpressionWithDepth(expression ast.Expression, depth int) (string, error) {
	return formatExpressionWithPrecedence(expression, precedenceLowest, depth)
}

func formatExpressionWithPrecedence(expression ast.Expression, parentPrecedence int, depth int) (string, error) {
	formatted, precedence, err := formatExpressionNode(expression, depth)
	if err != nil {
		return "", err
	}

	if precedence < parentPrecedence {
		return "(" + formatted + ")", nil
	}

	return formatted, nil
}

func formatExpressionNode(expression ast.Expression, depth int) (string, int, error) {
	switch typedExpression := expression.(type) {
	case ast.Identifier:
		return typedExpression.Name, precedencePrimary, nil
	case ast.MemberAccess:
		target, err := formatExpressionWithPrecedence(typedExpression.Target, precedencePrimary, depth)
		if err != nil {
			return "", 0, err
		}
		return target + "." + typedExpression.Name, precedencePrimary, nil
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
		value, err := formatArrayLiteral(typedExpression, depth)
		if err != nil {
			return "", 0, err
		}
		return value, precedencePrimary, nil
	case ast.RecordLiteral:
		record, err := formatRecordLiteral(typedExpression, depth)
		if err != nil {
			return "", 0, err
		}
		return record, precedencePrimary, nil
	case ast.SelfReference:
		return "$self." + strings.Join(typedExpression.Path, "."), precedencePrimary, nil
	case ast.PrefixExpression:
		right, err := formatExpressionWithPrecedence(typedExpression.Right, precedencePrefix, depth)
		if err != nil {
			return "", 0, err
		}

		return tokenLexeme(typedExpression.Operator) + right, precedencePrefix, nil
	case ast.InfixExpression:
		return formatInfixExpression(typedExpression, depth)
	case ast.ConditionalExpression:
		condition, err := formatExpressionWithPrecedence(typedExpression.Condition, precedenceConditional, depth)
		if err != nil {
			return "", 0, err
		}

		thenValue, err := formatExpressionWithDepth(typedExpression.Then, depth)
		if err != nil {
			return "", 0, err
		}

		elseValue, err := formatExpressionWithPrecedence(typedExpression.Else, precedenceConditional, depth)
		if err != nil {
			return "", 0, err
		}

		return fmt.Sprintf("%s ? %s : %s", condition, thenValue, elseValue), precedenceConditional, nil
	default:
		return "", 0, fmt.Errorf("format expression: unsupported %T", expression)
	}
}

func formatArrayLiteral(array ast.ArrayLiteral, depth int) (string, error) {
	if len(array.Elements) == 0 {
		return "[]", nil
	}

	values := make([]string, 0, len(array.Elements))
	multiline := len(array.Elements) > 1
	for _, element := range array.Elements {
		value, err := formatExpressionWithDepth(element, depth+1)
		if err != nil {
			return "", err
		}
		if strings.Contains(value, "\n") {
			multiline = true
		}
		values = append(values, value)
	}

	if !multiline {
		return "[" + strings.Join(values, ", ") + "]", nil
	}

	lines := []string{"["}
	indent := strings.Repeat("  ", depth+1)
	closingIndent := strings.Repeat("  ", depth)
	for index, value := range values {
		lines = append(lines, formatMultilineArrayItem(value, indent, index < len(values)-1))
	}
	lines = append(lines, closingIndent+"]")
	return strings.Join(lines, "\n"), nil
}

func formatRecordLiteral(record ast.RecordLiteral, depth int) (string, error) {
	if len(record.Fields) == 0 {
		return "{}", nil
	}

	lines := []string{"{"}
	indent := strings.Repeat("  ", depth+1)
	closingIndent := strings.Repeat("  ", depth)
	for _, field := range record.Fields {
		value, err := formatExpressionWithDepth(field.Value, depth+1)
		if err != nil {
			return "", err
		}

		optional := ""
		if field.Optional {
			optional = "?"
		}

		lines = append(lines, formatMultilineRecordField(field.Name, optional, value, indent))
	}

	lines = append(lines, closingIndent+"}")
	return strings.Join(lines, "\n"), nil
}

func formatInfixExpression(expression ast.InfixExpression, depth int) (string, int, error) {
	precedence := infixPrecedence(expression.Operator)

	leftPrecedence := precedence
	if expression.Operator == lexer.TokenDoubleStar {
		leftPrecedence = precedence + 1
	}

	rightPrecedence := precedence + 1
	if expression.Operator == lexer.TokenDoubleStar {
		rightPrecedence = precedence
	}

	left, err := formatExpressionWithPrecedence(expression.Left, leftPrecedence, depth)
	if err != nil {
		return "", 0, err
	}

	right, err := formatExpressionWithPrecedence(expression.Right, rightPrecedence, depth)
	if err != nil {
		return "", 0, err
	}

	return fmt.Sprintf("%s %s %s", left, tokenLexeme(expression.Operator), right), precedence, nil
}

func formatScriptDelimiter(lines []string) string {
	width := 3
	for _, line := range lines {
		for _, part := range strings.Split(line, "\n") {
			if len(part) > width {
				width = len(part)
			}
		}
	}

	return "|" + strings.Repeat("=", width) + "|"
}

func formatMultilineArrayItem(value string, indent string, trailingComma bool) string {
	lines := strings.Split(value, "\n")
	lines[0] = indent + lines[0]
	if trailingComma {
		lines[len(lines)-1] += ","
	}
	return strings.Join(lines, "\n")
}

func formatMultilineRecordField(name string, optional string, value string, indent string) string {
	lines := strings.Split(value, "\n")
	lines[0] = indent + fmt.Sprintf("%s%s: %s", name, optional, lines[0])
	lines[len(lines)-1] += ";"
	return strings.Join(lines, "\n")
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
