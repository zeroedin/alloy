package pipeline_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Build Pipeline", func() {
	Describe("Taxonomy collection page properties", func() {
		It("taxonomy collection items expose title and url in templates", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Props Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/about.md":   "---\ntitle: About\ntags: [\"about\"]\n---\n# About",
				"content/roadmap.md": "---\ntitle: Roadmap\ntags: [\"about\"]\n---\n# Roadmap",
				"layouts/default.liquid": `<html><body>{{ content }}{% for p in taxonomies.tags.about %}<span class="item">{{ p.title }}|{{ p.url }}</span>{% endfor %}</body></html>`,
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Collect all rendered HTML into a single string so assertions
			// fire unconditionally — no if-guard that could silently pass.
			var allHTML string
			for _, html := range result.RenderedContent {
				allHTML += html
			}
			Expect(allHTML).To(ContainSubstring(`class="item"`),
				"at least one page must render the taxonomy collection loop — "+
					"if this fails, the layout didn't render or the collection is empty")
			Expect(allHTML).To(ContainSubstring("About|"),
				"taxonomy collection items must expose title via ToTemplateMap()")
			Expect(allHTML).To(ContainSubstring("Roadmap|"),
				"all pages tagged 'about' must appear with their title")
			Expect(allHTML).To(MatchRegexp(`About\|/about`),
				"taxonomy collection items must expose url via ToTemplateMap()")
		})
	})

	// ── Taxonomy template access (issue #380) ───────────────────────
	// taxonomies.* must be populated and accessible in both layouts and
	// content templates. The user reported taxonomies appearing empty
	// when content is in subdirectories (e.g., content/blog/post-a.md).

	Describe("Taxonomy template access (issue #380)", func() {
		It("taxonomies.tags is accessible in content templates", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Access Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\", \"web\"]\nlayout: default\n---\n# Post A",
				"content/post-b.md": "---\ntitle: Post B\ntags: [\"go\"]\nlayout: default\n---\n# Post B",
				"content/index.md":  "---\ntitle: Index\nlayout: default\n---\n{% for post in taxonomies.tags.go %}<span class=\"tagged\">{{ post.title }}</span>{% endfor %}",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty(),
				"index page must render")
			Expect(html).To(ContainSubstring(`class="tagged"`),
				"taxonomies.tags.go must be iterable in content templates — "+
					"if missing, taxonomies are not injected into the content render context")
			Expect(html).To(ContainSubstring("Post A"),
				"Post A is tagged 'go' and must appear in taxonomies.tags.go")
			Expect(html).To(ContainSubstring("Post B"),
				"Post B is tagged 'go' and must appear in taxonomies.tags.go")
		})

		It("taxonomies are accessible in layouts, not just content", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Layout Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\"]\nlayout: default\n---\n# Post A",
				"content/post-b.md": "---\ntitle: Post B\ntags: [\"go\"]\nlayout: default\n---\n# Post B",
				"layouts/default.liquid": "<html><body>{{ content }}<nav>{% for post in taxonomies.tags.go %}<a href=\"{{ post.url }}\">{{ post.title }}</a>{% endfor %}</nav></body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			var allHTML string
			for _, html := range result.RenderedContent {
				allHTML += html
			}
			Expect(allHTML).To(ContainSubstring("<nav>"),
				"layout nav must render")
			Expect(allHTML).To(ContainSubstring("Post A"),
				"Post A tagged 'go' must appear in layout taxonomy loop")
		})

		It("taxonomies work when content is in subdirectories", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Taxonomy Subdir Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags":       {Render: &renderFalse},
					"categories": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/blog/_data.yaml":  "layout: post",
				"content/blog/post-a.md":   "---\ntitle: Post A\ndate: 2026-04-01\ntags: [\"go\", \"web\"]\ncategories: [\"tutorials\"]\nlayout: default\n---\n# Post A",
				"content/blog/post-b.md":   "---\ntitle: Post B\ndate: 2026-04-02\ntags: [\"go\", \"testing\"]\ncategories: [\"tutorials\"]\nlayout: default\n---\n# Post B",
				"content/blog/post-c.md":   "---\ntitle: Post C\ndate: 2026-04-03\ntags: [\"css\", \"web\"]\ncategories: [\"design\"]\nlayout: default\n---\n# Post C",
				"content/series-test.md":   "---\ntitle: Series Test\nlayout: default\n---\n{% for post in taxonomies.tags.go %}<span class=\"go-tag\">{{ post.title }}</span>{% endfor %}",
				"layouts/default.liquid":   "<html><body>{{ content }}</body></html>",
				"layouts/post.liquid":      "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["series-test.md"]
			Expect(html).NotTo(BeEmpty(),
				"series-test page must render")
			Expect(html).To(ContainSubstring(`class="go-tag"`),
				"taxonomies.tags.go must be iterable when tagged content is in subdirectories — "+
					"if empty, taxonomy building may not collect pages from nested content dirs")
			Expect(html).To(ContainSubstring("Post A"),
				"Post A is tagged 'go' and must appear")
			Expect(html).To(ContainSubstring("Post B"),
				"Post B is tagged 'go' and must appear")
			Expect(html).NotTo(ContainSubstring("Post C"),
				"Post C is NOT tagged 'go' — must not appear")
		})

		It("taxonomy with no matching pages produces empty collection", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Empty Taxonomy Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags":       {Render: &renderFalse},
					"categories": {Render: &renderFalse},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\"]\nlayout: default\n---\n# Post A",
				"content/index.md":  "---\ntitle: Index\nlayout: default\n---\n{% for post in taxonomies.categories.news %}{{ post.title }}{% endfor %}\n\nDONE_MARKER",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty(),
				"index page must render")
			Expect(html).To(ContainSubstring("DONE_MARKER"),
				"index page must render even when taxonomy term has no pages")
			Expect(html).NotTo(ContainSubstring("Post A"),
				"Post A is not in categories.news — must not appear")
		})
	})

	// ── Taxonomy layout with front matter (issue #418) ──────────────
	// Taxonomy layouts may contain YAML front matter (e.g., layout: base
	// for chaining). The pipeline must strip front matter before parsing
	// the taxonomy layout — otherwise the --- delimiters render as text.

	Describe("Taxonomy layout with front matter (issue #418)", func() {
		It("taxonomy layout front matter is stripped before rendering", func() {
			renderTrue := true
			cfg := &config.Config{
				Title:   "Taxonomy Layout FM Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderTrue, Layout: "tags"},
				},
			}
			contentMap := map[string]string{
				"content/post-a.md": "---\ntitle: Post A\ntags: [\"go\"]\nlayout: default\n---\n# Post A",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"layouts/tags.liquid": "---\nlayout: base\n---\n<div class=\"taxonomy\">{{ taxonomy.term }}</div>",
				"layouts/base.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred(),
				"build must not error when taxonomy layout has front matter — "+
					"if this fails, the layout parser is not stripping front matter")
			Expect(result).NotTo(BeNil())

			// Taxonomy pages are generated (no RelPath), so their
			// RenderedContent key is page.URL (e.g., "/tags/", "/tags/go/").
			found := false
			for key, html := range result.RenderedContent {
				if strings.Contains(key, "/tags/") || key == "/tags/" {
					found = true
					Expect(html).NotTo(ContainSubstring("---"),
						"taxonomy layout front matter delimiters must be stripped — "+
							"if '---' appears in output, StripLayoutFrontMatter was not called")
					Expect(html).NotTo(ContainSubstring("layout: base"),
						"taxonomy layout front matter content must not appear in rendered output")
					Expect(html).To(ContainSubstring("taxonomy"),
						"taxonomy layout must render its content")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one taxonomy page must appear in RenderedContent with a /tags/ URL key — "+
					"taxonomy pages have no RelPath, so renderedContentKey must use page.URL")
		})
	})

	// ── page.toc pipeline wiring (issue #274) ───────────────────────
	// page.toc must be populated during Build and accessible in templates.
	Describe("Taxonomy page URLs in conflict detection (issue #695)", func() {
		It("authored page conflicting with taxonomy index URL errors before rendering (issue #695)", func() {
			cfg := &config.Config{
				Title:   "Tax Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":          "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":   "<html><body>taxonomy index</body></html>",
				"content/page-a.md":               "---\ntitle: Page A\nlayout: default\ntags: [\"go\"]\n---\n# Page A",
				"content/tags.md":                 "---\ntitle: Tags Override\nlayout: default\npermalink: /tags/\n---\n# My Tags",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"an authored page with permalink /tags/ must conflict with the "+
					"auto-generated taxonomy index page at /tags/ — taxonomy page "+
					"URLs must be included in pre-render conflict detection (issue #695)")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil(),
				"BuildResult must be nil when a taxonomy conflict is detected — "+
					"validation catches this before any rendering occurs (issue #695)")
		})

		It("authored page conflicting with taxonomy term URL errors before rendering (issue #695)", func() {
			cfg := &config.Config{
				Title:   "Term Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":          "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":   "<html><body>taxonomy term</body></html>",
				"content/page-a.md":               "---\ntitle: Page A\nlayout: default\ntags: [\"golang\"]\n---\n# Page A",
				"content/golang-guide.md":         "---\ntitle: Golang Guide\nlayout: default\npermalink: /tags/golang/\n---\n# Golang Guide",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"an authored page with permalink /tags/golang/ must conflict with "+
					"the auto-generated taxonomy term page — term URLs are "+
					"deterministic from the taxonomy config permalink pattern and "+
					"discovered terms (issue #695)")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil(),
				"BuildResult must be nil when a taxonomy term conflict is detected "+
					"(issue #695)")
		})

		It("non-conflicting taxonomy pages build successfully (issue #695)", func() {
			cfg := &config.Config{
				Title:   "Tax No Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":          "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":   "<html><body>{{ content }}</body></html>",
				"content/page-a.md":               "---\ntitle: Page A\nlayout: default\ntags: [\"go\", \"web\"]\n---\n# Page A",
				"content/page-b.md":               "---\ntitle: Page B\nlayout: default\ntags: [\"go\"]\npermalink: /guides/\n---\n# Page B",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"pages with distinct permalinks that don't collide with taxonomy "+
					"URLs must build successfully — taxonomy conflict detection "+
					"must not produce false positives (issue #695)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(BeNumerically(">=", 2),
				"authored pages must be counted in the result")
		})

		It("render: false taxonomy does not reserve paths in conflict detection (issue #695)", func() {
			renderFalse := false
			cfg := &config.Config{
				Title:   "Render False Tax Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Render: &renderFalse},
				},
			}
			files := map[string]string{
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
				"content/page-a.md":      "---\ntitle: Page A\nlayout: default\ntags: [\"go\"]\n---\n# Page A",
				"content/tags.md":        "---\ntitle: My Tags\nlayout: default\npermalink: /tags/\n---\n# Tags",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"a taxonomy with render: false does not generate output pages — "+
					"its index/term URLs must not be included in conflict detection, "+
					"so an authored /tags/ page must build successfully (issue #695)")
			Expect(result).NotTo(BeNil())
		})

		It("custom taxonomy permalink pattern conflicts with authored page (issue #695)", func() {
			cfg := &config.Config{
				Title:   "Custom Pattern Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {Permalink: "/topics/:slug/"},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":          "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":   "<html><body>taxonomy</body></html>",
				"content/page-a.md":               "---\ntitle: Page A\nlayout: default\ntags: [\"golang\"]\n---\n# Page A",
				"content/topics-golang.md":        "---\ntitle: Topics Golang\nlayout: default\npermalink: /topics/golang/\n---\n# Topics",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"a custom taxonomy permalink pattern /topics/:slug/ must generate "+
					"term page at /topics/golang/ — an authored page at that path "+
					"must be detected as a conflict (issue #695)")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil())
		})

		It("alias colliding with taxonomy index URL is detected (issue #695)", func() {
			cfg := &config.Config{
				Title:   "Alias Tax Conflict Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":          "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":   "<html><body>taxonomy</body></html>",
				"content/page-a.md":               "---\ntitle: Page A\nlayout: default\ntags: [\"go\"]\n---\n# Page A",
				"content/about.md":                "---\ntitle: About\nlayout: default\npermalink: /about/\naliases:\n  - /tags/\n---\n# About",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"an alias pointing to /tags/ must conflict with the auto-generated "+
					"taxonomy index page — aliases are part of the output manifest "+
					"and must be checked against taxonomy URLs (issue #695)")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil())
		})
	})

	// ── Multi-language taxonomy URL prefix conflicts (issue #705) ──
	// Non-root languages get URL-prefixed taxonomy pages (e.g., /fr/tags/).
	// Conflict detection must include these prefixed URLs. The urlPrefix != ""
	// branch in the taxonomy conflict loop must be exercised.

	Describe("Multi-language taxonomy URL prefix conflicts (issue #705)", func() {
		It("non-root language taxonomy index URL conflicts with authored page (issue #705)", func() {
			cfg := &config.Config{
				Title:   "Multi-Lang Tax Index Conflict",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Languages: map[string]*config.LanguageConfig{
					"en": {Weight: 1, Root: true},
					"fr": {Weight: 2},
				},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":         "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":  "<html><body>taxonomy</body></html>",
				"content/en/page-a.md":           "---\ntitle: Page A\nlayout: default\ntags: [\"go\"]\n---\n# Page A",
				"content/fr/page-a.md":           "---\ntitle: Page A FR\nlayout: default\ntags: [\"go\"]\n---\n# Page A FR",
				"content/fr/tags-override.md":    "---\ntitle: Tags Override FR\nlayout: default\npermalink: /tags/\n---\n# Tags FR",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"an authored page with permalink /tags/ in the fr batch gets "+
					"prefixed to /fr/tags/ by the pipeline — this must conflict "+
					"with the auto-generated taxonomy index at /fr/tags/ (issue #705)")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil(),
				"BuildResult must be nil when a prefixed taxonomy conflict is detected")
		})

		It("non-root language taxonomy term URL conflicts with authored page (issue #705)", func() {
			cfg := &config.Config{
				Title:   "Multi-Lang Tax Term Conflict",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Languages: map[string]*config.LanguageConfig{
					"en": {Weight: 1, Root: true},
					"fr": {Weight: 2},
				},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":         "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":  "<html><body>taxonomy</body></html>",
				"content/en/page-a.md":           "---\ntitle: Page A\nlayout: default\ntags: [\"golang\"]\n---\n# Page A",
				"content/fr/page-a.md":           "---\ntitle: Page A FR\nlayout: default\ntags: [\"golang\"]\n---\n# Page A FR",
				"content/fr/golang-guide.md":     "---\ntitle: Golang Guide FR\nlayout: default\npermalink: /tags/golang/\n---\n# Golang FR",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"an authored page with permalink /tags/golang/ in the fr batch gets "+
					"prefixed to /fr/tags/golang/ by the pipeline — this must conflict "+
					"with the auto-generated taxonomy term page at /fr/tags/golang/ "+
					"(issue #705)")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil(),
				"BuildResult must be nil when a prefixed taxonomy term conflict is detected")
		})

		It("root language taxonomy URLs remain unprefixed in multi-lang build (issue #705)", func() {
			cfg := &config.Config{
				Title:   "Multi-Lang Root Tax Conflict",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Languages: map[string]*config.LanguageConfig{
					"en": {Weight: 1, Root: true},
					"fr": {Weight: 2},
				},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":         "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":  "<html><body>taxonomy</body></html>",
				"content/en/page-a.md":           "---\ntitle: Page A\nlayout: default\ntags: [\"go\"]\n---\n# Page A",
				"content/fr/page-a.md":           "---\ntitle: Page A FR\nlayout: default\ntags: [\"go\"]\n---\n# Page A FR",
				"content/en/tags-override.md":    "---\ntitle: Tags Override\nlayout: default\npermalink: /tags/\n---\n# Tags EN",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).To(HaveOccurred(),
				"root language taxonomy at /tags/ must conflict with authored page "+
					"at /tags/ — root language URLs are unprefixed, so the conflict "+
					"detection must match them without a language prefix (issue #705)")
			Expect(err.Error()).To(ContainSubstring("output path conflict"),
				"error message must identify the conflict type")
			Expect(result).To(BeNil())
		})

		It("taxonomy URLs across different languages do not conflict (issue #705)", func() {
			cfg := &config.Config{
				Title:   "Multi-Lang No Conflict",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
				Languages: map[string]*config.LanguageConfig{
					"en": {Weight: 1, Root: true},
					"fr": {Weight: 2},
				},
				Taxonomies: map[string]*config.TaxonomyConfig{
					"tags": {},
				},
			}
			files := map[string]string{
				"layouts/default.liquid":         "<html><body>{{ content }}</body></html>",
				"layouts/taxonomies/tags.liquid":  "<html><body>taxonomy</body></html>",
				"content/en/page-a.md":           "---\ntitle: Page A\nlayout: default\ntags: [\"go\", \"web\"]\n---\n# Page A",
				"content/fr/page-a.md":           "---\ntitle: Page A FR\nlayout: default\ntags: [\"go\", \"web\"]\n---\n# Page A FR",
			}
			result, err := pipeline.BuildWithContent(cfg, files)
			Expect(err).NotTo(HaveOccurred(),
				"taxonomy pages at /tags/ (en, root) and /fr/tags/ (fr, non-root) "+
					"occupy distinct output paths — multi-language taxonomy URLs "+
					"must not produce false-positive conflicts (issue #705)")
			Expect(result).NotTo(BeNil())
			Expect(result.PageCount).To(BeNumerically(">=", 2),
				"both language batches must produce pages")
		})
	})

	// ── Early validation: conflict detection before rendering (issue #690) ──
	// Validation (permalink/alias conflicts) must run after onPagesReady but
	// before content rendering. If a conflict is detected, the build fails
	// immediately with no rendering work performed.
})
