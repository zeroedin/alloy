package template_test

import (
	"strings"

	"github.com/zeroedin/alloy/internal/content"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── Ordered data in Go templates (issue #1237) ────────────────────────
//
// Go's html/template cannot traverse *ordered.Map with dot notation, so
// every access to ordered data needed nested oget calls. Verified on main
// with the built CLI:
//
//	{{ .site.data.pkgjson.zebra }}
//	  -> can't evaluate field zebra in type interface {}
//	{{ range $k, $v := .site.data.pkgjson }}
//	  -> range can't iterate over {[zebra apple ...] map[...]}
//	{{ range .site.data.items }}{{ .label }}{{ end }}     (array elements)
//	  -> can't evaluate field label in type interface {}
//
// A project with no JSON file at all hits the same wall through a plugin,
// because hook returns are rewrapped as *ordered.Map — so the fix belongs
// at the engine boundary, not at the JSON loader.
//
// The fix converts every *ordered.Map reaching the Go engine's render
// context to map[string]interface{}, recursively through maps and slices.
// Iteration order is the cost: a value that supports dot notation must be
// a map, and a Go map cannot carry order.
//
// Spec: PLAN.md → "Ordered Data in Go Templates (issue #1237)".

var _ = Describe("Ordered data in Go templates (issue #1237)", func() {
	var engine tmpl.TemplateEngine

	BeforeEach(func() {
		engine = tmpl.NewGoEngine()
	})

	// render parses and renders in one step, failing the test on either error.
	render := func(src string, ctx map[string]interface{}) string {
		GinkgoHelper()
		tpl, err := engine.Parse("t", []byte(src))
		Expect(err).NotTo(HaveOccurred(), "template must parse")
		out, err := tpl.Render(ctx)
		Expect(err).NotTo(HaveOccurred(), "template must render")
		return string(out)
	}

	// fromJSON builds the same *ordered.Map a JSON data file produces.
	fromJSON := func(s string) interface{} {
		GinkgoHelper()
		v, err := ordered.UnmarshalJSONValue([]byte(s))
		Expect(err).NotTo(HaveOccurred())
		return v
	}

	// ── RED: dot notation on ordered data ─────────────────────────────

	Context("Dot notation", func() {
		It("resolves a key on an ordered map", func() {
			ctx := map[string]interface{}{
				"site": map[string]interface{}{
					"data": map[string]interface{}{
						"pkg": fromJSON(`{"version": "1.2.3", "name": "my-pkg"}`),
					},
				},
			}
			Expect(render(`{{ .site.data.pkg.version }}`, ctx)).To(Equal("1.2.3"),
				"dot notation must resolve ordered-map keys — this is the whole "+
					"point of issue #1237; on main it fails with "+
					"\"can't evaluate field version in type interface {}\"")
		})

		It("resolves through nested ordered maps at depth", func() {
			ctx := map[string]interface{}{
				"d": fromJSON(`{"nested": {"deep": {"leaf": "bottom"}}}`),
			}
			Expect(render(`{{ .d.nested.deep.leaf }}`, ctx)).To(Equal("bottom"),
				"conversion must recurse — a single-level conversion leaves "+
					"nested ordered maps untraversable")
		})

		It("resolves fields on ordered maps inside a slice", func() {
			// The commonest JSON shape. markHTMLSafe does not currently
			// recurse into []interface{}, so this is the case a maps-only
			// conversion silently misses.
			ctx := map[string]interface{}{
				"items": fromJSON(`[{"label": "first"}, {"label": "second"}]`),
			}
			Expect(render(`{{ range .items }}{{ .label }};{{ end }}`, ctx)).
				To(Equal("first;second;"),
					"conversion must recurse into slices — array elements are "+
						"ordered maps too")
		})

		It("resolves ordered maps nested inside a slice inside an ordered map", func() {
			ctx := map[string]interface{}{
				"d": fromJSON(`{"groups": [{"items": [{"id": "x"}]}]}`),
			}
			Expect(render(`{{ range .d.groups }}{{ range .items }}{{ .id }}{{ end }}{{ end }}`, ctx)).
				To(Equal("x"),
					"map and slice recursion must compose at arbitrary depth")
		})

		It("renders empty for a missing key rather than erroring", func() {
			ctx := map[string]interface{}{"d": fromJSON(`{"present": "yes"}`)}
			Expect(render(`[{{ .d.nope }}]`, ctx)).To(Equal("[]"),
				"missing keys must behave like plain maps — empty, not an error")
		})
	})

	// ── RED: iteration ────────────────────────────────────────────────

	Context("Iteration", func() {
		It("allows bare range over ordered data, in sorted key order", func() {
			// On main this is a render error: "range can't iterate over".
			// After conversion it succeeds. Go's text/template sorts map keys
			// via fmtsort with no extension point, so sorted is the contract.
			ctx := map[string]interface{}{
				"d": fromJSON(`{"zebra": "z", "apple": "a", "middle": "m"}`),
			}
			Expect(render(`{{ range $k, $v := .d }}{{ $k }}={{ $v }};{{ end }}`, ctx)).
				To(Equal("apple=a;middle=m;zebra=z;"),
					"bare range must succeed and yield sorted keys — insertion "+
						"order is not recoverable from a Go map")
		})

		It("makes orange deterministic across repeated renders", func() {
			// The payload: orange's map branch ranges the map directly, which
			// Go randomizes. Measured on main, three builds of unchanged input
			// produced three different orders (issue #1262). After conversion
			// every orange input is a plain map, so without an explicit sort
			// that nondeterminism becomes universal.
			//
			// Twelve keys: Go's map iteration randomization is reliably
			// observable above the small-map threshold, so a test with three
			// keys can pass by luck.
			ctx := map[string]interface{}{
				"d": fromJSON(`{"k07":7,"k11":11,"k02":2,"k09":9,"k00":0,` +
					`"k05":5,"k01":1,"k10":10,"k03":3,"k08":8,"k04":4,"k06":6}`),
			}
			src := `{{ range orange .d }}{{ .Key }} {{ end }}`

			first := render(src, ctx)
			for i := 0; i < 24; i++ {
				Expect(render(src, ctx)).To(Equal(first),
					"orange must sort its keys — ranging a Go map directly "+
						"produces different output on every render, which makes "+
						"build output nonreproducible")
			}

			Expect(strings.TrimSpace(first)).To(Equal(
				"k00 k01 k02 k03 k04 k05 k06 k07 k08 k09 k10 k11"),
				"sorted order is the contract, so the stable order must be the "+
					"sorted one and not whichever order happened to be first")
		})
	})

	// ── GREEN GUARDS: things that must keep working ───────────────────

	Context("Existing helpers keep working", func() {
		It("oget still resolves a key on converted data", func() {
			ctx := map[string]interface{}{"d": fromJSON(`{"white": "#fff"}`)}
			Expect(render(`{{ oget .d "white" }}`, ctx)).To(Equal("#fff"),
				"oget's map[string]interface{} branch must cover converted "+
					"values — note a NAMED map type would not satisfy that type "+
					"assertion, which is why the conversion target is plain "+
					"map[string]interface{}")
		})

		It("orange still exposes .Key and .Value", func() {
			ctx := map[string]interface{}{"d": fromJSON(`{"a": "1", "b": "2"}`)}
			Expect(render(`{{ range orange .d }}{{ .Key }}:{{ .Value }};{{ end }}`, ctx)).
				To(Equal("a:1;b:2;"),
					"orange's pair shape is documented and must not change; only "+
						"its ordering guarantee does")
		})

		It("value filters still resolve fields on converted data", func() {
			// where/sort/group_by/map resolve fields through getMapValue,
			// which handles map[string]interface{} and *ordered.Map (#477).
			// Conversion must land on the branch it already has.
			//
			// Deliberately uses oget rather than dot notation on the filter
			// output: oget works on both the pre-change and post-change shape,
			// so this passes today and must keep passing. A dot-notation
			// version would be red today and could not establish a baseline —
			// dot access on filter output is covered by the slice test above.
			// Built-in filters are registered by the pipeline, not by
			// NewGoEngine, and html/template binds functions at parse time —
			// so registration must happen before Parse.
			Expect(tmpl.RegisterBuiltinFilters(engine)).To(Succeed())

			ctx := map[string]interface{}{
				"items": fromJSON(`[{"id":1,"label":"first"},{"id":2,"label":"second"}]`),
			}
			Expect(render(`{{ range where .items "label" "second" }}{{ oget . "id" }}{{ end }}`, ctx)).
				To(Equal("2"),
					"where must keep matching after conversion — if this returns "+
						"empty, getMapValue is missing the converted type")
			Expect(render(`{{ map .items "label" }}`, ctx)).
				To(Equal("[first second]"),
					"map must keep extracting fields after conversion")
		})
	})

	Context("Conversion is engine-local and non-destructive", func() {
		It("does not mutate the source ordered map", func() {
			// The source tree is shared with PipelineState, incremental
			// rebuild state, and plugin payloads. Rendering must not disturb it.
			src := fromJSON(`{"zebra": "z", "apple": "a", "middle": "m"}`)
			om, ok := src.(*ordered.Map)
			Expect(ok).To(BeTrue(), "JSON objects must decode to *ordered.Map")

			render(`{{ .d.zebra }}{{ range orange .d }}{{ .Key }}{{ end }}`,
				map[string]interface{}{"d": src})

			Expect(om.Keys()).To(Equal([]string{"zebra", "apple", "middle"}),
				"the source *ordered.Map must still hold its insertion order "+
					"after a Go template render — conversion produces new values")
			Expect(om.Get("zebra")).To(Equal("z"),
				"source values must be untouched")
		})

		It("leaves Liquid receiving *ordered.Map with insertion order intact", func() {
			// Guards against hoisting the conversion into the shared context
			// builder, which would silently flatten Liquid's ordering.
			//
			// The data is routed through BuildTemplateContext().ToMap() — the
			// same path the pipeline uses — rather than being handed to Liquid
			// directly. A test that constructed the context inline would still
			// pass after such a hoist and would be no guard at all. The
			// loader-level half of this is already covered by the #453 guard in
			// internal/data/loader_test.go ("JSON LoadFileAny returns
			// *ordered.Map"); this covers the context-building half.
			page := &content.Page{
				RelPath:     "index.md",
				URL:         "/",
				FrontMatter: map[string]interface{}{"title": "Home"},
			}
			siteData := map[string]interface{}{
				"title": "Test Site",
				"data": map[string]interface{}{
					"nav": fromJSON(`{"zebra": "z", "apple": "a", "middle": "m"}`),
				},
			}
			ctx := tmpl.BuildTemplateContext(
				page, siteData, []*content.Page{page}, nil, nil, nil, "").ToMap()

			liquid := tmpl.NewLiquidEngine()
			tpl, err := liquid.Parse("l", []byte(
				`{% for kv in site.data.nav %}{{ kv[0] }};{% endfor %}`))
			Expect(err).NotTo(HaveOccurred())

			out, err := tpl.Render(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("zebra;apple;middle;"),
				"Liquid must still iterate in insertion order — if this goes "+
					"alphabetical, the conversion was hoisted out of the Go engine "+
					"into the shared context builder")
		})
	})

	Context("Boundaries this change does not move", func() {
		It("still requires index for keys that are not template identifiers", func() {
			// {{ .d.my-key }} is a PARSE error in Go templates ("bad character
			// U+002D"), not a render error, and is equally true of plain maps
			// today. Out of scope — must not be "fixed" by rewriting source.
			_, err := engine.Parse("dash", []byte(`{{ .d.my-key }}`))
			Expect(err).To(HaveOccurred(),
				"a dashed key in dot position must remain a parse error — this "+
					"is Go template syntax, not an ordered-map limitation")

			ctx := map[string]interface{}{"d": fromJSON(`{"my-key": "dash"}`)}
			Expect(render(`{{ index .d "my-key" }}`, ctx)).To(Equal("dash"),
				"index remains the documented way to reach such keys, and must "+
					"work on converted data")
		})
	})
})
