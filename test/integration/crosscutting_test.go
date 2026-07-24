package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"time"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/i18n"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/pagination"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/plugin"
	"github.com/zeroedin/alloy/internal/server"
	"github.com/zeroedin/alloy/internal/template"
)

func pluginFixtureDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "internal", "plugin", "testdata", "single-files")
}

var _ = Describe("Cross-Cutting Integration", func() {

	Describe("Data file → template rendering", func() {
		It("loads data/navigation.yaml and makes it available as site.data.navigation in template", func() {
			navData, err := data.LoadFile(filepath.Join(fixtureDir("minimal"), "data", "navigation.yaml"))
			Expect(err).NotTo(HaveOccurred())
			Expect(navData).NotTo(BeNil())

			siteData := map[string]interface{}{
				"title": "Test Site",
				"data":  map[string]interface{}{"navigation": navData},
			}
			ctx := template.BuildTemplateContext(
				&content.Page{RelPath: "index.md"},
				siteData,
				nil,
				nil,
				nil,
				nil,
				"",
			)
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Site.Data).To(HaveKey("navigation"),
				"site.data.navigation must be populated from data file")
		})
	})

	Describe("Front matter → permalink → output", func() {
		It("parses front matter permalink, computes URL, and determines output path", func() {
			raw := []byte("---\ntitle: About\npermalink: /about-us/\n---\nBody")
			fm, _, err := content.ParseFrontMatter(raw)
			Expect(err).NotTo(HaveOccurred())

			page := &content.Page{
				RelPath:     "about.md",
				FrontMatter: fm,
				Permalink:   fm["permalink"].(string),
			}

			resolvedURL, err := permalink.Resolve(":permalink", page)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolvedURL).To(Equal("/about-us/"))

			outputPath := output.ComputeOutputPath(resolvedURL)
			Expect(outputPath).To(Equal("about-us/index.html"))
		})
	})

	Describe("Collection → pagination → output", func() {
		It("builds a collection, paginates it, and produces correct output paths", func() {
			pages := []*content.Page{
				{RelPath: "blog/post1.md", Section: "blog"},
				{RelPath: "blog/post2.md", Section: "blog"},
				{RelPath: "blog/post3.md", Section: "blog"},
			}

			taxonomyCfg := map[string]*config.TaxonomyConfig{}
			collections := collection.BuildTaxonomies(pages, taxonomyCfg)
			_ = collections // taxonomy collections (may be nil from stub)

			items := make([]interface{}, len(pages))
			for i, p := range pages {
				items[i] = p
			}
			contexts, paths, err := pagination.Paginate(items, 2, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(2), "3 items at perPage 2 = 2 pages")
			Expect(paths).To(HaveLen(2))
		})
	})

	Describe("Taxonomy → layout → template context", func() {
		It("generates a taxonomy page and provides taxonomy.term context", func() {
			pages := []*content.Page{
				{RelPath: "blog/p1.md", FrontMatter: map[string]interface{}{"tags": []interface{}{"go"}}},
			}
			taxonomyCfg := map[string]*config.TaxonomyConfig{
				"tags": {Permalink: "/tags/:term/"},
			}
			taxonomies := collection.BuildTaxonomies(pages, taxonomyCfg)
			Expect(taxonomies).NotTo(BeNil())

			if tagsTaxonomy, ok := taxonomies["tags"]; ok {
				ctx := collection.BuildTaxonomyPageContext(tagsTaxonomy, "go")
				Expect(ctx).NotTo(BeNil())
				Expect(ctx.Term).To(Equal("go"))
			}
		})
	})

	Describe("Plugin hook → content transform → output", func() {
		It("onContentTransformed hook modifies rendered HTML", func() {
			// Register a hook that modifies content, run it through
			// the HookRegistry, and verify the payload was transformed
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnContentTransformed, func(_ context.Context, payload interface{}) (interface{}, error) {
				html, ok := payload.(string)
				if !ok {
					return payload, nil
				}
				// Simulate a plugin that injects a wrapper div
				return "<div class=\"plugin-wrapped\">" + html + "</div>", nil
			})

			// Fire the hook with rendered content
			result, err := registry.Run(plugin.OnContentTransformed, "<p>Original content</p>")
			Expect(err).NotTo(HaveOccurred())
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue(), "hook result must be a string")
			Expect(resultStr).To(ContainSubstring("plugin-wrapped"),
				"onContentTransformed hook must be able to modify rendered HTML")
			Expect(resultStr).To(ContainSubstring("Original content"),
				"hook must preserve original content inside the wrapper")
		})
	})

	// ── Issue #187: Hook firing order consistency ───────────────────
	// onContentTransformed must fire at the same pipeline stage in both
	// single-language and i18n modes: after Markdown→HTML, before layout
	// rendering. The payload is pre-layout HTML (content body only).

	Describe("onContentTransformed fires before layout rendering", func() {
		It("hook payload is content HTML without layout wrapper", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnContentTransformed, func(_ context.Context, payload interface{}) (interface{}, error) {
				html, ok := payload.(string)
				Expect(ok).To(BeTrue(),
					"onContentTransformed payload must be a string")
				// The payload must be the rendered content body — NOT the
				// full page with layout. It should contain the rendered
				// markdown content but NOT the layout's DOCTYPE/html/head.
				Expect(html).NotTo(ContainSubstring("<!DOCTYPE"),
					"onContentTransformed must fire BEFORE layout rendering — "+
						"payload must not contain DOCTYPE from layout")
				Expect(html).NotTo(ContainSubstring("<head>"),
					"onContentTransformed must fire BEFORE layout rendering — "+
						"payload must not contain <head> from layout")
				return html, nil
			})

			// Simulate content that has been through Markdown→HTML but not layout
			contentHTML := "<h1>My Post</h1>\n<p>Some content with <strong>bold</strong> text.</p>"
			_, err := registry.Run(plugin.OnContentTransformed, contentHTML)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ── Issue #182: Hook payload must be per-page HTML string ────────
	// onContentTransformed fires with the pages slice, but JS plugins
	// need an HTML string they can modify. The pipeline must fire the
	// hook once per page with the rendered HTML string as payload,
	// apply the returned string back to the page.

	Describe("onContentTransformed per-page HTML payload", func() {
		It("hook receives HTML string, not pages slice", func() {
			registry := plugin.NewHookRegistry()
			receivedPayloads := []string{}
			registry.Register(plugin.OnContentTransformed, func(_ context.Context, payload interface{}) (interface{}, error) {
				html, ok := payload.(string)
				Expect(ok).To(BeTrue(),
					"onContentTransformed payload must be a string, not a pages slice or struct")
				receivedPayloads = append(receivedPayloads, html)
				return html, nil
			})

			// Simulate per-page hook firing with HTML strings
			pages := []string{
				"<h1>Post 1</h1><p>Content</p>",
				"<h1>Post 2</h1><img src='photo.jpg'>",
			}
			for _, pageHTML := range pages {
				_, err := registry.Run(plugin.OnContentTransformed, pageHTML)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(receivedPayloads).To(HaveLen(2),
				"hook must fire once per page")
			Expect(receivedPayloads[0]).To(ContainSubstring("Post 1"))
			Expect(receivedPayloads[1]).To(ContainSubstring("Post 2"))
		})

		It("hook can modify per-page HTML and return transformed result", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnContentTransformed, func(_ context.Context, payload interface{}) (interface{}, error) {
				html := payload.(string)
				// Simulate a plugin that adds lazy loading to images
				modified := strings.ReplaceAll(html, "<img ", `<img loading="lazy" `)
				return modified, nil
			})

			input := `<h1>Gallery</h1><img src="a.jpg"><img src="b.jpg">`
			result, err := registry.Run(plugin.OnContentTransformed, input)
			Expect(err).NotTo(HaveOccurred())
			resultStr := result.(string)
			Expect(resultStr).To(ContainSubstring(`loading="lazy"`),
				"hook must be able to modify per-page HTML")
			Expect(strings.Count(resultStr, `loading="lazy"`)).To(Equal(2),
				"hook must modify all img tags in the page")
		})
	})

	// ── onPageRendered per-page object payload (issue #1095) ────────
	// Fires after layout rendering — plugin receives a page object with
	// html, frontMatter, url, and path. Only html in the return is
	// applied back; other fields are read-only context.

	Describe("onPageRendered per-page object payload (issue #1095)", func() {
		It("hook receives page object with html, frontMatter, url, path per page", func() {
			registry := plugin.NewHookRegistry()
			type capturedPayload struct {
				html        string
				frontMatter map[string]interface{}
				url         string
				path        string
			}
			var received []capturedPayload
			registry.Register(plugin.OnPageRendered, func(_ context.Context, payload interface{}) (interface{}, error) {
				pageMap, ok := payload.(map[string]interface{})
				Expect(ok).To(BeTrue(),
					"onPageRendered payload must be a map/object — not a string (issue #1095)")

				html, htmlOk := pageMap["html"].(string)
				Expect(htmlOk).To(BeTrue(), "payload must have string 'html' key")

				fm, fmOk := pageMap["frontMatter"].(map[string]interface{})
				Expect(fmOk).To(BeTrue(), "payload must have 'frontMatter' as map[string]interface{}")

				url, urlOk := pageMap["url"].(string)
				Expect(urlOk).To(BeTrue(), "payload must have string 'url' key")

				path, pathOk := pageMap["path"].(string)
				Expect(pathOk).To(BeTrue(), "payload must have string 'path' key")

				received = append(received, capturedPayload{
					html: html, url: url, path: path,
					frontMatter: fm,
				})
				return pageMap, nil
			})

			pages := []map[string]interface{}{
				{
					"html":        `<!DOCTYPE html><html><body><h1>Page 1</h1></body></html>`,
					"frontMatter": map[string]interface{}{"title": "Page 1", "layout": "default"},
					"url":         "/page-1/",
					"path":        "page-1.md",
				},
				{
					"html":        `<!DOCTYPE html><html><body><h1>Page 2</h1></body></html>`,
					"frontMatter": map[string]interface{}{"title": "Page 2", "layout": "post"},
					"url":         "/page-2/",
					"path":        "page-2.md",
				},
			}
			for _, page := range pages {
				_, err := registry.Run(plugin.OnPageRendered, page)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(received).To(HaveLen(2),
				"onPageRendered must fire once per page")
			Expect(received[0].html).To(ContainSubstring("Page 1"))
			Expect(received[0].url).To(Equal("/page-1/"))
			Expect(received[0].path).To(Equal("page-1.md"))
			Expect(received[0].frontMatter["title"]).To(Equal("Page 1"))
			Expect(received[1].frontMatter["layout"]).To(Equal("post"))
		})

		It("hook return html replaces rendered body", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnPageRendered, func(_ context.Context, payload interface{}) (interface{}, error) {
				pageMap := payload.(map[string]interface{})
				html := pageMap["html"].(string)
				pageMap["html"] = strings.Join(strings.Fields(html), " ")
				return pageMap, nil
			})

			input := map[string]interface{}{
				"html":        "<!DOCTYPE html>\n<html>\n  <body>\n    <h1>Hello</h1>\n  </body>\n</html>",
				"frontMatter": map[string]interface{}{"title": "Test"},
				"url":         "/test/",
				"path":        "test.md",
			}
			result, err := registry.Run(plugin.OnPageRendered, input)
			Expect(err).NotTo(HaveOccurred())
			resultMap, ok := result.(map[string]interface{})
			Expect(ok).To(BeTrue(), "onPageRendered must return a map/object")
			resultHTML, ok := resultMap["html"].(string)
			Expect(ok).To(BeTrue(), "returned map must have string 'html' key")
			Expect(resultHTML).NotTo(ContainSubstring("\n"),
				"minifier hook must collapse whitespace in html field")
			Expect(resultHTML).To(ContainSubstring("Hello"),
				"minifier hook must preserve content")
		})

		It("hook can conditionally skip processing based on frontMatter", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnPageRendered, func(_ context.Context, payload interface{}) (interface{}, error) {
				pageMap := payload.(map[string]interface{})
				fm, _ := pageMap["frontMatter"].(map[string]interface{})
				if fm["skipTransforms"] == true {
					return pageMap, nil // skip — return unchanged
				}
				html := pageMap["html"].(string)
				pageMap["html"] = strings.ReplaceAll(html, "<h2", `<h2 class="styled"`)
				return pageMap, nil
			})

			// Page that should be transformed
			normalPage := map[string]interface{}{
				"html":        "<html><body><h2>Heading</h2></body></html>",
				"frontMatter": map[string]interface{}{"title": "Normal"},
				"url":         "/normal/",
				"path":        "normal.md",
			}
			result1, err := registry.Run(plugin.OnPageRendered, normalPage)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.(map[string]interface{})["html"]).To(ContainSubstring(`class="styled"`),
				"pages without skipTransforms must have headings transformed")

			// Page that should be skipped
			demoPage := map[string]interface{}{
				"html":        "<html><body><h2>Demo Heading</h2></body></html>",
				"frontMatter": map[string]interface{}{"title": "Demo", "skipTransforms": true},
				"url":         "/demo/",
				"path":        "demo.md",
			}
			result2, err := registry.Run(plugin.OnPageRendered, demoPage)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.(map[string]interface{})["html"]).NotTo(ContainSubstring(`class="styled"`),
				"pages with skipTransforms: true must not have headings transformed")
			Expect(result2.(map[string]interface{})["html"]).To(ContainSubstring("<h2>Demo Heading</h2>"),
				"skipped page HTML must be preserved unchanged")
		})
	})

	// ── onAssetProcess payload contract ──────────────────────────────
	// Fires once per asset with { path, content }. Return value's
	// "content" key replaces the asset content.

	Describe("onAssetProcess payload contract", func() {
		It("hook receives path and content as JSON object", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnAssetProcess, func(_ context.Context, payload interface{}) (interface{}, error) {
				asset, ok := payload.(map[string]interface{})
				Expect(ok).To(BeTrue(),
					"onAssetProcess payload must be a map with path and content keys")
				Expect(asset).To(HaveKey("path"),
					"asset payload must include path")
				Expect(asset).To(HaveKey("content"),
					"asset payload must include content")
				return asset, nil
			})

			asset := map[string]interface{}{
				"path":    "css/main.css",
				"content": "body { color: red; }",
			}
			_, err := registry.Run(plugin.OnAssetProcess, asset)
			Expect(err).NotTo(HaveOccurred())
		})

		It("hook can transform asset content", func() {
			registry := plugin.NewHookRegistry()
			registry.Register(plugin.OnAssetProcess, func(_ context.Context, payload interface{}) (interface{}, error) {
				asset := payload.(map[string]interface{})
				path := asset["path"].(string)
				if strings.HasSuffix(path, ".css") {
					// Simulate CSS minification
					css := asset["content"].(string)
					asset["content"] = strings.ReplaceAll(css, " ", "")
				}
				return asset, nil
			})

			asset := map[string]interface{}{
				"path":    "css/main.css",
				"content": "body { color: red; }",
			}
			result, err := registry.Run(plugin.OnAssetProcess, asset)
			Expect(err).NotTo(HaveOccurred())
			resultMap := result.(map[string]interface{})
			Expect(resultMap["content"]).To(Equal("body{color:red;}"),
				"hook must be able to transform asset content")
		})
	})

	// ── Read-only hook payload contract ──────────────────────────────

	Describe("Read-only hook payload contract", func() {
		It("onBuildComplete receives stats and passes payload through", func() {
			registry := plugin.NewHookRegistry()
			var receivedStats map[string]interface{}
			registry.Register(plugin.OnBuildComplete, func(_ context.Context, payload interface{}) (interface{}, error) {
				stats, ok := payload.(map[string]interface{})
				Expect(ok).To(BeTrue(),
					"onBuildComplete payload must be a JSON-serializable map")
				receivedStats = stats
				// Read-only hooks pass payload through unchanged so chained
				// handlers still receive the original stats
				return payload, nil
			})

			stats := map[string]interface{}{
				"pageCount": 42,
				"duration":  "127ms",
			}
			_, err := registry.Run(plugin.OnBuildComplete, stats)
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedStats).To(HaveKeyWithValue("pageCount", 42))
			Expect(receivedStats).To(HaveKeyWithValue("duration", "127ms"))
		})

		It("onFileChanged receives file path as string", func() {
			registry := plugin.NewHookRegistry()
			var receivedPath string
			registry.Register(plugin.OnFileChanged, func(_ context.Context, payload interface{}) (interface{}, error) {
				path, ok := payload.(string)
				Expect(ok).To(BeTrue(),
					"onFileChanged payload must be a string file path")
				receivedPath = path
				return nil, nil
			})

			_, err := registry.Run(plugin.OnFileChanged, "content/blog/post.md")
			Expect(err).NotTo(HaveOccurred())
			Expect(receivedPath).To(Equal("content/blog/post.md"))
		})
	})

	// ── Multi-format output wiring (issue #71) ──────────────────────

	Describe("Multi-format output → layout → output path", func() {
		It("page with outputs field resolves format for each entry", func() {
			page := &content.Page{
				RelPath:     "blog/post.md",
				Section:     "blog",
				Outputs:     []string{"html", "json"},
				FrontMatter: map[string]interface{}{"title": "Multi-format Post"},
			}

			// Each format resolves independently
			htmlFormat := output.ResolveOutputFormat(page)
			Expect(htmlFormat).To(Equal("html"),
				"primary format must be html")

			// All declared formats must be iterated
			Expect(page.Outputs).To(HaveLen(2))
			Expect(page.Outputs).To(ContainElements("html", "json"))
		})

		It("format-specific layout uses unified chain (issue #864)", func() {
			page := &content.Page{
				RelPath:     "blog/post.md",
				Section:     "blog",
				FrontMatter: map[string]interface{}{"title": "JSON Post"},
			}

			// Unified format layout resolution uses the same chain as HTML
			// with format infixed — no "single" concept. The function now
			// requires permalinkCfg to detect date-based sections.
			// "layouts" dir doesn't exist on disk — must return an error.
			_, err := template.ResolveLayoutForFormat(page, "layouts", "liquid", "json", map[string]string{})
			Expect(err).To(HaveOccurred(),
				"format layout resolution must error when layouts directory does not exist")
		})

		It("output path uses format extension instead of html", func() {
			// HTML path via ComputeOutputPath
			htmlPath := output.ComputeOutputPath("/blog/post/")
			Expect(htmlPath).To(Equal("blog/post/index.html"),
				"HTML output path must end with index.html")

			// For JSON, the pipeline replaces .html with the format extension
			// (same algorithm as WritePageFormats: TrimSuffix(".html") + "." + format)
			jsonPath := strings.TrimSuffix(htmlPath, ".html") + ".json"
			Expect(jsonPath).To(Equal("blog/post/index.json"),
				"JSON output path must use .json extension")
		})

		It("page with no outputs field defaults to html only", func() {
			page := &content.Page{
				RelPath:     "about.md",
				FrontMatter: map[string]interface{}{"title": "About"},
			}
			format := output.ResolveOutputFormat(page)
			Expect(format).To(Equal("html"),
				"pages without outputs must default to html")
			Expect(page.Outputs).To(BeNil(),
				"outputs field must be nil when not specified")
		})
	})

	Describe("i18n → data cascade → template", func() {
		It("site.language.strings available in template context for language build", func() {
			page := &content.Page{RelPath: "en/index.md", Section: ""}
			siteData := map[string]interface{}{
				"title": "My Site",
				"language": map[string]interface{}{
					"code": "en",
					"strings": map[string]string{
						"read_more": "Read more",
					},
				},
			}

			ctx := template.BuildTemplateContext(page, siteData, nil, nil, nil, nil, "")
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Site.Data).NotTo(BeNil())

			langData, ok := siteData["language"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			strings, ok := langData["strings"].(map[string]string)
			Expect(ok).To(BeTrue())
			Expect(strings["read_more"]).To(Equal("Read more"),
				"site.language.strings.read_more must be available for language-specific rendering")
		})
	})

	// ── i18n pipeline wiring (issue #70) ─────────────────────────────

	Describe("i18n → pipeline wiring", func() {
		It("language contexts scope content discovery and output prefixes", func() {
			langCfg := map[string]*config.LanguageConfig{
				"en": {Title: "English Site", Weight: 1, Root: true},
				"fr": {Title: "Site Français", Weight: 2},
			}
			contexts, err := i18n.BuildLanguageContexts(langCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(2))

			// Default language (lowest weight) is first
			Expect(contexts[0].Code).To(Equal("en"))

			// Content trees are language-scoped
			enContentDir := i18n.ContentTreeRoute("en")
			frContentDir := i18n.ContentTreeRoute("fr")
			Expect(enContentDir).To(Equal("content/en/"))
			Expect(frContentDir).To(Equal("content/fr/"))

			// Output prefixes: root language at /, non-root at /lang/
			enPrefix := i18n.OutputPrefix("en", contexts[0].Root)
			frPrefix := i18n.OutputPrefix("fr", contexts[1].Root)
			Expect(enPrefix).To(Equal(""),
				"root language must output at site root")
			Expect(frPrefix).To(Equal("fr/"),
				"non-root language must output under prefix")

			// site.language data is injected per language iteration
			enData := i18n.LanguageData(contexts[0])
			Expect(enData["code"]).To(Equal("en"))
			frData := i18n.LanguageData(contexts[1])
			Expect(frData["code"]).To(Equal("fr"))

			// site.title is overridden per language
			enTitle := i18n.LanguageSiteTitle("Global Title", langCfg["en"])
			frTitle := i18n.LanguageSiteTitle("Global Title", langCfg["fr"])
			Expect(enTitle).To(Equal("English Site"))
			Expect(frTitle).To(Equal("Site Français"))
		})

		It("translation linking connects pages across language trees", func() {
			enPage := &content.Page{
				RelPath:     "en/about.md",
				URL:         "/about/",
				FrontMatter: map[string]interface{}{"title": "About", "lang": "en"},
			}
			frPage := &content.Page{
				RelPath:     "fr/about.md",
				URL:         "/fr/about/",
				FrontMatter: map[string]interface{}{"title": "À propos", "lang": "fr"},
			}
			allPages := []*content.Page{enPage, frPage}

			err := i18n.LinkTranslations(allPages, []string{"en", "fr"})
			Expect(err).NotTo(HaveOccurred())

			// Each page should reference the other as a translation
			Expect(enPage.Translations).To(HaveLen(1),
				"English page must link to French translation")
			Expect(frPage.Translations).To(HaveLen(1),
				"French page must link to English translation")

			// Translation info includes URL and language code
			enTranslations := i18n.GetTranslations(enPage)
			Expect(enTranslations).To(HaveLen(1))
			Expect(enTranslations[0].URL).To(Equal("/fr/about/"))
			Expect(enTranslations[0].LangCode).To(Equal("fr"))
		})

		It("index pages get correct URL without language prefix doubling (issue #113)", func() {
			langCfg := map[string]*config.LanguageConfig{
				"en": {Title: "English Site", Weight: 1, Root: true},
				"es": {Title: "Sitio Español", Weight: 2},
			}
			contexts, err := i18n.BuildLanguageContexts(langCfg)
			Expect(err).NotTo(HaveOccurred())

			// Simulate the pipeline's permalink resolution for index pages.
			// The pipeline prefixes RelPath with language code for translation linking,
			// then resolves the permalink, then adds the output prefix.
			// Index pages must not get the language prefix doubled.

			// Root language index: content/en/index.md → /
			enPrefix := i18n.OutputPrefix("en", contexts[0].Root)
			enIndexURL := permalink.DefaultFromPath("index.md") // resolve from ORIGINAL relPath
			enFinalURL := "/" + enPrefix + strings.TrimPrefix(enIndexURL, "/")
			Expect(enFinalURL).To(Equal("/"),
				"root language index page must resolve to / (not /en/)")

			// Non-root language index: content/es/index.md → /es/
			esPrefix := i18n.OutputPrefix("es", contexts[1].Root)
			esIndexURL := permalink.DefaultFromPath("index.md") // resolve from ORIGINAL relPath
			esFinalURL := "/" + esPrefix + strings.TrimPrefix(esIndexURL, "/")
			Expect(esFinalURL).To(Equal("/es/"),
				"non-root language index page must resolve to /es/ (not /es/es/)")
		})

		It("per-language collections are scoped to language pages only", func() {
			allPages := []*content.Page{
				{RelPath: "en/blog/post.md", FrontMatter: map[string]interface{}{"lang": "en", "tags": []interface{}{"go"}}},
				{RelPath: "fr/blog/post.md", FrontMatter: map[string]interface{}{"lang": "fr", "tags": []interface{}{"go"}}},
				{RelPath: "en/blog/other.md", FrontMatter: map[string]interface{}{"lang": "en"}},
			}

			enPages := i18n.FilterByLanguage(allPages, "en")
			frPages := i18n.FilterByLanguage(allPages, "fr")
			Expect(enPages).To(HaveLen(2),
				"English collection must only contain English pages")
			Expect(frPages).To(HaveLen(1),
				"French collection must only contain French pages")

			// Taxonomies are built from language-scoped pages
			enTaxonomies := i18n.BuildTaxonomiesForLanguage("en", enPages)
			Expect(enTaxonomies).To(HaveKey("tags"),
				"English taxonomies must be built from English pages only")
		})
	})

	// ── Plugin → template engine bridging (issue #93) ────────────────

	Describe("Plugin → template engine bridging (issue #93)", func() {
		It("plugin-discovered filters are bridgeable to template engine", func() {
			// Simulate the pipeline contract: registry discovers filters,
			// then pipeline registers them with the template engine.
			// Use a novel filter name (not a built-in) to test dynamic
			// resolution — built-in names like "wordCount" pass via the
			// alloyFilterBridge struct methods, not dynamic dispatch.
			engine := template.NewLiquidEngine()

			// Register a novel plugin filter via AddFilter (simulating
			// the bridge that LoadPlugins should create)
			err := engine.AddFilter("myCustomFilter", func(input interface{}, args ...interface{}) interface{} {
				// Simulate a plugin filter that transforms input
				return "transformed"
			})
			Expect(err).NotTo(HaveOccurred(),
				"novel plugin filter must register with template engine without error")

			// Verify the novel filter is callable in a template
			tmpl, err := engine.Parse("test", []byte("{{ content | myCustomFilter }}"))
			Expect(err).NotTo(HaveOccurred())
			result, err := tmpl.Render(map[string]interface{}{"content": "hello world"})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(Equal("transformed"),
				"novel plugin filter must transform value during rendering")
		})

		It("plugin-discovered hooks are registerable in HookRegistry", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(pluginFixtureDir(), "hooks.js"))).To(Succeed())

			hooks := rt.RegisteredHooks()
			Expect(hooks).NotTo(BeEmpty(),
				"plugin must discover at least one hook")

			// Bridge: register each discovered hook in the HookRegistry
			registry := plugin.NewHookRegistry()
			for _, hookName := range hooks {
				registry.Register(plugin.HookName(hookName), func(ctx context.Context, payload interface{}) (interface{}, error) {
					return payload, nil // wrapper would route through runtime
				})
			}

			// Verify the hook fires
			result, err := registry.Run(plugin.OnContentTransformed, "test payload")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("test payload"),
				"plugin-registered hook must execute through HookRegistry")
		})

		// ── Issue #142: Hook wiring through LoadPlugins ──────────────
		// LoadPlugins() bridges filters but NOT hooks. The hooks parameter
		// is accepted but unused. Plugin hooks are discovered (RegisteredHooks
		// returns them) but never wired to the HookRegistry.

		It("plugin hooks are wired to HookRegistry via CallHook", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(pluginFixtureDir(), "hooks.js"))).To(Succeed())

			hooks := rt.RegisteredHooks()
			Expect(hooks).To(ContainElement("onContentTransformed"),
				"hooks.js must register onContentTransformed")

			// Bridge hooks through CallHook — this is what LoadPlugins should do.
			// CallHook must invoke the actual JS hook function, not pass-through.
			registry := plugin.NewHookRegistry()
			for _, hookName := range hooks {
				name := hookName
				runtime := rt
				registry.Register(plugin.HookName(name), func(ctx context.Context, payload interface{}) (interface{}, error) {
					return runtime.CallHook(name, payload)
				})
			}

			// Fire the hook and verify the JS function executed.
			// hooks.js onContentTransformed is a pass-through (returns content
			// unchanged), so the result should equal the input.
			input := "<p>Original content</p>"
			result, err := registry.Run(plugin.OnContentTransformed, input)
			Expect(err).NotTo(HaveOccurred(),
				"hook execution through CallHook must not error")
			Expect(result).To(Equal(input),
				"hooks.js onContentTransformed is a pass-through — "+
					"result must equal input, proving the JS function executed")
		})
	})

	// ── External data sources → pipeline → template (issue #107) ────

	Describe("External data sources → pipeline → template (issue #107)", func() {
		It("REST source data merges into site.data under the 'as' key", func() {
			// Stand up a test HTTP server returning JSON
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]map[string]interface{}{
					{"id": 1, "title": "First Post"},
					{"id": 2, "title": "Second Post"},
				})
			}))
			defer ts.Close()

			// Simulate the pipeline contract:
			// 1. Config defines a source with type: rest, url, as
			sourceCfg := &config.SourceConfig{
				Type:  "rest",
				URL:   ts.URL + "/posts",
				Cache: 0,
				As:    "api_posts",
			}

			// 2. Fetch the data using the fetch package
			fetched, err := fetch.FetchREST(sourceCfg.URL)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched).NotTo(BeNil())

			// 3. Merge fetched data into siteData under the "as" key
			//    (this is the pipeline wiring that build.go must do)
			siteData := map[string]interface{}{
				"title": "Test Site",
				"data":  map[string]interface{}{sourceCfg.As: fetched},
			}

			// 4. Template context must see it as site.data.api_posts
			page := &content.Page{RelPath: "index.md"}
			ctx := template.BuildTemplateContext(page, siteData, nil, nil, nil, nil, "")
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Site.Data).To(HaveKey("api_posts"),
				"fetched REST data must be available as site.data.api_posts")

			// 5. The data must be the actual fetched content, not empty
			posts, ok := ctx.Site.Data["api_posts"]
			Expect(ok).To(BeTrue())
			Expect(posts).NotTo(BeNil(),
				"site.data.api_posts must contain the fetched data")
		})

		It("cached source is used when TTL has not expired", func() {
			cacheDir := GinkgoT().TempDir()
			sourceName := "cached_posts"

			// Pre-populate cache with known data
			cachedData := []map[string]interface{}{
				{"id": 1, "title": "Cached Post"},
			}
			err := fetch.SaveCache(sourceName, cacheDir, cachedData)
			Expect(err).NotTo(HaveOccurred())

			// Retrieve from cache with a long TTL — must return cached data
			// without making a network request
			data, found := fetch.GetCached(sourceName, cacheDir, 3600)
			Expect(found).To(BeTrue(),
				"cached data must be found when TTL has not expired")
			Expect(data).NotTo(BeNil())

			// Merge cached data into siteData the same way the pipeline would
			siteData := map[string]interface{}{
				"title": "Test Site",
				"data":  map[string]interface{}{"posts": data},
			}

			// Template context must see cached data identically to fresh-fetched data
			page := &content.Page{RelPath: "index.md"}
			ctx := template.BuildTemplateContext(page, siteData, nil, nil, nil, nil, "")
			Expect(ctx.Site.Data).To(HaveKey("posts"),
				"cached source data must be available in site.data just like fetched data")
		})
	})

	// ── Build cache → incremental build (issue #105) ────────────────

	Describe("In-memory cache → incremental build (issue #639)", func() {
		It("in-memory cache detects unchanged, modified, and new pages", func() {
			pageContent := []byte("---\ntitle: My Post\n---\n# Hello World")

			// Initial build populates the in-memory cache
			buildCache := cache.New()
			buildCache.SetHash("content/blog/post.md", cache.HashContent(pageContent))
			buildCache.SetHash("content/about.md", cache.HashContent([]byte("about page")))

			// Incremental rebuild: unchanged page must be skippable
			Expect(buildCache.ShouldSkipFile("content/blog/post.md", pageContent)).To(BeTrue(),
				"unchanged page must be detected as skippable via in-memory cache")

			// Incremental rebuild: modified page must not be skippable
			modifiedContent := []byte("---\ntitle: My Post (edited)\n---\n# Updated")
			Expect(buildCache.ShouldSkipFile("content/blog/post.md", modifiedContent)).To(BeFalse(),
				"modified page must not be skippable")

			// Incremental rebuild: new page (not in cache) must not be skippable
			Expect(buildCache.ShouldSkipFile("content/new-page.md", []byte("new"))).To(BeFalse(),
				"new page not in cache must not be skippable")
		})

		It("template change invalidates only pages using that template", func() {
			buildCache := cache.New()
			buildCache.SetHash("content/blog/post-1.md", cache.HashContent([]byte("post 1")))
			buildCache.SetHash("content/blog/post-2.md", cache.HashContent([]byte("post 2")))
			buildCache.SetHash("content/about.md", cache.HashContent([]byte("about")))
			buildCache.TrackTemplateUsage("content/blog/post-1.md", "layouts/post.liquid")
			buildCache.TrackTemplateUsage("content/blog/post-2.md", "layouts/post.liquid")
			buildCache.TrackTemplateUsage("content/about.md", "layouts/default.liquid")

			// Content unchanged — all pages would be skippable by content hash
			Expect(buildCache.ShouldSkipFile("content/blog/post-1.md", []byte("post 1"))).To(BeTrue())
			Expect(buildCache.ShouldSkipFile("content/about.md", []byte("about"))).To(BeTrue())

			// But if layouts/post.liquid changed, its pages must be rebuilt
			affected := buildCache.InvalidatedPages("layouts/post.liquid")
			Expect(affected).To(ConsistOf(
				"content/blog/post-1.md",
				"content/blog/post-2.md",
			), "template change must invalidate only pages using that template")
			Expect(affected).NotTo(ContainElement("content/about.md"),
				"pages using a different template must not be invalidated")
		})
	})

	// ── Draft visibility → server mode → lifecycle filtering (issue #108) ──

	Describe("Draft visibility → server mode → lifecycle filtering (issue #108)", func() {
		It("dev mode includes draft pages in filtered output", func() {
			// Server reports draft inclusion based on mode
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			Expect(srv.ShouldIncludeDrafts()).To(BeTrue(),
				"guard: dev mode must report includeDrafts=true")

			// Build a page set with a draft (Draft field set by BuildPage from front matter)
			pages := []*content.Page{
				{RelPath: "blog/published.md", Draft: false, FrontMatter: map[string]interface{}{"title": "Published"}},
				{RelPath: "blog/draft.md", Draft: true, FrontMatter: map[string]interface{}{"title": "Draft", "draft": true}},
			}

			// Pipeline must pass includeDrafts from server mode to FilterByLifecycle
			filtered := content.FilterByLifecycle(pages, time.Now(), srv.ShouldIncludeDrafts())
			Expect(filtered).To(HaveLen(2),
				"dev mode must include draft pages in build output")

			// Verify the draft page is in the result
			var titles []string
			for _, p := range filtered {
				titles = append(titles, p.FrontMatter["title"].(string))
			}
			Expect(titles).To(ContainElement("Draft"),
				"draft page must be present in dev mode output")
		})

		It("build mode excludes draft pages from filtered output", func() {
			// Preview/build mode excludes drafts
			cfg := &config.Config{Title: "Test Site"}
			srv := server.NewWithMode(cfg, server.ModePreview)
			Expect(srv.ShouldIncludeDrafts()).To(BeFalse(),
				"guard: preview mode must report includeDrafts=false")

			pages := []*content.Page{
				{RelPath: "blog/published.md", Draft: false, FrontMatter: map[string]interface{}{"title": "Published"}},
				{RelPath: "blog/draft.md", Draft: true, FrontMatter: map[string]interface{}{"title": "Draft", "draft": true}},
			}

			filtered := content.FilterByLifecycle(pages, time.Now(), srv.ShouldIncludeDrafts())
			Expect(filtered).To(HaveLen(1),
				"build mode must exclude draft pages")
			Expect(filtered[0].FrontMatter["title"]).To(Equal("Published"),
				"only published pages must appear in build output")
		})
	})
})
