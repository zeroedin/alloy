package server

import (
	"errors"

	"github.com/zeroedin/alloy/internal/config"
)

// ErrNotImplemented is returned by all stub functions.
var ErrNotImplemented = errors.New("not implemented")

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
	config *config.Config
	mode   ServerMode
}

// New creates a new Server with the given config in dev mode.
func New(cfg *config.Config) *Server {
	return &Server{config: cfg, mode: ModeDev}
}

// NewWithMode creates a new Server with the given config and explicit mode.
func NewWithMode(cfg *config.Config, mode ServerMode) *Server {
	return &Server{config: cfg, mode: mode}
}

// Mode returns the current server operating mode.
func (s *Server) Mode() ServerMode {
	return s.mode
}

// ShouldRunSSR returns true if the server should execute the Phase 2 SSR
// pipeline. Only true in preview mode when SSR is configured.
func (s *Server) ShouldRunSSR() bool {
	return false
}

// ShouldWriteToDisk returns true if the server should write output to _site/
// (preview mode) instead of serving from an in-memory map (dev mode).
func (s *Server) ShouldWriteToDisk() bool {
	return false
}

// ShouldIncludeDrafts returns true if draft content should be visible.
// Dev mode includes drafts; preview mode excludes them (same as build).
func (s *Server) ShouldIncludeDrafts() bool {
	return false
}

// Start launches the HTTP server on the given port.
func (s *Server) Start(port int) error {
	return ErrNotImplemented
}

// Stop gracefully shuts down the server.
func (s *Server) Stop() error {
	return ErrNotImplemented
}

// ServeHTTP handles an HTTP request and returns the response body for a given path.
func (s *Server) ServeHTTP(path string) ([]byte, error) {
	return nil, ErrNotImplemented
}

// ServeStaticFile serves a static file from the source directory (dev mode).
// In dev mode, static files are served directly without copying to output.
func (s *Server) ServeStaticFile(path string) ([]byte, error) {
	return nil, ErrNotImplemented
}

// ResolvePassthrough maps a URL path to a passthrough source directory file.
func (s *Server) ResolvePassthrough(urlPath string) (string, error) {
	return "", ErrNotImplemented
}

// StartOnPort attempts to start the server on a specific port.
// Returns a descriptive error if the port is already in use.
func (s *Server) StartOnPort(port int) error {
	return ErrNotImplemented
}

// ShouldOpenBrowser returns true if the server should auto-open a browser on start.
func (s *Server) ShouldOpenBrowser() bool {
	return false
}

// RenderPage renders a single page and returns its HTML.
// In dev mode, returns error overlay HTML on failure instead of propagating the error.
// In build mode, returns the error directly.
func (s *Server) RenderPage(path string, content []byte) ([]byte, error) {
	return nil, ErrNotImplemented
}

// HandleExternalSourceFailure handles an unreachable external data source.
// In dev mode: logs warning, continues with stale cache data.
// In build mode: returns error (build must abort even if stale cache exists).
func (s *Server) HandleExternalSourceFailure(sourceName string, err error) error {
	return ErrNotImplemented
}

// HandlePluginCrash handles a plugin subprocess crash.
// In both modes: stops the server / aborts the build.
func (s *Server) HandlePluginCrash(pluginName string, err error) error {
	return ErrNotImplemented
}

// SetNoDrafts configures the server to exclude draft content even in dev mode.
// This is triggered by the --no-drafts CLI flag.
func (s *Server) SetNoDrafts(noDrafts bool) {
	// stub — no-op
}

// InjectOverlay wraps the response HTML with the error overlay when there are
// active build errors. Only applies in dev mode.
func (s *Server) InjectOverlay(html []byte, overlay *OverlayState) ([]byte, error) {
	return nil, ErrNotImplemented
}

// WebSocketReloadMessage returns the JSON message sent to connected browsers
// to trigger a page reload. Format: {"type": "reload"}
func (s *Server) WebSocketReloadMessage() string {
	return ""
}

// DebounceInterval returns the file watcher debounce interval in milliseconds.
func (s *Server) DebounceInterval() int {
	return 0
}

// DetermineRebuildAction decides whether a set of file changes should trigger
// an incremental or full rebuild. Many simultaneous changes trigger a full rebuild.
func (s *Server) DetermineRebuildAction(changedFiles []string) RebuildScope {
	return 0
}
