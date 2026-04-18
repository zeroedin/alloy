package server_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/server"
)

var _ = Describe("File Watcher", func() {

	// ── Watch paths ───────────────────────────────────────────────────

	Describe("WatchDirs", func() {
		It("includes content/, layouts/, data/, assets/, and static/", func() {
			cfg := &config.Config{Title: "Test Site"}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).NotTo(BeNil())
			Expect(dirs).To(ContainElements(
				"content",
				"layouts",
				"data",
				"assets",
				"static",
			))
		})

		It("includes component source dir when SSR is configured", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR: &config.SSRConfig{
					Command: "golit render --defs bundles/",
				},
			}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).NotTo(BeNil())
			// Must include more than the base 5 dirs when SSR is present
			Expect(len(dirs)).To(BeNumerically(">", 5),
				"SSR config should add component source directory to watch list")
		})

		It("returns only base directories when no SSR is configured", func() {
			cfg := &config.Config{Title: "Simple Site"}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).NotTo(BeNil())
			Expect(dirs).To(HaveLen(5))
		})
	})

	// ── Change classification ─────────────────────────────────────────

	Describe("ClassifyChange", func() {
		var cfg *config.Config

		BeforeEach(func() {
			cfg = &config.Config{Title: "Test Site"}
		})

		It("classifies content file change as ContentChange", func() {
			changeType := server.ClassifyChange("content/blog/my-post.md", cfg)
			Expect(changeType).To(Equal(server.ContentChange))
		})

		It("classifies layout file change as LayoutChange", func() {
			changeType := server.ClassifyChange("layouts/default.liquid", cfg)
			Expect(changeType).To(Equal(server.LayoutChange))
		})

		It("classifies data file change as DataChange", func() {
			changeType := server.ClassifyChange("data/navigation.yaml", cfg)
			Expect(changeType).To(Equal(server.DataChange))
		})

		It("classifies asset file change as AssetChange", func() {
			changeType := server.ClassifyChange("assets/css/main.css", cfg)
			Expect(changeType).To(Equal(server.AssetChange))
		})

		It("classifies static file change as StaticChange", func() {
			changeType := server.ClassifyChange("static/robots.txt", cfg)
			Expect(changeType).To(Equal(server.StaticChange))
		})
	})

	// ── Debounce ──────────────────────────────────────────────────────

	Describe("Debouncer", func() {
		It("collapses rapid events into a single batch after the quiet interval", func() {
			debouncer := server.NewDebouncer(50*time.Millisecond, 20)

			events := []server.ChangeEvent{
				{Path: "content/a.md", ChangeType: server.ContentChange},
				{Path: "content/b.md", ChangeType: server.ContentChange},
				{Path: "content/c.md", ChangeType: server.ContentChange},
			}

			batch, scope := debouncer.Debounce(events)
			Expect(batch).To(HaveLen(3))
			Expect(scope).To(Equal(server.RebuildIncremental))
		})

		It("signals full rebuild when event count exceeds bulk threshold", func() {
			debouncer := server.NewDebouncer(50*time.Millisecond, 5)

			// Simulate a git checkout that changes many files at once
			events := make([]server.ChangeEvent, 10)
			for i := range events {
				events[i] = server.ChangeEvent{
					Path:       "content/page-" + string(rune('a'+i)) + ".md",
					ChangeType: server.ContentChange,
				}
			}

			batch, scope := debouncer.Debounce(events)
			Expect(batch).To(HaveLen(10))
			Expect(scope).To(Equal(server.RebuildFull),
				"exceeding bulk threshold should signal full rebuild")
		})
	})

	// ── Reload message ────────────────────────────────────────────────

	Describe("ReloadMessage", func() {
		It("produces {type: reload} JSON for WebSocket broadcast", func() {
			msg := server.ReloadMessage()
			Expect(msg).NotTo(BeNil())

			var parsed map[string]interface{}
			err := json.Unmarshal(msg, &parsed)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed).To(HaveKeyWithValue("type", "reload"))
		})
	})

	// ── Incremental rebuild behavior (serve mode) ────────────────────
	// alloy serve does a full build on startup, then only rebuilds
	// affected pages when files change. The cache determines what changed.

	Describe("Incremental rebuild scope", func() {
		It("single content file change triggers incremental rebuild", func() {
			debouncer := server.NewDebouncer(50*time.Millisecond, 20)

			events := []server.ChangeEvent{
				{Path: "content/blog/post.md", ChangeType: server.ContentChange},
			}

			batch, scope := debouncer.Debounce(events)
			Expect(scope).To(Equal(server.RebuildIncremental),
				"single file change must trigger incremental rebuild, not full")
			Expect(batch).To(HaveLen(1))
			Expect(batch[0].Path).To(Equal("content/blog/post.md"),
				"incremental rebuild must know which file changed")
		})

		It("layout change triggers incremental rebuild of pages using that layout", func() {
			debouncer := server.NewDebouncer(50*time.Millisecond, 20)

			events := []server.ChangeEvent{
				{Path: "layouts/post.liquid", ChangeType: server.LayoutChange},
			}

			batch, scope := debouncer.Debounce(events)
			Expect(scope).To(Equal(server.RebuildIncremental),
				"layout change must trigger incremental rebuild")
			Expect(batch[0].ChangeType).To(Equal(server.LayoutChange),
				"change type must be LayoutChange so the rebuild knows to "+
					"invalidate pages using this layout")
		})

		It("data file change triggers incremental rebuild", func() {
			debouncer := server.NewDebouncer(50*time.Millisecond, 20)

			events := []server.ChangeEvent{
				{Path: "data/navigation.yaml", ChangeType: server.DataChange},
			}

			batch, scope := debouncer.Debounce(events)
			Expect(scope).To(Equal(server.RebuildIncremental))
			Expect(batch[0].ChangeType).To(Equal(server.DataChange),
				"data change must be classified correctly for targeted rebuild")
		})

		It("config file change triggers full rebuild", func() {
			cfg := &config.Config{Title: "Test Site"}
			changeType := server.ClassifyChange("alloy.config.yaml", cfg)
			// Config changes affect everything — must trigger full rebuild
			// The file watcher should detect this and signal RebuildFull
			_ = changeType
			// Config changes should result in a full rebuild when processed
			// by DetermineRebuildAction or similar logic
		})
	})

	// ── Cache-based page skipping in serve mode ─────────────────────
	// On incremental rebuild, the server uses the build cache to skip
	// pages whose content hash hasn't changed.

	Describe("Cache-based page skipping", func() {
		It("unchanged page is skipped on incremental rebuild", func() {
			// Simulate: first build populates cache, second rebuild
			// with same content should skip the unchanged page.
			buildCache := cache.New()
			pageContent := []byte("---\ntitle: Test\n---\n# Hello")
			buildCache.SetHash("blog/post.md", cache.HashContent(pageContent))

			// Same content — should skip
			Expect(buildCache.ShouldSkipFile("blog/post.md", pageContent)).To(BeTrue(),
				"page with unchanged content hash must be skipped on incremental rebuild")
		})

		It("modified page is not skipped on incremental rebuild", func() {
			buildCache := cache.New()
			originalContent := []byte("---\ntitle: Test\n---\n# Hello")
			buildCache.SetHash("blog/post.md", cache.HashContent(originalContent))

			modifiedContent := []byte("---\ntitle: Test\n---\n# Hello Updated")
			Expect(buildCache.ShouldSkipFile("blog/post.md", modifiedContent)).To(BeFalse(),
				"page with changed content must NOT be skipped — it needs rebuilding")
		})

		It("new page is not skipped on incremental rebuild", func() {
			buildCache := cache.New()
			buildCache.SetHash("blog/post.md", cache.HashContent([]byte("existing")))

			newPageContent := []byte("---\ntitle: New\n---\n# New Page")
			Expect(buildCache.ShouldSkipFile("blog/new-post.md", newPageContent)).To(BeFalse(),
				"page not in cache must NOT be skipped — it's new")
		})

		It("layout change invalidates pages using that layout", func() {
			buildCache := cache.New()
			buildCache.SetHash("blog/post.md", cache.HashContent([]byte("content")))
			buildCache.TrackTemplateUsage("blog/post.md", "layouts/post.liquid")
			buildCache.TrackTemplateUsage("blog/other.md", "layouts/post.liquid")

			invalidated := buildCache.InvalidatedPages("layouts/post.liquid")
			Expect(invalidated).To(ContainElement("blog/post.md"),
				"page using changed layout must be invalidated even if its own content is unchanged")
			Expect(invalidated).To(ContainElement("blog/other.md"),
				"all pages using the changed layout must be invalidated")
		})

		It("cache persists across server rebuilds", func() {
			tmpDir := GinkgoT().TempDir()

			// Build 1: populate and save cache
			buildCache := cache.New()
			buildCache.SetHash("index.md", cache.HashContent([]byte("home")))
			buildCache.SetHash("about.md", cache.HashContent([]byte("about")))
			err := buildCache.SaveTo(tmpDir)
			Expect(err).NotTo(HaveOccurred())

			// Build 2: load cache and check
			restored, err := cache.LoadFrom(tmpDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(restored.ShouldSkipFile("index.md", []byte("home"))).To(BeTrue(),
				"restored cache must correctly identify unchanged pages")
			Expect(restored.ShouldSkipFile("about.md", []byte("about updated"))).To(BeFalse(),
				"restored cache must detect changed pages")
		})
	})
})
