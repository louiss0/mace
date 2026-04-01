package lsp

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/samber/lo"
	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/parser/ast"
)

var (
	importPathPattern           = regexp.MustCompile(`^\s*from\s+"([^"]+)"\s*([A-Za-z_]*)$`)
	importOpenPathPattern       = regexp.MustCompile(`^\s*from\s+"([^"]*)$`)
	importIdentifiersPattern    = regexp.MustCompile(`^\s*from\s+"([^"]+)"\s+import\s*([A-Za-z_]*)$`)
	directiveOutputValuePattern = regexp.MustCompile(`^\s*output\s*=\s*([A-Za-z_]*)$`)
	directiveSchemaPattern      = regexp.MustCompile(`^\s*schema\s*=\s*([A-Za-z_]*)$`)
	directiveSchemaFilePattern  = regexp.MustCompile(`^\s*schema_file\s*=\s*"([^"]*)$`)
)

var globalKeywordCompletions = []completionDefinition{
	{Label: "from", Kind: protocol.CompletionItemKindKeyword, Detail: "import declaration"},
}

var scriptKeywordCompletions = []completionDefinition{
	{Label: "array", Kind: protocol.CompletionItemKindKeyword, Detail: "type constructor"},
	{Label: "boolean", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "float", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "injectable", Kind: protocol.CompletionItemKindKeyword, Detail: "variable modifier"},
	{Label: "int", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "schema", Kind: protocol.CompletionItemKindKeyword, Detail: "schema declaration"},
	{Label: "string", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "type", Kind: protocol.CompletionItemKindKeyword, Detail: "type declaration"},
}

func completionItems(document document, uri protocol.DocumentUri, position protocol.Position) []protocol.CompletionItem {
	linePrefix := currentLinePrefix(document.text, position)
	scope := completionScopeAt(document.text, position)
	declarations := completionDeclarations(document, uri, position, linePrefix, scope)

	if scope == completionScopeFile {
		if items, handled := importCompletionItems(linePrefix, uri); handled {
			return items
		}
	}

	if scope == completionScopeOutput {
		if items, handled := directiveCompletionItems(document, uri, linePrefix); handled {
			return items
		}
	}

	prefix := identifierPrefixAt(document.text, position)
	items := []protocol.CompletionItem{}
	switch scope {
	case completionScopeFile:
		items = itemsFromDefinitions(globalKeywordCompletions, prefix)
	case completionScopeScript:
		items = itemsFromDefinitions(scriptKeywordCompletions, prefix)
		items = append(items, itemsFromDeclarations(declarations, prefix)...)
	case completionScopeOutput:
		items = itemsFromDeclarations(declarations, prefix)
	}

	return sortCompletionItems(items)
}

func completionDeclarations(
	document document,
	uri protocol.DocumentUri,
	position protocol.Position,
	linePrefix string,
	scope completionScope,
) []declarationDefinition {
	if len(document.analysis.declarations) > 0 {
		return document.analysis.declarations
	}

	switch scope {
	case completionScopeScript:
		file, ok := partialScriptFile(document.text, position)
		if !ok {
			return nil
		}

		return collectDeclarations(file, nil, filepath.Dir(documentPath(uri)))
	case completionScopeOutput:
		file := completionFile(document, linePrefix)
		if file == nil {
			return nil
		}

		return collectDeclarations(*file, nil, filepath.Dir(documentPath(uri)))
	default:
		return nil
	}
}

func partialScriptFile(text string, position protocol.Position) (ast.File, bool) {
	index := position.IndexIn(text)
	if index < 0 {
		return ast.File{}, false
	}

	lineStart := strings.LastIndex(text[:index], "\n")
	if lineStart < 0 {
		lineStart = 0
	} else {
		lineStart++
	}

	prefix := text[:lineStart]
	if !strings.Contains(prefix, "|") {
		return ast.File{}, false
	}

	file, err := parseFile(prefix + "\n|===|\n[output = data] {}")
	if err != nil {
		return ast.File{}, false
	}

	return file, true
}

func completionScopeAt(text string, position protocol.Position) completionScope {
	index := position.IndexIn(text)
	if index < 0 {
		return completionScopeFile
	}

	lines := strings.Split(text[:index], "\n")
	inScript := false
	outputStarted := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if isScriptDelimiterLine(trimmed) {
			inScript = !inScript
			continue
		}

		if inScript {
			continue
		}

		if strings.Contains(line, "[") || strings.Contains(line, "{") {
			outputStarted = true
		}
	}

	if inScript {
		return completionScopeScript
	}

	if outputStarted {
		return completionScopeOutput
	}

	return completionScopeFile
}

func importCompletionItems(linePrefix string, uri protocol.DocumentUri) ([]protocol.CompletionItem, bool) {
	if matches := importOpenPathPattern.FindStringSubmatch(linePrefix); len(matches) == 2 {
		return relativePathItems(uri, matches[1], nil), true
	}

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

func directiveCompletionItems(document document, uri protocol.DocumentUri, linePrefix string) ([]protocol.CompletionItem, bool) {
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

	if matches := directiveSchemaPattern.FindStringSubmatch(lastPart); len(matches) == 2 {
		return schemaReferenceItems(document, uri, linePrefix, matches[1]), true
	}

	if matches := directiveSchemaFilePattern.FindStringSubmatch(lastPart); len(matches) == 2 {
		return schemaFileItems(document, uri, linePrefix, matches[1]), true
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

	if state.outputMode == "" {
		return []completionDefinition{}
	}

	if state.seenSchema || state.seenSchemaFile {
		return []completionDefinition{}
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

func relativePathItems(uri protocol.DocumentUri, pathPrefix string, excludedPaths []string) []protocol.CompletionItem {
	documentPath, ok := documentPathFromURI(uri)
	if !ok {
		return []protocol.CompletionItem{}
	}

	items, err := directoryEntries(filepath.Dir(documentPath), pathPrefix, excludedPaths)
	if err != nil {
		return []protocol.CompletionItem{}
	}

	return sortCompletionItems(items)
}

func schemaReferenceItems(document document, uri protocol.DocumentUri, linePrefix string, prefix string) []protocol.CompletionItem {
	items := lo.Map(availableSchemaNames(document, uri, linePrefix), func(name string, _ int) protocol.CompletionItem {
		return protocol.CompletionItem{
			Label: name,
			Kind:  Ptr(protocol.CompletionItemKindStruct),
		}
	})
	items = lo.Filter(items, func(item protocol.CompletionItem, _ int) bool {
		return strings.HasPrefix(item.Label, prefix)
	})
	return sortCompletionItems(items)
}

func schemaFileItems(document document, uri protocol.DocumentUri, linePrefix string, pathPrefix string) []protocol.CompletionItem {
	return relativePathItems(uri, pathPrefix, importedPaths(document, linePrefix))
}

func availableSchemaNames(document document, uri protocol.DocumentUri, linePrefix string) []string {
	localSchemas := []string{}
	file := completionFile(document, linePrefix)
	if file != nil && file.Script != nil {
		localSchemas = lo.FilterMap(file.Script.Items, func(item ast.Declaration, _ int) (string, bool) {
			declaration, ok := item.(ast.SchemaDeclaration)
			if !ok {
				return "", false
			}

			return declaration.Name, true
		})
	}

	importedSchemas := lo.FlatMap(currentImports(document, linePrefix), func(importDecl ast.ImportDeclaration, _ int) []string {
		exportedSchemas, ok := importableSchemaIdentifiers(uri, importDecl)
		if !ok {
			return nil
		}

		return exportedSchemas
	})

	names := append(localSchemas, importedSchemas...)
	return lo.Uniq(sortStrings(names))
}

func importableSchemaIdentifiers(uri protocol.DocumentUri, importDecl ast.ImportDeclaration) ([]string, bool) {
	pathValue, ok := stringLiteralValue(importDecl.Path)
	if !ok {
		return nil, false
	}

	documentPath, ok := documentPathFromURI(uri)
	if !ok {
		return nil, false
	}

	resolvedPath := filepath.Clean(filepath.Join(filepath.Dir(documentPath), pathValue))
	contents, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, false
	}

	file, err := parseFile(string(contents))
	if err != nil {
		return nil, false
	}

	exportedSchemaNames := lo.Map(file.Output.SchemaFields, func(field ast.OutputSchemaField, _ int) string {
		return field.Name
	})

	return lo.Filter(importDecl.Identifiers, func(name string, _ int) bool {
		return lo.Contains(exportedSchemaNames, name)
	}), true
}

func importedPaths(document document, linePrefix string) []string {
	return lo.FilterMap(currentImports(document, linePrefix), func(importDecl ast.ImportDeclaration, _ int) (string, bool) {
		pathValue, ok := stringLiteralValue(importDecl.Path)
		if !ok {
			return "", false
		}

		return pathValue, true
	})
}

func currentImports(document document, linePrefix string) []ast.ImportDeclaration {
	file := completionFile(document, linePrefix)
	if file == nil {
		return nil
	}

	return file.Imports
}

func completionFile(document document, linePrefix string) *ast.File {
	if document.analysis.file != nil {
		return document.analysis.file
	}

	openIndex := strings.LastIndex(document.text, "[")
	if openIndex < 0 {
		return nil
	}

	closeIndex := strings.LastIndex(document.text, "]")
	if closeIndex > openIndex {
		return nil
	}

	prefix := document.text[:openIndex]
	outputMode := "data"
	if strings.Contains(linePrefix, "output = schema") {
		outputMode = "schema"
	}

	file, err := parseFile(prefix + "[output = " + outputMode + "] {}")
	if err != nil {
		return nil
	}

	return &file
}

func stringLiteralValue(literal ast.StringLiteral) (string, bool) {
	value, err := strconv.Unquote(literal.Lexeme)
	if err != nil {
		return "", false
	}

	return value, true
}

func directoryEntries(baseDir string, pathPrefix string, excludedPaths []string) ([]protocol.CompletionItem, error) {
	resolvedDir, itemPrefix, labelPrefix := importDirectory(baseDir, pathPrefix)
	entries, err := os.ReadDir(resolvedDir)
	if err != nil {
		return nil, err
	}

	items := lo.FilterMap(entries, func(entry os.DirEntry, _ int) (protocol.CompletionItem, bool) {
		name := entry.Name()
		if !strings.HasPrefix(name, itemPrefix) {
			return protocol.CompletionItem{}, false
		}
		if !entry.IsDir() && filepath.Ext(name) != ".mace" {
			return protocol.CompletionItem{}, false
		}

		label := joinImportPath(labelPrefix, name, entry.IsDir())
		if lo.Contains(excludedPaths, label) {
			return protocol.CompletionItem{}, false
		}

		kind := protocol.CompletionItemKindFile
		if entry.IsDir() {
			kind = protocol.CompletionItemKindFolder
		}

		return protocol.CompletionItem{
			Label: label,
			Kind:  Ptr(kind),
		}, true
	})

	return items, nil
}

func importDirectory(baseDir string, pathPrefix string) (string, string, string) {
	cleanPrefix := normalizedRelativePathPrefix(pathPrefix)
	parent, name := path.Split(cleanPrefix)
	if strings.HasSuffix(cleanPrefix, "/") {
		parent = cleanPrefix
		name = ""
	}

	return filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(parent))), name, parent
}

func normalizedRelativePathPrefix(pathPrefix string) string {
	if strings.HasPrefix(pathPrefix, "../") || pathPrefix == ".." {
		return filepath.ToSlash(pathPrefix)
	}
	if strings.HasPrefix(pathPrefix, "./") {
		return filepath.ToSlash(pathPrefix)
	}
	if pathPrefix == "" {
		return "./"
	}

	return "./" + filepath.ToSlash(pathPrefix)
}

func joinImportPath(parent string, name string, isDir bool) string {
	label := parent + name
	if isDir {
		return label + "/"
	}

	return label
}

func sortStrings(values []string) []string {
	slices.Sort(values)
	return values
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

type completionScope int

const (
	completionScopeFile completionScope = iota
	completionScopeScript
	completionScopeOutput
)
