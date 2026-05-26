package processor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/louiss0/mace/internal/parser/ast"
)

func resolveChoiceType(reference ast.ChoiceType, types *typeRegistry) (valueType, error) {
	values, err := resolveChoiceValues(reference.Members, types, map[string]struct{}{})
	if err != nil {
		return valueType{}, err
	}
	return valueType{choiceValues: values}, nil
}

func resolveChoiceValues(members []ast.Expression, types *typeRegistry, seen map[string]struct{}) ([]Value, error) {
	values := []Value{}
	seenValues := map[string]struct{}{}

	for _, member := range members {
		resolved, err := resolveChoiceMemberValues(member, types, seen)
		if err != nil {
			return nil, err
		}
		for _, value := range resolved {
			key, ok := scalarValueKey(value)
			if !ok {
				return nil, validationErrorf("choice members must use scalar literals")
			}
			if _, exists := seenValues[key]; exists {
				continue
			}
			seenValues[key] = struct{}{}
			values = append(values, value)
		}
	}

	return values, nil
}

func resolveChoiceMemberValues(member ast.Expression, types *typeRegistry, seen map[string]struct{}) ([]Value, error) {
	switch typed := member.(type) {
	case ast.Identifier:
		if _, exists := seen[typed.Name]; exists {
			return nil, validationErrorf("cyclic choice alias %q", typed.Name)
		}

		resolved, ok, err := types.Resolve(typed.Name)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, validationErrorf("unknown choice member %q", typed.Name)
		}

		choice, ok := resolved.(ast.ChoiceType)
		if !ok {
			return nil, validationErrorf("choice member %q must resolve to a choice type", typed.Name)
		}

		nextSeen := map[string]struct{}{}
		for name := range seen {
			nextSeen[name] = struct{}{}
		}
		nextSeen[typed.Name] = struct{}{}
		return resolveChoiceValues(choice.Members, types, nextSeen)
	case ast.StringLiteral:
		value, err := parseStaticString(typed.Lexeme)
		if err != nil {
			return nil, err
		}
		return []Value{value}, nil
	case ast.IntLiteral:
		value, err := parseInt(typed.Lexeme)
		if err != nil {
			return nil, err
		}
		return []Value{value}, nil
	case ast.FloatLiteral:
		value, err := parseFloat(typed.Lexeme)
		if err != nil {
			return nil, err
		}
		return []Value{value}, nil
	case ast.HexIntLiteral:
		value, err := parseHexInt(typed.Lexeme)
		if err != nil {
			return nil, err
		}
		return []Value{value}, nil
	case ast.HexFloatLiteral:
		value, err := parseHexFloat(typed.Lexeme)
		if err != nil {
			return nil, err
		}
		return []Value{value}, nil
	case ast.BooleanLiteral:
		return []Value{{Kind: ValueBoolean, Boolean: typed.Value}}, nil
	default:
		return nil, validationErrorf("choice members must be literals or choice names")
	}
}

func choiceContainsValue(values []Value, value Value) bool {
	key, ok := scalarValueKey(value)
	if !ok {
		return false
	}
	for _, candidate := range values {
		candidateKey, ok := scalarValueKey(candidate)
		if ok && candidateKey == key {
			return true
		}
	}
	return false
}

func choiceValuesEqual(left []Value, right []Value) bool {
	if len(left) != len(right) {
		return false
	}
	leftKeys := choiceValueKeys(left)
	rightKeys := choiceValueKeys(right)
	for index := range leftKeys {
		if leftKeys[index] != rightKeys[index] {
			return false
		}
	}
	return true
}

func choiceValueKeys(values []Value) []string {
	keys := make([]string, 0, len(values))
	for _, value := range values {
		key, ok := scalarValueKey(value)
		if ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func choiceTypeNameForSchema(reference ast.ChoiceType) string {
	values, err := resolveChoiceValues(reference.Members, newTypeRegistry(), map[string]struct{}{})
	if err != nil {
		parts := make([]string, 0, len(reference.Members))
		for _, member := range reference.Members {
			parts = append(parts, fmt.Sprintf("%T", member))
		}
		return fmt.Sprintf("choice[%s]", strings.Join(parts, ", "))
	}
	return choiceTypeName(values)
}

func choiceTypeName(values []Value) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, scalarValueDisplay(value))
	}
	return fmt.Sprintf("choice[%s]", strings.Join(parts, ", "))
}

func valueTypeFromChoiceValue(value Value) valueType {
	return valueType{kind: value.Kind}
}
