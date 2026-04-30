package plugin

import (
	"bufio"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed bridge.js
var bridgeScript string

// BridgeState represents the lifecycle state of the Node subprocess bridge.
type BridgeState int

const (
	// BridgeNotStarted is the initial state before Start() is called.
	BridgeNotStarted BridgeState = iota + 1
	// BridgeRunning means the Node subprocess is active and accepting messages.
	BridgeRunning
	// BridgeStopped means the Node subprocess has been shut down.
	BridgeStopped
)

// SSRInstance represents a single component instance for SSR rendering.
type SSRInstance struct {
	Hash string `json:"hash"`
	HTML string `json:"html"`
}

// Message represents a JSON-RPC message exchanged between Alloy (Go) and the
// Node bridge subprocess. Framed with LSP-style Content-Length headers over
// stdin/stdout.
type Message struct {
	ID        int           `json:"id"`
	Type      string        `json:"type,omitempty"`      // "hook", "ssr", "filter"
	Name      string        `json:"name,omitempty"`      // hook/filter name
	Payload   interface{}   `json:"payload,omitempty"`   // hook/filter payload
	Result    interface{}   `json:"result,omitempty"`    // response result
	Error     string        `json:"error,omitempty"`     // error message from bridge
	Instances []SSRInstance `json:"instances,omitempty"` // SSR render instances
}

// EncodeMessage serializes a Message into an LSP-style framed byte sequence:
// Content-Length: <N>\r\n\r\n<JSON body>
func EncodeMessage(msg *Message) ([]byte, error) {
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("message encoding error: %w", err)
	}

	frame := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)
	return []byte(frame), nil
}

// DecodeMessage parses an LSP-style framed byte sequence back into a Message.
// Returns an error if the Content-Length header is missing or malformed.
func DecodeMessage(data []byte) (*Message, error) {
	raw := string(data)

	if !strings.HasPrefix(raw, "Content-Length: ") {
		return nil, fmt.Errorf("malformed frame: missing Content-Length header")
	}

	parts := strings.SplitN(raw, "\r\n\r\n", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("malformed frame: missing header/body separator")
	}

	var msg Message
	if err := json.Unmarshal([]byte(parts[1]), &msg); err != nil {
		return nil, fmt.Errorf("message decode error: %w", err)
	}

	return &msg, nil
}

// NodeRuntime runs Tier 3 Node plugins via a persistent subprocess.
// Communicates via JSON-RPC over stdin/stdout using the embedded bridge.js.
type NodeRuntime struct {
	bridge      *NodeBridge
	projectRoot string
	filters     []string
	shortcodes  []string
	hooks       []string
}

// NewNodeRuntime creates a new Node.js plugin runtime with its own bridge.
// Defaults to the current working directory as the project root for module resolution.
func NewNodeRuntime() *NodeRuntime {
	cwd, _ := os.Getwd()
	return &NodeRuntime{projectRoot: cwd}
}

// ProjectRoot returns the project root used for Node module resolution.
func (r *NodeRuntime) ProjectRoot() string {
	return r.projectRoot
}

// SetProjectRoot sets the project root used for Node module resolution.
func (r *NodeRuntime) SetProjectRoot(root string) {
	r.projectRoot = root
}

// EvalFile loads a JS plugin file in the Node subprocess via ESM import().
// The absolute file path is sent to the bridge, which uses dynamic import()
// to load the plugin as a native ES module.
func (r *NodeRuntime) EvalFile(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	// Start bridge if not running
	if r.bridge == nil {
		r.bridge = NewNodeBridge(r.projectRoot)
		if err := r.bridge.Start(); err != nil {
			return fmt.Errorf("starting Node bridge: %w", err)
		}
	}

	resp, err := r.bridge.Send(&Message{
		Type:    "eval",
		Payload: absPath,
	})
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	// Parse discovered registrations from response
	if resultMap, ok := resp.Result.(map[string]interface{}); ok {
		if f, ok := resultMap["filters"].([]interface{}); ok {
			for _, v := range f {
				if s, ok := v.(string); ok {
					r.filters = append(r.filters, s)
				}
			}
		}
		if s, ok := resultMap["shortcodes"].([]interface{}); ok {
			for _, v := range s {
				if str, ok := v.(string); ok {
					r.shortcodes = append(r.shortcodes, str)
				}
			}
		}
		if h, ok := resultMap["hooks"].([]interface{}); ok {
			for _, v := range h {
				if s, ok := v.(string); ok {
					r.hooks = append(r.hooks, s)
				}
			}
		}
	}

	return nil
}

// RegisteredFilters returns the names of filters registered by the Node plugin.
func (r *NodeRuntime) RegisteredFilters() []string {
	return r.filters
}

// CallFilter routes a filter call through the Node subprocess.
func (r *NodeRuntime) CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error) {
	if r.bridge == nil {
		return input, nil
	}
	payload := map[string]interface{}{
		"input": input,
		"args":  args,
	}
	resp, err := r.bridge.Send(&Message{
		Type:    "filter",
		Name:    name,
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("node filter %q: %w", name, err)
	}
	return resp.Result, nil
}

// RegisteredShortcodes returns the names of shortcodes registered by the Node plugin.
func (r *NodeRuntime) RegisteredShortcodes() []string {
	return r.shortcodes
}

// CallShortcode routes a shortcode call through the Node subprocess.
func (r *NodeRuntime) CallShortcode(name string, args []string, innerContent string) (string, error) {
	if r.bridge == nil {
		return innerContent, nil
	}
	resp, err := r.bridge.Send(&Message{
		Type: "shortcode",
		Name: name,
		Payload: map[string]interface{}{
			"args":    args,
			"content": innerContent,
		},
	})
	if err != nil {
		return "", fmt.Errorf("node shortcode %q: %w", name, err)
	}
	if result, ok := resp.Result.(string); ok {
		return result, nil
	}
	return fmt.Sprint(resp.Result), nil
}

// RegisteredHooks returns the names of hooks registered by the Node plugin.
func (r *NodeRuntime) RegisteredHooks() []string {
	return r.hooks
}

// CallHook routes a hook call through the Node subprocess.
func (r *NodeRuntime) CallHook(name string, payload interface{}) (interface{}, error) {
	if r.bridge == nil {
		return payload, nil
	}
	resp, err := r.bridge.Send(&Message{
		Type:    "hook",
		Name:    name,
		Payload: payload,
	})
	if err != nil {
		return nil, fmt.Errorf("node hook %q: %w", name, err)
	}
	return resp.Result, nil
}

// SetSiteData is a stub for the Node runtime. Site data injection over the
// Node bridge is not yet implemented — returns nil (no-op) since Node plugins
// receive data through hook payloads rather than a persistent alloy.data binding.
func (r *NodeRuntime) SetSiteData(data map[string]interface{}) error {
	return nil
}

// Close shuts down the Node subprocess.
func (r *NodeRuntime) Close() {
	if r.bridge != nil {
		r.bridge.Stop()
	}
}

// NodeBridge manages the lifecycle of the Node subprocess used for Tier 3 plugins.
// Communication uses length-prefixed JSON-RPC over stdin/stdout.
type NodeBridge struct {
	state       BridgeState
	projectRoot string
	cmd         *exec.Cmd
	stdin       io.WriteCloser
	stdout      *bufio.Reader
	mu          sync.Mutex
	nextID      int
	scriptPath  string
}

// NewNodeBridge creates a Node bridge for the given project root.
func NewNodeBridge(projectRoot string) *NodeBridge {
	return &NodeBridge{
		state:       BridgeNotStarted,
		projectRoot: projectRoot,
	}
}

// State returns the current lifecycle state of the bridge.
func (b *NodeBridge) State() BridgeState {
	return b.state
}

// WorkingDir returns the working directory of the Node subprocess.
func (b *NodeBridge) WorkingDir() string {
	if b.cmd != nil && b.cmd.Dir != "" {
		return b.cmd.Dir
	}
	return b.projectRoot
}

// PID returns the process ID of the Node subprocess, or 0 if not running.
func (b *NodeBridge) PID() int {
	if b.cmd != nil && b.cmd.Process != nil {
		return b.cmd.Process.Pid
	}
	return 0
}

// Start spawns the Node subprocess with the embedded bridge script.
// When a project root is available, the bridge script is written under
// .alloy/ so Node's module resolution can find node_modules/ via
// normal ancestor directory traversal.
func (b *NodeBridge) Start() error {
	scriptPath, err := b.writeBridgeScript()
	if err != nil {
		return err
	}
	b.scriptPath = scriptPath

	b.cmd = exec.Command("node", b.scriptPath)
	if b.projectRoot != "" {
		if info, err := os.Stat(b.projectRoot); err == nil && info.IsDir() {
			b.cmd.Dir = b.projectRoot
		}
	}
	b.cmd.Stderr = os.Stderr

	b.stdin, err = b.cmd.StdinPipe()
	if err != nil {
		os.Remove(b.scriptPath)
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := b.cmd.StdoutPipe()
	if err != nil {
		os.Remove(b.scriptPath)
		return fmt.Errorf("stdout pipe: %w", err)
	}
	b.stdout = bufio.NewReader(stdoutPipe)

	if err := b.cmd.Start(); err != nil {
		os.Remove(b.scriptPath)
		return fmt.Errorf("starting node: %w", err)
	}

	b.state = BridgeRunning
	return nil
}

// writeBridgeScript writes the embedded bridge script to disk as .mjs
// so Node always treats it as ESM (required for dynamic import()).
// Prefers projectRoot/.alloy/ so Node can resolve node_modules/ via
// ancestor traversal. Falls back to OS temp dir.
func (b *NodeBridge) writeBridgeScript() (string, error) {
	if b.projectRoot != "" {
		alloyDir := filepath.Join(b.projectRoot, ".alloy")
		if err := os.MkdirAll(alloyDir, 0755); err == nil {
			scriptPath := filepath.Join(alloyDir, "bridge.mjs")
			if err := os.WriteFile(scriptPath, []byte(bridgeScript), 0644); err == nil {
				return scriptPath, nil
			}
		}
	}
	tmpFile, err := os.CreateTemp("", "alloy-bridge-*.mjs")
	if err != nil {
		return "", fmt.Errorf("creating bridge script: %w", err)
	}
	if _, err := tmpFile.WriteString(bridgeScript); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("writing bridge script: %w", err)
	}
	tmpFile.Close()
	return tmpFile.Name(), nil
}

// Send sends a JSON-RPC message and reads the response.
func (b *NodeBridge) Send(msg *Message) (*Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	msg.ID = b.nextID

	encoded, err := EncodeMessage(msg)
	if err != nil {
		return nil, err
	}

	if _, err := b.stdin.Write(encoded); err != nil {
		return nil, fmt.Errorf("writing to node: %w", err)
	}

	// Read response: Content-Length header + body
	headerLine, err := b.stdout.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("reading response header: %w", err)
	}
	headerLine = strings.TrimSpace(headerLine)
	if !strings.HasPrefix(headerLine, "Content-Length:") {
		return nil, fmt.Errorf("unexpected response: %s", headerLine)
	}
	lenStr := strings.TrimSpace(strings.TrimPrefix(headerLine, "Content-Length:"))
	contentLen, err := strconv.Atoi(lenStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Length: %s", lenStr)
	}

	// Read blank line separator
	if _, err := b.stdout.ReadString('\n'); err != nil {
		return nil, fmt.Errorf("reading header separator: %w", err)
	}

	// Read body
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(b.stdout, body); err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	var resp Message
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("node error: %s", resp.Error)
	}

	if resp.ID != msg.ID {
		return nil, fmt.Errorf("response ID mismatch: sent %d, got %d", msg.ID, resp.ID)
	}

	return &resp, nil
}

// Stop gracefully shuts down the Node subprocess.
func (b *NodeBridge) Stop() error {
	if b.stdin != nil {
		b.stdin.Close()
	}
	if b.cmd != nil && b.cmd.Process != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- b.cmd.Wait() }()
		select {
		case <-done:
		case <-ctx.Done():
			b.cmd.Process.Kill()
			<-done
		}
	}
	if b.scriptPath != "" {
		os.Remove(b.scriptPath)
	}
	b.state = BridgeStopped
	return nil
}

// LogPath returns the path where plugin stderr output is written.
func (b *NodeBridge) LogPath() string {
	return filepath.Join(b.projectRoot, ".alloy", "plugin.log")
}
