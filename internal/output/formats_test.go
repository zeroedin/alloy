package output_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/output"
)

var _ = Describe("Output Formats", func() {

	// ── Default format ────────────────────────────────────────────────

	Context("Default format", func() {
		It("outputs html when no outputs specified in page", func() {
			page := &content.Page{
				Outputs:     nil,
				FrontMatter: map[string]interface{}{"title": "Plain Page"},
			}
			format := output.ResolveOutputFormat(page)
			Expect(format).To(Equal("html"))
		})
	})

	// ── Multiple output formats ───────────────────────────────────────

	Context("Multiple output formats", func() {
		It("page with outputs html and json returns html as primary", func() {
			page := &content.Page{
				Outputs:     []string{"html", "json"},
				FrontMatter: map[string]interface{}{"title": "Multi-format Page"},
			}
			format := output.ResolveOutputFormat(page)
			Expect(format).To(Equal("html"))
		})
	})

	// ── Template file extension mapping (Liquid) ──────────────────────

	Context("Template file extension mapping (Liquid)", func() {
		It("looks for single.json.liquid for json format", func() {
			page := &content.Page{
				Section:     "blog",
				Layout:      "single",
				FrontMatter: map[string]interface{}{"title": "JSON Page"},
			}
			layoutPath, err := output.ResolveFormatLayout(page, "json", "layouts", "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(layoutPath).To(ContainSubstring("single.json.liquid"))
		})

		It("falls back to single.json if .liquid not found", func() {
			page := &content.Page{
				Section:     "blog",
				Layout:      "single",
				FrontMatter: map[string]interface{}{"title": "JSON Fallback"},
			}
			layoutPath, err := output.ResolveFormatLayout(page, "json", "layouts", "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(layoutPath).To(ContainSubstring("single.json"))
		})
	})
})
