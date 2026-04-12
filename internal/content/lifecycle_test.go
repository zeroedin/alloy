package content_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

var _ = Describe("FilterByLifecycle", func() {

	var (
		now    time.Time
		past   time.Time
		future time.Time
	)

	BeforeEach(func() {
		now = time.Date(2026, 4, 11, 0, 0, 0, 0, time.UTC)
		past = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
		future = time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	})

	// ── Draft filtering ───────────────────────────────────────────────

	Context("Draft filtering", func() {
		It("excludes draft:true pages from build output", func() {
			pages := []*content.Page{
				{Draft: true, FrontMatter: map[string]interface{}{"title": "Draft Post"}},
				{Draft: false, FrontMatter: map[string]interface{}{"title": "Published Post"}},
			}
			result := content.FilterByLifecycle(pages, now, false)
			Expect(result).To(HaveLen(1))
			Expect(result[0].FrontMatter["title"]).To(Equal("Published Post"))
		})

		It("includes draft:true pages in dev mode", func() {
			pages := []*content.Page{
				{Draft: true, FrontMatter: map[string]interface{}{"title": "Draft Post"}},
				{Draft: false, FrontMatter: map[string]interface{}{"title": "Published Post"}},
			}
			result := content.FilterByLifecycle(pages, now, true)
			Expect(result).To(HaveLen(2))
		})

		It("includes non-draft pages in all modes", func() {
			pages := []*content.Page{
				{Draft: false, FrontMatter: map[string]interface{}{"title": "Published"}},
			}

			buildResult := content.FilterByLifecycle(pages, now, false)
			Expect(buildResult).To(HaveLen(1))

			devResult := content.FilterByLifecycle(pages, now, true)
			Expect(devResult).To(HaveLen(1))
		})
	})

	// ── publishDate filtering ─────────────────────────────────────────

	Context("publishDate filtering", func() {
		It("excludes pages with future publishDate from build", func() {
			pages := []*content.Page{
				{PublishDate: &future, FrontMatter: map[string]interface{}{"title": "Future Post"}},
				{PublishDate: &past, FrontMatter: map[string]interface{}{"title": "Past Post"}},
			}
			result := content.FilterByLifecycle(pages, now, false)
			// The future post must be excluded, but the past post must be included
			Expect(result).To(HaveLen(1))
			Expect(result[0].FrontMatter["title"]).To(Equal("Past Post"))
		})

		It("includes pages with past publishDate", func() {
			pages := []*content.Page{
				{PublishDate: &past, FrontMatter: map[string]interface{}{"title": "Past Post"}},
			}
			result := content.FilterByLifecycle(pages, now, false)
			Expect(result).To(HaveLen(1))
			Expect(result[0].FrontMatter["title"]).To(Equal("Past Post"))
		})

		It("includes pages with no publishDate", func() {
			pages := []*content.Page{
				{PublishDate: nil, FrontMatter: map[string]interface{}{"title": "Undated Post"}},
			}
			result := content.FilterByLifecycle(pages, now, false)
			Expect(result).To(HaveLen(1))
			Expect(result[0].FrontMatter["title"]).To(Equal("Undated Post"))
		})
	})

	// ── expiryDate filtering ──────────────────────────────────────────

	Context("expiryDate filtering", func() {
		It("excludes pages with past expiryDate from build", func() {
			pages := []*content.Page{
				{ExpiryDate: &past, FrontMatter: map[string]interface{}{"title": "Expired Post"}},
				{ExpiryDate: &future, FrontMatter: map[string]interface{}{"title": "Valid Post"}},
			}
			result := content.FilterByLifecycle(pages, now, false)
			// The expired post must be excluded, but the valid post must be included
			Expect(result).To(HaveLen(1))
			Expect(result[0].FrontMatter["title"]).To(Equal("Valid Post"))
		})

		It("includes pages with future expiryDate", func() {
			pages := []*content.Page{
				{ExpiryDate: &future, FrontMatter: map[string]interface{}{"title": "Valid Post"}},
			}
			result := content.FilterByLifecycle(pages, now, false)
			Expect(result).To(HaveLen(1))
			Expect(result[0].FrontMatter["title"]).To(Equal("Valid Post"))
		})

		It("includes pages with no expiryDate", func() {
			pages := []*content.Page{
				{ExpiryDate: nil, FrontMatter: map[string]interface{}{"title": "Permanent Post"}},
			}
			result := content.FilterByLifecycle(pages, now, false)
			Expect(result).To(HaveLen(1))
			Expect(result[0].FrontMatter["title"]).To(Equal("Permanent Post"))
		})
	})

	// ── publishDate filtering in serve mode ──────────────────────────

	Context("publishDate filtering in serve mode", func() {
		It("excludes pages with future publishDate from serve (not just build)", func() {
			pages := []*content.Page{
				{PublishDate: &future, FrontMatter: map[string]interface{}{"title": "Future Post"}},
				{PublishDate: &past, FrontMatter: map[string]interface{}{"title": "Past Post"}},
			}
			// devMode=true, but future publishDate must still be excluded
			result := content.FilterByLifecycle(pages, now, true)
			Expect(result).To(HaveLen(1),
				"future publishDate must be excluded from serve, not just build")
			Expect(result[0].FrontMatter["title"]).To(Equal("Past Post"))
		})
	})

	// ── expiryDate filtering in serve mode ───────────────────────────

	Context("expiryDate filtering in serve mode", func() {
		It("excludes pages with past expiryDate from serve (not just build)", func() {
			pages := []*content.Page{
				{ExpiryDate: &past, FrontMatter: map[string]interface{}{"title": "Expired Post"}},
				{ExpiryDate: &future, FrontMatter: map[string]interface{}{"title": "Valid Post"}},
			}
			// devMode=true, but expired pages must still be excluded
			result := content.FilterByLifecycle(pages, now, true)
			Expect(result).To(HaveLen(1),
				"past expiryDate must be excluded from serve, not just build")
			Expect(result[0].FrontMatter["title"]).To(Equal("Valid Post"))
		})
	})

	// ── Draft pages ignore date constraints in dev mode ──────────────

	Context("Draft pages ignore date constraints in dev mode", func() {
		It("draft page with future publishDate is visible in dev mode", func() {
			pages := []*content.Page{
				{Draft: true, PublishDate: &future, FrontMatter: map[string]interface{}{"title": "Draft Future"}},
			}
			result := content.FilterByLifecycle(pages, now, true)
			Expect(result).To(HaveLen(1),
				"draft:true must override publishDate filtering in dev mode")
		})

		It("draft page with past expiryDate is visible in dev mode", func() {
			pages := []*content.Page{
				{Draft: true, ExpiryDate: &past, FrontMatter: map[string]interface{}{"title": "Draft Expired"}},
			}
			result := content.FilterByLifecycle(pages, now, true)
			Expect(result).To(HaveLen(1),
				"draft:true must override expiryDate filtering in dev mode")
		})

		It("draft page with future publishDate is excluded from build", func() {
			pages := []*content.Page{
				{Draft: true, PublishDate: &future, FrontMatter: map[string]interface{}{"title": "Draft Future"}},
				{Draft: false, FrontMatter: map[string]interface{}{"title": "Published"}},
			}
			// Guard: non-draft page must survive filtering
			result := content.FilterByLifecycle(pages, now, false)
			Expect(result).To(HaveLen(1),
				"guard: non-draft page must be included in build output")
			Expect(result[0].FrontMatter["title"]).To(Equal("Published"),
				"only the non-draft page must remain after build filtering")
		})
	})
})
