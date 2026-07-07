package processor

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Variants", func() {
	DescribeTable("accepts primitive variant alternatives",
		func(typeReference string, firstValue string, secondValue string) {
			processor := New()
			_, err := processor.Process(wrapScriptWithOutput(fmt.Sprintf(`|===|
type Value: %s;
Value first = %s;
Value second = %s;
|===|`, typeReference, firstValue, secondValue)))
			tAssert.NoError(err)
		},
		Entry("string-int", "variant[string, int]", `"Ada"`, `42`),
		Entry("string-float", "variant[string, float]", `"Ada"`, `1.5`),
		Entry("string-boolean", "variant[string, boolean]", `"Ada"`, `true`),
		Entry("int-float", "variant[int, float]", `42`, `1.5`),
		Entry("int-boolean", "variant[int, boolean]", `42`, `true`),
		Entry("float-boolean", "variant[float, boolean]", `1.5`, `true`),
	)

	It("accepts schema and primitive variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema User: { name: string, };
type Value: variant[User, string];
Value first = { name: "Ada", };
Value second = "fallback";
|===|`))
		tAssert.NoError(err)
	})

	It("accepts array variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Value: variant[array<string>, array<int>];
Value names = ["Ada", "Lin"];
Value counts = [1, 2];
|===|`))
		tAssert.NoError(err)
	})

	It("accepts nested array variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Value: variant[array<array<string>>, array<array<array<int>>>];
Value tags = [["api"]];
Value matrix = [[[1]]];
|===|`))
		tAssert.NoError(err)
	})

	It("accepts nested variant aliases", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Scalar: variant[string, int];
type Value: variant[Scalar, boolean];
Value first = "Ada";
Value second = 42;
Value third = true;
|===|`))
		tAssert.NoError(err)
	})

	It("rejects variant variables with non-matching values", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
type Scalar: variant[string, int];
Scalar value = true;
|===|`))
		tAssert.ErrorContains(err, "type mismatch")
	})

	It("rejects record literals that mix fields across variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema EmailLogin: { email: string, password: string, };
schema ApiKeyLogin: { api_key: string, };
type Login: variant[EmailLogin, ApiKeyLogin];
Login value = {
  email: "ada@example.com",
  password: "secret",
  api_key: "token",
};
|===|`))
		tAssert.ErrorContains(err, "type mismatch")
	})

	It("rejects record literals that match multiple variant alternatives", func() {
		processor := New()
		_, err := processor.Process(wrapScriptWithOutput(`|===|
schema Named: { id: string, };
schema OptionallyNamed: { id: string, nickname?: string, };
type Identity: variant[Named, OptionallyNamed];
Identity value = { id: "u1", };
|===|`))
		tAssert.ErrorContains(err, "exactly one variant member")
	})
})

var _ = Describe("Variant type system helpers", func() {
	It("validates type resolution branches", func() {
		workspace, err := os.MkdirTemp("", "processor-helpers-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()
		_ = os.WriteFile(filepath.Join(workspace, "schema.mace"), []byte("[output = schema]\n{ User: User, }\n"), 0o600)

		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas := newSchemaRegistry()
		schemas.Add("User", schema)
		symbols := newSymbolTable()
		symbols.Add("Alias", symbolKindType)
		symbols.Add("User", symbolKindSchema)
		symbols.Add("plain", symbolKindVariable)
		symbols.Add("record", symbolKindVariable)
		types := newTypeRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		types.AddAlias("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		vars := newVariableRegistry()
		vars.Add("plain", valueType{kind: ValueInt})
		vars.Add("record", valueType{kind: ValueRecord, record: &schema})

		tAssert.Error(validateDocDeclaration(ast.DocDeclaration{Target: "missing", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{}))
		tAssert.NoError(validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "Alias", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"sum"`}, Description: &ast.StringLiteral{Lexeme: `"""desc"""`}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"Alias": symbolKindType}))
		tAssert.NoError(validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema}))
		tAssert.Error(validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "plain", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"plain": symbolKindVariable}))
		tAssert.Error(validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"record": symbolKindVariable}))
		tAssert.Error(validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"record": symbolKindVariable}))

		schemas.Add("Audit", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		_, err = resolveUnionRecordType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.NamedType{Name: "Audit"}}}, symbols, types, schemas)
		tAssert.NoError(err)
		_, err = resolveUnionRecordType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}}, symbols, types, schemas)
		tAssert.Error(err)
		_, err = resolveUnionRecordType(ast.NamedType{Name: "Missing"}, symbols, types, schemas)
		tAssert.Error(err)

		_, err = resolveValueType(ast.PrimitiveType{Name: "string"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.ArrayType{Element: ast.PrimitiveType{Name: "int"}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "boolean"}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "User"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = resolveValueType(nil, symbols, types, schemas, nil)
		tAssert.Error(err)

		tAssert.NoError(validateTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(nil, symbols, types, schemas, nil))

		tAssert.NoError(validateVariantValueTypes([]valueType{{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}}))
		tAssert.Error(validateVariantValueTypes([]valueType{{kind: ValueUnknown}}))

		_, err = mergeRecordTypes(ast.RecordType{}, schema)
		tAssert.NoError(err)
		_, err = mergeRecordTypes(schema, ast.RecordType{})
		tAssert.NoError(err)
		_, err = mergeRecordTypes(schema, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "age", Type: ast.PrimitiveType{Name: "int"}}}})
		tAssert.NoError(err)
		_, err = mergeRecordTypes(schema, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}})
		tAssert.Error(err)

		_, err = parseImportPath(ast.StringLiteral{Lexeme: `"` + filepath.ToSlash(filepath.Join(workspace, "schema.mace")) + `"`})
		tAssert.NoError(err)
		_, err = resolveImportPath(workspace, "relative.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath("https://example.com/base/", "schema.mace")
		tAssert.NoError(err)
		_, err = resolveImportPath(workspace, filepath.Join(workspace, "abs.mace"))
		tAssert.Error(err)
		_, err = resolveBoundedPath(workspace, workspace, "../escape.mace")
		tAssert.Error(err)
		_, err = resolveBoundedRemotePath(workspace, "https://example.com/base/", "schema.mace", "https://example.com/base/schema.mace")
		tAssert.NoError(err)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		tAssert.Error(err)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)
		tAssert.Error(err)
		tAssert.Equal("./", formatImportRoot("."))
		tAssert.Equal("root/", formatImportRoot(filepath.Join(workspace, "root")))
		tAssert.Equal("https://example.com/base/", formatImportRoot("https://example.com/base/"))
		_, ok := parseRemoteURL("https://example.com/file.mace")
		tAssert.True(ok)
		_, ok = parseRemoteURL("ftp://example.com/file.mace")
		tAssert.False(ok)
		tAssert.Equal("https://example.com/dir/", basePathDir("https://example.com/dir/file.mace"))
		tAssert.Equal(filepath.Dir(filepath.Join(workspace, "dir", "file.mace")), basePathDir(filepath.Join(workspace, "dir", "file.mace")))
		tAssert.Equal("https://example.com/dir/", basePathDir("https://example.com/dir/file.mace"))

	})

	It("converts runtime value types back to AST type references", func() {
		typeReference := typeReferenceFromValueType
		recordType := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}

		tAssert.Equal(ast.ChoiceType{Members: []ast.Expression{
			ast.StringLiteral{Lexeme: `"Ada"`},
			ast.IntLiteral{Lexeme: "7"},
		}}, typeReference(valueType{choiceValues: []Value{
			{Kind: ValueString, String: "Ada"},
			{Kind: ValueInt, Int: 7},
		}}))
		tAssert.Equal(ast.VariantType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.PrimitiveType{Name: "int"},
		}}, typeReference(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}))
		tAssert.Equal(ast.ArrayType{Element: ast.PrimitiveType{Name: "boolean"}}, typeReference(valueType{kind: ValueArray, element: &valueType{kind: ValueBoolean}}))
		tAssert.Equal(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, typeReference(valueType{kind: ValueArray}))
		tAssert.Equal(ast.NamedType{Name: "User"}, typeReference(valueType{kind: ValueRecord, schemaName: "User"}))
		tAssert.Equal(ast.RecordMapType{Value: ast.PrimitiveType{Name: "float"}}, typeReference(valueType{kind: ValueRecord, element: &valueType{kind: ValueFloat}}))
		tAssert.Equal(recordType, typeReference(valueType{kind: ValueRecord, record: &recordType}))
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, typeReference(valueType{kind: ValueUnknown}))
	})

	It("derives value types and kind names from evaluated values", func() {
		valueTypeFor := valueTypeFromValue
		kindNameFor := Value.kindName

		arrayType := valueTypeFor(Value{Kind: ValueArray})
		tAssert.Equal(ValueArray, arrayType.kind)
		if tAssert.NotNil(arrayType.element) {
			tAssert.Equal(ValueUnknown, arrayType.element.kind)
		}

		arrayType = valueTypeFor(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}})
		tAssert.Equal(ValueArray, arrayType.kind)
		if tAssert.NotNil(arrayType.element) {
			tAssert.Equal(ValueString, arrayType.element.kind)
		}

		tAssert.Equal(ValueRecord, valueTypeFor(Value{Kind: ValueRecord}).kind)
		tAssert.Equal(ValueBoolean, valueTypeFor(Value{Kind: ValueBoolean}).kind)

		tAssert.Equal("array", kindNameFor(Value{Kind: ValueArray}))
		tAssert.Equal("int", kindNameFor(Value{Kind: ValueInt}))
		tAssert.Equal("float", kindNameFor(Value{Kind: ValueFloat}))
		tAssert.Equal("hex_int", kindNameFor(Value{Kind: ValueHexInt}))
		tAssert.Equal("hex_float", kindNameFor(Value{Kind: ValueHexFloat}))
		tAssert.Equal("boolean", kindNameFor(Value{Kind: ValueBoolean}))
		tAssert.Equal("record", kindNameFor(Value{Kind: ValueRecord}))
		tAssert.Equal("null", kindNameFor(Value{Kind: ValueNull}))
		tAssert.Equal("string", kindNameFor(Value{Kind: ValueString}))
		tAssert.Equal("unknown", kindNameFor(Value{Kind: ValueUnknown}))
	})

	It("converts AST type references to public schema types", func() {
		schemaType := schemaTypeFromTypeReference
		types := newTypeRegistry()
		types.AddAlias("ChoiceAlias", ast.ChoiceType{Members: []ast.Expression{
			ast.StringLiteral{Lexeme: `"Ada"`},
			ast.StringLiteral{Lexeme: `"Bob"`},
		}})

		result, err := schemaType(ast.PrimitiveType{Name: "string"}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaPrimitive("string"), result)

		result, err = schemaType(ast.NamedType{Name: "User"}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaNamed("User"), result)

		result, err = schemaType(ast.ArrayType{Element: ast.PrimitiveType{Name: "int"}}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaArray(schemaPrimitive("int")), result)

		result, err = schemaType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "boolean"}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeRecordMap, Element: &SchemaType{Kind: SchemaTypePrimitive, Name: "boolean"}}, result)

		result, err = schemaType(ast.UnionType{Members: []ast.TypeReference{
			ast.NamedType{Name: "User"},
			ast.NamedType{Name: "Audit"},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeUnion, Members: []SchemaType{schemaNamed("User"), schemaNamed("Audit")}}, result)

		result, err = schemaType(ast.VariantType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.PrimitiveType{Name: "int"},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeVariant, Members: []SchemaType{schemaPrimitive("string"), schemaPrimitive("int")}}, result)

		result, err = schemaType(ast.ChoiceType{Members: []ast.Expression{
			ast.Identifier{Name: "ChoiceAlias"},
			ast.StringLiteral{Lexeme: `"Carol"`},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(SchemaType{Kind: SchemaTypeNamed, Name: `choice["Ada", "Bob", "Carol"]`}, result)

		result, err = schemaType(ast.RecordType{Fields: []ast.SchemaField{
			{Name: "name", Type: ast.PrimitiveType{Name: "string"}},
			{Name: "age", Optional: true, Type: ast.PrimitiveType{Name: "int"}},
		}}, types)
		tAssert.NoError(err)
		tAssert.Equal(schemaRecord(map[expectedSchemaField]SchemaType{
			{name: "name"}:                schemaPrimitive("string"),
			{name: "age", optional: true}: schemaPrimitive("int"),
		}), result)

		_, err = schemaType(ast.ArrayType{Element: nil}, types)
		tAssert.ErrorContains(err, "unknown type reference")
	})

	It("infers merge and numeric binary result types", func() {
		mergeType := inferMergeType
		numericType := inferNumericBinary

		recordType := valueType{kind: ValueRecord, schemaName: "User"}
		result, err := mergeType(recordType, recordType)
		tAssert.NoError(err)
		tAssert.Equal(recordType, result)

		arrayElement := valueType{kind: ValueString}
		arrayType := valueType{kind: ValueArray, element: &arrayElement}
		result, err = mergeType(arrayType, arrayType)
		tAssert.NoError(err)
		tAssert.Equal(arrayType, result)

		_, err = mergeType(valueType{kind: ValueString}, recordType)
		tAssert.ErrorContains(err, "records or arrays")

		_, err = mergeType(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Audit"})
		tAssert.ErrorContains(err, "same type")

		result, err = numericType(lexer.TokenPlus, valueType{kind: ValueInt}, valueType{kind: ValueInt})
		tAssert.NoError(err)
		tAssert.Equal(ValueInt, result.kind)

		result, err = numericType(lexer.TokenPlus, valueType{kind: ValueInt}, valueType{kind: ValueFloat})
		tAssert.NoError(err)
		tAssert.Equal(ValueFloat, result.kind)

		result, err = numericType(lexer.TokenSlash, valueType{kind: ValueHexInt}, valueType{kind: ValueHexInt})
		tAssert.NoError(err)
		tAssert.Equal(ValueHexFloat, result.kind)

		result, err = numericType(lexer.TokenPlus, valueType{kind: ValueHexInt}, valueType{kind: ValueHexFloat})
		tAssert.NoError(err)
		tAssert.Equal(ValueHexFloat, result.kind)

		_, err = numericType(lexer.TokenPlus, valueType{kind: ValueString}, valueType{kind: ValueInt})
		tAssert.ErrorContains(err, "numeric operands")

		_, err = numericType(lexer.TokenPlus, valueType{kind: ValueHexInt}, valueType{kind: ValueInt})
		tAssert.ErrorContains(err, "hexadecimal operands")
	})

	It("resolves chained and cyclic type aliases", func() {
		resolveReference := (*typeRegistry).resolveTypeReference
		registry := newTypeRegistry()
		registry.AddAlias("Name", ast.PrimitiveType{Name: "string"})
		registry.AddAlias("DisplayName", ast.NamedType{Name: "Name"})
		registry.AddAlias("External", ast.NamedType{Name: "Missing"})
		registry.AddAlias("Loop", ast.NamedType{Name: "Loop"})

		resolved, err := resolveReference(registry, ast.NamedType{Name: "DisplayName"}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.PrimitiveType{Name: "string"}, resolved)

		resolved, err = resolveReference(registry, ast.NamedType{Name: "External"}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.NamedType{Name: "Missing"}, resolved)

		resolved, err = resolveReference(registry, ast.PrimitiveType{Name: "int"}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.PrimitiveType{Name: "int"}, resolved)

		_, err = resolveReference(registry, ast.NamedType{Name: "Loop"}, map[string]struct{}{})
		tAssert.ErrorContains(err, "cyclic type alias")
	})

	It("sanitizes imported value types and resolves exported references", func() {
		schemas := newSchemaRegistry()
		schemas.Add("Local", ast.RecordType{})

		recordType := valueType{kind: ValueRecord, schemaName: "Local"}
		arrayType := valueType{kind: ValueArray, element: &recordType}
		variantType := valueType{kind: ValueUnknown, members: []valueType{
			{kind: ValueRecord, schemaName: "External"},
			arrayType,
		}}

		sanitized := sanitizeImportedValueType(variantType, schemas)
		tAssert.Equal("External", sanitized.members[0].schemaName)
		if tAssert.NotNil(sanitized.members[1].element) {
			tAssert.Empty(sanitized.members[1].element.schemaName)
		}

		types := newTypeRegistry()
		types.AddAlias("Name", ast.PrimitiveType{Name: "string"})
		types.AddAlias("Names", ast.ArrayType{Element: ast.NamedType{Name: "Name"}})
		types.AddAlias("Loop", ast.NamedType{Name: "Loop"})
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{
			Name: "name",
			Type: ast.NamedType{Name: "Name"},
		}}})

		resolveExport := resolveExportedTypeReference
		resolved, err := resolveExport(ast.NamedType{Name: "Names"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, resolved)

		resolved, err = resolveExport(ast.RecordMapType{Value: ast.NamedType{Name: "Name"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, resolved)

		resolved, err = resolveExport(ast.UnionType{Members: []ast.TypeReference{
			ast.NamedType{Name: "Name"},
			ast.NamedType{Name: "Missing"},
		}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.UnionType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.NamedType{Name: "Missing"},
		}}, resolved)

		resolved, err = resolveExport(ast.VariantType{Members: []ast.TypeReference{
			ast.NamedType{Name: "Name"},
			ast.PrimitiveType{Name: "int"},
		}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.VariantType{Members: []ast.TypeReference{
			ast.PrimitiveType{Name: "string"},
			ast.PrimitiveType{Name: "int"},
		}}, resolved)

		resolved, err = resolveExport(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		tAssert.Equal(ast.RecordType{Fields: []ast.SchemaField{{
			Name: "name",
			Type: ast.PrimitiveType{Name: "string"},
		}}}, resolved)

		_, err = resolveExport(ast.NamedType{Name: "Loop"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.ErrorContains(err, "cyclic type alias")
		_, err = resolveExport(nil, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.ErrorContains(err, "unknown type reference")
	})

	It("covers validation and inference branches", func() {
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		variables := newVariableRegistry()
		symbols.Add("name", symbolKindVariable)
		variables.Add("name", valueType{kind: ValueString})
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		symbols.Add("Missing", symbolKindType)
		types.AddAlias("Missing", ast.PrimitiveType{Name: "string"})

		tAssert.NoError(validateDataOutputExpression(ast.Identifier{Name: "name"}, symbols))
		tAssert.NoError(validateDataOutputExpression(ast.Identifier{Name: "missing"}, symbols))
		tAssert.NoError(validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil))
		_, err := resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.Identifier{Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.BooleanLiteral{Value: true}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.NullLiteral{}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.RecordLiteral{}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.SelfReference{}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "1"}, Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Operator: lexer.TokenAndAnd, Right: ast.BooleanLiteral{Value: false}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueString}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		err = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.NoError(err)
	})

	It("validates type resolution and inference branches", func() {
		symbols := newSymbolTable()
		symbols.Add("User", symbolKindSchema)
		symbols.Add("Alias", symbolKindType)
		symbols.Add("value", symbolKindVariable)
		types := newTypeRegistry()
		types.AddAlias("Choice", ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "7"}}})
		types.AddAlias("UserAlias", ast.NamedType{Name: "User"})
		types.AddAlias("UnionAlias", ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}})
		schemas := newSchemaRegistry()
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		variables := newVariableRegistry()
		variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		variables.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		variables.Add("text", valueType{kind: ValueString})

		_, err := resolveValueType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Choice"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "UserAlias"}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		tAssert.Error(err)

		_, err = resolveUnionRecordType(ast.NamedType{Name: "UnionAlias"}, symbols, types, schemas)
		tAssert.NoError(err)
		_, err = resolveUnionRecordType(ast.PrimitiveType{Name: "string"}, symbols, types, schemas)
		tAssert.Error(err)

		_, err = schemaTypeFromTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, types)
		tAssert.NoError(err)
		_, err = schemaTypeFromTypeReference(nil, types)
		tAssert.Error(err)

		resultType, err := inferExpressionType(ast.Identifier{Name: "Alias"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		tAssert.Equal(ValueUnknown, resultType.kind)
		resultType, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueString, resultType.kind)
		_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "missing"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "array"}, Index: ast.IntLiteral{Lexeme: "bad"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.True(true)
		tAssert.True(true)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenQuestion, Right: ast.BooleanLiteral{Value: true}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Right: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Bob"`}}}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenPipe, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferInfixType(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.True(typesEqual(valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}))
	})

	It("covers record-valued evaluated type branches", func() {
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
		schemas.Add("User", schema)
		symbols.Add("User", symbolKindSchema)
		valueType := valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "Bea"}}}, valueType, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType, symbols, types, schemas, nil)
	})

	It("covers ensureAssignable branches", func() {
		tAssert.Error(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueNull, nullable: true}))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueString, nullable: true}, valueType{kind: ValueNull, nullable: true}))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueUnknown}, valueType{kind: ValueString}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueUnknown}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueInt}))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "User"}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"}))
		tAssert.NoError(ensureAssignable(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueArray}, valueType{kind: ValueArray}))
		tAssert.NoError(ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "a"}, {Kind: ValueString, String: "b"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "a"}}}))
		tAssert.Error(ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "a"}}}, valueType{choiceValues: []Value{{Kind: ValueInt, Int: 1}}}))
		tAssert.NoError(ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "a"}}}, valueType{exactValue: &Value{Kind: ValueString, String: "a"}}))
	})
})
