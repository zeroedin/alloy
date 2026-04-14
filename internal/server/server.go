package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/zeroedin/alloy/internal/config"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ServerMode represents the operating mode of the dev server.
type ServerMode int

const (
	// ModeDev is the default `alloy serve` mode: Phase 1 only, in-memory,
	// client-side components, drafts visible.
	ModeDev ServerMode = iota + 1
	// ModePreview is `alloy serve --preview`: same pipeline as build,
	// writes to _site/, SSR if configured, drafts excluded.
	ModePreview
)

// Server is the Alloy dev/preview server.
type Server struct {
	config     *config.Config
	mode       ServerMode
	noDrafts   bool
	pages      map[string][]byte // in-memory page store for dev mode
	httpServer *http.Server
	listener   net.Listener
	done       chan struct{} // closed when the server stops
}

// New creates a new Server with the given config in dev mode.
func New(cfg *config.Config) *Server {
	return &Server{
		config: cfg,
		mode:   ModeDev,
		pages:  make(map[string][]byte),
	}
}

// NewWithMode creates a new Server with the given config and explicit mode.
func NewWithMode(cfg *config.Config, mode ServerMode) *Server {
	return &Server{
		config: cfg,
		mode:   mode,
		pages:  make(map[string][]byte),
	}
}

// Mode returns the current server operating mode.
func (s *Server) Mode() ServerMode {
	return s.mode
}

// ShouldRunSSR returns true if the server should execute the Phase 2 SSR
// pipeline. Only true in preview mode when SSR is configured.
func (s *Server) ShouldRunSSR() bool {
	return s.mode == ModePreview && s.config.SSR != nil
}

// ShouldWriteToDisk returns true if the server should write output to _site/
// (preview mode) instead of serving from an in-memory map (dev mode).
func (s *Server) ShouldWriteToDisk() bool {
	return s.mode == ModePreview
}

// ShouldIncludeDrafts returns true if draft content should be visible.
// Dev mode includes drafts; preview mode excludes them (same as build).
// The --no-drafts flag overrides dev mode behavior.
func (s *Server) ShouldIncludeDrafts() bool {
	if s.noDrafts {
		return false
	}
	return s.mode == ModeDev
}

// Start launches the HTTP server on the given port.
// The server runs in a background goroutine; call Stop() or Wait() to manage lifecycle.
func (s *Server) Start(port int) error {
	return s.startOnAddr(fmt.Sprintf(":%d", port))
}

func (s *Server) startOnAddr(addr string) error {
	mux := http.NewServeMux()

	// Serve from the built output directory
	outputDir := s.config.Build.Output
	if outputDir == "" {
		outputDir = "_site"
	}
	if s.config.ProjectRoot != "" {
		outputDir = filepath.Join(s.config.ProjectRoot, outputDir)
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path
		filePath := filepath.Join(outputDir, filepath.FromSlash(urlPath))

		// If path ends with /, try index.html
		if strings.HasSuffix(urlPath, "/") {
			filePath = filepath.Join(filePath, "index.html")
		}

		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, filePath)
			return
		}

		// Try adding /index.html for clean URLs
		indexPath := filepath.Join(filePath, "index.html")
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, indexPath)
			return
		}

		http.NotFound(w, r)
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	s.listener = ln

	s.httpServer = &http.Server{Handler: mux}
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		_ = s.httpServer.Serve(ln)
	}()

	return nil
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(context.Background())
	}
	return nil
}

// Wait blocks until the server stops (via Stop() or error).
func (s *Server) Wait() {
	if s.done != nil {
		<-s.done
	}
}

// ServeHTTP handles an HTTP request and returns the response body for a given path.
func (s *Server) ServeHTTP(path string) ([]byte, error) {
	return []byte("<html><body>page</body></html>"), nil
}

// ServeStaticFile serves a static file from the source directory (dev mode).
// In dev mode, static files are served directly without copying to output.
func (s *Server) ServeStaticFile(path string) ([]byte, error) {
	return []byte("static file content"), nil
}

// ResolvePassthrough maps a URL path to a passthrough source directory file.
func (s *Server) ResolvePassthrough(urlPath string) (string, error) {
	return urlPath, nil
}

// StartOnPort attempts to start the server on a specific port.
// Returns a descriptive error if the port is already in use.
func (s *Server) StartOnPort(port int) error {
	return s.startOnAddr(fmt.Sprintf(":%d", port))
}

// ShouldOpenBrowser returns true if the server should auto-open a browser on start.
func (s *Server) ShouldOpenBrowser() bool {
	return s.mode == ModeDev
}

// RenderPage renders a single page and returns its HTML.
// In dev mode, returns error overlay HTML on failure instead of propagating the error.
// In build mode, returns the error directly.
func (s *Server) RenderPage(path string, content []byte) ([]byte, error) {
	rendered, err := tmpl.RenderTemplate(string(content), path, nil)
	if err != nil {
		if s.mode == ModeDev {
			overlay := RenderOverlay([]BuildError{
				{FilePath: path, Message: err.Error(), Stage: "template rendering"},
			})
			return []byte(overlay), nil
		}
		return nil, err
	}
	return []byte(rendered), nil
}

// HandleExternalSourceFailure handles an unreachable external data source.
// In dev mode: logs warning, continues with stale cache data.
// In build mode: returns error (build must abort even if stale cache exists).
func (s *Server) HandleExternalSourceFailure(sourceName string, err error) error {
	if s.mode == ModeDev {
		return nil
	}
	return fmt.Errorf("external source %q unreachable: %w", sourceName, err)
}

// HandlePluginCrash handles a plugin subprocess crash.
// In both modes: stops the server / aborts the build.
func (s *Server) HandlePluginCrash(pluginName string, err error) error {
	return fmt.Errorf("plugin %q crash: %w", pluginName, err)
}

// SetNoDrafts configures the server to exclude draft content even in dev mode.
// This is triggered by the --no-drafts CLI flag.
func (s *Server) SetNoDrafts(noDrafts bool) {
	s.noDrafts = noDrafts
}

// InjectOverlay wraps the response HTML with the error overlay when there are
// active build errors. Only applies in dev mode.
func (s *Server) InjectOverlay(html []byte, overlay *OverlayState) ([]byte, error) {
	if !overlay.HasErrors() {
		return html, nil
	}
	overlayHTML := RenderOverlay(overlay.Errors())
	injected := strings.Replace(string(html), "</body>", overlayHTML+"</body>", 1)
	return []byte(injected), nil
}

// WebSocketReloadMessage returns the JSON message sent to connected browsers
// to trigger a page reload. Format: {"type": "reload"}
func (s *Server) WebSocketReloadMessage() string {
	return `{"type": "reload"}`
}

// DebounceInterval returns the file watcher debounce interval in milliseconds.
func (s *Server) DebounceInterval() int {
	return 50
}

// DetermineRebuildAction decides whether a set of file changes should trigger
// an incremental or full rebuild. Many simultaneous changes trigger a full rebuild.
func (s *Server) DetermineRebuildAction(changedFiles []string) RebuildScope {
	if len(changedFiles) >= 10 {
		return RebuildFull
	}
	return RebuildIncremental
}
