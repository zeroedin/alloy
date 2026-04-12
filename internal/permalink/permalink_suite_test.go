package permalink_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPermalink(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Permalink Suite")
}
