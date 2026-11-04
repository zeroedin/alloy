package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Template errors fail the build (issues #1294, #1296) ──────────────
// A named layout that doesn't exist, and a pagination front-matter field
// that fails to parse or render, used to log a warning and let the build
// succeed with incomplete pages. Both now fail the build, in Build and in
// BuildIncremental (the dev-server path), like layout and content render
// errors already do.
//
// Out of scope: a page with no layout named and no default layout. That
// still builds and writes the page without a layout.
//
// See PLAN.md → "Layout Lookup Order" and "Front Matter Interpolation".

var _ = Describe("Template errors fail the build (issues #1294, #1296)", func() {
	cfg := func() *config.Config {
		return &config.Config{
			Title:   "Errors",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
	}

	// buildIncremental writes files to a temp project and builds them the
	// way `alloy dev` does: InitPipelineState, then BuildIncremental.
	buildIncremental := func(files map[string]string) (*pipeline.BuildResult, error) {
		GinkgoHelper()
		tmpDir := GinkgoT().TempDir()
		for name, body := range files {
			path := filepath.Join(tmpDir, name)
			Expect(os.MkdirAll(filepath.Dir(path), 0755)).To(Succeed())
			Expect(os.WriteFile(path, []byte(body), 0644)).To(Succeed())
		}
		c := &config.Config{
			Title:       "Errors Incremental",
			BaseURL:     "https://example.com",
			ProjectRoot: tmpDir,
			Build:       config.BuildConfig{Output: filepath.Join(tmpDir, "_site")},
			Structure:   config.StructureConfig{Content: "content", Layouts: "layouts", Data: "data"},
		}
		config.ApplyDefaults(c)
		registry, hooks, _ := pipeline.DiscoverPlugins(c)
		DeferCleanup(registry.Close)
		state, err := pipeline.InitPipelineState(c, registry, hooks)
		Expect(err).NotTo(HaveOccurred())
		return pipeline.BuildIncremental(c, nil, nil, nil, pipeline.BuildOptions{PipelineState: state})
	}

	// ── #1296: a named layout that doesn't exist ──────────────────────

	Describe("a named layout that doesn't exist (issue #1296)", func() {
		It("fails the build when a _data.yaml cascade names a missing layout", func() {
			result, err := pipeline.BuildWithContent(cfg(), map[string]string{
				"content/docs/_data.yaml": "layout: nosuch",
				"content/docs/a.md":       "---\ntitle: A\n---\nDoc",
				"layouts/default.liquid":  "<html>{{ content }}</html>",
			})
			Expect(err).To(HaveOccurred(),
				"a cascade layout is named just like a front-matter one; a missing file fails the build")
			Expect(result).To(BeNil(), "a failed build must not return a partial result")
			Expect(err.Error()).To(ContainSubstring(`layout "nosuch" not found`))
			Expect(err.Error()).To(ContainSubstring("docs/a.md"))
		})

		It("fails the build when an extension-bearing layout name is missing", func() {
			_, err := pipeline.BuildWithContent(cfg(), map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: missing.html\n---\nAbout",
				"layouts/default.liquid": "<html>{{ content }}</html>",
			})
			Expect(err).To(HaveOccurred(),
				"an extension-bearing name is used as-is; if that file is missing the build fails")
			Expect(err.Error()).To(ContainSubstring(`layout "missing.html" not found`))
			Expect(err.Error()).To(ContainSubstring("about.md"))
		})

		It("fails BuildIncremental too (the dev-server path)", func() {
			_, err := buildIncremental(map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: nonexistent\n---\nAbout",
				"layouts/default.liquid": "<html>{{ content }}</html>",
			})
			Expect(err).To(HaveOccurred(),
				"`alloy dev` rebuilds must fail the same way, so the error overlay shows it")
			Expect(err.Error()).To(ContainSubstring(`layout "nonexistent" not found`))
			Expect(err.Error()).To(ContainSubstring("about.md"))
		})

		It("still builds a page with layout: false (guard)", func() {
			result, err := pipeline.BuildWithContent(cfg(), map[string]string{
				"content/raw.md": "---\ntitle: Raw\nlayout: false\n---\nRaw body",
			})
			Expect(err).NotTo(HaveOccurred(), "layout: false names no layout, so nothing is missing")
			Expect(result.RenderedContent["raw.md"]).To(ContainSubstring("Raw body"))
		})
	})

	// ── #1294: pagination front-matter interpolation errors ───────────

	Describe("a pagination front-matter field that fails (issue #1294)", func() {
		const data = `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"}]`
		page := func(title string) string {
			return "---\ntitle: \"" + title + "\"\nlayout: default\npagination:\n  data: site.data.team\n" +
				"  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>"
		}

		It("fails the build on an unknown filter, naming the page, the field and the Liquid error", func() {
			result, err := pipeline.BuildWithContent(cfg(), map[string]string{
				"data/team.json":         data,
				"content/team.md":        page("{{ member.name | upcse }}"),
				"layouts/default.liquid": "<title>{{ page.title }}</title>{{ content }}",
			})
			Expect(err).To(HaveOccurred(),
				"a field that fails to render must not ship as raw template text")
			Expect(result).To(BeNil(), "a failed build must not return a partial result")
			Expect(err.Error()).To(ContainSubstring(`front matter interpolation for team.md field "title"`))
			Expect(err.Error()).To(ContainSubstring("undefined filter upcse"))
		})

		It("fails the build on a field that doesn't parse", func() {
			_, err := pipeline.BuildWithContent(cfg(), map[string]string{
				"data/team.json":         data,
				"content/team.md":        page("{{ member.name "),
				"layouts/default.liquid": "<title>{{ page.title }}</title>{{ content }}",
			})
			Expect(err).To(HaveOccurred(), "a parse error fails the build like a render error")
			Expect(err.Error()).To(ContainSubstring(`front matter interpolation for team.md field "title"`))
		})

		It("fails BuildIncremental too (the dev-server path)", func() {
			_, err := buildIncremental(map[string]string{
				"data/team.json":         data,
				"content/team.md":        page("{{ member.name | upcse }}"),
				"layouts/default.liquid": "<title>{{ page.title }}</title>{{ content }}",
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(`front matter interpolation for team.md field "title"`))
			Expect(err.Error()).To(ContainSubstring("undefined filter upcse"))
		})
	})
})
