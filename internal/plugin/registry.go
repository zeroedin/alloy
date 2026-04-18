package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// PluginTier represents the execution tier of a plugin.
type PluginTier int

const (
	// TierBuiltIn is Tier 1: Go built-in filters compiled into the binary.
	TierBuiltIn PluginTier = iota + 1
	// TierInProcess is Tier 2: In-process plugins via wazero (QuickJS or WASM).
	TierInProcess
	// TierNode is Tier 3: Node subprocess plugins via IPC bridge.
	TierNode
)

// PluginRuntime distinguishes sub-types within a tier.
type PluginRuntime string

const (
	RuntimeGoBuiltIn PluginRuntime = "go"
	RuntimeQuickJS   PluginRuntime = "quickjs"
	RuntimeWASM      PluginRuntime = "wasm"
	RuntimeNode      PluginRuntime = "node"
)

// PluginInfo describes a discovered plugin file.
type PluginInfo struct {
	Path    string        // File path relative to plugins dir
	Name    string        // Plugin name (filename without extension)
	Tier    PluginTier    // Execution tier
	Runtime PluginRuntime // Specific runtime within the tier
}

// PluginFilterRuntime is the interface for plugin runtimes that can provide
// filters and shortcodes to the template engine. Both QuickJSRuntime and
// WASMRuntime implement this interface.
type PluginFilterRuntime interface {
	RegisteredFilters() []string
	CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error)
	RegisteredShortcodes() []string
	CallShortcode(name string, args []string, innerContent string) (string, error)
}

// Registry manages plugin discovery and loading.
type Registry struct {
	pluginsDir      string
	plugins         []PluginInfo
	filterRegistry  map[string]string      // filter name → source
	conflictWarns   []string
	runtimes        []PluginFilterRuntime   // loaded runtimes for filter/shortcode bridging
}

// NewRegistry creates a plugin registry for the given plugins directory.
func NewRegistry(pluginsDir string) *Registry {
	return &Registry{
		pluginsDir:     pluginsDir,
		filterRegistry: make(map[string]string),
	}
}

// supportedExtensions lists file extensions recognized as plugins.
var supportedExtensions = map[string]bool{
	".js":   true,
	".ts":   true,
	".wasm": true,
}

// DiscoverPlugins scans the plugins directory and loads plugins by file extension.
func (r *Registry) DiscoverPlugins() error {
	entries, err := os.ReadDir(r.pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading plugins directory: %w", err)
	}

	var discovered []PluginInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if !supportedExtensions[ext] {
			continue
		}

		path := filepath.Join(r.pluginsDir, entry.Name())
		info, err := ClassifyPlugin(path)
		if err != nil {
			continue
		}
		discovered = append(discovered, *info)
	}

	// Sort by tier first, then alphabetically by name within each tier
	sort.Slice(discovered, func(i, j int) bool {
		if discovered[i].Tier != discovered[j].Tier {
			return discovered[i].Tier < discovered[j].Tier
		}
		return discovered[i].Name < discovered[j].Name
	})

	r.plugins = discovered
	return nil
}

// Plugins returns the list of discovered plugins after DiscoverPlugins is called.
func (r *Registry) Plugins() []PluginInfo {
	return r.plugins
}

// ConflictWarnings returns any name conflict warnings produced during plugin loading.
func (r *Registry) ConflictWarnings() []string {
	return r.conflictWarns
}

// RegisterFilter records a filter registration and tracks name conflicts.
func (r *Registry) RegisterFilter(name, source string) {
	if existing, ok := r.filterRegistry[name]; ok {
		r.conflictWarns = append(r.conflictWarns,
			fmt.Sprintf("filter %q conflict: registered by %s and %s", name, existing, source))
	}
	r.filterRegistry[name] = source
}

// Runtimes returns all loaded QuickJS runtimes for filter bridging.
func (r *Registry) Runtimes() []PluginFilterRuntime {
	return r.runtimes
}

// Close releases resources held by all loaded runtimes.
func (r *Registry) Close() {
	for _, rt := range r.runtimes {
		if c, ok := rt.(interface{ Close() }); ok {
			c.Close()
		}
	}
	r.runtimes = nil
}

// ClassifyPlugin determines the tier and runtime for a plugin file based on
// its extension and (for .js/.ts files) whether it exports runtime: "node".
func ClassifyPlugin(path string) (*PluginInfo, error) {
	ext := filepath.Ext(path)
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, ext)

	switch ext {
	case ".wasm":
		return &PluginInfo{
			Path:    path,
			Name:    name,
			Tier:    TierInProcess,
			Runtime: RuntimeWASM,
		}, nil

	case ".js", ".ts":
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading plugin file %s: %w", path, err)
		}

		src := string(content)
		if hasNodeRuntimeExport(src) {
			return &PluginInfo{
				Path:    path,
				Name:    name,
				Tier:    TierNode,
				Runtime: RuntimeNode,
			}, nil
		}

		return &PluginInfo{
			Path:    path,
			Name:    name,
			Tier:    TierInProcess,
			Runtime: RuntimeQuickJS,
		}, nil

	default:
		return nil, fmt.Errorf("unsupported plugin extension %q", ext)
	}
}

// hasNodeRuntimeExport checks if JS/TS source declares runtime = "node".
func hasNodeRuntimeExport(src string) bool {
	// Match patterns like: export const runtime = "node"
	return strings.Contains(src, `runtime = "node"`) ||
		strings.Contains(src, `runtime = 'node'`) ||
		strings.Contains(src, "runtime: \"node\"") ||
		strings.Contains(src, "runtime: 'node'")
}

// LoadPlugins loads all discovered plugins into the given HookRegistry.
// Tier 2 (QuickJS/WASM) plugins are evaluated in-process.
// Tier 3 (Node) plugins require a running NodeBridge.
// Returns warnings for plugins that fail to load (non-fatal).
func (r *Registry) LoadPlugins(hooks *HookRegistry) []string {
	var warnings []string
	for _, p := range r.plugins {
		switch p.Runtime {
		case RuntimeQuickJS:
			rt := NewQuickJSRuntime()
			if err := rt.Init(); err != nil {
				warnings = append(warnings, fmt.Sprintf("plugin %s: init failed: %v", p.Name, err))
				continue
			}
			if err := rt.EvalFile(p.Path); err != nil {
				warnings = append(warnings, fmt.Sprintf("plugin %s: eval failed: %v", p.Name, err))
				continue
			}
			// Register discovered filters with the registry for conflict detection
			for _, fname := range rt.RegisteredFilters() {
				r.RegisterFilter(fname, "plugins/"+p.Name)
			}
			// Bridge discovered hooks into the HookRegistry
			for _, hookName := range rt.RegisteredHooks() {
				name := hookName
				runtime := rt
				hooks.Register(HookName(name), func(ctx context.Context, payload interface{}) (interface{}, error) {
					return runtime.CallHook(name, payload)
				})
			}
			r.runtimes = append(r.runtimes, rt)
		case RuntimeWASM:
			rt := NewWASMRuntime()
			if err := rt.LoadModule(p.Path); err != nil {
				warnings = append(warnings, fmt.Sprintf("plugin %s: load failed: %v", p.Name, err))
				continue
			}
			for _, fname := range rt.RegisteredFilters() {
				r.RegisterFilter(fname, "plugins/"+p.Name)
			}
			r.runtimes = append(r.runtimes, rt)
		case RuntimeNode:
			if err := CheckNodeAvailable(); err != nil {
				warnings = append(warnings, fmt.Sprintf("plugin %s: %v", p.Name, err))
				continue
			}
			// Node plugins are loaded via the NodeBridge at runtime.
			// Hook registration happens through the JSON-RPC protocol.
		}
	}
	return warnings
}

// CheckNodeAvailable verifies that the `node` binary is available in PATH.
func CheckNodeAvailable() error {
	_, err := exec.LookPath("node")
	if err != nil {
		return fmt.Errorf("node not found in PATH: Tier 3 plugins require Node.js")
	}
	return nil
}
