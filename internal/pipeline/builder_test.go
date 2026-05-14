package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Builder interface", func() {

	It("DefaultBuilder satisfies the Builder interface (issue #601)", func() {
		var b pipeline.Builder = &pipeline.DefaultBuilder{}
		Expect(b).NotTo(BeNil(),
			"DefaultBuilder must implement the Builder interface — "+
				"this decouples cmd/dev.go from direct pipeline function calls "+
				"so rebuild orchestration can be tested with a mock (issue #601)")
	})

	It("DefaultBuilder.Build produces a valid BuildResult (issue #601)", func() {
		builder := &pipeline.DefaultBuilder{}
		cfg := &config.Config{
			Title:   "Builder Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}

		result, err := builder.Build(cfg, pipeline.BuildOptions{SkipSSR: true})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.PageCount).To(BeNumerically(">=", 0),
			"DefaultBuilder.Build must delegate to the pipeline and return a valid result")
	})

	It("DefaultBuilder.BuildIncremental returns result with cache (issue #601)", func() {
		builder := &pipeline.DefaultBuilder{}
		cfg := &config.Config{
			Title:   "Builder Incremental Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md": "---\ntitle: Home\n---\n# Home",
		}

		result, err := builder.BuildIncremental(cfg, contentMap, nil, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		Expect(result.Cache).NotTo(BeNil(),
			"DefaultBuilder.BuildIncremental must return an in-memory cache "+
				"on result.Cache — the implementation must populate result.Cache "+
				"with content hashes for incremental rebuild (issues #601, #639)")
	})
})
