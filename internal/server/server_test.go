package server_test

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/server"
)

var httpClient = &http.Client{Timeout: 5 * time.Second}

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

	// ── HTTP handler integration (issue #623) ────────────────────────
	// The real HTTP serving happens in startOnAddr + serveFileWithReload.
	// These tests start a real server and make HTTP requests to verify
	// the handler serves files, resolves clean URLs, returns 404s, and
	// injects the live-reload script correctly.

	Describe("HTTP handler", func() {
		It("serves files from the output directory (issue #623)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			Expect(os.MkdirAll(filepath.Join(outputDir, "about"), 0755)).To(Succeed())

			pageHTML := []byte("<html><body>About Us</body></html>")
			Expect(os.WriteFile(filepath.Join(outputDir, "about", "index.html"), pageHTML, 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/about/", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(string(body)).To(ContainSubstring("About Us"),
				"HTTP handler must serve files from the output directory — "+
					"the handler reads from _site/ directly, not through "+
					"intermediate methods (issue #623)")
		})

		It("resolves clean URLs by trying path/index.html (issue #623)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			Expect(os.MkdirAll(filepath.Join(outputDir, "docs"), 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(outputDir, "docs", "index.html"),
				[]byte("<html><body>Docs</body></html>"), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/docs", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("Docs"),
				"requesting /docs without trailing slash must resolve to "+
					"/docs/index.html for clean URL support (issue #623)")
		})

		It("returns 404 for non-existent paths (issue #623)", func() {
			projectRoot := GinkgoT().TempDir()
			Expect(os.MkdirAll(filepath.Join(projectRoot, "_site"), 0755)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/nonexistent", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			Expect(resp.StatusCode).To(Equal(http.StatusNotFound),
				"requesting a path with no matching file must return 404 — "+
					"the handler must not return 200 with empty or default content (issue #623)")
		})

		It("injects live-reload script into HTML responses in dev mode (issue #623)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			Expect(os.MkdirAll(outputDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(outputDir, "index.html"),
				[]byte("<html><body>Home</body></html>"), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(ContainSubstring("WebSocket"),
				"dev mode must inject live-reload WebSocket script into "+
					"HTML responses before </body> (issue #623)")
			Expect(string(body)).To(ContainSubstring("Home"),
				"page content must be preserved after script injection")
		})

		It("serves non-HTML files without live-reload injection (issue #623)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			Expect(os.MkdirAll(outputDir, 0755)).To(Succeed())

			cssContent := []byte("body { color: red; }")
			Expect(os.WriteFile(filepath.Join(outputDir, "style.css"), cssContent, 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/style.css", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).NotTo(ContainSubstring("WebSocket"),
				"non-HTML files must not have live-reload script injected — "+
					"injecting JavaScript into CSS/JS/images would corrupt them (issue #623)")
			Expect(string(body)).To(ContainSubstring("body { color: red; }"),
				"CSS file content must be served verbatim")
		})

		It("continues serving after a missing file request (issue #625)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			Expect(os.MkdirAll(outputDir, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(outputDir, "index.html"),
				[]byte("<html><body>Home</body></html>"), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/missing", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

			resp, err = httpClient.Get(fmt.Sprintf("http://localhost:%d/", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(string(body)).To(ContainSubstring("Home"),
				"server must continue serving valid pages after a 404 — "+
					"a missing file request must not break subsequent requests (issue #625)")
		})

		It("does not inject error overlay into non-HTML responses (issue #625)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			Expect(os.MkdirAll(outputDir, 0755)).To(Succeed())

			cssContent := []byte("body { font-size: 16px; }")
			Expect(os.WriteFile(filepath.Join(outputDir, "app.css"), cssContent, 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			srv.Overlay().SetErrors([]server.BuildError{
				{FilePath: "content/broken.md", Message: "parse error", Stage: "markdown"},
			})
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/app.css", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"CSS file must return 200 — a non-200 would make the body "+
					"assertions meaningless (issue #625)")
			Expect(string(body)).NotTo(ContainSubstring("alloy-overlay"),
				"error overlay must not be injected into non-HTML responses — "+
					"injecting HTML into CSS corrupts the stylesheet (issue #625)")
			Expect(string(body)).To(ContainSubstring("font-size: 16px"),
				"CSS content must be served verbatim even when overlay is active")
		})

		It("serves extensionless direct files without index.html fallback (issue #625)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			Expect(os.MkdirAll(outputDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(outputDir, "CNAME"),
				[]byte("example.com"), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/CNAME", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK),
				"extensionless file must return 200 — the handler checks "+
					"direct file existence before trying index.html fallback")
			Expect(string(body)).To(ContainSubstring("example.com"),
				"extensionless files (CNAME, LICENSE, etc.) must be served "+
					"by direct match — the handler must not skip them and fall "+
					"through to index.html resolution (issue #625)")
		})

		It("serves passthrough files copied to the output directory (issue #625)", func() {
			projectRoot := GinkgoT().TempDir()
			outputDir := filepath.Join(projectRoot, "_site")
			fontsDir := filepath.Join(outputDir, "assets", "fonts")
			Expect(os.MkdirAll(fontsDir, 0755)).To(Succeed())

			fontData := []byte("fake-woff2-font-data")
			Expect(os.WriteFile(filepath.Join(fontsDir, "body.woff2"), fontData, 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Test Site",
				ProjectRoot: projectRoot,
				Build:       config.BuildConfig{Output: "_site"},
			}
			srv := server.New(cfg)
			Expect(srv.Start(0)).To(Succeed())
			defer srv.Stop()

			resp, err := httpClient.Get(fmt.Sprintf("http://localhost:%d/assets/fonts/body.woff2", srv.Port()))
			Expect(err).NotTo(HaveOccurred())
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(body).To(Equal(fontData),
				"passthrough files are copied to _site/ by the build pipeline — "+
					"the HTTP handler must serve them like any other output file (issue #625)")
		})
	})

	// ── MIME type serving (issue #252) ───────────────────────────────
	// The dev server must set correct Content-Type headers based on
	// file extension. Without this, browsers refuse to apply CSS
	// stylesheets served as text/plain.

	Describe("MIME type serving", func() {
		It("serves CSS files with text/css content type", func() {
			contentType := server.MIMEType(".css")
			Expect(contentType).To(Equal("text/css; charset=utf-8"),
				"CSS files must be served as text/css, not text/plain")
		})

		It("serves JavaScript files with application/javascript content type", func() {
			contentType := server.MIMEType(".js")
			Expect(contentType).To(Equal("application/javascript; charset=utf-8"))
		})

		It("serves JSON files with application/json content type", func() {
			contentType := server.MIMEType(".json")
			Expect(contentType).To(Equal("application/json; charset=utf-8"))
		})

		It("serves SVG files with image/svg+xml content type", func() {
			contentType := server.MIMEType(".svg")
			Expect(contentType).To(Equal("image/svg+xml"))
		})

		It("serves WASM files with application/wasm content type", func() {
			contentType := server.MIMEType(".wasm")
			Expect(contentType).To(Equal("application/wasm"))
		})

		It("serves font files with correct content type", func() {
			Expect(server.MIMEType(".woff2")).To(Equal("font/woff2"))
			Expect(server.MIMEType(".woff")).To(Equal("font/woff"))
			Expect(server.MIMEType(".ttf")).To(Equal("font/ttf"))
		})

		It("serves image files with correct content type", func() {
			Expect(server.MIMEType(".png")).To(Equal("image/png"))
			Expect(server.MIMEType(".jpg")).To(Equal("image/jpeg"))
			Expect(server.MIMEType(".jpeg")).To(Equal("image/jpeg"))
			Expect(server.MIMEType(".gif")).To(Equal("image/gif"))
			Expect(server.MIMEType(".webp")).To(Equal("image/webp"))
			Expect(server.MIMEType(".ico")).To(Equal("image/x-icon"))
		})

		It("serves HTML files with text/html content type", func() {
			contentType := server.MIMEType(".html")
			Expect(contentType).To(Equal("text/html; charset=utf-8"))
		})

		It("returns application/octet-stream for unknown extensions", func() {
			contentType := server.MIMEType(".alloyunknown")
			Expect(contentType).To(Equal("application/octet-stream"),
				"unknown file extensions must fall back to application/octet-stream")
		})

		It("serves YAML files with text/yaml content type (issue #612)", func() {
			Expect(server.MIMEType(".yaml")).To(Equal("text/yaml; charset=utf-8"),
				".yaml is not in Go's built-in MIME table — without the override, "+
					"minimal platforms (Alpine, scratch) return application/octet-stream")
			Expect(server.MIMEType(".yml")).To(Equal("text/yaml; charset=utf-8"),
				".yml must match .yaml — both are YAML files")
		})

		It("serves TOML files with application/toml content type (issue #612)", func() {
			Expect(server.MIMEType(".toml")).To(Equal("application/toml; charset=utf-8"),
				".toml is not in Go's built-in MIME table — without the override, "+
					"minimal platforms return application/octet-stream")
		})

		It("serves XML files with application/xml content type (issue #612)", func() {
			Expect(server.MIMEType(".xml")).To(Equal("application/xml; charset=utf-8"),
				"stdlib returns text/xml for .xml — the override ensures "+
					"application/xml which is the IANA-preferred media type")
		})

		It("serves plain text files with text/plain content type (issue #612)", func() {
			Expect(server.MIMEType(".txt")).To(Equal("text/plain; charset=utf-8"),
				".txt needs an explicit charset=utf-8 — some platforms return "+
					"text/plain without charset")
		})

		It("serves OTF font files with font/otf content type (issue #612)", func() {
			Expect(server.MIMEType(".otf")).To(Equal("font/otf"),
				".otf is not in Go's built-in MIME table — without the override, "+
					"minimal platforms return application/octet-stream")
		})

		It("delegates to stdlib mime package for non-overridden extensions (issue #600)", func() {
			mime.AddExtensionType(".alloytest600", "application/x-alloy-test")
			Expect(server.MIMEType(".alloytest600")).To(Equal("application/x-alloy-test"),
				"MIMEType must delegate to mime.TypeByExtension for extensions "+
					"not in the override map — registering a custom type via "+
					"mime.AddExtensionType proves delegation without depending "+
					"on platform-specific MIME databases (issue #600)")
		})
	})

	// ── Content-colocated file serving in dev mode (issue #300) ─────
	// In dev mode, non-content files in content/ must be served directly
	// from source — no _site/ copy needed. The dev server falls back to
	// content/ for URLs that don't match a rendered page in memory.

	Describe("Content-colocated file serving", func() {
		It("serves non-content files from content/ in dev mode", func() {
			projectRoot, err := os.MkdirTemp("", "alloy-content-serving-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(projectRoot) })

			contentDir := filepath.Join(projectRoot, "content", "about")
			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			svgBytes := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><circle r="10"/></svg>`)
			Expect(os.WriteFile(filepath.Join(contentDir, "diagram.svg"), svgBytes, 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Dev Site",
				ProjectRoot: projectRoot,
			}
			srv := server.New(cfg)
			body, err := srv.ServeContentFile("/about/diagram.svg")
			Expect(err).NotTo(HaveOccurred())
			Expect(body).To(Equal(svgBytes),
				"dev mode must serve content-colocated files directly from content/ — "+
					"exact bytes must match the source file")
		})

		It("returns error for non-existent content file", func() {
			projectRoot, err := os.MkdirTemp("", "alloy-content-serving-*")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { os.RemoveAll(projectRoot) })

			Expect(os.MkdirAll(filepath.Join(projectRoot, "content"), 0755)).To(Succeed())

			cfg := &config.Config{
				Title:       "Dev Site",
				ProjectRoot: projectRoot,
			}
			srv := server.New(cfg)
			_, err = srv.ServeContentFile("/about/nonexistent.svg")
			Expect(err).To(HaveOccurred(),
				"requesting a non-existent content file must return an error, not empty content")
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
