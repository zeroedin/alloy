package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Liquid sees content and summary as plain strings (guard) ─────────
// liquidgo tests for string directly in property lookups part-way through
// a path ({{ page.summary.size }}) and in filters that read properties
// (sum). A page's rendered content and summary must reach Liquid as plain
// strings, or these silently go empty. Moved from the Go trust tests
// (#1267, #1283) when Go templates were removed (#1285).

var _ = Describe("Liquid sees content and summary as plain strings (guard)", func() {

	It("in filters and comparisons", func() {
		result, err := pipeline.BuildWithContent(&config.Config{
			Title:   "Plain Liquid",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}, map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: default\nsummary: \"<b>sum</b>\"\n---\nBody with **bold**",
			"layouts/default.liquid": "SIZE[{{ page.summary | size }}] EQ[{% if page.summary == \"<b>sum</b>\" %}yes{% endif %}] " +
				"STRIP[{{ content | strip_html | strip }}] CONT[{% if content contains \"bold\" %}yes{% endif %}] " +
				"LIST[{% for p in site.pages %}{{ p.summary | size }}{% endfor %}] RAW[{{ page.summary }}] " +
				"WHERE[{{ site.pages | where: \"summary\", \"<b>sum</b>\" | size }}]",
		})
		Expect(err).NotTo(HaveOccurred())
		out := result.RenderedContent["index.md"]
		Expect(out).To(ContainSubstring("SIZE[10]"))
		Expect(out).To(ContainSubstring("EQ[yes]"))
		Expect(out).To(ContainSubstring("STRIP[Body with bold]"))
		Expect(out).To(ContainSubstring("CONT[yes]"))
		Expect(out).To(ContainSubstring("LIST[10]"))
		Expect(out).To(ContainSubstring("WHERE[1]"),
			"where compares a page's summary as text")
		Expect(out).To(ContainSubstring("RAW[<b>sum</b>]"))
	})

	It("in property lookups and sum", func() {
		result, err := pipeline.BuildWithContent(&config.Config{
			Title:   "Plain Liquid Lookups",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}, map[string]string{
			"content/blog/a.md": "---\ntitle: A\nlayout: default\nsummary: \"10\"\n---\nA",
			"content/blog/b.md": "---\ntitle: B\nlayout: default\nsummary: \"9\"\n---\nB",
			"content/blog/c.md": "---\ntitle: C\nlayout: default\nsummary: \"9\"\n---\nC",
			"layouts/default.liquid": "PS[{{ page.summary.size }}] PC[{{ page.content.size }}] " +
				"LOOP[{% for p in site.pages %}{{ p.summary.size }},{% endfor %}] SUM[{{ site.pages | sum: \"summary\" }}]",
		})
		Expect(err).NotTo(HaveOccurred())
		out := result.RenderedContent["blog/a.md"]
		Expect(out).To(ContainSubstring("PS[2]"), "page.summary.size")
		Expect(out).To(ContainSubstring("PC[9]"), "page.content.size: <p>A</p> plus newline")
		Expect(out).To(ContainSubstring("LOOP[2,1,1,]"), "p.summary.size in a loop")
		Expect(out).To(ContainSubstring("SUM[28]"), "sum: \"summary\": 10 + 9 + 9")
	})
})
