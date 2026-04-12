package ssr_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/stretchr/testify/mock"
	"github.com/zeroedin/alloy/internal/ssr"
)

// MockSSREngine implements SSREngine for testing.
type MockSSREngine struct {
	mock.Mock
}

func (m *MockSSREngine) Render(html string) (string, error) {
	args := m.Called(html)
	return args.String(0), args.Error(1)
}

var _ = Describe("Mock-based SSR tests", func() {
	It("SSREngine.Render can be mocked to return DSD output", func() {
		engine := new(MockSSREngine)
		engine.On("Render", "<ds-card></ds-card>").Return(
			`<ds-card><template shadowrootmode="open"><slot></slot></template></ds-card>`,
			nil,
		)

		result, err := engine.Render("<ds-card></ds-card>")
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(ContainSubstring("shadowrootmode"))

		engine.AssertExpectations(GinkgoT())

		// Verify the real SSR scanner can detect components in the rendered output
		instances := ssr.ScanComponents(result)
		Expect(instances).NotTo(BeEmpty(),
			"ScanComponents must detect custom elements in SSR output")
	})

	It("SSREngine satisfies the interface", func() {
		var engine ssr.SSREngine = new(MockSSREngine)
		Expect(engine).NotTo(BeNil())

		// Verify StampBack can insert DSD into HTML with component markers
		html := `<ds-card title="Hello">content</ds-card>`
		stamped := ssr.StampBack(html, map[string]string{
			"ds-card": `<template shadowrootmode="open"><slot></slot></template>`,
		})
		Expect(stamped).To(ContainSubstring("shadowrootmode"),
			"StampBack must insert DSD template into component HTML")
	})
})
