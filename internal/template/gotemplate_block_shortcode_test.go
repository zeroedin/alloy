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

	// ── Depth guard: callback output containing shortcode syntax (issue #1009) ──

	Describe("depth guard for recursive shortcode output", func() {
		It("returns an error when callback output produces infinite shortcode expansion", func() {
			// A callback that returns output containing a new block shortcode
			// of the same name — this creates an infinite expansion loop.
			// The maxShortcodeIterations depth guard must catch this.
			callback := func(name string, args []string, content string) (string, error) {
				// Output contains another block shortcode — will recurse
				return `{{% recursive %}}` + content + `{{% /recursive %}}`, nil
			}

			input := []byte("{{% recursive %}}seed content{{% /recursive %}}")
			_, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).To(HaveOccurred(),
				"callback output that generates new block shortcodes must "+
					"trigger the iteration depth guard to prevent infinite loops")
			Expect(err.Error()).To(ContainSubstring("iteration"),
				"error message must indicate the iteration limit was exceeded")
		})

		It("terminates when callback output contains non-matching {{% markers", func() {
			// A callback returns output that contains `{{% ` but does NOT form
			// a valid opening tag (no closing `%}}`). Processing must terminate
			// normally without infinite looping.
			callCount := 0
			callback := func(name string, args []string, content string) (string, error) {
				callCount++
				return "<div>See {{% for syntax info</div>", nil
			}

			input := []byte(`{{% widget %}}content{{% /widget %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred(),
				"callback output with incomplete {{% markers (no closing %%}}) "+
					"must not cause infinite iteration — the markers don't match "+
					"the opening tag regex so processing terminates normally")
			Expect(callCount).To(Equal(1),
				"callback must be invoked exactly once")
			Expect(string(result)).To(ContainSubstring("See {{% for syntax info"),
				"non-shortcode {{% markers in callback output must be preserved as-is")
		})
	})

	// ── Interleaved mismatched tags (issue #1009) ─────────────────────

	Describe("interleaved mismatched tags", func() {
		It("returns an error for interleaved tags {{% a %}}{{% b %}}{{% /a %}}{{% /b %}}", func() {
			callback := func(name string, args []string, content string) (string, error) {
				return "<div>" + content + "</div>", nil
			}

			// Interleaved: a opens, b opens, a closes (mismatch — b is on top of stack)
			input := []byte("{{% alpha %}}{{% beta %}}{{% /alpha %}}{{% /beta %}}")
			_, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).To(HaveOccurred(),
				"interleaved mismatched tags must produce an error — "+
					"the nearest opening tag before {{% /alpha %}} is {{% beta %}}, not {{% alpha %}}")
			Expect(err.Error()).To(ContainSubstring("beta"),
				"error message must reference the opening tag name that was actually found (beta)")
			Expect(err.Error()).To(ContainSubstring("alpha"),
				"error message must reference the closing tag name (alpha)")
		})
	})

	// ── Split code region exclusion (issue #1009) ─────────────────────

	Describe("split code region exclusion", func() {
		It("treats opening tag inside <code> as literal even if closing tag is outside", func() {
			callback := func(name string, args []string, content string) (string, error) {
				Fail("callback must not be invoked when the opening tag is inside a code region")
				return "", nil
			}

			// Opening tag is inside <code>, closing tag is outside —
			// the opening tag must be treated as literal text (skipped
			// by the code region check). The closing tag alone becomes
			// an unexpected closing tag error.
			input := []byte(`<code>{{% widget %}}</code>some content{{% /widget %}}`)
			_, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).To(HaveOccurred(),
				"when the opening tag is inside <code>, it's literal text — "+
					"the closing tag outside has no matching opening tag")
			Expect(err.Error()).To(ContainSubstring("widget"),
				"error message must reference the orphaned closing tag name")
		})
	})

	// ── Escaped quotes in arguments (issue #1008) ─────────────────────

	Describe("escaped quotes in arguments", func() {
		It("parses escaped double quotes inside quoted arguments", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div>" + content + "</div>", nil
			}

			// Backslash-escaped quotes inside a quoted argument
			input := []byte(`{{% tag "she said \"hello\"" %}}body{{% /tag %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred(),
				"escaped quotes inside quoted arguments must be parsed correctly")
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Args).To(Equal([]string{`she said "hello"`}),
				"backslash-escaped double quotes must be unescaped in the extracted argument — "+
					"the argument regex must handle \\\" within quoted strings")
			Expect(calls[0].Content).To(Equal("body"))
			Expect(string(result)).To(Equal("<div>body</div>"))
		})

		It("parses escaped backslashes inside quoted arguments", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div>" + content + "</div>", nil
			}

			// Double backslash represents a literal backslash
			input := []byte(`{{% tag "path\\to\\file" %}}body{{% /tag %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred(),
				"escaped backslashes inside quoted arguments must be parsed correctly")
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Args).To(Equal([]string{`path\to\file`}),
				"double backslash must be unescaped to a single backslash in the extracted argument")
			Expect(string(result)).To(Equal("<div>body</div>"))
		})

		It("handles mix of escaped and unescaped quotes across multiple arguments", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div>" + content + "</div>", nil
			}

			input := []byte(`{{% tag "normal" "has \"quotes\"" "also normal" %}}body{{% /tag %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred(),
				"mixed escaped and unescaped arguments must parse correctly")
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Args).To(Equal([]string{"normal", `has "quotes"`, "also normal"}),
				"each argument must be independently parsed with escape handling")
			Expect(string(result)).To(Equal("<div>body</div>"))
		})

		It("does not break existing unescaped argument parsing", func() {
			var calls []shortcodeCall
			callback := func(name string, args []string, content string) (string, error) {
				calls = append(calls, shortcodeCall{Name: name, Args: args, Content: content})
				return "<div>" + content + "</div>", nil
			}

			// Standard arguments without escapes — must still work
			input := []byte(`{{% tag "simple" "args" %}}body{{% /tag %}}`)
			result, err := tmpl.ProcessBlockShortcodes(input, callback)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(HaveLen(1))
			Expect(calls[0].Args).To(Equal([]string{"simple", "args"}),
				"existing unescaped argument parsing must not regress")
			Expect(string(result)).To(Equal("<div>body</div>"))
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
