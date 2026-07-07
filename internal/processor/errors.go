package processor

import (
	"fmt"
	"strings"

	"github.com/louiss0/mace/internal/diagnostic"
)

type ErrorKind string

const (
	ErrorLexical     ErrorKind = "lexical"
	ErrorSyntax      ErrorKind = "syntax"
	ErrorDoc         ErrorKind = "doc"
	ErrorImport      ErrorKind = "import"
	ErrorDirective   ErrorKind = "directive"
	ErrorDeclaration ErrorKind = "declaration"
	ErrorType        ErrorKind = "type"
	ErrorValue       ErrorKind = "value"
	ErrorOperator    ErrorKind = "operator"
	ErrorSchema      ErrorKind = "schema"
	ErrorRuntime     ErrorKind = "runtime"
	ErrorInternal    ErrorKind = "internal"
)

type ErrorCode string

const (
	CodeArrayIndexOutOfRange         ErrorCode = "mace.type.invalid-array-access"
	CodeArrayValueRequired           ErrorCode = "mace.type.invalid-array-access"
	CodeImportFileFailedParse        ErrorCode = "mace.import.file-failed-to-parse"
	CodeImportFileNotFound           ErrorCode = "mace.import.file-not-found"
	CodeInternal                     ErrorCode = "mace.internal"
	CodeInvalidNullUsage             ErrorCode = "mace.type.invalid-null-usage"
	CodeInvalidOutputSchemaField     ErrorCode = "mace.type.invalid-output-schema-field"
	CodeMissingRequiredField         ErrorCode = "mace.type.record-does-not-match-schema"
	CodeOptionalFieldAccess          ErrorCode = "mace.type.optional-field-access"
	CodeOutputValueDeclaration       ErrorCode = "mace.type.unknown-identifier"
	CodeSelfReferenceUnknown         ErrorCode = "mace.type.unknown-self-field"
	CodeTypeMismatch                 ErrorCode = "mace.type.initializer-type-mismatch"
	CodeTypeRecordDoesNotMatchSchema ErrorCode = "mace.type.record-does-not-match-schema"
)

type DiagnosticFields struct {
	Name     string
	Field    string
	Schema   string
	Expected string
	Actual   string
	Index    string
	Level    int
	Operator string
	Path     string
	Details  map[string]string
}

type DiagnosticError struct {
	Kind    ErrorKind
	Code    ErrorCode
	Message string
	Range   diagnostic.Range
	Fields  DiagnosticFields
	Cause   error
}

func (err DiagnosticError) Error() string {
	return err.Message
}

func (err DiagnosticError) Unwrap() error {
	return err.Cause
}

func diagnosticErrorf(kind ErrorKind, code ErrorCode, fields DiagnosticFields, format string, args ...any) error {
	return DiagnosticError{
		Kind:    kind,
		Code:    code,
		Message: fmt.Sprintf("processor: %s", fmt.Sprintf(format, args...)),
		Fields:  fields,
	}
}

func typeMismatchError(expected string, actual string) error {
	return diagnosticErrorf(
		ErrorType,
		CodeTypeMismatch,
		DiagnosticFields{Expected: expected, Actual: actual},
		"type mismatch: expected %s, got %s",
		expected,
		actual,
	)
}

func invalidNullUsageError() error {
	return diagnosticErrorf(
		ErrorType,
		CodeInvalidNullUsage,
		DiagnosticFields{},
		"null can only be assigned to nullable variables and optional schema fields",
	)
}

func missingRequiredFieldError(field string, schema string) error {
	fields := DiagnosticFields{Field: field, Schema: schema}
	if schema == "" {
		return diagnosticErrorf(ErrorSchema, CodeMissingRequiredField, fields, "missing required field %q", field)
	}

	return diagnosticErrorf(ErrorSchema, CodeMissingRequiredField, fields, "missing required field %q for schema %q", field, schema)
}

func validationErrorf(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	return diagnosticErrorf(inferErrorKind(message), inferErrorCode(message), DiagnosticFields{}, "%s", message)
}

func inferErrorCode(message string) ErrorCode {
	switch {
	case strings.Contains(message, "duplicate output directive"):
		return ErrorCode("mace.directive.duplicate-key")
	case strings.Contains(message, "unknown output directive"):
		return ErrorCode("mace.directive.unknown-key")
	case strings.Contains(message, "schema directive is invalid when output mode is schema") || strings.Contains(message, "schema_file directive is invalid when output mode is schema"):
		return ErrorCode("mace.directive.output-schema-combined")
	case strings.Contains(message, "parse and parse_file directives cannot be used together"):
		return ErrorCode("mace.directive.schema-file-invalid")
	case strings.Contains(message, "unknown schema "):
		return ErrorCode("mace.directive.unknown-schema-name")
	case strings.Contains(message, "unable to read import file"):
		return CodeImportFileNotFound
	case strings.Contains(message, "unable to parse import file"):
		return CodeImportFileFailedParse
	case strings.Contains(message, "circular import"):
		return ErrorCode("mace.import.circular")
	case strings.Contains(message, "duplicate import"):
		return ErrorCode("mace.import.duplicate-name")
	case strings.Contains(message, "imported identifier"):
		return ErrorCode("mace.import.name-not-exposed")
	case strings.Contains(message, "import path") && strings.Contains(message, "escapes root"):
		return ErrorCode("mace.import.path-outside-root")
	case strings.Contains(message, "import path") && strings.Contains(message, "must end in .mace"):
		return ErrorCode("mace.import.invalid-extension")
	case strings.Contains(message, "unknown type ") || strings.Contains(message, "unknown type reference"):
		return ErrorCode("mace.declaration.unknown-type-reference")
	case strings.Contains(message, "requires an initializer") || strings.Contains(message, "requires a runtime value"):
		return ErrorCode("mace.declaration.variable-missing-initializer")
	case strings.Contains(message, "duplicate declaration"):
		return ErrorCode("mace.declaration.duplicate-variable")
	case strings.Contains(message, "duplicate field"):
		return ErrorCode("mace.declaration.duplicate-schema-field")
	case strings.Contains(message, "duplicate output field"):
		return ErrorCode("mace.declaration.duplicate-output-field")
	case strings.Contains(message, "array literal has mixed element types"):
		return ErrorCode("mace.type.mixed-array-literal")
	case strings.Contains(message, "array access requires an array value") || strings.Contains(message, "array index ") && strings.Contains(message, "out of range"):
		return CodeArrayIndexOutOfRange
	case strings.Contains(message, "unknown identifier"):
		return ErrorCode("mace.type.unknown-identifier")
	case strings.Contains(message, "unknown self reference"):
		return CodeSelfReferenceUnknown
	case strings.Contains(message, "invalid field type") && strings.Contains(message, "output = schema"):
		return CodeInvalidOutputSchemaField
	case strings.Contains(message, "expected boolean after '!'") || strings.Contains(message, "expected int after '~'") || strings.Contains(message, "expected numeric after unary operator"):
		return ErrorCode("mace.type.invalid-unary-operator")
	case strings.Contains(message, "expected numeric operands") || strings.Contains(message, "expected int operands") || strings.Contains(message, "expected boolean operands") || strings.Contains(message, "incompatible equality comparison") || strings.Contains(message, "merge operands") || strings.Contains(message, "expected ") && strings.Contains(message, " operands"):
		return ErrorCode("mace.type.invalid-binary-operator")
	case strings.Contains(message, "null can only be assigned to nullable variables and optional schema fields"):
		return CodeInvalidNullUsage
	case strings.Contains(message, "type mismatch"):
		return CodeTypeMismatch
	case strings.Contains(message, "missing required field") || strings.Contains(message, "unknown field") || strings.Contains(message, "is not optional in schema"):
		return CodeTypeRecordDoesNotMatchSchema
	default:
		return CodeInternal
	}
}

func inferErrorKind(message string) ErrorKind {
	switch {
	case strings.Contains(message, "documentation") || strings.Contains(message, "doc blocks") || strings.Contains(message, "_doc target"):
		return ErrorDoc
	case strings.Contains(message, "import"):
		return ErrorImport
	case strings.Contains(message, "directive"):
		return ErrorDirective
	case strings.Contains(message, "declaration") || strings.Contains(message, "type alias"):
		return ErrorDeclaration
	case strings.Contains(message, "operator") || strings.Contains(message, "operands") || strings.Contains(message, "division by zero") || strings.Contains(message, "exponent") || strings.Contains(message, "shift count"):
		return ErrorOperator
	case strings.Contains(message, "type mismatch") || strings.Contains(message, "unknown type") || strings.Contains(message, "type reference"):
		return ErrorType
	case strings.Contains(message, "schema") || strings.Contains(message, "field"):
		return ErrorSchema
	case strings.Contains(message, "runtime"):
		return ErrorRuntime
	case strings.Contains(message, "value") || strings.Contains(message, "literal") || strings.Contains(message, "expression") || strings.Contains(message, "self reference") || strings.Contains(message, "member access") || strings.Contains(message, "array access"):
		return ErrorValue
	default:
		return ErrorInternal
	}
}
