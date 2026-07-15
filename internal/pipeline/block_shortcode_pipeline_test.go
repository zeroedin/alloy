package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Go template block shortcode pipeline integration (issue #1011) ────
// ProcessBlockShortcodes must be called from renderPages() when the
// engine is "gotemplate". Without pipeline wiring the function is dead
// code — users get a template parse error (Go's html/template chokes
// on `{{%`) or raw `{{% %}}` tags in output.
//
// These tests exercise the full build pipeline via BuildWithContent:
// plugin discovery → engine creation → Goldmark → ProcessBlockShortcodes
// → Go template rendering → layout application.

var _ = Describe("Go template block shortcode pipeline integration (issue #1011)", func() {

	// ── Basic pipeline wiring ─────────────────────────────────────────

	It("renders a block shortcode through the full gotemplate pipeline", func() {
		cfg := &config.Config{
			Title:     "Block Shortcode Pipeline Test",
			BaseURL:   "https://example.com",
			Build:     config.BuildConfig{Output: "_site"},
			Templates: config.TemplatesConfig{Engine: "gotemplate"},
		}
		contentMap := map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: default\n---\n" +
				"# Welcome\n\n" +
				"{{% callout \"warning\" %}}\n" +
				"Don't do this in production.\n" +
				"{{% /callout %}}\n",
			"layouts/default.html": `<!DOCTYPE html><html><body>{{ .content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("callout", function(args, content) {
    var type = args[0];
    return '<div class="callout callout--' + type + '">' + content + '</div>';
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"gotemplate build with block shortcodes must not error — "+
				"if this fails with a template parse error, ProcessBlockShortcodes "+
				"is not being called from renderPages() before template parsing")
		Expect(result).NotTo(BeNil())

		html := result.RenderedContent["index.md"]
		Expect(html).NotTo(BeEmpty(),
			"index page must render")
		Expect(html).To(ContainSubstring(`callout--warning`),
			"block shortcode callback output must appear in the final rendered page — "+
				"the pipeline must call ProcessBlockShortcodes with a callback that "+
				"routes through the plugin registry's CallShortcode")
		Expect(html).To(ContainSubstring(`<div class="callout callout--warning">`),
			"shortcode wrapper must be present in output with correct CSS class")
		Expect(html).NotTo(ContainSubstring("{{% callout"),
			"raw {{% callout opening tag must not appear in rendered output")
		Expect(html).NotTo(ContainSubstring("{{% /callout"),
			"raw {{% /callout closing tag must not appear in rendered output")
		Expect(html).To(ContainSubstring("production"),
			"inner content of the block shortcode must appear in the rendered output")
	})

	// ── Nested block shortcodes through pipeline ──────────────────────

	It("renders nested block shortcodes through the pipeline", func() {
		cfg := &config.Config{
			Title:     "Nested Block Shortcode Pipeline Test",
			BaseURL:   "https://example.com",
			Build:     config.BuildConfig{Output: "_site"},
			Templates: config.TemplatesConfig{Engine: "gotemplate"},
		}
		contentMap := map[string]string{
			"content/page.md": "---\ntitle: Nested\nlayout: default\n---\n" +
				"{{% wrapper \"outer\" %}}\n" +
				"Before inner.\n" +
				"{{% wrapper \"inner\" %}}\n" +
				"Deep content.\n" +
				"{{% /wrapper %}}\n" +
				"After inner.\n" +
				"{{% /wrapper %}}\n",
			"layouts/default.html": `<!DOCTYPE html><html><body>{{ .content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("wrapper", function(args, content) {
    var level = args[0];
    return '<section class="level-' + level + '">' + content + '</section>';
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"nested block shortcodes in gotemplate must build without error")
		Expect(result).NotTo(BeNil())

		html := result.RenderedContent["page.md"]
		Expect(html).NotTo(BeEmpty(),
			"page with nested block shortcodes must render")
		Expect(html).To(ContainSubstring(`level-inner`),
			"inner block shortcode must be rendered")
		Expect(html).To(ContainSubstring(`level-outer`),
			"outer block shortcode must be rendered")
		Expect(html).To(ContainSubstring("Deep content"),
			"innermost content must appear in final output")
		Expect(html).NotTo(ContainSubstring("{{% wrapper"),
			"no raw {{% wrapper tags must remain in output")
	})

	// ── Block shortcode with Go template expressions ──────────────────

	It("block shortcodes coexist with Go template expressions in the same page", func() {
		cfg := &config.Config{
			Title:     "Mixed Syntax Test",
			BaseURL:   "https://example.com",
			Build:     config.BuildConfig{Output: "_site"},
			Templates: config.TemplatesConfig{Engine: "gotemplate"},
		}
		contentMap := map[string]string{
			"content/mixed.md": "---\ntitle: Mixed Page\nlayout: default\n---\n" +
				"# {{ .page.title }}\n\n" +
				"{{% note %}}\n" +
				"This is a note on {{ .site.title }}.\n" +
				"{{% /note %}}\n",
			"layouts/default.html": `<!DOCTYPE html><html><body>{{ .content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("note", function(args, content) {
    return '<aside class="note">' + content + '</aside>';
  });
}`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"block shortcodes and Go template expressions must coexist — "+
				"ProcessBlockShortcodes must run before template rendering so "+
				"{{% %}} tags are replaced first, then {{ }} expressions are "+
				"evaluated by the Go template engine")
		Expect(result).NotTo(BeNil())

		html := result.RenderedContent["mixed.md"]
		Expect(html).NotTo(BeEmpty(),
			"page with mixed syntax must render")
		Expect(html).To(ContainSubstring("Mixed Page"),
			"Go template expression {{ .page.title }} must resolve to the page title")
		Expect(html).To(ContainSubstring("Mixed Syntax Test"),
			"Go template expression {{ .site.title }} inside block shortcode content "+
				"must resolve — the inner content is the shortcode callback's output, "+
				"and any remaining {{ }} expressions are processed by the Go template engine")
		Expect(html).To(ContainSubstring(`<aside class="note">`),
			"block shortcode output must appear in rendered page")
		Expect(html).NotTo(ContainSubstring("{{% note"),
			"raw block shortcode tags must not remain in output")
	})

	// ── Block shortcode errors propagate as build errors ──────────────

	It("propagates block shortcode errors as build errors", func() {
		cfg := &config.Config{
			Title:     "Error Propagation Test",
			BaseURL:   "https://example.com",
			Build:     config.BuildConfig{Output: "_site"},
			Templates: config.TemplatesConfig{Engine: "gotemplate"},
		}
		contentMap := map[string]string{
			"content/bad.md": "---\ntitle: Bad\nlayout: default\n---\n" +
				"{{% callout \"warning\" %}}\n" +
				"This is never closed.\n",
			"layouts/default.html": `<!DOCTYPE html><html><body>{{ .content }}</body></html>`,
			"plugins/shortcodes.js": `export default function(alloy) {
  alloy.shortcode("callout", function(args, content) {
    return '<div>' + content + '</div>';
  });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).To(HaveOccurred(),
			"unclosed block shortcode must produce a build error — "+
				"this requires ProcessBlockShortcodes to be called from "+
				"the pipeline and its errors propagated")
		Expect(err.Error()).To(ContainSubstring("callout"),
			"build error must reference the unclosed shortcode name")
	})

	// ── Block shortcode without plugin (no-op) ───────────────────────

	It("gotemplate build succeeds when content has no block shortcodes", func() {
		cfg := &config.Config{
			Title:     "No Block Shortcodes Test",
			BaseURL:   "https://example.com",
			Build:     config.BuildConfig{Output: "_site"},
			Templates: config.TemplatesConfig{Engine: "gotemplate"},
		}
		contentMap := map[string]string{
			"content/index.md":     "---\ntitle: Home\nlayout: default\n---\n# Welcome\n\nRegular content.",
			"layouts/default.html": `<!DOCTYPE html><html><body>{{ .content }}</body></html>`,
		}
		result, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"gotemplate build without block shortcodes must not error — "+
				"ProcessBlockShortcodes must be a no-op when no {{% %}} tags are present")
		Expect(result).NotTo(BeNil())

		html := result.RenderedContent["index.md"]
		Expect(html).To(ContainSubstring("Welcome"),
			"regular content must render normally")
	})
})
