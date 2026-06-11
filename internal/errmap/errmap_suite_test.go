package errmap_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestErrmap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Errmap Suite")
}
