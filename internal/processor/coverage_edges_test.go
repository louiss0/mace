package processor

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

func TestCoverageEdgesScalarAndOperators(t *testing.T) {
	mustErr := func(err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected error")
		}
	}
	mustOK := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _, err := unescapeSequence(`\`)
	mustErr(err)
	for _, seq := range []string{`\\`, `\'`, `\"`, `\n`, `\r`, `\t`, `\u0041`, `\U00000041`} {
		_, _, err := unescapeSequence(seq)
		mustOK(err)
	}
	_, _, err = unescapeSequence(`\x`)
	mustErr(err)
	_, err = parseUnicodeEscape(`\u12`, 4)
	mustErr(err)
	_, err = parseUnicodeEscape(`\uZZZZ`, 4)
	mustErr(err)
	_, err = parseUnicodeEscape(`\uD800`, 4)
	mustErr(err)
	if _, _, err := interpolationContent("$(a(b)c)", 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := interpolationContent("$(abc", 0); err == nil {
		t.Fatal("expected interpolation error")
	}
	for _, v := range []Value{{Kind: ValueFloat, Float: 1.25}, {Kind: ValueBoolean, Boolean: true}, {Kind: ValueHexFloat, Float: 0}, {Kind: ValueHexFloat, Float: -10}, {Kind: ValueHexFloat, Float: 1.0 / 16}, {Kind: ValueHexFloat, Float: 0.9999999999}} {
		_, err := stringifyValue(v)
		mustOK(err)
	}
	_, err = stringifyValue(Value{Kind: ValueArray})
	mustErr(err)

	i := func(n int64) Value { return Value{Kind: ValueInt, Int: n} }
	h := func(n int64) Value { return Value{Kind: ValueHexInt, Int: n} }
	f := func(x float64) Value { return Value{Kind: ValueFloat, Float: x} }
	hf := func(x float64) Value { return Value{Kind: ValueHexFloat, Float: x} }
	s := Value{Kind: ValueString, String: "k"}
	r := Value{Kind: ValueRecord, Record: map[string]Value{"k": i(1)}}
	_, err = evaluateContains(s, r)
	mustOK(err)
	_, err = evaluateContains(i(1), r)
	mustErr(err)
	for _, op := range []lexer.TokenType{lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenDoubleStar} {
		_, err := evaluateNumeric(op, i(4), i(2))
		mustOK(err)
		_, err = evaluateNumeric(op, f(4), i(2))
		mustOK(err)
		_, err = evaluateNumeric(op, h(4), h(2))
		mustOK(err)
		_, err = evaluateNumeric(op, hf(4), h(2))
		mustOK(err)
	}
	_, err = evaluateNumeric(lexer.TokenSlash, i(1), i(0))
	mustErr(err)
	_, err = evaluateNumeric(lexer.TokenSlash, f(1), f(0))
	mustErr(err)
	_, err = evaluateNumeric(lexer.TokenSlash, h(1), h(0))
	mustErr(err)
	_, err = evaluateNumeric(lexer.TokenPlus, i(1), h(1))
	mustErr(err)
	_, err = evaluateNumeric(lexer.TokenEOF, i(1), i(1))
	mustErr(err)
	_, err = evaluateIntPower(2, -1)
	mustErr(err)
	for _, pair := range [][2]Value{{i(5), i(2)}, {f(5), f(2)}, {h(5), h(2)}} {
		_, err := evaluateModulo(pair[0], pair[1])
		mustOK(err)
	}
	for _, pair := range [][2]Value{{i(5), i(0)}, {f(5), f(0)}, {h(5), h(0)}, {h(5), hf(2)}, {s, i(1)}} {
		_, err := evaluateModulo(pair[0], pair[1])
		mustErr(err)
	}
	for _, op := range []lexer.TokenType{lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned} {
		_, err := evaluateShift(op, i(8), i(1))
		mustOK(err)
		_, err = evaluateShift(op, h(8), h(1))
		mustOK(err)
	}
	for _, pair := range [][2]Value{{i(1), i(-1)}, {h(1), h(-1)}, {h(1), i(1)}, {s, i(1)}} {
		_, err := evaluateShift(lexer.TokenShiftLeft, pair[0], pair[1])
		mustErr(err)
	}
	_, err = evaluateShift(lexer.TokenEOF, i(1), i(1))
	mustErr(err)
	_, err = evaluateShift(lexer.TokenEOF, h(1), h(1))
	mustErr(err)
	for _, op := range []lexer.TokenType{lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret} {
		_, err := evaluateBitwise(op, i(3), i(1))
		mustOK(err)
		_, err = evaluateBitwise(op, h(3), h(1))
		mustOK(err)
	}
	for _, pair := range [][2]Value{{h(1), i(1)}, {s, i(1)}} {
		_, err := evaluateBitwise(lexer.TokenAmpersand, pair[0], pair[1])
		mustErr(err)
	}
	_, err = evaluateBitwise(lexer.TokenEOF, i(1), i(1))
	mustErr(err)
	_, err = evaluateBitwise(lexer.TokenEOF, h(1), h(1))
	mustErr(err)
	_, err = evaluateEquality(lexer.TokenEqualEqual, i(1), i(1))
	mustOK(err)
	_, err = evaluateEquality(lexer.TokenNotEqual, h(1), hf(1))
	mustOK(err)
	_, err = evaluateEquality(lexer.TokenEqualEqual, i(1), s)
	mustErr(err)
}

func TestCoverageEdgesTypesAndPaths(t *testing.T) {
	_, err := resolveBoundedPath("/workspace", "/workspace", "/abs.mace")
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = resolveBoundedPath("/workspace", "/workspace", "../abs.mace")
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = resolveBoundedRemotePath("https://example.com/root/dir/", "https://example.com/root/", "../ok.mace", "https://example.com/root/ok.mace")
	if err != nil {
		t.Fatal(err)
	}
	_, err = resolveBoundedRemotePath("https://example.com/root/dir/", "https://example.com/root/file.mace", "../escape.mace", "https://example.com/escape.mace")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := formatImportRoot("/tmp/root/"); got == "" {
		t.Fatal(got)
	}
	if _, ok := parseRemoteURL("example.com/root"); ok {
		t.Fatal("relative should not be remote")
	}
	if _, ok := parseRemoteURL("://bad"); ok {
		t.Fatal("bad url")
	}
	_, err = parseImportPath(ast.StringLiteral{Lexeme: "\"unterminated"})
	if err == nil {
		t.Fatal("expected error")
	}
	vt, err := resolveValueType(ast.PrimitiveType{Name: "hex_int"}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
	if err != nil || vt.kind != ValueHexInt {
		t.Fatal(vt, err)
	}
	vt, err = resolveValueType(ast.PrimitiveType{Name: "hex_float"}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
	if err != nil || vt.kind != ValueHexFloat {
		t.Fatal(vt, err)
	}
	_, err = resolveValueType(ast.NamedType{Name: "Missing"}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCoverageEdgesChoices(t *testing.T) {
	types := newTypeRegistry()
	types.AddAlias("Names", ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}, ast.FloatLiteral{Lexeme: "1.5"}, ast.HexIntLiteral{Lexeme: "0xA"}, ast.HexFloatLiteral{Lexeme: "0x1.8"}, ast.BooleanLiteral{Value: true}}})
	types.AddAlias("Alias", ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Names"}}})
	values, err := resolveChoiceValues([]ast.Expression{ast.Identifier{Name: "Alias"}}, types, map[string]struct{}{})
	if err != nil || len(values) != 6 {
		t.Fatalf("values=%v err=%v", values, err)
	}
	for _, member := range []ast.Expression{ast.Identifier{Name: "Missing"}, ast.Identifier{Name: "NotChoice"}, ast.ArrayLiteral{}} {
		if _, err := resolveChoiceMemberValues(member, types, map[string]struct{}{}); err == nil {
			t.Fatalf("expected error for %T", member)
		}
	}
	types.AddAlias("NotChoice", ast.PrimitiveType{Name: "string"})
	types.AddAlias("Loop", ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Loop"}}})
	if _, err := resolveChoiceMemberValues(ast.Identifier{Name: "Loop"}, types, map[string]struct{}{}); err == nil {
		t.Fatal("expected cycle")
	}
	if _, err := resolveChoiceValues([]ast.Expression{ast.RecordLiteral{}}, types, map[string]struct{}{}); err == nil {
		t.Fatal("expected scalar error")
	}
	if choiceContainsValue([]Value{{Kind: ValueString, String: "Ada"}}, Value{Kind: ValueArray}) {
		t.Fatal("arrays are not scalar choices")
	}
	if choiceValuesEqual([]Value{{Kind: ValueString, String: "Ada"}}, []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bob"}}) {
		t.Fatal("different lengths should not match")
	}
	if choiceValuesEqual([]Value{{Kind: ValueString, String: "Ada"}}, []Value{{Kind: ValueString, String: "Bob"}}) {
		t.Fatal("different values should not match")
	}
	if got := choiceTypeNameForSchema(ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Missing"}, ast.StringLiteral{Lexeme: `"unterminated`}}}, types); got == "" {
		t.Fatal("expected fallback name")
	}
}

func TestCoverageEdgesInferenceAndAssignability(t *testing.T) {
	vars := newVariableRegistry()
	symbols := newSymbolTable()
	types := newTypeRegistry()
	schemas := newSchemaRegistry()
	stringType := valueType{kind: ValueString}
	intType := valueType{kind: ValueInt}
	vars.Add("user", valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}})
	vars.Add("map", valueType{kind: ValueRecord, element: &stringType})
	vars.Add("arr", valueType{kind: ValueArray, element: &intType})
	vars.Add("mystery", valueType{kind: ValueUnknown})
	symbols.Add("User", symbolKindSchema)
	schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})

	casesOK := []ast.Expression{
		ast.Identifier{Name: "missing"},
		ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: "name"},
		ast.MemberAccess{Target: ast.Identifier{Name: "map"}, Name: "anything"},
		ast.MemberAccess{Target: ast.Identifier{Name: "mystery"}, Name: "anything"},
		ast.ArrayAccess{Target: ast.Identifier{Name: "arr"}, Index: ast.IntLiteral{Lexeme: "0"}},
		ast.ArrayAccess{Target: ast.Identifier{Name: "mystery"}, Index: ast.IntLiteral{Lexeme: "0"}},
		ast.ArrayLiteral{},
		ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"a"`}}},
		ast.RecordLiteral{},
		ast.SelfReference{},
		ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.HexFloatLiteral{Lexeme: "0x1.8"}},
		ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.RecordLiteral{}},
		ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.FloatLiteral{Lexeme: "2.0"}},
		ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}},
		ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}},
		ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.HexFloatLiteral{Lexeme: "0x1.0"}},
		ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.HexIntLiteral{Lexeme: "0x2"}},
		ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}},
		ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"x"`}},
		ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"x"`}, Else: ast.NullLiteral{}},
		ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "missing"}, Else: ast.StringLiteral{Lexeme: `"x"`}},
		ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"x"`}, Else: ast.Identifier{Name: "missing"}},
	}
	for _, expr := range casesOK {
		if _, err := inferExpressionType(expr, vars, symbols, types, schemas, nil); err != nil {
			t.Fatalf("unexpected error for %T: %v", expr, err)
		}
	}
	casesErr := []ast.Expression{
		ast.Identifier{Name: "User"},
		ast.MemberAccess{Target: ast.IntLiteral{Lexeme: "1"}, Name: "x"},
		ast.MemberAccess{Target: ast.Identifier{Name: "user"}, Name: "missing"},
		ast.ArrayAccess{Target: ast.IntLiteral{Lexeme: "1"}, Index: ast.IntLiteral{Lexeme: "0"}},
		ast.PrefixExpression{Operator: lexer.TokenQuestion, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.RecordLiteral{}},
		ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.HexFloatLiteral{Lexeme: "0x1.0"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}},
		ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.HexFloatLiteral{Lexeme: "0x1.0"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}},
		ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.StringLiteral{Lexeme: `"x"`}, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.HexFloatLiteral{Lexeme: "0x1.0"}, Right: ast.HexIntLiteral{Lexeme: "0x1"}},
		ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.StringLiteral{Lexeme: `"x"`}, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.StringLiteral{Lexeme: `"x"`}, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.StringLiteral{Lexeme: `"x"`}, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}},
		ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.IntLiteral{Lexeme: "2"}},
		ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.StringLiteral{Lexeme: `"x"`}},
	}
	for _, expr := range casesErr {
		if _, err := inferExpressionType(expr, vars, symbols, types, schemas, nil); err == nil {
			t.Fatalf("expected error for %T", expr)
		}
	}
	if arrayAccessLevel(ast.IntLiteral{Lexeme: "1"}) != 0 {
		t.Fatal("non-array access level should be zero")
	}
	if !typesEqual(valueType{members: []valueType{{kind: ValueInt}}}, valueType{members: []valueType{{kind: ValueInt}}}) {
		t.Fatal("members should match")
	}
	if typesEqual(valueType{members: []valueType{{kind: ValueInt}}}, valueType{members: []valueType{{kind: ValueString}}}) {
		t.Fatal("members should differ")
	}
	if typesEqual(valueType{kind: ValueArray}, valueType{kind: ValueArray}) {
		t.Fatal("arrays without element types should differ")
	}
	if err := ensureAssignable(valueType{members: []valueType{{kind: ValueInt}, {kind: ValueString}}}, valueType{members: []valueType{{kind: ValueInt}}}); err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]valueType{{{kind: ValueString}, {kind: ValueUnknown}}, {{kind: ValueInt}, {kind: ValueString}}, {{kind: ValueRecord, schemaName: "A"}, {kind: ValueRecord, schemaName: "B"}}, {{kind: ValueArray, element: &intType}, {kind: ValueArray, element: &stringType}}} {
		if err := ensureAssignable(pair[0], pair[1]); err == nil {
			t.Fatal("expected assignability error")
		}
	}
}

func TestCoverageEdgesValidationAndEvaluation(t *testing.T) {
	vars := newVariableRegistry()
	env := newValueEnvironment()
	symbols := newSymbolTable()
	types := newTypeRegistry()
	schemas := newSchemaRegistry()
	stringType := valueType{kind: ValueString}
	intType := valueType{kind: ValueInt}
	record := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
	schemas.Add("User", record)
	symbols.Add("user", symbolKindVariable)
	vars.Add("user", valueType{kind: ValueRecord, schemaName: "User"})
	env.Add("user", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})

	okValidations := []struct {
		expr     ast.Expression
		expected valueType
	}{
		{ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "a"}, {Kind: ValueString, String: "b"}}}},
		{ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, valueType{kind: ValueArray, element: &intType}},
		{ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Else: ast.ArrayLiteral{}}, valueType{kind: ValueArray, element: &intType}},
		{ast.RecordLiteral{Fields: []ast.RecordField{{Name: "a", Value: ast.StringLiteral{Lexeme: `"x"`}}}}, valueType{kind: ValueRecord, element: &stringType}},
		{ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "a", Value: ast.StringLiteral{Lexeme: `"x"`}}}}, Else: ast.RecordLiteral{}}, valueType{kind: ValueRecord, element: &stringType}},
		{ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, schemaName: "User"}},
		{ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &record}},
		{ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Bob"`}}}}}, valueType{kind: ValueRecord, schemaName: "User"}},
	}
	for _, item := range okValidations {
		if err := validateExpressionAgainstType(item.expr, item.expected, vars, symbols, types, schemas, nil); err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
	}
	errValidations := []struct {
		expr     ast.Expression
		expected valueType
	}{
		{ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"c"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "a"}}}},
		{ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"x"`}}}, valueType{kind: ValueArray, element: &intType}},
		{ast.RecordLiteral{Fields: []ast.RecordField{{Name: "a", Value: ast.IntLiteral{Lexeme: "1"}}}}, valueType{kind: ValueRecord, element: &stringType}},
		{ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}}, valueType{kind: ValueRecord, schemaName: "User"}},
	}
	for _, item := range errValidations {
		if err := validateExpressionAgainstType(item.expr, item.expected, vars, symbols, types, schemas, nil); err == nil {
			t.Fatal("expected validation error")
		}
	}

	if _, err := evaluateMemberAccess(ast.MemberAccess{Target: ast.IntLiteral{Lexeme: "1"}, Name: "x"}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected member target error")
	}
	if _, err := evaluateArrayAccess(ast.ArrayAccess{Target: ast.IntLiteral{Lexeme: "1"}, Index: ast.IntLiteral{Lexeme: "0"}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected array target error")
	}
	if _, err := evaluateArrayAccess(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "bad"}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected bad index")
	}
	if _, err := evaluateArrayAccess(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "2"}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected out of range")
	}
	if _, err := evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected unknown infix")
	}
	if value, err := evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "bad"}}, env, Value{}, symbols, types, schemas, nil); err != nil || value.Boolean {
		t.Fatalf("expected short-circuit false, got %v %v", value, err)
	}
	if value, err := evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "bad"}}, env, Value{}, symbols, types, schemas, nil); err != nil || !value.Boolean {
		t.Fatalf("expected short-circuit true, got %v %v", value, err)
	}
	if _, err := evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.IntLiteral{Lexeme: "bad"}, Else: ast.IntLiteral{Lexeme: "1"}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected then error")
	}
	if _, err := evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.IntLiteral{Lexeme: "bad"}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected else error")
	}
	if _, err := evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "bad"}}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected array literal error")
	}
	if _, err := evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "x", Value: ast.IntLiteral{Lexeme: "bad"}}}}, env, Value{}, symbols, types, schemas, nil); err == nil {
		t.Fatal("expected record literal error")
	}
}

func TestCoverageEdgesHexFloatFormattingSweep(t *testing.T) {
	for i := int64(-4096); i <= 4096; i++ {
		_ = formatHexFloat(float64(i) / 257.0)
		_ = formatHexFloat(float64(i) / 65536.0)
	}
	for _, v := range []float64{1.0 / 3.0, 2.0 / 3.0, 15.0 / 16.0, 255.0 / 256.0, 4095.0 / 4096.0, 0.99999999999, 15.99999999999} {
		_ = formatHexFloat(v)
	}
}

func TestCoverageEdgesProcessorEntrypoints(t *testing.T) {
	processor := NewWithInput(map[string]Value{"injected": {Kind: ValueString, String: "Ada"}})
	if result, err := processor.Process(`[output = data] { result: "ok"; }`); err != nil || result.Output["result"].String != "ok" {
		t.Fatalf("process result=%v err=%v", result, err)
	}
	for _, input := range []string{`"unterminated`, `[output = data] { result: ; }`, `import { Missing } from "./missing.mace"; [output = data] { result: 1; }`} {
		if _, err := processor.ProcessInDir(input, "."); err == nil {
			t.Fatalf("expected ProcessInDir error for %q", input)
		}
	}
	if script, err := processor.ProcessScriptBlock(`|===|
string name = "Ada";
|===|`); err != nil || script.Variables["name"].String != "Ada" {
		t.Fatalf("script=%v err=%v", script, err)
	}
	for _, input := range []string{`"unterminated`, `|===| string name = ; |===|`, `|===| import { Missing } from "./missing.mace"; |===|`} {
		if _, err := processor.ProcessScriptBlock(input); err == nil {
			t.Fatalf("expected ProcessScriptBlock error for %q", input)
		}
	}
	if vars, err := processor.ProcessVariablesInScope(`|===|
string name = "Ada";
|===|
[output = data] { result: name; }`, "", ""); err != nil || vars["name"].String != "Ada" {
		t.Fatalf("vars=%v err=%v", vars, err)
	}
	for _, input := range []string{`"unterminated`, `|===| string name = ; |===|`, `import { Missing } from "./missing.mace"; [output = data] { result: 1; }`} {
		if _, err := processor.ProcessVariablesInScope(input, ".", "."); err == nil {
			t.Fatalf("expected ProcessVariablesInScope error for %q", input)
		}
	}
	if result, err := processor.ProcessOutputBlock(`[output = data] { result: "ok"; }`, ScriptResult{}); err != nil || result.Output["result"].String != "ok" {
		t.Fatalf("output result=%v err=%v", result, err)
	}
	for _, input := range []string{`"unterminated`, `[output = data] { result: ; }`, `[output = data] { result: missing; }`} {
		if _, err := processor.ProcessOutputBlock(input, ScriptResult{}); err == nil {
			t.Fatalf("expected ProcessOutputBlock error for %q", input)
		}
	}
	if _, err := ParseInputRecord(`"unterminated`); err == nil {
		t.Fatal("expected ParseInputRecord lex error")
	}
	if _, err := ParseInputRecord(`{ name: ; }`); err == nil {
		t.Fatal("expected ParseInputRecord parse error")
	}
}

func TestCoverageEdgesImportAndSchemaFileLoading(t *testing.T) {
	root := t.TempDir()
	write := func(name, contents string) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	schemaPath := write("schema.mace", `|===|
type Name: string;
schema User: { name: Name; };
|===|
[output = schema]
{
  User: User;
  Labels: record<string>;
}`)
	dataPath := write("data.mace", `[output = data] { name: "Ada"; }`)
	badPath := write("bad.mace", `[output = data] { name: ; }`)
	invalidSchemaPath := write("invalid_schema.mace", `|===|
schema User: { name: Missing; };
|===|
[output = schema] { User: User; }`)
	consumerPath := write("consumer.mace", `|===|
from "./schema.mace" import User;
schema Local: { user: User; };
|===|
[output = schema] { Local: Local; }`)
	cycleA := write("cycle_a.mace", `|===|
from "./cycle_b.mace" import B;
|===|
[output = schema] { A: string; }`)
	_ = cycleA
	write("cycle_b.mace", `|===|
from "./cycle_a.mace" import A;
|===|
[output = schema] { B: string; }`)

	if record, err := loadOutputSchemaRecord(schemaPath, root, "schema_file"); err != nil || len(record.Fields) != 2 {
		t.Fatalf("record=%v err=%v", record, err)
	}
	for _, item := range []struct{ path, msg string }{{filepath.Join(root, "missing.mace"), "unable to read"}, {badPath, "unable to parse"}, {dataPath, "must output a schema"}, {invalidSchemaPath, "unknown type"}} {
		if _, err := loadOutputSchemaRecord(item.path, root, "schema_file"); err == nil || !strings.Contains(err.Error(), item.msg) {
			t.Fatalf("expected %q for %s, got %v", item.msg, item.path, err)
		}
	}

	cache := map[string]map[string]ast.Declaration{}
	decls, err := loadSchemaFileDeclarations(consumerPath, root, cache, map[string]struct{}{})
	if err != nil || decls["Local"] == nil {
		t.Fatalf("decls=%v err=%v", decls, err)
	}
	if cached, err := loadSchemaFileDeclarations(consumerPath, root, cache, map[string]struct{}{}); err != nil || cached["Local"] == nil {
		t.Fatalf("cached=%v err=%v", cached, err)
	}
	for _, item := range []struct{ path, msg string }{{filepath.Join(root, "missing.mace"), "unable to read"}, {badPath, "unable to parse"}, {cycleA, "circular import"}} {
		if _, err := loadSchemaFileDeclarations(item.path, root, map[string]map[string]ast.Declaration{}, map[string]struct{}{}); err == nil || !strings.Contains(err.Error(), item.msg) {
			t.Fatalf("expected %q for %s, got %v", item.msg, item.path, err)
		}
	}

	resolved, err := resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote("schema.mace")}}, root, root)
	if err != nil || len(resolved) == 0 {
		t.Fatalf("resolved=%v err=%v", resolved, err)
	}
	for _, directives := range [][]ast.OutputDirective{
		{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote("schema.mace")}, {Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote("schema.mace")}},
		{{Kind: ast.OutputDirectiveSchemaFile, Value: `"unterminated`}},
		{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote("schema.txt")}},
	} {
		if _, err := resolveSchemaFileDeclarations(directives, root, root); err == nil {
			t.Fatal("expected schema file declaration error")
		}
	}
	if none, err := resolveSchemaFileDeclarations(nil, root, root); err != nil || none != nil {
		t.Fatalf("none=%v err=%v", none, err)
	}
}

func TestCoverageEdgesTypeNamesAndUnionRecords(t *testing.T) {
	stringType := valueType{kind: ValueString}
	for _, typ := range []valueType{
		{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}, nullable: true},
		{kind: ValueNull}, {kind: ValueString}, {kind: ValueInt}, {kind: ValueFloat}, {kind: ValueHexInt}, {kind: ValueHexFloat}, {kind: ValueBoolean},
		{kind: ValueArray}, {kind: ValueArray, element: &stringType},
		{kind: ValueRecord}, {kind: ValueRecord, schemaName: "User"}, {kind: ValueRecord, element: &stringType},
		{members: []valueType{{kind: ValueString}, {kind: ValueInt}}},
		{kind: ValueUnknown, nullable: true},
	} {
		if typ.name() == "" {
			t.Fatalf("empty name for %#v", typ)
		}
	}

	symbols := newSymbolTable()
	types := newTypeRegistry()
	schemas := newSchemaRegistry()
	left := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
	right := ast.RecordType{Fields: []ast.SchemaField{{Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
	schemas.Add("Left", left)
	types.AddAlias("RightAlias", right)
	types.AddAlias("UnionAlias", ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "Left"}, ast.NamedType{Name: "RightAlias"}}})
	record, err := resolveUnionRecordType(ast.NamedType{Name: "UnionAlias"}, symbols, types, schemas)
	if err != nil || len(record.Fields) != 2 {
		t.Fatalf("record=%v err=%v", record, err)
	}
	_, err = resolveUnionRecordType(ast.UnionType{Members: []ast.TypeReference{left, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}}, symbols, types, schemas)
	if err == nil {
		t.Fatal("expected duplicate incompatible field error")
	}
	types.AddAlias("BadAlias", ast.PrimitiveType{Name: "string"})
	for _, ref := range []ast.TypeReference{ast.NamedType{Name: "Missing"}, ast.NamedType{Name: "BadAlias"}, ast.PrimitiveType{Name: "string"}} {
		if _, err := resolveUnionRecordType(ref, symbols, types, schemas); err == nil {
			t.Fatalf("expected union record error for %T", ref)
		}
	}
	if err := validateVariantValueTypes([]valueType{{kind: ValueNull}}); err == nil {
		t.Fatal("expected invalid variant member")
	}
}
