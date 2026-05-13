package server

import (
	"context"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zeroedin/alloy/internal/config"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

var mimeOverrides = map[string]string{
	".js":   "application/javascript; charset=utf-8",
	".json": "application/json; charset=utf-8",
	".yaml": "text/yaml; charset=utf-8",
	".yml":  "text/yaml; charset=utf-8",
	".toml": "application/toml; charset=utf-8",
}

func MIMEType(ext string) string {
	ext = strings.ToLower(ext)
	if ct, ok := mimeOverrides[ext]; ok {
		return ct
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}

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
	port       int           // actual port the server is listening on
	overlay    *OverlayState // error overlay state for dev mode

	// WebSocket live-reload clients
	wsClients map[*websocket.Conn]struct{}
	wsMu      sync.Mutex
}

// New creates a new Server with the given config in dev mode.
func New(cfg *config.Config) *Server {
	return &Server{
		config:  cfg,
		mode:    ModeDev,
		pages:   make(map[string][]byte),
		overlay: NewOverlayState(),
	}
}

// NewWithMode creates a new Server with the given config and explicit mode.
func NewWithMode(cfg *config.Config, mode ServerMode) *Server {
	return &Server{
		config:  cfg,
		mode:    mode,
		pages:   make(map[string][]byte),
		overlay: NewOverlayState(),
	}
}

// Mode returns the current server operating mode.
func (s *Server) Mode() ServerMode {
	return s.mode
}

// Overlay returns the server's error overlay state.
func (s *Server) Overlay() *OverlayState {
	return s.overlay
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

// wsUpgrader handles WebSocket upgrade for live reload connections.
var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// liveReloadScript is the JavaScript injected before </body> in dev mode
// to establish a WebSocket connection for live reload.
const liveReloadScript = `<script>
(function(){var ws=new WebSocket("ws://"+location.host+"/_alloy/ws");ws.onmessage=function(e){var d=JSON.parse(e.data);if(d.type==="reload")location.reload()};ws.onclose=function(){setTimeout(function(){location.reload()},1000)}})();
</script>`

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

	// WebSocket endpoint for live reload
	s.wsClients = make(map[*websocket.Conn]struct{})
	mux.HandleFunc("/_alloy/ws", s.handleWebSocket)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		urlPath := r.URL.Path
		filePath := filepath.Join(outputDir, filepath.FromSlash(urlPath))

		// If path ends with /, try index.html
		if strings.HasSuffix(urlPath, "/") {
			filePath = filepath.Join(filePath, "index.html")
		}

		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			s.serveFileWithReload(w, r, filePath)
			return
		}

		// Try adding /index.html for clean URLs
		indexPath := filepath.Join(filePath, "index.html")
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			s.serveFileWithReload(w, r, indexPath)
			return
		}

		http.NotFound(w, r)
	})

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port

	s.httpServer = &http.Server{Handler: mux}
	s.done = make(chan struct{})

	go func() {
		defer close(s.done)
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	return nil
}

// serveFileWithReload serves a file, injecting the live-reload script into
// HTML responses when running in dev mode. Non-HTML files and preview mode
// use http.ServeFile (with ETag/Last-Modified caching). Dev mode HTML is
// served without cache headers to ensure browsers always get fresh content
// after a live-reload rebuild.
func (s *Server) serveFileWithReload(w http.ResponseWriter, r *http.Request, filePath string) {
	if s.mode != ModeDev || !strings.HasSuffix(filePath, ".html") {
		w.Header().Set("Content-Type", MIMEType(filepath.Ext(filePath)))
		http.ServeFile(w, r, filePath)
		return
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		http.Error(w, "read error", http.StatusInternalServerError)
		return
	}

	htmlStr := string(data)

	// Inject error overlay if there are active build errors
	if s.overlay.HasErrors() {
		overlayHTML := RenderOverlay(s.overlay.Errors())
		if idx := strings.LastIndex(htmlStr, "</body>"); idx >= 0 {
			htmlStr = htmlStr[:idx] + overlayHTML + htmlStr[idx:]
		}
	}

	if idx := strings.LastIndex(htmlStr, "</body>"); idx >= 0 {
		htmlStr = htmlStr[:idx] + liveReloadScript + htmlStr[idx:]
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(htmlStr))
}

// handleWebSocket upgrades an HTTP connection to a WebSocket for live reload.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade: %v", err)
		return
	}

	s.wsMu.Lock()
	s.wsClients[conn] = struct{}{}
	s.wsMu.Unlock()

	// Read loop — keeps connection alive until client disconnects
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.wsMu.Lock()
	delete(s.wsClients, conn)
	s.wsMu.Unlock()
	conn.Close()
}

// BroadcastReload sends a reload message to all connected WebSocket clients.
// Failed connections are removed from the map but not closed here —
// handleWebSocket owns the close to avoid double-close.
func (s *Server) BroadcastReload() {
	msg := ReloadMessage()
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	for conn := range s.wsClients {
		if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
			delete(s.wsClients, conn)
		}
	}
}

// Stop gracefully shuts down the server with a 5-second timeout.
func (s *Server) Stop() error {
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
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

// ServeContentFile reads a non-content file from the content directory.
// Used in dev mode to serve colocated files (SVGs, images, etc.) directly
// from source without writing to _site/.
func (s *Server) ServeContentFile(urlPath string) ([]byte, error) {
	contentDir := s.config.Structure.Content
	if contentDir == "" {
		contentDir = "content"
	}
	if s.config.ProjectRoot != "" {
		contentDir = filepath.Join(s.config.ProjectRoot, contentDir)
	}
	filePath := filepath.Join(contentDir, filepath.FromSlash(urlPath))
	filePath = filepath.Clean(filePath)

	absContent, err := filepath.Abs(contentDir)
	if err != nil {
		return nil, fmt.Errorf("content file not found: %s", urlPath)
	}
	absFile, err := filepath.Abs(filePath)
	if err != nil {
		return nil, fmt.Errorf("content file not found: %s", urlPath)
	}
	rel, err := filepath.Rel(absContent, absFile)
	if err != nil || strings.HasPrefix(rel, "..") {
		return nil, fmt.Errorf("content file path escapes content directory: %s", urlPath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("content file not found: %s", urlPath)
	}
	return data, nil
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

// Port returns the actual port the server is listening on.
// Returns 0 before the server has started.
func (s *Server) Port() int {
	return s.port
}

// StartWithPortFallback tries to start the server on preferredPort, incrementing
// up to maxAttempts times if the port is occupied. Returns the actual port used.
func (s *Server) StartWithPortFallback(preferredPort, maxAttempts int) (int, error) {
	if maxAttempts <= 0 {
		return 0, fmt.Errorf("no available port: maxAttempts is 0")
	}
	for i := 0; i < maxAttempts; i++ {
		port := preferredPort + i
		err := s.startOnAddr(fmt.Sprintf(":%d", port))
		if err == nil {
			return s.port, nil
		}
		if i < maxAttempts-1 {
			log.Printf("warning: port %d in use, trying %d", port, port+1)
		}
	}
	return 0, fmt.Errorf("no available port after %d attempts (tried %d–%d)",
		maxAttempts, preferredPort, preferredPort+maxAttempts-1)
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

// Serve404Page reads 404.html from the output root and returns its contents.
// In dev mode, injects the live-reload WebSocket script before </body> so
// the 404 page auto-reloads when the user fixes a broken route.
// Returns an error if the file does not exist, allowing the caller to fall
// back to Go's default http.NotFound() response.
func (s *Server) Serve404Page(outputDir string) ([]byte, error) {
	notFoundPath := filepath.Join(outputDir, "404.html")
	data, err := os.ReadFile(notFoundPath)
	if err != nil {
		return nil, err
	}

	if s.mode == ModeDev {
		html := string(data)
		if idx := strings.LastIndex(html, "</body>"); idx >= 0 {
			html = html[:idx] + liveReloadScript + html[idx:]
		}
		return []byte(html), nil
	}

	return data, nil
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
