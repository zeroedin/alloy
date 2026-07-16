package template_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── Shortcode calling convention (issue #981) ─────────────────────────
// Specifies the cross-engine contract for shortcode argument resolution
// and empty-return behavior. Both engines must produce identical results
// for the same logical operation.

var _ = Describe("Shortcode calling convention (issue #981)", func() {

	// ── 1. Variable argument resolution in Liquid ─────────────────────
	// Unquoted args in Liquid shortcodes must resolve as Liquid expressions
	// against the template context. Quoted args remain literal strings.
	// This matches Go template behavior where {{ func .page.field }}
	// naturally resolves .page.field before calling the function.

	Describe("Liquid variable argument resolution", func() {

		It("resolves an unquoted arg as a context variable", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("embed", func(args []string, content string) string {
				capturedArgs = args
				return "<embed>" + args[0] + "</embed>"
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% embed videoId %}`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{
				"videoId": "dQw4w9WgXcQ",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(1))
			Expect(capturedArgs[0]).To(Equal("dQw4w9WgXcQ"),
				"unquoted arg must resolve to the context variable value — "+
					"not the literal string 'videoId'")
			Expect(string(out)).To(ContainSubstring("<embed>dQw4w9WgXcQ</embed>"))
		})

		It("resolves a nested dotted variable path", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("youtube", func(args []string, content string) string {
				capturedArgs = args
				return "<iframe>" + args[0] + "</iframe>"
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% youtube page.videoId %}`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{
				"page": map[string]interface{}{
					"videoId": "abc123",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(1))
			Expect(capturedArgs[0]).To(Equal("abc123"),
				"dotted path (page.videoId) must resolve against nested context — "+
					"not pass 'page.videoId' as a literal string")
			Expect(string(out)).To(ContainSubstring("<iframe>abc123</iframe>"))
		})

		It("keeps quoted args as literal strings even when a matching variable exists", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("show", func(args []string, content string) string {
				capturedArgs = args
				return args[0]
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% show "videoId" %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"videoId": "resolved-value",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(1))
			Expect(capturedArgs[0]).To(Equal("videoId"),
				"quoted arg must remain a literal string — "+
					"quotes signal 'this is a string value, not a variable reference'")
		})

		It("handles mixed quoted and unquoted args", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("card", func(args []string, content string) string {
				capturedArgs = args
				return "<card>" + fmt.Sprintf("%v", args) + "</card>"
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% card "primary" page.size %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"page": map[string]interface{}{
					"size": "large",
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(2))
			Expect(capturedArgs[0]).To(Equal("primary"),
				"first arg (quoted) must remain literal")
			Expect(capturedArgs[1]).To(Equal("large"),
				"second arg (unquoted) must resolve from context")
		})

		It("falls back to literal string when unquoted arg does not resolve", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("tag", func(args []string, content string) string {
				capturedArgs = args
				return args[0]
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% tag nonexistent %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(1))
			Expect(capturedArgs[0]).To(Equal("nonexistent"),
				"unquoted arg that doesn't match any context variable must "+
					"fall back to literal string — backward compatible with existing behavior")
		})

		It("converts non-string variable values to strings", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("display", func(args []string, content string) string {
				capturedArgs = args
				return args[0]
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% display count %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"count": 42,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(1))
			Expect(capturedArgs[0]).To(Equal("42"),
				"non-string values must be converted to strings via fmt.Sprint — "+
					"TagFunc signature accepts []string, so all values must be stringified")
		})

		It("resolves variable args in block shortcodes", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			var capturedContent string
			err := engine.AddTag("callout", func(args []string, content string) string {
				capturedArgs = args
				capturedContent = content
				return "<div class=\"" + args[0] + "\">" + content + "</div>"
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(
				`{% callout level %}Important message.{% endcallout %}`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{
				"level": "warning",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(1))
			Expect(capturedArgs[0]).To(Equal("warning"),
				"block shortcode unquoted arg must resolve from context")
			Expect(capturedContent).To(Equal("Important message."),
				"block content must still be captured correctly")
			Expect(string(out)).To(ContainSubstring(`class="warning"`))
		})

		// Disambiguation test: proves resolution actually happens by changing
		// context values and observing different shortcode output.
		// Uses a single Parse, double Render to prove resolution is per-render,
		// not per-parse (a parse-time caching implementation would fail this).
		It("produces different output for different context values (disambiguation)", func() {
			engine := tmpl.NewLiquidEngine()

			err := engine.AddTag("greet", func(args []string, content string) string {
				return "Hello, " + args[0] + "!"
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% greet name %}`))
			Expect(err).NotTo(HaveOccurred())

			out1, err := tpl.Render(map[string]interface{}{"name": "Alice"})
			Expect(err).NotTo(HaveOccurred())
			out2, err := tpl.Render(map[string]interface{}{"name": "Bob"})
			Expect(err).NotTo(HaveOccurred())

			Expect(string(out1)).To(Equal("Hello, Alice!"),
				"shortcode output must reflect resolved variable value")
			Expect(string(out2)).To(Equal("Hello, Bob!"),
				"different context value must produce different output — "+
					"if both produce 'Hello, name!' then resolution is not happening")
			Expect(string(out1)).NotTo(Equal(string(out2)),
				"outputs must differ — proves args are resolved per-render, not cached as literals")
		})

		It("resolves multiple unquoted args without any quoted args", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("pair", func(args []string, content string) string {
				capturedArgs = args
				return args[0] + "=" + args[1]
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% pair key value %}`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{
				"key":   "color",
				"value": "blue",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(2))
			Expect(capturedArgs[0]).To(Equal("color"),
				"first unquoted arg must resolve from context")
			Expect(capturedArgs[1]).To(Equal("blue"),
				"second unquoted arg must resolve from context")
			Expect(string(out)).To(Equal("color=blue"))
		})

		It("converts bool and float variable values to strings", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("show", func(args []string, content string) string {
				capturedArgs = args
				return fmt.Sprintf("%s|%s", args[0], args[1])
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% show flag score %}`))
			Expect(err).NotTo(HaveOccurred())

			_, err = tpl.Render(map[string]interface{}{
				"flag":  true,
				"score": 3.14,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(capturedArgs).To(HaveLen(2))
			Expect(capturedArgs[0]).To(Equal("true"),
				"bool values must be converted to string via fmt.Sprint")
			Expect(capturedArgs[1]).To(HavePrefix("3.14"),
				"float values must be converted to string via fmt.Sprint")
		})

		It("distinguishes empty-string variable from non-existent variable", func() {
			engine := tmpl.NewLiquidEngine()

			var capturedArgs []string
			err := engine.AddTag("check", func(args []string, content string) string {
				capturedArgs = args
				return args[0]
			})
			Expect(err).NotTo(HaveOccurred())

			// Empty string variable — should resolve to ""
			tpl1, err := engine.Parse("test1", []byte(`{% check emptyVar %}`))
			Expect(err).NotTo(HaveOccurred())
			_, err = tpl1.Render(map[string]interface{}{
				"emptyVar": "",
			})
			Expect(err).NotTo(HaveOccurred())
			emptyResult := capturedArgs[0]

			// Non-existent variable — should fall back to literal "missing"
			tpl2, err := engine.Parse("test2", []byte(`{% check missing %}`))
			Expect(err).NotTo(HaveOccurred())
			_, err = tpl2.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			missingResult := capturedArgs[0]

			Expect(emptyResult).To(Equal(""),
				"variable set to empty string must resolve to empty string, not literal 'emptyVar'")
			Expect(missingResult).To(Equal("missing"),
				"non-existent variable must fall back to the literal token string")
			Expect(emptyResult).NotTo(Equal(missingResult),
				"empty-string variable and non-existent variable must produce different results — "+
					"if both return the same value, the implementation conflates the two cases")
		})
	})

	// ── 2. Empty return behavior ──────────────────────────────────────
	// When a shortcode callback returns "", both engines must emit nothing.
	// No <alloy-shortcode> placeholder, no wrapper element, just empty output.

	Describe("Empty return behavior", func() {

		It("Liquid inline shortcode returning empty string emits nothing", func() {
			engine := tmpl.NewLiquidEngine()

			err := engine.AddTag("noop", func(args []string, content string) string {
				return ""
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`before{% noop %}after`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("beforeafter"),
				"empty-returning inline shortcode must produce no output between surrounding content")
			Expect(string(out)).NotTo(ContainSubstring("alloy-shortcode"),
				"<alloy-shortcode> placeholder must not appear in output — "+
					"it leaks internal implementation details into production HTML")
			Expect(string(out)).NotTo(ContainSubstring("<alloy-"),
				"no alloy-prefixed custom elements should appear in rendered output")
		})

		It("Liquid block shortcode returning empty string emits nothing", func() {
			engine := tmpl.NewLiquidEngine()

			err := engine.AddTag("optional", func(args []string, content string) string {
				return ""
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(
				`before{% optional %}inner content{% endoptional %}after`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("beforeafter"),
				"empty-returning block shortcode must produce no output")
			Expect(string(out)).NotTo(ContainSubstring("alloy-shortcode"),
				"<alloy-shortcode> placeholder must not appear in output")
			Expect(string(out)).NotTo(ContainSubstring("inner content"),
				"block inner content must not leak when shortcode returns empty")
		})

		It("Go engine inline shortcode returning empty string emits nothing", func() {
			engine := tmpl.NewGoEngine()

			err := engine.AddTag("noop", func(args []string, content string) string {
				return ""
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`before{{ noop }}after`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("beforeafter"),
				"Go engine empty-returning shortcode must produce no output — "+
					"establishes baseline for cross-engine consistency")
		})

		It("both engines produce identical empty output (cross-engine parity)", func() {
			liquidEngine := tmpl.NewLiquidEngine()
			goEngine := tmpl.NewGoEngine()

			shortcodeFn := func(args []string, content string) string {
				return ""
			}

			err := liquidEngine.AddTag("empty", shortcodeFn)
			Expect(err).NotTo(HaveOccurred())
			err = goEngine.AddTag("empty", shortcodeFn)
			Expect(err).NotTo(HaveOccurred())

			liquidTpl, err := liquidEngine.Parse("test", []byte(`X{% empty %}Y`))
			Expect(err).NotTo(HaveOccurred())
			goTpl, err := goEngine.Parse("test", []byte(`X{{ empty }}Y`))
			Expect(err).NotTo(HaveOccurred())

			liquidOut, err := liquidTpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			goOut, err := goTpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())

			Expect(string(liquidOut)).To(Equal(string(goOut)),
				"Liquid and Go engines must produce identical output when shortcode returns empty — "+
					"cross-engine parity is the core contract of the calling convention")
			Expect(string(liquidOut)).To(Equal("XY"),
				"both engines: empty shortcode produces no output between X and Y")
		})

		It("Liquid shortcode returning non-empty string still renders normally", func() {
			engine := tmpl.NewLiquidEngine()

			err := engine.AddTag("hi", func(args []string, content string) string {
				return "HELLO"
			})
			Expect(err).NotTo(HaveOccurred())

			tpl, err := engine.Parse("test", []byte(`{% hi %}`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("HELLO"),
				"non-empty return must render the shortcode output as before — "+
					"the empty-return fix must not break normal shortcode rendering")
		})
	})
})
