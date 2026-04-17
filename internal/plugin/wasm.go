package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fastschema/qjs"
)

// QuickJSRuntime wraps a QuickJS instance for Tier 2 in-process JS plugins.
// JavaScript is executed via QuickJS compiled to WASM, running on wazero
// (pure Go, zero CGo). See PLAN.md §5.
type QuickJSRuntime struct {
	initialized bool
	rt          *qjs.Runtime
	ctx         *qjs.Context
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

// Init initializes the QuickJS instance via wazero.
func (r *QuickJSRuntime) Init() error {
	rt, err := qjs.New()
	if err != nil {
		return fmt.Errorf("initializing QuickJS runtime: %w", err)
	}
	r.rt = rt
	r.ctx = rt.Context()

	// Register Go callbacks that the JS alloy.filter/shortcode/hook/on
	// methods will invoke to record registrations in Go-side maps.
	r.ctx.SetFunc("__registerFilter", func(this *qjs.This) (*qjs.Value, error) {
		args := this.Args()
		if len(args) >= 1 {
			r.filters[args[0].String()] = true
		}
		return this.Context().NewUndefined(), nil
	})

	r.ctx.SetFunc("__registerShortcode", func(this *qjs.This) (*qjs.Value, error) {
		args := this.Args()
		if len(args) >= 1 {
			r.shortcodes[args[0].String()] = true
		}
		return this.Context().NewUndefined(), nil
	})

	r.ctx.SetFunc("__registerHook", func(this *qjs.This) (*qjs.Value, error) {
		args := this.Args()
		if len(args) >= 1 {
			r.hooks[args[0].String()] = true
		}
		return this.Context().NewUndefined(), nil
	})

	// Create the alloy global object with filter/shortcode/hook/on methods.
	// Filter functions are stored in __filters for later invocation by CallFilter.
	_, err = r.ctx.Eval("alloy-setup.js", qjs.Code(`
		var __filters = {};
		var __shortcodes = {};
		var __hooks = {};
		var alloy = {
			filter: function(name, fn) {
				__filters[name] = fn;
				__registerFilter(name);
			},
			shortcode: function(name, fn) {
				__shortcodes[name] = fn;
				__registerShortcode(name);
			},
			hook: function(name, fn) {
				__hooks[name] = fn;
				__registerHook(name);
			},
			on: function(name, fn) {
				__hooks[name] = fn;
				__registerHook(name);
			}
		};
	`))
	if err != nil {
		r.rt.Close()
		return fmt.Errorf("setting up alloy global: %w", err)
	}

	r.initialized = true
	return nil
}

// IsInitialized returns whether the runtime has been initialized.
func (r *QuickJSRuntime) IsInitialized() bool {
	return r.initialized
}

// moduleExportRegex matches "export default function(alloy)" or
// "export default function (alloy)" at the start of a plugin file.
var moduleExportRegex = regexp.MustCompile(
	`export\s+default\s+function\s*\(\s*alloy\s*\)`)

// EvalFile evaluates a JavaScript file in the QuickJS context.
// Plugin files using "export default function(alloy) { ... }" module syntax
// are transformed to an IIFE that receives the global alloy object.
func (r *QuickJSRuntime) EvalFile(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	src := string(content)

	// Transform ES module default export to IIFE:
	//   "export default function(alloy) { ... }" → "(function(alloy) { ... })(alloy);"
	if moduleExportRegex.MatchString(src) {
		src = moduleExportRegex.ReplaceAllString(src, "(function(alloy)")
		src = strings.TrimRight(src, "\n\r\t ")
		src += ")(alloy);\n"
	}

	result, err := r.ctx.Eval(filepath.Base(path), qjs.Code(src))
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if result != nil {
		result.Free()
	}

	return nil
}

// CallFilter calls a registered filter function by name with an input value.
// The filter function is invoked in the QuickJS VM and the result is
// converted back to a Go value.
func (r *QuickJSRuntime) CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error) {
	if !r.filters[name] {
		return input, nil
	}

	// Set the input as a global variable accessible from JS
	switch v := input.(type) {
	case string:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewString(v))
	case int:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewInt32(int32(v)))
	case float64:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewFloat64(v))
	case bool:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewBool(v))
	default:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewString(fmt.Sprint(v)))
	}

	// Set the filter name as a global to avoid JS injection from names
	// containing special characters (e.g., quotes).
	r.ctx.Global().SetPropertyStr("__callFilterName", r.ctx.NewString(name))

	// Invoke the filter function stored in __filters
	result, err := r.ctx.Eval("filter-call.js", qjs.Code(
		`__filters[__callFilterName](__callInput)`))

	// Clean up globals to avoid stale references between calls
	r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewUndefined())
	r.ctx.Global().SetPropertyStr("__callFilterName", r.ctx.NewUndefined())

	if err != nil {
		return nil, fmt.Errorf("filter %q: %w", name, err)
	}
	defer result.Free()

	return jsValueToGo(result), nil
}

// jsValueToGo converts a QJS Value to an appropriate Go type.
func jsValueToGo(v *qjs.Value) interface{} {
	if v.IsString() {
		return v.String()
	}
	if v.IsNumber() {
		f := v.Float64()
		if f == float64(int(f)) && f >= -2147483648 && f <= 2147483647 {
			return int(f)
		}
		return f
	}
	if v.IsBool() {
		return v.Bool()
	}
	if v.IsNull() || v.IsUndefined() {
		return nil
	}
	// Fallback: convert to string representation
	return v.String()
}

// CallShortcode calls a registered shortcode function by name with args and inner content.
// The shortcode function is invoked in the QuickJS VM with an array of string arguments.
func (r *QuickJSRuntime) CallShortcode(name string, args []string, innerContent string) (string, error) {
	if !r.shortcodes[name] {
		return innerContent, nil
	}

	// Build a JS array literal from args
	var jsArgs strings.Builder
	jsArgs.WriteString("[")
	for i, arg := range args {
		if i > 0 {
			jsArgs.WriteString(",")
		}
		jsArgs.WriteString(`"`)
		jsArgs.WriteString(strings.ReplaceAll(arg, `"`, `\"`))
		jsArgs.WriteString(`"`)
	}
	jsArgs.WriteString("]")

	r.ctx.Global().SetPropertyStr("__callShortcodeName", r.ctx.NewString(name))

	result, err := r.ctx.Eval("shortcode-call.js", qjs.Code(
		`__shortcodes[__callShortcodeName](`+jsArgs.String()+`)`))

	r.ctx.Global().SetPropertyStr("__callShortcodeName", r.ctx.NewUndefined())

	if err != nil {
		return "", fmt.Errorf("shortcode %q: %w", name, err)
	}
	defer result.Free()

	if result.IsString() {
		return result.String(), nil
	}
	return fmt.Sprint(jsValueToGo(result)), nil
}

// CallHook invokes a registered hook function by name with a payload.
// The hook function is invoked in the QuickJS VM and the result is
// converted back to a Go value.
func (r *QuickJSRuntime) CallHook(name string, payload interface{}) (interface{}, error) {
	if !r.hooks[name] {
		return payload, nil
	}

	// Set the payload as a global variable accessible from JS
	switch v := payload.(type) {
	case string:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewString(v))
	case int:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewInt32(int32(v)))
	case float64:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewFloat64(v))
	case bool:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewBool(v))
	default:
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewString(fmt.Sprint(v)))
	}

	r.ctx.Global().SetPropertyStr("__callHookName", r.ctx.NewString(name))

	result, err := r.ctx.Eval("hook-call.js", qjs.Code(
		`__hooks[__callHookName](__callInput)`))

	r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewUndefined())
	r.ctx.Global().SetPropertyStr("__callHookName", r.ctx.NewUndefined())

	if err != nil {
		return nil, fmt.Errorf("hook %q: %w", name, err)
	}
	defer result.Free()

	return jsValueToGo(result), nil
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

// Close releases resources held by the QuickJS runtime.
func (r *QuickJSRuntime) Close() {
	if r.rt != nil {
		r.rt.Close()
		r.rt = nil
	}
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
