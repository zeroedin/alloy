package pipeline_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Build Pipeline", func() {

	// ── BuildIncremental lifecycle hook dispatch (issue #731) ─────────
	// Build() fires all lifecycle hooks. BuildIncremental skips most of
	// them, causing SSR, content transforms, cascade mutations, and
	// build-complete side effects to silently regress on incremental
	// rebuilds.

	Describe("BuildIncremental lifecycle hook dispatch (issue #731)", func() {

		It("onContentLoaded fires during BuildIncremental", func() {
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
				[]byte("<html><body><p>{{ page.injectedByHook }}</p>{{ content }}</body></html>"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(pluginsDir, "inject-field.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].frontMatter.injectedByHook = 'content-loaded-ran';
    }
    return pages;
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Incremental Hook Test",
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

			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result1.RenderedContent["index.md"]).To(ContainSubstring("content-loaded-ran"),
				"onContentLoaded must fire during BuildIncremental — "+
					"the hook injects a frontMatter field that should appear in "+
					"the rendered output. Currently BuildIncremental skips "+
					"onContentLoaded entirely (issue #731)")

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home Updated\nlayout: default\n---\n# Home Updated"),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result2.RenderedContent["index.md"]).To(ContainSubstring("content-loaded-ran"),
				"onContentLoaded must fire on subsequent incremental rebuilds too — "+
					"not just the first one (issue #731)")
		})

		It("onDataCascadeReady fires during BuildIncremental", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: About\nlayout: default\n---\n# About"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body><p>{{ page.computedLabel }}</p>{{ content }}</body></html>"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(pluginsDir, "cascade-enrich.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onDataCascadeReady', { pages: true }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].data.computedLabel = 'cascade-' + (pages[i].data.title || 'unknown');
    }
    return pages;
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Cascade Hook Test",
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

			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result1.RenderedContent["index.md"]).To(ContainSubstring("cascade-About"),
				"onDataCascadeReady must fire during BuildIncremental — "+
					"the hook computes a label from cascade data that should "+
					"appear in the rendered output. Currently BuildIncremental "+
					"skips onDataCascadeReady entirely (issue #731)")

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: About Updated\nlayout: default\n---\n# About Updated"),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result2.RenderedContent["index.md"]).To(ContainSubstring("cascade-About Updated"),
				"onDataCascadeReady must fire on subsequent incremental rebuilds "+
					"with cache, not just the nil-cache path (issue #731)")
		})

		It("onContentTransformed fires during BuildIncremental", func() {
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

			Expect(os.WriteFile(filepath.Join(pluginsDir, "wrap-html.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onContentTransformed', {}, function(page) {
    page.html = '<div class="transformed">' + page.html + '</div>';
    return page;
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Content Transform Test",
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

			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result1.RenderedContent["index.md"]).To(ContainSubstring(`class="transformed"`),
				"onContentTransformed must fire during BuildIncremental — "+
					"the hook wraps content HTML in a div that should appear "+
					"in the rendered output. Currently BuildIncremental skips "+
					"onContentTransformed entirely (issue #731)")

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home Updated\nlayout: default\n---\n# Home Updated"),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result2.RenderedContent["index.md"]).To(ContainSubstring(`class="transformed"`),
				"onContentTransformed must fire on subsequent incremental rebuilds "+
					"with cache, not just the nil-cache path (issue #731)")
		})

		It("onPageRendered fires during BuildIncremental", func() {
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

			Expect(os.WriteFile(filepath.Join(pluginsDir, "ssr-marker.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    page.html = page.html + '<!-- ssr-marker -->';
    return page;
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Page Rendered Test",
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

			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result1.RenderedContent["index.md"]).To(ContainSubstring("<!-- ssr-marker -->"),
				"onPageRendered must fire during BuildIncremental — "+
					"the hook appends an SSR marker comment that should appear "+
					"in the final rendered output. This is the most visible "+
					"symptom of issue #731: Lit SSR and other post-render "+
					"transforms are skipped on incremental rebuilds")

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home Updated\nlayout: default\n---\n# Home Updated"),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result2.RenderedContent["index.md"]).To(ContainSubstring("<!-- ssr-marker -->"),
				"onPageRendered must fire on subsequent incremental rebuilds "+
					"with cache, not just the nil-cache path (issue #731)")
		})

		// ── BuildIncremental onPageRendered field-level payload verification (issue #1120) ──
		// The existing incremental test only checks that the hook fires (marker
		// is present). This test validates all 4 payload fields — html, frontMatter,
		// url, path — are correctly populated at the field level during
		// BuildIncremental, matching the contract defined in Build().

		It("BuildIncremental onPageRendered receives object payload with all fields at correct values", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			blogDir := filepath.Join(contentDir, "blog")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(blogDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(blogDir, "hello.md"),
				[]byte("---\ntitle: Hello World\nlayout: default\nauthor: Alice\n---\n# Hello"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Plugin validates all 4 payload fields at the field level.
			// Checks types and shapes (not specific values) so it works across
			// both the initial and incremental builds. Appends a marker with the
			// actual values so the test can assert on the final output.
			Expect(os.WriteFile(filepath.Join(pluginsDir, "field-check.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onPageRendered', {}, function(page) {
    if (typeof page !== 'object' || page === null) {
      throw new Error('payload must be object, got ' + typeof page);
    }
    if (typeof page.html !== 'string' || page.html.length === 0) {
      throw new Error('html must be non-empty string, got ' + typeof page.html);
    }
    if (typeof page.url !== 'string' || page.url.length === 0) {
      throw new Error('url must be non-empty string, got ' + typeof page.url);
    }
    if (typeof page.path !== 'string' || page.path.length === 0) {
      throw new Error('path must be non-empty string, got ' + typeof page.path);
    }
    if (!page.frontMatter || typeof page.frontMatter !== 'object') {
      throw new Error('frontMatter must be object, got ' + typeof page.frontMatter);
    }
    if (typeof page.frontMatter.title !== 'string' || page.frontMatter.title.length === 0) {
      throw new Error('frontMatter.title must be non-empty string, got ' + JSON.stringify(page.frontMatter.title));
    }
    if (page.frontMatter.author !== 'Alice') {
      throw new Error('frontMatter.author must be "Alice", got ' + JSON.stringify(page.frontMatter.author));
    }
    if (!page.path.includes('hello')) {
      throw new Error('path must contain "hello", got ' + page.path);
    }
    if (!page.url.includes('hello')) {
      throw new Error('url must contain "hello", got ' + page.url);
    }
    page.html = page.html + '<!-- fields-ok:' + page.frontMatter.title + ':' + page.path + ':' + page.url + ' -->';
    return page;
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Incremental Field Check",
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

			// First build (nil cache path)
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred(),
				"BuildIncremental must not error — if this fails with a field "+
					"validation error from the plugin, the onPageRendered payload "+
					"is not correctly populated in BuildIncremental (issue #1120)")

			html1 := result1.RenderedContent["blog/hello.md"]
			Expect(html1).To(ContainSubstring("<!-- fields-ok:Hello World:"),
				"plugin must have validated all payload fields and appended the "+
					"marker with frontMatter.title — proves field-level correctness "+
					"in BuildIncremental nil-cache path (issue #1120)")
			Expect(html1).To(ContainSubstring("hello"),
				"marker must contain 'hello' from both path and url fields")

			// Second build (cache path — content changed)
			Expect(os.WriteFile(filepath.Join(blogDir, "hello.md"),
				[]byte("---\ntitle: Hello Updated\nlayout: default\nauthor: Alice\n---\n# Hello Updated"),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/blog/hello.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred(),
				"BuildIncremental with cache must not error on second build — "+
					"field-level payload must still be correct (issue #1120)")

			html2 := result2.RenderedContent["blog/hello.md"]
			Expect(html2).To(ContainSubstring("<!-- fields-ok:Hello Updated:"),
				"plugin must see updated frontMatter.title on incremental rebuild — "+
					"proves BuildIncremental re-reads front matter before building "+
					"the onPageRendered payload (issue #1120)")
		})

		It("onBuildComplete fires during BuildIncremental", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")
			markerFile := filepath.Join(tmpDir, "build-complete.marker")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(pluginsDir, "build-complete.js"),
				[]byte(fmt.Sprintf(`export const runtime = "node";
import { writeFileSync } from 'fs';
export default function(alloy) {
  alloy.hook('onBuildComplete', {}, function(result) {
    writeFileSync(%q, 'fired', 'utf8');
    return result;
  });
}`, markerFile)),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Build Complete Test",
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

			_, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(markerFile).To(BeAnExistingFile(),
				"onBuildComplete must fire during BuildIncremental — "+
					"the hook writes a sentinel file that should exist after "+
					"the build completes. If missing, BuildIncremental is not "+
					"dispatching onBuildComplete at all (issue #731)")
		})
	})

	// ── BuildIncremental onContentLoaded html merge-back (issue #1050) ──
	// The onContentLoaded html merge-back fix (issue #976) was applied to
	// both Build() and BuildIncremental(). The Build() path has 6 spec
	// tests in hooks_test.go. These tests cover the incremental path
	// (incremental.go) which has different error handling (warning-only
	// vs fatal errors) and uses pagesToRender instead of pages.

	Describe("BuildIncremental onContentLoaded html merge-back (issue #1050)", func() {

		It("html-only mutation (no frontMatter) applied via BuildIncremental", func() {
			tmpDir := GinkgoT().TempDir()
			contentDir := filepath.Join(tmpDir, "content")
			layoutsDir := filepath.Join(tmpDir, "layouts")
			pluginsDir := filepath.Join(tmpDir, "plugins")
			outputDir := filepath.Join(tmpDir, "_site")

			Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(layoutsDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home\nlayout: default\n---\n# Home\n\nOriginal body."),
				0644)).To(Succeed())

			Expect(os.WriteFile(filepath.Join(layoutsDir, "default.liquid"),
				[]byte("<html><body>{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Plugin returns sparse entries: only path + html, no frontMatter key.
			// This catches implementations that gate html merge-back on
			// frontMatter presence (returnedPath inside frontMatter block).
			Expect(os.WriteFile(filepath.Join(pluginsDir, "html-only.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    var result = [];
    for (var i = 0; i < pages.length; i++) {
      result.push({
        path: pages[i].path,
        html: pages[i].html + '<div class="incremental-html-only">Injected without frontMatter</div>'
      });
    }
    return result;
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Incremental HTML-Only Test",
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

			// First build (nil cache, nil changedFiles — full build through incremental path)
			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result1.RenderedContent["index.md"]).To(
				ContainSubstring(`<div class="incremental-html-only">Injected without frontMatter</div>`),
				"html-only return entries must be applied in BuildIncremental — "+
					"if this fails, the incremental merge-back loop gates html "+
					"application on frontMatter presence (issue #1050)")

			// Second build (with cache and changed file — true incremental rebuild)
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home Updated\nlayout: default\n---\n# Home Updated\n\nChanged body."),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(result2.RenderedContent["index.md"]).To(
				ContainSubstring(`<div class="incremental-html-only">Injected without frontMatter</div>`),
				"html-only merge-back must work on subsequent incremental rebuilds "+
					"with cache, not just the nil-cache path (issue #1050)")
		})

		It("combined html and frontMatter mutation via BuildIncremental", func() {
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
				[]byte("<html><body><h1>{{ page.title }}</h1>{{ content }}</body></html>"),
				0644)).To(Succeed())

			// Plugin mutates both frontMatter and html in the same hook call.
			Expect(os.WriteFile(filepath.Join(pluginsDir, "mutate-both.js"),
				[]byte(`export default function(alloy) {
  alloy.hook('onContentLoaded', { pages: true, pageFields: ["*"] }, function(pages) {
    for (var i = 0; i < pages.length; i++) {
      pages[i].frontMatter.title = pages[i].frontMatter.title + ' (enriched)';
      pages[i].html = pages[i].html + '<span class="incremental-watermark">Processed</span>';
    }
    return pages;
  });
}`),
				0644)).To(Succeed())

			cfg := &config.Config{
				Title:       "Incremental Both Test",
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

			result1, err := pipeline.BuildIncremental(cfg, nil, nil, nil,
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			html1 := result1.RenderedContent["index.md"]
			Expect(html1).To(ContainSubstring("Home (enriched)"),
				"frontMatter.title mutation must be applied in BuildIncremental (issue #1050)")
			Expect(html1).To(ContainSubstring(`<span class="incremental-watermark">Processed</span>`),
				"html mutation must ALSO be applied in the same BuildIncremental hook call — "+
					"both frontMatter and html must be merged back from the return value (issue #1050)")

			// Incremental rebuild with cache
			Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
				[]byte("---\ntitle: Home v2\nlayout: default\n---\n# Home v2"),
				0644)).To(Succeed())

			result2, err := pipeline.BuildIncremental(cfg, nil, result1.Cache,
				[]string{"content/index.md"},
				pipeline.BuildOptions{PipelineState: pipelineState, CaptureRenderedContent: true})
			Expect(err).NotTo(HaveOccurred())

			html2 := result2.RenderedContent["index.md"]
			Expect(html2).To(ContainSubstring("Home v2 (enriched)"),
				"frontMatter mutation must work on incremental rebuilds with cache (issue #1050)")
			Expect(html2).To(ContainSubstring(`<span class="incremental-watermark">Processed</span>`),
				"html mutation must work on incremental rebuilds with cache (issue #1050)")
		})
	})
})
