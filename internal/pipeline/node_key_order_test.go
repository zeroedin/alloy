package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/plugin"
)

// ── Node plugins keep the key order of objects (issue #1374) ────────────
// An object a Node plugin gives back keeps its keys in the order JavaScript
// gives them, as on QuickJS (issue #571):
//   - from a hook: data that passes through unchanged keeps file order, and
//     an object the hook builds keeps the order the hook set its keys in;
//   - from a data source (alloy.source): site.data.<as> keeps the order the
//     source returned;
//   - from a filter: a returned object keeps its order. Its own size and
//     first keys still win, as for any Node filter result (issue #1366).
//
// Before, each was decoded into a Go map: hooks and sources gave a random
// order, different on each build, and filters gave sorted order.
//
// The objects have 12 keys, in no sorted order. A Go map of 8 keys or
// fewer comes back as its keys in insertion order, rotated by a random
// amount, which matches 1 time in 8; with 12 keys a lost order fails.
//
// See PLAN.md → "Hook Serialization Boundary — Type Preservation".
var _ = Describe("Node plugins keep the key order of objects (issue #1374)", func() {
	const (
		obj   = `{"zeta":1,"alpha":2,"mid":3,"beta":4,"omega":5,"gamma":6,"delta":7,"kappa":8,"iota":9,"eta":10,"theta":11,"lambda":12}`
		order = "zeta;alpha;mid;beta;omega;gamma;delta;kappa;iota;eta;theta;lambda;"
		node  = "export const runtime = \"node\";\n"
	)

	// loop prints the keys of the object at expr, in loop order.
	loop := func(expr string) string {
		return "{% for p in " + expr + " %}{{ p[0] }};{% endfor %}"
	}

	build := func(cfg *config.Config, files map[string]string) string {
		GinkgoHelper()
		files["content/index.md"] = "---\ntitle: Home\nlayout: default\n---\nhi"
		result, err := pipeline.BuildWithContent(cfg, files, pipeline.BuildOptions{CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		return result.RenderedContent["index.md"]
	}

	newConfig := func() *config.Config {
		return &config.Config{
			Title:   "Node Key Order",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
	}

	// A hook that passes site data through, replaces the data file
	// made.json with an object it builds, and adds a new key with another.
	const hookPlugin = "export default function(alloy) {\n" +
		"  alloy.hook('onDataFetched', { data: [\"*\"] }, (data) => {\n" +
		"    data.made = " + obj + ";\n" +
		"    data.added = " + obj + ";\n" +
		"    return data;\n" +
		"  });\n" +
		"}\n"

	hookFiles := func(runtimeLine string) map[string]string {
		return map[string]string{
			"data/tokens.json": obj,
			"data/nest.json":   `{"outer":` + obj + `}`,
			"data/made.json":   `{"placeholder":0}`,
			"plugins/hook.js":  runtimeLine + hookPlugin,
			"layouts/default.liquid": "HOOK[" + loop("site.data.tokens") + "] " +
				"NEST[" + loop("site.data.nest.outer") + "] " +
				"MADE[" + loop("site.data.made") + "] " +
				"ADDED[" + loop("site.data.added") + "] " +
				"FIRST[{{ site.data.tokens.first[0] }}] " +
				"SIZE[{{ site.data.tokens.size }}] " +
				"VALUES[{{ site.data.tokens.zeta }},{{ site.data.nest.outer.lambda }}] " +
				"{{ content }}",
		}
	}

	Describe("through a Node hook", Ordered, ContinueOnFailure, func() {
		var out string
		BeforeAll(func() {
			if plugin.CheckNodeAvailable() != nil {
				Skip("node is not installed")
			}
			out = build(newConfig(), hookFiles(node))
		})

		It("keeps file order for a data file the hook passes through", func() {
			Expect(out).To(ContainSubstring("HOOK[" + order + "]"))
		})
		It("keeps file order for an object nested in a data file", func() {
			Expect(out).To(ContainSubstring("NEST[" + order + "]"))
		})
		It("keeps the order the hook set for an object it builds", func() {
			Expect(out).To(ContainSubstring("MADE[" + order + "]"))
		})
		It("keeps the order the hook set for an object under a new key", func() {
			Expect(out).To(ContainSubstring("ADDED[" + order + "]"))
		})
		It("gives the first key in file order for first", func() {
			Expect(out).To(ContainSubstring("FIRST[zeta]"))
		})

		// Guards: these work today and must keep working.
		It("still gives the number of keys for size (guard)", func() {
			Expect(out).To(ContainSubstring("SIZE[12]"))
		})
		It("still gives each key's value (guard)", func() {
			Expect(out).To(ContainSubstring("VALUES[1,12]"))
		})
	})

	// The same hook on QuickJS, which keeps order today (issue #571).
	Describe("through a QuickJS hook (guards)", Ordered, ContinueOnFailure, func() {
		var out string
		BeforeAll(func() {
			out = build(newConfig(), hookFiles(""))
		})

		It("keeps file order for a data file and an object nested in it", func() {
			Expect(out).To(ContainSubstring("HOOK[" + order + "]"))
			Expect(out).To(ContainSubstring("NEST[" + order + "]"))
		})
		It("keeps the order the hook set for objects it builds", func() {
			Expect(out).To(ContainSubstring("MADE[" + order + "]"))
			Expect(out).To(ContainSubstring("ADDED[" + order + "]"))
		})
		It("gives the first key, the number of keys and each value", func() {
			Expect(out).To(ContainSubstring("FIRST[zeta]"))
			Expect(out).To(ContainSubstring("SIZE[12]"))
			Expect(out).To(ContainSubstring("VALUES[1,12]"))
		})
	})

	// No hook here, so each result reaches the template only through the
	// source or the filter.
	Describe("from a Node data source or filter", Ordered, ContinueOnFailure, func() {
		var out string
		BeforeAll(func() {
			if plugin.CheckNodeAvailable() != nil {
				Skip("node is not installed")
			}
			DeferCleanup(fetch.ResetPluginSources)
			cfg := newConfig()
			cfg.Sources = map[string]*config.SourceConfig{
				"one":  {Type: "plugin", Plugin: "one-object", As: "one"},
				"list": {Type: "plugin", Plugin: "object-list", As: "list"},
			}
			out = build(cfg, map[string]string{
				"plugins/values.js": node + "export default function(alloy) {\n" +
					"  alloy.source('one-object', async () => (" + obj + "));\n" +
					"  alloy.source('object-list', async () => ([" + obj + ", " + obj + "]));\n" +
					"  alloy.filter('make_object', () => (" + obj + "));\n" +
					"  alloy.filter('own_keys', () => ({size: 'own', first: 'mine', zeta: 1}));\n" +
					"}\n",
				"layouts/default.liquid": "SOURCE[" + loop("site.data.one") + "] " +
					"LIST[" + loop("site.data.list[1]") + "] " +
					`{% assign o = "x" | make_object %}FILTER[` + loop("o") + "] " +
					`{% assign own = "x" | own_keys %}OWN[{{ own.size }},{{ own.first }}] ` +
					"{{ content }}",
			})
		})

		It("keeps the order a source returned for an object", func() {
			Expect(out).To(ContainSubstring("SOURCE[" + order + "]"))
		})
		It("keeps the order a source returned for each object in a list", func() {
			Expect(out).To(ContainSubstring("LIST[" + order + "]"))
		})
		It("keeps the order of an object a filter returns", func() {
			Expect(out).To(ContainSubstring("FILTER[" + order + "]"))
		})

		// Guard: a Node filter result's own keys win today (issue #1366),
		// and keeping order must not change that.
		It("still gives a filter result's own size and first keys (guard)", func() {
			Expect(out).To(ContainSubstring("OWN[own,mine]"))
		})
	})
})
