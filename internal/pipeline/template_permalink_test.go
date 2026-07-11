package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Template permalinks (issue #830) ─────────────────────────────────
// Front matter permalinks containing {{ }} are rendered through the
// configured template engine. Token syntax and template syntax are
// separate modes — when {{ is detected, tokens are not resolved.
// Pagination template permalinks respect the configured engine instead
// of always falling back to Liquid.

var _ = Describe("Template permalinks (issue #830)", func() {

	// ── Regular page template permalinks ───────────────────────────

	Describe("Regular page template permalinks", func() {
		It("renders template permalink through Liquid engine", func() {
			cfg := &config.Config{
				Title:   "Template Permalink Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/blog/hello.md":  "---\ntitle: Hello World\nslug: hello-world\nlayout: default\npermalink: \"/posts/{{ page.slug }}/\"\n---\n# Hello",
				"layouts/default.liquid": "<html><body><span class=\"url\">{{ page.url }}</span>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed when a regular page uses a Liquid template permalink")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["blog/hello.md"]
			Expect(html).NotTo(BeEmpty(),
				"page with template permalink must render")
			Expect(html).To(ContainSubstring("<span class=\"url\">/posts/hello-world/</span>"),
				"template permalink must resolve {{ page.slug }} to the actual slug value — "+
					"if the output contains literal {{ page.slug }}, the permalink was not rendered "+
					"through the template engine")
		})

		It("renders template permalink through Go template engine", func() {
			cfg := &config.Config{
				Title:     "GoTemplate Permalink Test",
				BaseURL:   "https://example.com",
				Build:     config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"content/blog/hello.md": "---\ntitle: Hello World\nslug: hello-world\nlayout: default\npermalink: \"/posts/{{ .page.slug }}/\"\n---\n# Hello",
				"layouts/default.html":  "<html><body><span class=\"url\">{{ .page.url }}</span>{{ .content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed when a regular page uses a Go template permalink — "+
					"the configured engine must be used for permalink rendering, not Liquid")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["blog/hello.md"]
			Expect(html).NotTo(BeEmpty(),
				"page with Go template permalink must render")
			Expect(html).To(ContainSubstring("<span class=\"url\">/posts/hello-world/</span>"),
				"Go template permalink must resolve {{ .page.slug }} to the actual slug value — "+
					"if this fails, permalink rendering fell back to Liquid and failed to parse Go syntax")
		})

		It("template permalink accesses custom front matter fields", func() {
			cfg := &config.Config{
				Title:   "Custom Field Permalink Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/post.md":        "---\ntitle: My Post\nslug: my-post\nlang: en\nlayout: default\npermalink: \"/{{ page.lang }}/{{ page.slug }}/\"\n---\n# Content",
				"layouts/default.liquid": "<html><body><span class=\"url\">{{ page.url }}</span>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed — custom front matter fields (lang) must be "+
					"accessible in template permalink expressions via page.*")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["post.md"]
			Expect(html).NotTo(BeEmpty(),
				"page with custom front matter in template permalink must render")
			Expect(html).To(ContainSubstring("<span class=\"url\">/en/my-post/</span>"),
				"template permalink must resolve custom front matter fields — "+
					"{{ page.lang }} must produce 'en' and {{ page.slug }} must produce 'my-post'")
		})

		It("template permalink supports Liquid filters", func() {
			cfg := &config.Config{
				Title:   "Filter Permalink Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/post.md":        "---\ntitle: My First Post\nlayout: default\npermalink: \"/{{ page.title | slugify }}/\"\n---\n# Content",
				"layouts/default.liquid": "<html><body><span class=\"url\">{{ page.url }}</span>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed — Liquid filters like slugify must work in template permalinks")
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["post.md"]
			Expect(html).NotTo(BeEmpty(),
				"page with filter in template permalink must render")
			Expect(html).To(ContainSubstring("<span class=\"url\">/my-first-post/</span>"),
				"slugify filter must convert 'My First Post' to 'my-first-post' in the permalink — "+
					"if the output contains the unfiltered title, filters are not available during permalink rendering")
		})

		It("template permalink rendering to empty string is a build error", func() {
			cfg := &config.Config{
				Title:   "Empty Permalink Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/post.md":        "---\ntitle: My Post\nlayout: default\npermalink: \"{{ page.nonexistent_field }}\"\n---\n# Content",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			_, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).To(HaveOccurred(),
				"template permalink that renders to empty string must produce a build error — "+
					"this is distinct from permalink:false which is an intentional opt-out; "+
					"an empty render indicates a bug in the user's template expression")
		})

		It("template and token syntax are separate modes", func() {
			cfg := &config.Config{
				Title:   "Mixed Syntax Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/post.md":        "---\ntitle: My Post\nslug: my-post\nlayout: default\npermalink: \"/{{ page.slug }}/:year/\"\n---\n# Content",
				"layouts/default.liquid": "<html><body><span class=\"url\">{{ page.url }}</span>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["post.md"]
			Expect(html).NotTo(BeEmpty(),
				"page with mixed {{ and :token syntax must render")
			Expect(html).To(ContainSubstring(":year"),
				"when {{ is detected, token syntax like :year must remain literal — "+
					"template and token modes are mutually exclusive; "+
					"if :year was resolved to a year value, the modes were incorrectly mixed")
			Expect(html).To(ContainSubstring("/my-post/:year/"),
				"the resolved URL must be /my-post/:year/ — "+
					"{{ page.slug }} renders to 'my-post' via the template engine, "+
					":year stays literal because token resolution is skipped in template mode")
		})

		It("static permalink without {{ works unchanged (regression)", func() {
			cfg := &config.Config{
				Title:   "Static Permalink Regression Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"content/about.md":       "---\ntitle: About\nlayout: default\npermalink: \"/about/\"\n---\n# About Us",
				"layouts/default.liquid": "<html><body><span class=\"url\">{{ page.url }}</span>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["about.md"]
			Expect(html).NotTo(BeEmpty(),
				"static permalink (no {{ }}) must continue to work unchanged")
			Expect(html).To(ContainSubstring("<span class=\"url\">/about/</span>"),
				"static permalink must resolve to the literal string — "+
					"template permalink feature must not break existing static permalinks")
		})
	})

	// ── Engine-aware pagination template permalinks ────────────────

	Describe("Engine-aware pagination template permalinks", func() {
		It("pagination template permalink renders through Go template engine", func() {
			cfg := &config.Config{
				Title:     "GoTemplate Pagination Permalink Test",
				BaseURL:   "https://example.com",
				Build:     config.BuildConfig{Output: "_site"},
				Templates: config.TemplatesConfig{Engine: "gotemplate"},
			}
			contentMap := map[string]string{
				"data/team.json":       `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"}]`,
				"content/team.md":      "---\ntitle: Team\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ .member.slug }}/\"\n---\n<p>{{ .member.name }}</p>",
				"layouts/default.html": "<html><body>{{ .content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must succeed — pagination template permalink must render through "+
					"the configured Go template engine, not fall back to Liquid")
			Expect(result).NotTo(BeNil())

			// Pagination virtual pages are keyed by URL in RenderedContent
			aliceHTML := result.RenderedContent["/team/alice/"]
			Expect(aliceHTML).NotTo(BeEmpty(),
				"virtual page for Alice must render at /team/alice/ — "+
					"if empty, Go template engine was not used for pagination permalink rendering")
			Expect(aliceHTML).To(ContainSubstring("Alice"),
				"Alice's page must contain her name in the rendered body")

			bobHTML := result.RenderedContent["/team/bob/"]
			Expect(bobHTML).NotTo(BeEmpty(),
				"virtual page for Bob must render at /team/bob/")
			Expect(bobHTML).To(ContainSubstring("Bob"),
				"Bob's page must contain his name in the rendered body")
		})

		It("pagination template permalink continues to work with Liquid (regression)", func() {
			cfg := &config.Config{
				Title:   "Liquid Pagination Permalink Regression Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json":         `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"}]`,
				"content/team.md":        "---\ntitle: Team\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"existing Liquid pagination template permalinks must continue to work")
			Expect(result).NotTo(BeNil())

			aliceHTML := result.RenderedContent["/team/alice/"]
			Expect(aliceHTML).NotTo(BeEmpty(),
				"virtual page for Alice must render at /team/alice/")
			Expect(aliceHTML).To(ContainSubstring("Alice"),
				"Alice's page must contain her name — regression guard for existing behavior")

			bobHTML := result.RenderedContent["/team/bob/"]
			Expect(bobHTML).NotTo(BeEmpty(),
				"virtual page for Bob must render at /team/bob/")
		})
	})
})
