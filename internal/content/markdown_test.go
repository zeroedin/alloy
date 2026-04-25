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

		It("preserves template tags containing newlines through Markdown", func() {
			// PR #217 added (?s) to templateTagPattern so tags spanning
			// multiple lines are matched as a single unit. Without this,
			// goldmark splits the tag across lines and breaks it.
			source := []byte("{{ \"hello\nworld\" | newline_to_br }}\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("newline_to_br"),
				"multiline template tag must survive goldmark processing intact")
			Expect(html).To(ContainSubstring("{{"),
				"opening {{ must be preserved")
			Expect(html).To(ContainSubstring("}}"),
				"closing }} must be preserved")
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

	// ── Block shortcode boundaries (§6 TemplateBlocks, issue #202) ──

	Context("Block shortcode boundaries", func() {
		It("block shortcode tags are not wrapped in <p>", func() {
			source := []byte("# Title\n\n{% callout \"warning\" %}\nThis has **bold** text.\n{% endcallout %}\n\nAfter.\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// The opening {% callout %} must NOT be inside a <p> tag
			Expect(html).NotTo(ContainSubstring("<p>{% callout"),
				"block shortcode opening tag must not be wrapped in <p> — "+
					"goldmark must treat it as a block-level boundary")
			// The closing {% endcallout %} must NOT be inside a <p> tag
			Expect(html).NotTo(ContainSubstring("<p>{% endcallout"),
				"block shortcode closing tag must not be wrapped in <p>")
			// The opening and closing tags must be preserved
			Expect(html).To(ContainSubstring("{% callout"),
				"block shortcode opening tag must be preserved through markdown")
			Expect(html).To(ContainSubstring("{% endcallout %}"),
				"block shortcode closing tag must be preserved through markdown")
			// Inner content must be processed as markdown
			Expect(html).To(ContainSubstring("<strong>bold</strong>"),
				"markdown inside block shortcode must be rendered")
		})

		It("inline shortcode on same line as text stays inline", func() {
			source := []byte("Watch this: {% youtube \"abc123\" %} and more text.\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// Inline shortcode should be inside a <p> — that's correct
			Expect(html).To(ContainSubstring("<p>Watch this: {% youtube"),
				"inline shortcode mixed with text must stay in <p> context")
		})

		It("block shortcode with list content renders correctly", func() {
			source := []byte("{% callout \"info\" %}\n\n- Item one\n- Item two\n\n{% endcallout %}\n")
			out, err := content.RenderMarkdown(source, defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// List must be a proper <ul> not mangled by <p> wrapping
			Expect(html).To(ContainSubstring("<ul>"),
				"list inside block shortcode must render as <ul>")
			Expect(html).To(ContainSubstring("<li>Item one</li>"),
				"list items must render correctly inside block shortcode")
			// The shortcode tags must not interfere with list rendering
			Expect(html).NotTo(ContainSubstring("<p>{% callout"),
				"block shortcode must not wrap list content in <p>")
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

	// ── Auto heading IDs (issue #274) ─────────────────────────────
	// Goldmark must generate id attributes on all headings by default.

	Describe("Auto heading IDs", func() {
		It("generates id attributes on headings", func() {
			out, err := content.RenderMarkdown(
				[]byte("## Getting Started\n\n### Installation"),
				defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="getting-started"`),
				"h2 must have an auto-generated slugified id attribute")
			Expect(html).To(ContainSubstring(`id="installation"`),
				"h3 must have an auto-generated slugified id attribute")
		})

		It("handles duplicate headings with numeric suffix", func() {
			out, err := content.RenderMarkdown(
				[]byte("## Overview\n\nText.\n\n## Overview\n\nMore text.\n\n## Overview"),
				defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="overview"`),
				"first heading must have the base id")
			Expect(html).To(ContainSubstring(`id="overview-1"`),
				"second duplicate must get suffix -1")
			Expect(html).To(ContainSubstring(`id="overview-2"`),
				"third duplicate must get suffix -2")
		})

		It("respects manual heading attributes override", func() {
			out, err := content.RenderMarkdown(
				[]byte("## My Section {#custom-id}"),
				defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="custom-id"`),
				"manual {#id} attribute must override the auto-generated id")
			Expect(html).NotTo(ContainSubstring(`id="my-section"`),
				"auto-generated id must not appear when manual override is set")
		})
	})

	// ── Table of contents (issue #274) ────────────────────────────
	// page.toc is extracted from the goldmark AST during markdown
	// rendering. Nested array of {id, text, level, children}.

	Describe("Table of contents", func() {
		It("extracts headings into a nested TOC structure", func() {
			input := "## Getting Started\n\n### Installation\n\n### Quickstart\n\n## Configuration"
			_, toc, err := content.RenderMarkdownWithTOC([]byte(input), defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(2),
				"top-level TOC must contain two h2 entries")
			Expect(toc[0].Text).To(Equal("Getting Started"))
			Expect(toc[0].ID).To(Equal("getting-started"))
			Expect(toc[0].Level).To(Equal(2))
			Expect(toc[0].Children).To(HaveLen(2),
				"h3s must nest under their preceding h2")
			Expect(toc[0].Children[0].Text).To(Equal("Installation"))
			Expect(toc[0].Children[1].Text).To(Equal("Quickstart"))
			Expect(toc[1].Text).To(Equal("Configuration"))
			Expect(toc[1].Children).To(BeEmpty())
		})

		It("excludes h1 from TOC", func() {
			input := "# Page Title\n\n## Section One\n\n## Section Two"
			_, toc, err := content.RenderMarkdownWithTOC([]byte(input), defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(2),
				"h1 must be excluded from TOC — it is the page title")
			Expect(toc[0].Text).To(Equal("Section One"))
			Expect(toc[1].Text).To(Equal("Section Two"))
		})

		It("returns empty TOC for pages with no headings", func() {
			input := "Just a paragraph of text."
			_, toc, err := content.RenderMarkdownWithTOC([]byte(input), defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(BeEmpty(),
				"pages without headings must have an empty TOC")
		})

		It("uses manual {#id} override in TOC entry", func() {
			input := "## My Section {#custom-id}"
			_, toc, err := content.RenderMarkdownWithTOC([]byte(input), defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(1))
			Expect(toc[0].ID).To(Equal("custom-id"),
				"TOC must use the manually overridden id, not the auto-generated slug")
		})

		It("nests h4 under h3 under h2", func() {
			input := "## Top\n\n### Mid\n\n#### Deep"
			_, toc, err := content.RenderMarkdownWithTOC([]byte(input), defaultOpts)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(1))
			Expect(toc[0].Children).To(HaveLen(1))
			Expect(toc[0].Children[0].Children).To(HaveLen(1),
				"h4 must nest under h3 which nests under h2")
			Expect(toc[0].Children[0].Children[0].Text).To(Equal("Deep"))
		})
	})

	// ── Render hooks (issue #273) ──────────────────────────────────
	// Render hook templates in layouts/_markup/ override how specific
	// markdown elements are rendered to HTML.

	Describe("Render hooks", func() {
		It("render-codeblock.liquid overrides fenced code block output", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"codeblock": `<rh-code-block language="{{ markup.language }}">{{ markup.inner }}</rh-code-block>`,
				},
			}
			out, err := content.RenderMarkdown([]byte("```javascript\nconsole.log('hello');\n```"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<rh-code-block"),
				"render hook must override default <pre><code> output")
			Expect(html).To(ContainSubstring(`language="javascript"`),
				"markup.language must be available in the hook template")
			Expect(html).To(ContainSubstring("console.log"),
				"markup.inner must contain the code content")
			Expect(html).NotTo(ContainSubstring("<pre>"),
				"default <pre><code> must not appear when hook is active")
		})

		It("render-link.liquid overrides link output", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"link": `<a href="{{ markup.destination }}" class="custom">{{ markup.text }}</a>`,
				},
			}
			out, err := content.RenderMarkdown([]byte("[Click here](https://example.com)"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`class="custom"`),
				"render hook must override default link output")
			Expect(html).To(ContainSubstring(`href="https://example.com"`),
				"markup.destination must be available")
			Expect(html).To(ContainSubstring("Click here"),
				"markup.text must contain the link text")
		})

		It("render-heading.liquid overrides heading output", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} id="{{ markup.id }}"><a href="#{{ markup.id }}">{{ markup.inner }}</a></h{{ markup.level }}>`,
				},
			}
			out, err := content.RenderMarkdown([]byte("## My Section"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="my-section"`),
				"markup.id must be a slugified version of the heading text")
			Expect(html).To(ContainSubstring(`<a href="#my-section">`),
				"render hook must be able to wrap headings in permalink anchors")
			Expect(html).To(ContainSubstring("My Section"),
				"markup.inner must contain the heading content")
		})

		It("render-image.liquid overrides image output", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"image": `<figure><img src="{{ markup.src }}" alt="{{ markup.alt }}" loading="lazy"><figcaption>{{ markup.title }}</figcaption></figure>`,
				},
			}
			out, err := content.RenderMarkdown([]byte(`![A photo](/photo.jpg "Photo caption")`), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<figure>"),
				"render hook must override default <img> output")
			Expect(html).To(ContainSubstring(`loading="lazy"`),
				"hook can add custom attributes like lazy loading")
			Expect(html).To(ContainSubstring("Photo caption"),
				"markup.title must be available")
		})

		It("render-blockquote.liquid overrides blockquote output", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"blockquote": `<rh-alert>{{ markup.inner }}</rh-alert>`,
				},
			}
			out, err := content.RenderMarkdown([]byte("> This is a note"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<rh-alert>"),
				"render hook must override default <blockquote> output")
			Expect(html).To(ContainSubstring("This is a note"),
				"markup.inner must contain the blockquote content")
			Expect(html).NotTo(ContainSubstring("<blockquote>"),
				"default <blockquote> must not appear when hook is active")
		})

		It("render-table.liquid overrides table output", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"table": `<div class="table-wrapper">{{ markup.inner }}</div>`,
				},
			}
			out, err := content.RenderMarkdown([]byte("| A | B |\n|---|---|\n| 1 | 2 |"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`<div class="table-wrapper">`),
				"render hook must override default <table> output")
			Expect(html).To(ContainSubstring("</div>"),
				"hook wrapper must be present")
		})

		It("language-specific codeblock hook takes precedence", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"codeblock":         `<pre class="default"><code>{{ markup.inner }}</code></pre>`,
					"codeblock-mermaid": `<div class="mermaid">{{ markup.inner }}</div>`,
				},
			}
			out, err := content.RenderMarkdown([]byte("```mermaid\ngraph TD;\nA-->B;\n```"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`<div class="mermaid">`),
				"language-specific hook (render-codeblock-mermaid) must take precedence over generic")
			Expect(html).NotTo(ContainSubstring(`class="default"`),
				"generic codeblock hook must not be used when language-specific exists")
		})

		It("falls back to default rendering when no hook exists", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{},
			}
			out, err := content.RenderMarkdown([]byte("```go\nfmt.Println(\"hello\")\n```"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<pre>"),
				"without render hooks, goldmark default rendering must be used")
			Expect(html).To(ContainSubstring("<code"),
				"default <pre><code> output must appear when no hook is set")
		})

		It("render-link.liquid provides is_external for external links", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"link": `{% if markup.is_external %}<a href="{{ markup.destination }}" target="_blank">{{ markup.text }}</a>{% else %}<a href="{{ markup.destination }}">{{ markup.text }}</a>{% endif %}`,
				},
			}
			out, err := content.RenderMarkdown([]byte("[External](https://example.com) and [Internal](/about)"), opts)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`target="_blank"`),
				"external link must have target=_blank via markup.is_external")
			Expect(html).To(ContainSubstring(`<a href="/about">Internal</a>`),
				"internal link must not have target=_blank")
		})
	})
})
