package content_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

var _ = Describe("Page.SetRenderedHTML (issue #1185)", func() {

	// Page.SetRenderedHTML(s) stores both RenderedBody (as []byte) and the
	// cached HTML string simultaneously, avoiding the string→[]byte→string
	// round-trip that SetRenderedBody([]byte(html)) triggers:
	//   SetRenderedBody: string→[]byte (caller) → clear cache →
	//     next HTML() call: []byte→string (re-conversion)
	//   SetRenderedHTML: stores both at once → HTML() returns cached string
	//
	// The pipeline uses this when applying hook results back, where the
	// hook return value is already a Go string (from QuickJS GetPropertyStr).

	It("HTML() returns the correct string after SetRenderedHTML", func() {
		page := &content.Page{}
		page.SetRenderedHTML("<h1>Hello World</h1>")

		Expect(page.HTML()).To(Equal("<h1>Hello World</h1>"),
			"HTML() must return the string passed to SetRenderedHTML — "+
				"this is the primary contract: the cached string is stored "+
				"directly without intermediate []byte conversion")
	})

	It("RenderedBody matches the string passed to SetRenderedHTML", func() {
		page := &content.Page{}
		page.SetRenderedHTML("<p>content</p>")

		Expect(page.RenderedBody).To(Equal([]byte("<p>content</p>")),
			"RenderedBody must contain the []byte equivalent of the string — "+
				"code that reads RenderedBody directly (e.g., file writers) must "+
				"see the same content as HTML()")
	})

	It("overwrites previous RenderedBody and cached HTML", func() {
		page := &content.Page{
			RenderedBody: []byte("<p>old</p>"),
		}
		// Populate the HTML cache
		Expect(page.HTML()).To(Equal("<p>old</p>"))

		page.SetRenderedHTML("<p>new</p>")

		Expect(page.HTML()).To(Equal("<p>new</p>"),
			"SetRenderedHTML must overwrite the previously cached HTML string")
		Expect(page.RenderedBody).To(Equal([]byte("<p>new</p>")),
			"SetRenderedHTML must overwrite RenderedBody")
	})

	It("works with empty string", func() {
		page := &content.Page{
			RenderedBody: []byte("<p>existing</p>"),
		}
		_ = page.HTML() // populate cache

		page.SetRenderedHTML("")

		Expect(page.HTML()).To(Equal(""),
			"SetRenderedHTML with empty string must result in empty HTML()")
		Expect(page.RenderedBody).To(Equal([]byte("")),
			"SetRenderedHTML with empty string must set RenderedBody to empty []byte")
	})

	It("restores functionality after ReleaseRenderedBody", func() {
		page := &content.Page{
			RenderedBody: []byte("<p>original</p>"),
		}
		page.ReleaseRenderedBody()
		Expect(page.HTML()).To(Equal(""))

		page.SetRenderedHTML("<p>restored</p>")

		Expect(page.HTML()).To(Equal("<p>restored</p>"),
			"SetRenderedHTML must work after ReleaseRenderedBody — "+
				"the page must be usable again with new content")
		Expect(page.RenderedBody).To(Equal([]byte("<p>restored</p>")))
	})

	// ── Cache invalidation interaction (issue #1189) ─────────────────
	// SetRenderedBody after SetRenderedHTML must clear the cached string
	// so HTML() re-converts from the new RenderedBody. Without this,
	// the stale renderedStr from SetRenderedHTML would survive and
	// HTML() would return the wrong content.

	It("SetRenderedBody after SetRenderedHTML clears cached string", func() {
		page := &content.Page{}
		page.SetRenderedHTML("<p>from-hook</p>")

		// Verify the cache is populated
		Expect(page.HTML()).To(Equal("<p>from-hook</p>"),
			"precondition: SetRenderedHTML must populate the HTML cache")

		// SetRenderedBody must clear the cache set by SetRenderedHTML
		page.SetRenderedBody([]byte("<p>from-bytes</p>"))

		Expect(page.HTML()).To(Equal("<p>from-bytes</p>"),
			"SetRenderedBody after SetRenderedHTML must invalidate the "+
				"cached string so HTML() re-converts from the new RenderedBody. "+
				"If HTML() still returns '<p>from-hook</p>', SetRenderedBody "+
				"did not clear the renderedStr cache (issue #1189)")
		Expect(page.RenderedBody).To(Equal([]byte("<p>from-bytes</p>")),
			"RenderedBody must reflect the new []byte value")
	})
})
