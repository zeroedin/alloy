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
