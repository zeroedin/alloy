//go:build !windows

package plugin_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/plugin"
)

var _ = Describe("NodeRuntime.Restart (issue #1153)", func() {

	// ── Happy path ──────────────────────────────────────────────────────

	Describe("happy path", func() {
		var tmpDir string
		var rt *plugin.NodeRuntime

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-restart-happy-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
			rt = plugin.NewNodeRuntime()
			rt.SetProjectRoot(tmpDir)
		})

		AfterEach(func() {
			if rt != nil {
				rt.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("stops old bridge, starts fresh bridge, re-evaluates plugins, and subsequent CallHook uses fresh bridge", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/restart-hook.js")
			Expect(err).NotTo(HaveOccurred())

			// Load the plugin — establishes bridge + eval
			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(rt.RegisteredHooks()).To(ContainElement("onBuildComplete"),
				"precondition: hook must be registered before restart")

			// Verify hook works before restart
			prePayload := map[string]interface{}{"phase": "before"}
			preResult, err := rt.CallHook("onBuildComplete", prePayload)
			Expect(err).NotTo(HaveOccurred())
			preMap, ok := preResult.(*ordered.Map)
			Expect(ok).To(BeTrue(), "hook result must be an *ordered.Map after rewrap")
			Expect(preMap.Get("restarted")).To(Equal(true),
				"precondition: hook must return restarted=true before restart")

			// Restart
			err = rt.Restart()
			Expect(err).NotTo(HaveOccurred(),
				"Restart must succeed when bridge is healthy and plugins are valid")

			// Verify hook works AFTER restart with a fresh bridge
			postPayload := map[string]interface{}{"phase": "after"}
			postResult, err := rt.CallHook("onBuildComplete", postPayload)
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must succeed after Restart — the fresh bridge "+
					"must have re-evaluated all pluginPaths")
			postMap, ok := postResult.(*ordered.Map)
			Expect(ok).To(BeTrue(), "hook result must be an *ordered.Map after rewrap")
			Expect(postMap.Get("restarted")).To(Equal(true),
				"hook must execute on the fresh bridge and return restarted=true")
			Expect(postMap.Get("phase")).To(Equal("after"),
				"hook must process the new payload, not return stale data from the old bridge")
		})
	})

	// ── Empty pluginPaths ───────────────────────────────────────────────

	Describe("empty pluginPaths", func() {
		It("returns nil without starting a new bridge when no plugins were loaded", func() {
			rt := plugin.NewNodeRuntime()
			DeferCleanup(func() { rt.Close() })

			// No EvalFile called — pluginPaths is empty, bridge is nil
			err := rt.Restart()
			Expect(err).NotTo(HaveOccurred(),
				"Restart with no pluginPaths must be a no-op that returns nil")

			// CallHook must be a safe no-op (returns payload, no panic)
			result, err := rt.CallHook("onBuildComplete", "passthrough")
			Expect(err).NotTo(HaveOccurred(),
				"CallHook on a runtime with no bridge must not panic")
			Expect(result).To(Equal("passthrough"),
				"CallHook on a nil bridge must return the payload unchanged — "+
					"this is the existing nil-guard behavior in CallHook")
		})
	})

	// ── Start() failure ─────────────────────────────────────────────────
	// When bridge.Start() fails during Restart, the bridge must remain nil
	// and subsequent CallHook calls must be safe no-ops (not panics).

	Describe("Start() failure leaves bridge nil", func() {
		var tmpDir string
		var rt *plugin.NodeRuntime

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-restart-start-fail-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
			rt = plugin.NewNodeRuntime()
			rt.SetProjectRoot(tmpDir)
		})

		AfterEach(func() {
			if rt != nil {
				rt.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("returns error and leaves bridge nil so subsequent CallHook is a safe no-op", func() {
			fixturePath, err := filepath.Abs("testdata/single-files/restart-hook.js")
			Expect(err).NotTo(HaveOccurred())

			// Load plugin to populate pluginPaths
			err = rt.EvalFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())

			// Sabotage: set pluginPaths to include a file that will cause
			// a bridge eval error, simulating what happens when the fresh
			// bridge can't re-eval. We use SetPluginPaths to replace paths
			// with one that points to a non-existent file (the bridge will
			// successfully Start, but Send will fail — covered by the next
			// test case). For this test, verify the contract: if Restart()
			// returns an error, CallHook must not panic.
			//
			// Force a re-eval error by injecting a path that throws during import
			plugin.SetPluginPaths(rt, []string{
				"/nonexistent/path/that/does/not/exist.js",
			})

			// Restart should fail because the non-existent path can't be eval'd
			err = rt.Restart()
			Expect(err).To(HaveOccurred(),
				"Restart must return an error when re-eval of a plugin path fails")

			// After failed restart, CallHook must be a safe no-op
			result, err := rt.CallHook("onBuildComplete", "safe-payload")
			Expect(err).NotTo(HaveOccurred(),
				"CallHook after a failed Restart must not panic — "+
					"bridge must be nil and the nil-guard returns payload")
			Expect(result).To(Equal("safe-payload"),
				"CallHook on nil bridge must return the payload unchanged")
		})
	})

	// ── Send() failure during re-eval ───────────────────────────────────
	// When the fresh bridge starts successfully but re-eval of a plugin
	// path fails (e.g., the plugin throws during import), Restart must
	// stop the bridge, nil it, and return the error.

	Describe("Send() failure during re-eval", func() {
		var tmpDir string
		var rt *plugin.NodeRuntime

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-restart-eval-fail-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
			rt = plugin.NewNodeRuntime()
			rt.SetProjectRoot(tmpDir)
		})

		AfterEach(func() {
			if rt != nil {
				rt.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("returns error, stops bridge, and leaves bridge nil when a plugin throws during re-eval", func() {
			// First, load a valid plugin to establish a bridge
			goodPath, err := filepath.Abs("testdata/single-files/restart-hook.js")
			Expect(err).NotTo(HaveOccurred())
			err = rt.EvalFile(goodPath)
			Expect(err).NotTo(HaveOccurred())

			// Inject a path that throws during module evaluation
			badPath, err := filepath.Abs("testdata/single-files/restart-eval-error.js")
			Expect(err).NotTo(HaveOccurred())
			plugin.SetPluginPaths(rt, []string{goodPath, badPath})

			// Restart: bridge starts fine, first plugin evals ok, second throws
			err = rt.Restart()
			Expect(err).To(HaveOccurred(),
				"Restart must return an error when a plugin throws during re-eval")
			Expect(err.Error()).To(SatisfyAll(
				ContainSubstring("restart-eval-error.js"),
			), "error must identify the failing plugin filename")

			// After failed restart, bridge must be nil → CallHook is safe no-op
			result, err := rt.CallHook("onBuildComplete", "safe-after-eval-fail")
			Expect(err).NotTo(HaveOccurred(),
				"CallHook after failed re-eval must not panic — bridge must be nil")
			Expect(result).To(Equal("safe-after-eval-fail"),
				"CallHook on nil bridge must return payload unchanged")
		})
	})
})

var _ = Describe("Registry.RestartNodeRuntimes (issue #1153)", func() {

	// ── Single NodeRuntime ──────────────────────────────────────────────

	Describe("single NodeRuntime", func() {
		var tmpDir string
		var registry *plugin.Registry

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-restart-registry-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
		})

		AfterEach(func() {
			if registry != nil {
				registry.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("successfully restarts the single Node runtime", func() {
			// Create a plugin dir with a single Node plugin
			pluginDir := filepath.Join(tmpDir, "plugins")
			Expect(os.MkdirAll(pluginDir, 0755)).To(Succeed())

			fixturePath, err := filepath.Abs("testdata/single-files/restart-hook.js")
			Expect(err).NotTo(HaveOccurred())
			fixtureContent, err := os.ReadFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(pluginDir, "restart-hook.js"), fixtureContent, 0644)).To(Succeed())

			registry = plugin.NewRegistry(pluginDir)
			registry.SetProjectRoot(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())
			Expect(registry.Plugins()).To(HaveLen(1))

			hooks := plugin.NewHookRegistry()
			warnings := registry.LoadPlugins(hooks)
			Expect(warnings).To(BeEmpty())

			// Verify hook works before restart
			runtimes := registry.Runtimes()
			Expect(runtimes).To(HaveLen(1))
			nr, ok := runtimes[0].(*plugin.NodeRuntime)
			Expect(ok).To(BeTrue(), "runtime must be a *NodeRuntime")

			preResult, err := nr.CallHook("onBuildComplete", map[string]interface{}{"phase": "pre"})
			Expect(err).NotTo(HaveOccurred())
			preMap, ok := preResult.(*ordered.Map)
			Expect(ok).To(BeTrue())
			Expect(preMap.Get("restarted")).To(Equal(true),
				"precondition: hook must work before restart")

			// RestartNodeRuntimes
			err = registry.RestartNodeRuntimes()
			Expect(err).NotTo(HaveOccurred(),
				"RestartNodeRuntimes must succeed for a healthy Node runtime")

			// Verify hook works after restart
			postResult, err := nr.CallHook("onBuildComplete", map[string]interface{}{"phase": "post"})
			Expect(err).NotTo(HaveOccurred(),
				"CallHook must succeed after RestartNodeRuntimes — "+
					"the Node runtime's bridge was replaced with a fresh one")
			postMap, ok := postResult.(*ordered.Map)
			Expect(ok).To(BeTrue())
			Expect(postMap.Get("restarted")).To(Equal(true),
				"hook must execute on the fresh bridge")
			Expect(postMap.Get("phase")).To(Equal("post"),
				"hook must process the new payload on the fresh bridge")
		})
	})

	// ── Mixed runtimes (QuickJS + Node) ─────────────────────────────────

	Describe("mixed runtimes", func() {
		var tmpDir string
		var registry *plugin.Registry

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-restart-mixed-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
		})

		AfterEach(func() {
			if registry != nil {
				registry.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("only restarts NodeRuntime, leaves QuickJS untouched", func() {
			pluginDir := filepath.Join(tmpDir, "plugins")
			Expect(os.MkdirAll(pluginDir, 0755)).To(Succeed())

			// QuickJS plugin (Tier 2 — no runtime = "node" export)
			qjsContent := `export default function(alloy) {
    alloy.filter("qjsUpper", (v) => String(v).toUpperCase());
}`
			Expect(os.WriteFile(filepath.Join(pluginDir, "alpha-qjs.js"), []byte(qjsContent), 0644)).To(Succeed())

			// Node plugin (Tier 3)
			fixturePath, err := filepath.Abs("testdata/single-files/restart-hook.js")
			Expect(err).NotTo(HaveOccurred())
			fixtureContent, err := os.ReadFile(fixturePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(pluginDir, "zulu-node.js"), fixtureContent, 0644)).To(Succeed())

			registry = plugin.NewRegistry(pluginDir)
			registry.SetProjectRoot(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())
			Expect(registry.Plugins()).To(HaveLen(2),
				"precondition: must discover both QuickJS and Node plugins")

			hooks := plugin.NewHookRegistry()
			warnings := registry.LoadPlugins(hooks)
			Expect(warnings).To(BeEmpty())

			runtimes := registry.Runtimes()
			Expect(runtimes).To(HaveLen(2))

			// Identify which runtime is which
			var qjsRT plugin.PluginFilterRuntime
			var nodeRT *plugin.NodeRuntime
			for _, rt := range runtimes {
				if nr, ok := rt.(*plugin.NodeRuntime); ok {
					nodeRT = nr
				} else {
					qjsRT = rt
				}
			}
			Expect(nodeRT).NotTo(BeNil(), "must have a NodeRuntime in runtimes")
			Expect(qjsRT).NotTo(BeNil(), "must have a non-Node runtime in runtimes")

			// Verify QuickJS filter works before
			qjsResult, err := qjsRT.CallFilter("qjsUpper", "hello")
			Expect(err).NotTo(HaveOccurred())
			Expect(qjsResult).To(Equal("HELLO"),
				"precondition: QuickJS filter must work before restart")

			// RestartNodeRuntimes
			err = registry.RestartNodeRuntimes()
			Expect(err).NotTo(HaveOccurred())

			// QuickJS filter must still work (not affected by restart)
			qjsResult2, err := qjsRT.CallFilter("qjsUpper", "world")
			Expect(err).NotTo(HaveOccurred())
			Expect(qjsResult2).To(Equal("WORLD"),
				"QuickJS runtime must not be affected by RestartNodeRuntimes — "+
					"type assertion to *NodeRuntime must skip non-Node runtimes")

			// Node hook must still work (was restarted with fresh bridge)
			postResult, err := nodeRT.CallHook("onBuildComplete", map[string]interface{}{"mixed": true})
			Expect(err).NotTo(HaveOccurred())
			postMap, ok := postResult.(*ordered.Map)
			Expect(ok).To(BeTrue())
			Expect(postMap.Get("restarted")).To(Equal(true),
				"Node hook must work after restart")
		})
	})

	// ── No NodeRuntimes ─────────────────────────────────────────────────

	Describe("no NodeRuntimes", func() {
		It("returns nil when registry has no Node runtimes", func() {
			tmpDir := GinkgoT().TempDir()
			pluginDir := filepath.Join(tmpDir, "plugins")
			Expect(os.MkdirAll(pluginDir, 0755)).To(Succeed())

			// Only a QuickJS plugin (no Node plugins)
			qjsContent := `export default function(alloy) {
    alloy.filter("qjsOnly", (v) => String(v).toUpperCase());
}`
			Expect(os.WriteFile(filepath.Join(pluginDir, "qjs-only.js"), []byte(qjsContent), 0644)).To(Succeed())

			registry := plugin.NewRegistry(pluginDir)
			registry.SetProjectRoot(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())

			hooks := plugin.NewHookRegistry()
			warnings := registry.LoadPlugins(hooks)
			Expect(warnings).To(BeEmpty())
			DeferCleanup(func() { registry.Close() })

			// Verify no NodeRuntimes exist
			hasNode := false
			for _, rt := range registry.Runtimes() {
				if _, ok := rt.(*plugin.NodeRuntime); ok {
					hasNode = true
				}
			}
			Expect(hasNode).To(BeFalse(),
				"precondition: registry must have no NodeRuntimes")

			// RestartNodeRuntimes must be a no-op
			err := registry.RestartNodeRuntimes()
			Expect(err).NotTo(HaveOccurred(),
				"RestartNodeRuntimes must return nil when no Node runtimes exist — "+
					"it's a no-op, not an error")
		})
	})

	// ── Empty registry ──────────────────────────────────────────────────

	Describe("empty registry", func() {
		It("returns nil when registry has no runtimes at all", func() {
			registry := plugin.NewRegistry(GinkgoT().TempDir())
			DeferCleanup(func() { registry.Close() })

			err := registry.RestartNodeRuntimes()
			Expect(err).NotTo(HaveOccurred(),
				"RestartNodeRuntimes on empty registry must return nil")
		})
	})

	// ── Error propagation with multiple NodeRuntimes ────────────────────

	Describe("error propagation with multiple NodeRuntimes", func() {
		var tmpDir string
		var registry *plugin.Registry

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "alloy-restart-multi-*")
			Expect(err).NotTo(HaveOccurred())
			plugin.ResetStalePIDCleanup(tmpDir)
		})

		AfterEach(func() {
			if registry != nil {
				registry.Close()
			}
			os.RemoveAll(tmpDir)
		})

		It("continues to attempt all runtimes and returns first error when one fails", func() {
			pluginDir := filepath.Join(tmpDir, "plugins")
			Expect(os.MkdirAll(pluginDir, 0755)).To(Succeed())

			// Plugin A: healthy Node plugin (will restart fine)
			goodPath, err := filepath.Abs("testdata/single-files/restart-hook.js")
			Expect(err).NotTo(HaveOccurred())
			goodContent, err := os.ReadFile(goodPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(pluginDir, "alpha-good.js"), goodContent, 0644)).To(Succeed())

			// Plugin B: another healthy Node plugin
			// Use a different name so it creates a second NodeRuntime
			good2Content := []byte(`export const runtime = "node";
export default function(alloy) {
    alloy.filter("betaFilter", (input) => String(input).toLowerCase());
}
`)
			Expect(os.WriteFile(filepath.Join(pluginDir, "beta-good.js"), good2Content, 0644)).To(Succeed())

			registry = plugin.NewRegistry(pluginDir)
			registry.SetProjectRoot(tmpDir)
			Expect(registry.DiscoverPlugins()).To(Succeed())
			plugins := registry.Plugins()
			Expect(plugins).To(HaveLen(2))

			hooks := plugin.NewHookRegistry()
			warnings := registry.LoadPlugins(hooks)
			Expect(warnings).To(BeEmpty())

			runtimes := registry.Runtimes()
			Expect(runtimes).To(HaveLen(2))

			// Count NodeRuntimes — both must be Node
			nodeCount := 0
			for _, rt := range runtimes {
				if _, ok := rt.(*plugin.NodeRuntime); ok {
					nodeCount++
				}
			}
			Expect(nodeCount).To(Equal(2),
				"precondition: both runtimes must be NodeRuntime")

			// Sabotage first NodeRuntime to make its Restart() fail by
			// injecting a bad plugin path
			firstNode := runtimes[0].(*plugin.NodeRuntime)
			badPath, err := filepath.Abs("testdata/single-files/restart-eval-error.js")
			Expect(err).NotTo(HaveOccurred())
			plugin.AppendPluginPath(firstNode, badPath)

			// RestartNodeRuntimes — first runtime should fail, second should succeed
			err = registry.RestartNodeRuntimes()
			Expect(err).To(HaveOccurred(),
				"RestartNodeRuntimes must return an error when any runtime fails")
			Expect(err.Error()).To(ContainSubstring("restart-eval-error.js"),
				"error must identify the failing plugin")

			// The second NodeRuntime must have been attempted despite the first failing
			secondNode := runtimes[1].(*plugin.NodeRuntime)
			result, err := secondNode.CallFilter("betaFilter", "HELLO")
			Expect(err).NotTo(HaveOccurred(),
				"second NodeRuntime must have been restarted successfully — "+
					"RestartNodeRuntimes must continue past the first error")
			Expect(result).To(Equal("hello"),
				"second NodeRuntime's filter must work after restart, "+
					"proving the loop continued past the first failure")
		})
	})
})
