package static_test

import (
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
