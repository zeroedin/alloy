package cascade_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cascade"
)

var _ = Describe("DeepMerge", func() {

	// ── Deep merge rules ───────────────────────────────────────────────

	Context("Deep merge rules", func() {
		It("deep-merges nested objects", func() {
			base := map[string]interface{}{
				"author": map[string]interface{}{
					"name":  "Global",
					"email": "g@e.com",
				},
			}
			overlay := map[string]interface{}{
				"author": map[string]interface{}{
					"twitter": "@user",
				},
			}

			result := cascade.DeepMerge(base, overlay)
			Expect(result).NotTo(BeNil())

			author, ok := result["author"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "author should be a map")
			Expect(author).To(HaveKeyWithValue("name", "Global"))
			Expect(author).To(HaveKeyWithValue("email", "g@e.com"))
			Expect(author).To(HaveKeyWithValue("twitter", "@user"))
		})

		It("replaces arrays entirely", func() {
			base := map[string]interface{}{
				"tags": []interface{}{1, 2},
			}
			overlay := map[string]interface{}{
				"tags": []interface{}{3},
			}

			result := cascade.DeepMerge(base, overlay)
			Expect(result).NotTo(BeNil())
			Expect(result["tags"]).To(Equal([]interface{}{3}))
		})

		It("overlay wins for conflicting scalar keys", func() {
			base := map[string]interface{}{
				"title": "Base Title",
				"draft": false,
			}
			overlay := map[string]interface{}{
				"title": "Overlay Title",
			}

			result := cascade.DeepMerge(base, overlay)
			Expect(result).NotTo(BeNil())
			Expect(result["title"]).To(Equal("Overlay Title"))
			Expect(result["draft"]).To(Equal(false))
		})

		It("handles nil overlay (returns base unchanged)", func() {
			base := map[string]interface{}{
				"key": "value",
			}

			result := cascade.DeepMerge(base, nil)
			Expect(result).NotTo(BeNil())
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("handles empty overlay (returns base unchanged)", func() {
			base := map[string]interface{}{
				"key": "value",
			}
			overlay := map[string]interface{}{}

			result := cascade.DeepMerge(base, overlay)
			Expect(result).NotTo(BeNil())
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})

		It("handles nil base (returns overlay)", func() {
			overlay := map[string]interface{}{
				"key": "value",
			}

			result := cascade.DeepMerge(nil, overlay)
			Expect(result).NotTo(BeNil())
			Expect(result).To(HaveKeyWithValue("key", "value"))
		})
	})

	// ── Array replacement semantics ────────────────────────────────────

	Context("Array replacement semantics", func() {
		It("front matter array replaces directory data array entirely", func() {
			base := map[string]interface{}{
				"categories": []interface{}{"news", "tech", "opinion"},
			}
			overlay := map[string]interface{}{
				"categories": []interface{}{"blog"},
			}

			result := cascade.DeepMerge(base, overlay)
			Expect(result).NotTo(BeNil())
			Expect(result["categories"]).To(Equal([]interface{}{"blog"}))
		})

		It("does not concatenate arrays", func() {
			base := map[string]interface{}{
				"items": []interface{}{"a", "b"},
			}
			overlay := map[string]interface{}{
				"items": []interface{}{"c"},
			}

			result := cascade.DeepMerge(base, overlay)
			Expect(result).NotTo(BeNil())
			// Must be ["c"], not ["a", "b", "c"]
			Expect(result["items"]).To(Equal([]interface{}{"c"}))
			Expect(result["items"]).NotTo(Equal([]interface{}{"a", "b", "c"}))
		})
	})

	// ── Ancestor cascade lookup ──────────────────────────────────────

	Describe("Ancestor cascade lookup", func() {
		It("FindCascadeData returns exact directory match when available", func() {
			cascadeData := map[string]map[string]interface{}{
				"content/":      {"layout": "default"},
				"content/blog/": {"layout": "post"},
			}
			result := cascade.FindCascadeData(cascadeData, "content", "blog/my-post.md")
			Expect(result).NotTo(BeNil())
			Expect(result["layout"]).To(Equal("post"))
		})

		It("walks up to nearest ancestor when directory has no _data.yaml", func() {
			cascadeData := map[string]map[string]interface{}{
				"content/":      {"layout": "default"},
				"content/blog/": {"layout": "post"},
			}
			// blog/2024/ has no _data.yaml — should inherit from blog/
			result := cascade.FindCascadeData(cascadeData, "content", "blog/2024/my-post.md")
			Expect(result).NotTo(BeNil())
			Expect(result["layout"]).To(Equal("post"),
				"page in subdirectory without _data.yaml must inherit from nearest ancestor")
		})

		It("walks multiple levels to find ancestor cascade data", func() {
			cascadeData := map[string]map[string]interface{}{
				"content/": {"layout": "default"},
			}
			// blog/2024/march/ has no _data.yaml, neither does blog/ — only content/ has it
			result := cascade.FindCascadeData(cascadeData, "content", "blog/2024/march/post.md")
			Expect(result).NotTo(BeNil())
			Expect(result["layout"]).To(Equal("default"),
				"deeply nested page must inherit from root when no intermediate _data.yaml exists")
		})

		It("returns nil when no ancestor has cascade data", func() {
			result := cascade.FindCascadeData(map[string]map[string]interface{}{}, "content", "blog/post.md")
			Expect(result).To(BeNil())
		})

		It("returns root cascade for root-level pages", func() {
			cascadeData := map[string]map[string]interface{}{
				"content/": {"layout": "default"},
			}
			result := cascade.FindCascadeData(cascadeData, "content", "index.md")
			Expect(result).NotTo(BeNil())
			Expect(result["layout"]).To(Equal("default"))
		})

		It("works with LoadDirectoryCascade output for directories without _data.yaml", func() {
			result, err := cascade.LoadDirectoryCascade("test/fixtures/cascade/content")
			Expect(err).NotTo(HaveOccurred())

			// content/blog/deep/nested/ has no _data.yaml
			// Nearest ancestor with data is content/blog/deep/
			data := cascade.FindCascadeData(result, "content", "blog/deep/nested/leaf.md")
			Expect(data).NotTo(BeNil(),
				"FindCascadeData must find ancestor data for directory without _data.yaml")
			Expect(data).To(HaveKey("category"),
				"must inherit category from content/blog/deep/_data.yaml")
			Expect(data["category"]).To(Equal("deep-dive"))
		})
	})

	// ── Directory data cascade chain (§3) ───────────────────────────

	Describe("Directory data cascade chain", func() {
		It("content/_data.yaml applies to all content", func() {
			result, err := cascade.LoadDirectoryCascade("test/fixtures/cascade/content")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			rootData, ok := result["content/"]
			Expect(ok).To(BeTrue(), "root _data.yaml must be loaded")
			Expect(rootData).NotTo(BeEmpty())
		})

		It("content/blog/_data.yaml merges over parent and applies to blog/ and children", func() {
			result, err := cascade.LoadDirectoryCascade("test/fixtures/cascade/content")
			Expect(err).NotTo(HaveOccurred())

			blogData, ok := result["content/blog/"]
			Expect(ok).To(BeTrue(), "blog/ _data.yaml must be loaded")
			Expect(blogData).NotTo(BeNil(),
				"blog/ data must contain merged parent + blog data")
		})

		It("content/blog/2024/_data.yaml merges over parent and applies only to blog/2024/", func() {
			result, err := cascade.LoadDirectoryCascade("test/fixtures/cascade/content")
			Expect(err).NotTo(HaveOccurred())

			deepData, ok := result["content/blog/deep/"]
			Expect(ok).To(BeTrue(), "deeply nested _data.yaml must be loaded")
			Expect(deepData).NotTo(BeNil())
		})

		It("three-level cascade produces correct merged result at each level", func() {
			result, err := cascade.LoadDirectoryCascade("test/fixtures/cascade/content")
			Expect(err).NotTo(HaveOccurred())

			// Each level should have progressively more specific data
			Expect(len(result)).To(BeNumerically(">=", 2),
				"cascade must produce results for at least root and one subdirectory")
		})
	})
})
