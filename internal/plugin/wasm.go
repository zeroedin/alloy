package plugin

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/fastschema/qjs"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ordered"
)

// payloadType identifies the hook payload type for targeted outbound extraction.
type payloadType int

const (
	payloadRendered       payloadType = iota // onPageRendered: extract html + addDependencies
	payloadFormatRendered                    // onFormatRendered: extract content + addDependencies
	payloadTransform                         // onContentTransformed: extract html + toc + addDependencies
)

// QuickJSRuntime wraps a QuickJS instance for Tier 2 in-process JS plugins.
// JavaScript is executed via QuickJS compiled to WASM, running on wazero
// (pure Go, zero CGo). See PLAN.md §5.
type QuickJSRuntime struct {
	mu           sync.Mutex
	initialized  bool
	rt           *qjs.Runtime
	ctx          *qjs.Context
	filters      map[string]bool
	shortcodes   map[string]bool
	hooks        map[string]int        // hook name → priority
	hookScopes   map[string]*HookScope // hook name → scope
	evalWarnings []string              // warnings from plugin eval (e.g., duplicate hooks)
	pendingFM    map[string]interface{} // lazy frontMatter: set before hook call, read by __resolveFM callback
	pendingTOC   []content.TOCEntry     // lazy TOC: set before hook call, read by __resolveTOC callback
}

// NewQuickJSRuntime creates a new QuickJS runtime instance.
// Startup cost is ~10-50ms (one-time).
func NewQuickJSRuntime() *QuickJSRuntime {
	return &QuickJSRuntime{
		filters:    make(map[string]bool),
		shortcodes: make(map[string]bool),
		hooks:      make(map[string]int),
		hookScopes: make(map[string]*HookScope),
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
		if len(args) < 1 {
			return this.Context().NewUndefined(), nil
		}
		name := args[0].String()
		if _, exists := r.hooks[name]; exists {
			r.evalWarnings = append(r.evalWarnings,
				fmt.Sprintf("duplicate hook registration: %q registered multiple times, last registration wins", name))
		}
		if len(args) >= 2 {
			r.hooks[name] = int(args[1].Int32())
			if len(args) >= 3 {
				scopeJSON := args[2].String()
				if scopeJSON != "" {
					scope, err := parseScopeJSON(scopeJSON)
					if err != nil {
						log.Printf("warning: plugin hook %s: malformed scope JSON, using default scope: %v", name, err)
						r.hookScopes[name] = &HookScope{Pages: PagesScope{Mode: PagesScopeAll}}
					} else if scope != nil {
						r.hookScopes[name] = scope
					}
				}
			}
		} else {
			r.hooks[name] = 50
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
			hook: function(name, options, fn) {
				if (typeof options === 'function') {
					throw new Error('alloy.hook() requires options object as second argument: alloy.hook(name, { pages: true }, fn)');
				}
				if (typeof fn !== 'function') {
					throw new Error('alloy.hook() requires a function as third argument: alloy.hook(name, options, fn)');
				}
				if (!options || typeof options !== 'object') { options = {}; }
				__hooks[name] = fn;
				var p = (typeof options.priority === 'number') ? Math.floor(options.priority) : 50;
				__registerHook(name, p, JSON.stringify({
					data: options.data !== undefined ? options.data : null,
					pages: options.pages !== undefined ? options.pages : null,
					pageFields: options.pageFields !== undefined ? options.pageFields : null
				}));
			},
			on: function(name, options, fn) { alloy.hook(name, options, fn); }
		};
	`))
	if err != nil {
		r.rt.Close()
		return fmt.Errorf("setting up alloy global: %w", err)
	}

	// Register Go callbacks for lazy frontMatter/TOC resolution (issue #1187).
	// These are called from JS lazy getters only when the plugin reads the property.
	r.ctx.SetFunc("__resolveFM", func(this *qjs.This) (*qjs.Value, error) {
		fm := r.pendingFM
		if fm == nil {
			return this.Context().NewString("{}"), nil
		}
		b, err := jsonCodec.Marshal(fm)
		if err != nil {
			log.Printf("warning: lazy frontMatter marshal failed: %v", err)
			return this.Context().NewString("{}"), nil
		}
		return this.Context().NewString(string(b)), nil
	})

	r.ctx.SetFunc("__resolveTOC", func(this *qjs.This) (*qjs.Value, error) {
		toc := r.pendingTOC
		if toc == nil {
			return this.Context().NewString("[]"), nil
		}
		b, err := jsonCodec.Marshal(toc)
		if err != nil {
			log.Printf("warning: lazy TOC marshal failed: %v", err)
			return this.Context().NewString("[]"), nil
		}
		return this.Context().NewString(string(b)), nil
	})

	// Pre-compile JS functions (issues #1185, #1187).
	// __callHookByName replaces per-call Eval("__hooks[name](input)").
	// Lazy getter installers called via ctx.Invoke, not Eval.
	_, err = r.ctx.Eval("precompile.js", qjs.Code(`
		function __callHookByName(name, input) { return __hooks[name](input); }
		function __installLazyFM(target) {
			Object.defineProperty(target, 'frontMatter', {
				get: function() {
					var v = JSON.parse(__resolveFM());
					Object.defineProperty(this, 'frontMatter', {value: v, writable: true, enumerable: true, configurable: true});
					return v;
				},
				set: function(v) {
					Object.defineProperty(this, 'frontMatter', {value: v, writable: true, enumerable: true, configurable: true});
				},
				enumerable: true,
				configurable: true
			});
		}
		function __installLazyTOC(target) {
			Object.defineProperty(target, 'toc', {
				get: function() {
					var v = JSON.parse(__resolveTOC());
					Object.defineProperty(this, 'toc', {value: v, writable: true, enumerable: true, configurable: true});
					return v;
				},
				set: function(v) {
					Object.defineProperty(this, 'toc', {value: v, writable: true, enumerable: true, configurable: true});
				},
				enumerable: true,
				configurable: true
			});
		}
	`))
	if err != nil {
		r.rt.Close()
		return fmt.Errorf("setting up pre-compiled helpers: %w", err)
	}

	r.initialized = true
	return nil
}

// SetSiteData makes site data available as alloy.data in the JS context.
// Data is JSON-serialized from Go and parsed in JS. The resulting object
// is frozen to prevent cross-plugin mutation.
func (r *QuickJSRuntime) SetSiteData(data map[string]interface{}) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.initialized {
		return fmt.Errorf("quickjs runtime not initialized — call Init() first")
	}

	if data == nil {
		data = make(map[string]interface{})
	}

	jsonBytes, err := jsonCodec.Marshal(data)
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rt == nil {
		return fmt.Errorf("%s: runtime is closed", filepath.Base(path))
	}
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rt == nil {
		return input, nil
	}
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
		argsJSON, err := jsonCodec.Marshal(args)
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
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rt == nil {
		return innerContent, nil
	}
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
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.callHookLocked(name, payload)
}

// BatchCallHook loops synchronously through payloads calling callHookLocked
// for each one (issue #1185). No per-item goroutine/channel/context overhead.
// onProgress is called with 1-based indices after each item completes.
func (r *QuickJSRuntime) BatchCallHook(name string, payloads []interface{}, onProgress func(int)) ([]interface{}, error) {
	if len(payloads) == 0 {
		return []interface{}{}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	results := make([]interface{}, len(payloads))
	for i, p := range payloads {
		result, err := r.callHookLocked(name, p)
		if err != nil {
			return nil, err
		}
		results[i] = result
		if onProgress != nil {
			onProgress(i + 1)
		}
	}
	return results, nil
}

// callHookLocked implements CallHook without acquiring the mutex.
// Used by both CallHook (single) and BatchCallHook (batch).
func (r *QuickJSRuntime) callHookLocked(name string, payload interface{}) (interface{}, error) {
	if r.rt == nil {
		return payload, nil
	}
	if _, ok := r.hooks[name]; !ok {
		return payload, nil
	}

	// Fast path for payload structs (issue #1180): build JS objects directly
	// via the QuickJS API instead of JSON-serializing the entire struct.
	// Returns map[string]interface{} (the slow path below returns *ordered.Map);
	// both are handled by pipeline's toGoMap().
	switch v := payload.(type) {
	case HookRenderedPayload:
		return r.callHookRenderedPayload(name, v)
	case HookFormatRenderedPayload:
		return r.callHookFormatRenderedPayload(name, v)
	case HookTransformPayload:
		return r.callHookTransformPayload(name, v)
	case map[string]interface{}:
		// Hook chain fast path (issue #1185): when hooks chain, the second
		// hook receives a map[string]interface{} from the first hook's result.
		// Page-like maps (with html or content key) build JS objects directly.
		// Non-page maps fall through to the JSON path.
		if _, hasHTML := v["html"]; hasHTML {
			// Disambiguate transform vs rendered: onContentTransformed returns
			// html + toc, while onPageRendered returns only html.
			if _, hasTOC := v["toc"]; hasTOC {
				return r.callHookMapPayload(name, v, payloadTransform)
			}
			return r.callHookMapPayload(name, v, payloadRendered)
		}
		if _, hasContent := v["content"]; hasContent {
			return r.callHookMapPayload(name, v, payloadFormatRendered)
		}
		// Fall through to JSON path for non-page maps
	}

	// Slow path: JSON-serialize the payload and parse in the VM.
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
		jsonBytes, err := jsonCodec.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("hook %q: marshaling payload: %w", name, err)
		}
		r.ctx.Global().SetPropertyStr("__callInputJSON", r.ctx.NewString(string(jsonBytes)))
		parsed, err := r.ctx.Eval("hook-input.js", qjs.Code(`JSON.parse(__callInputJSON)`))
		r.ctx.Global().SetPropertyStr("__callInputJSON", r.ctx.NewUndefined())
		if err != nil {
			return nil, fmt.Errorf("hook %q: parsing payload: %w", name, err)
		}
		r.ctx.Global().SetPropertyStr("__callInput", parsed)
	}

	r.ctx.Global().SetPropertyStr("__callHookName", r.ctx.NewString(name))

	result, err := r.ctx.Eval("hook-call.js", qjs.Code(
		`(function() { var __r = __hooks[__callHookName](__callInput); `+
			`return typeof __r === 'object' && __r !== null ? JSON.stringify(__r) : __r; })()`))

	r.ctx.Global().SetPropertyStr("__callInput", r.ctx.NewUndefined())
	r.ctx.Global().SetPropertyStr("__callHookName", r.ctx.NewUndefined())

	if err != nil {
		return nil, fmt.Errorf("hook %q: %w", name, err)
	}
	defer result.Free()

	if result.IsString() {
		s := result.String()
		if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
			if parsed, err := ordered.UnmarshalJSONValue([]byte(s)); err == nil {
				return parsed, nil
			}
		}
		return s, nil
	}

	return jsValueToGo(result), nil
}

// callHookRenderedPayload is the fast path for HookRenderedPayload (issue #1180).
// Builds a JS object directly instead of JSON-serializing the ~800KB HTML field.
func (r *QuickJSRuntime) callHookRenderedPayload(name string, v HookRenderedPayload) (interface{}, error) {
	obj := r.ctx.NewObject()
	obj.SetPropertyStr("html", r.ctx.NewString(v.HTML))
	obj.SetPropertyStr("url", r.ctx.NewString(v.URL))
	obj.SetPropertyStr("path", r.ctx.NewString(v.Path))
	if err := r.setPayloadFrontMatter(obj, v.FrontMatter); err != nil {
		return nil, fmt.Errorf("hook %q: %w", name, err)
	}
	return r.invokeHookFastPath(name, obj, payloadRendered)
}

// callHookFormatRenderedPayload is the fast path for HookFormatRenderedPayload (issue #1180).
// Builds a JS object directly instead of JSON-serializing the ~800KB content field.
func (r *QuickJSRuntime) callHookFormatRenderedPayload(name string, v HookFormatRenderedPayload) (interface{}, error) {
	obj := r.ctx.NewObject()
	obj.SetPropertyStr("format", r.ctx.NewString(v.Format))
	obj.SetPropertyStr("content", r.ctx.NewString(v.Content))
	obj.SetPropertyStr("url", r.ctx.NewString(v.URL))
	obj.SetPropertyStr("path", r.ctx.NewString(v.Path))
	if err := r.setPayloadFrontMatter(obj, v.FrontMatter); err != nil {
		return nil, fmt.Errorf("hook %q: %w", name, err)
	}
	return r.invokeHookFastPath(name, obj, payloadFormatRendered)
}

// callHookTransformPayload is the fast path for HookTransformPayload (issue #1180).
// Builds a JS object directly instead of JSON-serializing the ~800KB HTML field.
// TOC uses a lazy getter (issue #1187); nil/empty TOC is omitted (matching omitempty).
func (r *QuickJSRuntime) callHookTransformPayload(name string, v HookTransformPayload) (interface{}, error) {
	obj := r.ctx.NewObject()
	obj.SetPropertyStr("html", r.ctx.NewString(v.HTML))
	obj.SetPropertyStr("url", r.ctx.NewString(v.URL))
	obj.SetPropertyStr("path", r.ctx.NewString(v.Path))
	if err := r.setPayloadFrontMatter(obj, v.FrontMatter); err != nil {
		return nil, fmt.Errorf("hook %q: %w", name, err)
	}
	if len(v.TOC) > 0 {
		r.pendingTOC = v.TOC
		installFn := r.ctx.Global().GetPropertyStr("__installLazyTOC")
		defer installFn.Free()
		if _, err := r.ctx.Invoke(installFn, r.ctx.Global(), obj); err != nil {
			return nil, fmt.Errorf("hook %q: installing lazy toc getter: %w", name, err)
		}
	}
	return r.invokeHookFastPath(name, obj, payloadTransform)
}

// callHookMapPayload is the fast path for map[string]interface{} payloads
// from hook chaining (issue #1185). Builds a JS object directly, using
// native strings for the large html/content field.
func (r *QuickJSRuntime) callHookMapPayload(name string, m map[string]interface{}, ptype payloadType) (interface{}, error) {
	obj := r.ctx.NewObject()
	for k, v := range m {
		switch val := v.(type) {
		case string:
			obj.SetPropertyStr(k, r.ctx.NewString(val))
		case int:
			obj.SetPropertyStr(k, r.ctx.NewFloat64(float64(val)))
		case float64:
			obj.SetPropertyStr(k, r.ctx.NewFloat64(val))
		case bool:
			obj.SetPropertyStr(k, r.ctx.NewBool(val))
		default:
			jsonBytes, err := jsonCodec.Marshal(val)
			if err != nil {
				log.Printf("warning: hook chain map key %q: marshal failed: %v", k, err)
				continue
			}
			obj.SetPropertyStr(k, r.ctx.ParseJSON(string(jsonBytes)))
		}
	}
	return r.invokeHookFastPath(name, obj, ptype)
}

// setPayloadFrontMatter installs a lazy frontMatter accessor on a JS payload
// object (issue #1187). The Go map is stored in r.pendingFM and only
// JSON-serialized when the plugin actually reads page.frontMatter.
func (r *QuickJSRuntime) setPayloadFrontMatter(obj *qjs.Value, fm map[string]interface{}) error {
	r.pendingFM = fm
	installFn := r.ctx.Global().GetPropertyStr("__installLazyFM")
	defer installFn.Free()
	_, err := r.ctx.Invoke(installFn, r.ctx.Global(), obj)
	if err != nil {
		return fmt.Errorf("installing lazy frontMatter getter: %w", err)
	}
	return nil
}

// invokeHookFastPath calls a hook function with a pre-built JS object using
// the pre-compiled __callHookByName function (issue #1185, #1186) and extracts
// the result using a type-specific extractor for targeted outbound extraction.
// The input is passed directly as a function argument — no globals are set
// (issue #1186: eliminates __callInput/__callHookName global setup).
func (r *QuickJSRuntime) invokeHookFastPath(name string, input *qjs.Value, ptype payloadType) (interface{}, error) {
	defer input.Free()
	defer func() {
		r.pendingFM = nil
		r.pendingTOC = nil
	}()

	nameVal := r.ctx.NewString(name)
	result, err := r.ctx.Global().InvokeJS("__callHookByName", nameVal, input)
	nameVal.Free()
	if err != nil {
		return nil, fmt.Errorf("hook %q: %w", name, err)
	}
	defer result.Free()

	switch ptype {
	case payloadRendered:
		return r.extractRenderedResult(result)
	case payloadFormatRendered:
		return r.extractFormatRenderedResult(result)
	case payloadTransform:
		return r.extractTransformResult(result)
	default:
		return r.extractHookResult(result)
	}
}

// extractRenderedResult extracts pipeline-consumed fields from an
// onPageRendered return: html + addDependencies (issue #1185).
func (r *QuickJSRuntime) extractRenderedResult(result *qjs.Value) (interface{}, error) {
	if result.IsString() {
		return result.String(), nil
	}
	if result.IsNull() || result.IsUndefined() {
		return nil, nil
	}
	if !result.IsObject() {
		return jsValueToGo(result), nil
	}
	m := make(map[string]interface{}, 2)
	htmlProp := result.GetPropertyStr("html")
	if htmlProp.IsString() {
		m["html"] = htmlProp.String()
	}
	htmlProp.Free()
	r.extractStructuredProperty(result, "addDependencies", m)
	return m, nil
}

// extractFormatRenderedResult extracts pipeline-consumed fields from an
// onFormatRendered return: content + addDependencies (issue #1185).
func (r *QuickJSRuntime) extractFormatRenderedResult(result *qjs.Value) (interface{}, error) {
	if result.IsString() {
		return result.String(), nil
	}
	if result.IsNull() || result.IsUndefined() {
		return nil, nil
	}
	if !result.IsObject() {
		return jsValueToGo(result), nil
	}
	m := make(map[string]interface{}, 2)
	contentProp := result.GetPropertyStr("content")
	if contentProp.IsString() {
		m["content"] = contentProp.String()
	}
	contentProp.Free()
	r.extractStructuredProperty(result, "addDependencies", m)
	return m, nil
}

// extractTransformResult extracts pipeline-consumed fields from an
// onContentTransformed return: html + toc + addDependencies (issue #1185).
func (r *QuickJSRuntime) extractTransformResult(result *qjs.Value) (interface{}, error) {
	if result.IsString() {
		return result.String(), nil
	}
	if result.IsNull() || result.IsUndefined() {
		return nil, nil
	}
	if !result.IsObject() {
		return jsValueToGo(result), nil
	}
	m := make(map[string]interface{}, 3)
	htmlProp := result.GetPropertyStr("html")
	if htmlProp.IsString() {
		m["html"] = htmlProp.String()
	}
	htmlProp.Free()
	r.extractStructuredProperty(result, "toc", m)
	r.extractStructuredProperty(result, "addDependencies", m)
	return m, nil
}

// extractStructuredProperty reads a non-primitive property from a JS result
// object. If defined, JSONStringify it and unmarshal into the Go map.
// Used for small structured fields (addDependencies, toc) on the fast path.
func (r *QuickJSRuntime) extractStructuredProperty(result *qjs.Value, key string, m map[string]interface{}) {
	prop := result.GetPropertyStr(key)
	defer prop.Free()
	if prop.IsUndefined() || prop.IsNull() {
		return
	}
	if prop.IsString() || prop.IsNumber() || prop.IsBool() {
		val := jsValueToGo(prop)
		if val != nil {
			m[key] = val
		}
		return
	}
	s, err := prop.JSONStringify()
	if err != nil {
		log.Printf("warning: hook result property %q: JSONStringify failed: %v", key, err)
		return
	}
	var parsed interface{}
	if err := jsonCodec.Unmarshal([]byte(s), &parsed); err != nil {
		log.Printf("warning: hook result property %q: unmarshal failed: %v", key, err)
		return
	}
	m[key] = parsed
}

// extractHookResult converts a JS hook return value to a Go value without
// JSON-serializing the entire result. Used as fallback for non-fast-path hooks.
func (r *QuickJSRuntime) extractHookResult(result *qjs.Value) (interface{}, error) {
	if result.IsString() {
		return result.String(), nil
	}
	if result.IsNull() || result.IsUndefined() {
		return nil, nil
	}
	if !result.IsObject() {
		return jsValueToGo(result), nil
	}
	if result.IsArray() {
		s, err := result.JSONStringify()
		if err != nil {
			return nil, fmt.Errorf("extracting array result: %w", err)
		}
		var parsed interface{}
		if err := jsonCodec.Unmarshal([]byte(s), &parsed); err != nil {
			return nil, fmt.Errorf("parsing array result: %w", err)
		}
		return parsed, nil
	}

	propNames, err := result.GetOwnPropertyNames()
	if err != nil {
		s, serr := result.JSONStringify()
		if serr != nil {
			return nil, fmt.Errorf("extracting result: property names: %w", err)
		}
		var parsed interface{}
		if err := jsonCodec.Unmarshal([]byte(s), &parsed); err != nil {
			return nil, fmt.Errorf("parsing result fallback: %w", err)
		}
		return parsed, nil
	}

	m := make(map[string]interface{}, len(propNames))
	for _, pname := range propNames {
		prop := result.GetPropertyStr(pname)
		if prop.IsString() || prop.IsNumber() || prop.IsBool() || prop.IsNull() || prop.IsUndefined() {
			val := jsValueToGo(prop)
			prop.Free()
			if val != nil {
				m[pname] = val
			}
			continue
		}
		s, serr := prop.JSONStringify()
		prop.Free()
		if serr != nil {
			log.Printf("warning: hook result property %q: JSONStringify failed: %v", pname, serr)
			continue
		}
		var parsed interface{}
		if err := jsonCodec.Unmarshal([]byte(s), &parsed); err != nil {
			m[pname] = s
			continue
		}
		m[pname] = parsed
	}
	return m, nil
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

// RegisteredHookDetails returns hook registrations with priority and scope info.
func (r *QuickJSRuntime) RegisteredHookDetails() []HookRegistration {
	regs := make([]HookRegistration, 0, len(r.hooks))
	for name, priority := range r.hooks {
		regs = append(regs, HookRegistration{Name: name, Priority: priority, Scope: r.hookScopes[name]})
	}
	return regs
}

// EvalWarnings returns warnings collected during plugin evaluation.
func (r *QuickJSRuntime) EvalWarnings() []string {
	return r.evalWarnings
}

// Close releases resources held by the QuickJS runtime.
// Acquires the mutex to wait for any in-flight WASM operations
// (e.g., from timed-out RunWithTimeout goroutines) to finish
// before freeing the runtime.
func (r *QuickJSRuntime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.rt != nil {
		r.rt.Close()
		r.rt = nil
		r.ctx = nil
		r.initialized = false
	}
}

// WASMRuntime wraps a wazero WASM module for Tier 2 compiled plugins.
type WASMRuntime struct {
	modulePath        string
	moduleName        string
	exports           map[string]bool
	hookNames         []string
	hookRegistrations []HookRegistration
	rt                wazero.Runtime
	mod               api.Module
	cacheDir          string                  // issue #391: wazero compilation cache directory
	cache             wazero.CompilationCache // owned by this runtime (closed in Close)
	sharedCache       wazero.CompilationCache // owned by Registry (not closed here)
}

// SetCacheDir configures a persistent compilation cache directory.
// When set, wazero persists compiled native code to disk so subsequent
// builds skip WASM recompilation. Must be called before LoadModule.
func (r *WASMRuntime) SetCacheDir(dir string) {
	r.cacheDir = dir
}

// SetCompilationCache sets a shared compilation cache owned by the caller.
// The cache is NOT closed by WASMRuntime — the caller manages its lifecycle.
func (r *WASMRuntime) SetCompilationCache(cache wazero.CompilationCache) {
	r.sharedCache = cache
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
	r.hookNames = nil
	r.hookRegistrations = nil

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
	if r.sharedCache != nil {
		rtConfig = rtConfig.WithCompilationCache(r.sharedCache)
	} else if r.cacheDir != "" {
		if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
			return fmt.Errorf("creating wasm cache directory %q: %w", r.cacheDir, err)
		}
		cache, err := wazero.NewCompilationCacheWithDir(r.cacheDir)
		if err != nil {
			return fmt.Errorf("initializing wasm compilation cache in %q: %w", r.cacheDir, err)
		}
		r.cache = cache
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

	if err := r.discoverHooks(ctx); err != nil {
		r.Close()
		return fmt.Errorf("wasm module %s: %w", filepath.Base(path), err)
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
			jsonBytes, err := jsonCodec.Marshal(strArgs)
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
	"hook": true, "hooks": true, "shortcode": true,
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

// RegisteredHooks returns hook names discovered from the hooks() export.
func (r *WASMRuntime) RegisteredHooks() []string {
	return r.hookNames
}

// RegisteredHookDetails returns hook registrations with per-hook priority and scope.
func (r *WASMRuntime) RegisteredHookDetails() []HookRegistration {
	out := make([]HookRegistration, len(r.hookRegistrations))
	copy(out, r.hookRegistrations)
	return out
}

// CallHook marshals a JSON envelope and dispatches to the hook(ptr, len) export.
func (r *WASMRuntime) CallHook(name string, payload interface{}) (interface{}, error) {
	if r.mod == nil {
		return nil, fmt.Errorf("wasm module not loaded — call LoadModule first")
	}
	hookFn := r.mod.ExportedFunction("hook")
	if hookFn == nil {
		return nil, fmt.Errorf("wasm module %s has no hook export", r.moduleName)
	}

	envelope := map[string]interface{}{
		"event":   name,
		"payload": payload,
	}
	inputJSON, err := jsonCodec.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshaling hook payload: %w", err)
	}

	result, err := r.callStringFilter(hookFn, string(inputJSON))
	if err != nil {
		return nil, fmt.Errorf("wasm hook %q: %w", name, err)
	}

	resultStr, ok := result.(string)
	if !ok {
		return result, nil
	}
	parsed, err := ordered.UnmarshalJSONValue([]byte(resultStr))
	if err != nil {
		return nil, fmt.Errorf("wasm hook %q returned invalid JSON: %w", name, err)
	}
	return parsed, nil
}

// discoverHooks calls the hooks() export to discover registered hook names.
// Validates that hook() exists when hooks are declared, filters empty names,
// and deduplicates.
func (r *WASMRuntime) discoverHooks(ctx context.Context) error {
	r.hookNames = nil
	r.hookRegistrations = nil
	hooksFn := r.mod.ExportedFunction("hooks")
	if hooksFn == nil {
		return nil
	}

	results, err := hooksFn.Call(ctx)
	if err != nil {
		return fmt.Errorf("calling hooks() export: %w", err)
	}
	if len(results) < 2 {
		return fmt.Errorf("hooks() export returned %d values, expected 2 (ptr, len)", len(results))
	}

	ptr := uint32(results[0])
	length := uint32(results[1])
	if ptr == 0 && length == 0 {
		if lastErrFn := r.mod.ExportedFunction("last_error"); lastErrFn != nil {
			if errResults, errErr := lastErrFn.Call(ctx); errErr == nil && len(errResults) >= 2 {
				errPtr, errLen := uint32(errResults[0]), uint32(errResults[1])
				if errPtr != 0 && errLen != 0 {
					if mem := r.mod.Memory(); mem != nil {
						if errMsg, ok := mem.Read(errPtr, errLen); ok {
							return fmt.Errorf("hooks() export failed: %s", string(errMsg))
						}
					}
				}
			}
		}
		return nil
	}

	mem := r.mod.Memory()
	if mem == nil {
		return fmt.Errorf("hooks() returned data but module has no exported memory")
	}
	data, ok := mem.Read(ptr, length)
	if !ok {
		return fmt.Errorf("hooks() memory read failed at offset %d len %d", ptr, length)
	}

	var raw []interface{}
	if err := jsonCodec.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("hooks() export returned invalid JSON (expected array): %w", err)
	}

	seen := make(map[string]bool, len(raw))
	var names []string
	var regs []HookRegistration
	for i, elem := range raw {
		var reg HookRegistration
		switch v := elem.(type) {
		case string:
			reg = HookRegistration{Name: v, Priority: 50}
		case map[string]interface{}:
			nameVal, exists := v["name"]
			if !exists {
				return fmt.Errorf("hooks()[%d]: registration object missing required \"name\" field", i)
			}
			name, ok := nameVal.(string)
			if !ok {
				return fmt.Errorf("hooks()[%d]: \"name\" must be a string, got %T", i, nameVal)
			}
			reg.Name = name
			reg.Priority = 50
			if p, ok := v["priority"]; ok {
				pf, ok := p.(float64)
				if !ok {
					return fmt.Errorf("hooks()[%d]: \"priority\" must be a number, got %T", i, p)
				}
				reg.Priority = int(pf)
			}
			scopeFields := make(map[string]interface{})
			for _, key := range []string{"pages", "data", "pageFields"} {
				if val, ok := v[key]; ok {
					scopeFields[key] = val
				}
			}
			if len(scopeFields) > 0 {
				scope, err := parseScopeMap(scopeFields)
				if err != nil {
					return fmt.Errorf("hooks()[%d] scope: %w", i, err)
				}
				reg.Scope = scope
			}
		default:
			return fmt.Errorf("hooks()[%d]: expected string or object, got %T", i, elem)
		}
		if reg.Name == "" || seen[reg.Name] {
			continue
		}
		seen[reg.Name] = true
		names = append(names, reg.Name)
		regs = append(regs, reg)
	}

	if len(names) > 0 && r.mod.ExportedFunction("hook") == nil {
		return fmt.Errorf("hooks() declares %d hooks but module has no hook() export", len(names))
	}

	r.hookNames = names
	r.hookRegistrations = regs
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
	r.hookNames = nil
	r.hookRegistrations = nil
	if r.mod != nil {
		r.mod.Close(ctx)
		r.mod = nil
	}
	if r.rt != nil {
		r.rt.Close(ctx)
		r.rt = nil
	}
	if r.cache != nil {
		r.cache.Close(ctx)
		r.cache = nil
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
