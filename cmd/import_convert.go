package main

import (
	"fmt"
	"math"
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

	"github.com/louiss0/mace/codec"
	"github.com/louiss0/mace/internal/parser"
)

var (
	yamlSchemaPattern    = regexp.MustCompile(`(?m)^\s*#\s*yaml-language-server:\s*\$schema\s*=\s*(\S+)\s*$`)
	tomlSchemaPattern    = regexp.MustCompile(`(?m)^\s*#:schema\s+(\S+)\s*$`)
	importFieldPattern   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	reservedImportFields = map[string]struct{}{
		"alias": {}, "enum": {}, "false": {}, "from": {}, "if": {}, "import": {},
		"null": {}, "output": {}, "schema": {}, "true": {}, "type": {},
	}
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
	name        string
	description string
	value       importExpression
}

type recordExpression struct {
	fields []recordField
}

type mergeExpression struct {
	parts []importExpression
}

type omittedExpression struct{}

type yamlAnchor struct {
	path      string
	value     importExpression
	resolving bool
}

type yamlImportState struct {
	anchors    map[string]yamlAnchor
	hoists     map[string]importExpression
	hoistOrder []string
}

type tomlImportConfig struct {
	fieldOrder map[string][]string
}

func importJSONSource(input string) (string, error) {
	return codec.ImportJSON(input)
}

func importYAMLSource(path string, input string) (string, error) {
	return importYAMLSourceToPath(path, defaultImportOutputPath(path), input)
}

func importYAMLSourceToPath(sourcePath string, outputPath string, input string) (string, error) {
	schemaPath := adjustedSchemaPath(sourcePath, outputPath, input, yamlSchemaPattern)

	file, err := yamlparser.Parse(yamllexer.Tokenize(input), 0)
	if err != nil {
		return "", fmt.Errorf("import yaml: %w", err)
	}

	root, err := yamlRootExpression(file)
	if err != nil {
		return "", err
	}
	applyYAMLTopLevelFieldComments(&root, input)

	return formatImportedOutput(schemaPath, root)
}

func importTOMLSource(path string, input string) (string, error) {
	return importTOMLSourceToPath(path, defaultImportOutputPath(path), input)
}

func importTOMLSourceToPath(sourcePath string, outputPath string, input string) (string, error) {
	schemaPath := adjustedSchemaPath(sourcePath, outputPath, input, tomlSchemaPattern)

	var value map[string]any
	metadata, err := burnttoml.Decode(input, &value)
	if err != nil {
		if path := tomlNumericErrorPath(input, err.Error()); path != "" {
			return "", fmt.Errorf("import toml: %w at %s", err, path)
		}
		return "", fmt.Errorf("import toml: %w", err)
	}

	root, err := tomlExpression(value, nil, tomlImportConfig{fieldOrder: tomlFieldOrder(metadata)})
	if err != nil {
		return "", err
	}

	record := root.(recordExpression)

	return formatImportedOutput(schemaPath, record)
}

func applyYAMLTopLevelFieldComments(root *recordExpression, input string) {
	descriptions := map[string]string{}
	pending := ""
	for _, line := range strings.Split(input, "\n") {
		if strings.HasPrefix(line, "#") {
			pending = strings.TrimSpace(strings.TrimPrefix(line, "#"))
			continue
		}
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			pending = ""
			continue
		}
		separator := strings.Index(line, ":")
		if separator < 1 {
			pending = ""
			continue
		}
		name := strings.Trim(strings.TrimSpace(line[:separator]), `"'`)
		normalized, err := normalizedImportFieldName(name)
		if err != nil {
			pending = ""
			continue
		}
		description := pending
		if comment := strings.Index(line[separator+1:], "#"); comment >= 0 {
			description = strings.TrimSpace(line[separator+1+comment+1:])
		}
		if description != "" {
			descriptions[normalized] = description
		}
		pending = ""
	}
	for index := range root.fields {
		if description := descriptions[root.fields[index].name]; description != "" {
			root.fields[index].description = description
		}
	}
}

func yamlRootExpression(file *yamlast.File) (recordExpression, error) {
	if len(file.Docs) == 0 {
		return recordExpression{}, fmt.Errorf("import yaml: expected one document")
	}
	if len(file.Docs) > 1 {
		return recordExpression{}, fmt.Errorf("import yaml: multiple documents are not supported")
	}

	state := yamlImportState{
		anchors: map[string]yamlAnchor{},
		hoists:  map[string]importExpression{},
	}
	expression, err := yamlNodeExpression(file.Docs[0].Body, "", &state)
	if err != nil {
		return recordExpression{}, err
	}
	if isOmittedImportExpression(expression) {
		return recordExpression{}, fmt.Errorf("import yaml: null document root is not supported")
	}
	if sequence, ok := expression.(arrayExpression); ok {
		return recordExpression{fields: []recordField{{name: "root", value: sequence}}}, nil
	}

	record, ok, err := yamlDocumentRecord(expression, &state)
	if err != nil {
		return recordExpression{}, err
	}
	if !ok {
		return recordExpression{}, fmt.Errorf("import yaml: scalar document root is not supported")
	}
	return yamlRecordWithHoists(record, &state)
}

func yamlRecordWithHoists(record recordExpression, state *yamlImportState) (recordExpression, error) {
	if len(state.hoists) == 0 {
		return record, nil
	}

	fieldByName := map[string]recordField{}
	fieldOrder := make([]string, 0, len(record.fields)+len(state.hoists))
	for _, field := range record.fields {
		fieldByName[field.name] = field
		fieldOrder = append(fieldOrder, field.name)
	}

	for _, name := range state.hoistOrder {
		if _, exists := fieldByName[name]; exists {
			continue
		}
		value, ok := state.hoists[name]
		if !ok || isOmittedImportExpression(value) {
			continue
		}
		fieldByName[name] = recordField{name: name, value: value}
		fieldOrder = append(fieldOrder, name)
	}

	orderedNames, err := yamlOrderedFieldNames(fieldOrder, fieldByName)
	if err != nil {
		return recordExpression{}, err
	}

	fields := make([]recordField, 0, len(orderedNames))
	for _, name := range orderedNames {
		fields = append(fields, fieldByName[name])
	}

	return recordExpression{fields: fields}, nil
}

func yamlNodeExpression(node yamlast.Node, selfPath string, state *yamlImportState) (importExpression, error) {
	switch typed := node.(type) {
	case nil:
		return omittedExpression{}, nil
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
		state.anchors[name] = yamlAnchor{path: targetPath, resolving: true}
		value, err := yamlNodeExpression(typed.Value, selfPath, state)
		if err != nil {
			return nil, err
		}
		anchor := state.anchors[name]
		anchor.value = value
		anchor.resolving = false
		state.anchors[name] = anchor
		if selfPath == targetPath {
			return value, nil
		}
		if !isOmittedImportExpression(value) {
			yamlRememberHoist(state, name, value)
		}
		return rawExpression{text: targetPath}, nil
	case *yamlast.AliasNode:
		name, err := yamlAliasName(typed.Value)
		if err != nil {
			return nil, err
		}
		anchor, ok := state.anchors[name]
		if !ok {
			return nil, fmt.Errorf("import yaml: unknown alias %q", name)
		}
		if anchor.resolving {
			return nil, fmt.Errorf("import yaml: cyclic alias %q", name)
		}
		if isOmittedImportExpression(anchor.value) {
			return omittedExpression{}, nil
		}
		return rawExpression{text: anchor.path}, nil
	case *yamlast.TagNode:
		return nil, fmt.Errorf("import yaml: unsupported application tag")
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
			if isOmittedImportExpression(expression) {
				continue
			}
			items = append(items, expression)
		}
		return arrayExpression{items: items}, nil
	case *yamlast.StringNode:
		return rawExpression{text: strconv.Quote(typed.Value)}, nil
	case *yamlast.LiteralNode:
		return rawExpression{text: tripleQuotedString(typed.Value.Value)}, nil
	case *yamlast.BoolNode:
		if typed.Value {
			return rawExpression{text: "true"}, nil
		}
		return rawExpression{text: "false"}, nil
	case *yamlast.IntegerNode:
		return rawExpression{text: fmt.Sprint(typed.Value)}, nil
	case *yamlast.FloatNode:
		return rawExpression{text: yamlFloatLiteral(typed.Value)}, nil
	case *yamlast.InfinityNode:
		if strings.HasPrefix(typed.GetToken().Value, "-") {
			return rawExpression{text: strconv.Quote("-Infinity")}, nil
		}
		return rawExpression{text: strconv.Quote("Infinity")}, nil
	case *yamlast.NanNode:
		return rawExpression{text: strconv.Quote("NaN")}, nil
	case *yamlast.NullNode:
		return omittedExpression{}, nil
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
		name, err = normalizedImportFieldName(name)
		if err != nil {
			return nil, err
		}
		if slices.ContainsFunc(fields, func(field recordField) bool { return field.name == name }) {
			return nil, fmt.Errorf("import yaml: duplicate normalized field %q", name)
		}
		expression, err := yamlNodeExpression(iter.Value(), selfFieldPath(selfPath, name), state)
		if err != nil {
			return nil, err
		}
		if isOmittedImportExpression(expression) {
			continue
		}
		fields = append(fields, recordField{name: name, value: expression})
	}

	record := recordExpression{fields: fields}
	if len(mergeParts) == 0 {
		return record, nil
	}

	parts := append([]importExpression{}, mergeParts...)
	parts = append(parts, record)
	return yamlResolvedRecord(mergeExpression{parts: parts}, state)
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
		return tomlFloatExpression(float64(typed), 32), nil
	case float64:
		return tomlFloatExpression(typed, 64), nil
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

func tomlNumericErrorPath(input string, message string) string {
	keyMatch := regexp.MustCompile(`last key "([^"]+)"`).FindStringSubmatch(message)
	valueMatch := regexp.MustCompile(`([-+]?\d+) is out of range`).FindStringSubmatch(message)
	if len(keyMatch) != 2 || len(valueMatch) != 2 {
		return ""
	}

	for _, line := range strings.Split(input, "\n") {
		if !strings.Contains(line, valueMatch[1]) {
			continue
		}
		opening := strings.Index(line, "[")
		closing := strings.LastIndex(line, "]")
		if opening < 0 || closing <= opening {
			return keyMatch[1]
		}
		for index, item := range strings.Split(line[opening+1:closing], ",") {
			if strings.TrimSpace(item) == valueMatch[1] {
				return fmt.Sprintf("%s[%d]", keyMatch[1], index)
			}
		}
	}
	return keyMatch[1]
}

func tomlFloatExpression(value float64, bits int) importExpression {
	if math.IsNaN(value) {
		return rawExpression{text: strconv.Quote("NaN")}
	}
	if math.IsInf(value, 1) {
		return rawExpression{text: strconv.Quote("Infinity")}
	}
	if math.IsInf(value, -1) {
		return rawExpression{text: strconv.Quote("-Infinity")}
	}
	return rawExpression{text: strconv.FormatFloat(value, 'f', -1, bits)}
}

func tomlRecordExpression(record map[string]any, path []string, config tomlImportConfig) (recordExpression, error) {
	orderedNames := orderedRecordNames(record, path, config.fieldOrder)
	fields := make([]recordField, 0, len(orderedNames))
	for _, name := range orderedNames {
		normalized, err := normalizedImportFieldName(name)
		if err != nil {
			return recordExpression{}, err
		}
		if slices.ContainsFunc(fields, func(field recordField) bool { return field.name == normalized }) {
			return recordExpression{}, fmt.Errorf("import toml: duplicate normalized field %q", normalized)
		}
		expression, err := tomlExpression(record[name], append(path, normalized), config)
		if err != nil {
			return recordExpression{}, err
		}
		fields = append(fields, recordField{name: normalized, value: expression})
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

func adjustedSchemaPath(sourcePath string, outputPath string, input string, pattern *regexp.Regexp) string {
	matches := pattern.FindStringSubmatch(input)
	if len(matches) != 2 {
		return ""
	}
	return adjustedSchemaReferenceToMace(matches[1], sourcePath, outputPath)
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

func adjustedSchemaReferenceToMace(reference string, sourcePath string, outputPath string) string {
	trimmed := strings.TrimSpace(reference)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "://") {
		return schemaReferenceToMace(trimmed)
	}

	basePath, suffix := schemaReferenceParts(trimmed)
	if basePath == "" {
		return schemaReferenceToMace(trimmed)
	}

	if filepath.IsAbs(basePath) {
		return filepath.ToSlash(schemaPathToMace(basePath, string(filepath.Separator))) + suffix
	}

	resolvedPath := filepath.Clean(filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(basePath)))
	rebasedPath := resolvedPath
	if outputPath != "" {
		relativePath, err := filepath.Rel(filepath.Dir(outputPath), resolvedPath)
		if err == nil {
			rebasedPath = relativePath
		}
	}

	return explicitRelativeSchemaPath(filepath.ToSlash(schemaPathToMace(rebasedPath, string(filepath.Separator)))) + suffix
}

func schemaReferenceParts(reference string) (string, string) {
	for index, character := range reference {
		if character == '?' || character == '#' {
			return reference[:index], reference[index:]
		}
	}
	return reference, ""
}

func explicitRelativeSchemaPath(path string) string {
	if path == "" {
		return ""
	}
	if strings.HasPrefix(path, "./") || strings.HasPrefix(path, "../") {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	return "./" + path
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
	directive := `[output = 'data']`
	if schemaPath != "" {
		directive = fmt.Sprintf(`[output = 'data', schema_file = '%s']`, schemaPath)
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

	formatted, err := formatMaceFile(file)
	if err != nil {
		return "", fmt.Errorf("import mace: format generated source: %w", err)
	}

	return formatted, nil
}

func defaultImportOutputPath(sourcePath string) string {
	return strings.TrimSuffix(sourcePath, filepath.Ext(sourcePath)) + ".mace"
}

func normalizedImportFieldName(name string) (string, error) {
	normalized := strings.Join(strings.Fields(name), "_")
	if err := validateImportFieldName(normalized); err != nil {
		return "", err
	}
	if _, reserved := reservedImportFields[normalized]; reserved {
		return "", fmt.Errorf("import mace: reserved field name %q", name)
	}
	return normalized, nil
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

func isOmittedImportExpression(expression importExpression) bool {
	_, isOmitted := expression.(omittedExpression)
	return isOmitted
}

func yamlRememberHoist(state *yamlImportState, name string, value importExpression) {
	if _, exists := state.hoists[name]; !exists {
		state.hoistOrder = append(state.hoistOrder, name)
	}
	state.hoists[name] = value
}

func yamlDocumentRecord(expression importExpression, state *yamlImportState) (recordExpression, bool, error) {
	switch typed := expression.(type) {
	case recordExpression:
		return typed, true, nil
	case mergeExpression:
		record, err := yamlResolvedRecord(typed, state)
		return record, err == nil, err
	default:
		return recordExpression{}, false, nil
	}
}

func yamlResolvedRecord(expression importExpression, state *yamlImportState) (recordExpression, error) {
	switch typed := expression.(type) {
	case recordExpression:
		return typed, nil
	case mergeExpression:
		fields := []recordField{}
		indexByName := map[string]int{}
		for _, part := range typed.parts {
			record, err := yamlResolvedRecord(part, state)
			if err != nil {
				return recordExpression{}, err
			}
			for _, field := range record.fields {
				if index, exists := indexByName[field.name]; exists {
					fields[index] = field
					continue
				}
				indexByName[field.name] = len(fields)
				fields = append(fields, field)
			}
		}
		return recordExpression{fields: fields}, nil
	case rawExpression:
		name, ok := yamlTopLevelReferenceName(typed.text)
		if !ok {
			return recordExpression{}, fmt.Errorf("import yaml: merge source %q is not a record", typed.text)
		}
		anchor, exists := state.anchors[name]
		if !exists {
			return recordExpression{}, fmt.Errorf("import yaml: unknown merge source %q", typed.text)
		}
		return yamlResolvedRecord(anchor.value, state)
	default:
		return recordExpression{}, fmt.Errorf("import yaml: merge source %T is not a record", expression)
	}
}

func yamlOrderedFieldNames(initialOrder []string, fieldByName map[string]recordField) ([]string, error) {
	orderIndex := map[string]int{}
	for index, name := range initialOrder {
		orderIndex[name] = index
	}

	dependenciesByName := map[string]map[string]struct{}{}
	dependentsByName := map[string][]string{}
	for name, field := range fieldByName {
		dependencies := yamlExpressionDependencies(field.value, fieldByName)
		dependenciesByName[name] = dependencies
		for dependency := range dependencies {
			dependentsByName[dependency] = append(dependentsByName[dependency], name)
		}
	}

	ready := []string{}
	for _, name := range initialOrder {
		if len(dependenciesByName[name]) == 0 {
			ready = append(ready, name)
		}
	}

	sortReady := func() {
		slices.SortFunc(ready, func(left string, right string) int {
			return orderIndex[left] - orderIndex[right]
		})
	}
	sortReady()

	ordered := make([]string, 0, len(fieldByName))
	queued := map[string]struct{}{}
	for _, name := range ready {
		queued[name] = struct{}{}
	}

	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		ordered = append(ordered, name)

		for _, dependent := range dependentsByName[name] {
			dependencies := dependenciesByName[dependent]
			delete(dependencies, name)
			if len(dependencies) != 0 {
				continue
			}
			ready = append(ready, dependent)
			queued[dependent] = struct{}{}
			sortReady()
		}
	}

	if len(ordered) != len(fieldByName) {
		return nil, fmt.Errorf("import yaml: top-level anchor references are cyclic")
	}
	return ordered, nil
}

func yamlExpressionDependencies(expression importExpression, fieldByName map[string]recordField) map[string]struct{} {
	dependencies := map[string]struct{}{}

	var visit func(importExpression)
	visit = func(expression importExpression) {
		switch typed := expression.(type) {
		case rawExpression:
			name, ok := yamlTopLevelReferenceName(typed.text)
			if !ok {
				return
			}
			if _, exists := fieldByName[name]; exists {
				dependencies[name] = struct{}{}
			}
		case arrayExpression:
			for _, item := range typed.items {
				visit(item)
			}
		case recordExpression:
			for _, field := range typed.fields {
				visit(field.value)
			}
		case mergeExpression:
			for _, part := range typed.parts {
				visit(part)
			}
		}
	}

	visit(expression)
	return dependencies
}

func yamlTopLevelReferenceName(path string) (string, bool) {
	if !strings.HasPrefix(path, "$self.") {
		return "", false
	}

	name := strings.TrimPrefix(path, "$self.")
	if name == "" || strings.ContainsAny(name, ".[") {
		return "", false
	}
	return name, true
}

func yamlFloatLiteral(value float64) string {
	if math.IsNaN(value) {
		return strconv.Quote("NaN")
	}
	if math.IsInf(value, 1) {
		return strconv.Quote("Infinity")
	}
	if math.IsInf(value, -1) {
		return strconv.Quote("-Infinity")
	}

	literal := strconv.FormatFloat(value, 'f', -1, 64)
	if strings.ContainsAny(literal, ".") {
		return literal
	}
	return literal + ".0"
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
		value := field.value.render(depth + 1)
		line := indent + field.name + ": " + value
		if index < len(expression.fields)-1 {
			line += ","
		}
		if field.description != "" {
			line += " /# " + field.description
		}
		lines = append(lines, line)
	}
	lines = append(lines, closingIndent+"}")
	return strings.Join(lines, "\n")
}

func (expression mergeExpression) render(depth int) string {
	record, err := yamlResolvedRecord(expression, &yamlImportState{})
	if err != nil {
		return recordExpression{}.render(depth)
	}
	return record.render(depth)
}

func (expression omittedExpression) render(int) string {
	return ""
}
