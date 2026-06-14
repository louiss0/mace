package processor

import (
	"fmt"
	"strings"
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
	CodeArrayIndexOutOfRange     ErrorCode = "array-index-out-of-range"
	CodeArrayValueRequired       ErrorCode = "array-value-required"
	CodeImportFileFailedParse    ErrorCode = "import-file-failed-parse"
	CodeImportFileNotFound       ErrorCode = "import-file-not-found"
	CodeInternal                 ErrorCode = "internal"
	CodeInvalidNullUsage         ErrorCode = "invalid-null-usage"
	CodeInvalidOutputSchemaField ErrorCode = "invalid-output-schema-field"
	CodeMissingInjectable        ErrorCode = "missing-injectable"
	CodeMissingRequiredField     ErrorCode = "missing-required-field"
	CodeOutputValueDeclaration   ErrorCode = "output-value-declaration"
	CodeOptionalFieldAccess      ErrorCode = "optional-field-access"
	CodeSelfReferenceUnknown     ErrorCode = "self-reference-unknown"
	CodeTypeMismatch             ErrorCode = "type-mismatch"
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
	return diagnosticErrorf(inferErrorKind(message), CodeInternal, DiagnosticFields{}, "%s", message)
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
	case strings.Contains(message, "runtime") || strings.Contains(message, "injection") || strings.Contains(message, "injectable"):
		return ErrorRuntime
	case strings.Contains(message, "value") || strings.Contains(message, "literal") || strings.Contains(message, "expression") || strings.Contains(message, "self reference") || strings.Contains(message, "member access") || strings.Contains(message, "array access"):
		return ErrorValue
	default:
		return ErrorInternal
	}
}
