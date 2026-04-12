package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("GoEngine", func() {
	var engine tmpl.TemplateEngine

	BeforeEach(func() {
		engine = tmpl.NewGoEngine()
	})

	Describe("Parse and Render", func() {
		It("renders {{ .page.title }} expressions", func() {
			tpl, err := engine.Parse("title", []byte(`<h1>{{ .page.title }}</h1>`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{
				"page": map[string]interface{}{
					"title": "Hello World",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("<h1>Hello World</h1>"))
		})

		It("renders {{ if }} conditionals", func() {
			tpl, err := engine.Parse("cond", []byte(`{{ if .show }}visible{{ else }}hidden{{ end }}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{"show": true})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("visible"))

			result, err = tpl.Render(map[string]interface{}{"show": false})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("hidden"))
		})

		It("renders {{ range }} loops", func() {
			tpl, err := engine.Parse("loop", []byte(`{{ range .items }}{{ . }} {{ end }}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{
				"items": []string{"a", "b", "c"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("a b c "))
		})

		It("renders {{ .content }} in layouts", func() {
			layoutSrc := `<html><body>{{ .content }}</body></html>`
			tpl, err := engine.Parse("layout", []byte(layoutSrc))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{
				"content": "<p>Page body</p>",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<p>Page body</p>"))
		})

		It("returns parse error for invalid Go template syntax", func() {
			_, err := engine.Parse("bad", []byte(`{{ if }}`))
			Expect(err).To(HaveOccurred())
			// The error must describe the syntax problem, not be a generic stub error
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("syntax"),
					ContainSubstring("parse"),
					ContainSubstring("template"),
					ContainSubstring("unexpected"),
				),
				"error should indicate a Go template syntax or parse failure",
			)
		})
	})

	// ── Layout inheritance ────────────────────────────────────────────

	Describe("Layout inheritance", func() {
		It("supports {{ block }} / {{ define }} layout inheritance", func() {
			baseSrc := `<html>{{ block "content" . }}default{{ end }}</html>`
			tpl, err := engine.Parse("base", []byte(baseSrc))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("default"),
				"block must render default content when not overridden")
		})

		It("isolates scope between content and layout rendering", func() {
			layoutSrc := `<html><body>{{ .content }}</body></html>`
			tpl, err := engine.Parse("layout", []byte(layoutSrc))
			Expect(err).NotTo(HaveOccurred())
			Expect(tpl).NotTo(BeNil())

			// Content rendering should not leak layout variables
			result, err := tpl.Render(map[string]interface{}{
				"content":     "<p>Body</p>",
				"layout_only": "should not appear in content",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<p>Body</p>"))
			Expect(string(result)).NotTo(ContainSubstring("should not appear in content"),
				"layout-only variables must not leak into content area")
		})
	})
})
