package processor

import (
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Type resolution", func() {
	It("covers type registry, schema resolution, and assignability branches", func() {
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas := newSchemaRegistry()
		schemas.Add("User", schema)
		symbols := newSymbolTable()
		symbols.Add("User", symbolKindSchema)
		types := newTypeRegistry()
		types.AddAlias("Alias", ast.PrimitiveType{Name: "string"})
		types.AddAlias("UserAlias", ast.NamedType{Name: "User"})

		_, _ = schemaTypeFromTypeReference(ast.PrimitiveType{Name: "string"}, types)
		_, _ = schemaTypeFromTypeReference(ast.NamedType{Name: "User"}, types)
		_, _ = schemaTypeFromTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, types)
		_, _ = schemaTypeFromTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, types)
		_, _ = schemaTypeFromTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
		_, _ = schemaTypeFromTypeReference(nil, types)

		_, _ = resolveValueType(ast.PrimitiveType{Name: "string"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "UserAlias"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)

		_, _ = resolveExportedTypeReference(ast.ArrayType{Element: ast.NamedType{Name: "User"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "User"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "User"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Missing"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})

		tAssert.NoError(validateTypeReference(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.NoError(validateTypeReference(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil))
		tAssert.Error(validateTypeReference(nil, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil))
		tAssert.NoError(validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil))
		_ = ensureAssignable(valueType{kind: ValueString, nullable: true}, valueType{kind: ValueNull})
		_ = ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"})
	})
})
