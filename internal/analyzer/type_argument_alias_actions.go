package analyzer

import (
	"regexp"
	"strconv"
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/lexer"
)

var scriptFencePattern = regexp.MustCompile(`(?m)^\|=+\|[ \t\r]*$`)

func typeArgumentAliasCodeActions(text string, tokens []lexer.Token, documentPath string) []analysisCodeActionCandidate {
	aliasName := nextExtractedTypeAliasName(tokens)
	actions := []analysisCodeActionCandidate{}
	for _, argument := range extractableTypeArguments(tokens) {
		start := tokenStartIndex(text, tokens[argument.start])
		endToken := tokens[argument.end]
		end := tokenStartIndex(text, endToken) + endToken.SourceLength()
		if argument.closingOffset >= 0 {
			end = tokenStartIndex(text, endToken) + argument.closingOffset
		}
		if start < 0 || end <= start {
			continue
		}

		updated := text[:start] + aliasName + text[end:]
		updated, ok := insertTypeAlias(updated, aliasName, text[start:end])
		if !ok {
			continue
		}
		rangeValue := protocol.Range{Start: positionFromIndex(text, start), End: positionFromIndex(text, end)}
		actions = append(actions, analysisCodeActionCandidate{
			Range: rangeValue,
			Action: protocol.CodeAction{
				Title: "Extract type argument into alias ‘" + aliasName + "’",
				Kind:  Ptr(protocol.CodeActionKindRefactorExtract),
				Edit: &protocol.WorkspaceEdit{Changes: map[protocol.DocumentUri][]protocol.TextEdit{
					pathURI(documentPath): {{Range: fullDocumentRange(text), NewText: updated}},
				}},
			},
		})
	}
	return actions
}

type typeArgumentRange struct {
	start         int
	end           int
	closingOffset int
}

func extractableTypeArguments(tokens []lexer.Token) []typeArgumentRange {
	arguments := []typeArgumentRange{}
	for tokenIndex, token := range tokens {
		if token.Type == lexer.TokenArray || token.Type == lexer.TokenRecord {
			if tokenIndex+2 >= len(tokens) || tokens[tokenIndex+1].Type != lexer.TokenLess {
				continue
			}
			if closingIndex, closingOffset, ok := matchingDelimiter(tokens, tokenIndex+1, lexer.TokenLess, lexer.TokenGreater); ok && closingIndex > tokenIndex+1 {
				arguments = append(arguments, typeArgumentRange{start: tokenIndex + 2, end: closingIndex, closingOffset: closingOffset})
			}
		}
		if token.Type == lexer.TokenVariant || token.Type == lexer.TokenUnion {
			if tokenIndex+2 >= len(tokens) || tokens[tokenIndex+1].Type != lexer.TokenLBracket {
				continue
			}
			if closingIndex, _, ok := matchingDelimiter(tokens, tokenIndex+1, lexer.TokenLBracket, lexer.TokenRBracket); ok {
				arguments = append(arguments, delimitedTypeArguments(tokens, tokenIndex+2, closingIndex)...)
			}
		}
	}
	return arguments
}

func matchingDelimiter(tokens []lexer.Token, openingIndex int, opening lexer.TokenType, closing lexer.TokenType) (int, int, bool) {
	depth := 0
	for index := openingIndex; index < len(tokens); index++ {
		token := tokens[index]
		if token.Type == opening {
			depth++
			continue
		}
		closingCount := 0
		if token.Type == closing {
			closingCount = 1
		} else if closing == lexer.TokenGreater && token.Type == lexer.TokenShiftRight {
			closingCount = 2
		} else if closing == lexer.TokenGreater && token.Type == lexer.TokenShiftRightUnsigned {
			closingCount = 3
		}
		for offset := 0; offset < closingCount; offset++ {
			depth--
			if depth == 0 {
				return index, offset, true
			}
		}
	}
	return 0, 0, false
}

func delimitedTypeArguments(tokens []lexer.Token, start int, end int) []typeArgumentRange {
	arguments := []typeArgumentRange{}
	argumentStart := start
	depth := 0
	for index := start; index < end; index++ {
		switch tokens[index].Type {
		case lexer.TokenLBracket, lexer.TokenLBrace, lexer.TokenLParen, lexer.TokenLess:
			depth++
		case lexer.TokenRBracket, lexer.TokenRBrace, lexer.TokenRParen, lexer.TokenGreater:
			depth--
		case lexer.TokenComma:
			if depth == 0 && argumentStart < index {
				arguments = append(arguments, typeArgumentRange{start: argumentStart, end: index - 1, closingOffset: -1})
				argumentStart = index + 1
			}
		}
	}
	if argumentStart < end {
		arguments = append(arguments, typeArgumentRange{start: argumentStart, end: end - 1, closingOffset: -1})
	}
	return arguments
}

func nextExtractedTypeAliasName(tokens []lexer.Token) string {
	declaredNames := map[string]bool{}
	for index := 0; index < len(tokens)-1; index++ {
		if (tokens[index].Type == lexer.TokenAliasKeyword || tokens[index].Type == lexer.TokenSchema) && tokens[index+1].Type == lexer.TokenIdentifier {
			declaredNames[tokens[index+1].Lexeme] = true
		}
	}
	for suffix := 1; ; suffix++ {
		name := "ExtractedType"
		if suffix > 1 {
			name += strconv.Itoa(suffix)
		}
		if !declaredNames[name] {
			return name
		}
	}
}

func insertTypeAlias(text string, aliasName string, typeText string) (string, bool) {
	declaration := "alias " + aliasName + ": " + typeText + ";\n"
	fences := scriptFencePattern.FindAllStringIndex(text, -1)
	if len(fences) >= 2 {
		closingFenceStart := fences[1][0]
		return text[:closingFenceStart] + declaration + text[closingFenceStart:], true
	}
	if outputStart := strings.Index(text, "[output"); outputStart >= 0 {
		return "|===|\n" + declaration + "|===|\n" + text[:outputStart] + text[outputStart:], true
	}
	return "", false
}
