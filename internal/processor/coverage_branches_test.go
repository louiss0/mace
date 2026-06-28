package processor

import (
	"testing"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
)

func TestCoverageBranchEdgesMore(t *testing.T) {
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

	vars := newVariableRegistry()
	symbols := newSymbolTable()
	types := newTypeRegistry()
	schemas := newSchemaRegistry()
	record := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
	schemas.Add("User", record)
	symbols.Add("User", symbolKindSchema)
	vars.Add("record", valueType{kind: ValueRecord, record: &record})
	vars.Add("namedRecord", valueType{kind: ValueRecord, schemaName: "User"})
	vars.Add("emptyRecord", valueType{kind: ValueRecord})
	vars.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueInt}})
	vars.Add("unknownArray", valueType{kind: ValueArray})
	vars.Add("unknownValue", valueType{kind: ValueUnknown})

	_, err := inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "namedRecord"}, Name: "name"}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "emptyRecord"}, Name: "name"}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "unknownValue"}, Name: "name"}, vars, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := inferExpressionType(ast.MemberAccess{Target: ast.IntLiteral{Lexeme: "1"}, Name: "name"}, vars, symbols, types, schemas, nil); return err }())
	_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "array"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "unknownArray"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "unknownValue"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	var unknownExpr ast.Expression
	mustErr(func() error { _, err := inferExpressionType(unknownExpr, vars, symbols, types, schemas, nil); return err }())

	mustOK(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.ArrayLiteral{}, valueType{kind: ValueArray}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, schemaName: "User"}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &record}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil))
	mustOK(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil))

	_, err = inferArrayLiteralType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenEOF, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())
	for _, op := range []lexer.TokenType{lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenDoubleStar} {
		_, err = inferInfixType(ast.InfixExpression{Operator: op, Left: ast.IntLiteral{Lexeme: "2"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		mustOK(err)
	}
	for _, op := range []lexer.TokenType{lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned, lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret, lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual, lexer.TokenPercent, lexer.TokenEqualEqual, lexer.TokenNotEqual} {
		_, err = inferInfixType(ast.InfixExpression{Operator: op, Left: ast.IntLiteral{Lexeme: "2"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		mustOK(err)
	}
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.ArrayLiteral{}}, vars, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.HexIntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := inferInfixType(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil); return err }())

	mustOK(func() error { _, err := evaluateHexNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexInt, Int: 2}); return err }())
	mustOK(func() error { _, err := evaluateHexNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 2.5}); return err }())
	mustErr(func() error { _, err := evaluateHexNumeric(lexer.TokenEOF, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexInt, Int: 2}); return err }())
	if ok, err := valuesEqual(Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 1}); err != nil || !ok {
		t.Fatal("expected hex equality")
	}
	if ok, err := valuesEqual(Value{Kind: ValueBoolean, Boolean: true}, Value{Kind: ValueBoolean, Boolean: true}); err != nil || !ok {
		t.Fatal("expected bool equality")
	}
	mustErr(func() error { _, err := valuesEqual(Value{Kind: ValueRecord, Record: map[string]Value{}}, Value{Kind: ValueRecord, Record: map[string]Value{}}); return err }())

	_, err = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	_, err = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := evaluateConditional(ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil); return err }())

	_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, vars, symbols, types, schemas, nil)
	mustOK(err)
	mustErr(func() error { _, err := inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil); return err }())

	if !typesEqual(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}) {
		t.Fatal("expected choice typesEqual")
	}
	if !typesEqual(valueType{members: []valueType{{kind: ValueString}}}, valueType{members: []valueType{{kind: ValueString}}}) {
		t.Fatal("expected members typesEqual")
	}
	if typesEqual(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"}) {
		t.Fatal("expected record schema mismatch")
	}

	mustOK(ensureAssignable(valueType{kind: ValueUnknown}, valueType{kind: ValueString}))
	mustOK(ensureAssignable(valueType{kind: ValueString, nullable: true}, valueType{kind: ValueNull}))
	mustErr(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueNull}))
	mustOK(ensureAssignable(valueType{members: []valueType{{kind: ValueString}}}, valueType{kind: ValueString}))
	mustErr(ensureAssignable(valueType{members: []valueType{{kind: ValueString}}}, valueType{kind: ValueInt}))
	mustOK(ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueString}))
	mustErr(ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueBoolean}))
	mustErr(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueUnknown}))
	mustErr(ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"}))
	mustErr(ensureAssignable(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray}))
}
