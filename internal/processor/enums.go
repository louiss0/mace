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
	if err != nil || (backingType.kind != ValueString && backingType.kind != ValueInt && backingType.kind != ValueFloat && backingType.kind != ValueHexInt && backingType.kind != ValueHexFloat) {
		return enumDefinition{}, enumError(CodeInvalidEnumBackingType, DiagnosticFields{Name: declaration.Name, Actual: declaration.BackingType.Name}, "invalid enum backing type %q for enum %q", declaration.BackingType.Name, declaration.Name)
	}

	valuesByKey := map[string]Value{}
	memberNames := map[string]struct{}{}
	members := make([]enumMember, 0, len(declaration.Members))
	hasExplicitValues := false
	hasImplicitValues := false

	for index, member := range declaration.Members {
		if _, exists := memberNames[member.Name]; exists {
			return enumDefinition{}, enumError(CodeDuplicateEnumMember, DiagnosticFields{Name: member.Name, Schema: declaration.Name}, "duplicate enum member %q in enum %q", member.Name, declaration.Name)
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
			return enumDefinition{}, enumError(CodeInvalidEnumValue, DiagnosticFields{Schema: declaration.Name}, "invalid enum value for enum %q", declaration.Name)
		}
		if _, exists := valuesByKey[key]; exists {
			return enumDefinition{}, enumError(CodeDuplicateEnumValue, DiagnosticFields{Schema: declaration.Name}, "duplicate enum value %s in enum %q", enumValueDisplay(value), declaration.Name)
		}
		valuesByKey[key] = value

		members = append(members, enumMember{Name: member.Name, Value: value})
	}

	if hasExplicitValues && hasImplicitValues {
		return enumDefinition{}, enumError(CodeEnumMixedValues, DiagnosticFields{Name: declaration.Name}, "enum %q mixes implicit and explicit member values", declaration.Name)
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
		case ValueFloat:
			return Value{Kind: ValueFloat, Float: float64(index) / 10}, nil
		case ValueHexInt, ValueHexFloat:
			return Value{}, enumError(CodeEnumRequiresExplicitValues, DiagnosticFields{Name: declaration.Name, Expected: declaration.BackingType.Name}, "enum %q requires explicit member values for %s backing", declaration.Name, declaration.BackingType.Name)
		default:
			return Value{}, enumError(CodeInvalidEnumBackingType, DiagnosticFields{Name: declaration.Name, Actual: declaration.BackingType.Name}, "invalid enum backing type %q for enum %q", declaration.BackingType.Name, declaration.Name)
		}
	}

	switch backingType.kind {
	case ValueString:
		literal, ok := member.Value.(ast.StringLiteral)
		if !ok {
			return Value{}, enumError(CodeEnumMemberValueType, DiagnosticFields{Name: member.Name, Schema: declaration.Name, Expected: "string"}, "enum member %q in enum %q must use a string literal", member.Name, declaration.Name)
		}
		return parseStaticString(literal.Lexeme)
	case ValueInt:
		literal, ok := member.Value.(ast.IntLiteral)
		if !ok {
			return Value{}, enumError(CodeEnumMemberValueType, DiagnosticFields{Name: member.Name, Schema: declaration.Name, Expected: "int"}, "enum member %q in enum %q must use an int literal", member.Name, declaration.Name)
		}
		return parseInt(literal.Lexeme)
	case ValueFloat:
		literal, ok := member.Value.(ast.FloatLiteral)
		if !ok {
			return Value{}, enumError(CodeEnumMemberValueType, DiagnosticFields{Name: member.Name, Schema: declaration.Name, Expected: "float"}, "enum member %q in enum %q must use a float literal", member.Name, declaration.Name)
		}
		return parseFloat(literal.Lexeme)
	case ValueHexInt:
		literal, ok := member.Value.(ast.HexIntLiteral)
		if !ok {
			return Value{}, enumError(CodeEnumMemberValueType, DiagnosticFields{Name: member.Name, Schema: declaration.Name, Expected: "hex_int"}, "enum member %q in enum %q must use a hex_int literal", member.Name, declaration.Name)
		}
		return parseHexInt(literal.Lexeme)
	case ValueHexFloat:
		literal, ok := member.Value.(ast.HexFloatLiteral)
		if !ok {
			return Value{}, enumError(CodeEnumMemberValueType, DiagnosticFields{Name: member.Name, Schema: declaration.Name, Expected: "hex_float"}, "enum member %q in enum %q must use a hex_float literal", member.Name, declaration.Name)
		}
		return parseHexFloat(literal.Lexeme)
	default:
		return Value{}, enumError(CodeInvalidEnumBackingType, DiagnosticFields{Name: declaration.Name, Actual: declaration.BackingType.Name}, "invalid enum backing type %q for enum %q", declaration.BackingType.Name, declaration.Name)
	}
}

func enumFloatStep(value float64) int64 {
	return int64(value*10 + 0.5)
}

func enumValueKey(value Value) (string, bool) {
	switch value.Kind {
	case ValueString:
		return "string:" + value.String, true
	case ValueInt:
		return "int:" + strconv.FormatInt(value.Int, 10), true
	case ValueFloat:
		return "float:" + strconv.FormatFloat(value.Float, 'f', 1, 64), true
	case ValueHexInt:
		return "hex_int:" + formatHexInt(value.Int), true
	case ValueHexFloat:
		return "hex_float:" + formatHexFloat(value.Float), true
	case ValueBoolean:
		return "boolean:" + strconv.FormatBool(value.Boolean), true
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
	case ValueFloat:
		return strconv.FormatFloat(value.Float, 'f', 1, 64)
	case ValueHexInt:
		return formatHexInt(value.Int)
	case ValueHexFloat:
		return formatHexFloat(value.Float)
	case ValueBoolean:
		return strconv.FormatBool(value.Boolean)
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

func mergeEnumDefinitions(name string, definitions []enumDefinition) (enumDefinition, error) {
	if len(definitions) == 0 {
		return enumDefinition{}, validationErrorf("union members must be schemas or enums")
	}

	backingType := definitions[0].BackingType
	members := []enumMember{}
	memberIndexes := map[string]int{}
	valuesByKey := map[string]Value{}
	usedIntValues := map[int64]struct{}{}
	usedFloatValues := map[int64]struct{}{}
	nextIntValue := int64(0)
	nextFloatStep := int64(0)

	for _, definition := range definitions {
		if definition.BackingType.kind != backingType.kind {
			return enumDefinition{}, validationErrorf("enum unions require the same backing type")
		}

		for _, member := range definition.Members {
			value := member.Value
			if index, exists := memberIndexes[member.Name]; exists {
				oldValueKey, ok := enumValueKey(members[index].Value)
				if ok {
					delete(valuesByKey, oldValueKey)
				}
			}
			if backingType.kind == ValueInt || backingType.kind == ValueHexInt {
				for {
					if _, exists := usedIntValues[value.Int]; !exists {
						break
					}
					value = Value{Kind: ValueInt, Int: nextIntValue}
					nextIntValue++
				}
				usedIntValues[value.Int] = struct{}{}
				if value.Int >= nextIntValue {
					nextIntValue = value.Int + 1
				}
				if backingType.kind == ValueHexInt {
					value.Kind = ValueHexInt
				}
			} else if backingType.kind == ValueFloat || backingType.kind == ValueHexFloat {
				step := enumFloatStep(value.Float)
				for {
					if _, exists := usedFloatValues[step]; !exists {
						break
					}
					step = nextFloatStep
					nextFloatStep++
				}
				value = Value{Kind: ValueFloat, Float: float64(step) / 10}
				if backingType.kind == ValueHexFloat {
					value.Kind = ValueHexFloat
				}
				usedFloatValues[step] = struct{}{}
				if step >= nextFloatStep {
					nextFloatStep = step + 1
				}
			} else {
				key, ok := enumValueKey(value)
				if !ok {
					return enumDefinition{}, enumError(CodeInvalidEnumValue, DiagnosticFields{Schema: definition.Name}, "invalid enum value for enum %q", definition.Name)
				}
				if _, exists := valuesByKey[key]; exists {
					value = Value{Kind: ValueString, String: member.Name}
				}
			}

			if index, exists := memberIndexes[member.Name]; exists {
				members[index] = enumMember{Name: member.Name, Value: value}
			} else {
				memberIndexes[member.Name] = len(members)
				members = append(members, enumMember{Name: member.Name, Value: value})
			}

			key, ok := enumValueKey(value)
			if !ok {
				return enumDefinition{}, enumError(CodeInvalidEnumValue, DiagnosticFields{Schema: definition.Name}, "invalid enum value for enum %q", definition.Name)
			}
			valuesByKey[key] = value
		}
	}

	return enumDefinition{Name: name, BackingType: backingType, Members: members, values: valuesByKey}, nil
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
