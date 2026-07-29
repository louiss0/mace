package diagnostic

import (
	"errors"
	"fmt"
	"testing"

	"github.com/louiss0/mace/internal/parser/ast"
	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestDiagnostic(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Diagnostic Suite")
}

var _ = Describe("Diagnostics", func() {
	It("preserves diagnostic messages and causes", func() {
		cause := errors.New("source failure")
		diagnosticError := Error{Message: "invalid source", Cause: cause}

		tAssert.Equal("invalid source", diagnosticError.Error())
		tAssert.ErrorIs(diagnosticError, cause)
	})

	It("extracts wrapped diagnostic errors", func() {
		expected := Error{Code: "mace.test", Message: "test diagnostic"}
		actual, ok := As(fmt.Errorf("processing failed: %w", expected))

		tAssert.True(ok)
		tAssert.Equal(expected, actual)

		actual, ok = As(errors.New("ordinary error"))
		tAssert.False(ok)
		tAssert.Empty(actual)
	})

	It("converts AST ranges without changing positions", func() {
		source := ast.SourceRange{
			Start: ast.SourcePosition{Line: 2, Column: 3},
			End:   ast.SourcePosition{Line: 4, Column: 5},
		}

		tAssert.Equal(Range{
			Start: Position{Line: 2, Column: 3},
			End:   Position{Line: 4, Column: 5},
		}, FromASTRange(source))
	})
})
