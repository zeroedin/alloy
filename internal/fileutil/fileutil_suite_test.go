package fileutil_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFileutil(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fileutil Suite")
}
