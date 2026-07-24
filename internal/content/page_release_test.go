package content_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

var _ = Describe("Page.ReleaseRenderedBody (issue #1107)", func() {

	// Page.ReleaseRenderedBody() nils RenderedBody, clears renderedStr
	// (the cached HTML() result), and nils FormatBodies. This is the
	// mechanism that allows the pipeline to free page output memory
	// immediately after writing each page to disk, converting peak
	// memory from O(total site HTML) to O(largest single page).

	It("nils RenderedBody", func() {
		page := &content.Page{
			RenderedBody: []byte("<html><body>Hello</body></html>"),
		}
		Expect(page.RenderedBody).NotTo(BeNil(),
			"precondition: RenderedBody must be set before release")

		page.ReleaseRenderedBody()

		Expect(page.RenderedBody).To(BeNil(),
			"ReleaseRenderedBody must nil out RenderedBody — this is the "+
				"primary memory savings, allowing the GC to reclaim the page's "+
				"rendered output after disk write (issue #1107)")
	})

	It("clears the cached HTML string so HTML() returns empty", func() {
		page := &content.Page{
			RenderedBody: []byte("<html><body>Cached</body></html>"),
		}
		// Trigger the HTML() cache by calling it once
		cachedHTML := page.HTML()
		Expect(cachedHTML).To(Equal("<html><body>Cached</body></html>"),
			"precondition: HTML() must return the rendered body as a string")

		page.ReleaseRenderedBody()

		Expect(page.HTML()).To(Equal(""),
			"ReleaseRenderedBody must clear renderedStr so HTML() returns "+
				"empty string — the cached string is a second reference to the "+
				"same data and must be released alongside RenderedBody. If "+
				"renderedStr is not cleared, the string holds the rendered HTML "+
				"in memory even after RenderedBody is nil (issue #1107)")
	})

	It("nils FormatBodies", func() {
		page := &content.Page{
			RenderedBody: []byte("<html><body>Primary</body></html>"),
			FormatBodies: map[string][]byte{
				"json": []byte(`{"title":"Test"}`),
				"xml":  []byte(`<entry><title>Test</title></entry>`),
			},
		}
		Expect(page.FormatBodies).To(HaveLen(2),
			"precondition: FormatBodies must be populated before release")

		page.ReleaseRenderedBody()

		Expect(page.FormatBodies).To(BeNil(),
			"ReleaseRenderedBody must nil out FormatBodies — alternate format "+
				"rendered bodies (JSON, XML, etc.) must also be freed after their "+
				"output files are written to disk. Leaving FormatBodies in memory "+
				"while RenderedBody is released only partially addresses the "+
				"memory optimization (issue #1107)")
	})

	It("is safe to call on a page with no FormatBodies", func() {
		page := &content.Page{
			RenderedBody: []byte("<html><body>No formats</body></html>"),
		}
		Expect(page.FormatBodies).To(BeNil(),
			"precondition: FormatBodies is nil (no alternate formats)")

		// Must not panic when FormatBodies is already nil
		Expect(func() {
			page.ReleaseRenderedBody()
		}).NotTo(Panic(),
			"ReleaseRenderedBody must be safe to call when FormatBodies is "+
				"nil — most pages have no alternate output formats")

		Expect(page.RenderedBody).To(BeNil())
	})

	It("is safe to call on a page with no RenderedBody", func() {
		page := &content.Page{}
		Expect(page.RenderedBody).To(BeNil(),
			"precondition: RenderedBody is nil (unrendered page)")

		// Must not panic when RenderedBody is already nil
		Expect(func() {
			page.ReleaseRenderedBody()
		}).NotTo(Panic(),
			"ReleaseRenderedBody must be safe to call when RenderedBody is "+
				"already nil — the pipeline may call it on pages that were "+
				"skipped during rendering")

		Expect(page.HTML()).To(Equal(""))
	})

	It("releases FormatBodies even when RenderedBody is nil", func() {
		// Covers the case where RenderedBody is nil but FormatBodies is
		// populated (e.g., API-only endpoints that produce JSON but no HTML).
		// If ReleaseRenderedBody() uses `if p.RenderedBody == nil { return }`
		// as an early exit, FormatBodies would not be released.
		page := &content.Page{
			FormatBodies: map[string][]byte{
				"json": []byte(`{"title":"API endpoint"}`),
			},
		}
		Expect(page.RenderedBody).To(BeNil(),
			"precondition: RenderedBody is nil (API-only page)")
		Expect(page.FormatBodies).To(HaveLen(1),
			"precondition: FormatBodies is populated")

		page.ReleaseRenderedBody()

		Expect(page.FormatBodies).To(BeNil(),
			"ReleaseRenderedBody must nil FormatBodies even when "+
				"RenderedBody is already nil — an early-exit guard on "+
				"nil RenderedBody would skip the FormatBodies release "+
				"and leak memory for API-only pages (issue #1107)")
	})

	It("is idempotent — calling twice does not panic", func() {
		page := &content.Page{
			RenderedBody: []byte("<html><body>Twice</body></html>"),
			FormatBodies: map[string][]byte{
				"json": []byte(`{"title":"Test"}`),
			},
		}

		page.ReleaseRenderedBody()
		Expect(func() {
			page.ReleaseRenderedBody()
		}).NotTo(Panic(),
			"ReleaseRenderedBody must be idempotent — the pipeline may "+
				"call it in cleanup paths that run regardless of prior calls")

		Expect(page.RenderedBody).To(BeNil())
		Expect(page.FormatBodies).To(BeNil())
		Expect(page.HTML()).To(Equal(""))
	})

	// MAINTENANCE: When adding new fields to the Page struct, add an
	// assertion below to verify ReleaseRenderedBody does not clear the
	// new field. This test is a static snapshot — it will not catch
	// accidental clearing of fields added after this test was written.
	It("does not affect other page fields", func() {
		page := &content.Page{
			SourcePath:   "/content/test.md",
			RelPath:      "test.md",
			Content:      []byte("# Test\nOriginal content"),
			Body:         []byte("# Test\nOriginal content"),
			RenderedBody: []byte("<html><body><h1>Test</h1></body></html>"),
			URL:          "/test/",
			Section:      "root",
			Layout:       "default",
			FormatBodies: map[string][]byte{
				"json": []byte(`{"title":"Test"}`),
			},
			FrontMatter: map[string]interface{}{
				"title": "Test",
			},
		}

		page.ReleaseRenderedBody()

		// RenderedBody and FormatBodies are released
		Expect(page.RenderedBody).To(BeNil())
		Expect(page.FormatBodies).To(BeNil())

		// All other fields must be preserved — they are still needed
		// for sitemap generation, cache tracking, and BuildResult metadata
		Expect(page.SourcePath).To(Equal("/content/test.md"),
			"ReleaseRenderedBody must not affect SourcePath")
		Expect(page.RelPath).To(Equal("test.md"),
			"ReleaseRenderedBody must not affect RelPath")
		Expect(page.Content).To(Equal([]byte("# Test\nOriginal content")),
			"ReleaseRenderedBody must not affect Content — the raw content "+
				"is still needed for content hash tracking in the build cache")
		Expect(page.Body).To(Equal([]byte("# Test\nOriginal content")),
			"ReleaseRenderedBody must not affect Body")
		Expect(page.URL).To(Equal("/test/"),
			"ReleaseRenderedBody must not affect URL — needed for sitemap")
		Expect(page.Section).To(Equal("root"),
			"ReleaseRenderedBody must not affect Section")
		Expect(page.Layout).To(Equal("default"),
			"ReleaseRenderedBody must not affect Layout")
		Expect(page.FrontMatter).To(HaveKeyWithValue("title", "Test"),
			"ReleaseRenderedBody must not affect FrontMatter")
	})

	It("allows SetRenderedBody to restore functionality after release", func() {
		page := &content.Page{
			RenderedBody: []byte("<html>Original</html>"),
		}
		_ = page.HTML() // populate cache

		page.ReleaseRenderedBody()
		Expect(page.RenderedBody).To(BeNil())
		Expect(page.HTML()).To(Equal(""))

		// Restore via SetRenderedBody — this is used by SSR Phase 2
		// which may run after release in future pipeline orderings
		page.SetRenderedBody([]byte("<html>Restored</html>"))
		Expect(page.RenderedBody).To(Equal([]byte("<html>Restored</html>")),
			"SetRenderedBody must work after ReleaseRenderedBody — the page "+
				"must be usable again if new content is assigned")
		Expect(page.HTML()).To(Equal("<html>Restored</html>"),
			"HTML() must return the new content after SetRenderedBody restores "+
				"a released page")
	})
})
