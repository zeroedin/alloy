package pipeline_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/config"
	"github.com/zeroedin/alloy/internal/fetch"
	"github.com/zeroedin/alloy/internal/pipeline"
	"github.com/zeroedin/alloy/internal/plugin"
)

// ── onAfterValidation keeps key order (issue #1401) ─────────────────────
// What onAfterValidation returns in cascade is written back into site.data
// with every object's keys in order, as onDataFetched's result is:
//   - a data file's object keeps file order, at every depth, including
//     objects in a list;
//   - a source's object keeps the order the source returned;
//   - an object the hook builds keeps the order the hook set its keys in.
//
// Before, the cascade was read as Go maps, so every object looped in sorted
// order after the hook, even when the hook returned it unchanged.
//
// The objects have 12 keys, in no sorted order, so a lost order fails
// whether it comes back sorted or random (see issue #1374's tests).
//
// See PLAN.md → "onAfterValidation keeps key order too".
var _ = Describe("onAfterValidation keeps key order (issue #1401)", func() {
	const (
		obj   = `{"zeta":1,"alpha":2,"mid":3,"beta":4,"omega":5,"gamma":6,"delta":7,"kappa":8,"iota":9,"eta":10,"theta":11,"lambda":12}`
		order = "zeta;alpha;mid;beta;omega;gamma;delta;kappa;iota;eta;theta;lambda;"
		// built is the order the hook sets its object's keys in.
		built = "kappa;zeta;beta;lambda;alpha;theta;mid;iota;omega;eta;delta;gamma;"
		node  = "export const runtime = \"node\";\n"
	)

	// loop prints the keys of the object at expr, in loop order.
	loop := func(expr string) string {
		return "{% for p in " + expr + " %}{{ p[0] }};{% endfor %}"
	}

	layout := "FLAT[" + loop("site.data.flat") + "] " +
		"NESTED[" + loop("site.data.file.top") + "] " +
		"LIST[" + loop("site.data.file.list[1]") + "] " +
		"SOURCE[" + loop("site.data.src") + "] " +
		"BUILT[" + loop("site.data.built") + "] " +
		"VALUES[{{ site.data.flat.zeta }},{{ site.data.flat.lambda }},{{ site.data.file.list[1].mid }}] " +
		"{{ content }}"

	const hook = "export default function(alloy) {\n" +
		"  alloy.hook('onAfterValidation', { data: ['*'] }, (p) => ({ cascade: { ...p.cascade,\n" +
		"    built: { kappa: 1, zeta: 2, beta: 3, lambda: 4, alpha: 5, theta: 6, mid: 7, iota: 8, omega: 9, eta: 10, delta: 11, gamma: 12 } } }));\n" +
		"}\n"

	nodeAvailable := func() bool { return plugin.CheckNodeAvailable() == nil }
	needNode := func() {
		if !nodeAvailable() {
			Skip("node is not installed")
		}
	}

	// build runs a full build with hookSrc as the only hook plugin, or none.
	// A Node plugin source (as: src) is added only when Node is installed.
	build := func(hookSrc string) string {
		GinkgoHelper()
		fetch.ResetPluginSources()
		DeferCleanup(fetch.ResetPluginSources)
		cfg := &config.Config{
			Title:   "After Validation Key Order",
			BaseURL: "https://example.com",
			Build:   config.BuildConfig{Output: "_site"},
		}
		files := map[string]string{
			"content/index.md":       "---\ntitle: Home\nlayout: default\n---\nhi",
			"layouts/default.liquid": layout,
			"data/flat.json":         obj,
			"data/file.json":         `{"top":` + obj + `,"list":[` + obj + `,` + obj + `]}`,
		}
		if nodeAvailable() {
			cfg.Sources = map[string]*config.SourceConfig{
				"src": {Type: "plugin", Plugin: "ordered-source", As: "src"},
			}
			files["plugins/source.js"] = node + "export default function(alloy) {\n" +
				"  alloy.source('ordered-source', async () => (" + obj + "));\n" +
				"}\n"
		}
		if hookSrc != "" {
			files["plugins/hook.js"] = hookSrc
		}
		result, err := pipeline.BuildWithContent(cfg, files, pipeline.BuildOptions{CaptureRenderedContent: true})
		Expect(err).NotTo(HaveOccurred())
		return result.RenderedContent["index.md"]
	}

	// Guard: the order each object has with no hook. The hook must keep it.
	Describe("with no hook (guard)", Ordered, ContinueOnFailure, func() {
		var out string
		BeforeAll(func() { out = build("") })

		It("loops over a data file's object in file order", func() {
			Expect(out).To(ContainSubstring("FLAT[" + order + "]"))
			Expect(out).To(ContainSubstring("NESTED[" + order + "]"))
			Expect(out).To(ContainSubstring("LIST[" + order + "]"))
		})
		It("loops over a source's object in the order the source returned", func() {
			needNode()
			Expect(out).To(ContainSubstring("SOURCE[" + order + "]"))
		})
	})

	for _, runtime := range []string{"QuickJS", "Node"} {
		rt := runtime
		Describe("after an onAfterValidation hook on "+rt, Ordered, ContinueOnFailure, func() {
			var out string
			BeforeAll(func() {
				src := hook
				if rt == "Node" {
					needNode()
					src = node + src
				}
				out = build(src)
			})

			It("loops over a data file's object in file order", func() {
				Expect(out).To(ContainSubstring("FLAT[" + order + "]"))
			})
			It("loops over an object inside a data file in file order", func() {
				Expect(out).To(ContainSubstring("NESTED[" + order + "]"))
			})
			It("loops over an object in a data file's list in file order", func() {
				Expect(out).To(ContainSubstring("LIST[" + order + "]"))
			})
			It("loops over a source's object in the order the source returned", func() {
				needNode()
				Expect(out).To(ContainSubstring("SOURCE[" + order + "]"))
			})
			It("loops over an object the hook builds in the order the hook set", func() {
				Expect(out).To(ContainSubstring("BUILT[" + built + "]"))
			})
			// Guard: only the order changes. The values pass through as they are.
			It("keeps every value (guard)", func() {
				Expect(out).To(ContainSubstring("VALUES[1,12,3]"))
			})
		})
	}

	// Guard: onDataFetched already keeps order, and must go on doing so.
	Describe("after an onDataFetched hook (guard)", Ordered, ContinueOnFailure, func() {
		var out string
		BeforeAll(func() {
			out = build("export default function(alloy) {\n" +
				"  alloy.hook('onDataFetched', { data: ['*'] }, (d) => ({ ...d,\n" +
				"    built: { kappa: 1, zeta: 2, beta: 3, lambda: 4, alpha: 5, theta: 6, mid: 7, iota: 8, omega: 9, eta: 10, delta: 11, gamma: 12 } }));\n" +
				"}\n")
		})

		It("loops over data files' objects in file order", func() {
			Expect(out).To(ContainSubstring("FLAT[" + order + "]"))
			Expect(out).To(ContainSubstring("NESTED[" + order + "]"))
			Expect(out).To(ContainSubstring("LIST[" + order + "]"))
		})
		It("loops over an object the hook builds in the order the hook set", func() {
			Expect(out).To(ContainSubstring("BUILT[" + built + "]"))
		})
	})
})
