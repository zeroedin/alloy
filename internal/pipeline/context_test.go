package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Build Pipeline", func() {
	Describe("Node plugin respects ProjectRoot (issue #439)", func() {
		It("DiscoverPlugins passes ProjectRoot to Node plugin bridge", func() {
			// Create a project with a Node plugin
			projectDir := GinkgoT().TempDir()

			pluginsDir := filepath.Join(projectDir, "plugins")
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(pluginsDir, "test-plugin.js"),
				[]byte("export const runtime = \"node\";\nexport default function(alloy) { alloy.filter('testNodeFilter', (v) => v); }"),
				0644,
			)).To(Succeed())

			cfg := &config.Config{
				Title:       "Node CWD Test",
				BaseURL:     "https://example.com",
				Build:       config.BuildConfig{Output: "_site"},
				ProjectRoot: projectDir,
				Plugins:     config.PluginsConfig{Node: true, Timeout: 5000},
			}
			config.ApplyDefaults(cfg)

			registry, _, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()

			// The Node runtime's project root must match cfg.ProjectRoot
			found := false
			for _, rt := range registry.Runtimes() {
				if nr, ok := rt.(interface{ ProjectRoot() string }); ok {
					found = true
					Expect(nr.ProjectRoot()).To(Equal(projectDir),
						"Node runtime project root must equal cfg.ProjectRoot — "+
							"when -r is used, the Node subprocess must run from the "+
							"project directory for correct node_modules/ resolution (issue #439)")
				}
			}
			Expect(found).To(BeTrue(),
				"at least one runtime must implement ProjectRoot() — "+
					"if false, the Node plugin was not loaded (Node not in PATH, "+
					"plugin classified as QuickJS, or eval failed silently)")
		})
	})

	Describe("Nested plugins path project root derivation (issue #807)", func() {
		It("DiscoverPlugins sets correct ProjectRoot with nested plugins directory", func() {
			projectDir := GinkgoT().TempDir()

			// Nested plugins path — e.g. tools/plugins instead of plugins/
			nestedPluginsDir := filepath.Join(projectDir, "tools", "plugins")
			Expect(os.MkdirAll(nestedPluginsDir, 0755)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(nestedPluginsDir, "nested-plugin.js"),
				[]byte("export const runtime = \"node\";\nexport default function(alloy) { alloy.filter('nestedFilter', (v) => v); }"),
				0644,
			)).To(Succeed())

			cfg := &config.Config{
				Title:       "Nested Plugins Root Test",
				BaseURL:     "https://example.com",
				Build:       config.BuildConfig{Output: "_site"},
				ProjectRoot: projectDir,
				Plugins:     config.PluginsConfig{Node: true, Timeout: 5000},
				Structure:   config.StructureConfig{Plugins: "tools/plugins"},
			}
			config.ApplyDefaults(cfg)

			registry, _, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()

			// With a nested path like tools/plugins, the old code derived
			// project root via filepath.Dir(pluginsDir) which returned
			// /project/tools — not the actual project root. The fix passes
			// cfg.ProjectRoot explicitly via Registry.SetProjectRoot().
			found := false
			for _, rt := range registry.Runtimes() {
				if nr, ok := rt.(interface{ ProjectRoot() string }); ok {
					found = true
					Expect(nr.ProjectRoot()).To(Equal(projectDir),
						"Node runtime project root must equal cfg.ProjectRoot, not "+
							"filepath.Dir(pluginsDir) — with structure.plugins: \"tools/plugins\" "+
							"the old derivation returned the wrong ancestor directory (issue #807)")
				}
			}
			Expect(found).To(BeTrue(),
				"at least one runtime must implement ProjectRoot() — "+
					"if false, the Node plugin in tools/plugins was not loaded")
		})
	})

	// ── onContentTransformed page object payload (issue #448) ───────
	// onContentTransformed must receive a page object with html, toc,
	// path, url, and frontMatter — not just an HTML string.
	// Plugins can mutate toc and frontMatter before layout rendering.
})
