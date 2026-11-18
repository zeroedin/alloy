package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
)

// ── Plugin filters pass lists and objects as values (issue #1366) ──────
// A plugin filter receives its input and returns its result as JavaScript
// values: a list arrives as an array, an object (a map, a data file object,
// front matter) as an object, and a returned array or object comes back to
// the template as a list or an object. Data file objects keep their keys in
// file order, both ways.
//
// Node filters already work this way: their input and result cross as JSON.
// QuickJS filters, the default runtime, received anything that wasn't a
// string, number or boolean as Go's debug text (`[a b]`,
// `&{[zebra apple] map[...]}`), and an array or object they returned came
// back as text. So a filter that works on Node gave wrong output on QuickJS,
// with no error. QuickJS filters now match Node filters.
//
// See PLAN.md → "Plugin filters receive and return values (issue #1366)".

var _ = Describe("Plugin filters pass lists and objects as values (issue #1366)", func() {
	// The probe filters, written once for both runtimes:
	//   seen  - shows the type and JSON of what the filter received
	//   keys  - the object's keys, as an array
	//   pair  - an object with two keys, a then b
	//   items - an array
	//   dbl   - a number times two
	const filters = `export default function(alloy) {
  alloy.filter("seen", (v) => typeof v + ":" + JSON.stringify(v));
  alloy.filter("keys", (o) => Object.keys(o ?? {}));
  alloy.filter("pair", (s) => ({ a: s + "1", b: s + "2" }));
  alloy.filter("items", (s) => [s, s + s, 3]);
  alloy.filter("dbl", (n) => n * 2);
}`

	// Each probe prints one marker, so each It checks one behavior.
	const layout = `{{ content }}` +
		`IN-OBJECT[{{ site.data.nav | seen }}]` +
		`IN-LIST[{{ site.data.list | seen }}]` +
		`IN-FRONTMATTER[{{ page.tags | seen }}]` +
		`IN-STRING[{{ "hi" | seen }}]` +
		`IN-NUMBER[{{ 7 | seen }}]` +
		`IN-DATE[{{ page.date | seen }}]` +
		`KEYS[{{ site.data.nav | keys | join: "," }}]` +
		`KEYS-SIZE[{{ site.data.nav | keys | size }}]` +
		`KEYS-FIRST[{{ site.data.nav | keys | first }}]` +
		`{% assign o = "x" | pair %}OUT-OBJECT[{{ o.a }}|{{ o.b }}]` +
		`OUT-OBJECT-LOOP[{% for kv in o %}{{ kv[0] }};{% endfor %}]` +
		`{% assign l = "y" | items %}OUT-LIST[{{ l.size }}|{{ l.first }}|{{ l | join: "," }}]` +
		`OUT-NUMBER[{{ 3 | dbl }}]`

	build := func(runtimeLine string) string {
		GinkgoHelper()
		cfg := &config.Config{
			Title:   "Filter values",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		result, err := pipeline.BuildWithContent(cfg, map[string]string{
			"data/nav.json":          `{"zebra": "z", "apple": "a", "middle": "m"}`,
			"data/list.json":         `[1, "two", {"k": "v"}]`,
			"content/index.md":       "---\ntitle: Home\nlayout: default\ntags: [x, y]\ndate: 2024-01-02\n---\n# Home",
			"layouts/default.liquid": layout,
			"plugins/filters.js":     runtimeLine + filters,
		})
		Expect(err).NotTo(HaveOccurred())
		return result.RenderedContent["index.md"]
	}

	// The same checks for both runtimes. On Node they pass today and are
	// guards; on QuickJS they fail until the fix.
	for _, rt := range []struct{ name, line string }{
		{"QuickJS, the default", ""},
		{"Node", "export const runtime = \"node\";\n"},
	} {
		rt := rt
		Describe("on "+rt.name, Ordered, ContinueOnFailure, func() {
			var out string
			BeforeAll(func() { out = build(rt.line) })

			It("receives a data file object as an object, keys in file order", func() {
				Expect(out).To(ContainSubstring(`IN-OBJECT[object:{"zebra":"z","apple":"a","middle":"m"}]`))
			})
			It("receives a list as an array", func() {
				Expect(out).To(ContainSubstring(`IN-LIST[object:[1,"two",{"k":"v"}]]`))
			})
			It("receives a front matter list as an array", func() {
				Expect(out).To(ContainSubstring(`IN-FRONTMATTER[object:["x","y"]]`))
			})
			It("receives a date as text in RFC 3339 form", func() {
				Expect(out).To(ContainSubstring(`IN-DATE[string:"2024-01-02T00:00:00Z"]`))
			})
			It("returns an array the template can join, count and index", func() {
				Expect(out).To(ContainSubstring(`KEYS[zebra,apple,middle]`))
				Expect(out).To(ContainSubstring(`KEYS-SIZE[3]`))
				Expect(out).To(ContainSubstring(`KEYS-FIRST[zebra]`))
				Expect(out).To(ContainSubstring(`OUT-LIST[3|y|y,yy,3]`))
			})
			It("returns an object the template can read by key", func() {
				Expect(out).To(ContainSubstring(`OUT-OBJECT[x1|x2]`))
			})

			// Guards: these work on both runtimes today.
			It("still receives strings and numbers as they are (guard)", func() {
				Expect(out).To(ContainSubstring(`IN-STRING[string:"hi"]`))
				Expect(out).To(ContainSubstring(`IN-NUMBER[number:7]`))
				Expect(out).To(ContainSubstring(`OUT-NUMBER[6]`))
			})
		})
	}

	// Key order of a returned object is QuickJS's alone for now: a Node
	// filter's result is decoded into a Go map, which has no order (#1374).
	It("returns an object whose keys keep their order on QuickJS", func() {
		Expect(build("")).To(ContainSubstring(`OUT-OBJECT-LOOP[a;b;]`))
	})
})
