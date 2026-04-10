package processor

import (
	"fmt"
	"maps"
	"strconv"

	"github.com/louiss0/mace/internal/parser/ast"
)

type enumDefinition struct {
	Name        string
	BackingType valueType
	Members     []enumMember
	values      map[string]Value
}

type enumMember struct {
	Name  string
	Value Value
}

func enumDefinitionFromDeclaration(declaration ast.EnumDeclaration) (enumDefinition, error) {
	backingType, err := primitiveValueType(declaration.BackingType.Name)
	if err != nil || (backingType.kind != ValueString && backingType.kind != ValueInt) {
		return enumDefinition{}, validationErrorf("invalid enum backing type %q for enum %q", declaration.BackingType.Name, declaration.Name)
	}

	valuesByKey := map[string]Value{}
	memberNames := map[string]struct{}{}
	members := make([]enumMember, 0, len(declaration.Members))
	hasExplicitValues := false
	hasImplicitValues := false

	for index, member := range declaration.Members {
		if _, exists := memberNames[member.Name]; exists {
			return enumDefinition{}, validationErrorf("duplicate enum member %q in enum %q", member.Name, declaration.Name)
		}
		memberNames[member.Name] = struct{}{}

		value, err := enumMemberValue(declaration, member, backingType, index)
		if err != nil {
			return enumDefinition{}, err
		}

		if member.HasValue {
			hasExplicitValues = true
		} else {
			hasImplicitValues = true
		}

		key, ok := enumValueKey(value)
		if !ok {
			return enumDefinition{}, validationErrorf("invalid enum value for enum %q", declaration.Name)
		}
		if _, exists := valuesByKey[key]; exists {
			return enumDefinition{}, validationErrorf("duplicate enum value %s in enum %q", enumValueDisplay(value), declaration.Name)
		}
		valuesByKey[key] = value

		members = append(members, enumMember{Name: member.Name, Value: value})
	}

	if hasExplicitValues && hasImplicitValues {
		return enumDefinition{}, validationErrorf("enum %q mixes implicit and explicit member values", declaration.Name)
	}

	return enumDefinition{
		Name:        declaration.Name,
		BackingType: backingType,
		Members:     members,
		values:      valuesByKey,
	}, nil
}

func enumMemberValue(declaration ast.EnumDeclaration, member ast.EnumMember, backingType valueType, index int) (Value, error) {
	if !member.HasValue {
		switch backingType.kind {
		case ValueString:
			return Value{Kind: ValueString, String: member.Name}, nil
		case ValueInt:
			return Value{Kind: ValueInt, Int: int64(index)}, nil
		default:
			return Value{}, validationErrorf("invalid enum backing type %q for enum %q", declaration.BackingType.Name, declaration.Name)
		}
	}

	switch backingType.kind {
	case ValueString:
		literal, ok := member.Value.(ast.StringLiteral)
		if !ok {
			return Value{}, validationErrorf("enum member %q in enum %q must use a string literal", member.Name, declaration.Name)
		}
		return parseStaticString(literal.Lexeme)
	case ValueInt:
		literal, ok := member.Value.(ast.IntLiteral)
		if !ok {
			return Value{}, validationErrorf("enum member %q in enum %q must use an int literal", member.Name, declaration.Name)
		}
		return parseInt(literal.Lexeme)
	default:
		return Value{}, validationErrorf("invalid enum backing type %q for enum %q", declaration.BackingType.Name, declaration.Name)
	}
}

func enumValueKey(value Value) (string, bool) {
	switch value.Kind {
	case ValueString:
		return "string:" + value.String, true
	case ValueInt:
		return "int:" + strconv.FormatInt(value.Int, 10), true
	default:
		return "", false
	}
}

func enumValueDisplay(value Value) string {
	switch value.Kind {
	case ValueString:
		return fmt.Sprintf("%q", value.String)
	case ValueInt:
		return strconv.FormatInt(value.Int, 10)
	default:
		return "unknown"
	}
}

func (definition enumDefinition) Clone() enumDefinition {
	values := make(map[string]Value, len(definition.values))
	maps.Copy(values, definition.values)

	members := make([]enumMember, len(definition.Members))
	copy(members, definition.Members)

	return enumDefinition{
		Name:        definition.Name,
		BackingType: definition.BackingType,
		Members:     members,
		values:      values,
	}
}

func (definition enumDefinition) Rename(name string) enumDefinition {
	cloned := definition.Clone()
	cloned.Name = name
	return cloned
}

func (definition enumDefinition) ContainsValue(value Value) bool {
	key, ok := enumValueKey(value)
	if !ok {
		return false
	}

	_, exists := definition.values[key]
	return exists
}

func (definition enumDefinition) Member(name string) (enumMember, bool) {
	for _, member := range definition.Members {
		if member.Name == name {
			return member, true
		}
	}

	return enumMember{}, false
}

type enumRegistry struct {
	enums map[string]enumDefinition
}

func newEnumRegistry() *enumRegistry {
	return &enumRegistry{
		enums: map[string]enumDefinition{},
	}
}

func (registry *enumRegistry) Clone() *enumRegistry {
	cloned := newEnumRegistry()
	for name, definition := range registry.enums {
		cloned.enums[name] = definition.Clone()
	}

	return cloned
}

func (registry *enumRegistry) Add(name string, definition enumDefinition) {
	registry.enums[name] = definition.Rename(name)
}

func (registry *enumRegistry) Get(name string) (enumDefinition, bool) {
	definition, exists := registry.enums[name]
	return definition, exists
}
