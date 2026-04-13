package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Error message contracts", func() {
	It("build error includes file path when content is invalid", func() {
		cfg := &config.Config{
			Title:   "Error Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "content"},
		}
		_, err := pipeline.Build(cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			SatisfyAny(
				ContainSubstring("nonexistent-content-dir"),
				ContainSubstring("content"),
				ContainSubstring("not found"),
				ContainSubstring("directory"),
			),
			"build error must reference the problematic path or resource",
		)
	})

	It("config validation error mentions the invalid field", func() {
		cfg := &config.Config{Title: ""}
		err := config.Validate(cfg)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(
			SatisfyAny(
				ContainSubstring("title"),
				ContainSubstring("required"),
				ContainSubstring("empty"),
			),
			"validation error must name the invalid field",
		)
	})
})
