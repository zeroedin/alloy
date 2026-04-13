package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
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
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Site.Title).To(Equal("Test Site"))
	})

	It("{{ site.data.navigation }} is available from data files", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Site.Data).To(HaveKey("navigation"))
	})

	It("{{ page.title }} is available from front matter", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Page.Title).To(Equal("My Post"))
	})

	It("{{ page.content }} is rendered HTML", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Content).To(ContainSubstring("<h1>"))
	})

	It("{{ page.url }} is the computed permalink", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Page.URL).To(Equal("/blog/my-post/"))
	})

	It("{{ page.collection }} identifies collection membership", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Page.Collection).To(Equal("blog"))
	})

	It("{{ site.pages }} contains all pages", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Site.Pages).To(HaveLen(1))
	})

	It("{{ collections.blog }} is the section collection", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		Expect(ctx.Collections).To(HaveKey("blog"))
	})

	It("{{ collections.taxonomies.tags.go }} is the taxonomy collection", func() {
		ctx := template.BuildTemplateContext(page, siteData, allPages, collections)
		Expect(ctx).NotTo(BeNil())
		taxonomies, ok := ctx.Collections["taxonomies"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		tags, ok := taxonomies["tags"].(map[string]interface{})
		Expect(ok).To(BeTrue())
		Expect(tags).To(HaveKey("go"))
	})
})
