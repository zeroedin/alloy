package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/pagination"
	"github.com/zeroedin/alloy/internal/template"
)

var _ = Describe("Template Context Shape (§3)", func() {
	var (
		page        *content.Page
		siteData    map[string]interface{}
		allPages    []*content.Page
		collections map[string]interface{}
	)

	BeforeEach(func() {
		page = &content.Page{
			RelPath:    "blog/my-post.md",
			Slug:       "my-post",
			Section:    "blog",
			URL:        "/blog/my-post/",
			Summary:    "A test post",
			Layout:     "post",
			Collection: "blog",
			FrontMatter: map[string]interface{}{
				"title": "My Post",
				"tags":  []string{"go", "web"},
			},
			RenderedBody: []byte("<h1>My Post</h1><p>Hello world</p>"),
		}
		siteData = map[string]interface{}{
			"title":   "Test Site",
			"baseURL": "https://example.com",
			"data": map[string]interface{}{
				"navigation": []map[string]string{
					{"label": "Home", "url": "/"},
					{"label": "About", "url": "/about/"},
				},
			},
		}
		allPages = []*content.Page{page}
		collections = map[string]interface{}{
			"blog": []*content.Page{page},
			"taxonomies": map[string]interface{}{
				"tags": map[string]interface{}{
					"go": []*content.Page{page},
				},
			},
		}
	})

	It("{{ site.title }} is available from config", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Site.Title).To(Equal("Test Site"))
	})

	It("{{ site.data.navigation }} is available from data files", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Site.Data).To(HaveKey("navigation"))
	})

	It("{{ page.title }} is available from front matter", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Page.Title).To(Equal("My Post"))
	})

	It("{{ page.content }} is rendered HTML", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Content).To(ContainSubstring("<h1>"))
	})

	It("{{ page.url }} is the computed permalink", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Page.URL).To(Equal("/blog/my-post/"))
	})

	It("{{ page.collection }} identifies collection membership", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Page.Collection).To(Equal("blog"))
	})

	It("{{ site.pages }} contains all pages", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Site.Pages).To(HaveLen(1))
	})

	It("{{ collections.blog }} is the section collection", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Collections).To(HaveKey("blog"))
	})

	It("{{ collections.taxonomies.tags.go }} is the taxonomy collection", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		taxonomies, ok := ctx.Collections["taxonomies"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		tags, ok := taxonomies["tags"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(tags).To(HaveKey("go"))
	})

	// ── Pagination context injection (§3 + §1c) ─────────────────────

	It("{{ pagination.* }} is available on paginated pages", func() {
		pagCtx := &pagination.PaginationContext{
			PageNumber:   1,
			TotalPages:   3,
			PreviousPage: "",
			NextPage:     "/articles/page/2/",
			First:        "/articles/",
			Last:         "/articles/page/3/",
			Items:        []interface{}{"a", "b", "c"},
		}
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, pagCtx, "articles")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Pagination).NotTo(BeNil())
		Expect(ctx.Pagination.PageNumber).To(Equal(1))
		Expect(ctx.Pagination.TotalPages).To(Equal(3))
		Expect(ctx.Pagination.NextPage).To(Equal("/articles/page/2/"))
		Expect(ctx.Pagination.PreviousPage).To(BeEmpty(),
			"first page must have empty previousPage")
	})

	It("{{ <as> }} is a top-level alias for pagination.items", func() {
		items := []interface{}{
			map[string]interface{}{"title": "Post A"},
			map[string]interface{}{"title": "Post B"},
		}
		pagCtx := &pagination.PaginationContext{
			PageNumber: 1,
			TotalPages: 1,
			Items:      items,
		}
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, pagCtx, "articles")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Custom).To(HaveKey("articles"),
			"as variable must appear in Custom map")
		Expect(ctx.Custom["articles"]).To(Equal(items),
			"as variable must alias pagination.items")
	})

	It("non-paginated pages have nil Pagination and nil Custom", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Pagination).To(BeNil(),
			"non-paginated pages must not have pagination context")
		Expect(ctx.Custom).To(BeNil(),
			"non-paginated pages must not have custom variables")
	})

	It("pagination with per-item pages sets single-item Items", func() {
		item := map[string]interface{}{"name": "Alice", "slug": "alice"}
		pagCtx := &pagination.PaginationContext{
			PageNumber: 1,
			TotalPages: 5,
			Items:      []interface{}{item},
			First:      "/team/alice/",
			Last:       "/team/eve/",
		}
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, pagCtx, "member")
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Custom).To(HaveKey("member"))
		memberItems, ok := ctx.Custom["member"].([]interface{})
		Expect(ok).To(BeTrue())
		Expect(memberItems).To(HaveLen(1),
			"per-item pagination must have exactly one item")
	})
})
