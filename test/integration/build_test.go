package integration_test

import (
	"fmt"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

func fixtureDir(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "fixtures", name)
}

var _ = Describe("Full build pipeline", func() {
	Describe("Minimal site", func() {
		It("builds successfully with minimal fixture", func() {
			cfgPath := filepath.Join(fixtureDir("minimal"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(BeNumerically(">", 0),
				"minimal site must produce at least one page")
		})

		It("produces output for each content file", func() {
			cfgPath := filepath.Join(fixtureDir("minimal"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PagesRendered).To(ContainElement(ContainSubstring("index")),
				"must render the index page")
		})
	})

	Describe("Cascade site", func() {
		It("builds with data cascade fixture", func() {
			cfgPath := filepath.Join(fixtureDir("cascade"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		// ── Issue #141: 3-level deep cascade merge ────────────────────
		// The cascade fixture has 3 levels of _data.yaml:
		//   content/_data.yaml        → layout: default, author.name: Content Author
		//   content/blog/_data.yaml   → layout: post, author: {name: Blog Author, twitter: @blogauthor}
		//   content/blog/deep/_data.yaml → author.name: Deep Author, category: deep-dive
		//
		// A page at content/blog/deep/nested/leaf.md must inherit merged
		// cascade from all 3 ancestor levels.

		It("3-level deep cascade merges all ancestor values into rendered output", func() {
			cfgPath := filepath.Join(fixtureDir("cascade"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// RenderedContent is a map[string]string keyed by Page.RelPath
			// (e.g., "blog/deep/nested/leaf.md" — relative to the content
			// directory, no "content/" prefix). The developer must add this
			// field to BuildResult when implementing this test.
			Expect(result.RenderedContent).NotTo(BeNil(),
				"BuildResult must include RenderedContent map")

			// Look up by RelPath — relative to content dir, no "content/" prefix
			leafHTML, ok := result.RenderedContent["blog/deep/nested/leaf.md"]
			Expect(ok).To(BeTrue(),
				"leaf.md must be present in RenderedContent by source path")
			Expect(leafHTML).NotTo(BeEmpty(),
				"leaf.md must produce rendered HTML")

			// author.name from blog/deep/_data.yaml (deepest override)
			Expect(leafHTML).To(ContainSubstring("Deep Author"),
				"cascade must include author.name from blog/deep/_data.yaml")

			// author.twitter from blog/_data.yaml (inherited — not overridden by deep/)
			Expect(leafHTML).To(ContainSubstring("@blogauthor"),
				"cascade must deep-merge: author.twitter from blog/_data.yaml "+
					"must survive when blog/deep/_data.yaml only overrides author.name")

			// category from blog/deep/_data.yaml (new key at deep level)
			Expect(leafHTML).To(ContainSubstring("deep-dive"),
				"cascade must include category from blog/deep/_data.yaml")

			// layout value from blog/_data.yaml (inherited through deep/).
			// Note: this tests the cascade VALUE, not layout file selection.
			// default.liquid renders {{ page.layout }} which shows "post".
			// A post.liquid layout is not needed for this test.
			Expect(leafHTML).To(ContainSubstring("post"),
				"cascade must inherit layout value from blog/_data.yaml through deep/ level")
		})
	})

	Describe("Collections site", func() {
		It("builds with collections fixture", func() {
			cfgPath := filepath.Join(fixtureDir("collections"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("creates blog collection from fixture content", func() {
			cfgPath := filepath.Join(fixtureDir("collections"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(BeNumerically(">", 0),
				"collections site must produce pages")
		})
	})

	// ── Issue #171: Plugin shortcode bridging (positive test) ─────────
	// This tests the full pipeline path: plugin discovery → LoadPlugins →
	// RegisteredShortcodes → engine.AddTag → CallShortcode → rendered output.
	// The fixture has a JS plugin that registers "greeting" via alloy.shortcode().

	Describe("Plugin shortcode site", func() {
		It("plugin-registered shortcode renders in page output", func() {
			cfgPath := filepath.Join(fixtureDir("plugin-shortcodes"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed when plugin shortcode is properly bridged")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(BeNumerically(">", 0),
				"plugin-shortcodes fixture must produce at least one page")

			// The rendered output must contain the shortcode's HTML output,
			// not the raw {% greeting "Alloy" %} tag.
			Expect(result.RenderedContent).NotTo(BeNil(),
				"BuildResult must include RenderedContent map")
			indexHTML, ok := result.RenderedContent["index.md"]
			Expect(ok).To(BeTrue(),
				"index.md must be present in RenderedContent")
			Expect(indexHTML).To(ContainSubstring(`<p class="greeting">Hello, Alloy!</p>`),
				"plugin shortcode must render its HTML output in the page — "+
					"proves the full pipeline: plugin discovery → LoadPlugins → "+
					"RegisteredShortcodes → engine.AddTag → CallShortcode → rendered output")
			Expect(indexHTML).NotTo(ContainSubstring("{% greeting"),
				"raw shortcode tag must not appear in rendered output")
		})
	})

	// ── Issue #187: Hook firing order in i18n mode ───────────────────
	// onContentTransformed must fire BEFORE layout rendering in both
	// single-language and i18n modes. The i18n fixture has a plugin
	// that detects if the hook payload contains layout HTML (DOCTYPE,
	// <head>) — which would mean it fired AFTER layout rendering.

	Describe("i18n hook firing order", func() {
		It("onContentTransformed receives pre-layout HTML in i18n mode", func() {
			cfgPath := filepath.Join(fixtureDir("i18n-hooks"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// The hook-checker plugin injects "HOOK_FIRED_AFTER_LAYOUT:"
			// if it detects DOCTYPE or <head> in the payload. Check all
			// rendered pages for this violation marker.
			Expect(result.RenderedContent).NotTo(BeNil())
			for path, html := range result.RenderedContent {
				Expect(html).NotTo(ContainSubstring("HOOK_FIRED_AFTER_LAYOUT"),
					fmt.Sprintf("onContentTransformed in i18n mode must fire BEFORE layout rendering — "+
						"page %s received layout HTML in hook payload", path))
			}
		})
	})
})
