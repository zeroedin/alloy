package assets_test

import (
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/assets"
)

// testdataDir returns the absolute path to the testdata directory
// relative to this test file.
func testdataDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "testdata")
}

var _ = Describe("Assets", func() {

	var outputDir string

	BeforeEach(func() {
		var err error
		outputDir, err = os.MkdirTemp("", "alloy-assets-test-*")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(outputDir)
	})

	// ── Copy ──────────────────────────────────────────────────────────

	Describe("CopyAssets", func() {
		It("copies all files from assets/ to output/ preserving structure", func() {
			assetsDir := filepath.Join(testdataDir(), "site-assets")
			err := assets.CopyAssets(assetsDir, outputDir)
			Expect(err).NotTo(HaveOccurred())

			// All three fixture files should exist in output
			Expect(filepath.Join(outputDir, "css", "main.css")).To(BeAnExistingFile())
			Expect(filepath.Join(outputDir, "js", "app.js")).To(BeAnExistingFile())
			Expect(filepath.Join(outputDir, "img", "hero.svg")).To(BeAnExistingFile())
		})

		It("preserves nested directory structure", func() {
			assetsDir := filepath.Join(testdataDir(), "site-assets")
			err := assets.CopyAssets(assetsDir, outputDir)
			Expect(err).NotTo(HaveOccurred())

			// Subdirectories must exist
			cssInfo, err := os.Stat(filepath.Join(outputDir, "css"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cssInfo.IsDir()).To(BeTrue())

			jsInfo, err := os.Stat(filepath.Join(outputDir, "js"))
			Expect(err).NotTo(HaveOccurred())
			Expect(jsInfo.IsDir()).To(BeTrue())

			imgInfo, err := os.Stat(filepath.Join(outputDir, "img"))
			Expect(err).NotTo(HaveOccurred())
			Expect(imgInfo.IsDir()).To(BeTrue())
		})

		It("returns no error when assets directory does not exist", func() {
			err := assets.CopyAssets("/nonexistent/assets/dir", outputDir)
			Expect(err).NotTo(HaveOccurred())
		})

		It("copies file content byte-identical with no transformation", func() {
			assetsDir := filepath.Join(testdataDir(), "site-assets")
			err := assets.CopyAssets(assetsDir, outputDir)
			Expect(err).NotTo(HaveOccurred())

			// Read source and output, compare byte-for-byte
			srcCSS, err := os.ReadFile(filepath.Join(assetsDir, "css", "main.css"))
			Expect(err).NotTo(HaveOccurred())

			outCSS, err := os.ReadFile(filepath.Join(outputDir, "css", "main.css"))
			Expect(err).NotTo(HaveOccurred())

			Expect(outCSS).To(Equal(srcCSS), "output must be byte-identical to source")
		})
	})

	// ── Hook integration (onAssetProcess) ─────────────────────────────
	//
	// SPEC GAP: The spec (S7) defines no built-in ignore/exclude mechanism
	// for assets. All filtering is delegated to the onAssetProcess hook.
	//
	// PROPOSAL for future spec revision: Add a config option such as
	//   build:
	//     assets:
	//       exclude: ["_*", ".DS_Store", "*.map", "*.scss"]
	// to allow simple glob-based exclusion of build artifacts, partials,
	// and OS metadata files without requiring a plugin. Common SSGs
	// (Hugo, 11ty, Jekyll) all provide this. Requiring a plugin for
	// basic ignore patterns like .DS_Store is unnecessary friction.
	//
	// Until then, plugins can skip files by returning unchanged or by
	// convention (e.g., a community "asset-ignore" plugin).

	Describe("ProcessAssets (onAssetProcess hook)", func() {
		It("calls hook function for each asset file", func() {
			assetsDir := filepath.Join(testdataDir(), "site-assets")

			var processed []string
			hookFn := func(f assets.AssetFile) (assets.AssetFile, error) {
				processed = append(processed, f.Path)
				return f, nil
			}

			err := assets.ProcessAssets(assetsDir, outputDir, hookFn)
			Expect(err).NotTo(HaveOccurred())

			// Hook should be called for all 3 fixture files
			Expect(processed).To(HaveLen(3))
			Expect(processed).To(ContainElements(
				"css/main.css",
				"js/app.js",
				"img/hero.svg",
			))
		})

		It("hook can modify file content before writing", func() {
			assetsDir := filepath.Join(testdataDir(), "site-assets")

			hookFn := func(f assets.AssetFile) (assets.AssetFile, error) {
				if f.Path == "css/main.css" {
					// Simulate a minification plugin
					f.Content = []byte("body{margin:0}")
				}
				return f, nil
			}

			err := assets.ProcessAssets(assetsDir, outputDir, hookFn)
			Expect(err).NotTo(HaveOccurred())

			outCSS, err := os.ReadFile(filepath.Join(outputDir, "css", "main.css"))
			Expect(err).NotTo(HaveOccurred())
			Expect(string(outCSS)).To(Equal("body{margin:0}"))
		})

		It("copies files unchanged when no hook is registered", func() {
			assetsDir := filepath.Join(testdataDir(), "site-assets")

			// nil hookFn = no plugins = verbatim copy
			err := assets.ProcessAssets(assetsDir, outputDir, nil)
			Expect(err).NotTo(HaveOccurred())

			srcCSS, err := os.ReadFile(filepath.Join(assetsDir, "css", "main.css"))
			Expect(err).NotTo(HaveOccurred())

			outCSS, err := os.ReadFile(filepath.Join(outputDir, "css", "main.css"))
			Expect(err).NotTo(HaveOccurred())

			Expect(outCSS).To(Equal(srcCSS), "without a hook, files must copy verbatim")
		})
	})

	// ── URL resolution ────────────────────────────────────────────────

	Describe("ResolveURL", func() {
		It("resolves asset path relative to baseURL", func() {
			result := assets.ResolveURL("css/main.css", "https://example.com")
			Expect(result).To(Equal("https://example.com/css/main.css"))
		})

		It("handles baseURL with trailing slash", func() {
			result := assets.ResolveURL("css/main.css", "https://example.com/")
			Expect(result).To(Equal("https://example.com/css/main.css"))
		})

		It("handles baseURL with subpath", func() {
			result := assets.ResolveURL("js/app.js", "https://example.com/blog")
			Expect(result).To(Equal("https://example.com/blog/js/app.js"))
		})

		It("handles root-relative baseURL", func() {
			result := assets.ResolveURL("img/hero.svg", "/")
			Expect(result).To(Equal("/img/hero.svg"))
		})
	})
})
