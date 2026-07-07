package processor

import (
	"fmt"

	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Choices", func() {
	DescribeTable("accepts choice variants with primitive literal fallbacks",
		func(choiceType string, primitiveType string, presetValue string, fallbackValue string) {
			processor := New()
			_, err := processor.Process(wrapScriptWithOutput(fmt.Sprintf(`|===|
type Preset: %s;
type Value: variant[Preset, %s];
Value preset = %s;
Value fallback = %s;
|===|`, choiceType, primitiveType, presetValue, fallbackValue)))
			tAssert.NoError(err)
		},
		Entry("string preset with string fallback", `choice["approved"]`, "string", `"approved"`, `"custom"`),
		Entry("int preset with int fallback", `choice[1]`, "int", `1`, `2`),
		Entry("float preset with float fallback", `choice[1.5]`, "float", `1.5`, `2.5`),
		Entry("hex int preset with hex int fallback", `choice[0x1]`, "hex_int", `0x1`, `0x2`),
		Entry("hex float preset with hex float fallback", `choice[0x1.8]`, "hex_float", `0x1.8`, `0x2.8`),
		Entry("boolean preset with boolean fallback", `choice[true]`, "boolean", `true`, `false`),
	)

	DescribeTable("processes valid choice declarations",
		func(input string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(input, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("choice string literal", `|===|
 type Fruit: choice["Apple", "Strawberry"];
 Fruit result = "Apple";
|===|
[output = data]
{
  result: (result),
}`, expectedValue{kind: ValueString, string: "Apple"}),
		Entry("choice aliases can be mixed", `|===|
 type Environment: choice["dev", "prod"];
 type Numeric: choice[1, 2];
 type Mode: choice[Environment, Numeric, true];
 Mode result = 2;
|===|
[output = data]
{
  result: (result),
}`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("choice float members preserve precision", `|===|
 type Ratio: choice[1.04, 1.0];
 Ratio first = 1.04;
 Ratio second = 1.0;
|===|
[output = data]
{
  result: { first: (first), second: (second), },
}`, expectedValue{kind: ValueRecord, record: map[string]expectedValue{
			"first":  {kind: ValueFloat, float: 1.04},
			"second": {kind: ValueFloat, float: 1.0},
		}}),
	)

	DescribeTable("rejects invalid choice declarations and assignments",
		func(input string, message string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.ErrorContains(err, message)
		},
		Entry("unknown choice alias", wrapScriptWithOutput(`|===|
 type Fruit: choice[MissingChoice];
|===|`), "unknown choice member"),
		Entry("non-choice alias in choice members", wrapScriptWithOutput(`|===|
 type Name: string;
 type Fruit: choice[Name];
|===|`), "must resolve to a choice type"),
		Entry("value outside choice domain", `|===|
 type Fruit: choice["Apple", "Strawberry"];
 Fruit result = "Pear";
|===|
[output = data]
{
  result: (result),
}`, "type mismatch: expected choice[\"Apple\", \"Strawberry\"], got \"Pear\""),
		Entry("conditional branch outside choice domain", `|===|
 boolean enabled = true;
 type Fruit: choice["Apple", "Strawberry"];
 Fruit result = (enabled ? "Pear" : "Apple");
|===|
[output = data]
{
  result: (result),
}`, "type mismatch: expected choice[\"Apple\", \"Strawberry\"], got \"Pear\""),
	)

})

var _ = Describe("Choice type helpers", func() {
	It("compares and displays choice values by scalar keys", func() {
		valuesEqual := choiceValuesEqual
		valueKeys := choiceValueKeys
		typeName := choiceTypeName
		containsValue := choiceContainsValue
		left := []Value{
			{Kind: ValueString, String: "Ada"},
			{Kind: ValueInt, Int: 7},
			{Kind: ValueBoolean, Boolean: true},
		}
		right := []Value{
			{Kind: ValueBoolean, Boolean: true},
			{Kind: ValueString, String: "Ada"},
			{Kind: ValueInt, Int: 7},
		}

		tAssert.True(valuesEqual(left, right))
		tAssert.False(valuesEqual(left, right[:2]))
		tAssert.Equal([]string{"boolean:true", "int:7", "string:Ada"}, valueKeys(left))
		tAssert.Empty(valueKeys([]Value{{Kind: ValueRecord}}))
		tAssert.Equal(`choice["Ada", 7, true]`, typeName(left))
		tAssert.True(containsValue(left, Value{Kind: ValueString, String: "Ada"}))
		tAssert.False(containsValue(left, Value{Kind: ValueRecord}))
	})

	It("covers choice resolution branches", func() {
		types := newTypeRegistry()
		types.AddAlias("Fruit", ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Apple"`}, ast.StringLiteral{Lexeme: `"Pear"`}}})
		types.AddAlias("Loop", ast.NamedType{Name: "Loop"})
		types.AddAlias("LoopChoice", ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "LoopChoice"}}})
		types.AddAlias("Plain", ast.PrimitiveType{Name: "string"})

		resolved, err := resolveChoiceType(ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Fruit"}, ast.IntLiteral{Lexeme: "7"}}}, types)
		tAssert.NoError(err)
		tAssert.True(choiceContainsValue(resolved.choiceValues, Value{Kind: ValueString, String: "Apple"}))
		tAssert.True(choiceContainsValue(resolved.choiceValues, Value{Kind: ValueInt, Int: 7}))
		tAssert.NoError(err)

		_, err = resolveChoiceValues([]ast.Expression{ast.RecordLiteral{}}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.Identifier{Name: "Missing"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.Identifier{Name: "Loop"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceType(ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "LoopChoice"}}}, types)
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.Identifier{Name: "Plain"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.StringLiteral{Lexeme: `"unterminated`}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.IntLiteral{Lexeme: "not-an-int"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.FloatLiteral{Lexeme: "not-a-float"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.HexIntLiteral{Lexeme: "0xZZ"}, types, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveChoiceMemberValues(ast.HexFloatLiteral{Lexeme: "0x1.Z"}, types, map[string]struct{}{})
		tAssert.Error(err)
		tAssert.Contains(choiceSchemaMemberLabel(ast.RecordLiteral{}), "ast.RecordLiteral")
	})

	It("falls back to source labels for unresolved choice schema members", func() {
		typeNameForSchema := choiceTypeNameForSchema
		reference := ast.ChoiceType{Members: []ast.Expression{
			ast.Identifier{Name: "Shared"},
			ast.StringLiteral{Lexeme: `"Ada"`},
			ast.IntLiteral{Lexeme: "7"},
			ast.FloatLiteral{Lexeme: "1.5"},
			ast.HexIntLiteral{Lexeme: "0xFF"},
			ast.HexFloatLiteral{Lexeme: "0x2.8"},
			ast.BooleanLiteral{Value: false},
			ast.RecordLiteral{},
		}}

		name := typeNameForSchema(reference, newTypeRegistry())
		nilName := typeNameForSchema(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Solo"`}}}, nil)

		tAssert.Contains(name, "Shared")
		tAssert.Contains(nilName, "Solo")
		tAssert.Contains(name, `"Ada"`)
		tAssert.Contains(name, "7")
		tAssert.Contains(name, "1.5")
		tAssert.Contains(name, "0xFF")
		tAssert.Contains(name, "0x2.8")
		tAssert.Contains(name, "false")
		tAssert.Contains(name, "ast.RecordLiteral")
	})
})
