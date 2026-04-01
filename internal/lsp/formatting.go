package lsp

import (
	"strings"

	"github.com/samber/lo"
)

func formatDocumentText(text string) string {
	newline := "\n"
	if strings.Contains(text, "\r\n") {
		newline = "\r\n"
	}

	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	formatted := lo.Map(lines, func(line string, _ int) string {
		return line
	})

	start := -1
	for index, line := range lines {
		if !isScriptDelimiterLine(line) {
			continue
		}

		if start == -1 {
			start = index
			continue
		}

		delimiter := widestScriptDelimiter(lines[start+1 : index])
		formatted[start] = delimiter
		formatted[index] = delimiter
		start = -1
	}

	return strings.Join(formatted, newline)
}

func isScriptDelimiterLine(line string) bool {
	if len(line) < 5 || line[0] != '|' || line[len(line)-1] != '|' {
		return false
	}

	for _, value := range line[1 : len(line)-1] {
		if value != '=' {
			return false
		}
	}

	return true
}

func widestScriptDelimiter(lines []string) string {
	width := lo.Reduce(lines, func(currentWidth int, line string, _ int) int {
		if len(line) > currentWidth {
			return len(line)
		}

		return currentWidth
	}, 3)

	return "|" + strings.Repeat("=", width) + "|"
}
