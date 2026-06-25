package collection_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/content"
)

var _ = Describe("Collection", func() {

	var (
		pages        []*content.Page
		permalinkCfg map[string]string
	)

	BeforeEach(func() {
		pages = []*content.Page{
			{Section: "blog", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Post 1"}},
			{Section: "blog", Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Post 2"}},
			{Section: "blog", Draft: true, FrontMatter: map[string]interface{}{"title": "Draft"}},
			{Section: "docs", FrontMatter: map[string]interface{}{"title": "Getting Started"}},
		}
		permalinkCfg = map[string]string{"blog": "/:year/:month/:slug/", "default": "/:slug/"}
	})

	// ── Blog collection creation ───────────────────────────────────────

	Describe("Blog collection creation", func() {
		It("creates collection for section with date-based permalink pattern (key contains :year)", func() {
			collections := collection.BuildCollections(pages, permalinkCfg, nil)
			Expect(collections).NotTo(BeNil())
			Expect(collections).To(HaveKey("blog"))
		})

		It("does not create collection for section without date tokens", func() {
			collections := collection.BuildCollections(pages, permalinkCfg, nil)
			Expect(collections).NotTo(BeNil())
			Expect(collections).NotTo(HaveKey("docs"))
		})

		It("collects all child pages into the section collection", func() {
			collections := collection.BuildCollections(pages, permalinkCfg, nil)
			Expect(collections).NotTo(BeNil())
			Expect(collections).To(HaveKey("blog"))
			// Non-draft blog pages: Post 1 and Post 2
			Expect(collections["blog"].Pages).To(HaveLen(2))
		})
	})

	// ── Default sorting ────────────────────────────────────────────────

	Describe("Default sorting", func() {
		It("sorts by date descending (newest first)", func() {
			blogPages := []*content.Page{
				{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Older"}},
				{Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Newer"}},
			}
			sorted := collection.SortPages(blogPages, "date", "desc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(2))
			Expect(sorted[0].FrontMatter["title"]).To(Equal("Newer"))
			Expect(sorted[1].FrontMatter["title"]).To(Equal("Older"))
		})

		It("dateless pages sort after dated pages", func() {
			mixedPages := []*content.Page{
				{FrontMatter: map[string]interface{}{"title": "No Date"}},
				{Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Has Date"}},
			}
			sorted := collection.SortPages(mixedPages, "date", "desc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(2))
			Expect(sorted[0].FrontMatter["title"]).To(Equal("Has Date"))
			Expect(sorted[1].FrontMatter["title"]).To(Equal("No Date"))
		})
	})

	// ── Custom sorting ─────────────────────────────────────────────────

	Describe("Custom sorting", func() {
		It("sorts ascending when order is asc", func() {
			blogPages := []*content.Page{
				{Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Newer"}},
				{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Older"}},
			}
			sorted := collection.SortPages(blogPages, "date", "asc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(2))
			Expect(sorted[0].FrontMatter["title"]).To(Equal("Older"))
			Expect(sorted[1].FrontMatter["title"]).To(Equal("Newer"))
		})
	})

	// ── Collection data ────────────────────────────────────────────────

	Describe("Collection data", func() {
		It("draft pages are excluded from collections", func() {
			collections := collection.BuildCollections(pages, permalinkCfg, nil)
			Expect(collections).NotTo(BeNil())
			Expect(collections).To(HaveKey("blog"))
			for _, p := range collections["blog"].Pages {
				Expect(p.Draft).To(BeFalse())
			}
		})
	})

	// ── Custom sort order from config ─────────────────────────────────

	Describe("Custom sort order from config", func() {
		It("sorts by front matter field (e.g., weight) when configured", func() {
			weightPages := []*content.Page{
				{FrontMatter: map[string]interface{}{"title": "Heavy", "weight": 10}},
				{FrontMatter: map[string]interface{}{"title": "Light", "weight": 1}},
				{FrontMatter: map[string]interface{}{"title": "Medium", "weight": 5}},
			}
			sorted := collection.SortByFrontMatter(weightPages, "weight", "asc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(3))
			Expect(sorted[0].FrontMatter["title"]).To(Equal("Light"),
				"weight=1 must sort first in ascending order")
			Expect(sorted[2].FrontMatter["title"]).To(Equal("Heavy"),
				"weight=10 must sort last in ascending order")
		})
	})

	// ── Default sort order verification ───────────────────────────────

	Describe("Default sort order verification", func() {
		It("default sort is date descending with dateless pages after dated ones", func() {
			mixedPages := []*content.Page{
				{FrontMatter: map[string]interface{}{"title": "No Date A"}},
				{Date: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Old"}},
				{FrontMatter: map[string]interface{}{"title": "No Date B"}},
				{Date: time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Newest"}},
				{Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Middle"}},
			}
			sorted := collection.SortPages(mixedPages, "date", "desc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(5))
			// Dated pages first, newest to oldest
			Expect(sorted[0].FrontMatter["title"]).To(Equal("Newest"))
			Expect(sorted[1].FrontMatter["title"]).To(Equal("Middle"))
			Expect(sorted[2].FrontMatter["title"]).To(Equal("Old"))
			// Dateless pages after all dated pages
			datelessTitles := []interface{}{
				sorted[3].FrontMatter["title"],
				sorted[4].FrontMatter["title"],
			}
			Expect(datelessTitles).To(ConsistOf("No Date A", "No Date B"),
				"dateless pages must appear after all dated pages")
		})
	})

	// ── Collection freezing ───────────────────────────────────────────

	Describe("Collection freezing", func() {
		It("frozen collection rejects AddPage", func() {
			c := &collection.Collection{
				Name:  "blog",
				Pages: []*content.Page{{FrontMatter: map[string]interface{}{"title": "Initial"}}},
			}
			// Guard: AddPage must work before freeze
			err := c.AddPage(&content.Page{FrontMatter: map[string]interface{}{"title": "Second"}})
			Expect(err).NotTo(HaveOccurred(),
				"guard: AddPage must succeed before freeze")

			c.Freeze()
			Expect(c.IsFrozen()).To(BeTrue(), "collection must be frozen after Freeze()")

			err = c.AddPage(&content.Page{FrontMatter: map[string]interface{}{"title": "Rejected"}})
			Expect(err).To(HaveOccurred(),
				"adding to a frozen collection must return an error")
		})
	})

	// ── Sort tiebreaker: filename ascending (§3) ────────────────────

	Describe("Sort tiebreaker", func() {
		It("uses full datetime comparison when dates are equal", func() {
			sameDayPages := []*content.Page{
				{Date: time.Date(2026, 3, 15, 14, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Afternoon"}, RelPath: "blog/afternoon.md"},
				{Date: time.Date(2026, 3, 15, 9, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Morning"}, RelPath: "blog/morning.md"},
			}
			sorted := collection.SortPages(sameDayPages, "date", "desc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(2))
			Expect(sorted[0].FrontMatter["title"]).To(Equal("Afternoon"),
				"full datetime must be compared, not just date — afternoon (14:00) before morning (09:00) in desc order")
		})

		It("uses filename alphabetical ascending as final tiebreaker", func() {
			identicalDatePages := []*content.Page{
				{Date: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Zebra"}, RelPath: "blog/zebra-post.md"},
				{Date: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Alpha"}, RelPath: "blog/alpha-post.md"},
				{Date: time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Middle"}, RelPath: "blog/middle-post.md"},
			}
			sorted := collection.SortPages(identicalDatePages, "date", "desc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(3))
			// All dates identical — tiebreaker is filename alphabetical ascending
			Expect(sorted[0].RelPath).To(Equal("blog/alpha-post.md"),
				"when dates are identical, filename alphabetical ascending is the tiebreaker")
			Expect(sorted[1].RelPath).To(Equal("blog/middle-post.md"))
			Expect(sorted[2].RelPath).To(Equal("blog/zebra-post.md"))
		})

		It("undated pages are ordered by filename ascending among themselves", func() {
			undatedPages := []*content.Page{
				{FrontMatter: map[string]interface{}{"title": "Zulu"}, RelPath: "docs/zulu.md"},
				{FrontMatter: map[string]interface{}{"title": "Alpha"}, RelPath: "docs/alpha.md"},
				{FrontMatter: map[string]interface{}{"title": "Mike"}, RelPath: "docs/mike.md"},
			}
			sorted := collection.SortPages(undatedPages, "date", "desc")
			Expect(sorted).NotTo(BeNil())
			Expect(sorted).To(HaveLen(3))
			Expect(sorted[0].RelPath).To(Equal("docs/alpha.md"),
				"undated pages must be sorted by filename ascending")
			Expect(sorted[1].RelPath).To(Equal("docs/mike.md"))
			Expect(sorted[2].RelPath).To(Equal("docs/zulu.md"))
		})
	})

	// ── Collection lifecycle interactions (§1f + §3) ────────────────

	Describe("Collection lifecycle interactions", func() {
		It("drafts included in collections in serve mode", func() {
			draftPages := []*content.Page{
				{Section: "blog", Draft: true, FrontMatter: map[string]interface{}{"title": "Draft Post"}},
				{Section: "blog", Draft: false, FrontMatter: map[string]interface{}{"title": "Published Post"}},
			}
			collections := collection.BuildCollectionsWithMode(draftPages, map[string]string{"blog": "/:year/:month/:slug/"}, nil, true)
			Expect(collections).NotTo(BeNil())
			Expect(collections).To(HaveKey("blog"))
			Expect(collections["blog"].Pages).To(HaveLen(2),
				"serve mode (devMode=true) must include drafts in collections")
		})

		It("future publishDate pages excluded from collections in both modes", func() {
			future := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
			futurePages := []*content.Page{
				{Section: "blog", PublishDate: &future, FrontMatter: map[string]interface{}{"title": "Future Post"}},
				{Section: "blog", FrontMatter: map[string]interface{}{"title": "Normal Post"}},
			}
			// Build mode
			buildCollections := collection.BuildCollectionsWithMode(futurePages, map[string]string{"blog": "/:year/:month/:slug/"}, nil, false)
			Expect(buildCollections).To(HaveKey("blog"))
			Expect(buildCollections["blog"].Pages).To(HaveLen(1),
				"future publishDate must be excluded from collections in build mode")

			// Serve mode
			serveCollections := collection.BuildCollectionsWithMode(futurePages, map[string]string{"blog": "/:year/:month/:slug/"}, nil, true)
			Expect(serveCollections).To(HaveKey("blog"))
			Expect(serveCollections["blog"].Pages).To(HaveLen(1),
				"future publishDate must be excluded from collections in serve mode too")
		})

		It("pagination operates on lifecycle-filtered collection", func() {
			past := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			allPages := []*content.Page{
				{Section: "blog", Draft: true, FrontMatter: map[string]interface{}{"title": "Draft"}},
				{Section: "blog", ExpiryDate: &past, FrontMatter: map[string]interface{}{"title": "Expired"}},
				{Section: "blog", FrontMatter: map[string]interface{}{"title": "Active 1"}},
				{Section: "blog", FrontMatter: map[string]interface{}{"title": "Active 2"}},
			}
			collections := collection.BuildCollectionsWithMode(allPages, map[string]string{"blog": "/:year/:month/:slug/"}, nil, false)
			Expect(collections).To(HaveKey("blog"))
			Expect(collections["blog"].Pages).To(HaveLen(2),
				"collection used for pagination must only contain lifecycle-filtered pages")
		})
	})

	// ── Explicit collection membership (issue #766) ──────────────────

	Describe("Explicit collection membership", func() {
		It("creates collection for section listed in collectionNames even without date tokens", func() {
			pages := []*content.Page{
				{Section: "releases", RelPath: "releases/v1.md", FrontMatter: map[string]interface{}{"title": "v1.0"}},
				{Section: "releases", RelPath: "releases/v2.md", FrontMatter: map[string]interface{}{"title": "v2.0"}},
				{Section: "docs", RelPath: "docs/intro.md", FrontMatter: map[string]interface{}{"title": "Intro"}},
			}
			permalinkCfg := map[string]string{"default": "/:slug/"}
			collectionNames := []string{"releases"}

			collections := collection.BuildCollections(pages, permalinkCfg, collectionNames)

			Expect(collections).To(HaveKey("releases"),
				"section listed in collectionNames must become a collection even without date tokens in permalink")
			Expect(collections["releases"].Pages).To(HaveLen(2))
			Expect(collections).NotTo(HaveKey("docs"),
				"section not in collectionNames and without date tokens must not become a collection")
		})

		It("creates collection from date tokens when collectionNames is nil", func() {
			pages := []*content.Page{
				{Section: "blog", RelPath: "blog/post.md", FrontMatter: map[string]interface{}{"title": "Post"}},
			}
			permalinkCfg := map[string]string{"blog": "/:year/:month/:slug/"}

			collections := collection.BuildCollections(pages, permalinkCfg, nil)

			Expect(collections).To(HaveKey("blog"),
				"date-token-based collection must still work when collectionNames is nil")
		})

		It("combines date-token and explicit membership without duplicating pages", func() {
			pages := []*content.Page{
				{Section: "blog", RelPath: "blog/post1.md", Date: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Post 1"}},
				{Section: "blog", RelPath: "blog/post2.md", Date: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), FrontMatter: map[string]interface{}{"title": "Post 2"}},
			}
			permalinkCfg := map[string]string{"blog": "/:year/:month/:slug/"}
			collectionNames := []string{"blog"}

			collections := collection.BuildCollections(pages, permalinkCfg, collectionNames)

			Expect(collections).To(HaveKey("blog"))
			Expect(collections["blog"].Pages).To(HaveLen(2),
				"section qualifying via both mechanisms must not duplicate pages")
		})

		It("excludes drafts from explicitly-defined collections", func() {
			pages := []*content.Page{
				{Section: "releases", RelPath: "releases/v1.md", FrontMatter: map[string]interface{}{"title": "v1.0"}},
				{Section: "releases", RelPath: "releases/v2.md", Draft: true, FrontMatter: map[string]interface{}{"title": "v2.0-draft"}},
			}
			permalinkCfg := map[string]string{"default": "/:slug/"}
			collectionNames := []string{"releases"}

			collections := collection.BuildCollections(pages, permalinkCfg, collectionNames)

			Expect(collections).To(HaveKey("releases"))
			Expect(collections["releases"].Pages).To(HaveLen(1),
				"drafts must be excluded from explicitly-defined collections")
		})

		It("excludes section index from explicitly-defined collections", func() {
			pages := []*content.Page{
				{Section: "releases", RelPath: "releases/index.md", FrontMatter: map[string]interface{}{"title": "Releases Index"}},
				{Section: "releases", RelPath: "releases/v1.md", FrontMatter: map[string]interface{}{"title": "v1.0"}},
			}
			permalinkCfg := map[string]string{"default": "/:slug/"}
			collectionNames := []string{"releases"}

			collections := collection.BuildCollections(pages, permalinkCfg, collectionNames)

			Expect(collections).To(HaveKey("releases"))
			Expect(collections["releases"].Pages).To(HaveLen(1),
				"section index must not be included as a collection member")
			Expect(collections["releases"].Pages[0].FrontMatter["title"]).To(Equal("v1.0"))
		})

		It("includes drafts in explicitly-defined collections in serve mode", func() {
			pages := []*content.Page{
				{Section: "releases", RelPath: "releases/v1.md", FrontMatter: map[string]interface{}{"title": "v1.0"}},
				{Section: "releases", RelPath: "releases/v2.md", Draft: true, FrontMatter: map[string]interface{}{"title": "v2.0-draft"}},
			}
			permalinkCfg := map[string]string{"default": "/:slug/"}
			collectionNames := []string{"releases"}

			collections := collection.BuildCollectionsWithMode(pages, permalinkCfg, collectionNames, true)

			Expect(collections).To(HaveKey("releases"))
			Expect(collections["releases"].Pages).To(HaveLen(2),
				"serve mode must include drafts in explicitly-defined collections")
		})
	})
})
