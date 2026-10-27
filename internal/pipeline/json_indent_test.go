package pipeline_test

import (
	"html"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── `json` filter indent argument, end to end (issue #1269) ──────────
// The argument works in both engines. Unit rules are in
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

	Context("Go templates", func() {
		// Go templates print data in alphabetical key order today, not file
		// order. That is out of scope here (#1285), so this fixture's file
		// order is already alphabetical and the test checks indentation only.
		const navYAML = "alpha: /\nbeta:\n  a: 1\n  b:\n    - x\n"
		const indented = `{
  "alpha": "/",
  "beta": {
    "a": 1,
    "b": [
      "x"
    ]
  }
}`

		build := func(layout string) string {
			GinkgoHelper()
			result, err := pipeline.BuildWithContent(goTrustConfig("JSON Indent Go"), map[string]string{
				"data/nav.yaml":        navYAML,
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\n",
				"layouts/default.html": layout,
			})
			Expect(err).NotTo(HaveOccurred())
			// html/template escapes the quotes for HTML (&#34;); the
			// browser shows them as quotes. Compare the text it shows.
			return html.UnescapeString(result.RenderedContent["index.md"])
		}

		It("prints one line with no argument (guard)", func() {
			Expect(build("<pre>{{ json .site.data.nav }}</pre>")).To(ContainSubstring(
				`<pre>{"alpha":"/","beta":{"a":1,"b":["x"]}}</pre>`))
		})

		It("indents with a string argument in function form", func() {
			Expect(build(`<pre>{{ json .site.data.nav "  " }}</pre>`)).To(ContainSubstring("<pre>" + indented + "</pre>"))
		})

		It("indents with a number argument in function form", func() {
			Expect(build("<pre>{{ json .site.data.nav 2 }}</pre>")).To(ContainSubstring("<pre>" + indented + "</pre>"))
		})
	})
})
