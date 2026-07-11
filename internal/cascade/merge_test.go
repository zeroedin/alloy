package cascade_test

import (
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/cascade"
)

func fixtureContentDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "test", "fixtures", "cascade", "content")
}

func fixtureGapContentDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "test", "fixtures", "cascade-gap", "content")
}

func fixtureOrderingContentDir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "test", "fixtures", "cascade-ordering", "content")
}

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

		// ── Multi-language cascade lookup (issue #914) ──────────────
		// In multi-language builds, cascade data keys include the language
		// prefix (e.g., "content/es/blog/"). FindCascadeData must receive
		// the full lang-prefixed RelPath (e.g., "es/blog/my-post.md") to
		// match these keys. The pipeline is responsible for passing the
		// original (un-stripped) RelPath to FindCascadeData and only
		// stripping the prefix for permalink token resolution.

		It("finds language-specific cascade data when given lang-prefixed relPath (issue #914)", func() {
			cascadeData := map[string]map[string]interface{}{
				"content/":          {"layout": "default"},
				"content/en/blog/":  {"permalink": "/posts/:slug/"},
				"content/es/blog/":  {"permalink": "/:slug/"},
			}
			// Spanish page with full lang-prefixed RelPath
			esResult := cascade.FindCascadeData(cascadeData, "content", "es/blog/my-post.md")
			Expect(esResult).NotTo(BeNil(),
				"FindCascadeData must find content/es/blog/ when given es/blog/my-post.md — "+
					"the language prefix is part of the directory tree, not a virtual namespace")
			Expect(esResult["permalink"]).To(Equal("/:slug/"),
				"Spanish cascade data must contain the Spanish permalink pattern, "+
					"not the English one or the root default")

			// English page with full lang-prefixed RelPath
			enResult := cascade.FindCascadeData(cascadeData, "content", "en/blog/my-post.md")
			Expect(enResult).NotTo(BeNil(),
				"FindCascadeData must find content/en/blog/ when given en/blog/my-post.md")
			Expect(enResult["permalink"]).To(Equal("/posts/:slug/"),
				"English cascade data must contain the English permalink pattern")

			// Stripped RelPath (wrong usage) would match content/blog/ which doesn't exist,
			// then fall back to content/ — proving the stripped path is wrong for this lookup
			strippedResult := cascade.FindCascadeData(cascadeData, "content", "blog/my-post.md")
			Expect(strippedResult).NotTo(BeNil(),
				"stripped path falls back to content/ root entry")
			Expect(strippedResult).NotTo(HaveKey("permalink"),
				"stripped path blog/my-post.md must NOT find a permalink key — "+
					"content/ has no permalink, and content/blog/ doesn't exist. "+
					"This proves that using the stripped RelPath for cascade lookup "+
					"would miss language-specific _data.yaml entries (issue #914)")
		})

		It("deep-merges nested maps across 3+ directory levels", func() {
			// 3 levels of _data.yaml with nested map keys, pre-accumulated
			// the way LoadDirectoryCascade produces them (parent merged into
			// child at each level):
			//   content/           → theme: { color: "blue", font: "sans-serif" }
			//   content/blog/      → theme: { color: "green", font: "sans-serif" }
			//   content/blog/deep/ → theme: { color: "green", font: "sans-serif", size: "large" }
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
						"font":  "sans-serif",
					},
				},
				"content/blog/deep/": {
					"theme": map[string]interface{}{
						"color": "green",
						"font":  "sans-serif",
						"size":  "large",
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
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
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
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			rootData, ok := result["content/"]
			Expect(ok).To(BeTrue(), "root _data.yaml must be loaded")
			Expect(rootData).NotTo(BeEmpty())
		})

		It("content/blog/_data.yaml merges over parent and applies to blog/ and children", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
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
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
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
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
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

	// ── FindCascadeData nearest-match optimization (issue #219) ──────
	// LoadDirectoryCascade already accumulates ancestor data into each
	// directory entry. FindCascadeData should return the nearest ancestor
	// match instead of re-merging all ancestors — the accumulated entry
	// already contains the full chain.

	Describe("FindCascadeData nearest-match (issue #219)", func() {
		It("nearest directory entry contains full ancestor chain (issue #219)", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
			Expect(err).NotTo(HaveOccurred())

			deepEntry := result["content/blog/deep/"]
			Expect(deepEntry).NotTo(BeNil(),
				"LoadDirectoryCascade must have an entry for content/blog/deep/")

			found := cascade.FindCascadeData(result, "content", "blog/deep/article.md")
			Expect(found).NotTo(BeNil())

			Expect(found).To(Equal(deepEntry),
				"FindCascadeData must return the same data as the nearest "+
					"LoadDirectoryCascade entry — LoadDirectoryCascade already "+
					"accumulates ancestor data via DeepMerge(parentData, data), "+
					"so re-merging all ancestors in FindCascadeData is redundant "+
					"(issue #219)")
		})

		It("nearest-match result contains all inherited keys from full chain (issue #219)", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
			Expect(err).NotTo(HaveOccurred())

			data := cascade.FindCascadeData(result, "content", "blog/deep/article.md")
			Expect(data).NotTo(BeNil())

			Expect(data).To(HaveKey("layout"),
				"must inherit layout from content/blog/_data.yaml (ancestor)")
			Expect(data["layout"]).To(Equal("post"))

			Expect(data).To(HaveKey("category"),
				"must have category from content/blog/deep/_data.yaml (self)")
			Expect(data["category"]).To(Equal("deep-dive"))

			author, ok := data["author"].(map[string]interface{})
			Expect(ok).To(BeTrue())
			Expect(author["name"]).To(Equal("Deep Author"),
				"deep/ author.name must override blog/ author.name")
			Expect(author["twitter"]).To(Equal("@blogauthor"),
				"deep/ must preserve author.twitter inherited from blog/ — "+
					"all ancestor keys must be present in the nearest-match "+
					"entry without re-merging (issue #219)")
		})

		It("nearest-match for directory without _data.yaml walks up correctly (issue #219)", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureContentDir())
			Expect(err).NotTo(HaveOccurred())

			data := cascade.FindCascadeData(result, "content", "blog/deep/nested/leaf.md")
			Expect(data).NotTo(BeNil(),
				"page in directory without _data.yaml must inherit from nearest ancestor")

			deepEntry := result["content/blog/deep/"]
			Expect(data).To(Equal(deepEntry),
				"FindCascadeData for blog/deep/nested/leaf.md must return the "+
					"content/blog/deep/ entry — the nearest ancestor with data, "+
					"already containing the full accumulated chain (issue #219)")
		})
	})

	// ── Non-contiguous _data.yaml chains and WalkDir ordering (issue #708) ──
	// Tests exercise two edge cases in LoadDirectoryCascade:
	// 1. Gap scenario: intermediate directory has no _data.yaml but parent
	//    and grandchild do — accumulation must bridge the gap.
	// 2. WalkDir ordering: directories like "1-posts/" sort before "_data.yaml"
	//    in lexical order, so WalkDir visits child _data.yaml before parent.
	//    Two-pass accumulation handles this correctly.

	Describe("Non-contiguous _data.yaml chains (issue #708)", func() {
		It("LoadDirectoryCascade bridges gap in _data.yaml chain", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureGapContentDir())
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(HaveKey("content/"),
				"root _data.yaml must be loaded")
			Expect(result).To(HaveKey("content/docs/advanced/"),
				"grandchild _data.yaml must be loaded despite missing intermediate")
			Expect(result).NotTo(HaveKey("content/docs/"),
				"intermediate directory without _data.yaml must not have an entry")
		})

		It("grandchild entry inherits root data across gap", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureGapContentDir())
			Expect(err).NotTo(HaveOccurred())

			advanced := result["content/docs/advanced/"]
			Expect(advanced).NotTo(BeNil())

			Expect(advanced["layout"]).To(Equal("default"),
				"grandchild must inherit layout from root across the gap — "+
					"LoadDirectoryCascade must walk up past content/docs/ (no entry) "+
					"to content/ (issue #708)")
			Expect(advanced["theme"]).To(Equal("light"),
				"grandchild must inherit theme from root across the gap")
			Expect(advanced["category"]).To(Equal("advanced"),
				"grandchild must retain its own category key")
		})

		It("FindCascadeData returns root data for pages in gap directory", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureGapContentDir())
			Expect(err).NotTo(HaveOccurred())

			data := cascade.FindCascadeData(result, "content", "docs/some-page.md")
			Expect(data).NotTo(BeNil(),
				"page in directory without _data.yaml must inherit from nearest "+
					"ancestor — FindCascadeData must walk up past docs/ to content/")

			rootEntry := result["content/"]
			Expect(data).To(Equal(rootEntry),
				"FindCascadeData for docs/some-page.md must return content/ entry — "+
					"the nearest ancestor with data (issue #708)")
		})

		It("FindCascadeData returns accumulated grandchild data across gap", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureGapContentDir())
			Expect(err).NotTo(HaveOccurred())

			data := cascade.FindCascadeData(result, "content", "docs/advanced/guide.md")
			Expect(data).NotTo(BeNil())

			Expect(data["layout"]).To(Equal("default"),
				"page in grandchild must see layout inherited from root across gap")
			Expect(data["category"]).To(Equal("advanced"),
				"page in grandchild must see its own category")

			advancedEntry := result["content/docs/advanced/"]
			Expect(data).To(Equal(advancedEntry),
				"FindCascadeData must return the accumulated grandchild entry — "+
					"it already contains root data bridged across the gap (issue #708)")
		})
	})

	Describe("WalkDir ordering with lexically-early directory names (issue #708)", func() {
		It("LoadDirectoryCascade accumulates correctly when child sorts before parent _data.yaml", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureOrderingContentDir())
			Expect(err).NotTo(HaveOccurred())

			Expect(result).To(HaveKey("content/"),
				"root _data.yaml must be loaded")
			Expect(result).To(HaveKey("content/1-posts/"),
				"1-posts/ _data.yaml must be loaded")
		})

		It("child entry inherits parent data despite WalkDir processing child first", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureOrderingContentDir())
			Expect(err).NotTo(HaveOccurred())

			posts := result["content/1-posts/"]
			Expect(posts).NotTo(BeNil())

			Expect(posts["layout"]).To(Equal("default"),
				"1-posts/ must inherit layout from root — WalkDir visits "+
					"1-posts/_data.yaml before content/_data.yaml because '1' < '_', "+
					"but two-pass accumulation ensures parent data is available (issue #708)")
			Expect(posts["section"]).To(Equal("posts"),
				"1-posts/ must retain its own section key")
		})

		It("FindCascadeData returns accumulated data for pages in ordering-sensitive directory", func() {
			result, err := cascade.LoadDirectoryCascade(fixtureOrderingContentDir())
			Expect(err).NotTo(HaveOccurred())

			data := cascade.FindCascadeData(result, "content", "1-posts/article.md")
			Expect(data).NotTo(BeNil())

			Expect(data["layout"]).To(Equal("default"),
				"page in 1-posts/ must see inherited layout from root")
			Expect(data["section"]).To(Equal("posts"),
				"page in 1-posts/ must see its own section")

			postsEntry := result["content/1-posts/"]
			Expect(data).To(Equal(postsEntry),
				"FindCascadeData must return the accumulated 1-posts/ entry (issue #708)")
		})
	})
})
