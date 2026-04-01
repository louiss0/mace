package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/parser/ast"
)

var (
	importPathPattern           = regexp.MustCompile(`^\s*from\s+"([^"]+)"\s*([A-Za-z_]*)$`)
	importIdentifiersPattern    = regexp.MustCompile(`^\s*from\s+"([^"]+)"\s+import\s*([A-Za-z_]*)$`)
	directiveOutputValuePattern = regexp.MustCompile(`^\s*output\s*=\s*([A-Za-z_]*)$`)
)

var globalKeywordCompletions = []completionDefinition{
	{Label: "array", Kind: protocol.CompletionItemKindKeyword, Detail: "type constructor"},
	{Label: "boolean", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "float", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "from", Kind: protocol.CompletionItemKindKeyword, Detail: "import declaration"},
	{Label: "injectable", Kind: protocol.CompletionItemKindKeyword, Detail: "variable modifier"},
	{Label: "int", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "schema", Kind: protocol.CompletionItemKindKeyword, Detail: "schema declaration"},
	{Label: "string", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "type", Kind: protocol.CompletionItemKindKeyword, Detail: "type declaration"},
}

func completionItems(document document, uri protocol.DocumentUri, position protocol.Position) []protocol.CompletionItem {
	linePrefix := currentLinePrefix(document.text, position)

	if items, handled := importCompletionItems(linePrefix, uri); handled {
		return items
	}

	if items, handled := directiveCompletionItems(linePrefix); handled {
		return items
	}

	prefix := identifierPrefixAt(document.text, position)
	items := itemsFromDefinitions(globalKeywordCompletions, prefix)
	items = append(items, itemsFromDeclarations(document.analysis.declarations, prefix)...)
	return sortCompletionItems(items)
}

func importCompletionItems(linePrefix string, uri protocol.DocumentUri) ([]protocol.CompletionItem, bool) {
	if matches := importIdentifiersPattern.FindStringSubmatch(linePrefix); len(matches) == 3 {
		path := matches[1]
		prefix := matches[2]
		names, ok := importableIdentifiers(uri, path)
		if !ok {
			return []protocol.CompletionItem{}, true
		}

		items := lo.Map(names, func(name string, _ int) protocol.CompletionItem {
			return protocol.CompletionItem{
				Label: name,
				Kind:  Ptr(protocol.CompletionItemKindVariable),
			}
		})
		items = lo.Filter(items, func(item protocol.CompletionItem, _ int) bool {
			return strings.HasPrefix(item.Label, prefix)
		})
		return sortCompletionItems(items), true
	}

	if matches := importPathPattern.FindStringSubmatch(linePrefix); len(matches) == 3 {
		path := matches[1]
		prefix := matches[2]
		if prefix != "" && !strings.HasPrefix("import", prefix) {
			return []protocol.CompletionItem{}, true
		}
		if _, ok := importableIdentifiers(uri, path); !ok {
			return []protocol.CompletionItem{}, true
		}

		return itemsFromDefinitions([]completionDefinition{
			{Label: "import", Kind: protocol.CompletionItemKindKeyword, Detail: "import declaration"},
		}, prefix), true
	}

	return nil, false
}

func directiveCompletionItems(linePrefix string) ([]protocol.CompletionItem, bool) {
	content, ok := directivePrefix(linePrefix)
	if !ok {
		return nil, false
	}

	parts := lo.FilterMap(strings.Split(content, ","), func(part string, _ int) (string, bool) {
		trimmed := strings.TrimSpace(part)
		return trimmed, trimmed != ""
	})

	if len(parts) == 0 {
		prefix := trailingIdentifierPrefix(content)
		return itemsFromDefinitions([]completionDefinition{
			{Label: "output", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
		}, prefix), true
	}

	lastPart := lo.LastOrEmpty(parts)
	if len(parts) == 1 {
		if !strings.Contains(lastPart, "=") {
			prefix := trailingIdentifierPrefix(lastPart)
			return itemsFromDefinitions([]completionDefinition{
				{Label: "output", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
			}, prefix), true
		}

		if matches := directiveOutputValuePattern.FindStringSubmatch(lastPart); len(matches) == 2 {
			prefix := matches[1]
			return itemsFromDefinitions([]completionDefinition{
				{Label: "data", Kind: protocol.CompletionItemKindKeyword, Detail: "output mode"},
				{Label: "schema", Kind: protocol.CompletionItemKindKeyword, Detail: "output mode"},
			}, prefix), true
		}
	}

	if strings.HasSuffix(content, ",") {
		options := nextDirectiveDefinitions(parts)
		return itemsFromDefinitions(options, ""), true
	}

	prefix := trailingIdentifierPrefix(lastPart)
	options := nextDirectiveDefinitions(parts[:len(parts)-1])
	return itemsFromDefinitions(options, prefix), true
}

func nextDirectiveDefinitions(parts []string) []completionDefinition {
	state := lo.Reduce(parts, func(agg directiveState, part string, _ int) directiveState {
		switch {
		case strings.HasPrefix(part, "output"):
			segments := strings.SplitN(part, "=", 2)
			if len(segments) == 2 {
				agg.outputMode = strings.TrimSpace(segments[1])
			}
		case strings.HasPrefix(part, "schema_file"):
			agg.seenSchemaFile = true
		case strings.HasPrefix(part, "schema"):
			agg.seenSchema = true
		}

		return agg
	}, directiveState{})

	if state.outputMode != "data" {
		return []completionDefinition{}
	}

	if state.seenSchema && state.seenSchemaFile {
		return []completionDefinition{}
	}

	if state.seenSchemaFile {
		return []completionDefinition{
			{Label: "schema", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
		}
	}

	if state.seenSchema {
		return []completionDefinition{
			{Label: "schema_file", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
		}
	}

	return []completionDefinition{
		{Label: "schema", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
		{Label: "schema_file", Kind: protocol.CompletionItemKindKeyword, Detail: "output directive"},
	}
}

func directivePrefix(linePrefix string) (string, bool) {
	openIndex := strings.LastIndex(linePrefix, "[")
	if openIndex < 0 {
		return "", false
	}

	closeIndex := strings.LastIndex(linePrefix, "]")
	if closeIndex > openIndex {
		return "", false
	}

	return linePrefix[openIndex+1:], true
}

func trailingIdentifierPrefix(value string) string {
	end := len(value)
	start := end
	for start > 0 && isIdentifierCharacter(value[start-1]) {
		start--
	}
	return value[start:end]
}

func currentLinePrefix(text string, position protocol.Position) string {
	index := position.IndexIn(text)
	if index < 0 {
		return ""
	}

	return lo.LastOrEmpty(strings.Split(text[:index], "\n"))
}

func importableIdentifiers(uri protocol.DocumentUri, importPath string) ([]string, bool) {
	documentPath, ok := documentPathFromURI(uri)
	if !ok {
		return nil, false
	}

	resolvedPath := filepath.Clean(filepath.Join(filepath.Dir(documentPath), importPath))
	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, false
	}

	file, err := parseFile(string(contents))
	if err != nil {
		return nil, false
	}

	names := lo.Map(file.Output.DataFields, func(field ast.OutputField, _ int) string {
		return field.Name
	})
	names = append(names, lo.Map(file.Output.SchemaFields, func(field ast.OutputSchemaField, _ int) string {
		return field.Name
	})...)
	return names, true
}

func documentPathFromURI(uri protocol.DocumentUri) (string, bool) {
	parsed, err := url.Parse(string(uri))
	if err != nil || parsed.Scheme != "file" {
		return "", false
	}

	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", false
	}

	if len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}

	return filepath.FromSlash(path), true
}

func itemsFromDefinitions(definitions []completionDefinition, prefix string) []protocol.CompletionItem {
	items := lo.FilterMap(definitions, func(definition completionDefinition, _ int) (protocol.CompletionItem, bool) {
		if !strings.HasPrefix(definition.Label, prefix) {
			return protocol.CompletionItem{}, false
		}

		item := protocol.CompletionItem{
			Label: definition.Label,
			Kind:  Ptr(definition.Kind),
		}
		if definition.Detail != "" {
			item.Detail = Ptr(definition.Detail)
		}
		return item, true
	})
	return sortCompletionItems(items)
}

func itemsFromDeclarations(declarations []declarationDefinition, prefix string) []protocol.CompletionItem {
	items := lo.FilterMap(declarations, func(declaration declarationDefinition, _ int) (protocol.CompletionItem, bool) {
		if declaration.Name == "" || !strings.HasPrefix(declaration.Name, prefix) {
			return protocol.CompletionItem{}, false
		}

		item := protocol.CompletionItem{
			Label: declaration.Name,
			Kind:  Ptr(declaration.Kind),
		}
		if declaration.Detail != "" {
			item.Detail = Ptr(declaration.Detail)
		}
		return item, true
	})
	return sortCompletionItems(items)
}

func sortCompletionItems(items []protocol.CompletionItem) []protocol.CompletionItem {
	slices.SortFunc(items, func(left protocol.CompletionItem, right protocol.CompletionItem) int {
		return strings.Compare(left.Label, right.Label)
	})
	return items
}

type directiveState struct {
	outputMode     string
	seenSchemaFile bool
	seenSchema     bool
}
