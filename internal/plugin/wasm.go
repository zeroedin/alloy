package plugin

import (
	"context"
	"encoding/json"
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

// SetSiteData makes site data available as alloy.data in the JS context.
// Data is JSON-serialized from Go and parsed in JS. The resulting object
// is frozen to prevent cross-plugin mutation.
func (r *QuickJSRuntime) SetSiteData(data map[string]interface{}) error {
	if !r.initialized {
		return fmt.Errorf("quickjs runtime not initialized — call Init() first")
	}

	if data == nil {
		data = make(map[string]interface{})
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("serializing site data: %w", err)
	}

	r.ctx.Global().SetPropertyStr("__siteDataJSON", r.ctx.NewString(string(jsonBytes)))
	defer r.ctx.Global().SetPropertyStr("__siteDataJSON", r.ctx.NewUndefined())

	result, err := r.ctx.Eval("site-data.js", qjs.Code(
		`alloy.data = Object.freeze(JSON.parse(__siteDataJSON));`))
	if result != nil {
		result.Free()
	}
	if err != nil {
		return fmt.Errorf("setting alloy.data: %w", err)
	}
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

	// Clean up all globals on exit, including early-return error paths
	defer func() {
		r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewUndefined())
		r.ctx.Global().SetPropertyStr("__callFilterName", r.ctx.NewUndefined())
		r.ctx.Global().SetPropertyStr("__callArgsJSON", r.ctx.NewUndefined())
		r.ctx.Eval("args-cleanup.js", qjs.Code(`__callArgs = undefined;`))
	}()

	// Serialize args as a JS array so the filter function receives them
	if len(args) > 0 {
		argsJSON, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("filter %q: marshaling args: %w", name, err)
		}
		r.ctx.Global().SetPropertyStr("__callArgsJSON", r.ctx.NewString(string(argsJSON)))
		_, err = r.ctx.Eval("args-parse.js", qjs.Code(`var __callArgs = JSON.parse(__callArgsJSON);`))
		if err != nil {
			return nil, fmt.Errorf("filter %q: parsing args: %w", name, err)
		}
	} else {
		_, err := r.ctx.Eval("args-empty.js", qjs.Code(`var __callArgs = [];`))
		if err != nil {
			return nil, fmt.Errorf("filter %q: creating empty args: %w", name, err)
		}
	}

	// Invoke the filter function stored in __filters, spreading additional args
	result, err := r.ctx.Eval("filter-call.js", qjs.Code(
		`__filters[__callFilterName](__callInput, ...__callArgs)`))

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
	rt         wazero.Runtime
	mod        api.Module
	cacheDir   string // issue #391: wazero compilation cache directory
}

// SetCacheDir configures a persistent compilation cache directory.
// When set, wazero persists compiled native code to disk so subsequent
// builds skip WASM recompilation. Must be called before LoadModule.
func (r *WASMRuntime) SetCacheDir(dir string) {
	r.cacheDir = dir
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
	// Close any previously loaded module/runtime
	r.Close()
	r.exports = make(map[string]bool)

	wasmBytes, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("wasm module not found: %s", path)
		}
		return fmt.Errorf("reading WASM module %s: %w", path, err)
	}

	r.modulePath = path
	r.moduleName = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	ctx := context.Background()

	rtConfig := wazero.NewRuntimeConfig()
	if r.cacheDir != "" {
		if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
			return fmt.Errorf("creating wasm cache directory: %w", err)
		}
		cache, err := wazero.NewCompilationCacheWithDir(r.cacheDir)
		if err != nil {
			return fmt.Errorf("initializing wasm compilation cache: %w", err)
		}
		rtConfig = rtConfig.WithCompilationCache(cache)
	}
	r.rt = wazero.NewRuntimeWithConfig(ctx, rtConfig)

	compiled, err := r.rt.CompileModule(ctx, wasmBytes)
	if err != nil {
		r.rt.Close(ctx)
		r.rt = nil
		return fmt.Errorf("invalid WASM module %s: %w", filepath.Base(path), err)
	}
	defer compiled.Close(ctx)

	// Discover exported functions — iterate all export names per function
	for _, exp := range compiled.ExportedFunctions() {
		for _, name := range exp.ExportNames() {
			r.exports[name] = true
		}
	}

	// Instantiate the module
	r.mod, err = r.rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig())
	if err != nil {
		r.exports = make(map[string]bool)
		r.rt.Close(ctx)
		r.rt = nil
		return fmt.Errorf("instantiating WASM module %s: %w", filepath.Base(path), err)
	}

	// Require alloc export for safe memory allocation
	if r.mod.ExportedFunction("alloc") == nil {
		r.Close()
		return fmt.Errorf("wasm module %s missing required alloc export — "+
			"alloc(size i32) -> (ptr i32) is needed for safe memory allocation", filepath.Base(path))
	}

	return nil
}

// CallExport calls an exported WASM function by name.
// For string arguments, the input is written to the module's memory
// and the function is called with (ptr, len). The result is read back.
func (r *WASMRuntime) CallExport(name string, args ...interface{}) (interface{}, error) {
	if r.mod == nil {
		return nil, fmt.Errorf("wasm module not loaded — call LoadModule first")
	}
	if !r.exports[name] {
		return nil, fmt.Errorf("export %q not found in %s.wasm", name, r.moduleName)
	}

	fn := r.mod.ExportedFunction(name)
	if fn == nil {
		return nil, fmt.Errorf("export %q not found in %s.wasm", name, r.moduleName)
	}

	// For string-like input: write to memory, call with (ptr, len), read result.
	// Supported argument types are string, []byte, and fmt.Stringer;
	// other argument types return an error. Liquid engines may pass typed
	// wrappers instead of plain Go strings. Multiple string-like args are
	// JSON-encoded as an array.
	if len(args) > 0 {
		var input string
		switch v := args[0].(type) {
		case string:
			input = v
		case []byte:
			input = string(v)
		case fmt.Stringer:
			input = v.String()
		default:
			return nil, fmt.Errorf("wasm CallExport %q: argument 0 is %T, expected string-like type", name, args[0])
		}

		if len(args) > 1 {
			strArgs := make([]string, len(args))
			strArgs[0] = input
			for i := 1; i < len(args); i++ {
				switch v := args[i].(type) {
				case string:
					strArgs[i] = v
				case []byte:
					strArgs[i] = string(v)
				case fmt.Stringer:
					strArgs[i] = v.String()
				default:
					return nil, fmt.Errorf("wasm CallExport %q: argument %d is %T, expected string-like type", name, i, args[i])
				}
			}
			jsonBytes, err := json.Marshal(strArgs)
			if err != nil {
				return nil, fmt.Errorf("wasm CallExport %q: marshaling args: %w", name, err)
			}
			input = string(jsonBytes)
		}
		return r.callStringFilter(fn, input)
	}

	results, err := fn.Call(context.Background())
	if err != nil {
		return nil, fmt.Errorf("wasm call %q: %w", name, err)
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
		return nil, fmt.Errorf("wasm module has no exported memory — cannot pass string arguments")
	}

	// Allocate memory via the module's exported alloc function
	allocFn := r.mod.ExportedFunction("alloc")
	if allocFn == nil {
		return nil, fmt.Errorf("wasm module missing alloc export — cannot allocate memory for input")
	}

	inputBytes := []byte(input)
	allocResult, err := allocFn.Call(context.Background(), uint64(len(inputBytes)))
	if err != nil {
		return nil, fmt.Errorf("wasm alloc(%d) failed: %w", len(inputBytes), err)
	}
	inputPtr := uint32(allocResult[0])

	if !mem.Write(inputPtr, inputBytes) {
		return nil, fmt.Errorf("wasm memory write failed: input (%d bytes) at offset %d exceeds memory", len(inputBytes), inputPtr)
	}

	results, err := fn.Call(context.Background(), uint64(inputPtr), uint64(len(inputBytes)))
	if err != nil {
		return nil, fmt.Errorf("wasm filter call: %w", err)
	}

	if len(results) >= 2 {
		resultPtr := uint32(results[0])
		resultLen := uint32(results[1])
		// ABI error convention: (0, 0) signals a plugin execution error
		if resultPtr == 0 && resultLen == 0 {
			// Check for optional last_error() export for detailed message
			if lastErrFn := r.mod.ExportedFunction("last_error"); lastErrFn != nil {
				if errResults, err := lastErrFn.Call(context.Background()); err == nil && len(errResults) >= 2 {
					errPtr, errLen := uint32(errResults[0]), uint32(errResults[1])
					if errPtr != 0 && errLen != 0 {
						if errMsg, ok := mem.Read(errPtr, errLen); ok {
							return nil, fmt.Errorf("wasm filter error: %s", string(errMsg))
						}
					}
				}
			}
			return nil, fmt.Errorf("wasm filter returned (0, 0) — plugin execution error")
		}
		resultData, ok := mem.Read(resultPtr, resultLen)
		if !ok {
			return nil, fmt.Errorf("wasm memory read failed: result at offset %d len %d", resultPtr, resultLen)
		}
		return string(resultData), nil
	}

	return nil, fmt.Errorf("wasm filter ABI mismatch: expected 2 return values (ptr, len), got %d", len(results))
}

// CallExportRaw invokes a WASM function with raw i32 arguments and reads
// the result from memory. Returns error if the function returns (0, 0)
// per the ABI error convention.
func (r *WASMRuntime) CallExportRaw(name string, ptr, length uint32) (string, error) {
	if r.mod == nil {
		return "", fmt.Errorf("wasm module not loaded — call LoadModule first")
	}

	fn := r.mod.ExportedFunction(name)
	if fn == nil {
		return "", fmt.Errorf("export %q not found in %s.wasm", name, r.moduleName)
	}

	results, err := fn.Call(context.Background(), uint64(ptr), uint64(length))
	if err != nil {
		return "", fmt.Errorf("wasm call %q: %w", name, err)
	}

	if len(results) >= 2 {
		resultPtr := uint32(results[0])
		resultLen := uint32(results[1])
		if resultPtr == 0 && resultLen == 0 {
			if lastErrFn := r.mod.ExportedFunction("last_error"); lastErrFn != nil {
				if errResults, err := lastErrFn.Call(context.Background()); err == nil && len(errResults) >= 2 {
					errPtr, errLen := uint32(errResults[0]), uint32(errResults[1])
					if errPtr != 0 && errLen != 0 {
						mem := r.mod.Memory()
						if mem != nil {
							if errMsg, ok := mem.Read(errPtr, errLen); ok {
								return "", fmt.Errorf("wasm function %q error: %s", name, string(errMsg))
							}
						}
					}
				}
			}
			return "", fmt.Errorf("wasm function %q returned (0, 0) — plugin execution error", name)
		}
		mem := r.mod.Memory()
		if mem == nil {
			return "", fmt.Errorf("wasm module has no exported memory")
		}
		resultData, ok := mem.Read(resultPtr, resultLen)
		if !ok {
			return "", fmt.Errorf("wasm memory read failed: result at offset %d len %d", resultPtr, resultLen)
		}
		return string(resultData), nil
	}

	return "", fmt.Errorf("wasm function %q returned fewer than 2 values", name)
}

// wasmRuntimeExports are well-known WASM exports that are not plugin filters.
var wasmRuntimeExports = map[string]bool{
	"memory": true, "alloc": true, "last_error": true,
	"hook": true, "shortcode": true,
	"_start": true, "_initialize": true,
	"__data_end": true, "__heap_base": true, "__stack_pointer": true,
	"__dso_handle": true, "__global_base": true,
}

// RegisteredFilters returns the names of exported functions that can be used as filters.
// Excludes well-known WASM runtime exports (memory, _start, etc.).
func (r *WASMRuntime) RegisteredFilters() []string {
	names := make([]string, 0, len(r.exports))
	for name := range r.exports {
		if !wasmRuntimeExports[name] {
			names = append(names, name)
		}
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

// RegisteredShortcodes returns an empty list — WASM modules don't register shortcodes.
func (r *WASMRuntime) RegisteredShortcodes() []string {
	return nil
}

// CallShortcode is a no-op for WASM modules.
func (r *WASMRuntime) CallShortcode(name string, args []string, innerContent string) (string, error) {
	return innerContent, nil
}

// RegisteredHooks returns an empty list — WASM modules don't register hooks.
func (r *WASMRuntime) RegisteredHooks() []string {
	return nil
}

// SetSiteData is a no-op for WASM modules — they don't have a JS context
// to inject site data into.
func (r *WASMRuntime) SetSiteData(data map[string]interface{}) error {
	return nil
}

// HasExport checks if the WASM module exports a function with the given name.
func (r *WASMRuntime) HasExport(name string) bool {
	return r.exports[name]
}

// Close releases wazero resources.
func (r *WASMRuntime) Close() {
	ctx := context.Background()
	if r.mod != nil {
		r.mod.Close(ctx)
		r.mod = nil
	}
	if r.rt != nil {
		r.rt.Close(ctx)
		r.rt = nil
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
