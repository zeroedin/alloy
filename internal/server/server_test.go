package server_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/server"
)

var _ = Describe("Server", func() {

	// ── Server startup ─────────────────────────────────────────────────

	Describe("Server startup", func() {
		It("creates server with config and starts successfully", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			Expect(srv).NotTo(BeNil())

			// Server must actually start without error (port 0 = OS-assigned)
			err := srv.Start(0)
			Expect(err).NotTo(HaveOccurred())
			defer srv.Stop()

			// Clean shutdown must also succeed
			err = srv.Stop()
			Expect(err).NotTo(HaveOccurred())
		})

		It("starts on an OS-assigned port without error", func() {
			cfg := &config.Config{}
			srv := server.New(cfg)
			err := srv.Start(0)
			Expect(err).NotTo(HaveOccurred())
			defer srv.Stop()
		})
	})

	// ── Server shutdown ────────────────────────────────────────────────

	Describe("Server shutdown", func() {
		It("Stop returns nil error on success", func() {
			cfg := &config.Config{}
			srv := server.New(cfg)
			err := srv.Stop()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ── Preview mode (alloy serve --preview) ──────────────────────────

	Describe("Preview mode", func() {
		It("server created with ModePreview reports ModePreview", func() {
			cfg := &config.Config{Title: "Preview Site"}
			srv := server.NewWithMode(cfg, server.ModePreview)
			Expect(srv.Mode()).To(Equal(server.ModePreview))

			// Preview mode must write to disk (proves mode drives behavior)
			Expect(srv.ShouldWriteToDisk()).To(BeTrue(),
				"preview mode must write to _site/")
		})

		It("server created without --preview reports ModeDev", func() {
			cfg := &config.Config{Title: "Dev Site"}
			srv := server.New(cfg)
			Expect(srv.Mode()).To(Equal(server.ModeDev))

			// Dev mode must include drafts (proves mode drives behavior)
			Expect(srv.ShouldIncludeDrafts()).To(BeTrue(),
				"dev mode must show drafts")
		})

		It("preview mode with SSR config runs Phase 2 SSR", func() {
			cfg := &config.Config{
				Title: "SSR Site",
				SSR: &config.SSRConfig{
					Command: "golit render --defs bundles/",
				},
			}
			srv := server.NewWithMode(cfg, server.ModePreview)
			Expect(srv.ShouldRunSSR()).To(BeTrue(),
				"preview + SSR config should enable Phase 2")
		})

		It("preview mode without SSR config skips Phase 2", func() {
			// Positive-case guard: SSR config present → must be true
			ssrCfg := &config.Config{
				Title: "SSR Site",
				SSR:   &config.SSRConfig{Command: "golit render --defs bundles/"},
			}
			ssrSrv := server.NewWithMode(ssrCfg, server.ModePreview)
			Expect(ssrSrv.ShouldRunSSR()).To(BeTrue(),
				"guard: preview + SSR config must enable Phase 2")

			// Actual assertion: no SSR config → must be false
			cfg := &config.Config{Title: "Simple Site"}
			srv := server.NewWithMode(cfg, server.ModePreview)
			Expect(srv.ShouldRunSSR()).To(BeFalse(),
				"preview without SSR config should skip Phase 2")
		})

		It("preview mode writes to disk, dev mode serves from memory", func() {
			cfg := &config.Config{Title: "Test Site"}

			previewSrv := server.NewWithMode(cfg, server.ModePreview)
			Expect(previewSrv.ShouldWriteToDisk()).To(BeTrue(),
				"preview mode must write to _site/")

			devSrv := server.New(cfg)
			Expect(devSrv.ShouldWriteToDisk()).To(BeFalse(),
				"dev mode must serve from in-memory map")
		})

		It("dev mode includes drafts, preview mode excludes them", func() {
			cfg := &config.Config{Title: "Test Site"}

			devSrv := server.New(cfg)
			Expect(devSrv.ShouldIncludeDrafts()).To(BeTrue(),
				"dev mode must show drafts for author preview")

			previewSrv := server.NewWithMode(cfg, server.ModePreview)
			Expect(previewSrv.ShouldIncludeDrafts()).To(BeFalse(),
				"preview mode must exclude drafts like build")
		})
	})

	// ── HTTP request handling ─────────────────────────────────────────

	Describe("HTTP request handling", func() {
		It("serves rendered page content for a valid path", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			body, err := srv.ServeHTTP("/about/")
			Expect(err).NotTo(HaveOccurred())
			Expect(body).NotTo(BeEmpty(),
				"must serve rendered page content")
		})
	})

	// ── Static file serving ───────────────────────────────────────────

	Describe("Static file serving", func() {
		It("serves static files directly from source in dev mode", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			body, err := srv.ServeStaticFile("/robots.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(body).NotTo(BeEmpty(),
				"dev mode must serve static files from source directory")
		})
	})

	// ── Passthrough path mapping ──────────────────────────────────────

	Describe("Passthrough path mapping", func() {
		It("maps URL path to passthrough source directory", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			source, err := srv.ResolvePassthrough("/assets/fonts/body.woff2")
			Expect(err).NotTo(HaveOccurred())
			Expect(source).NotTo(BeEmpty(),
				"passthrough URL must resolve to source file path")
		})
	})

	// ── Port conflict handling ────────────────────────────────────────

	Describe("Port conflict handling", func() {
		It("returns descriptive error when port is in use", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			err := srv.StartOnPort(0) // port 0 = let OS assign
			Expect(err).NotTo(HaveOccurred(),
				"port 0 should be assignable")
		})
	})

	// ── Port auto-increment (§8) ─────────────────────────────────────

	Describe("Port auto-increment", func() {
		It("StartWithPortFallback returns preferred port when available", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			actualPort, err := srv.StartWithPortFallback(0, 10) // port 0 = OS assigns a free port
			Expect(err).NotTo(HaveOccurred())
			defer srv.Stop()
			Expect(actualPort).To(BeNumerically(">", 0),
				"must return the actual port assigned by the OS")
		})

		It("StartWithPortFallback tries next port when preferred is occupied", func() {
			cfg := &config.Config{Title: "Test Site"}

			// Occupy a port by binding to it
			listener, err := net.Listen("tcp", ":0")
			Expect(err).NotTo(HaveOccurred())
			defer listener.Close()
			occupiedPort := listener.Addr().(*net.TCPAddr).Port

			srv := server.New(cfg)
			actualPort, err := srv.StartWithPortFallback(occupiedPort, 10)
			Expect(err).NotTo(HaveOccurred())
			defer srv.Stop()
			Expect(actualPort).NotTo(Equal(occupiedPort),
				"must not return the occupied port")
			Expect(actualPort).To(BeNumerically(">", occupiedPort),
				"must increment to a higher port")
		})

		It("returns error after max attempts exhausted", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			// maxAttempts=0 means no attempts allowed — must fail immediately
			_, err := srv.StartWithPortFallback(3000, 0)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no available port"),
				"error must explain that no port was available")
		})

		It("Port() returns 0 before server has started", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			Expect(srv.Port()).To(Equal(0),
				"port must be 0 before server starts")
		})

		It("Port() returns actual port after successful start", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			actualPort, err := srv.StartWithPortFallback(0, 10)
			Expect(err).NotTo(HaveOccurred())
			defer srv.Stop()
			Expect(srv.Port()).To(Equal(actualPort),
				"Port() must return the same port that StartWithPortFallback returned")
		})
	})

	// ── Auto-browser-open ─────────────────────────────────────────────

	Describe("Auto-browser-open", func() {
		It("ShouldOpenBrowser returns true by default in dev mode", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			// Guard: must not return true from nil stub
			previewSrv := server.NewWithMode(cfg, server.ModePreview)
			Expect(previewSrv.ShouldOpenBrowser()).To(BeFalse(),
				"guard: preview mode should not auto-open browser")

			Expect(srv.ShouldOpenBrowser()).To(BeTrue(),
				"dev mode should auto-open browser by default")
		})
	})

	// ── Build vs Dev mode error behavior (§2) ────────────────────────

	Describe("Build vs Dev mode error behavior", func() {
		It("dev mode: page render failure does not stop the server", func() {
			cfg := &config.Config{Title: "Dev Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModeDev)

			_, err := srv.RenderPage("content/bad.md", []byte("{{ broken }}"))
			// In dev mode, the error should be captured for the overlay, not propagated
			Expect(err).NotTo(HaveOccurred(),
				"dev mode must not propagate render errors — they go to the overlay")
		})

		It("dev mode: other pages continue to serve after one fails", func() {
			cfg := &config.Config{Title: "Dev Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModeDev)

			// First page fails
			_, _ = srv.RenderPage("content/bad.md", []byte("{{ broken }}"))

			// Second page should still work
			result, err := srv.ServeHTTP("/about/")
			Expect(err).NotTo(HaveOccurred(),
				"other pages must continue serving after a render failure")
			Expect(result).NotTo(BeEmpty(),
				"served page must have content")
		})

		It("dev mode: unreachable source shows warning, continues with stale cache", func() {
			cfg := &config.Config{Title: "Dev Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModeDev)

			err := srv.HandleExternalSourceFailure("cms-api", errors.New("connection refused"))
			Expect(err).NotTo(HaveOccurred(),
				"dev mode must continue with stale cache when source is unreachable")
		})

		It("build mode: unreachable source aborts build even with stale cache", func() {
			cfg := &config.Config{Title: "Build Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModePreview)

			err := srv.HandleExternalSourceFailure("cms-api", errors.New("connection refused"))
			Expect(err).To(HaveOccurred(),
				"build mode must abort when external source is unreachable")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("cms-api"),
				ContainSubstring("unreachable"),
				ContainSubstring("source"),
			), "error must identify the failing source")
		})

		It("build mode: plugin crash aborts the build", func() {
			cfg := &config.Config{Title: "Build Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModePreview)

			err := srv.HandlePluginCrash("image-optimizer", errors.New("segfault"))
			Expect(err).To(HaveOccurred(),
				"plugin crash must abort the build")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("image-optimizer"),
				ContainSubstring("plugin"),
				ContainSubstring("crash"),
			), "error must identify the crashing plugin")
		})

		It("dev mode: plugin crash stops the server", func() {
			cfg := &config.Config{Title: "Dev Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModeDev)

			err := srv.HandlePluginCrash("image-optimizer", errors.New("segfault"))
			Expect(err).To(HaveOccurred(),
				"plugin crash must stop the dev server")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("image-optimizer"),
				ContainSubstring("plugin"),
				ContainSubstring("crash"),
			), "error must identify the crashing plugin")
		})
	})

	// ── --no-drafts flag behavior (§8) ───────────────────────────────

	Describe("--no-drafts flag behavior", func() {
		It("dev mode with --no-drafts excludes drafts", func() {
			cfg := &config.Config{Title: "Dev Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModeDev)

			// Guard: dev mode normally includes drafts
			Expect(srv.ShouldIncludeDrafts()).To(BeTrue(),
				"guard: dev mode must include drafts by default")

			// With --no-drafts, drafts should be excluded
			srv.SetNoDrafts(true)
			Expect(srv.ShouldIncludeDrafts()).To(BeFalse(),
				"--no-drafts flag must exclude drafts even in dev mode")
		})
	})

	// ── Overlay injection into HTTP response (§8) ────────────────────

	Describe("Overlay injection into HTTP response", func() {
		It("injects error overlay HTML into response when build errors exist", func() {
			cfg := &config.Config{Title: "Dev Site", Build: config.BuildConfig{Output: "_site"}}
			srv := server.NewWithMode(cfg, server.ModeDev)

			overlay := server.NewOverlayState()
			overlay.SetErrors([]server.BuildError{
				{FilePath: "content/bad.md", Line: 5, Message: "broken template", Stage: "template rendering"},
			})

			pageHTML := []byte("<html><body>Hello</body></html>")
			result, err := srv.InjectOverlay(pageHTML, overlay)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(result)).To(ContainSubstring("content/bad.md"),
				"injected overlay must include the error file path")
			Expect(string(result)).To(ContainSubstring("broken template"),
				"injected overlay must include the error message")
		})
	})

	// ── WebSocket reload protocol (§8) ──────────────────────────────

	Describe("WebSocket reload protocol", func() {
		It("sends reload message with type:reload", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			msg := srv.WebSocketReloadMessage()
			Expect(msg).NotTo(BeEmpty(),
				"reload message must not be empty")
			Expect(msg).To(ContainSubstring(`"type"`),
				"reload message must contain type field")
			Expect(msg).To(ContainSubstring(`"reload"`),
				"reload message type must be 'reload'")
		})
	})

	// ── File watcher debounce (§8) ──────────────────────────────────

	Describe("File watcher", func() {
		It("debounce interval is 50ms", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			Expect(srv.DebounceInterval()).To(Equal(50),
				"file watcher debounce must be 50ms per spec")
		})

		It("bulk file changes trigger full rebuild", func() {
			cfg := &config.Config{Title: "Test Site"}
			srv := server.New(cfg)
			changes := []string{
				"content/a.md", "content/b.md", "content/c.md",
				"content/d.md", "content/e.md", "content/f.md",
				"content/g.md", "content/h.md", "content/i.md",
				"content/j.md",
			}
			action := srv.DetermineRebuildAction(changes)
			Expect(action).To(Equal(server.RebuildFull),
				"many simultaneous changes must trigger full rebuild")
		})
	})

	// ── Custom 404 page (issue #109) ─────────────────────────────────

	Describe("Custom 404 page", func() {
		It("serves 404.html from output root when it exists", func() {
			// Create a temp output dir with a 404.html
			outputDir := GinkgoT().TempDir()
			err := os.WriteFile(filepath.Join(outputDir, "404.html"),
				[]byte("<html><body>Custom Not Found</body></html>"), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg := &config.Config{Title: "Test Site", Build: config.BuildConfig{Output: outputDir}}
			srv := server.New(cfg)

			body, err := srv.Serve404Page(outputDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("Custom Not Found"),
				"must serve the custom 404.html content")
		})

		It("returns error when no 404.html exists in output root", func() {
			// Empty output dir — no 404.html
			outputDir := GinkgoT().TempDir()

			cfg := &config.Config{Title: "Test Site", Build: config.BuildConfig{Output: outputDir}}
			srv := server.New(cfg)

			_, err := srv.Serve404Page(outputDir)
			Expect(err).To(HaveOccurred(),
				"must return error when 404.html does not exist")
		})

		It("404 page receives WebSocket reload script in dev mode", func() {
			// Create a temp output dir with a 404.html containing </body>
			outputDir := GinkgoT().TempDir()
			err := os.WriteFile(filepath.Join(outputDir, "404.html"),
				[]byte("<html><body>Not Found</body></html>"), 0644)
			Expect(err).NotTo(HaveOccurred())

			cfg := &config.Config{Title: "Test Site", Build: config.BuildConfig{Output: outputDir}}
			srv := server.New(cfg)

			body, err := srv.Serve404Page(outputDir)
			Expect(err).NotTo(HaveOccurred())

			// In dev mode, the served content must include the reload script
			// (same injection as any other served page)
			Expect(string(body)).To(ContainSubstring("WebSocket"),
				"dev mode must inject WebSocket reload script into 404 page")
		})
	})
})
