package pipeline_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

var _ = Describe("Build Pipeline", func() {
	Describe("Pagination as variable in body", func() {
		It("pagination as variable is accessible in rendered content", func() {
			cfg := &config.Config{
				Title:   "Pagination As Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/categories.json": `[{"slug":"color","title":"Color"},{"slug":"space","title":"Space"}]`,
				"content/tokens.md": "---\ntitle: Tokens\nlayout: default\npagination:\n  data: site.data.categories\n  perPage: 1\n  as: category\npermalink: \"/tokens/{{ category.slug }}/\"\n---\n## {{ category.title }}\n\nSlug: {{ category.slug }}",
				"layouts/default.liquid": "<html><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			// Find a virtual page that rendered the category data
			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "Color") {
					found = true
					Expect(html).To(ContainSubstring("Color"),
						"category.title must resolve in the template body")
					Expect(html).To(ContainSubstring("Slug: color"),
						"category.slug must also resolve in the body")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with the pagination as variable — "+
					"if this fails, the as variable resolves in permalink but not body")
		})
	})

	// ── Pagination front matter interpolation (issue #378) ──────────
	// String-valued front matter fields with template tags must be
	// interpolated using the pagination as: variable for virtual pages.

	Describe("Pagination front matter interpolation", func() {
		It("title is interpolated from pagination as variable", func() {
			cfg := &config.Config{
				Title:   "FM Interpolation Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"}]`,
				"content/team.md": "---\ntitle: \"{{ member.name }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>Alice</title>"),
						"page.title must be interpolated from {{ member.name }} — "+
							"front matter template tags must resolve using the pagination as: variable")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with interpolated title")
		})

		It("front matter interpolation supports filters", func() {
			cfg := &config.Config{
				Title:   "FM Filter Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"alice","slug":"alice"}]`,
				"content/team.md": "---\ntitle: \"{{ member.name | upcase }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>ALICE</title>"),
						"front matter interpolation must support Liquid filters — "+
							"{{ member.name | upcase }} should produce ALICE")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with filter-processed title")
		})

		It("multiple front matter fields are interpolated", func() {
			cfg := &config.Config{
				Title:   "FM Multi Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice","role":"Engineer"}]`,
				"content/team.md": "---\ntitle: \"{{ member.name }}\"\ndescription: \"{{ member.role }} at Acme\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title><meta name=\"description\" content=\"{{ page.description }}\"></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>Alice</title>"),
						"page.title must be interpolated")
					Expect(html).To(ContainSubstring("Engineer at Acme"),
						"page.description must also be interpolated — "+
							"all string front matter fields with template tags should resolve")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render with multiple interpolated fields")
		})

		It("non-template front matter fields are unchanged", func() {
			cfg := &config.Config{
				Title:   "FM Passthrough Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice"}]`,
				"content/team.md": "---\ntitle: \"Static Title\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ member.slug }}/\"\n---\n<p>{{ member.name }}</p>",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					Expect(html).To(ContainSubstring("<title>Static Title</title>"),
						"front matter without template tags must pass through unchanged")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one virtual page must render")
		})

		It("paginated list pages do not interpolate front matter", func() {
			cfg := &config.Config{
				Title:   "FM List Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/team.json": `[{"name":"Alice","slug":"alice"},{"name":"Bob","slug":"bob"}]`,
				"content/team.md": "---\ntitle: \"Team Members\"\nheading: \"{{ member.name }}\"\nlayout: default\npagination:\n  data: site.data.team\n  perPage: 10\n  as: member\npermalink: \"/team/\"\n---\n{% for m in member %}<p>{{ m.name }}</p>{% endfor %}",
				"layouts/default.liquid": "<html><head><title>{{ page.title }}</title></head><body><h1>{{ page.heading }}</h1>{{ content }}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			found := false
			for _, html := range result.RenderedContent {
				if strings.Contains(html, "<p>Alice</p>") {
					found = true
					// page.heading should NOT have been interpolated to a member name
					// because perPage > 1 means member is a slice, not a single item
					Expect(html).NotTo(ContainSubstring("<h1>Alice</h1>"),
						"paginated list pages (perPage > 1) must NOT interpolate front matter — "+
							"the as: variable is a slice, not a single item")
					Expect(html).NotTo(ContainSubstring("<h1>Bob</h1>"),
						"paginated list pages must not interpolate to any individual item")
					break
				}
			}
			Expect(found).To(BeTrue(),
				"at least one paginated list page must render")
		})
	})

	// ── JSON data key order in templates (issue #456) ──────────────
	// Liquid templates iterating JSON data files must see keys in
	// the document's insertion order, not Go's random map order.

	Describe("JSON data key order in templates (issue #456)", func() {
		It("{% for %} over JSON data preserves insertion order", func() {
			cfg := &config.Config{
				Title:   "JSON Order Test",
				BaseURL: "https://example.com",
				Build:   config.BuildConfig{Output: "_site"},
			}
			contentMap := map[string]string{
				"data/colors.json": `{"white":"#fff","black":"#000","accent":"#e00","brand":"#ee0","surface":"#f0f"}`,
				"content/index.md": "---\ntitle: Colors\nlayout: default\n---\n# Colors",
				"layouts/default.liquid": "<html><body>{% for color in site.data.colors %}<span>{{ color[0] }}</span>{% endfor %}</body></html>",
			}
			result, err := pipeline.BuildWithContent(cfg, contentMap)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())

			html := result.RenderedContent["index.md"]
			Expect(html).NotTo(BeEmpty())

			// The spans must appear in JSON insertion order
			whiteIdx := strings.Index(html, "<span>white</span>")
			blackIdx := strings.Index(html, "<span>black</span>")
			accentIdx := strings.Index(html, "<span>accent</span>")
			brandIdx := strings.Index(html, "<span>brand</span>")
			surfaceIdx := strings.Index(html, "<span>surface</span>")

			Expect(whiteIdx).To(BeNumerically(">=", 0),
				"white must appear in output")
			Expect(blackIdx).To(BeNumerically(">", whiteIdx),
				"black must appear after white — JSON insertion order")
			Expect(accentIdx).To(BeNumerically(">", blackIdx),
				"accent must appear after black")
			Expect(brandIdx).To(BeNumerically(">", accentIdx),
				"brand must appear after accent")
			Expect(surfaceIdx).To(BeNumerically(">", brandIdx),
				"surface must appear after brand — "+
					"if order is wrong, JSON data was loaded into map[string]interface{} "+
					"instead of *ordered.Map (issue #453)")
		})
	})

	// ── Node plugin cwd with ProjectRoot (issue #439) ──────────────
	// When cfg.ProjectRoot is set (via -r flag), the Node bridge must
	// spawn its subprocess with cwd = ProjectRoot so node_modules/
	// imports resolve from the project directory.
})
