package processor

import (
	"math"
	"strings"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Operators", func() {
	It("attaches operator ranges to operand type errors", func() {
		_, err := New().Process(`|===|
boolean value = true + false;
|===|
[output = 'data'] { result: value, }`)
		tAssert.Error(err)

		var diagnostic DiagnosticError
		if tAssert.ErrorAs(err, &diagnostic) {
			tAssert.Equal(2, diagnostic.Range.Start.Line)
			tAssert.Equal(22, diagnostic.Range.Start.Column)
			tAssert.Equal(2, diagnostic.Range.End.Line)
			tAssert.Equal(23, diagnostic.Range.End.Column)
		}
	})

	DescribeTable("returns individual operator results",
		func(input string, expected expectedValue) {
			assertProcessedResult(input, expected)
		},
		Entry("unary plus", `[output = 'data'] { result: +7, }`, expectedValue{kind: ValueInt, int64: 7}),
		Entry("unary minus", `[output = 'data'] { result: -5, }`, expectedValue{kind: ValueInt, int64: -5}),
		Entry("logical not", `[output = 'data'] { result: !false, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise not", `[output = 'data'] { result: ~1, }`, expectedValue{kind: ValueInt, int64: ^int64(1)}),
		Entry("hex unary minus", `[output = 'data'] { result: -0xA, }`, expectedValue{kind: ValueHexInt, string: "-0xA"}),
		Entry("addition", `[output = 'data'] { result: 1 + 2, }`, expectedValue{kind: ValueInt, int64: 3}),
		Entry("hex addition", `[output = 'data'] { result: 0x0F + 0x01, }`, expectedValue{kind: ValueHexInt, string: "0x10"}),
		Entry("subtraction", `[output = 'data'] { result: 5 - 3, }`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("multiplication", `[output = 'data'] { result: 2 * 3, }`, expectedValue{kind: ValueInt, int64: 6}),
		Entry("division", `[output = 'data'] { result: 8 / 2, }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("hex division", `[output = 'data'] { result: 0x05 / 0x02, }`, expectedValue{kind: ValueHexFloat, string: "0x2.8"}),
		Entry("modulo", `[output = 'data'] { result: 9 % 4, }`, expectedValue{kind: ValueInt, int64: 1}),
		Entry("hex modulo", `[output = 'data'] { result: 0x05 % 0x02, }`, expectedValue{kind: ValueHexInt, string: "0x1"}),
		Entry("mixed modulo", `[output = 'data'] { result: 9 % 2.5, }`, expectedValue{kind: ValueFloat, float: 1.5}),
		Entry("exponentiation", `[output = 'data'] { result: 2 ** 3, }`, expectedValue{kind: ValueInt, int64: 8}),
		Entry("shift left", `[output = 'data'] { result: 1 << 3, }`, expectedValue{kind: ValueInt, int64: 8}),
		Entry("shift right", `[output = 'data'] { result: 8 >> 1, }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("unsigned shift right", `[output = 'data'] { result: 8 >>> 1, }`, expectedValue{kind: ValueInt, int64: 4}),
		Entry("hex shift left", `[output = 'data'] { result: 0x01 << 0x04, }`, expectedValue{kind: ValueHexInt, string: "0x10"}),
		Entry("less than", `[output = 'data'] { result: 1 < 2, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("hex greater than", `[output = 'data'] { result: 0x10 > 0x0F, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("less than or equal", `[output = 'data'] { result: 2 <= 2, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("greater than", `[output = 'data'] { result: 3 > 2, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("greater than or equal", `[output = 'data'] { result: 2 >= 2, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("equal", `[output = 'data'] { result: 3 == 3, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("not equal", `[output = 'data'] { result: 3 != 4, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise and", `[output = 'data'] { result: 6 & 3, }`, expectedValue{kind: ValueInt, int64: 2}),
		Entry("bitwise xor", `[output = 'data'] { result: 5 ^ 3, }`, expectedValue{kind: ValueInt, int64: 6}),
		Entry("bitwise or", `[output = 'data'] { result: 5 | 2, }`, expectedValue{kind: ValueInt, int64: 7}),
		Entry("hex bitwise or", `[output = 'data'] { result: 0x0F | 0x10, }`, expectedValue{kind: ValueHexInt, string: "0x1F"}),
		Entry("logical and", `[output = 'data'] { result: true && false, }`, expectedValue{kind: ValueBoolean, bool: false}),
		Entry("logical or", `[output = 'data'] { result: true || false, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("ternary", `[output = 'data'] { result: true ? 1 : 2, }`, expectedValue{kind: ValueInt, int64: 1}),
		Entry("arithmetic precedence", `[output = 'data'] { result: 1 + 2 * 3 - 4, }`, expectedValue{kind: ValueInt, int64: 3}),
		Entry("shift and additive precedence", `[output = 'data'] { result: 1 + 2 << 2, }`, expectedValue{kind: ValueInt, int64: 12}),
		Entry("bitwise precedence", `[output = 'data'] { result: 7 & 3 ^ 1 | 8, }`, expectedValue{kind: ValueInt, int64: 10}),
		Entry("comparison and logic precedence", `[output = 'data'] { result: 1 < 2 && 3 > 2 || false, }`, expectedValue{kind: ValueBoolean, bool: true}),
		Entry("conditional with logical expression", `[output = 'data'] { result: false || true ? 5 : 2, }`, expectedValue{kind: ValueInt, int64: 5}),
	)

	DescribeTable("rejects overflowing hex_int operations",
		func(expression string) {
			_, err := New().Process(`[output = 'data'] { result: ` + expression + `, }`)
			tAssert.ErrorContains(err, "hex_int overflow")
		},
		Entry("addition", "0x7FFFFFFFFFFFFFFF + 0x1"),
		Entry("subtraction", "-0x8000000000000000 - 0x1"),
		Entry("multiplication", "0x4000000000000000 * 0x2"),
		Entry("exponentiation", "0x2 ** 0x3F"),
		Entry("left shift", "0x4000000000000000 << 0x2"),
	)

	It("accepts the signed hex_int boundaries", func() {
		result, err := New().Process(`[output = 'data'] {
  maximum: 0x7FFFFFFFFFFFFFFF,
  minimum: -0x8000000000000000,
}`)
		tAssert.NoError(err)
		tAssert.Equal(int64(math.MaxInt64), result.Output["maximum"].Int)
		tAssert.Equal(int64(math.MinInt64), result.Output["minimum"].Int)
	})

	DescribeTable("rejects hex_int literals outside the signed range",
		func(literal string) {
			_, err := New().Process(`[output = 'data'] { result: ` + literal + `, }`)
			tAssert.ErrorContains(err, "invalid hex_int literal")
		},
		Entry("positive minimum magnitude", "0x8000000000000000"),
		Entry("larger positive magnitude", "0x10000000000000000"),
		Entry("negative beyond minimum", "-0x8000000000000001"),
	)

	It("parses and round-trips finite fixed-point hex_float values", func() {
		maximumLiteral := "0x" + strings.Repeat("F", 256) + ".0"
		result, err := New().Process(`[output = 'data'] {
  ordinary: 0x1.8,
  uppercase: 0xA.F,
  precise: 0x1.0000000000001,
  large: 0x10000000000000000.0,
  maximum: ` + maximumLiteral + `,
  negative_maximum: -` + maximumLiteral + `,
}`)
		tAssert.NoError(err)
		tAssert.Equal(1.5, result.Output["ordinary"].Float)
		tAssert.Equal(10.9375, result.Output["uppercase"].Float)
		tAssert.Equal(math.Float64bits(1+math.Ldexp(1, -52)), math.Float64bits(result.Output["precise"].Float))
		tAssert.Equal(math.Float64bits(math.Ldexp(1, 64)), math.Float64bits(result.Output["large"].Float))
		tAssert.Equal(math.Float64bits(math.MaxFloat64), math.Float64bits(result.Output["maximum"].Float))
		tAssert.Equal(math.Float64bits(-math.MaxFloat64), math.Float64bits(result.Output["negative_maximum"].Float))

		for _, name := range []string{"precise", "large", "maximum", "negative_maximum"} {
			formatted, formatErr := FormatScalarValue(result.Output[name])
			tAssert.NoError(formatErr)
			roundTrip, parseErr := New().Process(`[output = 'data'] { result: ` + formatted + `, }`)
			tAssert.NoError(parseErr)
			tAssert.Equal(math.Float64bits(result.Output[name].Float), math.Float64bits(roundTrip.Output["result"].Float))
		}
	})

	It("rejects a hex_float literal that rounds to infinity", func() {
		literal := "0x1" + strings.Repeat("0", 256) + ".0"
		_, err := New().Process(`[output = 'data'] { result: ` + literal + `, }`)
		tAssert.ErrorContains(err, "invalid hex_float literal")
	})

	DescribeTable("rejects non-finite hex_float arithmetic",
		func(operator lexer.TokenType, left float64, right float64) {
			_, err := evaluateHexNumeric(operator, Value{Kind: ValueHexFloat, Float: left}, Value{Kind: ValueHexFloat, Float: right})
			tAssert.ErrorContains(err, "non-finite hex_float result")
		},
		Entry("addition", lexer.TokenPlus, math.MaxFloat64, math.MaxFloat64),
		Entry("subtraction", lexer.TokenMinus, math.MaxFloat64, -math.MaxFloat64),
		Entry("multiplication", lexer.TokenStar, math.MaxFloat64, 2.0),
		Entry("division", lexer.TokenSlash, math.MaxFloat64, math.SmallestNonzeroFloat64),
		Entry("exponentiation", lexer.TokenDoubleStar, math.MaxFloat64, 2.0),
	)

	It("preserves negative zero as a hex_float", func() {
		result, err := New().Process(`[output = 'data'] { result: -0x0.0, }`)
		tAssert.NoError(err)
		tAssert.True(math.Signbit(result.Output["result"].Float))
		formatted, err := FormatScalarValue(result.Output["result"])
		tAssert.NoError(err)
		tAssert.Equal("-0x0.0", formatted)
	})

	It("round-trips representative finite float64 bit patterns", func() {
		values := []float64{
			0,
			math.Copysign(0, -1),
			math.SmallestNonzeroFloat64,
			-math.SmallestNonzeroFloat64,
			math.SmallestNonzeroFloat64 * 17,
			math.Pi,
			math.Nextafter(1, 2),
			math.MaxFloat64,
			-math.MaxFloat64,
		}

		for _, value := range values {
			formatted, err := FormatScalarValue(Value{Kind: ValueHexFloat, Float: value})
			tAssert.NoError(err)
			result, err := New().Process(`[output = 'data'] { result: ` + formatted + `, }`)
			tAssert.NoError(err)
			tAssert.Equal(math.Float64bits(value), math.Float64bits(result.Output["result"].Float))
		}
	})

	DescribeTable("returns math results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("addition and multiplication", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 * 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("subtraction and division", wrapScriptWithOutputFields(`|===|
int result = 20 - 4 / 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 18}),
		Entry("modulo", wrapScriptWithOutputFields(`|===|
int result = 9 % 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("exponentiation", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 8}),
		Entry("unary minus", wrapScriptWithOutputFields(`|===|
int result = -5;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: -5}),
		Entry("unary plus", wrapScriptWithOutputFields(`|===|
int result = +7;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 7}),
		Entry("float arithmetic", wrapScriptWithOutputFields(`|===|
float result = 1.5 + 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 4.0}),
		Entry("float division", wrapScriptWithOutputFields(`|===|
float result = 7.5 / 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 3.0}),
		Entry("mixed numeric addition", wrapScriptWithOutputFields(`|===|
float result = 1 + 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 3.5}),
		Entry("mixed numeric exponentiation", wrapScriptWithOutputFields(`|===|
float result = 2 ** 3.0;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 8.0}),
		Entry("mixed numeric modulo", wrapScriptWithOutputFields(`|===|
float result = 5 % 2.5;
|===|`, "result: result;"), expectedValue{kind: ValueFloat, float: 0.0}),
	)

	It("evaluates typed numeric variables without output wrappers", func() {
		processor := New()
		result, err := processor.ProcessInDir(`|===|
int int_value = 1 + 2 * 3;
float float_value = 1.5 + 2.5;
hex_int hex_int_value = 0x10 + 0x1;
hex_float hex_float_value = 0x1.8 + 0x0.8;
|===|
[output = 'data']
{
  int_value,
  float_value,
  hex_int_value,
  hex_float_value,
}`, "../..")
		tAssert.NoError(err)

		assertExpectedOutput(result, map[string]expectedValue{
			"int_value":       {kind: ValueInt, int64: 7},
			"float_value":     {kind: ValueFloat, float: 4},
			"hex_int_value":   {kind: ValueHexInt, string: "0x11"},
			"hex_float_value": {kind: ValueHexFloat, string: "0x2.0"},
		})
	})

	DescribeTable("returns operator precedence results",
		func(file string, expected expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			actual := requireOutputValue(result, "result")
			assertExpectedValue(actual, expected)
		},
		Entry("unary before exponent", wrapScriptWithOutputFields(`|===|
int result = -2 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 4}),
		Entry("exponent is right associative", wrapScriptWithOutputFields(`|===|
int result = 2 ** 3 ** 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 512}),
		Entry("shift after additive", wrapScriptWithOutputFields(`|===|
int result = 1 + 2 << 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 12}),
		Entry("relational after shift", wrapScriptWithOutputFields(`|===|
boolean result = 1 << 2 > 3;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("equality after relational", wrapScriptWithOutputFields(`|===|
boolean result = 1 < 2 == true;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("bitwise and before or", wrapScriptWithOutputFields(`|===|
int result = 1 | 2 & 4;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 1}),
		Entry("bitwise and before xor", wrapScriptWithOutputFields(`|===|
int result = 5 ^ 2 & 1;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
		Entry("logical and before or", wrapScriptWithOutputFields(`|===|
boolean result = true || false && false;
|===|`, "result: result;"), expectedValue{kind: ValueBoolean, bool: true}),
		Entry("conditional after logical or", wrapScriptWithOutputFields(`|===|
int result = false || true ? 5 : 2;
|===|`, "result: result;"), expectedValue{kind: ValueInt, int64: 5}),
	)

	DescribeTable("accepts non-math operators in script variables",
		func(file string, expected map[string]expectedValue) {
			processor := New()
			result, err := processor.ProcessInDir(file, "../..")
			tAssert.NoError(err)

			assertExpectedOutput(result, expected)
		},
		Entry("bitwise operators", wrapScriptWithOutputFields(`|===|
int masked = 6 & 3;
int combined = 5 | 2;
int toggled = 5 ^ 3;
int inverted = ~1;
|===|`, "masked: masked;\ncombined: combined;\ntoggled: toggled;\ninverted: inverted;"), map[string]expectedValue{
			"masked":   {kind: ValueInt, int64: 2},
			"combined": {kind: ValueInt, int64: 7},
			"toggled":  {kind: ValueInt, int64: 6},
			"inverted": {kind: ValueInt, int64: ^int64(1)},
		}),
		Entry("shift operators", wrapScriptWithOutputFields(`|===|
int left = 1 << 3;
int right = 8 >> 1;
int logical = 8 >>> 1;
|===|`, "left: left;\nright: right;\nlogical: logical;"), map[string]expectedValue{
			"left":    {kind: ValueInt, int64: 8},
			"right":   {kind: ValueInt, int64: 4},
			"logical": {kind: ValueInt, int64: 4},
		}),
		Entry("comparisons", wrapScriptWithOutputFields(`|===|
boolean less = 3 < 5;
boolean greater = 5 > 3;
|===|`, "less: less;\ngreater: greater;"), map[string]expectedValue{
			"less":    {kind: ValueBoolean, bool: true},
			"greater": {kind: ValueBoolean, bool: true},
		}),
		Entry("equality operators", wrapScriptWithOutputFields(`|===|
boolean equal = 3 == 3;
boolean not_equal = 3 != 4;
|===|`, "equal: equal;\nnot_equal: not_equal;"), map[string]expectedValue{
			"equal":     {kind: ValueBoolean, bool: true},
			"not_equal": {kind: ValueBoolean, bool: true},
		}),
		Entry("logical operators", wrapScriptWithOutputFields(`|===|
boolean result = true && false || true;
boolean not = !false;
|===|`, "result: result;\nnot: not;"), map[string]expectedValue{
			"result": {kind: ValueBoolean, bool: true},
			"not":    {kind: ValueBoolean, bool: true},
		}),
		Entry("ternary operator", wrapScriptWithOutputFields(`|===|
int value = true ? 1 : 2;
|===|`, "value: value;"), map[string]expectedValue{
			"value": {kind: ValueInt, int64: 1},
		}),
	)
})

var _ = Describe("Operator helpers", func() {
	It("covers evaluation branches", func() {
		vars := newValueEnvironment()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		vars.Add("name", Value{Kind: ValueString, String: "Ada"})
		symbols.Add("name", symbolKindVariable)
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})

		_, err := evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.BooleanLiteral{Value: true}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.BooleanLiteral{Value: true}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.BooleanLiteral{Value: true}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: false}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenQuestion, Right: ast.BooleanLiteral{Value: false}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)

		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.StringLiteral{Lexeme: `"a"`}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.StringLiteral{Lexeme: `"a"`}, Right: ast.StringLiteral{Lexeme: `"a"`}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)

		_, err = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		tAssert.Error(err)
		_, err = evaluateComparison(lexer.TokenLess, Value{Kind: ValueString, String: "x"}, Value{Kind: ValueInt, Int: 1})
		tAssert.Error(err)
		_, err = compareNumbers(lexer.TokenPlus, 1, 2)
		tAssert.Error(err)
		_, err = evaluateSelfReference(ast.SelfReference{Path: []string{"missing"}}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		tAssert.Error(err)
		_, err = evaluateSelfReference(ast.SelfReference{Path: []string{"name"}}, Value{Kind: ValueString, String: "Ada"})
		tAssert.Error(err)
		tAssert.Error(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueInt}, {kind: ValueInt}}, newVariableRegistry(), symbols, types, schemas, nil))
		_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateConditional(ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bob"`}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bob"`}}}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
	})

	It("converts runtime scalar values back to AST expressions", func() {
		expression := expressionFromValue

		tAssert.Equal(ast.StringLiteral{Lexeme: `"Ada"`}, expression(Value{Kind: ValueString, String: "Ada"}))
		tAssert.Equal(ast.IntLiteral{Lexeme: "7"}, expression(Value{Kind: ValueInt, Int: 7}))
		tAssert.Equal(ast.FloatLiteral{Lexeme: "1.5"}, expression(Value{Kind: ValueFloat, Float: 1.5}))
		tAssert.Equal(ast.HexIntLiteral{Lexeme: "0xFF"}, expression(Value{Kind: ValueHexInt, String: "0xFF"}))
		tAssert.Equal(ast.HexFloatLiteral{Lexeme: "0x2.8"}, expression(Value{Kind: ValueHexFloat, String: "0x2.8"}))
		tAssert.Equal(ast.BooleanLiteral{Value: true}, expression(Value{Kind: ValueBoolean, Boolean: true}))
		tAssert.Equal(ast.StringLiteral{Lexeme: `"null"`}, expression(Value{Kind: ValueNull}))
	})

	It("formats scalar values and evaluates member and prefix helpers", func() {
		formatValue := stringifyValue
		formatted, err := formatValue(Value{Kind: ValueString, String: "Ada"})
		tAssert.NoError(err)
		tAssert.Equal("Ada", formatted)
		formatted, err = formatValue(Value{Kind: ValueHexFloat, Float: -31.5})
		tAssert.NoError(err)
		tAssert.Equal("-0x1F.8", formatted)
		_, err = formatValue(Value{Kind: ValueArray})
		tAssert.ErrorContains(err, "scalar value")

		environment := newValueEnvironment()
		environment.Add("user", Value{Kind: ValueRecord, Record: map[string]Value{
			"name": {Kind: ValueString, String: "Ada"},
		}})
		member, err := evaluateMemberAccess(ast.MemberAccess{
			Target: ast.Identifier{Name: "user"},
			Name:   "name",
		}, environment, Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Equal("Ada", member.String)

		_, err = evaluateMemberAccess(ast.MemberAccess{
			Target: ast.Identifier{Name: "user"},
			Name:   "missing",
		}, environment, Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.ErrorContains(err, "unknown member")

		prefix, err := evaluatePrefix(ast.PrefixExpression{
			Operator: lexer.TokenBang,
			Right:    ast.BooleanLiteral{Value: false},
		}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.True(prefix.Boolean)

		prefix, err = evaluatePrefix(ast.PrefixExpression{
			Operator: lexer.TokenMinus,
			Right:    ast.HexFloatLiteral{Lexeme: "0x1.8"},
		}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueHexFloat, prefix.Kind)
		tAssert.Equal(-1.5, prefix.Float)

	})

	It("evaluates numeric helper operations", func() {
		hexNumeric := evaluateHexNumeric
		floatNumeric := evaluateFloatNumeric
		shiftValue := evaluateShift
		bitwiseValue := evaluateBitwise

		result, err := hexNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexInt, string: "0x5"})

		result, err = hexNumeric(lexer.TokenMinus, Value{Kind: ValueHexFloat, Float: 3.5}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexFloat, string: "0x2.8"})

		result, err = hexNumeric(lexer.TokenStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexFloat, Float: 2.5})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexFloat, string: "0x5.0"})

		result, err = hexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexInt, string: "0x8"})

		result, err = hexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexFloat, Float: 2}, Value{Kind: ValueHexFloat, Float: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexFloat, string: "0x8.0"})

		_, err = hexNumeric(lexer.TokenSlash, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 0})
		tAssert.ErrorContains(err, "division by zero")

		_, err = hexNumeric(lexer.TokenPercent, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.ErrorContains(err, "unknown numeric operator")

		result, err = floatNumeric(lexer.TokenSlash, 7.5, 2.5)
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueFloat, float: 3})

		result, err = floatNumeric(lexer.TokenDoubleStar, 2, 3)
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueFloat, float: 8})

		_, err = floatNumeric(lexer.TokenSlash, 1, 0)
		tAssert.ErrorContains(err, "division by zero")

		_, err = floatNumeric(lexer.TokenPercent, 1, 1)
		tAssert.ErrorContains(err, "unknown numeric operator")

		result, err = shiftValue(lexer.TokenShiftLeft, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 3})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueInt, int64: 8})

		result, err = shiftValue(lexer.TokenShiftRightUnsigned, Value{Kind: ValueHexInt, Int: -8}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.NoError(err)
		tAssert.Equal(ValueHexInt, result.Kind)

		_, err = shiftValue(lexer.TokenShiftLeft, Value{Kind: ValueHexFloat, Float: 1}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.ErrorContains(err, "hex_int operands")

		_, err = shiftValue(lexer.TokenShiftLeft, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: -1})
		tAssert.ErrorContains(err, "negative shift")

		_, err = shiftValue(lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		tAssert.ErrorContains(err, "unknown shift")

		result, err = bitwiseValue(lexer.TokenPipe, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueInt, int64: 3})

		result, err = bitwiseValue(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 3}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.NoError(err)
		assertExpectedValue(result, expectedValue{kind: ValueHexInt, string: "0x2"})

		_, err = bitwiseValue(lexer.TokenPipe, Value{Kind: ValueHexFloat, Float: 1}, Value{Kind: ValueHexInt, Int: 1})
		tAssert.ErrorContains(err, "hex_int operands")

		_, err = bitwiseValue(lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		tAssert.ErrorContains(err, "unknown bitwise")
	})

	It("covers arithmetic, parsing, and type resolution helpers", func() {
		_, err := parseInterpolatedString("unterminated", newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.Error(err)

		intValue, err := parseInt("42")
		tAssert.NoError(err)
		tAssert.Equal(int64(42), intValue.Int)

		floatValue, err := parseFloat("1.5")
		tAssert.NoError(err)
		tAssert.Equal(1.5, floatValue.Float)

		hexInt, err := parseHexInt("0x10")
		tAssert.NoError(err)
		tAssert.Equal(int64(16), hexInt.Int)

		hexFloat, err := parseHexFloat("0x1.8")
		tAssert.NoError(err)
		tAssert.Equal(1.5, hexFloat.Float)

		_, err = parseHexFloat("0x1")
		tAssert.Error(err)

		_, err = parseInt("bad")
		tAssert.Error(err)
		_, err = parseFloat("bad")
		tAssert.Error(err)
		hexIntBad, err := parseHexInt("bad")
		tAssert.NoError(err)
		tAssert.Equal(int64(2989), hexIntBad.Int)
		_, err = parseHexFloat("bad")
		tAssert.Error(err)

		_, err = resolveUnionRecordType(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry())
		tAssert.ErrorContains(err, "fusion members must be schemas")
	})

	It("covers numeric and boolean evaluation helpers", func() {
		result, err := evaluateModulo(Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 3})
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, result.Kind)

		_, err = evaluateModulo(Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 0})
		tAssert.Error(err)

		result, err = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 7})
		tAssert.NoError(err)
		tAssert.True(result.Boolean)

		result, err = evaluateComparison(lexer.TokenLess, Value{Kind: ValueInt, Int: 7}, Value{Kind: ValueInt, Int: 8})
		tAssert.NoError(err)
		tAssert.True(result.Boolean)

		result, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.False(result.Boolean)

		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)

		schemaResult, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "Profile"}}}}, newTypeRegistry())
		tAssert.NoError(err)
		tAssert.Len(schemaResult, 1)

		_, err = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, newTypeRegistry())
		tAssert.NoError(err)

		fields, err := evaluateOutputFields([]ast.OutputField{{Name: "value", Value: ast.NullLiteral{}}}, newValueEnvironment(), newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Empty(fields)

		coerced, err := coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueArray, coerced.Kind)

		processor := New()
		_, err = processor.processInput(`{ value: 1, }`, ".", ".", false)
		tAssert.NoError(err)

		_, err = processor.processScriptInput(`|===|
int base = 1;
|===|`, ".")
		tAssert.NoError(err)

		scriptResult := ScriptResult{}
		_, err = processor.processOutputInput(`[output = 'data'] { result: 1, }`, scriptResult, ".")
		tAssert.NoError(err)

		_, err = evaluateExpression(ast.Identifier{Name: "missing"}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.Error(err)
	})

	It("evaluates core operators and inference branches", func() {
		variables := newVariableRegistry()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas.Add("User", schema)
		types.AddAlias("UserAlias", ast.NamedType{Name: "User"})
		variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		symbols.Add("record", symbolKindVariable)
		symbols.Add("User", symbolKindSchema)
		tAssert.NoError(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil))
		var numericErr error
		for _, item := range []struct {
			operator    lexer.TokenType
			left, right Value
		}{
			{lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2}},
			{lexer.TokenMinus, Value{Kind: ValueHexInt, Int: 3}, Value{Kind: ValueHexInt, Int: 1}},
			{lexer.TokenStar, Value{Kind: ValueFloat, Float: 1.5}, Value{Kind: ValueFloat, Float: 2}},
			{lexer.TokenSlash, Value{Kind: ValueHexFloat, Float: 3.5}, Value{Kind: ValueHexFloat, Float: 2.0}},
			{lexer.TokenDoubleStar, Value{Kind: ValueInt, Int: 2}, Value{Kind: ValueInt, Int: 3}},
		} {
			_, numericErr = evaluateNumeric(item.operator, item.left, item.right)
			tAssert.NoError(numericErr)
		}
		_, numericErr = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueString, String: "x"}, Value{Kind: ValueInt, Int: 1})
		tAssert.Error(numericErr)
		_, numericErr = evaluateModulo(Value{Kind: ValueInt, Int: 5}, Value{Kind: ValueInt, Int: 2})
		tAssert.NoError(numericErr)
		_, numericErr = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		tAssert.NoError(numericErr)
		_, numericErr = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.NoError(numericErr)
		_, numericErr = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueString, String: "Ada"}, Value{Kind: ValueString, String: "Ada"})
		tAssert.NoError(numericErr)
		_, numericErr = evaluateComparison(lexer.TokenLess, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		tAssert.NoError(numericErr)
		_, numericErr = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(numericErr)
		_, numericErr = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(numericErr)
		_, numericErr = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(numericErr)
		_, numericErr = evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(numericErr)
		_, numericErr = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(numericErr)
		_, numericErr = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(numericErr)
		_, numericErr = inferInfixType(ast.InfixExpression{Operator: lexer.TokenCaret, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(numericErr)
		_, numericErr = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(numericErr)
		_, numericErr = inferArrayLiteralType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.StringLiteral{Lexeme: `"Bea"`}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(numericErr)
		_, numericErr = resolveValueType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil)
		tAssert.NoError(numericErr)
		_, numericErr = resolveValueType(ast.NamedType{Name: "UserAlias"}, symbols, types, schemas, nil)
		tAssert.NoError(numericErr)
		_, numericErr = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		tAssert.Error(numericErr)
		_, numericErr = schemaTypeFromTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
		tAssert.NoError(numericErr)
		_, numericErr = schemaTypeFromTypeReference(ast.NamedType{Name: "Missing"}, types)
		tAssert.NoError(numericErr)
		_, numericErr = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(numericErr)
		_, numericErr = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
		tAssert.NoError(numericErr)
	})
})

var _ = Describe("Hex float operators", func() {
	DescribeTable("rejects invalid hexadecimal expressions",
		func(input string, expected string) {
			processor := New()
			_, err := processor.ProcessInDir(input, "../..")
			tAssert.Error(err)
			tAssert.Contains(err.Error(), expected)
		},
		Entry("mixed decimal and hex arithmetic", wrapScriptWithOutput(`|===|
hex_int a = 0x10;
int b = 2;
hex_int c = a + b;
|===|`), "expected hexadecimal operands for operator"),
		Entry("hex float modulo", wrapScriptWithOutput(`|===|
hex_float a = 0x2.8;
hex_float b = 0x0.8;
hex_float c = a % b;
|===|`), "requires hex_int operands"),
		Entry("hex and decimal comparison", `[output = 'data'] { result: 0x10 > 16, }`, "expected operands from the same numeric family"),
		Entry("hex bitwise not", `[output = 'data'] { result: ~0x0F, }`, "expected int after '~'"),
	)
})
