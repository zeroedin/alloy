package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("ResolveLayout", func() {
	var layoutsDir string

	BeforeEach(func() {
		layoutsDir = "layouts"
	})

	// ── Blog-like section (date-based permalink) ────────────────────────

	Describe("Blog-like section with date-based permalinks", func() {
		// Permalink config indicating blog uses date-based URLs
		permalinkCfg := map[string]string{
			"blog": "/:section/:year/:month/:day/:slug/",
		}

		Context("child page (content/blog/my-post.md)", func() {
			var page *content.Page

			BeforeEach(func() {
				page = &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
			})

			It("uses layout from front matter when specified", func() {
				page.FrontMatter["layout"] = "custom"
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("layouts/custom.liquid"))
			})

			It("falls back to layouts/post.liquid", func() {
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("layouts/post.liquid"))
			})

			It("falls back to layouts/my-post.liquid (filename)", func() {
				// When post.liquid doesn't exist, try the page filename
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("layouts/my-post.liquid"))
			})

			It("falls back to layouts/default.liquid", func() {
				// When no specific layout is found, use default
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("layouts/default.liquid"))
			})

			It("returns error when no layout found", func() {
				_, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).To(HaveOccurred())
				// The error must describe the missing layout, not be a generic stub error
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
			var page *content.Page

			BeforeEach(func() {
				page = &content.Page{
					RelPath:     "blog/index.html",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
			})

			It("falls back to layouts/blog.liquid (section name)", func() {
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("layouts/blog.liquid"))
			})
		})
	})

	// ── Regular section ─────────────────────────────────────────────────

	Describe("Regular section", func() {
		permalinkCfg := map[string]string{}

		Context("content/docs/getting-started.md", func() {
			var page *content.Page

			BeforeEach(func() {
				page = &content.Page{
					RelPath:     "docs/getting-started.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{},
				}
			})

			It("falls back to layouts/getting-started.liquid", func() {
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("layouts/getting-started.liquid"))
			})

			It("falls back to layouts/default.liquid", func() {
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("layouts/default.liquid"))
			})
		})
	})

	// ── layout: false ───────────────────────────────────────────────────

	Describe("layout: false", func() {
		It("skips layout rendering entirely", func() {
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
			result, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(
				SatisfyAny(
					Equal("layouts/taxonomies/tags.liquid"),
					Equal("layouts/tags.liquid"),
				),
				"taxonomy layout must follow spec lookup order",
			)
		})

		It("uses layout override from taxonomy config when specified", func() {
			result, err := tmpl.ResolveTaxonomyLayout("tags", "term", layoutsDir, "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(
				SatisfyAny(
					Equal("layouts/taxonomies/term.liquid"),
					Equal("layouts/term.liquid"),
				),
				"taxonomy layout override must be respected",
			)
		})

		It("returns build error when no taxonomy layout found", func() {
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
			Expect(result).To(Equal("layouts/article.liquid"),
				"layout from _data.yaml cascade must be used when not in front matter")
		})

		It("front matter layout takes priority over _data.yaml layout", func() {
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
			Expect(result).To(Equal("layouts/custom.liquid"),
				"front matter layout must take priority over _data.yaml layout")
		})
	})
})
