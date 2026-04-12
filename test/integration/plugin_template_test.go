package integration_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Plugin-Template Integration", func() {

	Describe("Filter integration with Liquid engine", func() {
		It("Tier 1 built-in filter is accessible during Liquid template render", func() {
			engine := template.NewLiquidEngine()
			// Register a built-in filter
			err := engine.AddFilter("upcase", func(input interface{}, args ...interface{}) interface{} {
				return nil // stub filter
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte("{{ title | upcase }}"))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{"title": "hello"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("HELLO"),
				"built-in filter must transform value during Liquid rendering")
		})

		It("Tier 1 built-in filter is accessible during Go template render", func() {
			engine := template.NewGoEngine()
			err := engine.AddFilter("upcase", func(input interface{}, args ...interface{}) interface{} {
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{{ upcase .title }}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{"title": "hello"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("HELLO"),
				"built-in filter must transform value during Go template rendering")
		})

		It("plugin-registered filter is accessible during template render", func() {
			engine := template.NewLiquidEngine()
			// Simulate a plugin registering a custom filter
			err := engine.AddFilter("wordCount", func(input interface{}, args ...interface{}) interface{} {
				return nil // plugin-provided implementation
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte("{{ page.content | wordCount }}"))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{
				"page": map[string]interface{}{"content": "one two three"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty(),
				"plugin-registered filter must produce output during rendering")
		})
	})

	Describe("Shortcode integration with template engines", func() {
		It("plugin-registered shortcode expands in content rendering", func() {
			engine := template.NewLiquidEngine()
			err := engine.AddTag("youtube", func(args []string, content string) string {
				return "" // stub shortcode
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{% youtube "dQw4w9WgXcQ" %}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("youtube"),
				"shortcode must expand to output containing the tag type")
		})

		It("shortcode registered once wires into both Liquid and Go engines", func() {
			shortcodeFn := func(args []string, content string) string {
				return "<div class='callout'>" + content + "</div>"
			}

			liquidEngine := template.NewLiquidEngine()
			goEngine := template.NewGoEngine()

			errLiquid := liquidEngine.AddTag("callout", shortcodeFn)
			errGo := goEngine.AddTag("callout", shortcodeFn)

			Expect(errLiquid).NotTo(HaveOccurred(),
				"shortcode must register in Liquid engine without error")
			Expect(errGo).NotTo(HaveOccurred(),
				"same shortcode must register in Go engine without error")
		})
	})
})
