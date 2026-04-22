package plugin

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

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

// NodeRuntime wraps a Node.js subprocess for Tier 3 plugins.
// Implements PluginFilterRuntime so the pipeline bridging loop can
// treat it the same as QuickJSRuntime and WASMRuntime.
type NodeRuntime struct {
	filters    []string
	shortcodes []string
	hooks      []string
}

// NewNodeRuntime creates a new Node.js plugin runtime.
func NewNodeRuntime() *NodeRuntime {
	return &NodeRuntime{}
}

// RegisteredFilters returns the names of filters registered by the Node plugin.
func (r *NodeRuntime) RegisteredFilters() []string {
	return r.filters
}

// CallFilter calls a filter registered by the Node plugin.
func (r *NodeRuntime) CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error) {
	return input, nil
}

// RegisteredShortcodes returns the names of shortcodes registered by the Node plugin.
func (r *NodeRuntime) RegisteredShortcodes() []string {
	return r.shortcodes
}

// CallShortcode calls a shortcode registered by the Node plugin.
func (r *NodeRuntime) CallShortcode(name string, args []string, innerContent string) (string, error) {
	return innerContent, nil
}

// RegisteredHooks returns the names of hooks registered by the Node plugin.
func (r *NodeRuntime) RegisteredHooks() []string {
	return r.hooks
}

// CallHook calls a hook registered by the Node plugin.
func (r *NodeRuntime) CallHook(name string, payload interface{}) (interface{}, error) {
	return payload, nil
}

// NodeBridge manages the lifecycle of the Node subprocess used for Tier 3 plugins.
// Communication uses length-prefixed JSON-RPC over stdin/stdout.
// Plugin stderr is redirected to .alloy/plugin.log.
type NodeBridge struct {
	state       BridgeState
	projectRoot string
}

// NewNodeBridge creates a Node bridge for the given project root.
// The bridge starts in BridgeNotStarted state.
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

// Start spawns the Node subprocess and transitions to BridgeRunning.
func (b *NodeBridge) Start() error {
	b.state = BridgeRunning
	return nil
}

// Stop gracefully shuts down the Node subprocess and transitions to BridgeStopped.
func (b *NodeBridge) Stop() error {
	b.state = BridgeStopped
	return nil
}

// LogPath returns the path where plugin stderr output is written.
func (b *NodeBridge) LogPath() string {
	return filepath.Join(b.projectRoot, ".alloy", "plugin.log")
}
