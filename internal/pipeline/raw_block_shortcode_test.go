package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Raw block shortcode pipeline integration (issue #1245) ────────────
//
// End-to-end proof that the `>` marker does what it exists to do: the body
// of a `{%> name %}` / `{{%> name %}}` invocation reaches the shortcode
// function byte-for-byte, and the marker itself never reaches the template
// engine. The content-layer contract is specified in
// internal/content/raw_block_shortcode_test.go; these tests exercise the
// full build: Goldmark → ProcessBlockShortcodes / Liquid → layout.
//
// Spec: PLAN.md → "Raw block shortcode content (issue #1245)".

var _ = Describe("Raw block shortcode pipeline integration (issue #1245)", func() {

	// The motivating payload: markdown-hostile script content that today is
	// mangled (entity-escaped, em-dashed, or stripped) before the shortcode
	// function ever sees it.
	const scriptBody = `<script type="application/ld+json">` + "\n" +
		`  { "name": "A -- B", "ok": 1 < 2 && 3 > 2 }` + "\n" +
		`</script>`

	It("passes a raw body to a Go template block shortcode verbatim", func() {
		cfg := &config.Config{
			Title:     "Raw Block Shortcode GoTemplate Test",
			BaseURL:   "https://example.com",
			Build:     config.BuildConfig{Output: "_site"},
			Templates: config.TemplatesConfig{Engine: "gotemplate"},
		}
		contentMap := map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: default\n---\n" +
				"# Welcome\n\n" +
				"{{%> helmet %}}\n" + scriptBody + "\n{{% /helmet %}}\n",
			"layouts/default.html": `<!DOCTYPE html><html><body>{{ .content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("helmet", function(args, content) {
    return '<div class="helmet">' + content + '</div>';
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"a raw block shortcode must build — if this fails on a template parse "+
				"error, the > marker is reaching the Go template engine instead of "+
				"being stripped by the Goldmark renderer")
		Expect(result).NotTo(BeNil())

		html := result.RenderedContent["index.md"]
		Expect(html).To(ContainSubstring(`<div class="helmet">`),
			"the shortcode callback must run")
		Expect(html).To(ContainSubstring(`{ "name": "A -- B", "ok": 1 < 2 && 3 > 2 }`),
			"the shortcode must receive the body byte-for-byte — no entity escaping, "+
				"no typographer substitution")
		Expect(html).NotTo(ContainSubstring("&amp;&amp;"),
			"&& must not be entity-escaped on the way to the shortcode")
		Expect(html).NotTo(ContainSubstring("&ndash;"),
			"-- must not be smartened on the way to the shortcode")
		Expect(html).NotTo(ContainSubstring("{{%"),
			"no {{% delimiters may survive into the rendered page")
		Expect(html).NotTo(ContainSubstring(">"+" helmet"),
			"the > marker must not appear in rendered output")
	})

	It("passes a raw body to a Liquid block shortcode verbatim", func() {
		cfg := &config.Config{
			Title:   "Raw Block Shortcode Liquid Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: default\n---\n" +
				"# Welcome\n\n" +
				"{%> helmet %}\n" + scriptBody + "\n{% endhelmet %}\n",
			"layouts/default.liquid": `<!DOCTYPE html><html><body>{{ content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("helmet", function(args, content) {
    return '<div class="helmet">' + content + '</div>';
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"a raw block shortcode must build — if this fails with an unknown tag "+
				"error, the > marker is reaching liquidgo instead of being stripped")
		Expect(result).NotTo(BeNil())

		html := result.RenderedContent["index.md"]
		Expect(html).To(ContainSubstring(`<div class="helmet">`),
			"the shortcode callback must run")
		Expect(html).To(ContainSubstring(`{ "name": "A -- B", "ok": 1 < 2 && 3 > 2 }`),
			"the shortcode must receive the body byte-for-byte")
		Expect(html).NotTo(ContainSubstring("&amp;&amp;"))
		Expect(html).NotTo(ContainSubstring("&ndash;"))
		Expect(html).NotTo(ContainSubstring("{%"),
			"no Liquid delimiters may survive into the rendered page")
	})

	It("keeps a raw body intact under goldmark.unsafe: false", func() {
		unsafeFalse := false
		cfg := &config.Config{
			Title:   "Raw Block Shortcode Unsafe Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		cfg.Content.Markdown.Goldmark.Unsafe = &unsafeFalse
		contentMap := map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: default\n---\n" +
				"{%> helmet %}\n<script>var a = 1;</script>\n{% endhelmet %}\n",
			"layouts/default.liquid": `<!DOCTYPE html><html><body>{{ content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("helmet", function(args, content) {
    return '<div class="helmet">' + content + '</div>';
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())

		html := result.RenderedContent["index.md"]
		Expect(html).To(ContainSubstring("<script>var a = 1;</script>"),
			"the > marker is an explicit author opt-in and overrides unsafe: false "+
				"for that block's body — otherwise the feature is unusable on sites "+
				"that disable raw HTML")
		Expect(html).NotTo(ContainSubstring("raw HTML omitted"))
	})

	It("fails the build with the page path when a raw block is unterminated", func() {
		cfg := &config.Config{
			Title:   "Raw Block Shortcode Unterminated Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/blog/post.md": "---\ntitle: Post\nlayout: default\n---\n" +
				"{%> helmet %}\n<script>var a = 1;</script>\n",
			"layouts/default.liquid": `<html><body>{{ content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("helmet", function(args, content) {
    return content;
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).To(HaveOccurred(),
			"an unterminated raw block must fail the build rather than silently "+
				"consuming the rest of the file")
		Expect(result).To(BeNil(),
			"a failed build must not return a partial result")
		Expect(err.Error()).To(ContainSubstring("unterminated raw block shortcode"),
			"the build error must name the construct")
		Expect(err.Error()).To(ContainSubstring("blog/post.md"),
			"renderPages wraps content transformation errors with the page path, "+
				"so the author knows which file to fix")
		Expect(err.Error()).To(ContainSubstring("{% endhelmet %}"),
			"the error must name the close tag the author needs to add")
	})

	It("leaves non-raw block shortcodes Markdown-processed", func() {
		cfg := &config.Config{
			Title:   "Raw Block Shortcode Regression Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: default\n---\n" +
				"{% helmet %}\n**bold** body\n{% endhelmet %}\n",
			"layouts/default.liquid": `<html><body>{{ content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("helmet", function(args, content) {
    return '<div class="helmet">' + content + '</div>';
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.RenderedContent["index.md"]).To(ContainSubstring("<strong>bold</strong>"),
			"a block shortcode without the > marker must keep today's behavior — "+
				"its body is Markdown-processed")
	})
})
