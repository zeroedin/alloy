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

	// ── Passthrough directory watching (issue #275) ──────────────────
	// Passthrough from: directories must be included in WatchDirs
	// so changes are detected and recopied during serve mode.

	Describe("Passthrough watching", func() {
		It("WatchDirs includes passthrough from: directories", func() {
			cfg := &config.Config{
				Title: "Passthrough Site",
				Passthrough: []config.PassthroughMapping{
					{From: "../design-system/dist/elements", To: "elements"},
					{From: "vendor/js", To: "js/vendor"},
				},
			}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).To(ContainElement("../design-system/dist/elements"),
				"passthrough from: directories must be included in WatchDirs — "+
					"without this, changes to passthrough sources are never detected")
			Expect(dirs).To(ContainElement("vendor/js"),
				"all passthrough from: directories must be watched, not just the first")
		})

		It("WatchDirs includes both base dirs and passthrough dirs", func() {
			cfg := &config.Config{
				Title: "Mixed Site",
				Passthrough: []config.PassthroughMapping{
					{From: "../shared/fonts", To: "assets/fonts"},
				},
			}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).To(ContainElements("content", "layouts", "data", "assets", "static"),
				"base directories must still be present")
			Expect(dirs).To(ContainElement("../shared/fonts"),
				"passthrough directories must be added alongside base directories")
			Expect(len(dirs)).To(Equal(6),
				"5 base dirs + 1 passthrough dir = 6 total")
		})

		It("ClassifyChange identifies passthrough file changes", func() {
			cfg := &config.Config{
				Title: "Passthrough Site",
				Passthrough: []config.PassthroughMapping{
					{From: "vendor/js", To: "js/vendor"},
				},
			}
			changeType := server.ClassifyChange("vendor/js/lib.min.js", cfg)
			Expect(changeType).To(Equal(server.PassthroughChange),
				"files under passthrough from: directories must be classified as PassthroughChange — "+
					"this triggers a targeted file recopy instead of a full pipeline rebuild")
		})

		It("ClassifyChange does not misclassify non-passthrough files", func() {
			cfg := &config.Config{
				Title: "Passthrough Site",
				Passthrough: []config.PassthroughMapping{
					{From: "vendor/js", To: "js/vendor"},
				},
			}
			changeType := server.ClassifyChange("content/index.md", cfg)
			Expect(changeType).To(Equal(server.ContentChange),
				"content files must not be classified as PassthroughChange")
		})
	})

	// ── Watch directory watching (issue #530) ────────────────────────
	// Watch from: directories must be included in WatchDirs and
	// classified as the declared type (content/layout/data), triggering
	// RebuildPipeline instead of RebuildRecopy.

	Describe("Watch directory watching (issue #530)", func() {
		It("WatchDirs includes watch from: directories", func() {
			cfg := &config.Config{
				Title: "Watch Site",
				Watch: []config.WatchMapping{
					{From: "elements", Type: "content"},
					{From: "shared-layouts", Type: "layout"},
				},
			}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).To(ContainElement("elements"),
				"watch from: directories must be in WatchDirs — "+
					"without this, changes to watched sources are never detected (issue #530)")
			Expect(dirs).To(ContainElement("shared-layouts"),
				"all watch from: directories must be watched (issue #530)")
		})

		It("WatchDirs includes both base dirs and watch dirs", func() {
			cfg := &config.Config{
				Title: "Mixed Site",
				Watch: []config.WatchMapping{{From: "elements", Type: "content"}},
			}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).To(ContainElements("content", "layouts", "data", "assets", "static"),
				"base directories must still be present (issue #530)")
			Expect(dirs).To(ContainElement("elements"),
				"watch directories must be added alongside base directories (issue #530)")
			Expect(len(dirs)).To(Equal(6),
				"5 base dirs + 1 watch dir = 6 total (issue #530)")
		})

		It("WatchDirs handles watch from: with glob pattern", func() {
			cfg := &config.Config{
				Title: "Glob Watch Site",
				Watch: []config.WatchMapping{
					{From: "elements/**/docs/*.md", Type: "content"},
				},
			}
			dirs := server.WatchDirs(cfg)
			Expect(dirs).To(ContainElement("elements"),
				"glob watch from: must add the glob root — "+
					"same pattern as passthrough glob handling (issue #530)")
		})

		It("ClassifyChange identifies watch content file as ContentChange", func() {
			cfg := &config.Config{
				Title: "Watch Site",
				Watch: []config.WatchMapping{{From: "elements", Type: "content"}},
			}
			changeType := server.ClassifyChange("elements/rh-button/docs/overview.md", cfg)
			Expect(changeType).To(Equal(server.ContentChange),
				"watch type: content must classify as ContentChange — "+
					"this triggers RebuildPipeline (issue #530)")
		})

		It("ClassifyChange identifies watch layout file as LayoutChange", func() {
			cfg := &config.Config{
				Title: "Watch Site",
				Watch: []config.WatchMapping{{From: "shared-layouts", Type: "layout"}},
			}
			changeType := server.ClassifyChange("shared-layouts/header.liquid", cfg)
			Expect(changeType).To(Equal(server.LayoutChange),
				"watch type: layout must classify as LayoutChange — "+
					"this triggers RebuildPipeline (issue #530)")
		})

		It("ClassifyChange identifies watch data file as DataChange", func() {
			cfg := &config.Config{
				Title: "Watch Site",
				Watch: []config.WatchMapping{{From: "external-data", Type: "data"}},
			}
			changeType := server.ClassifyChange("external-data/navigation.yaml", cfg)
			Expect(changeType).To(Equal(server.DataChange),
				"watch type: data must classify as DataChange — "+
					"this triggers RebuildPipeline (issue #530)")
		})

		It("ClassifyChange does not misclassify standard dirs as watch", func() {
			cfg := &config.Config{
				Title: "Watch Site",
				Watch: []config.WatchMapping{{From: "elements", Type: "content"}},
			}
			changeType := server.ClassifyChange("layouts/default.liquid", cfg)
			Expect(changeType).To(Equal(server.LayoutChange),
				"standard dirs must classify correctly — "+
					"watch dirs must not interfere with existing classification (issue #530)")
		})

		It("ClassifyChange prefers watch over passthrough for same directory", func() {
			cfg := &config.Config{
				Title: "Overlap Site",
				Watch:       []config.WatchMapping{{From: "elements", Type: "content"}},
				Passthrough: []config.PassthroughMapping{{From: "elements", To: "assets/elements"}},
			}
			changeType := server.ClassifyChange("elements/rh-button/docs/overview.md", cfg)
			Expect(changeType).To(Equal(server.ContentChange),
				"when a directory appears in both watch and passthrough, "+
					"watch must take precedence — watch triggers RebuildPipeline "+
					"while passthrough triggers RebuildRecopy (issue #530)")
		})

		It("watch type content triggers RebuildPipeline", func() {
			scope := server.RebuildScopeForChangeType(server.ContentChange)
			Expect(scope).To(Equal(server.RebuildPipeline),
				"ContentChange must trigger pipeline rebuild, not recopy (issue #530)")
		})

		It("watch type layout triggers RebuildPipeline", func() {
			scope := server.RebuildScopeForChangeType(server.LayoutChange)
			Expect(scope).To(Equal(server.RebuildPipeline),
				"LayoutChange must trigger pipeline rebuild (issue #530)")
		})

		It("watch type data triggers RebuildPipeline", func() {
			scope := server.RebuildScopeForChangeType(server.DataChange)
			Expect(scope).To(Equal(server.RebuildPipeline),
				"DataChange must trigger pipeline rebuild (issue #530)")
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

		It("layout change event is classified as incremental with LayoutChange type", func() {
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

		It("config file change is classified as ContentChange (triggers full rebuild via watcher)", func() {
			cfg := &config.Config{Title: "Test Site"}
			changeType := server.ClassifyChange("alloy.config.yaml", cfg)
			// Config file doesn't match any watched directory (content/,
			// layouts/, data/, assets/, static/), so ClassifyChange returns
			// ContentChange as the default. The file watcher layer detects
			// config changes separately and triggers a full rebuild.
			Expect(changeType).To(Equal(server.ContentChange),
				"config file must be classified (not silently ignored)")
		})
	})

	// ── Passthrough targeted recopy (issue #291) ────────────────────
	// On PassthroughChange, only the changed file is recopied to
	// _site/<to>/<relative-path> — not the entire directory.

	Describe("Passthrough targeted recopy", func() {
		It("RecopyPassthroughFile copies single file to correct output path", func() {
			cfg := &config.Config{
				Title: "Recopy Test",
				Build: config.BuildConfig{Output: "_site"},
				Passthrough: []config.PassthroughMapping{
					{From: "vendor/js", To: "js/vendor"},
				},
			}
			outputPath, err := server.RecopyPassthroughFile("vendor/js/lib.min.js", cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(outputPath).To(Equal("_site/js/vendor/lib.min.js"),
				"recopy must map vendor/js/lib.min.js → _site/js/vendor/lib.min.js — "+
					"relative path within from: is preserved under to:")
		})

		It("RecopyPassthroughFile handles nested subdirectories", func() {
			cfg := &config.Config{
				Title: "Recopy Test",
				Build: config.BuildConfig{Output: "_site"},
				Passthrough: []config.PassthroughMapping{
					{From: "../design-system/dist", To: "elements"},
				},
			}
			outputPath, err := server.RecopyPassthroughFile("../design-system/dist/components/card/card.js", cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(outputPath).To(Equal("_site/elements/components/card/card.js"),
				"nested subdirectory structure within from: must be preserved in output")
		})

		It("RecopyPassthroughFile returns error for unmatched path", func() {
			cfg := &config.Config{
				Title: "Recopy Test",
				Build: config.BuildConfig{Output: "_site"},
				Passthrough: []config.PassthroughMapping{
					{From: "vendor/js", To: "js/vendor"},
				},
			}
			_, err := server.RecopyPassthroughFile("content/index.md", cfg)
			Expect(err).To(HaveOccurred(),
				"RecopyPassthroughFile must error when path doesn't match any passthrough mapping")
		})

		It("RecopyPassthroughFile skips files matching exclude patterns (issue #547)", func() {
			cfg := &config.Config{
				Title: "Recopy Exclude Test",
				Build: config.BuildConfig{Output: "_site"},
				Passthrough: []config.PassthroughMapping{
					{From: "elements", To: "assets/elements", Exclude: []string{"*.html", "demo/"}},
				},
			}
			_, err := server.RecopyPassthroughFile("elements/rh-button/demo.html", cfg)
			Expect(err).To(HaveOccurred(),
				"RecopyPassthroughFile must reject files matching an exclude pattern — "+
					"*.html should match demo.html at any depth (issue #547)")

			_, err = server.RecopyPassthroughFile("elements/demo/example.js", cfg)
			Expect(err).To(HaveOccurred(),
				"RecopyPassthroughFile must reject files under an excluded directory — "+
					"demo/ should exclude the entire demo/ tree (issue #547)")

			outputPath, err := server.RecopyPassthroughFile("elements/rh-button/rh-button.js", cfg)
			Expect(err).NotTo(HaveOccurred(),
				"RecopyPassthroughFile must accept non-excluded files (issue #547)")
			Expect(outputPath).To(Equal("_site/assets/elements/rh-button/rh-button.js"),
				"non-excluded files must map to correct output path (issue #547)")
		})

		It("RecopyPassthroughFile with glob from rejects non-matching and excluded files (issue #547)", func() {
			cfg := &config.Config{
				Title: "Recopy Glob Test",
				Build: config.BuildConfig{Output: "_site"},
				Passthrough: []config.PassthroughMapping{
					{From: "elements/**/*.{js,css}", To: "assets/elements", Exclude: []string{"*.min.js"}},
				},
			}

			_, err := server.RecopyPassthroughFile("elements/rh-button/rh-button.html", cfg)
			Expect(err).To(HaveOccurred(),
				"RecopyPassthroughFile must reject files that don't match the from glob — "+
					".html does not match **/*.{js,css} (issue #547)")

			_, err = server.RecopyPassthroughFile("elements/rh-button/rh-button.min.js", cfg)
			Expect(err).To(HaveOccurred(),
				"RecopyPassthroughFile must reject files matching exclude even if they match the from glob — "+
					"*.min.js is excluded (issue #547)")

			outputPath, err := server.RecopyPassthroughFile("elements/rh-button/rh-button.js", cfg)
			Expect(err).NotTo(HaveOccurred(),
				"RecopyPassthroughFile must accept files matching glob and not excluded (issue #547)")
			Expect(outputPath).To(Equal("_site/assets/elements/rh-button/rh-button.js"),
				"glob from must preserve path relative to glob root in output (issue #547)")
		})
	})

	// ── Serve mode rebuild dispatch (issue #291) ─────────────────────
	// alloy serve must dispatch rebuilds by ChangeType, not just
	// do a full pipeline rebuild for everything.

	Describe("Rebuild dispatch by change type", func() {
		It("PassthroughChange does not trigger a full pipeline rebuild", func() {
			scope := server.RebuildScopeForChangeType(server.PassthroughChange)
			Expect(scope).To(Equal(server.RebuildRecopy),
				"PassthroughChange must trigger a targeted recopy, not a full pipeline rebuild")
		})

		It("ContentChange triggers a pipeline rebuild", func() {
			scope := server.RebuildScopeForChangeType(server.ContentChange)
			Expect(scope).To(Equal(server.RebuildPipeline),
				"ContentChange must trigger a pipeline rebuild")
		})

		It("StaticChange triggers a recopy", func() {
			scope := server.RebuildScopeForChangeType(server.StaticChange)
			Expect(scope).To(Equal(server.RebuildRecopy),
				"StaticChange must trigger a file recopy, not a full pipeline rebuild")
		})

		It("LayoutChange triggers a pipeline rebuild", func() {
			scope := server.RebuildScopeForChangeType(server.LayoutChange)
			Expect(scope).To(Equal(server.RebuildPipeline),
				"LayoutChange must trigger a pipeline rebuild to re-render affected pages")
		})

		It("AssetChange triggers a recopy", func() {
			scope := server.RebuildScopeForChangeType(server.AssetChange)
			Expect(scope).To(Equal(server.RebuildRecopy),
				"AssetChange must trigger a file recopy, not a full pipeline rebuild")
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
