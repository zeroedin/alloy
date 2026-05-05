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
				_ = os.MkdirAll(subdir, 0755)
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
})
