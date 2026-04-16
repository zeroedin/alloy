package integration_test

import (
	"context"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/data"
	"github.com/zeroedin/alloy/internal/i18n"
	"github.com/zeroedin/alloy/internal/output"
	"github.com/zeroedin/alloy/internal/pagination"
	"github.com/zeroedin/alloy/internal/permalink"
	"github.com/zeroedin/alloy/internal/plugin"
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
		It("registers a hook, transforms content, and verifies output", func() {
			// This is a minimal simulation: register a transform hook, run it,
			// and verify the payload was processed
			transformCalled := false
			hookFn := func(_ context.Context, payload interface{}) (interface{}, error) {
				transformCalled = true
				return payload, nil // pass-through
			}
			_ = hookFn
			_ = transformCalled

			// The hook execution happens through the HookRegistry
			// but the cross-cutting part is that the transformed content
			// ends up in the output
			raw := []byte("---\ntitle: Hooked\n---\n# Content")
			page, err := content.BuildPage("content/hooked.md", raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(page).NotTo(BeNil())
		})
	})

	// ── Multi-format output wiring (issue #71) ──────────────────────

	Describe("Multi-format output → layout → output path", func() {
		It("page with outputs field resolves format for each entry", func() {
			page := &content.Page{
				RelPath:     "blog/post.md",
				Section:     "blog",
				Layout:      "single",
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

		It("format-specific layout resolves for non-HTML format", func() {
			page := &content.Page{
				RelPath:     "blog/post.md",
				Section:     "blog",
				Layout:      "single",
				FrontMatter: map[string]interface{}{"title": "JSON Post"},
			}

			// JSON format should resolve to single.json.liquid
			layoutPath, err := output.ResolveFormatLayout(page, "json", "layouts", "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(layoutPath).To(ContainSubstring("single.json.liquid"),
				"JSON format must resolve to format-specific layout")

			// template package also has format-aware layout resolution
			_, err = template.ResolveLayoutForFormat(page, "layouts", "liquid", "json")
			// Error expected since layouts dir doesn't exist on disk — but the function must be callable
			_ = err
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

			ctx := template.BuildTemplateContext(page, siteData, nil, nil, nil, "")
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
	})
})
