package integration_test

import (
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

func fixtureDir(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "fixtures", name)
}

var _ = Describe("Full build pipeline", func() {
	Describe("Minimal site", func() {
		It("builds successfully with minimal fixture", func() {
			cfgPath := filepath.Join(fixtureDir("minimal"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(cfg).NotTo(BeNil())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(BeNumerically(">", 0),
				"minimal site must produce at least one page")
		})

		It("produces output for each content file", func() {
			cfgPath := filepath.Join(fixtureDir("minimal"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PagesRendered).To(ContainElement(ContainSubstring("index")),
				"must render the index page")
		})
	})

	Describe("Cascade site", func() {
		It("builds with data cascade fixture", func() {
			cfgPath := filepath.Join(fixtureDir("cascade"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		// ── Issue #141: 3-level deep cascade merge ────────────────────
		// The cascade fixture has 3 levels of _data.yaml:
		//   content/_data.yaml        → layout: default, author.name: Content Author
		//   content/blog/_data.yaml   → layout: post, author: {name: Blog Author, twitter: @blogauthor}
		//   content/blog/deep/_data.yaml → author.name: Deep Author, category: deep-dive
		//
		// A page at content/blog/deep/nested/leaf.md must inherit merged
		// cascade from all 3 ancestor levels.

		It("3-level deep cascade merges all ancestor values into rendered output", func() {
			cfgPath := filepath.Join(fixtureDir("cascade"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Find the leaf page rendered output (content/blog/deep/nested/leaf.md)
			leafHTML := ""
			for _, pagePath := range result.PagesRendered {
				if filepath.Base(filepath.Dir(pagePath)) == "nested-leaf-post" {
					leafHTML = result.RenderedContent[pagePath]
					break
				}
			}
			Expect(leafHTML).NotTo(BeEmpty(),
				"leaf.md must be rendered")

			// author.name from blog/deep/_data.yaml (deepest override)
			Expect(leafHTML).To(ContainSubstring("Deep Author"),
				"cascade must include author.name from blog/deep/_data.yaml")

			// author.twitter from blog/_data.yaml (inherited — not overridden by deep/)
			Expect(leafHTML).To(ContainSubstring("@blogauthor"),
				"cascade must deep-merge: author.twitter from blog/_data.yaml "+
					"must survive when blog/deep/_data.yaml only overrides author.name")

			// category from blog/deep/_data.yaml (new key at deep level)
			Expect(leafHTML).To(ContainSubstring("deep-dive"),
				"cascade must include category from blog/deep/_data.yaml")

			// layout from blog/_data.yaml (inherited through deep/)
			Expect(leafHTML).To(ContainSubstring("post"),
				"cascade must inherit layout from blog/_data.yaml through deep/ level")
		})
	})

	Describe("Collections site", func() {
		It("builds with collections fixture", func() {
			cfgPath := filepath.Join(fixtureDir("collections"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("creates blog collection from fixture content", func() {
			cfgPath := filepath.Join(fixtureDir("collections"), "alloy.config.yaml")
			cfg, err := config.Load(cfgPath)
			Expect(err).NotTo(HaveOccurred())

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(BeNumerically(">", 0),
				"collections site must produce pages")
		})
	})
})
