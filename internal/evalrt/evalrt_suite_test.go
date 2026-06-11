package evalrt

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Tests live in the evalrt package itself (not evalrt_test) because the
// runtime identifiers are intentionally unexported. Test files must not match
// the rt_*.go embed pattern, so they drop the rt_ prefix.

func TestEvalrt(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Evalrt Suite")
}
