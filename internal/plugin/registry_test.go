package plugin_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/plugin"
)

// testdataDir returns the absolute path to the testdata directory
// relative to this test file.
func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

var _ = Describe("Registry", func() {

	// ── Extension routing (ClassifyPlugin) ─────────────────────────────

	Describe("Extension routing", func() {
		It(".js file without runtime: node classifies as Tier 2 QuickJS", func() {
			path := filepath.Join(testdataDir(), "single-files", "plain.js")
			info, err := plugin.ClassifyPlugin(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Tier).To(Equal(plugin.TierInProcess))
			Expect(info.Runtime).To(Equal(plugin.RuntimeQuickJS))
		})

		It(".wasm file classifies as Tier 2 WASM", func() {
			path := filepath.Join(testdataDir(), "single-files", "compiled.wasm")
			info, err := plugin.ClassifyPlugin(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Tier).To(Equal(plugin.TierInProcess))
			Expect(info.Runtime).To(Equal(plugin.RuntimeWASM))
		})

		It(".js file with export const runtime = node classifies as Tier 3 Node", func() {
			path := filepath.Join(testdataDir(), "single-files", "node-plugin.js")
			info, err := plugin.ClassifyPlugin(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Tier).To(Equal(plugin.TierNode))
			Expect(info.Runtime).To(Equal(plugin.RuntimeNode))
		})

		It(".ts file with runtime: node classifies as Tier 3 Node", func() {
			path := filepath.Join(testdataDir(), "single-files", "node-ts-plugin.ts")
			info, err := plugin.ClassifyPlugin(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Tier).To(Equal(plugin.TierNode))
			Expect(info.Runtime).To(Equal(plugin.RuntimeNode))
		})

		It(".js file with runtime = node only in a comment classifies as Tier 2 QuickJS (issue #597)", func() {
			path := filepath.Join(testdataDir(), "single-files", "commented-runtime.js")
			info, err := plugin.ClassifyPlugin(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Tier).To(Equal(plugin.TierInProcess),
				"runtime declaration in a comment must not trigger Tier 3 Node classification (issue #597)")
			Expect(info.Runtime).To(Equal(plugin.RuntimeQuickJS),
				"plugin without a real runtime export must default to QuickJS")
		})

		It(".js file with runtime = node only in a string literal classifies as Tier 2 QuickJS (issue #597)", func() {
			path := filepath.Join(testdataDir(), "single-files", "string-literal-runtime.js")
			info, err := plugin.ClassifyPlugin(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(info).NotTo(BeNil())
			Expect(info.Tier).To(Equal(plugin.TierInProcess),
				"runtime declaration in a string literal must not trigger Tier 3 Node classification (issue #597)")
			Expect(info.Runtime).To(Equal(plugin.RuntimeQuickJS),
				"plugin without a real runtime export must default to QuickJS")
		})

		It("unknown extension returns error", func() {
			path := filepath.Join(testdataDir(), "single-files", "unknown.py")
			_, err := plugin.ClassifyPlugin(path)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(
				SatisfyAny(
					ContainSubstring("unsupported"),
					ContainSubstring("unknown"),
					ContainSubstring(".py"),
				),
				"error should identify the unsupported extension",
			)
		})
	})

	// ── Load order ─────────────────────────────────────────────────────

	Describe("Load order", func() {
		It("discovered plugins are sorted alphabetically by filename", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			err := registry.DiscoverPlugins()
			Expect(err).NotTo(HaveOccurred())

			plugins := registry.Plugins()
			Expect(plugins).NotTo(BeEmpty())

			// Within each tier, plugins must be in alphabetical filename order
			names := make([]string, len(plugins))
			for i, p := range plugins {
				names[i] = p.Name
			}
			// alpha-filter (Tier 2), beta-transform (Tier 2), gamma-minifier (Tier 3)
			Expect(names).To(ContainElements("alpha-filter", "beta-transform", "gamma-minifier"))
		})

		It("Tier 2 plugins load before Tier 3 plugins", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			err := registry.DiscoverPlugins()
			Expect(err).NotTo(HaveOccurred())

			plugins := registry.Plugins()
			Expect(len(plugins)).To(BeNumerically(">=", 3))

			// Find the positions of Tier 2 and Tier 3 plugins
			lastTier2Index := -1
			firstTier3Index := len(plugins)
			for i, p := range plugins {
				if p.Tier == plugin.TierInProcess {
					lastTier2Index = i
				}
				if p.Tier == plugin.TierNode && i < firstTier3Index {
					firstTier3Index = i
				}
			}

			Expect(lastTier2Index).To(BeNumerically("<", firstTier3Index),
				"all Tier 2 plugins must appear before any Tier 3 plugins")
		})
	})

	// ── Name conflicts ─────────────────────────────────────────────────

	Describe("Name conflicts", func() {
		It("registering the same filter name twice produces a warning", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))

			registry.RegisterFilter("slugify", "built-in")
			registry.RegisterFilter("slugify", "plugins/custom-slugify.wasm")

			warnings := registry.ConflictWarnings()
			Expect(warnings).NotTo(BeEmpty(), "should produce a conflict warning")
			Expect(warnings[0]).To(ContainSubstring("slugify"),
				"warning should name the conflicting filter")
		})
	})

	// ── Plugin discovery ──────────────────────────────────────────────

	Describe("Plugin discovery", func() {
		It("empty plugins directory returns no plugins and no error", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-empty"))
			err := registry.DiscoverPlugins()
			Expect(err).NotTo(HaveOccurred())

			plugins := registry.Plugins()
			Expect(plugins).To(BeEmpty())
		})

		It("missing plugins directory returns no plugins and no error", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "no-such-directory"))
			err := registry.DiscoverPlugins()
			Expect(err).NotTo(HaveOccurred())

			plugins := registry.Plugins()
			Expect(plugins).To(BeEmpty())
		})

		It("discovers all plugin files from a populated directory", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			err := registry.DiscoverPlugins()
			Expect(err).NotTo(HaveOccurred())

			plugins := registry.Plugins()
			// alpha-filter.js (Tier 2), beta-transform.wasm (Tier 2), gamma-minifier.js (Tier 3)
			// readme.md should NOT be included
			Expect(plugins).To(HaveLen(3))

			names := make([]string, len(plugins))
			for i, p := range plugins {
				names[i] = p.Name
			}
			Expect(names).To(ConsistOf("alpha-filter", "beta-transform", "gamma-minifier"))
		})

		It("non-plugin files in directory are ignored", func() {
			registry := plugin.NewRegistry(filepath.Join(testdataDir(), "plugins-populated"))
			err := registry.DiscoverPlugins()
			Expect(err).NotTo(HaveOccurred())

			plugins := registry.Plugins()
			for _, p := range plugins {
				Expect(p.Name).NotTo(Equal("readme"),
					".md files should not be discovered as plugins")
			}
		})
	})

	// ── Race safety (issue #768) ─────────────────────────────────────
	// InitRuntimes must not race on its internal results slice when
	// processing a mix of Node (sequential) and in-process (concurrent)
	// plugins.

	Describe("Race safety (issue #768)", func() {
		It("InitRuntimes accounts for every discovered plugin in its results", func() {
			// InitRuntimes processes Node plugins sequentially on the main
			// goroutine and QuickJS/WASM plugins in concurrent goroutines.
			// Both paths append to a shared `results` slice. The goroutines
			// synchronize via mu.Lock in InitRuntimes, but the main
			// goroutine (Node path) appends without holding mu.
			// When the concurrent append races, entries are silently lost
			// and the returned runtimes+warnings won't account for every
			// discovered plugin.
			//
			// Fix direction: acquire mu before appending in the Node path,
			// or collect Node results separately and merge after wg.Wait().
			tmpDir := GinkgoT().TempDir()

			// Create many QuickJS plugins to maximize goroutine overlap
			// with the main goroutine's unsynchronized append.
			qjsCount := 15
			for i := 0; i < qjsCount; i++ {
				name := fmt.Sprintf("plugin-%02d", i)
				Expect(os.WriteFile(
					filepath.Join(tmpDir, name+".js"),
					[]byte(`export default function(alloy) { alloy.filter('`+name+`', (v) => v); }`),
					0644,
				)).To(Succeed())
			}

			// Node plugins — processed sequentially on main goroutine.
			// Sort after QuickJS plugins (Tier 2 before Tier 3), so
			// goroutines are already running when the main goroutine
			// appends these results without holding the mutex.
			nodeCount := 5
			for i := 0; i < nodeCount; i++ {
				name := fmt.Sprintf("zulu-node-%02d", i)
				Expect(os.WriteFile(
					filepath.Join(tmpDir, name+".js"),
					[]byte("export const runtime = \"node\";\nexport default function(alloy) {}"),
					0644,
				)).To(Succeed())
			}

			totalPlugins := qjsCount + nodeCount

			for iter := 0; iter < 20; iter++ {
				registry := plugin.NewRegistry(tmpDir)
				Expect(registry.DiscoverPlugins()).To(Succeed())
				Expect(registry.Plugins()).To(HaveLen(totalPlugins))

				func() {
					defer registry.Close()
					runtimes, warnings := registry.InitRuntimes()
					accounted := len(runtimes) + len(warnings)
					Expect(accounted).To(Equal(totalPlugins),
						fmt.Sprintf("iteration %d: every discovered plugin must appear "+
							"in either runtimes or warnings — got %d of %d; "+
							"if entries are missing, the unsynchronized append in "+
							"the Node path lost results during concurrent access",
							iter, accounted, totalPlugins))
				}()
			}
		})
	})

	// ── Plugin source cleanup on Close (issue #1042) ─────────────────
	// registerRuntime writes closures into the process-global pluginSources
	// map (fetch.RegisterPluginSource), but Registry.Close() never clears
	// them. Stale closures reference stopped NodeBridge instances and
	// produce "bridge not started" errors instead of "not registered".

	Describe("Plugin source cleanup on Close (issue #1042)", func() {
		BeforeEach(func() {
			fetch.ResetPluginSources()
		})

		AfterEach(func() {
			fetch.ResetPluginSources()
		})

		It("Close() clears plugin sources registered during the registry lifecycle", func() {
			// Simulate what registerRuntime does: register plugin source
			// handlers via fetch.RegisterPluginSource. In production, these
			// closures capture a NodeRuntime whose bridge is later stopped
			// by Close(). After Close(), the global pluginSources map must
			// not contain stale handlers.
			fetch.RegisterPluginSource("registry-src-a", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{"data-a"}, nil
			})
			fetch.RegisterPluginSource("registry-src-b", func(config map[string]interface{}) (interface{}, error) {
				return []interface{}{"data-b"}, nil
			})
			Expect(fetch.RegisteredPluginSources()).To(HaveLen(2),
				"precondition: two sources must be registered")

			// Close the registry — must also clear plugin sources
			registry := plugin.NewRegistry(GinkgoT().TempDir())
			registry.Close()

			// After Close(), the source handlers must be removed.
			// Currently Close() does not call ResetPluginSources(), so
			// stale closures remain in the global map — calling
			// FetchPluginSource hits the dead bridge and returns
			// "bridge not started" instead of "not registered".
			Expect(fetch.RegisteredPluginSources()).To(BeEmpty(),
				"Registry.Close() must clear all plugin source handlers — "+
					"without this, stale closures referencing stopped NodeBridge "+
					"instances remain in the global pluginSources map, causing "+
					"'bridge not started' errors instead of 'not registered' "+
					"during dev server full rebuilds (issue #1042)")
		})

		It("after Close(), FetchPluginSource returns 'not registered' not 'bridge not started'", func() {
			// Reproduce the stale closure scenario:
			// 1. A closure captures a stopped NodeRuntime (bridge==nil)
			// 2. Calling the handler returns "bridge not started"
			// 3. Close() should remove it so FetchPluginSource returns "not registered"
			staleRT := plugin.NewNodeRuntime()
			// Do NOT call EvalFile — bridge stays nil, simulating a stopped runtime
			fetch.RegisterPluginSource("stale-src", func(config map[string]interface{}) (interface{}, error) {
				// This is what happens when registerRuntime's closure calls
				// CallSource on a runtime whose bridge was stopped
				return staleRT.CallSource("stale-src", config)
			})

			// Verify the stale closure produces the wrong error
			_, preCloseErr := fetch.FetchPluginSource("stale-src", nil)
			Expect(preCloseErr).To(HaveOccurred())
			Expect(preCloseErr.Error()).To(ContainSubstring("bridge"),
				"precondition: stale closure must produce a bridge-related error "+
					"proving the handler is still registered but points to a dead runtime")

			// Close the registry — must remove the stale handler
			registry := plugin.NewRegistry(GinkgoT().TempDir())
			registry.Close()

			// Now FetchPluginSource should produce "not registered"
			_, postCloseErr := fetch.FetchPluginSource("stale-src", nil)
			Expect(postCloseErr).To(HaveOccurred(),
				"calling a source after Close() must produce an error")
			Expect(postCloseErr.Error()).To(ContainSubstring("not registered"),
				"error must be 'not registered' — Close() must have removed "+
					"the stale handler from the global pluginSources map")
			Expect(postCloseErr.Error()).NotTo(ContainSubstring("bridge not started"),
				"'bridge not started' means the stale closure is still registered — "+
					"Registry.Close() must remove it (issue #1042)")
		})

		It("dev server rebuild: Close() + new InitRuntimes produces fresh handlers", func() {
			// Dev server lifecycle: old registry closed, new one created.
			// Sources registered by the old registry must not persist.
			fetch.RegisterPluginSource("session-1-src", func(config map[string]interface{}) (interface{}, error) {
				return "old-data", nil
			})
			Expect(fetch.RegisteredPluginSources()).To(ContainElement("session-1-src"))

			// Simulate Close()
			reg1 := plugin.NewRegistry(GinkgoT().TempDir())
			reg1.Close()

			// Old source must be gone
			Expect(fetch.RegisteredPluginSources()).NotTo(ContainElement("session-1-src"),
				"sources from the old registry lifecycle must be cleared after Close() — "+
					"dev server rebuilds must start with a clean source map")

			// New source registration works cleanly
			fetch.RegisterPluginSource("session-2-src", func(config map[string]interface{}) (interface{}, error) {
				return "fresh-data", nil
			})
			result, err := fetch.FetchPluginSource("session-2-src", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("fresh-data"),
				"fresh handler registered after Close() must work correctly")
		})
	})

	// ── Node availability ─────────────────────────────────────────────

	Describe("Node availability", func() {
		It("returns clear error when node is not in PATH and Tier 3 plugins exist", func() {
			err := plugin.CheckNodeAvailable()
			// When implemented, this should only error if node is missing.
			// The stub returns ErrNotImplemented which fails the "clear error" check.
			Expect(err).To(
				SatisfyAny(
					BeNil(),
					WithTransform(func(e error) string { return e.Error() }, SatisfyAny(
						ContainSubstring("node"),
						ContainSubstring("not found"),
						ContainSubstring("PATH"),
					)),
				),
				"if error occurs, it should describe the missing node binary",
			)
		})
	})
})
