package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── Output safety in Liquid (issue #1285) ──────────────────────────────
// Liquid prints values as written; `| escape` escapes them. With Go
// templates removed (#1285), the five safe* filters (safeHTML, safeCSS,
// safeJS, safeURL, safeHTMLAttr) are removed too: they only marked values
// as trusted for Go's html/template and did nothing in Liquid. A template
// that still uses one gets Liquid's undefined-filter error.
//
// See PLAN.md → "Liquid is the only template engine (issue #1285)".

var safeFilterNames = []string{"safeHTML", "safeCSS", "safeJS", "safeURL", "safeHTMLAttr"}

var _ = Describe("Output safety in Liquid (issue #1285)", func() {
	var engine tmpl.TemplateEngine

	BeforeEach(func() {
		engine = tmpl.NewLiquidEngine()
		Expect(tmpl.RegisterBuiltinFilters(engine)).To(Succeed())
	})

	render := func(src string, v string) (string, error) {
		GinkgoHelper()
		tpl, err := engine.Parse("safety", []byte(src))
		if err != nil {
			return "", err
		}
		out, err := tpl.Render(map[string]interface{}{"v": v})
		return string(out), err
	}

	It("prints data raw by default (guard)", func() {
		out, err := render(`{{ v }}`, "<b>hi</b>")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("<b>hi</b>"), "Liquid's default is unescaped output")
	})

	It("escapes with | escape (guard)", func() {
		out, err := render(`{{ v | escape }}`, "<b>hi</b>")
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal("&lt;b&gt;hi&lt;/b&gt;"), "| escape is Liquid's opt-in to escaping")
	})

	for _, name := range safeFilterNames {
		name := name

		It(name+" is no longer a filter", func() {
			_, err := render(`{{ v | `+name+` }}`, "<b>hi</b>")
			Expect(err).To(HaveOccurred(),
				"%s existed only for Go templates and is removed with them (#1285)", name)
			Expect(err.Error()).To(ContainSubstring("undefined filter " + name))
		})

		// RenderTemplate renders Liquid without the engine (Server.RenderPage,
		// permalinks). Its filters resolve as alloyFilterBridge methods, so
		// the methods go too.
		It(name+" is no longer a filter through RenderTemplate", func() {
			_, err := tmpl.RenderTemplate(`{{ v | `+name+` }}`, "safety", map[string]interface{}{"v": "<b>hi</b>"})
			Expect(err).To(HaveOccurred(),
				"%s must be gone from RenderTemplate too (#1285)", name)
			Expect(err.Error()).To(ContainSubstring("undefined filter " + name))
		})
	}
})
