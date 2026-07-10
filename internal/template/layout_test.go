package template_test

import (
	"fmt"
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

	// ── Layout chaining (issue #276) ──────────────────────────────────
	// Layouts can reference a parent layout via front matter layout:
	// directive. The pipeline renders inside-out, stripping front matter.

	Describe("Layout chaining", func() {
		It("extractLayoutParent reads layout: from layout front matter", func() {
			dir, err := os.MkdirTemp("", "layout-chain-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(dir) })

			// Layout with front matter referencing a parent
			layoutPath := filepath.Join(dir, "child.liquid")
			err = os.WriteFile(layoutPath, []byte("---\nlayout: \"base\"\n---\n<main>{{ content }}</main>"), 0644)
			Expect(err).NotTo(HaveOccurred())

			parent := tmpl.ExtractLayoutParent(layoutPath)
			Expect(parent).To(Equal("base"),
				"extractLayoutParent must read the layout: directive from layout front matter")
		})

		It("extractLayoutParent returns empty for layouts without parent", func() {
			dir, err := os.MkdirTemp("", "layout-chain-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(dir) })

			// Root layout — no front matter
			layoutPath := filepath.Join(dir, "base.liquid")
			err = os.WriteFile(layoutPath, []byte("<html><body>{{ content }}</body></html>"), 0644)
			Expect(err).NotTo(HaveOccurred())

			parent := tmpl.ExtractLayoutParent(layoutPath)
			Expect(parent).To(Equal(""),
				"layouts without front matter layout: directive must return empty parent")
		})

		It("StripLayoutFrontMatter removes front matter from layout content", func() {
			input := "---\nlayout: \"base\"\n---\n<main>{{ content }}</main>"
			stripped := tmpl.StripLayoutFrontMatter(input)
			Expect(stripped).NotTo(ContainSubstring("---"),
				"front matter delimiters must be stripped from layout content")
			Expect(stripped).NotTo(ContainSubstring("layout:"),
				"front matter directives must not appear in rendered output")
			Expect(stripped).To(ContainSubstring("<main>"),
				"layout body content must be preserved after stripping front matter")
		})

		It("ResolveLayoutChain follows parent references to build ordered chain", func() {
			dir, err := os.MkdirTemp("", "layout-chain-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(dir) })

			// base.liquid — root layout (no parent)
			err = os.WriteFile(filepath.Join(dir, "base.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// has-toc.liquid — references base as parent
			err = os.WriteFile(filepath.Join(dir, "has-toc.liquid"),
				[]byte("---\nlayout: \"base\"\n---\n<div class=\"toc-layout\">{{ content }}</div>"), 0644)
			Expect(err).NotTo(HaveOccurred())

			// Render: page content → has-toc → base
			chain, err := tmpl.ResolveLayoutChain(filepath.Join(dir, "has-toc.liquid"), dir, "liquid")
			Expect(err).NotTo(HaveOccurred())
			Expect(chain).To(HaveLen(2),
				"layout chain must include has-toc and base (2 levels)")
			Expect(chain[0]).To(ContainSubstring("has-toc"),
				"first in chain is the innermost layout")
			Expect(chain[1]).To(ContainSubstring("base"),
				"last in chain is the root layout")
		})

		It("layout chain depth exceeding 10 levels returns error", func() {
			dir, err := os.MkdirTemp("", "layout-chain-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(dir) })

			// Create a chain of 12 levels: level-0 → level-1 → ... → level-11
			for i := 0; i < 12; i++ {
				var content string
				if i < 11 {
					content = fmt.Sprintf("---\nlayout: \"level-%d\"\n---\n<div>{{ content }}</div>", i+1)
				} else {
					content = "<div>{{ content }}</div>" // root
				}
				err = os.WriteFile(filepath.Join(dir, fmt.Sprintf("level-%d.liquid", i)), []byte(content), 0644)
				Expect(err).NotTo(HaveOccurred())
			}

			_, err = tmpl.ResolveLayoutChain(filepath.Join(dir, "level-0.liquid"), dir, "liquid")
			Expect(err).To(HaveOccurred(),
				"layout chain exceeding 10 levels must return an error")
		})
	})

	// ── Unified format layout resolution (issue #864) ──────────────────
	// Format layout resolution mirrors the standard HTML chain with the
	// output format infixed before the engine extension. One algorithm,
	// one lookup order. No "single" concept.

	Describe("Unified format layout resolution (issue #864)", func() {

		// ── Blog-like section (date-based permalink) ───────────────────

		Describe("Blog-like section with date-based permalinks", func() {
			permalinkCfg := map[string]string{
				"blog": "/:section/:year/:month/:day/:slug/",
			}

			Context("child page format chain (content/blog/my-post.md, json)", func() {
				newPage := func() *content.Page {
					return &content.Page{
						RelPath:     "blog/my-post.md",
						Section:     "blog",
						Outputs:     []string{"html", "json"},
						FrontMatter: map[string]interface{}{},
					}
				}

				It("uses front-matter layout with format infixed (step 1)", func() {
					layoutsDir := createLayoutsDir("custom.json.liquid", "post.json.liquid", "my-post.json.liquid", "default.json.liquid")
					page := newPage()
					page.FrontMatter["layout"] = "custom"
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.json.liquid")),
						"front-matter layout with format infixed must take highest priority")
				})

				It("falls back to post.json.liquid for date-based section child (step 2)", func() {
					layoutsDir := createLayoutsDir("post.json.liquid", "my-post.json.liquid", "default.json.liquid")
					page := newPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "post.json.liquid")),
						"post with format infixed must be first fallback for date-based section children")
				})

				It("falls back to my-post.json.liquid when post.json.liquid missing (step 3)", func() {
					layoutsDir := createLayoutsDir("my-post.json.liquid", "default.json.liquid")
					page := newPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "my-post.json.liquid")),
						"filename match with format infixed must be used when post layout missing")
				})

				It("falls back to default.json.liquid as final fallback (step 4)", func() {
					layoutsDir := createLayoutsDir("default.json.liquid")
					page := newPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "default.json.liquid")),
						"default with format infixed must be the final fallback")
				})

				It("returns build error when no format layout found (step 5)", func() {
					layoutsDir := createLayoutsDir()
					page := newPage()
					_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(SatisfyAny(
						ContainSubstring("no layout"),
						ContainSubstring("not found"),
					))
				})
			})

			Context("index page format chain (content/blog/index.html, xml)", func() {
				newIndexPage := func() *content.Page {
					return &content.Page{
						RelPath:     "blog/index.html",
						Section:     "blog",
						Outputs:     []string{"html", "xml"},
						FrontMatter: map[string]interface{}{},
					}
				}

				It("falls back to blog.xml.liquid for section index (step 2)", func() {
					layoutsDir := createLayoutsDir("blog.xml.liquid", "index.xml.liquid", "default.xml.liquid")
					page := newIndexPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "xml", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "blog.xml.liquid")),
						"section name with format infixed must be preferred for index pages")
				})

				It("falls back to index.xml.liquid when section layout missing (step 3)", func() {
					layoutsDir := createLayoutsDir("index.xml.liquid", "default.xml.liquid")
					page := newIndexPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "xml", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "index.xml.liquid")),
						"filename match (index.xml.liquid) must be tried when section layout missing")
				})

				It("falls back to default.xml.liquid as final fallback (step 4)", func() {
					layoutsDir := createLayoutsDir("default.xml.liquid")
					page := newIndexPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "xml", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "default.xml.liquid")),
						"default with format infixed must be final fallback for index pages")
				})

				It("returns build error when no format layout found (step 5)", func() {
					layoutsDir := createLayoutsDir()
					page := newIndexPage()
					_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "xml", permalinkCfg)
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(SatisfyAny(
						ContainSubstring("no layout"),
						ContainSubstring("not found"),
					))
				})
			})
		})

		// ── Regular section ────────────────────────────────────────────

		Describe("Regular section", func() {
			permalinkCfg := map[string]string{}

			Context("format chain (content/docs/getting-started.md, json)", func() {
				newPage := func() *content.Page {
					return &content.Page{
						RelPath:     "docs/getting-started.md",
						Section:     "docs",
						Outputs:     []string{"html", "json"},
						FrontMatter: map[string]interface{}{},
					}
				}

				It("uses front-matter layout with format infixed", func() {
					layoutsDir := createLayoutsDir("custom.json.liquid", "getting-started.json.liquid", "default.json.liquid")
					page := newPage()
					page.FrontMatter["layout"] = "custom"
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.json.liquid")),
						"front-matter layout with format infixed must work for regular sections")
				})

				It("falls back to getting-started.json.liquid for filename match", func() {
					layoutsDir := createLayoutsDir("getting-started.json.liquid", "default.json.liquid")
					page := newPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "getting-started.json.liquid")),
						"filename match with format infixed must be preferred for regular sections")
				})

				It("falls back to default.json.liquid when filename match missing", func() {
					layoutsDir := createLayoutsDir("default.json.liquid")
					page := newPage()
					result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).NotTo(HaveOccurred())
					Expect(result).To(Equal(filepath.Join(layoutsDir, "default.json.liquid")),
						"default with format infixed must be the fallback for regular sections")
				})

				It("does not include post step for non-date-based sections", func() {
					// Only post.json.liquid exists — regular section should skip it
					layoutsDir := createLayoutsDir("post.json.liquid")
					page := newPage()
					_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).To(HaveOccurred(),
						"regular sections must not use post layout — only date-based sections do")
				})

				It("does not use section name for non-index pages", func() {
					// docs.json.liquid exists but page is NOT an index — should skip section step
					layoutsDir := createLayoutsDir("docs.json.liquid")
					page := newPage()
					_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
					Expect(err).To(HaveOccurred(),
						"section name layout must only be tried for index pages, not child pages")
				})
			})
		})

		// ── layout: false ──────────────────────────────────────────────

		Describe("layout: false applies to format outputs", func() {
			It("skips format layout resolution entirely", func() {
				layoutsDir := createLayoutsDir("default.json.liquid")
				page := &content.Page{
					RelPath: "blog/raw-post.md",
					Section: "blog",
					Outputs: []string{"html", "json"},
					FrontMatter: map[string]interface{}{
						"layout": false,
					},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(""),
					"layout: false must suppress format output layout resolution — same as HTML chain")
			})
		})

		// ── Cascade _data.yaml support ─────────────────────────────────

		Describe("Cascade _data.yaml flows into format resolution", func() {
			It("uses cascade layout with format infixed", func() {
				layoutsDir := createLayoutsDir("article.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				cascadeData := map[string]interface{}{
					"layout": "article",
				}
				result, err := tmpl.ResolveLayoutForFormatWithCascade(page, layoutsDir, "liquid", "json", map[string]string{}, cascadeData)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "article.json.liquid")),
					"cascade layout must be used with format infixed in format chain")
			})

			It("front-matter layout takes priority over cascade in format chain", func() {
				layoutsDir := createLayoutsDir("custom.json.liquid", "article.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{"layout": "custom"},
				}
				cascadeData := map[string]interface{}{
					"layout": "article",
				}
				result, err := tmpl.ResolveLayoutForFormatWithCascade(page, layoutsDir, "liquid", "json", map[string]string{}, cascadeData)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.json.liquid")),
					"front-matter layout must take priority over cascade in format chain")
			})

			It("falls through to auto candidates when neither front matter nor cascade set", func() {
				layoutsDir := createLayoutsDir("my-post.json.liquid", "default.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				// No cascade layout set — should fall through to auto chain
				result, err := tmpl.ResolveLayoutForFormatWithCascade(page, layoutsDir, "liquid", "json", map[string]string{}, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "my-post.json.liquid")),
					"when no explicit layout, cascade format resolver must fall through to auto candidates")
			})

			It("layout: false in front matter overrides cascade layout in format chain", func() {
				layoutsDir := createLayoutsDir("article.json.liquid", "default.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{
						"layout": false,
					},
				}
				cascadeData := map[string]interface{}{
					"layout": "article",
				}
				result, err := tmpl.ResolveLayoutForFormatWithCascade(page, layoutsDir, "liquid", "json", map[string]string{}, cascadeData)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(""),
					"layout: false in front matter must suppress format output even when cascade sets a layout")
			})
		})

		// ── Bare-extension fallback for format layouts ──────────────────

		Describe("Bare-extension fallback for format layouts", func() {
			It("falls back to default.json when default.json.liquid missing", func() {
				layoutsDir := createLayoutsDir("default.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.json")),
					"Liquid engine must fall back to default.json when default.json.liquid missing")
			})

			It("prefers .json.liquid over bare .json when both exist", func() {
				layoutsDir := createLayoutsDir("default.json.liquid", "default.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.json.liquid")),
					".json.liquid must be preferred over bare .json when both exist")
			})

			It("falls back to default.xml when default.xml.liquid missing", func() {
				layoutsDir := createLayoutsDir("default.xml")
				page := &content.Page{
					RelPath:     "blog/index.md",
					Section:     "blog",
					Outputs:     []string{"html", "xml"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "xml", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.xml")),
					"Liquid engine must fall back to default.xml when default.xml.liquid missing")
			})

			It("per-candidate interleaving: post.json wins over my-post.json.liquid", func() {
				permalinkCfg := map[string]string{
					"blog": "/:section/:year/:month/:day/:slug/",
				}
				layoutsDir := createLayoutsDir("post.json", "my-post.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "post.json")),
					"per-candidate interleaving: post.json (higher-priority, bare extension) must win over my-post.json.liquid (lower-priority)")
			})

			It("falls back to filename-specific bare extension format layout", func() {
				layoutsDir := createLayoutsDir("my-post.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "my-post.json")),
					"Liquid engine must fall back to filename-specific bare extension format layout")
			})
		})

		// ── No "single" concept ────────────────────────────────────────

		Describe("No single concept in format chain", func() {
			It("does not look for single.json.liquid in the chain", func() {
				// Only single.json.liquid exists — the unified chain must not find it
				// because "single" is not a valid auto candidate
				layoutsDir := createLayoutsDir("single.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"single is not a valid auto candidate — unified chain must not look for single.json.liquid")
			})
		})

		// ── Go template engine (issue #834) ───────────────────────────
		// PLAN.md §1e: "Go engine — uses bare extension files directly."
		// For format layouts, Go engine uses name.format (bare extension),
		// NOT name.format.html. The format extension IS the file extension.

		Describe("Go engine format layout uses bare extensions (issue #834)", func() {

			It("resolves JSON format layout with bare extension (default.json, not default.json.html)", func() {
				layoutsDir := createLayoutsDir("default.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.json")),
					"Go engine must use bare .format extension — default.json, not default.json.html")
			})

			It("resolves XML format layout with bare extension (default.xml, not default.xml.html)", func() {
				layoutsDir := createLayoutsDir("default.xml")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "xml"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "xml", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.xml")),
					"Go engine must use bare .format extension — default.xml, not default.xml.html")
			})

			It("resolves post.json for date-based section child with bare extension", func() {
				permalinkCfg := map[string]string{
					"blog": "/:section/:year/:month/:day/:slug/",
				}
				layoutsDir := createLayoutsDir("post.json", "default.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "post.json")),
					"Go engine must resolve post.json (bare extension) for date-based section children")
			})

			It("resolves section-specific format layout with bare extension (blog.xml)", func() {
				layoutsDir := createLayoutsDir("blog.xml", "default.xml")
				page := &content.Page{
					RelPath:     "blog/index.html",
					Section:     "blog",
					Outputs:     []string{"html", "xml"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "xml", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "blog.xml")),
					"Go engine must resolve blog.xml (bare extension) for section index format layouts")
			})

			It("resolves filename-specific format layout with bare extension (my-post.json)", func() {
				layoutsDir := createLayoutsDir("my-post.json", "default.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "my-post.json")),
					"Go engine must resolve my-post.json (bare extension) for filename-specific format layouts")
			})

			It("resolves front-matter layout with bare format extension (custom.json)", func() {
				layoutsDir := createLayoutsDir("custom.json", "default.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{"layout": "custom"},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.json")),
					"Go engine front-matter layout must use bare format extension — custom.json, not custom.json.html")
			})

			It("does NOT match name.format.html — rejects engine suffix on format candidates", func() {
				// Only the wrong pattern exists (name.format.html) — Go engine must not find it
				layoutsDir := createLayoutsDir("default.json.html")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"Go engine must NOT match default.json.html — bare extension (default.json) is the only valid pattern")
			})

			It("gotemplate does NOT try .liquid files for format layouts", func() {
				layoutsDir := createLayoutsDir("default.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"Go engine must never try .liquid files for format layout resolution")
			})

			It("respects full candidate chain priority with bare extensions", func() {
				// All candidates exist — post.json must win (highest priority for date-based section child)
				permalinkCfg := map[string]string{
					"blog": "/:section/:year/:month/:day/:slug/",
				}
				layoutsDir := createLayoutsDir("post.json", "my-post.json", "default.json")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "post.json")),
					"Go engine format candidate chain must follow same priority as HTML chain — post > filename > default")
			})

			It("returns build error when no bare-extension format layout found", func() {
				layoutsDir := createLayoutsDir()
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", map[string]string{})
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(SatisfyAny(
					ContainSubstring("no layout"),
					ContainSubstring("not found"),
				))
			})
		})

		// ── Extension-bearing layout + format outputs (issue #869) ──────

		Describe("Extension-bearing layout name with format outputs (issue #869)", func() {

			It("errors when layout: 'article.liquid' is used with format outputs", func() {
				layoutsDir := createLayoutsDir("article.liquid", "article.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{"layout": "article.liquid"},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"extension-bearing layout must error when used with format outputs")
				Expect(err.Error()).To(ContainSubstring("extension-bearing"),
					"error must mention that the layout name is extension-bearing")
				Expect(err.Error()).To(ContainSubstring("article"),
					"error must suggest the bare name alternative")
			})

			It("errors when layout: 'article.html' is used with format outputs", func() {
				layoutsDir := createLayoutsDir("article.html", "article.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{"layout": "article.html"},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"extension-bearing .html layout must error when used with format outputs")
			})

			It("errors when layout: 'feed.xml' is used with format outputs", func() {
				layoutsDir := createLayoutsDir("feed.xml")
				page := &content.Page{
					RelPath:     "blog/index.md",
					Section:     "blog",
					Outputs:     []string{"html", "xml"},
					FrontMatter: map[string]interface{}{"layout": "feed.xml"},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "xml", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"extension-bearing .xml layout must error when used with format outputs")
			})

			It("does NOT error for extension-bearing layout with default HTML-only output", func() {
				// layout: "article.liquid" with no outputs key (HTML only) is fine —
				// ResolveLayout handles it, ResolveLayoutForFormat is never called.
				// This test exercises ResolveLayout to confirm no regression.
				layoutsDir := createLayoutsDir("article.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{"layout": "article.liquid"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing layout with default HTML output must not error")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "article.liquid")),
					"extension-bearing layout must resolve normally for HTML-only pages")
			})

			It("does NOT error for bare layout name with format outputs", func() {
				// layout: "article" (bare name) + outputs: [html, json] is fine —
				// format infixing produces article.json.liquid as expected.
				layoutsDir := createLayoutsDir("article.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{"layout": "article"},
				}
				result, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "liquid", "json", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"bare layout name with format outputs must not error — format infixing works")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "article.json.liquid")),
					"bare layout name must resolve via format infixing")
			})

			It("errors for extension-bearing cascade layout with format outputs", func() {
				layoutsDir := createLayoutsDir("article.liquid", "article.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				cascadeData := map[string]interface{}{
					"layout": "article.liquid",
				}
				_, err := tmpl.ResolveLayoutForFormatWithCascade(page, layoutsDir, "liquid", "json", map[string]string{}, cascadeData)
				Expect(err).To(HaveOccurred(),
					"extension-bearing cascade layout must error when used with format outputs")
				Expect(err.Error()).To(ContainSubstring("extension-bearing"),
					"error must mention that the cascade layout name is extension-bearing")
			})
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

	// ── Liquid bare-extension layout fallback (issue #827, revised #860) ──
	// PLAN.md §1e: "If the Liquid engine finds no .liquid file, it falls
	// back to the bare extension and parses it as Liquid."
	// Bare layout names (no extension) get .liquid → .html fallback.
	// Extension-bearing names (e.g., "base.html") are used as-is.
	// Both apply to front matter, cascade, and layout chain sources.

	Describe("Liquid bare-extension layout fallback (issue #827)", func() {

		// ── ResolveLayout ──────────────────────────────────────────

		Describe("ResolveLayout", func() {

			It("falls back to default.html when default.liquid is missing", func() {
				// Only bare-extension default exists — Liquid engine must find it
				layoutsDir := createLayoutsDir("default.html")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.html")),
					"Liquid engine must fall back to default.html when default.liquid is missing")
			})

			It("prefers default.liquid over default.html when both exist", func() {
				layoutsDir := createLayoutsDir("default.liquid", "default.html")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.liquid")),
					".liquid must be preferred over bare extension when both exist")
			})

			It("falls back to post.html for date-based section when post.liquid missing", func() {
				permalinkCfg := map[string]string{
					"blog": "/:section/:year/:month/:day/:slug/",
				}
				// post.html exists but post.liquid does not
				layoutsDir := createLayoutsDir("post.html")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "post.html")),
					"Liquid engine must fall back to post.html for date-based sections")
			})

			It("falls back to section-name.html for index page when .liquid missing", func() {
				layoutsDir := createLayoutsDir("blog.html")
				page := &content.Page{
					RelPath:     "blog/index.html",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "blog.html")),
					"Liquid engine must fall back to blog.html for section index pages")
			})

			It("falls back to filename.html when filename.liquid missing", func() {
				layoutsDir := createLayoutsDir("getting-started.html")
				page := &content.Page{
					RelPath:     "docs/getting-started.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "getting-started.html")),
					"Liquid engine must fall back to filename.html when filename.liquid missing")
			})

			It("falls back to custom.html for bare front matter layout when custom.liquid missing", func() {
				// layout: "custom" is a bare name (no extension) — gets .liquid → .html
				// fallback, same as auto candidates. Must find custom.html, not error.
				layoutsDir := createLayoutsDir("custom.html", "default.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "custom"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"bare front matter layout name must fall back to .html when .liquid missing")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.html")),
					"bare front matter layout 'custom' must resolve to custom.html via fallback")
			})

			It("per-candidate interleaving: post.html wins over my-post.liquid", func() {
				// When both post.html (bare extension of higher-priority candidate) and
				// my-post.liquid (lower-priority candidate) exist, per-candidate
				// interleaving means post.html is tried first.
				// This disambiguates per-candidate from global ordering.
				permalinkCfg := map[string]string{
					"blog": "/:section/:year/:month/:day/:slug/",
				}
				layoutsDir := createLayoutsDir("post.html", "my-post.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "post.html")),
					"per-candidate interleaving: post.html (higher-priority candidate, bare extension) must win over my-post.liquid (lower-priority candidate)")
			})

			It("falls back to my-post.html for date-based section when post and my-post.liquid missing", func() {
				// Date-based section, post.liquid/post.html missing, my-post.liquid missing,
				// but my-post.html exists — should find it via filename step bare-extension.
				permalinkCfg := map[string]string{
					"blog": "/:section/:year/:month/:day/:slug/",
				}
				layoutsDir := createLayoutsDir("my-post.html")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", permalinkCfg)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "my-post.html")),
					"Liquid engine must fall back to my-post.html for date-based section filename step")
			})
		})

		// ── Extension-bearing layout names (issue #860) ──────────────

		Describe("Extension-bearing layout names (issue #860)", func() {

			It("resolves layout: 'base.html' to base.html directly without appending .liquid", func() {
				// Extension-bearing name — used as literal filename. No .liquid appended.
				layoutsDir := createLayoutsDir("base.html", "base.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "base.html"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing layout name must not error when file exists")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "base.html")),
					"layout: 'base.html' must resolve to base.html directly — not base.html.liquid, not base.liquid")
			})

			It("resolves layout: 'base.liquid' to base.liquid directly without double extension", func() {
				// Extension-bearing name with .liquid — used as-is. Must not produce base.liquid.liquid.
				layoutsDir := createLayoutsDir("base.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "base.liquid"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing .liquid layout name must not error when file exists")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "base.liquid")),
					"layout: 'base.liquid' must resolve to base.liquid directly — no double extension")
			})

			It("errors for extension-bearing layout when file missing", func() {
				// layout: "missing.html" — extension-bearing, used as literal filename.
				// File doesn't exist — build error. No fallback, no auto candidates.
				layoutsDir := createLayoutsDir("default.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "missing.html"},
				}
				_, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"extension-bearing layout with missing file must error — no fallback, no auto candidates")
			})

			It("resolves layout: 'feed.xml' to feed.xml directly", func() {
				// Extension-bearing name with output format extension.
				layoutsDir := createLayoutsDir("feed.xml", "default.liquid")
				page := &content.Page{
					RelPath:     "blog/index.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{"layout": "feed.xml"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing .xml layout name must not error when file exists")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "feed.xml")),
					"layout: 'feed.xml' must resolve to feed.xml directly — recognized output format extension")
			})

			It("resolves extension-bearing cascade layout: 'sidebar.html' directly", func() {
				// Extension-bearing name via cascade — same literal filename behavior.
				layoutsDir := createLayoutsDir("sidebar.html", "default.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{},
				}
				cascadeData := map[string]interface{}{
					"layout": "sidebar.html",
				}
				result, err := tmpl.ResolveLayoutWithCascade(page, layoutsDir, "liquid", map[string]string{}, cascadeData)
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing cascade layout must resolve directly when file exists")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "sidebar.html")),
					"cascade layout: 'sidebar.html' must resolve to sidebar.html directly")
			})

			It("resolves layout: 'data.json' to data.json directly", func() {
				// .json is a recognized extension — used as literal filename.
				layoutsDir := createLayoutsDir("data.json", "default.liquid")
				page := &content.Page{
					RelPath:     "api/endpoint.md",
					Section:     "api",
					FrontMatter: map[string]interface{}{"layout": "data.json"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing .json layout name must not error when file exists")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "data.json")),
					"layout: 'data.json' must resolve to data.json directly")
			})

			It("resolves layout: 'page.txt' to page.txt directly", func() {
				// .txt is a recognized extension — used as literal filename.
				layoutsDir := createLayoutsDir("page.txt", "default.liquid")
				page := &content.Page{
					RelPath:     "docs/readme.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "page.txt"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing .txt layout name must not error when file exists")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "page.txt")),
					"layout: 'page.txt' must resolve to page.txt directly")
			})

			It("treats unrecognized extension .md as bare name", func() {
				// layout: "page.md" — .md is NOT a recognized extension.
				// Treated as bare name: tries page.md.liquid then page.md.html.
				layoutsDir := createLayoutsDir("page.md.html", "default.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "page.md"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"unrecognized extension .md must be treated as bare name with fallback")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "page.md.html")),
					"layout: 'page.md' must try page.md.liquid then page.md.html — .md is not a recognized extension")
			})

			It("resolves dotted name with recognized final extension", func() {
				// layout: "some.thing.html" — filepath.Ext returns ".html" (recognized).
				// Used as literal filename, not treated as bare name.
				layoutsDir := createLayoutsDir("some.thing.html", "default.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "some.thing.html"},
				}
				result, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).NotTo(HaveOccurred(),
					"dotted name with recognized final extension must resolve as extension-bearing")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "some.thing.html")),
					"layout: 'some.thing.html' must resolve to some.thing.html directly — final .html is recognized")
			})

			It("errors for bare name when neither .liquid nor .html exists", func() {
				// layout: "article" — bare name, tries article.liquid then article.html.
				// Neither exists — must error, must not fall through to auto candidates.
				layoutsDir := createLayoutsDir("default.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{"layout": "article"},
				}
				_, err := tmpl.ResolveLayout(page, layoutsDir, "liquid", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"bare layout name must error when neither .liquid nor .html exists — must not fall through to auto candidates")
			})
		})

		// ── ResolveLayoutForFormat (bare-extension tests moved to ────
		// "Unified format layout resolution" section — issue #864) ──────

		// ── ResolveTaxonomyLayout ──────────────────────────────────

		Describe("ResolveTaxonomyLayout", func() {

			It("falls back to taxonomies/tags.html when taxonomies/tags.liquid missing", func() {
				layoutsDir := createLayoutsDir("taxonomies/tags.html")
				result, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "liquid")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "taxonomies", "tags.html")),
					"Liquid engine must fall back to taxonomies/tags.html")
			})

			It("falls back to tags.html when tags.liquid missing", func() {
				layoutsDir := createLayoutsDir("tags.html")
				result, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "liquid")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "tags.html")),
					"Liquid engine must fall back to tags.html at root level")
			})

			It("prefers taxonomies/tags.liquid over taxonomies/tags.html", func() {
				layoutsDir := createLayoutsDir("taxonomies/tags.liquid", "taxonomies/tags.html")
				result, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "liquid")
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "taxonomies", "tags.liquid")),
					".liquid must be preferred over bare extension for taxonomy layouts")
			})
		})

		// ── ResolveLayoutChain ─────────────────────────────────────

		Describe("ResolveLayoutChain", func() {

			It("resolves parent layout via bare extension when .liquid missing", func() {
				dir, err := os.MkdirTemp("", "layout-chain-fallback-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(dir) })

				// base.html — root layout (bare extension, no .liquid)
				err = os.WriteFile(filepath.Join(dir, "base.html"),
					[]byte("<html><body>{{ content }}</body></html>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// child.liquid — references base as parent
				err = os.WriteFile(filepath.Join(dir, "child.liquid"),
					[]byte("---\nlayout: \"base\"\n---\n<main>{{ content }}</main>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				chain, err := tmpl.ResolveLayoutChain(filepath.Join(dir, "child.liquid"), dir, "liquid")
				Expect(err).NotTo(HaveOccurred())
				Expect(chain).To(HaveLen(2),
					"chain must include child and base (2 levels)")
				Expect(chain[0]).To(ContainSubstring("child.liquid"))
				Expect(chain[1]).To(Equal(filepath.Join(dir, "base.html")),
					"parent layout must be resolved via bare extension when .liquid missing")
			})

			It("prefers parent.liquid over parent.html in chain", func() {
				dir, err := os.MkdirTemp("", "layout-chain-fallback-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(dir) })

				// base.liquid AND base.html both exist
				err = os.WriteFile(filepath.Join(dir, "base.liquid"),
					[]byte("<html><body>{{ content }}</body></html>"), 0644)
				Expect(err).NotTo(HaveOccurred())
				err = os.WriteFile(filepath.Join(dir, "base.html"),
					[]byte("<html><body>{{ content }}</body></html>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// child.liquid references base
				err = os.WriteFile(filepath.Join(dir, "child.liquid"),
					[]byte("---\nlayout: \"base\"\n---\n<main>{{ content }}</main>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				chain, err := tmpl.ResolveLayoutChain(filepath.Join(dir, "child.liquid"), dir, "liquid")
				Expect(err).NotTo(HaveOccurred())
				Expect(chain).To(HaveLen(2))
				Expect(chain[1]).To(Equal(filepath.Join(dir, "base.liquid")),
					".liquid must be preferred over bare extension for parent layouts in chain")
			})

			It("resolves mixed chain with .liquid and bare-extension parents", func() {
				dir, err := os.MkdirTemp("", "layout-chain-fallback-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(dir) })

				// root.html — bare extension root (no .liquid)
				err = os.WriteFile(filepath.Join(dir, "root.html"),
					[]byte("<html>{{ content }}</html>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// middle.liquid — .liquid file referencing root
				err = os.WriteFile(filepath.Join(dir, "middle.liquid"),
					[]byte("---\nlayout: \"root\"\n---\n<div>{{ content }}</div>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// inner.liquid — references middle
				err = os.WriteFile(filepath.Join(dir, "inner.liquid"),
					[]byte("---\nlayout: \"middle\"\n---\n<section>{{ content }}</section>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				chain, err := tmpl.ResolveLayoutChain(filepath.Join(dir, "inner.liquid"), dir, "liquid")
				Expect(err).NotTo(HaveOccurred())
				Expect(chain).To(HaveLen(3),
					"chain must include inner, middle, and root (3 levels)")
				Expect(chain[0]).To(ContainSubstring("inner.liquid"))
				Expect(chain[1]).To(Equal(filepath.Join(dir, "middle.liquid")))
				Expect(chain[2]).To(Equal(filepath.Join(dir, "root.html")),
					"bare-extension parent must be resolved at any level in the chain")
			})

			It("errors when parent layout has neither .liquid nor bare extension", func() {
				dir, err := os.MkdirTemp("", "layout-chain-fallback-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(dir) })

				// child.liquid references "missing" — neither missing.liquid nor missing.html exists
				err = os.WriteFile(filepath.Join(dir, "child.liquid"),
					[]byte("---\nlayout: \"missing\"\n---\n<main>{{ content }}</main>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = tmpl.ResolveLayoutChain(filepath.Join(dir, "child.liquid"), dir, "liquid")
				Expect(err).To(HaveOccurred(),
					"chain must error when parent has neither .liquid nor bare extension")
				Expect(err.Error()).To(ContainSubstring("missing"),
					"error must reference the missing parent layout name")
			})

			It("resolves extension-bearing chain parent layout: 'base.html' directly", func() {
				// Chain parent has extension-bearing name — use as literal filename.
				// Must not append .liquid (producing base.html.liquid).
				dir, err := os.MkdirTemp("", "layout-chain-ext-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(dir) })

				// base.html exists as the parent layout
				err = os.WriteFile(filepath.Join(dir, "base.html"),
					[]byte("<html>{{ content }}</html>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// child.liquid references "base.html" — extension-bearing
				err = os.WriteFile(filepath.Join(dir, "child.liquid"),
					[]byte("---\nlayout: \"base.html\"\n---\n<main>{{ content }}</main>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				chain, err := tmpl.ResolveLayoutChain(filepath.Join(dir, "child.liquid"), dir, "liquid")
				Expect(err).NotTo(HaveOccurred(),
					"extension-bearing chain parent must resolve directly when file exists")
				Expect(chain).To(HaveLen(2))
				Expect(chain[1]).To(Equal(filepath.Join(dir, "base.html")),
					"chain parent layout: 'base.html' must resolve to base.html directly — no .liquid appended")
			})

			It("errors for extension-bearing chain parent when file missing", func() {
				dir, err := os.MkdirTemp("", "layout-chain-ext-missing-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(dir) })

				// child.liquid references "gone.html" — extension-bearing, file doesn't exist
				err = os.WriteFile(filepath.Join(dir, "child.liquid"),
					[]byte("---\nlayout: \"gone.html\"\n---\n<main>{{ content }}</main>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = tmpl.ResolveLayoutChain(filepath.Join(dir, "child.liquid"), dir, "liquid")
				Expect(err).To(HaveOccurred(),
					"extension-bearing chain parent with missing file must error")
			})
		})

		// ── ResolveLayoutWithCascade ───────────────────────────────

		Describe("ResolveLayoutWithCascade", func() {

			It("falls back to article.html for bare cascade layout when article.liquid missing", func() {
				// Cascade sets layout: "article" — a bare name (no extension).
				// Gets .liquid → .html fallback. Must find article.html, not error.
				layoutsDir := createLayoutsDir("article.html", "default.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				cascadeData := map[string]interface{}{
					"layout": "article",
				}
				result, err := tmpl.ResolveLayoutWithCascade(page, layoutsDir, "liquid", map[string]string{}, cascadeData)
				Expect(err).NotTo(HaveOccurred(),
					"bare cascade layout name must fall back to .html when .liquid missing")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "article.html")),
					"bare cascade layout 'article' must resolve to article.html via fallback")
			})

			It("falls back to custom.html for bare front matter layout via cascade path when custom.liquid missing", func() {
				// Front matter layout: "custom" via ResolveLayoutWithCascade — bare name,
				// gets .liquid → .html fallback same as direct ResolveLayout path.
				layoutsDir := createLayoutsDir("custom.html", "default.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{"layout": "custom"},
				}
				result, err := tmpl.ResolveLayoutWithCascade(page, layoutsDir, "liquid", map[string]string{}, nil)
				Expect(err).NotTo(HaveOccurred(),
					"bare front matter layout via ResolveLayoutWithCascade must fall back to .html when .liquid missing")
				Expect(result).To(Equal(filepath.Join(layoutsDir, "custom.html")),
					"bare front matter layout 'custom' via cascade path must resolve to custom.html via fallback")
			})

			It("falls through to ResolveLayout bare-extension fallback when no explicit layout set", func() {
				// No front matter layout, no cascade layout — falls through to ResolveLayout.
				// Only bare-extension default.html exists — Liquid engine must find it.
				layoutsDir := createLayoutsDir("default.html")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					FrontMatter: map[string]interface{}{},
				}
				result, err := tmpl.ResolveLayoutWithCascade(page, layoutsDir, "liquid", map[string]string{}, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(filepath.Join(layoutsDir, "default.html")),
					"when no explicit layout, ResolveLayoutWithCascade must fall through to ResolveLayout which applies bare-extension fallback")
			})
		})

		// ── Go engine unchanged ───────────────────────────────────

		Describe("Go engine unchanged", func() {

			It("gotemplate engine does NOT try .liquid files", func() {
				// Only .liquid file exists — Go engine must not find it
				layoutsDir := createLayoutsDir("default.liquid")
				page := &content.Page{
					RelPath:     "docs/guide.md",
					Section:     "docs",
					FrontMatter: map[string]interface{}{},
				}
				_, err := tmpl.ResolveLayout(page, layoutsDir, "gotemplate", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"Go engine must never try .liquid files — no reverse fallback")
			})

			It("gotemplate ResolveLayoutForFormat does NOT try .liquid files", func() {
				layoutsDir := createLayoutsDir("default.json.liquid")
				page := &content.Page{
					RelPath:     "blog/my-post.md",
					Section:     "blog",
					Outputs:     []string{"html", "json"},
					FrontMatter: map[string]interface{}{},
				}
				_, err := tmpl.ResolveLayoutForFormat(page, layoutsDir, "gotemplate", "json", map[string]string{})
				Expect(err).To(HaveOccurred(),
					"Go engine ResolveLayoutForFormat must never try .liquid files")
			})

			It("gotemplate ResolveTaxonomyLayout does NOT try .liquid files", func() {
				layoutsDir := createLayoutsDir("taxonomies/tags.liquid", "tags.liquid")
				_, err := tmpl.ResolveTaxonomyLayout("tags", "", layoutsDir, "gotemplate")
				Expect(err).To(HaveOccurred(),
					"Go engine ResolveTaxonomyLayout must never try .liquid files")
			})

			It("gotemplate ResolveLayoutChain does NOT try .liquid files for parent", func() {
				dir, err := os.MkdirTemp("", "layout-chain-gotemplate-*")
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(func() { os.RemoveAll(dir) })

				// base.liquid exists but NOT base.html — Go engine must not find it
				err = os.WriteFile(filepath.Join(dir, "base.liquid"),
					[]byte("<html>{{ content }}</html>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				// child.html references base
				err = os.WriteFile(filepath.Join(dir, "child.html"),
					[]byte("---\nlayout: \"base\"\n---\n<main>{{ content }}</main>"), 0644)
				Expect(err).NotTo(HaveOccurred())

				_, err = tmpl.ResolveLayoutChain(filepath.Join(dir, "child.html"), dir, "gotemplate")
				Expect(err).To(HaveOccurred(),
					"Go engine ResolveLayoutChain must never try .liquid files for parent resolution")
			})
		})
	})
})
