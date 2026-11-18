package template_test

import (
	"fmt"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/template"
)

// ── Shortcodes in partials (issue #1368) ───────────────────────────────
// A plugin shortcode is inline or a block depending on whether the template
// closes it with {% endname %}. A partial loaded by {% render %} or
// {% include %} decides that from its own source, like any other template.
// It doesn't follow the page that includes it.
//
// See PLAN.md → "Layouts, Partials, Shortcodes".
var _ = Describe("Shortcodes in partials (issue #1368)", func() {
	// renderWithPartial renders page with the partial "card" holding partial,
	// and a plugin shortcode "box" that wraps its content.
	renderWithPartial := func(page, partial string) (string, error) {
		GinkgoHelper()
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "card.liquid"), []byte(partial), 0o644)).To(Succeed())
		engine := template.NewLiquidEngine()
		Expect(template.RegisterBuiltinFilters(engine)).To(Succeed())
		setter, ok := engine.(interface{ SetIncludesDir(string) })
		Expect(ok).To(BeTrue(), "engine must implement SetIncludesDir")
		setter.SetIncludesDir(dir)
		Expect(engine.AddTag("box", func(args []string, content string) string {
			return fmt.Sprintf("<box %v>%s</box>", args, content)
		})).To(Succeed())
		tpl, err := engine.Parse("page", []byte(page))
		if err != nil {
			return "", err
		}
		out, err := tpl.Render(map[string]interface{}{})
		return string(out), err
	}

	check := func(page, partial, want string) {
		out, err := renderWithPartial(page, partial)
		Expect(err).NotTo(HaveOccurred())
		Expect(out).To(Equal(want))
	}

	DescribeTable("a partial uses a shortcode in a different form from the page", check,
		Entry("page inline, partial block, with render",
			`{% box "p" %}|{% render "card" %}`, `{% box "c" %}body{% endbox %}`,
			"<box [p]></box>|<box [c]>body</box>"),
		Entry("page inline, partial block, with include",
			`{% box "p" %}|{% include "card" %}`, `{% box "c" %}body{% endbox %}`,
			"<box [p]></box>|<box [c]>body</box>"),
		Entry("page block, partial inline, with render",
			`{% box "p" %}x{% endbox %}|{% render "card" %}`, `{% box "c" %} after`,
			"<box [p]>x</box>|<box [c]></box> after"),
		Entry("page block, partial inline, with include",
			`{% box "p" %}x{% endbox %}|{% include "card" %}`, `{% box "c" %} after`,
			"<box [p]>x</box>|<box [c]></box> after"),
		Entry("page without the shortcode, partial block",
			`[{% render "card" %}]`, `{% box "c" %}body{% endbox %}`,
			"[<box [c]>body</box>]"),
	)

	// Guards: these work today and must keep working.
	DescribeTable("a partial uses a shortcode in the same form as the page (guards)", check,
		Entry("both inline, with render",
			`{% box "p" %}|{% render "card" %}`, `{% box "c" %} after`,
			"<box [p]></box>|<box [c]></box> after"),
		Entry("both block, with include",
			`{% box "p" %}x{% endbox %}|{% include "card" %}`, `{% box "c" %}body{% endbox %}`,
			"<box [p]>x</box>|<box [c]>body</box>"),
		Entry("page without the shortcode, partial inline",
			`[{% render "card" %}]`, `{% box "c" %}`,
			"[<box [c]></box>]"),
	)
})
