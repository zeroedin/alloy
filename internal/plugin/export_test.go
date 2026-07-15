package plugin

import (
	"bufio"
	"io"
	"path/filepath"
)

var ParseScopeJSON = parseScopeJSON
var ParseScopeMap = parseScopeMap

// ResetStalePIDCleanup clears the once-per-root guard so each test
// gets a fresh cleanup pass regardless of execution order.
func ResetStalePIDCleanup(projectRoot string) {
	key := filepath.Clean(projectRoot)
	stalePIDCleanupMu.Lock()
	defer stalePIDCleanupMu.Unlock()
	delete(stalePIDCleanupRoots, key)
}

// AppendHook appends a hook name to a NodeRuntime's internal hooks slice for testing.
func AppendHook(r *NodeRuntime, name string) {
	r.hooks = append(r.hooks, name)
}

// RegisterRuntime exposes Registry.registerRuntime for testing EvalWarner forwarding.
func RegisterRuntime(reg *Registry, rt PluginFilterRuntime, name string, hooks *HookRegistry) {
	reg.registerRuntime(rt, name, hooks)
}

// HookPriorities returns the priorities of all registered hooks for a given event.
func HookPriorities(r *HookRegistry, event HookName) []int {
	hooks := r.hooks[event]
	priorities := make([]int, len(hooks))
	for i, h := range hooks {
		priorities[i] = h.priority
	}
	return priorities
}

// nopWriteCloser wraps an io.Writer to satisfy io.WriteCloser.
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// NewBridgeWithReader creates a NodeBridge wired to the given reader
// for testing Send's error-path diagnostic without a live subprocess.
// The bridge is set to BridgeRunning state with stdin going to io.Discard.
func NewBridgeWithReader(reader *bufio.Reader) *NodeBridge {
	return &NodeBridge{
		state:  BridgeRunning,
		stdout: reader,
		stdin:  nopWriteCloser{io.Discard},
	}
}
