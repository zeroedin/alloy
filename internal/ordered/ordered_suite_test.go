package ordered_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestOrdered(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ordered Suite")
}
