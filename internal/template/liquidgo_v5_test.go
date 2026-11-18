package template_test

import (
	"fmt"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
	"github.com/zeroedin/alloy/internal/template"
)

// ── Alloy runs on liquidgo v5 (issue #1363) ────────────────────────────
// Alloy's Liquid library fork was based on upstream from January 2026.
// Upstream has since moved to a v5 module with a new API, ported Shopify
// Liquid 5.14, corrected many results, and stopped joining a template's
// output with +=, which made a for loop over a long list very slow.
//
// These tests go through Alloy's own engine, so they don't depend on the
// library's API. The first two groups fail on the old fork and pass after
// the port. The guards pass on both and must keep passing.
//
// See PLAN.md → "Liquid library: liquidgo v5 (issue #1363)".

var _ = Describe("Alloy runs on liquidgo v5 (issue #1363)", func() {
	newEngine := func() template.TemplateEngine {
		GinkgoHelper()
		engine := template.NewLiquidEngine()
		Expect(template.RegisterBuiltinFilters(engine)).To(Succeed())
		return engine
	}

	render := func(source string, ctx map[string]interface{}) string {
		GinkgoHelper()
		tpl, err := newEngine().Parse("test", []byte(source))
		Expect(err).NotTo(HaveOccurred())
		out, err := tpl.Render(ctx)
		Expect(err).NotTo(HaveOccurred())
		return string(out)
	}

	// ── Results the update corrects ─────────────────────────────────
	DescribeTable("gives the results Shopify's Liquid gives",
		func(source string, ctx map[string]interface{}, expected string) {
			Expect(render(source, ctx)).To(Equal(expected))
		},
		Entry("decimal arithmetic is exact",
			`{{ 0.1 | plus: 0.2 }}`, nil, "0.3"),
		Entry("dividing two integers gives an integer",
			`{{ 14 | divided_by: 3 }}`, nil, "4"),
		Entry("size counts characters, not bytes",
			`{{ "héllo" | size }}`, nil, "5"),
		Entry("a number from JSON equals the same integer",
			`{% if n == 100 %}equal{% else %}different{% endif %}`,
			map[string]interface{}{"n": float64(100)}, "equal"),
	)

	// ── A for loop costs in step with its output ────────────────────
	// The old library joined output with +=, which copies everything
	// written so far on every addition. A loop four times as long then
	// allocated about sixteen times the memory. This counts bytes
	// allocated, not time, so it gives the same answer on every machine.
	Describe("a for loop over a long list", func() {
		loopBytes := func(items int) uint64 {
			GinkgoHelper()
			list := make([]interface{}, items)
			for i := range list {
				list[i] = fmt.Sprintf("item-%04d", i)
			}
			tpl, err := newEngine().Parse("loop", []byte(
				`{% for i in list %}<a href="/{{ i }}/">{{ i }}</a>{% endfor %}`))
			Expect(err).NotTo(HaveOccurred())
			ctx := map[string]interface{}{"list": list}

			var before, after runtime.MemStats
			runtime.GC()
			runtime.ReadMemStats(&before)
			out, err := tpl.Render(ctx)
			runtime.ReadMemStats(&after)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(ContainSubstring(fmt.Sprintf("item-%04d", items-1)),
				"sanity: the loop rendered every item")
			return after.TotalAlloc - before.TotalAlloc
		}

		It("allocates in step with the length of the list", func() {
			const short, long = 500, 2000 // long is 4 times short
			small := loopBytes(short)
			large := loopBytes(long)

			Expect(float64(large)/float64(small)).To(BeNumerically("<", 6.0),
				"a list 4 times as long must not allocate much more than 4 times "+
					"the bytes (%d items: %d bytes, %d items: %d bytes). A ratio "+
					"near 16 means output is still joined with += inside the loop",
				short, small, long, large)
		})
	})

	// ── Guards: what must not change ────────────────────────────────
	Describe("keeps what templates rely on (guards)", func() {
		It("fails on markup that doesn't parse", func() {
			tpl, err := newEngine().Parse("bad", []byte(`{{ | broken_syntax }}`))
			if err == nil {
				_, err = tpl.Render(map[string]interface{}{})
			}
			Expect(err).To(HaveOccurred(),
				"malformed markup must fail the build, not render as nothing; "+
					"liquidgo v5's default lax mode lets it through")
		})

		It("reads a data file object by key, size and first, in file order", func() {
			nav, err := ordered.UnmarshalJSONValue([]byte(`{"zebra": "z", "apple": "a", "middle": "m"}`))
			Expect(err).NotTo(HaveOccurred())
			out := render(
				`KEY[{{ nav.apple }}] SIZE[{{ nav.size }}] FIRST[{{ nav.first[0] }}={{ nav.first[1] }}] `+
					`ORDER[{% for pair in nav %}{{ pair[0] }};{% endfor %}] MISSING[{{ nav.nope }}]`,
				map[string]interface{}{"nav": nav})
			Expect(out).To(ContainSubstring("KEY[a]"))
			Expect(out).To(ContainSubstring("SIZE[3]"))
			Expect(out).To(ContainSubstring("FIRST[zebra=z]"))
			Expect(out).To(ContainSubstring("ORDER[zebra;apple;middle;]"))
			Expect(out).To(ContainSubstring("MISSING[]"))
		})

		It("uses one shortcode inline in one template and as a block in another", func() {
			engine := newEngine()
			Expect(engine.AddTag("box", func(args []string, content string) string {
				return fmt.Sprintf("<box %v>%s</box>", args, content)
			})).To(Succeed())

			inline, err := engine.Parse("inline", []byte(`{% box "a" %}`))
			Expect(err).NotTo(HaveOccurred())
			block, err := engine.Parse("block", []byte(`{% box "b" %}body{% endbox %}`))
			Expect(err).NotTo(HaveOccurred())
			again, err := engine.Parse("inline-again", []byte(`{% box "c" %} after`))
			Expect(err).NotTo(HaveOccurred())

			out, err := inline.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("<box [a]></box>"))
			out, err = block.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("<box [b]>body</box>"))
			out, err = again.Render(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(out)).To(Equal("<box [c]></box> after"),
				"parsing a block use must not turn later inline uses into blocks")
		})
	})
})
