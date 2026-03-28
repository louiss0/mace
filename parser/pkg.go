package parser

import (
	"fmt"
	"strings"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser/ast"
)

const (
	precedenceLowest = iota
	precedenceTernary
	precedenceOr
	precedenceAnd
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
)

type Parser struct {
	tokens   []lexer.Token
	position int
}

func New(tokens []lexer.Token) *Parser {
	return &Parser{
		tokens: tokens,
	}
}

func (p *Parser) ParseFile() (ast.File, error) {
	if len(p.tokens) == 0 {
		return ast.File{}, fmt.Errorf("parser: empty token stream")
	}

	imports := []ast.ImportDeclaration{}
	for p.current().Type == lexer.TokenFrom {
		importDecl, err := p.parseImportDeclaration()
		if err != nil {
			return ast.File{}, err
		}
		imports = append(imports, importDecl)
	}

	var script *ast.ScriptBlock
	if p.current().Type == lexer.TokenScriptDelimiter {
		scriptBlock, err := p.parseScriptBlock()
		if err != nil {
			return ast.File{}, err
		}
		script = &scriptBlock
	}

	if p.current().Type != lexer.TokenLBracket {
		return ast.File{}, p.unexpectedTokenError("parser: expected output directive")
	}

	outputBlock, err := p.parseOutputBlock()
	if err != nil {
		return ast.File{}, err
	}

	if !p.isAtEnd() {
		return ast.File{}, p.unexpectedTokenError("parser: unexpected token after output block")
	}

	return ast.File{
		Imports: imports,
		Script:  script,
		Output:  outputBlock,
	}, nil
}

func (p *Parser) ParseExpression() (ast.Expression, error) {
	if len(p.tokens) == 0 {
		return nil, fmt.Errorf("parser: empty token stream")
	}

	return p.parseExpression(precedenceLowest)
}

func (p *Parser) parseImportDeclaration() (ast.ImportDeclaration, error) {
	if _, err := p.consume(lexer.TokenFrom, "parser: expected 'from'"); err != nil {
		return ast.ImportDeclaration{}, err
	}

	pathToken, err := p.consume(lexer.TokenString, "parser: expected string literal in import")
	if err != nil {
		return ast.ImportDeclaration{}, err
	}

	if _, err := p.consume(lexer.TokenImport, "parser: expected 'import'"); err != nil {
		return ast.ImportDeclaration{}, err
	}

	firstIdentifier, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in import list")
	if err != nil {
		return ast.ImportDeclaration{}, err
	}

	identifiers := []string{firstIdentifier.Lexeme}
	for p.current().Type == lexer.TokenComma {
		p.advance()
		nextIdentifier, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier after ',' in import list")
		if err != nil {
			return ast.ImportDeclaration{}, err
		}
		identifiers = append(identifiers, nextIdentifier.Lexeme)
	}

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after import declaration"); err != nil {
		return ast.ImportDeclaration{}, err
	}

	return ast.ImportDeclaration{
		Path:        ast.StringLiteral{Lexeme: pathToken.Lexeme},
		Identifiers: identifiers,
	}, nil
}

func (p *Parser) parseScriptBlock() (ast.ScriptBlock, error) {
	if _, err := p.consume(lexer.TokenScriptDelimiter, "parser: expected script delimiter"); err != nil {
		return ast.ScriptBlock{}, err
	}

	items := []ast.Declaration{}
	for !p.isAtEnd() && p.current().Type != lexer.TokenScriptDelimiter {
		declaration, err := p.parseDeclaration()
		if err != nil {
			return ast.ScriptBlock{}, err
		}
		items = append(items, declaration)
	}

	if _, err := p.consume(lexer.TokenScriptDelimiter, "parser: expected closing script delimiter"); err != nil {
		return ast.ScriptBlock{}, err
	}

	return ast.ScriptBlock{Items: items}, nil
}

func (p *Parser) parseDeclaration() (ast.Declaration, error) {
	switch p.current().Type {
	case lexer.TokenTypeKeyword:
		return p.parseTypeDeclaration()
	case lexer.TokenSchema:
		return p.parseSchemaDeclaration()
	default:
		return p.parseVariableDeclaration()
	}
}

func (p *Parser) parseVariableDeclaration() (ast.Declaration, error) {
	injectable := false
	if p.current().Type == lexer.TokenInjectable {
		injectable = true
		p.advance()
	}

	typeRef, err := p.parseTypeReference()
	if err != nil {
		return nil, err
	}

	nameToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in variable declaration")
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' in variable declaration"); err != nil {
		return nil, err
	}

	value, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after variable declaration"); err != nil {
		return nil, err
	}

	return ast.VariableDeclaration{
		Injectable: injectable,
		Type:       typeRef,
		Name:       nameToken.Lexeme,
		Value:      value,
	}, nil
}

func (p *Parser) parseTypeDeclaration() (ast.Declaration, error) {
	if _, err := p.consume(lexer.TokenTypeKeyword, "parser: expected 'type'"); err != nil {
		return nil, err
	}

	nameToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in type declaration")
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' in type declaration"); err != nil {
		return nil, err
	}

	typeRef, err := p.parseTypeReference()
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after type declaration"); err != nil {
		return nil, err
	}

	return ast.TypeDeclaration{
		Name: nameToken.Lexeme,
		Type: typeRef,
	}, nil
}

func (p *Parser) parseSchemaDeclaration() (ast.Declaration, error) {
	if _, err := p.consume(lexer.TokenSchema, "parser: expected 'schema'"); err != nil {
		return nil, err
	}

	nameToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in schema declaration")
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' in schema declaration"); err != nil {
		return nil, err
	}

	recordType, err := p.parseRecordType()
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after schema declaration"); err != nil {
		return nil, err
	}

	return ast.SchemaDeclaration{
		Name: nameToken.Lexeme,
		Type: recordType,
	}, nil
}

func (p *Parser) parseRecordType() (ast.RecordType, error) {
	if _, err := p.consume(lexer.TokenLBrace, "parser: expected '{' to start record type"); err != nil {
		return ast.RecordType{}, err
	}

	fields := []ast.SchemaField{}
	for !p.isAtEnd() && p.current().Type != lexer.TokenRBrace {
		field, err := p.parseSchemaField()
		if err != nil {
			return ast.RecordType{}, err
		}
		fields = append(fields, field)
	}

	if _, err := p.consume(lexer.TokenRBrace, "parser: expected '}' to close record type"); err != nil {
		return ast.RecordType{}, err
	}

	return ast.RecordType{Fields: fields}, nil
}

func (p *Parser) parseSchemaField() (ast.SchemaField, error) {
	name, optional, err := p.parseFieldHeader("schema field")
	if err != nil {
		return ast.SchemaField{}, err
	}

	typeRef, err := p.parseTypeReference()
	if err != nil {
		return ast.SchemaField{}, err
	}

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after schema field"); err != nil {
		return ast.SchemaField{}, err
	}

	return ast.SchemaField{
		Name:     name,
		Optional: optional,
		Type:     typeRef,
	}, nil
}

func (p *Parser) parseOutputBlock() (ast.OutputBlock, error) {
	directives, err := p.parseOutputDirective()
	if err != nil {
		return ast.OutputBlock{}, err
	}

	if _, err := p.consume(lexer.TokenLBrace, "parser: expected '{' to start output block"); err != nil {
		return ast.OutputBlock{}, err
	}

	items := []ast.OutputField{}
	for !p.isAtEnd() && p.current().Type != lexer.TokenRBrace {
		field, err := p.parseOutputField()
		if err != nil {
			return ast.OutputBlock{}, err
		}
		items = append(items, field)
	}

	if _, err := p.consume(lexer.TokenRBrace, "parser: expected '}' to close output block"); err != nil {
		return ast.OutputBlock{}, err
	}

	return ast.OutputBlock{
		Directives: directives,
		Items:      items,
	}, nil
}

func (p *Parser) parseOutputDirective() ([]ast.OutputDirective, error) {
	if _, err := p.consume(lexer.TokenLBracket, "parser: expected '[' to start output directive"); err != nil {
		return nil, err
	}

	directive, err := p.parseDirectivePair()
	if err != nil {
		return nil, err
	}

	directives := []ast.OutputDirective{directive}
	for p.current().Type == lexer.TokenComma {
		p.advance()
		nextDirective, err := p.parseDirectivePair()
		if err != nil {
			return nil, err
		}
		directives = append(directives, nextDirective)
	}

	if _, err := p.consume(lexer.TokenRBracket, "parser: expected ']' after output directives"); err != nil {
		return nil, err
	}

	return directives, nil
}

func (p *Parser) parseDirectivePair() (ast.OutputDirective, error) {
	switch p.current().Type {
	case lexer.TokenOutput:
		p.advance()
		if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' after output directive"); err != nil {
			return ast.OutputDirective{}, err
		}

		valueToken := p.current()
		if valueToken.Type != lexer.TokenData && valueToken.Type != lexer.TokenSchema {
			return ast.OutputDirective{}, p.unexpectedTokenError("parser: expected 'data' or 'schema' in output directive")
		}
		p.advance()

		return ast.OutputDirective{
			Kind:  ast.OutputDirectiveOutput,
			Value: valueToken.Lexeme,
		}, nil
	case lexer.TokenSchemaFile:
		p.advance()
		if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' after schema_file directive"); err != nil {
			return ast.OutputDirective{}, err
		}

		pathToken, err := p.consume(lexer.TokenString, "parser: expected string literal in schema_file directive")
		if err != nil {
			return ast.OutputDirective{}, err
		}

		return ast.OutputDirective{
			Kind:  ast.OutputDirectiveSchemaFile,
			Value: pathToken.Lexeme,
		}, nil
	case lexer.TokenSchema:
		p.advance()
		if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' after schema directive"); err != nil {
			return ast.OutputDirective{}, err
		}

		nameToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in schema directive")
		if err != nil {
			return ast.OutputDirective{}, err
		}

		return ast.OutputDirective{
			Kind:  ast.OutputDirectiveSchema,
			Value: nameToken.Lexeme,
		}, nil
	default:
		return ast.OutputDirective{}, p.unexpectedTokenError("parser: expected directive pair")
	}
}

func (p *Parser) parseOutputField() (ast.OutputField, error) {
	name, optional, err := p.parseFieldHeader("output field")
	if err != nil {
		return ast.OutputField{}, err
	}

	value, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return ast.OutputField{}, err
	}

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after output field"); err != nil {
		return ast.OutputField{}, err
	}

	return ast.OutputField{
		Name:     name,
		Optional: optional,
		Value:    value,
	}, nil
}

func (p *Parser) parseFieldHeader(context string) (string, bool, error) {
	nameToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in "+context)
	if err != nil {
		return "", false, err
	}

	optional := false
	if p.current().Type == lexer.TokenQuestion {
		optional = true
		p.advance()
	}

	if _, err := p.consume(lexer.TokenColon, "parser: expected ':' after "+context+" name"); err != nil {
		return "", false, err
	}

	return nameToken.Lexeme, optional, nil
}

func (p *Parser) parseTypeReference() (ast.TypeReference, error) {
	switch p.current().Type {
	case lexer.TokenStringType, lexer.TokenIntType, lexer.TokenFloatType, lexer.TokenBooleanType:
		token := p.current()
		p.advance()
		return ast.PrimitiveType{Name: token.Lexeme}, nil
	case lexer.TokenArray:
		p.advance()
		if _, err := p.consume(lexer.TokenLess, "parser: expected '<' after array type"); err != nil {
			return nil, err
		}
		element, err := p.parseTypeReference()
		if err != nil {
			return nil, err
		}
		if err := p.consumeTypeCloser("parser: expected '>' after array type"); err != nil {
			return nil, err
		}
		return ast.ArrayType{Element: element}, nil
	case lexer.TokenIdentifier:
		token := p.current()
		p.advance()
		return ast.NamedType{Name: token.Lexeme}, nil
	default:
		return nil, p.unexpectedTokenError("parser: expected type reference")
	}
}

func (p *Parser) consumeTypeCloser(message string) error {
	switch p.current().Type {
	case lexer.TokenGreater:
		p.advance()
		return nil
	case lexer.TokenShiftRight:
		token := p.current()
		token.Type = lexer.TokenGreater
		token.Lexeme = ">"
		token.Column++
		p.tokens[p.position] = token
		return nil
	case lexer.TokenShiftRightUnsigned:
		token := p.current()
		token.Type = lexer.TokenShiftRight
		token.Lexeme = ">>"
		token.Column++
		p.tokens[p.position] = token
		return nil
	default:
		return p.unexpectedTokenError(message)
	}
}

func (p *Parser) parseExpression(precedence int) (ast.Expression, error) {
	token := p.current()
	left, err := p.parsePrefix(token)
	if err != nil {
		return nil, err
	}

	for !p.isAtEnd() && precedence < p.currentPrecedence() {
		operator := p.current()
		p.advance()

		if operator.Type == lexer.TokenQuestion {
			left, err = p.parseConditionalExpression(left, operator)
		} else {
			left, err = p.parseInfixExpression(left, operator)
		}

		if err != nil {
			return nil, err
		}
	}

	return left, nil
}

func (p *Parser) parsePrefix(token lexer.Token) (ast.Expression, error) {
	switch token.Type {
	case lexer.TokenIdentifier:
		p.advance()
		return ast.Identifier{Name: token.Lexeme}, nil
	case lexer.TokenString:
		p.advance()
		return ast.StringLiteral{Lexeme: token.Lexeme}, nil
	case lexer.TokenInt:
		p.advance()
		return ast.IntLiteral{Lexeme: token.Lexeme}, nil
	case lexer.TokenFloat:
		p.advance()
		return ast.FloatLiteral{Lexeme: token.Lexeme}, nil
	case lexer.TokenBoolean:
		p.advance()
		return ast.BooleanLiteral{Value: token.Lexeme == "true"}, nil
	case lexer.TokenSelf:
		return p.parseSelfReference()
	case lexer.TokenLBracket:
		return p.parseArrayLiteral()
	case lexer.TokenLBrace:
		return p.parseRecordLiteral()
	case lexer.TokenLParen:
		p.advance()
		expression, err := p.parseExpression(precedenceLowest)
		if err != nil {
			return nil, err
		}
		if _, err := p.consume(lexer.TokenRParen, "parser: expected ')' after expression"); err != nil {
			return nil, err
		}
		return expression, nil
	case lexer.TokenBang, lexer.TokenTilde, lexer.TokenPlus, lexer.TokenMinus:
		p.advance()
		right, err := p.parseExpression(precedencePrefix)
		if err != nil {
			return nil, err
		}
		return ast.PrefixExpression{
			Operator: token.Type,
			Right:    right,
		}, nil
	default:
		return nil, p.unexpectedTokenError("parser: expected expression")
	}
}

func (p *Parser) parseSelfReference() (ast.Expression, error) {
	if _, err := p.consume(lexer.TokenSelf, "parser: expected '$self'"); err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenDot, "parser: expected '.' after $self"); err != nil {
		return nil, err
	}

	firstSegment, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier after $self.")
	if err != nil {
		return nil, err
	}

	segments := []string{firstSegment.Lexeme}
	for p.current().Type == lexer.TokenDot {
		p.advance()
		segment, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier after '.' in self reference")
		if err != nil {
			return nil, err
		}
		segments = append(segments, segment.Lexeme)
	}

	return ast.SelfReference{Path: segments}, nil
}

func (p *Parser) parseArrayLiteral() (ast.Expression, error) {
	if _, err := p.consume(lexer.TokenLBracket, "parser: expected '[' to start array literal"); err != nil {
		return nil, err
	}

	elements := []ast.Expression{}
	if p.current().Type != lexer.TokenRBracket {
		for {
			element, err := p.parseExpression(precedenceLowest)
			if err != nil {
				return nil, err
			}
			elements = append(elements, element)

			if p.current().Type != lexer.TokenComma {
				break
			}
			p.advance()
		}
	}

	if _, err := p.consume(lexer.TokenRBracket, "parser: expected ']' after array literal"); err != nil {
		return nil, err
	}

	return ast.ArrayLiteral{Elements: elements}, nil
}

func (p *Parser) parseRecordLiteral() (ast.Expression, error) {
	if _, err := p.consume(lexer.TokenLBrace, "parser: expected '{' to start record literal"); err != nil {
		return nil, err
	}

	fields := []ast.RecordField{}
	for !p.isAtEnd() && p.current().Type != lexer.TokenRBrace {
		field, err := p.parseRecordField()
		if err != nil {
			return nil, err
		}
		fields = append(fields, field)
	}

	if _, err := p.consume(lexer.TokenRBrace, "parser: expected '}' to close record literal"); err != nil {
		return nil, err
	}

	return ast.RecordLiteral{Fields: fields}, nil
}

func (p *Parser) parseRecordField() (ast.RecordField, error) {
	name, optional, err := p.parseFieldHeader("record field")
	if err != nil {
		return ast.RecordField{}, err
	}

	value, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return ast.RecordField{}, err
	}

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after record field"); err != nil {
		return ast.RecordField{}, err
	}

	return ast.RecordField{
		Name:     name,
		Optional: optional,
		Value:    value,
	}, nil
}

func (p *Parser) parseInfixExpression(left ast.Expression, operator lexer.Token) (ast.Expression, error) {
	precedence := p.precedenceFor(operator.Type)
	rightPrecedence := precedence
	if operator.Type == lexer.TokenDoubleStar {
		rightPrecedence = precedence - 1
	}

	right, err := p.parseExpression(rightPrecedence)
	if err != nil {
		return nil, err
	}

	return ast.InfixExpression{
		Left:     left,
		Operator: operator.Type,
		Right:    right,
	}, nil
}

func (p *Parser) parseConditionalExpression(left ast.Expression, operator lexer.Token) (ast.Expression, error) {
	if operator.Type != lexer.TokenQuestion {
		return nil, p.unexpectedTokenError("parser: expected '?' for conditional expression")
	}

	thenExpression, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenColon, "parser: expected ':' in conditional expression"); err != nil {
		return nil, err
	}

	elseExpression, err := p.parseExpression(precedenceTernary - 1)
	if err != nil {
		return nil, err
	}

	return ast.ConditionalExpression{
		Condition: left,
		Then:      thenExpression,
		Else:      elseExpression,
	}, nil
}

func (p *Parser) consume(tokenType lexer.TokenType, message string) (lexer.Token, error) {
	if p.current().Type != tokenType {
		return lexer.Token{}, p.unexpectedTokenError(message)
	}
	token := p.current()
	p.advance()
	return token, nil
}

func (p *Parser) current() lexer.Token {
	if len(p.tokens) == 0 {
		return lexer.Token{Type: lexer.TokenEOF}
	}

	if p.position >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}

	return p.tokens[p.position]
}

func (p *Parser) advance() {
	if !p.isAtEnd() {
		p.position++
	}
}

func (p *Parser) isAtEnd() bool {
	if len(p.tokens) == 0 {
		return true
	}

	return p.current().Type == lexer.TokenEOF
}

func (p *Parser) currentPrecedence() int {
	return p.precedenceFor(p.current().Type)
}

func (p *Parser) precedenceFor(tokenType lexer.TokenType) int {
	switch tokenType {
	case lexer.TokenQuestion:
		return precedenceTernary
	case lexer.TokenOrOr:
		return precedenceOr
	case lexer.TokenAndAnd:
		return precedenceAnd
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

func (p *Parser) unexpectedTokenError(message string) error {
	token := p.current()
	if token.Type == lexer.TokenEOF {
		return fmt.Errorf("%s: EOF", message)
	}

	sanitizedLexeme := strings.ReplaceAll(token.Lexeme, "\n", "\\n")
	sanitizedLexeme = strings.ReplaceAll(sanitizedLexeme, "\r", "\\r")

	return fmt.Errorf("%s at %d:%d near %q", message, token.Line, token.Column, sanitizedLexeme)
}
