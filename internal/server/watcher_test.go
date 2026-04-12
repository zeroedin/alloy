package server_test

import (
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

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
					Build: "golit render _site/**/*.html",
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
})
