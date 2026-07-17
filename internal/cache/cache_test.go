package cache_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cache"
)

var _ = Describe("Build Cache (§10 Performance Architecture)", func() {

	// ── Content-hash cache (.alloy/cache.json) ───────────────────────

	Describe("Content-hash storage", func() {
		It("new cache has zero entries and tracks additions", func() {
			c := cache.New()
			Expect(c).NotTo(BeNil())
			Expect(c.Entries()).To(Equal(0))

			// Guard: adding an entry must increase count
			c.SetHash("content/index.md", "abc123")
			Expect(c.Entries()).To(Equal(1),
				"guard: Entries must reflect stored hashes")
		})

		It("SetHash + GetHash roundtrip stores and retrieves a hash", func() {
			c := cache.New()
			c.SetHash("content/blog/post.md", "abc123def456")
			hash := c.GetHash("content/blog/post.md")
			Expect(hash).To(Equal("abc123def456"),
				"GetHash must return the hash stored by SetHash")
		})

		It("HasChanged returns true when hash differs from stored", func() {
			c := cache.New()
			c.SetHash("content/about.md", "oldhash111")

			Expect(c.HasChanged("content/about.md", "newhash222")).To(BeTrue(),
				"different hash means content changed")
		})

		It("HasChanged returns false when hash matches stored", func() {
			c := cache.New()
			c.SetHash("content/about.md", "samehash")

			// Positive-case guard: different hash must return true
			Expect(c.HasChanged("content/about.md", "differenthash")).To(BeTrue(),
				"guard: different hash must be detected as changed")

			// Actual assertion: matching hash means unchanged
			Expect(c.HasChanged("content/about.md", "samehash")).To(BeFalse(),
				"matching hash means content is unchanged")
		})
	})

	// ── Content-hash computation ─────────────────────────────────────

	Describe("HashContent", func() {
		It("returns a SHA-256 hex string for given content", func() {
			hash := cache.HashContent([]byte("hello world"))
			// SHA-256 of "hello world" is a known value
			Expect(hash).To(Equal(
				"b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"),
				"must produce correct SHA-256 hex digest")
		})

		It("produces identical hashes for identical content", func() {
			content := []byte("---\ntitle: My Post\n---\n\n# Hello\n")
			hash1 := cache.HashContent(content)
			hash2 := cache.HashContent(content)

			// Guard: hash must be non-empty (stub returns "")
			Expect(hash1).NotTo(BeEmpty(),
				"guard: hash must be a non-empty string")
			Expect(hash1).To(Equal(hash2),
				"same content must always produce the same hash")
		})
	})

	// ── Template invalidation tracking (§2) ──────────────────────────

	Describe("Template invalidation", func() {
		It("TrackTemplateUsage records page-to-template mapping", func() {
			c := cache.New()
			c.TrackTemplateUsage("content/blog/post-1.md", "layouts/post.liquid")
			c.TrackTemplateUsage("content/blog/post-2.md", "layouts/post.liquid")
			c.TrackTemplateUsage("content/about.md", "layouts/default.liquid")

			pages := c.InvalidatedPages("layouts/post.liquid")
			Expect(pages).To(ConsistOf(
				"content/blog/post-1.md",
				"content/blog/post-2.md",
			), "only pages using post.liquid should be invalidated")
		})

		It("InvalidatedPages returns nil for untracked templates", func() {
			c := cache.New()
			c.TrackTemplateUsage("content/index.md", "layouts/home.liquid")

			// Guard: tracked template must return pages
			Expect(c.InvalidatedPages("layouts/home.liquid")).NotTo(BeEmpty(),
				"guard: tracked template must return its pages")

			// Actual assertion: unknown template returns nil
			pages := c.InvalidatedPages("layouts/unknown.liquid")
			Expect(pages).To(BeNil(),
				"untracked template should return nil")
		})

		It("TrackTemplateUsage deduplicates repeated page-template pairs (issue #589)", func() {
			c := cache.New()
			c.TrackTemplateUsage("content/blog/post-1.md", "layouts/post.liquid")
			c.TrackTemplateUsage("content/blog/post-1.md", "layouts/post.liquid")
			c.TrackTemplateUsage("content/blog/post-1.md", "layouts/post.liquid")

			pages := c.InvalidatedPages("layouts/post.liquid")
			Expect(pages).To(HaveLen(1),
				"TrackTemplateUsage must deduplicate — calling it three times with "+
					"the same page-template pair must produce exactly one entry, "+
					"not three (issue #589: replace O(n) slice scan with map lookup)")
		})

	})

	// ── Cache lifecycle ──────────────────────────────────────────────

	Describe("Clear", func() {
		It("removes all entries from the cache", func() {
			c := cache.New()
			c.SetHash("content/index.md", "hash1")
			c.SetHash("content/about.md", "hash2")
			c.TrackTemplateUsage("content/index.md", "layouts/default.liquid")

			// Guard: entries must exist before clear
			Expect(c.Entries()).To(Equal(2),
				"guard: cache must have entries before clear")

			c.Clear()
			Expect(c.Entries()).To(Equal(0),
				"Clear must remove all hash entries")
			Expect(c.GetHash("content/index.md")).To(BeEmpty(),
				"cleared cache must not return old hashes")
			Expect(c.InvalidatedPages("layouts/default.liquid")).To(BeNil(),
				"cleared cache must not return old template tracking")
		})
	})

	// ── Incremental build invalidation rules (§2) ──────────────────

	Describe("Incremental build invalidation rules", func() {
		It("unchanged content file is skipped entirely", func() {
			c := cache.New()
			content := []byte("---\ntitle: My Post\n---\n# Hello")
			hash := cache.HashContent(content)
			c.SetHash("content/blog/post.md", hash)

			// Guard: different content must NOT be skipped
			Expect(c.ShouldSkipFile("content/blog/post.md", []byte("different content"))).To(BeFalse(),
				"guard: changed content must not be skipped")

			// Same content must be skipped
			Expect(c.ShouldSkipFile("content/blog/post.md", content)).To(BeTrue(),
				"unchanged content file must be skipped (no re-parse, no re-render)")
		})

		It("config change triggers full rebuild", func() {
			c := cache.New()

			// Guard: same config hash should not trigger rebuild
			c.SetHash("__config__", "original_hash")
			Expect(c.IsConfigChanged("original_hash")).To(BeFalse(),
				"guard: unchanged config must not trigger rebuild")

			// Changed config hash triggers full rebuild
			Expect(c.IsConfigChanged("new_hash")).To(BeTrue(),
				"config change must trigger full rebuild of all pages")
		})

		It("global data change rebuilds all pages that could read it", func() {
			c := cache.New()
			c.SetHash("content/index.md", "h1")
			c.SetHash("content/about.md", "h2")
			c.SetHash("content/blog/post.md", "h3")

			affected := c.InvalidatedByGlobalData()
			Expect(affected).To(ConsistOf(
				"content/index.md",
				"content/about.md",
				"content/blog/post.md",
			), "global data change must rebuild all pages")
		})

		It("directory _data.yaml change rebuilds only that directory's pages", func() {
			c := cache.New()
			c.TrackDirectoryData("content/blog/post-1.md", "content/blog/")
			c.TrackDirectoryData("content/blog/post-2.md", "content/blog/")
			c.TrackDirectoryData("content/about.md", "content/")

			affected := c.InvalidatedByDirectoryData("content/blog/")
			Expect(affected).To(ConsistOf(
				"content/blog/post-1.md",
				"content/blog/post-2.md",
			), "directory _data.yaml change must only rebuild that directory's pages")
			Expect(affected).NotTo(ContainElement("content/about.md"),
				"pages in other directories must not be affected")
		})

		It("template change invalidates only pages using that template", func() {
			c := cache.New()
			c.TrackTemplateUsage("content/blog/post-1.md", "layouts/post.liquid")
			c.TrackTemplateUsage("content/blog/post-2.md", "layouts/post.liquid")
			c.TrackTemplateUsage("content/about.md", "layouts/default.liquid")

			affected := c.InvalidatedPages("layouts/post.liquid")
			Expect(affected).To(ConsistOf(
				"content/blog/post-1.md",
				"content/blog/post-2.md",
			), "template change must only invalidate pages using that template")
		})

		It("component definition change triggers Phase 2 re-SSR only", func() {
			c := cache.New()
			// Phase 1 output is unchanged — only Phase 2 SSR needs to re-run
			phase1Content := []byte("<html><body><ds-card>content</ds-card></body></html>")
			phase1Hash := cache.HashContent(phase1Content)
			c.SetHash("content/index.md", phase1Hash)

			// The content hasn't changed, so Phase 1 should be skippable
			Expect(c.ShouldSkipFile("content/index.md", phase1Content)).To(BeTrue(),
				"Phase 1 output unchanged — Phase 1 must be skipped")

			// But a component definition change means Phase 2 must re-run
			// This is tracked separately from content hashes
			c.SetHash("__component:ds-card__", "old_definition_hash")
			Expect(c.HasChanged("__component:ds-card__", "new_definition_hash")).To(BeTrue(),
				"component definition change must trigger Phase 2 re-SSR")
		})

		It("TrackDirectoryData deduplicates repeated page-directory pairs (issue #589)", func() {
			c := cache.New()
			c.TrackDirectoryData("content/blog/post-1.md", "content/blog/")
			c.TrackDirectoryData("content/blog/post-1.md", "content/blog/")
			c.TrackDirectoryData("content/blog/post-1.md", "content/blog/")

			affected := c.InvalidatedByDirectoryData("content/blog/")
			Expect(affected).To(HaveLen(1),
				"TrackDirectoryData must deduplicate — calling it three times with "+
					"the same page-directory pair must produce exactly one entry, "+
					"not three (issue #589: replace O(n) slice scan with map lookup)")
		})
	})

	// ── Virtual page tracking (issue #970) ───────────────────────────
	// BuildIncremental must know which RelPaths were virtual pages in
	// the previous build so it can pre-populate renderRelPaths before
	// onPagesReady runs. The cache stores this set and carries it
	// forward across builds via Clone.

	Describe("Virtual page tracking (issue #970)", func() {
		It("TrackVirtualPage and VirtualPagePaths roundtrip", func() {
			c := cache.New()
			c.TrackVirtualPage("_virtual/demos/button.html")
			c.TrackVirtualPage("_virtual/demos/card.html")

			paths := c.VirtualPagePaths()
			Expect(paths).To(ConsistOf(
				"_virtual/demos/button.html",
				"_virtual/demos/card.html",
			), "VirtualPagePaths must return all RelPaths tracked via TrackVirtualPage")
		})

		It("VirtualPagePaths returns empty slice when no virtual pages tracked", func() {
			c := cache.New()

			paths := c.VirtualPagePaths()
			Expect(paths).To(BeEmpty(),
				"VirtualPagePaths must return an empty slice when no virtual "+
					"pages have been tracked — not nil, to avoid nil-pointer issues "+
					"in callers that range over the result")
		})

		It("TrackVirtualPage deduplicates repeated RelPaths", func() {
			c := cache.New()
			c.TrackVirtualPage("_virtual/demos/button.html")
			c.TrackVirtualPage("_virtual/demos/button.html")
			c.TrackVirtualPage("_virtual/demos/button.html")

			paths := c.VirtualPagePaths()
			Expect(paths).To(HaveLen(1),
				"TrackVirtualPage must deduplicate — tracking the same RelPath "+
					"three times must produce exactly one entry (issue #970)")
		})

		It("Clone preserves virtual page tracking", func() {
			c := cache.New()
			c.TrackVirtualPage("_virtual/demos/button.html")
			c.TrackVirtualPage("_virtual/demos/card.html")

			cloned := c.Clone()

			// Guard: original must have virtual pages
			Expect(c.VirtualPagePaths()).To(ConsistOf(
				"_virtual/demos/button.html",
				"_virtual/demos/card.html",
			), "guard: original cache must have virtual pages before clone")

			// Cloned cache must have the same virtual pages
			clonedPaths := cloned.VirtualPagePaths()
			Expect(clonedPaths).To(ConsistOf(
				"_virtual/demos/button.html",
				"_virtual/demos/card.html",
			), "Clone must deep-copy virtual page tracking — the cloned "+
				"cache must return the same virtual page RelPaths as the "+
				"original (issue #970)")
		})

		It("Clone produces an independent copy of virtual page set", func() {
			c := cache.New()
			c.TrackVirtualPage("_virtual/demos/button.html")

			cloned := c.Clone()

			// Modify original after cloning
			c.TrackVirtualPage("_virtual/demos/card.html")

			// Cloned cache must not be affected by changes to original
			clonedPaths := cloned.VirtualPagePaths()
			Expect(clonedPaths).To(ConsistOf("_virtual/demos/button.html"),
				"Clone must produce an independent copy — modifying the "+
					"original after cloning must not affect the clone")
			Expect(clonedPaths).NotTo(ContainElement("_virtual/demos/card.html"),
				"changes to original cache must not leak into cloned cache")
		})

		It("Clear removes virtual page tracking", func() {
			c := cache.New()
			c.TrackVirtualPage("_virtual/demos/button.html")

			// Guard: virtual pages must exist before clear
			Expect(c.VirtualPagePaths()).NotTo(BeEmpty(),
				"guard: cache must have virtual pages before clear")

			c.Clear()

			Expect(c.VirtualPagePaths()).To(BeEmpty(),
				"Clear must remove all virtual page tracking alongside "+
					"content hashes and template tracking")
		})

		It("virtual page tracking is independent of content hashes", func() {
			c := cache.New()
			c.SetHash("index.md", "abc123")
			c.TrackVirtualPage("_virtual/demos/button.html")

			// Content hash and virtual page are independent
			Expect(c.GetHash("index.md")).To(Equal("abc123"),
				"content hash must be retrievable alongside virtual page tracking")
			Expect(c.VirtualPagePaths()).To(ConsistOf("_virtual/demos/button.html"),
				"virtual page tracking must be retrievable alongside content hashes")

			// Virtual pages must NOT appear in content hash count
			Expect(c.Entries()).To(Equal(1),
				"Entries must count only content hashes, not virtual page tracking — "+
					"virtual pages are a separate concern from content hashes")
		})
	})
})
