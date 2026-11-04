package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Layout partials in a full build (issue #1297) ──────────────────────
// {% include %} and {% render %} resolve partials from the layouts
// directory only if the build gives the Liquid engine that directory
// (createEngine → NewLiquidEngineWithIncludes). The template package tests
// set it on the engine themselves, so without these tests a build that
// forgot to set it would pass CI while every include printed
// "Liquid error: This liquid context does not allow includes."
//
// See PLAN.md → "Layouts, Partials, Shortcodes" (partials resolve from the
// layouts directory, trying name.liquid, then name.html).

var _ = Describe("Layout partials in a full build (issue #1297)", func() {
	const layout = `<main>{% include "partials/nav" %}|{% render "partials/nav" %}{{ content }}</main>`

	build := func(partialName string) string {
		GinkgoHelper()
		result, err := pipeline.BuildWithContent(&config.Config{
			Title:   "Includes",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}, map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nHi",
			"layouts/default.liquid": layout,
			"layouts/" + partialName: "<nav>N</nav>",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).NotTo(BeNil())
		return result.RenderedContent["index.md"]
	}

	It("renders {% include %} and {% render %} partials from layouts/", func() {
		html := build("partials/nav.liquid")
		Expect(html).To(ContainSubstring("<nav>N</nav>|<nav>N</nav>"),
			"both tags must render the partial from the layouts directory")
		Expect(html).NotTo(ContainSubstring("Liquid error"),
			"the build must give the engine its includes directory")
	})

	It("falls back to a .html partial when there is no .liquid one", func() {
		html := build("partials/nav.html")
		Expect(html).To(ContainSubstring("<nav>N</nav>|<nav>N</nav>"),
			"partials resolve as name.liquid, then name.html")
		Expect(html).NotTo(ContainSubstring("Liquid error"))
	})

	// When both files exist, the .liquid partial wins (issue #1302). The two
	// tests above each have only one of them, so they would pass even if the
	// lookup order were reversed.
	It("prefers the .liquid partial when a .html one also exists", func() {
		result, err := pipeline.BuildWithContent(&config.Config{
			Title:   "Includes",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}, map[string]string{
			"content/index.md":            "---\ntitle: Home\nlayout: default\n---\nHi",
			"layouts/default.liquid":      layout,
			"layouts/partials/nav.liquid": "<nav>LIQUID</nav>",
			"layouts/partials/nav.html":   "<nav>HTML</nav>",
		})
		Expect(err).NotTo(HaveOccurred())
		html := result.RenderedContent["index.md"]
		Expect(html).To(ContainSubstring("<nav>LIQUID</nav>|<nav>LIQUID</nav>"),
			"{% include %} and {% render %} must both use name.liquid before name.html")
		Expect(html).NotTo(ContainSubstring("<nav>HTML</nav>"),
			"the .html partial is only a fallback for when there is no .liquid one")
	})

	// The dev server builds through InitPipelineState + BuildIncremental,
	// which is a separate entry point from Build.
	It("renders partials through BuildIncremental with the pipeline state", func() {
		tmpDir := GinkgoT().TempDir()
		files := map[string]string{
			"content/index.md":            "---\ntitle: Home\nlayout: default\n---\nHi",
			"layouts/default.liquid":      layout,
			"layouts/partials/nav.liquid": "<nav>N</nav>",
		}
		for name, body := range files {
			path := filepath.Join(tmpDir, name)
			Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
			Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
		}

		cfg := &config.Config{
			Title:       "Includes Incremental",
			BaseURL:     "https://example.com",
			ProjectRoot: tmpDir,
			Build:       config.BuildConfig{Output: filepath.Join(tmpDir, "_site")},
			Structure: config.StructureConfig{
				Content: "content",
				Layouts: "layouts",
			},
		}
		config.ApplyDefaults(cfg)
		registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
		defer registry.Close()
		state, err := pipeline.InitPipelineState(cfg, registry, hooks)
		Expect(err).NotTo(HaveOccurred())

		result, err := pipeline.BuildIncremental(cfg, nil, nil, nil, pipeline.BuildOptions{
			PipelineState:          state,
			CaptureRenderedContent: true,
		})
		Expect(err).NotTo(HaveOccurred())
		html := result.RenderedContent["index.md"]
		Expect(html).To(ContainSubstring("<nav>N</nav>|<nav>N</nav>"),
			"the incremental entry point must give the engine its includes directory too")
		Expect(html).NotTo(ContainSubstring("Liquid error"))
	})
})
