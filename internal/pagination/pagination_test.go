package pagination_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/pagination"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Paginate", func() {

	// ── Virtual pages (perPage 1) ─────────────────────────────────────

	Context("Virtual pages (perPage 1)", func() {
		It("creates one page per data item when perPage is 1", func() {
			data := []interface{}{"a", "b", "c"}
			contexts, paths, err := pagination.Paginate(data, 1, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(3))
			Expect(paths).To(HaveLen(3))
		})

		It("each virtual page receives the item via pagination context Items", func() {
			data := []interface{}{"alpha", "beta"}
			contexts, _, err := pagination.Paginate(data, 1, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(2))
			Expect(contexts[0].Items).To(Equal([]interface{}{"alpha"}))
			Expect(contexts[1].Items).To(Equal([]interface{}{"beta"}))
		})
	})

	// ── Paginated list pages (perPage > 1) ────────────────────────────

	Context("Paginated list pages (perPage > 1)", func() {
		It("chunks items into groups of perPage", func() {
			data := []interface{}{"a", "b", "c", "d", "e"}
			contexts, _, err := pagination.Paginate(data, 2, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(3))
			Expect(contexts[0].Items).To(HaveLen(2))
			Expect(contexts[1].Items).To(HaveLen(2))
			Expect(contexts[2].Items).To(HaveLen(1))
		})

		It("first page outputs at base path with no segment", func() {
			data := []interface{}{"a", "b", "c", "d"}
			_, paths, err := pagination.Paginate(data, 2, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(paths).NotTo(BeEmpty())
			Expect(paths[0]).To(Equal("/blog/"))
		})

		It("subsequent pages output at base/page/N/", func() {
			data := []interface{}{"a", "b", "c", "d", "e"}
			_, paths, err := pagination.Paginate(data, 2, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(paths).To(HaveLen(3))
			Expect(paths[1]).To(Equal("/blog/page/2/"))
			Expect(paths[2]).To(Equal("/blog/page/3/"))
		})

		It("47 items with perPage 10 produces 5 pages", func() {
			data := make([]interface{}, 47)
			for i := range data {
				data[i] = i
			}
			contexts, paths, err := pagination.Paginate(data, 10, "/posts/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(5))
			Expect(paths).To(HaveLen(5))
		})

		It("last page may have fewer items than perPage", func() {
			data := make([]interface{}, 47)
			for i := range data {
				data[i] = i
			}
			contexts, _, err := pagination.Paginate(data, 10, "/posts/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(5))
			Expect(contexts[4].Items).To(HaveLen(7))
		})
	})

	// ── Pagination path configuration ─────────────────────────────────

	Context("Pagination path configuration", func() {
		It("uses provided path segment in output URLs", func() {
			data := []interface{}{"a", "b", "c"}
			_, paths, err := pagination.Paginate(data, 1, "/archive/", "p")
			Expect(err).NotTo(HaveOccurred())
			Expect(paths).To(HaveLen(3))
			Expect(paths[1]).To(Equal("/archive/p/2/"))
		})
	})

	// ── Pagination context object ─────────────────────────────────────

	Context("Pagination context object", func() {
		var contexts []pagination.PaginationContext

		BeforeEach(func() {
			data := []interface{}{"a", "b", "c", "d", "e"}
			var err error
			contexts, _, err = pagination.Paginate(data, 2, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(3))
		})

		It("provides pageNumber (1-based)", func() {
			Expect(contexts[0].PageNumber).To(Equal(1))
			Expect(contexts[1].PageNumber).To(Equal(2))
			Expect(contexts[2].PageNumber).To(Equal(3))
		})

		It("provides totalPages", func() {
			Expect(contexts[0].TotalPages).To(Equal(3))
			Expect(contexts[1].TotalPages).To(Equal(3))
			Expect(contexts[2].TotalPages).To(Equal(3))
		})

		It("provides previousPage as empty string for first page", func() {
			Expect(contexts[0].PreviousPage).To(Equal(""))
		})

		It("provides nextPage as empty string for last page", func() {
			Expect(contexts[2].NextPage).To(Equal(""))
		})

		It("provides first page URL", func() {
			Expect(contexts[0].First).To(Equal("/blog/"))
			Expect(contexts[1].First).To(Equal("/blog/"))
			Expect(contexts[2].First).To(Equal("/blog/"))
		})

		It("provides last page URL", func() {
			Expect(contexts[0].Last).To(Equal("/blog/page/3/"))
			Expect(contexts[1].Last).To(Equal("/blog/page/3/"))
			Expect(contexts[2].Last).To(Equal("/blog/page/3/"))
		})

		It("provides items for current page", func() {
			Expect(contexts[0].Items).To(Equal([]interface{}{"a", "b"}))
			Expect(contexts[1].Items).To(Equal([]interface{}{"c", "d"}))
			Expect(contexts[2].Items).To(Equal([]interface{}{"e"}))
		})
	})

	// ── Edge cases ────────────────────────────────────────────────────

	Context("Edge cases", func() {
		It("handles empty data source", func() {
			data := []interface{}{}
			contexts, paths, err := pagination.Paginate(data, 10, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(BeEmpty())
			Expect(paths).To(BeEmpty())
		})

		It("handles single item", func() {
			data := []interface{}{"only"}
			contexts, paths, err := pagination.Paginate(data, 10, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(1))
			Expect(paths).To(HaveLen(1))
			Expect(contexts[0].Items).To(Equal([]interface{}{"only"}))
		})

		It("handles perPage larger than data length", func() {
			data := []interface{}{"a", "b", "c"}
			contexts, paths, err := pagination.Paginate(data, 100, "/blog/", "page")
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(1))
			Expect(paths).To(HaveLen(1))
			Expect(contexts[0].Items).To(Equal([]interface{}{"a", "b", "c"}))
		})
	})

	// ── Template permalink with renderer callback (issue #315, #422) ──
	// PaginateWithTemplatePermalink accepts a renderer callback so it
	// works with both Liquid and Go template engines.
	// Issue #422: migrated from PaginateWithLiquidPermalink (deleted).

	Context("Template permalink with renderer callback", func() {
		It("renders per-item permalink for virtual pages", func() {
			data := []interface{}{
				map[string]interface{}{"name": "Alice", "slug": "alice"},
				map[string]interface{}{"name": "Bob", "slug": "bob"},
			}
			renderer := func(src string, ctx map[string]interface{}) (string, error) {
				engine := tmpl.NewLiquidEngine()
				tpl, err := engine.Parse("permalink", []byte(src))
				if err != nil {
					return "", err
				}
				result, err := tpl.Render(ctx)
				if err != nil {
					return "", err
				}
				return string(result), nil
			}
			contexts, paths, err := pagination.PaginateWithTemplatePermalink(
				data,
				"/team/{{ member.slug }}/",
				"member",
				renderer,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(2))
			Expect(paths).To(HaveLen(2))
			Expect(paths[0]).To(Equal("/team/alice/"))
			Expect(paths[1]).To(Equal("/team/bob/"))
		})

		It("renders per-item permalink using provided renderer", func() {
			data := []interface{}{
				map[string]interface{}{"name": "Alice", "slug": "alice"},
				map[string]interface{}{"name": "Bob", "slug": "bob"},
			}
			// Use the real Liquid engine — matches the production path
			renderer := func(src string, ctx map[string]interface{}) (string, error) {
				engine := tmpl.NewLiquidEngine()
				tpl, err := engine.Parse("permalink", []byte(src))
				if err != nil {
					return "", err
				}
				result, err := tpl.Render(ctx)
				if err != nil {
					return "", err
				}
				return string(result), nil
			}
			contexts, paths, err := pagination.PaginateWithTemplatePermalink(
				data,
				"/team/{{ member.slug }}/",
				"member",
				renderer,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(contexts).To(HaveLen(2))
			Expect(paths).To(HaveLen(2))
			Expect(paths[0]).To(Equal("/team/alice/"),
				"renderer callback must be used for URL generation")
			Expect(paths[1]).To(Equal("/team/bob/"))
		})
	})

	// ── Data source resolution ────────────────────────────────────────

	Context("Data source resolution", func() {
		It("resolves site.data.team reference to data file contents", func() {
			siteData := map[string]interface{}{
				"team": []interface{}{
					map[string]interface{}{"name": "Alice"},
					map[string]interface{}{"name": "Bob"},
				},
			}
			result, err := pagination.ResolveDataSource("site.data.team", siteData, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(2))
		})

		It("resolves collections.articles reference to collection pages", func() {
			collections := map[string]interface{}{
				"articles": []interface{}{"page1", "page2", "page3"},
			}
			result, err := pagination.ResolveDataSource("collections.articles", nil, collections)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(HaveLen(3))
		})
	})

	// ── Custom path segment ──────────────────────────────────────────

	Context("Custom path segment", func() {
		It("uses 'p' as path segment when configured", func() {
			data := []interface{}{"a", "b", "c", "d", "e"}
			_, paths, err := pagination.Paginate(data, 2, "/articles/", "p")
			Expect(err).NotTo(HaveOccurred())
			Expect(paths).To(HaveLen(3))
			Expect(paths[0]).To(Equal("/articles/"))
			Expect(paths[1]).To(Equal("/articles/p/2/"),
				"custom path segment 'p' must be used instead of default 'page'")
			Expect(paths[2]).To(Equal("/articles/p/3/"))
		})
	})
})
