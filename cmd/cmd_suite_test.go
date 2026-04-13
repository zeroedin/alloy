package cmd_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCmd(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cmd Suite")
}

// Clean up test artifacts that "alloy init" creates in CWD.
// The init smoke test (no directory arg) writes alloy.config.yaml
// into the cmd/ package directory. Remove it before and after the
// suite so tests are idempotent across repeated runs.
var _ = BeforeSuite(func() {
	os.Remove("alloy.config.yaml")
})

var _ = AfterSuite(func() {
	os.Remove("alloy.config.yaml")
})
