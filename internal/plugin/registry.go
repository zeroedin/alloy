package plugin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
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
// filters, shortcodes, and hooks to the template engine and hook registry.
// QuickJSRuntime, WASMRuntime, and NodeRuntime all implement this interface.
type PluginFilterRuntime interface {
	RegisteredFilters() []string
	CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error)
	RegisteredShortcodes() []string
	CallShortcode(name string, args []string, innerContent string) (string, error)
	RegisteredHooks() []string
	// SetSiteData injects site data so plugins can access it (e.g. alloy.data in JS).
	// Implementations must treat nil as an empty map. Data values must be JSON-serializable.
	SetSiteData(data map[string]interface{}) error
}

// HookDetailer is implemented by runtimes that can report hook priorities.
// Runtimes without priority support fall back to RegisteredHooks() with default priority 50.
type HookDetailer interface {
	RegisteredHookDetails() []HookRegistration
}

// EvalWarner is implemented by runtimes that collect warnings during plugin evaluation.
type EvalWarner interface {
	EvalWarnings() []string
}

// initializedPlugin pairs a discovered plugin with its Phase-A-initialized runtime.
type initializedPlugin struct {
	info    PluginInfo
	runtime PluginFilterRuntime
}

// Registry manages plugin discovery and loading.
type Registry struct {
	pluginsDir      string
	plugins         []PluginInfo
	filterRegistry  map[string]string       // filter name → source
	conflictWarns   []string
	runtimes        []PluginFilterRuntime   // loaded runtimes for filter/shortcode bridging
	preInitialized  []initializedPlugin     // Phase A results, consumed by LoadPlugins
	phaseADone      bool                    // true after InitRuntimes completes
	wasmCacheDir    string                  // persistent compilation cache for WASM modules
	wasmCache       wazero.CompilationCache // shared cache, closed in Close()
}

// NewRegistry creates a plugin registry for the given plugins directory.
func NewRegistry(pluginsDir string) *Registry {
	return &Registry{
		pluginsDir:     pluginsDir,
		filterRegistry: make(map[string]string),
	}
}

// SetWASMCacheDir configures a persistent compilation cache directory
// for WASM modules. When set, compiled native code is reused across builds.
// Creates the cache eagerly so it can be shared across all WASM runtimes.
func (r *Registry) SetWASMCacheDir(dir string) {
	r.wasmCacheDir = dir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	cache, err := wazero.NewCompilationCacheWithDir(dir)
	if err != nil {
		return
	}
	r.wasmCache = cache
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

// HasFilter reports whether a filter with the given name has been registered.
func (r *Registry) HasFilter(name string) bool {
	_, ok := r.filterRegistry[name]
	return ok
}

// Runtimes returns all loaded plugin runtimes for filter/shortcode bridging.
func (r *Registry) Runtimes() []PluginFilterRuntime {
	return r.runtimes
}

// Close releases resources held by all loaded runtimes and any
// pre-initialized runtimes that were never consumed by LoadPlugins.
func (r *Registry) Close() {
	for _, rt := range r.runtimes {
		closeRuntime(rt)
	}
	r.runtimes = nil
	for _, ip := range r.preInitialized {
		closeRuntime(ip.runtime)
	}
	r.preInitialized = nil
	if r.wasmCache != nil {
		r.wasmCache.Close(context.Background())
		r.wasmCache = nil
	}
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

// registerRuntime registers a loaded runtime's filters and hooks, and appends
// it to the registry's runtime list. Shared by both loading paths.
func (r *Registry) registerRuntime(rt PluginFilterRuntime, pluginName string, hooks *HookRegistry) {
	for _, fname := range rt.RegisteredFilters() {
		r.RegisterFilter(fname, "plugins/"+pluginName)
	}
	if caller, ok := rt.(interface {
		CallHook(string, interface{}) (interface{}, error)
	}); ok {
		batcher, hasBatch := rt.(interface {
			BatchCallHook(string, []interface{}) ([]interface{}, error)
		})

		if hd, ok := rt.(HookDetailer); ok {
			for _, reg := range hd.RegisteredHookDetails() {
				name := reg.Name
				singleFn := func(ctx context.Context, payload interface{}) (interface{}, error) {
					return caller.CallHook(name, payload)
				}
				if reg.Scope != nil {
					if err := ValidateScope(HookName(name), *reg.Scope); err != nil {
						hooks.warnings = append(hooks.warnings, fmt.Sprintf("plugin %s: hook %s: %v", pluginName, name, err))
					}
					if hasBatch {
						batchFn := func(ctx context.Context, payloads []interface{}) ([]interface{}, error) {
							return batcher.BatchCallHook(name, payloads)
						}
						hooks.RegisterBatchWithOptions(HookName(name), singleFn, batchFn, *reg.Scope, reg.Priority)
					} else {
						hooks.RegisterWithOptions(HookName(name), singleFn, *reg.Scope, reg.Priority)
					}
				} else {
					if hasBatch {
						batchFn := func(ctx context.Context, payloads []interface{}) ([]interface{}, error) {
							return batcher.BatchCallHook(name, payloads)
						}
						hooks.RegisterBatchWithPriority(HookName(name), singleFn, batchFn, reg.Priority)
					} else {
						hooks.RegisterWithPriority(HookName(name), singleFn, reg.Priority)
					}
				}
			}
		} else {
			for _, hookName := range rt.RegisteredHooks() {
				name := hookName
				singleFn := func(ctx context.Context, payload interface{}) (interface{}, error) {
					return caller.CallHook(name, payload)
				}
				if hasBatch {
					batchFn := func(ctx context.Context, payloads []interface{}) ([]interface{}, error) {
						return batcher.BatchCallHook(name, payloads)
					}
					hooks.RegisterBatchWithPriority(HookName(name), singleFn, batchFn, 50)
				} else {
					hooks.Register(HookName(name), singleFn)
				}
			}
		}
	}
	if warner, ok := rt.(EvalWarner); ok {
		for _, w := range warner.EvalWarnings() {
			hooks.warnings = append(hooks.warnings, fmt.Sprintf("plugin %s: %s", pluginName, w))
		}
	}
	r.runtimes = append(r.runtimes, rt)
}

// closeRuntime closes a runtime if it implements Close().
func closeRuntime(rt PluginFilterRuntime) {
	if c, ok := rt.(interface{ Close() }); ok {
		c.Close()
	}
}

// InitRuntimes concurrently initializes runtimes for all discovered plugins
// (Phase A). Runtimes are created and compiled but not evaluated — no filters
// or hooks are registered. Call LoadPlugins after to complete Phase B.
//
// QuickJS and WASM plugins run in goroutines (CPU-bound compilation).
// Node plugins are initialized sequentially on the calling goroutine
// because subprocess spawn is I/O-bound with side effects.
func (r *Registry) InitRuntimes() ([]PluginFilterRuntime, []string) {
	type result struct {
		idx     int
		plugin  initializedPlugin
		warning string
	}

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results []result
	)

	nodeAvailableErr := CheckNodeAvailable()

	for i, p := range r.plugins {
		if p.Runtime == RuntimeNode {
			if nodeAvailableErr != nil {
				results = append(results, result{idx: i, warning: fmt.Sprintf("plugin %s: %v", p.Name, nodeAvailableErr)})
				continue
			}
			rt := NewNodeRuntime()
			rt.SetProjectRoot(filepath.Dir(filepath.Clean(r.pluginsDir)))
			results = append(results, result{idx: i, plugin: initializedPlugin{info: p, runtime: rt}})
			continue
		}

		wg.Add(1)
		go func(idx int, p PluginInfo) {
			defer wg.Done()
			var res result
			res.idx = idx

			switch p.Runtime {
			case RuntimeQuickJS:
				rt := NewQuickJSRuntime()
				if err := rt.Init(); err != nil {
					res.warning = fmt.Sprintf("plugin %s: init failed: %v", p.Name, err)
				} else {
					res.plugin = initializedPlugin{info: p, runtime: rt}
				}
			case RuntimeWASM:
				rt := NewWASMRuntime()
				if r.wasmCache != nil {
					rt.SetCompilationCache(r.wasmCache)
				} else if r.wasmCacheDir != "" {
					rt.SetCacheDir(r.wasmCacheDir)
				}
				if err := rt.LoadModule(p.Path); err != nil {
					res.warning = fmt.Sprintf("plugin %s: load failed: %v", p.Name, err)
				} else {
					res.plugin = initializedPlugin{info: p, runtime: rt}
				}
			}

			mu.Lock()
			results = append(results, res)
			mu.Unlock()
		}(i, p)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool { return results[i].idx < results[j].idx })

	var runtimes []PluginFilterRuntime
	var warnings []string
	for _, ip := range r.preInitialized {
		closeRuntime(ip.runtime)
	}
	r.preInitialized = nil
	for _, res := range results {
		if res.warning != "" {
			warnings = append(warnings, res.warning)
			continue
		}
		if res.plugin.runtime != nil {
			runtimes = append(runtimes, res.plugin.runtime)
			r.preInitialized = append(r.preInitialized, res.plugin)
		}
	}
	r.phaseADone = true

	return runtimes, warnings
}

// LoadPlugins loads all discovered plugins into the given HookRegistry.
// If InitRuntimes was called first, uses pre-initialized runtimes (Phase B only).
// Otherwise initializes and evaluates sequentially (both phases).
// Returns warnings for plugins that fail to load (non-fatal).
func (r *Registry) LoadPlugins(hooks *HookRegistry) []string {
	if r.phaseADone {
		return r.loadPreInitialized(hooks)
	}
	return r.loadSequential(hooks)
}

// loadPreInitialized completes Phase B for runtimes that were pre-initialized
// by InitRuntimes: eval plugin source and register filters/hooks.
func (r *Registry) loadPreInitialized(hooks *HookRegistry) []string {
	var warnings []string
	for _, ip := range r.preInitialized {
		switch ip.info.Runtime {
		case RuntimeQuickJS:
			rt := ip.runtime.(*QuickJSRuntime)
			if err := rt.EvalFile(ip.info.Path); err != nil {
				closeRuntime(rt)
				warnings = append(warnings, fmt.Sprintf("plugin %s: eval failed: %v", ip.info.Name, err))
				continue
			}
			r.registerRuntime(rt, ip.info.Name, hooks)
		case RuntimeWASM:
			r.registerRuntime(ip.runtime, ip.info.Name, hooks)
		case RuntimeNode:
			rt := ip.runtime.(*NodeRuntime)
			if err := rt.EvalFile(ip.info.Path); err != nil {
				closeRuntime(rt)
				warnings = append(warnings, fmt.Sprintf("plugin %s: eval failed: %v", ip.info.Name, err))
				continue
			}
			r.registerRuntime(rt, ip.info.Name, hooks)
		}
	}
	r.preInitialized = nil
	return warnings
}

// loadSequential initializes and evaluates all plugins sequentially (both phases).
func (r *Registry) loadSequential(hooks *HookRegistry) []string {
	var warnings []string
	nodeAvailableErr := CheckNodeAvailable()
	for _, p := range r.plugins {
		switch p.Runtime {
		case RuntimeQuickJS:
			rt := NewQuickJSRuntime()
			if err := rt.Init(); err != nil {
				warnings = append(warnings, fmt.Sprintf("plugin %s: init failed: %v", p.Name, err))
				continue
			}
			if err := rt.EvalFile(p.Path); err != nil {
				closeRuntime(rt)
				warnings = append(warnings, fmt.Sprintf("plugin %s: eval failed: %v", p.Name, err))
				continue
			}
			r.registerRuntime(rt, p.Name, hooks)
		case RuntimeWASM:
			rt := NewWASMRuntime()
			if r.wasmCache != nil {
				rt.SetCompilationCache(r.wasmCache)
			} else if r.wasmCacheDir != "" {
				rt.SetCacheDir(r.wasmCacheDir)
			}
			if err := rt.LoadModule(p.Path); err != nil {
				warnings = append(warnings, fmt.Sprintf("plugin %s: load failed: %v", p.Name, err))
				continue
			}
			r.registerRuntime(rt, p.Name, hooks)
		case RuntimeNode:
			if nodeAvailableErr != nil {
				warnings = append(warnings, fmt.Sprintf("plugin %s: %v", p.Name, nodeAvailableErr))
				continue
			}
			rt := NewNodeRuntime()
			rt.SetProjectRoot(filepath.Dir(filepath.Clean(r.pluginsDir)))
			if err := rt.EvalFile(p.Path); err != nil {
				closeRuntime(rt)
				warnings = append(warnings, fmt.Sprintf("plugin %s: eval failed: %v", p.Name, err))
				continue
			}
			r.registerRuntime(rt, p.Name, hooks)
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
