package server_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/server"
)

var _ = Describe("Error Overlay (dev mode only)", func() {

	// ── BuildError struct ─────────────────────────────────────────────

	Describe("BuildError", func() {
		It("carries file path, line, message, stage, and snippet", func() {
			err := server.BuildError{
				FilePath: "content/blog/my-post.md",
				Line:     14,
				Message:  `Unknown filter "formattDate" — did you mean "formatDate"?`,
				Stage:    "template rendering",
				Snippet:  `{{ page.date | formattDate: "%B %d, %Y" }}`,
			}

			Expect(err.FilePath).To(Equal("content/blog/my-post.md"))
			Expect(err.Line).To(Equal(14))
			Expect(err.Message).To(ContainSubstring("formattDate"))
			Expect(err.Stage).To(Equal("template rendering"))
			Expect(err.Snippet).To(ContainSubstring("formattDate"))

			// Verify the struct renders into a usable overlay
			html := server.RenderOverlay([]server.BuildError{err})
			Expect(html).NotTo(BeEmpty(), "overlay must produce HTML output")
		})
	})

	// ── RenderOverlay HTML output ─────────────────────────────────────

	Describe("RenderOverlay", func() {
		var buildErr server.BuildError

		BeforeEach(func() {
			buildErr = server.BuildError{
				FilePath: "content/blog/my-post.md",
				Line:     14,
				Message:  `Unknown filter "formattDate"`,
				Stage:    "template rendering",
				Snippet:  `{{ page.date | formattDate: "%B %d, %Y" }}`,
			}
		})

		It("includes the file path in the overlay HTML", func() {
			html := server.RenderOverlay([]server.BuildError{buildErr})
			Expect(html).To(ContainSubstring("content/blog/my-post.md"))
		})

		It("includes the line number in the overlay HTML", func() {
			html := server.RenderOverlay([]server.BuildError{buildErr})
			Expect(html).To(ContainSubstring("14"))
		})

		It("includes the source code snippet in the overlay HTML", func() {
			html := server.RenderOverlay([]server.BuildError{buildErr})
			Expect(html).To(ContainSubstring("formattDate"))
		})

		It("includes the pipeline stage in the overlay HTML", func() {
			html := server.RenderOverlay([]server.BuildError{buildErr})
			Expect(html).To(ContainSubstring("template rendering"))
		})
	})

	// ── OverlayState lifecycle ────────────────────────────────────────

	Describe("OverlayState", func() {
		It("tracks errors and clears them after successful rebuild", func() {
			state := server.NewOverlayState()
			Expect(state.HasErrors()).To(BeFalse(), "fresh state should have no errors")

			// Simulate a failed rebuild
			state.SetErrors([]server.BuildError{
				{
					FilePath: "content/about.md",
					Line:     3,
					Message:  "unterminated front matter",
					Stage:    "front matter extraction",
				},
			})
			Expect(state.HasErrors()).To(BeTrue(), "should have errors after failed rebuild")
			Expect(state.Errors()).To(HaveLen(1))

			// Simulate a successful rebuild — errors must clear
			state.ClearErrors()
			Expect(state.HasErrors()).To(BeFalse(), "errors must clear after successful rebuild")
			Expect(state.Errors()).To(BeEmpty())
		})
	})

	// ── Warning banner ────────────────────────────────────────────────

	Describe("RenderWarningBanner", func() {
		It("produces HTML containing the data source warning text", func() {
			warnings := []string{
				`Source "posts": https://api.example.com/posts.json unreachable — using cached data (age: 2h 15m)`,
			}
			html := server.RenderWarningBanner(warnings)
			Expect(html).NotTo(BeEmpty(), "warning banner must produce HTML output")
			Expect(html).To(ContainSubstring("posts"))
			Expect(html).To(ContainSubstring("unreachable"))
		})
	})
})
