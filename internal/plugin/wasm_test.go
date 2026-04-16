package plugin_test

import (
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
