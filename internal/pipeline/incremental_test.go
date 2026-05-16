package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Build Pipeline", func() {
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

		It("writes rendered pages to the output directory (issue #581)", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutDir := filepath.Join(tmpDir, "layouts")
			outputDir := filepath.Join(tmpDir, "_site")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
				[]byte("{{ content }}"), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\n---\nOriginal content"), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Disk Write Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}

			initialResult, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(initialResult.PageCount).To(BeNumerically(">=", 1))

			initialHTML, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(initialHTML)).To(ContainSubstring("Original content"))

			previousCache := cache.New()
			previousCache.SetHash("index.md", cache.HashContent(
				[]byte("---\ntitle: Home\n---\nOriginal content")))

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\n---\nUpdated content"), 0644)).To(Succeed())

			result, err := pipeline.BuildIncremental(cfg, nil, previousCache,
				[]string{"content/index.md"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.PageCount).To(Equal(1),
				"incremental build must render the changed page")

			updatedHTML, err := os.ReadFile(filepath.Join(outputDir, "index.html"))
			Expect(err).NotTo(HaveOccurred(),
				"BuildIncremental must write rendered pages to the output directory — "+
					"currently renders to RenderedContent in memory but never calls "+
					"output.WriteFile, so _site/ remains stale (issue #581)")
			Expect(string(updatedHTML)).To(ContainSubstring("Updated content"),
				"output file must contain the updated content after incremental rebuild — "+
					"if this shows 'Original content', BuildIncremental rendered correctly "+
					"but did not write the result to disk (issue #581)")
		})

		It("wraps content in layout after incremental rebuild (issue #628)", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutDir := filepath.Join(tmpDir, "layouts")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
				[]byte("<html><head><title>Site</title></head><body>{{ content }}</body></html>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Welcome"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Layout Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: filepath.Join(tmpDir, "_site")},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}

			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			html := result.RenderedContent["index.md"]
			Expect(html).To(ContainSubstring("<html>"),
				"incremental rebuild must apply layout wrapping — "+
					"BuildIncremental currently calls renderPages (Pass 1) but "+
					"skips layout resolution and renderPageThroughLayouts (Pass 2), "+
					"producing raw content without any layout (issue #628)")
			Expect(html).To(ContainSubstring("<head>"),
				"layout <head> section must be present after incremental rebuild")
			Expect(html).To(ContainSubstring("Welcome"),
				"page content must be preserved inside the layout wrapper")
		})

		It("applies layout chain in incremental rebuild (issue #628)", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutDir := filepath.Join(tmpDir, "layouts")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutDir, "base.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(layoutDir, "has-toc.liquid"),
				[]byte("---\nlayout: \"base\"\n---\n<div class=\"toc\">{{ content }}</div>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "guide.md"),
				[]byte("---\ntitle: Guide\nlayout: has-toc\n---\n# Getting Started"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Chain Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: filepath.Join(tmpDir, "_site")},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}

			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			html := result.RenderedContent["guide.md"]
			Expect(html).To(ContainSubstring("<html>"),
				"root layout (base) must be applied in incremental rebuild — "+
					"layout chain resolution requires Pass 2 which BuildIncremental "+
					"currently skips entirely (issue #628)")
			Expect(html).To(ContainSubstring("<div class=\"toc\">"),
				"intermediate layout (has-toc) must be applied — "+
					"the full chain has-toc → base must resolve the same as Build()")
			Expect(html).To(ContainSubstring("Getting Started"),
				"page content must be preserved through the layout chain")
		})

		It("produces same layout structure as full build (issue #628)", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutDir := filepath.Join(tmpDir, "layouts")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
				[]byte("<!DOCTYPE html><html><head><title>{{ page.title }}</title></head><body>{{ content }}</body></html>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
				[]byte("---\ntitle: About\nlayout: default\n---\n# About Us"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Parity Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: filepath.Join(tmpDir, "_site")},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}

			fullResult, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			fullHTML := fullResult.RenderedContent["about.md"]

			incrResult, err := pipeline.BuildIncremental(cfg, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			incrHTML := incrResult.RenderedContent["about.md"]

			Expect(incrHTML).To(ContainSubstring("<!DOCTYPE html>"),
				"incremental rebuild must preserve DOCTYPE from layout — "+
					"full build produces DOCTYPE but incremental skips layout "+
					"wrapping entirely (issue #628)")
			Expect(incrHTML).To(ContainSubstring("<head>"),
				"incremental rebuild must include <head> from layout")
			Expect(fullHTML).To(ContainSubstring("<!DOCTYPE html>"),
				"sanity: full build must include DOCTYPE (test is invalid otherwise)")
			Expect(fullHTML).To(ContainSubstring("<head>"),
				"sanity: full build must include <head> (test is invalid otherwise)")
		})

		It("skips layout wrapping when page has layout: false (issue #633)", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutDir := filepath.Join(tmpDir, "layouts")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
				[]byte("<html><head><title>Site</title></head><body>{{ content }}</body></html>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "embed.md"),
				[]byte("---\ntitle: Widget\nlayout: false\n---\n<div class=\"widget\">Embeddable</div>"),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Layout False Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: filepath.Join(tmpDir, "_site")},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}

			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			html := result.RenderedContent["embed.md"]
			Expect(html).To(ContainSubstring("Embeddable"),
				"page content must still be rendered even with layout: false")
			Expect(html).NotTo(ContainSubstring("<html>"),
				"page with layout: false must NOT be wrapped in a layout — "+
					"BuildIncremental must respect the layout: false opt-out "+
					"the same way Build does (issue #633)")
			Expect(html).NotTo(ContainSubstring("<head>"),
				"no layout markup should appear when layout: false is set")
		})

		It("returns updated cache with rendered page hashes (issue #639)", func() {
			cfg := &config.Config{
				Title:   "Cache Return Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/index.md": "---\ntitle: Home\n---\n# Home",
				"content/about.md": "---\ntitle: About\n---\n# About",
			}

			result, err := pipeline.BuildIncremental(cfg, contentMap, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			Expect(result.Cache).NotTo(BeNil(),
				"BuildIncremental must return an in-memory cache on result.Cache — "+
					"without this, dev.go cannot maintain cache state across "+
					"incremental rebuilds and falls back to stale disk reads (issue #639)")

			Expect(result.Cache.ShouldSkipFile("index.md", []byte("---\ntitle: Home\n---\n# Home"))).To(BeTrue(),
				"returned cache must contain hashes for rendered pages — "+
					"ShouldSkipFile must return true for content matching what was just built")
		})

		It("returned cache prevents stale skips on content revert (issue #639)", func() {
			cfg := &config.Config{
				Title:   "Revert Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}

			contentA := "---\ntitle: Home\n---\n# Original"
			contentB := "---\ntitle: Home\n---\n# Modified"

			// Step 1: initial build with content A
			contentMap := map[string]string{
				"content/index.md": contentA,
			}
			result1, err := pipeline.BuildIncremental(cfg, contentMap, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.Cache).NotTo(BeNil(),
				"first build must return a cache (issue #639)")

			// Step 2: content changes to B — rebuild with cache from step 1
			contentMap["content/index.md"] = contentB
			result2, err := pipeline.BuildIncremental(cfg, contentMap, result1.Cache, []string{"content/index.md"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.PageCount).To(Equal(1),
				"changed content must trigger a rebuild")
			Expect(result2.Cache).NotTo(BeNil(),
				"second build must return an updated cache (issue #639)")

			// Step 3: content reverts to A — rebuild with cache from step 2
			// This is the bug: with stale disk cache, hash A matches the
			// initial cache and the page is incorrectly skipped
			contentMap["content/index.md"] = contentA
			result3, err := pipeline.BuildIncremental(cfg, contentMap, result2.Cache, []string{"content/index.md"})
			Expect(err).NotTo(HaveOccurred())
			Expect(result3.PageCount).To(Equal(1),
				"reverting content must trigger a rebuild — the cache from step 2 "+
					"has hash B, not hash A, so the page must not be skipped. "+
					"Without in-memory cache propagation, dev.go reloads the stale "+
					"initial cache from disk where hash A matches, causing an "+
					"incorrect skip that leaves output stale (issue #639)")
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

	// ── SSR Phase 2 with paginated pages (issue #522) ───────────────
	// Paginated virtual pages share RelPath but have unique URLs.
	// SSR Phase 2 must use renderedContentKey (URL for paginated pages)
	// instead of RelPath to avoid map key collisions.

	Describe("SSR Phase 2 with paginated pages (issue #522)", func() {
		It("each paginated page gets distinct SSR output", func() {
			cfg := &config.Config{
				Title:   "SSR Pagination Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				SSR:     &config.SSRConfig{Command: "cat"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"},{"name":"Charlie","slug":"charlie"}]`,
				"content/team.md": "---\ntitle: \"{{ member.name }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<ds-card>{{ member.name }}</ds-card>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// All 3 paginated pages must have distinct content after SSR.
			// Bug: Build() uses page.RelPath as SSR key — all 3 pages share
			// RelPath "team.md", so only the last survives. All 3 get
			// Charlie's content instead of their own.
			aliceHTML := result.RenderedContent["/team/alice/"]
			bobHTML := result.RenderedContent["/team/bob/"]
			charlieHTML := result.RenderedContent["/team/charlie/"]

			Expect(aliceHTML).To(ContainSubstring("Alice"),
				"Alice's page must contain Alice's content after SSR — "+
					"not Charlie's (issue #522)")
			Expect(bobHTML).To(ContainSubstring("Bob"),
				"Bob's page must contain Bob's content after SSR — "+
					"not Charlie's (issue #522)")
			Expect(charlieHTML).To(ContainSubstring("Charlie"),
				"Charlie's page must contain Charlie's content after SSR")

			// Each page must be distinct — no duplication
			Expect(aliceHTML).NotTo(Equal(bobHTML),
				"paginated pages must not all contain the same SSR output — "+
					"RelPath key collision causes all pages to share last page's content")
		})

		It("non-paginated pages render correctly alongside paginated SSR pages", func() {
			cfg := &config.Config{
				Title:   "SSR Mixed Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				SSR:     &config.SSRConfig{Command: "cat"},
			}
			contentMap := map[string]string{
				"data/items.json": `[{"name":"One","slug":"one"},{"name":"Two","slug":"two"}]`,
				"content/index.md": "---\ntitle: Home\nlayout: default\n---\n<ds-hero>Welcome</ds-hero>",
				"content/items.md": "---\ntitle: \"{{ item.name }}\"\nlayout: default\npagination:\n  data: site.data.items\n  perPage: 1\n  as: item\npermalink: \"/items/{{ item.slug }}/\"\n---\n<ds-card>{{ item.name }}</ds-card>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Non-paginated page must still render correctly
			homeHTML := result.RenderedContent["index.md"]
			Expect(homeHTML).To(ContainSubstring("Welcome"),
				"non-paginated page must render correctly when paginated pages are also present")

			// Paginated pages must each have their own content
			oneHTML := result.RenderedContent["/items/one/"]
			twoHTML := result.RenderedContent["/items/two/"]
			Expect(oneHTML).To(ContainSubstring("One"),
				"first paginated page must have its own content after SSR (issue #522)")
			Expect(twoHTML).To(ContainSubstring("Two"),
				"second paginated page must have its own content after SSR (issue #522)")
		})
	})

	// ── Phase 1 → Phase 2 handoff (§2) ──────────────────────────────
	// Per spec §6: Phase 2 operates in memory. For each page with custom
	// elements, Alloy pipes the full page HTML to ssr.command via stdin.
	// Mode "exec" (default): one process per page. Mode "stream": persistent
	// process with NUL-delimited messages. The SSR engine handles all
	// component rendering internally.
})
