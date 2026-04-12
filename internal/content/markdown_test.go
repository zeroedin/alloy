package content_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

var _ = Describe("RenderMarkdown", func() {
	defaultOpts := content.MarkdownOptions{
		Unsafe:       true,
		Typographer:  true,
		TemplateTags: true,
	}

	// ── Basic Markdown ─────────────────────────────────────────────────

	Context("Basic Markdown", func() {
		It("converts headings to h1-h6 tags", func() {
			source := []byte("# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<h1>H1</h1>"))
			Expect(html).To(ContainSubstring("<h2>H2</h2>"))
			Expect(html).To(ContainSubstring("<h3>H3</h3>"))
			Expect(html).To(ContainSubstring("<h4>H4</h4>"))
			Expect(html).To(ContainSubstring("<h5>H5</h5>"))
			Expect(html).To(ContainSubstring("<h6>H6</h6>"))
		})

		It("converts paragraphs", func() {
			source := []byte("First paragraph.\n\nSecond paragraph.\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<p>First paragraph.</p>"))
			Expect(html).To(ContainSubstring("<p>Second paragraph.</p>"))
		})

		It("converts bold and italic", func() {
			source := []byte("This is **bold** and *italic* text.\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<strong>bold</strong>"))
			Expect(html).To(ContainSubstring("<em>italic</em>"))
		})

		It("converts links", func() {
			source := []byte("[Alloy](https://example.com)\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(`<a href="https://example.com">Alloy</a>`))
		})

		It("converts unordered lists", func() {
			source := []byte("- item one\n- item two\n- item three\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<ul>"))
			Expect(html).To(ContainSubstring("<li>item one</li>"))
			Expect(html).To(ContainSubstring("<li>item two</li>"))
			Expect(html).To(ContainSubstring("<li>item three</li>"))
			Expect(html).To(ContainSubstring("</ul>"))
		})

		It("converts code blocks with language attribute", func() {
			source := []byte("```go\nfmt.Println(\"hello\")\n```\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<code"))
			Expect(html).To(ContainSubstring("go"))
			Expect(html).To(ContainSubstring("fmt.Println"))
		})
	})

	// ── CommonMark extensions ──────────────────────────────────────────

	Context("CommonMark extensions", func() {
		It("renders tables", func() {
			source := []byte("| Name | Age |\n| ---- | --- |\n| Alice | 30 |\n| Bob | 25 |\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<table>"))
			Expect(html).To(ContainSubstring("<th>Name</th>"))
			Expect(html).To(ContainSubstring("<td>Alice</td>"))
			Expect(html).To(ContainSubstring("</table>"))
		})

		It("renders task lists (checkboxes)", func() {
			source := []byte("- [x] Done\n- [ ] Not done\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`type="checkbox"`))
			Expect(html).To(ContainSubstring("Done"))
			Expect(html).To(ContainSubstring("Not done"))
		})
	})

	// ── Unsafe HTML passthrough ────────────────────────────────────────

	Context("Unsafe HTML passthrough", func() {
		It("passes raw HTML blocks through when unsafe=true", func() {
			source := []byte("<div class=\"custom\">Hello</div>\n")
			opts := content.MarkdownOptions{Unsafe: true, Typographer: true, TemplateTags: true}
			out, err := content.RenderMarkdown(source, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(`<div class="custom">Hello</div>`))
		})

		It("strips raw HTML when unsafe=false", func() {
			source := []byte("<div class=\"custom\">Hello</div>\n")
			opts := content.MarkdownOptions{Unsafe: false}
			out, err := content.RenderMarkdown(source, opts)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).NotTo(ContainSubstring("<div"))
		})
	})

	// ── Template tag preservation ──────────────────────────────────────

	Context("Template tag preservation", func() {
		It("preserves {{ variable }} expressions through Markdown", func() {
			source := []byte("Hello {{ name }}!\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("{{ name }}"))
		})

		It("preserves {% tag %} blocks through Markdown", func() {
			source := []byte("{% if show %}Visible{% endif %}\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{% if show %}"))
			Expect(html).To(ContainSubstring("{% endif %}"))
		})

		It("does not interfere with inline code containing {{ }}", func() {
			source := []byte("Use `{{ variable }}` in templates.\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<code>{{ variable }}</code>"))
		})

		It("does not interfere with fenced code blocks containing {{ }}", func() {
			source := []byte("```liquid\n{{ page.title }}\n{% for item in items %}\n  {{ item }}\n{% endfor %}\n```\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{{ page.title }}"))
			Expect(html).To(ContainSubstring("{% for item in items %}"))
		})

		It("disables template tag preservation when templateTags is false", func() {
			source := []byte("Hello {{ name }}!\n")
			opts := content.MarkdownOptions{Unsafe: true, Typographer: true, TemplateTags: false}
			out, err := content.RenderMarkdown(source, opts)
			Expect(err).NotTo(HaveOccurred())
			// With templateTags disabled, {{ name }} should NOT be preserved
			// as a raw node — goldmark may escape or mangle it
			html := string(out)
			Expect(html).NotTo(ContainSubstring("{{ name }}"),
				"templateTags:false must disable the auto-detection extension")
		})

		It("preserves {% raw %}...{% endraw %} as literal template syntax", func() {
			source := []byte("Show this: {% raw %}{{ not_a_variable }}{% endraw %}\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// The raw block must pass through goldmark; the template engine
			// later strips {% raw %} and outputs the literal {{ }}
			Expect(html).To(ContainSubstring("{% raw %}"),
				"{% raw %} must survive Markdown processing")
			Expect(html).To(ContainSubstring("{% endraw %}"),
				"{% endraw %} must survive Markdown processing")
		})
	})

	// ── Goldmark extensions (§6 footnotes, typographer) ──────────────

	Context("Goldmark extensions", func() {
		It("renders footnotes (§6 goldmark extensions)", func() {
			source := []byte("This has a footnote[^1].\n\n[^1]: This is the footnote text.\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// Footnotes should produce a link and a footnote section
			Expect(html).To(ContainSubstring("footnote"),
				"footnotes extension must produce footnote markup")
		})

		It("applies typographer for smart quotes and em-dashes", func() {
			source := []byte("She said \"hello\" -- and left...\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// Typographer should convert:
			//   "hello" → \u201chello\u201d (smart quotes)
			//   -- → \u2014 (em-dash)
			//   ... → \u2026 (ellipsis)
			Expect(html).To(SatisfyAny(
				ContainSubstring("\u201c"), // left double quote
				ContainSubstring("&ldquo;"),
			), "typographer must convert straight quotes to smart quotes")
		})
	})

	// ── .txt file handling (§6) ──────────────────────────────────────

	Context(".txt file handling", func() {
		It("wraps plain text in <pre> tags", func() {
			source := []byte("This is plain text.\nNo markdown here.")
			out, err := content.RenderText(source)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<pre>"),
				".txt files must be wrapped in <pre> tags")
			Expect(html).To(ContainSubstring("This is plain text."))
			Expect(html).To(ContainSubstring("</pre>"))
		})
	})
})
