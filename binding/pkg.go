package binding

import (
	"fmt"
	"go/format"
	"slices"
	"strings"

	"github.com/louiss0/mace/parser/ast"
	"github.com/louiss0/mace/processor"
)

func OutputMap(result processor.Result) map[string]any {
	return valuesToMap(result.Output)
}

func GenerateStructs(file ast.File, packageName string) (string, error) {
	if packageName == "" {
		return "", fmt.Errorf("generate structs: package name is required")
	}

	typeAliases, schemas := collectTypes(file)
	if len(schemas) == 0 {
		return "", fmt.Errorf("generate structs: no schemas found")
	}

	builder := strings.Builder{}
	builder.WriteString("package ")
	builder.WriteString(packageName)
	builder.WriteString("\n\n")

	schemaNames := make([]string, 0, len(schemas))
	for name := range schemas {
		schemaNames = append(schemaNames, name)
	}
	slices.Sort(schemaNames)

	for index, name := range schemaNames {
		structSource, err := generateStruct(name, schemas[name], typeAliases, schemas)
		if err != nil {
			return "", err
		}

		if index > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(structSource)
	}

	formatted, err := format.Source([]byte(builder.String()))
	if err != nil {
		return "", fmt.Errorf("generate structs: format source: %w", err)
	}

	return string(formatted), nil
}

func valuesToMap(values map[string]processor.Value) map[string]any {
	output := make(map[string]any, len(values))
	for name, value := range values {
		output[name] = valueToAny(value)
	}

	return output
}

func valueToAny(value processor.Value) any {
	switch value.Kind {
	case processor.ValueString:
		return value.String
	case processor.ValueInt:
		return value.Int
	case processor.ValueFloat:
		return value.Float
	case processor.ValueBoolean:
		return value.Boolean
	case processor.ValueArray:
		items := make([]any, 0, len(value.Array))
		for _, item := range value.Array {
			items = append(items, valueToAny(item))
		}
		return items
	case processor.ValueRecord:
		return valuesToMap(value.Record)
	default:
		return nil
	}
}

func collectTypes(file ast.File) (map[string]ast.TypeReference, map[string]ast.RecordType) {
	typeAliases := map[string]ast.TypeReference{}
	schemas := map[string]ast.RecordType{}

	if file.Script == nil {
		return typeAliases, schemas
	}

	for _, declaration := range file.Script.Items {
		switch typedDeclaration := declaration.(type) {
		case ast.TypeDeclaration:
			typeAliases[typedDeclaration.Name] = typedDeclaration.Type
		case ast.SchemaDeclaration:
			schemas[typedDeclaration.Name] = typedDeclaration.Type
		}
	}

	return typeAliases, schemas
}

func generateStruct(name string, recordType ast.RecordType, typeAliases map[string]ast.TypeReference, schemas map[string]ast.RecordType) (string, error) {
	builder := strings.Builder{}
	builder.WriteString("type ")
	builder.WriteString(exportName(name))
	builder.WriteString(" struct {\n")

	for _, field := range recordType.Fields {
		fieldType, err := goType(field.Type, typeAliases, schemas, map[string]struct{}{})
		if err != nil {
			return "", err
		}
		if field.Optional {
			fieldType = "*" + fieldType
		}

		builder.WriteString("\t")
		builder.WriteString(exportName(field.Name))
		builder.WriteString(" ")
		builder.WriteString(fieldType)
		builder.WriteString(" `json:\"")
		builder.WriteString(field.Name)
		if field.Optional {
			builder.WriteString(",omitempty")
		}
		builder.WriteString("\"`\n")
	}

	builder.WriteString("}")
	return builder.String(), nil
}

func goType(typeReference ast.TypeReference, typeAliases map[string]ast.TypeReference, schemas map[string]ast.RecordType, seen map[string]struct{}) (string, error) {
	switch typedReference := typeReference.(type) {
	case ast.PrimitiveType:
		switch typedReference.Name {
		case "string":
			return "string", nil
		case "int":
			return "int64", nil
		case "float":
			return "float64", nil
		case "boolean":
			return "bool", nil
		default:
			return "", fmt.Errorf("generate structs: unknown primitive type %q", typedReference.Name)
		}
	case ast.ArrayType:
		elementType, err := goType(typedReference.Element, typeAliases, schemas, seen)
		if err != nil {
			return "", err
		}
		return "[]" + elementType, nil
	case ast.NamedType:
		if _, ok := schemas[typedReference.Name]; ok {
			return exportName(typedReference.Name), nil
		}

		alias, ok := typeAliases[typedReference.Name]
		if !ok {
			return exportName(typedReference.Name), nil
		}
		if _, ok := seen[typedReference.Name]; ok {
			return "", fmt.Errorf("generate structs: cyclic type alias %q", typedReference.Name)
		}

		nextSeen := mapsClone(seen)
		nextSeen[typedReference.Name] = struct{}{}
		return goType(alias, typeAliases, schemas, nextSeen)
	default:
		return "", fmt.Errorf("generate structs: unsupported type reference %T", typeReference)
	}
}

func exportName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return ""
	}

	builder := strings.Builder{}
	for _, part := range parts {
		if part == "" {
			continue
		}
		builder.WriteString(strings.ToUpper(part[:1]))
		if len(part) > 1 {
			builder.WriteString(part[1:])
		}
	}

	return builder.String()
}

func mapsClone(source map[string]struct{}) map[string]struct{} {
	target := make(map[string]struct{}, len(source))
	for key, value := range source {
		target[key] = value
	}
	return target
}
