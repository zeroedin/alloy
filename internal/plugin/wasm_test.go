package plugin_test

import (
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
		It("CallExport returns error when WASM function returns (0, 0)", func() {
			rt := plugin.NewWASMRuntime()
			Expect(rt.LoadModule(filepath.Join(testdataDir(), "single-files", "compiled.wasm"))).To(Succeed())

			// Simulate a WASM function that returns (0, 0) — this signals
			// an execution error per the ABI contract. CallExport must
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
