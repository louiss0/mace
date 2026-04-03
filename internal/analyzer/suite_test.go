package analyzer

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

var tAssert *assert.Assertions

func TestAnalyzer(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Analyzer Suite")
}
