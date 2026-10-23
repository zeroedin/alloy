package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── Output safety filters (issue #1266) ───────────────────────────────
// Go's html/template escapes every interpolated value by the rules of
// its context — HTML body, <style>, <script>, URL attribute, attribute
// position. A plain string never bypasses that; only a value typed
// template.HTML, template.CSS, template.JS, template.URL or
// template.HTMLAttr does, and each type unlocks only its own context.
//
// Alloy ships five filters named after Hugo's — safeHTML, safeCSS, safeJS,
// safeURL, safeHTMLAttr — one per context. In Go templates each marks
// its input trusted for that context. Liquid does not auto-escape, so
// there they return the input unchanged as a plain string.
//
// Today safeHTML returns a plain string, so it does nothing in Go
// templates, and the other four do not exist.
//
// See PLAN.md → "Output safety (issue #1266)".

var _ = Describe("Output safety filters (issue #1266)", func() {

	Describe("Go templates", func() {
		var engine tmpl.TemplateEngine

		BeforeEach(func() {
			engine = tmpl.NewGoEngine()
			// html/template binds functions at parse time, so filters must
			// be registered before Parse.
			Expect(tmpl.RegisterBuiltinFilters(engine)).To(Succeed())
		})

		render := func(src string, v string) string {
			GinkgoHelper()
			tpl, err := engine.Parse("safety", []byte(src))
			Expect(err).NotTo(HaveOccurred())
			out, err := tpl.Render(map[string]interface{}{"v": v})
			Expect(err).NotTo(HaveOccurred())
			return string(out)
		}

		Context("each filter unlocks its own context", func() {
			It("safeHTML renders trusted HTML unescaped in the body", func() {
				Expect(render(`<p>{{ safeHTML .v }}</p>`, "<b>hi</b>")).
					To(Equal("<p><b>hi</b></p>"),
						"safeHTML must return template.HTML so html/template does "+
							"not escape it — today it returns a plain string and "+
							"the output is &lt;b&gt;hi&lt;/b&gt; (issue #1266)")
			})

			It("safeHTML works in pipe form", func() {
				Expect(render(`<p>{{ .v | safeHTML }}</p>`, "<b>hi</b>")).
					To(Equal("<p><b>hi</b></p>"),
						"{{ .v | safeHTML }} is the same call as {{ safeHTML .v }} (issue #1266)")
			})

			It("safeCSS renders trusted CSS inside <style>", func() {
				out := render(`<style>p { {{ safeCSS .v }} }</style>`, "color: red; background: url(x.png)")
				Expect(out).To(Equal("<style>p { color: red; background: url(x.png) }</style>"),
					"safeCSS must return template.CSS — without it html/template "+
						"replaces the value with ZgotmplZ (issue #1266)")
			})

			It("safeJS renders trusted JavaScript inside <script> unquoted", func() {
				out := render(`<script>var a = {{ safeJS .v }};</script>`, "alert(1)")
				Expect(out).To(Equal("<script>var a = alert(1);</script>"),
					"safeJS must return template.JS — without it html/template "+
						"emits the value as a quoted string literal (issue #1266)")
			})

			It("safeURL lets a trusted URL with any scheme through in href", func() {
				out := render(`<a href="{{ safeURL .v }}">x</a>`, "javascript:void(0)")
				Expect(out).NotTo(ContainSubstring("ZgotmplZ"),
					"safeURL must return template.URL — without it html/template "+
						"rejects non-http schemes as #ZgotmplZ (issue #1266)")
				Expect(out).To(HavePrefix(`<a href="javascript:void`),
					"the trusted URL's scheme must survive; html/template may still "+
						"percent-encode characters such as parentheses")
			})

			It("safeHTMLAttr renders a trusted attribute in attribute position", func() {
				out := render(`<div {{ safeHTMLAttr .v }}>x</div>`, `title="hello"`)
				Expect(out).To(Equal(`<div title="hello">x</div>`),
					"safeHTMLAttr must return template.HTMLAttr — without it "+
						"html/template replaces the value with ZgotmplZ (issue #1266)")
			})
		})

		Context("without a filter, every context stays escaped (guards)", func() {
			It("escapes HTML in the body", func() {
				Expect(render(`<p>{{ .v }}</p>`, "<b>hi</b>")).
					To(Equal("<p>&lt;b&gt;hi&lt;/b&gt;</p>"),
						"data is escaped by default in Go templates — #1266 adds an "+
							"opt-out, it does not change the default")
			})

			It("blanks CSS in <style>", func() {
				Expect(render(`<style>p { {{ .v }} }</style>`, "color: red; background: url(x.png)")).
					To(ContainSubstring("ZgotmplZ"))
			})

			It("quotes a value in <script>", func() {
				Expect(render(`<script>var a = {{ .v }};</script>`, "alert(1)")).
					To(Equal(`<script>var a = "alert(1)";</script>`))
			})

			It("rejects an unsafe URL scheme in href", func() {
				Expect(render(`<a href="{{ .v }}">x</a>`, "javascript:void(0)")).
					To(ContainSubstring("#ZgotmplZ"))
			})

			It("blanks a value in attribute position", func() {
				Expect(render(`<div {{ .v }}>x</div>`, `title="hello"`)).
					To(ContainSubstring("ZgotmplZ"))
			})
		})

		Context("trust is narrow (guards)", func() {
			It("safeHTML does not unlock a URL context", func() {
				// Each type unlocks only its own context: template.HTML in an
				// href is still checked as a URL.
				Expect(render(`<a href="{{ safeHTML .v }}">x</a>`, "javascript:void(0)")).
					To(ContainSubstring("#ZgotmplZ"),
						"safeHTML must not be usable to smuggle a URL past "+
							"html/template's scheme check — use safeURL (issue #1266)")
			})

			It("trust ends at the next filter", func() {
				// upcase returns a plain string, which html/template escapes.
				Expect(render(`<p>{{ safeHTML .v | upcase }}</p>`, "<b>hi</b>")).
					To(Equal("<p>&lt;B&gt;HI&lt;/B&gt;</p>"),
						"a filter applied after safeHTML returns an ordinary string, "+
							"so the result is escaped again — safe* must be the last "+
							"step (issue #1266)")
			})
		})
	})

	Describe("Liquid", func() {
		var engine tmpl.TemplateEngine

		BeforeEach(func() {
			engine = tmpl.NewLiquidEngine()
			Expect(tmpl.RegisterBuiltinFilters(engine)).To(Succeed())
		})

		render := func(src string, v string) string {
			GinkgoHelper()
			tpl, err := engine.Parse("safety", []byte(src))
			Expect(err).NotTo(HaveOccurred())
			out, err := tpl.Render(map[string]interface{}{"v": v})
			Expect(err).NotTo(HaveOccurred(),
				"every safe* filter must be registered in Liquid too, so templates "+
					"stay portable between engines (issue #1266)")
			return string(out)
		}

		const raw = `<b>"hi"</b>`

		for _, name := range []string{"safeHTML", "safeCSS", "safeJS", "safeURL", "safeHTMLAttr"} {
			name := name

			It(name+" returns its input unchanged", func() {
				Expect(render(`{{ v | `+name+` }}`, raw)).To(Equal(raw),
					"Liquid does not auto-escape, so %s has nothing to bypass and "+
						"must pass the value through untouched (issue #1266)", name)
			})

			It(name+" returns a plain string other filters can use", func() {
				Expect(render(`{{ v | `+name+` | size }}`, raw)).To(Equal("11"),
					"%s must hand Liquid a plain string — a template.HTML-style "+
						"named type makes size return 0 (issue #1266)", name)
			})
		}

		It("prints data raw by default (guard)", func() {
			Expect(render(`{{ v }}`, "<b>hi</b>")).To(Equal("<b>hi</b>"),
				"Liquid's default is unescaped output. #1266 documents this "+
					"divergence from Go templates rather than changing it")
		})

		It("escapes with | escape (guard)", func() {
			Expect(render(`{{ v | escape }}`, "<b>hi</b>")).To(Equal("&lt;b&gt;hi&lt;/b&gt;"),
				"| escape is Liquid's opt-in to escaping (issue #1266)")
		})
	})
})
