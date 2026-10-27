package template_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/zeroedin/alloy/internal/ordered"
	tmpl "github.com/zeroedin/alloy/internal/template"
)

// ── `json` filter indent argument (issue #1269) ──────────────────────
// `json` prints a value on one line. An optional argument indents it, with
// the same rules as the `space` argument of JavaScript's JSON.stringify:
//   - a string is the indent, cut to 10 characters; "" means no indent
//   - a number is that many spaces, cut toward zero and capped at 10;
//     below 1 means no indent
//   - any other argument is ignored
// With no indent the output is exactly what it is today.
//
// See PLAN.md → "`json` indent argument (issue #1269)".

// jsonIndentFixture is a nested data file as the loaders produce it:
// *ordered.Map objects in file order, which is not alphabetical.
func jsonIndentFixture() *ordered.Map {
	leaf := ordered.New()
	leaf.Set("y", true)
	leaf.Set("c", nil)
	products := ordered.New()
	products.Set("zeta", "/z/")
	products.Set("alpha", []interface{}{"one", leaf})
	products.Set("empty", []interface{}{})
	nav := ordered.New()
	nav.Set("home", "/")
	nav.Set("products", products)
	nav.Set("note", "<a & b>")
	return nav
}

const jsonIndentCompact = `{"home":"/","products":{"zeta":"/z/","alpha":["one",{"y":true,"c":null}],"empty":[]},"note":"\u003ca \u0026 b\u003e"}`

const jsonIndentTwo = `{
  "home": "/",
  "products": {
    "zeta": "/z/",
    "alpha": [
      "one",
      {
        "y": true,
        "c": null
      }
    ],
    "empty": []
  },
  "note": "\u003ca \u0026 b\u003e"
}`

const jsonIndentTab = "{\n\t\"home\": \"/\",\n\t\"products\": {\n\t\t\"zeta\": \"/z/\",\n\t\t\"alpha\": [\n\t\t\t\"one\",\n\t\t\t{\n\t\t\t\t\"y\": true,\n\t\t\t\t\"c\": null\n\t\t\t}\n\t\t],\n\t\t\"empty\": []\n\t},\n\t\"note\": \"\\u003ca \\u0026 b\\u003e\"\n}"

// withIndent re-indents jsonIndentTwo, one unit per two leading spaces.
func withIndent(unit string) string {
	lines := strings.Split(jsonIndentTwo, "\n")
	for i, line := range lines {
		body := strings.TrimLeft(line, " ")
		lines[i] = strings.Repeat(unit, (len(line)-len(body))/2) + body
	}
	return strings.Join(lines, "\n")
}

var _ = Describe("json filter indent argument (issue #1269)", func() {

	It("prints one line with no argument, exactly as before (guard)", func() {
		Expect(tmpl.JSONFilter(jsonIndentFixture())).To(Equal(jsonIndentCompact))
	})

	It("keeps HTML-escaping characters like < and & in strings (guard)", func() {
		// The indent changes layout only. Escaping <, > and & keeps the
		// output safe to print raw in a Liquid page.
		out := tmpl.JSONFilter(jsonIndentFixture(), "  ").(string)
		Expect(out).To(ContainSubstring(`"\u003ca \u0026 b\u003e"`))
		Expect(out).NotTo(ContainSubstring("<a & b>"))
	})

	DescribeTable("a string argument is the indent",
		func(arg string, expected string) {
			Expect(tmpl.JSONFilter(jsonIndentFixture(), arg)).To(Equal(expected))
		},
		Entry("two spaces", "  ", jsonIndentTwo),
		Entry("a tab", "\t", jsonIndentTab),
		Entry("an empty string means no indent", "", jsonIndentCompact),
		Entry("is cut to 10 characters", "abcdefghijklmn", withIndent("abcdefghij")),
		Entry("counts characters, not bytes", "éééééééééééé", withIndent("éééééééééé")),
	)

	DescribeTable("a number argument is that many spaces",
		func(arg interface{}, expected string) {
			Expect(tmpl.JSONFilter(jsonIndentFixture(), arg)).To(Equal(expected))
		},
		Entry("int 2", 2, jsonIndentTwo),
		Entry("int64 2 (a Liquid number literal)", int64(2), jsonIndentTwo),
		Entry("float64 2 (a number from a data file)", float64(2), jsonIndentTwo),
		Entry("4 spaces", 4, withIndent("    ")),
		Entry("a fraction is cut toward zero", 2.9, jsonIndentTwo),
		Entry("is capped at 10", 25, withIndent("          ")),
		Entry("0 means no indent", 0, jsonIndentCompact),
		Entry("below 1 means no indent", 0.5, jsonIndentCompact),
		Entry("a negative number means no indent", -3, jsonIndentCompact),
	)

	DescribeTable("any other argument is ignored",
		func(arg interface{}) {
			Expect(tmpl.JSONFilter(jsonIndentFixture(), arg)).To(Equal(jsonIndentCompact))
		},
		Entry("nil", nil),
		Entry("a bool", true),
		Entry("a map", map[string]interface{}{"indent": "  "}),
		Entry("a list", []interface{}{"  "}),
	)

	It("keeps file key order at every level when indenting", func() {
		// *ordered.Map writes its own JSON; the indent must not lose its
		// order. jsonIndentTwo is in file order, not alphabetical order.
		Expect(tmpl.JSONFilter(jsonIndentFixture(), "  ")).To(Equal(jsonIndentTwo))
	})

	It("reads only the first argument", func() {
		Expect(tmpl.JSONFilter(jsonIndentFixture(), "  ", "\t")).To(Equal(jsonIndentTwo))
	})

	It("indents a list and a scalar too", func() {
		Expect(tmpl.JSONFilter([]interface{}{"a", 1}, 2)).To(Equal("[\n  \"a\",\n  1\n]"))
		Expect(tmpl.JSONFilter("x", 2)).To(Equal(`"x"`))
	})

	It("still returns {} for a value JSON can't encode (guard)", func() {
		bad := map[string]interface{}{"f": func() {}}
		Expect(tmpl.JSONFilter(bad)).To(Equal("{}"))
		Expect(tmpl.JSONFilter(bad, 2)).To(Equal("{}"))
	})
})
