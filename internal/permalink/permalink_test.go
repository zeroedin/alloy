package permalink_test

import (
	"fmt"
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

		It("root index.md resolves to /", func() {
			result := permalink.DefaultFromPath("index.md")
			Expect(result).To(Equal("/"),
				"content/index.md is the site root — must resolve to /")
		})

		It("section index.md resolves to section path (blog/index.md -> /blog/)", func() {
			result := permalink.DefaultFromPath("blog/index.md")
			Expect(result).To(Equal("/blog/"),
				"content/blog/index.md is the section landing — must resolve to /blog/")
		})

		It("page bundle index.md resolves to parent path (blog/post/index.md -> /blog/post/)", func() {
			result := permalink.DefaultFromPath("blog/post/index.md")
			Expect(result).To(Equal("/blog/post/"),
				"page bundle index.md must resolve to its parent directory path")
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

		It("root index.md resolves to / even when default pattern exists", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "Home"},
				Section:     "",
				RelPath:     "index.md",
			}
			permalinkCfg := map[string]string{
				"blog":    "/:year/:month/:slug/",
				"default": "/:slug/",
			}
			result, err := permalink.ResolveForSection(page, permalinkCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/"),
				"index.md must resolve to / — default pattern must not produce /home/")
		})

		It("section index.md resolves to /blog/ even when blog pattern exists", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "Blog"},
				Section:     "blog",
				RelPath:     "blog/index.md",
			}
			permalinkCfg := map[string]string{
				"blog":    "/:year/:month/:slug/",
				"default": "/:slug/",
			}
			result, err := permalink.ResolveForSection(page, permalinkCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/blog/"),
				"blog/index.md is the section landing page — must not apply date pattern")
		})

		It("front matter permalink overrides index file default", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "Home", "permalink": "/docs/"},
				Section:     "",
				RelPath:     "index.md",
			}
			permalinkCfg := map[string]string{
				"default": "/:slug/",
			}
			result, err := permalink.ResolveForSection(page, permalinkCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/docs/"),
				"front matter permalink must override index file default — enables subdirectory deployments")
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

	// ── Cascade permalink resolution (issue #302) ──────────────────
	// Permalink patterns come from _data.yaml cascade, not site config.
	// ResolveFromCascade reads the "permalink" key from cascade data.

	Describe("Cascade permalink resolution", func() {
		It("resolves permalink from cascade data", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "My Post"},
				Section:     "blog",
				RelPath:     "blog/my-post.md",
			}
			cascadeData := map[string]interface{}{
				"permalink": "/:year/:month/:slug/",
			}
			result, err := permalink.ResolveFromCascade(page, cascadeData)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/2026/04/my-post/"),
				"permalink pattern from _data.yaml cascade must be applied via token replacement")
		})

		It("falls back to DefaultFromPath when no cascade permalink", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "About"},
				Section:     "docs",
				RelPath:     "docs/about.md",
			}
			// No "permalink" key in cascade data
			cascadeData := map[string]interface{}{
				"layout": "doc",
			}
			result, err := permalink.ResolveFromCascade(page, cascadeData)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/docs/about/"),
				"without cascade permalink, file path must map directly to URL")
		})

		It("front matter permalink overrides cascade permalink", func() {
			page := &content.Page{
				Date: time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{
					"title":     "My Post",
					"permalink": "/custom/path/",
				},
				Section: "blog",
				RelPath: "blog/my-post.md",
			}
			cascadeData := map[string]interface{}{
				"permalink": "/:year/:month/:slug/",
			}
			result, err := permalink.ResolveFromCascade(page, cascadeData)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/custom/path/"),
				"front matter permalink must override cascade permalink")
		})

		It("index files skip cascade permalink", func() {
			page := &content.Page{
				Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
				FrontMatter: map[string]interface{}{"title": "Blog"},
				Section:     "blog",
				RelPath:     "blog/index.md",
			}
			cascadeData := map[string]interface{}{
				"permalink": "/:year/:month/:slug/",
			}
			result, err := permalink.ResolveFromCascade(page, cascadeData)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("/blog/"),
				"index files must skip cascade permalink and use DefaultFromPath")
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

	// ── Template permalink rendering (issue #830) ────────────────────
	// Front matter permalinks containing {{ }} are rendered through a
	// PermalinkRenderer callback. Token syntax and template syntax are
	// separate modes — when {{ is detected, tokens are not resolved.

	Describe("Template permalink rendering (issue #830)", func() {

		// ── Resolve with renderer ─────────────────────────────────────

		Describe("Resolve with renderer", func() {
			It("renders template permalink through renderer when {{ is present", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				called := false
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					called = true
					Expect(source).To(Equal("/{{ page.slug }}/"),
						"renderer must receive the raw permalink template string")
					return "/my-post/", nil
				})
				result, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("/my-post/"),
					"Resolve must return the renderer's output when permalink contains {{")
				Expect(called).To(BeTrue(),
					"renderer must be invoked for template permalinks")
			})

			It("returns error when renderer returns error", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/"},
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					return "", fmt.Errorf("template parse error: unexpected EOF")
				})
				_, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).To(HaveOccurred(),
					"Resolve must propagate renderer errors")
				Expect(err.Error()).To(ContainSubstring("template parse error"),
					"error message must include the renderer's error")
			})

			It("returns error when renderer returns empty string", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.nonexistent }}/"},
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					return "", nil // rendered to empty — not an engine error, but a content error
				})
				_, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).To(HaveOccurred(),
					"template permalink rendering to empty string must be a fatal error — "+
						"distinct from permalink:false which returns (\"\", nil) with no error")
			})

			It("returns error when renderer returns whitespace-only string", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.nonexistent }}/"},
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					return "   \n  ", nil
				})
				_, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).To(HaveOccurred(),
					"whitespace-only render result must be treated as empty — fatal error")
			})

			It("does not invoke renderer when permalink has no {{ (fast path)", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/static/path/"},
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					Fail("renderer must not be invoked for static permalinks — " +
						"the {{ detection fast path should skip template rendering entirely")
					return "", nil
				})
				result, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("/static/path/"),
					"static permalink must be returned verbatim without invoking renderer")
			})

			It("does not resolve tokens when permalink contains {{ (separate modes)", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.section }}/:slug/"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					// Return the source as-is to prove tokens weren't resolved before rendering
					return source, nil
				})
				result, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring(":slug"),
					"token syntax must NOT be resolved when {{ is present — "+
						"template and token modes are mutually exclusive")
			})

			It("provides page context with front matter fields to renderer", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/", "lang": "en", "custom_field": "custom-value"},
					Slug:        "my-post",
					Section:     "blog",
					Summary:     "A summary",
					RelPath:     "blog/my-post.md",
				}
				var capturedCtx map[string]interface{}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					capturedCtx = ctx
					return "/my-post/", nil
				})
				_, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).NotTo(HaveOccurred())

				Expect(capturedCtx).To(HaveKey("page"),
					"renderer context must include a 'page' key")
				pageCtx, ok := capturedCtx["page"].(map[string]interface{})
				Expect(ok).To(BeTrue(), "page context must be a map")

				Expect(pageCtx).To(HaveKeyWithValue("title", "My Post"),
					"page context must include front matter title")
				Expect(pageCtx).To(HaveKey("date"),
					"page context must include page date")
				Expect(pageCtx).To(HaveKeyWithValue("slug", "my-post"),
					"page context must include slug")
				Expect(pageCtx).To(HaveKeyWithValue("lang", "en"),
					"page context must include custom front matter fields")
				Expect(pageCtx).To(HaveKeyWithValue("custom_field", "custom-value"),
					"page context must include arbitrary front matter fields")
				Expect(pageCtx).To(HaveKeyWithValue("summary", "A summary"),
					"page context must include summary")
				Expect(pageCtx).To(HaveKeyWithValue("collection", "blog"),
					"page context must include collection (section)")
			})

			It("does not include page.url in renderer context", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
					// URL is empty at permalink resolution time — it's what we're computing
				}
				var capturedCtx map[string]interface{}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					capturedCtx = ctx
					return "/my-post/", nil
				})
				_, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).NotTo(HaveOccurred())

				pageCtx := capturedCtx["page"].(map[string]interface{})
				Expect(pageCtx).NotTo(HaveKey("url"),
					"page.url must NOT be in the template permalink context — "+
						"it's the value being computed, including it would be a circular reference")
			})

			It("actively excludes page.url even when page.URL is set", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
					URL:         "/some/previous/url/", // non-empty — could leak into context
				}
				var capturedCtx map[string]interface{}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					capturedCtx = ctx
					return "/my-post/", nil
				})
				_, err := permalink.Resolve("/:year/:slug/", page, renderer)
				Expect(err).NotTo(HaveOccurred())

				pageCtx := capturedCtx["page"].(map[string]interface{})
				Expect(pageCtx).NotTo(HaveKey("url"),
					"page.url must be actively excluded from the template permalink context — "+
						"even when page.URL is non-empty (e.g., from a previous resolution), "+
						"including it would create a circular reference or stale data")
			})

			It("returns error when template permalink present but no renderer provided", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				// No renderer passed — variadic is empty
				_, err := permalink.Resolve("/:year/:slug/", page)
				Expect(err).To(HaveOccurred(),
					"template permalink with no renderer must return an error — "+
						"if the pipeline forgets to wire a renderer, silently returning "+
						"literal {{ }} in a URL would produce broken output")
			})
		})

		// ── ResolveForSection with renderer ────────────────────────────

		Describe("ResolveForSection with renderer", func() {
			It("template permalink in front matter overrides section pattern", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/posts/"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				permalinkCfg := map[string]string{
					"blog": "/:year/:month/:slug/",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					return "/my-post/posts/", nil
				})
				result, err := permalink.ResolveForSection(page, permalinkCfg, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("/my-post/posts/"),
					"template permalink in front matter must override section pattern from config")
			})

			It("does not invoke renderer for section pattern without {{ in front matter", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				permalinkCfg := map[string]string{
					"blog": "/:year/:slug/",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					Fail("renderer must not be invoked when front matter has no template permalink — " +
						"section patterns use token resolution, not template rendering")
					return "", nil
				})
				result, err := permalink.ResolveForSection(page, permalinkCfg, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("/2026/my-post/"),
					"section pattern must still resolve through token replacement")
			})

			It("does not render {{ in section config patterns through renderer", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				permalinkCfg := map[string]string{
					"blog": "/{{ page.section }}/:slug/",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					Fail("renderer must not be invoked for section config patterns — " +
						"section/default config patterns use token resolution only, " +
						"{{ in a config pattern is treated as literal text")
					return "", nil
				})
				result, err := permalink.ResolveForSection(page, permalinkCfg, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(ContainSubstring("{{ page.section }}"),
					"section config patterns do not support {{ }} — "+
						"the string must pass through token resolution unchanged, "+
						"leaving {{ page.section }} as literal text")
			})
		})

		// ── ResolveFromCascade with renderer ───────────────────────────

		Describe("ResolveFromCascade with renderer", func() {
			It("template permalink in front matter overrides cascade pattern", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post", "permalink": "/{{ page.slug }}/"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				cascadeData := map[string]interface{}{
					"permalink": "/:year/:month/:slug/",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					return "/my-post/", nil
				})
				result, err := permalink.ResolveFromCascade(page, cascadeData, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("/my-post/"),
					"template permalink in front matter must override cascade pattern")
			})

			It("returns error when cascade pattern renders to empty string", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				cascadeData := map[string]interface{}{
					"permalink": "/{{ page.nonexistent }}/",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					return "", nil
				})
				_, err := permalink.ResolveFromCascade(page, cascadeData, renderer)
				Expect(err).To(HaveOccurred(),
					"cascade template permalink rendering to empty string must be a fatal error — "+
						"same rule as front matter template permalinks")
			})

			It("returns error when cascade pattern renders to whitespace", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				cascadeData := map[string]interface{}{
					"permalink": "/{{ page.nonexistent }}/",
				}
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					return "  \n  ", nil
				})
				_, err := permalink.ResolveFromCascade(page, cascadeData, renderer)
				Expect(err).To(HaveOccurred(),
					"cascade template permalink rendering to whitespace must be a fatal error — "+
						"same rule as front matter template permalinks")
			})

			It("renders cascade pattern containing {{ through renderer", func() {
				page := &content.Page{
					Date:        time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC),
					FrontMatter: map[string]interface{}{"title": "My Post"},
					Slug:        "my-post",
					Section:     "blog",
					RelPath:     "blog/my-post.md",
				}
				cascadeData := map[string]interface{}{
					"permalink": "/{{ page.collection }}/{{ page.slug }}/",
				}
				called := false
				renderer := permalink.PermalinkRenderer(func(source string, ctx map[string]interface{}) (string, error) {
					called = true
					Expect(source).To(Equal("/{{ page.collection }}/{{ page.slug }}/"),
						"renderer must receive the cascade pattern as template source")
					return "/blog/my-post/", nil
				})
				result, err := permalink.ResolveFromCascade(page, cascadeData, renderer)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("/blog/my-post/"),
					"cascade pattern with {{ must be rendered through the template engine")
				Expect(called).To(BeTrue(),
					"renderer must be invoked for cascade patterns containing {{")
			})
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
