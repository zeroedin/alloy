package template_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
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

	// ── RenderTemplate cached environment (issue #364) ──────────────
	// RenderTemplate must not create a fresh liquid.Environment per call.
	// Multiple calls must produce identical output (shared environment).

	Context("RenderTemplate cached environment (issue #364)", func() {
		It("produces identical output across multiple calls", func() {
			ctx := map[string]interface{}{"name": "Alice"}
			result1, err := tmpl.RenderTemplate("Hello {{ name }}", "test1.liquid", ctx)
			Expect(err).NotTo(HaveOccurred())
			result2, err := tmpl.RenderTemplate("Hello {{ name }}", "test2.liquid", ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1).To(Equal(result2),
				"RenderTemplate must produce identical output across calls — "+
					"shared environment must not leak state between renders")
		})

		It("supports standard Liquid tags without per-call registration", func() {
			ctx := map[string]interface{}{"items": []interface{}{"a", "b", "c"}}
			result, err := tmpl.RenderTemplate(
				"{% for item in items %}{{ item }}{% endfor %}",
				"tags.liquid", ctx,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("abc"),
				"standard tags must work without per-call RegisterStandardTags — "+
					"environment should be cached with tags pre-registered")
		})

		It("alloy built-in filters are available in RenderTemplate", func() {
			ctx := map[string]interface{}{"name": "Hello World"}
			result, err := tmpl.RenderTemplate("{{ name | slugify }}", "filter.liquid", ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("hello-world"),
				"alloy-specific filters (slugify) must be available in RenderTemplate — "+
					"if this fails, the cached environment lacks RegisterBuiltinFilters. "+
					"RenderTemplate currently creates a bare environment without alloy filters")
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

	// ── Issue #376: Plugin filters in {% include %} partials ────────
	// Plugin-registered filters must work inside {% include %} partials.
	// The Liquid engine rewrites novel filter names during Parse(), but
	// included partials are parsed by liquidgo internally via
	// ReadTemplateFile — which must also apply the rewriting.

	Context("Plugin filters in {% include %} partials (issue #376)", func() {
		// Helper to set includes dir with assertion that it succeeds
		setIncludesDir := func(engine tmpl.TemplateEngine, dir string) {
			setter, ok := engine.(interface{ SetIncludesDir(string) })
			Expect(ok).To(BeTrue(),
				"engine must implement SetIncludesDir for include tests")
			setter.SetIncludesDir(dir)
		}

		It("plugin filter works in an included partial", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			err := engine.AddFilter("tokenType", func(input interface{}, args ...interface{}) interface{} {
				return "leaf"
			})
			Expect(err).NotTo(HaveOccurred())

			tmpDir := GinkgoT().TempDir()
			err = os.WriteFile(
				filepath.Join(tmpDir, "token-info.liquid"),
				[]byte(`<span>{{ tokenPath | tokenType }}</span>`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			setIncludesDir(engine, tmpDir)

			tpl, err := engine.Parse("test", []byte(
				`<div>{% include "token-info" tokenPath: "color" %}</div>`,
			))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred(),
				"plugin filter must not cause 'undefined filter' error in {% include %} partial — "+
					"ReadTemplateFile must apply the same filter rewriting as Parse()")
			Expect(string(out)).To(ContainSubstring("<span>leaf</span>"),
				"plugin filter must execute and produce output inside partial")
		})

		It("multiple plugin filters work in an included partial", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			err := engine.AddFilter("tokenType", func(input interface{}, args ...interface{}) interface{} {
				return "leaf"
			})
			Expect(err).NotTo(HaveOccurred())
			err = engine.AddFilter("tokenLabel", func(input interface{}, args ...interface{}) interface{} {
				return "Color Token"
			})
			Expect(err).NotTo(HaveOccurred())

			tmpDir := GinkgoT().TempDir()
			err = os.WriteFile(
				filepath.Join(tmpDir, "multi.liquid"),
				[]byte(`{{ path | tokenType }}-{{ path | tokenLabel }}`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			setIncludesDir(engine, tmpDir)

			tpl, err := engine.Parse("test", []byte(
				`{% include "multi" path: "color" %}`,
			))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred(),
				"multiple plugin filters must work in partials")
			Expect(string(out)).To(ContainSubstring("leaf-Color Token"))
		})

		It("built-in filters still work alongside plugin filters in partials", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			err := engine.AddFilter("tokenType", func(input interface{}, args ...interface{}) interface{} {
				return "leaf"
			})
			Expect(err).NotTo(HaveOccurred())

			tmpDir := GinkgoT().TempDir()
			err = os.WriteFile(
				filepath.Join(tmpDir, "mixed.liquid"),
				[]byte(`{{ name | upcase }}-{{ name | tokenType }}`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			setIncludesDir(engine, tmpDir)

			tpl, err := engine.Parse("test", []byte(
				`{% include "mixed" name: "color" %}`,
			))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred(),
				"built-in and plugin filters must coexist in partials")
			Expect(string(out)).To(ContainSubstring("COLOR-leaf"),
				"upcase (built-in) and tokenType (plugin) must both work")
		})

		It("plugin filter works in a recursively included partial", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			err := engine.AddFilter("nodeType", func(input interface{}, args ...interface{}) interface{} {
				return "branch"
			})
			Expect(err).NotTo(HaveOccurred())

			tmpDir := GinkgoT().TempDir()
			err = os.WriteFile(
				filepath.Join(tmpDir, "inner.liquid"),
				[]byte(`<em>{{ val | nodeType }}</em>`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile(
				filepath.Join(tmpDir, "outer.liquid"),
				[]byte(`<div>{% include "inner" val: item %}</div>`),
				0644,
			)
			Expect(err).NotTo(HaveOccurred())

			setIncludesDir(engine, tmpDir)

			tpl, err := engine.Parse("test", []byte(
				`{% include "outer" item: "root" %}`,
			))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred(),
				"plugin filter must work in recursively included partials — "+
					"filter rewriting must apply at every include level")
			Expect(string(out)).To(ContainSubstring("<em>branch</em>"),
				"plugin filter must execute in the innermost partial")
		})
	})

	// ── flatten filter through Liquid engine (issue #477) ───────────
	// flatten is in knownLiquidFilters and dispatched via the
	// alloyFilterBridge.Flatten method (reflection). This test verifies
	// the full parse → render path works end-to-end.

	Context("flatten filter through Liquid engine (issue #477)", func() {
		It("flatten works in a Liquid template", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			tpl, err := engine.Parse("test", []byte(
				`{% assign nested = "a,b|c,d" | split: "|" %}{% assign flat = nested | flatten %}{{ flat | join: "," }}`,
			))
			// Note: split by "|" gives ["a,b", "c,d"] — that's flat strings, not nested arrays.
			// We need actual nested arrays to test flatten. Use assign with split twice.
			Expect(err).NotTo(HaveOccurred())
			// This test verifies the filter is reachable through the Liquid engine.
			// Even if the template logic doesn't produce nested arrays from pure Liquid,
			// the parse/render path still verifies that `flatten` is recognized in
			// knownLiquidFilters and dispatched via alloyFilterBridge.Flatten.
			_, renderErr := tpl.Render(map[string]interface{}{})
			Expect(renderErr).NotTo(HaveOccurred(),
				"flatten must be dispatchable through the Liquid engine — "+
					"if this fails with 'undefined filter flatten', the filter is "+
					"not being recognized as a known Liquid filter or dispatched via "+
					"alloyFilterBridge.Flatten (issue #477)")
		})

		It("flatten on nested array data produces flat output", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			// Standard Liquid does not support filter pipes in {% for %} range
			// expressions. Use the two-step {% assign %} pattern instead.
			tpl, err := engine.Parse("test", []byte(
				`{% assign flat = nested | flatten %}{% for item in flat %}{{ item }},{% endfor %}`,
			))
			Expect(err).NotTo(HaveOccurred())

			ctx := map[string]interface{}{
				"nested": []interface{}{
					[]interface{}{"a", "b"},
					[]interface{}{"c", "d"},
				},
			}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred(),
				"flatten on nested array must not error in Liquid")
			Expect(string(out)).To(Equal("a,b,c,d,"),
				"flatten must collapse [[a,b],[c,d]] and iterate in order — "+
					"uses standard {% assign %} + {% for %} pattern (issue #483)")
		})
	})

	// ── Liquid bridge integration: where/sort/map on ordered.Map (#477) ─
	// These tests verify the alloyFilterBridge methods (Where, Sort, Map)
	// work through a full Liquid parse → render cycle with *ordered.Map data.
	// Unit tests in filters_test.go cover the FilterFunc level; these cover
	// the Liquid dispatch path.

	Context("Liquid bridge filters on ordered.Map data (issue #477)", func() {
		It("where filter finds matching ordered.Map items in Liquid", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			tpl, err := engine.Parse("test", []byte(
				`{% assign matches = items | where: "role", "engineer" %}{% for m in matches %}{{ m.name }},{% endfor %}`,
			))
			Expect(err).NotTo(HaveOccurred())

			items, _ := ordered.UnmarshalJSONValue([]byte(
				`[{"name":"Alice","role":"engineer"},{"name":"Bob","role":"designer"},{"name":"Charlie","role":"engineer"}]`,
			))
			ctx := map[string]interface{}{"items": items}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("Alice,Charlie,"),
				"where must filter *ordered.Map items by role=engineer through Liquid engine")
		})

		It("sort filter orders ordered.Map items in Liquid", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			tpl, err := engine.Parse("test", []byte(
				`{% assign sorted = items | sort: "name" %}{% for s in sorted %}{{ s.name }},{% endfor %}`,
			))
			Expect(err).NotTo(HaveOccurred())

			items, _ := ordered.UnmarshalJSONValue([]byte(
				`[{"name":"Charlie"},{"name":"Alice"},{"name":"Bob"}]`,
			))
			ctx := map[string]interface{}{"items": items}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("Alice,Bob,Charlie,"),
				"sort must order *ordered.Map items by name through Liquid engine")
		})

		It("map filter plucks field from ordered.Map items in Liquid", func() {
			engine := tmpl.NewLiquidEngine()
			tmpl.RegisterBuiltinFilters(engine)

			tpl, err := engine.Parse("test", []byte(
				`{% assign names = items | map: "name" %}{{ names | join: "," }}`,
			))
			Expect(err).NotTo(HaveOccurred())

			items, _ := ordered.UnmarshalJSONValue([]byte(
				`[{"name":"Alice","role":"engineer"},{"name":"Bob","role":"designer"}]`,
			))
			ctx := map[string]interface{}{"items": items}
			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("Alice,Bob"),
				"map must pluck name from *ordered.Map items through Liquid engine")
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

	// ── Plugin filter rewriting correctness (issue #362) ────────────
	// Verifies that rewriteFilterToPlugin produces correct template source
	// regardless of whether regex patterns are compiled per-call or cached.
	// These tests establish the behavioral contract that must hold after
	// the developer caches compiled patterns on the liquidEngine struct.

	Describe("Plugin filter rewriting correctness (issue #362)", func() {
		It("novel filter without args is rewritten to plugin_filter bridge", func() {
			engine := tmpl.NewLiquidEngine()
			err := engine.AddFilter("shout", func(input interface{}, args ...interface{}) interface{} {
				return fmt.Sprintf("!%v!", input)
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{{ name | shout }}`))
			Expect(err).NotTo(HaveOccurred())
			out, err := tpl.Render(map[string]interface{}{"name": "hello"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("!hello!"),
				"novel filter without args must route through plugin_filter bridge")
		})

		It("novel filter with args is rewritten preserving arguments", func() {
			engine := tmpl.NewLiquidEngine()
			err := engine.AddFilter("wrap", func(input interface{}, args ...interface{}) interface{} {
				if len(args) > 0 {
					return fmt.Sprintf("%v%v%v", args[0], input, args[0])
				}
				return input
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{{ name | wrap: "*" }}`))
			Expect(err).NotTo(HaveOccurred())
			out, err := tpl.Render(map[string]interface{}{"name": "bold"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("*bold*"),
				"novel filter with args must route through plugin_filter bridge with args intact")
		})

		It("multiple novel filters in the same template are all rewritten correctly", func() {
			engine := tmpl.NewLiquidEngine()
			err := engine.AddFilter("exclaim", func(input interface{}, args ...interface{}) interface{} {
				return fmt.Sprintf("%v!", input)
			})
			Expect(err).NotTo(HaveOccurred())
			err = engine.AddFilter("question", func(input interface{}, args ...interface{}) interface{} {
				return fmt.Sprintf("%v?", input)
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{{ a | exclaim }} {{ b | question }}`))
			Expect(err).NotTo(HaveOccurred())
			out, err := tpl.Render(map[string]interface{}{"a": "yes", "b": "really"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("yes! really?"),
				"multiple novel filters in one template must all be rewritten correctly")
		})

		It("filter registration order does not affect rewriting correctness", func() {
			// Register filters in two different orders, parse the same template,
			// and verify identical output.
			for _, order := range [][]string{{"alpha", "beta", "gamma"}, {"gamma", "alpha", "beta"}} {
				engine := tmpl.NewLiquidEngine()
				for _, name := range order {
					n := name
					err := engine.AddFilter(n, func(input interface{}, args ...interface{}) interface{} {
						return fmt.Sprintf("[%s:%v]", n, input)
					})
					Expect(err).NotTo(HaveOccurred())
				}

				tpl, err := engine.Parse("test", []byte(`{{ x | alpha }}{{ x | beta }}{{ x | gamma }}`))
				Expect(err).NotTo(HaveOccurred())
				out, err := tpl.Render(map[string]interface{}{"x": "v"})
				Expect(err).NotTo(HaveOccurred())
				Expect(string(out)).To(Equal("[alpha:v][beta:v][gamma:v]"),
					"filter registration order must not affect rewriting — "+
						"cached patterns keyed by name must produce identical output regardless of order")
			}
		})

		It("parsing the same filter across multiple templates produces consistent results", func() {
			engine := tmpl.NewLiquidEngine()
			err := engine.AddFilter("tag", func(input interface{}, args ...interface{}) interface{} {
				return fmt.Sprintf("<%v>", input)
			})
			Expect(err).NotTo(HaveOccurred())

			for i := 0; i < 10; i++ {
				tpl, err := engine.Parse(fmt.Sprintf("page-%d", i), []byte(`{{ name | tag }}`))
				Expect(err).NotTo(HaveOccurred())
				out, err := tpl.Render(map[string]interface{}{"name": fmt.Sprintf("p%d", i)})
				Expect(err).NotTo(HaveOccurred())
				Expect(string(out)).To(Equal(fmt.Sprintf("<p%d>", i)),
					"cached pattern must produce correct output on every Parse call, not just the first")
			}
		})
	})

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
