package content_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("RenderMarkdown", func() {
	defaultOpts := content.MarkdownOptions{
		Unsafe:        true,
		Typographer:   true,
		TemplateTags:  true,
		AutoHeadingID: true,
	}
	defaultMD := content.CreateGoldmark(defaultOpts)

	// ── Basic Markdown ─────────────────────────────────────────────────

	Context("Basic Markdown", func() {
		It("converts headings to h1-h6 tags", func() {
			// AutoHeadingID: false — test heading tag conversion in isolation
			// without auto-generated id attributes (issue #306).
			// Auto heading ID behavior is tested separately in the
			// "Auto heading IDs" describe block with AutoHeadingID: true.
			noAutoID := content.MarkdownOptions{
				Unsafe:        true,
				Typographer:   true,
				TemplateTags:  true,
				AutoHeadingID: false,
			}
			source := []byte("# H1\n## H2\n### H3\n#### H4\n##### H5\n###### H6\n")
			out, _, err := content.RenderMarkdown(source, content.CreateGoldmark(noAutoID))
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
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<p>First paragraph.</p>"))
			Expect(html).To(ContainSubstring("<p>Second paragraph.</p>"))
		})

		It("converts bold and italic", func() {
			source := []byte("This is **bold** and *italic* text.\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<strong>bold</strong>"))
			Expect(html).To(ContainSubstring("<em>italic</em>"))
		})

		It("converts links", func() {
			source := []byte("[Alloy](https://example.com)\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(`<a href="https://example.com">Alloy</a>`))
		})

		It("converts unordered lists", func() {
			source := []byte("- item one\n- item two\n- item three\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
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
			out, _, err := content.RenderMarkdown(source, defaultMD)
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
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<table>"))
			Expect(html).To(ContainSubstring("<th>Name</th>"))
			Expect(html).To(ContainSubstring("<td>Alice</td>"))
			Expect(html).To(ContainSubstring("</table>"))
		})

		It("renders task lists (checkboxes)", func() {
			source := []byte("- [x] Done\n- [ ] Not done\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
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
			out, _, err := content.RenderMarkdown(source, content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(`<div class="custom">Hello</div>`))
		})

		It("strips raw HTML when unsafe=false", func() {
			source := []byte("<div class=\"custom\">Hello</div>\n")
			opts := content.MarkdownOptions{Unsafe: false}
			out, _, err := content.RenderMarkdown(source, content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).NotTo(ContainSubstring("<div"))
		})
	})

	// ── Custom element block parsing (issue #784) ─────────────────────

	Describe("Custom element block parsing", func() {
		It("treats a custom element on its own line as a block-level element", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<alloy-code>\nsome code\n</alloy-code>\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<alloy-code>"))
			Expect(html).To(ContainSubstring("</alloy-code>"))
			Expect(html).NotTo(ContainSubstring("<p>"),
				"custom elements must be treated as block-level HTML, not wrapped in <p>")
		})

		It("does not terminate the block at blank lines inside custom elements", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<my-widget>\nfirst paragraph\n\nsecond paragraph\n</my-widget>\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("first paragraph"))
			Expect(html).To(ContainSubstring("second paragraph"))
			Expect(html).NotTo(ContainSubstring("<p>"),
				"blank lines inside custom elements must not trigger paragraph "+
					"wrapping — content must be preserved verbatim like <pre>")
		})

		It("preserves content verbatim inside custom elements (no smart quotes)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, CustomElements: true,
			}
			md := "<my-component>\n\"quoted\" -- dashes\n</my-component>\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`"quoted" -- dashes`),
				"typographer must not process content inside custom elements — "+
					"smart quotes and em-dashes must not be applied")
			Expect(html).NotTo(ContainSubstring("&ldquo;"),
				"smart quotes must not appear inside custom element content")
		})

		It("terminates the block at the matching closing tag", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<my-component>\ninner content\n</my-component>\n\n**bold after**\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<my-component>"))
			Expect(html).To(ContainSubstring("inner content"))
			Expect(html).To(ContainSubstring("</my-component>"))
			Expect(html).To(ContainSubstring("<strong>bold after</strong>"),
				"markdown content after the closing tag must be processed normally")
		})

		It("preserves attributes on the custom element opening tag", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<alloy-code lang=\"go\" theme=\"dark\">\nfunc main() {}\n</alloy-code>\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`lang="go"`),
				"attributes on the opening tag must be preserved")
			Expect(html).To(ContainSubstring(`theme="dark"`),
				"multiple attributes must be preserved")
		})

		It("handles nested custom elements without premature closure", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<wa-tab-group>\n<wa-tab panel=\"one\">Tab 1</wa-tab>\n\n<wa-tab-panel name=\"one\">\nPanel content\n</wa-tab-panel>\n</wa-tab-group>\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<wa-tab-group>"))
			Expect(html).To(ContainSubstring("</wa-tab-group>"))
			Expect(html).To(ContainSubstring("<wa-tab-panel name=\"one\">"))
			Expect(html).NotTo(ContainSubstring("<p>"),
				"nested custom elements must not cause paragraph wrapping — "+
					"inner closing tags like </wa-tab> must not terminate the outer block")
		})

		It("handles same-name nested custom elements correctly", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<my-list>\n<my-list>\ninner\n</my-list>\nouter\n</my-list>\n\n**after**\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("outer"))
			Expect(html).To(ContainSubstring("<strong>after</strong>"),
				"same-name nesting requires depth tracking — the first </my-list> "+
					"must not terminate the outer block")
		})

		It("does not falsely match prefix-overlapping closing tags on the first line", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<my-list><my-list-item>inline item</my-list-item>\nsecond line\n</my-list>\n\n**after**\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("second line"),
				"</my-list-item> on the opener line must not close the <my-list> block — "+
					"countTagOccurrences boundary checking must reject '</my-list-' "+
					"because the char after the tag name is '-', not '>' or whitespace")
			Expect(html).NotTo(ContainSubstring("<p>second line</p>"),
				"second line must remain inside the custom element block, not be "+
					"ejected into a paragraph by premature block closure")
			Expect(html).To(ContainSubstring("</my-list>"),
				"the actual </my-list> closing tag must be present in the block output")
			Expect(html).To(ContainSubstring("<strong>after</strong>"),
				"markdown after the block must be processed normally, confirming "+
					"the block was not prematurely closed by the prefix-matching tag")
		})

		It("tracks depth correctly when multiple same-name openers appear on a single line", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<my-el><my-el>\ninner\n</my-el>\nstill inside\n</my-el>\n\n**after**\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("still inside"),
				"two <my-el> openers on line one must set depth to 2 — "+
					"the first </my-el> decrements to 1 and must not close the block")
			Expect(html).NotTo(ContainSubstring("<p>still inside</p>"),
				"still inside must remain inside the custom element block, not be "+
					"rendered as a paragraph after premature block closure")
			Expect(html).To(ContainSubstring("<strong>after</strong>"),
				"markdown after the second </my-el> must be processed normally, "+
					"confirming the block closes only when depth reaches 0")
		})

		It("does not change behavior for standard HTML elements", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: true}
			md := "<div>\nfirst\n\nsecond\n</div>\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<p>second</p>"),
				"standard HTML elements must retain default Goldmark behavior — "+
					"custom element block parsing must not affect tags without hyphens")
		})

		It("falls back to default handling when customElements is disabled", func() {
			opts := content.MarkdownOptions{Unsafe: true, CustomElements: false}
			md := "<my-widget>\nfirst\n\nsecond\n</my-widget>\n"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<p>"),
				"with customElements disabled, blank lines inside custom elements "+
					"must terminate the HTML block (standard Goldmark behavior)")
		})
	})

	// ── Template tag preservation ──────────────────────────────────────

	Context("Template tag preservation", func() {
		It("preserves {{ variable }} expressions through Markdown", func() {
			source := []byte("Hello {{ name }}!\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("{{ name }}"))
		})

		It("preserves {% tag %} blocks through Markdown", func() {
			source := []byte("{% if show %}Visible{% endif %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{% if show %}"))
			Expect(html).To(ContainSubstring("{% endif %}"))
		})

		It("does not interfere with inline code containing {{ }}", func() {
			source := []byte("Use `{{ variable }}` in templates.\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<code>{{ variable }}</code>"))
		})

		It("does not interfere with fenced code blocks containing {{ }}", func() {
			source := []byte("```liquid\n{{ page.title }}\n{% for item in items %}\n  {{ item }}\n{% endfor %}\n```\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
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
			out, _, err := content.RenderMarkdown(source, defaultMD)
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
			out, _, err := content.RenderMarkdown(source, content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			// With templateTags disabled, {{ name }} should NOT be preserved
			// as a raw node — goldmark may escape or mangle it
			html := string(out)
			Expect(html).NotTo(ContainSubstring("{{ name }}"),
				"templateTags:false must disable the auto-detection extension")
		})

		It("preserves {% raw %}...{% endraw %} as literal template syntax", func() {
			source := []byte("Show this: {% raw %}{{ not_a_variable }}{% endraw %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
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
			out, _, err := content.RenderMarkdown(source, defaultMD)
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
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// Inline shortcode should be inside a <p> — that's correct
			Expect(html).To(ContainSubstring("<p>Watch this: {% youtube"),
				"inline shortcode mixed with text must stay in <p> context")
		})

		It("block shortcode with list content renders correctly", func() {
			source := []byte("{% callout \"info\" %}\n\n- Item one\n- Item two\n\n{% endcallout %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
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

	// ── Expression tag paragraph preservation (issue #378) ───────────
	// {{ }} expressions keep surrounding <p> wrappers (user-authored or
	// goldmark-added), while {% %} block shortcodes have <p> stripped.

	Context("Expression tag paragraph preservation", func() {
		It("{{ }} expression on its own line keeps <p> wrapper", func() {
			source := []byte("{{ page.title }}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("<p>{{ page.title }}</p>"),
				"expression tags on their own line must keep goldmark's paragraph wrapper")
		})

		It("{{ }} expression inside user-authored <p> keeps tags", func() {
			source := []byte("<p>{{ member.name }}</p>\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring("<p>{{ member.name }}</p>"),
				"user-authored <p> tags around expressions must be preserved")
		})

		It("{% %} block shortcode on its own line has <p> stripped", func() {
			source := []byte("{% hero %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).NotTo(ContainSubstring("<p>{% hero %}</p>"),
				"block shortcode tags must not be wrapped in <p>")
			Expect(string(out)).To(ContainSubstring("{% hero %}"),
				"block shortcode tag must be preserved")
		})
	})

	// ── Goldmark template tag extensions (issue #564) ──────────────────
	// Tests that the goldmark template tag implementation uses proper custom
	// AST nodes and parsers (not placeholder regex substitution). Custom AST
	// nodes must be preserved regardless of the unsafe setting.

	Context("Goldmark template tag extensions (issue #564)", func() {
		safeOpts := content.MarkdownOptions{
			Unsafe: false, Typographer: true, TemplateTags: true, AutoHeadingID: true,
		}
		safeMD := content.CreateGoldmark(safeOpts)

		It("multiple template tags on one line are inline, not block", func() {
			source := []byte("{% if show %}Visible{% endif %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<p>{% if show %}Visible{% endif %}</p>"),
				"multiple template tags on one line must be treated as inline — "+
					"the block parser must only match lines with exactly one tag "+
					"and no other content (issue #564)")
		})

		It("preserves block template tags when unsafe is false", func() {
			source := []byte("{% hero %}\nContent here.\n{% endhero %}\n")
			out, _, err := content.RenderMarkdown(source, safeMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{% hero %}"),
				"block template tags must be preserved regardless of unsafe setting — "+
					"implementation must use custom AST nodes, not ast.RawHTML (issue #564)")
			Expect(html).To(ContainSubstring("{% endhero %}"))
			Expect(html).NotTo(ContainSubstring("<!-- raw HTML omitted -->"),
				"template tags must not be treated as raw HTML by goldmark")
		})

		It("preserves inline expression tags when unsafe is false", func() {
			source := []byte("Hello {{ name }}!\n")
			out, _, err := content.RenderMarkdown(source, safeMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{{ name }}"),
				"inline expression tags must be preserved regardless of unsafe setting — "+
					"implementation must use custom AST nodes, not ast.RawHTML (issue #564)")
			Expect(html).NotTo(ContainSubstring("<!-- raw HTML omitted -->"))
		})

		It("preserves inline {% %} control tags when unsafe is false", func() {
			source := []byte("Show {% if active %}this{% endif %} text.\n")
			out, _, err := content.RenderMarkdown(source, safeMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{% if active %}"),
				"inline control tags must be preserved regardless of unsafe setting (issue #564)")
			Expect(html).To(ContainSubstring("{% endif %}"))
			Expect(html).NotTo(ContainSubstring("<!-- raw HTML omitted -->"))
		})

		It("does not produce empty <p></p> from blank lines adjacent to block template tags", func() {
			source := []byte("{% helmet %}\n<style>\n  .intro h2 { color: red; }\n</style>\n{% endhelmet %}\n\n<section class=\"intro\">\nHello\n</section>\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).NotTo(ContainSubstring("<p></p>"),
				"blank lines adjacent to block template tags must not produce empty paragraphs — "+
					"the implementation must not inject artificial blank-line boundaries (issue #564)")
			Expect(html).To(ContainSubstring("{% helmet %}"))
			Expect(html).To(ContainSubstring("{% endhelmet %}"))
		})

		It("handles multiple consecutive block tags without blank lines between them", func() {
			source := []byte("{% note %}\nFirst.\n{% endnote %}\n{% warning %}\nSecond.\n{% endwarning %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{% note %}"))
			Expect(html).To(ContainSubstring("{% endnote %}"))
			Expect(html).To(ContainSubstring("{% warning %}"))
			Expect(html).To(ContainSubstring("{% endwarning %}"))
			Expect(html).NotTo(ContainSubstring("<p></p>"),
				"no empty paragraphs between consecutive block tags (issue #564)")
			Expect(html).NotTo(ContainSubstring("<p>{% note"),
				"block-level template tags must not be wrapped in <p> (issue #564)")
		})

		It("adjacent HTML blocks next to template tags do not interfere", func() {
			source := []byte("<style>\n.foo { color: red; }\n</style>\n\n{% hero %}\nContent\n{% endhero %}\n\n<div class=\"box\">\nMore\n</div>\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<style>"))
			Expect(html).To(ContainSubstring("{% hero %}"))
			Expect(html).To(ContainSubstring("{% endhero %}"))
			Expect(html).To(ContainSubstring("<div class=\"box\">"))
			Expect(html).NotTo(ContainSubstring("<p></p>"),
				"no empty paragraphs between HTML blocks and template tags (issue #564)")
		})

		It("block template tag with blank lines inside renders inner content correctly", func() {
			source := []byte("{% hero %}\n\nParagraph after blank.\n\n{% endhero %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<p>Paragraph after blank.</p>"),
				"inner content between block template tags must render as normal markdown (issue #564)")
			Expect(html).To(ContainSubstring("{% hero %}"))
			Expect(html).NotTo(ContainSubstring("<p>{% hero"),
				"block template tags must not be wrapped in <p> (issue #564)")
		})

		It("preserves whitespace-trimming template tags ({%- -%})", func() {
			source := []byte("{%- if show -%}\nVisible\n{%- endif -%}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{%- if show -%}"))
			Expect(html).To(ContainSubstring("{%- endif -%}"))
		})

		It("preserves template tags inside blockquotes", func() {
			source := []byte("> {{ page.pullquote }}\n>\n> -- {{ page.author }}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{{ page.pullquote }}"))
			Expect(html).To(ContainSubstring("{{ page.author }}"))
			Expect(html).To(ContainSubstring("<blockquote>"))
		})

		It("preserves template tags inside list items", func() {
			source := []byte("- {{ item.title }}\n- {% include \"partial\" %}\n- Regular text\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("{{ item.title }}"))
			Expect(html).To(ContainSubstring("{% include"))
			Expect(html).To(ContainSubstring("<li>"))
		})

		It("does not leave placeholder artifacts in output", func() {
			source := []byte("{% hero %}\n{{ page.title }}\n{% endhero %}\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).NotTo(ContainSubstring("ALLOY_TPL"),
				"no placeholder artifacts must appear in output (issue #564)")
			Expect(html).NotTo(ContainSubstring("ELPMT"),
				"no placeholder artifacts must appear in output (issue #564)")
		})

		It("template tag in heading contributes to TOC text", func() {
			source := []byte("## {{ page.section_title }}\n\nBody text.\n")
			_, toc, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(1))
			Expect(toc[0].Text).To(ContainSubstring("{{ page.section_title }}"),
				"extractText must include TemplateTagInline node text in TOC entries — "+
					"headings containing template tags must produce usable TOC text (issue #564)")
		})
	})

	// ── Goldmark extensions (§6 footnotes, typographer) ──────────────

	Context("Goldmark extensions", func() {
		It("renders footnotes (§6 goldmark extensions)", func() {
			source := []byte("This has a footnote[^1].\n\n[^1]: This is the footnote text.\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			// Footnotes should produce a link and a footnote section
			Expect(html).To(ContainSubstring("footnote"),
				"footnotes extension must produce footnote markup")
		})

		It("applies typographer for smart quotes and em-dashes", func() {
			source := []byte("She said \"hello\" -- and left...\n")
			out, _, err := content.RenderMarkdown(source, defaultMD)
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

		It("escapes HTML entities in plain text content (issue #583)", func() {
			source := []byte("<script>alert('xss')</script> & \"quotes\"")
			out, err := content.RenderText(source)
			Expect(err).NotTo(HaveOccurred())
			rendered := string(out)
			Expect(rendered).NotTo(ContainSubstring("<script>"),
				"RenderText must escape HTML in .txt content — without escaping, "+
					"a content file containing <script> tags produces stored XSS "+
					"in the generated output (issue #583)")
			Expect(rendered).To(ContainSubstring("&lt;script&gt;"),
				"angle brackets must be escaped to HTML entities")
			Expect(rendered).To(ContainSubstring("&amp;"),
				"ampersands must be escaped to HTML entities")
			Expect(rendered).To(ContainSubstring("&lt;/script&gt;"),
				"closing tags must also be escaped")
			Expect(rendered).To(ContainSubstring("&#34;"),
				"double quotes must be escaped — defense-in-depth against "+
					"attribute-context injection if output is ever reused; "+
					"html.EscapeString uses numeric entity &#34; not named &quot;")
		})

		It("escapes iframe, img, and event handler XSS vectors (issue #583)", func() {
			source := []byte(`<iframe src="evil.com"></iframe><img onerror="alert(1)" src=x><a href="javascript:void(0)">click</a>`)
			out, err := content.RenderText(source)
			Expect(err).NotTo(HaveOccurred())
			rendered := string(out)
			Expect(rendered).NotTo(ContainSubstring("<iframe"),
				"iframe tags must be escaped — embedding arbitrary frames "+
					"is stored XSS (issue #583)")
			Expect(rendered).NotTo(ContainSubstring("<img"),
				"img tags with event handlers must be escaped — "+
					"onerror/onload execute arbitrary JS (issue #583)")
			Expect(rendered).NotTo(ContainSubstring("<a "),
				"anchor tags with javascript: hrefs must be escaped (issue #583)")
			Expect(rendered).To(ContainSubstring("&lt;iframe"),
				"iframe must appear as escaped entity")
			Expect(rendered).To(ContainSubstring("&lt;img"),
				"img must appear as escaped entity")
		})

		It("preserves pre wrapping after escaping (issue #583)", func() {
			source := []byte("<b>bold</b> & <i>italic</i>")
			out, err := content.RenderText(source)
			Expect(err).NotTo(HaveOccurred())
			rendered := string(out)
			Expect(rendered).To(HavePrefix("<pre>"),
				"escaped content must still be wrapped in <pre> — "+
					"escaping must not break the wrapper element")
			Expect(rendered).To(HaveSuffix("</pre>"),
				"closing </pre> tag must be present after escaped content")
			Expect(rendered).To(ContainSubstring("&lt;b&gt;"),
				"HTML tags inside text content must be escaped, not rendered")
		})
	})

	// ── Auto heading IDs (issue #274) ─────────────────────────────
	// Goldmark must generate id attributes on all headings by default.

	Describe("Auto heading IDs", func() {
		// defaultOpts has AutoHeadingID: true (production default)

		It("generates id attributes on headings", func() {
			out, _, err := content.RenderMarkdown(
				[]byte("## Getting Started\n\n### Installation"),
				defaultMD)
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="getting-started"`),
				"h2 must have an auto-generated slugified id attribute")
			Expect(html).To(ContainSubstring(`id="installation"`),
				"h3 must have an auto-generated slugified id attribute")
		})

		It("handles duplicate headings with numeric suffix", func() {
			out, _, err := content.RenderMarkdown(
				[]byte("## Overview\n\nText.\n\n## Overview\n\nMore text.\n\n## Overview"),
				defaultMD)
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
			out, _, err := content.RenderMarkdown(
				[]byte("## My Section {#custom-id}"),
				defaultMD)
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
			_, toc, err := content.RenderMarkdown([]byte(input), defaultMD)
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
			_, toc, err := content.RenderMarkdown([]byte(input), defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(2),
				"h1 must be excluded from TOC — it is the page title")
			Expect(toc[0].Text).To(Equal("Section One"))
			Expect(toc[1].Text).To(Equal("Section Two"))
		})

		It("returns empty TOC for pages with no headings", func() {
			input := "Just a paragraph of text."
			_, toc, err := content.RenderMarkdown([]byte(input), defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(BeEmpty(),
				"pages without headings must have an empty TOC")
		})

		It("uses manual {#id} override in TOC entry", func() {
			input := "## My Section {#custom-id}"
			_, toc, err := content.RenderMarkdown([]byte(input), defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(1))
			Expect(toc[0].ID).To(Equal("custom-id"),
				"TOC must use the manually overridden id, not the auto-generated slug")
		})

		It("nests h4 under h3 under h2", func() {
			input := "## Top\n\n### Mid\n\n#### Deep"
			_, toc, err := content.RenderMarkdown([]byte(input), defaultMD)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(1))
			Expect(toc[0].Children).To(HaveLen(1))
			Expect(toc[0].Children[0].Children).To(HaveLen(1),
				"h4 must nest under h3 which nests under h2")
			Expect(toc[0].Children[0].Children[0].Text).To(Equal("Deep"))
		})
	})

	// ── TOCEntry JSON serialization (issue #592) ─────────────────
	// content.TOCEntry must serialize to JSON with lowercase keys
	// matching the plugin hook payload contract. Without JSON tags,
	// encoding/json uses uppercase field names (ID, Text, Level,
	// Children), which breaks plugin hook payloads.

	Describe("TOCEntry JSON serialization (issue #592)", func() {
		It("serializes with lowercase JSON keys", func() {
			toc := content.TOCEntry{
				ID:    "section",
				Text:  "Section",
				Level: 2,
				Children: []content.TOCEntry{
					{ID: "subsection", Text: "Subsection", Level: 3},
				},
			}

			data, err := json.Marshal(toc)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).To(HaveKeyWithValue("id", "section"),
				"content.TOCEntry must serialize ID as lowercase 'id' — "+
					"without JSON tags, encoding/json uses uppercase 'ID' which "+
					"breaks plugin hook payloads (issue #592)")
			Expect(parsed).To(HaveKeyWithValue("text", "Section"))
			Expect(parsed).To(HaveKeyWithValue("level", float64(2)))
			Expect(parsed).To(HaveKey("children"))

			children, ok := parsed["children"].([]interface{})
			Expect(ok).To(BeTrue())
			Expect(children).To(HaveLen(1))

			child, ok := children[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(child).To(HaveKeyWithValue("id", "subsection"))
		})

		It("omits children when empty", func() {
			toc := content.TOCEntry{
				ID:       "leaf",
				Text:     "Leaf",
				Level:    3,
				Children: []content.TOCEntry{},
			}

			data, err := json.Marshal(toc)
			Expect(err).NotTo(HaveOccurred())

			var parsed map[string]interface{}
			Expect(json.Unmarshal(data, &parsed)).To(Succeed())

			Expect(parsed).NotTo(HaveKey("children"),
				"content.TOCEntry must use omitempty on children — "+
					"leaf entries should not include an empty children array "+
					"in plugin hook payloads (issue #592)")
		})
	})

	// ── Render hooks (issue #273, #310) ───────────────────────────
	// Render hook templates in layouts/_markup/ override how specific
	// markdown elements are rendered to HTML.
	// Tests must provide a HookRenderer callback — when HookRenderer
	// is nil, hooks are ignored (issue #310).

	Describe("Render hooks", func() {
		// HookRenderer wraps Liquid template rendering. The content
		// package cannot import template (circular dep) in production
		// code, but the external test package (content_test) can.
		// Tests use the real Liquid engine to match the production path.
		hookRenderer := func(templateSrc string, ctx map[string]interface{}) (string, error) {
			engine := tmpl.NewLiquidEngine()
			tpl, err := engine.Parse("hook", []byte(templateSrc))
			if err != nil {
				return "", err
			}
			result, err := tpl.Render(ctx)
			if err != nil {
				return "", err
			}
			return string(result), nil
		}

		It("render-codeblock.liquid overrides fenced code block output", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"codeblock": `<rh-code-block language="{{ markup.language }}">{{ markup.inner }}</rh-code-block>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("```javascript\nconsole.log('hello');\n```"), content.CreateGoldmark(opts))
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
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("[Click here](https://example.com)"), content.CreateGoldmark(opts))
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
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("## My Section"), content.CreateGoldmark(opts))
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
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte(`![A photo](/photo.jpg "Photo caption")`), content.CreateGoldmark(opts))
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
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("> This is a note"), content.CreateGoldmark(opts))
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
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("| A | B |\n|---|---|\n| 1 | 2 |"), content.CreateGoldmark(opts))
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
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("```mermaid\ngraph TD;\nA-->B;\n```"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`<div class="mermaid">`),
				"language-specific hook (render-codeblock-mermaid) must take precedence over generic")
			Expect(html).NotTo(ContainSubstring(`class="default"`),
				"generic codeblock hook must not be used when language-specific exists")
		})

		It("escapes Liquid delimiters in markup.inner for codeblock hooks", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"codeblock": `<alloy-code lang="{{ markup.language }}">{{ markup.inner }}</alloy-code>`,
				},
				HookRenderer: hookRenderer,
			}
			md := "```liquid\n{% for post in collections.blog %}\n  {{ post.title }}\n{% endfor %}\n```"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<alloy-code"),
				"render hook must produce custom element")
			Expect(html).NotTo(ContainSubstring("{%"),
				"Liquid control tags in code content must be escaped before reaching the hook template")
			Expect(html).NotTo(ContainSubstring("{{"),
				"Liquid expression tags in code content must be escaped before reaching the hook template")
			Expect(html).To(ContainSubstring("&#123;%"),
				"Liquid control tags must be entity-encoded in markup.inner")
			Expect(html).NotTo(ContainSubstring("%}"),
				"Liquid control closing tags in code content must be escaped before reaching the hook template")
			Expect(html).To(ContainSubstring("%&#125;"),
				"Liquid control closing tags must be entity-encoded in markup.inner")
			Expect(html).To(ContainSubstring("&#123;&#123;"),
				"Liquid expression tags must be entity-encoded in markup.inner")
			Expect(html).NotTo(ContainSubstring("}}"),
				"Liquid expression closing tags in code content must be escaped before reaching the hook template")
			Expect(html).To(ContainSubstring("&#125;&#125;"),
				"Liquid expression closing tags must be entity-encoded in markup.inner")
		})

		It("escapes Liquid delimiters in markup.inner for language-specific codeblock hooks", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"codeblock-liquid": `<div class="liquid-example">{{ markup.inner }}</div>`,
				},
				HookRenderer: hookRenderer,
			}
			md := "```liquid\n{{ page.title }}\n```"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`class="liquid-example"`),
				"language-specific hook must be used")
			Expect(html).NotTo(ContainSubstring("{{ page"),
				"Liquid expressions must not survive unescaped into language-specific hook output")
			Expect(html).To(ContainSubstring("&#123;&#123;"),
				"Liquid expressions must be entity-encoded in language-specific hook output")
		})

		It("does not alter code content without Liquid syntax in codeblock hooks", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"codeblock": `<rh-code-block>{{ markup.inner }}</rh-code-block>`,
				},
				HookRenderer: hookRenderer,
			}
			md := "```go\nfmt.Println(\"hello\")\n```"
			out, _, err := content.RenderMarkdown([]byte(md), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("fmt.Println"),
				"code without Liquid syntax must pass through unmodified")
		})

		It("falls back to default rendering when no hook exists", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks:        map[string]string{},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("```go\nfmt.Println(\"hello\")\n```"), content.CreateGoldmark(opts))
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
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("[External](https://example.com) and [Internal](/about)"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`target="_blank"`),
				"external link must have target=_blank via markup.is_external")
			Expect(html).To(ContainSubstring(`<a href="/about">Internal</a>`),
				"internal link must not have target=_blank")
		})

		// ── Render hook context enrichment (issue #824) ──────────────
		// These tests verify that render hooks receive the full context
		// documented in PLAN.md: heading attributes, link title, and
		// heading inner HTML vs plain text.

		It("render-link.liquid receives markup.title from link title (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"link": `<a href="{{ markup.destination }}" title="{{ markup.title }}">{{ markup.text }}</a>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte(`[Click here](https://example.com "Link tooltip")`), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`title="Link tooltip"`),
				"markup.title must contain the link title from [text](url \"title\") syntax")
		})

		It("render-link.liquid markup.title is empty when no title is provided (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				Hooks: map[string]string{
					"link": `<a href="{{ markup.destination }}" title="{{ markup.title }}">{{ markup.text }}</a>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte(`[Click here](https://example.com)`), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`title=""`),
				"markup.title must be empty string when link has no title")
		})

		It("render-heading.liquid receives markup.inner as rendered HTML (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				AutoHeadingID: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} id="{{ markup.id }}">{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("## Hello **world**"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<strong>world</strong>"),
				"markup.inner must contain rendered HTML, not plain text — inline formatting must be preserved")
		})

		It("render-heading.liquid receives markup.text as plain text (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				AutoHeadingID: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }}><span class="text">{{ markup.text }}</span>{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("## Hello **world**"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`<span class="text">Hello world</span>`),
				"markup.text must be plain text with all inline formatting stripped")
			Expect(html).To(ContainSubstring("<strong>world</strong>"),
				"markup.inner must still contain the rendered HTML version")
		})

		It("render-heading.liquid receives markup.attributes from goldmark attribute syntax (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				AutoHeadingID: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} id="{{ markup.attributes.id }}" class="{{ markup.attributes.class }}">{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("## My Section {.highlight #custom-id}"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="custom-id"`),
				"markup.attributes.id must contain the id from {#custom-id} syntax")
			Expect(html).To(ContainSubstring(`class="highlight"`),
				"markup.attributes.class must contain the class from {.highlight} syntax")
		})

		It("render-heading.liquid markup.attributes includes arbitrary key-value pairs (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				AutoHeadingID: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} data-section="{{ markup.attributes.data-section }}">{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte(`## Intro {data-section="hero"}`), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`data-section="hero"`),
				"markup.attributes must include arbitrary key=value attributes from goldmark syntax")
		})

		It("render-heading.liquid renders without error when no attributes are present (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				AutoHeadingID: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} data-attrs="{{ markup.attributes }}">{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("## Plain Heading"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring("<h2"),
				"heading hook must still work when no attributes are present")
			Expect(html).To(ContainSubstring("Plain Heading"),
				"heading content must render correctly without attributes")
		})

		It("render-heading.liquid markup.id falls back to auto-slug when {.class} present without {#id} (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				AutoHeadingID: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} id="{{ markup.id }}" class="{{ markup.attributes.class }}">{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("## My Title {.featured}"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="my-title"`),
				"markup.id must fall back to auto-generated slug when no {#id} attribute is present")
			Expect(html).To(ContainSubstring(`class="featured"`),
				"markup.attributes.class must still be populated from {.class} syntax")
		})

		It("render-heading.liquid markup.id still works with attributes override (issue #824)", func() {
			opts := content.MarkdownOptions{
				Unsafe: true, Typographer: true, TemplateTags: true,
				AutoHeadingID: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} id="{{ markup.id }}">{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer: hookRenderer,
			}
			out, _, err := content.RenderMarkdown([]byte("## My Section {#custom-id}"), content.CreateGoldmark(opts))
			Expect(err).NotTo(HaveOccurred())
			html := string(out)
			Expect(html).To(ContainSubstring(`id="custom-id"`),
				"markup.id must reflect the custom id from {#custom-id} attribute syntax, not the auto-generated slug")
		})
	})

	// ── Shared goldmark instance (issue #353, #700) ────────────────
	// RenderMarkdown accepts a pre-built goldmark.Markdown instance
	// and returns ([]byte, []TOCEntry, error). The caller creates the
	// instance once via CreateGoldmark and reuses it across all page
	// renders. No convenience wrappers — single consolidated API.

	Describe("Shared goldmark instance (issue #353)", func() {
		It("RenderMarkdown accepts a pre-built goldmark.Markdown instance (issue #353, #700)", func() {
			opts := content.MarkdownOptions{
				Unsafe:        true,
				Typographer:   true,
				TemplateTags:  true,
				AutoHeadingID: true,
			}
			md := content.CreateGoldmark(opts)

			source := []byte("## Hello World\n\nA paragraph.\n")
			html, toc, err := content.RenderMarkdown(source, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(html)).To(ContainSubstring("<h2"),
				"RenderMarkdown must accept a pre-built goldmark.Markdown "+
					"instance — the pipeline creates one instance per build via "+
					"CreateGoldmark and passes it to RenderMarkdown for all "+
					"page renders (issue #353, #700)")
			Expect(toc).To(HaveLen(1),
				"RenderMarkdown must return TOC as the second value — "+
					"always extracts headings, callers that don't need TOC "+
					"discard with _ (issue #353, #700)")
			Expect(toc[0].Text).To(Equal("Hello World"))
		})

		It("RenderMarkdown returns empty TOC for pages without headings (issue #353, #700)", func() {
			opts := content.MarkdownOptions{
				Unsafe:        true,
				Typographer:   true,
				TemplateTags:  true,
				AutoHeadingID: true,
			}
			md := content.CreateGoldmark(opts)

			source := []byte("Just a paragraph, no headings.\n")
			html, toc, err := content.RenderMarkdown(source, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(html)).To(ContainSubstring("<p>Just a paragraph"),
				"content must render correctly")
			Expect(toc).To(BeEmpty(),
				"RenderMarkdown must return empty TOC when no headings "+
					"exist — callers like BuildPhase1 discard the TOC with _ "+
					"(issue #353, #700)")
		})

		It("reusing the same goldmark instance across multiple pages produces correct output (issue #353)", func() {
			opts := content.MarkdownOptions{
				Unsafe:        true,
				Typographer:   true,
				TemplateTags:  true,
				AutoHeadingID: true,
			}
			md := content.CreateGoldmark(opts)

			pages := []struct {
				source  string
				heading string
			}{
				{"## Introduction\n\nFirst page content.\n", "Introduction"},
				{"## Getting Started\n\n### Installation\n\nSecond page.\n", "Getting Started"},
				{"## API Reference\n\n### Methods\n\n#### Get\n\nThird page.\n", "API Reference"},
			}

			for i, page := range pages {
				html, toc, err := content.RenderMarkdown([]byte(page.source), md)
				Expect(err).NotTo(HaveOccurred(),
					"page %d must render without error using shared goldmark instance", i+1)
				Expect(string(html)).To(ContainSubstring(page.heading),
					"page %d must contain its heading — goldmark.Markdown.Convert "+
						"is stateless between calls, so a shared instance must "+
						"produce identical results to per-call allocation "+
						"(issue #353, #700)", i+1)
				Expect(toc).NotTo(BeEmpty(),
					"page %d must have TOC entries from "+
						"RenderMarkdown (issue #353, #700)", i+1)
				Expect(toc[0].Text).To(Equal(page.heading),
					"page %d TOC must reflect that page's headings, not leak "+
						"state from previous renders (issue #353)", i+1)
			}
		})

		It("shared goldmark instance produces correct TOC nesting (issue #353)", func() {
			opts := content.MarkdownOptions{
				Unsafe:        true,
				Typographer:   true,
				TemplateTags:  true,
				AutoHeadingID: true,
			}
			md := content.CreateGoldmark(opts)

			source := []byte("## Top\n\n### Mid\n\n#### Deep\n")
			_, toc, err := content.RenderMarkdown(source, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(toc).To(HaveLen(1))
			Expect(toc[0].Children).To(HaveLen(1))
			Expect(toc[0].Children[0].Children).To(HaveLen(1),
				"TOC nesting must work correctly with RenderMarkdown — "+
					"the two-step parse/render/walk behavior must be preserved "+
					"(issue #353, #700)")
		})

		It("shared goldmark instance works with render hooks (issue #353)", func() {
			hookRenderer := func(templateSrc string, ctx map[string]interface{}) (string, error) {
				engine := tmpl.NewLiquidEngine()
				tpl, err := engine.Parse("hook", []byte(templateSrc))
				if err != nil {
					return "", err
				}
				result, err := tpl.Render(ctx)
				if err != nil {
					return "", err
				}
				return string(result), nil
			}

			opts := content.MarkdownOptions{
				Unsafe:       true,
				Typographer:  true,
				TemplateTags: true,
				Hooks: map[string]string{
					"heading": `<h{{ markup.level }} id="{{ markup.id }}" class="custom">{{ markup.inner }}</h{{ markup.level }}>`,
				},
				HookRenderer:  hookRenderer,
				AutoHeadingID: true,
			}
			md := content.CreateGoldmark(opts)

			source1 := []byte("## First Page\n\nContent.\n")
			html1, toc1, err := content.RenderMarkdown(source1, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(html1)).To(ContainSubstring(`class="custom"`),
				"render hooks must work with a pre-built goldmark instance — "+
					"the pipeline creates one goldmark per build with hooks "+
					"fully configured (issue #353, #700)")
			Expect(toc1).To(HaveLen(1))

			source2 := []byte("## Second Page\n\n### Sub Section\n")
			html2, toc2, err := content.RenderMarkdown(source2, md)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(html2)).To(ContainSubstring(`class="custom"`),
				"render hooks must continue working on subsequent renders "+
					"with the same goldmark instance (issue #353, #700)")
			Expect(toc2).To(HaveLen(1),
				"TOC must reflect second page's headings, not first page's")
			Expect(toc2[0].Text).To(Equal("Second Page"))
			Expect(toc2[0].Children).To(HaveLen(1))
		})
	})
})
