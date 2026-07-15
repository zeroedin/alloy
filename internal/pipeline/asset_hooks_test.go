package pipeline_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// Spec reference: PLAN.md §5 Lifecycle Events — Per-asset hook (path + content object)
//
// onAssetProcess fires once per asset file during asset copy. Payload is a
// JSON object with `path` (relative within assets dir) and `content` (file
// content as string). Return value's "content" key replaces the asset content.
//
// IMPLEMENTATION.md §Hook payload contract — Per-asset hook (onAssetProcess):
// fire once per asset with HookAssetPayload{Path: relPath, Content: fileContent}.
// Return value's "content" key replaces the asset content.
//
// Currently broken (issue #974): build.go fires onAssetProcess once per build
// with {assetsDir, outputDir} directory paths and discards the return value.
// These tests encode the documented per-asset contract.

var _ = Describe("onAssetProcess per-asset dispatch (issue #974)", func() {

	// ── Payload shape ────────────────────────────────────────────────
	// Each invocation must receive { path: string, content: string },
	// not the directory-level { assetsDir, outputDir } payload.

	It("receives per-asset payload with path and content keys, not directory paths", func() {
		cfg := &config.Config{
			Title:   "Asset Hook Shape Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"assets/css/main.css":    "body { color: red; }",
			"plugins/check-shape.js": `export default function(alloy) {
  alloy.hook('onAssetProcess', {}, (asset) => {
    if (typeof asset !== 'object' || asset === null) {
      throw new Error('payload must be an object, got ' + typeof asset);
    }
    if (asset.assetsDir !== undefined || asset.outputDir !== undefined) {
      throw new Error('payload contains directory paths (assetsDir/outputDir) — must be per-asset {path, content}, not directory-level');
    }
    if (typeof asset.path !== 'string') {
      throw new Error('asset.path must be a string, got ' + typeof asset.path);
    }
    if (typeof asset.content !== 'string') {
      throw new Error('asset.content must be a string, got ' + typeof asset.content);
    }
    return asset;
  });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"onAssetProcess must receive per-asset {path, content} payload — "+
				"if this fails with 'directory paths' error, build.go still sends "+
				"{assetsDir, outputDir} instead of per-asset dispatch (issue #974)")
	})

	// ── Per-asset dispatch (one invocation per file) ─────────────────
	// The hook must fire once per asset file, not once per build.

	It("fires once per asset file, not once per build", func() {
		cfg := &config.Config{
			Title:   "Per-Asset Dispatch Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"assets/css/main.css":    "body { margin: 0; }",
			"assets/js/app.js":       "console.log('hello');",
			"assets/data.json":       `{"key":"value"}`,
			"plugins/count-calls.js": `export default function(alloy) {
  let callCount = 0;
  const seenPaths = [];
  alloy.hook('onAssetProcess', {}, (asset) => {
    callCount++;
    seenPaths.push(asset.path);
    if (callCount > 3) {
      throw new Error('onAssetProcess called more than 3 times for 3 assets — paths seen: ' + seenPaths.join(', '));
    }
    return asset;
  });

  alloy.hook('onBuildComplete', {}, (stats) => {
    if (callCount < 3) {
      throw new Error('onAssetProcess called only ' + callCount + ' time(s) for 3 asset files — must fire once per asset. Paths seen: ' + seenPaths.join(', '));
    }
    return stats;
  });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"onAssetProcess must fire once per asset file — "+
				"if this fails with 'called only 1 time(s)', build.go fires once "+
				"per build instead of per-asset (issue #974)")
	})

	// ── Path is relative within assets directory ─────────────────────
	// Payload path must be relative to the assets dir (e.g. "css/main.css"),
	// not the full filesystem path.

	It("path is relative within the assets directory", func() {
		cfg := &config.Config{
			Title:   "Relative Path Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"assets/css/main.css":    "body { color: blue; }",
			"plugins/check-path.js": `export default function(alloy) {
  alloy.hook('onAssetProcess', {}, (asset) => {
    if (asset.path.includes('/assets/') || asset.path.startsWith('/')) {
      throw new Error('asset.path must be relative within assets dir, got: ' + asset.path);
    }
    if (asset.path !== 'css/main.css') {
      throw new Error('expected path "css/main.css", got: ' + asset.path);
    }
    return asset;
  });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"asset.path must be relative within the assets directory (e.g. 'css/main.css'), "+
				"not a full filesystem path")
	})

	// ── Content matches source file ──────────────────────────────────
	// The content field must contain the actual file content.

	It("content field contains the actual file content", func() {
		cfg := &config.Config{
			Title:   "Content Match Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"assets/css/main.css":    "body { color: green; font-size: 16px; }",
			"plugins/check-content.js": `export default function(alloy) {
  let verified = false;
  alloy.hook('onAssetProcess', {}, (asset) => {
    if (typeof asset.path !== 'string') {
      throw new Error('asset.path must be a string, got ' + typeof asset.path);
    }
    if (asset.path === 'css/main.css') {
      const expected = 'body { color: green; font-size: 16px; }';
      if (asset.content !== expected) {
        throw new Error('content mismatch for ' + asset.path + ': expected "' + expected + '", got "' + asset.content + '"');
      }
      verified = true;
    }
    return asset;
  });
  alloy.hook('onBuildComplete', {}, (stats) => {
    if (!verified) {
      throw new Error('onAssetProcess was never called with path "css/main.css" — hook must fire per-asset with relative path');
    }
    return stats;
  });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"asset.content must contain the actual file content as a string — "+
				"if this fails with 'never called with path', build.go sends directory "+
				"paths instead of per-asset payload (issue #974)")
	})

	// ── Return value replaces asset content in output ─────────────────
	// The returned {content} must replace the file content written to the
	// output directory. Uses manual temp dir + Build() to verify output.

	It("returned content replaces the asset file in the output directory", func() {
		tmpDir := GinkgoT().TempDir()
		contentDir := filepath.Join(tmpDir, "content")
		layoutDir := filepath.Join(tmpDir, "layouts")
		assetsDir := filepath.Join(tmpDir, "assets", "css")
		pluginsDir := filepath.Join(tmpDir, "plugins")
		outputDir := filepath.Join(tmpDir, "_site")

		Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(assetsDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

		Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
			[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
			[]byte("<html><body>{{ content }}</body></html>"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(assetsDir, "main.css"),
			[]byte("body { color: red; margin: 8px; }"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(pluginsDir, "minify.js"),
			[]byte(`export default function(alloy) {
  alloy.hook('onAssetProcess', {}, (asset) => {
    if (asset.path.endsWith('.css')) {
      return { content: 'body{color:red;margin:0}' };
    }
    return asset;
  });
}`), 0644)).To(Succeed())

		cfg := &config.Config{
			Title:       "Asset Transform Test",
			BaseURL:     "https://example.com",
			ProjectRoot: tmpDir,
			Build:       config.BuildConfig{Output: outputDir},
			Structure: config.StructureConfig{
				Content: "content",
				Layouts: "layouts",
				Assets:  "assets",
				Plugins: "plugins",
			},
		}

		_, err := pipeline.Build(cfg)
		Expect(err).NotTo(HaveOccurred())

		outCSS, err := os.ReadFile(filepath.Join(outputDir, "css", "main.css"))
		Expect(err).NotTo(HaveOccurred(),
			"transformed asset must exist in output directory")
		Expect(string(outCSS)).To(Equal("body{color:red;margin:0}"),
			"onAssetProcess return value must replace asset content in output — "+
				"currently build.go discards the return value (issue #974)")
	})

	// ── Unmodified return preserves content ──────────────────────────
	// When a plugin returns the asset unchanged, output must match source.

	It("preserves content when plugin returns asset unchanged", func() {
		tmpDir := GinkgoT().TempDir()
		contentDir := filepath.Join(tmpDir, "content")
		layoutDir := filepath.Join(tmpDir, "layouts")
		assetsDir := filepath.Join(tmpDir, "assets", "js")
		pluginsDir := filepath.Join(tmpDir, "plugins")
		outputDir := filepath.Join(tmpDir, "_site")

		Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(assetsDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

		jsContent := "const greeting = 'hello world';\nconsole.log(greeting);\n"
		Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
			[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
			[]byte("<html><body>{{ content }}</body></html>"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(assetsDir, "app.js"),
			[]byte(jsContent), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(pluginsDir, "passthrough.js"),
			[]byte(`export default function(alloy) {
  let hookFired = false;
  alloy.hook('onAssetProcess', {}, (asset) => {
    if (typeof asset.path !== 'string' || typeof asset.content !== 'string') {
      throw new Error('per-asset payload must have string path and content fields');
    }
    hookFired = true;
    return asset;
  });
  alloy.hook('onBuildComplete', {}, (stats) => {
    if (!hookFired) {
      throw new Error('onAssetProcess hook was never called with per-asset payload');
    }
    return stats;
  });
}`), 0644)).To(Succeed())

		cfg := &config.Config{
			Title:       "Asset Passthrough Test",
			BaseURL:     "https://example.com",
			ProjectRoot: tmpDir,
			Build:       config.BuildConfig{Output: outputDir},
			Structure: config.StructureConfig{
				Content: "content",
				Layouts: "layouts",
				Assets:  "assets",
				Plugins: "plugins",
			},
		}

		_, err := pipeline.Build(cfg)
		Expect(err).NotTo(HaveOccurred(),
			"passthrough hook must not error — if this fails with "+
				"'per-asset payload must have string path and content fields', "+
				"build.go sends directory paths instead of per-asset dispatch (issue #974)")

		outJS, err := os.ReadFile(filepath.Join(outputDir, "js", "app.js"))
		Expect(err).NotTo(HaveOccurred(),
			"asset must exist in output directory even with passthrough hook")
		Expect(string(outJS)).To(Equal(jsContent),
			"when plugin returns asset unchanged, output must be byte-identical to source")
	})

	// ── Hook error halts the build ───────────────────────────────────
	// An error thrown by the hook must propagate as a build error.

	It("hook error halts the build with descriptive error message", func() {
		cfg := &config.Config{
			Title:   "Asset Hook Error Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"assets/broken.css":      "invalid { content",
			"plugins/fail-hook.js": `export default function(alloy) {
  alloy.hook('onAssetProcess', {}, (asset) => {
    if (asset.path === 'broken.css') {
      throw new Error('CSS parse error in ' + asset.path);
    }
    return asset;
  });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).To(HaveOccurred(),
			"onAssetProcess hook errors must halt the build")
		Expect(err.Error()).To(ContainSubstring("onAssetProcess"),
			"error message must reference the hook name for debugging")
	})

	// ── Selective transformation by extension ────────────────────────
	// Plugin can filter by path extension, transforming only matching files.
	// Non-matching files pass through unchanged.

	It("plugin can selectively transform by file extension", func() {
		tmpDir := GinkgoT().TempDir()
		contentDir := filepath.Join(tmpDir, "content")
		layoutDir := filepath.Join(tmpDir, "layouts")
		cssDir := filepath.Join(tmpDir, "assets", "css")
		jsDir := filepath.Join(tmpDir, "assets", "js")
		pluginsDir := filepath.Join(tmpDir, "plugins")
		outputDir := filepath.Join(tmpDir, "_site")

		Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(cssDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(jsDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

		cssContent := "body { color: red; }"
		jsContent := "const x = 1;"
		Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
			[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
			[]byte("<html><body>{{ content }}</body></html>"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(cssDir, "style.css"),
			[]byte(cssContent), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(jsDir, "app.js"),
			[]byte(jsContent), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(pluginsDir, "css-only.js"),
			[]byte(`export default function(alloy) {
  alloy.hook('onAssetProcess', {}, (asset) => {
    if (asset.path.endsWith('.css')) {
      return { content: '/* minified */' + asset.content.replace(/\s+/g, '') };
    }
    return asset;
  });
}`), 0644)).To(Succeed())

		cfg := &config.Config{
			Title:       "Selective Transform Test",
			BaseURL:     "https://example.com",
			ProjectRoot: tmpDir,
			Build:       config.BuildConfig{Output: outputDir},
			Structure: config.StructureConfig{
				Content: "content",
				Layouts: "layouts",
				Assets:  "assets",
				Plugins: "plugins",
			},
		}

		_, err := pipeline.Build(cfg)
		Expect(err).NotTo(HaveOccurred())

		outCSS, err := os.ReadFile(filepath.Join(outputDir, "css", "style.css"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outCSS)).To(HavePrefix("/* minified */"),
			"CSS file must be transformed by the hook")
		Expect(string(outCSS)).NotTo(Equal(cssContent),
			"CSS file must not match the original (transform was applied)")

		outJS, err := os.ReadFile(filepath.Join(outputDir, "js", "app.js"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(outJS)).To(Equal(jsContent),
			"JS file must pass through unchanged when hook only transforms .css files")
	})

	// ── No assets directory ──────────────────────────────────────────
	// When no assets/ directory exists, hook does not fire and build succeeds.

	It("build succeeds without error when no assets directory exists", func() {
		cfg := &config.Config{
			Title:   "No Assets Test",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		contentMap := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\n# Home",
			"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			"plugins/asset-check.js": `export default function(alloy) {
  let called = false;
  alloy.hook('onAssetProcess', {}, (asset) => {
    called = true;
    return asset;
  });
  alloy.hook('onBuildComplete', {}, (stats) => {
    if (called) {
      throw new Error('onAssetProcess should not fire when no assets directory exists');
    }
    return stats;
  });
}`,
		}
		_, err := pipeline.BuildWithContent(cfg, contentMap)
		Expect(err).NotTo(HaveOccurred(),
			"build must succeed when no assets directory exists — "+
				"onAssetProcess must not fire for nonexistent assets dir")
	})

	// ── Empty return content writes empty file ───────────────────────
	// A plugin returning { content: "" } should write an empty file.

	It("writes empty file when plugin returns empty content string", func() {
		tmpDir := GinkgoT().TempDir()
		contentDir := filepath.Join(tmpDir, "content")
		layoutDir := filepath.Join(tmpDir, "layouts")
		assetsDir := filepath.Join(tmpDir, "assets")
		pluginsDir := filepath.Join(tmpDir, "plugins")
		outputDir := filepath.Join(tmpDir, "_site")

		Expect(os.MkdirAll(contentDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(layoutDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(assetsDir, 0755)).To(Succeed())
		Expect(os.MkdirAll(pluginsDir, 0755)).To(Succeed())

		Expect(os.WriteFile(filepath.Join(contentDir, "index.md"),
			[]byte("---\ntitle: Home\nlayout: default\n---\n# Home"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(layoutDir, "default.liquid"),
			[]byte("<html><body>{{ content }}</body></html>"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(assetsDir, "empty-me.txt"),
			[]byte("this content will be cleared"), 0644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(pluginsDir, "clear.js"),
			[]byte(`export default function(alloy) {
  alloy.hook('onAssetProcess', {}, (asset) => {
    return { content: '' };
  });
}`), 0644)).To(Succeed())

		cfg := &config.Config{
			Title:       "Empty Content Test",
			BaseURL:     "https://example.com",
			ProjectRoot: tmpDir,
			Build:       config.BuildConfig{Output: outputDir},
			Structure: config.StructureConfig{
				Content: "content",
				Layouts: "layouts",
				Assets:  "assets",
				Plugins: "plugins",
			},
		}

		_, err := pipeline.Build(cfg)
		Expect(err).NotTo(HaveOccurred())

		outFile, err := os.ReadFile(filepath.Join(outputDir, "empty-me.txt"))
		Expect(err).NotTo(HaveOccurred(),
			"asset file must still exist in output even with empty content")
		Expect(string(outFile)).To(BeEmpty(),
			"plugin returning { content: '' } must write an empty file — "+
				"return value must not be discarded (issue #974)")
	})
})
