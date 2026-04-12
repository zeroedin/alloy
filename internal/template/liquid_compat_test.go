package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Liquid Compatibility (§12 Phase 1)", func() {

	Describe("forloop.parentloop", func() {
		It("provides access to parent loop context in nested for loops", func() {
			engine := template.NewLiquidEngine()
			tmpl, err := engine.Parse("test", []byte(
				`{% for outer in outers %}{% for inner in inners %}{{ forloop.parentloop.index }}{% endfor %}{% endfor %}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{
				"outers": []string{"a", "b"},
				"inners": []string{"x", "y"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("1"),
				"forloop.parentloop.index must resolve to parent loop iteration")
		})
	})

	Describe("Whitespace control", func() {
		It("strips whitespace with {%- and -%} tags", func() {
			engine := template.NewLiquidEngine()
			tmpl, err := engine.Parse("test", []byte(
				`  {%- if true -%}  hello  {%- endif -%}  `))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("hello"),
				"whitespace control tags must strip surrounding whitespace")
		})
	})

	Describe("{% render %} scoping", func() {
		It("rendered template cannot access parent scope variables", func() {
			engine := template.NewLiquidEngine()
			// The render tag creates an isolated scope — parent vars are not accessible
			tmpl, err := engine.Parse("test", []byte(
				`{% assign secret = "hidden" %}{% render "child" %}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			// The child template should NOT have access to "secret"
			Expect(string(result)).NotTo(ContainSubstring("hidden"),
				"{% render %} must create an isolated scope — no parent variable leakage")
		})
	})

	Describe("{% tablerow %}", func() {
		It("generates HTML table rows from a collection", func() {
			engine := template.NewLiquidEngine()
			tmpl, err := engine.Parse("test", []byte(
				`<table>{% tablerow item in items cols:2 %}{{ item }}{% endtablerow %}</table>`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{
				"items": []string{"a", "b", "c"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<tr"),
				"tablerow must generate <tr> elements")
			Expect(string(result)).To(ContainSubstring("<td"),
				"tablerow must generate <td> elements")
		})
	})
})
