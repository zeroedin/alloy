package content_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

// ── Raw block shortcode content (issue #1245) ─────────────────────────
//
// A `>` immediately after the opening delimiter on a block tag's OPEN tag
// (`{%> name %}` for Liquid, `{{%> name %}}` for Go templates) makes that
// invocation's body pass through Goldmark completely unparsed. Close tags
// keep each engine's native syntax and are unchanged.
//
// The signal lives on the call site, not the shortcode definition — the same
// shortcode is legitimately used raw on one page and Markdown-parsed on
// another. The `>` is an Alloy-level marker that never reaches the template
// engine: the Goldmark renderer strips it, so everything downstream
// (ProcessBlockShortcodes, Liquid's block-tag detection) sees native syntax.
//
// Spec: PLAN.md → "Raw block shortcode content (issue #1245)".

var _ = Describe("Raw block shortcode content (issue #1245)", func() {
	rawOpts := content.MarkdownOptions{
		Unsafe:        true,
		Typographer:   true,
		TemplateTags:  true,
		AutoHeadingID: true,
	}
	rawMD := content.CreateGoldmark(rawOpts)

	// ── Body is captured verbatim ──────────────────────────────────────

	Context("Verbatim body capture (Liquid)", func() {
		It("does not parse block-level Markdown inside a raw body", func() {
			source := []byte("{%> code %}\n# not a heading\n* not a list\n> not a quote\n{% endcode %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("# not a heading"),
				"a raw body must keep Markdown block syntax as literal text")
			Expect(html).To(ContainSubstring("* not a list"))
			Expect(html).To(ContainSubstring("> not a quote"))
			Expect(html).NotTo(ContainSubstring("<h1"),
				"headings must not be parsed inside a raw body")
			Expect(html).NotTo(ContainSubstring("<ul>"),
				"lists must not be parsed inside a raw body")
			Expect(html).NotTo(ContainSubstring("<blockquote>"),
				"blockquotes must not be parsed inside a raw body")
		})

		It("does not parse inline Markdown inside a raw body", func() {
			source := []byte("{%> code %}\n**not bold** and _not italic_ and `not code`\n{% endcode %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("**not bold** and _not italic_ and `not code`"),
				"inline Markdown must survive a raw body as literal text")
			Expect(html).NotTo(ContainSubstring("<strong>"),
				"emphasis must not be parsed inside a raw body")
			Expect(html).NotTo(ContainSubstring("<em>"))
			Expect(html).NotTo(ContainSubstring("<code>"))
		})

		It("does not HTML-escape a raw body", func() {
			source := []byte("{%> code %}\nif (a < 2 && b > 1) { go(\"x\"); }\n{% endcode %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`if (a < 2 && b > 1) { go("x"); }`),
				"a raw body must not be entity-escaped — the shortcode function "+
					"receives exactly the bytes the author wrote")
			Expect(html).NotTo(ContainSubstring("&lt;"),
				"< must not become &lt; inside a raw body")
			Expect(html).NotTo(ContainSubstring("&amp;"),
				"& must not become &amp; inside a raw body")
		})

		It("does not apply typographer substitutions inside a raw body", func() {
			source := []byte("{%> code %}\n\"quoted\" -- dashed ...\n{% endcode %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`"quoted" -- dashed ...`),
				"typographer must not smarten quotes, dashes, or ellipses in a raw body")
			Expect(html).NotTo(ContainSubstring("&ldquo;"))
			Expect(html).NotTo(ContainSubstring("&ndash;"))
			Expect(html).NotTo(ContainSubstring("&hellip;"))
		})

		It("preserves leading indentation without creating a code block", func() {
			source := []byte("{%> code %}\n    indented four spaces\n{% endcode %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("    indented four spaces"),
				"indentation must be preserved byte-for-byte in a raw body")
			Expect(html).NotTo(ContainSubstring("<pre><code>"),
				"an indented line in a raw body must not become an indented code block")
		})

		It("keeps a fenced code block literal inside a raw body", func() {
			source := []byte("{%> code %}\n```js\nconst a = 1;\n```\n{% endcode %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("```js"),
				"fence markers must stay literal inside a raw body")
			Expect(html).NotTo(ContainSubstring("<pre><code"),
				"a fenced code block inside a raw body must not be converted")
		})

		It("does not terminate the block at a blank line", func() {
			source := []byte("{%> code %}\nfirst\n\nsecond\n{% endcode %}\n\nAfter.\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("first"))
			Expect(html).To(ContainSubstring("second"))
			Expect(html).NotTo(ContainSubstring("<p>second</p>"),
				"a blank line must not end the raw block and resume Markdown parsing")
			Expect(html).To(ContainSubstring("<p>After.</p>"),
				"content after the close tag must resume normal Markdown parsing")
		})

		It("passes raw HTML through even when unsafe is false", func() {
			// The `>` modifier is an explicit per-call-site opt-in by the page
			// author and overrides goldmark.unsafe for that body only. Without
			// this, the motivating use case (feeding script/structured-data
			// content to a shortcode) is impossible under unsafe: false.
			safeMD := content.CreateGoldmark(content.MarkdownOptions{
				Unsafe: false, Typographer: true, TemplateTags: true,
			})
			source := []byte("{%> helmet %}\n<script>var a = 1;</script>\n{% endhelmet %}\n")
			out, _, err := content.RenderMarkdown(source, safeMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<script>var a = 1;</script>"),
				"a raw body must pass through verbatim regardless of the unsafe setting")
			Expect(html).NotTo(ContainSubstring("raw HTML omitted"),
				"unsafe: false must not strip HTML inside a raw block body")
		})

		It("keeps headings inside a raw body out of the table of contents", func() {
			source := []byte("{%> code %}\n## Not A Real Heading\n{% endcode %}\n")
			out, toc, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("## Not A Real Heading"))
			Expect(toc).To(BeEmpty(),
				"a raw body is never parsed, so its headings must not produce TOC entries")
		})
	})

	// ── The `>` marker is stripped before the engine sees it ───────────

	Context("Marker stripping (Liquid)", func() {
		It("strips the > from the open tag and leaves the close tag alone", func() {
			source := []byte("{%> helmet %}\nbody\n{% endhelmet %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{% helmet %}"),
				"the open tag must reach the template engine in native Liquid syntax")
			Expect(html).NotTo(ContainSubstring("{%>"),
				"the > marker is an Alloy-level signal and must never reach the engine")
			Expect(html).To(ContainSubstring("{% endhelmet %}"),
				"the close tag is native syntax already and must be emitted unchanged")
		})

		It("normalizes to a single space when the > is not followed by one", func() {
			source := []byte("{%>helmet %}\nbody\n{% endhelmet %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("{% helmet %}"),
				"stripping must leave exactly one space between delimiter and tag name")
		})

		It("preserves arguments on the open tag", func() {
			source := []byte("{%> callout \"warning\" %}\nbody\n{% endcallout %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(`{% callout "warning" %}`),
				"only the > is removed — arguments and quoting are untouched")
		})

		It("does not wrap the open tag in a paragraph", func() {
			source := []byte("{%> helmet %}\n# not a heading\n{% endhelmet %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).NotTo(ContainSubstring("<p>{% helmet"),
				"a raw block open tag is block-level and must not be wrapped in <p>")
			Expect(html).NotTo(ContainSubstring("<p></p>"),
				"a raw block must not leave empty paragraphs behind")
			Expect(html).NotTo(ContainSubstring("<h1"),
				"the body is still raw — block-level status must not come at the "+
					"cost of Markdown-parsing the body")
		})
	})

	// ── Depth counting ────────────────────────────────────────────────

	Context("Matching close tag (Liquid)", func() {
		It("depth-counts a balanced same-name pair inside the body", func() {
			source := []byte("{%> callout %}\nouter\n{% callout %}\ninner\n{% endcallout %}\nstill outer\n{% endcallout %}\n\nAfter.\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("still outer"),
				"a balanced nested pair must not close the outer raw block early")
			Expect(html).NotTo(ContainSubstring("<p>still outer</p>"),
				"content after the nested pair is still raw body, not Markdown")
			Expect(html).To(ContainSubstring("<p>After.</p>"),
				"the outer block must close at its own close tag")
		})

		It("counts a nested raw open tag toward depth", func() {
			source := []byte("{%> callout %}\nouter\n{%> callout %}\ninner\n{% endcallout %}\nstill outer\n{% endcallout %}\n\nAfter.\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("still outer"),
				"a nested {%> open tag increments depth just like a plain one")
			Expect(html).NotTo(ContainSubstring("<p>still outer</p>"),
				"content after the nested pair is still raw body, not Markdown")
			Expect(html).NotTo(ContainSubstring("<p>inner</p>"),
				"the nested block's body is part of the outer raw body")
			Expect(html).To(ContainSubstring("<p>After.</p>"))
		})

		It("ignores a close tag that shares its line with other text", func() {
			source := []byte("{%> helmet %}\nUse {% endhelmet %} to close.\ndone\n{% endhelmet %}\n\nAfter.\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("Use {% endhelmet %} to close."),
				"only own-line tags affect depth — an inline close tag is body text")
			Expect(html).NotTo(ContainSubstring("<p>Use {%"),
				"body lines are emitted verbatim, not wrapped as Markdown paragraphs")
			Expect(html).NotTo(ContainSubstring("<p>done</p>"),
				"the block must not have closed at the inline close tag")
			Expect(html).To(ContainSubstring("<p>After.</p>"))
		})

		It("does not let delimiter families cross", func() {
			source := []byte("{%> helmet %}\n{{% helmet %}}\n# not a heading\n{% endhelmet %}\n\nAfter.\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{{% helmet %}}"),
				"a Go template tag inside a Liquid raw block is body text, "+
					"not a depth-affecting open tag")
			Expect(html).To(ContainSubstring("# not a heading"))
			Expect(html).NotTo(ContainSubstring("<h1"),
				"the body after the foreign delimiter is still raw")
			Expect(html).To(ContainSubstring("<p>After.</p>"),
				"the Liquid close tag must still end the block")
		})

		It("counts whitespace-control close tags", func() {
			source := []byte("{%> helmet %}\n# not a heading\n{%- endhelmet -%}\n\nAfter.\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{%- endhelmet -%}"),
				"whitespace-control variants close a raw block and are emitted unchanged")
			Expect(html).To(ContainSubstring("# not a heading"))
			Expect(html).NotTo(ContainSubstring("<h1"),
				"the body before a whitespace-control close tag is still raw")
			Expect(html).To(ContainSubstring("<p>After.</p>"))
		})
	})

	// ── Block-only: the marker means nothing inline ───────────────────

	Context("Block-only recognition", func() {
		It("does not treat an inline {%> invocation as a raw block", func() {
			// There is no inline raw form. The marker is left in place so the
			// template engine fails loudly (Liquid: unknown tag) instead of
			// silently producing wrong output.
			source := []byte("{%> helmet %}inline body{% endhelmet %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{%>"),
				"an inline {%> is not a raw block, so the marker must not be stripped — "+
					"the engine's native parse error is the intended safety net")
			Expect(html).To(ContainSubstring("<p>"),
				"an inline invocation stays inline")
		})

		It("leaves a plain block shortcode's Markdown processing unchanged", func() {
			source := []byte("{% callout \"info\" %}\n# real heading\n\n**bold**\n{% endcallout %}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<h1"),
				"a block shortcode without the > marker must still Markdown-process its body")
			Expect(html).To(ContainSubstring("<strong>bold</strong>"))
			Expect(html).To(ContainSubstring(`{% callout "info" %}`))
		})
	})

	// ── Display mode (templateTags: false) ────────────────────────────

	Context("Display mode (templateTags: false)", func() {
		displayMD := content.CreateGoldmark(content.MarkdownOptions{
			Unsafe: true, Typographer: true, TemplateTags: false,
		})

		It("shows the open tag as typed, marker included", func() {
			// Display mode exists to show template source verbatim, so the
			// reader sees the characters the author actually wrote. Tags are
			// escaped with zero-width spaces between the delimiter characters,
			// which leaves "%> helmet" contiguous.
			source := []byte("{%> helmet %}\nbody\n{% endhelmet %}\n")
			out, _, err := content.RenderMarkdown(source, displayMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("%> helmet"),
				"in display mode the > must be shown as typed, not stripped")
		})

		It("still captures the body verbatim", func() {
			source := []byte("{%> code %}\n# not a heading\n{% endcode %}\n")
			out, _, err := content.RenderMarkdown(source, displayMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("# not a heading"),
				"raw capture does not depend on the templateTags setting")
			Expect(html).NotTo(ContainSubstring("<h1"))
		})
	})

	// ── Unterminated raw block is a build error ───────────────────────

	Context("Unterminated raw block", func() {
		It("errors instead of swallowing the rest of the file (Liquid)", func() {
			source := []byte("{%> helmet %}\nbody line\nno close here\n")
			_, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).To(HaveOccurred(),
				"an unterminated raw block must be a build error, not silent capture to EOF")
			msg := err.Error()
			Expect(msg).To(ContainSubstring("unterminated raw block shortcode"),
				"the error must name the construct")
			Expect(msg).To(ContainSubstring("{%> helmet %}"),
				"the error must quote the open tag as the author wrote it")
			Expect(msg).To(ContainSubstring("line 1"),
				"the error must give the 1-based line number of the open tag")
			Expect(msg).To(ContainSubstring("{% endhelmet %}"),
				"the error must name the close tag the author needs to add")
		})

		It("reports the line number of the open tag, not the end of file", func() {
			source := []byte("intro\n\nmore intro\n\n{%> helmet %}\nbody\n")
			_, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("line 5"),
				"the reported line must point at the open tag so the author can find it")
		})

		It("reports the outermost open tag when nested opens are outstanding", func() {
			source := []byte("{%> callout %}\nouter\n{% callout %}\ninner\n{% endcallout %}\nnever closed\n")
			_, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).To(HaveOccurred(),
				"depth never returns to 0, so this is unterminated")
			Expect(err.Error()).To(ContainSubstring("line 1"),
				"the outermost open tag is the one the author must fix")
		})

		It("errors in display mode too", func() {
			displayMD := content.CreateGoldmark(content.MarkdownOptions{
				Unsafe: true, TemplateTags: false,
			})
			source := []byte("{%> helmet %}\nbody\n")
			_, _, err := content.RenderMarkdown(source, displayMD)
			Expect(err).To(HaveOccurred(),
				"the parser is shared, so the error is raised regardless of templateTags")
		})
	})

	// ── Go template delimiters ────────────────────────────────────────

	Context("Go template raw blocks", func() {
		It("captures the body verbatim and strips the marker", func() {
			source := []byte("{{%> helmet %}}\n# not a heading\nif (a < 2 && b > 1) {}\n{{% /helmet %}}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{{% helmet %}}"),
				"the open tag must reach the engine in native Go template syntax")
			Expect(html).NotTo(ContainSubstring("{{%>"),
				"the > marker must never reach the engine")
			Expect(html).To(ContainSubstring("{{% /helmet %}}"),
				"the close tag uses /name and is emitted unchanged")
			Expect(html).To(ContainSubstring("# not a heading"))
			Expect(html).To(ContainSubstring("if (a < 2 && b > 1) {}"))
			Expect(html).NotTo(ContainSubstring("<h1"))
			Expect(html).NotTo(ContainSubstring("&lt;"))
		})

		It("preserves arguments so the block shortcode preprocessor still matches", func() {
			// ProcessBlockShortcodes' open pattern requires whitespace after
			// {{%, so stripping must not run the delimiter into the tag name.
			source := []byte("{{%> helmet \"a b\" %}}\nbody\n{{% /helmet %}}\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(`{{% helmet "a b" %}}`),
				"the stripped open tag must match the {{% tag \"args\" %}} pattern exactly")
		})

		It("depth-counts a balanced same-name pair inside the body", func() {
			source := []byte("{{%> callout %}}\nouter\n{{% callout %}}\ninner\n{{% /callout %}}\nstill outer\n{{% /callout %}}\n\nAfter.\n")
			out, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("still outer"),
				"a balanced nested pair must not close the outer raw block early")
			Expect(html).NotTo(ContainSubstring("<p>still outer</p>"),
				"content after the nested pair is still raw body, not Markdown")
			Expect(html).To(ContainSubstring("<p>After.</p>"))
		})

		It("errors on an unterminated block with the Go template close tag", func() {
			source := []byte("{{%> helmet %}}\nbody\n")
			_, _, err := content.RenderMarkdown(source, rawMD)
			Expect(err).To(HaveOccurred())
			msg := err.Error()
			Expect(msg).To(ContainSubstring("unterminated raw block shortcode"))
			Expect(msg).To(ContainSubstring("{{%> helmet %}}"),
				"the error must quote the open tag as written")
			Expect(msg).To(ContainSubstring("{{% /helmet %}}"),
				"the expected close tag must use the Go template /name form")
		})
	})
})
