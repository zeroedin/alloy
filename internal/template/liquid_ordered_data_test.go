package template_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/content"
	"github.com/zeroedin/alloy/internal/ordered"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── Liquid iterates data files in file order (guard) ─────────────────
// Data files load as *ordered.Map, and Liquid iterates them in file order.
// The data goes through BuildTemplateContext().ToMap(), the path the
// pipeline uses, so a change to the shared context builder that flattens
// ordered maps into plain maps fails here. The loader half is covered by
// internal/data/loader_test.go ("JSON LoadFileAny returns *ordered.Map").
// Moved from the Go ordered-data tests (#1237) when Go templates were
// removed (#1285).

var _ = Describe("Liquid iterates data files in file order (guard)", func() {
	It("keeps *ordered.Map insertion order through the shared context builder", func() {
		nav, err := ordered.UnmarshalJSONValue([]byte(`{"zebra": "z", "apple": "a", "middle": "m"}`))
		Expect(err).NotTo(HaveOccurred())
		page := &content.Page{
			RelPath:     "index.md",
			URL:         "/",
			FrontMatter: map[string]interface{}{"title": "Home"},
		}
		siteData := map[string]interface{}{
			"title": "Test Site",
			"data":  map[string]interface{}{"nav": nav},
		}
		ctx := tmpl.BuildTemplateContext(
			page, siteData, []*content.Page{page}, nil, nil, nil, "").ToMap()

		tpl, err := tmpl.NewLiquidEngine().Parse("l", []byte(
			`{% for kv in site.data.nav %}{{ kv[0] }};{% endfor %}`))
		Expect(err).NotTo(HaveOccurred())
		out, err := tpl.Render(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(Equal("zebra;apple;middle;"),
			"Liquid must iterate in file order; alphabetical means the shared "+
				"context builder flattened the ordered map")
	})
})
