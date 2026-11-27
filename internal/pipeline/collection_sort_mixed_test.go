package pipeline_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── A collection's sortBy puts numbers before text (issue #1418) ─────────
// A collection sorted by a front matter key compares values as the sort
// filter does: numbers first, in numeric order, then everything else as
// text. Missing values go last. order: "desc" reverses the numbers and the
// text, and missing values stay last.
//
// Before, a number and a text value compared as two strings. With weights
// "2", 10 and "1a", that made a loop (2 < 10, "10" < "1a", "1a" < "2"), so
// the order depended on the pages' paths.
//
// See PLAN.md → "How the sort filter compares values".
var _ = Describe("A collection's sortBy puts numbers before text (issue #1418)", Ordered, ContinueOnFailure, func() {
	titles := func(list string) string {
		return "{% for p in " + list + " %}{{ p.title }};{% endfor %}"
	}
	page := func(title, weight string) string {
		fm := "---\ntitle: " + title + "\n"
		if weight != "" {
			fm += "weight: " + weight + "\n"
		}
		return fm + "---\n" + strings.ToLower(title)
	}

	var out map[string]string
	BeforeAll(func() {
		files := map[string]string{
			"content/index.md": "---\ntitle: Home\nlayout: home\n---\nhome",

			// The same weights, "2", 10 and "1a", in three path orders.
			"content/mixa/a1.md": page("T2", `"2"`),
			"content/mixa/a2.md": page("T10", "10"),
			"content/mixa/a3.md": page("T1A", `"1a"`),
			"content/mixb/b1.md": page("T1A", `"1a"`),
			"content/mixb/b2.md": page("T10", "10"),
			"content/mixb/b3.md": page("T2", `"2"`),
			"content/mixc/c1.md": page("T2", `"2"`),
			"content/mixc/c2.md": page("T1A", `"1a"`),
			"content/mixc/c3.md": page("T10", "10"),

			// Every kind of value: ints (-3, 10), a float (1.5), a
			// digit-only string ("2"), text ("-1", "1a", "b") and none.
			"content/kinds/k1.md": page("B", `"b"`),
			"content/kinds/k2.md": page("TEN", "10"),
			"content/kinds/k3.md": page("NONE", ""),
			"content/kinds/k4.md": page("ONEA", `"1a"`),
			"content/kinds/k5.md": page("HALF", "1.5"),
			"content/kinds/k6.md": page("MINUSONE", `"-1"`),
			"content/kinds/k7.md": page("TWO", `"2"`),
			"content/kinds/k8.md": page("MINUSTHREE", "-3"),

			"layouts/home.liquid": "MIXA[" + titles("collections.mixa") + "] " +
				"MIXB[" + titles("collections.mixb") + "] " +
				"MIXC[" + titles("collections.mixc") + "] " +
				"KINDS[" + titles("collections.kinds") + "] " +
				"KINDSDESC[" + titles("collections.kindsdesc") + "]",
		}
		// kindsdesc: the same pages as kinds, sorted desc.
		desc := map[string]string{}
		for rel, body := range files {
			if name, ok := strings.CutPrefix(rel, "content/kinds/"); ok {
				desc["content/kindsdesc/"+name] = body
			}
		}
		for rel, body := range desc {
			files[rel] = body
		}
		asc := &config.CollectionConfig{SortBy: "weight", Order: "asc"}
		cfg := &config.Config{
			Title:   "Collection Sort Mixed",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
			Collections: map[string]*config.CollectionConfig{
				"mixa":      asc,
				"mixb":      asc,
				"mixc":      asc,
				"kinds":     asc,
				"kindsdesc": {SortBy: "weight", Order: "desc"},
			},
		}
		result, err := pipeline.BuildWithContent(cfg, files, pipeline.BuildOptions{CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		out = result.RenderedContent
	})

	It(`sorts "2", 10 and "1a" as 2, 10, 1a whatever the pages' paths`, func() {
		Expect(out["index.md"]).To(ContainSubstring("MIXA[T2;T10;T1A;]"))
		Expect(out["index.md"]).To(ContainSubstring("MIXB[T2;T10;T1A;]"))
		Expect(out["index.md"]).To(ContainSubstring("MIXC[T2;T10;T1A;]"))
	})
	It("sorts numbers first, then text, then pages without the key", func() {
		Expect(out["index.md"]).To(ContainSubstring("KINDS[MINUSTHREE;HALF;TWO;TEN;MINUSONE;ONEA;B;NONE;]"))
	})
	It("reverses numbers and text with order: desc, and still puts pages without the key last", func() {
		Expect(out["index.md"]).To(ContainSubstring("KINDSDESC[B;ONEA;MINUSONE;TEN;TWO;HALF;MINUSTHREE;NONE;]"))
	})
})
