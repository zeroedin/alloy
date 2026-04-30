package cache_test

import (
	"os"
	"path/filepath"

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

	// ── Persistence (save/load to .alloy/) ───────────────────────────

	Describe("Persistence", func() {
		It("SaveTo writes cache.json to the target directory", func() {
			tmpDir := GinkgoT().TempDir()
			cacheDir := filepath.Join(tmpDir, ".alloy")

			c := cache.New()
			c.SetHash("content/index.md", "hash1")
			c.SetHash("content/about.md", "hash2")

			err := c.SaveTo(cacheDir)
			Expect(err).NotTo(HaveOccurred(),
				"SaveTo must write cache.json without error")

			// cache.json must exist on disk
			_, err = os.Stat(filepath.Join(cacheDir, "cache.json"))
			Expect(err).NotTo(HaveOccurred(),
				"cache.json must be created in the target directory")
		})

		It("LoadFrom restores all entries from cache.json", func() {
			tmpDir := GinkgoT().TempDir()
			cacheDir := filepath.Join(tmpDir, ".alloy")

			// Save a cache with entries
			original := cache.New()
			original.SetHash("content/index.md", "aaa")
			original.SetHash("content/about.md", "bbb")
			err := original.SaveTo(cacheDir)
			Expect(err).NotTo(HaveOccurred())

			// Load it back
			restored, err := cache.LoadFrom(cacheDir)
			Expect(err).NotTo(HaveOccurred(),
				"LoadFrom must read cache.json without error")
			Expect(restored).NotTo(BeNil())
			Expect(restored.GetHash("content/index.md")).To(Equal("aaa"),
				"restored cache must contain original entries")
			Expect(restored.GetHash("content/about.md")).To(Equal("bbb"))
		})

		It("LoadFrom returns empty cache when cache.json does not exist", func() {
			tmpDir := GinkgoT().TempDir()
			emptyDir := filepath.Join(tmpDir, ".alloy-missing")

			c, err := cache.LoadFrom(emptyDir)
			Expect(err).NotTo(HaveOccurred(),
				"missing cache.json is a fresh build, not an error")
			Expect(c).NotTo(BeNil())
			Expect(c.Entries()).To(Equal(0),
				"fresh cache must have zero entries")
		})

		It("SaveTo creates the .alloy directory if it does not exist", func() {
			tmpDir := GinkgoT().TempDir()
			cacheDir := filepath.Join(tmpDir, ".alloy", "nested")

			c := cache.New()
			c.SetHash("content/index.md", "hash1")
			err := c.SaveTo(cacheDir)
			Expect(err).NotTo(HaveOccurred(),
				"SaveTo must create missing directories")

			info, err := os.Stat(cacheDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.IsDir()).To(BeTrue(),
				"target directory must be created")
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
	})
})
