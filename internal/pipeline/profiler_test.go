package pipeline_test

import (
	"bytes"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Profiler", func() {

	// ── StageTimer ──────────────────────────────────────────────────

	Context("StageTimer", func() {
		It("records stage timings in order", func() {
			timer := &pipeline.StageTimer{}
			timer.Start("discovery")
			time.Sleep(1 * time.Millisecond)
			timer.Stop()
			timer.Start("rendering")
			time.Sleep(1 * time.Millisecond)
			timer.Stop()

			timings := timer.Timings()
			Expect(timings).To(HaveLen(2))
			Expect(timings[0].Name).To(Equal("discovery"))
			Expect(timings[1].Name).To(Equal("rendering"))
			Expect(timings[0].Duration).To(BeNumerically(">", 0))
			Expect(timings[1].Duration).To(BeNumerically(">", 0))
		})

		It("auto-stops previous stage when starting a new one", func() {
			timer := &pipeline.StageTimer{}
			timer.Start("stage-a")
			time.Sleep(1 * time.Millisecond)
			timer.Start("stage-b") // should auto-stop stage-a
			time.Sleep(1 * time.Millisecond)
			timer.Stop()

			timings := timer.Timings()
			Expect(timings).To(HaveLen(2))
			Expect(timings[0].Name).To(Equal("stage-a"))
			Expect(timings[1].Name).To(Equal("stage-b"))
		})

		It("Stop on no active stage is a no-op", func() {
			timer := &pipeline.StageTimer{}
			timer.Stop() // should not panic
			Expect(timer.Timings()).To(BeEmpty())
		})

		It("Timings returns empty slice when no stages recorded", func() {
			timer := &pipeline.StageTimer{}
			Expect(timer.Timings()).To(BeEmpty())
		})
	})

	// ── PrintStageTimings ───────────────────────────────────────────

	Context("PrintStageTimings", func() {
		It("prints formatted timing table", func() {
			timings := []pipeline.StageTiming{
				{Name: "discovery", Duration: 100 * time.Millisecond},
				{Name: "rendering", Duration: 400 * time.Millisecond},
			}
			var buf bytes.Buffer
			pipeline.PrintStageTimings(&buf, timings)
			output := buf.String()

			Expect(output).To(ContainSubstring("Stage"))
			Expect(output).To(ContainSubstring("Duration"))
			Expect(output).To(ContainSubstring("%Total"))
			Expect(output).To(ContainSubstring("discovery"))
			Expect(output).To(ContainSubstring("rendering"))
			Expect(output).To(ContainSubstring("Total"))
		})

		It("prints nothing for empty timings", func() {
			var buf bytes.Buffer
			pipeline.PrintStageTimings(&buf, nil)
			Expect(buf.String()).To(BeEmpty())
		})
	})

	// ── BuildOptions.Profile ────────────────────────────────────────

	Context("BuildOptions.Profile populates StageTimings", func() {
		It("Build with Profile=true returns non-empty StageTimings", func() {
			cfg := &config.Config{
				Title:   "Profile Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap, pipeline.BuildOptions{Profile: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.StageTimings).NotTo(BeEmpty(),
				"Build with Profile=true must populate StageTimings — "+
					"if empty, the StageTimer is not wired into Build()")
		})

		It("Build with Profile=false returns empty StageTimings", func() {
			cfg := &config.Config{
				Title:   "No Profile Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.StageTimings).To(BeEmpty(),
				"Build without Profile must not populate StageTimings — "+
					"timing overhead should only be incurred when requested")
		})
	})
})
