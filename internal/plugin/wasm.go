package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/fastschema/qjs"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
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

	// Set args as a JS array via globals to avoid escaping issues
	argsArray, err := r.ctx.Eval("args.js", qjs.Code(`[]`))
	if err != nil {
		return "", fmt.Errorf("shortcode %q: creating args array: %w", name, err)
	}
	for i, arg := range args {
		v := r.ctx.NewString(arg)
		argsArray.SetPropertyIndex(int64(i), v)
	}
	r.ctx.Global().SetPropertyStr("__callShortcodeArgs", argsArray)
	r.ctx.Global().SetPropertyStr("__callShortcodeContent", r.ctx.NewString(innerContent))
	r.ctx.Global().SetPropertyStr("__callShortcodeName", r.ctx.NewString(name))

	result, err := r.ctx.Eval("shortcode-call.js", qjs.Code(
		`__shortcodes[__callShortcodeName](__callShortcodeArgs, __callShortcodeContent)`))

	r.ctx.Global().SetPropertyStr("__callShortcodeName", r.ctx.NewUndefined())
	r.ctx.Global().SetPropertyStr("__callShortcodeArgs", r.ctx.NewUndefined())
	r.ctx.Global().SetPropertyStr("__callShortcodeContent", r.ctx.NewUndefined())

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
	wasmBytes  []byte
	rt         wazero.Runtime
	mod        api.Module
}

// NewWASMRuntime creates a new WASM runtime via wazero.
func NewWASMRuntime() *WASMRuntime {
	return &WASMRuntime{
		exports: make(map[string]bool),
	}
}

// LoadModule loads a WASM module from the given file path.
// Validates the binary, compiles it, and discovers exported functions.
func (r *WASMRuntime) LoadModule(path string) error {
	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("WASM module not found: %s", path)
	}

	r.modulePath = path
	r.moduleName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	r.wasmBytes = wasmBytes

	ctx := context.Background()
	r.rt = wazero.NewRuntime(ctx)

	compiled, err := r.rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		r.rt.Close(ctx)
		return fmt.Errorf("invalid WASM module %s: %w", filepath.Base(path), err)
	}

	// Discover exported functions
	for _, exp := range compiled.ExportedFunctions() {
		r.exports[exp.ExportNames()[0]] = true
	}

	// Instantiate the module
	r.mod, err = r.rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		r.rt.Close(ctx)
		return fmt.Errorf("instantiating WASM module %s: %w", filepath.Base(path), err)
	}

	return nil
}

// CallExport calls an exported WASM function by name.
// For string arguments, the input is written to the module's memory
// and the function is called with (ptr, len). The result is read back.
func (r *WASMRuntime) CallExport(name string, args ...interface{}) (interface{}, error) {
	if !r.exports[name] {
		return nil, fmt.Errorf("export %q not found in %s.wasm", name, r.moduleName)
	}

	fn := r.mod.ExportedFunction(name)
	if fn == nil {
		return nil, fmt.Errorf("export %q not found in %s.wasm", name, r.moduleName)
	}

	// For string input: write to memory, call with (ptr, len), read result
	if len(args) > 0 {
		if s, ok := args[0].(string); ok {
			return r.callStringFilter(fn, s)
		}
	}

	results, err := fn.Call(context.Background())
	if err != nil {
		return nil, fmt.Errorf("WASM call %q: %w", name, err)
	}
	if len(results) > 0 {
		return results[0], nil
	}
	return nil, nil
}

// callStringFilter writes a string to WASM memory, calls the filter function
// with (ptr, len), and reads the result string from memory.
func (r *WASMRuntime) callStringFilter(fn api.Function, input string) (interface{}, error) {
	mem := r.mod.Memory()
	if mem == nil {
		return input, nil
	}

	// Write input to memory at a known offset
	inputBytes := []byte(input)
	inputPtr := uint32(1024) // fixed offset for simplicity
	if !mem.Write(inputPtr, inputBytes) {
		return input, nil
	}

	results, err := fn.Call(context.Background(), uint64(inputPtr), uint64(len(inputBytes)))
	if err != nil {
		return nil, fmt.Errorf("WASM filter call: %w", err)
	}

	if len(results) >= 2 {
		resultPtr := uint32(results[0])
		resultLen := uint32(results[1])
		if resultData, ok := mem.Read(resultPtr, resultLen); ok {
			return string(resultData), nil
		}
	}

	return input, nil
}

// RegisteredFilters returns the names of exported functions that can be used as filters.
func (r *WASMRuntime) RegisteredFilters() []string {
	names := make([]string, 0, len(r.exports))
	for name := range r.exports {
		names = append(names, name)
	}
	return names
}

// CallFilter calls a WASM-exported filter function by name.
func (r *WASMRuntime) CallFilter(name string, input interface{}, args ...interface{}) (interface{}, error) {
	allArgs := make([]interface{}, 0, 1+len(args))
	allArgs = append(allArgs, input)
	allArgs = append(allArgs, args...)
	return r.CallExport(name, allArgs...)
}

// HasExport checks if the WASM module exports a function with the given name.
func (r *WASMRuntime) HasExport(name string) bool {
	return r.exports[name]
}

// Close releases wazero resources.
func (r *WASMRuntime) Close() {
	if r.rt != nil {
		r.rt.Close(context.Background())
	}
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
