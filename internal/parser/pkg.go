package parser

import (
	"fmt"
	"strings"

	"github.com/louiss0/mace/internal/diagnostic"
	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

const (
	precedenceLowest = iota
	precedenceTernary
	precedenceCoalesce
	precedenceOr
	precedenceAnd
	precedenceBitwiseOr
	precedenceBitwiseXor
	precedenceBitwiseAnd
	precedenceMerge
	precedenceEquality
	precedenceTypeTest
	precedenceRelational
	precedenceShift
	precedenceAdditive
	precedenceMultiplicative
	precedenceExponent
	precedencePrefix
	precedenceMember
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
		return ast.File{}, p.diagnosticError(lexer.Token{}, diagnostic.Code("mace.syntax.missing-expression"), "parser: empty token stream")
	}

	var script *ast.ScriptBlock
	imports := []ast.ImportDeclaration{}
	if p.current().Type == lexer.TokenScriptDelimiter {
		scriptBlock, err := p.parseScriptBlock()
		if err != nil {
			return ast.File{}, err
		}
		imports = append(imports, scriptBlock.Imports...)
		script = &scriptBlock
	}

	if p.current().Type != lexer.TokenLBracket && p.current().Type != lexer.TokenLBrace {
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

func (p *Parser) ParseScriptBlock() (ast.ScriptBlock, error) {
	if len(p.tokens) == 0 {
		return ast.ScriptBlock{}, p.diagnosticError(lexer.Token{}, diagnostic.Code("mace.syntax.missing-expression"), "parser: empty token stream")
	}

	script, err := p.parseScriptBlock()
	if err != nil {
		return ast.ScriptBlock{}, err
	}

	if !p.isAtEnd() {
		return ast.ScriptBlock{}, p.unexpectedTokenError("parser: unexpected token after script block")
	}

	return script, nil
}

func (p *Parser) ParseOutputBlock() (ast.OutputBlock, error) {
	if len(p.tokens) == 0 {
		return ast.OutputBlock{}, p.diagnosticError(lexer.Token{}, diagnostic.Code("mace.syntax.missing-expression"), "parser: empty token stream")
	}

	if p.current().Type != lexer.TokenLBracket && p.current().Type != lexer.TokenLBrace {
		return ast.OutputBlock{}, p.unexpectedTokenError("parser: expected output block")
	}

	output, err := p.parseOutputBlock()
	if err != nil {
		return ast.OutputBlock{}, err
	}

	if !p.isAtEnd() {
		return ast.OutputBlock{}, p.unexpectedTokenError("parser: unexpected token after output block")
	}

	return output, nil
}

func (p *Parser) ParseExpression() (ast.Expression, error) {
	if len(p.tokens) == 0 {
		return nil, p.diagnosticError(lexer.Token{}, diagnostic.Code("mace.syntax.missing-expression"), "parser: empty token stream")
	}

	expression, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return nil, err
	}
	if !p.isAtEnd() {
		return nil, p.unexpectedTokenError("parser: unexpected token after expression")
	}

	return expression, nil
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

	if p.current().Type == lexer.TokenMinus {
		p.advance()
		asToken, err := p.consume(lexer.TokenIdentifier, "parser: expected 'as' after 'import-'")
		if err != nil {
			return ast.ImportDeclaration{}, err
		}
		if asToken.Lexeme != "as" {
			return ast.ImportDeclaration{}, p.unexpectedTokenError("parser: expected 'as' after 'import-'")
		}
		aliasToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier after 'import-as'")
		if err != nil {
			return ast.ImportDeclaration{}, err
		}
		if err := p.consumeImportSeparator(); err != nil {
			return ast.ImportDeclaration{}, err
		}
		imported := ast.ImportedIdentifier{Name: aliasToken.Lexeme}
		return ast.ImportDeclaration{
			Path:     ast.StringLiteral{Token: pathToken, Lexeme: pathToken.Lexeme},
			ImportAs: &imported,
		}, nil
	}

	firstIdentifier, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in import list")
	if err != nil {
		return ast.ImportDeclaration{}, err
	}

	firstImported := ast.ImportedIdentifier{Name: firstIdentifier.Lexeme}
	if p.current().Type == lexer.TokenColon {
		p.advance()
		aliasToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier as alias in import list")
		if err != nil {
			return ast.ImportDeclaration{}, err
		}
		firstImported.Alias = aliasToken.Lexeme
	}

	identifiers := []ast.ImportedIdentifier{firstImported}
	for p.current().Type == lexer.TokenComma {
		p.advance()
		nextIdentifier, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier after ',' in import list")
		if err != nil {
			return ast.ImportDeclaration{}, err
		}
		imported := ast.ImportedIdentifier{Name: nextIdentifier.Lexeme}
		if p.current().Type == lexer.TokenColon {
			p.advance()
			aliasToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier as alias in import list")
			if err != nil {
				return ast.ImportDeclaration{}, err
			}
			imported.Alias = aliasToken.Lexeme
		}
		identifiers = append(identifiers, imported)
	}

	if err := p.consumeImportSeparator(); err != nil {
		return ast.ImportDeclaration{}, err
	}

	return ast.ImportDeclaration{
		Path:        ast.StringLiteral{Token: pathToken, Lexeme: pathToken.Lexeme},
		Identifiers: identifiers,
	}, nil
}

func (p *Parser) consumeImportSeparator() error {
	if p.consumeOptionalToken(lexer.TokenSemicolon) {
		return nil
	}
	if p.current().Type == lexer.TokenScriptDelimiter {
		return nil
	}
	return p.unexpectedTokenError("parser: expected ';' after import declaration")
}

func (p *Parser) parseScriptBlock() (ast.ScriptBlock, error) {
	if _, err := p.consume(lexer.TokenScriptDelimiter, "parser: expected script delimiter"); err != nil {
		return ast.ScriptBlock{}, err
	}

	imports := []ast.ImportDeclaration{}
	for p.current().Type == lexer.TokenFrom {
		importDecl, err := p.parseImportDeclaration()
		if err != nil {
			return ast.ScriptBlock{}, err
		}
		imports = append(imports, importDecl)
	}
	if p.current().Type == lexer.TokenScriptDelimiter && len(imports) == 0 {
		return ast.ScriptBlock{}, p.unexpectedTokenError("parser: empty script block")
	}

	items := []ast.Declaration{}
	for !p.isAtEnd() && p.current().Type != lexer.TokenScriptDelimiter {
		if p.current().Type == lexer.TokenFrom {
			return ast.ScriptBlock{}, p.unexpectedTokenError("parser: import declarations must appear at top of script block")
		}

		declaration, err := p.parseDeclaration()
		if err != nil {
			return ast.ScriptBlock{}, err
		}
		items = append(items, declaration)
	}

	if _, err := p.consume(lexer.TokenScriptDelimiter, "parser: expected closing script delimiter"); err != nil {
		return ast.ScriptBlock{}, err
	}

	return ast.ScriptBlock{Imports: imports, Items: items}, nil
}

func (p *Parser) parseDeclaration() (ast.Declaration, error) {
	switch p.current().Type {
	case lexer.TokenTypeKeyword:
		return p.parseTypeDeclaration()
	case lexer.TokenSchema:
		return p.parseSchemaDeclaration()
	case lexer.TokenGenDoc:
		return p.parseDocDeclaration(ast.DocumentationKindGeneral, lexer.TokenGenDoc, "gen_doc")
	case lexer.TokenSchemaDoc:
		return p.parseDocDeclaration(ast.DocumentationKindSchema, lexer.TokenSchemaDoc, "schema_doc")
	default:
		return p.parseVariableDeclaration()
	}
}

func (p *Parser) parseVariableDeclaration() (ast.Declaration, error) {
	nullable := false
	if p.current().Type == lexer.TokenNullable {
		nullable = true
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

	hasValue := false
	var value ast.Expression
	if p.current().Type == lexer.TokenAssign {
		p.advance()

		value, err = p.parseExpression(precedenceLowest)
		if err != nil {
			return nil, err
		}
		hasValue = true
	} else {
		return nil, p.unexpectedTokenError("parser: expected '=' in variable declaration")
	}

	description := p.parseOptionalInlineDescription()
	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after variable declaration"); err != nil {
		return nil, err
	}

	return ast.VariableDeclaration{
		Nullable:    nullable,
		HasValue:    hasValue,
		Type:        typeRef,
		NameToken:   nameToken,
		Name:        nameToken.Lexeme,
		Value:       value,
		Description: description,
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

	if _, err := p.consume(lexer.TokenColon, "parser: expected ':' in type declaration"); err != nil {
		return nil, err
	}

	typeRef, err := p.parseTypeReference()
	if err != nil {
		return nil, err
	}

	description := p.parseOptionalInlineDescription()

	if _, err := p.consume(lexer.TokenSemicolon, "parser: expected ';' after type declaration"); err != nil {
		return nil, err
	}

	return ast.TypeDeclaration{
		NameToken:   nameToken,
		Name:        nameToken.Lexeme,
		Type:        typeRef,
		Description: description,
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

	if _, err := p.consume(lexer.TokenColon, "parser: expected ':' in schema declaration"); err != nil {
		return nil, err
	}

	recordType, err := p.parseRecordType()
	if err != nil {
		return nil, err
	}

	p.consumeOptionalToken(lexer.TokenSemicolon)

	return ast.SchemaDeclaration{
		NameToken: nameToken,
		Name:      nameToken.Lexeme,
		Type:      recordType,
	}, nil
}

func (p *Parser) parseDocDeclaration(kind ast.DocumentationKind, keywordType lexer.TokenType, keyword string) (ast.Declaration, error) {
	keywordToken, err := p.consume(keywordType, fmt.Sprintf("parser: expected '%s'", keyword))
	if err != nil {
		return nil, err
	}

	targetToken, err := p.consume(lexer.TokenIdentifier, fmt.Sprintf("parser: expected identifier in %s declaration", keyword))
	if err != nil {
		return nil, err
	}

	if _, err := p.consume(lexer.TokenLBrace, fmt.Sprintf("parser: expected '{' to start %s declaration", keyword)); err != nil {
		return nil, err
	}

	documentation := ast.Documentation{Props: map[string]ast.StringLiteral{}}
	seenEntries := map[string]struct{}{}
	for !p.isAtEnd() && p.current().Type != lexer.TokenRBrace {
		entryToken, err := p.consume(lexer.TokenIdentifier, fmt.Sprintf("parser: expected %s entry", keyword))
		if err != nil {
			return nil, err
		}
		entryName := entryToken.Lexeme
		if entryName == "props" {
			entryName = "fields"
		}
		if _, exists := seenEntries[entryName]; exists {
			return nil, p.diagnosticError(entryToken, diagnostic.Code("mace.doc.duplicate-entry"), fmt.Sprintf("parser: duplicate %s entry %q at %d:%d", keyword, entryToken.Lexeme, entryToken.Line, entryToken.Column))
		}
		seenEntries[entryName] = struct{}{}

		if _, err := p.consume(lexer.TokenColon, fmt.Sprintf("parser: expected ':' after %s entry name", keyword)); err != nil {
			return nil, err
		}

		switch entryToken.Lexeme {
		case "summary", "description":
			valueToken, err := p.consume(lexer.TokenString, fmt.Sprintf("parser: expected string literal in %s entry", keyword))
			if err != nil {
				return nil, err
			}

			if err := p.consumePairSeparator(fmt.Sprintf("%s entry", keyword)); err != nil {
				return nil, err
			}

			value := ast.StringLiteral{Token: valueToken, Lexeme: valueToken.Lexeme}
			if entryToken.Lexeme == "summary" {
				documentation.Summary = &value
			} else {
				documentation.Description = &value
			}
		case "props", "fields":
			if kind != ast.DocumentationKindSchema {
				return nil, p.diagnosticError(entryToken, diagnostic.Code("mace.doc.unknown-entry"), fmt.Sprintf("parser: %s entry is only allowed in schema_doc at %d:%d", entryToken.Lexeme, entryToken.Line, entryToken.Column))
			}
			if _, err := p.consume(lexer.TokenLBrace, fmt.Sprintf("parser: expected '{' to start %s entry", entryToken.Lexeme)); err != nil {
				return nil, err
			}

			for !p.isAtEnd() && p.current().Type != lexer.TokenRBrace {
				nameToken, err := p.consume(lexer.TokenIdentifier, fmt.Sprintf("parser: expected identifier in %s entry", entryToken.Lexeme))
				if err != nil {
					return nil, err
				}
				if _, exists := documentation.Props[nameToken.Lexeme]; exists {
					return nil, p.diagnosticError(nameToken, diagnostic.Code("mace.doc.duplicate-entry"), fmt.Sprintf("parser: duplicate %s entry %q at %d:%d", entryToken.Lexeme, nameToken.Lexeme, nameToken.Line, nameToken.Column))
				}
				if _, err := p.consume(lexer.TokenColon, fmt.Sprintf("parser: expected ':' after %s entry name", entryToken.Lexeme)); err != nil {
					return nil, err
				}
				valueToken, err := p.consume(lexer.TokenString, fmt.Sprintf("parser: expected string literal in %s entry", entryToken.Lexeme))
				if err != nil {
					return nil, err
				}
				if err := p.consumePairSeparator(fmt.Sprintf("%s entry", entryToken.Lexeme)); err != nil {
					return nil, err
				}
				documentation.Props[nameToken.Lexeme] = ast.StringLiteral{Token: valueToken, Lexeme: valueToken.Lexeme}
			}

			if _, err := p.consume(lexer.TokenRBrace, fmt.Sprintf("parser: expected '}' to close %s entry", entryToken.Lexeme)); err != nil {
				return nil, err
			}
			if err := p.consumePairSeparator(fmt.Sprintf("%s entry", entryToken.Lexeme)); err != nil {
				return nil, err
			}
		default:
			return nil, p.diagnosticError(entryToken, diagnostic.Code("mace.doc.unknown-entry"), fmt.Sprintf("parser: unknown %s entry %q at %d:%d", keyword, entryToken.Lexeme, entryToken.Line, entryToken.Column))
		}
	}

	if _, err := p.consume(lexer.TokenRBrace, fmt.Sprintf("parser: expected '}' to close %s declaration", keyword)); err != nil {
		return nil, err
	}
	if _, err := p.consume(lexer.TokenSemicolon, fmt.Sprintf("parser: expected ';' after %s declaration", keyword)); err != nil {
		return nil, err
	}

	return ast.DocDeclaration{Kind: kind, KeywordToken: keywordToken, TargetToken: targetToken, Target: targetToken.Lexeme, Documentation: documentation}, nil
}

func (p *Parser) parseRecordType() (ast.RecordType, error) {
	startToken, err := p.consume(lexer.TokenLBrace, "parser: expected '{' to start record type")
	if err != nil {
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

	return ast.RecordType{StartToken: startToken, Fields: fields}, nil
}

func (p *Parser) parseSchemaField() (ast.SchemaField, error) {
	nameToken, name, optional, err := p.parseFieldHeader("schema field")
	if err != nil {
		return ast.SchemaField{}, err
	}

	typeRef, err := p.parseTypeReference()
	if err != nil {
		return ast.SchemaField{}, err
	}

	description := p.parseOptionalInlineDescription()

	trailingDescription, trailingToken, err := p.consumeRecordSeparatorWithInlineDescription("schema field")
	if err != nil {
		return ast.SchemaField{}, err
	}

	description, err = p.mergeInlineDescriptions("schema field", description, trailingDescription, trailingToken)
	if err != nil {
		return ast.SchemaField{}, err
	}

	return ast.SchemaField{
		NameToken:   nameToken,
		Name:        name,
		Optional:    optional,
		Type:        typeRef,
		Description: description,
	}, nil
}

func (p *Parser) parseOutputBlock() (ast.OutputBlock, error) {
	directives := []ast.OutputDirective{}
	if p.current().Type == lexer.TokenLBracket {
		parsedDirectives, err := p.parseOutputDirective()
		if err != nil {
			return ast.OutputBlock{}, err
		}
		directives = parsedDirectives
	}

	var doc *ast.StringLiteral
	if len(directives) > 0 && p.current().Type == lexer.TokenString {
		if !strings.HasPrefix(p.current().Lexeme, `"""`) {
			return ast.OutputBlock{}, p.unexpectedTokenError("parser: expected multiline string doc block")
		}
		parsed := ast.StringLiteral{Token: p.current(), Lexeme: p.current().Lexeme}
		doc = &parsed
		p.advance()
	}

	mode := outputModeFromDirectives(directives)

	if _, err := p.consume(lexer.TokenLBrace, "parser: expected '{' to start output block"); err != nil {
		return ast.OutputBlock{}, err
	}

	dataFields := []ast.OutputField{}
	schemaFields := []ast.OutputSchemaField{}
	for !p.isAtEnd() && p.current().Type != lexer.TokenRBrace {
		if mode == ast.OutputModeSchema {
			field, err := p.parseOutputSchemaField()
			if err != nil {
				return ast.OutputBlock{}, err
			}
			schemaFields = append(schemaFields, field)
			continue
		}

		field, err := p.parseOutputField()
		if err != nil {
			return ast.OutputBlock{}, err
		}
		dataFields = append(dataFields, field)
	}

	if _, err := p.consume(lexer.TokenRBrace, "parser: expected '}' to close output block"); err != nil {
		return ast.OutputBlock{}, err
	}

	return ast.OutputBlock{
		Directives:   directives,
		Doc:          doc,
		Mode:         mode,
		DataFields:   dataFields,
		SchemaFields: schemaFields,
	}, nil
}

func outputModeFromDirectives(directives []ast.OutputDirective) ast.OutputMode {
	for _, directive := range directives {
		if directive.Kind == ast.OutputDirectiveOutput && directive.Value == "schema" {
			return ast.OutputModeSchema
		}
	}

	return ast.OutputModeData
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
	case lexer.TokenParse:
		p.advance()
		if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' after parse directive"); err != nil {
			return ast.OutputDirective{}, err
		}

		nameToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier in parse directive")
		if err != nil {
			return ast.OutputDirective{}, err
		}

		return ast.OutputDirective{
			Kind:  ast.OutputDirectiveParse,
			Value: nameToken.Lexeme,
		}, nil
	case lexer.TokenParseFile:
		p.advance()
		if _, err := p.consume(lexer.TokenAssign, "parser: expected '=' after parse_file directive"); err != nil {
			return ast.OutputDirective{}, err
		}

		pathToken, err := p.consume(lexer.TokenString, "parser: expected string literal in parse_file directive")
		if err != nil {
			return ast.OutputDirective{}, err
		}

		return ast.OutputDirective{
			Kind:  ast.OutputDirectiveParseFile,
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
	if p.isFieldShorthandStart() {
		return p.parseOutputFieldShorthand()
	}

	nameToken, name, optional, err := p.parseFieldHeader("output field")
	if err != nil {
		return ast.OutputField{}, err
	}

	value, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return ast.OutputField{}, err
	}

	description := p.parseOptionalInlineDescription()

	trailingDescription, trailingToken, err := p.consumeRecordSeparatorWithInlineDescription("output field")
	if err != nil {
		return ast.OutputField{}, err
	}

	description, err = p.mergeInlineDescriptions("output field", description, trailingDescription, trailingToken)
	if err != nil {
		return ast.OutputField{}, err
	}

	return ast.OutputField{
		NameToken:   nameToken,
		Name:        name,
		Optional:    optional,
		Value:       value,
		Description: description,
	}, nil
}

func (p *Parser) parseOutputFieldShorthand() (ast.OutputField, error) {
	nameToken, description, err := p.parseFieldShorthand("output field")
	if err != nil {
		return ast.OutputField{}, err
	}

	return ast.OutputField{
		NameToken:   nameToken,
		Name:        nameToken.Lexeme,
		Shorthand:   true,
		Value:       ast.Identifier{Name: nameToken.Lexeme},
		Description: description,
	}, nil
}

func (p *Parser) parseOutputSchemaField() (ast.OutputSchemaField, error) {
	nameToken, name, optional, err := p.parseFieldHeader("output schema field")
	if err != nil {
		return ast.OutputSchemaField{}, err
	}

	typeRef, err := p.parseTypeReference()
	if err != nil {
		return ast.OutputSchemaField{}, err
	}

	description := p.parseOptionalInlineDescription()

	trailingDescription, trailingToken, err := p.consumeRecordSeparatorWithInlineDescription("output schema field")
	if err != nil {
		return ast.OutputSchemaField{}, err
	}

	description, err = p.mergeInlineDescriptions("output schema field", description, trailingDescription, trailingToken)
	if err != nil {
		return ast.OutputSchemaField{}, err
	}

	return ast.OutputSchemaField{
		NameToken:   nameToken,
		Name:        name,
		Optional:    optional,
		Type:        typeRef,
		Description: description,
	}, nil
}

func (p *Parser) parseOptionalInlineDescription() string {
	if p.current().Type != lexer.TokenInlineDescription {
		return ""
	}

	description := p.current().Lexeme
	p.advance()
	return description
}

func (p *Parser) parseFieldHeader(context string) (lexer.Token, string, bool, error) {
	nameToken := p.current()
	if !isFieldNameToken(nameToken.Type) {
		return lexer.Token{}, "", false, p.unexpectedTokenError("parser: expected identifier in " + context)
	}
	p.advance()

	optional := false
	if p.current().Type == lexer.TokenQuestion {
		optional = true
		p.advance()
	}

	if _, err := p.consume(lexer.TokenColon, "parser: expected ':' after "+context+" name"); err != nil {
		return lexer.Token{}, "", false, err
	}

	return nameToken, nameToken.Lexeme, optional, nil
}

func isFieldNameToken(tokenType lexer.TokenType) bool {
	switch tokenType {
	case lexer.TokenIdentifier, lexer.TokenTypeKeyword, lexer.TokenSchema, lexer.TokenOutput, lexer.TokenParse, lexer.TokenParseFile, lexer.TokenSchemaFile, lexer.TokenData, lexer.TokenFrom, lexer.TokenImport, lexer.TokenRecord:
		return true
	default:
		return false
	}
}

func (p *Parser) parseTypeReference() (ast.TypeReference, error) {
	switch p.current().Type {
	case lexer.TokenStringType, lexer.TokenIntType, lexer.TokenFloatType, lexer.TokenHexIntType, lexer.TokenHexFloatType, lexer.TokenBooleanType:
		token := p.current()
		p.advance()
		return ast.PrimitiveType{Token: token, Name: token.Lexeme}, nil
	case lexer.TokenArray:
		token := p.current()
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
		return ast.ArrayType{Token: token, Element: element}, nil
	case lexer.TokenRecord:
		token := p.current()
		p.advance()
		if _, err := p.consume(lexer.TokenLess, "parser: expected '<' after record type"); err != nil {
			return nil, err
		}
		valueType, err := p.parseTypeReference()
		if err != nil {
			return nil, err
		}
		if err := p.consumeTypeCloser("parser: expected '>' after record type"); err != nil {
			return nil, err
		}
		return ast.RecordMapType{Token: token, Value: valueType}, nil
	case lexer.TokenUnion:
		token := p.current()
		p.advance()
		if _, err := p.consume(lexer.TokenLBracket, "parser: expected '[' after fusion type"); err != nil {
			return nil, err
		}
		members := []ast.TypeReference{}
		for {
			member, err := p.parseTypeReference()
			if err != nil {
				return nil, err
			}
			members = append(members, member)
			if p.current().Type != lexer.TokenComma {
				break
			}
			p.advance()
		}
		if _, err := p.consume(lexer.TokenRBracket, "parser: expected ']' after fusion type"); err != nil {
			return nil, err
		}
		return ast.UnionType{Token: token, Members: members}, nil
	case lexer.TokenVariant:
		token := p.current()
		p.advance()
		if _, err := p.consume(lexer.TokenLBracket, "parser: expected '[' after variant type"); err != nil {
			return nil, err
		}
		members := []ast.TypeReference{}
		for {
			member, err := p.parseTypeReference()
			if err != nil {
				return nil, err
			}
			members = append(members, member)
			if p.current().Type != lexer.TokenComma {
				break
			}
			p.advance()
		}
		if _, err := p.consume(lexer.TokenRBracket, "parser: expected ']' after variant type"); err != nil {
			return nil, err
		}
		return ast.VariantType{Token: token, Members: members}, nil
	case lexer.TokenChoice:
		return p.parseChoiceType()
	case lexer.TokenLBrace:
		return p.parseRecordType()
	case lexer.TokenIdentifier, lexer.TokenTypeKeyword:
		token := p.current()
		p.advance()
		return ast.NamedType{Token: token, Name: token.Lexeme}, nil
	default:
		return nil, p.unexpectedTokenError("parser: expected type reference")
	}
}

func (p *Parser) parseChoiceType() (ast.TypeReference, error) {
	token, err := p.consume(lexer.TokenChoice, "parser: expected 'choice'")
	if err != nil {
		return nil, err
	}
	if _, err := p.consume(lexer.TokenLBracket, "parser: expected '[' after choice type"); err != nil {
		return nil, err
	}

	members := []ast.Expression{}
	for {
		member, err := p.parseChoiceMember()
		if err != nil {
			return nil, err
		}
		members = append(members, member)
		if p.current().Type != lexer.TokenComma {
			break
		}
		p.advance()
	}

	if _, err := p.consume(lexer.TokenRBracket, "parser: expected ']' after choice type"); err != nil {
		return nil, err
	}

	return ast.ChoiceType{Token: token, Members: members}, nil
}

func (p *Parser) parseChoiceMember() (ast.Expression, error) {
	switch token := p.current(); token.Type {
	case lexer.TokenString:
		p.advance()
		return ast.StringLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenInt:
		p.advance()
		return ast.IntLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenFloat:
		p.advance()
		return ast.FloatLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenHexInt:
		p.advance()
		return ast.HexIntLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenHexFloat:
		p.advance()
		return ast.HexFloatLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenBoolean:
		p.advance()
		return ast.BooleanLiteral{Token: token, Value: token.Lexeme == "true"}, nil
	case lexer.TokenIdentifier, lexer.TokenTypeKeyword, lexer.TokenRecord:
		p.advance()
		return ast.Identifier{Token: token, Name: token.Lexeme}, nil
	default:
		return nil, p.unexpectedTokenError("parser: expected literal or choice name")
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
		} else if operator.Type == lexer.TokenCoalesce {
			left, err = p.parseCoalesceExpression(left, operator)
		} else if operator.Type == lexer.TokenIs {
			left, err = p.parseTypeTestExpression(left, operator)
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
	case lexer.TokenIdentifier, lexer.TokenTypeKeyword, lexer.TokenRecord:
		p.advance()
		return ast.Identifier{Token: token, Name: token.Lexeme}, nil
	case lexer.TokenString:
		p.advance()
		return ast.StringLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenInt:
		p.advance()
		return ast.IntLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenFloat:
		p.advance()
		return ast.FloatLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenHexInt:
		p.advance()
		return ast.HexIntLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenHexFloat:
		p.advance()
		return ast.HexFloatLiteral{Token: token, Lexeme: token.Lexeme}, nil
	case lexer.TokenBoolean:
		p.advance()
		return ast.BooleanLiteral{Token: token, Value: token.Lexeme == "true"}, nil
	case lexer.TokenNull:
		p.advance()
		return ast.NullLiteral{Token: token}, nil
	case lexer.TokenSelf:
		return p.parseSelfReference()
	case lexer.TokenDollar:
		return p.parseDollarReference()
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
			OperatorToken: token,
			Operator:      token.Type,
			Right:         right,
		}, nil
	default:
		return nil, p.unexpectedTokenError("parser: expected expression")
	}
}

func (p *Parser) parseSelfReference() (ast.Expression, error) {
	selfToken, err := p.consume(lexer.TokenSelf, "parser: expected '$self'")
	if err != nil {
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

	return ast.SelfReference{Token: selfToken, Path: segments}, nil
}

func (p *Parser) parseDollarReference() (ast.Expression, error) {
	dollarToken, err := p.consume(lexer.TokenDollar, "parser: expected '$' to start parsed variable reference")
	if err != nil {
		return nil, err
	}

	nameToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier after '$'")
	if err != nil {
		return nil, err
	}

	identifierToken := lexer.Token{
		Type:   lexer.TokenIdentifier,
		Lexeme: "$" + nameToken.Lexeme,
		Line:   dollarToken.Line,
		Column: dollarToken.Column,
	}
	return ast.Identifier{Token: identifierToken, Name: identifierToken.Lexeme}, nil
}

func (p *Parser) parseArrayLiteral() (ast.Expression, error) {
	startToken, err := p.consume(lexer.TokenLBracket, "parser: expected '[' to start array literal")
	if err != nil {
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

	return ast.ArrayLiteral{StartToken: startToken, Elements: elements}, nil
}

func (p *Parser) parseRecordLiteral() (ast.Expression, error) {
	startToken, err := p.consume(lexer.TokenLBrace, "parser: expected '{' to start record literal")
	if err != nil {
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

	return ast.RecordLiteral{StartToken: startToken, Fields: fields}, nil
}

func (p *Parser) parseRecordField() (ast.RecordField, error) {
	if p.isFieldShorthandStart() {
		return p.parseRecordFieldShorthand()
	}

	nameToken, name, optional, err := p.parseFieldHeader("record field")
	if err != nil {
		return ast.RecordField{}, err
	}

	value, err := p.parseExpression(precedenceLowest)
	if err != nil {
		return ast.RecordField{}, err
	}
	description := p.parseOptionalInlineDescription()

	trailingDescription, trailingToken, err := p.consumeRecordSeparatorWithInlineDescription("record field")
	if err != nil {
		return ast.RecordField{}, err
	}

	if _, err := p.mergeInlineDescriptions("record field", description, trailingDescription, trailingToken); err != nil {
		return ast.RecordField{}, err
	}

	return ast.RecordField{
		NameToken: nameToken,
		Name:      name,
		Optional:  optional,
		Value:     value,
	}, nil
}

func (p *Parser) parseRecordFieldShorthand() (ast.RecordField, error) {
	nameToken, _, err := p.parseFieldShorthand("record field")
	if err != nil {
		return ast.RecordField{}, err
	}

	return ast.RecordField{
		NameToken: nameToken,
		Name:      nameToken.Lexeme,
		Shorthand: true,
		Value:     ast.Identifier{Name: nameToken.Lexeme},
	}, nil
}

func (p *Parser) parseFieldShorthand(context string) (lexer.Token, string, error) {
	nameToken := p.current()
	p.advance()

	description := p.parseOptionalInlineDescription()
	trailingDescription, trailingToken, err := p.consumeRecordSeparatorWithInlineDescription(context)
	if err != nil {
		return lexer.Token{}, "", err
	}

	description, err = p.mergeInlineDescriptions(context, description, trailingDescription, trailingToken)
	if err != nil {
		return lexer.Token{}, "", err
	}

	return nameToken, description, nil
}

func (p *Parser) isFieldShorthandStart() bool {
	nameToken := p.current()
	if !isFieldNameToken(nameToken.Type) {
		return false
	}

	if p.position+1 >= len(p.tokens) {
		return false
	}

	nextToken := p.tokens[p.position+1]
	switch nextToken.Type {
	case lexer.TokenComma, lexer.TokenRBrace, lexer.TokenInlineDescription:
		return true
	default:
		return false
	}
}

func (p *Parser) consumeOptionalToken(tokenType lexer.TokenType) bool {
	if p.current().Type != tokenType {
		return false
	}

	p.advance()
	return true
}

func (p *Parser) consumePairSeparator(context string) error {
	switch p.current().Type {
	case lexer.TokenComma:
		p.advance()
		return nil
	default:
		return p.unexpectedTokenError(fmt.Sprintf("parser: expected ',' after %s", context))
	}
}

func (p *Parser) consumeRecordSeparator(context string) error {
	switch p.current().Type {
	case lexer.TokenComma:
		p.advance()
		return nil
	case lexer.TokenRBrace:
		return nil
	default:
		return p.unexpectedTokenError(fmt.Sprintf("parser: expected ',' after %s", context))
	}
}

func (p *Parser) consumeRecordSeparatorWithInlineDescription(context string) (string, lexer.Token, error) {
	if err := p.consumeRecordSeparator(context); err != nil {
		return "", lexer.Token{}, err
	}

	if p.current().Type != lexer.TokenInlineDescription {
		return "", lexer.Token{}, nil
	}

	descriptionToken := p.current()
	p.advance()
	return descriptionToken.Lexeme, descriptionToken, nil
}

func (p *Parser) mergeInlineDescriptions(context string, leading string, trailing string, trailingToken lexer.Token) (string, error) {
	if leading != "" && trailing != "" {
		return "", p.diagnosticError(trailingToken, diagnostic.Code("mace.doc.duplicate-inline-description"), fmt.Sprintf("parser: duplicate inline description on %s at %d:%d", context, trailingToken.Line, trailingToken.Column))
	}

	if trailing != "" {
		return trailing, nil
	}

	return leading, nil
}

func (p *Parser) parseInfixExpression(left ast.Expression, operator lexer.Token) (ast.Expression, error) {
	if operator.Type == lexer.TokenDot || operator.Type == lexer.TokenOptionalDot {
		memberToken, err := p.consume(lexer.TokenIdentifier, "parser: expected identifier after member access operator")
		if err != nil {
			return nil, err
		}
		return ast.MemberAccess{Target: left, Name: memberToken.Lexeme, Optional: operator.Type == lexer.TokenOptionalDot}, nil
	}

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

func (p *Parser) parseTypeTestExpression(left ast.Expression, operator lexer.Token) (ast.Expression, error) {
	if _, chained := left.(ast.TypeTestExpression); chained {
		return nil, p.diagnosticError(operator, diagnostic.Code("mace.syntax.invalid-is-type"), "parser: chained 'is' expressions are not allowed")
	}
	if p.current().Type == lexer.TokenEOF {
		return nil, p.diagnosticError(p.current(), diagnostic.Code("mace.syntax.is-missing-type"), "parser: expected type reference after 'is'")
	}

	targetType, err := p.parseTypeReference()
	if err != nil {
		return nil, p.diagnosticError(p.current(), diagnostic.Code("mace.syntax.invalid-is-type"), "parser: expected valid type reference after 'is'")
	}

	return ast.TypeTestExpression{
		Expression: left,
		TargetType: targetType,
		EndToken:   p.tokens[p.position-1],
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

func (p *Parser) parseCoalesceExpression(left ast.Expression, operator lexer.Token) (ast.Expression, error) {
	right, err := p.parseExpression(precedenceCoalesce - 1)
	if err != nil {
		return nil, err
	}

	return ast.InfixExpression{Left: left, Operator: operator.Type, Right: right}, nil
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
	case lexer.TokenCoalesce:
		return precedenceCoalesce
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
	case lexer.TokenEqualEqual, lexer.TokenNotEqual:
		return precedenceEquality
	case lexer.TokenIs:
		return precedenceTypeTest
	case lexer.TokenMerge:
		return precedenceMerge
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual, lexer.TokenIn:
		return precedenceRelational
	case lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned:
		return precedenceShift
	case lexer.TokenPlus, lexer.TokenMinus:
		return precedenceAdditive
	case lexer.TokenStar, lexer.TokenSlash, lexer.TokenPercent:
		return precedenceMultiplicative
	case lexer.TokenDoubleStar:
		return precedenceExponent
	case lexer.TokenDot, lexer.TokenOptionalDot:
		return precedenceMember
	default:
		return precedenceLowest
	}
}
