package output_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/output"
)

var _ = Describe("Feed Templates (§1d)", func() {

	// ── Feed template discovery ──────────────────────────────────────

	Describe("Feed template discovery", func() {
		It("discovers layouts/feed.xml as site-wide feed", func() {
			templates, err := output.ResolveFeedTemplates("testdata/layouts-with-feeds")
			Expect(err).NotTo(HaveOccurred())
			Expect(templates).NotTo(BeNil())

			var siteWide *output.FeedTemplate
			for i := range templates {
				if templates[i].OutputPath == "/feed.xml" {
					siteWide = &templates[i]
					break
				}
			}
			Expect(siteWide).NotTo(BeNil(),
				"layouts/feed.xml must produce /feed.xml output path")
			Expect(siteWide.Section).To(BeEmpty(),
				"site-wide feed must have empty section scope")
		})

		It("discovers layouts/blog/feed.xml as section feed", func() {
			templates, err := output.ResolveFeedTemplates("testdata/layouts-with-feeds")
			Expect(err).NotTo(HaveOccurred())
			Expect(templates).NotTo(BeNil())

			var blogFeed *output.FeedTemplate
			for i := range templates {
				if templates[i].Section == "blog" {
					blogFeed = &templates[i]
					break
				}
			}
			Expect(blogFeed).NotTo(BeNil(),
				"layouts/blog/feed.xml must be discovered as a section feed")
			Expect(blogFeed.OutputPath).To(Equal("/blog/feed.xml"),
				"section feed must output to /blog/feed.xml")
		})

		It("returns empty list when no feed templates exist", func() {
			templates, err := output.ResolveFeedTemplates("testdata/layouts-no-feeds")
			Expect(err).NotTo(HaveOccurred())
			Expect(templates).To(BeEmpty(),
				"no feed templates must produce empty list — feeds are opt-in")
		})
	})

	// ── Feed template rendering ──────────────────────────────────────

	Describe("Feed template rendering", func() {
		It("renders feed template with site pages context", func() {
			tmpl := output.FeedTemplate{
				TemplatePath: "layouts/feed.xml",
				OutputPath:   "/feed.xml",
			}
			context := map[string]interface{}{
				"site": map[string]interface{}{
					"title":   "Test Site",
					"baseURL": "https://example.com",
				},
				"pages": []map[string]interface{}{
					{"title": "Post 1", "url": "/blog/post-1/"},
					{"title": "Post 2", "url": "/blog/post-2/"},
				},
			}
			result, err := output.RenderFeedTemplate(tmpl, context)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty(),
				"feed template must produce output")
		})

		It("feed output is not processed through Markdown", func() {
			tmpl := output.FeedTemplate{
				TemplatePath: "layouts/feed.xml",
				OutputPath:   "/feed.xml",
			}
			result, err := output.RenderFeedTemplate(tmpl, map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeEmpty())
			// Feed XML should not contain markdown-converted HTML artifacts
			body := string(result)
			Expect(body).NotTo(ContainSubstring("<p>"),
				"feed output must not be processed through Markdown rendering")
		})
	})
})
