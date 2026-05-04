package pipeline_test

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// spyReporter records all ProgressReporter calls for test assertions.
type spyReporter struct {
	stages    []string
	messages  []string
	updates   []spyUpdate
	ended     int
	summaries []spySummary
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

func (s *spyReporter) StartStage(name string, total int) { s.stages = append(s.stages, name) }
func (s *spyReporter) Message(text string)               { s.messages = append(s.messages, text) }
func (s *spyReporter) Update(current int, filePath string, elapsed time.Duration) {
	s.updates = append(s.updates, spyUpdate{current: current, filePath: filePath, elapsed: elapsed})
}
func (s *spyReporter) EndStage() { s.ended++ }
func (s *spyReporter) Summary(pageCount int, duration time.Duration, pagesSkipped int) {
	s.summaries = append(s.summaries, spySummary{pageCount: pageCount, duration: duration, pagesSkipped: pagesSkipped})
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
				Build:   config.BuildConfig{Output: "_site", Clean: true},
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

	// ── Incremental rebuild for serve mode (issue #225) ─────────────
	// BuildIncremental accepts a previous cache and a list of changed
	// files. It only rebuilds pages that changed or were invalidated.
	// Used by alloy serve on file watcher events.

	Describe("BuildIncremental", func() {
		It("skips unchanged pages using cache", func() {
			cfg := &config.Config{
				Title:   "Incremental Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
				"content/blog.md":  "---\ntitle: Blog\n---\n# Blog",
			}

			// Simulate: first full build populates cache from the same content
			previousCache := cache.New()
			for path, body := range contentMap {
				// Strip "content/" prefix to match Page.RelPath convention
				relPath := path[len("content/"):]
				previousCache.SetHash(relPath, cache.HashContent([]byte(body)))
			}

			// Only about.md changed
			contentMap["content/about.md"] = "---\ntitle: About\n---\n# About Updated"
			changedFiles := []string{"content/about.md"}

			result, err := pipeline.BuildIncremental(cfg, contentMap, previousCache, changedFiles)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"incremental build must only render the changed page")
			Expect(result.PagesSkipped).To(Equal(2),
				"unchanged pages must be skipped via cache comparison")
		})

		It("rebuilds all pages when no cache exists (first build)", func() {
			cfg := &config.Config{
				Title:   "Incremental Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
			}

			// No previous cache — all pages must be built
			result, err := pipeline.BuildIncremental(cfg, contentMap, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(2),
				"without cache, incremental build must render all pages (same as full build)")
		})

		It("rebuilds pages invalidated by layout change", func() {
			cfg := &config.Config{
				Title:   "Incremental Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
				"content/blog.md":  "---\ntitle: Blog\n---\n# Blog",
			}

			// Cache has all pages + template tracking
			previousCache := cache.New()
			for path, body := range contentMap {
				relPath := path[len("content/"):]
				previousCache.SetHash(relPath, cache.HashContent([]byte(body)))
			}
			previousCache.TrackTemplateUsage("index.md", "layouts/default.liquid")
			previousCache.TrackTemplateUsage("about.md", "layouts/default.liquid")
			previousCache.TrackTemplateUsage("blog.md", "layouts/post.liquid")

			// Layout changed — only pages using that layout need rebuilding
			changedFiles := []string{"layouts/default.liquid"}

			result, err := pipeline.BuildIncremental(cfg, contentMap, previousCache, changedFiles)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(2),
				"layout change must rebuild all pages using that layout (index + about)")
			Expect(result.PagesSkipped).To(Equal(1),
				"pages using a different layout (blog) must be skipped")
		})
	})

	// ── Incremental rebuild with SSR (issue #231) ──────────────────
	// alloy serve --preview runs the full pipeline including SSR.
	// Incremental rebuilds must run Phase 2 on rebuilt pages that
	// have custom elements, and handle component definition changes.

	// BuildResult.SSRPagesRendered tracks which pages went through Phase 2.
	// SSRSkipped is for "no SSR config" — SSRPagesRendered tracks actual
	// SSR invocations when config IS present.

	Describe("BuildIncremental with SSR", func() {
		It("runs Phase 2 SSR on incrementally rebuilt pages with custom elements", func() {
			cfg := &config.Config{
				Title:   "SSR Incremental Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				SSR:     &config.SSRConfig{Command: "cat"},
			}
			contentMap := map[string]string{
				"content/index.md":      "---\ntitle: Home\n---\n<h1>Home</h1>",
				"content/components.md": "---\ntitle: Components\n---\n<ds-card>Hello</ds-card>",
			}

			// Cache from previous build
			previousCache := cache.New()
			for path, body := range contentMap {
				relPath := path[len("content/"):]
				previousCache.SetHash(relPath, cache.HashContent([]byte(body)))
			}

			// components.md changed — has custom elements, needs SSR
			contentMap["content/components.md"] = "---\ntitle: Components\n---\n<ds-card>Updated</ds-card>"
			changedFiles := []string{"content/components.md"}

			result, err := pipeline.BuildIncremental(cfg, contentMap, previousCache, changedFiles)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"only the changed page must be rebuilt in Phase 1")
			Expect(result.SSRSkipped).To(BeFalse(),
				"SSR must not be skipped when SSR config is present")
			Expect(result.SSRPagesRendered).To(Equal(1),
				"exactly 1 page (components.md) must go through Phase 2 SSR — "+
					"it has custom elements and its content changed")
		})

		It("component definition change triggers re-SSR without Phase 1 rebuild", func() {
			cfg := &config.Config{
				Title:   "SSR Incremental Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				SSR:     &config.SSRConfig{Command: "cat"},
			}
			contentMap := map[string]string{
				"content/index.md":  "---\ntitle: Home\n---\n<h1>Home</h1>",
				"content/page-a.md": "---\ntitle: Page A\n---\n<ds-card>A</ds-card>",
				"content/page-b.md": "---\ntitle: Page B\n---\n<ds-button>B</ds-button>",
			}

			// Cache from previous build — all pages unchanged
			previousCache := cache.New()
			for path, body := range contentMap {
				relPath := path[len("content/"):]
				previousCache.SetHash(relPath, cache.HashContent([]byte(body)))
			}

			// A component source file changed (not a content file).
			// Pages using ds-card must be re-SSR'd even though their
			// content hasn't changed. Phase 1 skips all pages.
			changedFiles := []string{"components/ds-card/ds-card.js"}

			result, err := pipeline.BuildIncremental(cfg, contentMap, previousCache, changedFiles)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(0),
				"no content changed — Phase 1 must skip all pages")
			Expect(result.PagesSkipped).To(Equal(3),
				"all 3 pages skipped in Phase 1")
			Expect(result.SSRPagesRendered).To(Equal(1),
				"only page-a.md (uses ds-card) must be re-SSR'd — "+
					"page-b.md (ds-button) and index.md (no components) are unaffected")
		})

		It("skips SSR for pages whose Phase 1 output is unchanged", func() {
			cfg := &config.Config{
				Title:   "SSR Incremental Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				SSR:     &config.SSRConfig{Command: "cat"},
			}
			contentMap := map[string]string{
				"content/index.md":      "---\ntitle: Home\n---\n<h1>Home</h1>",
				"content/components.md": "---\ntitle: Components\n---\n<ds-card>Hello</ds-card>",
			}

			// Cache from previous build — nothing changed
			previousCache := cache.New()
			for path, body := range contentMap {
				relPath := path[len("content/"):]
				previousCache.SetHash(relPath, cache.HashContent([]byte(body)))
			}

			// No files changed — incremental rebuild should skip everything
			result, err := pipeline.BuildIncremental(cfg, contentMap, previousCache, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(0),
				"no pages changed — nothing to rebuild in Phase 1")
			Expect(result.PagesSkipped).To(Equal(2),
				"all pages must be skipped in Phase 1")
			Expect(result.SSRPagesRendered).To(Equal(0),
				"no SSR invocations — nothing changed, no pages need re-SSR")
		})
	})

	// ── Phase 1 → Phase 2 handoff (§2) ──────────────────────────────
	// Per spec §6: Phase 2 operates in memory. For each page with custom
	// elements, Alloy pipes the full page HTML to ssr.command via stdin.
	// Mode "exec" (default): one process per page. Mode "stream": persistent
	// process with NUL-delimited messages. The SSR engine handles all
	// component rendering internally.

	Describe("Phase 1 → Phase 2 handoff", func() {
		It("Phase 1 produces intermediate HTML preserving raw custom element tags", func() {
			cfg := &config.Config{
				Title: "Component Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			intermediate, err := pipeline.BuildPhase1(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"Phase 1 must complete without error")
			Expect(intermediate).NotTo(BeEmpty(),
				"Phase 1 must produce at least one page of intermediate HTML")
		})

		It("Phase 2 invokes command per page, piping full HTML via stdin", func() {
			// Intermediate HTML contains a custom element (hyphenated tag).
			// BuildPhase2 must attempt to invoke the command for each page
			// containing custom elements. Using a nonexistent command proves
			// the invocation is attempted — the page's original HTML is
			// preserved (SSR failed, no transform applied).
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card title="Hello">content</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "golit render --defs bundles/",
			}
			// The command won't exist in the test environment. BuildPhase2
			// must not abort — it skips failed pages and preserves original HTML.
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"SSR command failure must not abort the build — errors are collected")
			Expect(result).To(HaveKey("content/index.md"),
				"failed SSR page must be present in result")
			Expect(result["content/index.md"]).To(ContainSubstring("ds-card"),
				"failed SSR page must preserve original HTML — proves command was "+
					"attempted (not silently skipped) and original HTML was kept on failure")
		})

		It("Phase 2 receives Phase 1 output as its input", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR: &config.SSRConfig{
					// cat reads stdin, writes to stdout — proves the per-page
					// stdio model works end-to-end
					Command: "cat",
				},
				Build: config.BuildConfig{Output: "_site"},
			}

			// Phase 1 produces intermediate HTML
			intermediate, err := pipeline.BuildPhase1(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(intermediate).NotTo(BeEmpty(),
				"Phase 1 must produce intermediate output")

			// Phase 2 takes Phase 1 output directly and pipes to ssr.command
			// via stdin. cat passes HTML through unchanged.
			result, err := pipeline.BuildPhase2(intermediate, cfg.SSR)
			Expect(err).NotTo(HaveOccurred(),
				"Phase 2 with cat must succeed — cat passes stdin to stdout")
			Expect(result).NotTo(BeNil())
			// Every page from Phase 1 must appear in Phase 2 output
			for path := range intermediate {
				Expect(result).To(HaveKey(path),
					"Phase 2 output must contain every page from Phase 1")
				Expect(result[path]).NotTo(BeEmpty(),
					"Phase 2 output for %s must not be empty", path)
			}
		})

		It("without SSR config, Phase 1 output is the final HTML", func() {
			cfg := &config.Config{
				Title: "No SSR Site",
				Build: config.BuildConfig{Output: "_site"},
				// SSR is nil — no ssr: config block
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"without ssr: config, Phase 2 must be skipped entirely")
		})
	})

	// ── SSR per-page render ─────────────────────────────────────────
	// Phase 2 extracts the inner content of <body>, pipes it to
	// ssr.command via stdin, and re-inserts the SSR'd body content
	// into the original document skeleton. The SSR engine never sees
	// <!DOCTYPE>, <html>, <head>, or <body> tags.

	Describe("SSR per-page render", func() {
		It("BuildPhase2 preserves original HTML when command is not found", func() {
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-button>Click</ds-button></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "nonexistent-ssr-tool render --defs bundles/",
			}
			// Command not found is a per-page failure, not a build-aborting error.
			// The page's original HTML must be preserved in the result.
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"SSR command not found must not abort the build — page is skipped")
			Expect(result).To(HaveKey("content/index.md"),
				"page must be present in result even when SSR command is not found")
			Expect(result["content/index.md"]).To(ContainSubstring("ds-button"),
				"page must preserve original HTML when SSR command is not found")
		})

		It("BuildPhase2 does not fall back to local DSD transform", func() {
			// When the command is unavailable, BuildPhase2 must NOT
			// silently insert <template shadowrootmode> via a local transform.
			// SSR is the external engine's responsibility. The page's
			// original HTML must be preserved unchanged.
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card>content</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "nonexistent-ssr-tool render",
			}
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"SSR command failure must not abort the build")
			Expect(result).To(HaveKey("content/index.md"))
			Expect(result["content/index.md"]).NotTo(ContainSubstring("shadowrootmode"),
				"BuildPhase2 must not silently fall back to local DSD transform "+
					"when the ssr.command is unavailable")
		})

		It("BuildPhase2 preserves document skeleton after SSR", func() {
			// Phase 2 must extract body content, pipe it to the SSR command,
			// and re-insert the result into the original document skeleton.
			// The <head>, <script>, and other document tags must survive SSR.
			intermediate := map[string]string{
				"content/index.md": `<!DOCTYPE html><html><head><title>Test</title><script src="app.js"></script></head><body><h1>Hello</h1><ds-card>content</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				// cat passes body content through unchanged — proves the
				// document skeleton is preserved by Alloy, not the SSR engine
				Command: "cat",
			}
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"BuildPhase2 with cat must succeed")
			Expect(result).To(HaveKey("content/index.md"))
			html := result["content/index.md"]
			Expect(html).To(ContainSubstring("<!DOCTYPE html>"),
				"document skeleton must preserve DOCTYPE after SSR")
			Expect(html).To(ContainSubstring("<head>"),
				"document skeleton must preserve <head> after SSR")
			Expect(html).To(ContainSubstring(`<script src="app.js"></script>`),
				"document skeleton must preserve <script> tags in <head> after SSR")
			Expect(html).To(ContainSubstring("<ds-card>"),
				"body content must be present after SSR")
			Expect(html).To(ContainSubstring("</html>"),
				"document skeleton must preserve closing </html> after SSR")
		})

		It("BuildPhase2 passes through HTML unchanged when no custom elements present", func() {
			// Pages without custom elements (no hyphenated tags) should pass
			// through Phase 2 unchanged — no command invocations needed.
			intermediate := map[string]string{
				"content/plain.md": `<html><body><h1>Hello</h1><p>No components here.</p></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				// Command that would fail if invoked — proves it's NOT called
				Command: "false",
			}
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"BuildPhase2 must not error on pages without custom elements")
			Expect(result).NotTo(BeNil())
			Expect(result["content/plain.md"]).To(Equal(intermediate["content/plain.md"]),
				"HTML without custom elements must pass through Phase 2 unchanged")
		})
	})

	// ── SSR stream mode ────────────────────────────────────────────

	Describe("SSR stream mode", func() {
		It("BuildPhase2 uses persistent process when mode is stream", func() {
			// With mode "stream", BuildPhase2 must start a persistent process
			// and send NUL-delimited messages instead of spawning per page.
			// Using a nonexistent command proves the stream startup is attempted.
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card>Hello</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "nonexistent-ssr-tool serve --stdio",
				Mode:    "stream",
			}
			_, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).To(HaveOccurred(),
				"BuildPhase2 in stream mode must attempt to start the persistent process")
		})

		It("BuildPhase2 defaults to exec mode when mode is empty", func() {
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card>Hello</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "nonexistent-ssr-tool render --defs bundles/",
				// Mode is empty — defaults to exec
			}
			// Command not found is a per-page failure — page is skipped,
			// original HTML preserved. The test proves exec mode is used
			// (not stream) by verifying the page is in the result with
			// its original HTML intact.
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"SSR command failure must not abort the build")
			Expect(result).To(HaveKey("content/index.md"),
				"page must be present in result when exec mode command fails")
			Expect(result["content/index.md"]).To(ContainSubstring("ds-card"),
				"page must preserve original HTML — proves exec mode was used "+
					"(not stream) and the page was skipped on failure")
		})
	})

	// ── Issue #162: SSR timeout wiring ──────────────────────────────
	// RenderPageWithTimeout exists and is tested in internal/ssr, but
	// BuildPhase2 exec mode calls RenderPage (no timeout). ssrCfg.Timeout
	// is parsed but never used. Exec mode must enforce the timeout.

	Describe("SSR timeout wiring", func() {
		It("BuildPhase2 exec mode enforces ssrCfg.Timeout", func() {
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card>Hello</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "sleep 1",
				Timeout: "50ms",
			}
			// sleep 1 takes 1 second — the 50ms timeout must kill it.
			// Timeout is a per-page failure: page is skipped, original HTML
			// preserved. The build does not abort.
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"SSR timeout must not abort the build — page is skipped")
			Expect(result).To(HaveKey("content/index.md"),
				"timed-out page must be present in result")
			Expect(result["content/index.md"]).To(ContainSubstring("ds-card"),
				"timed-out page must preserve original HTML")
		})

		It("BuildPhase2 uses default timeout when ssrCfg.Timeout is empty", func() {
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card>Hello</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "cat",
				// Timeout is empty — should default to 30s, not hang forever
			}
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"cat must complete well within the default 30s timeout")
			Expect(result).NotTo(BeNil())
		})
	})

	// ── Issue #173: Stream mode timeout wiring ──────────────────────
	// Stream mode must enforce ssr.timeout per page, same as exec mode.

	Describe("SSR stream mode timeout", func() {
		It("BuildPhase2 stream mode enforces ssrCfg.Timeout", func() {
			intermediate := map[string]string{
				"content/index.md": `<html><body><ds-card>Hello</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "sleep 1",
				Mode:    "stream",
				Timeout: "50ms",
			}
			// sleep 1 takes 1 second — the 50ms timeout must kill the read.
			// Timeout is a per-page failure: page is skipped, original HTML
			// preserved. The build does not abort.
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"stream mode timeout must not abort the build — page is skipped")
			Expect(result).To(HaveKey("content/index.md"),
				"timed-out page must be present in result")
			Expect(result["content/index.md"]).To(ContainSubstring("ds-card"),
				"timed-out page must preserve original HTML")
		})
	})

	// ── Issue #164: SSR error collection (skip, don't abort) ─────────
	// Per spec: failed pages should be skipped (original HTML preserved),
	// errors collected, and reported at the end — not abort the build.

	Describe("SSR error collection", func() {
		It("exec mode skips failed pages and continues with remaining pages", func() {
			intermediate := map[string]string{
				// This page has a custom element — SSR will be attempted
				"content/good.md": `<html><body><ds-card>Good</ds-card></body></html>`,
				// This page has no custom elements — should pass through
				"content/plain.md": `<html><body><h1>No components</h1></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				// nonexistent command — SSR will fail for pages with components
				Command: "nonexistent-ssr-tool",
			}
			// BuildPhase2 must NOT abort on SSR failure. It must collect
			// errors and return a result containing all pages.
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"BuildPhase2 must not return a fatal error when SSR fails — "+
					"errors should be collected, not abort the build")
			Expect(result).NotTo(BeNil())
			Expect(result["content/plain.md"]).To(Equal(intermediate["content/plain.md"]),
				"pages without custom elements must pass through unchanged "+
					"even when SSR fails for other pages")
		})

		It("failed page preserves original HTML instead of being dropped", func() {
			intermediate := map[string]string{
				"content/page.md": `<html><body><ds-card>Content</ds-card></body></html>`,
			}
			ssrCfg := &config.SSRConfig{
				Command: "nonexistent-ssr-tool",
			}
			// When SSR fails for a page, the original (un-SSR'd) HTML must
			// be preserved in the output — not dropped, not cause a fatal error.
			result, err := pipeline.BuildPhase2(intermediate, ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"SSR failure for one page must not abort the build")
			Expect(result).To(HaveKey("content/page.md"),
				"failed SSR page must be present in result with original HTML")
			Expect(result["content/page.md"]).To(ContainSubstring("ds-card"),
				"failed SSR page must preserve original HTML with raw custom elements")
		})
	})

	// ── SSR skip behavior ────────────────────────────────────────────

	Describe("SSR skip behavior", func() {
		It("no SSR config sets SSRSkipped to true", func() {
			cfg := &config.Config{
				Title: "Plain Site",
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"build without ssr: config must skip Phase 2")
		})

		It("with SSR config, Phase 2 runs and SSRSkipped is false", func() {
			// Guard: without SSR, SSRSkipped must be true
			noSSRCfg := &config.Config{
				Title: "No SSR",
				Build: config.BuildConfig{Output: "_site"},
			}
			noSSRResult, err := pipeline.Build(noSSRCfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(noSSRResult).NotTo(BeNil())
			Expect(noSSRResult.SSRSkipped).To(BeTrue(),
				"guard: no SSR config must set SSRSkipped=true")

			// Actual: with SSR, Build attempts Phase 2 (invokes ssr.command).
			// Use "cat" — reads stdin, writes to stdout. Proves the per-page
			// stdio model works end-to-end.
			ssrCfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			ssrResult, err := pipeline.Build(ssrCfg)
			Expect(err).NotTo(HaveOccurred(),
				"Build with cat as SSR command must succeed — cat passes stdin to stdout")
			Expect(ssrResult).NotTo(BeNil())
			Expect(ssrResult.SSRSkipped).To(BeFalse(),
				"build with ssr: config must run Phase 2")
		})
	})

	// ── Content-colocated passthrough copy (issue #300) ─────────────
	// Non-content files in content/ must be copied to _site/ preserving
	// their path relative to content/.

	Describe("Content-colocated passthrough copy", func() {
		It("copies non-content files to output directory", func() {
			cfg := &config.Config{
				Title:   "Passthrough Copy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about/index.md":    "---\ntitle: About\n---\n# About",
				"content/about/diagram.svg": `<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`,
				"content/about/photo.png":   "fake png bytes",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// The content page must be rendered
			Expect(result.PageCount).To(Equal(1),
				"only .md files should be content pages")

			// Non-content files must be copied to output
			Expect(result.ContentPassthroughs).To(ContainElement("about/diagram.svg"),
				"SVG in content/ must be copied to _site/about/diagram.svg")
			Expect(result.ContentPassthroughs).To(ContainElement("about/photo.png"),
				"PNG in content/ must be copied to _site/about/photo.png")
		})

		It("does not copy _data.yaml as passthrough", func() {
			cfg := &config.Config{
				Title:   "Data Exclusion Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/blog/index.md":    "---\ntitle: Blog\n---\n# Blog",
				"content/blog/_data.yaml":  "layout: post",
				"content/blog/icon.svg":    "<svg></svg>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.ContentPassthroughs).To(HaveLen(1),
				"_data.yaml must not be copied as passthrough")
			Expect(result.ContentPassthroughs).To(ContainElement("blog/icon.svg"))
		})
	})

	// ── Early static/asset copy (issue #492) ────────────────────────
	// Static and passthrough copies run in a background goroutine.
	// BuildWithContent uses a temp dir that is cleaned up on return,
	// so filesystem assertions on the output dir are not possible here.
	// These tests verify the build succeeds with static files in the
	// content map — a failure indicates the background goroutine or
	// output dir creation order is broken.

	Describe("Early static/asset copy (issue #492)", func() {
		It("build succeeds with static files in content map", func() {
			cfg := &config.Config{
				Title:   "Static Copy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"static/robots.txt":      "User-agent: *\nDisallow:",
				"static/css/main.css":    "body { margin: 0; }",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed with static files — "+
					"if this errors, output dir creation or static copy goroutine failed")
			Expect(result).NotTo(BeNil())
			Expect(result.RenderedContent).To(HaveKey("index.md"),
				"rendered content must be present alongside static files")
		})

		It("build succeeds with passthrough mappings", func() {
			cfg := &config.Config{
				Title:   "Passthrough Copy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"content/blog/icon.svg":  "<svg></svg>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed with content-colocated passthrough files")
			Expect(result).NotTo(BeNil())
			Expect(result.ContentPassthroughs).To(ContainElement("blog/icon.svg"),
				"passthrough files must be tracked in BuildResult")
		})
	})

	// ── Taxonomy collection page properties (issue #328) ────────────
	// Pages in taxonomy collections must expose title, url, slug via
	// ToTemplateMap() — not raw *content.Page structs.

	Describe("Taxonomy collection page properties", func() {
		It("taxonomy collection items expose title and url in templates", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Props Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/about.md":   "---\ntitle: About\ntags: [\"about\"]\n---\n# About",
				"content/roadmap.md": "---\ntitle: Roadmap\ntags: [\"about\"]\n---\n# Roadmap",
				"layouts/default.liquid": `<html><body>{{ content }}{% for p in taxonomies.tags.about %}<span class="item">{{ p.title }}|{{ p.url }}</span>{% endfor %}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Collect all rendered HTML into a single string so assertions
			// fire unconditionally — no if-guard that could silently pass.
			var allHTML string
			for _, html := range result.RenderedContent {
				allHTML += html
			}
			Expect(allHTML).To(ContainSubstring(`class="item"`),
				"at least one page must render the taxonomy collection loop — "+
					"if this fails, the layout didn't render or the collection is empty")
			Expect(allHTML).To(ContainSubstring("About|"),
				"taxonomy collection items must expose title via ToTemplateMap()")
			Expect(allHTML).To(ContainSubstring("Roadmap|"),
				"all pages tagged 'about' must appear with their title")
			Expect(allHTML).To(MatchRegexp(`About\|/about`),
				"taxonomy collection items must expose url via ToTemplateMap()")
		})
	})

	// ── Taxonomy template access (issue #380) ───────────────────────
	// taxonomies.* must be populated and accessible in both layouts and
	// content templates. The user reported taxonomies appearing empty
	// when content is in subdirectories (e.g., content/blog/post-a.md).

	Describe("Taxonomy template access (issue #380)", func() {
		It("taxonomies.tags is accessible in content templates", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Access Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\", \"web\"]\nlayout: default\n---\n# Post A",
				"content/post-b.md": "---\ntitle: Post B\ntags: [\"go\"]\nlayout: default\n---\n# Post B",
				"content/index.md":  "---\ntitle: Index\nlayout: default\n---\n{% for post in taxonomies.tags.go %}<span class=\"tagged\">{{ post.title }}</span>{% endfor %}",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty(),
				"index page must render")
			Expect(html).To(ContainSubstring(`class="tagged"`),
				"taxonomies.tags.go must be iterable in content templates — "+
					"if missing, taxonomies are not injected into the content render context")
			Expect(html).To(ContainSubstring("Post A"),
				"Post A is tagged 'go' and must appear in taxonomies.tags.go")
			Expect(html).To(ContainSubstring("Post B"),
				"Post B is tagged 'go' and must appear in taxonomies.tags.go")
		})

		It("taxonomies are accessible in layouts, not just content", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Layout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\"]\nlayout: default\n---\n# Post A",
				"content/post-b.md": "---\ntitle: Post B\ntags: [\"go\"]\nlayout: default\n---\n# Post B",
				"layouts/default.liquid": "<html><body>{{ content }}<nav>{% for post in taxonomies.tags.go %}<a href=\"{{ post.url }}\">{{ post.title }}</a>{% endfor %}</nav></body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			var allHTML string
			for _, html := range result.RenderedContent {
				allHTML += html
			}
			Expect(allHTML).To(ContainSubstring("<nav>"),
				"layout nav must render")
			Expect(allHTML).To(ContainSubstring("Post A"),
				"Post A tagged 'go' must appear in layout taxonomy loop")
		})

		It("taxonomies work when content is in subdirectories", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Subdir Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags":       {Render: &renderFalse},
					"categories": {Render: &renderFalse},
				},
				Permalinks: map[string]string{
					"blog":    "/:year/:month/:slug/",
					"default": "/:slug/",
				},
			}
			contentMap := map[string]string{
				"content/blog/_data.yaml":  "layout: post",
				"content/blog/post-a.md":   "---\ntitle: Post A\ndate: 2026-04-01\ntags: [\"go\", \"web\"]\ncategories: [\"tutorials\"]\nlayout: default\n---\n# Post A",
				"content/blog/post-b.md":   "---\ntitle: Post B\ndate: 2026-04-02\ntags: [\"go\", \"testing\"]\ncategories: [\"tutorials\"]\nlayout: default\n---\n# Post B",
				"content/blog/post-c.md":   "---\ntitle: Post C\ndate: 2026-04-03\ntags: [\"css\", \"web\"]\ncategories: [\"design\"]\nlayout: default\n---\n# Post C",
				"content/series-test.md":   "---\ntitle: Series Test\nlayout: default\n---\n{% for post in taxonomies.tags.go %}<span class=\"go-tag\">{{ post.title }}</span>{% endfor %}",
				"layouts/default.liquid":   "<html><body>{{ content }}</body></html>",
				"layouts/post.liquid":      "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["series-test.md"]
			Expect(html).NotTo(BeEmpty(),
				"series-test page must render")
			Expect(html).To(ContainSubstring(`class="go-tag"`),
				"taxonomies.tags.go must be iterable when tagged content is in subdirectories — "+
					"if empty, taxonomy building may not collect pages from nested content dirs")
			Expect(html).To(ContainSubstring("Post A"),
				"Post A is tagged 'go' and must appear")
			Expect(html).To(ContainSubstring("Post B"),
				"Post B is tagged 'go' and must appear")
			Expect(html).NotTo(ContainSubstring("Post C"),
				"Post C is NOT tagged 'go' — must not appear")
		})

		It("taxonomy with no matching pages produces empty collection", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Empty Taxonomy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags":       {Render: &renderFalse},
					"categories": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\"]\nlayout: default\n---\n# Post A",
				"content/index.md":  "---\ntitle: Index\nlayout: default\n---\n{% for post in taxonomies.categories.news %}{{ post.title }}{% endfor %}\n\nDONE_MARKER",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty(),
				"index page must render")
			Expect(html).To(ContainSubstring("DONE_MARKER"),
				"index page must render even when taxonomy term has no pages")
			Expect(html).NotTo(ContainSubstring("Post A"),
				"Post A is not in categories.news — must not appear")
		})
	})

	// ── Taxonomy layout with front matter (issue #418) ──────────────
	// Taxonomy layouts may contain YAML front matter (e.g., layout: base
	// for chaining). The pipeline must strip front matter before parsing
	// the taxonomy layout — otherwise the --- delimiters render as text.

	Describe("Taxonomy layout with front matter (issue #418)", func() {
		It("taxonomy layout front matter is stripped before rendering", func() {
			renderTrue := true
			cfg := &config.Config{
				Title:   "Taxonomy Layout FM Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderTrue, Layout: "tags"},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\"]\nlayout: default\n---\n# Post A",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"layouts/tags.liquid": "---\nlayout: base\n---\n<div class=\"taxonomy\">{{ taxonomy.term }}</div>",
				"layouts/base.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must not error when taxonomy layout has front matter — "+
					"if this fails, the layout parser is not stripping front matter")
			Expect(result).NotTo(BeNil())

			// Taxonomy pages are generated (no RelPath), so their
			// RenderedContent key is page.URL (e.g., "/tags/", "/tags/go/").
			found := false
			for key, html := range result.RenderedContent {
				if strings.Contains(key, "/tags/") || key == "/tags/" {
					found = true
					Expect(html).NotTo(ContainSubstring("---"),
						"taxonomy layout front matter delimiters must be stripped — "+
							"if '---' appears in output, StripLayoutFrontMatter was not called")
					Expect(html).NotTo(ContainSubstring("layout: base"),
						"taxonomy layout front matter content must not appear in rendered output")
					Expect(html).To(ContainSubstring("taxonomy"),
						"taxonomy layout must render its content")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one taxonomy page must appear in RenderedContent with a /tags/ URL key — "+
					"taxonomy pages have no RelPath, so renderedContentKey must use page.URL")
		})
	})

	// ── page.toc pipeline wiring (issue #274) ───────────────────────
	// page.toc must be populated during Build and accessible in templates.

	Describe("TOC pipeline wiring", func() {
		It("page.toc is accessible in layout templates", func() {
			cfg := &config.Config{
				Title:   "TOC Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/guide.md": "---\ntitle: Guide\nlayout: default\n---\n## Getting Started\n\n### Installation\n\n## Configuration",
				"layouts/default.liquid": `<html><body>{{ content }}<nav>{% for item in page.toc %}<a href="#{{ item.id }}">{{ item.text }}</a>{% endfor %}</nav></body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["guide.md"]
			Expect(html).To(ContainSubstring(`href="#getting-started"`),
				"page.toc must be populated and accessible in layout templates — "+
					"TOC links must render with heading IDs")
			Expect(html).To(ContainSubstring(">Getting Started<"),
				"TOC entry text must be available in templates")
			Expect(html).To(ContainSubstring(">Configuration<"),
				"all h2 headings must appear in page.toc")
		})
	})

	// ── External data files (issue #271) ────────────────────────────
	// Files outside data/ can be mapped into site.data via config.

	Describe("External data files", func() {
		It("loads external data file into site.data namespace", func() {
			cfg := &config.Config{
				Title:   "External Data Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Data: config.DataConfig{
					Files: map[string]string{
						"cem": "external/custom-elements.json",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":              "---\ntitle: Home\nlayout: default\n---\n# Home",
				"external/custom-elements.json": `{"schemaVersion":"1.0","modules":[{"kind":"javascript-module"}]}`,
				"layouts/default.liquid":         `<html><body>{{ content }}<p>Schema: {{ site.data.cem.schemaVersion }}</p></body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("Schema: 1.0"),
				"external data file must be loaded into site.data.cem — "+
					"template must access site.data.cem.schemaVersion")
		})

		It("errors when external key collides with data/ directory file", func() {
			cfg := &config.Config{
				Title:   "Collision Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Data: config.DataConfig{
					Files: map[string]string{
						"nav": "external/nav.json",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md":   "---\ntitle: Home\n---\n# Home",
				"data/nav.yaml":      "- title: Home\n  url: /",
				"external/nav.json":  `[{"title":"About","url":"/about/"}]`,
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"external data key 'nav' collides with data/nav.yaml — "+
					"must be a build error, not silent overwrite")
		})

		It("errors when external data file not found", func() {
			cfg := &config.Config{
				Title:   "Missing Data Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Data: config.DataConfig{
					Files: map[string]string{
						"missing": "nonexistent/file.json",
					},
				},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"missing external data file must be a build error — "+
					"not silently skipped")
		})
	})

	// ── Render hook pipeline wiring (issues #310, #311) ─────────────
	// The pipeline must discover render hook templates from
	// layouts/_markup/ and wire them into MarkdownOptions.

	Describe("Render hook pipeline wiring", func() {
		It("render hooks from layouts/_markup/ are applied during build", func() {
			cfg := &config.Config{
				Title:   "Render Hook Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/page.md": "---\ntitle: Test\n---\n[Click here](https://example.com)",
				"layouts/_markup/render-link.liquid": `<a href="{{ markup.destination }}" class="custom-link">{{ markup.text }}</a>`,
				"layouts/default.liquid":             "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["page.md"]
			Expect(html).To(ContainSubstring(`class="custom-link"`),
				"render hook from layouts/_markup/render-link.liquid must be applied "+
					"during the build pipeline — proves discovery + wiring works end-to-end")
		})
	})

	// ── Pagination 'as' variable in template body (issue #340) ──────
	// The pagination 'as' alias must be available in the template body,
	// not just in the permalink pattern.

	Describe("Pagination as variable in body", func() {
		It("pagination as variable is accessible in rendered content", func() {
			cfg := &config.Config{
				Title:   "Pagination As Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/categories.json": `[{"slug":"color","title":"Color"},{"slug":"space","title":"Space"}]`,
				"content/tokens.md": "---\ntitle: Tokens\nlayout: default\npagination:\n  data: site.data.categories\n  perPage: 1\n  as: category\npermalink: \"/tokens/{{ category.slug }}/\"\n---\n## {{ category.title }}\n\nSlug: {{ category.slug }}",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Find a virtual page that rendered the category data
			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "Color") {
					found = true
					Expect(html).To(ContainSubstring("Color"),
						"category.title must resolve in the template body")
					Expect(html).To(ContainSubstring("Slug: color"),
						"category.slug must also resolve in the body")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with the pagination as variable — "+
					"if this fails, the as variable resolves in permalink but not body")
		})
	})

	// ── Pagination front matter interpolation (issue #378) ──────────
	// String-valued front matter fields with template tags must be
	// interpolated using the pagination as: variable for virtual pages.

	Describe("Pagination front matter interpolation", func() {
		It("title is interpolated from pagination as variable", func() {
			cfg := &config.Config{
				Title:   "FM Interpolation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"}]`,
				"content/team.md": "---\ntitle: \"{{ member.name }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>Alice</title>"),
						"page.title must be interpolated from {{ member.name }} — "+
							"front matter template tags must resolve using the pagination as: variable")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with interpolated title")
		})

		It("front matter interpolation supports filters", func() {
			cfg := &config.Config{
				Title:   "FM Filter Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"alice","slug":"alice"}]`,
				"content/team.md": "---\ntitle: \"{{ member.name | upcase }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>ALICE</title>"),
						"front matter interpolation must support Liquid filters — "+
							"{{ member.name | upcase }} should produce ALICE")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with filter-processed title")
		})

		It("multiple front matter fields are interpolated", func() {
			cfg := &config.Config{
				Title:   "FM Multi Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice","role":"Engineer"}]`,
				"content/team.md": "---\ntitle: \"{{ member.name }}\"\ndescription: \"{{ member.role }} at Acme\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title><meta name=\"description\" content=\"{{ page.description }}\"></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>Alice</title>"),
						"page.title must be interpolated")
					Expect(html).To(ContainSubstring("Engineer at Acme"),
						"page.description must also be interpolated — "+
							"all string front matter fields with template tags should resolve")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with multiple interpolated fields")
		})

		It("non-template front matter fields are unchanged", func() {
			cfg := &config.Config{
				Title:   "FM Passthrough Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice"}]`,
				"content/team.md": "---\ntitle: \"Static Title\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>Static Title</title>"),
						"front matter without template tags must pass through unchanged")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render")
		})

		It("paginated list pages do not interpolate front matter", func() {
			cfg := &config.Config{
				Title:   "FM List Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"}]`,
				"content/team.md": "---\ntitle: \"Team Members\"\nheading: \"{{ member.name }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 10\n  as: member\npermalink: \"/team/\"\n---\n{% for m in member %}<p>{{ m.name }}</p>{% endfor %}",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body><h1>{{ page.heading }}</h1>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					// page.heading should NOT have been interpolated to a member name
					// because perPage > 1 means member is a slice, not a single item
					Expect(html).NotTo(ContainSubstring("<h1>Alice</h1>"),
						"paginated list pages (perPage > 1) must NOT interpolate front matter — "+
							"the as: variable is a slice, not a single item")
					Expect(html).NotTo(ContainSubstring("<h1>Bob</h1>"),
						"paginated list pages must not interpolate to any individual item")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one paginated list page must render")
		})
	})

	// ── JSON data key order in templates (issue #456) ──────────────
	// Liquid templates iterating JSON data files must see keys in
	// the document's insertion order, not Go's random map order.

	Describe("JSON data key order in templates (issue #456)", func() {
		It("{% for %} over JSON data preserves insertion order", func() {
			cfg := &config.Config{
				Title:   "JSON Order Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/colors.json": `{"white":"#fff","black":"#000","accent":"#e00","brand":"#ee0","surface":"#f0f"}`,
				"content/index.md": "---\ntitle: Colors\nlayout: default\n---\n# Colors",
				"layouts/default.liquid": "<html><body>{% for color in site.data.colors %}<span>{{ color[0] }}</span>{% endfor %}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty())

			// The spans must appear in JSON insertion order
			whiteIdx := strings.Index(html, "<span>white</span>")
			blackIdx := strings.Index(html, "<span>black</span>")
			accentIdx := strings.Index(html, "<span>accent</span>")
			brandIdx := strings.Index(html, "<span>brand</span>")
			surfaceIdx := strings.Index(html, "<span>surface</span>")

			Expect(whiteIdx).To(BeNumerically(">=", 0),
				"white must appear in output")
			Expect(blackIdx).To(BeNumerically(">", whiteIdx),
				"black must appear after white — JSON insertion order")
			Expect(accentIdx).To(BeNumerically(">", blackIdx),
				"accent must appear after black")
			Expect(brandIdx).To(BeNumerically(">", accentIdx),
				"brand must appear after accent")
			Expect(surfaceIdx).To(BeNumerically(">", brandIdx),
				"surface must appear after brand — "+
					"if order is wrong, JSON data was loaded into map[string]interface{} "+
					"instead of *ordered.Map (issue #453)")
		})
	})

	// ── Node plugin cwd with ProjectRoot (issue #439) ──────────────
	// When cfg.ProjectRoot is set (via -r flag), the Node bridge must
	// spawn its subprocess with cwd = ProjectRoot so node_modules/
	// imports resolve from the project directory.

	Describe("Node plugin respects ProjectRoot (issue #439)", func() {
		It("DiscoverPlugins passes ProjectRoot to Node plugin bridge", func() {
			// Create a project with a Node plugin
			projectDir := GinkgoT().TempDir()

			pluginsDir := filepath.Join(projectDir, "plugins")
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(pluginsDir, "test-plugin.js"),
				[]byte("// runtime: \"node\"\nexport default function(alloy) { alloy.filter('testNodeFilter', (v) => v); }"),
				0644,
			)).To(Succeed())

			cfg := &config.Config{
				Title:       "Node CWD Test",
				BaseURL:     "https://example.com",
				Build:       config.BuildConfig{Output: "_site"},
				ProjectRoot: projectDir,
				Plugins:     config.PluginsConfig{Node: true, Timeout: 5000},
			}
			config.ApplyDefaults(cfg)

			registry, _, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()

			// The Node runtime's project root must match cfg.ProjectRoot
			found := false
			for _, rt := range registry.Runtimes() {
				if nr, ok := rt.(interface{ ProjectRoot() string }); ok {
					found = true
					Expect(nr.ProjectRoot()).To(Equal(projectDir),
						"Node runtime project root must equal cfg.ProjectRoot — "+
							"when -r is used, the Node subprocess must run from the "+
							"project directory for correct node_modules/ resolution (issue #439)")
				}
			}
			Expect(found).To(BeTrue(),
				"at least one runtime must implement ProjectRoot() — "+
					"if false, the Node plugin was not loaded (Node not in PATH, "+
					"plugin classified as QuickJS, or eval failed silently)")
		})
	})

	// ── onContentTransformed page object payload (issue #448) ───────
	// onContentTransformed must receive a page object with html, toc,
	// path, url, and frontMatter — not just an HTML string.
	// Plugins can mutate toc and frontMatter before layout rendering.

	Describe("onContentTransformed page object payload (issue #448)", func() {
		It("onContentTransformed receives page object with toc and frontMatter", func() {
			cfg := &config.Config{
				Title:   "Hook Payload Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md": "---\ntitle: About\nlayout: default\n---\n## Section One\n\nContent here.\n\n## Section Two\n\nMore content.",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"plugins/toc-check.js": "export default function(alloy) {\n  alloy.hook('onContentTransformed', (page) => {\n    if (typeof page === 'string') throw new Error('payload must be object, got string');\n    if (!page.html) throw new Error('page.html missing');\n    if (!page.path) throw new Error('page.path missing');\n    if (!page.frontMatter) throw new Error('page.frontMatter missing');\n    return page;\n  });\n}",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"onContentTransformed must receive a page object, not a string — "+
					"if this fails with 'payload must be object, got string', "+
					"the hook still sends string(page.RenderedBody) instead of "+
					"the page object {html, toc, path, url, frontMatter} (issue #448)")
			Expect(result).NotTo(BeNil())
		})

		It("onContentTransformed can mutate page.toc", func() {
			cfg := &config.Config{
				Title:   "TOC Mutation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.html": "---\ntitle: Index\nlayout: default\n---\n<h2 id=\"custom\">Custom Heading</h2>\n<p>No goldmark TOC for HTML content.</p>",
				"layouts/default.liquid": "<html><body>{% for entry in page.toc %}<a href=\"#{{ entry.id }}\">{{ entry.text }}</a>{% endfor %}{{ content }}</body></html>",
				"plugins/toc-builder.js": "export default function(alloy) {\n  alloy.hook('onContentTransformed', (page) => {\n    if (!page.toc || page.toc.length === 0) {\n      page.toc = [{id: 'custom', text: 'Custom Heading', level: 2}];\n    }\n    return page;\n  });\n}",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.html"]
			Expect(html).To(ContainSubstring("Custom Heading</a>"),
				"plugin-built TOC must be available in layout via page.toc — "+
					"the onContentTransformed hook must be able to set page.toc "+
					"for non-markdown pages that don't go through goldmark (issue #448)")
		})
	})

	// ── SetSiteData pipeline wiring (issue #339) ────────────────────
	// Build() must call rt.SetSiteData(siteData) for each plugin runtime
	// after data loading so alloy.data is available in plugins.

	Describe("SetSiteData pipeline wiring", func() {
		It("plugin filter can access site.data via alloy.data during build", func() {
			cfg := &config.Config{
				Title:   "SiteData Wiring Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":        "---\ntitle: Home\nlayout: default\n---\n{{ \"space\" | tokenType }}",
				"data/tokens.json":        `{"space":{"type":"dimension","value":"16px"}}`,
				"plugins/token-reader.js": "export default function(alloy) { alloy.filter('tokenType', function(name) { return alloy.data.tokens[name].type; }); }",
				"layouts/default.liquid":  "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("dimension"),
				"plugin filter must access alloy.data.tokens.space.type — "+
					"proves SetSiteData is called in the pipeline after data loading")
		})
	})

	// ── Template tags in <code> not escaped for HTML content (#352) ─
	// escapeTemplateTagsInCode must only run on .md files, not .html.

	Describe("Template tags in code elements", func() {
		It("HTML content preserves Liquid expressions inside <code>", func() {
			cfg := &config.Config{
				Title:   "Code Escape Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/tokens.html": "---\ntitle: Tokens\nlayout: default\n---\n{% assign val = \"4px\" %}\n<td><code>{{ val }}</code></td>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["tokens.html"]
			Expect(html).To(ContainSubstring("<code>4px</code>"),
				"Liquid expressions inside <code> in .html files must be interpolated — "+
					"not entity-encoded. escapeTemplateTagsInCode must only run on .md files")
			Expect(html).NotTo(ContainSubstring("&#123;"),
				"template tags must NOT be entity-encoded in .html content")
		})

		It("Liquid content preserves Liquid expressions inside <code>", func() {
			cfg := &config.Config{
				Title:   "Code Escape Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Content: config.ContentConfig{Formats: []string{"liquid"}},
			}
			contentMap := map[string]string{
				"content/tokens.liquid": "---\ntitle: Tokens\nlayout: default\n---\n{% assign val = \"4px\" %}\n<td><code>{{ val }}</code></td>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["tokens.liquid"]
			Expect(html).To(ContainSubstring("<code>4px</code>"),
				"Liquid expressions inside <code> in .liquid files must be interpolated — "+
					"not entity-encoded. escapeTemplateTagsInCode must only run on .md files")
			Expect(html).NotTo(ContainSubstring("&#123;"),
				"template tags must NOT be entity-encoded in .liquid content")
		})

		It("markdown content still escapes template tags in inline code", func() {
			cfg := &config.Config{
				Title:   "Code Escape Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/example.md":     "---\ntitle: Example\nlayout: default\n---\nUse `{{ page.title }}` in templates.",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["example.md"]
			Expect(html).To(ContainSubstring("&#123;"),
				"template tags inside inline code in .md files must be escaped — "+
					"inline code should display template syntax literally")
		})
	})

	// ── {% inline %} pipeline wiring (issue #295) ──────────────────
	// RegisterInlineTag must be called in createEngine() so the tag
	// works in actual builds, not just unit tests.

	Describe("Inline tag pipeline wiring", func() {
		It("{% inline %} resolves and inlines files through BuildWithContent", func() {
			cfg := &config.Config{
				Title:   "Inline Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about/index.md":    "---\ntitle: About\n---\n# About\n\n{% inline \"./diagram.svg\" %}",
				"content/about/diagram.svg": `<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build with {% inline %} must not fail with 'unknown tag' — "+
					"RegisterInlineTag must be called in createEngine()")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["about/index.md"]
			Expect(html).To(ContainSubstring("<svg"),
				"{% inline %} must resolve and insert the SVG content through the build pipeline")
			Expect(html).To(ContainSubstring(`circle r="10"`),
				"inlined SVG content must be present in the rendered output")
		})
	})

	// ── Gotemplate layout rendering (issue #385) ────────────────────
	// Pages using gotemplate engine with layout: "default" must resolve
	// layouts/default.html and render through it. Regression: gotemplate
	// layouts stopped being applied in CLI builds.

	Describe("Gotemplate layout rendering (issue #385)", func() {
		It("gotemplate engine applies layout from .html file", func() {
			cfg := &config.Config{
				Title:   "Go Template Layout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"content/about.md":    "---\ntitle: About\nlayout: default\n---\n# About\n\nThis uses Go templates.",
				"layouts/default.html": `<!DOCTYPE html><html><head><title>{{ .page.title }}</title></head><body><main>{{ .content }}</main></body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["about.md"]
			Expect(html).NotTo(BeEmpty(),
				"about page must render")
			Expect(html).To(ContainSubstring("<!DOCTYPE html>"),
				"gotemplate layout must wrap content with HTML document — "+
					"if missing, layout resolution failed for .html files")
			Expect(html).To(ContainSubstring("<title>About</title>"),
				"layout must have access to .page.title from front matter")
			Expect(html).To(ContainSubstring("<main>"),
				"layout must render the <main> wrapper")
			Expect(html).To(ContainSubstring("About"),
				"rendered markdown content must appear inside the layout")
		})

		It("gotemplate engine renders page.title and site.title in layout", func() {
			cfg := &config.Config{
				Title:   "My Site",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\n# Welcome",
				"layouts/default.html": `<html><head><title>{{ .page.title }} - {{ .site.title }}</title></head><body>{{ .content }}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty(),
				"index page must render")
			Expect(html).To(ContainSubstring("<title>Home - My Site</title>"),
				"gotemplate layout must resolve both .page.title and .site.title")
		})
	})

	// ── Go template engine with JSON data (issue #458) ─────────────
	// *ordered.Map is a struct — Go templates can't use dot-notation or
	// {{ range }} on it directly. FuncMap helpers bridge the gap:
	// oget for key access, orange for ordered iteration.

	Describe("Go template engine with JSON ordered data (issue #458)", func() {
		It("gotemplate accesses JSON data via oget function", func() {
			cfg := &config.Config{
				Title:     "GoTemplate JSON Test",
				BaseURL:   "https://example.com",
				Build:     config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"data/colors.json":     `{"white":"#fff","black":"#000"}`,
				"content/index.md":     "---\ntitle: Colors\nlayout: default\n---\n# Colors",
				"layouts/default.html": `<html><body><span class="color">{{ oget .site.data.colors "white" }}</span>{{ .content }}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"gotemplate build with JSON data must not error — "+
					"oget must be registered as a FuncMap helper that calls "+
					"ordered.Map.Get() for key-based access (issue #458)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("#fff"),
				"oget must return the value for the key — "+
					"{{ oget .site.data.colors \"white\" }} must resolve to #fff")
		})

		It("gotemplate iterates JSON data in insertion order via orange", func() {
			cfg := &config.Config{
				Title:     "GoTemplate JSON Order Test",
				BaseURL:   "https://example.com",
				Build:     config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"data/colors.json": `{"white":"#fff","black":"#000","accent":"#e00","brand":"#ee0","surface":"#f0f"}`,
				"content/index.md": "---\ntitle: Colors\nlayout: default\n---\n# Colors",
				"layouts/default.html": `<html><body>{{ range orange .site.data.colors }}<span>{{ .Key }}</span>{{ end }}{{ .content }}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"gotemplate range over JSON data must not error — "+
					"orange must be registered as a FuncMap helper that returns "+
					"[]ordered.KVPair for ordered iteration (issue #458)")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]

			whiteIdx := strings.Index(html, "<span>white</span>")
			blackIdx := strings.Index(html, "<span>black</span>")
			accentIdx := strings.Index(html, "<span>accent</span>")

			Expect(whiteIdx).To(BeNumerically(">=", 0),
				"white must appear in output")
			Expect(blackIdx).To(BeNumerically(">", whiteIdx),
				"black must appear after white — JSON insertion order")
			Expect(accentIdx).To(BeNumerically(">", blackIdx),
				"accent must appear after black — "+
					"{{ range orange .site.data.colors }} must iterate in JSON "+
					"insertion order (issue #458)")
		})
	})

	// ── Layout resolution diagnostics (issue #385) ────────────────────
	// When a page explicitly requests a layout via front matter but the
	// layout file doesn't exist, the build must log a warning — not
	// silently produce layoutless output.

	Describe("Layout resolution diagnostics (issue #385)", func() {
		It("page with explicit layout but missing file still renders (without layout)", func() {
			cfg := &config.Config{
				Title:   "Missing Layout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md": "---\ntitle: About\nlayout: nonexistent\n---\n# About",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"missing layout must not cause build failure — page renders without layout")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1))

			html := result.RenderedContent["about.md"]
			Expect(html).NotTo(BeEmpty(),
				"page must still render even without layout")
			Expect(html).To(ContainSubstring("About"),
				"markdown content must be present even without layout wrapping")
		})

		It("Build defaults ProjectRoot to cwd when empty", func() {
			cfg := &config.Config{
				Title:   "No Root Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1),
				"Build must work even when initial ProjectRoot is empty — "+
					"BuildWithContent sets ProjectRoot to tmpDir, but Build() should "+
					"also default to cwd when ProjectRoot is empty")
		})
	})

	// ── Layout chaining (issue #276) ────────────────────────────────
	// Layout files can reference a parent layout via front matter.
	// The pipeline renders inside-out: content → child → parent → root.
	// Front matter in layout files must be stripped, not output as text.

	Describe("Layout chaining", func() {
		It("renders content through a chain of layouts", func() {
			cfg := &config.Config{
				Title:   "Chain Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/page.md":          "---\ntitle: Test\nlayout: has-toc\n---\n# Hello",
				"layouts/has-toc.liquid":   "---\nlayout: \"base\"\n---\n<div class=\"toc\">{{ content }}</div>",
				"layouts/base.liquid":      "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(Equal(1))

			html := result.RenderedContent["page.md"]
			Expect(html).To(ContainSubstring("<html>"),
				"output must include root layout (base.liquid) wrapper")
			Expect(html).To(ContainSubstring("<div class=\"toc\">"),
				"output must include middle layout (has-toc.liquid) wrapper")
			Expect(html).To(ContainSubstring("Hello"),
				"output must include the page content")
			Expect(html).NotTo(ContainSubstring("---"),
				"layout front matter must be stripped — not output as literal text")
			Expect(html).NotTo(ContainSubstring("layout:"),
				"layout: directive must not appear in rendered output")
		})

		It("layout front matter is not rendered as literal text", func() {
			cfg := &config.Config{
				Title:   "FrontMatter Strip Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/page.md":         "---\ntitle: Test\nlayout: child\n---\nContent here",
				"layouts/child.liquid":    "---\nlayout: \"parent\"\n---\n<main>{{ content }}</main>",
				"layouts/parent.liquid":   "<html>{{ content }}</html>",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["page.md"]
			Expect(html).NotTo(ContainSubstring("layout: \"parent\""),
				"layout front matter must be stripped before rendering — "+
					"this is the bug reported in #276")
		})
	})

	// ── BuildOptions: SkipSSR (issue #264) ──────────────────────────
	// alloy dev always skips SSR. Build() accepts BuildOptions with
	// SkipSSR to skip Phase 2 regardless of ssr: config.

	Describe("BuildOptions SkipSSR", func() {
		It("SkipSSR=true skips Phase 2 even when SSR is configured", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"Build with SkipSSR=true must skip Phase 2 entirely — "+
					"this is how alloy dev avoids SSR overhead regardless of config")
		})

		It("SkipSSR=false with SSR config runs Phase 2 normally", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg, pipeline.BuildOptions{SkipSSR: false})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeFalse(),
				"Build with SkipSSR=false must run Phase 2 when SSR is configured — "+
					"this is the alloy build / alloy serve path")
		})

		It("no BuildOptions runs Phase 2 when SSR is configured", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "cat"},
				Build: config.BuildConfig{Output: "_site"},
			}
			// No opts — existing callers must continue to work
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeFalse(),
				"Build without options must run Phase 2 when SSR is configured — "+
					"backward compatible with existing alloy build behavior")
		})

		It("BuildIncremental respects SkipSSR", func() {
			cfg := &config.Config{
				Title:   "SSR Incremental",
				BaseURL: "https://example.com",
				SSR:     &config.SSRConfig{Command: "cat"},
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n<ds-card>Hello</ds-card>",
			}

			result, err := pipeline.BuildIncremental(
				cfg, contentMap, nil, nil,
				pipeline.BuildOptions{SkipSSR: true},
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.SSRSkipped).To(BeTrue(),
				"BuildIncremental with SkipSSR=true must skip Phase 2 — "+
					"alloy dev incremental rebuilds never run SSR")
			Expect(result.SSRPagesRendered).To(Equal(0),
				"no pages should go through SSR when SkipSSR is true")
		})
	})

	// ── Progress reporter wiring (issue #255) ─────────────────────
	// The ProgressReporter must be called by both Build() and
	// BuildIncremental(). This is critical for alloy serve where
	// users watch the terminal waiting for the server to start.

	Describe("Progress reporter", func() {
		AfterEach(func() {
			pipeline.SetReporter(nil)
		})

		It("Build calls StartStage, Update, EndStage, and Summary", func() {
			spy := &spyReporter{}
			pipeline.SetReporter(spy)

			cfg := &config.Config{
				Title:   "Progress Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			result, err := pipeline.Build(cfg)
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

		It("Build with content calls Update for each page", func() {
			spy := &spyReporter{}
			pipeline.SetReporter(spy)

			cfg := &config.Config{
				Title:   "Progress Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			content := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
			}
			result, err := pipeline.BuildWithContent(cfg, content)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(spy.updates).To(HaveLen(result.PageCount),
				"Build must call Update once per page rendered — "+
					"each Update drives the progress bar forward")
		})

		It("BuildIncremental calls only Summary on the reporter", func() {
			spy := &spyReporter{}
			pipeline.SetReporter(spy)

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

			result, err := pipeline.BuildIncremental(cfg, contentMap, previousCache, changedFiles)
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
			pipeline.SetReporter(nil)

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
})
