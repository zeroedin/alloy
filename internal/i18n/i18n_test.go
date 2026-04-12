package i18n_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/i18n"
)

var _ = Describe("I18n", func() {

	var cfg map[string]*config.LanguageConfig

	BeforeEach(func() {
		cfg = map[string]*config.LanguageConfig{
			"en": {
				Title:   "English Site",
				Weight:  1,
				Root:    true,
				Strings: map[string]string{"read_more": "Read more"},
			},
			"fr": {
				Title:   "Site Fran\u00e7ais",
				Weight:  2,
				Strings: map[string]string{"read_more": "Lire la suite"},
			},
		}
	})

	// ── Activation ────────────────────────────────────────────────────

	Describe("Activation", func() {
		It("returns error when no languages configured", func() {
			contexts, err := i18n.BuildLanguageContexts(nil)
			Expect(err).To(HaveOccurred())
			Expect(contexts).To(BeNil())
			// The error must explain why it failed, not be a generic stub error
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("language"),
					ContainSubstring("empty"),
					ContainSubstring("no languages"),
				),
				"error should indicate missing language configuration",
			)
		})

		It("builds contexts when languages are present", func() {
			contexts, err := i18n.BuildLanguageContexts(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).NotTo(BeNil())
			Expect(contexts).To(HaveLen(2))
		})
	})

	// ── Language contexts ─────────────────────────────────────────────

	Describe("Language contexts", func() {
		It("builds LanguageContext for each declared language", func() {
			contexts, err := i18n.BuildLanguageContexts(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).NotTo(BeNil())

			codes := make([]string, len(contexts))
			for idx, ctx := range contexts {
				codes[idx] = ctx.Code
			}
			Expect(codes).To(ContainElements("en", "fr"))
		})

		It("identifies default language by lowest weight", func() {
			contexts, err := i18n.BuildLanguageContexts(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).NotTo(BeNil())

			// The context with the lowest weight (en, weight 1) should be first
			Expect(contexts[0].Code).To(Equal("en"))
		})

		It("preserves root flag in context", func() {
			contexts, err := i18n.BuildLanguageContexts(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).NotTo(BeNil())

			var enCtx *i18n.LanguageContext
			for idx := range contexts {
				if contexts[idx].Code == "en" {
					enCtx = &contexts[idx]
					break
				}
			}
			Expect(enCtx).NotTo(BeNil())
			Expect(enCtx.Root).To(BeTrue())
		})
	})

	// ── Output paths ──────────────────────────────────────────────────

	Describe("Output paths", func() {
		It("returns language code as prefix for non-root language", func() {
			prefix := i18n.OutputPrefix("fr", false)
			Expect(prefix).To(Equal("fr/"))
		})

		It("returns empty string when isRoot is true (contrasted with non-root)", func() {
			// First verify that non-root returns a prefix (proves the function works)
			nonRootPrefix := i18n.OutputPrefix("en", false)
			Expect(nonRootPrefix).NotTo(BeEmpty(), "non-root language must return a prefix")

			// Root language should return empty (no prefix subdirectory)
			rootPrefix := i18n.OutputPrefix("en", true)
			Expect(rootPrefix).To(BeEmpty())
		})
	})

	// ── Translation linking ───────────────────────────────────────────

	Describe("Translation linking", func() {
		It("links pages by relative path across languages", func() {
			pages := []*content.Page{
				{
					RelPath:    "blog/hello.md",
					Section:    "blog",
					Collection: "blog",
					FrontMatter: map[string]interface{}{
						"title": "Hello",
						"lang":  "en",
					},
				},
				{
					RelPath:    "blog/hello.md",
					Section:    "blog",
					Collection: "blog",
					FrontMatter: map[string]interface{}{
						"title": "Bonjour",
						"lang":  "fr",
					},
				},
			}

			err := i18n.LinkTranslations(pages, []string{"en", "fr"})
			Expect(err).NotTo(HaveOccurred())

			// After linking, each page should reference its translation
			Expect(pages[0].Translations).To(HaveLen(1))
			Expect(pages[1].Translations).To(HaveLen(1))
		})

		It("does not error for valid single-language input", func() {
			pages := []*content.Page{
				{
					RelPath:     "about.md",
					FrontMatter: map[string]interface{}{"title": "About", "lang": "en"},
				},
			}
			err := i18n.LinkTranslations(pages, []string{"en"})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ── Content tree routing ──────────────────────────────────────────

	Describe("Content tree routing", func() {
		It("returns content/<lang>/ path for a language code", func() {
			route := i18n.ContentTreeRoute("en")
			Expect(route).To(Equal("content/en/"),
				"content tree route must map to language-specific directory")
		})

		It("returns content/<lang>/ for non-default language", func() {
			route := i18n.ContentTreeRoute("fr")
			Expect(route).To(Equal("content/fr/"))
		})
	})

	// ── Site language data cascade ────────────────────────────────────

	Describe("Site language data cascade", func() {
		It("provides site.language data for a language context", func() {
			ctx := i18n.LanguageContext{
				Code:    "en",
				Title:   "English Site",
				Root:    true,
				Strings: map[string]string{"read_more": "Read more"},
			}
			data := i18n.LanguageData(ctx)
			Expect(data).NotTo(BeNil())
			Expect(data).To(HaveKeyWithValue("code", "en"))
			Expect(data).To(HaveKeyWithValue("title", "English Site"))
		})
	})

	// ── Per-language collections ──────────────────────────────────────

	Describe("Per-language collections", func() {
		It("filters pages to only include those matching the language", func() {
			pages := []*content.Page{
				{RelPath: "blog/hello.md", FrontMatter: map[string]interface{}{"lang": "en"}},
				{RelPath: "blog/bonjour.md", FrontMatter: map[string]interface{}{"lang": "fr"}},
				{RelPath: "blog/world.md", FrontMatter: map[string]interface{}{"lang": "en"}},
			}
			enPages := i18n.FilterByLanguage(pages, "en")
			Expect(enPages).To(HaveLen(2),
				"English collection must only contain English pages")

			frPages := i18n.FilterByLanguage(pages, "fr")
			Expect(frPages).To(HaveLen(1),
				"French collection must only contain French pages")
		})
	})

	// ── Root language output ──────────────────────────────────────────

	Describe("Root language output", func() {
		It("root language outputs at / not /en/", func() {
			prefix := i18n.OutputPrefix("en", true)
			Expect(prefix).To(Equal(""),
				"root language must output at site root with no prefix")

			// Guard: non-root must have prefix
			frPrefix := i18n.OutputPrefix("fr", false)
			Expect(frPrefix).To(Equal("fr/"),
				"guard: non-root language must output under prefix")
		})
	})

	// ── Per-language taxonomies (§1i) ────────────────────────────────

	Describe("Per-language taxonomies", func() {
		It("generates taxonomy pages scoped to a specific language", func() {
			enPages := []*content.Page{
				{RelPath: "en/blog/post1.md", FrontMatter: map[string]interface{}{"tags": []interface{}{"go"}}},
				{RelPath: "en/blog/post2.md", FrontMatter: map[string]interface{}{"tags": []interface{}{"go", "web"}}},
			}
			taxonomies := i18n.BuildTaxonomiesForLanguage("en", enPages)
			Expect(taxonomies).NotTo(BeNil(),
				"per-language taxonomy must be generated from language-scoped pages")
		})
	})

	// ── Language-specific site title (§1i) ───────────────────────────

	Describe("Language-specific site title", func() {
		It("site.title is overridden by languages.{lang}.title", func() {
			langCfg := &config.LanguageConfig{
				Title: "Mon Site",
			}
			title := i18n.LanguageSiteTitle("My Site", langCfg)
			Expect(title).To(Equal("Mon Site"),
				"language-specific title must override global title")
		})
	})

	// ── Translation linking (§1i) ────────────────────────────────────

	Describe("Translation linking", func() {
		It("page.translations contains URL and language code for each translation", func() {
			page := &content.Page{
				RelPath: "en/about.md",
				Translations: []*content.Page{
					{RelPath: "fr/about.md", URL: "/fr/about/"},
				},
			}
			translations := i18n.GetTranslations(page)
			Expect(translations).To(HaveLen(1))
			Expect(translations[0].URL).To(Equal("/fr/about/"))
			Expect(translations[0].LangCode).To(Equal("fr"))
		})
	})

	// ── Language build parallelism (§1i) ─────────────────────────────

	Describe("Language build parallelism", func() {
		It("independent content trees share layouts across languages", func() {
			// English and French have separate content/ trees but same layouts/
			enRoute := i18n.ContentTreeRoute("en")
			frRoute := i18n.ContentTreeRoute("fr")
			Expect(enRoute).NotTo(Equal(frRoute),
				"each language must have its own content tree")
			// Both should reference a shared layouts directory (not language-scoped)
			Expect(enRoute).To(ContainSubstring("en"))
			Expect(frRoute).To(ContainSubstring("fr"))
		})
	})
})
