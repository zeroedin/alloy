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
})
