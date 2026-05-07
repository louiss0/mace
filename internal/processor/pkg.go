package processor

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/samber/lo"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/parser"
	"github.com/louiss0/mace/internal/parser/ast"
)

type Processor struct {
	injections map[string]Value
}

type Result struct {
	File   ast.File
	Output map[string]Value
	Schema map[SchemaField]SchemaType
}

type ScriptResult struct {
	Script    ast.ScriptBlock
	Variables map[string]Value
	context   processContext
}

type SchemaField struct {
	Name     string
	Optional bool
}

type SchemaTypeKind int

const (
	SchemaTypeUnknown SchemaTypeKind = iota
	SchemaTypePrimitive
	SchemaTypeNamed
	SchemaTypeArray
	SchemaTypeRecord
	SchemaTypeUnion
	SchemaTypeVariant
)

type SchemaType struct {
	Kind    SchemaTypeKind
	Name    string
	Element *SchemaType
	Fields  map[SchemaField]SchemaType
	Members []SchemaType
}

type processContext struct {
	baseDir     string
	symbols     *symbolTable
	types       *typeRegistry
	schemas     *schemaRegistry
	enums       *enumRegistry
	variables   *variableRegistry
	environment *valueEnvironment
}

func newProcessContext(baseDir string) processContext {
	return processContext{
		baseDir:     baseDir,
		symbols:     newSymbolTable(),
		types:       newTypeRegistry(),
		schemas:     newSchemaRegistry(),
		enums:       newEnumRegistry(),
		variables:   newVariableRegistry(),
		environment: newValueEnvironment(),
	}
}

func (context processContext) clone() processContext {
	if context.symbols == nil {
		return processContext{}
	}

	return processContext{
		baseDir:     context.baseDir,
		symbols:     context.symbols.Clone(),
		types:       context.types.Clone(),
		schemas:     context.schemas.Clone(),
		enums:       context.enums.Clone(),
		variables:   context.variables.Clone(),
		environment: context.environment.Clone(),
	}
}

func New() *Processor {
	return &Processor{injections: map[string]Value{}}
}

func NewWithInjections(injections map[string]Value) *Processor {
	cloned := make(map[string]Value, len(injections))
	for name, value := range injections {
		cloned[name] = value
	}

	return &Processor{injections: cloned}
}

func (p *Processor) Process(input string) (Result, error) {
	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}

	return p.ProcessInDir(input, baseDir)
}

func (p *Processor) ProcessInDir(input string, baseDir string) (Result, error) {
	if baseDir == "" {
		baseDir = "."
	}

	return p.processInput(input, baseDir)
}

func (p *Processor) ProcessScriptBlock(input string) (ScriptResult, error) {
	baseDir, err := os.Getwd()
	if err != nil {
		baseDir = "."
	}

	return p.processScriptInput(input, baseDir)
}

func (p *Processor) ProcessVariablesInDir(input string, baseDir string) (map[string]Value, error) {
	if baseDir == "" {
		baseDir = "."
	}

	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return nil, err
	}

	context, err := buildProcessContext(file.Imports, file.Script, baseDir, p.injections)
	if err != nil {
		return nil, err
	}

	return context.environment.Values(), nil
}

func (p *Processor) ProcessOutputBlock(input string, scriptResult ScriptResult) (Result, error) {
	baseDir := scriptResult.context.baseDir
	if baseDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			baseDir = "."
		} else {
			baseDir = cwd
		}
	}

	return p.processOutputInput(input, scriptResult, baseDir)
}

func (p *Processor) ProcessFile(path string) (Result, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Result{}, validationErrorf("unable to read file %q", path)
	}

	return p.processInput(string(contents), filepath.Dir(path))
}

func ParseInjectionRecord(input string) (map[string]Value, error) {
	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}

	expression, err := parser.New(tokens).ParseExpression()
	if err != nil {
		return nil, err
	}

	value, err := evaluateExpression(expression, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), newEnumRegistry())
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueRecord {
		return nil, validationErrorf("injection input must be a record literal")
	}

	return value.Record, nil
}

func (p *Processor) processInput(input string, baseDir string) (Result, error) {
	tokens, err := lex(input)
	if err != nil {
		return Result{}, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return Result{}, err
	}

	context, err := buildProcessContext(file.Imports, file.Script, baseDir, p.injections)
	if err != nil {
		return Result{}, err
	}

	return p.processParsedOutput(file.Output, file, context)
}

func (p *Processor) processScriptInput(input string, baseDir string) (ScriptResult, error) {
	tokens, err := lex(input)
	if err != nil {
		return ScriptResult{}, err
	}

	script, err := parser.New(tokens).ParseScriptBlock()
	if err != nil {
		return ScriptResult{}, err
	}

	context, err := buildProcessContext(script.Imports, &script, baseDir, p.injections)
	if err != nil {
		return ScriptResult{}, err
	}

	return ScriptResult{
		Script:    script,
		Variables: context.environment.Values(),
		context:   context,
	}, nil
}

func (p *Processor) processOutputInput(input string, scriptResult ScriptResult, baseDir string) (Result, error) {
	tokens, err := lex(input)
	if err != nil {
		return Result{}, err
	}

	outputBlock, err := parser.New(tokens).ParseOutputBlock()
	if err != nil {
		return Result{}, err
	}

	context := scriptResult.context
	if context.symbols == nil {
		context = newProcessContext(baseDir)
	} else {
		context = context.clone()
		context.baseDir = baseDir
	}

	file := ast.File{
		Script: &scriptResult.Script,
		Output: outputBlock,
	}

	return p.processParsedOutput(outputBlock, file, context)
}

func (p *Processor) processParsedOutput(outputBlock ast.OutputBlock, file ast.File, context processContext) (Result, error) {
	outputContext, err := prepareOutputContext(outputBlock, context)
	if err != nil {
		return Result{}, err
	}

	if outputBlock.Mode == ast.OutputModeSchema {
		if err := validateSchemaOutputScriptVariables(file); err != nil {
			return Result{}, err
		}
		if err := validateSchemaOutputFields(outputBlock.SchemaFields, outputContext.symbols, outputContext.types, outputContext.schemas, outputContext.enums); err != nil {
			return Result{}, err
		}
		schema, err := evaluateSchemaOutput(outputBlock)
		if err != nil {
			return Result{}, err
		}

		return Result{File: file, Output: map[string]Value{}, Schema: schema}, nil
	}

	if err := validateDataOutputFields(outputBlock.DataFields, outputContext.symbols); err != nil {
		return Result{}, err
	}

	if schemaName, ok := outputSchemaName(outputBlock.Directives); ok {
		if err := validateOutputSchema(schemaName, outputBlock.DataFields, outputContext.variables, outputContext.symbols, outputContext.types, outputContext.schemas, outputContext.enums); err != nil {
			return Result{}, err
		}
	}

	output, err := evaluateOutputFields(outputBlock.DataFields, outputContext.environment, outputContext.symbols, outputContext.types, outputContext.schemas, outputContext.enums)
	if err != nil {
		return Result{}, err
	}

	if schemaName, ok := outputSchemaName(outputBlock.Directives); ok {
		if err := validateEvaluatedOutputSchema(schemaName, output, outputContext.symbols, outputContext.types, outputContext.schemas, outputContext.enums); err != nil {
			return Result{}, err
		}
	}

	return Result{File: file, Output: output, Schema: map[SchemaField]SchemaType{}}, nil
}

func validateSchemaOutputScriptVariables(file ast.File) error {
	if file.Script == nil {
		return nil
	}

	for _, item := range file.Script.Items {
		declaration, ok := item.(ast.VariableDeclaration)
		if !ok {
			continue
		}
		return validationErrorf("script variable %q is not allowed when output = schema", declaration.Name)
	}

	return nil
}

func lex(input string) ([]lexer.Token, error) {
	lexerInstance := lexer.New(input)
	tokens := []lexer.Token{}

	for {
		token, err := lexerInstance.NextToken()
		if err != nil {
			return nil, err
		}

		tokens = append(tokens, token)
		if token.Type == lexer.TokenEOF {
			return tokens, nil
		}
	}
}

func buildProcessContext(imports []ast.ImportDeclaration, script *ast.ScriptBlock, baseDir string, injections map[string]Value) (processContext, error) {
	return buildProcessContextWithState(
		imports,
		script,
		baseDir,
		injections,
		map[string]map[string]importedDeclaration{},
		map[string]struct{}{},
	)
}

func buildProcessContextWithState(imports []ast.ImportDeclaration, script *ast.ScriptBlock, baseDir string, injections map[string]Value, cache map[string]map[string]importedDeclaration, stack map[string]struct{}) (processContext, error) {
	context := newProcessContext(baseDir)

	imported, err := resolveImportsWithState(ast.File{Imports: imports}, baseDir, cache, stack)
	if err != nil {
		return processContext{}, err
	}

	for _, importedDecl := range imported {
		if context.symbols.Has(importedDecl.name) {
			return processContext{}, validationErrorf("duplicate import %q", importedDecl.name)
		}
		switch importedDecl.kind {
		case symbolKindType:
			context.symbols.Add(importedDecl.name, symbolKindType)
			context.types.AddAlias(importedDecl.name, importedDecl.typeRef)
		case symbolKindSchema:
			context.symbols.Add(importedDecl.name, symbolKindSchema)
			context.schemas.Add(importedDecl.name, importedDecl.record)
		case symbolKindVariable:
			context.symbols.Add(importedDecl.name, symbolKindVariable)
			context.variables.Add(importedDecl.name, importedDecl.vtype)
			context.environment.Add(importedDecl.name, importedDecl.value)
		case symbolKindEnum:
			context.symbols.Add(importedDecl.name, symbolKindEnum)
			context.enums.Add(importedDecl.name, importedDecl.enumDef)
		default:
			return processContext{}, validationErrorf("unknown import %q", importedDecl.name)
		}
	}

	if script != nil {
		if err := collectDeclarations(script.Items, context.symbols, context.types, context.schemas, context.enums); err != nil {
			return processContext{}, err
		}
		if err := validateDeclarations(script.Items, context.symbols, context.types, context.schemas, context.enums, context.variables, injections); err != nil {
			return processContext{}, err
		}
		if err := validateInjections(script.Items, injections); err != nil {
			return processContext{}, err
		}
		if err := evaluateScript(script.Items, context.environment, injections, context.symbols, context.types, context.schemas, context.enums); err != nil {
			return processContext{}, err
		}
	}

	return context, nil
}

func prepareOutputContext(output ast.OutputBlock, context processContext) (processContext, error) {
	outputContext := context.clone()
	if outputContext.symbols == nil {
		outputContext = newProcessContext(context.baseDir)
	}

	if err := validateOutputDirectiveStructure(output); err != nil {
		return processContext{}, err
	}

	schemaFileDeclarations, err := resolveSchemaFileDeclarations(output.Directives, outputContext.baseDir)
	if err != nil {
		return processContext{}, err
	}

	for _, declaration := range schemaFileDeclarations {
		if outputContext.symbols.Has(declaration.name) {
			return processContext{}, validationErrorf("duplicate declaration %q", declaration.name)
		}

		switch declaration.kind {
		case symbolKindType:
			outputContext.symbols.Add(declaration.name, symbolKindType)
			outputContext.types.AddAlias(declaration.name, declaration.typeRef)
		case symbolKindSchema:
			outputContext.symbols.Add(declaration.name, symbolKindSchema)
			outputContext.schemas.Add(declaration.name, declaration.record)
		case symbolKindEnum:
			outputContext.symbols.Add(declaration.name, symbolKindEnum)
			outputContext.enums.Add(declaration.name, declaration.enumDef)
		default:
			return processContext{}, validationErrorf("unknown declaration %q in schema_file", declaration.name)
		}
	}

	if err := validateOutputDirectiveReferences(output, outputContext.symbols); err != nil {
		return processContext{}, err
	}

	return outputContext, nil
}

type importedDeclaration struct {
	name    string
	kind    symbolKind
	typeRef ast.TypeReference
	record  ast.RecordType
	enumDef enumDefinition
	value   Value
	vtype   valueType
}

func resolveImportsWithState(file ast.File, baseDir string, cache map[string]map[string]importedDeclaration, stack map[string]struct{}) ([]importedDeclaration, error) {
	if len(file.Imports) == 0 {
		return nil, nil
	}

	imports := map[string]importedDeclaration{}

	for _, importDecl := range file.Imports {
		path, err := parseImportPath(importDecl.Path)
		if err != nil {
			return nil, err
		}

		resolvedPath := filepath.Join(baseDir, path)
		declarations, err := loadImportExports(resolvedPath, cache, stack)
		if err != nil {
			return nil, err
		}

		for _, name := range importDecl.Identifiers {
			if _, exists := imports[name]; exists {
				return nil, validationErrorf("duplicate import %q", name)
			}

			decl, ok := declarations[name]
			if !ok {
				return nil, validationErrorf("imported identifier %q not found in %q", name, path)
			}

			imports[name] = decl
		}
	}

	imported := make([]importedDeclaration, 0, len(imports))
	for _, item := range imports {
		imported = append(imported, item)
	}
	return imported, nil
}

func parseImportPath(literal ast.StringLiteral) (string, error) {
	value, err := parseStaticString(literal.Lexeme)
	if err != nil {
		return "", err
	}
	return value.String, nil
}

func loadImportExports(path string, cache map[string]map[string]importedDeclaration, stack map[string]struct{}) (map[string]importedDeclaration, error) {
	if declarations, ok := cache[path]; ok {
		return declarations, nil
	}
	if _, ok := stack[path]; ok {
		return nil, validationErrorf("circular import detected at %q", path)
	}

	stack[path] = struct{}{}
	defer delete(stack, path)

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, validationErrorf("unable to read import file %q", path)
	}

	tokens, err := lex(string(contents))
	if err != nil {
		return nil, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return nil, validationErrorf("unable to parse import file %q: %s", path, err)
	}

	context, err := buildProcessContextWithState(
		file.Imports,
		file.Script,
		filepath.Dir(path),
		map[string]Value{},
		cache,
		stack,
	)
	if err != nil {
		return nil, err
	}

	declarations, err := collectImportExports(file.Output, context)
	if err != nil {
		return nil, err
	}
	cache[path] = declarations
	return declarations, nil
}

func resolveSchemaFileDeclarations(directives []ast.OutputDirective, baseDir string) ([]importedDeclaration, error) {
	var path string
	for _, directive := range directives {
		if directive.Kind != ast.OutputDirectiveSchemaFile {
			continue
		}

		if path != "" {
			return nil, validationErrorf("duplicate output directive %q", directiveKindName(directive.Kind))
		}

		parsedPath, err := parseStaticString(directive.Value)
		if err != nil {
			return nil, err
		}

		path = parsedPath.String
	}

	if path == "" {
		return nil, nil
	}

	resolvedPath := filepath.Join(baseDir, path)
	declarations, err := loadSchemaFileDeclarations(resolvedPath, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
	if err != nil {
		return nil, err
	}

	outputDeclarations := []importedDeclaration{}
	for name, declaration := range declarations {
		switch typedDeclaration := declaration.(type) {
		case ast.TypeDeclaration:
			outputDeclarations = append(outputDeclarations, importedDeclaration{
				name:    name,
				kind:    symbolKindType,
				typeRef: typedDeclaration.Type,
			})
		case ast.SchemaDeclaration:
			outputDeclarations = append(outputDeclarations, importedDeclaration{
				name:   name,
				kind:   symbolKindSchema,
				record: typedDeclaration.Type,
			})
		case ast.EnumDeclaration:
			enumDef, err := enumDefinitionFromDeclaration(typedDeclaration)
			if err != nil {
				return nil, err
			}

			outputDeclarations = append(outputDeclarations, importedDeclaration{
				name:    name,
				kind:    symbolKindEnum,
				enumDef: enumDef,
			})
		}
	}

	return outputDeclarations, nil
}

func loadSchemaFileDeclarations(path string, cache map[string]map[string]ast.Declaration, stack map[string]struct{}) (map[string]ast.Declaration, error) {
	if declarations, ok := cache[path]; ok {
		return declarations, nil
	}
	if _, ok := stack[path]; ok {
		return nil, validationErrorf("circular import detected at %q", path)
	}

	stack[path] = struct{}{}
	defer delete(stack, path)

	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, validationErrorf("unable to read import file %q", path)
	}

	tokens, err := lex(string(contents))
	if err != nil {
		return nil, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return nil, validationErrorf("unable to parse import file %q: %s", path, err)
	}

	for _, importDecl := range file.Imports {
		importPath, err := parseImportPath(importDecl.Path)
		if err != nil {
			return nil, err
		}

		resolvedPath := filepath.Join(filepath.Dir(path), importPath)
		if _, err := loadSchemaFileDeclarations(resolvedPath, cache, stack); err != nil {
			return nil, err
		}
	}

	declarations := map[string]ast.Declaration{}
	if file.Script != nil {
		for _, declaration := range file.Script.Items {
			switch typedDecl := declaration.(type) {
			case ast.VariableDeclaration:
				declarations[typedDecl.Name] = typedDecl
			case ast.TypeDeclaration:
				declarations[typedDecl.Name] = typedDecl
			case ast.SchemaDeclaration:
				declarations[typedDecl.Name] = typedDecl
			case ast.EnumDeclaration:
				declarations[typedDecl.Name] = typedDecl
			}
		}
	}

	cache[path] = declarations
	return declarations, nil
}

func collectImportExports(output ast.OutputBlock, context processContext) (map[string]importedDeclaration, error) {
	outputContext, err := prepareOutputContext(output, context)
	if err != nil {
		return nil, err
	}

	if output.Mode == ast.OutputModeSchema {
		if err := validateSchemaOutputFields(output.SchemaFields, outputContext.symbols, outputContext.types, outputContext.schemas, outputContext.enums); err != nil {
			return nil, err
		}

		exports := make(map[string]importedDeclaration, len(output.SchemaFields))
		for _, field := range output.SchemaFields {
			exported, err := schemaFieldImportDeclaration(field, outputContext)
			if err != nil {
				return nil, err
			}

			exports[field.Name] = exported
		}

		return exports, nil
	}

	if schemaName, ok := outputSchemaName(output.Directives); ok {
		if err := validateOutputSchema(schemaName, output.DataFields, outputContext.variables, outputContext.symbols, outputContext.types, outputContext.schemas, outputContext.enums); err != nil {
			return nil, err
		}
	}

	values, err := evaluateOutputFields(output.DataFields, outputContext.environment, outputContext.symbols, outputContext.types, outputContext.schemas, outputContext.enums)
	if err != nil {
		return nil, err
	}

	exports := make(map[string]importedDeclaration, len(output.DataFields))
	for _, field := range output.DataFields {
		exportedType, err := exportedOutputFieldType(field, output, outputContext)
		if err != nil {
			return nil, err
		}

		exports[field.Name] = importedDeclaration{
			name:  field.Name,
			kind:  symbolKindVariable,
			value: values[field.Name],
			vtype: exportedType,
		}
	}

	return exports, nil
}

func schemaFieldImportDeclaration(field ast.OutputSchemaField, context processContext) (importedDeclaration, error) {
	switch typedRef := field.Type.(type) {
	case ast.NamedType:
		if record, ok := context.schemas.Get(typedRef.Name); ok {
			exportedRecord, err := resolveExportedRecordType(record, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
			if err != nil {
				return importedDeclaration{}, err
			}

			return importedDeclaration{
				name:   field.Name,
				kind:   symbolKindSchema,
				record: exportedRecord,
			}, nil
		}

		enumDef, ok, err := resolveExportedEnumDefinition(typedRef, context.types, context.enums)
		if err != nil {
			return importedDeclaration{}, err
		}
		if ok {
			return importedDeclaration{
				name:    field.Name,
				kind:    symbolKindEnum,
				enumDef: enumDef.Rename(field.Name),
			}, nil
		}
	case ast.RecordType:
		exportedRecord, err := resolveExportedRecordType(typedRef, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		if err != nil {
			return importedDeclaration{}, err
		}

		return importedDeclaration{
			name:   field.Name,
			kind:   symbolKindSchema,
			record: exportedRecord,
		}, nil
	}

	exportedType, err := resolveExportedTypeReference(field.Type, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
	if err != nil {
		return importedDeclaration{}, err
	}

	return importedDeclaration{
		name:    field.Name,
		kind:    symbolKindType,
		typeRef: exportedType,
	}, nil
}

func exportedOutputFieldType(field ast.OutputField, output ast.OutputBlock, context processContext) (valueType, error) {
	if schemaName, ok := outputSchemaName(output.Directives); ok {
		schema, exists := context.schemas.Get(schemaName)
		if !exists {
			return valueType{}, validationErrorf("unknown schema %q", schemaName)
		}

		for _, schemaField := range schema.Fields {
			if schemaField.Name != field.Name {
				continue
			}

			resolvedType, err := resolveValueType(schemaField.Type, context.symbols, context.types, context.schemas, context.enums)
			if err != nil {
				return valueType{}, err
			}

			return sanitizeImportedValueType(resolvedType, context.schemas), nil
		}
	}

	inferredType, err := inferExpressionType(field.Value, context.variables, context.symbols, context.types, context.schemas, context.enums)
	if err != nil {
		return valueType{}, err
	}

	return sanitizeImportedValueType(inferredType, context.schemas), nil
}

func sanitizeImportedValueType(input valueType, schemas *schemaRegistry) valueType {
	sanitized := input
	if sanitized.element != nil {
		element := sanitizeImportedValueType(*sanitized.element, schemas)
		sanitized.element = &element
	}
	if len(sanitized.members) > 0 {
		members := make([]valueType, 0, len(sanitized.members))
		for _, member := range sanitized.members {
			members = append(members, sanitizeImportedValueType(member, schemas))
		}
		sanitized.members = members
	}

	if sanitized.kind != ValueRecord {
		return sanitized
	}
	if sanitized.schemaName == "" {
		return sanitized
	}

	if _, ok := schemas.Get(sanitized.schemaName); ok {
		sanitized.schemaName = ""
	}

	return sanitized
}

func resolveExportedTypeReference(typeRef ast.TypeReference, types *typeRegistry, schemas *schemaRegistry, aliasStack map[string]struct{}, schemaStack map[string]struct{}) (ast.TypeReference, error) {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return ref, nil
	case ast.ArrayType:
		element, err := resolveExportedTypeReference(ref.Element, types, schemas, aliasStack, schemaStack)
		if err != nil {
			return nil, err
		}

		return ast.ArrayType{Element: element}, nil
	case ast.UnionType:
		members := make([]ast.TypeReference, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolvedMember, err := resolveExportedTypeReference(member, types, schemas, aliasStack, schemaStack)
			if err != nil {
				return nil, err
			}

			members = append(members, resolvedMember)
		}

		return ast.UnionType{Members: members}, nil
	case ast.VariantType:
		members := make([]ast.TypeReference, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolvedMember, err := resolveExportedTypeReference(member, types, schemas, aliasStack, schemaStack)
			if err != nil {
				return nil, err
			}

			members = append(members, resolvedMember)
		}

		return ast.VariantType{Members: members}, nil
	case ast.RecordType:
		return resolveExportedRecordType(ref, types, schemas, aliasStack, schemaStack)
	case ast.NamedType:
		if record, ok := schemas.Get(ref.Name); ok {
			if _, exists := schemaStack[ref.Name]; exists {
				return nil, validationErrorf("cyclic schema reference %q", ref.Name)
			}

			nextSchemaStack := cloneNameSet(schemaStack)
			nextSchemaStack[ref.Name] = struct{}{}
			return resolveExportedRecordType(record, types, schemas, aliasStack, nextSchemaStack)
		}

		if _, ok := aliasStack[ref.Name]; ok {
			return nil, validationErrorf("cyclic type alias %q", ref.Name)
		}

		resolved, exists, err := types.Resolve(ref.Name)
		if err != nil {
			return nil, err
		}
		if !exists {
			return ref, nil
		}

		nextAliasStack := cloneNameSet(aliasStack)
		nextAliasStack[ref.Name] = struct{}{}
		return resolveExportedTypeReference(resolved, types, schemas, nextAliasStack, schemaStack)
	default:
		return nil, validationErrorf("unknown type reference")
	}
}

func resolveExportedRecordType(record ast.RecordType, types *typeRegistry, schemas *schemaRegistry, aliasStack map[string]struct{}, schemaStack map[string]struct{}) (ast.RecordType, error) {
	fields := make([]ast.SchemaField, 0, len(record.Fields))
	for _, field := range record.Fields {
		resolvedType, err := resolveExportedTypeReference(field.Type, types, schemas, aliasStack, schemaStack)
		if err != nil {
			return ast.RecordType{}, err
		}

		fields = append(fields, ast.SchemaField{
			Name:     field.Name,
			Optional: field.Optional,
			Type:     resolvedType,
		})
	}

	return ast.RecordType{Fields: fields}, nil
}

func resolveExportedEnumDefinition(typeRef ast.TypeReference, types *typeRegistry, enums *enumRegistry) (enumDefinition, bool, error) {
	switch ref := typeRef.(type) {
	case ast.NamedType:
		if enumDef, ok := enums.Get(ref.Name); ok {
			return enumDef, true, nil
		}

		resolved, exists, err := types.Resolve(ref.Name)
		if err != nil {
			return enumDefinition{}, false, err
		}
		if !exists {
			return enumDefinition{}, false, nil
		}

		return resolveExportedEnumDefinition(resolved, types, enums)
	default:
		return enumDefinition{}, false, nil
	}
}

func cloneNameSet(values map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(values))
	for name := range values {
		cloned[name] = struct{}{}
	}

	return cloned
}

func collectDeclarations(items []ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	for _, declaration := range items {
		switch decl := declaration.(type) {
		case ast.VariableDeclaration:
			if symbols.Has(decl.Name) {
				return validationErrorf("duplicate declaration %q", decl.Name)
			}
			symbols.Add(decl.Name, symbolKindVariable)
		case ast.TypeDeclaration:
			if symbols.Has(decl.Name) {
				return validationErrorf("duplicate declaration %q", decl.Name)
			}
			symbols.Add(decl.Name, symbolKindType)
			types.AddAlias(decl.Name, decl.Type)
		case ast.SchemaDeclaration:
			if symbols.Has(decl.Name) {
				return validationErrorf("duplicate declaration %q", decl.Name)
			}
			symbols.Add(decl.Name, symbolKindSchema)
			schemas.Add(decl.Name, decl.Type)
		case ast.EnumDeclaration:
			if symbols.Has(decl.Name) {
				return validationErrorf("duplicate enum declaration %q", decl.Name)
			}
			enumDef, err := enumDefinitionFromDeclaration(decl)
			if err != nil {
				return err
			}
			symbols.Add(decl.Name, symbolKindEnum)
			enums.Add(decl.Name, enumDef)
		case ast.DocDeclaration:
			continue
		default:
			return validationErrorf("unknown declaration")
		}
	}

	return nil
}

func validateDeclarations(items []ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry, variables *variableRegistry, injections map[string]Value) error {
	seenDocs := map[string]struct{}{}
	docsByTarget := map[string]ast.DocDeclaration{}
	declaredKinds := map[string]symbolKind{}
	for _, declaration := range items {
		docDeclaration, ok := declaration.(ast.DocDeclaration)
		if !ok {
			continue
		}
		docsByTarget[docDeclaration.Target] = docDeclaration
	}

	for _, declaration := range items {
		if err := validateDeclaration(declaration, symbols, types, schemas, enums, variables, injections, seenDocs, docsByTarget, declaredKinds); err != nil {
			return err
		}

		switch decl := declaration.(type) {
		case ast.TypeDeclaration:
			declaredKinds[decl.Name] = symbolKindType
		case ast.SchemaDeclaration:
			declaredKinds[decl.Name] = symbolKindSchema
		case ast.EnumDeclaration:
			declaredKinds[decl.Name] = symbolKindEnum
		case ast.VariableDeclaration:
			declaredKinds[decl.Name] = symbolKindVariable
		}
	}

	return nil
}

func validateDeclaration(declaration ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry, variables *variableRegistry, injections map[string]Value, seenDocs map[string]struct{}, docsByTarget map[string]ast.DocDeclaration, declaredKinds map[string]symbolKind) error {
	switch decl := declaration.(type) {
	case ast.VariableDeclaration:
		if err := validateTypeReference(decl.Type, symbols, types, schemas, enums); err != nil {
			return err
		}
		expectedType, err := resolveValueType(decl.Type, symbols, types, schemas, enums)
		if err != nil {
			return err
		}

		if decl.HasValue {
			actualType, err := inferExpressionType(decl.Value, variables, symbols, types, schemas, enums)
			if err != nil {
				return err
			}
			if err := ensureAssignable(expectedType, actualType); err != nil {
				return err
			}
			if err := validateExpressionAgainstType(decl.Value, expectedType, variables, symbols, types, schemas, enums); err != nil {
				return err
			}
		} else if !decl.Injectable {
			return validationErrorf("variable %q requires an initializer", decl.Name)
		}

		if decl.Injectable {
			if injectedValue, ok := injections[decl.Name]; ok {
				if err := ensureAssignable(expectedType, valueTypeFromValue(injectedValue)); err != nil {
					return err
				}
			}
		}
		variables.Add(decl.Name, expectedType)
		return nil
	case ast.TypeDeclaration:
		if err := validateTypeReference(decl.Type, symbols, types, schemas, enums); err != nil {
			return err
		}
		if enumDef, ok, err := resolveUnionEnumDefinition(decl.Name, decl.Type, types, enums); err != nil {
			return err
		} else if ok {
			enums.Add(decl.Name, enumDef)
		}
		if decl.Description != "" {
			if _, ok := docsByTarget[decl.Name]; ok {
				return validationErrorf("type %q is already documented by a documentation declaration", decl.Name)
			}
		}
		return nil
	case ast.SchemaDeclaration:
		if err := validateRecordType(decl.Type, symbols, types, schemas, enums); err != nil {
			return err
		}
		if docDeclaration, ok := docsByTarget[decl.Name]; ok {
			for _, field := range decl.Type.Fields {
				if field.Description == "" {
					continue
				}
				if _, documented := docDeclaration.Documentation.Props[field.Name]; documented {
					return validationErrorf("schema field %q in %q is already documented by schema_doc props", field.Name, decl.Name)
				}
			}
		}
		return nil
	case ast.EnumDeclaration:
		_, err := enumDefinitionFromDeclaration(decl)
		return err
	case ast.DocDeclaration:
		return validateDocDeclaration(decl, symbols, schemas, seenDocs, declaredKinds)
	default:
		return validationErrorf("unknown declaration")
	}
}

func validateInjections(items []ast.Declaration, injections map[string]Value) error {
	if len(injections) == 0 {
		return nil
	}

	injectableNames := map[string]struct{}{}
	for _, declaration := range items {
		variable, ok := declaration.(ast.VariableDeclaration)
		if !ok || !variable.Injectable {
			continue
		}

		injectableNames[variable.Name] = struct{}{}
	}

	for name := range injections {
		if _, ok := injectableNames[name]; ok {
			continue
		}

		return validationErrorf("unknown injectable %q", name)
	}

	return nil
}

func validateTypeReference(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return nil
	case ast.ArrayType:
		return validateTypeReference(ref.Element, symbols, types, schemas, enums)
	case ast.UnionType:
		if members, ok, err := resolveUnionEnumValueTypes(ref, types, enums, false); err != nil || ok {
			if err != nil {
				return err
			}
			return validateVariantValueTypes(members)
		}
		_, err := resolveUnionRecordType(ref, symbols, types, schemas)
		if err != nil && strings.Contains(err.Error(), "union members must be schemas") {
			return validationErrorf("union members must be schemas or enums")
		}
		return err
	case ast.VariantType:
		for _, member := range ref.Members {
			if err := validateTypeReference(member, symbols, types, schemas, enums); err != nil {
				return err
			}
		}
		resolved, err := resolveValueType(ref, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		return validateVariantValueTypes(resolved.members)
	case ast.RecordType:
		return validateRecordType(ref, symbols, types, schemas, enums)
	case ast.NamedType:
		if symbols.IsType(ref.Name) {
			_, _, err := types.Resolve(ref.Name)
			return err
		}
		if !symbols.IsSchema(ref.Name) && !symbols.IsEnum(ref.Name) && !symbols.IsImport(ref.Name) {
			return validationErrorf("unknown type %q", ref.Name)
		}
		return nil
	default:
		return validationErrorf("unknown type reference")
	}
}

func validateRecordType(record ast.RecordType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	fieldNames := map[string]struct{}{}
	for _, field := range record.Fields {
		if _, exists := fieldNames[field.Name]; exists {
			return validationErrorf("duplicate field %q", field.Name)
		}
		fieldNames[field.Name] = struct{}{}

		if err := validateTypeReference(field.Type, symbols, types, schemas, enums); err != nil {
			return err
		}
	}

	return nil
}

func validateDocDeclaration(declaration ast.DocDeclaration, symbols *symbolTable, schemas *schemaRegistry, seenDocs map[string]struct{}, declaredKinds map[string]symbolKind) error {
	targetKind, ok := symbols.Get(declaration.Target)
	if !ok {
		return validationErrorf("documentation target %q must reference an existing declaration", declaration.Target)
	}
	if _, exists := seenDocs[declaration.Target]; exists {
		return validationErrorf("duplicate documentation declaration for %q", declaration.Target)
	}

	keyword := "gen_doc"
	declaredKind, declared := declaredKinds[declaration.Target]
	if declaration.Kind == ast.DocumentationKindSchema {
		keyword = "schema_doc"
		if !declared || (declaredKind != symbolKindSchema && declaredKind != symbolKindEnum) {
			return validationErrorf("schema_doc target %q must appear after its schema or enum declaration", declaration.Target)
		}
		if targetKind != symbolKindSchema && targetKind != symbolKindEnum {
			return validationErrorf("schema_doc target %q must reference a schema or enum", declaration.Target)
		}
	} else {
		if !declared || (declaredKind != symbolKindType && declaredKind != symbolKindVariable) {
			return validationErrorf("gen_doc target %q must appear after its type or variable declaration", declaration.Target)
		}
		if targetKind != symbolKindType && targetKind != symbolKindVariable {
			return validationErrorf("gen_doc target %q must reference a type or variable", declaration.Target)
		}
	}
	seenDocs[declaration.Target] = struct{}{}

	if declaration.Documentation.Summary != nil {
		if _, err := parseStaticString(declaration.Documentation.Summary.Lexeme); err != nil {
			return err
		}
	}
	if declaration.Documentation.Description != nil {
		if _, err := parseDocString(declaration.Documentation.Description.Lexeme); err != nil {
			return err
		}
	}

	if len(declaration.Documentation.Props) > 0 {
		if declaration.Kind != ast.DocumentationKindSchema {
			return validationErrorf("%s props for %q require a schema target", keyword, declaration.Target)
		}

		record, ok := schemas.Get(declaration.Target)
		if !ok {
			return validationErrorf("unknown schema %q for %s props", declaration.Target, keyword)
		}

		fieldNames := map[string]struct{}{}
		for _, field := range record.Fields {
			fieldNames[field.Name] = struct{}{}
		}

		for name, value := range declaration.Documentation.Props {
			if _, exists := fieldNames[name]; !exists {
				return validationErrorf("%s props field %q does not exist on schema %q", keyword, name, declaration.Target)
			}
			if _, err := parseStaticString(value.Lexeme); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateOutputDirectiveStructure(output ast.OutputBlock) error {
	if output.Doc != nil {
		if len(output.Directives) == 0 {
			return validationErrorf("inline doc blocks require a directive list")
		}
		if _, err := parseDocString(output.Doc.Lexeme); err != nil {
			return err
		}
	}

	if len(output.Directives) == 0 {
		return nil
	}

	var outputValue string
	seenKinds := map[ast.OutputDirectiveKind]struct{}{}

	for _, directive := range output.Directives {
		if _, exists := seenKinds[directive.Kind]; exists {
			return validationErrorf("duplicate output directive %q", directiveKindName(directive.Kind))
		}
		seenKinds[directive.Kind] = struct{}{}

		switch directive.Kind {
		case ast.OutputDirectiveOutput:
			outputValue = directive.Value
		case ast.OutputDirectiveSchema:
			if output.Mode == ast.OutputModeSchema {
				return validationErrorf("schema directive is invalid when output mode is schema")
			}
		case ast.OutputDirectiveSchemaFile:
			if output.Mode == ast.OutputModeSchema {
				return validationErrorf("schema_file directive is invalid when output mode is schema")
			}
		default:
			return validationErrorf("unknown output directive")
		}
	}

	if outputValue == "" {
		return validationErrorf("missing output directive")
	}

	return nil
}

func validateOutputDirectiveReferences(output ast.OutputBlock, symbols *symbolTable) error {
	for _, directive := range output.Directives {
		if directive.Kind != ast.OutputDirectiveSchema {
			continue
		}

		if !symbols.IsSchema(directive.Value) && !symbols.IsImport(directive.Value) {
			return validationErrorf("unknown schema %q", directive.Value)
		}
	}

	return nil
}

func validateSchemaOutputFields(fields []ast.OutputSchemaField, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	fieldNames := map[string]struct{}{}
	for _, field := range fields {
		if _, exists := fieldNames[field.Name]; exists {
			return validationErrorf("duplicate output field %q", field.Name)
		}
		fieldNames[field.Name] = struct{}{}

		if err := validateSchemaOutputFieldType(field.Type, symbols); err != nil {
			return err
		}

		if err := validateTypeReference(field.Type, symbols, types, schemas, enums); err != nil {
			return err
		}
	}

	return nil
}

func validateSchemaOutputFieldType(typeRef ast.TypeReference, symbols *symbolTable) error {
	switch ref := typeRef.(type) {
	case ast.ArrayType:
		return validateSchemaOutputFieldType(ref.Element, symbols)
	case ast.RecordType:
		for _, field := range ref.Fields {
			if err := validateSchemaOutputFieldType(field.Type, symbols); err != nil {
				return err
			}
		}
		return nil
	case ast.NamedType:
		if symbols.IsVariable(ref.Name) {
			return validationErrorf("invalid field type %q in output = schema", ref.Name)
		}
	}

	return nil
}

func outputSchemaName(directives []ast.OutputDirective) (string, bool) {
	for _, directive := range directives {
		if directive.Kind == ast.OutputDirectiveSchema {
			return directive.Value, true
		}
	}
	return "", false
}

func validateDataOutputFields(fields []ast.OutputField, symbols *symbolTable) error {
	for _, field := range fields {
		if err := validateDataOutputExpression(field.Value, symbols); err != nil {
			return err
		}
	}

	return nil
}

func validateDataOutputExpression(expression ast.Expression, symbols *symbolTable) error {
	switch expr := expression.(type) {
	case ast.Identifier:
		if symbols.IsType(expr.Name) || symbols.IsSchema(expr.Name) || symbols.IsEnum(expr.Name) {
			return validationErrorf("output value %q cannot reference type or schema declaration", expr.Name)
		}
	case ast.ArrayLiteral:
		for _, element := range expr.Elements {
			if err := validateDataOutputExpression(element, symbols); err != nil {
				return err
			}
		}
	case ast.RecordLiteral:
		for _, field := range expr.Fields {
			if err := validateDataOutputExpression(field.Value, symbols); err != nil {
				return err
			}
		}
	case ast.PrefixExpression:
		return validateDataOutputExpression(expr.Right, symbols)
	case ast.InfixExpression:
		if err := validateDataOutputExpression(expr.Left, symbols); err != nil {
			return err
		}
		return validateDataOutputExpression(expr.Right, symbols)
	case ast.ConditionalExpression:
		if err := validateDataOutputExpression(expr.Condition, symbols); err != nil {
			return err
		}
		if err := validateDataOutputExpression(expr.Then, symbols); err != nil {
			return err
		}
		return validateDataOutputExpression(expr.Else, symbols)
	}

	return nil
}

func validateOutputSchema(schemaName string, items []ast.OutputField, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	schema, ok := schemas.Get(schemaName)
	if !ok {
		return validationErrorf("unknown schema %q", schemaName)
	}

	fieldsByName := map[string]ast.OutputField{}
	for _, item := range items {
		if _, exists := fieldsByName[item.Name]; exists {
			return validationErrorf("duplicate output field %q", item.Name)
		}
		fieldsByName[item.Name] = item
	}

	schemaFields := schemaFieldMap(schema)
	for _, field := range schema.Fields {
		item, exists := fieldsByName[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			return validationErrorf("missing required field %q for schema %q", field.Name, schemaName)
		}
		if item.Optional && !field.Optional {
			return validationErrorf("field %q is not optional in schema %q", field.Name, schemaName)
		}
		expectedType, err := resolveValueType(field.Type, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		actualType, err := inferExpressionType(item.Value, variables, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if err := ensureAssignable(expectedType, actualType); err != nil {
			return err
		}
		if err := validateExpressionAgainstType(item.Value, expectedType, variables, symbols, types, schemas, enums); err != nil {
			return err
		}
	}

	for name := range fieldsByName {
		if _, exists := schemaFields[name]; !exists {
			return validationErrorf("unknown output field %q for schema %q", name, schemaName)
		}
	}

	return nil
}

func validateExpressionAgainstType(expression ast.Expression, expectedType valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	if len(expectedType.members) > 0 {
		return validateExpressionAgainstVariantMembers(expression, expectedType.members, variables, symbols, types, schemas, enums)
	}

	switch expectedType.kind {
	case ValueString, ValueInt, ValueFloat, ValueBoolean:
		actualType, err := inferExpressionType(expression, variables, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		return ensureAssignable(expectedType, actualType)
	case ValueArray:
		if expectedType.element == nil {
			return nil
		}
		switch typed := expression.(type) {
		case ast.ArrayLiteral:
			for _, element := range typed.Elements {
				if err := validateExpressionAgainstType(element, *expectedType.element, variables, symbols, types, schemas, enums); err != nil {
					return err
				}
			}
		case ast.ConditionalExpression:
			if err := validateExpressionAgainstType(typed.Then, expectedType, variables, symbols, types, schemas, enums); err != nil {
				return err
			}
			if err := validateExpressionAgainstType(typed.Else, expectedType, variables, symbols, types, schemas, enums); err != nil {
				return err
			}
		}
	case ValueRecord:
		if expectedType.record == nil && expectedType.schemaName == "" {
			return nil
		}
		switch typed := expression.(type) {
		case ast.RecordLiteral:
			if expectedType.schemaName != "" {
				return validateRecordLiteral(typed, expectedType.schemaName, variables, symbols, types, schemas, enums)
			}
			if expectedType.record != nil {
				return validateRecordLiteralAgainstRecordType(typed, *expectedType.record, "", variables, symbols, types, schemas, enums)
			}
			return nil
		case ast.ConditionalExpression:
			if err := validateExpressionAgainstType(typed.Then, expectedType, variables, symbols, types, schemas, enums); err != nil {
				return err
			}
			if err := validateExpressionAgainstType(typed.Else, expectedType, variables, symbols, types, schemas, enums); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateExpressionAgainstVariantMembers(expression ast.Expression, members []valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	actualType, err := inferExpressionType(expression, variables, symbols, types, schemas, enums)
	if err != nil {
		return err
	}

	matchCount := 0
	for _, member := range members {
		if err := ensureAssignable(member, actualType); err != nil {
			continue
		}
		if err := validateExpressionAgainstType(expression, member, variables, symbols, types, schemas, enums); err == nil {
			matchCount++
		}
	}

	if matchCount == 1 {
		return nil
	}
	if matchCount == 0 {
		return validationErrorf("type mismatch: expected %s, got %s", valueType{members: members}.name(), actualType.name())
	}

	return validationErrorf("type mismatch: expected exactly one variant member for %s", valueType{members: members}.name())
}

func validateRecordLiteral(expr ast.RecordLiteral, schemaName string, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	schema, ok := schemas.Get(schemaName)
	if !ok {
		return validationErrorf("unknown schema %q", schemaName)
	}

	return validateRecordLiteralAgainstRecordType(expr, schema, schemaName, variables, symbols, types, schemas, enums)
}

func validateRecordLiteralAgainstRecordType(expr ast.RecordLiteral, recordType ast.RecordType, schemaName string, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	fieldsByName := map[string]ast.RecordField{}
	for _, field := range expr.Fields {
		if _, exists := fieldsByName[field.Name]; exists {
			return validationErrorf("duplicate record field %q", field.Name)
		}
		fieldsByName[field.Name] = field
	}

	schemaFields := schemaFieldMap(recordType)
	for _, field := range recordType.Fields {
		recordField, exists := fieldsByName[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			if schemaName != "" {
				return validationErrorf("missing required field %q for schema %q", field.Name, schemaName)
			}
			return validationErrorf("missing required field %q", field.Name)
		}
		if recordField.Optional && !field.Optional {
			if schemaName != "" {
				return validationErrorf("field %q is not optional in schema %q", field.Name, schemaName)
			}
			return validationErrorf("field %q is not optional", field.Name)
		}
		expectedType, err := resolveValueType(field.Type, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		actualType, err := inferExpressionType(recordField.Value, variables, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if err := ensureAssignable(expectedType, actualType); err != nil {
			return err
		}
		if err := validateExpressionAgainstType(recordField.Value, expectedType, variables, symbols, types, schemas, enums); err != nil {
			return err
		}
	}

	for name := range fieldsByName {
		if _, exists := schemaFields[name]; !exists {
			if schemaName != "" {
				return validationErrorf("unknown field %q for schema %q", name, schemaName)
			}
			return validationErrorf("unknown field %q", name)
		}
	}

	return nil
}

func validateEvaluatedOutputSchema(schemaName string, fields map[string]Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	schema, ok := schemas.Get(schemaName)
	if !ok {
		return validationErrorf("unknown schema %q", schemaName)
	}

	schemaFields := schemaFieldMap(schema)
	for _, field := range schema.Fields {
		value, exists := fields[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			return validationErrorf("missing required field %q for schema %q", field.Name, schemaName)
		}

		expectedType, err := resolveValueType(field.Type, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if err := validateEvaluatedValueAgainstType(value, expectedType, symbols, types, schemas, enums); err != nil {
			return err
		}
	}

	for name := range fields {
		if _, exists := schemaFields[name]; !exists {
			return validationErrorf("unknown output field %q for schema %q", name, schemaName)
		}
	}

	return nil
}

func validateEvaluatedValueAgainstType(value Value, expectedType valueType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	if len(expectedType.members) > 0 {
		return validateEvaluatedValueAgainstVariantMembers(value, expectedType.members, symbols, types, schemas, enums)
	}
	if expectedType.enumName != "" {
		enumDef, ok := enums.Get(expectedType.enumName)
		if !ok {
			return validationErrorf("unknown enum %q", expectedType.enumName)
		}
		if !enumDef.ContainsValue(value) {
			return validationErrorf("invalid enum value %s for enum %q", enumValueDisplay(value), expectedType.enumName)
		}
		return nil
	}

	switch expectedType.kind {
	case ValueString, ValueInt, ValueFloat, ValueBoolean:
		if value.Kind != expectedType.kind {
			return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), value.kindName())
		}
	case ValueArray:
		if value.Kind != ValueArray {
			return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), value.kindName())
		}
		if expectedType.element == nil {
			return nil
		}
		for _, item := range value.Array {
			if err := validateEvaluatedValueAgainstType(item, *expectedType.element, symbols, types, schemas, enums); err != nil {
				return err
			}
		}
	case ValueRecord:
		if expectedType.record == nil && expectedType.schemaName == "" {
			return nil
		}
		if value.Kind != ValueRecord {
			return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), value.kindName())
		}

		recordType := expectedType.record
		if expectedType.schemaName != "" {
			schema, ok := schemas.Get(expectedType.schemaName)
			if !ok {
				return validationErrorf("unknown schema %q", expectedType.schemaName)
			}
			recordType = &schema
		}
		if recordType == nil {
			return nil
		}

		schemaFields := schemaFieldMap(*recordType)
		for _, field := range recordType.Fields {
			fieldValue, exists := value.Record[field.Name]
			if !exists {
				if field.Optional {
					continue
				}
				return validationErrorf("missing required field %q for schema %q", field.Name, expectedType.schemaName)
			}

			fieldType, err := resolveValueType(field.Type, symbols, types, schemas, enums)
			if err != nil {
				return err
			}
			if err := validateEvaluatedValueAgainstType(fieldValue, fieldType, symbols, types, schemas, enums); err != nil {
				return err
			}
		}

		for name := range value.Record {
			if _, exists := schemaFields[name]; !exists {
				return validationErrorf("unknown field %q for schema %q", name, expectedType.schemaName)
			}
		}
	}

	return nil
}

func validateEvaluatedValueAgainstVariantMembers(value Value, members []valueType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	if variantMembersAreEnums(members) {
		if value.Kind == members[0].kind {
			return nil
		}
		return validationErrorf("type mismatch: expected %s, got %s", valueType{members: members}.name(), value.kindName())
	}

	matchCount := 0
	for _, member := range members {
		if err := validateEvaluatedValueAgainstType(value, member, symbols, types, schemas, enums); err == nil {
			matchCount++
		}
	}

	if matchCount == 1 {
		return nil
	}
	if matchCount == 0 {
		return validationErrorf("type mismatch: expected %s, got %s", valueType{members: members}.name(), value.kindName())
	}

	return validationErrorf("type mismatch: expected exactly one variant member for %s", valueType{members: members}.name())
}

func variantMembersAreEnums(members []valueType) bool {
	if len(members) == 0 {
		return false
	}
	for _, member := range members {
		if member.enumName == "" {
			return false
		}
	}
	return true
}

func schemaFieldMap(schema ast.RecordType) map[string]ast.SchemaField {
	fields := map[string]ast.SchemaField{}
	for _, field := range schema.Fields {
		fields[field.Name] = field
	}
	return fields
}

func directiveKindName(kind ast.OutputDirectiveKind) string {
	switch kind {
	case ast.OutputDirectiveOutput:
		return "output"
	case ast.OutputDirectiveSchemaFile:
		return "schema_file"
	case ast.OutputDirectiveSchema:
		return "schema"
	default:
		return "unknown"
	}
}

type Value struct {
	Kind    ValueKind
	Int     int64
	Float   float64
	Boolean bool
	String  string
	Array   []Value
	Record  map[string]Value
}

type valueEnvironment struct {
	values             map[string]Value
	missingInjectables map[string]struct{}
}

func newValueEnvironment() *valueEnvironment {
	return &valueEnvironment{
		values:             map[string]Value{},
		missingInjectables: map[string]struct{}{},
	}
}

func (environment *valueEnvironment) Add(name string, value Value) {
	environment.values[name] = value
	delete(environment.missingInjectables, name)
}

func (environment *valueEnvironment) AddMissingInjectable(name string) {
	delete(environment.values, name)
	environment.missingInjectables[name] = struct{}{}
}

func (environment *valueEnvironment) Get(name string) (Value, bool) {
	value, ok := environment.values[name]
	return value, ok
}

func (environment *valueEnvironment) IsMissingInjectable(name string) bool {
	_, ok := environment.missingInjectables[name]
	return ok
}

func (environment *valueEnvironment) Values() map[string]Value {
	values := make(map[string]Value, len(environment.values))
	for name, value := range environment.values {
		values[name] = value
	}

	return values
}

func (environment *valueEnvironment) Clone() *valueEnvironment {
	cloned := newValueEnvironment()
	cloned.values = environment.Values()
	for name := range environment.missingInjectables {
		cloned.missingInjectables[name] = struct{}{}
	}

	return cloned
}

func evaluateSchemaOutput(output ast.OutputBlock) (map[SchemaField]SchemaType, error) {
	if output.Mode != ast.OutputModeSchema {
		return map[SchemaField]SchemaType{}, nil
	}

	fields := make(map[SchemaField]SchemaType, len(output.SchemaFields))
	for _, field := range output.SchemaFields {
		schemaType, err := schemaTypeFromTypeReference(field.Type)
		if err != nil {
			return nil, err
		}

		fields[SchemaField{Name: field.Name, Optional: field.Optional}] = schemaType
	}

	return fields, nil
}

func evaluateScript(items []ast.Declaration, environment *valueEnvironment, injections map[string]Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	for _, declaration := range items {
		variable, ok := declaration.(ast.VariableDeclaration)
		if !ok {
			continue
		}

		if variable.Injectable {
			if injectedValue, ok := injections[variable.Name]; ok {
				expectedType, err := resolveValueType(variable.Type, symbols, types, schemas, enums)
				if err != nil {
					return err
				}
				if err := validateEvaluatedValueAgainstType(injectedValue, expectedType, symbols, types, schemas, enums); err != nil {
					return err
				}
				environment.Add(variable.Name, injectedValue)
				continue
			}

			if !variable.HasValue {
				environment.AddMissingInjectable(variable.Name)
				continue
			}
		}

		if !variable.HasValue {
			return validationErrorf("variable %q requires an initializer", variable.Name)
		}

		value, err := evaluateExpression(variable.Value, environment, Value{}, symbols, types, schemas, enums)
		if err != nil {
			return err
		}

		expectedType, err := resolveValueType(variable.Type, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		value, err = coerceEvaluatedValueAgainstType(variable.Value, value, expectedType, environment, Value{}, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if err := validateEvaluatedValueAgainstType(value, expectedType, symbols, types, schemas, enums); err != nil {
			return err
		}

		environment.Add(variable.Name, value)
	}

	return nil
}

func evaluateOutputFields(items []ast.OutputField, environment *valueEnvironment, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (map[string]Value, error) {
	fields := map[string]Value{}
	self := Value{Kind: ValueRecord, Record: fields}
	for _, item := range items {
		value, err := evaluateExpression(item.Value, environment, self, symbols, types, schemas, enums)
		if err != nil {
			return nil, err
		}
		fields[item.Name] = value
	}

	return fields, nil
}

func coerceEvaluatedValueAgainstType(expression ast.Expression, value Value, expectedType valueType, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	if expectedType.kind == ValueArray && expectedType.element != nil {
		arrayLiteral, ok := expression.(ast.ArrayLiteral)
		if !ok || value.Kind != ValueArray {
			return value, nil
		}

		values := make([]Value, 0, len(value.Array))
		for index, element := range arrayLiteral.Elements {
			coerced, err := coerceEvaluatedValueAgainstType(element, value.Array[index], *expectedType.element, environment, self, symbols, types, schemas, enums)
			if err != nil {
				return Value{}, err
			}
			values = append(values, coerced)
		}
		return Value{Kind: ValueArray, Array: values}, nil
	}

	if !variantMembersAreEnums(expectedType.members) {
		return value, nil
	}

	memberAccess, ok := expression.(ast.MemberAccess)
	if !ok {
		return value, nil
	}
	identifier, ok := memberAccess.Target.(ast.Identifier)
	if !ok {
		return value, nil
	}
	merged, ok, err := mergedEnumFromValueTypes("", expectedType.members, enums)
	if err != nil || !ok {
		return value, err
	}
	for _, member := range expectedType.members {
		if member.enumName != identifier.Name {
			continue
		}
		if _, exists := enums.Get(member.enumName); !exists {
			continue
		}
		mergedMember, exists := merged.Member(memberAccess.Name)
		if !exists {
			return Value{}, validationErrorf("unknown enum member %q in enum union", memberAccess.Name)
		}
		return mergedMember.Value, nil
	}

	return value, nil
}

func mergedEnumFromValueTypes(name string, members []valueType, enums *enumRegistry) (enumDefinition, bool, error) {
	definitions := make([]enumDefinition, 0, len(members))
	for _, member := range members {
		if member.enumName == "" {
			return enumDefinition{}, false, nil
		}
		definition, exists := enums.Get(member.enumName)
		if !exists {
			return enumDefinition{}, false, nil
		}
		definitions = append(definitions, definition)
	}
	definition, err := mergeEnumDefinitions(name, definitions)
	return definition, true, err
}

func evaluateExpression(expression ast.Expression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	switch expr := expression.(type) {
	case ast.Identifier:
		value, ok := environment.Get(expr.Name)
		if !ok {
			if environment.IsMissingInjectable(expr.Name) {
				return Value{}, validationErrorf("injectable %q requires a runtime value", expr.Name)
			}
			if symbols != nil {
				if kind, exists := symbols.Get(expr.Name); exists && kind != symbolKindVariable {
					return Value{}, validationErrorf("type reference %q is not a valid value expression", expr.Name)
				}
			}
			return Value{}, validationErrorf("unknown identifier %q", expr.Name)
		}
		return value, nil
	case ast.MemberAccess:
		return evaluateMemberAccess(expr, environment, self, symbols, types, schemas, enums)
	case ast.ArrayAccess:
		return evaluateArrayAccess(expr, environment, self, symbols, types, schemas, enums)
	case ast.IntLiteral:
		return parseInt(expr.Lexeme)
	case ast.FloatLiteral:
		return parseFloat(expr.Lexeme)
	case ast.StringLiteral:
		return parseInterpolatedString(expr.Lexeme, environment, self, symbols, types, schemas, enums)
	case ast.BooleanLiteral:
		return Value{Kind: ValueBoolean, Boolean: expr.Value}, nil
	case ast.ArrayLiteral:
		return evaluateArrayLiteral(expr, environment, self, symbols, types, schemas, enums)
	case ast.RecordLiteral:
		return evaluateRecordLiteral(expr, environment, self, symbols, types, schemas, enums)
	case ast.SelfReference:
		return evaluateSelfReference(expr, self)
	case ast.PrefixExpression:
		return evaluatePrefix(expr, environment, self, symbols, types, schemas, enums)
	case ast.InfixExpression:
		return evaluateInfix(expr, environment, self, symbols, types, schemas, enums)
	case ast.ConditionalExpression:
		return evaluateConditional(expr, environment, self, symbols, types, schemas, enums)
	default:
		return Value{}, validationErrorf("unsupported expression")
	}
}

func parseInt(lexeme string) (Value, error) {
	value, err := strconv.ParseInt(lexeme, 10, 64)
	if err != nil {
		return Value{}, validationErrorf("invalid int literal %q", lexeme)
	}
	return Value{Kind: ValueInt, Int: value}, nil
}

func parseFloat(lexeme string) (Value, error) {
	value, err := strconv.ParseFloat(lexeme, 64)
	if err != nil {
		return Value{}, validationErrorf("invalid float literal %q", lexeme)
	}
	return Value{Kind: ValueFloat, Float: value}, nil
}

func parseStaticString(lexeme string) (Value, error) {
	value, err := decodeStringLexeme(lexeme, false, nil)
	if err != nil {
		return Value{}, err
	}
	return Value{Kind: ValueString, String: value}, nil
}

func parseDocString(lexeme string) (Value, error) {
	if !strings.HasPrefix(lexeme, `"""`) {
		return Value{}, validationErrorf("doc blocks must use a block string")
	}
	value, err := decodeStringLexeme(lexeme, false, nil)
	if err != nil {
		return Value{}, err
	}
	return Value{Kind: ValueString, String: value}, nil
}

func parseInterpolatedString(lexeme string, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	value, err := decodeStringLexeme(lexeme, true, func(expressionText string) (string, error) {
		tokens, err := lex(expressionText)
		if err != nil {
			return "", err
		}

		expression, err := parser.New(tokens).ParseExpression()
		if err != nil {
			return "", err
		}

		value, err := evaluateExpression(expression, environment, self, symbols, types, schemas, enums)
		if err != nil {
			return "", err
		}
		return stringifyValue(value)
	})
	if err != nil {
		return Value{}, err
	}
	return Value{Kind: ValueString, String: value}, nil
}

func decodeStringLexeme(lexeme string, allowInterpolation bool, interpolate func(string) (string, error)) (string, error) {
	content, quoteMode, err := stringContent(lexeme)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for index := 0; index < len(content); {
		if content[index] == '\\' {
			if index+1 >= len(content) {
				return "", validationErrorf("invalid string literal %q", lexeme)
			}
			escaped, err := unescapeByte(content[index+1])
			if err != nil {
				return "", err
			}
			builder.WriteByte(escaped)
			index += 2
			continue
		}
		if quoteMode != stringQuoteSingle && strings.HasPrefix(content[index:], "$(") {
			if !allowInterpolation {
				return "", validationErrorf("interpolation is not allowed in this string")
			}
			end, expressionText, err := interpolationContent(content, index)
			if err != nil {
				return "", err
			}
			resolved, err := interpolate(expressionText)
			if err != nil {
				return "", err
			}
			builder.WriteString(resolved)
			index = end
			continue
		}
		builder.WriteByte(content[index])
		index++
	}

	return builder.String(), nil
}

type stringQuoteMode int

const (
	stringQuoteSingle stringQuoteMode = iota
	stringQuoteDouble
	stringQuoteBlock
)

func stringContent(lexeme string) (string, stringQuoteMode, error) {
	switch {
	case strings.HasPrefix(lexeme, `"""`) && strings.HasSuffix(lexeme, `"""`) && len(lexeme) >= 6:
		return lexeme[3 : len(lexeme)-3], stringQuoteBlock, nil
	case strings.HasPrefix(lexeme, `"`) && strings.HasSuffix(lexeme, `"`) && len(lexeme) >= 2:
		return lexeme[1 : len(lexeme)-1], stringQuoteDouble, nil
	case strings.HasPrefix(lexeme, `'`) && strings.HasSuffix(lexeme, `'`) && len(lexeme) >= 2:
		return lexeme[1 : len(lexeme)-1], stringQuoteSingle, nil
	default:
		return "", stringQuoteDouble, validationErrorf("invalid string literal %q", lexeme)
	}
}

func unescapeByte(value byte) (byte, error) {
	switch value {
	case '\\', '\'', '"':
		return value, nil
	case 'n':
		return '\n', nil
	case 'r':
		return '\r', nil
	case 't':
		return '\t', nil
	default:
		return 0, validationErrorf("invalid string escape \\%c", value)
	}
}

func interpolationContent(content string, start int) (int, string, error) {
	depth := 1
	index := start + 2
	for index < len(content) {
		switch content[index] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index + 1, content[start+2 : index], nil
			}
		}
		index++
	}
	return 0, "", validationErrorf("unterminated interpolation")
}

func stringifyValue(value Value) (string, error) {
	switch value.Kind {
	case ValueString:
		return value.String, nil
	case ValueInt:
		return strconv.FormatInt(value.Int, 10), nil
	case ValueFloat:
		return strconv.FormatFloat(value.Float, 'f', -1, 64), nil
	case ValueBoolean:
		return strconv.FormatBool(value.Boolean), nil
	default:
		return "", validationErrorf("interpolation requires a scalar value")
	}
}

func evaluateMemberAccess(expr ast.MemberAccess, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	if identifier, ok := expr.Target.(ast.Identifier); ok {
		if enumDef, exists := enums.Get(identifier.Name); exists {
			member, exists := enumDef.Member(expr.Name)
			if !exists {
				return Value{}, validationErrorf("unknown enum member %q in enum %q", expr.Name, identifier.Name)
			}
			return member.Value, nil
		}
	}

	target, err := evaluateExpression(expr.Target, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if target.Kind != ValueRecord {
		return Value{}, validationErrorf("member access requires a record value")
	}
	member, ok := target.Record[expr.Name]
	if !ok {
		return Value{}, validationErrorf("unknown member %q", expr.Name)
	}
	return member, nil
}

func evaluateArrayAccess(expr ast.ArrayAccess, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	target, err := evaluateExpression(expr.Target, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if target.Kind != ValueArray {
		return Value{}, validationErrorf("array access requires an array value at level %d", arrayAccessLevel(expr))
	}

	index, err := strconv.Atoi(expr.Index.Lexeme)
	if err != nil {
		return Value{}, validationErrorf("array access requires a valid integer index")
	}
	if index < 0 || index >= len(target.Array) {
		return Value{}, validationErrorf("array index %d is out of range at level %d", index, arrayAccessLevel(expr))
	}
	return target.Array[index], nil
}

func evaluatePrefix(expr ast.PrefixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	right, err := evaluateExpression(expr.Right, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}

	switch expr.Operator {
	case lexer.TokenBang:
		if right.Kind != ValueBoolean {
			return Value{}, validationErrorf("type mismatch: expected boolean after '!'")
		}
		return Value{Kind: ValueBoolean, Boolean: !right.Boolean}, nil
	case lexer.TokenTilde:
		if right.Kind != ValueInt {
			return Value{}, validationErrorf("type mismatch: expected int after '~'")
		}
		return Value{Kind: ValueInt, Int: ^right.Int}, nil
	case lexer.TokenPlus:
		if right.Kind != ValueInt && right.Kind != ValueFloat {
			return Value{}, validationErrorf("type mismatch: expected numeric after unary operator")
		}
		return right, nil
	case lexer.TokenMinus:
		switch right.Kind {
		case ValueInt:
			return Value{Kind: ValueInt, Int: -right.Int}, nil
		case ValueFloat:
			return Value{Kind: ValueFloat, Float: -right.Float}, nil
		default:
			return Value{}, validationErrorf("type mismatch: expected numeric after unary operator")
		}
	default:
		return Value{}, validationErrorf("unknown prefix operator")
	}
}

func evaluateInfix(expr ast.InfixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	if expr.Operator == lexer.TokenAndAnd {
		return evaluateLogicalAnd(expr, environment, self, symbols, types, schemas, enums)
	}
	if expr.Operator == lexer.TokenOrOr {
		return evaluateLogicalOr(expr, environment, self, symbols, types, schemas, enums)
	}

	left, err := evaluateExpression(expr.Left, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	right, err := evaluateExpression(expr.Right, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}

	switch expr.Operator {
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenDoubleStar:
		return evaluateNumeric(expr.Operator, left, right)
	case lexer.TokenPercent:
		return evaluateModulo(left, right)
	case lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned:
		return evaluateShift(expr.Operator, left, right)
	case lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret:
		return evaluateBitwise(expr.Operator, left, right)
	case lexer.TokenEqualEqual, lexer.TokenNotEqual, lexer.TokenStrictEqual, lexer.TokenStrictNotEqual:
		return evaluateEquality(expr.Operator, left, right)
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		return evaluateComparison(expr.Operator, left, right)
	default:
		return Value{}, validationErrorf("unknown infix operator")
	}
}

func evaluateNumeric(operator lexer.TokenType, left, right Value) (Value, error) {
	if !isNumericValue(left) || !isNumericValue(right) {
		return Value{}, validationErrorf("type mismatch: expected numeric operands for operator")
	}
	if left.Kind == ValueInt && right.Kind == ValueInt {
		return evaluateIntNumeric(operator, left.Int, right.Int)
	}
	return evaluateFloatNumeric(operator, numericValue(left), numericValue(right))
}

func evaluateIntNumeric(operator lexer.TokenType, left, right int64) (Value, error) {
	switch operator {
	case lexer.TokenPlus:
		return Value{Kind: ValueInt, Int: left + right}, nil
	case lexer.TokenMinus:
		return Value{Kind: ValueInt, Int: left - right}, nil
	case lexer.TokenStar:
		return Value{Kind: ValueInt, Int: left * right}, nil
	case lexer.TokenSlash:
		if right == 0 {
			return Value{}, validationErrorf("division by zero")
		}
		return Value{Kind: ValueInt, Int: left / right}, nil
	case lexer.TokenDoubleStar:
		return evaluateIntPower(left, right)
	default:
		return Value{}, validationErrorf("unknown numeric operator")
	}
}

func evaluateFloatNumeric(operator lexer.TokenType, left, right float64) (Value, error) {
	switch operator {
	case lexer.TokenPlus:
		return Value{Kind: ValueFloat, Float: left + right}, nil
	case lexer.TokenMinus:
		return Value{Kind: ValueFloat, Float: left - right}, nil
	case lexer.TokenStar:
		return Value{Kind: ValueFloat, Float: left * right}, nil
	case lexer.TokenSlash:
		if right == 0 {
			return Value{}, validationErrorf("division by zero")
		}
		return Value{Kind: ValueFloat, Float: left / right}, nil
	case lexer.TokenDoubleStar:
		return Value{Kind: ValueFloat, Float: math.Pow(left, right)}, nil
	default:
		return Value{}, validationErrorf("unknown numeric operator")
	}
}

func evaluateIntPower(base int64, exponent int64) (Value, error) {
	if exponent < 0 {
		return Value{}, validationErrorf("negative exponent for int")
	}
	result := int64(1)
	for exponent > 0 {
		if exponent%2 == 1 {
			result *= base
		}
		base *= base
		exponent /= 2
	}
	return Value{Kind: ValueInt, Int: result}, nil
}

func evaluateModulo(left, right Value) (Value, error) {
	if !isNumericValue(left) || !isNumericValue(right) {
		return Value{}, validationErrorf("type mismatch: expected numeric operands for '%%'")
	}
	if left.Kind == ValueInt && right.Kind == ValueInt {
		if right.Int == 0 {
			return Value{}, validationErrorf("division by zero")
		}
		return Value{Kind: ValueInt, Int: left.Int % right.Int}, nil
	}

	leftNumber := numericValue(left)
	rightNumber := numericValue(right)
	if rightNumber == 0 {
		return Value{}, validationErrorf("division by zero")
	}
	return Value{Kind: ValueFloat, Float: math.Mod(leftNumber, rightNumber)}, nil
}

func evaluateShift(operator lexer.TokenType, left, right Value) (Value, error) {
	if left.Kind != ValueInt || right.Kind != ValueInt {
		return Value{}, validationErrorf("type mismatch: expected int operands for shift")
	}
	if right.Int < 0 {
		return Value{}, validationErrorf("negative shift count")
	}

	shift := uint(right.Int)
	switch operator {
	case lexer.TokenShiftLeft:
		return Value{Kind: ValueInt, Int: left.Int << shift}, nil
	case lexer.TokenShiftRight:
		return Value{Kind: ValueInt, Int: left.Int >> shift}, nil
	case lexer.TokenShiftRightUnsigned:
		return Value{Kind: ValueInt, Int: int64(uint64(left.Int) >> shift)}, nil
	default:
		return Value{}, validationErrorf("unknown shift operator")
	}
}

func evaluateBitwise(operator lexer.TokenType, left, right Value) (Value, error) {
	if left.Kind != ValueInt || right.Kind != ValueInt {
		return Value{}, validationErrorf("type mismatch: expected int operands for bitwise operator")
	}

	switch operator {
	case lexer.TokenAmpersand:
		return Value{Kind: ValueInt, Int: left.Int & right.Int}, nil
	case lexer.TokenPipe:
		return Value{Kind: ValueInt, Int: left.Int | right.Int}, nil
	case lexer.TokenCaret:
		return Value{Kind: ValueInt, Int: left.Int ^ right.Int}, nil
	default:
		return Value{}, validationErrorf("unknown bitwise operator")
	}
}

func evaluateEquality(operator lexer.TokenType, left, right Value) (Value, error) {
	if left.Kind != right.Kind {
		return Value{}, validationErrorf("type mismatch: incompatible equality comparison")
	}

	equal, err := valuesEqual(left, right)
	if err != nil {
		return Value{}, err
	}

	if operator == lexer.TokenNotEqual || operator == lexer.TokenStrictNotEqual {
		equal = !equal
	}

	return Value{Kind: ValueBoolean, Boolean: equal}, nil
}

func valuesEqual(left, right Value) (bool, error) {
	switch left.Kind {
	case ValueInt:
		return left.Int == right.Int, nil
	case ValueFloat:
		return left.Float == right.Float, nil
	case ValueBoolean:
		return left.Boolean == right.Boolean, nil
	case ValueString:
		return left.String == right.String, nil
	default:
		return false, validationErrorf("unsupported equality comparison")
	}
}

func evaluateComparison(operator lexer.TokenType, left, right Value) (Value, error) {
	if !isNumericValue(left) || !isNumericValue(right) {
		return Value{}, validationErrorf("type mismatch: expected numeric operands for comparison")
	}
	return compareNumbers(operator, numericValue(left), numericValue(right))
}

func isNumericValue(value Value) bool {
	return value.Kind == ValueInt || value.Kind == ValueFloat
}

func numericValue(value Value) float64 {
	if value.Kind == ValueFloat {
		return value.Float
	}
	return float64(value.Int)
}

func compareNumbers(operator lexer.TokenType, left, right float64) (Value, error) {
	switch operator {
	case lexer.TokenLess:
		return Value{Kind: ValueBoolean, Boolean: left < right}, nil
	case lexer.TokenLessEqual:
		return Value{Kind: ValueBoolean, Boolean: left <= right}, nil
	case lexer.TokenGreater:
		return Value{Kind: ValueBoolean, Boolean: left > right}, nil
	case lexer.TokenGreaterEqual:
		return Value{Kind: ValueBoolean, Boolean: left >= right}, nil
	default:
		return Value{}, validationErrorf("unknown comparison operator")
	}
}

func evaluateLogicalAnd(expr ast.InfixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	left, err := evaluateExpression(expr.Left, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if left.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	if !left.Boolean {
		return Value{Kind: ValueBoolean, Boolean: false}, nil
	}

	right, err := evaluateExpression(expr.Right, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if right.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	return Value{Kind: ValueBoolean, Boolean: right.Boolean}, nil
}

func evaluateLogicalOr(expr ast.InfixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	left, err := evaluateExpression(expr.Left, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if left.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	if left.Boolean {
		return Value{Kind: ValueBoolean, Boolean: true}, nil
	}

	right, err := evaluateExpression(expr.Right, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if right.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	return Value{Kind: ValueBoolean, Boolean: right.Boolean}, nil
}

func evaluateConditional(expr ast.ConditionalExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	condition, err := evaluateExpression(expr.Condition, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if condition.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean condition")
	}

	if condition.Boolean {
		return evaluateExpression(expr.Then, environment, self, symbols, types, schemas, enums)
	}

	return evaluateExpression(expr.Else, environment, self, symbols, types, schemas, enums)
}

func evaluateArrayLiteral(expr ast.ArrayLiteral, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	values := make([]Value, 0, len(expr.Elements))
	for _, element := range expr.Elements {
		value, err := evaluateExpression(element, environment, self, symbols, types, schemas, enums)
		if err != nil {
			return Value{}, err
		}
		values = append(values, value)
	}
	return Value{Kind: ValueArray, Array: values}, nil
}

func evaluateRecordLiteral(expr ast.RecordLiteral, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (Value, error) {
	fields := map[string]Value{}
	for _, field := range expr.Fields {
		if _, exists := fields[field.Name]; exists {
			return Value{}, validationErrorf("duplicate record field %q", field.Name)
		}
		value, err := evaluateExpression(field.Value, environment, self, symbols, types, schemas, enums)
		if err != nil {
			return Value{}, err
		}
		fields[field.Name] = value
	}
	return Value{Kind: ValueRecord, Record: fields}, nil
}

func evaluateSelfReference(expr ast.SelfReference, self Value) (Value, error) {
	if self.Kind != ValueRecord {
		return Value{}, validationErrorf("self reference is unavailable")
	}

	current := self
	for _, name := range expr.Path {
		if current.Kind != ValueRecord {
			return Value{}, validationErrorf("self reference points to non-record value")
		}
		next, ok := current.Record[name]
		if !ok {
			return Value{}, validationErrorf("unknown self reference %q", name)
		}
		current = next
	}
	return current, nil
}

func valueTypeFromValue(value Value) valueType {
	switch value.Kind {
	case ValueArray:
		if len(value.Array) == 0 {
			element := valueType{kind: ValueUnknown}
			return valueType{kind: ValueArray, element: &element}
		}
		element := valueTypeFromValue(value.Array[0])
		return valueType{kind: ValueArray, element: &element}
	case ValueRecord:
		return valueType{kind: ValueRecord}
	default:
		return valueType{kind: value.Kind}
	}
}

func (value Value) kindName() string {
	switch value.Kind {
	case ValueArray:
		return "array"
	case ValueInt:
		return "int"
	case ValueFloat:
		return "float"
	case ValueBoolean:
		return "boolean"
	case ValueRecord:
		return "record"
	case ValueString:
		return "string"
	default:
		return "unknown"
	}
}

type ValueKind int

const (
	ValueUnknown ValueKind = iota
	ValueString
	ValueInt
	ValueFloat
	ValueBoolean
	ValueArray
	ValueRecord
)

type valueType struct {
	kind       ValueKind
	element    *valueType
	schemaName string
	record     *ast.RecordType
	enumName   string
	members    []valueType
}

func (t valueType) isNumeric() bool {
	return t.kind == ValueInt || t.kind == ValueFloat
}

func (t valueType) name() string {
	switch t.kind {
	case ValueString:
		if t.enumName != "" {
			return t.enumName
		}
		return "string"
	case ValueInt:
		if t.enumName != "" {
			return t.enumName
		}
		return "int"
	case ValueFloat:
		return "float"
	case ValueBoolean:
		return "boolean"
	case ValueArray:
		if t.element != nil {
			return fmt.Sprintf("array<%s>", t.element.name())
		}
		return "array"
	case ValueRecord:
		if t.schemaName != "" {
			return t.schemaName
		}
		return "record"
	default:
		if len(t.members) > 0 {
			parts := lo.Map(t.members, func(member valueType, _ int) string {
				return member.name()
			})
			return fmt.Sprintf("variant[%s]", strings.Join(parts, ", "))
		}
		return "unknown"
	}
}

func resolveValueType(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (valueType, error) {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return primitiveValueType(ref.Name)
	case ast.ArrayType:
		element, err := resolveValueType(ref.Element, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueArray, element: &element}, nil
	case ast.UnionType:
		if members, ok, err := resolveUnionEnumValueTypes(ref, types, enums, false); err != nil {
			return valueType{}, err
		} else if ok {
			return valueType{members: members}, nil
		}
		record, err := resolveUnionRecordType(ref, symbols, types, schemas)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueRecord, record: &record}, nil
	case ast.VariantType:
		if _, ok, err := resolveUnionEnumValueTypes(ast.UnionType(ref), types, enums, true); err != nil || ok {
			if err != nil {
				return valueType{}, err
			}
		}
		members := make([]valueType, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolved, err := resolveValueType(member, symbols, types, schemas, enums)
			if err != nil {
				return valueType{}, err
			}
			members = append(members, resolved)
		}
		if err := validateVariantValueTypes(members); err != nil {
			return valueType{}, err
		}
		return valueType{members: members}, nil
	case ast.RecordType:
		return valueType{kind: ValueRecord, record: &ref}, nil
	case ast.NamedType:
		if enumDef, ok := enums.Get(ref.Name); ok {
			return valueType{kind: enumDef.BackingType.kind, enumName: ref.Name}, nil
		}
		resolved, resolvedByAlias, err := types.Resolve(ref.Name)
		if err != nil {
			return valueType{}, err
		}
		if resolvedByAlias {
			return resolveValueType(resolved, symbols, types, schemas, enums)
		}
		if symbols.IsSchema(ref.Name) || symbols.IsImport(ref.Name) {
			record, ok := schemas.Get(ref.Name)
			if ok {
				return valueType{kind: ValueRecord, schemaName: ref.Name, record: &record}, nil
			}
			return valueType{kind: ValueRecord, schemaName: ref.Name}, nil
		}
		return valueType{}, validationErrorf("unknown type %q", ref.Name)
	default:
		return valueType{}, validationErrorf("unknown type reference")
	}
}

func validateVariantValueTypes(members []valueType) error {
	members = flattenVariantValueTypes(members)
	hasEnum := false
	hasSchema := false
	hasPrimitive := false
	enumBacking := ValueUnknown

	for _, member := range members {
		if member.kind == ValueArray {
			return validationErrorf("variant members must be primitives, schemas, or enums")
		}
		switch {
		case member.enumName != "":
			hasEnum = true
			if enumBacking == ValueUnknown {
				enumBacking = member.kind
			} else if enumBacking != member.kind {
				return validationErrorf("enum variants require the same backing type")
			}
		case member.kind == ValueRecord:
			hasSchema = true
		case member.kind == ValueString || member.kind == ValueInt || member.kind == ValueFloat || member.kind == ValueBoolean:
			hasPrimitive = true
		default:
			return validationErrorf("variant members must be primitives, schemas, or enums")
		}
	}

	if hasEnum && (hasPrimitive || hasSchema) {
		return validationErrorf("enum variants may only combine enums with the same backing type")
	}

	return nil
}

func flattenVariantValueTypes(members []valueType) []valueType {
	flattened := make([]valueType, 0, len(members))
	for _, member := range members {
		if len(member.members) == 0 {
			flattened = append(flattened, member)
			continue
		}

		flattened = append(flattened, flattenVariantValueTypes(member.members)...)
	}

	return flattened
}

func resolveUnionEnumDefinition(name string, typeRef ast.TypeReference, types *typeRegistry, enums *enumRegistry) (enumDefinition, bool, error) {
	ref, ok := typeRef.(ast.UnionType)
	if !ok {
		return enumDefinition{}, false, nil
	}

	definitions := make([]enumDefinition, 0, len(ref.Members))
	for _, member := range ref.Members {
		definition, ok, err := resolveUnionEnumMember(member, types, enums)
		if err != nil || !ok {
			return enumDefinition{}, false, err
		}
		definitions = append(definitions, definition)
	}

	definition, err := mergeEnumDefinitions(name, definitions)
	return definition, true, err
}

func resolveUnionEnumValueTypes(typeRef ast.TypeReference, types *typeRegistry, enums *enumRegistry, requireUniqueKeys bool) ([]valueType, bool, error) {
	ref, ok := typeRef.(ast.UnionType)
	if !ok {
		return nil, false, nil
	}

	members := make([]valueType, 0, len(ref.Members))
	memberNames := map[string]struct{}{}
	for _, member := range ref.Members {
		definition, ok, err := resolveUnionEnumMember(member, types, enums)
		if err != nil || !ok {
			return nil, false, err
		}
		if requireUniqueKeys {
			for _, enumMember := range definition.Members {
				if _, exists := memberNames[enumMember.Name]; exists {
					return nil, false, validationErrorf("duplicate enum member %q in enum variant", enumMember.Name)
				}
				memberNames[enumMember.Name] = struct{}{}
			}
		}
		members = append(members, valueType{kind: definition.BackingType.kind, enumName: definition.Name})
	}

	return members, true, nil
}

func resolveUnionEnumMember(typeRef ast.TypeReference, types *typeRegistry, enums *enumRegistry) (enumDefinition, bool, error) {
	switch ref := typeRef.(type) {
	case ast.NamedType:
		if enumDef, ok := enums.Get(ref.Name); ok {
			return enumDef, true, nil
		}

		resolved, resolvedByAlias, err := types.Resolve(ref.Name)
		if err != nil {
			return enumDefinition{}, false, err
		}
		if resolvedByAlias {
			return resolveUnionEnumMember(resolved, types, enums)
		}
	}

	return enumDefinition{}, false, nil
}

func resolveUnionRecordType(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry) (ast.RecordType, error) {
	switch ref := typeRef.(type) {
	case ast.RecordType:
		return ref, nil
	case ast.UnionType:
		merged := ast.RecordType{}
		for _, member := range ref.Members {
			record, err := resolveUnionRecordType(member, symbols, types, schemas)
			if err != nil {
				return ast.RecordType{}, err
			}
			merged, err = mergeRecordTypes(merged, record)
			if err != nil {
				return ast.RecordType{}, err
			}
		}
		return merged, nil
	case ast.NamedType:
		if record, ok := schemas.Get(ref.Name); ok {
			return record, nil
		}

		resolved, resolvedByAlias, err := types.Resolve(ref.Name)
		if err != nil {
			return ast.RecordType{}, err
		}
		if resolvedByAlias {
			return resolveUnionRecordType(resolved, symbols, types, schemas)
		}

		return ast.RecordType{}, validationErrorf("union members must be schemas")
	default:
		return ast.RecordType{}, validationErrorf("union members must be schemas")
	}
}

func mergeRecordTypes(left, right ast.RecordType) (ast.RecordType, error) {
	if len(left.Fields) == 0 {
		return right, nil
	}
	if len(right.Fields) == 0 {
		return left, nil
	}

	merged := ast.RecordType{Fields: append([]ast.SchemaField{}, left.Fields...)}
	fieldIndexes := map[string]int{}
	for index, field := range merged.Fields {
		fieldIndexes[field.Name] = index
	}

	for _, field := range right.Fields {
		index, exists := fieldIndexes[field.Name]
		if !exists {
			fieldIndexes[field.Name] = len(merged.Fields)
			merged.Fields = append(merged.Fields, field)
			continue
		}

		existing := merged.Fields[index]
		if !reflect.DeepEqual(existing.Type, field.Type) {
			return ast.RecordType{}, validationErrorf("conflicting field %q in union schema composition", field.Name)
		}
		merged.Fields[index].Optional = existing.Optional && field.Optional
	}

	return merged, nil
}

func primitiveValueType(name string) (valueType, error) {
	switch name {
	case "string":
		return valueType{kind: ValueString}, nil
	case "int":
		return valueType{kind: ValueInt}, nil
	case "float":
		return valueType{kind: ValueFloat}, nil
	case "boolean":
		return valueType{kind: ValueBoolean}, nil
	default:
		return valueType{}, validationErrorf("unknown type %q", name)
	}
}

func schemaTypeFromTypeReference(typeRef ast.TypeReference) (SchemaType, error) {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return SchemaType{Kind: SchemaTypePrimitive, Name: ref.Name}, nil
	case ast.NamedType:
		return SchemaType{Kind: SchemaTypeNamed, Name: ref.Name}, nil
	case ast.ArrayType:
		element, err := schemaTypeFromTypeReference(ref.Element)
		if err != nil {
			return SchemaType{}, err
		}

		return SchemaType{Kind: SchemaTypeArray, Element: &element}, nil
	case ast.UnionType:
		members := make([]SchemaType, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolved, err := schemaTypeFromTypeReference(member)
			if err != nil {
				return SchemaType{}, err
			}
			members = append(members, resolved)
		}
		return SchemaType{Kind: SchemaTypeUnion, Members: members}, nil
	case ast.VariantType:
		members := make([]SchemaType, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolved, err := schemaTypeFromTypeReference(member)
			if err != nil {
				return SchemaType{}, err
			}
			members = append(members, resolved)
		}
		return SchemaType{Kind: SchemaTypeVariant, Members: members}, nil
	case ast.RecordType:
		fields := make(map[SchemaField]SchemaType, len(ref.Fields))
		for _, field := range ref.Fields {
			fieldType, err := schemaTypeFromTypeReference(field.Type)
			if err != nil {
				return SchemaType{}, err
			}

			fields[SchemaField{Name: field.Name, Optional: field.Optional}] = fieldType
		}

		return SchemaType{Kind: SchemaTypeRecord, Fields: fields}, nil
	default:
		return SchemaType{}, validationErrorf("unknown type reference")
	}
}

func inferExpressionType(expression ast.Expression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (valueType, error) {
	switch expr := expression.(type) {
	case ast.Identifier:
		if variableType, ok := variables.Get(expr.Name); ok {
			return variableType, nil
		}
		if symbols != nil {
			if kind, ok := symbols.Get(expr.Name); ok && kind != symbolKindVariable {
				return valueType{}, validationErrorf("type reference %q is not a valid value expression", expr.Name)
			}
		}
		return valueType{kind: ValueUnknown}, nil
	case ast.MemberAccess:
		if identifier, ok := expr.Target.(ast.Identifier); ok {
			enumDef, exists := enums.Get(identifier.Name)
			if exists {
				if _, exists := enumDef.Member(expr.Name); !exists {
					return valueType{}, validationErrorf("unknown enum member %q in enum %q", expr.Name, identifier.Name)
				}
				return valueType{kind: enumDef.BackingType.kind, enumName: identifier.Name}, nil
			}
		}

		targetType, err := inferExpressionType(expr.Target, variables, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		if targetType.kind == ValueUnknown {
			return valueType{kind: ValueUnknown}, nil
		}
		if targetType.kind != ValueRecord {
			return valueType{}, validationErrorf("member access requires a record value")
		}
		if targetType.record == nil {
			if targetType.schemaName == "" {
				return valueType{kind: ValueUnknown}, nil
			}
			record, ok := schemas.Get(targetType.schemaName)
			if !ok {
				return valueType{kind: ValueUnknown}, nil
			}
			targetType.record = &record
		}
		for _, field := range targetType.record.Fields {
			if field.Name != expr.Name {
				continue
			}
			return resolveValueType(field.Type, symbols, types, schemas, enums)
		}
		return valueType{}, validationErrorf("unknown field %q", expr.Name)
	case ast.ArrayAccess:
		targetType, err := inferExpressionType(expr.Target, variables, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		if targetType.kind == ValueUnknown {
			return valueType{kind: ValueUnknown}, nil
		}
		if targetType.kind != ValueArray {
			return valueType{}, validationErrorf("array access requires an array value at level %d", arrayAccessLevel(expr))
		}
		if targetType.element == nil {
			return valueType{kind: ValueUnknown}, nil
		}
		return *targetType.element, nil
	case ast.IntLiteral:
		return valueType{kind: ValueInt}, nil
	case ast.FloatLiteral:
		return valueType{kind: ValueFloat}, nil
	case ast.StringLiteral:
		return valueType{kind: ValueString}, nil
	case ast.BooleanLiteral:
		return valueType{kind: ValueBoolean}, nil
	case ast.ArrayLiteral:
		return inferArrayLiteralType(expr, variables, symbols, types, schemas, enums)
	case ast.RecordLiteral:
		return valueType{kind: ValueRecord}, nil
	case ast.SelfReference:
		return valueType{kind: ValueUnknown}, nil
	case ast.PrefixExpression:
		return inferPrefixType(expr, variables, symbols, types, schemas, enums)
	case ast.InfixExpression:
		return inferInfixType(expr, variables, symbols, types, schemas, enums)
	case ast.ConditionalExpression:
		return inferConditionalType(expr, variables, symbols, types, schemas, enums)
	default:
		return valueType{}, validationErrorf("unknown expression")
	}
}

func arrayAccessLevel(expression ast.Expression) int {
	access, ok := expression.(ast.ArrayAccess)
	if !ok {
		return 0
	}

	if parent, ok := access.Target.(ast.ArrayAccess); ok {
		return arrayAccessLevel(parent) + 1
	}

	return 1
}

func inferArrayLiteralType(expr ast.ArrayLiteral, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (valueType, error) {
	if len(expr.Elements) == 0 {
		return valueType{kind: ValueArray, element: &valueType{kind: ValueUnknown}}, nil
	}

	elementTypes := []valueType{}
	for _, element := range expr.Elements {
		elementType, err := inferExpressionType(element, variables, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		elementTypes = appendUniqueValueType(elementTypes, elementType)
	}

	if len(elementTypes) == 1 {
		return valueType{kind: ValueArray, element: &elementTypes[0]}, nil
	}

	elementType := valueType{members: elementTypes}
	return valueType{kind: ValueArray, element: &elementType}, nil
}

func inferPrefixType(expr ast.PrefixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (valueType, error) {
	rightType, err := inferExpressionType(expr.Right, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}

	switch expr.Operator {
	case lexer.TokenBang:
		if rightType.kind != ValueBoolean {
			return valueType{}, validationErrorf("type mismatch: expected boolean after '!'")
		}
		return valueType{kind: ValueBoolean}, nil
	case lexer.TokenTilde:
		if rightType.kind != ValueInt {
			return valueType{}, validationErrorf("type mismatch: expected int after '~'")
		}
		return valueType{kind: ValueInt}, nil
	case lexer.TokenPlus, lexer.TokenMinus:
		if !rightType.isNumeric() {
			return valueType{}, validationErrorf("type mismatch: expected numeric after unary operator")
		}
		return rightType, nil
	default:
		return valueType{}, validationErrorf("unknown prefix operator")
	}
}

func inferInfixType(expr ast.InfixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (valueType, error) {
	leftType, err := inferExpressionType(expr.Left, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}
	rightType, err := inferExpressionType(expr.Right, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}

	switch expr.Operator {
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenDoubleStar:
		return inferNumericBinary(expr.Operator, leftType, rightType)
	case lexer.TokenPercent:
		if !leftType.isNumeric() || !rightType.isNumeric() {
			return valueType{}, validationErrorf("type mismatch: expected numeric operands for '%%'")
		}
		if leftType.kind == ValueInt && rightType.kind == ValueInt {
			return valueType{kind: ValueInt}, nil
		}
		return valueType{kind: ValueFloat}, nil
	case lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned:
		if leftType.kind != ValueInt || rightType.kind != ValueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for shift")
		}
		return valueType{kind: ValueInt}, nil
	case lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret:
		if leftType.kind != ValueInt || rightType.kind != ValueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for bitwise operator")
		}
		return valueType{kind: ValueInt}, nil
	case lexer.TokenEqualEqual, lexer.TokenNotEqual, lexer.TokenStrictEqual, lexer.TokenStrictNotEqual:
		if leftType.kind != ValueUnknown && rightType.kind != ValueUnknown && !typesEqual(leftType, rightType) {
			return valueType{}, validationErrorf("type mismatch: incompatible equality comparison")
		}
		return valueType{kind: ValueBoolean}, nil
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		if !leftType.isNumeric() || !rightType.isNumeric() {
			return valueType{}, validationErrorf("type mismatch: expected numeric operands for comparison")
		}
		return valueType{kind: ValueBoolean}, nil
	case lexer.TokenAndAnd, lexer.TokenOrOr:
		if leftType.kind != ValueBoolean || rightType.kind != ValueBoolean {
			return valueType{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
		}
		return valueType{kind: ValueBoolean}, nil
	default:
		return valueType{}, validationErrorf("unknown infix operator")
	}
}

func inferNumericBinary(operator lexer.TokenType, leftType, rightType valueType) (valueType, error) {
	if !leftType.isNumeric() || !rightType.isNumeric() {
		return valueType{}, validationErrorf("type mismatch: expected numeric operands for operator")
	}
	if leftType.kind == ValueInt && rightType.kind == ValueInt {
		return valueType{kind: ValueInt}, nil
	}
	return valueType{kind: ValueFloat}, nil
}

func inferConditionalType(expr ast.ConditionalExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) (valueType, error) {
	conditionType, err := inferExpressionType(expr.Condition, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}
	if conditionType.kind != ValueBoolean {
		return valueType{}, validationErrorf("type mismatch: expected boolean condition")
	}

	thenType, err := inferExpressionType(expr.Then, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}
	elseType, err := inferExpressionType(expr.Else, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}

	if thenType.kind != ValueUnknown && elseType.kind != ValueUnknown && thenType.kind != elseType.kind {
		return valueType{}, validationErrorf("type mismatch: conditional branches differ")
	}

	if thenType.kind == ValueUnknown {
		return elseType, nil
	}

	return thenType, nil
}

func typesEqual(leftType, rightType valueType) bool {
	if len(leftType.members) > 0 || len(rightType.members) > 0 {
		if len(leftType.members) != len(rightType.members) {
			return false
		}
		for index := range leftType.members {
			if !typesEqual(leftType.members[index], rightType.members[index]) {
				return false
			}
		}
		return true
	}
	if leftType.kind != rightType.kind {
		return false
	}
	if leftType.kind == ValueArray {
		if leftType.element == nil || rightType.element == nil {
			return false
		}
		return typesEqual(*leftType.element, *rightType.element)
	}
	if leftType.kind == ValueRecord {
		if leftType.schemaName == "" || rightType.schemaName == "" {
			return true
		}
		return leftType.schemaName == rightType.schemaName
	}
	if (leftType.enumName == "") != (rightType.enumName == "") {
		return false
	}
	if leftType.enumName != "" && rightType.enumName != "" {
		return leftType.enumName == rightType.enumName
	}
	return true
}

func ensureAssignable(expectedType, actualType valueType) error {
	if len(expectedType.members) > 0 {
		if len(actualType.members) > 0 {
			for _, actualMember := range actualType.members {
				if err := ensureAssignable(expectedType, actualMember); err != nil {
					return err
				}
			}
			return nil
		}
		for _, member := range expectedType.members {
			if err := ensureAssignable(member, actualType); err == nil {
				return nil
			}
		}
		return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
	}
	if len(actualType.members) > 0 {
		return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
	}
	if expectedType.kind == ValueUnknown {
		return nil
	}
	if actualType.kind == ValueUnknown {
		return validationErrorf("type mismatch: expected %s, got unknown", expectedType.name())
	}
	if expectedType.kind != actualType.kind {
		return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
	}
	if (expectedType.enumName == "") != (actualType.enumName == "") {
		return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
	}
	if expectedType.enumName != "" && actualType.enumName != "" && expectedType.enumName != actualType.enumName {
		return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
	}
	if expectedType.kind == ValueRecord {
		if expectedType.schemaName != "" && actualType.schemaName != "" && expectedType.schemaName != actualType.schemaName {
			return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
		}
	}
	if expectedType.kind == ValueArray {
		if expectedType.element == nil || actualType.element == nil {
			return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
		}
		if err := ensureAssignable(*expectedType.element, *actualType.element); err != nil {
			return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
		}
		return nil
	}
	return nil
}

func appendUniqueValueType(types []valueType, next valueType) []valueType {
	for _, existing := range types {
		if typesEqual(existing, next) {
			return types
		}
	}

	return append(types, next)
}

type symbolKind int

const (
	symbolKindImport symbolKind = iota
	symbolKindType
	symbolKindSchema
	symbolKindVariable
	symbolKindEnum
)

type symbolTable struct {
	symbols map[string]symbolKind
}

func newSymbolTable() *symbolTable {
	return &symbolTable{
		symbols: map[string]symbolKind{},
	}
}

func (s *symbolTable) Clone() *symbolTable {
	cloned := newSymbolTable()
	for name, kind := range s.symbols {
		cloned.symbols[name] = kind
	}

	return cloned
}

func (s *symbolTable) Add(name string, kind symbolKind) {
	s.symbols[name] = kind
}

func (s *symbolTable) Has(name string) bool {
	_, exists := s.symbols[name]
	return exists
}

func (s *symbolTable) Get(name string) (symbolKind, bool) {
	kind, exists := s.symbols[name]
	return kind, exists
}

func (s *symbolTable) IsImport(name string) bool {
	kind, exists := s.symbols[name]
	return exists && kind == symbolKindImport
}

func (s *symbolTable) IsType(name string) bool {
	kind, exists := s.symbols[name]
	return exists && kind == symbolKindType
}

func (s *symbolTable) IsSchema(name string) bool {
	kind, exists := s.symbols[name]
	return exists && kind == symbolKindSchema
}

func (s *symbolTable) IsVariable(name string) bool {
	kind, exists := s.symbols[name]
	return exists && kind == symbolKindVariable
}

func (s *symbolTable) IsEnum(name string) bool {
	kind, exists := s.symbols[name]
	return exists && kind == symbolKindEnum
}

type typeRegistry struct {
	aliases map[string]ast.TypeReference
}

func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		aliases: map[string]ast.TypeReference{},
	}
}

func (t *typeRegistry) Clone() *typeRegistry {
	cloned := newTypeRegistry()
	for name, reference := range t.aliases {
		cloned.aliases[name] = reference
	}

	return cloned
}

func (t *typeRegistry) AddAlias(name string, reference ast.TypeReference) {
	t.aliases[name] = reference
}

func (t *typeRegistry) Resolve(name string) (ast.TypeReference, bool, error) {
	reference, exists := t.aliases[name]
	if !exists {
		return nil, false, nil
	}

	visited := map[string]struct{}{name: {}}
	resolved, err := t.resolveTypeReference(reference, visited)
	if err != nil {
		return nil, true, err
	}

	return resolved, true, nil
}

func (t *typeRegistry) resolveTypeReference(reference ast.TypeReference, visited map[string]struct{}) (ast.TypeReference, error) {
	named, ok := reference.(ast.NamedType)
	if !ok {
		return reference, nil
	}

	if _, exists := visited[named.Name]; exists {
		return nil, validationErrorf("cyclic type alias %q", named.Name)
	}

	alias, exists := t.aliases[named.Name]
	if !exists {
		return reference, nil
	}

	visited[named.Name] = struct{}{}
	return t.resolveTypeReference(alias, visited)
}

type schemaRegistry struct {
	schemas map[string]ast.RecordType
}

func newSchemaRegistry() *schemaRegistry {
	return &schemaRegistry{
		schemas: map[string]ast.RecordType{},
	}
}

func (s *schemaRegistry) Clone() *schemaRegistry {
	cloned := newSchemaRegistry()
	for name, record := range s.schemas {
		cloned.schemas[name] = record
	}

	return cloned
}

func (s *schemaRegistry) Add(name string, record ast.RecordType) {
	s.schemas[name] = record
}

func (s *schemaRegistry) Get(name string) (ast.RecordType, bool) {
	record, exists := s.schemas[name]
	return record, exists
}

type variableRegistry struct {
	variables map[string]valueType
}

func newVariableRegistry() *variableRegistry {
	return &variableRegistry{
		variables: map[string]valueType{},
	}
}

func (v *variableRegistry) Clone() *variableRegistry {
	cloned := newVariableRegistry()
	for name, valueType := range v.variables {
		cloned.variables[name] = valueType
	}

	return cloned
}

func (v *variableRegistry) Add(name string, valueType valueType) {
	v.variables[name] = valueType
}

func (v *variableRegistry) Get(name string) (valueType, bool) {
	valueType, exists := v.variables[name]
	return valueType, exists
}

type validationError struct {
	message string
}

func (err validationError) Error() string {
	return err.message
}

func validationErrorf(format string, args ...any) error {
	return validationError{message: fmt.Sprintf("processor: %s", fmt.Sprintf(format, args...))}
}
