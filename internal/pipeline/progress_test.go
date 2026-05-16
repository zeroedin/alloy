package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Build Pipeline", func() {
	Describe("Progress reporter", func() {
		It("Build calls StartStage, Update, EndStage, and Summary", func() {
			spy := &spyReporter{}

			cfg := &config.Config{
				Title:   "Progress Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg, pipeline.BuildOptions{Reporter: spy})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(spy.stages).NotTo(BeEmpty(),
				"Build must call StartStage at least once — "+
					"without this, serve mode has no progress output during initial build")
			Expect(spy.ended).To(BeNumerically(">", 0),
				"Build must call EndStage to finalize each stage")
			Expect(spy.summaries).To(HaveLen(1),
				"Build must call Summary exactly once at the end")
			Expect(spy.summaries[0].pageCount).To(Equal(result.PageCount),
				"Summary pageCount must match the build result")
		})

		It("Build with content calls Update for each page in every stage", func() {
			spy := &spyReporter{}

			cfg := &config.Config{
				Title:   "Progress Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
			}
			result, err := pipeline.BuildWithContent(cfg, content, pipeline.BuildOptions{Reporter: spy})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Each per-page stage (Rendering, Layouts, Writing — no Transforms
			// without plugins) must call Update once per page.
			// Stages with total=-1 (Discovering, Copying, Finalizing) are excluded.
			// Total updates = PageCount × perPageStageCount.
			Expect(spy.updates).To(HaveLen(result.PageCount*spy.perPageStageCount()),
				"Build must call Update once per page per stage — "+
					"each Update drives the progress bar forward (issue #506)")
		})

		It("Build reports progress for all pipeline stages (issue #493)", func() {
			spy := &spyReporter{}

			cfg := &config.Config{
				Title:   "All Stages Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"content/about.md":       "---\ntitle: About\nlayout: default\n---\n# About",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			_, err := pipeline.BuildWithContent(cfg, content, pipeline.BuildOptions{Reporter: spy})
			Expect(err).NotTo(HaveOccurred())

			// All pipeline stages must report progress — not just content rendering.
			// Currently only "Rendering" has a progress bar. The 8-12s gap between
			// "Rendering 100%" and build completion has no feedback.
			Expect(spy.stages).To(ContainElement("Rendering"),
				"content rendering must report progress")
			Expect(spy.stages).To(ContainElement("Layouts"),
				"layout rendering must report progress — "+
					"this is Pass 2, currently has no progress bar (issue #493)")
			Expect(spy.stages).To(ContainElement("Writing"),
				"output writing must report progress")
		})

		It("BuildIncremental calls only Summary on the reporter (issue #591)", func() {
			spy := &spyReporter{}

			cfg := &config.Config{
				Title:   "Incremental Progress",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
			}

			// First build populates cache
			previousCache := cache.New()
			for path, body := range contentMap {
				relPath := path[len("content/"):]
				previousCache.SetHash(relPath, cache.HashContent([]byte(body)))
			}

			// Change one page
			contentMap["content/about.md"] = "---\ntitle: About\n---\n# About Updated"
			changedFiles := []string{"content/about.md"}

			result, err := pipeline.BuildIncremental(cfg, contentMap, previousCache, changedFiles,
				pipeline.BuildOptions{Reporter: spy})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(spy.stages).To(BeEmpty(),
				"BuildIncremental must NOT call StartStage — "+
					"incremental rebuilds are fast (1-3 pages, <100ms), "+
					"a multi-stage progress bar would be visual noise")
			Expect(spy.updates).To(BeEmpty(),
				"BuildIncremental must NOT call Update — "+
					"no per-page progress for incremental rebuilds")
			Expect(spy.ended).To(Equal(0),
				"BuildIncremental must NOT call EndStage — "+
					"no stages to end")
			Expect(spy.summaries).To(HaveLen(1),
				"BuildIncremental must call Summary exactly once — "+
					"compact one-line output: 'Rebuilt 3 pages in 47ms (417 cached)'")
			Expect(spy.summaries[0].pagesSkipped).To(Equal(result.PagesSkipped),
				"Summary pagesSkipped must match the incremental result — "+
					"this drives the '(N cached)' display in serve rebuild output")
		})

		It("no reporter does not panic", func() {
			cfg := &config.Config{
				Title:   "No Reporter",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(),
				"Build with nil reporter must succeed without panicking — "+
					"this is the --quiet and piped non-TTY path")
		})
	})

	// ── Layout template caching (issue #585) ────────────────────────
	// parseLayout reads + strips + parses a layout file on every call.
	// RenderContext must cache parsed templates so pages sharing a
	// layout avoid redundant file I/O and template parsing.
})
