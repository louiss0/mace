package parser

import (
	"fmt"
	"strings"

	"github.com/louiss0/mace/internal/diagnostic"
	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

func (p *Parser) diagnosticError(token lexer.Token, code diagnostic.Code, message string) error {
	return diagnostic.Error{
		Kind:     "syntax",
		Code:     code,
		Message:  message,
		Range:    diagnostic.FromASTRange(ast.TokenRange(token)),
		Severity: diagnostic.SeverityError,
	}
}

func (p *Parser) unexpectedTokenError(message string) error {
	return p.unexpectedTokenErrorAt(p.current(), message)
}

func (p *Parser) unexpectedTokenErrorAt(token lexer.Token, message string) error {
	if token.Type == lexer.TokenEOF {
		formatted := fmt.Sprintf("%s: EOF", message)
		return p.diagnosticError(token, parserDiagnosticCode(formatted), formatted)
	}

	sanitizedLexeme := strings.ReplaceAll(token.Lexeme, "\n", "\\n")
	sanitizedLexeme = strings.ReplaceAll(sanitizedLexeme, "\r", "\\r")
	formatted := fmt.Sprintf("%s at %d:%d near %q", message, token.Line, token.Column, sanitizedLexeme)
	return p.diagnosticError(token, parserDiagnosticCode(formatted), formatted)
}

func parserDiagnosticCode(message string) diagnostic.Code {
	switch {
	case strings.Contains(message, "empty script block"):
		return diagnostic.Code("mace.syntax.empty-script-block")
	case strings.Contains(message, "expected closing script delimiter") && strings.Contains(message, "EOF"):
		return diagnostic.Code("mace.syntax.unterminated-script-block")
	case strings.Contains(message, "script delimiter") || strings.Contains(message, "script block delimiters"):
		return diagnostic.Code("mace.syntax.inconsistent-script-delimiters")
	case strings.Contains(message, "import declarations must appear at top of script block") || strings.Contains(message, "expected 'from'") || strings.Contains(message, "expected string literal in import") || strings.Contains(message, "expected 'import'"):
		return diagnostic.Code("mace.syntax.malformed-import")
	case strings.Contains(message, "directive"):
		return diagnostic.Code("mace.syntax.malformed-directive-list")
	case strings.Contains(message, "schema declaration") || strings.Contains(message, "record type") || strings.Contains(message, "schema field"):
		return diagnostic.Code("mace.syntax.malformed-schema")
	case strings.Contains(message, "expected expression"):
		return diagnostic.Code("mace.syntax.missing-expression")
	case strings.Contains(message, "expected ':' in conditional expression"):
		return diagnostic.Code("mace.syntax.conditional-missing-colon")
	case strings.Contains(message, "duplicate inline description"):
		return diagnostic.Code("mace.doc.duplicate-inline-description")
	case strings.Contains(message, "duplicate ") && strings.Contains(message, " entry"):
		return diagnostic.Code("mace.doc.duplicate-entry")
	case strings.Contains(message, "unknown ") && strings.Contains(message, " entry"):
		return diagnostic.Code("mace.doc.unknown-entry")
	default:
		return diagnostic.Code("mace.syntax.unexpected-token")
	}
}
