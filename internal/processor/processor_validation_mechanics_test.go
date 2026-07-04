package processor

import (
	"os"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Validation helpers", func() {
	It("extracts guarded names and validates guarded output expressions", func() {
		guarded := extractGuardedNames(ast.InfixExpression{
			Left:     ast.StringLiteral{Lexeme: `"profile"`},
			Operator: lexer.TokenIn,
			Right:    ast.Identifier{Name: "record"},
		}, map[string]struct{}{})
		tAssert.Contains(guarded, "profile")

		guarded = extractGuardedNames(ast.InfixExpression{
			Left: ast.InfixExpression{
				Left:     ast.StringLiteral{Lexeme: `"profile"`},
				Operator: lexer.TokenIn,
				Right:    ast.Identifier{Name: "record"},
			},
			Operator: lexer.TokenAndAnd,
			Right: ast.InfixExpression{
				Left:     ast.StringLiteral{Lexeme: `"age"`},
				Operator: lexer.TokenIn,
				Right:    ast.Identifier{Name: "record"},
			},
		}, map[string]struct{}{})
		tAssert.Contains(guarded, "profile")
		tAssert.Contains(guarded, "age")

		symbols := newSymbolTable()
		symbols.Add("TypeName", symbolKindType)
		optional := map[string]struct{}{"record": {}}
		err := validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "value"}, symbols, optional, map[string]struct{}{})
		tAssert.ErrorContains(err, "requires a presence check")

		err = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "value"}, symbols, optional, map[string]struct{}{"record": {}})
		tAssert.NoError(err)

		err = validateDataOutputExpression(ast.Identifier{Name: "TypeName"}, symbols, optional, map[string]struct{}{})
		tAssert.ErrorContains(err, "cannot reference type or schema declaration")
	})

	It("resolves parse-file schema names from imported files", func() {
		workspace, err := os.MkdirTemp("", "mace-processor-parse-file-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		path := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{
  Profile: Profile;
  Alias: Alias;
  ignore: string;
}`)
		_ = path

		directives := []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./schema.mace"`}}
		names, err := resolveParseFileExportedSchemaNames(directives, workspace, workspace)
		tAssert.NoError(err)
		tAssert.Equal([]string{"Alias", "Profile"}, names)

		directives = []ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"./missing.txt"`}}
		_, err = resolveParseFileExportedSchemaNames(directives, workspace, workspace)
		tAssert.Error(err)
	})
})

var _ = Describe("Validation helper coverage", func() {
	It("covers validation and evaluation branches", func() {
		workspace, err := os.MkdirTemp("", "processor-validation-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas := newSchemaRegistry()
		schemas.Add("User", schema)
		types := newTypeRegistry()
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		symbols.Add("name", symbolKindVariable)
		vars.Add("name", valueType{kind: ValueString})

		tAssert.NoError(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "opt", Value: ast.IntLiteral{Lexeme: "7"}}}}, valueType{kind: ValueRecord, schemaName: "User"}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "unknown", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "7"}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil))

		tAssert.NoError(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "opt": {Kind: ValueInt, Int: 7}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "x"}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("Missing", map[string]Value{}, symbols, types, schemas, nil))

		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Bea"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueInt, Int: 7}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"extra": {Kind: ValueString, String: "x"}}}, valueType{kind: ValueRecord, record: &schema}, symbols, types, schemas, nil))
	})

	It("covers validation helper branches", func() {
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas.Add("User", schema)
		symbols.Add("name", symbolKindVariable)
		vars.Add("name", valueType{kind: ValueString})

		tAssert.NoError(validateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, "User", vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordLiteral(ast.RecordLiteral{}, "Missing", vars, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "7"}}}}, schema, "", vars, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueString}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueInt}}, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueInt}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil))
		tAssert.Error(validateRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Missing"}}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordType(schema, symbols, types, schemas, nil))
	})

	It("covers declaration and output validation branches", func() {
		symbols := newSymbolTable()
		symbols.Add("User", symbolKindSchema)
		symbols.Add("Alias", symbolKindType)
		symbols.Add("value", symbolKindVariable)
		types := newTypeRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		schemas := newSchemaRegistry()
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}})
		variables := newVariableRegistry()
		variables.Add("value", valueType{kind: ValueString})

		tAssert.Error(validateDeclaration(ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{}))
		tAssert.NoError(validateDeclaration(ast.VariableDeclaration{Name: "name", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{}))
		tAssert.Error(validateDeclaration(ast.TypeDeclaration{Name: "Alias", Type: ast.PrimitiveType{Name: "string"}, Description: "doc"}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{"Alias": {}}, map[string]symbolKind{"Alias": symbolKindType}))
		tAssert.Error(validateDeclaration(ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}, Description: "doc"}}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{"User": {Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}}, map[string]symbolKind{"User": symbolKindSchema}))
		tAssert.NoError(validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"User": symbolKindSchema}))
		tAssert.Error(validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "value", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"value": symbolKindVariable}))
		tAssert.NoError(validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "Alias", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"sum"`}}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"Alias": symbolKindType}))

		tAssert.NoError(validateTypeReference(ast.PrimitiveType{Name: "string"}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, symbols, types, schemas, nil))
		symbols.Add("ImportName", symbolKindImport)
		tAssert.NoError(validateTypeReference(ast.NamedType{Name: "User"}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.NamedType{Name: "ImportName"}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(nil, symbols, types, schemas, nil))

		_, err := resolveValueType(ast.NamedType{Name: "User"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil)
		tAssert.Error(err)

		workspace, err := os.MkdirTemp("", "processor-output-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()
		writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ User: User; }`)
		writeFixtureFile(workspace, "parse.mace", `[output = schema]
{ User: User; Other: Other; }`)
		writeFixtureFile(workspace, "not-schema.mace", `[output = data]
{ result: 1; }`)
		context := newProcessContext(workspace, workspace)
		name, ok, err := outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("User", name)
		name, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, context)
		tAssert.NoError(err)
		tAssert.True(ok)
		tAssert.Equal("User", name)
		_, ok, err = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"parse.mace"`}}, context)
		tAssert.Error(err)
		tAssert.False(ok)
		names, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		tAssert.NoError(err)
		tAssert.Equal([]string{"User"}, names)
		_, err = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"not-schema.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)
		tAssert.Error(err)

		tAssert.NoError(validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"doc"`}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Schema"}, {Kind: ast.OutputDirectiveSchema, Value: "Schema"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Schema"}}}))
		tAssert.Error(validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Schema"}, {Kind: ast.OutputDirectiveParseFile, Value: "schema.mace"}}}))

		tAssert.NoError(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "value"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateSchemaOutputFieldType(ast.NamedType{Name: "value"}, symbols))
		tAssert.NoError(validateSchemaOutputFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, symbols))

		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil))
		tAssert.Error(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}, variables, symbols, types, schemas, nil))
		tAssert.Error(validateOutputSchema("Missing", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil))
	})

	It("covers remaining validation and inference branches", func() {
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas.Add("User", schema)
		symbols.Add("User", symbolKindSchema)
		symbols.Add("Thing", symbolKindType)
		types.AddAlias("Alias", ast.NamedType{Name: "User"})
		vars.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		vars.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		vars.Add("flag", valueType{kind: ValueBoolean})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "opt", Value: ast.NullLiteral{}}}, vars, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"opt": {Kind: ValueNull}}, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("Missing", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "unknown": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, vars, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		_ = typesEqual(valueType{kind: ValueRecord}, valueType{kind: ValueRecord, schemaName: "User"})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString})
	})

	It("covers remaining validation and inference branches", func() {
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		symbols := newSymbolTable()
		variables := newVariableRegistry()
		symbols.Add("User", symbolKindSchema)
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		variables.Add("name", valueType{kind: ValueString})

		var seenDocs map[string]struct{}
		var docsByTarget map[string]ast.DocDeclaration
		var declaredKinds map[string]symbolKind

		tAssert.NoError(validateDeclaration(ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"x"`}}, symbols, types, schemas, nil, variables, seenDocs, docsByTarget, declaredKinds))
		tAssert.Error(validateDeclaration(ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}, HasValue: false}, symbols, types, schemas, nil, variables, seenDocs, docsByTarget, declaredKinds))
		tAssert.NoError(validateTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.Error(validateDocDeclaration(ast.DocDeclaration{Target: "value", Documentation: ast.Documentation{}}, symbols, schemas, variables, seenDocs, declaredKinds))
		_, err := resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: `"User"`}}, ast.OutputDirectiveSchema, ".", ".")
		tAssert.Error(err)
		tAssert.NoError(validateDataOutputExpression(ast.Identifier{Name: "name"}, symbols, map[string]struct{}{}, map[string]struct{}{}))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.Identifier{Name: "name"}}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.Identifier{Name: "name"}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.InfixExpression{Left: ast.Identifier{Name: "name"}, Operator: lexer.TokenEqualEqual, Right: ast.Identifier{Name: "name"}}, valueType{kind: ValueBoolean}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, valueType{kind: ValueBoolean}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, "User", variables, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		fieldMap, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.NamedType{Name: "User"}}}}, types)
		tAssert.NoError(err)
		tAssert.NotNil(fieldMap)
		_, err = coerceEvaluatedValueAgainstType(ast.Identifier{Name: "name"}, Value{Kind: ValueString, String: "x"}, valueType{kind: ValueString}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "x"}, valueType{kind: ValueString}, symbols, types, schemas, nil))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString}))
		tAssert.True(typesEqual(valueType{kind: ValueString}, valueType{kind: ValueString}))
	})

	It("covers output and validation edge branches", func() {
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		symbols := newSymbolTable()
		variables := newVariableRegistry()
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		symbols.Add("User", symbolKindSchema)
		variables.Add("name", valueType{kind: ValueString})
		tAssert.NoError(validateDeclaration(ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, symbols, types, schemas, nil, variables, nil, map[string]ast.DocDeclaration{}, map[string]symbolKind{}))
		tAssert.Error(validateTypeReference(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil))
		tAssert.NoError(validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, symbols, map[string]struct{}{}, map[string]struct{}{}))
		tAssert.NoError(validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "name"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, variables, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.Identifier{Name: "name"}, valueType{kind: ValueInt}, variables, symbols, types, schemas, nil))
		tAssert.NoError(validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "name"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, "User", variables, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("Missing", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		tAssert.Error(validateEvaluatedOutputSchema("User", map[string]Value{"unknown": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil))
		fields, err := evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "User", Type: ast.NamedType{Name: "User"}}}}, types)
		tAssert.NoError(err)
		tAssert.NotNil(fields)
		coerced, err := coerceEvaluatedValueAgainstType(ast.Identifier{Name: "name"}, Value{Kind: ValueString, String: "x"}, valueType{kind: ValueString}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, coerced.Kind)
		environment := newValueEnvironment()
		environment.Add("name", Value{Kind: ValueString, String: "Ada"})
		_, err = evaluateExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateExpression(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Operator: lexer.TokenAndAnd, Right: ast.BooleanLiteral{Value: false}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateExpression(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Operator: lexer.TokenOrOr, Right: ast.BooleanLiteral{Value: false}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "name"}, Else: ast.Identifier{Name: "name"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "name"}, ast.StringLiteral{Lexeme: `"b"`}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.InfixExpression{Left: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"a"`}}}}, Operator: lexer.TokenMerge, Right: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"b"`}}}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"a"`}, Else: ast.StringLiteral{Lexeme: `"b"`}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateExpression(ast.BooleanLiteral{Value: true}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueInt})
		tAssert.Error(err)
		_, _ = parseInt("123")
		_, _ = parseFloat("1.25")
		_, _ = parseHexInt("0xzz")
		_, _ = parseHexFloat("0x1.8")
		_, err = parseInterpolatedString(`"$("`, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = parseUnicodeEscape(`\u12`, 4)
		tAssert.Error(err)
	})
})
