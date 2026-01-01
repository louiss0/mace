package processor

import (
	"fmt"

	"github.com/louiss0/mace/lexer"
	"github.com/louiss0/mace/parser"
	"github.com/louiss0/mace/parser/ast"
)

type Processor struct{}

type Result struct {
	File ast.File
}

func New() *Processor {
	return &Processor{}
}

func (p *Processor) Process(input string) (Result, error) {
	tokens, err := lex(input)
	if err != nil {
		return Result{}, err
	}

	file, err := parser.New(tokens).ParseFile()
	if err != nil {
		return Result{}, err
	}

	if err := validateFile(file); err != nil {
		return Result{}, err
	}

	return Result{File: file}, nil
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

func validateFile(file ast.File) error {
	symbols := newSymbolTable()
	types := newTypeRegistry()
	variables := newVariableRegistry()

	for _, importDecl := range file.Imports {
		for _, name := range importDecl.Identifiers {
			if symbols.Has(name) {
				return validationErrorf("duplicate import %q", name)
			}
			symbols.Add(name, symbolKindImport)
		}
	}

	if file.Script != nil {
		if err := collectDeclarations(file.Script.Items, symbols, types); err != nil {
			return err
		}
		if err := validateDeclarations(file.Script.Items, symbols, types, variables); err != nil {
			return err
		}
	}

	if err := validateOutputDirectives(file.Output.Directives, symbols); err != nil {
		return err
	}

	return nil
}

func collectDeclarations(items []ast.Declaration, symbols *symbolTable, types *typeRegistry) error {
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
		default:
			return validationErrorf("unknown declaration")
		}
	}

	return nil
}

func validateDeclarations(items []ast.Declaration, symbols *symbolTable, types *typeRegistry, variables *variableRegistry) error {
	for _, declaration := range items {
		if err := validateDeclaration(declaration, symbols, types, variables); err != nil {
			return err
		}
	}

	return nil
}

func validateDeclaration(declaration ast.Declaration, symbols *symbolTable, types *typeRegistry, variables *variableRegistry) error {
	switch decl := declaration.(type) {
	case ast.VariableDeclaration:
		if err := validateTypeReference(decl.Type, symbols, types); err != nil {
			return err
		}
		expectedType, err := resolveValueType(decl.Type, symbols, types)
		if err != nil {
			return err
		}
		actualType, err := inferExpressionType(decl.Value, variables, symbols, types)
		if err != nil {
			return err
		}
		if err := ensureAssignable(expectedType, actualType); err != nil {
			return err
		}
		variables.Add(decl.Name, expectedType)
		return nil
	case ast.TypeDeclaration:
		return validateTypeReference(decl.Type, symbols, types)
	case ast.SchemaDeclaration:
		return validateRecordType(decl.Type, symbols, types)
	default:
		return validationErrorf("unknown declaration")
	}
}

func validateTypeReference(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry) error {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return nil
	case ast.ArrayType:
		return validateTypeReference(ref.Element, symbols, types)
	case ast.NamedType:
		if symbols.IsType(ref.Name) {
			_, _, err := types.Resolve(ref.Name)
			return err
		}
		if !symbols.IsSchema(ref.Name) && !symbols.IsImport(ref.Name) {
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

func validateOutputDirectives(directives []ast.OutputDirective, symbols *symbolTable) error {
	var outputSet bool
	seenKinds := map[ast.OutputDirectiveKind]struct{}{}

	for _, directive := range directives {
		if _, exists := seenKinds[directive.Kind]; exists {
			return validationErrorf("duplicate output directive %q", directiveKindName(directive.Kind))
		}
		seenKinds[directive.Kind] = struct{}{}

		switch directive.Kind {
		case ast.OutputDirectiveOutput:
			outputSet = true
		case ast.OutputDirectiveSchema:
			if !symbols.IsSchema(directive.Value) && !symbols.IsImport(directive.Value) {
				return validationErrorf("unknown schema %q", directive.Value)
			}
		case ast.OutputDirectiveSchemaFile:
		default:
			return validationErrorf("unknown output directive")
		}
	}

	if !outputSet {
		return validationErrorf("missing output directive")
	}

	return nil
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

type valueKind int

const (
	valueUnknown valueKind = iota
	valueString
	valueInt
	valueFloat
	valueBoolean
	valueArray
	valueRecord
)

type valueType struct {
	kind    valueKind
	element *valueType
}

func (t valueType) isNumeric() bool {
	return t.kind == valueInt || t.kind == valueFloat
}

func (t valueType) name() string {
	switch t.kind {
	case valueString:
		return "string"
	case valueInt:
		return "int"
	case valueFloat:
		return "float"
	case valueBoolean:
		return "boolean"
	case valueArray:
		if t.element != nil {
			return fmt.Sprintf("array<%s>", t.element.name())
		}
		return "array"
	case valueRecord:
		return "record"
	default:
		return "unknown"
	}
}

func resolveValueType(typeRef ast.TypeReference, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	switch ref := typeRef.(type) {
	case ast.PrimitiveType:
		return primitiveValueType(ref.Name)
	case ast.ArrayType:
		element, err := resolveValueType(ref.Element, symbols, types)
		if err != nil {
			return valueType{}, err
		}
		return valueType{kind: valueArray, element: &element}, nil
	case ast.NamedType:
		resolved, resolvedByAlias, err := types.Resolve(ref.Name)
		if err != nil {
			return valueType{}, err
		}
		if resolvedByAlias {
			return resolveValueType(resolved, symbols, types)
		}
		if symbols.IsSchema(ref.Name) || symbols.IsImport(ref.Name) {
			return valueType{kind: valueRecord}, nil
		}
		return valueType{}, validationErrorf("unknown type %q", ref.Name)
	default:
		return valueType{}, validationErrorf("unknown type reference")
	}
}

func primitiveValueType(name string) (valueType, error) {
	switch name {
	case "string":
		return valueType{kind: valueString}, nil
	case "int":
		return valueType{kind: valueInt}, nil
	case "float":
		return valueType{kind: valueFloat}, nil
	case "boolean":
		return valueType{kind: valueBoolean}, nil
	default:
		return valueType{}, validationErrorf("unknown type %q", name)
	}
}

func inferExpressionType(expression ast.Expression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	switch expr := expression.(type) {
	case ast.Identifier:
		if variableType, ok := variables.Get(expr.Name); ok {
			return variableType, nil
		}
		return valueType{kind: valueUnknown}, nil
	case ast.IntLiteral:
		return valueType{kind: valueInt}, nil
	case ast.FloatLiteral:
		return valueType{kind: valueFloat}, nil
	case ast.StringLiteral:
		return valueType{kind: valueString}, nil
	case ast.BooleanLiteral:
		return valueType{kind: valueBoolean}, nil
	case ast.ArrayLiteral:
		return inferArrayLiteralType(expr, variables, symbols, types)
	case ast.RecordLiteral:
		return valueType{kind: valueRecord}, nil
	case ast.SelfReference:
		return valueType{kind: valueUnknown}, nil
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
		return valueType{kind: valueArray, element: &valueType{kind: valueUnknown}}, nil
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

	return valueType{kind: valueArray, element: &firstType}, nil
}

func inferPrefixType(expr ast.PrefixExpression, variables *variableRegistry, symbols *symbolTable, types *typeRegistry) (valueType, error) {
	rightType, err := inferExpressionType(expr.Right, variables, symbols, types)
	if err != nil {
		return valueType{}, err
	}

	switch expr.Operator {
	case lexer.TokenBang:
		if rightType.kind != valueBoolean {
			return valueType{}, validationErrorf("type mismatch: expected boolean after '!'")
		}
		return valueType{kind: valueBoolean}, nil
	case lexer.TokenTilde:
		if rightType.kind != valueInt {
			return valueType{}, validationErrorf("type mismatch: expected int after '~'")
		}
		return valueType{kind: valueInt}, nil
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
		if leftType.kind != valueInt || rightType.kind != valueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for '%%'")
		}
		return valueType{kind: valueInt}, nil
	case lexer.TokenShiftLeft, lexer.TokenShiftRight, lexer.TokenShiftRightUnsigned:
		if leftType.kind != valueInt || rightType.kind != valueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for shift")
		}
		return valueType{kind: valueInt}, nil
	case lexer.TokenAmpersand, lexer.TokenPipe, lexer.TokenCaret:
		if leftType.kind != valueInt || rightType.kind != valueInt {
			return valueType{}, validationErrorf("type mismatch: expected int operands for bitwise operator")
		}
		return valueType{kind: valueInt}, nil
	case lexer.TokenEqualEqual, lexer.TokenNotEqual, lexer.TokenStrictEqual, lexer.TokenStrictNotEqual:
		if leftType.kind != valueUnknown && rightType.kind != valueUnknown && leftType.kind != rightType.kind {
			return valueType{}, validationErrorf("type mismatch: incompatible equality comparison")
		}
		return valueType{kind: valueBoolean}, nil
	case lexer.TokenLess, lexer.TokenLessEqual, lexer.TokenGreater, lexer.TokenGreaterEqual:
		if !leftType.isNumeric() || !rightType.isNumeric() {
			return valueType{}, validationErrorf("type mismatch: expected numeric operands for comparison")
		}
		if leftType.kind != rightType.kind {
			return valueType{}, validationErrorf("type mismatch: expected %s operands", leftType.name())
		}
		return valueType{kind: valueBoolean}, nil
	case lexer.TokenAndAnd, lexer.TokenOrOr:
		if leftType.kind != valueBoolean || rightType.kind != valueBoolean {
			return valueType{}, validationErrorf("type mismatch: expected boolean operands for logical operator")
		}
		return valueType{kind: valueBoolean}, nil
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
	if conditionType.kind != valueBoolean {
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

	if thenType.kind != valueUnknown && elseType.kind != valueUnknown && thenType.kind != elseType.kind {
		return valueType{}, validationErrorf("type mismatch: conditional branches differ")
	}

	if thenType.kind == valueUnknown {
		return elseType, nil
	}

	return thenType, nil
}

func typesEqual(leftType, rightType valueType) bool {
	if leftType.kind != rightType.kind {
		return false
	}
	if leftType.kind == valueArray {
		if leftType.element == nil || rightType.element == nil {
			return false
		}
		return typesEqual(*leftType.element, *rightType.element)
	}
	return true
}

func ensureAssignable(expectedType, actualType valueType) error {
	if expectedType.kind == valueUnknown {
		return nil
	}
	if actualType.kind == valueUnknown {
		return validationErrorf("type mismatch: expected %s, got unknown", expectedType.name())
	}
	if expectedType.kind != actualType.kind {
		return validationErrorf("type mismatch: expected %s, got %s", expectedType.name(), actualType.name())
	}
	if expectedType.kind == valueArray {
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
)

type symbolTable struct {
	symbols map[string]symbolKind
}

func newSymbolTable() *symbolTable {
	return &symbolTable{
		symbols: map[string]symbolKind{},
	}
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

type typeRegistry struct {
	aliases map[string]ast.TypeReference
}

func newTypeRegistry() *typeRegistry {
	return &typeRegistry{
		aliases: map[string]ast.TypeReference{},
	}
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

type variableRegistry struct {
	variables map[string]valueType
}

func newVariableRegistry() *variableRegistry {
	return &variableRegistry{
		variables: map[string]valueType{},
	}
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
