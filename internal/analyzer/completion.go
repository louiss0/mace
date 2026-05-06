package analyzer

import (
	"fmt"
	"maps"
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

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
)

var (
	importPathPattern           = regexp.MustCompile(`^\s*from\s+"([^"]+)"\s*([A-Za-z_]*)$`)
	importOpenPathPattern       = regexp.MustCompile(`^\s*from\s+"([^"]*)$`)
	importIdentifiersPattern    = regexp.MustCompile(`^\s*from\s+"([^"]+)"\s+import\s*([A-Za-z_]*)$`)
	directiveOutputValuePattern = regexp.MustCompile(`^\s*output\s*=\s*([A-Za-z_]*)$`)
	directiveSchemaPattern      = regexp.MustCompile(`^\s*schema\s*=\s*([A-Za-z_]*)$`)
	directiveSchemaFilePattern  = regexp.MustCompile(`^\s*schema_file\s*=\s*"([^"]*)$`)
)

const completionPlaceholderIdentifier = "mace_cursor_placeholder"

var globalKeywordCompletions = []completionDefinition{
	{Label: "from", Kind: protocol.CompletionItemKindKeyword, Detail: "import declaration"},
}

var scriptKeywordCompletions = []completionDefinition{
	{Label: "array", Kind: protocol.CompletionItemKindKeyword, Detail: "type constructor"},
	{Label: "boolean", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "enum", Kind: protocol.CompletionItemKindKeyword, Detail: "enum declaration"},
	{Label: "float", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "gen_doc", Kind: protocol.CompletionItemKindKeyword, Detail: "type or variable documentation declaration"},
	{Label: "injectable", Kind: protocol.CompletionItemKindKeyword, Detail: "variable modifier"},
	{Label: "int", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "schema", Kind: protocol.CompletionItemKindKeyword, Detail: "schema declaration"},
	{Label: "schema_doc", Kind: protocol.CompletionItemKindKeyword, Detail: "schema or enum documentation declaration"},
	{Label: "string", Kind: protocol.CompletionItemKindKeyword, Detail: "primitive type"},
	{Label: "type", Kind: protocol.CompletionItemKindKeyword, Detail: "type declaration"},
	{Label: "union", Kind: protocol.CompletionItemKindKeyword, Detail: "schema composition"},
	{Label: "variant", Kind: protocol.CompletionItemKindKeyword, Detail: "type constructor"},
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

	if scope == completionScopeScript {
		if items, handled := arrayIndexCompletionItems(document, uri, position, linePrefix, scope); handled {
			return items
		}

		if items, handled := enumMemberCompletionItems(document, uri, position, linePrefix); handled {
			return items
		}

		if items, handled := initializerCompletionItems(document, uri, position); handled {
			return items
		}
	}

	if scope == completionScopeOutput {
		bareSelfItems := bareSelfCompletionItems(linePrefix, position)

		if items, handled := directiveCompletionItems(document, uri, linePrefix); handled {
			return items
		}

		if items, handled := outputInitializerCompletionItems(document, uri, position); handled {
			return mergeCompletionItems(items, bareSelfItems)
		}

		if items, handled := selfKeywordCompletionItems(linePrefix, position); handled {
			return items
		}

		if items, handled := selfCompletionItems(document, uri, position); handled {
			return items
		}

		if items, handled := arrayIndexCompletionItems(document, uri, position, linePrefix, scope); handled {
			return items
		}

		if items, handled := enumMemberCompletionItems(document, uri, position, linePrefix); handled {
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
		items = mergeCompletionItems(items, bareSelfCompletionItems(linePrefix, position))
	}

	return sortCompletionItems(items)
}

func initializerCompletionItems(document document, uri protocol.DocumentUri, position protocol.Position) ([]protocol.CompletionItem, bool) {
	placeholderPosition, ok := completionPlaceholderPosition(document.text, position, "=:")
	if !ok {
		return nil, false
	}

	file, ok := completionFileWithPlaceholder(document.text, placeholderPosition)
	if !ok {
		return nil, false
	}

	baseDir := filepath.Dir(documentPath(uri))
	if baseDir == "" {
		baseDir = "."
	}

	model := buildCompletionModel(*file, baseDir, map[string]completionModel{})
	expectedType, path, ok := placeholderCompletionType(*file, model)
	if !ok {
		return nil, false
	}

	return sortCompletionItems(completionItemsForType(expectedType, model, len(path) > 0)), true
}

func outputInitializerCompletionItems(document document, uri protocol.DocumentUri, position protocol.Position) ([]protocol.CompletionItem, bool) {
	placeholderPosition, ok := completionPlaceholderPosition(document.text, position, ":")
	if !ok {
		return nil, false
	}

	file, ok := completionFileWithPlaceholder(document.text, placeholderPosition)
	if !ok {
		return nil, false
	}

	baseDir := filepath.Dir(documentPath(uri))
	if baseDir == "" {
		baseDir = "."
	}

	model := buildCompletionModel(*file, baseDir, map[string]completionModel{})
	expectedType, path, ok := placeholderOutputCompletionType(*file, model)
	if !ok {
		return nil, false
	}

	return sortCompletionItems(completionItemsForType(expectedType, model, len(path) > 1)), true
}

func bareSelfCompletionItems(linePrefix string, position protocol.Position) []protocol.CompletionItem {
	trimmedPrefix := strings.TrimRight(linePrefix, " \t")
	if trimmedPrefix == "" {
		return nil
	}

	lastCharacter := trimmedPrefix[len(trimmedPrefix)-1]
	if !strings.ContainsRune(":([,{+-*/%&|^!?=<>", rune(lastCharacter)) {
		return nil
	}

	return selfReferenceCompletionItems("", position)
}

func selfKeywordCompletionItems(linePrefix string, position protocol.Position) ([]protocol.CompletionItem, bool) {
	trimmedPrefix := strings.TrimRight(linePrefix, " \t")
	if trimmedPrefix == "" {
		return nil, false
	}
	if strings.HasSuffix(trimmedPrefix, "$self.") {
		return nil, false
	}

	segmentEnd := len(trimmedPrefix)
	segmentStart := segmentEnd
	for segmentStart > 0 {
		character := trimmedPrefix[segmentStart-1]
		if character == '$' || isIdentifierCharacter(character) {
			segmentStart--
			continue
		}
		break
	}

	segment := trimmedPrefix[segmentStart:segmentEnd]
	if segment == "" || segment[0] != '$' {
		return nil, false
	}
	if !strings.HasPrefix("$self", segment) {
		return []protocol.CompletionItem{}, true
	}

	return selfReferenceCompletionItems(segment, position), true
}

func selfCompletionItems(document document, uri protocol.DocumentUri, position protocol.Position) ([]protocol.CompletionItem, bool) {
	linePrefix := currentLinePrefix(document.text, position)
	path, prefix, ok := selfCompletionContext(linePrefix)
	if !ok {
		return nil, false
	}

	value, ok := selfCompletionValue(document, uri, position, path)
	if !ok {
		return []protocol.CompletionItem{}, true
	}

	items := lo.Map(selfCompletionEntries(value), func(name string, _ int) protocol.CompletionItem {
		return protocol.CompletionItem{
			Label: name,
			Kind:  Ptr(protocol.CompletionItemKindField),
		}
	})
	items = lo.Filter(items, func(item protocol.CompletionItem, _ int) bool {
		return strings.HasPrefix(item.Label, prefix)
	})
	return sortCompletionItems(items), true
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

func selfCompletionContext(linePrefix string) ([]string, string, bool) {
	index := strings.LastIndex(linePrefix, "$self.")
	if index < 0 {
		return nil, "", false
	}

	suffix := linePrefix[index+len("$self."):]
	if strings.ContainsAny(suffix, "[]{}():;,+-*/%&|^!?=<>\"' \t") {
		return nil, "", false
	}

	segments := lo.Filter(strings.Split(suffix, "."), func(segment string, _ int) bool {
		return segment != ""
	})
	if strings.HasSuffix(suffix, ".") {
		return segments, "", true
	}
	if len(segments) == 0 {
		return nil, "", true
	}

	return segments[:len(segments)-1], segments[len(segments)-1], true
}

func arrayIndexCompletionItems(document document, uri protocol.DocumentUri, position protocol.Position, linePrefix string, scope completionScope) ([]protocol.CompletionItem, bool) {
	targetText, prefix, ok := arrayIndexCompletionContext(linePrefix)
	if !ok {
		return nil, false
	}

	arrayValue, ok := resolveArrayCompletionTarget(document, uri, position, targetText, scope)
	if !ok || arrayValue.Kind != processor.ValueArray {
		return []protocol.CompletionItem{}, true
	}

	items := make([]protocol.CompletionItem, 0, len(arrayValue.Array))
	for index := range arrayValue.Array {
		label := strconv.Itoa(index)
		if !strings.HasPrefix(label, prefix) {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:  label,
			Kind:   Ptr(protocol.CompletionItemKindValue),
			Detail: Ptr("array index"),
		})
	}

	return sortCompletionItems(items), true
}

func arrayIndexCompletionContext(linePrefix string) (string, string, bool) {
	trimmedPrefix := strings.TrimRight(linePrefix, " \t")
	index := strings.LastIndex(trimmedPrefix, "[")
	if index < 0 {
		return "", "", false
	}

	prefix := trimmedPrefix[index+1:]
	if prefix != "" && !isDigits(prefix) {
		return "", "", false
	}

	target := strings.TrimSpace(trimmedPrefix[:index])
	for start := len(target) - 1; start >= 0; start-- {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_.$[]()", rune(target[start])) {
			target = strings.TrimSpace(target[start+1:])
			break
		}
	}
	if target == "" {
		return "", "", false
	}

	return target, prefix, true
}

func resolveArrayCompletionTarget(document document, uri protocol.DocumentUri, position protocol.Position, targetText string, scope completionScope) (processor.Value, bool) {
	expression, err := parseExpression(targetText)
	if err != nil {
		return processor.Value{}, false
	}

	variables := partialScriptVariables(document.text, uri, position)
	self := processor.Value{Kind: processor.ValueRecord, Record: map[string]processor.Value{}}
	if scope == completionScopeOutput {
		variables = scriptVariablesForOutput(document.text, uri)
		result, ok := partialOutputResult(document, uri, position)
		if ok {
			self = processor.Value{Kind: processor.ValueRecord, Record: result.Output}
		}
	}

	value, ok := resolveCompletionValue(expression, variables, self)
	if ok {
		return value, true
	}

	return resolveLocalArrayCompletionTarget(document.text, position, expression)
}

func resolveLocalArrayCompletionTarget(text string, position protocol.Position, expression ast.Expression) (processor.Value, bool) {
	file, ok := partialScriptFile(text, position)
	if !ok || file.Script == nil {
		return processor.Value{}, false
	}

	declarations := map[string]ast.Expression{}
	for _, item := range file.Script.Items {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok || !declaration.HasValue {
			continue
		}
		declarations[declaration.Name] = declaration.Value
	}

	return resolveLocalCompletionValue(expression, declarations, map[string]struct{}{})
}

func resolveLocalCompletionValue(expression ast.Expression, declarations map[string]ast.Expression, seen map[string]struct{}) (processor.Value, bool) {
	switch typed := expression.(type) {
	case ast.Identifier:
		declaration, ok := declarations[typed.Name]
		if !ok {
			return processor.Value{}, false
		}
		if _, recursive := seen[typed.Name]; recursive {
			return processor.Value{}, false
		}
		nextSeen := maps.Clone(seen)
		nextSeen[typed.Name] = struct{}{}
		return resolveLocalCompletionValue(declaration, declarations, nextSeen)
	case ast.MemberAccess:
		target, ok := resolveLocalCompletionValue(typed.Target, declarations, seen)
		if !ok || target.Kind != processor.ValueRecord {
			return processor.Value{}, false
		}
		value, ok := target.Record[typed.Name]
		return value, ok
	case ast.ArrayAccess:
		target, ok := resolveLocalCompletionValue(typed.Target, declarations, seen)
		if !ok || target.Kind != processor.ValueArray {
			return processor.Value{}, false
		}
		index, err := strconv.Atoi(typed.Index.Lexeme)
		if err != nil || index < 0 || index >= len(target.Array) {
			return processor.Value{}, false
		}
		return target.Array[index], true
	case ast.ArrayLiteral:
		values := make([]processor.Value, 0, len(typed.Elements))
		for _, element := range typed.Elements {
			value, ok := resolveLocalCompletionValue(element, declarations, seen)
			if !ok {
				return processor.Value{}, false
			}
			values = append(values, value)
		}
		return processor.Value{Kind: processor.ValueArray, Array: values}, true
	case ast.RecordLiteral:
		fields := map[string]processor.Value{}
		for _, field := range typed.Fields {
			value, ok := resolveLocalCompletionValue(field.Value, declarations, seen)
			if !ok {
				return processor.Value{}, false
			}
			fields[field.Name] = value
		}
		return processor.Value{Kind: processor.ValueRecord, Record: fields}, true
	case ast.StringLiteral, ast.IntLiteral, ast.FloatLiteral, ast.BooleanLiteral:
		return resolveCompletionValue(expression, nil, processor.Value{})
	default:
		return processor.Value{}, false
	}
}

func partialScriptVariables(text string, uri protocol.DocumentUri, position protocol.Position) map[string]processor.Value {
	index := positionIndex(text, position)
	if index < 0 {
		return nil
	}

	lineStart := strings.LastIndex(text[:index], "\n")
	if lineStart < 0 {
		lineStart = 0
	} else {
		lineStart++
	}

	prefix := text[:lineStart]
	if !strings.Contains(prefix, "|") {
		return nil
	}

	partialText := prefix + "\n|===|\n[output = data] {}"
	return processVariablesInDocument(partialText, uri)
}

func scriptVariablesForOutput(text string, uri protocol.DocumentUri) map[string]processor.Value {
	tokens, err := lex(text)
	if err != nil {
		return nil
	}

	inScript := false
	scriptStart := -1
	scriptEnd := -1
	for _, token := range tokens {
		if token.Type != lexer.TokenScriptDelimiter {
			continue
		}
		if !inScript {
			inScript = true
			scriptStart = tokenStartIndex(text, token)
			continue
		}
		scriptEnd = tokenStartIndex(text, token) + len(token.Lexeme)
		break
	}
	if scriptStart < 0 || scriptEnd <= scriptStart {
		return nil
	}

	partialText := text[:scriptEnd] + "\n[output = data] {}"
	return processVariablesInDocument(partialText, uri)
}

func processVariablesInDocument(text string, uri protocol.DocumentUri) map[string]processor.Value {
	baseDir := filepath.Dir(documentPath(uri))
	if baseDir == "" {
		baseDir = "."
	}

	variables, err := processor.New().ProcessVariablesInDir(text, baseDir)
	if err != nil {
		return nil
	}
	return variables
}

func resolveCompletionValue(expression ast.Expression, variables map[string]processor.Value, self processor.Value) (processor.Value, bool) {
	switch typed := expression.(type) {
	case ast.Identifier:
		value, ok := variables[typed.Name]
		return value, ok
	case ast.SelfReference:
		return outputValueAtSegments(self, typed.Path)
	case ast.MemberAccess:
		target, ok := resolveCompletionValue(typed.Target, variables, self)
		if !ok || target.Kind != processor.ValueRecord {
			return processor.Value{}, false
		}
		value, ok := target.Record[typed.Name]
		return value, ok
	case ast.ArrayAccess:
		target, ok := resolveCompletionValue(typed.Target, variables, self)
		if !ok || target.Kind != processor.ValueArray {
			return processor.Value{}, false
		}
		index, err := strconv.Atoi(typed.Index.Lexeme)
		if err != nil || index < 0 || index >= len(target.Array) {
			return processor.Value{}, false
		}
		return target.Array[index], true
	case ast.ArrayLiteral:
		values := make([]processor.Value, 0, len(typed.Elements))
		for _, element := range typed.Elements {
			value, ok := resolveCompletionValue(element, variables, self)
			if !ok {
				return processor.Value{}, false
			}
			values = append(values, value)
		}
		return processor.Value{Kind: processor.ValueArray, Array: values}, true
	case ast.RecordLiteral:
		fields := map[string]processor.Value{}
		for _, field := range typed.Fields {
			value, ok := resolveCompletionValue(field.Value, variables, self)
			if !ok {
				return processor.Value{}, false
			}
			fields[field.Name] = value
		}
		return processor.Value{Kind: processor.ValueRecord, Record: fields}, true
	case ast.StringLiteral:
		return processor.Value{Kind: processor.ValueString, String: strings.Trim(typed.Lexeme, "\"'")}, true
	case ast.IntLiteral:
		value, err := strconv.ParseInt(typed.Lexeme, 10, 64)
		if err != nil {
			return processor.Value{}, false
		}
		return processor.Value{Kind: processor.ValueInt, Int: value}, true
	case ast.FloatLiteral:
		value, err := strconv.ParseFloat(typed.Lexeme, 64)
		if err != nil {
			return processor.Value{}, false
		}
		return processor.Value{Kind: processor.ValueFloat, Float: value}, true
	case ast.BooleanLiteral:
		return processor.Value{Kind: processor.ValueBoolean, Boolean: typed.Value}, true
	default:
		return processor.Value{}, false
	}
}

func outputValueAtSegments(value processor.Value, path []string) (processor.Value, bool) {
	current := value
	for _, segment := range path {
		if current.Kind != processor.ValueRecord {
			return processor.Value{}, false
		}
		child, ok := current.Record[segment]
		if !ok {
			return processor.Value{}, false
		}
		current = child
	}
	return current, true
}

func isDigits(value string) bool {
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func enumMemberCompletionItems(document document, uri protocol.DocumentUri, position protocol.Position, linePrefix string) ([]protocol.CompletionItem, bool) {
	enumName, prefix, ok := enumMemberCompletionContext(linePrefix)
	if !ok {
		return nil, false
	}

	model, ok := completionModelAt(document, uri, position, linePrefix)
	if !ok {
		return []protocol.CompletionItem{}, true
	}

	resolved := resolveCompletionType(ast.NamedType{Name: enumName}, model, map[string]struct{}{})
	if resolved.kind != completionTypeEnum {
		return []protocol.CompletionItem{}, true
	}

	items := lo.FilterMap(resolved.enum.members, func(member completionEnumMember, _ int) (protocol.CompletionItem, bool) {
		if !strings.HasPrefix(member.Name, prefix) {
			return protocol.CompletionItem{}, false
		}

		detail := resolved.enum.access(member.Name)
		if member.Detail != "" {
			detail = member.Detail
		}

		return protocol.CompletionItem{
			Label:  member.Name,
			Kind:   Ptr(protocol.CompletionItemKindEnumMember),
			Detail: Ptr(detail),
		}, true
	})
	return sortCompletionItems(items), true
}

func enumMemberCompletionContext(linePrefix string) (string, string, bool) {
	trimmedPrefix := strings.TrimRight(linePrefix, " \t")
	if strings.HasSuffix(trimmedPrefix, "$self.") {
		return "", "", false
	}

	segmentEnd := len(trimmedPrefix)
	segmentStart := segmentEnd
	for segmentStart > 0 {
		character := trimmedPrefix[segmentStart-1]
		if isIdentifierCharacter(character) || character == '.' {
			segmentStart--
			continue
		}
		break
	}

	segment := trimmedPrefix[segmentStart:segmentEnd]
	parts := strings.Split(segment, ".")
	if len(parts) != 2 {
		return "", "", false
	}
	if parts[0] == "" || parts[1] == "" && !strings.HasSuffix(segment, ".") {
		return "", "", false
	}
	if strings.Contains(parts[0], "$") {
		return "", "", false
	}

	return parts[0], parts[1], true
}

func completionModelAt(document document, uri protocol.DocumentUri, position protocol.Position, linePrefix string) (completionModel, bool) {
	file := document.analysis.file
	if file == nil {
		if partialFile, ok := partialScriptFile(document.text, position); ok {
			file = &partialFile
		} else {
			file = completionFile(document, linePrefix)
		}
	}
	if file == nil {
		return completionModel{}, false
	}

	baseDir := filepath.Dir(documentPath(uri))
	if baseDir == "" {
		baseDir = "."
	}

	return buildCompletionModel(*file, baseDir, map[string]completionModel{}), true
}

func partialScriptFile(text string, position protocol.Position) (ast.File, bool) {
	index := positionIndex(text, position)
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
	index := positionIndex(text, position)
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
		symbols, ok := importableSymbols(uri, path)
		if !ok {
			return []protocol.CompletionItem{}, true
		}

		items := lo.Map(symbols, func(symbol importableSymbol, _ int) protocol.CompletionItem {
			return protocol.CompletionItem{
				Label: symbol.Name,
				Kind:  Ptr(symbol.Kind),
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

	state := parseDirectiveState(parts)

	if matches := directiveSchemaPattern.FindStringSubmatch(lastPart); len(matches) == 2 {
		if state.outputMode == "schema" {
			return []protocol.CompletionItem{}, true
		}
		return schemaReferenceItems(document, uri, linePrefix, matches[1]), true
	}

	if matches := directiveSchemaFilePattern.FindStringSubmatch(lastPart); len(matches) == 2 {
		if state.outputMode == "schema" {
			return []protocol.CompletionItem{}, true
		}
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
	state := parseDirectiveState(parts)

	if state.outputMode == "" || state.outputMode == "schema" {
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

func parseDirectiveState(parts []string) directiveState {
	return lo.Reduce(parts, func(agg directiveState, part string, _ int) directiveState {
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
}

func directivePrefix(linePrefix string) (string, bool) {
	trimmedStart := len(linePrefix) - len(strings.TrimLeft(linePrefix, " \t"))
	if trimmedStart >= len(linePrefix) || linePrefix[trimmedStart] != '[' {
		return "", false
	}

	openIndex := strings.LastIndex(linePrefix, "[")
	if openIndex != trimmedStart {
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
	index := positionIndex(text, position)
	if index < 0 {
		return ""
	}

	return lo.LastOrEmpty(strings.Split(text[:index], "\n"))
}

func currentLineSuffix(text string, position protocol.Position) string {
	index := positionIndex(text, position)
	if index < 0 {
		return ""
	}

	lineEnd := strings.IndexByte(text[index:], '\n')
	if lineEnd < 0 {
		return text[index:]
	}

	return text[index : index+lineEnd]
}

func completionPlaceholderPosition(text string, position protocol.Position, operators string) (protocol.Position, bool) {
	linePrefix := currentLinePrefix(text, position)
	trimmedPrefix := strings.TrimSpace(linePrefix)
	if trimmedPrefix != "" {
		lastCharacter := trimmedPrefix[len(trimmedPrefix)-1]
		if strings.ContainsRune(operators, rune(lastCharacter)) {
			return position, true
		}
	}

	lineSuffix := strings.TrimLeft(currentLineSuffix(text, position), " \t")
	if lineSuffix == "" {
		return protocol.Position{}, false
	}
	if !strings.ContainsRune(operators, rune(lineSuffix[0])) {
		return protocol.Position{}, false
	}

	index := positionIndex(text, position)
	if index < 0 {
		return protocol.Position{}, false
	}

	whitespaceWidth := len(currentLineSuffix(text, position)) - len(lineSuffix)
	return positionFromIndex(text, index+whitespaceWidth+1), true
}

type importableSymbol struct {
	Name string
	Kind protocol.CompletionItemKind
}

func importableSymbols(uri protocol.DocumentUri, importPath string) ([]importableSymbol, bool) {
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

	symbols := []importableSymbol{}
	for _, field := range file.Output.DataFields {
		symbols = append(symbols, importableSymbol{Name: field.Name, Kind: protocol.CompletionItemKindVariable})
	}
	for _, field := range file.Output.SchemaFields {
		kind := protocol.CompletionItemKindClass
		resolved := resolveCompletionType(field.Type, buildCompletionModel(file, filepath.Dir(resolvedPath), map[string]completionModel{}), map[string]struct{}{})
		switch resolved.kind {
		case completionTypeSchema:
			kind = protocol.CompletionItemKindStruct
		case completionTypeEnum:
			kind = protocol.CompletionItemKindEnum
		}
		symbols = append(symbols, importableSymbol{Name: field.Name, Kind: kind})
	}
	return symbols, true
}

func importableIdentifiers(uri protocol.DocumentUri, importPath string) ([]string, bool) {
	symbols, ok := importableSymbols(uri, importPath)
	if !ok {
		return nil, false
	}
	return lo.Map(symbols, func(symbol importableSymbol, _ int) string { return symbol.Name }), true
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

func selfCompletionValue(document document, uri protocol.DocumentUri, position protocol.Position, path []string) (processor.Value, bool) {
	result, ok := partialOutputResult(document, uri, position)
	if !ok {
		return processor.Value{}, false
	}

	current := processor.Value{Kind: processor.ValueRecord, Record: result.Output}
	for _, segment := range path {
		if current.Kind != processor.ValueRecord {
			return processor.Value{}, false
		}

		next, ok := current.Record[segment]
		if !ok {
			return processor.Value{}, false
		}
		current = next
	}

	return current, true
}

func selfCompletionEntries(value processor.Value) []string {
	if value.Kind != processor.ValueRecord {
		return nil
	}

	return sortStrings(lo.Keys(value.Record))
}

func partialOutputResult(document document, uri protocol.DocumentUri, position protocol.Position) (processor.Result, bool) {
	text := document.text
	tokens, err := lex(text)
	if err != nil {
		return processor.Result{}, false
	}

	outputOpenIndex, ok := outputBlockOpenIndex(text, tokens)
	if !ok {
		return processor.Result{}, false
	}

	fieldRanges, ok := outputFieldRanges(text, tokens, outputOpenIndex)
	if !ok {
		return processor.Result{}, false
	}

	cursorIndex := positionIndex(text, position)
	if cursorIndex < 0 {
		return processor.Result{}, false
	}

	currentFieldIndex := -1
	for index, fieldRange := range fieldRanges {
		if cursorIndex < fieldRange.Start || cursorIndex > fieldRange.End {
			continue
		}
		currentFieldIndex = index
		break
	}
	if currentFieldIndex < 0 {
		return processor.Result{}, false
	}

	body := strings.Join(lo.Map(fieldRanges[:currentFieldIndex], func(fieldRange outputFieldRange, _ int) string {
		return text[fieldRange.Start:fieldRange.End]
	}), "")
	partialText := text[:outputOpenIndex+1]
	if strings.TrimSpace(body) != "" {
		partialText += "\n" + body
	}
	partialText += "\n}"

	baseDir := filepath.Dir(documentPath(uri))
	if baseDir == "" {
		baseDir = "."
	}

	result, err := processor.New().ProcessInDir(partialText, baseDir)
	if err != nil {
		return processor.Result{}, false
	}

	return result, true
}

type outputFieldRange struct {
	Start int
	End   int
}

func outputBlockOpenIndex(text string, tokens []lexer.Token) (int, bool) {
	inScript := false
	directiveDepth := 0

	for _, token := range tokens {
		switch token.Type {
		case lexer.TokenScriptDelimiter:
			inScript = !inScript
		case lexer.TokenLBracket:
			if !inScript {
				directiveDepth++
			}
		case lexer.TokenRBracket:
			if !inScript && directiveDepth > 0 {
				directiveDepth--
			}
		case lexer.TokenLBrace:
			if !inScript && directiveDepth == 0 {
				return tokenStartIndex(text, token), true
			}
		}
	}

	return 0, false
}

func outputFieldRanges(text string, tokens []lexer.Token, outputOpenIndex int) ([]outputFieldRange, bool) {
	outputTokens := lo.Filter(tokens, func(token lexer.Token, _ int) bool {
		return tokenStartIndex(text, token) >= outputOpenIndex
	})

	braceDepth := 0
	fieldStarts := []int{}

	for index := 0; index < len(outputTokens); index++ {
		token := outputTokens[index]
		switch token.Type {
		case lexer.TokenLBrace:
			braceDepth++
		case lexer.TokenRBrace:
			braceDepth--
		case lexer.TokenIdentifier:
			if braceDepth != 1 || !isOutputFieldHeader(outputTokens, index) {
				continue
			}
			fieldStarts = append(fieldStarts, tokenStartIndex(text, token))
		}
	}

	if len(fieldStarts) == 0 {
		return nil, false
	}

	outputCloseIndex := -1
	for index := len(outputTokens) - 1; index >= 0; index-- {
		if outputTokens[index].Type != lexer.TokenRBrace {
			continue
		}
		outputCloseIndex = tokenStartIndex(text, outputTokens[index])
		break
	}
	if outputCloseIndex < 0 {
		return nil, false
	}

	return lo.Map(fieldStarts, func(start int, index int) outputFieldRange {
		end := outputCloseIndex
		if index+1 < len(fieldStarts) {
			end = fieldStarts[index+1]
		}
		return outputFieldRange{Start: start, End: end}
	}), true
}

func isOutputFieldHeader(tokens []lexer.Token, index int) bool {
	if index+1 >= len(tokens) {
		return false
	}
	if tokens[index+1].Type == lexer.TokenColon {
		return true
	}
	return index+2 < len(tokens) &&
		tokens[index+1].Type == lexer.TokenQuestion &&
		tokens[index+2].Type == lexer.TokenColon
}

func tokenStartIndex(text string, token lexer.Token) int {
	position := protocol.Position{
		Line:      protocol.UInteger(token.Line - 1),
		Character: protocol.UInteger(token.Column - 1),
	}
	return positionIndex(text, position)
}

func stringLiteralValue(literal ast.StringLiteral) (string, bool) {
	value, err := strconv.Unquote(literal.Lexeme)
	if err != nil {
		return "", false
	}

	return value, true
}

func completionFileWithPlaceholder(text string, position protocol.Position) (*ast.File, bool) {
	index := positionIndex(text, position)
	if index < 0 {
		return nil, false
	}

	linePrefix := currentLinePrefix(text, position)
	replacement := completionPlaceholderIdentifier
	trimmedPrefix := strings.TrimSpace(linePrefix)
	if strings.HasSuffix(trimmedPrefix, "=") || strings.HasSuffix(trimmedPrefix, ":") {
		replacement += ";"
	}

	textWithPlaceholder := text[:index] + replacement + text[index:]
	file, err := parseFile(textWithPlaceholder)
	if err == nil {
		return &file, true
	}

	if completionScopeAt(text, position) != completionScopeScript {
		return nil, false
	}

	file, ok := partialScriptFileWithPlaceholder(textWithPlaceholder, position)
	if !ok {
		return nil, false
	}

	return &file, true
}

func partialScriptFileWithPlaceholder(text string, position protocol.Position) (ast.File, bool) {
	index := positionIndex(text, position)
	if index < 0 {
		return ast.File{}, false
	}

	lineEnd := strings.Index(text[index:], "\n")
	if lineEnd < 0 {
		lineEnd = len(text)
	} else {
		lineEnd += index
	}

	prefix := text[:lineEnd]
	if !strings.Contains(prefix, "|") {
		return ast.File{}, false
	}

	file, err := parseFile(prefix + "\n|===|\n[output = data] {}")
	if err != nil {
		return ast.File{}, false
	}

	return file, true
}

func placeholderCompletionType(file ast.File, model completionModel) (ast.TypeReference, []string, bool) {
	for _, item := range fileScriptItems(file) {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok || !declaration.HasValue {
			continue
		}

		path, ok := placeholderPath(declaration.Value)
		if !ok {
			continue
		}

		expectedType, ok := completionTypeAtPath(declaration.Type, path, model)
		if !ok {
			return nil, nil, false
		}

		return expectedType, path, true
	}

	return nil, nil, false
}

func placeholderOutputCompletionType(file ast.File, model completionModel) (ast.TypeReference, []string, bool) {
	if file.Output.Mode != ast.OutputModeData {
		return nil, nil, false
	}

	schemaName, ok := outputSchemaDirective(file)
	if !ok {
		return nil, nil, false
	}

	rootType := ast.NamedType{Name: schemaName}
	for _, field := range file.Output.DataFields {
		path, ok := placeholderPath(field.Value)
		if !ok {
			continue
		}

		fullPath := append([]string{field.Name}, path...)
		expectedType, ok := completionTypeAtPath(rootType, fullPath, model)
		if !ok {
			return nil, nil, false
		}

		return expectedType, fullPath, true
	}

	return nil, nil, false
}

func placeholderPath(expression ast.Expression) ([]string, bool) {
	switch typed := expression.(type) {
	case ast.Identifier:
		if typed.Name == completionPlaceholderIdentifier {
			return []string{}, true
		}
	case ast.RecordLiteral:
		for _, field := range typed.Fields {
			path, ok := placeholderPath(field.Value)
			if !ok {
				continue
			}
			return append([]string{field.Name}, path...), true
		}
	case ast.ArrayLiteral:
		for _, element := range typed.Elements {
			path, ok := placeholderPath(element)
			if ok {
				return path, true
			}
		}
	case ast.PrefixExpression:
		return placeholderPath(typed.Right)
	case ast.InfixExpression:
		if path, ok := placeholderPath(typed.Left); ok {
			return path, true
		}
		return placeholderPath(typed.Right)
	case ast.ConditionalExpression:
		if path, ok := placeholderPath(typed.Condition); ok {
			return path, true
		}
		if path, ok := placeholderPath(typed.Then); ok {
			return path, true
		}
		return placeholderPath(typed.Else)
	}

	return nil, false
}

func completionTypeAtPath(typeReference ast.TypeReference, path []string, model completionModel) (ast.TypeReference, bool) {
	current := typeReference
	for _, segment := range path {
		resolved := resolveCompletionType(current, model, map[string]struct{}{})
		if resolved.kind != completionTypeSchema {
			return nil, false
		}

		field, ok := lo.Find(resolved.record.Fields, func(field ast.SchemaField) bool {
			return field.Name == segment
		})
		if !ok {
			return nil, false
		}

		current = field.Type
	}

	return current, true
}

func completionItemsForType(typeReference ast.TypeReference, model completionModel, allowSchemaLiteral bool) []protocol.CompletionItem {
	resolved := resolveCompletionType(typeReference, model, map[string]struct{}{})
	switch resolved.kind {
	case completionTypeEnum:
		return lo.Map(resolved.enum.members, func(member completionEnumMember, _ int) protocol.CompletionItem {
			detail := member.Detail
			if detail == "" {
				detail = resolved.enum.access(member.Name)
			}

			return protocol.CompletionItem{
				Label:  resolved.enum.access(member.Name),
				Kind:   Ptr(protocol.CompletionItemKindEnumMember),
				Detail: Ptr(detail),
			}
		})
	case completionTypeSchema:
		if !allowSchemaLiteral {
			return nil
		}

		return []protocol.CompletionItem{
			{
				Label: schemaLiteral(resolved.record, model, map[string]struct{}{}),
				Kind:  Ptr(protocol.CompletionItemKindStruct),
			},
		}
	case completionTypeVariant:
		groups := make([][]protocol.CompletionItem, 0, len(resolved.members))
		for _, member := range resolved.members {
			groups = append(groups, completionItemsForType(member, model, allowSchemaLiteral))
		}
		return mergeCompletionItems(groups...)
	default:
		return nil
	}
}

func outputSchemaDirective(file ast.File) (string, bool) {
	directive, ok := lo.Find(file.Output.Directives, func(directive ast.OutputDirective) bool {
		return directive.Kind == ast.OutputDirectiveSchema
	})
	if !ok || directive.Value == "" {
		return "", false
	}

	return directive.Value, true
}

func buildCompletionModel(file ast.File, baseDir string, cache map[string]completionModel) completionModel {
	model := completionModel{
		aliases: map[string]ast.TypeReference{},
		schemas: map[string]ast.RecordType{},
		enums:   map[string]completionEnum{},
	}

	for _, item := range fileScriptItems(file) {
		switch declaration := item.(type) {
		case ast.TypeDeclaration:
			model.aliases[declaration.Name] = declaration.Type
		case ast.SchemaDeclaration:
			model.schemas[declaration.Name] = declaration.Type
		case ast.EnumDeclaration:
			model.enums[declaration.Name] = completionEnumFromDeclaration(declaration)
		}
	}

	for _, importDecl := range file.Imports {
		importPath, ok := stringLiteralValue(importDecl.Path)
		if !ok {
			continue
		}

		resolvedPath := filepath.Clean(filepath.Join(baseDir, importPath))
		importedModel, importedFile, ok := importedCompletionModel(resolvedPath, cache)
		if !ok {
			continue
		}

		for _, name := range importDecl.Identifiers {
			field, ok := lo.Find(importedFile.Output.SchemaFields, func(field ast.OutputSchemaField) bool {
				return field.Name == name
			})
			if !ok {
				continue
			}

			resolved := resolveCompletionType(field.Type, importedModel, map[string]struct{}{})
			switch resolved.kind {
			case completionTypeSchema:
				model.schemas[name] = resolved.record
			case completionTypeEnum:
				model.enums[name] = resolved.enum.rename(name)
			default:
				model.aliases[name] = field.Type
			}
		}
	}

	return model
}

func importedCompletionModel(path string, cache map[string]completionModel) (completionModel, ast.File, bool) {
	if model, ok := cache[path]; ok {
		_, file, _, parsed := parsedFile(path)
		return model, file, parsed
	}

	_, file, _, ok := parsedFile(path)
	if !ok {
		return completionModel{}, ast.File{}, false
	}

	cache[path] = completionModel{
		aliases: map[string]ast.TypeReference{},
		schemas: map[string]ast.RecordType{},
		enums:   map[string]completionEnum{},
	}
	model := buildCompletionModel(file, filepath.Dir(path), cache)
	cache[path] = model
	return model, file, true
}

func resolveCompletionType(typeReference ast.TypeReference, model completionModel, seen map[string]struct{}) completionType {
	switch typed := typeReference.(type) {
	case ast.PrimitiveType:
		return completionType{kind: completionTypePrimitive, primitive: typed.Name}
	case ast.ArrayType:
		return completionType{kind: completionTypeArray}
	case ast.UnionType:
		record, ok := completionUnionRecord(typed.Members, model, seen)
		if !ok {
			return completionType{}
		}
		return completionType{kind: completionTypeSchema, record: record}
	case ast.VariantType:
		return completionType{kind: completionTypeVariant, members: typed.Members}
	case ast.RecordType:
		return completionType{kind: completionTypeSchema, record: typed}
	case ast.NamedType:
		if enumValue, ok := model.enums[typed.Name]; ok {
			return completionType{kind: completionTypeEnum, enum: enumValue}
		}
		if schemaValue, ok := model.schemas[typed.Name]; ok {
			return completionType{kind: completionTypeSchema, record: schemaValue}
		}
		if _, ok := seen[typed.Name]; ok {
			return completionType{}
		}

		aliasValue, ok := model.aliases[typed.Name]
		if !ok {
			return completionType{}
		}

		nextSeen := map[string]struct{}{typed.Name: struct{}{}}
		for name := range seen {
			nextSeen[name] = struct{}{}
		}
		return resolveCompletionType(aliasValue, model, nextSeen)
	default:
		return completionType{}
	}
}

func completionUnionRecord(members []ast.TypeReference, model completionModel, seen map[string]struct{}) (ast.RecordType, bool) {
	merged := ast.RecordType{}
	fieldIndexes := map[string]int{}

	for _, member := range members {
		resolved := resolveCompletionType(member, model, seen)
		if resolved.kind != completionTypeSchema {
			return ast.RecordType{}, false
		}

		for _, field := range resolved.record.Fields {
			index, exists := fieldIndexes[field.Name]
			if !exists {
				fieldIndexes[field.Name] = len(merged.Fields)
				merged.Fields = append(merged.Fields, field)
				continue
			}

			existing := merged.Fields[index]
			if typeReferenceDetail(existing.Type) != typeReferenceDetail(field.Type) {
				return ast.RecordType{}, false
			}
			merged.Fields[index].Optional = existing.Optional && field.Optional
		}
	}

	return merged, true
}

func completionEnumFromDeclaration(declaration ast.EnumDeclaration) completionEnum {
	members := lo.Map(declaration.Members, func(member ast.EnumMember, index int) completionEnumMember {
		return completionEnumMember{
			Name:   member.Name,
			Detail: completionEnumMemberDetail(declaration, member, index),
		}
	})

	return completionEnum{name: declaration.Name, members: members}
}

func completionEnumMemberDetail(declaration ast.EnumDeclaration, member ast.EnumMember, index int) string {
	if member.HasValue {
		return fmt.Sprintf("enum member %s.%s = %s", declaration.Name, member.Name, expressionSummary(member.Value))
	}

	if declaration.BackingType.Name == "string" {
		return fmt.Sprintf("enum member %s.%s = %q", declaration.Name, member.Name, member.Name)
	}

	if declaration.BackingType.Name == "int" {
		return fmt.Sprintf("enum member %s.%s = %d", declaration.Name, member.Name, index)
	}

	return fmt.Sprintf("enum member %s.%s", declaration.Name, member.Name)
}

func schemaLiteral(record ast.RecordType, model completionModel, seen map[string]struct{}) string {
	fields := lo.Map(record.Fields, func(field ast.SchemaField, _ int) string {
		name := field.Name
		if field.Optional {
			name += "?"
		}
		return fmt.Sprintf("%s: %s", name, defaultLiteralForType(field.Type, model, seen))
	})

	return "{ " + strings.Join(fields, ", ") + " }"
}

func defaultLiteralForType(typeReference ast.TypeReference, model completionModel, seen map[string]struct{}) string {
	resolved := resolveCompletionType(typeReference, model, seen)
	switch resolved.kind {
	case completionTypePrimitive:
		switch resolved.primitive {
		case "string":
			return `""`
		case "int":
			return "0"
		case "float":
			return "0.0"
		case "boolean":
			return "false"
		default:
			return `""`
		}
	case completionTypeArray:
		return "[]"
	case completionTypeEnum:
		if len(resolved.enum.members) > 0 {
			return resolved.enum.access(resolved.enum.members[0].Name)
		}
		return `""`
	case completionTypeSchema:
		key := recordTypeDetail(resolved.record)
		if _, ok := seen[key]; ok {
			return "{}"
		}

		nextSeen := map[string]struct{}{key: struct{}{}}
		for name := range seen {
			nextSeen[name] = struct{}{}
		}
		return schemaLiteral(resolved.record, model, nextSeen)
	case completionTypeVariant:
		for _, member := range resolved.members {
			literal := defaultLiteralForType(member, model, seen)
			if literal != "{}" {
				return literal
			}
		}
		return "{}"
	default:
		return "{}"
	}
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

func selfReferenceCompletionItems(prefix string, position protocol.Position) []protocol.CompletionItem {
	items := itemsFromDefinitions([]completionDefinition{{
		Label:  "$self",
		Kind:   protocol.CompletionItemKindKeyword,
		Detail: "output self reference",
	}}, prefix)

	if prefix == "" {
		return items
	}

	replaceStart := position
	replaceStart.Character -= uint32(len(prefix))
	replaceRange := protocol.Range{Start: replaceStart, End: position}
	for index := range items {
		items[index].TextEdit = protocol.TextEdit{
			Range:   replaceRange,
			NewText: items[index].Label,
		}
	}
	return items
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

func mergeCompletionItems(groups ...[]protocol.CompletionItem) []protocol.CompletionItem {
	itemsByLabel := map[string]protocol.CompletionItem{}
	for _, group := range groups {
		for _, item := range group {
			itemsByLabel[item.Label] = item
		}
	}

	items := lo.Values(itemsByLabel)
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

type completionModel struct {
	aliases map[string]ast.TypeReference
	schemas map[string]ast.RecordType
	enums   map[string]completionEnum
}

type completionEnum struct {
	name    string
	members []completionEnumMember
}

type completionEnumMember struct {
	Name   string
	Detail string
}

func (enum completionEnum) access(memberName string) string {
	if enum.name == "" {
		return memberName
	}

	return enum.name + "." + memberName
}

func (enum completionEnum) rename(name string) completionEnum {
	cloned := completionEnum{name: name, members: make([]completionEnumMember, len(enum.members))}
	copy(cloned.members, enum.members)
	if enum.name == "" || enum.name == name {
		return cloned
	}

	oldPrefix := "enum member " + enum.name + "."
	newPrefix := "enum member " + name + "."
	for index, member := range cloned.members {
		if strings.HasPrefix(member.Detail, oldPrefix) {
			member.Detail = newPrefix + strings.TrimPrefix(member.Detail, oldPrefix)
			cloned.members[index] = member
		}
	}

	return cloned
}

type completionTypeKind int

const (
	completionTypeUnknown completionTypeKind = iota
	completionTypePrimitive
	completionTypeArray
	completionTypeSchema
	completionTypeEnum
	completionTypeVariant
)

type completionType struct {
	kind      completionTypeKind
	primitive string
	record    ast.RecordType
	enum      completionEnum
	members   []ast.TypeReference
}

type completionScope int

const (
	completionScopeFile completionScope = iota
	completionScopeScript
	completionScopeOutput
)
