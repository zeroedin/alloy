package cache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
)

var _ = Describe("Virtual Dependency Tracking (issue #1058)", func() {

	// ── TrackVirtualDependency + InvalidatedVirtualPages ─────────────
	//
	// File-derived virtual pages declare dependencies on source files
	// outside the content directory. The cache stores a reverse map
	// (source path → virtual page RelPaths) so BuildIncremental can
	// selectively rebuild only the virtual pages whose source files
	// changed, instead of re-rendering all 400+ virtual pages on every
	// incremental rebuild.

	Describe("TrackVirtualDependency and InvalidatedVirtualPages", func() {
		It("returns invalidated virtual pages for a tracked source file", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")

			pages := c.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(pages).To(ConsistOf("_virtual/demos/button.html"),
				"InvalidatedVirtualPages must return virtual pages that declared "+
					"a dependency on the given source file (issue #1058)")
		})

		It("returns nil for a source file with no tracked virtual pages", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")

			// Guard: tracked source must return pages
			Expect(c.InvalidatedVirtualPages("elements/button/demo.html")).NotTo(BeEmpty(),
				"guard: tracked source must return its dependent virtual pages")

			pages := c.InvalidatedVirtualPages("elements/unrelated/other.html")
			Expect(pages).To(BeNil(),
				"InvalidatedVirtualPages must return nil for a source file "+
					"that no virtual page depends on — this distinguishes "+
					"'no dependencies tracked' from 'dependencies tracked but "+
					"none matched' (issue #1058)")
		})

		It("returns all virtual pages depending on the same source file", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/shared/base.html")
			c.TrackVirtualDependency("_virtual/demos/card.html", "elements/shared/base.html")
			c.TrackVirtualDependency("_virtual/demos/dialog.html", "elements/shared/base.html")

			pages := c.InvalidatedVirtualPages("elements/shared/base.html")
			Expect(pages).To(ConsistOf(
				"_virtual/demos/button.html",
				"_virtual/demos/card.html",
				"_virtual/demos/dialog.html",
			), "all virtual pages depending on the same source file must "+
				"be returned by InvalidatedVirtualPages — a shared dependency "+
				"like a base template invalidates every page that uses it "+
				"(issue #1058)")
		})

		It("invalidates a virtual page from any of its declared sources", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/styles.css")
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/script.js")

			pages1 := c.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(pages1).To(ConsistOf("_virtual/demos/button.html"),
				"virtual page must be invalidated by its first dependency (issue #1058)")

			pages2 := c.InvalidatedVirtualPages("elements/button/styles.css")
			Expect(pages2).To(ConsistOf("_virtual/demos/button.html"),
				"virtual page must be invalidated by its second dependency — "+
					"a page declaring dependencies: ['demo.html', 'styles.css', 'script.js'] "+
					"must be invalidated when ANY of those files changes (issue #1058)")

			pages3 := c.InvalidatedVirtualPages("elements/button/script.js")
			Expect(pages3).To(ConsistOf("_virtual/demos/button.html"),
				"virtual page must be invalidated by its third dependency (issue #1058)")
		})

		It("deduplicates repeated dependency tracking calls", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")

			pages := c.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(pages).To(HaveLen(1),
				"TrackVirtualDependency must deduplicate — calling it three times "+
					"with the same virtual-page/source pair must produce exactly "+
					"one entry, not three (issue #1058)")
		})
	})

	// ── Clear / ClearVirtualPages ───────────────────────────────────

	Describe("Clear and ClearVirtualPages reset virtualDependencies", func() {
		It("Clear resets virtualDependencies", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")

			// Guard: dependency must be tracked before Clear
			Expect(c.InvalidatedVirtualPages("elements/button/demo.html")).To(
				ConsistOf("_virtual/demos/button.html"),
				"guard: dependency must be tracked before Clear")

			c.Clear()

			pages := c.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(pages).To(BeNil(),
				"Clear must reset virtualDependencies — stale dependency "+
					"entries from a previous build would cause incorrect "+
					"selective rendering in subsequent rebuilds (issue #1058)")
		})

		It("ClearVirtualPages resets virtualDependencies", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")
			c.TrackVirtualDependency("_virtual/demos/card.html", "elements/card/demo.html")

			// Guard: dependencies must be tracked before ClearVirtualPages
			Expect(c.InvalidatedVirtualPages("elements/button/demo.html")).To(
				ConsistOf("_virtual/demos/button.html"),
				"guard: button dependency must be tracked before clear")

			c.ClearVirtualPages()

			buttonPages := c.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(buttonPages).To(BeNil(),
				"ClearVirtualPages must reset virtualDependencies alongside "+
					"virtualPages — both are re-populated together after each "+
					"build, so stale entries must not persist (issue #1058)")

			cardPages := c.InvalidatedVirtualPages("elements/card/demo.html")
			Expect(cardPages).To(BeNil(),
				"ClearVirtualPages must reset all virtualDependencies entries (issue #1058)")
		})
	})

	// ── Clone ────────────────────────────────────────────────────────

	Describe("Clone preserves virtual dependencies", func() {
		It("cloned cache returns the same InvalidatedVirtualPages results", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")
			c.TrackVirtualDependency("_virtual/demos/card.html", "elements/card/demo.html")

			cloned := c.Clone()

			pagesA := cloned.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(pagesA).To(ConsistOf("_virtual/demos/button.html"),
				"cloned cache must preserve virtual dependency mappings (issue #1058)")

			pagesB := cloned.InvalidatedVirtualPages("elements/card/demo.html")
			Expect(pagesB).To(ConsistOf("_virtual/demos/card.html"),
				"cloned cache must preserve all virtual dependency entries (issue #1058)")
		})

		It("cloned cache is independent from original", func() {
			c := cache.New()
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")

			cloned := c.Clone()

			// Modify original after cloning
			c.TrackVirtualDependency("_virtual/demos/extra.html", "elements/button/demo.html")

			// Clone must not be affected by post-clone modifications to original
			pages := cloned.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(pages).To(ConsistOf("_virtual/demos/button.html"),
				"cloned cache must be independent — modifications to the "+
					"original after cloning must not affect the clone (issue #1058)")
			Expect(pages).NotTo(ContainElement("_virtual/demos/extra.html"),
				"entry added to original after clone must not appear in clone — "+
					"the virtualDependencies map must be deep-copied, not shared (issue #1058)")
		})
	})
})
