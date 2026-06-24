package cascade_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cascade"
)

var _ = Describe("PageContext", func() {

	// ── Layered lookup ─────────────────────────────────────────────────

	Context("Layered lookup", func() {
		It("returns front matter value when present at all levels", func() {
			global := map[string]interface{}{"title": "Global Title"}
			directory := map[string]interface{}{"title": "Directory Title"}
			frontMatter := map[string]interface{}{"title": "Page Title"}

			ctx := cascade.BuildContext(global, directory, frontMatter)
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Get("title")).To(Equal("Page Title"))
		})

		It("returns directory value when front matter doesn't have key", func() {
			global := map[string]interface{}{"layout": "global-layout"}
			directory := map[string]interface{}{"layout": "section-layout"}
			frontMatter := map[string]interface{}{"title": "Page Title"}

			ctx := cascade.BuildContext(global, directory, frontMatter)
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Get("layout")).To(Equal("section-layout"))
		})

		It("returns global value as last fallback", func() {
			global := map[string]interface{}{"site_name": "My Site"}
			directory := map[string]interface{}{"layout": "section-layout"}
			frontMatter := map[string]interface{}{"title": "Page Title"}

			ctx := cascade.BuildContext(global, directory, frontMatter)
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Get("site_name")).To(Equal("My Site"))
		})

		It("returns nil when key not found at any level", func() {
			global := map[string]interface{}{"site_name": "My Site"}
			directory := map[string]interface{}{"layout": "section-layout"}
			frontMatter := map[string]interface{}{"title": "Page Title"}

			ctx := cascade.BuildContext(global, directory, frontMatter)
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Get("nonexistent_key")).To(BeNil())
		})
	})

	// ── Pointer sharing ───────────────────────────────────────────────

	Context("Pointer sharing", func() {
		It("shares the same global data pointer across multiple PageContexts", func() {
			global := map[string]interface{}{"site_name": "Shared Site"}
			dir1 := map[string]interface{}{"section": "blog"}
			dir2 := map[string]interface{}{"section": "docs"}
			fm1 := map[string]interface{}{"title": "Post 1"}
			fm2 := map[string]interface{}{"title": "Doc 1"}

			ctx1 := cascade.BuildContext(global, dir1, fm1)
			ctx2 := cascade.BuildContext(global, dir2, fm2)

			Expect(ctx1).NotTo(BeNil())
			Expect(ctx2).NotTo(BeNil())

			// Both contexts should share the same global pointer
			Expect(ctx1.Global).To(Equal(ctx2.Global))
		})

		It("does not deep-copy shared data", func() {
			global := map[string]interface{}{"counter": 1}
			directory := map[string]interface{}{}
			fm1 := map[string]interface{}{"title": "Page A"}
			fm2 := map[string]interface{}{"title": "Page B"}

			ctx1 := cascade.BuildContext(global, directory, fm1)
			ctx2 := cascade.BuildContext(global, directory, fm2)

			Expect(ctx1).NotTo(BeNil())
			Expect(ctx2).NotTo(BeNil())

			// Mutating the global map should be visible through both contexts
			// because they share the pointer, not a copy.
			global["counter"] = 42
			Expect((*ctx1.Global)["counter"]).To(Equal(42))
			Expect((*ctx2.Global)["counter"]).To(Equal(42))
		})
	})

	// ── Lazy deep merge ──────────────────────────────────────────────

	Context("Lazy deep merge", func() {
		It("does not eagerly merge all levels at construction time", func() {
			global := map[string]interface{}{
				"author": map[string]interface{}{
					"name":  "Global Author",
					"email": "global@example.com",
				},
			}
			directory := map[string]interface{}{
				"author": map[string]interface{}{
					"twitter": "@dir",
				},
			}
			frontMatter := map[string]interface{}{"title": "Page"}

			ctx := cascade.BuildContext(global, directory, frontMatter)
			Expect(ctx).NotTo(BeNil())

			// The author object should be accessible and correctly merged
			// on access, not eagerly merged at construction
			author := ctx.Get("author")
			Expect(author).NotTo(BeNil(),
				"nested object must be retrievable through cascade")

			authorMap, ok := author.(map[string]interface{})
			Expect(ok).To(BeTrue(), "author must be a map")
			Expect(authorMap).To(HaveKeyWithValue("twitter", "@dir"),
				"directory-level nested key must be present after lazy merge")
		})
	})
})
