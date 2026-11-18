package template_test

import (
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/template"
)

// ── Shortcodes next to raw, comment and doc (issue #1373) ───────────────
// Before a template is parsed, Alloy adds an {% endname %} after each
// inline use of a shortcode, so that every use parses as a block (issue
// #1367). It must leave {% raw %}, {% comment %} and {% doc %} regions as
// they are: a shortcode inside one is text, not a use.
//
// The cases below broke during review of #1372 and were fixed there, with
// no test. Each gives the output main gave before #1372.
//
// See PLAN.md → "Layouts, Partials, Shortcodes".
var _ = Describe("Shortcodes next to raw, comment and doc (issue #1373)", func() {
	// newEngine returns an engine with the shortcodes box and card, which
	// print <box [args]>body</box> and <card [args]>body</card>.
	newEngine := func() template.TemplateEngine {
		GinkgoHelper()
		engine := template.NewLiquidEngine()
		Expect(template.RegisterBuiltinFilters(engine)).To(Succeed())
		for _, name := range []string{"box", "card"} {
			name := name
			Expect(engine.AddTag(name, func(args []string, content string) string {
				return fmt.Sprintf("<%s %v>%s</%s>", name, args, content, name)
			})).To(Succeed())
		}
		return engine
	}

	check := func(src, want string) {
		GinkgoHelper()
		tpl, err := newEngine().Parse("page", []byte(src))
		Expect(err).NotTo(HaveOccurred())
		out, err := tpl.Render(map[string]interface{}{})
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(Equal(want))
	}

	DescribeTable("a shortcode inside raw, comment or doc is text, not a use", check,
		Entry("raw",
			`{% raw %}{% box "r" %}{% endraw %}|{% box "a" %}`,
			`{% box "r" %}|<box [a]></box>`),
		Entry("comment",
			`{% comment %}{% box "r" %}{% endcomment %}|{% box "a" %}`,
			`|<box [a]></box>`),
		Entry("doc",
			`{% doc %}{% box "r" %}{% enddoc %}|{% box "a" %}`,
			`|<box [a]></box>`),
	)

	DescribeTable("templates that broke during review of #1372 give main's output", check,
		Entry("a raw region inside a block holds a use of the same shortcode",
			`{% box "a" %}{% raw %}{% box "r" %}{% endraw %}x{% endbox %}`,
			`<box [a]>{% raw %}{% box "r" %}{% endraw %}x</box>`),
		Entry("a raw region holds an unfinished tag",
			`{% raw %}{% unfinished {% endraw %}{% box "a" %}`,
			`{% unfinished <box [a]></box>`),
		Entry("a use inside {% liquid %}",
			"{% liquid\nbox \"a\"\n%}",
			`<box [a]></box>`),
		Entry("a block's end tag after a raw with no endraw",
			`{% box "a" %}{% raw %}{% endbox %}`,
			`<box [a]>{% raw %}</box>`),
		Entry(`an argument holds "{%"`,
			`{% box "{%" %}|after`,
			`<box [{%]></box>|after`),
		Entry("a raw region after a block whose body opens a raw",
			`{% box "a" %}{% raw %}{% endbox %}{% raw %}text{% endraw %}`,
			`<box [a]>{% raw %}</box>text`),
		Entry("an endraw with text after its name",
			`{% raw %}{% box "r" %}{% endraw ignored %}|{% box "a" %}`,
			`{% box "r" %}|<box [a]></box>`),
		Entry("a block's body holds nested comments with a use in them",
			`{% box "a" %}{% comment %}{% comment %}{% endcomment %}{% box "b" %}{% endcomment %}body{% endbox %}`,
			`<box [a]>{% comment %}{% comment %}{% endcomment %}{% box "b" %}{% endcomment %}body</box>`),
		Entry("nested comments hold a use, then a raw holds its end tag",
			`{% comment %}{% comment %}{% endcomment %}{% box "r" %}{% endcomment %}{% card "c" %}{% raw %}{% endbox %}{% endraw %}`,
			`<card [c]></card>{% endbox %}`),
	)

	// Three earlier versions of this step took time that grew with the
	// square of the number of tags: from 3.4s to 36.6s on templates like
	// these, which now parse in a few milliseconds. Each test parses a
	// template and one with 4 times as many tags. Parse time that grows in
	// step with the tags takes about 4 times as long; time that grows with
	// the square takes about 16 times as long. Comparing the two parses
	// gives the same answer on a fast machine, a slow one, and under the
	// race detector.
	Describe("parse time grows in step with the number of tags", func() {
		const (
			size = 500
			// 4 is exact linear growth; measured 3.4 to 4.4. The slow
			// versions measured 14 to 18.
			maxGrowth = 8.0
		)

		// parseTime returns how long one parse of src takes. Some of these
		// templates don't parse: only the time matters.
		parseTime := func(src string) time.Duration {
			engine := newEngine()
			start := time.Now()
			_, _ = engine.Parse("page", []byte(src))
			return time.Since(start)
		}

		// The small and large templates are parsed in pairs, one right
		// after the other, so that a busy moment on the machine slows both
		// parses of a pair, not only the large one. The lowest ratio of five
		// pairs counts: parse time that grows with the square of the number
		// of tags gives a high ratio in every pair.
		DescribeTable("a template with 4 times the tags takes at most 8 times as long",
			func(build func(n int) string) {
				small, large := build(size), build(4*size)
				best := 0.0
				var bestSmall, bestLarge time.Duration
				for range 5 {
					s := parseTime(small)
					l := parseTime(large)
					if ratio := float64(l) / float64(s); best == 0 || ratio < best {
						best, bestSmall, bestLarge = ratio, s, l
					}
				}
				Expect(best).To(BeNumerically("<", maxGrowth),
					"%d tags: %v, %d tags: %v. A higher ratio means parse time grows "+
						"with the square of the number of tags", size, bestSmall, 4*size, bestLarge)
			},
			Entry("raw tags, then one endraw", func(n int) string {
				return strings.Repeat("{% raw %}", n) + `{% endraw %}{% box "a" %}`
			}),
			Entry("raw tags with no endraw", func(n int) string {
				return strings.Repeat("{% raw %}", n) + `{% box "a" %}`
			}),
			Entry("a comment holding comment and raw pairs, then an endraw, then uses", func(n int) string {
				return "{% comment %}" + strings.Repeat("{% comment %}{% raw %}", n) +
					"{% endraw %}" + strings.Repeat(`{% box "a" %}`, n)
			}),
		)
	})
})
