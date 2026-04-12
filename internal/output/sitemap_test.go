package output_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/output"
)

var _ = Describe("GenerateSitemap", func() {

	var (
		pages   []*content.Page
		sitemapCfg config.SitemapConfig
		baseURL string
	)

	BeforeEach(func() {
		pages = []*content.Page{
			{
				URL:         "/about/",
				FrontMatter: map[string]interface{}{"title": "About"},
			},
			{
				URL:         "/blog/hello-world/",
				FrontMatter: map[string]interface{}{"title": "Hello World"},
			},
			{
				URL:         "/hidden/",
				FrontMatter: map[string]interface{}{"title": "Hidden", "sitemap": false},
			},
		}
		sitemapCfg = config.SitemapConfig{ChangeFreq: "weekly", Priority: 0.5}
		baseURL = "https://example.com"
	})

	// ── Basic sitemap ─────────────────────────────────────────────────

	Context("Basic sitemap", func() {
		It("generates valid XML", func() {
			xmlBytes, err := output.GenerateSitemap(pages, sitemapCfg, baseURL)
			Expect(err).NotTo(HaveOccurred())
			xmlStr := string(xmlBytes)
			Expect(xmlStr).To(ContainSubstring("<?xml"))
			Expect(xmlStr).To(ContainSubstring("<urlset"))
		})

		It("includes page URLs in sitemap", func() {
			xmlBytes, err := output.GenerateSitemap(pages, sitemapCfg, baseURL)
			Expect(err).NotTo(HaveOccurred())
			xmlStr := string(xmlBytes)
			Expect(xmlStr).To(ContainSubstring("https://example.com/about/"))
			Expect(xmlStr).To(ContainSubstring("https://example.com/blog/hello-world/"))
		})

		It("includes changefreq from config", func() {
			xmlBytes, err := output.GenerateSitemap(pages, sitemapCfg, baseURL)
			Expect(err).NotTo(HaveOccurred())
			xmlStr := string(xmlBytes)
			Expect(xmlStr).To(ContainSubstring("<changefreq>weekly</changefreq>"))
		})
	})

	// ── Per-page overrides ────────────────────────────────────────────

	Context("Per-page overrides", func() {
		It("excludes pages with sitemap false front matter", func() {
			xmlBytes, err := output.GenerateSitemap(pages, sitemapCfg, baseURL)
			Expect(err).NotTo(HaveOccurred())
			xmlStr := string(xmlBytes)
			Expect(xmlStr).NotTo(ContainSubstring("https://example.com/hidden/"))
		})
	})
})
