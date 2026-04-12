package template_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("ResolveLayout", func() {

	// createLayoutsDir builds a temp directory containing the given layout files
	// (empty files) and registers cleanup. Each test controls which candidates
	// exist on disk, so the fallback chain can be tested without contradiction.
	createLayoutsDir := func(files ...string) string {
		dir, err := os.MkdirTemp("", "layouts-*")
		Expect(err).NotTo(HaveOccurred())
		for _, f := range files {
			path := filepath.Join(dir, f)
			Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
			Expect(os.WriteFile(path, []byte("<!-- layout -->"), 0644)).To(Succeed())
		}
		DeferCleanup(func() { os.RemoveAll(dir) })
		return dir
	}

	// ── Blog-like section (date-based permalink) ────────────────────────

	Describe("Blog-like section with date-based permalinks", func() {
		// Permalink config indicating blog uses date-based URLs
		permalinkCfg := map[string]string{
			"blog": "/:section/:year/:month/:day/:slug/",
		}

		Context("child page (content/blog/my-post.md)", func() {
			newPage := func() *content.Page {
				return &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
			}

			It("uses layout from front matter when specified (§4 step 1)", func() {
				layoutsDir := createLayoutsDir("custom.liquid", "post.liquid", "my-post.liquid", "default.liquid")
				page := newPage()
				page.FrontMatter["layout"] = "custom"
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.liquid")),
					"front matter layout must take highest priority even when all other candidates exist")
			})

			It("falls back to layouts/post.liquid for date-based section child (§4 step 2)", func() {
				// post.liquid present, no front matter layout — should be first fallback
				layoutsDir := createLayoutsDir("post.liquid", "my-post.liquid", "default.liquid")
				page := newPage()
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "post.liquid")),
					"post.liquid must be used for children of date-based permalink sections")
			})

			It("falls back to layouts/my-post.liquid when post.liquid missing (§4 step 3)", func() {
				// Only my-post.liquid and default.liquid exist — post.liquid is absent
				layoutsDir := createLayoutsDir("my-post.liquid", "default.liquid")
				page := newPage()
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "my-post.liquid")),
					"filename match must be used when post.liquid is missing")
			})

			It("falls back to layouts/default.liquid when no specific layout exists (§4 step 4)", func() {
				// Only default.liquid exists — post.liquid and my-post.liquid are absent
				layoutsDir := createLayoutsDir("default.liquid")
				page := newPage()
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.liquid")),
					"default.liquid must be the final fallback before build error")
			})

			It("returns build error when no layout found (§4 step 5)", func() {
				// Empty layouts dir — no candidates exist
				layoutsDir := createLayoutsDir()
				page := newPage()
				_, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(
					SatisfyAny(
						ContainSubstring("layout"),
						ContainSubstring("not found"),
						ContainSubstring("no layout"),
					),
					"error should indicate a missing layout",
				)
			})
		})

		Context("index page (content/blog/index.html)", func() {
			It("falls back to layouts/blog.liquid for section index (§4 step 2)", func() {
				// Section name layout exists for index page
				layoutsDir := createLayoutsDir("blog.liquid", "index.liquid", "default.liquid")
				page := &content.Page{
					RelPath:     "blog/index.html",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "blog.liquid")),
					"section name layout must be preferred for index pages")
			})

			It("falls back to layouts/index.liquid when section layout missing (§4 step 3)", func() {
				// Only index.liquid and default exist — blog.liquid is absent
				layoutsDir := createLayoutsDir("index.liquid", "default.liquid")
				page := &content.Page{
					RelPath:     "blog/index.html",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "index.liquid")),
					"filename match (index.liquid) must be tried when section layout is missing")
			})
		})
	})

	// ── Regular section ─────────────────────────────────────────────────

	Describe("Regular section", func() {
		permalinkCfg := map[string]string{}

		Context("content/docs/getting-started.md", func() {
			newPage := func() *content.Page {
				return &content.Page{
					RelPath:     "docs/getting-started.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{},
				}
			}

			It("falls back to layouts/getting-started.liquid (§4 step 2)", func() {
				// Filename match exists along with default
				layoutsDir := createLayoutsDir("getting-started.liquid", "default.liquid")
				page := newPage()
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "getting-started.liquid")),
					"filename match must be preferred over default for regular sections")
			})

			It("falls back to layouts/default.liquid when filename match missing (§4 step 3)", func() {
				// Only default.liquid exists
				layoutsDir := createLayoutsDir("default.liquid")
				page := newPage()
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.liquid")),
					"default.liquid must be used when no filename match exists")
			})
		})
	})

	// ── layout: false ───────────────────────────────────────────────────

	Describe("layout: false", func() {
		It("skips layout rendering entirely", func() {
			layoutsDir := createLayoutsDir("default.liquid")
			page := &content.Page{
				RelPath: "blog/raw-post.md",
				Section: "blog",
				FrontMatter: map[string]interface{}{
					"layout": false,
				},
			}
			result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(""))
		})
	})

	// ── Go template engine layout lookup ────────────────────────────────

	Describe("Go template engine layout lookup", func() {
		It("resolves layouts with bare .html extension for gotemplate engine", func() {
			layoutsDir := createLayoutsDir("my-post.html", "default.html")
			page := &content.Page{
				RelPath:     "blog/my-post.md",
				Section:     "blog",
				FrontMatter: map[string]interface{}{},
			}
			result, err := tmpl.ResolveLayout(page, layoutsDir, "gotemplate", map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			// Go template engine uses bare extension (.html), not .liquid
			Expect(result).NotTo(HaveSuffix(".liquid"),
				"gotemplate engine must not produce .liquid extensions")
			Expect(result).To(
				SatisfyAny(
					HaveSuffix(".html"),
					HaveSuffix(".htm"),
				),
				"gotemplate engine must resolve to bare HTML extension",
			)
		})
	})

	// ── Taxonomy layout lookup ──────────────────────────────────────────

	Describe("Taxonomy layout lookup", func() {
		It("looks up layouts/taxonomies/<name>.liquid first, then layouts/<name>.liquid", func() {
			layoutsDir := createLayoutsDir("taxonomies/tags.liquid", "tags.liquid")
			result, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(filepath.Join(layoutsDir, "taxonomies", "tags.liquid")),
				"taxonomies/ subdirectory layout must take priority")
		})

		It("falls back to layouts/<name>.liquid when taxonomies/ subdir layout missing", func() {
			layoutsDir := createLayoutsDir("tags.liquid")
			result, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(filepath.Join(layoutsDir, "tags.liquid")),
				"root-level taxonomy layout must be used as fallback")
		})

		It("uses layout override from taxonomy config when specified", func() {
			layoutsDir := createLayoutsDir("taxonomies/term.liquid", "term.liquid")
			result, err := tmpl.ResolveTaxonomyLayout("tags", "term", layoutsDir, "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(
				SatisfyAny(
					Equal(filepath.Join(layoutsDir, "taxonomies", "term.liquid")),
					Equal(filepath.Join(layoutsDir, "term.liquid")),
				),
				"taxonomy layout override must be respected",
			)
		})

		It("returns build error when no taxonomy layout found", func() {
			layoutsDir := createLayoutsDir() // empty
			_, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "liquid")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("tags"),
				"taxonomy layout error must mention the taxonomy name")
		})
	})

	// ── Circular layout detection ──────────────────────────────────────

	Describe("Circular layout detection", func() {
		It("detects circular layout references", func() {
			err := tmpl.DetectCircularLayouts("testdata/layouts")
			// When implemented, this should detect A→B→A cycles
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("circular"),
				"error must describe the circular reference")
		})
	})

	// ── Multiple output format layout lookup ────────────────────────────

	Describe("Output format layout lookup", func() {
		It("resolves format-specific layout (single.json.liquid for json output)", func() {
			layoutsDir := createLayoutsDir("single.json.liquid")
			page := &content.Page{
				RelPath:     "blog/my-post.md",
				Section:     "blog",
				Outputs:     []string{"html", "json"},
				FrontMatter: map[string]interface{}{},
			}
			result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(
				SatisfyAny(
					ContainSubstring(".json.liquid"),
					ContainSubstring(".json"),
				),
				"output format layout must include format in the filename",
			)
		})
	})

	// ── Layout from _data.yaml cascade (§4) ─────────────────────────

	Describe("Layout from _data.yaml cascade", func() {
		It("uses layout from _data.yaml when not specified in front matter", func() {
			layoutsDir := createLayoutsDir("article.liquid")
			page := &content.Page{
				RelPath:     "blog/my-post.md",
				Section:     "blog",
				FrontMatter: map[string]interface{}{},
				// No layout in front matter — should come from _data.yaml cascade
			}
			cascadeData := map[string]interface{}{
				"layout": "article",
			}
			result, err := tmpl.ResolveLayoutWithCascade(page, layoutsDir, "liquid", map[string]string{}, cascadeData)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(filepath.Join(layoutsDir, "article.liquid")),
				"layout from _data.yaml cascade must be used when not in front matter")
		})

		It("front matter layout takes priority over _data.yaml layout", func() {
			layoutsDir := createLayoutsDir("custom.liquid", "article.liquid")
			page := &content.Page{
				RelPath:     "blog/my-post.md",
				Section:     "blog",
				FrontMatter: map[string]interface{}{"layout": "custom"},
			}
			cascadeData := map[string]interface{}{
				"layout": "article",
			}
			result, err := tmpl.ResolveLayoutWithCascade(page, layoutsDir, "liquid", map[string]string{}, cascadeData)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.liquid")),
				"front matter layout must take priority over _data.yaml layout")
		})
	})
})
