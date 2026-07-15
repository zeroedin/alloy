package template_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── Go template block shortcode preprocessor (issue #1002) ──────────
// Go template content uses {{% tag "args" %}}...{{% /tag %}} for block
// shortcodes. A preprocessor scans post-Goldmark HTML for these paired
// delimiters, extracts inner content, calls the shortcode plugin, and
// replaces the block with the plugin's output. This runs after Goldmark
// (inner content is already HTML) and before Go template rendering.

var _ = Describe("Go template block shortcode preprocessor", func() {

	// Mock shortcode callback that records calls and returns predictable output
	type shortcodeCall struct {
		Name    string
		Args    []string
		Content string
	}

	// ── Basic block shortcode extraction ──────────────────────────────

	Describe("basic block shortcode extraction", func() {
		It("extracts a block shortcode and passes args and content to the callback", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div class=\"callout callout--" + args[0] + "\">" + content + "</div>", nil
			}

			input := []byte(`{{% callout "warning" %}}Watch out!{{% /callout %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(1), "callback must be invoked exactly once")
			Expect(calls[0].Name).To(Equal("callout"),
				"shortcode name must be extracted from the opening tag")
			Expect(calls[0].Args).To(Equal([]string{"warning"}),
				"quoted arguments must be extracted from the opening tag")
			Expect(calls[0].Content).To(Equal("Watch out!"),
				"inner content between tags must be passed as content")
			Expect(string(result)).To(ContainSubstring(`callout--warning`),
				"output must contain the callback's rendered result")
			Expect(string(result)).NotTo(ContainSubstring("{{% callout"),
				"raw opening tag must not appear in output after processing")
			Expect(string(result)).NotTo(ContainSubstring("{{% /callout"),
				"raw closing tag must not appear in output after processing")
		})

		It("passes multiple quoted arguments correctly", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div>" + content + "</div>", nil
			}

			input := []byte(`{{% card "primary" "large" "dismissible" %}}Card body.{{% /card %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Args).To(Equal([]string{"primary", "large", "dismissible"}),
				"all quoted arguments must be extracted in order")
			Expect(calls[0].Content).To(Equal("Card body."))
			Expect(string(result)).To(Equal("<div>Card body.</div>"))
		})

		It("handles a block shortcode with no arguments", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<aside>" + content + "</aside>", nil
			}

			input := []byte("{{% sidebar %}}Side content.{{% /sidebar %}}")
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Name).To(Equal("sidebar"))
			Expect(calls[0].Args).To(BeEmpty(),
				"a tag with no quoted arguments must pass an empty args slice")
			Expect(calls[0].Content).To(Equal("Side content."))
			Expect(string(result)).To(Equal("<aside>Side content.</aside>"))
		})

		It("handles empty inner content", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<hr/>", nil
			}

			input := []byte("{{% divider %}}{{% /divider %}}")
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Content).To(Equal(""),
				"empty inner content must be passed as empty string")
			Expect(string(result)).To(Equal("<hr/>"))
		})
	})

	// ── Markdown-rendered inner content ───────────────────────────────

	Describe("Markdown-rendered inner content", func() {
		It("passes HTML-rendered inner content from Goldmark output", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div class=\"note\">" + content + "</div>", nil
			}

			// Simulates post-Goldmark output where Markdown has been rendered
			input := []byte("{{% note %}}\n<p>This is <strong>bold</strong> and a <a href=\"/\">link</a>.</p>\n{{% /note %}}")
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Content).To(ContainSubstring("<strong>bold</strong>"),
				"HTML-rendered content from Goldmark must be passed through as content")
			Expect(calls[0].Content).To(ContainSubstring(`<a href="/">link</a>`),
				"rendered links must be present in content")
			Expect(string(result)).To(ContainSubstring("<strong>bold</strong>"),
				"rendered HTML must appear in the final output")
		})
	})

	// ── Multiple block shortcodes in sequence ─────────────────────────

	Describe("multiple block shortcodes", func() {
		It("processes multiple sequential block shortcodes independently", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<section>" + content + "</section>", nil
			}

			input := []byte(
				`<p>Before.</p>` +
					"\n{{% alpha %}}First.{{% /alpha %}}\n" +
					`<p>Between.</p>` +
					"\n{{% beta %}}Second.{{% /beta %}}\n" +
					`<p>After.</p>`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(2), "both block shortcodes must be processed")
			Expect(calls[0].Name).To(Equal("alpha"))
			Expect(calls[0].Content).To(Equal("First."))
			Expect(calls[1].Name).To(Equal("beta"))
			Expect(calls[1].Content).To(Equal("Second."))

			output := string(result)
			Expect(output).To(ContainSubstring("<p>Before.</p>"),
				"content before shortcodes must be preserved")
			Expect(output).To(ContainSubstring("<section>First.</section>"),
				"first shortcode output must be present")
			Expect(output).To(ContainSubstring("<p>Between.</p>"),
				"content between shortcodes must be preserved")
			Expect(output).To(ContainSubstring("<section>Second.</section>"),
				"second shortcode output must be present")
			Expect(output).To(ContainSubstring("<p>After.</p>"),
				"content after shortcodes must be preserved")
		})
	})

	// ── Nesting ───────────────────────────────────────────────────────

	Describe("nested block shortcodes", func() {
		It("resolves nested block shortcodes from innermost to outermost", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div class=\"" + name + "\">" + content + "</div>", nil
			}

			input := []byte(
				`{{% outer "a" %}}` +
					`Before inner. ` +
					`{{% inner "b" %}}Nested content.{{% /inner %}}` +
					` After inner.` +
					`{{% /outer %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(2),
				"both inner and outer shortcodes must be processed")

			// Inner must be processed first
			Expect(calls[0].Name).To(Equal("inner"),
				"innermost shortcode must be processed first")
			Expect(calls[0].Args).To(Equal([]string{"b"}))
			Expect(calls[0].Content).To(Equal("Nested content."))

			// Outer receives the rendered inner shortcode in its content
			Expect(calls[1].Name).To(Equal("outer"),
				"outermost shortcode must be processed second")
			Expect(calls[1].Args).To(Equal([]string{"a"}))
			Expect(calls[1].Content).To(ContainSubstring(`<div class="inner">Nested content.</div>`),
				"outer shortcode content must include the rendered inner shortcode output")

			output := string(result)
			Expect(output).To(Equal(
				`<div class="outer">Before inner. <div class="inner">Nested content.</div> After inner.</div>`),
				"final output must contain fully nested rendered shortcodes")
		})

		It("resolves same-name nested block shortcodes via depth tracking", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				level := "default"
				if len(args) > 0 {
					level = args[0]
				}
				return "<div class=\"box-" + level + "\">" + content + "</div>", nil
			}

			input := []byte(
				`{{% box "outer" %}}` +
					`{{% box "inner" %}}Deep content.{{% /box %}}` +
					`{{% /box %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(2),
				"both same-name nested shortcodes must be processed")
			Expect(calls[0].Name).To(Equal("box"))
			Expect(calls[0].Args).To(Equal([]string{"inner"}),
				"inner box must be processed first")
			Expect(calls[1].Args).To(Equal([]string{"outer"}),
				"outer box must be processed second")

			output := string(result)
			Expect(output).To(Equal(
				`<div class="box-outer"><div class="box-inner">Deep content.</div></div>`),
				"same-name nested shortcodes must resolve correctly via depth tracking")
		})
	})

	// ── Error handling ────────────────────────────────────────────────

	Describe("error handling", func() {
		It("returns an error for an unclosed block shortcode", func() {
			callback := func(name string, args []string, content string) (string, error) {
				return "<div>" + content + "</div>", nil
			}

			input := []byte("{{% callout %}}This is never closed.")
			_, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).To(HaveOccurred(),
				"unclosed block shortcode must produce a build error")
			Expect(err.Error()).To(ContainSubstring("callout"),
				"error message must include the unclosed tag name")
		})

		It("returns an error for mismatched closing tag name", func() {
			callback := func(name string, args []string, content string) (string, error) {
				return "<div>" + content + "</div>", nil
			}

			input := []byte("{{% callout %}}Content.{{% /sidebar %}}")
			_, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).To(HaveOccurred(),
				"mismatched closing tag must produce an error")
			Expect(err.Error()).To(ContainSubstring("callout"),
				"error message must reference the opening tag name")
		})

		It("returns an error when shortcode callback fails", func() {
			callback := func(name string, args []string, content string) (string, error) {
				return "", fmt.Errorf("plugin error: shortcode %q crashed", name)
			}

			input := []byte("{{% broken %}}Content.{{% /broken %}}")
			_, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).To(HaveOccurred(),
				"shortcode callback error must propagate as a build error")
			Expect(err.Error()).To(ContainSubstring("broken"),
				"error must include the shortcode name")
		})
	})

	// ── Content without block shortcodes ──────────────────────────────

	Describe("passthrough behavior", func() {
		It("returns content unchanged when no {{% %}} tags are present", func() {
			callback := func(name string, args []string, content string) (string, error) {
				Fail("callback must not be invoked when no block shortcodes are present")
				return "", nil
			}

			input := []byte("<h1>Hello</h1>\n<p>No shortcodes here.</p>\n{{ page.title }}")
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal(string(input)),
				"content without {{% %}} tags must pass through unchanged")
		})

		It("does not process Liquid-style {% %} tags", func() {
			callback := func(name string, args []string, content string) (string, error) {
				Fail("callback must not be invoked for Liquid-style tags")
				return "", nil
			}

			input := []byte("{% callout %}Liquid block.{% endcallout %}")
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal(string(input)),
				"Liquid-style {% %} tags must not be processed by the Go template preprocessor")
		})

		It("does not process Go template {{ }} expression tags", func() {
			callback := func(name string, args []string, content string) (string, error) {
				Fail("callback must not be invoked for expression tags")
				return "", nil
			}

			input := []byte(`{{ youtube "abc" }}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal(string(input)),
				"Go template {{ }} expression tags must not be processed by the block preprocessor")
		})

		It("does not process {{% %}} delimiters inside <pre> elements", func() {
			callback := func(name string, args []string, content string) (string, error) {
				Fail("callback must not be invoked for delimiters inside code blocks")
				return "", nil
			}

			input := []byte(`<p>Before.</p>
<pre><code>{{% callout "warning" %}}
example content
{{% /callout %}}</code></pre>
<p>After.</p>`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal(string(input)),
				"{{% %}} delimiters inside <pre>/<code> elements are literal text, not shortcode invocations")
		})

		It("does not process {{% %}} delimiters inside inline <code> elements", func() {
			callback := func(name string, args []string, content string) (string, error) {
				Fail("callback must not be invoked for delimiters inside inline code")
				return "", nil
			}

			input := []byte(`<p>Use <code>{{% callout %}}</code> for block shortcodes.</p>`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal(string(input)),
				"{{% %}} delimiters inside <code> elements are literal text, not shortcode invocations")
		})
	})
})
