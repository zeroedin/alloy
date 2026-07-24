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

	// ── Plugin dependency tracking: addDependencies (issue #1100) ──
	//
	// Plugins that transform page output based on external files
	// (SSR components, Sass partials, translation files, etc.) can
	// declare those dependencies via addDependencies in the return value
	// of per-page hooks (onPageRendered, onContentTransformed). The
	// pipeline extracts these and tracks them in the build cache. On
	// subsequent incremental rebuilds, when a dependency file changes,
	// only the pages that declared that dependency are re-rendered.
	//
	// This is distinct from the virtual page dependency tracking in
	// issue #1058, which applies to plugin-injected virtual pages. This
	// feature applies to content pages — filesystem-discovered pages that
	// go through the normal build pipeline.

	Describe("Plugin dependency tracking via addDependencies (issue #1100)", func() {

		// Helper: creates a project with content pages and a plugin that
		// returns addDependencies from onPageRendered. The plugin simulates
		// an SSR plugin that imports web component definitions and reports
		// which component files each page depends on.
		//
		// Pages:
		//   index.md   → depends on: elements/rh-card/rh-card.js, elements/rh-icon/rh-icon.js
		//   about.md   → depends on: elements/rh-icon/rh-icon.js (only)
		//   blog.md    → no addDependencies returned (plugin doesn't transform it)
		//
		// Returns (tmpDir, config).
		setupDepTrackingProject := func() (string, *config.Config) {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			// Content pages
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home\n<rh-card>content</rh-card>\n<rh-icon>icon</rh-icon>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
				[]byte("---\ntitle: About\nlayout: default\n---\n# About\n<rh-icon>icon</rh-icon>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "blog.md"),
				[]byte("---\ntitle: Blog\nlayout: default\n---\n# Blog\nNo components here."),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Plugin that returns addDependencies based on page content.
			// The plugin detects custom element tags in the HTML and maps
			// them to component source files. This simulates the SSR use
			// case described in issue #1100.
			pluginJS := `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    var deps = [];
    if (page.html.indexOf('rh-card') !== -1) {
      deps.push('elements/rh-card/rh-card.js');
    }
    if (page.html.indexOf('rh-icon') !== -1) {
      deps.push('elements/rh-icon/rh-icon.js');
    }
    if (deps.length > 0) {
      return { html: page.html, addDependencies: deps };
    }
    return page;
  });
}`
			Expect(os.WriteFile(filepath.Join(pluginsDir, "ssr-deps.js"),
				[]byte(pluginJS), 0644)).To(Succeed())

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

		It("tracks dependencies from onPageRendered addDependencies in the build cache", func() {
			_, cfg := setupDepTrackingProject()

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Cache).NotTo(BeNil())

			// index.md uses rh-card and rh-icon → both deps tracked
			cardPages := result.Cache.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(cardPages).To(ConsistOf("index.md"),
				"Build() must extract addDependencies from onPageRendered "+
					"return values and track them in the cache — index.md "+
					"uses <rh-card> so it must depend on rh-card.js "+
					"(issue #1100)")

			iconPages := result.Cache.PagesForDependency("elements/rh-icon/rh-icon.js")
			Expect(iconPages).To(ConsistOf("index.md", "about.md"),
				"Both index.md and about.md use <rh-icon> so both must "+
					"be tracked as depending on rh-icon.js — this enables "+
					"targeted incremental rebuild when rh-icon.js changes "+
					"(issue #1100)")

			// blog.md doesn't use any components → not in any dependency
			for _, dep := range []string{
				"elements/rh-card/rh-card.js",
				"elements/rh-icon/rh-icon.js",
			} {
				pages := result.Cache.PagesForDependency(dep)
				Expect(pages).NotTo(ContainElement("blog.md"),
					"blog.md has no custom elements so the plugin returns "+
						"no addDependencies for it — blog.md must not appear "+
						"in any dependency tracking (issue #1100)")
			}
		})

		It("incrementally rebuilds only pages whose dependency changed", func() {
			tmpDir, cfg := setupDepTrackingProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial — all pages rendered, deps tracked
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Sanity: all 3 pages rendered in initial build
			Expect(result1.RenderedContent).To(HaveKey("index.md"),
				"sanity: initial build must render index.md")
			Expect(result1.RenderedContent).To(HaveKey("about.md"),
				"sanity: initial build must render about.md")
			Expect(result1.RenderedContent).To(HaveKey("blog.md"),
				"sanity: initial build must render blog.md")

			// Sanity: deps tracked in cache
			Expect(result1.Cache.PagesForDependency("elements/rh-icon/rh-icon.js")).To(
				ConsistOf("index.md", "about.md"),
				"sanity: initial build must track icon dependency for both pages")

			// Modify a content file so the incremental rebuild has at
			// least one content change to process (otherwise everything
			// is skipped by content hash).
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home Updated\nlayout: default\n---\n# Home Updated\n<rh-card>content</rh-card>\n<rh-icon>icon</rh-icon>"),
				0644)).To(Succeed())

			// Build 2: incremental — rh-icon.js changed.
			// Pages depending on rh-icon.js: index.md, about.md
			// blog.md has no dependency on rh-icon.js → should be skipped
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md", "elements/rh-icon/rh-icon.js"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// index.md: content changed + dependency changed → MUST be rendered
			Expect(result2.RenderedContent).To(HaveKey("index.md"),
				"index.md has both a content change and a dependency change — "+
					"it must be re-rendered (issue #1100)")

			// about.md: content NOT changed, but dependency changed →
			// MUST be rendered because rh-icon.js is in changedFiles
			// and about.md depends on it.
			Expect(result2.RenderedContent).To(HaveKey("about.md"),
				"about.md depends on elements/rh-icon/rh-icon.js which "+
					"appeared in changedFiles — about.md must be re-rendered "+
					"even though its own content hasn't changed. Without "+
					"issue #1100, about.md would be skipped because its "+
					"content hash is unchanged, and users would see stale "+
					"SSR output (issue #1100)")

			// blog.md: content NOT changed, no dependency on rh-icon.js
			// → MUST be skipped
			Expect(result2.RenderedContent).NotTo(HaveKey("blog.md"),
				"blog.md has no dependency on rh-icon.js and its content "+
					"hasn't changed — it must be skipped during incremental "+
					"rebuild. This is the key optimization: only pages whose "+
					"dependencies actually changed are re-rendered, not all "+
					"720 pages (issue #1100)")
		})

		It("skips dependency-tracked pages when their dependencies are unchanged", func() {
			tmpDir, cfg := setupDepTrackingProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Touch blog.md so we have a content change to trigger rebuild
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "blog.md"),
				[]byte("---\ntitle: Blog Updated\nlayout: default\n---\n# Blog Updated\nStill no components."),
				0644)).To(Succeed())

			// Build 2: only blog.md changed, no dependency files changed.
			// index.md and about.md depend on rh-icon.js but rh-icon.js
			// is NOT in changedFiles → they should be skipped.
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/blog.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// blog.md content changed → rendered
			Expect(result2.RenderedContent).To(HaveKey("blog.md"),
				"blog.md content changed → must be re-rendered")

			// index.md: content unchanged, dependencies unchanged → skipped
			Expect(result2.RenderedContent).NotTo(HaveKey("index.md"),
				"index.md depends on rh-card.js and rh-icon.js, but neither "+
					"appeared in changedFiles — index.md must be skipped "+
					"(issue #1100)")

			// about.md: content unchanged, dependencies unchanged → skipped
			Expect(result2.RenderedContent).NotTo(HaveKey("about.md"),
				"about.md depends on rh-icon.js, but rh-icon.js did not "+
					"appear in changedFiles — about.md must be skipped "+
					"(issue #1100)")
		})

		It("re-tracks dependencies on each rebuild to reflect changed deps", func() {
			tmpDir, cfg := setupDepTrackingProject()

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial — index.md depends on rh-card + rh-icon
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.Cache.PagesForDependency("elements/rh-card/rh-card.js")).To(
				ConsistOf("index.md"),
				"sanity: initial build tracks rh-card dependency for index.md")

			// Modify index.md to REMOVE the rh-card usage
			Expect(os.WriteFile(filepath.Join(tmpDir, "content", "index.md"),
				[]byte("---\ntitle: Home v2\nlayout: default\n---\n# Home v2\n<rh-icon>icon only now</rh-icon>"),
				0644)).To(Succeed())

			// Build 2: incremental — index.md content changed
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// After rebuild, index.md no longer uses rh-card → dependency
			// should no longer be tracked
			cardPages := result2.Cache.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(cardPages).NotTo(ContainElement("index.md"),
				"after index.md removes <rh-card> usage, the rebuilt cache "+
					"must no longer track index.md as depending on rh-card.js — "+
					"dependencies are re-tracked on each build from the current "+
					"onPageRendered output, not accumulated from previous "+
					"builds (issue #1100)")

			// rh-icon still used → still tracked
			iconPages := result2.Cache.PagesForDependency("elements/rh-icon/rh-icon.js")
			Expect(iconPages).To(ContainElement("index.md"),
				"index.md still uses <rh-icon> so the dependency must "+
					"persist in the rebuilt cache (issue #1100)")
		})
	})

	Describe("addDependencies from onContentTransformed (issue #1100)", func() {

		It("tracks dependencies from onContentTransformed return value", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home\nSome content with translations."),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Plugin that returns addDependencies from onContentTransformed.
			// Simulates an i18n plugin that reads translation files.
			pluginJS := `export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    return {
      html: page.html,
      toc: page.toc,
      frontMatter: page.frontMatter,
      addDependencies: ['locales/en.json', 'locales/shared.json']
    };
  });
}`
			Expect(os.WriteFile(filepath.Join(pluginsDir, "i18n-deps.js"),
				[]byte(pluginJS), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "ContentTransformed Dep Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Cache).NotTo(BeNil())

			enPages := result.Cache.PagesForDependency("locales/en.json")
			Expect(enPages).To(ConsistOf("index.md"),
				"addDependencies from onContentTransformed must be tracked "+
					"in the build cache — the i18n plugin declares that "+
					"index.md depends on locales/en.json for its content "+
					"transformation (issue #1100)")

			sharedPages := result.Cache.PagesForDependency("locales/shared.json")
			Expect(sharedPages).To(ConsistOf("index.md"),
				"all dependencies from onContentTransformed addDependencies "+
					"must be tracked (issue #1100)")
		})
	})

	Describe("addDependencies validation (issue #1100)", func() {

		setupWithPlugin := func(pluginJS string) *config.Config {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(pluginsDir, "test-plugin.js"),
				[]byte(pluginJS), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Validation Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)
			return cfg
		}

		It("ignores non-array addDependencies gracefully", func() {
			// Plugin returns addDependencies as a string instead of array.
			// The build must succeed — non-array is a plugin bug, but
			// shouldn't crash the build.
			pluginJS := `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html, addDependencies: 'not-an-array' };
  });
}`
			cfg := setupWithPlugin(pluginJS)
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"non-array addDependencies must not fail the build — "+
					"the value is malformed but the build should continue "+
					"with a warning (issue #1100)")

			// No dependencies should be tracked for this page
			pages := result.Cache.PagesForDependency("not-an-array")
			Expect(pages).To(BeNil(),
				"string addDependencies must not produce a dependency entry")
		})

		It("handles empty addDependencies array without error", func() {
			pluginJS := `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    return { html: page.html, addDependencies: [] };
  });
}`
			cfg := setupWithPlugin(pluginJS)
			result, err := pipeline.Build(cfg)
			Expect(err).NotTo(HaveOccurred(),
				"empty addDependencies array must not cause an error — "+
					"it means the plugin examined the page but found no "+
					"external dependencies (issue #1100)")
			Expect(result.Cache).NotTo(BeNil())
		})
	})

	// ── onFileChanged return value processing (issue #1100) ────────
	//
	// The onFileChanged hook changes from read-only to actionable.
	// When the plugin returns { invalidateByDependency: [...] }, the
	// dev server uses the cache's reverse index to determine which
	// pages to rebuild incrementally.

	Describe("onFileChanged invalidateByDependency integration (issue #1100)", func() {

		It("dependency-invalidated pages are rebuilt even when content unchanged", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home\n<rh-card>card</rh-card>"),
				0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(contentDir, "about.md"),
				[]byte("---\ntitle: About\nlayout: default\n---\n# About\nNo components."),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Plugin declares dependencies via onPageRendered
			pluginJS := `export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    if (page.html.indexOf('rh-card') !== -1) {
      return { html: page.html, addDependencies: ['elements/rh-card/rh-card.js'] };
    }
    return page;
  });
}`
			Expect(os.WriteFile(filepath.Join(pluginsDir, "ssr-plugin.js"),
				[]byte(pluginJS), 0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Invalidation Test",
				BaseURL:     "https://example.com",
				ProjectRoot: tmpDir,
				Build:       config.BuildConfig{Output: outputDir},
				Structure: config.StructureConfig{
					Content: "content",
					Layouts: "layouts",
				},
			}
			config.ApplyDefaults(cfg)

			registry, hooks, _ := pipeline.DiscoverPlugins(cfg)
			defer registry.Close()
			pipelineState, psErr := pipeline.InitPipelineState(cfg, registry, hooks)
			Expect(psErr).NotTo(HaveOccurred())

			// Build 1: initial
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// Verify dependency tracked
			Expect(result1.Cache.PagesForDependency("elements/rh-card/rh-card.js")).To(
				ConsistOf("index.md"),
				"sanity: dependency must be tracked after initial build")

			// Build 2: incremental — ONLY the dependency file changed.
			// No content files changed. The dependency path appears in
			// changedFiles (the dev server puts it there after processing
			// onFileChanged's invalidateByDependency return).
			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"elements/rh-card/rh-card.js"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			// index.md: depends on rh-card.js which changed → MUST rebuild
			Expect(result2.RenderedContent).To(HaveKey("index.md"),
				"index.md depends on elements/rh-card/rh-card.js which "+
					"appeared in changedFiles — the page must be re-rendered "+
					"even though its own content is unchanged. This is the "+
					"core mechanism of issue #1100: dependency files in "+
					"changedFiles trigger rebuilds of dependent pages via "+
					"the cache's reverse index (issue #1100)")
			Expect(result2.RenderedContent["index.md"]).To(
				ContainSubstring("rh-card"),
				"rendered output must contain the page's content")

			// about.md: no dependency on rh-card.js, content unchanged → skip
			Expect(result2.RenderedContent).NotTo(HaveKey("about.md"),
				"about.md has no dependency on rh-card.js and its content "+
					"hasn't changed — it must be skipped during incremental "+
					"rebuild (issue #1100)")
		})
	})
})
