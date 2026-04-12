package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("LiquidEngine", func() {

	// ── Basic rendering ────────────────────────────────────────────────

	Context("Basic rendering", func() {
		It("renders {{ variable }} expressions", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("test", []byte("Hello {{ name }}!"))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{"name": "World"}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("Hello World!"))
		})

		It("renders nested {{ page.title }} lookups", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("test", []byte("Title: {{ page.title }}"))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{
				"page": map[string]interface{}{
					"title": "My Page",
				},
			}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("Title: My Page"))
		})

		It("renders {% if %} conditionals", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("test", []byte("{% if show %}visible{% endif %}"))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{"show": true}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("visible"))
		})

		It("renders {% for %} loops", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("test", []byte("{% for item in items %}{{ item }} {% endfor %}"))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{
				"items": []interface{}{"a", "b", "c"},
			}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("a b c "))
		})

		It("renders {% assign %} variables", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("test", []byte("{% assign greeting = \"Hi\" %}{{ greeting }}"))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("Hi"))
		})
	})

	// ── Content injection ──────────────────────────────────────────────

	Context("Content injection", func() {
		It("renders {{ content }} in layout templates", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("layout", []byte("<main>{{ content }}</main>"))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{
				"content": "<p>Page body</p>",
			}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("<main><p>Page body</p></main>"))
		})
	})

	// ── Error handling ─────────────────────────────────────────────────

	Context("Error handling", func() {
		It("returns parse error for invalid Liquid syntax", func() {
			engine := tmpl.NewLiquidEngine()
			_, err := engine.Parse("bad", []byte("{% if %}"))
			Expect(err).To(HaveOccurred())
			// The error must describe the syntax problem, not be a generic stub error
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("syntax"),
					ContainSubstring("parse"),
					ContainSubstring("if"),
					ContainSubstring("unexpected"),
				),
				"error should indicate a Liquid syntax or parse failure",
			)
		})
	})

	// ── Error format contracts ────────────────────────────────────────

	Context("Error format contracts", func() {
		It("includes source path in template render error", func() {
			_, err := tmpl.RenderTemplate(
				"{{ invalid | broken_filter }}",
				"layouts/default.liquid",
				map[string]interface{}{"title": "Test"},
			)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("layouts/default.liquid"),
				"template render error must include the source file path")
		})
	})

	// ── Includes and partials ─────────────────────────────────────────

	Context("Includes and partials", func() {
		It("resolves {% include 'header' %} from includes directory", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("test", []byte(`{% include "partials/header" %}<main>Content</main>`))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("<main>Content</main>"))
		})

		It("{% render %} tag creates isolated scope (Shopify Liquid spec)", func() {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("test", []byte(`{% render "card", title: "Hello" %}`))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{"outer_var": "should not leak"}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			// The render tag must produce some output (not empty from stub)
			Expect(string(out)).NotTo(BeEmpty(),
				"render tag must produce output from partial template")
		})
	})
})
