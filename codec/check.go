package codec

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	burnttoml "github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type CheckIssue struct {
	Path     string `json:"path"`
	Reason   string `json:"reason"`
	Format   string `json:"format"`
	Key      string `json:"key,omitempty"`
	Line     int    `json:"line,omitempty"`
	Column   int    `json:"column,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Expected string `json:"expected,omitempty"`
}

type CheckReport struct {
	Syntax                   []CheckIssue `json:"syntax"`
	KeyIncompatibility       []CheckIssue `json:"key_incompatibility"`
	TypeIncompatibility      []CheckIssue `json:"type_incompatibility"`
	StructureIncompatibility []CheckIssue `json:"structure_incompatibility"`
}

type FileCheckReport struct {
	Path   string      `json:"path"`
	Format string      `json:"format"`
	Errors CheckReport `json:"errors"`
}

type checkFileReports struct {
	Files []FileCheckReport `json:"files"`
}

var yamlLineColumnPattern = regexp.MustCompile(`line (\d+):(?: column (\d+):)?`)

func (report CheckReport) HasIssues() bool {
	return len(report.Syntax) > 0 ||
		len(report.KeyIncompatibility) > 0 ||
		len(report.TypeIncompatibility) > 0 ||
		len(report.StructureIncompatibility) > 0
}

func CheckJSON(input string) CheckReport {
	decoder := json.NewDecoder(strings.NewReader(input))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return CheckReport{Syntax: []CheckIssue{jsonSyntaxIssue(input, err)}}
	}

	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return CheckReport{Syntax: []CheckIssue{{Path: "$", Reason: "unexpected trailing content", Format: "json"}}}
	} else if !errors.Is(err, io.EOF) {
		return CheckReport{Syntax: []CheckIssue{jsonSyntaxIssue(input, err)}}
	}

	report := newCheckReport()
	record, ok := value.(map[string]any)
	if !ok {
		report.StructureIncompatibility = append(report.StructureIncompatibility, CheckIssue{
			Path:     "$",
			Reason:   "root value must be a record",
			Format:   "json",
			Actual:   importedValueTypeName(value),
			Expected: "record",
		})
		return report
	}

	checkRecordValue(record, "json", "$", &report)
	return report
}

func CheckYAML(input string) CheckReport {
	decoder := yaml.NewDecoder(strings.NewReader(input))
	report := newCheckReport()
	documents := []yaml.Node{}

	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			report.Syntax = append(report.Syntax, yamlSyntaxIssue(err))
			return report
		}
		documents = append(documents, document)
	}

	for index := range documents {
		path := "$"
		if len(documents) > 1 {
			path = objectCheckPath(path, fmt.Sprintf("document_%d", index+1))
		}
		visited := map[*yaml.Node]struct{}{}
		checkYAMLNode(&documents[index], path, &report, visited)
	}

	if len(documents) > 1 {
		report.StructureIncompatibility = append(report.StructureIncompatibility, CheckIssue{
			Path:     "$",
			Reason:   "multiple YAML documents require migration before direct Mace use",
			Format:   "yaml",
			Actual:   strconv.Itoa(len(documents)) + " documents",
			Expected: "single document",
		})
	}

	return report
}

func positionedCheckIssue(node *yaml.Node, issue CheckIssue) CheckIssue {
	if node == nil {
		return issue
	}
	if node.Line > 0 {
		issue.Line = node.Line
	}
	if node.Column > 0 {
		issue.Column = node.Column
	}
	return issue
}

func CheckTOML(input string) CheckReport {
	var value map[string]any
	if _, err := burnttoml.Decode(input, &value); err != nil {
		return CheckReport{Syntax: []CheckIssue{tomlSyntaxIssue(err)}}
	}

	report := newCheckReport()
	checkRecordValue(value, "toml", "$", &report)
	return report
}

func CheckFile(path string) (FileCheckReport, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return FileCheckReport{}, fmt.Errorf("read check file: %w", err)
	}

	format, err := checkFormat(path, contents)
	if err != nil {
		return FileCheckReport{}, err
	}

	report := FileCheckReport{Path: path, Format: format}
	switch format {
	case "json":
		report.Errors = CheckJSON(string(contents))
	case "yaml":
		report.Errors = CheckYAML(string(contents))
	case "toml":
		report.Errors = CheckTOML(string(contents))
	default:
		return FileCheckReport{}, fmt.Errorf("unsupported check format %q", format)
	}

	return report, nil
}

func FormatCheckReport(report CheckReport) (string, error) {
	return Marshal(report)
}

func FormatFileCheckReports(reports []FileCheckReport) (string, error) {
	return Marshal(checkFileReports{Files: reports})
}

func newCheckReport() CheckReport {
	return CheckReport{
		Syntax:                   []CheckIssue{},
		KeyIncompatibility:       []CheckIssue{},
		TypeIncompatibility:      []CheckIssue{},
		StructureIncompatibility: []CheckIssue{},
	}
}

func checkFormat(path string, contents []byte) (string, error) {
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".json":
		return "json", nil
	case ".yaml", ".yml":
		return "yaml", nil
	case ".toml":
		return "toml", nil
	}

	mime := http.DetectContentType(contents)
	if mime == "application/json" {
		return "json", nil
	}

	return "", fmt.Errorf("could not detect check format for %q; use .json, .yaml, .yml, or .toml", path)
}

func checkRecordValue(record map[string]any, format string, path string, report *CheckReport) {
	names := make([]string, 0, len(record))
	for name := range record {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		childPath := objectCheckPath(path, name)
		if !isMaceIdentifier(name) {
			report.KeyIncompatibility = append(report.KeyIncompatibility, CheckIssue{
				Path:   childPath,
				Reason: "key is not a valid Mace identifier",
				Format: format,
				Key:    name,
			})
		}
		checkValue(record[name], format, childPath, report)
	}
}

func checkValue(value any, format string, path string, report *CheckReport) {
	switch typed := value.(type) {
	case map[string]any:
		checkRecordValue(typed, format, path, report)
	case []any:
		for index, item := range typed {
			checkValue(item, format, indexCheckPath(path, index), report)
		}
	case map[any]any:
		keys := make([]string, 0, len(typed))
		byName := map[string]any{}
		for key, item := range typed {
			name := fmt.Sprint(key)
			keys = append(keys, name)
			byName[name] = item
		}
		sort.Strings(keys)
		for _, name := range keys {
			childPath := objectCheckPath(path, name)
			if !isMaceIdentifier(name) {
				report.KeyIncompatibility = append(report.KeyIncompatibility, CheckIssue{
					Path:   childPath,
					Reason: "key is not a valid Mace identifier",
					Format: format,
					Key:    name,
				})
			}
			checkValue(byName[name], format, childPath, report)
		}
	default:
		_ = typed
	}
}

func checkYAMLNode(node *yaml.Node, path string, report *CheckReport, visited map[*yaml.Node]struct{}) {
	if node == nil {
		return
	}
	if _, exists := visited[node]; exists {
		return
	}
	visited[node] = struct{}{}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			checkYAMLNode(child, path, report, visited)
		}
	case yaml.MappingNode:
		if tag := node.ShortTag(); tag != "!!map" && tag != "" {
			report.TypeIncompatibility = append(report.TypeIncompatibility, positionedCheckIssue(node, CheckIssue{
				Path:   path,
				Reason: "YAML mapping tag does not map directly to Mace records",
				Format: "yaml",
				Actual: tag,
			}))
		}
		for index := 0; index+1 < len(node.Content); index += 2 {
			keyNode := node.Content[index]
			valueNode := node.Content[index+1]
			childPath := path
			if keyNode.Value == "<<" || keyNode.ShortTag() == "!!merge" {
				childPath = objectCheckPath(path, keyNode.Value)
				checkYAMLNode(valueNode, childPath, report, visited)
				continue
			}
			if keyNode.Kind != yaml.ScalarNode || keyNode.ShortTag() != "!!str" {
				report.KeyIncompatibility = append(report.KeyIncompatibility, positionedCheckIssue(keyNode, CheckIssue{
					Path:   path,
					Reason: "mapping keys must be strings",
					Format: "yaml",
					Key:    keyNode.Value,
				}))
				checkYAMLNode(valueNode, path, report, visited)
				continue
			}
			childPath = objectCheckPath(path, keyNode.Value)
			if !isMaceIdentifier(keyNode.Value) {
				report.KeyIncompatibility = append(report.KeyIncompatibility, positionedCheckIssue(keyNode, CheckIssue{
					Path:   childPath,
					Reason: "key is not a valid Mace identifier",
					Format: "yaml",
					Key:    keyNode.Value,
				}))
			}
			checkYAMLNode(valueNode, childPath, report, visited)
		}
	case yaml.SequenceNode:
		if tag := node.ShortTag(); tag != "!!seq" && tag != "" {
			report.TypeIncompatibility = append(report.TypeIncompatibility, positionedCheckIssue(node, CheckIssue{
				Path:   path,
				Reason: "YAML sequence tag does not map directly to Mace arrays",
				Format: "yaml",
				Actual: tag,
			}))
		}
		for index, child := range node.Content {
			checkYAMLNode(child, indexCheckPath(path, index), report, visited)
		}
	case yaml.ScalarNode:
		tag := node.ShortTag()
		if isSupportedYAMLScalarTag(tag) {
			return
		}
		reason := "YAML scalar type does not map directly to a Mace scalar"
		if tag == "!!timestamp" {
			reason = "YAML timestamp values do not map directly to Mace scalars"
		}
		report.TypeIncompatibility = append(report.TypeIncompatibility, positionedCheckIssue(node, CheckIssue{
			Path:   path,
			Reason: reason,
			Format: "yaml",
			Actual: tag,
		}))
	case yaml.AliasNode:
		checkYAMLNode(node.Alias, path, report, visited)
	}
}

func isSupportedYAMLScalarTag(tag string) bool {
	switch tag {
	case "", "!!null", "!!str", "!!bool", "!!int", "!!float":
		return true
	default:
		return false
	}
}

func jsonSyntaxIssue(input string, err error) CheckIssue {
	issue := CheckIssue{Path: "$", Reason: err.Error(), Format: "json"}

	var syntaxError *json.SyntaxError
	if errors.As(err, &syntaxError) {
		issue.Line, issue.Column = lineColumnAtOffset(input, syntaxError.Offset)
		return issue
	}

	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		issue.Line, issue.Column = lineColumnAtOffset(input, typeError.Offset)
	}

	return issue
}

func yamlSyntaxIssue(err error) CheckIssue {
	issue := CheckIssue{Path: "$", Reason: err.Error(), Format: "yaml"}
	matches := yamlLineColumnPattern.FindStringSubmatch(err.Error())
	if len(matches) >= 2 {
		line, parseErr := strconv.Atoi(matches[1])
		if parseErr == nil {
			issue.Line = line
		}
	}
	if len(matches) >= 3 && matches[2] != "" {
		column, parseErr := strconv.Atoi(matches[2])
		if parseErr == nil {
			issue.Column = column
		}
	}
	return issue
}

func tomlSyntaxIssue(err error) CheckIssue {
	issue := CheckIssue{Path: "$", Reason: err.Error(), Format: "toml"}

	var parseError burnttoml.ParseError
	if errors.As(err, &parseError) {
		issue.Line = parseError.Position.Line
		issue.Column = parseError.Position.Col
	}

	return issue
}

func lineColumnAtOffset(input string, offset int64) (int, int) {
	if offset <= 0 {
		return 0, 0
	}

	line := 1
	column := 1
	for index, value := range input {
		if int64(index)+1 >= offset {
			return line, column
		}
		if value == '\n' {
			line++
			column = 1
			continue
		}
		column++
	}

	return line, column
}

func objectCheckPath(base string, name string) string {
	if isMaceIdentifier(name) {
		if base == "" {
			return "$." + name
		}
		return base + "." + name
	}
	return base + "[" + strconv.Quote(name) + "]"
}

func indexCheckPath(base string, index int) string {
	return fmt.Sprintf("%s[%d]", base, index)
}

func isMaceIdentifier(value string) bool {
	if value == "" {
		return false
	}
	if !isASCIILetter(value[0]) {
		return false
	}
	for index := 1; index < len(value); index++ {
		character := value[index]
		if !isASCIILetter(character) && !isASCIIDigit(character) && character != '_' {
			return false
		}
	}
	return true
}

func isASCIILetter(value byte) bool {
	return (value >= 'a' && value <= 'z') || (value >= 'A' && value <= 'Z')
}

func isASCIIDigit(value byte) bool {
	return value >= '0' && value <= '9'
}

func importedValueTypeName(value any) string {
	if value == nil {
		return "null"
	}
	switch value.(type) {
	case map[string]any:
		return "record"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	}

	kind := reflect.TypeOf(value).Kind()
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "number"
	default:
		return kind.String()
	}
}
