package cache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
)

var _ = Describe("Page Dependency Tracking (issue #1100)", func() {

	// ── TrackDependency + PagesForDependency ────────────────────────
	//
	// Plugins that transform page output based on external files (SSR
	// components, Sass partials, translation files, etc.) declare those
	// dependencies via addDependencies in hook return values. The cache
	// stores a reverse map (dependency path → content page RelPaths) so
	// BuildIncremental can selectively rebuild only pages whose external
	// dependencies changed, instead of triggering a full rebuild.
	//
	// This is parallel to virtual page dependency tracking (issue #1058)
	// but applies to CONTENT pages — filesystem-discovered pages that go
	// through onPageRendered / onContentTransformed, not plugin-injected
	// virtual pages.

	Describe("TrackDependency and PagesForDependency", func() {
		It("returns pages that depend on a tracked file", func() {
			c := cache.New()
			c.TrackDependency("about/index.html", "elements/rh-card/rh-card.js")

			pages := c.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages).To(ConsistOf("about/index.html"),
				"PagesForDependency must return content pages that declared "+
					"a dependency on the given file path via addDependencies "+
					"(issue #1100)")
		})

		It("returns nil for a file path with no tracked pages", func() {
			c := cache.New()
			c.TrackDependency("about/index.html", "elements/rh-card/rh-card.js")

			// Guard: tracked dependency must return pages
			Expect(c.PagesForDependency("elements/rh-card/rh-card.js")).NotTo(BeEmpty(),
				"guard: tracked dependency must return its dependent pages")

			pages := c.PagesForDependency("elements/rh-icon/rh-icon.js")
			Expect(pages).To(BeNil(),
				"PagesForDependency must return nil for a file that no page "+
					"depends on — this distinguishes 'no dependencies tracked' "+
					"from 'tracked but no matches' (issue #1100)")
		})

		It("returns all pages depending on the same file", func() {
			c := cache.New()
			c.TrackDependency("index.html", "elements/rh-icon/rh-icon.js")
			c.TrackDependency("about/index.html", "elements/rh-icon/rh-icon.js")
			c.TrackDependency("blog/index.html", "elements/rh-icon/rh-icon.js")

			pages := c.PagesForDependency("elements/rh-icon/rh-icon.js")
			Expect(pages).To(ConsistOf(
				"index.html",
				"about/index.html",
				"blog/index.html",
			), "all content pages depending on the same external file must "+
				"be returned by PagesForDependency — a shared dependency "+
				"like a web component definition invalidates every page "+
				"that uses it (issue #1100)")
		})

		It("tracks a single page with multiple dependencies", func() {
			c := cache.New()
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")
			c.TrackDependency("index.html", "elements/rh-icon/rh-icon.js")
			c.TrackDependency("index.html", "elements/rh-footer/rh-footer.js")

			pages1 := c.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages1).To(ConsistOf("index.html"),
				"page must be invalidated by its first dependency (issue #1100)")

			pages2 := c.PagesForDependency("elements/rh-icon/rh-icon.js")
			Expect(pages2).To(ConsistOf("index.html"),
				"page must be invalidated by its second dependency — a page "+
					"declaring addDependencies(['rh-card.js', 'rh-icon.js', 'rh-footer.js']) "+
					"must be invalidated when ANY of those files changes (issue #1100)")

			pages3 := c.PagesForDependency("elements/rh-footer/rh-footer.js")
			Expect(pages3).To(ConsistOf("index.html"),
				"page must be invalidated by its third dependency (issue #1100)")
		})

		It("deduplicates repeated dependency tracking calls", func() {
			c := cache.New()
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")

			pages := c.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages).To(HaveLen(1),
				"TrackDependency must deduplicate — calling it three times "+
					"with the same page/dependency pair must produce exactly "+
					"one entry, not three (issue #1100)")
			Expect(pages).To(ConsistOf("index.html"))
		})

		It("tracks dependencies independently from virtual dependencies", func() {
			c := cache.New()
			// Track a virtual page dependency (issue #1058)
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/rh-card/rh-card.js")
			// Track a content page dependency (issue #1100)
			c.TrackDependency("about/index.html", "elements/rh-card/rh-card.js")

			// PagesForDependency must only return content page dependencies
			pages := c.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages).To(ConsistOf("about/index.html"),
				"PagesForDependency must return only content page dependencies, "+
					"not virtual page dependencies — the two tracking systems "+
					"are independent (issue #1100 vs #1058)")
			Expect(pages).NotTo(ContainElement("_virtual/demos/button.html"),
				"virtual page dependency must not leak into content page "+
					"dependency tracking")

			// InvalidatedVirtualPages must only return virtual page dependencies
			virtualPages := c.InvalidatedVirtualPages("elements/rh-card/rh-card.js")
			Expect(virtualPages).To(ConsistOf("_virtual/demos/button.html"),
				"InvalidatedVirtualPages must return only virtual page deps")
			Expect(virtualPages).NotTo(ContainElement("about/index.html"),
				"content page dependency must not leak into virtual page "+
					"dependency tracking")
		})
	})

	// ── ClearDependencies ──────────────────────────────────────────

	Describe("ClearDependencies", func() {
		It("resets all page dependency tracking", func() {
			c := cache.New()
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")
			c.TrackDependency("about/index.html", "elements/rh-icon/rh-icon.js")

			// Guard: dependencies must be tracked before clear
			Expect(c.PagesForDependency("elements/rh-card/rh-card.js")).To(
				ConsistOf("index.html"),
				"guard: dependency must be tracked before ClearDependencies")

			c.ClearDependencies()

			pages1 := c.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages1).To(BeNil(),
				"ClearDependencies must reset all page dependency entries — "+
					"stale dependency entries from a previous build would cause "+
					"incorrect selective rendering in subsequent rebuilds "+
					"(issue #1100)")

			pages2 := c.PagesForDependency("elements/rh-icon/rh-icon.js")
			Expect(pages2).To(BeNil(),
				"ClearDependencies must reset all entries, not just the first")
		})

		It("does not affect virtual page dependencies", func() {
			c := cache.New()
			c.TrackDependency("about/index.html", "elements/rh-card/rh-card.js")
			c.TrackVirtualDependency("_virtual/demos/button.html", "elements/button/demo.html")

			c.ClearDependencies()

			// Content page deps cleared
			Expect(c.PagesForDependency("elements/rh-card/rh-card.js")).To(BeNil(),
				"ClearDependencies must clear content page dependencies")

			// Virtual page deps untouched
			virtualPages := c.InvalidatedVirtualPages("elements/button/demo.html")
			Expect(virtualPages).To(ConsistOf("_virtual/demos/button.html"),
				"ClearDependencies must not affect virtual page dependencies — "+
					"the two tracking systems are independent and cleared "+
					"separately (issue #1100)")
		})
	})

	// ── Clear ──────────────────────────────────────────────────────

	Describe("Clear resets page dependencies", func() {
		It("Clear removes all page dependencies alongside other data", func() {
			c := cache.New()
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")
			c.SetHash("index.html", "abc123")

			// Guard
			Expect(c.PagesForDependency("elements/rh-card/rh-card.js")).NotTo(BeNil(),
				"guard: dependency must be tracked before Clear")

			c.Clear()

			pages := c.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages).To(BeNil(),
				"Clear must reset page dependencies along with hashes, "+
					"templates, and virtual pages (issue #1100)")
		})
	})

	// ── Clone ──────────────────────────────────────────────────────

	Describe("Clone preserves page dependencies", func() {
		It("cloned cache returns the same PagesForDependency results", func() {
			c := cache.New()
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")
			c.TrackDependency("about/index.html", "elements/rh-icon/rh-icon.js")

			cloned := c.Clone()

			pages1 := cloned.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages1).To(ConsistOf("index.html"),
				"cloned cache must preserve page dependency mappings (issue #1100)")

			pages2 := cloned.PagesForDependency("elements/rh-icon/rh-icon.js")
			Expect(pages2).To(ConsistOf("about/index.html"),
				"cloned cache must preserve all page dependency entries (issue #1100)")
		})

		It("cloned cache is independent from original", func() {
			c := cache.New()
			c.TrackDependency("index.html", "elements/rh-card/rh-card.js")

			cloned := c.Clone()

			// Modify original after cloning
			c.TrackDependency("blog/index.html", "elements/rh-card/rh-card.js")

			// Clone must not be affected by post-clone modifications
			pages := cloned.PagesForDependency("elements/rh-card/rh-card.js")
			Expect(pages).To(ConsistOf("index.html"),
				"cloned cache must be independent — modifications to the "+
					"original after cloning must not affect the clone (issue #1100)")
			Expect(pages).NotTo(ContainElement("blog/index.html"),
				"entry added to original after clone must not appear in clone — "+
					"the pageDependencies map must be deep-copied, not shared "+
					"(issue #1100)")
		})
	})
})
