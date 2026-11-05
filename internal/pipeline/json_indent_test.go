package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── `json` filter indent argument, end to end (issue #1269) ──────────
// Unit rules are in
// internal/template/json_indent_test.go.
//
// See PLAN.md → "`json` indent argument (issue #1269)".

var _ = Describe("json filter indent argument in a real build (issue #1269)", func() {

	Context("Liquid", func() {
		// File order is not alphabetical, so the expected output also pins
		// that indenting keeps key order at every level.
		const navYAML = "home: /\nproducts:\n  zeta: /z/\n  alpha:\n    - one\n    - y: true\n      c: null\n"
		const indented = `{
  "home": "/",
  "products": {
    "zeta": "/z/",
    "alpha": [
      "one",
      {
        "y": true,
        "c": null
      }
    ]
  }
}`

		build := func(layout string) string {
			GinkgoHelper()
			result, err := pipeline.BuildWithContent(&config.Config{
				Title:   "JSON Indent Liquid",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}, map[string]string{
				"data/nav.yaml":          navYAML,
				"data/settings.yaml":     "indent: 2\n",
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n",
				"layouts/default.liquid": layout,
			})
			Expect(err).NotTo(HaveOccurred())
			return result.RenderedContent["index.md"]
		}

		It("prints one line with no argument (guard)", func() {
			Expect(build("J[{{ site.data.nav | json }}]")).To(ContainSubstring(
				`J[{"home":"/","products":{"zeta":"/z/","alpha":["one",{"y":true,"c":null}]}}]`))
		})

		It("indents with a string argument, keeping file order", func() {
			Expect(build(`J[{{ site.data.nav | json: "  " }}]`)).To(ContainSubstring("J[" + indented + "]"))
		})

		It("indents with a number literal", func() {
			Expect(build("J[{{ site.data.nav | json: 2 }}]")).To(ContainSubstring("J[" + indented + "]"))
		})

		It("indents with a number read from a data file", func() {
			Expect(build("J[{{ site.data.nav | json: site.data.settings.indent }}]")).To(ContainSubstring("J[" + indented + "]"))
		})
	})
})
