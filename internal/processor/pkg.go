package processor

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/samber/lo"

	"github.com/louiss0/mace/internal/lexer"
	"github.com/louiss0/mace/internal/name"
	"github.com/louiss0/mace/internal/parser"
	"github.com/louiss0/mace/internal/parser/ast"
)

type Processor struct {
	input      map[string]Value
	inputError error
}

type Result struct {
	File      ast.File
	Output    map[string]Value
	Schema    map[SchemaField]SchemaType
	Variables map[string]Value
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
	SchemaTypeRecordMap
)

type SchemaType struct {
	Kind    SchemaTypeKind
	Name    string
	Element *SchemaType
	Fields  map[SchemaField]SchemaType
	Members []SchemaType
}

type processContext struct {
	importBaseDir string
	importRootDir string
	symbols       *symbolTable
	types         *typeRegistry
	schemas       *schemaRegistry
	variables     *variableRegistry
	environment   *valueEnvironment
}

func newProcessContext(importBaseDir string, importRootDir string) processContext {
	return processContext{
		importBaseDir: importBaseDir,
		importRootDir: importRootDir,
		symbols:       newSymbolTable(),
		types:         newTypeRegistry(),
		schemas:       newSchemaRegistry(),
		variables:     newVariableRegistry(),
		environment:   newValueEnvironment(),
	}
}

func (context processContext) clone() processContext {
	if context.symbols == nil {
		return processContext{}
	}

	return processContext{
		importBaseDir: context.importBaseDir,
		importRootDir: context.importRootDir,
		symbols:       context.symbols.Clone(),
		types:         context.types.Clone(),
		schemas:       context.schemas.Clone(),
		variables:     context.variables.Clone(),
		environment:   context.environment.Clone(),
	}
}

func New() *Processor {
	return &Processor{input: map[string]Value{}}
}

func NewWithInput(input map[string]Value) *Processor {
	cloned := make(map[string]Value, len(input))
	processor := &Processor{input: cloned}
	rawNames := make([]string, 0, len(input))
	for rawName := range input {
		rawNames = append(rawNames, rawName)
	}
	slices.Sort(rawNames)

	for _, rawName := range rawNames {
		semanticName := string(name.NormalizeName(rawName))
		if _, exists := cloned[semanticName]; exists {
			if processor.inputError == nil {
				processor.inputError = duplicateInputFieldError(semanticName)
			}
			continue
		}

		normalizedValue, err := normalizeInputValue(input[rawName])
		if err != nil {
			if processor.inputError == nil {
				processor.inputError = err
			}
			continue
		}
		cloned[semanticName] = normalizedValue
	}

	return processor
}

func NewWithInjections(injections map[string]Value) *Processor {
	return NewWithInput(injections)
}

var getwd = os.Getwd

func (p *Processor) Process(input string) (Result, error) {
	importBaseDir, err := getwd()
	if err != nil {
		importBaseDir = "."
	}

	return p.ProcessInDir(input, importBaseDir)
}

func (p *Processor) ProcessInDir(input string, importBaseDir string) (Result, error) {
	if importBaseDir == "" {
		importBaseDir = "."
	}

	return p.processInput(input, importBaseDir, importBaseDir, true)
}

func (p *Processor) ProcessInScope(input string, importBaseDir string, importRootDir string) (Result, error) {
	if importBaseDir == "" {
		importBaseDir = "."
	}
	if importRootDir == "" {
		importRootDir = importBaseDir
	}

	return p.processInput(input, importBaseDir, importRootDir, false)
}

func (p *Processor) ProcessScriptBlock(input string) (ScriptResult, error) {
	importBaseDir, err := getwd()
	if err != nil {
		importBaseDir = "."
	}

	return p.processScriptInput(input, importBaseDir)
}

func (p *Processor) ProcessVariablesInDir(input string, importBaseDir string) (map[string]Value, error) {
	return p.ProcessVariablesInScope(input, importBaseDir, importBaseDir)
}

func (p *Processor) ProcessVariablesInScope(input string, importBaseDir string, importRootDir string) (map[string]Value, error) {
	if err := p.validateInput(); err != nil {
		return nil, err
	}

	if importBaseDir == "" {
		importBaseDir = "."
	}
	if importRootDir == "" {
		importRootDir = importBaseDir
	}

	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return nil, err
	}

	context, err := buildProcessContext(file.Imports, file.Script, importBaseDir, importRootDir, false, p.input)
	if err != nil {
		return nil, err
	}

	return context.environment.Values(), nil
}

func (p *Processor) ProcessOutputBlock(input string, scriptResult ScriptResult) (Result, error) {
	importBaseDir := scriptResult.context.importBaseDir
	if importBaseDir == "" {
		cwd, err := getwd()
		if err != nil {
			importBaseDir = "."
		} else {
			importBaseDir = cwd
		}
	}

	return p.processOutputInput(input, scriptResult, importBaseDir)
}

func (p *Processor) ProcessFile(path string) (Result, error) {
	return p.ProcessFileInDir(path, filepath.Dir(path))
}

func (p *Processor) ProcessFileInDir(path string, importRootDir string) (Result, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Result{}, validationErrorf("unable to read file %q", path)
	}

	if importRootDir == "" {
		importRootDir = filepath.Dir(path)
	}

	return p.processInput(string(contents), filepath.Dir(path), importRootDir, true)
}

func ParseInputRecord(input string) (map[string]Value, error) {
	tokens, err := lex(input)
	if err != nil {
		return nil, err
	}

	expression, err := parser.New(tokens).ParseExpression()
	if err != nil {
		return nil, err
	}

	value, err := evaluateExpression(expression, newValueEnvironment(), Value{}, newSymbolTable(), newTypeRegistry(), newSchemaRegistry(), nil)
	if err != nil {
		return nil, err
	}
	if value.Kind != ValueRecord {
		return nil, validationErrorf("input must be a record literal")
	}

	return value.Record, nil
}

func ParseInjectionRecord(input string) (map[string]Value, error) {
	return ParseInputRecord(input)
}

func cloneValueMap(values map[string]Value) map[string]Value {
	cloned := make(map[string]Value, len(values))
	for name, value := range values {
		cloned[name] = value
	}
	return cloned
}

func normalizeInputValue(value Value) (Value, error) {
	switch value.Kind {
	case ValueArray:
		array := make([]Value, len(value.Array))
		for index, element := range value.Array {
			normalizedElement, err := normalizeInputValue(element)
			if err != nil {
				return Value{}, err
			}
			array[index] = normalizedElement
		}
		value.Array = array
		return value, nil
	case ValueRecord:
		rawNames := make([]string, 0, len(value.Record))
		for rawName := range value.Record {
			rawNames = append(rawNames, rawName)
		}
		slices.Sort(rawNames)

		record := make(map[string]Value, len(value.Record))
		for _, rawName := range rawNames {
			semanticName := string(name.NormalizeName(rawName))
			if _, exists := record[semanticName]; exists {
				return Value{}, duplicateInputFieldError(semanticName)
			}

			normalizedValue, err := normalizeInputValue(value.Record[rawName])
			if err != nil {
				return Value{}, err
			}
			record[semanticName] = normalizedValue
		}
		value.Record = record
	}
	return value, nil
}

func duplicateInputFieldError(field string) error {
	return diagnosticErrorf(
		ErrorRuntime,
		CodeRuntimeDuplicateInputField,
		DiagnosticFields{Field: field},
		"duplicate input field %q after NFC normalization",
		field,
	)
}

func (p *Processor) validateInput() error {
	return p.inputError
}

func (p *Processor) processInput(input string, importBaseDir string, importRootDir string, enforceImportRoot bool) (Result, error) {
	if err := p.validateInput(); err != nil {
		return Result{}, err
	}

	tokens, err := lex(input)
	if err != nil {
		return Result{}, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return Result{}, err
	}

	context, err := buildProcessContext(file.Imports, file.Script, importBaseDir, importRootDir, enforceImportRoot, p.input)
	if err != nil {
		return Result{}, err
	}

	return p.processParsedOutput(file.Output, file, context)
}

func (p *Processor) processScriptInput(input string, importBaseDir string) (ScriptResult, error) {
	if err := p.validateInput(); err != nil {
		return ScriptResult{}, err
	}

	tokens, err := lex(input)
	if err != nil {
		return ScriptResult{}, err
	}

	script, err := parser.New(tokens).ParseScriptBlock()
	if err != nil {
		return ScriptResult{}, err
	}

	context, err := buildProcessContext(script.Imports, &script, importBaseDir, importBaseDir, true, p.input)
	if err != nil {
		return ScriptResult{}, err
	}

	return ScriptResult{
		Script:    script,
		Variables: context.environment.Values(),
		context:   context,
	}, nil
}

func (p *Processor) processOutputInput(input string, scriptResult ScriptResult, importBaseDir string) (Result, error) {
	if err := p.validateInput(); err != nil {
		return Result{}, err
	}

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
		context = newProcessContext(importBaseDir, importBaseDir)
	} else {
		context = context.clone()
		context.importBaseDir = importBaseDir
		if context.importRootDir == "" {
			context.importRootDir = importBaseDir
		}
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

	if err := validateOutputDocumentation(outputBlock.Directives, outputContext); err != nil {
		return Result{}, err
	}

	if outputBlock.Mode == ast.OutputModeSchema {
		if err := validateSchemaOutputScriptVariables(file); err != nil {
			return Result{}, err
		}
		if err := validateSchemaOutputFields(outputBlock.SchemaFields, outputContext.symbols, outputContext.types, outputContext.schemas, nil); err != nil {
			return Result{}, err
		}
		schema, _ := evaluateSchemaOutput(outputBlock, outputContext.types)

		return Result{File: file, Output: map[string]Value{}, Schema: schema, Variables: outputContext.environment.Values()}, nil
	}

	parsedInputErr := p.applyParsedOutputInput(outputBlock, &outputContext)
	if parsedInputErr != nil && !isRuntimeInputValidationError(parsedInputErr) {
		return Result{}, parsedInputErr
	}

	schemaName, hasSchema, _ := activeOutputSchemaName(outputBlock.Directives, outputContext)
	if hasParsedInputDirective(outputBlock.Directives) {
		if err := validateOutputExpressionReferences(outputBlock.DataFields, outputContext.variables); err != nil {
			return Result{}, err
		}
	}
	if err := validateDataOutputFields(outputBlock.DataFields, hasSchema, outputContext.variables, outputContext.symbols, outputContext.types, outputContext.schemas, nil); err != nil {
		return Result{}, err
	}

	if hasSchema {
		if err := validateOutputSchema(schemaName, outputBlock.DataFields, outputContext.variables, outputContext.symbols, outputContext.types, outputContext.schemas, nil); err != nil {
			return Result{}, err
		}
	}

	if parsedInputErr != nil {
		return Result{}, parsedInputErr
	}

	output, err := evaluateOutputFields(outputBlock.DataFields, outputContext.environment, outputContext.symbols, outputContext.types, outputContext.schemas, nil)
	if err != nil {
		return Result{}, err
	}

	if hasSchema {
		if err := validateEvaluatedOutputSchema(schemaName, output, outputContext.symbols, outputContext.types, outputContext.schemas, nil); err != nil {
			return Result{}, err
		}
	}

	return Result{File: file, Output: output, Schema: map[SchemaField]SchemaType{}, Variables: outputContext.environment.Values()}, nil
}

func validateSchemaOutputScriptVariables(file ast.File) error {
	if file.Output.Mode != ast.OutputModeSchema || file.Script == nil {
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

func buildProcessContext(imports []ast.ImportDeclaration, script *ast.ScriptBlock, importBaseDir string, importRootDir string, enforceImportRoot bool, input map[string]Value) (processContext, error) {
	return buildProcessContextWithState(
		imports,
		script,
		importBaseDir,
		importRootDir,
		enforceImportRoot,
		input,
		map[string]map[string]importedDeclaration{},
		map[string]struct{}{},
	)
}

func buildProcessContextWithState(imports []ast.ImportDeclaration, script *ast.ScriptBlock, importBaseDir string, importRootDir string, enforceImportRoot bool, input map[string]Value, cache map[string]map[string]importedDeclaration, stack map[string]struct{}) (processContext, error) {
	context := newProcessContext(importBaseDir, importRootDir)

	imported, err := resolveImportsWithState(ast.File{Imports: imports}, importBaseDir, importRootDir, enforceImportRoot, cache, stack)
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
		default:
			return processContext{}, validationErrorf("unknown import %q", importedDecl.name)
		}
	}

	if script != nil {
		if err := collectDeclarations(script.Items, context.symbols, context.types, context.schemas); err != nil {
			return processContext{}, err
		}
		if err := validateDeclarations(script.Items, context.symbols, context.types, context.schemas, context.variables); err != nil {
			return processContext{}, err
		}
		if err := evaluateScript(script.Items, context.environment, context.symbols, context.types, context.schemas, nil); err != nil {
			return processContext{}, err
		}
	}

	return context, nil
}

func prepareOutputContext(output ast.OutputBlock, context processContext) (processContext, error) {
	outputContext := context.clone()
	if outputContext.symbols == nil {
		outputContext = newProcessContext(context.importBaseDir, context.importRootDir)
	}

	if err := validateOutputDirectiveStructure(output); err != nil {
		return processContext{}, err
	}

	schemaFileDeclarations, err := resolveSchemaFileDeclarations(output.Directives, outputContext.importBaseDir, outputContext.importRootDir)
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
		default:
			return processContext{}, validationErrorf("unknown declaration %q in schema_file", declaration.name)
		}
	}

	if err := validateOutputDirectiveReferences(output, outputContext.symbols); err != nil {
		return processContext{}, err
	}

	return outputContext, nil
}

func (p *Processor) applyParsedOutputInput(output ast.OutputBlock, context *processContext) error {
	schemaName, ok, err := outputParseSchemaName(output.Directives, *context)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	inputValue := Value{Kind: ValueRecord, Record: cloneValueMap(p.input)}

	schema, ok := context.schemas.Get(schemaName)
	if !ok {
		return validationErrorf("unknown schema %q", schemaName)
	}

	self := &schema
	for _, field := range schema.Fields {
		fieldType, err := resolveValueTypeInRecord(field.Type, self, context.symbols, context.types, context.schemas, nil)
		if err != nil {
			return err
		}

		name := "$" + field.Name
		context.variables.Add(name, fieldType)
		if fieldValue, found := inputValue.Record[field.Name]; found {
			context.environment.Add(name, fieldValue)
		}
	}

	expectedType := valueType{kind: ValueRecord, schemaName: schemaName, record: &schema}
	return validateEvaluatedValueAgainstType(inputValue, expectedType, context.symbols, context.types, context.schemas, nil)
}

func isRuntimeInputValidationError(err error) bool {
	var diagnosticError DiagnosticError
	return errors.As(err, &diagnosticError) && diagnosticError.Code == CodeTypeRecordDoesNotMatchSchema
}

func hasParsedInputDirective(directives []ast.OutputDirective) bool {
	_, hasParse := outputParseSchemeName(directives)
	return hasParse || hasParseFile(directives)
}

func validateOutputExpressionReferences(fields []ast.OutputField, variables *variableRegistry) error {
	for _, field := range fields {
		if err := validateExpressionReferences(field.Value, variables); err != nil {
			return diagnosticErrorAtNode(err, field)
		}
	}
	return nil
}

func validateExpressionReferences(expression ast.Expression, variables *variableRegistry) error {
	switch typed := expression.(type) {
	case ast.GroupedExpression:
		return validateExpressionReferences(typed.Expression, variables)
	case ast.Identifier:
		if _, ok := variables.Get(typed.Name); !ok {
			return diagnosticErrorAtNode(
				diagnosticErrorf(
					ErrorValue,
					CodeOutputValueDeclaration,
					DiagnosticFields{Name: typed.Name},
					"unknown identifier %q",
					typed.Name,
				),
				typed,
			)
		}
	case ast.MemberAccess:
		return validateExpressionReferences(typed.Target, variables)
	case ast.MatchExpression:
		if err := validateExpressionReferences(typed.Value, variables); err != nil {
			return err
		}
		for _, arm := range typed.Arms {
			if err := validateExpressionReferences(arm.Value, variables); err != nil {
				return err
			}
		}
	case ast.ArrayLiteral:
		for _, element := range typed.Elements {
			if err := validateExpressionReferences(element, variables); err != nil {
				return err
			}
		}
	case ast.RecordLiteral:
		for _, field := range typed.Fields {
			if err := validateExpressionReferences(field.Value, variables); err != nil {
				return err
			}
		}
	case ast.PrefixExpression:
		return validateExpressionReferences(typed.Right, variables)
	case ast.InfixExpression:
		if err := validateExpressionReferences(typed.Left, variables); err != nil {
			return err
		}
		return validateExpressionReferences(typed.Right, variables)
	case ast.ConditionalExpression:
		if err := validateExpressionReferences(typed.Condition, variables); err != nil {
			return err
		}
		if err := validateExpressionReferences(typed.Then, variables); err != nil {
			return err
		}
		return validateExpressionReferences(typed.Else, variables)
	}
	return nil
}

type importedDeclaration struct {
	name    string
	kind    symbolKind
	typeRef ast.TypeReference
	record  ast.RecordType
	value   Value
	vtype   valueType
}

func resolveImportsWithState(file ast.File, importBaseDir string, importRootDir string, enforceImportRoot bool, cache map[string]map[string]importedDeclaration, stack map[string]struct{}) ([]importedDeclaration, error) {
	if len(file.Imports) == 0 {
		return nil, nil
	}

	imports := map[string]importedDeclaration{}

	for _, importDecl := range file.Imports {
		path, err := parseImportPath(importDecl.Path)
		if err != nil {
			return nil, err
		}
		if err := validateMaceSourcePath(path); err != nil {
			return nil, err
		}

		resolvedPath, err := resolveImportPathInScope(importBaseDir, importRootDir, path, enforceImportRoot)
		if err != nil {
			return nil, err
		}
		declarations, err := loadImportExports(resolvedPath, importRootDir, enforceImportRoot, cache, stack)
		if err != nil {
			return nil, err
		}
		if importDecl.Binding != nil {
			localName := importDecl.Binding.LocalName()
			if _, exists := imports[localName]; exists {
				return nil, validationErrorf("duplicate import %q", localName)
			}
			decl, err := importFileAsDeclaration(localName, declarations)
			if err != nil {
				return nil, err
			}
			imports[localName] = decl
			continue
		}

		for _, imported := range importDecl.Identifiers {
			localName := imported.LocalName()
			if _, exists := imports[localName]; exists {
				return nil, validationErrorf("duplicate import %q", localName)
			}

			decl, ok := declarations[imported.Name]
			if !ok {
				return nil, validationErrorf("imported identifier %q not found in %q", imported.Name, path)
			}

			aliasedDecl := decl
			aliasedDecl.name = localName
			imports[localName] = aliasedDecl
		}
	}

	imported := make([]importedDeclaration, 0, len(imports))
	for _, item := range imports {
		imported = append(imported, item)
	}
	return imported, nil
}

func importFileAsDeclaration(name string, declarations map[string]importedDeclaration) (importedDeclaration, error) {
	fields := make([]ast.SchemaField, 0, len(declarations))
	recordValues := map[string]Value{}
	allVariables := true
	for exportedName, declaration := range declarations {
		switch declaration.kind {
		case symbolKindSchema:
			fields = append(fields, ast.SchemaField{Name: exportedName, Type: declaration.record})
			allVariables = false
		case symbolKindType:
			fields = append(fields, ast.SchemaField{Name: exportedName, Type: declaration.typeRef})
			allVariables = false
		case symbolKindVariable:
			recordValues[exportedName] = declaration.value
			fields = append(fields, ast.SchemaField{Name: exportedName, Type: typeReferenceFromValueType(declaration.vtype)})
		default:
			return importedDeclaration{}, validationErrorf("unknown import %q", exportedName)
		}
	}
	if allVariables {
		return importedDeclaration{name: name, kind: symbolKindVariable, value: Value{Kind: ValueRecord, Record: recordValues}, vtype: valueType{kind: ValueRecord, record: &ast.RecordType{Fields: fields}}}, nil
	}
	return importedDeclaration{name: name, kind: symbolKindSchema, record: ast.RecordType{Fields: fields}}, nil
}

func typeReferenceFromValueType(input valueType) ast.TypeReference {
	if len(input.choiceValues) > 0 {
		members := make([]ast.Expression, 0, len(input.choiceValues))
		for _, value := range input.choiceValues {
			members = append(members, expressionFromValue(value))
		}
		return ast.ChoiceType{Members: members}
	}
	if len(input.members) > 0 {
		members := make([]ast.TypeReference, 0, len(input.members))
		for _, member := range input.members {
			members = append(members, typeReferenceFromValueType(member))
		}
		return ast.VariantType{Members: members}
	}
	switch input.kind {
	case ValueString:
		return ast.PrimitiveType{Name: "string"}
	case ValueInt:
		return ast.PrimitiveType{Name: "int"}
	case ValueFloat:
		return ast.PrimitiveType{Name: "float"}
	case ValueHexInt:
		return ast.PrimitiveType{Name: "hex_int"}
	case ValueHexFloat:
		return ast.PrimitiveType{Name: "hex_float"}
	case ValueBoolean:
		return ast.PrimitiveType{Name: "boolean"}
	case ValueArray:
		if input.element != nil {
			return ast.ArrayType{Element: typeReferenceFromValueType(*input.element)}
		}
		return ast.ArrayType{Element: ast.PrimitiveType{Name: "string"}}
	case ValueRecord:
		if input.schemaName != "" {
			return ast.NamedType{Name: input.schemaName}
		}
		if input.element != nil {
			return ast.RecordMapType{Value: typeReferenceFromValueType(*input.element)}
		}
		if input.record != nil {
			return *input.record
		}
		return ast.RecordType{}
	default:
		return ast.PrimitiveType{Name: "string"}
	}
}

func expressionFromValue(value Value) ast.Expression {
	switch value.Kind {
	case ValueString:
		return ast.StringLiteral{Lexeme: strconv.Quote(value.String)}
	case ValueInt:
		return ast.IntLiteral{Lexeme: strconv.FormatInt(value.Int, 10)}
	case ValueFloat:
		return ast.FloatLiteral{Lexeme: strconv.FormatFloat(value.Float, 'f', -1, 64)}
	case ValueHexInt:
		return ast.HexIntLiteral{Lexeme: value.String}
	case ValueHexFloat:
		return ast.HexFloatLiteral{Lexeme: value.String}
	case ValueBoolean:
		return ast.BooleanLiteral{Value: value.Boolean}
	default:
		return ast.StringLiteral{Lexeme: strconv.Quote(scalarValueDisplay(value))}
	}
}

func parseImportPath(literal ast.StringLiteral) (string, error) {
	value, err := parseStaticString(literal.Lexeme)
	if err != nil {
		return "", err
	}
	return value.String, nil
}

func resolveImportPath(importBaseDir string, importPath string) (string, error) {
	if remoteURL, ok := parseRemoteURL(importPath); ok {
		return remoteURL.String(), nil
	}
	if baseURL, ok := parseRemoteURL(importBaseDir); ok {
		resolvedURL, err := baseURL.Parse(importPath)
		if err != nil {
			return "", validationErrorf("unable to resolve path %q", importPath)
		}
		return resolvedURL.String(), nil
	}
	if filepath.IsAbs(importPath) {
		return "", validationErrorf("import path %q must be relative: base=%q", importPath, importBaseDir)
	}

	cleanPath := filepath.Clean(filepath.FromSlash(importPath))
	return filepath.Clean(filepath.Join(importBaseDir, cleanPath)), nil
}

func resolveImportPathInScope(importBaseDir string, importRootDir string, importPath string, enforceImportRoot bool) (string, error) {
	if !enforceImportRoot {
		return resolveImportPath(importBaseDir, importPath)
	}

	return resolveBoundedPath(importBaseDir, importRootDir, importPath)
}

func resolveBoundedPath(importBaseDir string, importRootDir string, importPath string) (string, error) {
	resolvedPath, err := resolveImportPath(importBaseDir, importPath)
	if err != nil {
		return "", validationErrorf("import path %q must be relative: root=%q, base=%q", importPath, formatImportRoot(importRootDir), importBaseDir)
	}
	if _, ok := parseRemoteURL(resolvedPath); ok {
		return resolveBoundedRemotePath(importBaseDir, importRootDir, importPath, resolvedPath)
	}

	cleanPath := filepath.Clean(filepath.FromSlash(importPath))
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", validationErrorf("import path %q escapes root: root=%q, base=%q, resolved=%q", importPath, formatImportRoot(importRootDir), importBaseDir, resolvedPath)
	}

	absoluteRoot, err := filepath.Abs(importRootDir)
	if err != nil {
		return "", validationErrorf("unable to resolve path %q", importPath)
	}
	absolutePath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return "", validationErrorf("unable to resolve path %q", importPath)
	}

	relativePath, err := filepath.Rel(absoluteRoot, absolutePath)
	if err != nil {
		return "", validationErrorf("unable to resolve path %q", importPath)
	}
	if relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", validationErrorf("import path %q escapes root: root=%q, base=%q, resolved=%q", importPath, formatImportRoot(importRootDir), importBaseDir, resolvedPath)
	}

	return absolutePath, nil
}

func resolveBoundedRemotePath(importBaseDir string, importRootDir string, importPath string, resolvedPath string) (string, error) {
	rootURL, ok := parseRemoteURL(importRootDir)
	if !ok {
		return resolvedPath, nil
	}
	resolvedURL, ok := parseRemoteURL(resolvedPath)
	if !ok {
		return "", validationErrorf("import path %q escapes root: root=%q, base=%q, resolved=%q", importPath, formatImportRoot(importRootDir), importBaseDir, resolvedPath)
	}
	if resolvedURL.Scheme != rootURL.Scheme || resolvedURL.Host != rootURL.Host {
		return "", validationErrorf("import path %q escapes root: root=%q, base=%q, resolved=%q", importPath, formatImportRoot(importRootDir), importBaseDir, resolvedPath)
	}

	rootPath := rootURL.EscapedPath()
	resolvedPathValue := resolvedURL.EscapedPath()
	if rootPath == "" {
		rootPath = "/"
	}
	if !strings.HasSuffix(rootPath, "/") {
		rootPath = path.Dir(rootPath) + "/"
	}
	if resolvedPathValue != strings.TrimSuffix(rootPath, "/") && !strings.HasPrefix(resolvedPathValue, rootPath) {
		return "", validationErrorf("import path %q escapes root: root=%q, base=%q, resolved=%q", importPath, formatImportRoot(importRootDir), importBaseDir, resolvedPath)
	}

	return resolvedPath, nil
}

func formatImportRoot(importRootDir string) string {
	if importRootDir == "" || importRootDir == "." {
		return "./"
	}
	if _, ok := parseRemoteURL(importRootDir); ok {
		return importRootDir
	}

	cleanRoot := filepath.Clean(importRootDir)
	label := filepath.Base(cleanRoot)
	if label == "." || label == string(filepath.Separator) || label == "" {
		return filepath.ToSlash(cleanRoot)
	}
	return label + "/"
}

func parseRemoteURL(raw string) (*url.URL, bool) {
	parsedURL, err := url.Parse(raw)
	if err != nil || parsedURL == nil {
		return nil, false
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, false
	}
	if parsedURL.Host == "" {
		return nil, false
	}
	return parsedURL, true
}

func basePathDir(sourcePath string) string {
	if parsedURL, ok := parseRemoteURL(sourcePath); ok {
		parsedURL.Path = path.Dir(parsedURL.Path)
		if !strings.HasSuffix(parsedURL.Path, "/") {
			parsedURL.Path += "/"
		}
		parsedURL.RawPath = ""
		parsedURL.RawQuery = ""
		parsedURL.Fragment = ""
		return parsedURL.String()
	}
	return filepath.Dir(sourcePath)
}

func validateMaceSourcePath(sourcePath string) error {
	if !strings.HasSuffix(sourcePath, ".mace") {
		return validationErrorf("import path %q must end in .mace", sourcePath)
	}
	return nil
}

func readMaceSource(sourcePath string) (string, error) {
	if _, ok := parseRemoteURL(sourcePath); !ok {
		contents, err := os.ReadFile(sourcePath)
		if err != nil {
			return "", err
		}
		return string(contents), nil
	}

	response, err := http.Get(sourcePath)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = response.Body.Close()
	}()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("unexpected status %d", response.StatusCode)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func loadImportExports(path string, importRootDir string, enforceImportRoot bool, cache map[string]map[string]importedDeclaration, stack map[string]struct{}) (map[string]importedDeclaration, error) {
	if declarations, ok := cache[path]; ok {
		return declarations, nil
	}
	if _, ok := stack[path]; ok {
		return nil, validationErrorf("circular import detected at %q", path)
	}

	stack[path] = struct{}{}
	defer delete(stack, path)

	contents, err := readMaceSource(path)
	if err != nil {
		return nil, validationErrorf("unable to read import file %q", path)
	}

	tokens, err := lex(contents)
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
		basePathDir(path),
		importRootDir,
		enforceImportRoot,
		map[string]Value{},
		cache,
		stack,
	)
	if err != nil {
		return nil, err
	}

	if err := validateSchemaOutputScriptVariables(file); err != nil {
		return nil, err
	}

	declarations, err := collectImportExports(file.Output, context)
	if err != nil {
		return nil, err
	}
	cache[path] = declarations
	return declarations, nil
}

func resolveSchemaFileDeclarations(directives []ast.OutputDirective, importBaseDir string, importRootDir string) ([]importedDeclaration, error) {
	var path string
	for _, directive := range directives {
		if directive.Kind != ast.OutputDirectiveSchemaFile && directive.Kind != ast.OutputDirectiveParseFile {
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
	if err := validateMaceSourcePath(path); err != nil {
		return nil, err
	}

	resolvedPath, err := resolveBoundedPath(importBaseDir, importRootDir, path)
	if err != nil {
		return nil, err
	}
	declarations, err := loadSchemaFileDeclarations(resolvedPath, importRootDir, map[string]map[string]ast.Declaration{}, map[string]struct{}{})
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
		}
	}

	if hasSchemaFile(directives) {
		record, err := loadOutputSchemaRecord(resolvedPath, importRootDir, "schema_file")
		if err != nil {
			return nil, err
		}
		outputDeclarations = append(outputDeclarations, importedDeclaration{
			name:   "__schema_file",
			kind:   symbolKindSchema,
			record: record,
		})
	}

	if hasParseFile(directives) {
		record, err := loadOutputSchemaRecord(resolvedPath, importRootDir, "parse_file")
		if err != nil {
			return nil, err
		}
		outputDeclarations = append(outputDeclarations, importedDeclaration{
			name:   "__parse_file",
			kind:   symbolKindSchema,
			record: record,
		})
	}

	return outputDeclarations, nil
}

func loadOutputSchemaRecord(path string, importRootDir string, directiveName string) (ast.RecordType, error) {
	contents, err := readMaceSource(path)
	if err != nil {
		return ast.RecordType{}, validationErrorf("unable to read import file %q", path)
	}
	tokens, err := lex(contents)
	if err != nil {
		return ast.RecordType{}, err
	}
	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return ast.RecordType{}, validationErrorf("unable to parse import file %q: %s", path, err)
	}
	if file.Output.Mode != ast.OutputModeSchema {
		return ast.RecordType{}, validationErrorf("%s target %q must output a schema", directiveName, path)
	}
	context, err := buildProcessContext(file.Imports, file.Script, basePathDir(path), importRootDir, true, nil)
	if err != nil {
		return ast.RecordType{}, err
	}
	if err := validateSchemaOutputFields(file.Output.SchemaFields, context.symbols, context.types, context.schemas, nil); err != nil {
		return ast.RecordType{}, err
	}

	fields := make([]ast.SchemaField, 0, len(file.Output.SchemaFields))
	for _, field := range file.Output.SchemaFields {
		fields = append(fields, ast.SchemaField{Name: field.Name, Optional: field.Optional, Type: field.Type, Description: field.Description})
	}
	return resolveExportedRecordType(ast.RecordType{Fields: fields}, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
}

func loadSchemaFileDeclarations(path string, importRootDir string, cache map[string]map[string]ast.Declaration, stack map[string]struct{}) (map[string]ast.Declaration, error) {
	if declarations, ok := cache[path]; ok {
		return declarations, nil
	}
	if _, ok := stack[path]; ok {
		return nil, validationErrorf("circular import detected at %q", path)
	}

	stack[path] = struct{}{}
	defer delete(stack, path)

	contents, err := readMaceSource(path)
	if err != nil {
		return nil, validationErrorf("unable to read import file %q", path)
	}

	tokens, err := lex(contents)
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

		resolvedPath, err := resolveBoundedPath(basePathDir(path), importRootDir, importPath)
		if err != nil {
			return nil, err
		}
		if _, err := loadSchemaFileDeclarations(resolvedPath, importRootDir, cache, stack); err != nil {
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
		if err := validateSchemaOutputFields(output.SchemaFields, outputContext.symbols, outputContext.types, outputContext.schemas, nil); err != nil {
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

	schemaName, hasSchema, err := activeOutputSchemaName(output.Directives, outputContext)
	if err != nil {
		return nil, err
	}
	if err := validateDataOutputFields(output.DataFields, hasSchema, outputContext.variables, outputContext.symbols, outputContext.types, outputContext.schemas, nil); err != nil {
		return nil, err
	}
	if hasSchema {
		if err := validateOutputSchema(schemaName, output.DataFields, outputContext.variables, outputContext.symbols, outputContext.types, outputContext.schemas, nil); err != nil {
			return nil, err
		}
	}

	values, err := evaluateOutputFields(output.DataFields, outputContext.environment, outputContext.symbols, outputContext.types, outputContext.schemas, nil)
	if err != nil {
		return nil, err
	}

	exports := make(map[string]importedDeclaration, len(output.DataFields))
	for _, field := range output.DataFields {
		exportedType, err := exportedOutputFieldType(field, output, outputContext)
		if err != nil {
			return nil, err
		}
		exportedType.nullable = field.Optional

		value, exists := values[field.Name]
		if !exists {
			value = Value{Kind: ValueNull}
		}
		value.Type = &exportedType

		exports[field.Name] = importedDeclaration{
			name:  field.Name,
			kind:  symbolKindVariable,
			value: value,
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
	case ast.RecordMapType:
		exportedValue, err := resolveExportedTypeReference(typedRef.Value, context.types, context.schemas, map[string]struct{}{}, map[string]struct{}{})
		if err != nil {
			return importedDeclaration{}, err
		}
		return importedDeclaration{
			name:    field.Name,
			kind:    symbolKindType,
			typeRef: ast.RecordMapType{Value: exportedValue},
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
	schemaName, ok, err := activeOutputSchemaName(output.Directives, context)
	if err != nil {
		return valueType{}, err
	}
	if ok {
		schema, exists := context.schemas.Get(schemaName)
		if !exists {
			return valueType{}, validationErrorf("unknown schema %q", schemaName)
		}

		for _, schemaField := range schema.Fields {
			if schemaField.Name != field.Name {
				continue
			}

			resolvedType, err := resolveValueType(schemaField.Type, context.symbols, context.types, context.schemas, nil)
			if err != nil {
				return valueType{}, err
			}

			return sanitizeImportedValueType(resolvedType, context.schemas), nil
		}
	}

	inferredType, err := inferExpressionType(field.Value, context.variables, context.symbols, context.types, context.schemas, nil)
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
	case ast.RecordMapType:
		value, err := resolveExportedTypeReference(ref.Value, types, schemas, aliasStack, schemaStack)
		if err != nil {
			return nil, err
		}
		return ast.RecordMapType{Value: value}, nil
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
	case ast.ChoiceType:
		if _, err := resolveChoiceType(ref, types); err != nil {
			return nil, err
		}
		return ref, nil
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

func cloneNameSet(values map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(values))
	for name := range values {
		cloned[name] = struct{}{}
	}

	return cloned
}

func collectDeclarations(items []ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry) error {
	for _, declaration := range items {
		switch decl := declaration.(type) {
		case ast.VariableDeclaration:
			if symbols.Has(decl.Name) {
				return diagnosticErrorAtNode(validationErrorf("duplicate declaration %q", decl.Name), decl)
			}
			symbols.Add(decl.Name, symbolKindVariable)
		case ast.TypeDeclaration:
			if symbols.Has(decl.Name) {
				return diagnosticErrorAtNode(validationErrorf("duplicate declaration %q", decl.Name), decl)
			}
			symbols.Add(decl.Name, symbolKindType)
			types.AddAlias(decl.Name, decl.Type)
		case ast.SchemaDeclaration:
			if symbols.Has(decl.Name) {
				return diagnosticErrorAtNode(validationErrorf("duplicate declaration %q", decl.Name), decl)
			}
			symbols.Add(decl.Name, symbolKindSchema)
			schemas.Add(decl.Name, decl.Type)
		case ast.DocDeclaration:
			continue
		default:
			return validationErrorf("unknown declaration")
		}
	}

	return nil
}

func validateDeclarations(items []ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, variables *variableRegistry) error {
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
		if err := validateDeclaration(declaration, symbols, types, schemas, nil, variables, seenDocs, docsByTarget, declaredKinds); err != nil {
			return diagnosticErrorAtNode(err, declaration)
		}

		switch decl := declaration.(type) {
		case ast.TypeDeclaration:
			declaredKinds[decl.Name] = symbolKindType
		case ast.SchemaDeclaration:
			declaredKinds[decl.Name] = symbolKindSchema
		case ast.VariableDeclaration:
			declaredKinds[decl.Name] = symbolKindVariable
		}
	}

	return nil
}

func validateDeclaration(declaration ast.Declaration, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, variables *variableRegistry, seenDocs map[string]struct{}, docsByTarget map[string]ast.DocDeclaration, declaredKinds map[string]symbolKind) error {
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
			if nullExpression, found := nullExpressionIn(decl.Value); found {
				return diagnosticErrorAtNode(invalidNullUsageError(), nullExpression)
			}
			actualType, err := inferExpressionType(decl.Value, variables, symbols, types, schemas, enums)
			if err != nil {
				return err
			}
			if conditionalRequiresCollectionContext(decl.Value, actualType) && len(expectedType.members) == 0 {
				return validationErrorf("ambiguous conditional with an empty collection requires a variant variable type")
			}
			if !expressionContainsEmptyCollection(decl.Value) {
				if err := ensureAssignable(expectedType, actualType); err != nil {
					return err
				}
			}
			if err := validateExpressionAgainstType(decl.Value, expectedType, variables, symbols, types, schemas, enums); err != nil {
				return err
			}
		} else {
			return validationErrorf("variable %q requires an initializer", decl.Name)
		}
		if decl.Description != "" {
			if _, ok := docsByTarget[decl.Name]; ok {
				return validationErrorf("variable %q is already documented by a documentation declaration", decl.Name)
			}
		}
		variables.Add(decl.Name, expectedType)
		return nil
	case ast.TypeDeclaration:
		if err := validateTypeReference(decl.Type, symbols, types, schemas, enums); err != nil {
			return err
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
					return validationErrorf("schema field %q in %q is already documented by schema_doc fields", field.Name, decl.Name)
				}
			}
		}
		return nil
	case ast.DocDeclaration:
		return validateDocDeclaration(decl, symbols, schemas, variables, seenDocs, declaredKinds)
	default:
		return validationErrorf("unknown declaration")
	}
}

func validateTypeReference(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	return validateTypeReferenceWithContext(typeRef, symbols, types, schemas, enums, nil, false)
}

func validateTypeReferenceWithContext(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, self *ast.RecordType, guarded bool) error {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return nil
	case ast.SelfType:
		if self == nil || !guarded {
			return validationErrorf("expected type reference")
		}
		return nil
	case ast.ArrayType:
		return validateTypeReferenceWithContext(ref.Element, symbols, types, schemas, enums, self, true)
	case ast.RecordMapType:
		return validateTypeReferenceWithContext(ref.Value, symbols, types, schemas, enums, self, true)
	case ast.UnionType:
		for _, member := range ref.Members {
			if err := validateTypeReferenceWithContext(member, symbols, types, schemas, enums, self, guarded); err != nil {
				return err
			}
		}
		_, err := resolveFusionValueTypeWithContext(ref, symbols, types, schemas, enums, self, map[string]struct{}{})
		return err
	case ast.VariantType:
		for _, member := range ref.Members {
			if err := validateTypeReferenceWithContext(member, symbols, types, schemas, enums, self, guarded); err != nil {
				return err
			}
		}
		resolved, err := resolveValueTypeWithContext(ref, symbols, types, schemas, enums, self, map[string]struct{}{})
		if err != nil {
			return err
		}
		return validateVariantValueTypes(resolved.members)
	case ast.RecordType:
		return validateRecordType(ref, symbols, types, schemas, enums)
	case ast.ChoiceType:
		_, err := resolveChoiceType(ref, types)
		return err
	case ast.NamedType:
		if symbols.IsType(ref.Name) {
			_, _, err := types.Resolve(ref.Name)
			return diagnosticErrorAtNode(err, ref)
		}
		if !symbols.IsSchema(ref.Name) && !symbols.IsImport(ref.Name) {
			return diagnosticErrorAtNode(validationErrorf("unknown type %q", ref.Name), ref)
		}
		return nil
	default:
		return validationErrorf("unknown type reference")
	}
}

func validateRecordType(record ast.RecordType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	fieldNames := map[string]struct{}{}
	self := &record
	for _, field := range record.Fields {
		if _, exists := fieldNames[field.Name]; exists {
			return diagnosticErrorAtNode(validationErrorf("duplicate field %q", field.Name), field)
		}
		fieldNames[field.Name] = struct{}{}

		if err := validateTypeReferenceWithContext(field.Type, symbols, types, schemas, enums, self, false); err != nil {
			return diagnosticErrorAtNode(err, field)
		}
	}

	return nil
}

func validateDocDeclaration(declaration ast.DocDeclaration, symbols *symbolTable, schemas *schemaRegistry, variables *variableRegistry, seenDocs map[string]struct{}, declaredKinds map[string]symbolKind) error {
	targetKind, ok := symbols.Get(declaration.Target)
	if !ok {
		return validationErrorf("documentation target %q must reference an existing declaration", declaration.Target)
	}
	if targetKind == symbolKindImport {
		keyword := "gen_doc"
		if declaration.Kind == ast.DocumentationKindSchema {
			keyword = "schema_doc"
		}
		return validationErrorf("%s target %q is imported; use %s in the file that exports it", keyword, declaration.Target, keyword)
	}
	if _, exists := seenDocs[declaration.Target]; exists {
		return validationErrorf("duplicate documentation declaration for %q", declaration.Target)
	}

	keyword := "gen_doc"
	declaredKind, declared := declaredKinds[declaration.Target]
	variableType, hasVariableType := variables.Get(declaration.Target)
	isObjectVariable := hasVariableType && variableType.kind == ValueRecord
	if declaration.Kind == ast.DocumentationKindSchema {
		keyword = "schema_doc"
		if !declared || (declaredKind != symbolKindSchema && declaredKind != symbolKindVariable) {
			return validationErrorf("schema_doc target %q must appear after its schema or object-valued variable declaration", declaration.Target)
		}
		if targetKind != symbolKindSchema && targetKind != symbolKindVariable {
			return validationErrorf("schema_doc target %q must reference a schema or object-valued variable", declaration.Target)
		}
		if targetKind == symbolKindVariable && !isObjectVariable {
			return validationErrorf("schema_doc target %q must reference a schema or object-valued variable", declaration.Target)
		}
	} else {
		if !declared || (declaredKind != symbolKindType && declaredKind != symbolKindVariable) {
			return validationErrorf("gen_doc target %q must appear after its type or non-object variable declaration", declaration.Target)
		}
		if targetKind != symbolKindType && targetKind != symbolKindVariable {
			return validationErrorf("gen_doc target %q must reference a type or non-object variable", declaration.Target)
		}
		if targetKind == symbolKindVariable && isObjectVariable {
			return validationErrorf("gen_doc target %q must reference a type or non-object variable", declaration.Target)
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
			return validationErrorf("%s fields for %q require a schema-style target", keyword, declaration.Target)
		}

		fieldNames := map[string]struct{}{}
		switch targetKind {
		case symbolKindSchema:
			record, ok := schemas.Get(declaration.Target)
			if !ok {
				return validationErrorf("unknown schema %q for %s fields", declaration.Target, keyword)
			}
			for _, field := range record.Fields {
				fieldNames[field.Name] = struct{}{}
			}
		case symbolKindVariable:
			if !isObjectVariable {
				return validationErrorf("%s fields for %q require a schema-style target", keyword, declaration.Target)
			}
			if variableType.record == nil {
				return validationErrorf("unknown object shape for %q %s fields", declaration.Target, keyword)
			}
			for _, field := range variableType.record.Fields {
				fieldNames[field.Name] = struct{}{}
			}
		default:
			return validationErrorf("%s fields for %q require a schema-style target", keyword, declaration.Target)
		}

		for name, value := range declaration.Documentation.Props {
			if _, exists := fieldNames[name]; !exists {
				return validationErrorf("%s fields entry %q does not exist on %q", keyword, name, declaration.Target)
			}
			if _, err := parseStaticString(value.Lexeme); err != nil {
				return err
			}
		}
	}

	return nil
}

func validateOutputDirectiveStructure(output ast.OutputBlock) error {
	if len(output.Directives) == 0 {
		return nil
	}

	hasOutput := false
	hasParse := false
	hasParseFile := false
	seenKinds := map[ast.OutputDirectiveKind]struct{}{}

	for _, directive := range output.Directives {
		if _, exists := seenKinds[directive.Kind]; exists {
			return validationErrorf("duplicate output directive %q", directiveKindName(directive.Kind))
		}
		seenKinds[directive.Kind] = struct{}{}

		switch directive.Kind {
		case ast.OutputDirectiveOutput:
			hasOutput = true
		case ast.OutputDirectiveSchema:
			if output.Mode == ast.OutputModeSchema {
				return validationErrorf("schema directive is invalid when output mode is schema")
			}
		case ast.OutputDirectiveSchemaFile:
			if output.Mode == ast.OutputModeSchema {
				return validationErrorf("schema_file directive is invalid when output mode is schema")
			}
		case ast.OutputDirectiveParse:
			hasParse = true
			if output.Mode == ast.OutputModeSchema {
				return validationErrorf("parse directive is invalid when output mode is schema")
			}
		case ast.OutputDirectiveParseFile:
			hasParseFile = true
			if output.Mode == ast.OutputModeSchema {
				return validationErrorf("parse_file directive is invalid when output mode is schema")
			}
		case ast.OutputDirectiveDescription:
			if directive.Description == nil {
				return validationErrorf("description directive requires a value")
			}
		default:
			return validationErrorf("unknown output directive")
		}
	}

	if hasParse && hasParseFile {
		return validationErrorf("parse and parse_file directives cannot be used together")
	}
	if !hasOutput {
		return validationErrorf("directive list must include an output directive")
	}

	return nil
}

func validateOutputDocumentation(directives []ast.OutputDirective, context processContext) error {
	for _, directive := range directives {
		if directive.Kind != ast.OutputDirectiveDescription {
			continue
		}

		value, err := evaluateExpression(directive.Description, context.environment, Value{}, context.symbols, context.types, context.schemas, nil)
		if err != nil {
			return err
		}
		if value.Kind != ValueString {
			return validationErrorf("description directive must evaluate to a string")
		}
	}

	return nil
}

func validateOutputDirectiveReferences(output ast.OutputBlock, symbols *symbolTable) error {
	for _, directive := range output.Directives {
		if directive.Kind != ast.OutputDirectiveSchema && directive.Kind != ast.OutputDirectiveParse {
			continue
		}

		if !symbols.IsSchema(directive.Value) && !symbols.IsImport(directive.Value) {
			return validationErrorf("unknown schema %q", directive.Value)
		}
	}

	return nil
}

func validateSchemaOutputFields(fields []ast.OutputSchemaField, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	fieldNames := map[string]struct{}{}
	for _, field := range fields {
		if _, exists := fieldNames[field.Name]; exists {
			return diagnosticErrorAtNode(validationErrorf("duplicate output field %q", field.Name), field)
		}
		fieldNames[field.Name] = struct{}{}

		if err := validateSchemaOutputFieldType(field.Type, symbols); err != nil {
			return diagnosticErrorAtNode(err, field)
		}

		if err := validateTypeReference(field.Type, symbols, types, schemas, enums); err != nil {
			return diagnosticErrorAtNode(err, field)
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
			return diagnosticErrorf(ErrorType, CodeInvalidOutputSchemaField, DiagnosticFields{Name: ref.Name}, "invalid field type %q in output = schema", ref.Name)
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

func activeOutputSchemaName(directives []ast.OutputDirective, context processContext) (string, bool, error) {
	if schemaName, ok := outputSchemaName(directives); ok {
		return schemaName, true, nil
	}
	if hasSchemaFile(directives) {
		return "__schema_file", true, nil
	}

	return "", false, nil
}

func outputParseSchemeName(directives []ast.OutputDirective) (string, bool) {
	for _, directive := range directives {
		if directive.Kind == ast.OutputDirectiveParse {
			return directive.Value, true
		}
	}
	return "", false
}

func outputParseSchemaName(directives []ast.OutputDirective, context processContext) (string, bool, error) {
	if parseName, ok := outputParseSchemeName(directives); ok {
		return parseName, true, nil
	}
	if !hasParseFile(directives) {
		return "", false, nil
	}
	if schemaName, ok := outputSchemaName(directives); ok {
		return schemaName, true, nil
	}

	schemaNames, err := resolveParseFileExportedSchemaNames(directives, context.importBaseDir, context.importRootDir)
	if err != nil {
		return "", false, err
	}
	if len(schemaNames) == 1 {
		return schemaNames[0], true, nil
	}
	if len(schemaNames) == 0 {
		return "__parse_file", true, nil
	}

	return "", false, validationErrorf("parse_file directive is ambiguous without a schema directive")
}

func resolveParseFileExportedSchemaNames(directives []ast.OutputDirective, importBaseDir string, importRootDir string) ([]string, error) {
	return resolveOutputSchemaNames(directives, ast.OutputDirectiveParseFile, importBaseDir, importRootDir)
}

func resolveOutputSchemaNames(directives []ast.OutputDirective, kind ast.OutputDirectiveKind, importBaseDir string, importRootDir string) ([]string, error) {
	var path string
	for _, directive := range directives {
		if directive.Kind != kind {
			continue
		}

		parsedPath, err := parseStaticString(directive.Value)
		if err != nil {
			return nil, err
		}

		path = parsedPath.String
		break
	}
	if path == "" {
		return nil, nil
	}
	if err := validateMaceSourcePath(path); err != nil {
		return nil, err
	}

	resolvedPath, err := resolveBoundedPath(importBaseDir, importRootDir, path)
	if err != nil {
		return nil, err
	}

	contents, err := readMaceSource(resolvedPath)
	if err != nil {
		return nil, validationErrorf("unable to read import file %q", resolvedPath)
	}
	tokens, err := lex(contents)
	if err != nil {
		return nil, err
	}
	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return nil, validationErrorf("unable to parse import file %q: %s", resolvedPath, err)
	}
	if file.Output.Mode != ast.OutputModeSchema {
		return nil, validationErrorf("%s target %q must output a schema", directiveKindName(kind), resolvedPath)
	}

	schemaNames := make([]string, 0, len(file.Output.SchemaFields))
	for _, field := range file.Output.SchemaFields {
		named, ok := field.Type.(ast.NamedType)
		if !ok {
			continue
		}
		if kind == ast.OutputDirectiveParseFile && field.Name != named.Name {
			continue
		}

		schemaNames = append(schemaNames, named.Name)
	}

	slices.Sort(schemaNames)
	return schemaNames, nil
}

func hasParseFile(directives []ast.OutputDirective) bool {
	for _, directive := range directives {
		if directive.Kind == ast.OutputDirectiveParseFile {
			return true
		}
	}
	return false
}

func hasSchemaFile(directives []ast.OutputDirective) bool {
	for _, directive := range directives {
		if directive.Kind == ast.OutputDirectiveSchemaFile {
			return true
		}
	}
	return false
}

func validateDataOutputFields(fields []ast.OutputField, hasSchema bool, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if _, exists := seen[field.Name]; exists {
			return diagnosticErrorAtNode(validationErrorf("duplicate output field %q", field.Name), field)
		}
		seen[field.Name] = struct{}{}

		if !hasSchema && expressionContainsEmptyRecord(field.Value) {
			return diagnosticErrorAtNode(validationErrorf("empty record output requires an output schema"), field)
		}
		if err := validateDataOutputExpression(field.Value, symbols); err != nil {
			return diagnosticErrorAtNode(err, field)
		}
		if err := validateExpressionPresence(field.Value, variables, symbols, types, schemas, enums, false); err != nil {
			return diagnosticErrorAtNode(err, field)
		}
	}

	return nil
}

func conditionalRequiresCollectionContext(expression ast.Expression, expressionType valueType) bool {
	return len(expressionType.members) > 0 && conditionalContainsEmptyCollection(expression)
}

func conditionalContainsEmptyCollection(expression ast.Expression) bool {
	conditional, ok := expression.(ast.ConditionalExpression)
	return ok && (expressionContainsEmptyCollection(conditional.Then) ||
		expressionContainsEmptyCollection(conditional.Else))
}

func expressionContainsEmptyCollection(expression ast.Expression) bool {
	if isEmptyCollectionExpression(expression) {
		return true
	}

	switch typed := expression.(type) {
	case ast.ArrayLiteral:
		return lo.ContainsBy(typed.Elements, expressionContainsEmptyCollection)
	case ast.RecordLiteral:
		return lo.ContainsBy(typed.Fields, func(field ast.RecordField) bool {
			return expressionContainsEmptyCollection(field.Value)
		})
	case ast.MemberAccess:
		return expressionContainsEmptyCollection(typed.Target)
	case ast.PrefixExpression:
		return expressionContainsEmptyCollection(typed.Right)
	case ast.InfixExpression:
		return expressionContainsEmptyCollection(typed.Left) || expressionContainsEmptyCollection(typed.Right)
	case ast.MatchExpression:
		if expressionContainsEmptyCollection(typed.Value) {
			return true
		}
		return lo.ContainsBy(typed.Arms, func(arm ast.MatchArm) bool {
			return expressionContainsEmptyCollection(arm.Value)
		})
	case ast.ConditionalExpression:
		return expressionContainsEmptyCollection(typed.Condition) ||
			expressionContainsEmptyCollection(typed.Then) ||
			expressionContainsEmptyCollection(typed.Else)
	default:
		return false
	}
}

func expressionContainsEmptyRecord(expression ast.Expression) bool {
	switch typed := expression.(type) {
	case ast.ArrayLiteral:
		return lo.ContainsBy(typed.Elements, expressionContainsEmptyRecord)
	case ast.RecordLiteral:
		if len(typed.Fields) == 0 {
			return true
		}
		return lo.ContainsBy(typed.Fields, func(field ast.RecordField) bool {
			return expressionContainsEmptyRecord(field.Value)
		})
	case ast.MemberAccess:
		return expressionContainsEmptyRecord(typed.Target)
	case ast.PrefixExpression:
		return expressionContainsEmptyRecord(typed.Right)
	case ast.InfixExpression:
		return expressionContainsEmptyRecord(typed.Left) || expressionContainsEmptyRecord(typed.Right)
	case ast.MatchExpression:
		if expressionContainsEmptyRecord(typed.Value) {
			return true
		}
		return lo.ContainsBy(typed.Arms, func(arm ast.MatchArm) bool {
			return expressionContainsEmptyRecord(arm.Value)
		})
	case ast.ConditionalExpression:
		return expressionContainsEmptyRecord(typed.Condition) ||
			expressionContainsEmptyRecord(typed.Then) ||
			expressionContainsEmptyRecord(typed.Else)
	default:
		return false
	}
}

func nullExpressionIn(expression ast.Expression) (ast.Expression, bool) {
	findNullInExpressions := func(expressions []ast.Expression) (ast.Expression, bool) {
		for _, expression := range expressions {
			if nullExpression, found := nullExpressionIn(expression); found {
				return nullExpression, true
			}
		}
		return nil, false
	}

	switch typed := expression.(type) {
	case ast.NullLiteral:
		return typed, true
	case ast.ArrayLiteral:
		return findNullInExpressions(typed.Elements)
	case ast.RecordLiteral:
		values := lo.Map(typed.Fields, func(field ast.RecordField, _ int) ast.Expression {
			return field.Value
		})
		return findNullInExpressions(values)
	case ast.MemberAccess:
		return nullExpressionIn(typed.Target)
	case ast.MatchExpression:
		nullExpression, found := nullExpressionIn(typed.Value)
		if found {
			return nullExpression, true
		}
		values := lo.Map(typed.Arms, func(arm ast.MatchArm, _ int) ast.Expression {
			return arm.Value
		})
		return findNullInExpressions(values)
	case ast.PrefixExpression:
		return nullExpressionIn(typed.Right)
	case ast.InfixExpression:
		return findNullInExpressions([]ast.Expression{typed.Left, typed.Right})
	case ast.ConditionalExpression:
		return findNullInExpressions([]ast.Expression{typed.Condition, typed.Then, typed.Else})
	default:
		return nil, false
	}
}

func isEmptyCollectionExpression(expression ast.Expression) bool {
	switch typed := expression.(type) {
	case ast.ArrayLiteral:
		return len(typed.Elements) == 0
	case ast.RecordLiteral:
		return len(typed.Fields) == 0
	default:
		return false
	}
}

func validateExpressionPresence(expression ast.Expression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, allowAbsent bool) error {
	switch expr := expression.(type) {
	case ast.MemberAccess:
		expressionType, err := inferExpressionType(expr, variables, symbols, types, schemas, enums)
		if err != nil {
			return diagnosticErrorAtNode(err, expr)
		}
		if expressionType.presence == presencePossiblyAbsent && !allowAbsent {
			return diagnosticErrorAtNode(possiblyAbsentValueError(), expr)
		}
		return validateExpressionPresence(expr.Target, variables, symbols, types, schemas, enums, true)
	case ast.MatchExpression:
		if _, err := inferMatchType(expr, variables, symbols, types, schemas, enums); err != nil {
			return err
		}
		if err := validateExpressionPresence(expr.Value, variables, symbols, types, schemas, enums, false); err != nil {
			return err
		}
		for _, arm := range expr.Arms {
			armVariables, err := matchArmVariables(expr.Value, arm.Pattern, variables, symbols, types, schemas, enums)
			if err != nil {
				return err
			}
			if err := validateExpressionPresence(arm.Value, armVariables, symbols, types, schemas, enums, false); err != nil {
				return err
			}
		}
	case ast.ArrayLiteral:
		for _, element := range expr.Elements {
			if err := validateExpressionPresence(element, variables, symbols, types, schemas, enums, false); err != nil {
				return err
			}
		}
	case ast.RecordLiteral:
		for _, field := range expr.Fields {
			if err := validateExpressionPresence(field.Value, variables, symbols, types, schemas, enums, false); err != nil {
				return err
			}
		}
	case ast.PrefixExpression:
		return validateExpressionPresence(expr.Right, variables, symbols, types, schemas, enums, false)
	case ast.InfixExpression:
		if expr.Operator == lexer.TokenCoalesce {
			_, err := inferCoalesceType(expr, variables, symbols, types, schemas, enums)
			return err
		}
		if err := validateExpressionPresence(expr.Left, variables, symbols, types, schemas, enums, false); err != nil {
			return err
		}
		return validateExpressionPresence(expr.Right, variables, symbols, types, schemas, enums, false)
	case ast.ConditionalExpression:
		if err := validateExpressionPresence(expr.Condition, variables, symbols, types, schemas, enums, false); err != nil {
			return err
		}
		thenVariables, elseVariables, err := conditionalVariables(expr.Condition, variables, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if err := validateExpressionPresence(expr.Then, thenVariables, symbols, types, schemas, enums, false); err != nil {
			return err
		}
		return validateExpressionPresence(expr.Else, elseVariables, symbols, types, schemas, enums, false)
	}

	return nil
}

func validateDataOutputExpression(expression ast.Expression, symbols *symbolTable) error {
	return validateDataOutputExpressionPosition(expression, symbols, true)
}

func validateDataOutputExpressionPosition(expression ast.Expression, symbols *symbolTable, isFieldRoot bool) error {
	switch expr := expression.(type) {
	case ast.NullLiteral:
		if !isFieldRoot {
			return diagnosticErrorAtNode(validationErrorf("null is only allowed in output"), expr)
		}
	case ast.Identifier:
		if symbols.IsType(expr.Name) || symbols.IsSchema(expr.Name) {
			return diagnosticErrorAtNode(
				diagnosticErrorf(ErrorValue, CodeOutputValueDeclaration, DiagnosticFields{Name: expr.Name}, "output value %q cannot reference type or schema declaration", expr.Name),
				expr,
			)
		}
	case ast.MemberAccess:
		return validateDataOutputExpressionPosition(expr.Target, symbols, false)
	case ast.MatchExpression:
		if err := validateDataOutputExpressionPosition(expr.Value, symbols, false); err != nil {
			return err
		}
		for _, arm := range expr.Arms {
			if err := validateDataOutputExpressionPosition(arm.Value, symbols, false); err != nil {
				return err
			}
		}
	case ast.ArrayLiteral:
		for _, element := range expr.Elements {
			if err := validateDataOutputExpressionPosition(element, symbols, false); err != nil {
				return err
			}
		}
	case ast.RecordLiteral:
		for _, field := range expr.Fields {
			if err := validateDataOutputExpressionPosition(field.Value, symbols, false); err != nil {
				return err
			}
		}
	case ast.PrefixExpression:
		return validateDataOutputExpressionPosition(expr.Right, symbols, false)
	case ast.InfixExpression:
		if err := validateDataOutputExpressionPosition(expr.Left, symbols, false); err != nil {
			return err
		}
		return validateDataOutputExpressionPosition(expr.Right, symbols, false)
	case ast.ConditionalExpression:
		if err := validateDataOutputExpressionPosition(expr.Condition, symbols, false); err != nil {
			return err
		}
		if err := validateDataOutputExpressionPosition(expr.Then, symbols, false); err != nil {
			return err
		}
		return validateDataOutputExpressionPosition(expr.Else, symbols, false)
	}

	return nil
}

func validateOutputSchema(schemaName string, items []ast.OutputField, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
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
	self := &schema
	for _, field := range schema.Fields {
		item, exists := fieldsByName[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			return missingRequiredFieldError(field.Name, schemaName)
		}
		if item.Optional && !field.Optional {
			return diagnosticErrorAtNode(
				validationErrorf("field %q is not optional in schema %q", field.Name, schemaName),
				item,
			)
		}
		expectedType, err := resolveValueTypeInRecord(field.Type, self, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if item.Shorthand {
			if _, ok := variables.Get(item.Name); !ok {
				return validationErrorf("unknown identifier %q", item.Name)
			}
		}
		if err := validateExpressionAgainstType(item.Value, expectedType, variables, symbols, types, schemas, enums); err != nil {
			return err
		}
	}

	for name, item := range fieldsByName {
		if _, exists := schemaFields[name]; !exists {
			return diagnosticErrorAtNode(
				validationErrorf("unknown output field %q for schema %q", name, schemaName),
				item,
			)
		}
	}

	return nil
}

func validateExpressionAgainstType(expression ast.Expression, expectedType valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	if _, ok := expression.(ast.NullLiteral); ok && expectedType.nullable {
		return nil
	}
	if match, ok := expression.(ast.MatchExpression); ok {
		return validateMatchAgainstType(match, expectedType, variables, symbols, types, schemas, enums)
	}
	if len(expectedType.members) > 0 {
		return validateExpressionAgainstVariantMembers(expression, expectedType.members, variables, symbols, types, schemas, enums)
	}
	if len(expectedType.choiceValues) > 0 {
		switch typed := expression.(type) {
		case ast.ConditionalExpression:
			return validateConditionalAgainstType(typed, expectedType, variables, symbols, types, schemas, enums)
		default:
			actualType, err := inferExpressionType(expression, variables, symbols, types, schemas, enums)
			if err != nil {
				return err
			}
			return ensureAssignable(expectedType, actualType)
		}
	}

	switch expectedType.kind {
	case ValueString, ValueInt, ValueFloat, ValueHexInt, ValueHexFloat, ValueBoolean:
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
			return validateConditionalAgainstType(typed, expectedType, variables, symbols, types, schemas, enums)
		default:
			actualType, err := inferExpressionType(expression, variables, symbols, types, schemas, enums)
			if err != nil {
				return err
			}
			return ensureAssignable(expectedType, actualType)
		}
	case ValueRecord:
		if expectedType.element != nil {
			switch typed := expression.(type) {
			case ast.RecordLiteral:
				for _, field := range typed.Fields {
					if err := validateExpressionAgainstType(field.Value, *expectedType.element, variables, symbols, types, schemas, enums); err != nil {
						return err
					}
				}
			case ast.ConditionalExpression:
				return validateConditionalAgainstType(typed, expectedType, variables, symbols, types, schemas, enums)
			default:
				actualType, err := inferExpressionType(expression, variables, symbols, types, schemas, enums)
				if err != nil {
					return err
				}
				return ensureAssignable(expectedType, actualType)
			}
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
			return validateConditionalAgainstType(typed, expectedType, variables, symbols, types, schemas, enums)
		default:
			actualType, err := inferExpressionType(expression, variables, symbols, types, schemas, enums)
			if err != nil {
				return err
			}
			return ensureAssignable(expectedType, actualType)
		}
	}
	return nil
}

func validateMatchAgainstType(expression ast.MatchExpression, expectedType valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	if _, err := inferMatchType(expression, variables, symbols, types, schemas, enums); err != nil {
		return err
	}
	for _, arm := range expression.Arms {
		armVariables, err := matchArmVariables(expression.Value, arm.Pattern, variables, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if err := validateExpressionAgainstType(arm.Value, expectedType, armVariables, symbols, types, schemas, enums); err != nil {
			return err
		}
	}
	return nil
}

func validateConditionalAgainstType(expression ast.ConditionalExpression, expectedType valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	thenVariables, elseVariables, err := conditionalVariables(expression.Condition, variables, symbols, types, schemas, enums)
	if err != nil {
		return err
	}
	if err := validateExpressionAgainstType(expression.Then, expectedType, thenVariables, symbols, types, schemas, enums); err != nil {
		return err
	}
	return validateExpressionAgainstType(expression.Else, expectedType, elseVariables, symbols, types, schemas, enums)
}

func validateExpressionAgainstVariantMembers(expression ast.Expression, members []valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	if conditional, ok := expression.(ast.ConditionalExpression); ok {
		thenVariables, elseVariables, err := conditionalVariables(conditional.Condition, variables, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		if err := validateExpressionAgainstVariantMembers(conditional.Then, members, thenVariables, symbols, types, schemas, enums); err != nil {
			return err
		}
		return validateExpressionAgainstVariantMembers(conditional.Else, members, elseVariables, symbols, types, schemas, enums)
	}

	if isEmptyCollectionExpression(expression) {
		return validateEmptyCollectionAgainstVariantMembers(expression, members, variables, symbols, types, schemas, enums)
	}

	actualType, err := inferExpressionType(expression, variables, symbols, types, schemas, enums)
	if err != nil {
		return err
	}
	if len(actualType.members) > 0 {
		return ensureAssignable(valueType{members: members}, actualType)
	}

	matchCount := countVariantChoiceMatchesForExpression(expression, actualType, members, variables, symbols, types, schemas, enums)
	if matchCount == 0 {
		matchCount = countVariantMatchesForExpression(expression, actualType, members, variables, symbols, types, schemas, enums)
	}

	if matchCount == 1 {
		return nil
	}
	if matchCount == 0 {
		return typeMismatchError(valueType{members: members}.name(), actualType.name())
	}

	return validationErrorf("type mismatch: expected exactly one variant member for %s", valueType{members: members}.name())
}

func validateEmptyCollectionAgainstVariantMembers(expression ast.Expression, members []valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	matchCount := 0
	for _, member := range members {
		if err := validateExpressionAgainstType(expression, member, variables, symbols, types, schemas, enums); err == nil {
			matchCount++
		}
	}
	if matchCount == 1 {
		return nil
	}
	if matchCount == 0 {
		return typeMismatchError(valueType{members: members}.name(), "empty collection")
	}

	return validationErrorf("type mismatch: expected exactly one variant member for empty collection")
}

func countVariantChoiceMatchesForExpression(expression ast.Expression, actualType valueType, members []valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) int {
	matchCount := 0
	for _, member := range members {
		if len(member.choiceValues) == 0 {
			continue
		}
		if err := ensureAssignable(member, actualType); err != nil {
			continue
		}
		if err := validateExpressionAgainstType(expression, member, variables, symbols, types, schemas, enums); err == nil {
			matchCount++
		}
	}
	return matchCount
}

func countVariantMatchesForExpression(expression ast.Expression, actualType valueType, members []valueType, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) int {
	matchCount := 0
	for _, member := range members {
		if !recordMapTypesMatch(member, actualType) {
			continue
		}
		if err := ensureAssignable(member, actualType); err != nil {
			continue
		}
		if err := validateExpressionAgainstType(expression, member, variables, symbols, types, schemas, enums); err == nil {
			matchCount++
		}
	}
	return matchCount
}

func validateRecordLiteral(expr ast.RecordLiteral, schemaName string, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	schema, ok := schemas.Get(schemaName)
	if !ok {
		return validationErrorf("unknown schema %q", schemaName)
	}

	return validateRecordLiteralAgainstRecordType(expr, schema, schemaName, variables, symbols, types, schemas, enums)
}

func validateRecordLiteralAgainstRecordType(expr ast.RecordLiteral, recordType ast.RecordType, schemaName string, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	fieldsByName := map[string]ast.RecordField{}
	for _, field := range expr.Fields {
		if _, exists := fieldsByName[field.Name]; exists {
			return validationErrorf("duplicate record field %q", field.Name)
		}
		fieldsByName[field.Name] = field
	}

	schemaFields := schemaFieldMap(recordType)
	self := &recordType
	for _, field := range recordType.Fields {
		recordField, exists := fieldsByName[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			if schemaName != "" {
				return missingRequiredFieldError(field.Name, schemaName)
			}
			return missingRequiredFieldError(field.Name, "")
		}
		if recordField.Optional && !field.Optional {
			if schemaName != "" {
				return validationErrorf("field %q is not optional in schema %q", field.Name, schemaName)
			}
			return validationErrorf("field %q is not optional", field.Name)
		}
		expectedType, err := resolveValueTypeInRecord(field.Type, self, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		expectedType.nullable = field.Optional
		if recordField.Shorthand {
			if _, ok := variables.Get(recordField.Name); !ok {
				return validationErrorf("unknown identifier %q", recordField.Name)
			}
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

func validateEvaluatedOutputSchema(schemaName string, fields map[string]Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	schema, ok := schemas.Get(schemaName)
	if !ok {
		return validationErrorf("unknown schema %q", schemaName)
	}

	schemaFields := schemaFieldMap(schema)
	self := &schema
	for _, field := range schema.Fields {
		value, exists := fields[field.Name]
		if !exists {
			if field.Optional {
				continue
			}
			return missingRequiredFieldError(field.Name, schemaName)
		}

		expectedType, err := resolveValueTypeInRecord(field.Type, self, symbols, types, schemas, enums)
		if err != nil {
			return err
		}
		expectedType.nullable = field.Optional
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

func validateEvaluatedValueAgainstType(value Value, expectedType valueType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	if value.Absent {
		return possiblyAbsentValueError()
	}
	if value.Kind == ValueNull {
		if expectedType.nullable {
			return nil
		}
		return invalidNullUsageError()
	}
	if len(expectedType.members) > 0 {
		return validateEvaluatedValueAgainstVariantMembers(value, expectedType.members, symbols, types, schemas, enums)
	}
	if len(expectedType.choiceValues) > 0 {
		if !choiceContainsValue(expectedType.choiceValues, value) {
			return typeMismatchError(expectedType.name(), scalarValueDisplay(value))
		}
		return nil
	}

	switch expectedType.kind {
	case ValueString, ValueInt, ValueFloat, ValueHexInt, ValueHexFloat, ValueBoolean:
		if value.Kind != expectedType.kind {
			return typeMismatchError(expectedType.name(), value.kindName())
		}
	case ValueArray:
		if value.Kind != ValueArray {
			return typeMismatchError(expectedType.name(), value.kindName())
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
		if value.Kind != ValueRecord {
			return typeMismatchError(expectedType.name(), value.kindName())
		}
		if expectedType.element != nil {
			for _, fieldValue := range value.Record {
				if err := validateEvaluatedValueAgainstType(fieldValue, *expectedType.element, symbols, types, schemas, enums); err != nil {
					return err
				}
			}
			return nil
		}
		if expectedType.record == nil && expectedType.schemaName == "" {
			return nil
		}

		recordType := expectedType.record
		if expectedType.schemaName != "" {
			schema, ok := schemas.Get(expectedType.schemaName)
			if !ok {
				return validationErrorf("unknown schema %q", expectedType.schemaName)
			}
			recordType = &schema
		}
		schemaFields := schemaFieldMap(*recordType)
		for _, field := range recordType.Fields {
			fieldValue, exists := value.Record[field.Name]
			if !exists {
				if field.Optional {
					continue
				}
				return missingRequiredFieldError(field.Name, expectedType.schemaName)
			}

			fieldType, err := resolveValueTypeInRecord(field.Type, recordType, symbols, types, schemas, enums)
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

func validateEvaluatedValueAgainstVariantMembers(value Value, members []valueType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	matchCount := countVariantChoiceMatchesForValue(value, members, symbols, types, schemas, enums)
	if matchCount == 0 {
		matchCount = countVariantMatchesForValue(value, members, symbols, types, schemas, enums)
	}

	if matchCount == 1 {
		return nil
	}
	if matchCount == 0 {
		return typeMismatchError(valueType{members: members}.name(), value.kindName())
	}

	return validationErrorf("type mismatch: expected exactly one variant member for %s", valueType{members: members}.name())
}

func countVariantChoiceMatchesForValue(value Value, members []valueType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) int {
	matchCount := 0
	for _, member := range members {
		if len(member.choiceValues) == 0 {
			continue
		}
		if err := validateEvaluatedValueAgainstType(value, member, symbols, types, schemas, enums); err == nil {
			matchCount++
		}
	}
	return matchCount
}

func countVariantMatchesForValue(value Value, members []valueType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) int {
	matchCount := 0
	for _, member := range members {
		if err := validateEvaluatedValueAgainstType(value, member, symbols, types, schemas, enums); err == nil {
			matchCount++
		}
	}
	return matchCount
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
	case ast.OutputDirectiveParse:
		return "parse"
	case ast.OutputDirectiveParseFile:
		return "parse_file"
	case ast.OutputDirectiveDescription:
		return "description"
	default:
		return "unknown"
	}
}

type Value struct {
	Absent  bool
	Kind    ValueKind
	Int     int64
	Float   float64
	Boolean bool
	String  string
	Array   []Value
	Record  map[string]Value
	Type    *valueType
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
	cloned := newValueEnvironment()
	cloned.values = environment.Values()

	return cloned
}

func evaluateSchemaOutput(output ast.OutputBlock, types *typeRegistry) (map[SchemaField]SchemaType, error) {
	if output.Mode != ast.OutputModeSchema {
		return map[SchemaField]SchemaType{}, nil
	}

	fields := make(map[SchemaField]SchemaType, len(output.SchemaFields))
	for _, field := range output.SchemaFields {
		schemaType, err := schemaTypeFromTypeReference(field.Type, types)
		if err != nil {
			return nil, err
		}

		fields[SchemaField{Name: field.Name, Optional: field.Optional}] = schemaType
	}

	return fields, nil
}

func evaluateScript(items []ast.Declaration, environment *valueEnvironment, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	for _, declaration := range items {
		variable, ok := declaration.(ast.VariableDeclaration)
		if !ok {
			continue
		}

		if !variable.HasValue {
			return diagnosticErrorAtNode(validationErrorf("variable %q requires an initializer", variable.Name), variable)
		}

		value, err := evaluateExpression(variable.Value, environment, Value{}, symbols, types, schemas, enums)
		if err != nil {
			return diagnosticErrorAtNode(err, variable)
		}

		expectedType, err := resolveValueType(variable.Type, symbols, types, schemas, enums)
		if err != nil {
			return diagnosticErrorAtNode(err, variable)
		}
		value, _ = coerceEvaluatedValueAgainstType(variable.Value, value, expectedType, environment, Value{}, symbols, types, schemas, enums)
		if err := validateEvaluatedValueAgainstType(value, expectedType, symbols, types, schemas, enums); err != nil {
			return diagnosticErrorAtNode(err, variable)
		}
		value.Type = &expectedType

		environment.Add(variable.Name, value)
	}

	return nil
}

func evaluateOutputFields(items []ast.OutputField, environment *valueEnvironment, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (map[string]Value, error) {
	fields := map[string]Value{}
	self := Value{Kind: ValueRecord, Record: fields}
	for _, item := range items {
		value, err := evaluateExpression(item.Value, environment, self, symbols, types, schemas, enums)
		if err != nil {
			return nil, diagnosticErrorAtNode(err, item)
		}
		if value.Absent {
			return nil, diagnosticErrorAtNode(possiblyAbsentValueError(), item)
		}
		if value.Kind == ValueNull {
			continue
		}
		fields[item.Name] = value
	}

	return fields, nil
}

func coerceEvaluatedValueAgainstType(expression ast.Expression, value Value, expectedType valueType, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
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

	return value, nil
}

func evaluateExpression(expression ast.Expression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	switch expr := expression.(type) {
	case ast.GroupedExpression:
		return evaluateExpression(expr.Expression, environment, self, symbols, types, schemas, enums)
	case ast.Identifier:
		value, ok := environment.Get(expr.Name)
		if !ok {
			if selfValue, exists := self.Record[expr.Name]; exists {
				return selfValue, nil
			}
			if symbols != nil {
				if kind, exists := symbols.Get(expr.Name); exists && kind != symbolKindVariable {
					return Value{}, diagnosticErrorAtNode(validationErrorf("type reference %q is not a valid value expression", expr.Name), expr)
				}
			}
			return Value{}, diagnosticErrorAtNode(validationErrorf("unknown identifier %q", expr.Name), expr)
		}
		return value, nil
	case ast.MemberAccess:
		return evaluateMemberAccess(expr, environment, self, symbols, types, schemas, enums)
	case ast.MatchExpression:
		return evaluateMatch(expr, environment, self, symbols, types, schemas, enums)
	case ast.IntLiteral:
		return parseInt(expr.Lexeme)
	case ast.FloatLiteral:
		return parseFloat(expr.Lexeme)
	case ast.HexIntLiteral:
		return parseHexInt(expr.Lexeme)
	case ast.HexFloatLiteral:
		return parseHexFloat(expr.Lexeme)
	case ast.StringLiteral:
		value, err := parseInterpolatedString(expr.Lexeme, environment, self, symbols, types, schemas, enums)
		return value, diagnosticErrorAtStringInterpolation(err, expr)
	case ast.BooleanLiteral:
		return Value{Kind: ValueBoolean, Boolean: expr.Value}, nil
	case ast.NullLiteral:
		return Value{Kind: ValueNull}, nil
	case ast.ArrayLiteral:
		return evaluateArrayLiteral(expr, environment, self, symbols, types, schemas, enums)
	case ast.RecordLiteral:
		return evaluateRecordLiteral(expr, environment, self, symbols, types, schemas, enums)
	case ast.SelfReference:
		value, err := evaluateSelfReference(expr, self)
		return value, diagnosticErrorAtNode(err, expr)
	case ast.PrefixExpression:
		return evaluatePrefix(expr, environment, self, symbols, types, schemas, enums)
	case ast.InfixExpression:
		value, err := evaluateInfix(expr, environment, self, symbols, types, schemas, enums)
		return value, diagnosticErrorAtNode(err, expr)
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
	if err != nil || !isFiniteFloat(value) {
		return Value{}, validationErrorf("invalid float literal %q", lexeme)
	}
	return Value{Kind: ValueFloat, Float: value}, nil
}

func parseHexInt(lexeme string) (Value, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(lexeme, "0x"), "0X")
	value, err := strconv.ParseInt(trimmed, 16, 64)
	if err != nil {
		return Value{}, validationErrorf("invalid hex_int literal %q", lexeme)
	}
	return Value{Kind: ValueHexInt, Int: value}, nil
}

func parseNegativeHexInt(lexeme string) (Value, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(lexeme, "0x"), "0X")
	magnitude, err := strconv.ParseUint(trimmed, 16, 64)
	if err != nil || magnitude > uint64(math.MaxInt64)+1 {
		return Value{}, validationErrorf("invalid hex_int literal %q", "-"+lexeme)
	}
	if magnitude == uint64(math.MaxInt64)+1 {
		return Value{Kind: ValueHexInt, Int: math.MinInt64}, nil
	}
	return Value{Kind: ValueHexInt, Int: -int64(magnitude)}, nil
}

func parseHexFloat(lexeme string) (Value, error) {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(lexeme, "0x"), "0X")
	parts := strings.Split(trimmed, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Value{}, validationErrorf("invalid hex_float literal %q", lexeme)
	}

	value, err := strconv.ParseFloat(lexeme+"p0", 64)
	if err != nil {
		numberError, isNumberError := err.(*strconv.NumError)
		if isNumberError && numberError.Err == strconv.ErrRange && math.IsInf(value, 1) && isFiniteHexFloatLiteral(parts[0], parts[1]) {
			return Value{Kind: ValueHexFloat, Float: math.MaxFloat64}, nil
		}
		if !isNumberError || numberError.Err != strconv.ErrRange || !isFiniteFloat(value) {
			return Value{}, validationErrorf("invalid hex_float literal %q", lexeme)
		}
	}
	if !isFiniteFloat(value) {
		return Value{}, validationErrorf("invalid hex_float literal %q", lexeme)
	}
	return Value{Kind: ValueHexFloat, Float: value}, nil
}

func isFiniteHexFloatLiteral(whole string, fraction string) bool {
	whole = strings.TrimLeft(whole, "0")
	if len(whole) < 256 {
		return true
	}
	if len(whole) > 256 {
		return false
	}
	if strings.Trim(whole, "Ff") != "" {
		return true
	}
	return strings.Trim(fraction, "0") == ""
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
	value, err := decodeStringLexeme(lexeme, true, func(expression string) (string, error) {
		return "$(" + expression + ")", nil
	})
	if err != nil {
		return Value{}, err
	}
	return Value{Kind: ValueString, String: value}, nil
}

func parseInterpolatedString(lexeme string, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
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

type interpolationSpanError struct {
	cause error
	start int
	end   int
}

func (err interpolationSpanError) Error() string {
	return err.cause.Error()
}

func (err interpolationSpanError) Unwrap() error {
	return err.cause
}

func decodeStringLexeme(lexeme string, allowInterpolation bool, interpolate func(string) (string, error)) (string, error) {
	content, quoteMode, err := stringContent(lexeme)
	if err != nil {
		return "", err
	}

	var builder strings.Builder
	for index := 0; index < len(content); {
		if content[index] == '\\' {
			escaped, width, err := unescapeSequence(content[index:])
			if err != nil {
				return "", err
			}
			builder.WriteString(escaped)
			index += width
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
				quoteWidth := 1
				if quoteMode == stringQuoteBlock {
					quoteWidth = 3
				}
				return "", interpolationSpanError{
					cause: err,
					start: quoteWidth + index,
					end:   quoteWidth + end,
				}
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

func unescapeSequence(sequence string) (string, int, error) {
	if len(sequence) < 2 {
		return "", 0, validationErrorf("invalid string escape")
	}

	switch sequence[1] {
	case '\\', '\'', '"':
		return string(sequence[1]), 2, nil
	case 'n':
		return "\n", 2, nil
	case 'r':
		return "\r", 2, nil
	case 't':
		return "\t", 2, nil
	case 'u':
		value, err := parseUnicodeEscape(sequence, 4)
		if err != nil {
			return "", 0, err
		}
		return string(value), 6, nil
	case 'U':
		value, err := parseUnicodeEscape(sequence, 8)
		if err != nil {
			return "", 0, err
		}
		return string(value), 10, nil
	default:
		return "", 0, validationErrorf("invalid string escape \\%c", sequence[1])
	}
}

func parseUnicodeEscape(sequence string, digitCount int) (rune, error) {
	width := 2 + digitCount
	if len(sequence) < width {
		return 0, validationErrorf("invalid unicode escape %q", sequence)
	}

	digits := sequence[2:width]
	for _, digit := range digits {
		if !isHexEscapeDigit(digit) {
			return 0, validationErrorf("invalid unicode escape \\%c%s", sequence[1], digits)
		}
	}

	codePoint, err := strconv.ParseInt(digits, 16, 32)
	if err != nil {
		return 0, validationErrorf("invalid unicode escape \\%c%s", sequence[1], digits)
	}

	value := rune(codePoint)
	if !utf8.ValidRune(value) || isSurrogateCodePoint(value) {
		return 0, validationErrorf("invalid unicode code point U+%04X", value)
	}

	return value, nil
}

func isHexEscapeDigit(value rune) bool {
	return (value >= '0' && value <= '9') || (value >= 'a' && value <= 'f') || (value >= 'A' && value <= 'F')
}

func isSurrogateCodePoint(value rune) bool {
	return value >= 0xD800 && value <= 0xDFFF
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

func FormatScalarValue(value Value) (string, error) {
	return stringifyValue(value)
}

func stringifyValue(value Value) (string, error) {
	switch value.Kind {
	case ValueString:
		return value.String, nil
	case ValueInt:
		return strconv.FormatInt(value.Int, 10), nil
	case ValueFloat:
		if !isFiniteFloat(value.Float) {
			return "", validationErrorf("cannot format non-finite float")
		}
		return strconv.FormatFloat(value.Float, 'f', -1, 64), nil
	case ValueHexInt:
		return formatHexInt(value.Int), nil
	case ValueHexFloat:
		if !isFiniteFloat(value.Float) {
			return "", validationErrorf("cannot format non-finite hex_float")
		}
		return formatHexFloat(value.Float), nil
	case ValueBoolean:
		return strconv.FormatBool(value.Boolean), nil
	default:
		return "", validationErrorf("interpolation requires a scalar value")
	}
}

func formatHexInt(value int64) string {
	if value < 0 {
		magnitude := uint64(-(value + 1))
		magnitude++
		return "-0x" + strings.ToUpper(strconv.FormatUint(magnitude, 16))
	}
	return "0x" + strings.ToUpper(strconv.FormatUint(uint64(value), 16))
}

func formatHexFloat(value float64) string {
	sign := ""
	if math.Signbit(value) {
		sign = "-"
		value = math.Abs(value)
	}
	if value == 0 {
		return sign + "0x0.0"
	}

	exact := new(big.Rat).SetFloat64(value)
	numerator := new(big.Int).Set(exact.Num())
	denominatorPower := exact.Denom().BitLen() - 1
	fractionalDigits := (denominatorPower + 3) / 4
	numerator.Lsh(numerator, uint(4*fractionalDigits-denominatorPower))

	if fractionalDigits == 0 {
		return sign + "0x" + strings.ToUpper(numerator.Text(16)) + ".0"
	}

	scale := new(big.Int).Lsh(big.NewInt(1), uint(4*fractionalDigits))
	whole := new(big.Int)
	fraction := new(big.Int)
	whole.QuoRem(numerator, scale, fraction)
	fractionText := fraction.Text(16)
	if padding := fractionalDigits - len(fractionText); padding > 0 {
		fractionText = strings.Repeat("0", padding) + fractionText
	}
	fractionText = strings.TrimRight(fractionText, "0")
	if fractionText == "" {
		fractionText = "0"
	}
	return sign + "0x" + strings.ToUpper(whole.Text(16)) + "." + strings.ToUpper(fractionText)
}

func evaluateMatch(expr ast.MatchExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	matched, err := evaluateExpression(expr.Value, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}

	for _, arm := range expr.Arms {
		if arm.Pattern.Type != nil {
			patternType, err := resolveValueType(arm.Pattern.Type, symbols, types, schemas, enums)
			if err != nil {
				return Value{}, err
			}
			if validateEvaluatedValueAgainstType(matched, patternType, symbols, types, schemas, enums) == nil {
				return evaluateExpression(arm.Value, environment, self, symbols, types, schemas, enums)
			}
			continue
		}

		patternValues, err := resolveChoiceMemberValues(arm.Pattern.Literal, types, map[string]struct{}{})
		if err != nil {
			return Value{}, err
		}
		if len(patternValues) == 1 && choiceContainsValue(patternValues, matched) {
			return evaluateExpression(arm.Value, environment, self, symbols, types, schemas, enums)
		}
	}

	return Value{}, validationErrorf("match expression has no matching arm")
}

func evaluateMemberAccess(expr ast.MemberAccess, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	target, err := evaluateExpression(expr.Target, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if target.Absent {
		if expr.Optional {
			return Value{Absent: true}, nil
		}
		return Value{}, optionalFieldAccessError(expr.Name)
	}
	if target.Kind == ValueNull {
		return Value{}, nullableVariableAccessError(expr.Name)
	}
	if target.Kind != ValueRecord {
		return Value{}, nonRecordMemberAccessError(expr.Name)
	}
	var memberType *valueType
	if target.Type != nil {
		record := target.Type.record
		if record == nil && target.Type.schemaName != "" {
			if schema, ok := schemas.Get(target.Type.schemaName); ok {
				record = &schema
			}
		}
		if record != nil {
			for _, field := range record.Fields {
				if field.Name != expr.Name {
					continue
				}
				if field.Optional && !expr.Optional {
					return Value{}, optionalFieldAccessError(expr.Name)
				}
				resolved, resolveErr := resolveValueTypeInRecord(field.Type, record, symbols, types, schemas, enums)
				if resolveErr != nil {
					return Value{}, resolveErr
				}
				memberType = &resolved
				break
			}
		}
	}
	member, ok := target.Record[expr.Name]
	if !ok {
		if expr.Optional {
			return Value{Absent: true}, nil
		}
		return Value{}, validationErrorf("unknown member %q", expr.Name)
	}
	member.Type = memberType
	return member, nil
}

func evaluatePrefix(expr ast.PrefixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	if expr.Operator == lexer.TokenMinus {
		if literal, ok := expr.Right.(ast.HexIntLiteral); ok {
			return parseNegativeHexInt(literal.Lexeme)
		}
	}

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
		if !isNumericValue(right) {
			return Value{}, validationErrorf("type mismatch: expected numeric after unary operator")
		}
		return right, nil
	case lexer.TokenMinus:
		switch right.Kind {
		case ValueInt, ValueHexInt:
			if right.Int == math.MinInt64 {
				return Value{}, validationErrorf("%s overflow", right.kindName())
			}
			return Value{Kind: right.Kind, Int: -right.Int}, nil
		case ValueFloat, ValueHexFloat:
			return finiteFloatValue(right.Kind, -right.Float)
		default:
			return Value{}, validationErrorf("type mismatch: expected numeric after unary operator")
		}
	default:
		return Value{}, validationErrorf("unknown prefix operator")
	}
}

func evaluateInfix(expr ast.InfixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	if expr.Operator == lexer.TokenCoalesce {
		return evaluateCoalesce(expr, environment, self, symbols, types, schemas, enums)
	}
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
	case lexer.TokenEqualEqual, lexer.TokenNotEqual:
		return evaluateEquality(expr.Operator, left, right)
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		return evaluateComparison(expr.Operator, left, right)
	default:
		return Value{}, validationErrorf("unknown infix operator")
	}
}

func evaluateCoalesce(expr ast.InfixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	left, err := evaluateExpression(expr.Left, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if !left.Absent {
		return left, nil
	}

	return evaluateExpression(expr.Right, environment, self, symbols, types, schemas, enums)
}

func evaluateNumeric(operator lexer.TokenType, left, right Value) (Value, error) {
	if !isNumericValue(left) || !isNumericValue(right) {
		return Value{}, validationErrorf("type mismatch: expected numeric operands for operator")
	}
	if isHexValue(left) || isHexValue(right) {
		if !isHexValue(left) || !isHexValue(right) {
			return Value{}, validationErrorf("type mismatch: expected hexadecimal operands for operator")
		}
		return evaluateHexNumeric(operator, left, right)
	}
	if left.Kind == ValueInt && right.Kind == ValueInt {
		return evaluateIntNumeric(operator, left.Int, right.Int)
	}
	return evaluateFloatNumeric(operator, numericValue(left), numericValue(right))
}

func evaluateHexNumeric(operator lexer.TokenType, left, right Value) (Value, error) {
	leftNumber := hexNumericValue(left)
	rightNumber := hexNumericValue(right)

	switch operator {
	case lexer.TokenPlus:
		if left.Kind == ValueHexInt && right.Kind == ValueHexInt {
			return checkedHexIntBinary(left.Int, right.Int, func(left, right *big.Int) *big.Int {
				return left.Add(left, right)
			})
		}
		return finiteFloatValue(ValueHexFloat, leftNumber+rightNumber)
	case lexer.TokenMinus:
		if left.Kind == ValueHexInt && right.Kind == ValueHexInt {
			return checkedHexIntBinary(left.Int, right.Int, func(left, right *big.Int) *big.Int {
				return left.Sub(left, right)
			})
		}
		return finiteFloatValue(ValueHexFloat, leftNumber-rightNumber)
	case lexer.TokenStar:
		if left.Kind == ValueHexInt && right.Kind == ValueHexInt {
			return checkedHexIntProduct(left.Int, right.Int)
		}
		return finiteFloatValue(ValueHexFloat, leftNumber*rightNumber)
	case lexer.TokenSlash:
		if rightNumber == 0 {
			return Value{}, validationErrorf("division by zero")
		}
		return finiteFloatValue(ValueHexFloat, leftNumber/rightNumber)
	case lexer.TokenDoubleStar:
		if left.Kind == ValueHexInt && right.Kind == ValueHexInt && right.Int >= 0 {
			return evaluateHexIntPower(left.Int, right.Int)
		}
		return finiteFloatValue(ValueHexFloat, math.Pow(leftNumber, rightNumber))
	default:
		return Value{}, validationErrorf("unknown numeric operator")
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
		return finiteFloatValue(ValueFloat, left+right)
	case lexer.TokenMinus:
		return finiteFloatValue(ValueFloat, left-right)
	case lexer.TokenStar:
		return finiteFloatValue(ValueFloat, left*right)
	case lexer.TokenSlash:
		if right == 0 {
			return Value{}, validationErrorf("division by zero")
		}
		return finiteFloatValue(ValueFloat, left/right)
	case lexer.TokenDoubleStar:
		return finiteFloatValue(ValueFloat, math.Pow(left, right))
	default:
		return Value{}, validationErrorf("unknown numeric operator")
	}
}

func checkedHexIntBinary(left int64, right int64, operation func(*big.Int, *big.Int) *big.Int) (Value, error) {
	result := operation(big.NewInt(left), big.NewInt(right))
	if !result.IsInt64() {
		return Value{}, validationErrorf("hex_int overflow")
	}
	return Value{Kind: ValueHexInt, Int: result.Int64()}, nil
}

func checkedHexIntProduct(left int64, right int64) (Value, error) {
	return checkedHexIntBinary(left, right, func(left, right *big.Int) *big.Int {
		return left.Mul(left, right)
	})
}

func evaluateHexIntPower(base int64, exponent int64) (Value, error) {
	result := Value{Kind: ValueHexInt, Int: 1}
	factor := base
	for exponent > 0 {
		if exponent&1 == 1 {
			product, err := checkedHexIntProduct(result.Int, factor)
			if err != nil {
				return Value{}, err
			}
			result = product
		}
		exponent >>= 1
		if exponent > 0 {
			product, err := checkedHexIntProduct(factor, factor)
			if err != nil {
				return Value{}, err
			}
			factor = product.Int
		}
	}
	return result, nil
}

func finiteFloatValue(kind ValueKind, value float64) (Value, error) {
	if !isFiniteFloat(value) {
		return Value{}, validationErrorf("non-finite %s result", Value{Kind: kind}.kindName())
	}
	return Value{Kind: kind, Float: value}, nil
}

func isFiniteFloat(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
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
	if isHexValue(left) || isHexValue(right) {
		if left.Kind != ValueHexInt || right.Kind != ValueHexInt {
			return Value{}, validationErrorf("type mismatch: modulo requires hex_int operands")
		}
		if right.Int == 0 {
			return Value{}, validationErrorf("division by zero")
		}
		return Value{Kind: ValueHexInt, Int: left.Int % right.Int}, nil
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
	return finiteFloatValue(ValueFloat, math.Mod(leftNumber, rightNumber))
}

func evaluateShift(operator lexer.TokenType, left, right Value) (Value, error) {
	if isHexValue(left) || isHexValue(right) {
		if left.Kind != ValueHexInt || right.Kind != ValueHexInt {
			return Value{}, validationErrorf("type mismatch: shift requires hex_int operands")
		}
		if right.Int < 0 {
			return Value{}, validationErrorf("negative shift count")
		}
		shift := uint(right.Int)
		switch operator {
		case lexer.TokenShiftLeft:
			if left.Int == 0 {
				return Value{Kind: ValueHexInt}, nil
			}
			if shift >= 64 {
				return Value{}, validationErrorf("hex_int overflow")
			}
			result := new(big.Int).Lsh(big.NewInt(left.Int), shift)
			if !result.IsInt64() {
				return Value{}, validationErrorf("hex_int overflow")
			}
			return Value{Kind: ValueHexInt, Int: result.Int64()}, nil
		case lexer.TokenShiftRight:
			return Value{Kind: ValueHexInt, Int: left.Int >> shift}, nil
		case lexer.TokenShiftRightUnsigned:
			return Value{Kind: ValueHexInt, Int: int64(uint64(left.Int) >> shift)}, nil
		default:
			return Value{}, validationErrorf("unknown shift operator")
		}
	}
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
	if isHexValue(left) || isHexValue(right) {
		if left.Kind != ValueHexInt || right.Kind != ValueHexInt {
			return Value{}, validationErrorf("type mismatch: bitwise operator requires hex_int operands")
		}
		switch operator {
		case lexer.TokenAmpersand:
			return Value{Kind: ValueHexInt, Int: left.Int & right.Int}, nil
		case lexer.TokenPipe:
			return Value{Kind: ValueHexInt, Int: left.Int | right.Int}, nil
		case lexer.TokenCaret:
			return Value{Kind: ValueHexInt, Int: left.Int ^ right.Int}, nil
		default:
			return Value{}, validationErrorf("unknown bitwise operator")
		}
	}
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
		if !isHexValue(left) || !isHexValue(right) {
			return Value{}, validationErrorf("type mismatch: incompatible equality comparison")
		}
	}

	equal, err := valuesEqual(left, right)
	if err != nil {
		return Value{}, err
	}

	if operator == lexer.TokenNotEqual {
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
	case ValueHexInt:
		if right.Kind == ValueHexFloat {
			return float64(left.Int) == right.Float, nil
		}
		return left.Int == right.Int, nil
	case ValueHexFloat:
		if right.Kind == ValueHexInt {
			return left.Float == float64(right.Int), nil
		}
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
	if isHexValue(left) || isHexValue(right) {
		if !isHexValue(left) || !isHexValue(right) {
			return Value{}, validationErrorf("type mismatch: expected operands from the same numeric family")
		}
		return compareNumbers(operator, hexNumericValue(left), hexNumericValue(right))
	}
	return compareNumbers(operator, numericValue(left), numericValue(right))
}

func isNumericValue(value Value) bool {
	return value.Kind == ValueInt || value.Kind == ValueFloat || value.Kind == ValueHexInt || value.Kind == ValueHexFloat
}

func isHexValue(value Value) bool {
	return value.Kind == ValueHexInt || value.Kind == ValueHexFloat
}

func numericValue(value Value) float64 {
	if value.Kind == ValueFloat {
		return value.Float
	}
	return float64(value.Int)
}

func hexNumericValue(value Value) float64 {
	if value.Kind == ValueHexFloat {
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

func evaluateLogicalAnd(expr ast.InfixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
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

func evaluateLogicalOr(expr ast.InfixExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
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

func evaluateConditional(expr ast.ConditionalExpression, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	condition, err := evaluateExpression(expr.Condition, environment, self, symbols, types, schemas, enums)
	if err != nil {
		return Value{}, err
	}
	if condition.Kind != ValueBoolean && condition.Kind != ValueNull && condition.Kind != ValueRecord {
		return Value{}, validationErrorf("type mismatch: expected boolean or record condition")
	}

	if isTruthy(condition) {
		return evaluateExpression(expr.Then, environment, self, symbols, types, schemas, enums)
	}

	return evaluateExpression(expr.Else, environment, self, symbols, types, schemas, enums)
}

func isTruthy(value Value) bool {
	return value.Kind != ValueNull && (value.Kind != ValueBoolean || value.Boolean)
}

func evaluateArrayLiteral(expr ast.ArrayLiteral, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	values := make([]Value, 0, len(expr.Elements))
	for _, element := range expr.Elements {
		value, err := evaluateExpression(element, environment, self, symbols, types, schemas, enums)
		if err != nil {
			return Value{}, err
		}
		if value.Absent {
			return Value{}, possiblyAbsentValueError()
		}
		values = append(values, value)
	}
	return Value{Kind: ValueArray, Array: values}, nil
}

func evaluateRecordLiteral(expr ast.RecordLiteral, environment *valueEnvironment, self Value, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (Value, error) {
	fields := map[string]Value{}
	for _, field := range expr.Fields {
		if _, exists := fields[field.Name]; exists {
			return Value{}, validationErrorf("duplicate record field %q", field.Name)
		}
		value, err := evaluateExpression(field.Value, environment, self, symbols, types, schemas, enums)
		if err != nil {
			return Value{}, err
		}
		if value.Absent {
			return Value{}, possiblyAbsentValueError()
		}
		if value.Kind == ValueNull {
			continue
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
			return Value{}, diagnosticErrorf(ErrorValue, CodeSelfReferenceUnknown, DiagnosticFields{Name: name}, "unknown self reference %q", name)
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
	case ValueHexInt:
		return "hex_int"
	case ValueHexFloat:
		return "hex_float"
	case ValueBoolean:
		return "boolean"
	case ValueRecord:
		return "record"
	case ValueNull:
		return "null"
	case ValueString:
		return "string"
	default:
		return "unknown"
	}
}

type ValueKind int

const (
	ValueUnknown ValueKind = iota
	ValueNull
	ValueString
	ValueInt
	ValueFloat
	ValueHexInt
	ValueHexFloat
	ValueBoolean
	ValueArray
	ValueRecord
)

type valuePresence int

const (
	presenceGuaranteed valuePresence = iota
	presencePossiblyAbsent
)

type valueType struct {
	kind         ValueKind
	nullable     bool
	presence     valuePresence
	element      *valueType
	schemaName   string
	record       *ast.RecordType
	exactValue   *Value
	choiceValues []Value
	members      []valueType
}

func isHexValueType(valueType valueType) bool {
	return valueType.kind == ValueHexInt || valueType.kind == ValueHexFloat
}

func (t valueType) isNumeric() bool {
	return t.kind == ValueInt || t.kind == ValueFloat || t.kind == ValueHexInt || t.kind == ValueHexFloat
}

func (t valueType) name() string {
	if len(t.choiceValues) > 0 {
		name := choiceTypeName(t.choiceValues)
		if t.nullable {
			return "nullable " + name
		}
		return name
	}

	var name string
	switch t.kind {
	case ValueNull:
		name = "null"
	case ValueString:
		name = "string"
	case ValueInt:
		name = "int"
	case ValueFloat:
		name = "float"
	case ValueHexInt:
		name = "hex_int"
	case ValueHexFloat:
		name = "hex_float"
	case ValueBoolean:
		name = "boolean"
	case ValueArray:
		if t.element != nil {
			name = fmt.Sprintf("array<%s>", t.element.name())
			break
		}
		name = "array"
	case ValueRecord:
		if t.element != nil {
			name = fmt.Sprintf("record<%s>", t.element.name())
			break
		}
		if t.schemaName != "" {
			name = t.schemaName
			break
		}
		name = "record"
	default:
		if len(t.members) > 0 {
			parts := lo.Map(t.members, func(member valueType, _ int) string {
				return member.name()
			})
			name = fmt.Sprintf("variant[%s]", strings.Join(parts, ", "))
			break
		}
		name = "unknown"
	}
	if t.nullable {
		return "nullable " + name
	}
	return name
}

func resolveValueType(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	return resolveValueTypeWithContext(typeRef, symbols, types, schemas, enums, nil, map[string]struct{}{})
}

func resolveValueTypeInRecord(typeRef ast.TypeReference, record *ast.RecordType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	return resolveValueTypeWithContext(typeRef, symbols, types, schemas, enums, record, map[string]struct{}{})
}

func resolveValueTypeWithAliases(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, aliases map[string]struct{}) (valueType, error) {
	return resolveValueTypeWithContext(typeRef, symbols, types, schemas, enums, nil, aliases)
}

func resolveValueTypeWithContext(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, self *ast.RecordType, aliases map[string]struct{}) (valueType, error) {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return primitiveValueType(ref.Name)
	case ast.SelfType:
		if self == nil {
			return valueType{}, validationErrorf("expected type reference")
		}
		return valueType{kind: ValueRecord, record: self}, nil
	case ast.ArrayType:
		element, err := resolveValueTypeWithContext(ref.Element, symbols, types, schemas, enums, self, aliases)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueArray, element: &element}, nil
	case ast.RecordMapType:
		value, err := resolveValueTypeWithContext(ref.Value, symbols, types, schemas, enums, self, aliases)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueRecord, element: &value}, nil
	case ast.ChoiceType:
		return resolveChoiceType(ref, types)
	case ast.UnionType:
		return resolveFusionValueTypeWithContext(ref, symbols, types, schemas, enums, self, aliases)
	case ast.VariantType:
		members := make([]valueType, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolved, err := resolveValueTypeWithContext(member, symbols, types, schemas, enums, self, aliases)
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
		record := ref
		return valueType{kind: ValueRecord, record: &record}, nil
	case ast.NamedType:
		if _, exists := aliases[ref.Name]; exists {
			return valueType{}, validationErrorf("cyclic type alias %q", ref.Name)
		}
		resolved, resolvedByAlias, err := types.Resolve(ref.Name)
		if err != nil {
			return valueType{}, err
		}
		if resolvedByAlias {
			return resolveValueTypeWithContext(resolved, symbols, types, schemas, enums, self, aliasesWith(aliases, ref.Name))
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

func aliasesWith(aliases map[string]struct{}, name string) map[string]struct{} {
	next := make(map[string]struct{}, len(aliases)+1)
	for alias := range aliases {
		next[alias] = struct{}{}
	}
	next[name] = struct{}{}
	return next
}

func resolveFusionValueType(reference ast.UnionType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, aliases map[string]struct{}) (valueType, error) {
	return resolveFusionValueTypeWithContext(reference, symbols, types, schemas, enums, nil, aliases)
}

func resolveFusionValueTypeWithContext(reference ast.UnionType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, self *ast.RecordType, aliases map[string]struct{}) (valueType, error) {
	members := make([]valueType, 0, len(reference.Members))
	allChoices := true
	allRecords := true

	for _, member := range reference.Members {
		resolved, err := resolveValueTypeWithContext(member, symbols, types, schemas, enums, self, aliases)
		if err != nil {
			return valueType{}, err
		}
		members = append(members, resolved)
		allChoices = allChoices && len(resolved.choiceValues) > 0
		allRecords = allRecords && resolved.kind == ValueRecord
	}

	if allChoices {
		values := make([]Value, 0)
		for _, member := range members {
			for _, value := range member.choiceValues {
				if !choiceContainsValue(values, value) {
					values = append(values, value)
				}
			}
		}
		return valueType{choiceValues: values}, nil
	}

	if allRecords {
		record, err := resolveUnionRecordType(reference, symbols, types, schemas)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueRecord, record: &record}, nil
	}

	return valueType{}, validationErrorf("fusion members must be schemas or choices")
}

func validateVariantValueTypes(members []valueType) error {
	members = flattenVariantValueTypes(members)

	for index, member := range members {
		switch {
		case len(member.choiceValues) > 0:
		case member.kind == ValueArray:
		case member.kind == ValueRecord:
		case member.kind == ValueString || member.kind == ValueInt || member.kind == ValueFloat || member.kind == ValueHexInt || member.kind == ValueHexFloat || member.kind == ValueBoolean:
		default:
			return validationErrorf("variant members must be primitives, arrays, or schemas")
		}

		for _, prior := range members[:index] {
			if typesEqual(prior, member) {
				return validationErrorf("duplicate variant member %s", member.name())
			}
		}
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

		return ast.RecordType{}, validationErrorf("fusion members must be schemas")
	default:
		return ast.RecordType{}, validationErrorf("fusion members must be schemas")
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
			return ast.RecordType{}, validationErrorf("conflicting field %q in fusion schema composition", field.Name)
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
	case "hex_int":
		return valueType{kind: ValueHexInt}, nil
	case "hex_float":
		return valueType{kind: ValueHexFloat}, nil
	case "boolean":
		return valueType{kind: ValueBoolean}, nil
	default:
		return valueType{}, validationErrorf("unknown type %q", name)
	}
}

func schemaTypeFromTypeReference(typeRef ast.TypeReference, types *typeRegistry) (SchemaType, error) {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return SchemaType{Kind: SchemaTypePrimitive, Name: ref.Name}, nil
	case ast.NamedType:
		return SchemaType{Kind: SchemaTypeNamed, Name: ref.Name}, nil
	case ast.ArrayType:
		element, err := schemaTypeFromTypeReference(ref.Element, types)
		if err != nil {
			return SchemaType{}, err
		}

		return SchemaType{Kind: SchemaTypeArray, Element: &element}, nil
	case ast.RecordMapType:
		value, err := schemaTypeFromTypeReference(ref.Value, types)
		if err != nil {
			return SchemaType{}, err
		}
		return SchemaType{Kind: SchemaTypeRecordMap, Element: &value}, nil
	case ast.UnionType:
		if values, ok, err := resolveFusionChoiceValues(ref, types); err != nil {
			return SchemaType{}, err
		} else if ok {
			return SchemaType{Kind: SchemaTypeNamed, Name: choiceTypeName(values)}, nil
		}
		members := make([]SchemaType, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolved, err := schemaTypeFromTypeReference(member, types)
			if err != nil {
				return SchemaType{}, err
			}
			members = append(members, resolved)
		}
		return SchemaType{Kind: SchemaTypeUnion, Members: members}, nil
	case ast.VariantType:
		members := make([]SchemaType, 0, len(ref.Members))
		for _, member := range ref.Members {
			resolved, err := schemaTypeFromTypeReference(member, types)
			if err != nil {
				return SchemaType{}, err
			}
			members = append(members, resolved)
		}
		return SchemaType{Kind: SchemaTypeVariant, Members: members}, nil
	case ast.ChoiceType:
		return SchemaType{Kind: SchemaTypeNamed, Name: choiceTypeNameForSchema(ref, types)}, nil
	case ast.RecordType:
		fields := make(map[SchemaField]SchemaType, len(ref.Fields))
		for _, field := range ref.Fields {
			fieldType, err := schemaTypeFromTypeReference(field.Type, types)
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

func resolveFusionChoiceValues(reference ast.TypeReference, types *typeRegistry) ([]Value, bool, error) {
	switch typed := reference.(type) {
	case ast.ChoiceType:
		resolved, err := resolveChoiceType(typed, types)
		if err != nil {
			return nil, false, err
		}
		return resolved.choiceValues, true, nil
	case ast.UnionType:
		values := make([]Value, 0)
		for _, member := range typed.Members {
			memberValues, ok, err := resolveFusionChoiceValues(member, types)
			if err != nil || !ok {
				return nil, false, err
			}
			for _, value := range memberValues {
				if !choiceContainsValue(values, value) {
					values = append(values, value)
				}
			}
		}
		return values, true, nil
	case ast.NamedType:
		resolved, ok, err := types.Resolve(typed.Name)
		if err != nil || !ok {
			return nil, false, err
		}
		return resolveFusionChoiceValues(resolved, types)
	default:
		return nil, false, nil
	}
}

func inferExpressionType(expression ast.Expression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	switch expr := expression.(type) {
	case ast.GroupedExpression:
		return inferExpressionType(expr.Expression, variables, symbols, types, schemas, enums)
	case ast.Identifier:
		if variableType, ok := variables.Get(expr.Name); ok {
			return variableType, nil
		}
		if symbols != nil {
			if kind, ok := symbols.Get(expr.Name); ok && kind != symbolKindVariable {
				return valueType{}, diagnosticErrorAtNode(validationErrorf("type reference %q is not a valid value expression", expr.Name), expr)
			}
		}
		return valueType{kind: ValueUnknown}, nil
	case ast.MemberAccess:
		return inferMemberExpressionType(expr, variables, symbols, types, schemas, enums, false)
	case ast.MatchExpression:
		return inferMatchType(expr, variables, symbols, types, schemas, enums)
	case ast.IntLiteral:
		value, err := parseInt(expr.Lexeme)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueInt, exactValue: &value}, nil
	case ast.FloatLiteral:
		value, err := parseFloat(expr.Lexeme)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueFloat, exactValue: &value}, nil
	case ast.HexIntLiteral:
		value, err := parseHexInt(expr.Lexeme)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueHexInt, exactValue: &value}, nil
	case ast.HexFloatLiteral:
		value, err := parseHexFloat(expr.Lexeme)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: ValueHexFloat, exactValue: &value}, nil
	case ast.StringLiteral:
		value, err := parseStaticString(expr.Lexeme)
		if err != nil {
			return valueType{kind: ValueString}, nil
		}
		return valueType{kind: ValueString, exactValue: &value}, nil
	case ast.BooleanLiteral:
		value := Value{Kind: ValueBoolean, Boolean: expr.Value}
		return valueType{kind: ValueBoolean, exactValue: &value}, nil
	case ast.NullLiteral:
		value := Value{Kind: ValueNull}
		return valueType{kind: ValueNull, exactValue: &value}, nil
	case ast.ArrayLiteral:
		return inferArrayLiteralType(expr, variables, symbols, types, schemas, enums)
	case ast.RecordLiteral:
		return inferRecordLiteralType(expr, variables, symbols, types, schemas, enums)
	case ast.SelfReference:
		return valueType{kind: ValueUnknown}, nil
	case ast.PrefixExpression:
		return inferPrefixType(expr, variables, symbols, types, schemas, enums)
	case ast.InfixExpression:
		inferredType, err := inferInfixType(expr, variables, symbols, types, schemas, enums)
		return inferredType, diagnosticErrorAtNode(err, expr)
	case ast.ConditionalExpression:
		return inferConditionalType(expr, variables, symbols, types, schemas, enums)
	default:
		return valueType{}, validationErrorf("unknown expression")
	}
}

func inferMemberExpressionType(expr ast.MemberAccess, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any, hasFollowingAccess bool) (valueType, error) {
	if path, stable := stableExpressionPath(expr); stable {
		if narrowedType, ok := variables.Get(path); ok {
			return narrowedType, nil
		}
	}

	var targetType valueType
	var err error
	if targetAccess, ok := expr.Target.(ast.MemberAccess); ok {
		targetType, err = inferMemberExpressionType(targetAccess, variables, symbols, types, schemas, enums, true)
	} else {
		targetType, err = inferExpressionType(expr.Target, variables, symbols, types, schemas, enums)
	}
	if err != nil {
		if isOptionalFieldAccessError(err) {
			if !expr.Optional {
				return valueType{}, diagnosticErrorAtToken(err, expr.OperatorToken)
			}
		} else {
			return valueType{}, err
		}
	}
	if targetType.kind == ValueUnknown && len(targetType.members) == 0 && len(targetType.choiceValues) == 0 {
		return valueType{kind: ValueUnknown}, nil
	}
	if targetType.nullable {
		return valueType{}, diagnosticErrorAtToken(nullableVariableAccessError(expr.Name), expr.OperatorToken)
	}
	if targetType.presence == presencePossiblyAbsent && !expr.Optional {
		return valueType{}, diagnosticErrorAtToken(optionalFieldAccessError(expr.Name), expr.OperatorToken)
	}

	memberType, err := inferMemberAccessType(targetType, expr, symbols, types, schemas, enums)
	if isOptionalFieldAccessError(err) {
		if hasFollowingAccess {
			return memberType, err
		}
		return valueType{}, diagnosticErrorAtToken(err, expr.OperatorToken)
	}
	if err != nil {
		memberRange := expr.Range()
		return memberType, diagnosticErrorAtRange(err, ast.SourceRange{
			Start: ast.TokenRange(expr.OperatorToken).Start,
			End:   memberRange.End,
		})
	}
	return memberType, nil
}

func isOptionalFieldAccessError(err error) bool {
	var diagnosticError DiagnosticError
	return errors.As(err, &diagnosticError) && diagnosticError.Code == CodeOptionalFieldAccess
}

func inferMatchType(expr ast.MatchExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	matchedType, err := inferExpressionType(expr.Value, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}
	if len(matchedType.members) == 0 && len(matchedType.choiceValues) == 0 {
		return valueType{}, diagnosticErrorAtNode(
			validationErrorf("match input must be a variant or choice"),
			expr.Value,
		)
	}
	if err := validateMatchPatterns(expr.Arms, matchedType, symbols, types, schemas, enums); err != nil {
		return valueType{}, diagnosticErrorAtNode(err, expr)
	}

	var result valueType
	for index, arm := range expr.Arms {
		armVariables, err := matchArmVariables(expr.Value, arm.Pattern, variables, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		armType, err := inferExpressionType(arm.Value, armVariables, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		if index == 0 {
			result = armType
			continue
		}
		result = unifyConditionalTypes(result, armType)
	}
	return result, nil
}

func validateMatchPatterns(arms []ast.MatchArm, matchedType valueType, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) error {
	if len(matchedType.members) > 0 {
		matchedMembers := flattenVariantValueTypes(matchedType.members)
		seen := make([]valueType, 0, len(arms))
		for _, arm := range arms {
			if arm.Pattern.Type == nil {
				return diagnosticErrorAtNode(
					validationErrorf("variant match arms require a type pattern"),
					arm.Pattern,
				)
			}
			patternType, err := resolveValueType(arm.Pattern.Type, symbols, types, schemas, enums)
			if err != nil {
				return diagnosticErrorAtNode(err, arm.Pattern)
			}
			patternMembers := []valueType{patternType}
			if len(patternType.members) > 0 {
				patternMembers = flattenVariantValueTypes(patternType.members)
			}
			for _, patternMember := range patternMembers {
				if !containsValueType(matchedMembers, patternMember) {
					return diagnosticErrorAtNode(
						validationErrorf("match pattern %s is not a member of %s", patternType.name(), matchedType.name()),
						arm.Pattern,
					)
				}
				if containsValueType(seen, patternMember) {
					return diagnosticErrorAtNode(
						validationErrorf("duplicate match pattern %s", patternMember.name()),
						arm.Pattern,
					)
				}
				seen = append(seen, patternMember)
			}
		}
		if len(seen) != len(matchedMembers) {
			return validationErrorf("match expression must be exhaustive for %s", matchedType.name())
		}
		return nil
	}

	seen := []Value{}
	for _, arm := range arms {
		if arm.Pattern.Literal == nil {
			return diagnosticErrorAtNode(
				validationErrorf("choice match arms require a literal pattern"),
				arm.Pattern,
			)
		}
		values, err := resolveChoiceMemberValues(arm.Pattern.Literal, types, map[string]struct{}{})
		if err != nil {
			return diagnosticErrorAtNode(err, arm.Pattern)
		}
		pattern := values[0]
		if !choiceContainsValue(matchedType.choiceValues, pattern) {
			return diagnosticErrorAtNode(
				validationErrorf("match pattern %s is not a member of %s", scalarValueDisplay(pattern), matchedType.name()),
				arm.Pattern,
			)
		}
		if choiceContainsValue(seen, pattern) {
			return diagnosticErrorAtNode(
				validationErrorf("duplicate match pattern %s", scalarValueDisplay(pattern)),
				arm.Pattern,
			)
		}
		seen = append(seen, pattern)
	}
	if len(seen) != len(matchedType.choiceValues) {
		return validationErrorf("match expression must be exhaustive for %s", matchedType.name())
	}
	return nil
}

func matchArmVariables(expression ast.Expression, pattern ast.MatchPattern, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (*variableRegistry, error) {
	if pattern.Type == nil {
		return variables, nil
	}
	path, stable := stableExpressionPath(expression)
	if !stable {
		return variables, nil
	}
	patternType, err := resolveValueType(pattern.Type, symbols, types, schemas, enums)
	if err != nil {
		return nil, err
	}

	narrowed := variables.Clone()
	narrowed.Add(path, patternType)
	return narrowed, nil
}

func containsValueType(types []valueType, candidate valueType) bool {
	return lo.ContainsBy(types, func(existing valueType) bool {
		return typesEqual(existing, candidate)
	})
}

func inferMemberAccessType(targetType valueType, expr ast.MemberAccess, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	if len(targetType.members) > 0 {
		memberTypes := make([]valueType, 0, len(targetType.members))
		for _, variantMember := range targetType.members {
			memberType, err := inferMemberAccessType(variantMember, expr, symbols, types, schemas, enums)
			if err != nil {
				return valueType{}, err
			}
			memberTypes = append(memberTypes, memberType)
		}

		presence := presenceGuaranteed
		if expr.Optional || targetType.presence == presencePossiblyAbsent {
			presence = presencePossiblyAbsent
		}
		return valueType{members: memberTypes, presence: presence}, nil
	}
	if targetType.kind == ValueUnknown && len(targetType.choiceValues) == 0 {
		return valueType{kind: ValueUnknown}, nil
	}
	if targetType.kind != ValueRecord {
		return valueType{}, nonRecordMemberAccessError(expr.Name)
	}
	if targetType.element != nil {
		memberType := *targetType.element
		memberType.presence = presenceGuaranteed
		if expr.Optional || targetType.presence == presencePossiblyAbsent {
			memberType.presence = presencePossiblyAbsent
		}
		return memberType, nil
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
		memberType, err := resolveValueTypeInRecord(field.Type, targetType.record, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		memberType.presence = presenceGuaranteed
		if field.Optional || targetType.presence == presencePossiblyAbsent {
			memberType.presence = presencePossiblyAbsent
		}
		if field.Optional && !expr.Optional {
			return memberType, optionalFieldAccessError(expr.Name)
		}
		return memberType, nil
	}
	return valueType{}, validationErrorf("unknown field %q on %s", expr.Name, targetType.name())
}

func inferArrayLiteralType(expr ast.ArrayLiteral, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	if len(expr.Elements) == 0 {
		return valueType{kind: ValueArray, element: &valueType{kind: ValueString}}, nil
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

func inferRecordLiteralType(expr ast.RecordLiteral, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	if len(expr.Fields) == 0 {
		return valueType{kind: ValueRecord}, nil
	}

	valueTypes := []valueType{}
	for _, field := range expr.Fields {
		fieldType, err := inferExpressionType(field.Value, variables, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		valueTypes = appendUniqueValueType(valueTypes, fieldType)
	}

	if len(valueTypes) == 1 {
		return valueType{kind: ValueRecord, element: &valueTypes[0]}, nil
	}

	variantType := valueType{members: valueTypes}
	return valueType{kind: ValueRecord, element: &variantType}, nil
}

func inferPrefixType(expr ast.PrefixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	if expr.Operator == lexer.TokenMinus {
		if literal, ok := expr.Right.(ast.HexIntLiteral); ok {
			value, err := parseNegativeHexInt(literal.Lexeme)
			if err != nil {
				return valueType{}, err
			}
			return valueType{kind: ValueHexInt, exactValue: &value}, nil
		}
	}

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

func inferInfixType(expr ast.InfixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	if expr.Operator == lexer.TokenCoalesce {
		return inferCoalesceType(expr, variables, symbols, types, schemas, enums)
	}

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
		if leftType.kind == ValueHexInt && rightType.kind == ValueHexInt {
			return valueType{kind: ValueHexInt}, nil
		}
		if leftType.kind == ValueHexInt || leftType.kind == ValueHexFloat || rightType.kind == ValueHexInt || rightType.kind == ValueHexFloat {
			return valueType{}, validationErrorf("type mismatch: modulo requires hex_int operands")
		}
		if leftType.kind == ValueInt && rightType.kind == ValueInt {
			return valueType{kind: ValueInt}, nil
		}
		return valueType{kind: ValueFloat}, nil
	case lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned:
		if leftType.kind == ValueHexInt && rightType.kind == ValueHexInt {
			return valueType{kind: ValueHexInt}, nil
		}
		if leftType.kind == ValueHexInt || leftType.kind == ValueHexFloat || rightType.kind == ValueHexInt || rightType.kind == ValueHexFloat {
			return valueType{}, validationErrorf("type mismatch: shift requires hex_int operands")
		}
		if leftType.kind != ValueInt || rightType.kind != ValueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for shift")
		}
		return valueType{kind: ValueInt}, nil
	case lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret:
		if leftType.kind == ValueHexInt && rightType.kind == ValueHexInt {
			return valueType{kind: ValueHexInt}, nil
		}
		if leftType.kind == ValueHexInt || leftType.kind == ValueHexFloat || rightType.kind == ValueHexInt || rightType.kind == ValueHexFloat {
			return valueType{}, validationErrorf("type mismatch: bitwise operator requires hex_int operands")
		}
		if leftType.kind != ValueInt || rightType.kind != ValueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for bitwise operator")
		}
		return valueType{kind: ValueInt}, nil
	case lexer.TokenEqualEqual, lexer.TokenNotEqual:
		if leftType.kind != ValueUnknown && rightType.kind != ValueUnknown && !typesEqual(leftType, rightType) {
			if !isHexValueType(leftType) || !isHexValueType(rightType) {
				return valueType{}, validationErrorf("type mismatch: incompatible equality comparison")
			}
		}
		return valueType{kind: ValueBoolean}, nil
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		if !leftType.isNumeric() || !rightType.isNumeric() {
			return valueType{}, validationErrorf("type mismatch: expected numeric operands for comparison")
		}
		if isHexValueType(leftType) != isHexValueType(rightType) {
			return valueType{}, validationErrorf("type mismatch: expected operands from the same numeric family")
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

func inferCoalesceType(expr ast.InfixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	operands := coalesceOperands(expr)
	var result valueType

	for index, operand := range operands {
		operandType, err := inferExpressionType(operand, variables, symbols, types, schemas, enums)
		if err != nil {
			return valueType{}, err
		}
		if index == len(operands)-1 {
			if operandType.nullable || operandType.presence != presenceGuaranteed {
				return valueType{}, validationErrorf("coalescing fallback must be guaranteed present")
			}
			if !typesEqual(result, operandType) {
				return valueType{}, typeMismatchError(result.name(), operandType.name())
			}
			result.presence = presenceGuaranteed
			return result, nil
		}
		if operandType.presence != presencePossiblyAbsent {
			return valueType{}, validationErrorf("coalescing operands before the fallback must be possibly absent")
		}
		if index == 0 {
			result = operandType
			continue
		}
		if !typesEqual(result, operandType) {
			return valueType{}, typeMismatchError(result.name(), operandType.name())
		}
	}

	return valueType{}, validationErrorf("coalescing requires a fallback")
}

func coalesceOperands(expression ast.Expression) []ast.Expression {
	infix, ok := expression.(ast.InfixExpression)
	if !ok || infix.Operator != lexer.TokenCoalesce {
		return []ast.Expression{expression}
	}

	operands := []ast.Expression{infix.Left}
	return append(operands, coalesceOperands(infix.Right)...)
}

func inferNumericBinary(operator lexer.TokenType, leftType, rightType valueType) (valueType, error) {
	if !leftType.isNumeric() || !rightType.isNumeric() {
		return valueType{}, validationErrorf("type mismatch: expected numeric operands for operator")
	}
	if isHexValueType(leftType) || isHexValueType(rightType) {
		if !isHexValueType(leftType) || !isHexValueType(rightType) {
			return valueType{}, validationErrorf("type mismatch: expected hexadecimal operands for operator")
		}
		if operator == lexer.TokenSlash {
			return valueType{kind: ValueHexFloat}, nil
		}
		if leftType.kind == ValueHexInt && rightType.kind == ValueHexInt {
			return valueType{kind: ValueHexInt}, nil
		}
		return valueType{kind: ValueHexFloat}, nil
	}
	if leftType.kind == ValueInt && rightType.kind == ValueInt {
		return valueType{kind: ValueInt}, nil
	}
	return valueType{kind: ValueFloat}, nil
}

func inferConditionalType(expr ast.ConditionalExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (valueType, error) {
	conditionType, err := inferExpressionType(expr.Condition, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}
	if conditionType.kind != ValueBoolean && (!conditionType.nullable || conditionType.kind != ValueRecord) {
		return valueType{}, validationErrorf("type mismatch: expected boolean condition")
	}

	thenVariables, elseVariables, err := conditionalVariables(expr.Condition, variables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}
	thenType, err := inferExpressionType(expr.Then, thenVariables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}
	elseType, err := inferExpressionType(expr.Else, elseVariables, symbols, types, schemas, enums)
	if err != nil {
		return valueType{}, err
	}

	if thenType.kind == ValueNull && !isUnknownValueType(elseType) {
		elseType.nullable = true
		return widenedValueType(elseType), nil
	}
	if elseType.kind == ValueNull && !isUnknownValueType(thenType) {
		thenType.nullable = true
		return widenedValueType(thenType), nil
	}

	if isUnknownValueType(thenType) {
		return elseType, nil
	}
	if isUnknownValueType(elseType) {
		return thenType, nil
	}

	return unifyConditionalTypes(thenType, elseType), nil
}

func isUnknownValueType(input valueType) bool {
	return input.kind == ValueUnknown && len(input.members) == 0 && len(input.choiceValues) == 0
}

func unifyConditionalTypes(thenType, elseType valueType) valueType {
	thenType = widenedValueType(thenType)
	elseType = widenedValueType(elseType)

	if thenType.kind == ValueRecord && elseType.kind == ValueRecord {
		if isEmptyRecordValueType(thenType) {
			return elseType
		}
		if isEmptyRecordValueType(elseType) {
			return thenType
		}
	}

	if typesEqual(thenType, elseType) {
		thenType.nullable = thenType.nullable || elseType.nullable
		thenType.presence = max(thenType.presence, elseType.presence)
		return thenType
	}

	members := flattenVariantValueTypes([]valueType{thenType, elseType})
	variantMembers := make([]valueType, 0, len(members))
	for _, member := range members {
		variantMembers = appendUniqueValueType(variantMembers, member)
	}
	if len(variantMembers) == 1 {
		return variantMembers[0]
	}

	return valueType{members: variantMembers}
}

func isEmptyRecordValueType(input valueType) bool {
	return input.schemaName == "" && input.record == nil && input.element == nil
}

func widenedValueType(input valueType) valueType {
	widened := input
	widened.exactValue = nil
	if widened.element != nil {
		element := widenedValueType(*widened.element)
		widened.element = &element
	}
	if len(widened.members) > 0 {
		members := make([]valueType, 0, len(widened.members))
		for _, member := range widened.members {
			members = append(members, widenedValueType(member))
		}
		widened.members = members
	}

	return widened
}

func conditionalVariables(condition ast.Expression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry, schemas *schemaRegistry, enums any) (*variableRegistry, *variableRegistry, error) {
	identifier, ok := condition.(ast.Identifier)
	if !ok {
		return variables, variables, nil
	}
	variableType, ok := variables.Get(identifier.Name)
	if !ok || !variableType.nullable {
		return variables, variables, nil
	}

	narrowed := variables.Clone()
	variableType.nullable = false
	narrowed.Add(identifier.Name, variableType)
	return narrowed, variables, nil
}

func stableExpressionPath(expression ast.Expression) (string, bool) {
	switch expr := expression.(type) {
	case ast.Identifier:
		return expr.Name, true
	case ast.MemberAccess:
		target, stable := stableExpressionPath(expr.Target)
		if !stable {
			return "", false
		}
		return target + "." + expr.Name, true
	default:
		return "", false
	}
}

func typesEqual(leftType, rightType valueType) bool {
	if len(leftType.choiceValues) > 0 || len(rightType.choiceValues) > 0 {
		return choiceValuesEqual(leftType.choiceValues, rightType.choiceValues)
	}
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
		if leftType.element != nil || rightType.element != nil {
			if leftType.element == nil || rightType.element == nil {
				return false
			}
			return typesEqual(*leftType.element, *rightType.element)
		}
		if leftType.schemaName == "" || rightType.schemaName == "" {
			return true
		}
		return leftType.schemaName == rightType.schemaName
	}
	return true
}

func recordMapTypesMatch(expectedType valueType, actualType valueType) bool {
	if expectedType.kind != ValueRecord || actualType.kind != ValueRecord {
		return true
	}
	if expectedType.element == nil {
		return true
	}
	if actualType.element == nil {
		return actualType.schemaName == "" && actualType.record == nil
	}
	return typesEqual(expectedType, actualType)
}

func ensureAssignable(expectedType, actualType valueType) error {
	if actualType.presence == presencePossiblyAbsent {
		return possiblyAbsentValueError()
	}
	if actualType.nullable && !expectedType.nullable {
		return invalidNullUsageError()
	}
	if actualType.kind == ValueNull {
		if expectedType.nullable {
			return nil
		}
		return invalidNullUsageError()
	}
	if actualType.nullable && !expectedType.nullable {
		return invalidNullUsageError()
	}
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
		return typeMismatchError(expectedType.name(), actualType.name())
	}
	if len(expectedType.choiceValues) > 0 {
		if len(actualType.choiceValues) > 0 {
			for _, actualValue := range actualType.choiceValues {
				if !choiceContainsValue(expectedType.choiceValues, actualValue) {
					return typeMismatchError(expectedType.name(), actualType.name())
				}
			}
			return nil
		}
		if actualType.exactValue != nil {
			if choiceContainsValue(expectedType.choiceValues, *actualType.exactValue) {
				return nil
			}
			return typeMismatchError(expectedType.name(), scalarValueDisplay(*actualType.exactValue))
		}
		for _, expectedValue := range expectedType.choiceValues {
			if expectedValue.Kind == actualType.kind {
				return nil
			}
		}
		return typeMismatchError(expectedType.name(), actualType.name())
	}
	if len(actualType.members) > 0 || len(actualType.choiceValues) > 0 {
		return typeMismatchError(expectedType.name(), actualType.name())
	}
	if expectedType.kind == ValueUnknown {
		return nil
	}
	if actualType.kind == ValueUnknown {
		return validationErrorf("type mismatch: expected %s, got unknown", expectedType.name())
	}
	if expectedType.kind != actualType.kind {
		return typeMismatchError(expectedType.name(), actualType.name())
	}
	if expectedType.kind == ValueRecord {
		if expectedType.schemaName != "" && actualType.schemaName != "" && expectedType.schemaName != actualType.schemaName {
			return typeMismatchError(expectedType.name(), actualType.name())
		}
	}
	if expectedType.kind == ValueArray {
		if expectedType.element == nil || actualType.element == nil {
			return typeMismatchError(expectedType.name(), actualType.name())
		}
		if actualType.element.kind == ValueUnknown && len(actualType.element.members) == 0 && len(actualType.element.choiceValues) == 0 {
			return nil
		}
		if err := ensureAssignable(*expectedType.element, *actualType.element); err != nil {
			return typeMismatchError(expectedType.name(), actualType.name())
		}
		return nil
	}
	return nil
}

func appendUniqueValueType(types []valueType, next valueType) []valueType {
	for index, existing := range types {
		if merged, ok := mergeInferredValueTypes(existing, next); ok {
			types[index] = merged
			return types
		}
	}

	return append(types, next)
}

func mergeInferredValueTypes(left valueType, right valueType) (valueType, bool) {
	if typesEqual(left, right) {
		return left, true
	}
	if isUnknownInferredValueType(left) {
		return right, true
	}
	if isUnknownInferredValueType(right) {
		return left, true
	}
	if left.kind != ValueArray || right.kind != ValueArray || left.element == nil || right.element == nil {
		return valueType{}, false
	}

	element, ok := mergeInferredValueTypes(*left.element, *right.element)
	if !ok {
		return valueType{}, false
	}

	return valueType{kind: ValueArray, element: &element}, true
}

func isUnknownInferredValueType(value valueType) bool {
	return value.kind == ValueUnknown && len(value.members) == 0 && len(value.choiceValues) == 0
}

type symbolKind int

const (
	symbolKindImport symbolKind = iota
	symbolKindType
	symbolKindSchema
	symbolKindVariable
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
