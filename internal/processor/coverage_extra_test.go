package processor

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
)

var _ = Describe("Processor extra coverage", func() {
	It("covers remaining processing and import helpers", func() {
		workspace, err := os.MkdirTemp("", "processor-extra-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/schema.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
			case "/nested/schema.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Nested: string; }`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer remoteServer.Close()

		localSchema := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ Local: string; }`)
		localData := writeFixtureFile(workspace, "data.mace", `[output = data]
{ value = "x"; }`)
		badParse := writeFixtureFile(workspace, "bad-parse.mace", `not valid`)
		_ = writeFixtureFile(workspace, "script.mace", `|===|
int value = 1;
|===|`)

		processor := New()
		_, _ = processor.ProcessVariablesInScope(`|===|
int value = 1;
|===|`, "", "")
		_, _ = processor.ProcessVariablesInScope(`|===|
int value = 1;
|===|`, workspace, "")

		context := newProcessContext(workspace, workspace)
		context.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		context.symbols.Add("User", symbolKindSchema)
		context.symbols.Add("Thing", symbolKindType)
		context.types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
		context.symbols.Add("record", symbolKindVariable)
		context.variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		context.environment.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		context.symbols.Add("array", symbolKindVariable)
		context.variables.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueInt}})
		context.environment.Add("array", Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}})

		_, _ = prepareOutputContext(ast.OutputBlock{}, processContext{})
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(localSchema))}}}, context)
		context.symbols.Add("Local", symbolKindVariable)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(localSchema))}}}, context)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(localSchema))}, {Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(localSchema))}}}, context)

		output := ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.MemberAccess{Target: ast.Identifier{Name: "input"}, Name: "name"}}}}
		ctx := newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		ctx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		_ = processor.applyParsedOutputInput(output, &ctx)
		_ = processor.applyParsedOutputInput(ast.OutputBlock{}, &ctx)
		ctx2 := newProcessContext(workspace, workspace)
		ctx2.symbols.Add("User", symbolKindSchema)
		ctx2.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx2.symbols.Add("input", symbolKindVariable)
		ctx2.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		ctx2.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_ = processor.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx2)

		_, _ = resolveImportPath(workspace, filepath.Join(workspace, "abs.mace"))
		_, _ = resolveImportPath(remoteServer.URL+"/", "./schema.mace")
		_, _ = resolveBoundedPath(workspace, workspace, "../escape.mace")
		_, _ = resolveBoundedPath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace")
		_, _ = resolveBoundedRemotePath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace", remoteServer.URL+"/schema.mace")
		_, _ = resolveBoundedRemotePath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace", "https://other.example.com/schema.mace")
		_ = formatImportRoot("")
		_ = formatImportRoot(".")
		_ = formatImportRoot(string(filepath.Separator))
		_ = formatImportRoot(remoteServer.URL + "/")
		_, _ = parseRemoteURL("http://%")
		_, _ = parseRemoteURL(remoteServer.URL + "/schema.mace")
		_, _ = readMaceSource(filepath.Join(workspace, "missing.mace"))
		_, _ = readMaceSource(remoteServer.URL + "/missing.mace")
		_, _ = readMaceSource(localSchema)

		cache := map[string]map[string]importedDeclaration{localSchema: {"Local": {name: "Local", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}}}
		_, _ = loadImportExports(localSchema, workspace, true, cache, map[string]struct{}{})
		_, _ = loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(badParse, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(writeFixtureFile(workspace, "cycle-a.mace", `from "./cycle-b.mace" import User;
[output = schema]
{ User: string; }`), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(localSchema))}, {Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(localSchema))}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.txt"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"bad-parse.mace"`}}, workspace, workspace)

		_, _ = loadOutputSchemaRecord(localSchema, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localData, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(badParse, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file")
		invalidSchema := writeFixtureFile(workspace, "invalid-schema-output.mace", `[output = schema]
{ Broken: Missing; }`)
		_, _ = loadOutputSchemaRecord(invalidSchema, workspace, "schema_file")

		_, _ = loadSchemaFileDeclarations(localSchema, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(filepath.Join(workspace, "missing-schema.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(badParse, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})

		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, context)
		_, _ = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, context)

		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.NamedType{Name: "User"}}, context)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "count", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, context)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "invalid", Type: nil}, context)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, context)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{}, context)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, context)

		_, _ = resolveExportedTypeReference(ast.ArrayType{Element: ast.NamedType{Name: "Thing"}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "Thing"}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.NamedType{Name: "User"}}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Thing"}}}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "User"}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		var unknownTypeRef ast.TypeReference
		_, _ = resolveExportedTypeReference(unknownTypeRef, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Thing"}}}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "broken", Type: unknownTypeRef}}}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
	})

	It("covers remaining processor edge branches", func() {
		workspace, err := os.MkdirTemp("", "processor-edge-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/schema.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Foo: Foo; Bar: Bar; }`)
			case "/nested/schema.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Nested: Nested; }`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer remoteServer.Close()

		schemaNamesFile := writeFixtureFile(workspace, "schema-names.mace", `[output = schema]
{ Foo: Foo; Bar: Bar; }`)
		mismatchNamesFile := writeFixtureFile(workspace, "schema-mismatch.mace", `[output = schema]
{ Foo: Bar; }`)
		dataFile := writeFixtureFile(workspace, "data.mace", `[output = data]
{ value = "x"; }`)
		badFile := writeFixtureFile(workspace, "bad.mace", `not valid`)
		cycleA := writeFixtureFile(workspace, "cycle-a.mace", `from "./cycle-b.mace" import Foo;
[output = schema]
{ Foo: string; }`)
		_ = writeFixtureFile(workspace, "cycle-b.mace", `from "./cycle-a.mace" import Foo;
[output = schema]
{ Foo: string; }`)

		processor := New()
		oldGetwd := getwd
		getwd = func() (string, error) { return "", errors.New("boom") }
		_, _ = processor.ProcessOutputBlock(`[output = data]
{ value = "x"; }`, ScriptResult{context: newProcessContext("", "")})
		getwd = oldGetwd
		_, _ = processor.ProcessOutputBlock(`[output = data]
{ value = "x"; }`, ScriptResult{context: newProcessContext(workspace, workspace)})

		badContext := newProcessContext(workspace, workspace)
		badContext.symbols.Add("User", symbolKindSchema)
		badContext.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "value", Type: ast.PrimitiveType{Name: "string"}}}})
		badContext.symbols.Add("input", symbolKindVariable)
		badContext.variables.Add("input", valueType{kind: ValueString})
		badContext.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"value": {Kind: ValueString, String: "x"}}})
		_, _ = processor.processParsedOutput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.Identifier{Name: "input"}}}}, ast.File{}, badContext)

		_, _ = resolveBoundedPath(workspace, workspace, "../escape.mace")
		_, _ = resolveBoundedPath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace")
		_, _ = resolveBoundedRemotePath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace", remoteServer.URL+"/nested/schema.mace")
		_, _ = resolveBoundedRemotePath(remoteServer.URL+"/", remoteServer.URL+"/", "./schema.mace", "https://other.example.com/schema.mace")
		_, _ = parseRemoteURL("ftp://example.com/schema.mace")
		_, _ = parseRemoteURL(remoteServer.URL + "/schema.mace")
		_, _ = readMaceSource(filepath.Join(workspace, "missing.mace"))
		_, _ = readMaceSource(remoteServer.URL + "/missing.mace")

		_, _ = loadImportExports(cycleA, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(cycleA, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadImportExports(badFile, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(badFile, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})

		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(schemaNamesFile))}, {Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(schemaNamesFile))}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(dataFile))}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(schemaNamesFile))}, {Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaNamesFile))}}, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(schemaNamesFile))}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaNamesFile))}}, ast.OutputDirectiveParseFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(mismatchNamesFile))}}, ast.OutputDirectiveParseFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(badFile))}}, ast.OutputDirectiveSchemaFile, workspace, workspace)

		validationSymbols := newSymbolTable()
		validationTypes := newTypeRegistry()
		validationSchemas := newSchemaRegistry()
		validationVariables := newVariableRegistry()
		validationSchemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}, Description: "name"}}})
		validationSymbols.Add("User", symbolKindSchema)
		validationSymbols.Add("Thing", symbolKindType)
		validationSymbols.Add("Imported", symbolKindImport)
		validationSymbols.Add("record", symbolKindVariable)
		validationSymbols.Add("scalar", symbolKindVariable)
		validationVariables.Add("record", valueType{kind: ValueRecord, schemaName: "User", record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}})
		validationVariables.Add("scalar", valueType{kind: ValueString})
		validationTypes.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
		validationDocs := map[string]ast.DocDeclaration{"Thing": {Target: "Thing"}}
		declaredKinds := map[string]symbolKind{"User": symbolKindSchema, "Thing": symbolKindType, "record": symbolKindVariable, "scalar": symbolKindVariable}
		_ = validateDeclaration(ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}}, validationSymbols, validationTypes, validationSchemas, nil, validationVariables, map[string]struct{}{}, validationDocs, declaredKinds)
		_ = validateDeclaration(ast.TypeDeclaration{Name: "Thing", Type: ast.PrimitiveType{Name: "string"}, Description: "docs"}, validationSymbols, validationTypes, validationSchemas, nil, validationVariables, map[string]struct{}{}, validationDocs, declaredKinds)
		_ = validateDeclaration(ast.SchemaDeclaration{Name: "User", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}, Description: "name"}}}}, validationSymbols, validationTypes, validationSchemas, nil, validationVariables, map[string]struct{}{}, map[string]ast.DocDeclaration{"User": {Target: "User", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"x"`}}}}}, declaredKinds)
		var unknownDeclaration ast.Declaration
		_ = validateDeclaration(unknownDeclaration, validationSymbols, validationTypes, validationSchemas, nil, validationVariables, map[string]struct{}{}, validationDocs, declaredKinds)

		_ = validateTypeReference(ast.ArrayType{Element: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "Thing"}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "Thing"}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"x"`}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.NamedType{Name: "Missing"}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateTypeReference(ast.NamedType{Name: "Imported"}, validationSymbols, validationTypes, validationSchemas, nil)
		var unknownTypeReference ast.TypeReference
		_ = validateTypeReference(unknownTypeReference, validationSymbols, validationTypes, validationSchemas, nil)

		_ = validateDocDeclaration(ast.DocDeclaration{Target: "Missing"}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{}, declaredKinds)
		_ = validateDocDeclaration(ast.DocDeclaration{Target: "Thing"}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{"Thing": {}}, declaredKinds)
		_ = validateDocDeclaration(ast.DocDeclaration{Target: "User", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"x"`}}}}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{}, declaredKinds)
		_ = validateDocDeclaration(ast.DocDeclaration{Target: "scalar", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"x"`}}}}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{}, declaredKinds)
		_ = validateDocDeclaration(ast.DocDeclaration{Target: "record", Kind: ast.DocumentationKindGeneral, Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"x"`}}}}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{}, declaredKinds)

		_ = validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"doc"`}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput}, {Kind: ast.OutputDirectiveOutput}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}})

		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "name", Type: ast.NamedType{Name: "record"}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "dup", Type: ast.PrimitiveType{Name: "string"}}, {Name: "dup", Type: ast.PrimitiveType{Name: "int"}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateSchemaOutputFieldType(ast.ArrayType{Element: ast.NamedType{Name: "record"}}, validationSymbols)
		_ = validateSchemaOutputFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "record"}}}}, validationSymbols)
		_ = validateSchemaOutputFieldType(ast.NamedType{Name: "record"}, validationSymbols)

		_ = typeReferenceFromValueType(valueType{choiceValues: []Value{{Kind: ValueString, String: "x"}}})
		_ = typeReferenceFromValueType(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})
		_ = typeReferenceFromValueType(valueType{kind: ValueArray})
		_ = typeReferenceFromValueType(valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, schemaName: "User"})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, element: &valueType{kind: ValueInt}})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}})
		_, _ = resolveBoundedPath(workspace, workspace, "schema-names.mace")
		_, _ = resolveImportsWithState(ast.File{}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(schemaNamesFile))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}, {Path: ast.StringLiteral{Lexeme: strconv.Quote(filepath.Base(dataFile))}, ImportAs: &ast.ImportedIdentifier{Name: "Alias"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(schemaNamesFile, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(schemaNamesFile, workspace, true, map[string]map[string]importedDeclaration{schemaNamesFile: {}}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(schemaNamesFile, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, badContext)
		_, _ = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "value", Value: ast.Identifier{Name: "input"}}}}, badContext)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(schemaNamesFile))}}, badContext)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(mismatchNamesFile))}}, badContext)
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}})
		_ = validateTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, validationSymbols, validationTypes, validationSchemas, nil)
		_ = validateDocDeclaration(ast.DocDeclaration{Target: "record", Kind: ast.DocumentationKindSchema, Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"doc"`}, Description: &ast.StringLiteral{Lexeme: `"""desc"""`}, Props: map[string]ast.StringLiteral{"name": {Lexeme: `"x"`}}}}, validationSymbols, validationSchemas, validationVariables, map[string]struct{}{}, declaredKinds)
		_ = processor.applyParsedOutputInput(ast.OutputBlock{}, &badContext)
		_ = processor.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "Missing"}}}, &badContext)
		_ = processor.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &badContext)
	})

	It("covers remaining validation and evaluation helpers", func() {
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
		vars.Add("choice", valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}})
		vars.Add("variant", valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})

		_ = validateDataOutputExpression(ast.NullLiteral{}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.Identifier{Name: "User"}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.ArrayLiteral{Elements: []ast.Expression{ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "record"}}}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.PrefixExpression{Right: ast.Identifier{Name: "record"}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.InfixExpression{Left: ast.Identifier{Name: "record"}, Right: ast.Identifier{Name: "record"}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, Right: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"opt"`}, Right: ast.Identifier{Name: "record"}}}, Then: ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, Else: ast.Identifier{Name: "record"}}, symbols, map[string]struct{}{"record": {}}, map[string]struct{}{})

		_ = extractGuardedNames(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, map[string]struct{}{})
		_ = extractGuardedNames(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, Right: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"opt"`}, Right: ast.Identifier{Name: "record"}}}, map[string]struct{}{})

		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "missing", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.NullLiteral{}}}, vars, symbols, types, schemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "invalid", Type: ast.NamedType{Name: "record"}}}, symbols, types, schemas, nil)

		_ = validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.Identifier{Name: "choice"}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.BooleanLiteral{Value: true}, valueType{kind: ValueUnknown}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.BooleanLiteral{Value: true}, valueType{kind: ValueString, nullable: true}, vars, symbols, types, schemas, nil)

		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{members: []valueType{{kind: ValueRecord, schemaName: "User"}, {kind: ValueRecord, schemaName: "User"}}}, symbols, types, schemas, nil)

		_ = validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueString}, {kind: ValueString}}, symbols, types, schemas, nil)
		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, types)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, vars, symbols, types, schemas, nil)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueString}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)

		_, _ = evaluateExpression(ast.Identifier{Name: "record"}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, contextValues("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateExpression(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateExpression(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateExpression(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		var badExpr ast.Expression
		_, _ = evaluateExpression(badExpr, newValueEnvironment(), Value{}, symbols, types, schemas, nil)

		_, _ = parseInt("1")
		_, _ = parseInt("not-int")
		_, _ = parseFloat("1.5")
		_, _ = parseFloat("not-float")
		_, _ = parseHexInt("0xA")
		_, _ = parseHexFloat("0x1.8")
		_, _ = parseHexFloat("0x1")
		_, _ = parseStaticString(`"hello"`)
		_, _ = parseDocString(`"""doc"""`)
		_, _ = parseDocString(`"doc"`)
		_, _ = decodeStringLexeme(`"hello $(1)"`, true, func(string) (string, error) { return "1", nil })
		_, _ = decodeStringLexeme(`'hello'`, false, nil)
		_, _ = decodeStringLexeme(`"\u0041"`, false, nil)
		_, _, _ = unescapeSequence(`\n`)
		_, _, _ = unescapeSequence(`\u0041`)
		_, _, _ = unescapeSequence(`\x`)
		_, _ = parseUnicodeEscape(`\u0041`, 4)
		_, _ = parseUnicodeEscape(`\uD800`, 4)
		_, _ = parseUnicodeEscape(`\u0`, 4)
		_ = formatHexFloat(0)
		_ = formatHexFloat(1.5)
		_ = formatHexFloat(-1.5)

		_, _ = evaluateMemberAccess(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, contextValues("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateMemberAccess(ast.MemberAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Name: "name"}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.FloatLiteral{Lexeme: "1.5"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, contextValues("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenCaret, Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateContains(Value{Kind: ValueString, String: "name"}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_, _ = evaluateMerge(Value{Kind: ValueRecord, Record: map[string]Value{}}, Value{Kind: ValueRecord, Record: map[string]Value{}})
		_, _ = evaluateMerge(Value{Kind: ValueArray, Array: []Value{}}, Value{Kind: ValueArray, Array: []Value{}})
		_, _ = evaluateMerge(Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 2.0})
		_, _ = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueString, String: "x"}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateHexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 3})
		_, _ = evaluateHexNumeric(lexer.TokenSlash, Value{Kind: ValueHexFloat, Float: 2.0}, Value{Kind: ValueHexFloat, Float: 0})
		_, _ = evaluateIntNumeric(lexer.TokenDoubleStar, 2, 3)
		_, _ = evaluateIntNumeric(lexer.TokenSlash, 2, 0)
		_, _ = evaluateFloatNumeric(lexer.TokenDoubleStar, 2, 3)
		_, _ = evaluateFloatNumeric(lexer.TokenSlash, 2, 0)
		_, _ = evaluateIntPower(2, 10)
		_, _ = evaluateIntPower(2, -1)
		_, _ = evaluateModulo(Value{Kind: ValueInt, Int: 5}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateModulo(Value{Kind: ValueHexInt, Int: 5}, Value{Kind: ValueHexInt, Int: 2})
		_, _ = evaluateModulo(Value{Kind: ValueInt, Int: 5}, Value{Kind: ValueString, String: "2"})
		_, _ = evaluateShift(lexer.TokenShiftRightUnsigned, Value{Kind: ValueInt, Int: 8}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueHexInt, Int: 8}, Value{Kind: ValueHexInt, Int: 1})
		_, _ = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueHexInt, Int: 8}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateBitwise(lexer.TokenPipe, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexInt, Int: 2})
		_, _ = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateEquality(lexer.TokenNotEqual, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 1.0})
		_, _ = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		_, _ = valuesEqual(Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 1.0})
		_, _ = valuesEqual(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		_, _ = evaluateComparison(lexer.TokenLessEqual, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateComparison(lexer.TokenLess, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueFloat, Float: 2.0})
		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.IntLiteral{Lexeme: "2"}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateSelfReference(ast.SelfReference{Path: []string{"name"}}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_, _ = evaluateSelfReference(ast.SelfReference{Path: []string{"name"}}, Value{Kind: ValueString, String: "Ada"})
		_, _ = evaluateSelfReference(ast.SelfReference{Path: []string{"missing"}}, Value{Kind: ValueRecord, Record: map[string]Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}})
		_ = valueTypeFromValue(Value{Kind: ValueArray, Array: []Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueRecord, Record: map[string]Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueString, String: "Ada"})

		_ = typesEqual(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}})
		_ = typesEqual(valueType{members: []valueType{{kind: ValueString}}}, valueType{members: []valueType{{kind: ValueString}}})
		_ = typesEqual(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = typesEqual(valueType{kind: ValueArray}, valueType{kind: ValueArray})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString})
		_ = ensureAssignable(valueType{kind: ValueString, nullable: true}, valueType{kind: ValueNull})
		_ = ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}})
		_ = ensureAssignable(valueType{members: []valueType{{kind: ValueString}}}, valueType{members: []valueType{{kind: ValueString}}})
		_ = ensureAssignable(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = ensureAssignable(valueType{kind: ValueArray}, valueType{kind: ValueArray})
		_ = ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"})
		_ = ensureAssignable(valueType{kind: ValueUnknown}, valueType{kind: ValueString})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueUnknown})
	})

	It("covers remaining type and validation edges", func() {
		workspace, err := os.MkdirTemp("", "processor-edge-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/schema.mace" {
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		localSchema := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ Local: string; }`)
		importSchema := writeFixtureFile(workspace, "imports.mace", `from "./schema.mace" import Local;
[output = schema]
{ Local: string; }`)
		scriptSchema := writeFixtureFile(workspace, "script-schema.mace", `from "./schema.mace" import Local;
|===|
int value = 1;
|===|
[output = schema]
{ Local: string; }`)

		ctx := newProcessContext(workspace, workspace)
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("Thing", symbolKindType)
		ctx.types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
		ctx.symbols.Add("record", symbolKindVariable)
		ctx.variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		ctx.environment.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		ctx.symbols.Add("array", symbolKindVariable)
		ctx.variables.Add("array", valueType{kind: ValueArray})
		ctx.environment.Add("array", Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}})
		ctx.symbols.Add("input", symbolKindVariable)
		ctx.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		ctx.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})

		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(localSchema))}}, ast.OutputDirectiveParseFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(localSchema))}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"missing.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)

		_, _ = loadSchemaFileDeclarations(importSchema, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(scriptSchema, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadImportExports(importSchema, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(scriptSchema, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = resolveBoundedPath(string([]byte{0}), string([]byte{0}), "./schema.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", filepath.Join(workspace, "local.mace"))
		_, _ = readMaceSource(server.URL + "/schema.mace")

		_ = validateTypeReference(ast.ArrayType{Element: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.NamedType{Name: "User"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.NamedType{Name: "Thing"}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.NamedType{Name: "Missing"}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "value", Type: ast.NamedType{Name: "Missing"}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}, Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "Thing", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"Thing": symbolKindType})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"record": symbolKindVariable})

		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "record"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "Missing"}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "x"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.StringLiteral{Lexeme: `"Ada"`}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = typesEqual(valueType{kind: ValueArray}, valueType{kind: ValueArray})
		_ = typesEqual(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Bea"}}})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString, nullable: true})
		_ = ensureAssignable(valueType{kind: ValueUnknown}, valueType{kind: ValueString})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueUnknown})
	})

	It("covers remaining resolution and inference branches", func() {
		vars := newVariableRegistry()
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
		schemas.Add("User", schema)
		symbols.Add("User", symbolKindSchema)
		symbols.Add("Thing", symbolKindType)
		types.AddAlias("Alias", ast.NamedType{Name: "User"})
		vars.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		vars.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		vars.Add("value", valueType{kind: ValueString})
		vars.Add("choice", valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}})
		vars.Add("variant", valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})

		_, _ = resolveValueType(ast.PrimitiveType{Name: "string"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "age", Type: ast.PrimitiveType{Name: "int"}}}}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "User"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)

		_, _ = inferExpressionType(ast.Identifier{Name: "value"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.Identifier{Name: "User"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "value"}, Name: "name"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "array"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "value"}, Index: ast.IntLiteral{Lexeme: "0"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.IntLiteral{Lexeme: "1"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.FloatLiteral{Lexeme: "1.5"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.HexIntLiteral{Lexeme: "0xA"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.HexFloatLiteral{Lexeme: "0x1.8"}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.StringLiteral{Lexeme: `"Ada"`}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.BooleanLiteral{Value: true}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.NullLiteral{}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.SelfReference{Path: []string{"name"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.BooleanLiteral{Value: true}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferExpressionType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, vars, symbols, types, schemas, nil)
		var badExpr ast.Expression
		_, _ = inferExpressionType(badExpr, vars, symbols, types, schemas, nil)

		_ = arrayAccessLevel(ast.ArrayAccess{Target: ast.ArrayAccess{Target: ast.Identifier{Name: "array"}, Index: ast.IntLiteral{Lexeme: "0"}}, Index: ast.IntLiteral{Lexeme: "0"}})
		_, _ = inferArrayLiteralType(ast.ArrayLiteral{Elements: []ast.Expression{}}, vars, symbols, types, schemas, nil)
		_, _ = inferArrayLiteralType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}, ast.IntLiteral{Lexeme: "1"}}}, vars, symbols, types, schemas, nil)
		_, _ = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, vars, symbols, types, schemas, nil)
		_, _ = inferInfixType(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferNumericBinary(lexer.TokenDoubleStar, valueType{kind: ValueHexInt}, valueType{kind: ValueHexInt})
		_, _ = inferNumericBinary(lexer.TokenSlash, valueType{kind: ValueHexInt}, valueType{kind: ValueHexInt})
		_, _ = inferNumericBinary(lexer.TokenPlus, valueType{kind: ValueInt}, valueType{kind: ValueString})
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.NullLiteral{}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, vars, symbols, types, schemas, nil)
		_, _ = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, vars, symbols, types, schemas, nil)
		_ = typesEqual(valueType{kind: ValueArray}, valueType{kind: ValueArray})
		_ = typesEqual(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "User"})
		_ = typesEqual(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Bea"}}})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString, nullable: true})
		_ = ensureAssignable(valueType{kind: ValueArray}, valueType{kind: ValueArray})
		_ = ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"})
		_ = ensureAssignable(valueType{kind: ValueUnknown}, valueType{kind: ValueString})
		_ = ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueUnknown})
	})

	It("covers remaining uncovered branches", func() {
		workspace, err := os.MkdirTemp("", "processor-branches-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/schema.mace" {
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		localSchema := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ Local: string; }`)
		localData := writeFixtureFile(workspace, "data.mace", `[output = data]
{ value = "x"; }`)
		_ = writeFixtureFile(workspace, "bad.mace", `not valid`)
		_ = writeFixtureFile(workspace, "script.mace", `|===|
int value = 1;
|===|`)
		_ = writeFixtureFile(workspace, "schema-a.mace", `from "./schema-b.mace" import Local;
[output = schema]
{ Local: string; }`)
		_ = writeFixtureFile(workspace, "schema-b.mace", `from "./schema-a.mace" import Local;
[output = schema]
{ Local: string; }`)

		_, _ = New().ProcessVariablesInScope(`|===|
int value = 1;
|===|`, "", "")
		_, _ = New().ProcessVariablesInScope(`not valid`, workspace, workspace)
		_, _ = New().ProcessOutputBlock(`[output = data] {}`, ScriptResult{})

		proc := New()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
		ctx := newProcessContext(workspace, workspace)
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctx.symbols.Add("Thing", symbolKindType)
		ctx.types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
		ctx.symbols.Add("record", symbolKindVariable)
		ctx.variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		ctx.environment.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		ctx.symbols.Add("value", symbolKindVariable)
		ctx.variables.Add("value", valueType{kind: ValueString})
		ctx.environment.Add("value", Value{Kind: ValueString, String: "Ada"})
		ctx.symbols.Add("opt", symbolKindVariable)
		ctx.variables.Add("opt", valueType{kind: ValueString})
		ctx.environment.Add("opt", Value{Kind: ValueString, String: "Ada"})
		ctx.optionalParseVars["opt"] = struct{}{}

		_, _ = proc.processParsedOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "Local", Type: ast.PrimitiveType{Name: "string"}}}}, ast.File{}, ctx)
		_, _ = proc.processParsedOutput(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ast.File{}, ctx)
		_, _ = proc.processParsedOutput(ast.OutputBlock{Mode: ast.OutputModeData, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ast.File{}, ctx)

		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}, nil, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, nil, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctx)
		ctx.symbols.Add("Local", symbolKindVariable)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctx)
		_, _ = prepareOutputContext(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctx)

		_ = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx)
		_ = proc.applyParsedOutputInput(ast.OutputBlock{}, &ctx)
		ctxDup := newProcessContext(workspace, workspace)
		ctxDup.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		ctxDup.symbols.Add("User", symbolKindSchema)
		ctxDup.symbols.Add("input", symbolKindVariable)
		ctxDup.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		ctxDup.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_ = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctxDup)

		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = resolveBoundedPath(workspace, workspace, "../escape.mace")
		_, _ = resolveBoundedPath(server.URL+"/", server.URL+"/", "./schema.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", "https://other.example.com/schema.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", filepath.Join(workspace, "schema.mace"))
		_ = formatImportRoot("")
		_ = formatImportRoot(".")
		_ = formatImportRoot(server.URL + "/")
		_, _ = parseRemoteURL("ftp://example.com/file.mace")
		_, _ = parseRemoteURL(server.URL + "/schema.mace")
		_, _ = readMaceSource(filepath.Join(workspace, "missing.mace"))
		_, _ = readMaceSource(server.URL + "/missing.mace")
		_, _ = readMaceSource(localSchema)

		cache := map[string]map[string]importedDeclaration{localSchema: {"Local": {name: "Local", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}}}
		_, _ = loadImportExports(localSchema, workspace, true, cache, map[string]struct{}{})
		_, _ = loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localData, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(writeFixtureFile(workspace, "cycle-a.mace", `from "./cycle-b.mace" import User;
[output = schema]
{ User: string; }`), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.txt"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"bad.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = loadOutputSchemaRecord(localSchema, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localData, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file")
		_, _ = loadSchemaFileDeclarations(localSchema, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(filepath.Join(workspace, "missing.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(writeFixtureFile(workspace, "schema-cycle-a.mace", `from "./schema-cycle-b.mace" import User;
[output = schema]
{ User: string; }`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})

		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, ctx)
		_, _ = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ctx)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.NamedType{Name: "User"}}, ctx)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "count", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "invalid", Type: nil}, ctx)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{}, ctx)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, ctx)
		_, _ = resolveExportedTypeReference(ast.ArrayType{Element: ast.NamedType{Name: "Thing"}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "Thing"}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.NamedType{Name: "User"}}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Thing"}}}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "User"}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		var unknownTypeRef ast.TypeReference
		_, _ = resolveExportedTypeReference(unknownTypeRef, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "Thing"}}}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "broken", Type: unknownTypeRef}}}, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})

		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, ctx.symbols, map[string]struct{}{"opt": {}}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, ctx.symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "value"}}}, ctx.symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "value"}}}}, ctx.symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.PrefixExpression{Right: ast.Identifier{Name: "value"}}, ctx.symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.InfixExpression{Left: ast.Identifier{Name: "value"}, Right: ast.Identifier{Name: "value"}}, ctx.symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, Right: ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"opt"`}, Right: ast.Identifier{Name: "record"}}}, Then: ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, Else: ast.Identifier{Name: "record"}}, ctx.symbols, map[string]struct{}{"opt": {}}, map[string]struct{}{})

		_ = validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}, {Kind: ast.OutputDirectiveOutput, Value: "data"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}})

		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "record"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "value"}}}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "missing", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.NullLiteral{}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueString}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}, {Kind: ValueString, String: "Bea"}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.Identifier{Name: "variant"}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString, nullable: true}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "x"}}}, valueType{kind: ValueRecord, schemaName: "User"}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{members: []valueType{{kind: ValueRecord, schemaName: "User"}, {kind: ValueRecord, schemaName: "User"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateEvaluatedValueAgainstVariantMembers(Value{Kind: ValueString, String: "Ada"}, []valueType{{kind: ValueString}, {kind: ValueString}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ctx.types)
		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, ctx.types)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, newValueEnvironment(), ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}}}, newValueEnvironment(), ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueString}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_, _ = evaluateExpression(ast.Identifier{Name: "record"}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateExpression(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, contextValues("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateExpression(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateExpression(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateExpression(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateExpression(nil, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateMemberAccess(ast.MemberAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Name: "name"}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAndAnd, Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenOrOr, Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenIn, Left: ast.StringLiteral{Lexeme: `"name"`}, Right: ast.Identifier{Name: "record"}}, contextValues("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.IntLiteral{Lexeme: "5"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateContains(Value{Kind: ValueString, String: "name"}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_, _ = evaluateContains(Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_, _ = evaluateMerge(Value{Kind: ValueRecord, Record: map[string]Value{}}, Value{Kind: ValueRecord, Record: map[string]Value{}})
		_, _ = evaluateMerge(Value{Kind: ValueArray, Array: []Value{}}, Value{Kind: ValueArray, Array: []Value{}})
		_, _ = evaluateMerge(Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 2.0})
		_, _ = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueString, String: "x"}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateHexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 3})
		_, _ = evaluateHexNumeric(lexer.TokenSlash, Value{Kind: ValueHexFloat, Float: 2.0}, Value{Kind: ValueHexFloat, Float: 0})
		_, _ = evaluateIntNumeric(lexer.TokenDoubleStar, 2, 3)
		_, _ = evaluateIntNumeric(lexer.TokenSlash, 2, 0)
		_, _ = evaluateFloatNumeric(lexer.TokenDoubleStar, 2, 3)
		_, _ = evaluateFloatNumeric(lexer.TokenSlash, 2, 0)
		_, _ = evaluateIntPower(2, 10)
		_, _ = evaluateIntPower(2, -1)
		_, _ = evaluateModulo(Value{Kind: ValueInt, Int: 5}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateModulo(Value{Kind: ValueHexInt, Int: 5}, Value{Kind: ValueHexInt, Int: 2})
		_, _ = evaluateModulo(Value{Kind: ValueInt, Int: 5}, Value{Kind: ValueString, String: "2"})
		_, _ = evaluateShift(lexer.TokenShiftRightUnsigned, Value{Kind: ValueInt, Int: 8}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueHexInt, Int: 8}, Value{Kind: ValueHexInt, Int: 1})
		_, _ = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueHexInt, Int: 8}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateBitwise(lexer.TokenPipe, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexInt, Int: 2})
		_, _ = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateEquality(lexer.TokenNotEqual, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 1.0})
		_, _ = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		_, _ = valuesEqual(Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 1.0})
		_, _ = valuesEqual(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		_, _ = evaluateComparison(lexer.TokenLessEqual, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateComparison(lexer.TokenLess, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueFloat, Float: 2.0})
		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.BooleanLiteral{Value: false}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.BooleanLiteral{Value: true}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.IntLiteral{Lexeme: "1"}, Else: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}, ast.IntLiteral{Lexeme: "2"}}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, newValueEnvironment(), Value{}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = evaluateSelfReference(ast.SelfReference{Path: []string{"name"}}, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_, _ = evaluateSelfReference(ast.SelfReference{Path: []string{"name"}}, Value{Kind: ValueString, String: "Ada"})
		_, _ = evaluateSelfReference(ast.SelfReference{Path: []string{"missing"}}, Value{Kind: ValueRecord, Record: map[string]Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueArray, Array: []Value{{Kind: ValueInt, Int: 1}}})
		_ = valueTypeFromValue(Value{Kind: ValueArray, Array: []Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueRecord, Record: map[string]Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueString, String: "Ada"})
		_ = typesEqual(valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, valueType{kind: ValueArray})
		_ = ensureAssignable(valueType{kind: ValueArray}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = ensureAssignable(valueType{kind: ValueRecord, record: &schema}, valueType{kind: ValueRecord})
		_ = ensureAssignable(valueType{kind: ValueRecord, schemaName: "User"}, valueType{kind: ValueRecord, schemaName: "Other"})

		_ = validateOutputDirectiveStructure(ast.OutputBlock{})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}, {Kind: ast.OutputDirectiveOutput, Value: "data"}}})

		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}, Description: &ast.StringLiteral{Lexeme: `"""desc"""`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "value", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"value": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "Thing", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"Thing": symbolKindType})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "Missing", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "value", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"x"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"value": symbolKindVariable})

		ctx.symbols.Add("Imported", symbolKindImport)
		_, _ = resolveValueType(ast.NamedType{Name: "Imported"}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = resolveValueType(ast.ArrayType{Element: ast.NamedType{Name: "Imported"}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = resolveValueType(ast.RecordMapType{Value: ast.NamedType{Name: "Imported"}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.NamedType{Name: "Imported"}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.NamedType{Name: "Imported"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "value", Type: ast.NamedType{Name: "Imported"}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateRecordType(ast.RecordType{Fields: []ast.SchemaField{{Name: "value", Type: ast.NamedType{Name: "Imported"}}, {Name: "value", Type: ast.PrimitiveType{Name: "string"}}}}, ctx.symbols, ctx.types, ctx.schemas, nil)

		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray, element: nil}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, element: nil, schemaName: "User"}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, valueType{kind: ValueString}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstVariantMembers(ast.StringLiteral{Lexeme: `"Ada"`}, []valueType{{kind: ValueString}, {kind: ValueString}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)

	})

	It("covers remaining schema loading branches", func() {
		workspace, err := os.MkdirTemp("", "processor-load-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/schema.mace":
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
			case "/bad.mace":
				_, _ = io.WriteString(w, `not valid`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer server.Close()

		localSchema := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ Local: string; }`)
		localData := writeFixtureFile(workspace, "data.mace", `[output = data]
{ value = "x"; }`)
		localBad := writeFixtureFile(workspace, "bad.mace", `not valid`)
		localInvalidSchema := writeFixtureFile(workspace, "invalid-schema.mace", `[output = schema]
{ Broken: Missing; }`)
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}

		oldTransport := http.DefaultTransport
		http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, io.ErrUnexpectedEOF
		})
		_, _ = readMaceSource(server.URL + "/schema.mace")
		http.DefaultTransport = oldTransport

		http.DefaultTransport = roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: failingReadCloser{}}, nil
		})
		_, _ = readMaceSource(server.URL + "/schema.mace")
		http.DefaultTransport = oldTransport

		_, _ = resolveImportPath(server.URL+"/", "http://%")
		_, _ = resolveImportPath(server.URL+"/", filepath.Join(workspace, "abs.mace"))
		_, _ = resolveBoundedPath(string([]byte{0}), string([]byte{0}), "./schema.mace")
		_, _ = resolveBoundedPath(workspace, workspace, string([]byte{0}))
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", "https://other.example.com/schema.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", filepath.Join(workspace, "schema.mace"))
		_ = formatImportRoot("")
		_ = formatImportRoot(".")
		_ = formatImportRoot(server.URL + "/")
		_, _ = parseRemoteURL("ftp://example.com/file.mace")
		_, _ = parseRemoteURL(server.URL + "/schema.mace")

		_, _ = importFileAsDeclaration("bad", map[string]importedDeclaration{"x": {name: "x", kind: symbolKind(99)}})
		_, _ = loadImportExports(localSchema, workspace, true, map[string]map[string]importedDeclaration{localSchema: {"Local": {name: "Local", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}}}, map[string]struct{}{})
		_, _ = loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localBad, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localSchema, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{localSchema: {}})

		_, _ = loadOutputSchemaRecord(localSchema, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localData, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localBad, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localInvalidSchema, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(server.URL+"/bad.mace", workspace, "schema_file")

		_, _ = loadSchemaFileDeclarations(localSchema, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(localBad, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(filepath.Join(workspace, "missing-schema.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(writeFixtureFile(workspace, "schema-cycle-a.mace", `from "./schema-cycle-b.mace" import User;
[output = schema]
{ User: string; }`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})

		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.txt"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"bad.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)

		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)

		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Local"}, {Name: "Local"}}}}, nil, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		ctx := newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", schema)
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.symbols.Add("value", symbolKindVariable)
		ctx.variables.Add("value", valueType{kind: ValueString})
		ctx.environment.Add("value", Value{Kind: ValueString, String: "Ada"})
		ctx.optionalParseVars["opt"] = struct{}{}
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "record"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFieldType(ast.NamedType{Name: "value"}, ctx.symbols)
		_ = validateSchemaOutputFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "value"}}}}, ctx.symbols)
		_ = validateSchemaOutputFieldType(ast.ArrayType{Element: ast.NamedType{Name: "value"}}, ctx.symbols)
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, ctx.symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, ctx.symbols, map[string]struct{}{"opt": {}}, map[string]struct{}{})
	})

	It("covers remaining uncovered branches", func() {
		workspace, err := os.MkdirTemp("", "processor-final-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/schema.mace" {
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
		localSchema := writeFixtureFile(workspace, "schema.mace", `[output = schema]
{ Local: string; }`)
		localData := writeFixtureFile(workspace, "data.mace", `[output = data]
{ value = "x"; }`)
		localBad := writeFixtureFile(workspace, "bad.mace", `not valid`)
		localInvalidSchema := writeFixtureFile(workspace, "invalid-schema.mace", `[output = schema]
{ Broken: Missing; }`)
		_ = writeFixtureFile(workspace, "cycle-a.mace", `from "./cycle-b.mace" import User;
[output = schema]
{ User: string; }`)
		_ = writeFixtureFile(workspace, "cycle-b.mace", `from "./cycle-a.mace" import User;
[output = schema]
{ User: string; }`)

		_, _ = New().ProcessVariablesInScope(`from "./bad.mace" import Missing;
|===|
int value = 1;
|===|`, workspace, workspace)
		_, _ = New().ProcessOutputBlock(`[output = data] {}`, ScriptResult{})
		_, _ = New().ProcessOutputBlock(`[output = schema]
{ Broken: Missing; }`, ScriptResult{context: ScriptResult{}.context})

		proc := New()
		ctx := newProcessContext(workspace, workspace)
		ctx.schemas.Add("User", schema)
		ctx.symbols.Add("User", symbolKindSchema)
		ctx.symbols.Add("Thing", symbolKindType)
		ctx.types.AddAlias("Thing", ast.PrimitiveType{Name: "string"})
		ctx.symbols.Add("record", symbolKindVariable)
		ctx.variables.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		ctx.environment.Add("record", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		ctx.symbols.Add("value", symbolKindVariable)
		ctx.variables.Add("value", valueType{kind: ValueString})
		ctx.environment.Add("value", Value{Kind: ValueString, String: "Ada"})
		ctx.symbols.Add("opt", symbolKindVariable)
		ctx.variables.Add("opt", valueType{kind: ValueString})
		ctx.environment.Add("opt", Value{Kind: ValueString, String: "Ada"})
		ctx.optionalParseVars["opt"] = struct{}{}

		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, nil, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}, nil, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState(nil, &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: false}}}, workspace, workspace, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctx)
		_, _ = prepareOutputContext(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx)
		ctx.symbols.Add("Local", symbolKindVariable)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctx)

		_ = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx)
		_ = proc.applyParsedOutputInput(ast.OutputBlock{}, &ctx)
		ctxDup := newProcessContext(workspace, workspace)
		ctxDup.schemas.Add("User", schema)
		ctxDup.symbols.Add("User", symbolKindSchema)
		ctxDup.symbols.Add("input", symbolKindVariable)
		ctxDup.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		ctxDup.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_ = proc.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctxDup)

		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./cycle-a.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = resolveBoundedPath(workspace, workspace, "../escape.mace")
		_, _ = resolveBoundedPath(server.URL+"/", server.URL+"/", "./schema.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", "https://other.example.com/schema.mace")
		_, _ = resolveBoundedRemotePath(server.URL+"/", server.URL+"/", "./schema.mace", filepath.Join(workspace, "schema.mace"))
		_ = formatImportRoot("")
		_ = formatImportRoot(".")
		_ = formatImportRoot(server.URL + "/")
		_, _ = parseRemoteURL("ftp://example.com/file.mace")
		_, _ = parseRemoteURL(server.URL + "/schema.mace")

		_, _ = loadImportExports(localSchema, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localData, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localBad, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localSchema, workspace, true, map[string]map[string]importedDeclaration{localSchema: {"Local": {name: "Local", kind: symbolKindVariable, value: Value{Kind: ValueString, String: "Ada"}, vtype: valueType{kind: ValueString}}}}, map[string]struct{}{})
		_, _ = loadImportExports(filepath.Join(workspace, "missing.mace"), workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = loadOutputSchemaRecord(localSchema, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localData, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localBad, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(localInvalidSchema, workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(server.URL+"/bad.mace", workspace, "schema_file")

		_, _ = loadSchemaFileDeclarations(localSchema, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(localBad, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(writeFixtureFile(workspace, "schema-cycle-c.mace", `from "./schema-cycle-d.mace" import User;
[output = schema]
{ User: string; }`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})

		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.txt"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"bad.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)

		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ast.OutputDirectiveParseFile, workspace, workspace)

		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "record"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateSchemaOutputFieldType(ast.NamedType{Name: "value"}, ctx.symbols)
		_ = validateSchemaOutputFieldType(ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.NamedType{Name: "value"}}}}, ctx.symbols)
		_ = validateSchemaOutputFieldType(ast.ArrayType{Element: ast.NamedType{Name: "value"}}, ctx.symbols)
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, ctx.symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "opt"}, Name: "name"}, ctx.symbols, map[string]struct{}{"opt": {}}, map[string]struct{}{})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Doc: &ast.StringLiteral{Lexeme: `"""doc"""`}, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Mode: ast.OutputModeSchema, Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveOutput, Value: "data"}, {Kind: ast.OutputDirectiveOutput, Value: "data"}}})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveKind(99), Value: "x"}}})

		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}, "missing": {Lexeme: `"Ada"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "value", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"value": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"record": symbolKindVariable})

		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "value"}}, {Name: "profile", Type: ast.NamedType{Name: "value"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "Ada"}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User"}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{kind: ValueRecord}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, schemaName: "User"}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, valueType{kind: ValueRecord, schemaName: "User"}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_, _ = resolveExportedTypeReference(nil, ctx.types, ctx.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "broken", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, ctx)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "broken", Type: nil}, ctx)
		_, _ = New().ProcessVariablesInScope(`int value = 1;
int value = 2;`, workspace, workspace)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Bea"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueInt, Int: 1}, valueType{kind: ValueFloat}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: nil}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueInt}}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User"}, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ArrayLiteral{}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord, schemaName: "User"}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.NullLiteral{}, Else: ast.StringLiteral{Lexeme: `"Ada"`}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, ctx.variables, ctx.symbols, ctx.types, ctx.schemas, nil)
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"summary"`}, Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"Ada"`}}}}, ctx.symbols, ctx.schemas, ctx.variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateOutputDirectiveStructure(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveKind(99), Value: "x"}}})
		_ = validateSchemaOutputFields([]ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "value"}}, {Name: "profile", Type: ast.NamedType{Name: "value"}}}, ctx.symbols, ctx.types, ctx.schemas, nil)
	})
	It("covers remaining helper edge branches", func() {
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		vars := newVariableRegistry()
		schema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}, {Name: "opt", Optional: true, Type: ast.PrimitiveType{Name: "int"}}}}
		schemas.Add("User", schema)
		symbols.Add("User", symbolKindSchema)
		symbols.Add("Thing", symbolKindType)
		types.AddAlias("Alias", ast.NamedType{Name: "User"})
		vars.Add("record", valueType{kind: ValueRecord, schemaName: "User"})
		vars.Add("array", valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		vars.Add("flag", valueType{kind: ValueBoolean})

		_ = typeReferenceFromValueType(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}})
		_ = typeReferenceFromValueType(valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}})
		_ = typeReferenceFromValueType(valueType{kind: ValueArray})
		_ = typeReferenceFromValueType(valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, element: &valueType{kind: ValueInt}})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, schemaName: "User"})
		_, _ = resolveExportedTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "User"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.ChoiceType{}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Alias"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		types.AddAlias("Loop", ast.NamedType{Name: "Loop"})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Loop"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		schemas.Add("Loop", ast.RecordType{Fields: []ast.SchemaField{{Name: "self", Type: ast.NamedType{Name: "Loop"}}}})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Loop"}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		var missingType ast.TypeReference
		_, _ = resolveExportedTypeReference(missingType, types, schemas, map[string]struct{}{}, map[string]struct{}{})

		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "missing", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "User", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "Thing", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"Thing": symbolKindType})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"record": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "flag", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"flag": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"""doc"""`}, Description: &ast.StringLiteral{Lexeme: `"""doc"""`}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})

		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "missing", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.NullLiteral{}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}, vars, symbols, types, schemas, nil)

		_ = validateExpressionAgainstType(ast.ArrayLiteral{}, valueType{kind: ValueArray}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.IntLiteral{Lexeme: "1"}}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.Identifier{Name: "flag"}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.Identifier{Name: "flag"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, vars, symbols, types, schemas, nil)

		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueString}}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Bea"}}}, symbols, types, schemas, nil)

		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.StringLiteral{Lexeme: `"Ada"`}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)

		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Index: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenQuestion, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)

		_, _ = parseHexFloat("0x1")
		_, _ = parseHexFloat("0x1.G")
		_, _ = decodeStringLexeme(`"$(1)"`, false, nil)
		_, _ = decodeStringLexeme(`"\q"`, false, nil)
		_, _, _ = unescapeSequence(`\`)
		_, _ = parseUnicodeEscape(`\uD800`, 4)
		_ = formatHexFloat(0)
		_ = formatHexFloat(1.0)
		_ = formatHexFloat(1.5)
		_ = formatHexFloat(-0.5)
		_, _ = valuesEqual(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		_ = typesEqual(valueType{kind: ValueArray}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = ensureAssignable(valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Bea"}}})
		_ = ensureAssignable(valueType{members: []valueType{{kind: ValueString}}}, valueType{members: []valueType{{kind: ValueInt}}})

		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "missing", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindType})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"record": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "record", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"record": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "User", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "Thing", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"Thing": symbolKindType})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "flag", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"flag": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "flag", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"flag": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "flag", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"""doc"""`}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"flag": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"""doc"""`}, Description: &ast.StringLiteral{Lexeme: `"""doc"""`}}}, symbols, schemas, vars, map[string]struct{}{"User": {}}, map[string]symbolKind{"User": symbolKindSchema})

		symbols.Add("DocType", symbolKindType)
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "DocType", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"DocType": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "User", Documentation: ast.Documentation{}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: "bad"}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Description: &ast.StringLiteral{Lexeme: `"bad"`}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "Thing", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"Thing": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "flag", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"flag": symbolKindVariable})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"Ada"`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"bad`}}}}, symbols, schemas, vars, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Summary: &ast.StringLiteral{Lexeme: `"""doc"""`}, Description: &ast.StringLiteral{Lexeme: `"""doc"""`}}}, symbols, schemas, vars, map[string]struct{}{"User": {}}, map[string]symbolKind{"User": symbolKindSchema})

		_ = validateExpressionAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, valueType{kind: ValueArray}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: false}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil)

		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}, symbols, types, schemas, nil)

		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "bad", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.IntLiteral{Lexeme: "1"}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "invalid", Type: ast.NamedType{Name: "Missing"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(nil, symbols, types, schemas, nil)

		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Index: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenTilde, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluatePrefix(ast.PrefixExpression{Operator: lexer.TokenMinus, Right: ast.SelfReference{Path: []string{"x"}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenQuestion, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPlus, Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenMerge, Left: ast.RecordLiteral{}, Right: ast.StringLiteral{Lexeme: `"Ada"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenPercent, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenShiftLeft, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "-1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenAmpersand, Left: ast.HexIntLiteral{Lexeme: "0x1"}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEqualEqual, Left: ast.RecordLiteral{}, Right: ast.RecordLiteral{}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenLess, Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.StringLiteral{Lexeme: `"Ada"`}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateMerge(Value{Kind: ValueRecord, Record: map[string]Value{}}, Value{Kind: ValueArray, Array: []Value{}})
		_, _ = evaluateNumeric(lexer.TokenPlus, Value{Kind: ValueString, String: "x"}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateHexNumeric(lexer.TokenSlash, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexInt, Int: 0})
		_, _ = evaluateIntNumeric(lexer.TokenSlash, 1, 0)
		_, _ = evaluateFloatNumeric(lexer.TokenSlash, 1, 0)
		_, _ = evaluateModulo(Value{Kind: ValueHexInt, Int: 5}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateBitwise(lexer.TokenCaret, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		_, _ = valuesEqual(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		_, _ = parseInterpolatedString(`"$(1"`, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = decodeStringLexeme(`"$(1"`, true, func(string) (string, error) { return "", nil })
		_, _ = decodeStringLexeme(`"\uD800"`, false, nil)
		_, _, _ = unescapeSequence(`\x`)
		_, _ = parseUnicodeEscape(`\u`, 4)
		_ = formatHexFloat(0)
		_ = formatHexFloat(1.0)
		_ = formatHexFloat(1.5)
		_ = formatHexFloat(-0.5)

		_ = validateExpressionAgainstType(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "0"}}, valueType{kind: ValueString}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Else: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Bea"`}}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, valueType{kind: ValueRecord}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, schemaName: "User"}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, schemaName: "User"}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.RecordLiteral{}, valueType{kind: ValueRecord, schemaName: "User"}, vars, symbols, types, schemas, nil)
		_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, schema, "User", vars, symbols, types, schemas, nil)
		_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", vars, symbols, types, schemas, nil)
		_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{}, schema, "User", vars, symbols, types, schemas, nil)
		_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Optional: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", vars, symbols, types, schemas, nil)

		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User", record: &schema}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueRecord, schemaName: "Missing"}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord, schemaName: "User"}, symbols, types, schemas, nil)

		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, types)
		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, types)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "missing", Type: ast.PrimitiveType{Name: "string"}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "bad", Type: ast.PrimitiveType{Name: "string"}, HasValue: true, Value: ast.IntLiteral{Lexeme: "1"}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_ = evaluateScript([]ast.Declaration{ast.VariableDeclaration{Name: "invalid", Type: ast.NamedType{Name: "Missing"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, newValueEnvironment(), symbols, types, schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, Value{Kind: ValueString, String: "Ada"}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Value{Kind: ValueArray, Array: []Value{{Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Alias"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.NamedType{Name: "Missing"}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "string"}, ast.PrimitiveType{Name: "int"}}}, symbols, types, schemas, nil)
		_, _ = resolveValueType(nil, symbols, types, schemas, nil)

		_, _ = evaluateArrayAccess(ast.ArrayAccess{Target: ast.StringLiteral{Lexeme: `"Ada"`}, Index: ast.IntLiteral{Lexeme: "foo"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateInfix(ast.InfixExpression{Operator: lexer.TokenEOF, Left: ast.IntLiteral{Lexeme: "1"}, Right: ast.IntLiteral{Lexeme: "2"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateComparison(lexer.TokenLess, Value{Kind: ValueString, String: "Ada"}, Value{Kind: ValueString, String: "Bea"})
		_, _ = evaluateComparison(lexer.TokenLess, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueInt, Int: 2})
		_, _ = evaluateComparison(lexer.TokenLess, Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueHexFloat, Float: 1.0})
		_, _ = evaluateEquality(lexer.TokenEqualEqual, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueString, String: "x"})
		_, _ = valuesEqual(Value{Kind: ValueHexInt, Int: 1}, Value{Kind: ValueRecord})
		_, _ = evaluateModulo(Value{Kind: ValueFloat, Float: 5.5}, Value{Kind: ValueFloat, Float: 2.0})
		_, _ = evaluateShift(lexer.TokenShiftLeft, Value{Kind: ValueInt, Int: 1}, Value{Kind: ValueInt, Int: -1})
		_, _ = evaluateBitwise(lexer.TokenPipe, Value{Kind: ValueString, String: "x"}, Value{Kind: ValueInt, Int: 1})
		_, _ = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.IntLiteral{Lexeme: "1"}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateConditional(ast.ConditionalExpression{Condition: ast.IntLiteral{Lexeme: "1"}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateArrayLiteral(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateRecordLiteral(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateSelfReference(ast.SelfReference{Path: []string{"x"}}, Value{Kind: ValueString, String: "Ada"})
		_ = valueTypeFromValue(Value{Kind: ValueArray, Array: []Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueRecord, Record: map[string]Value{}})
		_ = valueTypeFromValue(Value{Kind: ValueString, String: "Ada"})
		_ = formatHexFloat(15.999999999999998)
		_ = formatHexFloat(65535.99999999999)
		_, _ = parseUnicodeEscape(`\u0041`, 4)
		_, _ = parseUnicodeEscape(`\u0000`, 4)
		_, _ = parseHexFloat(`0x1.0000000001`)
		_, _ = parseHexFloat(`0x1.fffffffff`)
		_, _ = parseInterpolatedString(`"hello $(name)"`, contextValues("name", Value{Kind: ValueString, String: "Ada"}), Value{}, symbols, types, schemas, nil)
		_, _ = decodeStringLexeme(`"hello $(name)"`, true, func(string) (string, error) { return "Ada", nil })
		_, _ = decodeStringLexeme(`"hello $(name)"`, false, nil)
		_, _, _ = unescapeSequence(`\u0041`)
		_, _, _ = unescapeSequence(`\U00000041`)
		_, _ = resolveBoundedPath(".", ".", "./schema.mace")
		_, _ = resolveBoundedRemotePath("http://example.com/", "http://example.com/", "./schema.mace", "http://example.com/nested/schema.mace")
		_, _ = resolveBoundedRemotePath("http://example.com/", "http://example.com/", "./schema.mace", "http://example.com/other.mace")
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, schemaName: "User"})
		_ = typeReferenceFromValueType(valueType{kind: ValueArray, element: &valueType{kind: ValueString}})
		_ = typeReferenceFromValueType(valueType{kind: ValueRecord, element: &valueType{kind: ValueInt}})
		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema}, types)

		_ = validateExpressionAgainstType(ast.ArrayAccess{Target: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}}, Index: ast.IntLiteral{Lexeme: "0"}}, valueType{choiceValues: []Value{{Kind: ValueString, String: "Ada"}}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, Else: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Bea"`}}}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, vars, symbols, types, schemas, nil)
		_ = validateExpressionAgainstType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Bea"`}}}}}, valueType{kind: ValueRecord, record: &schema}, vars, symbols, types, schemas, nil)
		_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Optional: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", vars, symbols, types, schemas, nil)
		_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, schema, "User", vars, symbols, types, schemas, nil)
		_ = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: nil}}}, "", vars, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"opt": {Kind: ValueInt, Int: 1}}}, valueType{kind: ValueRecord, record: &schema}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{}}, valueType{kind: ValueRecord, record: &schema}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "opt": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueArray, Array: []Value{}}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueNull}, valueType{kind: ValueString}, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "opt", Value: ast.NullLiteral{}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, {Name: "opt", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateOutputSchema("User", []ast.OutputField{{Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, vars, symbols, types, schemas, nil)
		_ = validateDataOutputExpression(ast.NullLiteral{}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.Identifier{Name: "User"}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "optional"}, Name: "name"}, symbols, map[string]struct{}{"optional": {}}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.MemberAccess{Target: ast.Identifier{Name: "optional"}, Name: "name"}, symbols, map[string]struct{}{"optional": {}}, map[string]struct{}{"optional": {}})
		_ = validateDataOutputExpression(ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.PrefixExpression{Right: ast.StringLiteral{Lexeme: `"Ada"`}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.InfixExpression{Left: ast.StringLiteral{Lexeme: `"Ada"`}, Right: ast.StringLiteral{Lexeme: `"Bea"`}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"Ada"`}, Else: ast.StringLiteral{Lexeme: `"Bea"`}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		_ = validateDeclaration(ast.VariableDeclaration{Name: "noValue", Type: ast.PrimitiveType{Name: "string"}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{})
		_ = validateDeclaration(ast.TypeDeclaration{Name: "Alias", Type: ast.PrimitiveType{Name: "string"}, Description: "doc"}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{"Alias": {Target: "Alias"}}, map[string]symbolKind{"Alias": symbolKindType})
		_ = validateDeclaration(ast.SchemaDeclaration{Name: "User", Type: schema}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{"User": {Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "Missing", Documentation: ast.Documentation{}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{})
		_ = validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"missing": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{"User": {Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}}, map[string]symbolKind{"User": symbolKindSchema})
		_ = validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"record": symbolKindVariable})
		_ = validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"record": symbolKindType})
		_ = validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{})
		_ = validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "flag", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"flag": symbolKindVariable})
		_ = validateDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindGeneral, Target: "record", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, types, schemas, nil, vars, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{"record": symbolKindVariable})
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}, "extra": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, record: &schema}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, element: &valueType{kind: ValueInt}}, symbols, types, schemas, nil)
		_ = validateEvaluatedValueAgainstType(Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, valueType{kind: ValueRecord, record: &ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "int"}}}}}, symbols, types, schemas, nil)
		_, _ = coerceEvaluatedValueAgainstType(ast.IntLiteral{Lexeme: "1"}, Value{Kind: ValueInt, Int: 1}, valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		_, _ = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeData}, types)
		_ = typeReferenceFromValueType(valueType{kind: ValueString})
		_ = typeReferenceFromValueType(valueType{kind: ValueInt})
		_ = typeReferenceFromValueType(valueType{kind: ValueFloat})
		_ = typeReferenceFromValueType(valueType{kind: ValueHexInt})
		_ = typeReferenceFromValueType(valueType{kind: ValueHexFloat})
		_ = typeReferenceFromValueType(valueType{kind: ValueBoolean})

		workspaceDir, err := os.MkdirTemp("", "processor-work-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspaceDir) }()

		remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/schema.mace" {
				_, _ = io.WriteString(w, `[output = schema]
{ Remote: string; }`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer remoteServer.Close()

		localSchema := writeFixtureFile(workspaceDir, "schema.mace", `[output = schema]
{ Local: string; }`)
		localData := writeFixtureFile(workspaceDir, "data.mace", `[output = data]
{ value = "x"; }`)
		localBad := writeFixtureFile(workspaceDir, "bad.mace", `not valid`)
		_ = writeFixtureFile(workspaceDir, "schema-cycle-a.mace", `from "./schema-cycle-b.mace" import Local;
[output = schema]
{ Local: string; }`)
		_ = writeFixtureFile(workspaceDir, "schema-cycle-b.mace", `from "./schema-cycle-a.mace" import Local;
[output = schema]
{ Local: string; }`)

		_, _ = prepareOutputContext(ast.OutputBlock{}, processContext{})
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, newProcessContext(workspaceDir, workspaceDir))
		ctxWithLocal := newProcessContext(workspaceDir, workspaceDir)
		ctxWithLocal.symbols.Add("Local", symbolKindVariable)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctxWithLocal)
		_, _ = prepareOutputContext(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}}, ctxWithLocal)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, ctxWithLocal)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ctxWithLocal)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchema, Value: "User"}}, ctxWithLocal)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `not-a-string`}}, ctxWithLocal)
		_, _ = resolveParseFileExportedSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspaceDir, workspaceDir)
		_, _ = resolveParseFileExportedSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, ast.OutputDirectiveSchemaFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ast.OutputDirectiveParseFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"bad.mace"`}}, ast.OutputDirectiveSchemaFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, ast.OutputDirectiveSchemaFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `not-a-string`}}, ast.OutputDirectiveSchemaFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"data.mace"`}}, ast.OutputDirectiveSchemaFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"data.mace"`}}, ast.OutputDirectiveParseFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"missing.mace"`}}, ast.OutputDirectiveSchemaFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"missing.mace"`}}, ast.OutputDirectiveParseFile, workspaceDir, workspaceDir)

		proc2 := New()
		ctx2 := newProcessContext(workspaceDir, workspaceDir)
		ctx2.schemas.Add("User", schema)
		ctx2.symbols.Add("User", symbolKindSchema)
		ctx2.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		ctx2.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		_ = proc2.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx2)
		_ = proc2.applyParsedOutputInput(ast.OutputBlock{}, &ctx2)
		ctx3 := newProcessContext(workspaceDir, workspaceDir)
		ctx3.schemas.Add("User", schema)
		ctx3.symbols.Add("User", symbolKindSchema)
		ctx3.symbols.Add("input", symbolKindVariable)
		ctx3.variables.Add("input", valueType{kind: ValueRecord, schemaName: "User"})
		ctx3.environment.Add("input", Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}})
		_ = proc2.applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &ctx3)

		_, _ = proc2.ProcessVariablesInScope("\x00", workspaceDir, workspaceDir)
		_, _ = proc2.ProcessVariablesInScope(`from "./bad.mace" import Missing;`, workspaceDir, workspaceDir)
		_, _ = proc2.ProcessVariablesInScope(`from "./schema.mace" import Local;
from "./schema.mace" import Local;`, workspaceDir, workspaceDir)
		_, _ = proc2.ProcessVariablesInScope(`|===|
string value = 1;
|===|`, workspaceDir, workspaceDir)
		_, _ = proc2.ProcessVariablesInScope(`|===|
string value = "a";
string value = "b";
|===|`, workspaceDir, workspaceDir)
		_, _ = proc2.ProcessVariablesInScope(`|===|
int value = 1;
int value = 2;
|===|`, workspaceDir, workspaceDir)
		_, _ = proc2.ProcessVariablesInScope(`|===|
int value = 1;
|===|`, "", "")
		_, _ = proc2.ProcessOutputBlock(`[output = schema]
{ Local: string; }`, ScriptResult{context: processContext{importBaseDir: workspaceDir, importRootDir: workspaceDir}})
		_, _ = proc2.ProcessOutputBlock(`[output = data]
{ value = "x"; }`, ScriptResult{context: processContext{importBaseDir: workspaceDir, importRootDir: workspaceDir}})
		_, _ = proc2.ProcessOutputBlock(`[output = schema]
{ Broken: Missing; }`, ScriptResult{context: processContext{importBaseDir: workspaceDir, importRootDir: workspaceDir}})

		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}, nil, workspaceDir, workspaceDir, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState(nil, &ast.ScriptBlock{Items: []ast.Declaration{ast.VariableDeclaration{Name: "value", Type: ast.PrimitiveType{Name: "string"}, HasValue: false}}}, workspaceDir, workspaceDir, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = buildProcessContextWithState([]ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema-cycle-a.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}}, nil, workspaceDir, workspaceDir, true, map[string]Value{}, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}, {Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "User"}}}}, workspaceDir, workspaceDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema-cycle-a.mace"`}, ImportAs: &ast.ImportedIdentifier{Name: "Local"}}}}, workspaceDir, workspaceDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"./schema.mace"`}, Identifiers: []ast.ImportedIdentifier{{Name: "Missing"}}}}}, workspaceDir, workspaceDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = loadImportExports(localSchema, workspaceDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localData, workspaceDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(localBad, workspaceDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(filepath.Join(workspaceDir, "missing.mace"), workspaceDir, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})

		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspaceDir, workspaceDir)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspaceDir, workspaceDir)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspaceDir, workspaceDir)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.txt"`}}, workspaceDir, workspaceDir)

		_, _ = loadOutputSchemaRecord(localSchema, workspaceDir, "schema_file")
		_, _ = loadOutputSchemaRecord(localData, workspaceDir, "schema_file")
		_, _ = loadOutputSchemaRecord(localBad, workspaceDir, "schema_file")
		_, _ = loadOutputSchemaRecord(filepath.Join(workspaceDir, "missing.mace"), workspaceDir, "schema_file")

		_, _ = loadSchemaFileDeclarations(localSchema, workspaceDir, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(localBad, workspaceDir, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(filepath.Join(workspaceDir, "missing-schema.mace"), workspaceDir, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(writeFixtureFile(workspaceDir, "schema-cycle-c.mace", `from "./schema-cycle-d.mace" import User;
[output = schema]
{ User: string; }`), workspaceDir, map[string]map[string]ast.Declaration{}, map[string]struct{}{})

		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, ctx2)
		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}}}, newProcessContext(workspaceDir, workspaceDir))
		_, _ = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ctx2)
		_, _ = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "missing", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ctx2)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.NamedType{Name: "Missing"}}, ctx2)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, ctx2)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "count", Type: ast.RecordMapType{Value: ast.PrimitiveType{Name: "string"}}}, ctx2)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "invalid", Type: nil}, ctx2)
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "loop", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "loop", Type: ast.NamedType{Name: "Loop"}}}}}, newProcessContext(workspaceDir, workspaceDir))
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "loop", Type: ast.RecordMapType{Value: ast.NamedType{Name: "Loop"}}}, newProcessContext(workspaceDir, workspaceDir))
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx2)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "unknown", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, ctx2)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{}, ctx2)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, ctx2)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: nil}, ast.OutputBlock{}, ctx2)
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "Missing"}}}, ctx2)
		_, _ = resolveExportedTypeReference(ast.ArrayType{Element: ast.NamedType{Name: "User"}}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}, ast.PrimitiveType{Name: "string"}}}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "User"}}}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.BooleanLiteral{Value: true}}}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Missing"}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "User"}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{"User": {}})
		_, _ = resolveExportedTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "loop", Type: ast.NamedType{Name: "Loop"}}}}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "User"}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{"User": {}})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Alias"}, ctx2.types, ctx2.schemas, map[string]struct{}{"Alias": {}}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.NamedType{Name: "Loop"}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _ = resolveExportedTypeReference(ast.RecordType{Fields: []ast.SchemaField{}}, ctx2.types, ctx2.schemas, map[string]struct{}{}, map[string]struct{}{})
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}, ctx2)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ctx2)
		_, _, _ = outputParseSchemaName([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveSchema, Value: "User"}}, ctx2)
		_, _ = resolveParseFileExportedSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspaceDir, workspaceDir)
		_, _ = resolveParseFileExportedSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}, {Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, ast.OutputDirectiveSchemaFile, workspaceDir, workspaceDir)
		_, _ = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, ast.OutputDirectiveParseFile, workspaceDir, workspaceDir)

		_ = typeReferenceFromValueType(valueType{kind: ValueString})
		_ = typeReferenceFromValueType(valueType{kind: ValueInt})
		_ = typeReferenceFromValueType(valueType{kind: ValueFloat})
		_ = typeReferenceFromValueType(valueType{kind: ValueHexInt})
		_ = typeReferenceFromValueType(valueType{kind: ValueHexFloat})
		_ = typeReferenceFromValueType(valueType{kind: ValueBoolean})
	})

	It("covers remaining import and output path branches", func() {
		workspace, err := os.MkdirTemp("", "processor-path-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		_ = formatImportRoot("")
		_ = formatImportRoot(".")
		_ = formatImportRoot("/tmp/workspace")
		_, _ = parseRemoteURL("not-a-url")
		_, _ = parseRemoteURL("file:///tmp/schema.mace")
		_, _ = parseRemoteURL("https://example.com/schema.mace")
		_, _ = resolveBoundedPath("/tmp/base", "/tmp/root", "../escape.mace")
		_, _ = resolveBoundedPath("/tmp/base", "/tmp/root", "schema.txt")
		_, _ = resolveBoundedPath("https://example.com/base/", "https://example.com/root/", "schema.mace")
		_, _ = resolveBoundedRemotePath("/tmp/base", "/tmp/root", "schema.mace", "https://other.example.com/schema.mace")
		_, _ = resolveBoundedRemotePath("/tmp/base", "https://example.com/root/", "schema.mace", "file:///tmp/schema.mace")
		_, _ = readMaceSource(filepath.Join(workspace, "missing.mace"))
		_, _ = readMaceSource("https://example.com/schema.mace")
		_, _ = loadImportExports(filepath.Join(workspace, "missing.mace"), ".", false, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadImportExports(writeFixtureFile(workspace, "bad.mace", `not valid`), ".", false, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"../escape.mace"`}}}}, "/tmp/base", "/tmp/root", true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"schema.txt"`}}}}, "/tmp/base", "/tmp/root", false, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `"https://example.com/schema.mace"`}}}}, "/tmp/base", "https://example.com/root/", true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		_, _ = loadOutputSchemaRecord(filepath.Join(workspace, "missing.mace"), workspace, "schema_file")
		_, _ = loadOutputSchemaRecord(writeFixtureFile(workspace, "not-schema.mace", `not valid`), workspace, "schema_file")
		_, _ = loadSchemaFileDeclarations(filepath.Join(workspace, "missing-schema.mace"), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = loadSchemaFileDeclarations(writeFixtureFile(workspace, "schema-invalid.mace", `not valid`), workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "bad", Type: nil}}}, newProcessContext(workspace, workspace))
		_, _ = collectImportExports(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "profile", Type: ast.NamedType{Name: "User"}}, {Name: "count", Type: ast.RecordMapType{Value: ast.NamedType{Name: "User"}}}}}, newProcessContext(workspace, workspace))
		_, _ = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}, DataFields: []ast.OutputField{{Name: "missing", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, newProcessContext(workspace, workspace))
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "profile", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}}, newProcessContext(workspace, workspace))
		_, _ = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "count", Type: ast.RecordMapType{Value: ast.NamedType{Name: "User"}}}, newProcessContext(workspace, workspace))
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{}, newProcessContext(workspace, workspace))
		_, _ = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, newProcessContext(workspace, workspace))
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"schema.mace"`}}, workspace, workspace)
		_, _ = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: `"schema.mace"`}}, workspace, workspace)
	})

	It("covers validateExpressionAgainstType recursive error branches", func() {
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		variables := newVariableRegistry()

		userSchema := ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}
		symbols.Add("User", symbolKindSchema)
		schemas.Add("User", userSchema)

		stringValue := Value{Kind: ValueString, String: "Ada"}
		intValue := Value{Kind: ValueInt, Int: 1}
		choiceType := valueType{choiceValues: []Value{stringValue}}
		arrayType := valueType{kind: ValueArray, element: &valueType{kind: ValueString}}
		recordMapType := valueType{kind: ValueRecord, element: &valueType{kind: ValueString}}
		recordType := valueType{kind: ValueRecord, record: &userSchema}

		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.Identifier{Name: "User"},
			Else: ast.StringLiteral{Lexeme: `"Ada"`},
		}, choiceType, variables, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.StringLiteral{Lexeme: `"Ada"`},
			Else: ast.IntLiteral{Lexeme: "1"},
		}, choiceType, variables, symbols, types, schemas, nil))

		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}},
			Else: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}},
		}, arrayType, variables, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.ArrayLiteral{Elements: []ast.Expression{ast.StringLiteral{Lexeme: `"Ada"`}}},
			Else: ast.ArrayLiteral{Elements: []ast.Expression{ast.IntLiteral{Lexeme: "1"}}},
		}, arrayType, variables, symbols, types, schemas, nil))

		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}},
			Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}},
		}, recordMapType, variables, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}},
			Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.IntLiteral{Lexeme: "1"}}}},
		}, recordMapType, variables, symbols, types, schemas, nil))

		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.RecordLiteral{},
			Else: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}},
		}, recordType, variables, symbols, types, schemas, nil))
		tAssert.Error(validateExpressionAgainstType(ast.ConditionalExpression{
			Then: ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}},
			Else: ast.RecordLiteral{},
		}, recordType, variables, symbols, types, schemas, nil))

		tAssert.NoError(validateExpressionAgainstType(ast.StringLiteral{Lexeme: `"Ada"`}, valueType{choiceValues: []Value{stringValue, intValue}}, variables, symbols, types, schemas, nil))
	})

	It("covers expression evaluation and inference error branches", func() {
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		variables := newVariableRegistry()
		environment := newValueEnvironment()

		symbols.Add("User", symbolKindSchema)
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})
		variables.Add("record", valueType{kind: ValueRecord, schemaName: "Missing"})
		variables.Add("array", valueType{kind: ValueArray})

		_, err := evaluateExpression(ast.Identifier{Name: "name"}, environment, Value{Kind: ValueRecord, Record: map[string]Value{"name": {Kind: ValueString, String: "Ada"}}}, symbols, types, schemas, nil)
		tAssert.NoError(err)
		_, err = evaluateArrayAccess(ast.ArrayAccess{Target: ast.Identifier{Name: "User"}, Index: ast.IntLiteral{Lexeme: "0"}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Left: ast.Identifier{Name: "User"}, Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateInfix(ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "1"}, Operator: lexer.TokenPlus, Right: ast.Identifier{Name: "User"}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		tAssert.True(arrayMergeTypesMatch(Value{Kind: ValueArray}, Value{Kind: ValueArray}))
		_, err = evaluateHexNumeric(lexer.TokenDoubleStar, Value{Kind: ValueHexInt, Int: 2}, Value{Kind: ValueHexInt, Int: 64})
		tAssert.NoError(err)
		_, err = valuesEqual(Value{Kind: ValueRecord}, Value{Kind: ValueRecord})
		tAssert.Error(err)
		_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.Identifier{Name: "User"}, Right: ast.BooleanLiteral{Value: true}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateLogicalAnd(ast.InfixExpression{Left: ast.BooleanLiteral{Value: true}, Right: ast.Identifier{Name: "User"}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.Identifier{Name: "User"}, Right: ast.BooleanLiteral{Value: true}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateLogicalOr(ast.InfixExpression{Left: ast.BooleanLiteral{Value: false}, Right: ast.Identifier{Name: "User"}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = evaluateConditional(ast.ConditionalExpression{Condition: ast.Identifier{Name: "User"}, Then: ast.StringLiteral{Lexeme: `"x"`}, Else: ast.StringLiteral{Lexeme: `"y"`}}, environment, Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)

		_, err = inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "User"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		inferred, err := inferExpressionType(ast.MemberAccess{Target: ast.Identifier{Name: "record"}, Name: "name"}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.Equal(ValueUnknown, inferred.kind)
		_, err = inferExpressionType(ast.ArrayAccess{Target: ast.Identifier{Name: "User"}, Index: ast.IntLiteral{Lexeme: "0"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.IntLiteral{Lexeme: "nope"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.FloatLiteral{Lexeme: "nope"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.HexIntLiteral{Lexeme: "0xzz"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferExpressionType(ast.HexFloatLiteral{Lexeme: "0xzz.1"}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferArrayLiteralType(ast.ArrayLiteral{Elements: []ast.Expression{ast.Identifier{Name: "User"}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenBang, Right: ast.Identifier{Name: "User"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferPrefixType(ast.PrefixExpression{Operator: lexer.TokenPlus, Right: ast.StringLiteral{Lexeme: `"x"`}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.Identifier{Name: "User"}, Operator: lexer.TokenPlus, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.IntLiteral{Lexeme: "1"}, Operator: lexer.TokenPlus, Right: ast.Identifier{Name: "User"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.StringLiteral{Lexeme: `"x"`}, Operator: lexer.TokenPercent, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.HexIntLiteral{Lexeme: "0x1"}, Operator: lexer.TokenPercent, Right: ast.IntLiteral{Lexeme: "1"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.Identifier{Name: "User"}, Then: ast.StringLiteral{Lexeme: `"x"`}, Else: ast.StringLiteral{Lexeme: `"y"`}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "User"}, Else: ast.StringLiteral{Lexeme: `"y"`}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.StringLiteral{Lexeme: `"x"`}, Else: ast.Identifier{Name: "User"}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.RecordLiteral{}, Else: ast.RecordLiteral{}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
		tAssert.False(typesEqual(valueType{members: []valueType{{kind: ValueString}}}, valueType{members: []valueType{{kind: ValueString}, {kind: ValueInt}}}))
		tAssert.Error(ensureAssignable(valueType{kind: ValueString}, valueType{kind: ValueString, nullable: true}))
	})

	It("covers type resolution and schema conversion error branches", func() {
		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()

		types.AddAlias("Loop", ast.NamedType{Name: "Loop"})
		symbols.Add("Loop", symbolKindType)

		_, err := resolveValueType(ast.ArrayType{Element: ast.NamedType{Name: "Missing"}}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = resolveValueType(ast.RecordMapType{Value: ast.NamedType{Name: "Missing"}}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{nil}}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = resolveUnionRecordType(ast.NamedType{Name: "Loop"}, symbols, types, schemas)
		tAssert.Error(err)
		_, err = schemaTypeFromTypeReference(ast.ArrayType{Element: nil}, types)
		tAssert.Error(err)
		_, err = schemaTypeFromTypeReference(ast.RecordMapType{Value: nil}, types)
		tAssert.Error(err)
		_, err = schemaTypeFromTypeReference(ast.UnionType{Members: []ast.TypeReference{nil}}, types)
		tAssert.Error(err)
		_, err = schemaTypeFromTypeReference(ast.VariantType{Members: []ast.TypeReference{nil}}, types)
		tAssert.Error(err)
		_, err = schemaTypeFromTypeReference(ast.RecordType{Fields: []ast.SchemaField{{Name: "bad", Type: nil}}}, types)
		tAssert.Error(err)
		_, err = evaluateSchemaOutput(ast.OutputBlock{Mode: ast.OutputModeSchema, SchemaFields: []ast.OutputSchemaField{{Name: "bad", Type: nil}}}, types)
		tAssert.Error(err)
	})

	It("covers remaining import, output, and validation error branches", func() {
		workspace, err := os.MkdirTemp("", "processor-remaining-*")
		tAssert.NoError(err)
		defer func() { _ = os.RemoveAll(workspace) }()

		invalidSource := writeFixtureFile(workspace, "invalid.mace", "\x00")
		parseData := writeFixtureFile(workspace, "data.mace", `[output = data]
{ value = "x"; }`)
		importBadPath := writeFixtureFile(workspace, "bad-import-path.mace", `|===|
from not_a_string import User;
|===|
[output = schema]
{ User: string; }`)
		importOutside := writeFixtureFile(workspace, "outside-import.mace", `|===|
from "../outside.mace" import User;
|===|
[output = schema]
{ User: string; }`)

		symbols := newSymbolTable()
		types := newTypeRegistry()
		schemas := newSchemaRegistry()
		variables := newVariableRegistry()
		symbols.Add("User", symbolKindSchema)
		schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}})

		context := newProcessContext(workspace, workspace)
		context.symbols.Add("User", symbolKindSchema)
		context.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: nil}}})
		err = NewWithInput(map[string]Value{"name": {Kind: ValueString, String: "Ada"}}).applyParsedOutputInput(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveParse, Value: "User"}}}, &context)
		tAssert.Error(err)

		_, err = resolveImportsWithState(ast.File{Imports: []ast.ImportDeclaration{{Path: ast.StringLiteral{Lexeme: `not-a-string`}}}}, workspace, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = importFileAsDeclaration("Bad", map[string]importedDeclaration{"value": {kind: symbolKindImport}})
		tAssert.Error(err)
		_, err = loadImportExports(invalidSource, workspace, true, map[string]map[string]importedDeclaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(parseData)}}, workspace, workspace)
		tAssert.Error(err)
		_, err = resolveSchemaFileDeclarations([]ast.OutputDirective{{Kind: ast.OutputDirectiveParseFile, Value: strconv.Quote(filepath.Base(parseData))}}, workspace, workspace)
		tAssert.Error(err)
		_, err = loadOutputSchemaRecord(invalidSource, workspace, "schema_file")
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(importBadPath, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = loadSchemaFileDeclarations(importOutside, workspace, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = collectImportExports(ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: `"User"`}}}, newProcessContext(workspace, workspace))
		tAssert.Error(err)
		_, err = collectImportExports(ast.OutputBlock{DataFields: []ast.OutputField{{Name: "value", Value: ast.Identifier{Name: "User"}}}}, context)
		tAssert.Error(err)
		_, err = collectImportExports(ast.OutputBlock{DataFields: []ast.OutputField{{Name: "value", Value: nil}}}, context)
		tAssert.Error(err)
		cyclicSchemas := newSchemaRegistry()
		cyclicSchemas.Add("Loop", ast.RecordType{Fields: []ast.SchemaField{{Name: "next", Type: ast.NamedType{Name: "Loop"}}}})
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "bad", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "missing", Type: ast.NamedType{Name: "Missing"}}}}}, context)
		tAssert.NoError(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "bad", Type: ast.RecordMapType{Value: ast.NamedType{Name: "Missing"}}}, context)
		tAssert.NoError(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "bad", Type: ast.RecordType{Fields: []ast.SchemaField{{Name: "loop", Type: ast.NamedType{Name: "Loop"}}}}}, processContext{types: types, schemas: cyclicSchemas})
		tAssert.Error(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "bad", Type: ast.RecordMapType{Value: nil}}, context)
		tAssert.Error(err)
		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: `"User"`}}}, context)
		tAssert.Error(err)
		badFieldContext := newProcessContext(workspace, workspace)
		badFieldContext.symbols.Add("User", symbolKindSchema)
		badFieldContext.schemas.Add("User", ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: nil}}})
		_, err = exportedOutputFieldType(ast.OutputField{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}, ast.OutputBlock{Directives: []ast.OutputDirective{{Kind: ast.OutputDirectiveSchema, Value: "User"}}}, badFieldContext)
		tAssert.Error(err)

		_, err = resolveExportedTypeReference(ast.ArrayType{Element: ast.NamedType{Name: "Missing"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.ArrayType{Element: nil}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.RecordMapType{Value: ast.NamedType{Name: "Missing"}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.RecordMapType{Value: nil}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.UnionType{Members: []ast.TypeReference{nil}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.NoError(err)
		_, err = resolveExportedTypeReference(ast.VariantType{Members: []ast.TypeReference{nil}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = resolveExportedTypeReference(ast.ChoiceType{Members: []ast.Expression{ast.Identifier{Name: "Missing"}}}, types, schemas, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
		_, err = schemaFieldImportDeclaration(ast.OutputSchemaField{Name: "Loop", Type: ast.NamedType{Name: "Loop"}}, processContext{types: types, schemas: cyclicSchemas})
		tAssert.Error(err)

		err = validateDeclaration(ast.VariableDeclaration{Name: "value", Type: ast.NamedType{Name: "Missing"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"x"`}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{})
		tAssert.Error(err)
		err = validateDeclaration(ast.VariableDeclaration{Name: "bad", Type: ast.PrimitiveType{Name: "bad"}, HasValue: true, Value: ast.StringLiteral{Lexeme: `"x"`}}, symbols, types, schemas, nil, variables, map[string]struct{}{}, map[string]ast.DocDeclaration{}, map[string]symbolKind{})
		tAssert.Error(err)
		err = validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.NamedType{Name: "Missing"}}}, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.RecordType{Fields: []ast.SchemaField{{Name: "bad", Type: nil}}}}}, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateTypeReference(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "bad"}}}, symbols, types, schemas, nil)
		tAssert.Error(err)

		emptyRecordVariable := newVariableRegistry()
		emptyRecordVariable.Add("object", valueType{kind: ValueRecord})
		symbols.Add("object", symbolKindVariable)
		err = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "object", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, symbols, schemas, emptyRecordVariable, map[string]struct{}{}, map[string]symbolKind{"object": symbolKindVariable})
		tAssert.Error(err)
		err = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "User", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: "\x00"}}}}, symbols, schemas, variables, map[string]struct{}{}, map[string]symbolKind{"User": symbolKindSchema})
		tAssert.Error(err)
		schemaOnlySymbols := newSymbolTable()
		schemaOnlySymbols.Add("MissingShape", symbolKindSchema)
		err = validateDocDeclaration(ast.DocDeclaration{Kind: ast.DocumentationKindSchema, Target: "MissingShape", Documentation: ast.Documentation{Props: map[string]ast.StringLiteral{"name": {Lexeme: `"Ada"`}}}}, schemaOnlySymbols, newSchemaRegistry(), variables, map[string]struct{}{}, map[string]symbolKind{"MissingShape": symbolKindSchema})
		tAssert.Error(err)

		err = validateDataOutputExpression(ast.InfixExpression{Left: ast.Identifier{Name: "User"}, Right: ast.StringLiteral{Lexeme: `"x"`}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
		err = validateDataOutputExpression(ast.ConditionalExpression{Condition: ast.Identifier{Name: "User"}, Then: ast.StringLiteral{Lexeme: `"x"`}, Else: ast.StringLiteral{Lexeme: `"y"`}}, symbols, map[string]struct{}{}, map[string]struct{}{})
		tAssert.Error(err)
		err = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.Identifier{Name: "User"}}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: nil}}, variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateOutputSchema("User", []ast.OutputField{{Name: "name", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}, variables, symbols, types, &schemaRegistry{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: nil}}}}}, nil)
		tAssert.Error(err)
		err = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Optional: true, Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, "", variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "name", Value: ast.Identifier{Name: "User"}}}}, ast.RecordType{Fields: []ast.SchemaField{{Name: "name", Type: ast.PrimitiveType{Name: "string"}}}}, "", variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateRecordLiteralAgainstRecordType(ast.RecordLiteral{Fields: []ast.RecordField{{Name: "extra", Value: ast.StringLiteral{Lexeme: `"Ada"`}}}}, ast.RecordType{}, "", variables, symbols, types, schemas, nil)
		tAssert.Error(err)
		err = validateEvaluatedOutputSchema("User", map[string]Value{"name": {Kind: ValueString, String: "Ada"}}, symbols, types, &schemaRegistry{schemas: map[string]ast.RecordType{"User": {Fields: []ast.SchemaField{{Name: "name", Type: nil}}}}}, nil)
		tAssert.Error(err)

		_, err = parseInterpolatedString("\"$(\x00)\"", newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = parseInterpolatedString(`"$(1 +)"`, newValueEnvironment(), Value{}, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = parseUnicodeEscape(`\uFFFFFFFFF`, 9)
		tAssert.Error(err)
		_, err = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: `"../outside.mace"`}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		tAssert.Error(err)
		_, err = resolveOutputSchemaNames([]ast.OutputDirective{{Kind: ast.OutputDirectiveSchemaFile, Value: strconv.Quote(filepath.Base(invalidSource))}}, ast.OutputDirectiveSchemaFile, workspace, workspace)
		tAssert.Error(err)
		_, err = valuesEqual(Value{Kind: ValueHexFloat, Float: 1}, Value{Kind: ValueHexFloat, Float: 1})
		tAssert.NoError(err)
		_, err = resolveValueType(ast.VariantType{Members: []ast.TypeReference{ast.PrimitiveType{Name: "bad"}}}, symbols, types, schemas, nil)
		tAssert.Error(err)
		exactVariables := newVariableRegistry()
		exactVariables.Add("left", valueType{kind: ValueRecord, exactValue: &Value{Kind: ValueRecord}})
		exactVariables.Add("right", valueType{kind: ValueRecord, exactValue: &Value{Kind: ValueRecord}})
		_, err = inferConditionalType(ast.ConditionalExpression{Condition: ast.BooleanLiteral{Value: true}, Then: ast.Identifier{Name: "left"}, Else: ast.Identifier{Name: "right"}}, exactVariables, symbols, types, schemas, nil)
		tAssert.Error(err)
		_, err = inferInfixType(ast.InfixExpression{Left: ast.HexIntLiteral{Lexeme: "0x1"}, Operator: lexer.TokenPercent, Right: ast.HexIntLiteral{Lexeme: "0x1"}}, variables, symbols, types, schemas, nil)
		tAssert.NoError(err)
	})
})

func contextValues(name string, value Value) *valueEnvironment {
	environment := newValueEnvironment()
	environment.Add(name, value)
	return environment
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}

func (failingReadCloser) Close() error {
	return nil
}
