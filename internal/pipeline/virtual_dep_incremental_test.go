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

	// ── File-derived virtual page dependency tracking (issue #1058) ──
	//
	// After #970 fixed virtual page participation in incremental rebuilds,
	// ALL virtual pages from onPagesReady are re-rendered on every
	// incremental rebuild. For a site with 400 file-derived virtual pages,
	// this means all 400 re-render when a single source file changes.
	//
	// The dependency tracking optimization lets plugins declare which
	// source files each virtual page depends on. Only virtual pages whose
	// dependencies appear in changedFiles are re-rendered. Virtual pages
	// without declared dependencies use the safe fallback (always re-render).

	Describe("File-derived virtual page dependency tracking (issue #1058)", func() {

		// Helper: creates a project with content, layouts, and a plugin that
		// injects virtual pages via onPagesReady. When pluginJS is empty,
		// uses the default plugin with 4 virtual pages covering all
		// dependency states:
		//
		//   _virtual/demos/button.html  → depends on: elements/button/demo.html
		//   _virtual/demos/card.html    → depends on: elements/card/demo.html
		//   _virtual/demos/untracked.html → no dependencies field (safe fallback)
		//   _virtual/demos/empty-deps.html → dependencies: [] (tracked, no file deps)
		//
		// Returns (tmpDir, config).
		setupDepTrackingProject := func(pluginJS ...string) (string, *config.Config) {
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

			// Use provided plugin JS or the default 4-page plugin.
			js := `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function({ siteData }) {
    return {
      addPages: [
        {
          path: '_virtual/demos/button.html',
          url: '/demos/button/',
          dependencies: ['elements/button/demo.html'],
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
          dependencies: ['elements/card/demo.html'],
          frontMatter: {
            title: 'Card Demo',
            layout: 'demo',
            markdown: false
          },
          content: '<p>Card demo content</p>'
        },
        {
          path: '_virtual/demos/untracked.html',
          url: '/demos/untracked/',
          frontMatter: {
            title: 'Untracked Demo',
            layout: 'demo',
            markdown: false
          },
          content: '<p>Untracked demo content</p>'
        },
        {
          path: '_virtual/demos/empty-deps.html',
          url: '/demos/empty-deps/',
          dependencies: [],
          frontMatter: {
            title: 'Empty Deps Demo',
            layout: 'demo',
            markdown: false
          },
          content: '<p>Empty deps demo content</p>'
        }
      ]
    };
  });
}`
			if len(pluginJS) > 0 && pluginJS[0] != "" {
				js = pluginJS[0]
			}
			Expect(os.WriteFile(filepath.Join(pluginsDir, "virtual-demos.js"),
				[]byte(js), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Dep Tracking Test",
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

		It("selectively renders only virtual pages whose dependency changed", func() {
			tmpDir, cfg := setupDepTrackingProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial (nil cache) — all pages rendered
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())

			// Sanity: all 4 virtual pages rendered in initial build
			Expect(result1.RenderedContent["_virtual/demos/button.html"]).To(
				ContainSubstring("Button demo content"),
				"sanity: initial build must render button demo")
			Expect(result1.RenderedContent["_virtual/demos/card.html"]).To(
				ContainSubstring("Card demo content"),
				"sanity: initial build must render card demo")
			Expect(result1.RenderedContent["_virtual/demos/untracked.html"]).To(
				ContainSubstring("Untracked demo content"),
				"sanity: initial build must render untracked demo")
			Expect(result1.RenderedContent["_virtual/demos/empty-deps.html"]).To(
				ContainSubstring("Empty deps demo content"),
				"sanity: initial build must render empty-deps demo")

			// Trigger a content change so the incremental rebuild has something
			// to do — without this, all content pages are skipped.
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home Updated\nlayout: default\n---\n# Home Updated"),
				0644)).To(Succeed())

			// Build 2: incremental rebuild — only elements/button/demo.html
			// appears in changedFiles. This is a file OUTSIDE the content
			// directory that the button virtual page declared as a dependency.
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md", "elements/button/demo.html"},
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())

			// Button: dependency matches changedFiles → MUST be rendered
			Expect(result2.RenderedContent).To(HaveKey("_virtual/demos/button.html"),
				"virtual page with dependencies: ['elements/button/demo.html'] must "+
					"be re-rendered when 'elements/button/demo.html' appears in "+
					"changedFiles (issue #1058)")
			Expect(result2.RenderedContent["_virtual/demos/button.html"]).To(
				ContainSubstring("Button demo content"),
				"rendered button page must contain its content")

			// Card: dependency does NOT match changedFiles → MUST be skipped
			Expect(result2.RenderedContent).NotTo(HaveKey("_virtual/demos/card.html"),
				"virtual page with dependencies: ['elements/card/demo.html'] must "+
					"NOT be re-rendered when only 'elements/button/demo.html' changed — "+
					"the dependency tracking must skip virtual pages whose source "+
					"files are unchanged. Without issue #1058, the #970 naive "+
					"approach re-renders ALL virtual pages on every incremental "+
					"rebuild regardless of what changed.")

			// Untracked: no dependencies field → MUST be rendered (safe fallback)
			Expect(result2.RenderedContent).To(HaveKey("_virtual/demos/untracked.html"),
				"virtual page WITHOUT dependencies field must always be re-rendered "+
					"during incremental rebuilds — the absence of dependencies means "+
					"'unknown what this page depends on', so the safe fallback is "+
					"to always re-render it (issue #1058)")

			// Empty deps: dependencies: [] → MUST be skipped (tracked, no file deps)
			Expect(result2.RenderedContent).NotTo(HaveKey("_virtual/demos/empty-deps.html"),
				"virtual page with dependencies: [] must be skipped — an empty "+
					"dependencies array means 'this page depends on no local files', "+
					"so no file change can invalidate it. This is distinct from "+
					"absent dependencies (which means 'unknown deps, always re-render') "+
					"(issue #1058)")
		})

		It("renders all virtual pages on initial build regardless of dependencies", func() {
			_, cfg := setupDepTrackingProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Initial build with nil cache — all pages must be rendered,
			// regardless of their dependency declarations.
			result, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())

			Expect(result.RenderedContent["_virtual/demos/button.html"]).To(
				ContainSubstring("Button demo content"),
				"initial build must render virtual page with dependencies "+
					"(nil cache means full build — no selective filtering)")
			Expect(result.RenderedContent["_virtual/demos/card.html"]).To(
				ContainSubstring("Card demo content"),
				"initial build must render all virtual pages")
			Expect(result.RenderedContent["_virtual/demos/untracked.html"]).To(
				ContainSubstring("Untracked demo content"),
				"initial build must render untracked virtual pages")
			Expect(result.RenderedContent["_virtual/demos/empty-deps.html"]).To(
				ContainSubstring("Empty deps demo content"),
				"initial build must render virtual pages with empty dependencies — "+
					"dependency filtering only applies to incremental rebuilds, "+
					"not the initial build (issue #1058)")
		})

		It("renders multiple virtual pages when they share a changed dependency", func() {
			// Plugin with two virtual pages sharing a dependency on the same
			// base file, plus one with its own unique dependency.
			sharedDepPlugin := `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function({ siteData }) {
    return {
      addPages: [
        {
          path: '_virtual/demos/accordion.html',
          url: '/demos/accordion/',
          dependencies: ['elements/shared/base.html', 'elements/accordion/demo.html'],
          frontMatter: { title: 'Accordion', layout: 'demo', markdown: false },
          content: '<p>Accordion demo</p>'
        },
        {
          path: '_virtual/demos/tabs.html',
          url: '/demos/tabs/',
          dependencies: ['elements/shared/base.html', 'elements/tabs/demo.html'],
          frontMatter: { title: 'Tabs', layout: 'demo', markdown: false },
          content: '<p>Tabs demo</p>'
        },
        {
          path: '_virtual/demos/tooltip.html',
          url: '/demos/tooltip/',
          dependencies: ['elements/tooltip/demo.html'],
          frontMatter: { title: 'Tooltip', layout: 'demo', markdown: false },
          content: '<p>Tooltip demo</p>'
        }
      ]
    };
  });
}`
			tmpDir, cfg := setupDepTrackingProject(sharedDepPlugin)

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.RenderedContent["_virtual/demos/accordion.html"]).To(
				ContainSubstring("Accordion demo"),
				"sanity: initial build must render accordion")

			// Trigger content change for the incremental rebuild
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home v2\nlayout: default\n---\n# Home v2"),
				0644)).To(Succeed())

			// Build 2: shared base file changed — both accordion and tabs
			// depend on it, but tooltip does not.
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md", "elements/shared/base.html"},
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())

			// Accordion: depends on shared/base.html → rendered
			Expect(result2.RenderedContent).To(HaveKey("_virtual/demos/accordion.html"),
				"accordion depends on elements/shared/base.html which changed — "+
					"must be re-rendered (issue #1058)")

			// Tabs: also depends on shared/base.html → rendered
			Expect(result2.RenderedContent).To(HaveKey("_virtual/demos/tabs.html"),
				"tabs also depends on elements/shared/base.html which changed — "+
					"must be re-rendered alongside accordion (issue #1058)")

			// Tooltip: depends only on tooltip/demo.html, NOT shared/base.html → skipped
			Expect(result2.RenderedContent).NotTo(HaveKey("_virtual/demos/tooltip.html"),
				"tooltip depends only on elements/tooltip/demo.html which did not "+
					"change — must be skipped even though other virtual pages are "+
					"being re-rendered (issue #1058)")
		})

		It("cache tracks virtual dependencies across rebuild generations", func() {
			tmpDir, cfg := setupDepTrackingProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())

			// Build 1's cache must track virtual dependencies
			buttonDeps := result1.Cache.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(buttonDeps).To(ConsistOf("_virtual/demos/button.html"),
				"cache after initial build must track that button virtual page "+
					"depends on elements/button/demo.html — this enables selective "+
					"rebuild in subsequent incremental builds (issue #1058)")

			cardDeps := result1.Cache.InvalidatedVirtualPages("elements/card/demo.html")
			Expect(cardDeps).To(ConsistOf("_virtual/demos/card.html"),
				"cache must track all virtual page dependencies (issue #1058)")

			// Trigger content change for rebuild
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home v2\nlayout: default\n---\n# Home v2"),
				0644)).To(Succeed())

			// Build 2: incremental — verify cache is carried forward
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState})
			Expect(err).NotTo(HaveOccurred())

			// Build 2's cache must also track virtual dependencies
			buttonDeps2 := result2.Cache.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(buttonDeps2).To(ConsistOf("_virtual/demos/button.html"),
				"cache must track virtual dependencies across rebuild generations — "+
					"each build re-populates the dependency map from the current "+
					"onPagesReady output (issue #1058)")
		})

		It("Build() cache tracks virtual dependencies for BuildIncremental() handoff", func() {
			_, cfg := setupDepTrackingProject()

			// Build() discovers plugins internally and closes them on return.
			// This simulates cmd/dev.go: Build() runs first, then
			// BuildIncremental() uses result.Cache.
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Cache).NotTo(BeNil())

			buttonDeps := result.Cache.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(buttonDeps).To(ConsistOf("_virtual/demos/button.html"),
				"Build() cache must track virtual dependencies, not just "+
					"virtual page RelPaths — without this, the first "+
					"BuildIncremental() after Build() cannot do selective "+
					"rebuild based on dependencies (issue #1058)")

			cardDeps := result.Cache.InvalidatedVirtualPages("elements/card/demo.html")
			Expect(cardDeps).To(ConsistOf("_virtual/demos/card.html"),
				"Build() cache must track all virtual dependency entries (issue #1058)")

			// Untracked page should have no dependency entries.
			// ContainElement handles nil slices correctly (returns false).
			untrackedDeps := result.Cache.InvalidatedVirtualPages("anything")
			Expect(untrackedDeps).NotTo(ContainElement("_virtual/demos/untracked.html"),
				"untracked virtual page (no dependencies field) must not appear "+
					"in any InvalidatedVirtualPages result — it has no source "+
					"file dependencies to track (issue #1058)")
		})
	})
})
