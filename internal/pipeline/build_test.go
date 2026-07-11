package pipeline_test

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
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

})
