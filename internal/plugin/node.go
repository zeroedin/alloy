package plugin

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
	return nil, ErrNotImplemented
}

// DecodeMessage parses an LSP-style framed byte sequence back into a Message.
// Returns an error if the Content-Length header is missing or malformed.
func DecodeMessage(data []byte) (*Message, error) {
	return nil, ErrNotImplemented
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
	return ErrNotImplemented
}

// Stop gracefully shuts down the Node subprocess and transitions to BridgeStopped.
func (b *NodeBridge) Stop() error {
	return ErrNotImplemented
}

// LogPath returns the path where plugin stderr output is written.
func (b *NodeBridge) LogPath() string {
	return ""
}
