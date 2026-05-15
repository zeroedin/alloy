package content_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
)

var _ = Describe("ParseFrontMatter", func() {

	// ── YAML front matter (--- delimiters) ─────────────────────────────

	Context("YAML front matter (--- delimiters)", func() {
		It("extracts title from YAML front matter", func() {
			input := []byte("---\ntitle: \"My Post\"\n---\n# Content here")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("title", "My Post"))
		})

		It("returns remaining body content after front matter", func() {
			input := []byte("---\ntitle: \"My Post\"\n---\n# Content here\nMore text.")
			_, body, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("# Content here\nMore text."))
		})

		It("handles arrays (tags) in front matter", func() {
			input := []byte("---\ntitle: Tagged\ntags:\n  - go\n  - ssg\n  - alloy\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm["tags"]).To(ConsistOf("go", "ssg", "alloy"))
		})

		It("handles nested objects in front matter", func() {
			input := []byte("---\ntitle: Nested\nauthor:\n  name: Alice\n  email: alice@example.com\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			author, ok := fm["author"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "author should be a map")
			Expect(author["name"]).To(Equal("Alice"))
			Expect(author["email"]).To(Equal("alice@example.com"))
		})
	})

	// ── TOML front matter (+++ delimiters) ─────────────────────────────

	Context("TOML front matter (+++ delimiters)", func() {
		It("extracts title from TOML front matter", func() {
			input := []byte("+++\ntitle = \"TOML Title\"\n+++\nBody here")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("title", "TOML Title"))
		})

		It("returns remaining body after TOML front matter", func() {
			input := []byte("+++\ntitle = \"TOML Title\"\n+++\nBody here")
			_, body, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("Body here"))
		})
	})

	// ── JSON front matter ({ delimiter) ────────────────────────────────

	Context("JSON front matter ({ delimiter)", func() {
		It("extracts title from JSON front matter", func() {
			input := []byte("{\n  \"title\": \"JSON Title\"\n}\nBody here")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("title", "JSON Title"))
		})

		It("returns remaining body after JSON front matter", func() {
			input := []byte("{\n  \"title\": \"JSON Title\"\n}\nBody here")
			_, body, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(body)).To(Equal("Body here"))
		})
	})

	// ── Edge cases ─────────────────────────────────────────────────────

	Context("Edge cases", func() {
		It("returns build error when content file has no front matter delimiters", func() {
			input := []byte("# Just a heading\nSome content.")
			_, _, err := content.ParseFrontMatter(input)
			Expect(err).To(HaveOccurred(),
				"content files without front matter delimiters must be a build error (§1: front matter is required on content files only)")
			Expect(err.Error()).To(SatisfyAny(
				ContainSubstring("front matter"),
				ContainSubstring("delimiters"),
			), "error must describe the missing front matter")
		})

		It("error message suggests adding empty front matter when delimiters are missing", func() {
			input := []byte("# No front matter here\nJust content.")
			_, _, err := content.ParseFrontMatter(input)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("---"),
				"error must suggest adding empty front matter delimiters")
		})

		It("accepts empty front matter with all fields defaulting to nil/zero", func() {
			input := []byte("---\n---\nBody content here")
			fm, body, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred(),
				"empty front matter (---\\n---) must be valid per spec")
			Expect(fm).To(BeEmpty(),
				"empty front matter must produce empty map")
			Expect(string(body)).To(Equal("Body content here"),
				"body must follow the empty front matter block")
		})

		It("handles front matter with no body content", func() {
			input := []byte("---\ntitle: \"No Body\"\n---\n")
			fm, body, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("title", "No Body"))
			Expect(string(body)).To(BeEmpty())
		})

		It("returns error for malformed YAML in front matter", func() {
			input := []byte("---\ntitle: [invalid yaml\n---\nBody")
			_, _, err := content.ParseFrontMatter(input)
			Expect(err).To(HaveOccurred())
			// The error must describe the parse failure, not be a generic stub error
			Expect(err.Error()).To(ContainSubstring("yaml"), "error should indicate YAML parse failure")
		})
	})

	// ── Specific front matter fields ───────────────────────────────────

	Context("Specific front matter fields", func() {
		It("parses permalink field", func() {
			input := []byte("---\npermalink: /custom/path/\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("permalink", "/custom/path/"))
		})

		It("parses permalink: false as boolean false", func() {
			input := []byte("---\npermalink: false\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("permalink", false))
		})

		It("parses aliases as string array", func() {
			input := []byte("---\naliases:\n  - /old-url/\n  - /another-old/\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm["aliases"]).To(ConsistOf("/old-url/", "/another-old/"))
		})

		It("parses draft as boolean", func() {
			input := []byte("---\ndraft: true\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("draft", true))
		})

		It("parses date as time value", func() {
			input := []byte("---\ndate: 2025-06-15T10:30:00Z\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm["date"]).NotTo(BeNil())
		})

		It("parses summary as string", func() {
			input := []byte("---\nsummary: \"A brief description of the post.\"\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("summary", "A brief description of the post."))
		})

		It("parses pagination sub-object with data, perPage, as", func() {
			input := []byte("---\npagination:\n  data: posts\n  perPage: 10\n  as: items\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())

			pagination, ok := fm["pagination"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "pagination should be a map")
			Expect(pagination["data"]).To(Equal("posts"))
			Expect(pagination["perPage"]).To(BeNumerically("==", 10))
			Expect(pagination["as"]).To(Equal("items"))
		})

		It("parses outputs as string array (§1e multiple output formats)", func() {
			input := []byte("---\ntitle: Multi-Output\noutputs:\n  - html\n  - json\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm["outputs"]).To(ConsistOf("html", "json"),
				"outputs field must be parsed as a string array")
		})

		It("parses sitemap as object with per-page overrides (§1d)", func() {
			input := []byte("---\ntitle: Custom Sitemap\nsitemap:\n  priority: 0.8\n  changefreq: daily\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())

			sitemap, ok := fm["sitemap"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "sitemap should be a map when specifying overrides")
			Expect(sitemap["priority"]).To(BeNumerically("==", 0.8))
			Expect(sitemap["changefreq"]).To(Equal("daily"))
		})

		It("parses sitemap: false to exclude page from sitemap (§1d)", func() {
			input := []byte("---\ntitle: No Sitemap\nsitemap: false\n---\nBody")
			fm, _, err := content.ParseFrontMatter(input)
			Expect(err).NotTo(HaveOccurred())
			Expect(fm).To(HaveKeyWithValue("sitemap", false),
				"sitemap: false must be parsed as boolean false")
		})
	})

	// ── ToTemplateMap (issue #66, #67) ────────────────────────────────

	Describe("ToTemplateMap", func() {
		It("struct fields overlay FrontMatter keys of the same name", func() {
			p := &content.Page{
				URL:         "/computed/",
				FrontMatter: map[string]interface{}{"url": "/old/", "title": "My Post"},
			}
			m := p.ToTemplateMap()
			Expect(m["url"]).To(Equal("/computed/"),
				"struct URL must take precedence over FrontMatter url")
			Expect(m["title"]).To(Equal("My Post"),
				"FrontMatter-only keys must be preserved")
		})

		It("includes all FrontMatter keys in output map", func() {
			p := &content.Page{
				FrontMatter: map[string]interface{}{
					"title": "Test",
					"tags":  []string{"go", "ssg"},
					"draft": true,
				},
			}
			m := p.ToTemplateMap()
			Expect(m["title"]).To(Equal("Test"))
			Expect(m["tags"]).To(ConsistOf("go", "ssg"))
			Expect(m["draft"]).To(BeTrue())
		})

		It("does not set zero-value struct fields over FrontMatter", func() {
			p := &content.Page{
				URL:         "", // zero value
				FrontMatter: map[string]interface{}{"url": "/from-frontmatter/"},
			}
			m := p.ToTemplateMap()
			Expect(m["url"]).To(Equal("/from-frontmatter/"),
				"empty struct URL must not override FrontMatter url")
		})

		It("maps Section to collection key", func() {
			p := &content.Page{
				Section:     "blog",
				FrontMatter: map[string]interface{}{"title": "Post"},
			}
			m := p.ToTemplateMap()
			Expect(m["collection"]).To(Equal("blog"),
				"Section must be exposed as 'collection' for template access")
		})

		It("includes date and summary when set", func() {
			now := time.Now()
			p := &content.Page{
				Date:        now,
				Summary:     "A brief summary",
				FrontMatter: map[string]interface{}{"title": "Post"},
			}
			m := p.ToTemplateMap()
			Expect(m["date"]).To(Equal(now))
			Expect(m["summary"]).To(Equal("A brief summary"))
		})
	})

	// ── HTML() cached string accessor (issue #360) ──────────────────
	// Page.HTML() returns a cached string conversion of RenderedBody.
	// The conversion happens once on first access; subsequent calls
	// return the cached value. This eliminates redundant string([]byte)
	// allocations across plugin hooks, SSR, template context, and
	// result map construction.

	Describe("HTML() cached string accessor (issue #360)", func() {
		It("returns empty string when RenderedBody is nil", func() {
			p := &content.Page{}
			Expect(p.HTML()).To(BeEmpty(),
				"HTML() must return empty string when RenderedBody is nil")
		})

		It("returns empty string when RenderedBody is empty slice", func() {
			p := &content.Page{RenderedBody: []byte{}}
			Expect(p.HTML()).To(BeEmpty(),
				"HTML() must return empty string when RenderedBody is an empty byte slice")
		})

		It("returns the string value of RenderedBody", func() {
			p := &content.Page{RenderedBody: []byte("<h1>Hello</h1>")}
			Expect(p.HTML()).To(Equal("<h1>Hello</h1>"),
				"HTML() must return the string conversion of RenderedBody")
		})

		It("returns consistent value across multiple calls (cached)", func() {
			p := &content.Page{RenderedBody: []byte("<p>Cached</p>")}
			first := p.HTML()
			second := p.HTML()
			third := p.HTML()
			Expect(first).To(Equal("<p>Cached</p>"))
			Expect(second).To(Equal(first),
				"second call must return the same value as first — cache must be stable")
			Expect(third).To(Equal(first),
				"third call must return the same value as first — cache must be stable")
		})

		It("reflects the final RenderedBody value set before first HTML() call", func() {
			p := &content.Page{RenderedBody: []byte("<p>Draft</p>")}
			p.RenderedBody = []byte("<p>Final</p>")
			Expect(p.HTML()).To(Equal("<p>Final</p>"),
				"HTML() must reflect the last RenderedBody assignment — "+
					"the cache is populated on first access, after all mutations")
		})
	})

	// ── Error format contracts ────────────────────────────────────────

	Describe("Error format contracts", func() {
		It("includes file path in BuildPage error for unparseable content", func() {
			raw := []byte("---\ntitle: [broken yaml\n---\nBody")
			_, err := content.BuildPage("content/blog/broken.md", raw)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("content/blog/broken.md"),
				"error must include the source file path")
		})
	})

	// ── Summaries ─────────────────────────────────────────────────────

	Describe("Summaries", func() {
		It("BuildPage populates Summary from front matter summary field", func() {
			raw := []byte("---\ntitle: Summarized\nsummary: \"This is a summary.\"\n---\nBody content")
			page, err := content.BuildPage("blog/post.md", raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(page).NotTo(BeNil())
			Expect(page.Summary).To(Equal("This is a summary."))
		})

		It("BuildPage leaves Summary empty when not in front matter", func() {
			raw := []byte("---\ntitle: No Summary\n---\nBody content")
			page, err := content.BuildPage("blog/post.md", raw)
			Expect(err).NotTo(HaveOccurred())
			Expect(page).NotTo(BeNil())
			Expect(page.Summary).To(BeEmpty())
		})
	})
})
