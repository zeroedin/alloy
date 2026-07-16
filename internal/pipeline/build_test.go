package pipeline_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// spyReporter records all ProgressReporter calls for test assertions.
type spyReporter struct {
	stages       []string
	stageTotals  []int
	messages     []string
	updates      []spyUpdate
	ended        int
	summaries    []spySummary
}

type spyUpdate struct {
	current  int
	filePath string
	elapsed  time.Duration
}

type spySummary struct {
	pageCount    int
	duration     time.Duration
	pagesSkipped int
}

func (s *spyReporter) StartStage(name string, total int) {
	s.stages = append(s.stages, name)
	s.stageTotals = append(s.stageTotals, total)
}
func (s *spyReporter) Message(text string) { s.messages = append(s.messages, text) }
func (s *spyReporter) Update(current int, filePath string, elapsed time.Duration) {
	s.updates = append(s.updates, spyUpdate{current: current, filePath: filePath, elapsed: elapsed})
}
func (s *spyReporter) EndStage() { s.ended++ }
func (s *spyReporter) Summary(pageCount int, duration time.Duration, pagesSkipped int) {
	s.summaries = append(s.summaries, spySummary{pageCount: pageCount, duration: duration, pagesSkipped: pagesSkipped})
}

// perPageStageCount returns the number of stages that report per-page
// updates (total >= 0). Stages with total=-1 (Discovering, Copying,
// Finalizing) do not have per-page granularity.
func (s *spyReporter) perPageStageCount() int {
	count := 0
	for _, t := range s.stageTotals {
		if t >= 0 {
			count++
		}
	}
	return count
}

// Spec reference: PLAN.md §2 — Build Pipeline
// Tests are immutable. They encode the specification.
// If implementation cannot satisfy a test, the spec must be reviewed first.

var _ = Describe("Build Pipeline", func() {

	Describe("Phase ordering", func() {
		It("completes a build and returns a result", func() {
			cfg := &config.Config{
				Title:   "Test Site",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
		})

		It("skips Phase 2 entirely when no SSR configured", func() {
			cfg := &config.Config{
				Title:   "Test Site",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				// SSR is nil — no SSR configured
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"Phase 2 must be skipped when no ssr: config is present")
		})
	})

	Describe("Error behavior", func() {
		It("produces no output when a page fails to render", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Inject content with a broken template filter to force a render error
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"content/broken.md": "---\ntitle: Broken\n---\n{{ undefined | nonexistent_filter }}",
			})
			Expect(err).To(HaveOccurred(),
				"build must fail when any page has a render error")
			Expect(err.Error()).To(ContainSubstring("broken"),
				"error must reference the failing source file")
			Expect(result).To(BeNil(),
				"failed build must not return partial result — no partial deploys")
		})

		It("includes stage information in error messages", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Force a render error with broken template syntax
			_, err := pipeline.BuildWithContent(cfg, map[string]string{
				"content/bad.md": "---\ntitle: Bad\n---\n{{ | broken_syntax }}",
			})
			Expect(err).To(HaveOccurred(),
				"broken template must cause a build error")
			Expect(err.Error()).To(ContainSubstring("template rendering"),
				"error must identify the pipeline stage where the failure occurred")
		})
	})

	// ── Data directory error handling (issue #982) ────────────────────
	// Data directory load errors (stem collisions, malformed files) must
	// be fatal build errors — not warnings that silently drop ALL site
	// data. The loadSiteData function must propagate data.LoadDirectory
	// errors instead of swallowing them. This matches the external data
	// files path (already fatal) and the project's fail-fast philosophy.
	Describe("Data directory error handling (issue #982)", func() {

		It("stem collision in data directory fails the build", func() {
			cfg := &config.Config{
				Title: "Stem Collision Test",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Two data files sharing the stem "team" — team.yaml and team.json
			// both claim the key "team" in site.data.
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"data/team.yaml": "name: Alice\nrole: Lead\n",
				"data/team.json": `{"name": "Bob", "role": "Dev"}`,
			})
			Expect(err).To(HaveOccurred(),
				"stem collision in data directory must cause a fatal build error — "+
					"currently loadSiteData swallows the error from data.LoadDirectory "+
					"and silently drops ALL site data (issue #982)")
			Expect(err.Error()).To(SatisfyAll(
				ContainSubstring("team"),
				ContainSubstring("conflict"),
			), "error must name the conflicting stem and mention 'conflict' — "+
				"the user needs to know which files collided to fix the problem")
			Expect(result).To(BeNil(),
				"failed build must not return partial result")
		})

		It("malformed YAML in data directory fails the build", func() {
			cfg := &config.Config{
				Title: "Malformed YAML Test",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Invalid YAML: unterminated flow sequence
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"data/broken.yaml": "invalid: [yaml: {unterminated",
			})
			Expect(err).To(HaveOccurred(),
				"malformed YAML in data directory must cause a fatal build error — "+
					"currently loadSiteData logs a warning and drops the entire "+
					"data directory contents (issue #982)")
			Expect(err.Error()).To(ContainSubstring("broken.yaml"),
				"error must name the file that failed to parse so the user "+
					"can locate and fix the problem")
			Expect(result).To(BeNil(),
				"failed build must not return partial result")
		})

		It("malformed JSON in data directory fails the build", func() {
			cfg := &config.Config{
				Title: "Malformed JSON Test",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Invalid JSON: unterminated object
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"data/broken.json": `{"invalid json`,
			})
			Expect(err).To(HaveOccurred(),
				"malformed JSON in data directory must cause a fatal build error — "+
					"currently loadSiteData logs a warning and drops the entire "+
					"data directory contents (issue #982)")
			Expect(err.Error()).To(ContainSubstring("broken.json"),
				"error must name the file that failed to parse so the user "+
					"can locate and fix the problem")
			Expect(result).To(BeNil(),
				"failed build must not return partial result")
		})

		It("non-existent data directory is not an error", func() {
			cfg := &config.Config{
				Title:     "No Data Dir Test",
				Build:     config.BuildConfig{Output: "_site"},
				Structure: config.StructureConfig{Data: "nonexistent-data-dir"},
			}
			// A project with no data/ directory is valid — many projects
			// don't use data files. This must not error.
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"content/index.md": "---\ntitle: Home\n---\nhello",
			})
			Expect(err).NotTo(HaveOccurred(),
				"non-existent data directory must not cause an error — "+
					"not every project uses data files")
			Expect(result).NotTo(BeNil())
		})

		It("valid data files load and are accessible in templates", func() {
			cfg := &config.Config{
				Title: "Valid Data Test",
				Build: config.BuildConfig{Output: "_site"},
			}
			// Valid YAML data file alongside a content page — the layout
			// renders site.data.navigation to prove the data reached templates.
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"data/navigation.yaml":   "items:\n  - name: Home\n    url: /\n",
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nhello",
				"layouts/default.liquid": "NAV:{{ site.data.navigation.items[0].name }}|{{ content }}",
			})
			Expect(err).NotTo(HaveOccurred(),
				"valid data files must load without error")
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent["index.md"]).To(ContainSubstring("NAV:Home"),
				"site.data.navigation.items[0].name must resolve to 'Home' — "+
					"if this fails, data loaded successfully but was not injected "+
					"into the template context (issue #982 core behavior)")
		})

		It("healthy data files are not dropped when build aborts on error", func() {
			cfg := &config.Config{
				Title: "Innocent Bystander Test",
				Build: config.BuildConfig{Output: "_site"},
			}
			// navigation.yaml is valid, but team.yaml + team.json collide.
			// The bug in issue #982: the warning path drops ALL data files,
			// not just the conflicting ones. After the fix, the build must
			// abort entirely — no partial data loading.
			result, err := pipeline.BuildWithContent(cfg, map[string]string{
				"data/navigation.yaml": "items:\n  - name: Home\n    url: /\n",
				"data/team.yaml":       "name: Alice\n",
				"data/team.json":       `{"name": "Bob"}`,
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\nhello",
				"layouts/default.liquid": "NAV:{{ site.data.navigation.items[0].name }}|{{ content }}",
			})
			Expect(err).To(HaveOccurred(),
				"build must abort when data directory has a stem collision — "+
					"the current bug silently drops ALL data (including valid "+
					"navigation.yaml) behind a success exit code (issue #982)")
			Expect(result).To(BeNil(),
				"failed build must not return partial result — no partial deploys")
		})
	})

	Describe("Build result", func() {
		It("returns output directory path", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.OutputDir).To(Equal("_site"))
		})

		It("returns page count matching pages rendered", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(BeNumerically(">", 0),
				"build must render at least one page from content/")
		})

		It("returns build duration", func() {
			cfg := &config.Config{
				Title: "Test Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Duration).To(BeNumerically(">", 0))
		})

		It("returns zero page count when content directory is empty", func() {
			cfg := &config.Config{
				Title: "Empty Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.BuildWithContent(cfg, map[string]string{})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(0))
		})
	})

	// ── Unified pipeline: single-language as degenerate i18n (issue #280) ──
	// A site without languages: config must produce identical results
	// whether processed via a single batch or the old single-language path.
	// This proves the pipeline has ONE code path, not two forks.

	Describe("Unified pipeline", func() {
		It("single-language build produces same result with or without explicit language config", func() {
			content := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
			}

			// Build without languages: config
			cfgNoLang := &config.Config{
				Title:   "No Lang",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			resultNoLang, err := pipeline.BuildWithContent(cfgNoLang, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(resultNoLang).NotTo(BeNil())

			// Build with explicit single language via Languages map
			cfgWithLang := &config.Config{
				Title:   "With Lang",
				BaseURL: "https://example.com",
				Languages: map[string]*config.LanguageConfig{
					"en": {Root: true},
				},
				Build: config.BuildConfig{Output: "_site"},
			}
			resultWithLang, err := pipeline.BuildWithContent(cfgWithLang, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(resultWithLang).NotTo(BeNil())

			Expect(resultWithLang.PageCount).To(Equal(resultNoLang.PageCount),
				"single-language build must produce the same page count "+
					"regardless of whether languages: is set — proves one code path")
		})
	})

	// ── BuildWithContent delegates to Build (issue #283) ────────────
	// BuildWithContent must be a thin wrapper that delegates to Build().
	// It must not duplicate any pipeline logic. Tests verify that
	// pipeline stages that were previously missing now run.

	Describe("BuildWithContent delegates to Build", func() {
		It("forwards BuildOptions to Build", func() {
			cfg := &config.Config{
				Title:   "Delegate Test",
				BaseURL: "https://example.com",
				SSR:     &config.SSRConfig{Command: "cat"},
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
			}
			result, err := pipeline.BuildWithContent(cfg, content, pipeline.BuildOptions{SkipSSR: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"BuildWithContent must forward BuildOptions to Build — "+
					"SkipSSR=true must skip Phase 2")
		})

		It("runs lifecycle filtering through Build", func() {
			cfg := &config.Config{
				Title:         "Lifecycle Test",
				BaseURL:       "https://example.com",
				Build:         config.BuildConfig{Output: "_site"},
				IncludeDrafts: false,
			}
			content := map[string]string{
				"content/published.md": "---\ntitle: Published\n---\n# Published",
				"content/draft.md":     "---\ntitle: Draft\ndraft: true\n---\n# Draft",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"BuildWithContent must run lifecycle filtering via Build — "+
					"draft pages must be excluded when IncludeDrafts is false")
		})

		It("renders through layout chain via Build", func() {
			cfg := &config.Config{
				Title:   "Layout Chain Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/page.md":        "---\ntitle: Test\nlayout: child\n---\n# Hello",
				"layouts/child.liquid":   "---\nlayout: \"base\"\n---\n<main>{{ content }}</main>",
				"layouts/base.liquid":    "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["page.md"]
			Expect(html).To(ContainSubstring("<html>"),
				"BuildWithContent must render through layout chain via Build — "+
					"root layout wrapper must be present")
			Expect(html).To(ContainSubstring("<main>"),
				"middle layout wrapper must be present")
			Expect(html).NotTo(ContainSubstring("layout:"),
				"layout front matter must be stripped")
		})
	})

	// ── Build is always full rebuild (§2, issue #221) ───────────────
	// alloy build always renders all pages — no incremental skipping.
	// It is intended for CI/CD where a clean, complete output is required.
	// Incremental rebuilds (cache-based skipping) are only for alloy serve
	// (dev mode file watcher). The cache is written for dev mode's use
	// but alloy build does not read it.

	Describe("Build always renders all pages", func() {
		It("consecutive builds with same content render all pages each time", func() {
			cfg := &config.Config{
				Title:   "Build Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
			}

			// First build
			result1, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.PageCount).To(Equal(2),
				"first build must render all pages")

			// Second build — same content, must still render all pages
			result2, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.PageCount).To(Equal(2),
				"alloy build must always render all pages — no incremental skipping. "+
					"Incremental rebuilds are for alloy serve only.")
		})
	})

	// ── Early validation: conflict detection before rendering (issue #690) ──
	// Validation (permalink/alias conflicts) must run after onPagesReady but
	// before content rendering. If a conflict is detected, the build fails
	// immediately with no rendering work performed.

	Describe("Early validation before rendering (issue #690)", func() {
		It("permalink conflict errors before any rendering occurs (issue #690)", func() {
			cfg := &config.Config{
				Title:   "Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/page-a.md":     "---\ntitle: Page A\nlayout: default\npermalink: /about/\n---\n# Page A content",
				"content/page-b.md":     "---\ntitle: Page B\nlayout: default\npermalink: /about/\n---\n# Page B content",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"two pages claiming the same permalink must produce a build error — "+
					"validation detects this from page.URL alone, no rendering needed")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil(),
				"BuildResult must be nil when a permalink conflict is detected — "+
					"validation runs before content rendering so no rendering work "+
					"is performed and no result is produced (issue #690)")
		})

		It("alias conflict with another page's permalink errors before rendering (issue #690)", func() {
			cfg := &config.Config{
				Title:   "Alias Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/page-a.md":     "---\ntitle: Page A\nlayout: default\npermalink: /guide/\n---\n# Page A",
				"content/page-b.md":     "---\ntitle: Page B\nlayout: default\npermalink: /tutorial/\naliases:\n  - /guide/\n---\n# Page B",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"an alias that collides with another page's permalink must produce "+
					"a build error — aliases are known after permalink resolution, "+
					"before rendering begins")
			Expect(result).To(BeNil(),
				"BuildResult must be nil when an alias conflict is detected — "+
					"no rendering work is performed (issue #690)")
		})

		It("non-conflicting pages build successfully (issue #690)", func() {
			cfg := &config.Config{
				Title:   "No Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/page-a.md":     "---\ntitle: Page A\nlayout: default\npermalink: /about/\n---\n# About",
				"content/page-b.md":     "---\ntitle: Page B\nlayout: default\npermalink: /contact/\n---\n# Contact",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"pages with distinct permalinks must not trigger a conflict error — "+
					"early validation must pass and rendering must proceed normally")
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent).To(HaveLen(2),
				"both pages must be rendered when no conflicts exist — "+
					"early validation (issue #690) must not block valid builds")
		})

		It("validation timing is recorded before rendering timing with profiling (issue #690)", func() {
			cfg := &config.Config{
				Title:   "Profile Timing Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/index.md":      "---\ntitle: Home\nlayout: default\n---\n# Home",
			}
			result, err := pipeline.BuildWithContent(cfg, files, pipeline.BuildOptions{Profile: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.StageTimings).NotTo(BeEmpty(),
				"profiling must produce stage timings")

			var validationIdx, renderIdx int
			validationFound, renderFound := false, false
			for i, t := range result.StageTimings {
				if strings.Contains(t.Name, "Validation") {
					validationIdx = i
					validationFound = true
				}
				if strings.Contains(t.Name, "content render") || strings.Contains(t.Name, "Rendering") {
					if !renderFound {
						renderIdx = i
						renderFound = true
					}
				}
			}
			Expect(validationFound).To(BeTrue(),
				"stage timings must include a Validation entry")
			Expect(renderFound).To(BeTrue(),
				"stage timings must include a content render entry")
			Expect(validationIdx).To(BeNumerically("<", renderIdx),
				"Validation stage must appear before content rendering in stage timings — "+
					"conflict detection runs after permalink resolution but before any "+
					"markdown or template rendering begins (issue #690)")
		})
	})

	// ── Nested cascade permalink resolution (issue #910) ──────────────
	// Permalink patterns in _data.yaml cascade to subdirectories. A nested
	// _data.yaml can override the parent's permalink pattern — typically to
	// add dynamic tokens (like :year/:month) that the file system can't
	// express. The pipeline must resolve permalinks using per-page cascade
	// data (the nearest _data.yaml in the directory tree), not a flat
	// section-name-only map.

	Describe("Nested cascade permalink resolution (issue #910)", func() {
		It("nested _data.yaml permalink adds date tokens that parent does not have", func() {
			cfg := &config.Config{
				Title:   "Nested Cascade Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// blog/ uses simple slugs for static pages (about, contact).
			// blog/posts/ adds date-based URLs — the file system can't express
			// :year/:month, so a nested _data.yaml provides them.
			files := map[string]string{
				"layouts/default.liquid":             "URL:{{ page.url }}",
				"content/blog/_data.yaml":            "permalink: \"/blog/:slug/\"",
				"content/blog/posts/_data.yaml":      "permalink: \"/blog/:year/:month/:slug/\"",
				"content/blog/about.md":              "---\ntitle: About\ndate: 2026-04-10\nlayout: default\n---\n# About",
				"content/blog/posts/first-post.md":   "---\ntitle: First Post\ndate: 2026-04-10\nlayout: default\n---\n# First",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Guard: top-level section permalink still works
			aboutHTML := result.RenderedContent["blog/about.md"]
			Expect(aboutHTML).To(ContainSubstring("URL:/blog/about/"),
				"page in top-level section must use blog/_data.yaml permalink pattern — "+
					"/blog/:slug/ → /blog/about/")

			// Core assertion: nested _data.yaml permalink overrides parent
			postHTML := result.RenderedContent["blog/posts/first-post.md"]
			Expect(postHTML).To(ContainSubstring("URL:/blog/2026/04/first-post/"),
				"page in nested directory must use the nearest _data.yaml permalink pattern — "+
					"content/blog/posts/_data.yaml has permalink: /blog/:year/:month/:slug/ "+
					"which must resolve to /blog/2026/04/first-post/. The file system can't "+
					"express date-based URLs, so this nested _data.yaml is the only way to "+
					"add :year/:month tokens for a subdirectory. "+
					"This fails when the pipeline uses a flat section-name-only map "+
					"(buildPermalinkCfg) instead of per-page cascade data (issue #910)")
		})

		It("nested cascade inherits parent permalink when no local override", func() {
			cfg := &config.Config{
				Title:   "Cascade Inherit Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// blog/ sets a date-based permalink. blog/posts/ has a _data.yaml
			// for layout but no permalink key — must inherit the parent's pattern.
			files := map[string]string{
				"layouts/default.liquid":             "URL:{{ page.url }}",
				"content/blog/_data.yaml":            "permalink: \"/blog/:year/:slug/\"",
				"content/blog/posts/_data.yaml":      "layout: default",
				"content/blog/posts/inherited.md":    "---\ntitle: Inherited\ndate: 2026-06-15\nlayout: default\n---\n# Inherited",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["blog/posts/inherited.md"]
			Expect(html).To(ContainSubstring("URL:/blog/2026/inherited/"),
				"when nested _data.yaml has no permalink key, the parent's permalink pattern "+
					"must cascade down — /blog/:year/:slug/ → /blog/2026/inherited/. "+
					"LoadDirectoryCascade merges parent data into child entries, so the "+
					"parent's permalink key is available in the child's cascade data")
		})

		It("front matter permalink overrides nested cascade permalink", func() {
			cfg := &config.Config{
				Title:   "FM Override Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid":             "URL:{{ page.url }}",
				"content/blog/posts/_data.yaml":      "permalink: \"/blog/:year/:month/:slug/\"",
				"content/blog/posts/custom.md":       "---\ntitle: Custom\ndate: 2026-04-10\nlayout: default\npermalink: /my-custom-path/\n---\n# Custom",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["blog/posts/custom.md"]
			Expect(html).To(ContainSubstring("URL:/my-custom-path/"),
				"front matter permalink must override nested cascade permalink — "+
					"front matter always wins regardless of cascade depth")
		})

		It("deeply nested cascade uses most-specific _data.yaml permalink", func() {
			cfg := &config.Config{
				Title:   "Deep Cascade Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			// Three levels: blog/ uses simple slugs, blog/posts/ adds dates,
			// blog/posts/featured/ uses a curated URL scheme.
			files := map[string]string{
				"layouts/default.liquid":                          "URL:{{ page.url }}",
				"content/blog/_data.yaml":                         "permalink: \"/blog/:slug/\"",
				"content/blog/posts/_data.yaml":                   "permalink: \"/blog/:year/:month/:slug/\"",
				"content/blog/posts/featured/_data.yaml":          "permalink: \"/blog/featured/:slug/\"",
				"content/blog/posts/featured/highlight.md":        "---\ntitle: Highlight\ndate: 2026-04-10\nlayout: default\n---\n# Highlight",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["blog/posts/featured/highlight.md"]
			Expect(html).To(ContainSubstring("URL:/blog/featured/highlight/"),
				"page must use the most-specific (deepest) _data.yaml permalink pattern — "+
					"content/blog/posts/featured/_data.yaml overrides both parent levels. "+
					"This proves the pipeline resolves per-page cascade data, not just "+
					"top-level section patterns")
		})

		It("index file in nested directory skips cascade permalink", func() {
			cfg := &config.Config{
				Title:   "Index Skip Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid":             "URL:{{ page.url }}",
				"content/blog/posts/_data.yaml":      "permalink: \"/blog/:year/:month/:slug/\"",
				"content/blog/posts/index.md":        "---\ntitle: All Posts\nlayout: default\n---\n# All Posts",
				"content/blog/posts/first-post.md":   "---\ntitle: First Post\ndate: 2026-04-10\nlayout: default\n---\n# First",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Index files must skip cascade permalink — resolve to directory path
			indexHTML := result.RenderedContent["blog/posts/index.md"]
			Expect(indexHTML).To(ContainSubstring("URL:/blog/posts/"),
				"index file must resolve to its directory path (/blog/posts/) — "+
					"cascade permalink patterns must not apply to index files, "+
					"even in nested directories (issue #39)")

			// Non-index file in same directory must use cascade permalink
			postHTML := result.RenderedContent["blog/posts/first-post.md"]
			Expect(postHTML).To(ContainSubstring("URL:/blog/2026/04/first-post/"),
				"non-index page in the same directory must use the cascade permalink")
		})
	})

	// ── Multi-language cascade permalink resolution (issue #914) ─────
	// In multi-language builds, cascade data (from _data.yaml) is loaded from
	// the full content tree: content/en/blog/_data.yaml, content/es/blog/_data.yaml.
	// Cascade keys include the language prefix (e.g., "content/es/blog/").
	//
	// The pipeline strips the language prefix from page.RelPath before permalink
	// resolution to avoid URL doubling (/es/es/about/). However, the cascade
	// lookup (FindCascadeData) must use the ORIGINAL (un-stripped) RelPath so it
	// finds language-specific _data.yaml entries. The stripped RelPath should
	// only be used for ResolveFromCascade (token resolution + DefaultFromPath).
	//
	// Without this fix, FindCascadeData receives "blog/my-post.md" and looks for
	// "content/blog/" — but the data is keyed as "content/es/blog/". Language-
	// specific cascade permalink patterns are never found, and pages fall back
	// to DefaultFromPath.

	Describe("Multi-language cascade permalink resolution (issue #914)", func() {
		It("applies language-specific _data.yaml permalink to pages in that language", func() {
			cfg := &config.Config{
				Title:   "i18n Cascade Test",
				BaseURL: "https://example.com",
				Languages: map[string]*config.LanguageConfig{
					"en": {Root: true, Weight: 1},
					"es": {Weight: 2},
				},
				Build: config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid":         "URL:{{ page.url }}",
				"content/es/blog/_data.yaml":     "permalink: \"/:slug/\"",
				"content/es/blog/hola.md":        "---\ntitle: Hola\ndate: 2026-04-10\nlayout: default\n---\n# Hola",
				"content/en/blog/hello.md":       "---\ntitle: Hello\ndate: 2026-04-10\nlayout: default\n---\n# Hello",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			esHTML := result.RenderedContent["es/blog/hola.md"]
			Expect(esHTML).To(ContainSubstring("URL:/es/hola/"),
				"language-specific _data.yaml permalink must apply — "+
					"content/es/blog/_data.yaml has permalink: /:slug/ which should "+
					"resolve to /es/hola/ (cascade pattern + language output prefix). "+
					"This fails when FindCascadeData receives the stripped RelPath "+
					"(blog/hola.md) instead of the original (es/blog/hola.md), "+
					"causing it to look for content/blog/ instead of content/es/blog/")

			// Guard: English page without cascade data falls back to DefaultFromPath
			enHTML := result.RenderedContent["en/blog/hello.md"]
			Expect(enHTML).To(ContainSubstring("URL:/blog/hello/"),
				"English page (root language, no cascade permalink) must use "+
					"DefaultFromPath — /blog/hello/ without language prefix since "+
					"en has root: true")
		})

		It("applies root language cascade permalink without URL prefix", func() {
			cfg := &config.Config{
				Title:   "Root Lang Cascade Test",
				BaseURL: "https://example.com",
				Languages: map[string]*config.LanguageConfig{
					"en": {Root: true, Weight: 1},
					"es": {Weight: 2},
				},
				Build: config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid":         "URL:{{ page.url }}",
				"content/en/blog/_data.yaml":     "permalink: \"/articles/:slug/\"",
				"content/en/blog/hello.md":       "---\ntitle: Hello\ndate: 2026-04-10\nlayout: default\n---\n# Hello",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["en/blog/hello.md"]
			Expect(html).To(ContainSubstring("URL:/articles/hello/"),
				"root language page must use its language-specific cascade permalink "+
					"without URL prefix — content/en/blog/_data.yaml has "+
					"permalink: /articles/:slug/ → /articles/hello/. "+
					"Root language (root: true) outputs at site root, not /en/. "+
					"This fails when FindCascadeData uses the stripped RelPath "+
					"(blog/hello.md → content/blog/) instead of the original "+
					"(en/blog/hello.md → content/en/blog/)")
		})

		It("falls back to DefaultFromPath when language has no cascade permalink", func() {
			cfg := &config.Config{
				Title:   "Fallback Test",
				BaseURL: "https://example.com",
				Languages: map[string]*config.LanguageConfig{
					"en": {Root: true, Weight: 1},
					"fr": {Weight: 2},
				},
				Build: config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid":         "URL:{{ page.url }}",
				"content/fr/blog/bonjour.md":     "---\ntitle: Bonjour\ndate: 2026-04-10\nlayout: default\n---\n# Bonjour",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["fr/blog/bonjour.md"]
			Expect(html).To(ContainSubstring("URL:/fr/blog/bonjour/"),
				"when no _data.yaml exists in the language content tree, permalink "+
					"resolution must fall back to DefaultFromPath — "+
					"blog/bonjour.md → /blog/bonjour/ with /fr/ language prefix → "+
					"/fr/blog/bonjour/")
		})

		It("each language resolves from its own cascade data independently", func() {
			cfg := &config.Config{
				Title:   "Independent Cascade Test",
				BaseURL: "https://example.com",
				Languages: map[string]*config.LanguageConfig{
					"en": {Root: true, Weight: 1},
					"es": {Weight: 2},
				},
				Build: config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"layouts/default.liquid":         "URL:{{ page.url }}",
				"content/en/blog/_data.yaml":     "permalink: \"/posts/:slug/\"",
				"content/es/blog/_data.yaml":     "permalink: \"/:slug/\"",
				"content/en/blog/hello.md":       "---\ntitle: Hello\ndate: 2026-04-10\nlayout: default\n---\n# Hello",
				"content/es/blog/hola.md":        "---\ntitle: Hola\ndate: 2026-04-10\nlayout: default\n---\n# Hola",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			enHTML := result.RenderedContent["en/blog/hello.md"]
			Expect(enHTML).To(ContainSubstring("URL:/posts/hello/"),
				"English page must use content/en/blog/_data.yaml permalink — "+
					"/posts/:slug/ → /posts/hello/ (root language, no prefix). "+
					"Each language has independent cascade data")

			esHTML := result.RenderedContent["es/blog/hola.md"]
			Expect(esHTML).To(ContainSubstring("URL:/es/hola/"),
				"Spanish page must use content/es/blog/_data.yaml permalink — "+
					"/:slug/ → /hola/ with /es/ prefix → /es/hola/. "+
					"Languages must not share cascade data — if both used the same "+
					"pattern, this test would detect the cross-contamination")
		})
	})

	// ── Subdirectory data files accessible in templates (issue #983) ──
	// Data files in subdirectories of data/ must be accessible in templates
	// as nested namespaces: data/nav/main.yaml → site.data.nav.main

	Describe("Subdirectory data files in templates (issue #983)", func() {
		It("templates access nested data directory files via site.data.subdir.key", func() {
			cfg := &config.Config{
				Title:   "Nested Data Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"data/nav/main.yaml":    "items:\n  - label: Home\n    url: /\n  - label: About\n    url: /about/",
				"data/nav/footer.yaml":  "links:\n  - label: Privacy\n    url: /privacy/",
				"data/settings.yaml":    "site_name: Test Site",
				"layouts/default.liquid": "<html><body>NAV:{{ site.data.nav.main.items[0].label }}|FOOTER:{{ site.data.nav.footer.links[0].label }}|SITE:{{ site.data.settings.site_name }}</body></html>",
				"content/index.md":      "---\ntitle: Home\nlayout: default\n---\n# Home",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"build with nested data subdirectories must succeed — "+
					"LoadDirectory must recurse into data/nav/ (issue #983)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("NAV:Home"),
				"site.data.nav.main.items[0].label must resolve to 'Home' — "+
					"data/nav/main.yaml creates the nested namespace site.data.nav.main "+
					"(issue #983). If this fails, subdirectory data files are not "+
					"being loaded into nested namespaces")
			Expect(html).To(ContainSubstring("FOOTER:Privacy"),
				"site.data.nav.footer.links[0].label must resolve to 'Privacy' — "+
					"multiple files in the same subdirectory must coexist under "+
					"the same parent namespace")
			Expect(html).To(ContainSubstring("SITE:Test Site"),
				"root-level data files must still work alongside nested subdirectory data")
		})

		It("deeply nested data directories are accessible in templates", func() {
			cfg := &config.Config{
				Title:   "Deep Nested Data Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"data/api/v2/endpoints.yaml": "users:\n  path: /api/v2/users\n  method: GET",
				"layouts/default.liquid":     "<html><body>PATH:{{ site.data.api.v2.endpoints.users.path }}</body></html>",
				"content/index.md":           "---\ntitle: Home\nlayout: default\n---\n# Home",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"build with deeply nested data directories must succeed")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("PATH:/api/v2/users"),
				"site.data.api.v2.endpoints.users.path must resolve through "+
					"three levels of directory nesting — data/api/v2/endpoints.yaml "+
					"→ site.data.api.v2.endpoints (issue #983)")
		})

		It("directory-file stem collision in data/ is a build error", func() {
			cfg := &config.Config{
				Title:   "Dir-File Collision Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			files := map[string]string{
				"data/nav.yaml":         "items:\n  - Home",
				"data/nav/main.yaml":    "items:\n  - label: Home\n    url: /",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/index.md":      "---\ntitle: Home\nlayout: default\n---\n# Home",
			}
			_, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"data/nav.yaml and data/nav/ both claim the key \"nav\" — "+
					"the build must fail with a collision error, same semantics "+
					"as two files sharing a stem (issue #983)")
			Expect(err.Error()).To(ContainSubstring("nav"),
				"error must name the colliding key")
		})
	})

	// ── Plugin source dispatch (issue #979) ─────────────────────────
	// type: "plugin" in cfg.Sources must dispatch to fetch.FetchPluginSource,
	// merge the result into siteData, and make it available in templates as
	// site.data.<as>. This is the end-to-end integration test proving the
	// pipeline dispatch is wired — unit tests in fetch_test.go cover the
	// handler invocation mechanics.

	Describe("Plugin source dispatch (issue #979)", func() {
		BeforeEach(func() {
			fetch.ResetPluginSources()
		})
		AfterEach(func() {
			fetch.ResetPluginSources()
		})

		It("Build dispatches type: plugin source to registered handler and merges into site.data", func() {
			// Register a Go-level plugin source handler that returns blog post data
			fetch.RegisterPluginSource("cms-posts", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{
					map[string]interface{}{"title": "Hello World", "slug": "hello-world"},
					map[string]interface{}{"title": "Second Post", "slug": "second-post"},
				}, nil
			})

			cfg := &config.Config{
				Title:   "Plugin Source Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"blog": {
						Type:   "plugin",
						Plugin: "cms-posts",
						Cache:  3600,
						As:     "blog",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Posts",
				"layouts/default.liquid": "<html><body>{{ content }}<p>Posts: {{ site.data.blog.size }}</p></body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Posts: 2"),
				"plugin source data must be merged into site.data under the 'as' key — "+
					"template must access site.data.blog.size and get 2 posts. "+
					"This proves the case \"plugin\" dispatch in the source-fetch loop "+
					"is wired into the pipeline (issue #979)")
		})

		It("Build uses source name as data key when 'as' is omitted", func() {
			// Use distinct source key ("calendar") and plugin name ("events")
			// to verify the default key comes from the source map key, not Plugin.
			fetch.RegisterPluginSource("events", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{
					map[string]interface{}{"name": "Conference", "date": "2026-09-01"},
				}, nil
			})

			cfg := &config.Config{
				Title:   "Plugin Source Default Key Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"calendar": {
						Type:   "plugin",
						Plugin: "events",
						// As is omitted — should default to source name "calendar"
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Events\nlayout: default\n---\n# Events",
				"layouts/default.liquid": "<html><body>{{ content }}<p>Count: {{ site.data.calendar.size }}</p></body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Count: 1"),
				"when 'as' is omitted, source name (map key 'calendar') must be used as the data key, "+
					"not the plugin name ('events') — template must access site.data.calendar.size and get 1 event")
		})

		It("Build aborts when plugin source handler is not registered", func() {
			cfg := &config.Config{
				Title:   "Missing Source Handler Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"blog": {
						Type:   "plugin",
						Plugin: "nonexistent-handler",
						As:     "blog",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"build must abort when plugin source handler is not registered — "+
					"PLAN.md says source failures abort the build, not warn and continue")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("nonexistent-handler"),
				ContainSubstring("not registered"),
				ContainSubstring("blog"),
			), "error must identify the missing source handler or the source name")
		})

		It("Build aborts when plugin source handler returns an error", func() {
			fetch.RegisterPluginSource("broken-api", func(config map[string]interface{}) (interface{}, error) {
				return nil, fmt.Errorf("connection refused: CMS API at api.example.com:443")
			})

			cfg := &config.Config{
				Title:   "Source Error Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"posts": {
						Type:   "plugin",
						Plugin: "broken-api",
						As:     "posts",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"build must abort when a plugin source handler returns an error — "+
					"PLAN.md §5: 'External data source unreachable aborts the build'")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("connection refused"),
				ContainSubstring("broken-api"),
				ContainSubstring("posts"),
			), "error must contain the handler's error message or the source name")
		})

		It("plugin source data is available in result.SiteData", func() {
			fetch.RegisterPluginSource("products", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{
					map[string]interface{}{"name": "Widget", "price": float64(9.99)},
				}, nil
			})

			cfg := &config.Config{
				Title:   "SiteData Result Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"catalog": {
						Type:   "plugin",
						Plugin: "products",
						As:     "products",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Shop\nlayout: default\n---\n# Shop",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SiteData).To(HaveKey("products"),
				"plugin source data must appear in result.SiteData under the 'as' key")

			products, ok := result.SiteData["products"].([]interface{})
			Expect(ok).To(BeTrue(), "products must be a slice")
			Expect(products).To(HaveLen(1))
			first, ok := products[0].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(first["name"]).To(Equal("Widget"))
		})
	})

	// ── Plugin source caching through pipeline (issue #1044) ─────────
	// PLAN.md §5: "All source data (built-in and plugin) is cached to
	// .alloy/fetch-cache/ on disk." The case "plugin" branch in Build()
	// must check GetCached before calling FetchPluginSource, and SaveCache
	// after a successful fetch. Currently it calls the handler directly
	// on every build, re-executing expensive API calls unnecessarily.

	Describe("Plugin source caching through pipeline (issue #1044)", func() {
		BeforeEach(func() {
			fetch.ResetPluginSources()
		})

		// writeProjectFiles writes content and layout files into an existing
		// project root directory for use with Build() directly.
		// BuildWithContent creates and destroys a fresh tmpDir per call,
		// so cache written by build 1 is removed before build 2 starts.
		// Using Build() with a shared project root preserves cache across builds.
		writeProjectFiles := func(projectRoot string, files map[string]string) {
			for path, body := range files {
				fullPath := filepath.Join(projectRoot, path)
				Expect(os.MkdirAll(filepath.Dir(fullPath), 0755)).To(Succeed())
				Expect(os.WriteFile(fullPath, []byte(body), 0644)).To(Succeed())
			}
		}

		It("Build uses cached plugin source data when TTL has not expired", func() {
			callCount := 0
			fetch.RegisterPluginSource("cached-api", func(config map[string]interface{}) (interface{}, error) {
				callCount++
				return []interface{}{
					map[string]interface{}{"title": fmt.Sprintf("Post (gen %d)", callCount)},
				}, nil
			})

			projectRoot := GinkgoT().TempDir()
			writeProjectFiles(projectRoot, map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html>{{ content }}</html>",
			})

			cfg := &config.Config{
				Title:       "Cache Hit Test",
				BaseURL:     "https://example.com",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"blog": {
						Type:   "plugin",
						Plugin: "cached-api",
						Cache:  3600,
						As:     "posts",
					},
				},
			}

			// First build — handler must be called (cache miss)
			result1, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1).NotTo(BeNil())
			Expect(callCount).To(Equal(1),
				"first build must call the handler (cache miss)")

			// Second build — same project root, cache persists on disk.
			// Handler must NOT be called (cache hit, TTL=3600s).
			cfg2 := *cfg
			result2, err := pipeline.Build(&cfg2)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2).NotTo(BeNil())
			Expect(callCount).To(Equal(1),
				"second build must serve plugin source from cache when TTL has not "+
					"expired — currently the case 'plugin' branch in build.go calls "+
					"FetchPluginSource directly without checking GetCached first (issue #1044)")
		})

		It("Build with --refetch bypasses plugin source cache", func() {
			callCount := 0
			fetch.RegisterPluginSource("refetch-api", func(config map[string]interface{}) (interface{}, error) {
				callCount++
				return map[string]interface{}{"gen": float64(callCount)}, nil
			})

			projectRoot := GinkgoT().TempDir()
			writeProjectFiles(projectRoot, map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html>{{ content }}</html>",
			})

			cfg := &config.Config{
				Title:       "Refetch Test",
				BaseURL:     "https://example.com",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"data": {
						Type:   "plugin",
						Plugin: "refetch-api",
						Cache:  3600,
						As:     "data",
					},
				},
			}

			// First build — populates cache in shared projectRoot
			result1, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1).NotTo(BeNil())
			Expect(callCount).To(Equal(1))

			// Second build with --refetch — must bypass cache even though
			// the cache file exists in the same project root
			cfg2 := *cfg
			cfg2.Refetch = true
			_, err = pipeline.Build(&cfg2)
			Expect(err).NotTo(HaveOccurred())
			Expect(callCount).To(Equal(2),
				"--refetch must bypass plugin source cache and invoke the handler again — "+
					"same behavior as FetchRESTWithRefetch for REST sources")
		})

		It("Build populates cache after first successful plugin source fetch", func() {
			fetch.RegisterPluginSource("cache-populate", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{"item1", "item2"}, nil
			})

			projectRoot := GinkgoT().TempDir()
			writeProjectFiles(projectRoot, map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html>{{ content }}</html>",
			})

			cfg := &config.Config{
				Title:       "Cache Populate Test",
				BaseURL:     "https://example.com",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"items": {
						Type:   "plugin",
						Plugin: "cache-populate",
						Cache:  3600,
						As:     "items",
					},
				},
			}

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Verify cache was populated — check the same project root
			// that Build() used (no tmpDir indirection)
			cacheDir := fetch.CacheDir(projectRoot)
			_, found := fetch.GetCached("cache-populate", cacheDir, 3600)
			Expect(found).To(BeTrue(),
				"Build must call SaveCache after a successful plugin source fetch — "+
					"currently the case 'plugin' branch never calls SaveCache, "+
					"so the cache is never populated (issue #1044)")
		})
	})

	// ── Plugin source end-to-end pipeline dispatch (issue #1045) ─────
	// Full end-to-end test verifying the pipeline dispatch path:
	// fetch.RegisterPluginSource → config source → Build() dispatch →
	// site.data injection → template rendering.

	Describe("Plugin source end-to-end pipeline dispatch (issue #1045)", func() {
		BeforeEach(func() {
			fetch.ResetPluginSources()
		})

		It("plugin source data flows through Build to template output", func() {
			fetch.RegisterPluginSource("e2e-cms", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{
					map[string]interface{}{"title": "Alpha Post", "slug": "alpha"},
					map[string]interface{}{"title": "Beta Post", "slug": "beta"},
				}, nil
			})

			cfg := &config.Config{
				Title:   "E2E Source Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"blog": {
						Type:   "plugin",
						Plugin: "e2e-cms",
						As:     "posts",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Blog\nlayout: blog\n---\n# Blog Index",
				"layouts/blog.liquid":    "{% for post in site.data.posts %}TITLE:{{ post.title }}|{% endfor %}{{ content }}",
			}

			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Verify plugin source data reached the template context
			rendered := result.RenderedContent["index.md"]
			Expect(rendered).To(ContainSubstring("TITLE:Alpha Post|"),
				"plugin source data must flow through the full pipeline: "+
					"fetch.RegisterPluginSource → config source dispatch → "+
					"site.data injection → template rendering (issue #1045)")
			Expect(rendered).To(ContainSubstring("TITLE:Beta Post|"),
				"all source items must be accessible in the template loop")
			Expect(rendered).To(ContainSubstring("Blog Index"),
				"page content must render alongside source data")
		})

		It("plugin source config map is forwarded to the handler", func() {
			var receivedConfig map[string]interface{}
			fetch.RegisterPluginSource("config-forward", func(config map[string]interface{}) (interface{}, error) {
				receivedConfig = config
				return []interface{}{"ok"}, nil
			})

			cfg := &config.Config{
				Title:   "Config Forward Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Sources: map[string]*config.SourceConfig{
					"api": {
						Type:   "plugin",
						Plugin: "config-forward",
						Cache:  3600,
						As:     "api_data",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nhello",
				"layouts/default.liquid": "{{ content }}",
			}

			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())

			Expect(receivedConfig).NotTo(BeNil(),
				"handler must receive the config map from build.go dispatch")
			Expect(receivedConfig).To(HaveKeyWithValue("plugin", "config-forward"),
				"config map must include the plugin name")
			Expect(receivedConfig).To(HaveKeyWithValue("as", "api_data"),
				"config map must include the 'as' key for data namespace binding")
			Expect(receivedConfig).To(HaveKey("cache"),
				"config map must include the cache TTL setting")
		})
	})

})
