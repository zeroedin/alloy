package plugin

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

// Registry manages plugin discovery and loading.
type Registry struct {
	pluginsDir string
}

// NewRegistry creates a plugin registry for the given plugins directory.
func NewRegistry(pluginsDir string) *Registry {
	return &Registry{pluginsDir: pluginsDir}
}

// DiscoverPlugins scans the plugins directory and loads plugins by file extension.
func (r *Registry) DiscoverPlugins() error {
	return ErrNotImplemented
}

// Plugins returns the list of discovered plugins after DiscoverPlugins is called.
// Plugins are sorted by load order: Tier 1 first, then Tier 2, then Tier 3,
// with alphabetical filename order within each tier.
func (r *Registry) Plugins() []PluginInfo {
	return nil
}

// ConflictWarnings returns any name conflict warnings produced during plugin loading.
// A conflict occurs when two plugins register the same filter or shortcode name.
func (r *Registry) ConflictWarnings() []string {
	return nil
}

// RegisterFilter records a filter registration and tracks name conflicts.
// If a filter with the same name was already registered, a warning is recorded.
func (r *Registry) RegisterFilter(name, source string) {
	// stub — no-op
}

// ClassifyPlugin determines the tier and runtime for a plugin file based on
// its extension and (for .js/.ts files) whether it exports runtime: "node".
func ClassifyPlugin(path string) (*PluginInfo, error) {
	return nil, ErrNotImplemented
}

// CheckNodeAvailable verifies that the `node` binary is available in PATH.
// Returns a descriptive error if Node plugins exist but node is not found.
func CheckNodeAvailable() error {
	return ErrNotImplemented
}
