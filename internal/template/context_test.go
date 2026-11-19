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

	// ── Taxonomy context defaults (issue #380) ──────────────────────

	// ── site.pages front matter exposure (issue #712) ──────────────
	// A site.pages item must give Liquid a page's front matter and its
	// computed fields (url, slug, collection, summary) as top-level
	// properties, so filters like where, sort and map work on the list.
	//
	// These tests check what a template renders. They used to check that
	// ToMap() returns site as a Go map holding a ready-made list, which
	// forced the list to be built for every render, read or not
	// (issue #1360).

	renderWith := func(pages []*content.Page, source string) string {
		GinkgoHelper()
		ctx := template.BuildTemplateContext(page, siteData, pages, collections, nil, nil, "").ToMap()
		engine := template.NewLiquidEngine()
		Expect(template.RegisterBuiltinFilters(engine)).To(Succeed())
		tpl, err := engine.Parse("site-pages", []byte(source))
		Expect(err).NotTo(HaveOccurred())
		out, err := tpl.Render(ctx)
		Expect(err).NotTo(HaveOccurred())
		return string(out)
	}

	It("site.pages items give templates front matter fields (issue #712)", func() {
		out := renderWith(allPages,
			`SIZE[{{ site.pages | size }}] TITLE[{% for p in site.pages %}{{ p.title }}{% endfor %}] `+
				`WHERE[{{ site.pages | where: "title", "My Post" | size }}]`)
		Expect(out).To(ContainSubstring("SIZE[1]"))
		Expect(out).To(ContainSubstring("TITLE[My Post]"),
			"front matter fields must be top-level properties of a "+
				"site.pages item (issue #712)")
		Expect(out).To(ContainSubstring("WHERE[1]"),
			"where must be able to filter site.pages by a front matter "+
				"field (issue #712)")
	})

	It("site.pages items expose struct fields alongside front matter (issue #712)", func() {
		out := renderWith(allPages,
			`{% for p in site.pages %}URL[{{ p.url }}] SLUG[{{ p.slug }}] `+
				`COLL[{{ p.collection }}] SUM[{{ p.summary }}]{% endfor %}`)
		Expect(out).To(ContainSubstring("URL[/blog/my-post/]"),
			"url must be accessible on site.pages items")
		Expect(out).To(ContainSubstring("SLUG[my-post]"),
			"slug must be accessible on site.pages items")
		Expect(out).To(ContainSubstring("COLL[blog]"),
			"collection must be accessible on site.pages items")
		Expect(out).To(ContainSubstring("SUM[A test post]"), // a plain string (#1285)
			"summary must be accessible on site.pages items")
	})

	It("site.pages with multiple pages exposes all front matter (issue #712)", func() {
		docPage := &content.Page{
			RelPath: "docs/quickstart.md",
			Slug:    "quickstart",
			Section: "docs",
			URL:     "/docs/quickstart/",
			Layout:  "doc",
			FrontMatter: map[string]interface{}{
				"title":      "Quickstart",
				"nav_weight": 10,
				"layout":     "doc",
			},
		}
		out := renderWith([]*content.Page{page, docPage},
			`SIZE[{{ site.pages | size }}] `+
				`DOCS[{% assign docs = site.pages | where: "layout", "doc" | sort: "nav_weight" %}`+
				`{% for p in docs %}{{ p.title }}|{{ p.nav_weight }}|{{ p.url }}{% endfor %}]`)
		Expect(out).To(ContainSubstring("SIZE[2]"))
		Expect(out).To(ContainSubstring("DOCS[Quickstart|10|/docs/quickstart/]"),
			"custom front matter fields like nav_weight must be accessible "+
				"on site.pages items — this enables "+
				"site.pages | where: 'layout', 'doc' | sort: 'nav_weight' "+
				"(issue #712)")
	})

	It("taxonomies key is present in ToMap even when nil", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, nil, nil, "")
		m := ctx.ToMap()
		Expect(m).To(HaveKey("taxonomies"),
			"taxonomies must always be present in template context — "+
				"nil taxonomies should default to empty map")
		taxMap, ok := m["taxonomies"].(map[string]interface{})
		Expect(ok).To(BeTrue(),
			"taxonomies must be a map[string]interface{}")
		Expect(taxMap).To(BeEmpty(),
			"nil taxonomies must produce an empty map, not a populated one")
	})

	It("taxonomies key contains data when provided", func() {
		taxCtx := map[string]interface{}{
			"tags": map[string]interface{}{
				"go": []interface{}{page.ToTemplateMap()},
			},
		}
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections, taxCtx, nil, "")
		m := ctx.ToMap()
		Expect(m).To(HaveKey("taxonomies"))
		taxMap, ok := m["taxonomies"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(taxMap).To(HaveKey("tags"),
			"provided taxonomy data must be accessible")
	})
})
