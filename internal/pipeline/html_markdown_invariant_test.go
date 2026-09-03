package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Markdown-only Goldmark invariant (issue #1249) ────────────────────
//
// No pipeline path may hand an `.html` content body to Goldmark. Format
// detection decides: Goldmark runs for `.md` only, everything else reaches
// the template engine as authored.
//
// This was asserted nowhere, which is how `BuildPhase1` drifted into running
// Goldmark on every discovered page regardless of extension. Features are
// specified against this invariant — the raw block shortcode `>` modifier
// (issue #1245) is defined as *meaningless* in `.html` precisely because no
// Goldmark stage exists there to interpret it — so a violation silently gives
// `.html` files Markdown semantics the rest of the spec says they don't have.
//
// These are guard tests: they pass against `renderPages` today and must keep
// passing. Spec: PLAN.md → Phase 1 → "Markdown-only Goldmark invariant".

var _ = Describe("Markdown-only Goldmark invariant (issue #1249)", func() {

	const helmetPlugin = `export default function(alloy) {
  alloy.shortcode("helmet", function(args, content) {
    return '<div class="helmet">' + content + '</div>';
  });
}`

	liquidSite := func(body string) map[string]string {
		return map[string]string{
			"content/page.html":      "---\ntitle: Page\nlayout: default\n---\n" + body,
			"layouts/default.liquid": `<html><body>{{ content }}</body></html>`,
			"plugins/shortcodes.js":  helmetPlugin,
		}
	}

	liquidCfg := func() *config.Config {
		return &config.Config{
			Title:   "HTML Invariant Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
	}

	// ── No Markdown semantics in .html bodies ─────────────────────────

	Context("An .html content body is never Markdown-processed", func() {
		It("leaves Markdown constructs as literal text", func() {
			result, err := pipeline.BuildWithContent(liquidCfg(),
				liquidSite("# not a heading\n**not bold**\n- not a list\n"))
			Expect(err).NotTo(HaveOccurred())
			html := result.RenderedContent["page.html"]
			Expect(html).To(ContainSubstring("# not a heading"),
				"an .html body must reach output as authored")
			Expect(html).To(ContainSubstring("**not bold**"))
			Expect(html).To(ContainSubstring("- not a list"))
			Expect(html).NotTo(ContainSubstring("<h1"),
				"Goldmark must never run on an .html body")
			Expect(html).NotTo(ContainSubstring("<strong>"))
			Expect(html).NotTo(ContainSubstring("<ul>"))
		})

		It("does not entity-escape or apply typographer substitutions", func() {
			// Deliberately bare text, not wrapped in an HTML element: Goldmark
			// passes raw HTML blocks through untouched under unsafe: true, so
			// a payload inside <div> or <script> would survive even when the
			// invariant is broken and prove nothing.
			result, err := pipeline.BuildWithContent(liquidCfg(),
				liquidSite(`if (a < 2 && b > 1) { go("x -- y"); }`))
			Expect(err).NotTo(HaveOccurred())
			html := result.RenderedContent["page.html"]
			Expect(html).To(ContainSubstring(`if (a < 2 && b > 1) { go("x -- y"); }`),
				"an .html body must not be entity-escaped or typographed — "+
					"both are Markdown-stage transformations")
			Expect(html).NotTo(ContainSubstring("&lt;"))
			Expect(html).NotTo(ContainSubstring("&amp;"))
			Expect(html).NotTo(ContainSubstring("&ndash;"))
			Expect(html).NotTo(ContainSubstring("&rdquo;"))
			Expect(html).NotTo(ContainSubstring("<p>"),
				"no paragraph wrapping — that is Goldmark's doing")
		})

		It("does not turn an indented line into a code block", func() {
			result, err := pipeline.BuildWithContent(liquidCfg(),
				liquidSite("    indented four spaces\n"))
			Expect(err).NotTo(HaveOccurred())
			html := result.RenderedContent["page.html"]
			Expect(html).To(ContainSubstring("    indented four spaces"))
			Expect(html).NotTo(ContainSubstring("<pre><code>"),
				"indented-code-block parsing is Markdown-only")
		})

		It("passes a block shortcode body through unmodified", func() {
			// The premise of the raw block spec's .html carve-out: bodies in
			// .html files are already raw, which is why the > modifier has
			// nothing to do there.
			result, err := pipeline.BuildWithContent(liquidCfg(),
				liquidSite("{% helmet %}\n# not a heading\nvar a = 1 < 2 && 3 > 2;\n{% endhelmet %}\n"))
			Expect(err).NotTo(HaveOccurred())
			html := result.RenderedContent["page.html"]
			Expect(html).To(ContainSubstring(`<div class="helmet">`),
				"the shortcode must run")
			Expect(html).To(ContainSubstring("# not a heading"),
				"an .html shortcode body reaches the shortcode function byte-for-byte "+
					"with no raw modifier needed")
			Expect(html).To(ContainSubstring("var a = 1 < 2 && 3 > 2;"))
			Expect(html).NotTo(ContainSubstring("<h1"),
				"a Markdown heading in an .html shortcode body must stay literal")
			Expect(html).NotTo(ContainSubstring("&amp;"))
		})
	})

	// ── The raw block modifier is inert in .html ──────────────────────

	Context("The raw block > modifier is meaningless in .html (issue #1245)", func() {
		It("leaves the marker intact for Liquid to reject", func() {
			_, err := pipeline.BuildWithContent(liquidCfg(),
				liquidSite("{%> helmet %}\n<script>var a = 1;</script>\n{% endhelmet %}\n"))
			Expect(err).To(HaveOccurred(),
				"the > marker must survive an .html file and reach the engine — "+
					"the engine's native error is the specified safety net")
			Expect(err.Error()).To(ContainSubstring("unknown tag"),
				"liquidgo must be the thing that rejects it, proving no Goldmark "+
					"stage stripped the marker first")
			Expect(err.Error()).To(ContainSubstring("page.html"))
		})

		It("leaves the marker intact for Go templates to reject", func() {
			cfg := &config.Config{
				Title:     "HTML Invariant Test",
				BaseURL:   "https://example.com",
				Build:     config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"content/page.html": "---\ntitle: Page\nlayout: default\n---\n" +
					"{{%> helmet %}}\n<script>var a = 1;</script>\n{{% /helmet %}}\n",
				"layouts/default.html": `<html><body>{{ .content }}</body></html>`,
				"plugins/shortcodes.js": helmetPlugin,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"the > marker must survive an .html file on the Go template path too")
			Expect(err.Error()).To(ContainSubstring("unexpected closing tag"),
				"the block shortcode preprocessor must reject the unmatched close tag, "+
					"proving the {{%> open tag was never rewritten to native syntax")
		})

		It("does not raise the Markdown-stage unterminated error in .html", func() {
			// An unterminated raw block is a Goldmark-stage build error for .md.
			// In .html there is no Goldmark stage, so the file must fail (if at
			// all) through the engine instead — never through RenderMarkdown.
			_, err := pipeline.BuildWithContent(liquidCfg(),
				liquidSite("{%> helmet %}\n<script>var a = 1;</script>\n"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring("unterminated raw block shortcode"),
				"the Markdown-stage unterminated-block error must never fire for an "+
					".html file — reaching it proves Goldmark parsed an .html body")
			Expect(err.Error()).To(ContainSubstring("unknown tag"),
				"the engine's own error is the only failure mode here")
		})
	})
})
