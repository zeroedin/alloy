package template_test

import (
	"fmt"

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

	// ── Issue #200: Plugin filter shadows built-in in Liquid engine ──
	// RegisterBuiltinFilters registers built-ins first. Then AddFilter
	// registers a plugin filter with the same name. The plugin must win.

	Context("Plugin filter shadows built-in", func() {
		It("AddFilter after RegisterBuiltinFilters overrides the built-in", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			// Override "upcase" with a plugin version
			err := engine.AddFilter("upcase", func(input interface{}, args ...interface{}) interface{} {
				return "SHADOWED:" + fmt.Sprint(input)
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{{ "hello" | upcase }}`))
			Expect(err).NotTo(HaveOccurred())
			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("SHADOWED:hello"),
				"plugin filter registered via AddFilter after RegisterBuiltinFilters "+
					"must override the built-in — last loaded wins per spec §4")
		})
	})

	// ── Issue #199: Built-in filters through Liquid rendering ────────
	// Filter functions pass unit tests but may fail through the Liquid
	// engine due to type mismatches, registration issues, or dispatch.

	Context("Built-in filters through Liquid rendering", func() {
		// RegisterBuiltinFilters must be called so the Liquid engine
		// has access to Alloy's built-in filters (findRE, replaceRE, etc.)
		// Without it, the filter bridge has nothing to dispatch to.

		It("findRE returns matches in template", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)
			tpl, err := engine.Parse("test", []byte(`{{ "hello world 123" | findRE: "[0-9]+" }}`))
			Expect(err).NotTo(HaveOccurred())
			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			rendered := string(result)
			// Assert the output is NOT the original input — proves the
			// filter actually ran, not just passed through
			Expect(rendered).NotTo(Equal("hello world 123"),
				"findRE must transform the input, not pass it through")
			Expect(rendered).To(ContainSubstring("123"),
				"findRE must return matches when used in Liquid template")
		})

		It("replaceRE performs substitution in template", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)
			tpl, err := engine.Parse("test", []byte(`{{ "hello world" | replaceRE: "world", "alloy" }}`))
			Expect(err).NotTo(HaveOccurred())
			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			rendered := string(result)
			Expect(rendered).To(ContainSubstring("hello alloy"),
				"replaceRE must perform regex replacement in Liquid template")
			Expect(rendered).NotTo(Equal("world"),
				"replaceRE must not return the replacement argument as the entire output")
		})

		It("contains returns boolean usable in conditionals", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)
			// Test both positive and negative cases to prove contains
			// returns a boolean, not the input string (which would be
			// truthy for both cases)
			tpl, err := engine.Parse("test", []byte(
				`{% if "hello world" | contains: "world" %}YES{% else %}NO{% endif %}`+
					`|{% if "hello world" | contains: "nope" %}YES{% else %}NO{% endif %}`))
			Expect(err).NotTo(HaveOccurred())
			result, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("YES|NO"),
				"contains must return true when substring present and false when absent — "+
					"not return the input string unchanged")
		})

		It("newline_to_br converts newlines to br tags", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)
			// Pass the string with a real newline via render context —
			// a backtick template literal \n is a literal backslash-n
			tpl, err := engine.Parse("test", []byte(`{{ s | newline_to_br }}`))
			Expect(err).NotTo(HaveOccurred())
			result, err := tpl.Render(map[string]interface{}{
				"s": "hello\nworld",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<br"),
				"newline_to_br must produce <br> tags in Liquid template")
		})
	})
})
