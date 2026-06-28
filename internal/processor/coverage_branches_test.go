package processor

import (
	"path/filepath"
	"strconv"
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

	workspace := t.TempDir()
	vars := newVariableRegistry()
	symbols := newSymbolTable()
	types := newTypeRegistry()
	types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
	types.AddAlias("LoopA", ast.NamedType{Name: "LoopB"})
	types.AddAlias("LoopB", ast.NamedType{Name: "LoopA"})
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
	mustErr(func() error { _, err := resolveValueType(ast.NamedType{Name: "LoopA"}, symbols, types, schemas, nil); return err }())
	mustErr(func() error { _, err := resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil); return err }())

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

	_, _ = New().ProcessVariablesInScope(`|===|
string value = "x";
|===|`, "", "")
	_, _ = New().ProcessVariablesInScope(`|===|
string value = "x";
|===|`, "", workspace)
	_, _ = New().ProcessVariablesInScope(`|===|
string value = "x";
|===|`, workspace, "")
	_, _ = New().ProcessOutputBlock(`[output = data]
{ value = "x"; }`, ScriptResult{context: newProcessContext("", "")})

	proc := NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "broken": {Kind: ValueString, String: "x"}})
	ctx2 := newProcessContext(workspace, workspace)
	ctx2.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "broken", Type: nil}}})
	ctx2.symbols.Add("User", symbolKindSchema)
	ctx2.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
	ctx2.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"broken": {Kind: ValueString, String: "x"}}})
	_ = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx2)
	_ = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Missing"}}}, &ctx2)

	_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(filepath.Join(workspace, "missing.mace")))}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(filepath.Join(workspace, "schema.mace")))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(filepath.Join(workspace, "schema.mace")))}, Identifiers: []ast.ImportedIdentifier{{Name: "Foo"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
	_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(filepath.Join(workspace, "schema.mace")))}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

	_, _ = resolveBoundedPath(workspace, workspace, "../escape.mace")
	_, _ = resolveBoundedRemotePath("https://example.com/root/dir/", "https://example.com/root/", "../ok.mace", "https://example.com/root/ok.mace")
	_, _ = resolveBoundedRemotePath("https://example.com/root/dir/", "https://example.com/root/", "../escape.mace", "https://example.com/escape.mace")

	_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(filepath.Join(workspace, "schema.mace")))}, {Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(filepath.Join(workspace, "schema.mace")))}} , workspace, workspace)
	_, _ = loadOutputSchemaRecord(filepath.Join(workspace, "schema.mace"), workspace, "schema_file")
	_, _ = loadOutputSchemaRecord(filepath.Join(workspace, "data.mace"), workspace, "schema_file")

	_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, ctx2)
	_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.NamedType{Name: "User"}}, ctx2)
	_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{}, ctx2)

	_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}})
	_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, ctx2.symbols, map[string]struct{}{"record": {}}, map[string]struct{}{})
	_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
	_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
	_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueString}, symbols, types, schemas, nil)
	_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "Foo", Type: ast.PrimitiveType{Name: "string"}}}}, types)
	_, _ = coerceEvaluatedValueAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueArray}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = parseHexFloat("0x")
	_, _ = parseInterpolatedString(`"hello $("`, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = parseUnicodeEscape(`\u12`, 4)

	_, _ = schemaTypeFromTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, types)
	_, _ = schemaTypeFromTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, types)
	_, _ = schemaTypeFromTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, types)
	_, _ = schemaTypeFromTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, types)
	_, _ = schemaTypeFromTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, types)
	_, _ = schemaTypeFromTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
	_, _ = schemaTypeFromTypeReference(ast.NamedType{Name: "User"}, types)
	_, _ = schemaTypeFromTypeReference(nil, types)

	_, _ = resolveExportedTypeReference(ast.ArrayType{Element: ast.NamedType{Name: "Thing"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
	_, _ = resolveExportedTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "Thing"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
	_, _ = resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.NamedType{Name: "Missing"}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
	_, _ = resolveExportedTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Thing"}}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
	_, _ = resolveExportedTypeReference(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
	_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Missing"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})

	_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}})
	_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, ctx2.symbols, map[string]struct{}{"record": {}}, map[string]struct{}{"record": {}})
	_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Optional: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
	_ = validateEvaluatedOutputSchema("User", map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
	_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "Missing"}, symbols, types, schemas, nil)
	_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, record: &record}, symbols, types, schemas, nil)
	_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "Foo", Type: ast.PrimitiveType{Name: "string"}}}}, types)
	_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, types)
	_, _ = coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
	_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)

	ctx3 := newProcessContext(workspace, workspace)
	ctx3.symbols.Add("User", symbolKindSchema)
	ctx3.schemas.Add("User", record)
	ctx3.variables.Add("profile", valueType{kind: ValueRecord, schemaName: "User"})
	ctx3.environment.Add("profile", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
	_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}, {Name: "attrs", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}}}, ctx3)
	_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "profile", Value: ast.Identifier{Name: "profile"}}}}, ctx3)
	_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "attrs", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx3)
	_, _ = exportedOutputFieldType(ast.OutputField{Name: "profile", Value: ast.Identifier{Name: "profile"}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx3)
	_, _ = exportedOutputFieldType(ast.OutputField{Name: "missing", Value: ast.Identifier{Name: "profile"}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, ctx3)

	_, _ = resolveBoundedPath("https://example.com/root/dir/", "https://example.com/root/", "ok.mace")
	_, _ = resolveBoundedRemotePath("https://example.com/root/dir/", "https://example.com/root/", "ok.mace", "https://example.com/root/dir/ok.mace")
	_, _ = resolveBoundedRemotePath("https://example.com/root/dir/", "https://example.com/root/", "ok.mace", "https://example.com/other/ok.mace")
}

func TestCoverageBranchEdgesFixtures(t *testing.T) {
	if _, err := New().ProcessFile("../../fixtures/processor/imports/consumer.mace"); err != nil {
		t.Fatal(err)
	}
	if _, err := New().ProcessFile("../../fixtures/output/schema-file/working-schema.mace"); err != nil {
		t.Fatal(err)
	}
	if _, err := New().ProcessFile("../../fixtures/processor/imports/base.mace"); err != nil {
		t.Fatal(err)
	}
	if _, err := New().ProcessFile("../../fixtures/processor/imports/other.mace"); err != nil {
		t.Fatal(err)
	}
}
