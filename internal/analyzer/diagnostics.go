package analyzer

import (
	"strings"

	protocol "github.com/tliron/glsp/protocol_3_16"

	"github.com/louiss0/mace/internal/parser/ast"
	"github.com/louiss0/mace/internal/processor"
)

type diagnosticCode string

const (
	diagnosticSyntaxUnterminatedScriptBlock         diagnosticCode = "mace.syntax.unterminated-script-block"
	diagnosticSyntaxInconsistentScriptDelimiters    diagnosticCode = "mace.syntax.inconsistent-script-delimiters"
	diagnosticSyntaxEmptyScriptBlock                diagnosticCode = "mace.syntax.empty-script-block"
	diagnosticSyntaxMalformedImport                 diagnosticCode = "mace.syntax.malformed-import"
	diagnosticSyntaxMalformedDirectiveList          diagnosticCode = "mace.syntax.malformed-directive-list"
	diagnosticSyntaxMalformedSchema                 diagnosticCode = "mace.syntax.malformed-schema"
	diagnosticSyntaxMalformedVariableDeclaration    diagnosticCode = "mace.syntax.malformed-variable-declaration"
	diagnosticSyntaxMalformedOutputField            diagnosticCode = "mace.syntax.malformed-output-field"
	diagnosticSyntaxInvalidArrayAccessIndex         diagnosticCode = "mace.syntax.invalid-array-access-index"
	diagnosticSyntaxUnexpectedToken                 diagnosticCode = "mace.syntax.unexpected-token"
	diagnosticFileImportAfterScript                 diagnosticCode = "mace.file-structure.import-after-script-block"
	diagnosticFileImportAfterOutput                 diagnosticCode = "mace.file-structure.import-after-output-block"
	diagnosticFileScriptAfterOutput                 diagnosticCode = "mace.file-structure.script-after-output-block"
	diagnosticFileMissingOutputBlock                diagnosticCode = "mace.file-structure.missing-output-block"
	diagnosticFileMultipleOutputBlocks              diagnosticCode = "mace.file-structure.multiple-output-blocks"
	diagnosticFileDirectiveNotAttached              diagnosticCode = "mace.file-structure.directive-not-attached-to-output-block"
	diagnosticImportPathMissing                     diagnosticCode = "mace.import.path-missing"
	diagnosticImportPathNotString                   diagnosticCode = "mace.import.path-not-string-literal"
	diagnosticImportPathNotMace                     diagnosticCode = "mace.import.path-not-mace"
	diagnosticImportFileNotFound                    diagnosticCode = "mace.import.file-not-found"
	diagnosticImportFileFailedParse                 diagnosticCode = "mace.import.file-failed-to-parse"
	diagnosticImportCircular                        diagnosticCode = "mace.import.circular"
	diagnosticImportDuplicateName                   diagnosticCode = "mace.import.duplicate-name"
	diagnosticImportNameNotExposed                  diagnosticCode = "mace.import.name-not-exposed"
	diagnosticImportInternalDeclaration             diagnosticCode = "mace.import.internal-declaration"
	diagnosticImportTargetNotPublic                 diagnosticCode = "mace.import.target-not-public"
	diagnosticImportUnused                          diagnosticCode = "mace.import.unused"
	diagnosticDeclarationDuplicateType              diagnosticCode = "mace.declaration.duplicate-type"
	diagnosticDeclarationDuplicateSchema            diagnosticCode = "mace.declaration.duplicate-schema"
	diagnosticDeclarationDuplicateVariable          diagnosticCode = "mace.declaration.duplicate-variable"
	diagnosticDeclarationDuplicateSchemaField       diagnosticCode = "mace.declaration.duplicate-schema-field"
	diagnosticDeclarationDuplicateOutputField       diagnosticCode = "mace.declaration.duplicate-output-field"
	diagnosticDeclarationUnknownTypeReference       diagnosticCode = "mace.declaration.unknown-type-reference"
	diagnosticDeclarationVariableMissingType        diagnosticCode = "mace.declaration.variable-missing-type"
	diagnosticDeclarationVariableMissingInitializer diagnosticCode = "mace.declaration.variable-missing-initializer"
	diagnosticDeclarationUnusedVariable             diagnosticCode = "mace.declaration.unused-variable"
	diagnosticDeclarationUnusedType                 diagnosticCode = "mace.declaration.unused-type"
	diagnosticTypeInitializerMismatch               diagnosticCode = "mace.type.initializer-type-mismatch"
	diagnosticTypeInvalidUnaryOperator              diagnosticCode = "mace.type.invalid-unary-operator"
	diagnosticTypeInvalidBinaryOperator             diagnosticCode = "mace.type.invalid-binary-operator"
	diagnosticTypeMixedArrayLiteral                 diagnosticCode = "mace.type.mixed-array-literal"
	diagnosticTypeInvalidArrayAccess                diagnosticCode = "mace.type.invalid-array-access"
	diagnosticTypeUnknownIdentifier                 diagnosticCode = "mace.type.unknown-identifier"
	diagnosticTypeUnknownSelfField                  diagnosticCode = "mace.type.unknown-self-field"
	diagnosticTypeSelfForwardReference              diagnosticCode = "mace.type.self-forward-reference"
	diagnosticTypeRecordDoesNotMatchSchema          diagnosticCode = "mace.type.record-does-not-match-schema"
	diagnosticTypeOutputValueIncompatibleSchema     diagnosticCode = "mace.type.output-value-incompatible-schema"
	diagnosticTypeInvalidOutputSchemaField          diagnosticCode = "mace.type.invalid-output-schema-field"
	diagnosticTypeInvalidNullUsage                  diagnosticCode = "mace.type.invalid-null-usage"
	diagnosticDirectiveDuplicateKey                 diagnosticCode = "mace.directive.duplicate-key"
	diagnosticDirectiveUnknownKey                   diagnosticCode = "mace.directive.unknown-key"
	diagnosticDirectiveInvalidOutputValue           diagnosticCode = "mace.directive.invalid-output-value"
	diagnosticDirectiveOutputSchemaCombined         diagnosticCode = "mace.directive.output-schema-combined"
	diagnosticDirectiveSchemaAndSchemaFileCombined  diagnosticCode = "mace.directive.schema-and-schema-file-combined"
	diagnosticDirectiveSchemaOutputVariableIgnored  diagnosticCode = "mace.directive.schema-output-variable-ignored"
	diagnosticDirectiveUnknownSchemaName            diagnosticCode = "mace.directive.unknown-schema-name"
	diagnosticDirectiveSchemaFileInvalid            diagnosticCode = "mace.directive.schema-file-invalid"
	diagnosticDirectiveSchemaFileUnusable           diagnosticCode = "mace.directive.schema-file-unusable"
)

func diagnosticCodeValue(code diagnosticCode) *protocol.IntegerOrString {
	return Ptr(protocol.IntegerOrString{Value: string(code)})
}

func diagnosticWithCode(rangeValue protocol.Range, severity protocol.DiagnosticSeverity, code diagnosticCode, message string) protocol.Diagnostic {
	return protocol.Diagnostic{
		Range:    rangeValue,
		Severity: Ptr(severity),
		Code:     diagnosticCodeValue(code),
		Source:   Ptr(serverName),
		Message:  message,
	}
}

func classifyParseDiagnostic(message string) diagnosticCode {
	switch {
	case strings.Contains(message, "empty script block"):
		return diagnosticSyntaxEmptyScriptBlock
	case strings.Contains(message, "expected closing script delimiter") && strings.Contains(message, "EOF"):
		return diagnosticSyntaxUnterminatedScriptBlock
	case strings.Contains(message, "script delimiter"):
		return diagnosticSyntaxInconsistentScriptDelimiters
	case strings.Contains(message, "import"):
		return diagnosticSyntaxMalformedImport
	case strings.Contains(message, "directive"):
		return diagnosticSyntaxMalformedDirectiveList
	case strings.Contains(message, "schema declaration") || strings.Contains(message, "record type") || strings.Contains(message, "schema field"):
		return diagnosticSyntaxMalformedSchema
	case strings.Contains(message, "integer index in array access") || strings.Contains(message, "after array access index"):
		return diagnosticSyntaxInvalidArrayAccessIndex
	case strings.Contains(message, "not allowed when output = schema"):
		return diagnosticDirectiveSchemaOutputVariableIgnored
	case strings.Contains(message, "variable declaration"):
		return diagnosticSyntaxMalformedVariableDeclaration
	case strings.Contains(message, "output field"):
		return diagnosticSyntaxMalformedOutputField
	default:
		return diagnosticSyntaxUnexpectedToken
	}
}

func classifyProcessorDiagnostic(message string) diagnosticCode {
	switch {
	case strings.Contains(message, "duplicate output directive"):
		return diagnosticDirectiveDuplicateKey
	case strings.Contains(message, "duplicate documentation declaration"):
		return diagnosticSyntaxUnexpectedToken
	case strings.Contains(message, "unknown output directive"):
		return diagnosticDirectiveUnknownKey
	case strings.Contains(message, "schema directive is invalid when output mode is schema") || strings.Contains(message, "schema_file directive is invalid when output mode is schema"):
		return diagnosticDirectiveOutputSchemaCombined
	case strings.Contains(message, "unknown schema "):
		return diagnosticDirectiveUnknownSchemaName
	case strings.Contains(message, "unable to read import file"):
		return diagnosticImportFileNotFound
	case strings.Contains(message, "unable to parse import file"):
		return diagnosticImportFileFailedParse
	case strings.Contains(message, "circular import"):
		return diagnosticImportCircular
	case strings.Contains(message, "duplicate import"):
		return diagnosticImportDuplicateName
	case strings.Contains(message, "imported identifier"):
		return diagnosticImportNameNotExposed
	case strings.Contains(message, "schema_doc target") || strings.Contains(message, "gen_doc target") || strings.Contains(message, "documentation target"):
		return diagnosticSyntaxUnexpectedToken
	case strings.Contains(message, "unknown type ") || strings.Contains(message, "unknown type reference"):
		return diagnosticDeclarationUnknownTypeReference
	case strings.Contains(message, "not allowed when output = schema"):
		return diagnosticDirectiveSchemaOutputVariableIgnored
	case strings.Contains(message, "requires an initializer") || strings.Contains(message, "requires a runtime value"):
		return diagnosticDeclarationVariableMissingInitializer
	case strings.Contains(message, "duplicate declaration"):
		return diagnosticDeclarationDuplicateVariable
	case strings.Contains(message, "already documented by a documentation declaration"):
		return diagnosticSyntaxUnexpectedToken
	case strings.Contains(message, "duplicate field"):
		return diagnosticDeclarationDuplicateSchemaField
	case strings.Contains(message, "duplicate output field"):
		return diagnosticDeclarationDuplicateOutputField
	case strings.Contains(message, "array literal has mixed element types"):
		return diagnosticTypeMixedArrayLiteral
	case strings.Contains(message, "array access requires an array value") || strings.Contains(message, "array index ") && strings.Contains(message, "out of range"):
		return diagnosticTypeInvalidArrayAccess
	case strings.Contains(message, "unknown identifier"):
		return diagnosticTypeUnknownIdentifier
	case strings.Contains(message, "unknown self reference"):
		return diagnosticTypeUnknownSelfField
	case strings.Contains(message, "invalid field type") && strings.Contains(message, "output = schema"):
		return diagnosticTypeInvalidOutputSchemaField
	case strings.Contains(message, "cannot reference type or schema declaration"):
		return diagnosticTypeUnknownIdentifier
	case strings.Contains(message, "expected boolean after '!'") || strings.Contains(message, "expected int after '~'") || strings.Contains(message, "expected numeric after unary operator"):
		return diagnosticTypeInvalidUnaryOperator
	case strings.Contains(message, "expected numeric operands") || strings.Contains(message, "expected int operands") || strings.Contains(message, "expected boolean operands") || strings.Contains(message, "incompatible equality comparison") || strings.Contains(message, "merge operands") || strings.Contains(message, "expected ") && strings.Contains(message, " operands"):
		return diagnosticTypeInvalidBinaryOperator
	case strings.Contains(message, "null can only be assigned to nullable variables and optional schema fields"):
		return diagnosticTypeInvalidNullUsage
	case strings.Contains(message, "type mismatch"):
		return diagnosticTypeInitializerMismatch
	case strings.Contains(message, "missing required field") || strings.Contains(message, "unknown field") || strings.Contains(message, "is not optional in schema"):
		return diagnosticTypeRecordDoesNotMatchSchema
	default:
		return diagnosticSyntaxUnexpectedToken
	}
}

func classifyDiagnosticCode(message string) diagnosticCode {
	switch {
	case strings.HasPrefix(message, "parser:"):
		return classifyParseDiagnostic(message)
	case strings.HasPrefix(message, "processor:"):
		return classifyProcessorDiagnostic(message)
	default:
		return diagnosticSyntaxUnexpectedToken
	}
}

func diagnosticCodeFromProcessorError(err processor.DiagnosticError) diagnosticCode {
	switch err.Code {
	case processor.CodeArrayIndexOutOfRange, processor.CodeArrayValueRequired:
		return diagnosticTypeInvalidArrayAccess
	case processor.CodeInvalidNullUsage:
		return diagnosticTypeInvalidNullUsage
	case processor.CodeInvalidOutputSchemaField:
		return diagnosticTypeInvalidOutputSchemaField
	case processor.CodeMissingRequiredField:
		return diagnosticTypeRecordDoesNotMatchSchema
	case processor.CodeOutputValueDeclaration:
		return diagnosticTypeUnknownIdentifier
	case processor.CodeSelfReferenceUnknown:
		return diagnosticTypeUnknownSelfField
	case processor.CodeTypeMismatch:
		return diagnosticTypeInitializerMismatch
	}

	switch err.Kind {
	case processor.ErrorImport:
		return diagnosticImportFileFailedParse
	case processor.ErrorDirective:
		return diagnosticDirectiveUnknownKey
	case processor.ErrorDeclaration:
		return diagnosticDeclarationDuplicateVariable
	case processor.ErrorType:
		return diagnosticTypeInitializerMismatch
	case processor.ErrorValue:
		return diagnosticTypeUnknownIdentifier
	case processor.ErrorOperator:
		return diagnosticTypeInvalidBinaryOperator
	case processor.ErrorSchema:
		return diagnosticTypeRecordDoesNotMatchSchema
	default:
		return classifyProcessorDiagnostic(err.Message)
	}
}

func classifySelfReferenceCode(file ast.File, name string) diagnosticCode {
	if outputFieldIndex(file, name) >= 0 {
		return diagnosticTypeSelfForwardReference
	}

	return diagnosticTypeUnknownSelfField
}

func outputFieldIndex(file ast.File, name string) int {
	for index, field := range file.Output.DataFields {
		if field.Name == name {
			return index
		}
	}

	return -1
}
