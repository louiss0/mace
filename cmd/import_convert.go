package main

import (
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	burnttoml "github.com/BurntSushi/toml"
	yamlast "github.com/goccy/go-yaml/ast"
	yamllexer "github.com/goccy/go-yaml/lexer"
	yamlparser "github.com/goccy/go-yaml/parser"

	"github.com/louiss0/mace/internal/formatter"
	"github.com/louiss0/mace/internal/parser"
)

var (
	yamlSchemaPattern  = regexp.MustCompile(`(?m)^\s*#\s*yaml-language-server:\s*\$schema\s*=\s*(\S+)\s*$`)
	tomlSchemaPattern  = regexp.MustCompile(`(?m)^\s*#:schema\s+(\S+)\s*$`)
	importFieldPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type importExpression interface {
	render(depth int) string
}

type rawExpression struct {
	text string
}

type arrayExpression struct {
	items []importExpression
}

type recordField struct {
	name  string
	value importExpression
}

type recordExpression struct {
	fields []recordField
}

type mergeExpression struct {
	parts []importExpression
}

type yamlImportState struct {
	anchors map[string]string
	hoists  map[string]importExpression
}

type tomlImportConfig struct {
	fieldOrder map[string][]string
}

func importYAMLSource(path string, input string) (string, error) {
	schemaPath := adjustedSchemaPath(path, input, yamlSchemaPattern)

	file, err := yamlparser.Parse(yamllexer.Tokenize(input), 0)
	if err != nil {
		return "", fmt.Errorf("import yaml: %w", err)
	}

	root, err := yamlRootExpression(file)
	if err != nil {
		return "", err
	}

	return formatImportedOutput(schemaPath, root)
}

func importTOMLSource(path string, input string) (string, error) {
	schemaPath := adjustedSchemaPath(path, input, tomlSchemaPattern)

	var value map[string]any
	metadata, err := burnttoml.Decode(input, &value)
	if err != nil {
		return "", fmt.Errorf("import toml: %w", err)
	}

	root, err := tomlExpression(value, nil, tomlImportConfig{fieldOrder: tomlFieldOrder(metadata)})
	if err != nil {
		return "", err
	}

	record, ok := root.(recordExpression)
	if !ok {
		return "", fmt.Errorf("import toml: expected record root")
	}

	return formatImportedOutput(schemaPath, record)
}

func yamlRootExpression(file *yamlast.File) (recordExpression, error) {
	if len(file.Docs) == 0 {
		return recordExpression{}, fmt.Errorf("import yaml: expected at least one document")
	}

	if len(file.Docs) == 1 {
		state := yamlImportState{anchors: map[string]string{}, hoists: map[string]importExpression{}}
		expression, err := yamlNodeExpression(file.Docs[0].Body, "", &state)
		if err != nil {
			return recordExpression{}, err
		}
		record, ok := expression.(recordExpression)
		if !ok {
			return recordExpression{fields: []recordField{{name: "document_1", value: expression}}}, nil
		}
		return yamlRecordWithHoists(record, &state)
	}

	fields := make([]recordField, 0, len(file.Docs))
	for index, document := range file.Docs {
		name := fmt.Sprintf("document_%d", index+1)
		state := yamlImportState{anchors: map[string]string{}, hoists: map[string]importExpression{}}
		expression, err := yamlNodeExpression(document.Body, "", &state)
		if err != nil {
			return recordExpression{}, err
		}
		record, ok := expression.(recordExpression)
		if !ok {
			fields = append(fields, recordField{name: name, value: expression})
			continue
		}
		record, err = yamlRecordWithHoists(record, &state)
		if err != nil {
			return recordExpression{}, err
		}
		fields = append(fields, recordField{name: name, value: record})
	}

	return recordExpression{fields: fields}, nil
}

func yamlRecordWithHoists(record recordExpression, state *yamlImportState) (recordExpression, error) {
	if len(state.hoists) == 0 {
		return record, nil
	}

	rootNames := map[string]struct{}{}
	for _, field := range record.fields {
		rootNames[field.name] = struct{}{}
	}

	hoistedNames := make([]string, 0, len(state.hoists))
	for name := range state.hoists {
		if _, exists := rootNames[name]; exists {
			continue
		}
		hoistedNames = append(hoistedNames, name)
	}
	slices.Sort(hoistedNames)

	fields := make([]recordField, 0, len(hoistedNames)+len(record.fields))
	for _, name := range hoistedNames {
		fields = append(fields, recordField{name: name, value: state.hoists[name]})
	}
	fields = append(fields, record.fields...)
	return recordExpression{fields: fields}, nil
}

func yamlNodeExpression(node yamlast.Node, selfPath string, state *yamlImportState) (importExpression, error) {
	switch typed := node.(type) {
	case nil:
		return rawExpression{text: `""`}, nil
	case *yamlast.DocumentNode:
		return yamlNodeExpression(typed.Body, selfPath, state)
	case *yamlast.AnchorNode:
		name, err := yamlAnchorName(typed.Name)
		if err != nil {
			return nil, err
		}
		if selfPath == "" {
			return nil, fmt.Errorf("import yaml: anchor %q must be attached to a named value", name)
		}
		targetPath := "$self." + name
		state.anchors[name] = targetPath
		value, err := yamlNodeExpression(typed.Value, selfPath, state)
		if err != nil {
			return nil, err
		}
		if selfPath == targetPath {
			return value, nil
		}
		state.hoists[name] = value
		return rawExpression{text: targetPath}, nil
	case *yamlast.AliasNode:
		name, err := yamlAliasName(typed.Value)
		if err != nil {
			return nil, err
		}
		targetPath, ok := state.anchors[name]
		if !ok {
			return nil, fmt.Errorf("import yaml: unknown alias %q", name)
		}
		return rawExpression{text: targetPath}, nil
	case *yamlast.TagNode:
		return yamlNodeExpression(typed.Value, selfPath, state)
	case *yamlast.MappingNode:
		return yamlMappingExpression(typed.MapRange(), selfPath, state)
	case *yamlast.MappingValueNode:
		return yamlMappingExpression(typed.MapRange(), selfPath, state)
	case *yamlast.SequenceNode:
		items := make([]importExpression, 0, len(typed.Values))
		for index, item := range typed.Values {
			expression, err := yamlNodeExpression(item, selfIndexPath(selfPath, index), state)
			if err != nil {
				return nil, err
			}
			items = append(items, expression)
		}
		return arrayExpression{items: items}, nil
	case *yamlast.StringNode:
		return rawExpression{text: strconv.Quote(typed.Value)}, nil
	case *yamlast.LiteralNode:
		return rawExpression{text: tripleQuotedString(typed.Value.Value)}, nil
	case *yamlast.BoolNode:
		return rawExpression{text: typed.GetToken().Value}, nil
	case *yamlast.IntegerNode:
		return rawExpression{text: fmt.Sprint(typed.Value)}, nil
	case *yamlast.FloatNode:
		return rawExpression{text: typed.GetToken().Value}, nil
	case *yamlast.InfinityNode:
		return rawExpression{text: strconv.Quote(typed.GetToken().Value)}, nil
	case *yamlast.NanNode:
		return rawExpression{text: strconv.Quote(typed.GetToken().Value)}, nil
	case *yamlast.NullNode:
		return rawExpression{text: `""`}, nil
	default:
		return nil, fmt.Errorf("import yaml: unsupported node %T", node)
	}
}

func yamlMappingExpression(iter *yamlast.MapNodeIter, selfPath string, state *yamlImportState) (importExpression, error) {
	mergeParts := []importExpression{}
	fields := []recordField{}

	for iter.Next() {
		key := iter.Key()
		if key.IsMergeKey() {
			parts, err := yamlMergeExpressions(iter.Value(), selfPath, state)
			if err != nil {
				return nil, err
			}
			mergeParts = append(mergeParts, parts...)
			continue
		}

		name, err := yamlFieldName(key)
		if err != nil {
			return nil, err
		}
		if err := validateImportFieldName(name); err != nil {
			return nil, err
		}
		expression, err := yamlNodeExpression(iter.Value(), selfFieldPath(selfPath, name), state)
		if err != nil {
			return nil, err
		}
		fields = append(fields, recordField{name: name, value: expression})
	}

	record := recordExpression{fields: fields}
	if len(mergeParts) == 0 {
		return record, nil
	}

	parts := append([]importExpression{}, mergeParts...)
	parts = append(parts, record)
	return mergeExpression{parts: parts}, nil
}

func yamlMergeExpressions(node yamlast.Node, selfPath string, state *yamlImportState) ([]importExpression, error) {
	switch typed := node.(type) {
	case *yamlast.TagNode:
		return yamlMergeExpressions(typed.Value, selfPath, state)
	case *yamlast.SequenceNode:
		parts := make([]importExpression, 0, len(typed.Values))
		for index, item := range typed.Values {
			expression, err := yamlNodeExpression(item, selfIndexPath(selfPath, index), state)
			if err != nil {
				return nil, err
			}
			parts = append(parts, expression)
		}
		return parts, nil
	default:
		expression, err := yamlNodeExpression(node, selfPath, state)
		if err != nil {
			return nil, err
		}
		return []importExpression{expression}, nil
	}
}

func yamlFieldName(node yamlast.MapKeyNode) (string, error) {
	switch typed := node.(type) {
	case *yamlast.StringNode:
		return typed.Value, nil
	case *yamlast.MappingKeyNode:
		return yamlFieldNameFromNode(typed.Value)
	default:
		return yamlFieldNameFromNode(node)
	}
}

func yamlFieldNameFromNode(node yamlast.Node) (string, error) {
	switch typed := node.(type) {
	case *yamlast.StringNode:
		return typed.Value, nil
	default:
		return "", fmt.Errorf("import yaml: unsupported map key %T", node)
	}
}

func yamlAnchorName(node yamlast.Node) (string, error) {
	switch typed := node.(type) {
	case *yamlast.StringNode:
		return typed.Value, nil
	default:
		return "", fmt.Errorf("import yaml: unsupported anchor name %T", node)
	}
}

func yamlAliasName(node yamlast.Node) (string, error) {
	switch typed := node.(type) {
	case *yamlast.StringNode:
		return typed.Value, nil
	default:
		return "", fmt.Errorf("import yaml: unsupported alias %T", node)
	}
}

func tomlExpression(value any, path []string, config tomlImportConfig) (importExpression, error) {
	if stringer, ok := value.(fmt.Stringer); ok {
		if _, isTime := value.(time.Time); !isTime {
			return rawExpression{text: strconv.Quote(stringer.String())}, nil
		}
	}

	switch typed := value.(type) {
	case nil:
		return rawExpression{text: `""`}, nil
	case string:
		if strings.ContainsRune(typed, '\n') {
			return rawExpression{text: tripleQuotedString(typed)}, nil
		}
		return rawExpression{text: strconv.Quote(typed)}, nil
	case bool:
		if typed {
			return rawExpression{text: "true"}, nil
		}
		return rawExpression{text: "false"}, nil
	case int, int8, int16, int32, int64:
		return rawExpression{text: fmt.Sprintf("%d", reflect.ValueOf(typed).Int())}, nil
	case uint, uint8, uint16, uint32, uint64:
		return rawExpression{text: fmt.Sprintf("%d", reflect.ValueOf(typed).Uint())}, nil
	case float32:
		return rawExpression{text: strconv.FormatFloat(float64(typed), 'f', -1, 32)}, nil
	case float64:
		return rawExpression{text: strconv.FormatFloat(typed, 'f', -1, 64)}, nil
	case time.Time:
		return rawExpression{text: strconv.Quote(typed.Format(time.RFC3339Nano))}, nil
	case map[string]any:
		return tomlRecordExpression(typed, path, config)
	case []any:
		items := make([]importExpression, 0, len(typed))
		for index, item := range typed {
			expression, err := tomlExpression(item, appendIndex(path, index), config)
			if err != nil {
				return nil, err
			}
			items = append(items, expression)
		}
		return arrayExpression{items: items}, nil
	case []map[string]any:
		items := make([]importExpression, 0, len(typed))
		for index, item := range typed {
			expression, err := tomlRecordExpression(item, appendIndex(path, index), config)
			if err != nil {
				return nil, err
			}
			items = append(items, expression)
		}
		return arrayExpression{items: items}, nil
	default:
		return reflectTOMLExpression(reflect.ValueOf(value), path, config)
	}
}

func tomlRecordExpression(record map[string]any, path []string, config tomlImportConfig) (recordExpression, error) {
	orderedNames := orderedRecordNames(record, path, config.fieldOrder)
	fields := make([]recordField, 0, len(orderedNames))
	for _, name := range orderedNames {
		if err := validateImportFieldName(name); err != nil {
			return recordExpression{}, err
		}
		expression, err := tomlExpression(record[name], append(path, name), config)
		if err != nil {
			return recordExpression{}, err
		}
		fields = append(fields, recordField{name: name, value: expression})
	}
	return recordExpression{fields: fields}, nil
}

func reflectTOMLExpression(value reflect.Value, path []string, config tomlImportConfig) (importExpression, error) {
	if !value.IsValid() {
		return rawExpression{text: `""`}, nil
	}

	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return rawExpression{text: `""`}, nil
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		items := make([]importExpression, 0, value.Len())
		for index := 0; index < value.Len(); index++ {
			expression, err := reflectTOMLExpression(value.Index(index), appendIndex(path, index), config)
			if err != nil {
				return nil, err
			}
			items = append(items, expression)
		}
		return arrayExpression{items: items}, nil
	case reflect.Map:
		record := map[string]any{}
		for _, key := range value.MapKeys() {
			record[key.String()] = value.MapIndex(key).Interface()
		}
		return tomlRecordExpression(record, path, config)
	default:
		return nil, fmt.Errorf("import toml: unsupported value %T", value.Interface())
	}
}

func tomlFieldOrder(metadata burnttoml.MetaData) map[string][]string {
	fieldOrder := map[string][]string{"": {}}
	seen := map[string]map[string]struct{}{"": {}}

	for _, key := range metadata.Keys() {
		for index := range key {
			parent := pathKey(key[:index])
			name := key[index]
			if _, ok := seen[parent]; !ok {
				seen[parent] = map[string]struct{}{}
			}
			if _, ok := seen[parent][name]; ok {
				continue
			}
			seen[parent][name] = struct{}{}
			fieldOrder[parent] = append(fieldOrder[parent], name)
		}
	}

	return fieldOrder
}

func orderedRecordNames(record map[string]any, path []string, fieldOrder map[string][]string) []string {
	ordered := []string{}
	used := map[string]struct{}{}

	for _, name := range fieldOrder[pathKey(path)] {
		if _, ok := record[name]; !ok {
			continue
		}
		ordered = append(ordered, name)
		used[name] = struct{}{}
	}

	remaining := make([]string, 0, len(record))
	for name := range record {
		if _, ok := used[name]; ok {
			continue
		}
		remaining = append(remaining, name)
	}
	slices.Sort(remaining)

	return append(ordered, remaining...)
}

func adjustedSchemaPath(_ string, input string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(input)
	if len(matches) != 2 {
		return ""
	}
	return schemaReferenceToMace(matches[1])
}

func schemaReferenceToMace(reference string) string {
	if reference == "" {
		return ""
	}

	trimmed := strings.TrimSpace(reference)
	if strings.Contains(trimmed, "://") {
		parts := strings.SplitN(trimmed, "?", 2)
		parts[0] = schemaPathToMace(parts[0], "/")
		return strings.Join(parts, "?")
	}

	separator := string(filepath.Separator)
	if strings.Contains(trimmed, "/") {
		separator = "/"
	}
	return schemaPathToMace(trimmed, separator)
}

func schemaPathToMace(path string, separator string) string {
	fragment := ""
	basePath := path
	if hash := strings.Index(basePath, "#"); hash >= 0 {
		fragment = basePath[hash:]
		basePath = basePath[:hash]
	}

	query := ""
	if question := strings.Index(basePath, "?"); question >= 0 {
		query = basePath[question:]
		basePath = basePath[:question]
	}

	if extension := filepath.Ext(basePath); extension != "" {
		basePath = strings.TrimSuffix(basePath, extension)
	}
	basePath += ".mace"
	if separator == "/" {
		basePath = filepath.ToSlash(basePath)
	}
	return basePath + query + fragment
}

func formatImportedOutput(schemaPath string, root recordExpression) (string, error) {
	directive := `[output = data]`
	if schemaPath != "" {
		directive = fmt.Sprintf(`[output = data, schema_file = %q]`, schemaPath)
	}

	source := directive + "\n" + root.render(0)
	return formatImportedSource(source)
}

func formatImportedSource(source string) (string, error) {
	tokens, err := lex(source)
	if err != nil {
		return "", fmt.Errorf("import mace: lex generated source: %w", err)
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return "", fmt.Errorf("import mace: parse generated source: %w", err)
	}

	formatted, err := formatter.FormatFile(file)
	if err != nil {
		return "", fmt.Errorf("import mace: format generated source: %w", err)
	}

	return formatted, nil
}

func validateImportFieldName(name string) error {
	if importFieldPattern.MatchString(name) {
		return nil
	}
	return fmt.Errorf("import mace: unsupported field name %q", name)
}

func selfFieldPath(base string, name string) string {
	if base == "" {
		return "$self." + name
	}
	return base + "." + name
}

func selfIndexPath(base string, index int) string {
	if base == "" {
		return ""
	}
	return fmt.Sprintf("%s[%d]", base, index)
}

func appendIndex(path []string, index int) []string {
	return append(append([]string{}, path...), fmt.Sprintf("[%d]", index))
}

func pathKey(path []string) string {
	return strings.Join(path, ".")
}

func tripleQuotedString(value string) string {
	return `"""` + value + `"""`
}

func (expression rawExpression) render(int) string {
	return expression.text
}

func (expression arrayExpression) render(depth int) string {
	if len(expression.items) == 0 {
		return "[]"
	}

	indent := strings.Repeat("  ", depth+1)
	closingIndent := strings.Repeat("  ", depth)
	lines := []string{"["}
	for index, item := range expression.items {
		line := indent + item.render(depth+1)
		if index < len(expression.items)-1 {
			line += ","
		}
		lines = append(lines, line)
	}
	lines = append(lines, closingIndent+"]")
	return strings.Join(lines, "\n")
}

func (expression recordExpression) render(depth int) string {
	if len(expression.fields) == 0 {
		return "{}"
	}

	indent := strings.Repeat("  ", depth+1)
	closingIndent := strings.Repeat("  ", depth)
	lines := []string{"{"}
	for index, field := range expression.fields {
		line := indent + field.name + ": " + field.value.render(depth+1)
		if index < len(expression.fields)-1 {
			line += ","
		}
		lines = append(lines, line)
	}
	lines = append(lines, closingIndent+"}")
	return strings.Join(lines, "\n")
}

func (expression mergeExpression) render(depth int) string {
	parts := make([]string, 0, len(expression.parts))
	for _, part := range expression.parts {
		parts = append(parts, part.render(depth))
	}
	return strings.Join(parts, " <> ")
}
