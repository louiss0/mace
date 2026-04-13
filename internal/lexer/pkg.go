package lexer

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type Lexer struct {
	input    string
	position int
	line     int
	column   int
}

func New(input string) *Lexer {
	return &Lexer{
		input:  input,
		line:   1,
		column: 1,
	}
}

func (l *Lexer) NextToken() (Token, error) {
	if err := l.skipWhitespaceAndComments(); err != nil {
		return Token{}, err
	}

	if l.isAtEnd() {
		return Token{
			Type:   TokenEOF,
			Lexeme: "",
			Line:   l.line,
			Column: l.column,
		}, nil
	}

	startPosition := l.position
	startLine := l.line
	startColumn := l.column
	current := l.advance()

	switch current {
	case '\'', '"':
		return l.lexString(current, startPosition, startLine, startColumn)
	case '$':
		if l.match('s') && l.match('e') && l.match('l') && l.match('f') {
			return l.makeToken(TokenSelf, startPosition, startLine, startColumn), nil
		}
		return Token{}, fmt.Errorf("lexer: unexpected character %q at %d:%d", current, startLine, startColumn)
	case '=':
		if l.match('=') {
			if l.match('=') {
				return l.makeToken(TokenStrictEqual, startPosition, startLine, startColumn), nil
			}
			return l.makeToken(TokenEqualEqual, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenAssign, startPosition, startLine, startColumn), nil
	case ';':
		return l.makeToken(TokenSemicolon, startPosition, startLine, startColumn), nil
	case ',':
		return l.makeToken(TokenComma, startPosition, startLine, startColumn), nil
	case ':':
		return l.makeToken(TokenColon, startPosition, startLine, startColumn), nil
	case '?':
		return l.makeToken(TokenQuestion, startPosition, startLine, startColumn), nil
	case '.':
		return l.makeToken(TokenDot, startPosition, startLine, startColumn), nil
	case '+':
		return l.makeToken(TokenPlus, startPosition, startLine, startColumn), nil
	case '-':
		return l.makeToken(TokenMinus, startPosition, startLine, startColumn), nil
	case '*':
		if l.match('*') {
			return l.makeToken(TokenDoubleStar, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenStar, startPosition, startLine, startColumn), nil
	case '/':
		if l.match('#') {
			for l.peek() == ' ' || l.peek() == '\t' {
				l.advance()
			}
			for !l.isAtEnd() {
				if l.peek() == ';' || l.peek() == '\n' || l.peek() == '\r' {
					break
				}
				l.advance()
			}
			return Token{
				Type:   TokenInlineDescription,
				Lexeme: strings.TrimSpace(l.input[startPosition+2:l.position]),
				Line:   startLine,
				Column: startColumn,
			}, nil
		}
		return l.makeToken(TokenSlash, startPosition, startLine, startColumn), nil
	case '%':
		return l.makeToken(TokenPercent, startPosition, startLine, startColumn), nil
	case '!':
		if l.match('=') {
			if l.match('=') {
				return l.makeToken(TokenStrictNotEqual, startPosition, startLine, startColumn), nil
			}
			return l.makeToken(TokenNotEqual, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenBang, startPosition, startLine, startColumn), nil
	case '~':
		return l.makeToken(TokenTilde, startPosition, startLine, startColumn), nil
	case '<':
		if l.match('=') {
			return l.makeToken(TokenLessEqual, startPosition, startLine, startColumn), nil
		}
		if l.match('<') {
			return l.makeToken(TokenShiftLeft, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenLess, startPosition, startLine, startColumn), nil
	case '>':
		if l.match('=') {
			return l.makeToken(TokenGreaterEqual, startPosition, startLine, startColumn), nil
		}
		if l.match('>') {
			if l.match('>') {
				return l.makeToken(TokenShiftRightUnsigned, startPosition, startLine, startColumn), nil
			}
			return l.makeToken(TokenShiftRight, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenGreater, startPosition, startLine, startColumn), nil
	case '&':
		if l.match('&') {
			return l.makeToken(TokenAndAnd, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenAmpersand, startPosition, startLine, startColumn), nil
	case '^':
		return l.makeToken(TokenCaret, startPosition, startLine, startColumn), nil
	case '|':
		if l.isScriptDelimiterStart(startPosition) {
			for l.peek() == '=' {
				l.advance()
			}
			if l.peek() == '|' {
				l.advance()
				return l.makeToken(TokenScriptDelimiter, startPosition, startLine, startColumn), nil
			}
		}
		if l.match('|') {
			return l.makeToken(TokenOrOr, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenPipe, startPosition, startLine, startColumn), nil
	case '(':
		return l.makeToken(TokenLParen, startPosition, startLine, startColumn), nil
	case ')':
		return l.makeToken(TokenRParen, startPosition, startLine, startColumn), nil
	case '{':
		return l.makeToken(TokenLBrace, startPosition, startLine, startColumn), nil
	case '}':
		return l.makeToken(TokenRBrace, startPosition, startLine, startColumn), nil
	case '[':
		return l.makeToken(TokenLBracket, startPosition, startLine, startColumn), nil
	case ']':
		return l.makeToken(TokenRBracket, startPosition, startLine, startColumn), nil
	}

	if isLetter(current) {
		for isLetter(l.peek()) || isDigit(l.peek()) || l.peek() == '_' {
			l.advance()
		}
		lexeme := l.input[startPosition:l.position]
		if tokenType, ok := keywordToken(lexeme); ok {
			return Token{
				Type:   tokenType,
				Lexeme: lexeme,
				Line:   startLine,
				Column: startColumn,
			}, nil
		}
		return Token{
			Type:   TokenIdentifier,
			Lexeme: lexeme,
			Line:   startLine,
			Column: startColumn,
		}, nil
	}

	if isDigit(current) {
		for isDigit(l.peek()) {
			l.advance()
		}

		if l.peek() == '.' && isDigit(l.peekNext()) {
			l.advance()
			for isDigit(l.peek()) {
				l.advance()
			}
			return l.makeToken(TokenFloat, startPosition, startLine, startColumn), nil
		}
		return l.makeToken(TokenInt, startPosition, startLine, startColumn), nil
	}

	return Token{}, fmt.Errorf("lexer: unexpected character %q at %d:%d", current, startLine, startColumn)
}

func (l *Lexer) lexString(quote byte, startPosition, startLine, startColumn int) (Token, error) {
	if quote == '"' && strings.HasPrefix(l.input[l.position:], `""`) {
		l.advance()
		l.advance()
		for !l.isAtEnd() {
			if strings.HasPrefix(l.input[l.position:], `"""`) {
				l.advance()
				l.advance()
				l.advance()
				return l.makeToken(TokenString, startPosition, startLine, startColumn), nil
			}
			if l.peek() == '\\' {
				l.advance()
				if l.isAtEnd() {
					break
				}
			}
			l.advance()
		}
		return Token{}, fmt.Errorf("lexer: unterminated string at %d:%d", startLine, startColumn)
	}

	for !l.isAtEnd() {
		if l.peek() == '\\' {
			l.advance()
			if l.isAtEnd() {
				break
			}
			l.advance()
			continue
		}
		if l.peek() == quote {
			l.advance()
			return l.makeToken(TokenString, startPosition, startLine, startColumn), nil
		}
		if l.peek() == '\n' || l.peek() == '\r' {
			break
		}
		l.advance()
	}

	return Token{}, fmt.Errorf("lexer: unterminated string at %d:%d", startLine, startColumn)
}

func (l *Lexer) skipWhitespaceAndComments() error {
	for {
		if l.isAtEnd() {
			return nil
		}

		switch l.peek() {
		case ' ', '\t':
			l.advance()
			continue
		case '\n', '\r':
			l.advance()
			continue
		case '/':
			if l.peekNext() != '=' {
				return nil
			}
			if err := l.skipComment(); err != nil {
				return err
			}
			continue
		default:
			return nil
		}
	}
}

func (l *Lexer) skipComment() error {
	startLine := l.line
	startColumn := l.column
	l.advance()
	l.advance()

	if l.shouldUseBlockComment() {
		for !l.isAtEnd() {
			if l.peek() == '=' && l.peekNext() == '/' {
				l.advance()
				l.advance()
				return nil
			}
			l.advance()
		}
		return fmt.Errorf("lexer: unterminated block comment at %d:%d", startLine, startColumn)
	}

	for !l.isAtEnd() && l.peek() != '\n' && l.peek() != '\r' {
		l.advance()
	}

	return nil
}

func (l *Lexer) shouldUseBlockComment() bool {
	rest := l.input[l.position:]
	blockEnd := strings.Index(rest, "=/")
	if blockEnd == -1 {
		return false
	}

	lineEnd := strings.IndexAny(rest, "\r\n")
	if lineEnd == -1 {
		return true
	}

	return blockEnd < lineEnd
}

func (l *Lexer) isScriptDelimiterStart(startPosition int) bool {
	if l.input[startPosition] != '|' {
		return false
	}

	index := startPosition + 1
	equalsCount := 0
	for index < len(l.input) && l.input[index] == '=' {
		equalsCount++
		index++
	}

	if equalsCount < 3 {
		return false
	}

	return index < len(l.input) && l.input[index] == '|'
}

func (l *Lexer) makeToken(tokenType TokenType, startPosition, startLine, startColumn int) Token {
	return Token{
		Type:   tokenType,
		Lexeme: l.input[startPosition:l.position],
		Line:   startLine,
		Column: startColumn,
	}
}

func (l *Lexer) isAtEnd() bool {
	return l.position >= len(l.input)
}

func (l *Lexer) advance() byte {
	if l.isAtEnd() {
		return 0
	}

	current, size := utf8.DecodeRuneInString(l.input[l.position:])
	if current == utf8.RuneError && size == 1 {
		l.position++
		l.column++
		return 0
	}

	if current == '\r' {
		nextRune, nextSize := l.peekNextRune()
		if nextRune == '\n' {
			l.position += size + nextSize
		} else {
			l.position += size
		}
		l.line++
		l.column = 1
		return '\n'
	}

	if current == '\n' {
		l.position += size
		l.line++
		l.column = 1
		return '\n'
	}

	l.position += size
	l.column++
	if current <= utf8.RuneSelf {
		return byte(current)
	}
	return 0
}

func (l *Lexer) match(expected byte) bool {
	if l.isAtEnd() || l.peek() != expected {
		return false
	}
	l.advance()
	return true
}

func (l *Lexer) peek() byte {
	if l.isAtEnd() {
		return 0
	}
	current, _ := utf8.DecodeRuneInString(l.input[l.position:])
	if current == utf8.RuneError {
		return 0
	}
	if current <= utf8.RuneSelf {
		return byte(current)
	}
	return 0
}

func (l *Lexer) peekNext() byte {
	if l.isAtEnd() {
		return 0
	}
	nextRune, _ := l.peekNextRune()
	if nextRune <= utf8.RuneSelf {
		return byte(nextRune)
	}
	return 0
}

func (l *Lexer) peekNextRune() (rune, int) {
	_, size := utf8.DecodeRuneInString(l.input[l.position:])
	if l.position+size >= len(l.input) {
		return 0, 0
	}
	return utf8.DecodeRuneInString(l.input[l.position+size:])
}

func keywordToken(lexeme string) (TokenType, bool) {
	switch lexeme {
	case "from":
		return TokenFrom, true
	case "import":
		return TokenImport, true
	case "type":
		return TokenTypeKeyword, true
	case "schema":
		return TokenSchema, true
	case "doc":
		return TokenDoc, true
	case "enum":
		return TokenEnum, true
	case "array":
		return TokenArray, true
	case "union":
		return TokenUnion, true
	case "variant":
		return TokenVariant, true
	case "string":
		return TokenStringType, true
	case "int":
		return TokenIntType, true
	case "float":
		return TokenFloatType, true
	case "boolean":
		return TokenBooleanType, true
	case "output":
		return TokenOutput, true
	case "schema_file":
		return TokenSchemaFile, true
	case "data":
		return TokenData, true
	case "injectable":
		return TokenInjectable, true
	case "true", "false":
		return TokenBoolean, true
	default:
		return TokenIdentifier, false
	}
}

func isLetter(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

func isDigit(value byte) bool {
	return value >= '0' && value <= '9'
}
