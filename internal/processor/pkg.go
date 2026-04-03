package processor

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"

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
)

type SchemaType struct {
	Kind    SchemaTypeKind
	Name    string
	Element *SchemaType
	Fields  map[SchemaField]SchemaType
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

	value, err := evaluateExpression(expression, newValueEnvironment(), Value{})
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

	context, err := buildProcessContext(nil, &script, baseDir, p.injections)
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
		if err := validateSchemaOutputFields(outputBlock.SchemaFields, outputContext.symbols, outputContext.types); err != nil {
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

	output, err := evaluateOutputFields(outputBlock.DataFields, outputContext.environment)
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
	value, err := parseString(literal.Lexeme)
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

		parsedPath, err := parseString(directive.Value)
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
		if err := validateSchemaOutputFields(output.SchemaFields, outputContext.symbols, outputContext.types); err != nil {
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

	values, err := evaluateOutputFields(output.DataFields, outputContext.environment)
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

	inferredType, err := inferExpressionType(field.Value, context.variables, context.symbols, context.types)
	if err != nil {
		return valueType{}, err
	}

	return sanitizeImportedValueType(inferredType, context.schemas), nil
}

func sanitizeImportedValueType(input valueType, schemas *schemaRegistry) valueType {
	if input.kind != ValueArray && input.kind != ValueRecord {
		return input
	}

	sanitized := input
	if sanitized.element != nil {
		element := sanitizeImportedValueType(*sanitized.element, schemas)
		sanitized.element = &element
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
		default:
			return validationErrorf("unknown declaration")
		}
	}

	return nil
}

func validateDeclarations(items []ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry, variables *variableRegistry, injections map[string]Value) error {
	for _, declaration := range items {
		if err := validateDeclaration(declaration, symbols, types, schemas, enums, variables, injections); err != nil {
			return err
		}
	}

	return nil
}

func validateDeclaration(declaration ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry, variables *variableRegistry, injections map[string]Value) error {
	switch decl := declaration.(type) {
	case ast.VariableDeclaration:
		if err := validateTypeReference(decl.Type, symbols, types); err != nil {
			return err
		}
		expectedType, err := resolveValueType(decl.Type, symbols, types, schemas, enums)
		if err != nil {
			return err
		}

		if decl.HasValue {
			actualType, err := inferExpressionType(decl.Value, variables, symbols, types)
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
			} else if !decl.HasValue {
				return validationErrorf("injectable %q requires a runtime value", decl.Name)
			}
		}
		variables.Add(decl.Name, expectedType)
		return nil
	case ast.TypeDeclaration:
		return validateTypeReference(decl.Type, symbols, types)
	case ast.SchemaDeclaration:
		return validateRecordType(decl.Type, symbols, types)
	case ast.EnumDeclaration:
		_, err := enumDefinitionFromDeclaration(decl)
		return err
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

func validateTypeReference(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry) error {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return nil
	case ast.ArrayType:
		return validateTypeReference(ref.Element, symbols, types)
	case ast.RecordType:
		return validateRecordType(ref, symbols, types)
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

func validateRecordType(record ast.RecordType, symbols *symbolTable, types *typeRegistry) error {
	fieldNames := map[string]struct{}{}
	for _, field := range record.Fields {
		if _, exists := fieldNames[field.Name]; exists {
			return validationErrorf("duplicate field %q", field.Name)
		}
		fieldNames[field.Name] = struct{}{}

		if err := validateTypeReference(field.Type, symbols, types); err != nil {
			return err
		}
	}

	return nil
}

func validateOutputDirectiveStructure(output ast.OutputBlock) error {
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

func validateSchemaOutputFields(fields []ast.OutputSchemaField, symbols *symbolTable, types *typeRegistry) error {
	fieldNames := map[string]struct{}{}
	for _, field := range fields {
		if _, exists := fieldNames[field.Name]; exists {
			return validationErrorf("duplicate output field %q", field.Name)
		}
		fieldNames[field.Name] = struct{}{}

		if err := validateSchemaOutputFieldType(field.Type, symbols); err != nil {
			return err
		}

		if err := validateTypeReference(field.Type, symbols, types); err != nil {
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
		actualType, err := inferExpressionType(item.Value, variables, symbols, types)
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
	switch expectedType.kind {
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
		if expectedType.schemaName == "" {
			return nil
		}
		switch typed := expression.(type) {
		case ast.RecordLiteral:
			return validateRecordLiteral(typed, expectedType.schemaName, variables, symbols, types, schemas, enums)
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

func validateRecordLiteral(expr ast.RecordLiteral, schemaName string, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums *enumRegistry) error {
	schema, ok := schemas.Get(schemaName)
	if !ok {
		return validationErrorf("unknown schema %q", schemaName)
	}

	fieldsByName := map[string]ast.RecordField{}
	for _, field := range expr.Fields {
		if _, exists := fieldsByName[field.Name]; exists {
			return validationErrorf("duplicate record field %q", field.Name)
		}
		fieldsByName[field.Name] = field
	}

	schemaFields := schemaFieldMap(schema)
	for _, field := range schema.Fields {
		recordField, exists := fieldsByName[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			return validationErrorf("missing required field %q for schema %q", field.Name, schemaName)
		}
		if recordField.Optional && !field.Optional {
			return validationErrorf("field %q is not optional in schema %q", field.Name, schemaName)
		}
		expectedType, err := resolveValueType(field.Type, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		actualType, err := inferExpressionType(recordField.Value, variables, symbols, types)
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
			return validationErrorf("unknown field %q for schema %q", name, schemaName)
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
		if expectedType.schemaName == "" {
			return nil
		}
		schema, ok := schemas.Get(expectedType.schemaName)
		if !ok {
			return validationErrorf("unknown schema %q", expectedType.schemaName)
		}
		if value.Kind != ValueRecord {
			return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), value.kindName())
		}

		schemaFields := schemaFieldMap(schema)
		for _, field := range schema.Fields {
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
	values map[string]Value
}

func newValueEnvironment() *valueEnvironment {
	return &valueEnvironment{
		values: map[string]Value{},
	}
}

func (environment *valueEnvironment) Add(name string, value Value) {
	environment.values[name] = value
}

func (environment *valueEnvironment) Get(name string) (Value, bool) {
	value, ok := environment.values[name]
	return value, ok
}

func (environment *valueEnvironment) Values() map[string]Value {
	values := make(map[string]Value, len(environment.values))
	for name, value := range environment.values {
		values[name] = value
	}

	return values
}

func (environment *valueEnvironment) Clone() *valueEnvironment {
	return &valueEnvironment{values: environment.Values()}
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
				return validationErrorf("injectable %q requires a runtime value", variable.Name)
			}
		}

		if !variable.HasValue {
			return validationErrorf("variable %q requires an initializer", variable.Name)
		}

		value, err := evaluateExpression(variable.Value, environment, Value{})
		if err != nil {
			return err
		}

		expectedType, err := resolveValueType(variable.Type, symbols, types, schemas, enums)
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

func evaluateOutputFields(items []ast.OutputField, environment *valueEnvironment) (map[string]Value, error) {
	fields := map[string]Value{}
	self := Value{Kind: ValueRecord, Record: fields}
	for _, item := range items {
		value, err := evaluateExpression(item.Value, environment, self)
		if err != nil {
			return nil, err
		}
		fields[item.Name] = value
	}

	return fields, nil
}

func evaluateExpression(expression ast.Expression, environment *valueEnvironment, self Value) (Value, error) {
	switch expr := expression.(type) {
	case ast.Identifier:
		value, ok := environment.Get(expr.Name)
		if !ok {
			return Value{}, validationErrorf("unknown identifier %q", expr.Name)
		}
		return value, nil
	case ast.IntLiteral:
		return parseInt(expr.Lexeme)
	case ast.FloatLiteral:
		return parseFloat(expr.Lexeme)
	case ast.StringLiteral:
		return parseString(expr.Lexeme)
	case ast.BooleanLiteral:
		return Value{Kind: ValueBoolean, Boolean: expr.Value}, nil
	case ast.ArrayLiteral:
		return evaluateArrayLiteral(expr, environment, self)
	case ast.RecordLiteral:
		return evaluateRecordLiteral(expr, environment, self)
	case ast.SelfReference:
		return evaluateSelfReference(expr, self)
	case ast.PrefixExpression:
		return evaluatePrefix(expr, environment, self)
	case ast.InfixExpression:
		return evaluateInfix(expr, environment, self)
	case ast.ConditionalExpression:
		return evaluateConditional(expr, environment, self)
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

func parseString(lexeme string) (Value, error) {
	if len(lexeme) < 2 || lexeme[0] != '"' || lexeme[len(lexeme)-1] != '"' {
		return Value{}, validationErrorf("invalid string literal %q", lexeme)
	}
	return Value{Kind: ValueString, String: lexeme[1 : len(lexeme)-1]}, nil
}

func evaluatePrefix(expr ast.PrefixExpression, environment *valueEnvironment, self Value) (Value, error) {
	right, err := evaluateExpression(expr.Right, environment, self)
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

func evaluateInfix(expr ast.InfixExpression, environment *valueEnvironment, self Value) (Value, error) {
	if expr.Operator == lexer.TokenAndAnd {
		return evaluateLogicalAnd(expr, environment, self)
	}
	if expr.Operator == lexer.TokenOrOr {
		return evaluateLogicalOr(expr, environment, self)
	}

	left, err := evaluateExpression(expr.Left, environment, self)
	if err != nil {
		return Value{}, err
	}
	right, err := evaluateExpression(expr.Right, environment, self)
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
	if left.Kind != right.Kind {
		return Value{}, validationErrorf("type mismatch: expected %s operands", left.kindName())
	}

	switch left.Kind {
	case ValueInt:
		return evaluateIntNumeric(operator, left.Int, right.Int)
	case ValueFloat:
		return evaluateFloatNumeric(operator, left.Float, right.Float)
	default:
		return Value{}, validationErrorf("type mismatch: expected numeric operands for operator")
	}
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
	if left.Kind != ValueInt || right.Kind != ValueInt {
		return Value{}, validationErrorf("type mismatch: expected int operands for '%%'")
	}
	if right.Int == 0 {
		return Value{}, validationErrorf("division by zero")
	}
	return Value{Kind: ValueInt, Int: left.Int % right.Int}, nil
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
	if left.Kind != right.Kind {
		return Value{}, validationErrorf("type mismatch: expected %s operands", left.kindName())
	}

	switch left.Kind {
	case ValueInt:
		return compareNumbers(operator, float64(left.Int), float64(right.Int))
	case ValueFloat:
		return compareNumbers(operator, left.Float, right.Float)
	default:
		return Value{}, validationErrorf("type mismatch: expected numeric operands for comparison")
	}
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

func evaluateLogicalAnd(expr ast.InfixExpression, environment *valueEnvironment, self Value) (Value, error) {
	left, err := evaluateExpression(expr.Left, environment, self)
	if err != nil {
		return Value{}, err
	}
	if left.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	if !left.Boolean {
		return Value{Kind: ValueBoolean, Boolean: false}, nil
	}

	right, err := evaluateExpression(expr.Right, environment, self)
	if err != nil {
		return Value{}, err
	}
	if right.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	return Value{Kind: ValueBoolean, Boolean: right.Boolean}, nil
}

func evaluateLogicalOr(expr ast.InfixExpression, environment *valueEnvironment, self Value) (Value, error) {
	left, err := evaluateExpression(expr.Left, environment, self)
	if err != nil {
		return Value{}, err
	}
	if left.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	if left.Boolean {
		return Value{Kind: ValueBoolean, Boolean: true}, nil
	}

	right, err := evaluateExpression(expr.Right, environment, self)
	if err != nil {
		return Value{}, err
	}
	if right.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
	}
	return Value{Kind: ValueBoolean, Boolean: right.Boolean}, nil
}

func evaluateConditional(expr ast.ConditionalExpression, environment *valueEnvironment, self Value) (Value, error) {
	condition, err := evaluateExpression(expr.Condition, environment, self)
	if err != nil {
		return Value{}, err
	}
	if condition.Kind != ValueBoolean {
		return Value{}, validationErrorf("type mismatch: expected boolean condition")
	}

	if condition.Boolean {
		return evaluateExpression(expr.Then, environment, self)
	}

	return evaluateExpression(expr.Else, environment, self)
}

func evaluateArrayLiteral(expr ast.ArrayLiteral, environment *valueEnvironment, self Value) (Value, error) {
	values := make([]Value, 0, len(expr.Elements))
	var elementType *valueType
	for _, element := range expr.Elements {
		value, err := evaluateExpression(element, environment, self)
		if err != nil {
			return Value{}, err
		}
		currentType := valueTypeFromValue(value)
		if elementType == nil {
			elementType = &currentType
		} else if !typesEqual(*elementType, currentType) {
			return Value{}, validationErrorf("array literal has mixed element types")
		}
		values = append(values, value)
	}
	return Value{Kind: ValueArray, Array: values}, nil
}

func evaluateRecordLiteral(expr ast.RecordLiteral, environment *valueEnvironment, self Value) (Value, error) {
	fields := map[string]Value{}
	for _, field := range expr.Fields {
		if _, exists := fields[field.Name]; exists {
			return Value{}, validationErrorf("duplicate record field %q", field.Name)
		}
		value, err := evaluateExpression(field.Value, environment, self)
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
	enumName   string
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
	case ast.RecordType:
		return valueType{kind: ValueRecord}, nil
	case ast.NamedType:
		resolved, resolvedByAlias, err := types.Resolve(ref.Name)
		if err != nil {
			return valueType{}, err
		}
		if resolvedByAlias {
			return resolveValueType(resolved, symbols, types, schemas, enums)
		}
		if enumDef, ok := enums.Get(ref.Name); ok {
			return valueType{kind: enumDef.BackingType.kind, enumName: ref.Name}, nil
		}
		if symbols.IsSchema(ref.Name) || symbols.IsImport(ref.Name) {
			return valueType{kind: ValueRecord, schemaName: ref.Name}, nil
		}
		return valueType{}, validationErrorf("unknown type %q", ref.Name)
	default:
		return valueType{}, validationErrorf("unknown type reference")
	}
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

func inferExpressionType(expression ast.Expression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	switch expr := expression.(type) {
	case ast.Identifier:
		if variableType, ok := variables.Get(expr.Name); ok {
			return variableType, nil
		}
		return valueType{kind: ValueUnknown}, nil
	case ast.IntLiteral:
		return valueType{kind: ValueInt}, nil
	case ast.FloatLiteral:
		return valueType{kind: ValueFloat}, nil
	case ast.StringLiteral:
		return valueType{kind: ValueString}, nil
	case ast.BooleanLiteral:
		return valueType{kind: ValueBoolean}, nil
	case ast.ArrayLiteral:
		return inferArrayLiteralType(expr, variables, symbols, types)
	case ast.RecordLiteral:
		return valueType{kind: ValueRecord}, nil
	case ast.SelfReference:
		return valueType{kind: ValueUnknown}, nil
	case ast.PrefixExpression:
		return inferPrefixType(expr, variables, symbols, types)
	case ast.InfixExpression:
		return inferInfixType(expr, variables, symbols, types)
	case ast.ConditionalExpression:
		return inferConditionalType(expr, variables, symbols, types)
	default:
		return valueType{}, validationErrorf("unknown expression")
	}
}

func inferArrayLiteralType(expr ast.ArrayLiteral, variables *variableRegistry, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	if len(expr.Elements) == 0 {
		return valueType{kind: ValueArray, element: &valueType{kind: ValueUnknown}}, nil
	}

	firstType, err := inferExpressionType(expr.Elements[0], variables, symbols, types)
	if err != nil {
		return valueType{}, err
	}

	for _, element := range expr.Elements[1:] {
		elementType, err := inferExpressionType(element, variables, symbols, types)
		if err != nil {
			return valueType{}, err
		}
		if !typesEqual(firstType, elementType) {
			return valueType{}, validationErrorf("array literal has mixed element types")
		}
	}

	return valueType{kind: ValueArray, element: &firstType}, nil
}

func inferPrefixType(expr ast.PrefixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	rightType, err := inferExpressionType(expr.Right, variables, symbols, types)
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

func inferInfixType(expr ast.InfixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	leftType, err := inferExpressionType(expr.Left, variables, symbols, types)
	if err != nil {
		return valueType{}, err
	}
	rightType, err := inferExpressionType(expr.Right, variables, symbols, types)
	if err != nil {
		return valueType{}, err
	}

	switch expr.Operator {
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenStar, lexer.TokenSlash, lexer.TokenDoubleStar:
		return inferNumericBinary(expr.Operator, leftType, rightType)
	case lexer.TokenPercent:
		if leftType.kind != ValueInt || rightType.kind != ValueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for '%%'")
		}
		return valueType{kind: ValueInt}, nil
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
		if leftType.kind != ValueUnknown && rightType.kind != ValueUnknown && leftType.kind != rightType.kind {
			return valueType{}, validationErrorf("type mismatch: incompatible equality comparison")
		}
		return valueType{kind: ValueBoolean}, nil
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		if !leftType.isNumeric() || !rightType.isNumeric() {
			return valueType{}, validationErrorf("type mismatch: expected numeric operands for comparison")
		}
		if leftType.kind != rightType.kind {
			return valueType{}, validationErrorf("type mismatch: expected %s operands", leftType.name())
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
	if leftType.kind != rightType.kind {
		return valueType{}, validationErrorf("type mismatch: expected %s operands", leftType.name())
	}
	return valueType{kind: leftType.kind}, nil
}

func inferConditionalType(expr ast.ConditionalExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	conditionType, err := inferExpressionType(expr.Condition, variables, symbols, types)
	if err != nil {
		return valueType{}, err
	}
	if conditionType.kind != ValueBoolean {
		return valueType{}, validationErrorf("type mismatch: expected boolean condition")
	}

	thenType, err := inferExpressionType(expr.Then, variables, symbols, types)
	if err != nil {
		return valueType{}, err
	}
	elseType, err := inferExpressionType(expr.Else, variables, symbols, types)
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
	if leftType.enumName != "" && rightType.enumName != "" {
		return leftType.enumName == rightType.enumName
	}
	return true
}

func ensureAssignable(expectedType, actualType valueType) error {
	if expectedType.kind == ValueUnknown {
		return nil
	}
	if actualType.kind == ValueUnknown {
		return validationErrorf("type mismatch: expected %s, got unknown", expectedType.name())
	}
	if expectedType.kind != actualType.kind {
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
		return ensureAssignable(*expectedType.element, *actualType.element)
	}
	return nil
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
