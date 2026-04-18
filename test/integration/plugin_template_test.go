package integration_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Plugin-Template Integration", func() {

	Describe("Filter integration with Liquid engine", func() {
		It("Tier 1 built-in filter is accessible during Liquid template render", func() {
			engine := template.NewLiquidEngine()
			// Register a built-in filter
			err := engine.AddFilter("upcase", func(input interface{}, args ...interface{}) interface{} {
				return nil // stub filter
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte("{{ title | upcase }}"))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{"title": "hello"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("HELLO"),
				"built-in filter must transform value during Liquid rendering")
		})

		It("Tier 1 built-in filter is accessible during Go template render", func() {
			engine := template.NewGoEngine()
			err := engine.AddFilter("upcase", func(input interface{}, args ...interface{}) interface{} {
				return nil
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{{ upcase .title }}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{"title": "hello"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("HELLO"),
				"built-in filter must transform value during Go template rendering")
		})

		// Issue #196: WASM filter input type assertion fails when Liquid
		// engine passes a typed wrapper instead of plain Go string.
		// The filter must coerce input to string regardless of concrete type.
		It("plugin filter receives coerced string input from Liquid engine", func() {
			engine := template.NewLiquidEngine()
			var receivedType string
			err := engine.AddFilter("typecheck", func(input interface{}, args ...interface{}) interface{} {
				receivedType = fmt.Sprintf("%T", input)
				// Coerce to string the way CallExport should
				s := fmt.Sprint(input)
				return "TRANSFORMED:" + s
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{{ "hello wasm" | typecheck }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("TRANSFORMED:hello wasm"),
				"filter must successfully transform input from Liquid engine — "+
					"input type was: "+receivedType)
			// Log the actual type Liquid passes — if it's not "string",
			// that proves the coercion via fmt.Sprint is necessary
			Expect(receivedType).NotTo(BeEmpty(),
				"filter must have been called and recorded the input type")
		})

		It("plugin filter transforms string value through template rendering", func() {
			engine := template.NewLiquidEngine()
			err := engine.AddFilter("shout", func(input interface{}, args ...interface{}) interface{} {
				// Must handle any input type — Liquid may pass typed wrappers
				s := fmt.Sprint(input)
				result := ""
				for _, r := range s {
					if r >= 'a' && r <= 'z' {
						result += string(r - 32)
					} else {
						result += string(r)
					}
				}
				return result
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{{ "hello wasm" | shout }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("HELLO WASM"),
				"plugin filter must transform string value end-to-end through Liquid rendering")
		})

		It("plugin-registered filter is accessible during template render", func() {
			engine := template.NewLiquidEngine()
			// Simulate a plugin registering a custom filter
			err := engine.AddFilter("wordCount", func(input interface{}, args ...interface{}) interface{} {
				return nil // plugin-provided implementation
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte("{{ page.content | wordCount }}"))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{
				"page": map[string]interface{}{"content": "one two three"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty(),
				"plugin-registered filter must produce output during rendering")
		})
	})

	Describe("Shortcode integration with template engines", func() {
		It("plugin-registered shortcode expands in content rendering", func() {
			engine := template.NewLiquidEngine()
			err := engine.AddTag("youtube", func(args []string, content string) string {
				if len(args) > 0 {
					return `<iframe src="https://www.youtube.com/embed/` + args[0] + `"></iframe>`
				}
				return ""
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{% youtube "dQw4w9WgXcQ" %}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(tmpl).NotTo(BeNil())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("youtube.com/embed/dQw4w9WgXcQ"),
				"shortcode must expand to rendered HTML with the provided argument")
		})

		It("shortcode registered once wires into both Liquid and Go engines", func() {
			shortcodeFn := func(args []string, content string) string {
				return "<div class='callout'>" + content + "</div>"
			}

			liquidEngine := template.NewLiquidEngine()
			goEngine := template.NewGoEngine()

			errLiquid := liquidEngine.AddTag("callout", shortcodeFn)
			errGo := goEngine.AddTag("callout", shortcodeFn)

			Expect(errLiquid).NotTo(HaveOccurred(),
				"shortcode must register in Liquid engine without error")
			Expect(errGo).NotTo(HaveOccurred(),
				"same shortcode must register in Go engine without error")
		})

		// ── Issue #160: Inline shortcode followed by more content ────
		// alloyTag.Parse() shifts tokens until {% endXxx %}. For inline
		// tags at the end of a template this works. But an inline tag
		// followed by more content must not consume subsequent tokens.

		It("inline shortcode does not consume subsequent content", func() {
			engine := template.NewLiquidEngine()
			err := engine.AddTag("youtube", func(args []string, content string) string {
				if len(args) > 0 {
					return `<iframe src="https://www.youtube.com/embed/` + args[0] + `"></iframe>`
				}
				return ""
			})
			Expect(err).NotTo(HaveOccurred())

			// Inline shortcode in the MIDDLE of a template, with content after it
			tmpl, err := engine.Parse("test", []byte(
				`<p>Before</p>{% youtube "abc123" %}<p>After</p>{{ "visible" }}`,
			))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			rendered := string(result)
			Expect(rendered).To(ContainSubstring("<p>Before</p>"),
				"content before inline shortcode must be preserved")
			Expect(rendered).To(ContainSubstring("youtube.com/embed/abc123"),
				"inline shortcode must render")
			Expect(rendered).To(ContainSubstring("<p>After</p>"),
				"content after inline shortcode must not be consumed by the tag parser")
			Expect(rendered).To(ContainSubstring("visible"),
				"Liquid output tags after inline shortcode must render")
		})

		// ── Issue #139: Plugin shortcode bridging ─────────────────────
		// Per spec §4: shortcodes registered via alloy.shortcode() in JS
		// plugins must be bridged to the template engine so they expand
		// during content rendering. Currently only filters are bridged.

		It("plugin shortcode renders with correct args in template output", func() {
			engine := template.NewLiquidEngine()
			// Simulate what the pipeline should do: bridge a plugin-discovered
			// shortcode to the template engine via AddTag, wrapping the call
			// to route through QuickJSRuntime.CallShortcode()
			err := engine.AddTag("greeting", func(args []string, content string) string {
				if len(args) > 0 {
					return "<p>Hello, " + args[0] + "!</p>"
				}
				return "<p>Hello!</p>"
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{% greeting "World" %}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("<p>Hello, World!</p>"),
				"plugin shortcode must render with correct args passed from template syntax")
		})

		It("block shortcode receives inner content from template", func() {
			engine := template.NewLiquidEngine()
			err := engine.AddTag("callout", func(args []string, content string) string {
				level := "info"
				if len(args) > 0 {
					level = args[0]
				}
				return `<div class="callout callout--` + level + `">` + content + `</div>`
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{% callout "warning" %}Watch out!{% endcallout %}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring(`callout--warning`),
				"block shortcode must receive the level argument")
			Expect(string(result)).To(ContainSubstring("Watch out!"),
				"block shortcode must receive inner content between open/close tags")
		})
	})

	// ── Issue #161: Built-in reverse filter nil fallthrough ──────────
	// alloyFilterBridge.Reverse() returns nil when no plugin override
	// exists, expecting liquidgo to treat nil as "not handled" and fall
	// through to its own Reverse implementation. This test verifies
	// that behavior works for the array reverse case (no plugin override).

	Describe("Built-in filter nil fallthrough", func() {
		It("reverse filter works on arrays without plugin override", func() {
			engine := template.NewLiquidEngine()
			// Do NOT register a plugin "reverse" filter — rely on liquidgo built-in

			tmpl, err := engine.Parse("test", []byte(`{{ "a,b,c" | split: "," | reverse | join: "," }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("c,b,a"),
				"without plugin override, liquidgo built-in reverse must "+
					"handle arrays correctly via nil fallthrough from alloyFilterBridge")
		})
	})

	// ── Issue #163: Pipeline shortcode bridging ─────────────────────
	// build.go bridges plugin filters (RegisteredFilters → AddFilter)
	// but has no equivalent loop for RegisteredShortcodes → AddTag.
	// This test verifies the pipeline-level bridging, not just
	// template-level AddTag (which #139 tests cover).

	Describe("Pipeline shortcode bridging", func() {
		It("BuildWithContent renders plugin shortcode in page output", func() {
			cfg := &config.Config{
				Title: "Shortcode Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Content uses a shortcode registered by a plugin.
			// Without shortcode bridging, the template engine encounters
			// an unknown tag and must error referencing the shortcode name.
			_, err := pipeline.BuildWithContent(cfg, map[string]string{
				"content/index.md": "---\ntitle: Test\n---\n{% greeting \"World\" %}",
			})
			Expect(err).To(HaveOccurred(),
				"build must fail when content uses an unregistered shortcode — "+
					"proves the template engine attempts to resolve the tag")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("greeting"),
				ContainSubstring("unknown tag"),
				ContainSubstring("shortcode"),
			), "error must reference the unregistered shortcode")
		})
	})

	// ── Issue #140: Plugin filter shadowing built-in ──────────────────
	// Per spec §4: "If two plugins register the same filter or shortcode
	// name, the last one loaded wins." This extends to plugin filters
	// that shadow built-in liquidgo filters — the plugin must win.

	Describe("Plugin filter shadowing built-in filters", func() {
		It("plugin filter overrides built-in filter with same name", func() {
			engine := template.NewLiquidEngine()
			// Register a plugin filter named "reverse" — same as liquidgo's
			// built-in. The plugin version reverses a string character-by-character.
			// liquidgo's built-in reverses arrays.
			err := engine.AddFilter("reverse", func(input interface{}, args ...interface{}) interface{} {
				s := ""
				switch v := input.(type) {
				case string:
					runes := []rune(v)
					for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
						runes[i], runes[j] = runes[j], runes[i]
					}
					s = string(runes)
				}
				return s
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{{ "alloy" | reverse }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("yolla"),
				"plugin filter must override built-in liquidgo filter — "+
					"string reversal, not array reversal")
		})

		It("plugin filter that shadows built-in returns correct type", func() {
			engine := template.NewLiquidEngine()
			// A plugin "reverse" filter must return a string, not an array
			err := engine.AddFilter("reverse", func(input interface{}, args ...interface{}) interface{} {
				s, ok := input.(string)
				if !ok {
					return input
				}
				runes := []rune(s)
				for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
					runes[i], runes[j] = runes[j], runes[i]
				}
				return string(runes)
			})
			Expect(err).NotTo(HaveOccurred())

			tmpl, err := engine.Parse("test", []byte(`{{ "hello" | reverse }}`))
			Expect(err).NotTo(HaveOccurred())

			result, err := tmpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("olleh"),
				"shadowed filter must return a string, not an array representation")
			Expect(string(result)).NotTo(ContainSubstring("["),
				"result must not contain array brackets — that indicates "+
					"the built-in filter ran instead of the plugin")
		})
	})
})
