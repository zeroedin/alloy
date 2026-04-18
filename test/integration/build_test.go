package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	// ── Issue #113: i18n index page URL doubling ─────────────────────
	// Index pages in i18n mode must not get the language prefix doubled.
	// Root language (en, root:true): content/en/index.md → /
	// Non-root language (fr): content/fr/index.md → /fr/
	// Bug: index pages get /en/ or /fr/fr/ instead.

	Describe("i18n index page URLs", func() {
		It("root language index page resolves to / not /en/", func() {
			cfgPath := filepath.Join(fixtureDir("i18n-basic"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent).NotTo(BeNil())

			// Find the English index page — should be at / not /en/
			found := false
			for path, html := range result.RenderedContent {
				if path == "index.md" || path == "en/index.md" {
					found = true
					Expect(html).To(ContainSubstring(`<div class="url">/</div>`),
						fmt.Sprintf("root language index page (%s) must have URL / not /en/ — "+
							"language prefix must not be added for root language", path))
					break
				}
			}
			Expect(found).To(BeTrue(),
				"English index page must be present in RenderedContent")
		})

		It("non-root language index page resolves to /fr/ not /fr/fr/", func() {
			cfgPath := filepath.Join(fixtureDir("i18n-basic"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent).NotTo(BeNil())

			// Find the French index page — should be at /fr/ not /fr/fr/
			found := false
			for path, html := range result.RenderedContent {
				if path == "fr/index.md" {
					found = true
					Expect(html).To(ContainSubstring(`<div class="url">/fr/</div>`),
						fmt.Sprintf("non-root language index page (%s) must have URL /fr/ not /fr/fr/ — "+
							"language prefix must not be doubled", path))
					Expect(html).NotTo(ContainSubstring(`/fr/fr/`),
						"language prefix must not be doubled for index pages")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"French index page must be present in RenderedContent")
		})
	})

	// ── Issue #107: External data sources through pipeline ───────────
	// The pipeline must read sources: config, fetch data, and merge
	// into site.data so templates can access it.

	Describe("External data sources through pipeline", func() {
		It("REST source data is available in template rendering", func() {
			// Start a test HTTP server that returns JSON
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]map[string]interface{}{
					{"title": "First Post"},
					{"title": "Second Post"},
				})
			}))
			defer ts.Close()

			// Load the data-sources fixture — config has sources: block with
			// placeholder URL. Patch only the URL to point at test server.
			cfgPath := filepath.Join(fixtureDir("data-sources"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg.Sources).To(HaveKey("api_posts"),
				"fixture config must have sources.api_posts defined in YAML")

			// Patch the URL to point at the test server (can't be static in YAML)
			cfg.Sources["api_posts"].URL = ts.URL + "/posts"

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"Build with sources: config must succeed")
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent).NotTo(BeNil())

			// The template uses {{ site.data.api_posts }} — verify the
			// fetched data appears in the rendered output
			indexHTML, ok := result.RenderedContent["index.md"]
			Expect(ok).To(BeTrue(),
				"index.md must be present in RenderedContent")
			Expect(indexHTML).To(ContainSubstring("First Post"),
				"REST source data must be available in template as site.data.api_posts — "+
					"proves the pipeline fetches, merges, and renders external data")
			Expect(indexHTML).To(ContainSubstring("Second Post"),
				"all fetched records must be available in template")
		})
	})

	// ── Issue #141: Cascade through pipeline (verification) ──────────
	// The existing cascade test passes. This additional test verifies
	// the cascade works specifically for the bug scenario: a page in a
	// subdirectory WITHOUT its own _data.yaml inherits from ancestors.

	Describe("Cascade inheritance without own _data.yaml", func() {
		It("page in nested dir without _data.yaml inherits from all ancestors", func() {
			cfgPath := filepath.Join(fixtureDir("cascade"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent).NotTo(BeNil())

			// leaf.md is in content/blog/deep/nested/ which has NO _data.yaml.
			// It must inherit from blog/deep/_data.yaml AND blog/_data.yaml
			// AND content/_data.yaml through the cascade chain.
			leafHTML, ok := result.RenderedContent["blog/deep/nested/leaf.md"]
			Expect(ok).To(BeTrue(),
				"leaf.md must be in RenderedContent")

			// These values come from DIFFERENT ancestor levels — proves
			// cascade walks the full chain, not just nearest parent.
			Expect(leafHTML).To(ContainSubstring("Deep Author"),
				"must inherit author.name from blog/deep/_data.yaml")
			Expect(leafHTML).To(ContainSubstring("@blogauthor"),
				"must inherit author.twitter from blog/_data.yaml (deep merge)")
			Expect(leafHTML).To(ContainSubstring("deep-dive"),
				"must inherit category from blog/deep/_data.yaml")
			Expect(leafHTML).To(ContainSubstring("post"),
				"must inherit layout from blog/_data.yaml")
		})
	})
})
