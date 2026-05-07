package static_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/static"
)

var _ = Describe("Static file handling", func() {

	Describe("CopyStatic", func() {

		Context("static/ directory", func() {
			It("copies all files from static/ to output root", func() {
				err := static.CopyStatic("testdata/static", "testdata/output")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles empty static directory gracefully", func() {
				err := static.CopyStatic("testdata/empty-static", "testdata/output")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles missing static directory gracefully", func() {
				err := static.CopyStatic("testdata/nonexistent", "testdata/output")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	Describe("CopyPassthrough", func() {

		Context("passthrough copy", func() {
			It("copies from external directory to configured output path", func() {
				mappings := []config.PassthroughMapping{
					{From: "testdata/external/fonts", To: "assets/fonts"},
				}
				err := static.CopyPassthrough(mappings, "testdata/project", "testdata/output")
				Expect(err).NotTo(HaveOccurred())
			})

			It("handles multiple passthrough mappings", func() {
				mappings := []config.PassthroughMapping{
					{From: "testdata/external/fonts", To: "assets/fonts"},
					{From: "testdata/external/images", To: "assets/images"},
				}
				err := static.CopyPassthrough(mappings, "testdata/project", "testdata/output")
				Expect(err).NotTo(HaveOccurred())
			})

			It("returns error when 'from' path does not exist", func() {
				mappings := []config.PassthroughMapping{
					{From: "testdata/nonexistent/path", To: "assets/missing"},
				}
				err := static.CopyPassthrough(mappings, "testdata/project", "testdata/output")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	// ── Concurrent copy correctness (issue #511) ────────────────────
	// These tests create enough files to exercise the bounded worker pool.
	// Run with -race to detect data races in concurrent copy paths.

	Describe("Concurrent copy correctness", func() {
		var srcDir, dstDir string
		const fileCount = 200

		BeforeEach(func() {
			var err error
			srcDir, err = os.MkdirTemp("", "alloy-copy-src-*")
			Expect(err).NotTo(HaveOccurred())
			dstDir, err = os.MkdirTemp("", "alloy-copy-dst-*")
			Expect(err).NotTo(HaveOccurred())

			// Create fileCount files across nested directories
			for i := 0; i < fileCount; i++ {
				subdir := filepath.Join(srcDir, fmt.Sprintf("dir-%d", i%10))
				Expect(os.MkdirAll(subdir, 0755)).To(Succeed())
				name := filepath.Join(subdir, fmt.Sprintf("file-%d.txt", i))
				err := os.WriteFile(name, []byte(fmt.Sprintf("content-%d", i)), 0644)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			os.RemoveAll(srcDir)
			os.RemoveAll(dstDir)
		})

		It("copies all files correctly with CopyStatic", func() {
			err := static.CopyStatic(srcDir, dstDir)
			Expect(err).NotTo(HaveOccurred())

			// Verify every file was copied with correct content
			for i := 0; i < fileCount; i++ {
				rel := filepath.Join(fmt.Sprintf("dir-%d", i%10), fmt.Sprintf("file-%d.txt", i))
				dst := filepath.Join(dstDir, rel)
				data, err := os.ReadFile(dst)
				Expect(err).NotTo(HaveOccurred(), "file %s must exist", rel)
				Expect(string(data)).To(Equal(fmt.Sprintf("content-%d", i)))
			}
		})

		It("first error cancels remaining work", func() {
			// Remove read permission on a file mid-tree to trigger an error
			badFile := filepath.Join(srcDir, "dir-0", "file-0.txt")
			err := os.Chmod(badFile, 0000)
			Expect(err).NotTo(HaveOccurred())

			err = static.CopyStatic(srcDir, dstDir)
			Expect(err).To(HaveOccurred(), "copy must propagate file errors")

			// Restore permission for cleanup
			_ = os.Chmod(badFile, 0644)
		})
	})

	// ── Passthrough overlap protection (§1h) ─────────────────────────

	Describe("Passthrough overlap protection", func() {
		It("silently ignores passthrough from managed directory", func() {
			// Passthrough from: pointing to a managed directory (content, layouts, assets, static, data)
			// must be silently ignored per spec
			mappings := []config.PassthroughMapping{
				{From: "content", To: "extra-content"},
			}
			managedDirs := []string{"content", "layouts", "assets", "static", "data"}
			err := static.CopyPassthroughWithValidation(mappings, "testdata/project", "testdata/output", managedDirs)
			Expect(err).NotTo(HaveOccurred(),
				"passthrough from managed directory must be silently ignored, not error")
		})

		It("processes passthrough from non-managed directory normally", func() {
			mappings := []config.PassthroughMapping{
				{From: "testdata/external/fonts", To: "assets/fonts"},
			}
			managedDirs := []string{"content", "layouts", "assets", "static", "data"}
			err := static.CopyPassthroughWithValidation(mappings, "testdata/project", "testdata/output", managedDirs)
			Expect(err).NotTo(HaveOccurred())
		})
	})

	// ── Passthrough exclude filtering (issue #547) ──────────────────
	// The exclude field allows gitignore-style patterns that subtract
	// files from the passthrough copy. Patterns without / match at any
	// depth; patterns ending with / match directory trees.

	Describe("Passthrough exclude filtering (issue #547)", func() {
		var srcDir, dstDir string

		BeforeEach(func() {
			var err error
			srcDir, err = os.MkdirTemp("", "alloy-exclude-src-*")
			Expect(err).NotTo(HaveOccurred())
			dstDir, err = os.MkdirTemp("", "alloy-exclude-dst-*")
			Expect(err).NotTo(HaveOccurred())

			// Create a directory tree with mixed file types:
			// src/
			//   app.js
			//   style.css
			//   index.html
			//   sub/
			//     lib.js
			//     page.html
			//     data.map
			//   demo/
			//     example.html
			//     example.js
			for _, f := range []struct {
				path    string
				content string
			}{
				{"app.js", "js-root"},
				{"style.css", "css-root"},
				{"index.html", "html-root"},
				{"sub/lib.js", "js-sub"},
				{"sub/page.html", "html-sub"},
				{"sub/data.map", "map-sub"},
				{"demo/example.html", "html-demo"},
				{"demo/example.js", "js-demo"},
				{"sub/demo/nested.html", "html-sub-demo"},
				{"sub/demo/nested.js", "js-sub-demo"},
			} {
				full := filepath.Join(srcDir, f.path)
				Expect(os.MkdirAll(filepath.Dir(full), 0755)).To(Succeed())
				Expect(os.WriteFile(full, []byte(f.content), 0644)).To(Succeed())
			}
		})

		AfterEach(func() {
			os.RemoveAll(srcDir)
			os.RemoveAll(dstDir)
		})

		It("exclude *.html skips all .html files at any depth", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out", Exclude: []string{"*.html"}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred(),
				"CopyPassthrough with exclude must not error (issue #547)")

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "app.js")).To(BeAnExistingFile(),
				"non-excluded .js file must be copied (issue #547)")
			Expect(filepath.Join(outDir, "style.css")).To(BeAnExistingFile(),
				"non-excluded .css file must be copied (issue #547)")
			Expect(filepath.Join(outDir, "sub", "lib.js")).To(BeAnExistingFile(),
				"non-excluded nested .js file must be copied (issue #547)")
			Expect(filepath.Join(outDir, "index.html")).NotTo(BeAnExistingFile(),
				"*.html exclude must skip root-level .html files (issue #547)")
			Expect(filepath.Join(outDir, "sub", "page.html")).NotTo(BeAnExistingFile(),
				"*.html exclude must skip nested .html files — "+
					"gitignore-style: patterns without / match at any depth (issue #547)")
			Expect(filepath.Join(outDir, "demo", "example.html")).NotTo(BeAnExistingFile(),
				"*.html exclude must skip deeply nested .html files (issue #547)")
		})

		It("exclude demo/ skips entire directory tree", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out", Exclude: []string{"demo/"}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "app.js")).To(BeAnExistingFile(),
				"files outside excluded directory must be copied (issue #547)")
			Expect(filepath.Join(outDir, "demo", "example.html")).NotTo(BeAnExistingFile(),
				"demo/ exclude must skip all files in the demo directory (issue #547)")
			Expect(filepath.Join(outDir, "demo", "example.js")).NotTo(BeAnExistingFile(),
				"demo/ exclude must skip ALL files in the demo directory, not just .html (issue #547)")
		})

		It("exclude demo/*.html skips .html only in demo/", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out", Exclude: []string{"demo/*.html"}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "demo", "example.html")).NotTo(BeAnExistingFile(),
				"demo/*.html must exclude .html files in demo/ (issue #547)")
			Expect(filepath.Join(outDir, "demo", "example.js")).To(BeAnExistingFile(),
				"demo/*.html must not exclude non-.html files in demo/ (issue #547)")
			Expect(filepath.Join(outDir, "index.html")).To(BeAnExistingFile(),
				"demo/*.html must not exclude .html files outside demo/ — "+
					"pattern contains / so matches against relative path, not filename (issue #547)")
		})

		It("exclude sub/demo/*.html skips .html only in sub/demo/ (multi-segment path)", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out", Exclude: []string{"sub/demo/*.html"}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "sub", "demo", "nested.html")).NotTo(BeAnExistingFile(),
				"sub/demo/*.html must exclude .html files in sub/demo/ (issue #547)")
			Expect(filepath.Join(outDir, "sub", "demo", "nested.js")).To(BeAnExistingFile(),
				"sub/demo/*.html must not exclude non-.html files in sub/demo/ (issue #547)")
			Expect(filepath.Join(outDir, "demo", "example.html")).To(BeAnExistingFile(),
				"sub/demo/*.html must not exclude .html in top-level demo/ — "+
					"multi-segment pattern matches against full relative path (issue #547)")
			Expect(filepath.Join(outDir, "index.html")).To(BeAnExistingFile(),
				"sub/demo/*.html must not exclude root-level .html files (issue #547)")
		})

		It("multiple exclude patterns applied together", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out", Exclude: []string{"*.html", "*.map"}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "app.js")).To(BeAnExistingFile(),
				".js files must not be excluded (issue #547)")
			Expect(filepath.Join(outDir, "index.html")).NotTo(BeAnExistingFile(),
				"*.html pattern must exclude .html files (issue #547)")
			Expect(filepath.Join(outDir, "sub", "data.map")).NotTo(BeAnExistingFile(),
				"*.map pattern must exclude .map files (issue #547)")
		})

		It("no exclude field copies everything (backward compat)", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out"},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "app.js")).To(BeAnExistingFile())
			Expect(filepath.Join(outDir, "index.html")).To(BeAnExistingFile(),
				"nil Exclude must copy all files — backward compatibility (issue #547)")
			Expect(filepath.Join(outDir, "sub", "data.map")).To(BeAnExistingFile())
			Expect(filepath.Join(outDir, "demo", "example.html")).To(BeAnExistingFile())
		})

		It("empty exclude array copies everything", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out", Exclude: []string{}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "index.html")).To(BeAnExistingFile(),
				"empty Exclude slice must copy all files — same as nil (issue #547)")
		})
	})

	// ── Passthrough from glob (issue #547) ──────────────────────────
	// The from field supports glob patterns via doublestar. When from
	// contains metacharacters, only matching files are copied. Output
	// preserves directory structure relative to the glob root.

	Describe("Passthrough from glob (issue #547)", func() {
		var srcDir, dstDir string

		BeforeEach(func() {
			var err error
			srcDir, err = os.MkdirTemp("", "alloy-glob-src-*")
			Expect(err).NotTo(HaveOccurred())
			dstDir, err = os.MkdirTemp("", "alloy-glob-dst-*")
			Expect(err).NotTo(HaveOccurred())

			// Create src/
			//   a/foo.js
			//   a/foo.css
			//   a/foo.html
			//   a/foo.min.js
			//   b/bar.js
			//   b/bar.css
			for _, f := range []struct {
				path    string
				content string
			}{
				{"a/foo.js", "js-a"},
				{"a/foo.css", "css-a"},
				{"a/foo.html", "html-a"},
				{"a/foo.min.js", "min-js-a"},
				{"b/bar.js", "js-b"},
				{"b/bar.css", "css-b"},
			} {
				full := filepath.Join(srcDir, f.path)
				Expect(os.MkdirAll(filepath.Dir(full), 0755)).To(Succeed())
				Expect(os.WriteFile(full, []byte(f.content), 0644)).To(Succeed())
			}
		})

		AfterEach(func() {
			os.RemoveAll(srcDir)
			os.RemoveAll(dstDir)
		})

		It("from glob selects only matching files", func() {
			mappings := []config.PassthroughMapping{
				{From: filepath.Join(srcDir, "**/*.js"), To: "out"},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred(),
				"CopyPassthrough with glob from must not error (issue #547)")

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "a", "foo.js")).To(BeAnExistingFile(),
				"glob **/*.js must match .js files (issue #547)")
			Expect(filepath.Join(outDir, "b", "bar.js")).To(BeAnExistingFile(),
				"glob **/*.js must match .js in subdirectories (issue #547)")
			Expect(filepath.Join(outDir, "a", "foo.css")).NotTo(BeAnExistingFile(),
				"glob **/*.js must not match .css files (issue #547)")
			Expect(filepath.Join(outDir, "a", "foo.html")).NotTo(BeAnExistingFile(),
				"glob **/*.js must not match .html files (issue #547)")
		})

		It("from glob preserves directory structure relative to glob root", func() {
			mappings := []config.PassthroughMapping{
				{From: filepath.Join(srcDir, "**/*.js"), To: "out"},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "a", "foo.js")).To(BeAnExistingFile(),
				"output must preserve directory structure — a/foo.js not just foo.js (issue #547)")
			Expect(filepath.Join(outDir, "b", "bar.js")).To(BeAnExistingFile(),
				"output must preserve directory structure — b/bar.js not just bar.js (issue #547)")

			data, readErr := os.ReadFile(filepath.Join(outDir, "a", "foo.js"))
			Expect(readErr).NotTo(HaveOccurred())
			Expect(string(data)).To(Equal("js-a"),
				"copied file content must match source (issue #547)")
		})

		It("from glob with brace expansion matches multiple extensions", func() {
			mappings := []config.PassthroughMapping{
				{From: filepath.Join(srcDir, "**/*.{js,css}"), To: "out"},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred(),
				"brace expansion in from glob must be supported via doublestar (issue #547)")

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "a", "foo.js")).To(BeAnExistingFile(),
				"brace expansion must match .js files (issue #547)")
			Expect(filepath.Join(outDir, "a", "foo.css")).To(BeAnExistingFile(),
				"brace expansion must match .css files (issue #547)")
			Expect(filepath.Join(outDir, "a", "foo.html")).NotTo(BeAnExistingFile(),
				"brace expansion must not match unlisted extensions (issue #547)")
		})

		It("from glob + exclude subtracts from matched files", func() {
			mappings := []config.PassthroughMapping{
				{From: filepath.Join(srcDir, "**/*.{js,css}"), To: "out", Exclude: []string{"*.min.js"}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred(),
				"glob from + exclude must work together (issue #547)")

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "a", "foo.js")).To(BeAnExistingFile(),
				"non-excluded .js file must be copied (issue #547)")
			Expect(filepath.Join(outDir, "a", "foo.css")).To(BeAnExistingFile(),
				"non-excluded .css file must be copied (issue #547)")
			Expect(filepath.Join(outDir, "a", "foo.min.js")).NotTo(BeAnExistingFile(),
				"*.min.js exclude must subtract from glob matches (issue #547)")
		})

		It("from plain directory with exclude still works", func() {
			mappings := []config.PassthroughMapping{
				{From: srcDir, To: "out", Exclude: []string{"*.html"}},
			}
			err := static.CopyPassthrough(mappings, "/", dstDir)
			Expect(err).NotTo(HaveOccurred())

			outDir := filepath.Join(dstDir, "out")
			Expect(filepath.Join(outDir, "a", "foo.js")).To(BeAnExistingFile(),
				"plain directory from + exclude must still copy non-excluded files (issue #547)")
			Expect(filepath.Join(outDir, "a", "foo.html")).NotTo(BeAnExistingFile(),
				"plain directory from + exclude must filter out excluded files (issue #547)")
		})
	})
})
