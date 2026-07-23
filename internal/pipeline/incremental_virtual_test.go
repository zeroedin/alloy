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

	// ── Virtual pages in incremental rebuilds (issue #970) ───────────
	//
	// Virtual pages injected by onPagesReady are filtered out during
	// BuildIncremental because pagesToRender is decided before
	// onPagesReady runs. The fix tracks virtual page RelPaths in the
	// build cache so they can be pre-populated into renderRelPaths
	// before onPagesReady recreates them.

	Describe("Virtual pages in incremental rebuilds (issue #970)", func() {

		// Helper: sets up a project with a plugin that injects virtual pages
		// via onPagesReady. Returns the config and a cleanup function.
		// The plugin creates two virtual pages:
		//   _virtual/demos/button.html  → /demos/button/
		//   _virtual/demos/card.html    → /demos/card/
		setupVirtualPageProject := func() (string, *config.Config) {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			// A real content page (filesystem-discovered)
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "demo.liquid"),
				[]byte("<html><body class=\"demo\">{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Plugin that injects virtual pages via onPagesReady.
			// Uses addPages return shape — injection-only, no mutation.
			Expect(os.WriteFile(filepath.Join(pluginsDir, "virtual-demos.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function({ siteData }) {
    return {
      addPages: [
        {
          path: '_virtual/demos/button.html',
          url: '/demos/button/',
          frontMatter: {
            title: 'Button Demo',
            layout: 'demo',
            markdown: false
          },
          content: '<p>Button demo content</p>'
        },
        {
          path: '_virtual/demos/card.html',
          url: '/demos/card/',
          frontMatter: {
            title: 'Card Demo',
            layout: 'demo',
            markdown: false
          },
          content: '<p>Card demo content</p>'
        }
      ]
    };
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Virtual Page Incremental Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			return tmpDir, cfg
		}

		It("virtual pages from onPagesReady are rendered in initial incremental build", func() {
			_, cfg := setupVirtualPageProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Initial build (nil cache) — all pages must be rendered,
			// including virtual pages from onPagesReady.
			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Virtual pages must appear in RenderedContent keyed by RelPath.
			// renderedContentKey() returns RelPath for non-paginated pages
			// with non-empty RelPath — virtual pages from addPages have
			// RelPath set to their path field (e.g. "_virtual/demos/button.html").
			buttonHTML := result.RenderedContent["_virtual/demos/button.html"]
			Expect(buttonHTML).To(ContainSubstring("Button demo content"),
				"virtual page _virtual/demos/button.html must be rendered in initial "+
					"incremental build — onPagesReady injects it into allPages "+
					"and nil cache means all pages should render")
			Expect(buttonHTML).To(ContainSubstring(`class="demo"`),
				"virtual page must be wrapped in its layout (demo.liquid)")

			cardHTML := result.RenderedContent["_virtual/demos/card.html"]
			Expect(cardHTML).To(ContainSubstring("Card demo content"),
				"virtual page _virtual/demos/card.html must also be rendered in initial build")

			// Real page must also render normally
			homeHTML := result.RenderedContent["index.md"]
			Expect(homeHTML).To(ContainSubstring("Home"),
				"filesystem-discovered page must render alongside virtual pages")
		})

		It("virtual pages are re-rendered in subsequent incremental builds after content change", func() {
			tmpDir, cfg := setupVirtualPageProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial (nil cache)
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.RenderedContent["_virtual/demos/button.html"]).To(ContainSubstring("Button demo content"),
				"sanity: initial build must render virtual page")

			// Modify a real content file to trigger incremental rebuild
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home Updated\nlayout: default\n---\n# Home Updated"),
				0644)).To(Succeed())

			// Build 2: incremental with cache from build 1
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// This is the core #970 bug: virtual pages are filtered out of
			// pagesToRender because their RelPaths are never in renderRelPaths.
			// After the fix, the cache tracks virtual page RelPaths from
			// the previous build and pre-populates renderRelPaths with them.
			buttonHTML := result2.RenderedContent["_virtual/demos/button.html"]
			Expect(buttonHTML).To(ContainSubstring("Button demo content"),
				"virtual page _virtual/demos/button.html must be re-rendered in incremental "+
					"rebuild — currently filtered out because pagesToRender is "+
					"decided before onPagesReady runs, and virtual page RelPaths "+
					"are never in renderRelPaths (issue #970)")

			cardHTML := result2.RenderedContent["_virtual/demos/card.html"]
			Expect(cardHTML).To(ContainSubstring("Card demo content"),
				"virtual page _virtual/demos/card.html must also be re-rendered — all "+
					"virtual pages from the previous build must participate in "+
					"incremental rebuilds (issue #970)")
		})

		It("virtual pages are re-rendered when no content changes but layout changes", func() {
			tmpDir, cfg := setupVirtualPageProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.RenderedContent["_virtual/demos/button.html"]).To(ContainSubstring(`class="demo"`),
				"sanity: initial build must apply demo layout")

			// Change the demo layout
			Expect(os.WriteFile(filepath.Join(tmpDir, "layouts", "demo.liquid"),
				[]byte("<html><body class=\"demo-updated\">{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Build 2: layout change — virtual pages using demo layout must re-render
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"layouts/demo.liquid"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			buttonHTML := result2.RenderedContent["_virtual/demos/button.html"]
			Expect(buttonHTML).To(ContainSubstring(`class="demo-updated"`),
				"virtual page must reflect the updated layout — the templates "+
					"cache tracks which pages used which layout, so layout changes "+
					"must invalidate virtual pages that used the changed layout. "+
					"This requires virtual pages to be in renderRelPaths first (issue #970)")
			Expect(buttonHTML).To(ContainSubstring("Button demo content"),
				"virtual page content must be preserved after layout change")
		})

		It("cache from incremental build tracks virtual page RelPaths", func() {
			_, cfg := setupVirtualPageProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Cache).NotTo(BeNil(),
				"BuildIncremental must return a cache")

			virtualPaths := result.Cache.VirtualPagePaths()
			Expect(virtualPaths).To(ContainElement("_virtual/demos/button.html"),
				"cache must track virtual page RelPath '_virtual/demos/button.html' — "+
					"without this, the next incremental rebuild cannot pre-populate "+
					"renderRelPaths with virtual page paths (issue #970)")
			Expect(virtualPaths).To(ContainElement("_virtual/demos/card.html"),
				"cache must track all virtual page RelPaths, not just the first")

			// Filesystem-discovered pages must NOT be tracked as virtual
			Expect(virtualPaths).NotTo(ContainElement("index.md"),
				"filesystem-discovered pages must not be tracked as virtual pages — "+
					"only pages injected by onPagesReady are virtual")
		})

		It("virtual pages are written to the output directory", func() {
			tmpDir, cfg := setupVirtualPageProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Verify initial output files exist
			buttonPath := filepath.Join(tmpDir, "_site", "demos", "button", "index.html")
			buttonOnDisk, readErr := os.ReadFile(buttonPath)
			Expect(readErr).NotTo(HaveOccurred(),
				"virtual page must be written to output directory on initial build")
			Expect(string(buttonOnDisk)).To(ContainSubstring("Button demo content"),
				"output file must contain the virtual page content")

			// Delete the output file so build 2 must recreate it — otherwise
			// build 1's file persists on disk and the assertion passes vacuously.
			Expect(os.Remove(buttonPath)).To(Succeed())

			// Modify content to trigger incremental rebuild
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home v2\nlayout: default\n---\n# Home v2"),
				0644)).To(Succeed())

			// Build 2: incremental — virtual pages must still be written
			_, err = pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Re-read the output file — must exist because build 2 rewrote it.
			// We deleted it after build 1, so this proves the incremental
			// rebuild actually wrote the virtual page to disk.
			buttonOnDisk2, readErr2 := os.ReadFile(buttonPath)
			Expect(readErr2).NotTo(HaveOccurred(),
				"virtual page must be re-written to output directory after "+
					"incremental rebuild — file was deleted after build 1 to prove "+
					"build 2 actually writes it (issue #970)")
			Expect(string(buttonOnDisk2)).To(ContainSubstring("Button demo content"),
				"output file must contain virtual page content after incremental rebuild")
		})

		It("multiple incremental rebuilds continue rendering virtual pages", func() {
			tmpDir, cfg := setupVirtualPageProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.RenderedContent["_virtual/demos/button.html"]).To(ContainSubstring("Button demo content"),
				"sanity: initial build must render virtual page")

			// Build 2: first incremental
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: v2\nlayout: default\n---\n# v2"),
				0644)).To(Succeed())
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Build 3: second incremental — using cache from build 2
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: v3\nlayout: default\n---\n# v3"),
				0644)).To(Succeed())
			result3, err := pipeline.BuildIncremental(cfg, nil, result2.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			buttonHTML := result3.RenderedContent["_virtual/demos/button.html"]
			Expect(buttonHTML).To(ContainSubstring("Button demo content"),
				"virtual pages must survive across multiple incremental rebuilds — "+
					"the cache must be updated after each build with the current "+
					"virtual page RelPaths, not just the initial build's (issue #970)")

			// Verify cache chain: build 3's cache must still track virtual pages
			virtualPaths := result3.Cache.VirtualPagePaths()
			Expect(virtualPaths).To(ContainElement("_virtual/demos/button.html"),
				"cache must track virtual pages across multiple rebuild generations")
		})

		It("Build() cache tracks virtual pages for BuildIncremental() handoff", func() {
			tmpDir, cfg := setupVirtualPageProject()

			// Build() discovers plugins internally and closes them on return.
			// This simulates the cmd/dev.go pattern: Build() runs first (line 88),
			// stores result.Cache (line 93-94), then BuildIncremental() uses it.
			result1, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.Cache).NotTo(BeNil(),
				"Build() must return a cache for the BuildIncremental() handoff")

			// Verify Build() tracked virtual page RelPaths in its cache
			virtualPaths := result1.Cache.VirtualPagePaths()
			Expect(virtualPaths).To(ContainElement("_virtual/demos/button.html"),
				"Build() cache must track virtual page RelPath '_virtual/demos/button.html' — "+
					"without this, the first BuildIncremental() after Build() cannot "+
					"pre-populate renderRelPaths with virtual page paths")
			Expect(virtualPaths).To(ContainElement("_virtual/demos/card.html"),
				"Build() cache must track all virtual page RelPaths")

			// Filesystem-discovered pages must NOT be tracked as virtual
			Expect(virtualPaths).NotTo(ContainElement("index.md"),
				"Build() must not track filesystem-discovered pages as virtual — "+
					"only pages injected by onPagesReady are virtual")

			// Now pass Build()'s cache to BuildIncremental() — Build() closed
			// its internal registry, so we need a fresh one for BuildIncremental().
			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Modify content to trigger incremental rebuild
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Updated\nlayout: default\n---\n# Updated"),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Virtual pages must be rendered because Build()'s cache tracked
			// their RelPaths and BuildIncremental() pre-populated renderRelPaths
			// from the cache. This is the Build()→BuildIncremental() handoff
			// that cmd/dev.go relies on.
			buttonHTML := result2.RenderedContent["_virtual/demos/button.html"]
			Expect(buttonHTML).To(ContainSubstring("Button demo content"),
				"virtual page must be rendered after Build()→BuildIncremental() "+
					"cache handoff — Build() tracks virtual RelPaths in its cache, "+
					"BuildIncremental() pre-populates renderRelPaths from that cache")
			Expect(buttonHTML).To(ContainSubstring(`class="demo"`),
				"virtual page must use its layout after cache handoff")

			cardHTML := result2.RenderedContent["_virtual/demos/card.html"]
			Expect(cardHTML).To(ContainSubstring("Card demo content"),
				"all virtual pages must survive the Build()→BuildIncremental() handoff")
		})

		It("virtual pages removed by plugin are not rendered in next incremental build", func() {
			tmpDir, cfg := setupVirtualPageProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: both virtual pages rendered
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.RenderedContent["_virtual/demos/button.html"]).To(ContainSubstring("Button demo content"),
				"sanity: initial build renders button demo")
			Expect(result1.RenderedContent["_virtual/demos/card.html"]).To(ContainSubstring("Card demo content"),
				"sanity: initial build renders card demo")

			// Replace the plugin with one that only returns button (drops card)
			Expect(os.WriteFile(filepath.Join(tmpDir, "plugins", "virtual-demos.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function({ siteData }) {
    return {
      addPages: [
        {
          path: '_virtual/demos/button.html',
          url: '/demos/button/',
          frontMatter: {
            title: 'Button Demo',
            layout: 'demo',
            markdown: false
          },
          content: '<p>Button demo content</p>'
        }
      ]
    };
  });
}`),
				0644)).To(Succeed())

			// Reload plugins to pick up the changed plugin file
			registry2, hooks2, _ := pipeline.DiscoverPlugins(cfg)
			defer registry2.Close()
			pipelineState2, psErr2 := pipeline.InitPipelineState(cfg, registry2, hooks2)
			Expect(psErr2).NotTo(HaveOccurred())

			// Build 2: card virtual page no longer returned by plugin
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home v2\nlayout: default\n---\n# Home v2"),
				0644)).To(Succeed())
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState2, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Button must still render
			Expect(result2.RenderedContent["_virtual/demos/button.html"]).To(ContainSubstring("Button demo content"),
				"virtual page still returned by plugin must continue rendering")

			// Card must NOT render — the plugin no longer returns it.
			// Even though the cache has card's RelPath from the previous build,
			// onPagesReady no longer injects it, so it should not appear.
			// Key is the RelPath ("_virtual/demos/card.html"), not the URL.
			Expect(result2.RenderedContent).NotTo(HaveKey("_virtual/demos/card.html"),
				"virtual page removed by plugin must not be rendered — "+
					"the cache pre-populates renderRelPaths with previous virtual "+
					"page paths, but if onPagesReady no longer injects the page, "+
					"it should not appear in pagesToRender or RenderedContent")

			// Updated cache must not track the removed virtual page
			virtualPaths := result2.Cache.VirtualPagePaths()
			Expect(virtualPaths).NotTo(ContainElement("_virtual/demos/card.html"),
				"cache must be updated to reflect current virtual pages — "+
					"removed pages must not persist in the cache")
		})
	})
})
