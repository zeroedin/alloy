package template_test

import (
	"fmt"
	"os"
	"path/filepath"

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

	// ── {% inline %} tag (issue #288) ──────────────────────────────
	// Content-relative file inlining. Reads a file relative to the
	// content file's directory and inserts raw contents. No template
	// processing — raw UTF-8 text insertion.

	Describe("inline tag", func() {
		It("inlines an SVG file from the content directory", func() {
			// Set up a temp content directory with an SVG file
			tmpDir, err := os.MkdirTemp("", "inline-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			contentDir := filepath.Join(tmpDir, "content", "about")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			svgContent := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`
			Expect(os.WriteFile(filepath.Join(contentDir, "diagram.svg"), []byte(svgContent), 0644)).To(Succeed())

			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterInlineTag(engine)

			tpl, err := engine.Parse("test", []byte(`Before {% inline "./diagram.svg" %} After`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"_contentDir": contentDir,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring(`<svg xmlns=`),
				"inline tag must insert raw SVG content into output")
			Expect(string(result)).To(ContainSubstring("Before"),
				"content before the inline tag must be preserved")
			Expect(string(result)).To(ContainSubstring("After"),
				"content after the inline tag must be preserved")
		})

		It("inserts content raw without template processing", func() {
			tmpDir, err := os.MkdirTemp("", "inline-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			contentDir := filepath.Join(tmpDir, "content")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			// File contains Liquid-like syntax that must NOT be processed
			fileContent := `<div data-x="{{value}}">{% if true %}yes{% endif %}</div>`
			Expect(os.WriteFile(filepath.Join(contentDir, "raw.html"), []byte(fileContent), 0644)).To(Succeed())

			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterInlineTag(engine)

			tpl, err := engine.Parse("test", []byte(`{% inline "./raw.html" %}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"_contentDir": contentDir,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("{{value}}"),
				"inline must insert raw content — Liquid expressions must NOT be evaluated")
			Expect(string(result)).To(ContainSubstring("{% if true %}"),
				"inline must insert raw content — Liquid tags must NOT be processed")
		})

		It("returns error for binary file types", func() {
			tmpDir, err := os.MkdirTemp("", "inline-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			contentDir := filepath.Join(tmpDir, "content")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "photo.png"), []byte("fake png"), 0644)).To(Succeed())

			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterInlineTag(engine)

			tpl, err := engine.Parse("test", []byte(`{% inline "./photo.png" %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"_contentDir": contentDir,
			})
			Expect(err).To(HaveOccurred(),
				"inline must reject binary file types")
			Expect(err.Error()).To(ContainSubstring(".png"),
				"error must mention the rejected file extension")
		})

		It("returns error for absolute paths", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterInlineTag(engine)

			tpl, err := engine.Parse("test", []byte(`{% inline "/etc/passwd" %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"_contentDir": "/some/dir",
			})
			Expect(err).To(HaveOccurred(),
				"inline must reject absolute paths")
		})

		It("returns error when file not found", func() {
			tmpDir, err := os.MkdirTemp("", "inline-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterInlineTag(engine)

			tpl, err := engine.Parse("test", []byte(`{% inline "./nonexistent.svg" %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"_contentDir": tmpDir,
			})
			Expect(err).To(HaveOccurred(),
				"inline must error when file is not found — not silently produce empty output")
		})

		It("resolves parent directory paths within content root", func() {
			tmpDir, err := os.MkdirTemp("", "inline-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			// Shared SVG one level up from content subdir
			contentRoot := filepath.Join(tmpDir, "content")
			Expect(os.MkdirAll(filepath.Join(contentRoot, "about"), 0755)).To(Succeed())
			sharedSvg := `<svg><rect width="100" height="100"/></svg>`
			Expect(os.WriteFile(filepath.Join(contentRoot, "shared.svg"), []byte(sharedSvg), 0644)).To(Succeed())

			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterInlineTag(engine)

			tpl, err := engine.Parse("test", []byte(`{% inline "../shared.svg" %}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tpl.Render(map[string]interface{}{
				"_contentDir":  filepath.Join(contentRoot, "about"),
				"_contentRoot": contentRoot,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<svg>"),
				"inline must resolve ../ paths that stay within the content root")
		})

		It("rejects paths that escape the content directory", func() {
			tmpDir, err := os.MkdirTemp("", "inline-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			contentRoot := filepath.Join(tmpDir, "content")
			Expect(os.MkdirAll(filepath.Join(contentRoot, "about"), 0755)).To(Succeed())

			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterInlineTag(engine)

			tpl, err := engine.Parse("test", []byte(`{% inline "../../../../etc/passwd" %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"_contentDir":  filepath.Join(contentRoot, "about"),
				"_contentRoot": contentRoot,
			})
			Expect(err).To(HaveOccurred(),
				"inline must reject paths that traverse outside the content root — "+
					"this is a security boundary preventing arbitrary file reads")
		})
	})

	// ── Render hook discovery (issues #310, #311) ─────────────────
	// DiscoverRenderHooks scans layouts/_markup/ for render hook
	// template files and returns a map of hook name → template source.

	Describe("Render hook discovery", func() {
		It("discovers render hook templates from layouts/_markup/", func() {
			tmpDir, err := os.MkdirTemp("", "hooks-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			markupDir := filepath.Join(tmpDir, "layouts", "_markup")
			Expect(os.MkdirAll(markupDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(markupDir, "render-link.liquid"),
				[]byte(`<a href="{{ markup.destination }}">{{ markup.text }}</a>`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(markupDir, "render-codeblock.liquid"),
				[]byte(`<pre class="custom">{{ markup.inner }}</pre>`), 0644)).To(Succeed())

			hooks, err := tmpl.DiscoverRenderHooks(filepath.Join(tmpDir, "layouts"), "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(hooks).To(HaveLen(2))
			Expect(hooks).To(HaveKey("link"),
				"render-link.liquid must be discovered as hook 'link'")
			Expect(hooks).To(HaveKey("codeblock"),
				"render-codeblock.liquid must be discovered as hook 'codeblock'")
			Expect(hooks["link"]).To(ContainSubstring("markup.destination"),
				"hook value must be the template source content")
		})

		It("discovers language-specific codeblock hooks", func() {
			tmpDir, err := os.MkdirTemp("", "hooks-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			markupDir := filepath.Join(tmpDir, "layouts", "_markup")
			Expect(os.MkdirAll(markupDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(markupDir, "render-codeblock-mermaid.liquid"),
				[]byte(`<div class="mermaid">{{ markup.inner }}</div>`), 0644)).To(Succeed())

			hooks, err := tmpl.DiscoverRenderHooks(filepath.Join(tmpDir, "layouts"), "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(hooks).To(HaveKey("codeblock-mermaid"),
				"render-codeblock-mermaid.liquid must be discovered as hook 'codeblock-mermaid'")
		})

		It("returns empty map when _markup/ directory does not exist", func() {
			tmpDir, err := os.MkdirTemp("", "hooks-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			Expect(os.MkdirAll(filepath.Join(tmpDir, "layouts"), 0755)).To(Succeed())
			// No _markup/ directory

			hooks, err := tmpl.DiscoverRenderHooks(filepath.Join(tmpDir, "layouts"), "liquid")
			Expect(err).NotTo(HaveOccurred(),
				"missing _markup/ directory must not be an error")
			Expect(hooks).To(BeEmpty(),
				"no hooks when _markup/ doesn't exist")
		})

		It("only discovers files matching the configured engine extension", func() {
			tmpDir, err := os.MkdirTemp("", "hooks-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			markupDir := filepath.Join(tmpDir, "layouts", "_markup")
			Expect(os.MkdirAll(markupDir, 0755)).To(Succeed())

			// Both liquid and html files exist
			Expect(os.WriteFile(filepath.Join(markupDir, "render-link.liquid"),
				[]byte("liquid link"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(markupDir, "render-link.html"),
				[]byte("go link"), 0644)).To(Succeed())

			// With liquid engine, only .liquid files are discovered
			hooks, err := tmpl.DiscoverRenderHooks(filepath.Join(tmpDir, "layouts"), "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(hooks["link"]).To(Equal("liquid link"),
				"must discover .liquid files when engine is liquid")

			// With gotemplate engine, only .html files are discovered
			hooks, err = tmpl.DiscoverRenderHooks(filepath.Join(tmpDir, "layouts"), "gotemplate")
			Expect(err).NotTo(HaveOccurred())
			Expect(hooks["link"]).To(Equal("go link"),
				"must discover .html files when engine is gotemplate")
		})

		It("ignores unrecognized filenames", func() {
			tmpDir, err := os.MkdirTemp("", "hooks-test-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(tmpDir) })

			markupDir := filepath.Join(tmpDir, "layouts", "_markup")
			Expect(os.MkdirAll(markupDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(markupDir, "render-link.liquid"),
				[]byte("valid"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(markupDir, "not-a-hook.liquid"),
				[]byte("invalid"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(markupDir, "render-unicorn.liquid"),
				[]byte("not a valid hook type"), 0644)).To(Succeed())

			hooks, err := tmpl.DiscoverRenderHooks(filepath.Join(tmpDir, "layouts"), "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(hooks).To(HaveLen(1),
				"only recognized render-{type} files must be discovered")
			Expect(hooks).To(HaveKey("link"))
		})
	})
})
