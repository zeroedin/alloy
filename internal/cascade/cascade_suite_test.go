package cascade_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCascade(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cascade Suite")
}
