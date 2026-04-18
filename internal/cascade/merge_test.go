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

		It("deep-merges nested maps across 3+ directory levels", func() {
			// 3 levels of _data.yaml with nested map keys:
			//   content/           → theme: { color: "blue", font: "sans-serif" }
			//   content/blog/      → theme: { color: "green" }  (overrides color, font inherited)
			//   content/blog/deep/ → theme: { size: "large" }   (adds size, color+font inherited)
			cascadeData := map[string]map[string]interface{}{
				"content/": {
					"theme": map[string]interface{}{
						"color": "blue",
						"font":  "sans-serif",
					},
				},
				"content/blog/": {
					"theme": map[string]interface{}{
						"color": "green",
					},
				},
				"content/blog/deep/": {
					"theme": map[string]interface{}{
						"size": "large",
					},
				},
			}

			result := cascade.FindCascadeData(cascadeData, "content", "blog/deep/post.md")
			Expect(result).NotTo(BeNil())

			theme, ok := result["theme"].(map[string]interface{})
			Expect(ok).To(BeTrue(),
				"theme must be a nested map after cascade merge")

			// color: "green" from blog/ (overrides root "blue")
			Expect(theme["color"]).To(Equal("green"),
				"nested key theme.color must be overridden by blog/_data.yaml")

			// font: "sans-serif" from content/ (inherited — blog/ didn't override it)
			Expect(theme["font"]).To(Equal("sans-serif"),
				"nested key theme.font must be inherited from root — "+
					"deep merge must preserve keys not overridden by descendants")

			// size: "large" from blog/deep/ (new key at deepest level)
			Expect(theme["size"]).To(Equal("large"),
				"nested key theme.size must be added from blog/deep/_data.yaml")
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

			// blog/ overrides layout and author.name from root
			Expect(blogData["layout"]).To(Equal("post"),
				"blog/ must override layout from root")

			// blog/ adds author.twitter while inheriting author as nested object
			author, ok := blogData["author"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "author must be a map after deep merge")
			Expect(author["name"]).To(Equal("Blog Author"),
				"blog/ must override author.name from root")
			Expect(author["twitter"]).To(Equal("@blogauthor"),
				"blog/ must add author.twitter")

			// blog/ replaces scripts array entirely (not concatenation)
			Expect(blogData["scripts"]).To(Equal([]interface{}{"blog.js"}),
				"blog/ scripts must replace root scripts, not concatenate")
		})

		It("content/blog/deep/_data.yaml merges over parent chain (root + blog)", func() {
			result, err := cascade.LoadDirectoryCascade("test/fixtures/cascade/content")
			Expect(err).NotTo(HaveOccurred())

			deepData, ok := result["content/blog/deep/"]
			Expect(ok).To(BeTrue(), "deeply nested _data.yaml must be loaded")
			Expect(deepData).NotTo(BeNil())

			// deep/ overrides only author.name — everything else must survive
			// from the full parent chain (root → blog → deep)
			author, ok := deepData["author"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "author must be a map after 3-level deep merge")
			Expect(author["name"]).To(Equal("Deep Author"),
				"deep/ must override author.name")
			Expect(author["twitter"]).To(Equal("@blogauthor"),
				"deep/ must preserve author.twitter from blog/ (not replaced by deep/ override)")

			// layout was set at blog/ level, not overridden at deep/ — must survive
			Expect(deepData["layout"]).To(Equal("post"),
				"deep/ must inherit layout from blog/ when not overridden")

			// scripts was set at blog/ level, not overridden at deep/ — must survive
			Expect(deepData["scripts"]).To(Equal([]interface{}{"blog.js"}),
				"deep/ must inherit scripts array from blog/ when not overridden")

			// category is new at deep/ level — must be present
			Expect(deepData["category"]).To(Equal("deep-dive"),
				"deep/ must add category key from its own _data.yaml")
		})

		It("three-level cascade produces correct merged result at each level", func() {
			result, err := cascade.LoadDirectoryCascade("test/fixtures/cascade/content")
			Expect(err).NotTo(HaveOccurred())

			// Must have entries for all three levels with _data.yaml
			Expect(result).To(HaveKey("content/"),
				"root level must be in cascade")
			Expect(result).To(HaveKey("content/blog/"),
				"blog level must be in cascade")
			Expect(result).To(HaveKey("content/blog/deep/"),
				"deep level must be in cascade")
		})
	})
})
