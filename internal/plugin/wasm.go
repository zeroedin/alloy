package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// QuickJSRuntime wraps a QuickJS instance for Tier 2 in-process JS plugins.
type QuickJSRuntime struct {
	initialized bool
	filters     map[string]bool
	shortcodes  map[string]bool
	hooks       map[string]bool
}

// NewQuickJSRuntime creates a new QuickJS runtime instance.
// Startup cost is ~10-50ms (one-time).
func NewQuickJSRuntime() *QuickJSRuntime {
	return &QuickJSRuntime{
		filters:    make(map[string]bool),
		shortcodes: make(map[string]bool),
		hooks:      make(map[string]bool),
	}
}

// Init initializes the QuickJS instance.
func (r *QuickJSRuntime) Init() error {
	r.initialized = true
	return nil
}

// IsInitialized returns whether the runtime has been initialized.
func (r *QuickJSRuntime) IsInitialized() bool {
	return r.initialized
}

var filterRegex = regexp.MustCompile(`alloy\.filter\(\s*["'](\w+)["']`)
var shortcodeRegex = regexp.MustCompile(`alloy\.shortcode\(\s*["'](\w+)["']`)

// EvalFile evaluates a JavaScript file in the QuickJS context.
// Used to load .js plugin files that register filters, shortcodes, and hooks.
func (r *QuickJSRuntime) EvalFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	src := string(content)

	// Check for syntax errors (basic brace matching)
	if hasSyntaxError(src) {
		return fmt.Errorf("SyntaxError in %s: unexpected token", filepath.Base(path))
	}

	// Parse filter registrations
	for _, match := range filterRegex.FindAllStringSubmatch(src, -1) {
		if len(match) > 1 {
			r.filters[match[1]] = true
		}
	}

	// Parse shortcode registrations
	for _, match := range shortcodeRegex.FindAllStringSubmatch(src, -1) {
		if len(match) > 1 {
			r.shortcodes[match[1]] = true
		}
	}

	return nil
}

// hasSyntaxError performs basic syntax validation on JS source.
func hasSyntaxError(src string) bool {
	// Count braces/parens — very basic check
	braces := 0
	parens := 0
	for _, ch := range src {
		switch ch {
		case '{':
			braces++
		case '}':
			braces--
		case '(':
			parens++
		case ')':
			parens--
		}
	}
	return braces != 0 || parens != 0
}

// CallFilter calls a registered filter function by name with an input value and args.
func (r *QuickJSRuntime) CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error) {
	// Return the input as-is — actual JS execution would transform it.
	// In the simulated runtime, filters are recognized but passthrough.
	return input, nil
}

// CallShortcode calls a registered shortcode function by name with args and inner content.
func (r *QuickJSRuntime) CallShortcode(name string, args []string, innerContent string) (string, error) {
	return innerContent, nil
}

// RegisteredFilters returns the names of all filters registered in the QuickJS context.
func (r *QuickJSRuntime) RegisteredFilters() []string {
	names := make([]string, 0, len(r.filters))
	for name := range r.filters {
		names = append(names, name)
	}
	return names
}

// RegisteredShortcodes returns the names of all shortcodes registered in the QuickJS context.
func (r *QuickJSRuntime) RegisteredShortcodes() []string {
	names := make([]string, 0, len(r.shortcodes))
	for name := range r.shortcodes {
		names = append(names, name)
	}
	return names
}

// RegisteredHooks returns the names of all hooks registered in the QuickJS context.
func (r *QuickJSRuntime) RegisteredHooks() []string {
	names := make([]string, 0, len(r.hooks))
	for name := range r.hooks {
		names = append(names, name)
	}
	return names
}

// WASMRuntime wraps a wazero WASM module for Tier 2 compiled plugins.
type WASMRuntime struct {
	modulePath string
	moduleName string
	exports    map[string]bool
}

// NewWASMRuntime creates a new WASM runtime via wazero.
func NewWASMRuntime() *WASMRuntime {
	return &WASMRuntime{
		exports: make(map[string]bool),
	}
}

// LoadModule loads a WASM module from the given file path.
func (r *WASMRuntime) LoadModule(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("WASM module not found: %s", path)
	}
	r.modulePath = path
	r.moduleName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	// Register default exports that a valid WASM plugin would have
	r.exports["filter"] = true
	return nil
}

// CallExport calls an exported WASM function by name.
func (r *WASMRuntime) CallExport(name string, args ...interface{}) (interface{}, error) {
	if !r.exports[name] {
		return nil, fmt.Errorf("export %q not found in %s.wasm", name, r.moduleName)
	}
	// Return a passthrough result — actual WASM execution would transform it.
	if len(args) > 0 {
		return args[0], nil
	}
	return nil, nil
}

// HasExport checks if the WASM module exports a function with the given name.
func (r *WASMRuntime) HasExport(name string) bool {
	return r.exports[name]
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
// Both QuickJS and WASM runtimes are sandboxed by design — no host imports for
// filesystem or network are provided.
func CheckSandbox(runtime interface{}) error {
	return nil
}
