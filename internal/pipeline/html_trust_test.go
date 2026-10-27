package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Go templates trust HTML by origin, not by key name (issue #1267) ──
// The Go engine rendered any string under a key named `content` or
// `summary`, at any depth, as raw HTML. A data file with a `content`
// field could inject script while its sibling fields were escaped.
//
// The rule: Go templates print raw only the HTML Alloy produced or
// vouches for — the page's rendered body (`{{ .content }}`,
// `{{ .page.content }}`) and the page's front-matter summary (on the
// page and in page lists). Everything else is escaped whatever its key is
// called. Liquid output is unchanged.
//
// See PLAN.md → "Go templates trust HTML by origin (issue #1267)".

// goTrustConfig returns a minimal Go-template site config for BuildWithContent.
func goTrustConfig(title string) *config.Config {
	return &config.Config{
		Title:     title,
		BaseURL:   "https://example.com",
		Build:     config.BuildConfig{Output: "_site"},
		Templates: config.TemplatesConfig{Engine: "gotemplate"},
	}
}

const trustData = `{
  "content": "Great! <script>steal()</script>",
  "summary": "<u>top</u>",
  "rating": "5 <stars>",
  "nested": {"summary": "<u>deep</u>", "content": "<i>deep</i>"}
}`

var _ = Describe("Go templates trust HTML by origin (issue #1267)", func() {

	Context("values Alloy did not produce are escaped", func() {
		build := func() string {
			GinkgoHelper()
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Data"), map[string]string{
				"data/reviews.json":    trustData,
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\nBody",
				"layouts/default.html": "C[{{ .site.data.reviews.content }}] S[{{ .site.data.reviews.summary }}] NS[{{ .site.data.reviews.nested.summary }}] NC[{{ .site.data.reviews.nested.content }}] R[{{ .site.data.reviews.rating }}]",
			})
			Expect(err).NotTo(HaveOccurred())
			return result.RenderedContent["index.md"]
		}

		It("escapes a data file's content field", func() {
			Expect(build()).To(ContainSubstring("C[Great! &lt;script&gt;steal()&lt;/script&gt;]"),
				"a data file's `content` is the author's data, not HTML Alloy "+
					"produced — it must be escaped like its sibling fields, or the "+
					"script runs (issue #1267)")
		})

		It("escapes a data file's summary field", func() {
			Expect(build()).To(ContainSubstring("S[&lt;u&gt;top&lt;/u&gt;]"),
				"only a page's summary is trusted, not any field named summary (issue #1267)")
		})

		It("escapes content and summary at any depth in data", func() {
			out := build()
			Expect(out).To(ContainSubstring("NS[&lt;u&gt;deep&lt;/u&gt;]"))
			Expect(out).To(ContainSubstring("NC[&lt;i&gt;deep&lt;/i&gt;]"),
				"the old rule matched the key name at any depth (issue #1267)")
		})

		It("escapes a front-matter content field that fills page.content on an empty page", func() {
			// With no body, page.content falls back to front matter. That is
			// authored text, not the rendered body.
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust FM Content"), map[string]string{
				"content/empty.md":     "---\ntitle: Empty\nlayout: default\ncontent: \"<i>fm</i>\"\n---\n",
				"layouts/default.html": "P[{{ .page.content }}]",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent["empty.md"]).To(ContainSubstring("P[&lt;i&gt;fm&lt;/i&gt;]"),
				"page.content is trusted only when it is the page's rendered "+
					"body (issue #1267)")
		})

		It("still escapes ordinary data fields (guard)", func() {
			Expect(build()).To(ContainSubstring("R[5 &lt;stars&gt;]"))
		})
	})

	Context("HTML Alloy produced or vouches for stays raw (guards)", func() {
		files := map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: default\nsummary: \"<b>sum</b>\"\n---\nBody with **bold**",
			"content/post.md":  "---\ntitle: Post\nlayout: default\nsummary: \"<em>post</em>\"\n---\nPost body",
			"layouts/default.html": "TOP[{{ .content }}] PAGE[{{ .page.content }}] SUM[{{ .page.summary }}] " +
				"LIST[{{ range .site.pages }}{{ .summary }};{{ end }}] INC[{{ include \"card\" .page }}]",
			"layouts/card.html": "{{ .content }}",
		}
		var out string
		BeforeEach(func() {
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Guards"), files)
			Expect(err).NotTo(HaveOccurred())
			out = result.RenderedContent["index.md"]
		})

		It("renders the layout's {{ .content }} raw", func() {
			Expect(out).To(ContainSubstring("TOP[<p>Body with <strong>bold</strong></p>"),
				"the rendered page body is HTML Alloy produced — it must stay raw")
		})

		It("renders {{ .page.content }} raw", func() {
			Expect(out).To(ContainSubstring("PAGE[<p>Body with <strong>bold</strong></p>"))
		})

		It("renders the page's summary raw (maintainer decision)", func() {
			Expect(out).To(ContainSubstring("SUM[<b>sum</b>]"),
				"a page's front-matter summary stays trusted (issue #1267 decision)")
		})

		It("renders summaries in page lists raw", func() {
			Expect(out).To(ContainSubstring("<em>post</em>;"),
				"site.pages items carry the same trusted summary as the page itself")
			Expect(out).To(ContainSubstring("<b>sum</b>;"))
		})

		It("keeps the page body raw through an include", func() {
			Expect(out).To(ContainSubstring("INC[<p>Body with <strong>bold</strong></p>"),
				"trust belongs to the value, so it survives being passed as dot "+
					"to an include")
		})
	})

	Context("filters compare trusted values as text", func() {
		It("matches a page's summary with where in Go templates", func() {
			// A trusted summary must still equal the plain string it holds
			// (#1279 review). Already broken in Go templates before #1267:
			// summary was template.HTML and Where compared with ==.
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Where"), map[string]string{
				"content/index.md":     "---\ntitle: Home\nlayout: default\nsummary: \"<b>sum</b>\"\n---\nBody",
				"content/post.md":      "---\ntitle: Post\nlayout: default\nsummary: \"other\"\n---\nPost",
				"layouts/default.html": "WHERE[{{ len (where .site.pages \"summary\" \"<b>sum</b>\") }}]",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent["index.md"]).To(ContainSubstring("WHERE[1]"),
				"where must match a trusted value against a plain string argument "+
					"(issue #1267)")
		})
	})

	Context("sort and set filters compare trusted summaries as text (#1281)", func() {
		// Page summaries reach Go-template filters as template.HTML. Filters
		// must read their text: sort compares number-like text as numbers,
		// and the set filters must treat a trusted "9" and a plain "9" as the
		// same value.
		var out string
		BeforeEach(func() {
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Filters"), map[string]string{
				"content/blog/a.md": "---\ntitle: A\nlayout: default\nsummary: \"10\"\n---\nA",
				"content/blog/b.md": "---\ntitle: B\nlayout: default\nsummary: \"9\"\n---\nB",
				"content/blog/c.md": "---\ntitle: C\nlayout: default\nsummary: \"9\"\n---\nC",
				"data/plain.json":   `["9","x"]`,
				"layouts/default.html": "SORT[{{ range sort .site.pages \"summary\" }}{{ .title }}{{ end }}] " +
					"UNIQ[{{ len (uniq (concat (map .site.pages \"summary\") .site.data.plain)) }}] " +
					"INT[{{ intersect (map .site.pages \"summary\") .site.data.plain }}] " +
					"UNION[{{ union (map .site.pages \"summary\") .site.data.plain }}] " +
					"COMP[{{ complement (map .site.pages \"summary\") .site.data.plain }}]",
			})
			Expect(err).NotTo(HaveOccurred())
			out = result.RenderedContent["blog/a.md"]
		})

		It("sorts number-like summaries numerically", func() {
			Expect(out).To(ContainSubstring("SORT[BCA]"),
				"9, 9, 10 in numeric order — compared as text-by-type, a trusted "+
					"summary skips the numeric branch and sorts as 10, 9, 9")
		})

		It("uniq treats a trusted value and an equal plain string as one", func() {
			Expect(out).To(ContainSubstring("UNIQ[3]"),
				"{10, 9, x} — keyed by the raw value, trusted \"9\" and plain \"9\" "+
					"would count twice")
		})

		It("intersect, union and complement match trusted values against plain strings", func() {
			Expect(out).To(ContainSubstring("INT[[9 9]]"))
			Expect(out).To(ContainSubstring("UNION[[10 9 x]]"))
			Expect(out).To(ContainSubstring("COMP[[10]]"))
		})
	})

	Context("a page's own summary stays raw in page lists however the page was made (#1281)", func() {
		var out string
		BeforeEach(func() {
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Lists"), map[string]string{
				"data/team.json":       `[{"name":"Ann","slug":"ann"},{"name":"Bo","slug":"bo"}]`,
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\nHome",
				"content/team.md":      "---\ntitle: Team\nlayout: default\nsummary: \"<em>static</em>\"\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/team/{{ .member.slug }}/\"\n---\nT",
				"content/people.md":    "---\ntitle: People\nlayout: default\nsummary: \"<i>{{ .member.name }}</i>\"\npagination:\n  data: site.data.team\n  perPage: 1\n  as: member\npermalink: \"/people/{{ .member.slug }}/\"\n---\nP",
				"layouts/default.html": "LIST[{{ range .site.pages }}{{ .summary }};{{ end }}]",
				"plugins/add.js": `export default function(alloy) {
  alloy.hook('onPagesReady', { pages: false }, function(payload) {
    return { addPages: [{ path: 'extra.md', url: '/extra/', frontMatter: { title: 'Extra', layout: 'default', summary: '<em>plugin</em>' }, content: 'x' }] };
  });
}`,
			})
			Expect(err).NotTo(HaveOccurred())
			out = result.RenderedContent["index.md"]
		})

		It("keeps a paginated page's static summary raw", func() {
			Expect(out).To(ContainSubstring("<em>static</em>;<em>static</em>;"),
				"pagination virtual pages carry their summary only in front matter; "+
					"it is still the page's own summary (issue #1267)")
		})

		It("keeps a paginated page's interpolated summary raw", func() {
			Expect(out).To(ContainSubstring("<i>Ann</i>;<i>Bo</i>;"),
				"the per-item summary pagination renders from the page's own summary is trusted")
		})

		It("keeps a plugin addPages page's summary raw", func() {
			Expect(out).To(ContainSubstring("<em>plugin</em>;"),
				"the frontMatter.summary of a page added with addPages is that page's own summary")
		})
	})

	Context("a summary the page inherits is escaped (#1283)", func() {
		// Only a summary in the page's own front matter is trusted. The data
		// cascade merges directory _data.yaml and global data into every
		// page's front matter as defaults — including a data file named
		// summary — and that is data, not the page's own summary.
		It("escapes a summary inherited from a directory _data.yaml", func() {
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Inherited Dir"), map[string]string{
				"content/blog/post.md":    "---\ntitle: Post\nlayout: default\n---\nPost",
				"content/blog/_data.yaml": "summary: \"<script>dir()</script>\"\n",
				"layouts/default.html":    "P[{{ .page.summary }}] LIST[{{ range .site.pages }}{{ .summary }};{{ end }}]",
			})
			Expect(err).NotTo(HaveOccurred())
			out := result.RenderedContent["blog/post.md"]
			Expect(out).To(ContainSubstring("P[&lt;script&gt;dir()&lt;/script&gt;]"),
				"an inherited summary on the page must be escaped (maintainer decision, #1282 review)")
			Expect(out).To(ContainSubstring("LIST[&lt;script&gt;dir()&lt;/script&gt;;]"),
				"an inherited summary in page lists must be escaped")
		})

		It("escapes a summary inherited from a data file named summary", func() {
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Inherited Global"), map[string]string{
				"data/summary.yaml":    "\"<script>global()</script>\"\n",
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\nHome",
				"layouts/default.html": "P[{{ .page.summary }}] LIST[{{ range .site.pages }}{{ .summary }};{{ end }}]",
			})
			Expect(err).NotTo(HaveOccurred())
			out := result.RenderedContent["index.md"]
			Expect(out).To(ContainSubstring("P[&lt;script&gt;global()&lt;/script&gt;]"))
			Expect(out).To(ContainSubstring("LIST[&lt;script&gt;global()&lt;/script&gt;;]"))
		})
	})

	Context("Liquid is unchanged (guard)", func() {
		It("sees content and summary as plain strings in filters and comparisons", func() {
			// Trust markers exist only in Go-template builds; Liquid gets the
			// plain strings it always had (#1283).
			result, err := pipeline.BuildWithContent(&config.Config{
				Title:   "Trust Liquid",
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
				"where compares a page's summary as text — a marker that does not "+
					"compare equal to a plain string would make it match nothing "+
					"(#1279 review)")
			Expect(out).To(ContainSubstring("RAW[<b>sum</b>]"))
		})

		It("sees plain strings in property lookups and sum (#1283)", func() {
			// liquidgo calls ToLiquid only at the end of a lookup; .size
			// part-way through, and filters like sum that read properties,
			// test for string directly. A marker in a Liquid build would empty
			// these. Values match a Liquid build with plain strings.
			result, err := pipeline.BuildWithContent(&config.Config{
				Title:   "Trust Liquid Lookups",
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
			Expect(out).To(ContainSubstring("PC[9]"), "page.content.size — <p>A</p> plus newline")
			Expect(out).To(ContainSubstring("LOOP[2,1,1,]"), "p.summary.size in a loop")
			Expect(out).To(ContainSubstring("SUM[28]"), "sum: \"summary\" — 10 + 9 + 9")
		})
	})

	Context("opting back in", func() {
		It("renders a data file's HTML raw with safeHTML (needs #1266)", func() {
			// This is the supported replacement for the old key-name trust.
			// It stays red until the #1266 implementation makes safeHTML
			// return template.HTML — #1267 must not ship before it.
			result, err := pipeline.BuildWithContent(goTrustConfig("Trust Opt In"), map[string]string{
				"data/reviews.json":    trustData,
				"content/index.md":     "---\ntitle: Home\nlayout: default\n---\nBody",
				"layouts/default.html": "O[{{ safeHTML .site.data.reviews.nested.content }}]",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RenderedContent["index.md"]).To(ContainSubstring("O[<i>deep</i>]"),
				"authors whose data-file HTML now escapes need a working opt-in; "+
					"safeHTML is it (issue #1267, depends on #1266)")
		})
	})
})
