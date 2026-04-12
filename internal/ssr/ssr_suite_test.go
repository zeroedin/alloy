package ssr_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSSR(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "SSR Suite")
}
