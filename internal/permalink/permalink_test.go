package permalink_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/permalink"
)

var _ = Describe("Permalink", func() {

	// ── Token replacement ──────────────────────────────────────────────

	Describe("Token replacement", func() {
		var page *content.Page

		BeforeEach(func() {
			page = &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "My First Post"},
				Section:     "blog",
				RelPath:     "blog/my-first-post.md",
			}
		})

		It("replaces :year with 4-digit year from page date", func() {
			result := permalink.ResolveTokens("/:year/", page)
			Expect(result).To(Equal("/2026/"))
		})

		It("replaces :month with 2-digit month", func() {
			result := permalink.ResolveTokens("/:month/", page)
			Expect(result).To(Equal("/04/"))
		})

		It("replaces :day with 2-digit day", func() {
			result := permalink.ResolveTokens("/:day/", page)
			Expect(result).To(Equal("/10/"))
		})

		It("replaces :slug with slugified title", func() {
			result := permalink.ResolveTokens("/:slug/", page)
			Expect(result).To(Equal("/my-first-post/"))
		})

		It("replaces :slug with filename when no title", func() {
			page.FrontMatter = map[string]interface{}{}
			result := permalink.ResolveTokens("/:slug/", page)
			Expect(result).To(Equal("/my-first-post/"))
		})

		It("replaces :section with top-level directory", func() {
			result := permalink.ResolveTokens("/:section/", page)
			Expect(result).To(Equal("/blog/"))
		})

		It("replaces :filename with source filename without extension", func() {
			result := permalink.ResolveTokens("/:filename/", page)
			Expect(result).To(Equal("/my-first-post/"))
		})

		It("handles multiple tokens in one pattern (/:year/:month/:slug/)", func() {
			result := permalink.ResolveTokens("/:year/:month/:slug/", page)
			Expect(result).To(Equal("/2026/04/my-first-post/"))
		})
	})

	// ── Front matter overrides ─────────────────────────────────────────

	Describe("Front matter overrides", func() {
		It("uses static front matter permalink directly", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "My First Post", "permalink": "/custom/path/"},
				Section:     "blog",
				RelPath:     "blog/my-first-post.md",
			}
			result, err := permalink.Resolve("/:year/:slug/", page)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/custom/path/"))
		})

		It("front matter slug overrides :slug token", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "My First Post", "slug": "custom-slug"},
				Section:     "blog",
				RelPath:     "blog/my-first-post.md",
			}
			result, err := permalink.Resolve("/:year/:slug/", page)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/2026/custom-slug/"))
		})
	})

	// ── Liquid fallback ────────────────────────────────────────────────

	Describe("Liquid fallback", func() {
		It("ContainsLiquidTags detects {{ }} in permalink string", func() {
			result := permalink.ContainsLiquidTags("/{{ page.slug }}/{{ page.lang }}/")
			Expect(result).To(BeTrue())
		})

		It("ContainsLiquidTags returns false for token-only strings like /:year/:slug/", func() {
			// First verify positive case works (Liquid tags ARE detected)
			Expect(permalink.ContainsLiquidTags("/{{ page.slug }}/")).To(BeTrue(),
				"should detect {{ }} as Liquid tags")

			// Then verify token-only strings are NOT detected as Liquid
			result := permalink.ContainsLiquidTags("/:year/:slug/")
			Expect(result).To(BeFalse())
		})
	})

	// ── permalink: false ───────────────────────────────────────────────

	Describe("permalink: false", func() {
		It("returns empty string for permalink: false (signal no output)", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "Hidden Page", "permalink": false},
				Section:     "blog",
				RelPath:     "blog/hidden.md",
			}
			result, err := permalink.Resolve("/:year/:slug/", page)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeEmpty())
		})
	})

	// ── DefaultFromPath ────────────────────────────────────────────────

	Describe("DefaultFromPath", func() {
		It("computes URL from relative path (blog/my-post.md -> /blog/my-post/)", func() {
			result := permalink.DefaultFromPath("blog/my-post.md")
			Expect(result).To(Equal("/blog/my-post/"))
		})
	})

	// ── :title token ──────────────────────────────────────────────────

	Describe(":title token", func() {
		It("replaces :title with raw title from front matter (not slugified)", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "My First Post"},
				Section:     "blog",
				RelPath:     "blog/my-first-post.md",
			}
			result := permalink.ResolveTokens("/:title/", page)
			// :title uses raw title, NOT slugified — contrast with :slug
			Expect(result).To(Equal("/My First Post/"))
		})
	})

	// ── Config-level section-to-pattern lookup ────────────────────────

	Describe("Section-to-pattern lookup", func() {
		It("applies the section-specific permalink pattern from config", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "My First Post"},
				Section:     "blog",
				RelPath:     "blog/my-first-post.md",
			}
			permalinkCfg := map[string]string{
				"blog":    "/:year/:month/:slug/",
				"docs":    "/docs/:slug/",
				"default": "/:slug/",
			}
			result, err := permalink.ResolveForSection(page, permalinkCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/2026/04/my-first-post/"),
				"blog section must use blog-specific permalink pattern")
		})

		It("falls back to default pattern when no section match", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "About"},
				Section:     "pages",
				RelPath:     "pages/about.md",
			}
			permalinkCfg := map[string]string{
				"blog":    "/:year/:month/:slug/",
				"default": "/:slug/",
			}
			result, err := permalink.ResolveForSection(page, permalinkCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/about/"),
				"unmatched section must fall back to default pattern")
		})

		It("falls back to file path when no config at all", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "About"},
				Section:     "pages",
				RelPath:     "pages/about.md",
			}
			// Empty config — no section match, no default
			result, err := permalink.ResolveForSection(page, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/pages/about/"),
				"no config must fall back to file path default")
		})
	})

	// ── Permalink resolution fallback chain ───────────────────────────

	Describe("Fallback chain", func() {
		It("front matter permalink takes priority over section pattern", func() {
			page := &content.Page{
				Date: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{
					"title":     "My Post",
					"permalink": "/custom/path/",
				},
				Section: "blog",
				RelPath: "blog/my-post.md",
			}
			permalinkCfg := map[string]string{
				"blog": "/:year/:month/:slug/",
			}
			result, err := permalink.ResolveForSection(page, permalinkCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/custom/path/"),
				"front matter permalink must override config pattern")
		})
	})

	// ── No recognized tokens ─────────────────────────────────────────

	Describe("No recognized tokens", func() {
		It("returns pattern unchanged when it contains no recognized tokens", func() {
			page := &content.Page{
				Slug:    "my-post",
				Section: "blog",
			}
			result := permalink.ResolveTokens("/static/path/", page)
			Expect(result).To(Equal("/static/path/"))
		})
	})

	// ── Alias URL generation ──────────────────────────────────────────

	Describe("Alias URL generation", func() {
		It("returns all alias paths from front matter", func() {
			page := &content.Page{
				RelPath: "about.md",
				Section: "",
				Aliases: []string{"/about-us/", "/team/"},
				FrontMatter: map[string]interface{}{
					"title":     "About Us",
					"permalink": "/about/",
					"aliases":   []interface{}{"/about-us/", "/team/"},
				},
			}
			aliases, err := permalink.ResolveAliases(page)
			Expect(err).NotTo(HaveOccurred())
			Expect(aliases).To(ConsistOf("/about-us/", "/team/"),
				"must return all alias paths from front matter")
		})

		It("returns empty slice when no aliases defined", func() {
			page := &content.Page{
				RelPath:     "about.md",
				Section:     "",
				FrontMatter: map[string]interface{}{"title": "About"},
			}
			// Guard: page WITH aliases must return non-empty
			pageWithAliases := &content.Page{
				RelPath: "contact.md",
				Section: "",
				Aliases: []string{"/reach-us/"},
				FrontMatter: map[string]interface{}{
					"title":   "Contact",
					"aliases": []interface{}{"/reach-us/"},
				},
			}
			withResult, withErr := permalink.ResolveAliases(pageWithAliases)
			Expect(withErr).NotTo(HaveOccurred())
			Expect(withResult).NotTo(BeEmpty(),
				"guard: page with aliases must return them")

			// Actual: page without aliases
			aliases, err := permalink.ResolveAliases(page)
			Expect(err).NotTo(HaveOccurred())
			Expect(aliases).To(BeEmpty(),
				"page with no aliases must return empty slice")
		})
	})
})
