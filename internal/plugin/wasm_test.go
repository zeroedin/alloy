package plugin_test

import (
	"context"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("Tier 2 Plugin Runtime (WASM + QuickJS)", func() {

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

})
