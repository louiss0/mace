package processor

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	"github.com/stretchr/testify/assert"
)

func TestProcessor(t *testing.T) {
	tAssert = assert.New(t)
	RunSpecs(t, "Processor Suite")
}
