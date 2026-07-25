package plugin_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/plugin"
)

// extractGoMap extracts a map[string]interface{} from a CallHook result.
// Handles both direct map[string]interface{} returns (from the fast path)
// and *ordered.Map returns (from the JSON parse path).
func extractGoMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	if om, ok := v.(*ordered.Map); ok {
		return om.ToGoMap()
	}
	return nil
}

var _ = Describe("Tier 2 Plugin Runtime (WASM + QuickJS)", func() {

	// setupQuickJSWithHook creates a QuickJS runtime, inlines the given
	// plugin JS (which must register a hook via alloy.hook), and returns
	// the runtime. Caller must DeferCleanup(rt.Close).
	setupQuickJSWithHook := func(hookJS string) *plugin.QuickJSRuntime {
		tmpDir := GinkgoT().TempDir()
		pluginPath := filepath.Join(tmpDir, "test-hook.js")
		Expect(os.WriteFile(pluginPath, []byte(hookJS), 0644)).To(Succeed())

		rt := plugin.NewQuickJSRuntime()
		Expect(rt.Init()).To(Succeed())
		Expect(rt.EvalFile(pluginPath)).To(Succeed())
		DeferCleanup(rt.Close)
		return rt
	}

	// ── QuickJS Runtime ──────────────────────────────────────────────

	Describe("QuickJS Runtime", func() {
		It("initializes the QuickJS instance", func() {
			rt := plugin.NewQuickJSRuntime()
			err := rt.Init()
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.IsInitialized()).To(BeTrue(),
				"QuickJS runtime must report initialized after Init()")
		})

		It("evaluates a JS plugin file in the QuickJS context", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())

			path := filepath.Join(testdataDir(), "single-files", "plain.js")
			err := rt.EvalFile(path)
			Expect(err).NotTo(HaveOccurred(),
				"EvalFile must load and execute the JS plugin without error")
		})

		It("registers a filter from JS plugin via alloy.filter()", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "plain.js"))).To(Succeed())

			filters := rt.RegisteredFilters()
			Expect(filters).NotTo(BeEmpty(),
				"JS plugin must register at least one filter via alloy.filter()")
		})

		It("registers a shortcode from JS plugin via alloy.shortcode()", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "plain.js"))).To(Succeed())

			shortcodes := rt.RegisteredShortcodes()
			Expect(shortcodes).NotTo(BeEmpty(),
				"JS plugin must register at least one shortcode via alloy.shortcode()")
		})

		It("calls a registered filter and returns the transformed value", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "plain.js"))).To(Succeed())

			result, err := rt.CallFilter("wordCount", "hello world foo")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(),
				"filter must return a non-nil result")
		})

		It("CallFilter returns transformed value, not passthrough", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "plain.js"))).To(Succeed())

			result, err := rt.CallFilter("wordCount", "hello world foo")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(Equal("hello world foo"),
				"CallFilter must transform the input, not return it unchanged")
		})

		It("CallFilter executes arbitrary JS, not just recognized patterns", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "reverse.js"))).To(Succeed())

			filters := rt.RegisteredFilters()
			Expect(filters).To(ContainElement("reverse"),
				"guard: reverse filter must be discovered")

			result, err := rt.CallFilter("reverse", "hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("olleh"),
				"CallFilter must execute the actual JS function — "+
					"reverse uses split/reverse/join which simulateJSFilter cannot pattern-match")
		})

		// ── CallFilter with arguments (issue #318) ──────────────────
		// Filter functions must receive additional Liquid arguments,
		// not just the input value.

		It("CallFilter passes additional arguments to JS function", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "lookup.js"))).To(Succeed())

			// {{ "ready" | lookup: {"ready": "Done", "pending": "In Progress"} }}
			hash := map[string]interface{}{
				"ready":   "Done",
				"pending": "In Progress",
			}
			result, err := rt.CallFilter("lookup", "ready", hash)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Done"),
				"CallFilter must pass additional args to the JS function — "+
					"the hash argument must reach the filter as the second parameter")
		})

		It("CallFilter passes multiple arguments to JS function", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "lookup.js"))).To(Succeed())

			// {{ "hello world" | replace_custom: "world", "alloy" }}
			result, err := rt.CallFilter("replace_custom", "hello world", "world", "alloy")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("hello alloy"),
				"CallFilter must pass all arguments — "+
					"replace_custom needs input + two string args")
		})

		It("CallFilter works with zero additional arguments (backward compat)", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "reverse.js"))).To(Succeed())

			// {{ "hello" | reverse }} — no extra args
			result, err := rt.CallFilter("reverse", "hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("olleh"),
				"CallFilter with no additional args must still work")
		})

		// ── Plugin site data access (issue #317) ─────────────────────
		// Plugins can access site.data via alloy.data in the JS context.

		It("SetSiteData makes data available as alloy.data in JS", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())

			siteData := map[string]interface{}{
				"statusLegend": map[string]interface{}{
					"ready":   map[string]interface{}{"pretty": "Done", "color": "green"},
					"pending": map[string]interface{}{"pretty": "In Progress", "color": "yellow"},
				},
			}
			Expect(rt.SetSiteData(siteData)).To(Succeed())

			// Eval a script that reads alloy.data
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "site-data-reader.js"))).To(Succeed())

			result, err := rt.CallFilter("statusPretty", "ready")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Done"),
				"filter must access alloy.data.statusLegend.ready.pretty — "+
					"proves site data is available in the QuickJS context")
		})

		It("SetSiteData handles nested data structures", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())

			siteData := map[string]interface{}{
				"nav": []interface{}{
					map[string]interface{}{"title": "Home", "url": "/"},
					map[string]interface{}{"title": "About", "url": "/about/"},
				},
			}
			Expect(rt.SetSiteData(siteData)).To(Succeed())

			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "site-data-reader.js"))).To(Succeed())

			result, err := rt.CallFilter("navCount", "ignored")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(BeNumerically("==", 2),
				"filter must access alloy.data.nav array — "+
					"proves arrays are preserved through JSON serialization")
		})

		It("parses alloy.hook() registrations from JS plugin", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "hooks.js"))).To(Succeed())

			hooks := rt.RegisteredHooks()
			Expect(hooks).NotTo(BeEmpty(),
				"EvalFile must parse alloy.hook() calls and register hook names")
			Expect(hooks).To(ContainElement("onContentTransformed"),
				"alloy.hook('onContentTransformed', ...) must be discovered")
		})

		It("parses alloy.on() as alias for alloy.hook()", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "hooks.js"))).To(Succeed())

			hooks := rt.RegisteredHooks()
			Expect(hooks).To(ContainElement("onPageRendered"),
				"alloy.on() must be treated as alias for alloy.hook()")
		})

		// ── Hook priority (issue #464) ──────────────────────────────
		// Hooks execute by priority (lower first), then registration
		// order within the same priority.

		It("hooks execute in priority order (lower first)", func() {
			registry := plugin.NewHookRegistry()
			var order []string

			registry.RegisterWithPriority(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "ssr")
				return payload, nil
			}, 100)

			registry.RegisterWithPriority(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "transforms")
				return payload, nil
			}, 10)

			_, err := registry.Run(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"transforms", "ssr"}),
				"hooks must execute in priority order — lower priority runs first. "+
					"transforms (10) must run before ssr (100) regardless of registration order")
		})

		It("default priority is 50", func() {
			registry := plugin.NewHookRegistry()
			var order []string

			// Register with default priority (50)
			registry.Register(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "default")
				return payload, nil
			})

			// Register with priority 10 (should run first)
			registry.RegisterWithPriority(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "early")
				return payload, nil
			}, 10)

			// Register with priority 90 (should run last)
			registry.RegisterWithPriority(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "late")
				return payload, nil
			}, 90)

			_, err := registry.Run(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"early", "default", "late"}),
				"Register without priority must default to 50 — "+
					"early (10) < default (50) < late (90)")
		})

		It("same priority preserves registration order", func() {
			registry := plugin.NewHookRegistry()
			var order []string

			registry.RegisterWithPriority(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "alpha")
				return payload, nil
			}, 50)

			registry.RegisterWithPriority(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "beta")
				return payload, nil
			}, 50)

			registry.RegisterWithPriority(plugin.OnPageRendered, func(ctx context.Context, payload interface{}) (interface{}, error) {
				order = append(order, "gamma")
				return payload, nil
			}, 50)

			_, err := registry.Run(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(order).To(Equal([]string{"alpha", "beta", "gamma"}),
				"hooks with the same priority must execute in registration order "+
					"(plugin load order: tier-first, then alphabetical within each tier)")
		})

		It("surfaces QuickJS error with plugin filename and line number", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "syntax-error.js"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("syntax-error.js"),
				ContainSubstring("SyntaxError"),
				ContainSubstring("line"),
			), "QuickJS error must include plugin filename or error details")
		})
	})

	// ── WASM Runtime ─────────────────────────────────────────────────

	Describe("WASM Runtime", func() {
		It("loads a WASM module via wazero", func() {
			rt := plugin.NewWASMRuntime()
			err := rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))
			Expect(err).NotTo(HaveOccurred(),
				"LoadModule must load a WASM file without error")
		})

		It("calls an exported WASM function", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			result, err := rt.CallExport("filter", "hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(),
				"WASM exported function must return a result")
		})

		It("surfaces WASM trap as user-facing error with plugin name", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			_, err := rt.CallExport("nonexistent_function")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("compiled.wasm"),
				ContainSubstring("export"),
				ContainSubstring("not found"),
			), "WASM error must include plugin name and call context")
		})
	})

	// ── WASM ABI error convention (#192) ─────────────────────────────
	// Per PLAN.md §5: if a WASM export returns (0, 0), the host treats
	// it as a plugin execution error — not an empty string.

	Describe("WASM ABI error convention", func() {
		It("CallExportRaw returns error when WASM function returns (0, 0)", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			// Simulate a WASM function that returns (0, 0) — this signals
			// an execution error per the ABI contract. CallExportRaw must
			// return an error, not an empty string.
			result, err := rt.CallExportRaw("filter", 0, 0)
			Expect(err).To(HaveOccurred(),
				"(0, 0) return from WASM must be treated as an execution error, not empty string")
			Expect(result).To(BeEmpty(),
				"error result must be empty")
		})

		It("CallExport does not return error for valid (ptr, len) return", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			// A normal filter call with valid input should return a
			// non-zero (ptr, len) and no error
			result, err := rt.CallExport("filter", "hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(),
				"valid WASM call must return a result")
		})
	})

	// ── WASM filter execution ───────────────────────────────────────
	// Issue #181: LoadModule is a stub. These tests verify actual WASM
	// execution — calling a filter and getting a real transformed result.

	Describe("WASM filter execution", func() {
		It("WASM filter transforms input and returns result", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			// CallExport with "filter" must execute the WASM function
			// and return a transformed value, not the input unchanged.
			// A passthrough stub must not satisfy this test.
			input := "hello world"
			result, err := rt.CallExport("filter", input)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil(),
				"WASM filter must return a non-nil result")
			Expect(result).NotTo(Equal(input),
				"WASM filter must transform the input — returning it unchanged "+
					"proves the WASM code did not execute")
		})

		It("WASM module registers discoverable filters", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			filters := rt.RegisteredFilters()
			Expect(filters).NotTo(BeEmpty(),
				"WASM module must register at least one filter via its exports")
		})

		It("WASM registered filter is callable through CallFilter", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			filters := rt.RegisteredFilters()
			Expect(filters).NotTo(BeEmpty())

			// Call the first registered filter by name
			result, err := rt.CallFilter(filters[0], "test input")
			Expect(err).NotTo(HaveOccurred(),
				"calling a WASM-registered filter by name must not error")
			Expect(result).NotTo(BeNil(),
				"WASM filter must return a result")
		})

		It("LoadModule returns error for invalid WASM binary", func() {
			rt := plugin.NewWASMRuntime()
			// Create a temp file with invalid WASM content
			tmpDir := GinkgoT().TempDir()
			badWasm := filepath.Join(tmpDir, "bad.wasm")
			Expect(os.WriteFile(badWasm, []byte("not a wasm file"), 0644)).To(Succeed())

			err := rt.LoadModule(badWasm)
			Expect(err).To(HaveOccurred(),
				"LoadModule must return error for invalid WASM binary")
		})

		// Issue #190: CallExport only handles single string argument.
		// Multiple args must be JSON-encoded and arrive correctly.
		It("CallExport passes multiple arguments as JSON array", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			// CallExport with multiple arguments must encode them as a
			// JSON array string so the WASM module can parse them.
			// Currently extra args are silently ignored.
			result, err := rt.CallExport("filter", "hello", "extra_arg1", "extra_arg2")
			Expect(err).NotTo(HaveOccurred(),
				"CallExport with multiple arguments must not error")
			Expect(result).NotTo(BeNil(),
				"CallExport with multiple arguments must return a result")

			// Multi-arg must behave differently from single-arg — proves
			// the extra arguments were not silently dropped.
			singleArgResult, err := rt.CallExport("filter", "hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(Equal(singleArgResult),
				"multi-arg CallExport must not behave the same as single-arg — "+
					"proves extra arguments are passed to the WASM function")
		})

		It("CallExport returns error for non-string arguments", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			// Non-string arguments can't be serialized to WASM linear memory
			// without explicit handling. CallExport must error, not silently
			// pass zero parameters.
			_, err := rt.CallExport("filter", 42)
			Expect(err).To(HaveOccurred(),
				"CallExport must return error for non-string arguments — "+
					"not silently ignore them or pass zero WASM parameters")
		})

		It("LoadModule rejects WASM module missing alloc export", func() {
			// A valid WASM binary without an alloc export must fail LoadModule.
			// alloc is required for safe memory allocation — without it,
			// the host has no safe way to write input to WASM memory.
			tmpDir := GinkgoT().TempDir()
			noAllocWasm := filepath.Join(tmpDir, "no-alloc.wasm")

			// Minimal valid WASM module: magic number + version, no exports
			Expect(os.WriteFile(noAllocWasm, []byte{
				0x00, 0x61, 0x73, 0x6d,
				0x01, 0x00, 0x00, 0x00,
			}, 0644)).To(Succeed())

			rt := plugin.NewWASMRuntime()
			err := rt.LoadModule(noAllocWasm)
			Expect(err).To(HaveOccurred(),
				"LoadModule must reject a valid WASM module that does not export alloc(size)")
			Expect(err.Error()).To(ContainSubstring("alloc"),
				"error must specifically mention the missing alloc export")
		})
	})

	// ── WASM pipeline bridging (#189) ───────────────────────────────
	// Registry.Runtimes() must include WASM runtimes so the pipeline
	// can bridge their filters into the template engine.

	Describe("WASM pipeline bridging", func() {
		It("WASMRuntime implements the same filter interface as QuickJSRuntime", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			// WASMRuntime must have RegisteredFilters and CallFilter
			// so the pipeline bridging loop can treat it like QuickJSRuntime
			filters := rt.RegisteredFilters()
			Expect(filters).NotTo(BeNil(),
				"WASMRuntime must implement RegisteredFilters()")

			if len(filters) > 0 {
				result, err := rt.CallFilter(filters[0], "test")
				Expect(err).NotTo(HaveOccurred(),
					"WASMRuntime must implement CallFilter()")
				Expect(result).NotTo(BeNil())
			}
		})

		// Issue #189: Registry.Runtimes() only returns QuickJS runtimes.
		// WASM runtimes loaded via LoadPlugins are not retained.
		It("Registry.Runtimes includes WASM runtimes after LoadPlugins", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)

			// Runtimes() must return both QuickJS and WASM runtimes
			// so the pipeline bridging loop can iterate all of them
			runtimes := registry.Runtimes()
			hasWASM := false
			for _, rt := range runtimes {
				if _, ok := any(rt).(*plugin.WASMRuntime); ok {
					hasWASM = true
					break
				}
			}
			Expect(hasWASM).To(BeTrue(),
				"Registry.Runtimes() must include WASM runtimes loaded from plugins/ — "+
					"currently only returns QuickJS runtimes, so WASM filters are never bridged")
		})
	})

	// ── Parallel plugin startup (issue #401) ────────────────────────
	// LoadPlugins should support concurrent runtime initialization
	// (Phase A) followed by sequential eval + registration (Phase B).
	// Parallel init must produce identical results to sequential.

	Describe("Parallel plugin startup (issue #401)", func() {
		It("InitRuntimes initializes multiple runtimes concurrently", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			Expect(registry.DiscoverPlugins()).To(Succeed())

			runtimes, warnings := registry.InitRuntimes()
			_ = warnings

			Expect(runtimes).NotTo(BeEmpty(),
				"InitRuntimes must return at least one initialized runtime — "+
					"if empty, concurrent init failed to produce any usable runtimes")
		})

		It("InitRuntimes returns Phase-A-only runtimes matching discovered plugin count", func() {
			// InitRuntimes is Phase A only — runtimes are initialized but
			// EvalFile has NOT been called, so no filters/hooks are registered.
			// This test verifies the correct number of runtimes are created,
			// matching the number of discovered plugins (minus any that fail init).
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			Expect(registry.DiscoverPlugins()).To(Succeed())
			plugins := registry.Plugins()

			runtimes, _ := registry.InitRuntimes()

			// At minimum, QuickJS plugins should init successfully.
			// WASM/Node may fail depending on test fixtures.
			Expect(len(runtimes)).To(BeNumerically("<=", len(plugins)),
				"InitRuntimes cannot return more runtimes than discovered plugins")
			Expect(runtimes).NotTo(BeEmpty(),
				"at least one plugin must init successfully")
		})

		It("InitRuntimes collects errors without blocking other plugins", func() {
			tmpDir := GinkgoT().TempDir()
			// Valid QuickJS plugin
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "good.js"),
				[]byte("export default function(alloy) { alloy.filter('good', (v) => v); }"),
				0644,
			)).To(Succeed())
			// Invalid WASM file — will fail LoadModule
			Expect(os.WriteFile(
				filepath.Join(tmpDir, "bad.wasm"),
				[]byte("not a wasm file"),
				0644,
			)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			DeferCleanup(registry.Close)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			runtimes, warnings := registry.InitRuntimes()
			Expect(warnings).NotTo(BeEmpty(),
				"InitRuntimes must collect warnings for plugins that fail to init — "+
					"bad.wasm should produce a warning, not block good.js")
			Expect(runtimes).NotTo(BeEmpty(),
				"good.js must still initialize even though bad.wasm failed — "+
					"one plugin's init failure must not block others")
		})
	})

	// ── Unified Runtime interface (#237) ────────────────────────────
	// All plugin tiers must implement the same Runtime interface.
	// The pipeline's LoadPlugins bridging loop treats all tiers
	// identically — no tier-specific code.

	Describe("Unified Runtime interface", func() {
		It("QuickJSRuntime implements RegisteredHooks and CallHook", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "hooks.js"))).To(Succeed())

			// Must implement RegisteredHooks — same interface as filters/shortcodes
			hooks := rt.RegisteredHooks()
			Expect(hooks).NotTo(BeEmpty(),
				"QuickJSRuntime must implement RegisteredHooks as part of Runtime interface")

			// Must implement CallHook — same interface as CallFilter/CallShortcode
			result, err := rt.CallHook(hooks[0], "<p>test</p>")
			Expect(err).NotTo(HaveOccurred(),
				"QuickJSRuntime must implement CallHook as part of Runtime interface")
			Expect(result).NotTo(BeNil())
		})

		// ── Issue #444: WASM hook registration and execution ─────────
		// WASM modules must support hooks via the hooks()/hook() export ABI.
		// WASMRuntime must implement CallHook so registerRuntime can bridge
		// WASM hooks to HookRegistry without tier-specific code.

		Context("WASM hook registration and execution (issue #444)", func() {
			It("WASMRuntime implements CallHook", func() {
				var rt interface{} = plugin.NewWASMRuntime()
				_, ok := rt.(interface {
					CallHook(string, interface{}) (interface{}, error)
				})
				Expect(ok).To(BeTrue(),
					"WASMRuntime must implement CallHook as part of the unified Runtime interface — "+
						"registerRuntime uses a type assertion for CallHook; without it, "+
						"WASM hooks are silently skipped (issue #444)")
			})

			It("WASMRuntime discovers hooks from hooks export", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

				hooks := rt.RegisteredHooks()
				Expect(hooks).NotTo(BeEmpty(),
					"WASMRuntime.RegisteredHooks must return hook names discovered from "+
						"the hooks() export — a WASM module exporting hooks() with a JSON "+
						"array of hook names must have those names appear here (issue #444)")
			})

			It("PluginFilterRuntime interface includes CallHook", func() {
				var rt plugin.PluginFilterRuntime = plugin.NewQuickJSRuntime()
				_ = rt
				_, ok := rt.(interface {
					CallHook(string, interface{}) (interface{}, error)
				})
				Expect(ok).To(BeTrue(),
					"CallHook must be part of PluginFilterRuntime (or at minimum, all "+
						"runtimes implementing PluginFilterRuntime must also implement "+
						"CallHook) so registerRuntime does not need a type assertion (issue #444)")
			})

			It("WASM module without hooks export returns empty RegisteredHooks", func() {
				rt := plugin.NewWASMRuntime()
				hooks := rt.RegisteredHooks()
				Expect(hooks).To(BeEmpty(),
					"WASMRuntime without a loaded module must return empty hooks — "+
						"no hooks export means no hooks registered (issue #444)")
			})

			It("LoadPlugins bridges WASM hooks to HookRegistry when CallHook is available", func() {
				tmpDir := GinkgoT().TempDir()
				src, err := os.ReadFile(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))
				Expect(err).NotTo(HaveOccurred())
				Expect(os.WriteFile(filepath.Join(tmpDir, "wasm-hook-plugin.wasm"), src, 0644)).To(Succeed())

				registry := plugin.NewRegistry(tmpDir)
				Expect(registry.DiscoverPlugins()).To(Succeed())

				hookRegistry := plugin.NewHookRegistry()
				registry.LoadPlugins(hookRegistry)
				DeferCleanup(registry.Close)

				Expect(hookRegistry.HasHooks(plugin.OnContentTransformed)).To(BeTrue(),
					"LoadPlugins must bridge WASM hooks to HookRegistry — "+
						"a WASM module registering onContentTransformed via hooks() export "+
						"must result in a registered hook in the HookRegistry (issue #444)")
			})

			It("WASM CallHook wraps event name in JSON payload", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

				var iface interface{} = rt
				caller, ok := iface.(interface {
					CallHook(string, interface{}) (interface{}, error)
				})
				if !ok {
					Fail("WASMRuntime must implement CallHook (issue #444)")
				}

				result, err := caller.CallHook("onContentTransformed", "<p>test</p>")
				Expect(err).NotTo(HaveOccurred(),
					"CallHook must marshal the event name and payload as JSON "+
						"and call the hook export — the WASM module processes the event (issue #444)")
				Expect(result).NotTo(BeNil(),
					"CallHook must return the (possibly modified) payload from the WASM module")
			})

			It("WASM hooks get default priority 50", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

				var iface interface{} = rt
				detailer, ok := iface.(plugin.HookDetailer)
				if !ok {
					hooks := rt.RegisteredHooks()
					Expect(hooks).NotTo(BeEmpty(),
						"WASMRuntime must report hooks for priority to be meaningful (issue #444)")
					return
				}
				details := detailer.RegisteredHookDetails()
				Expect(details).NotTo(BeEmpty())
				for _, reg := range details {
					Expect(reg.Priority).To(Equal(50),
						"WASM hooks must default to priority 50 — no mechanism for "+
							"per-hook priority in the WASM ABI (issue #444)")
				}
			})

			It("CallHook returns error when hook export returns (0,0)", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

				var iface interface{} = rt
				caller, ok := iface.(interface {
					CallHook(string, interface{}) (interface{}, error)
				})
				if !ok {
					Fail("WASMRuntime must implement CallHook (issue #444)")
				}

				_, err := caller.CallHook("onUnknownEvent", "<p>test</p>")
				Expect(err).To(HaveOccurred(),
					"CallHook must return an error when the hook export returns (0,0) — "+
						"per the WASM ABI error convention, (0,0) signals a plugin execution "+
						"error and the host must check last_error() for details (issue #444)")
			})

			It("LoadModule returns error when hooks() export returns invalid JSON", func() {
				rt := plugin.NewWASMRuntime()
				err := rt.LoadModule(filepath.Join(testdataDir(), "single-files", "bad-hooks-export.wasm"))
				Expect(err).To(HaveOccurred(),
					"LoadModule must return an error when the hooks() export returns "+
						"data that is not a valid JSON array of strings — malformed hook "+
						"discovery must fail loud, not silently treat the module as hook-less (issue #444)")
				Expect(err.Error()).NotTo(ContainSubstring("not found"),
					"error must be about invalid hooks() data, not a missing module — "+
						"bad-hooks-export.wasm must exist in testdata/single-files/ and export "+
						"hooks() returning invalid JSON (non-array, malformed, or non-string elements)")
			})

			It("CallHook returns error for malformed WASM JSON response", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-malformed-hook-response.wasm"))).To(Succeed(),
					"wasm-malformed-hook-response.wasm fixture must exist — "+
						"a WASM module whose hook() export returns valid (ptr, len) "+
						"pointing to bytes that are not valid JSON (issue #444)")

				var iface interface{} = rt
				caller, ok := iface.(interface {
					CallHook(string, interface{}) (interface{}, error)
				})
				if !ok {
					Fail("WASMRuntime must implement CallHook (issue #444)")
				}

				_, err := caller.CallHook("onContentTransformed", "<p>test</p>")
				Expect(err).To(HaveOccurred(),
					"CallHook must return an error when the hook export returns bytes "+
						"that are not valid JSON — do not silently return nil or fall back "+
						"to the original payload (issue #444)")
			})
		})

		// ── Issue #742: WASM per-hook priority and scope metadata ─────
		// The hooks() export must accept mixed-type JSON arrays: plain
		// strings (backward compat, priority 50, nil scope) and objects
		// with name, priority, and scope fields. discoverHooks() must
		// type-switch each element and store full HookRegistration data
		// so RegisteredHookDetails() returns real values instead of
		// hardcoded priority 50 / nil scope.

		Context("WASM per-hook priority and scope metadata (issue #742)", func() {
			It("mixed-type hooks array parses strings and objects correctly", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-mixed-hooks.wasm"))).To(Succeed(),
					"LoadModule must succeed when hooks() returns a mixed JSON array "+
						"containing both plain strings and registration objects — "+
						"discoverHooks() must unmarshal into []interface{} and type-switch "+
						"each element (issue #742)")

				hooks := rt.RegisteredHooks()
				Expect(hooks).To(ContainElement("onBuildComplete"),
					"plain string entries in the mixed array must appear in RegisteredHooks — "+
						"backward-compatible string parsing must survive the switch to "+
						"[]interface{} unmarshaling (issue #742)")
				Expect(hooks).To(ContainElement("onContentTransformed"),
					"object entries with a name field must appear in RegisteredHooks — "+
						"discoverHooks must extract the name from registration objects "+
						"and include it in the hook names list (issue #742)")

				var iface interface{} = rt
				detailer, ok := iface.(plugin.HookDetailer)
				Expect(ok).To(BeTrue(), "WASMRuntime must implement HookDetailer (issue #742)")
				details := detailer.RegisteredHookDetails()
				Expect(details).To(HaveLen(2),
					"mixed array with one string and one object must produce exactly "+
						"two hook registrations (issue #742)")

				var stringReg, objectReg plugin.HookRegistration
				for _, reg := range details {
					switch reg.Name {
					case "onBuildComplete":
						stringReg = reg
					case "onContentTransformed":
						objectReg = reg
					}
				}

				Expect(stringReg.Name).To(Equal("onBuildComplete"),
					"string entry must be present in hook details (issue #742)")
				Expect(stringReg.Priority).To(Equal(50),
					"plain string entries must default to priority 50 — "+
						"backward compatibility with the pure-string format (issue #742)")
				Expect(stringReg.Scope).To(BeNil(),
					"plain string entries must have nil scope — "+
						"no scope metadata in string format (issue #742)")

				Expect(objectReg.Name).To(Equal("onContentTransformed"),
					"object entry name field must be extracted correctly (issue #742)")
				Expect(objectReg.Priority).To(Equal(10),
					"object entry priority field must be extracted — "+
						"discoverHooks must parse the priority from the registration "+
						"object and store it in the HookRegistration (issue #742)")
				Expect(objectReg.Scope).NotTo(BeNil(),
					"object entry with scope fields must produce a non-nil HookScope — "+
						"discoverHooks must pass scope fields to parseScopeMap (issue #742)")
				Expect(objectReg.Scope.Pages.Mode).To(Equal(plugin.PagesScopeGlob),
					"pages: \"blog/**\" must produce PagesScopeGlob — "+
						"scope fields in the registration object must be parsed "+
						"identically to QuickJS/Node scope handling (issue #742)")
				Expect(objectReg.Scope.Pages.Glob).To(Equal("blog/**"),
					"glob pattern must be preserved in scope (issue #742)")
				Expect(objectReg.Scope.Data).To(Equal([]string{"navigation", "team"}),
					"data array must be extracted from scope fields — "+
						"limits site data serialized across the WASM memory boundary (issue #742)")
				Expect(objectReg.Scope.PageFields).To(Equal([]string{"title", "url", "tags"}),
					"pageFields array must be extracted from scope fields — "+
						"limits per-page field serialization across the WASM memory "+
						"boundary (issue #742)")
			})

			It("object with only name defaults to priority 50 and nil scope", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-name-only-hooks.wasm"))).To(Succeed(),
					"LoadModule must succeed when hooks() returns an object with only "+
						"a name field — priority and scope fields are optional (issue #742)")

				var iface interface{} = rt
				detailer, ok := iface.(plugin.HookDetailer)
				Expect(ok).To(BeTrue(), "WASMRuntime must implement HookDetailer (issue #742)")
				details := detailer.RegisteredHookDetails()
				Expect(details).To(HaveLen(1))

				reg := details[0]
				Expect(reg.Name).To(Equal("onContentTransformed"))
				Expect(reg.Priority).To(Equal(50),
					"omitted priority must default to 50 — same default as plain "+
						"string entries, consistent with QuickJS/Node behavior (issue #742)")
				Expect(reg.Scope).To(BeNil(),
					"omitted scope fields must produce nil scope — "+
						"no scope metadata means full payload (issue #742)")
			})

			It("object with priority uses provided value", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-priority-only-hooks.wasm"))).To(Succeed(),
					"LoadModule must succeed when hooks() returns an object with "+
						"name and priority but no scope fields (issue #742)")

				var iface interface{} = rt
				detailer, ok := iface.(plugin.HookDetailer)
				Expect(ok).To(BeTrue(), "WASMRuntime must implement HookDetailer (issue #742)")
				details := detailer.RegisteredHookDetails()
				Expect(details).To(HaveLen(1))

				reg := details[0]
				Expect(reg.Name).To(Equal("onContentTransformed"))
				Expect(reg.Priority).To(Equal(25),
					"priority field must be extracted from the registration object — "+
						"WASM plugins must be able to control execution order relative "+
						"to other plugins via per-hook priority (issue #742)")
				Expect(reg.Scope).To(BeNil(),
					"omitted scope fields must produce nil scope even when priority "+
						"is provided (issue #742)")
			})

			It("object missing name field returns error", func() {
				rt := plugin.NewWASMRuntime()
				err := rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-missing-name-hooks.wasm"))
				Expect(err).To(HaveOccurred(),
					"LoadModule must return an error when a hooks() registration object "+
						"is missing the required name field — the name identifies which "+
						"hook event to register for (issue #742)")
				Expect(err.Error()).To(ContainSubstring("name"),
					"error message must mention the missing name field so plugin "+
						"authors can diagnose the malformed hooks() return value (issue #742)")
			})

			It("object with non-string name returns error", func() {
				rt := plugin.NewWASMRuntime()
				err := rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-bad-name-type-hooks.wasm"))
				Expect(err).To(HaveOccurred(),
					"LoadModule must return an error when a hooks() registration object "+
						"has a non-string name field — name must be a string identifying "+
						"the hook event (issue #742)")
				Expect(err.Error()).To(ContainSubstring("name"),
					"error message must mention the name field type mismatch so plugin "+
						"authors can diagnose the malformed hooks() return value (issue #742)")
			})

			It("scope with pages: false parses to PagesScopeNone", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-scope-pages-false.wasm"))).To(Succeed(),
					"LoadModule must succeed when hooks() returns an object with "+
						"pages: false scope — parseScopeMap handles boolean pages (issue #742)")

				var iface interface{} = rt
				detailer, ok := iface.(plugin.HookDetailer)
				Expect(ok).To(BeTrue(), "WASMRuntime must implement HookDetailer (issue #742)")
				details := detailer.RegisteredHookDetails()
				Expect(details).To(HaveLen(1))

				reg := details[0]
				Expect(reg.Scope).NotTo(BeNil(),
					"pages: false must produce a non-nil HookScope — "+
						"PagesScopeNone is an active opt-out, not the absence of scope (issue #742)")
				Expect(reg.Scope.Pages.Mode).To(Equal(plugin.PagesScopeNone),
					"pages: false must produce PagesScopeNone — hooks with this "+
						"scope skip page dispatch entirely, reducing unnecessary "+
						"serialization across the WASM memory boundary (issue #742)")
			})

			It("scope with taxonomy pages parses to PagesScopeTaxonomy", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-scope-taxonomy.wasm"))).To(Succeed(),
					"LoadModule must succeed when hooks() returns an object with "+
						"taxonomy scope — parseScopeMap handles map-valued pages (issue #742)")

				var iface interface{} = rt
				detailer, ok := iface.(plugin.HookDetailer)
				Expect(ok).To(BeTrue(), "WASMRuntime must implement HookDetailer (issue #742)")
				details := detailer.RegisteredHookDetails()
				Expect(details).To(HaveLen(1))

				reg := details[0]
				Expect(reg.Scope).NotTo(BeNil(),
					"taxonomy scope must produce a non-nil HookScope (issue #742)")
				Expect(reg.Scope.Pages.Mode).To(Equal(plugin.PagesScopeTaxonomy),
					"pages: {\"tags\": [\"go\", \"wasm\"]} must produce PagesScopeTaxonomy — "+
						"WASM plugins must be able to filter to pages matching specific "+
						"taxonomy terms (issue #742)")
				Expect(reg.Scope.Pages.Taxonomies).To(HaveKeyWithValue("tags", []string{"go", "wasm"}),
					"taxonomy terms must be parsed correctly from the registration "+
						"object — parseScopeMap must handle the polymorphic pages field "+
						"identically for WASM and QuickJS/Node plugins (issue #742)")
			})

			It("pure string array still works after mixed-type support (backward compat)", func() {
				rt := plugin.NewWASMRuntime()
				Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed(),
					"existing WASM modules returning pure [\"hookName\"] arrays must "+
						"continue to load successfully — the switch from []string to "+
						"[]interface{} unmarshaling must not break existing modules (issue #742)")

				hooks := rt.RegisteredHooks()
				Expect(hooks).To(ContainElement("onContentTransformed"),
					"hook names from pure string arrays must still be discovered (issue #742)")

				var iface interface{} = rt
				detailer, ok := iface.(plugin.HookDetailer)
				Expect(ok).To(BeTrue(), "WASMRuntime must implement HookDetailer (issue #742)")
				details := detailer.RegisteredHookDetails()
				Expect(details).NotTo(BeEmpty())
				for _, reg := range details {
					Expect(reg.Priority).To(Equal(50),
						"pure string entries must default to priority 50 — "+
							"the mixed-type parsing must preserve backward-compatible "+
							"defaults for string elements (issue #742)")
					Expect(reg.Scope).To(BeNil(),
						"pure string entries must have nil scope (issue #742)")
				}
			})

			It("unsupported element type in hooks array returns error", func() {
				rt := plugin.NewWASMRuntime()
				err := rt.LoadModule(filepath.Join(testdataDir(), "single-files", "wasm-bad-element-type-hooks.wasm"))
				Expect(err).To(HaveOccurred(),
					"LoadModule must return an error when a hooks() array element is "+
						"neither a string nor an object — numbers, booleans, and nulls "+
						"are not valid hook registrations (issue #742)")
			})

			It("WASM priority survives full LoadPlugins bridge path", func() {
				tmpDir := GinkgoT().TempDir()
				src, err := os.ReadFile(filepath.Join(testdataDir(), "single-files", "wasm-priority-only-hooks.wasm"))
				Expect(err).NotTo(HaveOccurred())
				Expect(os.WriteFile(filepath.Join(tmpDir, "wasm-priority-plugin.wasm"), src, 0644)).To(Succeed())

				registry := plugin.NewRegistry(tmpDir)
				Expect(registry.DiscoverPlugins()).To(Succeed())

				hookRegistry := plugin.NewHookRegistry()
				registry.LoadPlugins(hookRegistry)
				DeferCleanup(registry.Close)

				Expect(hookRegistry.HasHooks(plugin.OnContentTransformed)).To(BeTrue(),
					"LoadPlugins must bridge WASM hooks with per-hook priority to "+
						"HookRegistry — a WASM module registering onContentTransformed "+
						"with priority 25 via hooks() object format must result in a "+
						"registered hook in the HookRegistry (issue #742)")

				priorities := plugin.HookPriorities(hookRegistry, plugin.OnContentTransformed)
				Expect(priorities).To(HaveLen(1),
					"exactly one hook should be registered for onContentTransformed (issue #742)")
				Expect(priorities[0]).To(Equal(25),
					"the registered hook priority must be 25, not the default 50 — "+
						"verifies that per-hook priority from hooks() object format "+
						"survives the full path: discoverHooks → RegisteredHookDetails → "+
						"registerRuntime → RegisterWithPriority (issue #742)")
			})
		})

		It("NodeRuntime implements the Runtime interface", func() {
			// NodeRuntime must exist and implement the same interface as
			// QuickJSRuntime and WASMRuntime. The pipeline bridging loop
			// iterates []Runtime — all three types must be assignable.
			rt := plugin.NewNodeRuntime()
			Expect(rt).NotTo(BeNil(),
				"NewNodeRuntime must return a non-nil runtime")

			// Methods must be callable without panic before Init.
			// Return value may be nil or empty — the point is the
			// method exists on the type.
			_ = rt.RegisteredFilters()
			_ = rt.RegisteredShortcodes()
			_ = rt.RegisteredHooks()
		})

		// ── Issue #241: Tier 3 Node plugin evaluation and subprocess ──
		// These tests are in the unified Runtime section because
		// NodeRuntime implements the same interface. They specifically
		// test Node subprocess spawning and JS evaluation.

		It("NodeRuntime.EvalFile discovers hooks and filters from JS plugin", func() {
			rt := plugin.NewNodeRuntime()

			err := rt.EvalFile(filepath.Join(testdataDir(), "single-files", "node-simple.js"))
			Expect(err).NotTo(HaveOccurred(),
				"NodeRuntime must evaluate Node plugin JS files")

			// node-simple.js registers: filter "nodeUpper" + hook "onContentTransformed"
			filters := rt.RegisteredFilters()
			Expect(filters).To(ContainElement("nodeUpper"),
				"NodeRuntime must discover filters registered via alloy.filter() in JS")

			hooks := rt.RegisteredHooks()
			Expect(hooks).To(ContainElement("onContentTransformed"),
				"NodeRuntime must discover hooks registered via alloy.hook() in JS")
		})

		It("NodeRuntime.CallFilter routes call through subprocess and returns result", func() {
			rt := plugin.NewNodeRuntime()
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "node-simple.js"))).To(Succeed())

			// nodeUpper converts to uppercase — proves the JS function executed
			result, err := rt.CallFilter("nodeUpper", "hello alloy")
			Expect(err).NotTo(HaveOccurred(),
				"NodeRuntime.CallFilter must route to Node subprocess")
			Expect(result).To(Equal("HELLO ALLOY"),
				"Node filter must transform input — proves JS function executed, "+
					"not just returned input unchanged")
		})

		It("NodeRuntime.CallHook routes call through subprocess and returns modified payload", func() {
			rt := plugin.NewNodeRuntime()
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "node-simple.js"))).To(Succeed())

			// onContentTransformed appends "<!-- node-plugin -->"
			result, err := rt.CallHook("onContentTransformed", "<p>test</p>")
			Expect(err).NotTo(HaveOccurred(),
				"NodeRuntime.CallHook must route to Node subprocess")
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue())
			Expect(resultStr).To(ContainSubstring("<!-- node-plugin -->"),
				"Node hook must modify the payload — proves JS function executed "+
					"via subprocess, not just returned input unchanged")
		})

		// ── Issue #441: ESM import replaces eval() ──────────────────
		// EvalFile must send the absolute file path to the bridge,
		// not the source code. The bridge uses import() instead of eval().

		It("EvalFile loads ESM plugin with import statements (issue #441)", func() {
			// This plugin uses `import { basename } from "node:path"` —
			// an ESM import statement that eval() CANNOT handle.
			// If this test passes, the bridge is using import(), not eval().
			rt := plugin.NewNodeRuntime()
			pluginPath := filepath.Join(testdataDir(), "single-files", "node-esm-import.js")

			err := rt.EvalFile(pluginPath)
			Expect(err).NotTo(HaveOccurred(),
				"EvalFile must load ESM plugins with import statements — "+
					"if this fails with a syntax error, the bridge is still using eval() "+
					"instead of import(). eval() cannot handle import statements (issue #441)")

			filters := rt.RegisteredFilters()
			Expect(filters).To(ContainElement("baseName"),
				"ESM plugin filter must be discovered via import()")

			// Prove the import actually works — basename("a/b/c.txt") → "c.txt"
			result, err := rt.CallFilter("baseName", "/path/to/file.txt")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("file.txt"),
				"ESM import of node:path must work — proves import() resolved the module")
		})

		It("NodeBridge.Start spawns a Node subprocess", func() {
			bridge := plugin.NewNodeBridge(filepath.Join(testdataDir()))
			err := bridge.Start()
			Expect(err).NotTo(HaveOccurred(),
				"NodeBridge.Start must spawn a Node subprocess")
			Expect(bridge.State()).To(Equal(plugin.BridgeRunning),
				"bridge state must be Running after Start")

			// Verify an actual process is running — PID must be non-zero
			Expect(bridge.PID()).To(BeNumerically(">", 0),
				"NodeBridge must have a non-zero PID after Start — "+
					"proves a real subprocess was spawned, not just a state change")

			Expect(bridge.Stop()).To(Succeed(),
				"NodeBridge.Stop must cleanly shut down the subprocess")
			Expect(bridge.State()).To(Equal(plugin.BridgeStopped),
				"bridge state must be Stopped after Stop")
		})

		// ── Issue #248: Node module resolution ──────────────────────
		// The Node subprocess must resolve imports from the project root,
		// not from the temp directory where the bridge script lives.

		It("NodeBridge runs from project root for module resolution", func() {
			// The bridge must set cmd.Dir to the project root so Node
			// resolves imports from the project's node_modules/
			bridge := plugin.NewNodeBridge(filepath.Join(testdataDir()))
			Expect(bridge.Start()).To(Succeed())
			DeferCleanup(bridge.Stop)

			// The subprocess's working directory must be the project root
			Expect(bridge.WorkingDir()).To(Equal(filepath.Join(testdataDir())),
				"NodeBridge subprocess must run from the project root — "+
					"not the temp directory where the bridge script lives. "+
					"Without this, import('@lit-labs/ssr') fails because "+
					"node_modules/ can't be found from the temp path.")
		})

		It("NodeRuntime passes project root to bridge", func() {
			// When LoadPlugins creates a NodeRuntime, it must pass the
			// project root so the bridge subprocess can resolve imports.
			// Currently NewNodeRuntime() passes "" (empty string).
			rt := plugin.NewNodeRuntime()
			Expect(rt.ProjectRoot()).NotTo(BeEmpty(),
				"NodeRuntime must know the project root so it can pass it "+
					"to NewNodeBridge for module resolution")
		})

		It("Registry.Runtimes includes Node runtimes after LoadPlugins", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)

			// Runtimes() must return all tiers — QuickJS, WASM, and Node
			runtimes := registry.Runtimes()
			Expect(runtimes).NotTo(BeEmpty(),
				"Registry.Runtimes() must return loaded runtimes")

			// All returned runtimes must implement the full Runtime interface.
			// Methods may return nil or empty slices — the point is they exist
			// and are callable. (WASM runtimes may not support shortcodes/hooks
			// until their exports are discovered.)
			for _, rt := range runtimes {
				// These must not panic — proves the method exists on the type
				_ = rt.RegisteredFilters()
				_ = rt.RegisteredShortcodes()
				_ = rt.RegisteredHooks()
			}
		})

		// ── Issue #244/#245: LoadPlugins Node wiring ────────────────
		// LoadPlugins must call EvalFile on Node plugins, register
		// discovered filters, and bridge hooks to HookRegistry.

		It("LoadPlugins evaluates Node plugins and registers their filters", func() {
			// Use a dedicated temp dir with just node-simple.js to avoid
			// noise from other fixtures in single-files/
			tmpDir := GinkgoT().TempDir()
			src, err := os.ReadFile(filepath.Join(testdataDir(), "single-files", "node-simple.js"))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(tmpDir, "node-simple.js"), src, 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)
			DeferCleanup(registry.Close)

			// node-simple.js registers filter "nodeUpper" via alloy.filter().
			// LoadPlugins must call EvalFile to discover this registration
			// and add it to the registry's filter list.
			Expect(registry.HasFilter("nodeUpper")).To(BeTrue(),
				"LoadPlugins must call EvalFile on Node plugins and register "+
					"discovered filters — nodeUpper from node-simple.js must be registered")
		})

		It("LoadPlugins bridges Node hooks to HookRegistry", func() {
			tmpDir := GinkgoT().TempDir()
			src, err := os.ReadFile(filepath.Join(testdataDir(), "single-files", "node-simple.js"))
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(tmpDir, "node-simple.js"), src, 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hookRegistry := plugin.NewHookRegistry()
			registry.LoadPlugins(hookRegistry)
			DeferCleanup(registry.Close)

			// node-simple.js hooks onContentTransformed and appends a marker.
			// LoadPlugins must bridge this hook to the HookRegistry.
			input := "<p>test</p>"
			result, err := hookRegistry.Run(plugin.OnContentTransformed, input)
			Expect(err).NotTo(HaveOccurred(),
				"LoadPlugins must bridge Node hooks to HookRegistry")
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue())
			Expect(resultStr).To(ContainSubstring("<!-- node-plugin -->"),
				"Node hook must fire via HookRegistry and modify the payload — "+
					"proves LoadPlugins called EvalFile AND bridged the hook")
		})

		It("LoadPlugins continues with warning when Node plugin EvalFile fails", func() {
			// Create a temp directory with a broken Node plugin
			tmpDir := GinkgoT().TempDir()
			brokenPlugin := filepath.Join(tmpDir, "broken-node.js")
			Expect(os.WriteFile(brokenPlugin, []byte(`export const runtime = "node";\n{{{ invalid js`), 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			warnings := registry.LoadPlugins(hooks)
			Expect(warnings).NotTo(BeEmpty(),
				"LoadPlugins must return a warning when Node plugin EvalFile fails — "+
					"not abort the entire plugin loading process")
		})

		It("LoadPlugins bridges hooks to HookRegistry", func() {
			// Use a QuickJS runtime directly with hook-modifier.js which
			// appends a marker. This avoids dependency on Node availability.
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(filepath.Join(testdataDir(), "single-files", "hook-modifier.js"))).To(Succeed())

			hooks := rt.RegisteredHooks()
			Expect(hooks).To(ContainElement("onContentTransformed"),
				"hook-modifier.js must register onContentTransformed")

			// Simulate what LoadPlugins does: bridge discovered hooks
			hookRegistry := plugin.NewHookRegistry()
			for _, hookName := range hooks {
				name := hookName
				runtime := rt
				hookRegistry.Register(plugin.HookName(name), func(_ context.Context, payload interface{}) (interface{}, error) {
					return runtime.CallHook(name, payload)
				})
			}

			// Fire the hook — proves the bridging pattern works
			input := "<p>test</p>"
			result, err := hookRegistry.Run(plugin.OnContentTransformed, input)
			Expect(err).NotTo(HaveOccurred(),
				"bridged hook must execute without error")
			resultStr, ok := result.(string)
			Expect(ok).To(BeTrue(),
				"hook result must be a string")
			Expect(resultStr).To(ContainSubstring("<!-- hook-modified -->"),
				"hook must modify the payload — proves CallHook executed the JS function "+
					"and the bridging pattern works for all runtimes")
		})
	})

	// ── Sandbox enforcement ──────────────────────────────────────────

	Describe("Sandbox enforcement", func() {
		It("Tier 2 runtime has no filesystem access", func() {
			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())

			err := plugin.CheckSandbox(rt)
			Expect(err).NotTo(HaveOccurred(),
				"Tier 2 sandbox must prevent filesystem access")
		})

		It("Tier 2 runtime has no network access", func() {
			rt := plugin.NewWASMRuntime()
			err := plugin.CheckSandbox(rt)
			Expect(err).NotTo(HaveOccurred(),
				"Tier 2 sandbox must prevent network access")
		})
	})

	// ── WASM compilation cache (issue #391) ──────────────────────────
	// WASMRuntime.LoadModule must support a compilation cache directory
	// so compiled native code persists across builds. This eliminates
	// the 509ms WASM recompilation cost on warm builds.

	Describe("WASM compilation cache (issue #391)", func() {
		It("LoadModule accepts a cache directory for compiled modules", func() {
			cacheDir := GinkgoT().TempDir()
			rt := plugin.NewWASMRuntime()
			rt.SetCacheDir(cacheDir)
			err := rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))
			Expect(err).NotTo(HaveOccurred(),
				"LoadModule with cache directory must not error")

			// Verify the cache directory is not empty after loading
			entries, err := os.ReadDir(cacheDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entries).NotTo(BeEmpty(),
				"compilation cache directory must contain cached artifacts after LoadModule — "+
					"if empty, wazero compilation cache is not configured")
		})

		It("warm build reuses cache from cold build", func() {
			cacheDir := GinkgoT().TempDir()
			wasmPath := filepath.Join(testdataDir(), "single-files", "compiled.wasm")

			// Cold build — first compilation, populates cache
			rt1 := plugin.NewWASMRuntime()
			rt1.SetCacheDir(cacheDir)
			Expect(rt1.LoadModule(wasmPath)).To(Succeed())
			rt1.Close()

			// Record cache state after cold build
			entriesAfterCold, err := os.ReadDir(cacheDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(entriesAfterCold).NotTo(BeEmpty(),
				"cache directory must be populated after cold build")

			// Warm build — cache already populated, must still succeed
			rt2 := plugin.NewWASMRuntime()
			rt2.SetCacheDir(cacheDir)
			Expect(rt2.LoadModule(wasmPath)).To(Succeed(),
				"LoadModule must succeed when loading from a pre-populated cache — "+
					"if this fails, the cache format is incompatible across loads")
			rt2.Close()

			// Cache should still contain artifacts (not wiped)
			entriesAfterWarm, err := os.ReadDir(cacheDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(len(entriesAfterWarm)).To(BeNumerically(">=", len(entriesAfterCold)),
				"warm build must not wipe the cache directory")
		})

		It("cached module produces identical filter output", func() {
			cacheDir := GinkgoT().TempDir()
			wasmPath := filepath.Join(testdataDir(), "single-files", "compiled.wasm")

			// Cold build
			rt1 := plugin.NewWASMRuntime()
			rt1.SetCacheDir(cacheDir)
			Expect(rt1.LoadModule(wasmPath)).To(Succeed())
			result1, err1 := rt1.CallFilter("filter", "hello")
			rt1.Close()

			// Warm build
			rt2 := plugin.NewWASMRuntime()
			rt2.SetCacheDir(cacheDir)
			Expect(rt2.LoadModule(wasmPath)).To(Succeed())
			result2, err2 := rt2.CallFilter("filter", "hello")
			rt2.Close()

			Expect(err1).NotTo(HaveOccurred())
			Expect(err2).NotTo(HaveOccurred())
			Expect(result2).To(Equal(result1),
				"cached WASM module must produce identical output to uncached — "+
					"compilation cache must not affect runtime behavior")
		})
	})

	// ── Hook priority through EvalFile → registerRuntime (issue #478) ────
	// Unit tests exercise RegisterWithPriority directly on HookRegistry.
	// This integration test verifies the full JS→Go bridge path:
	// alloy.hook(name, { priority }, fn) → __registerHook → RegisteredHookDetails →
	// registerRuntime → RegisterWithOptions → execution order.
	// (QuickJS always passes scope JSON, so registerRuntime takes the
	// RegisterWithOptions path, not RegisterWithPriority.)

	Describe("Hook priority through EvalFile → registerRuntime (issue #478)", func() {
		It("priority option survives full JS→Go bridge path and controls execution order", func() {
			tmpDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(tmpDir, "priority-alpha.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 100 }, function(html) {
    return html + '[alpha]';
  });
}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "priority-beta.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10 }, function(html) {
    return html + '[beta]';
  });
}`), 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)
			DeferCleanup(registry.Close)

			result, err := hooks.RunWithTimeout(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<p>test</p>[beta][alpha]"),
				"priority 10 (beta) must run before priority 100 (alpha) — "+
					"DiscoverPlugins sorts alphabetically so alpha registers first, "+
					"but priority must override registration order. If this fails, "+
					"the JS→Go priority bridge is broken somewhere in the chain: "+
					"alloy.hook() → __registerHook → RegisteredHookDetails → "+
					"registerRuntime → RegisterWithOptions (issue #478)")
		})

		It("omitted priority defaults to 50 through full JS→Go bridge path", func() {
			tmpDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(tmpDir, "default-priority.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(html) {
    return html + '[default]';
  });
}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "explicit-priority.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 100 }, function(html) {
    return html + '[explicit]';
  });
}`), 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)
			DeferCleanup(registry.Close)

			result, err := hooks.RunWithTimeout(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<p>test</p>[default][explicit]"),
				"omitted priority must default to 50 and run before explicit priority 100 — "+
					"verifies the ternary default branch in alloy.hook() JS bridge "+
					"survives the full registration path (issue #478)")
		})

		It("priority 0 is a valid priority through full JS→Go bridge path", func() {
			tmpDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(tmpDir, "priority-first.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 50 }, function(html) {
    return html + '[normal]';
  });
}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "priority-zero.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 0 }, function(html) {
    return html + '[zero]';
  });
}`), 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)
			DeferCleanup(registry.Close)

			result, err := hooks.RunWithTimeout(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<p>test</p>[zero][normal]"),
				"priority 0 must be treated as a valid priority, not ignored — "+
					"must run before default priority 50 (issue #478)")
		})

		It("same priority preserves registration order through full JS→Go bridge path", func() {
			tmpDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(tmpDir, "same-priority-alpha.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 50 }, function(html) {
    return html + '[alpha]';
  });
}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "same-priority-beta.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 50 }, function(html) {
    return html + '[beta]';
  });
}`), 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)
			DeferCleanup(registry.Close)

			result, err := hooks.RunWithTimeout(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<p>test</p>[alpha][beta]"),
				"equal priority must preserve alphabetical registration order — "+
					"DiscoverPlugins sorts alphabetically so alpha registers first "+
					"and must run first when priorities match (issue #478)")
		})

		It("negative priority runs before positive priorities through full JS→Go bridge path", func() {
			tmpDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(tmpDir, "negative-priority.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: -10 }, function(html) {
    return html + '[negative]';
  });
}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "positive-priority.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10 }, function(html) {
    return html + '[positive]';
  });
}`), 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)
			DeferCleanup(registry.Close)

			result, err := hooks.RunWithTimeout(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<p>test</p>[negative][positive]"),
				"negative priority must run before positive priority — "+
					"verifies signed integer handling through the full JS→Go bridge (issue #478)")
		})

		It("non-integer priority is floored through full JS→Go bridge path", func() {
			tmpDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(tmpDir, "float-priority.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10.9 }, function(html) {
    return html + '[float]';
  });
}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tmpDir, "integer-priority.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 11 }, function(html) {
    return html + '[integer]';
  });
}`), 0644)).To(Succeed())

			registry := plugin.NewRegistry(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			registry.LoadPlugins(hooks)
			DeferCleanup(registry.Close)

			result, err := hooks.RunWithTimeout(plugin.OnPageRendered, "<p>test</p>")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("<p>test</p>[float][integer]"),
				"priority 10.9 must be floored to 10 and run before priority 11 — "+
					"verifies Math.floor() in alloy.hook() JS bridge survives "+
					"the full registration path (issue #478)")
		})
	})

	// ── CallHook payload struct fast path (issue #1180) ──────────────
	// QuickJS CallHook currently JSON-serializes the entire payload for
	// struct types (HookRenderedPayload, HookFormatRenderedPayload,
	// HookTransformPayload), which means ~800KB HTML goes through 4
	// serialization passes per page. The fix adds type-specific cases
	// that build JS objects directly via the QuickJS API, only
	// JSON-serializing the small frontMatter map. These tests verify
	// the behavioral contract that the fast path must satisfy.

	Describe("CallHook payload struct fast path (issue #1180)", func() {

		It("HookRenderedPayload delivers all fields to JS and returns modified html", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    if (typeof page !== 'object') return { html: 'ERROR: expected object, got ' + typeof page };
    if (typeof page.html !== 'string') return { html: 'ERROR: html is ' + typeof page.html };
    if (typeof page.url !== 'string') return { html: 'ERROR: url is ' + typeof page.url };
    if (typeof page.path !== 'string') return { html: 'ERROR: path is ' + typeof page.path };
    if (typeof page.frontMatter !== 'object') return { html: 'ERROR: frontMatter is ' + typeof page.frontMatter };
    return {
      html: page.html + '<!-- url:' + page.url + ' path:' + page.path + ' title:' + page.frontMatter.title + ' -->'
    };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<h1>Hello</h1>",
				FrontMatter: map[string]interface{}{"title": "Test Page"},
				URL:         "/test/",
				Path:        "content/test.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must not error when dispatching HookRenderedPayload — "+
					"the type switch must handle this struct type, either via a "+
					"dedicated fast path case or the existing JSON default case")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"CallHook with HookRenderedPayload must return a map — "+
					"got %T; the hook returns {html: ...} which must be "+
					"deserialized as a map on the Go side", result)

			html, htmlOk := m["html"].(string)
			Expect(htmlOk).To(BeTrue(),
				"result map must contain an 'html' key with a string value — "+
					"the hook concatenated metadata into the html field")
			Expect(html).To(ContainSubstring("<h1>Hello</h1>"),
				"original HTML must be preserved in the result")
			Expect(html).To(ContainSubstring("url:/test/"),
				"hook must receive the url field from HookRenderedPayload — "+
					"if missing, the fast path failed to set the url property")
			Expect(html).To(ContainSubstring("path:content/test.md"),
				"hook must receive the path field from HookRenderedPayload — "+
					"if missing, the fast path failed to set the path property")
			Expect(html).To(ContainSubstring("title:Test Page"),
				"hook must receive frontMatter.title from HookRenderedPayload — "+
					"if missing, the fast path failed to serialize frontMatter")
		})

		It("HookRenderedPayload preserves HTML with JSON-special characters", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html };
  });
}`)
			// HTML containing characters that require JSON escaping:
			// double quotes, backslashes, newlines, tabs, angle brackets,
			// unicode, null bytes in attribute values, script tags
			specialHTML := "<div class=\"test\" data-attr='val'>\n\t" +
				"<script>var x = \"hello\\nworld\";</script>\n" +
				"<p>Price: $100 & tax < $20 > $10</p>\n" +
				"<p>Unicode: café résumé naïve</p>\n" +
				"<p>Emoji: \U0001F600</p>"

			payload := plugin.HookRenderedPayload{
				HTML:        specialHTML,
				FrontMatter: map[string]interface{}{},
				URL:         "/special/",
				Path:        "content/special.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"result must be a map type (map[string]interface{} or *ordered.Map)")
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue(), "result must contain html as string")
			Expect(html).To(Equal(specialHTML),
				"HTML with JSON-special characters must survive the QuickJS "+
					"round-trip unchanged — if this fails, the fast path "+
					"is corrupting characters that would normally be escaped "+
					"by JSON.parse/JSON.stringify (quotes, backslashes, "+
					"newlines, unicode codepoints)")
		})

		It("HookRenderedPayload with empty frontMatter delivers empty object to JS", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var keys = Object.keys(page.frontMatter);
    return { html: 'keys:' + keys.length + ' type:' + typeof page.frontMatter };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<p>test</p>",
				FrontMatter: map[string]interface{}{},
				URL:         "/empty-fm/",
				Path:        "content/empty.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html := m["html"].(string)
			Expect(html).To(Equal("keys:0 type:object"),
				"empty frontMatter must arrive as an empty JS object (not null, "+
					"not undefined) — plugins must be able to call Object.keys() "+
					"without TypeError. If this fails with 'type:undefined', the "+
					"fast path is not setting the frontMatter property")
		})

		It("HookRenderedPayload with nested frontMatter objects and arrays", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var fm = page.frontMatter;
    var tag0 = fm.tags[0];
    var tag1 = fm.tags[1];
    var authorName = fm.author.name;
    var authorAge = fm.author.age;
    var nested = fm.meta.deep.value;
    return {
      html: 'tags:' + tag0 + ',' + tag1 +
            ' author:' + authorName + '(' + authorAge + ')' +
            ' nested:' + nested
    };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML: "<p>nested test</p>",
				FrontMatter: map[string]interface{}{
					"tags": []interface{}{"go", "performance"},
					"author": map[string]interface{}{
						"name": "Alice",
						"age":  30,
					},
					"meta": map[string]interface{}{
						"deep": map[string]interface{}{
							"value": "found-it",
						},
					},
				},
				URL:  "/nested/",
				Path: "content/nested.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html := m["html"].(string)
			Expect(html).To(ContainSubstring("tags:go,performance"),
				"frontMatter arrays must be accessible in JS — "+
					"the fast path must JSON-serialize frontMatter so "+
					"nested arrays and objects are preserved")
			Expect(html).To(ContainSubstring("author:Alice(30)"),
				"nested frontMatter objects must be accessible in JS")
			Expect(html).To(ContainSubstring("nested:found-it"),
				"deeply nested frontMatter values must be accessible in JS")
		})

		It("HookRenderedPayload return with addDependencies is extracted correctly", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return {
      html: page.html + '<!-- ssr -->',
      addDependencies: ['elements/rh-card/rh-card.js', 'elements/rh-icon/rh-icon.js']
    };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<rh-card>content</rh-card>",
				FrontMatter: map[string]interface{}{},
				URL:         "/deps/",
				Path:        "content/deps.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())

			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(Equal("<rh-card>content</rh-card><!-- ssr -->"),
				"html must contain the hook's modifications")

			deps, ok := m["addDependencies"]
			Expect(ok).To(BeTrue(),
				"addDependencies key must be present in the result map — "+
					"the fast path's outbound extraction must preserve "+
					"non-html keys from the hook return value")
			depsSlice, ok := deps.([]interface{})
			Expect(ok).To(BeTrue(),
				"addDependencies must be a []interface{} — got %T; "+
					"the fast path must extract array values correctly", deps)
			Expect(depsSlice).To(HaveLen(2))
			Expect(depsSlice[0]).To(Equal("elements/rh-card/rh-card.js"))
			Expect(depsSlice[1]).To(Equal("elements/rh-icon/rh-icon.js"))
		})

		It("HookRenderedPayload identity return preserves pipeline-consumed fields", func() {
			// Issue #1185: targeted outbound extraction reads only the fields
			// the pipeline consumes from the return value. For onPageRendered,
			// the pipeline reads html and addDependencies. Read-only context
			// fields (url, path, frontMatter) are NOT extracted — they are
			// inbound-only (sent to JS for conditional logic, never read back).
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return page;
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<p>unchanged</p>",
				FrontMatter: map[string]interface{}{"title": "Identity"},
				URL:         "/identity/",
				Path:        "content/identity.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"identity return (return page) must produce a map result — "+
					"the hook echoes the input object back unchanged")
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(Equal("<p>unchanged</p>"),
				"identity return must preserve the html field exactly — "+
					"the targeted outbound extraction must handle the "+
					"case where the hook returns the input object unmodified")

			// url, path, and frontMatter are read-only context fields.
			// The targeted extractor does NOT extract them from the return
			// value because the pipeline never reads them back. Their
			// absence from the result map is correct behavior.
			_, hasURL := m["url"]
			Expect(hasURL).To(BeFalse(),
				"url must NOT be present in the result — it is a read-only "+
					"inbound field that the pipeline does not consume from "+
					"the return value (issue #1185 targeted extraction)")
			_, hasPath := m["path"]
			Expect(hasPath).To(BeFalse(),
				"path must NOT be present in the result — same as url, "+
					"it is read-only context for conditional processing")
			_, hasFM := m["frontMatter"]
			Expect(hasFM).To(BeFalse(),
				"frontMatter must NOT be present in the result — the pipeline "+
					"does not read frontMatter back from onPageRendered returns")
		})

		It("HookFormatRenderedPayload delivers all fields to JS and returns modified content", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(payload) {
    if (payload.format !== 'json') return payload;
    return {
      content: payload.content.replace(/\s+/g, '') +
        '/*fmt:' + payload.format +
        ' url:' + payload.url +
        ' path:' + payload.path +
        ' title:' + payload.frontMatter.title + '*/'
    };
  });
}`)
			payload := plugin.HookFormatRenderedPayload{
				Format:      "json",
				Content:     "{ \"key\": \"value\" }",
				URL:         "/api/data/",
				Path:        "content/api/data.md",
				FrontMatter: map[string]interface{}{"title": "API Data"},
			}
			result, err := rt.CallHook("onFormatRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"CallHook with HookFormatRenderedPayload must return a map")
			content, ok := m["content"].(string)
			Expect(ok).To(BeTrue(),
				"result must contain 'content' key as string")
			Expect(content).To(ContainSubstring(`{"key":"value"}`),
				"content must contain the minified JSON")
			Expect(content).To(ContainSubstring("fmt:json"),
				"hook must receive the format field")
			Expect(content).To(ContainSubstring("url:/api/data/"),
				"hook must receive the url field")
			Expect(content).To(ContainSubstring("path:content/api/data.md"),
				"hook must receive the path field")
			Expect(content).To(ContainSubstring("title:API Data"),
				"hook must receive frontMatter from the payload")
		})

		It("HookTransformPayload delivers all fields including toc to JS", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var tocInfo = 'no-toc';
    if (page.toc && page.toc.length > 0) {
      tocInfo = 'toc:' + page.toc[0].text + '(L' + page.toc[0].level + ')';
    }
    return {
      html: page.html + '<!-- ' + tocInfo +
        ' url:' + page.url +
        ' path:' + page.path +
        ' title:' + page.frontMatter.title + ' -->',
      toc: page.toc
    };
  });
}`)
			payload := plugin.HookTransformPayload{
				HTML: "<h2>Introduction</h2><p>Body text</p>",
				TOC: []content.TOCEntry{
					{Text: "Introduction", ID: "introduction", Level: 2},
				},
				URL:         "/transform/",
				Path:        "content/transform.md",
				FrontMatter: map[string]interface{}{"title": "Transform Test"},
			}
			result, err := rt.CallHook("onContentTransformed", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"CallHook with HookTransformPayload must return a map")
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(ContainSubstring("toc:Introduction(L2)"),
				"hook must receive the toc array from HookTransformPayload — "+
					"the fast path must serialize TOC entries so JS can "+
					"access .text and .level properties")
			Expect(html).To(ContainSubstring("url:/transform/"),
				"hook must receive the url field")
			Expect(html).To(ContainSubstring("path:content/transform.md"),
				"hook must receive the path field from HookTransformPayload")
			Expect(html).To(ContainSubstring("title:Transform Test"),
				"hook must receive frontMatter from the payload")

			// Verify toc in outbound result — the hook returns { html: ..., toc: page.toc }
			// and the fast path must preserve toc in the result map.
			tocRaw, tocExists := m["toc"]
			Expect(tocExists).To(BeTrue(),
				"toc must be present in the result map — the hook returns "+
					"{ html: ..., toc: page.toc } and the fast path's outbound "+
					"extraction must preserve non-html keys like toc")
			tocSlice, tocOk := tocRaw.([]interface{})
			Expect(tocOk).To(BeTrue(),
				"toc must be a []interface{} — got %T; the fast path must "+
					"extract array values correctly from the JS result", tocRaw)
			Expect(tocSlice).To(HaveLen(1),
				"toc must contain exactly 1 entry matching the input")
			tocEntry := extractGoMap(tocSlice[0])
			Expect(tocEntry).NotTo(BeNil())
			Expect(tocEntry["text"]).To(Equal("Introduction"),
				"toc entry text must be preserved through the round-trip")
		})

		It("HookRenderedPayload with nil frontMatter does not panic", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var fmType = typeof page.frontMatter;
    var isNull = page.frontMatter === null || page.frontMatter === undefined;
    var safe = 'none';
    if (!isNull && fmType === 'object') {
      safe = Object.keys(page.frontMatter).length.toString();
    }
    return { html: 'fm:' + fmType + ' null:' + isNull + ' keys:' + safe };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<p>nil fm</p>",
				FrontMatter: nil,
				URL:         "/nil-fm/",
				Path:        "content/nil-fm.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must not panic or error when FrontMatter is nil — "+
					"json.Marshal(nil) produces \"null\" which JSON.parse turns "+
					"into JS null; the fast path must nil-guard frontMatter")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			// The hook handles both null and object cases; either is acceptable
			// as long as it doesn't panic. The fast path should coerce nil to
			// empty object (matching pipeline behavior in buildPageRenderedPayload
			// which does `if fm == nil { fm = map[string]interface{}{} }`).
			Expect(html).NotTo(ContainSubstring("undefined"),
				"nil frontMatter must not arrive as JS undefined — "+
					"it should be null or an empty object")
		})

		It("HookTransformPayload with nil TOC delivers null or empty array to JS", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var tocInfo = 'null';
    if (page.toc !== null && page.toc !== undefined) {
      tocInfo = 'len:' + page.toc.length;
    }
    return { html: 'toc:' + tocInfo, toc: page.toc };
  });
}`)
			payload := plugin.HookTransformPayload{
				HTML:        "<p>no toc</p>",
				TOC:         nil,
				URL:         "/no-toc/",
				Path:        "content/no-toc.md",
				FrontMatter: map[string]interface{}{"title": "No TOC"},
			}
			result, err := rt.CallHook("onContentTransformed", payload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must not error when TOC is nil — "+
					"the omitempty JSON tag produces no toc field; "+
					"the fast path must handle nil TOC gracefully")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			// With omitempty, nil TOC is omitted from JSON → JS sees undefined.
			// With a fast path that explicitly sets toc, it could be null or [].
			// Either null/undefined (toc:null) or empty (toc:len:0) is acceptable.
			Expect(html).To(SatisfyAny(
				Equal("toc:null"),
				Equal("toc:len:0"),
			), "nil TOC must arrive as null/undefined or empty array in JS — "+
				"must not cause a TypeError or arrive as a non-empty value")
		})

		It("HookTransformPayload with empty TOC slice delivers empty array to JS", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var tocInfo = 'null';
    if (page.toc !== null && page.toc !== undefined) {
      tocInfo = 'len:' + page.toc.length;
    }
    return { html: 'toc:' + tocInfo };
  });
}`)
			payload := plugin.HookTransformPayload{
				HTML:        "<p>empty toc</p>",
				TOC:         []content.TOCEntry{},
				URL:         "/empty-toc/",
				Path:        "content/empty-toc.md",
				FrontMatter: map[string]interface{}{"title": "Empty TOC"},
			}
			result, err := rt.CallHook("onContentTransformed", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			// Empty slice with omitempty is omitted from JSON (same as nil).
			// The fast path may explicitly set it as []. Either is acceptable.
			Expect(html).To(SatisfyAny(
				Equal("toc:null"),
				Equal("toc:len:0"),
			), "empty TOC slice must arrive as null/undefined or empty array — "+
				"omitempty causes [] to serialize as absent, but the fast path "+
				"may set it explicitly; both behaviors are correct")
		})

		It("HookFormatRenderedPayload preserves content with JSON-special characters", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(payload) {
    return { content: payload.content };
  });
}`)
			specialContent := "{\"key\": \"value with \\\"quotes\\\" and\\nnewlines\", " +
				"\"html\": \"<p>angle & brackets</p>\", " +
				"\"unicode\": \"café résumé naïve\"}"

			payload := plugin.HookFormatRenderedPayload{
				Format:      "json",
				Content:     specialContent,
				URL:         "/special-fmt/",
				Path:        "content/special-fmt.md",
				FrontMatter: map[string]interface{}{},
			}
			result, err := rt.CallHook("onFormatRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			content, ok := m["content"].(string)
			Expect(ok).To(BeTrue())
			Expect(content).To(Equal(specialContent),
				"content with JSON-special characters must survive the QuickJS "+
					"round-trip unchanged — same requirement as HTML in "+
					"HookRenderedPayload but for the content field")
		})

		It("HookFormatRenderedPayload identity return preserves pipeline-consumed fields", func() {
			// Issue #1185: targeted outbound extraction reads only the fields
			// the pipeline consumes from the return value. For onFormatRendered,
			// the pipeline reads only content (and addDependencies if present).
			// Read-only context fields (format, url, path, frontMatter) are NOT
			// extracted — they are inbound-only.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(payload) {
    return payload;
  });
}`)
			payload := plugin.HookFormatRenderedPayload{
				Format:      "xml",
				Content:     "<root><item>data</item></root>",
				URL:         "/feed/",
				Path:        "content/feed.md",
				FrontMatter: map[string]interface{}{"title": "RSS Feed"},
			}
			result, err := rt.CallHook("onFormatRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"identity return from HookFormatRenderedPayload must produce a map")
			Expect(m["content"]).To(Equal("<root><item>data</item></root>"),
				"identity return must preserve content — the only mutable field "+
					"the pipeline reads from onFormatRendered returns")

			// format, url, path, and frontMatter are read-only context fields.
			// The targeted extractor does NOT extract them from the return value.
			_, hasFormat := m["format"]
			Expect(hasFormat).To(BeFalse(),
				"format must NOT be present in the result — it is a read-only "+
					"inbound field (issue #1185 targeted extraction)")
			_, hasURL := m["url"]
			Expect(hasURL).To(BeFalse(),
				"url must NOT be present in the result — read-only context field")
			_, hasPath := m["path"]
			Expect(hasPath).To(BeFalse(),
				"path must NOT be present in the result — read-only context field")
			_, hasFM := m["frontMatter"]
			Expect(hasFM).To(BeFalse(),
				"frontMatter must NOT be present in the result — the pipeline "+
					"does not read frontMatter back from onFormatRendered returns")
		})

		It("HookTransformPayload identity return preserves pipeline-consumed fields", func() {
			// Issue #1185: targeted outbound extraction reads only the fields
			// the pipeline consumes from the return value. For onContentTransformed,
			// the pipeline reads html, toc, and addDependencies. Read-only context
			// fields (url, path, frontMatter) are NOT extracted — they are
			// inbound-only (sent to JS for conditional logic, never read back).
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    return page;
  });
}`)
			payload := plugin.HookTransformPayload{
				HTML:        "<h2>Section</h2><p>Body text</p>",
				FrontMatter: map[string]interface{}{"title": "Transform Identity"},
				URL:         "/transform-id/",
				Path:        "content/transform-id.md",
				TOC: []content.TOCEntry{
					{ID: "section", Text: "Section", Level: 2},
				},
			}
			result, err := rt.CallHook("onContentTransformed", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"identity return from HookTransformPayload must produce a map")

			// html and toc ARE pipeline-consumed — they must be present.
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue(),
				"html must be present in the result — it is a mutable field "+
					"the pipeline reads from onContentTransformed returns")
			Expect(html).To(Equal("<h2>Section</h2><p>Body text</p>"),
				"identity return must preserve html exactly")

			_, hasTOC := m["toc"]
			Expect(hasTOC).To(BeTrue(),
				"toc must be present in the result — the pipeline reads toc "+
					"from onContentTransformed returns (unlike onPageRendered)")

			// url, path, and frontMatter are read-only context fields.
			// The targeted extractor does NOT extract them from the return value.
			_, hasURL := m["url"]
			Expect(hasURL).To(BeFalse(),
				"url must NOT be present in the result — read-only context field "+
					"(issue #1185 targeted extraction)")
			_, hasPath := m["path"]
			Expect(hasPath).To(BeFalse(),
				"path must NOT be present in the result — read-only context field")
			_, hasFM := m["frontMatter"]
			Expect(hasFM).To(BeFalse(),
				"frontMatter must NOT be present in the result — the pipeline "+
					"does not read frontMatter back from onContentTransformed returns")
		})

		It("HookRenderedPayload hook returning raw string is handled as backward compat", func() {
			// A hook registered for the object API that returns page.html
			// (a raw string) instead of { html: page.html } (an object).
			// The fast path must handle string returns without error.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return page.html + '<!-- string-return -->';
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<p>test</p>",
				FrontMatter: map[string]interface{}{},
				URL:         "/string-return/",
				Path:        "content/string-return.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must not error when hook returns a string instead "+
					"of an object — the fast path outbound extraction must "+
					"handle string results as a backward compat fallback")

			s, ok := result.(string)
			Expect(ok).To(BeTrue(),
				"a hook returning a raw string must produce a string result, "+
					"not a map — got %T", result)
			Expect(s).To(Equal("<p>test</p><!-- string-return -->"),
				"string return must contain the hook's modifications")
		})

		It("HookRenderedPayload with large HTML does not corrupt content", func() {
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html + '<!-- length:' + page.html.length + ' -->' };
  });
}`)
			// Build a ~100KB HTML string — large enough to stress the
			// serialization path but not so large it slows tests
			var largeHTML string
			for i := 0; i < 1000; i++ {
				largeHTML += "<div class=\"card\"><h3>Card " +
					fmt.Sprintf("%d", i) +
					"</h3><p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
					"Sed do eiusmod tempor incididunt ut labore.</p></div>\n"
			}
			expectedLen := len(largeHTML)

			payload := plugin.HookRenderedPayload{
				HTML:        largeHTML,
				FrontMatter: map[string]interface{}{},
				URL:         "/large/",
				Path:        "content/large.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(ContainSubstring(fmt.Sprintf("<!-- length:%d -->", expectedLen)),
				"JS must receive the correct string length for large HTML — "+
					"if the length differs, the fast path is truncating or "+
					"corrupting the HTML during native string transfer")
			Expect(html).To(HavePrefix("<div class=\"card\"><h3>Card 0</h3>"),
				"start of large HTML must be preserved")
			Expect(html).To(ContainSubstring("Card 999"),
				"end of large HTML must be preserved — "+
					"if missing, the fast path truncated the string")
		})
	})

	// ── Pre-compiled JS helpers (issue #1186) ──────────────────────────
	// QuickJS Init() must pre-compile static JS expressions as named
	// functions to avoid re-parsing and re-compiling the same code on
	// every hook invocation. Three functions must exist after Init():
	// __callHookByName (replaces Eval in invokeHookFastPath),
	// __installLazyFM (installs lazy getter for frontMatter property),
	// __installLazyTOC (installs lazy getter for toc property).
	// Lazy getters defer JSON.parse until the plugin accesses the property,
	// eliminating parse cost when the property is unused.

	Describe("Pre-compiled JS helpers (issue #1186)", func() {

		// checkGlobalIsFunction verifies that a named global JS function
		// exists after Init(). Used by the helper existence tests.
		checkGlobalIsFunction := func(name, failureMsg string) {
			tmpDir := GinkgoT().TempDir()
			pluginPath := filepath.Join(tmpDir, "check.js")
			pluginJS := fmt.Sprintf(`export default function(alloy) {
  alloy.filter('checkType', function() { return typeof %s; });
}`, name)
			Expect(os.WriteFile(pluginPath, []byte(pluginJS), 0644)).To(Succeed())

			rt := plugin.NewQuickJSRuntime()
			Expect(rt.Init()).To(Succeed())
			Expect(rt.EvalFile(pluginPath)).To(Succeed())
			DeferCleanup(rt.Close)

			result, err := rt.CallFilter("checkType", "ignored")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("function"), failureMsg)
		}

		Context("helper function existence", func() {
			It("Init() defines __callHookByName as a pre-compiled JS function", func() {
				checkGlobalIsFunction("__callHookByName",
					"Init() must define __callHookByName as a global JS function — "+
						"invokeHookFastPath must use ctx.Invoke(fn) with this pre-compiled "+
						"function instead of ctx.Eval() to avoid re-parsing the expression "+
						"on every hook call (issue #1186)")
			})

			It("Init() defines __installLazyFM as a pre-compiled JS function", func() {
				checkGlobalIsFunction("__installLazyFM",
					"Init() must define __installLazyFM as a global JS function — "+
						"setPayloadFrontMatter must use ctx.Invoke(fn) with this "+
						"pre-compiled function to install a lazy getter that defers "+
						"JSON.parse until the plugin accesses frontMatter (issue #1186)")
			})

			It("Init() defines __installLazyTOC as a pre-compiled JS function", func() {
				checkGlobalIsFunction("__installLazyTOC",
					"Init() must define __installLazyTOC as a global JS function — "+
						"callHookTransformPayload must use ctx.Invoke(fn) with this "+
						"pre-compiled function to install a lazy getter that defers "+
						"JSON.parse until the plugin accesses toc (issue #1186)")
			})
		})

		Context("lazy frontMatter getter", func() {
			It("HookRenderedPayload frontMatter is delivered via lazy getter", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var desc = Object.getOwnPropertyDescriptor(page, 'frontMatter');
    var isLazy = desc && typeof desc.get === 'function';
    return { html: 'lazy:' + isLazy };
  });
}`)
				payload := plugin.HookRenderedPayload{
					HTML:        "<p>test</p>",
					FrontMatter: map[string]interface{}{"title": "Lazy Test"},
					URL:         "/lazy-fm/",
					Path:        "content/lazy-fm.md",
				}
				result, err := rt.CallHook("onPageRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("lazy:true"),
					"frontMatter must be delivered as a lazy getter via "+
						"Object.defineProperty with get/set — __installLazyFM must "+
						"install an accessor property that defers JSON.parse until "+
						"first access. If 'lazy:false', the property is set eagerly "+
						"via SetPropertyStr/ParseJSON (issue #1186)")
			})

			It("HookFormatRenderedPayload frontMatter is delivered via lazy getter", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(payload) {
    var desc = Object.getOwnPropertyDescriptor(payload, 'frontMatter');
    var isLazy = desc && typeof desc.get === 'function';
    return { content: 'lazy:' + isLazy };
  });
}`)
				payload := plugin.HookFormatRenderedPayload{
					Format:      "json",
					Content:     "{}",
					URL:         "/lazy-fmt-fm/",
					Path:        "content/lazy-fmt-fm.md",
					FrontMatter: map[string]interface{}{"title": "Format Lazy FM"},
				}
				result, err := rt.CallHook("onFormatRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				content, ok := m["content"].(string)
				Expect(ok).To(BeTrue())
				Expect(content).To(Equal("lazy:true"),
					"frontMatter on HookFormatRenderedPayload must also use the "+
						"lazy getter — setPayloadFrontMatter is shared across all "+
						"three payload types. If this fails but onPageRendered passes, "+
						"the developer wired HookFormatRenderedPayload differently "+
						"(issue #1186)")
			})

			It("lazy frontMatter setter allows override and re-access", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var original = page.frontMatter.title;
    page.frontMatter = { title: 'overridden', extra: 42 };
    var overridden = page.frontMatter.title;
    var extra = page.frontMatter.extra;
    return {
      html: 'original:' + original + ' overridden:' + overridden + ' extra:' + extra
    };
  });
}`)
				payload := plugin.HookRenderedPayload{
					HTML:        "<p>test</p>",
					FrontMatter: map[string]interface{}{"title": "Original"},
					URL:         "/lazy-setter/",
					Path:        "content/lazy-setter.md",
				}
				result, err := rt.CallHook("onPageRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("original:Original overridden:overridden extra:42"),
					"lazy frontMatter setter must replace the lazy getter value — "+
						"setting page.frontMatter = {...} must override the deferred "+
						"JSON.parse result so subsequent reads return the new value, "+
						"not the original parsed JSON (issue #1186)")
			})

			It("lazy frontMatter setter handles null assignment without re-parsing", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var original = page.frontMatter.title;
    page.frontMatter = null;
    var afterNull = page.frontMatter;
    return {
      html: 'original:' + original + ' afterNull:' + afterNull
    };
  });
}`)
				payload := plugin.HookRenderedPayload{
					HTML:        "<p>test</p>",
					FrontMatter: map[string]interface{}{"title": "Before Null"},
					URL:         "/lazy-null-set/",
					Path:        "content/lazy-null-set.md",
				}
				result, err := rt.CallHook("onPageRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("original:Before Null afterNull:null"),
					"setting page.frontMatter = null must stick — the getter must "+
						"not re-trigger JSON.parse because the sentinel matched null. "+
						"Use a boolean flag or unique sentinel object instead of "+
						"_parsed === null to distinguish 'not yet parsed' from "+
						"'explicitly set to null' (issue #1186)")
			})

			It("lazy frontMatter property is enumerable", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var keys = Object.keys(page);
    var hasFM = keys.indexOf('frontMatter') >= 0;
    return { html: 'hasFM:' + hasFM };
  });
}`)
				payload := plugin.HookRenderedPayload{
					HTML:        "<p>test</p>",
					FrontMatter: map[string]interface{}{"title": "Enumerable"},
					URL:         "/lazy-enum/",
					Path:        "content/lazy-enum.md",
				}
				result, err := rt.CallHook("onPageRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("hasFM:true"),
					"lazy frontMatter must be enumerable — Object.keys(page) must "+
						"include 'frontMatter'. __installLazyFM must set enumerable: "+
						"true in the Object.defineProperty descriptor, otherwise "+
						"JSON.stringify(page) and Object.keys(page) silently drop "+
						"the frontMatter property (issue #1186)")
			})

			It("lazy frontMatter property is configurable", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var desc = Object.getOwnPropertyDescriptor(page, 'frontMatter');
    var isConfigurable = desc && desc.configurable === true;
    return { html: 'configurable:' + isConfigurable };
  });
}`)
				payload := plugin.HookRenderedPayload{
					HTML:        "<p>test</p>",
					FrontMatter: map[string]interface{}{"title": "Configurable"},
					URL:         "/lazy-config/",
					Path:        "content/lazy-config.md",
				}
				result, err := rt.CallHook("onPageRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("configurable:true"),
					"lazy frontMatter must be configurable — plugins that re-define "+
						"the property via Object.defineProperty must not throw. "+
						"__installLazyFM must set configurable: true (issue #1186)")
			})
		})

		Context("lazy toc getter", func() {
			It("HookTransformPayload toc is delivered via lazy getter", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var desc = Object.getOwnPropertyDescriptor(page, 'toc');
    var isLazy = desc && typeof desc.get === 'function';
    return { html: 'lazy:' + isLazy, toc: page.toc };
  });
}`)
				payload := plugin.HookTransformPayload{
					HTML: "<h2>Heading</h2>",
					TOC: []content.TOCEntry{
						{Text: "Heading", ID: "heading", Level: 2},
					},
					URL:         "/lazy-toc/",
					Path:        "content/lazy-toc.md",
					FrontMatter: map[string]interface{}{"title": "Lazy TOC"},
				}
				result, err := rt.CallHook("onContentTransformed", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("lazy:true"),
					"toc must be delivered as a lazy getter via "+
						"Object.defineProperty with get/set — __installLazyTOC must "+
						"install an accessor property that defers JSON.parse until "+
						"first access. If 'lazy:false', the property is set eagerly "+
						"via ParseJSON (issue #1186)")
			})

			It("lazy toc getter returns correct parsed values on access", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var tocLen = page.toc ? page.toc.length : 0;
    var firstText = tocLen > 0 ? page.toc[0].text : 'none';
    var firstLevel = tocLen > 0 ? page.toc[0].level : 0;
    return {
      html: 'len:' + tocLen + ' text:' + firstText + ' level:' + firstLevel,
      toc: page.toc
    };
  });
}`)
				payload := plugin.HookTransformPayload{
					HTML: "<h2>Setup</h2><h3>Install</h3>",
					TOC: []content.TOCEntry{
						{Text: "Setup", ID: "setup", Level: 2},
						{Text: "Install", ID: "install", Level: 3},
					},
					URL:         "/lazy-toc-vals/",
					Path:        "content/lazy-toc-vals.md",
					FrontMatter: map[string]interface{}{"title": "TOC Values"},
				}
				result, err := rt.CallHook("onContentTransformed", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("len:2 text:Setup level:2"),
					"lazy toc getter must return correctly parsed JSON array — "+
						"the getter triggers JSON.parse on first access and returns "+
						"the parsed TOC array with text, id, and level properties "+
						"intact (issue #1186)")
			})

			It("lazy toc setter allows override", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var originalLen = page.toc.length;
    page.toc = [{ text: 'Custom', id: 'custom', level: 2 }];
    var newLen = page.toc.length;
    var newText = page.toc[0].text;
    return {
      html: 'origLen:' + originalLen + ' newLen:' + newLen + ' text:' + newText,
      toc: page.toc
    };
  });
}`)
				payload := plugin.HookTransformPayload{
					HTML: "<h2>Original</h2>",
					TOC: []content.TOCEntry{
						{Text: "Original", ID: "original", Level: 2},
						{Text: "Second", ID: "second", Level: 2},
					},
					URL:         "/lazy-toc-setter/",
					Path:        "content/lazy-toc-setter.md",
					FrontMatter: map[string]interface{}{"title": "TOC Setter"},
				}
				result, err := rt.CallHook("onContentTransformed", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("origLen:2 newLen:1 text:Custom"),
					"lazy toc setter must replace the lazy getter value — "+
						"setting page.toc = [...] must override the deferred parse "+
						"so subsequent reads return the new array (issue #1186)")
			})

			It("lazy toc property is enumerable", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var keys = Object.keys(page);
    var hasTOC = keys.indexOf('toc') >= 0;
    return { html: 'hasTOC:' + hasTOC, toc: page.toc };
  });
}`)
				payload := plugin.HookTransformPayload{
					HTML: "<h2>Heading</h2>",
					TOC: []content.TOCEntry{
						{Text: "Heading", ID: "heading", Level: 2},
					},
					URL:         "/lazy-toc-enum/",
					Path:        "content/lazy-toc-enum.md",
					FrontMatter: map[string]interface{}{"title": "TOC Enum"},
				}
				result, err := rt.CallHook("onContentTransformed", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("hasTOC:true"),
					"lazy toc must be enumerable — Object.keys(page) must "+
						"include 'toc'. __installLazyTOC must set enumerable: true "+
						"in the Object.defineProperty descriptor, otherwise "+
						"JSON.stringify(page) silently drops toc (issue #1186)")
			})
		})

		Context("dispatch mechanism", func() {
			It("invokeHookFastPath does not set __callInput/__callHookName globals", func() {
				rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var hasCallInput = typeof __callInput !== 'undefined';
    var hasCallHookName = typeof __callHookName !== 'undefined';
    return { html: 'globals:' + hasCallInput + ',' + hasCallHookName };
  });
}`)
				payload := plugin.HookRenderedPayload{
					HTML:        "<p>test</p>",
					FrontMatter: map[string]interface{}{},
					URL:         "/no-globals/",
					Path:        "content/no-globals.md",
				}
				result, err := rt.CallHook("onPageRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal("globals:false,false"),
					"invokeHookFastPath must use ctx.Invoke(callHookByNameFn, ...) "+
						"which passes name and input as function arguments — the old "+
						"__callInput and __callHookName globals must not be set. "+
						"If 'globals:true,true', invokeHookFastPath still uses "+
						"ctx.Eval() with global variables (issue #1186)")
			})
		})
	})

	Describe("QuickJS outbound fast path and lazy fields (issue #1185)", func() {

		// --- Lazy frontMatter via Object.defineProperty getter ---

		It("lazy frontMatter: plugin reading page.frontMatter.title receives correct data", func() {
			// Issue #1185: frontMatter is installed as a lazy getter via
			// Object.defineProperty. The Go side stores the map in pendingFM
			// and only marshals+parses when JS actually reads the property.
			// This test verifies the lazy getter delivers correct data.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return {
      html: page.html + '<!-- title:' + page.frontMatter.title +
        ' count:' + page.frontMatter.count + ' -->'
    };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<p>content</p>",
				FrontMatter: map[string]interface{}{"title": "Lazy FM", "count": 42},
				URL:         "/lazy-fm/",
				Path:        "content/lazy-fm.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(ContainSubstring("title:Lazy FM"),
				"lazy frontMatter getter must deliver the title field — "+
					"the Go callback must marshal frontMatter on first access "+
					"and the parsed JS object must have correct string values")
			Expect(html).To(ContainSubstring("count:42"),
				"lazy frontMatter getter must deliver numeric fields — "+
					"JSON parse inside the getter must preserve number types")
		})

		It("lazy frontMatter: plugin writing page.frontMatter before reading does not error", func() {
			// Issue #1185: the lazy getter installs a setter (no-op that
			// replaces the accessor with a data property) so plugins that
			// write page.frontMatter = {...} before reading don't throw
			// TypeError ("Cannot set property frontMatter of #<Object>
			// which has only a getter").
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    page.frontMatter = { title: 'Overwritten' };
    return {
      html: page.html + '<!-- title:' + page.frontMatter.title + ' -->'
    };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<p>content</p>",
				FrontMatter: map[string]interface{}{"title": "Original"},
				URL:         "/lazy-fm-write/",
				Path:        "content/lazy-fm-write.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred(),
				"writing page.frontMatter before reading must not throw — "+
					"the lazy getter's setter must replace the accessor with "+
					"a data property so the assignment succeeds")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(ContainSubstring("title:Overwritten"),
				"after writing page.frontMatter = {...}, subsequent reads "+
					"must see the overwritten value, not the original Go data")
		})

		It("lazy frontMatter: plugin that never reads frontMatter does not trigger errors", func() {
			// Issue #1185: when no plugin reads page.frontMatter, the lazy
			// getter's Go callback is never invoked. No marshal, no parse,
			// no errors. The hook must complete successfully.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html + '<!-- no-fm-access -->' };
  });
}`)
			payload := plugin.HookRenderedPayload{
				HTML:        "<p>content</p>",
				FrontMatter: map[string]interface{}{"title": "Untouched"},
				URL:         "/lazy-fm-skip/",
				Path:        "content/lazy-fm-skip.md",
			}
			result, err := rt.CallHook("onPageRendered", payload)
			Expect(err).NotTo(HaveOccurred(),
				"hook that never reads page.frontMatter must not error — "+
					"lazy getter must not eagerly materialize or throw")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(Equal("<p>content</p><!-- no-fm-access -->"),
				"hook output must be correct when frontMatter is never accessed")
		})

		// --- Lazy TOC via same Object.defineProperty getter pattern ---

		It("lazy TOC: plugin reading page.toc receives correct entries", func() {
			// Issue #1185: TOC uses the same lazy getter pattern as frontMatter.
			// Marshal+parse only when JS reads page.toc.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    var tocInfo = 'none';
    if (page.toc && page.toc.length > 0) {
      tocInfo = page.toc[0].text + ':L' + page.toc[0].level +
        ':' + page.toc[0].id;
    }
    return {
      html: page.html + '<!-- toc:' + tocInfo + ' -->',
      toc: page.toc
    };
  });
}`)
			payload := plugin.HookTransformPayload{
				HTML: "<h2>Getting Started</h2><p>Body</p>",
				TOC: []content.TOCEntry{
					{Text: "Getting Started", ID: "getting-started", Level: 2},
				},
				URL:         "/lazy-toc/",
				Path:        "content/lazy-toc.md",
				FrontMatter: map[string]interface{}{"title": "Lazy TOC"},
			}
			result, err := rt.CallHook("onContentTransformed", payload)
			Expect(err).NotTo(HaveOccurred())

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(ContainSubstring("toc:Getting Started:L2:getting-started"),
				"lazy TOC getter must deliver correct TOCEntry fields — "+
					"text, level, and id must all survive the lazy marshal+parse")
		})

		It("lazy TOC: plugin writing page.toc before reading does not error", func() {
			// Issue #1185: same setter pattern as frontMatter — writing
			// page.toc = [...] before reading must not throw TypeError.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    page.toc = [{ text: 'Custom', id: 'custom', level: 1 }];
    return {
      html: page.html + '<!-- custom-toc:' + page.toc[0].text + ' -->',
      toc: page.toc
    };
  });
}`)
			payload := plugin.HookTransformPayload{
				HTML: "<h2>Original</h2>",
				TOC: []content.TOCEntry{
					{Text: "Original", ID: "original", Level: 2},
				},
				URL:         "/lazy-toc-write/",
				Path:        "content/lazy-toc-write.md",
				FrontMatter: map[string]interface{}{},
			}
			result, err := rt.CallHook("onContentTransformed", payload)
			Expect(err).NotTo(HaveOccurred(),
				"writing page.toc = [...] before reading must not throw — "+
					"the lazy getter's setter must replace the accessor with "+
					"a data property so the assignment succeeds")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(ContainSubstring("custom-toc:Custom"),
				"after writing page.toc = [...], subsequent reads must see "+
					"the overwritten value, not the original Go TOC data")
		})

		It("lazy TOC: plugin that never reads toc does not trigger errors", func() {
			// Issue #1185: when no plugin reads page.toc, the lazy getter's
			// Go callback is never invoked. No marshal, no parse, no errors.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    return { html: page.html + '<!-- no-toc-access -->' };
  });
}`)
			payload := plugin.HookTransformPayload{
				HTML: "<h2>Heading</h2><p>Text</p>",
				TOC: []content.TOCEntry{
					{Text: "Heading", ID: "heading", Level: 2},
				},
				URL:         "/lazy-toc-skip/",
				Path:        "content/lazy-toc-skip.md",
				FrontMatter: map[string]interface{}{},
			}
			result, err := rt.CallHook("onContentTransformed", payload)
			Expect(err).NotTo(HaveOccurred(),
				"hook that never reads page.toc must not error — "+
					"lazy getter must not eagerly materialize or throw")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(Equal("<h2>Heading</h2><p>Text</p><!-- no-toc-access -->"),
				"hook output must be correct when toc is never accessed")
		})

		// --- Hook chain: map[string]interface{} fast path ---

		It("hook chain: second hook receives html from first hook's map result", func() {
			// Issue #1185: when hooks chain (first hook returns map, second
			// hook receives that map as payload), CallHook must handle the
			// map[string]interface{} payload via the map fast path. The map
			// has an "html" key, indicating it's a page-like payload that
			// should build a JS object directly (same as HookRenderedPayload).
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html + '<!-- hook-applied -->' };
  });
}`)
			// Simulate what the hook registry does on a chain: the second
			// invocation receives a map[string]interface{} (the first hook's
			// result) instead of a typed HookRenderedPayload struct.
			chainPayload := map[string]interface{}{
				"html": "<p>from first hook</p>",
				"url":  "/chain/",
				"path": "content/chain.md",
			}
			result, err := rt.CallHook("onPageRendered", chainPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle map[string]interface{} with 'html' key — "+
					"this is the payload shape when hooks chain (second hook "+
					"receives first hook's map result)")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())
			Expect(html).To(Equal("<p>from first hook</p><!-- hook-applied -->"),
				"second hook in chain must receive the html from the first "+
					"hook's map result and apply its own transformation")
		})

		It("hook chain: second hook receives content from first hook's map result", func() {
			// Issue #1185: the map fast path is guarded by html OR content key
			// presence. This test uses a "content"-keyed map (the onFormatRendered
			// chain scenario) to verify the content key guard is implemented,
			// not just the html key guard.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(payload) {
    return { content: payload.content + '<!-- format-chain -->' };
  });
}`)
			chainPayload := map[string]interface{}{
				"content": `{"items":[1,2,3]}`,
				"format":  "json",
				"url":     "/feed.json",
				"path":    "content/feed.md",
			}
			result, err := rt.CallHook("onFormatRendered", chainPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle map[string]interface{} with 'content' key — "+
					"this is the payload shape when onFormatRendered hooks chain "+
					"(second hook receives first hook's map result)")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			content, ok := m["content"].(string)
			Expect(ok).To(BeTrue())
			Expect(content).To(Equal(`{"items":[1,2,3]}<!-- format-chain -->`),
				"second hook in chain must receive the content from the first "+
					"hook's map result — the map fast path must detect the "+
					"'content' key (not just 'html') as a page-like payload")
		})

		It("hook chain: non-page map payload passes through correctly", func() {
			// Issue #1185: the map fast path is guarded by html/content key
			// presence. Non-page maps (e.g., onBuildComplete payloads) do
			// NOT have html/content keys and must fall through to the JSON
			// serialization path. Verify correct behavior for both paths.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onBuildComplete', {}, function(data) {
    return {
      pageCount: data.pageCount,
      marker: 'processed'
    };
  });
}`)
			nonPageMap := map[string]interface{}{
				"pageCount": 42,
				"duration":  "1.5s",
				"outputDir": "_site",
			}
			result, err := rt.CallHook("onBuildComplete", nonPageMap)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle map[string]interface{} without html/content "+
					"keys — these maps must fall through to the JSON path, not "+
					"be caught by the page-like map fast path")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			marker, ok := m["marker"].(string)
			Expect(ok).To(BeTrue(),
				"non-page map result must be extractable as a map")
			Expect(marker).To(Equal("processed"),
				"hook must receive and return data from non-page maps correctly")
		})

		It("hook chain: onDataFetched map payload is NOT caught by page-like fast path", func() {
			// Issue #1188: the map[string]interface{} fast path is guarded by
			// html/content key presence. onDataFetched payloads are site-wide data
			// maps that should never use the page-like fast path — they use the
			// full JSON round-trip. This test specifically targets onDataFetched
			// (the issue's primary example of a non-page map payload) to verify
			// the guard doesn't accidentally catch data-shaped maps.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onDataFetched', { data: ["*"] }, function(data) {
    return {
      demos: (data.demos || []).concat([{ name: 'new-demo', slug: 'new' }]),
      navigation: data.navigation,
      injected: true
    };
  });
}`)
			dataPayload := map[string]interface{}{
				"demos": []interface{}{
					map[string]interface{}{"name": "button", "slug": "button"},
				},
				"navigation": []interface{}{
					map[string]interface{}{"title": "Home", "url": "/"},
				},
			}
			result, err := rt.CallHook("onDataFetched", dataPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle onDataFetched map payload without error — "+
					"the map has no html/content keys, so it must fall through "+
					"to the JSON serialization path (issue #1188)")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(),
				"onDataFetched result must be extractable as a map")
			injected, ok := m["injected"].(bool)
			Expect(ok).To(BeTrue(),
				"result must contain the 'injected' key set by the hook")
			Expect(injected).To(BeTrue(),
				"hook must have executed and returned injected: true")

			demos, ok := m["demos"].([]interface{})
			Expect(ok).To(BeTrue(),
				"result must contain 'demos' as an array — "+
					"if this fails, the data map was corrupted by the serialization path")
			Expect(demos).To(HaveLen(2),
				"demos array must contain original entry plus the concatenated new-demo — "+
					"proves the hook received and processed the full data payload correctly")
		})

		// --- Hook chain map: hook name dispatch (issue #1202) ---
		//
		// The map fast path must determine the payload type from the hook
		// name — NOT from the map's key contents. The key-based heuristic
		// (html + toc → payloadTransform) fails when a plugin returns
		// extra keys (e.g., onPageRendered returning {html, toc}).

		It("hook chain: onPageRendered map with toc key must use rendered extraction, not transform", func() {
			// Issue #1202: a plugin's onPageRendered hook returns {html, toc}.
			// When this map feeds the next hook in the chain, the key heuristic
			// sees html + toc and misclassifies as payloadTransform. The correct
			// behavior is payloadRendered (based on the hook name), which means
			// extractRenderedResult runs — it extracts html + addDependencies
			// but NOT toc. If the wrong extractor runs (extractTransformResult),
			// toc would appear in the result map.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    // Return both html and toc — a valid return from any hook.
    // The toc key must NOT trick the classifier on the next chain call.
    return { html: page.html + '<!-- rendered-hook -->', toc: page.toc };
  });
}`)
			// Simulate a chained payload: the map comes from a prior hook
			// (e.g., Node IPC) that returned both html and toc.
			chainPayload := map[string]interface{}{
				"html": "<p>original</p>",
				"toc":  []interface{}{map[string]interface{}{"level": 2, "text": "Heading"}},
				"url":  "/test/",
				"path": "content/test.md",
			}
			result, err := rt.CallHook("onPageRendered", chainPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle onPageRendered map with toc key without error")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(), "result must be a map")
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue(), "result must contain html as string")
			Expect(html).To(Equal("<p>original</p><!-- rendered-hook -->"),
				"hook must have modified html correctly")

			// Key assertion: toc must NOT be in the result. extractRenderedResult
			// only extracts html + addDependencies. If this fails, the classifier
			// used the key heuristic (html + toc → payloadTransform) instead of
			// the hook name (onPageRendered → payloadRendered).
			_, hasTOC := m["toc"]
			Expect(hasTOC).To(BeFalse(),
				"onPageRendered result must NOT contain toc — "+
					"extractRenderedResult only extracts html + addDependencies. "+
					"If toc is present, the map fast path used the key heuristic "+
					"(html + toc → payloadTransform) instead of the hook name "+
					"(onPageRendered → payloadRendered) — issue #1202")
		})

		It("hook chain: onContentTransformed map without toc key must use transform extraction", func() {
			// Issue #1202 (reverse case): an onContentTransformed hook receives
			// a chained map that has html but no toc key (e.g., prior hook
			// stripped toc or never returned it). The key heuristic sees html
			// without toc and classifies as payloadRendered. The correct
			// behavior is payloadTransform (based on the hook name), which
			// means extractTransformResult runs — it extracts html + toc +
			// addDependencies. If the wrong extractor runs (extractRenderedResult),
			// the toc field returned by this hook would be dropped.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    // Return html + toc. The toc must survive extraction because
    // this is onContentTransformed (payloadTransform), not onPageRendered.
    return {
      html: page.html + '<!-- transform-hook -->',
      toc: [{ level: 2, text: 'Generated Heading' }]
    };
  });
}`)
			// Simulate a chained payload: map has html but NO toc key.
			// Under the key heuristic, this maps to payloadRendered (WRONG).
			// Under hook name dispatch, this maps to payloadTransform (CORRECT).
			chainPayload := map[string]interface{}{
				"html": "<h2>Heading</h2><p>Text</p>",
				"url":  "/test/",
				"path": "content/test.md",
			}
			result, err := rt.CallHook("onContentTransformed", chainPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle onContentTransformed map without toc key without error")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(), "result must be a map")
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue(), "result must contain html as string")
			Expect(html).To(Equal("<h2>Heading</h2><p>Text</p><!-- transform-hook -->"),
				"hook must have modified html correctly")

			// Key assertion: toc MUST be in the result. extractTransformResult
			// extracts html + toc + addDependencies. If this fails, the classifier
			// used the key heuristic (html without toc → payloadRendered) instead
			// of the hook name (onContentTransformed → payloadTransform).
			toc, hasTOC := m["toc"]
			Expect(hasTOC).To(BeTrue(),
				"onContentTransformed result MUST contain toc — "+
					"extractTransformResult extracts html + toc + addDependencies. "+
					"If toc is missing, the map fast path used the key heuristic "+
					"(html without toc → payloadRendered) instead of the hook name "+
					"(onContentTransformed → payloadTransform) — issue #1202")

			tocSlice, ok := toc.([]interface{})
			Expect(ok).To(BeTrue(), "toc must be an array")
			Expect(tocSlice).To(HaveLen(1), "toc must contain the single heading entry")
		})

		It("hook chain: onFormatRendered map with html key must use format-rendered extraction, not rendered", func() {
			// Issue #1211: a plugin's onFormatRendered hook receives a chained
			// map with both html and content keys. The key heuristic sees html
			// first (wasm.go:494) and classifies as payloadRendered. The correct
			// behavior is payloadFormatRendered (based on the hook name), which
			// means extractFormatRenderedResult runs — it extracts content +
			// addDependencies. If the wrong extractor runs (extractRenderedResult),
			// the content field would be lost from the result.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', {}, function(page) {
    // Return both content and html. The html key must NOT trick the
    // classifier — onFormatRendered uses payloadFormatRendered.
    return {
      content: page.content + '<!-- format-hook -->',
      html: '<p>stale html</p>'
    };
  });
}`)
			// Simulate a chained payload: the map has both html and content.
			// Under the key heuristic, hasHTML fires first → payloadRendered (WRONG).
			// Under hook name dispatch, onFormatRendered → payloadFormatRendered (CORRECT).
			chainPayload := map[string]interface{}{
				"content": "<article>Original</article>",
				"html":    "<p>leftover html from prior hook</p>",
				"url":     "/test/",
				"path":    "content/test.md",
			}
			result, err := rt.CallHook("onFormatRendered", chainPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle onFormatRendered map with html key without error")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(), "result must be a map")

			// Key assertion: content MUST be in the result. extractFormatRenderedResult
			// extracts content + addDependencies. If this fails, the classifier
			// used the key heuristic (html present → payloadRendered) instead of
			// the hook name (onFormatRendered → payloadFormatRendered).
			content, hasContent := m["content"].(string)
			Expect(hasContent).To(BeTrue(),
				"onFormatRendered result MUST contain content — "+
					"extractFormatRenderedResult extracts content + addDependencies. "+
					"If content is missing, the map fast path used the key heuristic "+
					"(html present → payloadRendered) instead of the hook name "+
					"(onFormatRendered → payloadFormatRendered) — issue #1211")
			Expect(content).To(Equal("<article>Original</article><!-- format-hook -->"),
				"hook must have modified content correctly")

			// html must NOT be in the result — extractFormatRenderedResult does
			// not extract html (only content + addDependencies).
			_, hasHTML := m["html"]
			Expect(hasHTML).To(BeFalse(),
				"onFormatRendered result must NOT contain html — "+
					"extractFormatRenderedResult only extracts content + addDependencies. "+
					"If html is present, the wrong extractor ran — issue #1211")
		})

		It("hook chain: onContentTransformed map with content key but no html must use transform extraction, not format-rendered", func() {
			// Issue #1212: a chained onContentTransformed map has a content key
			// but no html key (e.g., a prior hook returned only content and toc).
			// The key heuristic misses hasHTML (no html key), falls through to
			// hasContent (wasm.go:502), and classifies as payloadFormatRendered.
			// The correct behavior is payloadTransform (based on the hook name),
			// which means extractTransformResult runs — it extracts html + toc +
			// addDependencies. If the wrong extractor runs (extractFormatRenderedResult),
			// the toc field returned by the hook would be dropped.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    // Return html + toc from the transformation. The content key in the
    // INPUT must not cause misclassification of the OUTPUT extraction.
    return {
      html: '<h2>Rebuilt</h2><p>' + page.content + '</p>',
      toc: [{ level: 2, text: 'Rebuilt' }]
    };
  });
}`)
			// Simulate a chained payload: map has content but NO html key.
			// Under the key heuristic, hasHTML is false, hasContent is true →
			// payloadFormatRendered (WRONG).
			// Under hook name dispatch, onContentTransformed → payloadTransform (CORRECT).
			chainPayload := map[string]interface{}{
				"content": "raw markdown content",
				"url":     "/test/",
				"path":    "content/test.md",
			}
			result, err := rt.CallHook("onContentTransformed", chainPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must handle onContentTransformed map with content key without error")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil(), "result must be a map")

			// Key assertion: toc MUST be in the result. extractTransformResult
			// extracts html + toc + addDependencies. If this fails, the classifier
			// used the key heuristic (content present → payloadFormatRendered)
			// instead of the hook name (onContentTransformed → payloadTransform).
			toc, hasTOC := m["toc"]
			Expect(hasTOC).To(BeTrue(),
				"onContentTransformed result MUST contain toc — "+
					"extractTransformResult extracts html + toc + addDependencies. "+
					"If toc is missing, the map fast path used the key heuristic "+
					"(content present, no html → payloadFormatRendered) instead of "+
					"the hook name (onContentTransformed → payloadTransform) — issue #1212")
			tocSlice, ok := toc.([]interface{})
			Expect(ok).To(BeTrue(), "toc must be an array")
			Expect(tocSlice).To(HaveLen(1), "toc must contain the single heading entry")

			html, hasHTML := m["html"].(string)
			Expect(hasHTML).To(BeTrue(), "result must contain html as string")
			Expect(html).To(Equal("<h2>Rebuilt</h2><p>raw markdown content</p>"),
				"hook must have built html from the content input correctly")
		})

		// --- BatchCallHook on QuickJSRuntime ---

		It("BatchCallHook results match sequential CallHook calls", func() {
			// Issue #1185: QuickJSRuntime.BatchCallHook loops synchronously
			// without per-item goroutine/channel/context overhead. Results
			// must be identical to calling CallHook sequentially.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html + '<!-- transformed -->' };
  });
}`)
			payloads := []interface{}{
				plugin.HookRenderedPayload{
					HTML:        "<p>page one</p>",
					FrontMatter: map[string]interface{}{},
					URL:         "/one/",
					Path:        "content/one.md",
				},
				plugin.HookRenderedPayload{
					HTML:        "<p>page two</p>",
					FrontMatter: map[string]interface{}{},
					URL:         "/two/",
					Path:        "content/two.md",
				},
				plugin.HookRenderedPayload{
					HTML:        "<p>page three</p>",
					FrontMatter: map[string]interface{}{},
					URL:         "/three/",
					Path:        "content/three.md",
				},
			}

			// Sequential CallHook — the reference results
			seqResults := make([]interface{}, len(payloads))
			for i, p := range payloads {
				r, err := rt.CallHook("onPageRendered", p)
				Expect(err).NotTo(HaveOccurred())
				seqResults[i] = r
			}

			// BatchCallHook — must produce identical results
			batchResults, err := rt.BatchCallHook("onPageRendered", payloads, nil)
			Expect(err).NotTo(HaveOccurred(),
				"BatchCallHook must not error for valid payloads")
			Expect(batchResults).To(HaveLen(len(payloads)),
				"BatchCallHook must return one result per payload")

			for i, batchResult := range batchResults {
				bm := extractGoMap(batchResult)
				sm := extractGoMap(seqResults[i])
				Expect(bm).NotTo(BeNil())
				Expect(sm).NotTo(BeNil())

				bHTML, bOk := bm["html"].(string)
				sHTML, sOk := sm["html"].(string)
				Expect(bOk).To(BeTrue())
				Expect(sOk).To(BeTrue())
				Expect(bHTML).To(Equal(sHTML),
					fmt.Sprintf("BatchCallHook result[%d] must match sequential "+
						"CallHook result — both must produce identical html", i))
			}
		})

		It("BatchCallHook calls onProgress callback with 1-based indices", func() {
			// Issue #1185: onProgress is part of the BatchCallHook API per
			// IMPLEMENTATION.md. The callback receives 1-based item count
			// (i+1) after each item completes. A nil callback is the no-op
			// path (tested above); this test exercises the non-nil path.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html + '<!-- batch -->' };
  });
}`)
			payloads := []interface{}{
				plugin.HookRenderedPayload{
					HTML: "<p>a</p>", FrontMatter: map[string]interface{}{},
					URL: "/a/", Path: "content/a.md",
				},
				plugin.HookRenderedPayload{
					HTML: "<p>b</p>", FrontMatter: map[string]interface{}{},
					URL: "/b/", Path: "content/b.md",
				},
				plugin.HookRenderedPayload{
					HTML: "<p>c</p>", FrontMatter: map[string]interface{}{},
					URL: "/c/", Path: "content/c.md",
				},
			}

			var progressIndices []int
			onProgress := func(i int) {
				progressIndices = append(progressIndices, i)
			}

			results, err := rt.BatchCallHook("onPageRendered", payloads, onProgress)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(3))
			Expect(progressIndices).To(Equal([]int{1, 2, 3}),
				"onProgress must be called with 1-based indices [1, 2, 3] — "+
					"the callback receives i+1 after each item completes")
		})

		It("BatchCallHook with empty payloads returns empty slice", func() {
			// Edge case: empty input must return an empty (non-nil) slice
			// and nil error, without invoking any JS.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html + '<!-- should-not-fire -->' };
  });
}`)
			results, err := rt.BatchCallHook("onPageRendered", []interface{}{}, nil)
			Expect(err).NotTo(HaveOccurred(),
				"BatchCallHook with empty payloads must not error")
			Expect(results).NotTo(BeNil(),
				"BatchCallHook must return a non-nil slice even for empty input")
			Expect(results).To(HaveLen(0),
				"BatchCallHook with empty payloads must return an empty slice")
		})

		It("BatchCallHook propagates errors with 0-based item index", func() {
			// Issue #1190: When a JS hook throws for a specific payload,
			// BatchCallHook must return an error that includes the 0-based
			// item index so callers can identify which payload failed.
			// The error must wrap the original error with fmt.Errorf("item %d: %w", i, err).
			rt := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    if (page.url === '/fail/') throw new Error('deliberate test error');
    return { html: page.html + '<!-- ok -->' };
  });
}`)
			payloads := []interface{}{
				plugin.HookRenderedPayload{
					HTML: "<p>ok</p>", FrontMatter: map[string]interface{}{},
					URL: "/ok/", Path: "content/ok.md",
				},
				plugin.HookRenderedPayload{
					HTML: "<p>fail</p>", FrontMatter: map[string]interface{}{},
					URL: "/fail/", Path: "content/fail.md",
				},
				plugin.HookRenderedPayload{
					HTML: "<p>never reached</p>", FrontMatter: map[string]interface{}{},
					URL: "/never/", Path: "content/never.md",
				},
			}

			_, err := rt.BatchCallHook("onPageRendered", payloads, nil)
			Expect(err).To(HaveOccurred(),
				"BatchCallHook must return an error when a JS hook throws")
			Expect(err.Error()).To(ContainSubstring("item 1"),
				"error must include the 0-based item index (1) identifying "+
					"which payload triggered the failure — without the index, "+
					"callers cannot diagnose which page caused the error (issue #1190)")
			Expect(err.Error()).To(ContainSubstring("deliberate test error"),
				"error must preserve the original JS error message so callers "+
					"can diagnose the root cause (issue #1190)")
			Expect(errors.Unwrap(err)).NotTo(BeNil(),
				"error must use %%w verb so callers can unwrap the original error "+
					"via errors.Is/errors.As — using %%s or %%v would produce identical "+
					"string output but break error chain traversal (issue #1190)")
		})

		// --- Pre-compiled JS hook invocation ---

		It("pre-compiled hook invocation produces correct results across multiple calls", func() {
			// Issue #1185: __callHookByName is a pre-compiled JS function
			// (compiled once during Init) that replaces per-call Eval of
			// "__hooks[__callHookName](__callInput)". This test verifies that
			// the hook invocation path produces correct results when called
			// repeatedly — the key property of pre-compiled functions.
			// NOTE: This is a baseline guard — the pre-compiled optimization
			// is purely internal with no observable behavioral difference.
			// It passes on the base branch (confirms current behavior) and
			// guards against regressions when the implementation changes the
			// invocation mechanism.
			rt := setupQuickJSWithHook(`export default function(alloy) {
  var callCount = 0;
  alloy.hook('onPageRendered', {}, function(page) {
    callCount++;
    return { html: page.html + '<!-- call:' + callCount + ' -->' };
  });
}`)
			// Call the same hook multiple times to exercise the pre-compiled
			// invocation path. Each call must produce a distinct result
			// (incrementing counter) to prove the function state is preserved.
			for i := 1; i <= 3; i++ {
				payload := plugin.HookRenderedPayload{
					HTML:        "<p>test</p>",
					FrontMatter: map[string]interface{}{},
					URL:         "/precompiled/",
					Path:        "content/precompiled.md",
				}
				result, err := rt.CallHook("onPageRendered", payload)
				Expect(err).NotTo(HaveOccurred())

				m := extractGoMap(result)
				Expect(m).NotTo(BeNil())
				html, ok := m["html"].(string)
				Expect(ok).To(BeTrue())
				Expect(html).To(Equal(fmt.Sprintf("<p>test</p><!-- call:%d -->", i)),
					fmt.Sprintf("call %d must produce correct result with "+
						"incrementing counter — pre-compiled JS function must "+
						"preserve closure state across invocations", i))
			}
		})
	})

	// ── Hook chain context preservation (issue #1216) ────────────────
	// When multiple hooks chain on the same event (different priorities),
	// the second hook must receive the full payload context (url, path,
	// frontMatter) — not just the mutable field (html/content). The
	// outbound fast path extractors strip context fields because the
	// pipeline doesn't consume them on return, but the chained hook
	// still needs them for conditional processing.

	Describe("Hook chain context preservation (issue #1216)", func() {
		// setupChainedHooks creates two QuickJS runtimes with hooks on the
		// same event at different priorities, registers both into a shared
		// HookRegistry via RegisterRuntime, and returns the registry.
		setupChainedHooks := func(hookAJS, hookBJS string) *plugin.HookRegistry {
			rtA := setupQuickJSWithHook(hookAJS)
			rtB := setupQuickJSWithHook(hookBJS)

			hooks := plugin.NewHookRegistry()
			reg := plugin.NewRegistry(GinkgoT().TempDir())
			plugin.RegisterRuntime(reg, rtA, "plugin-a", hooks)
			plugin.RegisterRuntime(reg, rtB, "plugin-b", hooks)

			return hooks
		}

		It("onPageRendered chain: second hook receives url, path, frontMatter", func() {
			// Issue #1216: extractRenderedResult strips url/path/frontMatter
			// from the first hook's result. The second hook receives only
			// { html } — context fields are lost. The fix must carry forward
			// url and path as strings and install a lazy frontMatter getter.
			hooks := setupChainedHooks(
				// Plugin A (priority 10): modifies html, strips context on return
				`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10 }, function(page) {
    return { html: page.html + '<!-- hook-a -->' };
  });
}`,
				// Plugin B (priority 20): reports received context in html
				`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 20 }, function(page) {
    var u = (typeof page.url !== 'undefined') ? page.url : 'MISSING';
    var p = (typeof page.path !== 'undefined') ? page.path : 'MISSING';
    var fm = (typeof page.frontMatter === 'object' && page.frontMatter !== null) ? 'PRESENT' : 'MISSING';
    return { html: page.html + '<!-- url:' + u + ' --><!-- path:' + p + ' --><!-- fm:' + fm + ' -->' };
  });
}`)

			payloads := []interface{}{
				plugin.HookRenderedPayload{
					HTML:        "<p>hello</p>",
					FrontMatter: map[string]interface{}{"title": "Test Page"},
					URL:         "/test/",
					Path:        "content/test.md",
				},
			}

			results, err := hooks.RunBatchWithProgress(plugin.OnPageRendered, payloads, nil)
			Expect(err).NotTo(HaveOccurred(),
				"chained onPageRendered hooks must not error")
			Expect(results).To(HaveLen(1))

			m := extractGoMap(results[0])
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())

			Expect(html).To(ContainSubstring("<!-- hook-a -->"),
				"first hook's html modification must be present in chain output")
			Expect(html).To(ContainSubstring("<!-- url:/test/ -->"),
				"second hook must receive url from original payload — "+
					"extractRenderedResult must preserve url through the chain (issue #1216)")
			Expect(html).To(ContainSubstring("<!-- path:content/test.md -->"),
				"second hook must receive path from original payload — "+
					"extractRenderedResult must preserve path through the chain (issue #1216)")
			Expect(html).To(ContainSubstring("<!-- fm:PRESENT -->"),
				"second hook must receive frontMatter from original payload — "+
					"the chaining layer must install a lazy frontMatter getter "+
					"so the next hook can access page.frontMatter (issue #1216)")
		})

		It("onContentTransformed chain: second hook receives url, path, frontMatter", func() {
			// Issue #1216: extractTransformResult strips url/path/frontMatter.
			// Same bug as onPageRendered but through RunWithTimeout chaining.
			hooks := setupChainedHooks(
				// Plugin A (priority 10): modifies html
				`export default function(alloy) {
  alloy.hook('onContentTransformed', { priority: 10 }, function(page) {
    return { html: page.html + '<!-- transform-a -->' };
  });
}`,
				// Plugin B (priority 20): reports received context in html
				`export default function(alloy) {
  alloy.hook('onContentTransformed', { priority: 20 }, function(page) {
    var u = (typeof page.url !== 'undefined') ? page.url : 'MISSING';
    var p = (typeof page.path !== 'undefined') ? page.path : 'MISSING';
    var fm = (typeof page.frontMatter === 'object' && page.frontMatter !== null) ? 'PRESENT' : 'MISSING';
    return { html: page.html + '<!-- url:' + u + ' --><!-- path:' + p + ' --><!-- fm:' + fm + ' -->' };
  });
}`)

			payload := plugin.HookTransformPayload{
				HTML:        "<p>content</p>",
				FrontMatter: map[string]interface{}{"category": "docs"},
				URL:         "/docs/intro/",
				Path:        "content/docs/intro.md",
			}

			result, err := hooks.RunWithTimeout(plugin.OnContentTransformed, payload)
			Expect(err).NotTo(HaveOccurred(),
				"chained onContentTransformed hooks must not error")

			m := extractGoMap(result)
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())

			Expect(html).To(ContainSubstring("<!-- transform-a -->"),
				"first hook's html modification must be present in chain output")
			Expect(html).To(ContainSubstring("<!-- url:/docs/intro/ -->"),
				"second hook must receive url from original payload — "+
					"extractTransformResult must preserve url through the chain (issue #1216)")
			Expect(html).To(ContainSubstring("<!-- path:content/docs/intro.md -->"),
				"second hook must receive path from original payload — "+
					"extractTransformResult must preserve path through the chain (issue #1216)")
			Expect(html).To(ContainSubstring("<!-- fm:PRESENT -->"),
				"second hook must receive frontMatter from original payload — "+
					"the chaining layer must install a lazy frontMatter getter "+
					"so the next hook can access page.frontMatter (issue #1216)")
		})

		It("onPageRendered chain: url mutation in hook return does not propagate (read-only)", func() {
			// Issue #1216: url and path are read-only context. If hook A
			// returns { html: "...", url: "/mutated/" }, the chaining layer
			// must carry forward the ORIGINAL url from the input payload,
			// not the hook's returned url. This prevents one plugin from
			// corrupting context for downstream plugins.
			hooks := setupChainedHooks(
				// Plugin A (priority 10): returns mutated url alongside html
				`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 10 }, function(page) {
    return { html: page.html + '<!-- hook-a -->', url: '/mutated/' };
  });
}`,
				// Plugin B (priority 20): reports the url it received
				`export default function(alloy) {
  alloy.hook('onPageRendered', { priority: 20 }, function(page) {
    var u = (typeof page.url !== 'undefined') ? page.url : 'MISSING';
    return { html: page.html + '<!-- received-url:' + u + ' -->' };
  });
}`)

			payloads := []interface{}{
				plugin.HookRenderedPayload{
					HTML:        "<p>read-only test</p>",
					FrontMatter: map[string]interface{}{},
					URL:         "/original/",
					Path:        "content/original.md",
				},
			}

			results, err := hooks.RunBatchWithProgress(plugin.OnPageRendered, payloads, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1))

			m := extractGoMap(results[0])
			Expect(m).NotTo(BeNil())
			html, ok := m["html"].(string)
			Expect(ok).To(BeTrue())

			Expect(html).To(ContainSubstring("<!-- received-url:/original/ -->"),
				"second hook must receive the ORIGINAL url (/original/), not the "+
					"mutated url (/mutated/) from hook A's return — url is read-only "+
					"context that reflects page state, not hook output (issue #1216)")
			Expect(html).NotTo(ContainSubstring("<!-- received-url:/mutated/ -->"),
				"mutated url from hook A must NOT propagate to hook B — "+
					"the chaining layer must use the original payload's url (issue #1216)")
		})

		It("onFormatRendered chain: second hook receives full context (regression guard)", func() {
			// Issue #1216: onFormatRendered is immune to the context-stripping
			// bug because the pipeline dispatches via RunEachWithTimeout with
			// a payloadFn that rebuilds a fresh HookFormatRenderedPayload
			// before each hook. This test verifies that behavior as a
			// regression guard — it must pass green on the current codebase.
			rtA := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', { priority: 10 }, function(payload) {
    return { content: payload.content + '<!-- format-a -->' };
  });
}`)
			rtB := setupQuickJSWithHook(`export default function(alloy) {
  alloy.hook('onFormatRendered', { priority: 20 }, function(payload) {
    var u = (typeof payload.url !== 'undefined') ? payload.url : 'MISSING';
    var p = (typeof payload.path !== 'undefined') ? payload.path : 'MISSING';
    var fm = (typeof payload.frontMatter === 'object' && payload.frontMatter !== null) ? 'PRESENT' : 'MISSING';
    return { content: payload.content + '<!-- url:' + u + ' --><!-- path:' + p + ' --><!-- fm:' + fm + ' -->' };
  });
}`)

			hooks := plugin.NewHookRegistry()
			reg := plugin.NewRegistry(GinkgoT().TempDir())
			plugin.RegisterRuntime(reg, rtA, "format-plugin-a", hooks)
			plugin.RegisterRuntime(reg, rtB, "format-plugin-b", hooks)

			currentContent := `{"items":[1,2,3]}`
			fm := map[string]interface{}{"title": "Feed"}

			err := hooks.RunEachWithTimeout(plugin.OnFormatRendered,
				func(_ int, _ *plugin.HookScope) interface{} {
					return plugin.HookFormatRenderedPayload{
						Format:      "json",
						Content:     currentContent,
						URL:         "/feed.json",
						Path:        "content/feed.md",
						FrontMatter: fm,
					}
				},
				func(_ int, _ *plugin.HookScope, result interface{}) error {
					if m := extractGoMap(result); m != nil {
						if c, ok := m["content"].(string); ok {
							currentContent = c
						}
					}
					return nil
				},
			)
			Expect(err).NotTo(HaveOccurred(),
				"chained onFormatRendered hooks must not error")

			Expect(currentContent).To(ContainSubstring("<!-- format-a -->"),
				"first hook's content modification must be present")
			Expect(currentContent).To(ContainSubstring("<!-- url:/feed.json -->"),
				"second hook must receive url — onFormatRendered rebuilds payload "+
					"per hook via payloadFn, so context is always fresh (issue #1216)")
			Expect(currentContent).To(ContainSubstring("<!-- path:content/feed.md -->"),
				"second hook must receive path from fresh payload (issue #1216)")
			Expect(currentContent).To(ContainSubstring("<!-- fm:PRESENT -->"),
				"second hook must receive frontMatter from fresh payload (issue #1216)")
		})
	})

})
