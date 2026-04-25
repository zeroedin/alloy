package collection_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/collection"
	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/content"
)

func boolPtr(b bool) *bool { return &b }

var _ = Describe("Taxonomy", func() {

	var (
		pages       []*content.Page
		taxonomyCfg map[string]*config.TaxonomyConfig
	)

	BeforeEach(func() {
		pages = []*content.Page{
			{FrontMatter: map[string]interface{}{"title": "Post 1", "tags": []interface{}{"go", "web"}, "mood": []interface{}{"happy"}}},
			{FrontMatter: map[string]interface{}{"title": "Post 2", "tags": []interface{}{"go", "testing"}}},
		}
		taxonomyCfg = map[string]*config.TaxonomyConfig{
			"tags": {Permalink: "/tags/:slug/", Layout: "tags"},
		}
	})

	// ── Taxonomy collection building ───────────────────────────────────

	Describe("Taxonomy collection building", func() {
		It("creates taxonomy collection from declared taxonomy keys", func() {
			taxonomies := collection.BuildTaxonomies(pages, taxonomyCfg)
			Expect(taxonomies).NotTo(BeNil())
			Expect(taxonomies).To(HaveKey("tags"))
		})

		It("groups pages by front matter tag values", func() {
			taxonomies := collection.BuildTaxonomies(pages, taxonomyCfg)
			Expect(taxonomies).NotTo(BeNil())
			Expect(taxonomies["tags"].Terms).To(HaveKey("go"))
			Expect(taxonomies["tags"].Terms["go"]).To(HaveLen(2))
		})

		It("a page with multiple tags appears in multiple term collections", func() {
			taxonomies := collection.BuildTaxonomies(pages, taxonomyCfg)
			Expect(taxonomies).NotTo(BeNil())
			Expect(taxonomies["tags"].Terms).To(HaveKey("web"))
			Expect(taxonomies["tags"].Terms["web"]).To(HaveLen(1))
			Expect(taxonomies["tags"].Terms).To(HaveKey("testing"))
			Expect(taxonomies["tags"].Terms["testing"]).To(HaveLen(1))
		})

		It("ignores undeclared front matter array keys (e.g., mood not in config)", func() {
			taxonomies := collection.BuildTaxonomies(pages, taxonomyCfg)
			Expect(taxonomies).NotTo(BeNil())
			Expect(taxonomies).NotTo(HaveKey("mood"))
		})
	})

	// ── Taxonomy page generation ───────────────────────────────────────

	Describe("Taxonomy page generation", func() {
		It("generates index page for each taxonomy (/tags/)", func() {
			taxonomies := collection.BuildTaxonomies(pages, taxonomyCfg)
			Expect(taxonomies).NotTo(BeNil())
			Expect(taxonomies).To(HaveKey("tags"))

			generated := collection.GenerateTaxonomyPages(taxonomies["tags"], taxonomyCfg["tags"])
			Expect(generated).NotTo(BeNil())

			var indexFound bool
			for _, p := range generated {
				if p.URL == "/tags/" {
					indexFound = true
					break
				}
			}
			Expect(indexFound).To(BeTrue(), "expected an index page at /tags/")
		})

		It("generates per-term page for each term (/tags/javascript/)", func() {
			taxonomies := collection.BuildTaxonomies(pages, taxonomyCfg)
			Expect(taxonomies).NotTo(BeNil())
			Expect(taxonomies).To(HaveKey("tags"))

			generated := collection.GenerateTaxonomyPages(taxonomies["tags"], taxonomyCfg["tags"])
			Expect(generated).NotTo(BeNil())

			var goTermFound bool
			for _, p := range generated {
				if p.URL == "/tags/go/" {
					goTermFound = true
					break
				}
			}
			Expect(goTermFound).To(BeTrue(), "expected a term page at /tags/go/")
		})

		It("uses default permalink pattern when not customized", func() {
			defaultCfg := &config.TaxonomyConfig{Layout: "categories"}
			taxonomy := &collection.TaxonomyCollection{
				Name: "categories",
				Terms: map[string][]*content.Page{
					"golang": {pages[0]},
				},
			}

			generated := collection.GenerateTaxonomyPages(taxonomy, defaultCfg)
			Expect(generated).NotTo(BeNil())

			var termFound bool
			for _, p := range generated {
				if p.URL == "/categories/golang/" {
					termFound = true
					break
				}
			}
			Expect(termFound).To(BeTrue(), "expected a term page at /categories/golang/ using default pattern")
		})
	})

	// ── Taxonomy page template context ────────────────────────────────

	Describe("Taxonomy page template context", func() {
		It("index page context provides all terms with metadata", func() {
			taxonomy := &collection.TaxonomyCollection{
				Name: "tags",
				Terms: map[string][]*content.Page{
					"go":      {pages[0], pages[1]},
					"web":     {pages[0]},
					"testing": {pages[1]},
				},
			}
			ctx := collection.BuildTaxonomyPageContext(taxonomy, "")
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Term).To(Equal(""),
				"index page context must have empty Term")
			Expect(ctx.Terms).NotTo(BeEmpty(),
				"index page context must provide Terms list")
			Expect(ctx.Terms).To(HaveLen(3))
		})

		It("term page context provides term name and pages", func() {
			taxonomy := &collection.TaxonomyCollection{
				Name: "tags",
				Terms: map[string][]*content.Page{
					"go":  {pages[0], pages[1]},
					"web": {pages[0]},
				},
			}
			ctx := collection.BuildTaxonomyPageContext(taxonomy, "go")
			Expect(ctx).NotTo(BeNil())
			Expect(ctx.Term).To(Equal("go"),
				"term page context must have the term name")
			Expect(ctx.Pages).To(HaveLen(2),
				"term page context must have matching pages")
		})

		It("ToMap serializes term pages as 'pages' key, not 'items' (issue #96)", func() {
			taxonomy := &collection.TaxonomyCollection{
				Name: "tags",
				Terms: map[string][]*content.Page{
					"go": {pages[0], pages[1]},
				},
			}
			ctx := collection.BuildTaxonomyPageContext(taxonomy, "go")
			m := ctx.ToMap()
			Expect(m).To(HaveKey("pages"),
				"ToMap must serialize term pages under 'pages' key per spec")
			Expect(m).NotTo(HaveKey("items"),
				"ToMap must not use 'items' key for taxonomy term pages")
		})

		It("ToMap serializes each term's pages as 'pages' key in terms list (issue #96)", func() {
			taxonomy := &collection.TaxonomyCollection{
				Name: "tags",
				Terms: map[string][]*content.Page{
					"go":  {pages[0]},
					"web": {pages[1]},
				},
			}
			ctx := collection.BuildTaxonomyPageContext(taxonomy, "")
			m := ctx.ToMap()
			terms, ok := m["terms"].([]map[string]interface{})
			Expect(ok).To(BeTrue(), "terms must be a slice of maps")
			for _, term := range terms {
				Expect(term).To(HaveKey("pages"),
					"each term in terms list must have 'pages' key")
				Expect(term).NotTo(HaveKey("items"),
					"each term in terms list must not use 'items' key")
			}
		})
	})

	// ── Duplicate term slug detection ────────────────────────────────

	Describe("Duplicate term slug detection", func() {
		It("detects duplicate taxonomy term slugs from different source values", func() {
			taxonomy := &collection.TaxonomyCollection{
				Name: "tags",
				Terms: map[string][]*content.Page{
					"C++":    {},
					"c-plus": {},
				},
			}
			dupes := collection.DetectDuplicateTermSlugs(taxonomy)
			Expect(dupes).NotTo(BeEmpty(),
				"terms that slug to the same value must be flagged")
		})
	})

	// ── Custom taxonomy permalink patterns ────────────────────────────

	Describe("Custom taxonomy permalink patterns", func() {
		It("uses custom permalink pattern from taxonomy config", func() {
			customCfg := &config.TaxonomyConfig{
				Permalink: "/topics/:slug/",
				Layout:    "term",
			}
			taxonomy := &collection.TaxonomyCollection{
				Name: "tags",
				Terms: map[string][]*content.Page{
					"go": {pages[0]},
				},
			}
			generated := collection.GenerateTaxonomyPages(taxonomy, customCfg)
			Expect(generated).NotTo(BeNil())

			var goTermFound bool
			for _, p := range generated {
				if p.URL == "/topics/go/" {
					goTermFound = true
					break
				}
			}
			Expect(goTermFound).To(BeTrue(),
				"custom permalink /topics/:slug/ must produce /topics/go/")
		})
	})

	// ── render: false (issue #319) ──────────────────────────────────
	// Taxonomies with render: false build collection data but do not
	// generate output pages.

	Describe("render: false", func() {
		It("taxonomy data is built even when render is false", func() {
			tagsCfg := map[string]*config.TaxonomyConfig{
				"tags": {Render: boolPtr(false)},
			}
			taxonomies := collection.BuildTaxonomies(pages, tagsCfg)
			Expect(taxonomies).To(HaveKey("tags"),
				"taxonomy data must be built even with render: false — "+
					"collections.taxonomies.tags.* must be available in templates")
			tc := taxonomies["tags"]
			Expect(tc.Terms).NotTo(BeEmpty(),
				"terms must be populated for collection access")
		})

		It("does not generate pages when render is false", func() {
			tagsCfg := &config.TaxonomyConfig{Render: boolPtr(false)}
			tc := collection.BuildTaxonomies(pages, map[string]*config.TaxonomyConfig{
				"tags": tagsCfg,
			})["tags"]

			taxPages := collection.GenerateTaxonomyPages(tc, tagsCfg)
			Expect(taxPages).To(BeEmpty(),
				"GenerateTaxonomyPages must return no pages when render is false")
		})

		// NOTE: Duplicate term slug checking is skipped in the pipeline
		// (generateTaxonomyPages in build.go) for render: false taxonomies.
		// DetectDuplicateTermSlugs itself is unchanged — the pipeline
		// simply doesn't call it when render is false.

		It("generates pages when render is true (default)", func() {
			tagsCfg := &config.TaxonomyConfig{Render: boolPtr(true), Layout: "tags"}
			tc := collection.BuildTaxonomies(pages, map[string]*config.TaxonomyConfig{
				"tags": tagsCfg,
			})["tags"]

			taxPages := collection.GenerateTaxonomyPages(tc, tagsCfg)
			Expect(taxPages).NotTo(BeEmpty(),
				"GenerateTaxonomyPages must generate pages when render is true")
		})
	})
})
