package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/plugin"
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
	//   pair  - an object with two keys, b then a (not sorted order)
	//   meta  - an object with its own size and first keys
	//   items - an array
	//   dbl   - a number times two
	const filters = `export default function(alloy) {
  alloy.filter("seen", (v) => typeof v + ":" + JSON.stringify(v));
  alloy.filter("keys", (o) => Object.keys(o ?? {}));
  alloy.filter("pair", (s) => ({ b: s + "2", a: s + "1" }));
  alloy.filter("meta", (s) => ({ size: "large", first: "primary" }));
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
		`IN-BIG-NUMBER[{{ 3000000000 | seen }}]` +
		`IN-DATE[{{ page.date | seen }}]` +
		`KEYS[{{ site.data.nav | keys | join: "," }}]` +
		`KEYS-SIZE[{{ site.data.nav | keys | size }}]` +
		`KEYS-FIRST[{{ site.data.nav | keys | first }}]` +
		`{% assign o = "x" | pair %}OUT-OBJECT[{{ o.a }}|{{ o.b }}]` +
		`OUT-OBJECT-LOOP[{% for kv in o %}{{ kv[0] }};{% endfor %}]` +
		`{% assign m = "x" | meta %}OUT-OWN-KEYS[{{ m.size }}|{{ m.first }}]` +
		`{% assign l = "y" | items %}OUT-LIST[{{ l.size }}|{{ l.first }}|{{ l | join: "," }}]` +
		`OUT-NUMBER[{{ 3 | dbl }}]`

	// buildWith builds a one-page site with the probe filters, on the
	// runtime runtimeLine names, and returns the page. Each build starts the
	// plugin in a new runtime.
	buildWith := func(runtimeLine, layout string) string {
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
	build := func(runtimeLine string) string {
		GinkgoHelper()
		return buildWith(runtimeLine, layout)
	}

	// The same checks for both runtimes. All pass on both, except that on
	// Node a returned object's keys come back sorted until #1374 is fixed.
	for _, rt := range []struct{ name, line string }{
		{"QuickJS, the default", ""},
		{"Node", "export const runtime = \"node\";\n"},
	} {
		rt := rt
		Describe("on "+rt.name, Ordered, ContinueOnFailure, func() {
			var out string
			BeforeAll(func() {
				// Node is optional for running the tests (README).
				if rt.line != "" && plugin.CheckNodeAvailable() != nil {
					Skip("node is not installed")
				}
				out = build(rt.line)
			})

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
			// The filter writes b before a, so a sorted decoder fails this.
			// Node decodes a filter's result into a Go map today, and Liquid
			// loops over one in sorted order (issue #1374).
			It("returns an object whose keys keep their order", func() {
				Expect(out).To(ContainSubstring(`OUT-OBJECT-LOOP[b;a;]`))
			})
			It("returns an object whose own size and first keys win", func() {
				Expect(out).To(ContainSubstring(`OUT-OWN-KEYS[large|primary]`))
			})
			It("receives an integer too big for 32 bits as it is", func() {
				Expect(out).To(ContainSubstring(`IN-BIG-NUMBER[number:3000000000]`))
			})

			// Guards: these worked on both runtimes before #1366.
			It("still receives strings and numbers as they are (guard)", func() {
				Expect(out).To(ContainSubstring(`IN-STRING[string:"hi"]`))
				Expect(out).To(ContainSubstring(`IN-NUMBER[number:7]`))
				Expect(out).To(ContainSubstring(`OUT-NUMBER[6]`))
			})
		})
	}

	// The first filter call a runtime makes can return an array or object
	// too (issue #1382). The layout above calls a filter that returns text
	// first, so it can't show a fault that only the first call has, such as
	// storing the result in a global that doesn't exist yet. Each build here
	// starts the plugin in a new runtime, and the layout's first filter call
	// returns an array or object. A filter that returns text comes next, so
	// a result kept from the call before shows.
	for _, rt := range []struct{ name, line string }{
		{"QuickJS, the default", ""},
		{"Node", "export const runtime = \"node\";\n"},
	} {
		rt := rt
		Describe("on "+rt.name+", when the first filter call returns an array or object (issue #1382)", func() {
			BeforeEach(func() {
				if rt.line != "" && plugin.CheckNodeAvailable() != nil {
					Skip("node is not installed")
				}
			})

			It("returns an object from the first call, then text from the next", func() {
				out := buildWith(rt.line,
					`{% assign o = "x" | pair %}FIRST-OBJECT[{{ o.a }}|{{ o.b }}]`+
						`THEN-TEXT[{{ "hi" | seen }}]`)
				Expect(out).To(ContainSubstring(`FIRST-OBJECT[x1|x2]`))
				Expect(out).To(ContainSubstring(`THEN-TEXT[string:"hi"]`),
					"a filter that returns text gives its text, not the object from the call before")
			})

			It("returns an array from the first call, then text from the next", func() {
				out := buildWith(rt.line,
					`FIRST-LIST[{{ "y" | items | join: "," }}]`+
						`THEN-TEXT[{{ "hi" | seen }}]`)
				Expect(out).To(ContainSubstring(`FIRST-LIST[y,yy,3]`))
				Expect(out).To(ContainSubstring(`THEN-TEXT[string:"hi"]`),
					"a filter that returns text gives its text, not the array from the call before")
			})
		})
	}
})
