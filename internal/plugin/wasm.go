package plugin

// QuickJSRuntime wraps a QuickJS instance for Tier 2 in-process JS plugins.
type QuickJSRuntime struct {
	initialized bool
}

// NewQuickJSRuntime creates a new QuickJS runtime instance.
// Startup cost is ~10-50ms (one-time).
func NewQuickJSRuntime() *QuickJSRuntime {
	return &QuickJSRuntime{}
}

// Init initializes the QuickJS instance.
func (r *QuickJSRuntime) Init() error {
	return ErrNotImplemented
}

// IsInitialized returns whether the runtime has been initialized.
func (r *QuickJSRuntime) IsInitialized() bool {
	return r.initialized
}

// EvalFile evaluates a JavaScript file in the QuickJS context.
// Used to load .js plugin files that register filters, shortcodes, and hooks.
func (r *QuickJSRuntime) EvalFile(path string) error {
	return ErrNotImplemented
}

// CallFilter calls a registered filter function by name with an input value and args.
func (r *QuickJSRuntime) CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error) {
	return nil, ErrNotImplemented
}

// CallShortcode calls a registered shortcode function by name with args and inner content.
func (r *QuickJSRuntime) CallShortcode(name string, args []string, innerContent string) (string, error) {
	return "", ErrNotImplemented
}

// RegisteredFilters returns the names of all filters registered in the QuickJS context.
func (r *QuickJSRuntime) RegisteredFilters() []string {
	return nil
}

// RegisteredShortcodes returns the names of all shortcodes registered in the QuickJS context.
func (r *QuickJSRuntime) RegisteredShortcodes() []string {
	return nil
}

// WASMRuntime wraps a wazero WASM module for Tier 2 compiled plugins.
type WASMRuntime struct{}

// NewWASMRuntime creates a new WASM runtime via wazero.
func NewWASMRuntime() *WASMRuntime {
	return &WASMRuntime{}
}

// LoadModule loads a WASM module from the given file path.
func (r *WASMRuntime) LoadModule(path string) error {
	return ErrNotImplemented
}

// CallExport calls an exported WASM function by name.
func (r *WASMRuntime) CallExport(name string, args ...interface{}) (interface{}, error) {
	return nil, ErrNotImplemented
}

// HasExport checks if the WASM module exports a function with the given name.
func (r *WASMRuntime) HasExport(name string) bool {
	return false
}

// SandboxViolationError represents an attempt to access a forbidden resource.
type SandboxViolationError struct {
	Resource string // "filesystem", "network", etc.
	Detail   string
}

func (e *SandboxViolationError) Error() string {
	return "sandbox violation: " + e.Resource + ": " + e.Detail
}

// CheckSandbox validates that the Tier 2 runtime has no filesystem or network access.
func CheckSandbox(runtime interface{}) error {
	return ErrNotImplemented
}
